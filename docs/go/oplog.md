# 운영 기록 (ops.tsv)

2026-11-23 ht-oplog(사용자 「허더 툴스가 지금 계속 작동하면서 로그를 좀 남겨야 할듯해 … 잘못 작동한것도 있을수 있고」).
에이전트·사람이 부른 도구의 멈춤·경고·시간 넘김·0이 아닌 종료는 화면에만 나와 터미널이 닫히면 사라졌다.
이제 같은 글을 기계마다 파일 하나에 한 줄로 남긴다(꾸러미 `internal/oplog`).

## 자리·돌림

- 파일은 `${XDG_STATE_HOME:-~/.local/state}/herdr-tools/ops.tsv`(폴더 0700, 파일 0600)다.
- 환경 `HERDR_TOOLS_OPLOG=<파일>`이면 그 파일에 쓴다(시험용 — `XDG_STATE_HOME`을 바꾸지 않고).
- 시험 바이너리(`go test`가 지은 `<꾸러미>.test`)는 지정이 없으면 시작할 때 `HERDR_TOOLS_OPLOG=/dev/null`을 세운다(`internal/oplog` init).
  환경 값이라 그 시험이 지어 띄운 도구 바이너리도 물려받는다 — 기록을 보려는 시험은 경로를 주고, 빈 값을 주면 기본 자리다.
- 쓰기 전에 4MB를 넘으면 `ops.tsv.1`로 이름을 바꾸고 한 세대만 둔다(남이 먼저 돌려 파일이 없으면 새 파일에 그대로 덧붙인다).

## 칸 (탭 8)

- 시각(UTC `2006-01-02T15:04:05Z`) · 도구 · 하위 명령 · pid · 종류 · 종료 코드(없으면 `-`) · cwd(뒤 64바이트) · 글.
- 하위 명령은 `argv[1]` 하나(32바이트)이고, 없거나 `=`가 든 옵션이면 `-`다(훅은 훅 이름).
- 글은 도구가 표준 오류에 쓴 글에서 「<도구>: 」 머리를 뺀 것이다. 탭·줄바꿈은 빈칸, 줄 전체는 1000바이트 안이다.
- 환경 변수 값·전체 argv는 적지 않는다.
- 쓰기는 한 번의 O_APPEND라 여러 프로세스가 함께 써도 줄이 섞이지 않는다(덧붙이기는 `internal/events.AppendLine`).

## 종류와 길목

| 종류 | 자리 |
|---|---|
| `die` | hc-lane·hc-fill·hc-merger·herdr-sync·herdr-tools-publish·ovis-split·herdr-settings-merge(`die1`·`die2`)·herdr-tools-config(`die2`)의 `die` |
| `refuse` | hc-fill `refuse`(종료 2), `internal/proc` 「모름 — 띄우지 않음」 |
| `timeout` | `internal/proc` 시간 넘김·출력 관 기다림 넘김, hc-merger `dieGit`의 git 시간 넘김(75) |
| `warn` | `internal/proc`의 나머지 알림(못 띄움·고아 처리·자식 기록 실패 등 — 모두 `note` 하나로), hc-lane `errf`, herdr-sync `warn` |
| `exit` | hc-lane·hc-fill `exit()`의 0이 아닌 종료 — 같은 프로세스가 도구의 `die`·`refuse`(`oplog.Fatal`) 줄을 이미 남겼거나 사용법(`usage()`, 종료 2)으로 끝나면 남기지 않는다. proc의 계속 도는 알림(`refuse`·`timeout`·`warn`)은 이 줄을 끄지 않는다 |
| `panic` | herdr-hooks `call`의 recover(훅 이름·종료 코드) |

## 지키는 것

- 기록은 곁에 더할 뿐이다 — 표준 오류 글·종료 코드는 한 글자도 바뀌지 않는다(계약 시험이 본다).
- 못 쓰면(폴더·권한·집 폴더 모름) 도구 동작은 그대로이고, 표준 오류에 「<도구>: 운영 기록을 남기지 못했다(동작은 그대로): …」를 프로세스마다 한 번만 쓴다.
- 훅의 시간 상한과 판정(허용·거절)은 바꾸지 않는다.

## 이상이 아닌데 남는 것

- hc-lane `errf`는 43곳 모두 `warn`으로 남는다 — 「대기열 밖에서 띄움(--direct)」 같은 알림 줄과, `clean`이 실패한 단계의 기록 끝 20줄을 한 줄씩 옮기는 줄도 섞인다. 모으는 쪽은 글로 가른다.

## 안 남는 것

- 사용법(`-h`·`--help`·인자 오류의 사용법 글, 종료 2) — 이상이 아니다.

- launchd로 도는 도구의 표준 오류 전체 — 지킴이 `--log`가 이미 남긴다.
- 잡지 않은 패닉(Go 기본 종료 2와 스택)과 신호로 죽은 경우 — 모든 main의 패닉 잡기는 범위 밖이다.
- `die`를 거치지 않고 `fmt.Fprintf(os.Stderr, …)` 뒤 바로 `os.Exit`하는 자리 — hc-lane·hc-fill은 `exit()`의 `exit` 줄(종료 코드만)로 남고, 그 밖의 도구는 안 남는다.
- 같은 프로세스에서 잠금 안 `die` 뒤 계속 도는 hc-fill watch의 그 뒤 종료 줄(프로세스마다 `exit` 겹침 막기가 한 번이라).
- 기록 모으기(`hc-lane logs`)·지킴이 `--log` 돌림은 따로 레인이다.

시험: `internal/oplog`(꼴·1000바이트·4MB 돌림·쓰기 실패·동시 쓰기·refuse 뒤 exit·시험 자식 상속), `internal/proc` `TestTimeoutGoesToOpLog`, `cmd/herdr-hooks` `TestPanicGoesToOpLog`, `tests/util` `TestOpLog*`(hc-lane die·hc-fill 종료 1·사용법 안 남김).
