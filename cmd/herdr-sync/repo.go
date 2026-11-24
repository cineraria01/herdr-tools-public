package main

// 맥 ↔ 저장소: --pull <기계>:…로 맥에 가져오기, 3-way 모음·깖, prebuild

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"herdr-tools/internal/ojson"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/toolcfg"
)

var (
	body     bool     // 저장소에서 지침 본문을 깔았다(prebuild·모음을 한 번 더)
	rdiff    []string // 맥 ↔ 저장소 다름 묶음
	rconf    int      // 맥 ↔ 저장소 충돌 수
	RBH, RAH []string // --check: 저장소가 새로움 · 맥이 새로움(저장소 경로)
	RSUF     string   // 맥이 저장소보다 뒤면 기계 줄 끝 표시
	rows     string
	rc       int
)

func readJSONFile(p string) (any, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return ojson.Parse(b)
}

func objGet(v any, k string) any {
	if o, ok := v.(*ojson.Object); ok {
		return o.M[k]
	}
	return nil
}

func strList(v any) []string {
	a, _ := v.([]any)
	var r []string
	for _, e := range a {
		if s, ok := e.(string); ok {
			r = append(r, s)
		}
	}
	return r
}

// repoSync: 맥 ↔ 저장소 3-way. 모두 정한 뒤, 충돌이 하나라도 있으면 쓰지 않는다. 저장소는 줄마다 UD(범용 또는 개인)다.
// 범용 저장소로 모을 맥 값에 개인 값이 있으면 충돌이다. 단계는 repo_steps.go
func repoSync() {
	q, ok := query("")
	if !ok {
		die("맥 값을 읽지 못했다")
	}
	RS := repoSigs() // 저장소 쪽 서명
	LK := tleaks(q, RS)
	RP := repoBasePaths()
	rbp, acts, idx := repoPlan(q, RS, RP, LK)
	if check {
		repoCheckRows(idx, acts, RS)
		return
	}
	if rconf > 0 {
		row("맥", rdiff, rconf)
		fmt.Print(rows)
		fmt.Fprintln(os.Stderr, "herdr-sync: 저장소와 충돌이 있어 멈춘다. --take repo:<경로>(맥 값) 또는 --pull repo:<경로>(저장소 값)로 푼다. 개인 값이면 맥 파일에서 빼거나 그 줄을 개인 저장소로 옮긴다")
		exit(1)
	}
	// 모음을 먼저 커밋·push하고 base를 쓴 뒤에 깐다. 깔다 멈춰도 모은 것이 스테이징에만 남지 않는다.
	// 쓰기·스테이징·커밋이 실패하면 그 저장소에서 이번에 한 것을 되돌리고(undo) 멈춘다. 「맥 모음」 줄은 그 저장소 커밋이 된 뒤에 찍는다
	rbp = append(rbp, repoGather(q, idx, acts, RS)...)
	repoPush()
	if len(rbp) > 0 && runIn(mac("B"), bytes.NewReader(joinLines(rbp)), nil) != 0 {
		die("저장소 base를 쓰지 못했다")
	}
	rbp2, bad := repoInstall(idx, acts, RS)
	if len(rbp2) > 0 && runIn(mac("B"), bytes.NewReader(joinLines(rbp2)), nil) != 0 { // 깐 것의 base에 저장소 경로를 함께
		die("저장소 base를 쓰지 못했다")
	}
	if bad {
		exit(1)
	}
	rdiff = nil
}

// build: 맞추기 전에 개인 설정 sync.prebuild를 sh -c로 돌린다(예: 맥 지침에서 Codex 공통 구역을 다시 만듦). 없으면 아무것도 안 한다
func build() {
	if check {
		return
	}
	pre, err := toolcfg.Lookup(CONF, "sync.prebuild", "", os.Stderr)
	if err != nil {
		die("개인 설정을 읽지 못했다: %s/config", CONF)
	}
	if pre == "" {
		return
	}
	c := exec.Command("sh", "-c", pre)
	c.Stderr = os.Stderr
	if rc := run(c); rc != 0 { // 명령 원문은 찍지 않는다 — 개인 경로·인증 인자가 들 수 있다(gofix-ops B17)
		die("sync.prebuild 실패 (개인 설정 키 sync.prebuild, 종료 %d)", rc)
	}
}

