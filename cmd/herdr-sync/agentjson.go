package main

// --log --json(2026-11-24 agent-skill-guides, 칸 docs/ops/agents.md): 배포 기록 줄을 칸 이름으로 — 사람용 줄과 같은 사실

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// takeJSON: 인자 어디에 있든 --json을 떼어 낸다
func takeJSON(args []string) ([]string, bool) {
	var out []string
	js := false
	for _, a := range args {
		if a == "--json" {
			js = true
			continue
		}
		out = append(out, a)
	}
	return out, js
}

// deployJSON: 배포 기록 한 줄. 기계별 결과는 「<기계>=<결과>」 낱말(빈칸 구분)을 나눈 것
type deployJSON struct {
	When     string            `json:"when"`
	Kind     string            `json:"kind"` // 배포 | 되돌림
	Exit     *int              `json:"exit"` // 종료 코드(숫자가 아니면 null)
	Tools    string            `json:"tools_commit"`
	Source   string            `json:"source"`
	Config   string            `json:"config_commit"`
	By       string            `json:"by"`
	Machines map[string]string `json:"machines"` // 기계 → 같음|씀|충돌|실패|건너뜀
	Raw      string            `json:"raw"`      // 기계별 결과 칸 그대로
}

func logJSON(rs []logRow) int {
	out := []deployJSON{}
	for _, r := range rs {
		d := deployJSON{When: r.when, Kind: r.kind, Tools: r.c12, Source: r.src, Config: r.conf, By: r.by, Raw: r.res, Machines: map[string]string{}}
		if v, err := strconv.Atoi(r.rc); err == nil {
			d.Exit = &v
		}
		for _, w := range strings.Fields(r.res) {
			if m, s, ok := strings.Cut(w, "="); ok {
				d.Machines[m] = s
			}
		}
		out = append(out, d)
	}
	b, err := json.Marshal(out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "herdr-sync: JSON을 만들지 못했다: %v\n", err)
		return 1
	}
	fmt.Println(string(b))
	return 0
}
