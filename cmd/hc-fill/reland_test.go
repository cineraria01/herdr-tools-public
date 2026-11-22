package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func TestRelandName(t *testing.T) {
	for _, c := range []struct {
		base string
		k    int
		want string
	}{
		{"fill-gc", 1, "fill-gc-reland"},
		{"fill-gc", 2, "fill-gc-reland2"},
		{"abcdefghij-abcdefghij-abcdefghij", 1, "abcdefghij-abcdefghij-abc-reland"},
		{"abcdefghij-abcdefghij-abcd-efgh", 2, "abcdefghij-abcdefghij-ab-reland2"},
		{"abcdefghijabcdefghijabcd-xyzw", 1, "abcdefghijabcdefghijabcd-reland"}, // 자른 끝 - 는 뗀다
	} {
		got := relandName(c.base, c.k)
		if got != c.want || !okname(got) {
			t.Errorf("relandName(%q,%d) = %q, 바람 %q", c.base, c.k, got, c.want)
		}
	}
}

// rlEv: 병합기 기계 box가 적은 반송 사건(cmd/hc-merger fail()의 글 꼴)
func rlEv(tm, lane, n, why, to string) string {
	msg := "병합기: " + why + " — 원본 aaaaaaaa, 기준 bbbbbbbb, 제출 ID box-" + lane + ", refs/merge-failed/" + n + " 로 옮김(lane/" + lane + " 는 그대로), 로그 /s/merger-logs/" + lane + ".log"
	return strings.Join([]string{tm, "box", "-", lane, "막힘", "-", msg, "-", to, "-", "-", "-"}, "\t")
}

