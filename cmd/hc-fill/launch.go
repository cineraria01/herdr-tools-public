package main

// 띄우기 앞뒤 판정 — 셸 판 lib/hc-fill/launch.sh 앞부분(fetch_proj … afin_sec). 함수 차례도 셸 판과 같다

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

var sinceIDRe = regexp.MustCompile(`^r[0-9]+-[0-9]+-[0-9]+$`) // 줄 ID r<초>-<pid>-<난수>

// lcCut: cut -f <i> 한 줄 — 탭이 없는 줄은 통째로(cut 기본), 칸이 모자라면 빈 값
func lcCut(line string, i int) string {
	if !strings.Contains(line, "\t") {
		return line
	}
	return splitRow(line).F(i)
}

// lcE: [ -e <경로> ] (심볼릭 링크는 따라감)
func lcE(path string) bool { _, err := os.Stat(path); return err == nil }

// fetch_proj <프로젝트>: FR(local 저장소)·FB(기본 가지)를 둔다. 0 됨, 1 설정에 local repo 줄 없음, 2 fetch 실패
func fetchProj(p string) int { return lcFetch(p, false) }

// lcFetch: fetch_proj — quiet면 dry-run 알림 줄을 찍지 않는다(셸 판 fetch_proj … >/dev/null)
func lcFetch(p string, quiet bool) int {
	FR = repoOf(p, LOCAL)
	if FR == "" {
		return 1
	}
	ref, ok := projectRef(FR)
	FB = ref
	if !ok {
		return 2
	}
	if strings.HasPrefix(FB, "refs/heads/") {
		return 0
	}
	if !inlist(p, FETCHED) {
		FETCHED += "," + p
		if dry {
			if !quiet {
				say("%sfetch 건너뜀: 지금 있는 origin 참조로 판정 (%s)", P, FR)
			}
		} else if _, rc := (Cmd{Name: "git", Args: []string{"-C", FR, "fetch", "-q", "origin"}, Err: errPass}).Run(); rc != 0 {
			FETCHBAD += "," + p
		}
	}
	if inlist(p, FETCHBAD) {
		return 2
	}
	return 0
}

// since <선행>: 줄 ID에 기록한 접수 초. ID가 없는 옛 줄만 10칸의 분 시각(이 기계 현지 시각)으로 복원한다
func since(lane string) (string, bool) {
	sa := ""
	for _, r := range qRows() {
		if r.F(1) != lane {
			continue
		}
		if x := r.F(17); sinceIDRe.MatchString(x) {
			sa = strings.SplitN(x[1:], "-", 2)[0]
		} else {
			sa = r.F(10)
		}
		break
	}
	if isDigits(sa) {
		return sa, true
	}
	if sa == "" || sa == "-" {
		return "", false
	}
	t, err := time.ParseInLocation("2006-01-02T15:04:05", sa+":00", time.Local)
	if err != nil {
		return "", false
	}
	return fmt.Sprint(t.Unix()), true
}

// lmark <선행> <접수 초>: 기준 가지에 든 표식은 0(LMS), 없으면 1, 조회 실패는 2
func lmark(lane, sinceS string) int {
	LMS, LMFROM = "", sinceS
	if !strings.HasPrefix(FB, "origin/") {
		return 1
	}
	refs, rc := run("git", "-C", FR, "ls-remote", "origin", "refs/hc-landed/"+lane+"/*")
	if rc != 0 {
		return 2
	}
	pre := "refs/hc-landed/" + lane + "/"
	t := num(sinceS)
	missing := 1
	for _, l := range lines(refs) {
		f := fields(l)
		if len(f) < 2 || !strings.HasPrefix(f[1], pre) {
			continue
		}
		s := f[1][len(pre):]
		if !isDigits(s) || num(s) < t {
			continue
		}
		_, rc := run("git", "-C", FR, "merge-base", "--is-ancestor", f[0], FB)
		switch rc {
		case 0:
			LMS = f[0]
			return 0
		case 1:
		default:
			missing = 2
		}
	}
	return missing
}

