# 상태 한 장 (hc-fill state)

- 기계·대기열·레인·병합 줄·배포·감시·손잡이를 명령 하나로 보인다(2026-11-24 ops-state, 방향은 [설계안](agent-managed-design.md) 4-1).
- 왜: 전체를 보려면 지휘 에이전트가 명령 예닐곱 개(대기열 status·ls, 기계별 원격 호출, 병합기 일지, 원격 참조, 도구 판)를 손으로 맞춰야 했다.
- 사람용 `hc-fill state`와 기계가 읽는 `hc-fill state --json`은 같은 사실을 담는다.
- 읽기만 한다(잠금·상태 파일 쓰기 없음). 모르는 값은 모름(json `null`과 `why`)이고 0·정상으로 적지 않는다.
- 판단은 하지 않는다. 「눈에 띄는 것」은 판단이 필요해 보이는 사실의 목록일 뿐이고, 조치는 지휘가 정한다.

- 칸 이름은 시험 `internal/opsstate/state_test.go`(⑤)가 아래 「json 칸」 표와 맞춘다 — 칸을 더하면 표에 한 줄 더한다.

## 쓰는 법

- `hc-fill state` 사람용 표, `hc-fill state --json` 압축 json 한 줄, `--all`은 추리지 않은 전체(아래 「기본과 --all」). 인자는 이 셋뿐이고 함께 쓸 수 있다.
- 지휘 에이전트는 `--json`의 `fingerprint`를 기억해 두고, 다음에 같으면 나머지를 다시 읽지 않는다.
- `notable`이 비어 있지 않으면 `hc-fill advise`로 그 줄의 근거·다음 명령을 본다. 조치는 손잡이([손잡이](knobs.md))·`hc-fill machine`·`prio`·`retry`·`merger restart` 같은 기존 명령으로 한다.

## 기본과 --all

- 기본 출력은 지금 움직이는 것만 보이고 나머지는 개수 한 줄로 줄인다(2026-11-24 ops-state-trim — 배포 직후 942줄이었다: 레인 절 472줄 가운데 끝난 줄 291, 감시 14줄 가운데 「안 돎」 12).
  `--all`은 추리지 않은 전체, `--json`도 같은 규칙이다(`--all --json`은 전체). 숨긴 개수는 json `hidden`에 있고, `--all`이면 `hidden`이 `null`이다.
- 레인: 끝난 레인은 더 할 일이 없는 레인이다 — 터미널이 살아 있지 않고(살았는지 모름도 아님), 마지막 사건이 정리됨·착지이거나 단계가 「착지 …」·「끝남」.
  단계 글의 낱말로만 가르지 않는다(2026-11-24 지휘 지시 — 착지·정리까지 끝났는데 단계가 「감사 통과 받음」으로 남은 179줄이 숨지 않았다). 감사 통과를 받았어도 터미널이 살아 있거나 착지 사건이 없으면 움직이는 것이다.
  끝난 레인은 마지막 사건이 `state_done`분(손잡이, 기본 60) 안이면 남기고, 넘으면 숨겨 「끝난 레인 <n>줄 숨김(--all)」 한 줄로 센다.
  나머지 단계(작업 중·감사·감사 대기·완료·제출됨·주차·대기·막힘·띄우기 거절·모름)는 나이와 무관하게 남긴다. 끝난 레인이라도 마지막 사건 시각을 모르면 남긴다(오래됐다고 말할 수 없다).
- 감시: 도는 것과 모름만 남기고, 안 도는 것은 「안 도는 지휘 흔적 <n>개」 한 줄로 센다.
- 배포: 배포 기록에 건너뜀만 있는 기계(이 기계 판 대상이 아님 — 다른 OS 등)는 「대상 아님 <n>대」 한 줄로 센다. 실패·충돌만 있는 기계는 모름 그대로 남는다.
- 기계 건강: 닿은 기계 모두 건강 넷이 모름이면 줄마다 적지 않고 절 끝에 「건강 값 없음 — 점검이 아직 안 남김」 한 줄, 하나라도 값이 있으면 값이 있는 기계 줄에만 적는다.
- 지문은 늘 추린 상태로 만든다(`--all`도 같은 지문 — 숨긴 줄·숨긴 개수는 지문에 없다). 눈에 띄는 것은 보이는 것으로 만든다(`--all`이면 안 도는 감시도 나온다).
- 지휘가 봐야 하는 줄(막힘·띄우기 거절·감사 대기·모름)은 숨기지 않는다. 끝난 레인만 시간으로 숨긴다.