// relandTick: rebase 충돌 반송만 다시 맞춤 줄로(같은 프로젝트·주인·높음·감사 끔), 검증 실패·since 앞·원격에 없는 반송은 안 넣음,
// 같은 바탕 레인은 reland_max번까지 그 뒤 막힘, 같은 반송은 두 번 넣지 않음
func TestRelandTick(t *testing.T) {
	setData(t)
	git := func(dir string, a ...string) {
		t.Helper()
		c := exec.Command("git", a...)
		c.Dir = dir
		if o, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, o)
		}
	}
	for k, v := range map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_AUTHOR_NAME": "t",
		"GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"} {
		t.Setenv(k, v)
	}
	o, w := DATA+"/o.git", DATA+"/w"
	git(DATA, "init", "-q", "--bare", "-b", "main", o)
	git(DATA, "init", "-q", "-b", "main", w)
	git(w, "commit", "-q", "--allow-empty", "-m", "base")
	git(w, "remote", "add", "origin", o)
	for _, n := range []string{"100-l-a", "300-l-a-reland", "400-l-a-reland2", "500-l-b"} {
		git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/"+n)
	}
	// 가짜 hc-lane: report 인자를 한 줄로
	os.WriteFile(DATA+"/hc-lane", []byte("#!/bin/sh\nprintf '%s|' \"$@\" >>"+DATA+"/reports\necho >>"+DATA+"/reports\n"), 0o755)
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	confLines = lines("local here\nrepo app here " + w + "\nmerger app box\n")
	LOCAL, MGM, dry, EVBAD, CONN = "here", " box", false, false, []string{"box"}
	MQP, MQS, MQBAD = "", nil, ""
	conflict := "착지 실패(종료 3, 충돌 a.go b.go)"
	EV = []string{rlEv("2026-11-22T00:00:00Z", "l-a", "100-l-a", conflict, "t9")}
	relandTick() // 첫 바퀴: 기록 파일만 만든다(그 앞 반송은 안 봄)
	if q := qRows(); len(q) != 0 || !isFile(DATA+"/reland.tsv") {
		t.Fatalf("첫 바퀴는 기록 파일만, 넣지 않는다: %v", q)
	}
	os.WriteFile(DATA+"/reland.tsv", []byte("#since\t2026-11-22T00:00:00Z\n"), 0o644)
	// 검증 실패 — 안 넣음 · 원격에 없음 — 기록만 · since 앞 — 안 봄 · 병합기 기계 아님 — 안 봄
	EV = append(EV,
		rlEv("2026-11-22T00:01:00Z", "l-c", "200-l-c", "착지 실패(종료 1)", "t9"),
		rlEv("2026-11-22T00:02:00Z", "l-d", "250-l-d", conflict, "t9"),
		rlEv("2026-11-21T23:00:00Z", "l-b", "500-l-b", conflict, "t9"),
		strings.Replace(rlEv("2026-11-22T00:03:00Z", "l-e", "260-l-e", conflict, "-"), "\tbox\t", "\tother\t", 1),
	)
	relandTick()
	r := qrow("l-a-reland")
	if r == nil || r.F(2) != "app" || r.F(6) != "wait" || r.F(12) != "t9" || r.F(13) != "high" || r.F(15) != "no-audit" {
		t.Fatalf("다시 맞춤 줄: %v", r)
	}
	if len(qRows()) != 1 {
		t.Fatalf("다른 반송은 넣지 않는다: %v", qRows())
	}
	b, _ := readFile(r.F(3))
	for _, s := range []string{"git fetch origin refs/merge-failed/100-l-a ", "충돌 파일(병합기가 3자 합침으로 본 것): a.go b.go", "/s/merger-logs/l-a.log", "머리 l-a:", "rebase --abort 금지"} {
		if !strings.Contains(b, s) {
			t.Errorf("지시문에 %q 없음:\n%s", s, b)
		}
	}
	rep, _ := readFile(DATA + "/reports")
	if !strings.Contains(rep, "report|중간|-|자동 다시 맞춤 넣음 l-a-reland — l-a") || !strings.Contains(rep, "--to|t9|--key|hf:reland:100-l-a|") {
		t.Fatalf("주인 알림: %q", rep)
	}
	relandTick() // 같은 반송을 다시 보지 않는다
	if rep, _ = readFile(DATA + "/reports"); len(qRows()) != 1 || strings.Count(rep, "\n") != 1 {
		t.Fatalf("두 번 넣음: %v", qRows())
	}
	// 다시 맞춤이 또 반송 → 둘째(-reland2), 그것도 반송 → 상한(2) 넘어 막힘
	os.WriteFile(Q, nil, 0o644)
	EV = append(EV, rlEv("2026-11-22T01:00:00Z", "l-a-reland", "300-l-a-reland", conflict, "t9"))
	relandTick()
	if qrow("l-a-reland2") == nil {
		t.Fatalf("둘째 다시 맞춤: %v", qRows())
	}
	EV = append(EV, rlEv("2026-11-22T02:00:00Z", "l-a-reland2", "400-l-a-reland2", conflict, "t9"))
	relandTick()
	rep, _ = readFile(DATA + "/reports")
	if len(qRows()) != 1 || !strings.Contains(rep, "report|막힘|-|l-a-reland2 병합기 rebase 충돌로 또 반송 — 자동 다시 맞춤 2번 넘음") {
		t.Fatalf("상한 막힘: %v %q", qRows(), rep)
	}
	rl, _ := readFile(DATA + "/reland.tsv")
	for _, s := range []string{"250-l-d\tl-d\tl-d\t없음", "100-l-a\tl-a\tl-a\t넣음\tl-a-reland", "400-l-a-reland2\tl-a-reland2\tl-a\t상한"} {
		if !strings.Contains(rl, s) {
			t.Errorf("기록에 %q 없음:\n%s", s, rl)
		}
	}
	// 끔(reland_max 0) — 원격에 실제 있는 새 반송이어도 대기열·처리 장부·주인 알림이 모두 그대로(R-31: 원격에 없는 반송은 켜져 있어도 줄이 안 늘어 끄기를 못 가렸다)
	confLines = append(confLines, "reland_max 0")
	git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/600-l-f")
	EV = append(EV, rlEv("2026-11-22T03:00:00Z", "l-f", "600-l-f", conflict, "t9"))
	snap := func() string {
		q, _ := readFile(Q)
		rl, _ := readFile(DATA + "/reland.tsv")
		rp, _ := readFile(DATA + "/reports")
		return q + "\x00" + rl + "\x00" + rp
	}
	before := snap()
	MQP, MQS, MQBAD = "", nil, ""
	relandTick()
	if after := snap(); after != before {
		t.Fatalf("reland_max 0이면 끔 — 바뀜:\n전 %q\n뒤 %q", before, after)
	}
}

