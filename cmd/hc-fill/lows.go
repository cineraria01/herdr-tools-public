package main

// 감사 [낮음] 후속 레인(go-lows-followup, 2026-11-22 사용자 「채울일이 있으면 자동 준비 되는것도 고로 넣어줘」, Go 판만):
// 자동 감사가 통과(높음·중간 0)인데 낮음이 있으면 aQueued가 차수 줄 16칸에 lows=<n>을 적고, run 바퀴가 auditTick 다음에
// lowsTick을 부른다 — 그 보고에서 낮음 원문을 뽑아 <레인>-lows를 넣는다(같은 프로젝트·주인, 선행 <레인>이라 착지 뒤에만 뜸,
// 먼저 띄움 끔, 우선 낮음, 자동 감사는 다른 레인과 같이). 처리한 차수는 16칸에 lowsq(넣음)·lowsx(안 넣음)를 더해 다시 보지 않는다.
// 설정: lows <프로젝트…|*>(켜기, 빼면 끔) · lows_hour(시간당 상한, 기본 3) · lows_min(낮음 최소 개수, 기본 1)

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// lowsN: 16칸 표시에서 lows=<n>(없으면 0)과 처리 끝남(lowsq·lowsx)
func lowsN(afl string) (n int, done bool) {
	for _, f := range strings.Split(afl, ",") {
		switch {
		case f == "lowsq" || f == "lowsx":
			done = true
		case strings.HasPrefix(f, "lows=") && isDigits(f[5:]):
			n = atoi10(f[5:])
		}
	}
	return n, done
}

// addFlag: 쉼표 목록 16칸에 하나 더한다(- 이면 그것만)
func addFlag(afl, f string) string {
	if afl == "" || afl == "-" {
		return f
	}
	return afl + "," + f
}

// lowsOn <프로젝트>: 설정 lows 줄에 그 프로젝트나 *가 있나
func lowsOn(p string) bool {
	for _, x := range strings.Fields(conf("lows")) {
		if x == p || x == "*" {
			return true
		}
	}
	return false
}

// lowsName <레인>: <레인>-lows, 32자를 넘으면 <앞 22자>-<cksum 4자리 16진>-lows
func lowsName(lane string) string {
	if n := lane + "-lows"; len([]rune(n)) <= 32 {
		return n
	}
	return fmt.Sprintf("%s-%04x-lows", runeCut(lane, 22), cksum([]byte(lane))%65536)
}

// lowsRecent: lows.tsv(넣은시각(초) 차수 레인 후속 n)에서 지난 한 시간에 넣은 수
func lowsRecent() int {
	k, now := 0, nowUnix()
	for _, r := range readRows(DATA + "/lows.tsv") {
		if t, err := strconv.ParseInt(r.F(1), 10, 64); err == nil && now-t < 3600 {
			k++
		}
	}
	return k
}

// lowsBrief: 후속 지시문 — 낮음 원문 + 처리 규칙 + 원 지시문 자리. 착지·감사 절은 띄울 때 hc-fill·hc-lane이 붙인다
func lowsBrief(name, lane, aid, m, rep string, items []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 레인 %s — %s 자동 감사(%s)가 남긴 [낮음] %d개 후속 (hc-fill 자동)\n\n", name, lane, aid, len(items))
	fmt.Fprintf(&b, "%s는 감사 통과(높음·중간 0)로 착지했다. 그 감사가 「공개를 막지 않는다」고 남긴 [낮음] 지적을 처리한다.\n\n", lane)
	b.WriteString("## 할 것\n")
	b.WriteString("- 지적마다 고치거나, 고칠 필요가 없으면 까닭 한 줄. 고칠 때는 고치기 전 코드에서 실패하는 시험을 먼저 둔다(시험할 수 있는 것만).\n")
	b.WriteString("- 보고(최종 메시지)에 지적마다 「고침 <커밋>」이나 「안 고침: <까닭>」.\n")
	b.WriteString("- 하나도 고치지 않으면 커밋 없이 까닭만 보고하고 `hc-lane report 완료 - \"커밋 0 — 낮음 모두 안 고침\"`(지휘가 본다).\n")
	b.WriteString("- 저장소 공통 규칙(커밋 제목 머리 `" + name + ":`, 시험·검증, 개인 값 금지 등)은 원 레인 지시문을 따른다: " +
		m + " 기계 `~/.local/share/hc-fill/runs/" + lane + "-run.md`(다른 기계면 `hc file read --on " + m + " …`).\n\n")
	fmt.Fprintf(&b, "## 지적 원문 (감사 보고 %s:%s)\n", m, rep)
	for i, it := range items {
		fmt.Fprintf(&b, "\n### 지적 %d\n\n%s\n", i+1, it)
	}
	return b.String()
}

// sLowsRead: 보고 파일을 찍는다. 없으면 NOFILE 한 줄
const sLowsRead = `[ -f "$d" ] || { echo NOFILE; exit 0; }; echo FILE; cat "$d"`

