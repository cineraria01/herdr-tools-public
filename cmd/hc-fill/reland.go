package main

// 자동 다시 맞춤(Go 판에서 더함, 2026-11-22 go-auto-reland — 11/21 병합기 rebase 충돌 반송이 하루 열 번 넘었고, 그때마다 지휘가
// 「lane 가지를 가져와 rebase·검증·다시 제출」 지시문을 손으로 써 띄웠다). 병합기 기계가 적은 막힘 사건 가운데 rebase 충돌
// (「병합기: 착지 실패(종료 3, 충돌 …)」 — cmd/hc-merger/judge.go)만 골라 대기열에 <레인>-reland(같은 프로젝트·같은 주인·우선 높음·
// 자동 감사 끔)를 넣고 주인 지휘에 「자동 다시 맞춤 넣음」 사건 한 줄. 같은 레인(-reland 꼬리를 뗀 이름)은 설정 reland_max번
// (기본 2, 0이면 끔)까지, 넘으면 주인 지휘에 막힘만. 검증 실패(종료 1)·일시 실패(75 등)는 꼴이 달라 건드리지 않는다.
// 처리한 반송은 reland.tsv 「큐 이름 레인 바탕 결과 다시맞춤 시각」, 첫 줄 「#since <UTC>」 — 파일이 처음 생길 때의 시각이고,
// 그 앞의 반송(지휘가 이미 손으로 다시 띄운 것)은 보지 않는다.

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	relandRe  = regexp.MustCompile(`^병합기: 착지 실패\(종료 3, 충돌 (.*?)\) — `)
	relandQRe = regexp.MustCompile(`refs/merge-failed/([0-9]+-[a-z0-9_-]+) `)
	relandLog = regexp.MustCompile(`, 로그 (\S+)`)
	relandSuf = regexp.MustCompile(`-reland[0-9]*$`)
)

// relandMax: 설정 reland_max(빼면 2, 0이면 끔)
func relandMax() int {
	v := conf("reland_max")
	if v == "" {
		return 2
	}
	if !isDigits(v) {
		say("알림: 설정 reland_max는 횟수(숫자)다, 2로: %s", v)
		return 2
	}
	return atoi10(v)
}

// relandName <바탕> <k>: k번째 다시 맞춤 이름 — <바탕>-reland, 둘째부터 -reland<k>. 32자를 넘으면 바탕을 줄인다
func relandName(base string, k int) string {
	suf := "-reland"
	if k > 1 {
		suf += strconv.Itoa(k)
	}
	if len(base)+len(suf) > 32 {
		base = strings.TrimRight(base[:32-len(suf)], "-_")
	}
	return base + suf
}

// relandTick: run 바퀴가 사건(EV)을 읽은 뒤 부른다. dry-run·사건 못 읽음은 건너뜀
func relandTick() {
	mx := relandMax()
	if mx == 0 || dry || EVBAD || len(EV) == 0 {
		return
	}
	rf := DATA + "/reland.tsv"
	if !isFile(rf) {
		if err := writeAtomic(rf, "#since\t"+utcnow()+"\n"); err != nil {
			fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		}
		return
	}
	text, ok := readFile(rf)
	if !ok {
		say("다시 맞춤 보류: %s 를 못 읽음", rf)
		return
	}
	since, seen, cnt := "~", map[string]bool{}, map[string]int{}
	for _, l := range lines(text) {
		r := splitRow(l)
		switch {
		case r.F(1) == "#since":
			since = r.F(2)
		case r.F(1) != "":
			seen[r.F(1)] = true
			if r.F(4) == "넣음" {
				cnt[r.F(3)]++
			}
		}
	}
	mg := map[string]bool{}
	for _, m := range fields(MGM) {
		mg[m] = true
	}
	for _, e := range EV {
		r := splitRow(e)
		if r.F(5) != "막힘" || !mg[r.F(2)] || r.F(1) < since {
			continue
		}
		mm, qm := relandRe.FindStringSubmatch(r.F(7)), relandQRe.FindStringSubmatch(r.F(7))
		if mm == nil || qm == nil || seen[qm[1]] {
			continue
		}
		seen[qm[1]] = true
		relandOne(rf, qm[1], mm[1], r, mx, cnt)
	}
}

