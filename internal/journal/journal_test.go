package journal

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/events"
)

func tmp(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "st", "journal.jsonl")
	t.Setenv(Env, p)
	t.Setenv("HC_TERMINAL_ID", "")
	return p
}

// ① 한 줄에 칸이 다 차고, 탭·줄바꿈은 한 줄로 편다
func TestAppendFields(t *testing.T) {
	tmp(t)
	t.Setenv("HC_TERMINAL_ID", "w1:t2")
	if err := Append(Entry{Tool: "hc-fill", Action: "knob set", Target: "x", Why: "한\n줄", Result: "됨", Undo: "hc-fill knob unset x", State: "자리 2/3"}); err != nil {
		t.Fatal(err)
	}
	es, bad, err := Read(Filter{})
	if err != nil || bad != 0 || len(es) != 1 {
		t.Fatalf("%v %d %v", es, bad, err)
	}
	e := es[0]
	if e.Time.IsZero() || e.By != "w1:t2" || e.Why != "한 줄" || e.Undo != "hc-fill knob unset x" || e.State != "자리 2/3" {
		t.Fatalf("%+v", e)
	}
	t.Setenv("HC_TERMINAL_ID", "")
	if err := Append(Entry{Tool: "t", Action: "a"}); err != nil {
		t.Fatal(err)
	}
	es, _, _ = Read(Filter{})
	if es[1].By != "사람" || es[1].Undo != "-" {
		t.Fatalf("%+v", es[1])
	}
}

// ② --since·--by 거름
func TestFilter(t *testing.T) {
	tmp(t)
	now := time.Now()
	for i, by := range []string{"a", "b", "a"} {
		if err := Append(Entry{Time: now.Add(time.Duration(i-3) * time.Hour), By: by, Tool: "t", Action: "x", Target: "y"}); err != nil {
			t.Fatal(err)
		}
	}
	if es, _, _ := Read(Filter{By: "a"}); len(es) != 2 {
		t.Fatalf("by: %d", len(es))
	}
	if es, _, _ := Read(Filter{Since: now.Add(-150 * time.Minute)}); len(es) != 2 {
		t.Fatalf("since: %d", len(es))
	}
	if es, _, _ := Read(Filter{Since: now.Add(-150 * time.Minute), By: "b"}); len(es) != 1 {
		t.Fatalf("since+by: %d", len(es))
	}
}

// ③ 같은 대상·같은 조치가 기간 안에 되풀이되면 한 줄 알림(막지 않는다)
func TestRepeatNotice(t *testing.T) {
	tmp(t)
	var w bytes.Buffer
	e := Entry{Tool: "hc-fill", Action: "retry", Target: "lane-x"}
	Record(&w, e)
	if w.Len() != 0 {
		t.Fatalf("첫 조치에 알림: %q", w.String())
	}
	Record(&w, Entry{Tool: "hc-fill", Action: "retry", Target: "lane-y"})
	Record(&w, e)
	Record(&w, e)
	got := w.String()
	if strings.Count(got, "\n") != 2 || !strings.Contains(got, "최근 2시간에 같은 조치(retry lane-x) 3번째") {
		t.Fatalf("%q", got)
	}
	if es, _, _ := Read(Filter{Action: "retry", Target: "lane-x"}); len(es) != 3 {
		t.Fatalf("알림이 조치를 막음: %d", len(es))
	}
	// 기간 밖의 옛 줄은 세지 않는다
	w.Reset()
	Record(&w, Entry{Time: time.Now().Add(-5 * time.Hour), Tool: "t", Action: "old", Target: "z"})
	Record(&w, Entry{Tool: "t", Action: "old", Target: "z"})
	if w.Len() != 0 {
		t.Fatalf("기간 밖 셈: %q", w.String())
	}
}

// ④ 일지를 못 쓰면 한 줄 알림 — 패닉·멈춤 없이 돌아온다
func TestWriteFailure(t *testing.T) {
	d := t.TempDir()
	block := filepath.Join(d, "file")
	if err := os.WriteFile(block, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(Env, filepath.Join(block, "journal.jsonl")) // 부모가 파일이라 폴더를 못 만든다
	var w bytes.Buffer
	Record(&w, Entry{Tool: "hc-fill", Action: "knob set", Target: "x"})
	if !strings.Contains(w.String(), "hc-fill: 일지를 남기지 못했다(조치는 그대로)") {
		t.Fatalf("%q", w.String())
	}
}

// 크기 상한에서 옛 파일로 넘기고 지우지 않는다 — 읽기는 옛 파일까지
func TestRotateKeeps(t *testing.T) {
	p := tmp(t)
	if err := Append(Entry{Tool: "t", Action: "first"}); err != nil {
		t.Fatal(err)
	}
	old := maxSize
	maxSize = 10 // 첫 줄로 이미 넘었다
	t.Cleanup(func() { maxSize = old })
	if err := events.AppendLine(p, "{깨진 줄", 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Append(Entry{Tool: "t", Action: "second"}); err != nil {
		t.Fatal(err)
	}
	if fs, _ := filepath.Glob(p + ".*"); len(fs) != 1 {
		t.Fatalf("옛 파일: %v", fs)
	}
	es, bad, err := Read(Filter{})
	if err != nil || len(es) != 2 || es[0].Action != "first" || es[1].Action != "second" || bad != 1 {
		t.Fatalf("%v %d %v", es, bad, err)
	}
}

// 읽기 실패는 「없음」이 아니다
func TestReadErrorNotEmpty(t *testing.T) {
	p := tmp(t)
	if err := os.MkdirAll(p, 0o700); err != nil { // 파일 자리에 폴더 — 읽기 실패
		t.Fatal(err)
	}
	if _, _, err := Read(Filter{}); err == nil {
		t.Fatal("오류 없음")
	}
	t.Setenv(Env, filepath.Join(t.TempDir(), "none.jsonl"))
	if es, _, err := Read(Filter{}); err != nil || len(es) != 0 {
		t.Fatalf("파일 없음: %v %v", es, err)
	}
}
