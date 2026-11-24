package main

// 운영 에이전트(2026-11-24 ops-agent, docs/ops/agent.md — 사용자 「난 지금과 같은 판단 자체를 에이전트가 빠르게 최대한 해서 난 문제 없이
// 빠르게 모두 완료되도록 하는게 허더툴스의 기능이야」): 지금 손댈 것(advise)에 새 항목이 생기면 바퀴가 에이전트를 한 번 띄워
// 판단·조치하게 하고(상주 대화가 아니라 한 바퀴), 끝나면 결과 한 줄을 일지·사건으로 남긴다. 도구는 띄우기·끝 처리·권한 선만 —
// 판단은 에이전트가, 켜고 끄기·문턱은 손잡이(ops_agent*)가 정한다. 관문(조치 명령 거르기)은 opsagent_gate.go

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"herdr-tools/internal/knob"
	"herdr-tools/internal/lockx"
	"herdr-tools/internal/opsstate"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/toolcfg"
)

const (
	oaEnvRun  = "HC_FILL_AGENT_RUN" // 운영 에이전트 실행 표식 — 띄울 명령이 에이전트 환경까지 넘긴다
	oaName    = "hc-fill-agent"     // 띄우는 터미널 이름
	oaTmpl    = "ops-agent.md"      // 설정 폴더의 지시문 틀(본보기 config.example/)
	oaLaunchK = "ops_agent_launch"  // 설정 줄: 띄울 명령(sh -c)
	oaRecent  = 10                  // status가 보이는 최근 바퀴 수
	oaGrace   = 120                 // 띄운 뒤 이 초 안의 done·idle은 끝으로 보지 않는다(첫 턴이 아직 안 시작했을 수 있다)
)

type oaRun struct {
	ID       string   `json:"id"`
	Addr     string   `json:"addr"`
	Mode     string   `json:"mode"`
	Dir      string   `json:"dir"`
	Hash     string   `json:"hash"` // 띄울 명령·틀 지문 — 실행 중에 바뀌면 끔으로 두고 사람에게
	By       string   `json:"by"`
	Started  int64    `json:"started"`
	Deadline int64    `json:"deadline"`
	Keys     []string `json:"keys"`
}

type oaKey struct {
	Tries   int    `json:"tries"` // 이 항목으로 띄운 수
	Done    bool   `json:"done"`  // 에이전트가 끝까지 본 적 있음 — 그대로 남아도 다시 띄우지 않는다
	Asked   bool   `json:"asked"` // 「사람이 정한다」를 보냈음
	Summary string `json:"summary"`
}

type oaDone struct {
	ID      string   `json:"id"`
	Start   int64    `json:"start"`
	End     int64    `json:"end"`
	Result  string   `json:"result"`
	Summary string   `json:"summary"`
	Keys    []string `json:"keys"`
}

type oaState struct {
	Run      *oaRun            `json:"run"`
	Keys     map[string]*oaKey `json:"keys"`
	Launches []int64           `json:"launches"` // 띄운 시각(초) — 시간당 상한
	Checked  int64             `json:"checked"`  // 손댈 것을 마지막으로 본 시각(advise_every 간격)
	Recent   []oaDone          `json:"recent"`
	// 못 띄움(띄울 명령 실패)이 이어진 수와 그 운영 사건을 보냈나 — 시도(oaKey.Tries)로 세지 않는다. 뜨면 비운다
	LaunchFails int  `json:"launch_fails,omitempty"`
	FailSent    bool `json:"fail_sent,omitempty"`
	// Left: 끝난 실행의 터미널 주소 가운데 아직 닫지 못한 것(닫기를 모름으로 못 함) — 다음 띄우기 직전에 한 번 더
	Left string `json:"left,omitempty"`
}

func oaDir() string       { return DATA + "/ops-agent" }
func oaStatePath() string { return oaDir() + "/state.json" }

