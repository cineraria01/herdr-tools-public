package main

// 저장소 읽기 막힘 격리(ht-worker-cordon, 2026-11-27, docs/hc-fill/cordon.md — 사용자 「그 방식으로 하자 자동 격리로」. 업계의 건강 점검 ·
// 격리(cordon) · 자동 복귀). 워커의 코어가 띄운 프로그램만 저장소 폴더를 못 읽게 된 기계(OS 권한 창에 답할 사람이 없음)가 연결·부하로는
// 「가장 한가한 좋은 기계」로 보여 레인·병합 검증이 그 기계에서 멈췄다(3시간 넘게 사람이 발견).
//   - 점검: 바퀴마다 이미 부르는 원격 조각(loadSnip — 부하 + hc-lane merger --spec-idle)에 그 기계 repo 줄 폴더들의 읽기(--repo-read,
//     cmd/hc-merger/reporead.go)를 얹는다. 그 조각을 이 바퀴에 안 부른 기계(자리 참·레인 없음·대기 줄 없음)만 cordonEvery마다 따로
//     한 번(격리·나쁨인 기계는 바퀴마다). 판정은 좋음·나쁨·모름(hc-sweep 건강 점검과 같은 낱말 — 모름은 나쁨이 아니다):
//     나쁨 = 어느 폴더든 읽기 오류·상한 안에 안 끝남, 모름 = 호출 실패·잘린 출력(끝 줄 없음)·옛 판 워커(줄 없음)·폴더 수가 다름.
//     모름은 기록을 바꾸지 않는다(격리하지도 풀지도 않음).
//   - 격리 단위는 기계: 고장 꼴(코어 문맥의 폴더 권한)이 기계 전체에 걸리고, 거르기를 machRoom 한 곳(멈춤과 같은 길 — 배정·감사·
//     advise 빈 자리·놀고 있음·치우침이 함께 맞음)에 둔다. 어느 폴더든 나쁘면 그 기계가 나쁨.
//   - 나쁨이 손잡이 cordon분(기본 5) 이어지면 격리: 새 레인·그 레인의 감사를 안 띄움(떠 있는 레인·터미널은 그대로). 병합기에는 공통 손잡이
//     원격 참조에 기한 있는 「cordon@<기계>」 줄로(knobRemoteOf — 판정은 여기 한 곳, 병합기는 읽기만, 기한이 지나면 모름 = 빼지 않음).
//     사람의 pause@와 섞지 않는다(다른 기록·다른 이름).
//   - 알림: 문턱에 한 번(막힘, 운영 사건 hc-fill-cordon — 그 기계 레인 주인마다, 레인이 없으면 받는 쪽 규칙 그대로), 좋음으로 돌아오면 다음
//     바퀴에 풀고 한 번(중간). cordon 0이면 격리 없이 기본 문턱에 알리기만. 실패한 알림은 같은 키로 다음 바퀴에.
//   - 기록 DATA/cordon.tsv(reach.tsv와 같은 꼴, 칸은 cols.go cd…) — hc-fill machine·status·state·advise가 읽는다.
//   - 점검은 여럿(cordonCks — ht-cordon-remote-auth가 원격 로그인 점검 cdAuth를 더함, cordonauth.go): 판정 낱말·기록 꼴·문턱·알림 길은 같고
//     점검마다 제 기록 파일·글·간격·판정. 기계는 어느 점검이든 격리면 격리(한 점검의 모름·좋음이 다른 점검의 나쁨을 지우지 않음).

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"herdr-tools/internal/knob"
)

// cordonCk: 격리 점검 하나
type cordonCk struct {
	key   string // 기록 DATA/cordon<key>.tsv · 알림 키 hf:cordon<key>:… (저장소 읽기는 빈 값 — 오늘 기록·키 그대로)
	name  string // 「저장소 읽기」 — 기계 줄·알림
	bad   string // 「막힘」
	pre   string // 격리 기계 줄·상태 칸에서 까닭 앞에(저장소 읽기는 빈 값 — 오늘 글 그대로)
	look  string // 알림의 사람이 볼 것
	every int64  // 좋은 기계를 다시 보는 간격(초)
	// verdict: 이번 바퀴 그 기계 판정 — 1 좋음 · 2 나쁨(까닭) · 3 물었는데 모름(좋은 기계의 다음 확인만 간격 뒤로) · 0 못 물음
	verdict func(m string) (int, string)
}

