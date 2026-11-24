package main

// hc 부르기·읽기: hc ls·machine ls JSON(셸 판 jq 식과 같은 뜻), 원격 sh(onm), 실행 키(pget), 터미널 표 조회

import (
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/proc"
)

// decodeAll: 글 안의 JSON 값들(jq 입력 흐름). 못 읽는 곳이 있으면 ok=false
func decodeAll(s string) ([]any, bool) {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var vs []any
	for {
		var v any
		if err := d.Decode(&v); err == io.EOF {
			return vs, true
		} else if err != nil {
			return vs, false
		}
		vs = append(vs, v)
	}
}

// jget: v의 키 경로(없거나 객체가 아니면 nil)
func jget(v any, keys ...string) any {
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

// jstr: jq 「tostring」 — 문자열은 그대로, 그 밖은 압축 JSON. nil은 ""
func jstr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	var c bytes.Buffer
	if json.Compact(&c, b) != nil {
		return string(b)
	}
	return c.String()
}

// jor: jq 「x // d」 — null·false면 d
func jor(v any, d string) string {
	if v == nil || v == false {
		return d
	}
	return jstr(v)
}

// tsvEsc: jq @tsv 칸 이스케이프
func tsvEsc(s string) string {
	return strings.NewReplacer("\\", "\\\\", "\t", "\\t", "\n", "\\n", "\r", "\\r").Replace(s)
}

