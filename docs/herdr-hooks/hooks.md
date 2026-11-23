# herdr-hooks: 훅

- **훅**(2026-11-21 hooks-go, 셸 훅 파일은 2026-11-22 걷음): `cmd/herdr-hooks`(한 바이너리).
  settings.json의 PreToolUse 훅 명령이 `~/.local/bin/herdr-hooks <훅>`을 바로 부른다(명령 줄 꼴은 `claude/hooks/tests/hooktest_test.go`의 `hookCmds` — 개인 settings에 그 글자 그대로).
  바이너리가 없거나 실행할 수 없으면 안전 장치 `cotenant-guard`만 막고(2, 까닭 한 줄) 나머지 넷은 통과(0).
  실행 파일을 환경 변수로 바꿔 끼우지 못한다.
  keys.json 판별 글자는 훅 이름(`hook:PreToolUse:cotenant-guard` 꼴).
  안전 판정은 모르면 막는다(2026-11-22 gofix-hooks·gofix2-hooks): git·ps·hc는 훅 기한(cotenant 4초·loop-no-question 8초, 로컬 git은 `HERDR_TIMEOUT_GIT`이 더 짧으면 그것) 안에서만 기다리고 넘기면 모름이다.
  그 자식은 감독 무리에서 돈다 — 무리 우두머리는 훅과 관으로 이어진 작은 `/bin/sh`라, 훅이 돌아오거나 기한에 걸리거나 TERM·KILL로 죽으면 손자까지 무리째 끝난다.
  cotenant-guard는 git status·rev-parse 실패·시간 넘김(종료 128 「저장소 아님」이라도 위로 `.git` 표식이 있으면), 온전하지 않은 `status -z` 레코드(이름 바꿈의 짝 빠짐 등), ps 실패(주인은 산 것으로), `hc ls` 실패·2초 넘김·불완전한 목록(동거로), 임대 폴더·owner 쓰기 실패를 통과로 읽지 않고 막으며, 임대 시각을 못 읽으면 종료 1, 따옴표로 지킨 공백이 든 쓰기 대상을 다시 가르지 않고 펼친다.
  hc 에이전트 동거는 `hc ls` cwd(띄운 폴더, 원문 그대로 디스크로)에 더해, 그것으로 아무도 없으면 이 기계의 나머지 에이전트마다 `hc get`의 `foreground.cwd`(지금 폴더)도 본다 — 같은 2초 안에서, 못 얻거나 넘기면 모름 = 동거(2026-11-23 cwd-now-inuse — 띄운 뒤 `cd`로 같은 작업 트리에 들어온 에이전트를 놓쳤다).
  `hc ls`·`hc get` 판독은 `internal/hcjson` 한 곳이다(A5).
  임대 열쇠는 git이 내는 철자다(뿌리·하위 폴더는 디스크의 대소문자, 이름은 core.precomposeunicode의 NFC) — 적은 철자가 달라도 같은 파일은 같은 임대이고, 옛 판이 적은 철자로 남긴 임대도 함께 본다.
  주인을 적기 전에 멈춘 첫 획득은 넘겨받을 때 첫 획득의 검사를 다시 하고, 막힌 첫 획득·여러 대상 Bash의 앞 임대는 거둔다.
  C·UTF-8 밖의 로캘에서 작업 트리에 닿는 쓰기 글롭은 고를 파일을 정하지 못해 막는다.
  loop-no-question은 `hc get` 시간 넘김·실패·판독할 수 없는 응답이면 질문 창을 막는다.
  뜻밖의 패닉은 편의 막 넷 0·cotenant 1. 시험 `claude/hooks/tests/*_test.go`: settings.json과 같은 명령 줄을 밖에서 불러 종료 코드·문구·임대 파일을 본다(HOME마다 `~/.local/bin/herdr-hooks`에 `-tags hookstest` 시험 빌드 — 시험 손잡이 `HERDR_HOOKS_TEST_*`는 그 빌드에만).
