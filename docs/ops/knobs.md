# 손잡이 (hc-fill knob — 실행 중에 바로 먹는 조정)

- 사람과 지휘 에이전트가 같은 명령으로 「지금 이 값만 잠깐 바꾸기」를 한다(2026-11-24 ops-knobs, 방향은 [설계안](agent-managed-design.md) 머리).
- 왜: 설정 `hc-fill.tsv`는 개인 git 저장소 안이라 고치면 저장소가 더러워져 herdr-sync가 멈추고, 병합기 값은 시작 때 읽는 환경 변수라 다시 띄워야 바뀌었다.
- 저장은 지휘 기계의 git 밖 파일 `~/.local/state/herdr-ops/knobs.tsv`(줄 = 이름·값·누가·왜·언제·언제까지·고정)이고, 개인 설정 파일은 건드리지 않는다.

## 쓰는 법

- `hc-fill knob ls [--json]` — 손잡이마다 지금 먹는 값·출처(`기본`/`설정`/`손잡이`)·누가·왜·언제까지·고정·허용 범위·설명·읽는 도구.
- `hc-fill knob set <이름> <값> --why <까닭> [--for <분> | --until <시각>] [--pin] [--force]`
  - `--why`는 꼭 받는다(빈 글은 거절).
  - 누가 = 부른 터미널 주소(`HC_TERMINAL_ID`), 없으면 `사람`.
  - `--until` 꼴은 RFC3339 또는 `2006-01-02 15:04`(이 기계 시간대), 지난 시각은 거절.
- `hc-fill knob unset <이름> [--force] [--why <까닭>]` — 손잡이를 지워 설정 값으로 돌린다.
- set·unset은 [운영 일지](journal.md)에 한 줄씩 남는다(되돌리는 명령과 함께).
- 거절(종료 1): 등록 표에 없는 이름(가까운 이름을 권한다), 범위 밖 값, `--why` 없음, 고정된 값을 `--force` 없이 바꾸거나 지우기. 인자 꼴이 틀리면 사용법(종료 2).

## 우선순위·기한·고정

- 우선순위: 손잡이 > 설정 파일 > 기본값.
- 바뀐 값은 재시작 없이 다음 바퀴에 먹는다(hc-fill 바퀴는 바퀴마다 새 프로세스라 저장 파일을 새로 읽는다).
- 기한(`--for`·`--until`)이 지나면 그 손잡이는 저절로 없는 것이 되고 설정 값으로 돌아간다. 지난 줄은 다음 쓰기 때 파일에서 걷힌다.
- 고정(`--pin`): 사람이 못 박은 값을 에이전트가 무심코 뒤집지 않게, `--force` 없이는 바꾸거나 지우지 못한다. 고정도 기한이 있으면 그때 풀린다.
- 저장 파일이 깨졌거나 못 읽으면 손잡이가 없는 것으로(설정 값으로) 돌고 표준 오류에 한 줄 알린다. 깨진 파일 위에는 쓰지 않는다 — 사람이 보고 고치거나 지운다.
- 저장 파일에 손으로 적은 줄이라도 등록 표에 없는 이름·범위 밖 값은 먹지 않는다.

## 등록 표 (무엇이 손잡이인가)

- 손잡이가 될 수 있는 값은 `internal/knob`의 등록 표 `Defs`(이름·기본·범위·단위·설명·읽는 도구·원격 여부)에 있는 것뿐이다.
- 새 손잡이는 그 표에 한 줄 더하고 도구는 한 함수로 읽는다.
  - hc-fill: 표의 이름이 설정 키와 같으면 `conf(<키>)`가 손잡이를 먼저 본다 — 읽는 코드는 그대로다.
  - 다른 기계의 도구: `knob.ReadRemote`(아래)로 읽고 `Remote.Get(<이름>)`.