// parseTerms: hc ls --json → 터미널 표(hcjson.ParseList — 주소 없는 항목이 있으면 목록 전체를 못 읽음, 주소만 읽히는 항목은 모름으로 남김).
// 없음·null 칸은 「-」(agent 「-」는 유형 모름 — countSeats가 그 기계에 새 일을 놓지 않는다). ok=false면 못 읽음(모름)
func parseTerms(raw string) ([]Term, bool) {
	l, err := hcjson.ParseList([]byte(raw))
	if err != nil {
		return nil, false
	}
	ts := make([]Term, 0, len(l.Terminals))
	for _, t := range l.Terminals {
		m := LOCAL
		if i := strings.IndexByte(t.Address, '/'); i >= 0 {
			m = t.Address[:i]
		}
		// 모름 항목(R-3M01)은 칸이 비어 「-」 — agent 「-」라 countSeats가 그 기계에 새 일을 놓지 않고, 주소는 남아 「없음」으로 읽히지 않는다
		ts = append(ts, Term{tsvEsc(m), tsvEsc(t.Address), tsvEsc(orDash(t.Agent)), tsvEsc(orDash(t.Name)), tsvEsc(orDash(t.Status)), tsvEsc(orDash(t.Cwd))})
	}
	return ts, true
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// connectedOf: hc machine ls --json 출력에서 연결된 기계(hcjson.ParseMachines). ok=false면 못 읽음
func connectedOf(raw string) ([]string, bool) {
	ms, err := hcjson.ParseMachines([]byte(raw))
	if err != nil {
		return nil, false
	}
	var c []string
	for _, m := range ms {
		if m.State == "connected" {
			c = append(c, m.Name)
		}
	}
	return c, true
}

// hcGet: hc get <주소> --json 한 번(proc 상한)을 hcjson.ParseGet으로 — 표준 출력은 터미널 객체, 종료 3의 오류 객체는 표준 오류에 온다.
// 오류는 모름(연결 실패·시간 넘김·잘림·찌꺼기·모르는 state·주소 다름)
func hcGet(addr string) (hcjson.Terminal, bool, error) {
	tickCall("hc", []string{"get"})
	x := exec.Command("hc", "get", addr, "--json")
	var o, e bytes.Buffer
	x.Stdout, x.Stderr = &o, &e
	rc := proc.Code(proc.RunMax(x, proc.Limit(x.Args)))
	b := o.Bytes()
	if rc != 0 && len(bytes.TrimSpace(e.Bytes())) > 0 { // 오류 객체는 표준 오류에(비었으면 표준 출력을 그대로 판정)
		b = e.Bytes()
	}
	return hcjson.ParseGet(b, rc, addr)
}

// jsonList: raw의 .<key> 배열에서 sel을 통과한 것의 field를 줄로(jq -er 'map(select(…) | .<field>) | join("\n")'). ok=false면 배열 아님
func jsonList(raw, key, field string, sel func(any) bool) ([]string, bool) {
	vs, ok := decodeAll(raw)
	if !ok || len(vs) == 0 {
		return nil, false
	}
	var out []string
	good := false
	for _, v := range vs {
		arr, isArr := jget(v, key).([]any)
		if !isArr {
			continue
		}
		good = true
		for _, x := range arr {
			if sel == nil || sel(x) {
				out = append(out, jstr(jget(x, field)))
			}
		}
	}
	// join("\n") 뒤 줄로 읽은 것과 같게(빈 값 하나면 줄 없음)
	return lines(strings.Join(out, "\n")), good
}

func isConnected(x any) bool { return jstr(jget(x, "state")) == "connected" }

// connected: 연결된 기계(못 읽으면 빈 목록 — 「연결 안 됨」 쪽으로만 읽힌다)
func connected() []string {
	raw, rc := run("hc", "machine", "ls", "--json")
	if rc != 0 {
		return nil
	}
	l, _ := connectedOf(raw)
	return l
}

// connHas: CONN에 그 기계
func connHas(m string) bool { return has(CONN, m) }

// upOK: local이거나 ICON에 있음
func upOK(m string) bool { return m == LOCAL || has(ICON, m) }

// onm: 그 기계에서(local이면 바로) sh로 X+body를 args로. in이 nil이면 표준 입력은 /dev/null(셸 판 -i 없음). 표준 오류는 물려줌
func onm(in io.Reader, m, body string, args ...string) (string, int) {
	a := append([]string{"-c", X + body, "_"}, args...)
	if m == LOCAL {
		// 로컬도 원격(hc exec --on … sh)과 같은 상한 — proc.Limit이 sh를 몰라 받은편지 해시 등이 멈추면 run.lock을 쥔 채 기다렸다(재감사 fill R-13).
		// 넘기면 124(모름) — 부르는 쪽은 「다음 바퀴에」로 읽는다
		return Cmd{Name: "sh", Args: a, In: in, Err: errPass, Max: proc.Limit([]string{"hc", "exec", "--on", m, "--", "sh"})}.Run()
	}
	h := []string{"exec"}
	if in != nil {
		h = append(h, "-i")
	}
	h = append(h, "--on", m, "--", "sh")
	return Cmd{Name: "hc", Args: append(h, a...), In: in, Err: errPass}.Run()
}

// pget <주소>: PRUN(<기계 접두 없는 터미널>@<created_ms>)·PNAME. 0 살아 있음, 3 끝남 확정(완전한 not_found·finished), 1 모름
// (hcGet 오류, live인데 created_ms·name 칸이 없음 — 실행 신원을 모르면 주소 재사용을 가를 수 없다)
func pget(addr string) int {
	PRUN, PNAME = "", ""
	t, found, err := hcGet(addr)
	switch {
	case err != nil:
		return 1
	case !found || t.State == "finished":
		return 3
	}
	if _, ok := t.Raw["name"]; !ok || t.Created <= 0 {
		return 1
	}
	PRUN = addr[strings.LastIndexByte(addr, '/')+1:] + "@" + strconv.FormatInt(t.Created, 10)
	PNAME = orDash(t.Name)
	return 0
}

// startState <주소> <레인>: 띄운 터미널의 hc get status가 WAIT초 안에 잡히는지. 0 잡힘(START_RUN), 1 안 잡힘 확정(그 레인 터미널이
// 살아 있고 status가 null·끝남), 2 모름(조회 실패·잘린 응답·status 칸 없음·다른 이름이나 주소·시작 시각 없음·not_found — 시작했을 수 있다,
// 재감사 R-01·R-05, 재감사2 fill/R-201). 마감까지 한 번이라도 모름이었고 끝남이 확인되지 않았으면 2
func startState(addr, lane string) int {
	end := time.Now().Unix() + int64(WAIT)
	unk := false
	for {
		r := startOnce(addr, lane)
		switch {
		case r == 0 || r == 3:
			return min(r, 1)
		case r == 2:
			unk = true
		}
		if time.Now().Unix() >= end {
			if unk {
				return 2
			}
			return 1
		}
		time.Sleep(time.Duration(POLL) * time.Second)
	}
}

// startOnce: startState 한 번(hcGet — hcjson.ParseGet). 0 잡힘, 1 살아 있고 status null, 3 끝남 확인, 2 모름
func startOnce(addr, lane string) int {
	t, found, err := hcGet(addr)
	if err != nil || !found || !t.HasName || t.Name != lane {
		return 2 // 모름·확인 직후 사라짐(두 조회가 어긋남)·그 주소가 다른 터미널
	}
	if t.State == "finished" {
		return 3
	}
	if _, ok := t.Raw["status"]; !ok || t.Created <= 0 {
		return 2 // status 칸 자체가 없거나 실행 신원이 없는 부분 응답 — null(아직 안 잡힘)과 다르다
	}
	if t.Status == "" {
		return 1
	}
	START_RUN = addr[strings.LastIndexByte(addr, '/')+1:] + "@" + strconv.FormatInt(t.Created, 10)
	return 0
}

// alive <주소>: 이번 바퀴 터미널 표의 상태, 없으면 gone
func alive(addr string) string {
	for _, t := range TERMS {
		if t.Addr == addr {
			return t.Status
		}
	}
	return "gone"
}

// lalive <주소> <이름>: 그 주소에 그 이름 터미널
func lalive(addr, name string) bool {
	for _, t := range TERMS {
		if t.Addr == addr && t.Name == name {
			return true
		}
	}
	return false
}

// qrow <이름>: 대기열 그 줄(없으면 nil)
func qrow(n string) Row {
	for _, r := range qRows() {
		if r.F(qName) == n {
			return r
		}
	}
	return nil
}

// qown <이름>: 그 줄 주인(없으면 -)
func qown(n string) string {
	if r := qrow(n); r != nil {
		return dash(r.F(qOwner))
	}
	return "-"
}

// hcLaneReport: env -u HC_TERMINAL_ID -u HC_LANE hc-lane report <상태> - <글> --name <레인> [--to] [--key]. 0 됨
func hcLaneReport(state, msg, name, to, key string) int {
	a := []string{"report", state, "-", msg, "--name", name}
	if to != "" {
		a = append(a, "--to", to)
	}
	if key != "" {
		a = append(a, "--key", key)
	}
	_, rc := Cmd{Name: "hc-lane", Args: a, Unset: []string{"HC_TERMINAL_ID", "HC_LANE"}}.Run()
	return rc
}
