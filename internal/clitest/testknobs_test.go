package clitest

// 시험 손잡이는 배포 바이너리에 없다(2026-11-24 test-knobs-buildtag): 시험 전용 환경 변수를 읽는 코드는 빌드 태그 파일에만 있고,
// 시험 빌드(Bin — TestTag)만 넣는다. 상주하는 도구가 환경 변수 하나로 셸 명령을 돌리거나 스스로 죽는 길을 막는다.

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// testKnobs: 시험 손잡이 전부 — 도구, 그 손잡이를 넣는 태그, 환경 변수. 새 손잡이는 여기에 더한다(TestTestKnobsListed가 본다)
var testKnobs = []struct {
	tool, tag string
	vars      []string
}{
	{"hc-fill", TestTag, []string{"HC_FILL_TEST_QSET", "HC_FILL_TEST_QSET_CMD", "HC_FILL_CRASH_AT"}},
	{"herdr-hooks", "hookstest", []string{"HERDR_HOOKS_TEST_GIT", "HERDR_HOOKS_TEST_NOW_OFFSET", "HERDR_HOOKS_TEST_SECONDS"}},
}

// ① 태그 없이 지은 바이너리(배포 꼴)에 손잡이 이름 글자가 없다 ② 태그를 주면 있다(목록이 헛돌지 않게)
func TestTestKnobsNotInPlainBuild(t *testing.T) {
	for _, k := range testKnobs {
		t.Run(k.tool, func(t *testing.T) {
			d := TempDir(t)
			plain, err := os.ReadFile(BinPlain(t, d, k.tool))
			if err != nil {
				t.Fatal(err)
			}
			tp := filepath.Join(d, k.tool+".tagged")
			if err := build(Root(t), tp, k.tool, k.tag); err != nil {
				t.Fatal(err)
			}
			tagged, err := os.ReadFile(tp)
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range k.vars {
				if bytes.Contains(plain, []byte(v)) {
					t.Errorf("태그 없는 %s 바이너리에 시험 손잡이 %s가 들어 있다 — 읽는 코드를 //go:build %s 파일로", k.tool, v, k.tag)
				}
				if !bytes.Contains(tagged, []byte(v)) {
					t.Errorf("-tags %s로 지은 %s에 %s가 없다 — 목록이 낡았다", k.tag, k.tool, v)
				}
			}
		})
	}
}

var knobLit = regexp.MustCompile(`"([A-Z][A-Z0-9_]*(?:_TEST(?:_[A-Z0-9_]*)?|_CRASH_AT))"`)

// 제품 코드(cmd·internal의 _test.go 아닌 파일)에서 시험 손잡이 꼴 이름(…_TEST…·…_CRASH_AT)은 빌드 태그 파일에만 있고 목록에 있다
func TestTestKnobsListed(t *testing.T) {
	root := Root(t)
	listed := map[string]bool{}
	for _, k := range testKnobs {
		for _, v := range k.vars {
			listed[v] = true
		}
	}
	seen := map[string]bool{}
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, de os.DirEntry, err error) error {
			if err != nil || de.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			ms := knobLit.FindAllSubmatch(b, -1)
			if len(ms) == 0 {
				return nil
			}
			first, _, _ := strings.Cut(string(b), "\n")
			tagged := first == "//go:build "+TestTag || first == "//go:build hookstest"
			rel, _ := filepath.Rel(root, p)
			for _, m := range ms {
				v := string(m[1])
				seen[v] = true
				if !tagged {
					t.Errorf("%s: 시험 손잡이 %s를 태그 없는 제품 파일에서 읽는다 — //go:build %s 파일로 옮겨라", rel, v, TestTag)
				}
				if !listed[v] {
					t.Errorf("%s: 시험 손잡이 %s가 testKnobs 목록에 없다", rel, v)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for v := range listed {
		if !seen[v] {
			t.Errorf("목록의 %s를 읽는 제품 코드가 없다 — 목록에서 빼라", v)
		}
	}
}

// 배포 빌드(herdr-sync — 기계마다 짓는 셸 줄과 릴리스 자산)는 태그 없이 짓는다
func TestDeployBuildNoTags(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(Root(t), "cmd", "herdr-sync", "release.go"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, `build -trimpath`) || strings.Contains(l, `"build", "-trimpath"`) {
			n++
			if strings.Contains(l, "tags") {
				t.Errorf("배포 빌드 줄에 태그가 있다: %s", strings.TrimSpace(l))
			}
		}
	}
	if n < 2 {
		t.Fatalf("배포 빌드 줄(기계마다·릴리스)을 %d개만 찾았다 — 빌드 줄 꼴이 바뀌었으면 이 시험을 맞춰라", n)
	}
}