func (c *cordonCk) why() string  { return "격리(" + c.name + " " + c.bad + ")" } // 안 띄운 까닭(machRoom·MWHY)
func (c *cordonCk) file() string { return DATA + "/cordon" + c.key + ".tsv" }

// cordonWhy: 저장소 읽기 막힘으로 안 띄운 까닭
const cordonWhy = "격리(저장소 읽기 막힘)"

// cdRepo: 저장소 읽기 — 좋은 기계는 2분마다(격리 문턱 5분 + 이 간격 + 한 바퀴가 합격 기준 10분 안에 들게)
var cdRepo = &cordonCk{"", "저장소 읽기", "막힘", "", "그 기계에서 worker-mac-check(코어가 띄운 프로그램의 폴더 권한 — OS 권한 창이 떠 있을 수 있다)", 120, cordonVerdict}

var cordonCks = []*cordonCk{cdRepo, cdAuth}

// due: 이번 바퀴에 그 기계를 물을 차례 — 좋고 알림이 안 남은 기계는 간격마다만, 나쁘거나 격리 중·회복 알림이 남거나 처음 보면 바퀴마다
func (c *cordonCk) due(rows map[string]cordonRow, m string, now int64) bool {
	p, seen := rows[m]
	return !seen || p.St != "ok" || p.Alerted || now-p.At >= c.every
}

type cordonRow struct {
	M       string
	St      string // ok | bad
	Since   int64  // bad: 나쁨 시작 · ok: 알린 나쁨의 시작(회복 알림이 남음, Alerted) 또는 0
	At      int64  // 마지막 판정(좋음·나쁨) 때
	On      bool   // 격리 중
	Alerted bool   // bad: 나쁨 알림 보냄 · ok: 회복 알림이 남음
	Why     string // 마지막 나쁨 까닭(프로젝트 폴더: 오류)
}

func cordonFile() string { return cdRepo.file() }

// cordonLoad: 저장소 읽기 기록
func cordonLoad() map[string]cordonRow { return cdRepo.load() }

// load: 기록. 꼴이 다른(옛·깨진) 줄은 없는 것으로 — 그 기계는 처음 보는 것처럼 문턱부터 센다
func (c *cordonCk) load() map[string]cordonRow {
	o := map[string]cordonRow{}
	for _, r := range readRows(c.file()) {
		st, why := r.F(cdState), r.F(cdWhy)
		if why == "-" {
			why = ""
		}
		if r.NF() != cdNF || r.F(cdMach) == "" || st != "ok" && st != "bad" {
			continue
		}
		o[r.F(cdMach)] = cordonRow{r.F(cdMach), st, int64(atoi10(r.F(cdSince))), int64(atoi10(r.F(cdAt))), r.F(cdOn) == "1",
			r.F(cdAlerted) == "1", why}
	}
	return o
}

func b01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func (x cordonRow) line() string {
	return strings.Join([]string{x.M, x.St, fmt.Sprint(x.Since), fmt.Sprint(x.At), b01(x.On), b01(x.Alerted), dash(flat1(x.Why))}, "\t")
}

// CORDON: 이번 실행이 본 격리 기록(점검 key → 기계 → 줄, 처음 물을 때 읽고, cordonTick이 바꾼다)
var CORDON map[string]map[string]cordonRow

// cordonWhyOf: 그 기계를 격리한 첫 점검의 까닭(없으면 빈 값 — machRoom: 새 레인·감사를 안 띄움)
func cordonWhyOf(m string) string {
	if CORDON == nil {
		CORDON = map[string]map[string]cordonRow{}
		for _, c := range cordonCks {
			CORDON[c.key] = c.load()
		}
	}
	for _, c := range cordonCks {
		if CORDON[c.key][m].On {
			return c.why()
		}
	}
	return ""
}

