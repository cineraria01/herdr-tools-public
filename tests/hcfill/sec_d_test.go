package hcfill_test

// 셸 시험 10b(지시문 사본)·10d(주인 지휘)·10e(우선순위).

import (
	"fmt"
	"os"
	"strings"
)

func init() { reg(40, "d", secD) }

// dq: awk -F'\t' '$1==n {print $c}' 대기열 — 맞는 줄마다 한 줄.
func (k *K) dq(name string, c int) string {
	var l []string
	for _, r := range rows(k.Q) {
		if col(r, 1) == name {
			l = append(l, col(r, c))
		}
	}
	return strings.Join(l, "\n")
}

// dCat: $(cat 파일) — 끝 줄바꿈을 뺀 글.
func dCat(p string) string { return strings.TrimRight(read(p), "\n") }

// dPaste: paste -d" " a b | tr "\n" , .
func dPaste(a, b string) string {
	la, lb := lines(read(a)), lines(read(b))
	n := len(la)
	if len(lb) > n {
		n = len(lb)
	}
	var s strings.Builder
	for i := 0; i < n; i++ {
		x, y := "", ""
		if i < len(la) {
			x = la[i]
		}
		if i < len(lb) {
			y = lb[i]
		}
		s.WriteString(x + " " + y + ",")
	}
	return s.String()
}

// dTail3: 칸 수 · 주인 · 우선.
func (k *K) dTail3(name string) string {
	var l []string
	for _, r := range rows(k.Q) {
		if col(r, 1) == name {
			l = append(l, fmt.Sprintf("%d %s %s", len(r), col(r, 12), col(r, 13)))
		}
	}
	return strings.Join(l, "\n")
}

// dPrioOf: 열셋째 칸(비면 -).
func (k *K) dPrioOf(name string) string {
	var l []string
	for _, r := range rows(k.Q) {
		if col(r, 1) == name {
			v := col(r, 13)
			if v == "" {
				v = "-"
			}
			l = append(l, v)
		}
	}
	return strings.Join(l, "\n")
}

// dNames: 띄운 차례 — lane.log에서 --name 다음 줄들, 이웃 겹침을 뺀 뒤 낱말마다 뒤에 공백.
func (k *K) dNames() string {
	var got []string
	p := false
	for _, l := range lines(read(k.T + "/lane.log")) {
		if p {
			got = append(got, l)
			p = false
		}
		if l == "--name" {
			p = true
		}
	}
	var s strings.Builder
	for i, x := range got {
		if i > 0 && got[i-1] == x {
			continue
		}
		s.WriteString(x + " ")
	}
	return s.String()
}

