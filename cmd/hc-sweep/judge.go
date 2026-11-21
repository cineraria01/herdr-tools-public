package main

// 터미널 쓸기: 규칙(R1·R2·R3)·보호·닫기 — 셸 판 lib/hc-sweep/sweep.py와 같다

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func utf8Valid(b []byte) bool { return utf8.Valid(b) }

// tri: 파이썬 판의 True·False·키 없음
type tri int

const (
	missing tri = iota
	yes
	no
)

func triOf(b bool) tri {
	if b {
		return yes
	}
	return no
}

// facts: hc get 밖에서 모은 판정 재료(파이썬 판은 같은 dict에 넣었다)
type facts struct {
	worktree, unlanded tri
	tail               *string
	fill               string
	submitted          bool
	wtwhy              string
}

// isWorker: 지휘가 띄운 worker(parent 있음) 또는 hc-lane 레인(라벨 hc-lane=1 — hc-fill 레인은 parent가 없다)
func isWorker(t map[string]any) bool {
	return truthy(t["parent"]) || str(obj(t, "labels"), "hc-lane") == "1"
}

// judge: (닫을 규칙 또는 "", 남기는 까닭 — --dry-run이 찍는다)
func judge(t map[string]any, f facts, now float64, keep map[string]bool) (string, string) {
	status, isObj := t["status"].(map[string]any)
	name, labels, worker := str(t, "name"), obj(t, "labels"), isWorker(t)
	st := str(status, "state")
	guards := []struct {
		bad bool
		why string
	}{
		{str(t, "state") != "live" || strings.Contains(str(t, "address"), "/"), "live 아님·다른 기계"},
		{!isObj || !truthy(status["app"]), "에이전트 상태 없음"},
		{isObj && st != "idle" && st != "done", "idle·done 아님"},
		{strings.Contains(name, "loop") || strings.HasSuffix(name, "-proxy"), "보호: 이름(loop·-proxy)"},
		{keep[name] || keep[str(t, "address")], "보호: keep 목록"},
		{strings.HasPrefix(name, "lane-watch-") || str(labels, "hc-lane-watch") == "1", "보호: hc-lane 감시"},
		{(truthy(t["where"]) || truthy(labels["where"])) && !worker, "보호: 사람이 연 칸(where)"},
		{truthy(t["shell"]), "보호: 셸 칸"},
	}
	for _, g := range guards {
		if g.bad {
			return "", g.why
		}
	}
	created, changed := now, now
	if v, ok := num(t, "created_ms"); ok && v != 0 {
		created = v
	}
	if v, ok := num(t, "status_ms"); ok && v != 0 {
		changed = v
	}
	turn, turnOK := num(t, "turn")
	if !worker && turnOK && turn == 0 && changed-created < minute && now-created >= 30*minute {
		return "R1", ""
	}
	if f.worktree != no { // 남은 워크트리이거나 모름
		why := "작업 트리 남음"
		if f.wtwhy != "" {
			why += " — " + f.wtwhy
		}
		return "", why
	}
	idle := int((now - changed) / minute)
	why := "worker 아님"
	if worker {
		wait := 30 * minute
		if f.submitted {
			wait = 0
		}
		switch {
		case now-changed < wait:
			why = fmt.Sprintf("쉰 지 %d분(R2 30분 전)", idle)
		case f.unlanded != no:
			why = "착지 전 커밋(git cherry +) 또는 모름"
		case f.fill == "up":
			why = "hc-fill이 결과를 되돌려 주기 전"
		case f.tail == nil || busyRe.MatchString(*f.tail):
			why = "배경 실행 흔적·화면 못 읽음"
		default:
			return "R2", ""
		}
	}
	if now-changed >= 6*60*minute {
		return "R3", ""
	}
	return "", fmt.Sprintf("%s, 쉰 지 %d분(R3 6시간 전)", why, idle)
}

