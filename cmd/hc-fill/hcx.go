package main

// hc 부르기·읽기: hc ls·machine ls JSON(셸 판 jq 식과 같은 뜻), 원격 sh(onm), 실행 키(pget), 터미널 표 조회

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"
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

// parseTerms: hc ls --json → 터미널 표. ok=false면 .terminals가 배열이 아니거나 JSON이 아님
func parseTerms(raw string) ([]Term, bool) {
	vs, ok := decodeAll(raw)
	if !ok || len(vs) == 0 {
		return nil, false
	}
	var ts []Term
	good := false
	for _, v := range vs {
		arr, isArr := jget(v, "terminals").([]any)
		if !isArr {
			continue
		}
		good = true
		for _, t := range arr {
			// 주소가 글자가 아니거나 비면 그 목록으로 자리·중복을 셀 수 없다 — 셸 판(jq .address | contains 오류)처럼 못 읽음(gofix-fill F02)
			addr, isStr := jget(t, "address").(string)
			if !isStr || addr == "" {
				return nil, false
			}
			m := LOCAL
			if i := strings.IndexByte(addr, '/'); i >= 0 {
				m = addr[:i]
			}
			st := "-"
			switch s := jget(t, "status").(type) {
			case map[string]any:
				st = jor(s["state"], "-")
			case nil:
			default:
				st = jstr(s)
			}
			ts = append(ts, Term{tsvEsc(m), tsvEsc(addr), tsvEsc(jor(jget(t, "agent"), "-")), tsvEsc(jor(jget(t, "name"), "-")),
				tsvEsc(st), tsvEsc(jor(jget(t, "cwd"), "-"))})
		}
	}
	return ts, good
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

// connected: 연결된 기계(못 읽으면 빈 목록)
func connected() []string {
	raw, rc := run("hc", "machine", "ls", "--json")
	if rc != 0 {
		return nil
	}
	l, _ := jsonList(raw, "machines", "name", isConnected)
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
		return Cmd{Name: "sh", Args: a, In: in, Err: errPass}.Run()
	}
	h := []string{"exec"}
	if in != nil {
		h = append(h, "-i")
	}
	h = append(h, "--on", m, "--", "sh")
	return Cmd{Name: "hc", Args: append(h, a...), In: in, Err: errPass}.Run()
}

func objects(vs []any) []map[string]any {
	var o []map[string]any
	for _, v := range vs {
		if m, ok := v.(map[string]any); ok {
			o = append(o, m)
		}
	}
	return o
}

// pget <주소>: PRUN(<기계 접두 없는 터미널>@<created_ms>)·PNAME. 0 살아 있음, 3 끝남 확정(not_found·finished), 1 모름
func pget(addr string) int {
	PRUN, PNAME = "", ""
	out, rc := Cmd{Name: "hc", Args: []string{"get", addr, "--json"}, Err: errMerge}.Run()
	vs, ok := decodeAll(out)
	if ok && len(vs) > 0 {
		last := vs[len(vs)-1]
		k := jget(last, "error", "kind")
		if k == nil || k == false {
			k = jget(last, "kind")
		}
		if jstr(k) == "not_found" {
			return 3
		}
	}
	if rc != 0 {
		return 1
	}
	if !ok {
		return 1
	}
	obs := objects(vs)
	if len(obs) == 0 {
		return 1
	}
	o := obs[0]
	if jstr(o["state"]) == "finished" {
		return 3
	}
	c := ""
	if o["created_ms"] != nil && o["created_ms"] != false {
		c = jstr(o["created_ms"])
	}
	if !isDigits(c) {
		return 1
	}
	PRUN = addr[strings.LastIndexByte(addr, '/')+1:] + "@" + c
	PNAME = jor(o["name"], "-")
	return 0
}

// startState <주소> <레인>: 띄운 터미널의 hc get status가 WAIT초 안에 잡히는지. 0 잡힘(START_RUN), 1 안 잡힘 확정(그 레인 터미널이
// 살아 있는데 status 없음·끝남·not_found), 2 모름(조회 실패·잘린 응답·다른 이름이나 주소·시작 시각 없음 — 시작했을 수 있다, 재감사 R-01·R-05)
func startState(addr, lane string) int {
	end := time.Now().Unix() + int64(WAIT)
	for {
		if r := startOnce(addr, lane); r == 0 || time.Now().Unix() >= end {
			return r
		}
		time.Sleep(time.Duration(POLL) * time.Second)
	}
}

// startOnce: startState 한 번. 응답 전체가 JSON 값 하나이고 hc get이 성공했을 때만 읽는다(gofix-fill F03)
func startOnce(addr, lane string) int {
	out, rc := run("hc", "get", addr, "--json")
	vs, ok := decodeAll(out)
	if !ok || len(vs) != 1 {
		return 2
	}
	o, isObj := vs[0].(map[string]any)
	if !isObj {
		return 2
	}
	if jstr(jget(o, "error", "kind")) == "not_found" {
		return 1
	}
	if rc != 0 || o["address"] != any(addr) {
		return 2
	}
	name, named := o["name"].(string)
	if named && name != lane || !named && o["name"] != nil {
		return 2 // 그 주소가 다른 터미널이다
	}
	if o["state"] == any("finished") {
		return 1
	}
	created := jstr(o["created_ms"])
	if !named || o["state"] != any("live") || !isDigits(created) {
		return 2
	}
	switch o["status"].(type) {
	case map[string]any:
		START_RUN = addr[strings.LastIndexByte(addr, '/')+1:] + "@" + created
		return 0
	case nil:
		return 1
	}
	return 2
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
		if r.F(1) == n {
			return r
		}
	}
	return nil
}

// qown <이름>: 그 줄 주인(없으면 -)
func qown(n string) string {
	if r := qrow(n); r != nil {
		return dash(r.F(12))
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