## 어디서 모으나

- 바퀴가 남긴 기록을 다시 쓴다(원격을 더 부르지 않는다). 나이는 `sources[]`의 `tick` 줄(마지막 바퀴가 끝난 뒤 초).
  - 기계 부하·코어·쓴 자리·안 띄운 까닭: `<DATA>/load.tsv`([부하](../hc-fill/load-steer.md)). 자리 상한·레인 상한은 설정 `machine` 줄과 `seats@` 손잡이.
  - 병합기 살아 있음·줄 길이: `<DATA>/merger-alive.tsv`(hc-fill 바퀴가 적는다). 바퀴가 읽은 큐의 레인도 줄 차례대로 적어(12칸), 제출한 레인의 단계가 「제출됨(병합기 줄 n번째)」다 — 큐를 못 읽었거나 옛 기록이면 「줄 모름」.
    살아 있는 병합기는 같은 줄에 [병합기 상태 한 장](../hc-lane/merger-state.md)(13칸, 못 읽은 까닭 14칸 — run 바퀴가 `hc-merger --state`로 이 기계·다른 기계 모두 같은 길로 받음)을 둔다. 지금 단계·마지막 진행·기다리는 대상(주소)·착지 안쪽 단계는 이것만 읽는다.
    그 기록의 단계가 추측 검증 기다림(부하 대기 포함)이면 구성원에 「· 묶음 검증 중 <기계,…> n분째」, 착지면 맨 앞 레인에 「· 착지 중 n분째[ — <안쪽 단계> m분째]」를 붙인다(`hc-fill status`도 같은 덧말).
  - 병합기 검증·건너뜀·맡김(이력): 병합기가 이 기계에 있는 프로젝트만 이 기계 병합기 일지 `<XDG_STATE_HOME, 없으면 ~/.local/state>/hc-lane/merger-journal.tsv`(읽기만).
    최근 착지 20번(후보 SHA로 묶고 「이미 … 에 있음」 정리는 뺌 — `hc-lane merger --why`와 같은 셈)의 수·검증 건너뜀 수와 마지막 착지의 착지 시간(시작 → 끝, 검증 포함), 최근 20번의 검증 맡김(추측 기다림). 일지는 지금 상태 판정에 쓰지 않는다.
    이 기계 병합기 프로젝트가 둘 이상이면 일지 줄을 대기열의 레인 → 프로젝트로 가른다(대기열에서 정리된 레인 줄은 빠진다).
  - 레인 마지막 사건·최근 사건: `<DATA>/last.tsv`([마지막 보고](../hc-fill/last-report.md)). 그 사건 시각부터 지금까지가 `stage_age_s`다.
  - 대기 까닭: 띄우기 거절 기록 · `overlap.tsv` 겹침 · `need.tsv` 능력 · `pinned.tsv` 묶인 기계([묶인 기계](../hc-fill/pinned.md)) · 대기열의 선행 칸 · 그 밖은 「자리·차례」(기계별 까닭은 `machines[].skip_why`).
- 이번에 부르는 것(`sources[]`의 `hc` 줄 — 호출 수·초): `hc ls --json`(레인 살아 있음·감시 터미널) · `hc machine ls --json`(기계 연결). 둘 다 이 기계 코어에 묻는 것이다.
- 배포 기록의 옛 이 기계 이름 `맥`은 설정 `local` 기계로 읽는다(herdr-sync가 그 이름으로 적기 전 기록 — [이 기계 이름](../herdr-sync/local-name.md)).
- 배포(`deploy` 줄): herdr-sync 배포 기록 `~/.local/state/herdr-sync/deploy.tsv`에서 기계마다 결과가 같음·씀인 마지막 줄의 커밋(「기록상 판」)과, 범용 저장소(`HERDR_TOOLS_REPO`, 없으면 herdr-sync와 같은 기본 자리)의 `git rev-list --count <판>..origin/main`(fetch 없이 — 판마다 한 번).
  기계의 실제 바이너리를 보려면 `herdr-sync --status`(기계마다 원격 호출)를 쓴다.