// cordonOn: 그 기계가 어느 점검으로든 격리 중
func cordonOn(m string) bool { return cordonWhyOf(m) != "" }

// repoCordonOn: 저장소 읽기로 격리 중(작업 트리 읽기를 건너뛸 때 — overlap_actual.go)
func repoCordonOn(m string) bool { cordonWhyOf(m); return CORDON[cdRepo.key][m].On }

// repoPath: 그 기계의 repo 줄 하나(점검 차례)
type repoPath struct{ p, path string }

func repoPaths(m string) []repoPath {
	var o []repoPath
	for _, f := range confRows("repo") {
		if F1(f, 2) == m {
			if r := strings.TrimRight(repoOf(f[1], m), "/"); r != "" {
				o = append(o, repoPath{f[1], r})
			}
		}
	}
	return o
}

// repoCk: 한 기계의 읽기 점검 결과(폴더 차례대로 「ok」·「missing」·「bad <까닭>」)
type repoCk []string

// REPOCK: 이번 바퀴에 읽은 점검(기계 → 결과). 없는 기계는 모름
var REPOCK map[string]repoCk

// repoParse: 「repo-read <차례> …」 줄들과 끝 줄 「repo-read end <수>」. 끝 줄이 없거나(잘림·옛 판) 차례가 빠지면 모름(false)
func repoParse(out string) (repoCk, bool) {
	got := map[int]string{}
	n := -1
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(strings.TrimSpace(l), " ", 3)
		if len(f) < 3 || f[0] != "repo-read" {
			continue
		}
		if f[1] == "end" {
			if v, err := strconv.Atoi(f[2]); err == nil && v >= 0 && n < 0 {
				n = v
			}
			continue
		}
		i, err := strconv.Atoi(f[1])
		st, _, _ := strings.Cut(f[2], " ")
		if err == nil && i > 0 && (st == "ok" || st == "missing" || st == "bad") {
			got[i] = f[2]
		}
	}
	if n < 0 || len(got) != n {
		return nil, false
	}
	o := make(repoCk, n)
	for i := range o {
		if o[i] = got[i+1]; o[i] == "" {
			return nil, false
		}
	}
	return o, true
}

// loadTake: 원격 조각 출력 하나에서 부하(LOADS)와 읽기 점검(REPOCK)을 함께 받는다
func loadTake(m, out string) {
	if l := loadParse(out); l.OK {
		loadPut(m, l)
	}
	repoTake(m, out)
}

// repoTake: 읽기 점검과 원격 로그인 점검(cordonTick이 따로 부른 조각 — 부하는 loadTick이 안 읽는 기계라 모름 그대로 둔다).
// 원격 로그인은 loadSnip이 물은 조각(표식 줄 authAsked — 차례를 다시 셈하지 않는다: 감사 atp61ld 중간 1)의 끝 줄이 있는 온전한 출력에서만
func repoTake(m, out string) {
	ck, ok := repoParse(out)
	if !ok {
		return
	}
	asked := strings.Contains("\n"+out+"\n", "\n"+authAsked+"\n")
	loadMu.Lock()
	defer loadMu.Unlock()
	if REPOCK == nil {
		REPOCK = map[string]repoCk{}
	}
	REPOCK[m] = ck
	if asked {
		if AUTHCK == nil {
			AUTHCK = map[string]string{}
		}
		AUTHCK[m] = authLine(out)
	}
}

// cordonVerdict: 이번 바퀴 그 기계 판정 — 1 좋음 · 2 나쁨(까닭) · 0 모름(폴더 수가 설정과 다르면 모름)
func cordonVerdict(m string) (int, string) {
	ck, ok := REPOCK[m]
	ps := repoPaths(m)
	if !ok || len(ck) != len(ps) || len(ps) == 0 {
		return 0, ""
	}
	for i, r := range ck {
		if msg, bad := strings.CutPrefix(r, "bad "); bad || r == "bad" {
			return 2, fmt.Sprintf("%s %s: %s", ps[i].p, ps[i].path, msg)
		}
	}
	return 1, ""
}

