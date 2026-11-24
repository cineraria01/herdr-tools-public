package main

// 이 기계 이름(sync-local-name, 2026-11-24) ②③: 설정(hc-fill.tsv의 local 줄)이 있으면 진행 줄·표·배포 기록·--status·--log에 그 이름이
// 들고 「맥」은 0곳이다(이 장면의 「맥」은 모두 기계 이름 자리 — 뜻 낱말 없음). 옛 기록(「맥」)과 새 기록이 섞여도 --log·--log --json·
// --status·--at이 한 기계로 읽는다. 표기만 바뀌고 무엇을 어디에 까는지는 같다(대상 목록·--take의 옛 이름도 이 기계).

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	ct "herdr-tools/internal/clitest"
)

const testLocal = "here-a"

// setLocal: 개인 저장소에 hc-fill.tsv(local 줄)를 커밋·push — 더러운 저장소로 멈추지 않게
func (s *sx) setLocal(name string) {
	s.put(s.T+"/conf/hc-fill.tsv", "# 대기열 설정\n#local nope\nlocal    "+name+"\nlocal    second\n")
	s.git(s.T+"/conf", "add", "hc-fill.tsv")
	s.git(s.T+"/conf", "commit", "-qm", "local")
	s.gitLand(s.T+"/conf", "push", "-q", "origin", "main")
}

// ② 설정 있음: 견본 장면 그대로 돌려 견본의 「맥」 자리가 모두 그 이름(JSON 줄은 지도 차례만 달라 따로 본다)
func TestLocalNameSet(t *testing.T) {
	want, err := os.ReadFile("testdata/localname.golden")
	if err != nil {
		t.Fatal(err)
	}
	s := newSX(t)
	s.setLocal(testLocal)
	got := localScenario(s)
	if strings.Contains(got, "맥") {
		t.Errorf("설정 있음인데 「맥」이 남았다:\n%s", got)
	}
	wl, gl := ct.Lines(strings.ReplaceAll(string(want), "맥", testLocal)), ct.Lines(got)
	if len(wl) != len(gl) {
		t.Fatalf("줄 수가 다르다: 견본 %d · 지금 %d\n%s", len(wl), len(gl), got)
	}
	for i := range wl {
		if strings.HasPrefix(wl[i], "[{") {
			var rs []deployJSON
			if err := json.Unmarshal([]byte(gl[i]), &rs); err != nil || len(rs) != 2 || rs[0].Machines[testLocal] != "씀" || rs[1].Machines[testLocal] != "씀" {
				t.Errorf("--log --json의 기계 지도: %v %s", err, gl[i])
			}
			continue
		}
		if wl[i] != gl[i] {
			t.Errorf("%d번 줄: 바란 것 %q · 지금 %q", i+1, wl[i], gl[i])
		}
	}
}

