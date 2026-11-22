package hcfill_test

// 셸 시험 139~341줄: 0. 도구와 설정 · 1. add·ls·rm · 2. 차례와 상한 · 2b · 3. 레인상한 · 3b·3b-2 자리 세기.

import (
	"os"
	"sort"
	"strconv"
	"strings"
)

func init() { reg(10, "a", secA) }

// aAfterIs: 줄 key 바로 다음 줄이 want인 곳이 있나(grep -Fx -A1 -- key | grep -Fxq want).
func aAfterIs(p, key, want string) bool {
	l := lines(read(p))
	for i, x := range l {
		if x == key && i+1 < len(l) && l[i+1] == want {
			return true
		}
	}
	return false
}

// aLastBlock: 마지막 「---」 줄과 그 뒤 줄들(셸의 awk 마지막 묶음).
func aLastBlock(p string) []string {
	var b []string
	for _, x := range lines(read(p)) {
		if x == "---" {
			b = nil
		}
		b = append(b, x)
	}
	return b
}

// aUniq: sort -u 결과(줄바꿈으로 이음).
func aUniq(p string) string {
	m := map[string]bool{}
	for _, x := range lines(read(p)) {
		m[x] = true
	}
	var l []string
	for x := range m {
		l = append(l, x)
	}
	sort.Strings(l)
	return strings.Join(l, "\n")
}

// aHeld: held.tsv에 「주소<TAB>시각」 줄들을 쓴다.
func (k *K) aHeld(kv ...string) {
	var b strings.Builder
	for i := 0; i+1 < len(kv); i += 2 {
		b.WriteString(kv[i] + "\t" + kv[i+1] + "\n")
	}
	write(k.t, k.HELD, b.String())
}

func aAgo(now int64, sec int64) string { return strconv.FormatInt(now-sec, 10) }

