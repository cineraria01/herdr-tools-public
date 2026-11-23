package proc

// 고아 무리(2026-11-21 Go 전환 감사 M02·M03, 바탕 ①): Run이 상한을 걸려고 따로 무리로 띄운 자식은 부른 쪽 무리를 꺼도 안 꺼진다.
//   - 이 프로세스가 TERM·INT·HUP을 받으면(무시하게 떠 있지 않으면) 그 무리를 다 거둔 뒤 같은 신호로 끝난다(watchSignals).
//     도구가 신호를 스스로 받으면 TakeSignals로 이 처리기를 두지 않는다 — 두 처리기가 서로 다른 유예로 다투지 않게(재감사 R-H04).
//   - SIGKILL은 받을 수 없다 — 무리마다 기록 폴더에 「무리 번호·우두머리 시작 시각·부모 pid·부모 시작 시각」을 남기고 끝나면 지운다.
//     자식은 이 기록이 디스크에 다 쓰인 뒤에야 실제 명령을 시작한다(gate — 재감사 R-02·R-H05). 기록을 못 쓰면 띄우지 않는다(모름).
//     같은 사용자의 다음 실행(이 꾸러미로 무리를 처음 띄울 때)이 부모가 죽은 기록을 보고, 우두머리가 기록한 그것(시작 시각이 같음)이면 거둔다.
//     우두머리가 이미 끝났거나 기록에 시작 시각이 없으면 그 무리가 기록한 그것인지 입증할 수 없다 — 건드리지 않고 기록을 둔다(R-C02·R-07).
//     모르면(ps를 못 읽음) 건드리지 않고 기록을 둔다 — 다음 실행이 다시 본다.

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/oplog"
)

var (
	prepMu    sync.Mutex
	prepared  bool // 준비가 끝까지 성공했다(실패하면 다음 호출이 다시 한다 — 재감사 2바퀴 P2)
	sigOnce   sync.Once
	selfMu    sync.Mutex
	selfStart string // 이 프로세스의 시작 시각(기록용)

	sigMu     sync.Mutex
	sigOwned  bool           // 도구가 신호를 스스로 받는다(TakeSignals)
	sigCh     chan os.Signal // proc 처리기가 받는 관(없으면 nil)
	stopHooks []func()       // 신호로 끝내기 직전(무리를 다 거둔 뒤) 부를 일(OnStop)
)

// RecordDir: 고아 기록 폴더 — HERDR_PROC_DIR, 없으면 사용자 캐시 폴더 아래. 빈 값이면 기록하지 않는다
func RecordDir() string {
	if d := os.Getenv("HERDR_PROC_DIR"); d != "" {
		return d
	}
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "herdr-tools", "proc")
}

// TakeSignals: 도구가 TERM·INT·HUP을 스스로 받는다(signal.Notify 뒤 StopCurrent를 부르고 끝냄) — proc의 처리기를 두지 않는다.
// 같은 신호를 두 처리기가 받으면 먼저 끝나는 쪽(짧은 유예)이 다른 쪽이 거두던 무리를 남긴 채 프로세스를 끝냈다(재감사 R-H04)
func TakeSignals() {
	sigMu.Lock()
	defer sigMu.Unlock()
	sigOwned = true
	if sigCh != nil {
		signal.Stop(sigCh)
		sigCh = nil
	}
}

// OnStop: proc 처리기가 신호로 끝내기 직전(무리를 다 거둔 뒤) f를 부른다 — 임시 폴더 지우기 등(재감사 ops R-10).
// TakeSignals를 부른 도구는 제 처리기에서 스스로 한다
func OnStop(f func()) {
	sigMu.Lock()
	defer sigMu.Unlock()
	stopHooks = append(stopHooks, f)
}

