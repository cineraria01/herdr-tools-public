package main

// 충돌을 병합 쪽에서 푼다(merger-conflict-agent, 2026-11-24 사용자 「워커에서 Pr 을 하거나 하면 그걸 가지고 병합기 지휘에서 병합을 머지나,
// 리베이스 충돌 자체를 풀어서 머지 시키는거고」): rebase 충돌(미리 훑기·혼자 착지 종료 3)을 바로 반송하지 않고, 병합 쪽 작업 트리
// <클론>.merger-resolve/<큐 이름>에 rebase가 멈춘 상태를 남긴다. 손잡이 conflict(<일지 폴더>/conflict 또는 HC_LANE_MERGER_CONFLICT,
// 바퀴마다 읽음)로 길을 고른다:
//   - agent(기본): 푸는 에이전트 merge-<레인>을 띄운다(띄울 명령 = 손잡이 resolve_launch, 지시문 틀 = 설정 폴더 merger-resolve.md)
//   - event: 띄우지 않고 주인 지휘에 막힘 사건 「충돌 — 풀 수 있음: <작업 트리> · <이어 가는 명령>」
//   - reject: 지금처럼 바로 반송
// 푸는 동안 그 제출은 이 바퀴의 줄 목록에서 빼고(뒤 후보는 지금처럼 착지) 바퀴마다 결과(<상태 폴더>/result)를 본다.
// 「풀림」이면 풀이만 보는 감사(틀 merger-resolve-audit.md, 판정 줄은 hc-fill 감사와 같은 꼴 「감사 완료 <ID> 높음 n·중간 n·낮음 n」)를
// 띄우고, 높음·중간 0이면 그 풀이를 같은 큐 이름(원래 차례)에 판정 ID·patch-id를 실은 제출 기록과 함께 다시 넣는다 — 그 뒤는 보통
// 승인 착지 길(검증 맡김·HERDR_LAND_PATCHID 대조)이다. 감사 없이 풀이가 큐에 들어가는 길은 없다(다시 넣기는 판정을 읽은 뒤에만).
// 못 풂·감사 지적·시간 넘김(손잡이 resolve_max)·못 띄움이면 지금처럼 반송한다(까닭에 「병합 쪽 풀이 못 함: …」, 풀다 만 diff는 로그).

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/gitx"
)

const (
	jResolving = "푸는 중"  // 충돌 — 병합 쪽에서 푸는 중(일지)
	jResolved  = "풀이 넣음" // 풀이 감사 통과 — 같은 큐 자리에 다시 넣음(일지)
	launchMax  = 60 * time.Second
)

// conflictMode: 손잡이 conflict — agent(기본)·event·reject. 모르는 값은 경고와 함께 agent
func (m *M) conflictMode() string {
	v := m.knobStr("conflict", "HC_LANE_MERGER_CONFLICT")
	switch v {
	case "", "agent":
		return "agent"
	case "event", "reject":
		return v
	}
	fmt.Fprintf(os.Stderr, "hc-lane merger: 경고 — 손잡이 conflict 는 agent·event·reject: %q — agent 로\n", v)
	return "agent"
}

// knobStr: 글 손잡이 — 공통 손잡이 merger_<파일>(knobs.go)이 있으면 그 값, 다음은 <일지 폴더>/<파일>(옛 길, 바퀴마다 읽음), 없으면 환경 변수
func (m *M) knobStr(file, env string) string {
	v, _ := m.knobSrc(file, env)
	return v
}

// knobSrc: knobStr과 그 값이 온 곳 — 반송 글에 적어, 원격 손잡이만 보고 옛 파일을 놓치지 않게(2026-11-25 한 저장소의 PR 제출 반송)
func (m *M) knobSrc(file, env string) (string, string) {
	if v, ok := rk["merger_"+file]; ok {
		return v, "공통 손잡이·설정 파일 merger_" + file
	}
	if b, err := os.ReadFile(m.mj + "/" + file); err == nil {
		return strings.TrimSpace(string(b)), m.mj + "/" + file
	}
	return strings.TrimSpace(os.Getenv(env)), "환경 변수 " + env
}

