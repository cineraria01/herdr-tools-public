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

	"herdr-tools/internal/gitx"
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
	detached           bool // 분리·깨끗한 읽기 전용 작업 트리(detachedClean) — worker가 아니어도 R2로 본다
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
	// 착지로 끝난 keep 셸 레인: hc keep이 켜진 레인 터미널은 에이전트가 끝나면 셸로 바뀌어 status 없이 live로 남는다(2026-11-21
	// mini-m4 land-claim-net 13시간). 레인이 제출·착지로 끝났고 작업 트리가 없으면 상태 없음 보호를 풀어 R2로 본다. 사람 keep 목록·
	// 이름 보호·착지 전 커밋은 아래 그대로 막는다
	endedKeep := t["status"] == nil && t["keep"] == true && worker && f.worktree == no && f.submitted &&
		shellName(str(obj(t, "foreground"), "name"))
	guards := []struct {
		bad bool
		why string
	}{
		{str(t, "state") != "live" || strings.Contains(str(t, "address"), "/"), "live 아님·다른 기계"},
		{(!isObj || !truthy(status["app"])) && !endedKeep, "에이전트 상태 없음"},
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
	if worker || f.detached {
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

// shellName: 전경 프로그램이 셸인가(hc keep이 잃은 프로그램 대신 띄우는 것)
func shellName(n string) bool {
	switch strings.TrimPrefix(filepath.Base(n), "-") {
	case "zsh", "bash", "sh", "fish", "dash", "ksh":
		return true
	}
	return false
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

// submitted: 그 터미널(주소·이름, 실행 키 칸이 있으면 <주소>@<created_ms>)의 마지막 hc-lane 사건(정리됨 빼고)이 「제출」·「착지」인가
// (레인이 제 손을 떠남 — 못 읽으면 false)
func submitted(t map[string]any, path string) bool {
	rows, err := eventRows(path)
	if err != nil {
		return false
	}
	name := dash(str(t, "name"))
	last := ""
	for _, r := range rows { // 옛 꼴(실행 키 없음) 줄도 thisRun으로 이 실행에 묶는다(R-01 — 2020년 제출 줄을 새 실행에 쓰지 않게)
		if len(r) > 4 && r[2] == str(t, "address") && r[4] != "정리됨" && r[3] == name && thisRun(t, r) {
			last = r[4]
		}
	}
	return last == "제출" || last == "착지"
}

// thisRun: 사건 줄이 이 터미널의 이 실행 것인가(B03 — 이름은 다시 쓰인다). 실행 키 칸(10째)이 있으면 <주소>@<created_ms>와 같아야
// 하고, 없거나 「-」인 옛 꼴이면 같은 주소이고 그 초가 이 터미널이 생긴 초보다 뒤여야 한다(같은 초는 앞뒤 모호 — R-01). 모르면 false(증거 아님)
func thisRun(t map[string]any, row []string) bool {
	created, ok := num(t, "created_ms")
	if !ok || created <= 0 {
		return false
	}
	if len(row) >= 10 && row[9] != "-" {
		return row[9] == str(t, "address")+"@"+pyStr(t["created_ms"])
	}
	at, err := time.Parse("2006-01-02T15:04:05Z", row[0])
	return err == nil && len(row) > 2 && row[2] == str(t, "address") && afterCreated(at.Unix(), created)
}

// afterCreated: 초 단위 시각 sec이 created_ms가 든 초보다 뒤인가(같은 초면 앞뒤를 모른다)
func afterCreated(sec int64, created float64) bool {
	return created > 0 && sec > int64(created)/1000
}

// hcGit: cwd에서 git(hc exec — launchd가 띄운 git은 ~/Documents에서 TCC에 멈춘다)
func hcGit(r runner, cwd string, secs int, args ...string) result {
	a := append([]string{"exec", "--cwd", cwd, "--env", "LC_ALL=C", "--timeout", strconv.Itoa(secs), "--", "git"}, args...)
	return r(time.Duration(secs+20)*time.Second, nil, "hc", a...)
}

// unlanded: cwd 저장소에 origin/main에 없는 커밋이 있나(로컬 origin/main, fetch 안 함). cwd 폴더가 없거나(파일로 확인) 저장소가
// 아니면 false, 그 밖에 판정이 안 되면 true(닫지 않는 쪽). 폴더가 있는데 git이 다른 까닭으로 실패하면(문구에 No such file이 들어도) 모름(E07)
func unlanded(cwd string, r runner) bool {
	if cwd == "" {
		return false
	}
	if _, err := os.Stat(cwd); os.IsNotExist(err) {
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
	return !strings.Contains(res.errs, "not a git repository")
}

// screenTail: 화면 마지막 비지 않은 6줄. 못 읽거나 빈 화면이면(읽었다는 근거가 없다 — E08) busyRe에 걸리는 값(닫지 않는 쪽)
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
	if len(keep) == 0 {
		return "still running (빈 화면 — 모름)"
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
	// cherry는 병합 커밋을 보지 않는다(R-201): main에 없는 병합이 HEAD에 있으면 그 충돌 풀이가 main에 들었는지 패치로는 모른다 —
	// 사건·패치 같음은 증거가 아니고, HEAD 자체가 main에 든 표식 커밋의 조상일 때만 착지다
	merges, err := git(20, "rev-list", "--merges", "-n", "1", "origin/main..HEAD")
	if err != nil {
		return "", "", "모름(" + err.Error() + ")"
	}
	merges = strings.TrimSpace(merges)
	if merges != "" && !gitx.ObjID(merges) { // 커밋 이름 하나도 빈 출력도 아닌 꼴 = 모름
		return "", "", "모름(git rev-list 출력 꼴)"
	}
	merged := merges != ""
	branch, mainRepo := strings.TrimSpace(branch), filepath.Dir(strings.TrimSpace(common))
	rows, err := eventRows(events)
	if err != nil && !os.IsNotExist(err) {
		return "", "", "모름(사건 못 읽음)"
	}
	said := false
	for _, row := range rows {
		if len(row) > 4 && row[3] == name && (row[4] == "착지" || row[4] == "제출" || row[4] == "완료" || row[4] == "판정") && thisRun(t, row) && !merged {
			said = true
		}
	}
	if !said { // 원격 표식 refs/hc-landed/<가지>/<UTC 초>: 이 터미널이 생긴 뒤 것만(같은 이름의 옛 레인 표식은 증거 아님 — B03)
		out, err := git(30, "ls-remote", "origin", "refs/hc-landed/"+branch+"/*")
		if err != nil {
			return "", "", "모름(" + err.Error() + ")"
		}
		created, _ := num(t, "created_ms")
		for _, l := range strings.Split(out, "\n") {
			f := strings.Fields(l)
			if len(f) != 2 || !strings.HasPrefix(f[1], "refs/hc-landed/"+branch+"/") {
				continue
			}
			s, err := strconv.ParseInt(strings.TrimPrefix(f[1], "refs/hc-landed/"+branch+"/"), 10, 64)
			if err != nil || !afterCreated(s, created) {
				continue
			}
			// 표식 커밋 C(병합기가 main에 넣은 후보)가 origin/main에 들었고 이 가지의 변경을 담았을 때만 증거(R-01 — 관계없는 커밋의
			// 표식 거름). 병합기는 rebase하니 HEAD가 C의 조상이 아닐 수 있다 — 패치로(cherry C HEAD에 + 없음) 본다
			anc := hcGit(r, cwd, 20, "merge-base", "--is-ancestor", f[0], "origin/main")
			if anc.err != nil || (anc.rc != 0 && anc.rc != 1) {
				return "", "", "모름(git merge-base 실패)"
			}
			if anc.rc == 1 {
				continue
			}
			head := hcGit(r, cwd, 20, "merge-base", "--is-ancestor", "HEAD", f[0])
			if head.err != nil || (head.rc != 0 && head.rc != 1) {
				return "", "", "모름(git merge-base 실패)"
			}
			if head.rc == 0 { // HEAD가 그대로 main에 들었다
				said = true
				continue
			}
			if merged { // 재작성된 병합: 결과 보존을 입증할 자료가 없다
				continue
			}
			in, err := git(20, "cherry", f[0], "HEAD")
			if err != nil {
				return "", "", "모름(" + err.Error() + ")"
			}
			if !strings.Contains("\n"+in, "\n+") {
				said = true
			}
		}
		if !said {
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

// keepList: ~/.claude-tasks/keep의 이름·주소. 파일이 없으면 빈 목록, 있는데 못 읽으면 ok=false(그 바퀴는 닫지 않는다 — E12)
func keepList() (map[string]bool, bool) {
	keep := map[string]bool{}
	b, err := os.ReadFile(filepath.Join(tasksDir, "keep"))
	if os.IsNotExist(err) {
		return keep, true
	}
	if err != nil {
		return nil, false
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "#") {
			keep[strings.TrimSpace(l)] = true
		}
	}
	return keep, true
}

// sameRun: 닫기 직전 다시 본 g가 판정한 t와 같은 실행이고 상태가 그대로인가(B04 — 주소는 다시 쓰인다). 이름·created_ms·status_ms·
// state live·에이전트 상태 idle·done이 같아야 한다. 이 확인과 hc kill 사이의 재사용은 코어가 실행 조건 kill을 주기 전에는 못 막는다(모델 밖)
func sameRun(t, g map[string]any) bool {
	c1, ok1 := num(t, "created_ms")
	c2, ok2 := num(g, "created_ms")
	s1, _ := num(t, "status_ms")
	s2, _ := num(g, "status_ms")
	st := str(obj(g, "status"), "state")
	return ok1 && ok2 && c1 == c2 && s1 == s2 && str(g, "name") == str(t, "name") && str(g, "address") == str(t, "address") &&
		str(g, "state") == "live" && (st == "idle" || st == "done")
}

// laneEvent: 레인 터미널(라벨 hc-lane=1)이면 events.tsv에 「정리됨」 한 줄(8칸 — 셸 판과 같음)을 적는 함수를 준다. kill 전에 파일을
// 열어 두고(못 열면 ok=false — 닫지 않는다: 닫으면 hc-lane wait가 뜻밖의 종료를 놓친다), 끝남을 확인한 뒤에만 적는다(R-36 — kill이
// 실패했는데 정리됨이 남으면 wait가 그 줄을 끝남으로 소비한다). 레인 아님이면 아무것도 안 하는 함수. 첫 부름만 듣는다:
// done(true)는 적고 닫고, done(false)는 적지 않고 닫는다(그 뒤 부름은 true)
func laneEvent(t map[string]any, why string) (done func(write bool) bool, ok bool) {
	if str(obj(t, "labels"), "hc-lane") != "1" {
		return func(bool) bool { return true }, true
	}
	if os.MkdirAll(laneDir, 0o755) != nil {
		return nil, false
	}
	fh, err := os.OpenFile(filepath.Join(laneDir, "events.tsv"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, false
	}
	return func(write bool) bool {
		if fh == nil {
			return true
		}
		f := fh
		fh = nil
		if !write {
			return f.Close() == nil
		}
		_, err := f.WriteString(laneEventLine(t, why) + "\n")
		return f.Close() == nil && err == nil
	}, true
}

// laneEventLine: 「정리됨」 사건 한 줄(칸 안 탭·줄바꿈은 빈칸으로)
func laneEventLine(t map[string]any, why string) string {
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
	return strings.Join(f, "\t")
}

// sameOwner: g가 t와 같은 실행(이름·created_ms가 같음)의 행인가 — 주소는 다시 쓰인다(R-02). 칸이 없으면 모름(false)
func sameOwner(t, g map[string]any) bool {
	c1, ok1 := num(t, "created_ms")
	c2, ok2 := num(g, "created_ms")
	return ok1 && ok2 && c1 == c2 && str(g, "name") == str(t, "name")
}

// closeTerm: 닫기. 「닫힘…」이면 닫힌 것이 확인됨(그때만 부른 쪽이 작업 트리를 정리한다), 「남김…」이면 손대지 않음, 그 밖은 실패·모름
func closeTerm(t map[string]any, why string) string {
	address, name, cwd, title := str(t, "address"), str(t, "name"), str(t, "cwd"), str(t, "title")
	pid, _ := num(t, "pid")
	sid := sessionID(int(pid)) // 느린 조회(ps)는 마지막 확인 앞에서 끝낸다(R-02 — 그동안 주소가 바뀔 수 있다)
	event, ok := laneEvent(t, why)
	if !ok {
		record(address, name, why+" 실패(정리됨 못 씀)", cwd, "-", title)
		return "실패(정리됨 못 씀)"
	}
	defer event(false) // 끝남을 확인하지 못한 길: 적지 않고 닫는다
	if g, err := hc("get", address); err != nil || !sameRun(t, g) {
		return "남김(닫기 직전 다시 보니 실행·상태가 바뀜 또는 모름)"
	}
	if err := record(address, name, why, cwd, sid, title); err != nil { // 종료 의도(정리됨은 끝남을 확인한 뒤)
		return "실패(closed.log 못 씀)"
	}
	if _, err := hc("kill", address); err != nil {
		record(address, name, why+" 실패(kill): "+err.Error(), cwd, "-", title)
		return "실패(kill)"
	}
	finished := false
	for i := 0; i < 30 && !finished; i++ {
		g, err := hc("get", address)
		if isNotFound(err) {
			if !event(true) {
				record(address, name, why+" 실패(정리됨 못 씀)", cwd, "-", title)
				return "실패(정리됨 못 씀)"
			}
			return "닫힘(이미 지워짐)" // 감싸는 실행기가 끝날 때 자기 터미널을 지우기도 한다
		}
		if err != nil { // 망·시간 초과·모르는 답: 닫혔는지 모른다 — 작업 트리 정리로 가지 않는다(B05). 다음 바퀴에 다시 본다
			record(address, name, why+" 모름(kill 뒤 hc get 실패): "+err.Error(), cwd, "-", title)
			return "모름(kill 뒤 hc get 실패)"
		}
		if !sameOwner(t, g) { // 그 주소가 다른 실행 행이 됨: 우리 것이 끝났는지·그 행을 지워도 되는지 모른다(R-02)
			record(address, name, why+" 모름(kill 뒤 주소가 다른 실행)", cwd, "-", title)
			return "모름(kill 뒤 주소가 다른 실행)"
		}
		if finished = str(g, "state") == "finished"; !finished {
			time.Sleep(time.Second)
		}
	}
	if !finished {
		record(address, name, why+" 실패(30초 안에 안 끝남)", cwd, "-", title)
		return "실패(안 끝남)"
	}
	if !event(true) {
		record(address, name, why+" 실패(정리됨 못 씀)", cwd, "-", title)
		return "실패(정리됨 못 씀)"
	}
	if _, err := hc("rm", address); err != nil {
		record(address, name, why+" 실패(rm): "+err.Error(), cwd, "-", title)
		return "실패(rm)"
	}
	return "닫힘"
}
