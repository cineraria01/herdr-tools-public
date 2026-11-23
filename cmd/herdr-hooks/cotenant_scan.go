package main

// cotenant-guard Go 판 — 남의 미커밋 임대 훑기(scan_foreign)·hc 에이전트 찾기·동거 판정·foreign_under (cotenant.go에서 옮김)

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/hcjson"
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
	cleaning := g.leases + "/.cleaning"
	// 정리자를 못 잡으면 읽고 맞추기만 한다(지우지 않는다) — 바뀐 경로의 git 철자 열쇠(md5)만 보면 옛 판이 다른 철자로 적은
	// 임대를 놓친다(재감사 R-H203)
	clean := g.claimgen(cleaning)
	var owners, stale []string
	ents, err := os.ReadDir(g.leases)
	if err != nil { // 목록을 못 읽음은 「임대 없음」이 아니다(모름) — 정리 없이 바뀐 경로만(작업 중 발견해 함께 고침)
		if clean {
			rmF(cleaning)
		}
		g.changedLeases()
		return
	}
	for _, e := range ents {
		n := e.Name()
		if pid, ok := strings.CutPrefix(n, ".trash-"); ok && !bashKill0(pid) {
			stale = append(stale, g.leases+"/"+n) // 끝난 정리자가 못 지운 휴지통(재감사 R-L02)
		}
		if !strings.HasPrefix(n, ".") {
			if _, err := os.Lstat(g.leases + "/" + n + "/owner"); err == nil {
				owners = append(owners, n)
			}
		}
	}
	if clean && len(stale) > 0 { // 지금 정리자는 이 실행(.cleaning) — 그 pid가 끝난 휴지통만 뒤에서 지운다
		bgRm(stale...)
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
	if len(lr) == 0 {
		if clean {
			rmF(cleaning)
		}
		if part {
			g.changedLeases()
		}
		return
	}
	// 임대 경로는 옛 판이 적은 철자(NFD·다른 대소문자)일 수 있다 — git이 같은 파일로 찾게 icase로 묻고, 견줄 때는 NFC·소문자로
	// (재감사 R-H03: 혼용 기간의 임대를 「안 바뀜」으로 읽어 지우지 않게. 대소문자만 다른 두 파일이 있는 파일시스템에서는 더 많이
	// 맞아 덜 지울 뿐이다)
	pathspecs := make([]string, len(lr))
	for i, p := range lr {
		pathspecs[i] = ":(literal,icase)" + p
	}
	dirty, ok := g.statusZ(pathspecs)
	keys, nok := g.foldKeys(lr)
	if !ok || !nok { // 실패한 출력은 파싱하지 않는다(아스트라 E02·R-H05) — 정리 없이 바뀐 경로만 다시(그 status도 실패면 막힘)
		if clean {
			rmF(cleaning)
		}
		g.changedLeases()
		return
	}
	// 바뀐 경로와 그 위 폴더 모두 — 소문자 열쇠 → git 철자. 임대 경로(줄)마다 맞으면 FDIRTY에는 git 철자로 적는다(재감사 R-H203:
	// 옛 철자 SUB/C.TXT를 그대로 내 foreignUnder의 접두 비교 git add sub가 놓쳤다). 열쇠 하나에 git 철자가 여럿이면(대소문자만 바꾼
	// 이름 변경의 새·옛 이름) 모두 적는다 — 하나만 남기면 옛 철자 글롭 git add 'N*.txt'가 놓쳤다(재감사 R-3A01)
	d := map[string][]string{}
	for _, r := range dirty {
		for x := r; ; {
			if k := strings.ToLower(x); !slices.Contains(d[k], x) {
				d[k] = append(d[k], x)
			}
			i := strings.LastIndexByte(x, '/')
			if i < 0 {
				break
			}
			x = x[:i]
		}
	}
	for i, r := range keys {
		if ps, ok := d[r]; ok {
			hit[i] = true
			for _, p := range ps {
				if lw[i] == "" {
					break
				}
				fd = append(fd, p+us+lw[i])
			}
		}
	}
	if part { // 상한에 걸려 못 읽은 나머지는 바뀐 경로의 임대로 — 읽은 몫의 판정도 남긴다
		g.changedLeases()
		if len(fd) > 0 {
			g.fdirty = strings.Join(fd, "\n") + "\n" + g.fdirty
		}
	} else if len(fd) > 0 {
		g.fdirty = strings.Join(fd, "\n") + "\n"
	}
	if !clean {
		return
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
	bgRm(trash)
}

// bgRm: /bin/rm -rf 경로… 를 뒤에서(기다리지 않는다) — 휴지통 폴더만
func bgRm(paths ...string) {
	c := exec.Command("/bin/rm", append([]string{"-rf", "--"}, paths...)...)
	if c.Start() == nil {
		c.Process.Release() //nolint:errcheck
	}
}

// foldKeys: 임대 경로를 견줄 열쇠로 — git argv의 NFC(ASCII 아닌 것만 check-attr 한 번), 소문자. git이 실패하면 ok=false(모름)
func (g *cg) foldKeys(paths []string) ([]string, bool) {
	keys := make([]string, len(paths))
	var idx []int
	args := []string{"-C", g.root, "check-attr", "-z", "herdr-hooks-path", "--"}
	for i, p := range paths {
		keys[i] = p
		if !isASCII(p) {
			idx = append(idx, i)
			args = append(args, p)
		}
	}
	if len(idx) > 0 {
		out, rc := g.gitRun(nil, "", args...)
		f := strings.Split(out, "\x00")
		if rc != 0 || len(f) != 3*len(idx)+1 || f[len(f)-1] != "" {
			return nil, false
		}
		for k, i := range idx {
			keys[i] = f[3*k]
		}
	}
	for i := range keys {
		keys[i] = strings.ToLower(keys[i])
	}
	return keys, true
}

// hcWho: hc ls에서 이 기계의 산 에이전트 CLI 가운데 ROOT 안에 있는 것(허용 목록 — 개발 서버·빌드·셸은 아님).
// hc가 있는데 hc ls가 실패·2초 넘김·읽을 수 없는 JSON이면 모름 — WHO에 그렇게 적어 동거로 친다(아스트라 E03·E04·Fable E2:
// 「hc 에이전트 없음」으로 읽어 남의 미커밋 파일을 넘겼다. 셸 판은 그대로 없음). 2초는 출력 관을 쥔 손자까지(hookRun)
func (g *cg) hcWho() {
	hc, err := exec.LookPath("hc")
	if err != nil {
		return
	}
	end := time.Now().Add(2 * time.Second) // ls와 get 모두 이 2초 안에서(지금 상한)
	out, _, rc := hookRun([]string{"hc", "ls"}, 2*time.Second, nil, "", hc, "ls", "--json")
	terms, perr := hcTerminals(out)
	if rc != 0 || perr != "" {
		why := fmt.Sprintf("종료 %d", rc)
		if rc == timeoutRC {
			why = "시간 넘김"
		} else if rc == 0 {
			why = perr
		}
		g.who += ", hc ls 실패(" + why + ") — 같은 트리의 hc 에이전트를 모름"
		return
	}
	var rest []hcjson.Terminal
	for _, t := range terms {
		if strings.Contains(t.Address, "/") || t.Address == g.hcTerm {
			continue
		}
		if !(strings.HasPrefix(t.Agent, "claude") || strings.HasPrefix(t.Agent, "codex") || t.Agent == "agy" || strings.HasPrefix(t.Agent, "grok") ||
			t.Agent == "cursor-agent" || strings.HasPrefix(t.Agent, "gemini") || t.Agent == "opencode") {
			continue
		}
		if g.intree(t.CwdText) {
			g.who += ", " + whoName(t) + "[" + t.Address + "]"
		} else {
			rest = append(rest, t)
		}
	}
	if g.who != "" {
		return
	}
	// 띄운 폴더로는 아무도 — 띄운 뒤 cd로 들어온 에이전트도 지금 폴더(hc get foreground.cwd)로 본다. 못 얻거나 2초를 넘기면 모름 =
	// 동거(2026-11-23 cwd-now-inuse)
	for _, t := range rest {
		if time.Until(end) <= 0 { // hookRun은 0 이하 상한을 「없음」으로 읽는다 — 여기서 넘김으로
			g.who += ", " + whoName(t) + "[" + t.Address + "] 지금 폴더 모름(시간 넘김)"
			return
		}
		gout, gerr, grc := hookRun([]string{"hc", "get"}, time.Until(end), nil, "", hc, "get", t.Address, "--json")
		if grc != 0 && strings.TrimSpace(gerr) != "" {
			gout = gerr // 종료 3의 not_found 오류 객체는 표준 오류에 온다
		}
		gt, found, err := hcjson.ParseGet([]byte(gout), grc, t.Address)
		switch {
		case err != nil || (found && gt.State == "live" && gt.NowCwd == ""):
			g.who += ", " + whoName(t) + "[" + t.Address + "] 지금 폴더 모름"
			return
		case !found || gt.State != "live":
		case g.intree(gt.NowCwd):
			g.who += ", " + whoName(t) + "[" + t.Address + "]"
			return
		}
	}
}

func whoName(t hcjson.Terminal) string {
	if t.Name == "" {
		return t.Agent
	}
	return t.Name
}

// hcTerminals: hc ls --json 응답의 산(live) 터미널(hcjson.ParseList — A5, 2026-11-23 cwd-now-inuse로 판독을 한 곳에). 완전한
// 목록만 읽는다 — 목록을 못 읽거나 이 기계 항목 하나라도 계약을 어기면(hcjson Unknown) 그 까닭(빈 값이 아님 = 모름. 재감사 R-H02:
// 빈 성공·terminals 없음·null·주소 없는 live를 「hc 에이전트 없음」으로 읽어 미커밋 파일을 넘겼다)
func hcTerminals(out string) ([]hcjson.Terminal, string) {
	l, err := hcjson.ParseList([]byte(out))
	if err != nil {
		return nil, err.Error()
	}
	var terms []hcjson.Terminal
	for _, t := range l.Terminals {
		if t.Unknown != "" && !strings.Contains(t.Address, "/") {
			return nil, t.Unknown
		}
		if t.Unknown == "" && t.State == "live" {
			terms = append(terms, t)
		}
	}
	return terms, ""
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
			if patLocale == locOther { // 로캘을 모르면 더 넓게 맞춰 덜 막지 않는다(R-M12)
				prefix = widePat(prefix)
			}
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
