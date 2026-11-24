package main

// 풀이 터미널 감시(2026-11-24 knob-test-leak, 지휘 지시 — 병합 쪽 충돌 풀이의 첫 실사용에서 푸는 에이전트가 폴더 신뢰 질문에서 멈춰
// 있었다. 풀이 작업 트리 <클론>.merger-resolve/<큐 이름>은 제출마다 새 폴더라 매번 뜨는데, 레인은 hc-lane 띄운 직후 확인이 넘기고
// 병합기가 resolve_launch로 띄운 에이전트에는 그 감시도 멈춤 사건도 없었다 — 지휘가 손으로 넘겼다, 못 봤으면 resolve_max 뒤 반송).
// 바퀴마다 푸는 중인 제출의 터미널(r.addr — 풀이 또는 풀이 감사)을 본다:
//   - 신뢰 질문: status가 아직 없거나 blocked이면 화면(hc read)을 보고, internal/trustq가 질문이라 하면 설정 lane.auto_trust(on 기본)일 때
//     그 키(Down·Enter)를 한 번 보낸다(<상태 폴더>/trusted에 그 주소 — 같은 터미널에 두 번 안 보냄). 글자가 다름·커서 없음·설정 모름은 건드리지 않는다.
//   - 멈춤: status blocked·error가 손잡이 resolve_stuck분(기본 3, 0이면 끔) 이어지면 운영 사건 merger 한 번(「병합 쪽 풀이 에이전트 멈춤
//     <주소> — hc read <주소>」, 키 hc-merger-resolve-stuck:<주소>:<처음 본 초>). 풀리면 다시 센다(<상태 폴더>/stuck).
// hc 조회가 실패하면 모름 — 아무것도 하지 않는다(다음 바퀴에).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/trustq"
)

const hcLook = 10 * time.Second

// resolveStuck: 손잡이 resolve_stuck(분, 기본 3, 0이면 끔)
func (m *M) resolveStuck() int {
	return intSetting("resolve_stuck", m.knobStr("resolve_stuck", "HC_LANE_MERGER_RESOLVE_STUCK"), 3, 0)
}

// rwatch: 푸는 중인 터미널 한 바퀴 감시
func (m *M) rwatch(r *rstate) {
	if r.addr == "" || r.addr[0] == '-' {
		return
	}
	for _, p := range strings.FieldsFunc(r.addr, func(c rune) bool { return c == ':' || c == '/' }) { // 주소 조각마다(기계/터미널·w1:t2)
		if !addrOK(p) {
			return // hc에 옵션·이상한 글자로 넘기지 않는다
		}
	}
	o, rc, _ := hcRun(hcLook, "get", r.addr, "--json")
	t, found, err := hcjson.ParseGet([]byte(o), rc, r.addr)
	if err != nil || !found || t.State != "live" {
		return
	}
	if (t.Status == "" || t.Status == "blocked") && strings.TrimSpace(readS(r.dir+"/trusted")) != r.addr {
		m.rtrust(r)
	}
	sf := r.dir + "/stuck"
	if t.Status != "blocked" && t.Status != "error" {
		removeQuiet(sf)
		return
	}
	f := strings.Fields(readS(sf)) // 주소 처음본초 알림(0|1)
	now := time.Now().Unix()
	if len(f) != 3 || f[0] != r.addr {
		f = []string{r.addr, strconv.FormatInt(now, 10), "0"}
		writeVia(sf, strings.Join(f, " ")+"\n") //nolint:errcheck // 못 쓰면 다음 바퀴에 처음부터 센다
		return
	}
	since, _ := strconv.ParseInt(f[1], 10, 64)
	lim := m.resolveStuck()
	if lim == 0 || f[2] == "1" || now-since < int64(lim)*60 {
		return
	}
	msg := fmt.Sprintf("병합 쪽 풀이 에이전트 멈춤 %s — hc read %s (레인 %s, %s %d분째, 작업 트리 %s)", r.addr, r.addr, r.ln, t.Status, (now-since)/60, r.wt)
	args := []string{"report", "막힘", "-", msg, "--name", "merger", "--key", "hc-merger-resolve-stuck:" + r.addr + ":" + f[1]}
	if _, rc, e := boundedRun(reportMax, gSelf, nil, args...); rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 풀이 멈춤 사건을 적지 못했다(종료 %d): %s — 다음 바퀴에\n", rc, strings.TrimSpace(e))
		return
	}
	writeVia(sf, r.addr+" "+f[1]+" 1\n") //nolint:errcheck // 못 쓰면 같은 키로 다시 — report가 한 번만 적는다
	fmt.Fprintf(os.Stderr, "hc-lane merger: %s\n", msg)
}

// rtrust: 화면이 신뢰 질문이면(설정 lane.auto_trust on일 때) 키를 한 번 보낸다
func (m *M) rtrust(r *rstate) {
	scr, rc, _ := hcRun(hcLook, "read", r.addr)
	if rc != 0 {
		return
	}
	v, keys := trustq.Look(scr)
	switch v {
	case trustq.None:
		return
	case trustq.Keys:
	default:
		fmt.Fprintf(os.Stderr, "hc-lane merger: 모름 — 풀이 터미널 %s 신뢰 질문 글자가 다르거나 커서가 안 보여 건드리지 않았다(hc read %s)\n", r.addr, r.addr)
		writeVia(r.dir+"/trusted", r.addr+"\n") //nolint:errcheck // 한 번 알리면 됨 — 멈추면 멈춤 사건이 간다
		return
	}
	at, ok := autoTrust()
	if !ok {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 설정 lane.auto_trust를 읽지 못해 풀이 터미널 %s 신뢰 질문을 건드리지 않았다 — 다음 바퀴에\n", r.addr)
		return
	}
	if at == "off" {
		writeVia(r.dir+"/trusted", r.addr+"\n") //nolint:errcheck // 꺼짐 — 멈추면 멈춤 사건
		return
	}
	if _, rc, e := hcRun(hcLook, append([]string{"keys", r.addr}, keys...)...); rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 풀이 터미널 %s 신뢰 질문 키를 보내지 못했다(종료 %d): %s\n", r.addr, rc, strings.TrimSpace(e))
		return
	}
	writeVia(r.dir+"/trusted", r.addr+"\n") //nolint:errcheck // 못 쓰면 다음 바퀴에 화면을 다시 본다(질문이 없으면 안 보냄)
	fmt.Printf("hc-lane merger: 풀이 터미널 신뢰 질문 넘김(%s)\n", r.addr)
}

// autoTrust: 설정 lane.auto_trust(on·off, 기본 on) — hc-lane과 같은 길(곁의 herdr-tools-config, 없으면 PATH, 그것도 없으면 기본)
func autoTrust() (string, bool) {
	c := filepath.Join(filepath.Dir(gSelf), "herdr-tools-config")
	if st, err := os.Stat(c); err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
		p, err := exec.LookPath("herdr-tools-config")
		if err != nil {
			return "on", true
		}
		c = p
	}
	o, rc, _ := boundedRun(hcLook, c, nil, "get", "lane.auto_trust", "on")
	if v := strings.TrimSpace(o); rc == 0 && (v == "on" || v == "off") {
		return v, true
	}
	return "", false
}

func readS(p string) string { b, _ := os.ReadFile(p); return string(b) }
