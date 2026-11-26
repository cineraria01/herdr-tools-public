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
	k.ljq("box-1/x1:claude-code:a", "box-1/x2:claude-code:b")
	k.lj("box-1/x1:claude-code:a", "box-1/x2:claude-code:b", "box-1/p:env:proxy", "box-2/z:zsh:shell", "box-2/h1:claude-code:human", "box-2/h2:codex:human2")
	k.add("l1")
	k.run("run")
	k.check("상한: box-1에 대기열이 띄운 일하는 에이전트 2개면 box-2로(다른 종류·대기열 밖 사람 세션은 세지 않음)", k.where("l1") == "up box-2 box-2/l1" &&
		in(k.out, "자리 box-2 box-2/h1 human: working, 대기열 밖 → 안 셈") && in(k.out, "자리 box-1 box-1/x1 a: working → 셈"))

	// 2b. 원격 띄우기의 창 표시 알림
	k.fresh()
	k.add("nv")
	k.runE([]string{"HC_FAKE_LANE_NOAPP=1"}, "run")
	k.check("원격 표시 알림: 띄움은 그대로, hc-lane 알림을 「알림: Herdr 창 …」 한 줄로", k.where("nv") == "up box-1 box-1/nv" && len(grepLines(k.out, "^알림: Herdr 창이 없어 box-1 ")) == 1)

	// 3. 「n개 미만일 때만」과 레인상한: shared는 상한 3·레인상한 2
	k.fresh()
	boxes := []string{"box-1/a:claude-code:a", "box-1/b:claude-code:b", "box-2/a:claude-code:a2", "box-2/b:claude-code:b2"}
	k.ljq(boxes...)
	k.lj(append(boxes, "shared/d1:claude-code:loop-x", "shared/d2:claude-code:loop-y")...)
	k.add("s1")
	k.add("s2")
	k.add("s3")
	k.run("run")
	k.check("shared: 루프 에이전트 2개는 대기열 밖이라 자리를 안 씀 — 레인상한 2까지 shared, 셋째는 slow",
		k.where("s1") == "up shared shared/s1" && k.where("s2") == "up shared shared/s2" && k.where("s3") == "up slow slow/s3")
	k.run("machine")
	k.check("기계 줄: shared 자리 밖 「대기열 밖 2」", in(k.out, "· 대기열 밖 2"), k.out)
	k.fresh()
	k.ljq(boxes...)
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
	// 자리는 대기열이 띄운 것 가운데 지금 일하는 것만(ht-seat-working-only, docs/hc-fill/slots.md)
	k.fresh()
	k.ljq("box-1/k1:claude-code:k1:done:"+W+"/app.wt/k1", "box-1/k2:claude-code:k2:idle:"+W+"/app.wt/k2")
	k.add("f1")
	k.run("run")
	k.check("자리: 대기열 쓰기 레인 done·idle(감사·착지 기다림) → 쉼 → 안 셈(box-1에 뜸)", k.where("f1") == "up box-1 box-1/f1" &&
		in(k.out, "자리 box-1 box-1/k1 k1: done, 쉼 → 안 셈") && in(k.out, "자리 box-1 box-1/k2 k2: idle, 쉼 → 안 셈"))
	k.fresh()
	k.ljq("box-1/b:claude-code:b:blocked:" + W + "/app")
	k.lj("box-1/au:claude-code:audit:done:"+W+"/app", "box-1/gone:claude-code:gone:done:"+W+"/app.wt/gone", "box-1/b:claude-code:b:blocked:"+W+"/app")
	k.add("f2")
	k.run("run")
	k.check("자리: 대기열 밖 done 터미널은 안 셈, 대기열 레인 blocked는 셈(box-1에 1개 → box-1)", k.where("f2") == "up box-1 box-1/f2" &&
		in(k.out, "자리 box-1 box-1/au audit: done, 대기열 밖 → 안 셈") && in(k.out, "자리 box-1 box-1/b b: blocked → 셈"))
	k.fresh()
	laneEnv := `{"address":"box-1/e1","agent":"env","name":"e1","state":"live","cwd":"/"}`
	k.ljq("box-1/a:claude-code:a", "box-1/e1:env:e1")
	k.set(`HC_FAKE_LS={"terminals":[{"address":"t0","agent":"claude-code","name":"coord","state":"live","status":"working","cwd":"/"},` + laneEnv +
		`,{"address":"box-1/a","agent":"claude-code","name":"a","state":"live","status":"working","cwd":"/"},{"address":"box-1/px","agent":"env","name":"proxy","state":"live","cwd":"/"}]}`)
	k.set(`HC_FAKE_LS_LANE={"terminals":[` + laneEnv + `]}`)
	k.add("f4")
	k.run("run")
	k.check("자리: 상태 없는 대기열 레인(agent env — 막 띄워 뜨는 중)도 셈, 상태 없는 대기열 밖 env는 안 셈(box-1 꽉 참 → box-2)", k.where("f4") == "up box-2 box-2/f4" && in(k.out, "자리 box-1 box-1/e1 e1: env, 상태 모름(뜨는 중) → 셈") && !in(k.out, "box-1/px"))

	// 3b-2. 끝난 레인이 자리를 붙잡지 않게
	k1k2 := []string{"box-1/k1:claude-code:k1:done:" + W + "/app.wt/k1", "box-1/k2:claude-code:k2:idle:" + W + "/app.wt/k2"}
	// 착지 여부·쉰 시간·제출·읽기 레인과 상관없이 쉬는 레인은 안 센다(옛 판정 ①~④는 걷음) — 쉬기 시작한 때(held.tsv)는 선행 판정이 쓴다
	k.fresh()
	k.ljq(k1k2...)
	now := nowS()
	k.aHeld("box-1/k1", aAgo(now, 61*60+5), "box-1/k2", aAgo(now, 10*60))
	k.add("f5")
	k.run("run")
	k.check("자리: 쉰 시간과 상관없이 쉬는 레인은 안 셈(box-1에 뜸)", k.where("f5") == "up box-1 box-1/f5" && in(k.out, "자리 box-1 box-1/k2 k2: idle, 쉼 → 안 셈"))
	var k1seen []string
	for _, r := range rows(k.HELD) {
		if col(r, 1) == "box-1/k1" {
			k1seen = append(k1seen, col(r, 2))
		}
	}
	k.check("자리 ②: held.tsv의 처음 본 시각은 그대로 둔다", strings.Join(k1seen, "\n") == aAgo(now, 61*60+5))
	// 쉬던 레인이 다시 일하면(감사 지적) 그때부터 셈
	k.fresh()
	k.ljq("box-1/k1:claude-code:k1:idle:"+W+"/app.wt/k1", "box-1/k2:claude-code:k2:idle:"+W+"/app.wt/k2")
	k.add("f6s")
	k.run("run")
	k.check("자리: 쉬는 레인 둘이면 box-1에 뜸", k.where("f6s") == "up box-1 box-1/f6s")
	k.lj("box-1/k1:claude-code:k1:working:"+W+"/app.wt/k1", "box-1/k2:claude-code:k2:idle:"+W+"/app.wt/k2", "box-1/f6s:claude-code:f6s")
	k.add("f6t")
	k.run("run")
	k.check("자리: 다시 일하는(working) 레인은 셈(k1·f6s 일함 — box-1 꽉 참 → box-2)", k.where("f6t") == "up box-2 box-2/f6t" && in(k.out, "자리 box-1 box-1/k1 k1: working → 셈"))
	k.fresh()
	k.ljq("box-1/k1:claude-code:k1:working:"+W+"/app.wt/k1", "box-1/k2:claude-code:k2:idle:"+W+"/app.wt/k2")
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
	k.ljq(k1k2...)
	now = nowS()
	k.aHeld("box-1/k1", aAgo(now, 9*60), "box-1/k2", aAgo(now, 2*60))
	k.add("fb")
	k.run("run")
	k.check("설정: hold 08·gc 09·idle 08·early_wait 09도 십진수 — 종료 0, 알림 없음", k.rc == 0 && k.where("fb") == "up box-1 box-1/fb" && !in(k.out, "알림: 설정 hold"))
	k.cpConf()
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
