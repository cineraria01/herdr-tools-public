# 고아 프로세스 (레인이 남긴 배경 프로세스)

- **검증에서 막기**: `.claude/herdr-verify.sh`는 단계(go 명령)마다 따로 프로세스 무리(pgid)로 돌린다.
  단계가 끝난 뒤 그 무리에 남은 프로세스, 또는 검증 중 새로 생겨 부모가 1이 된 프로세스 가운데 명령 줄·cwd에 이 폴더 경로가 든 것(과 자손)은 고아다.
  목록(pid·cwd·명령)을 찍고 끄고(TERM, 3초 뒤 KILL) 검증 실패. 단 cwd로만 걸리고 명령 줄에 `/.claude/`가 든 것은 뺀다 — 레인은 같은 작업 트리에서 Claude 세션이 돌며 검증을 부르므로, 세션의 훅·플러그인(예: OMC node)이 떼어 띄운 프로세스가 cwd로 걸렸다(2026-11-19).
  명령 줄에 이 폴더 경로가 들면 `/.claude/`가 있어도 잡는다.
  고아 줄의 cwd로 이 폴더의 다른 세션 소속인지 사람이 가린다.
- **늘 쓸기**: `hc-sweep`(launchd 5분마다)이 끝난 터미널을 닫은 뒤, 이 사용자의 PPID 1 프로세스 가운데 명령 줄의 절대 경로나 cwd가 레인 작업 트리(`<저장소><접미사>/<레인>`) 아래이고 `--min-age`분(기본 10) 넘은 것을 본다.
  - 남김: 그 작업 트리 아래를 cwd로 쓰는 live hc 터미널이 있음(실제 경로로 견줌).
  - 보류(찍기만): 작업 트리가 있는데 본 저장소 아래를 쓰는 터미널이 있음(감사 스냅숏 꼴), 작업 트리가 있는데 cwd로만 걸림, 자손에 live 터미널 프로세스가 있음.
  - 끔: 그 밖(작업 트리가 지워졌거나 쓰는 터미널 없음).
    그것과 자손에 TERM, 5초 뒤 KILL. pid·나이·명령 앞 80자를 `~/.claude-tasks/closed.log`에.
  - 터미널 닫기 R2(일 끝난 worker)는 parent가 있거나 라벨 `hc-lane=1`인 것 — hc-fill이 띄운 레인은 parent가 없다.
    이 기계 hc-fill 대기열에 결과를 아직 되돌려 주지 않은(`up`) 받은편지 레인은 닫지 않는다.
  - 착지한 레인의 교착 풀기(2026-11-21 mini-m6-1 land-metrics 16시간 — hc-sweep은 남은 작업 트리 때문에 터미널을, `hc-lane gc`는 그 폴더를 쓰는 터미널 때문에 작업 트리를 남겨 서로 기다렸다): worker가 링크된 작업 트리에서 쉰 지 30분(또는 제출 뒤)이고, 그 레인이 착지했고(`git cherry origin/main HEAD`에 `+` 없음 + 착지 증거 — 이 레인 이름·이 실행의 hc-lane 사건 착지·제출·완료·판정(실행 키 칸 `<주소>@<created_ms>`가 같은 것, 키 없는 옛 줄은 같은 주소이고 이 터미널이 생긴 뒤 것), 없으면 이 터미널이 생긴 뒤의 원격 `refs/hc-landed/<가지>/<초>` — 같은 이름의 옛 레인 증거는 안 친다) 작업 트리가 깨끗하면(`status --porcelain --untracked-files=all` 빔) 보호를 풀고 R2로 닫은 뒤 `hc-lane clean --repo <메인 저장소> --branch <가지>`를 hc exec로 부른다(결과는 closed.log 「작업 트리 정리」).
    git은 모두 hc exec(TCC), 모름이면 남김.
  - 분리·깨끗한 읽기 전용 작업 트리(2026-11-23 seat-clean-detached — 감사 스크립트가 `git worktree add --detach`로 직접 만든 작업 트리의 done 터미널 여섯이 다섯 시간 자리를 붙잡았다): 링크된 작업 트리가 분리 HEAD이고 변경 0(`status --porcelain --untracked-files=all` 빔)이고 HEAD가 어느 가지(로컬·원격)에든 들었으면(`rev-list -n 1 HEAD --not --branches --remotes` 빔) 쉰 지 30분 뒤 worker가 아니어도 R2(keep 목록·배경 실행 흔적 보호 그대로)로 닫고, 다시 확인한 뒤 `git worktree remove`(--force 없이 — 그사이 변경이 생겼으면 git이 거절)로 지운다(closed.log 「작업 트리 정리(분리·깨끗…)」). 가지 위·git 실패는 위 규칙.
    - 닫기 전과 지우기 직전에 `hc ls`로 그 폴더(또는 그 아래 — 심볼릭 링크를 풀고 경계까지, `/a/wt`가 `/a/wt2`에 안 걸림, hc-lane busyTerms와 같은 `hcjson.Under`)를 cwd로 쓰는 이 기계의 다른 live 터미널이 있거나, `hc ls`가 실패하거나 이 기계 모름 항목이 있으면 닫지도 지우지도 않는다(「작업 트리 남음 — …」. 2026-11-23 seat-clean-fix — 끝난 감사 터미널 여섯을 닫으며 같은 폴더를 쓰던 확인 세션들의 작업 트리를 지웠다).
  - 끝난 keep 셸 레인(2026-11-22 sweep-landed-keep — mini-m4 land-claim-net이 착지·정리 뒤 13시간 자리를 붙잡았다): Herdr 창에 붙인 터미널은 `hc keep`이 켜져 에이전트가 끝나면 셸로 바뀌고 status 없이 live로 남는다.
    상태 없는 셸 칸도 목록에 넣어 `hc get`으로 보고, keep이 켜졌고 전경이 셸이며 hc-lane 레인(R2와 같은 worker 판정)이고 작업 트리가 없고 이 실행의 마지막 hc-lane 사건(정리됨 빼고)이 제출·착지면 「에이전트 상태 없음」 보호를 풀어 R2로 닫는다.
    사람 keep 목록(`~/.claude-tasks/keep`의 이름·주소)·이름 보호·착지 전 커밋은 그대로 막고, 작업 트리가 남았거나 모르면 위 규칙을 따른다.
    레인 자신도 마지막 `hc-lane clean`(자기 레인 — `HC_LANE`이 그 가지·이름)에서 자기 터미널(`HC_TERMINAL_ID`)의 keep을 끈다(`hc keep <주소> --off`, 정리할 것 없음이어도 — 다른 레인 정리·dry-run은 안 건드림, 실패는 알리기만).
  - `hc-sweep --dry-run`은 찾기만 — 남기는 터미널마다 「남김: <주소> <이름> — <까닭>」 한 줄. hc ls 실패·terminals 목록 아님·설정 읽기 실패·cwd 모르는 이 기계 터미널이면 아무것도 끄지 않는다.
  - **구현**: `cmd/hc-sweep`. 외부 명령(hc·git·ps·lsof·sqlite3)은 쓸기와 같은 프로세스 무리로 띄우고(쓸기 무리를 끄면 함께 끝난다) 상한 시간에 그 자식을 끈다.
    잠금 `<상태>/hc-sweep/sweep.lock`(flock — 남이 쥐면 75, `--dry-run`은 안 잡음).
    모름은 닫지 않는다(2026-11-21 Go 전환 감사): 닫기 직전 `hc get`으로 같은 실행·같은 상태인지 다시 보고, kill 뒤 `hc get`이 not_found가 아닌 실패면 「모름」(작업 트리 정리 안 함), keep을 못 읽으면 그 바퀴 닫지 않음, 빈 화면·폴더가 있는데 실패한 `git cherry`는 모름, 고아는 시작 시각까지 견주고 기록에 명령 인자를 남기지 않음, lsof·ps 출력이 온전하지 않으면 그 정리를 건너뜀, 고아 판정 실패는 종료 1. 색인 DB는 `sqlite3` 명령으로 읽는다(`PRAGMA query_only` — macOS sqlite3는 `-shm` 없는 WAL DB를 `-readonly`로 못 연다).
    계약 시험 `tests/util/hcsweep*_test.go`.
