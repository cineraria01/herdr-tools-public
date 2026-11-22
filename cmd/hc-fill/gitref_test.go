package main

// gofix3-gitref(재감사 아스트라 2바퀴 ②-C): 병합기 큐 조회는 응답 전체가 온전할 때만 받는다

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// fill/R-214: 큐 조회가 정상 켬 표식 한 줄 + 이름 없는 SHA 한 줄·종료 0이면 그 프로젝트 큐 전체가 모름(2) — 「큐·착지에 없음」(1)으로
// 확정하지 않는다. 다시 맞춤은 미룬다
func TestGitrefQueuePartial(t *testing.T) {
	w, git := rlRepo(t)
	git(w, "push", "-q", "origin", "HEAD:refs/herdr/merger/box", "HEAD:refs/merge-queue/1795317520-l-a")
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	a := strings.Repeat("a", 40)
	sh := "#!/bin/sh\ncase \"$*\" in *refs/merge-queue/*)\n  printf '%s\\trefs/herdr/merger/box\\n%s\\n' " + a + " " + a + "; exit 0 ;;\nesac\nexec \"" + real + "\" \"$@\"\n"
	if err := os.WriteFile(DATA+"/git", []byte(sh), 0o755); err != nil {
		t.Fatal(err)
	}
	if q, l := mqueued("app", "l-a"), mqlanding("app", "l-a"); q != 2 || l != 2 {
		t.Fatalf("잘린 줄이 섞인 큐 응답: mqueued %d · mqlanding %d, 바람 2·2(모름)", q, l)
	}
	if err := os.Remove(DATA + "/git"); err != nil {
		t.Fatal(err)
	}
	MQP, MQS, MQBAD = "", nil, ""
	if q, l := mqueued("app", "l-a"), mqlanding("app", "l-a"); q != 0 || l != 1 {
		t.Fatalf("온전한 응답: mqueued %d · mqlanding %d, 바람 0·1", q, l)
	}
	if m := mgmach("app"); len(m) != 1 || m[0] != "box" {
		t.Fatalf("켬 표식: %v", m)
	}
}
