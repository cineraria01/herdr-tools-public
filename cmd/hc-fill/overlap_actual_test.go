package main

// go-overlap-actual 시험: 실제 변경 겹침(키 A) — 실제 겹침·안 겹침·주차 레인 재개 순서·조회 실패

import (
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestOvcmpActual(t *testing.T) {
	own := []Row{{"a1", "A", "cmd/x.go"}, {"a2", "A", "lib/m.go"}, {"a3", "R", "docs/"}, {"a4", "M", "bin/t"},
		{"a5", "R", "z.go"}, {"a5", "A", "y.go"}}
	cand := []Row{{"R", "cmd/x.go"}, {"M", "lib/m.go"}, {"A", "docs/n.md"}, {"A", "bin/t"}, {"R", "z.go"}, {"A", "y.go"}}
	// A×R·R×A(폴더 예약 아래)·A×A는 실제, A×M·M×A는 언급, 한 앞 레인에 예약과 실제가 같이 있으면 실제
	want := "a1\t실제\tcmd/x.go|a2\t언급\tlib/m.go|a3\t실제\tdocs/n.md|a4\t언급\tbin/t|a5\t실제\ty.go|"
	if got := rowsBar(ovcmp(own, cand)); got != want {
		t.Errorf("ovcmp\n got %q\nwant %q", got, want)
	}
}

func TestActParse(t *testing.T) {
	out := "app\tl1\t+\napp\tl1\t=a b.go\napp\tl1\t=docs/안내.md\napp\tl2\t+\napp\tl3\t=orphan.go\nnoise\n"
	got := actParse(out)
	want := map[string][]string{"app\tl1": {"a b.go", "docs/안내.md"}, "app\tl2": {}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("actParse\n got %#v\nwant %#v", got, want)
	}
}

// actSave: 시험이 바꾸는 전역을 되돌린다
func actSave(t *testing.T) {
	t.Helper()
	oc, ol, oo, ow, om, oa, ot, ov, of, ob, od, ok := confLines, LOCAL, OREC, OWN, OVM, ACT, TERMS, OVWAIT, FETCHED, FETCHBAD, dry, OVKEYS
	oe, oth, ovw, oap := QEDGES, THENMAP, OVWARN, ACTPARK
	t.Cleanup(func() {
		confLines, LOCAL, OREC, OWN, OVM, ACT, TERMS, OVWAIT, FETCHED, FETCHBAD, dry, OVKEYS = oc, ol, oo, ow, om, oa, ot, ov, of, ob, od, ok
		QEDGES, THENMAP, OVWARN, ACTPARK = oe, oth, ovw, oap
	})
}

func actGitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeT(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(path[:strings.LastIndexByte(path, '/')], 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// qRowT: 대기열 줄(17칸) — 이름·프로젝트·상태·먼저 띄움 칸만
func qRowT(n, p, st, ef string) Row {
	r := Row{n, p, "-", "-", "-", st, "-", "-", "-", "-", "-", "-", "-", ef, "-", "-", "-"}
	return r
}

func TestActTick(t *testing.T) {
	actSave(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"} {
		t.Setenv(k, v)
	}
	d := t.TempDir()
	org, repo := d+"/o.git", d+"/app"
	actGitT(t, d, "init", "-q", "--bare", "-b", "main", org)
	actGitT(t, d, "clone", "-q", org, repo)
	writeT(t, repo+"/base.txt", "b\n")
	writeT(t, repo+"/lib/b.go", "package b\n")
	actGitT(t, repo, "add", "base.txt", "lib/b.go")
	actGitT(t, repo, "commit", "-q", "-m", "base")
	actGitT(t, repo, "push", "-q", "origin", "HEAD:main")
	actGitT(t, repo, "remote", "set-head", "origin", "main")
	// 주차 가지 refs/park/pk(cmd/x.go), 제출 가지 lane/sub(README.md, 무시할 docs/frag/n.md)
	actGitT(t, repo, "checkout", "-q", "-b", "pk")
	writeT(t, repo+"/cmd/x.go", "package x\n")
	actGitT(t, repo, "add", "cmd/x.go")
	actGitT(t, repo, "commit", "-q", "-m", "pk")
	actGitT(t, repo, "push", "-q", "origin", "HEAD:refs/park/pk")
	actGitT(t, repo, "checkout", "-q", "-b", "sub", "origin/main")
	writeT(t, repo+"/README.md", "r\n")
	writeT(t, repo+"/docs/frag/n.md", "n\n")
	actGitT(t, repo, "add", "README.md", "docs/frag/n.md")
	actGitT(t, repo, "commit", "-q", "-m", "sub")
	actGitT(t, repo, "push", "-q", "origin", "HEAD:refs/heads/lane/sub")
	actGitT(t, repo, "checkout", "-q", "main")
	// 떠 있는 레인 live의 작업 트리: 커밋 lib/a.go, 안 올린 lib/b.go 고침, 새 파일 lib/new.go
	actGitT(t, repo, "worktree", "add", "-q", "-b", "live", repo+".wt/live", "origin/main")
	wt := repo + ".wt/live"
	writeT(t, wt+"/lib/a.go", "package a\n")
	actGitT(t, wt, "add", "lib/a.go")
	actGitT(t, wt, "commit", "-q", "-m", "live")
	writeT(t, wt+"/lib/b.go", "package b // 고침\n")
	writeT(t, wt+"/lib/new.go", "package n\n")

	confLines = lines("local here\nrepo app here " + repo + "\noverlap_ignore app docs/frag/*\n")
	LOCAL, dry, FETCHED, FETCHBAD = "here", false, "", ""
	// live는 작업 트리, sub는 떠 있지만 작업 트리가 없어 원격 제출 가지로, gone은 떠 있지 않고 가지도 없어 못 읽음
	OREC = []Row{{"live", "app", "here", "t1", "s", "b", ",-,", "-"}, {"sub", "app", "here", "t2", "s", "b", ",-,", "-"},
		{"gone", "app", "here", "t3", "s", "b", ",-,", "-"}}
	TERMS = []Term{{M: "here", Addr: "t1", Name: "live"}, {M: "here", Addr: "t2", Name: "sub"}}
	rows := []Row{qRowT("pk", "app", "wait", "parked"), qRowT("fresh", "app", "wait", "-")}
	actTick(rows)
	want := map[string][]string{"app\tlive": {"lib/a.go", "lib/b.go", "lib/new.go"}, "app\tsub": {"README.md"}, "app\tpk": {"cmd/x.go"}}
	if !reflect.DeepEqual(ACT, want) {
		t.Errorf("실제 변경\n got %#v\nwant %#v", ACT, want)
	}
	if got := rowsBar(actKeys("app", "pk")); got != "A\tcmd/x.go|" {
		t.Errorf("actKeys pk: %q", got)
	}

	// 조회 실패: 원격을 못 읽으면 그 프로젝트는 실제 변경 없이(예약만으로) 판정
	actGitT(t, repo, "remote", "set-url", "origin", d+"/없음.git")
	FETCHED, FETCHBAD = "", ""
	actTick(rows)
	if len(ACT) != 0 {
		t.Errorf("조회 실패인데 실제 변경이 잡혔다: %#v", ACT)
	}
}

// 감사 중간(atow8kz-rpid2c): 쓰기 소유가 없는 프로젝트에서도 주차 줄의 실제 변경을 읽어, 같은 파일을 고친 주차 줄 둘이
// 같은 바퀴에 함께 재개되지 않는다(먼저 재개한 pk1의 키 A가 OWN에 들어가 pk2는 재개 미룸)
func TestActParkedPairNoOwner(t *testing.T) {
	actSave(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	d := t.TempDir()
	org, repo := d+"/o.git", d+"/app"
	actGitT(t, d, "init", "-q", "--bare", "-b", "main", org)
	actGitT(t, d, "clone", "-q", org, repo)
	writeT(t, repo+"/base.txt", "b\n")
	actGitT(t, repo, "add", "base.txt")
	actGitT(t, repo, "commit", "-q", "-m", "base")
	actGitT(t, repo, "push", "-q", "origin", "HEAD:main")
	actGitT(t, repo, "remote", "set-head", "origin", "main")
	for _, n := range []string{"pk1", "pk2"} {
		actGitT(t, repo, "checkout", "-q", "-b", n, "origin/main")
		writeT(t, repo+"/cmd/x.go", "package x // "+n+"\n")
		actGitT(t, repo, "add", "cmd/x.go")
		actGitT(t, repo, "commit", "-q", "-m", n)
		actGitT(t, repo, "push", "-q", "origin", "HEAD:refs/park/"+n)
	}
	actGitT(t, repo, "checkout", "-q", "main")
	confLines = lines("local here\nrepo app here " + repo + "\n")
	LOCAL, dry, FETCHED, FETCHBAD, OVM, TERMS = "here", false, "", "", "block", nil
	OREC, OWN, QEDGES, THENMAP = nil, nil, nil, nil
	actTick([]Row{qRowT("pk1", "app", "wait", "parked"), qRowT("pk2", "app", "wait", "parked")})
	if got := ACT["app\tpk2"]; len(got) != 1 || got[0] != "cmd/x.go" {
		t.Fatalf("쓰기 소유 없는 프로젝트의 주차 줄 실제 변경을 안 읽음: ACT=%#v", ACT)
	}
	b := d + "/b.md"
	writeT(t, b, "준비한 레인\n")
	if !ovlCheck("pk1", "app", b, "-", "-", "-", "-") {
		t.Fatal("pk1: 앞 레인이 없는데 막혔다")
	}
	ovlPlaced("pk1", "app", b, "-", "-")
	OREC = []Row{{"pk1", "app", "here", "t1", "s", b, ",-,", "-"}} // place → ownAdd 흉내
	OVWAIT = nil
	if ovlCheck("pk2", "app", b, "-", "-", "-", "-") {
		t.Errorf("pk2가 pk1과 같은 cmd/x.go를 고쳤는데 같은 바퀴에 함께 재개됨 (OWN=%v)", OWN)
	} else if got := rowsBar(OVWAIT); got != "pk2\tpk1\tcmd/x.go\t-\tapp|" {
		t.Errorf("재개 미룸 기록: %q", got)
	}
}

// 겹침 판정: 실제 겹침은 막고(새 줄은 겹침 대기, 주차한 줄은 재개 미룸), 안 겹치면 띄우고, 앞 레인이 착지해 소유가 풀리면 주차한 줄이 이어 뜬다.
// 실제 변경을 못 읽은 주차 줄은 지시문 예약만으로 판정
func TestOvlCheckActual(t *testing.T) {
	actSave(t)
	d := t.TempDir()
	confLines = lines("local here\nrepo app here " + d + "/app\n")
	writeT(t, d+"/app/cmd/x.go", "package x\n") // 언급 키는 저장소에 있는 파일만
	LOCAL, OVM, TERMS = "here", "block", nil
	QEDGES, THENMAP = nil, nil
	bf := func(n, s string) string {
		writeT(t, d+"/"+n+".md", s)
		return d + "/" + n + ".md"
	}
	front := func() {
		OREC = []Row{{"front", "app", "here", "t1", "s", "b", ",-,", "-"}}
		OWN = []Row{{"app", "front", "A", "cmd/x.go"}}
	}
	front()
	ACT = map[string][]string{"app\tfront": {"cmd/x.go"}, "app\tpk": {"cmd/x.go", "lib/y.go"}, "app\tpk2": {"lib/y.go"}}
	ACTPARK = map[string]bool{"app\tpk": true, "app\tpk2": true}
	check := func(n, brief string) bool {
		OVWAIT = nil
		return ovlCheck(n, "app", brief, "-", "-", "-", "-")
	}
	if check("pk", bf("pk", "준비한 레인\n")) {
		t.Error("주차한 pk: 실제 변경이 앞 레인 실제 변경과 겹치는데 이어 떴다")
	} else if got := rowsBar(OVWAIT); got != "pk\tfront\tcmd/x.go\t-\tapp|" {
		t.Errorf("재개 미룸 기록: %q", got)
	}
	if check("fresh", bf("fresh", "파일: cmd/x.go\n")) {
		t.Error("새 줄: 예약이 앞 레인 실제 변경과 겹치는데 떴다")
	}
	if !check("pk2", bf("pk2", "준비한 레인\n")) {
		t.Error("주차한 pk2: 안 겹치는데 막혔다")
	}
	if !check("other", bf("other", "파일: lib/z.go\ncmd/x.go 는 읽기만\n")) {
		t.Error("새 줄: 언급만 겹치는데 막혔다(언급은 경고)")
	} else if got := rowsBar(OVWARN); got != "front\t언급\tcmd/x.go|" {
		t.Errorf("언급 경고: %q", got)
	}
	// 조회 실패: pk의 실제 변경을 못 읽음 → 지시문 키만(예약 없음) → 띄움
	delete(ACT, "app\tpk")
	if !check("pk", d+"/pk.md") {
		t.Error("조회 실패: 예약 판정만이어야 하는데 막혔다")
	}
	// 재개 순서: 앞 레인이 착지해 쓰기 소유가 풀리면 주차한 pk가 이어 뜬다
	ACT["app\tpk"] = []string{"cmd/x.go"}
	front()
	if check("pk", d+"/pk.md") {
		t.Fatal("앞 레인 착지 전에 pk가 이어 떴다")
	}
	OREC, OWN = nil, nil
	if !check("pk", d+"/pk.md") {
		t.Error("앞 레인 착지 뒤에도 pk가 막혔다")
	}
}
