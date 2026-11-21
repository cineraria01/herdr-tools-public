package main

// cotenant-guard Go 판 — 남의 미커밋 임대 훑기(scan_foreign)·hc 에이전트 찾기·동거 판정·foreign_under (cotenant.go에서 옮김)

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// scanForeign: FDIRTY = ROOT에서 다른 산 프로세스가 쥔 미커밋 파일 임대(rel\037name\037pid\037since 줄).
// 트리마다 한 정리자(.cleaning)가 임대를 다 읽고, 넘겨받기가 어차피 넘길 깨끗한 임대를 지운다. 나머지는 바뀐 경로만 읽는다.
// 셸 판의 시간 상한(읽기 약 1~2초, 지우기 2~3초)·개수 상한(읽기 5000, 지우기 1000)도 같다
func (g *cg) scanForeign() {
	if g.fSet && g.fRoot == g.root {
		return
	}
	g.fSet, g.fRoot, g.fdirty = true, g.root, ""
	if !isDir(g.leases) {
		return
	}
	g.me()
	var lr, lw, ld, lp, dirty, fd, gone, cr []string
	var ci []int
	hit := map[int]bool{}
	part := false
	if !g.claimgen(g.leases + "/.cleaning") {
		g.changedLeases()
		return
	}
	var owners []string
	if ents, err := os.ReadDir(g.leases); err == nil {
		for _, e := range ents {
			if n := e.Name(); !strings.HasPrefix(n, ".") {
				if _, err := os.Lstat(g.leases + "/" + n + "/owner"); err == nil {
					owners = append(owners, n)
				}
			}
		}
	}
	sort.Strings(owners)
	for _, n := range owners {
		if !(g.secs() < 2 && len(lr) < 5000) {
			part = true
			break
		}
		f := g.leases + "/" + n + "/owner"
		line, nl, ok := readLine(f)
		if ok {
			g.setOwner(line)
		}
		if !ok || !nl {
			continue
		}
		if g.oPID == g.mypid && g.mine() {
			continue
		}
		if g.aok[g.oPID+" "+g.oStart] || g.alive(g.oPID, g.oStart) {
			lw = append(lw, g.oName+us+g.oPID+us+g.oSince)
		} else {
			lw = append(lw, "")
		}
		lr = append(lr, g.oPath)
		ld = append(ld, g.leases+"/"+n)
		lp = append(lp, g.oPID+us+g.oSince)
	}
	cleaning := g.leases + "/.cleaning"
	if len(lr) == 0 {
		rmF(cleaning)
		if part {
			g.changedLeases()
		}
		return
	}
	pathspecs := make([]string, len(lr))
	for i, p := range lr {
		pathspecs[i] = ":(literal)" + p
	}
	out, ok := g.statusZ(pathspecs)
	if !ok { // 실패한 출력은 파싱하지 않는다(아스트라 E02) — 정리 없이 바뀐 경로만 다시(그 status도 실패면 막힘)
		rmF(cleaning)
		g.changedLeases()
		return
	}
	recs := strings.Split(out, "\x00") // statusZ가 NUL로 끝남을 확인했다 — 마지막 칸은 빈 칸
	tail := recs[len(recs)-1]
	recs = recs[:len(recs)-1]
	for i := 0; i < len(recs); i++ {
		e := recs[i]
		st := e
		if len(st) > 2 {
			st = st[:2]
		}
		if len(e) > 3 {
			dirty = append(dirty, e[3:])
		} else {
			dirty = append(dirty, "")
		}
		if strings.HasPrefix(st, "R") || strings.HasPrefix(st, "C") {
			next := tail
			if i+1 < len(recs) {
				next = recs[i+1]
			}
			i++
			if strings.HasPrefix(st, "R") {
				dirty = append(dirty, next)
			}
		}
	}
	if len(dirty) > 0 {
		// awk 한 번: 바뀐 경로와 그 위 폴더 모두를 모으고, 임대 경로(줄)마다 맞으면 그 줄 번호
		d := map[string]bool{}
		for _, r := range strings.Split(strings.Join(dirty, "\n"), "\n") {
			d[r] = true
			for x := r; ; {
				i := strings.LastIndexByte(x, '/')
				if i < 0 {
					break
				}
				x = x[:i]
				d[x] = true
			}
		}
		for i, r := range strings.Split(strings.Join(lr, "\n"), "\n") {
			if d[r] {
				hit[i] = true
				if i < len(lw) && lw[i] != "" {
					fd = append(fd, lr[i]+us+lw[i])
				}
			}
		}
	}
	if part {
		g.changedLeases()
	} else if len(fd) > 0 {
		g.fdirty = strings.Join(fd, "\n") + "\n"
	}
	idle := g.leases + "/.idle"
	if f, err := os.OpenFile(idle, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666); err == nil {
		f.Close()
		if fi, err := os.Stat(idle); err == nil {
			t := fi.ModTime().Add(-600 * time.Second)
			if os.Chtimes(idle, t, t) != nil {
				rmF(idle)
			}
		} else {
			rmF(idle)
		}
	} else {
		rmF(idle)
	}
	claimOf := func(i int) string {
		pid, since, _ := strings.Cut(lp[i], us)
		return ld[i] + "/claim-" + pid + "-" + digitsOnly(since)
	}
	for i := range lr {
		if len(gone) >= 1000 || g.secs() >= 3 {
			break
		}
		if hit[i] {
			continue
		}
		if lw[i] != "" && !olderThan(ld[i]+"/owner", idle) {
			cr = append(cr, lr[i])
			ci = append(ci, i)
			continue
		}
		c := claimOf(i)
		if !claim(c) && !(olderThan(c, idle) && g.claimgen(c)) {
			continue
		}
		gone = append(gone, ld[i])
	}
	if len(cr) > 0 && len(gone) < 1000 && g.secs() < 3 {
		out, rc := g.gitRun(nil, strings.Join(cr, "\x00")+"\x00", "-C", g.root, "check-ignore", "-z", "--stdin")
		if rc != 0 { // 1 = 무시된 것 없음, 그 밖(실패·시간 넘김)은 모름 — 어느 쪽이든 지우지 않는다
			out = ""
		}
		ign := strings.Split(out, "\x00")
		ign = ign[:len(ign)-1]
		j := 0
		for k := range cr {
			ig := ""
			if j < len(ign) {
				ig = ign[j]
			}
			if cr[k] != ig {
				continue
			}
			j++
			if i := ci[k]; claim(claimOf(i)) {
				gone = append(gone, ld[i])
			}
		}
	}
	if len(gone) == 0 {
		rmF(cleaning)
		return
	}
	// 한 번에 자리에서 치우고(죽어도 claim만 남은 임대가 없게) 휴지통 폴더만 뒤에서 지운다
	trash := g.leases + "/.trash-" + strconv.Itoa(os.Getpid())
	if os.MkdirAll(trash, 0o777) == nil {
		for _, s := range append(gone, cleaning) {
			os.Rename(s, trash+"/"+baseName(s))
		}
	}
	c := exec.Command("/bin/rm", "-rf", trash)
	if c.Start() == nil {
		c.Process.Release()
	}
}

