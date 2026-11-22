package gitx

// gofix3-gitref(Go 전환 재감사 아스트라 2바퀴 ②-C): git 출력 판독 계약 — LsRemote·SplitZ·Tree·Int·PatchID 트리 확인.
// 가짜 SHA는 실행 중에 만든다(추적 파일 비밀 꼴 검사)

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// fakeOut: git ls-remote --exit-code 를 받으면 FAKE_OUT(printf %b)을 내고 FAKE_RC로 끝나는 가짜 git. 다른 꼴로 부르면 99
func fakeOut(t *testing.T, out string, rc int) {
	t.Helper()
	fakeGit(t, `[ "$1" = ls-remote ] && [ "$2" = --exit-code ] && [ "$3" = origin ] || exit 99
printf '%b' "$FAKE_OUT"; exit $FAKE_RC`)
	t.Setenv("FAKE_OUT", out)
	t.Setenv("FAKE_RC", strconv.Itoa(rc))
}

func TestLsRemoteContract(t *testing.T) {
	a, b := strings.Repeat("a", 40), strings.Repeat("b", 64)
	for _, c := range []struct {
		name, out string
		rc        int
		pats      []string
		want      map[string]string
		st        State
	}{
		{"정확 이름 한 줄", a + `\trefs/a\n`, 0, []string{"refs/a"}, map[string]string{"refs/a": a}, Yes},
		{"접두 둘(64자)", a + `\trefs/q/1\n` + b + `\trefs/q/2\n`, 0, []string{"refs/q/*"}, map[string]string{"refs/q/1": a, "refs/q/2": b}, Yes},
		{"빈 출력 2", "", 2, []string{"refs/a"}, map[string]string{}, No},
		{"빈 출력 0", "", 0, []string{"refs/a"}, nil, Unknown},
		{"2인데 출력", a + `\trefs/a\n`, 2, []string{"refs/a"}, nil, Unknown},
		{"짧은 SHA 이름 없음", `abcdef012345\n`, 0, []string{"refs/a"}, nil, Unknown},
		{"정상 줄 + 이름 없는 줄", a + `\trefs/q/1\n` + a + `\n`, 0, []string{"refs/q/*"}, nil, Unknown},
		{"요청 밖 이름", a + `\trefs/x\n`, 0, []string{"refs/a"}, nil, Unknown},
		{"정확 이름 끝 한 글자 잘림", a + `\trefs/hc-landed/ab\n`, 0, []string{"refs/hc-landed/abc"}, nil, Unknown},
		{"접두 밖", a + `\trefs/qx/1\n`, 0, []string{"refs/q/*"}, nil, Unknown},
		{"같은 이름 두 줄", a + `\trefs/a\n` + a + `\trefs/a\n`, 0, []string{"refs/a"}, nil, Unknown},
		{"끝 LF 없음", a + `\trefs/a`, 0, []string{"refs/a"}, nil, Unknown},
		{"대문자 16진", strings.ToUpper(a) + `\trefs/a\n`, 0, []string{"refs/a"}, nil, Unknown},
		{"탭 대신 공백", a + ` refs/a\n`, 0, []string{"refs/a"}, nil, Unknown},
		{"빈 줄 섞임", a + `\trefs/q/1\n\n`, 0, []string{"refs/q/*"}, nil, Unknown},
		{"이름에 공백", a + `\trefs/q/1 x\n`, 0, []string{"refs/q/*"}, nil, Unknown},
		{"128", "", 128, []string{"refs/a"}, nil, Unknown},
		{"124", a + `\trefs/a\n`, 124, []string{"refs/a"}, nil, Unknown},
	} {
		t.Run(c.name, func(t *testing.T) {
			fakeOut(t, c.out, c.rc)
			got, st := LsRemote("", "origin", nil, c.pats...)
			if st != c.st || (st != Unknown && !reflect.DeepEqual(got, c.want)) || (st == Unknown && got != nil) {
				t.Fatalf("LsRemote = %v %v, 바람 %v %v", got, st, c.want, c.st)
			}
		})
	}
}

