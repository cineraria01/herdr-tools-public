package hcfill_test

// 10h. 자동 감사 앞부분(셸 시험 「# 10h. 자동 감사」 ~ 「# 넘기기 멱등성」 앞): 차수 열기·판정 받기·수거·지적 되풀이·부모 실행.
// 쓰기 레인의 「완료 <sha>」 → 그 실행·그 커밋(40자)에 묶인 감사 차수(ID a…) → 긴급 감사 줄 audit-<ID> → 판정을
// 레인 기계 verdicts/<레인>/<ID>.md에 두고 깨움 → 레인이 hc-lane verdict로 수거(<ID>.ack).

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func init() { reg(50, "e", secE) }

// ePrio: 셸의 prio_of — 그 이름 줄의 13칸(비면 "-", 줄이 없으면 "").
func (k *K) ePrio(name string) string {
	for _, r := range rows(k.Q) {
		if col(r, 1) == name {
			if col(r, 13) == "" {
				return "-"
			}
			return col(r, 13)
		}
	}
	return ""
}

// eArows: audit.tsv에서 17칸 이상 줄 수(awk NF >= 17 | grep -c .).
func (k *K) eArows() int {
	n := 0
	for _, r := range rows(k.AT()) {
		if len(r) >= 17 {
			n++
		}
	}
	return n
}

// eQ15: 대기열에서 15칸이 v인 줄 수 · ePre15: 15칸이 audit=로 시작하는 줄들의 1칸.
func (k *K) eQ15(v string) int {
	n := 0
	for _, r := range rows(k.Q) {
		if col(r, 15) == v {
			n++
		}
	}
	return n
}

func (k *K) ePre15() []string {
	var l []string
	for _, r := range rows(k.Q) {
		if strings.HasPrefix(col(r, 15), "audit=") {
			l = append(l, col(r, 1))
		}
	}
	return l
}

// eLine: 글에 prefix로 시작하고 sub가 든 줄이 있나(grep "^prefix" | grep -Fq sub).
func eLine(s, prefix, sub string) bool {
	for _, l := range lines(s) {
		if strings.HasPrefix(l, prefix) && strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

// eLs: "$FILL" ls 2>&1 (k.out은 그대로).
func (k *K) eLs() string { o, _ := k.exe(k.T, nil, k.FILL, "ls"); return o }

// eCut1: cut -f 1 (줄마다 첫 칸, 끝 줄바꿈 없이).
func eCut1(p string) string {
	var l []string
	for _, r := range rows(p) {
		l = append(l, r[0])
	}
	return strings.Join(l, "\n")
}

// eMd: find dir -name "*.md" | wc -l.
func eMd(dir string) int {
	n := 0
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(d.Name(), ".md") {
			n++
		}
		return nil
	})
	return n
}

var eAidRe = regexp.MustCompile(`^a.*-.{6}$`)

