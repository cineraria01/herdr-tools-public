package main

// 실물 점검(ht-upstream-contract, docs/hc-sweep/upstream.md 「실물 점검」): 진짜 에이전트 세션 하나를 띄워 쉼 판정(internal/rest)이 기대는
// 동작을 대조한다 — 파일 꼴을 낱낱이 재기보다 「판정이 맞는 답을 내는가」(지휘 정정 1). 장면 넷은 2026-11-28 하루 세 번 잘못 읽은 꼴이다:
// ① 도는 배경 셸 → 활동 ② 끝난 배경 셸 → 쉼 ③ 도는 전경 명령 → 활동 ④ 죽은 전경 명령 뒤 → 쉼(그 명령 pid를 이 점검이 정확히 끈다 —
// 상한에 걸려 죽은 꼴). 곁들여 세션 파일·판·기록 자리·하위 에이전트 끝 증거·hc get 칸, 참고로 화면의 배경 작업 글자(통과에 안 묶음).
// 결과는 그 세션의 판 묶음 기록에(통과 → 믿음·소식 :up, 틀림 → 틀림·막힘 한 번, 못 봄만 → 그대로 — 대기열이 상한 안에서 다시 띄운다).
// 점검용 세션은 hc-lane --read로(신뢰 질문·시작 확인을 받는다) 임시 폴더에서 띄우고, 끝나면 그 터미널만 닫는다

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/rest"
	"herdr-tools/internal/upstream"
)

// 결과 낱말
const (
	pOK  = "맞음"
	pBad = "틀림"
	pUnk = "못 봄"
	pRef = "참고"
)

// 시간(시험이 줄인다): 전체 상한, 한 장면 기다림, 살피는 간격, 배경 셸 길이(초)
var (
	probeTotal = 15 * time.Minute
	probeWait  = 3 * time.Minute
	probeTick  = 2 * time.Second
	probeBgSec = 45
)

const (
	probeName  = "upstream-probe-agent"
	probeBgMk  = "upstream-probe-bg"
	probeFgMk  = "upstream-probe-fg"
	probeFgSec = 100 // Bash 도구 기본 상한(2분) 안 — 넘으면 Claude가 배경으로 넘긴다(2026-11-29 실측)
)

func probePrompt() string {
	return fmt.Sprintf("점검용 세션이다(herdr-tools 실물 점검 — 파일을 만들거나 고치지 말고 설명 없이). 차례대로: "+
		"(1) Bash 도구를 run_in_background:true로 써서 python3 -c \"import time; time.sleep(%d)\" # %s 를 띄운다(기다리지 않는다). "+
		"(2) Agent 도구로 하위 에이전트 하나를 띄워 OK라고만 답하게 하고 끝나기를 기다린다. (3) 끝이라고만 답하고 턴을 끝낸다. "+
		"배경 작업이 끝났다는 알림이 오면 끝이라고만 답한다.", probeBgSec, probeBgMk)
}

func probeFgMsg() string {
	return fmt.Sprintf("설명 없이: Bash 도구(전경, run_in_background 없이)로 python3 -c \"import time; time.sleep(%d)\" # %s 를 돌린다. "+
		"끝나거나 죽으면 다시 돌리지 말고 끝이라고만 답한다.", probeFgSec, probeFgMk)
}

type pRow struct{ item, res, detail string }

type prober struct {
	r     runner
	addr  string
	end   time.Time
	rows  []pRow
	sess  map[string]any
	sid   string
	t     map[string]any
	app   bool                   // hc get이 Claude라고 함
	words map[string]bool        // 본 상태 낱말
	getEr string                 // hc get 칸 검사에서 처음 본 틀림
	held  func() map[string]bool // 열린 경로(시험이 바꾼다)
	kill  func(pid int)          // 전경 명령 끄기(시험이 바꾼다)
	cmdOf func(pid int) string   // 그 pid의 명령 줄
}

func (p *prober) add(item, res, detail string) { p.rows = append(p.rows, pRow{item, res, detail}) }

// until: f가 참이 될 때까지(장면 상한·전체 상한 안)
func (p *prober) until(limit time.Duration, f func() bool) bool {
	end := time.Now().Add(limit)
	for {
		if f() {
			return true
		}
		if time.Now().After(end) || time.Now().After(p.end) {
			return false
		}
		time.Sleep(probeTick)
	}
}

