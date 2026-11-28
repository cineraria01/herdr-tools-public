# hc-lane 띄우기 (지시문 사본·신뢰 질문·시작 확인·fetch)

```sh
hc-fill add --name <레인> --project <프로젝트> --brief <지시문>                            # 쓰기 레인은 대기열로(아래)
hc-lane --direct --on <기계> --brief <지시문> --name <레인> --repo <그 기계 저장소> -- claude … "{brief} 를 읽고 진행하라"
hc-lane --on <기계> --read --brief <감사 지시문> --name <레인> --repo <그 기계 저장소>   # -- 뒤가 없으면 설정 lane.run 틀
hc-lane fetch --on <기계> --repo <그 기계 저장소> [<참조…>]                               # 감사 대상 커밋 받기
```

- 쓰기 레인은 대기열로(2026-11-19 — 지휘가 쓰기 레인을 직접 띄워 hc-fill 대기열이 비고, 자리가 나도 이어 뜰 것이 없었다): 지휘 터미널(`$HC_TERMINAL_ID`가 살아 있고 레인·감시 터미널이 아님 — 레인 주인을 정하는 것과 같은 판정)이 쓰기 레인(`--read`·`--print-steps`·`--dry-run` 아님)을 `HC_LANE_FILL=1`(hc-fill이 띄울 때) 없이 띄우면 아무것도 만들기 전에 `hc-lane: 쓰기 레인은 hc-fill add로 대기열에 넣는다 …` 한 줄과 종료 2. 지금 꼭 이 자리에 띄워야 하면 `--direct`(오류 출력 `대기열 밖에서 띄움(--direct)`, 레인 기록 `lanes.tsv` 넷째 칸 `direct` — 직접 띄운 비율을 셀 수 있게. wait·send는 구현 레인과 같게 본다).
  레인 안(`HC_LANE`)·터미널 밖(launchd·cron)·레인·감시 터미널·판정 못 함(`hc get` 실패)은 그대로 띄운다.
- **실패 종류**(2026-11-24 fill-reject-hold, 2026-11-28 ht-retry-class — 업계의 「재시도해도 되는 오류와 안 되는 오류의 구분」: 띄우는 쪽이 한 곳(`cmd/hc-lane/launchaux.go` `rcRefuse`)에서 정해 종료 코드로 돌려주고, hc-fill은 그 값만 본다 — 글을 읽어 가르지 않는다).
  기준은 「같은 입력·같은 원격 상태로 다시 부르면 같은 결과인가」다. 결정적이면 hc-fill이 그 줄을 멈추고 지휘에 한 번 알린다([띄운 뒤 확인](../hc-fill/launch-check.md)).

  | 실패 자리 | 종류 | 종료 | 풀림 |
  |---|---|---|---|
  | 로컬 브랜치·원격 브랜치(`origin/<브랜치>`, fetch 뒤)·워크트리 폴더가 이미 있음 | 결정적 | 11 | 지휘 retry·edit·rm |
  | `--brief` 지시문 없음, 실행 명령의 `{brief}` 자리와 `--brief`가 안 맞음 | 결정적 | 11 | 지휘 |
  | `--resume`의 주차 가지가 없음(원격 없는 저장소·주차 사건 없는 옛 제출 가지 포함) | 결정적 | 11 | 지휘 |
  | 임대 `refs/hc-lanes/<레인>`를 다른 실행이 쥠(「이미 돎」, [레인 임대](lease.md)) | 결정적 | 11 + 그 앞 `hc-lane: 풀리면 다시: 임대 <sha>` 한 줄 | 그 임대 값이 바뀌거나 없어짐 — hc-fill이 바퀴마다 보고 스스로 다시 띄움 |
  | `--revive` 관문의 「이미 돎」(그 작업 트리를 쓰는 살아 있는 터미널·임대 주인이 다름) · 그 밖의 `--revive` 거절 | 결정적 | 10 · 11 | `hc-fill revive`가 받는다(대기열 바퀴는 `--revive`를 쓰지 않는다, [이어 띄우기](revive.md)) |
  | 있나 조회 자체의 실패(hc·git 실패·시간 넘김)·fetch 실패·임대 조회·push 실패(못 읽음 — 모르면 결정적이 아니다)·`hc run` 거절·설정 값 오류·살아 있는 같은 이름 터미널(끝나면 풀림) | 일시 | 1 | 다음 기계·다음 바퀴 |
  | `hc run` 결과 미확정·주소 못 읽음 · 시작 확인 실패·미확정 | 미확정 | 5·8(6·7은 로그인·키체인) | 그 기계를 다시 본다 |
  | 신호·panic·그 밖의 종료 | 미확정(hc-fill이 그렇게 읽음) | 그 밖 | 같음 |

  결정적 실패는 아무것도 남기지 않고(만든 워크트리는 되돌림) `hc-lane: <까닭>` 한 줄로 끝난다.
  옛 판 hc-lane의 임대 「이미 돎」 종료 10은 새 hc-fill이 일시로 읽고(안전 쪽 — 지금 알림 `launch_fail_warn` 그대로), 새 hc-lane의 11은 옛 hc-fill이 지금 거절 길(멈춤·알림·retry)로 읽는다.