// hcWho: hc ls에서 이 기계의 산 에이전트 CLI 가운데 ROOT 안에 있는 것(허용 목록 — 개발 서버·빌드·셸은 아님).
// hc가 있는데 hc ls가 실패·2초 넘김·읽을 수 없는 JSON이면 모름 — WHO에 그렇게 적어 동거로 친다(아스트라 E03·E04·Fable E2:
// 「hc 에이전트 없음」으로 읽어 남의 미커밋 파일을 넘겼다. 셸 판은 그대로 없음). 2초는 출력 관을 쥔 손자까지(hookRun)
func (g *cg) hcWho() {
	hc, err := exec.LookPath("hc")
	if err != nil {
		return
	}
	out, _, rc := hookRun([]string{"hc", "ls"}, 2*time.Second, nil, "", hc, "ls", "--json")
	vals, jerr := jqValues([]byte(out))
	if rc != 0 || jerr != nil {
		why := fmt.Sprintf("종료 %d", rc)
		if rc == timeoutRC {
			why = "시간 넘김"
		} else if rc == 0 {
			why = "JSON을 읽지 못함"
		}
		g.who += ", hc ls 실패(" + why + ") — 같은 트리의 hc 에이전트를 모름"
		return
	}
	var b strings.Builder
hcLoop:
	for _, v := range vals {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		var elems []any
		switch x := m["terminals"].(type) {
		case []any:
			elems = x
		case map[string]any:
			for _, e := range x {
				elems = append(elems, e)
			}
		}
		for _, e := range elems {
			if e != nil {
				if _, ok := e.(map[string]any); !ok {
					break hcLoop
				}
			}
			if mapGet(e, "state") != "live" {
				continue
			}
			addr, ok := mapGet(e, "address").(string)
			if !ok {
				break hcLoop
			}
			if strings.Contains(addr, "/") {
				continue
			}
			var f []string
			for _, k := range []string{"address", "agent", "cwd", "name"} {
				x := mapGet(e, k)
				if !jqTruthy(x) {
					x = ""
				}
				f = append(f, jqToString(x))
			}
			b.WriteString(strings.Join(f, us) + "\n")
		}
	}
	for _, l := range heredocLines(capture(b.String())) {
		f := bashRead(l, 0x1f, 4)
		addr, agent, cwd, name := f[0], f[1], f[2], f[3]
		if addr == "" || addr == g.hcTerm {
			continue
		}
		if !(strings.HasPrefix(agent, "claude") || strings.HasPrefix(agent, "codex") || agent == "agy" || strings.HasPrefix(agent, "grok") ||
			agent == "cursor-agent" || strings.HasPrefix(agent, "gemini") || agent == "opencode") {
			continue
		}
		if g.intree(cwd) {
			if name == "" {
				name = agent
			}
			g.who += ", " + name + "[" + addr + "]"
		}
	}
}

