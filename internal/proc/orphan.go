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
)

var (
	prepOnce  sync.Once
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

// prepare: 처음 무리를 띄울 때 한 번 — 신호를 받으면 무리째 거두게 하고, 앞 실행의 고아 무리를 거둔다
func prepare() {
	prepOnce.Do(func() {
		if !groupsSupported {
			return
		}
		watchSignals()
		self(time.Time{})
		Reap()
	})
}

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

// record: 따로 띄운 무리 pg를 적는다(부모가 죽으면 다음 실행이 거둔다). 못 적으면 오류 — 부른 쪽은 그 자식에 명령을 시작시키지 않는다
func record(pg int, start, selfSt string) error {
	p := recPath(pg)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("고아 기록 폴더를 만들지 못했다: %w", err)
	}
	line := fmt.Sprintf("%d\t%s\t%d\t%s\n", pg, start, os.Getpid(), selfSt)
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
	if len(f) == 4 && f[2] == strconv.Itoa(os.Getpid()) {
		os.Remove(p)
	}
}

// orphanRec: 부모가 죽은 고아 기록 하나
type orphanRec struct {
	path      string
	pg, ppid  int
	leadStart string
}

// deadRecords: 기록 폴더에서 다른 실행(부모가 죽음)이 남긴 게시 완료 기록들. 숫자 pid 이름이 아닌 파일(다른 실행이 쓰는 중인
// 임시 파일 .<pid>.tmp… 등)은 건드리지도 읽지도 않는다(재감사 R-03·R-H06). 꼴이 틀린 기록도 그대로 둔다(우리 것이라는 근거가 없다)
func deadRecords() []orphanRec {
	d := RecordDir()
	if d == "" || !groupsSupported {
		return nil
	}
	es, err := os.ReadDir(d)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "%s: 경고 — 고아 기록 폴더를 읽지 못했다: %v\n", Who, err)
		}
		return nil
	}
	var r []orphanRec
	for _, e := range es {
		pg, err := strconv.Atoi(e.Name())
		if err != nil || pg <= 1 || strconv.Itoa(pg) != e.Name() {
			continue
		}
		p := filepath.Join(d, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue // 다른 실행이 막 지웠다
		}
		f := strings.Split(strings.TrimRight(string(b), "\n"), "\t")
		if len(f) != 4 || f[0] != e.Name() {
			continue
		}
		ppid, err := strconv.Atoi(f[2])
		if err != nil || ppid == os.Getpid() {
			continue
		}
		if Alive(ppid) {
			pst, err := StartTime(ppid)
			if err != nil || pst == f[3] {
				continue // 부모가 살아 있거나 모름 — 그 부모가 거둔다
			}
		}
		r = append(r, orphanRec{path: p, pg: pg, ppid: ppid, leadStart: f[1]})
	}
	return r
}

// Reap: 부모가 죽은 고아 기록마다 그 무리가 기록한 그것이면 거두고 기록을 지운다. 거둔 무리 수를 돌려준다
func Reap() int {
	n := 0
	for _, o := range deadRecords() {
		switch ownGroup(o.pg, o.leadStart, false) {
		case groupNone:
			os.Remove(o.path)
		case groupMine:
			fmt.Fprintf(os.Stderr, "%s: 죽은 실행(pid %d)이 남긴 자식 무리 %d 를 거둔다\n", Who, o.ppid, o.pg)
			StopGroup(o.pg, Grace)
			if groupGone(o.pg) {
				os.Remove(o.path)
				n++
			}
		}
	}
	return n
}

// Orphans: 앞 실행(부모가 죽음)이 남긴 무리를 거둔 뒤에도 그 우두머리가 아직 도는 것(거두지 못함·소유 모름)의 수.
// 0이 아니면 부른 쪽은 그 자식과 겹쳐 쓰지 않게 그 바퀴를 미룬다(75). 우두머리가 끝난 무리(입증할 수 없어 둔 기록)는 세지 않는다
func Orphans() int {
	prepare()
	n := 0
	for _, o := range deadRecords() {
		if Alive(o.pg) {
			n++
		}
	}
	return n
}
