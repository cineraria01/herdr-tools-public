//go:build !windows

package main

// agent-skill-guides(2026-11-24): ③ --json 칸이 docs/ops/agents.md의 칸과 같고 사람용 표와 같은 사실 ④ --dry-run은 아무것도 안 바꾼다
// (HOME·대기열·일지·손잡이·조정 기록 파일의 전후가 같음) ⑤ 잘못 부름·없는 레인에 「다음:」 한 줄 — 첫 줄·종료 코드는 그대로

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// docFields: docs/ops/agents.md에서 「### <제목>」 절 안 「칸…:」 줄의 `이름`들
func docFields(t *testing.T, title string) []string {
	t.Helper()
	b, err := os.ReadFile("../../docs/ops/agents.md")
	if err != nil {
		t.Fatal(err)
	}
	_, sec, ok := strings.Cut(string(b), "\n### "+title+"\n")
	if !ok {
		t.Fatalf("문서에 절 「### %s」이 없다", title)
	}
	sec, _, _ = strings.Cut(sec, "\n### ")
	for _, l := range strings.Split(sec, "\n") {
		if strings.HasPrefix(l, "- 칸") {
			var fs []string
			for _, m := range regexp.MustCompile("`([a-z_0-9]+)`").FindAllStringSubmatch(l, -1) {
				fs = append(fs, m[1])
			}
			return fs
		}
	}
	t.Fatalf("절 「### %s」에 「- 칸」 줄이 없다", title)
	return nil
}

// jsonKeys: 값 하나를 JSON으로 만들었을 때의 객체 키(차례 그대로 — 구조체 칸 차례)
func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	if _, err := d.Token(); err != nil {
		t.Fatal(err)
	}
	var ks []string
	for d.More() {
		k, err := d.Token()
		if err != nil {
			t.Fatal(err)
		}
		ks = append(ks, k.(string))
		var skip json.RawMessage
		if err := d.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return ks
}

// ③ 칸 이름 = 문서
func TestAgentJSONFieldsMatchDoc(t *testing.T) {
	for _, c := range []struct {
		title string
		v     any
	}{{"hc-fill ls --json", lsRow{}}, {"hc-fill machine --json", machRow{}}} {
		if got, want := jsonKeys(t, c.v), docFields(t, c.title); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: 코드 칸 %q ≠ 문서 칸 %q", c.title, got, want)
		}
	}
}

// fillKit: 지은 hc-fill과 가짜 hc(ls는 빈 목록, 그 밖은 실패)·설정·대기열 한 줄(wait)을 둔 HOME
type fillKit struct {
	bin, home, data string
	env             []string
}

func newFillKit(t *testing.T) fillKit {
	t.Helper()
	d := t.TempDir()
	k := fillKit{bin: d + "/bin/hc-fill", home: d + "/home", data: d + "/home/data"}
	if out, err := exec.Command("go", "build", "-o", k.bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("빌드: %v %s", err, out)
	}
	put := func(p, s string, mode os.FileMode) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), mode); err != nil {
			t.Fatal(err)
		}
	}
	put(d+"/bin/hc", "#!/bin/sh\n[ \"$1 $2\" = \"ls --json\" ] && { echo '{\"terminals\":[]}'; exit 0; }\nexit 1\n", 0o755)
	put(k.home+"/conf/hc-fill.tsv", "local here\nmachine here 2 - -\nmachine box 1 - -\nrepo app here /nowhere/app\n", 0o644)
	put(k.data+"/queue.tsv", "lane-a\tapp\t/nowhere/b.md\tlane-z\t-\twait\t-\t-\t-\t1\t-\t-\thigh\t-\tno-audit\t-\n", 0o644)
	k.env = append(os.Environ(), "HOME="+k.home, "HERDR_TOOLS_CONFIG="+k.home+"/conf", "HC_FILL_DATA="+k.data,
		"HERDR_TOOLS_OPLOG="+d+"/ops.tsv", "PATH="+d+"/bin:/usr/bin:/bin", "HC_TERMINAL_ID=")
	return k
}

func (k fillKit) run(t *testing.T, args ...string) (out, se string, rc int) {
	t.Helper()
	c := exec.Command(k.bin, args...)
	c.Env = k.env
	var o, e strings.Builder
	c.Stdout, c.Stderr = &o, &e
	err := c.Run()
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		rc = ee.ExitCode()
	case err != nil:
		t.Fatal(err)
	}
	return o.String(), e.String(), rc
}

