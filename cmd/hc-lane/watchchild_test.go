package main

// ht-watch-child-job(2026-11-27): 레인(w2/t2v1)이 자기 기계에서 hc-lane --read로 띄운 스크립트 터미널(w2/t2v7 jt-real, 부모 = 그 레인)이
// exit 0으로 끝나자 지휘 기계 watch가 「멈춤 뜻밖의 종료(finished)」를 냈고, 주인을 부모 레인 t2v1로 잡아(지휘가 아님 → 주인 사라짐)
// 모든 지휘에게 보냈다. 레인은 이 기계 레인 기록에 든 것뿐이고, 부모가 레인이면 주인은 그 레인의 주인이다.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"herdr-tools/internal/hcjson"
)

// recLanes: 시험 폴더 d에 레인 기록(lanes.tsv 「주소 이름 주인 [종류]」)
func recLanes(t *testing.T, d string, rows ...string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(d, "lanes.tsv"), []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// childEv: 지휘 기계 watch — 기록에는 지휘 t3yx가 띄운 레인 w2/t2v1만, hc ls에는 그 레인과 그 레인이 띄운 jt-real
func childEv(t *testing.T, childExit, laneState string) *ev {
	t.Helper()
	d := t.TempDir()
	recLanes(t, d, "w2/t2v1\tht-job-tmpdir\tt3yx\tfill")
	js := `{"terminals":[` +
		`{"address":"w2/t2v1","name":"ht-job-tmpdir","state":"` + laneState + `","status":null,"turn":3,"parent":null,"reason":"exited","cwd":"/"},` +
		`{"address":"w2/t2v7","name":"jt-real","state":"finished","status":null,"turn":0,"parent":"w2/t2v1","exit_code":` + childExit + `,"reason":"exited","cwd":"/"}]}`
	l, err := hcjson.ParseList([]byte(js))
	if err != nil {
		t.Fatal(err)
	}
	e := &ev{ed: d, cur: filepath.Join(d, "cur"), all: true, tls: l, lsok: true}
	for _, x := range l.Terminals {
		e.pmap = append(e.pmap, []string{x.Address, orHyph(x.Parent)})
	}
	return e
}

func stopLanes(ls []string) map[string]string {
	m := map[string]string{}
	for _, l := range ls {
		if f := strings.Split(l, tab); len(f) >= 7 && f[4] == "멈춤" {
			m[f[3]] = l
		}
	}
	return m
}

// ①② 레인이 띄운 스크립트 터미널이 끝나면(0이든 아니든) 레인 사건이 아니다 — 끝남(hc ls)으로도, hc-sweep 정리됨으로도
func TestWatchChildNotLane(t *testing.T) {
	for _, code := range []string{"0", "1"} {
		e := childEv(t, code, "live")
		if s := stopLanes(e.stopped(nil, nil)); s["jt-real"] != "" {
			t.Errorf("exit %s: 레인이 띄운 스크립트 터미널을 멈춤으로 냈다: %q", code, s["jt-real"])
		}
		cl := strings.Join([]string{"2026-11-27T01:20:00Z", "w2", "w2/t2v7", "jt-real", "정리됨", "-", "hc-sweep R2", "0"}, tab)
		if s := stopLanes(e.newEvents(nil, []string{cl}, "")); s["jt-real"] != "" {
			t.Errorf("exit %s: 레인이 띄운 스크립트 터미널의 정리됨을 멈춤으로 냈다: %q", code, s["jt-real"])
		}
	}
}

// ③ 기록에 든 진짜 레인이 뜻밖에 끝나면 지금처럼 멈춤, 주인은 기록의 지휘
func TestWatchRealLaneStops(t *testing.T) {
	e := childEv(t, "0", "finished")
	evl := []string{strings.Join([]string{"2026-11-27T01:00:00Z", "w2", "w2/t2v1", "ht-job-tmpdir", "중간", "-", "작업 중", "2"}, tab)}
	l := stopLanes(e.stopped(evl, nil))["ht-job-tmpdir"]
	if !strings.Contains(l, "뜻밖의 종료(finished)") {
		t.Fatalf("기록에 든 레인이 끝났는데 멈춤이 없다: %q", l)
	}
	if o := e.evowner(l); o != "t3yx" {
		t.Errorf("멈춤의 주인 = %q, 기대 t3yx", o)
	}
}

// 주인: 기록 없는 하위 터미널의 사건(주인 칸 없음)은 부모 레인의 주인으로 — 부모 레인 자체(지휘 아님)가 아니다
func TestWatchOwnerViaParentLane(t *testing.T) {
	e := childEv(t, "0", "live")
	l := strings.Join([]string{"2026-11-27T01:20:00Z", "w2", "w2/t2v7", "jt-real", "중간", "-", "측정", "0"}, tab)
	if o := e.evowner(l); o != "t3yx" {
		t.Errorf("부모 레인을 거친 주인 = %q, 기대 t3yx", o)
	}
}

// ④ 주인 없는 받은편지 레인(기록 주인 -, 부모 없음)은 지금처럼 주인 모름(-) — 모든 지휘 쪽(repoSend)
func TestWatchInboxLaneNoOwner(t *testing.T) {
	d := t.TempDir()
	recLanes(t, d, "w2/t9\tinbox-a\t-\tfill")
	l, err := hcjson.ParseList([]byte(`{"terminals":[{"address":"w2/t9","name":"inbox-a","state":"finished","status":null,"turn":2,"parent":null,"reason":"exited","cwd":"/"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	e := &ev{ed: d, cur: filepath.Join(d, "cur"), all: true, tls: l, lsok: true, pmap: [][]string{{"w2/t9", "-"}}}
	s := stopLanes(e.stopped(nil, nil))["inbox-a"]
	if !strings.Contains(s, "뜻밖의 종료(finished)") {
		t.Fatalf("받은편지 레인이 끝났는데 멈춤이 없다: %q", s)
	}
	if o := e.evowner(s); o != "-" {
		t.Errorf("받은편지 레인 주인 = %q, 기대 -", o)
	}
}
