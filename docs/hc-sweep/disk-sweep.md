# hc-sweep: 디스크 쓸기

- **디스크 쓸기**(`cmd/hc-sweep/disk.go`, 색인 쓸기처럼 따로 돈다): `$TMPDIR`(launchd에 없으면 `getconf DARWIN_USER_TEMP_DIR`) 바로 아래 내 폴더 `go-build*`·`Test*` 가운데 안쪽 모든 항목이 `sweep.tmp_hours`시간(기본 3) 넘게 안 바뀌었고 lsof로 아무 프로세스도 열지 않은 것만 지우고 closed.log에 「임시 찌꺼기 지움」 한 줄(개수·크기).
  lsof가 실패하면 하나도 안 지운다.
  홈 볼륨 여유가 `sweep.disk_free_gb`(기본 10) 아래면 `hc-lane report 중간 --name hc-disk-low` 사건 한 번(볼륨 사용률과 임시 폴더·`~/Library/Caches/go-build`·`~/.cache/go-build` 크기 함께 — 알림은 Go 캐시를 지우지 않고 크기만, `~/Library/Caches/go-build`는 macOS 기본 GOCACHE일 수 있다.
  watch 줄 끝 괄호에 기계 이름), 여유가 문턱의 1.5배를 넘으면 다시 알릴 수 있게 된다.
  표식 `<상태>/hc-sweep/disk-low`를 「대기」로 먼저 쓰고 `--key hc-disk-low:<표식 시각>`으로 보낸 뒤 「보냄」으로 바꾼다 — 보내다 끊기거나 실패하면 다음 바퀴가 같은 키로 다시(hc-lane이 같은 키는 한 번만 적는다).
  검증이 ENOSPC로 멈췄다(2026-11-21 워커 맥 여유 1GB).
  lsof는 launchd PATH에 없는 /usr/sbin에서도 찾는다.
  **Go 빌드 캐시 상한**: 여유가 문턱의 1.5배 아래면 `GOCACHE`(없으면 `~/.cache/go-build` — 병합기 기본과 같음)의 `<두 자리 16진>/` 아래 항목을 오래된 것부터 지워 `sweep.gocache_gb`(기본 6, 0이면 끔) 아래로, `GOCACHE`와 다른 옛 자리 `~/Library/Caches/go-build`는 통째로 비운다.
  go 명령이 돌면 건너뛰고, 한 일은 closed.log 「Go 캐시 줄임」(2026-11-21 mini-m4 여유 9GB — 레인 작업 트리마다 빌드해 하루 13.4GB).
  시험 `tests/util/hcsweepdisk_test.go`.