// cordonTick: launchRows에서 loadTick 다음(띄우기 전) — 못 본 기계를 보고, 판정·격리·알림, 기록을 쓴다
func cordonTick() {
	now := nowUnix()
	prev := map[*cordonCk]map[string]cordonRow{}
	for _, c := range cordonCks {
		prev[c] = c.load()
	}
	var wg sync.WaitGroup
	for _, mc := range machines() {
		m := mc.Name
		loadMu.Lock() // 앞 기계로 띄운 고루틴이 repoTake로 쓴다(잠금 없이 읽으면 경합 — 작업 중 발견해 함께 고침)
		_, have := REPOCK[m]
		loadMu.Unlock()
		// 이 바퀴에 온전한 출력을 받았으면(have) 원격 로그인도 차례였다면 이미 물었다(repoTake)
		if dry || have || !cdRepo.due(prev[cdRepo], m, now) && !authDue(m) || len(repoPaths(m)) == 0 || m != LOCAL && !connHas(m) {
			continue
		}
		wg.Add(1)
		go func(m string) {
			defer wg.Done()
			if out, rc := onm(nil, m, loadSnip(m)); rc == 0 {
				repoTake(m, out)
			}
		}(m)
	}
	wg.Wait()
	CORDON = map[string]map[string]cordonRow{}
	for _, c := range cordonCks {
		next := c.judge(prev[c], now)
		CORDON[c.key] = next
		if dry || len(next) == 0 && !exists(c.file()) {
			continue
		}
		var ms []string
		for m := range next {
			ms = append(ms, m)
		}
		sort.Strings(ms)
		var b strings.Builder
		for _, m := range ms {
			b.WriteString(next[m].line() + "\n")
		}
		if err := writeAtomic(c.file(), b.String()); err != nil {
			say("격리 기록을 못 씀: %v", err)
		}
	}
}

// judge: 앞 기록 + 이번 판정 → 새 기록(모름인 기계는 앞 기록 그대로 — 물었는데 모름이면 좋은 기계의 다음 확인 때만 뒤로). 알림도 여기서
func (c *cordonCk) judge(prev map[string]cordonRow, now int64) map[string]cordonRow {
	lim := knobInt("cordon")
	at := lim // 알림 문턱 — 0(격리 안 함)이면 표 기본에 알리기만
	if at == 0 {
		d, _ := knob.Find("cordon")
		at = atoi10(d.Default)
	}
	out := map[string]cordonRow{}
	for m, x := range prev {
		out[m] = x
	}
	for _, mc := range machines() {
		m := mc.Name
		v, why := c.verdict(m)
		x, seen := prev[m]
		switch v {
		case 3: // 물었는데 모름(원격이 느림·죽음·속도 제한) — 판정은 그대로, 좋은 기계만 다음 확인을 간격 뒤로(장애 때 호출이 늘지 않게)
			if seen && (x.St != "ok" || x.Alerted) {
				continue
			}
			if !seen {
				x = cordonRow{M: m, St: "ok"}
			}
			x.At = now
		case 2:
			if !seen || x.St != "bad" {
				x = cordonRow{M: m, St: "bad", Since: now}
			}
			x.At, x.Why = now, why
			mins := (now - x.Since) / 60
			x.On = lim > 0 && mins >= int64(lim)
			if mins >= int64(at) && !x.Alerted {
				act := "격리: 새 레인·감사·병합 검증을 그 기계에 안 보냄(새 일만 막음 — 돌아오면 스스로 풂)"
				if !x.On {
					act = "격리 안 함(손잡이 cordon 0 — 알리기만)"
				}
				lanes := reachLanes(m, ROWS)
				msg := fmt.Sprintf("기계 %s %s %s %d분째(%s) — %s. 떠 있는 레인 %s은 그대로. 사람이 볼 것: %s",
					m, c.name, c.bad, mins, why, act, dash(strings.Join(lanes, ",")), c.look)
				say("%s", msg)
				x.Alerted = cordonSend("막힘", msg, fmt.Sprintf("hf:cordon%s:%s:%d", c.key, m, x.Since), lanes)
			}
		case 1:
			if !seen || x.St == "bad" && !x.Alerted {
				x = cordonRow{M: m, St: "ok"}
			} else if x.St == "bad" { // 알린 나쁨 — 회복 알림이 남음
				x = cordonRow{M: m, St: "ok", Since: x.Since, Alerted: true, Why: x.Why}
			}
			x.At, x.On = now, false
			if x.Alerted {
				msg := fmt.Sprintf("기계 %s %s 돌아옴 — %s 이어졌다가 풀림, 이 까닭의 격리를 푼다(다른 까닭이 없으면 새 일을 다시 받는다, 그동안 까닭 %s)",
					m, c.name, stDur(now-x.Since), dash(x.Why))
				say("%s", msg)
				if cordonSend("중간", msg, fmt.Sprintf("hf:cordon%s:%s:%d:up", c.key, m, x.Since), reachLanes(m, ROWS)) {
					x = cordonRow{M: m, St: "ok", At: now}
				}
			}
		default:
			continue
		}
		out[m] = x
	}
	return out
}