// resolveMax: 한 단계(풀이·풀이 감사)의 상한, 손잡이 resolve_max(초, 기본 3600)
func (m *M) resolveMax() int64 {
	return int64(intSetting("resolve_max", m.knobStr("resolve_max", "HC_LANE_MERGER_RESOLVE_MAX"), 3600, 1))
}

func confDir() string {
	if d := os.Getenv("HERDR_TOOLS_CONFIG"); d != "" {
		return d
	}
	return os.Getenv("HOME") + "/.config/herdr-tools"
}

// rstate: 푸는 중인 제출 하나(<일지 폴더>/resolve/<큐 이름>/st — 「키\t값」 줄)
type rstate struct {
	dir                                   string
	q, s, ln, rec, B, wt, why, mode       string
	phase, aid, R, auto, pid, files, addr string
	t0                                    int64
}

var rkeys = []string{"q", "s", "ln", "rec", "B", "wt", "why", "mode", "phase", "aid", "R", "auto", "pid", "files", "addr"}

func (r *rstate) fields() map[string]*string {
	return map[string]*string{"q": &r.q, "s": &r.s, "ln": &r.ln, "rec": &r.rec, "B": &r.B, "wt": &r.wt, "why": &r.why, "mode": &r.mode,
		"phase": &r.phase, "aid": &r.aid, "R": &r.R, "auto": &r.auto, "pid": &r.pid, "files": &r.files, "addr": &r.addr}
}

func (r *rstate) save() error {
	var sb strings.Builder
	f := r.fields()
	for _, k := range rkeys {
		fmt.Fprintf(&sb, "%s\t%s\n", k, flat(*f[k]))
	}
	fmt.Fprintf(&sb, "t0\t%d\n", r.t0)
	return writeVia(r.dir+"/st", sb.String())
}

func loadR(dir string) (*rstate, bool) {
	b, err := os.ReadFile(dir + "/st")
	if err != nil {
		return nil, false
	}
	r := &rstate{dir: dir}
	f := r.fields()
	for _, l := range strings.Split(string(b), "\n") {
		k, v, _ := strings.Cut(l, "\t")
		if k == "t0" {
			r.t0, _ = strconv.ParseInt(v, 10, 64)
		} else if p := f[k]; p != nil {
			*p = v
		}
	}
	return r, strings.HasPrefix(r.q, "refs/merge-queue/") && sha40.MatchString(r.s) && r.ln != "" && r.wt != "" && r.phase != "" && r.t0 > 0
}

func (r *rstate) name() string { return strings.TrimPrefix(r.q, "refs/merge-queue/") }

