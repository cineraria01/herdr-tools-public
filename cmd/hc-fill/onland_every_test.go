package main

import (
	"os/exec"
	"strings"
	"testing"
)

// 착지마다(land-every, deploy-log-rollback ⑥): 켠 첫 바퀴는 기준만, 잇단 착지 둘은 한 바퀴에 한 번으로(실행기가 한 번 돌림),
// 같은 머리는 다시 넣지 않음, 명령의 끝 줄이 사건 글에, 실패는 막힘
func TestOnlandEveryCoalescesLandings(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	d := t.TempDir()
	origin, up, local := d+"/o.git", d+"/up", d+"/local"
	gitT(t, d, nil, "init", "-q", "--bare", "-b", "main", origin)
	gitT(t, d, nil, "clone", "-q", origin, up)
	gitT(t, up, nil, "commit", "-q", "--allow-empty", "-m", "base: 처음")
	gitT(t, up, nil, "push", "-q", "origin", "main")
	gitT(t, d, nil, "clone", "-q", origin, local)
	rec := onlandEnv(t, local, `echo 준비; echo "배포할 것 2커밋: $HC_ONLAND_LANES"`)
	confLines = append(confLines, "land-every dev")
	SELF = "true" // 실행기는 띄운 척만 — 아래에서 직접 돌린다
	onlandTick()
	if nonEmpty(onlandDir() + "/dev.pending") {
		t.Fatal("켠 첫 바퀴에 명령 차례를 넣음(기준만 적어야)")
	}
	gitT(t, up, nil, "commit", "-q", "--allow-empty", "-m", "lane-a: 하나")
	gitT(t, up, nil, "commit", "-q", "--allow-empty", "-m", "lane-b: 둘")
	sha := gitT(t, up, nil, "rev-parse", "HEAD")
	gitT(t, up, nil, "push", "-q", "origin", "main")
	FETCHED = ""
	onlandTick()
	FETCHED = ""
	onlandTick() // 같은 머리 — 더 넣지 않는다
	rs := readRows(onlandDir() + "/dev.pending")
	if len(rs) != 2 || rs[0].F(opLane) != "lane-a" || rs[1].F(opLane) != "lane-b" || rs[1].F(opOwner) != "-" || rs[1].F(opSHA) != sha {
		t.Fatalf("pending = %v, 바람 lane-a·lane-b(주인 -, %s)", rs, sha)
	}
	if rc := onlandRun("dev"); rc != 0 {
		t.Fatalf("실행기 rc %d", rc)
	}
	reps, _ := readFile(rec)
	ls := lines(strings.TrimSpace(reps))
	if len(ls) != 1 || !strings.HasPrefix(ls[0], "report|중간|-|착지 뒤 dev 성공: 배포할 것 2커밋: lane-a,lane-b — lane-a,lane-b 착지(") || strings.Contains(ls[0], "--to") {
		t.Fatalf("사건 한 번(끝 줄·주인 없음):\n%s", reps)
	}

	// 실패 → 막힘(끝 줄을 붙여)
	confLines = []string{"local here", "repo app here " + local, "on-land dev app echo 스스로 배포 실패; exit 1", "land-every dev"}
	gitT(t, up, nil, "commit", "-q", "--allow-empty", "-m", "lane-c: 셋")
	gitT(t, up, nil, "push", "-q", "origin", "main")
	FETCHED = ""
	onlandTick()
	if rc := onlandRun("dev"); rc != 0 {
		t.Fatalf("실행기 rc %d", rc)
	}
	reps, _ = readFile(rec)
	ls = lines(strings.TrimSpace(reps))
	if len(ls) != 2 || !strings.HasPrefix(ls[1], "report|막힘|-|착지 뒤 dev 실패(종료 1): 스스로 배포 실패 — lane-c 착지(") {
		t.Fatalf("실패 사건:\n%s", reps)
	}
}

func TestSubjLane(t *testing.T) {
	for in, want := range map[string]string{"lane-a: 고침": "lane-a", "Merge branch x": "Merge", "a b: c": "a", "": "?"} {
		if got := subjLane(in); got != want {
			t.Errorf("subjLane(%q) = %q, 바람 %q", in, got, want)
		}
	}
}