func secE(k *K) {
	W := k.W
	app := func(n string) string { return W + "/app.wt/" + n }
	mkdirs(k.t, app("w1"), app("w2"))

	k.afresh()
	// 이 절은 수거 뒤 정리(aCleanup)의 닫기·「정리 필요」를 본다 — 판정을 넘긴 바퀴의 닫기(fill-audit-close)는 끄고
	// cmd/hc-fill/auditclose_test.go가 따로 본다
	k.confAppend("audit_close 0")
	k.add("w1", "--only", "slow")
	k.run("run")
	rf := k.DATA + "/runs/w1-run.md"
	k.check("자동 감사: 쓰기 레인 run 지시문 끝에 「감사·착지 (hc-fill 자동)」 절(완료 보고 · hc-lane verdict <ID> · land --verdict), 감사 칸 auto",
		k.where("w1") == "up slow slow/w1" && k.col15("w1") == "auto" && hasF(rf, "## 감사·착지 (hc-fill 자동)") && hasF(rf, "hc-lane report 완료 HEAD") && hasF(rf, "hc-lane verdict <ID>") && hasF(rf, "hc-lane land <기본> --verdict <ID>"), read(rf))
	k.check("자동 감사: 완료 사건 전에는 감사 줄 없음", len(k.ePre15()) == 0 && !nonEmpty(k.AT()))
	k.lj("slow/w1:claude-code:w1:done:" + app("w1"))
	k.ev("slow/w1", "w1", "완료", "abc1234", "끝")
	k.ev("slow/w1", "w1", "중간", "-", "완료 뒤 중간 보고")
	k.ev("-", "w1", "막힘", "-", "hc-fill이 남긴 것(다른 터미널)")
	k.run("run")
	A1 := k.aidof("w1")
	k.check("완료 뒤 중간 보고가 끼어도 그 실행의 최신 완료로 차수를 연다 → 긴급 감사 줄 audit-<ID>(주인·우선·감사 칸), 같은 바퀴에 띄움",
		eAidRe.MatchString(A1) && k.where("audit-"+A1) == "up slow slow/audit-"+A1 && k.col15("audit-"+A1) == "audit=w1" && k.ePrio("audit-"+A1) == "urgent" && strings.Contains(k.out, "감사 넣음 audit-"+A1+" ← w1 abc12340 (1번째, slow)"), read(k.Q), read(k.AT()))
	k.check("감사 줄 띄우기: hc-lane --read, --repo는 그 레인 작업 트리, events는 그 기계 --on",
		hasFx(k.T+"/lane.log", "--read") && hasFx(k.T+"/lane.log", "/Users/other/src/app.wt/w1") && hasF(k.T+"/events.log", "events --on slow"), read(k.T+"/lane.log"))
	ab := k.ABR() + "/" + A1 + ".md"
	k.check("감사 지시문에 재현 프로세스의 시간 제한·종료·회수 규칙을 넣음", hasF(ab, "trap") && hasF(ab, "wait") && hasF(ab, "시간 제한"))
	k.check("감사 지시문 briefs/audit/<ID>.md: 틀 자리를 다 채움(40자 sha·차수 ID·보고서 reports/audit-<ID>.md·원 지시문)",
		hasF(ab, "git -C /Users/other/src/app.wt/w1 diff origin/main..."+s40("abc1234")) && hasF(ab, "~/.local/share/hc-fill/runs/w1-run.md") && hasF(ab, "~/.local/share/hc-fill/reports/audit-"+A1+".md") && hasF(ab, "hc-lane report 판정 "+s40("abc1234")+" \"감사 완료 "+A1+" 높음") && !hasE(ab, `\{[a-z_]*\}`), read(ab))
	arun := k.DATA + "/runs/audit-" + A1 + "-run.md"
	k.check("감사 줄의 run 지시문에는 감사·착지 절 없음(읽기 레인), 차수마다 따로", exists(arun) && !isDir(arun) && !hasF(arun, "감사·착지 (hc-fill 자동)"))
	k.check("자리: 감사에 넘겨진 쓰기 레인(done)은 쉬므로 안 셈 — 자리 1인 slow에 감사가 뜸", strings.Contains(k.out, "자리 slow slow/w1 w1: done, 쉼 → 안 셈"))
	got := strings.Join([]string{k.acol(A1, 2), k.acol(A1, 3), k.acol(A1, 4), k.acol(A1, 5), k.acol(A1, 6), k.acol(A1, 7), k.acol(A1, 8)}, " ")
	k.check("audit.tsv 차수 줄: 레인 차수 부모실행(<터미널>@<created_ms>) 40자 sha 기계 부모주소 queued",
		got == "w1 1 w1@1700000000000 "+s40("abc1234")+" slow slow/w1 queued", read(k.AT()))
	k.run("ls")
	k.check("ls: 감사 중 · 감사 대상", eLine(k.out, "w1 ", "감사 중(1번째)") && eLine(k.out, "audit-"+A1+" ", "감사 대상 w1"))
	write(k.t, k.T+"/lane.log", "")
	k.lj("slow/w1:claude-code:w1:done:"+app("w1"), "slow/audit-"+A1+":claude-code:audit-"+A1+":working:"+app("w1"))
	k.run("run")
	k.check("같은 실행·같은 sha는 두 번 열지 않음", !hasFx(k.T+"/lane.log", "--name") && !strings.Contains(k.out, "감사 넣음") && k.eArows() == 1)
	// 옛 꼴 판정(차수 ID 없음·6칸 -)과 다른 차수 ID 판정은 받지 않는다
	k.lj("slow/w1:claude-code:w1:done:"+app("w1"), "slow/audit-"+A1+":claude-code:audit-"+A1+":working:"+app("w1"))
	k.ev("slow/audit-"+A1, "audit-"+A1, "판정", "-", "감사 완료 abc1234 높음 0·중간 0·낮음 2")
	k.ev("slow/audit-"+A1, "audit-"+A1, "판정", s40("abc1234"), "감사 완료 azzzzzz-zzzzzz 높음 0·중간 0·낮음 2")
	k.run("run")
	k.check("옛 꼴 판정·다른 차수 ID 판정은 받지 않음(판정 파일·깨움 없음)", !exists(k.VD()+"/w1/"+A1+".md") && !exists(k.T+"/send.log") && k.acol(A1, 8) == "queued")
	k.verd("slow/audit-"+A1, A1, "0", "0", "2")
	k.lj("slow/w1:claude-code:w1:done:"+app("w1"), "slow/audit-"+A1+":claude-code:audit-"+A1+":done:"+app("w1"))
	k.run("run")
	send := k.T + "/send.log"
	k.check("판정 받음(통과) → 레인 기계 verdicts/w1/<ID>.md(aid·run·40자 sha·verdict·report), 깨움(hc-lane verdict <ID>), 상태 deliver",
		k.vkey("w1", "aid", A1) == A1 && k.vkey("w1", "run", A1) == "w1@1700000000000" && k.vkey("w1", "sha", A1) == s40("abc1234") && k.vkey("w1", "verdict", A1) == "pass" && k.vkey("w1", "report", A1) == "~/.local/share/hc-fill/reports/audit-"+A1+".md" && hasF(send, "send slow/w1 hc-fill: 감사 판정이 왔다 — 이 작업 트리에서 hc-lane verdict "+A1+" 를 돌리고") && k.acol(A1, 8) == "deliver" && !hasF(send, "감사 통과 ("), read(send), read(k.AT()))
	k.run("run")
	k.check("수거 전 다시 깨움은 wake_every 뒤에만(이번 바퀴 깨움 없음), 판정 파일은 그대로(ALREADY)", countF(send, "hc-lane verdict "+A1) == 1 && k.acol(A1, 8) == "deliver")
	k.run("ls")
	k.check("ls: 감사 통과 — 수거 기다림", eLine(k.out, "w1 ", "감사 통과 — 수거 기다림"))
	write(k.t, k.VD()+"/w1/"+A1+".ack", "landing\t2026-11-20T00:00:00Z\tw1@1700000000000\tp\n")
	// fill-audit-term-close: 그 주소를 다른 실행(created_ms가 띄울 때 확정한 것과 다름)이 쓰면 닫지 않고 지금처럼 지휘에 알린다
	k.runE([]string{"HC_FAKE_CREATED=slow/audit-" + A1 + "=1800000000000"}, "run")
	rl := k.T + "/report.log"
	k.check("레인이 수거(<ID>.ack landing) → 차수 taken, 주소를 다른 실행이 씀 → 끄지 않고 정리 주소를 지휘에 한 번(키 hf:<ID>:cleanup), 감사 줄 뺌",
		k.acol(A1, 8) == "taken" && hasF(rl, "--key hf:"+A1+":cleanup") && hasF(rl, "감사 터미널 slow/audit-"+A1+" 정리 필요") && k.where("audit-"+A1) == "" && !hasE(k.T+"/hc.log", "^kill"), read(rl))
	k.run("run")
	k.check("끝난 차수는 다시 안 다룸(정리 사건 한 번), ls 「감사 통과 받음」", k.nrep("hf:"+A1+":cleanup") == 1 && eLine(k.eLs(), "w1 ", "감사 통과 받음"))

	// 지적 → 수거(taken) → 고침 → 새 차수(2번째, 지적수 이어 감) → 지적 → 상한이면 주인 지휘에(키 hf:<ID>:limit)
	k.afresh()
	k.fill(nil, "add", "--name", "w2", "--project", "app", "--brief", k.brief("w2"), "--only", "box-1", "--coord", "tq1")
	k.run("run")
	w2 := "box-1/w2:claude-code:w2:done:" + app("w2")
	k.lj(w2)
	k.ev("box-1/w2", "w2", "완료", "aaaa111", "1차")
	k.run("run")
	B1 := k.aidof("w2")
	k.check("주인 지휘가 있는 레인: 감사 줄 주인도 그 지휘", k.qcol("audit-"+B1, 12) == "tq1")
	k.verd("box-1/audit-"+B1, B1, "1", "0", "0")
	k.lj(w2, "box-1/audit-"+B1+":claude-code:audit-"+B1+":done:"+app("w2"))
	k.run("run")
	k.check("지적 → 판정 파일 verdict fix·지적수 1, ls 「감사 지적 1번째 — 수거 기다림」",
		k.vkey("w2", "verdict", B1) == "fix" && k.acol(B1, 9) == "1" && eLine(k.eLs(), "w2 ", "감사 지적 1번째 — 수거 기다림"), read(k.AT()))
	write(k.t, k.VD()+"/w2/"+B1+".ack", "taken\t2026-11-20T00:00:00Z\tw2@1700000000000\n")
	k.run("run")
	k.check("지적 수거(taken) → 차수 taken", k.acol(B1, 8) == "taken")
	// fill-audit-term-close: 이름(audit-<차수 ID>)·실행이 띄울 때 확정한 것과 같으면 hc-fill이 그 정확한 주소를 스스로 닫는다(막힘 알림 없음)
	k.check("판정 수거 뒤 이름·차수 맞는 감사 터미널은 스스로 닫음(hc kill <주소>, 사건 한 줄, 정리 알림 없음), 감사 줄 뺌",
		hasE(k.T+"/hc.log", "^kill box-1/audit-"+B1+"$") && strings.Contains(k.out, "감사 터미널 닫음 box-1/audit-"+B1+" (audit-"+B1+", 차수 "+B1) &&
			k.nrep("hf:"+B1+":cleanup") == 0 && k.where("audit-"+B1) == "", read(k.T+"/hc.log"), k.out)
	write(k.t, k.T+"/lane.log", "")
	k.ev("box-1/w2", "w2", "완료", "bbbb222", "고침")
	k.lj(w2)
	k.run("run")
	B2 := k.aidof("w2")
	k.check("새 sha 완료 → 새 차수(다른 ID, 2번째, 지적수 1 이어 감), 새 감사 줄",
		B2 != B1 && k.acol(B2, 3) == "2" && k.acol(B2, 9) == "1" && k.state("audit-"+B2) == "up" && hasF(k.ABR()+"/"+B2+".md", s40("bbbb222")), read(k.Q), read(k.AT()))
	k.verd("box-1/audit-"+B1, B1, "0", "0", "0") // 지난 차수의 판정이 늦게 와도
	k.lj(w2, "box-1/audit-"+B2+":claude-code:audit-"+B2+":done:"+app("w2"))
	k.verd("box-1/audit-"+B2, B2, "0", "1", "3")
	k.run("run")
	k.check("2번째 지적(지적수 1 < audit_max 2) → 지적수 2, 지난 차수 판정은 안 씀",
		k.acol(B2, 9) == "2" && k.vkey("w2", "verdict", B2) == "fix" && k.acol(B1, 8) == "taken", read(k.AT()))
	write(k.t, k.VD()+"/w2/"+B2+".ack", "taken\tx\tw2@1700000000000\n")
	k.run("run")
	k.ev("box-1/w2", "w2", "완료", "cccc333", "또 고침")
	k.lj(w2)
	k.run("run")
	B3 := k.aidof("w2")
	k.lj(w2, "box-1/audit-"+B3+":claude-code:audit-"+B3+":done:"+app("w2"))
	k.verd("box-1/audit-"+B3, B3, "2", "0", "0")
	k.run("run")
	k.check("상한(지적 2번) → 주인 지휘에 hc-lane report 막힘 「<레인> 감사 3번째 … 지휘 판단」(--to 주인, 키 hf:<ID>:limit), 판정 파일 없음",
		hasF(rl, "report 막힘 - w2 감사 3번째 높음 2·중간 0 — 지휘 판단") && hasF(rl, "--name w2 --to tq1 --key hf:"+B3+":limit") && !exists(k.VD()+"/w2/"+B3+".md") && k.acol(B3, 8) == "coord", read(rl))
	k.run("run")
	k.check("지휘로 넘긴 뒤 되풀이 없음, ls 「지휘로 넘김」", k.nrep("hf:"+B3+":limit") == 1 && eLine(k.eLs(), "w2 ", "지휘로 넘김"))

	// 판정 줄을 못 읽으면 지휘로(주인 없음 → --to 없이 모든 지휘)
	k.afresh()
	k.add("w3", "--only", "box-1")
	k.run("run")
	k.lj("box-1/w3:claude-code:w3:done:/")
	k.ev("box-1/w3", "w3", "완료", "dddd444", "끝")
	k.run("run")
	C1 := k.aidof("w3")
	k.ev("box-1/audit-"+C1, "audit-"+C1, "판정", s40("dddd444"), "감사 완료 "+C1+" 끝 — 괜찮음")
	k.run("run")
	k.check("판정 줄을 못 읽음 → 모든 지휘에 막힘(--to 없음, 키 hf:<ID>:unreadable)",
		hasF(rl, "report 막힘 - w3 감사 1번째 판정 줄을 못 읽음") && !hasF(rl, "--to") && hasF(rl, "--key hf:"+C1+":unreadable") && !exists(send), read(rl))

	// 부모 실행: 같은 주소의 다른 실행(created_ms 다름)·없어진 부모의 판정은 받지 않고 지휘로
	k.afresh()
	k.add("p1", "--only", "box-1")
	k.run("run")
	k.lj("box-1/p1:claude-code:p1:done:/")
	k.ev("box-1/p1", "p1", "완료", "1212aaa", "끝")
	k.run("run")
	P1 := k.aidof("p1")
	k.verd("box-1/audit-"+P1, P1, "0", "0", "0")
	k.lj("box-1/p1:claude-code:p1:done:/", "box-1/audit-"+P1+":claude-code:audit-"+P1+":done:/")
	k.runE([]string{"HC_FAKE_CREATED=" + "box-1/p1=1700000999000"}, "run") // 이어 붙임: 공개 내보내기 비밀 꼴(긴 토큰)에 안 걸리게
	k.check("부모 주소의 created_ms가 다름(주소 재사용) → 판정 안 씀, 지휘에 한 번(hf:<ID>:parent-ended)",
		!exists(k.VD()+"/p1/"+P1+".md") && k.nrep("hf:"+P1+":parent-ended") == 1 && k.acol(P1, 8) == "coord", read(k.AT()))
	k.afresh()
	k.add("p2", "--only", "box-1")
	k.run("run")
	k.lj("box-1/p2:claude-code:p2:done:/")
	k.ev("box-1/p2", "p2", "완료", "1313bbb", "끝")
	k.run("run")
	P2 := k.aidof("p2")
	k.verd("box-1/audit-"+P2, P2, "0", "0", "0")
	k.lj("box-1/audit-" + P2 + ":claude-code:audit-" + P2 + ":done:/")
	k.runE([]string{"HC_FAKE_GONE=box-1/p2"}, "run")
	k.check("부모 터미널이 없어짐(not_found) → 판정 안 씀, 지휘에 한 번", !exists(k.VD()+"/p2/"+P2+".md") && k.nrep("hf:"+P2+":parent-ended") == 1)
	// 10칸 발행 실행이 다른 완료(같은 주소의 옛 실행)는 이 실행의 완료로 보지 않는다
	k.afresh()
	k.add("p3", "--only", "box-1")
	k.run("run")
	k.lj("box-1/p3:claude-code:p3:done:/")
	k.ev10("box-1/p3", "p3", "완료", "1414ccc", "옛 실행", "p3@1600000000000")
	k.run("run")
	k.check("10칸 발행 실행이 지금 실행이 아닌 완료로는 차수를 열지 않음", k.aidof("p3") == "")
	k.ev10("box-1/p3", "p3", "완료", "1515ddd", "이 실행", "p3@1700000000000")
	k.run("run")
	k.check("10칸이 지금 실행이면 연다", k.acol(k.aidof("p3"), 5) == s40("1515ddd"))

	// S1 완료 → 감사 → S2 완료 → 중간 → S1 판정: 지난 차수는 superseded(지휘 알림 없음), S2 차수가 열린다
	k.afresh()
	k.add("sp", "--only", "box-1")
	k.run("run")
	k.lj("box-1/sp:claude-code:sp:done:/")
	k.ev("box-1/sp", "sp", "완료", "2020aaa", "S1")
	k.run("run")
	S1 := k.aidof("sp")
	k.lj("box-1/sp:claude-code:sp:done:/", "box-1/audit-"+S1+":claude-code:audit-"+S1+":done:/")
	k.ev("box-1/sp", "sp", "완료", "2121bbb", "S2")
	k.ev("box-1/sp", "sp", "중간", "-", "중간")
	k.verd("box-1/audit-"+S1, S1, "0", "0", "0")
	k.run("run")
	S2 := k.aidof("sp")
	k.check("S1 판정이 S2 완료 뒤에 와도: S1 차수 superseded(판정 파일·지휘 사건 없음), S2 차수 열림(2번째)",
		k.acol(S1, 8) == "superseded" && !exists(k.VD()+"/sp/"+S1+".md") && (!nonEmpty(rl) || k.nrep("hf:"+S1+":") == 0) && S2 != S1 && k.acol(S2, 5) == s40("2121bbb") && k.acol(S2, 3) == "2", read(k.AT()))
	// 지난 감사가 아직 돌면 새 완료는 그 감사가 끝난 뒤에(감사 둘이 함께 안 돎)
	k.afresh()
	k.add("w6", "--only", "box-1")
	k.run("run")
	k.lj("box-1/w6:claude-code:w6:done:/")
	k.ev("box-1/w6", "w6", "완료", "2222bbb", "끝")
	k.run("run")
	G1 := k.aidof("w6")
	k.lj("box-1/w6:claude-code:w6:done:/", "box-1/audit-"+G1+":claude-code:audit-"+G1+":working:/")
	k.ev("box-1/w6", "w6", "완료", "3333ccc", "더 고침")
	k.run("run")
	k.check("지난 감사가 아직 돌면 새 완료는 미룸",
		strings.Contains(k.out, "자동 감사 미룸 w6: 새 완료 3333ccc — 지난 감사("+G1+" 2222bbb0)가 아직 돈다") && k.aidof("w6") == G1 && k.acol(G1, 8) == "queued")
}
