# hc-sweep: Tailscale 점검

- **Tailscale 점검**(2026-11-22 go-tailscale-heal): `hc-sweep`은 매 바퀴(hc ls 실패와 상관없이) 이 기계의 Tailscale CLI(PATH의 `tailscale`, 없으면 `/Applications/Tailscale.app/Contents/MacOS/Tailscale`·`/opt/homebrew/bin`·`/usr/local/bin`, `HC_SWEEP_TAILSCALE`=경로 또는 `off`)로 `status`를 본다.
  「Tailscale is stopped.」면 ① 플래그 없이 `up` → ② 「non-default flags」 오류면 그 출력이 알려 준 `tailscale up …` 한 줄의 플래그 그대로 `up`(줄이 하나가 아니거나 인용·`--reset` 같은 낱말이 있으면 손대지 않고 알림만, 로그인 주소는 옮기지 않음) → 결과를 사건 `hc-tailscale` 한 줄(기계 이름 포함).
  시도는 한 시간에 `sweep.tailscale_up_per_hour`번(기본 3, 0이면 알림만, 기록 `~/.local/state/hc-sweep/tailscale-up`), 넘으면 그 시간에 알림 한 번. `client version … != tailscaled server version …` 경고면 그 판 짝마다 알림 한 번(사람이 앱을 열어야 할 수 있다).
  11/21 mini-m4가 앱 자동 업데이트 뒤 두 번 꺼졌다.
  시험 `cmd/hc-sweep/tailscale_test.go`.
