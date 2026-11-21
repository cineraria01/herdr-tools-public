package main

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
	"time"
)

// 소비자가 바로 실패하면 생산자를 끝까지 기다리지 않고 끈다 — 기다리는 동안 동기화 잠금을 쥔다(감사 A08)
func TestPipelineConsumerFails(t *testing.T) {
	d := t.TempDir()
	prod := exec.Command("sh", "-c", `i=0; while [ $i -lt 30 ]; do echo x; sleep 0.1; i=$((i+1)); done; : > "$1/done"`, "_", d)
	cons := exec.Command("sh", "-c", "exit 1")
	start := time.Now()
	p, c := pipeline([]byte("pre\n"), prod, cons, nil)
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("소비자 실패 뒤 %v 기다렸다", took)
	}
	if c != 1 || p == 0 {
		t.Errorf("종료 코드 생산자 %d 소비자 %d", p, c)
	}
	time.Sleep(3500 * time.Millisecond)
	if _, err := os.Stat(d + "/done"); err == nil {
		t.Errorf("생산자가 끝까지 돌았다")
	}
}

// 조각(agent.sh): 일반 파일·나무 파일을 못 읽으면 「없음(-)」으로 답하지 않고 실패로 끝난다(감사 B18·B08)
func TestAgentUnreadable(t *testing.T) {
	h := t.TempDir()
	os.MkdirAll(h+"/tr/sub", 0o755)
	for _, p := range []string{h + "/a", h + "/tr/sub/x"} {
		if err := os.WriteFile(p, []byte("v\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	q := func(args ...string) (string, error) {
		c := exec.Command("sh", append([]string{"-c", agent, "_"}, args...)...)
		c.Env = append(os.Environ(), "HOME="+h, "HSB=-")
		out, err := c.Output()
		return string(out), err
	}
	if out, err := q("q", "a", "--", "--", "tr"); err != nil || !bytes.Contains([]byte(out), []byte("F\ttr/sub/x\t")) {
		t.Fatalf("정상 조회: %v %q", err, out)
	}
	os.Chmod(h+"/a", 0)
	if out, err := q("q", "a", "--", "--"); err == nil {
		t.Errorf("못 읽는 파일인데 0으로 끝남: %q", out)
	}
	os.Chmod(h+"/a", 0o644)
	os.Chmod(h+"/tr/sub/x", 0)
	if out, err := q("q", "--", "--", "tr"); err == nil {
		t.Errorf("못 읽는 나무 파일인데 0으로 끝남: %q", out)
	}
	if out, err := q("t", h+"/tr", "tr", "/dev/null"); err == nil {
		t.Errorf("나무 걷기(t)가 0으로 끝남: %q", out)
	}
	os.Chmod(h+"/tr/sub/x", 0o644)
}

// 잠금 fd의 파일이 경로에서 지워졌거나 바뀌었으면 같은 파일이 아니다 — 예전 셸 판 lockf가 끝나며 잠금 파일을 지운 꼴(감사 A07)
func TestSameFile(t *testing.T) {
	p := t.TempDir() + "/lock"
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if !sameFile(f, p) {
		t.Fatal("같은 파일인데 거짓")
	}
	os.Remove(p)
	if sameFile(f, p) {
		t.Error("지워진 뒤에도 참")
	}
	os.WriteFile(p, nil, 0o644)
	if sameFile(f, p) {
		t.Error("새로 만든 파일과도 참")
	}
}

// 정상: 앞부분 + 생산자 출력이 순서대로 소비자에게 가고 둘 다 0
func TestPipelineOrder(t *testing.T) {
	var out bytes.Buffer
	p, c := pipeline([]byte("a\n"), exec.Command("printf", "b\\nc\\n"), exec.Command("cat"), &out)
	if p != 0 || c != 0 || out.String() != "a\nb\nc\n" {
		t.Errorf("p %d c %d 출력 %q", p, c, out.String())
	}
	// 앞부분이 파이프 버퍼(64KB)보다 커도 막히지 않는다
	big := bytes.Repeat([]byte("y\n"), 200000)
	out.Reset()
	p, c = pipeline(big, exec.Command("true"), exec.Command("wc", "-l"), &out)
	if p != 0 || c != 0 || bytes.TrimSpace(out.Bytes()) == nil || string(bytes.TrimSpace(out.Bytes())) != "200000" {
		t.Errorf("큰 앞부분: p %d c %d %q", p, c, out.String())
	}
}
