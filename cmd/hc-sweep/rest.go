package main

// 쉼 판정(ht-sweep-busy-idle, docs/hc-sweep/orphans.md 「정말 쉬는가」): hc 상태 done·idle은 「주 대화의 턴이 끝났다」일 뿐이다 — Claude의
// 배경 하위 에이전트는 같은 프로세스 안에서 돌아 도구를 쓸 때 말고는 자식 프로세스도 없고, 세션 파일의 status도 idle이다(2026-11-28 실측:
// R2가 하위 에이전트 셋을 기다리던 터미널을 닫아 일을 잃었다). 그 터미널의 Claude 세션(프로세스 나무의 ~/.claude/sessions/<pid>.json)이
// 남기는 대화 기록(~/.claude/projects/*/<세션 id>.jsonl과 <세션 id>/subagents/agent-*)으로 본다. 터미널을 닫는 길은 모두 settle을 거친다

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// rest: 쉼 판정. 0값은 모름이다(확인을 거치지 않은 닫기를 closeTerm이 거절한다)
type rest int

const (
	restUnknown rest = iota
	restQuiet
	restBusy
)

// rested: quiet의 답. last는 대화 기록의 가장 늦은 수정 시각(ms, 0 = 기록 없음)
type rested struct {
	state    rest
	last     float64
	sig, sid string
}

