//go:build !windows

package main

// PR 거울(pr-surface): 가짜 gh(부른 인자·토큰 유무·본문을 적음)·가짜 git(레인 작업 트리의 원격 주소·push)·가짜 hc-lane(운영 사건)으로
// 한 바퀴의 거울 단계(prTick)를 본다. 실제 PR은 열지 않는다

import (
	"io"
	"os"
	"strings"
	"testing"

	"herdr-tools/internal/events"
)

const prTGh = `#!/bin/sh
tok=unset; [ -z "$GH_TOKEN" ] || tok=set
echo "gh TOKEN=$tok $*" >>"$HC_T_LOG"
[ -z "$HC_T_GHFAIL" ] || { echo "HTTP 502" >&2; exit 1; }
for a; do [ "$a" != --body-file ] || { printf '%s\n=====\n' "$(cat)" >>"$HC_T_DIR/body.log"; break; }; done
case "$1 $2" in
"pr create") echo "https://github.com/example-org/proj/pull/7" ;;
"pr view") echo "${HC_T_PRSTATE:-MERGED}" ;;
"pr review") [ -z "$HC_T_OWN" ] || { echo "failed: Can not approve your own pull request" >&2; exit 1; } ;;
esac
exit 0
`

const prTGit = `#!/bin/sh
case "$1 $2" in
"remote get-url") [ -z "$HC_T_NOURL" ] || exit 2; echo "git@github.com:example-org/proj.git" ;;
"push -q") echo "git $*" >>"$HC_T_LOG" ;;
*) exit 9 ;;
esac
`

const prTLane = `#!/bin/sh
echo "hc-lane $*" >>"$HC_T_LOG"
`

// 가짜 hc: hc get은 늘 실패(모름) — 살아 있는 표(TERMS)의 레인도 joblast가 주소·이름으로 사건을 고른다(전과 같은 길)
const prTHc = "#!/bin/sh\nexit 1\n"

type prT struct {
	t        *testing.T
	dir, log string
}

// 가짜 40자 sha(비밀 꼴 검사에 걸리지 않게 실행 중에 만든다)
var prSHA1, prSHA2 = strings.Repeat("1", 40), strings.Repeat("2", 40)

