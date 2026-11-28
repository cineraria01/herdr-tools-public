package main

import (
	"path/filepath"
	"testing"
	"time"

	"herdr-tools/internal/upstream"
)

// 감사 atp89b9-umluoe 중간 1: 묶음의 수명 — 보류·점검 대상은 살아 있는 세션의 묶음만(바퀴가 정함), 점검이 잰 판이 맡긴 묶음과 다르면
// 그 묶음은 잴 수 없음(다시 띄우지 않음). ㄱ 옛 판이 틀리고 그 세션이 끝난 뒤 새 판 통과 → 보류 끝 ㄴ hc를 올릴 때 설치본과 다른 판으로 도는
// 옛 세션 → 점검 한 번 뒤 그 묶음은 점검 대상 아님(대기열이 3번×25분 쓰지 않음) ㄷ 처음 깔림에 판 둘 → 설치본 아닌 기준은 점검 한 번 뒤 빠짐
func TestUpstreamBundleLifetime(t *testing.T) {
	var s sent
	tick := func(p, hc string, clis ...string) { upstreamCheck("w1", p, false, upRun("hc "+hc), clis, s.rep(true)) }
	// ㄱ
	p := filepath.Join(t.TempDir(), "upstream.json")
	tick(p, "0.1.22", "2.0.0")
	tick(p, "0.1.22", "2.0.0", "2.0.1")
	probeApply("w1", p, "claude-2.0.1_hc-0.1.22", "claude-2.0.1_hc-0.1.22", "fail", []pRow{{"x", pBad, ""}}, s.rep(true))
	tick(p, "0.1.22", "2.0.0", "2.0.2") // 틀린 판 세션은 끝남, 고친 새 판
	probeApply("w1", p, "claude-2.0.2_hc-0.1.22", "claude-2.0.2_hc-0.1.22", "pass", nil, s.rep(true))
	if id, held := loadUp(t, p).Owed(); id != "claude-2.0.0_hc-0.1.22" || held { // 남은 것은 아직 점검 안 한 기준의 차례뿐
		t.Fatalf("ㄱ 틀린 판 세션이 끝나면 보류 끝: %q %v", id, held)
	}
	tick(p, "0.1.22", "2.0.1") // 틀린 판 세션이 다시 보이면 그 세션 동안은 보류(점검은 다시 안 함)
	if id, held := loadUp(t, p).Owed(); id != "" || !held {
		t.Fatalf("ㄱ 틀린 판이 살아 있으면 보류: %q %v", id, held)
	}
	// ㄴ
	p = filepath.Join(t.TempDir(), "upstream.json")
	tick(p, "0.1.22", "2.0.1")
	tick(p, "0.1.23", "2.0.1", "2.0.2") // hc 올림, 옛 세션 2.0.1과 설치본 2.0.2
	r := loadUp(t, p)
	id, _ := r.Owed()
	if id != "claude-2.0.1_hc-0.1.23" {
		t.Fatalf("ㄴ 앞 점검 대상: %q", id)
	}
	probeApply("w1", p, id, "claude-2.0.2_hc-0.1.23", "pass", nil, s.rep(true)) // 점검은 설치본 판만 잰다
	if id2, held := loadUp(t, p).Owed(); id2 != "" || !held {
		t.Fatalf("ㄴ 옛 판 세션이 사는 동안 보류는 이어지되 점검 대상은 없음: %q %v", id2, held)
	}
	tick(p, "0.1.23", "2.0.2") // 옛 세션 끝남
	if id2, held := loadUp(t, p).Owed(); id2 != "" || held {
		t.Fatalf("ㄴ 옛 세션이 끝나면 보류 끝: %q %v", id2, held)
	}
	// ㄷ
	p = filepath.Join(t.TempDir(), "upstream.json")
	tick(p, "0.1.22", "2.0.0", "2.0.1")
	r = loadUp(t, p)
	id, held := r.Owed()
	if id != "claude-2.0.0_hc-0.1.22" || held {
		t.Fatalf("ㄷ 기준 점검 차례: %q %v", id, held)
	}
	probeApply("w1", p, id, "claude-2.0.1_hc-0.1.22", "pass", nil, s.rep(true))
	if id, held := loadUp(t, p).Owed(); id != "" || held {
		t.Fatalf("ㄷ 설치본 아닌 기준은 빠지고 설치본은 통과: %q %v", id, held)
	}
	if b := loadUp(t, p).Bundles["claude-2.0.0_hc-0.1.22"]; b.State != upstream.Trusted || !b.Skip {
		t.Fatalf("ㄷ 기준은 믿음 그대로·잴 수 없음 표시: %+v", b)
	}
}

// 감사 atp89b9 낮음 1: 주소를 못 받았을 때 닫을 것은 이번 임시 폴더에서 도는 점검용 세션 이름 하나뿐(다른 폴더·다른 이름·끝난 것은 아님)
func TestProbeLeftover(t *testing.T) {
	dir := t.TempDir()
	ls := `{"machines":[],"terminals":[` +
		`{"address":"t1","cwd":"/other","name":"upstream-probe-agent","state":"live"},` +
		`{"address":"t2","cwd":"` + dir + `","name":"someone","state":"live"},` +
		`{"address":"t3","cwd":"` + dir + `","name":"upstream-probe-agent","state":"live"}]}`
	r := func(_ time.Duration, _ []string, name string, args ...string) result {
		if name == "hc" && len(args) > 0 && args[0] == "ls" {
			return result{out: ls}
		}
		return result{rc: 1}
	}
	if a := probeLeftover(r, dir); a != "t3" {
		t.Fatalf("이번 폴더의 점검용 세션만: %q", a)
	}
	if a := probeLeftover(r, t.TempDir()); a != "" {
		t.Fatalf("다른 폴더면 없음: %q", a)
	}
	if a := probeLeftover(func(time.Duration, []string, string, ...string) result { return result{rc: 1} }, dir); a != "" {
		t.Fatalf("hc ls 실패면 없음: %q", a)
	}
}