var (
	projDir = filepath.Join(home, ".claude", "projects")
	sidRe   = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z_-]{0,127}$`) // 경로·glob 글자 없는 세션 id만
	// taskRoot: Claude가 세션마다 배경 작업 출력 파일을 두는 뿌리 — Claude 자신의 규칙($CLAUDE_CODE_TMPDIR 또는 /tmp, 그 아래 claude-<uid>).
	// 작업별 TMPDIR은 따르지 않는다(2026-11-28 실측)
	taskRoot = filepath.Join(cmp.Or(os.Getenv("CLAUDE_CODE_TMPDIR"), "/tmp"), "claude-"+strconv.Itoa(os.Getuid()))
	trailer  = regexp.MustCompile(`\n\[(killed|exited with code [^\]\n]*)\]\n$`) // Claude가 배경 작업이 끝날 때 붙이는 끝 줄
)

// sessionOf: pid와 그 자손 가운데 처음 찾은 Claude 세션 파일(<sessDir>/<pid>.json). 없으면 nil·nil(Claude가 아님),
// ps 실패·파일을 못 읽음·JSON 아님은 오류(모름)
func sessionOf(pid int, r runner) (map[string]any, error) {
	if _, err := os.Stat(sessDir); pid <= 0 || os.IsNotExist(err) { // Claude 세션 폴더가 없는 기계(리눅스 워커 등)는 ps도 안 부른다
		return nil, nil
	}
	var children map[int][]int
	for todo := []int{pid}; len(todo) > 0; {
		p := todo[0]
		todo = todo[1:]
		b, err := os.ReadFile(filepath.Join(sessDir, strconv.Itoa(p)+".json"))
		if err == nil {
			var m map[string]any
			if json.Unmarshal(b, &m) != nil {
				return nil, fmt.Errorf("세션 파일 JSON 아님")
			}
			return m, nil
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("세션 파일 못 읽음")
		}
		if children == nil { // 뿌리에 없을 때만 ps 한 번
			res := r(30*time.Second, nil, "ps", "-A", "-o", "pid=,ppid=")
			if res.err != nil || res.rc != 0 {
				return nil, fmt.Errorf("ps 실패")
			}
			children = map[int][]int{}
			for _, l := range strings.Split(res.out, "\n") {
				f := strings.Fields(l)
				c, e1 := strconv.Atoi(f0(f, 0))
				pp, e2 := strconv.Atoi(f0(f, 1))
				if len(f) == 2 && e1 == nil && e2 == nil {
					children[pp] = append(children[pp], c)
				}
			}
		}
		todo = append(todo, children[p]...)
	}
	return nil, nil
}

func f0(f []string, i int) string {
	if i < len(f) {
		return f[i]
	}
	return ""
}

func msOf(fi os.FileInfo) float64 { return float64(fi.ModTime().UnixNano()) / 1e6 }

// quiet: 「이 터미널은 지금 정말 쉬는가」. 세션 파일이 없으면(Claude가 아닌 CLI·기록 없는 기계) 쉼(지금 규칙 그대로). 기록이 r3 안에 바뀐
// 하위 에이전트에 끝난 증거(agentEnded)가 없거나, 끝 표시 없는 배경 작업(bgTasks — 끝난 하위 에이전트가 남긴 셸도)이 있으면 활동.
// 그 밖은 쉼이고 last가 마지막 활동. 읽다 실패하면 모름
func quiet(t map[string]any, now, r3 float64, r runner) rested {
	pid, _ := num(t, "pid")
	sess, err := sessionOf(int(pid), r)
	if err != nil {
		return rested{sig: err.Error()}
	}
	if sess == nil {
		return rested{state: restQuiet}
	}
	sid := str(sess, "sessionId")
	if !sidRe.MatchString(sid) {
		return rested{sig: "세션 id 꼴"}
	}
	q := rested{state: restQuiet, sid: sid}
	ents, err := os.ReadDir(projDir)
	if err != nil && !os.IsNotExist(err) {
		return rested{sid: sid, sig: "대화 기록 폴더 못 읽음"}
	}
	busy, counted := 0, map[string]bool{} // 옛 폴더 이름을 가리키는 링크로 같은 기록이 두 번 보여도 한 번만 센다
	seeMax := func(m float64) {
		if m > q.last {
			q.last = m
		}
	}
	for _, e := range ents {
		base := filepath.Join(projDir, e.Name())
		if st, err := os.Stat(base); err != nil || !st.IsDir() { // 링크된 옛 폴더도 따른다. 깨진 링크·파일은 건너뜀
			continue
		}
		if fi, err := os.Stat(filepath.Join(base, sid+".jsonl")); err == nil {
			seeMax(msOf(fi))
		} else if !os.IsNotExist(err) {
			return rested{sid: sid, sig: "대화 기록 못 읽음"}
		}
		sub := filepath.Join(base, sid, "subagents")
		metas, err := filepath.Glob(filepath.Join(sub, "agent-*.meta.json"))
		if err != nil {
			return rested{sid: sid, sig: "하위 기록 못 읽음"}
		}
		if fis, err := os.ReadDir(sub); err == nil {
			for _, x := range fis {
				if fi, err := x.Info(); err == nil && strings.HasSuffix(x.Name(), ".jsonl") {
					seeMax(msOf(fi))
				}
			}
		} else if !os.IsNotExist(err) {
			return rested{sid: sid, sig: "하위 기록 못 읽음"}
		}
		for _, m := range metas {
			b, err := os.ReadFile(m)
			var meta map[string]any
			if err != nil || json.Unmarshal(b, &meta) != nil {
				return rested{sid: sid, sig: "하위 기록 못 읽음"}
			}
			touched := 0.0
			for _, p := range []string{m, strings.TrimSuffix(m, ".meta.json") + ".jsonl"} {
				if fi, err := os.Stat(p); err == nil && msOf(fi) > touched {
					touched = msOf(fi)
				}
			}
			seeMax(touched)
			// r3 안에 바뀐 하위 에이전트는 끝난 증거가 없으면 활동(meta 꼴이 달라도 닫지 않는 쪽 — 감사 atp6tdq L2). r3 넘게 조용하면(죽은 것) 안 친다
			id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "agent-"), ".meta.json")
			if now-touched >= r3 || counted[id] {
				continue
			}
			ended, err := agentEnded(meta, strings.TrimSuffix(m, ".meta.json")+".jsonl")
			if err != nil {
				return rested{sid: sid, sig: "대화 기록 못 읽음"}
			}
			if !ended {
				busy++
				counted[id] = true
			}
		}
	}
	tasks, err := bgTasks(sid, now, r3, seeMax)
	if err != nil {
		return rested{sid: sid, sig: "배경 작업 못 읽음"}
	}
	var why []string
	if busy > 0 {
		why = append(why, fmt.Sprintf("배경 하위 에이전트 %d개 안 끝남", busy))
	}
	if tasks > 0 {
		why = append(why, fmt.Sprintf("배경 작업 %d개 도는 중", tasks))
	}
	if len(why) > 0 {
		q.state, q.sig = restBusy, strings.Join(why, " · ")
	}
	return q
}

// bgTasks: 그 세션의 배경 작업(run_in_background 셸·Monitor·시간을 넘겨 배경으로 넘어간 명령 — 하위 에이전트가 띄운 것도 주 세션에 든다) 가운데
// 끝 표시가 없고 r3 안에 바뀐 것의 수. 파일은 <taskRoot>/<폴더 이름>/<세션 id>/tasks/<작업 id>.output, 끝나면 Claude가 끝에 trailer를 붙인다.
// 링크(a….output — 하위 에이전트 기록, 지금 규칙이 본다)는 안 센다. 뿌리·세션 폴더가 없으면 0, 못 읽으면 오류(모름). 수정 시각은 see로 마지막 활동에
func bgTasks(sid string, now, r3 float64, see func(float64)) (int, error) {
	dirs, err := os.ReadDir(taskRoot)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, counted := 0, map[string]bool{}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(taskRoot, d.Name(), sid, "tasks")
		fis, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return 0, err
		}
		for _, x := range fis {
			if !x.Type().IsRegular() || !strings.HasSuffix(x.Name(), ".output") {
				continue
			}
			fi, err := x.Info()
			if os.IsNotExist(err) { // 끝난 전경 명령의 파일은 Claude가 지운다
				continue
			}
			if err != nil {
				return 0, err
			}
			see(msOf(fi))
			if now-msOf(fi) >= r3 || counted[x.Name()] { // r3 넘게 조용한 미완은 죽은 것(Claude가 끝 표시 없이 죽음 등)
				continue
			}
			ended, err := endsWithTrailer(filepath.Join(dir, x.Name()), fi.Size())
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return 0, err
			}
			if !ended {
				n++
				counted[x.Name()] = true
			}
		}
	}
	return n, nil
}

// endsWithTrailer: 파일 끝 64바이트가 끝 표시로 끝나는가(출력 가운데의 같은 글은 끝이 아님)
func endsWithTrailer(path string, size int64) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	off := max(size-64, 0)
	b := make([]byte, size-off)
	if _, err := f.ReadAt(b, off); err != nil && err != io.EOF {
		return false, err
	}
	return trailer.Match(b), nil
}

// agentEnded: 하위 에이전트가 끝났다는 증거는 그 자기 기록 둘뿐 — meta "finished":true(끝까지 간 배경), 자기 기록의 마지막 대화 줄이
// assistant end_turn(이름 붙인 배경 에이전트 「팀원」은 meta에 끝 표시가 없고 쉴 때 이 꼴 — 감사 atp6tdq M1). 주 기록의 끝남 알림은 다시
// 불린 에이전트를 끝남으로 읽어 쓰지 않는다(지휘 결정 2026-11-28 — 멈춘 것은 r3까지 남는 쪽을 받아들임, docs/hc-sweep/busy-idle.md)
func agentEnded(meta map[string]any, own string) (bool, error) {
	if meta["finished"] == true {
		return true, nil
	}
	return turnEnded(own)
}

// turnEnded: 기록 끝 64KB에서 마지막 대화 줄(type user·assistant)이 assistant·stop_reason end_turn인가(그 뒤에 붙는 훅 결과 줄은 건너뜀).
// 도구를 쓰는 중이면 assistant tool_use나 user(tool_result)다. 파일이 없거나 대화 줄을 못 찾으면 false(끝났다는 증거 없음)
func turnEnded(path string) (bool, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false, err
	}
	off := st.Size() - 64<<10
	if off < 0 {
		off = 0
	}
	b := make([]byte, st.Size()-off)
	if _, err := f.ReadAt(b, off); err != nil && err != io.EOF {
		return false, err
	}
	ls := strings.Split(string(b), "\n")
	for i := len(ls) - 1; i >= 0; i-- {
		var l struct {
			Type    string `json:"type"`
			Message struct {
				StopReason string `json:"stop_reason"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(ls[i]), &l) != nil || l.Type != "user" && l.Type != "assistant" { // 잘린 첫 줄·훅 결과 줄
			continue
		}
		return l.Type == "assistant" && l.Message.StopReason == "end_turn", nil
	}
	return false, nil
}

