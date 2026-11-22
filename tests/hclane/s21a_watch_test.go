package hclane_test

// tests/hc-lane.test.sh 21절 fetch(로그인 셸 꼴)와 watch 앞부분(2008–2115줄): 새 사건을 지휘에 hc send, 커서를 wait와 함께,
// 남은 잠금 넘겨받기, 지휘가 사라지면 끝, 원격 연결 실패, 레인을 띄우면 지휘의 watch를 저절로.
// 이 절부터 XDG_STATE_HOME=$T/wst·HC_FAKE_LS·HC_LANE_WAIT_TICK=1을 export한다(셸 그대로 — 뒤 절이 물려받는다).

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func init() { reg(102, "s21a3", s21aWatch) }

// s21aWD: 셸 $WD(watch 상태 폴더).
func (k *K) s21aWD() string { return k.T + "/wst/hc-lane" }

// s21aSends: 셸 sends — $HCLOG.send의 줄 수.
func (k *K) s21aSends() int { return s21aCountRe(read(k.HCLOG+".send"), `.`) }

// s21aRep: `HC_LANE=<레인> "$LANE" report … >/dev/null`.
func (k *K) s21aRep(lane string, args ...string) {
	k.exe(k.T, "", []string{"HC_LANE=" + lane}, k.LANE, append([]string{"report"}, args...)...)
}

// s21aW: `VAR=값 "$LANE" watch … >"$T/wo" 2>"$T/we" &`.
func (k *K) s21aW(extra []string, args ...string) *Bg {
	return k.bg(k.T, k.T+"/wo", k.T+"/we", extra, k.LANE, append([]string{"watch"}, args...)...)
}

// s21aUTC: date -u +%Y-%m-%dT%H:%M:%SZ.
func s21aUTC() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

