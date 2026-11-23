// Package hcjson: hc ls·get·machine ls --json 응답의 엄격 판독. hc를 부르지 않는다(실행은 부르는 쪽이 proc 상한 안에서).
//
// 계약: 입력 전체가 JSON 값 정확히 하나, 목록은 주소를 읽을 수 없는 항목이 하나라도 있으면 전체 오류, 주소는 읽히는데 다른 칸이
// 계약을 어긴 항목은 그 항목만 Unknown(살아 있고 무엇을 하는지 모름 — R-3M01), 칸의 형이 틀리면 오류
// (null·없음만 「없음」), state·status는 아는 값만, live 항목의 cwd는 절대 경로(유닉스, 또는 Windows 드라이브·UNC). 오류 = 모름 — 부르는 쪽은 상태·큐·사건·기록을
// 바꾸지 않는다. 서로 다른 시각의 두 조회가 어긋나면 「없음」이 아니라 모름이고, 같은 주소라도 Created·Parent·Name이 기록과
// 다르면 다른 실행(주소 재사용)이다 — 그 기록으로 발송·소유를 정하지 않는다.
package hcjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Terminal: hc ls·get의 터미널 한 항목
type Terminal struct {
	Address string                     // 필수 · 비지 않은 문자열(원격은 「기계/tN」)
	State   string                     // 필수 · "live" | "finished"
	Status  string                     // "" | "idle" | "working" | "done" | "blocked" | "error" (없음·null → ""; get의 객체는 그 state)
	Agent   string                     // 없음·null → ""
	Cwd     string                     // live면 필수: 절대 경로를 filepath.Clean 한 값. finished면 없음·null 허용
	Name    string                     // 없음·null → "" 이고 HasName=false
	HasName bool                       //
	Parent  string                     // 없음·null → ""
	PID     int                        // (get) 없음·null → 0
	Created int64                      // (get) created_ms, 없음·null → 0
	Labels  map[string]string          // 없음·null → 빈 맵
	Raw     map[string]json.RawMessage // 항목의 칸 원문 그대로(turn·where·exit_code … 도구 고유 칸은 여기서)
	// Unknown: (ls) 이 항목만 계약을 어김 — 까닭. 주소만 믿을 수 있고 State는 "live", 나머지 칸은 비운다.
	// 부르는 쪽은 그 터미널을 「살아 있고 무엇을 하는지 모름」으로 본다: 자리 셈·정리·발송에서 건드리지 않는다(R-3M01)
	Unknown string
}

// Machine: hc ls·machine ls의 기계 한 항목. 둘 다 필수 · 비지 않은 문자열
type Machine struct{ Name, State string }

// List: hc ls --json [--all]
type List struct {
	Terminals   []Terminal
	Machines    []Machine
	HasMachines bool // 응답에 machines 칸이 있었다(없으면 기계 연결을 「모름」으로)
}

var (
	states   = map[string]bool{"live": true, "finished": true}
	statuses = map[string]bool{"idle": true, "working": true, "done": true, "blocked": true, "error": true}
)

// one: 입력 전체가 JSON 값 정확히 하나(앞뒤 공백만)일 때 그 값
func one(b []byte) (json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	var v json.RawMessage
	if err := d.Decode(&v); err != nil {
		return nil, fmt.Errorf("JSON 값 하나를 못 읽음: %w", err)
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("JSON 값 뒤에 찌꺼기")
	}
	return v, nil
}

func isNull(r json.RawMessage) bool { return len(r) == 0 || string(r) == "null" }

