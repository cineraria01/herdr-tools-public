package toolcfg

// sync-lint: sync.list의 걷힌 셸 판 줄·바이너리 줄과 딸린 줄(toolcfg.go에서 나눔 — 파일 줄 한도)

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SyncLint: `herdr-tools-config sync-lint <범용 저장소> <sync.list>` — herdr-sync도 맞추기 전에 부른다. 더할 줄은 stdout, 머리 줄·지울 줄은 stderr.
// 0 통과 · 1 걷힌 셸 판 줄이 남음·바이너리 줄이 틀림·딸린 줄이 모자람 · 2 sync.list 없음
func SyncLint(repo, list string, stdout, stderr io.Writer) int {
	data, err := os.ReadFile(list)
	if !isFile(list) || err != nil {
		fmt.Fprintf(stderr, "%s: sync.list 없음: %s\n", me, list)
		return 2
	}
	var rows [][]string
	for _, l := range strings.Split(string(data), "\n") {
		if f := ListFields(l); len(f) >= 4 {
			rows = append(rows, f)
		}
	}
	bad := 0
	// 1. 걷힌 셸 판 줄(2026-11-22 go-shell-remove): 저장소에 없는 tools:lib/…·tools:…*.sh·*.py 줄, cmd/<도구>가 있는데 저장소에 없는
	// tools:bin/<도구> 줄이 남아 있으면 herdr-sync가 맥에 남은 옛 파일을 저장소로 다시 모은다(맥이 정본) — 맞추기 전에 멈추고 바이너리 줄을 권한다
	var stale, repl []string
	for _, f := range rows {
		r, ok := strings.CutPrefix(f[2], "tools:")
		if !ok || strings.Contains(r, "*") {
			continue
		}
		x, isBin := strings.CutPrefix(r, "bin/")
		goTool := isBin && !strings.Contains(x, "/") && isDir(repo+"/cmd/"+x)
		shellish := goTool || strings.HasPrefix(r, "lib/") || strings.HasSuffix(r, ".sh") || strings.HasSuffix(r, ".py")
		if !shellish || exists(repo+"/"+strings.TrimSuffix(r, "/")) {
			continue
		}
		stale = append(stale, strings.Join(f[:4], "  "))
		if goTool {
			repl = append(repl, fmt.Sprintf("%s  %sbin/%s  tools:cmd/%s  %s", f[0], macRoot(f[1]), x, x, f[3]))
		}
	}
	if len(stale) > 0 {
		fmt.Fprintf(stderr, "%s: %s — 셸 판이 걷혔다(저장소에 없는 셸·파이썬·포장 줄). 이 줄을 지우고(기계에 남은 것은 「지움」 줄로):\n  %s\n",
			me, list, strings.Join(stale, "\n  "))
		if len(repl) > 0 {
			fmt.Fprintf(stderr, "%s: 도구는 바이너리 줄로 바꾼다. 이 줄을 더한다:\n", me)
			fmt.Fprintln(stdout, strings.Join(repl, "\n"))
		}
		bad = 1
	}
	if syncLintBins(repo, list, rows, stdout, stderr) {
		bad = 1
	}
	return bad
}

var cmdLineRe = regexp.MustCompile(`^tools:cmd/([^/]+)/?$`)

// BinLine: sync.list 바이너리 줄(저장소 경로 tools:cmd/<도구> — 나무 줄 아님)이면 그 도구 이름. herdr-sync가 그 도구의 Go 바이너리를
// 맥 경로(<뿌리>bin/<도구>, 보통 .local/bin/<도구>)에 원자 교체로 깐다
func BinLine(mac, repoPath string) string {
	if strings.HasSuffix(mac, "/**") || strings.Contains(mac, "*") {
		return ""
	}
	if m := cmdLineRe.FindStringSubmatch(repoPath); m != nil {
		return m[1]
	}
	return ""
}

// covers: 대상 s가 대상 t를 덮나(같음·all, local은 macos도)
func covers(s, t string) bool { return s == t || s == "all" || (t == "local" && s == "macos") }

// syncLintBins: 바이너리 줄마다 — 맥 경로가 <뿌리>bin/<도구>이고 cmd/<도구>가 있어야 하고, 그 대상을 덮는 Go 소스 나무 줄 셋
// (그 기계에서 지을 때 — 릴리스를 못 받을 때)과, 곁 도구(cmd/<곁>/OWNER 첫 줄이 이 도구 — 예: hc-merger는 hc-lane이 부른다)의
// 바이너리 줄이 있어야 한다. 모자라면 참
func syncLintBins(repo, list string, rows [][]string, stdout, stderr io.Writer) bool {
	type bin struct{ b, root, tool, t string }
	var bins []bin
	have := map[string][]string{} // 맥 경로\t저장소 경로 → 대상들
	bad := false
	for _, f := range rows {
		x := BinLine(f[1], f[2])
		if x == "" {
			have[f[1]+"\t"+f[2]] = append(have[f[1]+"\t"+f[2]], f[3])
			continue
		}
		if !isDir(repo+"/cmd/"+x) || !strings.HasSuffix(f[1], "bin/"+x) {
			fmt.Fprintf(stderr, "%s: %s — 바이너리 줄이 틀렸다(맥 경로는 <뿌리>bin/%s, 저장소에 cmd/%s가 있어야): %s\n", me, list, x, x, strings.Join(f[:4], "  "))
			bad = true
			continue
		}
		bins = append(bins, bin{f[0], macRoot(f[1]), x, f[3]})
	}
	has := func(k, t string) bool {
		for _, s := range have[k] {
			if covers(s, t) {
				return true
			}
		}
		return false
	}
	hasBin := func(tool, t string) bool {
		for _, b := range bins {
			if b.tool == tool && covers(b.t, t) {
				return true
			}
		}
		return false
	}
	var out []string
	seen := map[string]bool{}
	more := func(l string) {
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	srcs := [3][2]string{{"lib/herdr-go/go.mod", "tools:go.mod"}, {"lib/herdr-go/cmd/**", "tools:cmd/"}, {"lib/herdr-go/internal/**", "tools:internal/"}}
	cmds, _ := filepath.Glob(repo + "/cmd/*/OWNER")
	for _, b := range bins {
		for _, p := range srcs {
			if !has(b.root+p[0]+"\t"+p[1], b.t) {
				more(fmt.Sprintf("%s  %s  %s  %s", b.b, b.root+p[0], p[1], b.t))
			}
		}
		for _, o := range cmds {
			c := filepath.Base(filepath.Dir(o))
			if c != b.tool && firstLine(o) == b.tool && !hasBin(c, b.t) {
				more(fmt.Sprintf("%s  %sbin/%s  tools:cmd/%s  %s", b.b, b.root, c, c, b.t))
			}
		}
	}
	if len(out) == 0 {
		return bad
	}
	fmt.Fprintf(stderr, "%s: %s — 바이너리 줄에 딸린 줄이 없다(Go 소스 나무 — 그 기계에서 지을 때, 곁 도구 바이너리). 이 줄을 더한다:\n", me, list)
	fmt.Fprintln(stdout, strings.Join(out, "\n"))
	return true
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }
