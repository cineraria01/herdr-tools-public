package main

// 맨 앞 추측 나눠 돌리기(ht-merger-shards, 2026-11-26 사용자 「그럼 위 방법에서 추천하는 방식으로 처리해」 — 업계의 test sharding across
// runners. 큰 저장소의 Go를 고친 착지는 go 시험이 벽을 정했고(중앙 1065초), 검증 스크립트는 한 기계 안에서만 묶음으로 나눠 돌았다).
// 「통과 조각 이어 쓰기(specparts.go)를 처음부터 여럿이 함께」로 짓는다 — 새 통과 기록 꼴·새 관문은 없다:
//
//   - 손잡이 merger_shards(프로젝트별 merger_shards@<프로젝트>, 1~8, 기본 1 = 지금 그대로). 맨 앞 후보만 나눈다 — K = min(손잡이, 지금
//     맡길 수 있는 서로 다른 기계 수), K가 1이면 지금 길.
//   - 나눔 일꾼 K개: 같은 추측 아이디, hc-lane merger --spec-verify <아이디> <기본> <i>/<K>(터미널 spec-<아이디 8>-<i>). 일꾼은 검증
//     스크립트에 HERDR_VERIFY_SHARD=<i>/<K>를 주고, 통과 조각을 몫마다 따로(refs/hc-merger/spec-part/<아이디>-s<i>) 올리며 통과 기록은
//     쓰지 않는다. 끝(통과·환경 실패·모름·바쁨)은 refs/hc-merger/spec-shard/<아이디>-s<i> 한 커밋 — 「그 몫은 끝남」일 뿐이다.
//     몫의 **시험 실패**(실패한 시험 줄이 있음)만 지금 추측 실패(spec-fail/<아이디>)로 — 그 뒤는 지금의 한 번 더 확인·반송·묶음 실패 길.
//   - 마무리: 모든 몫이 끝났거나 맨 앞 기다림 상한이면 한 기계에 지금 추측 검증을 그대로 다시 띄운다(specRedo — spec 참조를 그 기계로
//     고쳐 씀, 몫 없음). 일꾼은 이 아이디의 모든 조각 참조를 이어 붙여 깔고(specPartsGet) 빈틈만 돈 뒤 지금과 같은 통과 기록을 쓴다 —
//     착지 관문(통과 기록의 트리 = 넣는 트리)은 그대로. 다시 맡긴 실행(한 번 더 확인)도 같은 조각을 이어 쓴다.
//   - 병합기 쪽 나눔 상태는 <일지 폴더>/specshard 한 줄 「<아이디> <맨 앞 SHA> <시작 초> k=<K> <i>:<기계>… [fin=<기계>@<초>]」 —
//     맨 앞이 그 아이디이고 fin이 없을 때만 나눔 중이다(그 아이디를 다시 띄우면 — 마무리·한 번 더 확인·바쁨·상한 — specRedo가 fin을
//     적어 끝내고, 기준이 바뀌면 아이디가 달라 저절로 끝). 진행 판정(specFresh)도 나눔 중이면 몫 터미널 spec-<아이디 8>-<i>와 작업 트리를
//     몫마다 본다 — 상한을 넘겨도 진행 중인 몫이 있으면 더 기다린다. 일지 「추측 기다림」 줄에 「나눔 K(<기계>,…)」·
//     「마무리 <기계> n초」.
//   - 조각·끝 참조는 그 추측이 길에서 벗어날 때 specClean이 함께 치운다(<아이디>-s<i>의 아이디로 본다).

import (
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"herdr-tools/internal/verified"
)

const specShards = "refs/hc-merger/spec-shard/" // 나눔 몫의 끝(+ <아이디>-s<i>)

var shardArgRe = regexp.MustCompile(`^([1-8])/([1-8])$`)

// shardArg: 일꾼의 셋째 인자 「i/K」 — 맞으면 몫 번호
func shardArg(s string) (int, bool) {
	m := shardArgRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	i, _ := strconv.Atoi(m[1])
	k, _ := strconv.Atoi(m[2])
	return i, k > 1 && i <= k
}

