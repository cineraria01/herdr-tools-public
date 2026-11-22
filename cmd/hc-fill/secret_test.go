package main

import (
	"os"
	"strings"
	"testing"
)

// 시험 파일 자체가 공개 내보내기의 비밀 꼴에 걸리지 않게 값은 조각을 이어 만든다
var rnd = "Zq8" + "Lm2Xv9" + "Rt4Wp7" + "Ks1Hd6"

// 통과해야 할 평범한 줄(go-secret-fp — 11/21 감사 보고서의 코드 글 「token = os.environ…」에 gofix-fill이 안 떴다)
func TestSecretPlain(t *testing.T) {
	for _, l := range []string{
		"mode=block",
		"HC_FILL_IMPL=bash",
		`token = os.environ.get("GITHUB_TOKEN")`,
		`token = os.environ["HC_TOKEN"]`,
		"- `token = os.environ…`(값이 아닌 코드)",
		"AUTH_FILE=~/.config/proxy.json",
		"session=7fdd8fef-bb21-497d-ab6f-3ad5ea357e36",
		"KEY=hf:ln:paste:1234567890123",
		"API_TOKEN=${HC_TOKEN}",
		"password: <자리>",
		`secret_key = "change-me-in-production"`,
		"auth=hc-fill-2026-11-22",
		"sessionDir=/Users/x/.local/state/" + "herdr-sync/bak/20261122T0054",
		"keyboard = 12345678901234567890",
		"문서: API_KEY·token= 자리는 설정에 둔다",
		"session_key: aaaaaaaa1111111111bbbb", // 엔트로피 낮음
		"task-" + "sk-like-words-but-not-a-key-here",
	} {
		if k, n, nm := lcSecretHit([]byte("앞줄\n" + l + "\n")); k != "" {
			t.Errorf("평범한 줄이 걸림 %q → %s %d %q", l, k, n, nm)
		}
	}
}

// 막아야 할 비밀 줄: 꼴 이름과 줄 번호(이름=값은 이름도)
func TestSecretBlock(t *testing.T) {
	for _, c := range []struct{ line, kind, name string }{
		{"API_TOKEN=abcdef" + "0123456789xyz", "이름=값", "API_TOKEN"},
		{`"client_secret": "` + rnd + `"`, "이름=값", "client_secret"},
		{"Authorization: Bearer " + rnd, "이름=값", "Authorization"},
		{"export PASSWORD='" + rnd + "'", "이름=값", "PASSWORD"},
		{"Cookie: sid=" + rnd, "이름=값", "Cookie"},
		{"x.sessionId = " + rnd + ";", "이름=값", "x.sessionId"},
		{"gh" + "p_" + rnd + rnd, "ghp_", ""},
		{"github" + "_pat_" + rnd, "github_pat_", ""},
		{"sk" + "-ant-api03-" + rnd, "sk-", ""},
		{"sk" + "-proj-" + rnd, "sk-", ""},
		{"AK" + "IA" + "ABCDEFGH" + "23456789", "AKIA", ""},
		{"xo" + "xb-1234-" + rnd, "xox", ""},
		{"AI" + "za" + rnd + rnd[:14], "AIza", ""},
		{"ey" + "J" + rnd + ".ey" + "J" + rnd + "." + rnd, "eyJ(JWT)", ""},
		{"-----BEGIN OPENSSH " + "PRIVATE KEY-----", "개인 키", ""},
	} {
		k, n, nm := lcSecretHit([]byte("첫 줄\n둘째\n" + c.line + "\n"))
		if k != c.kind || n != 3 || nm != c.name {
			t.Errorf("%q: got %q %d %q, want %q 3 %q", c.line, k, n, nm, c.kind, c.name)
		}
	}
}

// 막을 때 알림은 줄 번호·이름만 — 값은 찍지 않는다
func TestSecretWhyNoValue(t *testing.T) {
	old := HOME
	defer func() { HOME = old }()
	HOME = t.TempDir()
	d := HOME + "/.claude-tasks" // 붙일 폴더 기본값(paste.go lcPasteDirs)
	if err := os.Mkdir(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d+"/r.md", []byte("# 감사\ntoken = os.environ[\"T\"]\n\nDB_PASSWORD="+rnd+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, shown, _, why := lcPasteScan("~/.claude-tasks/r.md")
	if p != "" || shown != "~/.claude-tasks/r.md" || why != "비밀 꼴(이름=값)을 담았다 — 4번째 줄 이름 DB_PASSWORD" || strings.Contains(why, rnd) {
		t.Errorf("why %q", why)
	}
	// 코드 글만 든 보고서는 붙인다
	if err := os.WriteFile(d+"/ok.md", []byte("token = os.environ.get(\"HC_TOKEN\")\nmode=block\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := lcPasteHome("~/.claude-tasks/ok.md", "-", "ln"); !ok || !strings.Contains(got, "os.environ") {
		t.Errorf("코드 글 보고서: %q %v", got, ok)
	}
}
