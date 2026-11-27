package main

// ego 작업 공간 쓸기(2026-11-27 ht-sweep-ego-spaces, docs/hc-sweep/ego-spaces.md): 에이전트가 만들고 지금도 쥔 ego 브라우저 작업 공간이
// 손잡이 sweep_ego_idle분 동안 그대로면(목록이 주는 칸의 지문이 안 바뀜) finish({ keep: [] })로 닫는다 — 유휴 자원 회수(idle reaper).
// 목록에 마지막 사용 시각이 없어 지문이 바뀐 때를 hc-sweep이 스스로 적는다. ego가 안 떠 있으면 부르지 않는다(ego-browser가 앱을 띄운다).
// 사용자 공간·사용자에게 넘긴 공간·모르는 소유 값은 건드리지 않는다. 호출이 상한을 넘기거나 출력을 못 읽으면 모름 — 기록을 바꾸지 않는다.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
)

const (
	egoMark    = "HCSWEEP "
	egoTimeout = 20 * time.Second
	egoListJS  = `console.log("` + egoMark + `" + JSON.stringify(await listTaskSpaces()))`
)

var (
	egoServiceRe = regexp.MustCompile(`/Contents/MacOS/ego lite( |$)`) // 앱 본체(도우미는 「ego Helper (…)」)
	egoCmdRe     = regexp.MustCompile(`(^|/)ego-browser( |$)`)
)

// egoSpace: listTaskSpaces() 한 칸. 포인터는 칸이 빠졌는지 가르려고(판이 달라 칸이 없으면 모름)
type egoSpace struct {
	ID        *int      `json:"id"`
	Name      *string   `json:"name"`
	CreatedBy *string   `json:"createdBy"`
	Ownership *string   `json:"ownership"`
	TaskID    *string   `json:"taskId"`
	ProfileID *string   `json:"profileId"`
	Titles    *[]string `json:"recentTabTitles"`
}

// egoSeen: 기록 한 칸 — 지문, 지문이 그 값이 된 때·마지막으로 본 때(유닉스 초), 닫기를 ego가 지켰나(지문이 바뀔 때까지 다시 안 함)
type egoSeen struct {
	FP    string `json:"fp"`
	Since int64  `json:"since"`
	Seen  int64  `json:"seen"`
	Kept  bool   `json:"kept,omitempty"`
}

// egoParse: 표식 줄의 JSON 배열. 표식 줄이 없거나 하나라도 칸이 빠지면 false
func egoParse(out string) ([]egoSpace, bool) {
	for _, l := range strings.Split(out, "\n") {
		j, ok := strings.CutPrefix(strings.TrimSpace(l), egoMark)
		if !ok {
			continue
		}
		var s []egoSpace
		if json.Unmarshal([]byte(j), &s) != nil {
			return nil, false
		}
		for _, e := range s {
			if e.ID == nil || e.Name == nil || e.CreatedBy == nil || e.Ownership == nil || e.TaskID == nil || e.ProfileID == nil || e.Titles == nil {
				return nil, false
			}
		}
		return s, true
	}
	return nil, false
}

// egoMine: 닫을 후보 — 에이전트가 만들었고 지금 에이전트가 쥠(이름이 빈 것은 모름). 그 밖의 값은 모두 사용자 것으로 본다
func egoMine(e egoSpace) bool {
	return *e.CreatedBy == "agent" && *e.Ownership == "agent" && *e.Name != ""
}