// get: hc get을 다시 읽어 p.t에. 칸 검사(hcjson.ParseGet — 상태 낱말이 아는 것 가운데 하나·pid·만든 때, status_ms 칸)도 이때
func (p *prober) get() bool {
	res := p.r(30*time.Second, nil, "hc", "get", p.addr, "--json")
	if res.err != nil || res.rc != 0 {
		return false
	}
	t, found, err := hcjson.ParseGet([]byte(res.out), res.rc, p.addr)
	var m map[string]any
	switch {
	case err != nil && p.getEr == "":
		p.getEr = err.Error()
	case err == nil && found && (t.PID <= 0 || t.Created <= 0 || t.Raw["status_ms"] == nil) && p.getEr == "":
		p.getEr = "pid·created_ms·status_ms 칸 가운데 없는 것"
	}
	if json.Unmarshal([]byte(res.out), &m) != nil {
		return false
	}
	p.t = m
	if st, ok := m["status"].(map[string]any); ok {
		if s, _ := st["state"].(string); s != "" {
			p.words[s] = true
		}
		a, _ := st["app"].(string)
		p.app = p.app || strings.Contains(strings.ToLower(a), "claude")
	}
	return true
}

// look: 쉼 판정(판 확인 없이 — 확인하려는 것이 그 판이다)
func (p *prober) look() rest.Verdict {
	now := float64(time.Now().UnixNano()) / 1e6
	return rest.Look(p.t, now, float64(lim.r3)*minute, func() (string, bool) {
		res := p.r(30*time.Second, nil, "ps", "-A", "-o", "pid=,ppid=")
		return res.out, res.err == nil && res.rc == 0
	}, p.held)
}

// tasks: 그 세션의 배경 작업 출력 파일(링크 — 하위 에이전트 기록 — 빼고)
func (p *prober) tasks() []string {
	fs, _ := filepath.Glob(filepath.Join(rest.TaskRoot, "*", p.sid, "tasks", "*.output"))
	var o []string
	for _, f := range fs {
		if fi, err := os.Lstat(f); err == nil && fi.Mode().IsRegular() {
			o = append(o, f)
		}
	}
	return o
}

func (p *prober) isHeld(f string) bool {
	real, err := filepath.EvalSymlinks(f)
	m := p.held()
	return err == nil && m != nil && m[real]
}

func (p *prober) trailer(f string) bool { ok, _ := rest.Trailer(f); return ok }

// turnEnded: 주 대화의 마지막 대화 줄이 턴 끝(hc 상태는 배경 셸이 도는 동안 working이다 — 2026-11-29 실측)
func (p *prober) turnEnded() bool {
	fs, _ := filepath.Glob(filepath.Join(rest.ProjDir, "*", p.sid+".jsonl"))
	if len(fs) == 0 {
		return false
	}
	ok, _ := rest.TurnEnded(fs[0])
	return ok
}

// scene: 장면 하나 — 판정이 want인가(busy는 배경 작업 까닭이어야 — 하위 에이전트가 안 끝나 busy면 그 장면을 본 것이 아니다)
func (p *prober) scene(item string, want rest.State, form string) {
	p.get()
	v := p.look()
	res := pOK
	if v.State != want || want == rest.Busy && !strings.Contains(v.Sig, "배경 작업") {
		res = pBad
	}
	p.add(item, res, fmt.Sprintf("판정 %s(바람 %s)%s%s", v.State, want, sigText(v.Sig), form))
}

func sigText(s string) string {
	if s == "" {
		return ""
	}
	return " · " + s
}

// files: 세션 파일·판·기록 자리·하위 에이전트(장면 ① 때 — 턴을 끝냈으니 모두 있어야 한다)
func (p *prober) files() {
	fs, _ := filepath.Glob(filepath.Join(rest.ProjDir, "*", p.sid+".jsonl"))
	if len(fs) == 0 {
		p.add("대화 기록 자리", pBad, "<projects>/*/"+p.sid+".jsonl 없음")
	} else {
		p.add("대화 기록 자리", pOK, filepath.Base(filepath.Dir(fs[0]))+"/"+p.sid+".jsonl")
	}
	metas, _ := filepath.Glob(filepath.Join(rest.ProjDir, "*", p.sid, "subagents", "agent-*.meta.json"))
	if len(metas) == 0 {
		p.add("하위 에이전트 기록", pBad, "<세션>/subagents/agent-*.meta.json 없음")
		p.add("하위 에이전트 끝 증거", pUnk, "기록 없음")
		return
	}
	p.add("하위 에이전트 기록", pOK, fmt.Sprintf("meta %d개", len(metas)))
	// 주 턴이 끝난 직후엔 배경 하위 에이전트가 아직 끝을 안 적었을 수 있다(2026-11-29 실측 — 몇 초 뒤 finished) — 상한 안에서 기다린다
	left := ""
	if !p.until(probeWait, func() bool {
		left = ""
		for _, m := range metas {
			if ok, err := rest.AgentEnded(m); err != nil || !ok {
				left = filepath.Base(m)
			}
		}
		return left == ""
	}) {
		p.add("하위 에이전트 끝 증거", pBad, left+": 상한 안에 meta finished도 자기 기록 끝 턴도 없음")
		return
	}
	p.add("하위 에이전트 끝 증거", pOK, "meta finished 또는 자기 기록 끝 턴")
}

