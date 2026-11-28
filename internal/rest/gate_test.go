package rest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 문을 건너뛰지 못하게(ht-rest-one-gate): 저장소의 모든 도구에서 터미널을 닫는 호출(hc kill)과 대기열의 자동 제출(asubRun)은 같은 함수
// 안에서 쉼 판정(rest.Ask) 뒤에만 있다. 판정이 필요 없는 닫기는 아래 목록뿐이고 줄마다 까닭이 있다 — 닫는 길을 더하며 문을 빼먹으면 실패한다
var noGate = map[string]string{
	"hc-sweep/judge.go closeTerm":        "settle(judge + 쉼 판정)이 낸 쉼만 받는다 — TestEveryCloseGoesThroughSettle이 settle 뒤에만 부르는지 본다",
	"hc-fill/revive.go reviveShellClose": "재시동 뒤 셸만 남은 빈 셸(에이전트가 이미 없다)을 닫는다 — 안에서 도는 에이전트 일이 없다",
	"hc-merger/frontsteady.go termKill":  "병합기 추측 검증 일꾼(검증 스크립트를 도는 env 터미널, 에이전트 아님)을 끝내는 것이 목적",
}

func TestEveryKillGoesThroughGate(t *testing.T) {
	files, err := filepath.Glob("../../cmd/*/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("도구 소스를 못 찾음: %v", err)
	}
	gate := regexp.MustCompile(`rest\.Ask\(`)
	act := regexp.MustCompile(`(?m)^.*\bhc\w*\b.*"kill".*$|asubRun\(`)
	seen := map[string]bool{}
	acts := 0
	for _, p := range files {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		rel := strings.TrimPrefix(filepath.ToSlash(p), "../../cmd/")
		for _, fn := range strings.Split(string(b), "\nfunc ")[1:] {
			head := fn
			if strings.HasPrefix(head, "(") { // 메서드: 받는 쪽 건너뜀
				head = head[strings.Index(head, ") ")+2:]
			}
			name := head[:strings.IndexAny(head, "([ ")]
			key := rel + " " + name
			for _, m := range act.FindAllStringIndex(fn, -1) {
				if strings.HasPrefix(fn[m[0]:], "asubRun(") && m[0] == 0 { // 정의 자체
					continue
				}
				acts++
				if _, ok := noGate[key]; ok {
					seen[key] = true
					continue
				}
				if g := gate.FindStringIndex(fn); g == nil || g[0] > m[0] {
					t.Errorf("%s: 쉼 판정(rest.Ask) 없이 닫거나 제출함 — %q", key, strings.TrimSpace(fn[m[0]:m[1]]))
				}
			}
		}
	}
	for k := range noGate {
		if !seen[k] {
			t.Errorf("판정 없이 닫아도 되는 목록의 %s가 이제 닫지 않는다 — 목록에서 뺀다", k)
		}
	}
	if acts < 9 { // hc-sweep 1·hc-fill 4(+제출 1)·hc-lane 1·hc-job-server 1·병합기 1 — 무늬가 깨져 아무것도 못 보면 실패
		t.Fatalf("닫기·제출 자리 %d곳만 찾음(무늬가 깨졌나)", acts)
	}
}
