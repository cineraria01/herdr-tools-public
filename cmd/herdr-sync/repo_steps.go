package main

// repoSync(repo.go)의 단계들(2026-11-24 long-func-sync — 415줄 한 함수를 단계별로): 서명 → 판정 → (--check 보고) → 모음·커밋 → push → 깖.
// 판정 한 줄(repoAct)은 읽기·쓰기가 없어 꾸러미 안 단위 시험(repo_act_test.go)으로 잰다

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// repoSigs: 저장소 쪽 서명 RS(단위마다, 없으면 "-"). 나무는 줄마다 한 번에 걷는다
func repoSigs() []string {
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
	return RS
}

// repoBasePaths: 단위마다 repo-base 셋째 칸 = 그 base를 적을 때의 저장소 경로(옛 두 칸 줄·줄 없음은 "").
// 줄을 옮겨 지금 경로와 다르면 base 없음으로 본다: 새 자리에 저장소 파일이 없으면 「맥만 있음」 = 모음이 된다(맥 파일을 지우지 않는다). 옛 두 칸 줄은 그대로 믿는다
func repoBasePaths() []string {
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
	return RP
}

// repoAct: 맥·저장소가 다른 한 줄의 판정(읽기·쓰기 없음). m·r·b = 맥·저장소·base 서명("-" 없음), oldBase = base의 저장소 경로를 모름
// (옛 두 칸 줄·줄 없음), take·pull = --take·--pull repo:로 고른 줄, section = 구역 파일(#). act "모음"·"깖", ""면 충돌(why가 ""면 부르는 쪽이 줄 수 다름으로)
func repoAct(m, r, b string, oldBase, take, pull, section bool) (act, why string) {
	switch {
	case take:
		act = "모음"
	case pull:
		act = "깖"
	case r == b:
		act = "모음"
	case m == b:
		act = "깖"
	}
	if act == "깖" && r == "-" && section { // 구역 파일이 저장소에서 지워졌으면 충돌로
		act = ""
	}
	// 옛 두 칸 base(경로를 모름)에서 저장소 쪽이 없으면, 정말 지운 것인지 줄을 옮긴 것인지 모르니 맥 파일을 지우지 않고 멈춘다
	if act == "깖" && r == "-" && oldBase && !pull {
		act, why = "", "저장소에 없음 — 옛 base라 지운 것인지 모른다"
	}
	return act, why
}

