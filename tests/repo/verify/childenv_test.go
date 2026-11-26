//go:build !windows

package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// ht-verify-env-scrub(2026-11-26): 병합기가 병합 검증 자리 기계의 검증에 준 HERDR_VERIFY_GATE=1이 검증이 띄운 go test(이 꾸러미 시험)에
// 새, 「값 없음」을 보는 시험(TestGateAloneAfterShards·TestPlan)이 그 기계에서만 실패해 맞는 제출이 반송됐다. 검증이 자식을 띄우는 한 곳
// (start)이 이 실행의 조종 변수를 빼고, 자식이 일부러 받는 것(childKeep)과 검증이 정해 주는 것(env 인자)만 넘긴다
func TestChildEnvScrub(t *testing.T) {
	ctl := []string{"HERDR_VERIFY_GATE", "HERDR_VERIFY_SHARD", "HERDR_VERIFY_PARTS", "HERDR_VERIFY_PARTS_FROM", "HERDR_VERIFY_ALL",
		"HERDR_VERIFY_JOBS", "HERDR_VERIFY_NO_LOCK", "HERDR_VERIFY_RETRY", "HERDR_VERIFY_SCOPE", "HERDR_VERIFY_RAMTMP",
		"HERDR_VERIFY_BOOT_TMPDIR", "HERDR_SPEC_VERIFY", "HERDR_SPEC_TREE", "HERDR_SPEC_BASE", "HERDR_VERIFIED_TREE"}
	for _, k := range ctl {
		t.Setenv(k, "outer")
	}
	t.Setenv("HERDR_VERIFY_NO_LINT", "1") // tests/repo의 개인 값 검사가 읽는다(공개 사본에서 건너뛰기) — 물려준다
	f, read := outFile(t)
	c, err := start(t.TempDir(), []string{"env"}, []string{"HERDR_VERIFY_BOOT_TMPDIR=/boot"}, f)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Wait(); err != nil {
		t.Fatal(err)
	}
	got := "\n" + read()
	for _, k := range ctl {
		if strings.Contains(got, "\n"+k+"=outer\n") {
			t.Errorf("자식이 조종 변수 %s를 물려받았다", k)
		}
	}
	for _, kv := range []string{"HERDR_VERIFY_NO_LINT=1", "HERDR_VERIFY_BOOT_TMPDIR=/boot"} {
		if !strings.Contains(got, "\n"+kv+"\n") {
			t.Errorf("자식에게 %s가 가야\n%s", kv, got)
		}
	}
}

// 지킴: 검증 프로그램·검증 스크립트·계약 문서에 나오는 조종 변수 이름은 모두 자식에게서 빠진다(ctlVar) — childKeep만 예외.
// 조종 변수를 새로 더하고 ctlVar를 그 이름에 맞게 안 고치면 여기서 실패한다
func TestChildEnvGuard(t *testing.T) {
	re := regexp.MustCompile(`HERDR_(?:VERIFY|SPEC)_[A-Z0-9_]*[A-Z0-9]|HERDR_VERIFIED_TREE`)
	files := []string{"../../../.claude/herdr-verify.sh", "../../../docs/hc-lane/merger-spec-verify.md"}
	srcs, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range srcs {
		if strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			files = append(files, e.Name())
		}
	}
	seen := map[string]bool{}
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range re.FindAllString(string(b), -1) {
			seen[k] = true
		}
	}
	for _, k := range []string{"HERDR_VERIFY_GATE", "HERDR_VERIFY_SHARD", "HERDR_VERIFY_PARTS", "HERDR_SPEC_VERIFY", "HERDR_VERIFIED_TREE"} {
		if !seen[k] {
			t.Errorf("이름 찾기가 %s를 못 봤다 — 정규식·파일 목록을 고쳐라", k)
		}
	}
	for k := range seen {
		if !childKeep[k] && !ctlVar(k) {
			t.Errorf("조종 변수 %s가 자식(시험)에게 넘어간다 — ctlVar에 더하거나, 자식이 일부러 받아야 하면 근거와 함께 childKeep에", k)
		}
		if childKeep[k] && ctlVar(k) {
			t.Errorf("%s는 childKeep인데 ctlVar가 뺀다", k)
		}
	}
}
