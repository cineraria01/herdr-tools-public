# 시험

시험은 모두 Go다(셸 시험 `*.test.sh`는 두지 않는다 — `tests/repo`의 `TestNoShellTests`가 막는다).
계약 시험은 시험 프로세스에서 지은 바이너리(`internal/clitest` `Bin`·`BinDir` — 도구마다 `go build` 한 번)를 가짜 hc·ssh·git(PATH 앞)과 임시 HOME·임시 저장소로 부른다.
실제 기계·레인에 닿지 않는다.

```sh
bash .claude/herdr-verify.sh        # 저장소 검증(hc-lane land가 부른다): go vet ./... → go test ./tests/repo/(싼 검사) → TestCotenantGuard 홀로 → go test -race 조각 여럿 함께, 단계마다 고아 검사
bash .claude/herdr-verify.sh --cheap-only   # 싼 검사까지(.claude/herdr-precheck.sh — 병합기 제출 직전)
go test -race ./...                 # 계약 시험 전부
go test ./tests/repo/               # 싼 검사: 줄 수 예산·싼 검사 파일 빠짐·gofmt·셸 문법(남은 .sh)·셸 시험 없음·개인 값(lint --strict)·비밀 꼴
go test ./tests/hclane/ ./tests/merger/ ./tests/hcfill/ ./tests/util/ ./claude/hooks/tests/ ./cmd/...   # 도구별(hc-lane·병합기·hc-fill·작은 도구·훅·운영 도구)
```

전체 검증(`--cheap-only` 아님)이 통과로 끝나고 시작과 끝에 작업 트리가 깨끗한(추적 파일 변경·추적 안 된 파일 없음) 같은 트리면, 그 `HEAD^{tree}`를 이 작업 트리의 git 폴더 `herdr-verified-tree`에 「<트리> <유닉스 초>」로 남긴다(`tests/repo/verify/mark.go` — 원격으로 안 나감, 읽는 곳은 제출의 [사전 검사 건너뜀](../hc-lane/merger.md) 하나).

## 시험의 기다림 — 새 장면을 쓸 때의 규칙

검증 바닥은 조각 초의 합 ÷ 자리라, 일 없이 기다리는 초가 검증 시간이 된다(2026-11-24 tests-wait-trim: 병합기 상주 시험 122초·hc-fill gc 배경 32초를 걷었다).
- 도구의 기본 대기(시작 확인 `--start-wait`, 쉼 `sleep`, `HC_LANE_*`·`HC_FILL_*`·`HC_MERGER_*` 손잡이)를 끝까지 기다리지 않는다 — 그 대기 동안 시험이 보는 상태(사건·기록·종료 코드)가 안 바뀌면 손잡이로 줄이거나 PATH의 가짜 명령으로 바로 끝낸다.
- 「그쯤이면 됐겠지」 고정 `time.Sleep`·가짜 명령의 고정 잠 대신 조건(파일·사건·놓아줌 표식)이 될 때까지 폴링한다(상한은 넉넉히 — 상한에 걸리면 실패).
- 「안 일어난다」를 보는 창은 제품 주기를 손잡이로 함께 줄일 때만 줄인다(창만 줄이면 거짓 통과).
  시간 넘김을 보는 장면은 상한 손잡이를 줄이되 부하에서 흔들리면 되돌린다.
  「N초 안에 끝남」 같은 시간 단언은 새로 만들거나 조이지 않는다.

## 새 명령을 더할 때

`cmd/<도구>`를 새로 만들면 함께 고칠 곳이다.
싼 단계의 `TestNewCmd`(`tests/repo/newcmd_test.go`, 파일 읽기만)가 도구마다 보고, 빠지면 「<무엇>이 빠졌다 — <어느 파일>에 <어떤 줄>을 더하라」로 실패한다(2026-11-24 newcmd-cheap — 바이너리 줄 확인이 무거운 조각의 herdr-sync 계약 시험에서만 돌아, 새 명령을 더한 레인이 병합기 검증 뒤에야 반송됐다. 그 계약 시험의 확인도 그대로 있다).

