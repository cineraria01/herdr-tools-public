package hcfill_test

// 10h. 자동 감사 뒷부분(셸 시험 「# 넘기기 멱등성」 ~ 「# 10h+. 최종 감사」 앞): 넘기기 멱등성·중단 이어 가기·판정 없음·
// 완료 sha·옛 기록·옛 틀·진짜 hc-lane report·verdict·감사 대상 아닌 줄·이름 규칙.

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

func init() { reg(51, "e2", secE2) }

// eOpen: afresh → 레인 하나(box-1) 띄우고 done 터미널·완료 사건 → 차수 ID.
func (k *K) eOpen(name, sha, msg string) string {
	k.afresh()
	k.add(name, "--only", "box-1")
	k.run("run")
	k.lj("box-1/" + name + ":claude-code:" + name + ":done:/")
	k.ev("box-1/"+name, name, "완료", sha, msg)
	k.run("run")
	return k.aidof(name)
}

// eAudDone: 레인 done·감사 done 터미널.
func (k *K) eAudDone(name, id string) {
	k.lj("box-1/"+name+":claude-code:"+name+":done:/", "box-1/audit-"+id+":claude-code:audit-"+id+":done:/")
}

// eSed1: 줄마다 처음 맞는 하나만 바꾼다(sed s/re/repl/ — g 없음).
func eSed1(s, re, repl string) string {
	r := regexp.MustCompile(re)
	l := strings.Split(s, "\n")
	for i, x := range l {
		if m := r.FindStringIndex(x); m != nil {
			l[i] = x[:m[0]] + repl + x[m[1]:]
		}
	}
	return strings.Join(l, "\n")
}

