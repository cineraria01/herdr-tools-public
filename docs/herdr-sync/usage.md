# herdr-sync: 사용법 (맥에서만)

```sh
herdr-sync --check              # 고치지 않고 모든 기계를 한 표로. 다름·충돌이 있으면 종료 1
herdr-sync                      # 맞춘다
herdr-sync box-1 box-2          # 고른 기계만
herdr-sync --take <기계>:<경로>  # 충돌을 맥 값으로 덮는다 (그 기계에 백업)
herdr-sync --pull <기계>:<경로>  # 그 기계 값을 맥 실제 파일로 가져온 뒤 맞춘다
herdr-sync --status             # 기계마다 깔린 판·밀린 커밋·병합 줄 (배포 기록·되돌리기·스스로 배포는 deploy.md)
```

- `--check` 표의 첫 줄은 맥 ↔ 두 저장소다: `맥: 저장소가 새로움 n개 (<저장소 경로> …) — herdr-sync로 깐다`(레인이 착지했는데 맥에 아직 안 깔림), `맥: 저장소보다 새로움 n개 (…) — 커밋 안 됨?`(맥에서 고쳤는데 아직 안 모음), 다 맞으면 `맥: 저장소와 같음`. 기계 줄은 **맥 실제 파일**과 견준 것이라, 맥이 저장소보다 뒤면 기계 줄 끝에 `(맥 판 기준 — 저장소 새 판은 아직)`이 붙는다 — 그때 `같음`은 「맥 옛 판과 같음」이지 「저장소 새 판이 퍼졌음」이 아니다.
  착지 뒤 퍼졌는지 볼 때는 맥 줄부터 본다(2026-11-19 착지 직후 `--check`의 기계 줄 `같음`만 보고 「이미 퍼졌다」고 오판했다).
- 출력·기록의 `맥`은 이 기계 이름이다 — 개인 설정 `hc-fill.tsv`의 `local` 줄이 있으면 그 이름으로 적는다([이 기계 이름](local-name.md)).
- 경로는 `sync.list`의 맥 경로(`~` 아래)다.
  예: `.claude/docs/notes.md`, 지침 본문은 `.claude/CLAUDE.md#body`, Codex 공통 구역은 `.codex/AGENTS.md#codex-shared`, 링크는 `.claude/skills/my-skill@`, 설정 키는 `.model`, 설정 훅은 `hook:PreToolUse:cotenant-guard`(herdr-settings-merge의 판정 이름), 나무 안 파일은 `.agents/skills/my-skill/scripts/run.sh`.
- `<기계>:*`는 그 기계의 충돌 전부, `<기계>:<폴더>/**`는 그 나무 줄의 그 기계 충돌 전부다(`repo:<폴더>/**`도 된다).
- Codex 공통 구역과 설정 키는 `--pull`을 받지 않는다(구역은 맥 지침에서 다시 만들고, 설정은 맥 `settings.json`을 고친다).
  `--take`를 쓴다.
- 저장소 위치: 범용 `HERDR_TOOLS_REPO`(기본 `~/Documents/app/herdr-tools`), 개인 `HERDR_TOOLS_CONFIG`(기본 `~/.config/herdr-tools`).
  둘 다 git·main 브랜치·깨끗함이어야 한다.
  개인 저장소(`sync.list`)가 없으면 종료 2, 아무것도 쓰지 않는다.
- 대상 이름: `all`(모든 기계) · `macos`(uname이 Darwin) · `local`(맥만) · 기계 이름을 쉼표로. 옛 이름 `all9`·`mac7`·`mac-only`가 보이면 새 이름을 알리고 종료 2(옛 판은 그것을 기계 이름으로 읽어 아무 데도 안 깔고 「같음」으로 보였다).
- 순서: 두 저장소 `git pull --ff-only` → `sync.list` 읽기(`sync-lint` — 걷힌 셸 판 줄·틀린 바이너리 줄·딸린 줄 빠짐이면 종료 2) → `--pull` 처리 → 개인 설정 `sync.prebuild`(있으면 `sh -c`로, 예: 맥 지침에서 Codex 공통 구역 다시 만들기. 실패하면 멈춤) → 맥 ↔ 두 저장소 → 공개 내보내기(곁 `~/.local/bin/herdr-tools-publish`) → 기계 조회 → 릴리스 → 맥 바이너리 → 기계마다 파일 → 바이너리 → 설정(`~/.local/bin/herdr-settings-merge`) → 지울 것 → 마지막에 기계마다 한 줄 `같음` / `다름 (묶음…)` / `충돌 n건`. 저장소에서 지침 본문을 깔았으면 prebuild와 맥 ↔ 저장소를 한 번 더 돈다.
- sh를 못 띄우는 기계(Windows 등, hc 오류 `spawn_failed`·`cannot start `)는 `<기계>: 건너뜀 (sh 없음 — Windows 등)`, uname이 Darwin·Linux가 아닌 기계(Git sh의 `MINGW64_NT-…`·`MSYS_NT-…` 등)는 `<기계>: 건너뜀 (OS <uname>)` 한 줄만 찍고 아무것도 쓰지 않는다(`--take`·`--pull`로 집어도).
  둘 다 종료 코드에 넣지 않는다.
  진짜 연결 실패·시간 초과는 지금처럼 `다름 (연결)`·종료 1이다.