// oaLoad: 상태 파일. 없으면 빈 상태, 깨졌으면 오류(모름 — 「도는 것 없음」으로 읽지 않는다)
func oaLoad() (oaState, error) {
	st := oaState{Keys: map[string]*oaKey{}}
	b, err := os.ReadFile(oaStatePath())
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, fmt.Errorf("운영 에이전트 상태를 못 읽음: %w", err)
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, fmt.Errorf("운영 에이전트 상태가 깨짐(%s): %w", oaStatePath(), err)
	}
	if st.Keys == nil {
		st.Keys = map[string]*oaKey{}
	}
	return st, nil
}

func oaSave(st oaState) {
	b, err := json.MarshalIndent(st, "", "  ")
	if err == nil {
		err = os.MkdirAll(oaDir(), 0o755)
	}
	if err == nil {
		err = writeAtomic(oaStatePath(), string(b)+"\n")
	}
	if err != nil {
		say("운영 에이전트: 상태를 쓰지 못했다: %v", err)
	}
}

// oaMode: 손잡이 ops_agent — 끔·판단만·조치 말고는 끔
func oaMode() string {
	switch v := conf("ops_agent"); v {
	case "판단만", "조치":
		return v
	}
	return "끔"
}

func oaTmplPath() string { return filepath.Dir(CONF) + "/" + oaTmpl }

// oaHash: 띄울 명령·틀의 지문(틀을 못 읽으면 빈 값)
func oaHash() string {
	t, ok := readFile(oaTmplPath())
	if !ok {
		return ""
	}
	return sha256hex([]byte(confRaw(oaLaunchK) + "\n" + t))
}

// opsAgentTick: run 바퀴 단계 — 도는 실행의 끝을 보고, 없으면 새 손댈 것이 있을 때 한 번 띄운다. 무엇이 실패해도 바퀴를 막지 않는다
func opsAgentTick(now time.Time) {
	if dry {
		return
	}
	if err := os.MkdirAll(oaDir(), 0o755); err != nil {
		say("운영 에이전트: %v", err)
		return
	}
	lf, busy, err := lockx.TryLock(oaDir() + "/lock")
	if err != nil || busy {
		return // 사람이 부른 agent run이 띄우는 중 — 다음 바퀴에
	}
	defer lf.Close()
	st, err := oaLoad()
	if err != nil {
		say("운영 에이전트: %v — 이번 바퀴는 건너뜀", err)
		return
	}
	if st.Run != nil {
		oaFinish(&st, now)
		oaSave(st)
		return
	}
	if oaMode() == "끔" {
		return
	}
	if every := knobInt("advise_every"); every > 0 && now.Unix()-st.Checked < int64(every)*60 && now.Unix() >= st.Checked {
		return
	}
	st.Checked = now.Unix()
	if err := oaStart(&st, now, false, ""); err != nil {
		say("운영 에이전트: %v — 다음 바퀴에 다시(상한까지)", err)
	}
	oaSave(st)
}

