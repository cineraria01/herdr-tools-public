// Tailscale 지킴이(2026-11-22 go-tailscale-heal): 이 기계의 Tailscale이 꺼져 있으면(「Tailscale is stopped.」) 다시 켠다 —
// ① 플래그 없이 up → ② 「non-default flags」 오류면 그 출력이 알려 준 「tailscale up …」 줄의 플래그 그대로 up(지어내지 않는다,
// 줄 꼴이 다르면 하지 않고 알림만). 한 일은 사건 hc-tailscale 한 줄(기계 이름 포함). 앱·연결부 판이 다르면(client version !=
// tailscaled server version) 그 판 짝마다 알림 한 번. 11/21 mini-m4가 앱 자동 업데이트 뒤 두 번 꺼져 손으로 up 했다.
// 설정 sweep.tailscale_up_per_hour(없으면 3, 0이면 다시 켜지 않고 알림만). CLI가 없는 기계는 아무것도 안 한다.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// tsCands: PATH에 없을 때 볼 자리(앱 묶음·Homebrew·tailscaled 깔린 곳)
var tsCands = []string{"/Applications/Tailscale.app/Contents/MacOS/Tailscale", "/opt/homebrew/bin/tailscale", "/usr/local/bin/tailscale"}

// tsFind: CLI 경로. HC_SWEEP_TAILSCALE가 off면 점검 안 함, 다른 값이면 그 경로(시험·별난 자리)
func tsFind() string {
	switch v := os.Getenv("HC_SWEEP_TAILSCALE"); v {
	case "off":
		return ""
	case "":
	default:
		return v
	}
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p
	}
	for _, p := range tsCands {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

var (
	tsVerRe  = regexp.MustCompile(`client version "([^"]+)" != tailscaled server version "([^"]+)"`)
	tsFlagRe = regexp.MustCompile(`^--[a-z][a-z0-9-]*(=[A-Za-z0-9._:/,@+-]*)?$`)
)

// tsUpFlags: non-default 오류 출력이 알려 준 줄의 플래그. 오류 꼴이 아니거나, 그 줄이 하나가 아니거나, 인용·빈칸 섞인 값처럼
// 그대로 옮길 수 없는 낱말이 있으면 ok=false(손대지 않음)
func tsUpFlags(out string) ([]string, bool) {
	if !strings.Contains(out, "non-default flags") {
		return nil, false
	}
	var line string
	n := 0
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, "tailscale up ") {
			line, n = l, n+1
		}
	}
	if n != 1 {
		return nil, false
	}
	flags := strings.Fields(strings.TrimPrefix(line, "tailscale up "))
	for _, f := range flags {
		if !tsFlagRe.MatchString(f) || f == "--reset" || f == "--force-reauth" {
			return nil, false
		}
	}
	return flags, len(flags) > 0
}

// tsBrief: 알림에 넣을 첫 줄. 로그인 주소는 옮기지 않는다
func tsBrief(r result) string {
	s := strings.TrimSpace(r.out + "\n" + r.errs)
	if strings.Contains(s, "login.tailscale.com") || strings.Contains(s, "To authenticate") {
		return "로그인이 필요하다(사람이 앱에서)"
	}
	if r.err != nil {
		return r.err.Error()
	}
	first, _, _ := strings.Cut(s, "\n")
	return cutRunes(dash(first), 200)
}

// tsTake: 한 시간 한도 안이면 이번 시도를 적고 true. 기록은 시각(유닉스 초) 한 줄씩. 못 읽거나 못 쓰면 false(모름이면 안 함)
func tsTake(state string, perHour int, now time.Time) bool {
	b, err := os.ReadFile(state)
	if err != nil && !os.IsNotExist(err) {
		return false
	}
	var keep []string
	for _, l := range strings.Fields(string(b)) {
		if t, err := strconv.ParseInt(l, 10, 64); err == nil && now.Unix()-t < 3600 {
			keep = append(keep, l)
		}
	}
	if len(keep) >= perHour {
		return false
	}
	keep = append(keep, strconv.FormatInt(now.Unix(), 10))
	if os.MkdirAll(filepath.Dir(state), 0o755) != nil {
		return false
	}
	return os.WriteFile(state, []byte(strings.Join(keep, "\n")+"\n"), 0o644) == nil
}

