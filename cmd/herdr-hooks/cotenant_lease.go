package main

// cotenant-guard Go 판 — 레지스트리·자기 찾기·산 프로세스·임대(owner·claim·lease)·바뀐 임대 정리 (cotenant.go에서 옮김)

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// regLoad: Claude가 쓰는 세션 레지스트리 — 파일마다 JSON 흐름으로 읽어 「pid sessionId procStart cwd name」 줄
func (g *cg) regLoad() {
	if g.regDone {
		return
	}
	g.regDone = true
	dir := g.cfg + "/sessions"
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range ents {
		if n := e.Name(); !strings.HasPrefix(n, ".") && strings.HasSuffix(n, ".json") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		data, err := os.ReadFile(dir + "/" + n)
		if err != nil {
			continue
		}
		vals, _ := jqValues(data)
		for _, v := range vals {
			if v != nil {
				if _, ok := v.(map[string]any); !ok {
					break
				}
			}
			var f []string
			for _, k := range []string{"pid", "sessionId", "procStart", "cwd", "name"} {
				x := mapGet(v, k)
				if !jqTruthy(x) {
					x = ""
				}
				f = append(f, jqToString(x))
			}
			b.WriteString(strings.Join(f, us) + "\n")
		}
	}
	g.reg = capture(b.String())
}

// me: 이 훅을 돌리는 Claude 프로세스의 MYPID MYSTART MYNAME (훅의 부모, 레지스트리에서 세션 ID로)
func (g *cg) me() {
	if g.meDone {
		return
	}
	g.meDone = true
	g.regLoad()
	ppid := strconv.Itoa(os.Getppid())
	for _, l := range heredocLines(g.reg) {
		f := bashRead(l, 0x1f, 5)
		if f[1] != g.sid {
			continue
		}
		g.mypid, g.mystart, g.myname = f[0], f[2], f[4]
		if f[0] == ppid {
			break
		}
	}
	if g.mypid == "" {
		g.mypid = ppid
		g.mystart = norm(psLstart(ppid))
	}
	if g.myname == "" {
		g.myname = "claude"
	}
	if g.hcTerm != "" {
		g.myname += "[" + g.hcTerm + "]"
	}
}

// psLstart: $(LC_ALL=C TZ=UTC /bin/ps -o lstart= -p PID 2>/dev/null)
func psLstart(pid string) string {
	c := exec.Command("/bin/ps", "-o", "lstart=", "-p", pid)
	c.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	out, _ := c.Output()
	return capture(string(out))
}

// bashKill0: bash 내장 `kill -0 "$1"` — 수 꼴(앞뒤 공백·부호 허용)이 아니면 실패
func bashKill0(p string) bool {
	s := strings.Trim(p, " \t\n\v\f\r")
	if s == "" {
		return false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return false
	}
	return syscall.Kill(int(int32(v)), 0) == nil
}

// alive PID PROCSTART: pid가 있고 기록한 때 시작했다(procStart가 pid 재사용을 막는다)
func (g *cg) alive(pid, st string) bool {
	if pid == "" || !bashKill0(pid) {
		return false
	}
	if g.aok[pid+" "+st] {
		return true
	}
	ls, ok := g.psc[pid]
	if !ok {
		ls = norm(psLstart(pid))
		g.psc[pid] = ls
	}
	if ls == "" {
		return false
	}
	if st == "" {
		return true
	}
	if ls == norm(st) {
		g.aok[pid+" "+st] = true
		return true
	}
	return false
}

func (g *cg) dirty(rel string) bool {
	out, _ := g.gitRun([]string{"GIT_OPTIONAL_LOCKS=0"}, "", "-C", g.root, "status", "--porcelain", "-uall", "--", ":(literal)"+rel)
	return capture(out) != ""
}

// readLine: bash `read -r … < 파일` — 첫 줄과, 줄바꿈으로 끝났는지(아니면 read가 실패하지만 칸은 채운다)
func readLine(path string) (string, bool, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false, false
	}
	s := strings.ReplaceAll(string(b), "\x00", "")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i], true, true
	}
	return s, false, true
}

