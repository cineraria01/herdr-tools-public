package main

// 바퀴 계측(2026-11-24 fill-tick-cost — 바퀴 간격 중앙 93초인데 어디서 쓰는지 몰랐다): run 한 바퀴의 단계마다 걸린 ms와 바깥 명령
// 횟수를 바퀴 끝(exit 훅)에 로그 한 줄로 찍고 <DATA>/tick.tsv 끝에 남긴다(최근 tickKeep 줄). status가 그 마지막 줄을 보인다.
// 늘 켜 둔다 — 단계 경계마다 시각 하나, 바깥 명령마다 지도 한 칸 더하기가 전부다

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const tickKeep = 50 // tick.tsv에 남길 바퀴 수

var tk struct {
	mu     sync.Mutex
	on     bool
	t0     time.Time
	cur    string
	curT   time.Time
	stages []tkStage
	calls  map[string]int // "hc exec" 등 → 횟수
	ccur   int            // 지금 단계의 바깥 명령 수
}

type tkStage struct {
	name  string
	ms    int64
	calls int
}

// tickStart: 바퀴 계측을 켠다(run만). 끝은 exit 훅이 찍는다 — 바퀴가 중간에 exit·die로 끝나도 거기까지 찍힌다
func tickStart() {
	tk.mu.Lock()
	tk.on, tk.t0, tk.cur, tk.curT, tk.stages, tk.calls, tk.ccur = true, time.Now(), "준비", time.Now(), nil, map[string]int{}, 0
	tk.mu.Unlock()
	exitHooks = append(exitHooks, tickEnd)
}

// tickMark: 앞 단계를 닫고 name 단계를 연다
func tickMark(name string) {
	tk.mu.Lock()
	defer tk.mu.Unlock()
	if !tk.on {
		return
	}
	tickClose(time.Now())
	tk.cur, tk.curT = name, time.Now()
}

func tickClose(now time.Time) {
	tk.stages = append(tk.stages, tkStage{tk.cur, now.Sub(tk.curT).Milliseconds(), tk.ccur})
	tk.ccur = 0
}

// tickCall: 바깥 명령 하나(Cmd.Run). 열쇠는 이름과 첫 인자(hc exec·hc get·git fetch…) — git -C <경로> 꼴은 -C를 건너뛴다
func tickCall(name string, args []string) {
	tk.mu.Lock()
	defer tk.mu.Unlock()
	if !tk.on {
		return
	}
	tk.calls[callKey(name, args)]++
	tk.ccur++
}

func callKey(name string, args []string) string {
	if name != "hc" && name != "git" && name != "hc-lane" {
		return name // sh -c <본문> 등 — 본문은 열쇠가 아니다
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if name == "git" && (a == "-C" || a == "-c") {
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		return name + " " + a
	}
	return name
}

// tickEnd: 바퀴 끝 — 로그 한 줄과 tick.tsv(dry-run은 로그만)
func tickEnd() {
	tk.mu.Lock()
	if !tk.on {
		tk.mu.Unlock()
		return
	}
	now := time.Now()
	tickClose(now)
	tk.on = false
	row := tickRow(now.Unix(), now.Sub(tk.t0).Milliseconds(), tk.stages, tk.calls)
	tk.mu.Unlock()
	say("%s", tickLine(row))
	if dry {
		return
	}
	old, _ := readFile(DATA + "/tick.tsv")
	ls := append(lines(old), row)
	if len(ls) > tickKeep {
		ls = ls[len(ls)-tickKeep:]
	}
	if err := writeAtomic(DATA+"/tick.tsv", strings.Join(ls, "\n")+"\n"); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: 바퀴 계측: %v\n", err)
	}
}

// tickRow: tick.tsv 한 줄 「<끝 초>\t<바퀴 ms>\t<단계=ms/명령수,…>\t<명령=횟수,…>」(0ms·명령 0 단계는 뺀다, 명령은 많은 것부터)
func tickRow(end, ms int64, st []tkStage, calls map[string]int) string {
	var s []string
	for _, x := range st {
		if x.ms > 0 || x.calls > 0 {
			s = append(s, fmt.Sprintf("%s=%d/%d", x.name, x.ms, x.calls))
		}
	}
	ks := make([]string, 0, len(calls))
	for k := range calls {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		if calls[ks[i]] != calls[ks[j]] {
			return calls[ks[i]] > calls[ks[j]]
		}
		return ks[i] < ks[j]
	})
	var c []string
	for _, k := range ks {
		c = append(c, fmt.Sprintf("%s=%d", k, calls[k]))
	}
	return fmt.Sprintf("%d\t%d\t%s\t%s", end, ms, dash(strings.Join(s, ",")), dash(strings.Join(c, ",")))
}

