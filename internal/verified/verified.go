// Package verified: 「이 트리는 저장소 검증을 통과했다」 원격 기록(merger-spec, 2026-11-22 사용자 「넣기 — 지금 바로」).
//
// 기록 = 원격 참조 refs/herdr/verified/<트리 SHA> → 커밋 하나. 그 커밋의 트리가 곧 검증한 트리이고(그래서 트리 객체가 원격에 남는다),
// 제목은 「herdr-verified tree=<트리> script=<검증 스크립트 blob> machine=<기계> at=<유닉스 초>」. 부모는 없다.
// 병합기의 추측 검증(cmd/hc-merger spec*.go)이 검증 기계에서 통과하면 쓰고, 착지(hc-lane land, HERDR_MERGER=1)가 같은 트리·같은 검증
// 스크립트 blob의 기록이 있을 때만 HERDR_VERIFIED_TREE=<트리>를 검증에 넘긴다. 건너뛸지는 저장소 검증 스크립트가 정한다 —
// 그 스크립트가 지금 HEAD^{tree}가 그 값과 같고 작업 트리가 깨끗할 때만 건너뛴다(docs/hc-lane/merger-spec-verify.md).
// 결과는 셋(gitx.State): 있음 · 없음 · 모름(git 실패 — 모름이면 건너뛰지 않는다).
package verified

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/proc"
)

const (
	Prefix = "refs/herdr/verified/"
	Script = ".claude/herdr-verify.sh"
	head   = "herdr-verified"
)

// Rec: 기록 한 건
type Rec struct {
	Tree, Script, Machine string
	At                    int64
}

// Message: 기록 커밋의 제목
func (r Rec) Message() string {
	return fmt.Sprintf("%s tree=%s script=%s machine=%s at=%d", head, r.Tree, r.Script, r.Machine, r.At)
}

// Parse: 제목(첫 줄)을 읽는다. 꼴이 틀리면 ok=false
func Parse(msg string) (Rec, bool) {
	line, _, _ := strings.Cut(msg, "\n")
	f := strings.Fields(line)
	if len(f) != 5 || f[0] != head {
		return Rec{}, false
	}
	var r Rec
	for _, kv := range f[1:] {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "tree":
			r.Tree = v
		case "script":
			r.Script = v
		case "machine":
			r.Machine = v
		case "at":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return Rec{}, false
			}
			r.At = n
		default:
			return Rec{}, false
		}
	}
	return r, sha(r.Tree) && sha(r.Script) && r.Machine != ""
}

func sha(s string) bool {
	return (len(s) == 40 || len(s) == 64) && strings.Trim(s, "0123456789abcdef") == ""
}

// ScriptBlob: 그 트리의 검증 스크립트 blob. 스크립트가 없으면 없음
func ScriptBlob(dir, tree string, stderr io.Writer) (string, gitx.State) {
	o, rc := gitx.Out(dir, stderr, "rev-parse", "-q", "--verify", tree+":"+Script)
	if rc == 128 || rc == 1 {
		// 트리는 있는데 그 경로가 없다(rev-parse가 128) — 트리 자체가 있는지 다시 본다(없는 트리를 「스크립트 없음」으로 읽지 않게)
		if gitx.Run(dir, nil, nil, "cat-file", "-e", tree+"^{tree}") == 0 {
			return "", gitx.No
		}
		return "", gitx.Unknown
	}
	return o, gitx.Probe(o, rc)
}

// Lookup: 원격 기록을 찾는다. 있음은 기록 커밋의 트리가 tree이고 제목이 맞고 script가 그 트리의 검증 스크립트 blob일 때뿐.
// 기록이 있는데 맞지 않으면 없음(그 트리를 다시 검증한다)
func Lookup(dir, tree string, stderr io.Writer) (Rec, gitx.State) {
	if !sha(tree) {
		return Rec{}, gitx.No
	}
	ls, rc := gitx.Out(dir, stderr, "ls-remote", "origin", Prefix+tree)
	if rc != 0 {
		return Rec{}, gitx.Unknown
	}
	c, _, _ := strings.Cut(ls, "\t")
	if c == "" {
		return Rec{}, gitx.No
	}
	if gitx.Run(dir, nil, nil, "cat-file", "-e", c+"^{commit}") != 0 &&
		gitx.Run(dir, nil, stderr, "fetch", "-q", "--no-write-fetch-head", "origin", Prefix+tree) != 0 {
		return Rec{}, gitx.Unknown
	}
	body, rc := gitx.Out(dir, stderr, "cat-file", "commit", c)
	if rc != 0 {
		return Rec{}, gitx.Unknown
	}
	hdr, msg, _ := strings.Cut(body, "\n\n")
	r, ok := Parse(msg)
	if !ok || r.Tree != tree || !strings.HasPrefix(hdr, "tree "+tree+"\n") {
		fmt.Fprintf(stderr, "verified: %s%s 기록 꼴이 맞지 않다 — 기록 없음으로 본다\n", Prefix, tree)
		return Rec{}, gitx.No
	}
	b, st := ScriptBlob(dir, tree, stderr)
	switch st {
	case gitx.Unknown:
		return Rec{}, gitx.Unknown
	case gitx.No:
		return Rec{}, gitx.No
	}
	if b != r.Script {
		return Rec{}, gitx.No
	}
	return r, gitx.Yes
}

// Record: 기록을 원격에 만든다(이미 있으면 바꾸지 않음 — lease 「없음」). 있음 = 기록됨(또는 같은 트리의 맞는 기록이 이미 있음)
func Record(dir string, r Rec, stderr io.Writer) gitx.State {
	if !sha(r.Tree) {
		return gitx.Unknown
	}
	b, st := ScriptBlob(dir, r.Tree, stderr)
	if st != gitx.Yes {
		return st // 검증 스크립트 없는 트리는 기록하지 않는다
	}
	r.Script = b
	c := exec.Command("git", "commit-tree", r.Tree, "-m", r.Message())
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=herdr-verified", "GIT_AUTHOR_EMAIL=herdr-verified@localhost",
		"GIT_COMMITTER_NAME=herdr-verified", "GIT_COMMITTER_EMAIL=herdr-verified@localhost")
	var o bytes.Buffer
	c.Stdout, c.Stderr = &o, stderr
	if proc.Code(proc.Run(c)) != 0 {
		return gitx.Unknown
	}
	cm := strings.TrimSpace(o.String())
	ref := Prefix + r.Tree
	if gitx.Run(dir, nil, stderr, "push", "-q", "--force-with-lease="+ref+":", "origin", cm+":"+ref) == 0 {
		return gitx.Yes
	}
	// 거절이면 누가 먼저 썼을 수 있다 — 맞는 기록이면 있음
	_, st = Lookup(dir, r.Tree, stderr)
	if st == gitx.No {
		return gitx.Unknown
	}
	return st
}

// Remove: 기록을 지운다(본 값 조건). 없으면 그대로 있음
func Remove(dir, tree, was string, stderr io.Writer) bool {
	return gitx.Run(dir, nil, stderr, "push", "-q", "--force-with-lease="+Prefix+tree+":"+was, "origin", ":"+Prefix+tree) == 0
}
