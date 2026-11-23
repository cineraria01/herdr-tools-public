//go:build !windows

package main

// merger-journal-rounds A(2026-11-23 — 지휘가 일지만 보고 「줄이 있는데 병합기가 쉰 틈 2.0·3.7시간」으로 잘못 읽었다: 그 틈은 모두 일지에
// 안 남는 바퀴였다): 검증을 돌았거나 줄을 붙잡은 바퀴는 착지·돌려보냄이 아니어도 구성원마다 한 줄(같은 바퀴는 7칸 바퀴 시작이 같다).
// 추측 기다림은 한 번의 기다림을 한 줄로(되풀이 횟수).

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// 가짜 hc-lane land: six.txt면 lease 바뀜(6), tr.txt면 기다림 상한(75), bad.txt면 시험 실패 한 줄을 찍고 1
const fakeJLand = `if [ "$1" = land ]; then
  printf 'land|%s\n' "$HC_LANE" >>"$LANDLOG"
  git fetch -q origin || exit 1
  git rebase -q origin/main || exit 3
  [ ! -e six.txt ] || exit 6
  [ ! -e tr.txt ] || exit 75
  if [ -e bad.txt ]; then echo '--- FAIL: TestBad (0.00s)'; exit 1; fi
  git push -q --atomic origin HEAD:refs/heads/main $HERDR_LAND_PUSH_EXTRA || exit 1
  exit 0
fi
`

func jFx(t *testing.T) *fx {
	f := newFx(t)
	f.git(f.seed, "push", "-q", "origin", "main:refs/herdr/merger/box")
	t.Setenv("LANDLOG", f.dir+"/land.log")
	old, err := os.ReadFile(gSelf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gSelf, []byte(strings.Replace(string(old), "#!/bin/sh\n", "#!/bin/sh\n"+fakeJLand, 1)), 0o777); err != nil {
		t.Fatal(err)
	}
	return f
}

// jrows: 일지에서 결과가 res인 줄(12칸)
func jrows(t *testing.T, res string) [][]string {
	t.Helper()
	b, err := os.ReadFile(gEvdir + "/merger-journal.tsv")
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var o [][]string
	for _, l := range strings.Split(string(b), "\n") {
		if c := strings.Split(l, "\t"); len(c) == 12 && c[9] == res {
			o = append(o, c)
		}
	}
	return o
}

func jdump() string {
	b, _ := os.ReadFile(gEvdir + "/merger-journal.tsv")
	return string(b)
}

// sameRound: 줄들이 같은 바퀴(7·8칸 같음, 시각이 있음)이고 레인이 lanes 차례이며 묶음 크기 칸이 구성원 수
func sameRound(t *testing.T, rows [][]string, lanes ...string) {
	t.Helper()
	if len(rows) != len(lanes) {
		t.Fatalf("구성원마다 한 줄 %v:\n%s", lanes, jdump())
	}
	for i, r := range rows {
		if r[2] != lanes[i] || r[3] != "box-"+lanes[i] || r[6] != fmt.Sprint(len(lanes)) || r[7] == "-" || r[8] == "-" ||
			r[7] != rows[0][7] || r[8] != rows[0][8] || r[10] != "-" {
			t.Fatalf("같은 바퀴의 구성원 줄이어야(레인 %s):\n%s", lanes[i], jdump())
		}
		if !strings.Contains(r[11], "구성원 "+strings.Join(lanes, "+")) {
			t.Fatalf("까닭에 구성원: %q", r[11])
		}
	}
}

// 묶음 실패(반송 없음, 검증은 다 돎): 구성원마다 「묶음 실패」, 까닭에 실패한 시험 첫 줄·착지 로그 경로
func TestJournalBatchFail(t *testing.T) {
	f := jFx(t)
	f.sub2(1000000001, "f1", "", "", "f1.txt", "f1\n", false)
	f.sub2(1000000002, "f2", "", "", "f2.txt", "f2\n", false)
	f.sub2(1000000003, "f3", "", "", "bad.txt", "bad\n", false)
	m := f.merger()
	m.round()
	rows := jrows(t, "묶음 실패")
	sameRound(t, rows, "f1", "f2", "f3")
	w := rows[0][11]
	if !strings.Contains(w, "종료 1") || !strings.Contains(w, "--- FAIL: TestBad") || !strings.Contains(w, "로그 "+m.lgf) {
		t.Fatalf("까닭에 종료 코드·첫 실패 줄·로그 경로: %q", w)
	}
	if c, _ := os.ReadFile(m.mj + "/cap"); strings.TrimSpace(string(c)) != "2" {
		t.Fatalf("판정은 그대로(cap 2): %q", c)
	}
}

