package hcfill_test

// gofix4-fill(2026-11-23 Go 전환 재감사 3바퀴, 감사 HEAD 7cccace): fill/R-202 — 미확정 시도(?)를 「안 떴다」로 확정하기 전에
// 같은 바퀴의 라벨 목록과 이름 모를 터미널도 본다. 기대값은 지적 원문(착수 1·wait).

func init() { reg(100, "gofix4", secGofix4) }

func secGofix4(k *K) {
	// 미확정 시도 하나 만들기: box-1에서 hc-lane 종료 8(시작 확인 미확정)
	unsure := func(name string) {
		k.fresh()
		k.set("HC_FAKE_LANE_START=box-1/" + name + "=8")
		k.add(name)
		k.run("run")
		k.unset("HC_FAKE_LANE_START")
		k.check("fill/R-202 준비 "+name+": box-1 미확정 시도 하나", k.kStarts(name) == 1 && k.kUnsure(name, "box-1"), read(k.DATA+"/try.tsv"), k.out)
	}

	// 전체 목록에는 없고 라벨 목록(뒤에 읽음)에만 있다 — 목록 사이에 보인 그 터미널: 안 떴다고 확정하지 않는다
	unsure("sample")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/sample","name":"sample","state":"live","cwd":"/"}]}`)
	k.run("run")
	k.unset("HC_FAKE_LS_LANE")
	k.check("fill/R-202 라벨 목록에만 있는 같은 이름 터미널: 다른 기계에 또 띄우지 않고 미확정 그대로",
		k.kStarts("sample") == 1 && k.state("sample") == "wait" && k.kUnsure("sample", "box-1"), k.where("sample"), read(k.DATA+"/try.tsv"), k.out)

	// 라벨 목록 항목에 이름이 없어도(이름 모름) 그 기계의 그 터미널일 수 있다
	unsure("sample2")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/sample2","state":"live","cwd":"/"}]}`)
	k.run("run")
	k.unset("HC_FAKE_LS_LANE")
	k.check("fill/R-202 라벨 목록에만 있는 이름 없는 터미널: 착수 1·wait",
		k.kStarts("sample2") == 1 && k.state("sample2") == "wait" && k.kUnsure("sample2", "box-1"), k.where("sample2"), k.out)

	// 전체 목록에 그 기계 터미널이 있는데 이름 칸만 빠졌다(모름 항목) — 같은 것
	unsure("sample3")
	k.set(`HC_FAKE_LS={"terminals":[{"address":"t0","agent":"claude-code","name":"coord","state":"live","status":"working","cwd":"/"},{"address":"box-1/sample3","agent":"claude-code","state":"live","status":"working","cwd":"/"}]}`)
	k.run("run")
	k.lj()
	k.check("fill/R-202 이름 없는 전체 목록 항목: 착수 1·wait",
		k.kStarts("sample3") == 1 && k.state("sample3") == "wait" && k.kUnsure("sample3", "box-1"), k.where("sample3"), k.out)

	// 대조: 연결된 box-1에 그 터미널이 정말 없으면(두 목록 다 없음) 안 떴다고 확정하고 다시 띄운다
	unsure("sample4")
	k.run("run")
	k.check("fill/R-202 대조: 두 목록 다 없으면 다시 띄운다", k.kStarts("sample4") == 2 && k.state("sample4") == "up", k.where("sample4"), k.out)
}