// cached_mark <프로젝트> <선행>: ls는 원격을 읽지 않고 마지막 바퀴에서 확인한 표식만 로컬 기본 가지와 다시 견준다
func cachedMark(p, lane string) bool {
	f := DATA + "/after-landed.tsv"
	if !nonEmpty(f) {
		return false
	}
	stamp, ok := since(lane)
	if !ok {
		return false
	}
	sha := ""
	for _, r := range readRows(f) {
		if r.F(1) == p && r.F(2) == lane && r.F(3) == stamp {
			sha = r.F(4)
			break
		}
	}
	if sha == "" {
		return false
	}
	_, rc := run("git", "-C", FR, "merge-base", "--is-ancestor", sha, FB)
	return rc == 0
}

// landing_hit <선행>: 선행의 제목이나 표식에서 확인한 착지(hit). 0 들어감 · 1 아직 · 2 조회 오류
func landingHit(lane string) int {
	h, ok := landed(lane)
	hit = h
	if !ok {
		return 2
	}
	if hit != "" {
		return 0
	}
	se, ok := since(lane)
	if !ok {
		return 1
	}
	if rc := lmark(lane, se); rc != 0 {
		return rc
	}
	hit = head(LMS, 8) + " — 착지 표식"
	return 0
}

// afterAt <선행 항목> <기다리는 줄 프로젝트>: 선행 이름과 착지를 찾을 프로젝트 — <이름>@<프로젝트>(--after-any-project)면 그 프로젝트.
// 일반 배치(afterOK)·먼저 띄움 판정(afterTick)·ls·실행 지시문(earlySec)이 모두 이것으로 푼다 — 먼저 띄움 쪽이 「spec@tools」를
// 통째로 이 프로젝트에서 찾아 착지한 선행을 「대기열에 없음」 stop으로 봤다(재감사2 fill/R-217)
func afterAt(a, p string) (string, string) {
	if n, op, ok := strings.Cut(a, "@"); ok {
		return n, op
	}
	return a, p
}

// after_ok <레인> <프로젝트> <쉼표 목록>: 0 다 들어감, 1 아직(기본 가지는 bBranch), 2 판정 못 함
func afterOK(lane, p, list string) int {
	switch fetchProj(p) {
	case 1:
		say("선행 %s: 판정 못 함(설정에 %s 의 %s repo 줄이 없다)", lane, p, LOCAL)
		return 2
	case 2:
		say("선행 %s: 판정 못 함(git fetch 실패: %s)", lane, FR)
		return 2
	}
	bBranch = FB
	ok := 0
	for _, a := range words(list) {
		// 들어감 = 커밋 제목 머리 규칙 또는 그 선행 줄을 넣은 뒤의 착지 표식이 origin/<기본>의 조상(대기열에 그 줄이 있을 때)
		// <이름>@<프로젝트>(--after-any-project)는 그 프로젝트 저장소 기본 가지에서 찾는다(fill-edit-after)
		n, op := afterAt(a, p)
		at := op != p
		if at && fetchProj(op) != 0 {
			say("선행 %s ← %s: 판정 못 함(%s 저장소 설정·fetch)", lane, a, op)
			fetchProj(p)
			return 2
		}
		lh := landingHit(n)
		if at {
			fetchProj(p) // FR·FB를 이 프로젝트로 되돌림(fetch는 바퀴에 한 번)
		}
		switch lh {
		case 0:
			say("선행 %s ← %s: 들어감 (%s)", lane, a, hit)
		case 1:
			say("선행 %s ← %s: 아직 (%s)", lane, a, bBranch)
			ok = 1
		default:
			say("선행 %s ← %s: 판정 못 함(커밋·착지 표식 조회 실패)", lane, a)
			return 2
		}
	}
	return ok
}

