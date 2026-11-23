package hcjson

import (
	"errors"
	"testing"
)

// hc exec --json 판독(hc-exec-result). 견본은 2026-11-23 탐침의 실제 출력 그대로(hc 0.1.21, 로컬과 --on 같은 꼴).
// 결과(프로그램의 종료 — 0이 아니어도·시간 넘김·신호)는 hc 종료 0 + 표준 출력에 ExecResult 하나, hc 자신의 실패는
// hc 종료 ≠0 + 표준 출력 빔 + 표준 오류에 {"error":{kind,…}}. 꼴이 다르면 모름(오류).

func TestParseExecResults(t *testing.T) {
	cases := []struct {
		name, out       string
		code, signal    int
		timedOut, trunc bool
		stdout, stderr  string
	}{
		{"true", `{"code":0,"ms":2,"signal":null,"stderr":"","stdout":"","timed_out":false,"truncated":false}` + "\n", 0, 0, false, false, "", ""},
		{"false", `{"code":1,"ms":2,"signal":null,"stderr":"","stdout":"","timed_out":false,"truncated":false}` + "\n", 1, 0, false, false, "", ""},
		{"exit 3", `{"code":3,"ms":4,"signal":null,"stderr":"ZXJyCg==","stdout":"b3V0Cg==","timed_out":false,"truncated":false}` + "\n", 3, 0, false, false, "out\n", "err\n"},
		{"시간 넘김", `{"code":null,"ms":1011,"signal":15,"stderr":"","stdout":"","timed_out":true,"truncated":false}` + "\n", -1, 15, true, false, "", ""},
		{"신호", `{"code":null,"ms":7,"signal":15,"stderr":"","stdout":"","timed_out":false,"truncated":false}` + "\n", -1, 15, false, false, "", ""},
		{"잘림", `{"code":0,"ms":36,"signal":null,"stderr":"","stdout":"MDEyMzQ1Njc4OQo=","timed_out":false,"truncated":true}` + "\n", 0, 0, false, true, "0123456789\n", ""},
	}
	for _, c := range cases {
		r, err := ParseExec([]byte(c.out), nil, 0)
		if err != nil {
			t.Errorf("%s: 오류 %v", c.name, err)
			continue
		}
		if r.Code != c.code || r.Signal != c.signal || r.TimedOut != c.timedOut || r.Truncated != c.trunc ||
			string(r.Stdout) != c.stdout || string(r.Stderr) != c.stderr {
			t.Errorf("%s: %+v", c.name, r)
		}
	}
}

