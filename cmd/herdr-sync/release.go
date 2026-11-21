package main

// 릴리스: Go 판 도구 바이너리를 맥에서 한 번 지어 공개 저장소 릴리스로 올리고, 기계는 받아 쓴다
// (2026-11-21 사용자 「각 컴퓨터 마다 빌드 하지말고 github 에다가 빌드 한것도 처리해서 차라리 각 컴퓨터에서는 받아서 하면 어때 ? <앱 저장소> 과 같은 방식으로」 — 이름은 개인 값이라 가림).
//   - 빌드: herdr-tools-publish가 개인 값·비밀 꼴을 검사하고 내보낸 바로 그 비공개 커밋(HERDR_TOOLS_PUBLISH_RESULT로 받는다)을 git archive로
//     꺼내 그 트리에서 cmd/* 를 darwin/arm64로, 리눅스 대상 기계가 있는 도구(linuxTools)는 linux/amd64로도(-trimpath, CGO_ENABLED=0, GOTOOLCHAIN=local, 커밋을 -ldflags -X로 새김, 상한 HERDR_TIMEOUT_BUILD 600초).
//     그 커밋의 트리가 범용 저장소 HEAD(기계에 깐 소스)와 다르면 이번에는 올리지 않는다(gofix-ops A02·O-02 — 검사한 소스 = 지은 소스 = 깐 소스)
//   - 올리기: publish.repo의 릴리스 bin-<커밋 12자>에 도구별 바이너리(<도구> = darwin/arm64, <도구>-linux-amd64) + SHA256SUMS 하나(gh). 이미 올린 자산은 덮지 않는다(--clobber 없음 — gh는 덮을 때
//     먼저 지운다, A05): 같은 SHA256SUMS면 빠진 자산만 채우고, 다르면 크게 경고하고 둔다. 릴리스 조회가 「없음」이 아닌 오류(인증·통신·시간 넘김)면 모름 — 건너뜀
//   - 받기: macOS 기계(맥 포함)와 리눅스 기계(x86_64만 — installFor, 2026-11-21 go-linux-dist 사용자 「난 다 고로 변경 하고 싶어」)마다 맥이 계산한 sha256과 소스 판(srcDigest)을 넘기고, 그 기계가 공개 릴리스를 토큰 없이 받아
//     해시가 같을 때만 ~/.local/lib/herdr-go/.bin/<도구>에 바꿔 넣고 곁에 <도구>.src(소스 판)를 적는다(install). 다르면 쓰지 않고 크게 경고
//   - lib/go-run.sh: <도구>.src가 지금 소스 판과 같으면 받은 것을 쓰고(빌드 안 함), 다르면 그 기계에서 빌드, 그것도 안 되면 셸 판
// 로그에는 publish.repo 주소 원문을 찍지 않는다(설정 키 이름만 — 주소에 인증값이 들 수 있다, A12).
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
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/cksum"
	"herdr-tools/internal/proc"
)

