package main

// 운영 일지(opsjournal.go, 2026-11-24 ops-journal): 배포 한 번에 일지 한 줄(배포 기록 deploy.tsv 한 줄과 짝 — 기계마다 줄을 더 적지
// 않는다), --why는 왜 칸으로, --check는 적지 않음, 되돌리는 명령은 앞 성공 배포 판으로 --at. --auto on|off는 바뀐 때만 한 줄.

import (
	"encoding/json"
	"strings"
	"testing"

	ct "herdr-tools/internal/clitest"
	"herdr-tools/internal/journal"
)

func TestContractJournal(t *testing.T) {
	s := newSX(t)
	jf := s.MAC + "/.local/state/herdr-tools/journal.jsonl"
	t.Setenv(journal.Env, jf) // 시험 바이너리는 일지를 /dev/null로 돌려 자식에 물려준다 — 이 시험만 파일로
	s.synced()
	read := func() []journal.Entry {
		var es []journal.Entry
		for _, l := range ct.Lines(ct.Read(jf)) {
			var e journal.Entry
			if err := json.Unmarshal([]byte(l), &e); err != nil {
				t.Fatalf("깨진 일지 줄 %q: %v", l, err)
			}
			es = append(es, e)
		}
		return es
	}
	n0, d0 := len(read()), logRows(s.T)
	s.ok("준비의 배포마다 일지 한 줄(배포 기록과 같은 수)", n0 == d0 && n0 > 0)

	s.run("--why", "새 판 시험")
	es := read()
	c := ct.Git(s.T+"/repo", "rev-parse", "HEAD")[:12]
	s.ok("배포: 종료 0, 일지·배포 기록 한 줄씩", s.r.RC == 0 && len(es) == n0+1 && logRows(s.T) == d0+1)
	e := es[len(es)-1]
	s.ok("배포 줄: 도구·조치·대상·왜·결과·되돌리기·상태", e.Tool == "herdr-sync" && e.Action == "배포" && e.Target == "모든 기계" &&
		e.Why == "새 판 시험" && e.Result == "됨" && strings.HasPrefix(e.Undo, "herdr-sync --at ") && e.By != "" &&
		strings.HasPrefix(e.State, "판 "+c+" · 맥=") && strings.Contains(e.State, "w1=") && strings.HasSuffix(e.State, "(자세히 herdr-sync --log)"))

	s.run("--check")
	s.ok("--check는 일지에 적지 않음", len(read()) == n0+1)

	s.run("w1")
	es = read()
	s.ok("고른 기계: 대상·되돌리기에 그 기계", len(es) == n0+2 && es[len(es)-1].Target == "w1" && strings.HasSuffix(es[len(es)-1].Undo, " w1"))

	s.run("--auto", "on", "--why", "밤사이 착지 따라")
	s.run("--auto", "on")
	s.run("--auto")
	es = read()
	a := es[len(es)-1]
	s.ok("--auto on: 바뀐 때만 한 줄, 되돌리기는 off", s.r.RC == 0 && len(es) == n0+3 && a.Action == "auto on" && a.Why == "밤사이 착지 따라" &&
		a.Undo == "herdr-sync --auto off")
	s.run("--auto", "off")
	s.ok("--auto off: 한 줄", len(read()) == n0+4 && read()[n0+3].Action == "auto off")
}