- 지금 표의 hc-fill 설정 키: `idle`·`hold`·`done_stall`·`wake_every`·`ack_max`(분), `early_wait`(시간). 뜻은 [설정](../hc-fill/config.md)과 같다.
  부하 보고 고르기([부하](../hc-fill/load-steer.md)): `load_band`·`load_busy`(%), `load_starve`·`skew_min`·`skew_every`(분).
  기계 닿지 않음([끊김](../hc-fill/unreach.md)): `unreach`(분).
  멈춘 레인 다시 굴리기([nudge](../hc-fill/edit.md)): `nudge_every`(분, 한 레인에 다시 보내기까지, 0 막지 않음).
  상태 한 장의 눈에 띄는 것([상태](state.md)): `state_stage`(분), `state_hot`·`state_cool`(%). 기본 출력이 끝난 레인을 남기는 시간: `state_done`(분).
  지금 손댈 것([advise](advise.md)): `advise_wait`·`advise_merge_slow`·`advise_every`(분, 0 끔) — 풀 수 있는 대기·사슬의 문턱은 `idle`과 같다. 밀린 배포의 착지 뒤 유예 `deploy_grace`(분, 기본 5). 착지 뒤 명령의 종료 75 다시 돌리기 상한 `onland_retry_wait`(분, 기본 10, 0 끔 — [착지 뒤 명령](../hc-fill/on-land.md)).
  운영 에이전트([agent](agent.md)): `ops_agent`(끔|판단만|조치, 기본 끔)·`ops_agent_max_hour`(번)·`ops_agent_timeout`(분)·`ops_agent_tries`(번)·`ops_agent_scope`(프로젝트)·`ops_agent_close`(0·1, 기본 1) — 운영 에이전트 실행 안에서는 이 여섯을 바꾸지 못한다.
- 옮긴 hc-fill 설정 키(knobs-migrate, 2026-11-24 — 기본은 지금 코드 값 그대로): `gc`·`idle_empty`·`reland_wait`(분), `reland_max`·`audit_max`(번),
  `audit_close`·`audit_seat_rounds`(바퀴), `lows_hour`·`lows_min`(개).
- 병합기 살아 있음(hc-fill, merger-alive): `merger_dead`·`merger_unknown`(분, 0 끔), `merger_restart`(`0`|`1`).
  옛 표식 파일 `<hc-fill 상태 폴더>/merger_dead`·`merger_unknown`·`merger_restart`는 손잡이가 없을 때 그대로 먹고, `knob ls`에 출처 `설정`으로 보인다.
- 목록 손잡이 `auto_submit`(프로젝트 이름을 쉼표로, `-` = 끔)과 그 시간 상한 `auto_submit_max`(초) — [자동 감사](../hc-fill/audit.md) 「대기열이 제출」.
- 목록 손잡이 `pr`(PR 거울을 켤 프로젝트, `-` = 끔)과 `pr_max`(바퀴당 gh 호출)·`pr_fail`(바퀴)·`pr_timeout`(초)·`pr_inbox`(완료 없는 레인의 PR, 0·1) — [PR 거울](../hc-fill/pr.md).
- 묶음 손잡이: 표 줄의 이름이 `접두@`이고 `Per`가 있으면 저장 이름은 `접두@<값>`이다 — `seats@<기계>`·`pause@<기계>`(값 부분은 설정 `machine` 줄 이름만 받는다, `hc-fill machine`이 쓰는 길).
  표 기본은 없고(설정 줄 값이 기본), `knob ls`에는 둔 값만 한 줄씩 나온다.

## 무엇이 손잡이가 아닌가

- 안전 관문은 등록 표에 올리지 않는다: 착지 판정·push 관문·보호 규칙·훅·잠금·겹침 막기(`overlap`)·자동 감사 켬(`audit`)·사실 기록(`local`·`repo`·`machine`·`machine-has`).
- 이것들은 설계안의 「㉮ 안전 불변」이고, 바꾸려면 설정 파일이나 코드를 사람이 고친다.

## 다른 기계의 도구가 읽는 길

- 표에서 `Remote`인 손잡이는 지휘 기계의 hc-fill이 바퀴마다 원격 참조 `refs/herdr/knobs`로 올린다.
  - 꼴: 빈 트리 커밋, 날짜 0(같은 내용이면 같은 SHA), 제목 `hc-knobs`, 빈 줄 뒤 줄마다 `이름\t값\t언제까지(초|-)`.
  - 대상: 이 기계 `repo` 줄의 프로젝트마다.
  - 올린 내용의 지문을 `~/.local/share/hc-fill/knobs-remote.tsv`에 두어, 바뀌지 않은 바퀴는 git을 부르지 않는다. 손잡이를 지우거나 기한이 지나 실을 것이 없어지면 참조를 지운다.
- 읽는 쪽 공용 함수: `knob.ReadRemote(<클론>, …)`(ls-remote → fetch → 읽기). 바퀴마다 이미 ls-remote를 하는 도구는 그 SHA로 `knob.ReadCommit`을 부른다.
  - 읽을 때도 기한·등록 표·범위를 다시 본다.
  - 참조가 없거나 못 읽으면 표의 기본값이다(모르는 값으로 돌지 않는다).
- 저장 파일·원격 참조 어디서 읽든 등록 표의 범위 밖 값과 기한 지난 값은 먹지 않는다(그 손잡이가 없는 것으로).

## 병합기 (`merger_*`, 병합기 기계)

