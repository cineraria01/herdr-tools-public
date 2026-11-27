package hclane_test

// 27. 이어 띄우기(--revive, 2026-11-28 ht-lane-revive): 기계가 꺼져 레인 터미널만 죽고 작업 트리·가지·커밋 안 된 변경·대화 기록은
// 남았을 때, 같은 작업 트리에 새 터미널만 띄운다. 작업 트리를 만들지도 손대지도 않고, 임대는 새로 잡지 않고 이어 쓴다.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"herdr-tools/internal/clitest"
)

func init() { reg(94, "s27-revive", s27Revive) }

// s27proj: Claude가 대화 기록을 두는 폴더 이름(영숫자 밖의 글자마다 「-」 — 실측 2026-11-28)
func s27proj(p string) string {
	var b strings.Builder
	for _, r := range p {
		switch {
		case r < 0x80 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r > 0xFFFF:
			b.WriteString("--")
		default:
			b.WriteString("-")
		}
	}
	return b.String()
}

// s27snap: 작업 트리의 커밋 안 된 변경 전부(상태·추적 변경·스테이징·추적 안 된 파일 바이트)
func s27snap(k *K, wt string) string {
	s := k.git(wt, "status", "--porcelain=v1", "-uall") + "\n" + k.git(wt, "diff") + "\n" + k.git(wt, "diff", "--cached")
	for _, f := range []string{"half.txt", "a", "b"} {
		s += "\n" + f + ":" + read(filepath.Join(wt, f))
	}
	return s
}

