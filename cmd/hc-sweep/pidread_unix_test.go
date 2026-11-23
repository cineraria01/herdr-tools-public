//go:build !windows

package main

// 2026-11-24 sweep-pid-read: 「보류: 터미널 pid를 못 읽음」 원인 재현. 터미널 쓸기가 이번 바퀴에 닫은 터미널은 고아 쓸기가 받은
// hc ls 목록(닫기 전)에 live로 남아, 그 hc get이 not_found로 끝나 바퀴 전체가 보류였다(실측: 보류 82바퀴 모두 같은 바퀴에 닫힘,
// pid를 읽은 244바퀴는 닫힘 0). 끝난 터미널은 pid 모음에서 빼고, 그 터미널 cwd의 작업 트리에 걸린 후보만 보류한다

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// prOut: f가 표준 출력에 찍은 것
func prOut(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	f()
	os.Stdout = old
	w.Close()
	return <-done
}

// prOrphan: cwd가 wt이고 명령 줄에 wt 아래 경로가 든, 부모가 1인 고아 대역(2분 뒤 스스로 끝)
func prOrphan(t *testing.T, wt string) int {
	t.Helper()
	pidf := filepath.Join(t.TempDir(), "pid")
	sp := exec.Command("sh", "-c", `"$0" "$1" </dev/null >/dev/null 2>&1 & echo $! > "$2"`, os.Args[0], filepath.Join(wt, "marker"), pidf)
	sp.Dir = wt
	sp.Env = append(os.Environ(), "O2_HELPER=idle")
	if err := sp.Run(); err != nil {
		t.Fatal(err)
	}
	p := o2ReadPid(t, pidf)
	o2Watch(t, p)
	for end := time.Now().Add(20 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if ps, err := processes(run); err == nil && ps[p].ppid == 1 {
			return p
		}
		if time.Now().After(end) {
			t.Fatalf("고아 대역 %d의 부모가 1이 안 됨", p)
		}
	}
}

func TestPidReadEndedTerminal(t *testing.T) {
	d := t.TempDir()
	W := filepath.Join(d, "repo.wt")
	wtA, wtB, wtK := filepath.Join(W, "a"), filepath.Join(W, "b"), filepath.Join(W, "k")
	for _, x := range []string{wtA, wtB, wtK, filepath.Join(d, "bin"), filepath.Join(d, "elsewhere")} {
		if err := os.MkdirAll(x, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	oldBin := binDir
	binDir = filepath.Join(d, "nobin") // 설정 도구 없음 = 기본값(끄기 off)
	t.Cleanup(func() { binDir = oldBin })
	pa, pb, pk := prOrphan(t, wtA), prOrphan(t, wtB), prOrphan(t, wtK)
	for _, x := range []string{wtA, wtB, wtK} { // 세 작업 트리 모두 지워짐(닫은 레인을 hc-lane clean이 지운 꼴)
		if err := os.RemoveAll(x); err != nil {
			t.Fatal(err)
		}
	}
	// 가짜 hc: get tEnd는 $d/end(없으면 not_found — 실제 hc 0.1.22 꼴: 오류 JSON을 stderr에, 종료 3), get tK는 pk를 pid로
	end := filepath.Join(d, "end")
	hcScript := fmt.Sprintf(`#!/bin/sh
case "$1 $2" in
  "get tEnd") [ -f %[1]q ] && exec cat %[1]q
    echo '{"error":{"hint":"see `+"`hc ls --all`"+`","kind":"not_found","message":"no terminal \"tEnd\""}}' >&2; exit 3 ;;
  "get tK") echo '{"address":"tK","state":"live","pid":%[2]d}'; exit 0 ;;
esac
echo "모르는 호출: $*" >&2; exit 1
`, end, pk)
	if err := os.WriteFile(filepath.Join(d, "bin", "hc"), []byte(hcScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join(d, "bin")+":"+os.Getenv("PATH"))
	terms := []any{ // 고아 쓸기가 받는 목록은 터미널 쓸기가 닫기 전의 hc ls — 닫은 tEnd도 live로 남아 있다
		map[string]any{"address": "tEnd", "state": "live", "cwd": wtA},
		map[string]any{"address": "tK", "state": "live", "cwd": filepath.Join(d, "elsewhere")},
	}
	sweep := func() (string, int) {
		var n int
		out := prOut(t, func() {
			var err error
			if n, err = sweepOrphans(terms, true, 0, d); err != nil {
				t.Fatalf("sweepOrphans: %v", err)
			}
		})
		return out, n
	}
	line := func(out string, p int, re string) bool {
		return regexp.MustCompile(fmt.Sprintf(`(?m)^%s pid %d `, re, p)).MatchString(out)
	}

	// 끝난 터미널(not_found·finished): pid 모음에서 빼고 진행 — 그 작업 트리의 후보(a)만 보류, 나머지는 원래 판정
	for name, body := range map[string]string{"not_found": "", "finished": `{"address":"tEnd","state":"finished","pid":1}`} {
		os.Remove(end)
		if body != "" {
			if err := os.WriteFile(end, []byte(body+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		out, n := sweep()
		if strings.Contains(out, "터미널 pid를 못 읽음") {
			t.Errorf("%s: 끝난 터미널 하나로 바퀴 전체 보류:\n%s", name, out)
		}
		if !line(out, pa, `보류 고아:`) || !strings.Contains(out, "이번 바퀴에 끝난 터미널") {
			t.Errorf("%s: 끝난 터미널(tEnd) 작업 트리의 후보 a(%d)가 보류가 아님:\n%s", name, pa, out)
		}
		if !line(out, pb, `끌 고아:`) || n != 1 {
			t.Errorf("%s: 쓰는 터미널 없는 b(%d)가 끌 후보가 아님(n=%d):\n%s", name, pb, n, out)
		}
		if !line(out, pk, `보류 고아:`) || !strings.Contains(out, "자손에 터미널 프로세스 있음") {
			t.Errorf("%s: live 터미널(tK) 프로세스인 k(%d)가 보류가 아님:\n%s", name, pk, out)
		}
	}

	// 지킬 것: hc 자신의 실패·pid 없음은 지금처럼 전부 보류(끌 것 0) — 까닭(주소: 무엇)은 바퀴에 한 줄
	for name, c := range map[string]struct{ body, why string }{
		"hc 실패":  {"", "tEnd2: 모르는 호출"},
		"pid 없음": {`{"address":"tEnd","state":"live"}`, "tEnd: pid 없음"},
	} {
		os.Remove(end)
		if c.body != "" {
			if err := os.WriteFile(end, []byte(c.body+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		} else { // not_found가 아닌 실패: 가짜 hc가 모르는 주소
			terms[0] = map[string]any{"address": "tEnd2", "state": "live", "cwd": wtA}
		}
		out, n := sweep()
		terms[0] = map[string]any{"address": "tEnd", "state": "live", "cwd": wtA}
		if n != 0 || strings.Contains(out, "끌 고아:") {
			t.Errorf("%s: pid를 못 읽었는데 끌 후보 %d:\n%s", name, n, out)
		}
		for _, p := range []int{pa, pb, pk} {
			if !line(out, p, `보류 고아:`) {
				t.Errorf("%s: %d가 보류가 아님:\n%s", name, p, out)
			}
		}
		if strings.Count(out, c.why) != 1 || strings.Count(out, "터미널 pid를 못 읽음 —") != 1 {
			t.Errorf("%s: 까닭 줄(%q)이 바퀴에 한 번이 아님:\n%s", name, c.why, out)
		}
	}
}