// prepare: 처음 무리를 띄울 때 — 신호를 받으면 무리째 거두게 하고, 이 프로세스의 시작 시각을 읽고, 앞 실행의 고아 무리를 거둔다.
// 모두 deadline 안에서(빈 값이면 마감 없음) — 넘기면 오류, 준비는 다음 호출이 다시 한다(성공해야 닫힌다 — 재감사 2바퀴 P2·ops R-212·merger R-214)
func prepare(deadline time.Time) error {
	if !groupsSupported {
		return nil
	}
	sigOnce.Do(watchSignals)
	prepMu.Lock()
	defer prepMu.Unlock()
	if prepared {
		return nil
	}
	if _, err := self(deadline); err != nil {
		return fmt.Errorf("이 프로세스의 시작 시각: %w", err)
	}
	sweep(deadline)
	if past(deadline) {
		return fmt.Errorf("앞 실행의 고아 무리 확인: %w", ErrTimeout)
	}
	prepared = true
	return nil
}

// past: 마감이 있고 지났다
func past(deadline time.Time) bool { return !deadline.IsZero() && !time.Now().Before(deadline) }

// self: 이 프로세스의 시작 시각(처음 한 번 읽어 둔다 — 못 읽었으면 부를 때마다 다시). deadline이 있으면 그 안에서만
func self(deadline time.Time) (string, error) {
	selfMu.Lock()
	defer selfMu.Unlock()
	if selfStart == "" {
		st, err := startTimeBy(os.Getpid(), deadline)
		if err != nil {
			return "", err
		}
		selfStart = st
	}
	return selfStart, nil
}

func watchSignals() {
	sigMu.Lock()
	defer sigMu.Unlock()
	if sigOwned {
		return
	}
	var sigs []os.Signal
	for _, s := range stopSignals {
		if !signal.Ignored(s) {
			sigs = append(sigs, s)
		}
	}
	if len(sigs) == 0 {
		return
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sigs...)
	sigCh = ch
	go func() {
		s := <-ch
		StopCurrent(Grace)
		sigMu.Lock()
		hs := append([]func(){}, stopHooks...)
		sigMu.Unlock()
		for _, f := range hs {
			f()
		}
		reraise(s)
	}()
}

func recPath(pg int) string {
	d := RecordDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, strconv.Itoa(pg))
}

// keepMark: 다섯째 칸 — 끝까지 가게 둔 자식(상한 없는 Wait·WaitFor(…, keep) — 병합기 바퀴 일꾼·착지·게시기)이다. 부모가 죽어도 그 일은
// 끝까지 가게 둔다: 다음 실행은 거두지 않고 그 무리가 기록한 그것인 동안 센다(Orphans — 75로 미룸, 재감사 2바퀴 P3). 아니면 「-」
const keepMark = "keep"

// record: 따로 띄운 무리 pg를 적는다(부모가 죽으면 다음 실행이 거두거나, keep이면 끝날 때까지 센다). 못 적으면 오류 — 부른 쪽은 그
// 자식에 명령을 시작시키지 않는다. 칸: 무리 · 우두머리 시작 시각 · 부모 pid · 부모 시작 시각 · keep|- · 도구(Who). 기록 폴더는 도구가
// 함께 쓴다 — 남은 무리는 그것을 남긴 도구의 바퀴만 미룬다(재감사 3바퀴 fill/R-3M02·merger/R-3M01)
func record(pg int, start, selfSt string, keep bool) error {
	p := recPath(pg)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("고아 기록 폴더를 만들지 못했다: %w", err)
	}
	mark := "-"
	if keep {
		mark = keepMark
	}
	who := strings.NewReplacer("\t", " ", "\n", " ").Replace(Who)
	line := fmt.Sprintf("%d\t%s\t%d\t%s\t%s\t%s\n", pg, start, os.Getpid(), selfSt, mark, who)
	if err := atomic.WriteFile(p, []byte(line), 0o600); err != nil {
		return fmt.Errorf("고아 기록을 쓰지 못했다: %w", err)
	}
	return nil
}

