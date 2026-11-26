package main

// ht-advise-stage-lookup(2026-11-26): advise가 일하는 선행을 「단계 모름(떠 있는 레인에 없음)」으로 보였다 — 사슬 머리는 선행 여럿 가운데
// 첫째(이미 착지해 대기열에서 빠진 것)의 단계만 찾았고, 선행 단계는 상태 한 장의 떠 있는 레인(추린 것)에서만 찾았고, 나이를 모르면 「모름째」.
// 선행 단계는 대기열 전체(주인·상태 상관없이)에서, 대기열에 없으면 이 기계 저장소 기본 가지의 착지에서 찾는다

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/opsstate"
)

func TestAdvisePredStage(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	advFx(t)
	repo := t.TempDir()
	gitT(t, repo, nil, "init", "-q", "-b", "main")
	gitT(t, repo, []string{"GIT_COMMITTER_DATE=" + time.Now().Add(-2*time.Hour).Format(time.RFC3339)}, "commit", "-q", "--allow-empty", "-m", "lane-z: 끝")
	sha8 := gitT(t, repo, nil, "rev-parse", "HEAD")[:8]
	gitT(t, repo, []string{"GIT_COMMITTER_DATE=" + time.Now().Add(-100*time.Minute).Format(time.RFC3339)}, "commit", "-q", "--allow-empty", "-m", "lane-y: 끝")
	ysha8 := gitT(t, repo, nil, "rev-parse", "HEAD")[:8]
	// 다른 프로젝트(web) 저장소: lane-p가 그 main에 착지. 이 프로젝트(kb) main에는 없다
	web := t.TempDir()
	gitT(t, web, nil, "init", "-q", "-b", "main")
	gitT(t, web, []string{"GIT_COMMITTER_DATE=" + time.Now().Add(-90*time.Minute).Format(time.RFC3339)}, "commit", "-q", "--allow-empty", "-m", "lane-p: 끝")
	psha8 := gitT(t, web, nil, "rev-parse", "HEAD")[:8]
	confLines = append(confLines, "repo kb m "+repo, "repo kb w "+repo, "repo web m "+web)

	now := time.Now()
	add := now.Add(-3 * time.Hour).Format("2006-01-02T15:04")
	q, _ := os.ReadFile(DATA + "/queue.tsv")
	row := func(c ...string) string { return strings.Join(c, "\t") + "\n" }
	os.WriteFile(DATA+"/queue.tsv", []byte(string(q)+
		// 다른 지휘(x/t9)가 넣은 일하는 선행 · 주차에서 이어 띄운 선행(보고 없음 — 나이 모름)
		row("lane-o", "kb", "b", "-", "-", "up", "x", "x/t3", "-", add, "-", "x/t9", "normal", "-", "-", "-", "r7")+
		row("lane-r", "kb", "b", "-", "-", "up", "w", "w/t6", "-", add, "-", "m/t1", "normal", "resumed", "-", "-", "r8")+
		// 다른 프로젝트 web 대기열에서 일하는 선행 lane-q(뒤에 온 같은 이름 kb 줄은 터미널이 끝남 — 이름만으로 찾으면 섞인다)
		row("lane-q", "web", "b", "-", "-", "up", "x", "x/t4", "-", add, "-", "x/t9", "normal", "-", "-", "-", "r11")+
		row("lane-q", "kb", "b", "-", "-", "up", "w", "w/t7", "-", add, "-", "m/t1", "normal", "-", "-", "-", "r12")+
		// 착지 뒤 정리됨으로 끝나 남은 up 줄 lane-y(감사 atp306v 중간 1) · 착지 없이 끝난 up 줄 lane-d
		row("lane-y", "kb", "b", "-", "-", "up", "w", "w/t8", "-", add, "-", "m/t1", "normal", "-", "-", "-", "r13")+
		row("lane-d", "kb", "b", "-", "-", "up", "w", "w/t9", "-", add, "-", "m/t1", "normal", "-", "-", "-", "r14")+
		// 같은 프로젝트 선행 lane-s(kb 줄은 되돌려줌) — 뒤에 온 web의 같은 이름 up 줄과 섞지 않는다(감사 atp306v 낮음 1)
		row("lane-s", "kb", "b", "-", "-", "sent", "-", "-", "-", add, "-", "m/t1", "normal", "-", "-", "-", "r15")+
		row("lane-s", "web", "b", "-", "-", "up", "x", "x/t2", "-", add, "-", "x/t9", "normal", "-", "-", "-", "r16")+
		// 사슬 머리 lane-w(선행 — 첫째는 착지해 대기열에 없음, 다른 프로젝트 꼴 둘 등), 그 뒤 lane-v
		row("lane-w", "kb", "b", "lane-z,lane-o,lane-r,lane-gone,lane-p@web,lane-q@web,lane-y,lane-d,lane-s", "-", "wait", "-", "-", "-", add, "-", "m/t1", "normal", "-", "-", "-", "r9")+
		row("lane-v", "kb", "b", "lane-w", "-", "wait", "-", "-", "-", add, "-", "m/t1", "normal", "-", "-", "-", "r10")), 0o644)
	os.WriteFile(DATA+"/lsjson", []byte(`{"terminals":[{"address":"w/t5","name":"lane-a","status":"working"},`+
		`{"address":"x/t3","name":"lane-o","status":"working"},{"address":"w/t6","name":"lane-r","status":"working"},`+
		`{"address":"x/t4","name":"lane-q","status":"working"},{"address":"x/t2","name":"lane-s","status":"working"},`+
		`{"address":"t2","name":"`+watchName("m/t1")+`","status":"idle"}]}`), 0o644)
	l, _ := os.ReadFile(DATA + "/last.tsv")
	os.WriteFile(DATA+"/last.tsv", []byte(string(l)+"lane-o\t중간\t-\t"+now.Add(-30*time.Minute).Format("15:04")+"\t반쯤\t-\tfresh\tr7\n"+
		"lane-q\t중간\t-\t"+now.Add(-20*time.Minute).Format("15:04")+"\t반쯤\t-\tfresh\tr11\n"+
		"lane-y\t정리됨\t-\t"+now.Add(-time.Hour).Format("15:04")+"\t정리\t-\tfresh\tr13\n"+
		"lane-d\t정리됨\t-\t"+now.Add(-time.Hour).Format("15:04")+"\t정리\t-\tfresh\tr14\n"), 0o644)

	FR, FB = "keep-fr", "keep-fb" // 바퀴가 쓰는 저장소·가지는 그대로(참고 ht-advise-chain-known 시험)
	by := map[string]opsstate.Item{}
	for _, it := range adviseNow(now).Items {
		by[it.Key] = it
	}
	if FR != "keep-fr" || FB != "keep-fb" {
		t.Errorf("FR·FB가 바뀜: %q %q", FR, FB)
	}
	c, ok := by["chain:lane-w"]
	if !ok {
		t.Fatalf("사슬 항목 없음: %+v", by)
	}
	w, ok := by["free-wait:lane-w"]
	if !ok {
		t.Fatalf("선행 대기 항목 없음: %+v", by)
	}
	for _, ev := range []string{c.Evidence, w.Evidence} {
		for _, want := range []string{
			"lane-z 착지 " + sha8 + " 2시간",   // 착지한 선행 → 착지 sha8 나이 전
			"lane-o 작업 중(working) 30분째(x)", // 다른 지휘 주인의 일하는 선행(기계 함께)
			"lane-r 작업 중(working)(w)",      // 주차에서 이어 띄운 선행(나이 모름 — 단계만)
			"lane-gone 대기열에 없음",
			"lane-p@web 착지 " + psha8 + " 1시간30분 전", // 다른 프로젝트 main에 착지
			"lane-q@web 작업 중(working) 20분째(x)",     // 다른 프로젝트 대기열에서 일하는 중
			"lane-y 착지 " + ysha8 + " 1시간40분 전",     // 착지 뒤 정리됨으로 끝난 up 줄 → 착지 표시
			"lane-d 끝남 1시간00분째(w)",                 // 착지 없이 끝난 up 줄 → 끝남 그대로
			"lane-s 되돌려줌 3시간00분째",                  // 같은 프로젝트 줄(다른 프로젝트 같은 이름과 안 섞임)
		} {
			if !strings.Contains(ev, want) {
				t.Errorf("근거에 %q 없음:\n%s", want, ev)
			}
		}
		if !strings.Contains(ev, "전") || strings.Contains(ev, "모름째") || strings.Contains(ev, "떠 있는 레인에 없음") {
			t.Errorf("착지 나이 「전」·모름째 없음·옛 문구 없음:\n%s", ev)
		}
	}
}