- `--brief`: 지시문에 `--print-steps` 착지·정리 절차를 붙인 사본(`--read`는 판정 알림 줄)을 대상 기계에 두고 실행 명령의 `{brief}`를 그 경로로 바꾼다.
  자리는 설정 `lane.brief_dir`(그 기계 경로, `~`는 그 기계 집)의 `<레인>-run.md`, 없으면 지시문과 같은 폴더의 `<이름>-run.md`(그 기계에 같은 경로를 쓸 수 없으면 실패하고 `lane.brief_dir`을 알린다).
  `{brief}`가 없거나 `--brief` 없이 `{brief}`가 있으면 아무것도 만들기 전에 오류.
- 띄운 직후: 화면에 Claude 폴더 신뢰 질문(`❯ No, exit` / `Yes, I trust this folder`)이 보이면 그 터미널에만 신뢰를 고른다(`lane.auto_trust off`면 안 함).
  질문 글자가 다르거나 커서(`❯`)가 어느 줄에도 안 보이면 건드리지 않고 `모름 — 시작 화면 확인`. 외부 가져오기 질문(`Allow external CLAUDE.md file imports?` — 작업 폴더 밖 파일을 `@`로 가져오는 CLAUDE.md)은 `External imports:` 목록 경로가 모두 작업 폴더의 git 저장소(`git -C <작업 폴더> rev-parse --show-toplevel`) 안일 때만 그 터미널에 `Yes, allow external imports`를 고른다(`lane.auto_trust off`면 안 함).
  밖이거나 읽을 수 없는 줄(가운데 `…` 생략·`… +N imports not shown.`)이 하나라도 있으면 고르지 않고 `모름 — 외부 가져오기 질문(<경로>)`, 종료 5. 커밋하지 않는 일을 저장소 안 하위 폴더(`.qa/…`)나 저장소가 아닌 폴더에서 띄울 때도 맨 `hc run` 대신 `hc-lane --read --repo <그 폴더>`를 쓰면 이 시작 확인을 받는다(`--read`는 준 폴더 그대로에서 띄우고, 저장소가 아니면 `LANE_BASE`가 빈 값).
- 시작 확인: `--start-wait <초>`(이 호출만) 또는 `lane.start_wait`초(기본 60, `0`이면 신뢰 질문·시작 확인 끔) 안에 `hc get` status가 안 잡히면 화면 끝 15줄에서 막힘 낱말을 찾아 한 줄로 알린다.
  종료 5 시작 안 됨(원인 모름·신뢰 질문·터미널 끝남), 6 로그인·인증(`Login expired`·`/login` 등), 7 키체인, 8 시작 미확정. 터미널은 남기고, 마지막 JSON 줄에 `lane` 주소와 `start`(`ok`·`off`·`no_status`·`unknown`·`login`·`keychain`·`noturn`)가 있다.
  `hc get` 응답은 통째 검증한다(`internal/hcjson` — JSON 값 하나·주소가 띄운 주소·state·status 칸). 마감까지 응답을 확인 못 한 바퀴(조회 실패·잘림·status 칸 없음·다른 이름·not_found)가 있었고 끝남이 확인되지 않았으면 8이다 — 시작했을 수 있다.
  hc status를 내는 에이전트(Claude 등)용이다 — status를 내지 않는 명령·CLI는 `--start-wait 0`으로 띄운다.
  hc-fill은 5·6·7을 「상태 안 잡힘」으로 받아 그 터미널을 남긴 것으로 적고 그 기계를 빼고 다음 기계로 간다. 8은 미확정 — 그 기계 자리를 찬 것으로 보고 시도 기록(`?`)을 남겨 다른 기계에 또 띄우지 않는다.
- 첫 턴 확인(2026-11-20 launch-submit-check — 원격 워커의 자동 감사가 시작 훅 안내 뒤 첫 지시를 입력칸에 둔 채 idle로 멈춰 4분 뒤 지휘가 Enter를 쳤다.
  세션 기록에 사용자 줄이 그때까지 없었다): 첫 지시는 입력칸에 넣지 않고 **실행 인자**로 넘긴다(hc-fill 설정 `launch` 틀의 `{task}`, `--brief`의 `{brief}`, 직접 띄울 때 실행 명령의 위치 인자 — Claude 위치 인자 프롬프트, `hc send`로 나중에 넣지 않는다).
  그 길로 넘겼는데도 Claude가 스스로 제출하지 않은 드문 경합이라 감시는 **알림 전용**이다 — 놓치거나 늦게 알려도 띄우기에는 영향 없음(종료 코드·`lane` 주소 그대로, 키를 보내지 않고 화면도 읽지 않는다).
  status가 잡힌 뒤 `lane.first_turn_wait`초(기본 60, `0`이면 끔) 동안 `HC_LANE_POLL`초마다 `hc get`(마감까지 남은 초 시한)을 보고, turn > 0 또는 status `working`을 한 번이라도 보면 조용히 끝낸다(실측: 첫 턴 동안은 `working`·turn 0이고 그 턴이 끝나야 turn 1).
  조회 실패·빈 값은 다음 조회로 넘긴다.
  마감까지 못 보면 오류 출력 한 줄과 사건 `막힘` 「첫 턴 없음 <주소>」(원격은 `@<기계>`, 레인 이름·주인 지휘 `--to`) 하나, `start` `noturn`. hc-fill 대기열에는 첫 status에서 주소와 `start` `ok`를 돌려주고 따로 감시하며, 진단은 상태 폴더의 `hc-lane/watch.log`에 남긴다.
  사건을 못 적으면 진단 한 줄만 남긴다.
  `jq`가 없으면 건너뛴다.