// settle: 닫기 판정 = judge(규칙) + quiet(정말 쉬는가). 규칙이 나온 터미널에만 기록을 본다(바퀴 시간). 턴이 끝나고 1분 넘어 기록이 바뀌었으면
// 쉰 시간은 마지막 활동부터 다시 센다(제출 뒤 빠른 닫기도 R2 문턱부터). 모름은 R3(상태가 r3만큼 그대로)일 때만 닫는다
func settle(t map[string]any, f facts, now float64, keep map[string]bool, r runner) (string, string, rested) {
	why, kept := judge(t, f, now, keep)
	if why == "" {
		return "", kept, rested{}
	}
	q := quiet(t, now, float64(lim.r3)*minute, r)
	switch {
	case q.state == restBusy:
		return "", "안에서 일이 도는 중(" + q.sig + ")", q
	case q.state == restUnknown && why != "R3":
		return "", "쉼 모름(" + q.sig + ")", q
	case q.state == restUnknown:
		q.state = restQuiet
	case q.last > orNow(t, now)+minute:
		moved := map[string]any{}
		for k, v := range t {
			moved[k] = v
		}
		moved["status_ms"] = q.last
		g := f
		g.submitted = false
		if why, _ = judge(moved, g, now, keep); why == "" {
			q.state, q.sig = restBusy, fmt.Sprintf("대화 기록이 %d분 전에 바뀜", int((now-q.last)/minute))
			return "", "안에서 일이 도는 중(" + q.sig + ")", q
		}
	}
	return why, "", q
}

// keptOnce: 쉼 판정으로 남긴 터미널을 closed.log에 그 주소·이름·세션·까닭 종류마다 한 번만 적는다(바퀴마다 되풀이하지 않게). 새로 적었나
func keptOnce(t map[string]any, q rested, kept string) bool {
	kind := "남김(쉼 모름)"
	if q.state == restBusy {
		kind = "남김(안에서 일이 도는 중)"
	}
	key := []string{str(t, "address"), dash(str(t, "name")), kind}
	b, _ := os.ReadFile(filepath.Join(tasksDir, "closed.log"))
	for _, l := range strings.Split(string(b), "\n") {
		if x := strings.Split(l, "\t"); len(x) > 5 && x[1] == key[0] && x[2] == key[1] && x[3] == kind && x[5] == dash(q.sid) {
			return false
		}
	}
	record(key[0], str(t, "name"), kind, str(t, "cwd"), dash(q.sid), kept)
	return true
}
