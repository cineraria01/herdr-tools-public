package main

// PR 거울(2026-11-24 pr-surface, docs/hc-fill/pr.md — 사용자 「각 워커가 자기 할일이 끝나면 pr 을 열어서 올리고 … 지휘 컴퓨터가
// 충돌 머지 병합」): 대기열이 원본, PR은 거울(한 방향). 레인 완료 → PR 열기, 감사 판정 → 리뷰·댓글 + 커밋 상태, 병합기 사건(제출·착지·
// 돌려보냄·병합 쪽 풀이) → 댓글·상태, 대기열에서 뺌 → 닫기. PR에서 읽어 판단하는 것은 없다(앞 시도가 응답 없이 끝난 댓글의 숨은
// 표식 확인만). 거울은 최선 노력 — gh가 느리거나 실패해도 대기열·감사·병합·착지는 그대로, 실패는 다음 바퀴에 다시.
// 손잡이 pr(프로젝트 목록, 기본 「-」 = 끔 — 꺼져 있으면 gh·파일·출력 하나도 없음), pr_max(바퀴당 gh 호출), pr_fail, pr_timeout.
// PR 작성자 인증(2026-11-24 지휘 추가 지시): 설정 pr_token_cmd(토큰을 내놓는 명령)가 있으면 PR 열기·머리 가지 만들기만 그 토큰으로,
// 나머지(댓글·상태·리뷰)는 기본 gh 로그인. 그 토큰으로 연 PR은 작성자가 다르니 감사 판정이 진짜 리뷰(승인·변경 요청)가 된다.

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/ghpr"
	"herdr-tools/internal/toolcfg"
)

// prEnt: pr.tsv 한 줄 — 레인(병합기 사건의 레인 이름 = 머리 가지 pr/<레인>) 하나에 PR 하나
type prEnt struct {
	Lane, Proj, Repo, Num, URL, State, Opened, By, Head string // State open·merged·closed, By bot(토큰으로 엶)·self
}

func (e prEnt) line() string {
	return strings.Join([]string{e.Lane, e.Proj, e.Repo, e.Num, e.URL, e.State, e.Opened, e.By, e.Head}, "\t")
}

func (e prEnt) repo() ghpr.Repo {
	p := strings.Split(e.Repo, "/")
	if len(p) != 3 {
		return ghpr.Repo{}
	}
	return ghpr.Repo{Host: p[0], Owner: p[1], Name: p[2]}
}

func (e prEnt) num() int { n, _ := strconv.Atoi(e.Num); return n }

// prRun: 한 바퀴의 거울 상태
type prRun struct {
	ents  map[string]*prEnt
	order []string
	sent  map[string]string // 사건 열쇠 → try(부르고 응답 모름)·sent
	gh    *ghpr.Client
	bot   *ghpr.Client // PR 열기·머리 가지(토큰이 있으면 그 인증, 없으면 gh와 같음)
	mask  func(string) string
	ok    int // 이 바퀴 성공한 gh 일
	bad   int // 이 바퀴 실패한 gh 일
	stop  bool
	last  string // 마지막 실패 글
}

func prFile(n string) string { return DATA + "/" + n }

// prProjects: 손잡이·설정 pr의 프로젝트(바퀴마다 새로 읽어 바로 먹는다). 빈 목록 = 끔
func prProjects() []string {
	v := strings.ReplaceAll(conf("pr"), " ", "")
	if v == "" || v == "-" {
		return nil
	}
	return strings.Split(v, ",")
}

func prOn(p string) bool { return p != "" && has(prProjects(), p) }

func prLoad() (map[string]*prEnt, []string) {
	m, order := map[string]*prEnt{}, []string(nil)
	s, _ := readFile(prFile("pr.tsv"))
	for _, l := range lines(s) {
		f := strings.Split(l, "\t")
		if len(f) < 9 {
			continue
		}
		e := &prEnt{f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8]}
		if _, ok := m[e.Lane]; !ok {
			order = append(order, e.Lane)
		}
		m[e.Lane] = e
	}
	return m, order
}

