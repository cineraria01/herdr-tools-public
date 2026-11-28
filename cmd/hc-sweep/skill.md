# hc-sweep — 에이전트 안내 (`hc-sweep --skill`)

## 하는 일

- 기계마다 주기적으로(launchd) 도는 청소·점검이다: 오래 쉰 idle·done worker 터미널 닫기, 레인이 남긴 고아 프로세스 정리, 색인·디스크(Go 캐시·임시 찌꺼기) 쓸기, Tailscale 다시 켜기, 건강 점검(잠자기 끔·키체인), 에이전트가 남긴 ego 작업 공간 닫기(손잡이 `sweep_ego_idle`분 그대로, 0이면 끔 — 사용자 공간은 안 건드림).
- 착지 전 커밋·작업 트리가 남은 레인·배경 실행이 남은 터미널, 배경 하위 에이전트가 도는(대화 기록이 막 바뀐) Claude 터미널, 이름 없는 터미널은 닫지 않는다. 이어 시킬 레인은 keep 목록에 이름을 넣는다.
- 판단이 필요한 것은 사건으로 알린다(디스크 부족·Tailscale·건강). 고치는 것은 지휘·사람이다.

## 흔한 흐름

1. 미리 보기: `hc-sweep --dry-run` — 닫을 것·지울 것과 이유만 찍는다(아무것도 안 바꾼다).
2. 지금 한 바퀴: `hc-sweep` (다른 쓸기가 돌면 종료 75 — 다음 바퀴를 기다린다).
3. 건강 결과 보기: `hc-sweep --health` (재지 않고 기록만, 종료 0 좋음 · 1 나쁨 · 3 모름).
4. 다른 기계: `hc exec --on <기계> -- hc-sweep --dry-run`.

## 상태 보기

- `hc-sweep --health` — 점검마다 좋음·나쁨·모름과 확인한 지 얼마.
- `hc-sweep --dry-run` — 지금 돌면 무엇을 할지.
- 기계 전체 한 장: `hc-fill state --json`의 기계 칸(건강은 모으는 곳이 없으면 모름).
- 기록 새 줄: `hc-lane logs --all`(hc-sweep 기록 포함).

## 조정과 되돌리기

- 고아·색인 최소 나이: `hc-sweep --min-age <분>` (기본 10).
- 문턱(디스크 여유·Go 캐시 상한·Tailscale 켜기 한도·임시 쓸기 알림)은 개인 설정 `sweep.*` 키(docs/hc-sweep/disk-sweep.md·tailscale.md).
- 닫은 터미널은 되살릴 수 없다 — 이어 시킬 레인은 닫히기 전에 일을 보내거나 keep 목록에 넣는다.
- 규칙 판정 자가 시험: `hc-sweep --self-test`.

## 사건을 받으면

- `hc-disk-low`: `hc-sweep --dry-run`으로 지울 것을 보고, 끝난 작업 트리는 `hc-lane gc --repo <저장소> --dry-run`부터. 임시 폴더에 쌓인 옛 찌꺼기는 `hc-sweep --tmp-backlog 48 --dry-run`(종류별 개수·크기, 끝에 `--only '…'` 한 줄)을 보고 지휘가 고른 종류만 `hc-sweep --tmp-backlog 48 --only '<종류>,…'`로 한 번 — `--only` 없이는 안 지운다(docs/hc-sweep/job-tmp.md).
- `hc-sweep-tmp-skip`: 그 기계에서 `hc-sweep --dry-run`으로 건너뜀이 이어지는지, lsof가 왜 느린지 본다.
- `hc-tailscale`: 그 기계 `tailscale status`, 이 기계 `hc machine ls`.
- `hc-health`: 그 기계 `hc-sweep --health`. 키체인 막힘이면 코어를 다시 띄우고, 키체인이 필요한 레인은 다른 기계로.
- `hc-stale-resident`: 옛 판·판 모름으로 도는 상주(건강 점검 길의 상주 판 점검). 글의 「다시 띄우기:」 명령을 그 기계에서 부른다(도구는 끄지 않는다). docs/hc-sweep/resident.md
- `hc-ego-idle`: 알리기만 — 닫힌 공간을 쓰던 에이전트는 같은 목표 이름으로 새 공간을 만들어 이어 간다. docs/hc-sweep/ego-spaces.md
- 사건마다 뜻·흔한 까닭: docs/ops/events.md

## 사람에게 물을 것

- 사용자 파일이 디스크를 차지할 때(지우지 않는다).
- 잠자기 끔을 다시 켜야 할 때(관리자 권한), Tailscale 로그인이 필요할 때.
- 어느 프로세스가 쥐는지 모르는 채 무언가를 지워야 할 때.

## 하지 않을 것

- 찾아낸 프로세스를 죽이지 않는다 — 고아 정리는 hc-sweep이 자기 판정으로만 한다. 손으로는 내가 띄운 정확한 pid만.
- 쓸기 잠금(sweep.lock)·건강 기록 파일을 손으로 지우지 않는다.
- `--only-under`는 시험용이다 — 운영에서 범위를 좁히는 데 쓰지 않는다.
- 설정·다른 앱을 hc-sweep 대신 손으로 바꾸지 않는다 — 알림을 보고 지휘·사람이 정한다.

## 더 읽을 곳

- docs/hc-sweep/disk-sweep.md · docs/hc-sweep/health.md · docs/hc-sweep/orphans.md · docs/hc-sweep/tailscale.md · docs/hc-sweep/index-sweep.md · docs/ops/events.md · docs/ops/agents.md
