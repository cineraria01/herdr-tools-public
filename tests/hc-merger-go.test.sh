#!/bin/bash
# Go 판 단위 시험(go.mod 모듈 전체 — cmd/hc-merger·internal/*): gofmt·go vet·go test -race(프로세스 무리 정리·잠금 안 물림·git 모름 → 쉼·
# 우선순위·복구 단계별 죽기·사건 줄 꼴·원자 쓰기). 공통 실행기 lib/go-run.sh가 바이너리를 짓고 고르는지도 본다.
# 운영 도구 계약 시험(cmd/<도구>/contract*_test.go — 옛 tests/<도구>.test.sh)도 여기서 Go 판(기본)으로 돈다. 셸 판은 tests/ops-go-bash.test.sh
# 훅 계약 시험(claude/hooks/tests/*_test.go — 옛 claude/hooks/tests/*.test.sh)도 여기서 돈다 — 판 스위치가 없으면 go·bash 두 판을 다(2026-11-21 go-tests-hooks)
# go가 없으면 실패한다 — Go 판을 쓰는 기계에는 go가 있어야 한다(없으면 go_run이 경고하고 셸 판으로 돈다)
# 실행: bash tests/hc-merger-go.test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
src=$here/..
[ -f "$src/go.mod" ] || src=$here/../../lib/herdr-go   # 깔린 곳(~/.local/bin/tests → ~/.local/lib/herdr-go)
gob=$(command -v go 2>/dev/null) || gob=
for a in /opt/homebrew/bin/go /usr/local/go/bin/go /usr/local/bin/go; do [ -n "$gob" ] || [ ! -x "$a" ] || gob=$a; done
[ -n "$gob" ] || { echo "FAIL - go가 없다(brew install go)"; exit 1; }
cd "$src" || { echo "FAIL - 소스 없음: $src"; exit 1; }
export GOTOOLCHAIN=local GOCACHE=${GOCACHE:-$PWD/.bin/go-build}
fail=0
gofmt=${gob%/go}/gofmt; [ -x "$gofmt" ] || gofmt=gofmt
f=$("$gofmt" -l cmd internal)
if [ -z "$f" ]; then echo "ok - gofmt"; else echo "FAIL - gofmt: $f"; fail=1; fi
if "$gob" vet ./...; then echo "ok - go vet"; else echo "FAIL - go vet"; fail=1; fi
# 시간 한도 600초: tests/hcfill(hc-fill 계약 시험, 바깥 exec 수백 번)이 혼자 2분 남짓 — 바쁜 기계에서도 끊기지 않게
# tests/hclane(hc-lane 계약 시험, 몇 분)은 빼고 tests/hc-lane-go.test.sh·hc-lane-bash.test.sh가 따로 돈다
if "$gob" test -race -count=1 -timeout 600s $("$gob" list ./... | grep -v '/tests/hclane$'); then echo "ok - go test -race"; else echo "FAIL - go test -race"; fail=1; fi

