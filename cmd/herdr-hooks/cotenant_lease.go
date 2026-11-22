package main

// cotenant-guard Go 판 — 레지스트리·자기 찾기·산 프로세스·임대(owner·claim·lease)·바뀐 임대 정리 (cotenant.go에서 옮김)

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"herdr-tools/internal/atomic"
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
		st, known := psLstart(ppid)
		if !known {
			g.block("이 세션 프로세스(pid " + ppid + ")의 시작 시각을 ps로 읽지 못했다(실패·시간 넘김). 임대 주인을 판정할 수 없어 막았다. 잠시 뒤 다시 하라.")
		}
		g.mypid, g.mystart = ppid, norm(st)
	}
	if g.myname == "" {
		g.myname = "claude"
	}
	if g.hcTerm != "" {
		g.myname += "[" + g.hcTerm + "]"
	}
}

// psLstart: $(LC_ALL=C TZ=UTC /bin/ps -o lstart= -p PID) — 훅 기한 안(hookexec.go). known=false면 모름(ps 실패·시간 넘김·
// 못 띄움). 그 pid가 없으면("", true) — ps 종료 1·빈 출력이고 kill -0도 실패할 때만(아스트라 E05: ps 실패를 「죽음」으로 읽었다)
func psLstart(pid string) (string, bool) {
	out, _, rc := hookRun([]string{"ps"}, 0, append(os.Environ(), "LC_ALL=C", "TZ=UTC"), "", "/bin/ps", "-o", "lstart=", "-p", pid)
	s := capture(out)
	switch {
	case rc == 0 && s != "":
		return s, true
	case rc == 1 && s == "" && !bashKill0(pid):
		return "", true
	}
	return "", false
}