- 손잡이: 공통 손잡이 저장의 살아 있는 줄. 최근 조치: 운영 일지 24시간 안 마지막 5줄([일지](journal.md)).

## 모름

- 마지막 바퀴에 그 기계 부하를 못 읽었으면(읽은 때가 바퀴 끝보다 5분 넘게 앞) `load1`·`cores`·`load_per_core`는 `null`이고 치우침에도 안 쓴다.
- `verifies`는 부하와 같은 호출에서 읽어 같은 규칙으로 `null`이고, 그 기계에서 `hc-lane merger --spec-idle`을 못 부르면(옛 판 등) 부하만 있고 `null`이다. 기계 줄·치우침 근거에는 1 이상일 때만 「검증 n」으로 보인다(모름을 0으로 적지 않는다, 2026-11-26 ht-skew-verify-cause — 레인 0인 바쁜 기계가 병합기 추측 검증으로 바빴는데 「도는 레인 0」만 보여 같은 날 두 번 손으로 찾았다).
- 닿지 못한 기계(`hc machine ls`에서 연결 아님)는 `reach: unknown`·`why: 연결 안 됨`이고 잰 칸은 모두 `null`이다 — 바퀴 기록에 지난 부하가 있어도 지금 값으로 적지 않는다.
- `hc machine ls`를 못 읽으면 이 기계 말고는 모두 `연결 모름`. `hc ls`를 못 읽으면 레인 단계가 「모름(hc ls 못 읽음)」이고 `lanes`·`watch[].alive`는 `null`.
- 아직 모으는 곳이 없는 칸은 `null`이다:
  - `machines[].health` 넷(잠자기·키체인·디스크·Tailscale) — 기계 건강 점검이 기록을 남기면 채운다.
  - `merge[].recent_verify_s`·`skip_ratio`·`lands`·`skipped`·`delegated`(이력 — 일지 통계) — 병합기가 다른 기계에 있으면 그 기계 일지에만 있다(원격으로 읽지 않는다, `merge[].why`에 적힌다). 이 기계 병합기라도 일지가 없거나 못 읽으면 모두 모름.
  - `merge[].last_round_age_s`·`phase…`·`waits`·`inner` — 병합기 기록(run 바퀴가 `hc-merger --state`로 이 기계·다른 기계 모두 같은 길로 받아 둔 것)이 없으면 모름. 옛 판 병합기면 `why`에 「모름(옛 판 병합기 — 다시 띄우면 기록)」.
- `stage_age_s`는 마지막 사건 시각(`last.tsv`는 시:분만)에서 잰다. 하루 넘게 머문 단계는 하루 미만으로 읽힌다.

## 상태 지문

- 판단 칸만으로 만든 12자: 기계(닿음·까닭·에이전트 수·자리·레인·상한·건강·조정·안 띄운 까닭) · 대기열(상태별 수·가장 오래 기다린 줄과 까닭) · 레인(이름·기계·단계·마지막 사건) · 병합 줄(길이·병합기) · 배포(판·밀림·까닭) · 감시 · 손잡이 · 최근 조치·사건 · 눈에 띄는 것.
- 넣지 않는 것: 만든 시각, 재료 나이, 머문 시간·기다린 시간, 1분 부하·코어당 부하, 검증 시간·건너뜀 비율. 글 칸(눈에 띄는 것·안 띄운 까닭)은 숫자를 빼고 넣는다 — 「95분」이 「96분」이 되어도 같은 지문이다.
  레인 단계 글은 경과 분(「감사 대기(자리 없음 N분)」)만 빼고 넣는다 — 감사 차수·sha는 판단 칸이라 남는다.