func TestParseExecHcFailure(t *testing.T) {
	cases := []struct {
		name, errOut, kind string
		rc                 int
	}{
		{"spawn_failed", `{"error":{"hint":"","kind":"spawn_failed","message":"cannot start /no/such/prog: No such file or directory (os error 2)"}}` + "\n", "spawn_failed", 1},
		{"machine_unknown", `{"error":{"hint":"machines: m1, m2","kind":"machine_unknown","message":"unknown machine \"nosuchmachine\""}}` + "\n", "machine_unknown", 4},
	}
	for _, c := range cases {
		_, err := ParseExec(nil, []byte(c.errOut), c.rc)
		var e *ExecError
		if !errors.As(err, &e) || e.Kind != c.kind {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

// 지휘 기계 hc 0.1.22 탐침(2026-11-23T10:11:55Z, 지휘가 돌림 — hc-fill reports/hc-exec-probe-0122.txt)의 줄 그대로.
// 7의 hint 속 기계 이름 목록만 개인 값이라 m1, m2로 바꿨다
func TestParseExec0122(t *testing.T) {
	cases := []struct {
		name, out, errOut string
		rc, code          int
		stdout, kind      string
	}{
		{"1 true", `{"code":0,"ms":11,"signal":null,"stderr":"","stdout":"","timed_out":false,"truncated":false}`, "", 0, 0, "", ""},
		{"2 false", `{"code":1,"ms":10,"signal":null,"stderr":"","stdout":"","timed_out":false,"truncated":false}`, "", 0, 1, "", ""},
		{"3 exit 3", `{"code":3,"ms":8,"signal":null,"stderr":"","stdout":"","timed_out":false,"truncated":false}`, "", 0, 3, "", ""},
		{"4 없는 실행 파일", "", `{"error":{"hint":"","kind":"spawn_failed","message":"cannot start /no/such: No such file or directory (os error 2)"}}`, 1, 0, "", "spawn_failed"},
		{"5 --on false", `{"code":1,"ms":16,"signal":null,"stderr":"","stdout":"","timed_out":false,"truncated":false}`, "", 0, 1, "", ""},
		{"6 --on 출력", `{"code":0,"ms":16,"signal":null,"stderr":"","stdout":"aGkK","timed_out":false,"truncated":false}`, "", 0, 0, "hi\n", ""},
		{"7 --on 모르는 기계", "", `{"error":{"hint":"machines: m1, m2","kind":"machine_unknown","message":"unknown machine \"nosuch\""}}`, 4, 0, "", "machine_unknown"},
	}
	for _, c := range cases {
		r, err := ParseExec([]byte(c.out), []byte(c.errOut), c.rc)
		if c.kind != "" {
			var e *ExecError
			if !errors.As(err, &e) || e.Kind != c.kind || e.RC != c.rc {
				t.Errorf("%s: %v", c.name, err)
			}
			continue
		}
		if err != nil || r.Code != c.code || string(r.Stdout) != c.stdout || r.Signal != 0 || r.TimedOut || r.Truncated {
			t.Errorf("%s: %+v %v", c.name, r, err)
		}
	}
}

// 꼴이 다르면 모두 모름: hc 종료 ≠0인데 출력이 있음·오류 객체가 아님, 종료 0인데 결과가 아님·잘린 JSON·칸 형이 틀림
func TestParseExecUnknown(t *testing.T) {
	ok := `{"code":0,"ms":2,"signal":null,"stderr":"","stdout":"","timed_out":false,"truncated":false}`
	cases := []struct {
		name, out, errOut string
		rc                int
	}{
		{"빈 출력 rc 0", "", "", 0},
		{"rc 1 빈 오류", "", "", 1},
		{"rc 1 글 오류", "", "boom\n", 1},
		{"rc 1 출력 있음", ok, "", 1},
		{"rc 124 시간 넘김 줄", "", "[hc] timed out after 1003 ms: ended\n", 124},
		{"잘린 JSON", `{"code":0,"ms":2,"sig`, "", 0},
		{"값 둘", ok + ok, "", 0},
		{"code 글", `{"code":"0","ms":2,"signal":null,"stderr":"","stdout":"","timed_out":false,"truncated":false}`, "", 0},
		{"code 없음", `{"ms":2,"signal":null,"stderr":"","stdout":"","timed_out":false,"truncated":false}`, "", 0},
		{"code·signal 둘 다 null", `{"code":null,"ms":2,"signal":null,"stderr":"","stdout":"","timed_out":false,"truncated":false}`, "", 0},
		{"stdout base64 아님", `{"code":0,"ms":2,"signal":null,"stderr":"","stdout":"!!","timed_out":false,"truncated":false}`, "", 0},
		{"timed_out 없음", `{"code":0,"ms":2,"signal":null,"stderr":"","stdout":"","truncated":false}`, "", 0},
		{"오류 객체 kind 없음", "", `{"error":{"message":"x"}}`, 1},
		{"rc 0인데 오류 객체", `{"error":{"kind":"unavailable"}}`, "", 0},
	}
	for _, c := range cases {
		r, err := ParseExec([]byte(c.out), []byte(c.errOut), c.rc)
		if err == nil {
			t.Errorf("%s: 모름이어야 하는데 %+v", c.name, r)
		}
		var e *ExecError
		if errors.As(err, &e) {
			t.Errorf("%s: 꼴이 다른 것을 hc 실패(%s)로 읽음", c.name, e.Kind)
		}
	}
}