// cordonSend: 그 기계 레인 주인 지휘마다 한 줄(레인이 없으면 「-」 — 받는 쪽 규칙 그대로). 막힘은 aesc, 회복은 중간. 모두 남겼으면 참
func cordonSend(state, msg, key string, lanes []string) bool {
	owners := reachOwners(lanes)
	if len(owners) == 0 {
		owners = []string{"-"}
	}
	ok := true
	for _, own := range owners {
		k := key + ":" + own
		if state == "막힘" {
			ok = aesc("hc-fill-cordon", own, msg, k) == 0 && ok
			continue
		}
		to := own
		if to == "-" {
			to = ""
		}
		if dry {
			fmt.Println(P + "지휘로 넘길 hc-fill-cordon: " + msg)
		} else if hcLaneReport("중간", msg, "hc-fill-cordon", to, k) != 0 {
			ok = false
		}
	}
	return ok
}

// cordonNotes: 기계 → 점검마다 「격리 <얼마>째(<까닭>)」 또는 격리 전 「<점검> <막힘> <얼마>째(…)」 — hc-fill machine·status 기계 줄 끝
func cordonNotes() map[string]string {
	o := map[string]string{}
	now := nowUnix()
	for _, c := range cordonCks {
		for m, x := range c.load() {
			switch {
			case x.St != "bad":
			case x.On:
				o[m] += "  격리 " + stDur(now-x.Since) + "째(" + c.pre + x.Why + ")"
			default:
				o[m] += "  " + c.name + " " + c.bad + " " + stDur(now-x.Since) + "째(" + x.Why + " — 격리 전)"
			}
		}
	}
	return o
}

// cordonOnRows: 기계 → 격리 중인 점검 줄들(점검 차례) — 원격 줄·상태 한 장
func cordonOnRows() map[string][]cordonRow {
	o := map[string][]cordonRow{}
	for _, c := range cordonCks {
		for m, x := range c.load() {
			if x.On {
				x.Why = c.pre + x.Why
				o[m] = append(o[m], x)
			}
		}
	}
	return o
}

// cordonRemote: 원격 손잡이 참조에 실을 격리 줄(knobRemoteOf — 어느 점검이든 같은 줄 하나, 병합기는 까닭을 보지 않는다). 기한 = 마지막
// 판정을 10분 단위로 올린 때 + 10분 — 바퀴마다 내용이 바뀌어 push가 늘지 않게(격리 동안 10분에 한 번), 그리고 hc-fill이 그 기계를 안 보면
// 10~20분 뒤 병합기가 모름(빼지 않음)으로 돌아가게
func cordonRemote(o map[string]knob.Entry) {
	for m, xs := range cordonOnRows() {
		n := knob.CordonPre + m
		e := knob.Entry{Name: n, Value: "1", By: "hc-fill"}
		var ws []string
		for _, x := range xs {
			e.Until = max(e.Until, (x.At/600+2)*600)
			ws = append(ws, x.Why)
		}
		e.Why = strings.Join(ws, "; ")
		o[n] = e
	}
}
