# go-run.sh: Go 판 도구 공통 실행기 — bin/<도구>(또는 그 lib)가 source해 go_run을 부른다 (2026-11-21 사용자 「Go로」, go-foundation)
#
# go_run <도구> [인자…]: Go 판(cmd/<도구>, 단일 바이너리)으로 exec — 되면 돌아오지 않는다. 돌아오면 종료 코드로 셸 판을 고른다:
#   0 셸 판으로 돈다(스위치가 bash거나, auto인데 Go 판을 쓸 수 없어 크게 경고함) · 1 스위치가 go인데 쓸 수 없음 · 2 스위치 값이 틀림
#  · 스위치: <도구>_IMPL(대문자·-는 _, 예 HC_MERGER_IMPL), 없으면 HERDR_IMPL, 없으면 auto. go·auto·bash(sh) 가운데 하나
#  · 바이너리: <도구>_BIN이 있으면 그것(빌드 안 함). 없으면 <모듈>/.bin/<도구> — 소스(go.mod·cmd/**·internal/**의 파일 — go:embed 글 포함)가 그보다
#    새로우면 그 기계에서 go build(-trimpath, CGO_ENABLED=0, GOTOOLCHAIN=local, 빌드 캐시·GOPATH <모듈>/.bin/ — HOME과 상관없이).
#    herdr-sync는 소스만 깐다(점으로 시작하는 .bin은 나무 줄이 걷지 않는다)
#  · 모듈 자리: 저장소에서는 이 파일의 ../(go.mod가 있는 루트), 깔린 곳에서는 이 파일 곁 herdr-go/(~/.local/lib/herdr-go)
#  · 바이너리는 --version이 0으로 끝나야 쓴다(깨진 바이너리를 부르지 않게)
#  · 받은 바이너리(2026-11-21 사용자 「각 컴퓨터 마다 빌드 하지말고 github 에다가 빌드 한것도 처리해서 … 받아서」): herdr-sync Go 판이 맥에서 지어
#    공개 릴리스에 올리고 기계마다 <모듈>/.bin/<도구>에 받아 두며 곁에 <도구>.src(그 소스 판)를 적는다. 그 판이 지금 소스 판(go_run_src)과
#    같으면 빌드하지 않고 쓴다. 다르면 그 기계에서 빌드하고(되면 .src를 지움), 빌드도 안 되면 받은 옛 바이너리를 쓰지 않고 셸 판
go_run_root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
if [ -f "$go_run_root/../go.mod" ]; then go_run_root=$(cd "$go_run_root/.." && pwd); else go_run_root=$go_run_root/herdr-go; fi

# go_run_src: 모듈 소스 판 — go.mod·cmd/**·internal/** 의 *.go·go.mod를 상대 경로 바이트 순으로 cksum한 목록의 cksum(crc.크기).
# herdr-sync Go 판 srcDigest와 같은 값(그 시험이 견준다)
go_run_src() {
  (cd "$go_run_root" && find go.mod cmd internal -type f \( -name '*.go' -o -name go.mod \) 2>/dev/null | LC_ALL=C sort | tr '\n' '\0' |
    xargs -0 cksum | cksum | awk '{print $1"."$2}')
}