// drive: 장면을 차례로. 어느 단계든 상한을 넘기면 남은 항목은 못 봄
func (p *prober) drive() {
	p.get()
	pid, _ := p.t["pid"].(float64)
	found := p.until(probeWait, func() bool {
		s, err := rest.SessionOf(int(pid), func() (string, bool) {
			res := p.r(30*time.Second, nil, "ps", "-A", "-o", "pid=,ppid=")
			return res.out, res.err == nil && res.rc == 0
		})
		p.get()
		p.sess = s
		return err == nil && s != nil
	})
	if !found {
		res := pUnk
		if p.app { // Claude가 떠 있는데 세션 파일이 없다 — 자리·이름 꼴이 바뀌었다
			res = pBad
		}
		p.add("세션 파일", res, "터미널 pid 나무에서 <sessions>/<pid>.json 못 찾음")
		return
	}
	p.sid, _ = p.sess["sessionId"].(string)
	p.add("세션 파일", pOK, "sessionId "+p.sid)
	if v, _ := p.sess["version"].(string); v != "" {
		p.add("판 읽기", pOK, "version "+v)
	} else {
		p.add("판 읽기", pBad, "세션 파일에 version 없음")
	}
	var bg string
	if !p.until(probeWait, func() bool {
		for _, f := range p.tasks() {
			if !p.trailer(f) && p.isHeld(f) {
				bg = f
			}
		}
		return bg != "" && p.turnEnded()
	}) {
		p.add("① 도는 배경 셸", pUnk, "턴을 끝내고 배경 셸이 도는 꼴을 상한 안에 못 봄(끝 표시 없고 쥔 출력 파일)")
		return
	}
	p.files()
	p.scene("① 도는 배경 셸", rest.Busy, "")
	scr := p.r(30*time.Second, nil, "hc", "read", p.addr)
	p.add("화면 배경 작업 글자", pRef, map[bool]string{true: "「shell still running」 보임", false: "「shell still running」 안 보임"}[strings.Contains(scr.out, "still running")])
	form := ""
	if !p.until(time.Duration(probeBgSec)*time.Second+probeWait, func() bool {
		switch {
		case p.trailer(bg):
			form = " · 끝 표시 붙음"
		case !p.isHeld(bg):
			form = " · 쥔 프로세스 없음"
		default:
			return false
		}
		return p.turnEnded()
	}) {
		p.add("② 끝난 배경 셸", pBad, "끝 표시도 안 붙고 쥔 프로세스도 그대로(또는 턴이 안 끝남)")
		return
	}
	p.scene("② 끝난 배경 셸", rest.Quiet, form)
	before := map[string]bool{}
	for _, f := range p.tasks() {
		before[f] = true
	}
	if res := p.r(30*time.Second, nil, "hc", "send", p.addr, probeFgMsg()); res.err != nil || res.rc != 0 {
		p.add("③ 도는 전경 명령", pUnk, "hc send 실패")
		return
	}
	var fg string
	if !p.until(probeWait, func() bool {
		for _, f := range p.tasks() {
			if !before[f] && p.isHeld(f) {
				fg = f
			}
		}
		return fg != ""
	}) {
		p.add("③ 도는 전경 명령", pUnk, "새 출력 파일을 상한 안에 못 봄")
		return
	}
	p.scene("③ 도는 전경 명령", rest.Busy, "")
	p.killFg(fg)
	form = ""
	if !p.until(probeWait, func() bool { return p.turnEnded() && !p.isHeld(fg) }) {
		p.add("④ 죽은 전경 명령 뒤", pUnk, "끈 뒤 턴이 상한 안에 안 끝남")
		return
	}
	switch _, err := os.Stat(fg); {
	case os.IsNotExist(err):
		form = " · 파일을 Claude가 지움"
	case p.trailer(fg):
		form = " · 끝 표시 붙음"
	default:
		form = " · 끝 표시 없이 남음"
	}
	p.scene("④ 죽은 전경 명령 뒤", rest.Quiet, form)
}

var pyRe = regexp.MustCompile(`(?i)python`)

