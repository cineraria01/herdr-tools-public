package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 고정 사건 → 기대 표(go-land-stats, 2026-11-23). 시각은 모두 2026-11-22 UTC, 실행 ID의 ms는 09:00(1795338000000)
func stE(tm, term, lane, st, sha, msg, run, key string) string {
	return strings.Join([]string{"2026-11-22T" + tm + ":00Z", "m1", term, lane, st, sha, msg, "0", "-", run, "-", key}, "\t")
}

func stE8(day, tm, term, lane, st, sha, msg string) string {
	return strings.Join([]string{day + "T" + tm + ":00Z", "m1", term, lane, st, sha, msg, "0"}, "\t")
}

func stJ(tm, lane, id, sub, batch, t0, t1, res, cand, why string) string {
	at := func(s string) string {
		if s == "-" {
			return s
		}
		return "2026-11-22T" + s + ":00Z"
	}
	return strings.Join([]string{at(tm), "1795339000-" + lane, lane, id, "-", at(sub), batch, at(t0), at(t1), res, cand, why}, "\t")
}

func statsFixture() (ev, jr []string) {
	A, B1, B2, C, D := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("e", 40), strings.Repeat("c", 40), strings.Repeat("d", 40)
	C1, C2, C3 := strings.Repeat("1", 40), strings.Repeat("2", 40), strings.Repeat("3", 40)
	r1, r3, r4, r5 := "t1@1795338000000", "t3@1795338000000", "t4@1795338000000", "t5@1795338000000"
	ev = []string{
		// A: 감사 통과 뒤 병합기 착지
		stE("10:00", "t1", "a", "완료", A, "일 끝", r1, "-"),
		stE("10:30", "t2", "audit-x", "판정", A, "감사 완료 x 높음 0·중간 0·낮음 1", "t2@1795338000000", "-"),
		stE("10:40", "t1", "a", "제출", A, "제출", r1, "submit:S1"),
		stE("11:21", "t1", "a", "착지", C1[:8], "병합기 착지 — 제출 aaaaaaaa, 제출 ID S1", r1, "-"),
		stE8("2026-11-22", "11:30", "t1", "a", "정리됨", "-", "hc-sweep R2"),
		// B: 돌려보냄 → 고쳐 다시 제출 → C와 한 묶음으로 착지
		stE("10:00", "t3", "b", "완료", B1, "일 끝", r3, "-"),
		stE("10:05", "t3", "b", "제출", B1, "제출", r3, "submit:S2"),
		stE("10:45", "t3", "b", "막힘", "-", "병합기: 착지 실패(종료 3, 충돌 x.go)", r3, "-"),
		stE("11:00", "t3", "b", "완료", B2, "고침", r3, "-"),
		stE("11:05", "t3", "b", "제출", B2, "제출", r3, "submit:S3"),
		stE8("2026-11-22", "12:30", "t3", "b", "정리됨", "-", "hc-sweep R2"),
		stE("11:00", "t4", "c", "완료", C, "일 끝", r4, "-"),
		stE("11:06", "t4", "c", "제출", C, "제출", r4, "submit:S4"),
		// D: 병합기 없이 레인이 직접 착지
		stE("10:00", "t5", "d", "완료", D, "일 끝", r5, "-"),
		stE("10:20", "t5", "d", "착지", D[:8], "main 착지", r5, "-"),
		// z: 작업 ID 칸이 없는 옛 줄(8칸)
		stE8("2026-11-22", "10:00", "t6", "z", "완료", "-", "옛 완료"),
		stE8("2026-11-22", "10:30", "t6", "z", "착지", "abcdef12", "옛 착지"),
		// 기간 밖(11-21) 착지 — 세지 않음
		stE8("2026-11-21", "10:30", "t7", "old", "착지", "abcdef34", "옛 착지"),
		// 실행 ID를 모르는 터미널의 정리됨
		stE8("2026-11-22", "12:40", "t9", "q", "정리됨", "-", "hc-sweep R2"),
	}
	ev = append(ev, ev[0]) // 같은 줄이 두 번 와도(--on 이 기계) 한 번
	jr = []string{
		stJ("11:20", "a", "S1", "10:40", "1", "10:50", "11:20", "착지", C1, "병합기 착지"),
		stJ("10:40", "b", "S2", "10:05", "1", "10:10", "10:40", "돌려보냄", "-", "착지 실패(종료 3, 충돌 x.go)"),
		stJ("12:10", "b", "S3", "11:05", "2", "11:40", "12:10", "착지", C2, "병합기 착지"),
		stJ("12:10", "c", "S4", "11:06", "2", "11:40", "12:10", "착지", C2, "병합기 착지"),
		// 제출 사건을 못 찾는 착지 · 착지 시작·끝 모름
		stJ("13:00", "y", "S9", "12:00", "1", "-", "-", "착지", C3, "병합기 착지"),
	}
	return ev, jr
}