// install: 기계에서 받아 바꿔 넣는 조각(POSIX sh — 리눅스 dash도). 인자: (주소 sha256 도구 소스판)…, 도구마다 정확히 한 줄
// W(씀)·S(같음)·E(까닭 — 옛 판 그대로)·K(건너뜀 — 그 기계에 소스가 없거나 그 기계 소스 판이 보낸 판과 다름: 받은 바이너리를 go-run이 쓰지 않을 테니 바꾸지 않는다, C02).
// 바꿔 넣기(A03): 받은 것을 sha256으로 확인한 뒤 옛 바이너리·옛 .src를 곁에 하드 링크로 걸어 두고, .src 지움 → 바이너리 → .src 순으로 바꾼다.
// 어느 단계에서 실패하든 걸어 둔 옛 판으로 되돌린다(바이너리 먼저 — 그사이 「옛 바이너리 + .src 없음」은 go-run이 판 모름으로 보고 다시 짓는다).
// 이 기계 소스 판(cur)은 lib/go-run.sh go_run_src와 같은 계산(srcDigest와 같은 값 — TestInstallSourceMismatch가 본다)
const install = `g=$HOME/.local/lib/herdr-go
d=$g/.bin
if [ ! -f "$g/go.mod" ]; then
  while [ $# -ge 4 ]; do printf 'K\t%s\t그 기계에 Go 소스가 없다\n' "$3"; shift 4; done
  exit 0
fi
mkdir -p "$d" || exit 1
sha() { if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1"; else sha256sum "$1"; fi | awk '{print $1}'; }
cur=$(cd "$g" && find go.mod cmd internal -type f ! -path '*/.*' 2>/dev/null | LC_ALL=C sort | tr '\n' '\0' | xargs -0 cksum | cksum | awk '{print $1"."$2}')
while [ $# -ge 4 ]; do
  url=$1 sum=$2 t=$3 src=$4; shift 4
  if [ "$cur" != "$src" ]; then printf 'K\t%s\t그 기계 소스 판이 다르다\n' "$t"; continue; fi
  if [ -x "$d/$t" ] && [ "$(cat "$d/$t.src" 2>/dev/null)" = "$src" ] && [ "$(sha "$d/$t")" = "$sum" ]; then printf 'S\t%s\n' "$t"; continue; fi
  f=$d/.$t.dl.$$ s=$d/.$t.src.$$ o=$d/.$t.old.$$ os=$d/.$t.oldsrc.$$
  if ! curl -fsSL --max-time 300 -o "$f" "$url"; then rm -f "$f"; printf 'E\t%s\t받지 못했다\n' "$t"; continue; fi
  if [ "$(sha "$f")" != "$sum" ]; then rm -f "$f"; printf 'E\t%s\t해시가 다르다 — 쓰지 않음\n' "$t"; continue; fi
  rm -f "$o" "$os"
  if ! { chmod 755 "$f" && printf '%s\n' "$src" > "$s" && { [ ! -e "$d/$t" ] || ln "$d/$t" "$o"; } && { [ ! -e "$d/$t.src" ] || ln "$d/$t.src" "$os"; }; }; then
    rm -f "$f" "$s" "$o" "$os"; printf 'E\t%s\t준비하지 못했다\n' "$t"; continue
  fi
  if rm -f "$d/$t.src" && mv -f "$f" "$d/$t" && mv -f "$s" "$d/$t.src"; then
    rm -f "$o" "$os"; printf 'W\t%s\n' "$t"
  else
    rm -f "$d/$t.src"
    if [ -e "$o" ]; then ln -f "$o" "$d/$t"; else rm -f "$d/$t"; fi
    [ ! -e "$os" ] || ln -f "$os" "$d/$t.src"
    rm -f "$f" "$s" "$o" "$os"; printf 'E\t%s\t바꿔 넣지 못했다 — 옛 판으로 되돌림\n' "$t"
  fi
done
`

// lxGuard: 리눅스 받기 앞머리 — 올리는 리눅스 바이너리는 amd64뿐이라 다른 구조면 받지 않고 도구마다 K 한 줄(go-run은 그 기계에서 짓거나 셸 판)
const lxGuard = `case $(uname -m) in
  x86_64|amd64) ;;
  *) while [ $# -ge 4 ]; do printf 'K\t%s\t리눅스 amd64가 아니다\n' "$3"; shift 4; done; exit 0 ;;
esac
`

// installFor: 그 기계 uname(-s)에 맞는 받기 조각
func installFor(osname string) string {
	if osname == "Linux" {
		return lxGuard + install
	}
	return install
}

// linuxTools: tools 가운데 linux/amd64로도 지을 도구 — 범용 저장소(repo)의 bin/<도구> 줄 대상이 리눅스 기계에 닿을 때(all이거나
// osOf가 Linux라고 아는 기계 이름). 이름을 박지 않고 sync.list에서 고른다(all 도구가 늘어도 따라온다).
// ponytail: 이름으로 리눅스 기계를 고른 줄은 그 기계가 이번에 닿아야 알아본다 — 못 닿은 날 지은 SHA256SUMS와 달라 그 커밋 릴리스를
// 덮지 않고 건너뛸 수 있다(다음 커밋에서 맞음). 지금 리눅스로 가는 도구 줄은 all뿐이다. 늘면 기계 OS를 상태에 적어 둔다
func linuxTools(tools []string, units []unit, repo string, osOf map[string]string) []string {
	var lx []string
	for m, o := range osOf {
		if o == "Linux" {
			lx = append(lx, m)
		}
	}
	want := map[string]bool{}
	for _, u := range units {
		t, ok := strings.CutPrefix(u.r, "bin/")
		if u.tree || u.d != repo || !ok || strings.Contains(t, "/") {
			continue
		}
		if u.t == "all" {
			want[t] = true
			continue
		}
		for _, m := range lx {
			if u.t != "macos" && u.t != "local" && targets(u.t, m, "Linux") {
				want[t] = true
			}
		}
	}
	var r []string
	for _, t := range tools {
		if want[t] {
			r = append(r, t)
		}
	}
	return r
}

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

