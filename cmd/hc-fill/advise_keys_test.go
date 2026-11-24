package main

// advise-fixes(2026-11-24): ④ 재료 모름 한 바퀴에 알림 열쇠가 지워져 회복 뒤 같은 사건이 다시 나감 ① 「감시 안 돎」 — 추린 상태에서도,
// 지휘가 사라진 흔적·착지한 줄뿐인 지휘는 빼고 ② 주차 줄의 겹침 풀기도 edit ⑦ 옛 열쇠가 쌓인 상태 파일로 첫 바퀴 → 새 사건 0.
// advFx(advise_test) 위에 가짜 hc의 ls를 바꿔 본다. 고치기 전 코드: ④·①(감시 항목 없음)·② 실패

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/opsstate"
)

func akWrite(t *testing.T, f, s string) {
	t.Helper()
	if err := os.WriteFile(DATA+"/"+f, []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
}

// akLs: 가짜 hc ls가 낼 터미널(주소 이름 상태 셋씩). fail이면 hc ls가 실패(종료 1)
func akLs(t *testing.T, fail bool, terms ...string) {
	t.Helper()
	ls := "cat '" + DATA + "/lsjson'"
	if fail {
		ls = "exit 1"
	}
	akWrite(t, "hc", "#!/bin/sh\ncase \"$1\" in ls) "+ls+";; machine) cat '"+DATA+"/mls' || exit 1;; esac\n")
	var js []string
	for i := 0; i+2 < len(terms); i += 3 {
		js = append(js, `{"address":"`+terms[i]+`","name":"`+terms[i+1]+`","status":"`+terms[i+2]+`"}`)
	}
	akWrite(t, "lsjson", `{"terminals":[`+strings.Join(js, ",")+`]}`)
}

func akKeys(t *testing.T) []string {
	t.Helper()
	b, _ := os.ReadFile(DATA + "/advise.tsv")
	return strings.Split(strings.TrimSpace(string(b)), "\n")[1:]
}

// ④ hc ls 못 읽은 바퀴에 열쇠 그대로 → 회복 뒤 같은 사건 0번. 대조: 읽었고 항목이 없으면 지워져, 다시 생기면 1번
func TestAdviseKeysHeldOnUnknown(t *testing.T) {
	advFx(t)
	watch := []string{"t2", watchName("m/t1"), "idle"}
	akLs(t, false, append([]string{"w/t5", "lane-a", "error"}, watch...)...)
	now := time.Now()
	adviseTick(now)
	if r := reports(t); len(r) != 1 || !has(akKeys(t), "lane-error:lane-a") {
		t.Fatalf("첫 사건: %q 열쇠 %q", r, akKeys(t))
	}
	akLs(t, true)
	adviseTick(now.Add(6 * time.Minute))
	if k := akKeys(t); !has(k, "lane-error:lane-a") || !has(k, "free-wait:lane-c") {
		t.Errorf("모름 바퀴에 열쇠가 지워짐: %q", k)
	}
	if r := reports(t); len(r) != 1 { // 단계 「모름」 레인의 lane-long 같은 새 항목도 모름 바퀴에는 미룬다
		t.Fatalf("모름 바퀴에 새 사건: %q", r)
	}
	akLs(t, false, append([]string{"w/t5", "lane-a", "error"}, watch...)...)
	adviseTick(now.Add(12 * time.Minute))
	if r := reports(t); len(r) != 1 {
		t.Fatalf("회복 뒤 같은 사건이 다시: %q", r)
	}
	// 대조: 읽었고 멈춤이 풀림 → 열쇠 지움, 다시 멈추면 한 번
	akLs(t, false, append([]string{"w/t5", "lane-a", "working"}, watch...)...)
	adviseTick(now.Add(18 * time.Minute))
	if has(akKeys(t), "lane-error:lane-a") {
		t.Errorf("항목이 없는데 열쇠가 남음: %q", akKeys(t))
	}
	n := len(reports(t)) // working 2시간 → 같은 단계에 오래(lane-long) 한 번
	akLs(t, false, append([]string{"w/t5", "lane-a", "error"}, watch...)...)
	adviseTick(now.Add(24 * time.Minute))
	if r := reports(t); len(r) != n+1 || !strings.Contains(r[n], "멈춘 레인(터미널 error): lane-a") {
		t.Fatalf("다시 멈춤 사건: %q", r)
	}
}

// 재료마다 그 재료의 열쇠만 남긴다(병합기 모름은 그 프로젝트, 배포 모름은 그 기계) — 모르는 꼴·hc 모름은 전부
func TestAdviseHeldScope(t *testing.T) {
	u := func(k string) opsstate.Item { return opsstate.Item{Key: "unknown:" + k, Kind: "unknown"} }
	h := adviseHeld([]opsstate.Item{u("merger:app"), u("deploy:w"), u("watch:m/t1"), u("source:journal")})
	for k, want := range map[string]bool{"merger:app": true, "merge-stall:app": true, "merge-slow:app": true, "merger:other": false,
		"deploy:w": true, "deploy:x": false, "watch:m/t1": true, "lane-error:a": false, "resident:x": false} {
		if h(k) != want {
			t.Errorf("%s: %v", k, !want)
		}
	}
	if h := adviseHeld([]opsstate.Item{u("source:hc")}); !h("lane-error:a") || !h("deploy:w") {
		t.Error("hc 모름은 전부")
	}
	if h := adviseHeld([]opsstate.Item{u("source:deploy"), u("resident")}); !h("deploy:x") || !h("resident:hc-fill") || h("chain:a") {
		t.Error("재료 deploy·상주 판 모름")
	}
	if h := adviseHeld(nil); h("lane-error:a") {
		t.Error("모름 없는 바퀴에 남김")
	}
}

// ① 감시 안 돎: 지휘 터미널이 있고 착지 전 열린 줄이 있으면 추린 상태로도 항목. 지휘가 사라졌거나 착지한 줄뿐이면 없음.
// 상태 한 장 기본 출력의 감시 칸은 그대로(숨긴 수만)
func TestAdviseWatchNeed(t *testing.T) {
	advFx(t)
	watchItem := func() int {
		n := 0
		for _, it := range adviseNow(time.Now()).Items {
			if it.Key == "watch:m/t1" {
				n++
			}
		}
		return n
	}
	akLs(t, false, "w/t5", "lane-a", "working", "t1", "coord", "idle") // 지휘 m/t1은 이 기계 t1, 감시 없음
	if n := watchItem(); n != 1 {
		t.Errorf("지휘는 있는데 감시 없음 → 항목 %d", n)
	}
	s, _ := stateCollectF(time.Now())
	if tr := opsstate.Trim(s, knobInt("state_done")); len(tr.Watch) != 0 || tr.Hidden.Watch != 1 {
		t.Errorf("기본 출력 감시 칸이 바뀜: %+v %+v", tr.Watch, tr.Hidden)
	}
	akLs(t, false, "w/t5", "lane-a", "working") // 지휘 사라짐(흔적)
	if n := watchItem(); n != 0 {
		t.Errorf("지휘가 사라진 흔적에 항목 %d", n)
	}
	akLs(t, false, "w/t5", "lane-a", "working", "t1", "coord", "idle")
	akWrite(t, "last.tsv", "lane-a\t착지\tabc\t"+time.Now().Add(-time.Hour).Format("15:04")+"\t착지\t-\tfresh\tr1\n")
	if n := watchItem(); n != 0 {
		t.Errorf("착지한 줄뿐인 지휘(감시는 한가로 끝남)에 항목 %d", n)
	}
}

// ② 주차 줄도 겹침 풀기는 edit --overlap-ok(rm은 준비 커밋을 버린다 — 그래서 옛 코드는 명령을 안 냈다)
func TestAdviseParkedEdit(t *testing.T) {
	advFx(t)
	q, _ := os.ReadFile(DATA + "/queue.tsv")
	akWrite(t, "queue.tsv", strings.Replace(string(q), "\tnormal\t-\t-\t-\tr3", "\tnormal\tparked\t-\t-\tr3", 1))
	for _, it := range adviseNow(time.Now()).Items {
		if it.Key == "free-wait:lane-c" {
			if len(it.Next) != 1 || it.Next[0] != "hc-fill edit lane-c --overlap-ok --why '겹침 풀기(hc-fill advise)'" {
				t.Errorf("주차 줄 명령: %+v", it)
			}
			return
		}
	}
	t.Fatal("주차 줄 겹침 항목 없음")
}

// ⑦ 운영 장면: 깔리는 순간 옛 열쇠가 잔뜩 쌓인 상태 파일 + 지휘가 사라진 감시 흔적 → 첫 바퀴 새 사건 0, 옛 열쇠는 걷힘
func TestAdviseFirstTickOldKeys(t *testing.T) {
	advFx(t)
	akLs(t, false, "w/t5", "lane-a", "working") // 지휘 m/t1·감시 모두 없음 — 흔적
	now := time.Now()
	var keys []string
	for _, it := range adviseNow(now).Items {
		keys = append(keys, it.Key)
	}
	for i := 0; i < 200; i++ {
		keys = append(keys, "free-wait:old-"+itoa(int64(i)))
	}
	akWrite(t, "advise.tsv", itoa(now.Unix()-3600)+"\n"+strings.Join(keys, "\n")+"\n")
	adviseTick(now)
	if r := reports(t); len(r) != 0 {
		t.Fatalf("첫 바퀴 사건: %q", r)
	}
	if k := akKeys(t); has(k, "free-wait:old-0") || len(k) == 0 {
		t.Errorf("옛 열쇠: %q", k)
	}
	adviseTick(now.Add(6 * time.Minute)) // 두 번째 바퀴도 0
	if r := reports(t); len(r) != 0 {
		t.Fatalf("두 번째 바퀴 사건: %q", r)
	}
}