// shardOwn: 몫의 참조 이름 끝 — 몫이 없으면 아이디 그대로
func shardOwn(id, shard string) string {
	if shard == "" {
		return id
	}
	i, _ := shardArg(shard)
	return fmt.Sprintf("%s-s%d", id, i)
}

// shardBase: 조각·끝 참조 이름 끝(<아이디>[-s<i>[of<K>]])의 아이디
func shardBase(name string) string {
	if len(name) > 42 && name[40:42] == "-s" {
		return name[:40]
	}
	return name
}

// shardEnded: 추측 id의 몫 i가 끝났나(끝 참조 <아이디>-s<i>of<K>)
func shardEnded(refs map[string]string, id, i string) bool { return shardEndRef(refs, id, i) != "" }

// shardEndRef: 추측 id의 몫 i의 끝 참조 이름(없으면 빈 값)
func shardEndRef(refs map[string]string, id, i string) string {
	p := specShards + id + "-s" + i + "of"
	for ref := range refs {
		if strings.HasPrefix(ref, p) {
			return ref
		}
	}
	return ""
}

// shardEndBusy: 몫 끝 참조 ref가 일 없이 끝난 「바쁨」(제목 state=busy)인가 — 원격에서 받아 읽는다(로컬 참조는 남기지 않는다). 못 읽으면 false
func shardEndBusy(ref string) bool {
	if fetchr(true, "-q", "origin", "+"+ref+":"+ref) != 0 {
		return false
	}
	defer runQ("update-ref", "-d", ref)
	s, rc := outQ("log", "-1", "--format=%s", ref)
	return rc == 0 && strings.HasSuffix(s, " state=busy")
}

// shardSettle: 맨 앞이 아닌 도는 추측 가운데 몫이 다 끝난 나눔 추측(기준이 바뀌어 길에서 벗어남 등)은 모름 — 몫만으로는 통과가 아니고,
// 「도는 중」으로 두면 그 기계를 붙잡고 조각·끝 참조도 안 치워진다(specClean이 다음에 거둔다)
func shardSettle(c *specCtx, front *specRow) {
	for i := range c.rows {
		r := &c.rows[i]
		if r == front || r.state != specRun {
			continue
		}
		k := 0
		for ref := range c.refs {
			if rest, ok := strings.CutPrefix(ref, specShards+r.id+"-s"); ok {
				_, kk, _ := strings.Cut(rest, "of")
				k, _ = strconv.Atoi(kk)
			}
		}
		done := 0
		for j := 1; j <= k; j++ {
			if shardEnded(c.refs, r.id, strconv.Itoa(j)) {
				done++
			}
		}
		if k > 1 && done == k {
			r.state, r.lost = specLost, "나눔 끝"
		}
	}
}

// shardsK: 손잡이 merger_shards(HC_LANE_MERGER_SHARDS, 1~8, 기본 1)
func (m *M) shardsK() int { return min(m.knobInt("shards", "HC_LANE_MERGER_SHARDS", 1, 1), 8) }

// specDispatch: 검증 기계에 일꾼을 띄운다(hc run --on, 상한 60초). 일꾼 쪽 손잡이(busy_wait·idle_load)는 env로 넘긴다.
// 병합 검증 자리를 받은 기계(merger_gate@<기계> 1)면 HERDR_VERIFY_GATE=1(검증 스크립트가 레인 검증과 다른 잠금을 쥔다 — verifierpick.go).
// shard(「i/K」)면 나눔 몫 일꾼(터미널 spec-<아이디 8>-<i>, 셋째 인자. 옛 판 일꾼은 인자 오류로 안 돌아 그 몫은 마무리가)
func (m *M) specDispatch(v verified.Verifier, id, shard string) bool {
	name, tail := "spec-"+id[:8], []string{"hc-lane", "merger", "--spec-verify", id, m.base}
	if i, ok := shardArg(shard); ok {
		name, tail = fmt.Sprintf("%s-%d", name, i), append(tail, shard)
	}
	args := []string{"run", "--bg", "--json", "--no-parent", "--rm", "--on", v.Machine,
		"--name", name, "--label", "hc-merger-spec=1", "--cwd", v.Path,
		"env", fmt.Sprintf("HC_LANE_SPEC_BUSY_WAIT=%d", m.busyWait()), fmt.Sprintf("HC_LANE_SPEC_IDLE_LOAD=%d", m.idleLoad())}
	args = append(append(args, gateEnv(v.Machine)...), tail...)
	_, rc, e := boundedRun(60*time.Second, "hc", nil, args...)
	if rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 추측 검증을 %s 에 띄우지 못했다(종료 %d): %s — 그 무리는 착지 때 직접 검증\n", v.Machine, rc, strings.TrimSpace(e))
		return false
	}
	return true
}