// conflictTake: 충돌한 제출을 병합 쪽 풀이로 넘긴다. skip != "" = 지금처럼 반송할 것, 그 까닭(손잡이 reject·제출 기록 없음·지금 다시
// 해 보니 안 부딪힘 — 부른 쪽이 반송 글 끝에 「 — 병합 쪽 풀이 안 함: <skip>」으로 붙인다). 빈 값 = 넘겼거나(푸는 중) 이미 푸는
// 중이거나 넘기다 못해 반송까지 했다 — 부른 쪽은 반송하지 않는다
func (m *M) conflictTake(q, s, ln, rec, B, why string) (skip string) {
	mode := m.conflictMode()
	if mode == "reject" {
		_, src := m.knobSrc("conflict", "HC_LANE_MERGER_CONFLICT")
		return "손잡이 conflict=reject(" + src + ")"
	}
	mt, ok := readMeta(strings.TrimPrefix(rec, "-"), s)
	if !ok {
		return "제출 기록을 못 읽음"
	}
	if mt.id == "" { // 풀이의 감사 판정을 묶을 제출 기록이 없다(손 제출)
		return "제출 기록 없음(손 제출 — 풀이 판정을 묶을 곳이 없다)"
	}
	m.B = B // 충돌 파일(conflicts)·반송 글의 기준
	n := strings.TrimPrefix(q, "refs/merge-queue/")
	dir := m.mj + "/resolve/" + n
	if fileExists(dir + "/st") {
		return "" // 이미 푸는 중(한 제출을 두 곳에서 풀지 않는다)
	}
	wt := m.top + ".merger-resolve/" + n
	if err := os.MkdirAll(dir, 0o777); err != nil || os.MkdirAll(filepath.Dir(wt), 0o777) != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 풀이 폴더를 만들지 못했다 — 지금처럼 반송\n")
		return "풀이 폴더를 만들지 못함"
	}
	r := &rstate{dir: dir, q: q, s: s, ln: ln, rec: rec, B: B, wt: wt, why: why, mode: mode, phase: "resolve", t0: time.Now().Unix()}
	lg := fmt.Sprintf("%s/%s-resolve-%s-%d.log", m.ml, ln, time.Now().Format("20060102-150405"), os.Getpid())
	if fileExists(wt) { // 앞 실행이 상태를 적기 전에 끊긴 작업 트리
		m.rdrop(&rstate{dir: dir + "/x", wt: wt})
	}
	if inLog("", lg, "-c", "core.hooksPath=/dev/null", "worktree", "add", "-q", "-f", "--detach", wt, s) != 0 {
		m.rdrop(r)
		return "풀이 작업 트리를 만들지 못함(로그 " + lg + ")"
	}
	rc := inLog(wt, lg, "-c", "core.hooksPath=/dev/null", "rebase", "-q", "--reapply-cherry-picks", B)
	if rc == 0 || !m.rebasing(wt) {
		fmt.Fprintf(os.Stderr, "hc-lane merger: %s 를 병합 쪽에서 다시 얹어 보니 충돌로 멈추지 않았다(종료 %d) — 지금처럼 반송\n", ln, rc)
		m.rdrop(r)
		return fmt.Sprintf("병합 쪽에서 다시 얹어 보니 충돌로 멈추지 않음(종료 %d, 로그 %s)", rc, lg)
	}
	r.files = m.conflicts(s)
	if err := r.save(); err != nil {
		m.rdrop(r)
		return "풀이 상태를 쓰지 못함"
	}
	if err := writeVia(dir+"/context.md", m.rcontext(r, mt)); err != nil {
		m.giveUp(r, "풀이 준비 글을 쓰지 못했다")
		return ""
	}
	jw := fmt.Sprintf("충돌 — 병합 쪽에서 푸는 중(%s, 충돌 %s, 작업 트리 %s)", mode, r.files, wt)
	if mode == "event" {
		m.jline(mt, n, ln, jResolving, "-", jw, r.t0, r.t0, "1")
		msg := fmt.Sprintf("충돌 — 풀 수 있음: %s · 양쪽 뜻을 살려 풀고 rebase --continue까지 마친 뒤 이어 가기: printf '풀림\\n' >%s/result (못 풀면 printf '못 풂 — <까닭>\\n' >%s/result). 레인 %s, 충돌 %s, 준비 글 %s/context.md, 상한 %d초 — 그 뒤 풀이 감사를 거쳐 착지",
			wt, dir, dir, ln, r.files, dir, m.resolveMax())
		m.ownerEvent(mt, "막힘", "hc-merger-resolve:"+s, msg)
		return ""
	}
	addr, err := m.rlaunch(r, "merge-"+ln, "merger-resolve.md", map[string]string{"{result}": dir + "/result"})
	if err != "" {
		m.giveUp(r, "푸는 에이전트를 못 띄움 — "+err)
		return ""
	}
	r.addr = addr
	if err := r.save(); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 풀이 상태에 터미널 주소를 적지 못했다: %v\n", err)
	}
	m.jline(mt, n, ln, jResolving, "-", jw, r.t0, r.t0, "1")
	m.ownerEvent(mt, "중간", "hc-merger-resolve:"+s, fmt.Sprintf("충돌 — 병합 쪽에서 푸는 중: 레인 %s, 충돌 %s, 에이전트 merge-%s, 작업 트리 %s", ln, r.files, ln, wt))
	return ""
}

