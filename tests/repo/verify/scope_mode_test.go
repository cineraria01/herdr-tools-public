//go:build !windows

package main

// 범위 모드의 출처(scopeMode, 2026-11-26 ht-verify-scope-on): HERDR_VERIFY_SCOPE > 손잡이 verify_scope(지휘 기계 저장 파일,
// 없으면 이 클론 origin의 refs/herdr/knobs) > 기본 on. 손잡이를 못 읽으면 observe(전부 돌기 — 안전한 쪽).

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/knob"
)

// modeRepo: origin(맨 저장소)이 있는 클론. knobs가 있으면 origin에 refs/herdr/knobs를 그 값으로 둔다
func modeRepo(t *testing.T, knobs map[string]string) string {
	t.Helper()
	O, R := tmp(t), tmp(t)
	scopeGitT(t, O, "init", "-q", "--bare")
	scopeGitT(t, R, "init", "-q")
	scopeGitT(t, R, "remote", "add", "origin", O)
	if len(knobs) > 0 {
		es := map[string]knob.Entry{}
		for n, v := range knobs {
			es[n] = knob.Entry{Name: n, Value: v}
		}
		tree, err := exec.Command("git", "-C", O, "mktree").Output() // 빈 입력 = 빈 트리
		if err != nil {
			t.Fatalf("mktree: %v", err)
		}
		c := exec.Command("git", "-c", "user.name=t", "-c", "user.email=t@t", "commit-tree", strings.TrimSpace(string(tree)), "-m", knob.FormatRemote(es, time.Now()))
		c.Dir = O
		c.Env = append(os.Environ(), "GIT_AUTHOR_DATE=@0 +0000", "GIT_COMMITTER_DATE=@0 +0000")
		b, err := c.Output()
		if err != nil {
			t.Fatalf("commit-tree: %v", err)
		}
		scopeGitT(t, O, "update-ref", knob.Ref, strings.TrimSpace(string(b)))
	}
	return R
}

func TestScopeMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // 저장 파일 없음 — 원격 참조로 읽는다
	mode := func(R, env string) (string, string) {
		t.Helper()
		t.Setenv("HERDR_VERIFY_SCOPE", env)
		var w bytes.Buffer
		return scopeMode(R, &w), w.String()
	}
	for _, c := range []struct {
		what  string
		knobs map[string]string
		env   string
		want  string
	}{
		{"손잡이 없음 → 기본 on", nil, "", "on"},
		{"다른 손잡이만 → 기본 on", map[string]string{"sweep_r1": "40"}, "", "on"},
		{"손잡이 observe", map[string]string{"verify_scope": "observe"}, "", "observe"},
		{"손잡이 off", map[string]string{"verify_scope": "off"}, "", "off"},
		{"환경 변수가 손잡이보다 앞섬", map[string]string{"verify_scope": "off"}, "on", "on"},
		{"환경 변수 observe가 기본 on보다 앞섬", nil, "observe", "observe"},
	} {
		if got, out := mode(modeRepo(t, c.knobs), c.env); got != c.want {
			t.Errorf("%s: %q여야 %q\n%s", c.what, c.want, got, out)
		}
	}
	// 손잡이 설정 파일(ht-config-knobs): 참조에 값이 없을 때만 먹고, 참조 값(지휘 기계가 손잡이 > 설정 파일로 푼 것)이 앞선다
	cd := t.TempDir()
	t.Setenv("HERDR_TOOLS_CONFIG", cd)
	if err := os.WriteFile(cd+"/"+knob.ConfName, []byte("verify_scope off # 이 저장소는 늘 전부\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cf, errs := knob.ParseConf("verify_scope off # x\n", "t"); len(errs) != 0 || cf[knobName].Value != "off" {
		t.Fatalf("설정 파일 꼴이 internal/knob과 어긋난다: %v %v", cf, errs)
	}
	if got, out := mode(modeRepo(t, map[string]string{"sweep_r1": "40"}), ""); got != "off" {
		t.Errorf("참조에 값 없음 → 설정 파일 off여야 %q\n%s", got, out)
	}
	if got, out := mode(modeRepo(t, map[string]string{"verify_scope": "observe"}), ""); got != "observe" {
		t.Errorf("참조 값이 설정 파일보다 앞서야 %q\n%s", got, out)
	}
	t.Setenv("HERDR_TOOLS_CONFIG", t.TempDir())

	// 손잡이를 못 읽음(origin 없음) → observe와 한 줄 알림
	if got, out := mode(tmp(t), ""); got != "observe" || !strings.Contains(out, "손잡이를 못 읽어") {
		t.Errorf("origin 없음: observe와 알림이어야 %q\n%s", got, out)
	}

	// 저장 파일이 있는 기계(지휘 기계): 그 파일에 살아 있는 값이 있으면 그것, 없으면 원격(원격 손잡이의 정본)
	H := t.TempDir()
	t.Setenv("HOME", H)
	R := modeRepo(t, map[string]string{"verify_scope": "off"})
	if got, out := mode(R, ""); got != "off" {
		t.Errorf("저장 파일이 없으면 원격 off여야 %q\n%s", got, out)
	}
	if err := os.MkdirAll(filepath.Dir(knob.Path(H)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(knob.Path(H), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, out := mode(R, ""); got != "off" {
		t.Errorf("저장 파일이 비었으면 원격 off여야 %q\n%s", got, out)
	}
	if err := os.WriteFile(knob.Path(H), []byte("verify_scope\tobserve\tt\tt\t1\t2\t0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, out := mode(R, ""); got != "off" {
		t.Errorf("저장 파일의 값이 기한 지남이면 원격 off여야 %q\n%s", got, out)
	}
	if err := os.WriteFile(knob.Path(H), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := knob.Set(knob.Path(H), knob.Entry{Name: "verify_scope", Value: "observe", By: "t", Why: "t", At: time.Now().Unix()}, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, out := mode(R, ""); got != "observe" {
		t.Errorf("저장 파일 observe: %q\n%s", got, out)
	}
	if err := os.WriteFile(knob.Path(H), []byte("깨진 줄\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, out := mode(R, ""); got != "observe" || !strings.Contains(out, "손잡이를 못 읽어") {
		t.Errorf("깨진 저장 파일: observe와 알림이어야 %q\n%s", got, out)
	}
	if d, ok := knob.Find(knobName); !ok || d.Default != knobDefault || knob.Ref != knobRef || d.Check("observe") != nil || d.Check("on") != nil || d.Check("off") != nil {
		t.Errorf("등록 표 verify_scope(%+v)·원격 참조가 검증 본문의 상수와 어긋난다", d)
	}
}

// 검증 본문(시험 아닌 소스)은 표준 라이브러리만 가져온다 — 레인이 internal을 깨도 검증은 지어지게(main.go 머리말)
func TestVerifyStdlibOnly(t *testing.T) {
	fs, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fs {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		p, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, im := range p.Imports {
			if path, _ := strconv.Unquote(im.Path.Value); strings.Contains(strings.SplitN(path, "/", 2)[0], ".") || strings.HasPrefix(path, "herdr-tools/") {
				t.Errorf("%s가 표준 라이브러리 밖 %s를 가져온다", f, path)
			}
		}
	}
}
