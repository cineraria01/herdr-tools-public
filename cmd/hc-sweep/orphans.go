package main

// 고아 쓸기 — 셸 판 lib/hc-sweep/orphans.py와 같다

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/proc"
)

type procInfo struct {
	ppid, age int
	cmd       string
}

// secs: ps etime([[dd-]hh:]mm:ss) → 초
func secs(etime string) (int, error) {
	days, rest := 0, etime
	if i := strings.LastIndex(etime, "-"); i >= 0 {
		d, err := strconv.Atoi(etime[:i])
		if err != nil {
			return 0, err
		}
		days, rest = d, etime[i+1:]
	}
	n := 0
	for _, p := range strings.Split(rest, ":") {
		v, err := strconv.Atoi(p)
		if err != nil {
			return 0, err
		}
		n = n*60 + v
	}
	return n + days*86400, nil
}

// processes: 이 사용자의 프로세스 {pid: (ppid, 나이 초, 명령)}
func processes(r runner) (map[int]procInfo, error) {
	res := r(60*time.Second, nil, "ps", "-x", "-o", "pid=,ppid=,etime=,command=")
	if res.err != nil || res.rc != 0 {
		return nil, fmt.Errorf("ps 실패")
	}
	procs := map[int]procInfo{}
	for _, l := range strings.Split(res.out, "\n") {
		f := strings.Fields(l)
		if len(f) < 3 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		ppid, err1 := strconv.Atoi(f[1])
		age, err2 := secs(f[2])
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("ps 줄을 못 읽음: %s", l)
		}
		cmd := ""
		if len(f) > 3 {
			// 명령 칸은 셋째 칸 뒤 나머지 그대로
			rest := strings.TrimLeft(l, " \t")
			for i := 0; i < 3; i++ {
				rest = strings.TrimLeft(rest[strings.IndexAny(rest, " \t"):], " \t")
			}
			cmd = rest
		}
		procs[pid] = procInfo{ppid, age, cmd}
	}
	return procs, nil
}

// exes: {pid: argv[0]}(ps comm — 맥은 공백 든 경로도 그대로, 리눅스는 잘린 이름이라 "/" 든 것만)
func exes(r runner) (map[int]string, error) {
	res := r(60*time.Second, nil, "ps", "-x", "-o", "pid=,comm=")
	if res.err != nil || res.rc != 0 {
		return nil, fmt.Errorf("ps 실패")
	}
	m := map[int]string{}
	for _, l := range strings.Split(res.out, "\n") {
		f := strings.SplitN(strings.TrimSpace(l), " ", 2)
		if len(f) != 2 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if c := strings.TrimSpace(f[1]); err == nil && strings.Contains(c, "/") {
			m[pid] = c
		}
	}
	return m, nil
}

// cwds: {pid: cwd}. 못 읽은 pid는 빠진다(명령 줄로만 판정)
func cwds(pids []int, r runner) map[int]string {
	m := map[int]string{}
	if len(pids) == 0 {
		return m
	}
	ps := make([]string, len(pids))
	for i, p := range pids {
		ps[i] = strconv.Itoa(p)
	}
	res := r(60*time.Second, nil, lsofPath(), "-a", "-d", "cwd", "-p", strings.Join(ps, ","), "-Fpn")
	if res.err != nil {
		return m
	}
	pid := 0
	for _, l := range strings.Split(res.out, "\n") {
		switch {
		case strings.HasPrefix(l, "p"):
			pid, _ = strconv.Atoi(l[1:])
		case strings.HasPrefix(l, "n") && pid != 0:
			m[pid] = l[1:]
		}
	}
	return m
}