// rcontext: 풀이 준비 글 — 기준·제출, 충돌 파일, 양쪽에서 그 파일을 고친 커밋 제목, 레인 지시문(이 기계에 있으면)
func (m *M) rcontext(r *rstate, mt meta) string {
	var sb strings.Builder
	fork, _ := out("merge-base", r.s, r.B)
	fmt.Fprintf(&sb, "# 충돌 준비 — %s\n\n- 큐 %s · 제출 ID %s\n- 기준 %s · 제출 %s · 갈래점 %s\n- 충돌 파일: %s\n- 못 풀면 반송될 까닭: %s\n",
		r.ln, r.name(), mt.id, r.B, r.s, fork, r.files, r.why)
	for _, side := range []struct{ t, tip string }{{"기준 쪽", r.B}, {"제출 쪽", r.s}} {
		args := []string{"log", "--format=%h %s", fork + ".." + side.tip}
		if r.files != "파일 모름(로그)" {
			args = append(append(args, "--"), strings.Fields(strings.TrimSuffix(r.files, " …"))...)
		}
		l, _ := out(args...)
		fmt.Fprintf(&sb, "\n## %s에서 그 파일을 고친 커밋\n%s\n", side.t, l)
	}
	rb := os.Getenv("HC_FILL_RUNS")
	if rb == "" {
		rb = os.Getenv("HOME") + "/.local/share/hc-fill/runs"
	}
	if p := rb + "/" + r.ln + "-run.md"; fileExists(p) {
		fmt.Fprintf(&sb, "\n## 레인 지시문\n%s\n", p)
	}
	return sb.String()
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// rlaunch: 틀을 채워 지시문을 쓰고 띄울 명령(손잡이 resolve_launch)을 sh -c 로 부른다 — 환경 HC_MERGE_NAME·HC_MERGE_DIR(작업 트리)·
// HC_MERGE_BRIEF(지시문 경로). 표준 출력 첫 줄은 띄운 터미널 주소로 적어 둔다. 못 띄우면 까닭
func (m *M) rlaunch(r *rstate, name, tmpl string, extra map[string]string) (addr, why string) {
	cmd := m.knobStr("resolve_launch", "HC_LANE_MERGER_RESOLVE_LAUNCH")
	if cmd == "" {
		return "", "띄울 명령이 없다(손잡이 resolve_launch — " + m.mj + "/resolve_launch 또는 HC_LANE_MERGER_RESOLVE_LAUNCH)"
	}
	tp := confDir() + "/" + tmpl
	t, err := os.ReadFile(tp)
	if err != nil {
		return "", "지시문 틀이 없다(" + tp + ")"
	}
	kv := []string{"{lane}", r.ln, "{worktree}", r.wt, "{base}", m.base, "{base_sha}", r.B, "{sub_sha}", r.s, "{conflicts}", r.files,
		"{context}", r.dir + "/context.md", "{queue}", r.name(), "{name}", name}
	for k, v := range extra {
		kv = append(kv, k, v)
	}
	bp := r.dir + "/" + strings.TrimSuffix(tmpl, ".md") + "-brief.md"
	if err := writeVia(bp, strings.NewReplacer(kv...).Replace(string(t))); err != nil {
		return "", "지시문을 쓰지 못했다"
	}
	o, rc, e := boundedRun(launchMax, "sh", []string{"HC_MERGE_NAME=" + name, "HC_MERGE_DIR=" + r.wt, "HC_MERGE_BRIEF=" + bp}, "-c", cmd)
	if rc != 0 {
		return "", fmt.Sprintf("띄울 명령 종료 %d: %s", rc, bcut(200, flat(strings.TrimSpace(e))))
	}
	addr, _, _ = strings.Cut(strings.TrimSpace(o), "\n")
	return addr, ""
}

// rdrop: 풀이 작업 트리·상태를 치운다(반송·다시 넣기 뒤, 또는 풀이를 시작하지 못함)
func (m *M) rdrop(r *rstate) {
	if fileExists(r.wt) && runQ("worktree", "remove", "-f", "-f", r.wt) != 0 {
		if err := os.RemoveAll(r.wt); err != nil {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 풀이 작업 트리를 지우지 못했다: %v\n", err)
		}
		runQ("worktree", "prune")
	}
	if err := os.RemoveAll(r.dir); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 풀이 상태를 지우지 못했다: %v\n", err)
	}
}

