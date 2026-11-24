package main

// 줄 미리 훑기(conflict-fast B, 2026-11-23 — 병합기 일지 11/20~23의 rebase 충돌 반송 28건은 줄 맨 앞에 와야 드러나 중앙값 29분을 기다린
// 뒤 0분 만에 반송됐다. 기준이 움직인 바퀴마다 줄 전체를 견줬다면 합 1029분 일찍 알았다): 바퀴 시작(큐를 읽은 뒤, 묶음을 고르기 전)에
// 줄에 선 모든 제출을 지금 기준과 견줘, 실제 rebase로도 충돌하는 것을 차례 전에 지금의 충돌 반송과 같은 길(fail — refs/merge-failed,
// 막힘 사건 「착지 실패(종료 3, 충돌 <파일>)」, 일지 줄)로 돌려보낸다. 까닭 뒤에 「 — 미리 훑기(차례 전)」를 붙여 가른다(hc-fill 다시 맞춤의
// 꼴 「…충돌 <파일>) — 」와 통계의 「종료 3」 판독은 그대로).
//
// 판정은 차례가 와서 착지가 할 rebase와 같게: 3자 합침(merge-tree)이 충돌인 것만 골라, 이 클론(같은 rerere 기록)의 임시 작업 트리에서
// 착지의 rb와 같은 명령(rebase --reapply-cherry-picks <기준>)을 해 본다. rebase가 커밋을 얹다 내용 충돌로 멈췄고 rerere가 아무것도
// 풀지 않았을 때만 반송한다. 깨끗함·모름(git 실패·시간 넘김)·rerere가 손댐·병합 커밋이 든 제출·착지 스크립트를 들이는 제출(차례에 다른
// 까닭으로 반송된다)·기준과 같은 patch-id 커밋이 든 제출(전용 착지 스크립트의 일반 rebase는 그 커밋을 건너뛴다)·기록을 못 읽는 제출은
// 건드리지 않고 차례에 맡긴다.
// 같은 기준·같은 제출은 한 번만 본다($mj/prescan: 첫 줄 기준, 그 뒤 본 제출 SHA).

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/proc"
)

// ponytail: 바퀴마다 rebase 시험 8번·60초까지 — 못 본 것은 다음 바퀴에 이어 본다. 줄이 아주 길어 늘 밀리면 상한을 올린다
const (
	prescanTrials = 8
	prescanTime   = 60 * time.Second
)

var rerereTouched = regexp.MustCompile(`(?m)^(Resolved|Staged) '.*' using previous resolution\.$`)

func (m *M) prescan() {
	qb := refsha(m.qs, "refs/heads/"+m.base)
	q, ok := m.queue()
	if qb == "" || !ok || len(q) == 0 {
		return
	}
	cf := m.mj + "/prescan"
	seen := map[string]bool{}
	if b, err := os.ReadFile(cf); err == nil {
		if ls := strings.Split(strings.TrimSpace(string(b)), "\n"); ls[0] == qb {
			for _, s := range ls[1:] {
				seen[s] = true
			}
		}
	}
	var todo [][2]string
	for _, e := range q {
		if !seen[e[1]] {
			todo = append(todo, e)
		}
	}
	if len(todo) == 0 {
		return
	}
	if fetchr(false, "-q", "origin", "+refs/heads/"+m.base+":refs/remotes/origin/"+m.base, "+refs/merge-queue/*:refs/hc-merger/q/*") != 0 {
		return // 모름 — 차례에 맡긴다(바퀴 본체가 같은 fetch를 다시 하고 실패면 쉰다)
	}
	B, rc := out("rev-parse", "-q", "--verify", "refs/remotes/origin/"+m.base+"^{commit}")
	if rc != 0 || B != qb {
		return // 그 사이 기준이 움직였다 — 다음 바퀴에 새 기준으로
	}
	hb, rc := out("ls-tree", "--name-only", B, "--", ".claude/herdr-land.sh")
	if rc != 0 {
		return
	}
	start, trials := time.Now(), 0
	for _, e := range todo {
		if trials >= prescanTrials || time.Since(start) > prescanTime {
			break
		}
		mq, ms := e[0], e[1]
		mn := strings.TrimPrefix(mq, "refs/merge-queue/")
		if _, err := os.Stat(m.mj + "/fail/" + mn); err == nil {
			continue // 돌려보내기를 다시 하는 중(fretry)
		}
		ln := ""
		if len(mn) > 11 && digits.MatchString(mn[:10]) && mn[10] == '-' {
			ln = mn[11:]
		}
		mm := dashIf(refsha(m.qs, "refs/merge-queue-meta/"+mn))
		if mt, ok := readMeta(mm, ms); !laneOK(ln) || m.reserved(ln) || !ok || (mt.base != "" && mt.base != m.base) {
			continue // 이름·기록·기본 브랜치 — 차례의 판정(pickBatch)에 맡긴다
		}
		v, lg := m.prescanOne(B, hb, ms, ln, &trials)
		seen[ms] = true // 모름도 본 것으로 — 바퀴마다 되풀이하지 않고 차례에 맡긴다(상한에 걸려 못 본 것만 다음 바퀴에)
		if v != psConflict {
			continue
		}
		m.B = B
		why := "착지 실패(종료 3, 충돌 " + m.conflicts(ms) + ") — 미리 훑기(차례 전)"
		if m.conflictTake(mq, ms, ln, mm, B, why) { // 병합 쪽에서 푼다(resolve.go) — 반송하지 않는다
			m.qs = dropRefs(m.qs, mq, "refs/merge-queue-meta/"+mn)
			continue
		}
		if !m.fail(mq, ms, ln, mm, lg, why) {
			m.mer = true
			continue
		}
		m.qs = dropRefs(m.qs, mq, "refs/merge-queue-meta/"+mn) // 이 바퀴의 묶음·추측에서 뺀다(지난 추측은 specClean이 거둔다)
	}
	var sb strings.Builder
	sb.WriteString(B + "\n")
	for _, e := range q {
		if seen[e[1]] {
			sb.WriteString(e[1] + "\n")
		}
	}
	if err := writeVia(cf, sb.String()); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 미리 훑기 기록을 쓰지 못했다(다음 바퀴에 다시 본다): %v\n", err)
	}
}

