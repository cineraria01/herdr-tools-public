package main

// 이 기계 이름(sync-local-name, 2026-11-24) ①: 설정(hc-fill.tsv의 local 줄)이 없으면 출력은 바꾸기 전과 바이트까지 같다. 같은 장면
// (처음 맞추기·충돌·--take·--check·배포·--log·--log --json·--status·--at·--landed)의 출력과 배포 기록을 견본(testdata/localname.golden —
// 바꾸기 전 코드로 만들어 먼저 커밋)과 견준다. 실행마다 바뀌는 것(커밋 해시·시각·임시 경로·pid·초)만 자리표로 바꾼다.
// 견본 다시 만들기: HERDR_SYNC_GOLDEN_UPDATE=1 go test -run TestLocalNameGolden ./cmd/herdr-sync/

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	ct "herdr-tools/internal/clitest"
)

var (
	goldHash  = regexp.MustCompile(`\b[0-9a-f]{12}([0-9a-f]{28})?\b`)
	goldTime  = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d([+-]\d\d:\d\d|Z)`)
	goldStamp = regexp.MustCompile(`\b\d{8}-\d{6}\b`)
	goldNum   = regexp.MustCompile(`(pid |herdr-sync\.)[0-9A-Za-z]+|\d+초째`)
)

// localScenario: 장면을 돌려 정규화한 기록(명령마다 「$ 인자 → 종료」·출력, 끝에 배포 기록)
func localScenario(s *sx) string {
	var b strings.Builder
	step := func(args ...string) {
		s.run(args...)
		b.WriteString("$ " + strings.Join(args, " ") + " → " + strconv.Itoa(s.r.RC) + "\n" + s.r.Out + "\n")
	}
	step()
	step("--take", "w1:.claude/docs/b.md")
	step("--take", "lx:.claude/CLAUDE.md#body")
	step("--take", "lx:.codex/AGENTS.md#codex-shared")
	step("--check")
	s.relPrep()
	step()
	step()
	c1 := ct.Git(s.T+"/repo", "rev-parse", "HEAD")[:12]
	s.landCommit("lane-two: 둘째")
	step("--status")
	step("--landed")
	step()
	step("--at", c1, "--check", "w1")
	step("--at", c1)
	step("--status")
	step("--log")
	step("--log", "2", "--json")
	ct.Unsetenv(s.t, "HERDR_SYNC_RELEASE_URL")
	b.WriteString("# deploy.tsv\n" + ct.Read(s.MAC+"/.local/state/herdr-sync/deploy.tsv"))
	o := strings.ReplaceAll(b.String(), s.T, "$T")
	o = goldTime.ReplaceAllString(o, "<시각>")
	o = goldStamp.ReplaceAllString(o, "<STAMP>")
	o = goldHash.ReplaceAllString(o, "<H>")
	return goldNum.ReplaceAllString(o, "<N>")
}

func TestLocalNameGolden(t *testing.T) {
	update := os.Getenv("HERDR_SYNC_GOLDEN_UPDATE") == "1" // newSX가 환경을 가르기 전에
	p, _ := filepath.Abs("testdata/localname.golden")
	s := newSX(t)
	got := localScenario(s)
	if update {
		ct.Put(t, p, got)
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		gl, wl := ct.Lines(got), ct.Lines(string(want))
		for i := 0; i < len(gl) || i < len(wl); i++ {
			var g, w string
			if i < len(gl) {
				g = gl[i]
			}
			if i < len(wl) {
				w = wl[i]
			}
			if g != w {
				t.Fatalf("설정 없음인데 출력이 견본과 다르다 — %d번 줄\n  견본: %q\n  지금: %q", i+1, w, g)
			}
		}
		t.Fatal("설정 없음인데 출력이 견본과 다르다(줄바꿈)")
	}
}