// laneRoot: 절대 경로가 레인 작업 트리 아래면 그 작업 트리(<…><접미사>/<레인>), 아니면 ""
func laneRoot(path, suffix string) string {
	if !strings.HasPrefix(path, "/") {
		return ""
	}
	parts := strings.Split(filepath.Clean(path), "/")
	for i := 0; i < len(parts)-1; i++ {
		if strings.HasSuffix(parts[i], suffix) && len(parts[i]) > len(suffix) {
			return strings.Join(parts[:i+2], "/")
		}
	}
	return ""
}

// exists: 지워졌으면 false. 그 밖 읽기 오류는 true(있다고 보고 터미널로 판정)
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}

// realpath: os.path.realpath처럼 — 있는 앞부분의 링크를 풀고 없는 뒤는 그대로
// (가리키는 곳이 없는 링크도 푼다 — 빠진 디스크 /Volumes로 가는 링크를 알아보게)
func realpath(p string) string { return realpathN(filepath.Clean(p), 40) }

func realpathN(p string, depth int) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	d := filepath.Dir(p)
	if d == p || depth == 0 {
		return p
	}
	q := filepath.Join(realpathN(d, depth-1), filepath.Base(p))
	if st, err := os.Lstat(q); err == nil && st.Mode()&os.ModeSymlink != 0 {
		if t, err := os.Readlink(q); err == nil {
			if !filepath.IsAbs(t) {
				t = filepath.Join(filepath.Dir(q), t)
			}
			return realpathN(filepath.Clean(t), depth-1)
		}
	}
	return q
}

func under(c, root string) bool { return c == root || strings.HasPrefix(c, root+"/") }

type orphan struct {
	pid, age  int
	root, cmd string
	roots     []string // 걸린 작업 트리 모두(root는 그 대표)
	why       string
	kill      bool
}

// findOrphans: 고아 후보. 그 작업 트리 아래를 쓰는 live 터미널이 있으면 빼고, 작업 트리가 있는데 본 저장소 아래를 쓰는 터미널이 있거나
// cwd로만 걸린 것은 보류(kill false). keep은 실행 파일 이름: exeOf(ps comm)에 있으면 그 경로, 없으면 명령 첫 낱말
func findOrphans(procs map[int]procInfo, cwdOf map[int]string, termCwds []string, suffix string, minAge float64, me int, only string,
	exists func(string) bool, keep map[string]bool, exeOf map[int]string) []orphan {
	var terms []string
	for _, c := range termCwds {
		terms = append(terms, realpath(c))
	}
	pids := make([]int, 0, len(procs))
	for p := range procs {
		pids = append(pids, p)
	}
	sort.Ints(pids)
	var found []orphan
	for _, pid := range pids {
		pi := procs[pid]
		exe := exeOf[pid]
		if exe == "" {
			if f := strings.Fields(pi.cmd); len(f) > 0 {
				exe = f[0]
			}
		}
		if pi.ppid != 1 || pid == me || float64(pi.age) < minAge || keep[filepath.Base(exe)] {
			continue
		}
		byCmd, all := map[string]bool{}, map[string]bool{}
		for _, x := range strings.Fields(pi.cmd) {
			if i := strings.Index(x, "="); i >= 0 {
				x = x[i+1:]
			}
			if r := laneRoot(x, suffix); r != "" {
				byCmd[r], all[r] = true, true
			}
		}
		if r := laneRoot(cwdOf[pid], suffix); r != "" {
			all[r] = true
		}
		var roots []string
		for r := range all {
			if only == "" || strings.HasPrefix(r, strings.TrimRight(only, "/")+"/") {
				roots = append(roots, r)
			} else {
				delete(byCmd, r)
			}
		}
		sort.Strings(roots)
		used := len(roots) == 0
		for _, r := range roots {
			if exists(r) {
				for _, c := range terms {
					if under(c, realpath(r)) {
						used = true
					}
				}
			}
		}
		if used {
			continue
		}
		var there []string
		for _, r := range roots {
			if exists(r) {
				there = append(there, r)
			}
		}
		mainUsed := false
		for _, r := range there {
			m := realpath(strings.TrimSuffix(filepath.Dir(r), suffix))
			for _, c := range terms {
				if under(c, m) {
					mainUsed = true
				}
			}
		}
		o := orphan{pid: pid, age: pi.age, cmd: pi.cmd, roots: roots, kill: true}
		switch {
		case mainUsed:
			o.why, o.kill = "보류: 본 저장소 터미널 있음", false
		case len(there) > 0 && len(byCmd) == 0:
			o.why, o.kill = "보류: cwd로만 걸림, 작업 트리 있음", false
		case len(there) > 0:
			o.why = "쓰는 터미널 없음"
		default:
			o.why = "작업 트리 지워짐"
		}
		if len(there) > 0 {
			o.root = there[0]
		} else {
			o.root = roots[0]
		}
		found = append(found, o)
	}
	return found
}