// linkedWorktree: cwd가 아직 있는 링크된 git 워크트리인가(.git이 파일). 없어진 폴더·일반 폴더·메인 저장소면 false.
// 모르면(cwd 없음·읽기 오류) true(닫지 않는 쪽). 파일 검사만 한다(launchd의 git은 TCC에 멈춘다)
func linkedWorktree(cwd string) bool {
	if !strings.HasPrefix(cwd, "/") {
		return true
	}
	if _, err := os.Stat(cwd); err != nil {
		return !os.IsNotExist(err)
	}
	for d := filepath.Clean(cwd); ; d = filepath.Dir(d) {
		st, err := os.Lstat(filepath.Join(d, ".git"))
		if err == nil {
			return !st.IsDir()
		}
		if !os.IsNotExist(err) {
			return true
		}
		if d == "/" {
			return false
		}
	}
}

// fillState: hc-fill 대기열에서 이 이름·주소의 받은편지 레인 상태(up·sent 등). 줄이 없거나 되돌릴 곳(11째 칸)이 없으면 "".
// 파일이 있는데 못 읽으면 "up"(닫지 않는 쪽)
func fillState(name, address, path string) string {
	if path == "" {
		d := os.Getenv("HC_FILL_DATA")
		if d == "" {
			d = filepath.Join(home, ".local", "share", "hc-fill")
		}
		path = filepath.Join(d, "queue.tsv")
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil || !utf8Valid(b) {
		return "up"
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		r := strings.Split(line, "\t")
		if len(r) >= 11 && r[0] == name && r[7] == address && r[10] != "" && r[10] != "-" {
			return r[5]
		}
	}
	return ""
}

// eventRows: hc-lane events.tsv 줄들(칸으로). 못 읽으면 err
func eventRows(path string) ([][]string, error) {
	if path == "" {
		path = filepath.Join(laneDir, "events.tsv")
	}
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	var rows [][]string
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if !utf8Valid(sc.Bytes()) {
			return nil, fmt.Errorf("UTF-8 아님")
		}
		rows = append(rows, strings.Split(sc.Text(), "\t"))
	}
	return rows, sc.Err()
}

// submitted: 그 터미널(주소·이름, 실행 키 칸이 있으면 <주소>@<created_ms>)의 마지막 hc-lane 사건(정리됨 빼고)이 「제출」인가(못 읽으면 false)
func submitted(t map[string]any, path string) bool {
	rows, err := eventRows(path)
	if err != nil {
		return false
	}
	name := dash(str(t, "name"))
	key := str(t, "address") + "@" + pyStr(t["created_ms"])
	last := ""
	for _, r := range rows {
		if len(r) > 4 && r[2] == str(t, "address") && r[4] != "정리됨" && r[3] == name && (len(r) < 10 || r[9] == "-" || r[9] == key) {
			last = r[4]
		}
	}
	return last == "제출"
}

// hcGit: cwd에서 git(hc exec — launchd가 띄운 git은 ~/Documents에서 TCC에 멈춘다)
func hcGit(r runner, cwd string, secs int, args ...string) result {
	a := append([]string{"exec", "--cwd", cwd, "--env", "LC_ALL=C", "--timeout", strconv.Itoa(secs), "--", "git"}, args...)
	return r(time.Duration(secs+20)*time.Second, nil, "hc", a...)
}

// unlanded: cwd 저장소에 origin/main에 없는 커밋이 있나(로컬 origin/main, fetch 안 함). cwd가 없거나 저장소가 아니면 false,
// 그 밖에 판정이 안 되면 true(닫지 않는 쪽)
func unlanded(cwd string, r runner) bool {
	if cwd == "" {
		return false
	}
	res := hcGit(r, cwd, 20, "cherry", "origin/main", "HEAD")
	if res.err != nil {
		return true
	}
	if res.rc == 0 {
		for _, l := range strings.Split(res.out, "\n") {
			if strings.HasPrefix(l, "+") {
				return true
			}
		}
		return false
	}
	return !(strings.Contains(res.errs, "not a git repository") || strings.Contains(res.errs, "No such file or directory"))
}

