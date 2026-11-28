# hc-job-server (자동 루프를 Herdr 터미널 하나로 지키는 감독)

- `hc-job-server <이름> <job 라벨> <작업 폴더> <재개 스크립트>` — launchd(`KeepAlive`, `ThrottleInterval 60`)가 Go 바이너리를 바로 띄운다(`/bin/bash`를 앞에 두지 않는다).
  감독은 `~/Documents`에 닿지 않는다(재개 스크립트만 읽고 작업 폴더는 넘기기만 한다).
- 틱(`HC_JOB_TICK`, 60초)마다 `hc get <이름> --json`으로 판정한다.
  - `live`: 채택 — 아무것도 하지 않는다(감독을 `kickstart -k` 해도 루프 pid·주소 그대로).
  - `live`인데 **멈춘 루프**: `status.state`가 `done`·`idle`이고 `status_ms`가 `HC_JOB_STALE`초(기본 5400, 0이면 안 봄) 넘게 그대로면 `hc kill <주소>` 하고, 다음 틱에 `finished` 길로 같은 세션을 다시 띄운다.
    Claude `/loop`가 최대 시간(약 7일)에 닿으면 ScheduleWakeup이 거절돼 Claude는 프롬프트에 살아 있는데 회차가 없다(2026-11-23 D1 루프 11시간).
    ScheduleWakeup 상한이 3600초이고 매시 cron 루프도 한 시간 안에 다시 `working`이 되니 5400초면 도는 루프를 오판하지 않는다.
    끄기 전에 쉼 판정을 묻는다(`HC_JOB_HC` 곁의 `hc-sweep --rest <주소>` — [정말 쉬는가](../hc-sweep/busy-idle.md) 「한 문」): 배경 작업·하위 에이전트가 도는 루프는 멈춘 것이 아니라 안 끄고, 쉼을 모르면(옛 판 hc-sweep 등) 상태가 `HC_JOB_STALE`의 네 배(기본 6시간) 넘게 그대로일 때만 끈다. 남긴 까닭은 바뀔 때만 한 줄.
    `working`·`blocked`·`error`와 status가 없는 터미널(Claude가 아님, 권한을 기다리는 `hc-loop-start`)은 건드리지 않는다.
  - `finished`: 끝난 기록만 `hc rm` 뒤 `absent`와 같이.
  - `absent`: `hc run --bg --outlive-parent --no-parent --name <이름> --label job=<라벨> -- /bin/bash <hc-loop-start> <작업 폴더> <재개 스크립트>`.
  - 그 밖(코어에 못 붙음·모르는 상태): 띄우지 않는다(없음으로 오판하면 중복이 생긴다).
- 재개 스크립트에서 처음 나오는 `--resume <ID>`의 Claude가 hc 밖에서 돌면 띄우지 않는다.
- 다시 띄우기(멈춘 루프 끄기 포함)는 `HC_JOB_GAP`(600초)에 한 번까지.
- 환경 변수: `HC_JOB_HC`(hc, 기본 `~/.local/bin/hc`), `HC_JOB_LOOP_START`(기본 `~/.local/bin/hc-loop-start` — 작업 폴더 권한을 기다렸다가 재개 스크립트를 exec하는 기계 쪽 스크립트), `HC_JOB_STALE`·`HC_JOB_GAP`·`HC_JOB_TICK`, `HC_JOB_ONCE`(한 틱만).
- 루프를 일부러 멈추려면 `launchctl bootout gui/$(id -u)/<라벨>` 뒤 `hc kill <이름>`.
- 시험 `cmd/hc-job-server/main_test.go`(가짜 hc로 한 틱).
