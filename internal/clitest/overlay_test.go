package clitest

// 변이용 손잡이 HERDR_BUILD_OVERLAY(2026-11-24 contract-subtests): 옛 build는 바깥 빌드 옵션을 비워(GOFLAGS=) -overlay 변이가 지은
// 도구에 닿지 않았다 — 변이 확인마다 저장소 사본을 만들어야 했다.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// overlay로 더한 파일(디스크에 없는 init)이 지은 도구에 들고, 손잡이가 없으면 안 든다. 상대 경로는 짓지 않고 오류
func TestBuildOverlay(t *testing.T) {
	root, d := Root(t), TempDir(t)
	mut := filepath.Join(d, "mut.go")
	src := "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nfunc init() {\n\tif len(os.Args) > 1 && os.Args[1] == \"--version\" {\n\t\tfmt.Println(\"변이됨\")\n\t\tos.Exit(0)\n\t}\n}\n"
	if err := os.WriteFile(mut, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	ov, err := json.Marshal(map[string]map[string]string{"Replace": {filepath.Join(root, "cmd/herdr-tools-config/zz_mut.go"): mut}})
	if err != nil {
		t.Fatal(err)
	}
	ovp := filepath.Join(d, "overlay.json")
	if err := os.WriteFile(ovp, ov, 0o644); err != nil {
		t.Fatal(err)
	}
	version := func(overlay string) (string, error) {
		old := buildOverlay
		buildOverlay = overlay
		defer func() { buildOverlay = old }()
		out := filepath.Join(d, "tool-"+filepath.Base(overlay))
		if err := build(root, out, "herdr-tools-config", ""); err != nil {
			return "", err
		}
		b, _ := exec.Command(out, "--version").CombinedOutput() //nolint:errcheck // 출력만 본다
		return string(b), nil
	}
	if got, err := version(ovp); err != nil || !strings.Contains(got, "변이됨") {
		t.Errorf("overlay를 줬는데 변이가 안 들었다: %q %v", got, err)
	}
	if got, err := version(""); err != nil || strings.Contains(got, "변이됨") {
		t.Errorf("손잡이 없이 지은 도구에 변이가 들었다: %q %v", got, err)
	}
	if _, err := version("overlay.json"); err == nil || !strings.Contains(err.Error(), "절대 경로") {
		t.Errorf("상대 경로 overlay: 오류여야 한다, %v", err)
	}
}