// lane_landed <레인 이름> <cwd>: cwd의 <저장소>.wt/… 앞 폴더 이름과 repo 줄 경로의 이름이 같은 프로젝트에서 착지했으면 참(LHIT=커밋 줄).
// 이름이 레인 꼴이 아니거나 프로젝트를 못 찾거나 fetch 실패면 거짓(판정 못 함 = 셈)
func laneLanded(name, cwd string) bool {
	LHIT = ""
	if !okname(name) {
		return false
	}
	lp := cwd
	if i := strings.Index(lp, ".wt/"); i >= 0 {
		lp = lp[:i]
	}
	lp = lp[strings.LastIndexByte(lp, '/')+1:]
	pj := ""
	for _, l := range confLines {
		f := fields(l)
		if len(f) == 0 || f[0] != "repo" {
			continue
		}
		x := strings.TrimRight(restAfter(l, 3), "/")
		x = x[strings.LastIndexByte(x, '/')+1:]
		if x == lp {
			pj = F1(f, 1)
			break
		}
	}
	if pj == "" || fetchProj(pj) != 0 {
		return false
	}
	h, ok := landed(name)
	LHIT = h
	return ok && LHIT != ""
}

// submitted <레인 이름> <주소>: 그 터미널이 이번 실행에 그 이름으로 적은 마지막 사건이 「제출」이면 참.
// 사건을 못 읽었거나(EVBAD) 그 기계 사건이 없으면 거짓(지금처럼 판정)
func submitted(name, addr string) bool {
	return !EVBAD && lcCut(joblast(name, addr, ""), 5) == "제출"
}

// wt_there <기계> <폴더> → yes|no|? (확인 못 하면 ?)
func wtThere(m, dir string) string {
	if m == LOCAL {
		if isDir(dir) {
			return "yes"
		}
		return "no"
	}
	if !connHas(m) {
		return "?"
	}
	v, rc := run("hc", "exec", "--on", m, "--", "sh", "-c", `[ -d "$1" ] && echo yes || echo no`, "_", dir)
	if rc != 0 { // 실패·시간 넘김의 출력은 믿지 않는다 — 모름(자리를 계속 센다, gofix-fill E05)
		return "?"
	}
	switch v = sub(v); v {
	case "yes", "no":
		return v
	}
	return "?"
}

func agentsOn(m string) int { return cnt(SEATS, m) + cnt(NOW, m) }

// lanes_on: hc-fill이 그 기계에 띄워 살아 있는 레인 수. 자동 감사 줄은 빼고 센다. 감사를 기다리는 쓰기 레인은 센다
func lanesOn(m string) int {
	k := 0
	for _, r := range ROWS {
		if r.F(6) != "up" || r.F(7) != m || strings.HasPrefix(r.F(15), "audit=") || r.F(8) == "" {
			continue
		}
		for _, t := range TERMS { // 주소+이름(주소 재사용), 끝난 keep 셸 레인은 빼고
			if t.Addr == r.F(8) && t.Name == r.F(1) {
				if !laneEnded(t) {
					k++
				}
				break
			}
		}
	}
	return k + cnt(NOWL, m)
}

// laneEnded: hc keep이 셸로 바꾼(전경이 셸) 레인 터미널인데 그 레인이 끝났나 — 이 기계 작업 트리가 없어졌거나, 이 실행(try.tsv
// 실행 키)의 마지막 사건(정리됨 빼고)이 제출·착지. 띄운 직후(작업 트리 있고 사건 없음)·모름은 false(자리로 센다). 2026-11-21 mini-m4
// land-claim-net이 착지·정리 뒤 keep 셸로 13시간 자리를 붙잡았다
func laneEnded(t Term) bool {
	switch strings.TrimPrefix(t.Agent, "-") {
	case "zsh", "bash", "sh", "fish", "dash", "ksh":
	default:
		return false
	}
	if t.M == LOCAL && strings.HasPrefix(t.Cwd, "/") {
		if _, err := os.Stat(t.Cwd); os.IsNotExist(err) {
			return true
		}
	}
	key := attemptRun(t.Name, t.Addr)
	if EVBAD || key == "" {
		return false
	}
	last := ""
	for _, e := range EV {
		r := splitRow(e)
		if r.F(3) == t.Addr && r.F(4) == t.Name && r.F(5) != "정리됨" && r.NF() >= 10 && r.F(10) == key {
			last = r.F(5)
		}
	}
	return last == "제출" || last == "착지"
}