# 공통 실행기(lib/go-run.sh — 저장소에서만. 깔린 곳은 ~/.local/lib/go-run.sh가 같은 파일). 가짜 도구 모듈로 본다
gr=$here/../lib/go-run.sh
if [ -f "$gr" ]; then
  T=$(mktemp -d "${TMPDIR:-/tmp}/go-run.XXXXXX") || exit 1
  mkdir -p "$T/lib" "$T/cmd/hc-fake"
  cp "$gr" "$T/lib/go-run.sh"
  printf 'module fake\n\ngo 1.24\n' >"$T/go.mod"
  printf 'package main\n\nimport (\n\t"fmt"\n\t"os"\n)\n\nfunc main() { fmt.Println("go", len(os.Args)-1, os.Getenv("FAKE_ENV")) }\n' >"$T/cmd/hc-fake/main.go"
  mkdir -p "$T/home"
  gr_() { (unset HC_FAKE_IMPL HC_FAKE_BIN HERDR_IMPL GOPATH GOCACHE XDG_STATE_HOME; export HOME=$T/home; for kv in "$@"; do export "$kv"; done
    . "$T/lib/go-run.sh"; FAKE_ENV=e go_run hc-fake a b; echo "돌아옴 $?") 2>"$T/err"; }
  t() { if [ "$2" = "$3" ]; then echo "ok - go-run: $1"; else echo "FAIL - go-run: $1 — 바람 「$3」, 얻음 「$2」"; cat "$T/err"; fail=1; fi; }
  t "처음엔 빌드해 exec(인자·함수 앞 환경 그대로)" "$(gr_)" "go 2 e"
  t "빌드가 HOME에 go/(GOPATH 기본값)를 만들지 않음" "$([ -e "$T/home/go" ] && echo 있음 || echo 없음)" 없음
  t "바이너리는 모듈 곁 .bin" "$([ -x "$T/.bin/hc-fake" ] && echo 있음)" 있음
  t "스위치 bash면 0으로 돌아옴" "$(gr_ HC_FAKE_IMPL=bash)" "돌아옴 0"
  t "HERDR_IMPL=bash도" "$(gr_ HERDR_IMPL=bash)" "돌아옴 0"
  t "틀린 스위치는 2" "$(gr_ HC_FAKE_IMPL=rust)" "돌아옴 2"
  t "_BIN이 깨졌으면 auto는 경고하고 0" "$(gr_ HC_FAKE_BIN=/없는/바이너리)" "돌아옴 0"
  t "그 경고" "$(grep -c '경고: Go 판을 쓸 수 없다' "$T/err")" 1
  t "_BIN이 깨졌고 IMPL=go면 1" "$(gr_ HC_FAKE_BIN=/없는/바이너리 HC_FAKE_IMPL=go)" "돌아옴 1"
  # 리눅스·go 없음(받기 대상이 아님, 셸 판이 정상 경로): 크게 경고하지 않고 처음 한 번만 한 줄(go-followups)
  nogo=(HC_FAKE_BIN=/없는/바이너리 PATH=/없음 GO_RUN_GOS= GO_RUN_OS=linux-gnu)
  t "리눅스·go 없음: 0" "$(gr_ "${nogo[@]}")" "돌아옴 0"
  t "리눅스·go 없음: 크게 경고하지 않고 한 줄 안내" "$(grep -c '경고' "$T/err") $(grep -c 'go가 없어 셸 판' "$T/err") $(wc -l <"$T/err" | tr -d ' ')" "0 1 1"
  t "리눅스·go 없음: 두 번째부터 조용히" "$(gr_ "${nogo[@]}") $(wc -l <"$T/err" | tr -d ' ')" "돌아옴 0 0"
  gr_ HC_FAKE_BIN=/없는/바이너리 PATH=/없음 GO_RUN_GOS= GO_RUN_OS=darwin27 >/dev/null
  t "macOS·go 없음은 그대로 크게" "$(grep -c '경고: Go 판을 쓸 수 없다' "$T/err")" 1
  # 판 대조(2026-11-21 gofix-core B17·L1·B18): 기계 빌드도 빌드 시작 때 계산한 판을 .src에 적고, 시각 말고 늘 판으로 본다.
  # 판이 다르거나 모르면(.src 없음) 새 빌드가 된 뒤에만 Go 판 — 빌드가 안 되면 옛 바이너리를 쓰지 않는다
  M=$T/cmd/hc-fake
  src_() { (cd "$T" && bash -c '. lib/go-run.sh && go_run_src'); }
  t "기계 빌드도 지은 판을 .src에" "$(cat "$T/.bin/hc-fake.src" 2>/dev/null)" "$(src_)"
  printf 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("새 판") }\n' >"$M/main.go"
  t "소스가 바뀌면 다시 빌드" "$(gr_)" "새 판"
  printf 'package main\n\nfunc main() { 깨짐 }\n' >"$M/main.go"; touch "$M/main.go"
  t "빌드 실패면 판이 다른 옛 바이너리를 쓰지 않고 셸 판" "$(gr_)" "돌아옴 0"
  t "그 경고" "$(grep -c '경고: Go 판을 쓸 수 없다' "$T/err")" 1
  t "그때 IMPL=go는 1" "$(gr_ HC_FAKE_IMPL=go)" "돌아옴 1"
  printf 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("옛 시각 판") }\n' >"$M/main.go"; touch -t 202601010000 "$M/main.go"
  t "옛 시각으로 깔린 새 소스(herdr-sync는 맥의 시각 그대로 깐다)도 판이 달라 다시 빌드" "$(gr_)" "옛 시각 판"
  # go:embed 글도 판에 든다(B10·M4)
  printf 'package main\n\nimport (\n\t_ "embed"\n\t"fmt"\n)\n\n//go:embed msg.txt\nvar msg string\n\nfunc main() { fmt.Print(msg) }\n' >"$M/main.go"
  printf 'v1\n' >"$M/msg.txt"
  t "임베드: 빌드" "$(gr_)" v1
  printf 'v2\n' >"$M/msg.txt"; touch -t 202601010000 "$M/msg.txt"
  t "임베드 파일만 바뀌어도(옛 시각이라도) 다시 빌드" "$(gr_)" v2
  rm -f "$T/.bin/hc-fake.src"; mv "$M/msg.txt" "$M/msg.bak"
  t ".src가 없는 바이너리(판 모름)는 빌드가 안 되면 쓰지 않음" "$(gr_)" "돌아옴 0"
  mv "$M/msg.bak" "$M/msg.txt"
  # 받은 바이너리(herdr-sync가 릴리스에서 받아 .bin/<도구>와 <도구>.src를 둠, 2026-11-21 ops-go): 소스 판이 같으면 빌드 없이 그것을
  dl() { printf '#!/bin/sh\necho 받은 판\n' >"$T/.bin/hc-fake"; chmod +x "$T/.bin/hc-fake"; printf '%s\n' "$1" >"$T/.bin/hc-fake.src"; }
  printf 'v4\n' >"$M/msg.txt"
  dl "$(src_)"; sleep 1; touch "$M/main.go" "$M/msg.txt"   # 소스가 받은 바이너리보다 새로워도(시각) 판이 같으면 빌드하지 않는다
  t "받은 바이너리: 판이 같으면 빌드 없이 씀" "$(gr_)" "받은 판"
  t "받은 바이너리: PATH가 비어도(훅) 판을 대조해 씀" "$(gr_ PATH=/없음)" "받은 판"
  printf 'v5\n' >"$M/msg.txt"
  t "받은 바이너리: 임베드 파일만 바뀌어도 판이 달라 기계에서 빌드" "$(gr_)" v5
  t "받은 바이너리: 기계에서 빌드하면 .src는 지은 판" "$(cat "$T/.bin/hc-fake.src" 2>/dev/null)" "$(src_)"
  dl "다른판.2"; mv "$M/msg.txt" "$M/msg.bak"
  t "받은 바이너리: 판이 다르고 빌드도 안 되면 셸 판(받은 옛 판을 안 씀)" "$(gr_)" "돌아옴 0"
  t "그 경고" "$(grep -c '경고: Go 판을 쓸 수 없다' "$T/err")" 1
  dl "다른판.3"
  t "받은 바이너리: 판이 다르고 빌드가 안 되면 IMPL=go는 1" "$(gr_ HC_FAKE_IMPL=go)" "돌아옴 1"
  mv "$M/msg.bak" "$M/msg.txt"
  # 빌드는 이 기계 것으로(L2): GOOS·GOARCH·GOFLAGS가 든 환경에서 불려도 남의 OS 바이너리를 남기지 않는다
  printf 'v6\n' >"$M/msg.txt"
  t "GOOS·GOARCH·GOFLAGS가 든 환경에서도 이 기계 바이너리를 지음" "$(gr_ GOOS=linux GOARCH=amd64 GOFLAGS=-tags=x)" v6
  t "그 뒤 깨끗한 환경에서도 그대로" "$(gr_)" v6
  # 상한(B09): 멈춘 go build·--version은 상한에서 무리째 끊고, auto는 셸 판·go는 1
  mkdir -p "$T/slowgo"; printf '#!/bin/sh\necho $$ >>"%s/slowgo.calls"\nexec sleep 15\n' "$T" >"$T/slowgo/go"; chmod +x "$T/slowgo/go"
  printf '#!/bin/sh\n[ "$1" = --version ] && { echo $$ >"%s/slowbin.pid"; exec sleep 15; }\necho 느린 판\n' "$T" >"$T/slowbin"; chmod +x "$T/slowbin"
  alive() { p=$(tail -1 "$1" 2>/dev/null); [ -n "$p" ] && ! kill -0 "$p" 2>/dev/null && echo 거둠 || echo "남음($p)"; }
  printf 'v7\n' >"$M/msg.txt"; s=$SECONDS
  t "빌드가 멈추면 상한에서 끊고 auto는 셸 판" "$(gr_ PATH="$T/slowgo:$PATH" GO_RUN_BUILD_SECS=1)" "돌아옴 0"
  t "그 빌드 무리를 거둠" "$(alive "$T/slowgo.calls")" 거둠
  t "빌드가 멈추면 IMPL=go는 1" "$(gr_ PATH="$T/slowgo:$PATH" GO_RUN_BUILD_SECS=1 HC_FAKE_IMPL=go)" "돌아옴 1"
  t "--version이 멈추면 상한에서 끊고 auto는 셸 판" "$(gr_ HC_FAKE_BIN="$T/slowbin" GO_RUN_VERSION_SECS=1)" "돌아옴 0"
  t "그 --version 무리를 거둠" "$(alive "$T/slowbin.pid")" 거둠
  t "멈춤 셋이 모두 상한 안에 끝남" "$([ $((SECONDS - s)) -lt 12 ] && echo 예 || echo "아니오 $((SECONDS - s))초")" 예
  # GO_RUN_BUILD=bg(Claude Code 훅 — 제한 5초 안에 짓지 않는다, 2026-11-21 hooks-go): 낡았으면 짓기는 뒤에서, 이번은 셸 판(경고 없이 0)
  # 짓기 잠금(B30)은 짓는 무리가 쥔 flock — 짓는 쪽이 살아 있으면 오래돼도 뺏지 않고, 끝나거나 죽으면 저절로 풀린다
  held() { perl -MFcntl=:flock -e 'open(my $f, ">>", shift) or exit 1; exit(flock($f, LOCK_EX | LOCK_NB) ? 1 : 0)' "$T/.bin/hc-fake.build.lock"; }
  bgwait() { for i in $(seq 1 240); do held || return 0; sleep 0.5; done; }
  printf 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("bg 판") }\n' >"$M/main.go"; : >"$T/slowgo.calls"
  t "bg: 소스가 바뀌면 옛 바이너리를 부르지 않고 0" "$(gr_ GO_RUN_BUILD=bg PATH="$T/slowgo:$PATH" GO_RUN_BUILD_SECS=4)" "돌아옴 0"
  t "bg: 경고 없음" "$(grep -c '경고' "$T/err")" 0
  [ ! -d "$T/.bin/hc-fake.building" ] || touch -t 202601010000 "$T/.bin/hc-fake.building"   # 옛 판의 나이 잠금이면 묵혀 둔다(B30 재현)
  gr_ GO_RUN_BUILD=bg PATH="$T/slowgo:$PATH" GO_RUN_BUILD_SECS=4 >/dev/null; sleep 1
  t "bg: 짓는 쪽이 살아 있으면 다시 짓지 않음" "$(wc -l <"$T/slowgo.calls" | tr -d ' ')" 1
  bgwait
  t "bg: 멈춘 짓기도 상한에서 끝나 잠금이 풀림" "$(held && echo 쥠 || echo 풀림) $(alive "$T/slowgo.calls")" "풀림 거둠"
  t "bg: 실제 go로 다시 — 이번은 0" "$(gr_ GO_RUN_BUILD=bg)" "돌아옴 0"
  bgwait
  t "bg: 뒤에서 지은 새 판을 다음 부름에 씀" "$(gr_ GO_RUN_BUILD=bg)" "bg 판"
  t "bg: 짓기가 끝나면 잠금이 풀림" "$(held && echo 쥠 || echo 풀림)" 풀림
  t "bg: IMPL=go인데 쓸 수 없으면 1" "$(gr_ GO_RUN_BUILD=bg HC_FAKE_BIN=/없는/바이너리 HC_FAKE_IMPL=go)" "돌아옴 1"
  t "PATH가 비어도(훅·시험이 비운다) 스위치를 읽는다" "$(gr_ PATH=/없음 HC_FAKE_IMPL=bash)" "돌아옴 0"
  t "PATH가 비어도 _BIN을 쓴다" "$(gr_ PATH=/없음 HC_FAKE_BIN="$T/.bin/hc-fake")" "bg 판"
  t "확인한 기본 바이너리는 .ok 표식" "$([ -e "$T/.bin/hc-fake.ok" ] && echo 있음 || echo 없음)" 있음
  sleep 1; printf '#!/bin/sh\nexit 3\n' >"$T/.bin/hc-fake.new"; chmod +x "$T/.bin/hc-fake.new"; mv -f "$T/.bin/hc-fake.new" "$T/.bin/hc-fake"
  t ".ok보다 새 바이너리는 다시 확인 — --version이 안 되면 판이 같아도 다시 지어 씀(L2)" "$(gr_)" "bg 판"
  t "go_run_up: 대문자·_" "$(. "$T/lib/go-run.sh"; go_run_up herdr-hooks-x9; echo "$go_run_up_r")" HERDR_HOOKS_X9
fi
exit $fail
