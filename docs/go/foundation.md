# Go 바탕

- **Go 바탕**(2026-11-21 go-foundation — herdr-tools 전부 Go로 옮기는 바탕): 모듈 `herdr-tools`(루트 `go.mod`), 도구는 `cmd/<도구>/`, 공통 꾸러미는 `internal/` — `gitx`(git 결과 셋: 있음·없음·모름, 실패한 명령의 반쪽 출력은 버림), `atomic`(임시 → fsync → rename → 폴더 fsync), `proc`(새 프로세스 무리로 띄우기·pid + 시작 시각으로 소유 확인·무리째 거두기, **바깥 명령 상한** `proc.Run`·`Output`·`Combined` — git·hc·hc-lane·ssh·gh·curl은 로컬 git `HERDR_TIMEOUT_GIT`(60초, worktree·fetch·push 등 원격 git은 아래), 원격 git·hc·ssh·gh·curl `HERDR_TIMEOUT_REMOTE`(120초), hc-lane `HERDR_TIMEOUT_LANE`(600초) 안에서만 기다리고 넘으면 무리째 거두고 「시간 넘김 <명령·하위 명령> <초>」 한 줄(인자 값·사건 본문은 안 적음)·종료 124(모름 — Go에서는 `errors.Is(err, proc.ErrTimeout)`, 자식이 끝났는데 출력 관이 안 닫힌 것도).
  0이면 상한 없음. `hc wait`·`hc run`(배경 아님)·`--timeout`을 받은 hc·`hc-lane land|wait|watch`는 기다림이 일이라 상한 없음(옵션은 하위 명령의 옵션 자리만 본다).
  따로 띄운 무리는 그 도구가 TERM·INT·HUP을 받으면 거둔 뒤 같은 신호로 끝나고, SIGKILL로 죽어 남은 무리는 같은 사용자의 다음 실행이 고아 기록(`HERDR_PROC_DIR`, 기본 사용자 캐시 폴더 `herdr-tools/proc`)으로 소유를 확인한 뒤 거둔다.
  `proc.Wait`의 자식 기록은 다 쓴 뒤에야 자식이 실제 명령을 시작한다.
  `proc.Wait`·`WaitMax`도 Run·Start처럼 고아 기록에 오른다(record가 비어도) — 기록을 못 쓰거나 준비를 못 마치면 띄우지 않고 124(모름 — 명령은 안 돎, 2026-11-23 gofix4-proc 재감사 3바퀴 merger/R-3M02: 옛 판 127은 「명령 실패」로 읽혔다). 실행 파일이 없으면 127. `proc.WaitFor`는 시작했는지(`ran`)도 돌려준다.
  상한 없는 `Wait`(병합기 바퀴 일꾼·게시기)와 `WaitFor(…, keep)`(병합기 착지 — 상한은 살아 있는 부모가 건다)는 기록에 keep 표식을 달아, 부모가 SIGKILL로 죽어도 그 일은 끝까지 가고 다음 실행은 거두지 않고 그것이 도는 동안 센다(75로 미룸) — 상한 있는 `WaitMax`는 Run처럼 소유를 확인하고 거둔다.
  기록 칸: 무리 · 우두머리 시작 시각 · 부모 pid · 부모 시작 시각 · keep|- · 도구(`proc.Who`). 기록 폴더는 도구가 함께 쓴다 — `proc.Orphans()`·`proc.Left()`(무리 번호·기록 경로 글)는 그 도구가 남긴 기록과 도구 칸 없는 옛 꼴 기록만 센다(gofix4 fill/R-3M02).
  `proc.Orphans()`는 우두머리 생존과 상관없이 무리가 남은 기록, 못 읽은 기록 폴더·기록, 꼴이 틀린 기록을 모두 센다(모름 = 미룸) — 신호는 우두머리가 살아 있고 시작 시각이 기록과 같을 때만.
  우두머리 번호를 시작 시각이 다른 프로세스가 쓰면(번호 재사용) keep 기록도 지우고, 부팅(pid 1 시작) 1분 넘게 전에 쓰인 기록은 지운다(gofix4 base/R-3M03·ops/R-3M01·merger/R-3M01).
  상한 걸린 실행의 마감은 첫 준비(신호 처리기·자기 시작 시각·고아 확인)까지 덮고, 준비는 성공할 때까지 다음 호출이 다시 한다.
  결과(종료 코드·출력)는 자식 종료와 출력 끝이 둘 다 마감 안(그리고 종료 뒤 `Grace` 안)일 때만 채택한다 — 쓰개 오류는 성공이 아니고, 쓰개가 막혀도 마감 + 회수 유예 안에 돌아오며 돌아온 뒤에는 새로 쓰지 않는다.
  상한을 직접 주는 `proc.RunMax`·`OutputMax`·`CombinedMax`, ctx가 끝나면 마감과 같게 거두는 `proc.RunCtx`(2026-11-22 gofix3-proc, 재감사 2바퀴 P1~P5).
  무리·신호는 유닉스만 — Windows는 빌드만), `lockx`(flock, O_CLOEXEC — 주인이 죽으면 풀림), `events`(hc-lane 사건 줄 12칸 읽고 쓰기, 지금 꼴 그대로).
  깔기는 위 「바이너리 줄」(소스 `~/.local/lib/herdr-go/`는 릴리스를 못 받을 때 그 기계에서 짓는 데만 쓴다).
  시험 `go test -race ./...`.
