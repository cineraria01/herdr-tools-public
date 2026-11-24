package main

// 띄운 직후 확인(셸 판 lib/hc-lane/start.sh의 start_check·imports_q·turn_check·lane_event와 같은 판정·같은 글).
// 화면 글자는 Claude 2.1 실측(신뢰 질문 「❯ No, exit」·「  Yes, I trust this folder」, 외부 가져오기 질문 「External imports:」).

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"herdr-tools/internal/proc"
)

var (
	trustQ     = regexp.MustCompile(`(?m)^ *(❯ | +)No, exit *$`)
	trustY     = regexp.MustCompile(`(?m)^ *(❯ | +)Yes, I trust this folder *$`)
	trustCurN  = regexp.MustCompile(`(?m)^ *❯ No, exit *$`)
	trustCurY  = regexp.MustCompile(`(?m)^ *❯ Yes, I trust this folder *$`)
	impCurY    = regexp.MustCompile(`^ *❯ ([0-9]+\. )?Yes, allow external imports *$`)
	impCurN    = regexp.MustCompile(`^ *❯ ([0-9]+\. )?No, disable external imports *$`)
	impY       = regexp.MustCompile(`^ *( +)([0-9]+\. )?Yes, allow external imports *$`)
	impEnd     = regexp.MustCompile(`^[[:space:]]*$|Important:|(Yes, allow|No, disable) external imports`)
	keychainRe = regexp.MustCompile(`(?i)keychain|errSec`)
	loginRe    = regexp.MustCompile(`(?i)login expired|/login|not logged in|log ?in again|invalid api key|oauth token|authentication_error|unauthorized|(^|[^0-9])401([^0-9]|$)`)
)

// startUnsure: 시작 확인 미확정 — 마감까지 hc get으로 시작도 확인된 실패도 못 정했다(조회 실패·잘린 응답·status 칸 없음·주소·이름 다름·
// not_found). 시작했을 수 있다 — hc-fill은 다른 기계에 또 띄우지 않고 그 시도·주소를 남긴다(재감사2 lane/R-211, fill/R-201)
const startUnsure = 8

// spoll: startPoll 판정 — 0값이 모름
type spoll int

const (
	spUnknown  spoll = iota // 모름(조회 실패·not_found·다른 터미널·status 칸 없음)
	spStatus                // status 잡힘
	spNoStatus              // 확인된 안 잡힘(살아 있고 status null)
	spFinished              // 확인된 끝남
)

// startPoll: 시작 확인 hc get 한 번. status가 잡히면 g는 그 응답 JSON
func startPoll(a string) (spoll, string) {
	t, found, err := hcGet(10, a)
	if err != nil || !found {
		return spUnknown, ""
	}
	if t.HasName && t.Name != name {
		return spUnknown, "" // 그 주소가 다른 터미널이다
	}
	if t.State == "finished" {
		return spFinished, ""
	}
	if _, ok := t.Raw["status"]; !ok {
		return spUnknown, "" // 칸 자체가 없는 부분 응답 — null(아직 안 잡힘)과 다르다
	}
	if t.Status == "" {
		return spNoStatus, ""
	}
	g, err := json.Marshal(t.Raw)
	if err != nil {
		return spUnknown, ""
	}
	return spStatus, string(g)
}

// hcRead: 실패한(시간 넘김 포함) hc read의 반쪽 화면은 빈 화면으로 — 그것으로 신뢰·가져오기 키를 보내지 않는다(gofix E07)
func hcRead(a string) string {
	out, r := outOf(nil, []string{"hc", "read", a})
	if r != 0 {
		return ""
	}
	return out
}

var (
	reHitURL = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://\S+`)
	reHitKV  = regexp.MustCompile(`(?i)\b(token|key|secret|password|passwd|code|sig|signature|auth|authorization|bearer)\b(\s*[=:]\s*|\s+)\S+`)
	reHitLng = regexp.MustCompile(`[A-Za-z0-9_+/=.-]{24,}`)
)

// redactHit: 막힘 줄(화면 원문)에서 비밀일 수 있는 값을 지운다 — 주소·「키=값」·긴 토큰 꼴(이 줄은 hc-fill 바퀴 기록에도 남는다, gofix E08)
func redactHit(s string) string {
	s = reHitURL.ReplaceAllString(s, "<주소>")
	s = reHitKV.ReplaceAllString(s, "$1=<가림>")
	return reHitLng.ReplaceAllString(s, "<가림>")
}