// giveUp: 못 풀었다 — 풀다 만 상태(diff·상태)를 로그에 남기고 지금처럼 반송(까닭에 원래 충돌 까닭 + 「병합 쪽 풀이 못 함: …」)
func (m *M) giveUp(r *rstate, why string) {
	lg := fmt.Sprintf("%s/%s-resolve-%s-%d.log", m.ml, r.ln, time.Now().Format("20060102-150405"), os.Getpid())
	if f, err := os.OpenFile(lg, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666); err == nil {
		fmt.Fprintf(f, "hc-lane merger: 병합 쪽 풀이 못 함 — %s (작업 트리 %s)\n", why, r.wt)
		if fileExists(r.wt) {
			gitx.Run(r.wt, f, f, "status", "--short")
			gitx.Run(r.wt, f, f, "diff", "HEAD")
		}
		f.Close()
	}
	fmt.Fprintf(os.Stderr, "hc-lane merger: %s 병합 쪽 풀이 못 함 — %s, 반송\n", r.ln, why)
	m.B = r.B
	if m.fail(r.q, r.s, r.ln, r.rec, lg, r.why+" — 병합 쪽 풀이 못 함: "+why) {
		m.rdrop(r)
		return
	}
	m.mer = true // 반송은 fail/ 기록으로 다음 바퀴가 다시 한다 — 그동안 풀이 상태는 둔다(큐가 바뀌면 resolveTick이 치운다)
}

var verdictLine = regexp.MustCompile(`감사 완료 (\S+) 높음 ([0-9]+)·중간 ([0-9]+)·낮음 ([0-9]+)`)

// resolveTick: 바퀴마다(큐를 읽은 뒤) 푸는 중인 제출을 잇고, 아직 푸는 것은 이 바퀴 줄 목록(m.qs)에서 뺀다. 큐를 바꿨으면 true
// (부른 쪽은 이 바퀴를 끝내고 바로 다음 바퀴 — 새 목록으로)
func (m *M) resolveTick() bool {
	des, err := os.ReadDir(m.mj + "/resolve")
	if err != nil {
		return false
	}
	changed := false
	sort.Slice(des, func(i, j int) bool { return des[i].Name() < des[j].Name() })
	for _, d := range des {
		dir := m.mj + "/resolve/" + d.Name()
		r, ok := loadR(dir)
		if !ok {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 풀이 상태 %s 가 깨졌다 — 치우고 큐에서 다시 판정\n", d.Name())
			m.rdrop(&rstate{dir: dir, wt: m.top + ".merger-resolve/" + d.Name()})
			continue
		}
		if _, err := os.Stat(m.mj + "/fail/" + r.name()); err == nil {
			continue // 반송을 다시 하는 중(fretry) — 반송이 끝나면 아래 큐 대조가 치운다
		}
		if tip := refsha(m.qs, r.q); tip != r.s {
			// 철회·레인이 다시 냄·반송됨·이미 다시 넣음(응답만 끊김) — 이 풀이는 끝
			fmt.Printf("hc-lane merger: %s 풀이를 치운다 — 큐가 그 제출이 아니다(%s)\n", r.ln, first8(dashIf(tip)))
			m.rdrop(r)
			continue
		}
		m.rwatch(r) // 푸는 터미널의 신뢰 질문·멈춤(resolvewatch.go)
		if m.rstep(r) {
			changed = true
			continue
		}
		m.qs = dropRefs(m.qs, r.q, "refs/merge-queue-meta/"+r.name()) // 푸는 동안 이 제출은 빼고 진행
	}
	return changed
}

