// evpick.go: events·wait·watch가 사건을 읽을 기계 고르기와 원격 실행(watch.go에서 옮김 — 줄 수 한도, 동작 그대로)
package main

import (
	"fmt"
	"os"
	"strings"

	"herdr-tools/internal/hcjson"
)

// pick: --all이면 hc machine ls에서 연결된 기계를 ems에, 아닌 기계를 unk에. --on으로 준 기계는 늘 ems에
func (e *ev) pick() {
	e.ems, e.unk, e.pbad = append([]string(nil), e.ons...), nil, false
	if !e.all {
		return
	}
	js, rc := hcRun(0, errDrop, "machine", "ls", "--json")
	ms, err := hcjson.ParseMachines([]byte(js))
	if rc != 0 || err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane %s: hc machine ls 실패 — 이 기계만 본다\n", e.ec)
		e.pbad = true
		return
	}
	for _, m := range ms {
		if has(e.ems, m.Name) {
			continue
		}
		if m.State == "connected" {
			e.ems = append(e.ems, m.Name)
		} else {
			e.unk = append(e.unk, m.Name)
		}
	}
}

// evrd: 원격에서 도는 사건 읽기(셸 판과 같은 글 — 원격 기계는 sh로 돈다)
const evrd = `f=${XDG_STATE_HOME:-$HOME/.local/state}/hc-lane/events.tsv; [ -f "$f" ] || exit 0; if [ "$1" = 0 ]; then cat "$f"; else tail -n "$1" "$f"; fi`

// nosh: 그 기계가 프로그램을 띄우지 못함(spawn_failed + 「cannot start」)
func nosh(s string) bool {
	return strings.Contains(s, `"spawn_failed"`) && strings.Contains(s, "cannot start ")
}

// rexec: hc exec --on(60초). 실패했는데 nosh면 종료 9
func rexec(m string, args ...string) (string, int) {
	var eb strings.Builder
	a := append([]string{"exec", "--on", m, "--timeout", "60", "--"}, args...)
	out, rc := captErr(90, &eb, "hc", a...) // --timeout 60은 원격 명령의 상한 — hc 자신이 멈춰도 끝나게(gofix M4)
	if rc != 0 && nosh(eb.String()) {
		rc = 9
	}
	return out, rc
}