- 시간만으로 바뀌는 길 하나: 최근 조치는 24시간 창의 꼬리라, 새 조치가 없어도 오래된 줄이 창 밖으로 빠지면 지문이 바뀐다(보이는 사실도 실제로 바뀐다).
- 최근 사건은 `last.tsv` 시:분으로 늦은 것 먼저 세운다 — 자정을 넘으면 차례가 어긋난다.

## 눈에 띄는 것

- [지금 손댈 것](advise.md)(`hc-fill advise`)의 항목을 요약 한 줄씩(「무엇 — 근거」) 적는다 — 같은 판정을 두 군데 두지 않는다(2026-11-24 ops-advise).
  항목 종류·문턱 손잡이·다음 명령·확신은 그 문서에 있다. 상태 한 장에는 다음 명령이 없으니, 줄이 있으면 `hc-fill advise`로 본다.
- 규칙으로 조치하지 않는다. 문턱은 손잡이다(`hc-fill knob set <이름> <값> --why …`): `state_stage`·`state_hot`·`state_cool`·`idle`·`advise_wait`.
- 이 기계의 상주가 옛 판·판 모름이면(hc-sweep 상주 판 점검 결과 한 장, [상주 판 점검](../hc-sweep/resident.md), 문턱 `sweep_resident_grace`) 그 줄도 여기에 나온다.

## 흔한 판단 예

- 「기계 A 코어당 3.1, 기계 B 0.1」 → 대기 줄이 있으면 `hc-fill machine A --seats <n> --for 60 --why …`로 A 자리를 잠깐 줄인다. 대기 줄이 없으면 그대로 둔다.
- 「레인 X 같은 단계에 120분 — 작업 중」 → 그 레인 화면을 `hc read`로 본다. 「완료」 단계에 오래면 감사 수거가 막혔는지 `hc-fill status`의 감사 비고를 본다.
- 「배포 4커밋 밀림」 → `herdr-sync --status`로 실제 판을 확인하고, 병합 줄이 비었을 때 배포한다.
- 「병합기 p: gone」 → `hc-fill merger restart p --why …`.
- 「지휘 t 감시 안 돎」 → 다음 바퀴에 hc-fill이 다시 띄운다(열린 레인이 있는 주인). 이어지면 `hc-lane coord --watch t`.

## json 칸

값이 모름이면 `null`이고, 그 까닭은 같은 줄의 `why`에 있다.

