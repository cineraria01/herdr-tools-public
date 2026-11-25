//go:build !windows

package main

// 사람 리뷰 반영(ht-pr-human-review): 가짜 gh(GraphQL 글 목록은 notes.json)·가짜 git(ls-remote는 ls 파일, fetch·push는 적기만)·가짜 hc(send만 됨)·
// 가짜 hc-lane으로 거울 바퀴(prTick)의 사람 리뷰 단계를 본다. 실제 GitHub·PR·레인 터미널은 건드리지 않는다

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const phTGh = `#!/bin/sh
echo "gh $*" >>"$HC_T_LOG"
[ -z "$HC_T_GHFAIL" ] || { echo "HTTP 502" >&2; exit 1; }
for a; do [ "$a" != --body-file ] || { printf '%s\n=====\n' "$(cat)" >>"$HC_T_DIR/body.log"; break; }; done
case "$1 $2" in
"api graphql") cat "$HC_T_DIR/notes.json" ;;
esac
exit 0
`

const phTGit = `#!/bin/sh
[ "$1" != -C ] || shift 2
echo "git $*" >>"$HC_T_LOG"
case "$1" in
ls-remote)
  shift; [ "$1" != --exit-code ] || shift; shift
  hit=0
  [ -f "$HC_T_DIR/ls" ] && while IFS='	' read -r sha ref; do
    for p; do
      case "$p" in
      *'*') case "$ref" in "${p%\*}"*) printf '%s\t%s\n' "$sha" "$ref"; hit=1; break ;; esac ;;
      *) [ "$ref" != "$p" ] || { printf '%s\t%s\n' "$sha" "$ref"; hit=1; break; } ;;
      esac
    done
  done <"$HC_T_DIR/ls"
  [ $hit = 1 ] || exit 2 ;;
fetch|push) exit 0 ;;
remote) echo "git@github.com:example-org/proj.git" ;;
*) exit 9 ;;
esac
`

const phTHc = `#!/bin/sh
[ "$1" = send ] || exit 1
echo "hc send $2" >>"$HC_T_LOG"
printf '%s\n' "$3" >>"$HC_T_DIR/sent.log"
`

const phQ1 = "1764000000-w1"