// oaStart: 손댈 것을 모아 띄울지 정하고 띄운다(띄우면 st.Run). manual이면 시도 기록·시간당 상한과 상관없이 지금 목록 전부를 맡긴다.
// 띄우려다 못 띄웠으면 오류(안 띄울 까닭이 있어 안 띄운 것은 nil)
func oaStart(st *oaState, now time.Time, manual bool, why string) error {
	launch := confRaw(oaLaunchK)
	tp := oaTmplPath()
	tmpl, ok := readFile(tp)
	if launch == "" || !ok {
		if manual {
			die("운영 에이전트를 띄울 설정이 없다 — 설정 %s 줄과 틀 %s(본보기 config.example/)", oaLaunchK, tp)
		}
		return nil
	}
	s, f := stateCollectF(now)
	for k := range st.Keys { // 이미 맡긴 항목은 보류(배포 밀림) 동안에도 남긴다 — 빠지면 시도 기록을 잊는다(감사 atozuyd 중간)
		f.Known[k] = true
	}
	tr := opsstate.Trim(s, knobInt("state_done"))
	all := opsstate.Advise(tr, f, stateThresholds())
	tr.Notable = opsstate.Summaries(all)
	tr.Fingerprint = opsstate.Fingerprint(tr)
	items := oaScoped(all)
	seen := map[string]bool{}
	var hand []string
	tries := knobInt("ops_agent_tries")
	for _, it := range items {
		seen[it.Key] = true
		k := st.Keys[it.Key]
		if k == nil {
			k = &oaKey{}
			st.Keys[it.Key] = k
		}
		k.Summary = it.Summary()
		switch {
		case manual || !k.Done && k.Tries < tries:
			hand = append(hand, it.Key)
		case !k.Done && !k.Asked:
			msg := fmt.Sprintf("사람이 정한다: %s — 운영 에이전트를 %d번 띄웠는데 못 풀림(시간 넘김·결과 없이 끝남). 권함: hc-fill agent status로 까닭을 보고 직접 조치하거나 손잡이 ops_agent_tries를 늘린다", k.Summary, k.Tries)
			if hcLaneReport("막힘", oaFlat(msg), "hc-fill-agent", "", "") == 0 {
				k.Asked = true
			}
		}
	}
	var unk []string
	for _, it := range all {
		if it.Kind == "unknown" {
			unk = append(unk, strings.TrimPrefix(it.Key, "unknown:"))
		}
	}
	for key := range st.Keys {
		// 사라진 항목은 잊는다 — 다시 생기면 새 항목(advise와 같은 뜻). 다만 목록을 읽었고 없을 때만: 그 항목을 가리는 「판정 못 함」이
		// 있으면 모름이라 그대로 둔다(지웠더니 회복 뒤 처음인 것처럼 다시 띄워 ops_agent_tries가 안 먹었다, ops-gate-tighten)
		if !seen[key] && !oaHidden(unk, key) {
			delete(st.Keys, key)
		}
	}
	if len(hand) == 0 {
		if manual {
			say("손댈 것 없음 — 안 띄움")
		}
		return nil
	}
	var recent []int64
	for _, t := range st.Launches {
		if now.Unix()-t < 3600 {
			recent = append(recent, t)
		}
	}
	st.Launches = recent
	if max := knobInt("ops_agent_max_hour"); !manual && len(recent) >= max {
		say("운영 에이전트: 시간당 상한 %d번(손잡이 ops_agent_max_hour) — 새 손댈 것 %d개는 다음에", max, len(hand))
		return nil
	}
	if st.Left != "" { // 끝 처리에서 닫지 못한 앞 실행의 터미널 — 같은 이름이 살아 있으면 띄울 명령이 name_taken으로 실패한다
		oaClose(st, st.Left, "앞 실행")
	}
	st.Launches = append(st.Launches, now.Unix())
	id := fmt.Sprintf("oa-%s-%d", now.Format("20060102-150405"), os.Getpid())
	dir := oaDir() + "/" + id
	work := oaDir() + "/work"
	mode := oaMode()
	timeout := knobInt("ops_agent_timeout")
	run := oaRun{ID: id, Mode: mode, Dir: dir, Hash: oaHash(), By: "hc-fill 스스로", Started: now.Unix(),
		Deadline: now.Add(time.Duration(timeout) * time.Minute).Unix(), Keys: hand}
	if manual {
		run.By = knobBy()
	}
	brief, err := oaWrite(run, tr, all, tmpl, work)
	target := strings.Join(hand, " ")
	if err != nil {
		say("운영 에이전트: 지시문을 쓰지 못했다: %v — 다음에", err)
		jotBy(run.By, "agent 띄우기", target, why, "실패: 지시문", "-", err.Error())
		return fmt.Errorf("지시문을 쓰지 못했다: %w", err)
	}
	out, errOut, rc := oaLaunch(launch, []string{oaEnvRun + "=" + id, "HC_FILL_AGENT_NAME=" + oaName, "HC_FILL_AGENT_DIR=" + work, "HC_FILL_AGENT_BRIEF=" + brief})
	addr, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	addr = strings.TrimSpace(addr)
	if rc != 0 || addr == "" {
		// 못 띄움은 시도(oaKey.Tries — 떠서 못 푼 것)로 세지 않는다: 띄울 명령이 고장 나면 모든 새 항목이 곧바로 사람에게 넘어간다.
		// 대신 이어진 수를 세어 운영 사건 한 번(뜰 때까지 되풀이 없음). 시간당 상한은 위에서 못 띄운 것도 센다(폭주 막기)
		st.LaunchFails++
		why1, full := oaLaunchWhy(addr, rc, out, errOut)
		if err := writeAtomic(dir+"/launch.err", full); err != nil {
			say("운영 에이전트: %s/launch.err를 쓰지 못했다: %v", dir, err)
		}
		say("운영 에이전트: 띄울 명령 종료 %d(주소 %q) — 다음 바퀴에 다시(못 띄움 %d번 이어짐): %s", rc, addr, st.LaunchFails, why1)
		jotBy(run.By, "agent 띄우기", target, why, fmt.Sprintf("실패: 종료 %d — %s", rc, oaClip(strings.Join(strings.Fields(why1), " "), 200)), "-", "방식 "+mode+" · "+dir+"/launch.err")
		if !st.FailSent && st.LaunchFails >= tries {
			first, _, _ := strings.Cut(why1, "\n")
			msg := fmt.Sprintf("운영 에이전트를 못 띄움 %d번 이어짐 — %s. 권함: %s/launch.err와 설정 %s 줄을 보고 고친다(뜨면 이 셈은 비고, 그때까지 되풀이하지 않는다)", st.LaunchFails, first, dir, oaLaunchK)
			if hcLaneReport("막힘", oaFlat(msg), "hc-fill-agent", "", "") == 0 {
				st.FailSent = true
			}
		}
		return fmt.Errorf("띄울 명령 종료 %d(주소 %q)", rc, addr)
	}
	for _, key := range hand {
		st.Keys[key].Tries++
	}
	st.LaunchFails, st.FailSent = 0, false
	run.Addr = addr
	st.Run = &run
	say("운영 에이전트 띄움 %s → %s (방식 %s, 손댈 것 %d개, 상한 %d분)", id, addr, mode, len(hand), timeout)
	jotBy(run.By, "agent 띄우기", target, why, "띄움 "+addr, "-", "실행 "+id+" · 방식 "+mode)
	return nil
}

