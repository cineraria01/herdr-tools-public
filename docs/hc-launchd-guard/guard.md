# launchd 작업: 지킴이(`hc-launchd-guard`)로 Herdr 터미널에서

launchd가 바로 띄운 새 Go 바이너리는 `~/Documents`에 처음 닿을 때 macOS 개인 정보 보호 창(「'hc-fill'이(가) 문서 폴더의 파일에 접근하려고 합니다」)을 띄우고 사람이 누를 때까지 잠든다.
Go 바이너리는 빌드마다 서명이 바뀌어 한 번 허용해도 새 판마다 다시 묻는다(2026-11-21 맥 hc-fill 37분 멈춤).
Herdr 터미널(herdr-core가 띄움)과 `hc exec`에서는 창이 안 뜬다.
그래서 지킴이로 옮긴다.

- **지킴이**: launchd는 `hc-launchd-guard [--log <파일>] <이름> -- <도구> [인자…]`만 띄운다.
  이 기계에 `<이름>` 터미널이 살아 있으면 아무것도 안 하고, 없으면 `hc run --bg --no-parent --outlive-parent --rm --name <이름> --cwd $HOME -- /usr/bin/env -u HC_TERMINAL_ID -u HC_SCOPE -u HC_TERMINAL_SOCKET <도구 절대 경로> …`로 Herdr 터미널에서 돌린다.
  터미널 표식을 지워 도구는 launchd에서처럼 「터미널 밖」으로 돈다(도구가 띄우는 레인이 그 터미널의 자식이 되어 같이 끝나지 않게).
  `hc ls`(20초)·`hc run`(60초)에는 상한이 있다.
  모르면(hc ls 실패·시간 넘김·목록 아님·같은 이름 행의 주소·상태 빠짐·모름)과 `hc run` 실패 뒤 터미널이 없으면 늘 75로 끝나 launchd가 다시 부르게 한다 — 그 자리에서 도구를 돌리는 길은 없다.
  지킴이 자신은 cwd를 `$HOME`으로 옮기고 `~/Documents`에 닿지 않아 launchd에서 돌아도 창이 안 뜬다(시험 `TestTCC`).
  옛 plist처럼 `python3 <파일>`로 넘겨도 그 파일이 파이썬이 아니면 해석기를 빼고 그 파일을 직접 돌린다.
  상주 작업은 `KeepAlive`+`ThrottleInterval 60`으로 60초마다 지킴이가 다시 불려 죽은 터미널을 다시 띄우고, 주기 작업(`StartInterval`)은 앞 판이 돌면 건너뛴다.
  예시 plist `launchd/`(hc-fill·hc-sweep·herdr-balance).
  시험 `cmd/hc-launchd-guard/guard_test.go`.
- 지킴이 없이 도구를 바로 띄우는 launchd 작업은 창에 멈출 수 있다.
  모두 지킴이로 옮긴다.
