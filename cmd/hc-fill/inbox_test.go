package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// 원격 sh 본문이 셸 판(lib/hc-fill/tick.sh)이 실제로 내는 글과 같다(골든 = 셸 판 함수의 출력)
func TestInboxBodiesMatchShell(t *testing.T) {
	for fn, want := range map[string]string{"s_ilist": inbSIlist, "s_iclaim": inbSIclaim, "s_ipub": inbSIpub} {
		out, err := exec.Command("bash", "-c", `. ../../lib/hc-fill/tick.sh && "$1"`, "_", fn).Output()
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if string(out) != want {
			t.Errorf("%s 본문이 셸 판과 다르다\n셸: %q\nGo: %q", fn, out, want)
		}
	}
}

// 받은편지 회신 단일성: 같은 rid·같은 내용은 SENT 뒤 ALREADY, 다른 rid의 다른 내용은 EXISTS(덮지 않음), 크기·해시가 틀리면 BADINPUT
func TestInboxPublishOnce(t *testing.T) {
	if _, err := exec.LookPath("lockf"); err != nil {
		t.Skip("lockf 없음(맥이 아님)")
	}
	old := LOCAL
	LOCAL = "here"
	t.Cleanup(func() { LOCAL = old })
	d := t.TempDir()
	pub := func(rid, body, h, z string) string {
		t.Helper()
		out, rc := onm(strings.NewReader(body), LOCAL, inbSIpub, d, "inq", rid, h, z)
		if rc != 0 {
			t.Fatalf("s_ipub 종료 %d: %q", rc, out)
		}
		return sub(out)
	}
	ok := func(body string) (string, string) { return sha256hex([]byte(body)), strconv.Itoa(len(body)) }
	a := "결과: 고침 abcd1234\n"
	h, z := ok(a)
	if got := pub("r1", a, h, z); got != "SENT" {
		t.Fatalf("첫 공개 = %q, SENT여야", got)
	}
	if got := pub("r1", a, h, z); got != "ALREADY" {
		t.Fatalf("같은 rid 다시 = %q, ALREADY여야", got)
	}
	b := "결과: 실패 다른 요청\n"
	hb, zb := ok(b)
	if got := pub("r2", b, hb, zb); got != "EXISTS" {
		t.Fatalf("다른 rid·다른 내용 = %q, EXISTS여야", got)
	}
	if got, _ := os.ReadFile(filepath.Join(d, "done", "inq.md")); string(got) != a {
		t.Fatalf("덮어썼다: %q", got)
	}
	if got := pub("r3", b, hb, "1"); got != "BADINPUT" {
		t.Fatalf("크기 틀림 = %q, BADINPUT이어야", got)
	}
	if got := pub("r4", b, h, zb); got != "BADINPUT" {
		t.Fatalf("해시 틀림 = %q, BADINPUT이어야", got)
	}
}

// irej 본문: 사유를 먼저 쓰고 .req·.brief.md를 옮긴다(옮겨진 요청에 사유가 늘 함께)
func TestInboxRejectWritesWhy(t *testing.T) {
	old := LOCAL
	LOCAL = "here"
	t.Cleanup(func() { LOCAL = old })
	d := t.TempDir()
	for _, f := range []string{"x.req", "x.brief.md"} {
		if err := os.WriteFile(filepath.Join(d, f), []byte("a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, rc := onm(nil, LOCAL, inbIrejSh, d, "x", "사유 하나"); rc != 0 {
		t.Fatalf("irej 종료 %d: %q", rc, out)
	}
	why, _ := os.ReadFile(filepath.Join(d, "rejected", "x.why"))
	if string(why) != "사유 하나\n" || !isFile(filepath.Join(d, "rejected", "x.req")) || !isFile(filepath.Join(d, "rejected", "x.brief.md")) || exists(filepath.Join(d, "x.req")) {
		t.Fatalf("거절 꼴이 틀림: why=%q", why)
	}
	// 사유를 못 쓰면 요청을 옮기지 않는다(차례 — 아스트라 F18: 최종 파일만 봐서는 옛 차례 「옮긴 뒤 사유」를 못 가렸다)
	for _, f := range []string{"y.req", "y.brief.md"} {
		if err := os.WriteFile(filepath.Join(d, f), []byte("a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(d, "rejected", "y.why.tmp"), 0o755); err != nil { // 사유 임시 파일 자리를 폴더로 막는다
		t.Fatal(err)
	}
	if _, rc := onm(nil, LOCAL, inbIrejSh, d, "y", "사유 둘"); rc == 0 {
		t.Fatal("사유를 못 썼는데 종료 0")
	}
	if !isFile(filepath.Join(d, "y.req")) || !isFile(filepath.Join(d, "y.brief.md")) || exists(filepath.Join(d, "rejected", "y.req")) {
		t.Fatal("사유를 못 썼는데 요청을 옮겼다")
	}
}
