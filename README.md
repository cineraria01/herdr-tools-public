# herdr-tools

여러 기계(맥 하나 + 워커 맥·서버)에서 Herdr로 에이전트 레인을 돌리는 운영 도구 모음이다. 개인 값(기계 이름·경로·계정·지침 본문)은 이 저장소에 없고, 따로 둔 **개인 저장소**(설정 폴더 `HERDR_TOOLS_CONFIG`, 기본 `~/.config/herdr-tools`)에서 읽는다. 꼴은 `config.example/`(가짜 값), 규칙은 `CONTRACT.md`.

| 도구 | 하는 일 | 도는 곳 |
|---|---|---|
| `hc-lane` | 레인 작업 트리 만들기·착지(`land`)·정리(`clean`·`gc`)·push 관문(`guard`) | macOS 기계 |
| `hc-fill` | 빈 자리에 기다리는 레인을 바로 띄우기 | 맥 |
| `herdr-sync` | 맥의 실제 파일을 두 저장소와 모든 기계에 맞추기 | 맥 |
| `herdr-settings-merge` | `~/.claude/settings.json`의 정한 키만 3-way로 맞추기(herdr-sync가 부른다) | 모든 기계 |
| `herdr-tools-config` | 개인 설정 읽기·개인 값 검사(`lint`) | 맥 |
| `herdr-tools-publish` | 범용 저장소를 기록 없이 공개 저장소로 내보내기(herdr-sync가 부른다) | 맥 |
| `herdr-balance` | Herdr 창에서 지휘 pane 옆 worker 높이 맞추기(`--watch`) | 맥 |
| `hc-sweep`, `ovis-split`, `hc-core-relaunch`, `worker-mac-check`·`worker-mac-setup` | 끝난 터미널 정리, Ovis 분할, 코어 다시 띄우기, 워커 맥 점검·설정(`worker-mac-setup mesh`: 워커끼리 ssh·hc 연결) | 맥·macOS 기계 |
| `claude/hooks/` | 같은 작업 트리 동거 편집 가드, 무인 루프 질문 창 막기, `hc-lane wait` 분리(&) 실행 막기, 레인 세션의 셸 파일 고치기·heredoc 커밋·zsh 확장 경로 막기(`lane-no-shell-edit.sh`) | macOS 기계 |

## herdr-sync: 정본은 맥의 실제 파일, 두 저장소는 기록

- 정본은 맥의 `~/.local/bin`, `~/.claude/…`, `~/.agents/skills/…` 같은 실제 파일이다.
- 맥에서 고치고 `herdr-sync`를 돌린다. 맥에서 바뀐 것은 그 줄의 저장소로 모아(**모음**) 저장소마다 따로 커밋·`HERDR_LAND=1` push하고, 모든 기계에 맞춘다.
- 레인이 어느 저장소를 고쳐 착지했으면, 다음 `herdr-sync`가 그것을 맥에 깔고(**깖**) 기계에 맞춘다.
- 무엇을 어디에 맞추는지는 개인 저장소의 `sync.list`가 정한다(꼴 `config.example/sync.list`). 저장소 경로가 `tools:<경로>`면 이 범용 저장소, 그 밖은 개인 저장소 파일이다. 새 파일을 맞추려면 한 줄만 더한다. 설정(`~/.claude/settings.json`)은 개인 저장소 `settings/keys.json`에 적은 키만 맞춘다(꼴 `config.example/settings/keys.json`).
- **개인 값 검사**: 이 범용 저장소로 모을 맥 값이 `herdr-tools-config lint`(개인 저장소 `private-patterns.txt`)에 걸리면 `충돌 저장소 <경로> (개인 값)`을 찍고 아무 데도 쓰지 않고 멈춘다. `--take repo:`로도 넘기지 않는다. 그 값을 범용으로 바꾸거나 그 줄을 개인 저장소로 옮긴다. 개인 저장소에 `private-patterns.txt`가 없거나 패턴 줄이 없으면 검사를 못 하므로 범용으로 모을 값은 모두 `(개인 값 검사 못 함 — …)` 충돌로 멈춘다.
- 처음 한 번은 저장소에서 바로 돌린다: `<이 저장소>/bin/herdr-sync`. 그러면 herdr-sync 자신도 `~/.local/bin`에 깔린다(`sync.list`에 그 줄이 있으면).

### 사용법 (맥에서만)

```sh
herdr-sync --check              # 고치지 않고 모든 기계를 한 표로. 다름·충돌이 있으면 종료 1
herdr-sync                      # 맞춘다
herdr-sync box-1 box-2          # 고른 기계만
herdr-sync --take <기계>:<경로>  # 충돌을 맥 값으로 덮는다 (그 기계에 백업)
herdr-sync --pull <기계>:<경로>  # 그 기계 값을 맥 실제 파일로 가져온 뒤 맞춘다
```

- `--check` 표의 첫 줄은 맥 ↔ 두 저장소다: `맥: 저장소가 새로움 n개 (<저장소 경로> …) — herdr-sync로 깐다`(레인이 착지했는데 맥에 아직 안 깔림), `맥: 저장소보다 새로움 n개 (…) — 커밋 안 됨?`(맥에서 고쳤는데 아직 안 모음), 다 맞으면 `맥: 저장소와 같음`. 기계 줄은 **맥 실제 파일**과 견준 것이라, 맥이 저장소보다 뒤면 기계 줄 끝에 `(맥 판 기준 — 저장소 새 판은 아직)`이 붙는다 — 그때 `같음`은 「맥 옛 판과 같음」이지 「저장소 새 판이 퍼졌음」이 아니다. 착지 뒤 퍼졌는지 볼 때는 맥 줄부터 본다(2026-11-19 착지 직후 `--check`의 기계 줄 `같음`만 보고 「이미 퍼졌다」고 오판했다).
- 경로는 `sync.list`의 맥 경로(`~` 아래)다. 예: `.claude/docs/notes.md`, 지침 본문은 `.claude/CLAUDE.md#body`, Codex 공통 구역은 `.codex/AGENTS.md#codex-shared`, 링크는 `.claude/skills/my-skill@`, 설정 키는 `.model`, 설정 훅은 `hook:PreToolUse:cotenant-guard.sh`(herdr-settings-merge의 판정 이름), 나무 안 파일은 `.agents/skills/my-skill/scripts/run.sh`.
- `<기계>:*`는 그 기계의 충돌 전부, `<기계>:<폴더>/**`는 그 나무 줄의 그 기계 충돌 전부다(`repo:<폴더>/**`도 된다).
- Codex 공통 구역과 설정 키는 `--pull`을 받지 않는다(구역은 맥 지침에서 다시 만들고, 설정은 맥 `settings.json`을 고친다). `--take`를 쓴다.
- 저장소 위치: 범용 `HERDR_TOOLS_REPO`(기본 `~/Documents/app/herdr-tools`), 개인 `HERDR_TOOLS_CONFIG`(기본 `~/.config/herdr-tools`). 둘 다 git·main 브랜치·깨끗함이어야 한다. 개인 저장소(`sync.list`)가 없으면 종료 2, 아무것도 쓰지 않는다.
- 대상 이름: `all`(모든 기계) · `macos`(uname이 Darwin) · `local`(맥만) · 기계 이름을 쉼표로. 옛 이름 `all9`·`mac7`·`mac-only`가 보이면 새 이름을 알리고 종료 2(옛 판은 그것을 기계 이름으로 읽어 아무 데도 안 깔고 「같음」으로 보였다).
- 순서: 두 저장소 `git pull --ff-only` → `sync.list` 읽기 → `--pull` 처리 → 개인 설정 `sync.prebuild`(있으면 `sh -c`로, 예: 맥 지침에서 Codex 공통 구역 다시 만들기. 실패하면 멈춤) → 맥 ↔ 두 저장소 → 기계마다 파일·설정(`~/.local/bin/herdr-settings-merge`)·지울 것 → 마지막에 기계마다 한 줄 `같음` / `다름 (묶음…)` / `충돌 n건`. 저장소에서 지침 본문을 깔았으면 prebuild와 맥 ↔ 저장소를 한 번 더 돈다.
- sh를 못 띄우는 기계(Windows 등, hc 오류 `spawn_failed`·`cannot start `)는 `<기계>: 건너뜀 (sh 없음 — Windows 등)`, uname이 Darwin·Linux가 아닌 기계(Git sh의 `MINGW64_NT-…`·`MSYS_NT-…` 등)는 `<기계>: 건너뜀 (OS <uname>)` 한 줄만 찍고 아무것도 쓰지 않는다(`--take`·`--pull`로 집어도). 둘 다 종료 코드에 넣지 않는다. 진짜 연결 실패·시간 초과는 지금처럼 `다름 (연결)`·종료 1이다.
- 다른 herdr-sync가 돌고 있으면 바로 끝난다(종료 75). 맥이 아닌 곳(`~/Applications/Herdr.app`·`/Applications/Herdr.app`이 없거나 `hc machine ls`에 기계가 없는 곳)에서는 종료 2.
- 설정은 대상 기계의 `herdr-settings-merge`가 `~/.local/share/herdr-sync/keys.json`(개인 저장소 `settings/keys.json`을 깐 것)과 `HERDR_MACHINE`으로 판정한다. 그 도구가 판정 줄 없이 실패하면 `다름 (도구문제:설정)`으로 보인다(충돌과 다르다).
- 저장소에 커밋하지 않은 변경이 있으면 아무것도 하지 않고 멈춘다. 표식 없는 `~/.codex/AGENTS.md`는 충돌로 보이고, `--take`하면 codebase-memory-mcp 블록만 남기고 구역을 넣는다(백업).

### 나무 줄: 폴더 통째로 (스킬 폴더 등)

`sync.list`의 맥 경로를 `<폴더>/**`로 쓰면 그 폴더 아래 모든 층의 파일을 하나하나 파일 줄처럼 맞춘다(저장소 경로는 폴더). 예:

```
스킬   .agents/skills/**             skills/agents/   macos
제외   .agents/skills/app-managed    -                -
```

