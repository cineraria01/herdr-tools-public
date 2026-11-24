//go:build !windows

package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeHcWatch: 풀이 시험의 가짜 hc — 부른 인자를 $HCFAKE/hc.log에, get은 $HCFAKE/get(없으면 실패), read는 $HCFAKE/screen
const fakeHcWatch = `#!/bin/sh
echo "$*" >>"$HCFAKE/hc.log"
case "$1" in
get) [ -f "$HCFAKE/get" ] || exit 1; cat "$HCFAKE/get" ;;
read) cat "$HCFAKE/screen" 2>/dev/null ;;
esac
exit 0
`

const trustScreen = "Accessing workspace:\n\n /x\n\n Is this a project you created or one you trust?\n\n ❯ No, exit\n   Yes, I trust this folder\n"

func hcGetJSON(addr, status string) string {
	st := "null"
	if status != "" {
		st = `"` + status + `"`
	}
	return `{"address":"` + addr + `","state":"live","status":` + st + `,"agent":"claude","name":"merge-cx","cwd":"/x","pid":1,"created_ms":1}` + "\n"
}

func (r *rfx) hcCalls(prefix string) int {
	b, _ := os.ReadFile(os.Getenv("HCFAKE") + "/hc.log")
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

// 가짜 터미널이 신뢰 질문 화면을 내면 Down·Enter가 한 번 간다(다음 바퀴에 또 보내지 않는다). lane.auto_trust off면 안 보낸다
func TestResolveWatchTrust(t *testing.T) {
	r, _ := resolveFx(t, "none", "")
	hf := os.Getenv("HCFAKE")
	os.WriteFile(hf+"/get", []byte(hcGetJSON("box:t-merge-cx", "")), 0o666)
	os.WriteFile(hf+"/screen", []byte(trustScreen), 0o666)
	os.WriteFile(r.dir+"/herdr-tools-config", []byte("#!/bin/sh\necho on\n"), 0o777)
	r.rounds(3)
	if n := r.hcCalls("keys box:t-merge-cx down enter"); n != 1 {
		b, _ := os.ReadFile(hf + "/hc.log")
		t.Fatalf("신뢰 질문 키를 한 번 보내야: %d번\n%s", n, b)
	}
}

func TestResolveWatchTrustOff(t *testing.T) {
	r, _ := resolveFx(t, "none", "")
	hf := os.Getenv("HCFAKE")
	os.WriteFile(hf+"/get", []byte(hcGetJSON("box:t-merge-cx", "blocked")), 0o666)
	os.WriteFile(hf+"/screen", []byte(trustScreen), 0o666)
	// 곁의 herdr-tools-config가 lane.auto_trust off를 돌려준다(hc-lane과 같은 길)
	os.WriteFile(r.dir+"/herdr-tools-config", []byte("#!/bin/sh\necho off\n"), 0o777)
	r.rounds(3)
	if n := r.hcCalls("keys "); n != 0 {
		t.Fatalf("lane.auto_trust off면 키를 보내지 않는다: %d번", n)
	}
}

// blocked가 손잡이 resolve_stuck분 이어지면 운영 사건(merger) 한 번 — 다음 바퀴에 또 내지 않는다, 풀리면 다시 센다
func TestResolveWatchStuck(t *testing.T) {
	r, q := resolveFx(t, "none", "")
	hf := os.Getenv("HCFAKE")
	os.WriteFile(hf+"/get", []byte(hcGetJSON("box:t-merge-cx", "blocked")), 0o666)
	r.rounds(2) // 띄움 → 처음 봄
	sf := r.clone + "/.git/hc-merger/resolve/" + strings.TrimPrefix(q, "refs/merge-queue/") + "/stuck"
	f := strings.Fields(readS(sf))
	if len(f) != 3 || f[0] != "box:t-merge-cx" || f[2] != "0" {
		t.Fatalf("처음 본 blocked를 적어야: %q", readS(sf))
	}
	if strings.Contains(r.evs(), "풀이 에이전트 멈춤") {
		t.Fatal("문턱 전에는 사건이 없다")
	}
	// 문턱(기본 3분)을 넘긴 것으로 — 처음 본 시각을 4분 앞으로
	os.WriteFile(sf, []byte("box:t-merge-cx "+strconv.FormatInt(time.Now().Unix()-240, 10)+" 0\n"), 0o666)
	r.rounds(2)
	if n := strings.Count(r.evs(), "병합 쪽 풀이 에이전트 멈춤 box:t-merge-cx — hc read box:t-merge-cx"); n != 1 {
		t.Fatalf("멈춤 사건 한 번: %d\n%s", n, r.evs())
	}
	if !strings.Contains(r.evs(), "--name|merger|--key|hc-merger-resolve-stuck:box:t-merge-cx:") {
		t.Fatalf("운영 사건 merger·키: %s", r.evs())
	}
	// 풀리면 다시 센다
	os.WriteFile(hf+"/get", []byte(hcGetJSON("box:t-merge-cx", "working")), 0o666)
	r.rounds(1)
	if fileExists(sf) {
		t.Fatal("blocked가 풀리면 센 것을 지운다")
	}
}