// oaHidden: 이번 목록의 「판정 못 함」(unknown:<u>, 머리 뗀 u들)이 항목 key를 가릴 수 있나 — opsstate.Advise의 unknown() 자리와 짝:
// 재료 deploy는 배포 항목만(이 기계에서 배포한 적 없으면 늘 모름이라 전부를 붙들면 아무것도 못 잊는다), 다른 재료(hc 등)는 전부,
// 병합기 P는 merger·merge-*:P, 상주는 resident:*, 기계 M은 machine:M과 skew, 배포·감시는 같은 열쇠. 모르는 종류는 전부(모름은 지킨다)
func oaHidden(unk []string, key string) bool {
	kind, name, _ := strings.Cut(key, ":")
	for _, u := range unk {
		uk, un, _ := strings.Cut(u, ":")
		switch uk {
		case "source":
			if un != "deploy" || kind == "deploy" {
				return true
			}
		case "merger":
			if strings.HasPrefix(kind, "merge") && name == un {
				return true
			}
		case "resident":
			if kind == "resident" {
				return true
			}
		case "machine":
			if key == u || key == "skew" {
				return true
			}
		case "deploy", "watch":
			if key == u {
				return true
			}
		default:
			return true
		}
	}
	return false
}

// oaLaunch: 띄울 명령(sh -c, 상한 60초) — 표준 출력과 표준 오류를 따로(Cmd.Run은 둘을 합치거나 버린다). 못 띄움은 127, 상한 넘김은 proc.TimeoutCode
func oaLaunch(launch string, env []string) (string, string, int) {
	tickCall("sh", []string{"-c", launch})
	x := exec.Command("sh", "-c", launch)
	x.Env = envWith(env, nil)
	var o, e bytes.Buffer
	x.Stdout, x.Stderr = &o, &e
	err := proc.RunMax(x, 60*time.Second)
	var ee *exec.ExitError
	var te *proc.TimeoutError
	switch {
	case errors.As(err, &te):
		return o.String(), e.String() + "\n(60초 상한을 넘김)", proc.TimeoutCode
	case err != nil && !errors.As(err, &ee):
		return o.String(), e.String() + "\n" + err.Error(), 127
	}
	return o.String(), e.String(), proc.Code(err)
}