// relandOne: 반송 하나(큐 이름 n) — 프로젝트를 찾고(원격에 refs/merge-failed/<n>이 아직 있어야), 상한이면 막힘, 아니면 줄을 넣는다.
// 원격을 못 읽거나 이름이 아직 대기열에 있으면 적지 않고 다음 바퀴에
func relandOne(rf, n, files string, r Row, mx int, cnt map[string]int) {
	lane, mach, owner := r.F(4), r.F(2), r.F(9)
	if owner == "-" {
		owner = ""
	}
	base := relandSuf.ReplaceAllString(lane, "")
	rec := func(res, name string) {
		if err := appendLine(rf, strings.Join([]string{n, lane, base, res, name, utcnow()}, "\t")); err != nil {
			fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		}
	}
	p, st := relandFind(n)
	switch st {
	case "모름":
		say("다시 맞춤 보류 %s: 원격 refs/merge-failed/%s 를 못 읽음 — 다음 바퀴에", lane, n)
		return
	case "없음":
		say("다시 맞춤 안 함 %s: refs/merge-failed/%s 가 원격에 없다(이미 치움)", lane, n)
		rec("없음", "-")
		return
	}
	if cnt[base] >= mx {
		msg := fmt.Sprintf("%s 병합기 rebase 충돌로 또 반송 — 자동 다시 맞춤 %d번 넘음, 지휘 판단 (refs/merge-failed/%s, 충돌 %s)", lane, mx, n, files)
		if aesc(lane, dash(owner), msg, "hf:reland:"+n) == 0 {
			rec("상한", "-")
		}
		return
	}
	name := relandName(base, cnt[base]+1)
	if qHasName(name) {
		// 넣은 뒤 기록 전에 끊긴 바퀴면 그 줄의 지시문이 이 반송을 가리킨다 — 기록만 잇는다
		if b, _ := readFile(qrow(name).F(3)); strings.Contains(b, "refs/merge-failed/"+n+" ") {
			rec("넣음", name)
			cnt[base]++
		} else {
			say("다시 맞춤 보류 %s: %s 가 아직 대기열에 있다 — 빠진 뒤에", lane, name)
		}
		return
	}
	lg := "모름(사건 글이 잘림)"
	if m := relandLog.FindStringSubmatch(r.F(7)); m != nil {
		lg = m[1]
	}
	bf := DATA + "/briefs/added/" + name + ".md"
	tmp := bf + ".tmp." + strconv.Itoa(os.Getpid())
	if err := os.MkdirAll(DATA+"/briefs/added", 0o755); err != nil {
		say("다시 맞춤 보류 %s: %v", lane, err)
		return
	}
	if err := os.WriteFile(tmp, []byte(relandBrief(name, lane, n, mach, files, lg, r.F(7))), 0o644); err != nil {
		say("다시 맞춤 보류 %s: 지시문을 못 씀: %v", lane, err)
		return
	}
	if qAdd(tmp, name, p, bf, "-", "-", "wait", "-", "-", "-", nowMin(), "-", dash(owner), "high", "-", "no-audit", "-") != 0 {
		os.Remove(tmp)
		say("다시 맞춤 보류 %s: 대기열에 못 넣음 — 다음 바퀴에", lane)
		return
	}
	rec("넣음", name)
	cnt[base]++
	msg := fmt.Sprintf("자동 다시 맞춤 넣음 %s — %s 병합기 rebase 충돌 반송(refs/merge-failed/%s, 충돌 %s)", name, lane, n, files)
	if hcLaneReport("중간", msg, lane, owner, "hf:reland:"+n) != 0 {
		say("알림 실패(hc-lane report) — %s", msg)
		return
	}
	say("%s", msg)
}

// relandFind <큐 이름>: 병합기 설정 프로젝트 가운데 원격에 refs/merge-failed/<n>이 있는 것. 있음·없음·모름(원격을 못 읽은 프로젝트가 있음)
func relandFind(n string) (string, string) {
	var ps []string
	for _, f := range confRows("merger") {
		if len(f) >= 2 {
			ps = append(ps, f[1])
		}
	}
	unk := false
	for _, p := range uniq(ps) {
		r := repoOf(p, LOCAL)
		if r == "" {
			unk = true
			continue
		}
		o, rc := run("git", "-C", r, "ls-remote", "origin", "refs/merge-failed/"+n)
		if rc != 0 {
			unk = true
			continue
		}
		if strings.TrimSpace(o) != "" {
			return p, "있음"
		}
	}
	if unk {
		return "", "모름"
	}
	return "", "없음"
}

// relandBrief: 다시 맞춤 레인 지시문(착지·정리 절차는 hc-fill이 run 지시문 끝에 붙인다)
func relandBrief(name, lane, n, mach, files, lg, ev string) string {
	return fmt.Sprintf(`# 레인 %[1]s — %[2]s 병합기 rebase 충돌 반송 다시 맞춤 (hc-fill 자동, %[8]s)

병합기(기계 %[4]s)가 %[2]s 의 제출을 rebase 충돌(착지 실패 종료 3)로 refs/merge-failed/%[3]s 에 돌려보냈다. 그 커밋을 지금 origin/main 위로 다시 맞춰 넣는다(같은 커밋이 이미 감사를 거쳤으니 자동 감사는 끔).
- 충돌 파일(병합기가 3자 합침으로 본 것): %[5]s
- 병합기 로그: %[6]s (기계 %[4]s의 경로 — 이 기계가 아니면 hc file read --on %[4]s <경로>, 못 읽으면 rebase로 직접 본다)
- 병합기 사건: %[7]s

## 할 것
1. 이 작업 트리(가지 %[1]s, origin/main에서 시작)를 반송된 제출로 옮긴다: git fetch origin refs/merge-failed/%[3]s 뒤 git reset --keep FETCH_HEAD
2. git rebase origin/main — 충돌은 양쪽 내용을 살려 풀고 고친 파일을 경로로 git add 한 뒤 git rebase --continue. git stash·rebase --skip·rebase --abort 금지.
3. 커밋 제목은 원래 그대로 둔다(머리 %[2]s: — 이 이름을 --after로 기다리는 레인이 풀리게). 풀이로 새 커밋을 더하면 머리는 %[1]s:.
4. 저장소 검증(.claude/herdr-verify.sh). 실패하면 원인을 고쳐 커밋하고 다시.
5. 충돌을 풀며 기능 코드의 뜻(동작)을 바꿔야 했으면 착지하지 말고 hc-lane report 막힘 - "<까닭>" 을 부르고 멈춘다. 아니면 아래 착지·정리 절차(hc-lane land main → hc-lane clean)대로.
6. 또 rebase 충돌로 반송되면 hc-fill이 설정 횟수까지 다시 넣고, 넘으면 지휘에 막힘으로 넘긴다 — 이 레인은 따로 할 일 없다.

보고는 한국어. 질문 창 금지(되돌릴 수 있는 판단은 추천안으로 정해 보고에 「가정/결정」). 멈출 때는 네가 띄운 정확한 pid로만.
`, name, lane, n, mach, files, lg, ev, nowMin())
}
