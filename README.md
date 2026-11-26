# herdr-tools

여러 기계(맥 하나 + 워커 맥·서버)에서 Herdr로 에이전트 레인을 돌리는 운영 도구 모음이다. 개인 값(기계 이름·경로·계정·지침 본문)은 이 저장소에 없고, 따로 둔 **개인 저장소**(설정 폴더 `HERDR_TOOLS_CONFIG`, 기본 `~/.config/herdr-tools`)에서 읽는다. 꼴은 `config.example/`(가짜 값), 규칙은 `CONTRACT.md`.

도구마다 `cmd/<도구>`의 Go 바이너리 하나다(표준 라이브러리만, 모듈 루트 `go.mod`, 공통 꾸러미 `internal/`). 기계마다 `~/.local/bin/<도구>`가 그 바이너리 자체이고, herdr-sync가 깐다([바이너리 줄](docs/herdr-sync/binary-lines.md)). 셸·파이썬 판과 되돌리기 스위치는 2026-11-22 걷었다(사용자 「셸 판 그냥 지금 즉시 지우고 go 로 된것만 집중 개선 수정하자」) — 옛 판은 태그 `shell-final-20261122`.

| 도구 | 하는 일 | 도는 곳 |
|---|---|---|
| `hc-lane`(+ 곁 `hc-merger`) | 레인 작업 트리 만들기·착지(`land`)·정리(`clean`·`gc`)·push 관문(`guard`)·병합기(`merger`) | macOS 기계 |
| `hc-fill` | 빈 자리에 기다리는 레인을 바로 띄우기 | 맥 |
| `herdr-sync` | 맥의 실제 파일을 두 저장소와 모든 기계에 맞추기 | 맥 |
| `herdr-settings-merge` | `~/.claude/settings.json`의 정한 키만 3-way로 맞추기(herdr-sync가 부른다) | 모든 기계 |
| `herdr-tools-config` | 개인 설정 읽기·개인 값 검사(`lint`) | 맥 |
| `herdr-tools-publish` | 범용 저장소를 기록 없이 공개 저장소로 내보내기(herdr-sync가 부른다) | 맥 |
| `herdr-balance` | Herdr 창에서 지휘 pane 옆 worker 높이 맞추기(`--watch`) | 맥 |
| `hc-launchd-guard` | launchd 작업을 Herdr 터미널에서 돌리는 지킴이([launchd 작업](docs/hc-launchd-guard/guard.md)) | macOS 기계 |
| `hc-sweep`, `ovis-split`, `hc-core-relaunch`, `hc-job-server`, `worker-mac-check`·`worker-mac-setup`, `omc-update` | 끝난 터미널 정리, Ovis 분할, 코어 다시 띄우기·판 맞추기(`--update`·`--all`), 자동 루프 감독(끝났거나 멈춘 루프를 같은 세션으로 다시 띄움), 워커 맥 점검·설정(`worker-mac-setup mesh`: 워커끼리 ssh·hc 연결), OMC 플러그인·omc CLI를 기계마다 같은 판으로(`--all`·`--check`) | 맥·macOS 기계 |
| `herdr-hooks` | Claude Code PreToolUse 훅 다섯(`herdr-hooks <훅>`): 같은 작업 트리 동거 편집 가드(`cotenant-guard`), 무인 루프 질문 창 막기, `hc-lane wait` 분리(&) 실행·셸만 남은 터미널로 `hc send|keys` 막기, 조회로 고른 pid kill 막기, 레인 세션의 셸 파일 고치기·heredoc 커밋·zsh 확장 경로 막기(`lane-no-shell-edit`) | macOS 기계 |

## 설치

- 처음 한 번은 저장소에서 `go run ./cmd/herdr-sync`를 돌린다.
  그러면 herdr-sync 자신과 개인 저장소 `sync.list`의 바이너리 줄 도구가 기계마다 `~/.local/bin`에 깔린다([herdr-sync](docs/herdr-sync/overview.md)).
- 개인 설정은 `config.example/`을 본떠 설정 폴더 `HERDR_TOOLS_CONFIG`(기본 `~/.config/herdr-tools`)에 둔다([설정](docs/herdr-tools-config/config.md)).
- launchd 작업은 `launchd/`의 예시 plist를 손으로 깐다([launchd 작업](docs/hc-launchd-guard/guard.md)).

## 목차

