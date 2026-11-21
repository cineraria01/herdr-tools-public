package main

// 릴리스 받기 조각(install)과 소스 판(srcDigest) 단위 시험 — 2026-11-21 gofix-ops(감사 A03·T02·A04·O-03).
// install 조각을 임시 HOME에서 sh로 직접 돌린다(실제 기계·네트워크 없음, 받기는 file://).

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// instEnv: 받는 자리(go.mod·cmd 소스·옛 바이너리 demo·옛 demo.src)와 새 산출물. 돌려주는 것: HOME, .bin, 새 산출물 경로, 그 sha256.
// 그 자리의 소스 판은 srcOf(home) — install은 보낸 판이 그 기계 소스 판과 같을 때만 바꿔 넣는다
func instEnv(t *testing.T) (home, bin, art, sum string) {
	t.Helper()
	home = t.TempDir()
	bin = home + "/.local/lib/herdr-go/.bin"
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(home+"/.local/lib/herdr-go/cmd/demo", 0o755)
	for p, c := range map[string]string{home + "/.local/lib/herdr-go/go.mod": "module herdr-tools\n",
		home + "/.local/lib/herdr-go/cmd/demo/main.go": "package main\n",
		bin + "/demo": "#!/bin/sh\necho OLD\n", bin + "/demo.src": "old\n"} {
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
	return home, bin, art, hex.EncodeToString(h[:])
}

// runInstall: install 조각을 돌린다. failSuffix가 있으면 PATH 앞 가짜 mv가 그 꼴로 끝나는 대상으로 옮기기만 실패시킨다
func runInstall(t *testing.T, home, failSuffix string, args ...string) string {
	t.Helper()
	fb := t.TempDir()
	if failSuffix != "" {
		mv := "#!/bin/sh\nfor a; do last=$a; done\ncase $last in *" + failSuffix + ") echo 'mv: 막음' >&2; exit 1 ;; esac\nexec /bin/mv \"$@\"\n"
		if err := os.WriteFile(fb+"/mv", []byte(mv), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	c := exec.Command("sh", append([]string{"-c", install, "_"}, args...)...)
	c.Env = append(os.Environ(), "HOME="+home, "PATH="+fb+":"+os.Getenv("PATH"))
	out, _ := c.Output() // 표준 출력(W·S·E 줄)만 — curl·mv 오류 글은 표준 오류
	return string(out)
}

func srcOf(t *testing.T, home string) string {
	t.Helper()
	s, err := srcDigest(home + "/.local/lib/herdr-go")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func readOr(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return "<없음>"
	}
	return string(b)
}

// 받기가 어느 단계에서 실패하든 옛 실행판(바이너리 바이트와 .src)이 그대로 남아야 한다 —
// 「새 바이너리 + .src 없음」이면 go-run이 그것을 그 기계 빌드로 보고 소스와 다른 판을 돌린다(감사 A03, 시험 T02)
func TestInstallKeepsOldOnFailure(t *testing.T) {
	for _, tc := range []struct{ name, fail, sum, url string }{
		{"받기 실패", "", "", "file:///없는/곳/demo"},
		{"해시 다름", "", strings.Repeat("0", 64), ""},
		{"바이너리 바꿔 넣기 실패", "/demo", "", ""},
		{"소스 판 바꿔 넣기 실패", "/demo.src", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, bin, art, sum := instEnv(t)
			url := "file://" + art
			if tc.url != "" {
				url = tc.url
			}
			if tc.sum != "" {
				sum = tc.sum
			}
			out := runInstall(t, home, tc.fail, url, sum, "demo", srcOf(t, home))
			if !strings.HasPrefix(out, "E\tdemo\t") {
				t.Errorf("E 줄이 아니다: %q", out)
			}
			if got := readOr(bin + "/demo"); got != "#!/bin/sh\necho OLD\n" {
				t.Errorf("옛 바이너리가 바뀌었다: %q", got)
			}
			if got := readOr(bin + "/demo.src"); got != "old\n" {
				t.Errorf("옛 소스 판이 바뀌었다: %q", got)
			}
			if m, _ := filepath.Glob(bin + "/.demo*"); len(m) != 0 {
				t.Errorf("임시 파일이 남았다: %v", m)
			}
		})
	}
	t.Run("성공", func(t *testing.T) {
		home, bin, art, sum := instEnv(t)
		src := srcOf(t, home)
		if out := runInstall(t, home, "", "file://"+art, sum, "demo", src); out != "W\tdemo\n" {
			t.Errorf("출력 %q", out)
		}
		if readOr(bin+"/demo") != "#!/bin/sh\necho NEW\n" || readOr(bin+"/demo.src") != src+"\n" {
			t.Errorf("새 판으로 바뀌지 않았다")
		}
		if m, _ := filepath.Glob(bin + "/.demo*"); len(m) != 0 {
			t.Errorf("임시 파일이 남았다: %v", m)
		}
		if out := runInstall(t, home, "", "file://"+art, sum, "demo", src); out != "S\tdemo\n" {
			t.Errorf("다시 부름: %q", out)
		}
	})
}

// 받는 기계의 소스 판이 보낸 판과 다르면 바꿔 넣지 않는다 — 받아도 go-run이 쓰지 않고 그 기계에 맞던 바이너리만 잃는다(감사 C02).
// Go 소스가 없는 기계도 도구마다 K 한 줄(빈 성공이 아니게 — C03)
func TestInstallSourceMismatch(t *testing.T) {
	home, bin, art, sum := instEnv(t)
	if out := runInstall(t, home, "", "file://"+art, sum, "demo", "1.2"); !strings.HasPrefix(out, "K\tdemo\t") {
		t.Errorf("출력 %q", out)
	}
	if readOr(bin+"/demo") != "#!/bin/sh\necho OLD\n" || readOr(bin+"/demo.src") != "old\n" {
		t.Errorf("소스 판이 다른데 바꿔 넣었다")
	}
	empty := t.TempDir()
	if out := runInstall(t, empty, "", "file://"+art, sum, "demo", "1.2", "file://"+art, sum, "two", "1.2"); out != "K\tdemo\t그 기계에 Go 소스가 없다\nK\ttwo\t그 기계에 Go 소스가 없다\n" {
		t.Errorf("소스 없음: %q", out)
	}
}

// 보낸 도구마다 정확히 한 줄 — 빠짐·겹침·모르는 도구는 경고(감사 C03)
func TestInstallReport(t *testing.T) {
	warns := func(out string) (n int) {
		for _, p := range installReport("w1", "bin-x", []string{"a", "b"}, out) {
			if p.warn {
				n++
			}
		}
		return
	}
	if n := warns("W\ta\nS\tb\n"); n != 0 {
		t.Errorf("정상인데 경고 %d", n)
	}
	if n := warns("S\ta\n"); n != 1 {
		t.Errorf("b 응답 없음: 경고 %d", n)
	}
	if n := warns("S\ta\nS\ta\nS\tb\n"); n != 1 {
		t.Errorf("a 두 번: 경고 %d", n)
	}
	if n := warns("S\ta\nS\tb\nW\tc\n"); n != 1 {
		t.Errorf("모르는 도구: 경고 %d", n)
	}
	if n := warns("K\ta\t그 기계 소스 판이 다르다\nS\tb\n"); n != 0 {
		t.Errorf("K는 건너뜀(경고 아님): %d", n)
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
	for n, c := range map[string]string{"a": "A", "SHA256SUMS": "sa  a\n"} {
		os.WriteFile(out+"/"+n, []byte(c), 0o644)
	}
	return
}

// 이미 있는 릴리스는 덮지 않는다: 조회·확인을 못 하면 모름(올리지 않음), SHA256SUMS가 다르면 둔다, 빠진 자산만 --clobber 없이 채운다(감사 A05·C01·O-12)
func TestUploadNeverClobbers(t *testing.T) {
	quiet := func(string) {}
	for _, tc := range []struct {
		name, viewErr, dlFail string
		have                  map[string]string // 이미 올린 자산
		want                  bool
		up                    string // 올린 이름들(없으면 "")
	}{
		{"없음 확인 → 만듦", "", "", nil, true, "create"},
		{"조회 오류(통신) → 모름", "HTTP 502: Bad Gateway", "", nil, false, ""},
		{"SHA256SUMS 받기 실패 → 모름", "", "1", map[string]string{"a": "A", "SHA256SUMS": "sa  a\n"}, false, ""},
		{"SHA256SUMS 다름 → 둔다", "", "", map[string]string{"a": "OLD", "SHA256SUMS": "old  a\n"}, false, ""},
		{"바이너리 빠짐 → 채움", "", "", map[string]string{"SHA256SUMS": "sa  a\n"}, true, "upload"},
		{"같음", "", "", map[string]string{"a": "A", "SHA256SUMS": "sa  a\n"}, true, ""},
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
			got := upload("o/r", "bin-x", "c12", out, []string{"a"}, "sa  a\n", quiet)
			calls := readOr(hct + "/gh.calls")
			if got != tc.want {
				t.Errorf("돌려준 값 %v, 바람 %v (calls %q)", got, tc.want, calls)
			}
			up := ""
			for _, l := range strings.Split(calls, "\n") {
				if strings.HasPrefix(l, "release upload") && strings.Contains(l, "--clobber") {
					t.Errorf("--clobber로 올렸다: %q", l)
				}
				if strings.HasPrefix(l, "release create") {
					up = "create"
				} else if strings.HasPrefix(l, "release upload") {
					up = "upload"
				}
			}
			if up != tc.up {
				t.Errorf("올림 %q, 바람 %q (calls %q)", up, tc.up, calls)
			}
			for n, c := range tc.have {
				if readOr(hct+"/rel/bin-x/"+n) != c {
					t.Errorf("있던 자산 %s가 바뀌었다", n)
				}
			}
		})
	}
}

// go:embed 입력(agent.sh 등 .go가 아닌 파일)만 바뀌어도 소스 판이 바뀌어야 받은 옛 바이너리를 버린다(감사 A04·O-03).
// lib/go-run.sh go_run_src도 같은 규칙이어야 한다(TestSrcDigestMatchesGoRun — gofix-core가 go-run.sh를 고친다)
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