func s27Revive(k *K) {
	k.mk("t27")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.seed(".claude/herdr-setup.sh", `echo setup >>"$HOME/setup.log"`)
	bf := k.T + "/rv.md"
	write(k.t, bf, "# 시험 지시문\n")
	run := func(extra []string, args ...string) {
		k.runE(extra, append(append([]string{"--brief", bf}, args...), "--", "claude", "{brief} 를 읽고 진행하라")...)
	}
	rv := func(extra []string, name string, args ...string) {
		run(extra, append([]string{"--revive", "--name", name, "--repo", k.R}, args...)...)
	}
	hn, _ := k.exe(k.T, "", nil, "hostname", "-s")

	// ① 작업 트리·가지 있음 + 터미널 없음 → 띄움(만들기·훅·풀기 건너뜀), 변경·임대 그대로
	run(nil, "--name", "rv-a", "--repo", k.R)
	wt := k.R + ".wt/rv-a"
	la := s22lref(k, "rv-a")
	k.check("준비: 보통 띄우기(설정 훅 한 번, 임대)", k.rc == 0 && la != "" && read(k.T+"/setup.log") == "setup\n")
	write(k.t, wt+"/a", "반쯤 고침\n")
	write(k.t, wt+"/half.txt", "반쯤 쓴\x00파일")
	write(k.t, wt+"/b", "스테이징\n")
	k.git(wt, "add", "--", "b")
	before := s27snap(k, wt)
	r0 := k.runs()
	rv(nil, "rv-a")
	k.check("① --revive: 같은 작업 트리에 hc run 한 번(--cwd 작업 트리·라벨 hc-lane=1), 임대 값 그대로 HC_LANE_LEASE, 설정 훅 안 돎",
		k.rc == 0 && k.runs() == r0+1 && k.pair("--cwd", wt) && k.pair("--label", "hc-lane=1") && k.has("HC_LANE_LEASE="+la) &&
			s22lref(k, "rv-a") == la && read(k.T+"/setup.log") == "setup\n")
	k.check("① --revive: 커밋 안 된 변경(추적·스테이징·추적 안 된 파일)이 바이트까지 같음", s27snap(k, wt) == before)
	rb := read(k.T + "/rv-run.md")
	k.check("① --revive: 지시문 사본에 원래 지시문 + 착지 절차 + 「재시동 뒤 이어 가기」 절, 대화 기록이 없으면 이어 가기 표시 없음",
		strings.HasPrefix(rb, "# 시험 지시문\n") && strings.Contains(rb, "### 착지·정리") && strings.Contains(rb, "### 재시동 뒤 이어 가기") &&
			strings.Contains(rb, "git status") && strings.Contains(k.out, `"worktree":"`+wt+`"`) && !strings.Contains(read(k.HCLOG), "HC_LANE_CONTINUE"))

	// ⑥ 대화 기록: 같은 폴더에는 감사(--read) 대화도 있다 — 첫 사용자 글이 이 레인 지시문 사본으로 시작하는 세션만 잇는다
	pd := k.T + "/.claude/projects/" + s27proj(wt) + "/"
	user := func(c string) string { return `{"type":"user","message":{"role":"user","content":"` + c + `"}}` + "\n" }
	write(k.t, pd+"aud-1.jsonl", user(k.T+"/audit-run.md 지시문을 읽고 진행하라. 대상 "+k.T+"/rv-run.md"))
	write(k.t, pd+"aud-2.jsonl", `{"type":"summary"}`+"\n"+user("다른 지시")+`{"type":"user","message":{"content":"`+k.T+`/rv-run.md"}}`+"\n")
	os.Remove(k.HCLOG)
	rv(nil, "rv-a")
	k.check("⑥ --revive: 같은 폴더의 기록이 감사 대화뿐이면(첫 글이 이 레인 사본이 아님) 이어 가기 표시 없음",
		k.rc == 0 && !strings.Contains(read(k.HCLOG), "HC_LANE_CONTINUE"))
	write(k.t, pd+"lane-1.jsonl", `{"type":"summary"}`+"\n"+user(k.T+"/rv-run.md 지시문을 읽고 진행하라. 너는 worker다."))
	write(k.t, pd+"lane-0.jsonl", user(k.T+"/rv-run.md 지시문을 읽고 진행하라."))
	old := time.Now().Add(-time.Hour)
	for i, f := range []string{"lane-0", "lane-1", "aud-1", "aud-2"} { // 감사 대화가 가장 새것
		tm := old.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(pd+f+".jsonl", tm, tm); err != nil {
			k.t.Fatal(err)
		}
	}
	os.Remove(k.HCLOG)
	rv(nil, "rv-a")
	cont := "HC_LANE_CONTINUE=" + "--resume=" + "lane-1" // 이어 쓴 한 덩어리는 공개 검사의 긴 토큰 꼴에 걸린다
	k.check("⑥ --revive: 레인 자신의 가장 새 세션을 골라 레인 환경 HC_LANE_CONTINUE=--resume=<세션 id>(더 새 감사 대화가 있어도)",
		k.rc == 0 && k.has(cont) && s27snap(k, wt) == before)
	// 실행 명령 틀(zsh·sh)이 따옴표 없는 $HC_LANE_CONTINUE를 펼친 꼴: 한 인자(zsh는 변수를 낱말로 나누지 않는다 — 감사 atp68z5 중간 1)
	for _, sh := range []string{"zsh", "sh"} {
		if _, err := exec.LookPath(sh); err != nil {
			continue
		}
		o, _ := k.exe(k.T, "", []string{cont}, sh, "-c", `printf '[%s]' $HC_LANE_CONTINUE --model x "$1"`, "_", "task")
		k.check("⑥ "+sh+" 틀에서 $HC_LANE_CONTINUE는 인자 하나 --resume=<id>", o == "[--resume=lane-1][--model][x][task]")
	}
	// ⑥ 대기열(hc-fill revive): 사본을 스스로 두고 그 경로로 첫 지시를 준 레인 — --brief 대신 --continue-from <그 경로>로 고른다
	qrun := k.T + "/runs/rv-a-run.md"
	write(k.t, pd+"lane-q.jsonl", user(qrun+" 지시문을 읽고 진행하라. 너는 worker다."))
	os.Remove(k.HCLOG)
	k.runE(nil, "--revive", "--continue-from", qrun, "--name", "rv-a", "--repo", k.R, "--", "claude", qrun+" 지시문을 읽고 진행하라.")
	k.check("⑥ --revive --continue-from: 첫 글이 그 사본인 대화를 골라 HC_LANE_CONTINUE=--resume=<id>, 사본을 새로 두지 않음",
		k.rc == 0 && k.has("HC_LANE_CONTINUE="+"--resume="+"lane-q") && s27snap(k, wt) == before)
	k.run("--continue-from", qrun, "--name", "rv-a", "--repo", k.R, "--", "claude")
	k.check("⑥ --continue-from은 --revive와만", k.rc == 1 && strings.Contains(k.errf(), "--continue-from"))

	// 쌓아 띄운 레인: 작업 트리가 이미 있어 선행 제출을 다시 확인하지 않는다(시작점을 쓰지 않음), 범위 환경은 그대로 넘긴다
	st := "HC_LANE_STACK=rv-p@" + strings.Repeat("0", 40)
	rv([]string{st}, "rv-a")
	k.check("--revive: HC_LANE_STACK이 있어도 선행 제출을 받지 않고 띄움, 레인 환경에 그대로", k.rc == 0 && k.has(st))

	// 띄우기가 거절돼도 작업 트리를 지우지 않는다(보통 띄우기의 되돌리기 — 고유 커밋 0이면 worktree remove --force)
	rv([]string{"HC_FAKE_RUN_FAIL=1"}, "rv-a")
	k.check("--revive: hc run이 거절해도 작업 트리·변경·가지·임대 그대로(되돌리기 안 함)",
		k.rc == 1 && isDir(wt) && s27snap(k, wt) == before && k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/rv-a") && s22lref(k, "rv-a") == la &&
			strings.Contains(k.errf(), "그대로 둠"))

	// ④ 살아 있는 세션·목록 못 읽음 → 띄우지 않음
	ls := func(cwds ...string) string {
		var ts []string
		for i, c := range cwds {
			ts = append(ts, fmt.Sprintf(`{"address":"t%d","agent":"claude-code","cwd":%q,"state":"live"}`, 70+i, c))
		}
		return `HC_FAKE_LS={"machines":[{"name":"box","state":"connected"}],"terminals":[` + strings.Join(ts, ",") + `]}`
	}
	r0 = k.runs()
	rv([]string{ls(wt + "/sub")}, "rv-a")
	k.check("④ --revive: 그 작업 트리(아래)를 쓰는 살아 있는 터미널이 있으면 종료 10, hc run 안 부름", k.rc == 10 && k.runs() == r0 && strings.Contains(k.errf(), "t70"))
	rv([]string{ls(clitest.AltSpelling(k.t, wt, false))}, "rv-a")
	k.check("④ --revive: 같은 폴더의 다른 철자 cwd도 쓰는 중(종료 10)", k.rc == 10 && k.runs() == r0)
	rv([]string{"HC_FAKE_LS_FAIL=1"}, "rv-a")
	k.check("④ --revive: hc ls를 못 읽으면 종료 1, 안 띄움", k.rc == 1 && k.runs() == r0)
	rv([]string{ls(`C:\lanes\rv-a`, "/elsewhere")}, "rv-a")
	k.check("④ --revive: Windows 꼴 cwd·다른 폴더만 섞인 목록이면 띄움", k.rc == 0 && k.runs() == r0+1)
	rv([]string{ls(wt)}, "rv-a") // 두 번째 실행: 앞 실행이 띄운 터미널이 그 작업 트리에 산다
	k.check("두 번째 --revive: 「살아 있는 세션」으로 종료 10", k.rc == 10 && k.runs() == r0+1 && strings.Contains(k.errf(), "이미 돎"))

	// ② 작업 트리 없음 · ③ 가지가 다름 → 거절(11), 진행 중인 rebase는 그대로 잇는다
	r0 = k.runs()
	rv(nil, "rv-none")
	k.check("② --revive: 작업 트리가 없으면 종료 11(입력 거절), 아무것도 안 만듦", k.rc == 11 && k.runs() == r0 && !exists(k.R+".wt/rv-none") &&
		!k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/rv-none") && strings.Contains(k.errf(), "작업 트리"))
	run(nil, "--name", "rv-b", "--repo", k.R)
	k.git(k.R+".wt/rv-b", "switch", "-q", "-c", "other")
	r0 = k.runs()
	rv(nil, "rv-b")
	k.check("③ --revive: 작업 트리의 가지가 레인 가지가 아니면 종료 11", k.rc == 11 && k.runs() == r0 && strings.Contains(k.errf(), "other"))
	run(nil, "--name", "rv-c", "--repo", k.R)
	wc := k.R + ".wt/rv-c"
	s22commit(k, wc, "b", "레인 쪽", "rv-c: b")
	k.seedp("b", "기준 쪽")
	k.sh(wc, `git fetch -q origin && ! git rebase -q origin/main >/dev/null 2>&1`)
	rbd := k.git(wc, "rev-parse", "--git-path", "rebase-merge")
	if !filepath.IsAbs(rbd) {
		rbd = filepath.Join(wc, rbd)
	}
	cb := read(wc + "/b")
	rv(nil, "rv-c")
	k.check("--revive: 진행 중인 rebase(HEAD 떨어짐)도 그 가지로 보고 띄움, rebase·충돌 파일 그대로",
		k.rc == 0 && isDir(rbd) && read(wc+"/b") == cb && strings.Contains(cb, "<<<<<<<"))

	// ⑤ 임대 주인 다름·임대 없음 → 거절, ⑦ --on
	run(nil, "--on", "box", "--name", "rv-o", "--repo", k.R)
	lo := s22lref(k, "rv-o")
	r0 = k.runs()
	rv(nil, "rv-o")
	k.check("⑤ --revive: 임대 주인이 다른 기계(box)면 종료 10, 안 띄움, 임대 그대로", k.rc == 10 && k.runs() == r0 && s22lref(k, "rv-o") == lo && strings.Contains(k.errf(), "box"))
	rv(nil, "rv-o", "--on", "box")
	k.check("⑦ --revive --on: 그 기계에서 같은 임대를 이어 씀(hc run --on box)", k.rc == 0 && k.runs() == r0+1 && k.pair("--on", "box") && k.has("HC_LANE_LEASE="+lo) && s22lref(k, "rv-o") == lo)
	run(nil, "--name", "rv-n", "--repo", k.R)
	k.exe(k.T, "", nil, k.LANE, "lease", "drop", "--repo", k.R, "--name", "rv-n")
	r0 = k.runs()
	rv(nil, "rv-n")
	k.check("⑤ --revive: 임대가 없으면(착지·clean·손으로 놓음) 종료 11, 안 띄움, 임대 새로 안 잡음", k.rc == 11 && k.runs() == r0 && s22lref(k, "rv-n") == "" && strings.Contains(k.errf(), "임대"))

	// 옛 판이 만든 작업 트리(upstream·임대 주인 표식·레인 기록 없음)와 손으로 만든 임대
	k.git(k.R, "worktree", "add", "-q", "-b", "rv-old", k.R+".wt/rv-old", "origin/main")
	k.sh(k.R, `c=$(git commit-tree -m "hc-lane lease rv-old $1 2026-01-01T00:00:00Z" "$(git hash-object -t tree -w /dev/null)") && git push -q origin "$c:refs/hc-lanes/rv-old"`, hn)
	lold := s22lref(k, "rv-old")
	rv(nil, "rv-old")
	k.check("옛 판 작업 트리: upstream·표식 없어도 가지·임대가 맞으면 띄움", k.rc == 0 && k.has("HC_LANE_LEASE="+lold) && s22lref(k, "rv-old") == lold)

	// 바퀴 순서: 이어 띄운 뒤 gc → 그 터미널이 쓰니 남김 / gc가 먼저 지움 → 이어 띄우기는 「작업 트리 없음」
	run(nil, "--name", "rv-g", "--repo", k.R)
	wg := k.R + ".wt/rv-g"
	rv(nil, "rv-g")
	k.runE([]string{ls(wg)}, "gc", "--repo", k.R, "--min-age", "0")
	k.check("바퀴: 이어 띄운 터미널이 쓰는 작업 트리는 gc가 남김", k.rc == 0 && isDir(wg))
	k.run("gc", "--repo", k.R, "--min-age", "0")
	gone := !exists(wg)
	r0 = k.runs()
	rv(nil, "rv-g")
	k.check("바퀴: gc가 먼저 지운 작업 트리는 이어 띄우지 않음(종료 11)", gone && k.rc == 11 && k.runs() == r0)

	// ⑧ 인자: --read·--carry·--resume과 함께, --brief 없이, 띄우기 밖 → 거절
	for _, a := range [][]string{{"--read"}, {"--carry", k.T + "/x.tar"}, {"--resume"}} {
		r0 = k.runs()
		rv(nil, "rv-a", a...)
		k.check("⑧ --revive "+a[0]+": 인자 거절(종료 1), hc run 안 부름", k.rc == 1 && k.runs() == r0 && strings.Contains(k.errf(), "--revive"))
	}
	k.run("--revive", "--name", "rv-a", "--repo", k.R, "--", "claude")
	k.check("⑧ --revive는 --brief가 있어야(재시동 절을 실을 곳)", k.rc == 1 && strings.Contains(k.errf(), "--brief"))
	k.run("clean", "--revive", "--repo", k.R, "--branch", "rv-a")
	k.check("⑧ --revive는 띄울 때만", k.rc == 1 && isDir(wt))

	// 모의 실행: 할 일만
	rv(nil, "rv-a", "--dry-run")
	k.check("--revive --dry-run: 작업 트리 만들기·임대 잡기 줄 없이 「이어 띄움」 줄",
		k.rc == 0 && strings.Contains(k.out, "[dry-run] 이어 띄움") && !strings.Contains(k.out, "worktree add") && !strings.Contains(k.out, "임대 잡기"))
}