func hcKeys(args ...string) bool {
	return proc.Run(exec.Command("hc", append([]string{"keys"}, args...)...)) == nil
}

func anyLine(re *regexp.Regexp, s string) bool {
	for _, ln := range strings.Split(s, "\n") {
		if re.MatchString(ln) {
			return true
		}
	}
	return false
}

// startCheck: Claude 폴더 신뢰 질문을 넘기고 hc get status가 잡히는지 본다. start=ok|no_status|unknown|login|keychain, 종료 0·5·6·7·8.
// 응답은 hcjson.ParseGet으로 통째 검증(주소·live·status) — 여는 괄호만 보고 성공으로 읽지 않는다. 마감까지 한 번이라도 모름이었고
// 확인된 끝남이 아니면 미확정(8). 실패여도 터미널은 남긴다
func startCheck(a string) int {
	sw, _ := strconv.ParseInt(swait, 10, 64)
	end := now() + sw
	tr, im, ik, itopSet = "", "", 0, false
	why, last, unk := "", false, false
	for {
		pr, g := startPoll(a)
		if pr == spUnknown {
			unk = true
		}
		if pr == spStatus {
			start = "ok"
			if _, err := exec.LookPath("jq"); fwait == 0 || err != nil {
				return 0
			}
			if os.Getenv("HC_LANE_FILL") == "1" {
				// hc-fill은 두 출력을 함께 받는다 — 감시가 그 파이프를 쥐면 다음 기계 배치 판단이 늦어진다
				turnCheckBg(a, g)
			} else {
				turnCheck(a, g)
			}
			return 0
		}
		if pr == spFinished {
			why, unk = "터미널이 끝났다", false
			break
		}
		scr := hcRead(a)
		if strings.Contains(scr, "Allow external CLAUDE.md file imports?") && ik < 3 && !importsQ(a, scr) {
			why = "외부 가져오기 질문에 작업 폴더 저장소 밖 경로가 있다"
			break
		}
		if tr == "" && (strings.Contains(scr, "Accessing workspace") || strings.Contains(scr, "trust this folder") ||
			strings.Contains(scr, "Do you trust") || strings.Contains(scr, "one you trust?")) {
			if strust == "off" {
				tr = "off"
			} else if strings.Contains(scr, "Is this a project you created or one you trust?") && trustQ.MatchString(scr) && trustY.MatchString(scr) {
				var keys []string
				if trustCurN.MatchString(scr) {
					keys = []string{"down", "enter"}
				} else if trustCurY.MatchString(scr) {
					keys = []string{"enter"}
				}
				if keys == nil {
					tr = "unknown"
					errf("hc-lane: 모름 — 시작 화면 확인(%s): 신뢰 질문에 커서(❯)가 안 보여 건드리지 않았다\n", a)
				} else if hcKeys(append([]string{a}, keys...)...) {
					tr = "passed"
					fmt.Printf("hc-lane: 신뢰 질문 넘김(%s)\n", a)
				} else {
					tr = "unknown"
					errf("hc-lane: 모름 — 시작 화면 확인(%s): 신뢰 질문 키를 보내지 못했다\n", a)
				}
			} else {
				tr = "unknown"
				errf("hc-lane: 모름 — 시작 화면 확인(%s): 신뢰 질문 같은데 글자가 달라 건드리지 않았다(hc read %s)\n", a, a)
			}
		}
		// 마감이 지나도 한 번은 더 본다: 이 바퀴에 키를 보냈으면 status는 다음 hc get에야 잡힌다
		if now() >= end {
			if last {
				break
			}
			last = true
		}
		sleepPoll()
	}
	if why == "" {
		why = swait + "초 안에 hc get status가 안 잡혔다"
		if unk {
			why += "(hc get 응답을 확인 못 한 바퀴가 있다 — 시작했을 수 있어 미확정)"
		}
	}
	switch tr {
	case "unknown":
		why += ", 신뢰 질문 모름"
	case "off":
		why += ", 신뢰 질문(lane.auto_trust off)"
	}
	switch im {
	case "unknown":
		why += ", 외부 가져오기 질문 모름"
	case "off":
		why += ", 외부 가져오기 질문(lane.auto_trust off)"
	}
	var ls []string
	for _, ln := range strings.Split(hcRead(a), "\n") {
		if strings.TrimSpace(ln) != "" {
			ls = append(ls, ln)
		}
	}
	if len(ls) > 15 {
		ls = ls[len(ls)-15:]
	}
	r, k, hit := 5, "", ""
	start = "no_status"
	if unk {
		r, start = startUnsure, "unknown"
	}
	for _, ln := range ls {
		if keychainRe.MatchString(ln) {
			start, r, k, hit = "keychain", 7, "키체인", keychainRe.FindString(ln)
			break
		}
	}
	if k == "" {
		for _, ln := range ls {
			if loginRe.MatchString(ln) {
				start, r, k, hit = "login", 6, "로그인·인증", loginRe.FindString(ln)
				break
			}
		}
	}
	// 막힘 줄에는 화면 원문 대신 판정 무늬에 맞은 고정 구절만 — 자유 형식 인증 화면(짧은 비밀값 포함)을 사건·바퀴 기록에 넣지 않는다(R-21)
	hit = redactHit(strings.TrimSpace(hit))
	if rs := []rune(hit); len(rs) > 160 {
		hit = string(rs[:160])
	}
	blk := ""
	if k != "" {
		blk = " — 막힘(" + k + "): " + hit
	}
	errf("hc-lane: 시작 안 됨 %s: %s%s. 터미널은 남겼다(hc read %s). 다른 기계로 옮기려면 이 레인 작업 폴더를 정리하고 다시 띄운다\n", a, why, blk, a)
	return r
}

