#!/bin/bash
# Go 판 단위 시험(go.mod 모듈 전체 — cmd/hc-merger·internal/*): gofmt·go vet·go test -race(프로세스 무리 정리·잠금 안 물림·git 모름 → 쉼·
# 우선순위·복구 단계별 죽기·사건 줄 꼴·원자 쓰기). 공통 실행기 lib/go-run.sh가 바이너리를 짓고 고르는지도 본다.
# 운영 도구 계약 시험(cmd/<도구>/contract*_test.go — 옛 tests/<도구>.test.sh)도 여기서 Go 판(기본)으로 돈다. 셸 판은 tests/ops-go-bash.test.sh
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
if "$gob" test -race -count=1 -timeout 600s ./...; then echo "ok - go test -race"; else echo "FAIL - go test -race"; fail=1; fi

# 공통 실행기(lib/go-run.sh — 저장소에서만. 깔린 곳은 ~/.local/lib/go-run.sh가 같은 파일). 가짜 도구 모듈로 본다
gr=$here/../lib/go-run.sh
if [ -f "$gr" ]; then
  T=$(mktemp -d "${TMPDIR:-/tmp}/go-run.XXXXXX") || exit 1
  mkdir -p "$T/lib" "$T/cmd/hc-fake"
  cp "$gr" "$T/lib/go-run.sh"
  printf 'module fake\n\ngo 1.24\n' >"$T/go.mod"
  printf 'package main\n\nimport (\n\t"fmt"\n\t"os"\n)\n\nfunc main() { fmt.Println("go", len(os.Args)-1, os.Getenv("FAKE_ENV")) }\n' >"$T/cmd/hc-fake/main.go"
  mkdir -p "$T/home"
  gr_() { (unset HC_FAKE_IMPL HC_FAKE_BIN HERDR_IMPL GOPATH GOCACHE; export HOME=$T/home; for kv in "$@"; do export "$kv"; done
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
  sleep 1; printf 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("새 판") }\n' >"$T/cmd/hc-fake/main.go"
  t "소스가 새로우면 다시 빌드" "$(gr_)" "새 판"
  printf 'package main\n\nfunc main() { 깨짐 }\n' >"$T/cmd/hc-fake/main.go"; touch "$T/cmd/hc-fake/main.go"
  t "빌드 실패면 옛 바이너리를 그대로(새 판)" "$(gr_)" "새 판"
  # 받은 바이너리(herdr-sync가 릴리스에서 받아 .bin/<도구>와 <도구>.src를 둠, 2026-11-21 ops-go): 소스 판이 같으면 빌드 없이 그것을
  printf 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("기계 빌드") }\n' >"$T/cmd/hc-fake/main.go"
  dl() { printf '#!/bin/sh\necho 받은 판\n' >"$T/.bin/hc-fake"; chmod +x "$T/.bin/hc-fake"; printf '%s\n' "$1" >"$T/.bin/hc-fake.src"; }
  src=$(cd "$T" && bash -c '. lib/go-run.sh && go_run_src')
  dl "$src"; sleep 1; touch "$T/cmd/hc-fake/main.go"   # 소스가 받은 바이너리보다 새로워도(시각) 판이 같으면 빌드하지 않는다
  t "받은 바이너리: 판이 같으면 빌드 없이 씀" "$(gr_)" "받은 판"
  t "받은 바이너리: PATH가 비어도(훅) 판을 대조해 씀" "$(gr_ PATH=/없음)" "받은 판"
  dl "다른판.1"
  t "받은 바이너리: 판이 다르면 기계에서 빌드" "$(gr_)" "기계 빌드"
  t "받은 바이너리: 빌드하면 .src를 지움" "$([ -e "$T/.bin/hc-fake.src" ] && echo 있음 || echo 없음)" 없음
  dl "다른판.2"; printf 'package main\n\nfunc main() { 깨짐 }\n' >"$T/cmd/hc-fake/main.go"
  t "받은 바이너리: 판이 다르고 빌드도 안 되면 셸 판(받은 옛 판을 안 씀)" "$(gr_)" "돌아옴 0"
  t "그 경고" "$(grep -c '경고: Go 판을 쓸 수 없다' "$T/err")" 1
  dl "다른판.3"
  t "받은 바이너리: 판이 다르고 빌드가 안 되면 IMPL=go는 1" "$(gr_ HC_FAKE_IMPL=go)" "돌아옴 1"
  # GO_RUN_BUILD=bg(Claude Code 훅 — 제한 5초 안에 짓지 않는다, 2026-11-21 hooks-go): 낡았으면 짓기는 뒤에서, 이번은 셸 판(경고 없이 0)
  sleep 1; printf 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("bg 판") }\n' >"$T/cmd/hc-fake/main.go"
  t "bg: 소스가 새로우면 옛 바이너리를 부르지 않고 0" "$(gr_ GO_RUN_BUILD=bg)" "돌아옴 0"
  t "bg: 경고 없음" "$(grep -c '경고' "$T/err")" 0
  for i in $(seq 1 240); do [ -d "$T/.bin/hc-fake.building" ] || break; sleep 0.5; done
  t "bg: 뒤에서 지은 새 판을 다음 부름에 씀" "$(gr_ GO_RUN_BUILD=bg)" "bg 판"
  t "bg: 짓기 잠금을 남기지 않음" "$([ -e "$T/.bin/hc-fake.building" ] && echo 있음 || echo 없음)" 없음
  t "bg: IMPL=go인데 쓸 수 없으면 1" "$(gr_ GO_RUN_BUILD=bg HC_FAKE_BIN=/없는/바이너리 HC_FAKE_IMPL=go)" "돌아옴 1"
  t "PATH가 비어도(훅·시험이 비운다) 스위치를 읽는다" "$(gr_ PATH=/없음 HC_FAKE_IMPL=bash)" "돌아옴 0"
  t "PATH가 비어도 _BIN을 쓴다" "$(gr_ PATH=/없음 HC_FAKE_BIN="$T/.bin/hc-fake")" "bg 판"
  t "확인한 기본 바이너리는 .ok 표식" "$([ -e "$T/.bin/hc-fake.ok" ] && echo 있음 || echo 없음)" 있음
  sleep 1; printf '#!/bin/sh\nexit 3\n' >"$T/.bin/hc-fake.new"; chmod +x "$T/.bin/hc-fake.new"; mv -f "$T/.bin/hc-fake.new" "$T/.bin/hc-fake"
  t ".ok보다 새 바이너리는 다시 확인(깨졌으면 auto는 경고하고 0)" "$(gr_)" "돌아옴 0"
  t "go_run_up: 대문자·_" "$(. "$T/lib/go-run.sh"; go_run_up herdr-hooks-x9; echo "$go_run_up_r")" HERDR_HOOKS_X9
fi
exit $fail
