package main

// 미리 줄(preverify, 2026-11-24 사용자 「지금 herdr-verify.sh 검증 돌리는거 좀 너무 과한건 아니야?」 → 「모두 처리해주고」).
//
// 레인이 「hc-lane report 완료 <sha>」를 적으면 hc-lane이 refs/merge-pre/<레인> → 표식 커밋(부모 = 그 sha, internal/verified/pre.go)을 올린다.
// 병합기는 진짜 줄(refs/merge-queue/*)의 추측을 다 챙긴 뒤 **빈 검증 기계가 남을 때만** 표식마다 「기준 트리 + 그 sha 하나」 추측을
// 지금 추측과 같은 꼴(아이디 specID(기준 트리, sha) · 같은 참조 · 같은 일꾼 · 같은 통과 기록)로 짓고 띄운다(큐가 빈 바퀴에도).
// 감사가 통과해 같은 sha가 혼자 제출되고 그 사이 기준이 안 움직였으면 병합기의 맨 앞 무리 아이디가 이 추측과 같아 spec.go 바퀴 3
// (통과면 빠른 착지 · 도는 중이면 기다림)으로 간다. 착지 판정(hc-lane land의 「rebase한 트리 = 기록의 트리」)은 바뀌지 않는다 —
// 표식은 착지 후보가 아니고(큐로 읽는 길이 없다) 기록을 더 일찍 만들 뿐이다.
//
// 치우기(본 값 lease): 끔·검증 기계 목록 없음, 상한(HC_LANE_PRE_MAX, 기본 45분) 넘김, 그 레인이 제출됨(진짜 줄이 넘겨받음),
// 그 sha가 이미 기준에 있음 → 표식을 지운다(레인을 정리하면 hc-lane clean도 지운다). 기준이 움직였거나 새 sha로 다시 보고했으면
// 옛 추측은 길에서 벗어나 specClean이 치운다. 표식 하나의 추측은 처음 한 번 + 기준 이동으로 HC_LANE_PRE_REBUILD(기본 1)번까지만 짓는다.
// 미리 검증 실패는 반송하지 않는다 — 제출되면 지금처럼 병합기가 직접 검증한다(spec-fail 참조가 기록이다).

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/verified"
)

const preLocal = "refs/hc-merger/pre/" // 받은 표식(로컬 사본)

// preMark: 원격 표식 하나
type preMark struct {
	ref, mark string // 원격 참조 · 표식 커밋
	lane, sha string // 레인 · 완료 보고한 커밋
	base      string
	at        int64
	ok        bool // 꼴이 맞음(아니면 치운다)
}

// specPreMax: HC_LANE_PRE_MAX(초, 기본 2700, 60 이상). 넘긴 표식은 치운다 — 미리 검증이 값을 하는 것은 「완료 보고 → 감사 → 제출」
// 사이뿐이라 감사가 끝날 만한 시간(45분)으로 둔다(preverify-wait-cap — 6시간이던 때는 제출하지 않을 표식이 기준이 움직일 때마다 전체 검증을 다시 돌렸다)
func specPreMax() int64 {
	return int64(intSetting("HC_LANE_PRE_MAX", os.Getenv("HC_LANE_PRE_MAX"), 2700, 60))
}

// specPreRebuild: HC_LANE_PRE_REBUILD(번, 기본 1, 0 이상). 표식 하나의 추측을 기준이 움직여 다시 짓는 횟수 상한(처음 한 번은 빼고).
// 넘기면 그 표식은 더 짓지 않는다 — 제출되면 지금처럼 병합기가 직접 검증한다
func specPreRebuild() int {
	return intSetting("HC_LANE_PRE_REBUILD", os.Getenv("HC_LANE_PRE_REBUILD"), 1, 0)
}

// preCounts: 표식 커밋마다 그 추측을 지은 횟수($mj/pre — 「<표식 커밋> <횟수>」 줄). 없거나 깨졌으면 빈 값(한 번 더 지을 뿐).
// 다시 완료 보고하면 표식 커밋이 새로 생겨 처음부터 센다
func (m *M) preCounts() map[string]int {
	n := map[string]int{}
	b, err := os.ReadFile(m.mj + "/pre")
	if err != nil {
		return n
	}
	for _, l := range strings.Split(string(b), "\n") {
		if f := strings.Fields(l); len(f) == 2 {
			if k, err := strconv.Atoi(f[1]); err == nil && k > 0 {
				n[f[0]] = k
			}
		}
	}
	return n
}

// preCountsWrite: 지금 남은 표식 것만 적는다(치운 표식의 줄은 여기서 빠진다). 못 적으면 다음 바퀴에 한 번 더 지을 뿐
func (m *M) preCountsWrite(n map[string]int) {
	if len(n) == 0 {
		removeQuiet(m.mj + "/pre")
		return
	}
	var sb strings.Builder
	for _, k := range slices.Sorted(maps.Keys(n)) {
		fmt.Fprintf(&sb, "%s %d\n", k, n[k])
	}
	if err := writeVia(m.mj+"/pre", sb.String()); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 미리 줄 — 지은 횟수를 적지 못했다: %v\n", err)
	}
}