// rstep: 한 풀이의 다음 걸음. 큐를 바꿨으면(반송·다시 넣음) true
func (m *M) rstep(r *rstate) bool {
	over := time.Now().Unix()-r.t0 > m.resolveMax()
	switch r.phase {
	case "resolve":
		res, _ := os.ReadFile(r.dir + "/result")
		l, _, _ := strings.Cut(strings.TrimSpace(string(res)), "\n")
		switch {
		case l == "풀림":
			return m.rcheck(r)
		case strings.HasPrefix(l, "못 풂"):
			m.giveUp(r, "에이전트 「"+bcut(200, l)+"」")
			return true
		case over:
			m.giveUp(r, fmt.Sprintf("풀이 시간 넘김(%d초)", m.resolveMax()))
			return true
		}
	case "audit":
		b, _ := os.ReadFile(r.dir + "/audit")
		for _, mm := range verdictLine.FindAllStringSubmatch(string(b), -1) {
			if mm[1] != r.aid {
				continue
			}
			if mm[2] != "0" || mm[3] != "0" {
				m.giveUp(r, fmt.Sprintf("풀이 감사 지적 높음 %s·중간 %s (판정 %s, 보고서 %s/audit.md)", mm[2], mm[3], r.aid, r.dir))
				return true
			}
			return m.resubmit(r)
		}
		if over {
			m.giveUp(r, fmt.Sprintf("풀이 감사 시간 넘김(%d초)", m.resolveMax()))
			return true
		}
	default:
		m.giveUp(r, "모르는 풀이 단계 "+r.phase)
		return true
	}
	return false
}

// rcheck: 「풀림」을 받았다 — 작업 트리가 rebase를 마쳤고 깨끗하며 기준 위에 병합 커밋 없이 얹혔는지 보고, 풀이만 보는 감사를 띄운다
func (m *M) rcheck(r *rstate) bool {
	bad := ""
	R, rc := gitx.Out(r.wt, nil, "rev-parse", "--verify", "HEAD")
	st, rc2 := gitx.Out(r.wt, nil, "status", "--porcelain", "--untracked-files=no")
	switch {
	case rc != 0 || rc2 != 0:
		bad = "작업 트리를 읽지 못했다"
	case m.rebasing(r.wt): // REBASE_HEAD는 rebase가 끝나도 남는다 — 상태 폴더로 본다
		bad = "rebase가 끝나지 않았다"
	case st != "":
		bad = "커밋 안 된 변경이 남았다"
	case R == r.B || gitx.Run(r.wt, nil, nil, "merge-base", "--is-ancestor", r.B, R) != 0:
		bad = "풀이가 기준 위에 얹히지 않았다"
	}
	if bad == "" {
		if mg, rc := gitx.Out(r.wt, nil, "rev-list", "-1", "--merges", r.B+".."+R); rc != 0 || mg != "" {
			bad = "풀이에 병합 커밋이 있다(또는 못 봄)"
		}
	}
	if bad != "" {
		m.giveUp(r, "「풀림」인데 "+bad)
		return true
	}
	var ab strings.Builder // 충돌 표시가 든 자동 합침 트리(merge-tree 종료 1 — 첫 줄이 트리)
	arc := gitx.Run("", &ab, nil, "merge-tree", "--write-tree", "--no-messages", r.B, r.s)
	auto, _, _ := strings.Cut(ab.String(), "\n")
	pid, rc := gitx.PatchID(os.Stderr, r.B, R)
	if rc != 0 || arc > 1 || !gitx.ObjID(auto) {
		return false // 모름 — 다음 바퀴에 다시
	}
	var id [4]byte
	if _, err := rand.Read(id[:]); err != nil {
		return false
	}
	r.R, r.pid, r.auto, r.aid, r.phase, r.t0 = R, pid, auto, "mr-"+hex.EncodeToString(id[:]), "audit", time.Now().Unix()
	removeQuiet(r.dir + "/audit")
	addr, err := m.rlaunch(r, "audit-merge-"+r.ln, "merger-resolve-audit.md", map[string]string{"{resolved}": R, "{auto_tree}": auto,
		"{audit_id}": r.aid, "{verdict}": r.dir + "/audit", "{report}": r.dir + "/audit.md"})
	if err != "" {
		m.giveUp(r, "풀이 감사를 못 띄움 — "+err)
		return true
	}
	r.addr = addr
	if r.save() != nil {
		m.giveUp(r, "풀이 상태를 쓰지 못했다")
		return true
	}
	fmt.Printf("hc-lane merger: %s 풀림 %s — 풀이 감사 %s\n", r.ln, first8(R), r.aid)
	return false
}

