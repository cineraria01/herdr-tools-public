# 시험

시험은 모두 Go다(셸 시험 `*.test.sh`는 두지 않는다 — `tests/repo`의 `TestNoShellTests`가 막는다).
계약 시험은 시험 프로세스에서 지은 바이너리(`internal/clitest` `Bin`·`BinDir` — 도구마다 `go build` 한 번)를 가짜 hc·ssh·git(PATH 앞)과 임시 HOME·임시 저장소로 부른다.
실제 기계·레인에 닿지 않는다.

```sh
bash .claude/herdr-verify.sh        # 저장소 검증(hc-lane land가 부른다): go vet ./... → go test ./tests/repo/(싼 검사) → TestCotenantGuard 홀로 → go test -race 조각 여럿 함께, 단계마다 고아 검사
bash .claude/herdr-verify.sh --cheap-only   # 싼 검사까지(.claude/herdr-precheck.sh — 병합기 제출 직전)
go run ./tests/repo/verify --flaky  # 이 기계의 흔들림 기록을 시험별 횟수로(아래 「검증의 흔들림」)
go test -race ./...                 # 계약 시험 전부
go test ./tests/repo/               # 싼 검사: 줄 수 예산·싼 검사 파일 빠짐·gofmt·셸 문법(남은 .sh)·셸 시험 없음·개인 값(lint --strict)·비밀 꼴
go test ./tests/hclane/ ./tests/merger/ ./tests/hcfill/ ./tests/util/ ./claude/hooks/tests/ ./cmd/...   # 도구별(hc-lane·병합기·hc-fill·작은 도구·훅·운영 도구)
```

싼 검사 파일 빠짐(`TestCheapKept`)은 이 가지가 기본 가지에서 갈라진 자리(merge-base)에 있던 `tests/repo` 파일을 본다 — 갈라진 뒤 다른 레인이 더한 파일은 받지 않은 것이라 막지 않고, 병합기는 rebase한 트리라 기본 가지 끝과 같다(갈라진 자리를 못 구하거나 얕은 클론이면 기본 가지 끝, 2026-11-24 cheapkept-base).

전체 검증(`--cheap-only` 아님)이 통과로 끝나고 시작과 끝에 작업 트리가 깨끗한(추적 파일 변경·추적 안 된 파일 없음) 같은 트리면, 그 `HEAD^{tree}`를 이 작업 트리의 git 폴더 `herdr-verified-tree`에 「<트리> <유닉스 초>」로 남긴다(`tests/repo/verify/mark.go` — 원격으로 안 나감, 읽는 곳은 제출의 [사전 검사 건너뜀](../hc-lane/merger.md) 하나).

## 시험의 환경 — 바깥 변수를 읽어야 할 때

