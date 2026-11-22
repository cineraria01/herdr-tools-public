package main

// 리눅스로 갈 바이너리 고르기 — 2026-11-21 go-linux-dist(사용자 「난 다 고로 변경 하고 싶어」), 2026-11-22 바이너리 줄로(go-shell-remove).
// 리눅스 기계의 CPU 가드는 TestInstallShBuildFallback(다른 CPU면 받지 않음)

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// 리눅스 대상 기계가 있는 도구만: 바이너리 줄 대상이 all이거나 리눅스 기계 이름을 담을 때. macos·local·macOS 기계 이름만인 줄은 뺀다
func TestLinuxTools(t *testing.T) {
	bins := []binUnit{{"merge", "all"}, {"lane", "macos"}, {"fill", "local"}, {"foo", "box-m,box-l"}, {"bar", "box-m"}}
	tools := []string{"bar", "fill", "foo", "lane", "merge"}
	osOf := map[string]string{"box-m": "Darwin", "box-l": "Linux"}
	if got, want := linuxTools(tools, bins, osOf), []string{"foo", "merge"}; !reflect.DeepEqual(got, want) {
		t.Errorf("linuxTools = %v, 바람 %v", got, want)
	}
	if got := linuxTools(tools, bins[1:3], osOf); len(got) != 0 {
		t.Errorf("리눅스 대상이 없는데 %v", got)
	}
	// all 줄은 리눅스 기계가 이번에 안 닿아도 짓는다(SHA256SUMS가 닿은 기계에 따라 바뀌지 않게)
	if got, want := linuxTools(tools, bins, map[string]string{"box-m": "Darwin"}), []string{"merge"}; !reflect.DeepEqual(got, want) {
		t.Errorf("리눅스 기계를 모를 때 %v, 바람 %v", got, want)
	}
}

// 기계에 갈 도구: 대상이 덮는 바이너리 줄의 도구, 이름 순·겹침 없이
func TestBinTools(t *testing.T) {
	defer func(b []binUnit) { BINS = b }(BINS)
	BINS = []binUnit{{"merge", "all"}, {"lane", "macos"}, {"fill", "local"}, {"lane", "box-l"}}
	if got, want := binTools("맥", "Darwin"), []string{"fill", "lane", "merge"}; !reflect.DeepEqual(got, want) {
		t.Errorf("맥: %v", got)
	}
	if got, want := binTools("box-l", "Linux"), []string{"lane", "merge"}; !reflect.DeepEqual(got, want) {
		t.Errorf("box-l: %v", got)
	}
}

// 리눅스 대상이 있는 도구 하나를 릴리스 진입점(release)으로 보내 자산 이름·ELF 형식·명세의 해시를 보고, 리눅스 기계 깔기(installBins)가
// 받은 바이트까지 본다 — 대상 고르기·깔기 조각만 보는 시험은 릴리스 본문에서 리눅스 빌드 줄을 빼도 통과했다(재감사 R-37)
func TestReleaseLinuxAsset(t *testing.T) {
	gob := goBin()
	if gob == "" || testing.Short() {
		t.Skip("go 없음 또는 -short")
	}
	gc, err := exec.Command(gob, "env", "GOCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOCACHE", strings.TrimSpace(string(gc))) // 데운 캐시로(교차 빌드도 짧게)
	hct, _ := ghEnv(t)
	src := map[string]string{
		"go.mod":                      "module herdr-tools\n\ngo 1.24\n",
		"cmd/hello/main.go":           "package main\n\nimport (\n\t\"fmt\"\n\n\t\"herdr-tools/internal/version\"\n)\n\nfunc main() { fmt.Println(\"hello\", version.Commit) }\n",
		"internal/version/version.go": "package version\n\nvar Commit = \"dev\"\n",
	}
	REPO, STATE = newRepo(t), t.TempDir()
	lxh := t.TempDir()
	for p, c := range src {
		for _, d := range []string{REPO, lxh + "/.local/lib/herdr-go"} {
			os.MkdirAll(filepath.Dir(d+"/"+p), 0o755)
			os.WriteFile(d+"/"+p, []byte(c), 0o644)
		}
	}
	gitT(t, REPO, "add", "-A")
	gitT(t, REPO, "commit", "-qm", "go")
	checked := gitT(t, REPO, "rev-parse", "HEAD")
	// 가짜 hc: exec … --on lx -- <명령> → 리눅스 기계 HOME에서(uname -m은 x86_64)
	fb := t.TempDir()
	os.WriteFile(fb+"/uname", []byte("#!/bin/sh\ncase $1 in -m) echo x86_64 ;; *) echo Linux ;; esac\n"), 0o755)
	os.WriteFile(hct+"/hc", []byte("#!/bin/sh\nwhile [ \"$1\" != -- ]; do shift; done; shift\nHOME="+lxh+" PATH="+fb+":$PATH exec \"$@\"\n"), 0o755)
	t.Setenv("HERDR_SYNC_RELEASE_URL", "file://"+hct+"/rel")
	oldB, oldOS := BINS, machOS
	defer func() { BINS, machOS = oldB, oldOS }()
	BINS = []binUnit{{"hello", "all"}}
	machOS = map[string]string{"lx": "Linux"}
	sd, err := srcDigest(REPO)
	if err != nil {
		t.Fatal(err)
	}
	rel := &relInfo{c12: checked[:12], src: sd}
	var warns []string
	release(rel, "https://github.com/test/pub.git", checked, func(s string) { warns = append(warns, s) })
	if len(warns) != 0 {
		t.Fatalf("경고: %v", warns)
	}
	if !rel.lx["hello"] || rel.tag != "bin-"+checked[:12] {
		t.Fatalf("리눅스 받을 곳이 비었다: %+v", rel)
	}
	dir := hct + "/rel/bin-" + checked[:12]
	b, err := os.ReadFile(dir + "/hello-linux-amd64")
	if err != nil || !bytes.HasPrefix(b, []byte("\x7fELF")) {
		t.Fatalf("리눅스 자산이 없거나 ELF가 아니다: %v", err)
	}
	h := sha256.Sum256(b)
	if sums := readOr(dir + "/SHA256SUMS"); !strings.Contains(sums, hex.EncodeToString(h[:])+"  hello-linux-amd64\n") {
		t.Errorf("명세에 리눅스 자산 해시가 없다: %q", sums)
	}
	if _, ok := installBins("lx", "Linux", rel); !ok {
		t.Errorf("리눅스 기계 깔기가 실패했다")
	}
	if got, err := os.ReadFile(lxh + "/.local/bin/hello"); err != nil || !bytes.Equal(got, b) {
		t.Errorf("리눅스 기계가 받은 파일이 자산과 다르다: %v", err)
	}
}
