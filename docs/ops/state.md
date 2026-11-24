# 상태 한 장 (hc-fill state)

- 기계·대기열·레인·병합 줄·배포·감시·손잡이를 명령 하나로 보인다(2026-11-24 ops-state, 방향은 [설계안](agent-managed-design.md) 4-1).
- 왜: 전체를 보려면 지휘 에이전트가 명령 예닐곱 개(대기열 status·ls, 기계별 원격 호출, 병합기 일지, 원격 참조, 도구 판)를 손으로 맞춰야 했다.
- 사람용 `hc-fill state`와 기계가 읽는 `hc-fill state --json`은 같은 사실을 담는다.
- 읽기만 한다(잠금·상태 파일 쓰기 없음). 모르는 값은 모름(json `null`과 `why`)이고 0·정상으로 적지 않는다.
- 판단은 하지 않는다. 「눈에 띄는 것」은 판단이 필요해 보이는 사실의 목록일 뿐이고, 조치는 지휘가 정한다.

- 칸 이름은 시험 `internal/opsstate/state_test.go`(⑤)가 아래 「json 칸」 표와 맞춘다 — 칸을 더하면 표에 한 줄 더한다.

## 쓰는 법

- `hc-fill state` 사람용 표, `hc-fill state --json` 압축 json 한 줄. 인자는 이 둘뿐이다.
- 지휘 에이전트는 `--json`의 `fingerprint`를 기억해 두고, 다음에 같으면 나머지를 다시 읽지 않는다.
- `notable`이 비어 있지 않으면 그 줄부터 본다. 조치는 손잡이([손잡이](knobs.md))·`hc-fill machine`·`prio`·`retry`·`merger restart` 같은 기존 명령으로 한다.

## 어디서 모으나

- 바퀴가 남긴 기록을 다시 쓴다(원격을 더 부르지 않는다). 나이는 `sources[]`의 `tick` 줄(마지막 바퀴가 끝난 뒤 초).
  - 기계 부하·코어·쓴 자리·안 띄운 까닭: `<DATA>/load.tsv`([부하](../hc-fill/load-steer.md)). 자리 상한·레인 상한은 설정 `machine` 줄과 `seats@` 손잡이.
  - 병합기 살아 있음·줄 길이: `<DATA>/merger-alive.tsv`(hc-fill 바퀴가 적는다).
  - 레인 마지막 사건·최근 사건: `<DATA>/last.tsv`([마지막 보고](../hc-fill/last-report.md)). 그 사건 시각부터 지금까지가 `stage_age_s`다.
  - 대기 까닭: 띄우기 거절 기록 · `overlap.tsv` 겹침 · `need.tsv` 능력 · 대기열의 선행 칸 · 그 밖은 「자리·차례」(기계별 까닭은 `machines[].skip_why`).
- 이번에 부르는 것(`sources[]`의 `hc` 줄 — 호출 수·초): `hc ls --json`(레인 살아 있음·감시 터미널) · `hc machine ls --json`(기계 연결). 둘 다 이 기계 코어에 묻는 것이다.
- 배포(`deploy` 줄): herdr-sync 배포 기록 `~/.local/state/herdr-sync/deploy.tsv`에서 기계마다 결과가 같음·씀인 마지막 줄의 커밋(「기록상 판」)과, 범용 저장소(`HERDR_TOOLS_REPO`, 없으면 herdr-sync와 같은 기본 자리)의 `git rev-list --count <판>..origin/main`(fetch 없이 — 판마다 한 번).
  기계의 실제 바이너리를 보려면 `herdr-sync --status`(기계마다 원격 호출)를 쓴다.
- 손잡이: 공통 손잡이 저장의 살아 있는 줄. 최근 조치: 운영 일지 24시간 안 마지막 5줄([일지](journal.md)).

## 모름