func egoFP(e egoSpace) string {
	b, _ := json.Marshal([]any{*e.Name, *e.CreatedBy, *e.Ownership, *e.TaskID, *e.ProfileID, *e.Titles})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

// egoCloseJS: 닫기 직전에 목록을 다시 보아 지문 칸이 그대로일 때만 finish — 그 사이 누가 쓰기 시작했으면 changed.
// 닫혔는지는 finish 영수증의 closedSpace로 본다(닫은 직후 목록에는 잠깐 남아 있을 수 있다 — 실제 확인 때 「지켜짐」으로 잘못 적혔다)
func egoCloseJS(e egoSpace) string {
	want, _ := json.Marshal(map[string]any{"id": *e.ID, "name": *e.Name, "taskId": *e.TaskID, "profileId": *e.ProfileID, "titles": *e.Titles})
	return `const w = ` + string(want) + `;
const f = () => listTaskSpaces().then((l) => l.find((s) => s.id === w.id));
const e = await f();
const same = e && e.createdBy === "agent" && e.ownership === "agent" && e.name === w.name && e.taskId === w.taskId &&
  e.profileId === w.profileId && JSON.stringify(e.recentTabTitles) === JSON.stringify(w.titles);
if (!e) console.log("` + egoMark + `gone");
else if (!same) console.log("` + egoMark + `changed");
else { const t = await taskSpace(w.id); const r = await t.finish({ keep: [] }); console.log("` + egoMark + `" + (r && r.closedSpace === true ? "closed" : "kept")); }`
}

func egoLoad(state string) map[string]egoSeen {
	m := map[string]egoSeen{}
	if b, err := os.ReadFile(state); err == nil && json.Unmarshal(b, &m) != nil {
		return map[string]egoSeen{} // 깨진 기록은 처음 봄으로(시계가 다시 시작할 뿐 — 일찍 닫지 않는다)
	}
	return m
}

// egoCheck: 한 바퀴. 찍을 줄들
func egoCheck(ego, machine string, idle time.Duration, state string, dry bool, r runner, report func(msg, key string) bool, now time.Time) []string {
	ps := r(15*time.Second, append(os.Environ(), "LC_ALL=C"), "/bin/ps", "-axww", "-o", "command=")
	if ps.err != nil || ps.rc != 0 {
		return []string{"ego 공간 쓸기: 모름(ps 실패 " + rcText(ps) + ") — 다음 바퀴에"}
	}
	busy := false
	up := false
	for _, l := range strings.Split(ps.out, "\n") {
		up = up || egoServiceRe.MatchString(l)
		busy = busy || egoCmdRe.MatchString(strings.TrimSpace(l))
	}
	if !up {
		return nil // ego가 안 떠 있음 — 부르면 앱을 띄운다
	}
	ls := r(egoTimeout, nil, ego, "nodejs", "-e", egoListJS)
	spaces, ok := egoParse(ls.out + "\n" + ls.errs) // ego-browser는 console.log를 표준 오류로 낸다(ego lite 0.6.3)
	if ls.err != nil || ls.rc != 0 || !ok {
		first, _, _ := strings.Cut(strings.TrimSpace(ls.errs+"\n"+ls.out), "\n")
		return []string{fmt.Sprintf("ego 공간 쓸기: 모름(목록 %s, 읽음 %v: %s) — 기록 그대로, 다음 바퀴에", rcText(ls), ok, cutRunes(first, 160))}
	}
	old, rec := egoLoad(state), map[string]egoSeen{}
	var due []egoSpace
	for _, e := range spaces {
		if !egoMine(e) {
			continue
		}
		id, fp := fmt.Sprint(*e.ID), egoFP(e)
		p, had := old[id]
		if !had || p.FP != fp || now.Unix()-p.Seen > int64(idle/time.Second) { // 처음 봄·바뀜·오래 못 봄(그 사이를 그대로로 세지 않음)
			p = egoSeen{FP: fp, Since: now.Unix()}
		}
		p.Seen = now.Unix()
		rec[id] = p
		if !p.Kept && now.Unix()-p.Since >= int64(idle/time.Second) {
			due = append(due, e)
		}
	}
	var lines, closed []string
	mins := func(e egoSpace) int64 { return (now.Unix() - rec[fmt.Sprint(*e.ID)].Since) / 60 }
	if len(due) > 0 && busy {
		lines = append(lines, fmt.Sprintf("ego 공간 쓸기: ego-browser 명령이 도는 중 — 이번 바퀴는 %d개를 안 닫음", len(due)))
		due = nil
	}
	if dry { // 말만 — 기록도 안 쓴다
		for _, e := range due {
			lines = append(lines, fmt.Sprintf("ego 공간 (닫을 것): %s (#%d, %d분 그대로)", *e.Name, *e.ID, mins(e)))
		}
		return lines
	}
	for _, e := range due {
		id := fmt.Sprint(*e.ID)
		res := r(egoTimeout, nil, ego, "nodejs", "-e", egoCloseJS(e))
		st := ""
		for _, l := range strings.Split(res.out+"\n"+res.errs, "\n") {
			if s, ok := strings.CutPrefix(strings.TrimSpace(l), egoMark); ok {
				st = s
			}
		}
		if res.err != nil || res.rc != 0 || st == "" {
			lines = append(lines, fmt.Sprintf("ego 공간 쓸기: %s (#%d) 닫기 모름(%s) — 남은 것은 다음 바퀴에", *e.Name, *e.ID, rcText(res)))
			break // 막힌 ego에 더 부르지 않는다(바퀴가 늦어지지 않게)
		}
		switch st {
		case "closed":
			closed = append(closed, fmt.Sprintf("%s (#%d, %d분 그대로)", *e.Name, *e.ID, mins(e)))
			delete(rec, id)
		case "gone":
			delete(rec, id)
		case "changed":
			rec[id] = egoSeen{FP: "-", Since: now.Unix(), Seen: now.Unix()} // 다음 바퀴 목록의 지문으로 다시 센다
		default: // kept: 사용자 탭이 남아 ego가 지킴
			p := rec[id]
			p.Kept = true
			rec[id] = p
			lines = append(lines, fmt.Sprintf("ego 공간 쓸기: %s (#%d) 지켜짐(사용자 탭이 남음) — 바뀔 때까지 다시 안 함", *e.Name, *e.ID))
		}
	}
	if b, err := json.Marshal(rec); err != nil || os.MkdirAll(filepath.Dir(state), 0o755) != nil || atomic.WriteFile(state, b, 0o644) != nil {
		lines = append(lines, "ego 공간 쓸기: 기록을 못 씀("+state+")")
	}
	if len(closed) > 0 {
		sort.Strings(closed)
		msg := fmt.Sprintf("ego 작업 공간 %d개를 닫음(%s, 그대로 %d분 넘김): %s", len(closed), machine, int(idle/time.Minute), strings.Join(closed, ", "))
		if !report(msg, "") {
			msg += " — 알림 실패(hc-lane report)"
		}
		lines = append(lines, "ego 공간 쓸기: "+msg)
	}
	return lines
}

// egoFind: ego-browser 경로. HC_SWEEP_EGO가 off면 안 함, 다른 값이면 그 경로(시험)
func egoFind() string {
	switch v := os.Getenv("HC_SWEEP_EGO"); v {
	case "off":
		return ""
	case "":
	default:
		return v
	}
	if p, err := exec.LookPath("ego-browser"); err == nil {
		return p
	}
	return ""
}

// egoRun: 손잡이 0이거나 ego-browser가 없으면(리눅스 등) 아무것도 안 한다
func egoRun(dry bool, r runner, idle time.Duration) {
	if idle <= 0 || runtime.GOOS == "windows" {
		return
	}
	ego := egoFind()
	if ego == "" {
		return
	}
	lane := filepath.Join(binDir, "hc-lane")
	report := func(msg, key string) bool { return laneReport(r, lane, "hc-ego-idle", msg, key) }
	for _, l := range egoCheck(ego, sweepMachine(), idle, filepath.Join(stateHome(), "hc-sweep", "ego-spaces.json"), dry, r, report, time.Now()) {
		fmt.Println(l)
	}
}