- 걷지 않는 것: 심볼릭 링크(따라가지 않는다. 링크는 `@` 줄로), 이름이 `.`으로 시작하는 것(`.git`·`.DS_Store` 등), `__pycache__`, `node_modules`, `제외` 줄 아래(앱이 계정으로 맞추는 폴더 등. 지움과 달리 기계에 있는 것을 건드리지 않는다).
- 따로 적은 줄이 맡은 경로는 나무가 맡지 않는다. 대상 기계에만 있는 파일은 건드리지 않는다.
- 그 기계에서 손댄 파일은 충돌이다(덮지 않는다). 내용은 같고 실행 권한만 다르면 씀이다.
- 7MB 넘는 파일은 `건너뜀 <경로> (7MB 넘음)`(hc exec 입출력 한도 8MB 아래로 묶어 보낸다), 저장소 `.gitignore`에 걸리는 파일은 `건너뜀 <경로> (저장소 .gitignore)`, 그 기계에서 링크인 폴더는 `건너뜀 <기계> <경로> (링크 — 따라가지 않음)`.
- 기계마다 hc 호출은 조회 한 번, 그리고 쓰기·충돌 내용 읽기·`--pull` 받기가 7MB 아래 tar 묶음마다 한 번이다(파일 수가 아니라 바이트에 비례, 평소 한 번). 실측(가짜 기계 둘, 파일 2,000개·36MB): 첫 맞추기 약 15초, 다 맞은 뒤 `--check` 약 2.5초.
- 줄을 옮겨도(따로 적은 줄을 지워 나무가 맡게 함, 나무의 저장소 폴더를 바꿈) 맥 파일을 지우지 않는다: repo-base에 저장소 경로를 함께 적어, 경로가 바뀌면 새 자리로 모은다.
- 링크 줄은 가리킬 곳이 그 기계에 없으면 만들지 않고 `건너뜀 <기계> <경로> (가리킬 곳 없음)`. 가리킬 곳을 같은 실행에서 나무로 쓰면 그 뒤에 만든다.

### 충돌과 푸는 법

판정은 파일·구역·키마다 3-way다. 기계마다 `~/.local/state/herdr-sync/base.tsv`에 herdr-sync가 마지막으로 써 준 값의 cksum이 있다.

| 그 기계 값 | 판정 |
|---|---|
| 맥 값과 같다 | 같음 |
| base와 같다 (그 기계가 손대지 않음) | 맥 값으로 씀 |
| 내용은 맥 값과 같고 실행 권한만 다르다 | 씀 (권한만 맞춘다) |
| 그 밖 (그 기계에서 따로 고쳤거나, 처음이라 base가 없는데 다름) | 충돌: 쓰지 않고 `충돌 <기계> <경로> (n줄 다름)` |

- 맥 값이 맞으면 `herdr-sync --take <기계>:<경로>`.
- 그 기계 값이 맞으면 `herdr-sync --pull <기계>:<경로>`. 맥 실제 파일이 바뀌고, 저장소와 다른 기계에도 퍼진다.
- 맥 ↔ 저장소도 같은 규칙이다(base는 맥 `~/.local/state/herdr-sync/repo-base.tsv`, 열쇠는 맥 경로, 셋째 칸은 저장소 경로 — 줄을 옮기면 base 없음으로 본다. 셋째 칸이 없는 옛 base에서 저장소 쪽이 없으면 맥 파일을 지우지 않고 충돌로 멈춘다). 저장소에서 지워진 것은 맥에서 백업으로 옮기고 `맥 지움 <경로> (저장소에서 지워짐)`. 모으다 실패하면 그 저장소에서 이번에 쓴 경로만 되돌리고 멈춘다. 둘 다 바뀌었으면 `충돌 저장소 <경로>`를 찍고 아무 데도 쓰지 않고 멈춘다. 맥 값을 남기려면 `--take repo:<경로>`, 저장소 값을 남기려면 `--pull repo:<경로>`.
- push가 거절되면 멈추고, 다음 실행이 그 저장소의 「지난번 push 못 한 커밋」을 다시 보낸다.
- 덮거나 지운 것은 그 기계의 `~/.local/state/herdr-sync/bak/<YYYYMMDD-HHMMSS>/<경로>`에 남는다.
- 쓰기는 임시 파일에 쓰고 cksum을 맞춘 뒤 바꿔 넣는다. 실행 권한은 맥 값을 따른다.

### 맞추지 않는 것 (기계마다 다른 것)

- 로그인·계정·토큰 파일과 `~/.config/*`
- `~/.claude/rules/<기계>.md` 같은 기계 이름 규칙(맞출 것만 `sync.list`에 적는다)
- `~/.claude/CLAUDE.md`의 `<!-- User customizations -->` 줄까지(그 기계의 머리 블록)
- `~/.codex/AGENTS.md`의 codex-shared 표식 밖
- 셸 rc, launchd 작업(`launchd/`는 손으로 깐다)
- herdr가 관리하는 `~/.claude/hooks/herdr-agent-state.sh`
- settings의 `keys.json` `refs` 키(`--check`에서 「참고」로만 보인다)
- 리눅스 기계는 `all` 묶음만 받는다(도구가 macOS 명령을 전제로 한다).
- 맥에서 지운 파일은 기계에서 저절로 지우지 않는다. 지울 것은 `sync.list`에 `지움` 줄로 더한다.

## 공개 저장소로 내보내기 (herdr-tools-publish)

작업은 이 저장소(비공개)에서 하고, 공개 저장소에는 지금 파일만 기록 없이 내보낸다. 비공개 기록(커밋 메시지·작성자 이메일, 옛 판의 개인 값)은 나가지 않는다.

```sh
herdr-tools-publish --dry-run   # 바뀔 파일 목록(A·M·D)과 검사 결과만. push 없음, publish.dir을 만들지 않음(없으면 임시로)
herdr-tools-publish             # 검사 → 공개 main 위에 커밋 하나 → push
```

- 설정 키(개인 설정 `config`): `publish.repo` 공개 저장소 주소(없으면 종료 2, herdr-sync는 부르지 않는다) · `publish.dir` 공개 쪽 bare clone 자리(기본 `~/.local/share/herdr-tools/public`, 없으면 만든다. 있는데 bare git이 아니거나 origin이 `publish.repo`와 다르면 종료 1) · `publish.author` 공개 커밋 작성자·커미터 `이름 <메일>`(없으면 공개 clone의 git 설정 `user.name`·`user.email`. git이 호스트 이름으로 지어낸 값은 받지 않고, 부른 쪽 환경의 `GIT_AUTHOR_*`·`GIT_COMMITTER_*`·`EMAIL`은 지운다 — noreply 메일을 둔다) · `publish.secrets` 더 막을 비밀 꼴 ERE 하나(여러 꼴은 `|`).
- 하는 일: `git fetch origin main` → `refs/remotes/origin/main`(같은 이름의 로컬 가지에 속지 않게 전체 이름) 트리의 블롭마다 `cat-file`로 `<sha>` 이름 파일에 꺼내 검사(경로대로 디스크에 꺼내지 않는다 — macOS 디스크는 대소문자·정규화를 구분하지 않아 경로가 겹치고 링크를 따라간다. `git archive`는 `export-subst` 같은 attributes로 비공개 커밋 정보를 넣을 수 있어 쓰지 않는다) → 그 트리·블롭 객체만 공개 쪽으로 옮겨(비공개 커밋 객체는 안 옮김, 공개 트리 sha = 비공개 트리 sha — 지운 파일·실행 비트·링크 포함) 공개 main 위에 커밋 하나, 메시지는 `herdr-tools <비공개 짧은 sha> 내보냄` 한 줄 → fast-forward push(강제 push 없음). 공개 main이 없으면 부모 없는 첫 커밋. 트리가 공개 main과 같으면 `같음`, 커밋하지 않는다.
- **막는 것(fail-closed)**: 아래 하나라도 걸리면 아무것도 push하지 않고 걸린 `파일:줄: 까닭`을 찍고 종료 1(비밀 값 자체는 찍지 않는다).
  - 개인 값: `herdr-tools-config lint`(개인 설정 `private-patterns.txt`) — 블롭 내용(이진·UTF-8 아닌 파일도 바이트로), 경로, 링크 대상 글자, 공개 커밋 작성자·커미터. 그 파일이 없거나 패턴 줄이 없거나 lint가 실패하면(못 읽은 파일 포함) 「검사 못 함」으로 종료 1.
  - 비밀 꼴(코드 안 범용 꼴): 개인 키 머리(BEGIN … PRIVATE KEY), GitHub 토큰(`ghp_`·`gho_`… `github_pat_`), `sk-` 키, Slack `xox?-`, AWS `AKIA`, Google `AIza`, 40자 넘는 16진, 32자 넘게 이어지고 대·소문자·숫자가 다 든 base64 꼴 토큰(알파벳 목록 `ABCDEFGH…`·`0123456789`가 든 것은 뺌), 그리고 `publish.secrets`. 내용·경로·링크 대상 글자를 바이트로 본다. 경로가 걸리면 이름은 가리고 폴더와 몇 번째 파일인지만 찍는다.
  - 글이 아닌 블롭: NUL이 들었거나 UTF-8로 안 풀리는 블롭(UTF-16 글·압축·이미지 등 — 바이트 검사가 속을 못 본다). 걸린 경로만 찍는다. 범용 도구 저장소에 이진 파일은 없어야 정상이라 허용 목록은 없다(필요해지면 그때 설정 키로).
  - 받는 항목은 일반 파일·실행 파일·링크 블롭과 비어 있지 않은 하위 트리뿐이다. 빈 하위 트리·하위 모듈·낯선 mode는 막는다(이름은 가리고 폴더만 찍음). 하위 트리 이름도 경로 검사를 받는다(`ls-tree -r -t`).
  - 줄바꿈이 든 경로, 대소문자·유니코드 정규화(NFC)만 다른 경로 쌍(폴더 포함, `n번째·m번째 파일`로 찍음).
  - 공개 커밋 작성자 검사는 커밋을 만들 그 공개 clone에서 한다. push·fetch는 `origin` 설정(pushurl 등)이 아니라 `publish.repo` 주소로 직접 한다. replace ref는 따르지 않는다(`GIT_NO_REPLACE_OBJECTS=1` — 검사한 블롭 = 옮기는 블롭). `--dry-run`은 실제 `publish.dir`에 아무것도 쓰지 않는다(객체는 임시 bare에).
  - 검사 도구(`herdr-tools-config`)가 이 도구 곁에 없으면 종료 1.