- `config.example/sync.list`에 바이너리 줄 「`<묶음>  .local/bin/<도구>  tools:cmd/<도구>  <대상>`」(대상은 도는 곳대로 `all`·`macos`·`local`, [바이너리 줄](../herdr-sync/binary-lines.md)).
- README 도구 표(`## 목차` 앞)에 `` `<도구>` ``가 든 줄.
- 문서 `docs/<도구>/<기능>.md`와 README 목차 한 줄(이름순 자리). 다른 문서가 그 도구를 설명하면 대신 `newcmd_test.go`의 `docsElsewhere`에 그 문서를 적는다(그 문서가 있고 도구 이름이 나와야 한다).
- `--version`은 `version.String("<도구>")`(꼴 「<도구> (Go …)」, `internal/version`). 다른 꼴을 쓰는 도구는 `versionOwn`에 까닭과 함께 적는다.
- 곁 도구(다른 도구가 부르는 것)면 `cmd/<곁>/OWNER`에 부르는 도구 이름, 바이너리 줄은 부르는 도구와 같은 대상(`config.example/sync.list` 머리 주석 — `sync-lint`가 본다).
- 예외 목록(`docsElsewhere`·`versionOwn`)에 있는데 `cmd/`에 없는 도구는 실패한다(도구를 지우면 그 줄도 지운다).

## 검증의 조각 실행

- 2026-11-23 verify-shard: 꾸러미 안 시험은 차례로 돌아, 검증 한 번(병합기 기계 970초)이 거의 `tests/hclane` 하나(927초)의 시간이었고 처음 5분쯤 뒤로는 코어 대부분이 놀았다.
  그래서 무거운 단계를 `go test` 조각 여럿으로 나눠 함께 돌린다.
- 나누는 표는 `tests/repo/verify/main.go`의 `splits`다.
  꾸러미마다 part(`-run` 꼴)를 적으면 part마다 조각 하나(앞 part가 고른 것은 `-skip`)와 그 꾸러미 나머지 조각(`-skip` part 전부)이 생긴다.
  표에 없는 꾸러미는 모아 한 조각이다.
  그래서 시험(하위 시험·절)은 조각 하나에서 한 번만 돌고, 새 시험·새 꾸러미는 나머지 조각에 저절로 든다(시험 파일은 옮기지 않는다).
- `tests/hcfill`의 `TestHcFill`은 `HCFILL_SEC` 절 묶음으로 나눈다.
  마지막 묶음은 소스의 `reg(<차례>, "<이름>", …)`에서 읽은 나머지 절 전부다(글자 그대로가 아닌 reg 호출이 있으면 나누지 않는다).
- `tests/hclane`의 `TestHcLane`은 하위 시험 묶음으로 나눈다.
  앞 절 상태에 기대는 절(`s20c`←`s20b`, `s21a2`~`s21a4`←`s21a1`)은 떼지 않는다 — 새 절이 떼어 낸 절의 상태에 기대면 그 묶음에 넣는다.
  레인을 띄우는 장면은 띄우기의 시작 확인을 보지 않으면 `--start-wait 0`을 준다 — 가짜 hc에서는 상태가 안 잡혀 상한(기본 60초)을 끝까지 기다린다(2026-11-24 s21b1-trim: 가장 긴 조각 `s21b1` 약 290초 → 약 60초).
- 두 계약 시험은 최상위 시험 여럿이다(2026-11-23 contract-split — 시험 하나일 때 가장 긴 조각이라 표로 못 나눴다): 병합기 `TestMergerContractA`~`E`(`tests/merger`), herdr-sync `TestContractA`~`E`(`cmd/herdr-sync`).
  묶음마다 자기 준비를 스스로 하니(뒤 묶음이 기대는 상태는 준비에서 제품 명령을 돌려 만든다) 혼자서도·어떤 차례로도 돈다.
  새 장면은 기대는 상태가 있는 묶음에 넣는다.
- `TestSplitsCoverRepo`가 표를 지킨다: 표의 꾸러미·part가 다 있고, 그 꾸러미의 최상위 시험(나눈 시험은 절마다)이 정확히 조각 하나에서 돈다.
  `TestShardsRealGo`는 진짜 `go test`로 같은 꼴의 조각을 돌려 시험마다 한 번씩 도는지 본다.
- 함께 도는 자리 수는 `HERDR_VERIFY_JOBS=<수>`이고, 없으면 코어 수다.
  조각은 한 자리를, 표에 없는 꾸러미를 모은 나머지 조각은 `go test -p <그 1/3>`로 그만큼 자리를 쓴다(나머지 조각만 따로 `-p` 코어 수로 돌면 시험 바이너리가 코어 수의 두 배쯤 함께 떠 시간을 기다리는 시험이 넘쳤다).
  2026-11-23 실측: 코어 수의 7/10보다 코어 수가 빨랐다(10코어 487초 대 542·544초, 8코어 612초 대 761초).
  함께 돌면 시험마다 1.5~2배 느려지지만(시험이 띄우는 프로세스 쪽에서 막혀 CPU는 2코어쯤만 쓴다) 총시간은 줄었다.
  `TestCotenantGuard`(부하를 재는 시험)는 조각 전에 홀로 돈다.