// shardMore: 맨 앞을 v에 맡기기로 했을 때 나눔에 더 쓸 기계(K−1대까지, 맡길 수 있는 것만 — 고른 기계는 busy에 적는다). K가 1이면 없음.
// 나누게 되면 띄우기 직전에 한 번 더 잰 기계로(shardRecheck — v가 빠지면 남은 첫 기계가 v)
func (m *M) shardMore(c *specCtx, busy map[string]bool, v verified.Verifier) (verified.Verifier, []verified.Verifier) {
	more := m.shardPick(c, busy, v)
	if len(more) == 0 {
		return v, nil
	}
	vs := m.shardRecheck(append([]verified.Verifier{v}, more...))
	return vs[0], vs[1:]
}

// shardRecheck: 나눌 기계들을 띄우기 직전에 한 번 더 잰다(probeRemote — 같은 판독의 verifies·gates). 이 바퀴 처음 잰 뒤 다른 병합기가
// 잡은 기계는 빼고(2026-11-26 22:49 몫 1이 「verifies=1 gates=1 — 바쁨」으로 일 없이 끝남), 하나도 안 남으면 그대로(일꾼이 바쁨을 남기면
// shardBusyRedo가 다시 맡긴다)
func (m *M) shardRecheck(vs []verified.Verifier) []verified.Verifier {
	ok := make([]bool, len(vs))
	pct := m.idleLoad()
	var wg sync.WaitGroup
	for j, v := range vs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			i, got, _ := probeRemote(v.Machine)
			ok[j] = got && (i.idle(pct) || gateOn(v.Machine) && i.gateFree())
		}()
	}
	wg.Wait()
	var keep []verified.Verifier
	for j, v := range vs {
		if ok[j] {
			keep = append(keep, v)
		}
	}
	if len(keep) < len(vs) {
		fmt.Printf("hc-lane merger: 나눌 기계를 띄우기 직전에 다시 쟀다 — %d대 가운데 %d대 한가함\n", len(vs), len(keep))
	}
	if len(keep) == 0 {
		return vs
	}
	return keep
}

// shardPick: shardMore의 고르기 — v 말고 더 쓸 맡길 수 있는 기계(K−1대까지)
func (m *M) shardPick(c *specCtx, busy map[string]bool, v verified.Verifier) []verified.Verifier {
	k := m.shardsK()
	if k <= 1 || v.Machine == "" {
		return nil
	}
	busy[v.Machine] = true
	var more []verified.Verifier
	for len(more) < k-1 {
		x := m.specFree(c, busy)
		if x.Machine == "" {
			break
		}
		busy[x.Machine] = true
		more = append(more, x)
	}
	return more
}