- push·fetch는 인증 질문에서 멈추지 않는다(`GIT_TERMINAL_PROMPT=0`, `GIT_SSH_COMMAND`가 없으면 `ssh -o BatchMode=yes`).
- 늘 내보내기: `herdr-sync`가 맥에서 다 맞춘 뒤(`--check`가 아닐 때) `publish.repo`가 있으면 부른다. 실패하면 `!!! … !!!`로 크게 경고하고, herdr-sync 종료 코드는 그대로다(기계 맞추기와 별개).
- 시험이 비밀 꼴을 다룰 때는 조각을 이어 만든다(시험 파일 자체가 검사에 걸리지 않게).

## hc-lane 사건 (report·events·wait — 레인이 적고 지휘는 그것만 읽는다)

화면 글자나 보고 파일 끝 줄을 긁는 감시는 칸이 한 줄이거나 파일 이름이 다르면 완료를 놓친다(2026-11-16 레인이 20분~1시간 20분 놀았다). 그래서 레인이 상태를 사건 파일에 직접 적고, 지휘는 그 파일만 읽는다.

- 레인: `hc-lane report <중간|완료|착지|막힘|판정> [<sha>|-] [<한 줄>] [--to <지휘>,…]`. `--to`는 받을 지휘를 아홉째 칸에 적는다 — 그 지휘들의 watch만 보낸다(레인 주인 판정 대신. hc-fill 놀고 있음 알림이 쓴다). 그 기계 `${XDG_STATE_HOME:-~/.local/state}/hc-lane/events.tsv`에 한 줄을 더한다(지우지 않음). 칸: 시각(UTC `2026-11-16T02:00:00Z`)·기계(`HC_LANE_MACHINE`, 없으면 hostname)·터미널(`$HC_TERMINAL_ID`)·레인·상태·sha·한 줄(탭·줄바꿈은 빈칸)·그때의 hc turn(`hc get`, 터미널 밖이면 `-`. 옛 7칸 줄도 읽는다). 레인 이름은 `--name` → `$HC_LANE` → hc 터미널 이름(`hc get`) → 작업 트리 가지(기본 가지면 `-` — 작업 트리를 지운 뒤 메인 폴더에서 report해도 `main`이 레인이 되지 않게). 화면에도 `<레인> <상태>: <sha> <한 줄>`을 찍는다. `--print-steps`가 쓰기 레인에는 중간·완료·착지·막힘 때, `--read` 레인에는 판정 때(`hc-lane report 판정 - "<판정 한 줄>"`) 부르라는 줄을 넣는다.
- hc-lane이 띄우는 레인은 `HC_LANE=<이름>` 환경과 hc 라벨 `hc-lane=1`을 받는다.
- 지휘: `hc-lane events [--all|--on <기계>…] [--since <시각|30m·2h·1d>]`는 이 기계(와 연결된 기계마다 `hc exec --on` 한 번)의 사건을 시각 순으로 찍는다. 원격 줄의 기계·터미널은 hc 이름(`<기계>/<t…>`)이다. 못 읽은 기계는 `모름 <기계>`. sh를 띄울 수 없는 기계(예: Windows — `hc exec`가 `spawn_failed`·`cannot start sh`)는 레인 기록이 없는 것으로 보고 건너뛴다(events·wait·watch 모두 `모름`·`연결 안 됨` 없음, 쉬는 레인 ps 확인도 같다).
- 지휘: `hc-lane wait [--all] [--timeout <분>(기본 110)] [--state 완료,착지,…]`를 배경으로 건다. 아직 본 적 없는 사건이 생기면 그 줄(들)을 찍고 종료 0, 상한이면 75. 찍은 사건은 이 기계 `…/hc-lane/seen.<$HC_TERMINAL_ID>`(지휘마다 따로, 터미널 밖이면 `seen`)에 적어 다시 알리지 않는다(바꾸는 것은 이 파일뿐). 출력이 `/dev/null`이면 읽을 사람이 없어 seen에 적지 않고 종료 2(배경으로 걸 때는 출력을 파일로 받는다). 새 커서는 `--since`(없으면 2시간 전)부터 본다 — 레인을 띄운 뒤 2시간 안에 첫 wait를 건다. 라벨 `hc-lane=1` 터미널은 그 터미널(주소·레인 이름)의 마지막 사건으로 본다: 착지·판정 그 턴(report turn+1 이하, report turn을 모르면 밖) 밖에서 끝났거나(한 줄 칸 `뜻밖의 종료(<상태>)` — 착지 전인 완료·막힘 뒤와, 착지·판정 뒤 이어 받은 일에서 끝난 것도 알린다. 착지 뒤에 같은 sha의 완료가 같은 턴(같은 turn 값)에만 이어지면 — 착지 뒤 완료를 한 번 더 적는 레인 틀 — 착지로 치고 turn은 그 완료의 것으로 본다. sha가 `-`(판정·sha 없음)이거나, 사이에 turn이 올랐거나, 다른 sha의 완료·중간이 뒤에 오면 새 일. sha는 소문자로 짧은 쪽 길이만큼 앞자리를 견주고, report는 대문자 sha도 소문자로 적는다), 완료·착지·막힘·판정 없이(또는 그 report의 turn+1보다 뒤에 — 이어 받은 일을 report 없이 끝냄) done·idle로 쉬거나, blocked·error면 `멈춤` 사건(한 줄 칸 `<상태> turn <n>` — turn이 바뀌면 다시 알리고, 같은 turn의 done·idle은 한 번). done·idle은 쉰 지 `lane.wait_grace`분(기본 10, `0`이면 끔)이 지나고 배경 작업이 없을 때 멈춤이다. 배경 작업은 프로세스로 본다: `hc get`의 터미널 `pid` 자손 가운데 명령 줄에 `lane.wait_procs` 글자(빈칸으로 나눈 목록, 기본 `shell-snapshots` — Claude Code Bash 도구 셸 `/bin/zsh -c source …/.claude/shell-snapshots/snapshot-zsh-….sh …`)가 든 것이 살아 있으면 배경 작업 중(MCP 서버처럼 늘 사는 자손은 그 꼴이 아니라 안 셈). 이 기계는 `ps`, 원격은 바퀴마다 기계당 `hc exec --on <기계> -- ps` 한 번. 그런 프로세스가 없거나(셸 없는 배경 Agent 등) pid를 모르거나 ps가 실패하거나 `lane.wait_procs`가 빈 값이면 화면 표시도 본다: 화면의 마지막 상태 줄(`lane.wait_line`, 기본 ✻ 턴 끝 줄 — 옛 턴의 줄·본문은 안 봄)에 `lane.wait_busy`(확장 정규식, 기본 `shells? still running|monitors? still running`, 빈 값이면 끔) 표시가 없을 때 멈춤 — 배경 셸이 도는 동안 턴을 끝낸 레인은 스스로 이어 가니 알리지 않는다. 배경 작업이 있어도 쉰 지 `lane.wait_busy_max`분(기본 30)이 넘으면 멈춤(blocked·error·끝남은 바로). 그 바퀴에 events를 못 읽은 기계의 레인은 blocked·error만 그대로 알리고, 사건이 있어야 하는 쉼·끝남은 `연결 안 됨 <기계> <날짜>`(레인마다 하루 한 번)로 알린다 — 다음 바퀴에 다시 읽히면 정상 판정. 줄은 1000바이트 안으로 잘라(끝 반쪽 UTF-8 버림) 여러 레인이 함께 더해도 섞이지 않는다. 바퀴마다(`HC_LANE_WAIT_TICK`초, 기본 60) `hc machine ls`·`hc ls` 한 번과 기계당 `hc exec` 한 번.
- **watch(늘 도는 감시, 지휘가 걸 일 없음)**: `hc-lane watch --to <지휘 주소> [--all]`은 wait와 같은 판정을 `lane.watch_tick`초(기본 30)마다 돌며, 새 사건이 생기면 지휘 터미널에 `hc send <지휘> "hc-lane: <레인> <상태> <sha> <한 줄> (<터미널>) ‖ …"`로 한 번 밀어 준다(지휘가 바빠도 입력에 쌓인다). 커서는 그 지휘의 wait와 같은 `seen.<지휘 주소>`라 둘이 겹쳐 두 번 알리지 않고, hc send가 실패하면 커서에 안 적고 다음 바퀴에 다시 보낸다. 같은 지휘로 하나만(뒤 것은 종료 75, 잠금 `…/hc-lane/watch.<주소>/pid` — 그 pid가 없거나 hc-lane watch가 아니면 넘겨받음). 못 읽은 기계는 `연결 안 됨 <기계>`를 끊길 때마다 한 번 보내고 다음 바퀴에 다시 본다. `hc ls`가 3바퀴 이어 실패하면 그 멈춤 한 줄을 한 번 보내고, 실패가 이어지는 동안은 더 보내지 않으며 되살아나면 `hc ls 회복` 한 번. 보내는 글의 제어 문자는 지운다. 스스로 끝남(종료 0): 지휘 터미널이 3바퀴 이어 `hc get`에 없거나(not_found) 끝남 — 코어 연결 실패·시간 초과는 모름이라 세지 않고 계속 돈다 —, 또는 그 지휘의 살아 있는 레인(라벨 `hc-lane=1`, parent가 그 지휘 — 모르면 그 지휘 것으로 침)도, hc-fill 대기열(`${HC_FILL_DATA:-~/.local/share/hc-fill}/queue.tsv`)에 아직 안 띄운 요청(wait — 받은편지 요청도 hc-fill이 바퀴마다 여기로 옮긴다. up은 세지 않는다: 살아 있는 up 레인은 hc ls가 세고, 끝난 up 줄은 `hc-fill rm` 전까지 남는다)도, 새 사건도 없이 `lane.watch_idle`분(기본 60).
  - **저절로 뜬다**: `hc-lane`이 레인·감사(`--read` 포함)를 띄울 때 지휘(`HC_LANE_COORD`, 없으면 `$HC_TERMINAL_ID`, 둘 다 없으면 건너뜀)의 watch가 돌고 있지 않으면 이 기계에 `hc run --bg --no-notify --rm --name lane-watch-<지휘> --label hc-lane-watch=1 -- hc-lane watch --all --to <지휘>`를 띄운다(지휘 터미널에서 띄우면 그 터미널이 parent라 지휘가 끝나면 같이 끝남). **hc-fill**(launchd라 터미널 밖)은 대기열 줄의 주인 지휘(`hc-fill add`를 부른 지휘)를, 주인이 없으면 `hc-lane coord`가 확인한 지휘 주소를 `HC_LANE_COORD`로 넘겨 같은 길을 탄다.
  - **지휘 정보는 지휘마다**(`…/hc-lane/coord.<주소>`, 「주소<탭>pid<탭>이름<탭>마지막으로 레인을 띄운 시각」): 지휘 주소는 설정에 박지 않는다(코어가 다시 뜨면 주소가 남의 터미널로 간다). 지휘 터미널이 레인·감사를 띄우거나 `hc-lane wait`를 걸거나 손으로 watch를 걸 때(watch 터미널의 parent가 `--to`), `hc-fill add`를 부를 때(`hc-lane coord --set`) 자기 정보를 남긴다 — 레인·감시 라벨 터미널은 안 적는다. watch는 바퀴마다와 **보낼 때마다** 자기 지휘의 정보와 지금 `hc get` pid·이름을 맞춰 보고, 다르거나 없으면 보내지 않고(남의 터미널 send 0) `지휘 없음` 한 줄을 그 기계 `…/hc-lane/watch.log`에 남기고 끝낸다 — 커서에 안 적었으니 사건은 events에 남아 지휘가 돌아와 wait/watch를 걸면 받는다.
  - **사건은 그 레인을 띄운 지휘에게만**: hc-lane이 레인을 띄울 때 레인 기록 `…/hc-lane/lanes.tsv`(「레인 주소<탭>레인<탭>띄운 지휘」)를 남긴다(`HC_LANE_OWNER`가 있으면 그 지휘 — hc-fill이 대기열 주인을 넘긴다 —, 아니면 지휘 터미널에서 띄웠을 때 그 지휘, 주인 없는 hc-fill 줄·터미널 밖은 `-`). 레인 환경에도 `HC_LANE_OWNER`가 들어가, 레인 안에서 hc-lane으로 띄운 하위 레인(감사 등)도 같은 지휘가 주인이다(물려받은 값이 없으면 `-`). 값은 지휘 기계의 주소라 원격 레인에서 `hc send`하려면 `<지휘 기계>/<주소>`. watch는 레인 기록(없으면 `hc ls` parent)의 지휘가 자기면 보내고, 다른 살아 있는 지휘면 보내지 않는다(커서에 안 적어 그 지휘 몫). **주인을 모르거나(`-`) 주인 지휘가 사라진 레인의 사건은 살아 있는 모든 지휘 watch가 보낸다** — 놓치는 것보다 겹치는 게 낫다(2026-11-17: 가장 최근 지휘 한 곳이 넘겨받아 영구 주인이 되어, 대기열에 넣은 다른 지휘가 2시간 못 받음). 아무도 넘겨받지 않고(레인 기록에 안 더함) `sent`에도 안 적어, 지휘마다 자기 `seen.<지휘>`로 한 번씩만 간다. 살아 있는 지휘가 없으면 아무도 안 보내고 events에 남는다. 그래서 새로 뜬 지휘 watch는 커서 바닥(2시간 전)부터 그런 레인의 사건을 한 번에 받는다. 사라진 주인이 같은 주소로 지휘 정보를 다시 쓰면(그 터미널이 다시 레인을 띄우거나 wait를 걸면) 그 레인은 다시 그 지휘 몫이 되고, 그 사이 다른 지휘가 받은 사건을 한 번 더 받을 수 있다(겹침이지 놓침은 아님). 그래서 같은 기계에 지휘가 둘이어도 각자 자기 사건만 받고, 한쪽이 hc-lane을 써도 다른 쪽 watch는 계속 돈다. 레인 기록이 `-`여도 hc parent가 있으면 그 지휘로 본다.
  - **세 줄 규칙**: (1) 「보냄」 표시 `…/hc-lane/sent`는 **watch만** 쓴다 — 끌어오는 wait는 자기 커서 `seen.<지휘>`에만 적고, 남의 지휘 wait는 보기만 한다. (2) 사건의 주인은 그 레인을 띄운 지휘(레인 기록). 주인을 모르거나 주인이 사라진 레인은 아무도 넘겨받지 않고 모든 지휘 watch가 보낸다. (3) watch는 「주인이 나(또는 모름·사라짐) + `sent`에 없음 + 내 `seen.<나>`에 없음」인 사건만 보내고, 보낸 뒤 `seen.<나>`에, 자기가 주인인 사건이면 `sent`에도 적는다(같은 지휘의 wait와 겹쳐 두 번 안 감). 「보낸 것」은 `sent` ∪ 그 레인의 옛 주인(레인 기록에 그 레인 지휘로 적힌 적 있는, 이 watch가 아닌 지휘)의 `seen.<옛 주인>`이다 — 주인이 사라진 사건을 보낼 때 옛 주인이 자기 wait로 본 사건은 받은 것으로 치고 이 watch 커서에 적는다(옛 주인의 seen 파일이 없으면 `sent`만) — 그래서 지휘가 끝나도 그 지휘가 받은 사건은 다시 가지 않고, 아무도 안 받은 것만 간다. hc-lane 띄우기·hc-fill은 대상 지휘가 안 맞으면 watch를 띄우지 않고 `지휘 없음` 한 줄. 그래서 지휘는 wait를 사건마다 다시 걸 필요가 없다(2026-11-16 실측: 다시 걸기를 한 번 빠뜨려 완료를 34·14분 늦게 봄). `hc-sweep`은 `lane-watch-*`를 닫지 않는다.
  - 손으로 띄우려면(레인 없이 감시만): `hc run --bg --no-notify --rm --name lane-watch-$HC_TERMINAL_ID --label hc-lane-watch=1 -- hc-lane watch --all --to $HC_TERMINAL_ID`