// killFg: 그 출력 파일을 쥔 프로세스 가운데 명령 줄에 이 점검 표식이 든 파이썬만 그 pid로 끈다(이 점검이 띄운 것만)
func (p *prober) killFg(f string) {
	res := p.r(30*time.Second, nil, "lsof", "-t", "--", f)
	for _, s := range strings.Fields(res.out) {
		pid, err := strconv.Atoi(s)
		if err != nil || pid <= 1 {
			continue
		}
		if c := p.cmdOf(pid); strings.Contains(c, "time.sleep(") && pyRe.MatchString(c) && !strings.Contains(c, "zsh") {
			p.kill(pid)
		}
	}
}

// verdict: 결과 — 틀림이 하나라도(참고 빼고) fail, 아니면 못 봄이 있으면 unknown, 모두 맞음이면 pass
func probeVerdict(rows []pRow) string {
	out := "pass"
	for _, r := range rows {
		switch r.res {
		case pBad:
			return "fail"
		case pUnk:
			out = "unknown"
		}
	}
	if len(rows) == 0 {
		return "unknown"
	}
	return out
}

// probeApply: 결과를 잰 묶음(id — 점검 세션의 판) 기록에, 사건은 통과 :up 한 번·틀림 막힘 한 번(같은 묶음에 되풀이 안 함). 점검이
// 맡은 묶음(asked — 시작 때 Owed)이 잰 것과 다르면 그 묶음은 잴 수 없음(설치본과 다른 판으로 도는 옛 세션 — 감사 atp89b9 중간 1).
// 찍을 줄
func probeApply(machine, path, asked, id, verdict string, rows []pRow, report func(st, msg, key string) bool) []string {
	var lines []string
	var tbl []string
	var bad []string
	for _, r := range rows {
		tbl = append(tbl, r.item+"\t"+r.res+"\t"+r.detail)
		if r.res == pBad {
			bad = append(bad, r.item)
		}
	}
	err := upstream.Update(path, func(rec *upstream.Rec) *upstream.Rec {
		if rec == nil {
			rec = &upstream.Rec{Bundles: map[string]*upstream.Bundle{}}
		}
		b := rec.Bundles[id]
		if b == nil {
			b = &upstream.Bundle{State: upstream.Pending, Seen: time.Now().Format(time.RFC3339)}
			rec.Bundles[id] = b
		}
		if a := rec.Bundles[asked]; a != nil && asked != id && verdict != "unknown" {
			a.Skip = true
			lines = append(lines, fmt.Sprintf("맡은 묶음 %s는 설치본(%s)과 판이 달라 점검으로 잴 수 없음 — 다시 띄우지 않는다(그 세션이 끝나면 보류도 끝)", asked, id))
		}
		b.Probed, b.Result, b.Rows = time.Now().Format(time.RFC3339), verdict, tbl
		key := "hc-upstream:" + machine + ":" + id
		switch verdict {
		case "pass":
			b.State = upstream.Trusted
			if !b.HasTold("up") {
				msg := fmt.Sprintf("실물 점검 통과(%s): %s — 이 판의 터미널 닫기는 평소대로", machine, id)
				if report("중간", msg, key+":up") {
					b.Told = append(b.Told, "up")
				}
				lines = append(lines, msg)
			}
		case "fail":
			b.State = upstream.Failed
			if !b.HasTold("fail") {
				msg := fmt.Sprintf("실물 점검 틀림(%s): %s — 틀린 항목 %s — 이 기계에서 이 판 터미널 닫기 보류 중(R3는 그대로) — 사람이 볼 것: 그 기계 hc-sweep --upstream 표의 틀림 줄(도구가 기대는 Claude·hc 동작이 바뀌었다 — 고치는 레인)",
					machine, id, strings.Join(bad, "·"))
				if report("막힘", msg, key) {
					b.Told = append(b.Told, "fail")
				}
				lines = append(lines, msg)
			}
		default:
			lines = append(lines, fmt.Sprintf("실물 점검 결과 모름(%s): %s — 기록 그대로, 대기열이 상한 안에서 다시", machine, id))
		}
		return rec
	})
	if err != nil {
		lines = append(lines, "판 기록을 못 씀: "+err.Error())
	}
	return lines
}

// probeLeftover: 이번 점검이 만든 임시 폴더(dir — 실행마다 새것)를 cwd로 쓰는 살아 있는 점검용 세션 주소(없거나 모르면 빈 값)
func probeLeftover(r runner, dir string) string {
	res := r(30*time.Second, nil, "hc", "ls", "--json")
	if res.err != nil || res.rc != 0 {
		return ""
	}
	l, err := hcjson.ParseList([]byte(res.out))
	if err != nil {
		return ""
	}
	want, _ := filepath.EvalSymlinks(dir)
	for _, t := range l.Terminals {
		got, _ := filepath.EvalSymlinks(t.Cwd)
		if t.Name == probeName && t.State == "live" && (t.Cwd == dir || got != "" && got == want) {
			return t.Address
		}
	}
	return ""
}

