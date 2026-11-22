# Go 작은 도구

- **작은 도구**(2026-11-21 util-go, 사용자 「모두 고로 옮기고 싶어」): `hc-sweep`·`herdr-balance`·`ovis-split`·`worker-mac-check`·`worker-mac-setup`은 `cmd/<도구>`다.
  worker-mac-*는 원격에서 도는 부분을 bash 조각(go:embed한 `remote.sh`·`mesh.sh`)으로 보내고 상한 시간·받은 줄 파싱·설정 읽기를 Go로 한다(공통 `internal/workermac`).
  인자 해석·JSON 꼴 공통 `internal/argp`·`internal/pyjson`. 계약(2026-11-21 Go 전환 감사): `herdr-balance --watch`는 잠금 `~/.local/state/herdr-balance/watch.lock`(flock — 남이 쥐면 75)을 쥐고 TERM·INT면 143·130으로 끝남, 목록에 필요한 칸(`spaces`·`tabs`·`terminals` 등)이 없으면 맞춤 실패(1)·상태 안 씀, `worker-mac-setup mesh --dry-run`은 읽기만·「새로 넣은 것」은 한 번씩, `-`로 시작하는 기계 이름은 64, 원격 점검·설정 본문이 끊기면(ssh 255·시간 넘김·끝 표식 없음) 실패. herdr-app 한 번 15초 상한·worker를 떼던 맞춤은 신호가 와도 마침, `hc machine ls`를 JSON으로 풀어 조회 실패면 등록 안 함·`hc machine add --install` 600초 상한, `ovis-split`은 주소가 없는 대상이 있으면 라벨 전에 1. 시험 `tests/util/`.
