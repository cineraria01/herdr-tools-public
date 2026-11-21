# hc-lane 함수: merger-go (bin/hc-lane가 읽는다) — 병합기 Go 판 부르기와 제출 우선
# (2026-11-20 사용자 「이걸 러스트로 만들거나 하는건 어떨까?」 → 「Go로, 마무리 뒤 (추천)」. 병합기·잠금 결함 다수가 bash 함정이었다)

# merger_go <merger 인자…>: Go 판(cmd/hc-merger)으로 exec — 돌아오지 않는다. 돌아오면 bash 판(merger_cmd)으로 돈다.
# 부르기·빌드·되돌리기는 공통 실행기 lib/go-run.sh(go_run)가 한다:
#  · HC_LANE_MERGER_IMPL(=HC_MERGER_IMPL, 없으면 HERDR_IMPL)=bash: 되돌리기 스위치 — 그대로 bash 판(lib/hc-lane/merger*.sh)
#  · HC_LANE_MERGER_BIN(=HC_MERGER_BIN): 이 바이너리를 그대로. 없으면 go_run이 소스가 새로우면 그 기계에서 go build
#  · 못 만들거나 실행이 안 되면 크게 경고하고 bash 판으로(병합기가 멈추지 않게). IMPL=go로 못박았으면 종료 1(시험이 bash로 조용히 통과하지 않게)
#  · 도움말(-h)은 bash 판 usage
merger_go() {
  local a self rc gr=$hl_d/../go-run.sh
  for a; do case $a in -h|--help) return 0 ;; esac; done
  if ! . "$gr" 2>/dev/null; then
    case ${HC_LANE_MERGER_IMPL:-${HC_MERGER_IMPL:-${HERDR_IMPL:-auto}}} in bash|sh) return 0 ;; go) die "merger: 공통 실행기를 못 읽었다(HC_LANE_MERGER_IMPL=go): $gr" ;; esac
    echo "hc-lane merger: !!!!!! 경고: 공통 실행기를 못 읽었다($gr — herdr-sync가 lib/go-run.sh도 까는지) — bash 판으로 돈다 !!!!!!" >&2
    return 0
  fi
  case $0 in /*) self=$0 ;; */*) self=$PWD/$0 ;; *) self=$(command -v "$0" 2>/dev/null || echo "$0") ;; esac
  # 잠금 fd(18·19)는 Go 판에 물려주지 않는다
  HC_MERGER_IMPL=${HC_LANE_MERGER_IMPL:-${HC_MERGER_IMPL-}} HC_MERGER_BIN=${HC_LANE_MERGER_BIN:-${HC_MERGER_BIN-}} HC_LANE_SELF=$self \
    go_run hc-merger "$@" 18>&- 19>&-
  rc=$?
  case $rc in
    0) ;;
    1) die "merger: Go 판을 쓸 수 없다(HC_LANE_MERGER_IMPL=go)" ;;
    *) die "HC_LANE_MERGER_IMPL은 go·bash 가운데 하나: ${HC_LANE_MERGER_IMPL:-${HC_MERGER_IMPL:-${HERDR_IMPL-}}}" ;;
  esac
}

# lprio: 제출 우선(HC_LANE_PRIORITY — 긴급·높음·보통·낮음, urgent·high·normal·low도). 없으면 보통, 모르는 값은 보통과 경고.
# 제출 기록에 priority=<값>으로 싣고, 병합기 Go 판이 우선 → 제출 시각 순으로 고른다
lprio() {
  case ${HC_LANE_PRIORITY-} in
    ''|보통|normal) printf 보통 ;; 긴급|urgent) printf 긴급 ;; 높음|high) printf 높음 ;; 낮음|low) printf 낮음 ;;
    *) echo "hc-lane land: 경고 — HC_LANE_PRIORITY는 긴급·높음·보통·낮음 가운데 하나: $HC_LANE_PRIORITY (보통으로 낸다)" >&2; printf 보통 ;;
  esac
}