// tickParse: tick.tsv 한 줄 → 바퀴 ms, 가장 긴 단계(이름·ms), 바깥 명령 합. 꼴이 아니면 ok 거짓
func tickParse(l string) (ms int64, long string, longMS int64, ncalls int, ok bool) {
	f := strings.Split(l, "\t")
	if len(f) != 4 || !isDigits(f[0]) || !isDigits(f[1]) {
		return 0, "", 0, 0, false
	}
	ms, _ = strconv.ParseInt(f[1], 10, 64)
	longMS = -1
	if f[2] != "-" {
		for _, x := range strings.Split(f[2], ",") {
			n, v, _ := strings.Cut(x, "=")
			a, _, _ := strings.Cut(v, "/")
			if d, err := strconv.ParseInt(a, 10, 64); err == nil && d > longMS {
				long, longMS = n, d
			}
		}
	}
	if f[3] != "-" {
		for _, x := range strings.Split(f[3], ",") {
			_, v, _ := strings.Cut(x, "=")
			n, _ := strconv.Atoi(v)
			ncalls += n
		}
	}
	return ms, long, longMS, ncalls, true
}

// tickLine: 로그·status에 찍는 한 줄
func tickLine(row string) string {
	ms, long, lms, n, ok := tickParse(row)
	if !ok {
		return "바퀴 계측: 읽지 못함"
	}
	f := strings.Split(row, "\t")
	return fmt.Sprintf("바퀴 계측: %s · 가장 긴 단계 %s %s · 바깥 명령 %d번 · 단계 %s · 명령 %s", secs(ms), dash(long), secs(lms), n, f[2], f[3])
}

func secs(ms int64) string { return strconv.FormatFloat(float64(ms)/1000, 'f', 1, 64) + "초" }

// tickStatus: status 끝 줄 — 최근 바퀴(tick.tsv 끝)와 최근 바퀴들의 걸린 시간 중앙값. 기록이 없으면 빈 글
func tickStatus() string {
	var ms []int64
	last := ""
	for _, l := range readLines(DATA + "/tick.tsv") {
		if m, _, _, _, ok := tickParse(l); ok {
			ms, last = append(ms, m), l
		}
	}
	if last == "" {
		return ""
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i] < ms[j] })
	end, _ := strconv.ParseInt(strings.SplitN(last, "\t", 2)[0], 10, 64)
	return fmt.Sprintf("%s (%s 전 · 최근 %d바퀴 중앙 %s)", tickLine(last), stDur(time.Now().Unix()-end), len(ms), secs(ms[len(ms)/2]))
}

// LSSNAP: 이번 바퀴 터미널 표(TERMS·CONN, readTables)를 「그 주소에 터미널 없음」의 근거로 써도 되는 동안 — 표를 읽은 뒤부터
// 이 바퀴가 새 레인을 띄우기 전까지(띄운 주소는 표에 없다)
var LSSNAP bool

// lsGone: 표를 믿을 수 있는 동안, 주소의 기계가 이 기계이거나 목록 앞뒤로 연결이었는데(CONN) 표에 그 주소가 없으면 참 — joblast가 hc get 없이
// 주소·이름으로 사건을 고른다(hc get이 끝남·모름일 때와 같은 길). 표에 「없음」은 alive()의 gone과 같은 근거다. 종료를 확정해야 하는 곳
// (받은편지 회신 — 「hc ls에 없어도 not_found가 아니면 실패를 쓰지 않음」)은 pget을 직접 불러 이것을 쓰지 않는다.
// 2026-11-24 fill-tick-cost: 끝난 떠 있음 줄마다 사건·멈춤 단계가 hc get을 한 번씩 불러(줄 200개에 433번) 바퀴가 줄 수에 비례해 늘었다
func lsGone(addr string) bool {
	if !LSSNAP || addr == "" || addr == "-" {
		return false
	}
	m := LOCAL
	if i := strings.IndexByte(addr, '/'); i >= 0 {
		m = addr[:i]
	}
	if m != LOCAL && !connHas(m) {
		return false
	}
	for _, t := range TERMS {
		if t.Addr == addr {
			return false
		}
	}
	return true
}

// restFor: watch가 다음 바퀴까지 쉴 시간 = 주기 − 이번 바퀴에 든 시간, 하한 floor(바퀴가 주기를 넘겨도 floor는 쉰다)
func restFor(every, took, floor time.Duration) time.Duration {
	if r := every - took; r > floor {
		return r
	}
	return floor
}
