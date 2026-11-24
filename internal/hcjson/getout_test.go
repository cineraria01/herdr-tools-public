package hcjson

import "testing"

// dedupe-helpers ③: hc get 출력 고르기 — 옛 네 벌(hc-sweep detached·hc-fill hcGet·hc-lane hcGetRaw·herdr-hooks cotenant_scan)과
// hc-job-server의 꼴을 덮는 표, 고른 것을 ParseGet에 넘긴 결과까지
func TestGetOutTable(t *testing.T) {
	const nf = `{"error":{"hint":"see hc ls --all","kind":"not_found","message":"no terminal \"t9\""}}`
	const live = `{"address":"t9","state":"live","cwd":"/w"}`
	cases := []struct {
		name           string
		stdout, stderr string
		rc             int
		pick           string
		found, ok      bool // ParseGet 결과: 있음, 오류 없음
	}{
		{"종료 0은 표준 출력", live, "경고 한 줄", 0, live, true, true},
		{"종료 3 not_found는 표준 오류(hc 0.1.22)", "", nf, 3, nf, false, true},
		{"종료 3 표준 오류가 공백뿐이면 표준 출력", nf, " \n", 3, nf, false, true},
		{"종료 3 표준 오류가 다른 글이면 모름", "", "core unreachable", 3, "core unreachable", false, false},
		{"종료 1은 모름", "", nf, 1, nf, false, false},
		{"종료 11 끝남은 표준 출력", live, "", 11, live, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := GetOut(c.stdout, c.stderr, c.rc)
			if got != c.pick {
				t.Fatalf("고른 출력 %q (바람 %q)", got, c.pick)
			}
			_, found, err := ParseGet([]byte(got), c.rc, "t9")
			if found != c.found || (err == nil) != c.ok {
				t.Fatalf("ParseGet found=%v err=%v", found, err)
			}
		})
	}
}