// phSetup: prSetup(레인 w1 살아 있음) + 열린 PR #7(w1) + 사람 리뷰 켬(extra로 덮을 수 있음) + 켠 때는 옛날
func phSetup(t *testing.T, extra ...string) *prT {
	t.Helper()
	k := prSetup(t, append([]string{"pr app", "pr_human me"}, extra...)...)
	for n, s := range map[string]string{"gh": phTGh, "git": phTGit, "hc": phTHc} {
		wr(t, k.dir+"/"+n, s)
		if err := os.Chmod(k.dir+"/"+n, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	wr(t, DATA+"/pr.tsv", strings.Join([]string{"w1", "app", "github.com/example-org/proj", "7", "https://github.com/example-org/proj/pull/7", "open", "2026-01-01T00:00:00Z", "self", prSHA1}, "\t")+"\n")
	wr(t, DATA+"/pr-human.tsv", "on\tapp\t2000-01-01T00:00:00Z\n")
	k.notes()
	return k
}

type phN struct{ id, kind, author, body, at, typ string }

// notes: PR #7의 글(GraphQL 응답 꼴) — 대화 댓글은 c<id>, 리뷰는 r<id>, 줄 댓글은 l<id>(리뷰 하나에 담음)
func (k *prT) notes(ns ...phN) {
	type actor struct {
		Type  string `json:"__typename"`
		Login string `json:"login"`
	}
	type node struct {
		ID       int    `json:"databaseId"`
		State    string `json:"state,omitempty"`
		Body     string `json:"body"`
		Path     string `json:"path,omitempty"`
		Line     int    `json:"line,omitempty"`
		At       string `json:"createdAt"`
		Sub      string `json:"submittedAt"`
		Author   actor  `json:"author"`
		Comments struct {
			Nodes []node `json:"nodes"`
		} `json:"comments"`
	}
	var cs, rs []node
	for i, n := range ns {
		typ := n.typ
		if typ == "" {
			typ = "User"
		}
		x := node{ID: 100 + i, Body: n.body, At: n.at, Sub: n.at, Author: actor{typ, n.author}}
		switch n.kind {
		case "comment":
			cs = append(cs, x)
		case "line":
			x.Path, x.Line = "a.go", 9
			rs = append(rs, node{ID: 900 + i, State: "COMMENTED", Sub: n.at, Author: x.Author})
			rs[len(rs)-1].Comments.Nodes = []node{x}
		default:
			x.State = map[string]string{"changes": "CHANGES_REQUESTED", "approve": "APPROVED", "review": "COMMENTED"}[n.kind]
			rs = append(rs, x)
		}
	}
	pr := map[string]any{"number": 7, "comments": map[string]any{"nodes": cs}, "reviews": map[string]any{"nodes": rs}}
	b, err := json.Marshal(map[string]any{"data": map[string]any{"viewer": map[string]string{"login": "me-1"},
		"repository": map[string]any{"pullRequests": map[string]any{"nodes": []any{pr}}}}})
	if err != nil {
		k.t.Fatal(err)
	}
	wr(k.t, k.dir+"/notes.json", string(b))
}

func (k *prT) sent() string { b, _ := os.ReadFile(k.dir + "/sent.log"); return string(b) }

func phRows(t *testing.T, what string) []Row {
	var o []Row
	for _, r := range readRows(DATA + "/pr-human.tsv") {
		if r.F(phWhat) == what {
			o = append(o, r)
		}
	}
	return o
}

// queued: w1(또는 lane)의 제출이 병합 줄에 섰다(MQS와 원격 ls)
func (k *prT) queued(lane, n, sha string) {
	MQS = append(MQS, Row{"app", "refs/merge-queue/" + n, sha})
	f, err := os.OpenFile(k.dir+"/ls", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		k.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(sha + "\trefs/merge-queue/" + n + "\n" + prSHA3 + "\trefs/merge-queue-meta/" + n + "\n"); err != nil {
		k.t.Fatal(err)
	}
}

// ① 꺼짐: pr_human 「-」·안 줌이면 글을 읽지 않고 장부·출력도 없다(거울만 켬 — 지금과 같음)
func TestPRHumanOff(t *testing.T) {
	for _, cfg := range []string{"pr_human -", "pr_human_block -"} {
		k := prSetup(t, "pr app", cfg)
		for n, s := range map[string]string{"gh": phTGh, "git": phTGit, "hc": phTHc} {
			wr(t, k.dir+"/"+n, s)
			if err := os.Chmod(k.dir+"/"+n, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		wr(t, DATA+"/pr.tsv", strings.Join([]string{"w1", "app", "github.com/example-org/proj", "7", "u", "open", "2026-01-01T00:00:00Z", "self", prSHA1}, "\t")+"\n")
		k.notes(phN{id: "x", kind: "changes", author: "me-1", body: "고쳐", at: "2030-01-01T00:00:00Z"})
		out := captured(t, prTick)
		if k.logged() != "" || out != "" || isFile(DATA+"/pr-human.tsv") || isFile(DATA+"/pr-human-now.tsv") || prHumanNote("w1") != "" {
			t.Fatalf("%s: 꺼졌는데 움직였다\nlog=%s\nout=%s", cfg, k.logged(), out)
		}
	}
}

// ② 도구 표식 글·허용 목록 밖·봇은 넘기지 않는다(밖·봇은 한 번만 알림), 켜기 전 글도
func TestPRHumanSkip(t *testing.T) {
	k := phSetup(t)
	wr(t, DATA+"/pr-human.tsv", "on\tapp\t2029-01-01T00:00:00Z\n")
	k.notes(phN{kind: "comment", author: "me-1", body: "감사 통과\n\n<!-- hc-fill-pr audit:a1 -->", at: "2030-01-01T00:00:00Z"},
		phN{kind: "changes", author: "stranger", body: "다 지워", at: "2030-01-01T00:00:00Z"},
		phN{kind: "comment", author: "ci", typ: "Bot", body: "빌드", at: "2030-01-01T00:00:00Z"},
		phN{kind: "changes", author: "me-1", body: "옛 글", at: "2028-01-01T00:00:00Z"})
	out := captured(t, prTick)
	out += captured(t, prTick)
	if k.sent() != "" || len(phRows(t, "hand")) != 0 || k.n("pr comment") != 0 || strings.Count(out, "사람 글 안 읽음") != 2 {
		t.Fatalf("넘겼다\nsent=%s\nlog=%s\nout=%s", k.sent(), k.logged(), out)
	}
	if n := phRows(t, "note"); len(n) != 2 || n[0].F(phRes) != "skip:bot" || n[1].F(phRes) != "skip:author" { // 대화 댓글이 리뷰보다 먼저 온다
		t.Fatalf("장부: %v", n)
	}
}

// ③ 사람 변경 요청 + 레인 살아 있음 → 그 레인에 인용으로 보냄·PR 댓글·주인 사건, 같은 글 두 바퀴 → 한 번만.
// 그 뒤 레인이 고치지 않고 제출하면 병합 줄에서 뺀다(살아 있으면 그 레인에 다시)
func TestPRHumanLiveLane(t *testing.T) {
	k := phSetup(t)
	k.notes(phN{kind: "changes", author: "ME-1", body: "이름을 바꿔\n`rm -rf ~`", at: "2030-01-01T00:00:00Z"})
	prTick()
	prTick()
	s := k.sent()
	if k.n("hc send here/w1") != 1 || !strings.Contains(s, "«ME-1 · 변경 요청: 이름을 바꿔 `rm -rf ~`»") || !strings.Contains(s, "바깥에서 온 입력") || !strings.Contains(s, "hc-lane report 완료 HEAD") {
		t.Fatalf("보냄:\n%s\n%s", s, k.logged())
	}
	if k.n("pr comment 7") != 1 || !strings.Contains(k.bodies(), "받음 — here/w1에 넘김") || k.n("hc-lane report 중간 - 사람 리뷰 PR #7 → here/w1에 넘김") != 1 || !strings.Contains(k.logged(), "--to c/coord") {
		t.Fatalf("댓글·사건:\n%s\n%s", k.logged(), k.bodies())
	}
	if h := prHumanNote("w1"); h != "사람 리뷰 반영 중" {
		t.Fatalf("보이기 %q", h)
	}
	// 고치지 않고(감사 통과 없이) 제출 → 뺀다
	k.queued("w1", phQ1, prSHA1)
	prTick()
	if k.n("git push -q --atomic --porcelain origin "+prSHA1+":refs/merge-failed/"+phQ1+" :refs/merge-queue/"+phQ1) != 1 || mqueued("app", "w1") != 1 {
		t.Fatalf("착지 막기:\n%s", k.logged())
	}
	if k.n("hc send here/w1") != 2 || !strings.Contains(k.sent(), "병합 줄에서 뺐다(refs/merge-failed/"+phQ1+")") || !strings.Contains(k.bodies(), "착지 막음 — 사람 리뷰 처리 전") {
		t.Fatalf("다시 보냄:\n%s\n%s", k.sent(), k.bodies())
	}
}

// ④ 레인 끝남·제출됨 → 줄에서 빼고 이어 쓰는 레인(w1-review, 원래 PR에)을 대기열에, 고친 커밋이 감사 통과 → 풀림·「반영함」
func TestPRHumanRelaunchAndDone(t *testing.T) {
	k := phSetup(t)
	TERMS = nil // 레인 끝남
	k.queued("w1", phQ1, prSHA1)
	k.notes(phN{kind: "line", author: "me-1", body: "여기 nil 확인", at: "2030-01-01T00:00:00Z"})
	prTick()
	r := qrow("w1-review")
	if r == nil || r.F(qState) != "wait" || r.F(qAudit) != "-" || r.F(qOwner) != "c/coord" || r.F(qPrio) != "high" || prOfGet("w1-review") != "w1" {
		t.Fatalf("이어 쓰는 레인: %v\n%s", r, k.logged())
	}
	b, _ := os.ReadFile(r.F(qBrief))
	if !strings.Contains(string(b), "git fetch origin refs/merge-failed/"+phQ1) || !strings.Contains(string(b), "«me-1 · 줄 댓글 a.go:9: 여기 nil 확인»") {
		t.Fatalf("지시문:\n%s", b)
	}
	if k.n(":refs/merge-failed/"+phQ1) != 1 || !strings.Contains(k.bodies(), "받음 — w1-review에 넘김") {
		t.Fatalf("막기·댓글:\n%s\n%s", k.logged(), k.bodies())
	}
	prTick() // 그대로 — 두 번 띄우지 않는다
	if qrow("w1-review2") != nil {
		t.Fatal("이어 쓰는 레인을 또 넣었다")
	}
	// 이어 쓰는 레인이 떠 고치고 감사 통과 → 풀림
	k.ev("2099-01-01T00:00:00Z", "here", "w1-review", "완료", prSHA2, "고침 (감사 hc-fill 자동)")
	at := []string{"a9", "w1-review", "1", "w1-review@1", prSHA2, "here", "here/w1-review", "taken", "0", "pass", "0", "0", "-", "-", "0", "hml=0.0.0", "-"}
	wr(t, AT, strings.Join(at, "\t")+"\n")
	k.queued("w1-review", "1764000100-w1-review", prSHA2)
	prTick()
	if d := phRows(t, "done"); len(d) != 1 || d[0].F(phSHA) != prSHA2 || mqueued("app", "w1-review") != 0 || prHumanNote("w1") != "" {
		t.Fatalf("풀림: %v\n%s", d, k.logged())
	}
	prTick()
	if !strings.Contains(k.bodies(), "반영함 22222222") || strings.Count(k.bodies(), "반영함") != 1 {
		t.Fatalf("반영함:\n%s", k.bodies())
	}
}

// ⑤ 대화 댓글만 → 넘기기만(막지 않음 — 줄에 선 제출 그대로), 살아 있는 레인이 없으면 지휘에 알림만
func TestPRHumanCommentOnly(t *testing.T) {
	k := phSetup(t)
	k.notes(phN{kind: "comment", author: "me-1", body: "참고로 문서도", at: "2030-01-01T00:00:00Z"})
	prTick()
	if k.n("hc send here/w1") != 1 || phRows(t, "hand")[0].F(phBlock) != "0" || prHumanNote("w1") != "" {
		t.Fatalf("넘김:\n%s", k.logged())
	}
	k2 := phSetup(t)
	TERMS = nil
	k2.queued("w1", phQ1, prSHA1)
	k2.notes(phN{kind: "comment", author: "me-1", body: "참고", at: "2030-01-01T00:00:00Z"})
	prTick()
	if k2.n("git push") != 0 || mqueued("app", "w1") != 0 || qrow("w1-review") != nil || k2.n("넘길 살아 있는 레인 없음") != 1 {
		t.Fatalf("막지 않음:\n%s", k2.logged())
	}
}

// ⑥ 착지 뒤 글 → 고치지 않고 주인 지휘에 사건(막힘) 한 번
func TestPRHumanAfterLand(t *testing.T) {
	k := phSetup(t)
	// PR #7은 착지(Merged), 같은 저장소에 열린 PR #8이 있어 글을 읽는다
	wr(t, DATA+"/pr.tsv", strings.Join([]string{"w2", "app", "github.com/example-org/proj", "8", "u8", "open", "2026-01-01T00:00:00Z", "self", prSHA2}, "\t")+"\n"+
		strings.Join([]string{"w1", "app", "github.com/example-org/proj", "7", "https://github.com/example-org/proj/pull/7", "merged", "2026-01-01T00:00:00Z", "self", prSHA1}, "\t")+"\n")
	k.notes(phN{kind: "changes", author: "me-1", body: "늦은 지적", at: "2030-01-01T00:00:00Z"})
	prTick()
	prTick()
	if k.sent() != "" || qrow("w1-review") != nil || k.n("hc-lane report 막힘 - 착지 뒤 사람 리뷰 PR #7") != 1 {
		t.Fatalf("착지 뒤:\n%s", k.logged())
	}
}

// ⑦ gh 실패 → 이번 바퀴는 없음, 다음 바퀴에 그대로. 바퀴당 상한 → 남은 댓글은 다음 바퀴에
func TestPRHumanGhFailBudget(t *testing.T) {
	k := phSetup(t)
	k.notes(phN{kind: "changes", author: "me-1", body: "고쳐", at: "2030-01-01T00:00:00Z"})
	t.Setenv("HC_T_GHFAIL", "1")
	prTick()
	if k.sent() != "" || len(phRows(t, "note")) != 0 {
		t.Fatalf("실패 바퀴에 넘겼다:\n%s", k.logged())
	}
	t.Setenv("HC_T_GHFAIL", "")
	confLines = append(confLines, "pr_max 1")
	prTick()
	if k.n("hc send here/w1") != 1 || k.n("pr comment") != 0 {
		t.Fatalf("상한 바퀴:\n%s", k.logged())
	}
	confLines = confLines[:len(confLines)-1]
	prTick()
	if k.n("hc send here/w1") != 1 || k.n("pr comment 7") != 1 {
		t.Fatalf("다음 바퀴:\n%s", k.logged())
	}
}

// ⑧ 재승인 기다림: 고친 커밋이 감사를 통과해도 사람 승인 전에는 제출을 빼 두고, 승인이 오면 줄 끝에 다시 세운다. human-ok도 같은 길
func TestPRHumanApproveWait(t *testing.T) {
	k := phSetup(t, "pr_human_approve 1")
	k.notes(phN{kind: "changes", author: "me-1", body: "고쳐", at: "2030-01-01T00:00:00Z"})
	prTick()
	k.ev("2099-01-01T00:00:00Z", "here", "w1", "완료", prSHA2, "고침")
	wr(t, AT, strings.Join([]string{"a9", "w1", "2", "w1@1", prSHA2, "here", "here/w1", "taken", "0", "pass", "0", "0", "-", "-", "0", "hml=0.0.0", "-"}, "\t")+"\n")
	k.queued("w1", phQ1, prSHA2)
	prTick()
	if h := phRows(t, "hold"); len(h) != 1 || h[0].F(phQKind) != "approve" || prHumanNote("w1") != "사람 리뷰 기다림(재승인)" || k.n("hc send here/w1") != 1 {
		t.Fatalf("재승인 기다림: %v\n%s", h, k.logged())
	}
	wr(t, k.dir+"/ls", prSHA2+"\trefs/merge-failed/"+phQ1+"\n"+prSHA3+"\trefs/merge-failed-meta/"+phQ1+"\n")
	k.notes(phN{kind: "changes", author: "me-1", body: "고쳐", at: "2030-01-01T00:00:00Z"}, phN{kind: "approve", author: "me-1", at: "2099-06-01T00:00:00Z"})
	prTick() // 승인을 읽음(막기·풀기는 바퀴 머리라 다음 바퀴에)
	prTick() // 풀림
	prTick() // 다시 세움·댓글
	if len(phRows(t, "done")) != 1 || len(phRows(t, "back")) != 1 || k.n(prSHA2+":refs/merge-queue/") != 1 || k.n(" :refs/merge-failed/"+phQ1+" ") != 1 || !strings.Contains(k.bodies(), "막음 풂 — 제출을 병합 줄에 다시 세움") {
		t.Fatalf("다시 세움:\n%s", k.logged())
	}
}

// ⑨ 막기는 gh와 떨어져 돈다: 무늬를 못 읽어 거울이 gh 앞에서 멈춘 바퀴에도, 바퀴 예산이 다 찬 바퀴에도 고치지 않은 제출을 줄에서 뺀다(감사 1차 중간 1)
func TestPRHumanHoldWithoutGh(t *testing.T) {
	for _, mode := range []string{"nomask", "budget"} {
		k := phSetup(t)
		k.notes(phN{kind: "changes", author: "me-1", body: "고쳐", at: "2030-01-01T00:00:00Z"})
		prTick() // 살아 있는 레인에 넘김
		k.queued("w1", phQ1, prSHA1)
		if mode == "nomask" {
			if err := os.Remove(os.Getenv("HERDR_TOOLS_CONFIG") + "/private-patterns.txt"); err != nil {
				t.Fatal(err)
			}
		} else {
			confLines = append(confLines, "pr_max 1")
			k.ev("2026-11-24T01:00:00Z", "here", "w2", "완료", prSHA2, "다른 레인") // opens가 예산을 먼저 쓴다
		}
		before := k.n("gh api graphql")
		prTick()
		if k.n(" :refs/merge-queue/"+phQ1+" ") != 1 || mqueued("app", "w1") != 1 {
			t.Fatalf("%s: 막지 않았다\n%s", mode, k.logged())
		}
		if mode == "nomask" && k.n("gh api graphql") != before {
			t.Fatalf("무늬 없이 gh를 불렀다:\n%s", k.logged())
		}
	}
}

// ⑩ human-ok: 레인이 「반영 안 함」 — 지휘가 풀면 막으려고 뺀 마지막 제출을 다시 세우고 「반영함 — 지휘가 풂」, 처리 전 글이 없으면 거절(감사 1차 중간 2)
func TestPRHumanOK(t *testing.T) {
	k := phSetup(t)
	if humanOK("w1", "x") != 1 {
		t.Fatal("처리 전 글이 없는데 풀었다")
	}
	k.notes(phN{kind: "changes", author: "me-1", body: "고쳐", at: "2030-01-01T00:00:00Z"})
	prTick()
	k.queued("w1", phQ1, prSHA1)
	prTick() // 뺌(fix)
	wr(t, k.dir+"/ls", prSHA1+"\trefs/merge-failed/"+phQ1+"\n"+prSHA3+"\trefs/merge-failed-meta/"+phQ1+"\n")
	if humanOK("w1", "반영할 것 없음 — 지휘 판단") != 0 || humanOK("w1", "또") != 1 {
		t.Fatal("human-ok 종료 코드")
	}
	prTick()
	prTick()
	if b := phRows(t, "back"); len(b) != 1 || b[0].F(phQ) != phQ1 || k.n(prSHA1+":refs/merge-queue/") != 1 || prHumanNote("w1") != "" {
		t.Fatalf("다시 세움: %v\n%s", b, k.logged())
	}
	if !strings.Contains(k.bodies(), "반영함 — 지휘가 풂: 반영할 것 없음 — 지휘 판단") || !strings.Contains(k.bodies(), "막음 풂 — 제출을 병합 줄에 다시 세움") {
		t.Fatalf("댓글:\n%s", k.bodies())
	}
}

// ⑪ 넘기기 전에 이미 감사를 통과한 같은 커밋으로 다시 부른 완료는 「처리됨」이 아니다(감사 1차 낮음 1). 사람 글의 제어 문자는 레인에 넘기지 않는다(낮음 2)
func TestPRHumanSameShaAndControl(t *testing.T) {
	k := phSetup(t)
	k.ev("2026-01-02T00:00:00Z", "here", "w1", "완료", prSHA1, "처음")
	wr(t, AT, strings.Join([]string{"a1", "w1", "1", "w1@1", prSHA1, "here", "here/w1", "taken", "0", "pass", "0", "0", "-", "-", "0", "hml=0.0.0", "-"}, "\t")+"\n")
	k.notes(phN{kind: "changes", author: "me-1", body: "고쳐\x1b[31m빨강\x03끝", at: "2030-01-01T00:00:00Z"})
	prTick()
	if s := k.sent(); strings.ContainsAny(s, "\x1b\x03") || !strings.Contains(s, "고쳐 [31m빨강 끝") {
		t.Fatalf("제어 문자: %q", s)
	}
	k.ev("2099-01-01T00:00:00Z", "here", "w1", "완료", prSHA1, "같은 HEAD로 다시")
	prTick()
	if len(phRows(t, "done")) != 0 || prHumanNote("w1") != "사람 리뷰 반영 중" {
		t.Fatalf("같은 커밋을 처리로 셈:\n%s", k.logged())
	}
}