func secD(k *K) {
	// # 10b. 지시문은 add 때 데이터 폴더로 복사, 못 읽는·빈 지시문은 add·run 모두 막는다
	k.fresh()
	k.run("add", "--name", "cp1", "--project", "app", "--brief", k.brief("cp1"))
	CP1 := k.DATA + "/briefs/added/cp1.md"
	k.check("add: 지시문 내용을 briefs/added/<레인>.md로 복사, 대기열은 그 경로",
		k.rc == 0 && hasF(CP1, "# 지시문 cp1") && k.dq("cp1", 3) == CP1)
	write(k.t, k.T+"/cp1.md", "# 바뀐 원본\n")
	k.run("add", "--name", "cp1", "--project", "app", "--brief", k.T+"/cp1.md")
	others := false
	if ents, err := os.ReadDir(k.DATA + "/briefs/added"); err == nil {
		for _, e := range ents {
			if e.Name() != "cp1.md" {
				others = true
			}
		}
	}
	k.check("add: 같은 이름 거절은 이미 넣은 사본을 덮지 않음", k.rc == 1 && hasF(CP1, "# 지시문 cp1") && !others)
	k.brief("nr")
	os.Chmod(k.T+"/nr.md", 0)
	k.run("add", "--name", "nr", "--project", "app", "--brief", k.T+"/nr.md")
	os.Chmod(k.T+"/nr.md", 0o600)
	k.check("add: 읽지 못하는 지시문 거절(대기열·사본 없음)", k.rc == 1 && k.where("nr") == "" &&
		!exists(k.DATA+"/briefs/added/nr.md") && strings.Contains(k.out, "지시문을 읽지 못함"))
	write(k.t, k.T+"/emp.md", "")
	k.run("add", "--name", "emp", "--project", "app", "--brief", k.T+"/emp.md")
	k.check("add: 빈 지시문 거절", k.rc == 1 && k.where("emp") == "" && strings.Contains(k.out, "지시문을 읽지 못함(빈 파일)"))
	write(k.t, k.T+"/ws.md", " \n\t\n")
	k.run("add", "--name", "ws", "--project", "app", "--brief", k.T+"/ws.md")
	k.check("add: 공백·줄바꿈만 든 지시문도 빈 것으로 거절", k.rc == 1 && k.where("ws") == "" && strings.Contains(k.out, "지시문을 읽지 못함(빈 파일)"))
	// 옛 꼴(데이터 폴더 밖 경로) 줄: 못 읽거나 비었으면 띄우지 않고 다음 레인으로
	k.fresh()
	k.brief("ob1")
	write(k.t, k.T+"/ob2.md", "")
	k.brief("ob3")
	os.Chmod(k.T+"/ob1.md", 0)
	write(k.t, k.Q, "ob1\tapp\t"+k.T+"/ob1.md\t-\t-\twait\t-\t-\t-\t2026-01-01T00:00\t-\n"+
		"ob2\tapp\t"+k.T+"/ob2.md\t-\t-\twait\t-\t-\t-\t2026-01-01T00:00\t-\n"+
		"ob3\tapp\t"+k.T+"/ob3.md\t-\t-\twait\t-\t-\t-\t2026-01-01T00:00\t-\n")
	k.run("run")
	os.Chmod(k.T+"/ob1.md", 0o600)
	k.check("run: 읽지 못하는 지시문은 띄우지 않고 실패로 남김", k.where("ob1") == "wait - -" && k.launched("ob1") == 0 &&
		strings.Contains(k.out, "실패 ob1: 지시문을 읽지 못함("))
	k.check("run: 빈 지시문도 띄우지 않음", k.where("ob2") == "wait - -" && k.launched("ob2") == 0 &&
		strings.Contains(k.out, "실패 ob2: 지시문을 읽지 못함(빈 파일)"))
	k.check("run: 다음 레인은 그대로 띄우고 run 지시문에 본문이 들어감", k.where("ob3") == "up box-1 box-1/ob3" &&
		hasF(k.DATA+"/runs/ob3-run.md", "# 지시문 ob3"))

	// # 10d. 주인 지휘(2026-11-17 hcfill-owner)
	k.fresh()
	k.runE([]string{"HC_TERMINAL_ID=tq1"}, "add", "--name", "ow1", "--project", "app", "--brief", k.brief("ow1"))
	k.check("주인: add가 부른 지휘를 열두째 칸에 적고 hc-lane coord --set으로 지휘 정보를 남김",
		k.rc == 0 && k.dq("ow1", 12) == "tq1" && dCat(k.T+"/coordset.log") == "tq1", read(k.Q))
	k.run("add", "--name", "ow2", "--project", "app", "--brief", k.brief("ow2"), "--coord", "tq2")
	k.check("주인: --coord로 바꿈", k.rc == 0 && k.dq("ow2", 12) == "tq2")
	k.add("ow3")
	k.check("주인: 터미널 밖 add는 주인 없음(-)", k.dq("ow3", 12) == "-")
	// 재감사 R-09: 주인 없이 넣는 것은 지휘가 아님이 hc get으로 확인될 때만 — t5ln은 레인 라벨(hc-lane=1)이 붙은 터미널
	k.wrap("hc", `if [ "$1" = get ] && [ "$2" = t5ln ]; then echo '{"address":"t5ln","created_ms":1700000000000,"name":"lane","state":"live","labels":{"hc-lane":"1"},"status":null}'; exit 0; fi`)
	k.runE([]string{"HC_FAKE_COORD_SET_FAIL=t5ln", "HC_TERMINAL_ID=t5ln"}, "add", "--name", "ow4", "--project", "app", "--brief", k.brief("ow4"))
	k.unwrap("hc")
	k.check("주인: 지휘 터미널이 아니면(레인 안) 주인 없이 넣고 한 줄", k.rc == 0 && k.dq("ow4", 12) == "-" && strings.Contains(k.out, "주인 없이"))
	k.runE([]string{"HC_FAKE_COORD_SET_FAIL=t5ln"}, "add", "--name", "ow5", "--project", "app", "--brief", k.brief("ow5"), "--coord", "t5ln")
	k.check("주인: --coord가 지휘가 아니면 거절", k.rc == 1 && k.where("ow5") == "")
	k.run("ls")
	k.check("ls: 주인 칸", k.rc == 0 && grep(k.out, "^ow1 .*tq1") && grep(k.out, "^ow2 .*tq2") &&
		strings.Contains(strings.SplitN(k.out, "\n", 2)[0], "주인"))
	k.run("run")
	k.check("띄울 때 주인을 HC_LANE_OWNER·HC_LANE_COORD로(주인 없는 줄은 hc-lane coord 지휘 t8kk, 주인 없음)",
		dPaste(k.T+"/owner.log", k.T+"/coord.log") == "tq1 tq1,tq2 tq2,없음 t8kk,없음 t8kk,", dPaste(k.T+"/owner.log", k.T+"/coord.log"))
	k.fresh()
	k.runE([]string{"HC_TERMINAL_ID=tq1"}, "add", "--name", "ow6", "--project", "app", "--brief", k.brief("ow6"), "--only", "here")
	k.run("run")
	k.check("로컬 레인은 주인 지휘 옆에 붙임(herdr-balance --main <주인>)", hasFx(k.T+"/balance.log", "--main tq1 --add ow6"), read(k.T+"/balance.log"))
	k.fresh()
	k.brief("ow7")
	write(k.t, k.Q, "ow7\tapp\t"+k.T+"/ow7.md\t-\t-\twait\t-\t-\t-\t2026-01-01T00:00\t-\n")
	k.run("run")
	k.check("11칸 옛 줄은 주인 없음으로 띄움", k.where("ow7") == "up box-1 box-1/ow7" && dCat(k.T+"/owner.log") == "없음")
	k.check("11칸 옛 줄은 주인 없음으로 띄움(주인 확인 안 함)", !exists(k.T+"/coordcheck.log"))
	// 감사 [중간] 1(a): 띄우기 전에 주인 지휘가 끝났으면 주인 없이, watch는 살아 있는 지휘에게
	k.fresh()
	k.runE([]string{"HC_TERMINAL_ID=tq1"}, "add", "--name", "ow8", "--project", "app", "--brief", k.brief("ow8"))
	k.runE([]string{"HC_FAKE_COORD_GONE=tq1", "HC_FAKE_GONE=tq1"}, "run") // 재감사 R-09: 끝남은 hc get(not_found)도 확인해야
	k.check("주인 지휘가 띄우기 전에 사라지면 주인 없이(-) 띄우고 watch는 hc-lane coord 지휘에게, 한 줄 알림",
		k.where("ow8") == "up box-1 box-1/ow8" && dCat(k.T+"/owner.log") == "없음" && dCat(k.T+"/coord.log") == "t8kk" &&
			dCat(k.T+"/coordcheck.log") == "tq1" && strings.Contains(k.out, "주인 지휘 tq1 가 지금 없다"),
		dPaste(k.T+"/owner.log", k.T+"/coord.log"))

	// # 10e. 우선순위(2026-11-17 hcfill-priority)
	k.fresh()
	k.add("n1")
	k.add("lo", "--priority", "낮음")
	k.add("h1", "--priority", "high")
	k.add("u1", "--priority", "긴급")
	k.add("n2", "--priority", "normal")
	k.add("h2", "--priority", "높음")
	k.check("우선순위 add: 한·영 값을 열셋째 칸에 urgent|high|normal|low로, 안 주면 normal",
		strings.Join([]string{k.dPrioOf("n1"), k.dPrioOf("lo"), k.dPrioOf("h1"), k.dPrioOf("u1"), k.dPrioOf("n2"), k.dPrioOf("h2")}, " ") ==
			"normal low high urgent normal high", read(k.Q))
	k.run("run")
	k.check("우선순위 차례: 긴급 > 높음 > 보통 > 낮음, 같은 값은 넣은 차례(가장 높은 것이 첫 기계)",
		k.dNames() == "u1 h1 h2 n1 n2 lo " && k.where("u1") == "up box-1 box-1/u1", "차례: "+k.dNames())
	k.run("add", "--name", "bad", "--project", "app", "--brief", k.brief("bad"), "--priority", "아주")
	k.check("우선순위 add: 모르는 값 거절(대기열 없음)", k.rc != 0 && k.where("bad") == "")
	k.run("ls")
	k.check("ls: 우선 칸(한국어)", k.rc == 0 && strings.Contains(strings.SplitN(k.out, "\n", 2)[0], "우선") &&
		grep(k.out, "^u1 .*긴급") && grep(k.out, "^lo .*낮음"))

	k.fresh()
	k.add("hb", "--priority", "high", "--only", "slow", "--after", "nope")
	k.add("ll", "--priority", "low", "--only", "slow")
	k.run("run")
	k.check("우선순위: 선행이 안 들어간 높은 것은 건너뛰고 들어간(없는) 낮은 것을 띄움", k.state("hb") == "wait" && k.where("ll") == "up slow slow/ll")

	k.fresh()
	k.lj("slow/x:claude-code:x")
	k.add("wn", "--only", "slow")
	k.add("wu", "--only", "slow", "--priority", "urgent")
	k.run("run")
	k.check("긴급: 자리가 없으면 기다림(돌던 것을 끊지 않음)", k.state("wu") == "wait" && k.state("wn") == "wait" && !hasE(k.T+"/hc.log", "^(kill|stop)"))
	k.lj()
	k.run("run")
	k.check("긴급: 다음 빈자리를 먼저 넣은 보통보다 먼저 잡음", k.where("wu") == "up slow slow/wu" && k.state("wn") == "wait")

	k.fresh()
	k.brief("od1")
	k.brief("od2")
	var b strings.Builder // 둘 다 --only slow(자리 1)
	for _, r := range [][]string{
		strings.Split("od1\tapp\t"+k.T+"/od1.md\t-\t-\twait\t-\t-\t-\t2026-01-01T00:00\t-", "\t"),
		strings.Split("od2\tapp\t"+k.T+"/od2.md\t-\t-\twait\t-\t-\t-\t2026-01-01T00:00\t-\ttq9", "\t"),
	} {
		r[4] = "slow"
		b.WriteString(strings.Join(r, "\t") + "\n")
	}
	write(k.t, k.Q, b.String())
	k.add("oh", "--only", "slow", "--priority", "high")
	k.run("ls")
	k.check("옛 줄(11·12칸): ls에 보통", k.rc == 0 && grep(k.out, "^od1 .*보통") && grep(k.out, "^od2 .*보통"))
	k.run("run")
	k.check("옛 줄(11·12칸)은 보통: 뒤에 넣은 높음이 먼저", k.where("oh") == "up slow slow/oh" && k.state("od1") == "wait", read(k.Q))
	k.run("prio", "od2", "urgent")
	k.check("prio: 12칸 옛 줄도 열셋째 칸에 적고 주인은 그대로", k.rc == 0 && k.dPrioOf("od2") == "urgent" && k.dq("od2", 12) == "tq9", read(k.Q))
	k.run("prio", "od1", "긴급")
	k.check("prio: 11칸 옛 줄은 주인 칸을 -로 채우고 열셋째 칸에", k.rc == 0 && k.dTail3("od1") == "13 - urgent", read(k.Q))
	k.run("prio", "oh", "low")
	k.check("prio: 떠 있는 레인도 바꾼다(큐 메타는 낸 것이 있을 때만 — fill-prio-pass)", k.rc == 0 && k.dPrioOf("oh") == "low", k.out)
	k.run("prio", "nope", "high")
	k.check("prio: 없는 레인은 종료 1", k.rc == 1)
	k.run("prio", "od1", "아주")
	k.check("prio: 모르는 값은 거절, 그대로", k.rc != 0 && k.dPrioOf("od1") == "urgent")

	k.fresh()
	k.add("pn1", "--only", "slow")
	k.add("pn2", "--only", "slow")
	k.run("prio", "pn2", "높음")
	k.run("run")
	k.check("prio: 바꾼 레인이 먼저 뜸", k.where("pn2") == "up slow slow/pn2" && k.state("pn1") == "wait")

	k.ifresh()
	k.mj("box-1", "shared", "slow")
	k.req(k.IS(), "iq-d", "app")
	k.req(k.IS(), "iq-l", "app")
	appendFile(k.t, k.IS()+"/iq-l.req", "priority\t낮음\n")
	k.req(k.IS(), "iq-b", "app")
	appendFile(k.t, k.IS()+"/iq-b.req", "priority\t아주\n")
	k.run("run")
	k.check("받은편지: priority 없으면 높음, 적혀 있으면 그 값, 모르는 값은 거절", k.dPrioOf("iq-d") == "high" && k.dPrioOf("iq-l") == "low" &&
		k.where("iq-b") == "" && hasF(k.IS()+"/rejected/iq-b.why", "priority"), read(k.Q))
}