// rlFakeGit: PATH 앞(DATA)에 git 대역 — refs/merge-failed/ 조회만 DATA/lsmode에 따라 꾸미고(part: 「deadbeef」 한 낱말·0,
// empty0: 빈 출력·0, fail: 128, trail: 진짜 응답 뒤 꼴 틀린 줄), 나머지는 진짜 git. 조회 인자는 DATA/lsr.log에
func rlFakeGit(t *testing.T) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	sh := "#!/bin/sh\ncase \"$*\" in *refs/merge-failed/*)\n  echo \"$*\" >>" + DATA + "/lsr.log\n" +
		"  case $(cat " + DATA + "/lsmode 2>/dev/null) in\n" +
		"    part) echo deadbeef; exit 0 ;;\n    empty0) exit 0 ;;\n    fail) exit 128 ;;\n" +
		"    trail) \"" + real + "\" \"$@\"; echo garbage; exit 0 ;;\n  esac ;;\nesac\nexec \"" + real + "\" \"$@\"\n"
	if err := os.WriteFile(DATA+"/git", []byte(sh), 0o755); err != nil {
		t.Fatal(err)
	}
}

// R-15: 원격 참조 조회는 「<40/64자 16진>\trefs/merge-failed/<n>」 줄만 있을 때 있음, --exit-code 종료 2·빈 출력일 때만 없음,
// 나머지(한 낱말·빈 출력+0·실패·꼴 틀린 줄)는 모름 — 모름이면 장부·대기열을 안 바꾸고 다음 바퀴에 다시 본다
func TestRelandFindStrict(t *testing.T) {
	w, git := rlRepo(t)
	CONN = []string{"box"}
	git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/100-l-a")
	rlFakeGit(t)
	for _, c := range []struct{ mode, n, want string }{
		{"part", "100-l-a", "모름"}, {"empty0", "100-l-a", "모름"}, {"fail", "100-l-a", "모름"}, {"trail", "100-l-a", "모름"},
		{"part", "900-l-z", "모름"}, {"empty0", "900-l-z", "모름"},
		{"real", "100-l-a", "있음"}, {"real", "900-l-z", "없음"},
	} {
		os.WriteFile(DATA+"/lsmode", []byte(c.mode), 0o644)
		if p, st := relandFind(c.n); st != c.want || (st == "있음") != (p == "app") {
			t.Errorf("%s %s: %q %q, 바람 %s", c.mode, c.n, p, st, c.want)
		}
	}
	if l, _ := readFile(DATA + "/lsr.log"); !strings.Contains(l, "ls-remote --exit-code origin refs/merge-failed/") {
		t.Errorf("--exit-code로 묻지 않음: %q", l)
	}
	// 빈 출력+0 뒤 다음 바퀴에 정상 응답 — 첫 바퀴는 아무것도 안 적고, 둘째 바퀴에 넣는다
	EV = []string{rlEv("2026-11-22T00:01:00Z", "l-a", "100-l-a", "착지 실패(종료 3, 충돌 a.go)", "t9")}
	os.WriteFile(DATA+"/lsmode", []byte("empty0"), 0o644)
	MQP, MQS, MQBAD = "", nil, ""
	relandTick()
	if rl, _ := readFile(DATA + "/reland.tsv"); strings.Contains(rl, "100-l-a") || len(qRows()) != 0 {
		t.Fatalf("모름인데 적음: %q %v", rl, qRows())
	}
	os.WriteFile(DATA+"/lsmode", []byte("real"), 0o644)
	MQP, MQS, MQBAD = "", nil, ""
	relandTick()
	if qrow("l-a-reland") == nil {
		t.Fatalf("복구 뒤 넣지 않음: %v", qRows())
	}
}

// R-17: 기능을 켠 첫 정상 바퀴(사건 0개여도)에 #since를 적고, 그 뒤 도착한 반송은 처리한다.
// 사건을 못 읽은 바퀴·병합기 기계가 연결 안 된 바퀴는 기준을 적지 않는다(그 사이 반송을 앞 이력으로 버리지 않게)
func TestRelandSinceFirst(t *testing.T) {
	w, git := rlRepo(t)
	os.Remove(DATA + "/reland.tsv")
	EV, CONN = nil, nil // box 연결 안 됨 — 그 기계의 사건을 못 봤다
	relandTick()
	if isFile(DATA + "/reland.tsv") {
		t.Fatal("병합기 기계 사건을 못 본 바퀴에 기준을 적음")
	}
	CONN, EVBAD = []string{"box"}, true
	relandTick()
	EVBAD = false
	if isFile(DATA + "/reland.tsv") {
		t.Fatal("사건을 못 읽은 바퀴에 기준을 적음")
	}
	relandTick() // 첫 정상 바퀴, 사건 0개
	rl, _ := readFile(DATA + "/reland.tsv")
	if !strings.HasPrefix(rl, "#since\t") {
		t.Fatalf("첫 정상 바퀴에 기준 없음: %q", rl)
	}
	git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/100-l-a")
	EV = []string{rlEv(strings.TrimPrefix(lines(rl)[0], "#since\t"), "l-a", "100-l-a", "착지 실패(종료 3, 충돌 a.go)", "t9")}
	MQP, MQS, MQBAD = "", nil, ""
	relandTick()
	if qrow("l-a-reland") == nil {
		t.Fatalf("켠 뒤 도착한 반송을 처리하지 않음: %v", qRows())
	}
}