시험이 부르는 도구의 환경은 허용 목록에서 시작한다(2026-11-24 test-env-allowlist, `internal/clitest` `Env`·`Isolate`).
전에는 바깥 환경을 통째로 받고 지우는 목록 여섯 벌(공용 도우미·util·hclane 틀 둘·merger·hcfill, 18~39개)으로 걸렀는데 목록마다 빠진 것이 달라, 레인 터미널·병합기 안에서 돌면 `LANE_*`·`HC_SCOPE`·`HERDR_TOOLS_CONFIG`·`HERDR_MERGER`·`HC_FILL_DATA` 같은 값이 새어 들었다.
- `clitest.EnvAllow`만 바깥에서 받는다: 실행 경로 `PATH`, 임시 폴더 `TMPDIR`·`HERDR_VERIFY_BOOT_TMPDIR`(검증 실행기의 메모리 디스크와 부팅 볼륨 — 그대로 넘어간다), `USER`·`LOGNAME`·`SHELL`·`TERM`, 언어 `LANG`·`LC_ALL`·`LC_CTYPE`, `TZ`, 그리고 시험 바이너리가 스스로 넣는 일지·운영 기록 `HERDR_TOOLS_JOURNAL`·`HERDR_TOOLS_OPLOG`(`internal/journal`·`internal/oplog`의 init이 `/dev/null`로 — 시험이 띄운 도구가 실제 일지에 쌓지 않게).
- `clitest.Env(T, keep…)`는 시험 틀의 자식 환경 바탕이다 — 허용 목록·keep의 바깥 값, `HOME=T`(임시), `GIT_CEILING_DIRECTORIES`(T 위 저장소를 못 찾게). 틀은 그 위에 자기 값(가짜 명령 PATH·git 이름·설정 폴더)을 덧씌운다.
- `clitest.Isolate(t, T, keep…)`는 시험 프로세스 자신의 환경을 허용 목록으로 줄인다(`os.Environ()`을 물려주는 `clitest.Cmd`·`Git`의 자식도 같아진다). 이 프로세스가 `go build`·`go test`를 부르니 `HOME`·`PWD`(go test가 꾸러미 폴더로 넣는다)와 go 도구 값(`GO*`·`CGO_*`)은 남긴다.
- 시험이 바깥 값을 꼭 읽어야 하면(시험 전용 덮어쓰기 `HC_FILL_EXAMPLE`·`HCFILL_SEC` 같은 것) 시험 프로세스에서 `os.Getenv`로 읽어 자식에 값을 넘기고(`Isolate`보다 먼저), 자식이 바깥 이름 그대로 받아야 하면 `Env`·`Isolate`의 keep에 이름을 적는다 — 허용 목록 자체는 늘리지 않는다.
- `clitest.Poison`·`Leaked`: 레인 터미널·병합기 값(`clitest.LaneEnv`, 아직 없는 손잡이 자리 하나 포함)을 넣고 틀의 자식 환경에 남은 것을 본다. 틀마다 `TestKitEnvNoLeak`(`tests/{util,hclane,merger,hcfill}/envleak_test.go`)가 지킨다 — 새 시험 틀은 `clitest.Env`에서 시작하고 같은 시험을 하나 둔다.

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
- 조각 출력은 끝날 때 한 덩이로 찍힌다(`== [i/n] …` 띄움, `-- [i/n] … 끝 <초> 종료 <코드> · +<시작>~+<끝>초 자리 <수>` 다음에 그 출력).
  싼 단계·홀로 단계도 `-- … 끝 <초> 종료 <코드> · +<시작>~+<끝>초` 한 줄이고(검증 시작부터의 초, 잠금 기다림은 뺀다), 조각 단계 끝에 「조각 단계 <초> · … — 자리 n, 쥔 자리·초, 빈 자리·초 (%)」 한 줄이다.
  2026-11-24 verify-pack-measure(워커 맥 10코어): 빈 몫 8~9% → 4~5% — 나머지 조각(자리 3) 안의 `tests/repo/verify`(150초)가 늦게 떠 그 조각이 단계를 묶어 따로 떼고, est를 실측으로(hc-fill 절 묶음마다 `secEst`). 조각 단계 274~285 → 251~257초.
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

## 검증의 범위 — 입력이 통과 기록과 같은 조각은 건너뛴다

2026-11-24 verify-scope(사용자 「지금 herdr-verify.sh 검증 돌리는거 좀 너무 과한건 아니야?」): 무엇을 고쳤든 조각 38개를 전부 돌았다(주석·문서만 고친 레인도).
그래서 조각(`TestCotenantGuard` 홀로 단계 포함)마다 「그 결과가 기댈 수 있는 것」의 내용 지문을 내고(`tests/repo/verify/scope.go`), 통과한 지문을 이 기계의 사용자 캐시 `herdr-tools/verify-pass/<지문>`(「초<탭>이름표」 한 줄)에 남긴다.
싼 단계(`go vet`·`tests/repo`)는 늘 돈다.

