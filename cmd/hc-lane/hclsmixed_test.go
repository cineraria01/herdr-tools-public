package main

import "testing"

// R-3M01: hc ls에 Windows 기계 터미널(cwd C:\…)이나 이상한 한 줄이 섞여도 목록을 버리지 않는다.
// Windows 줄은 그대로, 이상한 줄은 모름 — clean·gc의 「쓰는 터미널」 판단에서 쓰는 중으로 본다.
func TestParsePairsMixed(t *testing.T) {
	out := `{"machines":[{"name":"w","state":"connected"}],"terminals":[` +
		`{"address":"t1","state":"live","cwd":"/other"},` +
		`{"address":"w/t2","state":"live","cwd":"C:\\Users\\u","agent":"powershell.exe"},` +
		`{"address":"t3","state":"live","cwd":"."}]}`
	ps, r := parsePairs(out, "")
	if r != 0 || len(ps) != 3 {
		t.Fatalf("목록을 버림: r=%d %v", r, ps)
	}
	if ps[1][1] != `C:\Users\u` || ps[2][1] != unkCwd {
		t.Fatalf("칸 %v", ps)
	}
	oldWt, oldOn, oldSelf := wt, on, selfTerm
	defer func() { wt, on, selfTerm = oldWt, oldOn, oldSelf }()
	wt, on, selfTerm = "/w/lane", "", ""
	b := busyTerms(ps, false)
	if len(b) != 1 || b[0] != "t3 "+unkCwd {
		t.Fatalf("모름 항목만 쓰는 중이어야 한다: %v", b)
	}
	if _, r := parsePairs(`{"terminals":[{"address":"t1","state":"live","cwd":"/a"},{"state":"live"}]}`, ""); r != 2 {
		t.Fatalf("주소 없는 항목은 목록 전체 모름(2)이어야 한다: %d", r)
	}
}