// tree: pids와 그 자손
func tree(pids []int, procs map[int]procInfo) map[int]bool {
	kids := map[int][]int{}
	for p, pi := range procs {
		kids[pi.ppid] = append(kids[pi.ppid], p)
	}
	out := map[int]bool{}
	todo := append([]int(nil), pids...)
	for len(todo) > 0 {
		p := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		if !out[p] {
			out[p] = true
			todo = append(todo, kids[p]...)
		}
	}
	return out
}

// startOf: 그 pid의 시작 시각(ps lstart, 초). 모르면 오류
func startOf(pid int) (time.Time, error) {
	s, err := proc.StartTime(pid)
	if err != nil {
		return time.Time{}, err
	}
	return time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(strings.Fields(s), " "), time.Local)
}

// killTree: pids와 그 자손을 끈다(TERM, wait 뒤 남으면 KILL). 처음 본 그 실행만(E04 — pid 재사용): 다시 본 ppid·명령이 같고 나이가
// 처음 본 뒤 흐른 만큼 늘었고(seenAt), 신호 직전 시작 시각(lstart)이 그 나이와 맞는 것. KILL 직전에도 시작 시각을 다시 견준다.
// 시작 시각을 모르면(ps 실패) 그 프로세스는 건드리지 않는다. (KILL까지 간 수, pids[0]에 신호를 보냈나)
func killTree(pids []int, procs map[int]procInfo, seenAt time.Time, wait time.Duration) (int, bool, error) {
	now, err := processes(run)
	if err != nil {
		return 0, false, err
	}
	at, grown := time.Now(), time.Since(seenAt).Seconds()
	born := map[int]time.Time{}
	var todo []int
	for p := range tree(pids, procs) {
		n, ok := now[p]
		// 오차 3초: etime·lstart는 초 단위로 잘리고, 부하(1분 평균 20)에서 ps 한 번이 1초를 넘기도 한다
		if !ok || n.ppid != procs[p].ppid || n.cmd != procs[p].cmd || float64(n.age) < float64(procs[p].age)+grown-3 ||
			float64(n.age) > float64(procs[p].age)+grown+3 {
			continue
		}
		st, err := startOf(p)
		want := at.Add(-time.Duration(n.age) * time.Second)
		if err != nil || st.Sub(want) > 3*time.Second || want.Sub(st) > 3*time.Second {
			continue
		}
		born[p] = st
		todo = append(todo, p)
	}
	if _, ok := born[pids[0]]; !ok { // 루트가 처음 본 그 실행이 아니거나 모름: 그 트리의 자손도 우리가 본 것인지 모른다(R-19)
		return 0, false, nil
	}
	for _, p := range todo {
		if st, err := startOf(p); err == nil && st.Equal(born[p]) { // 신호 직전에 그 pid가 아직 그 실행인지 다시(R-19)
			term(p)
		}
	}
	end := time.Now().Add(wait)
	for time.Now().Before(end) {
		any := false
		for _, p := range todo {
			any = any || alive(p)
		}
		if !any {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	left := 0
	for _, p := range todo {
		if !alive(p) {
			continue
		}
		if st, err := startOf(p); err != nil || !st.Equal(born[p]) {
			continue // 그사이 끝나고 다른 실행이 그 pid를 받았거나 모름
		}
		hardKill(p)
		left++
	}
	_, rootDone := born[pids[0]]
	return left, rootDone, nil
}

// seen: closed.log에 이 pid·명령 앞부분의 관찰 기록이 이미 있나(제목 「나이 N분 <명령>」의 나이는 견주지 않는다)
func seen(pid int, head, path string) bool {
	if path == "" {
		path = filepath.Join(tasksDir, "closed.log")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, l := range strings.Split(string(b), "\n") {
		x := strings.Split(l, "\t")
		if len(x) > 6 && x[1] == "pid:"+strconv.Itoa(pid) && strings.Contains(x[3], "관찰") {
			if p := strings.SplitN(x[6], " ", 3); len(p) == 3 && p[2] == head {
				return true
			}
		}
	}
	return false
}

// sweepOrphans: 고아 쓸기 한 바퀴. (끌 것 수, 판정 실패 — 셸 판의 예외처럼 종료 코드에 넣는다)
// 기록·출력에는 명령 줄(인자)을 남기지 않는다(E05 — 자격증명이 섞일 수 있다): 실행 파일·pid·나이·작업 트리·까닭만
func sweepOrphans(termsAny any, dry bool, minAge float64, only string) (int, error) {
	suffix, ok1 := conf("sweep.wt_suffix", ".wt")
	keepS, ok2 := conf("sweep.orphan_keep", "codebase-memory-mcp herdr-core")
	mode, ok3 := conf("sweep.orphan_kill", "off")
	if !ok1 || !ok2 || !ok3 || suffix == "" || strings.Contains(suffix, "/") {
		fmt.Println("고아 쓸기 건너뜀: 설정 sweep.wt_suffix·sweep.orphan_keep·sweep.orphan_kill을 못 읽음")
		return 0, nil
	}
	if mode != "on" && mode != "off" {
		fmt.Printf("고아 쓸기: sweep.orphan_kill은 on·off다(지금 '%s') — 끄지 않는다\n", mode)
		mode = "off"
	}
	list, isList := termsAny.([]any)
	var terms []map[string]any
	for _, x := range list {
		t, ok := x.(map[string]any)
		if _, isStr := t["address"].(string); !ok || !isStr {
			isList = false
			break
		}
		terms = append(terms, t)
	}
	if !isList {
		fmt.Println("고아 쓸기 건너뜀: hc ls에 terminals 목록이 없다")
		return 0, nil
	}
	var local []map[string]any
	var liveCwds []string
	for _, t := range terms {
		st, isStr := t["state"].(string)
		if !strings.Contains(str(t, "address"), "/") && (!isStr || st != "live" && st != "finished") {
			// 이 기계 터미널이 살아 있는지 모르면 그 cwd를 「쓰는 터미널 없음」으로 볼 수 없다(R-16)
			fmt.Println("고아 쓸기 건너뜀: 상태를 모르는 이 기계 터미널이 있다")
			return 0, nil
		}
		if st != "live" {
			continue
		}
		_, hasCwd := t["cwd"].(string)
		if hasCwd {
			liveCwds = append(liveCwds, str(t, "cwd"))
		}
		if !strings.Contains(str(t, "address"), "/") {
			if !hasCwd {
				fmt.Println("고아 쓸기 건너뜀: cwd를 모르는 이 기계 터미널이 있다")
				return 0, nil
			}
			local = append(local, t)
		}
	}
	procs, err := processes(run)
	seenAt := time.Now()
	if err != nil {
		fmt.Println("고아 쓸기 실패:", err)
		return 0, err
	}
	var ones []int
	for p, pi := range procs {
		if pi.ppid == 1 {
			ones = append(ones, p)
		}
	}
	exeOf, err := exes(run)
	if err != nil {
		fmt.Println("고아 쓸기 실패:", err)
		return 0, err
	}
	keep := map[string]bool{}
	for _, k := range strings.Fields(keepS) {
		keep[k] = true
	}
	found := findOrphans(procs, cwds(ones, run), liveCwds, suffix, minAge, os.Getpid(), only, exists, keep, exeOf)
	// 자손에 live 터미널 프로세스(코어·에이전트)가 있으면 끄지 않는다. hc ls 뒤 끝난 터미널(not_found·finished — 터미널 쓸기가
	// 이번 바퀴에 닫은 것)은 live가 아니라 pid 모음에서 빼고, 그 cwd의 작업 트리에 걸린 후보만 보류한다(다음 바퀴에 원래 판정).
	// 그 밖의 실패(hc 실패·pid 없음)는 바퀴 전체 보류, 까닭(<주소>: <무엇>)은 한 줄
	termPids, pidsWhy := map[int]bool{}, ""
	var ended []string
	for _, o := range found {
		if !o.kill {
			continue
		}
		for _, t := range local {
			a := str(t, "address")
			g, err := hc("get", a)
			if isNotFound(err) || err == nil && str(g, "state") == "finished" {
				ended = append(ended, realpath(str(t, "cwd")))
				continue
			}
			p, isNum := g["pid"].(float64)
			if err != nil {
				pidsWhy = a + ": " + err.Error()
			} else if !isNum {
				pidsWhy = a + ": pid 없음"
			}
			if pidsWhy != "" {
				fmt.Println("고아 쓸기: 터미널 pid를 못 읽음 —", pidsWhy)
				break
			}
			termPids[int(p)] = true
		}
		break
	}
	targets := 0
	for _, o := range found {
		for _, c := range ended {
			for _, r := range o.roots {
				if o.kill && under(c, realpath(r)) {
					o.why, o.kill = "보류: 이번 바퀴에 끝난 터미널의 작업 트리", false
				}
			}
		}
		if o.kill && pidsWhy != "" {
			o.why, o.kill = "보류: 터미널 pid를 못 읽음", false
		} else if o.kill {
			for p := range tree([]int{o.pid}, procs) {
				if termPids[p] {
					o.why, o.kill = "보류: 자손에 터미널 프로세스 있음", false
				}
			}
		}
		head := exeOf[o.pid]
		if f := strings.Fields(o.cmd); head == "" && len(f) > 0 {
			head = f[0]
		}
		head = flat(cutRunes(head, 120))
		line := fmt.Sprintf("pid %d 나이 %d분 %s (%s) %s", o.pid, o.age/60, o.root, o.why, head)
		title := fmt.Sprintf("나이 %d분 %s", o.age/60, head)
		addr := "pid:" + strconv.Itoa(o.pid)
		switch {
		case !o.kill:
			fmt.Println("보류 고아:", line)
			continue
		case dry:
			fmt.Println("끌 고아:", line)
		case mode == "off":
			fmt.Println("고아(끄기 꺼짐, 기록만):", line)
			if !seen(o.pid, head, "") {
				record(addr, "-", "고아 관찰("+o.why+")", o.root, "-", title)
			}
		default:
			hard, did, err := killTree([]int{o.pid}, procs, seenAt, 5*time.Second)
			if err != nil {
				fmt.Println("고아 못 끔:", line, err)
				continue
			}
			if !did {
				fmt.Println("고아 안 끔(그사이 끝났거나 다른 실행이 그 pid를 받음·시작 시각 모름):", line)
				continue
			}
			k, mark := "", ""
			if hard > 0 {
				k, mark = " KILL", "(KILL)"
			}
			record(addr, "-", "고아("+o.why+")"+k, o.root, "-", title)
			fmt.Println("끈 고아:", line, mark)
		}
		targets++
	}
	return targets, nil
}