- 지문의 입력(넓게 잡는다 — 넓어 덜 건너뛰면 느릴 뿐이고, 좁아 잘못 건너뛰면 깨진 것이 main에 든다):
  - (가) 조각이 도는 꾸러미와 그 시험 짓기의 모듈 안 의존 전부(`go list -test`의 `<꾸러미>.test` Deps). 조각이 도는 꾸러미는 폴더의 추적 파일 전부(시험 파일·testdata·embed), 가져오는 꾸러미는 짓는 파일만(`_test.go`·`testdata/` 빼고).
  - (나) 시험이 지어 돌리는 도구와 그 도구가 부르는 도구: 꾸러미의 Go 글자 상수(주석은 아님)·Go 아닌 파일에 폴더 경로(`cmd/hc-lane`·`internal/proc`)가 나오거나, `cmd/<이름>`의 `<이름>`이 명령 꼴 낱말(앞뒤가 빈칸·`/`·따옴표·셸 기호·끝 — `exec.Command("hc-lane")`, `"bin/hc-lane land"`)로 나오면 그 꾸러미와 그 짓기 의존을 더하고, 그 짓는 파일에서 또 찾는다(hc-fill → hc-lane → hc-merger …). 손으로 적은 표는 없다.
  - (다) 모든 조각의 공통 입력: 이 판(`scopeVer`)·Go 판·OS·아키텍처·go 환경(`GOFLAGS` 등), 어느 꾸러미 폴더에도 안 든 추적 파일 전부(`go.mod`·`go.sum`·`.claude/`·`config.example/` 전체·`launchd/`·`tests/line-budget.tsv` …, 문서 빼고), `cmd/*/OWNER`, `cmd/` 아래 폴더 이름 목록(디스크 그대로 — 세션 훅이 남기는 점 폴더 `cmd/.omc` 등은 뺀다), 검증 본문(`tests/repo/verify`의 시험 아닌 소스), 시험이 읽는 문서, 조각의 argv·env.
  - 문서(`docs/` 아래, 꾸러미 폴더 밖의 `*.md`)는 입력이 아니다 — `config.example/`·`.claude/` 아래는 `.md`여도 공통 입력이다(설정 틀·검증 규칙). 그래서 문서만 고친 변경은 조각을 전부 건너뛴다(따로 규칙을 두지 않는다).
    단 Go 글자 상수(`tests/repo` 밖)에 그 경로(`docs/…`)가 나오거나, 뿌리의 `*.md` 이름이 저장소 뿌리를 읽는 시험 파일(clitest `Repo(t)`·`Root(t)`, `"../.."`)의 글자 상수에 나오면 공통 입력이다.
  - 실례(2026-11-24): 새 명령 폴더 `cmd/hc-job-server`를 더했더니 `cmd/herdr-sync` 계약 시험이 「`config.example/sync.list`에 바이너리 줄이 있다」에서 실패했다 — 그 시험의 입력은 herdr-sync 소스가 아니라 `cmd/` 폴더 이름 목록과 `config.example`이다. 둘 다 공통 입력이라 그 커밋에서 이 조각은 건너뛰지 않는다.
- **새 시험이 저장소의 다른 곳을 읽으면 그 경로가 입력에 드는지 확인한다.** 꾸러미 폴더 밖·문서·다른 도구를 읽거나 부르면 위 (가)~(다) 어디에 드는지 보고, 안 들면 글자 상수로 경로를 적거나 `scope.go`의 공통 입력을 넓힌다(`scopeVer`를 올려 옛 기록이 안 맞게).
  다른 꾸러미 폴더의 파일을 글롭으로 읽는 것(`internal/toolcfg`의 `cmd/*/OWNER`)은 그 파일을 공통 입력으로 넣는다(`scope.go`의 OWNER 줄처럼).
- 기록은 작업 트리가 깨끗할 때만(추적 파일 변경 없음 + 문서 밖 추적 안 된 파일 없음) 읽고 쓴다 — 아니면 전부 돌고 기록을 안 쓴다(한 줄 알림).
  통과 기록은 검증 끝에 작업 트리·`HEAD^{tree}`가 처음 그대로일 때만 쓴다. 원격으로 나누지 않는다(이 기계의 같은 내용이면 작업 트리가 달라도 같은 지문).
  30일 넘은 기록과 3000개를 넘는 오래된 기록은 검증 끝에 지운다.
- 모드(`HERDR_VERIFY_SCOPE`):
  - 없음·`observe`(기본): 지문을 내고 기록을 쓰되 전부 돈다. 기록과 같은 조각은 띄우는 줄에 「건너뛸 수 있었음」을 덧붙인다. 단 하나의 예외로, 모든 조각의 지문이 기록과 같으면(문서만 고친 변경 등) 전부 건너뛴다.
  - `on`: 기록과 같은 조각은 띄우지 않고 「건너뜀 [i/n] … — 입력이 통과 기록과 같음」 한 줄.
  - `off`: 지금처럼 전부 돌고 지문도 안 낸다.
  - `HERDR_VERIFY_ALL=1`이면 어떤 모드든 전부 돈다(기록은 쓴다).
  - 끝에 요약 「범위(<모드>): 조각 n개 중 k개 건너뛸 수 있었음(약 s초 — 기록에 남은 초의 합), 건너뜀 j개」. 아무 조각도 안 돌면 검증 잠금·메모리 디스크도 건너뛴다.
