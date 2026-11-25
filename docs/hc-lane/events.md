# hc-lane 사건 (report·events·wait — 레인이 적고 지휘는 그것만 읽는다)

화면 글자나 보고 파일 끝 줄을 긁는 감시는 칸이 한 줄이거나 파일 이름이 다르면 완료를 놓친다(2026-11-16 레인이 20분~1시간 20분 놀았다).
그래서 레인이 상태를 사건 파일에 직접 적고, 지휘는 그 파일만 읽는다.

- 레인: `hc-lane report <중간|완료|착지|막힘|판정|제출> [<sha>|-] [<한 줄>] [--to <지휘>,…]`. `--to`는 받을 지휘를 아홉째 칸에 적는다 — 그 지휘들의 watch만 보낸다(레인 주인 판정 대신. hc-fill 놀고 있음 알림이 쓴다).
  sha 확인(2026-11-22 lane-report-sha — 짧은 해시 오타가 그대로 받아져 자동 감사가 몇 분 뒤 막혔다): 완료·중간의 sha는 git 작업 트리 안에서 `git rev-parse --verify <sha>^{commit}`으로 풀어 전체 해시로 적는다.
  못 풀면(없음·여럿에 걸림) 사건을 쓰지 않고 「<sha>는 이 작업 트리에 없다 — HEAD는 <전체>」와 종료 2, 작업 트리 밖(도구가 `--name`으로 적는 사건)은 옛 꼴 그대로다.
  sha 자리에 `HEAD`를 주면 어느 상태든 이 작업 트리의 HEAD 전체 해시로 적고(밖이면 종료 2), 완료 sha가 HEAD가 아니면 경고 한 줄만 낸다(막지 않음 — 「그 뒤 커밋 n개」).
  hc-fill 「감사·착지」 부록과 `--print-steps`의 완료 보고 줄은 `hc-lane report 완료 HEAD …` 꼴이다.
  `제출`은 `hc-lane land`만 적는다(12칸 키 `submit:<제출 ID>`, ID 꼴 `<기계>-<UTC yyyymmddThhmmss>-<pid>`) — 그 키가 없거나 ID 꼴이 아닌 report 제출은 사건을 쓰지 않고 「제출은 hc-lane land가 적는다 …」와 종료 2(2026-11-23 fix-inbox-submit-id — 레인이 손으로 적은 `report 제출 감사 완료 …`의 첫 낱말 「감사」가 제출 ID로 읽혀 받은편지 회신이 이틀 멈췄다).
  읽는 쪽(hc-fill 회신·마지막 보고·겹침, `hc-lane stats`)도 제출 ID는 `events.SubmitID` 하나로 읽는다: 12칸 `submit:<ID>`를 먼저, 키 없는 옛 줄은 한 줄 칸 첫 낱말이 ID 꼴일 때만. ID 없는 `제출`은 제출로 치지 않고 그 앞의 진짜 제출을 본다(없으면 병합기 결과를 기다리지 않는다).
  그 기계 `${XDG_STATE_HOME:-~/.local/state}/hc-lane/events.tsv`에 한 줄을 더한다(지우지 않음).
  칸: 시각(UTC `2026-11-16T02:00:00Z`)·기계(`HC_LANE_MACHINE`, 없으면 hostname)·터미널(`$HC_TERMINAL_ID`)·레인·상태·sha·한 줄(탭·줄바꿈은 빈칸)·그때의 hc turn(`hc get`, 터미널 밖이면 `-`. 옛 7칸 줄도 읽는다).
  13칸 저장소(2026-11-24 watch-project-route): `--name` 없이 부르면 지금 폴더 저장소의 origin 원문(`git config remote.origin.url` — insteadOf를 풀지 않음)을 정규화한 「호스트/경로」(소문자, 사용자·포트·끝 `.git` 뗌 — scp 꼴·ssh·https가 같은 값)를 더 싣는다.
  클론 경로는 기계마다 달라 쓰지 않는다. 로컬 경로·`file:` 원격·원격 없음은 모름이라 지금 꼴 12칸 그대로, 1000바이트에 모자라면 한 줄 칸을 줄여 싣는다.
  `--name`(hc-fill·hc-sweep·병합기처럼 다른 폴더에서 남의 레인을 적는 길)은 싣지 않고, `hc-lane land`의 제출만 그 저장소 안에서 적으니 싣는다(`HC_LANE_REPO_ID`).
  14칸 종류(2026-11-24 ops-events): 운영 사건(레인 칸이 도구 이름 — [운영 사건 목록](../ops/events.md))은 `ops`를 싣는다(13칸이 없으면 `-`로 채움). `--kind ops|lane`으로 정하고, 없으면 그 목록의 이름으로 정한다.
  레인 사건은 싣지 않아 줄·열쇠가 지금 꼴 그대로다. 14칸이 없는 옛 줄도 그 목록의 이름으로 운영을 가른다(`events.KindOf`). 1000바이트에 모자라면 한 줄 칸을 줄여 종류 칸을 지킨다.
  옛 판 판독기는 13칸을 안 읽고(앞 12칸 그대로), 새 판은 12칸 줄을 「저장소 모름」으로 읽는다 — 받는 지휘는 [watch](watch.md) 「사건은 그 레인을 띄운 지휘에게만」.
  사건 줄의 형은 `internal/events` 한 곳이다(2026-11-24 events-named-cols): 읽기는 `events.Cols`(칸 수 상관없이, 없는 칸은 빈 값, `N`=칸 수)의 이름 칸, 쓰기는 `Line`·`Record`(13·14칸)·`LineN`, 읽은 줄을 고쳐 내보낼 때는 `Raw`(바이트 그대로 — 열쇠가 같다). 칸 번호로 읽지 않는다(`internal/events` 시험 `TestNoNumberedEventColumns`).
  레인 이름은 `--name` → `$HC_LANE` → hc 터미널 이름(`hc get`) → 작업 트리 가지(기본 가지면 `-` — 작업 트리를 지운 뒤 메인 폴더에서 report해도 `main`이 레인이 되지 않게).
  화면에도 `<레인> <상태>: <sha> <한 줄>`을 찍는다.
  `--print-steps`가 쓰기 레인에는 중간·완료·착지·막힘 때, `--read` 레인에는 판정 때(`hc-lane report 판정 - "<판정 한 줄>"`) 부르라는 줄을 넣는다.
