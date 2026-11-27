package main

// 같은 줄을 다른 실행으로 다시 띄우기(ht-relaunch-unify, 2026-11-28, docs/hc-fill/queue.md 「14칸」): 대기열 줄은 「일」이고 그 줄이 묶인
// 터미널은 「지금 실행」이다. 주차 → 이어 띄움, 반송 → 다시 띄움, 채택은 줄의 출처를 relMode로 읽고, 같은 이름 옛 실행은 takeover로 받고,
// 새 실행에는 bindRun으로 묶는다. 14칸(과 시도 기록 4칸)의 값 ↔ 뜻 대응은 modeOf·val만 안다(relaunch_test.go가 다른 파일의 14칸 읽기를 막는다)
// 디스크 꼴은 옛 값 일곱 가지 그대로다 — 배포 도중 옛 판 상주 hc-fill이 새 판이 쓴 줄을, 새 판이 옛 판 줄을 읽는다

// relSrc: 다시 띄울 출처
type relSrc int

const (
	srcNew    relSrc = iota // 새로 — 지시문 처음부터
	srcPark                 // 주차 가지 lane/<레인>(hc-lane park → 선행 착지 뒤 hc-lane --resume, park.md)
	srcBounce               // 반송 가지 refs/merge-failed/<제출>(반송 절, bounce.md)
)

// relMode: 14칸 한 칸을 두 뜻으로 갈라 읽은 값 — Early는 먼저 띄움 여부, Src·Ran은 다시 띄울 출처
type relMode struct {
	Early string // 출처가 새로일 때 칸 값 그대로: ""·-(정하지 않음)·no-early(끔)·early(먼저 띄운 실행)·stacked(쌓아 띄운 실행, stack.go), 모르는 값도 그대로
	Src   relSrc // 대기 줄은 그 출처에서 띄울 차례, 떠 있는 줄은 그 출처에서 띄웠거나 채택한 실행
	Ran   bool   // 출처에서 이미 다시 띄운 실행(resumed — 다시 주차 후보가 아니고, 주차를 적으면 그 실행을 닫는다)
}

var (
	mEarly   = relMode{Early: "early"}
	mNoEarly = relMode{Early: "no-early"}
	mStacked = relMode{Early: "stacked"}
	mParked  = relMode{Src: srcPark}
	mResumed = relMode{Src: srcPark, Ran: true}
	mBounced = relMode{Src: srcBounce}
)

// launchedMode <먼저 띄움>: 이번에 띄운 실행에 적을 14칸(RSRC·STACK) — 쌓아 띄움 > 반송(반송됨 표시를 지움) > 주차에서 이어(다시 먼저 띄움
// 후보가 아님) > 먼저 띄움, 아니면 ""(그대로)
func launchedMode(early bool) string {
	switch {
	case STACK != "":
		return mStacked.val()
	case RSRC == srcBounce:
		return "-"
	case RSRC == srcPark:
		return mResumed.val()
	case early:
		return mEarly.val()
	}
	return ""
}

// modeOf <14칸 값>: 칸 값의 뜻(모르는 값은 Early에 그대로 — val로 되쓰면 같은 글자)
func modeOf(v string) relMode {
	switch v {
	case "parked":
		return mParked
	case "resumed":
		return mResumed
	case "bounced":
		return mBounced
	}
	return relMode{Early: v}
}

// val: modeOf의 거꾸로 — 디스크에 쓸 칸 값
func (m relMode) val() string {
	switch {
	case m.Src == srcPark && m.Ran:
		return "resumed"
	case m.Src == srcPark:
		return "parked"
	case m.Src == srcBounce:
		return "bounced"
	}
	return m.Early
}

// rowMode: 대기열 줄의 14칸
func rowMode(r Row) relMode { return modeOf(r.F(qEarly)) }

// setMode: 대기열 줄의 14칸을 바꾼 줄(edit — qset 밖에서 줄을 통째로 고치는 곳)
func setMode(r Row, m relMode) Row { return r.Set(qEarly, m.val()) }

// end: 이 출처에서 다시 띄우려면 옛 실행이 끝으로 적었어야 할 보고(endClose) — 새로는 없음
func (m relMode) end() string {
	switch m.Src {
	case srcPark:
		return "주차"
	case srcBounce:
		return "제출"
	}
	return ""
}

// takeover <레인> <줄 ID> <출처> <같은 이름 hc-lane 레인 터미널>: 대기 줄이 그 옛 실행을 어떻게 받나.
// 0 채택(출처가 새로이거나 주차에서 이어 띄운 실행이 아직 주차를 안 적음 — adopt), 1 닫힘(그 출처의 끝 보고로 끝낸 옛 실행을 닫았거나 이미 없음 —
// 새로 띄운다), 2 남음(띄우지 않는다 — 까닭). 반송은 이어 띄운 실행이 없다(띄우면 칸이 -로)
func takeover(name, rid string, m relMode, t *Term) (int, string) {
	switch m.Src {
	case srcBounce:
		if gone, why := endClose(name, t.Addr, t.M, m.end()); !gone {
			return 2, "같은 이름 반송된 옛 실행 터미널 살아 있음 " + t.Addr + " (" + dash(t.Status) + ", 안 닫음: " + why + ")"
		}
		return 1, ""
	case srcPark:
		if !m.Ran {
			return parkCandidate(name, rid, t.M, t.Addr, t.Status)
		}
	}
	return 0, ""
}

// bindRun <레인> <줄 ID> <기계> <주소> <실행 키> <14칸> <감사 칸>: 줄을 새 실행에 묶는다 — 시도 기록(실행 키)을 먼저, 그다음 대기열 줄을
// 떠 있음·그 주소로. 14칸·감사 칸 ""은 그대로 둠. 반환 -1 시도 기록을 못 씀(대기열 그대로), 그 밖은 qset의 종료(0 됨)
func bindRun(name, rid, m, addr, run, e, u string) int {
	if tryput(name, rid, m, e, u, addr, run) != 0 {
		return -1
	}
	return qset(true, rid, name, "up", m, addr, stuck, e, u)
}