| 칸 | 뜻 |
|---|---|
| `schema` | 꼴 판(정수). 칸 뜻이 바뀌면 올린다 |
| `at` | 만든 시각(RFC3339) |
| `fingerprint` | 상태 지문 — 판단 칸만으로 만든 짧은 값. 지난번과 같으면 다시 읽지 않아도 된다 |
| `sources` | 재료마다 어디서 언제 읽었나 |
| `sources[].name` | 재료 이름: `tick` 대기열 바퀴 기록 · `hc` 이번에 부른 hc ls·hc machine ls · `deploy` 배포 기록과 git rev-list · `journal` 운영 일지 |
| `sources[].age_s` | 재료 나이(초), 모르면 `null` |
| `sources[].calls` | 이번 호출에서 부른 바깥 명령 수(`hc` 줄은 이 기계 코어에 묻는 hc, `deploy` 줄은 이 기계 git rev-list) |
| `sources[].secs` | 이번 호출에서 그 재료에 든 초 |
| `sources[].why` | 못 읽었으면 까닭 |
| `machines` | 기계마다 한 줄(이름은 설정에서 읽은 것) |
| `machines[].name` | 기계 이름 |
| `machines[].reach` | `ok` 닿음 · `unknown` 모름 |
| `machines[].why` | 모름의 까닭 |
| `machines[].load1` | 1분 부하 |
| `machines[].cores` | 코어 수 |
| `machines[].load_per_core` | 코어당 부하 |
| `machines[].live` | 자리를 쓰는 에이전트 수 — 대기열이 띄워 지금 일하는 것([자리 세기](../hc-fill/slots.md)) |
| `machines[].resting` | 자리에 안 드는 쉬는(done·idle) 대기열 터미널 수(감사·착지·다시 넣기 기다림) — 옛 기록이면 `null` |
| `machines[].outside` | 자리에 안 드는 대기열 밖 에이전트 수(사람·지휘 세션·손으로 띄운 레인·루프 — 상태를 보내는 터미널) — 옛 기록이면 `null` |
| `machines[].seats` | 자리 상한 |
| `machines[].lanes` | hc-fill이 띄운 레인 수 |
| `machines[].verifies` | 도는 저장소 검증 수(레인·추측·착지 검증 합친 검증 나무 수, `hc-lane merger --spec-idle`과 같은 셈, 부하와 같은 때 읽음) |
| `machines[].merge_verifies` | 그 가운데 병합기 검증(추측 일꾼·착지 실행, `--spec-idle`의 `gates=`) 수 — 자리·레인 상한에는 안 든다. 옛 판 워커·모름이면 `null` |
| `machines[].lane_cap` | hc-fill 레인 상한 |
| `machines[].free` | 마지막 바퀴의 띄우기가 판정한 보통 레인 빈 자리(연결·유형 모를 터미널·멈춤·자리·레인상한을 띄우기와 같은 함수로, `load.tsv`) — 옛 기록이면 모름. `hc-fill advise`의 빈 자리는 이 값만 읽는다 |
| `machines[].cordon` | 어느 점검 항목으로든 격리 중이면 `{"since_s": 가장 이른 시작부터 초, "why": "<까닭>; <까닭>…"}`(저장소 읽기는 「<프로젝트> <폴더>: <오류>」, 그 밖은 항목 머리 「원격 쓰기 거절 — 」 등 + 글), 아니면 `null`([격리](../hc-fill/cordon.md)) |
| `machines[].health` | 건강 |
| `machines[].health.sleep` | 잠자기 끔 상태 |
| `machines[].health.keychain` | 키체인 |
| `machines[].health.disk_free_gb` | 디스크 여유(GB) |
| `machines[].health.tailscale` | Tailscale 연결 |
| `machines[].steer` | 걸려 있는 조정(손잡이) |
| `machines[].skip_why` | 이번 바퀴 안 띄운 까닭 |
| `queue` | 대기열 |
| `queue.counts` | 상태별 줄 수 |
| `queue.oldest` | 가장 오래 기다린 줄, 없으면 `null` |
| `queue.oldest.lane` | 레인 이름 |
| `queue.oldest.age_s` | 기다린 초 |
| `queue.oldest.why` | 기다리는 까닭(선행·겹침·자리·거절) |
| `lanes` | 떠 있는 레인마다 한 줄 |
| `lanes[].name` | 레인 이름 |
| `lanes[].machine` | 기계 |
| `lanes[].stage` | 단계(`hc-fill status`와 같은 글) |
| `lanes[].stage_age_s` | 그 단계에 머문 초 |
| `lanes[].last_event` | 마지막 사건 한 줄 |
| `lanes[].done` | 더 할 일 없음(마지막 사건 정리됨·착지, 터미널 살아 있지 않음) |
| `lanes[].open` | 터미널이 살아 있음(또는 살았는지 모름) — 끝난 레인으로 치지 않는다 |
| `lanes[].queued` | 병합 줄에서 정상으로 기다림 — 병합기가 살아 있고 그 제출이 줄에 있거나 도는 묶음의 구성원. 「같은 단계 오래」(lane-long)를 올리지 않는다 |
| `lanes[].pr` | PR 거울이 연 PR `PR #n <주소>`([PR 거울](../hc-fill/pr.md)) — PR이 없으면 칸이 없다 |
| `merge` | 병합 줄(프로젝트마다) |
| `merge[].project` | 프로젝트 |
| `merge[].length` | 줄 길이 |
| `merge[].merger` | 병합기 `alive` · `gone` · `unknown` |
| `merge[].last_round_age_s` | 「마지막 진행」 나이(초 — 이름은 옛 것 그대로) — 병합기 기록의 마지막 진행 시각(`progress_at`)부터. 바퀴가 도는 시각(몇 초마다)도 일지 줄 시각도 아니다. 기록 모름이면 null |
| `merge[].recent_verify_s` | 마지막 착지의 착지 시간(초, 검증 포함) |
| `merge[].skip_ratio` | 최근 착지의 검증 건너뜀 비율(0~1) |
| `merge[].lands` | 최근 착지 수(20번까지) |
| `merge[].skipped` | 그중 검증 건너뜀 수 |
| `merge[].delegated` | 최근 검증 맡김 수(20번까지) |
| `merge[].oldest_age_s` | 줄에 선 가장 오래된 제출의 나이(초, 큐 참조 이름의 초 — `hc-fill status`의 「가장 오래」와 같은 출처) |
| `merge[].oldest_lane` | 그 제출의 레인 |
| `merge[].landing` | 병합기 기록의 단계가 착지(`land` 병합기 기계 · `land-on` 워커)면 맨 앞 레인(기록 `members` 첫째). 아니면·기록 모름이면 빈 값 |
| `merge[].landing_age_s` | 그 착지 단계의 나이(초, 기록 `phase_since`부터) |
| `merge[].phase` | 병합기 기록의 단계(`idle`·`round`·`spec-wait`·`land`·`land-on`·`prev-land` — [병합기 상태 한 장](../hc-lane/merger-state.md)). 기록 모름이면 빈 값(까닭은 `why` — 옛 판 병합기면 「모름(옛 판 병합기 — 다시 띄우면 기록)」) |
| `merge[].phase_why` | 그 단계의 까닭 한 줄(병합기가 쓴 글) |
| `merge[].phase_age_s` | 그 단계의 나이(초) |
| `merge[].rec_age_s` | 기록 나이(초, 병합기가 마지막으로 쓴 때 — 착지 중에도 30초마다 쓴다) |
| `merge[].waits` | 병합기가 기다리는 대상들 — `machine`·`addr`(터미널 주소, 병합기가 못 찾았으면 빈 값)·`role`(추측·몫 i/K·마무리·착지 실행 등 병합기가 쓴 글)·`age_s`(띄운 뒤 나이)·`live`(이번 `hc ls`에서 그 주소가 살아 있으면 `live`, 없으면 `gone`, `hc ls`를 못 읽으면 `?`) |
| `merge[].inner` | 착지 안쪽 단계 한 줄(착지 스크립트가 `HERDR_LAND_PHASE` 파일에 쓴 것 — 예 공개 선점 기다림), 안 쓰면 빈 값 |
| `merge[].inner_age_s` | 그 줄을 병합기가 처음 본 뒤 나이(초) |
| `merge[].land_on` | 단계가 워커 착지(`land-on`)면 그 워커, 아니면 빈 값 |
| `merge[].why` | 모르는 칸의 까닭 |
| `merge[].fails` | 레인 → 지금 줄에 선 그 제출이 든 묶음이 실패한 바퀴 수(병합기 기록 `fails`, 1 이상만). 기록 모름이면 null. 3 이상이면 advise 「거둘 수 있음」(`withdraw`) |
| `deploy` | 배포(기계마다) |
| `deploy[].machine` | 기계 |
| `deploy[].version` | 깔린 판 |
| `deploy[].behind` | origin/main에서 밀린 커밋 수 |
| `deploy[].why` | 모름의 까닭 |
| `watch` | 감시(지휘마다) |
| `watch[].owner` | 지휘 주소 |
| `watch[].alive` | watch가 도는가 |
| `knobs` | 켜져 있는 손잡이 |
| `knobs[].name` | 이름 |
| `knobs[].value` | 값 |
| `knobs[].by` | 누가 |
| `knobs[].why` | 왜 |
| `knobs[].until` | 언제까지 |
| `journal` | 최근 조치(일지 꼬리) |
| `events` | 최근 사건 요약 |
| `notable` | 눈에 띄는 것 — 지금 손댈 것(`hc-fill advise`)의 요약 한 줄씩 |
| `hidden` | 기본 출력이 숨긴 개수, `--all`이면 `null` |
| `hidden.lanes` | 숨긴 끝난 레인(더 할 일 없음, 끝난 지 `state_done`분 넘음) 수 |
| `hidden.watch` | 숨긴 안 도는 지휘 감시 흔적 수 |
| `hidden.deploy` | 숨긴 배포 대상 아닌 기계 수 |
