# hc-sweep: 디스크 쓸기

- **디스크 쓸기**(`cmd/hc-sweep/disk.go`, 색인 쓸기처럼 따로 돈다): `$TMPDIR`(launchd에 없으면 `getconf DARWIN_USER_TEMP_DIR`) 바로 아래 내 폴더 `go-build*`·`Test*` 가운데 안쪽 모든 항목이 `sweep.tmp_hours`시간(기본 3) 넘게 안 바뀌었고 lsof로 아무 프로세스도 열지 않은 것만 지우고 closed.log에 「임시 찌꺼기 지움」 한 줄(개수·크기).
  lsof(`cmd/hc-sweep/lsof.go`)는 한 번에 묻고 `-b`(마운트 지점·파일의 stat·readlink 같은 멈출 수 있는 커널 호출을 피함 — 응답 없는 네트워크·디스크 이미지 볼륨에 묶이지 않게)와 `-u <내 uid>`(root가 아니면 남의 프로세스 파일은 어차피 못 보고, 후보도 내 폴더뿐)로 좁힌다(2026-11-24 지휘 기계에서 lsof가 3분 넘어 바퀴마다 건너뛰었다).
  출력은 `-F fn`으로 f 줄까지 요청한다 — 출력 검사가 레코드마다 f·n 줄을 보는데, macOS lsof는 `-F n`에도 f 줄을 내지만 리눅스 lsof는 n 줄만 내 리눅스 기계에서 바퀴마다 「lsof 실패 — 건너뜀」이었다(2026-11-28 ht-sweep-lsof-linux).
  lsof가 실패하거나 상한을 넘기면 하나도 안 지운다(모름은 쥔 프로세스 없음이 아니다). 상한은 `sweep.lsof_secs`초(기본 60, 양수가 아니면 그렇다고 찍고 60) — 프로세스가 많아 lsof가 느린 기계는 이 값을 올린다(2026-11-25 지휘 기계에서 `-b`로 좁힌 뒤에도 2분을 넘겼다).
  후보만 묻는 꼴(`+D <후보>`)은 쓰지 않는다(2026-11-25 ht-sweep-lsof 실측): `-b`와 함께 못 쓰고(lsof 사용 오류), 후보만·cwd만 물어도 전체를 묻는 것과 걸린 시간이 같다 — lsof는 이름을 주어도 내 프로세스의 모든 fd를 커널에 묻고 그 뒤에 맞추므로 비용은 출력이 아니라 열거에 있다. `--dry-run`은 「후보 n개 lsof x초」를, 실패는 걸린 시간과 상한을 찍는다 — 느린 기계에서 값을 정할 근거.
  건너뜀이 `sweep.tmp_skip_alert`바퀴(기본 3, 0이면 끔) 잇달면 `hc-lane report 중간 --name hc-sweep-tmp-skip` 사건 한 번 — 그 기계의 임시 폴더 정리가 꺼진 셈이라는 것을 지휘가 알게, 판단(lsof 손보기·손으로 지우기)은 지휘가 한다.
  표식 `<상태>/hc-sweep/tmp-skip`(횟수·처음 시각·보냄)으로 되풀이하지 않고, 키 `hc-sweep-tmp-skip:<처음 시각>`이라 보내다 끊겨 다시 보내도 한 건, lsof가 되면 표식을 지워 새로 센다.
  hc가 띄운 작업의 임시 파일은 이름이 아니라 자리로 지운다 — 작업별 임시 폴더(docs/hc-sweep/job-tmp.md), 이 이름 꼴 쓸기는 hc가 띄우지 않은 세션 몫이다(꼴을 더하지 않는다).
  홈 볼륨 여유가 `sweep.disk_free_gb`(기본 10) 아래면 `hc-lane report 중간 --name hc-disk-low` 사건 한 번(볼륨 사용률과 임시 폴더·`~/Library/Caches/go-build`·`~/.cache/go-build`·작업 임시 폴더 뿌리 크기 함께, 아래 Go 캐시 정리를 한 뒤의 여유로 판정하고 그 바퀴에 지운 개수·크기를 글에 붙인다.
  watch 줄 끝 괄호에 기계 이름), 여유가 문턱의 1.5배를 넘으면 다시 알릴 수 있게 된다.
  표식 `<상태>/hc-sweep/disk-low`를 「대기」로 먼저 쓰고 `--key hc-disk-low:<표식 시각>`으로 보낸 뒤 「보냄」으로 바꾼다 — 보내다 끊기거나 실패하면 다음 바퀴가 같은 키로 다시(hc-lane이 같은 키는 한 번만 적는다).
  검증이 ENOSPC로 멈췄다(2026-11-21 워커 맥 여유 1GB).
  lsof는 launchd PATH에 없는 /usr/sbin에서도 찾는다.
  **오래 안 쓴 Go 빌드 캐시**: 여유가 `sweep.disk_free_gb` 아래면(알림과 같은 판정) 그 기계의 Go 캐시 자리 셋 — `GOCACHE`, `~/.cache/go-build`(병합기 기본), `~/Library/Caches/go-build`(macOS 기본) — 의 `<두 자리 16진>/` 아래 항목 가운데 mtime이 `sweep.gocache_hours`시간(기본 6, 0이면 끔) 넘은 것만 지운다(없는 자리는 조용히 넘어가고 같은 곳은 한 번).
  go는 쓰는 항목의 mtime을 1시간마다 새로 하니 go가 돌아도 지운다 — Go 자체 정리(5일 안 쓴 항목)와 같은 꼴이고, 캐시 통째 비우기(`go clean -cache`)는 하지 않는다(도는 빌드가 깨진다).
  한 일은 closed.log 「Go 캐시 줄임(N시간 넘게 안 씀)」에 자리·개수·크기·남은 여유, 지웠는데도 문턱 아래면 위 알림.
  (2026-11-26 mini-m4 `~/.cache/go-build` 48GB·여유 8.2GB — 저장소 검증·추측·워커 착지로 하루 40GB 넘게 늘었다. 그전 용량 상한 방식 `sweep.gocache_gb`는 go가 돌면 건너뛰어 일하는 워커에서는 돌지 않았고, `GOCACHE`가 없으면 `~/Library/Caches/go-build`를 「옛 자리」로 통째 비웠다 — 둘 다 걷었다.)
  시험 `tests/util/hcsweepdisk_test.go`(열린 경로는 가짜 lsof — 진짜 lsof 속도에 기대지 않는다)·`cmd/hc-sweep/lsofbound_test.go`.
