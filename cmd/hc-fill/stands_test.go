//go:build !windows

package main

// 그 완료가 아직 서 있는가(ht-submit-still-stands, stands.go): 통과 판정을 둔 차수(deliver)를 aDeliver 한 바퀴씩 돌려, 레인이 그 완료 뒤
// 막힘·주차를 보고했거나 선행이 아직 안 들어갔거나 사람 리뷰 막음이 처리 전이면 대기열 제출(land)도 「착지하라」 깨우기도 없고 주인 지휘에
// 한 번만 알리는지 본다. 틀은 대기열이 제출 시험(autosubmit_test.go — 가짜 hc·hc-lane)과 진짜 git 저장소(선행 판정)

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"herdr-tools/internal/tri"
)

const stRun = "w1@1700000000000"

type stT struct {
	*subT
	repo string
	ek   int
}

func stGit(t *testing.T, dir string, a ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, a...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", a, err, out)
	}
}

// stSetup: 레인 w1(로컬 m, 상태 status) · 통과 판정 a1(deliver, 표시 없음) · auto_submit app · 원격 없는 저장소 app(기본 refs/heads/main)
func stSetup(t *testing.T, status string) *stT {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	s := &stT{subT: asTSetup(t, "m", status, "app")}
	s.repo = s.dir + "/app"
	stGit(t, s.dir, "init", "-q", "-b", "main", s.repo)
	stGit(t, s.repo, "commit", "-q", "--allow-empty", "-m", "처음")
	EV, EVBAD, THENMAP, CONN = nil, false, nil, nil
	pcRef = map[string]pcRes{}
	return s
}

// ev <상태> <sha> [<실행>]: 레인 터미널 m/w1이 적은 사건(10칸, 실행 기본 stRun). 실행 「old」면 10칸 없는 옛 9칸 꼴
func (s *stT) ev(st, sha string, run ...string) {
	s.ek++
	r := stRun
	if len(run) > 0 {
		r = run[0]
	}
	l := auTEv("2026-11-28T01:"+twoDig(s.ek)+":00Z", "m/w1", "w1", st, sha, "글", r)
	if r == "old" {
		l = strings.Join(strings.Split(l, "\t")[:9], "\t")
	}
	EV = append(EV, l)
}

func twoDig(n int) string { return string(rune('0'+n/10)) + string(rune('0'+n%10)) }

// round: 한 바퀴의 aDeliver(audit_tick이 차수 줄을 읽어 부르는 그대로)
func (s *stT) round() string {
	r := arow("a1")
	auAid, auLn, auRnd, auPrun, auSha, auAm, auPa, auAst = r.F(atID), r.F(atLane), r.F(atRound), r.F(atRun), r.F(atSHA), r.F(atMach), r.F(atAddr), r.F(atState)
	auAfx, auAv, auAwk, auAwt, auAlc, auAlw, auAnv, auAfl = r.F(atFixes), r.F(atVerdict), r.F(atWakes), r.F(atWaited), r.F(atChecked), r.F(atWoke), r.F(atNoVerdict), r.F(atFlags)
	auAn = "audit-" + auAid
	FETCHED, FETCHBAD = "", ""
	pcDrop(s.repo)
	return captured(s.t, aDeliver)
}

func (s *stT) submitted() int { return strings.Count(s.logged(), "land land main --verdict a1") }
func (s *stT) woke() int      { return strings.Count(s.logged(), "감사 판정이 왔다") }
func (s *stT) alerts() int    { return strings.Count(s.logged(), "--key hf:a1:held") }

// setAfter: 지휘의 hc-fill edit w1 --after <선행> 자리(떠 있는 줄의 4칸)
func (s *stT) setAfter(a string) {
	r := qrow("w1").Set(qAfter, a)
	wr(s.t, Q, strings.Join(r, "\t")+"\n")
}

func (s *stT) noSubmit(why, out string) {
	s.t.Helper()
	if s.submitted() != 0 || s.woke() != 0 {
		s.t.Fatalf("%s: 제출 %d·착지 깨우기 %d — 나가지 않아야 한다\n출력:\n%s\n기록:\n%s", why, s.submitted(), s.woke(), out, s.logged())
	}
}

