package hcfill_test

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ht-worker-cordon 계약(docs/hc-fill/cordon.md): 진짜 hc-fill 바이너리의 run 바퀴가 box-1의 저장소 읽기 막힘을 문턱 뒤 격리하고
// (새 레인은 다른 기계로, 사건 한 번, machine·advise에 보임), 읽기가 돌아오면 다음 바퀴에 스스로 풀고 한 번 알린다.
// 가짜 hc exec는 그 기계 이름을 HC_FAKE_ON으로, 가짜 hc-lane merger --spec-idle --repo-read는 $T_LOG/repo-read.<기계>를 찍는다
func TestKitCordon(t *testing.T) {
	k := newK(t)
	k.fresh()
	k.set("HC_FAKE_SPEC_IDLE=load=0.10 cpus=8 verifies=0 gates=0")
	good := map[string]string{"box-1": "repo-read 1 ok\nrepo-read end 1", "box-2": "repo-read 1 ok\nrepo-read end 1",
		"shared": "repo-read 1 ok\nrepo-read end 1", "slow": "repo-read 1 ok\nrepo-read end 1", "here": "repo-read 1 ok\nrepo-read 2 ok\nrepo-read end 2"}
	for m, s := range good {
		write(t, k.T+"/repo-read."+m, s+"\n")
	}
	write(t, k.T+"/repo-read.box-1", "repo-read 1 bad open /home/me/src/app: interrupted system call\nrepo-read end 1\n")
	cf := k.DATA + "/cordon.tsv"
	back := func(sec int64) { // 기록의 때를 당긴다(문턱을 기다리지 않게)
		var o []string
		for _, l := range strings.Split(strings.TrimSpace(read(cf)), "\n") {
			f := strings.Split(l, "\t")
			for _, i := range []int{2, 3} {
				n, _ := strconv.ParseInt(f[i], 10, 64)
				f[i] = fmt.Sprint(n - sec)
			}
			o = append(o, strings.Join(f, "\t"))
		}
		write(t, cf, strings.Join(o, "\n")+"\n")
	}

	k.add("l1")
	k.run("run")
	t.Logf("바퀴 1(처음 본 나쁨):\n%s", k.out)
	k.check("첫 바퀴는 격리 안 함(문턱부터 센다)", !strings.Contains(read(cf), "\t1\t0\t"), read(cf))
	l1 := k.where("l1")
	back(6 * 60)
	k.add("l2")
	k.run("run")
	t.Logf("바퀴 2(6분 뒤):\n%s", k.out)
	k.check("격리 사건 한 번", k.nrep("--name hc-fill-cordon") == 1 && k.nrep("report 막힘") == 1, read(k.T+"/report.log"))
	k.check("l2는 box-1에 안 뜸", !strings.Contains(k.where("l2"), "box-1"), k.where("l2"))
	k.check("떠 있는 l1은 그대로", k.where("l1") == l1, k.where("l1"))
	k.run("machine")
	t.Logf("hc-fill machine(격리 중):\n%s", k.out)
	k.check("machine 줄 끝 격리", strings.Contains(k.out, "격리 6m째(app /home/me/src/app: open /home/me/src/app: interrupted system call)"))
	k.run("advise")
	t.Logf("hc-fill advise(격리 중):\n%s", k.out)
	k.check("advise 항목", strings.Contains(k.out, "box-1 격리 6분째") && strings.Contains(k.out, "사람이 정한다"))
	k.run("run")
	k.check("같은 격리 동안 되풀이 없음", k.nrep("report 막힘") == 1, read(k.T+"/report.log"))

	write(t, k.T+"/repo-read.box-1", good["box-1"]+"\n")
	k.run("run")
	t.Logf("바퀴(읽기 돌아옴):\n%s", k.out)
	k.check("회복 한 번", k.nrep("저장소 읽기 돌아옴") == 1, read(k.T+"/report.log"))
	k.run("machine")
	t.Logf("hc-fill machine(풀린 뒤):\n%s", k.out)
	k.check("풀린 뒤 machine에 격리 없음", !strings.Contains(k.out, "격리"))
	time.Sleep(10 * time.Millisecond)
	if b, err := os.ReadFile(k.T + "/report.log"); err == nil {
		t.Logf("report.log:\n%s", b)
	}
}