// tryput: 시도 기록(이름·줄 ID·기계·먼저·감사·확정 주소·실행). 대기열에서 빠져도 그 터미널이 끝날 때까지 보존한다. 0 됨
func tryput(name, rid, m, e, u, addr, r string) int {
	if dry {
		return 0
	}
	file := DATA + "/try.tsv"
	var b strings.Builder
	if exists(file) {
		text, ok := readFile(file)
		if !ok {
			return 1
		}
		for _, l := range lines(text) {
			if splitRow(l).F(1) != name {
				b.WriteString(l + "\n")
			}
		}
	}
	b.WriteString(strings.Join([]string{name, rid, m, dash(e), dash(u), dash(addr), dash(r)}, "\t") + "\n")
	if err := writeAtomic(file, b.String()); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		return 1
	}
	return 0
}

// try_old_busy: 줄을 교체했어도 이전 시도의 기계가 끊겼거나 그 이름의 터미널이 아직 있으면 기다린다(참).
// 이전 시도가 미확정(주소 ?)이면 주소를 모르니 그 기계의 같은 이름 터미널은 다 그 시도일 수 있다(재감사 R-03 — 새 줄이 채택하지 않게)
func tryOldBusy(name, rid string) bool {
	machine, addr := "", ""
	for _, r := range readRows(DATA + "/try.tsv") {
		if r.F(1) == name && r.F(2) != rid {
			machine, addr = r.F(3), r.F(6)
			break
		}
	}
	if machine == "" {
		return false
	}
	if machine != LOCAL && !connHas(machine) {
		return true
	}
	for _, t := range TERMS {
		if t.M == machine && t.Name == name && (addr == "" || addr == "-" || addr == "?" || t.Addr == addr) {
			return true
		}
	}
	return false
}

// tryUnsure: 같은 줄 ID의 미확정 시도(주소 칸 ? — 띄우기 시간 넘김·주소 못 읽음·띄운 직후 죽음)가 있으면 그 기계가 끊겼거나
// 그 이름의 터미널이 보이는 동안 기다린다(참 — 다른 기계에 또 띄우지 않는다, gofix-fill F01·E02). 연결된 기계에 그 이름 터미널이
// 없으면 안 떴다 — 기록을 확정 실패(-)로 고치고 거짓
func tryUnsure(name, rid string) bool {
	var r Row
	for _, x := range readRows(DATA + "/try.tsv") {
		if x.F(1) == name && x.F(2) == rid && x.F(6) == "?" {
			r = x
			break
		}
	}
	if r == nil {
		return false
	}
	m := r.F(3)
	if m != LOCAL && !connHas(m) {
		say("미확정 시도 기다림 %s @%s: 연결 안 됨 — 떴는지 몰라 다른 기계에 띄우지 않음", name, m)
		return true
	}
	for _, t := range TERMS {
		if t.M == m && t.Name == name {
			say("미확정 시도 기다림 %s @%s: 같은 이름 터미널 %s 가 있다(채택하지 못함 — 지휘가 정리)", name, m, t.Addr)
			return true
		}
	}
	if !dry && tryput(name, rid, m, r.F(4), r.F(5), "", "") != 0 {
		return true
	}
	return false
}

