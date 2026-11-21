# hc-fill 함수: go (bin/hc-fill가 맨 먼저 읽는다) — Go 판(cmd/hc-fill) 부르기 (2026-11-21 사용자 「모두 고로 옮기고 싶어」, 레인 hc-fill-go)
#
# 읽히는 순간 Go 판으로 exec한다 — 되면 돌아오지 않는다. 부르기·빌드·되돌리기는 공통 실행기 lib/go-run.sh(go_run):
#  · HC_FILL_IMPL(없으면 HERDR_IMPL)=bash: 되돌리기 스위치 — 그대로 이 셸 판(lib/hc-fill/*.sh)
#  · HC_FILL_BIN: 이 바이너리를 그대로. 없으면 go_run이 소스가 새로우면 그 기계에서 go build
#  · 못 만들거나 실행이 안 되면 크게 경고하고 셸 판으로(대기열이 멈추지 않게). HC_FILL_IMPL=go로 못박았으면 종료 1
# Go 판은 자기를 다시 부를 때(watch의 run, 배경 _gc) HC_FILL_SELF(이 포장)를 쓴다 — 그때도 이 스위치를 다시 거친다.
# 셸 판으로 돌기로 했으면 HC_FILL_INNER=1을 물려 셸 판이 부르는 자기 자신(lockf 안 run·_set 등)이 다시 고르지 않게 한다
if [ -z "${HC_FILL_INNER-}" ]; then
  if . "$hf_d/../go-run.sh" 2>/dev/null; then
    case $0 in /*) hf_self=$0 ;; */*) hf_self=$PWD/$0 ;; *) hf_self=$(command -v "$0" 2>/dev/null || echo "$0") ;; esac
    HC_FILL_SELF=$hf_self go_run hc-fill "$@"
    case $? in
      0) ;;
      1) printf 'hc-fill: Go 판을 쓸 수 없다(HC_FILL_IMPL=go)\n' >&2; exit 1 ;;
      *) exit 2 ;;
    esac
  else
    case ${HC_FILL_IMPL:-${HERDR_IMPL:-auto}} in
      bash | sh) ;;
      go) printf 'hc-fill: 공통 실행기를 못 읽었다(HC_FILL_IMPL=go): %s\n' "$hf_d/../go-run.sh" >&2; exit 1 ;;
      *) printf 'hc-fill: !!!!!! 경고: 공통 실행기를 못 읽었다(%s — herdr-sync가 lib/go-run.sh도 까는지) — 셸 판으로 돈다 !!!!!!\n' "$hf_d/../go-run.sh" >&2 ;;
    esac
  fi
  export HC_FILL_INNER=1
fi
