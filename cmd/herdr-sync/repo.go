package main

// 맥 ↔ 저장소: --pull <기계>:…로 맥에 가져오기, 3-way 모음·깖, prebuild

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"herdr-tools/internal/ojson"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/toolcfg"
)

var (
	body     bool     // 저장소에서 지침 본문을 깔았다(prebuild·모음을 한 번 더)
	rdiff    []string // 맥 ↔ 저장소 다름 묶음
	rconf    int      // 맥 ↔ 저장소 충돌 수
	RBH, RAH []string // --check: 저장소가 새로움 · 맥이 새로움(저장소 경로)
	RSUF     string   // 맥이 저장소보다 뒤면 기계 줄 끝 표시
	rows     string
	rc       int
)

func readJSONFile(p string) (any, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return ojson.Parse(b)
}

func objGet(v any, k string) any {
	if o, ok := v.(*ojson.Object); ok {
		return o.M[k]
	}
	return nil
}

func strList(v any) []string {
	a, _ := v.([]any)
	var r []string
	for _, e := range a {
		if s, ok := e.(string); ok {
			r = append(r, s)
		}
	}
	return r
}

// repoSync: 맥 ↔ 저장소 3-way. 모두 정한 뒤, 충돌이 하나라도 있으면 쓰지 않는다. 저장소는 줄마다 UD(범용 또는 개인)다.
// 범용 저장소로 모을 맥 값에 개인 값이 있으면 충돌이다
func repoSync() {
	q, ok := query("")
	if !ok {
		die("맥 값을 읽지 못했다")
	}
	// RS: 저장소 쪽 서명. 나무는 줄마다 한 번에 걷는다
	rk := map[string]string{}
	for _, t := range TR {
		var b bytes.Buffer
		c := macCmd("", "t", t.d+"/"+t.r, t.k, t.x)
		c.Stdout = &b
		if run(c) != 0 {
			die("저장소 나무를 읽지 못했다: %s/%s", t.d, t.r)
		}
		for _, l := range splitLines(b.String()) {
			if a := strings.Split(l, "\t"); a[0] == "F" && len(a) > 2 {
				rk[a[1]] = a[2]
			}
		}
	}
	RS := make([]string, len(U))
	for i, u := range U {
		if u.tree {
			if s, ok := rk[u.k]; ok {
				RS[i] = s
			} else {
				RS[i] = "-"
			}
		} else {
			RS[i] = rsig(u.d+"/"+u.r, u.k)
		}
	}
	LK := tleaks(q, RS)
	// repo-base 셋째 칸 = 그 base를 적을 때의 저장소 경로. 줄을 옮겨 지금 경로와 다르면 base 없음으로 본다:
	// 새 자리에 저장소 파일이 없으면 「맥만 있음」 = 모음이 된다(맥 파일을 지우지 않는다). 옛 두 칸 줄은 그대로 믿는다
	rp := map[string]string{}
	if data, err := os.ReadFile(STATE + "/repo-base.tsv"); err == nil {
		for _, l := range splitLines(string(data)) {
			a := strings.Split(l, "\t")
			if len(a) >= 3 {
				rp[a[0]] = a[2]
			} else {
				delete(rp, a[0])
			}
		}
	}
	RP := make([]string, len(U))
	for i, u := range U {
		RP[i] = rp[u.k]
	}
	var rbp, rbp2 []string
	acts := map[int]string{}
	var idx []int
	shown := map[string]bool{}
	for i, u := range U {
		k, m, b := u.k, q.C[i], q.B[i]
		act, why := "", ""
		if RP[i] != "" && RP[i] != u.url {
			b = "-"
		}
		if strings.HasSuffix(k, "@") {
			continue
		}
		switch m {
		case "!":
			die("맥 ~/%s에 '<!-- User customizations -->' 줄이 없다", strings.SplitN(k, "#", 2)[0])
		case "~", "?": // 맥에 Codex 지침이나 그 표식이 없다
			continue
		case "L":
			if !shown[q.L[i]] {
				say("건너뜀 맥 %s (링크 — 따라가지 않음)", q.L[i])
				shown[q.L[i]] = true
			}
			continue
		case "G":
			say("건너뜀 %s (7MB 넘음)", k)
			continue
		}
		r := RS[i]
		if m == r {
			if !(b == m && RP[i] == u.url) {
				rbp = append(rbp, k+"\t"+m+"\t"+u.url)
			}
			continue
		}
		switch {
		case spec(takes, "repo", k):
			act = "모음"
		case spec(pulls, "repo", k):
			act = "깖"
		case r == b:
			act = "모음"
		case m == b:
			act = "깖"
		}
		if act == "깖" && r == "-" && strings.Contains(k, "#") { // 구역 파일이 저장소에서 지워졌으면 충돌로
			act = ""
		}
		// 옛 두 칸 base(경로를 모름)에서 저장소 쪽이 없으면, 정말 지운 것인지 줄을 옮긴 것인지 모르니 맥 파일을 지우지 않고 멈춘다
		if act == "깖" && r == "-" && RP[i] == "" && !spec(pulls, "repo", k) {
			act, why = "", "저장소에 없음 — 옛 base라 지운 것인지 모른다"
		}
		if act == "모음" && m != "-" {
			if u.tree {
				if LK[i] != "" {
					why, act = LK[i], ""
				}
			} else if l, w := leak(i); l {
				why, act = w, ""
			}
		}
		if act == "" {
			if why == "" {
				why = fmt.Sprintf("%d줄 다름", lines(mpath(i), u.d+"/"+u.r))
			}
			say("충돌 저장소 %s (%s)", k, why)
			rconf++
			continue
		}
		acts[i] = act
		idx = append(idx, i)
		rdiff = append(rdiff, u.b)
	}
	if check { // 깖 = 저장소가 새로움(RBH), 모음 = 맥이 새로움(RAH). 맥 줄과 기계 줄 끝 표시에 쓴다
		for _, i := range idx {
			if acts[i] == "깖" {
				RBH = append(RBH, U[i].r)
			} else {
				RAH = append(RAH, U[i].r)
			}
			if acts[i] == "깖" && RS[i] == "-" {
				say("저장소 다름:지움 %s", U[i].k)
			} else {
				say("저장소 다름:%s %s", acts[i], U[i].k)
			}
		}
		return
	}
	if rconf > 0 {
		row("맥", rdiff, rconf)
		fmt.Print(rows)
		fmt.Fprintln(os.Stderr, "herdr-sync: 저장소와 충돌이 있어 멈춘다. --take repo:<경로>(맥 값) 또는 --pull repo:<경로>(저장소 값)로 푼다. 개인 값이면 맥 파일에서 빼거나 그 줄을 개인 저장소로 옮긴다")
		exit(1)
	}
	// 모음을 먼저 커밋·push하고 base를 쓴 뒤에 깐다. 깔다 멈춰도 모은 것이 스테이징에만 남지 않는다.
	// 나무 파일은 나무마다 tar 한 묶음으로 옮기고(실행 권한 포함) git add·rm도 한 번씩 한다.
	// 쓰기·스테이징·커밋이 실패하면 그 저장소에서 이번에 한 것을 되돌리고(undo) 멈춘다. 「맥 모음」 줄은 그 저장소 커밋이 된 뒤에 찍는다
	st := map[string][]string{}
	mm := map[string][]string{}
	var ctool, cconf []string
	utool, uconf := true, true
	// 쓰기 전에 일지(journal.go) — 쓰기와 커밋 사이에 죽어도 다음 실행이 자기 것만 되돌린다. old = 쓰기 전 저장소 서명
	var jes []jentry
	for _, i := range idx {
		if acts[i] == "모음" {
			jes = append(jes, jentry{U[i].d, "", U[i].r, U[i].k, q.C[i], RS[i]})
		}
	}
	of := func(repo string) []jentry {
		var r []jentry
		for _, e := range jes {
			if e.repo == repo {
				r = append(r, e)
			}
		}
		return r
	}
	bail := func(f string, a ...any) {
		var bad []string
		if utool {
			if err := undo(REPO, of(REPO), "st.tool"); err != nil {
				bad = append(bad, REPO+": "+err.Error())
			}
		}
		if uconf {
			if err := undo(CONF, of(CONF), "st.conf"); err != nil {
				bad = append(bad, CONF+": "+err.Error())
			}
		}
		if len(bad) > 0 { // 되돌림을 확인하지 못했다 — 일지를 남겨 다음 실행이 다시 되돌린다(R-07)
			die(f+" — 이번 쓰기를 되돌리지 못했다 (%s). 일지 %s를 남긴다: 다음 herdr-sync가 다시 되돌린다",
				append(a, strings.Join(bad, "; "), journalPath())...)
		}
		dropJournal()
		die(f, a...)
	}
	writeJournal(jes)
	mc := make([][]string, len(TR))
	mr := make([][]string, len(TR))
	for _, i := range idx {
		if acts[i] != "모음" {
			continue
		}
		u := U[i]
		k, m, d, r := u.k, q.C[i], u.d, u.d+"/"+u.r
		g := "conf"
		if d == REPO {
			g = "tool"
		}
		if u.tree {
			if m == "-" {
				mr[u.tj] = append(mr[u.tj], u.r)
			} else {
				mc[u.tj] = append(mc[u.tj], strings.TrimPrefix(k, TR[u.tj].k+"/"))
			}
		} else {
			st[g] = append(st[g], u.r)
			if m == "-" {
				if _, rc := git("-C", d, "rm", "-q", "--", u.r); rc != 0 {
					bail("git rm 실패: %s", r)
				}
			} else {
				tmp := fmt.Sprintf("%s.hsync.%d", r, os.Getpid())
				ok := os.MkdirAll(filepath.Dir(r), 0o755) == nil
				if ok {
					f, err := os.Create(tmp)
					ok = err == nil
					if ok {
						c := mac("r", k)
						c.Stdout = f
						ok = run(c) == 0
						f.Close()
					}
				}
				if ok {
					mode := os.FileMode(0o644)
					if fi, err := os.Stat(tmp); err == nil {
						mode = fi.Mode().Perm()
					}
					if sigx(m) == "x" {
						mode |= 0o111
					} else {
						mode &^= 0o111
					}
					ok = os.Chmod(tmp, mode) == nil && os.Rename(tmp, r) == nil
				}
				if ok {
					_, rc := git("-C", d, "add", "--", u.r)
					ok = rc == 0
				}
				if !ok {
					os.Remove(tmp)
					bail("저장소에 쓰지 못했다: %s", r)
				}
			}
			if g == "tool" {
				ctool = append(ctool, u.r)
			} else {
				cconf = append(cconf, u.r)
			}
		}
		rbp = append(rbp, k+"\t"+m+"\t"+u.url)
		mm[g] = append(mm[g], "맥 모음 "+k)
	}
	for j, t := range TR {
		if len(mc[j]) == 0 && len(mr[j]) == 0 {
			continue
		}
		g := "conf"
		if t.d == REPO {
			g = "tool"
		}
		var ma []string
		for _, x := range mc[j] {
			ma = append(ma, t.r+"/"+x)
		}
		st[g] = append(append(st[g], ma...), mr[j]...)
		if len(mc[j]) > 0 {
			dst := t.d + "/" + t.r
			want := map[string]string{} // 나무 안 상대 경로 → 쓸 값의 서명(일지와 같은 값)
			for _, e := range of(t.d) {
				want[strings.TrimPrefix(e.path, t.r+"/")] = e.want
			}
			if err := collectTree(t.d, dst, t.k, mc[j], want, tarCreate(home+"/"+t.k, writeLines(fmt.Sprintf("mc%d", j), mc[j]))); err != nil { // 준비 폴더 → 확인 → 교체(R-202)
				bail("저장소에 쓰지 못했다: %s (%v)", dst, err)
			}
			if _, rc := git("-C", t.d, "--literal-pathspecs", "add", "--pathspec-from-file="+writeLines(fmt.Sprintf("ma%d", j), ma)); rc != 0 {
				bail("git add 실패: %s", dst)
			}
		}
		if len(mr[j]) > 0 {
			if _, rc := git("-C", t.d, "--literal-pathspecs", "rm", "-q", "--pathspec-from-file="+writeLines(fmt.Sprintf("mr%d", j), mr[j])); rc != 0 {
				bail("git rm 실패: %s/%s", t.d, t.r)
			}
		}
		if g == "tool" {
			ctool = append(ctool, t.r)
		} else {
			cconf = append(cconf, t.r)
		}
	}
	// 저장소마다 따로 커밋·push한다. 지난번에 push하지 못한 커밋도 여기서 다시 보낸다
	commit := func(d string, paths []string) bool {
		msg := "herdr-sync 모음:"
		for _, p := range paths {
			msg += " " + p
		}
		_, rc := git(append([]string{"-C", d, "--literal-pathspecs", "commit", "-q", "-m", msg, "--"}, paths...)...)
		return rc == 0
	}
	if len(ctool) > 0 {
		if !commit(REPO, ctool) {
			bail("커밋 실패: %s", REPO)
		}
		utool = false
		for _, l := range mm["tool"] {
			fmt.Println(l)
		}
	}
	if len(cconf) > 0 {
		if !commit(CONF, cconf) {
			bail("커밋 실패: %s", CONF)
		}
		uconf = false
		for _, l := range mm["conf"] {
			fmt.Println(l)
		}
	}
	if len(jes) > 0 {
		dropJournal()
	}
	for _, d := range []string{CONF, REPO} {
		s, rc := git("-C", d, "rev-list", "-1", "origin/main..HEAD")
		if rc != 0 { // 실패·시간 넘김을 「보낼 것 없음」으로 읽고 base를 적지 않는다(gofix-ops B02·K-01)
			die("보낼 커밋을 알지 못했다 (git rev-list 종료 %d — 모름): %s. 다시 돌리면 남은 커밋을 보낸다", rc, d)
		}
		if s == "" {
			continue
		}
		c := exec.Command("git", "-C", d, "push", "-q", "origin", "HEAD:main")
		c.Env = append(os.Environ(), "HERDR_LAND=1")
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		if run(c) != 0 {
			die("push 실패: %s. 거기서 git pull --rebase 뒤 HERDR_LAND=1 git push origin HEAD:main", d)
		}
	}
	if len(rbp) > 0 && runIn(mac("B"), bytes.NewReader(joinLines(rbp)), nil) != 0 {
		die("저장소 base를 쓰지 못했다")
	}
	// 깖: 나무 파일은 저장소마다 tar 한 묶음을 맥 W에 넘긴다
	kc := map[bool][]string{}
	kp := map[bool][]string{}
	bad := false
	for _, i := range idx {
		if acts[i] != "깖" {
			continue
		}
		u := U[i]
		k, r := u.k, RS[i]
		if !u.tree {
			r = rsig(u.d+"/"+u.r, k)
		}
		switch {
		case r == "-":
			if run(mac("d", k, STAMP)) != 0 {
				die("맥에서 지우지 못했다: %s", k)
			}
			rbp2 = append(rbp2, k+"\t-\t"+u.url)
			say("맥 지움 %s (저장소에서 지워짐)", k)
			continue
		case u.tree:
			g := u.d == REPO
			kc[g] = append(kc[g], k+"\t"+sigx(r)+"\t"+r+"\t"+u.r)
			kp[g] = append(kp[g], k+"\t"+r+"\t"+u.url)
			continue
		}
		in, err := os.Open(u.d + "/" + u.r)
		if err != nil || runIn(mac("w", k, sigx(r), r, STAMP), in, nil) != 0 {
			die("맥에 깔지 못했다: %s", k)
		}
		in.Close()
		rbp2 = append(rbp2, k+"\t"+r+"\t"+u.url)
		say("맥 깖 %s", k)
		if k == ".claude/CLAUDE.md#body" {
			body = true
		}
	}
	for _, g := range []bool{false, true} {
		if len(kc[g]) == 0 {
			continue
		}
		d := CONF
		if g {
			d = REPO
		}
		var wo bytes.Buffer
		pre, tar := wpack(d, kc[g], "kc.src")
		if p, c := pipeline(pre, tar, mac("W", STAMP), &wo); p != 0 || c != 0 {
			warn("맥 나무 깔기 묶음이 실패로 끝났다 (보냄 %d · 받음 %d)", p, c)
			bad = true
		}
		okk := map[string]bool{}
		for _, r := range wres(kc[g], wo.String()) {
			if r[0] == "ok" {
				say("맥 깖 %s", r[1])
				okk[r[1]] = true
			} else {
				warn("맥에 깔지 못했다: %s (%s)", r[1], r[2])
				bad = true
			}
		}
		for _, l := range kp[g] {
			if okk[field(l, 1)] {
				rbp2 = append(rbp2, l)
			}
		}
	}
	if len(rbp2) > 0 && runIn(mac("B"), bytes.NewReader(joinLines(rbp2)), nil) != 0 { // 깐 것의 base에 저장소 경로를 함께
		die("저장소 base를 쓰지 못했다")
	}
	if bad {
		exit(1)
	}
	rdiff = nil
}

