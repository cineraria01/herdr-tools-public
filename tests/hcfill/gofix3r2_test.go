package hcfill_test

// gofix3-hcjson(2026-11-22 Go 전환 재감사 2바퀴 — 아스트라 6.2 max, 감사 HEAD 1eccd73): hc 응답 판독을 internal/hcjson으로 모은 뒤
// fill 지적마다 고치기 전 판에서 실패하던 시험. 기대값은 지적 원문과 계약으로 정했다.

import "strings"

func init() { reg(99, "gofix3r2", secGofix3R2) }

func secGofix3R2(k *K) {
	// fill/R-201: 안쪽 hc-lane이 시작 확인 미확정(종료 8)이면 첫 기계의 시도·주소를 남기고 둘째 기계에 또 띄우지 않는다
	k.fresh()
	k.set("HC_FAKE_LANE_START=box-1/u8=8")
	k.add("u8")
	k.run("run")
	k.unset("HC_FAKE_LANE_START")
	ll := k.T + "/lane.log"
	k.check("fill/R-201 준비: box-1에 띄우기를 시도했다", countE(ll, "^u8$") >= 1)
	k.check("fill/R-201 hc-lane 종료 8(미확정): box-2에 또 띄우지 않고 그 시도를 미확정으로 남긴다",
		countE(ll, "^box-2$") == 0 && !strings.HasPrefix(k.where("u8"), "up box-2") && grep(k.out, "모름 u8 @box-1"))
	n := countE(ll, "^u8$")
	k.lj("box-1/u8:claude-code:u8")
	k.run("run")
	k.lj()
	k.check("fill/R-201 다음 바퀴: box-1에 같은 이름 터미널이 있으면 다시 띄우지 않는다(둘째 기계에도)", countE(ll, "^u8$") == n && countE(ll, "^box-2$") == 0)

	// fill/R-201: 유효한 live 응답에서 status 칸만 빠졌으면 확인된 실패가 아니다(fill의 시작 확인)
	k.fresh()
	k.set("HC_FAKE_NOSTATUSKEY=box-1/u9")
	k.add("u9")
	k.run("run")
	k.unset("HC_FAKE_NOSTATUSKEY")
	k.check("fill/R-201 준비: box-1에 띄우기를 시도했다(status 칸 없음)", countE(ll, "^u9$") >= 1)
	k.check("fill/R-201 status 칸 없음: 다른 기계에 또 띄우지 않는다", countE(ll, "^box-2$") == 0 && !strings.HasPrefix(k.where("u9"), "up box-2") && grep(k.out, "모름 u9 @box-1"))

	// fill/R-203: 같은 주소·이름의 대기열 줄과 hc-lane=1 라벨이 있는데 agent 칸만 빠지면 — 자리를 비우지 않고 그 기계에 새 일을 놓지 않는다
	k.fresh()
	k.add("prior", "--only", "slow")
	k.run("run")
	k.check("fill/R-203 준비: 상한 1인 slow에 prior가 떴다", k.where("prior") == "up slow slow/prior")
	t0 := `{"address":"t0","agent":"claude-code","name":"coord","state":"live","status":"working","cwd":"/"}`
	pr := `{"address":"slow/prior","name":"prior","state":"live","status":"working","cwd":"/"}`
	k.set(`HC_FAKE_LS={"terminals":[`+t0+`,`+pr+`]}`, `HC_FAKE_LS_LANE={"terminals":[`+pr+`]}`)
	k.add("second", "--only", "slow")
	k.run("run")
	k.unset("HC_FAKE_LS_LANE")
	k.check("fill/R-203 agent 칸 없음: prior 자리는 그대로, second는 slow에 안 뜬다", k.where("prior") == "up slow slow/prior" && k.state("second") == "wait")
}
