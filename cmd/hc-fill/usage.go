package main

// 사용법 글 = 머리(명령 줄 요약) + 하위 명령마다의 조각 help/<명령>.txt(이름순) + 꼬리(설정·대기열·종료).
// 새 하위 명령은 그 명령 파일의 register와 help/<명령>.txt(첫 줄에 「        hc-fill <명령> …」 요약 줄)만 더한다 — 이 파일은 고치지 않는다
// (2026-11-22 go-cmd-registry).

import (
	"embed"

	"herdr-tools/internal/helptext"
)

//go:embed help/*.txt
var helpFS embed.FS

var usageText = usageHead + helptext.Join(helpFS, "help", "") + usageTail

const usageHead = `사용법: hc-fill add --name <레인> --project <프로젝트> --brief <지시문 경로> [--after <레인>[,…]] [--only <기계>[,…]] [--coord <지휘 주소>]
               [--priority <긴급|높음|보통|낮음>] [--no-early] [--no-audit] [--overlap-ok | --solo] [--on-land <이름>] [--pr-of <원래 레인>]   (우선은 영어 urgent|high|normal|low도 받음, 기본 보통)
        hc-fill add --name <이름> --project <프로젝트> --brief <지시문 경로> --then <대기 중인 선행>
        hc-fill status [--owner <지휘 주소> | --mine]   (Go 판만 — 레인마다 한 줄: 단계·기계·넣은 뒤·예상)
        hc-fill edit <레인> [--after <레인>[,…]|-] [--after-any-project] [--priority <우선순위>] [--only <기계>[,…]|-] [--brief <지시문 경로>]
               [--overlap-ok | --overlap-check] [--early | --no-early] [--pr-of <원래 레인>|-]
        hc-fill ls | rm <레인> [--force] | prio <레인> <우선순위> | note <레인> <글|파일> | run [--dry-run] | watch [--every <초, 기본 60>]
`

const usageTail = `설정: ${HERDR_TOOLS_CONFIG:-~/.config/herdr-tools}/hc-fill.tsv (칸 설명은 저장소 config.example/hc-fill.tsv)
대기열: ~/.local/share/hc-fill/queue.tsv, run 지시문: 그 기계 ~/.local/share/hc-fill/runs/<레인>-run.md
종료: 0 정상, 1 오류, 2 잘못 부름, 75 다른 run·watch가 돌고 있음
`
