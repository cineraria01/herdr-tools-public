# hc-sweep: 시험 고아 점검 (부모 없이 남은 시험 프로세스)

- 2026-11-26 ht-test-orphan-leak — 시험이 띄운 프로세스가 시험 뒤에도 며칠씩 남았다: 워커 맥 하나에 `herdr-test-bin.*/hc-merger --repo …/TestRecoveryResidentSleep…`(3일, 「옛 판 상주」로 잘못 잡힘), 다른 워커 맥에 `herdr-verify-ram-*/t/herdr-test-bin.*/herdr-sync --check`(30시간 CPU 275% — 병합기가 그 기계를 늘 「바쁨」으로 봄), 지휘 기계에 `…/T/Test…/fake-agent`(9일).
- 까닭: 시험의 자식은 새 무리로 뜬다(clitest·proc). 시험 바이너리가 끊겨 죽으면(마감 panic·TERM·KILL) `t.Cleanup`이 돌지 않아 그 무리가 부모 1로 남는다. 검증의 고아 검사는 저장소 경로만 보아 임시 폴더 아래 것을 놓쳤고, 메모리 디스크를 `-force`로 떼면 그 위에서 돌던 실행 파일의 코드 쪽이 사라져 프로세스가 CPU를 태운다(바쁜 고리가 아니다 — 운영의 `~/.local/bin` 실행 파일에는 안 난다).
- 막는 것 셋:
  - 검증(`tests/repo/verify`)은 조각의 TMPDIR을 그 검증만의 폴더(메모리 디스크 볼륨, 없으면 mktemp `herdr-verify-tmp.*`)로 주고, 끝날 때와 끊길 때 메모리 디스크를 떼기 전에 그 아래 경로가 명령 줄에 든 프로세스를 찍고 끈다(있으면 「실패 고아를 남김 — 검증 임시 폴더」). 다른 검증·시험의 것은 그 폴더 밖이라 건드리지 않는다.
  - 상주 판 점검(`internal/resident` `KindOf`)은 인자에 시험 임시 경로가 든 프로세스를 상주로 세지 않는다.
  - 이 점검: 검증 밖(레인이 직접 부른 `go test`가 끊긴 때 등)에서 남은 것을 알린다.
- **재기**: 건강 점검 길(바퀴마다, macOS)에서 `ps -axww -o pid=,ppid=,%cpu=,lstart=,command=` 한 번. 부모가 1이고 인자에 시험 임시 경로(`resident.TestPath` — 길 조각 `herdr-test-bin.*`·`herdr-verify-ram-*`, 또는 임시 폴더 `…/T`·`/tmp` 바로 아래 `Test*`)가 든 것 가운데 뜬 지 `sweep_test_orphan`분(손잡이, 기본 30)을 넘긴 것.
- **알림**: 운영 사건 `hc-test-orphan` 한 번 — `부모 없는 시험 고아: pid <n> (시작 <시각>, <경과>째, CPU <x>%) <명령 줄> — 끄기는 그 pid로: kill <n> (<기계>)`. 보낸 키(기계·pid·시작 시각)는 `~/.local/state/hc-sweep/test-orphans`에 남기고, 사라진 고아의 키는 다음 바퀴에 지운다.
- 끄지 않는다 — 끄기는 사람·지휘가 정확한 pid로. `ps` 실패는 모름(사건 없음).
- 시험: `cmd/hc-sweep/testorphan_test.go`(한 번만·부모 있음·시험 경로 아님·문턱 안·새 고아·ps 실패), `internal/resident/resident_test.go`(시험 경로는 상주 아님), `tests/repo/verify/reap_test.go`(검증 끝·끊김에 임시 폴더 고아를 끔).
