package main

import (
	"errors"
	"io/fs"
	"os"
	"sort"
	"strings"
)

// watchOwners: 열린 줄(watchOpen)이 있는 주인 지휘마다 hc-lane coord --watch — 그 지휘의 watch가 한가로 끝났으면 다시 띄운다
// (watch-idle-open, 2026-11-24: 마지막 제출에서 결과까지 60분 넘은 착지가 30% — 그사이 watch가 끝나 알림이 늦거나 다른 지휘에게 갔다).
// 감사 줄은 그 레인 줄이 센다. 지휘 확인·에이전트 확인·같은 지휘로 하나만은 hc-lane이 한다. 결과는 띄웠을 때만 한 줄.
// watch-need-live(2026-11-24): 지휘 터미널이 이 바퀴 표에 없으면(사라진 흔적) 부르지 않고, hc-lane이 「지휘 아님」(종료 1 — coordCheck
// 없음: 에이전트가 끝나 셸만 남음·pid·이름 다름 등)으로 거절한 지휘는 그 터미널의 hc ls 지문이 그대로인 동안 다시 부르지 않는다(watchDead) —
// 며칠 전 끝난 셸만 남은 지휘마다 매 분 헛호출이 났다
func watchOwners() {
	last := watchLast()
	dead := watchDead()
	keep := map[string]string{}
	seen := map[string]bool{}
	for _, r := range qRows() {
		o := r.F(qOwner)
		if o == "" || o == "-" || seen[o] || strings.HasPrefix(r.F(qAudit), "audit=") {
			continue
		}
		if !watchOpen(r, last[[2]string{r.F(qName), r.F(qRID)}], tickLive(r.F(qAddr), r.F(qName))) {
			continue
		}
		seen[o] = true
		fp, there := "", false
		for _, t := range TERMS {
			if ownerAddrIs(t.Addr, o) {
				fp, there = ownerFP(t.Status, t.Name, o), true
			}
		}
		switch {
		case !there && lsGone(o): // 사라진 흔적 — hc-lane도 「터미널이 없거나 끝남」으로 거절한다
			continue
		case there && dead[o] == fp:
			keep[o] = fp
			continue
		}
		out, rc := (Cmd{Name: "hc-lane", Args: []string{"coord", "--watch", o}, Err: errDrop}).Run()
		if strings.Contains(out, "띄움") {
			say("감시: 주인 지휘 %s 의 watch를 다시 띄움(열린 레인 있음)", o)
		}
		if rc == 1 && there {
			keep[o] = fp
		}
	}
	watchDeadSave(dead, keep)
}

// watchOpen: 감시가 받아야 할 열린 줄인가 — stateWatch(상태 한 장의 Need)와 watchOwners가 함께 쓴다.
// 주차한 대기 줄은 열림. 떠 있음 줄은 터미널이 끝났으면(live "" — ls·status의 「끝남」, stLive와 같은 값) 닫힘, 마지막 보고(같은 줄 ID)가
// 착지·판정·정리됨이면 닫힘. 보고 없음·모름은 터미널이 살아 있거나 모를 때(「?…」)만 열림(놓치지 않기).
// ponytail: 제출 뒤 레인 터미널이 끝난 줄도 닫힘 — 병합기 결과 사건은 사라지지 않고 그 지휘의 다음 watch·wait가 받는다(늦을 수 있다)
func watchOpen(r, last Row, live string) bool {
	switch r.F(qState) {
	case "wait":
		return rowMode(r) == mParked
	case "up":
	default:
		return false
	}
	if live == "" {
		return false
	}
	if last != nil && last.F(lsRID) == r.F(qRID) {
		switch last.F(lsState) {
		case "착지", "판정", "정리됨":
			return false
		}
	}
	return true
}

// watchLast: last.tsv(run 바퀴의 마지막 보고)를 레인·줄 ID로
func watchLast() map[[2]string]Row {
	m := map[[2]string]Row{}
	for _, l := range readRows(DATA + "/last.tsv") {
		m[[2]string{l.F(lsName), l.F(lsRID)}] = l
	}
	return m
}