- 정리됨: `hc-sweep`이 레인 터미널(라벨 `hc-lane=1`)을 닫을 때 닫기 전에 그 기계 events.tsv에 `정리됨` 한 줄(한 줄 칸 `hc-sweep <규칙>`)을 적는다(`hc-lane events`로 보인다). 그 줄을 못 쓰면 닫지 않는다(닫으면 wait가 reason killed로 보고 조용히 넘긴다). hc-sweep은 닫은 뒤 `hc rm`까지 해 터미널이 목록에서 사라지므로, wait는 이 줄로 판정한다: 그 터미널의 바로 앞 report가 착지·판정이고 정리됨 turn이 그 turn+1 안이면(또는 같은 turn의 쉼으로 이미 알렸으면) 버리고, 아니면 `멈춤`(한 줄 칸 `뜻밖의 종료(정리됨) hc-sweep <규칙>`) — wait가 안 돌던 틈에 닫힌 보고 없는 레인도 다음 wait에서 나온다. 쉬는 레인 판정에서는 정리됨이 마지막 report를 가리지 않는다.
- 마지막 보고(2026-11-17): 착지·판정에 더해 `--read`·hc-fill 레인은 `완료`도 마지막 보고다(레인 기록 `lanes.tsv` 넷째 칸 `read`·`fill` — hc-lane이 띄울 때 적는다. 구현 레인은 3칸 그대로라 `완료` 뒤 감사를 기다리다 혼자 끝나면 여전히 `뜻밖의 종료`). 지휘가 `hc kill`로 닫은 레인(`hc ls`의 `reason: killed`)의 끝남은 알리지 않는다 — parent가 없거나(hc-fill 레인, hc-fill이 `HC_LANE_FILL=1`로 띄움) parent가 아직 살아 있을 때만이다. 지휘 칸이 죽어 함께 끝난 레인(die_with_parent도 `killed`)은 그대로 알린다. hc는 원격 레인의 parent를 묻는 쪽 기준 주소로 준다(레인 기계에선 `<지휘 기계>/t7`, 지휘 기계에선 `t7` — 2026-11-17 실측)라 parent는 그대로 이 기계에서 묻고, 그 주소의 기계를 모를 때(machine_unknown — 바꿔 주지 않은 `<이 기계>/t…` 꼴)만 앞부분을 떼고 다시 묻는다. 조용히 넘긴 killed는 커서에 그 멈춤 열쇠로 적어 지휘가 끝난 뒤에도 다시 오지 않는다. 쉬는 레인은 그 작업 트리 경로가 명령 줄에 든 프로세스(떨어져 나간 배경 검증 등)가 살아 있으면 바쁨으로 본다(`lane.wait_busy_max`분까지).
- 열쇠에 기계 이름 없음(2026-11-17): `seen.*`·`sent`의 열쇠는 기계 칸을 `*`로, 주소는 끝의 `t…`만 쓴다. hc 기계 이름을 바꿔도(`old-name` → `new-name`) 본 사건이 다시 나오지 않고, 옛 판이 적은 줄(기계 이름이 든 그대로)도 같은 사건으로 읽는다.
- 지휘별 거름(`--mine`, 기본 꺼짐): 레인의 hc parent가 부른 터미널(`$HC_TERMINAL_ID` — 원격 레인의 parent는 보통 앞부분 없는 꼴이고, `<이 기계>/<t…>` 꼴도 끝으로 맞춤)인 사건만 찍는다. parent를 모르면(`hc ls`에 없음 — 지워진 터미널 등) 보여 준다. 한 기계에 지휘가 여럿일 때만 쓴다 — 지휘가 바뀌었거나(옛 레인의 parent는 죽은 옛 지휘) wait를 따로 `hc run`으로 감싸 다른 터미널에서 부르면 사건이 다 가려진다. 기본은 전부 찍는다(섞이는 것은 거짓 알림 쪽, 가려지는 것은 놓치는 쪽).
- `hc ls`가 3바퀴 이어 실패하면 wait는 `멈춤` 한 줄(`hc ls 실패 n바퀴(<시각>부터) — 레인 멈춤·끝남을 못 봄`)로 끝낸다.
- `lane.notify no`(기본 `yes`): hc-lane이 띄우는 레인·감사(`--read` 포함)에 `hc run --no-notify`를 붙여, hc가 지휘 입력에 넣는 종료·막힘 알림(`<hc>… exited (signal 1)` — hc-sweep이 일 끝난 레인을 닫을 때마다 온다)을 끈다. 그러면 막힘·오류·뜻밖의 종료는 `hc-lane wait`만 알리므로 **지휘에게 `hc-lane watch`가 돌 때만**(레인을 띄우면 저절로 뜬다) `no`로 둔다.
- 주의: hc도 `XDG_STATE_HOME`으로 코어 소켓을 찾는다. 사건 폴더를 옮기려고 그 값을 바꾸면 hc가 빈 코어를 따로 띄워 `hc ls`가 비고(멈춤을 못 봄), 경로가 길면 소켓 길이 제한으로 실패한다.

