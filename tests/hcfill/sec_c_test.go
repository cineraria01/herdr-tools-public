package hcfill_test

// 10c. 받은편지(셸 시험 518~840줄 앞 절반): 설정 inbox 줄의 기계 폴더에서 걷어 대기열에, 끝나면 보고를 요청한 기계 done/으로 되돌림.
// 가짜 hc exec는 이 기계에서 돌므로 기계마다 폴더를 따로 둔다(shared는 ~/ 풀기 시험).
// 2026-11-20 fill-handoff-integrity: 요청 ID(대기열 11칸 #<ID>)·장부 inbox.tsv·고정본 replies/<ID>.md·요청자 표시 done/.hc-fill/

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func init() { reg(30, "c", secC) }

// cCat: $(cat 파일) — 끝 줄바꿈을 뗀 내용(없으면 "").
func cCat(p string) string { return strings.TrimRight(read(p), "\n") }

// cIsF: [ -f 파일 ].
func cIsF(p string) bool { fi, err := os.Stat(p); return err == nil && fi.Mode().IsRegular() }

// cCmp: cmp -s a b.
func cCmp(a, b string) bool {
	x, e1 := os.ReadFile(a)
	y, e2 := os.ReadFile(b)
	return e1 == nil && e2 == nil && string(x) == string(y)
}

