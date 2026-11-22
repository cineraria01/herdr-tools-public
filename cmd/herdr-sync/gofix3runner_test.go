//go:build !windows

package main

// gofix3-runner(재감사 2바퀴) ops/R-205: 맥 대체 빌드(installBins "맥" 갈래)에도 HERDR_TIMEOUT_BUILD 상한과 무리 소유 —
// 빌드 3초, 상한 1초면 모름으로 돌아오고 설치 0, 돌아온 뒤에도 본체·bin.tsv가 생기지 않는다

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestR205MacBuildBounded(t *testing.T) {
	home, _, _, src := instEnv(t)
	fb := t.TempDir()
	goSh := "#!/bin/sh\nsleep 3\nprev=\nfor a; do [ \"$prev\" = -o ] && printf '#!/bin/sh\\necho built\\n' >\"$a\"; prev=$a; done\nexit 0\n"
	for n, b := range map[string]string{"go": goSh, "uname": uname("arm64")} {
		if err := os.WriteFile(filepath.Join(fb, n), []byte(b), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", fb+":/usr/bin:/bin")
	t.Setenv("HERDR_TIMEOUT_BUILD", "1")
	oldB, oldC := BINS, check
	BINS, check = []binUnit{{"demo", "macos"}}, false
	t.Cleanup(func() { BINS, check = oldB, oldC })
	start := time.Now()
	_, ok := installBins("맥", "Darwin", &relInfo{c12: "0123456789ab", src: src})
	el := time.Since(start)
	if ok {
		t.Errorf("상한에 걸린 맥 대체 빌드를 「다 깔림」으로 읽었다 (ops/R-205)")
	}
	if el > 2800*time.Millisecond {
		t.Errorf("HERDR_TIMEOUT_BUILD=1인데 빌드 3초를 기다렸다: %v (ops/R-205)", el)
	}
	time.Sleep(time.Until(start.Add(5 * time.Second)))
	if got := readOr(home + "/.local/bin/demo"); got != oldBody {
		t.Errorf("상한 뒤 본체가 바뀌었다: %q", got)
	}
	if _, err := os.Stat(home + "/.local/state/herdr-sync/bin.tsv"); err == nil {
		t.Errorf("상한 뒤 bin.tsv가 생겼다 — 남은 자손이 설치를 끝냄")
	}
}
