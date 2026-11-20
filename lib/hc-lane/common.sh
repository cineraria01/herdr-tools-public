# hc-lane 함수: common (bin/hc-lane가 읽는다. 함수는 bin/hc-lane 안 원래 정의 순서 그대로)

die() {
  printf 'hc-lane: %s\n' "$*" >&2; [ "${cmd-}" != clean ] || cj stopped "$*"
  # 병합기가 멈추면 막힘 사건(감사 land-merge-train-2 중간 4 — main에 쓰는 곳이 하나라 조용히 서면 전체 착지가 선다)
  [ -z "${mgrun_-}" ] || (mgrun_=; report_cmd 막힘 - "병합기 멈춤: $* (클론 ${top:-?})" --name merger) >/dev/null 2>&1 18>&- 19>&-
  exit 1
}
j() { printf %s "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }
# 원격과 기본 가지를 같은 조회 결과로 정한다. 조회 오류는 기본값 선택과 구분한다.
repo_defaults() {
  local refs
  x git -C "$1" remote get-url origin >/dev/null 2>&1
  case $? in
    0) remote=1
       refs=$(x git -C "$1" for-each-ref --format='%(symref)' refs/remotes/origin/HEAD) || return 1
       base=${refs#refs/remotes/origin/}; base=${base:-main} ;;
    2) remote=0 base=main
       refs=$(x git -C "$1" for-each-ref --format='%(refname)' refs/heads/main refs/heads/master) || return 1
       if ! grep -Fxq refs/heads/main <<<"$refs" && grep -Fxq refs/heads/master <<<"$refs"; then base=master; fi ;;
    *) return 1 ;;
  esac
}
origin_base() { repo_defaults "$1" && printf '%s\n' "$base"; }
# 병합 커밋은 기본 rebase에서 사라진다. fast-forward면 원본을 쓰고, 새 기준에는 자동으로 재현할 수 있는 병합만 다시 만든다.
rebase_merge_flag() {
  local merges merge details
  merges=$(git rev-list --merges "$1..HEAD") || return 75
  [ -n "$merges" ] || return 0
  git merge-base --is-ancestor "$1" HEAD
  case $? in 0) echo keep; return 0 ;; 1) ;; *) return 75 ;; esac
  for merge in $merges; do
    details=$(git show --remerge-diff --no-ext-diff --format=%P --name-only "$merge") || return 75
    # 부모가 셋 이상이거나, 두 부모를 다시 합친 트리와 다른 경로가 있으면 원래 풀이를 잃을 수 있다.
    case $details in *' '*' '*|*$'\n'*)
      lerr "hc-lane land: 병합 커밋 ${merge:0:9}의 풀이를 자동으로 재현할 수 없다. 원본을 보존했다. 새 기준에서 풀이를 확인한 뒤 다시 제출한다"
      return 2 ;;
    esac
  done
  echo --rebase-merges
}
cj() {  # clean 결과 JSON 한 줄(앞에서 멈춰 아직 정하지 않은 값은 빈 값)
  printf '{"clean":"%s","reason":"%s","dry_run":%s,"on":"%s","repo":"%s","worktree":"%s","branch":"%s","base":"%s","teardown":"%s"}\n' \
    "$1" "$(j "${2-}")" "$([ "${dry-0}" = 1 ] && echo true || echo false)" "$(j "${on-}")" "$(j "${repo-}")" "$(j "${wt-}")" \
    "$(j "${branch-}")" "$(j "${base-}")" "${td-none}"
}
usage() {
  cat >&2 <<'EOF'
사용법: hc-lane [--on <기계>] [--read | --direct] [--branch <이름>] [--dry-run] [--print-steps] [--brief <지시문>]
               --name <레인> --repo <저장소 경로> [-- <실행 명령…>]
  기본          fetch → <저장소>.wt/<브랜치>에 origin/<기본> 기준 워크트리 → .claude/herdr-setup.sh(있으면) → hc run --bg
                쓰기 레인은 hc run 직전에 레인 임대 refs/hc-lanes/<레인>을 없을 때만 잡는다(아래 lease): 이미 있으면 「이미 돎」 종료 10,
                조회·push 실패면 종료 1 — 둘 다 띄우지 않고 방금 만든 워크트리를 되돌린다. 레인 환경 HC_LANE_LEASE = 그 임대 값
                원격 origin이 없는 저장소는 fetch 없이 로컬 <기본>(main, 없으면 master) 기준
                (모든 git fetch는 같은 클론의 참조 잠금 경합(cannot lock ref 등)이면 1·2·4초 쉬고 3번까지 다시)
                메인 폴더 앞당김: fetch 뒤(--read도 fetch 뒤)와 land 착지 뒤, 메인 폴더가 기본 브랜치이고 추적 파일 변경·진행 중인
                merge/rebase·skip-worktree 표시가 없고 메인 폴더를 cwd로 쓰는 working·blocked 터미널(자신 빼고)이 없으면
                git merge --ff-only --no-overwrite-ignore origin/<기본>. 못 하면(추적 안 된·무시된 파일과 부딪혀 git이 거절 포함) 경고
                한 줄 「메인 폴더 앞당김 건너뜀: <까닭>」 뒤 그대로 둔다(레인·착지는 그대로 진행. stash·reset 안 씀)
  --read       워크트리 없이 --repo 폴더 그대로에서 띄운다(조사·감사처럼 커밋하지 않는 레인). 저장소 안 하위 폴더나
                저장소가 아닌 폴더도 된다(맨 hc run 대신 쓰면 아래 시작 확인을 받는다)
  쓰기 레인은 대기열로: 지휘 터미널($HC_TERMINAL_ID가 살아 있고 레인·감시 터미널이 아님)이 쓰기 레인을 HC_LANE_FILL=1(hc-fill) 없이
                띄우면 아무것도 만들기 전에 「쓰기 레인은 hc-fill add로 대기열에 넣는다 …」와 종료 2. 레인 안(HC_LANE)·터미널 밖·
                판정 못 함(hc get 실패)·--read·--print-steps·--dry-run은 막지 않는다
  --direct      그래도 지금 이 자리에 띄운다(오류 출력 「대기열 밖에서 띄움(--direct)」, 레인 기록 lanes.tsv 넷째 칸 direct)
  --branch      브랜치 이름(기본: 레인 이름). 워크트리 폴더 이름은 브랜치의 / 를 - 로 바꾼 것
  --on          git·설정·실행을 그 기계에서(hc exec/run --on). --repo는 그 기계의 절대 경로(로컬 ~ 펼침 주의).
                설정 단계는 그 기계에 zsh·~/.zshrc가 있으면 그걸 읽고 돈다(hc 코어 PATH에 bun 등이 없다)
                띄운 레인은 맥 Herdr 창의 그 기계 공간(이름 = 기계)에 탭으로 보인다(hc run --label herdr.space=<id>, 창 없으면 알림만)
  --dry-run     만들지도 띄우지도 않고 할 일과 착지 절차만 출력
  --print-steps 레인 지시문에 붙일 착지·정리 절차만 출력(실행 명령 불필요)
  --brief       지시문에 착지·정리 절차(--read는 판정 알림)를 붙인 실행용 사본을 대상 기계에 두고, 실행 명령의 {brief}
                자리에 그 경로를 넣는다. 사본 자리: 설정 lane.brief_dir(그 기계 경로, ~ 는 그 기계 집)/<레인>-run.md,
                없으면 지시문과 같은 폴더의 <이름>-run.md. -- 뒤 명령이 없으면 설정 lane.run 틀(셸 문법)
  설정 lane.notify no면 hc run --no-notify(hc가 지휘 입력에 종료·막힘을 넣지 않음 — 지휘가 hc-lane wait를 늘 걸 때만)
  HC_LANE_FILL=1(hc-fill)이면 hc run --no-parent(대기열 레인·감사 줄은 부른 터미널이 끝나도 함께 꺼지지 않게)
  띄운 뒤       화면에 Claude 폴더 신뢰 질문(「❯ No, exit」·「Yes, I trust this folder」)이 보이면 그 터미널에만 신뢰를 고른다
                (설정 lane.auto_trust off면 안 함). 글자가 다르면 건드리지 않고 「모름 — 시작 화면 확인」.
                외부 가져오기 질문(「Allow external CLAUDE.md file imports?」)은 목록 경로가 모두 작업 폴더의 git 저장소
                안일 때만 허용을 고른다(lane.auto_trust off면 안 함). 밖이 하나라도 있으면 「모름 — 외부 가져오기 질문(<경로>)」, 종료 5
                --start-wait <초>(이 호출만) 또는 설정 lane.start_wait초(기본 60, 0이면 이 단계를 끔. status를 내지 않는
                명령·CLI는 0으로) 안에 hc get status가 안 잡히면 화면 끝에서 막힘 낱말을 찾아
                한 줄로 알리고 종료 5 시작 안 됨(원인 모름·신뢰 질문·끝남), 6 로그인·인증 막힘, 7 키체인 막힘(터미널은 남긴다)
                status가 잡힌 뒤 설정 lane.first_turn_wait초(기본 60, 0이면 끔) 안에 turn > 0 또는 status working을
                한 번도 못 보면 사건 막힘 「첫 턴 없음 <주소>」과 start noturn(종료 0). hc-fill에는 첫 status에서 start ok와
                주소를 돌려주고 따로 감시한다(진단은 hc-lane/watch.log).
       hc-lane clean [--on <기계>] [--dry-run] [--archive] --repo <저장소 경로> (--branch <브랜치> | --name <레인>)
  clean         착지한 레인의 워크트리·브랜치를 지운다: fetch 뒤 브랜치(와 워크트리 HEAD)가 origin/<기본>에
                들어갔는지 → 추적 변경·무시되지 않은 untracked가 없는지(등록이 지워지고 폴더만 남았으면 저장소
                .gitignore로) 확인하고, 하나라도 걸리면 아무것도 지우지 않고 멈춘다(종료 1) → .claude/herdr-teardown.sh
                (있으면, 실패해도 계속) → worktree remove → 남은 무시 파일 폴더 삭제 → branch -d
                무시된 보관물 .qa/(어느 깊이든)는 teardown 전에 메인 폴더(첫 작업 트리)의 같은 자리로 복사(덮지 않음)하고
                「옮김 <워크트리>/.qa → <메인>/.qa」 한 줄. 같은 이름의 다른 파일·복사 실패·메인 폴더 없음이면 멈춤(안 지움)
                --archive(명시할 때만): 다른 곳에서 rebase돼 착지해 안 들어간 것으로 보이는 가지도, 작업 폴더가 깨끗하면 안 들어간
                커밋을 원격 refs/archive/<가지>-<sha8>에 push해 보관한 뒤 같은 절차로 지운다(가지는 보관한 값일 때만 지움).
                push가 하나라도 실패하면 아무것도 지우지 않는다. 되살리기: git fetch origin refs/archive/<가지>-<sha8>
       hc-lane fetch [--on <기계>] --repo <저장소 경로> [<참조…>]
  fetch         그 기계 저장소에서 git fetch origin [<참조…>]. --on이면 hc exec로, zsh·~/.zshrc가 있으면 그걸 읽은 셸에서
                (맨 ssh·hc 코어 환경에는 git 자격 도우미 PATH가 없다). 종료 0 받음, 1 실패
레인 환경: HC_LANE LANE_REPO LANE_WORKTREE LANE_BRANCH LANE_BASE (--read는 HC_LANE LANE_REPO LANE_BASE), HC_LANE_OWNER
(주인 지휘 — 레인 기록 셋째 칸, 레인 안에서 띄운 하위 레인도 물려받음. 늘 넣는다: 받은 HC_LANE_OWNER → 지휘 터미널에서 띄우면 그 지휘
→ 모르면 *(주인 없음). report가 사건 11칸에 싣는다), HERDR_LAND_PRIORITY가 있으면 그것도(hc-fill이 넣는
착지 줄 우선 — 이 레인을 기다리는 대기 줄 수). hc run 라벨 hc-lane=1
       hc-lane early-wait <선행 레인>…   (먼저 띄움 레인의 기다림 블록이 기한 뒤에 부른다)
  early-wait    hc-fill이 이 기계에 써 준 선행 상태(~/.local/share/hc-fill/after.tsv)로 선행마다 「<선행>: <까닭>」. 모두 go이고 기록이
                15분 안이면 「선행이 살아 진행 중 …」과 종료 0(계속 기다림), 기록 없음·묵음·stop이면 1, 인자 없으면 2
       hc-lane land [<기본 브랜치>] [--verdict <판정 ID>]   (레인 작업 폴더 안에서)
  --verdict     hc-fill 자동 감사가 승인한 변경만 착지: 그 판정(~/.local/share/hc-fill/verdicts/<레인>/<ID>.md)이 이 실행의 통과
                판정이고 처음 부를 때 HEAD가 승인 커밋이어야 한다(아니면 종료 2). 시작 표시(<ID>.ack landing + 승인 변경 patch-id)를 남기고,
                push 직전 <기준>..HEAD의 patch-id가 같을 때만 push한다(차이가 rebase뿐 — 다르면 종료 2, 새 커밋으로 report 완료).
                push 응답만 잃었으면 다시 부를 때 끝을 마친다. 끝나면 <ID>.done. hc-fill이 취소한 판정(void)은 종료 2.
                .claude/herdr-land.sh 위임 경로는 push 관문(hc-lane guard, patchid 판)이 push 범위를 대조한다(관문 없으면 거절)
       hc-lane verdict <판정 ID> [--name <레인>]   (레인 안에서 — hc-fill이 「감사 판정이 왔다 … 판정 <ID>」로 깨우면)
  verdict       그 판정 하나를 연다. 이 레인 실행(<터미널>@<created_ms>, $HC_LANE_JOB을 주면 그 값)의 판정이고 HEAD가 승인 커밋일 때:
                통과는 착지 줄(hc-lane land <기본> --verdict <ID>)만 찍고(착지가 시작 표시를 남긴다), 지적은 <ID>.ack에 taken을 한 번
                남기고 고칠 일을 찍는다. HEAD가 다르면 아무것도 안 쓰고 「지금 커밋으로 report 완료」. 다른 실행의 판정은 손대지 않는다(종료 1).
                취소된(void) 판정은 아무것도 안 함. hc get·git 실패는 아무것도 안 쓰고 종료 75(다시 부른다)
  land          착지: .claude/herdr-land.sh가 있으면 HERDR_LAND=1 HERDR_LAND_PRIORITY=<수, 없거나 숫자 아니면 0> bash .claude/herdr-land.sh <기본>. 없으면
                fetch → rebase origin/<기본> → .claude/herdr-verify.sh(없으면 실패, push 안 함) →
                HERDR_LAND=1 git push origin HEAD:<기본>(fast-forward만, 거절이면 fetch부터 HERDR_LAND_TRIES번, 기본 5)
                origin이 없으면 fetch·push 대신 로컬 <기본> 기준으로 rebase하고, 메인 클론의 로컬 <기본>을 fast-forward로 올린다
                (<기본>이 체크아웃된 폴더면 그 폴더에서 merge --ff-only, 아니면 update-ref. 앞서 나갔으면 처음부터 다시)
                rebase가 내용 충돌로 멈췄는데 rerere가 기록한 풀이로 그 충돌 경로를 모두 풀었으면(미병합·rerere remaining 없음)
                GIT_EDITOR=true git rebase --continue로 이어 간다. 일부만 풀렸거나 충돌이 아닌 중단(훅 등)이면 멈춘 그대로 종료 3
                (쓰기 레인 작업 트리를 만들 때 공용 git 폴더에 rr-cache를 만든다 — 저장소 설정에는 안 써서 rerere.enabled를 정하지
                않은 가지만 git 기본으로 켜지고 정해 둔 값·조건부 false는 그대로. 「Resolved」로 푼 경로는 land가 git add)
                원격이 있으면 main push에 --atomic으로 착지 표식 refs/hc-landed/<레인>/<UTC 초>(= 착지한 커밋)와 이 레인 임대 지우기
                (HC_LANE_LEASE 값 lease)를 함께 싣는다(herdr-land.sh에는 HERDR_LAND_PUSH_EXTRA로 넘김). 못 실었으면 착지 뒤 따로
                종료 0 착지, 1 검증 실패·검증 없음·push 실패·인자 오류, 3 rebase 충돌, 4 거절 횟수 넘음
                검증(herdr-land.sh면 그 전체) 출력은 화면 그대로 + ${XDG_STATE_HOME:-~/.local/state}/hc-lane/land-logs/<레인>-<시각>-<pid>.log
                (14일 지난 것은 지움). 실패면 실패 줄(--- FAIL·FAIL <패키지>·panic·(fail)·✗·✖·not ok, 최대 40줄 — 이번에 생긴
                .qa/<날짜>/herdr-verify-* 폴더의 *.log도 뒤짐)·그 폴더·「전체 로그 <경로>」를 오류 출력에, $HC_LANE이면 중간 사건에도 경로
                병합기가 켜진 저장소(원격 참조 refs/herdr/merger/<병합기 기계>가 있음 — hc-fill 설정 「merger <프로젝트> <기계>」가
                켜고 끔)에서는 착지 대신 제출: HEAD를 refs/heads/lane/<레인> + refs/merge-queue/<UTC초>-<레인> + 제출 기록에 한 번의
                원자 push(같은 레인의 옛 표식은 지움, 기본 브랜치는 안 건드림), 접수가 확인되면 「중간 <sha> 제출 <ID> → <큐>」 사건과
                끝 줄 「제출 …」, 종료 0(이미 들어간 커밋이면 할 것 없음 0). 켜졌는지(원격 설정·켬 표식) 못 읽으면 착지하지 않고 75.
                HERDR_MERGER=1이면 늘 착지. HERDR_LAND_AFTER는 최소판에 없다(주면 1). clean은 원격 lane/*에 그 SHA 그대로 있는 가지를 「제출됨」으로 지운다.
                HERDR_LAND_PUSH_EXTRA="<git push 인자> …"(refspec — 왼쪽 HEAD = 기본 브랜치에 넣는 커밋 — 와 --force-with-lease=<참조>:<SHA>;
                병합기는 「:<큐> --force-with-lease=<큐>:<SHA>」와 HEAD:refs/hc-landed/<제출 ID>)면 기본 브랜치와 한 번의 --atomic push로
                싣고, lease 참조가 바뀌었으면(재제출) 기본 브랜치를 올리지 않고 종료 6. 병합기 안 착지(HERDR_MERGER=1)는 기본 브랜치에
                .claude/herdr-land.sh가 있으면 그 기본 브랜치 판(제출 트리 속 판 아님)에 맡기고, 기본 브랜치에 없는 스크립트가 작업 트리에
                있거나 rebase로 막 생겼으면 종료 7. 이 둘(과 HERDR_MERGER)은 검증 자식에 물려주지 않는다
       hc-lane merger --repo <병합기 전용 클론> [--once] [--tick <초>] [<기본 브랜치>]   (병합기 기계에서 하나만)
  merger        큐(refs/merge-queue/*)를 이름 순으로 묶음(HC_LANE_MERGER_BATCH, 기본 3 — 앞 구성원 위에 차례로 얹고 검증 한 번·push
                한 번, 실패면 마지막 것 빼고 다시, 기본 브랜치의 .claude/herdr-merge-solo 경로는 묶음에 하나만)으로 —
                새 임시 작업 트리에서 준비 훅(.claude/herdr-setup.sh, 있으면) 뒤
                레인 이름 그대로의 로컬 가지로 HERDR_MERGER=1 hc-lane land, 큐 표식은 HERDR_LAND_PUSH_EXTRA로 기본 브랜치 push가 함께
                지운다. 기본 브랜치에 없는 .claude/herdr-land.sh를 제출이 새로 들이면 반송, 종료 7은 일시 실패. 바퀴마다 켬 표식이 이 기계(HC_LANE_MACHINE,
                없으면 hostname -s)인지 보고, 꺼졌으면 쉬고 다른 기계면 멈춘다. 표식은 길 안내일 뿐 — 정확성은 기본 브랜치 CAS
                (큐 참조를 건드리는 원격 쓰기는 모두 그 큐의 지금 SHA에 lease). 후보는 매번 새 임시 작업 트리(클론 옆 <클론>.merger-wt/ — .git 밖).
                착지 전에 일지(<git 폴더>/hc-merger/cur: 가지·기준·후보·제출)를 쓰고, 바퀴마다 새로 고르기 전에 그 일지부터 정리
                (원격 refs/hc-landed/<제출 ID>가 기본 브랜치의 조상 → 완료, 그 밖 → 후보 버림·제출은 큐에 남았으면 처음부터).
                착지 종료와 상관없이 원격이 후보를 담았나를 먼저 본다. 다른 기본 브랜치로 낸 제출은 건드리지 않음.
                일시 실패(75·4·원격 받기·push 결과 모름)는 큐에 두고 3번 넘으면 돌려보냄. git 조회 실패는 모두 「모르면 안 함」.
                들어가면 제출 기록을 refs/merge-done/<레인>으로 옮기고 lane 가지를 지운 뒤 「착지」 사건(--name <레인>, 제출 때 지휘가
                있으면 --to, 레인 터미널이 살아 있으면 hc send). 실패면 표식을 refs/merge-failed/<이름>으로 옮기고
                (lane 가지는 둠) 「막힘」 사건(까닭·로그 …/hc-lane/merger-logs/). 사건은 보낼 함(hc-merger/out)을 거쳐 끊겨도 다음
                바퀴가 보내고, 한 제출에 한 번만(사건·일지에 「제출 ID <ID>」 열쇠가 이미 있으면 안 적음). 잠금은 잠금 파일의 flock(준비 훅·착지 자식에는 안 물림 — 병합기가 죽으면 바로 놓임). GOCACHE가 없으면
                ~/.cache/go-build. 빈 바퀴는 --tick초(기본 30) 쉼, --once는 큐가 빌 때까지(원격 오류·일시 실패로 남긴 바퀴면 75).
                전용 클론 = 레인 작업 트리를 두지 않는 클론. 종료 75 이미 돎
       hc-lane gc [--on <기계>] [--dry-run] [--integ[=<접두>]] [--remote-branches] [--min-age <시간>] --repo <저장소 경로>
  gc            끝난 레인 정리: 연결된 작업 폴더마다 HEAD가 origin/<기본>에 다 들어갔고(조상이거나 git cherry에 + 없음)
                깨끗하고(추적 변경·untracked 없음, 무시된 보관물 .qa/ 없음) 그 폴더를 cwd로 쓰는 hc 터미널이 없고
                마지막 git 활동이 --min-age보다 오래됐을 때만 teardown → worktree remove --force → branch -D.
                작업 폴더 없이 남은 로컬 가지(기본·현재 HEAD·작업 폴더에 붙은 가지·설정 gc.keep_branches 무늬 빼고)도 같은 판정에
                커밋·가지 reflog가 --min-age보다 오래됐으면 branch -D(「지움 가지 <가지> <sha>」, 되돌리기 git branch <가지> <sha>).
                --integ: 원격 <접두>*(기본 refs/integ/) 가운데 커밋이 origin/<기본>에 다 들어갔고 --min-age보다 오래된 것만
                지운다(값이 그대로일 때만). --remote-branches: 원격 refs/heads/*(기본 가지·gc.keep_branches 빼고)도 같은 꼴로.
                안 들어간 가지는 지우지 않고 「남김 <가지> 안 들어감 <n>개 · 마지막 <날짜>」. --min-age 기본 24h(숫자에 s·m·h·d,
                단위 없으면 시간). 메인 폴더·기본 가지·tags·claims는 안 건드린다. 마지막 줄 「지움 n · 남김 m」. 오류 종료 1
       hc-lane guard install|status|remove --repo <저장소 경로> [--branch <기본 브랜치>(install, 기본: origin/HEAD)]
  guard         그 클론의 공용 git 폴더 hooks/pre-push(워크트리 모두에 먹음)에 관문을 넣는다: refs/heads/<기본>으로 가는
                push는 HERDR_LAND=1일 때만 받고, 아니면 새 착지 줄을 알리고 거절한다. 다른 브랜치·참조는 통과.
                다른 pre-push 훅이 있거나 core.hooksPath가 있으면 건드리지 않고 멈춘다. remove는 자기가 넣은 것만 지운다.
                status 종료: 0 깔림, 1 없음, 3 다른 훅. 원격이 하나도 없는 저장소는 push가 없어 「설치할 것 없음」(종료 0)
       hc-lane lease show|drop [--on <기계>] --repo <저장소 경로> --name <레인>
  lease         레인 임대 refs/hc-lanes/<레인>(프로젝트 원격). 쓰기 레인은 hc run 직전에 없을 때만 만들어 잡고(이미 있으면 「이미 돎」
                종료 10, 조회·push 실패도 안 띄움 — 종료 1), land 성공(main과 같은 원자 push)·clean에서 놓는다. 그 밖(레인이 죽음·
                지휘가 hc-fill rm)으로 임대가 남아 다시 못 띄우면 이 drop으로 손으로 놓는다. show: 「값 제목」 또는 「-」
       hc-lane park [<한 줄>]   (먼저 띄운 레인 작업 폴더 안에서)
  park          선행을 기다리는 레인이 자리를 쥐고 놀지 않게(2026-11-20 사용자 「노는 워크트리 없이」): 깨끗한 작업 트리의 HEAD를
                원격 refs/heads/lane/<레인>(지금 값에 lease)에 올리고, 같은 --atomic push로 이 레인 임대(HC_LANE_LEASE 값일 때만)를
                놓은 뒤 「주차」 사건. 다음은 hc-lane clean(원격 lane/<레인>에 그대로 있는 가지라 「제출됨」으로 지운다) 뒤 턴을 끝낸다.
                선행이 들어가면 hc-fill이 그 가지에서 다시 띄운다(--resume). 종료 0 주차, 1 실패(같은 줄을 다시), 3 원격 없음(기다림으로)
  --resume      (띄울 때) 새 작업 트리를 origin/<기본>이 아니라 주차한 가지 origin/lane/<브랜치>에서 만든다(upstream은 origin/<기본>).
                그 가지가 없으면 띄우지 않는다. 레인은 git rebase origin/<기본> 뒤 이어 간다(hc-fill이 지시문에 적음)
       hc-lane report <중간|완료|착지|막힘|판정|제출|주차> [<sha>|-] [<한 줄>…] [--name <레인>] [--to <지휘>[,…]] [--key <키>]   (레인 안에서)
  report        이 기계 ${XDG_STATE_HOME:-~/.local/state}/hc-lane/events.tsv에 한 줄을 더한다(지우지 않음):
                시각(UTC)·기계(HC_LANE_MACHINE, 없으면 hostname)·터미널($HC_TERMINAL_ID)·레인·상태·sha·한 줄(탭·줄바꿈은 빈칸)·
                그때의 hc turn(터미널 밖이면 -)·--to(없으면 -)·발행 실행(<터미널>@<hc get created_ms 초>, 모르면 -)·
                수신 지휘($HC_LANE_OWNER, 없으면 -)·키(없으면 -) — 12칸. 옛 7~9칸 줄도 읽는다.
                --key: 같은 키(12칸) 줄이 이미 있으면 더하지 않고 「이미 있음」(종료 0). 확인과 더하기는 events.lock 잠금 안에서 한 번
                (재시도가 사건을 두 번 내지 않게 — hc-fill 지휘 넘김). events.tsv를 못 읽으면 종료 1
                레인 이름은 --name → $HC_LANE(hc-lane이 띄울 때 넣음) → hc 터미널 이름 → 작업 트리 가지(기본 가지면 -) 순. sha 자리가 sha가 아니면 한 줄로 본다
                --to: 받을 지휘를 정한다(아홉째 칸) — 그 지휘들의 watch만 보낸다(레인 주인 판정 대신. hc-fill 「놀고 있음」 알림)
       hc-lane watch --to <지휘 주소> [--all | --on <기계>…] [--state <상태,…>] [--since <시각>] [--mine]
  watch         늘 도는 감시: wait와 같은 판정·같은 커서(seen.<지휘 주소> — wait와 겹쳐 두 번 알리지 않음)로 새 사건이 생기면
                지휘 터미널에 hc send <지휘> "hc-lane: <레인> <상태> <sha> <한 줄> (<터미널>) ‖ …" 한 번(보낸 뒤에만 커서에 적음,
                실패면 다음 바퀴에 다시). 바퀴는 설정 lane.watch_tick초(기본 30). 같은 지휘로 하나만(뒤 것은 종료 75).
                지휘가 질문·권한 창을 띄운 동안(hc get status blocked, status null이면 화면 맨 아래 덩어리 — 평소 입력 칸이 없고
                마지막 가로줄 아래에 lane.watch_dialog 글자)은 보내지 않고 창이 닫힌 뒤 한 번에. 상태를 못 읽으면 보냄. status blocked는
                lane.watch_hold_max분(기본 30, 0이면 막지 않음) 넘으면 보냄, 화면으로 잡은 창은 넘어도 안 보내고 watch.log에 한 줄.
                못 읽은 기계는 「연결 안 됨 <기계>」 한 번 보내고 다음 바퀴에 다시 본다(sh를 띄울 수 없는 기계 — 예: Windows — 는
                레인 기록이 없는 것으로 보고 건너뛴다). hc ls 실패는 시작·회복 때 한 번씩.
                끝남(종료 0): 지휘 터미널이 3바퀴 이어 hc get에 없거나 끝남(연결 실패는 세지 않음), 또는 그 지휘의 살아 있는
                레인(라벨 hc-lane=1, parent 모르면 그 지휘 것)도 hc-fill 대기열의 wait 요청도 새 사건도 없이
                lane.watch_idle분(기본 60). 레인·감사를 띄울 때 hc-lane이 지휘(HC_LANE_COORD — hc-fill이 대기열 주인 지휘, 없으면
                hc-lane coord로 넘김 —, 없으면 $HC_TERMINAL_ID)의 watch가 없으면 hc run --bg --no-notify --rm --name lane-watch-<지휘>
                --label hc-lane-watch=1 로 저절로 띄운다(--all). 바퀴마다·보낼 때마다 자기 지휘 정보(coord.<주소>)와 맞춰 보고
                다르면 안 보내고 끝냄. 사건은 레인 기록(lanes.tsv — HC_LANE_OWNER가 있으면 그 지휘, 없으면 띄운 지휘 터미널.
                없으면 hc parent)의 지휘 watch만 보내고, 주인을 모르거나(-) 그 지휘가 사라진 레인의 사건은 살아 있는 모든 지휘
                watch가 보낸다(넘겨받지 않음, 지휘마다 자기 커서로 한 번, sent에 안 적음)
       hc-lane coord [--set [<주소>] | --check <주소>]
  coord         지휘마다의 지휘 정보 …/hc-lane/coord.<주소>(주소·pid·이름·마지막 레인 띄운 시각 — 지휘 터미널이 레인을 띄우거나
                wait·watch를 걸 때, hc-fill add 때 남김) 가운데 지금 hc get pid·이름이 같고 가장 최근에 레인을 띄운 지휘의 주소를 찍고 0,
                없으면 「지휘 없음 — <까닭>」과 1. --set: 그 지휘(기본 $HC_TERMINAL_ID)의 지휘 정보를 지금 시각으로 남기고 주소를
                찍는다(살아 있는 지휘 터미널이 아니면 종료 1). --check: 그 지휘가 지금 살아 있고 지휘 정보와 맞으면 0,
                다름·없음 1, 모름 2(아무것도 안 씀 — hc-fill이 대기열 주인을 넘기기 전에 본다)
       hc-lane send --all [--audits] [--from <지휘 주소>] [--] <글…>   (hc-lane broadcast <글…> = send --all)
  send          이 지휘($HC_TERMINAL_ID 또는 --from)가 띄운 살아 있는 레인 전부(원격 포함 — 레인 기록 lanes.tsv의 마지막 지휘,
                없으면 hc parent)에 hc send. 감사(레인 기록 read·이름 audit-*)는 --audits일 때만. 한 줄씩 「보냄|실패 <주소> <레인>」,
                끝 줄 「보냄 n · 실패 m」. 종료 0 다 보냄(대상 없음 포함), 1 보내기·hc ls 실패, 2 인자(--all 없음 포함)
       hc-lane events [--all | --on <기계>…] [--since <시각|30m·2h·1d>]
  events        이 기계(와 --all이면 hc machine ls의 연결된 기계마다 hc exec --on 한 번)의 events.tsv를 시각 순으로.
                원격 줄의 기계·터미널은 hc 이름(<기계>/<t…>)으로 바꿔 찍는다. 읽지 못한 기계는 「모름 <기계>」 한 줄
                sh를 띄울 수 없는 기계(예: Windows)는 레인 기록이 없는 것으로 보고 건너뛴다(events·wait·watch 모두)
       hc-lane wait [--all | --on <기계>…] [--timeout <분>(기본 110)] [--state <상태,…>] [--since <시각>] [--mine]
  wait          아직 본 적 없는 사건이 생길 때까지 HC_LANE_WAIT_TICK초(기본 60)마다 보고, 생기면 그 줄(들)을 찍고 종료 0.
                상한이면 종료 75. 찍은 사건은 이 기계 …/hc-lane/seen.<$HC_TERMINAL_ID>(지휘마다, 터미널 밖이면 seen)에
                적어 다시 알리지 않는다(바꾸는 것은 그 파일뿐). 출력이 /dev/null이면 사건을 읽을 사람이 없어 seen에 안 적고 종료 2(배경이면 출력을 파일로). 새 커서는 --since(없으면 2시간 전)부터 본다.
                --mine이면 레인의 hc parent가 부른 터미널($HC_TERMINAL_ID — 원격 레인도 보통 앞부분 없는 <t…>, 옛 꼴 …/<t…>도 맞춤)인 사건만(parent를 모르면 보임).
                한 기계에 지휘가 여럿일 때만 쓴다(지휘가 바뀌었거나 wait를 다른 터미널에서 부르면 쓰지 않는다).
                hc-lane이 띄운 레인 터미널(라벨 hc-lane=1)이 착지·판정 그 턴(report turn+1 이하, turn을 모르면 밖) 밖에서 끝났거나(한 줄 칸
                「뜻밖의 종료(<상태>)」, 완료 — 착지 전 — 뒤와 착지·판정 뒤 이어 받은 일도 알림. 착지 뒤 같은 sha(- 아님)의 완료가
                같은 턴(같은 turn 값)에만 이어지면 착지로 치고 turn은 그 완료의 것), 완료·착지·막힘·판정 없이(또는
                그 report의 turn+1보다 뒤에) done·idle로 쉬거나, blocked·error면 「멈춤」 사건(한 줄 칸 「<상태> turn <n>」).
                done·idle은 쉰 지 설정 lane.wait_grace분(기본 10, 0이면 끔)이 지나고 배경 작업이 없을 때 멈춤. 배경 작업은
                터미널 pid 자손 가운데 명령 줄에 lane.wait_procs 글자(기본 shell-snapshots — Claude Bash 도구 셸)가 든 프로세스로
                본다(이 기계 ps, 원격은 기계당 hc exec 한 번). 그런 프로세스가 없거나 pid·ps를 모르면 화면의 마지막 상태 줄(lane.wait_line
                기본 ✻ 턴 끝 줄, 또는 표시 줄 가운데 맨 아래)에 lane.wait_busy(확장 정규식, 기본 shells? still running|monitors?
                still running, 빈 값이면 끔) 표시로 본다(배경 작업이 끝나면 레인이 스스로 이어 간다). 배경 작업이 있어도
                쉰 지 lane.wait_busy_max분(기본 30)이 넘으면 멈춤. blocked·error·끝남은 바로. hc-sweep이 적은 「정리됨」은 그
                터미널의 바로 앞 report가 착지·판정이고 그 turn+1 안이거나 같은 turn의 쉼으로 이미 알렸으면 버리고, 아니면
                「뜻밖의 종료(정리됨) …」 멈춤(틈에 닫혀 목록에서 사라져도).
                --read·hc-fill 레인(레인 기록 lanes.tsv 넷째 칸 read·fill)은 「완료」도 착지·판정처럼 마지막 보고다.
                지휘가 hc kill로 닫은 레인(hc ls reason killed — parent가 없거나 아직 살아 있을 때만. parent가 끝나 함께 끝난 것은
                그대로 알림)의 끝남은 알리지 않는다. 쉬는 레인은 그 작업 트리 경로가 명령 줄에 든 프로세스가 살아 있어도 바쁨으로 본다.
                커서·보냄 열쇠는 기계 이름을 뺀 꼴(주소는 끝의 t…)이라 hc 기계 이름을 바꿔도 본 사건이 다시 안 나온다(옛 꼴 줄도 읽음).
                hc ls가 3바퀴 이어 실패하면 「hc ls 실패 n바퀴」 멈춤 한 줄로 끝낸다.
                sh를 띄울 수 없는 기계(예: Windows)는 레인 기록이 없는 것으로 보고 건너뛴다(「모름」·「연결 안 됨」 없음).
                못 읽은 기계의 레인은 그 바퀴에 blocked·error만 그대로, 쉼·끝남은 「연결 안 됨 <기계> <날짜>」(하루 한 번). 원격은 바퀴마다 기계당 한 번(쉬는 레인 확인만 그 터미널에 hc get·read)
준비·정리 훅: .claude/herdr-setup.sh(환경 LANE_*: LANE_REPO=메인 폴더, LANE_BASE=기본 브랜치), .claude/herdr-teardown.sh
실행 명령은 env K=V 로 감싸므로 hc 템플릿 이름은 쓸 수 없다. 마지막 줄은 JSON 한 줄.
EOF
  exit 2
}
val() { [ $# -ge 2 ] || die "$1 값이 없다"; }
# cfg <키> <기본값>: 개인 설정(herdr-tools-config, 곁의 것 먼저). 도구가 없는 기계는 기본값, 읽기 오류는 종료 1
cfg() {
  c=${0%/*}/herdr-tools-config
  [ -x "$c" ] || c=$(command -v herdr-tools-config) || { printf '%s\n' "$2"; return 0; }
  "$c" get "$1" "$2" || die "설정 $1 을 읽지 못했다(herdr-tools-config)"
}
# capt <초> <명령…>: 그 초 안에 안 끝나면 끊는다(perl alarm, 없으면 그냥). 코어가 응답하지 않아도 멈추지 않게
capt() { if command -v perl >/dev/null 2>&1; then perl -e 'alarm shift; exec @ARGV' "$@"; else shift; "$@"; fi; }

# hc get의 최상위 터미널만 실행의 주인이다. 하위 foreground에도 시각이 있을 수 있다.
live_created_ms() { jq -er 'select(.state == "live") | .created_ms | numbers | select(. >= 0 and . == floor)' 2>/dev/null; }
terminal_created_ms() {
  local info
  [ -n "$1" ] && info=$(capt 3 hc get "$1" --json </dev/null 2>/dev/null) && live_created_ms <<<"$info"
}

utc() { if [ $# = 0 ]; then date -u +%Y-%m-%dT%H:%M:%SZ; else date -u -r "$1" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d "@$1" +%Y-%m-%dT%H:%M:%SZ; fi; }
flat() { printf %s "$1" | tr '\t\r\n' '   '; }
# bcut <바이트> <글>: 로캘과 상관없이 바이트로 자르고, 끝에 남은 반쪽 UTF-8 글자는 버린다(iconv -c)
bcut() {
  ( LC_ALL=C; s=$2
    [ ${#s} -gt "$1" ] || { printf %s "$s"; exit 0; }
    r=$(printf %s "${s:0:$1}" | iconv -c -f UTF-8 -t UTF-8 2>/dev/null); [ -n "$r" ] || r=${s:0:$1}
    printf %s "$r" )
}
# jrec <키…>: JSON을 { 마다 끊어 조각마다 키의 문자열 값(없거나 null이면 -)을 탭으로. 첫 키가 없는 조각은 버린다(jq 없이)
jrec() {
  tr '{' '\n' | awk -v K="$*" 'BEGIN { nk = split(K, k, " ") }
    { o = ""
      for (i = 1; i <= nk; i++) {
        v = "-"
        if (match($0, "\"" k[i] "\" *: *(\"[^\"]*\"|-?[0-9]+)")) { v = substr($0, RSTART, RLENGTH); sub(/^"[^"]*" *: *"?/, "", v); sub(/"$/, "", v) }
        if (i == 1 && v == "-") next
        o = o (i > 1 ? "\t" : "") v
      }
      print o }'
}
