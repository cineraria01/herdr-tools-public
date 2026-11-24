//go:build !windows

package main

// 충돌을 병합 쪽에서 푼다(merger-conflict-agent): 가짜 에이전트(정해진 풀이·판정을 바로 쓰는 스크립트)와 가짜 착지(batchFx — 승인
// 착지면 HEAD·patch-id 대조)로 바퀴를 돌린다. 실제 모델·hc는 부르지 않는다.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// fakeAgent: 띄울 명령. FAKE_RESOLVE = ok(양쪽을 살려 풀고 풀림) · cant(못 풂) · none(아무것도 안 씀),
// FAKE_AUDIT = pass · bad(중간 1) · wrongid(다른 차수 ID) · none. 띄운 이름은 $LAUNCHLOG에
const fakeAgent = `#!/bin/sh
printf '%s\n' "$HC_MERGE_NAME" >>"$LAUNCHLOG"
case "$HC_MERGE_NAME" in
merge-*)
  res=$(sed -n 's/^RESULT=//p' "$HC_MERGE_BRIEF")
  case "$FAKE_RESOLVE" in
  ok) cd "$HC_MERGE_DIR" && printf 'main\nlane\n' >c.txt && git add c.txt && GIT_EDITOR=true git -c core.hooksPath=/dev/null rebase --continue >/dev/null 2>&1 && printf '풀림\n' >"$res" ;;
  cant) printf '못 풂 — 자신 없음\n' >"$res" ;;
  esac ;;
audit-merge-*)
  v=$(sed -n 's/^VERDICT=//p' "$HC_MERGE_BRIEF"); a=$(sed -n 's/^AID=//p' "$HC_MERGE_BRIEF")
  case "$FAKE_AUDIT" in
  pass) printf '감사 완료 %s 높음 0·중간 0·낮음 1\n' "$a" >"$v" ;;
  bad) printf '감사 완료 %s 높음 0·중간 1·낮음 0\n' "$a" >"$v" ;;
  wrongid) printf '감사 완료 mr-00000000 높음 0·중간 0·낮음 0\n' >"$v" ;;
  esac ;;
esac
echo "box:t-$HC_MERGE_NAME"
`

type rfx struct {
	*fx
	landlog, launchlog string
}

// resolveFx: 병합기·가짜 착지·가짜 에이전트·지시문 틀. main의 c.txt를 레인 cx와 main이 서로 다르게 고쳐 cx 제출이 충돌한다
func resolveFx(t *testing.T, resolve, audit string) (*rfx, string) {
	r, q := resolveFx0(t, resolve, audit)
	r.mainC("main\n")
	return r, q
}

// resolveFx0: resolveFx에서 main을 옮기기 전(cx 제출은 아직 깨끗하다)
func resolveFx0(t *testing.T, resolve, audit string) (*rfx, string) {
	f, landlog := batchFx(t)
	r := &rfx{fx: f, landlog: landlog, launchlog: f.dir + "/launch.log"}
	conf := f.dir + "/conf"
	os.MkdirAll(conf, 0o777)
	os.WriteFile(conf+"/merger-resolve.md", []byte("# 풀이 {lane}\nRESULT={result}\nDIR={worktree}\n"), 0o666)
	os.WriteFile(conf+"/merger-resolve-audit.md", []byte("# 풀이 감사 {lane}\nVERDICT={verdict}\nAID={audit_id}\nDIFF={auto_tree} {resolved}\n"), 0o666)
	os.WriteFile(f.dir+"/agent.sh", []byte(fakeAgent), 0o777)
	for k, v := range map[string]string{"HERDR_TOOLS_CONFIG": conf, "LAUNCHLOG": r.launchlog, "FAKE_RESOLVE": resolve, "FAKE_AUDIT": audit,
		"HC_LANE_MERGER_RESOLVE_LAUNCH": f.dir + "/agent.sh", "HC_LANE_MERGER_CONFLICT": "", "HC_LANE_MERGER_RESOLVE_MAX": ""} {
		t.Setenv(k, v)
	}
	os.WriteFile(f.seed+"/c.txt", []byte("base\n"), 0o666)
	f.git(f.seed, "add", "-A")
	f.git(f.seed, "commit", "-qm", "c base")
	f.git(f.seed, "push", "-q", "origin", "main")
	q, _ := f.sub2(1000000001, "cx", "", "", "c.txt", "lane\n", false)
	return r, q
}

// mainC: main의 c.txt를 바꿔 push
func (r *rfx) mainC(body string) {
	r.git(r.seed, "checkout", "-q", "main")
	r.git(r.seed, "pull", "-q", "--rebase", "origin", "main")
	os.WriteFile(r.seed+"/c.txt", []byte(body), 0o666)
	r.git(r.seed, "commit", "-qam", "main c")
	r.git(r.seed, "push", "-q", "origin", "main")
}