// rebasing: 그 작업 트리에 멈춘 rebase가 있나(못 보면 있음으로)
func (m *M) rebasing(wt string) bool {
	for _, s := range []string{"rebase-merge", "rebase-apply"} {
		p, rc := gitx.Out(wt, nil, "rev-parse", "--git-path", s)
		if rc != 0 {
			return true
		}
		if !filepath.IsAbs(p) {
			p = wt + "/" + p
		}
		if fileExists(p) {
			return true
		}
	}
	return false
}

// resubmit: 풀이 감사 통과 — 풀이 R을 같은 큐 이름(원래 차례)에, 제출 기록은 부모 R·판정 ID·patch-id로 바꿔 한 번의 원자 push(lease)
func (m *M) resubmit(r *rstate) bool {
	rec := strings.TrimPrefix(r.rec, "-")
	body, rc := out("log", "-1", "--format=%B", rec)
	tree, rc2 := out("rev-parse", "--verify", rec+"^{tree}")
	if rc != 0 || rc2 != 0 {
		return false // 모름 — 다음 바퀴에
	}
	line, _, _ := strings.Cut(body, "\n")
	var kv []string
	for _, f := range strings.Fields(line) {
		switch k, _, _ := strings.Cut(f, "="); k {
		case "orig", "verdict", "patch", "resolved_from", "stack": // 풀이는 기준 위라 쌓임이 아니다(P가 조상이 아닐 수 있다)
			continue
		}
		kv = append(kv, f)
	}
	kv = append(kv, "orig="+r.R, "resolved_from="+r.s, "verdict="+r.aid, "patch="+r.pid)
	nm, rc := out("commit-tree", tree, "-p", r.R, "-m", strings.Join(kv, " "))
	if rc != 0 {
		return false
	}
	qm := "refs/merge-queue-meta/" + r.name()
	if runQ("push", "-q", "--atomic", "--porcelain", "origin", r.R+":"+r.q, "--force-with-lease="+r.q+":"+r.s,
		nm+":"+qm, "--force-with-lease="+qm+":"+rec) != 0 {
		rb, rc := lsRemote("origin", r.q, qm)
		switch {
		case rc != 0 || refsha(rb, r.q) == r.s:
			fmt.Fprintf(os.Stderr, "hc-lane merger: %s 풀이를 큐에 다시 넣지 못했다 — 다음 바퀴에 다시\n", r.ln)
			m.mer = true
			return false
		case refsha(rb, r.q) != r.R || refsha(rb, qm) != nm: // 철회·다시 제출
			m.rdrop(r)
			return true
		}
	}
	mt, _ := readMeta(nm, r.R)
	m.jline(mt, r.name(), r.ln, jResolved, r.R, fmt.Sprintf("풀이 감사 통과(판정 %s, patch-id %s) — 원래 차례에 다시 넣음, 원본 %s 충돌 %s", r.aid, first12(r.pid), first8(r.s), r.files),
		r.t0, time.Now().Unix(), "1")
	m.ownerEvent(mt, "중간", "hc-merger-resolved:"+r.R, fmt.Sprintf("충돌 — 병합 쪽 풀이 감사 통과(판정 %s): 레인 %s 풀이 %s 를 원래 차례에 다시 넣음(원본 %s)", r.aid, r.ln, first8(r.R), first8(r.s)))
	fmt.Printf("hc-lane merger: %s 풀이 %s 를 큐에 다시 넣음(판정 %s)\n", r.ln, first8(r.R), r.aid)
	m.rdrop(r)
	return true
}

// ownerEvent: 그 제출 주인 지휘에 사건(--name merger, 결과 열쇠) — 못 적어도 경고만
func (m *M) ownerEvent(mt meta, st, key, msg string) {
	owner := mt.owner
	if owner == "" {
		owner = "*"
	}
	args := []string{"report", st, "-", msg, "--name", "merger", "--key", key}
	if mt.owner != "" {
		args = append(args, "--to", mt.owner)
	}
	if _, rc, e := boundedRun(reportMax, gSelf, []string{"HC_LANE_OWNER=" + owner, "HC_LANE_OWNER_RUN=" + mt.ownerRun}, args...); rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 풀이 사건을 적지 못했다(종료 %d): %s\n", rc, strings.TrimSpace(e))
	}
}
