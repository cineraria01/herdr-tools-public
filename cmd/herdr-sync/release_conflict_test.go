package main

// sync-bin-conflict(2026-11-23): 기계에서 따로 바꾼 바이너리(bin.tsv의 마지막으로 깔아 준 sha와 다름)는 덮지 않고 충돌로.
// --take <기계>:.local/bin/<도구>면 덮고 덮기 전 판을 bak/<시각>/.local/bin/<도구>에 남긴다.
// 11-23 지휘가 손으로 옛 릴리스로 되돌린 hc-fill·hc-lane을 다음 herdr-sync가 다시 문제 판으로 덮어 hc-fill이 또 멈춘 일 뒤

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"testing"
)

const handBody = "#!/bin/sh\necho HAND\n"

// newArt: 새 릴리스 산출물(file:// 주소로)과 sha256
func newArt(t *testing.T, body string) (string, string) {
	t.Helper()
	p := t.TempDir() + "/demo"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte(body))
	return p, hex.EncodeToString(h[:])
}

// runShPre: 조각 앞에 변수 줄(take·stamp)을 붙여 돌린다
func runShPre(t *testing.T, home, pre string, args ...string) string {
	t.Helper()
	fb := t.TempDir()
	os.WriteFile(fb+"/uname", []byte(uname("arm64")), 0o755)
	c := exec.Command("sh", append([]string{"-c", pre + installSh, "_"}, args...)...)
	c.Env = append(os.Environ(), "HOME="+home, "PATH="+fb+":"+os.Getenv("PATH"))
	out, _ := c.Output()
	return string(out)
}

func TestInstallShHandReplaced(t *testing.T) {
	home, art, sum, src := instEnv(t)
	bin, st := home+"/.local/bin/demo", home+"/.local/state/herdr-sync/bin.tsv"
	// 기록 없음(처음 깖): 옛 본체가 있어도 지금처럼 바꾼다
	if out := runShPre(t, home, "", "set", "c0", src, "arm64", "demo", "file://"+art, sum); out != "W\tdemo\n" {
		t.Fatalf("기록 없음 → 깖: %q", out)
	}
	art2, sum2 := newArt(t, "#!/bin/sh\necho NEW2\n")
	// 같음(본체가 마지막으로 깔아 준 판): 새 판으로 바꾼다
	if out := runShPre(t, home, "", "set", "c1", src+"1", "arm64", "demo", "file://"+art2, sum2); out != "W\tdemo\n" || readOr(bin) != "#!/bin/sh\necho NEW2\n" {
		t.Fatalf("기록과 같음 → 바꿈: %q %q", out, readOr(bin))
	}
	// 손 교체: 새 판이 와도 덮지 않고 충돌 — --check도 같은 판정
	os.WriteFile(bin, []byte(handBody), 0o755)
	rec := readOr(st)
	art3, sum3 := newArt(t, "#!/bin/sh\necho NEW3\n")
	for _, mode := range []string{"check", "set"} {
		if out := runShPre(t, home, "", mode, "c2", src+"2", "arm64", "demo", "file://"+art3, sum3); out != "C\tdemo\t따로 바뀜\n" {
			t.Errorf("손 교체 %s: %q", mode, out)
		}
	}
	if readOr(bin) != handBody || readOr(st) != rec || len(leftovers(home)) != 0 {
		t.Fatalf("충돌인데 썼다: 본체 %q 기록 %q 남은 것 %v", readOr(bin), readOr(st), leftovers(home))
	}
	// 덮을 다른 도구만 --take면 이 도구는 그대로 충돌
	if out := runShPre(t, home, "take=' other ' stamp=20261123-040000\n", "set", "c2", src+"2", "arm64", "demo", "file://"+art3, sum3); out != "C\tdemo\t따로 바뀜\n" {
		t.Errorf("다른 도구 --take: %q", out)
	}
	// --take: 덮고 덮기 전 판을 bak/<시각>/.local/bin/demo에
	out := runShPre(t, home, "take=' demo ' stamp=20261123-040000\n", "set", "c2", src+"2", "arm64", "demo", "file://"+art3, sum3)
	if out != "W\tdemo\t따로 바뀐 판을 덮음 — 옛 판은 bak/20261123-040000/.local/bin/demo\n" {
		t.Fatalf("--take: %q", out)
	}
	if readOr(bin) != "#!/bin/sh\necho NEW3\n" || readOr(home+"/.local/state/herdr-sync/bak/20261123-040000/.local/bin/demo") != handBody ||
		readOr(st) != "demo\t"+sum3+"\t"+src+"2\n" {
		t.Fatalf("--take 뒤: 본체 %q 백업 %q 기록 %q", readOr(bin), readOr(home+"/.local/state/herdr-sync/bak/20261123-040000/.local/bin/demo"), readOr(st))
	}
	// 손 교체한 것이 마침 새 판과 같으면 충돌 아님(같음, 기록을 맞춘다)
	os.WriteFile(bin, []byte("#!/bin/sh\necho NEW4\n"), 0o755)
	art4, sum4 := newArt(t, "#!/bin/sh\necho NEW4\n")
	if out := runShPre(t, home, "", "set", "c3", src+"3", "arm64", "demo", "file://"+art4, sum4); out != "S\tdemo\n" || readOr(st) != "demo\t"+sum4+"\t"+src+"3\n" {
		t.Errorf("새 판과 같은 손 교체: %q %q", out, readOr(st))
	}
	// 지움(본체 없음): 지금처럼 깐다
	os.Remove(bin)
	if out := runShPre(t, home, "", "set", "c2", src+"2", "arm64", "demo", "file://"+art3, sum3); out != "W\tdemo\n" {
		t.Errorf("본체 없음: %q", out)
	}
}

// SHA를 못 읽으면(모름) 바꾸지 않고 충돌 — 기록이 없어도
func TestInstallShUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root는 권한 000도 읽는다")
	}
	home, art, sum, src := instEnv(t)
	bin := home + "/.local/bin/demo"
	os.Chmod(bin, 0)
	defer os.Chmod(bin, 0o755)
	if out := runShPre(t, home, "", "set", "c0", src, "arm64", "demo", "file://"+art, sum); out != "C\tdemo\t해시를 읽지 못함\n" {
		t.Fatalf("못 읽음: %q", out)
	}
	os.Chmod(bin, 0o755)
	if readOr(bin) != oldBody {
		t.Fatalf("못 읽는데 덮었다: %q", readOr(bin))
	}
}

// installReport: C는 set·--check 모두 충돌(못 깖이 아님 — 설정·지움을 미루지 않는다), W의 덮음 메모는 받음
func TestInstallReportConflict(t *testing.T) {
	ps := installReport("w1", []string{"a", "b"}, "C\ta\t따로 바뀜\nW\tb\t따로 바뀐 판을 덮음 — 옛 판은 bak/x/.local/bin/b\n")
	if len(ps) != 2 || !ps[0].conf || ps[0].bad || ps[0].s != "충돌 w1:.local/bin/a (따로 바뀜)" || ps[1].bad || ps[1].conf ||
		ps[1].s != "w1 받음 b (따로 바뀐 판을 덮음 — 옛 판은 bak/x/.local/bin/b)" {
		t.Fatalf("%+v", ps)
	}
}
