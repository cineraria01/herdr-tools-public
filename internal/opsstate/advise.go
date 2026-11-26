package opsstate

// 지금 손댈 것(2026-11-24 ops-advise, docs/ops/advise.md) — 사용자 「워커들 놀지 말고 다 쉬는것 없이 다 돌려서 그 시간이야 ?」:
// 놀고 있는 자리·풀 수 있는 대기·한 줄 사슬·멈춘 레인·굶는 감사·병합기·밀린 배포·옛 판 상주처럼 도구가 이미 아는 사실의 조합을
// 항목마다 「무엇 · 근거 · 놓치면 · 다음 명령 · 확신」으로 묶는다. 읽기만 한다 — 조치는 사람이나 지휘 에이전트가 다음 명령을 쳐서 한다.
// 문턱은 손잡이(Thresholds), 모름은 「판정 못 함」 항목(없음으로 내지 않는다). 상태 한 장의 「눈에 띄는 것」은 이 목록의 요약이다(Notable).

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/journal"
)

// AdviseSchema: advise json 꼴 판
const AdviseSchema = 1

// 확신
const (
	SureYes   = "확실"
	SureJudge = "판단 필요"
	SureHuman = "사람이 정한다" // 그대로 칠 명령이 없거나, 되돌릴 수 없는·남의 프로세스를 끄는 명령이라 내지 않음
)

// Item: 손댈 것 하나. Key는 숫자 없는 열쇠(사건을 같은 항목에 되풀이하지 않게)
type Item struct {
	Key      string   `json:"key"`
	Kind     string   `json:"kind"`
	What     string   `json:"what"`
	Evidence string   `json:"evidence"`
	Cost     string   `json:"cost"`
	Next     []string `json:"next"`
	Sure     string   `json:"sure"`
	// Claim: 이 항목 대상에 최근 다른 지휘가 한 조치(일지에서 hc-fill이 채움) — 「<지휘> n분 전 — <명령>(<까닭>)」, 없으면 빈 값.
	// 막지 않는다 — 같은 알림을 두 지휘가 따로 손대지 않게 보이기만(ht-advise-claim). 요약(Summary)·지문에는 넣지 않는다
	Claim string `json:"claim"`
}

// laneKinds: 열쇠 이름이 레인인 항목 종류(그 레인 주인 지휘에게 보낸다)
var laneKinds = map[string]bool{"chain": true, "rejected": true, "free-wait": true, "pinned": true, "audit-starve": true,
	"lane-error": true, "lane-blocked": true, "lane-done": true, "lane-escalated": true, "lane-long": true, "withdraw": true}

// Lane: 레인 항목이면 그 레인 이름, 아니면 빈 값
func (i Item) Lane() string {
	if !laneKinds[i.Kind] {
		return ""
	}
	_, n, _ := strings.Cut(i.Key, ":")
	return n
}

// Summary: 한 줄(상태 한 장의 눈에 띄는 것·사건 글)
func (i Item) Summary() string {
	if i.Evidence == "" {
		return i.What
	}
	return i.What + " — " + i.Evidence
}

// Advice: hc-fill advise 한 장
type Advice struct {
	Schema      int       `json:"schema"`
	At          time.Time `json:"at"`
	Fingerprint string    `json:"fingerprint"`
	Items       []Item    `json:"items"`
}