func s21aWatch(k *K) {
	// fetch: 그 기계 로그인 셸(zsh·~/.zshrc) 꼴로 git fetch
	k.mk("t18f")
	R, T, D, W := k.R, k.T, k.D, k.W
	old := k.git(R, "rev-parse", "origin/main")
	k.run("fetch", "--on", "box", "--repo", R, "main")
	k.check("fetch --on: 종료 0, origin/main이 새로 받혀짐", k.rc == 0 && k.git(R, "rev-parse", "origin/main") == k.git(D+"/origin.git", "rev-parse", "main") &&
		old != k.git(R, "rev-parse", "origin/main"))
	ex := k.hclog(".exec")
	k.check("fetch --on: hc exec --on box, ~/.zshrc를 읽는 셸", s21aPairIn(ex, "--on", "box") && strings.Contains(read(k.HCLOG+".exec"), "zshrc") && contains(ex, "fetch"))
	k.run("fetch", "--repo", R, "refs/heads/main:refs/remotes/origin/fetched")
	k.check("fetch: --on 없으면 그 자리에서, 참조 그대로", k.rc == 0 && k.gitOK(R, "rev-parse", "-q", "--verify", "refs/remotes/origin/fetched"))
	k.run("fetch", "--on", "box", "--repo", W+"/없는 저장소")
	k.check("fetch: 실패면 종료 1", k.rc == 1)
	k.run("fetch", "--on", "box")
	k.check("fetch: --repo 없으면 사용법", k.rc == 2)
	k.run("fetch", "--repo", R, "main", "--upload-pack=touch "+T+"/pwned; git-upload-pack")
	k.check("fetch: 둘째 참조부터도 - 로 시작하면 거절, 아무것도 안 돎(감사 [중간] 2)", k.rc != 0 && !exists(T+"/pwned") && strings.Contains(k.errf(), "upload-pack"))

	// watch(lane-push): 늘 도는 감시가 wait와 같은 판정으로 새 사건을 지휘 터미널에 hc send로 밀어 준다. 커서는 wait와 같은 지휘별 커서
	k.set("XDG_STATE_HOME="+T+"/wst", `HC_FAKE_LS={"terminals":[]}`, "HC_LANE_WAIT_TICK=1")
	k.unset("HC_FAKE_MACHINES")
	WD := k.s21aWD()
	write(k.t, WD+"/coord.t7cc", "t7cc\t4242\tt7cc\t0\n") // 지휘마다 지휘 정보(주소·pid·이름·마지막 레인 띄운 시각)
	os.Remove(k.HCLOG + ".send")
	os.Remove(k.HCLOG + ".sendfail")
	live := []string{"HC_FAKE_LIVE=t7cc"}
	send := func() string { return read(k.HCLOG + ".send") }
	w1 := k.s21aW(live, "--to", "t7cc")
	upto(func() bool { return isDir(WD + "/watch.t7cc") })
	k.s21aRep("lane-w", "완료", "abcd1234", "끝냄")
	upto(func() bool { return k.s21aSends() == 1 })
	k.check("watch: 새 사건은 지휘 터미널에 hc send 한 번(한 줄)", k.s21aSends() == 1 && s21aCut(send(), 1, 1) == "t7cc" && strings.Contains(send(), "lane-w 완료 abcd1234 끝냄"))
	time.Sleep(2500 * time.Millisecond)
	k.check("watch: 같은 사건은 다시 안 보냄", k.s21aSends() == 1)
	k.run("watch", "--to", "t7cc")
	k.check("watch: 같은 지휘 주소로 두 번째는 종료 75", k.rc == 75 && !w1.Done())
	k.runE([]string{"HC_TERMINAL_ID=t7cc"}, "wait", "--timeout", "0")
	k.check("watch: wait와 커서를 함께 씀(watch가 보낸 사건을 wait가 다시 안 알림)", k.rc == 75)
	write(k.t, k.HCLOG+".sendfail", "")
	k.s21aRep("lane-w", "막힘", "-", "막혔음")
	time.Sleep(2500 * time.Millisecond)
	k.check("watch: hc send가 실패하면 커서에 안 적음", k.s21aSends() == 1 && !strings.Contains(read(WD+"/seen.t7cc"), "막혔음"))
	os.Remove(k.HCLOG + ".sendfail")
	upto(func() bool { return k.s21aSends() == 2 })
	k.check("watch: 다음 바퀴에 다시 보냄(한 번)", k.s21aSends() == 2 && strings.Contains(send(), "lane-w 막힘 막혔음"))
	w1.stop()
	// 남은 프로세스는 이 시험의 출력 파일 $T/wo를 쥔 것으로 본다(끈 watch의 자식도 $T/wo를 물려받아 끝나기까지 잠깐 걸린다 — 8초까지)
	wgone := func() bool {
		o, _ := k.exe(T, "", nil, "lsof", "-t", T+"/wo")
		return w1.Done() && !exists(WD+"/watch.t7cc") && o == ""
	}
	upto(wgone)
	k.check("watch: 끄면 잠금을 지우고 프로세스를 안 남김", wgone())
	k.s21aRep("lane-w", "중간", "-", "반쯤")
	k.runE([]string{"HC_TERMINAL_ID=t7cc"}, "wait", "--timeout", "0")
	k.check("watch 꺼진 사이 사건은 wait가 받음", k.rc == 0 && strings.Contains(k.out, "반쯤"))
	// 남은 잠금(pid가 살아 있지만 hc-lane watch가 아님) — 호스트 PID 1의 정보에 기대지 않게 이 시험이 띄운 다른 명령(R-36)
	other := exec.Command("sleep", "120")
	if err := other.Start(); err != nil {
		k.t.Fatal(err)
	}
	defer func() { other.Process.Kill(); other.Wait() }()
	write(k.t, WD+"/watch.t7cc/pid", strconv.Itoa(other.Process.Pid)+"\n")
	w2 := k.s21aW(live, "--to", "t7cc")
	k.s21aRep("lane-w", "착지", "abcd1234", "끝")
	upto(func() bool { return k.s21aSends() == 3 })
	k.check("watch: 남은 잠금은 넘겨받음, wait가 받은 사건은 안 보냄", !w2.Done() && k.s21aSends() == 3 &&
		strings.Contains(lastLine(strings.TrimRight(send(), "\n")), "lane-w 착지 abcd1234") && !strings.Contains(send(), "반쯤"))
	w2.stop()
	// 지휘가 사라지면(hc get 실패 3번) 스스로 끝남
	w3 := k.s21aW(nil, "--to", "t7cc")
	upto(w3.Done)
	wrc := w3.Wait()
	k.check("watch: 지휘 터미널이 사라지면 스스로 종료 0, 잠금 지움", wrc == 0 && strings.Contains(read(T+"/we"), "사라짐") && !exists(WD+"/watch.t7cc"))
	// 원격 기계 연결 실패: 「연결 안 됨」 한 번, 다음 바퀴에 다시 보고 붙으면 그 사건
	MH := T + "/mh"
	k.set(`HC_FAKE_MACHINES={"machines":[{"name":"m9","state":"connected"},{"name":"m8","state":"connected"}]}`, "HC_FAKE_MACH_HOME="+MH)
	os.RemoveAll(MH + "/m9")
	write(k.t, MH+"/m8/.nosh", "") // m8: sh 없는 기계(lane-nosh) — 「연결 안 됨」을 보내지 않는다
	w4 := k.s21aW(live, "--all", "--to", "t7cc")
	upto(func() bool { return strings.Contains(send(), "연결 안 됨 m9") })
	time.Sleep(2500 * time.Millisecond)
	k.check("watch: 원격 연결 실패는 「연결 안 됨」 한 번만", s21aCountF(send(), "연결 안 됨 m9") == 1)
	k.check("watch: sh 없는 기계는 「연결 안 됨」·「모름」 없음(lane-nosh)", !strings.Contains(send(), "m8") && !strings.Contains(read(T+"/we"), "모름 m8"))
	write(k.t, MH+"/m9/.local/state/hc-lane/events.tsv", ev(s21aUTC(), "h9", "t3", "lane-r", "완료", "eeee9999", "원격")+
		ev(s21aUTC(), "h9", "-", "hc-disk-low", "중간", "-", "여유적음"))
	upto(func() bool {
		return strings.Contains(send(), "lane-r 완료 eeee9999") && strings.Contains(send(), "hc-disk-low")
	})
	k.check("watch: 다음 바퀴에 다시 붙으면 그 기계 사건을 보냄", strings.Contains(send(), "lane-r 완료 eeee9999 원격 (m9/t3)"))
	// 터미널 밖 사건(hc-sweep의 hc-disk-low 등 — 주소 -)은 끝 괄호에 그 기계 hc 이름
	k.check("watch: 터미널 없는 원격 사건은 끝 괄호에 기계 hc 이름", strings.Contains(send(), "hc-disk-low 중간 여유적음 (m9)"))
	w4.stop()
	k.unset("HC_FAKE_MACHINES", "HC_FAKE_MACH_HOME")
	// 지휘가 손으로 걸 일 없게: 레인·감사를 띄우면 그 지휘($HC_TERMINAL_ID)의 watch가 없을 때 저절로 하나
	k.mk("t21")
	R = k.R
	os.Remove(WD + "/coord.t7cc")
	k.runE([]string{"HC_FAKE_LIVE=t7cc", "HC_TERMINAL_ID=t7cc"}, "--direct", "--name", "lane-a1", "--repo", R, "--", "claude") // 지휘가 직접(lane-queue-only 뒤 --direct)
	co := read(WD + "/coord.t7cc")
	f4, err := strconv.Atoi(s21aCut(co, 4, 4))
	k.check("지휘 터미널에서 레인을 띄우면 지휘 정보(주소·pid·이름·띄운 시각)를 coord.<주소>에 남기고, 레인 기록에 그 지휘", s21aCut(co, 1, 3) == "t7cc\t4242\tt7cc" &&
		err == nil && f4 > 0 && s21aLine(read(WD+"/lanes.tsv"), "t9zz\tlane-a1\tt7cc\tdirect"))
	k.check("레인을 띄우면 지휘의 watch를 hc run --bg로 띄움(--all --to 지휘, 라벨 hc-lane-watch=1, --no-notify)", k.rc == 0 && k.pair("--name", "lane-watch-t7cc") &&
		k.has("watch") && k.has("--all") && k.pair("--to", "t7cc") && k.pair("--label", "hc-lane-watch=1") && k.has("--no-notify") && strings.Contains(k.out, "감시 lane-watch-t7cc 띄움"))
	lb := 0 // grep -A1 -Fx -- --label "$HCLOG" | grep -cFx hc-lane=1
	hl := k.hclog("")
	for i := 1; i < len(hl); i++ {
		if hl[i] == "hc-lane=1" && hl[i-1] == "--label" {
			lb++
		}
	}
	k.check("watch 터미널은 레인 라벨(hc-lane=1)이 아님", !contains(hl, "--label=hc-lane=1") && lb == 1)
	k.check("레인 JSON 끝 줄은 그대로 레인 주소", strings.Contains(lastLine(k.out), `"lane":"t9zz"`))
	w5 := k.s21aW(live, "--to", "t7cc")
	upto(func() bool { return strings.TrimSpace(read(WD+"/watch.t7cc/pid")) != "" })
	os.Remove(k.HCLOG)
	k.runE([]string{"HC_TERMINAL_ID=t7cc"}, "--read", "--name", "lane-a2", "--repo", R, "--", "claude")
	k.check("둘째 레인(--read 포함)은 watch가 돌고 있으면 안 띄움", k.rc == 0 && !k.has("lane-watch-t7cc") && k.runs() == 1)
	w5.stop()
	os.Remove(k.HCLOG)
	k.run("--name", "lane-a3", "--repo", R, "--", "claude")
	k.check("지휘가 없으면($HC_TERMINAL_ID 없음 — hc-fill 등) watch를 안 띄움", k.rc == 0 && !k.has("watch") && k.runs() == 1)
	os.Remove(k.HCLOG)
	k.runE([]string{"HC_FAKE_LIVE=t7cc", "HC_TERMINAL_ID=t7cc"}, "--dry-run", "--name", "lane-a4", "--repo", R, "--", "claude")
	k.check("--dry-run은 watch도 띄우지 않고 할 일만", k.rc == 0 && strings.Contains(k.out, "[dry-run] hc run --bg --no-notify --rm --name lane-watch-t7cc") && !exists(k.HCLOG))
}
