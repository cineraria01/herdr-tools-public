package main

// 병합기 큐·켬 표식 — 셸 판 lib/hc-fill/tick.sh의 mqload·mqueued·mgmach·merger_tick

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/landing"
)

// mqload <프로젝트>: 그 프로젝트 원격의 refs/merge-queue/*·refs/herdr/merger/*를 한 바퀴에 한 번 MQS에. 못 읽으면 MQBAD에
func mqload(p string) {
	if inlist(p, MQP) {
		return
	}
	MQP += "," + p
	r := repoOf(p, LOCAL)
	if r == "" {
		bad := false
		for _, f := range confRows("merger") {
			if F1(f, 1) == p {
				bad = true
				break
			}
		}
		if !bad {
			for _, x := range readRows(DATA + "/merger.tsv") {
				if x.F(mgProj) == p {
					bad = true
					break
				}
			}
		}
		if bad {
			MQBAD += "," + p
		}
		return
	}
	switch _, rc := run("git", "-C", r, "remote", "get-url", "origin"); rc {
	case 0:
	case 2:
		return
	default:
		MQBAD += "," + p
		return
	}
	// 응답 전체가 온전해야 받는다(gitx.LsRemote) — 잘린 줄 하나라도 있으면 이 프로젝트 큐 전체를 모름으로(재감사 fill/R-214: 잘린 줄을
	// 건너뛰어 큐·착지에 그 제출이 없다고 확정했다). 「없음」은 --exit-code 종료 2뿐
	refs, st := gitx.LsRemote(r, "origin", nil, "refs/merge-queue/*", "refs/herdr/merger/*", landing.Prefix+"*")
	if st == gitx.Unknown {
		MQBAD += "," + p
		return
	}
	for _, x := range landing.Refs(refs) {
		MQS = append(MQS, Row{p, x.Name, x.SHA})
	}
}

// mqlanding <프로젝트> <레인>: 병합기가 그 레인 제출을 지금 착지 중이면 0, 아니면 1, 못 읽으면 2(internal/landing — hc-lane land와 같은 판정)
func mqlanding(p, lane string) int {
	mqload(p)
	if inlist(p, MQBAD) {
		return 2
	}
	var refs []landing.Ref
	for _, r := range MQS {
		if r.F(mqProj) == p {
			refs = append(refs, landing.Ref{Name: r.F(mqRef), SHA: r.F(mqSHA)})
		}
	}
	if landing.Of(refs, lane) != "" {
		return 0
	}
	return 1
}

var mgQueueRe = regexp.MustCompile(`^refs/merge-queue/[0-9]+-`)

// mqueued <프로젝트> <레인>: 그 레인 제출이 큐에 있으면 0, 없으면 1, 못 읽으면 2
func mqueued(p, lane string) int {
	mqload(p)
	if inlist(p, MQBAD) {
		return 2
	}
	for _, r := range MQS {
		if r.F(mqProj) != p {
			continue
		}
		if loc := mgQueueRe.FindStringIndex(r.F(mqRef)); loc != nil && r.F(mqRef)[loc[1]:] == lane {
			return 0
		}
	}
	return 1
}

// mgmach <프로젝트>: 켜진 병합기 기계
func mgmach(p string) []string {
	mqload(p)
	var o []string
	for _, r := range MQS {
		if m, ok := strings.CutPrefix(r.F(mqRef), "refs/herdr/merger/"); r.F(mqProj) == p && ok {
			o = append(o, m)
		}
	}
	return o
}

// mgDrop: merger.tsv에서 그 프로젝트 줄을 뺀 줄들(awk -F'\t' '$1 != p'). ok=false면 못 읽음
func mgDrop(path, p string) (string, bool) {
	text, ok := readFile(path)
	if !ok {
		return "", false
	}
	var b strings.Builder
	for _, l := range lines(text) {
		if splitRow(l).F(mgProj) != p {
			b.WriteString(l + "\n")
		}
	}
	return b.String(), true
}

// merger_tick: 설정 줄 「merger <프로젝트> <기계>」를 원격 켬 표식 refs/herdr/merger/<기계>로(줄을 지우면 표식을 지운다)
func mergerTick() {
	mgf := DATA + "/merger.tsv"
	var want []Row
	var ps []string
	for _, f := range confRows("merger") {
		if len(f) >= 3 {
			want = append(want, Row{f[1], f[2]})
			ps = append(ps, f[1])
		}
	}
	for _, x := range readRows(mgf) {
		ps = append(ps, x.F(mgProj))
	}
	for _, p := range strings.Fields(strings.Join(uniq(ps), "\n")) {
		w := ""
		for _, x := range want {
			if x.F(mgProj) == p {
				w = x.F(mgMach)
				break
			}
		}
		r := repoOf(p, LOCAL)
		if r == "" {
			say("병합기 %s: 설정에 %s repo 줄이 없다 — 켬 표식을 못 고침", p, LOCAL)
			continue
		}
		mqload(p)
		if inlist(p, MQBAD) {
			say("병합기 %s: 원격을 못 읽음 — 켬 표식은 다음 바퀴에", p)
			continue
		}
		cur := mgmach(p)
		var args []string
		for _, c := range strings.Fields(strings.Join(cur, "\n")) {
			if c != w {
				args = append(args, ":refs/herdr/merger/"+c)
			}
		}
		if w != "" && !has(cur, w) {
			args = append(args, "+refs/remotes/origin/HEAD:refs/herdr/merger/"+w)
		}
		if !dry && w != "" {
			// 켬 push의 응답을 잃더라도 설정 줄을 지우면 다음 바퀴가 끌 수 있어야 한다.
			keep, ok := "", true
			if exists(mgf) {
				keep, ok = mgDrop(mgf, p)
			}
			if !ok || writeAtomic(mgf, keep+p+"\t"+w+"\n") != nil {
				say("병합기 %s: 관리 기록을 못 써 켬 표식을 보류", p)
				continue
			}
		}
		if len(args) > 0 {
			if dry {
				say("%s병합기 %s: git push origin %s", P, p, strings.Join(args, " "))
			} else if _, rc := run("git", append([]string{"-C", r, "push", "-q", "--atomic", "origin"}, args...)...); rc == 0 {
				if w != "" {
					say("병합기 %s: 켬 → %s", p, w)
				} else {
					say("병합기 %s: 끔", p)
				}
			} else {
				say("병합기 %s: 켬 표식 push 실패 — 다음 바퀴에", p)
				continue
			}
		}
		if dry {
			continue
		}
		if w == "" {
			if keep, ok := mgDrop(mgf, p); ok {
				if err := writeAtomic(mgf, keep); err != nil {
					fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
				}
			}
		}
	}
	// 큐는 그대로 — 병합기 살아 있음(mergeralive.go)이 다시 읽지 않고 쓴다
	maSnap = mqSnap{MQP, MQS, MQBAD}
	MQP, MQS, MQBAD = "", nil, "" // 켬 표식을 고쳤을 수 있으니 이 바퀴의 나머지는 다시 읽는다
}
