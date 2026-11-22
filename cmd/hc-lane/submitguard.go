package main

// 제출 막기(reland-live-lane — 한 프로젝트의 병합기가 sv-go-verify-lock 제출을 20분 검증한 뒤, 그 사이 같은 레인의 옛 커밋이 다시 제출돼 큐 참조가
// 바뀌어 push가 lease로 멈췄다): 같은 레인 제출을 병합기가 지금 착지 중이면(internal/landing) 종료 2, 내려는 커밋이 원격 lane/<레인>의
// 조상(옛 커밋)이면 경고 한 줄 뒤 --force-old 없이는 종료 2. 큐에 서 있기만 한 제출을 갈아 끼우는 것은 그대로 된다.
// 조회 뒤·push 전에 병합기가 착지를 시작하는 경합은 제출 push의 갈아 끼움 표식(submit.go supersede)이 막는다.

import (
	"fmt"
	"os"
)

// submitGuard <착지 중인 큐 이름|빈 값(landing.Of — 온전한 원격 조회로)> <원격 lane/<레인> SHA|빈 값> <내려는 커밋>
func submitGuard(qn, cur, sh string) {
	if qn != "" {
		eprintf("hc-lane land: %s은 병합기가 지금 착지 중 — 결과(착지·막힘)를 기다린다(refs/merge-queue/%s)\n", sln, qn)
		exit(2)
	}
	if cur == "" || cur == sh {
		return
	}
	rc := gitRunQ("merge-base", "--is-ancestor", sh, cur)
	if rc > 1 && fetchr(os.Stdout, os.Stderr, "fetch", "-q", "origin", "refs/heads/lane/"+sln) == 0 {
		rc = gitRunQ("merge-base", "--is-ancestor", sh, cur)
	}
	switch rc {
	case 0:
		eprintf("hc-lane land: 원격 lane/%s이 더 새롭다(%s) — 옛 커밋을 내려는 것인지 확인\n", sln, cut(cur, 8))
		if !lforceOld {
			eprintf("hc-lane land: 옛 커밋(%s)은 내지 않았다 — 맞으면 같은 줄에 --force-old\n", cut(sh, 8))
			exit(2)
		}
		fmt.Println("hc-lane land: --force-old — 옛 커밋을 그대로 낸다")
	case 1:
	default:
		die("land(제출): 원격 lane/%s 와 견주지 못했다(git merge-base 종료 %d) — 같은 줄을 다시", sln, rc)
	}
}
