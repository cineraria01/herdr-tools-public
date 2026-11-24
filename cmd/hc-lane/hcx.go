// hcx.go: 셸 판 common.sh의 작은 도구들(die·val·cfg·capt·utc·flat·bcut·jrec·live_created_ms)과 hc 부르기.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/events"
	"herdr-tools/internal/proc"
)

const tab = "\t"

// evstates: report가 받는 상태(셸 판 bin/hc-lane의 evstates와 같은 순서)
const evstates = "중간 완료 착지 막힘 판정 제출 주차"

// noHelp: 도움말(-h·--help)이 든 인자는 거른다(→ usage)
func noHelp(a []string) bool {
	for _, s := range a {
		if s == "-h" || s == "--help" {
			return false
		}
	}
	return true
}

// thenExit: 하위 명령이 끝나면 정리 일(cleanups — watch 잠금 지우기 등)을 하고 끝낸다(셸 trap EXIT)
func thenExit(run func([]string) int) func([]string) int {
	return func(a []string) int { exit(run(a)); return 0 }
}

// val: 「--옵션 값」의 값이 있나(셸 판 val — 없으면 「<옵션> 값이 없다」로 끝냄)
func val(a []string, i int) string {
	if i+1 >= len(a) {
		dieArg("%s 값이 없다", a[i])
	}
	return a[i+1]
}

// errMode: 자식의 표준 오류를 어떻게(버림 2>/dev/null · 합침 2>&1 · 그대로)
type errMode int

const (
	errDrop errMode = iota
	errMerge
	errPass
)

// capt: <sec>초 안에 안 끝나면 끊는다(셸 판 capt — perl alarm). sec 0이면 상한 없음. 표준 입력은 /dev/null.
// 표준 출력(끝 줄바꿈 뺌)과 종료 코드
func capt(sec int, em errMode, name string, args ...string) (string, int) {
	switch em {
	case errMerge:
		return captTo(sec, true, nil, name, args...)
	case errPass:
		return captTo(sec, false, os.Stderr, name, args...)
	}
	return captTo(sec, false, nil, name, args...)
}

func hcRun(sec int, em errMode, args ...string) (string, int) { return capt(sec, em, "hc", args...) }

// captErr: capt와 같되 표준 오류를 eb에 받는다
func captErr(sec int, eb io.Writer, name string, args ...string) (string, int) {
	return captTo(sec, false, eb, name, args...)
}

// captTo: merge면 표준 오류도 출력에, 아니면 eb(nil이면 버림)에. sec초(0이면 proc.Limit 표 — hc 120초 등, 표도 0이면 상한 없음) 안에
// 안 끝나면 출력 없이 proc.TimeoutCode(124, 모름 — gofix M4·L04: 끝없이 기다리거나 끊긴 반쪽 출력을 읽지 않는다).
// 무리는 이 프로세스와 같다(끄는 신호·hc-fill의 무리째 끄기를 함께 받고, watch의 신호 정리와 proc 신호 처리기가 다투지 않게).
// 상한에 걸리면 끄기 직전에 그 자식의 자손(배경 손자)을 찾아 함께 끈다 — 직계 자식만 끄면 손자가 남아 늦게 표식을 쓴다(재감사 R-23).
// 우두머리가 끝났는데 손자가 출력 관을 쥐고 있으면(WaitDelay) 출력을 믿지 않고 모름
func captTo(sec int, merge bool, eb io.Writer, name string, args ...string) (string, int) {
	d := time.Duration(sec) * time.Second
	if sec == 0 {
		d = proc.Limit(append([]string{name}, args...))
	}
	ctx := context.Background()
	if d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	c := exec.CommandContext(ctx, name, args...)
	c.Cancel = func() error {
		killTree(c.Process.Pid) // 자손 먼저(우두머리가 먼저 죽으면 자손이 고아가 돼 못 찾는다)
		return c.Process.Kill()
	}
	c.WaitDelay = time.Second
	var b bytes.Buffer
	c.Stdout, c.Stderr = &b, eb
	if merge {
		c.Stderr = &b
	}
	err := c.Run()
	if ctx.Err() != nil || errors.Is(err, exec.ErrWaitDelay) {
		return "", proc.TimeoutCode
	}
	return strings.TrimRight(b.String(), "\n"), proc.Code(err)
}

func utcNow() string              { return events.UTC(time.Now()) }
func utcAt(s int64) string        { return events.UTC(time.Unix(s, 0)) }
func flat(s string) string        { return events.Flat(s) }
func bcut(n int, s string) string { return events.Bcut(n, s) }