// shardLaunch: 추측 r을 기계들에 몫 i/K로 띄우고 나눔 상태를 적는다. 하나도 못 띄우면 false(부른 쪽이 지금처럼 모름으로)
func (m *M) shardLaunch(r *specRow, vs []verified.Verifier, lanes string, now int64) bool {
	k := len(vs)
	var got []string
	for i, v := range vs {
		if m.specDispatch(v, r.id, fmt.Sprintf("%d/%d", i+1, k)) {
			got = append(got, fmt.Sprintf("%d:%s", i+1, v.Machine))
		}
	}
	if len(got) == 0 {
		return false
	}
	front := "-"
	if len(m.b) > 0 {
		front = m.b[0].s
	}
	line := fmt.Sprintf("%s %s %d k=%d %s\n", r.id, front, now, k, strings.Join(got, " "))
	if err := writeVia(m.mj+"/specshard", line); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 나눔 상태를 적지 못했다(맨 앞 기다림 상한에서 마무리로 넘긴다): %v\n", err)
	}
	fmt.Printf("hc-lane merger: 추측 검증 나눠 띄움 — %s → %d몫(%s) (부모 트리 %s)\n", lanes, k, strings.Join(got, " "), first8(r.parent))
	return true
}

// shardState: <일지 폴더>/specshard
type shardState struct {
	id, front string
	t0, t1    int64
	k         int
	parts     [][2]string // 몫 번호 · 기계
	fin       string
	busy      map[string][]string // 몫 번호 → 일 없이 바쁨으로 끝난 기계들(「b<i>=<기계>,…」 — shardBusyRedo)
}

// line: 나눔 상태 한 줄(shardRead가 읽는 꼴)
func (s shardState) line() string {
	var f []string
	for _, p := range s.parts {
		f = append(f, p[0]+":"+p[1])
		if b := s.busy[p[0]]; len(b) > 0 {
			f = append(f, "b"+p[0]+"="+strings.Join(b, ","))
		}
	}
	if s.fin != "" {
		f = append(f, fmt.Sprintf("fin=%s@%d", s.fin, s.t1))
	}
	return fmt.Sprintf("%s %s %d k=%d %s\n", s.id, s.front, s.t0, s.k, strings.Join(f, " "))
}

func (m *M) shardRead() (shardState, bool) {
	var s shardState
	b, err := os.ReadFile(m.mj + "/specshard")
	if err != nil {
		return s, false
	}
	f := strings.Fields(string(b))
	if len(f) < 5 {
		return s, false
	}
	s.id, s.front = f[0], f[1]
	s.t0, _ = strconv.ParseInt(f[2], 10, 64)
	s.k, _ = strconv.Atoi(strings.TrimPrefix(f[3], "k="))
	s.busy = map[string][]string{}
	for _, x := range f[4:] {
		if fin, ok := strings.CutPrefix(x, "fin="); ok {
			mach, at, _ := strings.Cut(fin, "@")
			s.fin = mach
			s.t1, _ = strconv.ParseInt(at, 10, 64)
		} else if i, ms, ok := strings.Cut(x, "="); ok && strings.HasPrefix(i, "b") {
			s.busy[i[1:]] = strings.Split(ms, ",")
		} else if i, mach, ok := strings.Cut(x, ":"); ok {
			s.parts = append(s.parts, [2]string{i, mach})
		}
	}
	return s, s.k > 1 && len(s.parts) > 0
}

// shardStep: 맨 앞이 나눔 중이면 — 모든 몫이 끝났거나 맨 앞 기다림 상한이면 한 기계에 마무리를 띄운다(몫이 멈췄어도 처음부터 다시 돌지
// 않는다 — 끝난 몫의 조각은 마무리가 건너뛴다). 맡길 기계가 없으면 다음 바퀴에(상한을 넘겼으면 지금 길 — frontOver)
func (m *M) shardStep(c *specCtx, st []specStep, busy map[string]bool, now int64) {
	s, ok := m.shardRead()
	if !ok || len(st) == 0 || st[0].row == nil || st[0].row.id != s.id || s.fin != "" || st[0].row.state != specRun {
		return
	}
	r := st[0].row
	left := 0
	for _, p := range s.parts {
		if !shardEnded(c.refs, r.id, p[0]) {
			left++
		}
	}
	over := now-r.at > specFrontWait()
	if left > 0 && !over {
		return
	}
	if fresh, _ := m.specFresh(r); left > 0 && fresh {
		return // 상한을 넘겼어도 남은 몫이 진행 중 — 더 기다린다(frontOver가 같은 판정으로 merger_spec_timeout까지, 감사 atp3qsv 중간 1)
	}
	why := "모든 몫이 끝남"
	if left > 0 {
		why = fmt.Sprintf("맨 앞 기다림 상한 — 남은 몫 %d은 마무리가", left)
	}
	if !m.specRedo(c, r, busy, nil, "", "", st[0].g.lanes, now) {
		fmt.Printf("hc-lane merger: 나눔 %s(%s) — 마무리를 맡길 기계가 없다, 다음 바퀴에\n", st[0].g.lanes, why)
		return
	}
	fmt.Printf("hc-lane merger: 나눔 %s(%s) — 마무리 %s\n", st[0].g.lanes, why, r.machine) // 나눔 끝 표시는 specRedo가(shardHandoff)
}