// probeMain: hc-sweep --upstream-probe -- <에이전트 실행 명령…>({probe} 자리에 점검 지시). 종료 0 통과 · 1 틀림 · 3 결과 없음(못 봄·못 띄움) · 2 잘못 부름
func probeMain(args []string) int {
	if len(args) < 2 || args[0] != "--" {
		fmt.Fprintln(os.Stderr, "사용법: hc-sweep --upstream-probe -- <에이전트 실행 명령…>  ({probe} 자리에 점검 지시가 든다)")
		return 2
	}
	argv := append([]string(nil), args[1:]...)
	for i := range argv {
		argv[i] = strings.ReplaceAll(argv[i], "{probe}", probePrompt())
	}
	lim = sweepKnobs(time.Now())
	machine := sweepMachine()
	rec0, _ := upstream.Load(upstream.Path())
	asked, _ := rec0.Owed() // 이 점검이 맡은 묶음(잰 판과 다르면 잴 수 없음 — probeApply)
	dir, err := os.MkdirTemp("", "upstream-probe.")
	if err != nil {
		fmt.Fprintln(os.Stderr, "hc-sweep: 임시 폴더:", err)
		return 3
	}
	p := &prober{r: run, end: time.Now().Add(probeTotal), words: map[string]bool{},
		held: func() map[string]bool { return openPaths(run, 30*time.Second) },
		kill: hardKill,
		cmdOf: func(pid int) string {
			res := run(10*time.Second, nil, "ps", "-o", "command=", "-p", strconv.Itoa(pid))
			return res.out
		}}
	res := run(3*time.Minute, nil, filepath.Join(binDir, "hc-lane"), append([]string{"--read", "--repo", dir, "--name", probeName, "--"}, argv...)...)
	for _, l := range strings.Split(strings.TrimSpace(res.out), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil {
			if a, ok := m["lane"].(string); ok && m["name"] == probeName {
				p.addr = a
			}
		}
	}
	if p.addr == "" {
		fmt.Printf("점검용 세션을 못 띄움(hc-lane 종료 %d): %s\n", res.rc, cutRunes(strings.TrimSpace(res.out+" "+res.errs), 300))
		// 떴는데 출력을 못 받았을 수 있다(시작 확인 시간 넘김 등 — 감사 atp89b9 낮음 1): 이번에 만든 임시 폴더에서 도는 그 이름만 닫는다
		if a := probeLeftover(run, dir); a != "" {
			if k := run(30*time.Second, nil, "hc", "kill", a); k.err == nil && k.rc == 0 {
				fmt.Println("남은 점검용 세션을 닫음: " + a)
			}
		}
		return 3
	}
	p.drive()
	if p.getEr != "" {
		p.add("hc get 칸", pBad, p.getEr)
	} else {
		var ws []string
		for w := range p.words {
			ws = append(ws, w)
		}
		p.add("hc get 칸", pOK, "pid·state·status_ms·created_ms, 본 상태 "+strings.Join(ws, ","))
	}
	if k := run(30*time.Second, nil, "hc", "kill", p.addr); k.err != nil || k.rc != 0 { // 이 점검이 띄운 점검용 세션만
		fmt.Println("점검용 세션을 못 닫음: " + p.addr)
	}
	hcv := hcVersion(run)
	cli, _ := p.sess["version"].(string)
	id := upstream.ID(cli, hcv)
	if id == "" { // 세션 판을 못 읽음 — 이 기계가 점검을 기다리던 묶음에
		id = asked
	}
	verdict := probeVerdict(p.rows)
	fmt.Printf("실물 점검 %s · 판 묶음 %s · 기계 %s\n", verdict, dashS(id), machine)
	fmt.Println("항목\t결과\t자세히")
	for _, r := range p.rows {
		fmt.Println(r.item + "\t" + r.res + "\t" + r.detail)
	}
	if id != "" {
		lane := filepath.Join(binDir, "hc-lane")
		rep := func(st, msg, key string) bool { return laneReportAs(run, lane, st, "hc-upstream", msg, key) }
		for _, l := range probeApply(machine, upstream.Path(), asked, id, verdict, p.rows, rep) {
			fmt.Println(l)
		}
	}
	switch verdict {
	case "pass":
		return 0
	case "fail":
		return 1
	}
	return 3
}