// ④ 오늘 순서: 완료 → 막힘 → 선행 바꿈(안 들어간 선행) → 주차 → (감사 통과) — 제출 없음·깨우기 없음, 알림 한 번. 같은 바퀴 두 번(두 번째
// 실행)에도 알림 한 번·제출 0. 그 뒤 주차한 터미널이 닫히면(부모 실행 끝남) 다시 알리지 않고 superseded
func TestStandsIncident(t *testing.T) {
	s := stSetup(t, "idle")
	s.ev("완료", "abababab")
	s.ev("막힘", "-")
	s.setAfter("p2")
	s.ev("주차", "abababab")
	out := s.round()
	t.Logf("1바퀴 출력:\n%s", out)
	s.noSubmit("오늘 순서", out)
	if s.alerts() != 1 || !strings.Contains(out, "판정 보류 audit-a1 → w1") || !strings.Contains(s.logged(), "주차 abababab") {
		t.Fatalf("주인 지휘에 까닭(주차)과 감사 이름을 한 번: 알림 %d\n%s\n%s", s.alerts(), out, s.logged())
	}
	if r := arow("a1"); r.F(atState) != "deliver" || !inlist("held", r.F(atFlags)) || exists(s.vdir+"/a1.md") {
		t.Fatalf("판정은 그 줄에 남고(deliver·held) 판정 파일은 두지 않는다: %v", r)
	}
	out = s.round()
	s.noSubmit("두 번째 바퀴", out)
	if s.alerts() != 1 {
		t.Fatalf("두 번째 바퀴에 또 알렸다: %d", s.alerts())
	}
	// 주차한 터미널을 run이 닫음 → 부모 실행 끝남: 보류 때 알렸으니 parent-ended 막힘 없이 superseded
	wr(t, s.dir+"/hc", "#!/bin/sh\ncase $1 in get) printf '{\"address\":\"%s\",\"state\":\"finished\"}\\n' \"$2\"; exit 0 ;; esac\nexit 9\n")
	out = s.round()
	t.Logf("부모 끝난 바퀴 출력:\n%s\n가짜 hc-lane·hc 기록:\n%s", out, s.logged())
	if r := arow("a1"); r.F(atState) != "superseded" || strings.Contains(s.logged(), "parent-ended") || s.submitted() != 0 {
		t.Fatalf("끝난 부모: 상태 %s\n%s\n%s", r.F(atState), out, s.logged())
	}
}

// ① 완료 → 막힘 → 통과: 쉬는 레인이면 대기열이 제출하지 않고, ⑨ 턴 중이면 「착지하라」 깨우기를 보내지 않는다
func TestStandsBlocked(t *testing.T) {
	for _, st := range []string{"idle", "working"} {
		s := stSetup(t, st)
		s.ev("완료", "abababab")
		s.ev("막힘", "-")
		s.noSubmit(st, s.round())
		if s.alerts() != 1 {
			t.Fatalf("%s: 알림 %d", st, s.alerts())
		}
	}
}

// ② 완료 → 주차 → 통과
func TestStandsParked(t *testing.T) {
	s := stSetup(t, "done")
	s.ev("완료", "abababab")
	s.ev("주차", "abababab")
	s.noSubmit("주차", s.round())
	// 주차로 되돌린 줄(run의 parkTick — 대기·parked)도 서 있지 않다
	s2 := stSetup(t, "done")
	s2.ev("완료", "abababab")
	wr(t, Q, strings.Join(qrow("w1").Set(qState, "wait").Set(qEarly, "parked"), "\t")+"\n")
	s2.noSubmit("줄이 주차", s2.round())
	// 떠 있는 줄의 14칸 parked(주차 뒤 채택한 실행)는 주차가 아니다 — 그 실행의 완료면 낸다(감사 낮음 4)
	s3 := stSetup(t, "done")
	s3.ev("완료", "abababab")
	wr(t, Q, strings.Join(qrow("w1").Set(qEarly, "parked"), "\t")+"\n")
	if s3.round(); s3.submitted() != 1 {
		t.Fatalf("떠 있는 parked 줄의 완료: %s", s3.logged())
	}
}

