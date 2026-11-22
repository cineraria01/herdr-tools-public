package hclane_test

// gofix2-lane(2026-11-22 Go 전환 재감사 — 아스트라 6.2 max 1바퀴, 감사 HEAD 0f809e3): 지적 ID마다 고치기 전 Go 판에서 실패하던 시험.
// 기대값은 지적 원문과 계약으로 정했다(셸 판은 go-shell-remove가 걷었다).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGofix2(t *testing.T) {
	withK(t, func(k *K) {
		for _, c := range []struct {
			name string
			f    func(*K)
		}{
			{"R05", g2KeepUTF8}, {"R06", g2LsShape}, {"R07", g2LsMachine}, {"R08", g2ReflogGarbled},
			{"R09", g2GinEmpty}, {"R10", g2RecheckRC}, {"R13", g2CoordUnknown}, {"R11", g2QaEnum}, {"R12", g2QaDirRace},
			{"R14", g2SendShape}, {"R17R18", g2RunReply}, {"R20", g2WatchPsEmpty}, {"R25", g2QaLongName}, {"R26", g2RebaseLock},
			{"R13w", g2WatchGarbledGone}, {"R15", g2ResumeLegacy}, {"R16", g2PrecheckLookup}, {"R24", g2LegacyParkRemote}, {"R35", g2BgGrandchild},
		} {
			k.t.Run(c.name, func(t *testing.T) {
				pt := k.t
				k.t = t
				defer func() { k.t = pt }()
				c.f(k)
			})
		}
	})
}

// g2Git: 가짜 git(PATH 맨 앞) — body는 bash 조각, 끝에서 진짜 git으로 넘긴다. $real이 진짜 git
func (k *K) g2Git(body string) string {
	return k.gfBin(map[string]string{"git": "real=" + sq(k.gfRealGit()) + "\n" + body + "\nexec \"$real\" \"$@\""})
}

// R-05: UTF-8 로캘에서 [[:alpha:]]* 는 셸 case처럼 한글 가지도 맞는다 — 로컬·원격 모두 남긴다
func g2KeepUTF8(k *K) {
	k.mk("g205")
	k.sh(k.R, `git pull -q --ff-only && git branch 보관 && git push -q origin main:refs/heads/보관`)
	k.conf("gc.keep_branches [[:alpha:]]*")
	k.runE([]string{"LC_ALL=en_US.UTF-8"}, "gc", "--repo", k.R, "--dry-run", "--min-age", "0", "--remote-branches")
	k.check("R-05 gc: [[:alpha:]]* 는 한글 가지 「보관」을 남긴다(로컬)", s19lineHas(k, "가지 보관", "남길 목록") && !s19lineHas(k, "가지 보관", "지움"))
	k.check("R-05 gc --remote-branches: 원격 refs/heads/보관 도 남긴다", s19lineHas(k, "refs/heads/보관", "남길 목록") && !s19lineHas(k, "refs/heads/보관", "지움"))
	k.noconf()
}

// R-06: 형식만 맞는 JSON({}·terminals:null·주소만 있는 항목)은 「터미널 없음」이 아니다 — 지우지 않고 멈춘다(gc·clean)
func g2LsShape(k *K) {
	k.mk("g206")
	k.land("g206l")
	os.Remove(k.wt + "/.omc/state/s.json")
	live := fmt.Sprintf(`{"terminals":[{"address":"t5","agent":"claude-code","cwd":"%s","state":"live"}]}`, k.wt)
	k.runE([]string{"HC_FAKE_LS=" + live}, "gc", "--repo", k.R, "--dry-run", "--min-age", "0")
	k.check("R-06 준비: 정상 목록이면 살아 있는 터미널로 남김", k.rc == 0 && s19lineHas(k, k.wt, "살아 있는 터미널"))
	k.runE([]string{`HC_FAKE_LS={"terminals":[]}`}, "gc", "--repo", k.R, "--dry-run", "--min-age", "0")
	k.check("R-06 준비: 올바른 빈 목록이면 지울 후보", k.rc == 0 && s19lineHas(k, k.wt, "지움"))
	for _, ls := range []string{`{}`, `{"terminals":null}`, `{"terminals":[{"address":"t5"}]}`, `{"terminals":[{"address":"t5","cwd":null}]}`} {
		k.runE([]string{"HC_FAKE_LS=" + ls}, "gc", "--repo", k.R, "--dry-run", "--min-age", "0")
		k.check("R-06 gc: "+ls+" 이면 종료 1, 아무것도 안 고름", k.rc == 1 && !strings.Contains(k.out, "지움 /"))
		k.runE([]string{"HC_FAKE_LS=" + ls}, "clean", "--repo", k.R, "--branch", "g206l", "--dry-run")
		k.check("R-06 clean: "+ls+" 이면 멈추고 작업 트리를 남긴다", k.rc != 0 && k.kept("g206l") && !strings.Contains(k.out, "정리됨"))
	}
}

