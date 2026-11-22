package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// 완료 뒤 멈춤 헛알림(2026-11-22 stall-false-alarm): 병합기가 맡은 레인은 알리지 않는다 — 실제 두 꼴(묶음 착지·반송 뒤 이어받음)과
// 큐에 있음, 진짜 멈춤(옛 같은 이름 착지 표식만 있음)은 그대로, 원격을 못 읽으면 「(착지 판정 못 함)」을 붙여 알림
func TestStallMergerHandled(t *testing.T) {
	sha := "e4e507b4" + strings.Repeat("0", 32) // 가짜 SHA는 실행 중에(비밀 꼴 검사)
	cand := strings.Repeat("7", 40)
	done := time.Now().Add(-2 * time.Hour)
	after, before := done.Add(time.Hour).Unix(), done.Add(-24*time.Hour).Unix()
	for _, c := range []struct {
		name, lane, hcl, mq, mf, reland string
		mfBad                           bool
		want                            string // "" = 알리지 않음
	}{
		{name: "묶음 착지 d1-inbox-claim", lane: "d1-inbox-claim",
			hcl: fmt.Sprintf("%s\trefs/hc-landed/d1-inbox-claim/%d\n", cand, after)},
		{name: "반송 뒤 다시 맞춤이 이어받음 sv-shell-remove-reland", lane: "sv-shell-remove-reland",
			mf:     sha + "\trefs/merge-failed/1795349822-sv-shell-remove\n" + strings.Repeat("5", 40) + "\trefs/merge-failed/1795343585-sv-shell-remove\n",
			reland: "#since\t2026-11-22T00:00:00Z\n1795349822-sv-shell-remove\tsv-shell-remove-reland\tsv-shell-remove\t넣음\tsv-shell-remove-reland2\t2026-11-22T13:00:00Z\n"},
		{name: "원래 레인에 보냄", lane: "w1",
			mf:     sha + "\trefs/merge-failed/1795349822-w1\n",
			reland: "1795349822-w1\tw1\tw1\t보냄\tm/w1\t2026-11-22T13:00:00Z\n"},
		{name: "병합기 줄에 있음", lane: "w2", mq: strings.Repeat("6", 40) + "\trefs/merge-queue/1795349900-w2\n"},
		{name: "다른 이름 큐 항목이 그 sha", lane: "w3", mq: sha + "\trefs/merge-queue/1795349900-other\n"},
		{name: "진짜 멈춤 — 옛 같은 이름 착지 표식·반송만, 이어받은 기록 없음", lane: "w4",
			hcl:  fmt.Sprintf("%s\trefs/hc-landed/w4/%d\n", cand, before),
			mf:   sha + "\trefs/merge-failed/1795349822-w4\n",
			want: "w4 완료 뒤 멈춤 — 120분째 감사·착지 없음 (m/w4, 완료 e4e507b4) --name w4"},
		{name: "원격 못 읽음", lane: "w5", mfBad: true,
			want: "w5 완료 뒤 멈춤 — 120분째 감사·착지 없음 (m/w5, 완료 e4e507b4) (착지 판정 못 함) --name w5"},
	} {
		t.Run(c.name, func(t *testing.T) {
			DATA = t.TempDir()
			Q, AT, LOCAL, dry, EVBAD, CONN = DATA+"/queue.tsv", DATA+"/audit.tsv", "m", false, false, nil
			confLines = []string{"done_stall 60", "repo app m " + DATA}
			FETCHED, FETCHBAD, MQP, MQS, MQBAD = "", "", "", nil, ""
			n := c.lane
			TERMS = []Term{{M: "m", Addr: "m/" + n, Name: n}}
			wr := func(f, s string) {
				if err := os.WriteFile(DATA+"/"+f, []byte(s), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			log := DATA + "/hc-lane.log"
			wr("hc-lane", "#!/bin/sh\nprintf '%s\\n' \"$*\" >>"+log+"\n")
			wr("hcl.txt", c.hcl)
			wr("mq.txt", c.mq)
			wr("mf.txt", c.mf)
			mfrc := "0"
			if c.mfBad {
				mfrc = "128"
			}
			// 가짜 git: 기본 origin/main. 완료 sha는 클론에 있되 기본 브랜치에 없고(rebase한 병합기 착지), 착지 후보는 기본 브랜치에 듦
			wr("git", "#!/bin/sh\nd='"+DATA+"'\ncase \" $* \" in\n"+
				"*\" symbolic-ref \"*) echo origin/main;;\n"+
				"*\" merge-base \"*) case \" $* \" in *\" "+cand+" \"*) exit 0;; esac; exit 1;;\n"+
				"*\" ls-remote \"*refs/hc-landed/*) cat \"$d/hcl.txt\";;\n"+
				"*\" ls-remote \"*refs/merge-queue/*) cat \"$d/mq.txt\";;\n"+
				"*\" ls-remote \"*refs/merge-failed/*) cat \"$d/mf.txt\"; exit "+mfrc+";;\n"+
				"esac\nexit 0\n")
			t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
			if c.reland != "" {
				wr("reland.tsv", c.reland)
			}
			wr("queue.tsv", strings.Join([]string{n, "app", "b", "-", "m", "up", "m", "m/" + n, "-", "t", "-", "-", "normal", "-", "-", "-", "r1"}, "\t")+"\n")
			wr("try.tsv", n+"\tr1\tm\t-\t-\tm/"+n+"\t"+n+"@1700000000000\n")
			EV = []string{auTEv(done.UTC().Format("2006-01-02T15:04:05Z"), "m/"+n, n, "완료", sha, "끝", n+"@1700000000000")}
			stallTick()
			b, err := os.ReadFile(log)
			if c.want == "" {
				if err == nil {
					t.Fatalf("병합기가 맡은 레인을 알렸다: %q", b)
				}
				return
			}
			if err != nil {
				t.Fatalf("진짜 멈춤을 안 알렸다: %v", err)
			}
			if !strings.Contains(string(b), "report 막힘 - "+c.want) {
				t.Fatalf("알림 꼴: %q", b)
			}
		})
	}
}