// tailscaleCheck: 한 바퀴. report(msg, key)는 사건 한 줄(key 빈 값이면 키 없이). 찍을 줄들을 돌려준다
func tailscaleCheck(ts, machine string, perHour int, state string, dry bool, r runner, report func(msg, key string) bool) []string {
	var lines []string
	say := func(msg, key string) {
		if dry {
			lines = append(lines, "Tailscale (보낼 것): "+msg)
			return
		}
		if !report(msg, key) {
			msg += " — 알림 실패(hc-lane report)"
		}
		lines = append(lines, "Tailscale: "+msg)
	}
	st := r(30*time.Second, nil, ts, "status")
	all := st.out + "\n" + st.errs
	if m := tsVerRe.FindStringSubmatch(all); m != nil {
		say(fmt.Sprintf("Tailscale 앱과 연결부 판이 다름(%s): client %s != tailscaled %s — 사람이 앱을 열어야 할 수 있음", machine, m[1], m[2]),
			"hc-tailscale:ver:"+machine+":"+m[1]+":"+m[2])
	}
	if !strings.Contains(all, "Tailscale is stopped.") {
		return lines
	}
	head := "Tailscale 꺼짐(" + machine + ")"
	if perHour <= 0 {
		say(head+" — 다시 켜기 꺼짐(sweep.tailscale_up_per_hour 0), 알림만", "hc-tailscale:off:"+machine+":"+time.Now().Format("2006-01-02T15"))
		return lines
	}
	if dry {
		lines = append(lines, "Tailscale (할 것): "+head+" — tailscale up")
		return lines
	}
	if !tsTake(state, perHour, time.Now()) {
		say(fmt.Sprintf("%s — 한 시간 다시 켜기 한도 %d번을 넘었거나 기록을 못 읽어 알림만", head, perHour),
			"hc-tailscale:limit:"+machine+":"+time.Now().Format("2006-01-02T15"))
		return lines
	}
	up := r(60*time.Second, nil, ts, "up")
	if up.err == nil && up.rc == 0 {
		say(head+" — tailscale up으로 다시 켬", "")
		return lines
	}
	flags, ok := tsUpFlags(up.out + "\n" + up.errs)
	if !ok {
		say(head+" — tailscale up 실패, 알려 준 줄 꼴이 아니라 더 손대지 않음: "+tsBrief(up), "")
		return lines
	}
	up2 := r(60*time.Second, nil, ts, append([]string{"up"}, flags...)...)
	if up2.err == nil && up2.rc == 0 {
		say(head+" — 알려 준 플래그 그대로 tailscale up "+strings.Join(flags, " ")+"으로 다시 켬", "")
	} else {
		say(head+" — tailscale up "+strings.Join(flags, " ")+" 실패: "+tsBrief(up2), "")
	}
	return lines
}

// tailscaleRun: 설정을 읽고 한 바퀴
func tailscaleRun(dry bool, r runner) {
	ts := tsFind()
	if ts == "" {
		return
	}
	v, ok := conf("sweep.tailscale_up_per_hour", "3")
	if !ok {
		fmt.Println("Tailscale 점검: 설정을 못 읽어 건너뜀")
		return
	}
	perHour, err := strconv.Atoi(v)
	if err != nil {
		fmt.Printf("Tailscale 점검: sweep.tailscale_up_per_hour 값이 정수가 아님(%s) — 건너뜀\n", v)
		return
	}
	machine := os.Getenv("HC_LANE_MACHINE")
	if machine == "" {
		machine, _ = os.Hostname()
		machine, _, _ = strings.Cut(machine, ".")
	}
	state := filepath.Join(stateHome(), "hc-sweep", "tailscale-up")
	lane := filepath.Join(binDir, "hc-lane")
	report := func(msg, key string) bool { return laneReport(r, lane, "hc-tailscale", msg, key) }
	for _, l := range tailscaleCheck(ts, machine, perHour, state, dry, r, report) {
		fmt.Println(l)
	}
}
