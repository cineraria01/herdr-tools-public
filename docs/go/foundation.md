# Go 바탕

- **Go 바탕**(2026-11-21 go-foundation — herdr-tools 전부 Go로 옮기는 바탕): 모듈 `herdr-tools`(루트 `go.mod`), 도구는 `cmd/<도구>/`, 공통 꾸러미는 `internal/` — `gitx`(git 결과 셋: 있음·없음·모름, 실패한 명령의 반쪽 출력은 버림), `atomic`(임시 → fsync → rename → 폴더 fsync), `proc`(새 프로세스 무리로 띄우기·pid + 시작 시각으로 소유 확인·무리째 거두기, **바깥 명령 상한** `proc.Run`·`Output`·`Combined` — git·hc·hc-lane·ssh·gh·curl은 로컬 git `HERDR_TIMEOUT_GIT`(60초, worktree·fetch·push 등 원격 git은 아래), 원격 git·hc·ssh·gh·curl `HERDR_TIMEOUT_REMOTE`(120초), hc-lane `HERDR_TIMEOUT_LANE`(600초) 안에서만 기다리고 넘으면 무리째 거두고 「시간 넘김 <명령·하위 명령> <초>」 한 줄(인자 값·사건 본문은 안 적음)·종료 124(모름 — Go에서는 `errors.Is(err, proc.ErrTimeout)`, 자식이 끝났는데 출력 관이 안 닫힌 것도).
  0이면 상한 없음. `hc wait`·`hc run`(배경 아님)·`--timeout`을 받은 hc·`hc-lane land|wait|watch`는 기다림이 일이라 상한 없음(옵션은 하위 명령의 옵션 자리만 본다).
  따로 띄운 무리는 그 도구가 TERM·INT·HUP을 받으면 거둔 뒤 같은 신호로 끝나고, SIGKILL로 죽어 남은 무리는 같은 사용자의 다음 실행이 고아 기록(`HERDR_PROC_DIR`, 기본 사용자 캐시 폴더 `herdr-tools/proc`)으로 소유를 확인한 뒤 거둔다.
  `proc.Wait`의 자식 기록은 다 쓴 뒤에야 자식이 실제 명령을 시작한다.
  무리·신호는 유닉스만 — Windows는 빌드만), `lockx`(flock, O_CLOEXEC — 주인이 죽으면 풀림), `events`(hc-lane 사건 줄 12칸 읽고 쓰기, 지금 꼴 그대로).
  깔기는 위 「바이너리 줄」(소스 `~/.local/lib/herdr-go/`는 릴리스를 못 받을 때 그 기계에서 짓는 데만 쓴다).
  시험 `go test -race ./...`.
