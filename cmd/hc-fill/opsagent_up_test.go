package main

// ht-alert-sources(2026-11-28): 운영 에이전트 못 띄움 막힘에 열쇠, 다시 뜨면 같은 열쇠 + :up 풀림 한 번(한 문이 그 막힘을 풀림으로 본다).
// 풀림 쓰기가 실패하면 다음 바퀴에 다시 한 번

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

var oaKeyRe = regexp.MustCompile(`--key (\S+)`)

func TestOpsAgentLaunchFailUp(t *testing.T) {
	orFx(t)
	if err := os.WriteFile(DATA+"/terms.tsv", []byte("t1\thc-fill-agent\tdone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 0; i < 3; i++ {
		opsAgentTick(now.Add(time.Duration(i) * time.Minute))
	}
	var blk string
	for _, l := range reports(t) {
		if strings.Contains(l, "운영 에이전트를 못 띄움") {
			blk = l
		}
	}
	m := oaKeyRe.FindStringSubmatch(blk)
	if m == nil || !strings.HasPrefix(m[1], "hf:agent:launch-fail:") {
		t.Fatalf("못 띄움 막힘에 열쇠: %q", blk)
	}
	if err := os.WriteFile(DATA+"/terms.tsv", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	opsAgentTick(now.Add(5 * time.Minute))
	var up []string
	for _, l := range reports(t) {
		if strings.Contains(l, "--key "+m[1]+":up") {
			up = append(up, l)
		}
	}
	if len(up) != 1 || !strings.HasPrefix(up[0], "report 중간 ") {
		t.Fatalf("다시 뜨면 같은 열쇠 + :up 중간 한 번: %q", reports(t))
	}
	if st, _ := oaLoad(); st.FailKey != "" {
		t.Errorf("풀림을 적은 뒤 열쇠를 비움: %+v", st)
	}
}

// 감사 atp7ncn 낮음 2: 다시 뜬 뒤 풀림을 못 적은 채 못 띄움이 다시 문턱에 닿으면, 새 막힘보다 옛 열쇠의 :up을 먼저 적는다
func TestOpsAgentLaunchFailUpBeforeNew(t *testing.T) {
	orFx(t)
	if err := os.WriteFile(DATA+"/terms.tsv", []byte("t1\thc-fill-agent\tdone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, _ := oaLoad()
	st.FailKey = "hf:agent:launch-fail:1700" // 앞 흐름에서 뜬 뒤 풀림을 못 적은 채 남은 열쇠
	oaSave(st)
	now := time.Now()
	for i := 0; i < 3; i++ {
		opsAgentTick(now.Add(time.Duration(i) * time.Minute))
	}
	var keys []string
	for _, l := range reports(t) {
		if m := oaKeyRe.FindStringSubmatch(l); m != nil {
			keys = append(keys, m[1])
		}
	}
	if len(keys) != 2 || keys[0] != "hf:agent:launch-fail:1700:up" || !strings.HasPrefix(keys[1], "hf:agent:launch-fail:") || keys[1] == "hf:agent:launch-fail:1700" {
		t.Fatalf("옛 풀림 먼저, 그다음 새 막힘: %q", keys)
	}
}

// 반대 장면: 못 띄움이 알림 문턱 전에 풀림(막힘을 안 냄) → 풀림도 없음
func TestOpsAgentLaunchFailUpNoBlock(t *testing.T) {
	orFx(t)
	if err := os.WriteFile(DATA+"/terms.tsv", []byte("t1\thc-fill-agent\tdone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	opsAgentTick(now)
	if err := os.WriteFile(DATA+"/terms.tsv", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	opsAgentTick(now.Add(time.Minute))
	for _, l := range reports(t) {
		if strings.Contains(l, ":up") || strings.Contains(l, "못 띄움") {
			t.Fatalf("막힘 없이 풀림 사건: %q", l)
		}
	}
}
