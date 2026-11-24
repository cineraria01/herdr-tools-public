//go:build !windows

package main

// PR 거울 감사 1차 지적(atozu2o-mtfmab)을 막는 시험 — 고치기 전 코드로 실패한다

import (
	"os"
	"strings"
	"testing"
)

// 중간 1: gh가 시간을 넘기면 그 바퀴는 멈춘다 — 완료한 레인 넷이어도 gh 호출 1번(시간 단언 없이 호출 수로)
func TestPRSlowGhStopsRound(t *testing.T) {
	k := prSetup(t, "pr app", "pr_timeout 5")
	for i, n := range []string{"w1", "w2", "w3", "w4"} {
		if n != "w1" {
			k.row(n)
			if err := os.MkdirAll(k.dir+"/app.wt/"+n, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		k.ev("2026-11-24T01:0"+string(rune('0'+i))+":00Z", "here", n, "완료", prSHA1, "끝")
	}
	wr(t, k.dir+"/gh", "#!/bin/sh\necho \"gh $*\" >>\"$HC_T_LOG\"\nexec sleep 30\n")
	prTick()
	if k.n("gh ") != 1 || k.n("git push") != 1 {
		t.Fatalf("느린 gh 뒤에도 이어 불렀다:\n%s", k.logged())
	}
	// 다음 바퀴는 올린 커밋을 다시 올리지 않고 머리 가지부터
	wr(t, k.dir+"/gh", prTGh)
	prTick()
	if k.n("git push -q origin +"+prSHA1+":refs/herdr/pr-src/w1") != 1 {
		t.Fatalf("올린 커밋을 되풀이했다:\n%s", k.logged())
	}
}

// 낮음 2: gh가 없으면 레인 기계에 커밋을 올리지 않는다(운영 사건 한 번)
func TestPRNoGhNoPush(t *testing.T) {
	k := prSetup(t, "pr app")
	if err := os.Remove(k.dir + "/gh"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", k.dir+":/usr/bin:/bin")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	prTick()
	if k.n("git push") != 0 || k.n("--name hc-fill-pr") != 1 {
		t.Fatalf("gh 없음:\n%s", k.logged())
	}
}

// 낮음 4: 토큰 명령이 고쳐진 뒤 다시 실패하면 또 알린다
func TestPRTokenRealert(t *testing.T) {
	k := prSetup(t, "pr app", "pr_token_cmd sh -c 'test -f \"$HC_T_DIR/tokok\" && echo tok-a1'")
	prTick()
	wr(t, k.dir+"/tokok", "")
	prTick()
	if err := os.Remove(k.dir + "/tokok"); err != nil {
		t.Fatal(err)
	}
	prTick()
	if k.n("--name hc-fill-pr") != 2 {
		t.Fatalf("다시 알림:\n%s", k.logged())
	}
}

// 낮음 6: 줄인 다시 맞춤 이름도 다시 맞춤 기록(reland.tsv 넣음 줄)으로 원래 PR을 찾는다
func TestPRRelandLongName(t *testing.T) {
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	short := "w1x-reland" // 원래 이름에서 꼬리를 떼도 w1이 아님(줄인 이름 흉내)
	wr(t, DATA+"/reland.tsv", "#since\t2026-11-24T00:00:00Z\n1-w1\tw1\tmain\t넣음\t"+short+"\t2026-11-24T02:00:00Z\tc/coord\t알림\n")
	k.row(short)
	if err := os.MkdirAll(k.dir+"/app.wt/"+short, 0o755); err != nil {
		t.Fatal(err)
	}
	k.ev("2026-11-24T03:00:00Z", "here", short, "완료", prSHA2, "다시 맞춤")
	prTick()
	if k.n("pr create") != 1 || !strings.Contains(k.bodies(), "다시 맞춤 레인 "+short) {
		t.Fatalf("줄인 이름:\n%s\n%s", k.logged(), k.bodies())
	}
}

// 낮음 7: PR을 열기 전에 이미 착지했으면 열지 않는다(빈 PR 열기를 매 바퀴 되풀이하지 않게)
func TestPRLandedBeforeOpen(t *testing.T) {
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	k.ev("2026-11-24T01:30:00Z", "mg", "w1", "착지", prSHA1, "병합기 착지 — 제출 ID x")
	prTick()
	prTick()
	if k.n("gh ") != 0 || k.n("git push") != 0 {
		t.Fatalf("착지 뒤 열기:\n%s", k.logged())
	}
}