func (r *prRun) save() {
	var b strings.Builder
	for _, n := range r.order {
		b.WriteString(r.ents[n].line() + "\n")
	}
	if err := writeAtomic(prFile("pr.tsv"), b.String()); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: PR 거울: %v\n", err)
	}
	b.Reset()
	// ponytail: 열쇠 기록은 지우지 않고 자란다(사건 하나에 한 줄) — 커지면 닫힌·Merged PR 레인의 열쇠를 거둔다
	for k, v := range r.sent {
		b.WriteString(k + "\t" + v + "\n")
	}
	if err := writeAtomic(prFile("pr-sent.tsv"), b.String()); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: PR 거울: %v\n", err)
	}
}

// prOf <레인>: status·state에 붙일 「PR #n」과 주소(다시 맞춤 레인은 원래 레인의 PR). 없으면 빈 값
func prOf(lane string) (string, string) {
	m, _ := prLoad()
	e := m[lane]
	if e == nil {
		e = m[relandSuf.ReplaceAllString(lane, "")]
	}
	if e == nil || e.Num == "" {
		return "", ""
	}
	return "PR #" + e.Num, e.URL
}

// prTick: 바퀴의 거울 단계(사건·감사·다시 맞춤 뒤). 꺼져 있으면 아무것도 하지 않는다
func prTick() {
	projs := prProjects()
	if len(projs) == 0 || dry || EVBAD {
		return
	}
	r := &prRun{sent: map[string]string{}}
	r.ents, r.order = prLoad()
	for _, row := range readRows(prFile("pr-sent.tsv")) {
		if len(row) >= 2 {
			r.sent[row[0]] = row[1]
		}
	}
	mk, err := toolcfg.Masker(toolcfg.Dir())
	if err != nil {
		prAlert("nomask", "PR 거울: 개인 값을 가리지 못해 PR에 쓰지 않는다 — "+err.Error())
		return
	}
	r.mask = mk
	r.gh = &ghpr.Client{Timeout: time.Duration(knobInt("pr_timeout")) * time.Second, Max: knobInt("pr_max")}
	if !r.gh.Available() { // gh가 없는 기계 — 레인 기계에 커밋을 올리기 전에 멈춘다(감사 낮음 2)
		prAlert("nogh", "PR 거울: gh가 없다 — PR에 쓰지 않는다")
		return
	}
	r.bot = r.gh.As(prToken())
	r.opens()
	r.verdicts()
	r.mergerEvents()
	r.closes()
	r.save()
	r.health()
}

// prToken: 설정 pr_token_cmd(sh -c)의 표준 출력 첫 줄. 없으면 빈 값(기본 로그인). 실패하면 빈 값 + 운영 사건 한 번 — 값은 어디에도 찍지 않는다
func prToken() string {
	c := conf("pr_token_cmd")
	if c == "" {
		return ""
	}
	out, rc := Cmd{Name: "sh", Args: []string{"-c", c}, Max: 20 * time.Second}.Run()
	tok, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	if rc != 0 || strings.TrimSpace(tok) == "" {
		prAlert("token", fmt.Sprintf("PR 거울: 설정 pr_token_cmd가 토큰을 내놓지 못했다(종료 %d) — 기본 gh 로그인으로 연다", rc))
		return ""
	}
	prOpsDrop("token") // 고쳐졌다 — 다시 실패하면 또 알린다(감사 낮음 4)
	return strings.TrimSpace(tok)
}

