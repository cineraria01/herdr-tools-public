//go:build !windows

package main

// 감사 atozkxr-jlqwld: 미리 보기(rm·prio --dry-run)는 대기열 파일을 못 읽으면 「대기열에 없다」가 아니라 실제 명령과 같이
// 「대기열을 읽지 못해 고치지 않는다」·종료 1(R-29 — 모름은 모름, 중간 1), 없는 레인은 rm 미리 보기도 실제 rm처럼 종료 1(낮음 1)

import (
	"os"
	"strings"
	"testing"
)

func TestDryRunUnreadableQueueIsUnknown(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root는 권한 없이도 읽는다")
	}
	k := newFillKit(t)
	q := k.data + "/queue.tsv"
	if err := os.Chmod(q, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(q, 0o644) })
	for _, a := range [][]string{{"rm", "lane-a", "--dry-run"}, {"prio", "lane-a", "낮음", "--dry-run"}} {
		out, se, rc := k.run(t, a...)
		if rc != 1 || !strings.Contains(se, "대기열을 읽지 못해") || strings.Contains(se, "대기열에 없다") || strings.Contains(out, "미리 보기") {
			t.Errorf("%q: 못 읽음은 모름(종료 1·「읽지 못해」)이어야 — %d %q %q", a, rc, out, se)
		}
	}
}

func TestRmDryRunMissingLaneLikeReal(t *testing.T) {
	k := newFillKit(t)
	out, se, rc := k.run(t, "rm", "nolane", "--dry-run")
	if rc != 1 || !strings.HasPrefix(se, "hc-fill: 대기열에 없다: nolane\nhc-fill: 다음:") || out != "" {
		t.Fatalf("없는 레인 rm 미리 보기: 실제 rm처럼 종료 1이어야 — %d %q %q", rc, out, se)
	}
	_, rse, rrc := k.run(t, "rm", "nolane")
	if rrc != rc || rse != se {
		t.Fatalf("실제 rm과 글·종료가 달라졌다: 실제 %d %q, 미리 보기 %d %q", rrc, rse, rc, se)
	}
}
