package hclane_test

// ht-smalls-1129: 꼭대기 경로를 못 읽으면 「git 저장소가 아니다」는 git이 그렇게 답했을 때만, 그 밖은 「저장소를 읽지 못했다: … — <git 원문 한 줄>」(종료 1 그대로)

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoReadError(t *testing.T) {
	withK(t, func(k *K) {
		plain := filepath.Join(k.T, "plain")
		locked := filepath.Join(k.T, "locked")
		mkdirs(t, plain, locked)
		if err := os.Chmod(locked, 0); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(locked, 0o755)
		nope := filepath.Join(k.T, "nope")
		for _, c := range []struct{ name, dir, want, not string }{
			{"저장소 아님", plain, "git 저장소가 아니다: " + plain, "읽지 못했다"},
			{"폴더 없음", nope, "저장소를 읽지 못했다: " + nope + " — fatal: cannot change to", "저장소가 아니다"},
			{"읽기 막힘", locked, "저장소를 읽지 못했다: " + locked + " — fatal: cannot change to", "저장소가 아니다"},
		} {
			k.run("gc", "--repo", c.dir, "--dry-run")
			e := k.errf()
			k.check(c.name+": 글·종료 1", k.rc == 1 && strings.Contains(e, c.want) && !strings.Contains(e, c.not))
			if c.dir != plain {
				k.check(c.name+": git 원문은 한 번", strings.Count(e, "cannot change to") == 1)
			} else {
				k.check(c.name+": git 원문도 화면에(감사 낮음 — 빠졌었다)", strings.Contains(e, "fatal: not a git repository"))
			}
		}
	})
}