// gh: 표준 출력·종료 코드·표준 오류(그대로도 흘린다). 상한(proc.Limit — gh는 HERDR_TIMEOUT_REMOTE)에 걸리면 124 = 모름
func gh(args ...string) (string, int, string) {
	var eb bytes.Buffer
	c := exec.Command("gh", args...)
	c.Stderr = io.MultiWriter(os.Stderr, &eb)
	out, rc := output(c)
	return out, rc, eb.String()
}

// buildMax: 릴리스 빌드 하나의 상한(A09) — 넘으면 그 무리째(컴파일러 자손까지) 끄고 이번 릴리스를 보류한다
func buildMax() time.Duration {
	if n, err := strconv.Atoi(os.Getenv("HERDR_TIMEOUT_BUILD")); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 600 * time.Second
}

var hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

// release: checked = herdr-tools-publish가 검사하고 내보낸 비공개 커밋(40자)
func release(pub, checked string, machines []string, pubwarn func(string)) {
	if !isFile(REPO + "/go.mod") {
		return
	}
	slug := githubSlug(pub)
	if slug == "" {
		warn("릴리스 건너뜀 — 설정 publish.repo가 GitHub 저장소가 아니다")
		return
	}
	gob := goBin()
	if gob == "" {
		pubwarn("릴리스 건너뜀 — 맥에 go가 없다(brew install go). 기계는 각자 빌드한다")
		return
	}
	if !hex40.MatchString(checked) {
		pubwarn("릴리스 건너뜀 — herdr-tools-publish가 검사한 커밋을 알려 주지 않았다(HERDR_TOOLS_PUBLISH_RESULT). 기계는 각자 빌드한다")
		return
	}
	ht, rc1 := gitq("-C", REPO, "rev-parse", "HEAD^{tree}")
	ct, rc2 := gitq("-C", REPO, "rev-parse", checked+"^{tree}")
	if rc1 != 0 || rc2 != 0 {
		pubwarn("릴리스 건너뜀 — 범용 저장소 커밋을 읽지 못했다")
		return
	}
	if ht != ct {
		pubwarn("릴리스 건너뜀 — 공개 검사한 커밋이 범용 저장소 HEAD(기계에 깐 소스)와 다르다(그사이 origin/main이 앞섬) — 다음 herdr-sync에서")
		return
	}
	c12, tag := checked[:12], "bin-"+checked[:12]
	srcd := TMP + "/relsrc"
	if err := os.MkdirAll(srcd, 0o700); err != nil {
		pubwarn("릴리스 건너뜀 — 임시 폴더")
		return
	}
	if _, rc := git("-C", REPO, "archive", "--format=tar", "-o", TMP+"/relsrc.tar", checked); rc != 0 {
		pubwarn("릴리스 건너뜀 — 검사한 커밋을 꺼내지 못했다(git archive)")
		return
	}
	x := exec.Command("tar", "-xf", TMP+"/relsrc.tar", "-C", srcd)
	x.Stderr = os.Stderr
	if run(x) != 0 {
		pubwarn("릴리스 건너뜀 — 검사한 커밋을 풀지 못했다(tar)")
		return
	}
	src, err := srcDigest(srcd)
	if err != nil {
		pubwarn("릴리스 건너뜀 — " + err.Error())
		return
	}
	var tools []string
	ents, err := os.ReadDir(srcd + "/cmd")
	if err != nil {
		pubwarn("릴리스 건너뜀 — cmd 폴더를 읽지 못했다")
		return
	}
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
	sums := map[string]string{} // 자산 이름 → sha256
	build := func(goos, goarch, t, name string) bool {
		b := exec.Command(gob, "build", "-trimpath", "-buildvcs=false", "-ldflags", "-X herdr-tools/internal/version.Commit="+c12,
			"-o", out+"/"+name, "./cmd/"+t)
		b.Dir = srcd
		b.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0", "GOTOOLCHAIN=local", "GOCACHE="+cache,
			"GOPATH="+STATE+"/gopath", "GOFLAGS=")
		b.Stdin, b.Stdout, b.Stderr = nil, os.Stderr, os.Stderr
		switch proc.WaitMax(b, "", buildMax()) {
		case 0:
		case proc.TimeoutCode:
			pubwarn("릴리스 빌드 시간 넘김 (" + name + ") — 이번 릴리스는 보류, 기계는 각자 빌드한다")
			return false
		default:
			pubwarn("릴리스 빌드 실패 (" + name + ") — 기계는 각자 빌드한다")
			return false
		}
		s, err := sha256File(out + "/" + name)
		if err != nil {
			pubwarn("릴리스 빌드 실패 (" + name + " sha256)")
			return false
		}
		sums[name] = s
		return true
	}
	assets := append([]string(nil), tools...)
	for _, t := range tools {
		if !build("darwin", "arm64", t, t) {
			return
		}
	}
	lxt := linuxTools(tools, U, REPO, machOS)
	for _, t := range lxt {
		if !build("linux", "amd64", t, t+"-linux-amd64") {
			return
		}
		assets = append(assets, t+"-linux-amd64")
	}
	var sb strings.Builder
	for _, a := range assets {
		fmt.Fprintf(&sb, "%s  %s\n", sums[a], a)
	}
	if os.WriteFile(out+"/SHA256SUMS", []byte(sb.String()), 0o644) != nil {
		pubwarn("릴리스 건너뜀 — SHA256SUMS를 쓰지 못했다")
		return
	}
	if !upload(slug, tag, c12, out, assets, sb.String(), pubwarn) {
		return
	}
	base := envOr("HERDR_SYNC_RELEASE_URL", "https://github.com/"+slug+"/releases/download")
	var args, lxArgs []string
	for _, t := range tools {
		args = append(args, base+"/"+tag+"/"+t, sums[t], t, src)
	}
	for _, t := range lxt {
		a := t + "-linux-amd64"
		lxArgs = append(lxArgs, base+"/"+tag+"/"+a, sums[a], t, src) // 받는 자리는 .bin/<도구> 그대로
	}
	put := func(m string, tools []string, c *exec.Cmd) {
		var errb bytes.Buffer
		c.Stderr = &errb
		o, rc := output(c)
		for _, p := range installReport(m, tag, tools, o) {
			if p.warn {
				pubwarn(p.s)
			} else {
				say("%s", p.s)
			}
		}
		if rc != 0 {
			pubwarn(fmt.Sprintf("%s 바이너리 받기 실패 (%s) %s", m, tag, strings.TrimSpace(errb.String())))
		}
	}
	put("맥", tools, exec.Command("sh", append([]string{"-c", install, "_"}, args...)...))
	for _, m := range machines {
		ts, a := tools, args
		switch machOS[m] {
		case "Darwin":
		case "Linux":
			if len(lxt) == 0 {
				continue
			}
			ts, a = lxt, lxArgs
		default:
			continue
		}
		put(m, ts, exec.Command("hc", append([]string{"exec", "--timeout", "600", "--on", m, "--", "sh", "-c", installFor(machOS[m]), "_"}, a...)...))
	}
}