// screenTail: 화면 마지막 비지 않은 6줄. 못 읽으면 busyRe에 걸리는 값(닫지 않는 쪽)
func screenTail(address string, r runner) string {
	res := r(40*time.Second, nil, "hc", "read", address)
	if res.err != nil || res.rc != 0 {
		return "still running (화면 못 읽음)"
	}
	var keep []string
	for _, l := range strings.Split(res.out, "\n") {
		if strings.TrimSpace(l) != "" {
			keep = append(keep, l)
		}
	}
	if len(keep) > 6 {
		keep = keep[len(keep)-6:]
	}
	return strings.Join(keep, "\n")
}

// landedClean: 링크된 작업 트리(cwd)의 레인이 착지했고 깨끗하면 (메인 저장소, 가지, ""), 아니면 ("", "", 까닭).
// 착지 = git cherry origin/main HEAD에 + 없음 + 착지 증거(이 레인 이름의 hc-lane 사건 착지·제출·완료·판정, 없으면 원격
// refs/hc-landed/<가지>/*). 깨끗함 = status --porcelain --untracked-files=all이 빔. hc-sweep(작업 트리 때문에 터미널을 남김)과
// hc-lane gc(터미널 때문에 작업 트리를 남김)가 서로 기다리던 교착을 푼다(2026-11-21 mini-m6-1 16시간). 모르는 값은 남김
func landedClean(t map[string]any, r runner, events string) (string, string, string) {
	cwd, name := str(t, "cwd"), str(t, "name")
	git := func(secs int, args ...string) (string, error) {
		res := hcGit(r, cwd, secs, args...)
		if res.err != nil || res.rc != 0 {
			return "", fmt.Errorf("git %s 실패", args[0])
		}
		return res.out, nil
	}
	out, err := git(20, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return "", "", "모름(" + err.Error() + ")"
	}
	if strings.TrimSpace(out) != "" {
		return "", "", "커밋 안 된 변경"
	}
	branch, err := git(20, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return "", "", "모름(" + err.Error() + ")"
	}
	common, err := git(20, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", "", "모름(" + err.Error() + ")"
	}
	cherry, err := git(20, "cherry", "origin/main", "HEAD")
	if err != nil {
		return "", "", "모름(" + err.Error() + ")"
	}
	for _, l := range strings.Split(cherry, "\n") {
		if strings.HasPrefix(l, "+") {
			return "", "", "착지 전 커밋(git cherry +)"
		}
	}
	branch, mainRepo := strings.TrimSpace(branch), filepath.Dir(strings.TrimSpace(common))
	rows, err := eventRows(events)
	if err != nil && !os.IsNotExist(err) {
		return "", "", "모름(사건 못 읽음)"
	}
	said := false
	for _, row := range rows {
		if len(row) > 4 && row[3] == name && (row[4] == "착지" || row[4] == "제출" || row[4] == "완료" || row[4] == "판정") {
			said = true
		}
	}
	if !said {
		out, err := git(30, "ls-remote", "origin", "refs/hc-landed/"+branch+"/*")
		if err != nil {
			return "", "", "모름(" + err.Error() + ")"
		}
		if strings.TrimSpace(out) == "" {
			return "", "", "착지 증거 없음(사건·원격 표식)"
		}
	}
	if branch == "" || !strings.HasPrefix(mainRepo, "/") {
		return "", "", "가지·메인 저장소 모름"
	}
	return mainRepo, branch, ""
}

// laneClean: 닫은 레인의 작업 트리·가지를 hc-lane clean으로(teardown → worktree remove → branch -D). (성공, 한 줄)
func laneClean(mainRepo, branch string, r runner) (bool, string) {
	res := r(340*time.Second, nil, "hc", "exec", "--cwd", mainRepo, "--timeout", "300", "--",
		filepath.Join(binDir, "hc-lane"), "clean", "--repo", mainRepo, "--branch", branch)
	if res.err != nil {
		return false, cutRunes(res.err.Error(), 200)
	}
	msg := "-"
	for _, l := range strings.Split(res.errs+res.out, "\n") {
		if strings.TrimSpace(l) != "" {
			msg = l
			break
		}
	}
	return res.rc == 0, cutRunes(msg, 200)
}