// ht-cordon-remote-auth 계약: box-1의 원격 로그인이 거절되면(저장소 읽기는 좋음) 같은 길로 문턱 뒤 격리, 돌아오면 스스로 풂.
// 가짜 hc-lane merger --spec-idle --remote-auth는 $T_LOG/remote-auth.<기계>(없으면 좋음)를 찍고 부를 때마다 remote-auth.calls에 한 줄
func TestKitCordonAuth(t *testing.T) {
	k := newK(t)
	k.fresh()
	k.set("HC_FAKE_SPEC_IDLE=load=0.10 cpus=8 verifies=0 gates=0")
	for _, m := range []string{"box-1", "box-2", "shared", "slow"} {
		write(t, k.T+"/repo-read."+m, "repo-read 1 ok\nrepo-read end 1\n")
	}
	write(t, k.T+"/repo-read.here", "repo-read 1 ok\nrepo-read 2 ok\nrepo-read end 2\n")
	write(t, k.T+"/remote-auth.box-1", "remote-auth 1 128 git.example.test remote: Invalid username or token. Password authentication is not supported for Git operations. | fatal: Authentication failed for 'https://x-access-token:ghs_"+strings.Repeat("Zq9", 12)+"@git.example.test/o/r.git/'\n")
	cf := k.DATA + "/cordon-auth.tsv"
	back := func(sec int64) {
		var o []string
		for _, l := range strings.Split(strings.TrimSpace(read(cf)), "\n") {
			f := strings.Split(l, "\t")
			for _, i := range []int{2, 3} {
				n, _ := strconv.ParseInt(f[i], 10, 64)
				f[i] = fmt.Sprint(n - sec)
			}
			o = append(o, strings.Join(f, "\t"))
		}
		write(t, cf, strings.Join(o, "\n")+"\n")
	}
	k.add("l1")
	k.run("run")
	k.check("첫 바퀴는 격리 안 함", strings.Contains(read(cf), "box-1\tbad") && !strings.Contains(read(cf), "\t1\t0\t"), read(cf))
	back(6 * 60)
	k.add("l2")
	k.run("run")
	t.Logf("바퀴 2(6분 뒤):\n%s", k.out)
	k.check("격리 사건 한 번(원격 까닭)", k.nrep("report 막힘") == 1 && k.nrep("원격 로그인 거절") >= 1 && k.nrep("gh auth status") >= 1, read(k.T+"/report.log"))
	k.check("l2는 box-1에 안 뜸", !strings.Contains(k.where("l2"), "box-1"), k.where("l2"))
	k.check("자격·토큰이 기록·알림에 없음", !strings.Contains(read(cf)+read(k.T+"/report.log"), "ghs_") && !strings.Contains(read(cf), "x-access-token"), read(cf))
	k.run("machine")
	t.Logf("hc-fill machine(격리 중):\n%s", k.out)
	k.check("machine 줄 끝 원격 까닭", strings.Contains(k.out, "격리 6m째(원격 로그인 거절 — app git.example.test: "), k.out)
	k.run("advise")
	t.Logf("hc-fill advise(격리 중):\n%s", k.out)
	k.check("advise 원격 까닭", strings.Contains(k.out, "box-1 격리 6분째 — 원격 로그인 거절"), k.out)
	k.run("run")
	k.check("되풀이 없음", k.nrep("report 막힘") == 1, read(k.T+"/report.log"))
	os.Remove(k.T + "/remote-auth.box-1")
	k.run("run")
	t.Logf("바퀴(로그인 돌아옴):\n%s", k.out)
	k.check("회복 한 번", k.nrep("원격 로그인 돌아옴") == 1, read(k.T+"/report.log"))
	k.run("machine")
	t.Logf("hc-fill machine(풀린 뒤):\n%s", k.out)
	k.check("풀린 뒤 격리 없음", !strings.Contains(k.out, "격리"), k.out)
	calls := strings.Count(read(k.T+"/remote-auth.calls"), "box-2")
	k.run("run")
	k.check("좋은 기계는 간격 안에 다시 안 부름", strings.Count(read(k.T+"/remote-auth.calls"), "box-2") == calls, read(k.T+"/remote-auth.calls"))
}
