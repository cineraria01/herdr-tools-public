package main

// 릴리스: Go 판 도구 바이너리를 맥에서 한 번 지어 공개 저장소 릴리스로 올리고, 기계는 받아 쓴다
// (2026-11-21 사용자 「각 컴퓨터 마다 빌드 하지말고 github 에다가 빌드 한것도 처리해서 차라리 각 컴퓨터에서는 받아서 하면 어때 ? <앱 저장소> 과 같은 방식으로」 — 이름은 개인 값이라 가림).
//   - 빌드: 범용 저장소(깨끗한 main — ready·모음 뒤)의 cmd/* 를 darwin/arm64로(-trimpath, CGO_ENABLED=0, GOTOOLCHAIN=local,
//     커밋을 -ldflags -X로 새김). 빌드 입력은 범용 저장소만이라 개인 값이 들어가지 않는다(공개 전 검사는 herdr-tools-publish가 소스에)
//   - 올리기: publish.repo의 릴리스 bin-<커밋 12자>에 도구별 바이너리 + SHA256SUMS(gh). 같은 커밋 릴리스의 SHA256SUMS가 같으면 다시 안 올림
//   - 받기: macOS 기계(맥 포함)마다 맥이 계산한 sha256과 소스 판(srcDigest)을 넘기고, 그 기계가 공개 릴리스를 토큰 없이 받아
//     해시가 같을 때만 ~/.local/lib/herdr-go/.bin/<도구>에 원자 교체하고 곁에 <도구>.src(소스 판)를 적는다. 다르면 쓰지 않고 크게 경고
//   - lib/go-run.sh: <도구>.src가 지금 소스 판과 같으면 받은 것을 쓰고(빌드 안 함), 다르면 그 기계에서 빌드, 그것도 안 되면 셸 판
// 시험용: HERDR_SYNC_RELEASE_URL(받는 주소 앞부분, 기본 https://github.com/<주인/이름>/releases/download — file://도 됨)

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"herdr-tools/internal/cksum"
)

// install: 기계에서 받아 바꿔 넣는 조각(POSIX sh — 리눅스 dash도). 인자: (주소 sha256 도구 소스판)…, 도구마다 W(씀)·S(같음)·E(까닭) 한 줄
const install = `g=$HOME/.local/lib/herdr-go
[ -f "$g/go.mod" ] || exit 0
d=$g/.bin
mkdir -p "$d" || exit 1
sha() { if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1"; else sha256sum "$1"; fi | awk '{print $1}'; }
while [ $# -ge 4 ]; do
  url=$1 sum=$2 t=$3 src=$4; shift 4
  if [ -x "$d/$t" ] && [ "$(cat "$d/$t.src" 2>/dev/null)" = "$src" ] && [ "$(sha "$d/$t")" = "$sum" ]; then printf 'S\t%s\n' "$t"; continue; fi
  f=$d/.$t.dl.$$
  if ! curl -fsSL --max-time 300 -o "$f" "$url"; then rm -f "$f"; printf 'E\t%s\t받지 못했다\n' "$t"; continue; fi
  if [ "$(sha "$f")" != "$sum" ]; then rm -f "$f"; printf 'E\t%s\t해시가 다르다 — 쓰지 않음\n' "$t"; continue; fi
  if chmod 755 "$f" && rm -f "$d/$t.src" && mv -f "$f" "$d/$t" && printf '%s\n' "$src" > "$d/.$t.src.$$" && mv -f "$d/.$t.src.$$" "$d/$t.src"; then
    printf 'W\t%s\n' "$t"
  else rm -f "$f" "$d/.$t.src.$$"; printf 'E\t%s\t바꿔 넣지 못했다\n' "$t"; fi
done
`

var slugRe = regexp.MustCompile(`^(?:https://github\.com/|git@github\.com:|ssh://git@github\.com/)([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+?)(?:\.git)?/?$`)

