package hclane_test

// ht-autosubmit-rebase(2026-11-26 — 대기열 제출이 「판정이 붙은 제출 — rebase하면 승인 커밋이 바뀐다」 종료 3으로 7번 에이전트에 넘어감):
// 판정(--verdict)이 붙은 병합기 제출 세 꼴을 못 박는다.
// ① 기준만 움직임(충돌 없음) → rebase 없이 그대로 큐에, 기록한 승인 patch-id는 병합기가 기준 위로 얹은 뒤의 변경과 같다(승인 그대로)
// ② 실제 충돌 → 종료 3·큐 없음, 대기열이 넘김 까닭으로 쓰는 마지막 줄에 충돌 파일이 있다(전에는 그 줄에 파일이 없었다)
// ③ 승인 뒤 커밋이 더 생김 → 종료 2(승인 커밋과 제출 HEAD가 다르다 — 새 감사), 큐 없음

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAutosubmitVerdictRebase(t *testing.T) {
	withK(t, func(k *K) {
		k.mk("asr")
		o := k.D + "/origin.git"
		k.git(o, "update-ref", "refs/herdr/merger/box", "main")
		VD := filepath.Join(k.T, ".local/share/hc-fill/verdicts")
		queued := func(l string) string {
			return k.git(o, "for-each-ref", "--format=%(refname)", "refs/merge-queue/*-"+l, "refs/heads/lane/"+l)
		}
		vput := func(l, id string) {
			mkdirs(t, VD+"/"+l)
			sha := k.git(k.wt, "rev-parse", "HEAD")
			write(t, VD+"/"+l+"/"+id+".md", fmt.Sprintf("aid\t%s\nlane\t%s\nrun\trv-1\nsha\t%s\nverdict\tpass\nreport\t~/rep/%s.md\nround\t1\nbase\tmain\n", id, l, sha, id))
		}
		land := func(l, id string) {
			k.lndE([]string{"HC_LANE=" + l, "HC_LANE_JOB=rv-1", "HC_LANE_MGON_WAITS=0"}, k.wt, "main", "--verdict", id)
		}
		pid := func(dir, a, b string) string { // 병합기·착지가 대조하는 것과 같은 꼴(gitx.PatchID)
			o, _ := k.exe(dir, "", nil, "bash", "-c", `git --literal-pathspecs diff -U0 --binary "$1" "$2" -- | git patch-id --stable | cut -d' ' -f1`, "sh", a, b)
			return strings.TrimSpace(o)
		}

		// ① 기준만 움직임
		k.mklane("asr1")
		vput("asr1", "v1")
		k.seedp("other.txt", "남의착지")
		land("asr1", "v1")
		k.check("① 기준만 움직임: 종료 0, 큐에 들어감", k.rc == 0 && strings.Contains(queued("asr1"), "refs/merge-queue/"))
		meta := k.git(o, "for-each-ref", "--format=%(contents)", "refs/merge-queue-meta/*-asr1")
		m := regexp.MustCompile(`verdict=v1 patch=([0-9a-f]+)`).FindStringSubmatch(meta)
		// 병합기처럼 기준 위로 얹은 뒤 변경의 patch-id
		cl := filepath.Join(k.T, "asr-rebased")
		k.sh(k.T, `git clone -q "$1" "$2" && cd "$2" && git checkout -q -b x origin/lane/asr1 && git -c user.name=t -c user.email=t@t rebase -q origin/main`, o, cl)
		after := pid(cl, "origin/main", "HEAD")
		k.check("① 기준만 움직임: 제출 기록의 승인 patch-id = 기준 위로 얹은 뒤 변경의 patch-id", m != nil && after != "" && after == m[1])

		// ② 실제 충돌
		k.mklane("asr2")
		k.sh(k.wt, `printf 'lane\n' >b && git commit -qam lane-b`)
		vput("asr2", "v2")
		k.seedp("b", "origin")
		land("asr2", "v2")
		e := strings.TrimRight(k.errf(), "\n")
		k.check("② 실제 충돌: 종료 3, 큐 없음", k.rc == 3 && queued("asr2") == "")
		k.check("② 실제 충돌: 마지막 줄(대기열 넘김 까닭)에 판정·충돌 파일·--verdict 없이 다시", strings.Contains(lastLine(e), "판정 v2") &&
			strings.Contains(lastLine(e), "충돌 b") && strings.Contains(lastLine(e), "--verdict 없이"))

		// ③ 승인 뒤 커밋이 더 생김
		k.mklane("asr3")
		vput("asr3", "v3")
		k.sh(k.wt, `printf 'more\n' >more.txt && git add -- more.txt && git commit -qm more -- more.txt`)
		land("asr3", "v3")
		k.check("③ 승인 뒤 커밋: 종료 2(새 감사), 큐 없음", k.rc == 2 && queued("asr3") == "" && strings.Contains(k.errf(), "승인 커밋"))
	})
}