// try_prune: 대기열에 없고, 연결된 기계에서 그 이름 터미널도 없는 시도 기록을 지운다
func tryPrune() {
	file := DATA + "/try.tsv"
	if dry || !nonEmpty(file) {
		return
	}
	qtext, ok := readFile(Q)
	if !ok {
		return // 셸 판: awk가 대기열을 못 열면 그대로 둔다
	}
	text, ok := readFile(file)
	if !ok {
		return
	}
	mset := map[string]bool{LOCAL: true}
	for _, c := range CONN {
		mset[c] = true
	}
	seen := map[[2]string]bool{}
	for _, t := range TERMS {
		seen[[2]string{t.M, t.Name}] = true
	}
	if len(TERMS) == 0 {
		seen[[2]string{"", ""}] = true // printf '%s\n' "" 의 빈 줄
	}
	q := map[string]bool{}
	for _, l := range lines(qtext) {
		q[splitRow(l).F(1)] = true
	}
	var b strings.Builder
	for _, l := range lines(text) {
		r := splitRow(l)
		if q[r.F(1)] || !mset[r.F(3)] || seen[[2]string{r.F(3), r.F(1)}] {
			b.WriteString(l + "\n")
		}
	}
	if err := writeAtomic(file, b.String()); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
	}
}

// tryget: 그 이름·줄 ID·기계의 시도 기록 「먼저<TAB>감사」
func tryget(name, rid, m string) (string, bool) {
	for _, r := range readRows(DATA + "/try.tsv") {
		if r.F(1) == name && r.F(2) == rid && r.F(3) == m {
			return r.F(4) + "\t" + r.F(5), true
		}
	}
	return "", false
}

// park_candidate: 주차한 줄에서 채택할 수 있는 것은 같은 줄의 --resume 시도뿐이다. 주차를 보고한 터미널은 정리가 끝날 때까지 기다린다.
// 0 채택 · 1 건너뜀 · 2 기다림
func parkCandidate(name, rid, m, addr, st string) int {
	attempt, _ := tryget(name, rid, m)
	if strings.SplitN(attempt, "\t", 2)[0] == "resumed" {
		if EVBAD || has(EV, "모름 "+m) || pget(addr) != 0 {
			return 2
		}
		if runlast(name, addr, PRUN, "주차") == "" {
			return 0
		}
	}
	if st == "done" || st == "idle" {
		return 1
	}
	return 2
}

// inproj <프로젝트> <기계> <cwd>: cwd가 그 기계 그 프로젝트 저장소(또는 <저장소>.wt/ 아래)면 참
func inproj(p, m, cwd string) bool {
	ip := strings.TrimSuffix(repoOf(p, m), "/")
	if ip == "" {
		return false
	}
	return cwd == ip || strings.HasPrefix(cwd, ip+"/") || strings.HasPrefix(cwd, ip+".wt/")
}

// old_report <기계> <레인>: 같은 이름의 옛 보고가 새 레인 결과로 나가지 않게 .old-<시각>으로 비켜 둔다(지우지 않음)
func oldReport(m, lane string) int {
	_, rc := onm(nil, m, `[ ! -f "$d" ] || mv -f "$d" "$d.old-$1"`, "~/.local/share/hc-fill/reports/"+lane+".md", time.Now().Format("20060102150405"))
	return rc
}

// afin_sec: 설정 audit_final이 있으면 감사 절 3번을 「통과여도 착지하지 않는다 — 최종 감사 통과 알림 뒤에만」으로
func afinSec() {
	ASEC = strings.Replace(ASEC, "3. 출력이 「감사 통과 …」면 그 출력의 착지 줄",
		"3. 출력이 「감사 통과 …」여도 착지하지 않는다 — 최종 감사("+AFIN+")의 통과 알림을 기다린다. 그 전에는 hc-lane report도 하지 않는다. 최종 통과를 받으면 그 출력의 착지 줄", 1)
	ASEC = strings.Replace(ASEC, "4. 출력이 「감사 지적", "최종 감사가 지적하면 4번처럼 고친다.\n4. 출력이 「감사 지적", 1)
}