// Facts: State에 없는 advise 재료(모으는 쪽 hc-fill이 같은 바퀴 기록에서 채운다)
type Facts struct {
	Waits    []Wait
	Addr     map[string]string // 떠 있는 레인 → 터미널 주소
	Paused   map[string]bool   // 새 레인을 안 띄우는 기계(pause@)
	Resident []string          // 이 기계 상주 판 점검 결과 줄(resident.Show)
	// DeployHold: 배포 밀림을 아직 따지지 않을 까닭(배포 도는 중·착지 뒤 명령 차례 남음·마지막 착지 뒤 deploy_grace분 안), 없으면 빈 값.
	// 착지 직후 착지 뒤 배포가 도는 1~2분에 밀림 사건이 거짓으로 나갔다(2026-11-24 지휘)
	DeployHold string
	// Known: 이미 알린 항목 열쇠(advise 사건 기록·운영 에이전트 기록). 보류는 새 열쇠의 첫 알림만 미룬다 — 이미 알린 밀림을 보류 동안
	// 빼면 열쇠를 잊어 보류가 풀릴 때마다 같은 사건이 다시 나갔다(감사 atozuyd 중간)
	Known map[string]bool
	// Acts: 최근 일지(상태 한 장이 읽은 24시간 — 항목의 「조치함」, Claims)
	Acts []journal.Entry
	// Pred: 선행·겹침 앞 레인 단계 — 대기열 전체(주인·상태 상관없이, Trim이 숨긴 끝난 줄도), 대기열에 없으면 그 프로젝트 기본 가지의
	// 착지(Stage 「착지 <sha8>」, 나이 = 커밋 뒤 초). 착지도 없으면 넣지 않는다(= 대기열에 없음). nil = 재료 없음(단계 모름).
	// 열쇠는 「이름@프로젝트」(PredKey) — 같은 이름의 다른 프로젝트 줄과 섞지 않는다.
	// 추린 떠 있는 레인에서만 찾아 착지한 선행·대기 줄·나이 모름이 「단계 모름」으로 보였다(ht-advise-stage-lookup)
	Pred map[string]Lane
}

// Wait: 대기 줄 하나와 기다리는 까닭
type Wait struct {
	Lane, Project string
	AgeS          *int     // 넣은 뒤 초(모르면 nil — 문턱을 적용하지 않는다)
	After         []string // 선행(「이름」 또는 「이름@프로젝트」)
	Front         string   // 겹침 앞 레인
	Behind        bool     // Front는 떠 있는 레인이 아니라 줄 차례가 앞선 겹침 대기 줄(overlap.tsv 6칸 「줄」)
	Keys          []string // 겹치는 파일·폴더(overlap.tsv — 두 개까지만 적힌다)
	Solo          bool     // 단독 대기
	Reject        string   // 띄우기 거절 까닭
	Can           []string // 띄울 수 있는 기계(저장소 있음·기계 제한). nil = 모름(모든 기계로 본다)
	Wider         []string // 기계 제한 밖이지만 저장소가 있는 기계(--only로 넓힐 수 있는 곳)
	LightOnly     []string // 보통 레인이라 뺀 가벼운 일만 받는 기계(light_only@ — 기계 제한 안). Can이 비고 여기만 있으면 「가벼운 기계만 남음」
	Pin           string   // 묶인 기계가 모두 상한·멈춤이라 못 뜬 까닭(바퀴가 pinned.tsv에 적음), 아니면 빈 값
	PinS          *int     // 그렇게 된 지 초
}