# go_run_daemon: launchd·cron이 띄운 문맥인가(0 그렇다). 그 문맥의 새 Go 바이너리는 ~/Documents 첫 접근에서 macOS 개인 정보 보호(TCC)
# 창을 띄우고 사람 응답까지 잠든다 — ad-hoc 서명이 빌드마다 바뀌어 「허용」해도 새 판마다 다시 묻는다(2026-11-21 맥 hc-fill 37분 멈춤).
# 실측(2026-11-21 macOS 27): launchd 작업도 XPC_SERVICE_NAME=0이라 그것으로는 못 가른다. launchd 작업에는 TERM·HC_TERMINAL_ID가 없고,
# Herdr 터미널·hc exec(코어 문맥 — 창이 안 뜸)는 조상에 herdr-core가 있다. 그래서: 터미널·Herdr 터미널·SSH 표식이 다 없고
# 조상에 herdr-core·sshd가 없으면 그렇다. ps를 못 읽으면 그렇다(셸 판은 늘 돈다 — 창에 멈추는 쪽을 고르지 않는다).
# TERM=dumb도 없음으로 본다: launchd의 bash는 TERM이 없으면 스스로 dumb을 둔다(실측)
go_run_daemon() {
  [ -z "${HC_TERMINAL_ID-}" ] && [ "${TERM:-dumb}" = dumb ] && [ -z "${SSH_CONNECTION-}" ] || return 1
  ps -axo pid=,ppid=,comm= 2>/dev/null | awk -v p="$$" '
    { c = $3; for (i = 4; i <= NF; i++) c = c " " $i; sub(/.*\//, "", c); pp[$1] = $2; cm[$1] = c }
    END { for (k = 0; k < 64 && (p in pp) && p > 1; k++) { if (cm[p] ~ /^(herdr-core|sshd|sshd-session)$/) exit 1; p = pp[p] }; exit 0 }'
}

go_run() {
  local tool=$1 up v impl bin gob a fresh= stale= note
  shift
  up=$(printf %s "$tool" | tr 'a-z-' 'A-Z_')
  v=${up}_IMPL impl=${!v:-${HERDR_IMPL:-auto}}
  case $impl in
    bash|sh) return 0 ;;
    go|auto) ;;
    *) echo "$tool: ${up}_IMPL·HERDR_IMPL은 go·bash 가운데 하나: $impl" >&2; return 2 ;;
  esac
  # launchd 문맥이면 auto는 셸 판(IMPL=go로 못박으면 그대로 Go 판). 안내는 도구마다 처음 한 번(표식 파일)
  if [ "$impl" = auto ] && go_run_daemon; then
    note=${XDG_STATE_HOME:-$HOME/.local/state}/herdr-go/launchd-noted-$tool
    if [ ! -e "$note" ]; then
      mkdir -p "${note%/*}" 2>/dev/null && : >"$note" 2>/dev/null
      echo "$tool: launchd 문맥(터미널 밖)이라 셸 판으로 돈다 — Go 바이너리는 macOS 문서 폴더 권한 창에 멈춘다. Go 판은 hc-launchd-guard로 Herdr 터미널에서(README 「launchd 작업」)" >&2
    fi
    return 0
  fi
  v=${up}_BIN bin=${!v-}
  if [ -z "$bin" ]; then
    bin=$go_run_root/.bin/$tool
    if [ -f "$bin.src" ]; then   # 받은 바이너리: 소스 판이 같으면 그대로, 다르면 빌드(아래)
      if [ -x "$bin" ] && [ "$(cat "$bin.src" 2>/dev/null)" = "$(go_run_src)" ]; then fresh=1; else stale=1; fi
    fi
    # 새로움은 *.go만이 아니라 go:embed 파일(cmd/hc-lane/usage.txt)도 본다(hc-lane-go-start)
    if [ -z "$fresh" ] && [ -d "$go_run_root/cmd/$tool" ] && { [ -n "$stale" ] || [ ! -x "$bin" ] || [ -n "$(find "$go_run_root/go.mod" "$go_run_root/cmd" "$go_run_root/internal" \
      -type f -newer "$bin" 2>/dev/null | head -n 1)" ]; }; then
      gob=$(command -v go 2>/dev/null) || gob=
      for a in /opt/homebrew/bin/go /usr/local/go/bin/go /usr/local/bin/go; do [ -n "$gob" ] || [ ! -x "$a" ] || gob=$a; done
      if [ -n "$gob" ] && mkdir -p "$go_run_root/.bin"; then
        # 다른 실행이 같이 빌드해도 반쪽 파일을 부르지 않게 제 이름으로 만든 뒤 바꿔 넣는다
        # GOPATH도 곁에: 기본값 $HOME/go에 pkg/mod를 만들지 않게(시험이 HOME을 임시 폴더로 둔다). -buildvcs=false: 저장소 안에서도 git 상태에 안 걸림
        (cd "$go_run_root" && GOCACHE=${GOCACHE:-$PWD/.bin/go-build} GOPATH=${GOPATH:-$PWD/.bin/gopath} GOTOOLCHAIN=local CGO_ENABLED=0 \
          "$gob" build -trimpath -buildvcs=false -o ".bin/$tool.$$" "./cmd/$tool") >&2 &&
          mv -f "$go_run_root/.bin/$tool.$$" "$bin" && rm -f "$bin.src" && stale= || rm -f "$go_run_root/.bin/$tool.$$"
      fi
    fi
    [ -z "$stale" ] || bin="$bin (받은 바이너리의 판이 소스와 다르고 이 기계에서 빌드도 못 함)"   # 쓰지 않는다 — 아래에서 셸 판으로
  fi
  if [ -x "$bin" ] && "$bin" --version >/dev/null 2>&1; then exec "$bin" "$@"; fi
  [ "$impl" = auto ] || { echo "$tool: Go 판을 쓸 수 없다(${up}_IMPL=go): $bin" >&2; return 1; }
  printf '%s\n' "$tool: !!!!!! 경고: Go 판을 쓸 수 없다($bin — go가 없거나 build 실패) — 셸 판으로 돈다 !!!!!!" \
    "$tool: 고치기: 그 기계에 go를 깔고(brew install go) 다시 부른다. 일부러 셸 판이면 ${up}_IMPL=bash" >&2
  return 0
}
