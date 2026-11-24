package main

// 선행 판정 읽기를 한 프로세스(= run 한 바퀴) 안에서 한 번으로(2026-11-24 fill-after-cost — 대기 줄 23개쯤에서 띄우기·선행 단계가 각각 바깥
// 명령 170번쯤: 줄마다 기본 가지 확인(git remote get-url·symbolic-ref 2번), 선행마다 제목 이력(git log)·착지 표식(git ls-remote)·조상
// 확인(merge-base)을 다시 읽었다). 저장소마다 기본 가지·제목 이력·표식 목록·가지의 커밋 목록을 한 번 읽어 모든 줄과 두 단계(launch·after)가
// 함께 쓴다. 바퀴 사이에는 들고 다니지 않는다(바퀴마다 새 프로세스) — 「아직」은 바퀴마다 다시 읽고, 읽기 실패는 그 바퀴 안에서만 모름이다.
// fetch가 그 저장소를 새로 받으면 그 저장소의 이력·표식·조상 판정을 버린다(바퀴 앞쪽 단계와 띄우기 단계 사이에 fetch를 한 번 더 한다)

import "strings"

type pcRes struct {
	out string
	ok  bool
}

var (
	pcRef   = map[string]pcRes{}           // 저장소 → 기본 가지(projectRef)
	pcLog   = map[string]pcRes{}           // 저장소\t가지 → git log <가지> --format=%h %s
	pcMarks = map[string]pcRes{}           // 저장소 → git ls-remote origin refs/hc-landed/*
	pcIn    = map[string]map[string]bool{} // 저장소\t가지 → git rev-list <가지>(그 가지에 든 커밋, 못 읽었으면 빈 것)
	pcAnc   = map[string]int{}             // 저장소\tsha\t가지 → merge-base --is-ancestor 0|1(실패는 두지 않는다)
)

// tickReset: 바퀴 캐시를 모두 비운다(시험이 한 프로세스에서 바퀴를 여러 번 돌 때)
func tickReset() {
	pcRef, pcLog, pcMarks, pcIn, pcAnc = map[string]pcRes{}, map[string]pcRes{}, map[string]pcRes{}, map[string]map[string]bool{}, map[string]int{}
}

// pcDrop: 저장소를 새로 fetch했다 — 원격을 따라 바뀌는 읽기(이력·표식·조상)를 버린다. 기본 가지 이름(origin/HEAD)은 fetch로 안 바뀐다
func pcDrop(repo string) {
	delete(pcMarks, repo)
	pre := repo + "\t"
	for k := range pcLog {
		if strings.HasPrefix(k, pre) {
			delete(pcLog, k)
		}
	}
	for k := range pcIn {
		if strings.HasPrefix(k, pre) {
			delete(pcIn, k)
		}
	}
	for k := range pcAnc {
		if strings.HasPrefix(k, pre) {
			delete(pcAnc, k)
		}
	}
}

// pcSubjects: 저장소 가지의 「%h %s」 이력(바퀴에 한 번). ok=false면 git 실패(모름 — 반쪽 출력은 버림)
func pcSubjects(repo, ref string) (string, bool) {
	k := repo + "\t" + ref
	if r, hit := pcLog[k]; hit {
		return r.out, r.ok
	}
	o, rc := run("git", "-C", repo, "log", ref, "--format=%h %s")
	if rc != 0 {
		o = ""
	}
	pcLog[k] = pcRes{o, rc == 0}
	return o, rc == 0
}

// pcLanded: 저장소 원격의 착지 표식 전부 「sha<TAB>ref」 줄(바퀴에 한 번) — 선행마다 refs/hc-landed/<선행>/* 를 따로 묻던 것
func pcLanded(repo string) (string, bool) {
	if r, hit := pcMarks[repo]; hit {
		return r.out, r.ok
	}
	o, rc := run("git", "-C", repo, "ls-remote", "origin", "refs/hc-landed/*")
	if rc != 0 {
		o = ""
	}
	pcMarks[repo] = pcRes{o, rc == 0}
	return o, rc == 0
}

// pcAncestor: merge-base --is-ancestor sha ref의 종료 코드. 가지의 커밋 목록(바퀴에 한 번, 표식을 처음 견줄 때 읽음)에 든 sha면 명령 없이 0.
// 아니면(목록을 못 읽었거나 없는 sha) merge-base로 — 이 저장소에 없는 커밋의 오류(판정 못 함)를 「아님」과 가르는 옛 판정 그대로.
// 0·1은 두고 그 밖의 실패는 두지 않는다
func pcAncestor(repo, sha, ref string) int {
	k := repo + "\t" + ref
	in, hit := pcIn[k]
	if !hit {
		in = map[string]bool{}
		if o, rc := run("git", "-C", repo, "rev-list", ref); rc == 0 {
			for _, l := range lines(o) {
				in[strings.TrimSpace(l)] = true
			}
		}
		pcIn[k] = in
	}
	if in[sha] {
		return 0
	}
	ka := k + "\t" + sha
	if rc, hit := pcAnc[ka]; hit {
		return rc
	}
	_, rc := run("git", "-C", repo, "merge-base", "--is-ancestor", sha, ref)
	if rc == 0 || rc == 1 {
		pcAnc[ka] = rc
	}
	return rc
}
