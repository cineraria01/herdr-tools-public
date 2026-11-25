//go:build !windows

package main

// ht-fill-tell(2026-11-26): 지휘가 「hc ls | 이름 찾기 | hc send」로 같은 이름의 앞 run(끝난 터미널)에 보냈다. tell은 대기열 기록의
// 지금 run 주소에만 보낸다 — 같은 이름 터미널이 둘이어도 기록된 주소 하나, run이 없으면 보내지 않고 종료 1

import (
	"os"
	"strings"
	"testing"
)

// tellKit: 가짜 hc(get <주소>는 get-<주소>.json — 없으면 not_found 종료 3, ls는 같은 이름 둘, send는 sent에 덧붙임)
func tellKit(t *testing.T, queue string) (fillKit, string) {
	k := ehKit(t)
	f := k.home + "/fake"
	if err := os.MkdirAll(f, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := k.bin[:strings.LastIndexByte(k.bin, '/')]
	ehPut(t, bin+"/hc", `#!/bin/sh
F=`+f+`
case "$1" in
ls) echo '{"terminals":[{"address":"old/t1","name":"lane-a"},{"address":"t2","name":"lane-a"}]}'; exit 0 ;;
get) if [ -f "$F/get-$(echo "$2" | tr / _).json" ]; then cat "$F/get-$(echo "$2" | tr / _).json"; exit 0; fi
     echo '{"error":{"kind":"not_found","message":"없음"}}' >&2; exit 3 ;;
send) printf '%s|%s\n' "$2" "$3" >> "$F/sent"; exit 0 ;;
esac
exit 1
`, 0o755)
	term := func(addr, name, status string) {
		ehPut(t, f+"/get-"+strings.ReplaceAll(addr, "/", "_")+".json", `{"address":"`+addr+`","state":"live","status":"`+status+
			`","cwd":"/w","name":"`+name+`","created_ms":1700000000000,"argv":["claude"],"foreground":{"argv":["claude"],"pid":5}}`, 0o644)
	}
	term("old/t1", "lane-a", "done")  // 앞 run(주차 뒤 끝난 터미널) — 같은 이름
	term("t2", "lane-a", "working")   // 지금 run
	term("t3", "audit-x1", "working") // 지금 감사
	ehPut(t, k.data+"/queue.tsv", queue, 0o644)
	return k, f
}

const tellQ = "lane-a\tapp\t/b.md\t-\t-\tup\there\tt2\t-\t1\t-\t-\tnormal\t-\tauto\t-\tr1\n" +
	"audit-x1\tapp\t/a.md\t-\there\tup\there\tt3\t-\t1\t-\t-\turgent\tno-early\taudit=lane-a\t-\tr2\n"

func TestTellCurrentRunOnly(t *testing.T) {
	k, f := tellKit(t, tellQ)
	out, se, rc := k.run(t, "tell", "lane-a", "감사 지적 1을 고쳐라")
	if rc != 0 || !strings.Contains(out, "보냄 lane-a → t2") {
		t.Fatalf("tell: 종료 0이어야 — %d %q %q", rc, out, se)
	}
	if s := nudgeSent(f); len(s) != 1 || s[0] != "t2|감사 지적 1을 고쳐라" {
		t.Fatalf("지금 run(t2)에만 보내야: %q", s)
	}
	// --audit: 그 레인의 지금 감사 줄 터미널로
	if out, se, rc := k.run(t, "tell", "--audit", "lane-a", "범위는 cmd/만"); rc != 0 || !strings.Contains(out, "보냄 audit-x1 → t3") {
		t.Fatalf("tell --audit: %d %q %q", rc, out, se)
	}
	if s := nudgeSent(f); len(s) != 2 || s[1] != "t3|범위는 cmd/만" {
		t.Fatalf("감사 터미널(t3)에 보내야: %q", s)
	}
}

func TestTellRefuses(t *testing.T) {
	for _, c := range []struct{ name, queue, want string }{
		{"run 없음(대기)", strings.Replace(tellQ, "\tup\there\tt2\t", "\twait\t-\t-\t", 1), "떠 있는 레인이 아니다"},
		{"터미널 끝남", strings.Replace(tellQ, "\tt2\t", "\tt9\t", 1), "가 없다"},
		{"주소 재사용", strings.Replace(tellQ, "\tt2\t", "\tt3\t", 1), "주소 재사용"},
		{"대기열에 없음", "", "대기열에 없다"},
	} {
		t.Run(c.name, func(t *testing.T) {
			k, f := tellKit(t, c.queue)
			if _, se, rc := k.run(t, "tell", "lane-a", "글"); rc != 1 || !strings.Contains(se, c.want) || len(nudgeSent(f)) != 0 {
				t.Fatalf("종료 1·「%s」·안 보냄이어야 — %d %q %q", c.want, rc, se, nudgeSent(f))
			}
		})
	}
	k, f := tellKit(t, strings.SplitAfter(tellQ, "\n")[0])
	if _, se, rc := k.run(t, "tell", "--audit", "lane-a", "글"); rc != 1 || !strings.Contains(se, "도는 감사가 없다") || len(nudgeSent(f)) != 0 {
		t.Fatalf("감사 없음: 종료 1·안 보냄이어야 — %d %q %q", rc, se, nudgeSent(f))
	}
	if _, _, rc := k.run(t, "tell", "lane-a"); rc != 2 {
		t.Fatalf("글 없음: 종료 2여야 — %d", rc)
	}
}
