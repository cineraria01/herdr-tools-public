package main

// 운영 에이전트의 관문과 명령(2026-11-24 ops-agent, docs/ops/agent.md).
// agentGate: 모든 하위 명령 앞(main) — 운영 에이전트 실행(환경 HC_FILL_AGENT_RUN, 또는 도는 실행의 터미널)이 부른 것이면
// 읽는 꼴(허용 목록 oaReads)은 그대로, 넘길 것 종류(레인 지우기·ops_agent* 손잡이·--force·machine --rename·agent run·바퀴·속 명령)는
// 거절하고 「사람이 정한다」 사건, 끝난 실행·실행 상한 지남·꺼짐이면 거절, 「판단만」이면 일지에 「하려던 것」만 적고 안 한다. 사람이 부른 명령은 건드리지 않는다
// agent 명령: status [--json] · run [--why <까닭>]

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

func init() { register("agent", agentCmd) }

func agentGate(cmd string, args []string) {
	id := os.Getenv(oaEnvRun)
	st, err := oaLoad()
	if id == "" {
		// 표식이 없으면 도는 실행의 터미널인지로 — 이 기계 주소(「tN」)일 때만. 다른 기계 주소(「기계/tN」)는 터미널 ID가 기계마다 따로라
		// 이 기계의 같은 ID 사람 터미널을 에이전트로 잘못 본다(감사 atozu10 낮음) — 그때는 표식만 믿는다
		tid := os.Getenv("HC_TERMINAL_ID")
		if err != nil || st.Run == nil || tid == "" || strings.Contains(st.Run.Addr, "/") || oaTerm(tid) != st.Run.Addr {
			return
		}
		id = st.Run.ID
	}
	if oaReadOnly(cmd, args) {
		return
	}
	line := strings.Join(append([]string{"hc-fill", cmd}, args...), " ")
	if why := oaHandoff(cmd, args); why != "" {
		jotBy("운영 에이전트 "+id, "거절", cmd, why, "사람에게 넘김", "-", oaFlat(line))
		hcLaneReport("막힘", oaFlat("사람이 정한다: 운영 에이전트("+id+")가 「"+line+"」를 치려 함 — "+why+". 권함: 필요하면 사람이 직접 친다"), "hc-fill-agent", "", "")
		dieNext("결과 파일에 「사람이 정한다: …」로 적고 다음 일로", "운영 에이전트는 이 명령을 치지 않는다(%s) — 사람에게 넘겼다", why)
	}
	switch {
	case err != nil:
		die("%v — 조치 명령을 받지 않는다", err)
	case st.Run == nil || st.Run.ID != id:
		die("이 운영 에이전트 실행(%s)은 끝났다(시간 넘김·끝 처리) — 조치 명령을 받지 않는다", id)
	case time.Now().Unix() >= st.Run.Deadline:
		// 바퀴가 아직 끝 처리를 안 했어도(바퀴가 멈췄어도) 상한이 지났으면 끝난 실행이다(ops-gate-tighten)
		die("이 운영 에이전트 실행(%s)은 실행 상한 지남(%s) — 조치 명령을 받지 않는다", id, time.Unix(st.Run.Deadline, 0).Format("15:04"))
	}
	switch oaMode() {
	case "조치":
		return
	case "판단만":
		_, why := takeWhy(args)
		jotBy("운영 에이전트 "+id, "하려던 것", cmd, why, "안 함(판단만)", "-", oaFlat(line))
		say("판단만 — 하지 않고 일지에 「하려던 것」으로 적었다: %s", line)
		exit(0)
	}
	die("운영 에이전트가 꺼졌다(손잡이 ops_agent 끔) — 조치 명령을 받지 않는다")
}

// oaTerm: 터미널 주소의 마지막 조각(「기계/t5」와 「t5」를 같게)
func oaTerm(a string) string { return a[strings.LastIndexByte(a, '/')+1:] }

// oaDry: --dry-run을 실제로 받는 명령(takeDry가 인자 어디서든 떼어 미리 보기로 끝냄 · run은 바퀴 미리 보기). 다른 명령은 그 글자를
// 값으로 읽어 본 동작을 하니(감사 atozu10 중간 — nudge <레인> --dry-run이 글을 보냄) 읽기로 보지 않는다
var oaDry = map[string]bool{"prio": true, "rm": true, "knob": true, "machine": true, "machines": true, "run": true}