// pscan: 미리 훑기 판정 — 0값이 모름
type pscan int

const (
	psUnknown  pscan = iota // 모름(git 실패 — 차례에 맡긴다)
	psPass                  // 깨끗·건드리지 않음(이미 들었음·차례의 판정과 다를 수 있는 꼴)
	psConflict              // 실제 rebase도 충돌(lg = 그 rebase 로그)
)

// prescanOne: 제출 s를 기준 B 위로 미리 훑는다. rebase를 해 보면 trials를 하나 센다
func (m *M) prescanOne(B, hb, s, ln string, trials *int) (pscan, string) {
	switch runQ("merge-base", "--is-ancestor", s, B) {
	case 1:
	case 0:
		return psPass, "" // 이미 들었음 — 차례에
	default:
		return psUnknown, ""
	}
	switch _, rc := gitx.MergeConflicts("", B, s); rc {
	case 0:
		return psPass, ""
	case 1:
	default:
		return psUnknown, ""
	}
	mg, rc1 := outQ("rev-list", "-1", "--merges", B+".."+s)
	hl, rc2 := outQ("ls-tree", "--name-only", s, "--", ".claude/herdr-land.sh")
	eq, rc3 := gitx.CherryEquivalent("", B, s)
	if rc1 != 0 || rc2 != 0 || rc3 != 0 {
		return psUnknown, ""
	}
	// 병합 풀이를 다시 만드는 착지·착지 스크립트 반송·기준과 같은 patch-id 커밋(전용 착지 스크립트의 일반 rebase는 건너뛰고
	// 내장 착지의 --reapply-cherry-picks는 다시 얹는다 — 감사 atoxvrv-yecgh5 중간 2) — 차례의 판정과 다를 수 있다
	if mg != "" || (hl != "" && hb == "") || eq {
		return psPass, ""
	}
	*trials++
	lg := fmt.Sprintf("%s/%s-prescan-%s-%d.log", m.ml, ln, time.Now().Format("20060102-150405"), os.Getpid())
	return m.trial(B, s, lg), lg
}

// trial: 임시 작업 트리에서 s를 B 위로 rebase해 본다(착지 rb와 같은 명령, 이 클론의 rerere 그대로, 훅 없음). 충돌은 내용 충돌로 멈추고
// rerere가 손대지 않았을 때만, 그 밖의 실패는 모름. 멈춘 rebase는 abort(rerere 기록도 rerere clear로 지워진다), 작업 트리는 지운다
func (m *M) trial(B, s, lg string) pscan {
	wt, err := mktempDir(m.mwr)
	if err != nil {
		return psUnknown
	}
	defer func() {
		if run("worktree", "remove", "-f", "-f", wt) != 0 {
			os.RemoveAll(wt) // 다음 wtclean이 등록을 거둔다
		}
	}()
	if inLog("", lg, "-c", "core.hooksPath=/dev/null", "worktree", "add", "-q", "-f", "--detach", wt, s) != 0 {
		return psUnknown
	}
	var b bytes.Buffer
	c := exec.Command("git", "-c", "core.hooksPath=/dev/null", "rebase", "-q", "--reapply-cherry-picks", B)
	c.Dir, c.Env, c.Stdout, c.Stderr = wt, append(os.Environ(), "LC_ALL=C"), &b, &b
	rc := proc.Code(proc.Run(c))
	if f, err := os.OpenFile(lg, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666); err == nil {
		fmt.Fprintf(f, "hc-lane merger: 미리 훑기 — %s 를 기준 %s 위로 rebase(종료 %d)\n%s", first8(s), first8(B), rc, b.String())
		f.Close()
	}
	if rc == 0 {
		return psPass
	}
	v := psUnknown
	if rc == 1 && gitx.Run(wt, nil, nil, "rev-parse", "-q", "--verify", "REBASE_HEAD") == 0 &&
		strings.Contains(b.String(), "CONFLICT (") && !rerereTouched.Match(b.Bytes()) {
		v = psConflict
	}
	inLog(wt, lg, "rebase", "--abort")
	return v
}

// dropRefs: ls-remote 목록(「<SHA> <참조>」 줄)에서 그 참조 줄을 뺀다
func dropRefs(list string, refs ...string) string {
	var keep []string
	for _, line := range strings.Split(list, "\n") {
		if f := strings.Fields(line); len(f) == 2 && slices.Contains(refs, f[1]) {
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "\n")
}
