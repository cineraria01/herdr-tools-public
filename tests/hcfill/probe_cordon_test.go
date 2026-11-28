package hcfill_test

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ht-probe-cordon 계약(docs/hc-fill/cordon.md 「점검 항목」): 진짜 hc-fill 바이너리의 run 바퀴가 box-1의 항목마다(원격 쓰기·에이전트 풀·
// 키체인·임시 폴더) 문턱 뒤 격리하고(새 레인은 다른 기계로, 사건 한 번, machine·advise에 보임), 돌아오면 다음 바퀴에 스스로 푼다.
// 시계는 알리기만. 가짜 hc-lane은 $T_LOG/probe.<기계>(점검 줄)·remote-write.<기계>(쓰기 자격 줄)를 싣는다
func TestKitProbeItems(t *testing.T) {
	type item struct{ key, file, bad, look, adv string }
	items := []item{
		{"-write", "remote-write.box-1", "remote-write 1 128 git.example.test error: unable to read askpass response from '/usr/bin/false' | fatal: could not read Username for 'https://git.example.test': terminal prompts disabled",
			"원격 쓰기 거절", "원격 쓰기 거절"},
		{"-pool", "probe.box-1", "probe pool bad 쓸 계정 0", "에이전트 풀 빔", "에이전트 풀에 쓸 계정 없음"},
		{"-keychain", "probe.box-1", "probe keychain bad rc 36 User interaction is not allowed", "키체인 막힘", "키체인 막힘"},
		{"-tmp", "probe.box-1", "probe tmp bad /home/me/.hctmp.noindex 쓰기가 5s 안에 안 끝남", "임시 폴더 못 씀", "작업 임시 폴더를 못 씀"},
	}
	for _, it := range items {
		t.Run(it.key[1:], func(t *testing.T) {
			k := newK(t)
			k.fresh()
			k.set("HC_FAKE_SPEC_IDLE=load=0.10 cpus=8 verifies=0 gates=0")
			write(t, k.T+"/conf/config", "worker.proxy fake-pool\n")
			for _, m := range []string{"box-1", "box-2", "shared", "slow"} {
				write(t, k.T+"/repo-read."+m, "repo-read 1 ok\nrepo-read end 1\n")
			}
			write(t, k.T+"/repo-read.here", "repo-read 1 ok\nrepo-read 2 ok\nrepo-read end 2\n")
			write(t, k.T+"/"+it.file, it.bad+"\n")
			cf := k.DATA + "/cordon" + it.key + ".tsv"
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
			k.check("첫 바퀴는 격리 안 함(처음 봄부터 센다)", strings.Contains(read(cf), "box-1\tbad") && !strings.Contains(read(cf), "\t1\t0\t"), read(cf))
			back(6 * 60)
			k.add("l2")
			k.run("run")
			k.check("격리 사건 한 번", k.nrep("report 막힘") == 1 && k.nrep("--key hf:cordon"+it.key+":box-1:") >= 1, read(k.T+"/report.log"))
			k.check("l2는 box-1에 안 뜸", !strings.Contains(k.where("l2"), "box-1"), k.where("l2"))
			k.run("machine")
			t.Logf("hc-fill machine(%s 격리 중):\n%s", it.look, k.out)
			k.check("machine 줄 끝 까닭", strings.Contains(k.out, "격리 6m째("+it.look+" — "), k.out)
			k.run("advise")
			t.Logf("hc-fill advise(%s 격리 중):\n%s", it.look, k.out)
			k.check("advise 항목", strings.Contains(k.out, "box-1 격리 6분째 — "+it.adv), k.out)
			k.run("run")
			k.check("되풀이 없음", k.nrep("report 막힘") == 1, read(k.T+"/report.log"))
			good := map[string]string{"-write": "remote-write 1 0 git.example.test -", "-pool": "probe pool ok 쓸 계정 2",
				"-keychain": "probe keychain ok 열림", "-tmp": "probe tmp ok /home/me/.hctmp.noindex"}[it.key] // 줄을 지우면 옛 판 = 모름(풀지 않음)
			write(t, k.T+"/"+it.file, good+"\n")
			k.run("run")
			k.check("회복 한 번", k.nrep("report 중간") == 1 && k.nrep("돌아옴") == 1, read(k.T+"/report.log"))
			k.run("machine")
			t.Logf("hc-fill machine(%s 풀린 뒤):\n%s", it.look, k.out)
			k.check("풀린 뒤 격리 없음", !strings.Contains(k.out, "격리"), k.out)
			k.run("advise")
			t.Logf("hc-fill advise(%s 풀린 뒤):\n%s", it.look, k.out)
			k.check("풀린 뒤 advise 항목 없음", !strings.Contains(k.out, "box-1 격리"), k.out)
			if it.key == "-pool" {
				k.check("풀 명령은 설정 값으로 실림", strings.Contains(read(k.T+"/pool.calls"), "box-1 fake-pool"), read(k.T+"/pool.calls"))
			}
		})
	}
}

// 시계: box-1의 date가 10분 앞서면 문턱 뒤 알리기만(격리 없음 — 새 레인은 그대로 box-1에도 간다)
func TestKitProbeClock(t *testing.T) {
	k := newK(t)
	k.fresh()
	k.set("HC_FAKE_SPEC_IDLE=load=0.10 cpus=8 verifies=0 gates=0")
	write(t, k.T+"/bin/date", "#!/bin/sh\n[ \"${HC_FAKE_ON-}\" = box-1 ] && [ -f \"$T_LOG/date.box-1\" ] && { cat \"$T_LOG/date.box-1\"; exit 0; }\nexec /bin/date \"$@\"\n")
	os.Chmod(k.T+"/bin/date", 0o755)
	write(t, k.T+"/date.box-1", fmt.Sprint(time.Now().Unix()+600)+"\n")
	for _, m := range []string{"box-1", "box-2", "shared", "slow"} {
		write(t, k.T+"/repo-read."+m, "repo-read 1 ok\nrepo-read end 1\n")
	}
	write(t, k.T+"/repo-read.here", "repo-read 1 ok\nrepo-read 2 ok\nrepo-read end 2\n")
	cf := k.DATA + "/cordon-clock.tsv"
	k.run("run")
	k.check("시계 틀림을 봄", strings.Contains(read(cf), "box-1\tbad"), read(cf))
	var o []string
	for _, l := range strings.Split(strings.TrimSpace(read(cf)), "\n") {
		f := strings.Split(l, "\t")
		for _, i := range []int{2, 3} {
			n, _ := strconv.ParseInt(f[i], 10, 64)
			f[i] = fmt.Sprint(n - 6*60)
		}
		o = append(o, strings.Join(f, "\t"))
	}
	write(t, cf, strings.Join(o, "\n")+"\n")
	k.run("run")
	k.check("알림 한 번(알리기만)", k.nrep("report 막힘") == 1 && k.nrep("알리기만") == 1 && k.nrep("--key hf:cordon-clock:box-1:") == 1, read(k.T+"/report.log"))
	k.run("machine")
	t.Logf("hc-fill machine(시계 틀림):\n%s", k.out)
	k.check("격리 없음·알리기만 표시", !strings.Contains(k.out, "격리 6m") && strings.Contains(k.out, "시계 틀림 6m째("), k.out)
}