// githubSlug: publish.repo가 GitHub 저장소면 「주인/이름」
func githubSlug(url string) string {
	if m := slugRe.FindStringSubmatch(url); m != nil {
		return m[1]
	}
	return ""
}

// srcDigest: 모듈 소스 판 — go.mod·cmd/**·internal/** 의 일반 파일 전부(go:embed 글 포함 — 임베드만 바뀐 판도 알아보게, gofix-core B10·M4)를
// 상대 경로 바이트 순으로 「crc 크기 경로」 줄로 적은 목록의 cksum(crc.크기). 숨김 파일·숨김 폴더 아래는 뺀다(herdr-sync 나무 줄이 걷지 않아 기계에 없다).
// lib/go-run.sh go_run_src(`find … ! -path '*/.*' | LC_ALL=C sort | xargs cksum | cksum`)와 같은 값
func srcDigest(dir string) (string, error) {
	var files []string
	for _, top := range []string{"go.mod", "cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(dir, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type().IsRegular() {
				rel, _ := filepath.Rel(dir, p)
				files = append(files, rel)
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("소스 걷기 %s: %w", top, err)
		}
	}
	sort.Strings(files)
	var b bytes.Buffer
	for _, f := range files {
		fh, err := os.Open(filepath.Join(dir, f))
		if err != nil {
			return "", fmt.Errorf("소스 읽기 %s: %w", f, err)
		}
		c, n, err := cksum.Reader(fh)
		fh.Close()
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%d %d %s\n", c, n, f)
	}
	return cksum.Dot(b.Bytes()), nil
}

func goBin() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	for _, p := range []string{"/opt/homebrew/bin/go", "/usr/local/go/bin/go", "/usr/local/bin/go"} {
		if isFile(p) {
			return p
		}
	}
	return ""
}