// lastSeg: 끝의 /<…>(sub(/^.*\//, ""))
func lastSeg(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

func machineName() string {
	if m := os.Getenv("HC_LANE_MACHINE"); m != "" {
		return m
	}
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	if i := strings.Index(h, "."); i >= 0 {
		h = h[:i]
	}
	return h
}

// ckey: 주소를 파일 이름 조각으로(tr -c 'A-Za-z0-9_-' _)
func ckey(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-' {
			b.WriteByte(c)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// onlyChars: s가 모두 ok 글자인가(빈 값은 참)
func onlyChars(s, ok string) bool {
	for _, r := range s {
		if !strings.ContainsRune(ok, r) {
			return false
		}
	}
	return true
}

func atoi(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }

// cfg: 개인 설정(herdr-tools-config, 이 바이너리 곁의 것 먼저). 도구가 없는 기계는 기본값, 읽기 오류는 종료 1
func cfg(key, def string) string {
	c := filepath.Join(filepath.Dir(selfExe()), "herdr-tools-config")
	if st, err := os.Stat(c); err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
		p, err := exec.LookPath("herdr-tools-config")
		if err != nil {
			return def
		}
		c = p
	}
	out, rc := capt(0, errPass, c, "get", key, def)
	if rc != 0 {
		die("설정 %s 을 읽지 못했다(herdr-tools-config)", key)
	}
	return out
}

// cfgInt: 0 이상 정수 설정(아니면 「설정 <키>는 0 이상 정수(<단위>)」로 끝냄)
func cfgInt(key, def, unit string) int64 {
	v := cfg(key, def)
	if !digits(v) {
		die("설정 %s는 0 이상 정수(%s): %s", key, unit, v)
	}
	return atoi(v)
}

// ---- JSON(hc 출력) ----

// jone: 출력 전체가 JSON 객체 정확히 하나일 때만(앞뒤 공백 말고 찌꺼기 없음) 그 객체, 아니면 nil — 판정(있음·없음)에 쓰는 조회용
func jone(s string) map[string]any {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var m map[string]any
	if d.Decode(&m) != nil || m == nil {
		return nil
	}
	if _, err := d.Token(); err != io.EOF {
		return nil
	}
	return m
}

// jparse: hc 출력을 JSON 객체로(통째로, 안 되면 줄마다 첫 객체). 없으면 nil
func jparse(s string) map[string]any {
	try := func(t string) map[string]any {
		d := json.NewDecoder(strings.NewReader(t))
		d.UseNumber()
		var m map[string]any
		if d.Decode(&m) != nil {
			return nil
		}
		return m
	}
	if m := try(strings.TrimSpace(s)); m != nil {
		return m
	}
	for _, l := range strings.Split(s, "\n") {
		if m := try(strings.TrimSpace(l)); m != nil {
			return m
		}
	}
	return nil
}

// jscalar: 문자열이면 그대로, 정수면 그 글자. 그 밖(없음·null·객체)은 ok=false
func jscalar(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case json.Number:
		if _, err := x.Int64(); err == nil {
			return x.String(), true
		}
	}
	return "", false
}

func jget(m map[string]any, k string) (string, bool) {
	if m == nil {
		return "", false
	}
	return jscalar(m[k])
}

// jlistOK: hc 출력이 통째로 JSON 객체 하나(뒤 찌꺼기 없음)이고 listKey가 배열(null 아님)이며 그 원소가 모두 객체이고 req 키마다
// 비어 있지 않은 문자열이 있다 — 아니면(잘림·형식 바뀜) 목록을 모른다. 0으로 끝난 부실한 목록을 「대상 없음」으로 읽지 않게
// (gofix L05·K2, 재감사 R-14)
func jlistOK(s, listKey string, req ...string) bool {
	d := json.NewDecoder(strings.NewReader(s))
	var m map[string]json.RawMessage
	if d.Decode(&m) != nil || m == nil {
		return false
	}
	if _, err := d.Token(); err != io.EOF {
		return false
	}
	var arr []map[string]json.RawMessage
	if json.Unmarshal(m[listKey], &arr) != nil || arr == nil {
		return false
	}
	for _, it := range arr {
		if it == nil {
			return false
		}
		for _, k := range req {
			if v, ok := jstr(it[k]); !ok || v == "" {
				return false
			}
		}
	}
	return true
}

