package main

// 반송 다시 띄우기(2026-11-26 ht-bounce-relaunch — 한 프로젝트의 레인 둘이 병합기 검증 실패로 반송됐는데 레인 터미널은
// 이미 끝나 있었고, 대기열 줄은 「감사 통과 — 대기열이 제출함」으로 남아 지휘가 「-r2」 레인을 손으로 넣어 cherry-pick했다).
// 자동 다시 맞춤(reland.go)의 같은 바퀴·장부·횟수·원래 레인에 보냄 길을 넓힌다: rebase 충돌(종료 3)은 지금처럼 <레인>-reland 줄,
// 그 밖의 반송(검증 실패 등 — 「병합기: 착지 실패(종료 …」, refs/merge-failed/<n> 로 옮김)은 그 레인의 대기열 줄을 같은 이름·같은 줄 ID로
// 「반송됨」 대기(14칸 bounced)로 되돌리고, 다음 띄우기가 run 지시문 끝에 반송 절(bounceSec — 반송 커밋으로 옮기기·병합기 로그의 실패 줄·
// 지휘 메모 coord-notes)을 붙인다. 줄이 없으면(지휘가 rm) 안 띄움, 반송 커밋이 이미 기본 가지에 있으면(git cherry — 다른 이름으로
// cherry-pick해 들어간 것도) 안 띄움, 상한(reland_max)이면 주인 지휘에 막힘.
// ponytail: 반송 커밋의 시작은 hc-lane --resume(주차 가지 전용, 반송 가지는 R-15로 거절) 대신 절의 fetch·reset --keep — 다시 맞춤 지시문과 같은 길

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"herdr-tools/internal/journal"
	"herdr-tools/internal/tri"
)

// relandFailRe: 병합기의 반송 사건 — 충돌(relandRe)이 아닌 착지 실패도. 까닭은 「 — 원본 」 앞까지(추측 검증 까닭은 괄호 안에 「 — 」가 있다),
// 그 꼴이 아니면 첫 「 — 」 앞까지
var (
	relandFailRe  = regexp.MustCompile(`^병합기: (착지 실패\(종료 .*?) — `)
	relandFailWhy = regexp.MustCompile(`^병합기: (착지 실패\(종료 .*?) — 원본 `)
)

// bounceWhy <사건 글>: 반송 까닭 한 마디(지휘가 거둔 것은 「지휘가 거둠: <까닭>」 — withdraw.go)
func bounceWhy(msg string) string {
	if m := wdEvRe.FindStringSubmatch(msg); m != nil {
		return m[1]
	}
	if m := relandFailWhy.FindStringSubmatch(msg); m != nil {
		return m[1]
	}
	if m := relandFailRe.FindStringSubmatch(msg); m != nil {
		return m[1]
	}
	return "착지 실패"
}

// bounceFile <레인>: 다음 띄우기에 붙일 반송 절
func bounceFile(lane string) string { return DATA + "/bounce/" + lane + ".md" }

// bounceSec <레인>: run 지시문 끝 반송 절(끝 줄바꿈 없음). 못 읽으면 빈 값
func bounceSec(lane string) string {
	b, ok := readFile(bounceFile(lane))
	if !ok {
		return ""
	}
	return "\n" + strings.TrimRight(b, "\n")
}

// bounceInMain <프로젝트> <큐 이름>: 반송 커밋이 모두 기본 가지에 있나(git cherry — patch-id가 같으면 cherry-pick·다시 맞춤으로 든 것도)
func bounceInMain(p, n string) tri.State {
	repo := repoOf(p, LOCAL)
	if repo == "" {
		return tri.Unknown
	}
	ref, ok := projectRef(repo)
	if !ok {
		return tri.Unknown
	}
	fr := "refs/merge-failed/" + n
	args := []string{"-C", repo, "fetch", "-q", "--no-tags", "--no-write-fetch-head", "origin", fr}
	if b, isRemote := strings.CutPrefix(ref, "origin/"); isRemote {
		args = append(args, "+refs/heads/"+b+":refs/remotes/"+ref)
	}
	o, rc := run("git", "-C", repo, "ls-remote", "origin", fr)
	if rc != 0 || !relandLsHas(o, fr) {
		return tri.Unknown
	}
	sha, _, _ := strings.Cut(o, "\t")
	if _, rc := run("git", args...); rc != 0 {
		return tri.Unknown
	}
	c, rc := run("git", "-C", repo, "cherry", ref, sha)
	if rc != 0 {
		return tri.Unknown
	}
	for _, l := range lines(c) {
		if strings.HasPrefix(l, "+") {
			return tri.No
		}
	}
	return tri.Yes
}