// prOpsDrop <까닭 종류>: 알린 종류 기록에서 그 종류를 지운다(없으면 아무것도 안 씀)
func prOpsDrop(kind string) {
	var keep []string
	hit := false
	for _, row := range readRows(prFile("pr-ops.tsv")) {
		if len(row) >= 1 && row[0] == kind {
			hit = true
			continue
		}
		keep = append(keep, strings.Join(row, "\t")+"\n")
	}
	if !hit {
		return
	}
	if err := writeAtomic(prFile("pr-ops.tsv"), strings.Join(keep, "")); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: PR 거울: %v\n", err)
	}
}

// prAlert <까닭 종류> <글>: 운영 사건 hc-fill-pr 한 번(같은 종류는 pr-ops.tsv에 적혀 있으면 다시 안 보냄 — 성공한 바퀴가 지운다)
func prAlert(kind, msg string) {
	say("%s", msg)
	for _, row := range readRows(prFile("pr-ops.tsv")) {
		if len(row) >= 1 && row[0] == kind {
			return
		}
	}
	key := "hf:pr:" + kind + ":" + strconv.FormatInt(time.Now().Unix(), 10)
	if hcLaneReport("막힘", msg, "hc-fill-pr", "", key) != 0 {
		return // 다음 바퀴에 다시
	}
	if err := appendLine(prFile("pr-ops.tsv"), kind+"\t"+events.UTC(time.Now())); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: PR 거울: %v\n", err)
	}
}

// health: 이 바퀴 결과로 연속 실패를 세고, pr_fail바퀴 이어지면 운영 사건 한 번. 성공한 바퀴는 세기와 알린 종류(token 빼고)를 지운다
func (r *prRun) health() {
	if r.ok > 0 {
		var keep []string
		for _, row := range readRows(prFile("pr-ops.tsv")) {
			if len(row) >= 1 && row[0] == "token" {
				keep = append(keep, strings.Join(row, "\t")+"\n")
			}
		}
		if err := writeAtomic(prFile("pr-ops.tsv"), strings.Join(keep, "")); err != nil {
			fmt.Fprintf(os.Stderr, "hc-fill: PR 거울: %v\n", err)
		}
		os.Remove(prFile("pr-fail"))
		return
	}
	if r.bad == 0 {
		return
	}
	s, _ := readFile(prFile("pr-fail"))
	n := atoi10(strings.TrimSpace(s)) + 1
	if err := writeAtomic(prFile("pr-fail"), strconv.Itoa(n)+"\n"); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: PR 거울: %v\n", err)
	}
	if n >= knobInt("pr_fail") {
		prAlert("gh", fmt.Sprintf("PR 거울: gh 실패가 %d바퀴 이어짐 — %s", n, r.last))
	}
}

// do: gh 일 하나의 결과를 센다. 참 = 성공. 상한이면 이 바퀴는 멈춘다(남은 것은 다음 바퀴에)
func (r *prRun) do(what string, err error) bool {
	switch {
	case err == nil:
		r.ok++
		return true
	case errors.Is(err, ghpr.ErrBudget):
		if !r.stop {
			say("PR 거울: 바퀴당 gh 호출 상한(pr_max %d) — 나머지는 다음 바퀴에", r.gh.Max)
		}
		r.stop = true
	case errors.Is(err, ghpr.ErrNoGh):
		r.stop = true
		prAlert("nogh", "PR 거울: gh가 없다 — PR에 쓰지 않는다")
	case errors.Is(err, ghpr.ErrTimeout): // 느린 gh가 바퀴를 묶지 않게 — 이 바퀴는 여기서 멈춘다(감사 중간 1)
		r.bad++
		r.stop = true
		r.last = what + ": " + err.Error()
		say("PR 거울 시간 넘김 %s — 이 바퀴는 멈추고 다음 바퀴에 다시", r.last)
	default:
		r.bad++
		r.last = what + ": " + err.Error()
		say("PR 거울 실패 %s — 다음 바퀴에 다시", r.last)
	}
	return false
}