func secA(k *K) {
	in := strings.Contains
	// 0. 도구와 설정
	fi, err := os.Stat(k.FILL)
	k.check("도구가 있다", err == nil && fi.Mode().IsRegular())
	k.fresh()
	rm(k.conf())
	k.run("run")
	k.check("설정 없음: 종료 1, 설정 경로와 예시 안내", k.rc == 1 && in(k.out, k.T+"/conf/hc-fill.tsv") && in(k.out, "config.example"))
	k.cpConf()

	// 배포 목록은 hc-fill을 Go 바이너리 줄(tools:cmd/hc-fill)로 깐다 — 셸 판 포장(tools:bin/hc-fill)·셸 시험 격리 도우미(isolate.sh)는 걷혔다
	if sl := repo + "/config.example/sync.list"; exists(sl) {
		bin, old := false, false
		for _, x := range lines(read(sl)) {
			w := strings.Fields(x)
			if len(w) >= 4 && w[1] == ".local/bin/hc-fill" {
				bin = bin || w[2] == "tools:cmd/hc-fill"
				old = old || w[2] == "tools:bin/hc-fill"
			}
			old = old || (len(w) >= 3 && w[2] == "tools:tests/isolate.sh")
		}
		k.check("배포 목록: hc-fill은 바이너리 줄, 셸 판 줄 없음", bin && !old)
	}

	// 1. add · ls · rm
	k.fresh()
	k.run("add", "--name", "lane-a", "--project", "app", "--brief", k.brief("lane-a"))
	k.check("add: 종료 0, 대기열에 한 줄(대기)", k.rc == 0 && k.where("lane-a") == "wait - -")
	k.run("add", "--name", "lane-a", "--project", "app", "--brief", k.T+"/lane-a.md")
	k.check("add: 같은 이름 거절", k.rc == 1 && countE(k.Q, ".") == 1)
	k.run("add", "--name", "Bad", "--project", "app", "--brief", k.T+"/lane-a.md")
	k.check("add: 이름 글자 거절", k.rc != 0)
	k.run("add", "--name", "lane-b", "--project", "nope", "--brief", k.T+"/lane-a.md")
	k.check("add: 설정에 없는 프로젝트 거절", k.rc != 0)
	k.run("add", "--name", "lane-b", "--project", "app", "--brief", k.T+"/없음.md")
	k.check("add: 없는 지시문 거절", k.rc != 0)
	k.run("add", "--name", "lane-b", "--project", "app", "--brief", k.T+"/lane-a.md", "--only", "box-9")
	k.check("add: 설정에 없는 기계 --only 거절", k.rc != 0)
	k.run("ls")
	k.check("ls: 레인과 대기 상태", k.rc == 0 && grep(k.out, "lane-a.*대기"))
	k.run("rm", "lane-a")
	k.check("rm: 대기열에서 빠지고 아무것도 끄지 않음", k.rc == 0 && k.where("lane-a") == "" && !exists(k.T+"/hc.log"))
	k.run("rm", "lane-zz")
	k.check("rm: 없는 레인은 종료 1", k.rc == 1)

	// 2. 차례와 상한: box-1(2) → box-2(2) → shared → …
	k.fresh()
	k.add("l1")
	k.add("l2")
	k.add("l3")
	k.run("run")
	k.check("차례: 종료 0", k.rc == 0)
	k.check("차례: l1·l2는 box-1, l3은 상한 2를 넘어 box-2", k.where("l1") == "up box-1 box-1/l1" && k.where("l2") == "up box-1 box-1/l2" && k.where("l3") == "up box-2 box-2/l3")
	ll := k.T + "/lane.log"
	k.check("띄우기: hc-lane --on <기계> --name --repo <그 기계 경로> -- <설정의 launch 틀>",
		aAfterIs(ll, "--repo", "/home/me/src/app") && k.launched("example-agent") == 0 && hasFx(ll, `exec example-agent --model example-model "$1"`))
	k.check("띄우기: 원격 표시 알림이 없으면 「알림: Herdr 창」 줄 없음", !grep(k.out, "알림: Herdr 창"))
	runL1 := k.DATA + "/runs/l1-run.md"
	k.check("띄우기: run 지시문 = 지시문 + 착지 줄(그 기계 HOME 아래)", hasF(runL1, "# 지시문 l1") && hasF(runL1, "### 착지 STEPS l1 box-1"))
	k.check("띄우기: 첫 지시는 설정 task 틀로 run 경로를 담은 한 인자", hasFx(ll, runL1+" 지시문을 읽고 진행하라."))
	k.check("띄우기: run 지시문은 hc exec -i --on 으로 그 기계에", hasE(k.T+"/hc.log", "^exec -i --on box-1 "))
	k.run("run")
	k.check("다시 돌려도 떠 있는 레인을 또 띄우지 않음", countFx(ll, "--name") == 6 && !grep(k.out, "띄움"))

	k.fresh()
	k.lj("box-1/x1:claude-code:a", "box-1/x2:claude-code:b", "box-1/p:env:proxy", "box-2/z:zsh:shell")
	k.add("l1")
	k.run("run")
	k.check("상한: box-1에 에이전트 2개면 box-2로(다른 종류는 세지 않음)", k.where("l1") == "up box-2 box-2/l1")

	// 2b. 원격 띄우기의 창 표시 알림
	k.fresh()
	k.add("nv")
	k.runE([]string{"HC_FAKE_LANE_NOAPP=1"}, "run")
	k.check("원격 표시 알림: 띄움은 그대로, hc-lane 알림을 「알림: Herdr 창 …」 한 줄로", k.where("nv") == "up box-1 box-1/nv" && len(grepLines(k.out, "^알림: Herdr 창이 없어 box-1 ")) == 1)

	// 3. 「n개 미만일 때만」과 레인상한: shared는 상한 3·레인상한 2
	k.fresh()
	k.lj("box-1/a:claude-code:a", "box-1/b:claude-code:b", "box-2/a:claude-code:a2", "box-2/b:claude-code:b2", "shared/d1:claude-code:loop-x", "shared/d2:claude-code:loop-y", "shared/q:zsh:loop-q")
	k.add("s1")
	k.add("s2")
	k.run("run")
	k.check("shared: 에이전트 2개(루프 셸은 안 셈)면 1개만 더 → 다음은 slow", k.where("s1") == "up shared shared/s1" && k.where("s2") == "up slow slow/s2")
	k.fresh()
	k.lj("box-1/a:claude-code:a", "box-1/b:claude-code:b", "box-2/a:claude-code:a2", "box-2/b:claude-code:b2")
	k.add("s1")
	k.add("s2")
	k.add("s3")
	k.run("run")
	k.check("shared: 비어 있어도 레인상한 2 → 셋째는 slow", k.where("s1") == "up shared shared/s1" && k.where("s2") == "up shared shared/s2" && k.where("s3") == "up slow slow/s3")
	k.lj("box-1/a:claude-code:a", "box-1/b:claude-code:b", "box-2/a:claude-code:a2", "box-2/b:claude-code:b2", "shared/s1:claude-code:s1", "shared/s2:claude-code:s2", "slow/s3:claude-code:s3")
	k.add("s4")
	k.run("run")
	k.check("shared: 다음 바퀴에도 hc-fill 레인 2개가 살아 있으면 안 띄움 → here", k.where("s4") == "up here s4")
	k.check("here(local): --on 없이 띄우고 herdr-balance --main <설정 coord> --add", !has(aLastBlock(ll), "--on") && hasFx(k.T+"/balance.log", "--main t0 --add s4"))
	k.check("here(local): 경로는 local 기계의 repo 줄", hasFx(ll, "/Users/me/src/app"))
	k.check("레인을 띄울 때 hc-lane coord가 확인한 지휘 주소(설정 coord가 아님)를 HC_LANE_COORD로 넘김(지휘의 watch가 없으면 hc-lane이 띄운다 — 감사 [중간] 1·2차 B)", aUniq(k.T+"/coord.log") == "t8kk", read(k.T+"/coord.log"))
	k.check("레인을 띄울 때 HC_LANE_FILL=1(hc-lane이 레인 기록에 fill — 완료가 마지막 보고, 재감사 [낮음] 4)", aUniq(k.T+"/fill.log") == "1", read(k.T+"/fill.log"))

	// 3b. 자리 세기
	W := k.W
	mkdirs(k.t, W+"/app", W+"/app.wt/k1", W+"/app.wt/k2")
	k.fresh()
	k.lj("box-1/k1:claude-code:k1:done:"+W+"/app.wt/k1", "box-1/k2:claude-code:k2:idle:"+W+"/app.wt/k2")
	k.add("f1")
	k.run("run")
	k.check("자리: 쓰기 레인 done·idle + 작업 트리 있음 → 셈(box-1 꽉 참 → box-2)", k.where("f1") == "up box-2 box-2/f1" && in(k.out, "자리 box-1 box-1/k1 k1: done, 작업 트리 있음 → 셈"))
	k.fresh()
	k.lj("box-1/au:claude-code:audit:done:"+W+"/app", "box-1/gone:claude-code:gone:done:"+W+"/app.wt/gone", "box-1/b:claude-code:b:blocked:"+W+"/app")
	k.add("f2")
	k.run("run")
	k.check("자리: 읽기 전용 감사 done·작업 트리 정리된 done은 안 셈, blocked는 셈(box-1에 1개 → box-1)", k.where("f2") == "up box-1 box-1/f2" && in(k.out, "자리 box-1 box-1/au audit: done, 읽기 전용(작업 트리 아님) → 안 셈") && in(k.out, "자리 box-1 box-1/gone gone: done, 작업 트리 없음 → 안 셈"))
	k.fresh()
	k.lj("box-1/a:claude-code:a", "box-1/b:claude-code:b", "box-2/a:claude-code:a2", "box-2/b:claude-code:b2", "shared/l1:claude-code:loop-1:done:/", "shared/l2:claude-code:loop-2:done:/", "shared/l3:claude-code:loop-3:idle:/")
	k.add("f3")
	k.run("run")
	k.check("자리: all 칸 기계는 done도 다 셈(shared 3개 → slow)", k.where("f3") == "up slow slow/f3")
	k.fresh()
	laneEnv := `{"address":"box-1/e1","agent":"env","name":"audit-e1","state":"live","cwd":"/"}`
	k.set(`HC_FAKE_LS={"terminals":[{"address":"t0","agent":"claude-code","name":"coord","state":"live","status":"working","cwd":"/"},` + laneEnv +
		`,{"address":"box-1/a","agent":"claude-code","name":"a","state":"live","status":"working","cwd":"/"},{"address":"box-1/px","agent":"env","name":"proxy","state":"live","cwd":"/"}]}`)
	k.set(`HC_FAKE_LS_LANE={"terminals":[` + laneEnv + `]}`)
	k.add("f4")
	k.run("run")
	k.check("자리: 상태 없는 hc-lane 레인(agent env)도 셈, 라벨 없는 env는 안 셈(box-1 꽉 참 → box-2)", k.where("f4") == "up box-2 box-2/f4" && in(k.out, "자리 box-1 box-1/e1 audit-e1: env, 상태 모름(hc-lane 레인) → 셈") && !in(k.out, "box-1/px"))

	// 3b-2. 끝난 레인이 자리를 붙잡지 않게
	k1k2 := []string{"box-1/k1:claude-code:k1:done:" + W + "/app.wt/k1", "box-1/k2:claude-code:k2:idle:" + W + "/app.wt/k2"}
	k.fresh()
	k.lj(k1k2...)
	write(k.t, k.T+"/commits", "9999ffff fix(x): k1-extra 다른 레인\naaaa1111 feat(x): k1 끝\n")
	k.add("f4")
	k.run("run")
	k.check("자리 ①: done + 작업 트리 + 착지 커밋 → 안 셈(box-1에 1개 → box-1에 뜸)", k.where("f4") == "up box-1 box-1/f4" && in(k.out, "자리 box-1 box-1/k1 k1: done, 착지함(aaaa1111) → 안 셈") && in(k.out, "자리 box-1 box-1/k2 k2: idle, 작업 트리 있음 → 셈"))
	k.check("자리 ①: fetch는 바퀴에 한 번(자리 판정과 선행 판정이 같이 씀)", countF(k.T+"/git.log", "fetch -q origin") == 1)
	k.fresh()
	k.lj(k1k2...)
	write(k.t, k.T+"/commits", "aaaa1111 k1: 끝\n")
	k.add("f4b")
	k.runE([]string{"HC_FAKE_FETCH_FAIL=1"}, "run")
	k.check("자리 ①: fetch 실패면 판정 못 함 → 셈(box-1 꽉 참 → box-2)", k.where("f4b") == "up box-2 box-2/f4b" && in(k.out, "자리 box-1 box-1/k1 k1: done, 작업 트리 있음 → 셈"))
	k.fresh()
	k.lj(k1k2...)
	now := nowS()
	k.aHeld("box-1/k1", aAgo(now, 61*60+5), "box-1/k2", aAgo(now, 10*60))
	k.add("f5")
	k.run("run")
	k.check("자리 ②: 착지 없음 + 쉰 지 hold(60분) 넘음 → 안 셈, hold 안 → 셈(box-1에 뜸)", k.where("f5") == "up box-1 box-1/f5" && in(k.out, "자리 box-1 box-1/k1 k1: done, 쉰 지 61분(hold 60) → 안 셈") && in(k.out, "자리 box-1 box-1/k2 k2: idle, 작업 트리 있음 → 셈"))
	var k1seen []string
	for _, r := range rows(k.HELD) {
		if col(r, 1) == "box-1/k1" {
			k1seen = append(k1seen, col(r, 2))
		}
	}
	k.check("자리 ②: held.tsv의 처음 본 시각은 그대로 둔다", strings.Join(k1seen, "\n") == aAgo(now, 61*60+5))
	k.fresh()
	k.lj(k1k2...)
	now = nowS()
	k.aHeld("box-1/k1", aAgo(now, 30*60), "box-1/k2", aAgo(now, 10*60))
	k.add("f6")
	k.run("run")
	k.check("자리 ③: 쉰 지 hold 안(30분) → 셈(box-1 꽉 참 → box-2)", k.where("f6") == "up box-2 box-2/f6")
	// 자리 ③-2: 마지막 사건이 「제출」인 done·idle 레인은 hold 유예 없이 안 셈
	k.fresh()
	k.add("k1")
	k.run("run")
	k.lj("box-1/k1:claude-code:k1:idle:"+W+"/app.wt/k1", "box-1/k2:claude-code:k2:idle:"+W+"/app.wt/k2")
	write(k.t, k.T+"/events", "2026-11-21T10:00:00Z\tbox-1\tbox-1/k1\tk1\t제출\tabc12345\tsub-1 → refs/merge-queue/1-k1\t3\n")
	k.add("f6s")
	k.run("run")
	k.check("자리 ③-2: 제출 뒤 idle 레인은 쉰 지 hold 안이어도 안 셈(box-1에 뜸), 제출 없는 레인은 셈", k.where("f6s") == "up box-1 box-1/f6s" && in(k.out, "자리 box-1 box-1/k1 k1: idle, 제출함(병합기 큐 — 레인은 끝) → 안 셈") && in(k.out, "자리 box-1 box-1/k2 k2: idle, 작업 트리 있음 → 셈"))
	k.lj("box-1/k1:claude-code:k1:working:"+W+"/app.wt/k1", "box-1/k2:claude-code:k2:idle:"+W+"/app.wt/k2", "box-1/f6s:claude-code:f6s")
	k.add("f6t")
	k.run("run")
	k.check("자리 ③-2: 제출 뒤 다시 일하는(working) 레인은 셈(box-1 꽉 참 → box-2)", k.where("f6t") == "up box-2 box-2/f6t")
	k.fresh()
	k.confSed(`^hold .*`, "hold     0")
	k.lj(k1k2...)
	now = nowS()
	k.aHeld("box-1/k1", aAgo(now, 600*60), "box-1/k2", aAgo(now, 600*60))
	k.add("f7")
	k.run("run")
	k.check("자리 ④: hold 0이면 끔 — 오래 쉬어도 셈(box-2)", k.where("f7") == "up box-2 box-2/f7" && hasE(k.conf(), "^hold *0$"))
	k.cpConf()
	k.fresh()
	k.lj("box-1/k1:claude-code:k1:working:"+W+"/app.wt/k1", "box-1/k2:claude-code:k2:idle:"+W+"/app.wt/k2")
	now = nowS()
	k.aHeld("box-1/k1", aAgo(now, 90*60), "box-1/gone", aAgo(now, 90*60))
	k.add("f8")
	k.run("run")
	k.check("자리 ⑤: working으로 바뀌거나 사라진 주소는 held.tsv에서 빠지고, 새로 쉰 주소는 들어감", !hasE(k.HELD, "^box-1/k1\t") && !hasE(k.HELD, "^box-1/gone\t") && hasE(k.HELD, "^box-1/k2\t[0-9][0-9]*$"), read(k.HELD))
	k.fresh()
	k.lj("box-1/k1:claude-code:k1:done:" + W + "/app.wt/k1")
	k.add("f9")
	k.run("run", "--dry-run")
	k.check("자리: dry-run은 held.tsv를 쓰지 않음", !exists(k.HELD))
	// 감사 [낮음] 2: 0으로 시작하는 분 값(08·09)도 십진수로
	k.fresh()
	k.confSed(`^hold .*`, "hold     08")
	k.confSed(`^gc .*`, "gc       09")
	k.confSed(`^idle .*`, "idle     08")
	k.confSed(`^early_wait .*`, "early_wait 09")
	k.lj(k1k2...)
	now = nowS()
	k.aHeld("box-1/k1", aAgo(now, 9*60), "box-1/k2", aAgo(now, 2*60))
	k.add("fb")
	k.run("run")
	k.check("설정: hold 08·gc 09·idle 08·early_wait 09도 십진수 — 종료 0, 쉰 지 9분(hold 8) → 안 셈", k.rc == 0 && k.where("fb") == "up box-1 box-1/fb" && in(k.out, "쉰 지 9분(hold 8) → 안 셈"))
	k.cpConf()
	// 감사 [낮음] 4: 레인 기록 종류 read면 안 셈
	k.fresh()
	k.lj("box-1/k1:claude-code:audit-k1:done:"+W+"/app.wt/k1", "box-1/k2:claude-code:k2:idle:"+W+"/app.wt/k2")
	write(k.t, k.LT, "box-1/k1\told-lane\tt0\nbox-1/k1\taudit-k1\tt0\tread\nbox-1/k2\tk2\tt0\tfill\n")
	k.add("fc")
	k.run("run")
	k.check("자리: 레인 기록 종류 read(작업 트리 cwd 감사) → 읽기 레인 → 안 셈, fill은 지금처럼 셈(box-1에 뜸)", k.where("fc") == "up box-1 box-1/fc" && in(k.out, "자리 box-1 box-1/k1 audit-k1: done, 읽기 레인(레인 기록) → 안 셈") && in(k.out, "자리 box-1 box-1/k2 k2: idle, 작업 트리 있음 → 셈"))
	rm(k.LT)
	// 감사 [낮음] 1: 일찍 끝나는 바퀴는 held.tsv를 비운다. dry-run은 그대로
	k.fresh()
	k.confSed(`^idle .*`, "idle     0")
	write(k.t, k.HELD, "box-1/k1\t1\n")
	k.run("run", "--dry-run")
	k.check("일찍 끝나는 바퀴: dry-run은 held.tsv를 그대로 둠", k.rc == 0 && hasE(k.HELD, "^box-1/k1\t1$"))
	k.run("run")
	fi, err = os.Stat(k.HELD)
	k.check("일찍 끝나는 바퀴(빈 대기열 + idle 0): held.tsv를 비움", k.rc == 0 && err == nil && fi.Mode().IsRegular() && !nonEmpty(k.HELD), read(k.HELD))
	k.cpConf()
	k.cpConf()
	k.confAppend("hold     5")
	k.fresh()
	k.add("fa")
	k.run("run")
	k.check("설정: hold 줄 두 번이면 종료 1", k.rc == 1 && grep(k.out, "hold 줄이 두 번"))
	k.cpConf()
}