// 바퀴를 읽은 뒤 레인이 막힘을 적어 land --verdict가 「완료가 서 있지 않다」로 거절하면(시작 표시 전) 넘기지도 「착지하라」로 깨우지도 않는다(감사 낮음 2)
func TestStandsLandRefusedNoWake(t *testing.T) {
	s := stSetup(t, "idle")
	wr(t, s.dir+"/hc-lane", "#!/bin/sh\necho \"$1 $*\" >>\"$HC_T_LOG\"\ncase $1 in\n"+
		"verdict) echo \"감사 통과 1번째 $2 — sha. 착지: 이 작업 트리에서 hc-lane land main --verdict $2\"; exit 0 ;;\n"+
		"land) echo \"hc-lane land: 판정 $4 의 완료(abababab)가 서 있지 않다 — 이 실행이 그 뒤 「막힘 -」를 보고했다. 착지하지 않았다.\" >&2; exit 2 ;;\nesac\nexit 0\n")
	if !aAutoSubmit("main") || asNote != "" || s.woke() != 0 || strings.Contains(s.logged(), "send ") {
		t.Fatalf("거절을 넘기고 깨웠다: 글 %q\n%s", asNote, s.logged())
	}
}

// ③ 완료 → edit --after(안 들어간 선행) → 통과: 제출 안 함. 선행이 들어가면 남긴 판정으로 다음 바퀴에 낸다
func TestStandsAfterChanged(t *testing.T) {
	s := stSetup(t, "idle")
	s.ev("완료", "abababab")
	s.setAfter("p2")
	s.noSubmit("선행 아직", s.round())
	stGit(t, s.repo, "commit", "-q", "--allow-empty", "-m", "p2: 선행 일")
	out := s.round()
	if s.submitted() != 1 || s.alerts() != 1 {
		t.Fatalf("선행이 들어간 바퀴에 한 번 제출: 제출 %d 알림 %d\n%s", s.submitted(), s.alerts(), out)
	}
}

// ⑤ 완료 → 중간 글 → 통과: 제출(평소 흐름과 같은 한 바퀴). ⑥ 완료 → 막힘 → 같은 커밋 완료 → 통과: 제출
func TestStandsStillStands(t *testing.T) {
	for name, evs := range map[string][]string{"평소": {"완료"}, "중간": {"완료", "중간"}, "다시 완료": {"완료", "막힘", "완료"}} {
		s := stSetup(t, "idle")
		for _, e := range evs {
			sha := "abababab"
			if e != "완료" {
				sha = "-"
			}
			s.ev(e, sha)
		}
		if out := s.round(); s.submitted() != 1 || s.alerts() != 0 {
			t.Fatalf("%s: 한 바퀴에 한 번 제출해야 한다 — 제출 %d 알림 %d\n%s\n%s", name, s.submitted(), s.alerts(), out, s.logged())
		}
	}
}

// ⑦ 모름(선행 판정 못 함 — 다른 프로젝트 저장소 설정 없음): 이 바퀴는 제출·깨우기 없이(알림도 없음), 다음 바퀴에 다시 보고 낸다
func TestStandsUnknown(t *testing.T) {
	s := stSetup(t, "idle")
	s.ev("완료", "abababab")
	s.setAfter("p2@other")
	out := s.round()
	s.noSubmit("모름", out)
	if s.alerts() != 0 || !strings.Contains(out, "서 있는지 모른다") {
		t.Fatalf("모름은 보류 알림이 아니다: %d\n%s", s.alerts(), out)
	}
	o := s.dir + "/other"
	stGit(t, s.dir, "init", "-q", "-b", "main", o)
	stGit(t, o, "commit", "-q", "--allow-empty", "-m", "p2: 다른 프로젝트 선행")
	confLines = append(confLines, "repo other m "+o)
	if out = s.round(); s.submitted() != 1 {
		t.Fatalf("다음 바퀴에 다시 보고 내야 한다:\n%s\n%s", out, s.logged())
	}
}

// ⑩ 처리 안 된 사람 리뷰 막음(넘긴 때가 그 완료 뒤) → 제출 안 함. 막음이 그 완료보다 앞이면(그 완료가 고친 것) 지금 판정대로 처리됨 → 제출
func TestStandsHumanReview(t *testing.T) {
	for _, c := range []struct {
		at   string
		sent int
	}{{"2026-11-28T01:30:00Z", 0}, {"2026-11-28T00:30:00Z", 1}} {
		s := stSetup(t, "idle")
		confLines = append(confLines, "pr_human me")
		wr(t, DATA+"/pr.tsv", strings.Join([]string{"w1", "app", "github.com/example-org/proj", "7", "u", "open", "2026-01-01T00:00:00Z", "self", "-"}, "\t")+"\n")
		wr(t, DATA+"/pr-human.tsv", "hand\tgithub.com/example-org/proj#7\tr1\tw1\tsend:m/w1\t"+c.at+"\t1\n")
		s.ev("완료", strings.Repeat("ab", 20)) // report 완료 HEAD는 40자로 풀린다 — 사람 리뷰 처리 판정(phFixed)은 40자 완료를 본다
		out := s.round()
		if s.submitted() != c.sent || c.sent == 0 && !strings.Contains(out, "사람 리뷰 반영 중") {
			t.Fatalf("막음 %s: 제출 %d(바람 %d)\n%s", c.at, s.submitted(), c.sent, out)
		}
	}
}