- 설정 키: `sweep.wt_suffix`(기본 `.wt`), `sweep.orphan_keep`(보지 않을 실행 파일 이름 — `ps comm`(argv[0])으로 견주어 `…/Herdr Core.app/…/herdr-core`처럼 공백 든 경로도 맞음, 기본 `codebase-memory-mcp herdr-core`), `sweep.orphan_kill`(`on`·`off`, 기본 `off` = 찍고 closed.log에 pid·명령마다 「고아 관찰」 한 줄 — 기록을 며칠 보고 `on`).
  설정 도구(`herdr-tools-config`, sync.list `local`)가 곁에 없는 기계는 늘 기본값이라 off(관찰만)다.
  켜려면 그 기계에 도구가 있어야 한다.
- 레인 지시문(`hc-lane --print-steps`) 끝 단계에 「끝내기 전 `pgrep -fl <작업 트리>/`로 확인, 자기 셸·에이전트는 빼고 정확한 pid로 kill」 줄이 붙는다.
- 쓰기·읽기 레인 지시문(`--print-steps`, `--read` 포함 — hc-fill run 지시문도 이것을 쓴다) 머리에 「임시 파일은 `mktemp -d` 새 폴더, `rm -rf`·`rm -f`에 변수·와일드카드 경로 금지(꼭 지우면 글자 그대로나 `"${X:?}"`)」 줄이 붙는다.
  그런 rm은 Claude Code가 권한 창을 띄워 무인 레인을 멈춘다.