func (g *cg) setOwner(line string) {
	f := bashRead(line, 0x1f, 7)
	g.oSID, g.oPID, g.oStart, g.oAID, g.oName, g.oSince, g.oPath = f[0], f[1], f[2], f[3], f[4], f[5], f[6]
}

func (g *cg) owner(ld string) bool {
	g.oSID, g.oPID, g.oStart, g.oAID, g.oName, g.oSince, g.oPath = "", "", "", "", "", "", ""
	fi, err := os.Stat(ld + "/owner")
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	line, nl, ok := readLine(ld + "/owner")
	if !ok {
		return false
	}
	g.setOwner(line)
	return nl
}

func (g *cg) mine() bool {
	if g.oPID != g.mypid {
		return false
	}
	if g.oStart == "" || g.oStart == g.mystart {
		return true
	}
	return norm(g.oStart) == norm(g.mystart)
}

func (g *cg) own(ld, rel string) {
	g.me()
	line := strings.Join([]string{g.sid, g.mypid, g.mystart, g.aid, g.myname, time.Now().Format("2006-01-02 15:04:05"), rel}, us) + "\n"
	tmp := ld + "/owner." + strconv.Itoa(os.Getpid())
	if os.WriteFile(tmp, []byte(line), 0o666) == nil {
		os.Rename(tmp, ld+"/owner")
	}
}

// claim: 파일을 배타로 만든다(O_EXCL). 있으면(폴더여도) 실패
func claim(p string) bool {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

func lmtime(p string) (int64, bool) {
	fi, err := os.Lstat(p)
	if err != nil {
		return 0, false
	}
	return fi.ModTime().Unix(), true
}

func smtime(p string) (int64, bool) {
	fi, err := os.Stat(p)
	if err != nil {
		return 0, false
	}
	return fi.ModTime().Unix(), true
}

// touch: /usr/bin/touch — 없으면 만들고, 있으면 시각을 지금으로
func touch(p string) {
	t := time.Now()
	if os.Chtimes(p, t, t) != nil {
		if f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE, 0o666); err == nil {
			f.Close()
		}
	}
}

// claimgen: 주인 한 세대를 잡는다 — 60초 넘은 claim은 남은 찌꺼기라, 먼저 다시 무장(mtime = 지금)한 뒤
// FILE.steal-<묵은 mtime>을 만든 한 쪽만 가진다(셸 판 S3·R1 설명 그대로)
func (g *cg) claimgen(p string) bool {
	if claim(p) {
		return true
	}
	m, ok := lmtime(p)
	if !ok {
		g.claimAge = 0
		return false
	}
	g.claimAge = g.nowT() - m
	if g.claimAge < 60 {
		return false
	}
	if exists(p) {
		if f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666); err == nil {
			f.Close()
		} else {
			touch(p)
		}
	}
	if !claim(p + ".steal-" + strconv.FormatInt(m, 10)) {
		g.claimAge = 0
		return false
	}
	return true
}

