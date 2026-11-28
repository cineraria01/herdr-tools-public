// Package rest: 「이 터미널은 지금 정말 쉬는가」 한 곳(ht-rest-one-gate, docs/hc-sweep/busy-idle.md). hc 상태 done·idle은 「주 대화의 턴이
// 끝났다」일 뿐이다 — Claude의 배경 하위 에이전트·배경 셸은 같은 세션 안에서 돈다(2026-11-28 R2가 배경 셸 셋을 기다리던 터미널을 닫아 일을
// 잃었다). 그 터미널의 Claude 세션(프로세스 나무의 ~/.claude/sessions/<pid>.json)이 남기는 대화 기록(~/.claude/projects/*/<세션 id>.jsonl과
// <세션 id>/subagents/agent-*)과 배경 작업 출력 파일로 본다. 터미널을 닫거나 손을 뗀 것으로 치는 길은 모두 이 판정을 거친다: 그 기계의
// hc-sweep 안에서는 Of, 다른 도구·다른 기계는 Ask(그 기계의 `hc-sweep --rest <주소>` 한 줄 — Line·Parse). 모든 hc kill이 이 문을
// 거치는지는 gate_test.go가 저장소 전체로 본다
package rest

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"herdr-tools/internal/upstream"
)

// State: 쉼 판정. 0값은 모름이다(확인을 거치지 않은 조치를 받는 쪽이 거절한다)
type State int

const (
	Unknown State = iota
	Quiet
	Busy
)

var words = map[State]string{Unknown: "unknown", Quiet: "quiet", Busy: "busy"}

func (s State) String() string { return cmp.Or(words[s], "unknown") }

// Verdict: 판정. Last는 대화 기록·배경 작업 파일의 가장 늦은 수정 시각(ms, 0 = 기록 없음), Sig는 활동·모름의 까닭 한 줄
type Verdict struct {
	State    State
	Last     float64
	Sig, Sid string
}

