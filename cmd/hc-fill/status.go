package main

// status 명령(2026-11-22 go-lane-status — 사용자가 「어디까지 됐어?」를 열 번 넘게 물었고 지휘가 매번 ls·병합기 줄·사건을 따로 뒤졌다):
// 레인마다 한 줄 — 단계·기계·지난 시간·예상(근거와 함께). 읽기만 한다(잠금·상태 파일 쓰기 없음).
// 원격 조회는 프로젝트마다 git ls-remote 한 번(병합기 줄 refs/merge-queue/*와 착지 표식 refs/hc-landed/*를 함께), hc ls·hc machine ls 한 번씩.

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// stIn: 한 줄의 단계를 정하는 재료
type stIn struct {
	State, Early string // 대기열 6칸(wait|up|sent)·14칸
	Live         string // 살아 있는 터미널 상태(없으면 빈 값), 기계가 연결 안 됨이면 "?"
	Last         Row    // last.tsv 줄(레인 상태 sha8 시각 한줄 …) — 없으면 nil
	Aud          string // 자동 감사 비고(anote), 감사 대상이 아니면 빈 값
	Pos          int    // 병합기 줄 차례(1부터) · 0 줄에 없음 · -1 줄을 못 읽음
	Pin          string // 대기 줄: 묶인 기계(--only)가 모두 상한·멈춤이라 못 뜬 까닭(pinned.go), 아니면 빈 값
	Bg           bool   // 살아 있는 배경 셸을 기다림(hangBgWait)
}

// stStage0: 단계 한 마디(stStage가 「대기열이 제출함」을 덧붙인다 — autosubmit.go)
func stStage0(in stIn) string {
	switch in.State {
	case "wait":
		if in.Pin != "" { // 선행은 들어갔다 — 「선행 기다림」이 아니라 진짜 까닭(ht-advise-only-stuck)
			return "대기(묶인 기계 " + in.Pin + ")"
		}
		if in.Early == "parked" {
			return "대기(주차 — 선행 기다림)"
		}
		return "대기"
	case "sent":
		return "되돌려줌"
	}
	ls, sha, msg := in.Last.F(lsState), in.Last.F(lsSHA), in.Last.F(lsMsg)
	switch {
	case in.Pos == 1: // 우선·병합기 쉼은 안 보니 「지금 넣는 중」이라 하지 않는다(감사 낮음 1)
		return "제출됨(병합기 줄 맨 앞)"
	case in.Pos > 1:
		return fmt.Sprintf("제출됨(병합기 줄 %d번째)", in.Pos)
	case ls == "착지":
		return "착지 " + sha
	case ls == "막힘":
		return "막힘: " + dash(msg)
	case ls == "제출" && in.Pos < 0:
		return "제출됨(병합기 줄 모름)"
	case ls == "제출":
		return "제출됨(줄에서 빠짐 — 결과 기다림)"
	case ls == "주차":
		return "주차"
	case in.Aud != "" && in.Aud != "자동 감사(완료 기다림)" && in.Aud != "옛 감사 기록":
		return in.Aud
	case ls == "완료":
		return "완료 " + sha
	case strings.HasPrefix(in.Live, "?"):
		return "모름(" + in.Live[1:] + ")"
	case in.Live != "" && in.Bg:
		return "배경 일 기다림(" + in.Live + ")"
	case in.Live != "":
		return "작업 중(" + in.Live + ")"
	}
	return "끝남"
}

var mqNameRe = regexp.MustCompile(`^refs/merge-queue/([0-9]{10})-(.+)$`)
var landedRe = regexp.MustCompile(`^refs/hc-landed/[^/]+/([0-9]+)$`)

// stRemote: ls-remote 출력에서 병합기 줄(레인 이름, 제출 시각 순)과 레인 착지 시각(초, 오름차순)
// ponytail: 줄 차례는 제출 시각만 — 병합기는 제출 기록의 우선을 먼저 본다(기록을 받아야 해서 빼 둠). 우선이 엇갈리면 차례가 다를 수 있다
func stRemote(out string) (queue []string, landed []int64) {
	type q struct{ name, lane string }
	var qs []q
	for _, l := range lines(out) {
		f := fields(l)
		if len(f) != 2 {
			continue
		}
		if m := mqNameRe.FindStringSubmatch(f[1]); m != nil {
			qs = append(qs, q{f[1], m[2]})
		} else if m := landedRe.FindStringSubmatch(f[1]); m != nil {
			if s, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				landed = append(landed, s)
			}
		}
	}
	sort.Slice(qs, func(i, j int) bool { return qs[i].name < qs[j].name })
	for _, x := range qs {
		queue = append(queue, x.lane)
	}
	sort.Slice(landed, func(i, j int) bool { return landed[i] < landed[j] })
	return queue, landed
}