func TestStatsReport(t *testing.T) {
	ev, jr := statsFixture()
	want := strings.Join([]string{
		"기간: 2026-11-22T00:00:00Z ~ 2026-11-23T00:00:00Z",
		"== 착지",
		"착지 5건 (병합기 3 · 레인 직접 2) · 묶음 크기 중앙 1 · 최대 2",
		"제출 → 착지: n 4 · 중앙 60분 · 상위 10% 65분",
		"완료 → 착지: n 4 · 중앙 70분 · 상위 10% 130분 · 모름 2(제출 사건 없음 1 · 작업 ID 없음 1)",
		"감사 통과 → 착지: n 1 · 중앙 50분 · 상위 10% 50분 · 판정 없음 3 · 모름 2(제출 사건 없음 1 · 작업 ID 없음 1)",
		"시간당 피크: 착지 2건(2026-11-22T10) · 완료 4건(2026-11-22T10)",
		"== 병합기 검증 실행 (착지 한 번 부름 = 무거운 검증 한 번)",
		"실행 3 · 착지로 끝남 2 · 돌려보냄 1 · 착지 건당 실행 1.50 · 모름 1(시작·끝 시각 없음 1)",
		"실행 시간: n 3 · 중앙 30분 · 상위 10% 30분 · 합 1.5시간(돌려보낸 것 포함)",
		"돌려보낸 까닭: 충돌(종료 3) 1",
		"== 레인 자리",
		"막힘 1",
		"자리 점유(터미널 생김 → 정리됨): n 2 · 중앙 150분 · 상위 10% 210분 · 모름 1(실행 ID 없음 1)",
		"완료 → 정리됨: n 2 · 중앙 90분 · 상위 10% 150분",
	}, "\n") + "\n"
	got := statsReport(ev, jr, "2026-11-22T00:00:00Z", "2026-11-23T00:00:00Z")
	if i := strings.Index(got, "기간:"); i >= 0 {
		got = got[i:] // 머리 「정의:」 줄은 따로 본다
	}
	if got != want {
		t.Errorf("표가 다르다\n--- 받음\n%s--- 바람\n%s", got, want)
	}
}

// 감사 atoxryx-kxtet1: 병합기가 「병합기 착지」 아닌 까닭으로 보낸 착지 사건(10칸은 병합기 터미널 실행)을 레인 직접으로 또 세지 않는다 ·
// 「이미 … 에 있음」 일지 착지는 기본 브랜치가 안 옮겨 가 따로 · 다른 기계에서 받은 같은 일지 줄은 한 번 · 제출 시각이 빈 줄은 모름
func TestStatsReportMergerReasons(t *testing.T) {
	A, C1 := strings.Repeat("a", 40), strings.Repeat("1", 40)
	ev := []string{
		stE("10:00", "t1", "a", "완료", A, "일 끝", "t1@1795338000000", "-"),
		stE("10:40", "t1", "a", "제출", A, "제출", "t1@1795338000000", "submit:S1"),
		stE("11:21", "t1", "a", "착지", C1, "앞 병합기 실행이 넣었음 — 제출 aaaaaaaa, 제출 ID S1", "t165@1795330000000", "-"),
		stE("11:40", "t1", "e", "착지", C1, "제출이 이미 main 에 있음 — 제출 bbbbbbbb, 큐 1795339000-e", "t165@1795330000000", "-"),
	}
	j := stJ("11:20", "a", "S1", "10:40", "1", "10:50", "11:20", "착지", C1, "앞 병합기 실행이 넣었음")
	nosub := strings.Split(stJ("11:30", "f", "S7", "-", "1", "11:00", "11:30", "착지", strings.Repeat("7", 40), "병합기 착지"), tab)
	nosub[5] = "" // jrec은 큐 이름에서 초를 못 읽으면 빈 글
	jr := []string{j, j, stJ("11:40", "e", "S8", "11:39", "1", "-", "-", "착지", C1, "제출이 이미 main 에 있음"), strings.Join(nosub, tab)}
	got := statsReport(ev, jr, "2026-11-22T00:00:00Z", "2026-11-23T00:00:00Z")
	for _, w := range []string{
		"착지 2건 (병합기 2 · 레인 직접 0) · 묶음 크기 중앙 1 · 최대 1 · 이미 들어 있음 1",
		"제출 → 착지: n 1 · 중앙 40분 · 상위 10% 40분 · 모름 1(제출 시각 없음 1)",
		"완료 → 착지: n 1 · 중앙 80분 · 상위 10% 80분 · 모름 1(제출 사건 없음 1)",
		"실행 2 · 착지로 끝남 2 · 돌려보냄 0 · 착지 건당 실행 1.00\n",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("%q 없음:\n%s", w, got)
		}
	}
}