// shardBusyRedo: 나눔 중인 맨 앞의 몫이 일 없이 바쁨(끝 state=busy·통과 조각 없음)으로 끝났으면 그 바퀴에(뒤 추측보다 먼저) 같은 몫
// (같은 번호·같은 아이디)을 다른 한가한 기계에 다시 맡긴다 — 보통 추측의 바쁨 다시 고름(specBusyRedo)과 같은 상한 busy_max를 몫마다.
// 그 몫에 바쁨을 낸 기계와 그 추측의 다른 몫이 도는 기계는 빼고, 맡길 기계가 없거나 상한을 넘기거나 다른 몫이 다 끝났으면 지금처럼
// 마무리가 그 몫을 돈다.
// 다시 맡기면 바쁨 끝 참조를 지우고(본 값 lease — 마무리가 그 몫을 끝남으로 보지 않게) 나눔 상태의 그 몫 기계를 바꾼다(기록·진행 판정이 새 기계를 본다)
func (m *M) shardBusyRedo(c *specCtx, st []specStep, busy map[string]bool, now int64) {
	s, ok := m.shardRead()
	if !ok || len(st) == 0 || st[0].row == nil || st[0].row.id != s.id || s.fin != "" || st[0].row.state != specRun {
		return
	}
	r := st[0].row
	run := map[string]bool{}
	for _, p := range s.parts {
		if !shardEnded(c.refs, r.id, p[0]) {
			run[p[1]] = true
		}
	}
	changed := false
	for j, p := range s.parts {
		end := shardEndRef(c.refs, r.id, p[0])
		// 다른 몫이 다 끝났으면 다시 맡겨도 함께 끝날 몫이 없다 — 마무리가 바로 그 몫을 돈다(shardStep)
		if end == "" || len(run) == 0 || c.refs[specParts+r.id+"-s"+p[0]] != "" || !shardEndBusy(end) {
			continue
		}
		seen := s.busy[p[0]]
		if !slices.Contains(seen, p[1]) {
			seen, changed = append(seen, p[1]), true
			s.busy[p[0]] = seen
		}
		if len(seen) > m.busyMax() {
			fmt.Printf("hc-lane merger: 나눔 몫 %s/%d 이 %d번 바쁨(%s) — 손잡이 busy_max %d를 넘어 다시 맡기지 않는다(마무리가)\n", p[0], s.k, len(seen), strings.Join(seen, "·"), m.busyMax())
			continue
		}
		skip := maps.Clone(run)
		for _, x := range seen {
			skip[x] = true
		}
		v := m.specPick(c, busy, skip, "", true)
		if v.Machine == "" {
			fmt.Printf("hc-lane merger: 나눔 몫 %s/%d 이 %s 에서 바쁨 — 맡길 한가한 기계 없음(마무리가)\n", p[0], s.k, p[1])
			continue
		}
		if runO("push", "-q", "--force-with-lease="+end+":"+c.refs[end], "origin", ":"+end) != 0 {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 나눔 몫 %s/%d 의 바쁨 끝 참조를 지우지 못했다 — 다음 바퀴에\n", p[0], s.k)
			continue
		}
		delete(c.refs, end)
		busy[v.Machine], run[v.Machine] = true, true
		// 못 띄웠어도 그 기계로 적는다 — 몫이 끝나지 않으니 진행 판정(specFresh)이 터미널 없음으로 보고 상한에서 마무리가
		m.specDispatch(v, r.id, fmt.Sprintf("%s/%d", p[0], s.k))
		s.parts[j][1], changed = v.Machine, true
		m.wre = p[1] + "→" + v.Machine // 일지 「바쁨으로 다시 고름(<기계>→<기계>)」(jround.go)
		fmt.Printf("hc-lane merger: 나눔 몫 %s/%d 이 %s 에서 일 없이 바쁨 — %s 로 다시 맡김\n", p[0], s.k, p[1], v.Machine)
	}
	if changed {
		if err := writeVia(m.mj+"/specshard", s.line()); err != nil {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 나눔 상태를 적지 못했다(다시 맡긴 몫이 기록에 안 보인다): %v\n", err)
		}
	}
}