## hc-lane 띄우기 (지시문 사본·신뢰 질문·시작 확인·fetch)

```sh
hc-fill add --name <레인> --project <프로젝트> --brief <지시문>                            # 쓰기 레인은 대기열로(아래)
hc-lane --direct --on <기계> --brief <지시문> --name <레인> --repo <그 기계 저장소> -- claude … "{brief} 를 읽고 진행하라"
hc-lane --on <기계> --read --brief <감사 지시문> --name <레인> --repo <그 기계 저장소>   # -- 뒤가 없으면 설정 lane.run 틀
hc-lane fetch --on <기계> --repo <그 기계 저장소> [<참조…>]                               # 감사 대상 커밋 받기
```

- 쓰기 레인은 대기열로(2026-11-19 — 지휘가 쓰기 레인을 직접 띄워 hc-fill 대기열이 비고, 자리가 나도 이어 뜰 것이 없었다): 지휘 터미널(`$HC_TERMINAL_ID`가 살아 있고 레인·감시 터미널이 아님 — 레인 주인을 정하는 것과 같은 판정)이 쓰기 레인(`--read`·`--print-steps`·`--dry-run` 아님)을 `HC_LANE_FILL=1`(hc-fill이 띄울 때) 없이 띄우면 아무것도 만들기 전에 `hc-lane: 쓰기 레인은 hc-fill add로 대기열에 넣는다 …` 한 줄과 종료 2. 지금 꼭 이 자리에 띄워야 하면 `--direct`(오류 출력 `대기열 밖에서 띄움(--direct)`, 레인 기록 `lanes.tsv` 넷째 칸 `direct` — 직접 띄운 비율을 셀 수 있게. wait·send는 구현 레인과 같게 본다). 레인 안(`HC_LANE`)·터미널 밖(launchd·cron)·레인·감시 터미널·판정 못 함(`hc get` 실패)은 그대로 띄운다.
- `--brief`: 지시문에 `--print-steps` 착지·정리 절차를 붙인 사본(`--read`는 판정 알림 줄)을 대상 기계에 두고 실행 명령의 `{brief}`를 그 경로로 바꾼다. 자리는 설정 `lane.brief_dir`(그 기계 경로, `~`는 그 기계 집)의 `<레인>-run.md`, 없으면 지시문과 같은 폴더의 `<이름>-run.md`(그 기계에 같은 경로를 쓸 수 없으면 실패하고 `lane.brief_dir`을 알린다). `{brief}`가 없거나 `--brief` 없이 `{brief}`가 있으면 아무것도 만들기 전에 오류.
- 띄운 직후: 화면에 Claude 폴더 신뢰 질문(`❯ No, exit` / `Yes, I trust this folder`)이 보이면 그 터미널에만 신뢰를 고른다(`lane.auto_trust off`면 안 함). 질문 글자가 다르거나 커서(`❯`)가 어느 줄에도 안 보이면 건드리지 않고 `모름 — 시작 화면 확인`. 외부 가져오기 질문(`Allow external CLAUDE.md file imports?` — 작업 폴더 밖 파일을 `@`로 가져오는 CLAUDE.md)은 `External imports:` 목록 경로가 모두 작업 폴더의 git 저장소(`git -C <작업 폴더> rev-parse --show-toplevel`) 안일 때만 그 터미널에 `Yes, allow external imports`를 고른다(`lane.auto_trust off`면 안 함). 밖이거나 읽을 수 없는 줄(가운데 `…` 생략·`… +N imports not shown.`)이 하나라도 있으면 고르지 않고 `모름 — 외부 가져오기 질문(<경로>)`, 종료 5. 커밋하지 않는 일을 저장소 안 하위 폴더(`.qa/…`)나 저장소가 아닌 폴더에서 띄울 때도 맨 `hc run` 대신 `hc-lane --read --repo <그 폴더>`를 쓰면 이 시작 확인을 받는다(`--read`는 준 폴더 그대로에서 띄우고, 저장소가 아니면 `LANE_BASE`가 빈 값).
- 시작 확인: `--start-wait <초>`(이 호출만) 또는 `lane.start_wait`초(기본 60, `0`이면 신뢰 질문·시작 확인 끔) 안에 `hc get` status가 안 잡히면 화면 끝 15줄에서 막힘 낱말을 찾아 한 줄로 알린다. 종료 5 시작 안 됨(원인 모름·신뢰 질문·터미널 끝남), 6 로그인·인증(`Login expired`·`/login` 등), 7 키체인. 터미널은 남기고, 마지막 JSON 줄에 `lane` 주소와 `start`(`ok`·`off`·`no_status`·`login`·`keychain`)가 있다. hc status를 내는 에이전트(Claude 등)용이다 — status를 내지 않는 명령·CLI는 `--start-wait 0`으로 띄운다. hc-fill은 5·6·7을 「상태 안 잡힘」으로 받아 그 터미널을 남긴 것으로 적고 그 기계를 빼고 다음 기계로 간다.
- 다른 기계 레인(`--on`, `--read` 감사·hc-fill 원격 띄우기 포함)은 `hc run --label herdr.space=<id>`로 맥 Herdr 창의 **그 기계 공간**(이름 = 기계 이름, 기계마다 하나)에 탭으로 보인다. 창에 그 기계의 `<기계>` 공간이 있으면 그 id(창이 보이는 `<기계>/` 접두는 뗀다), 없으면 고정 id `lanes`로 붙이고 창이 만든 공간에 기계 이름을 붙인다(`space.rename`). 창(herdr-app)이 없으면 띄우기는 그대로 성공하고 알림 한 줄. 로컬 레인은 hc-fill이 `herdr-balance --add`(지휘 옆 분할)로 붙인다.
- 설정 훅(`.claude/herdr-setup.sh`·`herdr-teardown.sh`)은 hc 터미널 밖에서 부른 로컬(launchd hc-fill 등)이면 `hc exec --`(코어 문맥, `~/.zshrc`까지 읽음)로 돈다 — launchd가 띄운 bash는 `~/Documents`의 스크립트를 못 읽는다(TCC `Operation not permitted`). 설정 훅이나 `hc run`이 실패해 레인을 안 띄우면 방금 만든 워크트리·브랜치를 지운다(고유 커밋이 없을 때만, 있으면 남기고 지우는 명령을 알린다) — 다음 시도가 「로컬 브랜치가 이미 있다」로 막히지 않게.
- 메인 폴더 앞당김(2026-11-18 — 워커 기계 메인 폴더가 이틀 동안 main보다 375~497커밋 뒤라 `--read` 감사가 옛 `CLAUDE.md`·계약·시험을 읽었다): 레인·`--read` 레인을 띄울 때 fetch 바로 뒤와 `hc-lane land` 착지 성공 뒤, 메인 폴더(`--repo`의 첫 작업 트리)가 기본 브랜치에 있고 추적 파일 변경과 진행 중인 merge·rebase·cherry-pick·revert와 skip-worktree·assume-unchanged 표시 파일이 없고, 그 기계에서 메인 폴더(아래, `<저장소>.wt/` 밖)를 cwd로 쓰는 살아 있는 working·blocked 터미널(부른 터미널 자신 빼고, `hc ls`로 봄 — 실패면 건너뜀)이 없으면 `git merge --ff-only --no-overwrite-ignore origin/<기본>`(쉬는 세션 밑에서는 앞당긴다). 결과 한 줄 `hc-lane: 메인 폴더 앞당김 <a>→<b>`(또는 `최신 <a>`). 못 하면(다른 가지·더러움·진행 중·일하는 세션·로컬에만 있는 커밋·추적 안 된 파일이나 무시된 파일과 부딪혀 git이 거절) 오류 출력에 `hc-lane: 경고 — 메인 폴더 앞당김 건너뜀: <까닭> — 그대로 둠` 한 줄만 남기고 레인·착지는 그대로 간다. `stash`·`reset`·`checkout --`·강제는 쓰지 않는다. 원격이 없는 저장소는 착지가 이미 로컬 `<기본>`을 올리니 따로 하지 않는다. 되돌리기: 그 폴더에서 `git merge --ff-only`의 반대는 `git reset --keep <a>`(사람이 할 때만).
- `fetch`: 맨 `ssh`나 hc 코어 환경에는 git 자격 도우미 PATH가 없어 「could not read Username」이 난다. `--on`이면 `hc exec`로 그 기계 zsh 로그인 셸(`~/.zprofile`)에 `~/.zshrc`까지 읽고 `git fetch origin [<참조…>]`(설정 단계와 같은 셸). 참조는 하나라도 `-`로 시작하면 거절한다(git 옵션 주입).

## hc-lane gc (끝난 레인 정리)

- 무시된 보관물 `.qa/`: `hc-lane clean`은 지우기 전(teardown 전)에 워크트리의 무시된 `.qa/` 파일(어느 깊이든)을 메인 폴더(첫 작업 트리)의 같은 자리로 복사(덮지 않음, `--on`이면 그 기계 안)하고 `옮김 <워크트리>/.qa → <메인>/.qa (파일 N개 — 새로 a, 이미 같음 b)` 한 줄을 찍는다. 메인에 같은 이름의 다른 파일이 있거나 복사가 실패하거나 메인 폴더를 못 찾으면 그 파일 목록과 함께 멈추고 아무것도 지우지 않는다(2026-11-19 — 레인이 gc의 「남김 … 무시된 보관물 .qa」를 보고 손으로 지우다 출력을 잃었다). gc는 `.qa/`가 남은 작업 폴더를 지우지 않고 남긴다 — 손으로 지우지 말고 `hc-lane clean`.