// unrecord: 무리 pg의 기록을 지운다(이 프로세스가 쓴 것만)
func unrecord(pg int) {
	p := recPath(pg)
	if p == "" {
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	f := strings.Split(strings.TrimRight(string(b), "\n"), "\t")
	if recShape(f) && f[2] == strconv.Itoa(os.Getpid()) {
		os.Remove(p)
	}
}

// recShape: 기록 칸 꼴 — 넷·다섯째가 keep(옛 꼴, 도구 모름), 또는 여섯(다섯째 keep|-, 여섯째 도구 이름)
func recShape(f []string) bool {
	switch len(f) {
	case 4:
		return true
	case 5:
		return f[4] == keepMark
	case 6:
		return (f[4] == keepMark || f[4] == "-") && f[5] != ""
	}
	return false
}

// orphanRec: 부모가 죽은 고아 기록 하나
type orphanRec struct {
	path      string
	pg, ppid  int
	leadStart string
	keep      bool
	who       string // 남긴 도구(옛 꼴은 빈 값 — 누구의 것인지 모른다)
}

// mine: 이 도구의 바퀴가 셀 기록인가 — 이 도구가 남겼거나 도구를 모르는 옛 꼴
func (o orphanRec) mine() bool { return o.who == "" || o.who == Who }

func (o orphanRec) String() string { return fmt.Sprintf("무리 %d(기록 %s)", o.pg, o.path) }

// deadRecords: 기록 폴더에서 다른 실행(부모가 죽음)이 남긴 게시 완료 기록들과, 읽지 못해 모르는 것(unknown — 글).
// 숫자 pid 이름이 아닌 파일(다른 실행이 쓰는 중인 임시 파일 .<pid>.tmp… 등)은 건드리지도 읽지도 않는다(재감사 R-03·R-H06).
// 폴더(없음 말고)·기록을 못 읽거나, 꼴이 틀리거나, 부모가 살았는지 모르면 모름 — 지우지 않고 센다(재감사 2바퀴 P1·merger R-208).
// 부모가 살았는지 모르는 기록은 그것을 남긴 도구(또는 옛 꼴)일 때만 센다
func deadRecords(deadline time.Time) (r []orphanRec, unknown []string) {
	d := RecordDir()
	if d == "" || !groupsSupported {
		return nil, nil
	}
	es, err := os.ReadDir(d)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		note(oplog.Warn, "경고 — 고아 기록 폴더를 읽지 못했다(모름): %v", err)
		return nil, []string{"기록 폴더 " + d + "(읽지 못함)"}
	}
	for _, e := range es {
		pg, err := strconv.Atoi(e.Name())
		if err != nil || pg <= 1 || strconv.Itoa(pg) != e.Name() {
			continue
		}
		p := filepath.Join(d, e.Name())
		b, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue // 다른 실행이 막 지웠다
		}
		f := strings.Split(strings.TrimRight(string(b), "\n"), "\t")
		ppid, perr := 0, error(nil)
		ok := err == nil && recShape(f) && f[0] == e.Name()
		if ok {
			ppid, perr = strconv.Atoi(f[2])
		}
		if !ok || perr != nil {
			if err == nil {
				err = errors.New("꼴이 틀림")
			}
			note(oplog.Warn, "경고 — 고아 기록 %s 를 읽지 못했다(모름, 그대로 둔다): %v", p, err)
			unknown = append(unknown, "기록 "+p+"(읽지 못함)")
			continue
		}
		if ppid == os.Getpid() {
			continue
		}
		o := orphanRec{path: p, pg: pg, ppid: ppid, leadStart: f[1], keep: len(f) >= 5 && f[4] == keepMark}
		if len(f) == 6 {
			o.who = f[5]
		}
		if Alive(ppid) {
			pst, err := startTimeBy(ppid, deadline)
			if err != nil {
				if o.mine() {
					unknown = append(unknown, o.String()+" 부모 생존 모름") // 그 부모의 것일 수도, 죽은 실행의 것일 수도
				}
				continue
			}
			if pst == f[3] {
				continue // 부모가 살아 있다 — 그 부모가 거둔다
			}
		}
		r = append(r, o)
	}
	return r, unknown
}

// bootTime: pid 1의 시작 시각(≈ 부팅). 못 읽으면 거짓 — 모름: 어떤 기록도 「부팅 전」이라 하지 않는다
func bootTime(deadline time.Time) (time.Time, bool) {
	s, err := startTimeBy(1, deadline)
	if err != nil {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(strings.Fields(s), " "), time.Local)
	return t, err == nil
}

// beforeBoot: 기록 파일이 부팅보다 (1분 넘게) 먼저 쓰였다 — 그 무리는 재부팅으로 이미 없다(번호를 지금 쓰는 무리는 남이다). 모르면 거짓
func beforeBoot(p string, boot time.Time) bool {
	st, err := os.Stat(p)
	return err == nil && !boot.IsZero() && st.ModTime().Before(boot.Add(-time.Minute))
}

