package toolcfg

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// syncRepo: cmd/hc-lane·cmd/hc-merger(OWNER hc-lane)·go.mod만 있는 범용 저장소 꼴
func syncRepo(t *testing.T) string {
	r := t.TempDir()
	for _, d := range []string{"cmd/hc-lane", "cmd/hc-merger", "internal/x", "claude/hooks"} {
		if err := os.MkdirAll(r+"/"+d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for p, s := range map[string]string{"go.mod": "module x\n", "cmd/hc-merger/OWNER": "hc-lane\n", "cmd/hc-lane/main.go": "package main\n"} {
		if err := os.WriteFile(r+"/"+p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func lintList(t *testing.T, repo, list string) (int, string, string) {
	t.Helper()
	p := t.TempDir() + "/sync.list"
	if err := os.WriteFile(p, []byte(list), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := SyncLint(repo, p, &out, &errb)
	return rc, out.String(), errb.String()
}

const srcLines = "Go .local/lib/herdr-go/go.mod tools:go.mod all\nGo .local/lib/herdr-go/cmd/** tools:cmd/ all\nGo .local/lib/herdr-go/internal/** tools:internal/ all\n"

func TestSyncLintBinaryLines(t *testing.T) {
	r := syncRepo(t)
	ok := srcLines + "도구 .local/bin/hc-lane tools:cmd/hc-lane macos\n도구 .local/bin/hc-merger tools:cmd/hc-merger macos\n"
	if rc, out, e := lintList(t, r, ok); rc != 0 || out != "" {
		t.Fatalf("맞는 목록이 걸림: %d %q %q", rc, out, e)
	}
	// 곁 도구(hc-merger — OWNER hc-lane) 줄이 빠짐 → 더할 줄
	rc, out, _ := lintList(t, r, srcLines+"도구 .local/bin/hc-lane tools:cmd/hc-lane macos\n")
	if rc != 1 || !strings.Contains(out, "도구  .local/bin/hc-merger  tools:cmd/hc-merger  macos") {
		t.Fatalf("곁 도구: %d %q", rc, out)
	}
	// 소스 나무 줄이 대상을 못 덮음(local 소스로 macos 바이너리)
	rc, out, _ = lintList(t, r, strings.ReplaceAll(srcLines, " all", " local")+"도구 .local/bin/hc-lane tools:cmd/hc-lane macos\n도구 .local/bin/hc-merger tools:cmd/hc-merger macos\n")
	if rc != 1 || !strings.Contains(out, ".local/lib/herdr-go/cmd/**  tools:cmd/  macos") {
		t.Fatalf("소스 줄: %d %q", rc, out)
	}
	// 맥 경로가 bin/<도구>가 아님·cmd가 없음
	if rc, _, e := lintList(t, r, srcLines+"도구 .local/bin/other tools:cmd/hc-lane macos\n"); rc != 1 || !strings.Contains(e, "바이너리 줄이 틀렸다") {
		t.Fatalf("맥 경로: %d %q", rc, e)
	}
	if rc, _, e := lintList(t, r, srcLines+"도구 .local/bin/nope tools:cmd/nope macos\n"); rc != 1 || !strings.Contains(e, "바이너리 줄이 틀렸다") {
		t.Fatalf("없는 cmd: %d %q", rc, e)
	}
}

// 걷힌 셸 판 줄이 남으면 멈춘다 — 그대로 두면 herdr-sync가 맥에 남은 옛 포장을 저장소로 다시 모은다
func TestSyncLintStaleShellLines(t *testing.T) {
	r := syncRepo(t)
	list := srcLines + "도구 .local/bin/hc-lane tools:bin/hc-lane macos\n도구 .local/lib/hc-lane/** tools:lib/hc-lane/ macos\n" +
		"Go .local/lib/go-run.sh tools:lib/go-run.sh all\n훅 .claude/hooks/cotenant-guard.sh tools:claude/hooks/cotenant-guard.sh macos\n" +
		"도구 .local/bin/hc-merger tools:cmd/hc-merger macos\n"
	rc, out, e := lintList(t, r, list)
	if rc != 1 {
		t.Fatalf("걸러야 함: %d", rc)
	}
	for _, w := range []string{"tools:bin/hc-lane", "tools:lib/hc-lane/", "tools:lib/go-run.sh", "tools:claude/hooks/cotenant-guard.sh"} {
		if !strings.Contains(e, w) {
			t.Errorf("지울 줄에 %s 없음: %q", w, e)
		}
	}
	if !strings.Contains(out, "도구  .local/bin/hc-lane  tools:cmd/hc-lane  macos") {
		t.Errorf("바꿀 줄: %q", out)
	}
	// 저장소에 있는 셸 파일 줄(저장소 간 계약 스크립트 등)은 그대로
	os.MkdirAll(r+"/.claude", 0o755)
	os.WriteFile(r+"/.claude/herdr-verify.sh", []byte("#!/bin/sh\n"), 0o644)
	if rc, out, e := lintList(t, r, "x .claude/v.sh tools:.claude/herdr-verify.sh local\n"); rc != 0 {
		t.Errorf("있는 셸 파일 줄이 걸림: %d %q %q", rc, out, e)
	}
}