// build: 맞추기 전에 개인 설정 sync.prebuild를 sh -c로 돌린다(예: 맥 지침에서 Codex 공통 구역을 다시 만듦). 없으면 아무것도 안 한다
func build() {
	if check {
		return
	}
	pre, err := toolcfg.Lookup(CONF, "sync.prebuild", "", os.Stderr)
	if err != nil {
		die("개인 설정을 읽지 못했다: %s/config", CONF)
	}
	if pre == "" {
		return
	}
	c := exec.Command("sh", "-c", pre)
	c.Stderr = os.Stderr
	if rc := run(c); rc != 0 { // 명령 원문은 찍지 않는다 — 개인 경로·인증 인자가 들 수 있다(gofix-ops B17)
		die("sync.prebuild 실패 (개인 설정 키 sync.prebuild, 종료 %d)", rc)
	}
}

// row: 기계 한 줄(다름 묶음 목록·충돌 수)
func row(name string, diffs []string, conf int) {
	seen := map[string]bool{}
	var d []string
	for _, x := range diffs {
		for _, w := range strings.Fields(x) {
			if !seen[w] {
				seen[w] = true
				d = append(d, w)
			}
		}
	}
	s := ""
	if len(d) > 0 {
		s = "다름 (" + strings.Join(d, "·") + ")"
	}
	if conf > 0 {
		if s != "" {
			s += " · "
		}
		s += fmt.Sprintf("충돌 %d건", conf)
	}
	if s != "" {
		rc = 1
	} else {
		s = "같음"
	}
	if !check {
		rowResult(name, len(d) > 0, conf) // 배포 기록(deploylog.go)
	}
	rows += name + ": " + s + RSUF + "\n"
}