// R-07: --on box인데 목록에 box가 없거나 끊겼으면 box의 터미널을 모른다 — 「아무도 안 씀」으로 지우지 않는다
func g2LsMachine(k *K) {
	k.mk("g207")
	k.land("g207l")
	os.Remove(k.wt + "/.omc/state/s.json")
	ok := `{"machines":[{"name":"box","state":"connected"}],"terminals":[]}`
	k.runE([]string{"HC_FAKE_LS=" + ok}, "gc", "--on", "box", "--repo", k.R, "--dry-run", "--min-age", "0")
	k.check("R-07 준비: box가 연결돼 있고 터미널이 없으면 지울 후보", k.rc == 0 && s19lineHas(k, k.wt, "지움"))
	busy := fmt.Sprintf(`{"machines":[{"name":"box","state":"connected"}],"terminals":[{"address":"box/t8","agent":"claude-code","cwd":"%s","state":"live"}]}`, k.wt)
	k.runE([]string{"HC_FAKE_LS=" + busy}, "gc", "--on", "box", "--repo", k.R, "--dry-run", "--min-age", "0")
	k.check("R-07 준비: box/t8 이 쓰면 남김", k.rc == 0 && s19lineHas(k, k.wt, "살아 있는 터미널 box/t8"))
	for _, ls := range []string{`{"machines":[{"name":"box","state":"disconnected"}],"terminals":[]}`, `{"machines":[],"terminals":[]}`, `{"terminals":[]}`} {
		k.runE([]string{"HC_FAKE_LS=" + ls}, "gc", "--on", "box", "--repo", k.R, "--dry-run", "--min-age", "0")
		k.check("R-07 gc --on box: "+ls+" 이면 종료 1, 아무것도 안 고름", k.rc == 1 && !strings.Contains(k.out, "지움 /"))
	}
	k.runE([]string{"HC_FAKE_LS=" + `{"machines":[{"name":"box","state":"disconnected"}],"terminals":[]}`}, "clean", "--on", "box", "--repo", k.R, "--branch", "g207l", "--dry-run")
	k.check("R-07 clean --on box: box가 끊겼으면 멈추고 남긴다", k.rc != 0 && k.kept("g207l"))
}

// R-08: reflog가 0으로 끝났는데 꼴이 깨짐(recent@{) — 새 가지의 나이를 모르니 남기고 종료 1
func g2ReflogGarbled(k *K) {
	k.mk("g208")
	k.sh(k.R, `git pull -q --ff-only && GIT_COMMITTER_DATE='2020-01-01T00:00:00Z' GIT_AUTHOR_DATE='2020-01-01T00:00:00Z' git commit -q --allow-empty -m old && git push -q origin HEAD:main && git branch recent`)
	p := k.g2Git(`for a; do [ "$a" = reflog ] && { echo 'recent@{'; exit 0; }; done`)
	k.runE([]string{p}, "gc", "--repo", k.R, "--dry-run")
	k.check("R-08 gc: 깨진 reflog 출력이면 가지를 남기고 종료 1", k.rc == 1 && !s19lineHas(k, "recent", "지움") && strings.Contains(k.out, "지움 0"))
	p = k.g2Git(`for a; do [ "$a" = reflog ] && exit 0; done`)
	k.runE([]string{p}, "gc", "--repo", k.R, "--dry-run")
	k.check("R-08 gc: 빈 reflog(기록 없음)는 커밋 시각으로 판정 — 오류 아님", k.rc == 0 && s19lineHas(k, "recent", "지움"))
}