// bounceGate: 충돌 아닌 반송을 다시 띄울 수 있나 — 그 레인의 떠 있는(up) 대기열 줄이 있고 반송 커밋이 아직 기본 가지에 없다.
// 거짓이면 장부에 적었거나(줄 없음·이미 main·줄 바뀜) 다음 바퀴에(모름). 이미 이 반송으로 반송됨으로 돌린 줄(적기 전에 끊긴 바퀴)은
// 기록만 잇고, 다른 반송으로 반송됨인 줄(아직 안 뜸)은 이 반송으로 절을 다시 쓴다
func bounceGate(n, lane, p string, rec func(string, string), recTell func(string, string, string)) bool {
	r := qrow(lane)
	bounced := r != nil && r.F(qState) == "wait" && rowMode(r) == mBounced
	switch {
	case r == nil || r.F(qProj) != p:
		say("반송 다시 띄움 안 함 %s: 대기열에 그 레인 줄이 없다(지휘가 뺌 — refs/merge-failed/%s)", lane, n)
		rec("줄 없음", "-")
		return false
	case bounced && strings.Contains(bounceSec(lane), "refs/merge-failed/"+n+" "):
		recTell("다시 띄움", lane, bounceMsg(lane, n))
		return false
	case !bounced && r.F(qState) != "up":
		say("반송 다시 띄움 안 함 %s: 대기열 줄이 떠 있지 않다(상태 %s — 지휘가 바꿈)", lane, r.F(qState))
		rec("줄 바뀜", "-")
		return false
	}
	switch bounceInMain(p, n) {
	case tri.Yes:
		say("반송 다시 띄움 안 함 %s: 반송 커밋이 이미 기본 가지에 있다(refs/merge-failed/%s)", lane, n)
		rec("이미 main", "-")
		return false
	case tri.No:
		return true
	default:
		say("반송 다시 띄움 보류 %s: 반송 커밋이 기본 가지에 있는지 모름 — 다음 바퀴에", lane)
		return false
	}
}

// relandGone <레인> <프로젝트> <큐 이름> <대기열 줄>: rebase 충돌 반송의 레인을 지휘가 뺐나(hc-fill rm — 대기열에 그 프로젝트의 그 이름 줄이
// 없고 일지에 rm 됨 기록, ht-fill-smalls-1128 — 11-27 rm --force 직후의 반송에 -reland가 들어가 지휘가 닫은 레인이 되살아났다).
// 지웠으면 한 줄·「줄 없음」을 적고 참, 일지를 못 읽으면(모름) 적지 않고 참(다음 바퀴에). 대기열 밖에서 띄운 레인(줄도 rm 기록도 없음)은 거짓 — 지금처럼
// shortcut: rm 기록은 시각을 가리지 않는다 — 지운 이름을 대기열 밖 레인이 다시 쓰면 그 반송도 안 맞춘다, 겪으면 rm 시각을 제출 시각과 견준다
func relandGone(lane, p, n string, rows []Row, rec func(string, string)) bool {
	for _, r := range rows {
		if r.F(qName) == lane && r.F(qProj) == p {
			return false
		}
	}
	es, _, err := journal.Read(journal.Filter{Action: "rm", Target: lane})
	if err != nil {
		say("다시 맞춤 보류 %s: 대기열에서 지워진 레인인지 모름(일지를 못 읽음: %v) — 다음 바퀴에", lane, err)
		return true
	}
	for _, e := range es {
		if e.Tool == "hc-fill" && e.Result == "됨" {
			say("반송 다시 띄움 안 함 %s: 대기열에서 지워진 레인(refs/merge-failed/%s)", lane, n)
			rec("줄 없음", "-")
			return true
		}
	}
	return false
}

func bounceMsg(lane, n string) string {
	return fmt.Sprintf("반송 다시 띄움 %s — 병합기가 제출을 돌려보냄(refs/merge-failed/%s), 같은 이름으로 그 실패만 고치러 다시 띄운다", lane, n)
}

