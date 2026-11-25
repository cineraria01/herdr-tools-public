// auditreport.go: 감사 보고를 감사 아이디만으로 어느 기계에서든 읽게(ht-audit-report-reach, 2026-11-26).
// 보고는 감사가 돈 기계의 ~/.local/share/hc-fill/reports/audit-<ID>.md에만 있어, 다른 기계 레인이 못 읽고 지휘가 손으로 옮겼다.
// 감사가 판정을 적을 때(report 판정 「감사 완료 <ID> …」) 그 보고를 저장소 원격 refs/hc-audit/<ID>에 부모 없는 커밋(audit.md 하나)으로
// 올리고, hc-lane audit-report <ID>는 로컬에 없으면 그 참조를 fetch해 읽는다 — git 원격은 모든 기계가 닿는다(병합기 refs/merge-queue/*와 같은 길).
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/gitx"
	"herdr-tools/internal/proc"
)

func init() { register("audit-report", auditReportMain, nil) }

const auditRefPre = "refs/hc-audit/"

func auditReportPath(aid string) string {
	return os.Getenv("HOME") + "/.local/share/hc-fill/reports/audit-" + aid + ".md"
}

// auditPublish: 판정 사건 글(「감사 완료 <ID> …」)의 감사 보고를 지금 저장소의 원격에 올린다. 실패는 경고만(판정 사건은 이미 적혔다)
// ponytail: 참조는 쌓이기만 한다 — 많아지면 gc에서 오래된 refs/hc-audit/*를 지운다
func auditPublish(msg string) {
	f := strings.Fields(msg)
	if len(f) < 3 || f[0] != "감사" || f[1] != "완료" || !idChars(f[2]) {
		return
	}
	auditPush(f[2], "")
}

// auditPush: 이 기계의 보고 audit-<ID>.md를 repo(빈 값이면 지금 폴더)의 origin refs/hc-audit/<ID>에. 종료 0 올림, 1 보고 없음·올리기 실패
func auditPush(aid, repo string) int {
	p := auditReportPath(aid)
	if st, err := os.Stat(p); err != nil || !st.Mode().IsRegular() {
		return 1 // 보고 파일을 안 쓴 감사 — 올릴 것이 없다
	}
	var eb bytes.Buffer
	blob, rc := gitx.Out(repo, &eb, "hash-object", "-w", "--", p)
	tree := ""
	if rc == 0 {
		c := exec.Command("git", "mktree")
		c.Dir = repo
		c.Stdin = strings.NewReader("100644 blob " + blob + "\taudit.md\n")
		var o bytes.Buffer
		c.Stdout, c.Stderr = &o, &eb
		if proc.Run(c) == nil {
			tree = strings.TrimSpace(o.String())
		}
	}
	cm := ""
	if tree != "" {
		cm, rc = gitx.Out(repo, &eb, "commit-tree", tree, "-m", "hc-audit "+aid)
	}
	if cm == "" || rc != 0 || gitx.Run(repo, nil, &eb, "push", "-q", "-f", "origin", cm+":"+auditRefPre+aid) != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane report: 경고 — 감사 보고를 원격(%s%s)에 올리지 못했다. 다른 기계 레인은 hc-lane audit-report로 못 읽는다 — "+
			"이 기계 이 저장소에서 hc-lane audit-report --publish %s 로 다시 올린다: %s\n",
			auditRefPre, aid, aid, tailLine(strings.TrimSpace(eb.String())))
		return 1
	}
	fmt.Printf("감사 보고 올림 %s%s (어느 기계든 hc-lane audit-report %s)\n", auditRefPre, aid, aid)
	return 0
}