- 모름은 멈춤이다(2026-11-22 gofix-ops): git 조회(`status`·`rev-list` 등)가 실패하거나 시간을 넘기면, 맥·저장소·기계의 파일을 못 읽으면(권한 등), 기계 조회가 끝 줄(OS)까지 오지 않으면 그것을 깨끗함·없음·성공으로 읽지 않고 멈추거나 그 기계를 연결 실패로 친다.
  모음 중(스테이징과 커밋 사이)에 죽으면 다음 실행이 `~/.local/state/herdr-sync/repo-journal.tsv`로 자기 스테이징만 되돌리고 다시 모은다(그 뒤 남이 손댔으면 멈춤 — 확인한 뒤 일지를 지운다).
  앞 실행이 남긴 git 자식 무리는 거두고, 거두지 못하면 75. `sync.list`의 끝 줄바꿈 없음·줄 끝 CR·뿌리 밖 경로(절대 경로·`..`)는 종료 2. 기계마다 도는 조각은 sh다(`cmd/herdr-sync/agent.sh`를 임베드 — 원격 기계에 Go가 없어도 된다).
- **배포 한 번은 커밋 하나** (2026-11-24 sync-pin-commit — 배포 1~2분 사이에 기본 가지에 착지가 들어오자 맥은 앞 커밋으로 지은 판, 릴리스 없음, 나머지 기계는 `다름 (바이너리)`로 끝난 반쪽 배포).
  공개 내보내기가 검사한 커밋이 범용 저장소 HEAD보다 앞서 있으면(그사이 착지) 저장소를 그 커밋으로 fast-forward 하고(`범용 저장소 <앞> → <새> …` 한 줄) 맥 ↔ 저장소를 한 번 더 돈다.
  그 뒤로는 짓기·릴리스 이름·맥과 기계에 까는 판·`같음/다름`·배포 기록이 모두 그 커밋이다(기계는 맥에 깐 소스와 그 커밋의 릴리스를 받는다).
  끝 요약 뒤에 기본 가지가 그새 움직였으면 `배포한 판 <커밋> — 기본 가지는 그새 <커밋>로 움직였다(다음 배포에)` 한 줄(실패 아님, 종료 코드 그대로), 다시 보지 못하면 `… 움직였는지 모름` 한 줄.
  어느 기계가 그 판을 못 깔면(릴리스 없음·그 기계 소스가 다름 등) 그 기계 줄이 `다름 (바이너리) — 못 받은 판: <도구> 바이너리를 깔지 못했다: <까닭>`이고 종료 1이다.
- 첫 `git pull --ff-only`가 같은 체크아웃의 다른 git(대기열 바퀴의 fetch 등)과 잠금(`.lock`·`cannot lock ref`)으로 부딪히면 짧게 몇 번(최대 5번) 다시 하고, 그래도 안 되거나 진짜 갈라짐이면 git이 낸 까닭 한 줄을 붙여 멈춘다(2026-11-24 deploy-log-rollback).
- 다른 herdr-sync가 돌고 있으면 바로 끝난다(종료 75).
  맥이 아닌 곳(`~/Applications/Herdr.app`·`/Applications/Herdr.app`이 없거나 `hc machine ls`에 기계가 없는 곳)에서는 종료 2.
- 설정은 대상 기계의 `herdr-settings-merge`가 `~/.local/share/herdr-sync/keys.json`(개인 저장소 `settings/keys.json`을 깐 것)과 `HERDR_MACHINE`으로 판정한다.
  그 도구가 판정 줄 없이 실패하면 `다름 (도구문제:설정)`으로 보인다(충돌과 다르다).
  `keys.json` `exceptions.<기계>`에 훅 판정 이름(`hook:PreToolUse:cotenant-guard`)을 적으면 그 기계의 그 훅은 쓰지도 지우지도 비교하지도 않는다(`<기계> 예외 hook:…` — 예: 훅 바이너리가 깔리지 않는 리눅스 기계).
- 저장소에 커밋하지 않은 변경이 있으면 아무것도 하지 않고 멈춘다.
  표식 없는 `~/.codex/AGENTS.md`는 충돌로 보이고, `--take`하면 codebase-memory-mcp 블록만 남기고 구역을 넣는다(백업).
