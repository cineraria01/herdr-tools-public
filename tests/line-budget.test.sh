#!/bin/bash
# 줄 수 톱니바퀴(2026-11-20 herdr-tools-split — hc-lane 2,044줄·hc-fill 1,271줄을 여러 레인이 함께 고쳐 서로 기다리고 부딪혔다).
# 시험 아닌 추적 파일(tests/·*/tests/·*.md 빼고)은 한도(아래 LIMIT)를 넘지 않는다. 이미 넘은 파일은 tests/line-budget.tsv
# 「<줄 수> <경로>」에 적힌 줄 수까지만 — 늘면 실패(줄이면 통과. 예산은 사람·지휘가 낮춘다). 새 거대 파일은 예산 없이 실패:
# 늘릴 일은 새 기능 파일(lib/<도구>/<모듈>.sh 등)로 나눈다.
# 레인 안(HC_LANE)에서는 예산 파일을 늘리지 못한다: origin/main과의 갈래점 판보다 예산 줄이 늘거나 새로 생기면 실패
# (줄이거나 지우기만은 된다. 갈래점에 예산 파일이 없으면 — 처음 만드는 판 — 안 본다).
# 실행: bash tests/line-budget.test.sh   (저장소 안에서. git이 없으면 건너뜀)
set -u
here=$(cd "$(dirname "$0")" && pwd)
LIMIT=600
B=tests/line-budget.tsv
fail=0
ok() { echo "ok - $*"; }
bad() { echo "FAIL - $*"; fail=1; }

# check <폴더> <예산 파일> [<갈래점 예산 파일>]: 위반마다 한 줄, 없으면 아무것도 안 찍는다
check() {
  (cd "$1" && git ls-files -z | xargs -0 wc -l 2>/dev/null | grep -v ' total$') |
    awk -v lim="$LIMIT" -v lane="${4-}" '
      FILENAME == ARGV[1] { if ($0 !~ /^#/ && NF == 2) b[$2] = $1; next }
      FILENAME == ARGV[2] { if ($0 !~ /^#/ && NF == 2) o[$2] = $1; ob = 1; next }
      { n = $1; p = $0; sub(/^[ \t]*[0-9]+[ \t]/, "", p); if (p ~ /(^|\/)tests\// || p ~ /\.md$/) next; seen[p] = 1
        if (p in b) { if (n > b[p]) printf "늘었다 %s %d줄 (예산 %d) — 늘릴 것은 새 기능 파일로\n", p, n, b[p] }
        else if (n > lim) printf "새 거대 파일 %s %d줄 (한도 %d) — 기능별 파일로 나눈다\n", p, n, lim }
      END { for (p in b) if (!(p in seen)) printf "예산에 없는 파일 %s — 예산 줄을 지운다\n", p
            if (lane != "" && ob) for (p in b) if (!(p in o) || b[p] > o[p]) printf "레인이 예산을 늘렸다 %s %d (갈래점 %s)\n", p, b[p], (p in o) ? o[p] : "없음" }' \
      "$2" "${3:-/dev/null}" -
}

# 자기 시험: 임시 저장소에서 늘어남·새 거대 파일·예산 밖·레인의 예산 올림이 걸리는지
T=$(mktemp -d "${TMPDIR:-/tmp}/line-budget-test.XXXXXX") || exit 1
trap 'rm -rf "$T"' EXIT
mkdir -p "$T/r/tests" && cd "$T/r" && git init -q . || exit 1
seq 700 >big; seq 10 >small; seq 900 >tests/huge.test.sh; seq 900 >doc.md; git add . >/dev/null
printf '# 예산\n700 big\n' >"$T/b"
[ -z "$(check "$T/r" "$T/b")" ] && ok "예산 그대로·시험·문서 큰 파일은 통과" || bad "깨끗한 판이 걸림: $(check "$T/r" "$T/b")"
seq 701 >big
case $(check "$T/r" "$T/b") in *"늘었다 big 701줄"*) ok "예산보다 늘면 실패" ;; *) bad "늘어남을 못 잡음" ;; esac
seq 650 >big
[ -z "$(check "$T/r" "$T/b")" ] && ok "예산보다 줄면 통과" || bad "줄어듦이 걸림"
seq 601 >new; git add new
case $(check "$T/r" "$T/b") in *"새 거대 파일 new 601줄"*) ok "예산 없는 새 거대 파일은 실패" ;; *) bad "새 거대 파일을 못 잡음" ;; esac
git rm -q --cached new; printf '700 big\n10 gone\n' >"$T/b2"
case $(check "$T/r" "$T/b2") in *"예산에 없는 파일 gone"*) ok "사라진 파일의 예산 줄은 실패" ;; *) bad "남은 예산 줄을 못 잡음" ;; esac
printf '800 big\n' >"$T/b3"; printf '700 big\n' >"$T/o3"
case $(check "$T/r" "$T/b3" "$T/o3" lane) in *"레인이 예산을 늘렸다 big 800"*) ok "레인의 예산 올림은 실패" ;; *) bad "레인 예산 올림을 못 잡음" ;; esac
[ -z "$(check "$T/r" "$T/b3" "$T/o3")" ] && ok "레인 밖(지휘·사람)은 예산을 올릴 수 있다" || bad "레인 밖 예산 올림이 걸림"
printf '650 big\n' >"$T/b4"
[ -z "$(check "$T/r" "$T/b4" "$T/o3" lane)" ] && ok "레인이 예산을 낮추는 것은 통과" || bad "예산 낮춤이 걸림"
printf '700 big\n600 x\n' >"$T/b5"
case $(check "$T/r" "$T/b5" "$T/o3" lane) in *"레인이 예산을 늘렸다 x 600 (갈래점 없음)"*) ok "레인이 새 예산 줄을 더하면 실패" ;; *) bad "새 예산 줄을 못 잡음" ;; esac

# 이 저장소
R=$(cd "$here/.." && pwd)
if ! git -C "$R" rev-parse -q --verify HEAD >/dev/null 2>&1; then echo "ok - 건너뜀 저장소 판정(git 저장소 밖 사본)"; exit $fail; fi
[ -f "$R/$B" ] || bad "예산 파일이 없다: $B"
ob=
if [ -n "${HC_LANE-}" ] && mb=$(git -C "$R" merge-base HEAD origin/main 2>/dev/null) &&
  git -C "$R" cat-file -e "$mb:$B" 2>/dev/null; then
  ob=$T/base-budget; git -C "$R" show "$mb:$B" >"$ob"
fi
out=$(check "$R" "$R/$B" "$ob" ${ob:+lane})
if [ -z "$out" ]; then ok "이 저장소: 한도 ${LIMIT}줄·예산 $B 지킴${ob:+ (레인: 갈래점 예산과도 견줌)}"
else printf '%s\n' "$out" | sed 's/^/FAIL - /'; fail=1; fi
exit $fail