- 들어가는 조각 먼저: 빈자리가 앞 조각(자리 여럿을 원하는 나머지 조각)에 모자라면 그 뒤에서 빈자리에 들어가는 조각을 먼저 띄운다.
- 검증 잠금: 무거운 단계(`TestCotenantGuard`·조각) 앞에서 기계 단위 잠금(사용자 캐시 폴더 `herdr-tools/verify.lock`, flock)을 얻어 같은 기계의 검증 둘이 겹치지 않게 하고(기다린 초 한 줄, 검증 시간에서 뺀다), 30분 상한을 넘기면 경고 한 줄 뒤 그대로 진행하며, `HERDR_VERIFY_NO_LOCK=1`이면 잠금을 안 본다.
- 조각 출력은 끝날 때 한 덩이로 찍힌다(`== [i/n] …` 띄움, `-- [i/n] … 끝 <초> 종료 <코드>` 다음에 그 출력).
  이름표에는 조각마다 다른 환경(`HCFILL_SEC=…`)만 찍고, 모든 조각에 같은 메모리 디스크 환경(`TMPDIR`·`HERDR_VERIFY_BOOT_TMPDIR`)은 띄울 때만 넘긴다(단계 앞 「메모리 디스크 …」 한 줄이 알린다).
  실패 줄(`--- FAIL:`·`FAIL <꾸러미>`)은 그대로다.
  고아 검사는 조각마다 끝나자마자 그 무리를 보고(늦게 보면 pid가 한 바퀴 돌아 남의 새 무리를 끌 수 있다), 떼어 띄워 부모가 1이 된 것은 조각이 다 끝난 뒤 한 번 본다(함께 도는 다른 조각의 것을 고아로 읽지 않게).
- 메모리 디스크(2026-11-24 verify-ramdisk, `tests/repo/verify/ramtmp.go`): 자리를 늘려도 검증이 안 줄었고(자리 10·20·30에 546·530·527초) 병목은 디스크의 작은 쓰기였다(시험들이 임시 폴더에 git 저장소를 만들고 커밋·push하기를 되풀이).
  그래서 macOS에서는 조각 단계 앞에 메모리 디스크(APFS, 대소문자 구분 없음, `.metadata_never_index`)를 만들어 조각의 `TMPDIR`로 주고, 원래 임시 폴더는 `HERDR_VERIFY_BOOT_TMPDIR`로 넘기고, 끝나면(신호로 끝날 때도) 뗀다.
  `TestCotenantGuard`와 싼 단계는 그대로다(메모리 디스크에서도 같은 시간이었다).
  - 켜짐: macOS이고 `HERDR_VERIFY_RAMTMP`가 없으면(기본 1024MB — 워커 맥 10코어 실측 최대 사용 354MB의 약 3배) 또는 `<MB>`면 켠다. `0`이면 끈다. 크기가 기계 메모리의 1/4을 넘으면 켜지 않는다.
  - 못 만들면(명령 실패·1분 상한 넘김·확인 실패) 알림 한 줄 뒤 보통 임시 폴더로 그대로 검증한다 — 메모리 디스크 때문에 검증이 실패하지 않는다.
  - 끝에 「메모리 디스크 사용량 최대 …MB / …MB」 한 줄(거의 가득 차면 ENOSPC 거짓 실패일 수 있다는 말을 덧붙인다).
  - 지우기 안전: 포맷은 방금 붙인 장치 하나에만, 붙이기 출력이 `/dev/disk<수>` 한 줄이고 `hdiutil info`에서 그 장치가 메모리 디스크 이미지(`ram://…`)·요청한 섹터 수·마운트 없음일 때만 한다. 떼기는 메모리 디스크 이미지이고 우리 볼륨(`herdr-verify-ram-<pid>`)이 마운트돼 있을 때만, 시작할 때는 이름의 pid가 죽은 우리 볼륨만 치운다. 시험은 가짜 `hdiutil`·`diskutil`(`ramtmp_test.go`).
  - 메모리 디스크는 `/Volumes/` 아래라, 부팅 볼륨이 필요한 시험(hc-sweep 색인 장면 — `/Volumes/` 아래 원본은 빠진 디스크로 보고 안 지운다)은 `HERDR_VERIFY_BOOT_TMPDIR` 아래 폴더를 쓴다.
- 되돌리기: `splits`를 비우면 표에 없는 꾸러미 한 조각이 옛 꼴(`-skip ^TestCotenantGuard$ <꾸러미 전부>`)이 된다.