// upload: 릴리스를 만들거나 빠진 자산을 채운다. 이미 올린 자산은 덮지 않는다(A05). 받기로 넘어가도 되면 참(릴리스가 이 빌드와 같다고 확인됨)
func upload(slug, tag, c12, out string, tools []string, sums string, pubwarn func(string)) bool {
	files := func(names []string) []string {
		var r []string
		for _, n := range names {
			r = append(r, out+"/"+n)
		}
		return r
	}
	all := append(append([]string(nil), tools...), "SHA256SUMS")
	names, rc, errs := gh("release", "view", tag, "--repo", slug, "--json", "assets", "--jq", ".assets[].name")
	if rc != 0 {
		if rc != proc.TimeoutCode && strings.Contains(strings.ToLower(errs), "not found") { // gh: 「release not found」 — 없음이 확인됨
			args := append([]string{"release", "create", tag, "--repo", slug, "--title", tag,
				"--notes", "herdr-tools " + c12 + " Go 판 도구 바이너리(<도구> darwin/arm64, <도구>-linux-amd64) — herdr-sync가 맥에서 지어 올림, 기계는 SHA256SUMS로 확인해 받는다"}, files(all)...)
			if _, rc, _ := gh(args...); rc != 0 {
				pubwarn("릴리스 만들기 실패 " + tag + " (gh release create — 맥 gh 인증·공개 미러 쓰기 권한) — 기계는 각자 빌드한다")
				return false
			}
			say("릴리스 올림 %s", tag)
			return true
		}
		pubwarn("릴리스 조회 실패 " + tag + " (gh release view — 인증·통신·시간 넘김, 있는지 모름) — 이번에는 올리지 않는다, 다음 herdr-sync에서")
		return false
	}
	have := map[string]bool{}
	for _, n := range splitLines(names) {
		have[n] = true
	}
	if have["SHA256SUMS"] {
		old := TMP + "/relold"
		os.MkdirAll(old, 0o700)
		if _, rc, _ := gh("release", "download", tag, "--repo", slug, "--pattern", "SHA256SUMS", "--dir", old, "--clobber"); rc != 0 {
			pubwarn("릴리스 확인 실패 " + tag + " (gh release download SHA256SUMS) — 이번에는 올리지도 받지도 않는다, 다음 herdr-sync에서")
			return false
		}
		b, err := os.ReadFile(old + "/SHA256SUMS")
		if err != nil {
			pubwarn("릴리스 확인 실패 " + tag + " (받은 SHA256SUMS를 읽지 못했다)")
			return false
		}
		if string(b) != sums {
			pubwarn("릴리스 " + tag + "의 SHA256SUMS가 이번 빌드와 다르다 — 덮지 않는다(올린 자산을 지우지 않게). 기계는 각자 빌드한다")
			return false
		}
	}
	var miss []string
	for _, n := range all {
		if !have[n] {
			miss = append(miss, n)
		}
	}
	if len(miss) == 0 {
		say("릴리스 같음 %s", tag)
		return true
	}
	if _, rc, _ := gh(append([]string{"release", "upload", tag, "--repo", slug}, files(miss)...)...); rc != 0 {
		pubwarn("릴리스 올리기 실패 " + tag + " (gh release upload — 맥 gh 인증·공개 미러 쓰기 권한) — 기계는 각자 빌드한다")
		return false
	}
	say("릴리스 채움 %s (%s)", tag, strings.Join(miss, " "))
	return true
}

