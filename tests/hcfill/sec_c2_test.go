package hcfill_test

// 10c. 받은편지(셸 시험 518~840줄 뒤 절반): 취소 전달·접수 중단·sent 투영·병합기 회신·옛 줄 옮기기.

import (
	"fmt"
	"os"
	"strings"
)

func init() { reg(31, "c2", secC2) }

func secC2(k *K) {
	IS, RP, LED, RPL := k.IS(), k.RP(), k.LED(), k.RPL()

	// 취소도 전달: 결과 전 rm → 취소문, 결과 고정 뒤 rm → 결과가 감
	k.ifresh()
	k.mj("box-1", "shared", "slow")
	k.add("pre1") // 착지 안 된 선행 — inq-c는 대기로 남는다
	k.req(IS, "inq-c", "app", "pre1", "slow")
	k.run("run")
	RC := k.rid("inq-c")
	k.fill(nil, "rm", "inq-c")
	k.check("rm(결과 전): 고정본은 취소문, 장부 deliver, 대기열에서 빠짐", hasE(RPL+"/"+RC+".md", `^결과: 실패 취소 — hc-fill rm`) &&
		k.lst(RC) == "deliver" && k.where("inq-c") == "")
	k.run("run")
	k.check("rm 뒤 바퀴: 취소문이 요청자에게 감, 장부 sent", hasE(IS+"/done/inq-c.md", `^결과: 실패 취소`) && k.lst(RC) == "sent")
	k.req(IS, "inq-d", "app", "-", "slow")
	k.run("run")
	RD := k.rid("inq-d")
	write(k.t, RP+"/inq-d."+RD+".md", "결과: 고침 ddddddd\n")
	k.lj("slow/inq-d:claude-code:inq-d:done")
	k.runE([]string{"HC_FILL_CRASH_AT=after-reply-fix"}, "run")
	k.fill(nil, "rm", "inq-d")
	k.run("run")
	k.check("rm(결과 고정 뒤): 결과가 이긴다 — 요청자에 결과, 취소문 아님", cCat(IS+"/done/inq-d.md") == "결과: 고침 ddddddd" && k.lst(RD) == "sent")
	k.req(IS, "inq-e", "app", "-", "box-2") // box-2는 연결 안 됨 — 대기로 남는다
	k.run("run")
	RE := k.rid("inq-e")
	k.fill([]string{"HC_FILL_CRASH_AT=rm-after-cancel"}, "rm", "inq-e")
	k.check("중단 rm-after-cancel: 취소문은 고정, 줄은 아직 대기열에", cIsF(RPL+"/"+RE+".md") && k.cState("inq-e") == "wait")
	write(k.t, k.T+"/lane.log", "")
	k.run("run")
	k.check("그 뒤 바퀴: 결과가 고정된 대기 줄은 빼고(띄우지 않음) 취소문을 보냄", k.where("inq-e") == "" && !hasFx(k.T+"/lane.log", "inq-e") &&
		hasE(IS+"/done/inq-e.md", `^결과: 실패 취소`) && k.lst(RE) == "sent")
	k.req(IS, "inq-stop", "app", "-", "box-2")
	k.run("run")
	RS := k.rid("inq-stop")
	k.fill([]string{"HC_FILL_CRASH_AT=after-reply-fix"}, "rm", "inq-stop")
	k.mj("box-2", "shared", "slow")
	write(k.t, k.T+"/lane.log", "")
	k.run("run")
	k.check("대기 줄 취소문 고정 직후 중단: 다시 띄우지 않고 취소를 회신", k.where("inq-stop") == "" && !hasFx(k.T+"/lane.log", "inq-stop") &&
		k.lst(RS) == "sent" && hasE(IS+"/done/inq-stop.md", `^결과: 실패 취소`))
	// 빈 대기열 + idle 0 + 장부 deliver → 그 바퀴에 공개(빈 대기열 조기 종료보다 복구가 먼저)
	k.ifresh()
	k.confSed(`^idle .*`, "idle 0")
	if !hasE(k.conf(), `^idle `) {
		k.confAppend("idle 0")
	}
	k.mj("box-1", "shared", "slow")
	k.req(IS, "inq-z", "app", "-", "box-2")
	k.run("run")
	RZ := k.rid("inq-z")
	k.fill([]string{"HC_FILL_CRASH_AT=rm-after-cancel"}, "rm", "inq-z")
	k.cRewrite(k.Q, func(r []string) []string {
		if col(r, 1) == "inq-z" {
			return nil
		}
		return r
	})
	k.run("run")
	k.check("빈 대기열 + idle 0 + 장부 deliver: 그 바퀴에 공개", !nonEmpty(k.Q) && hasE(IS+"/done/inq-z.md", `^결과: 실패 취소`) && k.lst(RZ) == "sent")

	// 접수 중단: 접수 표시를 둔 뒤 죽음 → 다음 바퀴가 같은 요청 ID로 잇는다(줄 하나)
	k.ifresh()
	k.req(IS, "inq-q", "app", "-", "slow")
	k.runE([]string{"HC_FILL_CRASH_AT=inbox-after-claim"}, "run")
	CQ := cCut(read(IS+"/.hc-fill/claim/inq-q"), " ", 1)
	k.check("중단 inbox-after-claim: 접수 표시만 남고 대기열·장부 없음", CQ != "" && k.where("inq-q") == "" && cIsF(IS+"/inq-q.req"))
	k.run("run")
	k.check("그 뒤 바퀴: 같은 요청 ID로 받음(대기열 줄 하나), taken/<이름>.<ID>, 접수 표시 지움", k.rid("inq-q") == CQ && k.cQn("inq-q") == 1 &&
		cIsF(IS+"/taken/inq-q."+CQ+".req") && !exists(IS+"/.hc-fill/claim/inq-q"))
	k.ifresh()
	k.req(IS, "inq-w", "app", "-", "slow")
	k.runE([]string{"HC_FILL_CRASH_AT=inbox-after-add"}, "run")
	k.run("run")
	k.check("중단 inbox-after-add: 다음 바퀴는 옮기기만(거절·중복 줄 없음)", k.cQn("inq-w") == 1 && cIsF(IS+"/taken/inq-w."+k.rid("inq-w")+".req") &&
		!exists(IS+"/rejected"))
	// 접수 표시가 있는데 원본이 바뀌었다(새 요청) → 옛 접수가 끝날 때까지 손대지 않음
	k.ifresh()
	k.req(IS, "inq-v", "app", "-", "slow")
	k.runE([]string{"HC_FILL_CRASH_AT=inbox-after-claim"}, "run")
	write(k.t, IS+"/inq-v.brief.md", "# 바뀐 새 요청\n")
	k.run("run")
	k.check("접수 표시와 원본 해시가 다르면(새 요청) 옮기지도 받지도 않음", k.where("inq-v") == "" && cIsF(IS+"/inq-v.req") && cIsF(IS+"/inq-v.brief.md") &&
		k.cO("접수 중인 요청"))
	// .req 없이 남은 지시문은 건드리지 않음
	k.ifresh()
	write(k.t, IS+"/inq-u.brief.md", "# 새 지시문\n")
	write(k.t, IS+"/taken/inq-u.req", "project\tapp\n")
	k.run("run")
	k.check(".req 없이 남은 지시문(소속 불명)은 옮기지 않음", cIsF(IS+"/inq-u.brief.md") && !exists(IS+"/taken/inq-u.brief.md") && k.where("inq-u") == "")

	// 접수 원본은 taken에 도착했고 이전 표시를 치우기 전에 같은 이름의 새 요청이 도착했다.
	k.ifresh()
	k.req(IS, "again", "app", "-", "slow")
	k.runE([]string{"HC_FILL_CRASH_AT=inbox-after-add"}, "run")
	oi := k.rid("again")
	mkdirs(k.t, IS+"/taken")
	cMv(k, IS+"/again.brief.md", IS+"/taken/again."+oi+".brief.md")
	cMv(k, IS+"/again.req", IS+"/taken/again."+oi+".req")
	k.run("rm", "again")
	k.req(IS, "again", "app", "-", "slow")
	appendFile(k.t, IS+"/again.brief.md", "새 요청 내용\n")
	k.run("run")
	k.check("taken까지 도착한 옛 표시가 같은 이름의 새 요청을 영구 보류하지 않음", k.rid("again") != "" && k.rid("again") != oi &&
		cIsF(IS+"/taken/again."+k.rid("again")+".req"))

	// 미리보기에서 원격 접수 표시를 지우지 않는다.
	k.ifresh()
	write(k.t, IS+"/.hc-fill/claim/orphan", "orphan\n")
	k.run("run", "--dry-run")
	k.check("받은편지 dry-run은 남은 접수 표시도 보존", cIsF(IS+"/.hc-fill/claim/orphan"))
	k.run("run")
	k.check("실제 바퀴는 요청 없는 표시를 회수", !exists(IS+"/.hc-fill/claim/orphan"))

	// sent 투영이 대기열 잠금을 기다리는 사이 rm·add로 같은 이름의 새 줄이 들어왔다.
	k.ifresh()
	k.req(IS, "project-sent", "app", "-", "slow")
	k.run("run")
	ri := k.rid("project-sent")
	write(k.t, RP+"/project-sent."+ri+".md", "결과: 넘김 완료\n")
	k.brief("replacement")
	writeX(k.t, k.T+"/projection-bin/lockf", `#!/bin/sh
case " $* " in *" _set project-sent sent "*)
  "$FILL_UNDER_TEST" rm project-sent >/dev/null &&
    "$FILL_UNDER_TEST" add --name project-sent --project app --brief "$T_LOG/replacement.md" --only slow >/dev/null || exit 1 ;;
esac
exec /usr/bin/lockf "$@"
`)
	// Go 판은 lockf를 부르지 않는다 — 같은 자리(대기열 잠금 직전)의 시험 끼어들기 HC_FILL_TEST_QSET로
	k.runE([]string{"FILL_UNDER_TEST=" + k.FILL, k.cPath(k.T + "/projection-bin"), "HC_FILL_TEST_QSET=project-sent sent",
		fmt.Sprintf(`HC_FILL_TEST_QSET_CMD="%s" rm project-sent >/dev/null && "%s" add --name project-sent --project app --brief "%s/replacement.md" --only slow >/dev/null`,
			k.FILL, k.FILL, k.T)}, "run")
	k.check("옛 회신의 sent는 새 줄에 투영하지 않음", k.lst(ri) == "sent" && k.cState("project-sent") != "sent" &&
		hasF(k.DATA+"/briefs/added/project-sent.md", "# 지시문 replacement"))

	// 접수 도중 rm: 대기열은 기록됐고 원격 .req가 아직 남아 있다.
	for _, gap := range []string{"장부전", "장부후"} {
		k.ifresh()
		k.req(IS, "cutin", "app", "-", "slow")
		k.runE([]string{"HC_FILL_CRASH_AT=inbox-after-add"}, "run")
		ri = k.rid("cutin")
		if gap == "장부전" {
			write(k.t, LED, "")
		}
		k.run("rm", "cutin")
		k.run("run")
		k.check("접수 "+gap+" 중단 뒤 rm: 같은 요청을 다시 띄우지 않고 취소 회신", ri != "" && k.where("cutin") == "" && k.lst(ri) == "sent" &&
			hasF(IS+"/done/cutin.md", "결과: 실패 취소") && cIsF(IS+"/taken/cutin."+ri+".req"))
	}

	// 병합기 2단계(2차 높음 6): 병합기에 제출하고 끝난 받은편지 레인은 그 제출 ID의 병합기 결과가 나온 뒤 회신하고 결과를 끝 줄로 붙인다
	k.ifresh()
	k.mj("box-1", "shared", "slow")
	k.req(IS, "inq-m", "app", "-", "slow")
	k.run("run")
	RM := k.rid("inq-m")
	k.lj("slow/inq-m:claude-code:inq-m:done")
	write(k.t, RP+"/inq-m."+RM+".md", "고친 것\n결과: 고침 abc12345\n")
	k.runE([]string{"HC_FAKE_EVENTS_FAIL=1"}, "run")
	k.check("되돌려 주기: 사건 조회 실패 때 완성 보고를 고정하지 않음", k.lst(RM) == "open" && !exists(RPL+"/"+RM+".md"))
	write(k.t, k.T+"/events", "모름 slow\n")
	k.run("run")
	k.check("되돌려 주기: 제출자 기계 사건이 모름이면 완성 보고도 미룸", k.lst(RM) == "open" && !exists(IS+"/done/inq-m.md"))
	write(k.t, k.T+"/events", "2026-11-20T10:00:00Z\tslow\tslow/inq-m\tinq-m\t제출\tabc12345\tslow-20261120T100000-9 → refs/merge-queue/1-inq-m\t2\n")
	k.runE([]string{"HC_FAKE_MGM=box-2"}, "run")
	k.check("되돌려 주기: 제출하고 끝난 레인은 완성 보고가 있어도 병합기 결과 전에는 회신 안 함(고정도 안 함)", !exists(IS+"/done/inq-m.md") &&
		!exists(RPL+"/"+RM+".md") && k.cState("inq-m") == "up" && k.cO("병합기 결과 기다림(제출 ID slow-20261120T100000-9)"))
	appendFile(k.t, k.T+"/events", "2026-11-20T10:04:00Z\tbox-2\tbox-2/t9\tinq-m\t막힘\t-\t제출 ID slow-20261120T100000-90, 로그\t-\n")
	k.runE([]string{"HC_FAKE_MGM=box-2"}, "run")
	k.check("병합기 회신: ID 앞부분만 같은 다른 제출의 막힘은 무시", k.lst(RM) == "open" && !exists(RPL+"/"+RM+".md"))
	appendFile(k.t, k.T+"/events", "2026-11-20T10:05:00Z\tbox-2\tbox-2/t9\tinq-m\t착지\tdef4567890ab\t병합기 착지 — 제출 ID slow-20261120T100000-9 (갈래점 x)\t-\n")
	k.runE([]string{"HC_FAKE_MGM=box-2"}, "run")
	k.check("되돌려 주기: 그 제출 ID의 병합기 착지 뒤 회신, 끝 줄에 착지 sha",
		lastLine(read(IS+"/done/inq-m.md")) == "착지 def4567890ab (병합기 box-2, 제출 ID slow-20261120T100000-9)" &&
			cFirst(read(IS+"/done/inq-m.md")) == "고친 것" && k.cState("inq-m") == "sent" && k.lst(RM) == "sent",
		read(IS+"/done/inq-m.md"))

	// 제출 뒤에 중간 보고가 있어도 같은 실행의 제출 결과를 기다린다.
	k.ifresh()
	k.req(IS, "post", "app", "-", "slow")
	k.run("run")
	ri = k.rid("post")
	write(k.t, RP+"/post."+ri+".md", "결과: 고침 abc1234\n")
	write(k.t, k.T+"/events", "2026-11-20T10:00:00Z\tslow\tslow/post\tpost\t제출\tabc1234\tsub-1 → 큐\t1\n2026-11-20T10:01:00Z\tslow\tslow/post\tpost\t중간\t-\t정리\t2\n")
	k.runE([]string{"HC_FAKE_MGM=box-2"}, "run")
	k.check("제출 뒤 중간 보고가 끼어도 착지 전 회신을 보류", k.lst(ri) == "open" && !cIsF(IS+"/done/post.md"))
	appendFile(k.t, k.T+"/events", "2026-11-20T10:02:00Z\tbox-2\tbox-2/t9\tpost\t착지\tdef1234\t제출 ID sub-1\t-\n")
	k.runE([]string{"HC_FAKE_MGM=box-2"}, "run")
	k.check("제출 뒤 중간 보고가 있어도 정확한 착지로 회신", k.lst(ri) == "sent" && strings.Contains(lastLine(read(IS+"/done/post.md")), "def1234"))
	k.check("목록에 쓸 마지막 보고도 같은 제출의 착지", hasE(k.DATA+"/last.tsv", "^post\t착지\tdef1234\t"))

	// 지시문 있음 확인이 한 번 실패해도 거절하지 않음
	k.ifresh()
	k.req(IS, "inq-c", "app", "-", "slow")
	k.set("HC_FAKE_EXEC_FAIL=then echo yes")
	k.run("run")
	k.unset("HC_FAKE_EXEC_FAIL")
	k.check("받은편지: 지시문 확인 실패는 거절이 아니라 다음 바퀴에", !exists(IS+"/rejected") && cIsF(IS+"/inq-c.req") &&
		k.cO("inq-c: 지시문을 확인하지 못했다, 다음 바퀴에"))
	k.run("run")
	k.check("받은편지: 다음 바퀴에 받음", cIsF(IS+"/taken/inq-c."+k.rid("inq-c")+".req") && k.cState("inq-c") == "up")

	// 옛 줄(11칸에 #요청 ID 없음) 옮기기: 결정적 ID rl…, 떠 있던 줄은 옛 보고 경로, 옛 차례로 넣고 못 옮긴 요청은 옮기기만
	k.ifresh()
	k.req(IS, "inq-r", "app", "-", "slow")
	write(k.t, k.Q, "inq-r\tapp\t"+k.brief("inq-r")+"\t-\tslow\twait\t-\t-\t-\t2026-01-01T00:00\tslow:"+IS+"/done/inq-r.md\n")
	appendFile(k.t, k.Q, "inq-l\tapp\t"+k.brief("inq-l")+"\t-\tslow\tup\tslow\tslow/inq-l\t-\t2026-01-01T00:00\tslow:"+IS+"/done/inq-l.md\n")
	write(k.t, RP+"/inq-l.md", "결과: 고침 eeeeeee\n")
	k.lj("slow/inq-l:claude-code:inq-l:done")
	k.run("run", "--dry-run")
	k.check("옛 줄: dry-run은 옮기지 않음", !hasF(k.Q, "#"))
	k.run("run")
	RR := k.rid("inq-r")
	k.check("옛 줄 옮기기: 결정적 요청 ID rl…·장부 open, 옛 차례로 못 옮긴 요청은 그 ID로 taken/으로 옮기기만(거절·중복 줄 없음)",
		cRidOld.MatchString(RR) && cIsF(IS+"/taken/inq-r."+RR+".req") && !exists(IS+"/rejected") && k.cQn("inq-r") == 1, read(k.Q))
	k.check("옛 줄 옮기기: 이미 떠 있던 줄은 옛 보고 경로(reports/<레인>.md)로 되돌려 줌", cCat(IS+"/done/inq-l.md") == "결과: 고침 eeeeeee" &&
		k.lst(k.rid("inq-l")) == "sent", read(LED))

	// 10칸 옛 줄도 읽는다
	k.ifresh()
	k.brief("old1")
	write(k.t, k.Q, "old1\tapp\t"+k.T+"/old1.md\t-\t-\twait\t-\t-\t-\t2026-01-01T00:00\n")
	k.run("run")
	k.check("10칸 옛 줄: 띄우고 되돌려 주기 없음", k.where("old1") == "up box-1 box-1/old1" && !hasF(k.DATA+"/runs/old1-run.md", "결과 보고"))
	k.lj()
	k.run("run")
	k.check("10칸 옛 줄: 터미널이 사라져도 되돌려 주기 하지 않음", k.cState("old1") == "up" && !grep(k.out, "되돌려"))
}

// cMv: mv a b.
func cMv(k *K, a, b string) {
	if err := os.Rename(a, b); err != nil {
		k.t.Fatal(err)
	}
}

// cFirst: head -n 1.
func cFirst(s string) string {
	l := lines(s)
	if len(l) == 0 {
		return ""
	}
	return l[0]
}
