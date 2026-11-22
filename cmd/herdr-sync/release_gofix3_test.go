package main

// 재감사 2바퀴(gofix3-sync) installBins·release 재현 시험 — ops/R-216·R-218

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ops/R-216: 실제 수신 경로(installBins → hc exec)에서 hc가 「W」 한 줄을 돌려준 뒤 실패로 끝나면 받음을 찍지 않고 못 깖·다름
func TestInstallBinsTransportFailsAfterLine(t *testing.T) {
	old, oldc := BINS, check
	defer func() { BINS, check = old, oldc }()
	BINS, check = []binUnit{{"demo", "all"}}, false
	calls := t.TempDir() + "/calls"
	fakeBin(t, "hc", "echo x >> "+calls+"\nprintf 'W\\tdemo\\n'\nexit 255\n")
	so, se := captureStd(t, func() {
		diff, ok := installBins("w1", "Darwin", &relInfo{c12: "c", src: "s", base: "file:///r", tag: "bin-c", sums: map[string]string{"demo": "ab"}})
		if ok || len(diff) != 1 || diff[0] != "바이너리" {
			t.Errorf("전송 실패인데 다 깔림: %v %v", diff, ok)
		}
	})
	if readOr(calls) != "x\n" {
		t.Fatalf("원인이 안 일어남: hc 호출 %q", readOr(calls))
	}
	if strings.Contains(so, "받음") || strings.Contains(se, "받음") {
		t.Errorf("받음을 찍었다: %q %q", so, se)
	}
	if !strings.Contains(se, "w1 바이너리 깔기 실패 (종료 255") {
		t.Errorf("실패 표시가 없다: %q", se)
	}
}

// captureStd: f가 쓰는 표준 출력·오류를 받는다
func captureStd(t *testing.T, f func()) (string, string) {
	t.Helper()
	d := t.TempDir()
	o, _ := os.Create(d + "/o")
	e, _ := os.Create(d + "/e")
	so, se := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = o, e
	defer func() { os.Stdout, os.Stderr = so, se; o.Close(); e.Close() }()
	f()
	return readOr(d + "/o"), readOr(d + "/e")
}

// ops/R-218: 이름으로 댄 리눅스 대상 기계가 꺼져 OS를 모르면 그 커밋의 명세(SHA256SUMS)를 올리지 않는다 — 다르게 짜인 자산 목록이 먼저 공개되면
// 그 기계가 돌아와도 같은 커밋에 리눅스 자산을 더할 수 없었다
func TestReleaseHoldsWhenTargetOSUnknown(t *testing.T) {
	if goBin() == "" || testing.Short() {
		t.Skip("go 없음 또는 -short")
	}
	hct, _ := ghEnv(t)
	REPO, STATE = newRepo(t), t.TempDir()
	for p, c := range map[string]string{"go.mod": "module herdr-tools\n\ngo 1.24\n", "cmd/hello/main.go": "package main\n\nfunc main() {}\n"} {
		os.MkdirAll(filepath.Dir(REPO+"/"+p), 0o755)
		os.WriteFile(REPO+"/"+p, []byte(c), 0o644)
	}
	gitT(t, REPO, "add", "-A")
	gitT(t, REPO, "commit", "-qm", "go")
	checked := gitT(t, REPO, "rev-parse", "HEAD")
	t.Setenv("HERDR_SYNC_RELEASE_URL", "file://"+hct+"/rel")
	oldB, oldOS := BINS, machOS
	defer func() { BINS, machOS = oldB, oldOS }()
	BINS, machOS = []binUnit{{"hello", "macos"}, {"hello", "lx"}}, map[string]string{}
	rel := &relInfo{c12: checked[:12], src: "s"}
	var warns []string
	release(rel, "https://github.com/test/pub.git", checked, func(s string) { warns = append(warns, s) })
	if rel.tag != "" || exists(hct+"/rel/bin-"+checked[:12]+"/SHA256SUMS") {
		t.Errorf("대상 OS를 모르는데 명세를 올렸다: %+v", rel)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "OS를 모른다(lx") {
		t.Errorf("보류 경고: %v", warns)
	}
	if u := unknownTargets([]binUnit{{"a", "all"}, {"b", "x,y"}, {"c", "y"}}, map[string]string{"x": "Linux"}); strings.Join(u, ",") != "y" {
		t.Errorf("unknownTargets: %v", u)
	}
}
