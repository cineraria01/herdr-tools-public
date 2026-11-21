# go-run.sh: Go 판 도구 공통 실행기 — bin/<도구>(또는 그 lib)가 source해 go_run을 부른다 (2026-11-21 사용자 「Go로」, go-foundation)
#
# go_run <도구> [인자…]: Go 판(cmd/<도구>, 단일 바이너리)으로 exec — 되면 돌아오지 않는다. 돌아오면 종료 코드로 셸 판을 고른다:
#   0 셸 판으로 돈다(스위치가 bash거나, auto인데 Go 판을 쓸 수 없어 크게 경고함) · 1 스위치가 go인데 쓸 수 없음 · 2 스위치 값이 틀림
#  · 스위치: <도구>_IMPL(대문자·-는 _, 예 HC_MERGER_IMPL), 없으면 HERDR_IMPL, 없으면 auto. go·auto·bash(sh) 가운데 하나
#  · 바이너리: <도구>_BIN이 있으면 그것(빌드 안 함). 없으면 <모듈>/.bin/<도구> — 소스(go.mod·cmd/**·internal/**의 *.go)가 그보다
#    새로우면 그 기계에서 go build(-trimpath, CGO_ENABLED=0, GOTOOLCHAIN=local, 빌드 캐시·GOPATH <모듈>/.bin/ — HOME과 상관없이).
#    herdr-sync는 소스만 깐다(점으로 시작하는 .bin은 나무 줄이 걷지 않는다)
#  · 모듈 자리: 저장소에서는 이 파일의 ../(go.mod가 있는 루트), 깔린 곳에서는 이 파일 곁 herdr-go/(~/.local/lib/herdr-go)
#  · 바이너리는 --version이 0으로 끝나야 쓴다(깨진 바이너리를 부르지 않게)
go_run_root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
if [ -f "$go_run_root/../go.mod" ]; then go_run_root=$(cd "$go_run_root/.." && pwd); else go_run_root=$go_run_root/herdr-go; fi

go_run() {
  local tool=$1 up v impl bin gob a
  shift
  up=$(printf %s "$tool" | tr 'a-z-' 'A-Z_')
  v=${up}_IMPL impl=${!v:-${HERDR_IMPL:-auto}}
  case $impl in
    bash|sh) return 0 ;;
    go|auto) ;;
    *) echo "$tool: ${up}_IMPL·HERDR_IMPL은 go·bash 가운데 하나: $impl" >&2; return 2 ;;
  esac
  v=${up}_BIN bin=${!v-}
  if [ -z "$bin" ]; then
    bin=$go_run_root/.bin/$tool
    if [ -d "$go_run_root/cmd/$tool" ] && { [ ! -x "$bin" ] || [ -n "$(find "$go_run_root/go.mod" "$go_run_root/cmd" "$go_run_root/internal" \
      \( -name '*.go' -o -name go.mod \) -newer "$bin" 2>/dev/null | head -n 1)" ]; }; then
      gob=$(command -v go 2>/dev/null) || gob=
      for a in /opt/homebrew/bin/go /usr/local/go/bin/go /usr/local/bin/go; do [ -n "$gob" ] || [ ! -x "$a" ] || gob=$a; done
      if [ -n "$gob" ] && mkdir -p "$go_run_root/.bin"; then
        # 다른 실행이 같이 빌드해도 반쪽 파일을 부르지 않게 제 이름으로 만든 뒤 바꿔 넣는다
        # GOPATH도 곁에: 기본값 $HOME/go에 pkg/mod를 만들지 않게(시험이 HOME을 임시 폴더로 둔다). -buildvcs=false: 저장소 안에서도 git 상태에 안 걸림
        (cd "$go_run_root" && GOCACHE=${GOCACHE:-$PWD/.bin/go-build} GOPATH=${GOPATH:-$PWD/.bin/gopath} GOTOOLCHAIN=local CGO_ENABLED=0 \
          "$gob" build -trimpath -buildvcs=false -o ".bin/$tool.$$" "./cmd/$tool") >&2 &&
          mv -f "$go_run_root/.bin/$tool.$$" "$bin" || rm -f "$go_run_root/.bin/$tool.$$"
      fi
    fi
  fi
  if [ -x "$bin" ] && "$bin" --version >/dev/null 2>&1; then exec "$bin" "$@"; fi
  [ "$impl" = auto ] || { echo "$tool: Go 판을 쓸 수 없다(${up}_IMPL=go): $bin" >&2; return 1; }
  printf '%s\n' "$tool: !!!!!! 경고: Go 판을 쓸 수 없다($bin — go가 없거나 build 실패) — 셸 판으로 돈다 !!!!!!" \
    "$tool: 고치기: 그 기계에 go를 깔고(brew install go) 다시 부른다. 일부러 셸 판이면 ${up}_IMPL=bash" >&2
  return 0
}
