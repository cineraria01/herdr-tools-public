package main

// 에이전트가 쓰기 좋은 꼴(2026-11-24 agent-skill-guides, docs/ops/agents.md): 읽는 명령의 --json(ls·machine — 칸 이름은 문서와
// 시험이 맞춘다), 바꾸는 명령의 --dry-run(rm·prio·knob set|unset·machine <기계> — 무엇이 바뀔지만 찍고 아무것도 안 쓴다),
// 흔한 실패의 「다음:」 한 줄. 사람용 출력·종료 코드는 그대로이고 더하기만 한다

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"herdr-tools/internal/knob"
)

// lsRow: hc-fill ls --json 한 줄 — 칸은 사람용 표와 같은 사실(docs/ops/agents.md)
type lsRow struct {
	Lane     string   `json:"lane"`
	Project  string   `json:"project"`
	State    string   `json:"state"`    // 표의 상태 칸 글(대기·떠 있음(working)·끝남·모름(…) 등)
	Code     string   `json:"code"`     // 대기열 파일의 상태 값(wait·up·sent …)
	Address  string   `json:"address"`  // 터미널 주소(대기면 -)
	Owner    string   `json:"owner"`    // 주인 지휘(-는 모든 지휘)
	Priority string   `json:"priority"` // 긴급·높음·보통·낮음
	After    []string `json:"after"`    // 선행(없으면 빈 목록)
	Only     []string `json:"only"`     // 묶인 기계(없으면 빈 목록)
	Last     string   `json:"last"`     // 마지막 보고(없으면 빈 글)
	Note     string   `json:"note"`     // 표의 선행·비고 칸에서 마지막 보고를 뺀 글
}

type lsOut struct {
	Lanes   []lsRow  `json:"lanes"`
	Mergers []string `json:"mergers"` // 병합기 살아 있음 줄(표 끝 줄과 같은 글)
}

func lsWantJSON(args []string) bool {
	for _, a := range args {
		if a == "--json" {
			return true
		}
	}
	return false
}

// lsList: 쉼표 목록 칸(-는 빈 목록)
func lsList(s string) []string {
	if dash(s) == "-" {
		return []string{}
	}
	return strings.Split(s, ",")
}

func lsPrintJSON(rows []lsRow) {
	if rows == nil {
		rows = []lsRow{}
	}
	ms := maLines()
	if ms == nil {
		ms = []string{}
	}
	printJSON(lsOut{Lanes: rows, Mergers: ms})
}

func printJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		die("JSON을 만들지 못했다: %v", err)
	}
	fmt.Println(string(b))
}

// machRow: hc-fill machine --json 한 줄. 모르는 값은 null
type machRow struct {
	Machine  string   `json:"machine"`
	Load1    *float64 `json:"load1"`      // 1분 부하(모르면 null)
	Cores    *int     `json:"cores"`      // 코어 수(모르면 null)
	PerCore  *float64 `json:"per_core"`   // 코어당 부하(모르면 null)
	LoadAgeS *int64   `json:"load_age_s"` // 부하를 잰 지 몇 초(모르면 null)
	Used     *int     `json:"used"`       // 쓴 자리(지난 바퀴, 모르면 null)
	Cap      int      `json:"cap"`        // 자리 상한(설정)
	Adjust   string   `json:"adjust"`     // 실행 중 조정(손잡이 seats@·pause@ — 없으면 빈 글)
	Why      string   `json:"why"`        // 지난 바퀴에 안 띄운 까닭(없으면 빈 글)
	Unreach  string   `json:"unreach"`    // 끊김 비고(없으면 빈 글)
	Line     string   `json:"line"`       // 사람용 줄 그대로
}

func machineJSON() {
	recs := loadRecs()
	rn := reachNotes()
	now := time.Now().Unix()
	rows := []machRow{}
	for _, mc := range machines() {
		x, ok := recs[mc.Name]
		used := x.used
		if !ok {
			used = -1
		}
		cp := int(num(mc.Cap))
		r := machRow{Machine: mc.Name, Cap: cp, Adjust: machAdj(mc.Name), Why: x.why, Unreach: strings.TrimSpace(rn[mc.Name]),
			Line: loadLine(mc.Name, x.l, used, cp, machAdj(mc.Name), x.why) + rn[mc.Name]}
		if x.l.OK {
			l1, c, p := x.l.L1, x.l.Cores, x.l.Per()
			r.Load1, r.Cores, r.PerCore = &l1, &c, &p
			if x.l.At > 0 {
				age := now - x.l.At
				r.LoadAgeS = &age
			}
		}
		if ok {
			u := used
			r.Used = &u
		}
		rows = append(rows, r)
	}
	printJSON(rows)
}

