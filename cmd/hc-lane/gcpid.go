package main

// gc의 「들어감」 판정(gin)과 기준 쪽 patch-id 캐시(gc-patchid-once, 2026-11-24 — gc.go에서 떼어 냄)

import (
	"os"
	"strconv"
	"strings"

	"herdr-tools/internal/proc"
)

// pidCache: 기준 쪽 커밋 → patch-id 캐시(그 클론의 공통 git 폴더 아래, 줄 「<patch-id> <커밋>」). 커밋은 바뀌지 않으니 그 패치 id도
// 그대로 — 참조마다·매시간 기준 쪽 수천 커밋을 다시 계산하지 않는다. 캐시는 계산을 줄일 뿐 판정 근거가 아니다:
// 꼴이 틀린 줄은 버리고, 캐시로 맞춘 「들어감」은 그 기준 커밋을 새로 계산해 확인한다(ginPid). 못 읽거나 못 쓰면 지금처럼 계산
const pidCache = "hc-lane-patch-id"

// pidLine: 캐시의 성한 줄(awk 조건) — 16진 두 칸, 같은 길이(SHA-1 40·SHA-256 64)
const pidLine = `NF==2 && $1 ~ /^[0-9a-f]+$/ && $2 ~ /^[0-9a-f]+$/ && (length($1)==40 || length($1)==64) && length($2)==length($1)`

// ginScript: 비조상·병합 커밋 없는 가지의 고유 커밋(C)과 패치 id — B 가지 쪽(늘 새로), U 기준 쪽 새로 계산, K 기준 쪽 캐시에서.
// $4가 -면 캐시를 읽지 않고 새로 계산한 것으로 갈아 쓴다. git 명령마다 종료값을 본다 — 파이프가 앞 명령의 실패를 삼키지 않게
// 임시 파일을 거친다. 캐시 쓰기 실패는 판정과 무관(넘어감). 고유 커밋이 없으면(비조상과 모순) 확인 못 함 2(R-09)
const ginScript = `cd "$1" || exit 2; c=$(git rev-list --no-merges "$2..$3") || exit 2; [ -n "$c" ] || exit 2
    u=$(git rev-list --no-merges "$3..$2") || exit 2
    t=$(mktemp "${TMPDIR:-/tmp}/hc-lane-gin.XXXXXX") || exit 2
    trap 'rm -f "$t" "$t".?' EXIT
    k=$(git rev-parse --path-format=absolute --git-common-dir 2>/dev/null) && [ -d "$k" ] && k=$k/` + pidCache + ` || k=
    : >"$t.k" || exit 2
    if [ -n "$k" ] && [ "$4" != - ] && [ -f "$k" ]; then awk '` + pidLine + `' "$k" >"$t.k" 2>/dev/null || : >"$t.k" || exit 2; fi
    printf "%s\n" "$u" >"$t.u" || exit 2; printf "%s\n" "$c" >"$t.c" || exit 2
    awk 'FILENAME==ARGV[1]{h[$2]=1; next} $0!="" && !($1 in h)' "$t.k" "$t.u" >"$t.m" || exit 2
    : >"$t.p" || exit 2
    if [ -s "$t.m" ]; then
      git log -p --no-merges --no-color --no-ext-diff --no-walk=unsorted --stdin <"$t.m" >"$t" || exit 2
      git patch-id --verbatim <"$t" >"$t.p" || exit 2
      if [ -n "$k" ]; then w=$(mktemp "$k.XXXXXX" 2>/dev/null) && { cat "$t.k" "$t.p" >"$w" && mv -f "$w" "$k" || rm -f "$w"; } 2>/dev/null; fi
    fi
    sed "s/^/U /" "$t.p" || exit 2
    awk 'FILENAME==ARGV[1]{h[$1]=1; next} ($2 in h){print "K " $0}' "$t.u" "$t.k" || exit 2
    git log -p --no-merges --no-color --no-ext-diff --no-walk=unsorted --stdin <"$t.c" >"$t" || exit 2
    git patch-id --verbatim <"$t" >"$t.b" || exit 2
    sed "s/^/B /" "$t.b" || exit 2
    sed "/^$/d; s/^/C /" "$t.c"`

// pidScript: 주어진 커밋들의 patch-id를 새로(캐시 없이) — 캐시로 맞춘 「들어감」 확인용
const pidScript = `cd "$1" || exit 2; shift; t=$(mktemp "${TMPDIR:-/tmp}/hc-lane-pid.XXXXXX") || exit 2; trap 'rm -f "$t"' EXIT
    git log -p --no-merges --no-color --no-ext-diff --no-walk=unsorted "$@" >"$t" || exit 2
    git patch-id --verbatim <"$t"`