// oaLaunchWhy: 못 띄운 까닭 — (개인 값 가림 무늬를 거친 글, launch.err에 둘 원문 전체). 까닭은 표준 오류, 그리고 종료 0이 아니면
// 주소 대신 온 표준 출력 첫 줄. 무늬를 못 읽으면 글은 붙이지 않고 launch.err를 가리킨다(일지·사건에 원문을 내지 않는다)
func oaLaunchWhy(addr string, rc int, out, errOut string) (string, string) {
	var parts []string
	if s := strings.TrimSpace(errOut); s != "" {
		parts = append(parts, s)
	}
	if rc != 0 && addr != "" {
		parts = append(parts, "표준 출력: "+addr)
	}
	why := strings.Join(parts, "\n")
	if why == "" {
		why = "까닭 없음(표준 오류 비었음, 표준 출력 첫 줄 비었음)"
	}
	full := fmt.Sprintf("종료 %d\n--- 표준 출력\n%s\n--- 표준 오류\n%s\n", rc, out, errOut)
	mk, err := toolcfg.Masker(toolcfg.Dir())
	if err != nil {
		return "까닭은 launch.err에(개인 값 무늬를 못 읽어 여기 안 붙임: " + err.Error() + ")", full
	}
	return mk(why), full
}

// oaClip: 앞 n글자(룬)까지, 넘으면 「…」
func oaClip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// oaClose: 끝난 실행이 띄운 터미널을 상태에 적힌 그 주소로 닫는다(이름으로 찾지 않는다 — 다른 기계 주소 「기계/tN」은 hc가 그 기계로).
// 그 주소의 터미널이 끝났거나 없으면 할 일 없음, 이름이 띄울 때 준 이름(HC_FILL_AGENT_NAME)과 다르면 이 실행 것이 아니라 안 닫음.
// 모름·kill 실패는 사건이 아니라 일지 한 줄이고 st.Left에 남겨 다음 띄우기 직전에 한 번 더. 손잡이 ops_agent_close 0이면 안 닫는다
func oaClose(st *oaState, addr, id string) {
	st.Left = ""
	if addr == "" || knobInt("ops_agent_close") != 1 {
		return
	}
	t, found, err := hcGet(addr)
	switch {
	case err != nil:
		st.Left = addr
		jotBy("hc-fill 스스로", "agent 닫기", addr, "끝난 운영 에이전트 실행 "+id, "모름: "+oaClip(err.Error(), 200), "-", "다음 띄우기 직전에 다시")
		return
	case !found || t.State == "finished":
		return
	case t.Name != oaName:
		jotBy("hc-fill 스스로", "agent 닫기", addr, "끝난 운영 에이전트 실행 "+id, "안 닫음: 지금 이름 "+orDash(t.Name)+"(띄울 때 이름 "+oaName+"이 아님)", "-", "-")
		return
	}
	if o, rc := (Cmd{Name: "hc", Args: []string{"kill", addr}, Err: errMerge}).Run(); rc != 0 {
		st.Left = addr
		jotBy("hc-fill 스스로", "agent 닫기", addr, "끝난 운영 에이전트 실행 "+id, fmt.Sprintf("실패: 종료 %d — %s", rc, oaClip(strings.Join(strings.Fields(o), " "), 200)), "-", "다음 띄우기 직전에 다시")
		return
	}
	say("운영 에이전트: 끝난 실행 %s의 터미널 %s 닫음", id, addr)
}

