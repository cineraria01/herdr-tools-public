//go:build !windows

package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/knob"
)

// merger-knob-per-project: set·unset·ls의 「이름@프로젝트」와, 원격 참조에 그 저장소 몫으로 풀린 값만 올라가는지
func TestKnobPerProject(t *testing.T) {
	reround := knobFix(t)
	d := t.TempDir()
	for k, v := range map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"} {
		t.Setenv(k, v)
	}
	g := func(args ...string) string {
		o, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, o)
		}
		return strings.TrimSpace(string(o))
	}
	for _, p := range []string{"pa", "pb"} {
		g("init", "-q", "--bare", d+"/"+p+".git")
		g("clone", "-q", d+"/"+p+".git", d+"/"+p)
		g("clone", "-q", d+"/"+p+".git", d+"/"+p+"-far")
	}
	jf := d + "/journal.jsonl"
	t.Setenv("HERDR_TOOLS_JOURNAL", jf)
	lastUndo := func() string {
		b, err := os.ReadFile(jf)
		if err != nil {
			t.Fatal(err)
		}
		ls := strings.Split(strings.TrimSpace(string(b)), "\n")
		var j struct{ Undo string }
		if err := json.Unmarshal([]byte(ls[len(ls)-1]), &j); err != nil {
			t.Fatal(err)
		}
		return j.Undo
	}
	confLines = []string{"local here", "repo pa here " + d + "/pa", "repo pb here " + d + "/pb", "repo pc box-2 /srv/pc"}
	now := time.Now()
	set := func(args ...string) error { return knobSet(append(args, "--why", "검증이 긴 저장소"), now) }
	front := func(p string) string {
		r, st := knob.ReadRemote(d+"/"+p+"-far", io.Discard, time.Now())
		if st == gitx.Unknown {
			t.Fatalf("%s 원격을 못 읽음", p)
		}
		return r.Get("merger_front_wait")
	}

	// ④ 설정에 없는 프로젝트·병합기 손잡이가 아닌 것은 거절
	if err := set("merger_front_wait@pz", "1500"); err == nil || !strings.Contains(err.Error(), "설정에 없는 프로젝트") {
		t.Fatalf("없는 프로젝트 = %v", err)
	}
	if err := set("idle@pb", "5"); err == nil || !strings.Contains(err.Error(), "모르는 손잡이") {
		t.Fatalf("hc-fill 손잡이@프로젝트 = %v", err)
	}
	if err := set("merger_front_wait@pc", "1500"); err == nil || !strings.Contains(err.Error(), "repo 줄이 없는 프로젝트") {
		t.Fatalf("이 기계에 클론 없는 프로젝트 = %v", err)
	}

	// 되돌리기 줄(감사 atp0a1i): 공통 손잡이 + 처음 둔 프로젝트별 값 → unset 이름@프로젝트, 그 unset → 그 값을 다시 두는 줄
	if err := set("merger_batch", "4"); err != nil {
		t.Fatal(err)
	}
	reround()
	if err := set("merger_batch@pb", "6"); err != nil {
		t.Fatal(err)
	}
	if u := lastUndo(); u != "hc-fill knob unset merger_batch@pb" {
		t.Fatalf("프로젝트별 첫 set의 되돌리기 = %q", u)
	}
	reround()
	if err := knobUnset([]string{"merger_batch@pb"}, now); err != nil {
		t.Fatal(err)
	}
	if u := lastUndo(); u != "hc-fill knob set merger_batch@pb 6 --why 되돌림" {
		t.Fatalf("프로젝트별 unset의 되돌리기 = %q", u)
	}
	reround()
	if err := knobUnset([]string{"merger_batch"}, now); err != nil {
		t.Fatal(err)
	}
	reround()

	// ② 프로젝트별 값만 → pb는 그 값, pa는 기본
	if err := set("merger_front_wait@pb", "1500", "--for", "60"); err != nil {
		t.Fatal(err)
	}
	reround()
	knobsRemoteTick()
	if front("pb") != "1500" || front("pa") != "600" {
		t.Fatalf("공통 없음: pb=%s pa=%s", front("pb"), front("pa"))
	}
	if exec.Command("git", "--git-dir="+d+"/pa.git", "rev-parse", "-q", "--verify", knob.Ref).Run() == nil {
		t.Fatal("pa에는 실을 것이 없다 — 참조가 생기면 안 된다")
	}

	// ① 공통 700 + pb 1500 → pb 1500, pa 700
	if err := set("merger_front_wait", "700"); err != nil {
		t.Fatal(err)
	}
	reround()
	knobsRemoteTick()
	if front("pb") != "1500" || front("pa") != "700" {
		t.Fatalf("공통+프로젝트별: pb=%s pa=%s", front("pb"), front("pa"))
	}

	// ⑥ ls: 공통 줄과 프로젝트별 줄이 다 보이고 출처가 드러난다
	rows := map[string]knobRow{}
	for _, r := range knobRows(time.Now()) {
		rows[r.Name] = r
	}
	if r := rows["merger_front_wait"]; r.Value != "700" || r.Source != "손잡이" {
		t.Fatalf("공통 줄 = %+v", r)
	}
	if r := rows["merger_front_wait@pb"]; r.Value != "1500" || r.Source != "손잡이" || r.Why != "검증이 긴 저장소" || r.Until == "" || r.Tool != "hc-merger" {
		t.Fatalf("프로젝트별 줄 = %+v", r)
	}
	if _, ok := rows["merger_front_wait@pa"]; ok {
		t.Fatal("두지 않은 프로젝트 줄이 보인다")
	}

	// ③ unset → 공통으로
	if err := knobUnset([]string{"merger_front_wait@pb"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	reround()
	knobsRemoteTick()
	if front("pb") != "700" || front("pa") != "700" {
		t.Fatalf("unset 뒤: pb=%s pa=%s", front("pb"), front("pa"))
	}
	// 두 저장소 내용이 같으면 같은 SHA(날짜 0 커밋)
	if g("--git-dir="+d+"/pa.git", "rev-parse", knob.Ref) != g("--git-dir="+d+"/pb.git", "rev-parse", knob.Ref) {
		t.Fatal("같은 내용인데 SHA가 다르다")
	}
}