// comment <레인 PR> <사건 열쇠> <글>: 숨은 표식을 단 댓글 한 번. 앞 시도가 응답 없이 끝났으면(try) 표식을 먼저 찾아 있으면 단 것으로
func (r *prRun) comment(e *prEnt, key, body string) bool {
	switch r.sent[key] {
	case "sent":
		return true
	case "try":
		has, err := r.gh.HasMarker(e.repo(), e.num(), prMark(key))
		if !r.do("댓글 확인 "+e.Lane, err) {
			return false
		}
		if has {
			r.sent[key] = "sent"
			return true
		}
	}
	if r.stop {
		return false
	}
	r.sent[key] = "try"
	r.save() // 부르기 전에 남긴다 — 도중에 죽어도 다음 바퀴가 표식부터 본다
	if !r.do("댓글 "+e.Lane, r.gh.Comment(e.repo(), e.num(), r.mask(body)+"\n\n"+prMark(key))) {
		return false
	}
	r.sent[key] = "sent"
	return true
}

func prMark(key string) string { return "<!-- hc-fill-pr " + key + " -->" }

// status: 커밋 상태(같은 열쇠는 한 번 — 다시 달아도 무해하지만 호출을 아낀다)
func (r *prRun) status(e *prEnt, key, sha, state, ctx, desc string) bool {
	if r.sent[key] == "sent" {
		return true
	}
	if r.stop {
		return false
	}
	if len(sha) != 40 { // 상태를 달 커밋을 모름 — 건너뜀
		return true
	}
	if !r.do("상태 "+e.Lane, r.gh.Status(e.repo(), sha, state, prCtx()+"/"+ctx, r.mask(desc))) {
		return false
	}
	r.sent[key] = "sent"
	return true
}

// prCtx: 커밋 상태 이름 머리(설정 pr_context, 기본 herdr)
func prCtx() string {
	if v := conf("pr_context"); v != "" {
		return v
	}
	return "herdr"
}

// evKey: 사건 줄의 열쇠(줄 그대로의 해시 앞 16자)
func evKey(l string) string { return sha256hex([]byte(l))[:16] }

// closes: hc-fill rm이 남긴 닫기 요청(pr-close.tsv)
func (r *prRun) closes() {
	s, _ := readFile(prFile("pr-close.tsv"))
	var keep []string
	for _, l := range lines(s) {
		f := strings.SplitN(l, "\t", 3)
		if len(f) < 3 {
			continue
		}
		e := r.ents[f[0]]
		if e == nil || e.State != "open" || r.close(e, "rm:"+f[1], "대기열에서 뺌 — 착지 없이 끝났다: "+f[2]) {
			continue
		}
		keep = append(keep, l+"\n")
	}
	if len(keep) == 0 {
		os.Remove(prFile("pr-close.tsv"))
	} else if err := writeAtomic(prFile("pr-close.tsv"), strings.Join(keep, "")); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: PR 거울: %v\n", err)
	}
}

// close: 까닭 댓글 뒤 닫는다
func (r *prRun) close(e *prEnt, key, why string) bool {
	if !r.comment(e, key, why) || r.stop {
		return false
	}
	if !r.do("닫기 "+e.Lane, r.gh.Close(e.repo(), e.num())) {
		return false
	}
	e.State = "closed"
	say("PR #%s 닫음 %s — %s", e.Num, e.Lane, r.mask(why))
	return true
}

// prCloseReq <레인> <까닭>: rm이 부른다 — 그 레인에 열린 PR이 있으면 다음 바퀴에 닫게 적는다(꺼져 있거나 PR이 없으면 아무것도 안 함)
func prCloseReq(lane, why string) {
	m, _ := prLoad()
	if e := m[lane]; e == nil || e.State != "open" {
		return
	}
	l := lane + "\t" + strconv.FormatInt(time.Now().UnixNano(), 36) + "\t" + strings.ReplaceAll(why, "\t", " ")
	if err := appendLine(prFile("pr-close.tsv"), l); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: PR 닫기 요청을 못 적었다: %v\n", err)
	}
}