- 작업별 임시 폴더: 실행 명령은 레인 환경(`env …`) 뒤 감싸개(`sh -c <jobtmp> jobtmp <뿌리> <레인> exec …`)를 거쳐 그 레인 전용 `TMPDIR`을 받는다(설정 `jobtmp.root`, `off`면 그대로 — docs/hc-sweep/job-tmp.md). 병합기 일꾼·워커 착지도 같은 감싸개(`run` 꼴)다.
- 다른 기계 레인(`--on`, `--read` 감사·hc-fill 원격 띄우기 포함)은 `hc run --label herdr.space=<id>`로 맥 Herdr 창의 **그 기계 공간**(이름 = 기계 이름, 기계마다 하나)에 탭으로 보인다.
  창에 그 기계의 `<기계>` 공간이 있으면 그 id(창이 보이는 `<기계>/` 접두는 뗀다), 없으면 고정 id `lanes`로 붙이고 창이 만든 공간에 기계 이름을 붙인다(`space.rename`).
  창(herdr-app)이 없으면 띄우기는 그대로 성공하고 알림 한 줄. 로컬 레인은 hc-fill이 `herdr-balance --add`(지휘 옆 분할)로 붙인다.
- 설정 훅(`.claude/herdr-setup.sh`·`herdr-teardown.sh`)은 hc 터미널 밖에서 부른 로컬(launchd hc-fill 등)이면 `hc exec --`(코어 문맥, `~/.zshrc`까지 읽음)로 돈다 — launchd가 띄운 bash는 `~/Documents`의 스크립트를 못 읽는다(TCC `Operation not permitted`).
  설정 훅이나 `hc run`이 실패해 레인을 안 띄우면 방금 만든 워크트리·브랜치를 지운다(고유 커밋이 없을 때만, 있으면 남기고 지우는 명령을 알린다) — 다음 시도가 「로컬 브랜치가 이미 있다」로 막히지 않게.
- 메인 폴더 앞당김(2026-11-18 — 워커 기계 메인 폴더가 이틀 동안 main보다 375~497커밋 뒤라 `--read` 감사가 옛 `CLAUDE.md`·계약·시험을 읽었다): 레인·`--read` 레인을 띄울 때 fetch 바로 뒤와 `hc-lane land` 착지 성공 뒤, 메인 폴더(`--repo`의 첫 작업 트리)가 기본 브랜치에 있고 추적 파일 변경과 진행 중인 merge·rebase·cherry-pick·revert와 skip-worktree·assume-unchanged 표시 파일이 없고, 그 기계에서 메인 폴더(아래, `<저장소>.wt/` 밖)를 cwd로 쓰는 살아 있는 working·blocked 터미널(부른 터미널 자신 빼고, `hc ls`로 봄 — 실패면 건너뜀)이 없으면 `git merge --ff-only --no-overwrite-ignore origin/<기본>`(쉬는 세션 밑에서는 앞당긴다).
  결과 한 줄 `hc-lane: 메인 폴더 앞당김 <a>→<b>`(또는 `최신 <a>`).
  못 하면(다른 가지·더러움·진행 중·일하는 세션·로컬에만 있는 커밋·추적 안 된 파일이나 무시된 파일과 부딪혀 git이 거절) 오류 출력에 `hc-lane: 경고 — 메인 폴더 앞당김 건너뜀: <까닭> — 그대로 둠` 한 줄만 남기고 레인·착지는 그대로 간다.
  `stash`·`reset`·`checkout --`·강제는 쓰지 않는다.
  원격이 없는 저장소는 착지가 이미 로컬 `<기본>`을 올리니 따로 하지 않는다.
  되돌리기: 그 폴더에서 `git merge --ff-only`의 반대는 `git reset --keep <a>`(사람이 할 때만).
- `fetch`: 맨 `ssh`나 hc 코어 환경에는 git 자격 도우미 PATH가 없어 「could not read Username」이 난다.
  `--on`이면 `hc exec`로 그 기계 zsh 로그인 셸(`~/.zprofile`)에 `~/.zshrc`까지 읽고 `git fetch origin [<참조…>]`(설정 단계와 같은 셸).
  참조는 하나라도 `-`로 시작하면 거절한다(git 옵션 주입).