- `hc-lane gc [--on <기계>] --repo <저장소> [--dry-run] [--integ[=<접두>]] [--remote-branches] [--min-age <시간>]`. 먼저 `--dry-run`으로 본다(`origin/<기본>`은 fetch하지 않고 지금 값으로 판정, `--integ` 판정용 커밋 객체만 받는다 — 참조·작업 폴더·index는 그대로).
- 작업 폴더·가지: 메인 폴더가 아닌 연결된 작업 트리마다 HEAD가 `origin/<기본>`에 다 들어갔고(조상이거나 `git cherry`에 `+` 없음 — rebase로 들어간 것도), 깨끗하고(추적 변경·untracked 없음, 무시된 보관물 `.qa/` 없음), 그 폴더를 cwd로 쓰는 hc 터미널이 없고, 마지막 git 활동이 `--min-age`(기본 24h)보다 오래됐고 멈춘 rebase·merge·cherry-pick·bisect가 없을 때만 teardown → `worktree remove --force` → `branch -D`. 하나라도 아니면 `남김 <폴더> <까닭>`.
- 작업 폴더 없이 남은 로컬 가지: 기본 가지·현재 HEAD·작업 폴더에 붙은 가지·설정 `gc.keep_branches`(빈칸으로 나눈 glob 무늬, 기록용 가지)는 빼고, 같은 판정(병합 커밋이 끼면 안 들어감)에 마지막 커밋과 가지 reflog가 `--min-age`보다 오래됐으면 `branch -D`하고 `지움 가지 <가지> <sha>`를 찍는다. 안 들어간 가지는 지우지 않고 `남김 가지 <가지> 안 들어감 <n>개 · 마지막 <날짜>`.
- `--remote-branches`(기본 꺼짐): 원격 `refs/heads/*` 가운데 기본 가지·`gc.keep_branches`를 빼고 같은 판정이면 ls-remote 때 값 그대로일 때만 지운다. 나이는 커밋 시각뿐이라 옛 main 커밋에 방금 만든 원격 가지도 지우고, 로컬 가지의 원격 짝(upstream·PR)도 빼지 않는다. hc-fill의 정리 바퀴는 이것을 켜지 않는다.
- 「들어감」은 커밋마다 공백까지 같은 패치(`git patch-id --verbatim`)가 `origin/<기본>`에 있을 때다. 공백만 다른 변경은 안 들어감.
- `--integ`(기본 접두 `refs/integ/`): 원격 참조마다 커밋이 `origin/<기본>`에 다 들어갔고 커밋 시각이 `--min-age`보다 오래됐을 때만, ls-remote 때 값 그대로일 때만 지우고 `지움 <참조> <sha>`를 찍는다. 메인 폴더·기본 가지·현재 가지·`refs/tags`·`refs/claims`는 건드리지 않는다(원격 `refs/heads`는 `--remote-branches`일 때만).
- 지운 참조 되돌리기: `git push origin <sha>:<참조>`(찍힌 sha 그대로). 지운 가지는 `git branch <가지> <sha>`.
- 마지막 줄 `지움 n · 남김 m`. hc ls·fetch 실패는 아무것도 지우지 않고 종료 1. hc-fill이 설정 `gc` 분마다(기본 60) 부른다.

## 고아 프로세스 (레인이 남긴 배경 프로세스)

- **검증에서 막기**: `.claude/herdr-verify.sh`는 시험마다 따로 프로세스 무리(pgid)로 돌린다. 시험이 끝난 뒤 그 무리에 남은 프로세스, 또는 검증 중 새로 생겨 부모가 1이 된 프로세스 가운데 명령 줄·cwd에 이 폴더 경로가 든 것(과 자손)은 고아다. 목록을 찍고 끄고(TERM, 3초 뒤 KILL) 검증 실패.
- **늘 쓸기**: `hc-sweep`(launchd 5분마다)이 끝난 터미널을 닫은 뒤, 이 사용자의 PPID 1 프로세스 가운데 명령 줄의 절대 경로나 cwd가 레인 작업 트리(`<저장소><접미사>/<레인>`) 아래이고 `--min-age`분(기본 10) 넘은 것을 본다.
  - 남김: 그 작업 트리 아래를 cwd로 쓰는 live hc 터미널이 있음(실제 경로로 견줌).
  - 보류(찍기만): 작업 트리가 있는데 본 저장소 아래를 쓰는 터미널이 있음(감사 스냅숏 꼴), 작업 트리가 있는데 cwd로만 걸림, 자손에 live 터미널 프로세스가 있음.
  - 끔: 그 밖(작업 트리가 지워졌거나 쓰는 터미널 없음). 그것과 자손에 TERM, 5초 뒤 KILL. pid·나이·명령 앞 80자를 `~/.claude-tasks/closed.log`에.
  - 터미널 닫기 R2(일 끝난 worker)는 parent가 있거나 라벨 `hc-lane=1`인 것 — hc-fill이 띄운 레인은 parent가 없다. 이 기계 hc-fill 대기열에 결과를 아직 되돌려 주지 않은(`up`) 받은편지 레인은 닫지 않는다.
  - `hc-sweep --dry-run`은 찾기만. hc ls 실패·terminals 목록 아님·설정 읽기 실패·cwd 모르는 이 기계 터미널이면 아무것도 끄지 않는다.
- 설정 키: `sweep.wt_suffix`(기본 `.wt`), `sweep.orphan_keep`(보지 않을 실행 파일 이름 — `ps comm`(argv[0])으로 견주어 `…/Herdr Core.app/…/herdr-core`처럼 공백 든 경로도 맞음, 기본 `codebase-memory-mcp herdr-core`), `sweep.orphan_kill`(`on`·`off`, 기본 `off` = 찍고 closed.log에 pid·명령마다 「고아 관찰」 한 줄 — 기록을 며칠 보고 `on`). 설정 도구(`herdr-tools-config`, sync.list mac-only)가 곁에 없는 기계는 늘 기본값이라 off(관찰만)다. 켜려면 그 기계에 도구가 있어야 한다.
- **색인 쓸기**: `hc-sweep`은 매 바퀴 먼저(hc ls 실패와 상관없이) codebase-memory 색인 폴더(`${XDG_CACHE_HOME:-~/.cache}/codebase-memory-mcp`)의 `<이름>.db`(`_config.db` 빼고) 가운데 projects 표의 root_path가 모두 지워진 것을 `-wal`·`-shm`·`-journal`과 함께 지우고 closed.log에 `색인:<이름>`·root·크기를 남긴다. `--min-age`분 안(색인 중)·못 읽음·root가 `/Volumes/` 아래(링크를 풀어서도)·root 읽기 오류면 남긴다. 지운 것이 없으면 요약 줄도 없다. teardown이 돌지 않은 워크트리(옛 워크트리·손으로 지움·rollback)의 색인이 기계마다 수십 GB 쌓였다(2026-11-18).
- 레인 지시문(`hc-lane --print-steps`) 끝 단계에 「끝내기 전 `pgrep -fl <작업 트리>/`로 확인, 자기 셸·에이전트는 빼고 정확한 pid로 kill」 줄이 붙는다.
- 쓰기·읽기 레인 지시문(`--print-steps`, `--read` 포함 — hc-fill run 지시문도 이것을 쓴다) 머리에 「임시 파일은 `mktemp -d` 새 폴더, `rm -rf`·`rm -f`에 변수·와일드카드 경로 금지(꼭 지우면 글자 그대로나 `"${X:?}"`)」 줄이 붙는다. 그런 rm은 Claude Code가 권한 창을 띄워 무인 레인을 멈춘다.

## hc-fill (빈 자리에 기다리는 레인을 바로 띄우기, 맥)

```sh
hc-fill add --name <레인> --project <프로젝트> --brief <지시문> [--after <레인>,…] [--only <기계>,…] [--coord <지휘>] [--priority <긴급|높음|보통|낮음>] [--no-early]   # 대기열 끝에
hc-fill prio <레인> <긴급|높음|보통|낮음>   # 대기 중인 레인의 우선순위 바꾸기(영어 urgent|high|normal|low도 받음)
hc-fill ls                      # 대기 · 떠 있음(hc 상태) · 끝남 · 되돌려줌(받은편지 결과를 보냄) · 주인 지휘 · 우선 · 먼저 띄움(선행 …)
hc-fill run --dry-run           # 무엇을 어디에 띄울지만. --dry-run 없이 한 바퀴 띄운다
hc-fill watch --every 60        # 계속(한 번에 하나만, 이미 돌면 종료 75)
hc-fill rm <레인>               # 대기열에서만 뺀다(떠 있는 터미널은 그대로)
```

