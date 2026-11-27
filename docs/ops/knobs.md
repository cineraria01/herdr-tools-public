# 손잡이 (hc-fill knob — 실행 중에 바로 먹는 조정)

- 사람과 지휘 에이전트가 같은 명령으로 「지금 이 값만 잠깐 바꾸기」를 한다(2026-11-24 ops-knobs, 방향은 [설계안](agent-managed-design.md) 머리).
- 왜: 설정 `hc-fill.tsv`는 개인 git 저장소 안이라 고치면 저장소가 더러워져 herdr-sync가 멈추고, 병합기 값은 시작 때 읽는 환경 변수라 다시 띄워야 바뀌었다.
- 저장은 지휘 기계의 git 밖 파일 `~/.local/state/herdr-ops/knobs.tsv`(줄 = 이름·값·누가·왜·언제·언제까지·고정)이고, 개인 설정 파일은 건드리지 않는다(`knob save`만 아래 설정 파일에 쓴다).
- 손잡이는 「잠깐 바꾼 값」이다. 「정해 둔 값」은 아래 [설정 파일](#설정-파일-정해-둔-값)에 둔다.

## 설정 파일 (정해 둔 값)

- 2026-11-26 ht-config-knobs — 사용자 「이런것들은 하드 코딩이 아닌 설정 파일에서 바로 설정 할수 있으면 좋겠다, 기본적으로 모두 같은 범용으로 쓰고, 개인 설정 파일에서 설정하면 이런게 다 되면 좋도록 설계 구조 변경」.
- 개인 설정 폴더(`HERDR_TOOLS_CONFIG`, 기본 `~/.config/herdr-tools`)의 `knobs.tsv` 한 장에서 등록 표의 **모든** 손잡이를 정한다. 개인 저장소에 있으니 판 관리·되돌리기·다른 지휘가 보기가 된다.
  - 꼴: 한 줄에 `이름[@대상]  값  # 까닭`(공백·탭으로 나눔). 묶음 손잡이(`seats@<기계>` 등)와 병합기 프로젝트별(`merger_front_wait@<프로젝트>`)도 같은 이름 꼴이다.
  - 범용 틀은 `config.example/knobs.tsv` — 등록 표에서 만든 것이고(모든 손잡이가 주석 처리된 기본값·범위·설명), 손으로 고치지 않는다: `go run ./cmd/herdr-tools-config knobs-template > config.example/knobs.tsv`. 시험(`TestConfTemplate`)이 표와 바이트까지 맞춘다.
- 모든 기계가 같은 값: herdr-sync가 개인 저장소의 그 파일을 모든 기계의 같은 자리에 맞춘다(`config.example/sync.list`의 `손잡이` 줄 — 개인 `sync.list`에 같은 줄을 둔다).
  원격 참조(`refs/herdr/knobs`)에도 지휘 기계의 hc-fill이 손잡이 > 설정 파일로 푼 값을 싣는다 — 참조만 읽는 도구(병합기·검증·`sweep.knobs_repo`를 둔 정리 도구)도 같은 값이다.
- 우선순위(모든 도구 같다): 실행 중 손잡이 > 설정 파일 > 도구별 옛 설정(`hc-fill.tsv` 같은 이름 키·병합기 일지 폴더 파일·환경 변수) > 기본. 프로젝트별은 손잡이(`이름@프로젝트` → `이름`) 다음 설정 파일(같은 차례).
- 표에 없는 이름·범위 밖 값·값 없음·같은 이름 두 번(뒤 줄)은 그 줄만 버리고(아래 층으로) 한 줄씩 알린다(hc-fill·hc-sweep·hc-lane watch·herdr-sync — 병합기는 바퀴마다 새 프로세스라 알리지 않는다).
  미리 잡기: `herdr-tools-config knobs-lint [<파일>]` — 버릴 줄을 「파일:줄: 까닭」으로, 있으면 종료 1.
- 읽는 곳: hc-fill(`conf` — 바퀴마다 새로), 병합기(바퀴마다 참조 위에), hc-sweep(쓸기마다), hc-lane watch(`coord_digest*` 보낼 때마다), selfexec(`resident_reexec`), herdr-sync(`sweep_resident_grace`), 검증(`verify_scope` — 참조에 값이 없을 때).
- 옮기기: `hc-fill knob save <이름> [--why <까닭>] [--dry-run]` — 실행 중 손잡이 값을 설정 파일에 정해 두고 손잡이는 지운다(먹는 값은 그대로, 고정 손잡이도). 같은 이름 줄이 있으면 그 자리를 바꾸고 없으면 끝에 더한다. 까닭을 안 주면 손잡이의 까닭. 기한은 옮기지 않는다(이제 계속). git 커밋은 하지 않는다 — 「다음:」 줄의 명령으로 개인 저장소에서 커밋한다. 일지에 한 줄. 운영 에이전트 실행 안에서는 사람에게 넘긴다(미리 보기만 된다).

## 쓰는 법

- `hc-fill knob ls [--json]` — 손잡이마다 지금 먹는 값·출처(`기본`/`설정`/`설정 파일 <경로>:<줄>`/`손잡이`)·누가·왜·언제까지·고정·허용 범위·설명·읽는 도구.
  - 손잡이 줄은 「잠깐 바꾼 것」(기한 없는 손잡이에는 `knob save` 안내), 설정 파일 줄은 「정해 둔 것」. `--json`은 `source`(`설정 파일`)·`where`(경로:줄)·`kind`.
  - 묶음·프로젝트별 줄은 손잡이나 설정 파일에 둔 것만 한 줄씩.
- `hc-fill knob set <이름> <값> --why <까닭> [--for <분> | --until <시각>] [--pin] [--force]`
  - `--why`는 꼭 받는다(빈 글은 거절).
  - 누가 = 부른 터미널 주소(`HC_TERMINAL_ID`), 없으면 `사람`.
  - 다른 누가가 `advise_claim`분(기본 30) 안에 정한 같은 손잡이를 덮으면 표준 오류에 한 줄 `hc-fill: <누가>가 n분 전 <값>로 정함(<까닭>) — 덮음`(막지 않는다 — 같은 알림에 지휘 둘이 서로 덮었다, 2026-11-26 ht-advise-claim).
  - `--until` 꼴은 RFC3339 또는 `2006-01-02 15:04`(이 기계 시간대), 지난 시각은 거절.
- `hc-fill knob unset <이름> [--force] [--why <까닭>]` — 손잡이를 지워 설정 값으로 돌린다.
- set·unset은 [운영 일지](journal.md)에 한 줄씩 남는다(되돌리는 명령과 함께).
- 거절(종료 1): 등록 표에 없는 이름(가까운 이름을 권한다), 범위 밖 값, `--why` 없음, 고정된 값을 `--force` 없이 바꾸거나 지우기. 인자 꼴이 틀리면 사용법(종료 2).

## 우선순위·기한·고정

- 우선순위: 손잡이 > 설정 파일(`knobs.tsv`) > 도구별 옛 설정 > 기본값.
- 바뀐 값은 재시작 없이 다음 바퀴에 먹는다(hc-fill 바퀴는 바퀴마다 새 프로세스라 저장 파일·설정 파일을 새로 읽는다).
- 기한(`--for`·`--until`)이 지나면 그 손잡이는 저절로 없는 것이 되고 설정 파일·설정 값으로 돌아간다. 지난 줄은 다음 쓰기 때 파일에서 걷힌다.
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
  기계 닿지 않음([끊김](../hc-fill/unreach.md)): `unreach`(분). 다시 닿은 기계의 꺼져 죽은 레인 스스로 이어 띄우기([이어 띄우기](../hc-fill/revive.md)): `revive_max`(줄당 번, 기본 2, 0이면 스스로 안 함 — `hc-fill revive` 명령은 그대로). 저장소 읽기 막힘·원격 로그인 거절 격리([격리](../hc-fill/cordon.md)): `cordon`(분, 기본 5, 0이면 알리기만 — 두 점검이 함께 쓴다) — 격리 상태 `cordon@<기계>`는 손잡이가 아니라 hc-fill이 원격 참조에 싣는 실행 상태다(기한 있음, 사람이 걸지 않는다).
  다른 지휘가 막 넣은 레인 알림([주인 지휘](../hc-fill/owner.md)): `sibling_min`(분, 기본 120, 0 끔).
  멈춘 레인 다시 굴리기([nudge](../hc-fill/edit.md)): `nudge_every`(분, 한 레인에 다시 보내기까지, 0 막지 않음).
  멈춘 레인 감지([멈춘 레인](../hc-fill/hang.md)): `hang_min`(분, 기본 15, 0 끔)·`hang_esc`(0·1, 기본 1 — 「생각 중」 멈춤에 Escape와 이어 가라는 글).
  상태 한 장의 눈에 띄는 것([상태](state.md)): `state_stage`(분), `state_hot`·`state_cool`(%). 기본 출력이 끝난 레인을 남기는 시간: `state_done`(분).
  지금 손댈 것([advise](advise.md)): `advise_wait`·`advise_pinned`·`advise_merge_slow`·`advise_every`(분, 0 끔) — 풀 수 있는 대기·사슬의 문턱은 `idle`과 같다. 조치함·덮음 알림의 기간 `advise_claim`(분, 기본 30, 0 끔). 밀린 배포의 착지 뒤 유예 `deploy_grace`(분, 기본 5). 착지 뒤 명령의 종료 75 다시 돌리기 상한 `onland_retry_wait`(분, 기본 10, 0 끔 — [착지 뒤 명령](../hc-fill/on-land.md)).
  운영 에이전트([agent](agent.md)): `ops_agent`(끔|판단만|조치, 기본 끔)·`ops_agent_max_hour`(번)·`ops_agent_timeout`(분)·`ops_agent_tries`(번)·`ops_agent_scope`(프로젝트)·`ops_agent_close`(0·1, 기본 1) — 운영 에이전트 실행 안에서는 이 여섯을 바꾸지 못한다.
- 옮긴 hc-fill 설정 키(knobs-migrate, 2026-11-24 — 기본은 지금 코드 값 그대로): `gc`·`idle_empty`·`reland_wait`(분), `reland_max`·`audit_max`(번),
  `audit_close`·`audit_seat_rounds`(바퀴), `lows_hour`·`lows_min`(개).
- 병합기 살아 있음(hc-fill, merger-alive): `merger_dead`·`merger_unknown`(분, 0 끔), `merger_restart`(`0`|`1`).
  옛 표식 파일 `<hc-fill 상태 폴더>/merger_dead`·`merger_unknown`·`merger_restart`는 손잡이가 없을 때 그대로 먹고, `knob ls`에 출처 `설정`으로 보인다.
- 목록 손잡이 `auto_submit`(프로젝트 이름을 쉼표로, `-` = 끔)과 그 시간 상한 `auto_submit_max`(초) — [자동 감사](../hc-fill/audit.md) 「대기열이 제출」.
- 목록 손잡이 `stack`(쌓아 띄움을 켤 프로젝트, `-` = 끔)과 사슬 깊이 `stack_depth`(겹, 1~5, 기본 2) — [쌓아 띄움](../hc-fill/stack.md).
- 목록 손잡이 `pr`(PR 거울을 켤 프로젝트, `-` = 끔)과 `pr_max`(바퀴당 gh 호출)·`pr_fail`(바퀴)·`pr_timeout`(초)·`pr_inbox`(완료 없는 레인의 PR, 0·1), 사람 리뷰의 `pr_human`(읽을 작성자 목록, `me` = gh 로그인, `-` = 끔)·`pr_human_block`(착지를 막는 글 종류)·`pr_human_approve`(재승인 기다림, 0·1) — [PR 거울](../hc-fill/pr.md).
- 지휘 알림 묶기 `coord_digest`(분, 0~120, 기본 0 = 하나씩 — 읽는 도구 hc-lane watch, 보낼 때마다 이 저장 파일을 읽는다) — [watch](../hc-lane/watch.md) 「지휘 알림 묶기」. 주기 전이라도 보내는 개수 `coord_digest_max`(개, 기본 20).
- 알림 받는 쪽 `alert_route`(`all`|`one`, 기본 `all`)·대표 지휘 `alert_coord`(지휘 주소 목록, 프로젝트별 `alert_coord@<프로젝트>`)·놓침 막기 `alert_escalate_min`(분, 기본 30, 0 끔) — 읽는 도구 hc-lane watch(바퀴마다)·hc-fill(운영 에이전트 사건) — [알림 받는 쪽](../hc-fill/alert-route.md).
- 상수에서 올린 정책(ht-config-knobs, 2026-11-26 — 기본은 그때 값 그대로): `light_guess`(0·1, 기본 1 — 무게 안 정한 레인을 지시문 「파일:」 줄로 가벼움 추정, [부하](../hc-fill/load-steer.md)), `hang_shell_grace`(초, 기본 180 — [멈춘 레인](../hc-fill/hang.md)의 셸 기다림), `unsure_warn`(바퀴, 기본 5 — 띄움 미확정 막힘 사건), `orphan_alarm`(바퀴, 기본 10 — 남은 무리로 건너뛴 바퀴 막힘 사건), `pr_open_max`(분, 기본 15 — 착지 뒤 OPEN인 PR 닫기), 병합기 `merger_defer_alarm`(바퀴, 기본 10 — 같은 까닭으로 쉰 바퀴 막힘 사건).
- 묶음 손잡이: 표 줄의 이름이 `접두@`이고 `Per`가 있으면 저장 이름은 `접두@<값>`이다 — `seats@<기계>`·`pause@<기계>`·`light_only@<기계>`(값 부분은 설정 `machine` 줄 이름만 받는다, `hc-fill machine`이 쓰는 길).
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
- 우선순위: 손잡이(참조 — 지휘 기계의 설정 파일 값도 실림) > 이 기계의 손잡이 설정 파일 > 병합기 일지 폴더의 옛 손잡이 파일(`front_dispatch`·`front_local`·`idle_load`·`busy_wait`·`busy_max`·`conflict`·`resolve_max`) > 환경 변수(·`--tick`) > 기본.
  - 옛 파일을 손잡이 아래에 둔 까닭: 병합기 기계에서 손으로 둔 값은 그 기계 사람의 뜻이지만, 지휘가 공통 손잡이로 바꾼 값이 먹지 않으면 「한 화면에서 조정」이 깨진다. 옛 파일은 손잡이가 없을 때만 먹는다.
  - 환경 변수와 `--tick`은 병합기를 띄울 때의 값이라 손잡이 아래다.
- 손잡이: `merger_batch`(개 — 묶음 크기), `merger_tick`(초 — 빈 바퀴 쉼), `merger_spec_start`·`merger_spec_timeout`·`merger_front_wait`·`merger_pre_max`(초), `merger_pre_rebuild`(번),
  `merger_front_dispatch`·`merger_front_local`(`0`|`1`), `merger_culprit`(`parallel`|`serial` — 묶음 실패 뒤 범인 가리기, [병합기 묶음](../hc-lane/merger-batch.md)), `merger_idle_load`(%), `merger_busy_wait`(초), `merger_busy_max`(번), `merger_conflict`(`agent`|`event`|`reject`), `merger_resolve_max`(초), `merger_resolve_stuck`(분 — 풀이 터미널 멈춤 사건 문턱, 0 끔),
  `merger_front_freeze`·`merger_aside`(`0`|`1`, 기본 1 — 도는 맨 앞 무리를 그대로 · 착지 스크립트 저장소의 추측 시험 실패를 차례 근거로, [추측 검증 「맨 앞을 흔들지 않기」](../hc-lane/merger-spec-verify.md)),
  `merger_spec_fresh`(분 — 맨 앞 기다림 상한을 넘긴 추측이 이만큼 안에 갱신됐으면 옮기거나 직접 검증하지 않고 더 기다림, 0 끔),
  `merger_land_on`(`auto`|`local`|기계 목록 — 직접 검증 착지를 돌릴 기계, [병합기 「착지 실행 기계」](../hc-lane/merger.md)),
  `merger_shards`(몫, 1~8, 기본 1 — 맨 앞 추측을 검증 기계 여럿에 나눠 돌림, [추측 검증 「나눠 돌리기」](../hc-lane/merger-spec-verify.md) — 검증 스크립트가 그 계약을 따르는 저장소만),
  기계별 묶음 손잡이 `merger_gate@<기계>`(`0`|`1`, 기본 0 — 1이면 그 검증 기계는 레인 검증이 돌아도 병합 검증 자리로 맡김, [추측 검증 「병합 검증 자리」](../hc-lane/merger-spec-verify.md) — 프로젝트별 덮어쓰기가 아니라 기계 이름이고, 원격 참조에 그 이름 그대로 실린다).
  각 손잡이가 덮는 환경 변수는 `knob ls`의 설명 끝 괄호.
- 프로젝트별 덮어쓰기(merger-knob-per-project, 2026-11-24): 저장소마다 검증 시간이 크게 달라 병합기 손잡이를 저장소 하나에만 다르게 줄 수 있다.
  - 꼴: `hc-fill knob set <병합기 손잡이>@<프로젝트> <값> --why …` — 위 `merger_*` 손잡이만 받고, 프로젝트는 이 기계(지휘 기계)에 설정 `repo` 줄이 있는 이름만 받는다(오타·원격에 올릴 클론이 없는 프로젝트는 거절). 범위·`--for`·`--until`·`--pin`·`--force`는 낱개와 같다.
  - 병합기가 읽는 차례: `<이름>@<그 병합기의 프로젝트>` → `<이름>`(공통) → 일지 폴더 파일 → 환경 변수 → 기본. 프로젝트별 값의 기한이 지나면 공통으로 돌아간다.
  - 원격 참조에는 지휘 기계의 hc-fill이 **그 저장소 몫으로 푼 값**을 이름 그대로 올린다(프로젝트 이름은 싣지 않는다) — 병합기·검증 일꾼의 읽기 코드는 그대로이고, 다른 프로젝트의 값은 그 저장소 원격에 새지 않는다.
    프로젝트별 값이 없으면 올라가는 내용은 전과 바이트까지 같다.
  - `hc-fill knob unset <이름>@<프로젝트>` — 그 프로젝트도 공통 값으로 돌아간다. `knob ls`에는 공통 줄 바로 밑에 둔 프로젝트별 값만 한 줄씩(설명 끝 「프로젝트 … 몫」).
  - 본보기 — 검증이 긴 저장소만 맨 앞 기다림 상한을 길게: `hc-fill knob set merger_front_wait@<검증이 긴 프로젝트> 1500 --why "전체 추측 검증 10~15분"`, 공통 `merger_front_wait`는 기본 600 그대로.
- 빈 바퀴 쉼은 상주 부모가 정하므로, 일꾼이 읽은 참조를 `<일지 폴더>/knobs.remote`(SHA 한 줄 + 메시지)에 두고 부모가 쉬기 전마다 읽는다(기한은 그때 다시 본다).
- 참조가 없거나 못 읽은 바퀴는 손잡이 없이(옛 파일·환경 변수·기본) 돈다.
- 옮기지 않은 것: 충돌 풀이를 띄울 명령 `resolve_launch`(명령 글이라 범위로 거를 수 없다 — 옛 파일·환경 변수 그대로), 기술 상한(`HC_LANE_SPEC_IDLE_MAX`·`HERDR_TIMEOUT_*`·`HC_LANE_FETCH_WAITS`).
  병합기 일지 폴더의 옛 파일은 병합기 기계에만 있어 지휘 기계의 `knob ls`에는 안 보인다.

## 정리 도구 (`sweep_r1`·`sweep_r2`·`sweep_r3`, 모든 기계)

- 닫는 문턱(분): R1 턴 0인 채 띄운 지(기본 30), R2 worker가 쉰 지(기본 30, 제출했으면 0), R3 쉰 지(기본 360). 보호 규칙은 손잡이가 아니다.
- 읽는 곳은 쓸기마다 하나:
  - 이 기계에 저장 파일이 있으면(지휘 기계) 그 파일.
  - 없으면 개인 설정 `sweep.knobs_repo`(이 기계의 클론 경로)가 있을 때만 그 origin의 `refs/herdr/knobs`(ls-remote 한 번, 바뀐 때만 fetch).
  - 거기 없는 값은 이 기계의 손잡이 설정 파일(herdr-sync로 맞춘 같은 파일), 그것도 없으면 기본이다.
- `sweep_resident_grace`(분, 기본 5): 상주 판 점검의 유예 — 새 판이 깔린 지도, 상주가 뜬 지도 이만큼 지나야 옛 판·판 모름을 알린다([상주 판 점검](../hc-sweep/resident.md)). 같은 곳에서 읽는다. `herdr-sync --check`의 상주 확인도 이 유예를 쓴다(맥 저장 파일).
- `sweep_test_orphan`(분, 기본 30): 부모 없는 시험 고아(시험 임시 경로 프로세스)가 이만큼 남아 있으면 운영 사건 `hc-test-orphan` 한 번([시험 고아 점검](../hc-sweep/test-orphans.md)). 같은 곳에서 읽는다.
- `sweep_ego_idle`(분, 0~10080, 기본 120): 에이전트가 만들고 쥔 ego 작업 공간의 목록 지문이 이만큼 그대로면 닫고 운영 사건 `hc-ego-idle`([ego 작업 공간 쓸기](../hc-sweep/ego-spaces.md)). 0이면 끔(목록도 안 읽음). 같은 곳에서 읽는다.
- `resident_reexec`(0|1, 기본 1): 상주 고리가 새로 깔린 판으로 스스로 넘어간다 — 0이면 넘어가지 않고 손으로 다시 띄울 때까지 옛 판([selfexec](../go/selfexec.md)). 상주가 새 판을 본 바퀴에 **그 기계의** 저장 파일, 없으면 손잡이 설정 파일을 읽는다(원격 참조 없음 — 모든 기계에서 끄려면 설정 파일, 한 기계만 잠깐이면 그 기계 `~/.local/state/herdr-ops/knobs.tsv`).

## 검증 범위 (`verify_scope`, 검증하는 모든 기계)

- `verify_scope`(`observe`|`on`|`off`, 기본 `on`): 검증이 통과 기록과 같은 조각을 건너뛸지([시험](../repo/tests.md) 「검증의 범위」). 환경 변수 `HERDR_VERIFY_SCOPE`가 앞선다.
- 검증마다 읽는다 — 이 기계 저장 파일(지휘 기계)에 살아 있는 값이 있으면 그것, 없으면 검증하는 클론 origin의 `refs/herdr/knobs`(다른 기계는 hc-fill 다음 바퀴가 올린 뒤부터), 참조에 값이 없으면 손잡이 설정 파일. 못 읽으면 `observe`(전부 돈다).

## 옮기지 않은 앞 레인 스위치

- `herdr-sync --auto on|off`(스스로 배포 표식 `~/.local/state/herdr-sync/auto-deploy`) — 이 레인의 파일 밖(herdr-sync는 다른 레인이 함께 고치는 중)이라 그대로 두었다. 뒤 레인이 `auto_deploy` 손잡이로 옮긴다.
- `HC_LANE_PRECHECK_ALWAYS`(hc-lane) 같은 hc-lane 환경 변수도 같은 까닭으로 그대로다.