- hc-lane이 띄우는 레인은 `HC_LANE=<이름>` 환경과 hc 라벨 `hc-lane=1`을 받는다.
- 지휘: `hc-lane events [--all|--on <기계>…] [--since <시각|30m·2h·1d>]`는 이 기계(와 연결된 기계마다 `hc exec --on` 한 번)의 사건을 시각 순으로 찍는다.
  원격 줄의 기계·터미널은 hc 이름(`<기계>/<t…>`)이다.
  못 읽은 기계는 `모름 <기계>`. sh를 띄울 수 없는 기계(예: Windows — `hc exec`가 `spawn_failed`·`cannot start sh`)는 레인 기록이 없는 것으로 보고 건너뛴다(events·wait·watch 모두 `모름`·`연결 안 됨` 없음, 쉬는 레인 ps 확인도 같다).
- 지휘: `hc-lane wait [--all] [--timeout <분>(기본 110)] [--state 완료,착지,…]`를 배경으로 건다.
  아직 본 적 없는 사건이 생기면 그 줄(들)을 찍고 종료 0, 상한이면 75. 찍은 사건은 이 기계 `…/hc-lane/seen.<$HC_TERMINAL_ID>`(지휘마다 따로, 터미널 밖이면 `seen`)에 적어 다시 알리지 않는다(바꾸는 것은 이 파일뿐).
  출력이 `/dev/null`이면 읽을 사람이 없어 seen에 적지 않고 종료 2(배경으로 걸 때는 출력을 파일로 받는다).
  새 커서는 `--since`(없으면 2시간 전)부터 본다 — 레인을 띄운 뒤 2시간 안에 첫 wait를 건다.
  라벨 `hc-lane=1` 터미널은 그 터미널(주소·레인 이름)의 마지막 사건으로 본다: 착지·판정 그 턴(report turn+1 이하, report turn을 모르면 밖) 밖에서 끝났거나(한 줄 칸 `뜻밖의 종료(<상태>)` — 착지 전인 완료·막힘 뒤와, 착지·판정 뒤 이어 받은 일에서 끝난 것도 알린다.
  착지 뒤에 같은 sha의 완료가 같은 턴(같은 turn 값)에만 이어지면 — 착지 뒤 완료를 한 번 더 적는 레인 틀 — 착지로 치고 turn은 그 완료의 것으로 본다.
  sha가 `-`(판정·sha 없음)이거나, 사이에 turn이 올랐거나, 다른 sha의 완료·중간이 뒤에 오면 새 일. sha는 소문자로 짧은 쪽 길이만큼 앞자리를 견주고, report는 대문자 sha도 소문자로 적는다), 완료·착지·막힘·판정 없이(또는 그 report의 turn+1보다 뒤에 — 이어 받은 일을 report 없이 끝냄) done·idle로 쉬거나, blocked·error면 `멈춤` 사건(한 줄 칸 `<상태> turn <n>` — turn이 바뀌면 다시 알리고, 같은 turn의 done·idle은 한 번).
  done·idle은 쉰 지 `lane.wait_grace`분(기본 10, `0`이면 끔)이 지나고 배경 작업이 없을 때 멈춤이다.
  배경 작업은 프로세스로 본다: `hc get`의 터미널 `pid` 자손 가운데 명령 줄에 `lane.wait_procs` 글자(빈칸으로 나눈 목록, 기본 `shell-snapshots` — Claude Code Bash 도구 셸 `/bin/zsh -c source …/.claude/shell-snapshots/snapshot-zsh-….sh …`)가 든 것이 살아 있으면 배경 작업 중(MCP 서버처럼 늘 사는 자손은 그 꼴이 아니라 안 셈).
  이 기계는 `ps`, 원격은 바퀴마다 기계당 `hc exec --on <기계> -- ps` 한 번. 그런 프로세스가 없거나(셸 없는 배경 Agent 등) pid를 모르거나 ps가 실패하거나 `lane.wait_procs`가 빈 값이면 화면 표시도 본다: 화면의 마지막 상태 줄(`lane.wait_line`, 기본 ✻ 턴 끝 줄 — 옛 턴의 줄·본문은 안 봄)에 `lane.wait_busy`(확장 정규식, 기본 `shells? still running|monitors? still running`, 빈 값이면 끔) 표시가 없을 때 멈춤 — 배경 셸이 도는 동안 턴을 끝낸 레인은 스스로 이어 가니 알리지 않는다.
  배경 작업이 있어도 쉰 지 `lane.wait_busy_max`분(기본 30)이 넘으면 멈춤(blocked·error·끝남은 바로).
  그 바퀴에 events를 못 읽은 기계의 레인은 blocked·error만 그대로 알리고, 사건이 있어야 하는 쉼·끝남은 `연결 안 됨 <기계> <날짜>`(레인마다 하루 한 번)로 알린다 — 다음 바퀴에 다시 읽히면 정상 판정. 줄은 1000바이트 안으로 잘라(끝 반쪽 UTF-8 버림) 여러 레인이 함께 더해도 섞이지 않는다.
  바퀴마다(`HC_LANE_WAIT_TICK`초, 기본 60) `hc machine ls`·`hc ls` 한 번과 기계당 `hc exec` 한 번.