func few(a []string) string {
	if len(a) > 3 {
		return strings.Join(a[:3], " ") + " …"
	}
	return strings.Join(a, " ")
}

// macrow: --check의 맥 줄(맥 ↔ 두 저장소의 방향)
func macrow() {
	var p []string
	if len(RBH) > 0 {
		p = append(p, fmt.Sprintf("저장소가 새로움 %d개 (%s) — herdr-sync로 깐다", len(RBH), few(RBH)))
	}
	if len(RAH) > 0 {
		p = append(p, fmt.Sprintf("저장소보다 새로움 %d개 (%s) — 커밋 안 됨?", len(RAH), few(RAH)))
	}
	var d []string
	for _, w := range []string{"지움", "바이너리"} {
		for _, x := range rdiff {
			if x == w {
				d = append(d, w)
				break
			}
		}
	}
	if len(d) > 0 {
		p = append(p, "다름 ("+strings.Join(d, "·")+")")
	}
	if rconf > 0 {
		p = append(p, fmt.Sprintf("충돌 %d건", rconf))
	}
	s := strings.Join(p, " · ")
	if s != "" {
		rc = 1
	} else {
		s = "저장소와 같음"
	}
	rows = "맥: " + s + "\n"
}

// ready <저장소>: git·main·깨끗함 확인 뒤 pull --ff-only. git 조회가 실패하거나 시간을 넘기면(124 — 모름) 깨끗함·main으로 읽지 않고 멈춘다
// (gofix-ops B01·O-01·K-01: 실패한 status의 빈 출력을 깨끗함으로 읽어 커밋 안 된 내용을 맥·기계에 깔았다)
func ready(r string) {
	if msg := readyState(r); msg != "" {
		die("%s", msg)
	}
	if why := pullFF(r); why != "" { // 잠금 경합은 몇 번 다시(gitpull.go)
		die("git pull --ff-only 실패: %s (지난번 push 못 한 커밋이 있으면 거기서 git pull --rebase 뒤 HERDR_LAND=1 git push origin HEAD:main) — git: %s", r, why)
	}
}

