// Package selfexec: 오래 도는 고리가 새로 깔린 판으로 스스로 넘어간다(reexec-longrun, 2026-11-24 — 배포해도 화면 맞춤 감시·루프 감독이
// 손으로 다시 띄울 때까지 옛 판으로 돌았다). 뜰 때 잡은 자기 실행 파일의 stat이 바뀌었으면 같은 경로·인자·환경으로 exec한다 — pid·작업 폴더·
// 표준 입출력은 그대로라 launchd·지킴이·hc 터미널 어느 쪽이 띄웠든 그 자리에서 이어 돈다.
//
// 부르는 쪽은 바퀴 사이(이번 바퀴 일을 마치고 다음 바퀴 전)에서만 Check를 부른다. 그 자리에는 쥔 바퀴 잠금이 없다. 프로세스 수명 내내 쥔
// 커널 잠금(lockx)은 CLOEXEC라 exec 때 풀리고 새 판이 시작하며 다시 쥔다. 이 실행이 띄워 기다리는 자식(proc.Start·Run·Wait)이 있으면
// 하지 않는다 — exec하면 그 자식을 고아로 남긴다. 메모리 상태는 새 판에서 처음부터다.
package selfexec

import (
	"fmt"
	"os"
	"time"

	"herdr-tools/internal/proc"
	"herdr-tools/internal/resident"
	"herdr-tools/internal/version"
)

// Self: 지켜보는 자기 실행 파일. nil이면(경로를 못 잡음) Check는 아무것도 하지 않는다
type Self struct {
	path     string
	st, next os.FileInfo                                // 지금 판, 처음 본 바뀐 판(다음 바퀴에도 같으면 다 쓰인 것)
	exec     func(bin string, argv, env []string) error // 시험이 바꾼다
	log      func(line string)                          // 다시 실행·실패 한 줄
}

// New: 지금 실행 파일을 잡는다. log는 그 도구의 기록 줄(nil이면 표준 오류에 「who: …」). 못 잡으면 nil — Check가 그냥 지나간다
func New(who string, log func(line string)) *Self {
	p, err := os.Executable()
	if err != nil {
		return nil
	}
	s := at(who, p, log)
	if s != nil { // 판 표시(stale-resident-check): 이 판으로 떴다는 것을 상태 폴더에 — 스스로 넘어간 새 판도 New에서 다시 쓴다
		if err := resident.Save(who, p, s.st); err != nil {
			s.log("판 표시를 못 남김(" + err.Error() + ") — 상주 판 점검이 이 프로세스를 판 모름으로 본다")
		}
	}
	return s
}

func at(who, p string, log func(string)) *Self {
	st, err := os.Stat(p)
	if err != nil {
		return nil
	}
	if log == nil {
		log = func(l string) { fmt.Fprintf(os.Stderr, "%s: %s\n", who, l) }
	}
	return &Self{path: p, st: st, exec: execve, log: log}
}

// Check: 실행 파일이 바뀌었고 그 바뀐 stat이 두 바퀴 연속 같으면(손 cp·제자리 쓰기로 반쯤 쓴 판을 exec하지 않게) 새 판으로 exec.
// 성공하면 돌아오지 않는다. 실패하면 한 줄 남기고 그 판을 지금 판으로 삼아(같은 판으로 다시 시도하지 않는다) 돌아온다
func (s *Self) Check() {
	if s == nil {
		return
	}
	st, err := os.Stat(s.path)
	if err != nil || same(st, s.st) {
		s.next = nil // 못 읽음(까는 중)은 다음 바퀴에
		return
	}
	if s.next == nil || !same(st, s.next) {
		s.next = st
		return
	}
	if proc.Busy() {
		return // 띄워 기다리는 자식이 끝난 뒤 바퀴에
	}
	old := s.st.ModTime().Format(time.DateTime)
	if version.Commit != "" {
		old = version.Commit + " " + old
	}
	s.log(fmt.Sprintf("넘겨줌 %s 새 판이 깔렸다(옛 판 %s → 새 판 %s) — 새 판으로 이어 돈다", s.path, old, st.ModTime().Format(time.DateTime)))
	err = s.exec(s.path, os.Args, os.Environ())
	s.log(fmt.Sprintf("새 판으로 exec 실패(%v) — 이 판으로 계속", err))
	s.st, s.next = st, nil
}

func same(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.ModTime().Equal(b.ModTime()) && a.Size() == b.Size()
}
