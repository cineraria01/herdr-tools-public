package proc

// 고아 무리(2026-11-21 Go 전환 감사 M02·M03, 바탕 ①): Run이 상한을 걸려고 따로 무리로 띄운 자식은 부른 쪽 무리를 꺼도 안 꺼진다.
//   - 이 프로세스가 TERM·INT·HUP을 받으면(무시하게 떠 있지 않으면) 그 무리를 다 거둔 뒤 같은 신호로 끝난다(watchSignals).
//     도구가 따로 신호를 받아도(signal.Notify) 같은 신호는 그쪽에도 간다 — 도구 쪽이 먼저 끝내면 그것으로 끝이다.
//   - SIGKILL은 받을 수 없다 — 무리마다 기록 폴더에 「무리 번호·우두머리 시작 시각·부모 pid·부모 시작 시각」을 남기고 끝나면 지운다.
//     같은 사용자의 다음 실행(이 꾸러미로 무리를 처음 띄울 때)이 부모가 죽은 기록을 보고, 그 무리가 기록한 그것이면 거둔다(reap).
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

	"herdr-tools/internal/atomic"
)

var (
	prepOnce  sync.Once
	selfStart string // 이 프로세스의 시작 시각(기록용, 처음 한 번)
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

// prepare: 처음 무리를 띄울 때 한 번 — 신호를 받으면 무리째 거두게 하고, 앞 실행의 고아 무리를 거둔다
func prepare() {
	prepOnce.Do(func() {
		if !groupsSupported {
			return
		}
		watchSignals()
		st, err := StartTime(os.Getpid())
		if err == nil {
			selfStart = st
		}
		Reap()
	})
}

func watchSignals() {
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
	go func() {
		s := <-ch
		StopCurrent(Grace)
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

// record: 따로 띄운 무리 pg를 적는다(부모가 죽으면 다음 실행이 거둔다). 못 적으면 경고만 — 상한·신호로 거두는 것은 그대로다
func record(pg int, start string) {
	p := recPath(pg)
	if p == "" || selfStart == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "%s: 경고 — 고아 기록 폴더를 만들지 못했다: %v\n", Who, err)
		return
	}
	line := fmt.Sprintf("%d\t%s\t%d\t%s\n", pg, start, os.Getpid(), selfStart)
	if err := atomic.WriteFile(p, []byte(line), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "%s: 경고 — 고아 기록을 쓰지 못했다: %v\n", Who, err)
	}
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

// Reap: 부모가 죽은 고아 기록마다 그 무리가 기록한 그것이면 거두고 기록을 지운다. 거둔 무리 수를 돌려준다
func Reap() int {
	d := RecordDir()
	if d == "" || !groupsSupported {
		return 0
	}
	es, err := os.ReadDir(d)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "%s: 경고 — 고아 기록 폴더를 읽지 못했다: %v\n", Who, err)
		}
		return 0
	}
	n := 0
	for _, e := range es {
		p := filepath.Join(d, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue // 다른 실행이 막 지웠다
		}
		f := strings.Split(strings.TrimRight(string(b), "\n"), "\t")
		pg, e1 := strconv.Atoi(f[0])
		var ppid int
		var e2 error
		if len(f) == 4 {
			ppid, e2 = strconv.Atoi(f[2])
		}
		if len(f) != 4 || e1 != nil || e2 != nil || pg <= 1 || e.Name() != f[0] {
			os.Remove(p) // 우리 꼴이 아닌 기록(원자로 쓰니 반쪽은 없다)
			continue
		}
		if ppid == os.Getpid() {
			continue
		}
		if Alive(ppid) {
			pst, err := StartTime(ppid)
			if err != nil || pst == f[3] {
				continue // 부모가 살아 있거나 모름 — 그 부모가 거둔다
			}
		}
		switch ownGroup(pg, f[1]) {
		case groupNone:
			os.Remove(p)
		case groupMine:
			fmt.Fprintf(os.Stderr, "%s: 죽은 실행(pid %d)이 남긴 자식 무리 %d 를 거둔다\n", Who, ppid, pg)
			StopGroup(pg, Grace)
			if groupGone(pg) {
				os.Remove(p)
				n++
			}
		}
	}
	return n
}