// readyState: 저장소·main·깨끗함 판정 — 준비됐으면 "", 아니면 멈출 까닭
func readyState(r string) string {
	if _, rc := gitq("-C", r, "rev-parse", "--git-dir"); rc == proc.TimeoutCode {
		return "저장소 상태를 읽지 못했다 (git rev-parse 시간 넘김): " + r
	} else if rc != 0 {
		return "저장소가 없다: " + r
	}
	// 종료 0일 때만 출력을 브랜치 이름으로 읽는다 — 실패한 조회가 main을 찍었어도 버린다(R-02)
	if b, rc := gitq("-C", r, "symbolic-ref", "--short", "HEAD"); rc == proc.TimeoutCode {
		return "저장소 상태를 읽지 못했다 (git symbolic-ref 시간 넘김): " + r
	} else if rc != 0 {
		return fmt.Sprintf("저장소 브랜치를 읽지 못했다 (git symbolic-ref 종료 %d — main이 아니거나 모름): %s", rc, r)
	} else if b != "main" {
		return "저장소가 main 브랜치가 아니다: " + r
	}
	if s, rc := git("-C", r, "status", "--porcelain"); rc != 0 {
		return fmt.Sprintf("저장소 상태를 읽지 못했다 (git status 종료 %d — 모름): %s", rc, r)
	} else if s != "" {
		return "저장소에 커밋하지 않은 변경이 있다. 커밋하거나 되돌린 뒤 다시: " + r
	}
	return ""
}