- 정리됨: `hc-sweep`이 레인 터미널(라벨 `hc-lane=1`)을 닫을 때 닫기 전에 그 기계 events.tsv에 `정리됨` 한 줄(한 줄 칸 `hc-sweep <규칙>`)을 적는다(`hc-lane events`로 보인다).
  그 줄을 못 쓰면 닫지 않는다(닫으면 wait가 reason killed로 보고 조용히 넘긴다).
  그 줄도 report와 같은 길(`internal/events`)로 쓴다 — 기계·터미널·레인 64바이트, 줄 1000바이트, 지킬 수 없으면 못 씀(2026-11-24 events-named-cols).
  hc-sweep은 닫은 뒤 `hc rm`까지 해 터미널이 목록에서 사라지므로, wait는 이 줄로 판정한다: 그 터미널의 바로 앞 report가 착지·판정이고 정리됨 turn이 그 turn+1 안이면(또는 같은 turn의 쉼으로 이미 알렸으면) 버리고, 아니면 `멈춤`(한 줄 칸 `뜻밖의 종료(정리됨) hc-sweep <규칙>`, 바로 앞 report가 제출이면 정상 정리라 `제출 뒤 정리(정리됨) hc-sweep <규칙>` — 2026-11-21. 바로 앞 report가 제출·주차면 turn과 상관없이 아무것도 안 보내고, 끝남·done·idle 쉼도 같다 — 2026-11-22 go-alert-quiet) — wait가 안 돌던 틈에 닫힌 보고 없는 레인도 다음 wait에서 나온다.
  쉬는 레인 판정에서는 정리됨이 마지막 report를 가리지 않는다.
  제출·주차 뒤에 적은 `중간`(「병합기에 제출함 … clean 멈춤」 같은 덧말)은 마지막 report를 바꾸지 않는다 — 넘긴 레인은 그 뒤 쉬거나 닫히거나 정리돼도 조용하고, 다시 완료·막힘·착지·판정을 적으면 그 report로 판정한다(2026-11-23 stall-after-submit-note — 제출 뒤 중간 한 줄 때문에 착지 중·착지 뒤 「멈춤 done」이 턴마다 갔다).
  제출·주차·착지 바로 뒤 `막힘`(clean 멈춤 등)에 이은 `중간`은 마지막 report를 그 막힘 앞 상태로 되돌린다 — `hc-lane clean`은 레인 터미널이 자기 가지를 정리해 끝냈는데 그 터미널의 마지막 report가 막힘이면 `중간 - 막힘 풀림: hc-lane clean 성공 — …`을 적어, hc-fill 단계(last.tsv)·advise·쉬는 레인 판정이 막힘을 놓는다(2026-11-25 ht-clean-unstick — clean이 풀린 뒤에도 「막힘: clean 멈춤」이 남아 운영 에이전트가 사람에게 다시 올렸다). 넘김 없는 막힘 뒤 중간은 다시 일하는 중이라 그대로 중간이다.