// gcPruneScript: 캐시에서 기준($2)에 없는 커밋(되감김 등)과 깨진 줄을 걷는다. 무엇이 실패해도 그대로 둔다(성능만의 일)
const gcPruneScript = `cd "$1" || exit 0; k=$(git rev-parse --path-format=absolute --git-common-dir 2>/dev/null)/` + pidCache + ` || exit 0
    [ -f "$k" ] || exit 0; r=$(mktemp "${TMPDIR:-/tmp}/hc-lane-gcp.XXXXXX") || exit 0; trap 'rm -f "$r"' EXIT
    git rev-list --no-merges "$2" >"$r" || exit 0; w=$(mktemp "$k.XXXXXX") || exit 0
    { awk 'FILENAME==ARGV[1]{h[$1]=1; next} (` + pidLine + `) && ($2 in h)' "$r" "$k" >"$w" && mv -f "$w" "$k"; } || rm -f "$w"`

// gitSh: sh -c 조각을 대상 기계에서. sh -c 안의 git은 proc.Limit 표에 안 보인다(sh는 상한 없음) — 조각 전체에 로컬 git 상한을
// 건다. 넘으면 무리째(안쪽 git까지) 거두고 0이 아닌 종료(lane/R-217). hc exec로 가면 그 상한(proc.Limit)이 그대로 덮는다
func gitSh(script string, args ...string) (string, int) {
	argv := xargv(append([]string{"sh", "-c", script, "hc-lane"}, args...))
	lim := proc.Limit(argv)
	if lim == 0 {
		lim = proc.Limit([]string{"git"})
	}
	sc := command(argv)
	sc.Stderr = os.Stderr
	ob, err := proc.OutputMax(sc, lim)
	return trim(string(ob)), code(err)
}

// gcPrune: 한 번 도는 끝에 캐시 정리(결과는 판정과 무관)
func gcPrune() { gitSh(gcPruneScript, repo, bref) }

// gin: 그 커밋이 기준에 다 들어갔나. 0 들어감(조상, 또는 병합 커밋 없이 커밋마다 공백까지 같은 패치가 있음), 1 아님(안 들어간 수), 2 확인 못 함
func gin(c string) (string, int) {
	switch xrun(nil, os.Stderr, "git", "-C", repo, "merge-base", "--is-ancestor", c, bref) {
	case 0:
		return "0", 0
	case 1:
	default:
		return "0", 2
	}
	gm, r := xout("git", "-C", repo, "rev-list", "--merges", bref+".."+c)
	if r != 0 {
		return "0", 2
	}
	if gm != "" {
		n, r := xout("git", "-C", repo, "rev-list", "--count", bref+".."+c)
		if r != 0 {
			n = "?"
		}
		return n, 1
	}
	return ginPid(c, "")
}

// ginPid: 패치 id 견주기. mode "-"는 캐시 없이(캐시로 맞춘 「들어감」이 확인에서 어긋났을 때). 상한·실패는 확인 못 함 2 — 후보는 남긴다
func ginPid(c, mode string) (string, int) {
	out, r := gitSh(ginScript, repo, bref, c, mode)
	if r != 0 {
		return "0", 2
	}
	u, b, kc := map[string]bool{}, map[string]string{}, map[string]string{}
	var cs []string
	for _, ln := range strings.Split(out, "\n") {
		f := strings.Fields(ln)
		switch {
		case len(f) >= 2 && f[0] == "U":
			u[f[1]] = true
		case len(f) >= 3 && f[0] == "K":
			kc[f[1]] = f[2]
		case len(f) >= 3 && f[0] == "B":
			b[f[2]] = f[1]
		case len(f) >= 2 && f[0] == "C":
			cs = append(cs, f[1])
		}
	}
	if len(cs) == 0 { // 비조상인데 고유 커밋을 하나도 못 읽음 — 「모두 들어감」이 아니다(R-09)
		return "0", 2
	}
	m := 0
	want := map[string]string{} // 캐시로만 맞춘 기준 커밋 → 그 patch-id
	var chk []string
	for _, x := range cs {
		p, ok := b[x]
		switch {
		case !ok:
			m++
		case u[p]:
		case kc[p] != "":
			if _, d := want[kc[p]]; !d {
				want[kc[p]] = p
				chk = append(chk, kc[p])
			}
		default:
			m++
		}
	}
	if m > 0 {
		return strconv.Itoa(m), 1
	}
	if len(chk) > 0 { // 「들어감」은 지울 근거 — 캐시로 맞춘 기준 커밋을 새로 계산해 같은지 본다
		po, r := gitSh(pidScript, append([]string{repo}, chk...)...)
		if r != 0 {
			return "0", 2
		}
		got := map[string]string{}
		for _, ln := range strings.Split(po, "\n") {
			if f := strings.Fields(ln); len(f) == 2 {
				got[f[1]] = f[0]
			}
		}
		for s, p := range want {
			if got[s] != p { // 캐시가 틀렸다 — 캐시 없이 다시(K 줄이 없어 여기로 돌아오지 않는다, 새로 계산한 것으로 캐시를 갈아 씀)
				return ginPid(c, "-")
			}
		}
	}
	return "0", 0
}