// jlist: 목록(listKey)의 객체마다 키 값들(없거나 null이면 -). 첫 키가 없는 객체는 버린다(셸 판 jrec를 JSON으로 읽기)
func jlist(s, listKey string, keys ...string) [][]string {
	m := jparse(s)
	if m == nil {
		return nil
	}
	arr, _ := m[listKey].([]any)
	var out [][]string
	for _, it := range arr {
		o, ok := it.(map[string]any)
		if !ok {
			continue
		}
		r := make([]string, len(keys))
		for i, k := range keys {
			v, ok := jscalar(o[k])
			if !ok {
				v = "-"
			}
			r[i] = v
		}
		if r[0] == "-" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// liveCreatedMs: hc get JSON의 최상위 state가 live이고 created_ms가 0 이상 정수면 그 값(셸 판 live_created_ms)
func liveCreatedMs(s string) (string, bool) {
	m := jparse(s)
	if m == nil || m["state"] != "live" {
		return "", false
	}
	n, ok := m["created_ms"].(json.Number)
	if !ok {
		return "", false
	}
	if i, err := n.Int64(); err == nil {
		if i < 0 {
			return "", false
		}
		return strconv.FormatInt(i, 10), true
	}
	f, err := n.Float64()
	if err != nil || f < 0 || f != float64(int64(f)) {
		return "", false
	}
	return strconv.FormatInt(int64(f), 10), true
}

// terminalCreatedMs: 그 터미널의 지금 실행(created_ms). 모르면 ok=false
func terminalCreatedMs(addr string) (string, bool) {
	if addr == "" {
		return "", false
	}
	out, rc := hcRun(3, errDrop, "get", addr, "--json")
	if rc != 0 {
		return "", false
	}
	return liveCreatedMs(out)
}

// topNum: 최상위 정수 값. 없으면 fallback 객체(예 foreground)의 같은 키
func topNum(m map[string]any, k, fallback string) string {
	if v, ok := m[k].(json.Number); ok {
		if _, err := v.Int64(); err == nil {
			return v.String()
		}
	}
	if fallback != "" {
		if f, ok := m[fallback].(map[string]any); ok {
			if v, ok := f[k].(json.Number); ok {
				if _, err := v.Int64(); err == nil {
					return v.String()
				}
			}
		}
	}
	return ""
}

// mustRe: 설정에서 온 정규식(grep -E). i면 대소문자 무시. 틀리면 끝냄
func mustRe(key, pat string, i bool) *regexp.Regexp {
	p := pat
	if i {
		p = "(?i)" + p
	}
	re, err := regexp.Compile(p)
	if err != nil {
		die("설정 %s 정규식을 읽지 못했다: %s (%v)", key, pat, err)
	}
	return re
}

// tailFileLines: 파일 끝의 n줄만(끝에서 64KB씩 거꾸로 읽는다 — 줄 수와 무관하게 파일 전체를 올리지 않는다, gofix L14). n이 0 이하면 readLines
func tailFileLines(path string, n int) ([]string, error) {
	if n <= 0 {
		return readLines(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	const chunk = 64 << 10
	var buf []byte
	for pos := end; pos > 0 && bytes.Count(bytes.TrimRight(buf, "\n"), []byte("\n")) < n; {
		sz := int64(chunk)
		if pos < sz {
			sz = pos
		}
		pos -= sz
		b := make([]byte, sz)
		if _, err := f.ReadAt(b, pos); err != nil {
			return nil, err
		}
		buf = append(b, buf...)
	}
	s := strings.TrimRight(string(buf), "\n")
	if s == "" {
		return nil, nil
	}
	ls := strings.Split(s, "\n")
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	return ls, nil
}

// eachLine: 파일을 줄마다 fn에. 64KB를 넘는 줄은 64KB 조각마다 따로 fn에 준다(뒤쪽 조각도 버리지 않는다 — 긴 줄 끝의 실패 표식,
// 재감사 R-28). fn이 false면 멈춘다. 파일 전체를 올리지 않는다.
// ponytail: 조각 경계에 걸친 무늬는 놓친다 — 실패 표식은 짧아 드물다, 필요하면 앞 조각 끝 몇백 바이트를 겹쳐 준다
func eachLine(path string, fn func(string) bool) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64<<10)
	for {
		l, _, err := r.ReadLine()
		if err != nil {
			return
		}
		if !fn(string(l)) {
			return
		}
	}
}

// readLines: 파일의 줄들(없거나 못 읽으면 nil, err)
func readLines(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := strings.TrimRight(string(b), "\n")
	if s == "" {
		return nil, nil
	}
	return strings.Split(s, "\n"), nil
}

// appendLines: 줄들을 한 번에 더한다(O_APPEND)
func appendLines(path string, lines ...string) error {
	if len(lines) == 0 {
		return nil
	}
	return atomic.Append(path, 0o666, strings.Join(lines, "\n")+"\n")
}

// at: 탭 칸 f[i](0부터, 없으면 빈 값) — 사건 줄이 아닌 표(lanes.tsv·after.tsv·판정 파일·대기열)만. 사건 줄은 events.Cols의
// 이름 칸으로 읽는다(1부터 세는 옛 fld와 0부터 세는 f[n]이 한 줄에 섞였다 — events-named-cols, evnamed_guard_test)
func at(f []string, i int) string {
	if i < len(f) {
		return f[i]
	}
	return ""
}