// R-09: 비조상 가지인데 고유 커밋 열거가 0·빈 출력 — 「모두 들어감」이 아니라 확인 못 함(남김, 종료 1)
func g2GinEmpty(k *K) {
	k.mk("g209")
	k.sh(k.R, `git pull -q --ff-only && git checkout -q -b nonanc && GIT_COMMITTER_DATE='2020-01-01T00:00:00Z' GIT_AUTHOR_DATE='2020-01-01T00:00:00Z' git commit -q --allow-empty -m na && git checkout -q main`)
	k.run("gc", "--repo", k.R, "--dry-run", "--min-age", "0")
	k.check("R-09 준비: 정상이면 「안 들어감 1개」로 남김", k.rc == 0 && s19lineHas(k, "nonanc", "안 들어감 1개"))
	p := k.g2Git(`[ "$1" = rev-list ] && [ "$2" = --no-merges ] && exit 0`)
	k.runE([]string{p}, "gc", "--repo", k.R, "--dry-run", "--min-age", "0")
	k.check("R-09 gc: 비조상인데 고유 커밋이 빈 목록이면 남기고 종료 1", k.rc == 1 && !s19lineHas(k, "nonanc", "지움"))
	p = k.g2Git(`[ "$1" = log ] && [ "$2" = -p ] && exit 1`)
	k.runE([]string{p}, "gc", "--repo", k.R, "--dry-run", "--min-age", "0")
	k.check("R-09 gc: 패치 비교(git log -p)가 실패하면 남기고 종료 1", k.rc == 1 && !s19lineHas(k, "nonanc", "지움"))
}

