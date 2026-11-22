package main

// 바이너리 깔기 조각(installSh)·릴리스 올리기(upload)·소스 판(srcDigest) 단위 시험 — 2026-11-22 go-shell-remove
// (옛 install 조각 시험 gofix-ops A03·T02·A04·O-03을 새 꼴로, 재감사 R-01·R-10·R-11·R-40).
// installSh를 임시 HOME에서 sh로 직접 돌린다(실제 기계·네트워크 없음, 받기는 file://).

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const oldBody = "#!/bin/sh\necho OLD\n"

// instEnv: 받는 자리(~/.local/bin/demo 옛 본체, ~/.local/lib/herdr-go 작은 Go 모듈 — cmd/demo는 새긴 커밋을 찍는다)와 새 산출물.
// 돌려주는 것: HOME, 새 산출물 경로(file:// 주소로), 그 sha256, 그 기계 소스 판
func instEnv(t *testing.T) (home, art, sum, src string) {
	t.Helper()
	home = t.TempDir()
	g := home + "/.local/lib/herdr-go"
	for p, c := range map[string]string{
		g + "/go.mod":                      "module herdr-tools\n\ngo 1.21\n",
		g + "/cmd/demo/main.go":            "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n\n\t\"herdr-tools/internal/version\"\n)\n\nfunc main() {\n\tif len(os.Args) > 1 {\n\t\tfmt.Println(\"demo\")\n\t\treturn\n\t}\n\tfmt.Println(\"built\", version.Commit)\n}\n",
		g + "/internal/version/version.go": "package version\n\nvar Commit string\n",
		home + "/.local/bin/demo":          oldBody,
	} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(c), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	art = t.TempDir() + "/demo"
	nb := []byte("#!/bin/sh\necho NEW\n")
	if err := os.WriteFile(art, nb, 0o644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(nb)
	s, err := srcDigest(g)
	if err != nil {
		t.Fatal(err)
	}
	return home, art, hex.EncodeToString(h[:]), s
}

// runSh: installSh를 돌린다. fake: PATH 앞에 둘 가짜 명령(이름 → 본문). 표준 출력(S·W·B·E·D 줄)만 돌려준다
func runSh(t *testing.T, home string, fake map[string]string, args ...string) string {
	t.Helper()
	fb := t.TempDir()
	for n, b := range fake {
		if err := os.WriteFile(fb+"/"+n, []byte(b), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	c := exec.Command("sh", append([]string{"-c", installSh, "_"}, args...)...)
	c.Env = append(os.Environ(), "HOME="+home, "PATH="+fb+":"+os.Getenv("PATH"))
	if gc := os.Getenv("GOCACHE"); gc != "" {
		c.Env = append(c.Env, "GOCACHE="+gc)
	}
	out, _ := c.Output()
	return string(out)
}

func readOr(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return "<없음>"
	}
	return string(b)
}

// leftovers: ~/.local/bin에 남은 임시 파일(.demo.new.* 등)
func leftovers(home string) []string {
	m, _ := filepath.Glob(home + "/.local/bin/.*")
	return m
}

func uname(m string) string {
	return "#!/bin/sh\ncase $1 in -m) echo " + m + " ;; *) echo Darwin ;; esac\n"
}

// 받기: 해시가 맞으면 본체 하나를 mv 한 번으로, 기록을 남기고, 다음 실행은 S. --check는 아무것도 쓰지 않는다
func TestInstallShDownload(t *testing.T) {
	home, art, sum, src := instEnv(t)
	arm := map[string]string{"uname": uname("arm64")}
	if out := runSh(t, home, arm, "check", "c0", src, "arm64", "demo", "-", "-"); out != "D\tdemo\t판이 다르거나 모름\n" {
		t.Fatalf("깔기 전 --check: %q", out)
	}
	if readOr(home+"/.local/bin/demo") != oldBody || readOr(home+"/.local/state/herdr-sync/bin.tsv") != "<없음>" {
		t.Fatal("--check가 썼다")
	}
	if out := runSh(t, home, arm, "set", "c0", src, "arm64", "demo", "file://"+art, sum); out != "W\tdemo\n" {
		t.Fatalf("받기: %q", out)
	}
	if readOr(home+"/.local/bin/demo") != "#!/bin/sh\necho NEW\n" || readOr(home+"/.local/state/herdr-sync/bin.tsv") != "demo\t"+sum+"\t"+src+"\n" {
		t.Fatalf("본체·기록: %q %q", readOr(home+"/.local/bin/demo"), readOr(home+"/.local/state/herdr-sync/bin.tsv"))
	}
	if st, _ := os.Stat(home + "/.local/bin/demo"); st.Mode()&0o111 == 0 {
		t.Error("실행 권한 없음")
	}
	if out := runSh(t, home, arm, "set", "c0", src, "arm64", "demo", "file://"+art, sum); out != "S\tdemo\n" {
		t.Errorf("다시: %q", out)
	}
	if out := runSh(t, home, arm, "check", "c0", src, "arm64", "demo", "-", "-"); out != "S\tdemo\n" {
		t.Errorf("깐 뒤 --check(기록으로 알아봄): %q", out)
	}
	if l := leftovers(home); len(l) != 0 {
		t.Errorf("임시 파일이 남음: %v", l)
	}
	// 소스 판이 바뀌면(새 커밋) 기록과 어긋나 다시 깐다
	if out := runSh(t, home, arm, "check", "c0", src+"x", "arm64", "demo", "-", "-"); !strings.HasPrefix(out, "D\tdemo\t") {
		t.Errorf("소스 판이 다른 --check: %q", out)
	}
}

// sync-hc-timeout: 끝 시각(budget)이 지나면 받지도 짓지도 않고 E 「시간 상한」, 옛 본체 그대로. 시각은 가짜 date가 실행마다 1000초씩 민다
func TestInstallShBudget(t *testing.T) {
	home, art, sum, src := instEnv(t)
	fb := t.TempDir()
	os.WriteFile(fb+"/uname", []byte(uname("arm64")), 0o755)
	os.WriteFile(fb+"/date", []byte("#!/bin/sh\nf="+fb+"/n; n=$(cat $f 2>/dev/null || echo 0); echo $((n+1000)) > $f; echo $((n+1000))\n"), 0o755)
	c := exec.Command("sh", "-c", budgetSh(500), "_", "set", "c0", src, "arm64", "demo", "file://"+art, sum)
	c.Env = append(os.Environ(), "HOME="+home, "PATH="+fb+":"+os.Getenv("PATH"))
	out, _ := c.Output()
	if string(out) != "E\tdemo\t시간 상한 — 다음 herdr-sync에서\n" || readOr(home+"/.local/bin/demo") != oldBody || len(leftovers(home)) != 0 {
		t.Fatalf("시간 상한: %q 본체 %q 남은 것 %v", out, readOr(home+"/.local/bin/demo"), leftovers(home))
	}
	// 시각 안이면 그대로 받는다
	c = exec.Command("sh", "-c", budgetSh(5000), "_", "set", "c0", src, "arm64", "demo", "file://"+art, sum)
	c.Env = append(os.Environ(), "HOME="+home, "PATH="+fb+":"+os.Getenv("PATH"))
	if out, _ := c.Output(); string(out) != "W\tdemo\n" {
		t.Fatalf("시각 안: %q", out)
	}
}

// R-01·R-11: 바꿔 넣기가 안 되면 옛 본체 그대로·임시 파일 없음(되돌릴 백업도 지울 백업도 없다 — 본체 하나가 한 세대).
// 기록 쓰기만 실패하면 본체는 새 것이고 다음 실행이 알아본다. 기록이 본체와 어긋나면(쓰다 죽음) 판 모름으로 보고 다시 깐다
func TestInstallShOneGeneration(t *testing.T) {
	home, art, sum, src := instEnv(t)
	failTo := func(suffix string) map[string]string {
		return map[string]string{"uname": uname("arm64"),
			"mv": "#!/bin/sh\nfor a; do last=$a; done\ncase $last in *" + suffix + ") echo 'mv: 막음' >&2; exit 1 ;; esac\nexec /bin/mv \"$@\"\n"}
	}
	if out := runSh(t, home, failTo("/.local/bin/demo"), "set", "c0", src, "arm64", "demo", "file://"+art, sum); out != "E\tdemo\t바꿔 넣지 못했다 — 옛 본체 그대로\n" {
		t.Fatalf("mv 실패: %q", out)
	}
	if readOr(home+"/.local/bin/demo") != oldBody || len(leftovers(home)) != 0 || readOr(home+"/.local/state/herdr-sync/bin.tsv") != "<없음>" {
		t.Fatalf("mv 실패 뒤: 본체 %q 남은 것 %v 기록 %q", readOr(home+"/.local/bin/demo"), leftovers(home), readOr(home+"/.local/state/herdr-sync/bin.tsv"))
	}
	out := runSh(t, home, failTo("/bin.tsv"), "set", "c0", src, "arm64", "demo", "file://"+art, sum)
	if out != "W\tdemo\t기록을 쓰지 못했다\n" || readOr(home+"/.local/bin/demo") != "#!/bin/sh\necho NEW\n" {
		t.Fatalf("기록 실패: %q %q", out, readOr(home+"/.local/bin/demo"))
	}
	if out := runSh(t, home, map[string]string{"uname": uname("arm64")}, "set", "c0", src, "arm64", "demo", "file://"+art, sum); out != "S\tdemo\n" ||
		readOr(home+"/.local/state/herdr-sync/bin.tsv") != "demo\t"+sum+"\t"+src+"\n" {
		t.Fatalf("다음 실행이 기록을 채움: %q %q", out, readOr(home+"/.local/state/herdr-sync/bin.tsv"))
	}
	// 어긋난 기록(본체 sha가 다름): S가 아니다
	os.WriteFile(home+"/.local/bin/demo", []byte(oldBody), 0o755)
	if out := runSh(t, home, nil, "check", "c0", src, "arm64", "demo", "-", "-"); !strings.HasPrefix(out, "D\tdemo\t") {
		t.Errorf("본체가 기록과 다른데 --check: %q", out)
	}
}

// 받기가 안 되면(해시 다름·다른 CPU·릴리스 없음) 그 기계 소스 판이 범용 저장소와 같을 때만 그 기계에서 짓는다 — 판 모를 바이너리를 깔지 않는다(R-40)
func TestInstallShBuildFallback(t *testing.T) {
	if goBin() == "" {
		t.Skip("go가 없다")
	}
	home, art, _, src := instEnv(t)
	arm := map[string]string{"uname": uname("arm64")}
	if out := runSh(t, home, arm, "set", "c0", src+"x", "arm64", "demo", "file://"+art, "bad"); out != "E\tdemo\t받은 것의 해시가 다르다 · 그 기계 Go 소스 판이 범용 저장소와 다르다\n" {
		t.Fatalf("해시 다름·소스 다름: %q", out)
	}
	if readOr(home+"/.local/bin/demo") != oldBody || len(leftovers(home)) != 0 {
		t.Fatalf("못 깖 뒤: %q %v", readOr(home+"/.local/bin/demo"), leftovers(home))
	}
	if out := runSh(t, home, map[string]string{"uname": uname("aarch64")}, "set", "c0", src+"x", "x86_64", "demo", "file://"+art, "bad"); !strings.HasPrefix(out, "E\tdemo\tCPU가 aarch64 · ") {
		t.Errorf("다른 CPU: %q", out)
	}
	out := runSh(t, home, arm, "set", "0123456789ab", src, "arm64", "demo", "-", "-")
	if out != "B\tdemo\t릴리스 없음\n" {
		t.Fatalf("짓기: %q", out)
	}
	if got, err := exec.Command(home + "/.local/bin/demo").Output(); err != nil || string(got) != "built 0123456789ab\n" {
		t.Errorf("지은 본체가 커밋을 새기지 않음: %q %v", got, err)
	}
	if out := runSh(t, home, arm, "check", "0123456789ab", src, "arm64", "demo", "-", "-"); out != "S\tdemo\n" {
		t.Errorf("지은 것도 기록으로 알아봄: %q", out)
	}
	if l := leftovers(home); len(l) != 0 {
		t.Errorf("임시 파일이 남음: %v", l)
	}
}

// installSh의 그 기계 소스 판(cur)은 srcDigest와 같아야 한다 — 숨김 파일은 빼고, 임베드 글은 넣는다
func TestInstallShSourceDigest(t *testing.T) {
	if goBin() == "" {
		t.Skip("go가 없다")
	}
	home, _, _, _ := instEnv(t)
	g := home + "/.local/lib/herdr-go"
	os.WriteFile(g+"/cmd/demo/agent.sh", []byte("echo a\n"), 0o644)
	os.MkdirAll(g+"/.bin", 0o755)
	os.WriteFile(g+"/.bin/junk", []byte("x"), 0o644)
	src, _ := srcDigest(g)
	if out := runSh(t, home, map[string]string{"uname": uname("arm64")}, "set", "c0", src, "arm64", "demo", "-", "-"); out != "B\tdemo\t릴리스 없음\n" {
		t.Errorf("srcDigest와 다른 판으로 본다: %q", out)
	}
}

// 보낸 도구마다 정확히 한 줄 — 빠짐·겹침·모르는 도구·모르는 꼴은 못 깖(감사 C03)
func TestInstallReport(t *testing.T) {
	bad := func(out string) (n int) {
		for _, p := range installReport("w1", []string{"a", "b"}, out) {
			if p.bad {
				n++
			}
		}
		return
	}
	for out, want := range map[string]int{
		"W\ta\nS\tb\n": 0, "B\ta\t릴리스 없음\nS\tb\n": 0, "W\ta\t기록을 쓰지 못했다\nS\tb\n": 0,
		"S\ta\n": 1, "S\ta\nS\ta\nS\tb\n": 1, "S\ta\nS\tb\nW\tc\n": 1, "E\ta\t받지 못했다\nS\tb\n": 1,
		"K\ta\t옛 꼴\nS\tb\n": 1, "B\ta\nS\tb\n": 1, "D\ta\t없음\nS\tb\n": 1, // D는 --check에서만
	} {
		if n := bad(out); n != want {
			t.Errorf("%q: 못 깖 %d, 바람 %d", out, n, want)
		}
	}
}

// 빌드 입력은 게시기가 검사한 커밋의 원본 blob 그대로다 — 저장소 속성(export-ignore·export-subst·eol)이 붙은 파일도 빠지거나
// 바뀌지 않는다(재감사 R-14). 실행 권한·링크도 커밋 그대로
func TestExportTreeRawBlobs(t *testing.T) {
	r := newRepo(t)
	TMP = t.TempDir()
	files := map[string]string{
		".gitattributes": "notice.txt export-ignore\nsubst.txt export-subst\ncrlf.txt text eol=crlf\n",
		"notice.txt":     "embed me\n",
		"subst.txt":      "$Format:%H$\n",
		"crlf.txt":       "a\nb\n",
		"sub/x.sh":       "#!/bin/sh\n",
	}
	for p, c := range files {
		os.MkdirAll(filepath.Dir(r+"/"+p), 0o755)
		os.WriteFile(r+"/"+p, []byte(c), 0o644)
	}
	os.Chmod(r+"/sub/x.sh", 0o755)
	os.Symlink("notice.txt", r+"/lnk")
	gitT(t, r, "add", "-A")
	gitT(t, r, "commit", "-qm", "attrs")
	dst := t.TempDir()
	if err := exportTree(r, gitT(t, r, "rev-parse", "HEAD"), dst); err != nil {
		t.Fatal(err)
	}
	files["asset"] = "initial\n"
	for p, c := range files {
		if got := readOr(dst + "/" + p); got != c {
			t.Errorf("%s: %q, 바람 %q(원본 blob)", p, got, c)
		}
	}
	if fi, err := os.Stat(dst + "/sub/x.sh"); err != nil || fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("실행 권한이 빠졌다: %v", err)
	}
	if l, err := os.Readlink(dst + "/lnk"); err != nil || l != "notice.txt" {
		t.Errorf("링크: %q %v", l, err)
	}
}

// 전송(hc exec)이 실패로 끝났으면 응답 줄을 믿지 않는다 — 「받음」(확정 설치)을 찍지 않고, 받았는지 모른다고 경고한다(재감사 R-19)
func TestPutNotesTransportFailed(t *testing.T) {
	ns := putNotes("w1", []string{"hello"}, "W\thello\n", 255, "connection reset")
	warned := false
	for _, n := range ns {
		if !n.bad && strings.Contains(n.s, "받음") {
			t.Errorf("전송 실패인데 확정 줄: %q", n.s)
		}
		if n.bad && strings.Contains(n.s, "connection reset") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("실패 경고가 없다: %v", ns)
	}
	if ns := putNotes("w1", []string{"hello"}, "W\thello\n", 0, ""); len(ns) != 1 || ns[0].warn || ns[0].bad || ns[0].s != "w1 받음 hello" {
		t.Errorf("정상: %v", ns)
	}
}

// ghEnv: PATH 앞에 가짜 gh — 호출을 $HCT/gh.calls에. 릴리스 자산은 $HCT/rel/<태그>/, GH_VIEW_ERR가 있으면 view가 그 글로 실패,
// GH_DL_FAIL이면 download 실패
const fakeGHUnit = `#!/bin/sh
echo "$*" >> "$HCT/gh.calls"
op=$2 tag=$3; shift 3
d=$HCT/rel/$tag
case $op in
  view) if [ -n "${GH_VIEW_ERR-}" ]; then echo "$GH_VIEW_ERR" >&2; exit 1; fi
    [ -d "$d" ] || { echo "release not found" >&2; exit 1; }
    ls "$d" ;;
  download) [ -z "${GH_DL_FAIL-}" ] || exit 1
    while [ $# -gt 0 ]; do case $1 in --pattern) p=$2; shift ;; --dir) o=$2; shift ;; esac; shift; done
    cp "$d/$p" "$o/" ;;
  create | upload) mkdir -p "$d"; while [ $# -gt 0 ]; do case $1 in --repo | --title | --notes) shift ;; --clobber) ;; *) cp "$1" "$d/" ;; esac; shift; done ;;
esac
`

func ghEnv(t *testing.T) (hct, out string) {
	t.Helper()
	hct = t.TempDir()
	os.WriteFile(hct+"/gh", []byte(fakeGHUnit), 0o755)
	t.Setenv("PATH", hct+":"+os.Getenv("PATH"))
	t.Setenv("HCT", hct)
	TMP = t.TempDir()
	out = t.TempDir()
	for n, c := range map[string]string{"a": "A", "SHA256SUMS": sumsA} {
		os.WriteFile(out+"/"+n, []byte(c), 0o644)
	}
	return
}

// sumsA: 자산 a(바이트 "A") 하나의 SHA256SUMS
var sumsA = func() string { h := sha256.Sum256([]byte("A")); return hex.EncodeToString(h[:]) + "  a\n" }()

// 이미 있는 릴리스는 덮지 않는다: 조회·확인을 못 하면 모름(올리지 않음), SHA256SUMS가 다르면 둔다, 빠진 자산만 --clobber 없이 채운다(감사 A05·C01·O-12)
func TestUploadNeverClobbers(t *testing.T) {
	quiet := func(string) {}
	for _, tc := range []struct {
		name, viewErr, dlFail string
		have                  map[string]string // 이미 올린 자산
		want                  bool
		up                    string // 올린 이름들(없으면 "")
		files                 string // 올린 파일 이름(빈칸으로, 순서대로)
	}{
		{"없음 확인 → 만듦", "", "", nil, true, "create", "a SHA256SUMS"},
		{"조회 오류(통신) → 모름", "HTTP 502: Bad Gateway", "", nil, false, "", ""},
		// gh 오류 글에 「not found」가 들어 있어도 릴리스 없음(release not found)이 아니면 모름(R-12)
		{"조회 오류(프록시 이름 풀기) → 모름", "proxy host not found", "", nil, false, "", ""},
		{"조회 오류(저장소 없음) → 모름", "HTTP 404: Not Found (https://api.github.com/repos/o/r/releases/tags/bin-x)", "", nil, false, "", ""},
		{"SHA256SUMS 받기 실패 → 모름", "", "1", map[string]string{"a": "A", "SHA256SUMS": sumsA}, false, "", ""},
		{"SHA256SUMS 다름 → 둔다", "", "", map[string]string{"a": "OLD", "SHA256SUMS": "old  a\n"}, false, "", ""},
		{"바이너리 빠짐 → 채움", "", "", map[string]string{"SHA256SUMS": sumsA}, true, "upload", "a"},
		// SHA256SUMS 없이 바이너리만 남은 반쪽 릴리스: 남은 바이트가 이번 빌드와 같을 때만 명세를 올린다(R-13)
		{"명세 빠짐·남은 바이너리 같음 → 명세 채움", "", "", map[string]string{"a": "A"}, true, "upload", "SHA256SUMS"},
		{"명세 빠짐·남은 바이너리 다름 → 둔다", "", "", map[string]string{"a": "OLD"}, false, "", ""},
		{"명세 빠짐·남은 바이너리 받기 실패 → 모름", "", "1", map[string]string{"a": "A"}, false, "", ""},
		{"같음", "", "", map[string]string{"a": "A", "SHA256SUMS": sumsA}, true, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hct, out := ghEnv(t)
			t.Setenv("GH_VIEW_ERR", tc.viewErr)
			t.Setenv("GH_DL_FAIL", tc.dlFail)
			if tc.have != nil {
				os.MkdirAll(hct+"/rel/bin-x", 0o755)
				for n, c := range tc.have {
					os.WriteFile(hct+"/rel/bin-x/"+n, []byte(c), 0o644)
				}
			}
			got := upload("o/r", "bin-x", "c12", out, []string{"a"}, sumsA, quiet)
			calls := readOr(hct + "/gh.calls")
			if got != tc.want {
				t.Errorf("돌려준 값 %v, 바람 %v (calls %q)", got, tc.want, calls)
			}
			up, files := "", ""
			for _, l := range strings.Split(calls, "\n") {
				if strings.HasPrefix(l, "release upload") && strings.Contains(l, "--clobber") {
					t.Errorf("--clobber로 올렸다: %q", l)
				}
				if strings.HasPrefix(l, "release create") || strings.HasPrefix(l, "release upload") {
					up = strings.Fields(l)[1]
					for _, w := range strings.Fields(l) {
						if strings.HasPrefix(w, out+"/") {
							files = strings.TrimSpace(files + " " + strings.TrimPrefix(w, out+"/"))
						}
					}
				}
			}
			if up != tc.up || files != tc.files {
				t.Errorf("올림 %q %q, 바람 %q %q (calls %q)", up, files, tc.up, tc.files, calls)
			}
			// 올린 파일은 릴리스에 이번 빌드의 바이트 그대로 있다(R-31 — 명세만 다시 올리는 꼴을 잡는다)
			for _, n := range strings.Fields(tc.files) {
				if b := readOr(hct + "/rel/bin-x/" + n); b != readOr(out+"/"+n) {
					t.Errorf("올린 %s의 바이트가 이번 빌드와 다르다: %q", n, b)
				}
			}
			if tc.want {
				for _, n := range []string{"a", "SHA256SUMS"} {
					if !exists(hct + "/rel/bin-x/" + n) {
						t.Errorf("받기로 넘어가는데 릴리스에 %s가 없다", n)
					}
				}
			}
			for n, c := range tc.have {
				if readOr(hct+"/rel/bin-x/"+n) != c {
					t.Errorf("있던 자산 %s가 바뀌었다", n)
				}
			}
			if !tc.want && tc.have != nil && tc.have["SHA256SUMS"] == "" && exists(hct+"/rel/bin-x/SHA256SUMS") {
				t.Errorf("남은 바이너리를 확인하지 못했는데 명세를 올렸다")
			}
		})
	}
}

// go:embed 입력(agent.sh 등 .go가 아닌 파일)만 바뀌어도 소스 판이 바뀌어야 옛 기록을 버리고 다시 깐다(감사 A04·O-03)
func TestSrcDigestCountsEmbed(t *testing.T) {
	d := t.TempDir()
	for p, c := range map[string]string{"go.mod": "module x\n", "cmd/a/main.go": "package main\n", "cmd/a/agent.sh": "echo a\n"} {
		os.MkdirAll(filepath.Dir(filepath.Join(d, p)), 0o755)
		if err := os.WriteFile(filepath.Join(d, p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, err := srcDigest(d)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(d, "cmd/a/agent.sh"), []byte("echo b\n"), 0o644)
	if b, _ := srcDigest(d); b == a {
		t.Errorf("임베드 파일만 바뀌었는데 소스 판이 같다: %s", a)
	}
}