// hc-lane audit-report [--publish] <감사 ID> [--repo <저장소>]: 보고를 표준 출력에. 로컬에 없으면 origin refs/hc-audit/<ID>에서 받아 로컬에 남긴다.
// --publish는 감사 기계에서 판정 때 못 올린 보고를 다시 올린다(사건을 더 적지 않는다).
// 종료 0 읽음·올림, 1 어디에도 없음·올리기 실패, 2 인자·저장소 아님·origin 없음, 75 원격 조회 실패(다시)
func auditReportMain(a []string) int {
	var aid, repo string
	pub := false
	for i := 0; i < len(a); i++ {
		switch {
		case a[i] == "--repo":
			repo = val(a, i)
			i++
		case a[i] == "--publish":
			pub = true
		case strings.HasPrefix(a[i], "-"):
			dieArg("audit-report: 알 수 없는 인자: %s", a[i])
		case aid != "":
			dieArg("audit-report: 감사 ID는 하나: %s", a[i])
		default: // 파일 이름(audit-<ID>.md)을 줘도 된다
			aid = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(a[i]), "audit-"), ".md")
		}
	}
	if aid == "" || !idChars(aid) {
		fmt.Fprintf(os.Stderr, "hc-lane audit-report: 감사 ID를 준다(보고 파일 이름 audit-<ID>.md의 <ID>, 글자 A-Za-z0-9_-): %q\n", aid)
		return 2
	}
	// 저장소 밖·origin 없음은 다시 해도 낫지 않는다 — 75(다시)가 아니라 2. 로컬 사본 읽기는 저장소가 없어도 된다
	noOrigin := func() bool {
		if _, rc := gitx.Out(repo, nil, "remote", "get-url", "origin"); rc != 0 {
			fmt.Fprintf(os.Stderr, "hc-lane audit-report: %s 는 origin이 있는 git 저장소가 아니다 — 감사한 저장소에서 부르거나 --repo <저장소>를 준다\n", orDot(repo))
			return true
		}
		return false
	}
	p := auditReportPath(aid)
	if pub {
		if _, err := os.Stat(p); err != nil {
			fmt.Fprintf(os.Stderr, "hc-lane audit-report: 이 기계에 보고 %s 가 없다 — --publish는 감사가 돈 기계에서\n", p)
			return 1
		}
		if noOrigin() {
			return 2
		}
		return auditPush(aid, repo)
	}
	if b, err := os.ReadFile(p); err == nil {
		os.Stdout.Write(b)
		return 0
	}
	if noOrigin() {
		return 2
	}
	ref := auditRefPre + aid
	var eb, ob bytes.Buffer
	if gitx.Run(repo, nil, &eb, "fetch", "-q", "--no-tags", "origin", "+"+ref+":"+ref) != 0 {
		e := strings.TrimSpace(eb.String())
		if strings.Contains(e, "couldn't find remote ref") {
			fmt.Fprintf(os.Stderr, "hc-lane audit-report: 감사 보고 %s 가 없다 — 이 기계(%s)에도 원격(origin %s)에도. "+
				"아이디를 확인하거나(지휘 글의 「감사 완료 <ID>」), 감사가 아직 판정 전이거나, 다른 저장소의 감사면 --repo <그 저장소>. "+
				"판정 때 올리기가 실패했으면(감사 쪽 경고) 감사가 돈 기계에서 hc-lane audit-report --publish %s\n", aid, p, ref, aid)
			return 1
		}
		fmt.Fprintf(os.Stderr, "hc-lane audit-report: 원격에서 %s 를 받지 못했다(저장소 %s) — 잠시 뒤 다시: %s\n", ref, orDot(repo), tailLine(e))
		return 75
	}
	if gitx.Run(repo, &ob, &eb, "cat-file", "blob", ref+":audit.md") != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane audit-report: %s 에 audit.md가 없다 — 지휘에 알린다\n", ref)
		return 1
	}
	if os.MkdirAll(filepath.Dir(p), 0o755) == nil {
		_ = atomic.WriteFile(p, ob.Bytes(), 0o644) // 로컬에 남기기는 덤 — 못 써도 읽기는 됐다
	}
	os.Stdout.Write(ob.Bytes())
	return 0
}

// tailLine: 여러 줄 오류의 끝 줄
func tailLine(s string) string { return s[strings.LastIndex(s, "\n")+1:] }

func orDot(s string) string {
	if s == "" {
		return "."
	}
	return s
}