func sha256File(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func gh(args ...string) (string, int) {
	c := exec.Command("gh", args...)
	c.Stderr = os.Stderr
	return output(c)
}

func release(pub string, machines []string, pubwarn func(string)) {
	if !isFile(REPO + "/go.mod") {
		return
	}
	slug := githubSlug(pub)
	if slug == "" {
		warn("릴리스 건너뜀 — publish.repo가 GitHub 저장소가 아니다 (%s)", pub)
		return
	}
	gob := goBin()
	if gob == "" {
		pubwarn("릴리스 건너뜀 — 맥에 go가 없다(brew install go). 기계는 각자 빌드한다")
		return
	}
	commit, rc := gitq("-C", REPO, "rev-parse", "HEAD")
	if rc != 0 || len(commit) < 12 {
		pubwarn("릴리스 건너뜀 — 범용 저장소 커밋을 읽지 못했다")
		return
	}
	c12, tag := commit[:12], "bin-"+commit[:12]
	src, err := srcDigest(REPO)
	if err != nil {
		pubwarn("릴리스 건너뜀 — " + err.Error())
		return
	}
	var tools []string
	ents, _ := os.ReadDir(REPO + "/cmd")
	for _, e := range ents {
		if e.IsDir() {
			tools = append(tools, e.Name())
		}
	}
	if len(tools) == 0 {
		return
	}
	out := TMP + "/rel"
	if err := os.MkdirAll(out, 0o700); err != nil {
		pubwarn("릴리스 건너뜀 — 임시 폴더")
		return
	}
	cache := os.Getenv("GOCACHE")
	if cache == "" {
		cache = STATE + "/go-build" // 맥 한 곳에만 쌓인다(기계는 받아 쓰니 빌드 캐시가 없다)
	}
	sums := map[string]string{}
	for _, t := range tools {
		b := exec.Command(gob, "build", "-trimpath", "-buildvcs=false", "-ldflags", "-X herdr-tools/internal/version.Commit="+c12,
			"-o", out+"/"+t, "./cmd/"+t)
		b.Dir = REPO
		b.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0", "GOTOOLCHAIN=local", "GOCACHE="+cache,
			"GOPATH="+STATE+"/gopath", "GOFLAGS=")
		b.Stdout, b.Stderr = os.Stderr, os.Stderr
		if run(b) != 0 {
			pubwarn("릴리스 빌드 실패 (" + t + ") — 기계는 각자 빌드한다")
			return
		}
		s, err := sha256File(out + "/" + t)
		if err != nil {
			pubwarn("릴리스 빌드 실패 (" + t + " sha256)")
			return
		}
		sums[t] = s
	}
	var sb strings.Builder
	for _, t := range tools {
		fmt.Fprintf(&sb, "%s  %s\n", sums[t], t)
	}
	if os.WriteFile(out+"/SHA256SUMS", []byte(sb.String()), 0o644) != nil {
		pubwarn("릴리스 건너뜀 — SHA256SUMS를 쓰지 못했다")
		return
	}
	files := []string{}
	for _, t := range tools {
		files = append(files, out+"/"+t)
	}
	files = append(files, out+"/SHA256SUMS")
	if _, rc := gh("release", "view", tag, "--repo", slug, "--json", "tagName"); rc == 0 {
		old := TMP + "/relold"
		os.MkdirAll(old, 0o700)
		if _, rc := gh("release", "download", tag, "--repo", slug, "--pattern", "SHA256SUMS", "--dir", old, "--clobber"); rc == 0 {
			if b, err := os.ReadFile(old + "/SHA256SUMS"); err == nil && string(b) == sb.String() {
				say("릴리스 같음 %s", tag)
				goto installs
			}
		}
		if _, rc := gh(append([]string{"release", "upload", tag, "--repo", slug, "--clobber"}, files...)...); rc != 0 {
			pubwarn("릴리스 올리기 실패 " + tag + " (gh release upload — 맥 gh 인증·공개 미러 쓰기 권한) — 기계는 각자 빌드한다")
			return
		}
		say("릴리스 올림 %s (다시)", tag)
	} else {
		args := append([]string{"release", "create", tag, "--repo", slug, "--title", tag,
			"--notes", "herdr-tools " + c12 + " Go 판 도구 바이너리(darwin/arm64) — herdr-sync가 맥에서 지어 올림, 기계는 SHA256SUMS로 확인해 받는다"}, files...)
		if _, rc := gh(args...); rc != 0 {
			pubwarn("릴리스 만들기 실패 " + tag + " (gh release create — 맥 gh 인증·공개 미러 쓰기 권한) — 기계는 각자 빌드한다")
			return
		}
		say("릴리스 올림 %s", tag)
	}
installs:
	base := envOr("HERDR_SYNC_RELEASE_URL", "https://github.com/"+slug+"/releases/download")
	var args []string
	for _, t := range tools {
		args = append(args, base+"/"+tag+"/"+t, sums[t], t, src)
	}
	put := func(m string, c *exec.Cmd) {
		var errb bytes.Buffer
		c.Stderr = &errb
		o, rc := output(c)
		for _, l := range splitLines(o) {
			a := strings.Split(l, "\t")
			switch {
			case a[0] == "W" && len(a) > 1:
				say("%s 받음 %s (%s)", m, a[1], tag)
			case a[0] == "E" && len(a) > 2:
				pubwarn(fmt.Sprintf("%s %s 바이너리를 받지 않음: %s (%s)", m, a[1], a[2], tag))
			}
		}
		if rc != 0 {
			pubwarn(fmt.Sprintf("%s 바이너리 받기 실패 (%s) %s", m, tag, strings.TrimSpace(errb.String())))
		}
	}
	put("맥", exec.Command("sh", append([]string{"-c", install, "_"}, args...)...))
	for _, m := range machines {
		if machOS[m] != "Darwin" {
			continue
		}
		put(m, exec.Command("hc", append([]string{"exec", "--timeout", "600", "--on", m, "--", "sh", "-c", install, "_"}, args...)...))
	}
}