// shardHandoff: 추측 r을 다시 띄웠다(specRedo — 마무리·한 번 더 확인·바쁨·상한으로 옮김) — 그 아이디가 나눔 중이었으면 나눔은 여기서
// 끝나고 그 실행이 이어 맡는다(「fin=<기계>@<초>」, 일지 「마무리 <기계> n초」)
func (m *M) shardHandoff(r *specRow, now int64) {
	s, ok := m.shardRead()
	if !ok || s.id != r.id || s.fin != "" {
		return
	}
	s.fin, s.t1 = r.machine, now
	if err := writeVia(m.mj+"/specshard", s.line()); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 나눔 상태를 적지 못했다(일지에 마무리가 빠진다): %v\n", err)
	}
}

// shardNote: 일지 「추측 기다림」 줄에 덧붙일 글 — 그 맨 앞이 나눠 돌았으면 「, 나눔 K(<기계>,…)[, 마무리 <기계> n초]」
func (m *M) shardNote(front string, last int64) string {
	s, ok := m.shardRead()
	if !ok || s.front != front {
		return ""
	}
	var ms []string
	for _, p := range s.parts {
		ms = append(ms, p[1])
	}
	o := fmt.Sprintf(", 나눔 %d(%s)", s.k, strings.Join(ms, ","))
	if s.fin != "" {
		o += fmt.Sprintf(", 마무리 %s %d초", s.fin, max(0, last-s.t1))
	}
	return o
}

// specShardEnd: 일꾼 — 몫이 시험 실패 아닌 끝(통과·환경 실패·모름·바쁨). spec 참조가 그대로일 때만 끝 참조 <아이디>-s<i>of<K>를
// 쓴다(통과 기록은 안 쓴다 — 이름의 K로 병합기가 몫이 다 끝났는지 안다)
func specShardEnd(r specRow, shard, state string) int {
	_, k, _ := strings.Cut(shard, "/")
	own := shardOwn(r.id, shard) + "of" + k
	now, rc := lsRemote("origin", specRefs+r.id)
	if rc != 0 {
		return 1
	}
	if c, _, _ := strings.Cut(now, "\t"); c != r.commit {
		fmt.Println("hc-lane merger: 병합기가 이 추측을 버렸거나 다시 띄웠다 — 몫의 끝을 남기지 않는다")
		return 0
	}
	et, rc := out("hash-object", "-t", "tree", "-w", "/dev/null")
	if rc != 0 {
		return 1
	}
	ec, ok := commitTree("", et, "", fmt.Sprintf("hc-merger-spec-shard id=%s machine=%s at=%d state=%s", own, machine(), time.Now().Unix(), state))
	if !ok || runO("push", "-q", "-f", "origin", ec+":"+specShards+own) != 0 {
		return 1
	}
	fmt.Printf("hc-lane merger: 나눔 몫 %s 끝(%s) — 통과 기록은 마무리가 쓴다\n", own, state)
	return 0
}