// cotenant: 다른 산 에이전트가 ROOT를 같이 쓴다 — 레지스트리의 Claude, hc 에이전트 터미널, 미커밋 파일을 쥔 산 임대 주인. WHO가 그 이름들
func (g *cg) cotenant() bool {
	if g.cSet && g.cRoot == g.root {
		return g.cRC
	}
	g.cSet, g.cRoot, g.who = true, g.root, ""
	g.me()
	for _, l := range heredocLines(g.reg) {
		f := bashRead(l, 0x1f, 5)
		pid, st, cwd, name := f[0], f[2], f[3], f[4]
		if pid == "" || pid == g.mypid {
			continue
		}
		if g.intree(cwd) && g.alive(pid, st) {
			g.who += ", " + name + "(pid " + pid + ")"
		}
	}
	if g.who == "" {
		g.hcWho()
	}
	if g.who == "" {
		g.scanForeign()
		for _, l := range heredocLines(g.fdirty) {
			f := bashRead(l, 0x1f, 4)
			if f[0] != "" {
				g.who += ", " + f[1] + "(pid " + f[2] + ")"
			}
		}
	}
	g.who = strings.TrimPrefix(g.who, ", ")
	g.cRC = g.who != ""
	return g.cRC
}

// fdirtyNames: lines() — 「경로(이름), …」
func fdirtyNames(fdirty string) string {
	var out []string
	for _, l := range heredocLines(fdirty) {
		f := bashRead(l, 0x1f, 2)
		if f[0] != "" {
			name, _, _ := strings.Cut(f[1], us)
			out = append(out, f[0]+"("+name+")")
		}
	}
	return strings.Join(out, ", ")
}

// foreignUnder PREFIX [PATTERN]: 남의 미커밋 임대 파일이 PREFIX이거나 그 아래, 또는 PATTERN이면 PREFIX 글롭(*가 /도 넘는다)에 맞으면 막는다
func (g *cg) foreignUnder(prefix string, pattern bool, msg string) {
	g.scanForeign()
	for _, l := range heredocLines(g.fdirty) {
		f := bashRead(l, 0x1f, 2)
		if f[0] == "" {
			continue
		}
		switch {
		case prefix == "":
		case pattern:
			if !bashMatch(f[0], prefix) && !bashMatch(f[0], prefix+"/*") {
				continue
			}
		default:
			if f[0] != prefix && !strings.HasPrefix(f[0], prefix+"/") {
				continue
			}
		}
		name, _, _ := strings.Cut(f[1], us)
		g.block(f[0] + " 는 " + name + "이 고치는 중인 파일(미커밋)이다. " + msg)
	}
}