// importsQ: 「Allow external CLAUDE.md file imports?」 — 목록 경로가 모두 작업 폴더의 git 저장소 안일 때만 허용을 고른다.
// 밖·읽을 수 없는 줄이 하나라도 있으면 false(호출자가 종료 5). im=passed|off|unknown
func importsQ(a, scr string) bool {
	if strust != "on" {
		im = "off"
		return true
	}
	if im == "unknown" {
		return true
	}
	if !itopSet {
		itopSet = true
		itop, _ = xoutQ("git", "-C", cwd, "rev-parse", "--show-toplevel")
		if on != "" {
			ihome, _ = xout("sh", "-c", `printf %s "$HOME"`)
		} else {
			ihome = os.Getenv("HOME")
		}
	}
	var ps []string
	f := false
	for _, ln := range strings.Split(scr, "\n") {
		if !f {
			if strings.Contains(ln, "External imports:") {
				f = true
			}
			continue
		}
		if impEnd.MatchString(ln) {
			break
		}
		ps = append(ps, strings.TrimSpace(ln))
	}
	bad := ""
	add := func(s string) {
		if bad != "" {
			bad += ", "
		}
		bad += s
	}
	if len(ps) == 0 {
		bad = "(목록을 읽지 못함)"
	}
	for _, p := range ps {
		if p == "" {
			continue
		}
		if p == "~" {
			p = ihome
		} else if strings.HasPrefix(p, "~/") {
			p = ihome + "/" + p[2:]
		}
		ok := false
		switch {
		case strings.Contains(p, "…"), strings.Contains(p, "/../"), strings.HasSuffix(p, "/.."), strings.Contains(p, "/./"):
		case p == itop, strings.HasPrefix(p, itop+"/"):
			ok = true
		}
		if itop == "" || !ok {
			add(p)
		}
	}
	if bad != "" {
		t := itop
		if t == "" {
			t = "저장소 아님"
		}
		errf("hc-lane: 모름 — 외부 가져오기 질문(%s): 작업 폴더 저장소(%s) 밖이라 고르지 않았다(%s)\n", bad, t, a)
		return false
	}
	lines := strings.Split(scr, "\n")
	idx := func(re *regexp.Regexp) int {
		for i, ln := range lines {
			if re.MatchString(ln) {
				return i + 1
			}
		}
		return 0
	}
	var keys []string
	if idx(impCurY) > 0 {
		keys = []string{"enter"}
	} else if nl := idx(impCurN); nl > 0 {
		if yl := idx(impY); yl == 0 {
		} else if yl > nl {
			keys = []string{"down", "enter"}
		} else {
			keys = []string{"up", "enter"}
		}
	}
	if keys == nil {
		im = "unknown"
		errf("hc-lane: 모름 — 외부 가져오기 질문(%s): 고르기 줄·커서(❯)가 안 보여 건드리지 않았다\n", a)
	} else if hcKeys(append([]string{a}, keys...)...) {
		im = "passed"
		ik++
		fmt.Printf("hc-lane: 외부 가져오기 허용(%s, 저장소 %s 안)\n", a, itop)
	} else {
		im = "unknown"
		errf("hc-lane: 모름 — 외부 가져오기 질문(%s): 키를 보내지 못했다\n", a)
	}
	return true
}