// cSha: shasum -a 256 파일 | cut -d" " -f1(없으면 "").
func cSha(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// cCut: cut -d<d> -f<c> 파일의 출력(줄마다, 구분자 없는 줄은 그대로) — $(…)처럼 끝 줄바꿈 뗌.
func cCut(s, d string, c int) string {
	var l []string
	for _, x := range lines(s) {
		if !strings.Contains(x, d) {
			l = append(l, x)
			continue
		}
		f := strings.Split(x, d)
		if c <= len(f) {
			l = append(l, f[c-1])
		} else {
			l = append(l, "")
		}
	}
	return strings.Join(l, "\n")
}

// cState: where <이름> | cut -d" " -f1.
func (k *K) cState(name string) string { return cCut(k.where(name), " ", 1) }

// cQ11: awk '$1==이름 {print $11}' 대기열.
func (k *K) cQ11(name string) string {
	var l []string
	for _, r := range rows(k.Q) {
		if col(r, 1) == name {
			l = append(l, col(r, 11))
		}
	}
	return strings.Join(l, "\n")
}

// cQn: grep -c "^<이름>\t" 대기열.
func (k *K) cQn(name string) int {
	n := 0
	for _, l := range lines(read(k.Q)) {
		if strings.HasPrefix(l, name+"\t") {
			n++
		}
	}
	return n
}

// cPath: PATH=<dir>:$PATH.
func (k *K) cPath(dir string) string { return "PATH=" + dir + ":" + k.env["PATH"] }

// cO: printf "%s" "$out" | grep -Fq <s>.
func (k *K) cO(s string) bool { return strings.Contains(k.out, s) }

// cRewrite: TSV 파일을 줄마다 f로 고쳐 쓴다(nil이면 그 줄을 뺀다) — awk … >f.t && mv f.t f.
func (k *K) cRewrite(p string, f func([]string) []string) {
	var b strings.Builder
	for _, r := range rows(p) {
		if r = f(r); r != nil {
			b.WriteString(strings.Join(r, "\t") + "\n")
		}
	}
	write(k.t, p, b.String())
}

var (
	cRidRe  = regexp.MustCompile(`^r.*-.{6}$`)
	cRidOld = regexp.MustCompile(`^rl.{8}$`)
)

func secC(k *K) {
	IS, IB, IH, RP, LED, RPL := k.IS(), k.IB(), k.IH(), k.RP(), k.LED(), k.RPL()
	k.ifresh()
	k.mj("box-1", "shared", "slow")
	k.add("dupl")
	k.req(IS, "inq-a", "app", "-", "slow")
	k.req(IS, "nopr", "nope")
	k.req(IS, "Bad.X", "app")
	k.req(IS, "dupl", "app")
	k.req(IS, "badonly", "app", "-", "box-9")
	k.req(IS, "nobrief", "app")
	rm(IS + "/nobrief.brief.md")
	k.req(IS, "emptyb", "app")
	write(k.t, IS+"/emptyb.brief.md", " \n\n")
	k.req(IH, "inq-h", "app", "-", "box-1")
	k.req(IB, "inq-off", "app")
	k.run("run", "--dry-run")
	k.check("받은편지 dry-run: 받을·거절할만 찍고 아무것도 옮기지 않음", k.rc == 0 && k.cO("[dry-run] 받을 slow inq-a (app)") &&
		k.cO("[dry-run] 거절할 slow nopr: 설정에 없는 project: nope") && cIsF(IS+"/inq-a.req") && !exists(IS+"/rejected") &&
		k.where("inq-a") == "" && !exists(IS+"/.hc-fill"))
	k.run("run")
	RA := k.rid("inq-a")
	k.check("받은편지: 받아 대기열에(되돌릴곳 = 요청 기계:폴더/done/이름.md#<요청 ID>), 같은 바퀴에 띄움, 장부 open",
		k.rc == 0 && k.cO("받음 slow inq-a → 대기열 (app, 우선 높음, 요청 "+RA+")") && k.where("inq-a") == "up slow slow/inq-a" &&
			k.cQ11("inq-a") == "slow:"+IS+"/done/inq-a.md#"+RA && cRidRe.MatchString(RA) && k.lst(RA) == "open",
		read(k.Q), read(LED))
	k.check("받은편지: 지시문을 briefs/<기계>/<이름>.<요청 ID>.md로 복사, 요청 쪽 둘은 taken/<이름>.<ID>.*으로, 접수 표시는 지움",
		hasF(k.DATA+"/briefs/slow/inq-a."+RA+".md", "# 요청 inq-a") && cIsF(IS+"/taken/inq-a."+RA+".req") &&
			cIsF(IS+"/taken/inq-a."+RA+".brief.md") && !exists(IS+"/inq-a.req") && !exists(IS+"/.hc-fill/claim/inq-a"))
	f := k.DATA + "/runs/inq-a-run.md"
	k.check("받은편지: run 지시문 끝에 결과 보고 절(요청 ID 보고 경로·mv로 두기·결과 줄 꼴)", hasF(f, "# 요청 inq-a") &&
		hasF(f, "~/.local/share/hc-fill/reports/inq-a."+RA+".md") && hasF(f, "결과: 고침 <sha>") && hasF(f, "mv"))
	k.check("받은편지 거절: 사유 한 줄 rejected/<이름>.why, 둘은 rejected/로", hasFx(IS+"/rejected/nopr.why", "설정에 없는 project: nope") &&
		cIsF(IS+"/rejected/nopr.req") && cIsF(IS+"/rejected/nopr.brief.md") && !exists(IS+"/nopr.req") && k.where("nopr") == "")
	k.check("받은편지 거절: 이름 글자·같은 이름·없는 only 기계·지시문 없음·빈 지시문", hasF(IS+"/rejected/Bad.X.why", "이름은") &&
		hasF(IS+"/rejected/dupl.why", "이미 대기열에 있는 이름") && hasF(IS+"/rejected/badonly.why", "box-9") &&
		hasF(IS+"/rejected/nobrief.why", "nobrief.brief.md가 없거나 비었다") && hasF(IS+"/rejected/emptyb.why", "emptyb.brief.md가 없거나 비었다") &&
		cIsF(IS+"/rejected/emptyb.req") && k.where("emptyb") == "" && k.cQn("dupl") == 1)
	RH := k.rid("inq-h")
	k.check("받은편지: ~/ 는 그 기계 HOME으로(shared)", cIsF(IH+"/taken/inq-h."+RH+".req") &&
		k.cQ11("inq-h") == "shared:~/.local/share/hc-fill/inbox/done/inq-h.md#"+RH)
	k.check("받은편지: 연결 안 된 기계는 건너뜀, 요청은 그대로", k.cO("받은편지 건너뜀 box-2: 연결 안 됨") && cIsF(IB+"/inq-off.req") &&
		k.where("inq-off") == "")
	k.check("되돌려 주기: 손으로 넣은 레인(dupl)의 run 지시문에는 결과 보고 절 없음", !hasF(k.DATA+"/runs/dupl-run.md", "결과 보고"))

	// 되돌려 주기
	k.lj("slow/inq-a:claude-code:inq-a:working", "box-1/inq-h:claude-code:inq-h:working")
	k.req(IH, "inq-h", "app", "-", "box-1")
	k.run("run")
	k.check("되돌려 주기: 보고가 없으면 쓰지 않음", !exists(IS+"/done/inq-a.md") && k.cState("inq-a") == "up" && k.lst(RA) == "open")
	k.check("받은편지: 진행 중인 이름으로 새 요청 → 거절 사유, 대기열 줄 하나", hasF(IH+"/rejected/inq-h.why", "이미 대기열에 있는 이름") &&
		!exists(IH+"/inq-h.req") && k.cQn("inq-h") == 1)
	write(k.t, RP+"/inq-a."+RA+".md", "쓰는 중\n")
	k.lj("slow/inq-a:claude-code:inq-a:done", "box-1/inq-h:claude-code:inq-h:working")
	k.run("run")
	k.check("되돌려 주기: 마지막 줄이 「결과: …」가 아닌 반쪽 보고는 보내지 않음(터미널이 살아 있으면 기다림)",
		!exists(IS+"/done/inq-a.md") && !exists(RPL+"/"+RA+".md") && k.lst(RA) == "open")
	write(k.t, RP+"/inq-a."+RA+".md", "고친 것 설명\n결과: 고침 abc12345\n")
	k.lj("slow/inq-a:claude-code:inq-a:idle", "box-1/inq-h:claude-code:inq-h:working")
	k.run("run")
	k.check("되돌려 주기: 완성 보고 → 터미널이 idle로 살아 있어도·착지가 없어도 요청 기계 done/<이름>.md에, 장부·대기열 sent",
		cCat(IS+"/done/inq-a.md") == cCat(RP+"/inq-a."+RA+".md") && k.cState("inq-a") == "sent" && k.lst(RA) == "sent" &&
			k.cO("되돌려 줌 inq-a → slow:"+IS+"/done/inq-a.md (결과: 고침 abc12345)") && hasE(k.T+"/hc.log", `^exec -i --on slow `),
		read(LED))
	k.check("되돌려 주기: 고정본 replies/<ID>.md, 요청자 표시 .hc-fill/<ID>.sent(해시)·owner/<이름>", cCmp(RPL+"/"+RA+".md", IS+"/done/inq-a.md") &&
		cCat(IS+"/done/.hc-fill/"+RA+".sent") == cSha(RPL+"/"+RA+".md") && cCat(IS+"/done/.hc-fill/owner/inq-a") == RA)
	rm(IS + "/done/inq-a.md")
	k.run("run")
	k.check("되돌려 주기: sent는 다시 쓰지 않음, ls에 되돌려줌", !exists(IS+"/done/inq-a.md") && func() bool {
		o, _ := k.exe(k.T, nil, k.FILL, "ls")
		return grep(o, `inq-a.*되돌려줌.*받은편지 slow`)
	}())
	k.req(IS, "inq-a", "app", "-", "slow")
	k.run("run")
	k.check("받은편지: 끝난(sent) 이름으로 새 요청 → 삼키지 않고 rejected/<이름>.why로 알림", hasF(IS+"/rejected/inq-a.why", "이미 끝난 이름") &&
		cIsF(IS+"/rejected/inq-a.req") && !exists(IS+"/inq-a.req") && k.cState("inq-a") == "sent")
	k.fill(nil, "rm", "inq-a")
	rm(IS + "/rejected")
	k.req(IS, "inq-a", "app", "-", "slow")
	k.lj()
	k.run("run")
	RA2 := k.rid("inq-a")
	k.check("다시 받음: 같은 이름 새 요청은 새 요청 ID, 보고 경로도 새 ID(옛 보고는 그대로 둠)", k.where("inq-a") == "up slow slow/inq-a" &&
		RA2 != "" && RA2 != RA && cIsF(RP+"/inq-a."+RA+".md") && hasF(k.DATA+"/runs/inq-a-run.md", "reports/inq-a."+RA2+".md"))
	k.lj("slow/inq-a:claude-code:inq-a:done", "box-1/inq-h:claude-code:inq-h:working")
	k.run("run")
	k.check("다시 띄움: 옛 요청의 보고를 새 요청 결과로 보내지 않음", !exists(IS+"/done/inq-a.md") && k.cState("inq-a") == "up" && k.lst(RA2) == "open")
	// 요청자 done/<이름>.md에 다른 요청의 다른 결과가 있으면 덮지 않고 held + 지휘 한 번
	write(k.t, IS+"/done/inq-a.md", "남의 결과\n")
	write(k.t, RP+"/inq-a."+RA2+".md", "두 번째\n결과: 넘김 사유\n")
	k.run("run")
	k.check("되돌려 주기: 그 자리에 다른 요청의 다른 결과가 있으면 덮지 않고 held, 지휘에 한 번(키 hf:<ID>:held)",
		cCat(IS+"/done/inq-a.md") == "남의 결과" && k.lst(RA2) == "held" && hasF(k.T+"/report.log", "--key hf:"+RA2+":held"), read(LED))
	k.mj("shared", "slow")
	k.run("run")
	k.check("되돌려 주기: 레인 기계가 연결 안 됨이면 사라진 것으로 보지 않고 미룸", !exists(IH+"/done/inq-h.md") && k.cState("inq-h") == "up")
	k.mj("box-1", "shared", "slow")
	k.lj()
	k.run("run")
	k.check("되돌려 주기: hc ls에 없어도 hc get이 not_found가 아니면 실패를 쓰지 않음", !exists(IH+"/done/inq-h.md") && k.cState("inq-h") == "up" &&
		grep(k.out, "되돌려 주기 미룸 inq-h"))
	k.set("HC_FAKE_FINISHED=box-1/inq-h")
	k.run("run")
	k.unset("HC_FAKE_FINISHED")
	k.check("되돌려 주기: 보고 없이 프로세스가 끝난(finished) 레인도 「결과: 실패 보고 없음」", cCat(IH+"/done/inq-h.md") == "결과: 실패 보고 없음" &&
		k.cState("inq-h") == "sent")

	// 완성 보고의 종류와 값이 모두 있어야 고정한다.
	k.ifresh()
	k.req(IS, "fmt", "app", "-", "slow")
	k.run("run")
	ri := k.rid("fmt")
	mkdirs(k.t, RP)
	k.lj("slow/fmt:claude-code:fmt:working")
	for _, last := range []string{"결과: 고침중", "결과: 고침 미정", "결과: 실패  ", "결과: 넘김"} {
		write(k.t, RP+"/fmt."+ri+".md", last+"\n")
		k.run("run")
		k.check("완성 전 결과 줄 보류("+last+")", k.lst(ri) == "open" && !exists(IS+"/done/fmt.md"))
	}
	write(k.t, RP+"/fmt."+ri+".md", "결과: 넘김 다음 담당자가 처리\n")
	k.run("run")
	k.check("완성된 이유를 쓰면 같은 요청에 한 번 회신", k.lst(ri) == "sent" && cCmp(RP+"/fmt."+ri+".md", IS+"/done/fmt.md"))

	// 종료 판정은 오류 종류와 현재 터미널의 이름·상태를 본다.
	k.ifresh()
	k.req(IS, "not_found", "app", "-", "slow")
	k.run("run")
	k.lj()
	k.run("run")
	k.check("정상 터미널의 이름 not_found를 종료 오류로 오인하지 않음", k.lst(k.rid("not_found")) == "open" && !exists(IS+"/done/not_found.md"))
	k.ifresh()
	k.req(IS, "listed", "app", "-", "slow")
	k.run("run")
	k.lj("slow/listed:claude-code:listed:done:/")
	k.runE([]string{"HC_FAKE_FINISHED=slow/listed"}, "run")
	k.check("목록에 남은 finished 터미널도 보고 없음으로 회신", hasF(IS+"/done/listed.md", "결과: 실패 보고 없음"))
	k.ifresh()
	k.req(IS, "old-in", "app", "-", "slow")
	k.run("run")
	k.lj("slow/old-in:claude-code:other:working:/")
	k.runE([]string{"HC_FAKE_GET_NAME=other"}, "run")
	k.check("목록과 hc get 모두 주소 재사용을 확인하면 옛 요청을 닫음", hasF(IS+"/done/old-in.md", "결과: 실패 보고 없음"))

	// 같은 주소와 이름이어도 created_ms가 바뀌면 옛 요청의 프로세스는 끝났다.
	k.ifresh()
	k.req(IS, "same-end", "app", "-", "slow")
	k.run("run")
	ri = k.rid("same-end")
	k.lj("slow/same-end:claude-code:same-end:working:/")
	k.runE([]string{"HC_FAKE_CREATED=" + "slow/same-end=1700000555000"}, "run") // 이어 붙임: 공개 내보내기 비밀 꼴(긴 토큰)에 안 걸리게
	k.check("같은 주소·이름의 새 프로세스가 떠 있어도 옛 요청은 종료로 판정", k.lst(ri) == "sent" && hasFx(IS+"/done/same-end.md", "결과: 실패 보고 없음"))

	// 반쪽 보고 + 터미널 사라짐 → 「결과: 실패 보고 불완전」
	k.ifresh()
	k.req(IS, "inq-g", "app", "-", "slow")
	k.run("run")
	RG := k.rid("inq-g")
	write(k.t, RP+"/inq-g."+RG+".md", "반쯤 썼다\n")
	k.lj()
	k.set("HC_FAKE_GONE=slow/inq-g")
	k.run("run")
	k.unset("HC_FAKE_GONE")
	k.check("되돌려 주기: 반쪽 보고 + 터미널 끝남 확정 → 그 내용에 「결과: 실패 보고 불완전」", lastLine(read(IS+"/done/inq-g.md")) == "결과: 실패 보고 불완전" &&
		hasF(IS+"/done/inq-g.md", "반쯤 썼다") && k.lst(RG) == "sent")

	// 중단 주입: 공개 뒤 로컬 기록 전에 죽음 → 다음 바퀴는 같은 고정본으로 ALREADY(요청자 파일을 다시 쓰지 않음), 장부 sent
	k.ifresh()
	k.req(IS, "inq-k", "app", "-", "slow")
	k.run("run")
	RK := k.rid("inq-k")
	write(k.t, RP+"/inq-k."+RK+".md", "결과: 고침 1234567\n")
	k.lj("slow/inq-k:claude-code:inq-k:done")
	k.runE([]string{"HC_FILL_CRASH_AT=after-publish"}, "run")
	k.check("중단 after-publish: 그 지점에서 죽음, 요청자에는 공개됨, 장부는 아직 deliver", hasFx(k.DATA+"/crash.log", "after-publish") &&
		cIsF(IS+"/done/inq-k.md") && k.lst(RK) == "deliver")
	i0 := ino(IS + "/done/inq-k.md")
	k.run("run")
	k.check("중단 뒤: 같은 요청·같은 내용으로 재시도 → ALREADY, 요청자 파일은 그대로(공개 1번), 장부·대기열 sent",
		ino(IS+"/done/inq-k.md") == i0 && k.lst(RK) == "sent" && k.cState("inq-k") == "sent", read(LED))
	// 같은 이름의 두 요청이 같은 본문을 냈어도 요청자 경로는 첫 요청 소유다.
	k.ifresh()
	k.req(IS, "inq-id", "app", "-", "slow")
	k.run("run")
	ri1 := k.rid("inq-id")
	write(k.t, RP+"/inq-id."+ri1+".md", "결과: 넘김 확인 필요\n")
	k.run("run")
	k.fill(nil, "rm", "inq-id")
	k.req(IS, "inq-id", "app", "-", "slow")
	k.run("run")
	ri2 := k.rid("inq-id")
	copyFile(k.t, RP+"/inq-id."+ri1+".md", RP+"/inq-id."+ri2+".md")
	k.run("run")
	k.check("같은 결과라도 다른 요청의 파일이면 둘째 요청은 held", ri2 != "" && ri1 != ri2 && k.lst(ri1) == "sent" && k.lst(ri2) == "held" &&
		!exists(IS+"/done/.hc-fill/"+ri2+".sent") && cCat(IS+"/done/.hc-fill/owner/inq-id") == ri1 && hasF(k.T+"/report.log", "hf:"+ri2+":held"))
	k.ifresh()
	k.req(IS, "old-file", "app", "-", "slow")
	k.run("run")
	ri = k.rid("old-file")
	write(k.t, RP+"/old-file."+ri+".md", "결과: 넘김 확인 필요\n")
	copyFile(k.t, RP+"/old-file."+ri+".md", IS+"/done/old-file.md")
	oldInode := ino(IS + "/done/old-file.md")
	k.run("run")
	k.check("소유 표시 없는 예전 결과는 같은 해시면 그대로 회수", k.lst(ri) == "sent" && ino(IS+"/done/old-file.md") == oldInode &&
		cCat(IS+"/done/.hc-fill/owner/old-file") == ri)

	// 고정 뒤 장부 전에 죽음 → 다음 바퀴가 그 고정본으로 이어 감(보고가 그 사이 바뀌어도 처음 고정본)
	k.ifresh()
	k.req(IS, "inq-once", "app", "-", "slow")
	k.run("run")
	RO := k.rid("inq-once")
	write(k.t, RP+"/inq-once."+RO+".md", "결과: 고침 abc1234\n")
	writeX(k.t, k.T+"/receipt-bin/mv", `#!/bin/bash
/bin/mv "$@" || exit 1
if [ "${!#}" = "$RECEIPT_PATH" ]; then /bin/mv "$RECEIPT_PATH" "$RECEIPT_PATH.read"; exit 1; fi
`)
	k.runE([]string{"RECEIPT_PATH=" + IS + "/done/inq-once.md", k.cPath(k.T + "/receipt-bin")}, "run")
	k.check("공개 직후 중단: 요청자가 결과를 먼저 수거하고 보내는 쪽에는 완료 표시가 없음", nonEmpty(IS+"/done/inq-once.md.read") &&
		!exists(IS+"/done/.hc-fill/"+RO+".sent") && k.lst(RO) == "deliver")
	k.run("run")
	k.check("공개 직후 중단: 이미 수거한 결과를 다시 만들지 않고 sent 복구", !exists(IS+"/done/inq-once.md") &&
		nonEmpty(IS+"/done/.hc-fill/"+RO+".sent") && k.lst(RO) == "sent")

	k.ifresh()
	k.req(IS, "inq-f", "app", "-", "slow")
	k.run("run")
	RF := k.rid("inq-f")
	write(k.t, RP+"/inq-f."+RF+".md", "결과: 고침 aaaaaaa\n")
	k.lj("slow/inq-f:claude-code:inq-f:done")
	k.runE([]string{"HC_FILL_CRASH_AT=after-reply-fix"}, "run")
	write(k.t, RP+"/inq-f."+RF+".md", "결과: 고침 bbbbbbb\n")
	k.run("run")
	k.check("중단 after-reply-fix: 고정본이 확정본 — 다음 바퀴에 처음 내용으로 공개(장부 sent)", cCat(IS+"/done/inq-f.md") == "결과: 고침 aaaaaaa" &&
		k.lst(RF) == "sent")
	k.check("고정 뒤 중단해도 처음 결과의 종류 보존", cCut(read(LED), "\t", 5) == "result")
	// 잘린 입력(장부 크기 다름) → BADINPUT, 공개 0
	k.ifresh()
	k.req(IS, "inq-b", "app", "-", "slow")
	k.run("run")
	RB := k.rid("inq-b")
	write(k.t, RP+"/inq-b."+RB+".md", "결과: 고침 ccccccc\n")
	k.lj("slow/inq-b:claude-code:inq-b:done")
	k.runE([]string{"HC_FILL_CRASH_AT=after-reply-ledger"}, "run")
	k.cRewrite(LED, func(r []string) []string {
		if col(r, 1) == RB {
			for len(r) < 7 {
				r = append(r, "")
			}
			n, _ := strconv.Atoi(r[6])
			r[6] = strconv.Itoa(n + 5)
		}
		return r
	})
	k.run("run")
	k.check("공개: 받은 내용 크기·해시가 다르면 BADINPUT — 공개 안 함, 다음 바퀴에 다시", !exists(IS+"/done/inq-b.md") && k.lst(RB) == "deliver" &&
		k.cO("BADINPUT"))
}