// R-18: 31자 원래 이름의 사슬 — 앞 등록 이름의 바탕을 장부에서 찾아 사슬 전체에 reland_max(2)를 건다(잘린 이름에서 새 바탕을 만들지 않는다)
func TestRelandLongChain(t *testing.T) {
	w, git := rlRepo(t)
	CONN = []string{"box"}
	long := "abcdefghij-abcdefghij-abcdefghi" // 31자
	conflict := "착지 실패(종료 3, 충돌 a.go)"
	EV = nil
	lane := long
	for i := 1; i <= 4; i++ {
		n := strconv.Itoa(i) + "00-x" + strconv.Itoa(i)
		git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/"+n)
		EV = append(EV, rlEv("2026-11-22T0"+strconv.Itoa(i)+":00:00Z", lane, n, conflict, "t9"))
		os.WriteFile(Q, nil, 0o644)
		MQP, MQS, MQBAD = "", nil, ""
		relandTick()
		if q := qRows(); len(q) == 1 {
			lane = q[0].F(1)
		}
	}
	rl, _ := readFile(DATA + "/reland.tsv")
	if c := strings.Count(rl, "\t넣음\tabcdefghij"); c != 2 { // 결과 칸이 넣음인 줄(알림 줄은 「알림\t넣음\t<시각>」)
		t.Fatalf("자동 등록 %d번, 바람 2:\n%s", c, rl)
	}
	for _, s := range []string{"100-x1\t" + long + "\t" + long + "\t넣음\tabcdefghij-abcdefghij-abc-reland\t",
		"200-x2\tabcdefghij-abcdefghij-abc-reland\t" + long + "\t넣음\tabcdefghij-abcdefghij-ab-reland2\t",
		"300-x3\tabcdefghij-abcdefghij-ab-reland2\t" + long + "\t상한"} {
		if !strings.Contains(rl, s) {
			t.Errorf("기록에 %q 없음:\n%s", s, rl)
		}
	}
}

// R-19: 등록과 알림을 따로 적는다 — 알림이 실패하면(종료 3) 다음 바퀴에 같은 키로 다시, 된 뒤에는 다시 안 보낸다
func TestRelandNotifyRetry(t *testing.T) {
	w, git := rlRepo(t)
	CONN = []string{"box"}
	os.WriteFile(DATA+"/hc-lane", []byte("#!/bin/sh\nprintf '%s|' \"$@\" >>"+DATA+"/reports\necho >>"+DATA+"/reports\n"+
		"[ ! -e "+DATA+"/report-fail ] || exit 3\n"), 0o755)
	git(w, "push", "-q", "origin", "HEAD:refs/merge-failed/100-l-a")
	EV = []string{rlEv("2026-11-22T00:01:00Z", "l-a", "100-l-a", "착지 실패(종료 3, 충돌 a.go)", "t9")}
	os.WriteFile(DATA+"/report-fail", nil, 0o644)
	relandTick()
	if qrow("l-a-reland") == nil {
		t.Fatalf("등록 안 됨: %v", qRows())
	}
	os.Remove(DATA + "/report-fail")
	for i := 0; i < 2; i++ {
		MQP, MQS, MQBAD = "", nil, ""
		relandTick()
	}
	rep, _ := readFile(DATA + "/reports")
	key := "--key|hf:reland:100-l-a|"
	if strings.Count(rep, "\n") != 2 || strings.Count(rep, key) != 2 || len(qRows()) != 1 {
		t.Fatalf("알림 재시도(실패 1·성공 1, 같은 키): %q %v", rep, qRows())
	}
}