// repoPlan: 줄마다 판정 — 같은 줄의 base 다시 적기(rbp), 할 일(acts·idx). 충돌은 찍고 rconf·rdiff에 센다
func repoPlan(q *qres, RS, RP []string, LK map[int]string) (rbp []string, acts map[int]string, idx []int) {
	acts = map[int]string{}
	shown := map[string]bool{}
	for i, u := range U {
		k, m, b := u.k, q.C[i], q.B[i]
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
		act, why := repoAct(m, r, b, RP[i] == "", spec(takes, "repo", k), spec(pulls, "repo", k), strings.Contains(k, "#"))
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
	return rbp, acts, idx
}

// repoCheckRows: --check — 깖 = 저장소가 새로움(RBH), 모음 = 맥이 새로움(RAH). 맥 줄과 기계 줄 끝 표시에 쓴다
func repoCheckRows(idx []int, acts map[int]string, RS []string) {
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
}

// gather: 모음 쓰기 한 번의 상태. 쓰기·스테이징·커밋이 실패하면 아직 커밋 안 된 저장소(utool·uconf)의 이번 쓰기를 되돌리고(undo) 멈춘다
type gather struct {
	jes          []jentry // 쓰기 전 일지(journal.go). old = 쓰기 전 저장소 서명
	utool, uconf bool
	rbp          []string            // 모은 줄의 base
	mc, mr       [][]string          // 나무마다 모을(나무 안 상대 경로)·지울(저장소 경로) 파일
	ctool, cconf []string            // 저장소마다 커밋할 경로
	mm           map[string][]string // 「맥 모음」 줄 — 그 저장소 커밋이 된 뒤에 찍는다
}

func (g *gather) of(repo string) []jentry {
	var r []jentry
	for _, e := range g.jes {
		if e.repo == repo {
			r = append(r, e)
		}
	}
	return r
}

func (g *gather) bail(f string, a ...any) {
	var bad []string
	if g.utool {
		if err := undo(REPO, g.of(REPO), "st.tool"); err != nil {
			bad = append(bad, REPO+": "+err.Error())
		}
	}
	if g.uconf {
		if err := undo(CONF, g.of(CONF), "st.conf"); err != nil {
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

// repoGather: 모음을 저장소에 쓰고 저장소마다 커밋한다. 모은 줄의 base(rbp)를 돌려준다.
// 나무 파일은 나무마다 tar 한 묶음으로 옮기고(실행 권한 포함) git add·rm도 한 번씩 한다
func repoGather(q *qres, idx []int, acts map[int]string, RS []string) []string {
	g := &gather{utool: true, uconf: true, mm: map[string][]string{}, mc: make([][]string, len(TR)), mr: make([][]string, len(TR))}
	// 쓰기 전에 일지 — 쓰기와 커밋 사이에 죽어도 다음 실행이 자기 것만 되돌린다
	for _, i := range idx {
		if acts[i] == "모음" {
			g.jes = append(g.jes, jentry{U[i].d, "", U[i].r, U[i].k, q.C[i], RS[i]})
		}
	}
	writeJournal(g.jes)
	for _, i := range idx {
		if acts[i] == "모음" {
			g.unit(U[i], q.C[i])
		}
	}
	for j, t := range TR {
		g.tree(j, t)
	}
	g.commitAll()
	return g.rbp
}

// unit: 모음 한 줄 — 나무 파일은 나무 묶음에 넣고, 아니면 바로 쓰고(지움이면 git rm) 스테이징
func (g *gather) unit(u unit, m string) {
	k := u.k
	grp := "conf"
	if u.d == REPO {
		grp = "tool"
	}
	if u.tree {
		if m == "-" {
			g.mr[u.tj] = append(g.mr[u.tj], u.r)
		} else {
			g.mc[u.tj] = append(g.mc[u.tj], strings.TrimPrefix(k, TR[u.tj].k+"/"))
		}
	} else {
		if why := collectFile(u, m); why != "" {
			g.bail("%s", why)
		}
		if grp == "tool" {
			g.ctool = append(g.ctool, u.r)
		} else {
			g.cconf = append(g.cconf, u.r)
		}
	}
	g.rbp = append(g.rbp, k+"\t"+m+"\t"+u.url)
	g.mm[grp] = append(g.mm[grp], "맥 모음 "+k)
}

// collectFile: 맥 값 m을 저장소 파일로(m이 "-"면 git rm) 쓰고 스테이징. 실패하면 멈출 까닭, 되면 ""
func collectFile(u unit, m string) string {
	k, d, r := u.k, u.d, u.d+"/"+u.r
	if m == "-" {
		if _, rc := git("-C", d, "rm", "-q", "--", u.r); rc != 0 {
			return fmt.Sprintf("git rm 실패: %s", r)
		}
		return ""
	}
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
		return fmt.Sprintf("저장소에 쓰지 못했다: %s", r)
	}
	return ""
}

// tree: 나무 j의 모음 묶음을 저장소에 쓰고(준비 폴더 → 확인 → 교체, R-202) git add·rm을 한 번씩
func (g *gather) tree(j int, t root) {
	mc, mr := g.mc[j], g.mr[j]
	if len(mc) == 0 && len(mr) == 0 {
		return
	}
	var ma []string
	for _, x := range mc {
		ma = append(ma, t.r+"/"+x)
	}
	if len(mc) > 0 {
		dst := t.d + "/" + t.r
		want := map[string]string{} // 나무 안 상대 경로 → 쓸 값의 서명(일지와 같은 값)
		for _, e := range g.of(t.d) {
			want[strings.TrimPrefix(e.path, t.r+"/")] = e.want
		}
		if err := collectTree(t.d, dst, t.k, mc, want, tarCreate(home+"/"+t.k, writeLines(fmt.Sprintf("mc%d", j), mc))); err != nil {
			g.bail("저장소에 쓰지 못했다: %s (%v)", dst, err)
		}
		if _, rc := git("-C", t.d, "--literal-pathspecs", "add", "--pathspec-from-file="+writeLines(fmt.Sprintf("ma%d", j), ma)); rc != 0 {
			g.bail("git add 실패: %s", dst)
		}
	}
	if len(mr) > 0 {
		if _, rc := git("-C", t.d, "--literal-pathspecs", "rm", "-q", "--pathspec-from-file="+writeLines(fmt.Sprintf("mr%d", j), mr)); rc != 0 {
			g.bail("git rm 실패: %s/%s", t.d, t.r)
		}
	}
	if t.d == REPO {
		g.ctool = append(g.ctool, t.r)
	} else {
		g.cconf = append(g.cconf, t.r)
	}
}

// commitAll: 저장소마다 따로 커밋하고, 된 저장소의 「맥 모음」 줄을 찍는다. 다 되면 일지를 지운다
func (g *gather) commitAll() {
	commit := func(d string, paths []string) bool {
		msg := "herdr-sync 모음:"
		for _, p := range paths {
			msg += " " + p
		}
		_, rc := git(append([]string{"-C", d, "--literal-pathspecs", "commit", "-q", "-m", msg, "--"}, paths...)...)
		return rc == 0
	}
	if len(g.ctool) > 0 {
		if !commit(REPO, g.ctool) {
			g.bail("커밋 실패: %s", REPO)
		}
		g.utool = false
		for _, l := range g.mm["tool"] {
			fmt.Println(l)
		}
	}
	if len(g.cconf) > 0 {
		if !commit(CONF, g.cconf) {
			g.bail("커밋 실패: %s", CONF)
		}
		g.uconf = false
		for _, l := range g.mm["conf"] {
			fmt.Println(l)
		}
	}
	if len(g.jes) > 0 {
		dropJournal()
	}
}

// repoPush: 두 저장소의 보낼 커밋을 push한다. 지난번에 push하지 못한 커밋도 여기서 다시 보낸다
func repoPush() {
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
}

// repoInstall: 깖 — 맥에 깔거나 지우고, 깐 것의 base 줄(rbp2)을 돌려준다. 나무 파일은 저장소마다 tar 한 묶음을 맥 W에 넘긴다.
// bad = 나무 묶음에서 못 깐 것이 있다
func repoInstall(idx []int, acts map[int]string, RS []string) (rbp2 []string, bad bool) {
	kc := map[bool][]string{}
	kp := map[bool][]string{}
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
		ok, b := installTrees(d, kc[g], kp[g])
		rbp2 = append(rbp2, ok...)
		bad = bad || b
	}
	return rbp2, bad
}

// installTrees: 저장소 d의 나무 파일 묶음(kc)을 맥 W로 깔고, 깐 것의 base 줄(kp 가운데)과 실패 여부를 돌려준다
func installTrees(d string, kc, kp []string) (rbp []string, bad bool) {
	var wo bytes.Buffer
	pre, tar := wpack(d, kc, "kc.src")
	if p, c := pipeline(pre, tar, mac("W", STAMP), &wo); p != 0 || c != 0 {
		warn("맥 나무 깔기 묶음이 실패로 끝났다 (보냄 %d · 받음 %d)", p, c)
		bad = true
	}
	okk := map[string]bool{}
	for _, r := range wres(kc, wo.String()) {
		if r[0] == "ok" {
			say("맥 깖 %s", r[1])
			okk[r[1]] = true
		} else {
			warn("맥에 깔지 못했다: %s (%s)", r[1], r[2])
			bad = true
		}
	}
	for _, l := range kp {
		if okk[field(l, 1)] {
			rbp = append(rbp, l)
		}
	}
	return rbp, bad
}
