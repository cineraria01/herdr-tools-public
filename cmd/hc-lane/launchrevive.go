package main

// 이어 띄우기 --revive(2026-11-28 ht-lane-revive — 업계의 「같은 이름·같은 디스크로 다시 붙이기」: 일(작업 트리·가지·임대)과 실행(터미널)을
// 나눠, 죽은 실행만 새로 띄워 같은 작업 폴더에 붙인다). 작업 트리 만들기·설정 훅·실어 온 자료 풀기를 건너뛰고 작업 트리 파일에 손대지
// 않는다. 차례는 launchMain(reviveGate → 보통 띄우기의 나머지), 문서 docs/hc-lane/revive.md

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

var (
	reviveLease string // 이어 쓸 임대 값(원격 없는 저장소면 빈 값 — 보통 띄우기와 같다)
	revivePhys  string // 대상 기계에서 작업 트리의 실제 경로(pwd -P — Claude가 대화 기록 폴더 이름을 이것으로 만든다)
	reviveSess  string // 이어 갈 레인 세션 id — 레인 환경 HC_LANE_CONTINUE=--resume=<id>(launchMain이 사본을 둔 뒤 reviveSession으로)
)

// reviveGate: 이어 띄워도 되는지 아무것도 쓰기 전에 본다 — 작업 트리·가지(아니면 11), 그 작업 트리를 쓰는 살아 있는 터미널(있으면 10,
// 목록을 모르면 1), 임대(주인이 다르면 10, 없으면 11)
func reviveGate() {
	reviveTree()
	ps, r := termPairs()
	if r != 0 {
		die("--revive: %s — 작업 트리 %s%s 를 쓰는 터미널을 모른다. 띄우지 않았다", termWhy(r), wt, onNote())
	}
	if busy := busyTerms(ps, false); len(busy) > 0 {
		errf("hc-lane: 이미 돎 — 살아 있는 터미널이 작업 트리 %s%s 를 쓴다(%s). 띄우지 않았다\n", wt, onNote(), strings.Join(busy, ", "))
		exit(10)
	}
	reviveLeaseGate()
}

// reviveHeadSh: 대상 기계에서 그 폴더가 작업 트리의 맨 위인지와 그 가지. 진행 중인 rebase는 HEAD가 떨어져 있어 git이 가지를 어느 작업
// 트리에도 걸지 않는다(2026-11-28 실측, git 2.56) — rebase의 head-name에서 읽는다
const reviveHeadSh = `cd "$1" 2>/dev/null || { echo nodir; exit 0; }
t=$(git rev-parse --show-toplevel 2>/dev/null) && [ "$t" = "$(pwd -P)" ] || { echo notwt; exit 0; }
b=$(git symbolic-ref -q HEAD) || for d in rebase-merge rebase-apply; do
  f=$(git rev-parse --git-path "$d/head-name") && [ -f "$f" ] && b=$(cat "$f") && break
done
printf 'head %s\ndir %s\n' "$b" "$t"`

// reviveTree: 작업 트리가 있고 그 가지가 레인 가지여야 한다(아니면 11 — 다시 해도 같다). 실제 경로는 전역 revivePhys
func reviveTree() {
	o, r := xoutQ("sh", "-c", reviveHeadSh, "hc-lane", wt)
	o, d, _ := strings.Cut(strings.TrimSpace(o), "\n")
	revivePhys = strings.TrimPrefix(d, "dir ")
	switch {
	case r != 0:
		die("--revive: 작업 트리 %s%s 를 확인하지 못했다(종료 %d) — 띄우지 않았다", wt, onNote(), r)
	case o == "nodir":
		refuse("--revive: 작업 트리가 없다: %s%s — 이어 띄울 것이 없다(새로 띄우려면 --revive 없이)", wt, onNote())
	case o == "notwt":
		refuse("--revive: %s%s 는 git 작업 트리(맨 위)가 아니다", wt, onNote())
	case o != "head refs/heads/"+branch:
		now := strings.TrimPrefix(strings.TrimPrefix(o, "head"), " refs/heads/")
		if strings.TrimSpace(now) == "" {
			now = "떨어진 HEAD"
		}
		refuse("--revive: 작업 트리 %s%s 의 가지가 %s 가 아니다(지금 %s)", wt, onNote(), branch, now)
	}
}

