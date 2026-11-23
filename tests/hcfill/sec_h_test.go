package hcfill_test

// 셸 시험 10y: 기존 결함(숫자 꼴 이름·주소 재사용·끊긴 기계·띄우는 동안 바뀐 줄·감사 줄 다시 넣기·강제 종료 복구)과 주차.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func init() { reg(80, "h", secH) }

// hSetQ: 대기열에서 그 이름 줄의 칸들을 바꿔 쓴다(awk -v OFS='\t' '$1==n {$c = v} 1').
func (k *K) hSetQ(name string, cv map[int]string) {
	var out []string
	for _, r := range rows(k.Q) {
		if col(r, 1) == name {
			for c, v := range cv {
				for len(r) < c {
					r = append(r, "")
				}
				r[c-1] = v
			}
		}
		out = append(out, strings.Join(r, "\t"))
	}
	write(k.t, k.Q, strings.Join(out, "\n")+"\n")
}

// hCut: 파일의 c칸을 줄마다(cut -f c).
func hCut(p string, c int) string {
	var l []string
	for _, r := range rows(p) {
		l = append(l, col(r, c))
	}
	return strings.Join(l, "\n")
}

func secH(k *K) {
	W, Q := k.W, k.Q
	mkdirs(k.t, W+"/app", W+"/app.wt/k1", W+"/app.wt/k2")
	// 숫자 꼴 이름: awk의 ==는 양쪽이 숫자처럼 보이면 숫자로 견준다(10 == 010 == 1e1) — 이름은 글자로
	k.fresh()
	k.add("010", "--after", "zz9", "--no-early")
	k.run("add", "--name", "10", "--project", "app", "--brief", k.brief("10"))
	k.check("숫자 이름: 010이 있어도 10을 넣음(글자 비교)", k.rc == 0 && countE(Q, ".") == 2, read(Q))
	k.run("run")
	k.check("숫자 이름: 10을 띄워도 010 줄은 대기 그대로", k.where("010") == "wait - -" && k.state("10") == "up", read(Q))
	k.run("rm", "1e1")
	k.check("숫자 이름: rm 1e1은 없는 레인(10·010과 다름)", k.rc == 1 && k.where("010") == "wait - -" && k.where("10") != "")
	k.run("rm", "10")
	k.check("숫자 이름: rm 10은 010을 남김", k.rc == 0 && k.where("010") == "wait - -" && k.where("10") == "", read(Q))

	// 자리·레인 기록은 주소+이름으로(주소는 재사용된다): lanes.tsv의 옛 read 줄이 같은 주소의 새 쓰기 레인을 안 세게
	k.fresh()
	k.lj("box-1/k1:claude-code:k1:done:"+W+"/app.wt/k1", "box-1/k2:claude-code:k2:idle:"+W+"/app.wt/k2")
	write(k.t, k.LT, "box-1/k1\taudit-old\tt0\tread\nbox-1/k2\tk2\tt0\tfill\n")
	k.add("fd")
	k.run("run")
	k.check("자리: 레인 기록 read가 같은 주소의 다른 이름이면(주소 재사용) 읽기 레인으로 안 봄 → 셈(box-1 가득 → box-2)",
		k.where("fd") == "up box-2 box-2/fd" && strings.Contains(k.out, "자리 box-1 box-1/k1 k1: done, 작업 트리 있음 → 셈"))
	rm(k.LT)
	k.fresh()
	k.lj("box-1/a:claude-code:a", "box-1/b:claude-code:b", "box-2/a:claude-code:a2", "box-2/b:claude-code:b2")
	k.add("s1")
	k.add("s2")
	k.run("run")
	k.lj("box-1/a:claude-code:a", "box-1/b:claude-code:b", "box-2/a:claude-code:a2", "box-2/b:claude-code:b2", "shared/s1:claude-code:other", "shared/s2:claude-code:s2")
	k.add("s3")
	k.run("run")
	k.check("레인상한: 대기열 주소에 다른 이름 터미널(주소 재사용)은 그 레인으로 안 셈 → shared에 셋째", k.where("s3") == "up shared shared/s3")
	k.fresh()
	k.confSed(`^early$`, "early 1")
	k.add("ea", "--after", "pa", "--only", "box-1")
	k.add("eb", "--after", "pa", "--only", "box-1")
	k.run("run")
	k.lj("box-1/ea:claude-code:other")
	k.run("run")
	k.check("먼저 띄움 상한: 그 주소가 다른 이름이면(주소 재사용) 살아 있는 먼저 띄움으로 안 셈 → eb 띄움", k.where("eb") == "up box-1 box-1/eb")
	k.cpConf()

	// 「이미 떠 있음」은 hc ls에 있는 기계만 본다 — 끊긴 기계에서 그 이름이 돌던 것은 끝났는지 모른다: 띄우지 않음
	k.fresh()
	k.add("hold1", "--after", "zz9", "--no-early") // 대기열이 비어 바퀴가 일찍 끝나지 않게
	k.lj("box-1/k9:claude-code:dx:working:/home/me/src/app.wt/dx")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/k9","state":"live","cwd":"/"}]}`)
	k.run("run")
	k.add("dx")
	k.mj("box-2", "shared", "slow")
	k.lj()
	k.unset("HC_FAKE_LS_LANE")
	k.run("run")
	k.check("끊긴 기계에 지난 바퀴 같은 이름 터미널이 있었으면 띄우지 않음(모름)", k.where("dx") == "wait - -" && grep(k.out, `dx.*끊긴 기계 box-1`))
	k.mj("box-1", "box-2", "shared", "slow")
	k.run("run")
	k.check("다시 붙어 그 이름이 없으면 띄움", k.state("dx") == "up")

	// run이 대기열을 읽은 뒤(띄우는 동안) 그 레인이 rm·다시 add되면 새 줄을 옛 실행 주소로 덮지 않는다
	k.fresh()
	k.add("rr")
	k.set("HC_FAKE_LANE_HOOK='" + k.FILL + "' rm rr; '" + k.FILL + "' add --name rr --project app --brief '" + k.brief("rr2") + "'")
	k.run("run")
	k.unset("HC_FAKE_LANE_HOOK")
	k.check("띄우는 동안 rm·다시 add된 줄은 옛 실행으로 up이 되지 않음(새 줄 그대로 대기)",
		k.where("rr") == "wait - -" && hasF(k.qcol("rr", 3), "# 지시문 rr2"), read(Q))
	k.check("줄 ID: add마다 17칸에 새 값(같은 이름 다시 add해도 다름)", k.qcol("rr", 17) != "" && grep(k.out, `줄 바뀜 rr`), read(Q))
	k.lj("box-1/rr:claude-code:rr:working:/home/me/src/app.wt/rr")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/rr","state":"live","cwd":"/"}]}`)
	k.run("run")
	k.check("다음 바퀴에도 교체된 줄은 이전 시도의 터미널을 채택하지 않음", k.where("rr") == "wait - -")
	k.lj()
	k.unset("HC_FAKE_LS_LANE")
	k.run("run")
	k.check("옛 실행 정리 뒤 새 줄의 지시문으로 띄움", k.where("rr") == "up box-1 box-1/rr" && hasF(k.DATA+"/runs/rr-run.md", "# 지시문 rr2"))
	k.run("rm", "rr")
	k.add("rr")
	k.lj("box-1/rr:claude-code:rr:working:/home/me/src/app.wt/rr")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/rr","state":"live","cwd":"/"}]}`)
	k.run("run")
	k.check("띄우기를 마친 뒤 rm·add해도 옛 주소를 새 줄에 채택하지 않음", k.where("rr") == "wait - -")
	k.unset("HC_FAKE_LS_LANE")
	k.fresh()
	k.add("tidy")
	k.run("run")
	k.run("rm", "tidy")
	k.run("run")
	k.check("여러 기계가 연결되면 끝난 시도만 정리", !nonEmpty(k.DATA+"/try.tsv") && !grep(k.out, `awk:`))

	// 감사 줄은 차수마다 새 이름이라 바꿔 넣지 않는다 — 같은 이름을 다시 넣으면 거절하고 옛 줄의 남긴것은 그대로
	k.fresh()
	write(k.t, Q, "audit-ax\tapp\t/b\t-\tbox-1\twait\t-\t-\tbox-1=box-1/audit-ax\t2026-11-20T00:00\t-\t-\turgent\tno-early\taudit=x\n")
	rc := k.fill(nil, "_add", "audit-ax", "app", "/b2", "-", "box-1", "wait", "-", "-", "-", "2026-11-20T00:01", "-", "-", "urgent", "no-early", "audit=x")
	k.check("감사 줄 다시 넣기: 같은 이름은 거절, 옛 줄의 남긴것 그대로",
		rc != 0 && hCut(Q, 9) == "box-1=box-1/audit-ax" && hCut(Q, 3) == "/b" && countE(Q, ".") == 1, read(Q))

	// 바퀴가 강제 종료되어도 다음 바퀴 뒤에 임시 지시문이 남지 않는다
	k.fresh()
	k.add("snap-cut", "--only", "box-1")
	snap := k.T + "/snapshot-tmp"
	mkdirs(k.t, snap)
	k.runE([]string{"TMPDIR=" + snap, "HC_FILL_CRASH_AT=after-launch"}, "run")
	k.lj("box-1/snap-cut:claude-code:snap-cut:working:/home/me/src/app.wt/snap-cut")
	k.runE([]string{`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/snap-cut","state":"live","cwd":"/"}]}`, "TMPDIR=" + snap}, "run")
	left := false
	filepath.WalkDir(snap, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == "snap-cut.md" {
			left = true
		}
		return nil
	})
	k.check("띄운 뒤 강제 종료하고 복구하면 임시 지시문도 회수",
		hasFx(k.DATA+"/crash.log", "after-launch") && k.state("snap-cut") == "up" && !left && !exists(k.DATA+"/.run-brief.tmp"))

	// 띄운 뒤 up을 적기 전에 run이 죽으면 다음 바퀴 채택이 먼저 띄움 표시를 잃고, 이번 실행이 쓴 받은편지 결과를 .old-로 치웠다
	k.fresh()
	k.add("ck", "--after", "pz", "--only", "box-1")
	k.hSetQ("ck", map[int]string{11: "box-2:/tmp/x/done/ck.md"}) // 받은편지 줄로
	k.runE([]string{"HC_FILL_CRASH_AT=after-launch"}, "run")
	try := k.DATA + "/try.tsv"
	k.check("준비: 띄운 뒤 up 전에 run이 죽음 — 줄은 대기, 시도 기록이 남음", k.where("ck") == "wait - -" && hasE(try, "^ck\t"), read(Q))
	write(k.t, k.DATA+"/reports/ck.md", "결과: 고침 abc1234\n") // 그 실행이 쓴 결과
	k.lj("box-1/ck:claude-code:ck:working:/home/me/src/app.wt/ck")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/ck","state":"live","cwd":"/"}]}`)
	k.run("run")
	k.unset("HC_FAKE_LS_LANE")
	k.check("복구: 채택이 시도 기록의 먼저 띄움 표시(early)를 쓰고 실행 주소를 확정",
		k.where("ck") == "up box-1 box-1/ck" && k.col14("ck") == "early" && hCut(try, 6) == "box-1/ck", read(Q))
	var names []string
	if es, err := os.ReadDir(k.DATA + "/reports"); err == nil {
		for _, e := range es {
			names = append(names, e.Name())
		}
	}
	k.check("복구: 같은 실행의 받은편지 보고는 .old-로 치우지 않음",
		exists(k.DATA+"/reports/ck.md") && !grep(strings.Join(names, "\n"), `ck.md.old-`), strings.Join(names, " "))

	// 같은 주소와 이름이었던 지난 실행의 주차·제출 사건을 새 실행에 붙이지 않는다
	k.fresh()
	k.add("same-park", "--after", "prep", "--only", "box-1")
	k.run("run")
	k.lj("box-1/same-park:claude-code:same-park:working:/home/me/src/app.wt/same-park")
	write(k.t, k.T+"/events", "2026-11-20T01:00:00Z\tm\tbox-1/same-park\tsame-park\t주차\tabc1234\t옛 주차\t1\t-\tsame-park@1600000000000\n")
	k.run("run")
	k.check("옛 실행의 주차는 같은 주소·이름인 새 실행을 주차하지 않음", k.where("same-park") == "up box-1 box-1/same-park")
	k.ifresh()
	k.req(k.IS(), "same-in", "app", "-", "slow")
	k.run("run")
	ri := k.rid("same-in")
	write(k.t, k.T+"/events", "2026-11-20T01:00:00Z\tm\tslow/same-in\tsame-in\t제출\tabc1234\tslow-20261120T010000-1 → 큐\t1\t-\tsame-in@1600000000000\n")
	write(k.t, k.RP()+"/same-in."+ri+".md", "결과: 넘김 현재 요청\n")
	k.runE([]string{"HC_FAKE_GONE=slow/same-in"}, "run")
	a, b := k.RP()+"/same-in."+ri+".md", k.IS()+"/done/same-in.md"
	k.check("터미널이 사라져도 새 요청을 옛 실행의 병합 결과로 막지 않음", k.lst(ri) == "sent" && exists(a) && exists(b) && read(a) == read(b))
	k.cpConf()

	secHPark(k)
}