// oaScoped: 판정 못 함을 빼고, 손잡이 ops_agent_scope(프로젝트 목록)에 드는 항목만. 프로젝트를 모르는 항목은 늘 든다
func oaScoped(items []opsstate.Item) []opsstate.Item {
	scope := conf("ops_agent_scope")
	var o []opsstate.Item
	for _, it := range items {
		if it.Kind == "unknown" {
			continue
		}
		if p := oaProject(it); scope == "" || scope == "-" || p == "" || inlist(p, scope) {
			o = append(o, it)
		}
	}
	return o
}

// oaProject: 항목 열쇠 「<종류>:<이름>」의 이름이 대기열 레인이면 그 프로젝트, repo 줄의 프로젝트면 그것, 아니면 모름(빈 값)
func oaProject(it opsstate.Item) string {
	_, n, ok := strings.Cut(it.Key, ":")
	if !ok {
		return ""
	}
	for _, r := range qRows() {
		if r.F(qName) == n {
			return r.F(qProj)
		}
	}
	for _, f := range confRows("repo") {
		if len(f) > 1 && f[1] == n {
			return n
		}
	}
	return ""
}

// oaWrite: 실행 폴더에 목록(items.json)·상태 한 장(state.json)·지시문(brief.md — 틀 + 이번 바퀴 절)
func oaWrite(r oaRun, s opsstate.State, items []opsstate.Item, tmpl, work string) (string, error) {
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		return "", err
	}
	var ib, sb strings.Builder
	if err := opsstate.AdviseJSON(&ib, opsstate.Advice{Schema: opsstate.AdviseSchema, At: time.Unix(r.Started, 0), Fingerprint: s.Fingerprint, Items: oaScoped(items)}); err != nil {
		return "", err
	}
	if err := opsstate.JSON(&sb, s); err != nil {
		return "", err
	}
	if err := writeAtomic(r.Dir+"/items.json", ib.String()); err != nil {
		return "", err
	}
	if err := writeAtomic(r.Dir+"/state.json", sb.String()); err != nil {
		return "", err
	}
	foot := fmt.Sprintf(`

## 이번 바퀴 (hc-fill이 채움)
- 실행 ID: %s · 방식: %s · 상한: %s까지
- 손댈 것 목록(json, hc-fill advise --json 꼴): %s
- 이번에 맡긴 항목(새로 생겼거나 지난번에 못 푼 것): %s
- 상태 한 장(json, hc-fill state --json 꼴): %s
- 결과 파일: %s — 끝낼 때 쓴다(쓰면 끝난 것으로 본다). 첫 줄 = 한 줄 요약. 넘길 것은 줄마다 「사람이 정한다: <무엇·왜·권하는 선택>」, hc-fill 밖 명령을 판단만으로 둔 것은 줄마다 「하려던 것: <명령> — <까닭>」
`, r.ID, r.Mode, time.Unix(r.Deadline, 0).Format("2006-01-02 15:04"), r.Dir+"/items.json", strings.Join(r.Keys, ", "), r.Dir+"/state.json", r.Dir+"/result")
	p := r.Dir + "/brief.md"
	return p, writeAtomic(p, tmpl+foot)
}