// snap: HOME 아래 모든 파일의 경로 → 내용
func fileSnap(t *testing.T, root string) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		m[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// ③ ls --json: 사람용 표와 같은 사실(레인·프로젝트·상태·우선·선행), mergers는 빈 목록
func TestLsJSONSameFacts(t *testing.T) {
	k := newFillKit(t)
	out, se, rc := k.run(t, "ls", "--json")
	var got lsOut
	if rc != 0 || json.Unmarshal([]byte(out), &got) != nil {
		t.Fatalf("ls --json: 종료 0·JSON이어야 — %d %q %q", rc, out, se)
	}
	if len(got.Lanes) != 1 || got.Mergers == nil {
		t.Fatalf("한 줄·mergers 목록이어야: %+v", got)
	}
	r := got.Lanes[0]
	if r.Lane != "lane-a" || r.Project != "app" || r.State != "대기" || r.Code != "wait" || r.Priority != "높음" ||
		!reflect.DeepEqual(r.After, []string{"lane-z"}) || len(r.Only) != 0 || !strings.Contains(r.Note, "자동 감사 끔") {
		t.Fatalf("ls --json 줄 = %+v", r)
	}
	table, _, rc := k.run(t, "ls")
	if rc != 0 || !strings.Contains(table, "lane-a") || !strings.Contains(table, "높음") || !strings.Contains(table, r.Note) {
		t.Fatalf("사람용 표가 같은 사실이 아니다: %q (비고 %q)", table, r.Note)
	}
}

// ③ machine --json: 설정의 기계마다 한 줄, 부하를 못 잰 값은 null
func TestMachineJSON(t *testing.T) {
	k := newFillKit(t)
	out, se, rc := k.run(t, "machine", "--json")
	var rows []map[string]any
	if rc != 0 || json.Unmarshal([]byte(out), &rows) != nil {
		t.Fatalf("machine --json: %d %q %q", rc, out, se)
	}
	var ms []string
	for _, r := range rows {
		ms = append(ms, r["machine"].(string))
		if r["load1"] != nil || r["used"] != nil {
			t.Errorf("잰 적 없는 기계는 null이어야: %v", r)
		}
	}
	sort.Strings(ms)
	if !reflect.DeepEqual(ms, []string{"box", "here"}) {
		t.Fatalf("기계 = %q", ms)
	}
}

// ④ --dry-run은 아무것도 안 바꾼다(전후 파일 같음), 「미리 보기:」 줄. 실제로 부르면 바뀐다(시험이 헛돌지 않게)
func TestDryRunChangesNothing(t *testing.T) {
	k := newFillKit(t)
	before := fileSnap(t, k.home)
	for _, a := range [][]string{
		{"rm", "lane-a", "--dry-run", "--why", "시험"},
		{"prio", "lane-a", "낮음", "--dry-run"},
		{"knob", "set", "idle", "40", "--why", "시험", "--dry-run"},
		{"knob", "unset", "idle", "--dry-run"},
		{"machine", "box", "--pause", "--why", "시험", "--dry-run"},
		{"machine", "box", "--seats", "2", "--why", "시험", "--for", "30", "--dry-run"},
	} {
		out, se, rc := k.run(t, a...)
		if rc != 0 || !strings.HasPrefix(out, "미리 보기: ") {
			t.Errorf("%q: 종료 0·「미리 보기:」여야 — %d %q %q", a, rc, out, se)
		}
		if after := fileSnap(t, k.home); !reflect.DeepEqual(before, after) {
			t.Errorf("%q: 파일이 바뀌었다", a)
		}
	}
	if out, _, _ := k.run(t, "prio", "lane-a", "낮음", "--dry-run"); !strings.Contains(out, "높음 → 낮음") {
		t.Errorf("prio 미리 보기에 전 → 후가 없다: %q", out)
	}
	if out, _, _ := k.run(t, "rm", "lane-a", "--dry-run"); !strings.Contains(out, "되돌리기 hc-fill add --name lane-a") {
		t.Errorf("rm 미리 보기에 되돌리는 add가 없다: %q", out)
	}
	// 실제 부르기는 바꾼다
	if _, se, rc := k.run(t, "prio", "lane-a", "낮음"); rc != 0 {
		t.Fatalf("prio: %d %q", rc, se)
	}
	if reflect.DeepEqual(before, fileSnap(t, k.home)) {
		t.Fatal("실제 prio가 아무것도 안 바꿨다 — 시험이 헛돈다")
	}
}

// ④ 고정된 손잡이는 미리 보기에서도 실제처럼 거절(종료 1)
func TestDryRunPinnedRefused(t *testing.T) {
	k := newFillKit(t)
	if _, se, rc := k.run(t, "knob", "set", "idle", "30", "--why", "고정", "--pin"); rc != 0 {
		t.Fatalf("knob set --pin: %d %q", rc, se)
	}
	before := fileSnap(t, k.home)
	_, se, rc := k.run(t, "knob", "set", "idle", "40", "--why", "시험", "--dry-run")
	if rc != 1 || !strings.Contains(se, "고정된 값") {
		t.Fatalf("고정 값 미리 보기는 거절이어야: %d %q", rc, se)
	}
	if !reflect.DeepEqual(before, fileSnap(t, k.home)) {
		t.Fatal("거절한 미리 보기가 파일을 바꿨다")
	}
}

// ⑤ 잘못 부름(사용법, 종료 2)·없는 레인(종료 1)에 「다음:」 한 줄 — 첫 줄·종료 코드는 그대로
func TestNextHintOnCommonFailures(t *testing.T) {
	k := newFillKit(t)
	_, se, rc := k.run(t, "bogus")
	ls := strings.Split(strings.TrimRight(se, "\n"), "\n")
	if rc != 2 || !strings.HasPrefix(se, "사용법: hc-fill ") || !strings.HasPrefix(ls[len(ls)-1], "hc-fill: 다음: hc-fill --skill") {
		t.Fatalf("잘못 부름: 종료 2·사용법 뒤 「다음:」이어야 — %d %q", rc, se)
	}
	_, se, rc = k.run(t, "prio", "nolane", "높음")
	ls = strings.Split(strings.TrimRight(se, "\n"), "\n")
	if rc != 1 || len(ls) != 2 || ls[0] != "hc-fill: 대기열에 없다: nolane" || !strings.HasPrefix(ls[1], "hc-fill: 다음: hc-fill ls") {
		t.Fatalf("없는 레인: 종료 1·첫 줄 그대로·「다음:」이어야 — %d %q", rc, se)
	}
	_, se, rc = k.run(t, "prio", "nolane", "높음", "--dry-run")
	if rc != 1 || !strings.HasPrefix(se, "hc-fill: 대기열에 없다: nolane\nhc-fill: 다음:") {
		t.Fatalf("없는 레인 미리 보기도 같아야: %d %q", rc, se)
	}
}