// R-10: 지우기 직전 재조회가 SHA를 내고도 128 — 지우지 않는다(git branch -D를 부르지 않음)
func g2RecheckRC(k *K) {
	k.mk("g210")
	k.sh(k.R, `git pull -q --ff-only && git branch r10`)
	del := filepath.Join(k.T, "r10-del")
	os.Remove(del)
	p := k.g2Git(`case " $* " in
*" rev-parse -q --verify refs/heads/r10 "*) "$real" "$@"; exit 128 ;;
*" branch -q -D -- r10 "*) echo x >>` + sq(del) + `; exit 1 ;;
esac`)
	k.runE([]string{p}, "gc", "--repo", k.R, "--min-age", "0")
	k.check("R-10 gc: 재조회가 128이면 git branch -D를 부르지 않고 남김(종료 1)", !exists(del) && k.rc == 1 && !s19lineHas(k, "가지 r10", "지움") &&
		k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/r10"))
}

// R-13: 잘린 not_found(3)·상태 없는 주소 객체는 「없음」이 아니라 모름(2). 기본 선택은 확인 못 한 후보가 있으면 2
func g2CoordUnknown(k *K) {
	full := k.gfBin(map[string]string{"hc": `[ "$1" = get ] && { echo '{"address":"t1","cwd":"/","name":"boss","pid":4242,"state":"live"}'; exit 0; }; exit 3`})
	k.runE([]string{full}, "coord", "--set", "t1")
	k.check("R-13 준비: coord --set t1", k.rc == 0)
	cut := k.gfBin(map[string]string{"hc": `[ "$1" = get ] && { printf '{"error":{"kind":"not_found"' >&2; exit 3; }; exit 3`})
	k.runE([]string{cut}, "coord", "--check", "t1")
	k.check("R-13 coord --check: 잘린 not_found(3)는 모름(2)", k.rc == 2)
	nost := k.gfBin(map[string]string{"hc": `[ "$1" = get ] && { echo '{"address":"t1"}'; exit 0; }; exit 3`})
	k.runE([]string{nost}, "coord", "--check", "t1")
	k.check("R-13 coord --check: state 없는 주소 객체는 모름(2)", k.rc == 2)
	fin := k.gfBin(map[string]string{"hc": `[ "$1" = get ] && { echo '{"address":"t1","state":"finished"}'; exit 0; }; exit 3`})
	k.runE([]string{fin}, "coord", "--check", "t1")
	k.check("R-13 coord --check: 확인된 finished는 1", k.rc == 1)
	bad := k.gfBin(map[string]string{"hc": `[ "$1" = get ] && { echo 'error: cannot reach the core' >&2; exit 1; }; exit 3`})
	k.runE([]string{bad}, "coord")
	k.check("R-13 coord(기본 선택): 하나뿐인 등록을 조회 못 하면 2(없음 1이 아님)", k.rc == 2)
	k.runE([]string{nost}, "coord")
	k.check("R-13 coord(기본 선택): 상태 없는 응답뿐이면 2", k.rc == 2)
}

// R-11: .qa 폴더는 찾았는데 그 안 파일 열거(find)가 실패 — 보관을 확정하지 못했으니 작업 트리를 지우지 않는다
func g2QaEnum(k *K) {
	k.mk("g211")
	appendf(k.t, k.R+"/.git/info/exclude", ".qa/\n")
	k.land("g211l")
	os.Remove(k.wt + "/.omc/state/s.json")
	write(k.t, k.wt+"/.qa/r/REPORT.md", "r\n")
	p := k.gfBin(map[string]string{"find": `[ "$1" = ./.qa ] && exit 1; exec /usr/bin/find "$@"`})
	k.runE([]string{p}, "clean", "--repo", k.R, "--branch", "g211l")
	k.check("R-11 clean: .qa 안 열거 실패면 멈추고 원본을 남긴다", k.rc != 0 && k.kept("g211l") && s13cat(k.wt+"/.qa/r/REPORT.md") == "r")
}

// R-12: 넣는 순간 다른 레인이 목적지 자리에 폴더를 만들면 그 안에 넣고 「옮김」으로 끝내지 않는다 — 보관 실패, 원본 보존
func g2QaDirRace(k *K) {
	k.mk("g212")
	appendf(k.t, k.R+"/.git/info/exclude", ".qa/\n")
	k.land("g212l")
	os.Remove(k.wt + "/.omc/state/s.json")
	write(k.t, k.wt+"/.qa/r/REPORT.md", "r\n")
	lnp, err := exec.LookPath("ln")
	if err != nil {
		k.t.Fatal(err)
	}
	p := k.gfBin(map[string]string{"ln": `for d; do :; done; case $d in */.qa/r/REPORT.md) mkdir -p "$d" ;; esac; exec ` + sq(lnp) + ` "$@"`})
	k.runE([]string{p}, "clean", "--repo", k.R, "--branch", "g212l")
	k.check("R-12 clean: 목적지가 그사이 폴더가 되면 보관 실패로 멈추고 원본을 남긴다", k.rc != 0 && k.kept("g212l") && s13cat(k.wt+"/.qa/r/REPORT.md") == "r")
}

// R-14: send --all — null 배열·null 항목·주소 없는 live 항목·뒤 찌꺼기는 「보냄 0」 성공이 아니다
func g2SendShape(k *K) {
	for _, ls := range []string{`{"terminals":null}`, `{"terminals":[null]}`, `{"terminals":[{"state":"live","name":"lane-a","parent":"boss"}]}`, `{"terminals":[]}]`, `{}`} {
		p := k.gfBin(map[string]string{"hc": `case $1 in ls) printf '%s\n' ` + sq(ls) + `; exit 0 ;; send) echo "$*" >>"$HCLOG.send"; exit 0 ;; esac; exit 3`})
		k.runE([]string{p}, "send", "--all", "--from", "boss", "확인")
		k.check("R-14 send --all: "+ls+" 이면 실패 종료", k.rc != 0 && !strings.Contains(k.out, "보냄 0 · 실패 0"))
	}
	p := k.gfBin(map[string]string{"hc": `case $1 in ls) echo '{"terminals":[]}'; exit 0 ;; esac; exit 3`})
	k.runE([]string{p}, "send", "--all", "--from", "boss", "확인")
	k.check("R-14 send --all: 올바른 빈 배열만 0건 성공", k.rc == 0 && strings.Contains(k.out, "보냄 0 · 실패 0"))
}

// R-17: hc run 응답 — 주소 뒤가 잘린 JSON은 등록·성공하지 않고, 공백 있는 정상 JSON은 받는다.
// R-18: 시작했을 수 있는데 hc run 응답만 잃음(3) — 확정 실패(1)가 아니라 미확정(5), 작업 트리·임대 보존
func g2RunReply(k *K) {
	k.mk("g217")
	k.conf("lane.start_wait 0")
	lt := filepath.Join(k.T, ".local/state/hc-lane/lanes.tsv")
	cut := k.gfBin(map[string]string{"hc": `[ "$1" = run ] && { printf '{"address":"t9cut","agent"\n'; exit 0; }; exit 3`})
	k.runE([]string{cut}, "--read", "--name", "lane-r17a", "--repo", k.R, "--", "true")
	k.check("R-17 잘린 hc run 응답: 성공 아님(5), 레인 기록 없음", k.rc == 5 && !strings.Contains(read(lt), "t9cut"))
	sp := k.gfBin(map[string]string{"hc": `[ "$1" = run ] && { echo '{ "address" : "t9sp", "agent": "x", "state": "live" }'; exit 0; }; exit 3`})
	k.runE([]string{sp}, "--read", "--name", "lane-r17b", "--repo", k.R, "--", "true")
	k.check("R-17 공백 있는 정상 hc run 응답: 받고 레인 기록", k.rc == 0 && strings.Contains(read(lt), "t9sp\tlane-r17b"))
	mark := filepath.Join(k.T, "r18-ran")
	lost := k.gfBin(map[string]string{"hc": `[ "$1" = run ] && { : >` + sq(mark) + `; echo 'error: connection lost' >&2; exit 3; }; exit 3`})
	k.runE([]string{lost}, "--name", "lane-r18", "--repo", k.R, "--", "true")
	k.check("R-18 hc run 응답 잃음(3): 미확정 종료 5(1이 아님), 작업 트리 보존", exists(mark) && k.rc == 5 && k.kept("lane-r18"))
	k.noconf()
}

// R-20: ps가 0으로 끝났는데 빈 출력 — 살아 있는 pid의 실행 인자를 모르니 잠금을 넘겨받지 않고 75
func g2WatchPsEmpty(k *K) {
	st := filepath.Join(k.T, "wst-r20")
	wd := st + "/hc-lane"
	write(k.t, wd+"/coord.t7r20", "t7r20\t4242\tt7r20\t0\n")
	sl := exec.Command("sleep", "60")
	if err := sl.Start(); err != nil {
		k.t.Fatal(err)
	}
	defer func() { sl.Process.Kill(); sl.Wait() }()
	write(k.t, wd+"/watch.t7r20/pid", strconv.Itoa(sl.Process.Pid)+"\n")
	p := k.gfBin(map[string]string{"ps": `exit 0`})
	// 고치기 전 판은 잠금을 넘겨받아 끝없이 돈다 — 배경으로 띄우고 기한이 지나면 거둔다
	b := k.bg(k.T, filepath.Join(k.T, "r20o"), filepath.Join(k.T, "r20e"), []string{p, "XDG_STATE_HOME=" + st, "HC_FAKE_LIVE=t7r20", `HC_FAKE_LS={"terminals":[]}`, "HC_LANE_WAIT_TICK=1"}, k.LANE, "watch", "--to", "t7r20")
	done := upto(b.Done) || upto(b.Done) || upto(b.Done) // 24초(부하에서도 고친 판의 75는 몇 초 안)
	if !done {
		b.stop()
	}
	k.check("R-20 watch: ps 빈 출력(0)이면 잠금을 넘겨받지 않고 75", done && b.Wait() == 75 && strings.TrimSpace(read(wd+"/watch.t7r20/pid")) == strconv.Itoa(sl.Process.Pid))
}

// R-25: 이름 245바이트의 합법적인 .qa 파일 — 임시 이름이 길이 상한을 넘지 않고 보관·정리된다
func g2QaLongName(k *K) {
	k.mk("g225")
	appendf(k.t, k.R+"/.git/info/exclude", ".qa/\n")
	k.land("g225l")
	os.Remove(k.wt + "/.omc/state/s.json")
	nm := strings.Repeat("a", 242) + ".md"
	write(k.t, k.wt+"/.qa/r/"+nm, "long\n")
	k.run("clean", "--repo", k.R, "--branch", "g225l")
	k.check("R-25 clean: 245바이트 이름도 메인 폴더로 보관하고 정리", k.rc == 0 && s13cat(k.R+"/.qa/r/"+nm) == "long" && k.gone("g225l"))
}

// R-26: rebase가 index.lock으로 128 — 미해결 항목이 없으면 내용 충돌(3)이 아니라 다시 할 수 있는 75
func g2RebaseLock(k *K) {
	k.mk("g226")
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.git(k.R, "pull", "-q", "--ff-only")
	k.mklane("g226l")
	k.seedp("other.txt", "o")
	p := k.g2Git(`[ "$1" = rebase ] && [ "$2" = -q ] && { echo "fatal: Unable to create '$PWD/.git/index.lock': File exists." >&2; exit 128; }`)
	k.lndE([]string{p, "HC_LANE=g226l"}, k.wt, "main")
	k.check("R-26 land: index.lock 실패는 rebase 충돌(3)이 아니라 75", k.rc == 75 && !strings.Contains(k.errf()+k.out, "rebase 충돌"))
}

// R-13(watch): 잘린 not_found(3)가 되풀이돼도 「지휘 사라짐」으로 세지 않는다 — 살아 있는 지휘의 watch가 끝나지 않는다
func g2WatchGarbledGone(k *K) {
	st := filepath.Join(k.T, "wst-r13w")
	write(k.t, st+"/hc-lane/coord.t7gw", "t7gw\t4242\tt7gw\t0\n")
	// 대역은 답을 다 낸 뒤 t7gw 조회마다 한 줄(완료한 조회 수) — 시간이 아니라 이 수로 「구판이 끝낼 만큼 돌았다」를 정한다(재감사2 lane/R-223)
	done := filepath.Join(k.T, "r13w-gets")
	p := k.gfBin(map[string]string{"hc": `case $1 in
get) printf '{"error":{"kind":"not_found"' >&2; [ "$2" != t7gw ] || echo "$2 잘린 not_found" >>` + sq(done) + `; exit 3 ;;
ls) echo '{"machines":[],"terminals":[]}'; exit 0 ;;
machine) echo '{"machines":[]}'; exit 0 ;;
esac; exit 3`})
	b := k.bg(k.T, filepath.Join(k.T, "r13wo"), filepath.Join(k.T, "r13we"), []string{p, "XDG_STATE_HOME=" + st, "HC_LANE_WAIT_TICK=1"}, k.LANE, "watch", "--to", "t7gw")
	gets := func() int { return strings.Count(read(done), "잘린 not_found") }
	// 구판은 「없음」 세 바퀴째에 끝낸다 — 바퀴마다 t7gw 조회가 한 번 이상이니 5번이면 구판은 이미 끝냈을 수(60초 기한, 부하에도 넉넉히)
	for i := 0; i < 600 && gets() < 5; i++ {
		select {
		case <-b.done:
			i = 600
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
	n := gets()
	k.check("R-223 준비: 대역이 잘린 not_found 조회를 5번 이상 끝냈다("+strconv.Itoa(n)+"번)", n >= 5)
	k.check("R-13 watch: 잘린 not_found 5번 뒤에도 살아 있다", !b.Done())
	k.runE([]string{p, "XDG_STATE_HOME=" + st}, "watch", "--to", "t7gw")
	k.check("R-223 watch: 감시 잠금을 그대로 쥐고 있다(둘째 watch는 「이미 돈다」로 끝)", k.rc != 0 && strings.Contains(k.errf(), "감시가 이미 돈다"))
	b.stop()
}

// R-15: 주차 근거(refs/park·같은 SHA의 마지막 주차 사건) 없는 옛 제출 가지 lane/<레인>에서는 재개하지 않는다(모의·실제 같게).
// ls-remote의 깨진 줄은 주차 SHA가 아니다
func g2ResumeLegacy(k *K) {
	k.mk("g215")
	k.sh(k.R, `git pull -q --ff-only && git push -q origin HEAD:refs/heads/lane/r15x`)
	k.run("--resume", "--dry-run", "--name", "r15x", "--repo", k.R, "--", "claude")
	k.check("R-15 --resume --dry-run: 주차 사건 없는 lane/<레인>은 재개 안 함", k.rc != 0 && !strings.Contains(k.out, "에서 시작"))
	k.run("--resume", "--name", "r15x", "--repo", k.R, "--", "claude")
	k.check("R-15 --resume: 주차 사건 없는 lane/<레인>에서 작업 트리를 만들지 않음", k.rc != 0 && !exists(k.R+".wt/r15x"))
	sha := k.git(k.R, "rev-parse", "HEAD")
	k.runE([]string{"HC_LANE=r15x"}, "report", "주차", sha, "옛 판 주차")
	k.run("--resume", "--dry-run", "--name", "r15x", "--repo", k.R, "--", "claude")
	k.check("R-15 --resume --dry-run: 같은 SHA의 주차 사건이 있으면 옛 가지에서 시작", k.rc == 0 && strings.Contains(k.out, "refs/heads/lane/r15x"))
	p := k.g2Git(`[ "$1" = -C ] && [ "$3" = ls-remote ] && { printf 'garbled\trefs/park/r15g\n'; exit 0; }`)
	k.runE([]string{p}, "--resume", "--dry-run", "--name", "r15g", "--repo", k.R, "--", "claude")
	k.check("R-15 --resume --dry-run: 깨진 ls-remote 줄은 주차 SHA로 안 읽음", k.rc != 0 && !strings.Contains(k.out, "에서 시작"))
}

// R-16: 제출 커밋에서 사전 검사 훅을 찾는 조회가 실패하면 「훅 없음」으로 제출하지 않고 75
func g2PrecheckLookup(k *K) {
	k.mk("g216")
	k.mklane("g216l")
	mark := filepath.Join(k.T, "r16-ran")
	k.sh(k.wt, `mkdir -p .claude && printf ': >%s\nexit 1\n' "$1" >.claude/herdr-precheck.sh && git add -- .claude/herdr-precheck.sh && git commit -qm pc -- .claude/herdr-precheck.sh`, sq(mark))
	k.git(k.D+"/origin.git", "update-ref", "refs/herdr/merger/box", "main")
	p := k.g2Git(`case " $* " in *" ls-tree "*".claude/herdr-precheck.sh"*|*":.claude/herdr-precheck.sh "*) exit 128 ;; esac`)
	k.lndE([]string{p, "HC_LANE=g216l", "HC_LANE_MGON_WAITS=0"}, k.wt, "main")
	q := k.git(k.D+"/origin.git", "for-each-ref", "--format=%(refname)", "refs/merge-queue/")
	k.check("R-16 제출: 훅 조회 실패면 큐에 안 넣고 75", k.rc == 75 && q == "" && !exists(mark))
}

// R-24: 옛 판 주차 사건이 대상 기계(--on)에만 있어도 clean --on이 「주차됨」으로 읽는다(지휘 기계 사건만 보지 않는다)
func g2LegacyParkRemote(k *K) {
	k.mk("g224")
	k.run("--name", "lg24", "--repo", k.R, "--", "claude")
	w := k.R + ".wt/lg24"
	k.sh(w, `echo x >lg24.txt && git add -- lg24.txt && git commit -qm lg24 -- lg24.txt && git push -q origin HEAD:refs/heads/lane/lg24`)
	sa := k.git(w, "rev-parse", "HEAD")
	evf := filepath.Join(k.T, ".local/state/hc-lane/events.tsv")
	before := read(evf)
	k.runE([]string{"HC_LANE=lg24"}, "report", "주차", sa, "옛 판 주차")
	line := strings.TrimPrefix(read(evf), before)
	write(k.t, evf, before) // 지휘 기계에서는 지우고 대상 기계(box)에만 둔다
	mh := filepath.Join(k.T, "mh24")
	write(k.t, mh+"/box/.local/state/hc-lane/events.tsv", line)
	k.runE([]string{"HC_FAKE_MACH_HOME=" + mh}, "clean", "--on", "box", "--repo", k.R, "--branch", "lg24", "--dry-run")
	k.check("R-24 clean --on box: 대상 기계에만 있는 옛 주차 사건도 「주차됨」", k.rc == 0 && strings.Contains(k.out, "주차됨"))
}

// R-35: 배경 명령이 먼저 끝나도 절이 끝날 때(Cleanup) 그 명령의 남은 손자까지 거둔다
func g2BgGrandchild(k *K) {
	mark := filepath.Join(k.T, "r35-mark")
	k.t.Run("sub", func(t *testing.T) {
		pt := k.t
		k.t = t
		defer func() { k.t = pt }()
		b := k.bg(k.T, "", "", nil, "sh", "-c", `(sleep 2; : >`+sq(mark)+`) & exit 0`)
		upto(b.Done)
	})
	time.Sleep(3500 * time.Millisecond)
	k.check("R-35 bg: 우두머리가 먼저 끝나도 Cleanup이 손자까지 거둔다", !exists(mark))
}