- **셸 조각 끝 표식**(2026-11-22 gofix3-shx): Go가 `sh -c`·`hc exec`로 보내는 조각은 `internal/shx`로 감싸고, 출력은 `shx.Cut`이 ok일 때만 쓴다(rc 0이고 마지막 줄이 정확히 표식 — 잘림·중간 종료·rc≠0은 모름).
  `shx.Script`는 엄격 머리(`set -u`·`set -o pipefail`, 못 켜는 셸이면 97)를 붙이고 macOS에서만 도는 조각(hc-lane·hc-fill)에 쓴다.
  `shx.Tail`은 표식만 붙인다 — 리눅스 기계의 sh(dash 0.5.12)는 pipefail을 받지 않으니 herdr-sync 조회처럼 리눅스에서도 도는 조각은 결과를 바꾸는 단계를 파이프에 두지 않고 `|| exit N`으로 본다.
- **`hc exec` 결과 판독**(2026-11-23 hc-exec-result — 「git이 답한 종료 1(없음)」과 「hc가 실패한 종료 1(모름)」을 못 갈라 모름을 없음으로 읽었다): 종료 코드에 뜻을 두는 자리는 `hc exec --json`으로 부르고 `hcjson.ParseExec(표준 출력, 표준 오류, hc 종료)`로 읽는다.
  탐침(hc 0.1.21·0.1.22 두 판, 로컬·`--on` 같은 꼴 — 두 판의 견본이 `internal/hcjson/exec_test.go`에): 결과는 hc 종료 0 + 표준 출력에 `{code, signal, stdout(base64), stderr(base64), timed_out, truncated, ms}` 하나(0이 아닌 종료·시간 넘김·신호도 결과 — 시간 넘김·신호는 `code` null), hc 자신의 실패는 hc 종료 ≠0(띄우지 못함 1, 모르는 기계 4) + 표준 출력 빔 + 표준 오류에 `{"error":{kind,message,hint}}`.
  `--json` 없이는 hc 실패도 종료 1이고 표준 오류의 오류 객체는 프로그램이 쓴 글과 섞여 가를 수 없다. 판독은 엄격하다 — 꼴이 다르면 모름(`ExecError`가 아닌 오류), hc 실패는 `*hcjson.ExecError`(kind) — 둘 다 그 바퀴에 아무것도 바꾸지 않는다.
  옮긴 자리: hc-lane의 `xrun`·`xout`·`xoutQ`(`--on`·코어 문맥일 때 — clean·gc·띄우기의 있음·없음 조회가 다 이 길).
  있음·없음을 읽는 자리는 0 있음·1 없음만 믿고 그 밖은 멈추거나 남긴다(띄우기의 가지·폴더 「이미 있나」도 — 감사 지적으로 `== 0`만 보던 것을 갈랐다).
  결과면 프로그램의 출력과 종료 코드(시간 넘김 124, 신호 128+신호 — 예전 값 그대로), hc 실패·꼴 다름·잘린 출력이면 125(모름).
  그대로 둔 자리(종료 0이 아니면 이미 모름이거나 표준 오류의 오류 객체로 가르는 자리): hc-lane `zx`·`xi`·`xRun`·`rexec`, hc-fill `onm`, hc-sweep `hcExec`, herdr-sync·omc-update·worker-mac-*.