// lease 바뀜(종료 6)·일시 실패(75)도 그 바퀴의 줄
func TestJournalLeaseAndTransient(t *testing.T) {
	for _, c := range []struct{ file, res, code string }{{"six.txt", "lease 바뀜", "종료 6"}, {"tr.txt", "일시 실패", "종료 75"}} {
		t.Run(c.res, func(t *testing.T) {
			f := jFx(t)
			q, _ := f.sub2(1000000001, "a", "", "", c.file, "x\n", false)
			m := f.merger()
			m.round()
			rows := jrows(t, c.res)
			sameRound(t, rows, "a")
			if !strings.Contains(rows[0][11], c.code) || !strings.Contains(rows[0][11], "로그 "+m.lgf) {
				t.Fatalf("까닭에 종료 코드·로그: %q", rows[0][11])
			}
			if !f.has(q) || f.has("refs/merge-failed/1000000001-a") {
				t.Fatal("판정은 그대로(큐에 둠, 반송 없음)")
			}
		})
	}
}

// 맨 앞 rebase 충돌로 묶음 크기 1: 기준과 같은 patch-id 커밋을 품은 맨 앞(미리 훑기는 차례에 맡김)이 --reapply-cherry-picks로 충돌
func TestJournalFrontConflict(t *testing.T) {
	f := jFx(t)
	f.git(f.seed, "checkout", "-q", "-B", "lc", "main")
	f.sh("printf 'x\\n' >cf && git add -- cf && git commit -qm lc-a && printf '1\\n' >g && git add -- g && git commit -qm lc-b")
	f.queueHead(1000000001, "lc")
	f.sub2(1000000002, "b", "", "", "b.txt", "b\n", false)
	f.sh("printf 'x\\n' >cf && git add -- cf && git commit -qm main-pick && printf 'y\\n' >cf && git commit -qam main-cf && git push -q origin main")
	m := f.merger()
	m.round()
	if c, _ := os.ReadFile(m.mj + "/cap"); strings.TrimSpace(string(c)) != "1" {
		t.Fatalf("전제: 맨 앞 충돌로 cap 1: %q\n%s", c, jdump())
	}
	rows := jrows(t, "맨 앞 충돌")
	sameRound(t, rows, "lc", "b")
	if !strings.Contains(rows[0][11], "로그 "+m.lgf) {
		t.Fatalf("까닭에 로그: %q", rows[0][11])
	}
}

func (f *fx) sh(script string) {
	f.t.Helper()
	c := exec.Command("sh", "-c", script)
	c.Dir = f.seed
	if o, err := c.CombinedOutput(); err != nil {
		f.t.Fatalf("%s: %v\n%s", script, err, o)
	}
}

// queueHead: seed의 lane 가지 끝을 큐 이름 <초>-<레인>으로 낸다(sub2와 같은 제출 기록 꼴)
func (f *fx) queueHead(sec int, lane string) {
	sha := f.git(f.seed, "rev-parse", lane)
	fork := f.git(f.seed, "merge-base", lane, "main")
	et := f.git(f.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	meta := f.git(f.seed, "commit-tree", et, "-p", sha, "-m",
		fmt.Sprintf("merge-queue lane=%s id=box-%s orig=%s fork=%s base=main host=box term=- owner=- job=- created_ms=- priority=보통", lane, lane, sha, fork))
	name := fmt.Sprintf("%d-%s", sec, lane)
	f.git(f.seed, "push", "-q", "origin", sha+":refs/merge-queue/"+name, meta+":refs/merge-queue-meta/"+name, sha+":refs/heads/lane/"+lane)
	f.git(f.seed, "checkout", "-q", "main")
}

// 추측 기다림 28바퀴 = 한 줄(시작 ~ 끝, 되풀이 횟수). 기다림이 끝난 바퀴에 적고, 그 뒤 착지 줄은 그대로
func TestJournalSpecWaitOneLine(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 {
		t.Fatalf("B 띄움: %v", d)
	}
	for i := 0; i < 28; i++ {
		s.round(1)
	}
	if s.onMain("b.txt") || len(jrows(t, "추측 기다림")) != 0 {
		t.Fatalf("도는 동안은 착지도 줄도 없음:\n%s", jdump())
	}
	if rc := s.verifyOn(d[0]); rc != 0 {
		t.Fatalf("일꾼 종료 %d", rc)
	}
	s.round(1)
	rows := jrows(t, "추측 기다림")
	if len(rows) != 1 || rows[0][2] != "b" || rows[0][3] != "box-b" || rows[0][6] != "1" || rows[0][7] == "-" || rows[0][8] == "-" ||
		rows[0][7] > rows[0][8] || !strings.Contains(rows[0][11], "28바퀴") || !strings.Contains(rows[0][11], "구성원 b") {
		t.Fatalf("기다림 한 번 = 한 줄(28바퀴):\n%s", jdump())
	}
	if len(jrows(t, "착지")) != 2 || !s.onMain("b.txt") {
		t.Fatalf("착지 줄은 그대로 둘:\n%s", jdump())
	}
}
