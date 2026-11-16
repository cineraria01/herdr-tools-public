# herdr-tools 계약

## 목표
1. **명령 하나**: 맥에서 고치고 `herdr-sync`를 돌리면 모든 기계가 맞는다. `herdr-sync --check`는 고치지 않고 모든 기계를 한 표로 보인다. 다름이나 충돌이 하나라도 있으면 종료 1이다.
2. **정본은 맥의 실제 파일**이고, 두 저장소는 그 기록이다.
   - 범용 저장소(이 저장소, `HERDR_TOOLS_REPO`): 도구·훅·시험·`config.example/`. 개인 값이 0줄이다.
   - 개인 저장소(`HERDR_TOOLS_CONFIG`, 기본 `~/.config/herdr-tools`, 비공개): `sync.list`·`settings/keys.json`·`config`·`hc-fill.tsv`·`private-patterns.txt`·지침 본문·개인 도구.
   - 어느 저장소든 레인이 고쳐 착지하면, 다음 `herdr-sync`가 그것을 맥에 깔고 기계에 맞춘다.
   - 공개 저장소(설정 `publish.repo`)는 범용 저장소 `origin/main` 트리만 기록 없이 받는다(`herdr-tools-publish`, herdr-sync가 맞춘 뒤 부름). 겹치는 경로(대소문자·정규화)나 글이 아닌 블롭(NUL·UTF-8 아님)이 있거나 블롭 내용(이진 포함, 블롭마다)·경로·링크 대상·공개 커밋 작성자에 개인 값·비밀 꼴이 걸리거나 검사를 못 하면 내보내지 않는다. 공개 트리 sha는 비공개 트리 sha와 같다(attributes를 거치지 않음).
3. **충돌 없음**: 어느 쪽 변경도 말없이 덮지 않는다(아래 「3-way 규칙」).
4. **개인 값이 새지 않음**: 범용 저장소의 모든 파일과 범용 저장소로 모을 맥 값은 `herdr-tools-config lint`(개인 저장소 `private-patterns.txt`)에 걸리지 않아야 한다. 검증(`.claude/herdr-verify.sh`)과 herdr-sync 모음이 이것을 본다.

## 나누는 선
| 갈 곳 | 무엇 |
|---|---|
| 범용 | 기계 이름·경로·계정·프로젝트 이름 없이 동작하는 도구. 개인 값이 필요하면 `herdr-tools-config get <키> [<기본값>]`·`file <이름>`으로 읽고, 「없으면 그 기능을 안 한다」는 기본값을 도구가 준다. 시험 예시 이름은 `w1`·`lx`·`box-1`·`proj`·`example-…`처럼 중립으로 |
| 개인 | 사용자 지침 본문, 무엇을 어느 기계에 깔지(`sync.list`·`keys.json`), 특정 프로젝트·계정 풀 전용 도구, 개인 이름이 든 launchd 작업 |

- 설정 꼴: `$HERDR_TOOLS_CONFIG/config`에 한 줄 `<키> <값>`(값은 줄 끝까지, 앞뒤 빈칸 뺌), `<키>` 혼자면 빈 값, `#` 줄은 주석. 키 `^[a-z]+\.[a-z0-9_]+$`, 같은 키 두 번은 종료 2. 키 목록은 `config.example/config`.
- 없으면 잘못 동작하는 것(herdr-sync의 `sync.list`)은 기본값 없이 멈추고 아무것도 쓰지 않는다.

## 맞추는 것: 개인 저장소의 `sync.list`
한 줄에 하나씩 `묶음  맥 경로  저장소 경로  대상`. 예시는 `config.example/sync.list`.

| 칸 | 꼴 |
|---|---|
| 묶음 | 표의 이름(아무 글자). `지움`이면 지울 것 |
| 맥 경로 | `~` 아래 상대 경로. 이름 자리의 `*`는 맥과 저장소에 있는 파일 모두. `<파일>#body`는 `<!-- User customizations -->` 줄 아래 구역, `<파일>#codex-shared`는 `<!-- codex-shared:start -->`·`<!-- codex-shared:end -->` 사이 구역, `<경로>@`는 심볼릭 링크 |
| 저장소 경로 | `tools:<경로>`는 범용 저장소 안, 그 밖은 개인 저장소 안. 링크 줄은 링크가 가리킬 곳(`~` 아래). 지움 줄은 `-` |
| 대상 | `all` · `macos`(uname Darwin) · `local`(맥만) · 기계 이름을 쉼표로. 옛 이름 `all9`·`mac7`·`mac-only`는 종료 2 |

- 지움 줄은 그 기계(대상이 무엇이든 맥 포함)에 있으면 백업 폴더로 옮긴다.
- **맞추지 않는 것**(기계마다 다른 것)은 README 「맞추지 않는 것」.