// tickLive: run 바퀴 표(TERMS)로 본 그 줄 터미널 — stLive와 같은 뜻(빈 값 끝남, 「?」 모름, 그 밖 살아 있음의 상태).
// 주소가 표에 있는데 이름이 다르면 재사용된 남의 터미널(끝남). 표에 없으면 표를 믿을 수 있을 때만 끝남(lsGone — 띄운 뒤·끊긴 기계는 모름)
func tickLive(addr, name string) string {
	for _, t := range TERMS {
		if t.Addr == addr {
			if t.Name == name {
				return t.Status
			}
			return ""
		}
	}
	if lsGone(addr) {
		return ""
	}
	return "?표에 없음"
}

// ownerAddrIs: 터미널 주소가 그 주인 지휘인가 — 이 기계 터미널은 주소에 기계가 안 붙는다
func ownerAddrIs(addr, owner string) bool { return addr == owner || LOCAL+"/"+addr == owner }

// ownerFP: 거절을 다시 쓸 동안의 열쇠 — 그 지휘 터미널에 에이전트 상태가 있나(있음·없음만 — working·idle 같은 값은 살아 있는 에이전트에서
// 늘 바뀌어 거절된 지휘의 항목이 상태가 바뀔 때마다 다시 났다, 감사 atp0flj 중간 1)·이름, 그리고 hc-lane 지휘 정보 <상태 폴더>/hc-lane/coord.<주소>
// 내용(로컬 파일 — 원격 호출 없음). 셸만 남은 터미널에 에이전트를 다시 띄우면 상태가 생겨, 재개한 지휘가 레인을 띄우거나 wait를 걸면
// 지휘 정보(pid·시각)가 바뀌어 다시 묻는다. 지휘 정보를 못 읽으면(없음 말고) 「?」
func ownerFP(status, name, owner string) string {
	st := "에이전트"
	if status == "" || status == "-" {
		st = "-"
	}
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		dir = HOME + "/.local/state"
	}
	ci := "-"
	if b, err := os.ReadFile(dir + "/hc-lane/coord." + ckeyOf(owner)); err == nil {
		ci = strings.Join(strings.Fields(string(b)), " ")
	} else if !errors.Is(err, fs.ErrNotExist) {
		ci = "?"
	}
	return st + "/" + orDash(name) + "/" + ci
}

// ckeyOf: hc-lane이 주소로 파일·이름을 짓는 열쇠(cmd/hc-lane/hcx.go ckey와 같은 규칙 — 글자·숫자·_·- 밖은 _)
func ckeyOf(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-' {
			b.WriteByte(c)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// watchDead: hc-lane이 「지휘 아님」으로 거절한 주인 지휘 → 그때 hc ls 지문(DATA/watchdead.tsv — watchOwners가 바퀴마다 고쳐 씀)
func watchDead() map[string]string {
	m := map[string]string{}
	for _, l := range readLines(DATA + "/watchdead.tsv") {
		if o, fp, ok := strings.Cut(l, "\t"); ok {
			m[o] = fp
		}
	}
	return m
}

// watchDeadSave: 이 바퀴에 거절이 확인된(또는 지문이 그대로라 그대로 둔) 지휘만 남긴다. 바뀐 것이 없으면 쓰지 않는다
func watchDeadSave(old, keep map[string]string) {
	same := len(old) == len(keep)
	var ls []string
	for o, fp := range keep {
		same = same && old[o] == fp
		ls = append(ls, o+"\t"+fp+"\n")
	}
	if same {
		return
	}
	sort.Strings(ls)
	if err := writeAtomic(DATA+"/watchdead.tsv", strings.Join(ls, "")); err != nil {
		say("감시: watchdead.tsv 쓰기 실패 — %v", err)
	}
}

// ownerNotAgent: 상태 한 장 쪽 — 그 지휘 터미널의 지금 지문이 hc-lane이 거절했을 때와 같으면 참(사라진 흔적과 같게 다룬다)
func ownerNotAgent(dead map[string]string, owner string, t lsTerm) bool {
	fp, ok := dead[owner]
	return ok && fp == ownerFP(t.Status, t.Name, owner)
}
