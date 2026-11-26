package main

// 배포 한 번은 커밋 하나(sync-pin-commit, 2026-11-24 — 지휘가 herdr-sync를 돌리는 1~2분 사이에 병합기가 착지를 넣자 맥은 앞 커밋으로
// 지은 판, 릴리스 없음, 나머지 기계는 「다름 (바이너리)」로 끝난 반쪽 배포).
// 한 실행이 커밋을 두 군데서 따로 정했다: 저장소 받기(ready의 fetch + merge --ff-only —맥 소스·기계가 받는 소스·바이너리 판)와 게시기의
// fetch origin main(검사한 커밋 — 릴리스). 둘이 갈리면 릴리스를 건너뛰고 기계마다 짓기로 떨어졌다.
// 이제 게시를 맥 저장소 맞추기 바로 뒤(맥 값을 읽기 전)로 당기고, 게시기가 검사한 커밋이 범용 저장소 HEAD보다 앞서 있으면(그사이 착지)
// 저장소를 그 커밋으로 fast-forward 한 뒤 맥 소스를 한 번 더 맞춘다. 그 뒤로는 HEAD = 정한 커밋(pinned)이고 짓기·릴리스 이름·
// 맥·기계에 까는 판·「같음/다름」 비교가 모두 그것을 쓴다(기계는 맥에 깐 소스·그 커밋의 릴리스를 받는다). 끝에 기본 가지가 그새
// 움직였으면 한 줄 알린다(실패 아님).

import (
	"fmt"
	"os"
	"regexp"
)

var (
	pinned     string                  // 이번 실행이 까는 범용 저장소 커밋(40자, 모르면 "")
	pubChecked string                  // 게시기가 검사·내보낸 커밋(binsPrepare가 릴리스에 쓴다)
	pubRepo    string                  // 설정 publish.repo
	binLost    = map[string][]string{} // 기계 → 그 판을 못 깐 까닭(installBins가 채우고 row가 끝 요약 줄에 붙인다 — 「다름 (바이너리)」만으로는 무엇을 왜 못 받았는지 몰랐다)
	hex40      = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

func pubwarnBig(s string) { fmt.Fprintf(os.Stderr, "!!! herdr-sync: %s !!!\n", s) }

// pinCommit: 맥 저장소 맞추기(repoSync) 뒤·맥 값 읽기 전에 한 번. 공개 단계 조건은 그대로(--check·스스로 배포는 게시하지 않는다)
func pinCommit() {
	if !check && os.Getenv("HERDR_SYNC_NO_PUBLISH") == "" {
		pubChecked, pubRepo = publish(pubwarnBig)
		head, rc := gitq("-C", REPO, "rev-parse", "HEAD")
		if hex40.MatchString(pubChecked) && rc == 0 && head != pubChecked {
			// 검사한 커밋이 HEAD의 자손일 때만 옮긴다(아니면 그대로 — release가 「릴리스 건너뜀」으로 알린다)
			if _, rc := gitq("-C", REPO, "merge-base", "--is-ancestor", head, pubChecked); rc == 0 {
				if _, rc := gitq("-C", REPO, "merge", "-q", "--ff-only", pubChecked); rc != 0 {
					die("범용 저장소를 공개 검사한 커밋 %s로 옮기지 못했다 (git merge --ff-only) — 다시 돌린다", pubChecked[:12])
				}
				say("범용 저장소 %s → %s (그사이 기본 가지가 앞섬 — 이번 배포는 공개 검사한 이 커밋 하나로)", head[:12], pubChecked[:12])
				build()
				repoSync()
			}
		}
	}
	if h, rc := gitq("-C", REPO, "rev-parse", "HEAD"); rc == 0 && hex40.MatchString(h) {
		pinned = h
	}
}

// movedNote: 배포가 끝날 때 기본 가지가 그새 움직였으면 한 줄(종료 코드는 그대로). 다시 보지 못하면 모름으로 찍는다
func movedNote() {
	if check || pinned == "" {
		return
	}
	om, rc := "", 1
	if fetchMain() == "" {
		om, rc = gitq("-C", REPO, "rev-parse", "-q", "--verify", "refs/remotes/origin/main^{commit}")
	}
	switch {
	case rc != 0 || !hex40.MatchString(om):
		fmt.Printf("배포한 판 %s — 기본 가지를 다시 보지 못했다(fetch 실패 — 움직였는지 모름)\n", pinned[:12])
	case om != pinned:
		fmt.Printf("배포한 판 %s — 기본 가지는 그새 %s로 움직였다(다음 배포에)\n", pinned[:12], om[:12])
	}
}
