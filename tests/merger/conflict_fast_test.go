package merger_test

// conflict-fast B(2026-11-23 — 충돌 반송은 줄 맨 앞에 와야 드러나 중앙값 29분을 기다렸다): 바퀴 시작에 줄에 선 모든 제출을 지금 기준과
// 견줘, 실제 rebase로도 충돌하는 것은 차례 전에 지금의 충돌 반송과 같은 길(refs/merge-failed · 막힘 사건 「착지 실패(종료 3, 충돌 …)」 ·
// 일지 줄, 까닭에 「미리 훑기」)로 돌려보낸다. 3자 합침(merge-tree)으로는 충돌인데 커밋마다 얹는 rebase는 깨끗한 제출은 건드리지 않는다.

import (
	"regexp"
	"strings"
	"testing"
)

func TestMergerPrescan(t *testing.T) {
	k := newRK(t)
	f := k.fixture("prescan")
	f.enable()
	f.commitFile("a.txt", "a")
	cmust(k, "submit la", f.submit("la"))
	f.w(`git checkout -qB lx main`)
	f.commitFile("shared", "x")
	cmust(k, "submit lx", f.submit("lx"))
	// merge-tree는 충돌, 커밋마다 얹는 rebase는 깨끗: 첫 커밋이 shared를 기준과 같게 고치고 파일 하나를 더함(patch-id는 기준과 다름),
	// 둘째가 그 위에서 shared를 고침
	f.w(`git checkout -qB lr main && printf 'remote\n' >shared && printf '1\n' >lr1 && git add -- shared lr1 && git commit -qm lr-1`)
	f.commitFile("shared", "lr")
	cmust(k, "submit lr", f.submit("lr"))
	// 기준과 같은 patch-id 커밋 + 기준이 그 뒤 같은 파일을 다시 고침: --reapply-cherry-picks rebase는 충돌하지만 일반 rebase(전용 착지
	// 스크립트)는 그 커밋을 건너뛰어 깨끗 — 미리 반송하지 않는다(감사 atoxvrv-yecgh5 중간 2)
	f.w(`git checkout -qB lc main && printf 'x\n' >cf && git add -- cf && git commit -qm lc-a`)
	f.commitFile("g", "1")
	cmust(k, "submit lc", f.submit("lc"))
	// 기준: 메시지를 달리해 레인 커밋과 같은 초에 만들어져도 SHA가 같아지지 않게(감사 atoxvrv-yecgh5 중간 1)
	f.w(`git checkout -q main && printf 'remote\n' >shared && git commit -qam main-shared && printf 'x\n' >cf && git add -- cf && git commit -qm main-pick &&
printf 'y\n' >cf && git commit -qam main-cf && git push -q origin main`)
	f.w(`git fetch -q origin && git merge-tree --write-tree origin/main lr >/dev/null; [ $? = 1 ]`) // 전제: lr은 3자 합침으로 충돌
	f.w(`git merge-tree --write-tree origin/main lc >/dev/null; [ $? = 1 ] && git rev-list --cherry-mark --right-only origin/main...lc | grep -q '^='`)

	r := f.tick("HC_LANE_MERGER_BATCH=1")
	k.check("바퀴: 종료 0", r.RC == 0, clog(r))
	j := f.journal()
	idx := func(lane, result string) int {
		for i, row := range j {
			if col(row, 3) == lane && col(row, 10) == result {
				return i
			}
		}
		return -1
	}
	xi, ai := idx("lx", "돌려보냄"), idx("la", "착지")
	k.check("미리 훑기: 뒤에 선 lx가 앞 la 착지 전에 반송", xi >= 0 && ai >= 0 && xi < ai, clog(r))
	k.check("미리 훑기: 일지 까닭은 충돌 반송 꼴 + 미리 훑기", xi >= 0 && strings.HasPrefix(col(j[xi], 12), "착지 실패(종료 3, 충돌 shared)") &&
		strings.Contains(col(j[xi], 12), "미리 훑기"))
	k.check("미리 훑기: refs/merge-failed로 옮김, lane/lx는 남음", regexp.MustCompile(`refs/merge-failed/[0-9]{10}-lx`).MatchString(f.refs("refs/merge-failed")) &&
		f.ogOK("rev-parse", "-q", "--verify", "refs/heads/lane/lx"))
	ev := ""
	for _, row := range f.events() {
		if col(row, 4) == "lx" && col(row, 5) == "막힘" {
			ev = col(row, 7)
		}
	}
	// hc-fill 자동 다시 맞춤(cmd/hc-fill/reland.go relandRe)이 읽는 꼴 그대로
	m := regexp.MustCompile(`^병합기: 착지 실패\(종료 3, 충돌 (.*?)\) — `).FindStringSubmatch(ev)
	k.check("미리 훑기: 막힘 사건이 다시 맞춤 꼴(충돌 파일 shared)·로그 경로", m != nil && m[1] == "shared" && strings.Contains(ev, ", 로그 "), ev)
	k.check("충돌 없는 la는 그대로 착지", f.og("show", "main:a.txt") == "a")
	k.check("merge-tree만 충돌·rebase 깨끗한 lr은 미리 반송 안 함(차례에 착지)", idx("lr", "착지") >= 0 && f.og("show", "main:shared") == "lr" &&
		!strings.Contains(f.refs("refs/merge-failed"), "-lr"), clog(r))
	lcPre := false
	for _, row := range j {
		lcPre = lcPre || (col(row, 3) == "lc" && strings.Contains(col(row, 12), "미리 훑기"))
	}
	k.check("기준과 같은 patch-id 커밋이 든 lc는 미리 반송 안 함(차례에 맡김)", !lcPre, clog(r))
	k.check("미리 훑기: 클론에 남은 rebase·작업 트리 없음", !exists(f.mclone+"/.git/rebase-merge") && !exists(f.mclone+"/.git/rebase-apply") &&
		countNonEmpty(f.git(f.mclone, "worktree", "list")) == 1)
}