// sweep: 부모가 죽은 고아 기록마다 —
//   - 부팅 전에 쓰인 기록은 지운다(재부팅으로 남음, 재감사 3바퀴 merger/R-3M01).
//   - 그 무리가 기록한 그것(우두머리가 살아 있고 시작 시각이 같음)이면 거두고, 무리가 사라졌거나 우두머리 번호를 시작 시각이 다른
//     프로세스가 쓰면(번호 재사용) 기록을 지운다. 우두머리가 끝났는데 무리가 남았거나 소유를 모르면 신호 없이 둔다.
//   - keep 기록(끝까지 가게 둔 일)은 거두지 않는다 — 그것이 아직 돌거나 모르면 두고 세며, 사라졌거나 번호가 다시 쓰였으면 지운다
//     (재감사 3바퀴 base/R-3M03·ops/R-3M01 — 옛 판은 무리 번호만 봐 번호를 다시 쓴 무리가 사는 동안 내내 셌다).
//
// reaped = 거둔 수, left = 이 도구(Who)가 셀 남은 무리·모르는 기록의 글(무리 번호·기록 경로)
func sweep(deadline time.Time) (reaped int, left []string) {
	rs, left := deadRecords(deadline)
	var boot time.Time
	if len(rs) > 0 {
		boot, _ = bootTime(deadline)
	}
	for _, o := range rs {
		if beforeBoot(o.path, boot) {
			note(oplog.Warn, "부팅 전에 쓰인 고아 기록 %s 를 지운다(그 무리는 재부팅으로 없다)", o.path)
			os.Remove(o.path)
			continue
		}
		st := ownGroupBy(o.pg, o.leadStart, false, deadline)
		if o.keep {
			if st == groupNone {
				os.Remove(o.path)
			} else if o.mine() {
				left = append(left, o.String())
			}
			continue
		}
		switch st {
		case groupNone:
			os.Remove(o.path)
		case groupMine:
			note(oplog.Warn, "죽은 실행(pid %d)이 남긴 자식 무리 %d 를 거둔다", o.ppid, o.pg)
			StopGroup(o.pg, Grace)
			if groupGone(o.pg) {
				os.Remove(o.path)
				reaped++
			} else if o.mine() {
				left = append(left, o.String())
			}
		default:
			if o.mine() {
				left = append(left, o.String())
			}
		}
	}
	return reaped, left
}

// Reap: 부모가 죽은 고아 기록마다 그 무리가 기록한 그것이면 거두고 기록을 지운다. 거둔 무리 수를 돌려준다
func Reap() int {
	n, _ := sweep(time.Time{})
	return n
}

// Orphans: 앞 실행(부모가 죽음)이 남긴 무리를 거둔 뒤에도 남은 것 가운데 이 도구(Who, 또는 도구를 모르는 옛 꼴)의 것의 수 — 우두머리
// 생존과 상관없이 무리가 아직 있거나(거두지 못함·소유를 입증 못 함), 기록 폴더·기록을 못 읽었거나, 꼴이 틀린 기록(모름). 0이 아니면
// 부른 쪽은 그 자식과 겹쳐 쓰지 않게 그 바퀴를 미룬다(75). 부를 때마다 다시 본다(재감사 2바퀴 P1). 다른 도구가 남긴 무리는 이 도구의
// 자식이 아니라 세지 않는다(재감사 3바퀴 fill/R-3M02 — 옛 판은 같은 기계의 다른 도구가 남긴 무리에 hc-fill이 매 바퀴 75, 사건 없음)
func Orphans() int { return len(Left()) }

// Left: Orphans가 세는 것들의 글(무리 번호·기록 경로, 기록 이름순) — 같은 기록이면 같은 글이라 부른 쪽이 같은 까닭으로 쉰 바퀴를 센다
func Left() []string {
	if err := prepare(time.Time{}); err != nil {
		note(oplog.Warn, "경고 — 준비를 마치지 못했다: %v", err)
	}
	_, left := sweep(time.Time{})
	return left
}
