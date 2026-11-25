package merger_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
)

// hc-merger status(2026-11-26 ht-merger-status-all): --repo 없이 부르면 설정 「merger <프로젝트> <기계>」 줄마다 병합기 한 줄(hc-fill이
// run 바퀴에 적은 기록, 없으면 「기록 없음」)을 보이고 0 — 자가 점검이 「--repo 이 필요하다」 die를 결정 필요로 올렸다. 줄이 없으면 그 뜻 한 줄.
// --repo가 있으면(상주 병합기) 지금 그대로 — 다른 계약 시험이 본다. 인자 없음은 지금처럼 「--repo 이 필요하다」 종료 1
func TestMergerStatusAll(t *testing.T) {
	T := clitest.TempDir(t)
	clitest.Isolate(t, T)
	bd := clitest.BinDir(t, "hc-merger", "hc-fill")
	cfg, data := filepath.Join(T, "cfg"), filepath.Join(T, "data")
	for _, d := range []string{cfg, data} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	wr := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{"HOME=" + T, "PATH=" + bd + ":" + os.Getenv("PATH"), "HERDR_TOOLS_CONFIG=" + cfg, "HC_FILL_DATA=" + data, "XDG_STATE_HOME=" + T + "/st"}
	run := func(args ...string) clitest.Res {
		return clitest.Cmd{Env: env}.Run(filepath.Join(bd, "hc-merger"), args...)
	}

	wr(cfg+"/hc-fill.tsv", "local here\n")
	if r := run("status"); r.RC != 0 || r.Out != "병합기 없음 — 설정에 merger 줄이 없다" {
		t.Fatalf("줄 없음: 종료 %d 출력 %q 오류 %q", r.RC, r.Out, r.Err)
	}

	wr(cfg+"/hc-fill.tsv", "local here\nmerger app box-1 /c/app\nmerger web here\n")
	wr(data+"/merger-alive.tsv", "app\tbox-1\talive\t1\tbox-1/t7\t2\t-\t1\t-\t-\t-\n")
	r := run("status")
	l := strings.Split(r.Out, "\n")
	if r.RC != 0 || len(l) != 2 || !strings.HasPrefix(l[0], "병합기 app box-1: 살아 있음(box-1/t7) ") || !strings.Contains(l[0], "큐 2건") ||
		l[1] != "병합기 web here: 기록 없음(hc-fill run 바퀴가 아직 안 봄)" {
		t.Fatalf("전체 목록: 종료 %d 출력 %q 오류 %q", r.RC, r.Out, r.Err)
	}

	if r := run(); r.RC != 1 || !strings.Contains(r.Err, "--repo <병합기 전용 클론> 이 필요하다") {
		t.Fatalf("인자 없음은 그대로: 종료 %d 오류 %q", r.RC, r.Err)
	}

	// 감사 atp2plk 낮음 1: PATH 앞의 옛 판 hc-fill(merger status를 모름)이 아니라 곁의 hc-fill(배포가 함께 맞춘 판)을 부른다
	old := filepath.Join(T, "oldbin")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old+"/hc-fill", []byte("#!/bin/sh\necho 옛 판 >&2\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env[1] = "PATH=" + old + ":" + bd + ":" + os.Getenv("PATH")
	if r := run("status"); r.RC != 0 || len(strings.Split(r.Out, "\n")) != 2 {
		t.Fatalf("곁의 hc-fill: 종료 %d 출력 %q 오류 %q", r.RC, r.Out, r.Err)
	}
}