// ③ 옛 기록(「맥」) 뒤에 이름을 정한 경우: 앞뒤 기록을 한 기계로
func TestLocalNameMixedLog(t *testing.T) {
	s := newSX(t)
	s.c.T = t
	s.synced()
	s.relPrep()
	s.run()
	c1 := ct.Git(s.T+"/repo", "rev-parse", "HEAD")[:12]
	s.ok("옛 기록: 「맥」으로 적힘", strings.Contains(ct.Read(s.MAC+"/.local/state/herdr-sync/deploy.tsv"), "\t맥=씀 "))
	s.setLocal(testLocal)
	s.run()
	s.ok("새 배포: 표·기록에 이름", s.r.RC == 0 && s.has(testLocal+": 같음") && !s.contains("맥") &&
		strings.HasSuffix(strings.TrimSpace(ct.Read(s.MAC+"/.local/state/herdr-sync/deploy.tsv")), "\t"+testLocal+"=같음 w1=같음 lx=같음"))
	s.run("--log")
	s.ok("--log: 옛 줄도 이 기계 이름으로", s.r.RC == 0 && !s.contains("맥") && strings.Count(s.r.Out, " "+testLocal+"=") == len(ct.Lines(s.r.Out)))
	s.run("--log", "3", "--json")
	var rs []deployJSON
	err := json.Unmarshal([]byte(s.r.Out), &rs)
	s.ok("--log --json: 옛 줄의 기계 지도도 이 기계 이름, raw는 기록 그대로", err == nil && len(rs) == 3 && rs[1].Machines[testLocal] == "씀" &&
		rs[1].Machines["맥"] == "" && strings.HasPrefix(rs[1].Raw, "맥=씀") && rs[2].Machines[testLocal] == "같음")
	s.landCommit("lane-two: 둘째")
	s.run("--status")
	s.ok("--status: 이 기계 줄·기준 표기", s.r.RC == 0 && s.has(testLocal+": "+c1+" · 뒤 1커밋") && s.contains(" ("+testLocal+" 기준)") && !s.contains("맥"))
	s.run()
	c2 := ct.Git(s.T+"/repo", "rev-parse", "HEAD")[:12]
	s.run("--at", c1)
	s.ok("--at: 이 기계도 되돌림, 기록에 이름", s.r.RC == 0 && s.has(testLocal+" 받음 hello") && s.has(testLocal+": 같음") && !s.contains("맥") &&
		strings.HasSuffix(strings.TrimSpace(ct.Read(s.MAC+"/.local/state/herdr-sync/deploy.tsv")), "\t"+testLocal+"=씀 w1=씀 lx=같음"))
	s.run("--status", "w1")
	s.ok("--at 뒤 --status: 되돌린 판", s.has(testLocal+": "+c1+" · 뒤 1커밋") && s.has("w1: "+c1+" · 뒤 1커밋"))
	s.ok("두 판이 다르다", c1 != c2)
	// 상태 한 장이 권하는 「herdr-sync --status <이 기계>」·「herdr-sync <이 기계>」: 이 기계만, hc --on으로 자기에게 가지 않는다
	s.run("--status", testLocal)
	s.ok("--status <이 기계>: 이 기계 줄 하나", s.r.RC == 0 && strings.Count(s.r.Out, testLocal+": ") == 1 && !s.contains("w1: ") && !s.contains("lx: "))
	s.run(testLocal)
	s.ok("herdr-sync <이 기계>: 이 기계만 맞춤", s.r.RC == 0 && s.has(testLocal+": 같음") && !s.contains("w1: ") &&
		!strings.Contains(ct.Read(s.T+"/calls"), testLocal) && strings.HasSuffix(strings.TrimSpace(ct.Read(s.MAC+"/.local/state/herdr-sync/deploy.tsv")), "\t"+testLocal+"=씀")) // --at 뒤라 이 기계만 새 판을 받는다
	ct.Unsetenv(s.t, "HERDR_SYNC_RELEASE_URL")
	s.c.Done()
}

// 이름 정하기·옛 이름 읽기(단위)
func TestLocalNameRead(t *testing.T) {
	d := t.TempDir()
	get := func(body string, ms ...string) (string, error) {
		if body != "-" {
			ct.Put(t, d+"/hc-fill.tsv", body)
		} else {
			os.Remove(d + "/hc-fill.tsv")
		}
		return localName(d, ms)
	}
	for _, c := range []struct{ body, want string }{
		{"-", "맥"}, {"agent x\n", "맥"}, {"local\n", "맥"}, {"#local nope\n  local\tbox-1  \n", "box-1"}, {"local a\nlocal b\n", "a"},
	} {
		if v, err := get(c.body, "w1"); err != nil || v != c.want {
			t.Errorf("%q → %q %v, 바란 것 %q", c.body, v, err, c.want)
		}
	}
	for _, body := range []string{"local a b\n", "local a=b\n", "local a:b\n", "local w1\n"} {
		if v, err := get(body, "w1"); err == nil {
			t.Errorf("%q는 오류여야 한다: %q", body, v)
		}
	}
	os.Remove(d + "/hc-fill.tsv")
	if err := os.Mkdir(d+"/hc-fill.tsv", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := localName(d, nil); err == nil {
		t.Error("못 읽는 설정은 오류(모름)여야 한다")
	}

	defer func(o string) { LOCAL = o }(LOCAL)
	LOCAL = "box-1"
	if !targets("local", "box-1", "Darwin") || !targets("맥,w1", "box-1", "Darwin") || !targets("box-1", "box-1", "Darwin") || targets("맥", "w1", "Darwin") {
		t.Error("대상: 이 기계는 local·이름·옛 이름 「맥」 목록에 든다")
	}
	if !spec([]string{localSpec("맥:.local/bin/x")}, "box-1", ".local/bin/x") || localSpec("w1:k") != "w1:k" || spec([]string{localSpec("맥:*")}, "w1", "k") {
		t.Error("--take: 이 기계는 옛 이름 「맥:」으로도")
	}
	if g := localRes("맥=씀 w1=충돌"); g != "box-1=씀 w1=충돌" || localOf("w1") != "w1" {
		t.Errorf("옛 기록 읽기: %q", g)
	}
	LOCAL = oldLocal
	if g := localRes("맥=씀  w1=충돌"); g != "맥=씀  w1=충돌" {
		t.Errorf("이름이 없으면 기록 칸 그대로: %q", g)
	}
}
