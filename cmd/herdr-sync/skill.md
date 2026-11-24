# herdr-sync — 에이전트 안내 (`herdr-sync --skill`)

## 하는 일

- 맥(지휘 기계)의 실제 파일을 정본으로 두 저장소(범용 도구·개인 설정)와 hc에 등록된 모든 기계에 맞춘다 — 지침·훅·설정 키·도구 바이너리.
- 무엇을 맞추는지는 개인 저장소의 `sync.list`가 정한다. 맥에서만 돈다(다른 기계에서는 종료 2).
- 배포 기록·상태·되돌리기·착지 뒤 스스로 배포 손잡이도 준다. 배포할지·되돌릴지는 지휘가 정한다.

## 흔한 흐름

1. 범용 저장소에 착지 → 착지 뒤 명령(`herdr-sync --landed`)이 「배포할 것 <k>커밋」 사건을 보낸다.
2. 보기: `herdr-sync --status` (기계마다 깔린 판·밀린 커밋·병합 줄).
3. 미리 보기: `herdr-sync --check` (고치지 않고 비교만 — 다름·충돌이 있으면 종료 1).
4. 배포: `herdr-sync --why "<까닭>"` (모든 기계) 또는 `herdr-sync <기계> --why "<까닭>"`.
5. 문제가 생기면 되돌리기: `herdr-sync --at <커밋> --check` 로 본 뒤 `herdr-sync --at <커밋> --why "<까닭>"`.

## 상태 보기

- `herdr-sync --status [<기계>…]` — 고치지 않는다. 닿지 못한 기계는 「모름」(같음으로 읽지 않는다).
- `herdr-sync --log --json` — 배포 기록 마지막 줄들(칸: docs/ops/agents.md). 사람용은 `herdr-sync --log 20`.
- `herdr-sync --check` — 맞출 것·충돌 목록(첫 줄 「맥:」은 맥 ↔ 저장소 방향).
- 출력·배포 기록의 「맥」 자리는 이 기계 이름이다 — 개인 설정 `hc-fill.tsv`의 `local` 줄이 있으면 그 이름, 옛 기록의 「맥」도 같은 기계로 읽는다(docs/herdr-sync/local-name.md).
- `herdr-sync --auto` — 스스로 배포 켬·끔(인자 없이 보기만).
- 대기열 쪽에서 한 장으로: `hc-fill state --json`의 배포 칸.

## 조정과 되돌리기

- 충돌(그 기계에서 따로 고침): 맥 값으로 덮기 `herdr-sync --take <기계>:<경로>`, 그 기계 값을 맥으로 가져오기 `herdr-sync --pull <기계>:<경로>`. 덮은 직전 판은 그 기계 `~/.local/state/herdr-sync/bak/<시각>/`에 남는다.
- 되돌리기는 도구 바이너리만(`--at`) — 지침·설정은 개인 저장소를 git으로 되돌린 뒤 보통 배포.
- 스스로 배포: `herdr-sync --auto on --why "<까닭>"` · 끄기 `herdr-sync --auto off --why "<까닭>"`.
- 스스로 배포가 공개 단계(내보내기·릴리스 올림)까지 하게: `herdr-sync --auto-publish on --why "<까닭>"`(기본 끔 — 끄면 릴리스가 없는 커밋은 배포하지 않는다).
- 배포·되돌리기는 운영 일지(`hc-fill journal`)에 되돌리는 `--at` 명령과 함께 남는다 — `--why`를 붙인다.

## 사건을 받으면

- `on-land-deploy` 중간 「배포할 것 <k>커밋」: `herdr-sync --status`로 보고 배포할지 정한다(병합 줄이 남았으면 기다려도 된다).
- 막힘 「스스로 배포 못 함: 이 커밋의 릴리스가 없다」: 아무 데도 안 깔렸다 — `herdr-sync --why "<까닭>"`로 배포하거나 `--auto-publish on`.
- 막힘 「스스로 배포 실패 (종료 n)」: `herdr-sync --log 5`와 `herdr-sync --check`로 어느 기계가 실패·충돌인지 본다.
- 충돌: 그 기계에서 누가 왜 고쳤는지 모르면 덮기 전에 사람에게.
- 종료 75(다른 herdr-sync가 돎): 기다렸다가 같은 줄을 다시.

## 사람에게 물을 것

- 충돌을 `--take`로 덮어 그 기계의 따로 고친 값을 버리는 것(누가 왜 고쳤는지 모를 때).
- `--pull`로 기계 값을 정본(맥·저장소)에 들이는 것 — 모든 기계로 퍼진다.
- 개인 값(기계 이름·경로·계정)이 범용 저장소에 들어갈 때 — 도구가 거절한다, 우회하지 않는다.
- 공개 저장소로 내보내기(바깥 공개).

## 하지 않을 것

- 찾아낸 프로세스를 죽이지 않는다 — 내가 띄운 정확한 pid만(잠금·자식 무리 표식은 도구가 맡는다).
- `hc file write`로 손 복사하지 않는다 — 맞출 것은 `sync.list`에 한 줄.
- 로그인·계정 파일을 기계끼리 복사하지 않는다.
- `--check` 없이 처음 보는 충돌을 덮지 않는다.

## 더 읽을 곳

- docs/herdr-sync/usage.md · docs/herdr-sync/deploy.md · docs/herdr-sync/conflicts.md · docs/herdr-sync/overview.md · docs/ops/agents.md
- 사용법 전부: 맥에서 모르는 인자로 부르면 사용법을 오류 출력에 찍고 종료 2
