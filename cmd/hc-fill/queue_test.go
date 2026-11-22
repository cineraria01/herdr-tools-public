package main

import (
	"os"
	"path/filepath"
	"testing"
)

func qtWrite(t *testing.T, p, s string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
}

func qtRead(t *testing.T, p string) string {
	t.Helper()
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// lockCode: 잠금 안(qHeld)에서 fn — exit·die가 올린 종료 코드(돌아오면 -1)
func lockCode(fn func()) (code int) {
	qHeld = true
	defer func() {
		qHeld = false
		code = -1
		if e, ok := recover().(lockExit); ok {
			code = e.code
		}
	}()
	fn()
	return -1
}

func qtSkipRoot(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root는 권한 없이도 읽는다")
	}
}

// R-21: 잠금 안에서 대기열이나 대기열 줄의 지시문을 못 읽으면 add는 사본 교체·줄 추가 전에 끝난다(같은 이름 두 줄·옛 지시문 덮음 없음)
func TestQAddUnreadable(t *testing.T) {
	qtSkipRoot(t)
	setData(t)
	DRC = 1
	add := func(name, tmp string) int {
		return qAdd(tmp, name, "app", DATA+"/briefs/added/"+name+".md", "-", "-", "wait", "-", "-", "-", nowMin(), "-", "-", "normal", "-", "-", "-")
	}
	bx := DATA + "/briefs/added/x.md"
	qtWrite(t, bx, "옛 지시문\n", 0o644)
	q := pad16("x", "app", bx, "-", "-", "wait", "-", "-", "-", "2026-11-22T10:00").Line() + "\n"
	// 대기열을 못 읽음(덧쓰기는 됨): 같은 이름을 「없음」으로 읽지 않는다
	qtWrite(t, Q, q, 0o200)
	tmp := bx + ".tmp.1"
	qtWrite(t, tmp, "새 지시문\n", 0o644)
	if rc := add("x", tmp); rc != 1 {
		t.Fatalf("못 읽은 대기열에 add: rc %d, 1이어야", rc)
	}
	if got := qtRead(t, Q); got != q {
		t.Fatalf("대기열이 바뀜: %q", got)
	}
	if got := qtRead(t, bx); got != "옛 지시문\n" || !exists(tmp) {
		t.Fatalf("옛 지시문을 사본으로 바꿨다: %q (사본 남음 %v)", got, exists(tmp))
	}
	// 다른 줄의 지시문을 못 읽음: z가 거기 이어 붙었는지(--then) 모른다 — 더하지 않는다
	by := DATA + "/briefs/added/y.md"
	qtWrite(t, by, "본문\n## 이어서: z (hc-fill add --then)\n", 0o000)
	q2 := q + pad16("y", "app", by, "-", "-", "wait", "-", "-", "-", "2026-11-22T10:00").Line() + "\n"
	qtWrite(t, Q, q2, 0o644)
	tz := DATA + "/briefs/added/z.md.tmp.1"
	qtWrite(t, tz, "z 지시문\n", 0o644)
	if rc := add("z", tz); rc != 1 {
		t.Fatalf("못 읽은 지시문: rc %d, 1이어야", rc)
	}
	if got := qtRead(t, Q); got != q2 || exists(DATA+"/briefs/added/z.md") || !exists(tz) {
		t.Fatalf("더했다: %q", got)
	}
	qtRead(t, by)
	// 다 읽히면 그대로 더한다(없는 지시문 파일은 「안 붙음」)
	if rc := add("w", ""); rc != 0 {
		t.Fatalf("정상 add: rc %d", rc)
	}
}

// R-29: 실제 쓰기 명령(prio·rm·옛 줄 변환)에서 고쳐 쓰기 전 재읽기가 실패하면 종료 1, 기존 대기열은 그대로
func TestWriteCmdsRereadFail(t *testing.T) {
	qtSkipRoot(t)
	defer func() { qRowsWHook = nil }()
	for _, c := range []struct {
		name string
		f11  string
		nth  int // 몇 번째 재읽기를 실패시키나(rm은 첫 확인 뒤의 고쳐 쓰기 읽기)
		run  func()
	}{
		{"prio", "-", 1, func() { prioCmd([]string{"x", "높음"}) }},
		{"rm", "-", 2, func() { qRm("x") }},
		{"옛 줄 변환", "slow:/srv/in/done/x.md", 1, func() {
			if rmigrate() != 0 {
				exit(9)
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			setData(t)
			DRC = 1
			q := pad16("x", "app", DATA+"/b.md", "-", "-", "wait", "-", "-", "-", "2026-11-22T10:00", c.f11).Line() + "\n" +
				pad16("y", "app", DATA+"/c.md", "-", "-", "wait", "-", "-", "-", "2026-11-22T10:00").Line() + "\n"
			qtWrite(t, Q, q, 0o644)
			n := 0
			qRowsWHook = func() {
				if n++; n == c.nth {
					if err := os.Chmod(Q, 0o000); err != nil {
						t.Fatal(err)
					}
				}
			}
			code := lockCode(c.run)
			qRowsWHook = nil
			if code != 1 {
				t.Fatalf("종료 %d, 1이어야(재읽기 %d번)", code, n)
			}
			if n < c.nth {
				t.Fatalf("재읽기가 %d번뿐 — 고쳐 쓰기 전 qRowsW를 거치지 않았다", n)
			}
			if got := qtRead(t, Q); got != q {
				t.Fatalf("대기열이 바뀜: %q", got)
			}
		})
	}
}