func secE2(k *K) {
	send, rl := k.T+"/send.log", k.T+"/report.log"
	vd := func(lane, id, ext string) string { return k.VD() + "/" + lane + "/" + id + ext }

	// 넘기기 멱등성: 깨우기 실패·판정 파일 쓰기 전후 중단·다른 내용·수거 전 새 완료·수거와 취소 경합·수거 기다림 상한
	D1 := k.eOpen("d1", "3030aaa", "끝")
	k.verd("box-1/audit-"+D1, D1, "0", "0", "0")
	k.eAudDone("d1", D1)
	k.runE([]string{"HC_FAKE_SEND_FAIL=box-1/d1"}, "run")
	k.check("깨우기 hc send 실패: 판정 파일은 있음(영속 지시), 상태 deliver, 「깨우기 실패」",
		exists(vd("d1", D1, ".md")) && k.acol(D1, 8) == "deliver" && strings.Contains(k.out, "깨우기 실패 d1 box-1/d1"))
	k.run("run")
	k.check("다음 바퀴에 다시 깨움(지난번 깨움이 실패라 기한 없음)", countF(send, "hc-lane verdict "+D1) == 2 && k.acol(D1, 11) == "1")
	D2 := k.eOpen("d2", "3131bbb", "끝")
	k.verd("box-1/audit-"+D2, D2, "0", "0", "0")
	k.eAudDone("d2", D2)
	k.runE([]string{"HC_FILL_CRASH_AT=after-consume"}, "run")
	k.check("중단 after-consume: 판정을 기록(deliver)한 뒤 판정 파일 전에 죽음",
		hasFx(k.DATA+"/crash.log", "after-consume") && k.acol(D2, 8) == "deliver" && !exists(vd("d2", D2, ".md")))
	k.run("run")
	k.check("다음 바퀴: 기록에서 판정 파일을 만들고 깨움(한 번)", k.vkey("d2", "verdict", D2) == "pass" && countF(send, "hc-lane verdict "+D2) == 1)
	D3 := k.eOpen("d3", "3232ccc", "끝")
	k.verd("box-1/audit-"+D3, D3, "0", "0", "0")
	k.eAudDone("d3", D3)
	k.runE([]string{"HC_FILL_CRASH_AT=after-verdict-file"}, "run")
	i0 := ino(vd("d3", D3, ".md"))
	k.run("run")
	k.check("중단 after-verdict-file: 다음 바퀴는 같은 내용(ALREADY) — 파일 그대로, 깨움 한 번",
		ino(vd("d3", D3, ".md")) == i0 && countF(send, "hc-lane verdict "+D3) == 1 && k.acol(D3, 8) == "deliver")
	D4 := k.eOpen("d4", "3333ddd", "끝")
	write(k.t, vd("d4", D4, ".md"), "aid\t"+D4+"\nverdict\tfix\n")
	k.verd("box-1/audit-"+D4, D4, "0", "0", "0")
	k.eAudDone("d4", D4)
	k.run("run")
	k.check("판정 파일 자리에 다른 내용 → 덮지 않고 지휘에(hf:<ID>:conflict), 깨우지 않음",
		hasF(vd("d4", D4, ".md"), "verdict\tfix") && k.nrep("hf:"+D4+":conflict") == 1 && !exists(send))
	D5 := k.eOpen("d5", "3434eee", "끝")
	k.verd("box-1/audit-"+D5, D5, "0", "1", "0")
	k.eAudDone("d5", D5)
	k.run("run")
	k.ev("box-1/d5", "d5", "완료", "3535fff", "수거 전에 새 커밋")
	k.run("run")
	D5b := k.aidof("d5")
	k.check("수거 전 새 완료 → <ID>.ack에 void를 먼저 잡고 그 차수는 superseded, 새 차수",
		eCut1(vd("d5", D5, ".ack")) == "void" && k.acol(D5, 8) == "superseded" && D5b != D5 && k.acol(D5b, 5) == s40("3535fff"), read(k.AT()))
	D6 := k.eOpen("d6", "3636aaa", "끝")
	k.verd("box-1/audit-"+D6, D6, "0", "1", "0")
	k.eAudDone("d6", D6)
	k.run("run")
	write(k.t, vd("d6", D6, ".ack"), "taken\tx\td6@1700000000000\n")
	k.ev("box-1/d6", "d6", "완료", "3737bbb", "수거 뒤 고침")
	k.run("run")
	k.check("레인이 먼저 수거(taken)했으면 void는 못 잡고 그 차수는 taken(지적이 전달됨), 새 완료는 새 차수",
		eCut1(vd("d6", D6, ".ack")) == "taken" && k.acol(D6, 8) == "taken" && k.aidof("d6") != D6, read(k.AT()))
	// ack_max 0: eOpen의 afresh 뒤, add 전에 설정에 더한다(셸과 같은 차례)
	k.afresh()
	k.confAppend("ack_max 0")
	k.add("d7", "--only", "box-1")
	k.run("run")
	k.lj("box-1/d7:claude-code:d7:done:/")
	k.ev("box-1/d7", "d7", "완료", "3838ccc", "끝")
	k.run("run")
	D7 := k.aidof("d7")
	k.verd("box-1/audit-"+D7, D7, "0", "0", "0")
	k.eAudDone("d7", D7)
	k.run("run")
	k.run("run")
	k.check("ack_max(연결된 동안 센 수거 기다림) 넘으면 지휘에 한 번(hf:<ID>:noack), 기다림은 계속(deliver)",
		k.nrep("hf:"+D7+":noack") == 1 && k.acol(D7, 8) == "deliver", read(rl))
	// ht-verdict-stale-alert(+lows-1126-ht 감사 atp2nmt-bisrf4 낮음 1~3): 레인이 통과 뒤 새 커밋을 만들어 검증 중이면
	// (레인 터미널 working, 그 터미널 cwd의 HEAD ≠ 판정 sha) 기한이 지나도 막힘 대신 상태 줄 — 같은 sha에 한 번
	lw := "/elsewhere/lane" // 짐작 경로(<repo>.wt/<레인>)가 아닌 레인 터미널 cwd — 가짜 git은 HEAD를 이 폴더에서만 답한다
	dlv := func(n, sha string) (string, func(st string)) {
		k.afresh()
		k.confAppend("ack_max 0")
		k.add(n, "--only", "box-1")
		k.run("run")
		k.lj("box-1/" + n + ":claude-code:" + n + ":done:/")
		k.ev("box-1/"+n, n, "완료", sha, "끝")
		k.run("run")
		id := k.aidof(n)
		k.verd("box-1/audit-"+id, id, "0", "0", "0")
		return id, func(st string) {
			k.lj("box-1/"+n+":claude-code:"+n+":"+st+":"+lw, "box-1/audit-"+id+":claude-code:audit-"+id+":done:/")
		}
	}
	hd := func(h string) []string { return []string{"HC_FAKE_HEAD=" + s40(h), "HC_FAKE_HEAD_CWD=" + lw} }
	nl := "검증 중 — 다시 감사 기다림"
	D7b, st7b := dlv("d7b", "3838eee")
	st7b("working")
	all := ""
	for i := 0; i < 3; i++ {
		k.runE(hd("3939fff"), "run")
		all += k.out
	}
	k.check("옛 판정 + 레인 새 커밋(working, 터미널 cwd의 HEAD 다름) → noack 없음, 상태 줄 「새 커밋 … 검증 중」은 같은 sha에 한 번",
		k.nrep("hf:"+D7b+":noack") == 0 && k.acol(D7b, 8) == "deliver" && strings.Count(all, "새 커밋 3939fff0 "+nl) == 1 && strings.Count(all, nl) == 1, all, read(rl))
	k.runE(hd("4040aaa"), "run")
	k.check("HEAD가 또 바뀌면 상태 줄을 다시 한 번",
		k.nrep("hf:"+D7b+":noack") == 0 && strings.Contains(k.out, "새 커밋 4040aaa0 "+nl), k.out, read(rl))
	k.runE(hd("3838eee"), "run")
	k.check("같은 커밋(HEAD = 판정 sha) → 지금처럼 noack 한 번",
		k.nrep("hf:"+D7b+":noack") == 1, k.out, read(rl))
	D7c, st7c := dlv("d7c", "3838fff")
	st7c("idle")
	k.runE(hd("3939aaa"), "run")
	k.runE(hd("3939aaa"), "run")
	k.check("새 커밋이 있어도 레인 터미널이 쉬면(새 커밋 뒤 보고 없이 멈춤) 지금처럼 noack 한 번, 상태 줄 없음",
		k.nrep("hf:"+D7c+":noack") == 1 && !strings.Contains(k.out, nl), k.out, read(rl))
	D8 := k.eOpen("d8", "3939ddd", "끝")
	k.verd("box-1/audit-"+D8, D8, "0", "0", "0")
	k.lj("box-1/d8:claude-code:other:done:/", "box-1/audit-"+D8+":claude-code:audit-"+D8+":done:/")
	k.run("run")
	k.check("레인 주소에 다른 이름 터미널이 서 있으면 깨우지 않음(「깨우기 미룸」), 판정 파일은 둠",
		!exists(send) && exists(vd("d8", D8, ".md")) && strings.Contains(k.out, "깨우기 미룸 audit-"+D8))
	// fill-park-readopt: 통과 판정을 수거 없이 그 감사 sha 그대로 병합기에 제출·착지하면 차수가 deliver로 남아 막힘이 났다
	D9 := k.eOpen("d9", "4040aaa", "끝")
	k.verd("box-1/audit-"+D9, D9, "0", "0", "0")
	k.eAudDone("d9", D9)
	k.run("run")
	k.ev("box-1/d9", "d9", "제출", s40("4040aaa"), "box-1-x → refs/merge-queue/main/d9 (병합기 box-2 가 main 에 넣는다)")
	k.lj("box-1/audit-" + D9 + ":claude-code:audit-" + D9 + ":done:/")
	k.runE([]string{"HC_FAKE_GONE=box-1/d9"}, "run")
	k.check("통과 판정을 수거 없이 감사 sha로 제출(레인 터미널 닫힘) → 차수 taken, noack·parent-ended 막힘 없음",
		k.acol(D9, 8) == "taken" && k.nrep("hf:"+D9+":noack") == 0 && k.nrep("hf:"+D9+":parent-ended") == 0 && strings.Contains(k.out, "수거 없이 제출·착지"), read(k.AT()), read(rl))
	DA := k.eOpen("da", "4141bbb", "끝")
	k.verd("box-1/audit-"+DA, DA, "0", "0", "0")
	k.eAudDone("da", DA)
	k.run("run")
	k.ev("box-1/da", "da", "착지", "4141bbb0", "손으로 land")
	k.run("run")
	k.confAppend("ack_max 0")
	k.run("run")
	k.check("통과 판정을 수거 없이 감사 sha로 착지(sha8) → 차수 taken, 기한이 지나도 noack 없음",
		k.acol(DA, 8) == "taken" && k.nrep("hf:"+DA+":noack") == 0, read(k.AT()), read(rl))

	// 한 줄 전체가 판정 형식과 맞아야 한다. 잘린 줄과 상반된 숫자에서 통과를 만들지 않는다.
	for _, wording := range []string{"높음 0·중간 0", "높음 1·중간 0·낮음 0 (예시 높음 0)"} {
		ai := k.eOpen("verdict", "abcd001", "끝")
		k.ev("box-1/audit-"+ai, "audit-"+ai, "판정", k.acol(ai, 5), "감사 완료 "+ai+" "+wording)
		k.run("run")
		k.check("모호한 판정은 공개하지 않고 지휘로 보냄("+wording+")", !exists(vd("verdict", ai, ".md")) && k.nrep("hf:"+ai+":unreadable") == 1)
	}

	// 소비 의도만 남긴 뒤 부모가 새 커밋을 완료하면 옛 통과 파일을 공개하지 않는다.
	ai := k.eOpen("newer", "abcd001", "끝")
	k.verd("box-1/audit-"+ai, ai, "0", "0")
	k.runE([]string{"HC_FILL_CRASH_AT=after-consume"}, "run")
	k.ev("box-1/newer", "newer", "완료", "abcd002", "다음")
	k.runE([]string{"HC_FILL_CRASH_AT=after-verdict-file"}, "run")
	k.check("재시작 때 부모 완료가 바뀌면 옛 통과 파일을 한순간도 공개하지 않음", k.acol(ai, 8) == "superseded" && !exists(vd("newer", ai, ".md")))

	// 차수 열기 중단: 지시문·기록·대기열 사이에서 죽어도 다음 바퀴가 같은 ID·같은 확정 지시문으로 감사 줄 하나
	for _, cp := range []string{"after-brief", "after-opening", "after-put"} {
		k.afresh()
		k.add("o1", "--only", "box-1")
		k.run("run")
		k.lj("box-1/o1:claude-code:o1:done:/")
		k.ev("box-1/o1", "o1", "완료", "4040aaa", "끝")
		k.runE([]string{"HC_FILL_CRASH_AT=" + cp}, "run")
		oa := k.aidof("o1")
		ob := strings.TrimRight(read(k.ABR()+"/"+oa+".md"), "\n")
		k.run("run")
		k.check("중단 "+cp+" 뒤: 감사 줄 하나(같은 ID), 같은 확정 지시문, queued",
			k.eQ15("audit=o1") == 1 && oa != "" && k.aidof("o1") == oa && eMd(k.ABR()) == 1 && (cp == "after-opening" || (ob != "" && strings.TrimRight(read(k.ABR()+"/"+oa+".md"), "\n") == ob)) && k.acol(k.aidof("o1"), 8) == "queued", read(k.Q), read(k.AT()))
	}
	// 지휘 넘김 중단: 기록 뒤·사건 뒤 → 같은 키로 이어 감(hc-lane report --key가 한 번만 남긴다)
	E1 := k.eOpen("e1", "4141bbb", "끝")
	k.ev("box-1/audit-"+E1, "audit-"+E1, "판정", s40("4141bbb"), "감사 완료 "+E1+" 모름")
	k.eAudDone("e1", E1)
	k.runE([]string{"HC_FILL_CRASH_AT=after-escalate-record"}, "run")
	k.check("중단 after-escalate-record: 기록은 escalate:unreadable, 사건은 아직", k.acol(E1, 8) == "escalate:unreadable" && k.nrep("hf:"+E1+":") == 0)
	k.runE([]string{"HC_FILL_CRASH_AT=after-escalate-event"}, "run")
	k.check("중단 after-escalate-event: 사건 한 번(키 hf:<ID>:unreadable), 기록은 아직 escalate", k.nrep("hf:"+E1+":unreadable") == 1 && k.acol(E1, 8) == "escalate:unreadable")
	k.run("run")
	k.run("run")
	k.check("그 뒤 바퀴: 같은 키로 한 번 더 부르고(hc-lane이 같은 키는 안 더함) coord, 그 뒤로는 부르지 않음", k.nrep("hf:"+E1+":unreadable") == 2 && k.acol(E1, 8) == "coord")
	// 감사 줄을 rm하면(queued 차수) 지휘에 한 번, 다시 띄우지 않음
	k.afresh()
	k.add("closed", "--only", "box-1")
	k.run("run")
	k.lj("box-1/closed:claude-code:closed:idle:/")
	k.ev("box-1/closed", "closed", "완료", "5858aaa", "끝")
	k.run("run")
	CL := k.aidof("closed")
	var at []string
	for _, r := range rows(k.AT()) {
		if col(r, 1) == CL {
			for len(r) < 8 {
				r = append(r, "")
			}
			r[7] = "taken"
		}
		at = append(at, strings.Join(r, "\t"))
	}
	write(k.t, k.AT(), strings.Join(at, "\n")+"\n")
	copyFile(k.t, k.Q, k.T+"/before-disconnect")
	k.mj("box-2")
	k.lj()
	k.run("run")
	k.check("감사 정리: 기계가 끊기면 정리할 감사 줄을 보존", k.where("audit-"+CL) != "")
	copyFile(k.t, k.T+"/before-disconnect", k.Q)
	k.mj("box-1", "box-2")
	k.lj("box-1/audit-" + CL + ":claude-code:new-user:idle:/")
	k.run("run")
	k.check("감사 정리: 다른 터미널이 같은 주소를 쓰면 종료 안내를 보내지 않음", k.nrep("hf:"+CL+":cleanup") == 0 && k.where("audit-"+CL) == "")

	R2 := k.eOpen("r2", "4242ccc", "끝")
	k.fill(nil, "rm", "audit-"+R2)
	write(k.t, k.T+"/lane.log", "")
	k.run("run")
	k.run("run")
	k.check("감사 줄 rm → 차수 coord, 지휘에 한 번(hf:<ID>:cancelled), 감사 다시 안 띄움",
		k.acol(R2, 8) == "coord" && k.nrep("hf:"+R2+":cancelled") == 1 && !hasFx(k.T+"/lane.log", "audit-"+R2) && k.aidof("r2") == R2, read(k.AT()))

	// 판정 없이 감사 터미널이 끝나거나 사라지면 세 바퀴 뒤 지휘로, working이면 세지 않음
	V1 := k.eOpen("v1", "6666fff", "끝")
	k.eAudDone("v1", V1)
	k.ev("box-1/audit-"+V1, "audit-"+V1, "완료", "-", "판정 대신 완료로 적음")
	k.run("run")
	k.run("run")
	k.check("판정 없이 done 두 바퀴: 아직 지휘로 안 넘김", k.nrep("hf:"+V1+":") == 0 && k.acol(V1, 8) == "queued")
	k.run("run")
	k.check("판정 없이 done 세 바퀴 → 「<레인> 감사 1번째 판정 없음 — 지휘 판단」(hf:<ID>:noverdict)",
		hasF(rl, "report 막힘 - v1 감사 1번째 판정 없음 — 지휘 판단") && k.nrep("hf:"+V1+":noverdict") == 1 && k.acol(V1, 8) == "coord", read(k.AT()))
	V2 := k.eOpen("v2", "7777aaa", "끝")
	k.lj("box-1/v2:claude-code:v2:done:/")
	k.run("run")
	k.run("run")
	k.run("run")
	k.check("감사 터미널이 판정 없이 사라짐 세 바퀴 → 지휘로", k.nrep("hf:"+V2+":noverdict") == 1 && k.acol(V2, 8) == "coord", read(k.AT()))
	V3 := k.eOpen("v3", "8888bbb", "끝")
	k.lj("box-1/v3:claude-code:v3:done:/", "box-1/audit-"+V3+":claude-code:audit-"+V3+":working:/")
	k.run("run")
	k.run("run")
	k.run("run")
	k.check("감사가 working이면 판정 없음으로 세지 않음", !exists(rl) && k.acol(V3, 8) == "queued")

	// 완료 sha: 없음(-)·작업 트리에 없는 sha는 지휘에 한 번, 그 뒤 sha가 있는 완료는 그대로 감사
	k.eOpen("s1", "-", "sha 빠뜨림")
	k.run("run")
	k.check("완료에 sha 없음 → 지휘에 「자동 감사 못 함 <레인>: 완료에 sha 없음」 한 번(차수 nosha)",
		countF(rl, "자동 감사 못 함 s1: 완료에 sha 없음") == 1 && k.acol(k.aidof("s1"), 8) == "nosha" && k.eQ15("audit=s1") == 0, read(rl))
	k.ev("box-1/s1", "s1", "완료", "cccc999", "다시")
	k.run("run")
	k.check("그 뒤 sha가 있는 완료는 그대로 감사", k.state("audit-"+k.aidof("s1")) == "up")
	k.afresh()
	k.add("s2", "--only", "box-1")
	k.run("run")
	k.lj("box-1/s2:claude-code:s2:done:/")
	k.ev("box-1/s2", "s2", "완료", "dead999", "없는 커밋")
	k.runE([]string{"HC_FAKE_REVNONE=dead999"}, "run")
	k.runE([]string{"HC_FAKE_REVNONE=dead999"}, "run")
	k.check("완료 sha가 작업 트리에 없음(rev-parse 종료 1, 출력 없음) → 지휘에 한 번, 감사 줄 없음",
		countF(rl, "완료 sha dead999 가 작업 트리") == 1 && k.eQ15("audit=s2") == 0, read(rl))

	// 같은 이름을 새로 띄운 새 실행(created_ms 다름)은 옛 실행의 차수를 물려받지 않음(지적수 0부터)
	Q1 := k.eOpen("r1", "9898aaa", "옛 일")
	k.verd("box-1/audit-"+Q1, Q1, "0", "1", "0")
	k.eAudDone("r1", Q1)
	k.run("run")
	write(k.t, vd("r1", Q1, ".ack"), "taken\tx\tr1@1700000000000\n")
	k.run("run")
	k.set("HC_FAKE_CREATED=" + "box-1/r1=1700000555000") // 이어 붙임: 공개 내보내기 비밀 꼴(긴 토큰)에 안 걸리게
	k.ev10("box-1/r1", "r1", "완료", "9898aaa", "같은 커밋을 새 실행에서 완료", "r1@1700000555000")
	k.lj("box-1/r1:claude-code:r1:done:/")
	k.run("run")
	Q2 := k.aidof("r1")
	k.check("새 실행의 첫 차수는 1번째·지적수 0(옛 실행 기록과 따로)",
		Q2 != Q1 && k.acol(Q2, 3) == "1" && k.acol(Q2, 9) == "0" && k.acol(Q2, 4) == "r1@1700000555000", read(k.AT()))
	k.unset("HC_FAKE_CREATED")

	// 옛 감사 기록(7칸) 옮기기: 끝 상태는 legacy(같은 sha로 다시 안 엶), queued는 지휘에 한 번 — 옛 감사 줄은 정리
	k.afresh()
	k.add("lg", "--only", "box-1")
	k.add("lq", "--only", "box-1")
	k.run("run")
	write(k.t, k.AT(), "lg\tabc7777\t1\t0\tpass\t0\t0\nlq\tdef8888\t1\t0\tqueued\t0\t0\n")
	k.lj("box-1/lg:claude-code:lg:done:/", "box-1/lq:claude-code:lq:done:/")
	k.ev("box-1/lg", "lg", "완료", "abc7777", "옛 판정이 이미 간 완료")
	k.run("run", "--dry-run")
	k.check("옛 기록: dry-run은 옮기지 않음", k.eArows() == 0)
	k.run("run")
	k.check("옛 기록 옮김: legacy·escalate:legacy(결정적 ID al…), 같은 레인·같은 sha 완료로 다시 열지 않음, queued는 지휘에 한 번",
		k.acol(k.aidof("lg"), 8) == "legacy" && regexp.MustCompile(`^al.{8}$`).MatchString(k.aidof("lg")) && k.eQ15("audit=lg") == 0 && k.nrep("hf:"+k.aidof("lq")+":legacy") == 1 && k.acol(k.aidof("lq"), 8) == "coord", read(k.AT()))

	// 옛 설치 틀({audit_id} 없음): 지시문 끝에 새 판정 줄 절이 붙는다
	k.afresh()
	write(k.t, k.T+"/oldtpl.md", eSed1(read(k.EXA), `\{audit_id\}`, "{sha}"))
	k.confDel(`^audit `)
	k.confAppend("audit    " + k.T + "/oldtpl.md")
	k.add("ot", "--only", "box-1")
	k.run("run")
	k.lj("box-1/ot:claude-code:ot:done:/")
	k.ev("box-1/ot", "ot", "완료", "5050aaa", "끝")
	k.run("run")
	O1 := k.aidof("ot")
	k.check("옛 틀: 「## 판정 줄 (hc-fill — 이 절이 위 판정 줄보다 앞선다)」 절이 붙고 차수 ID·40자 sha가 든다",
		hasF(k.ABR()+"/"+O1+".md", "## 판정 줄 (hc-fill — 이 절이 위 판정 줄보다 앞선다)") && hasF(k.ABR()+"/"+O1+".md", "hc-lane report 판정 "+s40("5050aaa")+" \"감사 완료 "+O1+" 높음"), read(k.ABR()+"/"+O1+".md"))

	// 진짜 hc-lane report·verdict로 끝까지: 배포 틀 판정 줄 → 소비 → 판정 파일 → 레인이 hc-lane verdict → taken
	k.afresh()
	RR := k.T + "/realrepo"
	rm(RR)
	mkdirs(k.t, RR)
	writeX(k.t, k.T+"/bin2/hc", read(k.T+"/bin/hc")) // ln -sf $T/bin/hc $T/bin2/hc 와 같은 가짜
	k.exe(RR, nil, "/usr/bin/git", "init", "-q")
	k.exe(RR, nil, "/usr/bin/git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "one")
	RS, _ := k.exe(RR, nil, "/usr/bin/git", "rev-parse", "HEAD")
	if len(RS) < 8 {
		k.t.Fatalf("realrepo HEAD를 못 읽음: %q", RS)
	}
	k.add("rl", "--only", "box-1")
	k.run("run")
	k.lj("box-1/rl:claude-code:rl:done:" + RR)
	k.ev("box-1/rl", "rl", "완료", RS[:8], "끝")
	k.runE([]string{"HC_FAKE_SHA40=" + RS}, "run")
	RL := k.aidof("rl")
	RPATH := k.T + "/bin2:" + strings.TrimPrefix(k.env["PATH"], k.T+"/bin:") // 가짜 hc만(진짜 git으로 HEAD를 읽게)
	k.exe(k.T, []string{"PATH=" + RPATH, "XDG_STATE_HOME=" + k.T + "/xdg-a", "HC_TERMINAL_ID=audit-" + RL, "HC_LANE=audit-" + RL, "HC_LANE_MACHINE=m"},
		k.HL, "report", "판정", RS, "감사 완료 "+RL+" 높음 1·중간 0·낮음 0")
	var evs []string
	for _, r := range rows(k.T + "/xdg-a/hc-lane/events.tsv") {
		for len(r) < 3 {
			r = append(r, "")
		}
		r[2] = "box-1/" + r[2]
		evs = append(evs, strings.Join(r, "\t")+"\n")
	}
	appendFile(k.t, k.T+"/events", strings.Join(evs, ""))
	k.lj("box-1/rl:claude-code:rl:done:"+RR, "box-1/audit-"+RL+":claude-code:audit-"+RL+":done:/")
	k.run("run")
	k.exe(k.T, nil, "/usr/bin/git", "-C", RR, "worktree", "add", "-q", "--detach", RR+".wt", "HEAD") // 레인은 연결 작업 트리에서 부른다
	vo, _ := k.exe(RR+".wt", []string{"PATH=" + RPATH, "HC_TERMINAL_ID=rl", "HC_LANE=rl"}, k.HL, "verdict", RL)
	k.run("run")
	k.check("진짜 report 판정(배포 틀 꼴) → 소비 → 판정 파일 → 진짜 hc-lane verdict가 지적을 수거(taken) → 차수 taken",
		k.vkey("rl", "verdict", RL) == "fix" && strings.Contains(vo, "감사 지적") && eCut1(vd("rl", RL, ".ack")) == "taken" && k.acol(RL, 8) == "taken", vo, read(k.AT()))

	// 감사 대상이 아닌 줄: --no-audit · 받은편지 · audit 키 없음
	k.afresh()
	k.run("add", "--name", "na", "--project", "app", "--brief", k.brief("na"), "--only", "box-1", "--no-audit")
	k.check("add --no-audit: 감사 칸 no-audit, 알림에 「자동 감사 끔」", k.rc == 0 && k.col15("na") == "no-audit" && strings.Contains(k.out, "자동 감사 끔"))
	k.run("run")
	k.lj("box-1/na:claude-code:na:done:/")
	k.ev("box-1/na", "na", "완료", "abcdef7", "끝")
	k.run("run")
	k.check("--no-audit 줄: 감사·착지 절 없음, 완료해도 감사 줄 없음",
		!hasF(k.DATA+"/runs/na-run.md", "감사·착지") && k.aidof("na") == "" && k.col15("na") == "no-audit")
	// 2026-11-22 지휘(23:58): --no-audit 레인이 「감사 판정」을 기다리며 멈춘 일 넷 — 지시문 끝에 바로 착지하라는 고정 한 줄
	k.check("--no-audit 줄: run 지시문 끝 줄이 「자동 감사 없음 — … 바로 착지(hc-lane land)한다」",
		strings.HasSuffix(read(k.DATA+"/runs/na-run.md"), "\n자동 감사 없음 — 완료 뒤 감사 판정을 기다리지 말고 바로 착지(hc-lane land)한다\n"), read(k.DATA+"/runs/na-run.md"))
	k.ifresh()
	k.confAppend("audit    " + k.EXA)
	rm(k.T + "/events")
	k.mj("box-1", "shared", "slow")
	k.req(k.IS(), "inq-z", "app", "-", "slow")
	k.run("run")
	k.lj("slow/inq-z:claude-code:inq-z:done:/")
	write(k.t, k.T+"/events", "2026-11-19T01:00:00Z\tm\tslow/inq-z\tinq-z\t완료\t1234abc\t끝\t1\n")
	k.run("run")
	k.check("받은편지 줄: 감사·착지 절 없음, 완료해도 감사 줄 없음",
		k.state("inq-z") == "up" && !hasF(k.DATA+"/runs/inq-z-run.md", "감사·착지") && k.aidof("inq-z") == "")
	k.fresh()
	rm(k.T+"/events", k.T+"/events.log")
	k.cpConf()
	k.add("nk", "--only", "box-1")
	k.run("run")
	k.lj("box-1/nk:claude-code:nk:done:/")
	write(k.t, k.T+"/events", "2026-11-19T01:00:00Z\tm\tbox-1/nk\tnk\t완료\t1234abc\t끝\t1\n")
	k.run("run")
	k.check("audit 키 없음: 절 없음, 감사 줄·기록 없음(events는 마지막 보고로 읽음)",
		!hasF(k.DATA+"/runs/nk-run.md", "감사·착지") && len(k.ePre15()) == 0 && !exists(k.AT()) && k.col15("nk") != "auto")

	// 긴 이름 · dry-run
	ln := "abcdefghijklmnopqrstuvwxyz0123"
	k.eOpen(ln, "1111aaa", "끝")
	an := strings.Join(k.ePre15(), "\n")
	k.check("긴 레인 이름도 감사 줄 이름은 audit-<차수 ID>(32자 안), 감사 칸 audit=<레인>",
		utf8.RuneCountInString(an) <= 32 && an == "audit-"+k.aidof(ln) && k.col15(an) == "audit="+ln, "이름: "+an)
	k.afresh()
	k.add("w7", "--only", "box-1")
	k.run("run")
	k.lj("box-1/w7:claude-code:w7:done:/")
	k.ev("box-1/w7", "w7", "완료", "4444ddd", "끝")
	before := strings.TrimRight(read(k.Q), "\n")
	k.run("run", "--dry-run")
	k.check("dry-run: 「넣을 감사」만 찍고 대기열·기록·지시문 그대로",
		strings.Contains(k.out, "[dry-run] 넣을 감사 audit-<새 차수> ← w7 4444ddd0 (1번째, box-1)") && strings.TrimRight(read(k.Q), "\n") == before && !nonEmpty(k.AT()) && !isDir(k.ABR()))
	k.run("run")
	W7 := k.aidof("w7")
	k.verd("box-1/audit-"+W7, W7, "0", "0", "1")
	k.eAudDone("w7", W7)
	k.run("run", "--dry-run")
	k.check("dry-run: 「넘길 판정」만 찍고 판정 파일·깨움 없음",
		strings.Contains(k.out, "[dry-run] 넘길 판정 audit-"+W7+" → w7 box-1/w7 (pass") && !exists(send) && !exists(vd("w7", W7, ".md")) && k.acol(W7, 8) == "queued")
	// 레인상한 1인 기계: 감사 줄은 레인상한을 안 봄(교착 없음)
	k.afresh()
	k.confSed(`^machine  box-1    2   -$`, "machine  box-1    2   1")
	k.add("l1", "--only", "box-1")
	k.run("run")
	k.lj("box-1/l1:claude-code:l1:done:/")
	k.ev("box-1/l1", "l1", "완료", "5555eee", "끝")
	k.run("run")
	L1 := k.aidof("l1")
	k.check("레인상한 1인 기계에서 완료 → 감사 줄이 뜬다(감사 줄은 레인상한을 안 봄)", k.where("audit-"+L1) == "up box-1 box-1/audit-"+L1, read(k.Q))
	k.add("l2", "--only", "box-1")
	k.lj("box-1/l1:claude-code:l1:done:/", "box-1/audit-"+L1+":claude-code:audit-"+L1+":working:/")
	k.run("run")
	k.check("레인상한 1에서 감사를 기다리는 동안 다른 쓰기 줄은 대기", k.where("l2") == "wait - -")
	// 그 기계에서 상태가 안 잡혀 남긴(stuck) 감사 줄은 자리를 쥔 것으로 보지 않음 — 레인을 다시 셈
	k.afresh()
	mkdirs(k.t, k.W+"/app.wt/q1")
	k.add("q1", "--only", "box-1")
	k.run("run")
	k.lj("box-1/q1:claude-code:q1:done:" + k.W + "/app.wt/q1")
	k.ev("box-1/q1", "q1", "완료", "eeee777", "끝")
	k.runE([]string{"HC_FILL_CRASH_AT=after-put"}, "run")
	QA := k.aidof("q1")
	k.runE([]string{"HC_FAKE_NOSTATUS=box-1/audit-" + QA}, "run")
	k.run("run")
	k.check("stuck 감사 줄(그 레인 기계에 못 뜸): 감사 기다리는 레인을 다시 자리로 셈",
		k.where("audit-"+QA) == "wait - -" && strings.Contains(k.out, "자리 box-1 box-1/q1 q1: done, 작업 트리 있음 → 셈"), read(k.Q))
	k.check("자동 감사는 어떤 터미널도 끄지 않음(hc kill 없음)", !hasE(k.T+"/hc.log", "^kill"))

	// add·받은편지 이름 규칙(감사 줄 이름 자리·커밋 종류)
	k.run("add", "--name", "audit-x", "--project", "app", "--brief", k.brief("ax"))
	k.check("add: audit- 머리 이름은 거절(감사 줄 이름 자리)", k.rc == 1 && k.where("audit-x") == "" && strings.Contains(k.out, "audit-"))
	k.fresh()
	k.cpConf()
	k.run("add", "--name", "audit-y", "--project", "app", "--brief", k.brief("ay"))
	k.check("add: 감사 꺼짐(audit 키 없음)에서도 audit- 머리 이름 거절", k.rc == 1 && k.where("audit-y") == "")
	k.ifresh()
	k.mj("box-1", "shared", "slow")
	k.req(k.IS(), "audit-zz", "app")
	k.run("run")
	k.check("받은편지: audit- 머리 이름은 거절(rejected/<이름>.why, 감사 꺼짐에서도)", hasF(k.IS()+"/rejected/audit-zz.why", "audit-") && k.where("audit-zz") == "")
	k.cpConf()
	// 커밋 종류와 같은 이름(fix·docs…)은 착지·선행 판정이 아무 「fix: …」 커밋에나 맞으니 거절, 그 낱말로 시작하는 이름은 받음
	k.fresh()
	k.run("add", "--name", "fix", "--project", "app", "--brief", k.brief("kf"))
	k.check("add: 커밋 종류와 같은 이름(fix) 거절", k.rc == 1 && k.where("fix") == "" && strings.Contains(k.out, "레인 이름이 커밋 종류(fix)와 같으면"))
	k.run("add", "--name", "fix-login", "--project", "app", "--brief", k.brief("kl"))
	k.check("add: 커밋 종류로 시작하는 이름(fix-login)은 받음", k.rc == 0 && k.state("fix-login") == "wait")
	k.run("add", "--name", "kd", "--project", "app", "--brief", k.brief("kd"), "--after", "fix-login,docs")
	k.check("add: --after에 커밋 종류 이름(docs) 거절", k.rc == 1 && k.where("kd") == "" && strings.Contains(k.out, "커밋 종류(docs)"))
	k.ifresh()
	k.mj("box-1", "shared", "slow")
	k.req(k.IS(), "fix", "app")
	k.req(k.IS(), "kfa", "app", "docs")
	k.run("run")
	k.check("받은편지: 커밋 종류 이름(fix)·after(docs) 거절, why에 사유",
		hasF(k.IS()+"/rejected/fix.why", "레인 이름이 커밋 종류(fix)와 같으면 착지·선행 판정이 아무 fix: 커밋에나 맞는다") && hasF(k.IS()+"/rejected/kfa.why", "커밋 종류(docs)") && k.where("fix") == "" && k.where("kfa") == "")
	k.cpConf()

	k.afresh()
	k.confAppend("audit    " + k.T + "/없는틀.md")
	k.confDel(`^audit    ` + regexp.QuoteMeta(k.EXA) + `$`)
	k.add("w8")
	k.run("run")
	k.check("설정 audit 틀을 못 읽으면 종료 1, 띄우지 않음", k.rc == 1 && strings.Contains(k.out, "설정 audit 틀을 읽지 못함") && !exists(k.T+"/lane.log"))
	k.cpConf()
}
