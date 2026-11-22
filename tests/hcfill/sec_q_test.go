package hcfill_test

// go-alert-quiet(2026-11-22): 놀고 있음 알림은 띄울 것이 있는데 못 띄울 때(먼저 띄움 상한·겹침)만 늘 보낸다.
// 「대기열에 띄울 것 없음」은 설정 idle_empty분이 있을 때만, 그 간격에 한 번 빈 기계를 묶어서. Go 판만 고쳤다(셸 판은 늘 보냄).
//
//	사유                       idle_empty 없음/0   idle_empty 60
//	대기열에 띄울 것 없음      안 보냄             간격(60분)에 한 번, 묶어서
//	먼저 띄움 상한·겹침 대기   보냄(기존 규칙)     보냄(기존 규칙)

func init() { reg(97, "q", secQ) }

func secQ(k *K) {
	RL := k.T + "/report.log"
	nrl := func() int { return countE(RL, ".") }

	k.fresh()
	k.cpConf()
	k.confDel(`^idle_empty`)
	k.run("run")
	k.dBack(2, 660)
	k.run("run")
	k.check("조용(idle_empty 없음): 대기열에 띄울 것 없음은 idle분 넘어도 안 보냄", !nonEmpty(RL), read(RL))
	k.confAppend("idle_empty 0")
	k.dBack(2, 660)
	k.run("run")
	k.check("조용(idle_empty 0): 끔과 같다", !nonEmpty(RL), read(RL))

	k.confDel(`^idle_empty`)
	k.confAppend("idle_empty 60")
	k.run("run")
	k.check("idle_empty 60: 빈 기계들을 한 사건으로 묶어 보냄",
		nrl() == 1 && hasF(RL, "box-1 자리") && hasF(RL, "box-2 자리") && hasF(RL, "대기열에 띄울 것 없음 --name hc-fill-idle"), read(RL))
	k.lj("box-1/x1:claude-code:a", "box-1/x2:claude-code:b")
	k.run("run")
	k.lj()
	k.run("run")
	k.dBack(2, 660)
	rm(RL)
	k.run("run")
	k.check("idle_empty 60: 간격 안에 다시 빈 기계는 아직 안 보냄", !nonEmpty(RL), read(RL))
	k.dBack(3, 3700)
	k.run("run")
	k.check("idle_empty 60: 간격이 지나면 아직 안 알린 기계만", nrl() == 1 && hasF(RL, "box-1 자리") && !hasF(RL, "box-2 자리"), read(RL))

	k.fresh()
	k.cpConf()
	k.confDel(`^idle_empty`)
	k.confSed(`^early$`, "early 1")
	k.add("ca", "--after", "pa", "--only", "box-2")
	k.add("cb", "--after", "pa", "--only", "box-2")
	k.run("run")
	k.lj("box-2/ca:claude-code:ca")
	k.dBack(2, 700)
	k.run("run")
	k.check("idle_empty 없어도 먼저 띄움 상한 사유(띄울 것이 있는데 못 띄움)는 까닭과 함께 보냄",
		hasF(RL, "box-2 자리") && hasF(RL, "먼저 띄움 상한(early 1)에 걸려 남김") && !hasF(RL, "대기열에 띄울 것 없음"), read(RL))
	k.cpConf()
}