// row: 기계 한 줄(다름 묶음 목록·충돌 수)
func row(name string, diffs []string, conf int) {
	seen := map[string]bool{}
	var d []string
	for _, x := range diffs {
		for _, w := range strings.Fields(x) {
			if !seen[w] {
				seen[w] = true
				d = append(d, w)
			}
		}
	}
	s := ""
	if len(d) > 0 {
		s = "다름 (" + strings.Join(d, "·") + ")"
	}
	if conf > 0 {
		if s != "" {
			s += " · "
		}
		s += fmt.Sprintf("충돌 %d건", conf)
	}
	if s != "" {
		rc = 1
	} else {
		s = "같음"
	}
	if l := binLost[name]; len(l) > 0 {
		s += " — 못 받은 판: " + strings.Join(l, "; ")
	}
	if !check {
		rowResult(name, len(d) > 0, conf) // 배포 기록(deploylog.go)
	}
	rows += name + ": " + s + RSUF + "\n"
}

func few(a []string) string {
	if len(a) > 3 {
		return strings.Join(a[:3], " ") + " …"
	}
	return strings.Join(a, " ")
}

// macrow: --check의 맥 줄(맥 ↔ 두 저장소의 방향)
func macrow() {
	var p []string
	if len(RBH) > 0 {
		p = append(p, fmt.Sprintf("저장소가 새로움 %d개 (%s) — herdr-sync로 깐다", len(RBH), few(RBH)))
	}
	if len(RAH) > 0 {
		p = append(p, fmt.Sprintf("저장소보다 새로움 %d개 (%s) — 커밋 안 됨?", len(RAH), few(RAH)))
	}
	var d []string
	for _, w := range []string{"지움", "바이너리"} {
		for _, x := range rdiff {
			if x == w {
				d = append(d, w)
				break
			}
		}
	}
	if len(d) > 0 {
		p = append(p, "다름 ("+strings.Join(d, "·")+")")
	}
	if rconf > 0 {
		p = append(p, fmt.Sprintf("충돌 %d건", rconf))
	}
	s := strings.Join(p, " · ")
	if s != "" {
		rc = 1
	} else {
		s = "저장소와 같음"
	}
	rows = "맥: " + s + "\n"
}

// ready <저장소>: git·main·깨끗함 확인 뒤 pull --ff-only. git 조회가 실패하거나 시간을 넘기면(124 — 모름) 깨끗함·main으로 읽지 않고 멈춘다
// (gofix-ops B01·O-01·K-01: 실패한 status의 빈 출력을 깨끗함으로 읽어 커밋 안 된 내용을 맥·기계에 깔았다)
func ready(r string) {
	if msg := readyState(r); msg != "" {
		die("%s", msg)
	}
	if why := pullFF(r); why != "" { // 잠금 경합은 몇 번 다시(gitpull.go)
		die("git pull --ff-only 실패: %s (지난번 push 못 한 커밋이 있으면 거기서 git pull --rebase 뒤 HERDR_LAND=1 git push origin HEAD:main) — git: %s", r, why)
	}
}

// readyState: 저장소·main·깨끗함 판정 — 준비됐으면 "", 아니면 멈출 까닭
func readyState(r string) string {
	if _, rc := gitq("-C", r, "rev-parse", "--git-dir"); rc == proc.TimeoutCode {
		return "저장소 상태를 읽지 못했다 (git rev-parse 시간 넘김): " + r
	} else if rc != 0 {
		return "저장소가 없다: " + r
	}
	// 종료 0일 때만 출력을 브랜치 이름으로 읽는다 — 실패한 조회가 main을 찍었어도 버린다(R-02)
	if b, rc := gitq("-C", r, "symbolic-ref", "--short", "HEAD"); rc == proc.TimeoutCode {
		return "저장소 상태를 읽지 못했다 (git symbolic-ref 시간 넘김): " + r
	} else if rc != 0 {
		return fmt.Sprintf("저장소 브랜치를 읽지 못했다 (git symbolic-ref 종료 %d — main이 아니거나 모름): %s", rc, r)
	} else if b != "main" {
		return "저장소가 main 브랜치가 아니다: " + r
	}
	if s, rc := git("-C", r, "status", "--porcelain"); rc != 0 {
		return fmt.Sprintf("저장소 상태를 읽지 못했다 (git status 종료 %d — 모름): %s", rc, r)
	} else if s != "" {
		return "저장소에 커밋하지 않은 변경이 있다. 커밋하거나 되돌린 뒤 다시: " + r
	}
	return ""
}
