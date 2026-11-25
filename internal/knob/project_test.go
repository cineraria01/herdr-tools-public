package knob

import (
	"strings"
	"testing"
	"time"
)

// 병합기 손잡이의 프로젝트별 덮어쓰기(merger-knob-per-project): 「이름@프로젝트」는 병합기(Remote) 손잡이만, 원격에는 그 프로젝트 몫으로 풀린 값
func TestProjectFind(t *testing.T) {
	d, ok := Find("merger_front_wait@app-b")
	if !ok || d.Per != PerProject || d.Of() != "app-b" || d.Base() != "merger_front_wait" || d.Check("1500") != nil || d.Check("10") == nil {
		t.Fatalf("merger_front_wait@app-b = %+v %v", d, ok)
	}
	for _, n := range []string{"idle@app-b", "sweep_r1@app-b", "merger_dead@app-b", "merger_front_wait@", "merger_front_wait@a@b"} {
		if _, ok := Find(n); ok {
			t.Errorf("%q가 찾아졌다", n)
		}
	}
}

func remoteOf(es map[string]Entry, p string, now time.Time) Remote {
	return ParseRemote(FormatRemote(ForProject(es, p, now), now), now)
}

// ① 프로젝트별 1500 + 공통 600 ② 프로젝트별만(공통 없음) ③ 기한 지난 프로젝트별 값은 공통으로
func TestForProject(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	es := map[string]Entry{
		"merger_front_wait":       {Name: "merger_front_wait", Value: "600"},
		"merger_front_wait@app-b": {Name: "merger_front_wait@app-b", Value: "1500"},
		"merger_batch@app-b":      {Name: "merger_batch@app-b", Value: "5"},
		"merger_idle_load@app-b":  {Name: "merger_idle_load@app-b", Value: "80", Until: now.Unix() - 1},
		"merger_idle_load":        {Name: "merger_idle_load", Value: "70"},
	}
	b, a := remoteOf(es, "app-b", now), remoteOf(es, "app-a", now)
	if b.Get("merger_front_wait") != "1500" || a.Get("merger_front_wait") != "600" {
		t.Fatalf("맨 앞 기다림 B=%s A=%s", b.Get("merger_front_wait"), a.Get("merger_front_wait"))
	}
	if b.Get("merger_batch") != "5" || a.Get("merger_batch") != "3" {
		t.Fatalf("공통 없는 프로젝트별 값 B=%s A=%s(기본 3)", b.Get("merger_batch"), a.Get("merger_batch"))
	}
	if _, ok := a["merger_batch"]; ok {
		t.Fatal("다른 프로젝트 값이 샜다")
	}
	if b.Get("merger_idle_load") != "70" {
		t.Fatalf("기한 지난 프로젝트별 값 = %s(공통 70이어야)", b.Get("merger_idle_load"))
	}
	if m := FormatRemote(ForProject(es, "app-b", now), now); strings.Contains(m, "@") {
		t.Fatalf("원격에 프로젝트 이름이 실렸다: %q", m)
	}
	if ParseRemote("hc-knobs\n\nmerger_batch@app-b\t5\t-\n", now)["merger_batch@app-b"] != "" {
		t.Fatal("원격의 「이름@프로젝트」 줄을 받았다")
	}
	// Resolve: 프로젝트별 값이 없거나 지났으면 공통 손잡이 → 기본
	d, _ := Find("merger_idle_load@app-b")
	if v, src, _ := Resolve(d, es, nil, "", now); v != "70" || src != FromKnob {
		t.Fatalf("Resolve 지난 값 = %s %s", v, src)
	}
	d, _ = Find("merger_tick@app-b")
	if v, src, _ := Resolve(d, es, nil, "", now); v != "10" || src != FromDefault {
		t.Fatalf("Resolve 없음 = %s %s", v, src)
	}
}

// ⑤ 프로젝트별 값을 안 쓰면 원격 내용이 전과 바이트까지 같다(견본)
func TestForProjectGolden(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	es := map[string]Entry{
		"merger_front_wait": {Name: "merger_front_wait", Value: "1500", Until: 2_000_000},
		"merger_batch":      {Name: "merger_batch", Value: "4"},
		"sweep_r1":          {Name: "sweep_r1", Value: "45"},
		"idle":              {Name: "idle", Value: "20"},
		"seats@box-1":       {Name: "seats@box-1", Value: "2"},
	}
	const want = "hc-knobs\n\nmerger_batch\t4\t-\nmerger_front_wait\t1500\t2000000\nsweep_r1\t45\t-\n"
	if got := FormatRemote(es, now); got != want {
		t.Fatalf("FormatRemote = %q", got)
	}
	for _, p := range []string{"app-a", "app-b", ""} {
		if got := FormatRemote(ForProject(es, p, now), now); got != want {
			t.Fatalf("ForProject(%q) = %q", p, got)
		}
	}
}
