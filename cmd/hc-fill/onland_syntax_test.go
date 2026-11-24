//go:build !windows

package main

// on-land 명령 구문 검사(2026-11-24 지휘 지시): 괄호가 든 명령(sh -c 구문 오류) → status 끝에 한 줄, 바퀴는 그 설정 줄 키로 운영 사건.
// 맞는 명령은 조용하다. 명령은 돌리지 않는다. 고치기 전 코드는 status에 그 줄이 없고 사건도 없다

import (
	"os"
	"strings"
	"testing"
)

const olBad = "on-land app-dev app touch RAN && herdr-sync --landed --why 고침(x)"

func TestOnlandSyntaxStatus(t *testing.T) {
	k := newFillKit(t)
	f, err := os.OpenFile(k.home+"/conf/hc-fill.tsv", os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(olBad + "\non-land good app make dev-app\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}
	out, se, rc := k.run(t, "status")
	ls := strings.Split(strings.TrimRight(out, "\n"), "\n")
	last := ls[len(ls)-1]
	if rc != 0 || !strings.HasPrefix(last, "on-land app-dev 명령 구문 오류: ") || !strings.Contains(last, "(") || strings.Contains(out, "on-land good") {
		t.Fatalf("status 끝에 구문 오류 한 줄(맞는 줄은 없음)이어야: %d %q %q", rc, out, se)
	}
}

func TestOnlandSyntaxEvent(t *testing.T) {
	setData(t)
	dry = false
	d := DATA
	rejW(t, d+"/hc-lane", "#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"$OL_T/reports\"\n")
	if err := os.Chmod(d+"/hc-lane", 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OL_T", d)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	t.Chdir(d)
	confLines = []string{"on-land good app make dev-app"}
	onlandSyntaxTick()
	if _, err := os.Stat(d + "/reports"); err == nil {
		t.Fatal("맞는 명령에 사건을 적었다")
	}
	confLines = append(confLines, olBad)
	onlandSyntaxTick()
	onlandSyntaxTick()
	b, err := os.ReadFile(d + "/reports")
	if err != nil {
		t.Fatal(err)
	}
	rs := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(rs) != 2 || rs[0] != rs[1] || !strings.HasPrefix(rs[0], "report 막힘 - on-land app-dev 명령 구문 오류: ") ||
		!strings.Contains(rs[0], "--name on-land-app-dev --key hf:onland-syntax:app-dev:") {
		t.Fatalf("바퀴마다 같은 키의 막힘 사건(같은 키는 hc-lane이 한 번만 적는다): %q", rs)
	}
	if _, err := os.Stat(d + "/RAN"); err == nil {
		t.Fatal("검사가 명령을 돌렸다")
	}
}