- **지시문은 add 때 복사된다**: 내용을 `~/.local/share/hc-fill/briefs/added/<레인>.md`로 복사하고 대기열에는 그 경로를 적는다(launchd로 도는 `watch`는 macOS 개인정보 보호로 `~/Documents` 등을 못 읽는다). 그 뒤 원본을 고쳐도 반영되지 않는다(`rm` 뒤 다시 `add`). 못 읽거나 빈 지시문은 `add`가 거절하고, `run`도 띄우기 전에 실제로 읽어 보아 못 읽으면 「실패 <레인>: 지시문을 읽지 못함(<까닭>)」을 찍고 띄우지 않는다(대기로 남음).
- **주인 지휘**: `add`는 부른 지휘(`--coord`, 없으면 `$HC_TERMINAL_ID`)를 대기열 줄에 적고 `hc-lane coord --set`으로 지휘 정보를 남긴다. 띄울 때 그 지휘가 아직 살아 있으면(`hc-lane coord --check`) 그 지휘를 `HC_LANE_OWNER`(레인 기록 주인)·`HC_LANE_COORD`(watch 대상)로 넘겨, 그 레인의 `hc-lane report`·멈춤 알림은 그 지휘 한 곳에만 간다. local에 띄운 레인도 그 지휘 옆에 붙인다. 지휘 터미널이 아니면(레인 안·터미널 밖) 주인 없이(`-`) 넣고 한 줄 알린다 — 그 사건은 모든 지휘에게 간다. `--coord`로 준 주소가 지휘가 아니면 거절한다. 받은편지 요청은 주인이 없다. 대기열 레인은 `--after` 사슬로 몇 시간 뒤에 뜰 수 있어, 띄울 때 주인 지휘가 사라졌으면 주인 없이(`-`) 띄우고 watch는 `hc-lane coord` 지휘에게 맡긴다(알림 한 줄).
- **우선순위**: 긴급 > 높음 > 보통 > 낮음. `add`는 `--priority`가 없으면 보통, 받은편지 요청은 `.req`에 `priority`가 없으면 높음이다(회원이 겪는 오류 수정이 기능 개발보다 먼저). 긴급도 자리가 없으면 기다리고(돌던 레인을 끊지 않는다), 다음 빈자리를 가장 먼저 잡는다. `prio`는 대기(`wait`) 중인 레인만 바꾼다.
- 한 바퀴: 대기 레인을 우선순위 높은 것부터, 같으면 넣은 차례대로 본다. 선행이 안 풀린 레인은 건너뛰고 다음 레인을 본다. `--after` 레인이 모두 local 기계 저장소의 `origin/<기본>`에 들어갔으면 풀린다(판정 줄을 찍는다). 들어갔다는 것은 제목 머리가 레인 이름인 커밋이 있다는 뜻이다: 제목이 `<레인>…` 또는 `<종류>(<범위>): <레인>…`(단어 경계). 괄호·문장 안 언급과 본문은 세지 않는다. 설정의 machine 차례에서 처음 자리가 남는 기계에 `hc-lane --on <기계> --repo <그 기계 경로> -- <launch 틀>`로 띄운다. 지시문에 `hc-lane --print-steps` 착지 줄을 붙인 run 지시문은 그 기계 `~/.local/share/hc-fill/runs/<레인>-run.md`에 둔다. local 기계에 띄운 것은 `herdr-balance --main <주인 지휘, 없으면 설정 coord> --add`로 붙이고, 다른 기계에 띄운 것은 hc-lane이 맥 창의 그 기계 공간에 탭으로 보이게 한다.
- **자리 세기**(run 출력 「자리 <기계> <주소> <이름>: …」 줄): working·blocked는 늘 센다. done·idle은 hc-lane 작업 트리(`<저장소>.wt/…`)가 그 기계에 남은 쓰기 레인만 세되, 둘은 빼고 센다(2026-11-19 — 착지한 레인·지휘를 기다리는 레인이 사흘 동안 444바퀴 대기 레인의 자리를 막았다): ① **착지함** — 터미널 이름(=레인 이름)이 그 프로젝트 local 저장소 `origin/<기본>`의 커밋 제목 머리에 있으면(위 선행 판정과 같은 규칙, fetch도 바퀴에 한 번 같이 씀) `착지함(<sha>) → 안 셈`. 프로젝트는 cwd의 `<저장소>.wt/…` 앞 폴더 이름과 `repo` 줄 경로의 이름을 맞춰 찾는다. 못 찾거나 fetch가 실패하면 판정 못 함이라 센다. ② **오래 쉼** — 착지 안 했어도 done·idle로 이어서 설정 `hold`분(기본 60, `0`이면 끔)이 넘으면 `쉰 지 n분(hold m) → 안 셈`. 쉬기 시작한 때는 `~/.local/share/hc-fill/held.tsv`(주소<TAB>처음 본 시각)에 적고, working·blocked로 바뀌거나 사라진 주소는 빠진다(dry-run은 쓰지 않는다). 자리 세기 전에 끝나는 바퀴(빈 대기열 + `idle 0`, hc 실패 등)는 이 파일을 비운다 — 못 본 사이 다시 일한 레인이 묵은 시각을 물려받지 않게. ③ **읽기 레인** — 감사·조사(`hc-lane --read`)가 레인 작업 트리를 cwd로 써도, 이 기계 hc-lane 레인 기록(`~/.local/state/hc-lane/lanes.tsv`)에서 그 주소의 마지막 종류가 `read`면 `읽기 레인(레인 기록) → 안 셈`(기록이 없으면 위 판정). 지휘를 기다리던 레인이 나중에 착지(검증)를 다시 돌려 자리가 잠깐 넘쳐도, 검증 부하는 저장소 쪽 검증 잠금(기계마다 검증 자리 수)이 막는다. 셈 칸이 `all`인 기계는 다 센다(①·② 없음).
- **먼저 띄움**(2026-11-18 — 큰 일을 계약 → 바탕 → 영역으로 쪼개면 대기열 거의 모두가 한두 선행에 묶여, 자리가 30분 넘게 비어도 띄울 것이 없었다): 띄울 수 있는 것을 다 띄우고도 자리가 남으면, 선행이 「아직」인 대기 레인(판정 못 함 — fetch 실패 등 — 은 빼고, `add --no-early` 줄도 빼고)을 같은 차례(우선순위·넣은 차례)로 띄운다. 선행 가운데 대기열에 아직 안 뜬(`wait`) 레인이 있는 줄은 빼서 그 선행이 자리를 먼저 잡게 한다(그 선행이 이번 바퀴에 떴으면 — 먼저 띄움 포함 — 후보. 대기열 밖 선행은 그대로) — 뒤 레인이 자리를 다 먹으면 선행이 못 떠 기한까지 교착했다(감사 [중간] 1). run 지시문 끝에 「먼저 띄움」 절이 붙는다: ① 지금은 준비만(지시문·계약 읽기, 자기 몫 파일의 실패 시험·골든 고정, 선행이 고칠 파일은 아직 안 고침) ② `until git fetch -q origin && <선행마다 따로 한 줄: 커밋 제목 머리 판정 — 위 선행 판정과 같은 규칙> do … sleep 180; done`으로 기다림(배경 실행 금지 — `&`·nohup도 도구의 run_in_background도, 명령 시간 한도는 도구 최대로, 기한은 띄운 시각 + 설정 `early_wait`시간(기본 5)의 고정 시각 — 넘으면 「선행 대기 초과」를 `hc-lane report 막힘`하고 멈춤) ③ 선행이 들어가면 `git rebase origin/<기본>` 뒤 본 작업. 대기열 14번째 칸이 `early`가 되고 `ls`에 「먼저 띄움(선행 …)」. 살아 있는 먼저 띄움 레인 수의 상한은 설정 `early`(키 혼자·빼면 남은 자리 전부, `0`이면 끔). 같은 선행에 묶인 레인끼리도 함께 띄운다 — 파일 겹침은 지시문 몫이다. 선행의 산출물 없이는 준비조차 못 하는 레인은 `add --no-early`로 넣는다. 기한을 넘겨 「선행 대기 초과」로 멈춘 먼저 띄움 레인은 대기열 줄이 떠 있음(`up`)으로 남아 선행이 들어가도 다시 뜨지 않는다 — 지휘가 `hc-fill rm <레인>` 뒤 다시 넣는다(보통 레인의 막힘과 같은 규칙).
- **놀고 있음 알림**: 바퀴 끝에도 자리가 남은 기계(연결됨, 그 기계 `repo` 줄이 있음)가 설정 `idle`분(기본 10, `0`이면 끔) 넘게 이어지면 `hc-lane report 중간 - "<기계> 자리 n개 m분째 빔 · … — 대기열에 띄울 것 없음" --name hc-fill-idle [--to <지휘>,…]` 사건 하나를 남긴다 — 그 바퀴에 띄울 것도 먼저 띄울 것도 없었다는 뜻이다. 받는 쪽은 대기 줄의 주인 지휘 가운데 살아 있는 것(`hc-lane coord --check`가 「다름·없음」이 아닌 것)이고, 주인 없는 대기 줄이 있거나 대기 줄이 없거나 살아 있는 주인이 없으면 `--to` 없이(모든 지휘 watch가 보낸다). 지휘의 `hc-lane watch`가 그 사건을 지휘 터미널에 밀어 주니 지휘가 일을 더 넣는다. 같은 기계가 계속 비면 처음 알림 포함 30분 간격으로 세 번, 그 뒤로는 2시간마다(밤새 일이 없을 때 지휘 턴이 끝없이 나지 않게), 자리가 차면 그 기계 기록을 지운다(다시 비면 처음부터 셈). 사유는 둘로 나눈다: 먼저 띄움 상한(`early`)에 걸려 후보를 남겼으면 「먼저 띄움 상한(early n)에 걸려 남김」, 아니면 「대기열에 띄울 것 없음」. 보내기에 실패하면 한 줄 찍고 다음 바퀴에 다시. `--to`는 watch만 지킨다 — 끌어오는 `hc-lane wait`는 `--state`가 없으면 이 `중간` 사건에도 깨니, 레인 하나를 기다릴 때는 `--state 완료,착지,막힘,판정,멈춤`처럼 상태를 준다. 대기열이 비어도 이 알림을 위해 자리를 본다: 바퀴마다 `hc ls`·`hc machine ls`와 원격 done·idle 작업 트리 확인(`hc exec --on`)을 돌고, 빈 대기열이어도 설정 `launch`·`task` 줄이 틀리면 종료 1이다(`idle 0`이면 예전처럼 바로 끝냄, 종료 0). 기록 `~/.local/share/hc-fill/idle.tsv`, dry-run은 보지 않는다.
- 띄운 뒤 `HC_FILL_WAIT`초(기본 60) 안에 `hc get` status가 안 잡히면 그 기계를 그 레인에서 빼고 다음 기계로 간다. 그 터미널은 끄지 않고 `ls` 비고에 남긴다. 남긴 터미널이 나중에 status가 잡히면 「중복 위험 <레인> <주소>」를 찍고 그 레인은 더 띄우지 않는다(지휘가 하나를 정리).
- 같은 이름 터미널이 어디선가 살아 있으면 띄우지 않고 떠 있음으로 표시한다. 연결 안 된 기계는 건너뛴다. `hc ls`·`hc machine ls`가 실패하거나, `hc ls`에 설정한 `agent` 종류가 하나도 없으면(지휘 자신도 그 종류라 늘 하나는 있다 — 설정 오타·hc 이름 바뀜) 그 바퀴는 아무것도 띄우지 않는다(종료 1). 아무것도 지우거나 끄지 않는다.
- **받은편지**(설정 `inbox` 줄이 있을 때): 바퀴 처음에 그 기계 폴더의 `<이름>.req`(「키<TAB>값」 줄: `project`·`after`·`only`, 없으면 `-`. `priority`는 `긴급|높음|보통|낮음`(영어도 받음), 없으면 높음)를 검사해 `<이름>.brief.md`를 맥 `~/.local/share/hc-fill/briefs/<기계>/<이름>.md`로 복사하고 대기열에 넣은 뒤, 그 기계에서 둘을 `taken/`으로 옮긴다. 이름 글자·설정에 없는 project·only 기계·after 이름·모르는 priority 값·대기열에 이미 있는 이름·지시문 없음(공백만 든 빈 지시문 포함)은 둘을 `rejected/`로 옮기고 `rejected/<이름>.why`에 사유 한 줄. 실행 명령·경로·모델은 받지 않는다(설정이 정한다). 요청 쪽은 `.brief.md`를 먼저 쓰고 `.req`를 마지막에 `mv`로 둔다. 연결 안 된 기계는 건너뛴다(요청은 그 기계에 쌓여 있다가 다음 바퀴에).
- 받은편지 레인의 run 지시문 끝에는 「끝나면 이 기계 `~/.local/share/hc-fill/reports/<이름>.md`에 결과를 쓰고 마지막 줄은 `결과: 고침 <sha>`·`결과: 넘김 <사유>`·`결과: 실패 <사유>`」 절이 붙는다. 터미널이 사라졌거나 done·idle이고 보고가 있으면 그 보고를 요청한 기계 `<폴더>/done/<이름>.md`로 그대로 쓰고(상태 `sent`), 보고 없이 터미널이 사라지면 `결과: 실패 보고 없음`을 쓴다. 레인 기계나 요청한 기계가 연결 안 됨이면 다음 바퀴로 미룬다.
- 대기열 `~/.local/share/hc-fill/queue.tsv`(`HC_FILL_DATA`로 바꿈). 11번째 칸은 되돌릴곳(`<기계>:<경로>`, 손으로 넣은 줄은 `-`), 12번째 칸은 주인 지휘(없으면 `-`), 13번째 칸은 우선순위(`urgent|high|normal|low`), 14번째 칸은 먼저 띄움(`-` 가능 · `no-early` 끔 · `early` 먼저 띄움으로 띄움). 10~13칸 옛 줄도 그대로 읽는다(주인 없음, 우선 보통, 먼저 띄움 가능).
- **설정** `${HERDR_TOOLS_CONFIG:-~/.config/herdr-tools}/hc-fill.tsv`: 코드에는 기계·경로·실행기 같은 개인 값이 없고 모두 여기서 읽는다. 저장소의 `config.example/hc-fill.tsv`(가짜 값)를 복사해 고친다. 줄마다 첫 칸이 종류다. 한 값 키(`local`·`agent`·`coord`·`task`·`launch`·`gc`·`early`·`early_wait`·`idle`·`hold`)는 키 혼자면 빈 값이고, 같은 키가 두 줄이면 멈춘다(종료 1).