// reviveLeaseGate: 임대는 새로 잡지 않는다 — 지금 값의 주인이 같은 레인·같은 기계일 때만 그 값을 이어 쓴다(전역 reviveLease)
func reviveLeaseGate() {
	leaseBranch = branch
	lo, r := leaseX("show", name, "")
	switch {
	case r == 4 && lo == "no-origin":
		return
	case r != 0:
		die("--revive: 임대 refs/hc-lanes/%s 를 읽지 못했다(종료 %d) — 띄우지 않았다", name, r)
	case lo == "-":
		refuse("--revive: 임대 refs/hc-lanes/%s 가 없다 — 착지·clean으로 끝났거나 손으로 놓은 레인이다(이어 띄우지 않는다)", name)
	}
	sha, sub, _ := strings.Cut(lo, " ")
	if !sha40.MatchString(sha) || sub == "?" {
		die("--revive: 임대 refs/hc-lanes/%s 의 값을 읽지 못했다(%q) — 띄우지 않았다", name, cut(lo, 80))
	}
	if who := leaseWho(); !leaseOwnerOK(sub, name, who) {
		errf("hc-lane: 이미 돎 — 임대 refs/hc-lanes/%s 의 주인이 이 레인·기계(%s %s)가 아니다(%s). 띄우지 않았다\n", name, name, who, sub)
		exit(10)
	}
	reviveLease = sha
}

// leaseOwnerOK: 임대 제목 「hc-lane lease <레인> <기계> <UTC>」의 레인·기계가 같은가
func leaseOwnerOK(sub, lane, who string) bool {
	f := strings.Fields(sub)
	return len(f) == 5 && f[0] == "hc-lane" && f[1] == "lease" && f[2] == lane && f[3] == who
}

// reviveSessSh: 그 폴더의 대화 기록(<설정 폴더>/projects/<폴더 이름>/*.jsonl)을 새것부터 보고, 첫 사용자 줄의 글이 $2(이 레인 지시문 사본
// 경로, JSON 글 꼴)로 시작하는 첫 세션 id. 같은 폴더에는 같은 기계에서 뜬 감사(--read) 대화도 있어 「가장 새것」(--continue)이 아니라
// 레인 자신의 것을 고른다(감사 atp67l6 중간 1). 이어 간 세션도 첫 글이 그대로다(2026-11-28 실측 — --continue는 앞 대화를 복사한 새
// 파일, --resume은 같은 파일에 덧붙임)
const reviveSessSh = `d=${CLAUDE_CONFIG_DIR:-$HOME/.claude}/projects/$1
[ -d "$d" ] || exit 0
ls -t "$d" | while IFS= read -r f; do
  case $f in *.jsonl) ;; *) continue ;; esac
  if grep -m1 '"type":"user"' "$d/$f" | grep -qF -- "\"content\":\"$2"; then printf '%s\n' "${f%.jsonl}"; break; fi
done`

// reviveSession: 대상 기계에서 이어 갈 레인 세션 id(없으면 빈 값 — 같은 지시문으로 새로). 모르면 띄우지 않는다. runBrief가 정해진 뒤에 부른다.
// shortcut: Claude는 아주 긴 폴더 이름(약 200자 넘음)을 잘라 해시를 붙인다 — 그 꼴은 찾지 못해 새로 시작한다(레인 경로가 그만큼 길어지면 고친다)
func reviveSession() string {
	p := revivePhys
	if p == "" {
		p = wt
	}
	var jb strings.Builder // Claude(JSON.stringify)처럼 <>&는 그대로
	enc := json.NewEncoder(&jb)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(runBrief); err != nil {
		die("--revive: 지시문 사본 경로를 JSON 글로 바꾸지 못했다: %v", err)
	}
	jr := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(jb.String()), `"`), `"`)
	o, r := xoutQ("sh", "-c", reviveSessSh, "hc-lane", claudeProj(p), jr)
	if r != 0 {
		die("--revive: 대화 기록을 확인하지 못했다(종료 %d) — 띄우지 않았다", r)
	}
	return strings.TrimSpace(o)
}

// claudeProj: Claude가 폴더 p의 대화 기록을 두는 projects 아래 이름 — 영숫자 밖의 글자마다 「-」(UTF-16 둘로 적는 글자는 둘, 2026-11-28 실측)
func claudeProj(p string) string {
	var b strings.Builder
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r > 0xFFFF:
			b.WriteString("--")
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// reviveSteps: 지시문 사본 끝에 붙는 「재시동 뒤 이어 가기」 절
func reviveSteps(w io.Writer) {
	fmt.Fprint(w, "### 재시동 뒤 이어 가기 (hc-lane --revive가 채움)\n")
	fmt.Fprint(w, "- 앞 실행(터미널)이 죽은 뒤 같은 작업 트리·가지·임대로 다시 떴다. 일을 잇기 전에 차례대로:\n")
	fmt.Fprint(w, "1. git status와 진행 중인 rebase·merge(git status 첫 줄)를 먼저 본다 — 진행 중이면 그것부터 마저 푼다\n")
	fmt.Fprint(w, "2. 반쯤 쓴 파일을 확인한다(git diff · git diff --cached · 추적 안 된 파일). 되돌리거나 stash·reset·checkout·clean 하지 않는다\n")
	fmt.Fprintf(w, "3. 이 레인의 마지막 보고(hc-lane events --lane %s 의 끝 줄)가 「완료」이고 그 sha가 지금 HEAD와 같으면 다시 보고하지 않고 턴을 끝내 판정을 기다린다 — 아니면 위 지시문대로 이어서 일한다\n", name)
}