var (
	home    = os.Getenv("HOME")
	SessDir = filepath.Join(home, ".claude", "sessions") // Claude 자신의 규칙(<설정 집>/sessions — macOS·리눅스 같은 코드)
	ProjDir = filepath.Join(home, ".claude", "projects")
	// TaskRoot: Claude가 세션마다 배경 작업 출력 파일을 두는 뿌리 — Claude 자신의 규칙($CLAUDE_CODE_TMPDIR 또는 /tmp, 그 아래 claude-<uid>).
	// 작업별 TMPDIR은 따르지 않는다(2026-11-28 실측)
	TaskRoot = filepath.Join(cmp.Or(os.Getenv("CLAUDE_CODE_TMPDIR"), "/tmp"), "claude-"+strconv.Itoa(os.Getuid()))
	sidRe    = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z_-]{0,127}$`)           // 경로·glob 글자 없는 세션 id만
	trailer  = regexp.MustCompile(`\n\[(killed|exited with code [^\]\n]*)\]\n$`) // Claude가 배경 작업이 끝날 때 붙이는 끝 줄
)

// PS: `ps -A -o pid=,ppid=`의 출력(ok=false면 못 돌림 — 모름)
type PS func() (out string, ok bool)

// Held: 살아 있는 프로세스가 연 경로 집합(링크를 푼 실제 경로 — lsof가 내는 꼴). nil이면 못 읽음(모름). Of가 끝 표시 없는 배경 작업 파일이
// 있을 때만 한 번 부른다(hc-sweep은 openPaths)
type Held func() map[string]bool

var errHeld = errors.New("배경 작업을 쥔 프로세스 못 읽음")

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

// claude: hc get의 status.app이 Claude인가(코어가 화면으로 읽은 앱 이름 — 2026-11-28 실측 "claude-code"). 상태가 있는데 앱 칸이 없으면
// 모름(Claude일 수 있다 — 닫지 않는 쪽), 상태가 없으면(셸·hc keep이 남긴 셸) Claude 아님
func claude(t map[string]any) (bool, bool) {
	st, ok := t["status"].(map[string]any)
	if !ok {
		return false, t["status"] == nil
	}
	app := str(st, "app")
	return strings.Contains(strings.ToLower(app), "claude"), app != ""
}

// SessionOf: sessionOf를 실물 점검에(같은 찾기 — 점검이 따로 짜지 않게)
func SessionOf(pid int, ps PS) (map[string]any, error) { return sessionOf(pid, ps) }

// AgentEnded: 하위 에이전트 meta 파일 하나의 끝난 증거(Of와 같은 규칙 — 실물 점검용)
func AgentEnded(meta string) (bool, error) {
	b, err := os.ReadFile(meta)
	var m map[string]any
	if err != nil || json.Unmarshal(b, &m) != nil {
		return false, fmt.Errorf("하위 기록 못 읽음")
	}
	return agentEnded(m, strings.TrimSuffix(meta, ".meta.json")+".jsonl")
}

// TurnEnded: 기록의 마지막 대화 줄이 턴 끝인가(실물 점검이 주 대화의 턴 끝을 기다릴 때)
func TurnEnded(path string) (bool, error) { return turnEnded(path) }

// Trailer: 배경 작업 출력 파일이 끝 표시로 끝나는가(실물 점검용)
func Trailer(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return endsWithTrailer(path, fi.Size())
}

// sessionOf: pid와 그 자손 가운데 처음 찾은 Claude 세션 파일. 없으면 nil·nil, ps 실패·파일을 못 읽음·JSON 아님은 오류(모름)
func sessionOf(pid int, ps PS) (map[string]any, error) {
	if _, err := os.Stat(SessDir); pid <= 0 || os.IsNotExist(err) { // 세션 폴더가 없는 기계는 ps도 안 부른다
		return nil, nil
	}
	var children map[int][]int
	for todo := []int{pid}; len(todo) > 0; {
		p := todo[0]
		todo = todo[1:]
		b, err := os.ReadFile(filepath.Join(SessDir, strconv.Itoa(p)+".json"))
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
			out, ok := ps()
			if !ok {
				return nil, fmt.Errorf("ps 실패")
			}
			children = map[int][]int{}
			for _, l := range strings.Split(out, "\n") {
				f := strings.Fields(l)
				if len(f) != 2 {
					continue
				}
				c, e1 := strconv.Atoi(f[0])
				pp, e2 := strconv.Atoi(f[1])
				if e1 == nil && e2 == nil {
					children[pp] = append(children[pp], c)
				}
			}
		}
		todo = append(todo, children[p]...)
	}
	return nil, nil
}

func msOf(fi os.FileInfo) float64 { return float64(fi.ModTime().UnixNano()) / 1e6 }

// Of: 「이 터미널은 지금 정말 쉬는가」(t = hc get 답 하나, now·r3 ms). 세션 파일이 없으면 Claude가 아닌 CLI·셸은 쉼(지금 규칙 그대로),
// Claude이거나 Claude인지 모르면 모름(자료가 없다 — 판·자리가 바뀌어도 조용히 쉼으로 떨어지지 않게). 기록이 r3 안에 바뀐 하위 에이전트에
// 끝난 증거(agentEnded)가 없거나, 끝 표시 없고 살아 있는 프로세스가 쥔 배경 작업(bgTasks — 끝난 하위 에이전트가 남긴 셸도)이 있으면 활동.
// 그 밖은 쉼이고 Last가 마지막 활동. 읽다 실패하면 모름. 그 세션의 판 묶음이 실물 점검으로 확인되지 않았으면(internal/upstream —
// ht-upstream-contract) 자료가 있어도 모름: 이 판정이 기대는 파일 관례가 그 판에서도 맞는지 아직 모른다
func Of(t map[string]any, now, r3 float64, ps PS, held Held) Verdict {
	return of(t, now, r3, ps, held, true)
}

// Look: Of와 같되 판 확인을 건너뛴다 — 실물 점검(hc-sweep --upstream-probe)이 확인 안 된 판에서 판정 자체를 대조할 때만
func Look(t map[string]any, now, r3 float64, ps PS, held Held) Verdict {
	return of(t, now, r3, ps, held, false)
}

// UpFile: 판 기록(upstream.Path — 시험이 바꾼다)
var UpFile = upstream.Path()

func of(t map[string]any, now, r3 float64, ps PS, held Held, gate bool) Verdict {
	pid, _ := t["pid"].(float64)
	sess, err := sessionOf(int(pid), ps)
	if err != nil {
		return Verdict{Sig: err.Error()}
	}
	if sess != nil && gate {
		rec, err := upstream.Load(UpFile)
		if err != nil {
			return Verdict{Sid: str(sess, "sessionId"), Sig: "판 기록 못 읽음"}
		}
		if why := rec.Hold(str(sess, "version")); why != "" {
			return Verdict{Sid: str(sess, "sessionId"), Sig: why}
		}
	}
	if sess == nil {
		switch isC, known := claude(t); {
		case isC:
			return Verdict{Sig: "Claude인데 세션 자료 없음"}
		case !known:
			return Verdict{Sig: "앱 모름(hc get status.app 없음)"}
		}
		return Verdict{State: Quiet}
	}
	sid := str(sess, "sessionId")
	if !sidRe.MatchString(sid) {
		return Verdict{Sig: "세션 id 꼴"}
	}
	q := Verdict{State: Quiet, Sid: sid}
	ents, err := os.ReadDir(ProjDir)
	if err != nil && !os.IsNotExist(err) {
		return Verdict{Sid: sid, Sig: "대화 기록 폴더 못 읽음"}
	}
	busy, counted := 0, map[string]bool{} // 옛 폴더 이름을 가리키는 링크로 같은 기록이 두 번 보여도 한 번만 센다
	seeMax := func(m float64) { q.Last = max(q.Last, m) }
	for _, e := range ents {
		base := filepath.Join(ProjDir, e.Name())
		if st, err := os.Stat(base); err != nil || !st.IsDir() { // 링크된 옛 폴더도 따른다. 깨진 링크·파일은 건너뜀
			continue
		}
		if fi, err := os.Stat(filepath.Join(base, sid+".jsonl")); err == nil {
			seeMax(msOf(fi))
		} else if !os.IsNotExist(err) {
			return Verdict{Sid: sid, Sig: "대화 기록 못 읽음"}
		}
		sub := filepath.Join(base, sid, "subagents")
		metas, err := filepath.Glob(filepath.Join(sub, "agent-*.meta.json"))
		if err != nil {
			return Verdict{Sid: sid, Sig: "하위 기록 못 읽음"}
		}
		if fis, err := os.ReadDir(sub); err == nil {
			for _, x := range fis {
				if fi, err := x.Info(); err == nil && strings.HasSuffix(x.Name(), ".jsonl") {
					seeMax(msOf(fi))
				}
			}
		} else if !os.IsNotExist(err) {
			return Verdict{Sid: sid, Sig: "하위 기록 못 읽음"}
		}
		for _, m := range metas {
			b, err := os.ReadFile(m)
			var meta map[string]any
			if err != nil || json.Unmarshal(b, &meta) != nil {
				return Verdict{Sid: sid, Sig: "하위 기록 못 읽음"}
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
				return Verdict{Sid: sid, Sig: "대화 기록 못 읽음"}
			}
			if !ended {
				busy++
				counted[id] = true
			}
		}
	}
	tasks, err := bgTasks(sid, now, r3, seeMax, held)
	if errors.Is(err, errHeld) {
		return Verdict{Sid: sid, Sig: err.Error()}
	}
	if err != nil {
		return Verdict{Sid: sid, Sig: "배경 작업 못 읽음"}
	}
	var why []string
	if busy > 0 {
		why = append(why, fmt.Sprintf("배경 하위 에이전트 %d개 안 끝남", busy))
	}
	if tasks > 0 {
		why = append(why, fmt.Sprintf("배경 작업 %d개 도는 중", tasks))
	}
	if len(why) > 0 {
		q.State, q.Sig = Busy, strings.Join(why, " · ")
	}
	return q
}

// bgTasks: 그 세션의 배경 작업(run_in_background 셸·Monitor·시간을 넘겨 배경으로 넘어간 명령 — 하위 에이전트가 띄운 것도 주 세션에 든다) 가운데
// 끝 표시가 없고 r3 안에 바뀌었고 살아 있는 프로세스가 쥔 것의 수. 파일은 <TaskRoot>/<폴더 이름>/<세션 id>/tasks/<작업 id>.output, 끝나면
// Claude가 끝에 trailer를 붙인다. 끝 표시 없이 남는 파일도 있다(2026-11-28 — 상한에 걸려 죽은 전경 명령, 쥔 프로세스 0): 파일 꼴은 Claude
// 안쪽 일이라 「도는가」는 운영체제에 묻는다(held, 후보가 있을 때만 한 번). 링크(a….output — 하위 에이전트 기록, 위 규칙이 본다)는 안 센다.
// 뿌리·세션 폴더가 없으면 0, 못 읽으면 오류(모름 — 쥔 것을 못 읽으면 errHeld). 수정 시각은 쥠과 상관없이 see로 마지막 활동에
func bgTasks(sid string, now, r3 float64, see func(float64), held Held) (int, error) {
	dirs, err := os.ReadDir(TaskRoot)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var open []string
	counted := map[string]bool{}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(TaskRoot, d.Name(), sid, "tasks")
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
				open = append(open, filepath.Join(dir, x.Name()))
				counted[x.Name()] = true
			}
		}
	}
	if len(open) == 0 {
		return 0, nil
	}
	var m map[string]bool
	if held != nil {
		m = held()
	}
	if m == nil {
		return 0, errHeld
	}
	n := 0
	for _, p := range open {
		real, err := filepath.EvalSymlinks(p) // lsof는 링크를 푼 경로를 낸다(맥 /tmp → /private/tmp)
		if os.IsNotExist(err) {               // 세는 사이 끝나 Claude가 지움
			continue
		}
		if err != nil {
			return 0, err
		}
		if m[real] {
			n++
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
	off := max(st.Size()-64<<10, 0)
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