- 지문을 못 내면(`go list`·git 실패) 한 줄 알림(「알림 범위 지문 실패(…)」) 뒤 전부 돈다 — 지문 때문에 검증이 실패하지 않는다. 조각 하나의 꾸러미를 모르면 그 조각만 늘 돈다.
- 놓친 입력: `observe`에서 건너뛸 수 있었던 조각이 실패하면 지문이 입력을 놓쳤다는 증거다 — 「놓친 입력: [i/n] … — 건너뛸 조각이 실패」 한 줄과 함께 사용자 캐시 `herdr-tools/verify-scope-misses.log`에 「시각<탭>HEAD<탭>지문<탭>이름표」 한 줄을 남긴다.
  모아 보기: `cat ~/Library/Caches/herdr-tools/verify-scope-misses.log`(리눅스는 `~/.cache/herdr-tools/…`). 줄이 쌓이면 그 HEAD에서 그 조각이 무엇을 읽었는지 찾아 입력에 넣는다.
  (지문에 안 드는 바깥 상태 — 시간·부하·네트워크 — 로 흔들리는 시험도 여기에 남는다.)
- `TestCotenantGuard` 단계도 지문으로 건너뛴다: 부하를 재는 시험이지만 가짜 HOME으로 돌고 그 결과가 기대는 코드는 지문에 다 든다 — 같은 입력으로 통과한 기록이 있으면 다시 재지 않는다.
- 시험: `TestScopeKeys`(입력 규칙 — 꾸러미 한 줄·도구·도구가 부르는 도구·주석·문장 속 이름·testdata·문서·config.example·`.claude/`·`cmd/` 새 폴더·뿌리 문서·argv·env), `TestScopeDirty`, `TestScopePass`, `TestScopeRun`(진짜 go로 ①~⑫ 실행), `TestScopeRepo`(이 저장소의 조각 모두 지문 — 시간은 단언하지 않고 기록만).
- 실측(2026-11-24, 워커 맥 10코어): 이 저장소 지문 0.3초(조각 38개). 큰 조각은 꾸러미 43개 중 20~29개에 기댄다 — hc-lane이 허브라(hc-fill·병합기가 부르고 큰 계약 시험 넷이 돌린다) hc-lane·공용 `internal`을 고치면 거의 다 돈다.

## 검증의 흔들림 — 실패한 것을 혼자 한 번 다시

2026-11-24 flaky-retry: 병합기 반송의 검증 실패(종료 1) 가운데 여럿이 진짜 문제가 아니라 부하에 흔들린 시험이었다(홀로 도는 `TestCotenantGuard`의 시간 단언, 계약 시험의 띄우기 시간 넘김, `-race` 흔들림 — 한 레인은 흔들림만으로 두 번 반송돼 1시간 반 늦었다).
- 홀로 단계(`TestCotenantGuard`)나 조각이 실패하면, 다른 조각이 모두 끝난 뒤 실패한 것을 하나씩 혼자 한 번 다시 돌린다(`tests/repo/verify/retry.go`).
- 다시 통과하면 전체는 통과이고 「흔들림: [i/n] <조각> — <실패했던 시험> · 혼자 다시 돌려 통과」 한 줄을 찍는다.
- 다시 실패하면 지금처럼 실패이고 「두 번 실패: …」 줄을 찍는다. 통과할 때까지 되풀이하지 않는다.
- 다시 안 도는 것(「다시 안 돎: … — <까닭>」 한 줄): 출력에 `WARNING: DATA RACE`(진짜 경합) · `[build failed]` · `[setup failed]` · go 시간 넘김(`panic: test timed out`) · 실패한 시험 이름이 없음 · 고아를 남김 · 싼 단계 실패 등 다시 돌 수 없는 실패가 함께 있음.
  하나라도 다시 돌 수 없으면 아무것도 다시 안 돈다(다시 돌려도 결과가 못 바뀐다).