// stBatchGap: 앞 표식과 이 초 안이면 같은 착지(병합기는 묶음 구성원마다 레인 표식을 몇 초 사이로 둔다 — 감사 중간 1)
const stBatchGap = 120

// stEstimate: (줄 차례 ÷ 한 번에 넣은 최대 개수, 올림) × 최근 착지 간격 중앙값. 표식을 착지 한 번으로 묶은 뒤 마지막 6번의
// 간격 5개(쉰 틈에 덜 흔들리게 평균 대신 중앙값). 착지가 두 번 안 되면 빈 값
// ponytail: 묶음 크기는 최근 6번에서 본 최대 — 병합기 설정(HC_LANE_MERGER_BATCH)은 그 기계에 있어 읽지 않는다
func stEstimate(pos int, landed []int64) string {
	if pos < 1 || len(landed) == 0 {
		return ""
	}
	var ev []int64 // 착지 한 번의 첫 표식 시각
	var sz []int   // 그 착지의 표식 수
	for i, t := range landed {
		if i > 0 && t-landed[i-1] <= stBatchGap {
			sz[len(sz)-1]++
			continue
		}
		ev, sz = append(ev, t), append(sz, 1)
	}
	if len(ev) < 2 {
		return ""
	}
	if len(ev) > 6 {
		ev, sz = ev[len(ev)-6:], sz[len(sz)-6:]
	}
	batch := 1
	for _, n := range sz {
		batch = max(batch, n)
	}
	var gaps []int64
	for i := 1; i < len(ev); i++ {
		gaps = append(gaps, ev[i]-ev[i-1])
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i] < gaps[j] })
	med := gaps[len(gaps)/2]
	rounds := (pos + batch - 1) / batch
	return fmt.Sprintf("~%s (줄 %d번째 → 착지 %d번 × 최근 착지 간격 중앙값 %s, 최근 착지 %d번·한 번에 최대 %d개)",
		stDur(int64(rounds)*med), pos, rounds, stDur(med), len(ev), batch)
}

// stDur: 초 → 「1h05m」·「12m」
func stDur(s int64) string {
	if s < 0 {
		s = 0
	}
	m := (s + 30) / 60
	if m >= 60 {
		return fmt.Sprintf("%dh%02dm", m/60, m%60)
	}
	return fmt.Sprintf("%dm", m)
}

// stAge <대기열 10칸(이 기계 분)>: 넣은 뒤 지난 시간. 못 읽으면 -
func stAge(added string, now time.Time) string {
	t, err := time.ParseInLocation("2006-01-02T15:04", added, time.Local)
	if err != nil {
		return "-"
	}
	return stDur(int64(now.Sub(t).Seconds()))
}

// stLive: 떠 있는 레인의 터미널 상태(주소·이름이 함께 맞을 때만). 빈 값 = 목록에 없음(끝남), 「?까닭」 = 모름 —
// hc ls를 못 읽음(ls는 여기서 멈춘다), 그 기계가 연결 안 됨, 연결을 모르는데 원격 레인이 목록에 없음(ls와 같은 글)
func stLive(terms []lsTerm, termsOK bool, conn []string, connKnown bool, addr, name string) string {
	m := LOCAL
	if i := strings.IndexByte(addr, '/'); i >= 0 {
		m = addr[:i]
	}
	switch {
	case !termsOK:
		return "?hc ls 못 읽음"
	case connKnown && m != LOCAL && !has(conn, m):
		return "?연결 안 됨"
	}
	for _, t := range terms {
		if t.Addr == addr && t.Name == name {
			return t.Status
		}
	}
	if !connKnown && m != LOCAL {
		return "?연결 확인 못 함"
	}
	return ""
}