- 마지막 보고(2026-11-17): 착지·판정에 더해 `--read`·hc-fill 레인은 `완료`도 마지막 보고다(레인 기록 `lanes.tsv` 넷째 칸 `read`·`fill` — hc-lane이 띄울 때 적는다.
  구현 레인은 3칸 그대로라 `완료` 뒤 감사를 기다리다 혼자 끝나면 여전히 `뜻밖의 종료`).
  지휘가 `hc kill`로 닫은 레인(`hc ls`의 `reason: killed`)의 끝남은 알리지 않는다 — parent가 없거나(hc-fill 레인, hc-fill이 `HC_LANE_FILL=1`로 띄움) parent가 아직 살아 있을 때만이다.
  지휘 칸이 죽어 함께 끝난 레인(die_with_parent도 `killed`)은 그대로 알린다.
  hc는 원격 레인의 parent를 묻는 쪽 기준 주소로 준다(레인 기계에선 `<지휘 기계>/t7`, 지휘 기계에선 `t7` — 2026-11-17 실측)라 parent는 그대로 이 기계에서 묻고, 그 주소의 기계를 모를 때(machine_unknown — 바꿔 주지 않은 `<이 기계>/t…` 꼴)만 앞부분을 떼고 다시 묻는다.
  조용히 넘긴 killed는 커서에 그 멈춤 열쇠로 적어 지휘가 끝난 뒤에도 다시 오지 않는다.
  쉬는 레인은 그 작업 트리 경로가 명령 줄에 든 프로세스(떨어져 나간 배경 검증 등)가 살아 있으면 바쁨으로 본다(`lane.wait_busy_max`분까지).
- 열쇠에 기계 이름 없음(2026-11-17): `seen.*`·`sent`의 열쇠는 기계 칸을 `*`로, 주소는 끝의 `t…`만 쓴다.
  hc 기계 이름을 바꿔도(`old-name` → `new-name`) 본 사건이 다시 나오지 않고, 옛 판이 적은 줄(기계 이름이 든 그대로)도 같은 사건으로 읽는다.