// firstTurn: turn > 0 또는 status working
func firstTurn(g string) bool {
	var v struct {
		Turn   any `json:"turn"`
		Status any `json:"status"`
	}
	if json.Unmarshal([]byte(g), &v) != nil {
		return false
	}
	if n, ok := v.Turn.(float64); ok && n > 0 {
		return true
	}
	if m, ok := v.Status.(map[string]any); ok && m["state"] == "working" {
		return true
	}
	return false
}

// turnCheck: 첫 턴 감시(알림 전용). lane.first_turn_wait초 안에 turn > 0·working을 한 번도 못 보면 사건 막힘 「첫 턴 없음 <주소>」.
// 키는 보내지 않고 화면도 읽지 않는다. 늘 start=ok|noturn
func turnCheck(a, g string) {
	capAt := now() + fwait
	for {
		if firstTurn(g) {
			start = "ok"
			return
		}
		left := capAt - now()
		if left <= 0 {
			break
		}
		// 쉼은 poll과 남은 초 가운데 짧은 쪽(셸 판 awk와 같게)
		d := poll
		if pf, err := strconv.ParseFloat(poll, 64); err != nil || pf >= float64(left) {
			d = strconv.FormatInt(left, 10)
		}
		sleepFor(d)
		left = capAt - now()
		if left <= 0 {
			break
		}
		var r int
		if g, r = captP(left, "hc", "get", a, "--json"); r != 0 {
			g = ""
		}
		if now() > capAt {
			break
		}
	}
	start = "noturn"
	at := ""
	if on != "" {
		at = " @" + on
	}
	evErr := ""
	if !laneEvent("막힘", "첫 턴 없음 "+a+at) {
		evErr = " 막힘 사건을 적지 못했다."
	}
	errf("hc-lane: 첫 턴 없음 %s%s: %d초 안에 turn > 0·working이 없었다.%s 키는 보내지 않았다(화면은 hc read %s)\n", a, at, fwait, evErr, a)
}

// turnCheckBg: 같은 바이너리를 배경으로(출력은 …/hc-lane/watch.log, 못 열면 버림) — 띄우기는 기다리지 않는다
func turnCheckBg(a, g string) {
	self, err := os.Executable()
	if err != nil {
		return
	}
	c := exec.Command(self, "__turn-check")
	c.Env = append(os.Environ(), "HC_LANE_TC_ADDR="+a, "HC_LANE_TC_G="+g, "HC_LANE_TC_FWAIT="+strconv.FormatInt(fwait, 10),
		"HC_LANE_TC_POLL="+poll, "HC_LANE_TC_ON="+on, "HC_LANE_TC_NAME="+name, "HC_LANE_TC_LO="+loOwner)
	if os.MkdirAll(evdir(), 0o777) == nil {
		if f, err := os.OpenFile(evdir()+"/watch.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666); err == nil {
			defer f.Close()
			c.Stdout, c.Stderr = f, f
		}
	}
	if c.Start() == nil {
		go c.Process.Release()
	}
}

func turnCheckChild() int {
	g := os.Getenv("HC_LANE_TC_G")
	fwait, _ = strconv.ParseInt(os.Getenv("HC_LANE_TC_FWAIT"), 10, 64)
	poll, on, name, loOwner = os.Getenv("HC_LANE_TC_POLL"), os.Getenv("HC_LANE_TC_ON"), os.Getenv("HC_LANE_TC_NAME"), os.Getenv("HC_LANE_TC_LO")
	turnCheck(os.Getenv("HC_LANE_TC_ADDR"), g)
	return 0
}

// laneEvent: 띄운 레인 이름으로 이 기계 사건 기록에 한 줄(터미널 칸 -). 주인 지휘가 있으면 --to로 그 지휘 watch만
func laneEvent(state, msg string) bool {
	args := []string{"-u", "HC_TERMINAL_ID", "-u", "HC_LANE", selfPath, "report", state, "-", msg, "--name", name}
	if loOwner != "" && loOwner != "-" {
		args = append(args, "--to", loOwner)
	}
	return exec.Command("env", args...).Run() == nil
}
