package main

// 띄우기 --resume: 주차한 가지 받기(launch.go에서 그대로 옮김 — 2026-11-24 fill-reject-hold, launch.go 줄 수 여유)

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"herdr-tools/internal/proc"
)

// parkLeftClean: 주차한 실행의 hc-lane clean이 그때 그 폴더를 쓰던 터미널(도는 감사 등)에 막혀 작업 트리가 남았으면(ht-submit-still-stands —
// 그 뒤 clean을 다시 부르는 길이 없어 이어 띄우기가 「워크트리 폴더가 이미 있다」로 거절됐다), 띄우기 전에 지금 있는 clean을 한 번 부른다.
// 안전 검사는 clean 그대로(쓰는 터미널·커밋 안 된 변경이면 멈춤, 「주차됨」은 원격 refs/park/<레인>이 그 커밋일 때만). 못 지우면 아래 「이미 있다」 거절 그대로
func parkLeftClean() {
	if xrun(nil, io.Discard, "test", "-e", wt) != 0 {
		return
	}
	a := []string{"clean", "--repo", repo, "--branch", branch}
	if on != "" {
		a = append(a, "--on", on)
	}
	errf("hc-lane: --resume: 주차 뒤 남은 작업 트리 %s — hc-lane clean 을 한 번 부른다\n", wt)
	c := exec.Command(selfPath, a...)
	c.Stdout, c.Stderr = os.Stderr, os.Stderr
	if err := proc.Run(c); err != nil { // 결과는 바로 뒤 「이미 있나」가 다시 본다
		errf("hc-lane: --resume: 남은 작업 트리를 지우지 못했다(%v) — 그대로 둔다\n", err)
	}
}

// resumeFrom: park가 원격 refs/park/<레인>에 둔 가지를 직접 받는다(제출 가지 lane/<레인>과 다른 이름 — 병합기·clean이 헷갈리지 않게).
// 옛 판이 주차한 lane/<레인>은 refs/park/<레인>이 없을 때만 받는다(바꾼 날 주차해 둔 레인). 돌려준 값은 작업 트리 시작 참조
func resumeFrom() string {
	if !remote {
		refuse("--resume: 원격(origin)이 없는 저장소에는 주차한 가지가 없다: %s", repo)
	}
	wfrom := "refs/hc-park/" + name
	// 모의 실행도 원격을 본다(읽기만 — 참조를 쓰지 않고 시작점을 알린다, gofix L08)
	ls, r := xout("git", "-C", repo, "ls-remote", "origin", "refs/park/"+name, "refs/heads/lane/"+name)
	if r != 0 {
		die("--resume: 원격을 읽지 못했다(git ls-remote) — 주차한 가지 refs/park/%s 를 모른다: %s", name, repo)
	}
	// 응답 줄은 「<40자 SHA><탭><참조>」만 받는다 — 깨진 줄은 주차 SHA가 아니다(R-15)
	got := map[string]string{}
	for _, ln := range strings.Split(ls, "\n") {
		if ln == "" {
			continue
		}
		s, rf, ok := strings.Cut(ln, "\t")
		if !ok || !sha40.MatchString(s) || strings.ContainsAny(rf, " \t") {
			die("--resume: 원격 응답을 읽지 못했다(git ls-remote 줄 꼴: %q) — 주차한 가지를 모른다: %s", cut(ln, 80), repo)
		}
		got[rf] = s
	}
	src, ssha := "", ""
	if s, ok := got["refs/park/"+name]; ok {
		src, ssha = "refs/park/"+name, s
	} else if s, ok := got["refs/heads/lane/"+name]; ok {
		legacyResumeOK(s) // 옛 판 주차: 같은 레인·SHA의 마지막 주차·제출 사건이 「주차」일 때만(R-15)
		src, ssha = "refs/heads/lane/"+name, s
	}
	switch {
	case src == "":
		refuse("--resume: 주차한 가지 refs/park/%s 가 없다: %s", name, repo)
	case dry:
		fmt.Printf("[dry-run] 주차한 가지 %s (%s) 에서 시작\n", src, cut(ssha, 12))
	case xfetch(repo, "-q", "origin", "+"+src+":"+wfrom) != 0:
		die("--resume: 주차한 가지 refs/park/%s 를 받지 못했다: %s", name, repo)
	default:
		// 받은 것이 판정한 그 SHA여야 한다(그사이 움직였으면 판정이 맞지 않는다)
		if v, r := xout("git", "-C", repo, "rev-parse", "-q", "--verify", wfrom+"^{commit}"); r != 0 || v != ssha {
			die("--resume: 받은 주차 가지 %s 가 판정한 %s 와 다르거나 없다 — 같은 줄을 다시: %s", src, cut(ssha, 12), repo)
		}
	}
	return wfrom
}

// legacyResumeOK <lane/<레인> sha>: --resume이 옛 판 주차 가지에서 재개해도 되나 — 아니면 끝낸다. 반송된 옛 제출 가지에서 재개하지
// 않는다(R-15). 사건을 못 읽으면 모름이라 거절(11)이 아니라 종료 1(다시) — fill-reject-hold 감사 중간-1
func legacyResumeOK(s string) {
	if p, known := legacyParkedK(name, s); !known {
		die("--resume: 사건(events.tsv)을 읽지 못해 lane/%s 가 주차 가지인지 모른다 — 같은 줄을 다시: %s", name, repo)
	} else if !p {
		refuse("--resume: refs/park/%s 가 없고 lane/%s(%s)는 주차 사건이 없는 제출 가지다 — 재개하지 않는다: %s", name, name, cut(s, 12), repo)
	}
}