// oaRead: 읽는 꼴 — 하위 명령(sub, 있으면 첫 인자)과 받는 인자(flags 값 없음 · vals 값 하나). 여기 없는 인자가 하나라도 있으면
// 쓰기로 본다(허용 목록 — 쓰기 인자 목록을 두면 새 쓰기 인자 machine --rename이 읽기로 샜다, ops-gate-tighten)
type oaRead struct {
	sub         string
	flags, vals []string
}

var oaReads = map[string]oaRead{
	"ls":       {flags: []string{"--json"}},
	"status":   {flags: []string{"--mine"}, vals: []string{"--owner"}},
	"state":    {flags: []string{"--json", "--all"}},
	"advise":   {flags: []string{"--json"}},
	"journal":  {flags: []string{"--json"}, vals: []string{"--by", "--since"}},
	"knob":     {sub: "ls", flags: []string{"--json"}},
	"agent":    {sub: "status", flags: []string{"--json"}},
	"machine":  {flags: []string{"--json"}},
	"machines": {flags: []string{"--json"}},
	"merger":   {sub: "status"}, // hc-merger status가 부른다 — restart는 조치
}

// oaReadOnly: 아무것도 바꾸지 않는 부름 — 읽는 꼴(oaReads)과 미리 보기(아무것도 안 쓴다)
func oaReadOnly(cmd string, args []string) bool {
	for _, a := range args {
		if a == "--dry-run" && oaDry[cmd] {
			return true
		}
	}
	f, ok := oaReads[cmd]
	if !ok {
		return false
	}
	if f.sub != "" {
		if len(args) == 0 || args[0] != f.sub {
			return false
		}
		args = args[1:]
	}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case has(f.flags, a):
		case has(f.vals, a) && i+1 < len(args):
			i++
		default:
			return false
		}
	}
	return true
}

// oaHandoff: 운영 에이전트가 스스로 하지 않을 것(되돌릴 수 없음·남의 것·자기 권한)의 까닭, 아니면 빈 값
func oaHandoff(cmd string, args []string) string {
	switch {
	case cmd == "rm":
		return "레인 지우기는 사람이 정한다"
	case cmd == "agent":
		return "운영 에이전트 안에서 운영 에이전트를 띄우지 않는다"
	case cmd == "run" || cmd == "watch" || strings.HasPrefix(cmd, "_"):
		return "바퀴·속 명령은 hc-fill이 돈다"
	case (cmd == "machine" || cmd == "machines") && has(args, "--rename"):
		return "기계 이름 바꾸기는 사람이 정한다(설정·상태·병합기 표식을 한 번에 바꿔 되돌리기 어렵다)"
	case cmd == "knob" || cmd == "machine" || cmd == "machines":
		for i := 1; i < len(args); i++ { // 값을 받는 인자의 값(--why 글 등)은 건너뛴다
			switch a := args[i]; {
			case a == "--why" || a == "--for" || a == "--until" || a == "--seats":
				i++
			case a == "--force":
				return "--force(고정한 손잡이 덮기)는 사람이 정한다"
			case cmd == "knob" && strings.HasPrefix(a, "ops_agent"):
				return "자기 손잡이(ops_agent*)는 사람이 바꾼다"
			}
		}
	}
	return ""
}

func agentCmd(args []string) {
	if len(args) == 0 {
		usage()
	}
	switch args[0] {
	case "status":
		agentStatus(args[1:])
	case "run":
		rest, why := takeWhy(args[1:])
		if len(rest) > 0 {
			usage()
		}
		agentRun(why)
	default:
		usage()
	}
}

// agentRun: 사람이 지금 한 바퀴 — 바퀴와 같은 잠금 안에서(한 번에 하나), 시도 기록·시간당 상한과 상관없이 지금 손댈 것 전부를
func agentRun(why string) {
	if err := os.MkdirAll(oaDir(), 0o755); err != nil {
		die("%v", err)
	}
	lf := lockWait(oaDir()+"/lock", 90*time.Second)
	if lf == nil {
		die("hc-fill 바퀴가 운영 에이전트를 띄우는 중이다 — 잠시 뒤 hc-fill agent status")
	}
	defer lf.Close()
	st, err := oaLoad()
	if err != nil {
		die("%v", err)
	}
	now := time.Now()
	if st.Run != nil {
		oaFinish(&st, now)
		oaSave(st)
		if st.Run != nil {
			die("도는 운영 에이전트가 있다: %s(%s, %s까지) — 한 번에 하나", st.Run.ID, st.Run.Addr, time.Unix(st.Run.Deadline, 0).Format("15:04"))
		}
	}
	if oaMode() == "끔" {
		dieNext("hc-fill knob set ops_agent 판단만|조치 --why <까닭>", "운영 에이전트가 꺼져 있다(손잡이 ops_agent 끔)")
	}
	err = oaStart(&st, now, true, why)
	oaSave(st)
	if err != nil {
		die("운영 에이전트를 띄우지 못했다: %v", err)
	}
}

