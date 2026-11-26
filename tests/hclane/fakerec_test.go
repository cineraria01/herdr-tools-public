package hclane_test

// 가짜 hc ls 레인의 레인 기록(2026-11-27 ht-watch-child-job): wait·watch는 이 기계 레인 기록(lanes.tsv)에 든 터미널만 레인으로 본다.
// 실제 레인은 hc-lane 띄우기가 늘 기록하니, 틀이 HC_FAKE_LS로 꾸민 터미널도 그 레인의 기록을 둔다 — 이미 (주소·이름) 줄이 있으면
// 그대로(시험이 적은 주인을 덮지 않음), 없으면 「주소 이름 -」(주인 모름 — 기록이 없던 때와 같은 주인 판정). 항목에 "norec":true를
// 두면 기록하지 않는다(레인이 다른 기계에서 띄운 하위 터미널 — 지휘 기계 기록에 없다).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// fakeLaneRec: 그 환경(env)으로 hc-lane wait·watch·events를 부르기 전에 HC_FAKE_LS의 터미널을 레인 기록에 넣는다
func (k *K) fakeLaneRec(env, args []string) {
	if len(args) == 0 || (args[0] != "wait" && args[0] != "watch" && args[0] != "events") {
		return
	}
	m := map[string]string{}
	for _, s := range env {
		key, v, _ := strings.Cut(s, "=")
		m[key] = v
	}
	var ls struct {
		Terminals []struct {
			Address, Name string
			NoRec         bool `json:"norec"`
		}
	}
	if m["HC_FAKE_LS"] == "" || json.Unmarshal([]byte(m["HC_FAKE_LS"]), &ls) != nil {
		return
	}
	st := m["XDG_STATE_HOME"]
	if st == "" {
		st = filepath.Join(m["HOME"], ".local/state")
	}
	p := filepath.Join(st, "hc-lane", "lanes.tsv")
	have := map[string]bool{}
	b, _ := os.ReadFile(p)
	for _, l := range strings.Split(string(b), "\n") {
		if f := strings.Split(l, "\t"); len(f) >= 2 {
			have[f[0]+"\t"+f[1]] = true
		}
	}
	var add string
	for _, t := range ls.Terminals {
		if t.Name == "" { // hc-lane이 이름 없는 항목을 「-」로 읽는다(watch.go orHyph)
			t.Name = "-"
		}
		if kk := t.Address + "\t" + t.Name; !t.NoRec && !have[kk] {
			have[kk] = true
			add += kk + "\t-\n"
		}
	}
	if add == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		k.t.Fatal(err)
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		k.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(add); err != nil {
		k.t.Fatal(err)
	}
}