func (r *rfx) rounds(n int) {
	for i := 0; i < n; i++ {
		r.merger().round()
	}
}

func (r *rfx) launched() string {
	b, _ := os.ReadFile(r.launchlog)
	return string(b)
}

// ran: 그 이름으로 띄웠나(줄 전체가 같을 때 — merge-cx 와 audit-merge-cx 를 가른다)
func (r *rfx) ran(name string) bool {
	for _, l := range strings.Split(r.launched(), "\n") {
		if l == name {
			return true
		}
	}
	return false
}

func (r *rfx) mainFile(p string) string {
	o, _ := exec.Command("git", "--git-dir="+r.origin, "show", "main:"+p).Output()
	return string(o)
}

func (r *rfx) journal() string {
	b, _ := os.ReadFile(gEvdir + "/merger-journal.tsv")
	return string(b)
}

func (r *rfx) evs() string {
	b, _ := os.ReadFile(r.events)
	return string(b)
}

// ① 충돌 → 반송 없이 푸는 에이전트 → 풀이 감사 → 원래 차례에 다시 넣음 → 승인 착지(판정 ID·patch-id 대조)
func TestResolveAgentLands(t *testing.T) {
	r, q := resolveFx(t, "ok", "pass")
	fq := strings.Replace(q, "refs/merge-queue/", "refs/merge-failed/", 1)
	r.rounds(1)
	if r.has(fq) || !r.has(q) {
		t.Fatalf("충돌이면 반송하지 않고 큐에 두어야(반송 %v, 큐 %v)\n%s", r.has(fq), r.has(q), r.evs())
	}
	if !r.ran("merge-cx") || r.ran("audit-merge-cx") {
		t.Fatalf("푸는 에이전트 merge-cx 만 떠야(감사는 풀림 뒤): %q", r.launched())
	}
	if !strings.Contains(r.evs(), "충돌 — 병합 쪽에서 푸는 중") {
		t.Fatalf("주인 지휘에 「푸는 중」 사건: %s", r.evs())
	}
	r.rounds(4)
	if r.has(fq) || r.has(q) {
		t.Fatalf("착지해 큐가 비고 반송이 없어야(반송 %v, 큐 %v)\n%s", r.has(fq), r.has(q), r.evs())
	}
	if got := r.mainFile("c.txt"); got != "main\nlane\n" {
		t.Fatalf("양쪽을 살린 풀이가 main에: %q", got)
	}
	if !r.ran("audit-merge-cx") {
		t.Fatalf("풀이 감사가 떠야: %q", r.launched())
	}
	ll := landLines(t, r.landlog)
	if len(ll) != 1 || !strings.HasPrefix(ll[0], "land|mr-") || strings.Split(ll[0], "|")[2] == "" {
		t.Fatalf("착지는 풀이 감사 판정(mr-…)·patch-id로 한 번: %v", ll)
	}
	j := r.journal()
	for _, w := range []string{"\t" + jResolving + "\t", "\t" + jResolved + "\t", "풀이 착지(병합 쪽 풀이 판정 mr-"} {
		if !strings.Contains(j, w) {
			t.Fatalf("일지에 %q:\n%s", w, j)
		}
	}
	if fileExists(r.clone+".merger-resolve/"+strings.TrimPrefix(q, "refs/merge-queue/")) || fileExists(r.clone+"/.git/hc-merger/resolve/"+strings.TrimPrefix(q, "refs/merge-queue/")) {
		t.Fatal("착지 뒤 풀이 작업 트리·상태를 치워야")
	}
}

// reject 길의 공통 단언: 반송됐고 까닭에 원래 충돌 꼴(hc-fill 다시 맞춤이 읽는 「종료 3, 충돌 …) — 」)과 풀이 못 한 까닭
func (r *rfx) rejected(t *testing.T, q, why string) {
	t.Helper()
	fq := strings.Replace(q, "refs/merge-queue/", "refs/merge-failed/", 1)
	if !r.has(fq) || r.has(q) {
		t.Fatalf("반송돼야(반송 %v, 큐 %v)\n%s", r.has(fq), r.has(q), r.evs())
	}
	if !strings.Contains(r.evs(), "종료 3, 충돌 c.txt) — ") || !strings.Contains(r.evs(), why) {
		t.Fatalf("반송 사건에 충돌 꼴과 %q:\n%s", why, r.evs())
	}
	if r.onMain("c.txt") && r.mainFile("c.txt") != "main\n" {
		t.Fatal("못 푼 풀이가 main에 들면 안 된다")
	}
}