// lowsTick: 통과 차수의 낮음 후속 넣기(dry-run은 찍기만). 다시 볼 일(연결 끊김·상한·읽기 실패)은 표시를 안 남겨 다음 바퀴에
func lowsTick() {
	if !nonEmpty(AT) || strings.TrimSpace(conf("lows")) == "" {
		return
	}
	hour, lmin := lowsConf("lows_hour", 3), lowsConf("lows_min", 1)
	for _, r := range readRows(AT) {
		if r.NF() < 17 || r.F(10) != "pass" {
			continue
		}
		n, done := lowsN(r.F(16))
		if n == 0 || done {
			continue
		}
		aid, lane, m, rep := r.F(1), r.F(2), r.F(6), r.F(17)
		name := lowsName(lane)
		skip := func(why string) { // 안 넣음 — 표시를 남겨 다시 보지 않는다
			fmt.Println("후속 안 넣음 " + lane + ": " + why)
			if !dry {
				aupd(aid, "16", addFlag(r.F(16), "lowsx"))
			}
		}
		q := qrow(lane)
		switch {
		case strings.HasSuffix(lane, "-lows"):
			skip("이미 후속 레인(사슬 막기)")
			continue
		case strings.HasPrefix(lane, "audit-"):
			skip("감사 줄")
			continue
		case q == nil:
			skip("대기열에 그 레인 줄이 없다(주인·프로젝트 모름)")
			continue
		case q.F(15) == "no-audit" || strings.HasPrefix(q.F(15), "audit="):
			skip("자동 감사 끈 줄")
			continue
		}
		if _, _, inbox := replyOf(q.F(11)); inbox {
			skip("받은편지 레인")
			continue
		}
		p, own := q.F(2), qown(lane)
		switch {
		case !lowsOn(p):
			skip("프로젝트 " + p + " 꺼짐(설정 lows)")
			continue
		case n < lmin:
			skip(fmt.Sprintf("낮음 %d개 < lows_min %d", n, lmin))
			continue
		}
		bf := DATA + "/briefs/lows/" + name + ".md"
		if x := qrow(name); x != nil || thenof(name) != "" {
			if x != nil && x.F(3) == bf { // 넣은 뒤 표시 전에 끊겼다 — 표시·알림만 잇는다
				lowsDone(aid, r.F(16), lane, name, own, n)
			} else {
				skip("같은 이름 " + name + " 이 이미 대기열에 있다")
			}
			continue
		}
		if hour > 0 && lowsRecent() >= hour {
			fmt.Printf("후속 미룸 %s: 시간당 상한 lows_hour %d\n", lane, hour)
			continue
		}
		if !auOK(m) {
			fmt.Println("후속 미룸 " + lane + ": " + m + " 연결 안 됨")
			continue
		}
		out, rc := onm(nil, m, sLowsRead, rep)
		if rc != 0 {
			fmt.Println("후속 미룸 " + lane + ": 보고를 읽지 못함 " + m + ":" + rep)
			continue
		}
		var items []string
		if strings.HasPrefix(out, "FILE\n") {
			items = lowsItems(strings.TrimPrefix(out, "FILE\n"))
		}
		if len(items) != n {
			why := fmt.Sprintf("보고에서 낮음 %d개 중 %d개를 뽑음", n, len(items))
			if !strings.HasPrefix(out, "FILE\n") {
				why = "보고 파일 없음"
			}
			if dry {
				fmt.Println(P + "후속 못 넣음 " + lane + ": " + why)
				continue
			}
			if hcLaneReport("중간", "후속 못 넣음 "+name+": "+why+" — 손으로 ("+m+":"+rep+")", lane, dash0(own), "hf:"+aid+":lows") != 0 {
				fmt.Println("후속 알림 실패 " + lane + " — 다음 바퀴에")
				continue
			}
			skip(why)
			continue
		}
		if dry {
			fmt.Printf("%s넣을 후속 %s ← %s (낮음 %d개, 선행 %s, 우선 낮음)\n", P, name, aid, n, lane)
			continue
		}
		if os.MkdirAll(filepath.Dir(bf), 0o755) != nil || writeAtomic(bf, lowsBrief(name, lane, aid, m, rep, items)) != nil {
			fmt.Println("후속 미룸 " + lane + ": 지시문을 쓰지 못함 " + bf)
			continue
		}
		if qAdd("", name, p, bf, lane, "-", "wait", "-", "-", "-", nowMin(), "-", own, "low", "no-early", "-", "-") != 0 {
			fmt.Println("후속 미룸 " + lane + ": 대기열에 넣지 못함")
			continue
		}
		lowsDone(aid, r.F(16), lane, name, own, n)
	}
}

// lowsDone: 넣은 기록(lows.tsv)·표시(lowsq)·주인에 한 줄
func lowsDone(aid, afl, lane, name, own string, n int) {
	if err := appendLine(DATA+"/lows.tsv", strings.Join([]string{strconv.FormatInt(nowUnix(), 10), aid, lane, name, strconv.Itoa(n)}, "\t")); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
	}
	aupd(aid, "16", addFlag(afl, "lowsq"))
	msg := fmt.Sprintf("후속 넣음 %s: 낮음 %d개 (빼려면 hc-fill rm %s)", name, n, name)
	fmt.Println(msg)
	if hcLaneReport("중간", msg, lane, dash0(own), "hf:"+aid+":lows") != 0 {
		fmt.Println("후속 알림 실패 " + lane + "(hc-lane report) — 대기열에는 넣음")
	}
}

// dash0: - 는 빈 값(hcLaneReport의 --to 없음 = 모든 지휘)
func dash0(s string) string {
	if s == "-" {
		return ""
	}
	return s
}

// lowsConf: 설정 키의 개수 값(없으면 기본, 숫자가 아니면 알림 한 줄 뒤 기본)
func lowsConf(k string, def int) int {
	v := conf(k)
	if v == "" {
		return def
	}
	if !isDigits(v) {
		fmt.Printf("알림: 설정 %s는 개수(숫자)다, %d로: %s\n", k, def, v)
		return def
	}
	return atoi10(v)
}
