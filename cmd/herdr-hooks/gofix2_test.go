package main

// gofix2-hooks(2026-11-22 Go 전환 재감사 아스트라 1바퀴) 단위 시험 — 지적 ID는 재감사 보고서의 것(R-H·R-M·R-L).

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
)

// R-M01: 기한에 걸리거나 직접 자식이 먼저 끝나도 그 손자까지 거둔다 — 돌아온 뒤 손자가 완료 표식을 쓰면 안 된다.
// 재감사 R-M207: 표식 경로는 셸 글에 잇지 않고 따로 된 인자로($1·$2 — 공백 든 GOTMPDIR·TMPDIR에서도 같은 경로), 손자 시작 표식을 먼저 보고,
// 부재는 ENOENT만(clitest.Absent — 다른 오류는 모름이라 멈춘다)
func TestHookRunReapsGrandchildren(t *testing.T) {
	d := clitest.TempDir(t)
	for _, x := range []struct {
		name   string
		budget time.Duration
		script string
	}{
		{"기한", 500 * time.Millisecond, `(: >"$1"; sleep 1; : >"$2") & sleep 5`},
		{"자식 먼저 끝남", 4 * time.Second, `(: >"$1"; sleep 1; : >"$2") &`},
	} {
		hookBudget(x.budget)
		t0 := time.Now()
		_, _, rc := hookRun([]string{"sh"}, 0, nil, "", "/bin/sh", "-c", x.script, "sh", d+"/"+x.name+" 시작", d+"/"+x.name+" 끝")
		if rc != timeoutRC || time.Since(t0) > 900*time.Millisecond {
			t.Errorf("%s: rc=%d(want %d 모름), %v(want 기한·관 기다림 안)", x.name, rc, timeoutRC, time.Since(t0))
		}
		clitest.WaitFile(t, d+"/"+x.name+" 시작", 2*time.Second) // 원인(손자)이 실제로 떴다
	}
	time.Sleep(2 * time.Second) // 손자가 살아 있었다면 1초 뒤 표식을 썼다
	for _, m := range []string{"기한", "자식 먼저 끝남"} {
		if !clitest.Absent(t, d+"/"+m+" 끝") {
			t.Errorf("훅이 돌아온 뒤 손자가 표식 %q를 썼다", m+" 끝")
		}
	}
	if es, err := os.ReadDir(d); err != nil || len(es) != 2 {
		t.Errorf("표식은 예상 경로(시작 둘)에만 생겨야: %v %v", es, err)
	}
	hookBudget(4 * time.Second)
	if out, _, rc := hookRun([]string{"sh"}, 0, nil, "", "/bin/sh", "-c", "echo ok"); rc != 0 || out != "ok\n" {
		t.Errorf("정상 명령 rc=%d out=%q", rc, out)
	}
}

