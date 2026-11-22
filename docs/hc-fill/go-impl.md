# hc-fill: Go 구현

- **hc-fill 구현**(2026-11-21 사용자 「모두 고로 옮기고 싶어」, hc-fill-go): `hc-fill …`은 `cmd/hc-fill`이다.
  하위 명령은 그 명령 파일의 `init()`이 `register`로 넣고 `main.go`는 등록표를 돌기만 한다(2026-11-22 go-cmd-registry, 속 명령 `_gc`·`_onland`도 같다).
  도움말은 `usage.go`의 머리(명령 줄 요약)·꼬리(설정·대기열·종료) 사이에 명령마다의 조각 `cmd/hc-fill/help/<명령>.txt`를 이름순으로 잇는다(시험 `cmd/hc-fill/registry_test.go`).
  새 하위 명령은 그 명령 파일과 `help/<명령>.txt`(첫 줄에 「        hc-fill <명령> …」 요약 줄)에서만 더하고, `main.go`·`usage.go`는 고치지 않는다.
  상태 파일 queue.tsv·inbox.tsv·audit.tsv·own.tsv 등. 잠금은 flock(run.lock·queue.lock·watch.lock·gc.lock — O_CLOEXEC), 상태 쓰기는 임시 → fsync → rename(10분 넘게 남은 임시 파일은 다음 바퀴가 거둠).
  ls 「떠 있음」은 주소+이름(끊긴 기계는 「모름」), 감사 틀 `-C {worktree}` 인용, 완료 뒤 멈춤 알림(설정 `done_stall` — 터미널이 닫힌 「끝남」 레인·「커밋 0」 완료·완료 sha가 기본 브랜치에 든 레인은 빼고, 들었는지 모르면 알리지 않음), 바퀴 상한(`watch`가 한 바퀴를 `HC_FILL_ROUND_TIMEOUT`초, 기본 1200 넘기면 끝내고 다음 바퀴로).
  받은편지 레인도 지시문이 「hc-fill 감사」를 요구하거나 `inbox_audit on`이면 자동 감사, 다른 기계에 띄울 때 지시문이 가리키는 지휘 기계 파일 원문을 run 지시문에 붙임(파일 64KiB·합계 256KiB를 넘거나 바이너리면 「(붙이지 않음: 큼/바이너리 — 경로만)」으로 두고 띄움, 비밀 값 꼴이면 안 띄우고 지휘에 한 번).
  붙일 폴더는 설정 `paste_dirs`(빼면 `~/.claude-tasks`, 링크를 푼 실제 경로로 봄)로 한정하고 받은편지 레인(본문을 다른 기계가 씀)에는 붙이지 않는다.
  비밀 값 꼴(2026-11-22 go-secret-fp, `cmd/hc-fill/secret.go`)은 알려진 토큰 머리(개인 키·`sk-`·`ghp_`류·`github_pat_`·`AKIA`·`xox?-`·`AIza`·JWT `eyJ…`)이거나, 비밀스러운 이름(TOKEN·SECRET·KEY·PASSWORD·AUTH·COOKIE·SESSION·CREDENTIAL, 대소문자 무관)에 무작위해 보이는 값(영숫자 16자 넘게 이어짐·글자와 숫자 섞임·엔트로피 3비트/자 이상)이 붙은 것뿐이라 `mode=block`·`token = os.environ[…]` 같은 글은 지나간다.
  막을 때 알림에는 줄 번호와 이름만, 값은 찍지 않는다.
  **바깥 명령이 시간 상한에 걸린 결과는 모름**(gofix-fill): hc-lane 띄우기가 시간을 넘기거나 주소를 못 읽으면 시도 기록(try.tsv 주소 칸 `?`)을 남기고 다른 기계에 또 띄우지 않는다 — 다음 바퀴에 그 기계에 같은 이름 터미널이 보이면 채택, 연결됐는데 없으면 안 뜬 것으로 보고 다시, 끊겼으면 기다림(띄운 직후 run이 죽은 줄도 같다).
  선행 판정의 fetch 실패·시간 넘김은 대기열에서 빠진 선행을 「끝남」으로 정하지 않고, 주인 지휘 확인(`hc-lane coord --check`)이 종료 1(없음) 말고 실패하면 주인을 풀지 않고 그 레인을 다음 바퀴로 미룬다.
  hc ls 항목에 주소가 없거나, hc get이 실패·잘린 응답이거나, 원격 작업 트리 검사가 실패하면 그 출력을 믿지 않는다(자리는 셈).
  ls는 hc machine ls를 못 읽으면 목록에 없는 원격 레인을 「모름(연결 확인 못 함)」으로. 시험: 계약 시험 `tests/hcfill/`(가짜 도구는 `tests/hcfill/testdata/`, 일부 절만 `HCFILL_SEC`).