// bashKill0: bash 내장 `kill -0 "$1"` — 수 꼴(앞뒤 공백·부호 허용)이 아니면 실패. 1 이하(무리·모든 프로세스 신호)는 프로세스가 아니라 실패
func bashKill0(p string) bool {
	s := strings.Trim(p, " \t\n\v\f\r")
	if s == "" {
		return false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || int32(v) <= 1 {
		return false
	}
	pr, err := os.FindProcess(int(int32(v)))
	return err == nil && pr.Signal(syscall.Signal(0)) == nil
}

// psUnknown: psc 캐시의 「ps로 못 읽음」
const psUnknown = "\x01"

// alive PID PROCSTART: pid가 있고 기록한 때 시작했다(procStart가 pid 재사용을 막는다). ps를 못 읽으면 산 것으로 친다(임대를 지킨다)
func (g *cg) alive(pid, st string) bool {
	if pid == "" || !bashKill0(pid) {
		return false
	}
	if g.aok[pid+" "+st] {
		return true
	}
	ls, ok := g.psc[pid]
	if !ok {
		s, known := psLstart(pid)
		ls = norm(s)
		if !known {
			ls = psUnknown
		}
		g.psc[pid] = ls
	}
	if ls == psUnknown {
		return true
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

// dirty: rel에 미커밋 변경이 있는가. git status가 실패·시간 넘김이면 막는다(모름 — 「깨끗함」으로 읽으면 산 주인의 미커밋 파일을
// 넘겨줬다: 아스트라 H01·Fable E1·M2). 이번에 막 만든 임대 폴더는 lease가 거둔다
func (g *cg) dirty(rel string) bool {
	out, rc := g.gitRun([]string{"GIT_OPTIONAL_LOCKS=0"}, "", "-C", g.root, "status", "--porcelain", "-uall", "--", ":(literal)"+rel)
	if rc != 0 {
		g.gitFail(rel+" 의 미커밋 여부(git status)", rc)
	}
	return capture(out) != ""
}

// shq: 셸 낱말 하나로 작은따옴표 인용 — 안내문의 명령을 그대로 붙여 써도 되게(아스트라 E10)
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

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

// own: 임대 주인 줄을 원자로 쓴다 — 쓴 줄을 돌려준다(되돌릴 때 세대 확인용)
func (g *cg) own(ld, rel string) string {
	g.me()
	line := strings.Join([]string{g.sid, g.mypid, g.mystart, g.aid, g.myname, time.Now().Format("2006-01-02 15:04:05"), rel}, us) + "\n"
	// 파일 fsync·닫기·rename·폴더 fsync의 오류까지 본다 — 디스크에 남기 전에 편집을 허용하지 않게(재감사 R-M03)
	if err := atomic.WriteFile(ld+"/owner", []byte(line), 0o666); err != nil { // 임대를 남기지 못한 채 허용하면 다음 에이전트가 이 편집을 모른다(아스트라 E06 — 셸 판은 통과)
		var ce *atomic.CommitError
		if errors.As(err, &ce) { // 주인 줄은 이미 보인다 — 같은 주인의 재시도는 lease의 mine 갈래에서 폴더 동기화를 마친 뒤 허용(R-M202)
			g.block(rel + ": 임대를 바꿔 넣었지만 폴더 동기화를 마치지 못했다(" + err.Error() + "). 다시 시도하면 동기화를 마친 뒤 이어 간다. 계속 막히면 디스크·권한을 사람에게 알려라.")
		}
		g.block(rel + ": 임대를 기록하지 못했다(" + err.Error() + "). 판정을 남길 수 없어 막았다. 디스크·권한을 사람에게 알려라.")
	}
	return line
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

func digitsOnly(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
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

// firstOK: 주인 없는 임대를 잡기 전의 검사 — 미커밋 파일이면 Bash 쓰기(0)는 기록 없이 통과(false), 편집 도구(1)는 같은 트리에
// 다른 에이전트가 있으면 막는다(누구 것인지 모른다)
func (g *cg) firstOK(rel, mode string) bool {
	if !g.dirty(rel) {
		return true
	}
	if mode != "1" {
		return false
	}
	if g.cotenant() {
		g.block(rel + ": 임대 기록 없는 미커밋 변경이 있다(같은 작업 트리: " + g.who + "). 누구 것인지 git diff -- " + shq(rel) + "로 확인하라. 남의 것이면 다른 일을 먼저 하고 나중에 다시 시도하거나 지휘에 알리고, 네 것(Bash로 바꾼 것 등)이면 그 변경을 커밋한 뒤 고치거나 git worktree add -b <슬러그> ../" + baseName(g.root) + ".wt/<슬러그> origin/main 에서 하라.")
	}
	return true
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
	if !isDir(g.leases) {
		if err := os.MkdirAll(g.leases, 0o777); err != nil { // 임대 없이 허용하지 않는다(아스트라 E06 — 셸 판은 통과)
			g.block(rel + ": 임대 폴더를 만들지 못했다(" + err.Error() + "). 판정을 남길 수 없어 막았다. 디스크·권한을 사람에게 알려라.")
		}
	}
	ld := g.leases + "/" + md5hex(rel)
	if err := os.Mkdir(ld, 0o777); err == nil {
		// 이번 실행이 만든 빈 폴더 — 주인을 적기 전에 막히거나(git·ps 실패, 동거) 그냥 통과하면 거둔다(재감사 R-H06:
		// 남은 빈 폴더를 10초 뒤 재시도가 넘겨받기로 읽어 아래 검사를 건너뛰었다). os.Remove는 빈 폴더만 지운다
		kept := false
		defer func() {
			if !kept {
				os.Remove(ld) //nolint:errcheck // 그 사이 남이 넣었으면 남긴다
			}
		}()
		if !g.firstOK(rel, mode) {
			return
		}
		g.made = append(g.made, madeLease{ld: ld, line: g.own(ld, rel), newDir: true})
		kept = true
		return
	} else if !errors.Is(err, fs.ErrExist) { // 임대 없이 허용하지 않는다(아스트라 E06) — 「있음」으로 읽어 시각 오류로 빠지지 않게(R-M06)
		g.block(rel + ": 임대 폴더를 만들지 못했다(" + err.Error() + "). 판정을 남길 수 없어 막았다. 디스크·권한을 사람에게 알려라.")
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
		// 주인을 적기 전에 멈춘 첫 획득(훅이 죽음) — 첫 획득의 안전 검사를 다시 한 뒤에만 넘겨받는다(R-H06)
		if !g.firstOK(rel, mode) {
			return
		}
		g.takeover(ld, rel)
		return
	}
	if g.mine() {
		// R-M202: 앞 호출이 rename 뒤 폴더 동기화에서 막혔으면(*atomic.CommitError) 주인 줄은 이미 자기 것 — 그 동기화를 마친 뒤에만 허용.
		// ponytail: 표식 없이 자기 임대 재편집마다 폴더 fsync 한 번 — 느려지면 CommitError 표식을 둔다
		if err := atomic.SyncDir(ld); err != nil {
			g.block(rel + ": 자기 임대의 폴더 동기화를 마치지 못했다(" + err.Error() + "). 판정을 남길 수 없어 막았다. 디스크·권한을 사람에게 알려라.")
		}
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
	left := g.oSID != g.sid && g.dirty(rel) // 넘겨받기 전에 — git을 못 읽으면 임대를 그대로 두고 막는다
	g.takeover(ld, rel)
	if left {
		g.addWarn(rel + " 에 끝난 에이전트 " + g.oName + "(pid " + g.oPID + ", since " + g.oSince + ")의 미커밋 변경이 남아 있다. 고치기 전에 git diff -- " + shq(rel) + "을 먼저 보라.")
	}
}

// statusZ: git status --porcelain -z -uall [-- 경로…] 의 바뀐 경로들 — 이름 바꿈(R)은 원래 이름도(복사 C의 원본은 그대로라 뺀다).
// 실패면 ok=false: 종료 0이 아니거나, 레코드가 온전하지 않다(재감사 R-H05: 이름 바꿈의 원래 이름 레코드만 빠진 성공 응답을 읽어
// 산 주인의 임대를 지웠다 — 실패 표식을 끝에 붙이던 셸 판 꼴의 잘림은 아스트라 E02)
func (g *cg) statusZ(paths []string) ([]string, bool) {
	args := []string{"-C", g.root, "status", "--porcelain", "-z", "-uall"}
	if paths != nil {
		args = append(args, "--")
		args = append(args, paths...)
	}
	out, rc := g.gitRun([]string{"GIT_OPTIONAL_LOCKS=0"}, "", args...)
	if rc != 0 {
		return nil, false
	}
	return parseStatusZ(out)
}

// parseStatusZ: 레코드마다 「XY 경로」(X·Y는 상태 글자, 셋째는 공백, 경로는 비지 않음), X나 Y가 R·C면 바로 뒤에 비지 않은 원래
// 경로 레코드, 마지막은 NUL로 끝난다 — 하나라도 어긋나면 ok=false
func parseStatusZ(out string) ([]string, bool) {
	if out == "" {
		return nil, true
	}
	if !strings.HasSuffix(out, "\x00") {
		return nil, false
	}
	recs := strings.Split(out[:len(out)-1], "\x00")
	st := func(b byte) bool { return strings.IndexByte(" MTADRCU?!", b) >= 0 }
	var paths []string
	for i := 0; i < len(recs); i++ {
		r := recs[i]
		if len(r) < 4 || r[2] != ' ' || !st(r[0]) || !st(r[1]) {
			return nil, false
		}
		paths = append(paths, r[3:])
		if x, y := r[0], r[1]; x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			i++
			if i >= len(recs) || recs[i] == "" {
				return nil, false
			}
			if x == 'R' || y == 'R' {
				paths = append(paths, recs[i])
			}
		}
	}
	return paths, true
}

// changedLeases: FDIRTY를 바뀐 경로와 그 위 폴더의 임대에서만 — 트리 status 한 번
func (g *cg) changedLeases() {
	changed, ok := g.statusZ(nil)
	if !ok {
		g.block("git status가 실패했다(" + g.root + "). 남의 미커밋 파일을 판정할 수 없어 막았다. 색인·권한을 사람에게 알려라.")
	}
	var lines []string
	seen := map[string]bool{}
	for _, p := range changed {
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