- 실패한 것이 상한을 넘으면 흔들림이 아니라 깨진 것으로 보고 다시 안 돈다.
- 손잡이 `HERDR_VERIFY_RETRY`: 없으면 상한 3, `0`이면 끔(지금까지처럼 실패), `<수>`면 다시 돌 수 있는 실패 수의 상한.
- 실패 없는 검증은 시간이 늘지 않는다(끝에 기록 파일 하나를 읽을 뿐).
- 흔들려 통과한 조각은 통과 기록(위 「검증의 범위」)을 남기지 않는다 — 다음 검증에서 다시 돈다.
- 흔들림 기록: 사용자 캐시 `herdr-tools/verify-flaky.log`(통과 기록 곁)에 「시각<탭>트리<탭>조각<탭>시험(쉼표)<탭>그때 부하(1·5·15분 평균 / 코어 수)」 한 줄.
- 검증 끝 요약에 「이 기계에서 최근 7일 흔들림: <시험> k번, …」 한 줄(기록이 없으면 안 찍는다). `go run ./tests/repo/verify --flaky`는 기록 전부를 시험별 횟수·마지막 시각으로 보인다.
- **흔들림 기록이 쌓인 시험은 고친다.** 다시 돌기는 진짜 간헐 버그를 가릴 수 있다 — 그래서 줄·기록·요약에 남긴다. 시간 단언을 빼거나 기다림을 조건 폴링으로 바꾼다(위 「시험의 기다림」).
- 시험: `TestRetryPhase`·`TestNoRetry`·`TestFailedTests`·`TestFlakyLog`(판정·기록), `TestRetry*`·`TestVerifyFlakyArg`(가짜 go로 verify 전체 — 흔들림 통과·두 번 실패·경합·상한·끔·혼자 다시·홀로 단계), `TestRetryFlakyNoPassRecord`(진짜 go — 흔들려 통과한 조각은 통과 기록 없음).

## 시험 손잡이 — 배포 바이너리에는 없다

2026-11-24 test-knobs-buildtag: 상주하는 hc-fill이 시험 전용 환경 변수 하나로 셸 명령을 돌리거나(`HC_FILL_TEST_QSET_CMD`) 스스로 죽는(`HC_FILL_CRASH_AT`) 길이 배포 바이너리에 있었다.
그래서 시험만 쓰는 환경 변수(손잡이)를 읽는 코드는 빌드 태그 파일에만 두고, 시험 빌드만 넣는다.
운영에서도 쓰는 조정(기다림·간격·경로·끄기 스위치 — 문서에 적힌 `HC_*`·`HERDR_*`)은 손잡이가 아니다 — 그대로 둔다.

- 새 시험 손잡이는 `cmd/<도구>/testknobs_herdrtest.go`(첫 줄 `//go:build herdrtest`)에서만 환경 변수를 읽는다. 태그 없는 파일(`testknobs.go`)에는 아무것도 안 하는 기본(빈 값을 주는 함수 변수)을 두고, 태그 파일의 `init`이 그 변수를 채운다.
- 계약 시험이 부르는 바이너리(`clitest.Bin`)는 `-tags herdrtest`(`clitest.TestTag`)로 짓는다. 배포 빌드(herdr-sync — 기계마다 짓는 줄과 릴리스 자산)는 태그 없이 짓는다.
- 꾸러미 단위 시험(`go test ./cmd/<도구>`)은 태그 없이 도니, 손잡이가 필요하면 그 꾸러미의 `_test.go` `init`에서 같은 변수를 채운다(hc-fill `testknobs_unit_test.go`).
- 이름은 `…_TEST_…` 또는 `…_CRASH_AT` 꼴로 짓고 `internal/clitest/testknobs_test.go`의 `testKnobs` 목록에 더한다. 그 시험이 본다: 제품 파일(`_test.go` 아님)에서 그 꼴 이름은 태그 파일에만 있고 목록에 있다(`TestTestKnobsListed`), 태그 없이 지은 바이너리에 목록의 이름 글자가 없고 태그를 주면 있다(`TestTestKnobsNotInPlainBuild`), 배포 빌드 줄에 태그가 없다(`TestDeployBuildNoTags`). 태그 없는 hc-fill에 손잡이를 줘도 아무 일 없는 장면은 `tests/hcfill` `TestTestKnobsOffInPlainBuild`.
- herdr-hooks는 먼저 같은 꼴로 갈라 두었고 태그가 `hookstest`다(`claude/hooks/tests`가 그 태그로 짓는다).