## 3-way 규칙 (충돌 없음)
- 각 대상 기계는 `~/.local/state/herdr-sync/base.tsv`를 가진다. 내용은 경로(또는 구역·설정 키)와, herdr-sync가 마지막으로 써 준 값의 cksum이다.
- 판정은 파일·구역·키마다 한다.
  - 현재 값 = 맥 값이면 **같음**이다.
  - 현재 값 = base이면 그 기계가 손대지 않은 것이므로, 맥 값으로 **씀**이다. 백업을 남기고 base를 고친다.
  - 그 밖은 **충돌**이다. 쓰지 않고 `충돌 <기계> <경로>`와 다른 줄 수를 보인다. `--take <기계>:<경로>`는 맥 값으로 덮고(백업), `--pull <기계>:<경로>`는 그 기계 값을 맥 실제 파일로 가져온 뒤 다시 맞춘다. 경로 자리 `*`는 그 기계 충돌 전부.
- 맥 ↔ 저장소 사이도 같은 규칙이다. base는 맥 `~/.local/state/herdr-sync/repo-base.tsv`(열쇠는 맥 경로)이고, 저장소는 줄마다 범용 또는 개인이다.
  - 맥만 바뀌었으면 그 줄의 저장소로 **모음**이다. 저장소마다 따로 커밋하고 `HERDR_LAND=1 git push`한다. 범용 저장소로 모을 값이 개인 값 검사에 걸리면 `충돌 저장소 <경로> (개인 값)`, 검사를 못 하면(`$HERDR_TOOLS_CONFIG/private-patterns.txt`에 패턴 줄이 없음·도우미 실패) `(개인 값 검사 못 함…)` 충돌이고, 둘 다 `--take repo:`로도 넘기지 않는다.
  - 저장소만 바뀌었으면(레인 착지) 맥에 **깖**이다. 백업을 남긴다.
  - 둘 다 바뀌었으면 **충돌**이고 멈춘다. 충돌이 하나라도 있으면 어느 저장소에도 쓰지 않는다.
  - herdr-sync는 처음에 두 저장소를 확인(git·main·깨끗함)하고 `git pull --ff-only`로 최신으로 만든다. 개인 저장소가 없으면 범용 저장소도 건드리지 않고 종료 2.
- 개인 설정 `sync.prebuild`가 있으면 맞추기 전에 `sh -c`로 돌린다. 실패하면 멈춘다.
- 백업은 `~/.local/state/herdr-sync/bak/<YYYYMMDD-HHMMSS>/<경로>`에 남긴다.
- 잠금: 맥에서 herdr-sync는 한 번에 하나만 돈다(`lockf`, 종료 75). 맥이 아닌 곳에서 부르면 「맥에서만」을 찍고 종료 2.
- 쓰기는 임시 파일에 쓴 뒤 바꿔 넣는다. 반쯤 쓴 파일을 남기지 않는다. 실행 권한은 맥 값을 따른다.
- 원격 실행은 `hc exec [-i] --on <기계> -- sh -c '…'`(POSIX sh, dash 포함)로 한다.

## 설정 (`herdr-settings-merge`, 대상 기계에서 돈다)
- 개인 저장소 `settings/keys.json`에는 맞출 키 이름·훅 표식·참고 키·기계별 예외만 둔다(꼴 `config.example/settings/keys.json`). 값은 매번 맥 `settings.json`에서 읽는다.
  - `values`: 맞출 키(`.model`, `.env.X` 꼴).
  - `hooks`: 이벤트마다 command에 이 글자가 든 훅 항목만 맞춘다. 다른 훅 항목은 건드리지 않는다.
  - `refs`: 맞추지 않고 `--check`에서 「참고」로만 보일 키.
  - `exceptions`: 기계 이름마다 그 기계 값을 그대로 둘 키.
- 문자열 값 안의 맥 `$HOME`은 대상 기계의 `$HOME`으로 바꿔 넣는다.
- 키마다 3-way다. base는 대상 기계의 `~/.local/state/herdr-sync/settings-base.json`이다.
- 쓰기 전에 `settings.json`을 백업하고, jq로 고친 결과가 올바른 JSON일 때만 바꿔 넣는다.
- 입출력 약속:
  - 표준 입력: `{"values":{"<키>":<값>,…},"hooks":[{"event":"PreToolUse","matcher":"…","command":"…"},…]}`
  - 인자: `[--check] [--take <키|*>]`, 환경 `HERDR_MACHINE`(예외 판정)
  - 출력: 한 줄에 하나씩 `같음|씀|충돌|예외|참고 <키>`
  - 종료 코드: 0은 문제 없음, 1은 `--check`에서 다름이 있거나 충돌이 있음, 2는 잘못 부름. 다른 출력은 표준 오류로 낸다.

## 시험·착지
- 시험은 네트워크와 실제 기계 없이 돈다. 임시 HOME·임시 저장소와 가짜 `hc`·`ssh`·`git`(PATH 앞), 설정은 `config.example`.
- 검증은 `bash .claude/herdr-verify.sh`(문법 검사 + 개인 값 검사 + `tests/*.test.sh`), 착지는 `hc-lane land main`이다.
- 끝난 레인 정리는 `hc-lane gc`(main에 다 들어갔고 깨끗하고 쓰는 터미널이 없는 작업 폴더·가지, `--integ`면 들어갔고 하루 지난 `refs/integ/*`만 지운다)이고, hc-fill이 설정 `gc` 분마다 부른다.