// oaFinish: 도는 실행이 끝났나(결과 파일 · 터미널 끝남 · 상한 넘김) — 끝났으면 결과를 일지·사건으로 남기고 실행을 비운다
func oaFinish(st *oaState, now time.Time) {
	r := st.Run
	res, haveRes := readFile(r.Dir + "/result")
	kind := ""
	switch {
	case haveRes:
		kind = "끝남"
	case now.Unix() >= r.Deadline:
		kind = "시간 넘김"
	default:
		t, found, err := hcGet(r.Addr)
		if err != nil {
			return // 모름 — 상한까지 기다린다
		}
		switch {
		case !found || t.State == "finished":
			kind = "터미널 끝남(결과 없음)"
		case (t.Status == "done" || t.Status == "idle") && now.Unix()-r.Started >= oaGrace:
			// 대화형 CLI는 턴을 끝내도 터미널이 남는다 — 자동 감사(run2.go)처럼 done·idle을 끝으로(감사 atozu10 낮음: 상한까지 다음 실행이 막혔다)
			kind = "턴 끝남(결과 없음)"
		}
	}
	if kind == "" {
		return
	}
	summary := kind
	var asks []string
	for _, l := range lines(res) {
		l = strings.TrimSpace(l)
		switch {
		case l == "":
		case strings.HasPrefix(l, "사람이 정한다:"):
			asks = append(asks, l)
		case strings.HasPrefix(l, "하려던 것:"):
			jotBy("운영 에이전트 "+r.ID, "하려던 것", "-", strings.TrimSpace(strings.TrimPrefix(l, "하려던 것:")), "안 함("+r.Mode+")", "-", "결과 파일")
		case summary == kind:
			summary = l
		}
	}
	for _, k := range r.Keys {
		if x := st.Keys[k]; x != nil && haveRes {
			x.Done = true
		}
	}
	for i, a := range asks {
		if i == 5 {
			break // 한 바퀴에 다섯까지 — 나머지는 결과 파일에
		}
		hcLaneReport("막힘", oaFlat(a), "hc-fill-agent", "", "")
	}
	if h := oaHash(); h != r.Hash {
		e := knob.Entry{Name: "ops_agent", Value: "끔", By: "hc-fill 스스로", Why: "운영 에이전트 실행 중 띄울 명령·틀이 바뀜", At: now.Unix()}
		if err := knob.Set(knob.Path(HOME), e, true, now); err != nil {
			say("운영 에이전트: 손잡이를 끄지 못했다: %v", err)
		}
		jotBy("hc-fill 스스로", "knob set", "ops_agent", e.Why, "됨", "hc-fill knob unset ops_agent", "실행 "+r.ID)
		hcLaneReport("막힘", "사람이 정한다: 운영 에이전트 실행("+r.ID+") 중에 띄울 명령("+oaLaunchK+")이나 틀("+oaTmpl+")이 바뀌었다 — ops_agent를 끔으로 둠. 권함: 바뀐 것을 보고 맞으면 hc-fill knob unset ops_agent 뒤 다시 켠다", "hc-fill-agent", "", "")
	}
	msg := oaFlat(fmt.Sprintf("운영 에이전트 끝: %s (%s, %s, 손댈 것 %d개, 일지 hc-fill journal --by %s)", summary, kind, r.Mode, len(r.Keys), oaJournalBy(r)))
	if hcLaneReport("중간", msg, "hc-fill-agent", "", "") != 0 {
		say("운영 에이전트: 끝 사건을 보내지 못했다(hc-lane report)")
	}
	jotBy("hc-fill 스스로", "agent 끝", r.ID, "", kind, "-", summary)
	say("운영 에이전트 끝 %s: %s — %s", r.ID, kind, summary)
	st.Recent = append(st.Recent, oaDone{ID: r.ID, Start: r.Started, End: now.Unix(), Result: kind, Summary: summary, Keys: r.Keys})
	if len(st.Recent) > oaRecent {
		st.Recent = st.Recent[len(st.Recent)-oaRecent:]
	}
	st.Run = nil
	oaClose(st, r.Addr, r.ID) // 대화형 CLI는 턴을 끝내도 터미널이 남아, 같은 이름으로 띄우는 다음 실행이 name_taken으로 실패했다
}

// oaJournalBy: 그 실행의 조치가 일지에 남는 「누가」(에이전트 터미널 주소)
func oaJournalBy(r *oaRun) string {
	if r.Addr != "" {
		return r.Addr
	}
	return "운영 에이전트 " + r.ID
}

// oaFlat: 사건 글은 탭 구분 한 줄 — 탭·줄바꿈을 펴고 500바이트에서 자른다
func oaFlat(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 500 {
		s = strings.ToValidUTF8(s[:500], "") + "…"
	}
	return s
}
