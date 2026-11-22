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
		log, cfg                        string // 기본 브랜치 커밋 「sha 초 제목」 · 가지 설정 branch.<레인>.hcLane
		cfgBad, mfBad                   bool
		aud, done                       string // 대기열 감사 칸(빈 값 = auto) · 완료 sha(빈 값 = sha)
		want                            string // "" = 알리지 않음
	}{
		// 2026-11-23 stall-no-sha: 감사·조사 레인의 「완료 -」는 감사할 것도 착지할 것도 없다 · 자동 감사 끈 레인은 착지만 기다린다
		{name: "자동 감사 끈 레인 — 착지 없음", lane: "w11", aud: "no-audit",
			want: "w11 완료 뒤 멈춤 — 120분째 착지 없음 (m/w11, 완료 e4e507b4) --name w11"},
		{name: "커밋 없는 완료 — 완료 -", lane: "w10", done: "-"}, // 이름에 괄호를 넣지 않는다(임시 폴더 경로가 가짜 hc-lane 셸 줄에 그대로 든다)
		// 2026-11-22 지휘(23:20): 병합기·이름 바꿔 낸 제출로 착지한 레인에도 경보 — 기본 브랜치의 접수 뒤 커밋 제목 머리 「<이름>:」도 착지
		{name: "커밋 제목으로 착지", lane: "clean-renamed-submit",
			log: fmt.Sprintf("%s %d clean-renamed-submit: 이름을 바꿔 낸 제출\n", cand, after)},
		{name: "다시 맞춤 레인이 원래 이름으로 착지", lane: "w6-reland",
			reland: "1795349822-w6\tw6\tw6\t넣음\tw6-reland\t2026-11-22T13:00:00Z\n",
			log:    fmt.Sprintf("%s %d w6: 다시 맞춤\n", cand, after)},
		{name: "가지 설정의 낸 이름으로 착지", lane: "w7-fix", cfg: "w7",
			hcl: fmt.Sprintf("%s\trefs/hc-landed/w7/%d\n", cand, after)},
		{name: "옛 같은 이름 커밋·남의 이름 머리는 착지 아님", lane: "w8",
			log:  fmt.Sprintf("%s %d w8: 옛 착지\n%s %d w8-other: 남의 레인\n", cand, before, cand, after),
			want: "w8 완료 뒤 멈춤 — 120분째 감사·착지 없음 (m/w8, 완료 e4e507b4) --name w8"},
		{name: "가지 설정 못 읽음", lane: "w9", cfgBad: true,
			want: "w9 완료 뒤 멈춤 — 120분째 감사·착지 없음 (m/w9, 완료 e4e507b4) (착지 판정 못 함) --name w9"},
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
			wr("log.txt", c.log)
			wr("cfg.txt", c.cfg)
			mfrc, cfgrc := "0", "1" // 진짜 git config --get: 없으면 종료 1
			if c.mfBad {
				mfrc = "128"
			}
			if c.cfg != "" {
				cfgrc = "0"
			}
			if c.cfgBad {
				cfgrc = "128"
			}
			// 가짜 git: 기본 origin/main. 완료 sha는 클론에 있되 기본 브랜치에 없고(rebase한 병합기 착지), 착지 후보는 기본 브랜치에 듦
			wr("git", "#!/bin/sh\nd='"+DATA+"'\ncase \" $* \" in\n"+
				"*\" symbolic-ref \"*) echo origin/main;;\n"+
				"*\" merge-base \"*) case \" $* \" in *\" "+cand+" \"*) exit 0;; esac; exit 1;;\n"+
				"*\" ls-remote \"*refs/hc-landed/*) cat \"$d/hcl.txt\";;\n"+
				// 큐는 진짜 git처럼 ls-remote --exit-code에 맞는 참조가 없으면 종료 2(빈 출력에 0이면 gitx.LsRemote가 모름으로 읽는다)
				"*\" ls-remote \"*refs/merge-queue/*) cat \"$d/mq.txt\"; [ -s \"$d/mq.txt\" ] || exit 2;;\n"+
				"*\" ls-remote \"*refs/merge-failed/*) cat \"$d/mf.txt\"; exit "+mfrc+";;\n"+
				"*\" log \"*) cat \"$d/log.txt\";;\n"+
				"*\" config \"*\"branch."+n+".hcLane\"*) cat \"$d/cfg.txt\"; exit "+cfgrc+";;\n"+
				"esac\nexit 0\n")
			t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
			if c.reland != "" {
				wr("reland.tsv", c.reland)
			}
			aud, ds := c.aud, c.done
			if aud == "" {
				aud = "auto"
			}
			if ds == "" {
				ds = sha
			}
			wr("queue.tsv", strings.Join([]string{n, "app", "b", "-", "m", "up", "m", "m/" + n, "-", "t", "-", "-", "normal", "-", aud, "-", "r1"}, "\t")+"\n")
			wr("try.tsv", n+"\tr1\tm\t-\t-\tm/"+n+"\t"+n+"@1700000000000\n")
			EV = []string{auTEv(done.UTC().Format("2006-01-02T15:04:05Z"), "m/"+n, n, "완료", ds, "끝", n+"@1700000000000")}
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