func TestSplitZ(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
		ok   bool
	}{
		{"a\x00b\x00", []string{"a", "b"}, true},
		{"", []string{}, true},
		{"a\x00b", nil, false},
		{"a\x00\x00", nil, false},
		{"\x00", nil, false},
	} {
		got, ok := SplitZ(c.in)
		if ok != c.ok || (ok && !reflect.DeepEqual(got, c.want)) {
			t.Errorf("SplitZ(%q) = %q %v, 바람 %q %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestInt(t *testing.T) {
	for in, want := range map[string]int64{"1700000000\n": 1700000000, "0": 0, "42": 42} {
		if n, ok := Int(in); !ok || n != want {
			t.Errorf("Int(%q) = %d %v", in, n, ok)
		}
	}
	for _, in := range []string{"part", "", "\n", "-1", "+1", "1 2", " 1", "1\n2\n", "1\n\n", "99999999999999999999"} {
		if n, ok := Int(in); ok {
			t.Errorf("Int(%q)는 거절해야: %d", in, n)
		}
	}
}

// realRepo: 진짜 git으로 커밋 셋 — a(f=a) · b(f=b) · c(빈 커밋, 트리 = b)
func realRepo(t *testing.T) (dir, real, a, b, c string) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git 없음")
	}
	d := t.TempDir()
	for k, v := range map[string]string{"HOME": d, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null",
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"} {
		t.Setenv(k, v)
	}
	g := func(args ...string) string {
		c := exec.Command(real, args...)
		c.Dir = d
		o, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, o)
		}
		return strings.TrimSpace(string(o))
	}
	g("init", "-q")
	for i, s := range []string{"a\n", "b\n"} {
		if err := os.WriteFile(filepath.Join(d, "f"), []byte(s), 0o666); err != nil {
			t.Fatal(err)
		}
		g("add", "f")
		g("commit", "-qm", strconv.Itoa(i))
		if i == 0 {
			a = g("rev-parse", "HEAD")
		}
	}
	b = g("rev-parse", "HEAD")
	g("commit", "-q", "--allow-empty", "-m", "c")
	c = g("rev-parse", "HEAD")
	return d, real, a, b, c
}

func TestTree(t *testing.T) {
	d, real, a, _, _ := realRepo(t)
	if tr, rc := Tree(d, a, nil); rc != 0 || len(tr) != 40 {
		t.Fatalf("트리 ID: %q %d", tr, rc)
	}
	if _, rc := Tree(d, strings.Repeat("0", 40), nil); rc == 0 {
		t.Fatal("없는 커밋은 rc≠0")
	}
	fakeGit(t, `[ "$1" = rev-parse ] && { echo abc; exit 0; }; exec "`+real+`" "$@"`)
	if tr, rc := Tree(d, a, nil); rc == 0 {
		t.Fatalf("잘린 트리 ID는 rc≠0: %q", tr)
	}
}

// merger/R-202: 빈 diff는 두 트리가 같다고 따로 확인했을 때만 「-」 — 다르거나 확인 실패면 모름(rc≠0)
func TestPatchIDEmptyDiffNeedsSameTree(t *testing.T) {
	d, real, a, b, c := realRepo(t)
	t.Chdir(d)
	if p, rc := PatchID(nil, b, c); rc != 0 || p != "-" {
		t.Fatalf("트리 같음: 「-」·0이어야: %q %d", p, rc)
	}
	fakeGit(t, `[ "$1" = --literal-pathspecs ] && [ "$2" = diff ] && exit 0; exec "`+real+`" "$@"`)
	for i := 0; i < 2; i++ {
		if p, rc := PatchID(nil, a, b); rc == 0 {
			t.Fatalf("diff 빈 출력인데 트리 다름: 모름(rc≠0)이어야: %q %d", p, rc)
		}
	}
	fakeGit(t, `[ "$1" = --literal-pathspecs ] && [ "$2" = diff ] && exit 0; [ "$1" = rev-parse ] && exit 128; exec "`+real+`" "$@"`)
	if p, rc := PatchID(nil, b, c); rc == 0 {
		t.Fatalf("트리 조회 실패면 모름: %q %d", p, rc)
	}
}