- 마지막 바퀴에 그 기계 부하를 못 읽었으면(읽은 때가 바퀴 끝보다 5분 넘게 앞) `load1`·`cores`·`load_per_core`는 `null`이고 치우침에도 안 쓴다.
- 닿지 못한 기계(`hc machine ls`에서 연결 아님)는 `reach: unknown`·`why: 연결 안 됨`이고 잰 칸은 모두 `null`이다 — 바퀴 기록에 지난 부하가 있어도 지금 값으로 적지 않는다.
- `hc machine ls`를 못 읽으면 이 기계 말고는 모두 `연결 모름`. `hc ls`를 못 읽으면 레인 단계가 「모름(hc ls 못 읽음)」이고 `lanes`·`watch[].alive`는 `null`.
- 아직 모으는 곳이 없는 칸은 `null`이다:
  - `machines[].health` 넷(잠자기·키체인·디스크·Tailscale) — 기계 건강 점검이 기록을 남기면 채운다.
  - `merge[].last_round_age_s`·`recent_verify_s`·`skip_ratio` — 병합기 기계의 일지에만 있다(`merge[].why`에 적힌다).
- `stage_age_s`는 마지막 사건 시각(`last.tsv`는 시:분만)에서 잰다. 하루 넘게 머문 단계는 하루 미만으로 읽힌다.

## 상태 지문

- 판단 칸만으로 만든 12자: 기계(닿음·까닭·에이전트 수·자리·레인·상한·건강·조정·안 띄운 까닭) · 대기열(상태별 수·가장 오래 기다린 줄과 까닭) · 레인(이름·기계·단계·마지막 사건) · 병합 줄(길이·병합기) · 배포(판·밀림·까닭) · 감시 · 손잡이 · 최근 조치·사건 · 눈에 띄는 것.
- 넣지 않는 것: 만든 시각, 재료 나이, 머문 시간·기다린 시간, 1분 부하·코어당 부하, 검증 시간·건너뜀 비율. 글 칸(눈에 띄는 것·안 띄운 까닭)은 숫자를 빼고 넣는다 — 「95분」이 「96분」이 되어도 같은 지문이다.
  레인 단계 글은 경과 분(「감사 대기(자리 없음 N분)」)만 빼고 넣는다 — 감사 차수·sha는 판단 칸이라 남는다.
- 시간만으로 바뀌는 길 하나: 최근 조치는 24시간 창의 꼬리라, 새 조치가 없어도 오래된 줄이 창 밖으로 빠지면 지문이 바뀐다(보이는 사실도 실제로 바뀐다).
- 최근 사건은 `last.tsv` 시:분으로 늦은 것 먼저 세운다 — 자정을 넘으면 차례가 어긋난다.

## 눈에 띄는 것

규칙으로 조치하지 않고 사실만 적는다. 문턱은 손잡이다(`hc-fill knob set <이름> <값> --why …`).

| 사실 | 문턱 손잡이 |
|---|---|
| 레인이 같은 단계에 오래(착지 단계는 빼고) | `state_stage`(분, 기본 90, 0이면 끔) |
| 코어당 부하 치우침 — 바쁜 기계와 한가한 기계가 함께 | `state_hot`·`state_cool`(%, 기본 200·50, `state_hot 0`이면 끔) |
| 배포가 origin/main에서 밀림 | — (1커밋부터) |
| 기계·배포·재료를 모름, 병합기가 살아 있지 않음, 열린 레인이 있는 지휘의 감시가 안 돎·모름 | — |

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
| `machines[].live` | 살아 있는 에이전트 수 |
| `machines[].seats` | 자리 상한 |
| `machines[].lanes` | hc-fill이 띄운 레인 수 |
| `machines[].lane_cap` | hc-fill 레인 상한 |
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
| `merge` | 병합 줄(프로젝트마다) |
| `merge[].project` | 프로젝트 |
| `merge[].length` | 줄 길이 |
| `merge[].merger` | 병합기 `alive` · `gone` · `unknown` |
| `merge[].last_round_age_s` | 병합기 마지막 바퀴 나이(초) |
| `merge[].recent_verify_s` | 최근 착지의 검증 시간(초) |
| `merge[].skip_ratio` | 최근 착지의 검증 건너뜀 비율(0~1) |
| `merge[].why` | 모르는 칸의 까닭 |
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
| `notable` | 눈에 띄는 것 — 판단이 필요해 보이는 사실 |