func shq(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./@:,=") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func afterName(a string) string { n, _, _ := strings.Cut(a, "@"); return n }

// light: 문서·시험 파일(겹쳐도 rebase에서 그 파일만 맞추면 되는 것)
func light(k string) bool {
	return strings.HasSuffix(k, ".md") || strings.HasSuffix(k, ".txt") || strings.HasSuffix(k, "_test.go") ||
		strings.HasPrefix(k, "docs/") || strings.HasPrefix(k, "tests/") || strings.Contains(k, "/testdata/")
}

var auditWaitRe = regexp.MustCompile(`^감사 대기\(자리 없음 ([0-9]+)분\)`)

// Advise: 지금 손댈 것. s는 추린 상태(끝난 레인은 Finished로 건너뛴다), f는 그 밖 재료
func Advise(s State, f Facts, th Thresholds) []Item {
	var o []Item
	add := func(it Item) { o = append(o, it) }

	// 빈 자리(닿은 기계, 멈춤 아님): 마지막 바퀴의 띄우기가 판정한 값(Machine.Free) 그대로 — 따로 세지 않는다.
	// 자리·레인 칸으로 다시 세면 띄우기의 「레인상한 참」과 어긋나 거짓 「풀 수 있는 대기」가 났다(ht-advise-slot-cap). 모름(nil)은 세지 않는다
	free := map[string]int{}
	cores := map[string]int{}
	for _, m := range s.Machines {
		if m.Reach != "ok" || f.Paused[m.Name] || m.Free == nil || *m.Free <= 0 {
			continue
		}
		free[m.Name] = *m.Free
		if m.Cores != nil {
			cores[m.Name] = *m.Cores
		}
	}
	freeFor := func(can []string) (int, int, string) {
		n, c := 0, 0
		var t []string
		for _, m := range s.Machines {
			if free[m.Name] == 0 || can != nil && !has(can, m.Name) {
				continue
			}
			n += free[m.Name]
			c += cores[m.Name]
			t = append(t, fmt.Sprintf("%s %d", m.Name, free[m.Name]))
		}
		return n, c, strings.Join(t, "·")
	}
	// 손잡이 idle 0이면 끔(문서 「0이면 끔」 — 나이 ≥ 0이 늘 참이라 모든 대기 줄이 항목이 됐다, advise-fixes)
	old := func(w Wait) bool { return th.IdleMin > 0 && (w.AgeS == nil || *w.AgeS >= th.IdleMin*60) }
	waitBy := map[string]Wait{}
	for _, w := range f.Waits {
		waitBy[w.Lane] = w
	}
	parentWait := func(w Wait) string { // 대기 줄 가운데 선행
		for _, a := range w.After {
			if _, ok := waitBy[afterName(a)]; ok {
				return afterName(a)
			}
		}
		return ""
	}

	// 한 줄 사슬: 대기 줄이 대기 줄을 기다린다 — 머리(선행이 대기 줄이 아님)에서 가장 긴 길
	kids := map[string][]string{}
	for _, w := range f.Waits {
		if p := parentWait(w); p != "" {
			kids[p] = append(kids[p], w.Lane)
		}
	}
	var longest func(n string, seen map[string]bool) []string
	longest = func(n string, seen map[string]bool) []string {
		if seen[n] {
			return nil // 서로 기다림(고리) — 사슬 길이는 여기서 끊는다
		}
		seen[n] = true
		var best []string
		for _, k := range kids[n] {
			if p := longest(k, seen); len(p) > len(best) {
				best = p
			}
		}
		return append([]string{n}, best...)
	}
	for _, w := range f.Waits {
		if parentWait(w) != "" || len(kids[w.Lane]) == 0 {
			continue
		}
		path := longest(w.Lane, map[string]bool{})
		n, c, ft := freeFor(w.Can)
		if len(path) < 2 || n == 0 || !old(w) {
			continue
		}
		var next []string
		for _, l := range path[1:] {
			next = append(next, "hc-fill edit "+l+" --after - --why '사슬 풀기(hc-fill advise)'")
		}
		head := "선행 없음"
		if len(w.After) > 0 {
			head = "머리 " + w.Lane + "의 선행 " + predStages(s, f, w) // 선행마다 제 단계(advise_pred.go)
		}
		add(Item{Key: "chain:" + w.Lane, Kind: "chain",
			What:     fmt.Sprintf("한 줄 사슬: %s부터 %d줄이 하나씩만 돈다", w.Lane, len(path)),
			Evidence: fmt.Sprintf("사슬 %d(%s) · %s · 빈 자리 %d(%s) · 코어 %d", len(path), strings.Join(path, " → "), head, n, ft, c),
			Cost:     fmt.Sprintf("안 풀면: 사슬이 끝날 때까지(레인 %d개를 차례로 — 끝 예상 모름) 자리 %d개가 논다 · 풀면: 같은 파일을 고치는 줄이면 착지 때 rebase 충돌 풀이", len(path), n),
			Next:     next, Sure: SureJudge})
	}

	// 대기 줄마다: 거절 · 겹침 · 선행(사슬 가운데 줄은 사슬 항목이 맡는다)
	for _, w := range f.Waits {
		if w.Reject != "" {
			add(Item{Key: "rejected:" + w.Lane, Kind: "rejected", What: "띄우기 거절로 멈춘 줄: " + w.Lane,
				Evidence: "까닭 " + w.Reject + " · 기다린 지 " + age(w.AgeS),
				Cost:     "까닭을 고칠 때까지 이 줄은 다른 기계에도 안 뜬다",
				Next:     []string{"hc-fill retry " + w.Lane + " --why '거절 까닭 고침(hc-fill advise)'"}, Sure: SureJudge})
			continue
		}
		if parentWait(w) != "" || !old(w) || w.Behind { // 앞선 대기 줄 뒤에 선 줄은 그 앞 줄 항목이 맡는다(풀면 줄 차례가 깨진다)
			continue
		}
		if len(w.Can) == 0 && len(w.LightOnly) > 0 { // 띄울 곳이 가벼운 일만 받는 기계뿐 — 무게를 고치거나 손잡이를 풀 때까지 안 뜬다
			add(Item{Key: "light-only:" + w.Lane, Kind: "light-only",
				What:     "가벼운 기계만 남음: " + w.Lane,
				Evidence: fmt.Sprintf("보통 레인 · 띄울 수 있는 기계 %s는 가벼운 일만(light_only@) · 기다린 지 %s", strings.Join(w.LightOnly, ","), age(w.AgeS)),
				Cost:     "안 고치면: 이 줄은 안 뜬다 · --light로 고치면: 느린 기계에서 돌아 오래 걸릴 수 있다(지시문을 보고 정한다)",
				Next:     []string{"hc-fill edit " + w.Lane + " --light --why '가벼운 레인(hc-fill advise)'"}, Sure: SureJudge})
			continue
		}
		n, _, ft := freeFor(w.Can)
		if n == 0 {
			continue
		}
		switch {
		case w.Front != "" || w.Solo:
			sure, what, gain := SureJudge, "겹침", ""
			switch {
			case w.Solo:
				what, gain = "단독", "단독 레인(파일을 가로지르는 변경)이 떠 있거나 이 줄이 단독이라 함께 돌면 부딪힌다 — 단독을 풀지는 사람이 정한다"
			case len(w.Keys) == 0:
				gain = "겹치는 파일 모름"
			case strings.HasSuffix(w.Keys[0], "/") || len(w.Keys) > 1 && strings.HasSuffix(w.Keys[1], "/"):
				gain = "꾸러미 " + strings.Join(w.Keys, ",") + " 함께 고침 — 착지 때 rebase 충돌 풀이가 클 수 있다"
			case len(w.Keys) == 1 && light(w.Keys[0]):
				sure, gain = SureYes, "문서·시험 한 파일("+w.Keys[0]+") — 착지 때 rebase에서 그 파일만 맞추면 된다"
			default:
				gain = "파일 " + strings.Join(w.Keys, ",") + " 함께 고침(두 개까지만 적힘) — 착지 때 rebase 충돌 풀이"
			}
			// edit는 줄 ID·주차 표식·다른 칸을 그대로 두고 겹침칸만 바꾼다 — rm && add는 add가 실패하면 줄이 사라지고 옵션이 안 돌아왔다(advise-fixes)
			next := []string{"hc-fill edit " + w.Lane + " --overlap-ok --why '겹침 풀기(hc-fill advise)'"}
			if w.Solo {
				next, sure = nil, SureHuman
			}
			add(Item{Key: "free-wait:" + w.Lane, Kind: "free-wait",
				What:     fmt.Sprintf("풀 수 있는 대기: %s(%s %s)", w.Lane, what, w.Front),
				Evidence: fmt.Sprintf("빈 자리 %d(%s) · %s 기다린 지 %s · %s %s — 앞 레인 %s %s", n, ft, w.Lane, age(w.AgeS), what, strings.Join(w.Keys, ","), w.Front, predStage(s, f, w.Front, w.Project)),
				Cost:     "안 풀면: 앞 레인 착지까지 기다림(남은 예상 모름) 동안 자리가 논다 · 풀면: " + gain,
				Next:     next, Sure: sure})
		case len(w.After) > 0:
			add(Item{Key: "free-wait:" + w.Lane, Kind: "free-wait",
				What:     fmt.Sprintf("풀 수 있는 대기: %s(선행 %s)", w.Lane, strings.Join(w.After, ",")),
				Evidence: fmt.Sprintf("빈 자리 %d(%s) · %s 기다린 지 %s · 선행 %s", n, ft, w.Lane, age(w.AgeS), predStages(s, f, w)),
				Cost:     "안 풀면: 선행 착지까지 기다림(남은 예상 모름) 동안 자리가 논다 · 풀면: 선행의 산출물이 필요한 레인이면 못 푼다(지시문을 본다), 같은 파일이면 착지 때 rebase 충돌 풀이",
				Next:     []string{"hc-fill edit " + w.Lane + " --after - --why '선행 풀기(hc-fill advise)'"}, Sure: SureJudge})
		}
	}

	o = append(o, pinnedItems(s, f, th, free, freeFor)...) // 묶인 기계(advise_pinned.go)

	// 레인: 터미널 error·blocked·done(보고 없음) · 막힘 · 감사 자리 없음 · 같은 단계에 오래 — 레인마다 하나
	for _, l := range s.Lanes {
		if Finished(l) {
			continue
		}
		addr := ""
		if f.Addr != nil {
			addr = f.Addr[l.Name]
		}
		look := func(kind, what, cost string, extra ...string) {
			it := Item{Key: kind + ":" + l.Name, Kind: kind, What: what + ": " + l.Name,
				Evidence: fmt.Sprintf("%s · %s째 · 마지막 %s · 기계 %s", l.Stage, age(l.StageAgeS), or(l.LastEvent, "없음"), or(l.Machine, "-")),
				Cost:     cost, Sure: SureJudge}
			if addr == "" {
				it.Sure = SureHuman
				it.Evidence += " · 터미널 주소 모름"
			} else {
				it.Next = append([]string{"hc read " + addr}, extra...)
			}
			add(it)
		}
		switch {
		case strings.HasPrefix(l.Stage, "작업 중(error"):
			// 맨 hc send가 아니라 nudge — 주소의 터미널 이름 대조·이미 보고한 실행 거절·nudge_every 되풀이 막기를 거친다(advise-fixes)
			look("lane-error", "멈춘 레인(터미널 error)", "그대로 두면 이 레인은 더 나아가지 않고 자리를 잡는다",
				"hc-fill nudge "+shq(l.Name)+" --why '멈춘 레인(hc-fill advise)'")
		case strings.HasPrefix(l.Stage, "작업 중(blocked"):
			look("lane-blocked", "멈춘 레인(터미널이 답을 기다림)", "물음·권한 창에 답할 때까지 자리를 잡는다")
		case strings.HasPrefix(l.Stage, "작업 중(done"):
			look("lane-done", "끝났는데 보고 없음", "완료 보고가 없으면 감사·착지가 시작되지 않는다")
		case strings.HasPrefix(l.Stage, "막힘"):
			look("lane-escalated", "막힌 레인(지휘로 넘어옴)", "지휘가 정할 때까지 그 레인과 그 뒤 선행 사슬이 선다")
		default:
			if m := auditWaitRe.FindStringSubmatch(l.Stage); m != nil && th.WaitMin > 0 {
				if k, _ := strconv.Atoi(m[1]); k >= th.WaitMin {
					it := Item{Key: "audit-starve:" + l.Name, Kind: "audit-starve", What: "굶는 감사: " + l.Name,
						Evidence: fmt.Sprintf("%s · 기계 %s", l.Stage, or(l.Machine, "-")),
						Cost:     "감사가 뜰 때까지 완료한 레인이 착지하지 못한다", Sure: SureJudge}
					for _, x := range s.Machines {
						if x.Name == l.Machine && x.Seats != nil {
							it.Evidence += fmt.Sprintf(" · 자리 %s/%d", ival(x.Live), *x.Seats)
							it.Next = []string{fmt.Sprintf("hc-fill machine %s --seats %d --for 60 --why '감사 자리 없음 %s(hc-fill advise)'", x.Name, *x.Seats+1, l.Name)}
						}
					}
					if it.Next == nil {
						it.Sure = SureHuman
					}
					add(it)
					continue
				}
			}
			if l.Open && !l.Queued && th.StageMin > 0 && l.StageAgeS != nil && *l.StageAgeS >= th.StageMin*60 {
				look("lane-long", fmt.Sprintf("레인 %s 같은 단계에 %d분", l.Name, *l.StageAgeS/60), "멈췄으면 자리를 잡은 채 시간이 간다")
				o[len(o)-1].What = fmt.Sprintf("레인 %s: 같은 단계에 %d분 — %s", l.Name, *l.StageAgeS/60, l.Stage)
			}
		}
	}

	// 병합 줄
	busy := false
	for _, m := range s.Merge {
		busy = busy || m.Length == nil || *m.Length > 0
		switch {
		case m.Merger == "unknown":
			add(unknown("merger:"+m.Project, fmt.Sprintf("병합기 %s 살아 있는지(줄 %s)", m.Project, ival(m.Length)), or(m.Why, "병합기 살아 있음 기록 모름")))
		case m.Merger != "alive":
			it := Item{Key: "merger:" + m.Project, Kind: "merger", What: fmt.Sprintf("병합기 %s: %s(줄 %s)", m.Project, m.Merger, ival(m.Length)),
				Evidence: or(m.Why, "병합기 터미널 없음"), Cost: "병합기가 없으면 그 저장소의 모든 착지가 선다",
				Next: []string{"hc-fill merger restart " + m.Project + " --why '병합기 없음(hc-fill advise)'"}, Sure: SureYes}
			if m.Length != nil && *m.Length == 0 {
				it.Sure, it.Cost = SureJudge, "줄이 비어 지금 막힌 착지는 없다 — 다음 제출부터 선다"
			}
			add(it)
		case m.Length != nil && *m.Length > 0:
			for _, it := range mergeLineItems(m, th) {
				add(it)
			}
		}
	}

	// 배포: 착지했는데 기계에 안 깔림
	var depAge *int
	for _, x := range s.Sources {
		if x.Name == "deploy" {
			depAge = x.AgeS
		}
	}
	for _, d := range s.Deploy {
		switch {
		case d.Why == DeployNotTarget:
		case d.Version == "" || d.Behind == nil:
			add(unknown("deploy:"+d.Machine, "배포 "+d.Machine, or(d.Why, "까닭 모름")))
		case *d.Behind > 0 && (f.DeployHold == "" || f.Known["deploy:"+d.Machine]): // 보류는 기계 구분 없이 새 밀림의 첫 알림만
			it := Item{Key: "deploy:" + d.Machine, Kind: "deploy", What: fmt.Sprintf("배포 %s: %d커밋 밀림(%s)", d.Machine, *d.Behind, d.Version),
				Evidence: fmt.Sprintf("기록상 판이 origin/main에서 %d커밋 뒤 · 마지막 배포 기록 %s 전 — 그 뒤 착지한 고침이 이 기계에 없다", *d.Behind, age(depAge)),
				Cost:     "고친 도구가 안 깔려 같은 문제가 이어진다",
				Next:     []string{"herdr-sync --status " + d.Machine, "herdr-sync " + d.Machine}, Sure: SureYes}
			if busy {
				it.Sure, it.Cost = SureJudge, it.Cost+" · 병합 줄이 차 있어 곧 또 밀린다(비었을 때 한 번에 깔 수도)"
			}
			add(it)
		}
	}

	// 감시 — Trim이 숨긴 안 도는 감시까지(숨긴 뒤에 보면 이 갈래에 닿지 않았다, advise-fixes). 지휘가 사라진 흔적(Need 거짓)은 빼고
	for _, w := range append(append([]Watch(nil), s.Watch...), s.goneWatch...) {
		switch {
		case w.Alive == nil:
			add(unknown("watch:"+w.Owner, "지휘 "+w.Owner+" 감시", "hc ls 못 읽음"))
		case !*w.Alive && w.Need:
			add(Item{Key: "watch:" + w.Owner, Kind: "watch", What: "지휘 " + w.Owner + " 감시 안 돎(열린 레인 있음)",
				Evidence: "지휘 터미널은 있는데 감시 터미널이 hc ls에 없음", Cost: "그 지휘가 레인 사건을 받지 못한다",
				Next: []string{"hc-lane coord --watch " + w.Owner}, Sure: SureYes})
		}
	}

	// 옛 판 상주(hc-sweep 결과 줄). 다시 띄우는 명령은 모두 프로세스를 끈다(kill <pid>·hc kill <터미널>·launchctl kickstart -k) —
	// 결과 줄의 pid는 확인한 때의 것이라 그새 재사용됐을 수 있다. 다음 명령으로 내지 않고 근거에만 남긴다(사람이 정한다, 감사 atozr8z 중간 1)
	for _, r := range f.Resident {
		if !strings.HasPrefix(r, "옛 판으로 도는 상주: ") {
			add(unknown("resident", "상주 판", r))
			continue
		}
		who, _, _ := strings.Cut(strings.TrimPrefix(r, "옛 판으로 도는 상주: "), "(")
		add(Item{Key: "resident:" + who, Kind: "resident", What: "옛 판 상주: " + who, Evidence: r,
			Cost: "그 상주가 하던 일(사건 감시·화면 맞춤)이 옛 판 동작으로 돈다 · 다시 띄우는 명령은 프로세스를 끄니 pid가 지금도 그 상주인지 보고 친다",
			Sure: SureHuman})
	}

	o = append(o, machineItems(s, f, free, th)...)

	// 재료를 못 읽음 — 위 판정이 비었어도 「없음」이 아니다
	for _, x := range s.Sources {
		if x.Why != "" {
			add(unknown("source:"+x.Name, "재료 "+x.Name+" 못 읽음", x.Why))
		}
	}
	return o
}

// machineItems: 기계 끊김·치우침(Advise의 한 단계). free는 Advise가 센 빈 자리
func machineItems(s State, f Facts, free map[string]int, th Thresholds) []Item {
	var o []Item
	var hot, cool []string
	hotSet := map[string]bool{}
	for _, m := range s.Machines {
		if m.Reach != "ok" {
			if strings.Contains(m.Why, "모름") {
				o = append(o, unknown("machine:"+m.Name, "기계 "+m.Name, m.Why))
			} else {
				o = append(o, Item{Key: "machine:" + m.Name, Kind: "machine", What: "기계 " + m.Name + " 모름: " + or(m.Why, "까닭 모름"),
					Evidence: "그 기계 레인·자리를 잴 수 없다", Cost: "그 기계에 떠 있는 레인이 멈췄어도 모른다",
					Next: []string{"hc machine ls"}, Sure: SureJudge})
			}
			continue
		}
		if m.LoadPerCore == nil || th.SkewHi <= 0 {
			continue
		}
		switch p := *m.LoadPerCore; {
		case p >= th.SkewHi:
			hot = append(hot, fmt.Sprintf("%s %.2f(도는 레인 %s%s)", m.Name, p, ival(m.Lanes), VerNote(m.Verifies, m.MergeVerifies)))
			hotSet[m.Name] = true
		case p <= th.SkewLo:
			cool = append(cool, fmt.Sprintf("%s %.2f", m.Name, p))
		}
	}
	// 옮길 일이 있을 때만 — 도는 레인은 옮길 수 없고, 새 줄은 이미 부하를 보고 한가한 기계로 간다(2026-11-24 fill-polish-1124)
	if mv := SkewMovable(f.Waits, free, hotSet); len(hot) > 0 && len(cool) > 0 && len(mv) > 0 {
		o = append(o, Item{Key: "skew", Kind: "skew", What: "코어당 부하 치우침",
			Evidence: "바쁨 " + strings.Join(hot, ", ") + " · 한가 " + strings.Join(cool, ", ") +
				fmt.Sprintf(" · 대기 줄 %d 가운데 바쁜 기계로만 갈 줄 %d(%s)", len(f.Waits), len(mv), strings.Join(mv, ",")),
			Cost: "바쁜 기계의 레인·검증이 느려진다", Next: []string{"hc-fill machine"}, Sure: SureJudge})
	}
	return o
}

// VerNote: 도는 저장소 검증이 있으면 「 · 검증 n」 — 레인 0인 바쁜 기계의 까닭(병합기 추측 검증 등, ht-skew-verify-cause).
// 그 가운데 병합기 검증(g)이 있으면 「 · 검증 n(병합 g)」 — 병합 검증은 자리·레인상한에 안 든다(ht-fill-gate-seat).
// 모름(nil)·0은 빈 글: 모름을 0으로 적지 않고, 0이면 지금 글 그대로
func VerNote(v, g *int) string {
	if v == nil || *v <= 0 {
		return ""
	}
	if g != nil && *g > 0 {
		return fmt.Sprintf(" · 검증 %d(병합 %d)", *v, *g)
	}
	return fmt.Sprintf(" · 검증 %d", *v)
}

// SkewMovable: 치우침을 옮길 수 있는 대기 줄 — 띄울 차례인 줄(거절·겹침·단독·대기 줄 선행이 아님) 가운데 자리가 남는 후보
// (Can ∩ free, Can nil이면 모든 기계)가 있고 그 후보가 모두 hot인 줄. 한가하거나 부하 모르는 후보가 하나라도 있으면 배정이
// 코어당 부하 낮은 차례로 그쪽에 보내니 옮길 것이 없다. 자리 남는 후보가 없는 줄은 어디로 갈지 아직 모른다(항목 없음).
// ponytail: 부하 모르는 후보를 한가함 쪽으로 본다 — 배정(loadOrder)은 모르는 기계를 설정 차례 자리에 두니 그 차례가 hot 앞이 아니면 hot에 갈 수 있다
func SkewMovable(waits []Wait, free map[string]int, hot map[string]bool) []string {
	inWait := map[string]bool{}
	for _, w := range waits {
		inWait[w.Lane] = true
	}
	var o []string
next:
	for _, w := range waits {
		if w.Reject != "" || w.Front != "" || w.Solo {
			continue
		}
		for _, a := range w.After {
			if inWait[afterName(a)] {
				continue next
			}
		}
		n := 0
		for m, k := range free {
			if k <= 0 || w.Can != nil && !has(w.Can, m) {
				continue
			}
			if !hot[m] {
				continue next
			}
			n++
		}
		if n > 0 {
			o = append(o, w.Lane)
		}
	}
	return o
}

func unknown(key, what, why string) Item {
	return Item{Key: "unknown:" + key, Kind: "unknown", What: "판정 못 함: " + what, Evidence: why,
		Cost: "그 재료로 찾는 손댈 것을 놓칠 수 있다", Sure: SureHuman}
}

func has(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// Notable: 상태 한 장의 「눈에 띄는 것」 — 손댈 것의 요약(advise 재료 없이 상태로만 잴 수 있는 것)
func Notable(s State, th Thresholds) []string { return Summaries(Advise(s, Facts{}, th)) }

func Summaries(it []Item) []string {
	var o []string
	for _, x := range it {
		o = append(o, x.Summary())
	}
	return o
}

// AdviseJSON: 압축 json 한 줄(칸 이름은 docs/ops/advise.md 「json 칸」). 빈 목록은 []
func AdviseJSON(w io.Writer, a Advice) error {
	if a.Items == nil {
		a.Items = []Item{}
	}
	for i := range a.Items {
		if a.Items[i].Next == nil {
			a.Items[i].Next = []string{}
		}
	}
	e := json.NewEncoder(w)
	e.SetEscapeHTML(false)
	return e.Encode(a)
}

// AdviseHuman: 사람용 — 항목마다 무엇·근거·놓치면·다음 명령
func AdviseHuman(w io.Writer, a Advice) error {
	var b strings.Builder
	p := func(f string, x ...any) { fmt.Fprintf(&b, f+"\n", x...) }
	p("지금 손댈 것 %s · 상태 지문 %s", a.At.Local().Format("2006-01-02 15:04:05"), or(a.Fingerprint, "-"))
	if len(a.Items) == 0 {
		p("손댈 것 없음")
	}
	for i, x := range a.Items {
		p("\n%d. [%s] %s", i+1, x.Sure, x.What)
		p("   근거: %s", or(x.Evidence, "-"))
		p("   놓치면: %s", or(x.Cost, "-"))
		if x.Claim != "" {
			p("   조치함: %s", x.Claim)
		}
		if len(x.Next) == 0 {
			p("   다음: 없음(%s)", SureHuman)
		}
		for _, n := range x.Next {
			p("   $ %s", n)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