도구·기능 설명은 `docs/<도구>/<기능>.md` 한 파일에 한 주제다(한 문장 한 줄).
README에는 아래 목차 줄만 둔다.
새 기능은 새 파일과 목차 한 줄이고, 목차 줄은 경로 이름순이다(끝에 덧붙이지 않는다 — `tests/repo`의 `TestDocsIndex`가 본다).

- [Go 바탕](docs/go/foundation.md)
- [운영 기록 (ops.tsv)](docs/go/oplog.md)
- [Go 운영 도구](docs/go/ops-tools.md)
- [Go 릴리스 바이너리](docs/go/release-binaries.md)
- [새 판으로 이어 돌기 (selfexec)](docs/go/selfexec.md)
- [Go 작은 도구](docs/go/small-tools.md)
- [hc-core-relaunch: 코어 판 맞추기 (--update)](docs/hc-core-relaunch/update.md)
- [hc-fill: 다른 프로젝트 선행](docs/hc-fill/after-other-project.md)
- [hc-fill: 자동 감사](docs/hc-fill/audit.md)
- [hc-fill: 반송 다시 띄우기](docs/hc-fill/bounce.md)
- [hc-fill: 지시문 복사](docs/hc-fill/brief-copy.md)
- [hc-fill: .qa 자료 싣기](docs/hc-fill/carry.md)
- [hc-fill: 설정](docs/hc-fill/config.md)
- [hc-fill: 선행 사슬](docs/hc-fill/dep-chain.md)
- [hc-fill: 먼저 띄움](docs/hc-fill/early.md)
- [hc-fill: 줄 고치기 (edit)](docs/hc-fill/edit.md)
- [hc-fill: 빈 지시문 거절](docs/hc-fill/empty-brief.md)
- [hc-fill: Go 구현](docs/hc-fill/go-impl.md)
- [hc-fill: 멈춘 레인](docs/hc-fill/hang.md)
- [hc-fill: 놀고 있음 알림](docs/hc-fill/idle.md)
- [hc-fill: 받은편지](docs/hc-fill/inbox.md)
- [hc-fill: 마지막 보고](docs/hc-fill/last-report.md)
- [hc-fill: 띄운 뒤 확인](docs/hc-fill/launch-check.md)
- [hc-fill: 부하 보고 고르기](docs/hc-fill/load-steer.md)
- [hc-fill: 기계 능력](docs/hc-fill/machine-caps.md)
- [hc-fill: 기계 이름 바꾸기](docs/hc-fill/machine-rename.md)
- [hc-fill: 착지 뒤 명령](docs/hc-fill/on-land.md)
- [hc-fill 겹침: 실제 변경 (go-overlap-actual, 2026-11-22)](docs/hc-fill/overlap-actual.md)
- [hc-fill: 겹침](docs/hc-fill/overlap.md)
- [hc-fill: 주인 지휘](docs/hc-fill/owner.md)
- [hc-fill: 주차](docs/hc-fill/park.md)
- [hc-fill: 묶인 기계](docs/hc-fill/pinned.md)
- [hc-fill: PR 거울](docs/hc-fill/pr.md)
- [hc-fill: 우선순위](docs/hc-fill/priority.md)
- [hc-fill: 대기열 파일](docs/hc-fill/queue.md)
- [hc-fill: 거짓 반송 다시 넣기 (requeue)](docs/hc-fill/requeue.md)
- [hc-fill: 한 바퀴](docs/hc-fill/round.md)
- [hc-fill: 줄 ID·채택·끊긴 기계](docs/hc-fill/row-id.md)
- [hc-fill: 늘 돌리기](docs/hc-fill/service.md)
- [hc-fill: 자리 세기](docs/hc-fill/slots.md)
- [hc-fill: --then (같은 일의 앞뒤 단계는 한 레인)](docs/hc-fill/then.md)
- [hc-fill: 기계 닿지 않음](docs/hc-fill/unreach.md)
- [hc-fill (빈 자리에 기다리는 레인을 바로 띄우기, 맥)](docs/hc-fill/usage.md)
- [hc-fill: 병합 줄에서 거두기 (withdraw)](docs/hc-fill/withdraw.md)
- [hc-job-server (자동 루프를 Herdr 터미널 하나로 지키는 감독)](docs/hc-job-server/usage.md)
- [hc-lane 사건 (report·events·wait — 레인이 적고 지휘는 그것만 읽는다)](docs/hc-lane/events.md)
- [hc-lane gc (끝난 레인 정리)](docs/hc-lane/gc.md)
- [hc-lane: Go 구현](docs/hc-lane/go-impl.md)
- [hc-lane land: 착지 표식·착지 안전](docs/hc-lane/land.md)
- [hc-lane 띄우기 (지시문 사본·신뢰 질문·시작 확인·fetch)](docs/hc-lane/launch.md)
- [hc-lane 레인 임대](docs/hc-lane/lease.md)
- [hc-lane logs (도구 기록 모으기)](docs/hc-lane/logs.md)
- [hc-lane 병합기: 묶음](docs/hc-lane/merger-batch.md)
- [hc-lane 병합기: 추측 검증 (병합기 병렬 검증)](docs/hc-lane/merger-spec-verify.md)
- [병합기 상태 한 장 (merger-state)](docs/hc-lane/merger-state.md)
- [hc-lane 병합기 (제출·merger, 최소판)](docs/hc-lane/merger.md)
- [hc-lane park (주차)](docs/hc-lane/park.md)
- [hc-lane rerere (같은 충돌 다시 풀기)](docs/hc-lane/rerere.md)
- [hc-lane stats (착지 통계)](docs/hc-lane/stats.md)
- [hc-lane watch (늘 도는 감시)](docs/hc-lane/watch.md)
- [launchd 작업: 지킴이(`hc-launchd-guard`)로 Herdr 터미널에서](docs/hc-launchd-guard/guard.md)
- [hc-sweep: 디스크 쓸기](docs/hc-sweep/disk-sweep.md)
- [hc-sweep: 건강 점검 (잠자기 끔·키체인)](docs/hc-sweep/health.md)
- [hc-sweep: 색인 쓸기](docs/hc-sweep/index-sweep.md)
- [작업별 임시 폴더 (hc가 띄운 작업마다 자기 TMPDIR, 끝나면 통째로 지움)](docs/hc-sweep/job-tmp.md)
- [고아 프로세스 (레인이 남긴 배경 프로세스)](docs/hc-sweep/orphans.md)
- [hc-sweep: 상주 판 점검 (옛 판으로 도는 상주 찾기)](docs/hc-sweep/resident.md)
- [hc-sweep: Tailscale 점검](docs/hc-sweep/tailscale.md)
- [hc-sweep: 시험 고아 점검 (부모 없이 남은 시험 프로세스)](docs/hc-sweep/test-orphans.md)
- [herdr-hooks: 훅](docs/herdr-hooks/hooks.md)
- [herdr-sync: 바이너리 줄](docs/herdr-sync/binary-lines.md)
- [herdr-sync: 충돌과 푸는 법](docs/herdr-sync/conflicts.md)
- [herdr-sync: 배포 기록·상태·되돌리기·착지 뒤 배포](docs/herdr-sync/deploy.md)
- [herdr-sync: 이 기계 이름](docs/herdr-sync/local-name.md)
- [herdr-sync: 맞추지 않는 것 (기계마다 다른 것)](docs/herdr-sync/not-synced.md)
- [herdr-sync: 정본은 맥의 실제 파일, 두 저장소는 기록](docs/herdr-sync/overview.md)
- [herdr-sync: 나무 줄 — 폴더 통째로 (스킬 폴더 등)](docs/herdr-sync/tree-lines.md)
- [herdr-sync: 사용법 (맥에서만)](docs/herdr-sync/usage.md)
- [설정](docs/herdr-tools-config/config.md)
- [공개 저장소로 내보내기 (herdr-tools-publish)](docs/herdr-tools-publish/publish.md)
- [omc-update: OMC 플러그인과 omc CLI를 기계마다 같은 판으로](docs/omc-update/usage.md)
- [지금 손댈 것 (hc-fill advise)](docs/ops/advise.md)
- [에이전트가 쓰는 관리 도구 — 설계안](docs/ops/agent-managed-design.md)
- [운영 에이전트 (hc-fill agent)](docs/ops/agent.md)
- [사람과 에이전트가 같은 명령으로 (--skill · --json · --dry-run)](docs/ops/agents.md)
- [운영 사건 목록 (받으면 할 수 있는 것)](docs/ops/events.md)
- [운영 일지 (hc-fill journal)](docs/ops/journal.md)
- [손잡이 (hc-fill knob)](docs/ops/knobs.md)
- [상태 한 장 (hc-fill state)](docs/ops/state.md)
- [파일 크기: 줄 수 톱니바퀴](docs/repo/line-budget.md)
- [시험](docs/repo/tests.md)