// secHPark: 주차 — 먼저 띄운 레인은 준비 뒤 hc-lane park로 가지를 원격에 두고 끝낸다 → hc-fill이 줄을 주차로 되돌려 자리를 비우고 →
// 선행이 들어가면 hc-lane --resume으로 그 가지에서 다시 띄운다.
func secHPark(k *K) {
	Q := k.Q
	lane := k.T + "/lane.log"
	k.fresh()
	rm(k.T + "/events")
	k.cpConf()
	k.confAppend("audit "+k.EXA, "audit_final 최종 확인")
	k.add("pk", "--after", "pz", "--only", "box-1")
	k.run("run")
	rf := k.DATA + "/runs/pk-run.md"
	k.check("먼저 띄움 절: 준비 뒤 hc-lane park → clean → 턴 끝(기다리지 않음), 기다림 블록은 원격 없을 때만",
		k.where("pk") == "up box-1 box-1/pk" && hasF(rf, "hc-lane park") && hasF(rf, "park가 종료 3(원격 없는 저장소)일 때만"), read(rf))
	k.lj("box-1/pk:claude-code:pk:working:/home/me/src/app.wt/pk")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/pk","state":"live","cwd":"/"}]}`)
	write(k.t, k.T+"/events", "2026-11-20T01:00:00Z\tm\tbox-1/pk\tpk\t주차\tabcdef12\tlane/pk 준비\t1\n")
	k.run("run")
	k.check("주차 사건: 원래 터미널이 아직 정리 중이어도 parked를 유지",
		k.where("pk") == "wait - -" && k.col14("pk") == "parked" && strings.Contains(k.out, "주차 pk") && !grep(k.out, `이미 떠 있음 pk`), read(Q))
	write(k.t, k.T+"/commits", "aaaa1111 pz: 끝\n")
	write(k.t, lane, "")
	k.run("run")
	k.check("선행이 들어가도 원래 주차 터미널이 정리 중이면 이어 띄우기를 미룸", k.where("pk") == "wait - -" && !nonEmpty(lane))
	write(k.t, k.T+"/commits", "")
	k.lj("box-1/pk:claude-code:pk:idle:/home/me/src/app.wt/pk")
	k.run("ls")
	k.check("ls: 주차(선행 …)", len(fgrepLines(strings.Join(grepLines(k.out, `^pk `), "\n"), "주차(선행 pz")) > 0)
	k.run("run")
	k.check("주차 줄은 선행 전에 다시 먼저 띄우지 않음", k.where("pk") == "wait - -")
	rm(lane)
	write(k.t, k.T+"/commits", "aaaa1111 pz: 끝\n")
	k.lj()
	k.unset("HC_FAKE_LS_LANE")
	k.run("run")
	k.check("선행 착지 뒤: hc-lane --resume으로 다시 띄우고 「이어 가기」 절(먼저 띄움 절 없음), 먼저 칸 resumed",
		k.where("pk") == "up box-1 box-1/pk" && hasFx(lane, "--resume") && hasF(rf, "## 이어 가기") && !hasF(rf, "## 먼저 띄움") && k.col14("pk") == "resumed", read(rf))
	k.check("이어 띄운 작업 지시문에도 자동 감사와 최종 감사 조건이 있음", hasF(rf, "## 감사·착지 (hc-fill 자동)") && hasF(rf, "최종 감사(최종 확인)"))
	k.cpConf()
	k.lj("box-1/pk:claude-code:pk:working:/home/me/src/app.wt/pk")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/pk","state":"live","cwd":"/"}]}`)
	appendFile(k.t, k.T+"/events", "2026-11-20T03:00:00Z\tm\tbox-1/pk\tpk\t중간\tabcdef12\t이어 가는 중\t1\n")
	k.run("run")
	k.check("resumed 줄: 이어 띄운 터미널(마지막 사건 주차 아님)은 지금처럼 떠 있음 그대로", k.where("pk") == "up box-1 box-1/pk" && k.col14("pk") == "resumed", read(Q))

	// fill-park-readopt: 주차를 적은 터미널이 아직 차례를 마치는 중(working)이면 그 터미널을 다시 잡아 up+parked로 되돌렸다
	k.fresh()
	rm(k.T + "/events")
	k.add("pq", "--after", "pz", "--only", "box-1")
	k.run("run")
	k.lj("box-1/pq:claude-code:pq:working:/home/me/src/app.wt/pq")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/pq","state":"live","cwd":"/"}]}`)
	write(k.t, k.T+"/events", "2026-11-20T01:00:00Z\tm\tbox-1/pq\tpq\t주차\tabcdef12\tlane/pq 준비\t1\n")
	k.run("run")
	k.check("주차 사건 + 그 터미널이 아직 working: 채택하지 않고 대기·parked",
		k.where("pq") == "wait - -" && k.col14("pq") == "parked" && !grep(k.out, `이미 떠 있음 pq`), read(Q))
	rm(k.T + "/events.log")
	k.run("run")
	k.check("다음 바퀴도(working 그대로): 채택 안 함", k.where("pq") == "wait - -" && !grep(k.out, `이미 떠 있음 pq`))
	// 이미 망가진 줄(up+parked, 주차를 적은 터미널은 hc-sweep이 닫으며 「정리됨」을 덧붙임)은 run 바퀴가 스스로 대기·parked로 되돌린다
	rid := k.qcol("pq", 17)
	k.hSetQ("pq", map[int]string{6: "up", 7: "box-1", 8: "box-1/pq"})
	k.lj()
	k.unset("HC_FAKE_LS_LANE")
	appendFile(k.t, k.T+"/events", "2026-11-20T01:30:00Z\tm\tbox-1/pq\tpq\t정리됨\t-\thc-sweep 쉰 지 30분\t2\n")
	k.run("run", "--dry-run")
	k.check("dry-run: up+parked 복구할 줄을 보이고 대기열은 그대로",
		k.where("pq") == "up box-1 box-1/pq" && strings.Contains(k.out, "주차로 되돌릴 pq (box-1/pq) (주차 뒤 다시 잡힌 줄 복구)"), read(Q))
	k.run("run")
	k.check("up+parked·터미널 사라짐·마지막 주차: 대기·parked로 복구(줄 ID 그대로), 선행 전이라 안 띄움",
		k.where("pq") == "wait - -" && k.col14("pq")+" "+k.qcol("pq", 17) == "parked "+rid && strings.Contains(k.out, "주차 pq"), read(Q))
	rm(lane)
	write(k.t, k.T+"/commits", "aaaa1111 pz: 끝\n")
	k.run("run")
	k.check("복구한 줄: 선행 착지 뒤 --resume으로 이어 띄움", k.where("pq") == "up box-1 box-1/pq" && hasFx(lane, "--resume") && k.col14("pq") == "resumed", read(Q))
	// up+parked이고 그 터미널이 살아 있어도(working) 마지막 사건이 주차면 되돌리고 채택하지 않는다
	write(k.t, k.T+"/commits", "")
	k.hSetQ("pq", map[int]string{14: "parked"})
	k.lj("box-1/pq:claude-code:pq:working:/home/me/src/app.wt/pq")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/pq","state":"live","cwd":"/"}]}`)
	k.run("run")
	k.check("up+parked·터미널 working·마지막 주차: 대기·parked, 채택 안 함",
		k.where("pq") == "wait - -" && k.col14("pq") == "parked" && !grep(k.out, `이미 떠 있음 pq`), read(Q))
}