func digitsOnly(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func (g *cg) takeover(ld, rel string) {
	c := ld + "/claim-" + g.oPID + "-" + digitsOnly(g.oSince)
	if g.oPID+g.oSince == "" {
		c = ld + "/claim-orphan"
	}
	if !g.claimgen(c) {
		g.block(fmt.Sprintf("%s: 다른 에이전트가 %d초 전 이 파일 임대를 넘겨받기 시작했다. 같은 편집을 곧바로 되풀이하지 말고 다른 일을 먼저 하라. 그 넘겨받기가 끝나지 않았다면 60초가 지나면 저절로 풀린다. 그 뒤에도 막히면 지휘에 알려라.", rel, g.claimAge))
	}
	g.own(ld, rel)
}

func baseName(p string) string { return p[strings.LastIndexByte(p, '/')+1:] }

func (g *cg) checkIgnore(rel string) bool {
	_, rc := g.gitRun(nil, "", "-C", g.root, "check-ignore", "-q", "--", rel)
	return rc == 0
}

// refuseLive: 산 주인은 미커밋 파일을, 깨끗해도 10분 안에 손댄 파일을 지킨다. 무시된 파일은 바로 넘긴다
func (g *cg) refuseLive(ld, rel string) {
	busy := rel + " 은 " + g.oName + "(pid " + g.oPID + ", since " + g.oSince + ")이 고치는 중이다(임대는 그 프로세스가 끝나면 풀린다)"
	out := "다른 일을 먼저 하고 나중에 다시 시도하거나 지휘에 알리거나, 꼭 지금이면 git worktree add -b <슬러그> ../" + baseName(g.root) + ".wt/<슬러그> origin/main 에서 하라."
	if g.dirty(rel) {
		g.block(busy + "(미커밋 변경 있음). " + out)
	}
	if g.checkIgnore(rel) {
		return
	}
	now := g.nowT()
	m, ok := lmtime(ld + "/owner")
	if !ok {
		g.arithFail(ld + "/owner")
	}
	if now-m < 600 {
		g.block(busy + "(마지막 편집 10분 안, 커밋됐거나 아직 안 씀). " + out)
	}
}

func md5hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func (g *cg) addWarn(s string) {
	if g.warn != "" {
		g.warn += " "
	}
	g.warn += s
}

// lease REL MODE — 1(편집 도구)은 트리를 같이 쓸 때 주인 없는 미커밋 파일도 막는다 · 0 = Bash 쓰기 · check = 막기만, 기록 안 함
func (g *cg) lease(rel, mode string) {
	if mode == "check" {
		ld := g.leases + "/" + md5hex(rel)
		if !g.owner(ld) {
			return
		}
		g.me()
		if g.mine() {
			return
		}
		if g.alive(g.oPID, g.oStart) {
			g.refuseLive(ld, rel)
		}
		return
	}
	if !isDir(g.leases) && os.MkdirAll(g.leases, 0o777) != nil {
		return
	}
	ld := g.leases + "/" + md5hex(rel)
	if os.Mkdir(ld, 0o777) == nil {
		if g.dirty(rel) {
			if mode != "1" {
				os.Remove(ld)
				return
			}
			if g.cotenant() {
				os.Remove(ld)
				g.block(rel + ": 임대 기록 없는 미커밋 변경이 있다(같은 작업 트리: " + g.who + "). 누구 것인지 git diff -- '" + rel + "'로 확인하라. 남의 것이면 다른 일을 먼저 하고 나중에 다시 시도하거나 지휘에 알리고, 네 것(Bash로 바꾼 것 등)이면 그 변경을 커밋한 뒤 고치거나 git worktree add -b <슬러그> ../" + baseName(g.root) + ".wt/<슬러그> origin/main 에서 하라.")
			}
		}
		g.own(ld, rel)
		return
	}
	g.me()
	if !g.owner(ld) {
		now := g.nowT()
		m, ok := lmtime(ld)
		if !ok {
			g.arithFail(ld)
		}
		if now-m < 10 {
			g.block(rel + ": 다른 에이전트가 지금 이 파일 임대를 잡는 중이다. 잠시 뒤 다시 시도하라.")
		}
		g.takeover(ld, rel)
		return
	}
	if g.mine() {
		touch(ld + "/owner")
		if g.oAID != g.aid {
			aid := g.oAID
			if aid == "" {
				aid = "메인"
			}
			g.addWarn(rel + " 은 같은 세션의 다른 에이전트(" + aid + ")가 먼저 고친 파일이다. 서로 덮어쓰지 않게 파일 소유를 정하라.")
		}
		return
	}
	if g.alive(g.oPID, g.oStart) {
		if g.oSID == g.sid {
			g.addWarn("같은 세션 " + g.sid + " 를 다른 프로세스(pid " + g.oPID + ")도 돌리고 있다(중복 재개). 하나는 즉시 끝내라.")
		}
		g.refuseLive(ld, rel)
		g.takeover(ld, rel)
		return
	}
	g.takeover(ld, rel)
	if g.oSID != g.sid && g.dirty(rel) {
		g.addWarn(rel + " 에 끝난 에이전트 " + g.oName + "(pid " + g.oPID + ", since " + g.oSince + ")의 미커밋 변경이 남아 있다. 고치기 전에 git diff -- '" + rel + "'을 먼저 보라.")
	}
}

// statusZ: git status --porcelain -z -uall [-- 경로…] 의 출력, 실패면 ok=false
func (g *cg) statusZ(paths []string) (string, bool) {
	args := []string{"-C", g.root, "status", "--porcelain", "-z", "-uall"}
	if paths != nil {
		args = append(args, "--")
		args = append(args, paths...)
	}
	out, rc := g.gitRun([]string{"GIT_OPTIONAL_LOCKS=0"}, "", args...)
	return out, rc == 0
}

// changedLeases: FDIRTY를 바뀐 경로와 그 위 폴더의 임대에서만 — 트리 status 한 번
func (g *cg) changedLeases() {
	out, ok := g.statusZ(nil)
	if !ok {
		out += "\x01\x00"
	}
	recs := strings.Split(out, "\x00")
	if recs[len(recs)-1] == "" {
		recs = recs[:len(recs)-1]
	}
	var lines []string
	skip, src := false, false
	seen := map[string]bool{}
	for _, r := range recs {
		if r == "\x01" {
			lines = append(lines, "\x01")
			continue
		}
		if skip {
			skip = false
			continue
		}
		p := r
		if src {
			src = false
		} else {
			st := r
			if len(st) > 2 {
				st = st[:2]
			}
			p = ""
			if len(r) >= 3 {
				p = r[3:]
			}
			src, skip = strings.HasPrefix(st, "R"), strings.HasPrefix(st, "C")
		}
		for ; p != ""; p = parentStrip(p) {
			if seen[p] {
				continue
			}
			seen[p] = true
			b, err := os.ReadFile(g.leases + "/" + md5hex(p) + "/owner")
			if err != nil {
				continue
			}
			l := string(b)
			if i := strings.IndexByte(l, '\n'); i >= 0 {
				l = l[:i]
			}
			lines = append(lines, l)
		}
	}
	var fd []string
	for _, l := range lines {
		g.setOwner(l)
		if g.oSID == "\x01" {
			g.block("git status가 실패했다(" + g.root + "). 남의 미커밋 파일을 판정할 수 없어 막았다. 색인·권한을 사람에게 알려라.")
		}
		if g.oPID == g.mypid && g.mine() {
			continue
		}
		if g.alive(g.oPID, g.oStart) {
			fd = append(fd, g.oPath+us+g.oName+us+g.oPID+us+g.oSince)
		}
	}
	g.fdirty = ""
	if len(fd) > 0 {
		g.fdirty = strings.Join(fd, "\n") + "\n"
	}
}

var parentRe = regexp.MustCompile(`/?[^/]*\z`)

// parentStrip: perl s{/?[^/]*$}{}
func parentStrip(p string) string {
	if loc := parentRe.FindStringIndex(p); loc != nil {
		return p[:loc[0]] + p[loc[1]:]
	}
	return p
}

// rmF: /bin/rm -f — 폴더는 지우지 않는다
func rmF(p string) {
	if fi, err := os.Lstat(p); err == nil && fi.IsDir() {
		return
	}
	os.Remove(p)
}

// olderThan: bash 3.2 `[ f1 -ot f2 ]` — 초 단위, f2가 없으면 거짓, f1만 없으면 참
func olderThan(f1, f2 string) bool {
	m2, ok := smtime(f2)
	if !ok {
		return false
	}
	m1, ok := smtime(f1)
	if !ok {
		return true
	}
	return m1 < m2
}