// ② 에이전트 「못 풂」 → 반송(까닭 포함)
func TestResolveAgentCant(t *testing.T) {
	r, q := resolveFx(t, "cant", "pass")
	r.rounds(3)
	r.rejected(t, q, "병합 쪽 풀이 못 함: 에이전트 「못 풂 — 자신 없음」")
}

// ③ 풀이 감사 지적 → 반송
func TestResolveAuditFinding(t *testing.T) {
	r, q := resolveFx(t, "ok", "bad")
	r.rounds(4)
	r.rejected(t, q, "풀이 감사 지적 높음 0·중간 1")
	if len(landLines(t, r.landlog)) != 0 {
		t.Fatal("지적 받은 풀이는 착지를 부르지 않는다")
	}
}

// ④ 시간 넘김 → 반송(상한은 손잡이 파일 — 시작 시각을 옛 값으로 바꿔 기다리지 않는다)
func TestResolveTimeout(t *testing.T) {
	r, q := resolveFx(t, "none", "pass")
	r.rounds(1)
	m := r.merger()
	n := strings.TrimPrefix(q, "refs/merge-queue/")
	st, ok := loadR(m.mj + "/resolve/" + n)
	if !ok {
		t.Fatal("푸는 중 상태가 있어야")
	}
	st.t0 = 1000
	if err := st.save(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(m.mj+"/resolve_max", []byte("60\n"), 0o666)
	r.rounds(1)
	r.rejected(t, q, "풀이 시간 넘김(60초)")
}

// 에이전트를 못 띄움 → 반송
func TestResolveNoLauncher(t *testing.T) {
	r, q := resolveFx(t, "ok", "pass")
	t.Setenv("HC_LANE_MERGER_RESOLVE_LAUNCH", "")
	r.rounds(1)
	r.rejected(t, q, "푸는 에이전트를 못 띄움 — 띄울 명령이 없다")
}

// ⑤ 손잡이 「사건만」 → 에이전트를 띄우지 않고 지휘에 사건, 지휘가 풀고 이어 가는 명령 → 풀이 감사 → 착지
func TestResolveEventMode(t *testing.T) {
	r, q := resolveFx(t, "ok", "pass")
	m := r.merger()
	os.WriteFile(m.mj+"/conflict", []byte("event\n"), 0o666) // 실행 중에 바로 먹는 손잡이
	r.rounds(2)
	if r.ran("merge-cx") {
		t.Fatalf("사건만이면 푸는 에이전트를 띄우지 않는다: %q", r.launched())
	}
	ev := r.evs()
	n := strings.TrimPrefix(q, "refs/merge-queue/")
	wt := r.clone + ".merger-resolve/" + n
	if !strings.Contains(ev, "막힘|-|충돌 — 풀 수 있음: "+wt) || !strings.Contains(ev, "/result") {
		t.Fatalf("주인 지휘에 막힘 사건 「충돌 — 풀 수 있음: <작업 트리> · <이어 가는 명령>」:\n%s", ev)
	}
	// 지휘가 푼다
	c := exec.Command("sh", "-c", "printf 'main\\nlane\\n' >c.txt && git add c.txt && GIT_EDITOR=true git -c core.hooksPath=/dev/null rebase --continue >/dev/null 2>&1")
	c.Dir = wt
	if o, err := c.CombinedOutput(); err != nil {
		t.Fatalf("지휘 풀이: %v %s", err, o)
	}
	os.WriteFile(m.mj+"/resolve/"+n+"/result", []byte("풀림\n"), 0o666)
	r.rounds(4)
	if r.has(q) || r.mainFile("c.txt") != "main\nlane\n" {
		t.Fatalf("이어 가는 명령 뒤 풀이 감사를 거쳐 착지: 큐 %v, main %q", r.has(q), r.mainFile("c.txt"))
	}
	if r.ran("merge-cx") || !r.ran("audit-merge-cx") {
		t.Fatalf("지휘 풀이도 풀이 감사를 거친다: %q", r.launched())
	}
}

// ⑥ 손잡이 「바로 반송」 → 지금 그대로(에이전트·상태 없음)
func TestResolveRejectMode(t *testing.T) {
	r, q := resolveFx(t, "ok", "pass")
	t.Setenv("HC_LANE_MERGER_CONFLICT", "reject")
	r.rounds(1)
	fq := strings.Replace(q, "refs/merge-queue/", "refs/merge-failed/", 1)
	if !r.has(fq) || r.launched() != "" || strings.Contains(r.evs(), "병합 쪽") {
		t.Fatalf("바로 반송(지금 그대로): 반송 %v, 띄움 %q\n%s", r.has(fq), r.launched(), r.evs())
	}
	if !strings.Contains(r.evs(), "병합기: 착지 실패(종료 3, 충돌 c.txt) — 미리 훑기(차례 전) — 원본 ") {
		t.Fatalf("반송 글이 지금과 같아야:\n%s", r.evs())
	}
}

// ⑦ 푸는 동안 뒤 후보가 착지한다
func TestResolveOthersLand(t *testing.T) {
	r, q := resolveFx(t, "none", "pass")
	qo, _ := r.sub2(1000000002, "ok1", "", "", "ok1.txt", "ok1\n", false)
	r.rounds(2)
	if r.has(qo) || !r.onMain("ok1.txt") {
		t.Fatal("푸는 동안 뒤 후보 ok1 은 착지해야")
	}
	fq := strings.Replace(q, "refs/merge-queue/", "refs/merge-failed/", 1)
	if !r.has(q) || r.has(fq) {
		t.Fatal("푸는 제출은 큐에 그대로(반송 아님)")
	}
}

// ⑧ 감사 없이 풀이가 들어가는 길이 없다: 풀이 감사 판정이 없거나 다른 차수 ID면 큐의 제출이 바뀌지 않고 착지하지 않는다
func TestResolveNeedsAudit(t *testing.T) {
	for _, a := range []string{"none", "wrongid"} {
		t.Run(a, func(t *testing.T) {
			r, q := resolveFx(t, "ok", a)
			orig := r.git(r.origin, "rev-parse", q)
			r.rounds(6)
			if got := r.git(r.origin, "rev-parse", q); got != orig {
				t.Fatalf("판정 없이 큐의 제출이 바뀌면 안 된다: %s → %s", orig, got)
			}
			if len(landLines(t, r.landlog)) != 0 || r.mainFile("c.txt") != "main\n" {
				t.Fatal("판정 없이 착지하면 안 된다")
			}
		})
	}
}

// ⑨ 충돌 없는 제출은 지금과 같다(에이전트·풀이 상태 없음, 판정 없는 착지)
func TestResolveNoConflictSame(t *testing.T) {
	r, _ := resolveFx(t, "ok", "pass")
	t.Setenv("HC_LANE_MERGER_CONFLICT", "reject")
	r.rounds(1) // 충돌 제출 cx는 지금처럼 반송
	os.Remove(r.launchlog)
	t.Setenv("HC_LANE_MERGER_CONFLICT", "")
	qo, _ := r.sub2(1000000003, "ok2", "", "", "ok2.txt", "ok2\n", false)
	r.rounds(1)
	if r.has(qo) || !r.onMain("ok2.txt") || r.launched() != "" {
		t.Fatalf("충돌 없는 제출은 지금처럼 착지(띄움 %q)", r.launched())
	}
	if ll := landLines(t, r.landlog); len(ll) != 1 || ll[0] != "land|||ok2" {
		t.Fatalf("착지 인자가 지금과 같아야: %v", ll)
	}
	if des, _ := os.ReadDir(r.merger().mj + "/resolve"); len(des) != 0 {
		t.Fatal("풀이 상태가 생기면 안 된다")
	}
}

// 착지 중 rebase 충돌(종료 3 — 차례 전 미리 훑기 뒤에 기준이 움직임)도 반송하지 않고 병합 쪽에서 푼다
func TestResolveFromLanding(t *testing.T) {
	r, q := resolveFx0(t, "ok", "pass")
	f, landlog := r.fx, r.landlog
	// 가짜 착지가 rebase 직전에 한 번 main을 c.txt 충돌로 옮긴다(착지 중 기준이 움직임)
	race := f.dir + "/race"
	os.WriteFile(race, nil, 0o666)
	t.Setenv("RACE", race)
	t.Setenv("RACESEED", f.seed)
	old, _ := os.ReadFile(gSelf)
	hook := `if [ "$1" = land ] && [ -e "$RACE" ]; then rm -f "$RACE"; (cd "$RACESEED" && git checkout -q main && git pull -q --rebase origin main && printf 'main\n' >c.txt && git commit -qam race && git push -q origin main); fi
`
	os.WriteFile(gSelf, []byte(strings.Replace(string(old), "#!/bin/sh\n", "#!/bin/sh\n"+hook, 1)), 0o777)
	r.rounds(1)
	fq := strings.Replace(q, "refs/merge-queue/", "refs/merge-failed/", 1)
	if r.has(fq) || !r.ran("merge-cx") {
		t.Fatalf("착지 종료 3도 반송하지 않고 푸는 에이전트: 반송 %v 띄움 %q\n%s", r.has(fq), r.launched(), r.evs())
	}
	r.rounds(4)
	if r.has(q) || r.mainFile("c.txt") != "main\nlane\n" {
		t.Fatalf("풀이가 감사를 거쳐 착지: 큐 %v main %q\n%s", r.has(q), r.mainFile("c.txt"), fmt.Sprint(landLines(t, landlog)))
	}
}
