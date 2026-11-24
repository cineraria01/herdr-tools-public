package main

// 비슷한 레인 알림 오인 줄이기(dupwarn.go, 레인 dup-warn-noise) — 2026-11-22 실사용에서 상용구 경로·끝난 레인으로 20줄 오인

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// dupRepo: 저장소 하나(파일들은 빈 파일로) 만들고 그 프로젝트 app을 local에 둔다. 추가 설정 줄 extra
func dupRepo(t *testing.T, files []string, extra string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	setData(t)
	repo := t.TempDir()
	for _, f := range files {
		p := repo + "/" + f
		if strings.HasSuffix(f, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if i := strings.LastIndexByte(p, '/'); i > 0 {
			if err := os.MkdirAll(p[:i], 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dupGit(t, repo, "init", "-q", "-b", "main")
	dupGit(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "처음")
	LOCAL = "here"
	MQP, MQBAD, MQS = "", "", nil
	confLines = lines("local here\nrepo app here " + repo + "\n" + extra)
	return repo
}

func dupGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
}

func dupRow(n, b, st, addr string) string {
	return strings.Join(pad16(n, "app", b, "-", "-", st, "here", addr, "-", "x", "-", "t1", "normal", "-", "-", "-"), "\t") + "\n"
}

// 이번 실제 꼴: 모든 지시문이 프로젝트 이름·<레인> 자리표시·검증 명령·공통 파일을 적는다 → 제목이 다르면 알림 없음,
// 제목이 비슷해도 「언급 경로」 이유에 상용구가 끼지 않는다
func TestDupWarnBoilerplate(t *testing.T) {
	dupRepo(t, []string{"app/", "docs/prd/pending/", ".claude/herdr-verify.sh", "lib/common.go", "src/rank.go", "src/find.go"}, "")
	d := t.TempDir()
	boiler := "## 공통 규칙\n- `app` 저장소. 보고는 docs/prd/pending/<레인>.md 에.\n- 검증: .claude/herdr-verify.sh · lib/common.go 를 따른다\n- app/ 아래만\n"
	titles := []string{"화면 글꼴 바꾸기", "결제 영수증 메일", "로그인 쿠키 만료", "검색 결과 정렬 순서", "알림 소리 크기"}
	var q string
	for i, tt := range titles {
		n := "old" + string(rune('a'+i))
		q += dupRow(n, dupBrief(t, d, n, "# 레인 "+n+" — "+tt+" (app, 지휘 t1, 2026-11-22)\n"+boiler), "wait", "-")
	}
	sim := dupBrief(t, d, "sim", "# 레인 sim — 순위 원본 찾기 조정 (app, 지휘 t2, 2026-11-22)\n"+boiler+"src/rank.go 와 src/find.go\n")
	q += dupRow("sim", sim, "up", "here/w1")
	if err := os.WriteFile(Q, []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	mine := dupBrief(t, d, "rank-orig-find-tune", "# 레인 rank-orig-find-tune — 순위 원본 찾기 조정 (app, 지휘 t1, 2026-11-22)\n"+boiler+"src/rank.go 와 src/find.go\n")

	// 상용구는 키에서 빠진다: 프로젝트 이름(app/)·자리표시. 흔한 경로(.claude/herdr-verify.sh·lib/common.go)는 동적으로
	ks := dupKeys("app", mine)
	for _, k := range ks {
		if p := k.F(kyKey); p == "app/" || strings.Contains(p, "<") {
			t.Fatalf("상용구 키가 남음: %q (%v)", p, ks)
		}
	}
	var all [][]Row
	for _, r := range qRows() {
		all = append(all, dupKeys("app", r.F(qBrief)))
	}
	all = append(all, ks)
	c := dupCommon(all, 0.3)
	if !c[".claude/herdr-verify.sh"] || !c["lib/common.go"] || c["src/rank.go"] {
		t.Fatalf("흔한 경로 %v", c)
	}
	if len(dupCommon(all[:4], 0.3)) != 0 {
		t.Fatal("지시문 5개 미만이면 흔한 경로를 세지 않아야")
	}

	got := dupCapture(t, func() { dupWarn("rank-orig-find-tune", "app", mine) })
	want := "알림: 비슷한 레인 sim (주인 t1, 떠 있음 here/w1) — 제목 낱말 100% 겹침, 언급 경로 2개 겹침(src/rank.go,src/find.go)\n"
	if got != want {
		t.Fatalf("알림\n got %q\nwant %q", got, want)
	}
	// 상용구만 적은 다른 제목 지시문 → 조용
	other := dupBrief(t, d, "other", "# 레인 other — 배포 스크립트 시간 상한 (app, 지휘 t1, 2026-11-22)\n"+boiler)
	if got := dupCapture(t, func() { dupWarn("other", "app", other) }); got != "" {
		t.Fatalf("상용구만 겹침은 조용해야: %q", got)
	}
	// 흔한 경로 거르기를 끄면(비율 0) 상용구가 언급 겹침에 들어온다 — 설정 키가 먹는다
	confLines = append(confLines, "dup_warn 0.5 2 0")
	if got := dupCapture(t, func() { dupWarn("rank-orig-find-tune", "app", mine) }); !strings.Contains(got, "언급 경로 4개") {
		t.Fatalf("흔한 경로 비율 0: %q", got)
	}
}

// 예약(「파일:」·「쓰기:」) 겹침은 제목이 전혀 달라도 알린다
func TestDupWarnReserved(t *testing.T) {
	dupRepo(t, []string{"src/rank.go"}, "")
	d := t.TempDir()
	q := dupRow("res", dupBrief(t, d, "res", "# 레인 res — 화면 글꼴 바꾸기\n파일: src/rank.go\n"), "wait", "-") +
		dupRow("men", dupBrief(t, d, "men", "# 레인 men — 결제 영수증 메일\nsrc/rank.go 를 읽는다\n"), "wait", "-")
	if err := os.WriteFile(Q, []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	mine := dupBrief(t, d, "new", "# 레인 new — 로그인 쿠키 만료\n- 쓰기: src/rank.go\n")
	got := dupCapture(t, func() { dupWarn("new", "app", mine) })
	if got != "알림: 비슷한 레인 res (주인 t1, 대기) — 예약 1개 겹침(src/rank.go)\n" {
		t.Fatalf("예약 겹침만 알려야: %q", got)
	}
}

// 떠 있어도 끝난 레인(완료·제출 보고, 병합기 큐에 냄)은 안 본다 — 작업 중(중간 보고)인 것만
func TestDupWarnSkipsFinished(t *testing.T) {
	repo := dupRepo(t, nil, "merger app here\n")
	bare := t.TempDir()
	dupGit(t, bare, "init", "-q", "--bare")
	dupGit(t, repo, "remote", "add", "origin", bare)
	dupGit(t, repo, "push", "-q", "origin", "HEAD:refs/merge-queue/1-mq-lane")
	d := t.TempDir()
	title := "# 레인 %s — 순위 원본 찾기 조정 (app, 지휘 t1, 2026-11-22)\n본문\n"
	b := func(n string) string { return dupBrief(t, d, n, strings.Replace(title, "%s", n, 1)) }
	rDone, rSent, rLive := dupRow("done-lane", b("done-lane"), "up", "here/w1"), dupRow("sent-lane", b("sent-lane"), "up", "here/w2"),
		dupRow("live", b("live"), "up", "here/w3")
	q := rDone + rSent + rLive + dupRow("mq-lane", b("mq-lane"), "up", "here/w4")
	if err := os.WriteFile(Q, []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	rid := func(l string) string { return splitRow(strings.TrimSuffix(l, "\n")).F(qRID) }
	last := strings.Join([]string{"done-lane", "완료", "abc12345", "14:00", "끝", "14:01", "fresh", rid(rDone)}, "\t") + "\n" +
		strings.Join([]string{"sent-lane", "제출", "abc12346", "14:00", "제출", "14:01", "fresh", rid(rSent)}, "\t") + "\n" +
		strings.Join([]string{"live", "중간", "-", "14:00", "하는 중", "14:01", "fresh", rid(rLive)}, "\t") + "\n"
	if err := os.WriteFile(DATA+"/last.tsv", []byte(last), 0o644); err != nil {
		t.Fatal(err)
	}
	got := dupCapture(t, func() { dupWarn("new", "app", b("new")) })
	if got != "알림: 비슷한 레인 live (주인 t1, 떠 있음 here/w3) — 제목 낱말 100% 겹침\n" {
		t.Fatalf("작업 중인 live만: %q", got)
	}
	// 같은 이름이라도 다른 줄 ID(앞 실행)의 완료 보고는 이 줄을 끝난 것으로 보지 않는다
	if err := os.WriteFile(DATA+"/last.tsv", []byte(strings.Replace(last, rid(rDone), "other-id", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := dupCapture(t, func() { dupWarn("new", "app", b("new")) }); !strings.Contains(got, "done-lane") {
		t.Fatalf("다른 줄 ID의 보고: %q", got)
	}
}

// 알림이 여럿이면 가장 비슷한 3개만 + 「외 N」
func TestDupWarnCap(t *testing.T) {
	dupRepo(t, []string{"src/rank.go"}, "")
	d := t.TempDir()
	var q string
	for _, n := range []string{"a1", "a2", "a3", "a4", "a5"} {
		q += dupRow(n, dupBrief(t, d, n, "# 레인 "+n+" — 순위 원본 찾기 조정\n"), "wait", "-")
	}
	q += dupRow("half", dupBrief(t, d, "half", "# 레인 half — 순위 원본 찾기 다른 것 또 하나\n"), "wait", "-")
	q += dupRow("res", dupBrief(t, d, "res", "# 레인 res — 화면 글꼴\n파일: src/rank.go\n"), "wait", "-")
	if err := os.WriteFile(Q, []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	mine := dupBrief(t, d, "new", "# 레인 new — 순위 원본 찾기 조정\n파일: src/rank.go\n")
	got := strings.Split(strings.TrimSuffix(dupCapture(t, func() { dupWarn("new", "app", mine) }), "\n"), "\n")
	if len(got) != 4 || !strings.Contains(got[0], " res ") || !strings.Contains(got[1], " a1 ") || !strings.Contains(got[2], " a2 ") ||
		got[3] != "알림: 비슷한 레인 외 4개(가장 비슷한 3개만)" {
		t.Fatalf("상한 3: %q", got)
	}
}