// bounceRelaunch: 그 줄을 반송됨 대기로 — 절 파일을 먼저 쓰고(끊겨도 다음 바퀴 bounceGate가 잇는다) 줄을 바꾼 뒤 장부·알림.
// live가 거짓(원래 레인에 보낸 뒤 기다림이 지남)인데 원래 레인이 아직 살아 있으면 띄우지 않고 지휘에 넘긴다(같은 이름·작업 트리가 겹친다)
func bounceRelaunch(n, lane, base, why string, r Row, live bool, rec func(string, string), recTell func(string, string, string)) {
	owner := dash(r.F(evTo))
	q := qrow(lane)
	p := q.F(qProj)
	if !live {
		if a, st := relandLive(lane, p, n); st == "살아 있음" {
			if aesc(lane, owner, fmt.Sprintf("%s 반송 뒤 원래 레인(%s)에 보냈는데 %d분 안에 다시 내지 않았다 — 지휘 판단 (refs/merge-failed/%s)", lane, a, relandWait(), n), "hf:reland:"+n) == 0 {
				rec("넘김", a)
			}
			return
		}
	}
	lg := "모름(사건 글이 잘림)"
	if m := relandLog.FindStringSubmatch(r.F(evMsg)); m != nil {
		lg = m[1]
	}
	sec := bounceSecText(lane, n, r.F(evMach), why, lg, r.F(evMsg), bounceFails(r.F(evMach), lg), bounceNotes(q.F(qMach), lane))
	if err := os.MkdirAll(DATA+"/bounce", 0o755); err != nil {
		say("반송 다시 띄움 보류 %s: %v", lane, err)
		return
	}
	if err := writeAtomic(bounceFile(lane), sec); err != nil {
		say("반송 다시 띄움 보류 %s: 절을 못 씀: %v", lane, err)
		return
	}
	if qset(true, q.F(qRID), lane, "wait", "-", "-", q.F(qStuck), mBounced.val(), "") != 0 {
		say("반송 다시 띄움 보류 %s: 대기열 줄을 못 바꿈 — 다음 바퀴에", lane)
		return
	}
	recTell("다시 띄움", lane, bounceMsg(lane, n))
}

// bounceFails <병합기 기계> <로그>: 로그의 「실패 줄」 묶음(없으면 끝 20줄), 30줄 안. 못 읽으면 빈 값
func bounceFails(m, lg string) string {
	if !strings.HasPrefix(lg, "/") {
		return ""
	}
	o, rc := onm(nil, m, `[ -r "$d" ] || exit 1; s=$(sed -n '/실패 줄(로그에서/,$p' "$d"); [ -n "$s" ] || s=$(tail -n 20 "$d"); printf '%s\n' "$s" | head -n 30`, lg)
	if rc != 0 {
		return ""
	}
	return strings.TrimSpace(o)
}

// bounceNotes <레인 기계> <레인>: 지휘 메모(hc-fill note가 레인 기계에 남긴 coord-notes.md — 레인이 끝난 뒤 남긴 것도), 끝 60줄
func bounceNotes(m, lane string) string {
	if m == "" || m == "-" {
		return ""
	}
	o, rc := onm(nil, m, `[ ! -f "$d" ] || tail -n 60 "$d"`, coordNotes(lane))
	if rc != 0 {
		return ""
	}
	return strings.TrimSpace(o)
}

// bounceSecText: 반송 절 — 끝 줄바꿈 하나
func bounceSecText(lane, n, mach, why, lg, ev, fails, notes string) string {
	switch {
	case fails == "" && wdEvRe.MatchString(ev):
		fails = "(병합기 로그 없음 — 지휘가 거뒀다. 위 사건 글의 까닭을 고친다)"
	case fails == "":
		fails = "(로그를 못 읽음 — 위 로그 경로를 hc file read --on " + mach + " 로 본다)"
	}
	if notes == "" {
		notes = "(없음)"
	}
	return fmt.Sprintf(`## 반송 — 병합기가 이 레인의 제출을 돌려보냈다 (hc-fill, %[8]s)
위 본 작업은 끝나 감사를 거쳐 제출됐는데, 병합기(기계 %[3]s)가 %[4]s로 refs/merge-failed/%[2]s 에 돌려보냈다. hc-fill이 같은 이름으로 다시 띄웠다 — 그 실패만 고쳐 다시 낸다.
- 병합기 로그: %[5]s (기계 %[3]s의 경로 — 이 기계가 아니면 hc file read --on %[3]s <경로>)
- 병합기 사건: %[6]s
1. 이 작업 트리를 반송된 제출로 옮긴다: git fetch origin refs/merge-failed/%[2]s 뒤 git reset --keep FETCH_HEAD
2. git rebase origin/main — 충돌은 양쪽 내용을 살려 풀고 고친 파일을 경로로 git add 한 뒤 git rebase --continue. stash·rebase --skip·--abort 금지.
3. 아래 실패 줄의 원인을 고쳐 커밋한다(제목 머리 %[1]s:). 실패가 이 레인 탓이 아니면(기준 쪽 시험·병합기 환경) 고치지 말고 hc-lane report 막힘 - "<까닭>" 을 부르고 멈춘다.
4. 고친 둘레의 빌드·시험과 저장소 검증 뒤, 위 감사·착지 절차대로 다시 낸다(hc-lane report 완료 → 감사 판정 → land). 다시 주차하지 않는다.
### 실패 줄(병합기 로그)
%[7]s
### 지휘 메모(coord-notes)
%[9]s
`, lane, n, mach, why, lg, ev, fails, time.Now().Format("2006-01-02 15:04"), notes)
}
