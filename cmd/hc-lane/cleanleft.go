package main

// clean의 미등록 작업 폴더 검사 조각(gofix3-shx — clean.go 줄 수 한도로 따로 둔다)

import "herdr-tools/internal/shx"

// leftoverScript: 등록이 지워진 폴더에서 메인 폴더의 무시 규칙으로 무시되지 않는 것(추적 파일 포함)을 찍는다. 검사 오류는 종료 2.
// 열거(find)와 경로 변환(sed)을 나눠 각 종료 코드를 본다 — 파이프로 이으면 find 실패를 sed 성공이 가려 빈 목록(지워도 됨)이 됐다(lane/R-206).
// shx.Script로 감싸 끝 표식이 찍혔을 때만 목록을 믿는다
var leftoverScript = shx.Script(`
    cd "$2" || exit 2
    t=$(mktemp "${TMPDIR:-/tmp}/hc-lane-left.XXXXXX") || exit 2
    trap "rm -f \"\$t\" \"\$t.f\" \"\$t.i\"" EXIT
    if [ -f .git ] && [ ! -L .git ]; then
      g=$(sed -n "s/^gitdir: //p" .git)
      case $g in "$3"/worktrees/?*) [ ! -e "$g" ] || echo .git ;; *) echo .git ;; esac
    elif [ -e .git ] || [ -L .git ]; then echo .git
    fi
    find . -mindepth 1 -path ./.git -prune -o ! -type d -print >"$t.f" || exit 2
    sed "s|^\./||" "$t.f" >"$t" || exit 2
    git -c core.quotePath=false -C "$1" check-ignore --stdin <"$t" >"$t.i"; [ $? -le 1 ] || exit 2
    grep -vxF -f "$t.i" "$t"; [ $? -le 1 ] || exit 2`)

// leftover: leftoverScript를 돌려 무시되지 않는 것 목록. ok=false면 모름(지우지 않는다)
func leftover(repo, wt, common string) (string, bool) {
	out, r := xout("sh", "-c", leftoverScript, "hc-lane", repo, wt, common)
	left, ok := shx.Cut(out, r)
	return trim(left), ok
}
