//go:build !windows

package main

// agent-skill-guides ③: --log --json의 칸이 docs/ops/agents.md 「### herdr-sync --log --json」의 칸과 같고, 사람용 줄과 같은 사실

import (
	"encoding/json"
	"io"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// logOut: showLog(args)의 표준 출력과 종료 코드
func logOut(t *testing.T, args ...string) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	rc := showLog(args)
	os.Stdout = old
	w.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b), rc
}

func TestLogJSON(t *testing.T) {
	d := t.TempDir()
	old := deployLogFile
	deployLogFile = func() string { return d + "/deploy.tsv" }
	t.Cleanup(func() { deployLogFile = old })

	if out, rc := logOut(t, "--json"); rc != 0 || out != "[]\n" {
		t.Fatalf("기록 없음: %q %d", out, rc)
	}
	rows := "2026-11-24T10:00:00+09:00\t배포\t0\taaaaaaaaaaaa\tsrc1\tbbbbbbbbbbbb\tw-1\t맥=같음 box=씀\n" +
		"2026-11-24T11:00:00+09:00\t되돌림\t1\tcccccccccccc\tat:cccc\tbbbbbbbbbbbb\t-\t맥=씀 box=실패\n"
	if err := os.WriteFile(d+"/deploy.tsv", []byte(rows), 0o644); err != nil {
		t.Fatal(err)
	}
	out, rc := logOut(t, "1", "--json")
	var got []map[string]any
	if rc != 0 || json.Unmarshal([]byte(out), &got) != nil || len(got) != 1 {
		t.Fatalf("--log 1 --json: %q %d", out, rc)
	}
	g := got[0]
	if g["kind"] != "되돌림" || g["exit"] != float64(1) || g["tools_commit"] != "cccccccccccc" ||
		!reflect.DeepEqual(g["machines"], map[string]any{"맥": "씀", "box": "실패"}) {
		t.Fatalf("마지막 줄 = %v", g)
	}
	if human, _ := logOut(t, "1"); !strings.Contains(human, "되돌림 종료 1 · 범용 cccccccccccc") {
		t.Fatalf("사람용 줄이 같은 사실이 아니다: %q", human)
	}

	// 칸 이름 = 문서
	b, err := os.ReadFile("../../docs/ops/agents.md")
	if err != nil {
		t.Fatal(err)
	}
	_, sec, _ := strings.Cut(string(b), "\n### herdr-sync --log --json\n")
	sec, _, _ = strings.Cut(sec, "\n### ")
	var want []string
	for _, l := range strings.Split(sec, "\n") {
		if strings.HasPrefix(l, "- 칸") {
			for _, m := range regexp.MustCompile("`([a-z_0-9]+)`").FindAllStringSubmatch(l, -1) {
				want = append(want, m[1])
			}
		}
	}
	var keys []string
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(out)[1:])) // 배열의 첫 객체
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("코드 칸 %q ≠ 문서 칸 %q", keys, want)
	}
}