- 지휘별 거름(`--mine`, 기본 꺼짐): 레인의 hc parent가 부른 터미널(`$HC_TERMINAL_ID` — 원격 레인의 parent는 보통 앞부분 없는 꼴이고, `<이 기계>/<t…>` 꼴도 끝으로 맞춤)인 사건만 찍는다.
  parent를 모르면(`hc ls`에 없음 — 지워진 터미널 등) 보여 준다.
  한 기계에 지휘가 여럿일 때만 쓴다 — 지휘가 바뀌었거나(옛 레인의 parent는 죽은 옛 지휘) wait를 따로 `hc run`으로 감싸 다른 터미널에서 부르면 사건이 다 가려진다.
  기본은 전부 찍는다(섞이는 것은 거짓 알림 쪽, 가려지는 것은 놓치는 쪽).
- 레인 이름 거름(`--lane <레인>`, 여럿이면 되풀이 — events·wait, 2026-11-24 cli-usage-version): 그 레인 이름(넷째 칸)의 사건만 찍는다.
  레인 하나가 `hc-lane events --name <레인>`을 20초마다 되풀이해 운영 기록 116줄 가운데 81줄이 「알 수 없는 인자」였던 뒤에 더했다.
  wait에서 거른 사건은 seen에 적지 않아 거름 없는 wait가 그대로 받는다. 「모름 <기계>」와 `hc ls` 실패 사건은 거르지 않는다(모름은 모름).
  watch에는 없다(지휘가 받을 사건을 줄이면 놓친다).
- 종류 거름(`--kind ops|lane` — events·wait, 2026-11-24 ops-events): 운영 사건만, 또는 레인 사건만 찍는다(14칸, 없으면 [운영 사건 목록](../ops/events.md)의 이름으로 — `events.KindOf`). `--lane`과 함께면 둘 다 맞아야 한다.
  wait에서 거른 사건은 `--lane`처럼 seen에 적지 않는다. watch에는 없다(대신 운영 사건 글 머리에 `운영: `을 붙인다 — [watch](watch.md)).
  모르는 인자가 아는 인자와 가까우면(편집 거리 2 이하) 오류 둘째 줄에 권한다 — `--name`이면 `hc-lane: --name 대신 --lane?`(첫 줄·종료 코드는 그대로).
- `hc ls`가 3바퀴 이어 실패하면 wait는 `멈춤` 한 줄(`hc ls 실패 n바퀴(<시각>부터) — 레인 멈춤·끝남을 못 봄`)로 끝낸다.
- `lane.notify no`(기본 `yes`): hc-lane이 띄우는 레인·감사(`--read` 포함)에 `hc run --no-notify`를 붙여, hc가 지휘 입력에 넣는 종료·막힘 알림(`<hc>… exited (signal 1)` — hc-sweep이 일 끝난 레인을 닫을 때마다 온다)을 끈다.
  그러면 막힘·오류·뜻밖의 종료는 `hc-lane wait`만 알리므로 **지휘에게 `hc-lane watch`가 돌 때만**(레인을 띄우면 저절로 뜬다) `no`로 둔다.
- 대기열 레인은 부모 없이: hc-fill이 `HC_LANE_FILL=1`로 띄우는 레인·감사 줄(`--on` 포함)에 `hc run --no-parent`를 붙인다 — 대기열이 주인이다.
  hc run 기본값(부모 = 부른 터미널, 부모가 끝나면 함께 끝남)대로면 지휘 터미널에서 돈 `hc-fill add`·`run`이 띄운 레인이 지휘 세션이 닫힐 때(업데이트 재시작 등) 미커밋 일을 둔 채 꺼졌다(2026-11-19 idle-once 레인).
  지휘가 `hc-lane`으로 직접 띄우는 레인은 그대로다.
- 주의: hc도 `XDG_STATE_HOME`으로 코어 소켓을 찾는다.
  사건 폴더를 옮기려고 그 값을 바꾸면 hc가 빈 코어를 따로 띄워 `hc ls`가 비고(멈춤을 못 봄), 경로가 길면 소켓 길이 제한으로 실패한다.
