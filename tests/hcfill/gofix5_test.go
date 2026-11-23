package hcfill_test

// gofix5-fill(2026-11-23 Go 전환 재감사 4바퀴): fill/R-4M01 — 미확정 시도(?)가 실제로는 안 떴는데 그 기계에 무관한 이름 없는
// 터미널이 하나라도 있으면 그 줄이 사건 없이 끝없이 wait였다. 이름 모를 터미널은 라벨 목록(hc-lane=1)에 든 것만 후보로,
// 종료 8에서 받은 주소는 시도 기록에 남겨 다음 바퀴에 hc get으로 확정, 기다림이 이어지면 키로 한 번 사건.

import "strings"

func init() { reg(101, "r4n1", secR4n1) }

func secR4n1(k *K) {
	unsure := func(name, rc string) {
		k.fresh()
		k.set("HC_FAKE_LANE_START=box-1/" + name + "=" + rc)
		k.add(name)
		k.run("run")
		k.unset("HC_FAKE_LANE_START")
		k.check("fill/R-4M01 준비 "+name+": box-1 미확정 시도 하나", k.kStarts(name) == 1 && k.kUnsure(name, "box-1"), read(k.DATA+"/try.tsv"), k.out)
	}
	// 그 기계의 무관한 이름 없는 터미널(전체 목록에만 — 라벨 목록에 없다)
	other := `HC_FAKE_LS={"terminals":[{"address":"t0","agent":"claude-code","name":"coord","state":"live","status":"working","cwd":"/"},` +
		`{"address":"box-1/zzz","agent":"zsh","state":"live","status":"idle","cwd":"/"}]}`

	// 종료 8: 받은 주소가 hc get으로 없음 확정 — 무관한 이름 없는 터미널이 있어도 다시 띄운다
	unsure("r4n1", "8")
	k.set("HC_FAKE_GONE=box-1/r4n1")
	k.set(other)
	k.run("run")
	k.unset("HC_FAKE_GONE")
	k.lj()
	// 가짜 hc는 같은 주소에 다시 띄우니 그 바퀴 시작 확인도 GONE으로 모름 — 다시 띄움(착수 2)만 본다
	k.check("fill/R-4M01 받은 주소가 없음 확정 + 무관한 이름 없는 터미널: 다시 띄운다", k.kStarts("r4n1") == 2, k.where("r4n1"), k.out)

	// 주소를 못 받은 미확정(종료 9): 라벨 목록 밖 이름 없는 터미널은 그 시도가 아니다
	unsure("r4n1b", "9")
	k.set(other)
	k.run("run")
	k.lj()
	k.check("fill/R-4M01 주소 모름 + 라벨 목록 밖 이름 없는 터미널: 다시 띄운다", k.kStarts("r4n1b") == 2 && k.state("r4n1b") == "up", k.where("r4n1b"), k.out)

	// 받은 주소를 hc get이 모름(잘림·실패)이면 기다린다 — 모름은 상태를 안 바꾼다
	unsure("r4n1c", "8")
	k.set("HC_FAKE_GET_BAD=box-1/r4n1c")
	k.run("run")
	k.unset("HC_FAKE_GET_BAD")
	k.check("fill/R-4M01 받은 주소 hc get 모름: 착수 1·wait·미확정 그대로", k.kStarts("r4n1c") == 1 && k.state("r4n1c") == "wait" && k.kUnsure("r4n1c", "box-1"), k.where("r4n1c"), k.out)

	// 받은 주소가 hc get으로 살아 있다(두 목록엔 아직 없음): 기다리고, 여러 바퀴 이어지면 키로 한 번 사건
	unsure("r4n1d", "8")
	for i := 0; i < 4; i++ {
		k.run("run")
	}
	k.check("fill/R-4M01 살아 있는 받은 주소: 착수 1·wait, 4바퀴엔 사건 없음", k.kStarts("r4n1d") == 1 && k.state("r4n1d") == "wait" && k.nrep("r4n1d") == 0, k.where("r4n1d"), read(k.T+"/report.log"), k.out)
	k.run("run")
	k.check("fill/R-4M01 기다림 5바퀴: 키 붙은 막힘 사건", k.nrep("r4n1d") >= 1 && strings.Contains(read(k.T+"/report.log"), "--key hf:"), read(k.T+"/report.log"), k.out)
}