// prSetup: 레인 w1(프로젝트 app, 이 기계 here, 주소 here/w1, 작업 트리 있음), 사건 없음, 설정 extra
func prSetup(t *testing.T, extra ...string) *prT {
	t.Helper()
	knobFix(t)
	t.Setenv("HOME", HOME)
	k := &prT{t: t, dir: t.TempDir()}
	k.log = k.dir + "/log"
	repo := k.dir + "/app"
	if err := os.MkdirAll(repo+".wt/w1", 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := k.dir + "/cfg"
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	wr(t, cfg+"/private-patterns.txt", "# 시험\nsecret-project\nbox-[0-9]+\n")
	t.Setenv("HERDR_TOOLS_CONFIG", cfg)
	for n, s := range map[string]string{"gh": prTGh, "git": prTGit, "hc-lane": prTLane, "hc": prTHc} {
		wr(t, k.dir+"/"+n, s)
		if err := os.Chmod(k.dir+"/"+n, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", k.dir+":"+os.Getenv("PATH"))
	t.Setenv("HC_T_LOG", k.log)
	t.Setenv("HC_T_DIR", k.dir)
	for _, v := range []string{"HC_T_GHFAIL", "HC_T_OWN", "HC_T_NOURL", "GH_TOKEN"} {
		t.Setenv(v, "")
	}
	Q, AT, LOCAL, TERMS, LSSNAP, EVBAD, EV, MGM = DATA+"/queue.tsv", DATA+"/audit.tsv", "here", nil, true, false, nil, "mg"
	MQP, MQBAD, MQS = ",app", "", nil // 병합 줄은 읽었고 비어 있다(시험이 MQS에 더한다)
	t.Cleanup(func() { LSSNAP, EV, MGM, MQP, MQBAD, MQS = false, nil, "", "", "", nil })
	// 이 프로젝트의 거울은 옛날에 켰다(pr-inbox-no-retro — 켜기 전 보고로는 열지 않는다. 그 시험은 pr_noretro_test.go)
	wr(t, DATA+"/pr-on.tsv", "app\tpr\t2000-01-01T00:00:00Z\napp\tinbox\t2000-01-01T00:00:00Z\n")
	pcRef = map[string]pcRes{repo: {"origin/main", true}}
	confLines = append([]string{"repo app here " + repo}, extra...)
	wr(t, k.dir+"/brief.md", "# 기능 X 지시문\n본문\n")
	k.row("w1")
	return k
}

func (k *prT) row(name string) {
	q := []string{name, "app", k.dir + "/brief.md", "-", "-", "up", "here", "here/" + name, "-", "t", "-", "c/coord", "normal", "-", "auto", "-", "r-" + name}
	TERMS = append(TERMS, Term{M: "here", Addr: "here/" + name, Name: name, Status: "working"}) // 살아 있는 레인
	f, err := os.OpenFile(Q, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		k.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(strings.Join(q, "\t") + "\n"); err != nil {
		k.t.Fatal(err)
	}
}

// ev: 사건 한 줄을 EV에(바퀴가 읽은 것처럼)
func (k *prT) ev(at, mach, lane, state, sha, msg string) {
	EV = append(EV, events.Event{Time: at, Machine: mach, Term: mach + "/" + lane, Lane: lane, State: state, SHA: sha, Msg: msg, Turn: "1"}.Line())
}

func (k *prT) logged() string { b, _ := os.ReadFile(k.log); return string(b) }

func (k *prT) n(s string) int { return strings.Count(k.logged(), s) }

func (k *prT) bodies() string { b, _ := os.ReadFile(k.dir + "/body.log"); return string(b) }

// verdict: 받은 감사 판정 차수 a<n>(w1, sha, 높음.중간.낮음, 보고서 내용)
func (k *prT) verdict(id, v, hml, report string) {
	rep := "~/.local/share/hc-fill/reports/audit-" + id + ".md"
	if err := os.MkdirAll(HOME+"/.local/share/hc-fill/reports", 0o755); err != nil {
		k.t.Fatal(err)
	}
	wr(k.t, HOME+"/.local/share/hc-fill/reports/audit-"+id+".md", report)
	at := []string{id, "w1", "1", "w1@1", prSHA1, "here", "here/w1", "deliver", "0", v, "0", "0", "-", "-", "0", "hml=" + hml, rep}
	f, err := os.OpenFile(AT, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		k.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(strings.Join(at, "\t") + "\n"); err != nil {
		k.t.Fatal(err)
	}
}

// captured: f를 부르는 동안의 표준 출력
func captured(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	f()
	w.Close()
	os.Stdout = old
	return <-done
}

// ① 꺼짐: gh·git 0번, 출력 없음, 거울 파일 없음(다른 프로젝트만 켰으면 이 레인은 건드리지 않는다)
func TestPROff(t *testing.T) {
	for _, cfg := range [][]string{nil, {"pr -"}, {"pr other"}} {
		k := prSetup(t, cfg...)
		k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "기능 X 끝")
		out := captured(t, prTick)
		if k.logged() != "" || out != "" {
			t.Fatalf("%v: 꺼졌는데 불렀다\nlog=%s\nout=%s", cfg, k.logged(), out)
		}
		if pr, _ := prOf("w1"); pr != "" {
			t.Fatalf("%v: PR %s", cfg, pr)
		}
		for _, f := range []string{"pr.tsv", "pr-sent.tsv", "pr-ops.tsv", "pr-fail"} {
			if cfg != nil && cfg[0] == "pr other" {
				break
			}
			if isFile(DATA + "/" + f) {
				t.Fatalf("%v: 꺼졌는데 %s 를 썼다", cfg, f)
			}
		}
	}
}

// ② 완료 → PR 한 번(머리 가지를 숨은 참조 + gh api로), 다시 같은 바퀴 → 그대로, 새 완료 → 새 PR 없이 댓글 한 줄
func TestPROpenOnce(t *testing.T) {
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "기능 X 끝 (감사 hc-fill 자동)")
	prTick()
	if k.n("gh TOKEN=unset pr create") != 1 || k.n("git push -q origin +"+prSHA1+":refs/herdr/pr-src/w1") != 1 || k.n("git/refs/heads/pr/w1 -f sha="+prSHA1) != 1 {
		t.Fatalf("열기:\n%s", k.logged())
	}
	if !strings.Contains(k.logged(), "--head pr/w1") || !strings.Contains(k.logged(), "--base main") || !strings.Contains(k.logged(), "--title w1 — 기능 X 끝") {
		t.Fatalf("머리·기준·제목:\n%s", k.logged())
	}
	if b := k.bodies(); !strings.Contains(b, "기능 X 지시문") || !strings.Contains(b, "손으로 머지하지 않는다") {
		t.Fatalf("본문: %s", b)
	}
	if pr, u := prOf("w1"); pr != "PR #7" || !strings.HasSuffix(u, "/pull/7") {
		t.Fatalf("prOf = %q %q", pr, u)
	}
	prTick()
	if k.n("pr create") != 1 || k.n("pr comment") != 0 {
		t.Fatalf("같은 완료를 다시 썼다:\n%s", k.logged())
	}
	k.ev("2026-11-24T02:00:00Z", "here", "w1", "완료", prSHA2, "고침")
	prTick()
	if k.n("pr create") != 1 || k.n("pr comment 7") != 1 || !strings.Contains(k.bodies(), "다시 완료 22222222 — 고침") {
		t.Fatalf("다시 완료:\n%s\n%s", k.logged(), k.bodies())
	}
}

// ③ 판정 → 댓글 한 번 + 커밋 상태(작성자 같음 — 리뷰 0), 다시 바퀴를 돌아도 하나
func TestPRVerdictSameAuthor(t *testing.T) {
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	k.verdict("a1", "pass", "0.0.2", "## 보고\n[낮음] 하나\n")
	prTick()
	prTick()
	if k.n("pr review") != 0 || k.n("pr comment 7") != 1 || k.n("statuses/"+prSHA1) != 1 {
		t.Fatalf("판정:\n%s", k.logged())
	}
	if !strings.Contains(k.logged(), "context=herdr/audit") || !strings.Contains(k.logged(), "state=success") || !strings.Contains(k.logged(), "높음 0·중간 0·낮음 2 · 1차") {
		t.Fatalf("상태:\n%s", k.logged())
	}
	if b := k.bodies(); !strings.Contains(b, "감사 1차 통과 — 높음 0·중간 0·낮음 2") || !strings.Contains(b, "[낮음] 하나") || !strings.Contains(b, "<!-- hc-fill-pr audit:a1 -->") {
		t.Fatalf("본문:\n%s", b)
	}
}

// 작성자 다름(토큰으로 엶) → 통과는 승인 리뷰 1, 지적은 변경 요청 1, 댓글 0. 자기 PR이라 거절되면 댓글로
func TestPRVerdictOtherAuthor(t *testing.T) {
	k := prSetup(t, "pr app", "pr_token_cmd echo tok-xyz-987")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	if k.n("gh TOKEN=set pr create") != 1 || k.n("gh TOKEN=set api -X PATCH") != 1 {
		t.Fatalf("토큰으로 열기·머리 가지:\n%s", k.logged())
	}
	k.verdict("a1", "fix", "1.0.0", "[높음] 고칠 것\n")
	prTick()
	if k.n("--request-changes") != 1 || k.n("--approve") != 0 || k.n("pr comment") != 0 || k.n("gh TOKEN=unset pr review") != 1 {
		t.Fatalf("지적:\n%s", k.logged())
	}
	k.verdict("a2", "pass", "0.0.0", "깨끗\n")
	prTick()
	if k.n("--approve") != 1 || k.n("state=failure") != 1 || k.n("state=success") != 1 {
		t.Fatalf("통과:\n%s", k.logged())
	}
	t.Setenv("HC_T_OWN", "1")
	k.verdict("a3", "pass", "0.0.0", "또\n")
	prTick()
	if k.n("pr comment 7") != 1 || k.n("--approve") != 2 {
		t.Fatalf("자기 PR 거절 → 댓글:\n%s", k.logged())
	}
}

// 토큰 명령이 실패 → 기본 로그인으로 열고 운영 사건 한 번. 토큰 값은 출력·기록·파일 어디에도 없다
func TestPRToken(t *testing.T) {
	k := prSetup(t, "pr app", "pr_token_cmd exit 3")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	prTick()
	if k.n("gh TOKEN=unset pr create") != 1 || k.n("--name hc-fill-pr") != 1 {
		t.Fatalf("토큰 실패:\n%s", k.logged())
	}
	k = prSetup(t, "pr app", "pr_token_cmd printf 'tok-SECRET-4242\\n'")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	out := captured(t, prTick)
	if k.n("gh TOKEN=set pr create") != 1 {
		t.Fatalf("토큰으로 열지 않음:\n%s", k.logged())
	}
	all := out + k.logged() + k.bodies()
	des, _ := os.ReadDir(DATA)
	for _, d := range des {
		b, _ := os.ReadFile(DATA + "/" + d.Name())
		all += string(b)
	}
	if strings.Contains(all, "SECRET-4242") {
		t.Fatalf("토큰 값이 남았다:\n%s", all)
	}
}

// 착지: 병합기가 머리를 착지 커밋에 둠 → 댓글·검증 상태, 닫지 않음(Merged). 머리 갱신 실패 → 까닭 댓글 뒤 닫음. 돌려보냄 → 댓글·실패 상태
func TestPRLanded(t *testing.T) {
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	k.ev("2026-11-24T01:10:00Z", "mg", "w1", "막힘", "-", "병합기: 착지 실패(종료 1) — 제출 ID box-1, 로그 x")
	k.ev("2026-11-24T01:20:00Z", "mg", "w1", "착지", prSHA2, "병합기 착지, 직접 검증 9초 — 제출 11111111, 제출 ID box-1 · PR 머리 22222222 들어감")
	prTick()
	if k.n("pr close") != 0 || k.n("state=success") != 1 || k.n("state=failure") != 1 || k.n("pr comment 7") != 2 {
		t.Fatalf("Merged:\n%s", k.logged())
	}
	if b := k.bodies(); !strings.Contains(b, "돌려보냄 — 병합기: 착지 실패") || !strings.Contains(b, "착지 22222222 — 병합기 착지") || strings.Contains(b, "box-1") {
		t.Fatalf("본문(가림 포함):\n%s", b)
	}
	m, _ := prLoad()
	if m["w1"].State != "merged" {
		t.Fatalf("상태 %q", m["w1"].State)
	}
	k2 := prSetup(t, "pr app")
	k2.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	k2.ev("2026-11-24T01:20:00Z", "mg", "w1", "착지", prSHA2, "병합기 착지 — 제출 ID x · PR 머리 안 들어감(가지 갱신 실패 — Merged 표시 없음)")
	prTick()
	if k2.n("pr close 7") != 1 || !strings.Contains(k2.bodies(), "가지 갱신 실패로 Merged 표시 없음") {
		t.Fatalf("닫기:\n%s\n%s", k2.logged(), k2.bodies())
	}
	// ht-smalls-1129: 병합기가 적은 까닭(이미 들어 있음·경합)을 닫는 댓글에 그대로 — 닫기 동작은 같다
	for _, c := range []struct{ why, want string }{
		{"이미 들어 있어 올릴 머리 없음", "이미 들어 있어 올릴 머리 없음 — Merged 표시 없음"},
		{"머리 가지 갱신 거절(경합)", "머리 가지 갱신 거절(경합)로 Merged 표시 없음"},
	} {
		k3 := prSetup(t, "pr app")
		k3.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
		prTick()
		k3.ev("2026-11-24T01:20:00Z", "mg", "w1", "착지", prSHA2, "병합기 착지 — 제출 ID x · PR 머리 안 들어감("+c.why+" — Merged 표시 없음)")
		prTick()
		if k3.n("pr close 7") != 1 || !strings.Contains(k3.bodies(), "착지 22222222 — "+c.want) || strings.Contains(k3.bodies(), "가지 갱신 실패") {
			t.Fatalf("%s 닫기:\n%s\n%s", c.why, k3.logged(), k3.bodies())
		}
	}
}

// ⑥ gh가 계속 실패 → 대기열 파일 그대로, pr_fail바퀴째 운영 사건 한 번(더 돌아도 한 번)
func TestPRGhFails(t *testing.T) {
	k := prSetup(t, "pr app", "pr_fail 3")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	q0, _ := readFile(Q)
	t.Setenv("HC_T_GHFAIL", "1")
	for i := 0; i < 5; i++ {
		prTick()
	}
	if q1, _ := readFile(Q); q1 != q0 {
		t.Fatal("대기열이 바뀌었다")
	}
	if k.n("--name hc-fill-pr") != 1 {
		t.Fatalf("운영 사건:\n%s", k.logged())
	}
	t.Setenv("HC_T_GHFAIL", "")
	prTick()
	if k.n("pr create") != 1 || k.n("api -X PATCH") != 6 || isFile(DATA+"/pr-fail") {
		t.Fatalf("고친 뒤 이어 감:\n%s", k.logged())
	}
}

// ⑦ 다시 맞춤 레인 → 원래 PR에 이어 씀(새 PR 없음, 머리 가지는 원래 이름)
func TestPRReland(t *testing.T) {
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	k.row("w1-reland")
	if err := os.MkdirAll(k.dir+"/app.wt/w1-reland", 0o755); err != nil {
		t.Fatal(err)
	}
	k.ev("2026-11-24T03:00:00Z", "here", "w1-reland", "완료", prSHA2, "다시 맞춤")
	prTick()
	if k.n("pr create") != 1 || k.n("git/refs/heads/pr/w1 -f sha="+prSHA2) != 1 || !strings.Contains(k.bodies(), "다시 맞춤 레인 w1-reland") {
		t.Fatalf("다시 맞춤:\n%s\n%s", k.logged(), k.bodies())
	}
	if pr, _ := prOf("w1-reland"); pr != "PR #7" {
		t.Fatalf("prOf(다시 맞춤) = %q", pr)
	}
}

// ⑧ 대기열에서 뺌 → 까닭 댓글 뒤 닫음
func TestPRRmCloses(t *testing.T) {
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	prCloseReq("w1", "hc-fill rm 지휘 결정")
	prTick()
	prTick()
	if k.n("pr close 7") != 1 || !strings.Contains(k.bodies(), "대기열에서 뺌 — 착지 없이 끝났다: hc-fill rm 지휘 결정") || isFile(DATA+"/pr-close.tsv") {
		t.Fatalf("닫기:\n%s\n%s", k.logged(), k.bodies())
	}
}

// ⑨ 개인 값 무늬가 든 보고서·요약 → 가려서 올라감. 무늬를 못 읽으면 올리지 않는다(운영 사건 한 번)
func TestPRMask(t *testing.T) {
	k := prSetup(t, "pr app")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "secret-project 고침")
	prTick()
	k.verdict("a1", "pass", "0.0.0", "경로 /x/secret-project/y 와 기계 box-12\n")
	prTick()
	if all := k.logged() + k.bodies(); strings.Contains(all, "secret-project") || strings.Contains(all, "box-12") || !strings.Contains(k.bodies(), "‹가림›") {
		t.Fatalf("가림:\n%s", all)
	}
	k = prSetup(t, "pr app")
	t.Setenv("HERDR_TOOLS_CONFIG", t.TempDir())
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	prTick()
	if k.n("gh ") != 0 || k.n("--name hc-fill-pr") != 1 {
		t.Fatalf("무늬 없음:\n%s", k.logged())
	}
}

// ⑩ 바퀴당 gh 호출 상한 — 넘으면 다음 바퀴에
func TestPRBudget(t *testing.T) {
	k := prSetup(t, "pr app", "pr_max 3") // 커밋 올리기(레인 기계)도 한 번으로 센다 — 올리기·머리 가지·열기
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	k.verdict("a1", "pass", "0.0.0", "깨끗\n")
	prTick()
	if k.n("gh ") != 2 || k.n("pr create") != 1 {
		t.Fatalf("첫 바퀴:\n%s", k.logged())
	}
	prTick()
	if k.n("gh ") != 4 || k.n("statuses/") != 1 || k.n("pr comment") != 1 {
		t.Fatalf("둘째 바퀴:\n%s", k.logged())
	}
}

// 상한 1: 바퀴마다 한 단계씩(올리기 → 머리 가지 → 열기), 해낸 단계를 되풀이하지 않는다
func TestPRBudgetOne(t *testing.T) {
	k := prSetup(t, "pr app", "pr_max 1")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	prTick()
	prTick()
	prTick()
	if k.n("api -X PATCH") != 1 || k.n("pr create") != 1 || k.n("git push") != 1 {
		t.Fatalf("상한 1:\n%s", k.logged())
	}
}

// 원격이 없는 저장소 → 조용히 건너뜀 + 운영 사건 한 번
func TestPRNoRemote(t *testing.T) {
	k := prSetup(t, "pr app")
	t.Setenv("HC_T_NOURL", "1")
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	prTick()
	if k.n("gh ") != 0 || k.n("--name hc-fill-pr") != 1 {
		t.Fatalf("원격 없음:\n%s", k.logged())
	}
}
