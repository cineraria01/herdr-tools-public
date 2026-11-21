# hc-lane 함수: merger-go (bin/hc-lane가 읽는다) — 병합기 Go 판 부르기와 제출 우선
# (2026-11-20 사용자 「이걸 러스트로 만들거나 하는건 어떨까?」 → 「Go로, 마무리 뒤 (추천)」. 병합기·잠금 결함 다수가 bash 함정이었다)

# merger_go <merger 인자…>: Go 판(lib/hc-merger, 단일 바이너리)으로 exec — 돌아오지 않는다. 돌아오면 bash 판(merger_cmd)으로 돈다.
#  · HC_LANE_MERGER_IMPL=bash: 되돌리기 스위치 — 그대로 bash 판(lib/hc-lane/merger*.sh)
#  · 바이너리: HC_LANE_MERGER_BIN, 없으면 소스 곁 <lib>/hc-merger/.bin/hc-merger. 소스(*.go·go.mod)가 그보다 새로우면 그 기계에서 go build
#    (herdr-sync는 소스만 깐다 — 점으로 시작하는 .bin은 나무 줄이 걷지 않는다). 빌드 캐시도 그 곁(.bin/go-build — HOME과 상관없이 한 번)
#  · 못 만들거나 실행이 안 되면 크게 경고하고 bash 판으로(병합기가 멈추지 않게). IMPL=go로 못박았으면 종료 1(시험이 bash로 조용히 통과하지 않게)
#  · 도움말(-h)은 bash 판 usage
merger_go() {
  local impl=${HC_LANE_MERGER_IMPL:-auto} src bin gob a self
  case $impl in bash) return 0 ;; go|auto) ;; *) die "HC_LANE_MERGER_IMPL은 go·bash 가운데 하나: $impl" ;; esac
  for a; do case $a in -h|--help) return 0 ;; esac; done
  src=$hl_d/../hc-merger bin=${HC_LANE_MERGER_BIN:-$hl_d/../hc-merger/.bin/hc-merger}
  if [ -z "${HC_LANE_MERGER_BIN-}" ] && [ -f "$src/go.mod" ] &&
    { [ ! -x "$bin" ] || [ -n "$(find "$src" -maxdepth 1 \( -name '*.go' -o -name go.mod \) -newer "$bin" 2>/dev/null)" ]; }; then
    gob=$(command -v go 2>/dev/null) || gob=
    for a in /opt/homebrew/bin/go /usr/local/go/bin/go /usr/local/bin/go; do [ -n "$gob" ] || [ ! -x "$a" ] || gob=$a; done
    if [ -n "$gob" ] && mkdir -p "$src/.bin"; then
      # 다른 병합기가 같이 빌드해도 반쪽 파일을 부르지 않게 제 이름으로 만든 뒤 바꿔 넣는다
      (cd "$src" && GOCACHE=${GOCACHE:-$PWD/.bin/go-build} GOTOOLCHAIN=local CGO_ENABLED=0 "$gob" build -trimpath -o ".bin/hc-merger.$$" .) >&2 &&
        mv -f "$src/.bin/hc-merger.$$" "$bin" || rm -f "$src/.bin/hc-merger.$$"
    fi
  fi
  if [ -x "$bin" ] && "$bin" --version >/dev/null 2>&1; then
    case $0 in /*) self=$0 ;; */*) self=$PWD/$0 ;; *) self=$(command -v "$0" 2>/dev/null || echo "$0") ;; esac
    HC_LANE_SELF=$self exec "$bin" "$@" 18>&- 19>&-
  fi
  [ "$impl" = auto ] || die "merger: Go 판을 쓸 수 없다(HC_LANE_MERGER_IMPL=go): $bin"
  printf '%s\n' "hc-lane merger: !!!!!! 경고: 병합기 Go 판을 쓸 수 없다($bin — go가 없거나 build 실패) — bash 판으로 돈다 !!!!!!" \
    "hc-lane merger: 고치기: 그 기계에 go를 깔고(brew install go) 병합기를 다시 띄운다. 일부러 bash 판이면 HC_LANE_MERGER_IMPL=bash" >&2
}

# lprio: 제출 우선(HC_LANE_PRIORITY — 긴급·높음·보통·낮음, urgent·high·normal·low도). 없으면 보통, 모르는 값은 보통과 경고.
# 제출 기록에 priority=<값>으로 싣고, 병합기 Go 판이 우선 → 제출 시각 순으로 고른다
lprio() {
  case ${HC_LANE_PRIORITY-} in
    ''|보통|normal) printf 보통 ;; 긴급|urgent) printf 긴급 ;; 높음|high) printf 높음 ;; 낮음|low) printf 낮음 ;;
    *) echo "hc-lane land: 경고 — HC_LANE_PRIORITY는 긴급·높음·보통·낮음 가운데 하나: $HC_LANE_PRIORITY (보통으로 낸다)" >&2; printf 보통 ;;
  esac
}
