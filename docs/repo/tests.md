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
  실패 줄(`--- FAIL:`·`FAIL <꾸러미>`)은 그대로다.
  고아 검사는 조각마다 끝나자마자 그 무리를 보고(늦게 보면 pid가 한 바퀴 돌아 남의 새 무리를 끌 수 있다), 떼어 띄워 부모가 1이 된 것은 조각이 다 끝난 뒤 한 번 본다(함께 도는 다른 조각의 것을 고아로 읽지 않게).
- 되돌리기: `splits`를 비우면 표에 없는 꾸러미 한 조각이 옛 꼴(`-skip ^TestCotenantGuard$ <꾸러미 전부>`)이 된다.