| 줄 | 칸 | 뜻 |
|---|---|---|
| `local` | 이름 | 이 기계(`--on` 없이 띄우는 기계)의 이름 |
| `agent` | 종류 | 셀 에이전트(`hc ls --json`의 `agent`, 기본 `claude-code`) |
| `coord` | 주소 | 지휘 터미널. 주인 없는 줄을 local에 띄울 때 그 옆에 붙인다(`HC_FILL_COORD`가 앞선다, 주인 지휘가 있으면 그 옆). 없으면 붙이지 않는다 |
| `task` | 문장 | 첫 지시. `{run}` 자리에 run 지시문 경로 |
| `launch` | 명령 틀 | `hc-lane … --` 뒤 실행 명령(셸 문법으로 나눈다). `{task}` 자리에 첫 지시가 한 인자로. 모델·추론·실행기를 여기에 |
| `gc` | 분 | 끝난 레인 정리 간격(기본 60, `0`이면 끔). 처음 깔 때는 `gc 0`으로 두고 `hc-lane gc … --dry-run`을 본 뒤 켠다. run 바퀴 끝에 그 간격마다 한 번, `repo` 줄의 기계·저장소마다 `hc-lane gc`(연결 안 된 기계는 건너뜀), `--integ`는 local 기계 저장소에서만. 시각은 `~/.local/share/hc-fill/gc.last` |
| `early` | 개수 | 먼저 띄움 상한: 살아 있는 먼저 띄움 레인 수. 키 혼자·빼면 남은 자리 전부, `0`이면 끔 |
| `early_wait` | 시간 | 먼저 띄움 레인의 선행 기다림 기한(기본 5) |
| `idle` | 분 | 놀고 있음 알림: 자리가 이만큼 이어서 비면 지휘 watch에 사건(기본 10, `0`이면 끔). 계속 비면 처음 알림 포함 30분 간격으로 세 번, 그 뒤 2시간마다 |
| `hold` | 분 | 작업 트리가 남은 done·idle 레인도 이만큼 이어서 쉬면 자리를 안 센다(기본 60, `0`이면 끔 = 늘 셈). 위 「자리 세기」 ② |
| `machine` | 이름 상한 레인상한 셈 | 위에서부터 차례. 그 기계의 자리를 차지한 `agent` 수가 상한 **미만**일 때만 띄운다. working·blocked는 늘 차지하고, done·idle은 hc-lane 작업 트리(`<저장소>.wt/…`)가 그 기계에 남은 쓰기 레인(감사·착지 기다림)만 차지한다 — 읽기 전용 레인·작업 트리가 정리된 레인·착지한 레인·`hold`분 넘게 쉰 레인은 아니다(판정은 run 출력 「자리 …」 줄). 레인상한은 hc-fill이 띄워 살아 있는 레인 수의 상한(`-`는 없음) — 다른 일이 먼저인 기계를 조금만 쓸 때. 셈이 `all`이면 done·idle도 다 센다(쉬는 동안 done으로 보이는 루프가 있는 기계) |
| `repo` | 프로젝트 기계 경로 | 그 기계의 저장소. 경로가 없는 기계에는 그 프로젝트를 띄우지 않는다. local 기계 경로로 선행 판정 |
| `inbox` | 기계 폴더 | 받은편지를 걷어 올 기계와 그 기계의 폴더(`~/`는 그 기계 HOME). 여러 줄 가능, 없으면 받은편지를 보지 않는다 |

- 늘 돌리기(기본 꺼짐): `cp launchd/com.herdr-tools.hc-fill.plist ~/Library/LaunchAgents/ && launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.herdr-tools.hc-fill.plist`. 끄기 `launchctl bootout gui/$(id -u)/com.herdr-tools.hc-fill`. 기록 `~/.local/state/hc-fill/watch.log`.

## 설정

개인 값(기계 이름·경로·계정)은 저장소 밖 설정 폴더 `HERDR_TOOLS_CONFIG`(기본 `~/.config/herdr-tools`)에 둔다. 꼴과 키는 `config.example/`(가짜 값)을 본다.

```sh
herdr-tools-config get balance.remote ""   # 값. 키가 없으면 기본값, 기본값도 없으면 종료 3
herdr-tools-config file hc-fill.tsv        # 설정 폴더의 그 파일 절대 경로. 없으면 종료 3
herdr-tools-config lint [<파일>…]          # git ls-files(또는 준 파일)에서 개인 값 패턴(private-patterns.txt)에 걸린 줄. 걸리면 종료 1
```

- `config`는 한 줄에 `<키> <값>`(값은 줄 끝까지), 키 혼자면 빈 값, `#` 줄은 주석. 같은 키 두 번·잘못 부름은 종료 2.
- lint 패턴은 `$HERDR_TOOLS_CONFIG/private-patterns.txt`, 없으면 `~/.local/share/herdr-tools/private-patterns.txt`. 둘 다 없으면 건너뛴다(종료 0).
- lint는 이진·UTF-8 아닌 파일도 바이트로 본다(`LC_ALL=C grep -a` — macOS grep `-I`는 그런 파일을 통째로 건너뛰었다). 상대 경로로 받은 파일(인자 없을 때의 `git ls-files` 포함)은 경로 글자도 보고 `파일:0: 경로 <패턴>`으로 찍는다(절대 경로는 임시 파일 자리라 내용만). 못 읽는 파일이 있으면 종료 2(검사 못 함). `git ls-files`에 있지만 작업 트리에서 지운 파일은 뺀다. 패턴은 바이트 단위다(`LC_ALL=C` — 한글 같은 비ASCII 글자에서 `.`은 글자 하나가 아니라 바이트 하나). 대소문자는 무시한다(`-i`, ASCII만 접힘).

## 시험

```sh
bash .claude/herdr-verify.sh                             # 문법 검사 + tests/*.test.sh 전부 (hc-lane land가 부른다)
HC_LANE_BIN="$PWD/bin/hc-lane" bash tests/hc-lane.test.sh   # 절대 경로로 준다(HC_LANE은 레인 이름)
bash tests/herdr-sync.test.sh                            # 임시 HOME 셋·가짜 hc·임시 범용·개인 저장소. 실제 기계에 쓰지 않는다
bash tests/hc-fill.test.sh                               # 가짜 hc·hc-lane·git과 config.example 설정. 실제 레인을 띄우지 않는다
bash tests/herdr-tools-config.test.sh                    # 가짜 설정 폴더로 get·file·lint
bash tests/hc-sweep.test.sh                              # --self-test + 진짜 짧은 고아와 가짜 hc(--only-under 시험 폴더)
bash tests/herdr-verify.test.sh                          # 가짜 저장소에서 고아를 남기는 시험이 검증을 실패시키는지
bash tests/herdr-tools-publish.test.sh                   # 임시 비공개·공개 bare 저장소로 내보내기·막기
```

hc-lane 시험은 `HC_LANE_BIN`이 없으면 같은 폴더(`tests/`)나 그 위의 `hc-lane`을 찾는다. 이 저장소에서는 `bin/`에 있으니 `HC_LANE_BIN`을 준다(`HC_LANE`은 레인 이름이라 쓰지 않는다).