// sessionID: pid와 그 자손 가운데 ~/.claude/sessions/<pid>.json이 있는 것의 sessionId
func sessionID(pid int) string {
	children := map[int][]int{}
	res := run(30*time.Second, nil, "ps", "-A", "-o", "pid=,ppid=")
	for _, l := range strings.Split(res.out, "\n") {
		f := strings.Fields(l)
		if len(f) == 2 {
			c, e1 := strconv.Atoi(f[0])
			p, e2 := strconv.Atoi(f[1])
			if e1 == nil && e2 == nil {
				children[p] = append(children[p], c)
			}
		}
	}
	var todo []int
	if pid != 0 {
		todo = []int{pid}
	}
	for len(todo) > 0 {
		p := todo[0]
		todo = todo[1:]
		b, err := os.ReadFile(filepath.Join(sessDir, strconv.Itoa(p)+".json"))
		var m map[string]any
		if err == nil && json.Unmarshal(b, &m) == nil {
			return dash(str(m, "sessionId"))
		}
		todo = append(todo, children[p]...)
	}
	return "-"
}

func keepList() map[string]bool {
	keep := map[string]bool{}
	b, err := os.ReadFile(filepath.Join(tasksDir, "keep"))
	if err != nil {
		return keep
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "#") {
			keep[strings.TrimSpace(l)] = true
		}
	}
	return keep
}

// laneEvent: 레인 터미널(라벨 hc-lane=1)이면 events.tsv에 「정리됨」 한 줄(8칸 — 셸 판과 같음). kill 전에 적는다.
// 레인 아님·적음이면 true, 못 쓰면 false — 그때는 닫지 않는다(닫으면 hc-lane wait가 뜻밖의 종료를 놓친다)
func laneEvent(t map[string]any, why string) bool {
	if str(obj(t, "labels"), "hc-lane") != "1" {
		return true
	}
	machine := os.Getenv("HC_LANE_MACHINE")
	if machine == "" {
		h, _ := os.Hostname()
		machine = dash(strings.SplitN(h, ".", 2)[0])
	}
	turn := "-"
	if t["turn"] != nil {
		turn = pyStr(t["turn"])
	}
	f := []string{time.Now().UTC().Format("2006-01-02T15:04:05Z"), machine, str(t, "address"), dash(str(t, "name")), "정리됨", "-", "hc-sweep " + why, turn}
	for i := range f {
		f[i] = strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(f[i])
	}
	if os.MkdirAll(laneDir, 0o755) != nil {
		return false
	}
	return appendLine(filepath.Join(laneDir, "events.tsv"), strings.Join(f, "\t")) == nil
}

func closeTerm(t map[string]any, why string) string {
	address, name, cwd, title := str(t, "address"), str(t, "name"), str(t, "cwd"), str(t, "title")
	pid, _ := num(t, "pid")
	if err := record(address, name, why, cwd, sessionID(int(pid)), title); err != nil {
		return "실패(closed.log 못 씀)" // 먼저: closed.log를 못 쓰면 정리됨도 안 남기고 멈춘다
	}
	if !laneEvent(t, why) {
		record(address, name, why+" 실패(정리됨 못 씀)", cwd, "-", title)
		return "실패(정리됨 못 씀)"
	}
	if _, err := hc("kill", address); err != nil {
		record(address, name, why+" 실패(kill): "+err.Error(), cwd, "-", title)
		return "실패(kill)"
	}
	finished := false
	for i := 0; i < 30 && !finished; i++ {
		g, err := hc("get", address)
		if err != nil {
			return "닫힘(이미 지워짐)" // 감싸는 실행기가 끝날 때 자기 터미널을 지우기도 한다
		}
		if finished = str(g, "state") == "finished"; !finished {
			time.Sleep(time.Second)
		}
	}
	if !finished {
		record(address, name, why+" 실패(30초 안에 안 끝남)", cwd, "-", title)
		return "실패(안 끝남)"
	}
	if _, err := hc("rm", address); err != nil {
		record(address, name, why+" 실패(rm): "+err.Error(), cwd, "-", title)
		return "실패(rm)"
	}
	return "닫힘"
}