// obj: JSON 객체(null 아님)
func obj(r json.RawMessage) (map[string]json.RawMessage, error) {
	if len(r) == 0 || r[0] != '{' {
		return nil, errors.New("객체가 아님")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(r, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// arr: JSON 배열(null 아님)
func arr(r json.RawMessage) ([]json.RawMessage, error) {
	if len(r) == 0 || r[0] != '[' {
		return nil, errors.New("배열이 아님")
	}
	var a []json.RawMessage
	if err := json.Unmarshal(r, &a); err != nil {
		return nil, err
	}
	return a, nil
}

// str: 문자열 칸 — 없음·null은 ("", false, nil), 문자열 아닌 값은 오류
func str(m map[string]json.RawMessage, k string) (string, bool, error) {
	r, ok := m[k]
	if !ok || isNull(r) {
		return "", false, nil
	}
	var s string
	if r[0] != '"' || json.Unmarshal(r, &s) != nil {
		return "", false, fmt.Errorf("%s 칸이 문자열이 아님: %s", k, r)
	}
	return s, true, nil
}

// need: 필수 · 비지 않은 문자열 칸
func need(m map[string]json.RawMessage, k string) (string, error) {
	s, ok, err := str(m, k)
	if err != nil {
		return "", err
	}
	if !ok || s == "" {
		return "", fmt.Errorf("%s 칸이 없거나 빔", k)
	}
	return s, nil
}

// num: 음 아닌 JSON 정수 칸 — 없음·null은 0
func num(m map[string]json.RawMessage, k string) (int64, error) {
	r, ok := m[k]
	if !ok || isNull(r) {
		return 0, nil
	}
	for _, c := range r {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("%s 칸이 음 아닌 정수가 아님: %s", k, r)
		}
	}
	var n int64
	if err := json.Unmarshal(r, &n); err != nil {
		return 0, fmt.Errorf("%s 칸: %w", k, err)
	}
	return n, nil
}

func terminal(r json.RawMessage) (Terminal, error) {
	m, err := obj(r)
	if err != nil {
		return Terminal{}, fmt.Errorf("터미널 항목: %w", err)
	}
	t := Terminal{Raw: m, Labels: map[string]string{}}
	if t.Address, err = need(m, "address"); err != nil {
		return t, err
	}
	if t.State, err = need(m, "state"); err != nil {
		return t, err
	}
	if !states[t.State] {
		return t, fmt.Errorf("%s: 모르는 state %q", t.Address, t.State)
	}
	if t.Status, err = status(m); err != nil {
		return t, fmt.Errorf("%s: %w", t.Address, err)
	}
	for _, f := range []struct {
		k string
		p *string
		h *bool
	}{{"agent", &t.Agent, nil}, {"name", &t.Name, &t.HasName}, {"parent", &t.Parent, nil}, {"cwd", &t.Cwd, nil}} {
		s, ok, err := str(m, f.k)
		if err != nil {
			return t, fmt.Errorf("%s: %w", t.Address, err)
		}
		*f.p = s
		if f.h != nil {
			*f.h = ok
		}
	}
	if filepath.IsAbs(t.Cwd) {
		t.Cwd = filepath.Clean(t.Cwd)
	} else if winAbs(t.Cwd) {
		// 다른 기계(Windows)의 터미널 — 이 기계 경로와 겹칠 수 없으니 그대로 둔다(R-3M01)
	} else if t.State == "live" {
		return t, fmt.Errorf("%s: live인데 cwd가 절대 경로가 아님(%q)", t.Address, t.Cwd)
	}
	pid, err := num(m, "pid")
	if err != nil {
		return t, fmt.Errorf("%s: %w", t.Address, err)
	}
	t.PID = int(pid)
	if t.Created, err = num(m, "created_ms"); err != nil {
		return t, fmt.Errorf("%s: %w", t.Address, err)
	}
	if lr, ok := m["labels"]; ok && !isNull(lr) {
		lm, err := obj(lr)
		if err != nil {
			return t, fmt.Errorf("%s: labels: %w", t.Address, err)
		}
		for k := range lm {
			v, ok, err := str(lm, k)
			if err != nil || !ok {
				return t, fmt.Errorf("%s: labels의 %s 값이 문자열이 아님", t.Address, k)
			}
			t.Labels[k] = v
		}
	}
	return t, nil
}

// winAbs: Windows 절대 경로 — 드라이브(`C:\`·`C:/`) 또는 UNC(`\\서버\공유`)
func winAbs(p string) bool {
	if len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
		c := p[0] | 0x20
		return c >= 'a' && c <= 'z'
	}
	return len(p) >= 3 && p[0] == '\\' && p[1] == '\\' && p[2] != '\\'
}

// status: ls는 문자열, get은 {"state":…} 객체. 없음·null → ""
func status(m map[string]json.RawMessage) (string, error) {
	r, ok := m["status"]
	if !ok || isNull(r) {
		return "", nil
	}
	if r[0] == '{' {
		sm, err := obj(r)
		if err != nil {
			return "", fmt.Errorf("status: %w", err)
		}
		m = sm
		if r, ok = m["state"]; !ok || isNull(r) {
			return "", errors.New("status 객체에 state가 없음")
		}
		s, _, err := str(m, "state")
		if err != nil || !statuses[s] {
			return "", fmt.Errorf("모르는 status.state %s", r)
		}
		return s, nil
	}
	s, _, err := str(m, "status")
	if err != nil || !statuses[s] {
		return "", fmt.Errorf("모르는 status %s", r)
	}
	return s, nil
}

func machines(r json.RawMessage) ([]Machine, error) {
	a, err := arr(r)
	if err != nil {
		return nil, fmt.Errorf("machines: %w", err)
	}
	ms := make([]Machine, 0, len(a))
	for _, x := range a {
		m, err := obj(x)
		if err != nil {
			return nil, fmt.Errorf("기계 항목: %w", err)
		}
		var mc Machine
		if mc.Name, err = need(m, "name"); err != nil {
			return nil, fmt.Errorf("기계 항목: %w", err)
		}
		if mc.State, err = need(m, "state"); err != nil {
			return nil, fmt.Errorf("기계 %s: %w", mc.Name, err)
		}
		ms = append(ms, mc)
	}
	return ms, nil
}

// ParseList: hc ls --json [--all]. terminals가 없음·null·배열 아님이면 오류, 빈 배열은 확인된 없음
func ParseList(b []byte) (List, error) {
	v, err := one(b)
	if err != nil {
		return List{}, err
	}
	top, err := obj(v)
	if err != nil {
		return List{}, fmt.Errorf("hc ls: %w", err)
	}
	ta, err := arr(top["terminals"])
	if err != nil {
		return List{}, fmt.Errorf("hc ls terminals: %w", err)
	}
	l := List{Terminals: make([]Terminal, 0, len(ta))}
	for _, x := range ta {
		t, err := terminal(x)
		if err != nil {
			// 주소를 읽을 수 있으면 그 항목만 모름 — 한 줄로 목록 전체를 버리지 않는다(R-3M01). 주소조차 없으면 전체 오류
			m, oerr := obj(x)
			if oerr != nil {
				return List{}, err
			}
			a, aerr := need(m, "address")
			if aerr != nil {
				return List{}, err
			}
			t = Terminal{Address: a, State: "live", Labels: map[string]string{}, Raw: m, Unknown: err.Error()}
		}
		l.Terminals = append(l.Terminals, t)
	}
	if mr, ok := top["machines"]; ok {
		if l.Machines, err = machines(mr); err != nil {
			return List{}, err
		}
		l.HasMachines = true
	}
	return l, nil
}

// ParseMachines: hc machine ls --json
func ParseMachines(b []byte) ([]Machine, error) {
	v, err := one(b)
	if err != nil {
		return nil, err
	}
	top, err := obj(v)
	if err != nil {
		return nil, fmt.Errorf("hc machine ls: %w", err)
	}
	return machines(top["machines"])
}

// ParseGet: hc get <want> --json 의 출력 b와 종료 코드 rc. rc 0(또는 11 끝남)이면 주소가 want와 같은 터미널 객체 하나,
// rc 3이고 완전한 not_found 오류 객체 하나면 (_, false, nil) — 확인된 없음. 그 밖은 모두 오류(모름)
func ParseGet(b []byte, rc int, want string) (Terminal, bool, error) {
	if rc != 0 && rc != 3 && rc != 11 {
		return Terminal{}, false, fmt.Errorf("hc get %s 종료 %d", want, rc)
	}
	v, err := one(b)
	if err != nil {
		return Terminal{}, false, fmt.Errorf("hc get %s 종료 %d: %w", want, rc, err)
	}
	m, err := obj(v)
	if err != nil {
		return Terminal{}, false, fmt.Errorf("hc get %s: %w", want, err)
	}
	if er, ok := m["error"]; ok {
		if rc == 3 && len(m) == 1 {
			if em, err := obj(er); err == nil {
				if k, _, err := str(em, "kind"); err == nil && k == "not_found" {
					return Terminal{}, false, nil
				}
			}
		}
		return Terminal{}, false, fmt.Errorf("hc get %s 종료 %d: 오류 응답 %s", want, rc, er)
	}
	if rc == 3 {
		return Terminal{}, false, fmt.Errorf("hc get %s 종료 3인데 오류 객체가 아님", want)
	}
	t, err := terminal(v)
	if err != nil {
		return Terminal{}, false, fmt.Errorf("hc get %s: %w", want, err)
	}
	if t.Address != want {
		return Terminal{}, false, fmt.Errorf("hc get %s: 응답 주소가 %s", want, t.Address)
	}
	if rc == 11 {
		t.State = "finished"
	}
	return t, true, nil
}

// Machine: 이름이 name인 기계
func (l List) Machine(name string) (Machine, bool) {
	for _, m := range l.Machines {
		if m.Name == name {
			return m, true
		}
	}
	return Machine{}, false
}

// Under: cwd가 dir이거나 그 아래인가 — 글자로(filepath.Clean, 대소문자 그대로, `/a/wt`가 `/a/wt2`에 걸리지 않게 경계까지).
// 이 기계 경로는 Uses(디스크로 같은 폴더인지까지)를 쓴다 — Under만 쓰는 것은 원격 기계 경로(이 기계에서 stat할 수 없다)
func Under(cwd, dir string) bool {
	if cwd == "" || dir == "" {
		return false
	}
	c, d := filepath.Clean(cwd), filepath.Clean(dir)
	return c == d || strings.HasPrefix(c, strings.TrimSuffix(d, string(filepath.Separator))+string(filepath.Separator))
}

// Uses: 이 기계 터미널의 cwd가 dir이거나 그 아래인가 — 글자(Under)와 디스크로(gofix5 lane/R-4A02·merger/R-4A03: macOS 기본 파일
// 시스템은 대소문자·한글 정규화를 가리지 않아 다른 철자가 같은 폴더다. hc는 띄울 때 준 글자 그대로 돌려준다). 디스크는 dir을 stat하고
// cwd에서 글자로 위로 올라가며 os.SameFile로 견주고, 아니면 cwd의 가장 깊은 「있는」 조상을 filepath.EvalSymlinks로 푼 실제 경로에서도
// 위로 올라가며 견준다(S5-01 — 링크가 dir 맨 위가 아니라 그 아래 폴더를 가리키면 글자 조상 어디도 dir과 같지 않다). dir이 없으면 글자만,
// dir을 stat하지 못하거나(없음 말고) cwd 쪽 stat·링크 풀기가 「없음」 말고 다른 오류면 true(모르면 쓰는 중). 원격 기계 경로는 여기서
// stat할 수 없다 — 부르는 쪽이 Under만 쓴다
func Uses(cwd, dir string) bool {
	if Under(cwd, dir) {
		return true
	}
	if cwd == "" || dir == "" {
		return false
	}
	di, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return true
	}
	walk := func(p string) bool { // p와 그 글자 조상 가운데 dir과 같은 폴더가 있거나 모르면 true
		for {
			st, err := os.Stat(p)
			if (err == nil && os.SameFile(st, di)) || (err != nil && !os.IsNotExist(err)) {
				return true
			}
			up := filepath.Dir(p)
			if up == p {
				return false
			}
			p = up
		}
	}
	p := filepath.Clean(cwd)
	if walk(p) {
		return true
	}
	for {
		q, err := filepath.EvalSymlinks(p)
		if err == nil {
			return walk(q)
		}
		if !os.IsNotExist(err) {
			return true
		}
		up := filepath.Dir(p)
		if up == p {
			return false
		}
		p = up
	}
}

// ByAddress: 주소가 addr인 항목 전부(둘이면 둘 다 — 부르는 쪽이 모름으로)
func (l List) ByAddress(addr string) []Terminal {
	var ts []Terminal
	for _, t := range l.Terminals {
		if t.Address == addr {
			ts = append(ts, t)
		}
	}
	return ts
}