- 바퀴 일꾼이 바퀴마다 하는 ls-remote 한 번에 `refs/herdr/knobs`를 함께 싣는다 — 조회 수는 그대로다.
  SHA가 바뀐 바퀴에만 그 커밋을 읽고(클론에 없으면 fetch 한 번), 바뀌면 병합기 화면에 「손잡이 …」 한 줄.
  바퀴 일꾼은 바퀴마다 새 프로세스라 바꾼 값은 **다음 바퀴**에 먹는다(병합기를 다시 띄우지 않는다).
- 우선순위: 손잡이 > 병합기 일지 폴더의 옛 손잡이 파일(`front_dispatch`·`front_local`·`idle_load`·`busy_wait`·`busy_max`·`conflict`·`resolve_max`) > 환경 변수(·`--tick`) > 기본.
  - 옛 파일을 손잡이 아래에 둔 까닭: 병합기 기계에서 손으로 둔 값은 그 기계 사람의 뜻이지만, 지휘가 공통 손잡이로 바꾼 값이 먹지 않으면 「한 화면에서 조정」이 깨진다. 옛 파일은 손잡이가 없을 때만 먹는다.
  - 환경 변수와 `--tick`은 병합기를 띄울 때의 값이라 손잡이 아래다.
- 손잡이: `merger_batch`(개 — 묶음 크기), `merger_tick`(초 — 빈 바퀴 쉼), `merger_spec_start`·`merger_spec_timeout`·`merger_front_wait`·`merger_pre_max`(초), `merger_pre_rebuild`(번),
  `merger_front_dispatch`·`merger_front_local`(`0`|`1`), `merger_idle_load`(%), `merger_busy_wait`(초), `merger_busy_max`(번), `merger_conflict`(`agent`|`event`|`reject`), `merger_resolve_max`(초), `merger_resolve_stuck`(분 — 풀이 터미널 멈춤 사건 문턱, 0 끔).
  각 손잡이가 덮는 환경 변수는 `knob ls`의 설명 끝 괄호.
- 빈 바퀴 쉼은 상주 부모가 정하므로, 일꾼이 읽은 참조를 `<일지 폴더>/knobs.remote`(SHA 한 줄 + 메시지)에 두고 부모가 쉬기 전마다 읽는다(기한은 그때 다시 본다).
- 참조가 없거나 못 읽은 바퀴는 손잡이 없이(옛 파일·환경 변수·기본) 돈다.
- 옮기지 않은 것: 충돌 풀이를 띄울 명령 `resolve_launch`(명령 글이라 범위로 거를 수 없다 — 옛 파일·환경 변수 그대로), 기술 상한(`HC_LANE_SPEC_IDLE_MAX`·`HERDR_TIMEOUT_*`·`HC_LANE_FETCH_WAITS`).
  병합기 일지 폴더의 옛 파일은 병합기 기계에만 있어 지휘 기계의 `knob ls`에는 안 보인다.

## 정리 도구 (`sweep_r1`·`sweep_r2`·`sweep_r3`, 모든 기계)

- 닫는 문턱(분): R1 턴 0인 채 띄운 지(기본 30), R2 worker가 쉰 지(기본 30, 제출했으면 0), R3 쉰 지(기본 360). 보호 규칙은 손잡이가 아니다.
- 읽는 곳은 쓸기마다 하나:
  - 이 기계에 저장 파일이 있으면(지휘 기계) 그 파일.
  - 없으면 개인 설정 `sweep.knobs_repo`(이 기계의 클론 경로)가 있을 때만 그 origin의 `refs/herdr/knobs`(ls-remote 한 번, 바뀐 때만 fetch).
  - 둘 다 없거나 못 읽으면 기본이다 — 다른 기계에서 문턱을 바꾸려면 그 기계 설정에 `sweep.knobs_repo`를 둔다.
- `sweep_resident_grace`(분, 기본 5): 상주 판 점검의 유예 — 새 판이 깔린 지도, 상주가 뜬 지도 이만큼 지나야 옛 판·판 모름을 알린다([상주 판 점검](../hc-sweep/resident.md)). 같은 곳에서 읽는다.

## 옮기지 않은 앞 레인 스위치

- `herdr-sync --auto on|off`(스스로 배포 표식 `~/.local/state/herdr-sync/auto-deploy`) — 이 레인의 파일 밖(herdr-sync는 다른 레인이 함께 고치는 중)이라 그대로 두었다. 뒤 레인이 `auto_deploy` 손잡이로 옮긴다.
- `HC_LANE_PRECHECK_ALWAYS`(hc-lane) 같은 hc-lane 환경 변수도 같은 까닭으로 그대로다.