// takeDry: 인자 어디에 있든 --dry-run을 떼어 낸다
func takeDry(args []string) ([]string, bool) {
	var out []string
	d := false
	for _, a := range args {
		if a == "--dry-run" {
			d = true
			continue
		}
		out = append(out, a)
	}
	return out, d
}

// preview: 미리 보기 한 줄(표준 출력, 「미리 보기:」 머리 — 아무것도 안 바꿨다)
func preview(format string, a ...any) {
	fmt.Printf("미리 보기: "+format+" (--dry-run — 바꾸지 않음)\n", a...)
}

// rmPreview: rm --dry-run — 잠금 없이 읽기만. 주차 줄의 거절은 실제 rm과 같은 글·종료 2
func rmPreview(name string, force bool) {
	r := qrowW(name)
	if r == nil {
		noLane(name) // 실제 rm과 같은 글·종료 1(감사 atozkxr 낮음 1)
	}
	if r.F(qEarly) == "parked" && !force {
		refuse("%s: 주차 중(refs/park/%s) — 먼저 해 둔 준비 커밋이 있어 지우고 다시 넣으면 새로 뜬다. 선행만 바꿀 거면 hc-fill edit %s --after <선행>, 그래도 지우려면 --force", name, name, name)
	}
	preview("뺌 %s (상태 %s, 떠 있는 터미널은 그대로) · 되돌리기 %s", name, r.F(qState), rmUndo(r))
}

// qrowW: 그 이름 줄(없으면 nil). 대기열 파일이 있는데 못 읽으면 「없다」가 아니라 실제 명령과 같은 글·종료 1(qRowsW — R-29,
// 감사 atozkxr 중간 1: 미리 보기가 못 읽음을 「대기열에 없다」로 읽었다). 잠금 없이 읽기만
func qrowW(n string) Row {
	for _, r := range qRowsW() {
		if r.F(qName) == n {
			return r
		}
	}
	return nil
}

// prioPreview: prio --dry-run — 잠금 없이 읽기만. 없는 레인은 실제 prio와 같은 글·종료
func prioPreview(name, p string) {
	r := qrowW(name)
	if r == nil {
		noLane(name)
	}
	s := fmt.Sprintf("우선 %s: %s → %s · 되돌리기 hc-fill prio %s %s", name, plabel(r.F(qPrio)), plabel(p), name, plabel(r.F(qPrio)))
	if r.F(qState) != "wait" {
		s += " · 이미 병합기 큐에 냈으면 큐 메타 priority=도"
	}
	preview("%s", s)
}

// knobPreview: knob set·unset(과 machine <기계>) --dry-run — 이름·값·고정 검사는 실제와 같고 저장 파일은 안 쓴다
func knobPreview(name, what string, force bool, now time.Time) error {
	if o, ok := knobEntries()[name]; ok && o.Pin && o.Live(now) && !force {
		return fmt.Errorf("%w: %s(누가 %s, 왜 %s) — 바꾸려면 --force", knob.ErrPinned, o.Name, o.By, o.Why)
	}
	cur := "설정 값"
	if d, ok := knob.Find(name); ok {
		if v, src, _ := knob.Effective(d, knobEntries(), confRaw(name), now); v != "" {
			cur = v + "(" + string(src) + ")"
		}
	}
	preview("손잡이 %s %s (지금 %s) — 다음 바퀴부터", name, what, cur)
	return nil
}

// next: 흔한 실패 뒤 「다음:」 한 줄(오류 출력). 앞 줄의 글·종료 코드는 그대로 — 부르는 쪽이 이 뒤에 끝낸다
func next(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "hc-fill: 다음: "+format+"\n", a...)
}

// noLane: 「대기열에 없다: <레인>」(글·종료 코드는 die 그대로) 앞에 다음 할 일을 붙인다
func noLane(n string) {
	dieNext("hc-fill ls로 대기열의 레인 이름을 본다(끝난 레인은 대기열에서 빠진다)", "대기열에 없다: %s", n)
}

// dieNext: die와 같은 첫 줄·종료 코드에 「다음:」 둘째 줄을 더한다(hc-lane 인자 오류의 「대신 …?」 줄과 같은 꼴)
func dieNext(hint, format string, a ...any) {
	die("%s\nhc-fill: 다음: %s", fmt.Sprintf(format, a...), hint)
}
