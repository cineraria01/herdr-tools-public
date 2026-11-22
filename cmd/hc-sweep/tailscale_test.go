package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	tsStopped = "Tailscale is stopped.\n"
	tsRunning = "192.0.2.3   m1   alice@   macOS   -\n"
	tsVerWarn = `Warning: client version "1.104.6-tabc" != tailscaled server version "1.104.5-tdef"` + "\n"
	// 11/21 mini-m4에서 본 꼴(이름은 가짜)
	tsNonDef = "Error: changing settings via 'tailscale up' requires mentioning all\nnon-default flags. To proceed, either re-run your command with --reset or\n" +
		"use the command below to explicitly mention the current value of\nall non-default settings:\n\n\ttailscale up --accept-routes --advertise-exit-node --hostname=worker-a\n"
)

// fakeTS: 부른 인자를 적고, 인자 줄(빈칸 이음)에 맞는 결과를 돌려준다
type fakeTS struct {
	calls []string
	res   map[string]result
}

func (f *fakeTS) run(_ time.Duration, _ []string, _ string, args ...string) result {
	k := strings.Join(args, " ")
	f.calls = append(f.calls, k)
	return f.res[k]
}

func tsCheck(t *testing.T, f *fakeTS, perHour int, state string, dry bool) ([]string, []string) {
	t.Helper()
	var sent []string
	lines := tailscaleCheck("ts", "worker-a", perHour, state, dry, f.run, func(msg, key string) bool {
		sent = append(sent, msg+"|"+key)
		return true
	})
	return lines, sent
}

func TestTailscaleRunningNoop(t *testing.T) {
	f := &fakeTS{res: map[string]result{"status": {out: tsRunning}}}
	lines, sent := tsCheck(t, f, 3, filepath.Join(t.TempDir(), "s"), false)
	if len(lines)+len(sent) != 0 || len(f.calls) != 1 {
		t.Fatalf("정상인데 무언가 함: %v %v %v", lines, sent, f.calls)
	}
}

func TestTailscaleStoppedPlainUp(t *testing.T) {
	f := &fakeTS{res: map[string]result{"status": {out: tsStopped, rc: 1}, "up": {}}}
	_, sent := tsCheck(t, f, 3, filepath.Join(t.TempDir(), "s"), false)
	if strings.Join(f.calls, ",") != "status,up" || len(sent) != 1 || !strings.Contains(sent[0], "worker-a") || !strings.Contains(sent[0], "다시 켬") {
		t.Fatalf("%v %v", f.calls, sent)
	}
}

func TestTailscaleStoppedNonDefaultFlags(t *testing.T) {
	f := &fakeTS{res: map[string]result{"status": {out: tsStopped, rc: 1}, "up": {errs: tsNonDef, rc: 1},
		"up --accept-routes --advertise-exit-node --hostname=worker-a": {}}}
	_, sent := tsCheck(t, f, 3, filepath.Join(t.TempDir(), "s"), false)
	if len(f.calls) != 3 || f.calls[2] != "up --accept-routes --advertise-exit-node --hostname=worker-a" {
		t.Fatalf("알려 준 플래그 그대로가 아님: %v", f.calls)
	}
	if len(sent) != 1 || !strings.Contains(sent[0], "알려 준 플래그 그대로") {
		t.Fatal(sent)
	}
}

func TestTailscaleUnknownFormNoGuess(t *testing.T) {
	for _, out := range []string{
		"Error: something else\n",
		strings.Replace(tsNonDef, "--hostname=worker-a", "--hostname='a b'", 1), // 인용된 값
		tsNonDef + "\ttailscale up --reset\n",                                   // 줄이 둘
		"non-default flags\n\ttailscale up --reset\n",                           // 지어내면 안 되는 플래그
	} {
		f := &fakeTS{res: map[string]result{"status": {out: tsStopped, rc: 1}, "up": {errs: out, rc: 1}}}
		_, sent := tsCheck(t, f, 3, filepath.Join(t.TempDir(), "s"), false)
		if len(f.calls) != 2 || len(sent) != 1 || !strings.Contains(sent[0], "더 손대지 않음") {
			t.Fatalf("꼴이 다른데 손댐 %q: %v %v", out, f.calls, sent)
		}
	}
}

func TestTailscaleLoginURLNotEchoed(t *testing.T) {
	f := &fakeTS{res: map[string]result{"status": {out: tsStopped, rc: 1}, "up": {out: "To authenticate, visit:\n\thttps://login.tailscale.com/a/secret\n", rc: 1}}}
	_, sent := tsCheck(t, f, 3, filepath.Join(t.TempDir(), "s"), false)
	if len(sent) != 1 || strings.Contains(sent[0], "secret") || !strings.Contains(sent[0], "로그인이 필요") {
		t.Fatal(sent)
	}
}

func TestTailscaleVersionMismatch(t *testing.T) {
	f := &fakeTS{res: map[string]result{"status": {out: tsRunning, errs: tsVerWarn}}}
	_, sent := tsCheck(t, f, 3, filepath.Join(t.TempDir(), "s"), false)
	if len(sent) != 1 || !strings.Contains(sent[0], "판이 다름(worker-a)") || !strings.HasSuffix(sent[0], "|hc-tailscale:ver:worker-a:1.104.6-tabc:1.104.5-tdef") {
		t.Fatal(sent)
	}
	if len(f.calls) != 1 {
		t.Fatal("판만 다른데 up 함", f.calls)
	}
}

func TestTailscaleHourlyLimit(t *testing.T) {
	state := filepath.Join(t.TempDir(), "d", "s")
	f := &fakeTS{res: map[string]result{"status": {out: tsStopped, rc: 1}, "up": {errs: "boom", rc: 1}}}
	for i := 0; i < 3; i++ {
		tsCheck(t, f, 2, state, false)
	}
	if n := strings.Count(strings.Join(f.calls, ","), "up"); n != 2 {
		t.Fatalf("한도 2인데 up %d번", n)
	}
	_, sent := tsCheck(t, f, 2, state, false)
	if !strings.Contains(sent[0], "한도 2번") || !strings.Contains(sent[0], "|hc-tailscale:limit:worker-a:") {
		t.Fatal(sent)
	}
	// 한 시간 지난 기록은 세지 않는다
	if !tsTake(state, 2, time.Now().Add(2*time.Hour)) {
		t.Fatal("지난 기록을 셈")
	}
}

func TestTailscaleOffAndDry(t *testing.T) {
	f := &fakeTS{res: map[string]result{"status": {out: tsStopped, rc: 1}}}
	_, sent := tsCheck(t, f, 0, filepath.Join(t.TempDir(), "s"), false)
	if len(f.calls) != 1 || len(sent) != 1 || !strings.Contains(sent[0], "알림만") {
		t.Fatal(f.calls, sent)
	}
	f.calls = nil
	lines, sent := tsCheck(t, f, 3, filepath.Join(t.TempDir(), "s"), true)
	if len(f.calls) != 1 || len(sent) != 0 || len(lines) != 1 || !strings.Contains(lines[0], "할 것") {
		t.Fatal(f.calls, sent, lines)
	}
}