// 이미 쌓인 상태에 처음 돌 때(옛 꼴 사건 — 10칸 실행 없는 9칸): 그 실행 시작 뒤 옛 꼴 막힘도 센다, 옛 꼴 완료만이면 낸다
func TestStandsLegacyEvents(t *testing.T) {
	s := stSetup(t, "idle")
	s.ev("완료", "abababab", "old")
	s.ev("막힘", "-", "old")
	s.noSubmit("옛 꼴 막힘", s.round())
	s2 := stSetup(t, "idle")
	s2.ev("완료", "abababab", "old")
	if s2.round(); s2.submitted() != 1 {
		t.Fatalf("옛 꼴 완료만이면 낸다: %s", s2.logged())
	}
}

// 바퀴 순서 반대: 통과를 둔 바퀴에는 서 있었고(턴 중이라 깨움), 막힘이 그 뒤 바퀴에 읽히면 — 다시 깨우지도 대기열이 내지도 않는다
// (이미 둔 판정을 레인이 받아 land하면 hc-lane land의 lvStands가 그 기계의 지금 사건으로 거절한다 — cmd/hc-lane/stands_test.go)
func TestStandsReversedOrder(t *testing.T) {
	s := stSetup(t, "working")
	s.ev("완료", "abababab")
	s.round()
	if s.woke() != 1 || !exists(s.vdir+"/a1.md") {
		t.Fatalf("서 있는 바퀴는 지금처럼 판정을 두고 깨운다: %s", s.logged())
	}
	s.ev("막힘", "-")
	TERMS[0].Status = "idle"
	wr(t, DATA+"/audit.tsv", strings.Join(arow("a1").Set(atWoke, "0"), "\t")+"\n") // 깨우기 간격이 지났어도
	s.round()
	if s.woke() != 1 || s.submitted() != 0 || s.alerts() != 1 {
		t.Fatalf("막힘을 읽은 바퀴: 깨움 %d 제출 %d 알림 %d\n%s", s.woke(), s.submitted(), s.alerts(), s.logged())
	}
}

// ⑪ 쌓아 띄운 줄: 선행이 감사 통과로 병합 줄에 서 있으면(stackPick) 선행이 안 들어갔어도 서 있다. 같은 줄이 쌓아 띄운 줄이 아니면 서 있지 않다
func TestStandsStacked(t *testing.T) {
	sc := skSetup(t, true)
	a := sc.commit(sc.base, "A: 앞 일", 1, "a")
	sc.submit("merge-queue", "A", a, sc.base, " verdict=aud1 patch="+sc.patch(sc.base, a), sc.st)
	sc.row("A", "-", "up", "-")
	sc.row("B", "A", "up", "stacked")
	auLn, auPa, auPrun, auSha = "B", "here/B", "B@1", strings.Repeat("cd", 20)
	EV = []string{auTEv("2026-11-28T01:00:00Z", "here/B", "B", "완료", "cdcdcdcd", "글", "B@1")}
	if s, why := auStands(); s != tri.Yes {
		t.Fatalf("쌓아 띄운 줄의 평소 흐름: %v %s", s, why)
	}
	if STACK != "" || STREF != "" {
		t.Fatalf("판정이 이번 띄움 전역을 바꿨다: %q %q", STACK, STREF)
	}
	sc.q[1] = sc.q[1].Set(qEarly, "-")
	if err := os.WriteFile(Q, []byte(rowsText(sc.q)), 0o644); err != nil {
		t.Fatal(err)
	}
	FETCHED, FETCHBAD = "", ""
	if s, _ := auStands(); s != tri.No {
		t.Fatalf("쌓지 않은 줄은 선행이 들어가야 선다: %v", s)
	}
}
