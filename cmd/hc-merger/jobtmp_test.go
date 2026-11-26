//go:build !windows

package main

// 작업별 임시 폴더(ht-job-tmpdir, docs/hc-sweep/job-tmp.md): 병합기가 띄우는 추측 일꾼·몫·워커 착지는 감싸개 run 꼴을 거친다 —
// 명령이 끝나면 폴더를 지운다. 설정 jobtmp.root off면 지금 그대로

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"herdr-tools/internal/jobtmp"
	"herdr-tools/internal/verified"
)

// jobRootConf: PATH의 가짜 herdr-tools-config가 jobtmp.root에 v로 답한다(다른 키는 기본값)
func jobRootConf(t *testing.T, s *sfx, v string) {
	t.Helper()
	sc := "#!/bin/sh\n[ \"$1 $2\" = \"get " + jobtmp.Key + "\" ] && { echo '" + v + "'; exit 0; }\necho \"$3\"\n"
	if err := os.WriteFile(s.dir+"/bin/herdr-tools-config", []byte(sc), 0o777); err != nil {
		t.Fatal(err)
	}
}

func v1(s *sfx) verified.Verifier { return verified.Verifier{Machine: "v1", Path: s.dir + "/v1"} }

func specLines(s *sfx) []string {
	b, _ := os.ReadFile(s.hclog)
	var o []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, "--spec-verify") {
			o = append(o, l)
		}
	}
	return o
}

// ② 추측 일꾼: env 뒤 감싸개(run 꼴·뿌리·터미널 이름 spec-<아이디 8>) 다음 hc-lane merger --spec-verify
func TestSpecDispatchJobTmp(t *testing.T) {
	oldFront(t)
	s := specFx(t)
	jobRootConf(t, s, "~/jt")
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", false)
	s.sub(1000000003, "c", false)
	s.round(1)
	l := specLines(s)
	if len(l) != 2 || len(s.dispatched()) != 2 { // 기록 줄이 감싸개로 끊기지 않아 띄움 차례를 그대로 읽는다
		t.Fatalf("일꾼 둘(띄움 %v): %q", s.dispatched(), l)
	}
	if len(l) != 2 {
		t.Fatalf("일꾼 둘: %q", l)
	}
	for _, x := range l {
		if !strings.Contains(x, " jobtmp ~/jt spec-") || !strings.Contains(x, " run hc-lane merger --spec-verify ") {
			t.Fatalf("감싸개 없이 띄움: %s", x)
		}
	}
}

// ② 몫(나눔) 일꾼도
func TestShardDispatchJobTmp(t *testing.T) {
	s := specFx(t)
	jobRootConf(t, s, "~/jt")
	m := s.merger()
	if !m.specDispatch(v1(s), strings.Repeat("a", 40), "2/2") {
		t.Fatal("못 띄움")
	}
	l := specLines(s)
	if len(l) != 1 || !strings.Contains(l[0], " jobtmp ~/jt spec-aaaaaaaa-2 run hc-lane merger --spec-verify ") {
		t.Fatalf("몫 일꾼 감싸개: %q", l)
	}
}

// ⑧ off면 감싸지 않음
func TestSpecDispatchJobTmpOff(t *testing.T) {
	s := specFx(t)
	jobRootConf(t, s, "off")
	m := s.merger()
	m.specDispatch(v1(s), strings.Repeat("a", 40), "")
	if l := specLines(s); len(l) != 1 || strings.Contains(l[0], "jobtmp") {
		t.Fatalf("off인데 감쌈: %q", l)
	}
}

// ② 워커 착지: 착지 실행이 본 TMPDIR이 뿌리 아래 작업 폴더이고, 끝나면 그 폴더가 없다(run 꼴)
func TestLandOnWorkerJobTmp(t *testing.T) {
	s, runs := landFx(t)
	root, err := os.MkdirTemp("/tmp", "jt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	jobRootConf(t, s, root)
	t.Setenv("HC_TERMINAL_ID", "t1") // 워커 터미널 안처럼(가짜 hc run은 그 자리에서 돌린다)
	seen := s.dir + "/land-tmpdir"
	t.Setenv("LAND_TMPDIR_OUT", seen)
	s.sub(1000000001, "a", false)
	s.round(1)
	if r := runs(); len(r) != 1 || !s.onMain("a.txt") {
		t.Fatalf("착지: %v\n%s", r, jdump())
	}
	b, _ := os.ReadFile(seen)
	d := strings.TrimSpace(string(b))
	if filepath.Dir(filepath.Clean(d)) != root { // macOS는 끝 「/」(운영체제 기본 꼴, ht-jobtmp-slash)
		t.Fatalf("착지가 본 TMPDIR %q 가 %s 아래 아님", d, root)
	}
	if ents, _ := os.ReadDir(root); len(ents) != 0 {
		t.Fatalf("착지가 끝났는데 작업 폴더가 남음: %v", ents)
	}
}
