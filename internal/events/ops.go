// ops.go: 사건 종류(레인 / 운영 — 2026-11-24 ops-events). 운영 사건(병합기 멈춤·자리 빔·디스크 부족 …)은 레인 이름 칸에 도구 이름을
// 달고 레인 사건과 같은 파일에 적힌다. 쓰는 쪽은 14칸에 KindOps를 싣고(hc-lane report --kind ops), 읽는 쪽은 14칸이 없으면 아래 표의
// 이름·키 머리로 가른다(옛 판이 쓴 줄). 레인 사건 줄은 지금 꼴 그대로(14칸 없음) — 사건 열쇠(줄 그대로)가 배포 전후 같다.
// 사건 목록 문서: docs/ops/events.md(시험이 이 표와 맞춘다)
package events

import "strings"

// 사건 종류 — 14칸 값이자 hc-lane events|wait --kind 값
const (
	KindLane = "lane"
	KindOps  = "ops"
)

// 운영 사건 이름(레인 칸). merger-·on-land-는 뒤에 프로젝트·이름이 붙는다
const (
	OpsFillIdle  = "hc-fill-idle"
	OpsFillSkew  = "hc-fill-skew"
	OpsFill      = "hc-fill"
	OpsMerger    = "merger"
	OpsMergerP   = "merger-"  // + <프로젝트>
	OpsOnLand    = "on-land-" // + <이름>
	OpsDiskLow   = "hc-disk-low"
	OpsTailscale = "hc-tailscale"
	OpsTmpSkip   = "hc-sweep-tmp-skip"
	OpsHealth    = "hc-health"
	OpsUnreach   = "hc-fill-unreach"
	OpsCordon    = "hc-fill-cordon"
	OpsStaleRes  = "hc-stale-resident"
	OpsTestOrph  = "hc-test-orphan"
	OpsAdvise    = "hc-fill-advise"
	OpsFillPR    = "hc-fill-pr"
	OpsAgent     = "hc-fill-agent"
	mergerKeyPre = "hf:merger:" // merger-<프로젝트> 사건의 키 머리 — 진짜 레인 merger-alive 같은 이름과 가르는 것은 이름이 아니라 이것
	// AdviseAgentKey: 조치 방식 운영 에이전트가 맡을 손댈 것만 든 hc-fill-advise 사건의 키 머리(뒤에 초) — watch가 지휘에 묶음으로(coord-digest)
	AdviseAgentKey = "hf:advise:agent:"
	// AdviseRefKey: 레인 주인 지휘에게만 보낸 손댈 것을 다른 지휘에게 참고로 알리는 hc-fill-advise 사건의 키 머리(뒤에 초) — 묶음으로(ht-advise-claim)
	AdviseRefKey = "hf:advise:ref:"
)

// Op: 운영 사건 하나 — Name은 문서 제목 꼴(merger-<프로젝트>), Next는 받으면 가장 흔한 다음 명령(watch가 글 끝에 「→ <Next>」로 붙인다)
type Op struct{ Name, Next string }

// Ops: 운영 사건 목록 — docs/ops/events.md의 「## <이름>」과 「다음:」 줄이 이 표와 같아야 한다(ops_test)
var Ops = []Op{
	{OpsFillIdle, "hc-fill ls"},
	{OpsFillSkew, "hc-fill machine"},
	{OpsFill, "hc-fill status"},
	{OpsMerger, "hc-lane logs"},
	{OpsMergerP + "<프로젝트>", "hc-fill status"},
	{OpsOnLand + "<이름>", "hc-fill status"},
	{OpsDiskLow, "hc-sweep --dry-run"},
	{OpsTmpSkip, "hc-sweep --dry-run"},
	{OpsTailscale, "tailscale status"},
	{OpsHealth, "hc-sweep --health"},
	{OpsUnreach, "hc machine ls"},
	{OpsCordon, "hc-fill machine"},
	{OpsStaleRes, "hc-fill state"},
	{OpsTestOrph, "ps -o pid,ppid,etime,%cpu,command -p <pid>"},
	{OpsAdvise, "hc-fill advise"},
	{OpsFillPR, "hc-fill knob ls"},
	{OpsAgent, "hc-fill agent status"},
}

// OpOf: 레인 칸·키 칸으로 본 운영 사건(종류 칸 없는 옛 줄도). 아니면 false.
// merger-<프로젝트>는 키 머리 hf:merger:가 있을 때만(진짜 레인 merger-alive·merger-journal-why와 이름이 겹친다)
func OpOf(lane, key string) (Op, bool) {
	name := lane
	switch {
	case strings.HasPrefix(lane, OpsMergerP):
		if !strings.HasPrefix(key, mergerKeyPre) {
			return Op{}, false
		}
		name = OpsMergerP + "<프로젝트>"
	case strings.HasPrefix(lane, OpsOnLand) && len(lane) > len(OpsOnLand):
		name = OpsOnLand + "<이름>"
	}
	for _, o := range Ops {
		if o.Name == name {
			return o, true
		}
	}
	return Op{}, false
}

// KindOf: 사건 줄 칸(탭으로 나눈 것)의 종류 — 14칸이 lane·ops면 그것, 없거나 다른 값이면 이름 표(OpOf)로
func KindOf(f []string) string {
	return Cols(strings.Join(f, "\t")).KindOf()
}

// KindOf: 읽은 사건의 종류 — 14칸이 lane·ops면 그것, 없거나 다른 값이면 이름 표(OpOf)로
func (e Event) KindOf() string {
	if e.N >= 14 && (e.Kind == KindOps || e.Kind == KindLane) {
		return e.Kind
	}
	if _, ok := OpOf(e.Lane, e.Key); ok {
		return KindOps
	}
	return KindLane
}

// WithKind: 12·13칸 줄 뒤에 14칸 종류를(13칸이 없으면 「-」로 채움). 레인(KindLane)이고 이름 표로도 레인이면 줄 그대로
func WithKind(line, kind string) string {
	e := Cols(line)
	if kind == "" || kind == e.KindOf() && kind == KindLane || e.N < 12 || e.N > 13 {
		return line
	}
	if e.N == 12 {
		line += "\t-"
	}
	return line + "\t" + kind
}
