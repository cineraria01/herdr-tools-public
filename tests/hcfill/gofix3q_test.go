package hcfill_test

// gofix3-fill-queue(2026-11-23 Go 전환 재감사 2바퀴 — 감사 HEAD 1eccd73): 고치기 전 판에서 실패하던 시험. 기대값은 지적 원문과 계약으로 정했다.

import "strings"

func init() { reg(100, "gofix3q", secGofix3Q) }

func secGofix3Q(k *K) {
	// fill/R-217: --after-any-project(<이름>@<프로젝트>)로 먼저 띄운 레인 — 선행이 그 프로젝트에 착지하면 먼저 띄움 판정(after.tsv)·ls·
	// 실행 지시문의 기다림 블록이 일반 배치와 같은 해석(이름 spec, 프로젝트 tools)으로 「들어감」을 본다
	k.fresh()
	rm(k.T + "/events")
	k.cpConf()
	k.run("add", "--name", "spec", "--project", "tools", "--brief", k.brief("spec"), "--only", "box-1")
	k.run("add", "--name", "e1", "--project", "app", "--brief", k.brief("e1"), "--after", "spec", "--after-any-project", "--only", "slow")
	k.run("run")
	k.check("fill/R-217 준비: e1을 slow에 먼저 띄움(선행 spec@tools)", k.where("e1") == "up slow slow/e1" && k.col14("e1") == "early" && k.qcol("e1", 4) == "spec@tools", read(k.Q))
	rf := k.DATA + "/runs/e1-run.md"
	write(k.t, k.T+"/commits", "aaaa1111 spec: 끝\n")
	k.run("rm", "spec", "--force") // 착지해 대기열에서 빠짐
	k.lj("slow/e1:claude-code:e1")
	rm(k.T + "/git.log")
	k.run("run")
	k.check("fill/R-217 먼저 띄움 판정: 다른 프로젝트에 착지한 선행은 go 들어감(「대기열에 없음」 stop이 아님)",
		strings.HasPrefix(k.gAfv("spec@tools"), "go 들어감(aaaa1111)") && hasF(k.T+"/git.log", "-C /Users/me/src/tools"), read(k.gAF()), read(k.T+"/git.log"))
	k.run("ls")
	k.check("fill/R-217 ls: 선행 들어감", strings.Contains(k.dEln(), "먼저 띄움(선행 들어감 — 본 작업)"), k.out)
	// 실행 지시문의 기다림 조건: 이 레인 저장소(app) 이력에는 없고, hc-fill이 써 준 after.tsv의 「들어감」으로 풀린다
	writeX(k.t, k.T+"/eg/git", "#!/bin/sh\ncase $1 in fetch) exit 0 ;; log) cat \"$T_LOG/subjects\" ;; esac\n")
	cond := dCond(rf)
	k.check("fill/R-217 실행 지시문: after.tsv에 들어감이면 기다림이 풀린다", k.dEg(cond, "base: 처음") == "yes", cond, read(k.gAF()))
	write(k.t, k.gAF(), "spec@tools\tgo\t진행 중(working)\t1\n")
	k.check("fill/R-217 실행 지시문: 아직 진행 중이면 기다림", k.dEg(cond, "base: 처음") == "no", cond)
	k.lj()
}