// stPad: 화면 폭으로 채움(ls의 padB는 셸 printf처럼 바이트라 한글 칸이 어긋난다 — status는 Go 판만이라 맞출 셸 출력이 없다)
// ponytail: U+1100 이상은 두 칸으로 셈(한글·한자 기준). 「—」 같은 좁은 기호도 두 칸으로 쳐 한 칸 남을 수 있다
func stPad(s string, n int) string {
	w := 0
	for _, r := range s {
		if w++; r >= 0x1100 {
			w++
		}
	}
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func stLine(c ...string) string {
	return strings.TrimRight(stPad(c[0], 24)+" "+stPad(c[1], 12)+" "+stPad(c[2], 8)+" "+stPad(c[3], 36)+" "+c[4], " ")
}

func init() { register("status", statusCmd) }

// status [--owner <주인>|--mine]
func statusCmd(args []string) {
	owner := ""
	switch {
	case len(args) == 0:
	case len(args) == 2 && args[0] == "--owner" && args[1] != "":
		owner = args[1]
	case len(args) == 1 && args[0] == "--mine":
		if owner = os.Getenv("HC_TERMINAL_ID"); owner == "" {
			die("--mine: 이 터미널 주소($HC_TERMINAL_ID)가 없다 — --owner <주소>로")
		}
	default:
		usage()
	}
	var rows []Row
	for _, l := range readLines(Q) {
		f := Row(lsSplit(l, qNF))
		if strings.HasPrefix(f.F(qAudit), "audit=") || (owner != "" && f.F(qOwner) != owner) {
			continue // 감사 줄은 그 레인 줄의 단계로 보인다
		}
		rows = append(rows, f)
	}
	fmt.Println(stLine("레인", "기계", "넣은 뒤", "단계", "예상·근거"))
	if len(rows) == 0 { // 병합기 한 줄(maPrint, mergeralive.go)·바퀴 계측 줄(statusTick)은 표 끝에 — 줄이 없어도
		maPrint()
		statusTick()
		exit(0)
	}
	// 터미널·연결은 한 번씩. 못 읽으면 살아 있음을 모르는 채로(끝남이 아니라 모름)
	var terms []lsTerm
	termsOK := false
	if out, rc := run("hc", "ls", "--json"); rc == 0 && sub(out) != "" { // 빈 응답은 빈 목록이 아니다(ls와 같음)
		terms, termsOK = lsTerms(sub(out))
	}
	var conn []string
	connKnown := false
	if raw, rc := run("hc", "machine", "ls", "--json"); rc == 0 {
		conn, connKnown = jsonList(raw, "machines", "name", isConnected)
	}
	// 병합기 줄·착지 표식: 떠 있는 줄이 있는 프로젝트마다 ls-remote 한 번
	type rem struct {
		ok     bool
		queue  []string
		landed []int64
	}
	rems := map[string]*rem{}
	for _, r := range rows {
		p := r.F(qProj)
		if r.F(qState) != "up" || rems[p] != nil {
			continue
		}
		x := &rem{}
		rems[p] = x
		if repo := repoOf(p, LOCAL); repo != "" {
			if out, rc := run("git", "-C", repo, "ls-remote", "origin", "refs/merge-queue/*", "refs/hc-landed/*"); rc == 0 {
				x.ok = true
				x.queue, x.landed = stRemote(out)
			}
		}
	}
	now := time.Now()
	for _, r := range rows {
		name, m, addr := r.F(qName), r.F(qMach), r.F(qAddr)
		in := stIn{State: r.F(qState), Early: r.F(qEarly), Last: lsFirst(DATA+"/last.tsv", name)}
		if in.State == "wait" {
			in.Pin, _ = pinOf(name, r.F(qRID))
		}
		if in.State == "up" {
			in.Live = stLive(terms, termsOK, conn, connKnown, addr, name)
			in.Bg = hangBgWait(name, addr)
			if r.F(qAudit) == "auto" {
				in.Aud = anote(name)
			}
			x := rems[r.F(qProj)]
			if !x.ok {
				in.Pos = -1
			}
			for i, l := range x.queue {
				if l == name {
					in.Pos = i + 1
					break
				}
			}
			stage := stStage(in)
			if pr, _ := prOf(name); pr != "" { // PR 거울(pr.go) — PR이 없으면 줄이 전과 같다
				stage += " · " + pr
			}
			if h := prHumanNote(name); h != "" { // 사람 리뷰(prhuman.go) — 꺼져 있으면 줄이 전과 같다
				stage += " · " + h
			}
			fmt.Println(stLine(name, dash(m), stAge(r.F(qAdded), now), stage, stEstimate(in.Pos, x.landed)))
			continue
		}
		stage := stStage(in)
		if w := rejWhy(name, r.F(qRID)); in.State == "wait" && w != "" {
			stage = "띄우기 거절 — " + w
		}
		fmt.Println(stLine(name, dash(m), stAge(r.F(qAdded), now), stage, ""))
	}
	maPrint()
	statusTick()
	exit(0)
}

// statusTick: 표 끝에 바퀴 계측 한 줄(tick.go — 지휘가 바퀴가 느려진 것을 바로 본다). 기록이 없으면 찍지 않는다
// 그 앞에 기계마다 한 줄(fill-load-steer — 부하·자리·실행 중 조정·안 띄운 까닭, 바퀴 기록 load.tsv가 있을 때)
func statusTick() {
	for _, l := range machineLines(false) {
		fmt.Println("기계 " + l)
	}
	if s := tickStatus(); s != "" {
		fmt.Println(s)
	}
	msgs, _, _ := onlandSyntax() // 착지 뒤 명령 구문 오류(onland_syntax.go)
	for _, m := range msgs {
		fmt.Println(m)
	}
}