// 수집 못 한 기계(hc exec 실패)는 종료 1, 이 기계 파일 하나라도 못 읽으면 이 기계 것은 둘 다 버린다
func TestStatsCmdCollectFail(t *testing.T) {
	d := t.TempDir()
	clitestPut(t, d+"/bin/hc", "#!/bin/sh\ncase \"$1\" in machine) echo '{\"machines\":[{\"name\":\"box\",\"state\":\"connected\"}]}';; *) exit 4;; esac\n", 0o755)
	t.Setenv("PATH", d+"/bin:"+os.Getenv("PATH"))
	t.Setenv("XDG_STATE_HOME", d+"/st")
	if rc := statsCmd([]string{"--all"}); rc != 1 {
		t.Errorf("hc exec 실패인데 종료 %d", rc)
	}
	clitestPut(t, d+"/st/hc-lane/merger-journal.tsv", stJ("11:20", "a", "S1", "10:40", "1", "10:50", "11:20", "착지", strings.Repeat("1", 40), "병합기 착지")+"\n", 0o644)
	if err := os.MkdirAll(d+"/st/hc-lane/events.tsv", 0o755); err != nil { // 폴더라 읽기 실패
		t.Fatal(err)
	}
	ev, jr, ok := statsLocal(d + "/st/hc-lane")
	if ok || len(ev)+len(jr) != 0 {
		t.Errorf("사건 파일을 못 읽었는데 ok=%v ev %d jr %d", ok, len(ev), len(jr))
	}
}

func clitestPut(t *testing.T, p, s string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), mode); err != nil {
		t.Fatal(err)
	}
}

func TestStatsReportDefinitionAndEmpty(t *testing.T) {
	got := statsReport(nil, nil, "", "2026-11-23T00:00:00Z")
	for _, w := range []string{"정의: ", "docs/land-stats-baseline.md", "기간: (처음) ~ 2026-11-23T00:00:00Z", "착지 0건", "제출 → 착지: n 0", "실행 0 ·"} {
		if !strings.Contains(got, w) {
			t.Errorf("빈 표에 %q 없음:\n%s", w, got)
		}
	}
}

func TestStatsRemoteParse(t *testing.T) {
	ok := "== events.tsv\nE1\nE2\nEND events.tsv 2\n== merger-journal.tsv\nEND merger-journal.tsv 0\n"
	ev, jr, err := statsParse(ok)
	if err != nil || len(ev) != 2 || len(jr) != 0 {
		t.Fatalf("맞는 출력: ev %v jr %v err %v", ev, jr, err)
	}
	for name, bad := range map[string]string{
		"잘림":     "== events.tsv\nE1\nEND events.tsv 2\n== merger-journal.tsv\nEND merger-journal.tsv 0\n",
		"일지 없음":  "== events.tsv\nE1\nEND events.tsv 1\n",
		"끝 줄 없음": "== events.tsv\nE1\nE2\n",
		"빈 출력":   "",
	} {
		if _, _, err := statsParse(bad); err == nil {
			t.Errorf("%s: 오류여야 한다", name)
		}
	}
}
