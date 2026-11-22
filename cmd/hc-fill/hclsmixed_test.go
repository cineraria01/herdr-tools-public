package main

import "testing"

// R-3M01: hc ls에 Windows 기계 터미널이나 이상한 한 줄이 섞여도 바퀴가 목록을 버리지 않는다(02:37부터 아무것도 못 띄운 장애).
// 이상한 줄은 주소를 남기고 유형 모름(agent 「-」) — countSeats가 그 기계에 새 일을 놓지 않는다.
func TestParseTermsMixed(t *testing.T) {
	raw := `{"terminals":[` +
		`{"address":"t1","state":"live","cwd":"/w","agent":"claude-code","name":"a","status":"working"},` +
		`{"address":"w/t2","state":"live","cwd":"C:\\Users\\u","agent":"powershell.exe"},` +
		`{"address":"m/t3","state":"live","cwd":"rel","agent":"claude-code","name":"b"}]}`
	ts, ok := parseTerms(raw)
	if !ok || len(ts) != 3 {
		t.Fatalf("목록을 버림: ok=%v %v", ok, ts)
	}
	if ts[1].Agent != "powershell.exe" {
		t.Fatalf("Windows 줄: %+v", ts[1])
	}
	if ts[2].Addr != "m/t3" || ts[2].Agent != "-" || ts[2].Name != "-" {
		t.Fatalf("모름 줄: %+v", ts[2])
	}
}