// sweepTerminals: 터미널 쓸기 한 바퀴(요약 한 줄까지)
func sweepTerminals(terms any, dry bool) {
	keep, now := keepList(), float64(time.Now().UnixNano())/1e6
	list, _ := terms.([]any)
	var listed []map[string]any
	for _, x := range list {
		t, ok := x.(map[string]any)
		if ok && str(t, "state") == "live" && str(t, "address") != "" && !strings.Contains(str(t, "address"), "/") &&
			(str(t, "status") == "idle" || str(t, "status") == "done") {
			listed = append(listed, t)
		}
	}
	closed, targets := 0, 0
	for _, brief := range listed {
		t, err := hc("get", str(brief, "address"))
		if err != nil {
			if dry {
				fmt.Printf("남김: %s %s — hc get 실패\n", str(brief, "address"), dash(str(brief, "name")))
			}
			continue
		}
		f := facts{worktree: triOf(linkedWorktree(str(t, "cwd")))}
		var wtRepo, wtBranch string
		sub := submitted(t, "")
		if isWorker(t) && f.worktree == yes && (sub || now-orNow(t, now) >= 30*minute) {
			// 착지한 깨끗한 작업 트리: 닫고 hc-lane clean(교착 풀기)
			if wtRepo, wtBranch, f.wtwhy = landedClean(t, run, ""); wtRepo != "" {
				f.worktree, f.unlanded = no, no
			}
		}
		if isWorker(t) && f.worktree == no { // R2 보호에만 쓴다
			if wtRepo == "" {
				f.unlanded = triOf(unlanded(str(t, "cwd"), run))
			}
			tail := screenTail(str(t, "address"), run)
			f.tail, f.fill, f.submitted = &tail, fillState(str(t, "name"), str(t, "address"), ""), sub
		}
		why, kept := judge(t, f, now, keep)
		if why == "" {
			if dry {
				fmt.Printf("남김: %s %s — %s\n", str(t, "address"), dash(str(t, "name")), kept)
			}
			continue
		}
		created, changed := now, orNow(t, now)
		if v, ok := num(t, "created_ms"); ok && v != 0 {
			created = v
		}
		line := fmt.Sprintf("%s %s %s (만든 지 %d분·상태 그대로 %d분, turn %s) %s", why, str(t, "address"), dash(str(t, "name")),
			int((now-created)/minute), int((now-changed)/minute), pyStr(t["turn"]), pyStr(t["cwd"]))
		targets++
		if dry {
			if wtRepo != "" {
				line += " (닫은 뒤 hc-lane clean " + wtBranch + ")"
			}
			fmt.Println("닫을 것:", line)
			continue
		}
		res := closeTerm(t, why)
		if strings.HasPrefix(res, "닫힘") {
			closed++
		}
		fmt.Println(res+":", line)
		if wtRepo != "" && strings.HasPrefix(res, "닫힘") {
			ok, msg := laneClean(wtRepo, wtBranch, run)
			fail := ""
			if !ok {
				fail = " 실패"
			}
			record(str(t, "address"), str(t, "name"), "작업 트리 정리(hc-lane clean "+wtBranch+")"+fail, wtRepo, "-", msg)
			fmt.Printf("작업 트리 정리%s: %s %s — %s\n", fail, wtRepo, wtBranch, msg)
		}
	}
	word, n := "닫음", closed
	if dry {
		word, n = "닫을 것", targets
	}
	fmt.Printf("%s 이 기계 idle·done %d개 확인, %s %d개\n", time.Now().Format("2006-01-02 15:04:05"), len(listed), word, n)
}

func orNow(t map[string]any, now float64) float64 {
	if v, ok := num(t, "status_ms"); ok && v != 0 {
		return v
	}
	return now
}
