package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// knob-test-leak ⑤: requeue — 검증 실패 반송은 같은 커밋·같은 기록(감사 판정)으로 줄 끝에, 충돌 반송·취소된 판정·이미 든 제출·이미 줄에
// 있음은 거절, dry-run은 옮기지 않음, 일지에 남김
func TestRequeue(t *testing.T) {
	setData(t)
	git := func(dir string, a ...string) string {
		t.Helper()
		c := exec.Command("git", a...)
		c.Dir = dir
		o, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, o)
		}
		return strings.TrimSpace(string(o))
	}
	o, w := DATA+"/o.git", DATA+"/w"
	git(DATA, "init", "-q", "--bare", "-b", "main", o)
	git(DATA, "init", "-q", "-b", "main", w)
	git(w, "commit", "-q", "--allow-empty", "-m", "base")
	git(w, "remote", "add", "origin", o)
	git(w, "push", "-q", "origin", "main")
	// 제출 하나 = 제출 커밋 + 기록 커밋(부모 = 제출, 첫 줄에 verdict·id)
	submit := func(lane, n, verdict string) (string, string) {
		git(w, "checkout", "-q", "-B", "x-"+lane, "main")
		git(w, "commit", "-q", "--allow-empty", "-m", lane)
		s := git(w, "rev-parse", "HEAD")
		rec := git(w, "commit-tree", s+"^{tree}", "-p", s, "-m", "base=main verdict="+verdict+" patch=- id=box-"+lane)
		git(w, "push", "-q", "origin", s+":refs/merge-failed/"+n, rec+":refs/merge-failed-meta/"+n)
		return s, rec
	}
	sA, recA := submit("la", "1000000001-la", "a1")
	submit("lb", "1000000002-lb", "b1")
	submit("lc", "1000000003-lc", "c1")
	sD, _ := submit("ld", "1000000004-ld", "d1")
	git(w, "push", "-q", "origin", sD+":refs/heads/main") // ld는 이미 기본 가지에
	submit("le", "1000000005-le", "e1")
	git(w, "push", "-q", "origin", sA+":refs/merge-queue/1000000099-le") // le는 이미 줄에
	// 감사 표: c1은 취소됨(superseded)
	row := make([]string, atNF)
	for i := range row {
		row[i] = "-"
	}
	row[atID-1], row[atState-1] = "c1", "superseded"
	os.WriteFile(AT, []byte(strings.Join(row, "\t")+"\n"), 0o644)
	// 가짜 hc-lane events: 병합기 기계 box의 반송 사건
	ev := strings.Join([]string{
		rlEv("2026-11-24T06:00:00Z", "la", "1000000001-la", "착지 실패(종료 1, 추측 검증 — v1 에서 맨 앞 혼자 기준 aaaaaaaa 위 시험 실패, 첫 실패 「x」)", "t9"),
		rlEv("2026-11-24T06:00:01Z", "lb", "1000000002-lb", "착지 실패(종료 3, 충돌 a.go)", "t9"),
		rlEv("2026-11-24T06:00:02Z", "lc", "1000000003-lc", "착지 실패(종료 1)", "t9"),
		rlEv("2026-11-24T06:00:03Z", "ld", "1000000004-ld", "착지 실패(종료 1)", "t9"),
		rlEv("2026-11-24T06:00:04Z", "le", "1000000005-le", "착지 실패(종료 1)", "t9"),
	}, "\n") + "\n"
	os.WriteFile(DATA+"/ev", []byte(ev), 0o644)
	os.WriteFile(DATA+"/hc-lane", []byte("#!/bin/sh\n[ \"$1\" = events ] && cat "+DATA+"/ev\n"), 0o755)
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	jf := DATA + "/journal.jsonl"
	t.Setenv("HERDR_TOOLS_JOURNAL", jf)
	confLines = lines("local here\nrepo app here " + w + "\nmerger app box\n")
	LOCAL = "here"
	pcRef = map[string]pcRes{}

	refs := func() string { return git(w, "ls-remote", "origin", "refs/merge-*") }
	before := refs()
	if rc := requeue("la", "거짓 반송", true); rc != 0 || refs() != before {
		t.Fatalf("dry-run: rc %d, 원격이 바뀜:\n%s", rc, refs())
	}
	if rc := requeue("la", "거짓 반송", false); rc != 0 {
		t.Fatalf("검증 실패 반송 다시 넣기 rc %d", rc)
	}
	after := refs()
	if strings.Contains(after, "1000000001-la") {
		t.Fatalf("반송 참조가 남았다:\n%s", after)
	}
	var q, qm string
	for _, l := range strings.Split(after, "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && strings.HasPrefix(f[1], "refs/merge-queue/") && strings.HasSuffix(f[1], "-la") {
			q = f[0]
			qm = git(w, "ls-remote", "origin", strings.Replace(f[1], "merge-queue/", "merge-queue-meta/", 1))
		}
	}
	if q != sA || !strings.HasPrefix(qm, recA) {
		t.Fatalf("같은 커밋·같은 기록으로 줄에: q=%s 바람 %s, meta=%q 바람 %s\n%s", q, sA, qm, recA, after)
	}
	for lane, want := range map[string]string{"lb": "검증 실패 반송이 아니다", "lc": "감사 판정 c1 가 취소됐다", "ld": "이미 기본 가지에",
		"le": "이미 줄에 있다", "lz": "원격에 없다"} {
		out := dupCapture(t, func() {
			if rc := requeue(lane, "x", false); rc != 1 {
				t.Errorf("%s: rc %d, 바람 1(거절)", lane, rc)
			}
		})
		if !strings.Contains(out, want) {
			t.Errorf("%s: %q 에 %q 없음", lane, out, want)
		}
	}
	j, _ := os.ReadFile(jf)
	if !strings.Contains(string(j), `"action":"requeue","target":"la","why":"거짓 반송","result":"됨: refs/merge-failed/1000000001-la → refs/merge-queue/`) ||
		!strings.Contains(string(j), `"target":"lb"`) {
		t.Fatalf("일지:\n%s", j)
	}
}
