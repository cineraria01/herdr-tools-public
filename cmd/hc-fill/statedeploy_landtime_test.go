package main

// deploy-hold-landtime(2026-11-24 지휘): 감사를 거친 커밋은 착지 때 이미 커밋 시각이 deploy_grace보다 오래돼, 착지 몇 초 뒤
// 「배포 밀림」이 거짓으로 나갔다(그 사건을 받은 다른 지휘가 손 배포 → 착지 뒤 배포가 잠금 실패). 보류의 「마지막 착지」는 커밋 시각이
// 아니라 범용 저장소 클론이 origin/main의 지금 끝을 처음 본 때(reflog 맨 위, 없으면 참조 파일 고친 때)다.
// ①·③은 고치기 전 코드(커밋 시각)로 실패한다

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ltLand: 가짜 범용 저장소에 커밋 시각 ct인 커밋을 만들고 origin/main을 그리로 — reflog 줄의 시각은 seen(0이면 지금)
func ltLand(t *testing.T, ct, seen time.Time) string {
	t.Helper()
	repo := os.Getenv("HERDR_TOOLS_REPO")
	d := strconv.FormatInt(ct.Unix(), 10) + " +0000"
	tip := gitT(t, repo, nil, "rev-parse", "refs/remotes/origin/main")
	c := gitT(t, repo, []string{"GIT_AUTHOR_DATE=" + d, "GIT_COMMITTER_DATE=" + d}, "commit-tree", "-p", tip, "-m", "lane-x: 고침", tip+"^{tree}")
	var env []string
	if !seen.IsZero() {
		env = []string{"GIT_COMMITTER_DATE=" + strconv.FormatInt(seen.Unix(), 10) + " +0000"}
	}
	gitT(t, repo, env, "update-ref", "-m", "update by push", "refs/remotes/origin/main", c)
	return repo
}

// ① 커밋 시각 20분 전 커밋이 방금 착지(원격 끝을 방금 처음 봄), 배포 아직 안 뜸 → 보류, 보류 글에 쓴 값
func TestDeployHoldLandSeenNotCommitTime(t *testing.T) {
	dhFx(t)
	now := time.Now()
	ltLand(t, now.Add(-20*time.Minute), time.Time{})
	if it := deployItems(now); len(it) != 0 {
		t.Fatalf("방금 착지인데 밀림: %+v", it)
	}
	if h := deployHold(now); !strings.Contains(h, "origin/main 끝을 처음 본 때(reflog)") {
		t.Fatalf("보류 글에 쓴 값이 없음: %q", h)
	}
}

// ② 처음 본 지 grace(5분)가 지났고 배포 안 뜸 → 밀림 항목(대조)
func TestDeployHoldLandSeenAfterGrace(t *testing.T) {
	dhFx(t)
	now := time.Now()
	ltLand(t, now.Add(-20*time.Minute), now.Add(-6*time.Minute))
	if it := deployItems(now); len(it) != 1 || it[0].Key != "deploy:m" {
		t.Fatalf("grace 지난 밀림 항목 하나여야: %+v", it)
	}
}

// ③ reflog 없음 → 참조 파일 고친 때
func TestDeployHoldLandSeenRefFile(t *testing.T) {
	dhFx(t)
	now := time.Now()
	repo := ltLand(t, now.Add(-20*time.Minute), now.Add(-time.Hour))
	if err := os.Remove(repo + "/.git/logs/refs/remotes/origin/main"); err != nil {
		t.Fatal(err)
	}
	ref := repo + "/.git/refs/remotes/origin/main"
	if err := os.Chtimes(ref, now, now); err != nil {
		t.Fatal(err)
	}
	if h := deployHold(now); !strings.Contains(h, "참조 파일 고친 때") {
		t.Fatalf("참조 파일 고친 때로 보류해야: %q", h)
	}
	if it := deployItems(now); len(it) != 0 {
		t.Fatalf("참조 파일을 방금 고쳤는데 밀림: %+v", it)
	}
	old := now.Add(-time.Hour)
	if err := os.Chtimes(ref, old, old); err != nil {
		t.Fatal(err)
	}
	if it := deployItems(now); len(it) != 1 {
		t.Fatalf("참조 파일이 오래됐으면 밀림: %+v", it)
	}
}

// ⑥ deploy_grace 0 → 착지 시각 안 봄(방금 착지여도 밀림 — 전과 같음)
func TestDeployHoldGraceZero(t *testing.T) {
	dhFx(t)
	confLines = append(confLines, "deploy_grace 0")
	now := time.Now()
	ltLand(t, now, time.Time{})
	if err := os.WriteFile(onlandDir()+"/deploy.head", []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if h := deployHold(now); h != "" {
		t.Fatalf("grace 0인데 착지 시각으로 보류: %q", h)
	}
	if it := deployItems(now); len(it) != 1 {
		t.Fatalf("grace 0이면 밀림 항목: %+v", it)
	}
}