type note struct {
	s    string
	warn bool
}

// installReport: install 조각 출력 → 알릴 줄. 보낸 도구마다 W·S·E·K 가운데 정확히 한 줄이어야 한다 — 빠짐·겹침·모르는 도구는 경고(C03)
func installReport(m, tag string, tools []string, out string) []note {
	want := map[string]bool{}
	for _, t := range tools {
		want[t] = true
	}
	seen := map[string]int{}
	var r []note
	for _, l := range splitLines(out) {
		a := strings.Split(l, "\t")
		if len(a) < 2 || !want[a[1]] {
			r = append(r, note{fmt.Sprintf("%s 바이너리 받기: 알 수 없는 응답 %q (%s)", m, l, tag), true})
			continue
		}
		seen[a[1]]++
		switch {
		case a[0] == "W":
			r = append(r, note{fmt.Sprintf("%s 받음 %s (%s)", m, a[1], tag), false})
		case a[0] == "S":
		case a[0] == "E" && len(a) > 2:
			r = append(r, note{fmt.Sprintf("%s %s 바이너리를 받지 않음: %s (%s)", m, a[1], a[2], tag), true})
		case a[0] == "K" && len(a) > 2:
			r = append(r, note{fmt.Sprintf("%s %s 바이너리 건너뜀: %s (%s)", m, a[1], a[2], tag), false})
		default:
			r = append(r, note{fmt.Sprintf("%s 바이너리 받기: 알 수 없는 응답 %q (%s)", m, l, tag), true})
		}
	}
	for _, t := range tools {
		switch {
		case seen[t] == 0:
			r = append(r, note{fmt.Sprintf("%s %s 바이너리 받기 응답이 없다 — 다음 herdr-sync에서 (%s)", m, t, tag), true})
		case seen[t] > 1:
			r = append(r, note{fmt.Sprintf("%s %s 바이너리 받기 응답이 %d번 — 다음 herdr-sync에서 다시 본다 (%s)", m, t, seen[t], tag), true})
		}
	}
	return r
}
