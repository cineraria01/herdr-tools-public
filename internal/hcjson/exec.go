package hcjson

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Exec: hc exec --json의 결과(프로그램이 끝난 꼴 — 0이 아닌 종료·시간 넘김·신호도 결과다)
type Exec struct {
	Code      int // 프로그램의 종료 코드. 시간 넘김·신호로 끝나 code가 null이면 -1
	Signal    int // 끝낸 신호, 없으면(null) 0
	TimedOut  bool
	Truncated bool // hc가 출력을 잘랐다 — Stdout·Stderr는 앞부분뿐
	Stdout    []byte
	Stderr    []byte
}

// ExecError: hc 자신의 실패(표준 오류의 {"error":{kind,…}}) — 프로그램은 돌지 않았거나 결과를 모른다
type ExecError struct {
	Kind, Message string
	RC            int // hc의 종료 코드
}

func (e *ExecError) Error() string {
	return fmt.Sprintf("hc exec 실패(%s, 종료 %d): %s", e.Kind, e.RC, e.Message)
}

// ParseExec: hc exec --json의 (표준 출력, 표준 오류, hc 종료 코드)를 읽는다. hc 종료 0이면 표준 출력이 결과 하나(code·signal·
// stdout·stderr·timed_out·truncated 칸 필수), 0이 아니면 표준 출력이 비고 표준 오류가 오류 객체 하나여야 *ExecError.
// 그 밖의 꼴은 모두 ExecError가 아닌 오류(모름). 둘 다 「없음」이 아니다 — 부르는 쪽은 그 바퀴에 아무것도 바꾸지 않는다
func ParseExec(stdout, stderr []byte, rc int) (Exec, error) {
	if rc != 0 {
		if strings.TrimSpace(string(stdout)) != "" {
			return Exec{}, fmt.Errorf("hc 종료 %d인데 표준 출력이 있다", rc)
		}
		v, err := one(stderr)
		if err != nil {
			return Exec{}, fmt.Errorf("hc 종료 %d, 표준 오류: %w", rc, err)
		}
		m, err := obj(v)
		if err != nil {
			return Exec{}, fmt.Errorf("hc 종료 %d, 표준 오류: %w", rc, err)
		}
		e, err := obj(m["error"])
		if err != nil {
			return Exec{}, fmt.Errorf("hc 종료 %d, error 칸: %w", rc, err)
		}
		kind, err := need(e, "kind")
		if err != nil {
			return Exec{}, fmt.Errorf("hc 종료 %d: %w", rc, err)
		}
		msg, _, err := str(e, "message")
		if err != nil {
			return Exec{}, err
		}
		return Exec{}, &ExecError{Kind: kind, Message: msg, RC: rc}
	}
	v, err := one(stdout)
	if err != nil {
		return Exec{}, err
	}
	m, err := obj(v)
	if err != nil {
		return Exec{}, err
	}
	for _, k := range []string{"code", "signal", "stdout", "stderr", "timed_out", "truncated"} {
		if _, ok := m[k]; !ok {
			return Exec{}, fmt.Errorf("%s 칸이 없다", k)
		}
	}
	var r Exec
	code, err := num(m, "code")
	if err != nil {
		return Exec{}, err
	}
	sig, err := num(m, "signal")
	if err != nil {
		return Exec{}, err
	}
	codeNull, sigNull := isNull(m["code"]), isNull(m["signal"])
	if codeNull == sigNull { // 둘 중 정확히 하나만 — 종료 코드로 끝났거나 신호로 끝났거나
		return Exec{}, errors.New("code·signal 가운데 하나만 있어야 한다")
	}
	r.Code, r.Signal = int(code), int(sig)
	if codeNull {
		r.Code = -1
	}
	if r.TimedOut, err = boolean(m, "timed_out"); err != nil {
		return Exec{}, err
	}
	if r.Truncated, err = boolean(m, "truncated"); err != nil {
		return Exec{}, err
	}
	if r.Stdout, err = b64(m, "stdout"); err != nil {
		return Exec{}, err
	}
	if r.Stderr, err = b64(m, "stderr"); err != nil {
		return Exec{}, err
	}
	return r, nil
}

func boolean(m map[string]json.RawMessage, k string) (bool, error) {
	switch string(m[k]) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("%s 칸이 참·거짓이 아님: %s", k, m[k])
}

func b64(m map[string]json.RawMessage, k string) ([]byte, error) {
	s, ok, err := str(m, k)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%s 칸이 null", k)
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%s 칸이 base64가 아님: %w", k, err)
	}
	return b, nil
}