// preRead: 받은 표식을 읽는다(specLoad가 받은 뒤). 원격 값과 다른 것(받는 사이 바뀜)은 다음 바퀴에
func preRead(remote map[string]string) ([]preMark, bool) {
	if len(remote) == 0 {
		return nil, true
	}
	list, rc := out("for-each-ref", "--format=%(refname) %(objectname) %(parent) %(contents:subject)", preLocal)
	if rc != 0 {
		return nil, false
	}
	var ps []preMark
	for _, line := range strings.Split(list, "\n") {
		f := strings.SplitN(line, " ", 4)
		if len(f) != 4 {
			continue
		}
		ref := verified.PreRefs + strings.TrimPrefix(f[0], preLocal)
		if remote[ref] != f[1] {
			continue
		}
		p := preMark{ref: ref, mark: f[1], sha: f[2]}
		p.lane, p.base, p.at, p.ok = verified.ParsePre(f[3])
		p.ok = p.ok && p.lane == strings.TrimPrefix(ref, verified.PreRefs) && laneOK(p.lane) && len(p.sha) == 40
		ps = append(ps, p)
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].at < ps[j].at || ps[i].at == ps[j].at && ps[i].lane < ps[j].lane })
	return ps, true
}

// preBase: 큐가 빈 바퀴 — 미리 줄 추측의 기준(m.B·m.btree)과 로그를 잡는다. 못 잡으면 false(이 바퀴는 치우지도 않는다)
func (m *M) preBase() bool {
	if fetchr(false, "-q", "origin", "+refs/heads/"+m.base+":refs/remotes/origin/"+m.base) != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 미리 줄 — 기준을 받지 못했다(git fetch) — 다음 바퀴에")
		return false
	}
	B, rc := out("rev-parse", "-q", "--verify", "refs/remotes/origin/"+m.base+"^{commit}")
	if rc != 0 {
		return false
	}
	t, rc := out("rev-parse", "--verify", B+"^{tree}")
	if rc != 0 {
		return false
	}
	m.B, m.btree = B, t
	m.lgf = fmt.Sprintf("%s/pre-%s.log", m.ml, time.Now().Format("20060102"))
	return true
}

// specPre: 표식을 가려 치울 것을 c.preDel에 적고, 남은 것마다 「기준 + 그 sha」 추측을 찾거나(없고 빈 기계가 있으면) 지어 띄운다.
// 돌려주는 단계는 치우기(specClean)에서 길처럼 지킨다. 진짜 줄 추측을 다 챙긴 뒤에 부른다
func (m *M) specPre(c *specCtx, busy map[string]bool, now int64) []specStep {
	if len(c.pre) == 0 {
		removeQuiet(m.mj + "/pre") // 표식이 없으면 지은 횟수도 없다
		return nil
	}
	queued := map[string]bool{}
	for _, line := range strings.Split(m.qs, "\n") {
		if f := strings.Fields(line); len(f) == 2 {
			if mn, ok := strings.CutPrefix(f[1], "refs/merge-queue/"); ok && len(mn) > 11 && mn[10] == '-' {
				queued[mn[11:]] = true
			}
		}
	}
	maxAge, limit := specPreMax(), 1+specPreRebuild()
	counts, seen := m.preCounts(), map[string]int{}
	defer func() {
		if !maps.Equal(seen, counts) {
			m.preCountsWrite(seen)
		}
	}()
	var keep []specStep
	building := true
	for _, p := range c.pre {
		if k := counts[p.mark]; k > 0 {
			seen[p.mark] = k // 치우는 표식은 아래에서 뺀다
		}
		switch {
		case !p.ok, c.preOff, len(c.vers) == 0, now-p.at > maxAge, queued[p.lane]:
			c.preDel = append(c.preDel, [2]string{p.ref, p.mark})
			delete(seen, p.mark)
			continue
		case p.base != m.base: // 다른 기본 브랜치 — 그 병합기의 것
			continue
		}
		switch run("merge-base", "--is-ancestor", p.sha, m.B) {
		case 0: // 이미 들어감
			c.preDel = append(c.preDel, [2]string{p.ref, p.mark})
			delete(seen, p.mark)
			continue
		case 1:
		default:
			continue // 모름 — 다음 바퀴에
		}
		g := newGroup([]entry{{s: p.sha, l: p.lane}})
		id := specID(m.btree, g.key)
		var row *specRow
		running := false
		for i := range c.rows {
			if c.rows[i].id == id {
				row = &c.rows[i]
			} else if c.rows[i].state == specRun && c.rows[i].group == g.key {
				running = true // 옛 기준의 같은 sha 추측이 아직 돈다 — 끝날 때까지 하나만
			}
		}
		// 지은 횟수 상한(처음 + 기준 이동 HC_LANE_PRE_REBUILD번): 제출하지 않을 표식(감사 지적을 고치는 중·정리만 함)이 기준이 움직일
		// 때마다 전체 검증을 다시 돌리지 않게
		if row == nil && !running && building && seen[p.mark] < limit {
			v := m.specFree(c.vers, busy)
			if v.Machine == "" {
				building = false
			} else if r, ok := m.specBuild([]specStep{{g: g, parent: m.btree}}, v, now); !ok {
				building = false
			} else {
				seen[p.mark]++
				c.rows = append(c.rows, r)
				row = &c.rows[len(c.rows)-1]
				if r.state == specRun {
					busy[v.Machine] = true
					fmt.Printf("hc-lane merger: 미리 줄 — %s(%s)를 %s 에서 미리 검증(완료 보고 뒤 감사 동안, 제출 전)\n", p.lane, first8(p.sha), v.Machine)
				}
			}
		}
		if row != nil {
			keep = append(keep, specStep{g: g, parent: m.btree, row: row})
		}
	}
	return keep
}