// R-M09: 표본 번호 지도는 읽기 한 번의 값 — 앞 읽기(같은 명령이 #1)의 번호가 뒤 읽기(#2)에 남지 않는다
func TestCorpusIdxPerRead(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(d+"/c1", []byte("echo SYNTH_ONLY_MARK\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d+"/c2", []byte("a\x00echo SYNTH_ONLY_MARK\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_HOOKS_CORPUS", d+"/c1")
	corpus(t)
	t.Setenv("HERDR_HOOKS_CORPUS", d+"/c2")
	_, idx := corpus(t)
	if s, _ := idx.show("echo SYNTH_ONLY_MARK"); s != "HERDR_HOOKS_CORPUS #2" {
		t.Errorf("뒤 읽기의 번호 %q(want #2)", s)
	}
}

// logRec: 실패 로그를 받아 적는 reporter
type logRec struct{ b strings.Builder }

func (r *logRec) Helper() {}
func (r *logRec) Errorf(f string, a ...any) {
	fmt.Fprintf(&r.b, f+"\n", a...)
}

// R-M08: 통제된 불일치를 실제 로그 경로(checkHeredoc → diffErr)에 넣는다 — 실제 명령은 원문·출력 조각 없이 올바른 번호만,
// 합성 명령은 그대로 보인다
func TestCorpusLogPath(t *testing.T) {
	in := []string{"cat <<EOF\nSYNTH_ONLY_MARK\nEOF", "plain synthetic"}
	r := &logRec{}
	checkHeredoc(r, corpusIdx{in[0]: 3}, in, []string{"WANT_FRAGMENT_X", "WANT_FRAGMENT_Y"})
	log := r.b.String()
	for _, bad := range []string{"SYNTH_ONLY_MARK", "WANT_FRAGMENT_X", "cat <<EOF"} {
		if strings.Contains(log, bad) {
			t.Errorf("실제 명령의 조각 %q이 로그에 나왔다:\n%s", bad, log)
		}
	}
	for _, good := range []string{"HERDR_HOOKS_CORPUS #3", "WANT_FRAGMENT_Y", "plain synthetic"} {
		if !strings.Contains(log, good) {
			t.Errorf("로그에 %q이 없다:\n%s", good, log)
		}
	}
}

// R-H05: status -z 레코드를 끝까지 검증한다
func TestParseStatusZ(t *testing.T) {
	for _, x := range []struct {
		in   string
		want string // 경로를 |로 이은 것, "!"면 실패
	}{
		{"", ""},
		{" M a\x00?? b c\x00", "a|b c"},
		{"R  new\x00old\x00 M x\x00", "new|old|x"},
		{"C  cp\x00src\x00", "cp"},
		{" R wn\x00wo\x00", "wn|wo"},
		{"R  new\x00", "!"},       // 원래 이름 레코드 빠짐
		{"R  new\x00\x00", "!"},   // 원래 이름이 빈 값
		{" M a", "!"},             // NUL로 끝나지 않음
		{" M \x00", "!"},          // 빈 경로
		{"ZZ a\x00", "!"},         // 상태 글자 아님
		{" Ma\x00", "!"},          // 구분자 없음
		{"\x00", "!"},             // 빈 레코드
		{" M a\x00R  b\x00", "!"}, // 마지막 이름 바꿈의 짝 빠짐
		{"!! ign\x00?? u\x00", "ign|u"},
	} {
		got, ok := parseStatusZ(x.in)
		s := strings.Join(got, "|")
		if !ok {
			s = "!"
		}
		if s != x.want {
			t.Errorf("%q → %q(want %q)", x.in, s, x.want)
		}
	}
}

// R-H02: hc ls 응답은 완전한 목록만 — 어긋나면 까닭(모름)
func TestHcTerminals(t *testing.T) {
	for in, ok := range map[string]bool{
		`{"terminals":[]}`: true,
		`{"terminals":[{"state":"live","address":"t1","agent":"claude","cwd":"/r","name":null}]}`: true,
		`{"terminals":[{"state":"finished"}]}`:                                                    true,
		``:                                                                                        false,
		`{}`:                                                                                      false,
		`null`:                                                                                    false,
		`{"terminals":null}`:                                                                      false,
		`{"terminals":{}}`:                                                                        false,
		`{"terminals":["t1"]}`:                                                                    false,
		`{"terminals":[{"address":"t1"}]}`:                                                        false,
		`{"terminals":[{"state":"live","agent":"claude"}]}`:      false,
		`{"terminals":[{"state":"live","address":""}]}`:          false,
		`{"terminals":[{"state":"live","address":"t","cwd":7}]}`: false,
		`{"terminals":[]}{"terminals":[]}`:                       false,
		`{"terminals":[]} x`:                                     false,
	} {
		if _, why := hcTerminals(in); (why == "") != ok {
			t.Errorf("%q: 까닭 %q(want 읽음=%v)", in, why, ok)
		}
	}
}

// R-H01: hc get 응답 꼴
func TestHcTerminal(t *testing.T) {
	for in, ok := range map[string]bool{
		`{"address":"t1"}`: true, `{"address":"t1","name":null,"labels":null}`: true, `{"address":"t1","labels":{"job":"x"}}`: true,
		``: false, `{}`: false, `null`: false, `[]`: false, `{"name":"w"}`: false, `{"address":"t1","name":7}`: false,
		`{"address":"t1","labels":"loop"}`: false, `{"address":"t1","labels":{"job":1}}`: false, `{"address":"t1"} x`: false,
	} {
		if _, _, got := hcTerminal(in); got != ok {
			t.Errorf("%q: 읽음=%v(want %v)", in, got, ok)
		}
	}
}

// R-M12: 로캘 셋 — C·POSIX·빈 값은 바이트, UTF-8은 글자, 그 밖은 모름. widePat은 ?·[…]를 *로
func TestLocaleKind(t *testing.T) {
	for v, want := range map[string]int{"": locByte, "C": locByte, "POSIX": locByte, "ko_KR.UTF-8": locUTF8, "C.UTF-8": locUTF8,
		"en_US.utf8": locUTF8, "ko_KR.eucKR": locOther, "en_US.ISO8859-1": locOther, "ko_KR": locOther} {
		t.Setenv("LC_ALL", v)
		t.Setenv("LC_CTYPE", "")
		t.Setenv("LANG", "")
		if got := localeKind(); got != want {
			t.Errorf("LC_ALL=%q → %d(want %d)", v, got, want)
		}
	}
	saved := patByte
	t.Cleanup(func() { patByte = saved })
	patByte = true
	for p, want := range map[string]string{"?.txt": "*.txt", "[ab]x": "*x", "a[b": "a[b", `\?x`: `\?x`, "[!a]?": "**"} {
		if got := widePat(p); got != want {
			t.Errorf("widePat(%q)=%q(want %q)", p, got, want)
		}
	}
}