type oaStatusOut struct {
	Mode      string            `json:"mode"`
	Launch    bool              `json:"launch"`
	Template  bool              `json:"template"`
	Run       *oaRun            `json:"run"`
	HourCount int               `json:"hour_count"`
	HourMax   int               `json:"hour_max"`
	Timeout   int               `json:"timeout_min"`
	Tries     int               `json:"tries_max"`
	Scope     string            `json:"scope"`
	Keys      map[string]*oaKey `json:"keys"`
	Recent    []oaDone          `json:"recent"`
	Fails     int               `json:"launch_fails"` // 못 띄움이 이어진 수(뜨면 0)
	Left      string            `json:"left"`         // 끝난 실행의 터미널 가운데 아직 못 닫은 주소(없으면 "")
}

func agentStatus(args []string) {
	js := false
	for _, a := range args {
		if a != "--json" || js {
			usage()
		}
		js = true
	}
	st, err := oaLoad()
	if err != nil {
		die("%v", err)
	}
	now := time.Now().Unix()
	o := oaStatusOut{Mode: oaMode(), Launch: confRaw(oaLaunchK) != "", Template: isFile(oaTmplPath()), Run: st.Run,
		HourMax: knobInt("ops_agent_max_hour"), Timeout: knobInt("ops_agent_timeout"), Tries: knobInt("ops_agent_tries"),
		Scope: dash(conf("ops_agent_scope")), Keys: st.Keys, Recent: st.Recent, Fails: st.LaunchFails, Left: st.Left}
	for _, t := range st.Launches {
		if now-t < 3600 {
			o.HourCount++
		}
	}
	if o.Recent == nil {
		o.Recent = []oaDone{}
	}
	if js {
		b, err := json.Marshal(o)
		if err != nil {
			die("%v", err)
		}
		fmt.Println(string(b))
		return
	}
	yn := map[bool]string{true: "있음", false: "없음"}
	say("운영 에이전트: %s · 띄울 명령 %s · 틀 %s · 시간당 %d/%d · 상한 %d분 · 같은 항목 %d번 · 범위 %s", o.Mode, yn[o.Launch], yn[o.Template],
		o.HourCount, o.HourMax, o.Timeout, o.Tries, o.Scope)
	if o.Mode != "끔" && (!o.Launch || !o.Template) {
		say("  켜졌지만 띄우지 못한다 — 설정 %s 줄과 틀 %s가 둘 다 있어야", oaLaunchK, oaTmplPath())
	}
	if r := st.Run; r != nil {
		say("  도는 중: %s → %s (%s, %s부터 %s까지) 손댈 것 %s", r.ID, r.Addr, r.Mode, time.Unix(r.Started, 0).Format("15:04"),
			time.Unix(r.Deadline, 0).Format("15:04"), strings.Join(r.Keys, ", "))
	} else {
		say("  도는 것 없음")
	}
	if o.Fails > 0 {
		say("  못 띄움 %d번 이어짐 — 까닭은 hc-fill journal의 「agent 띄우기」 실패 줄과 실행 폴더의 launch.err", o.Fails)
	}
	if o.Left != "" {
		say("  못 닫은 앞 실행 터미널 %s — 다음 띄우기 직전에 다시 닫아 본다", o.Left)
	}
	for k, x := range st.Keys {
		say("  항목 %s: %d번 띄움%s%s", k, x.Tries, map[bool]string{true: " · 끝까지 봄", false: ""}[x.Done], map[bool]string{true: " · 사람에게 넘김", false: ""}[x.Asked])
	}
	for _, d := range st.Recent {
		say("  %s %s: %s — %s", time.Unix(d.End, 0).Format("01-02 15:04"), d.ID, d.Result, d.Summary)
	}
}
