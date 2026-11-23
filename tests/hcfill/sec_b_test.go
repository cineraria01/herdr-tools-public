package hcfill_test

// 셸 시험 342~517줄: 4. 선행 조건 · 3c·3d 설정 · 5. 두 번 띄우지 않음 · 6·7·8 · 9·9b · 10. ls · 10b. 끝난 레인 정리.

import (
	"strconv"
	"strings"
	"time"
)

func init() { reg(20, "b", secB) }

// bQline: 대기열에서 첫 칸이 name인 줄들(awk -F'\t' '$1==name').
func (k *K) bQline(name string) string {
	var l []string
	for _, r := range rows(k.Q) {
		if col(r, 1) == name {
			l = append(l, strings.Join(r, "\t"))
		}
	}
	return strings.Join(l, "\n")
}

// bConfSedBak: sed -i.bak — 고치기 전 설정을 .bak으로 남기고 고친다.
func (k *K) bConfSedBak(re, repl string) {
	copyFile(k.t, k.conf(), k.conf()+".bak")
	k.confSed(re, repl)
}

func secB(k *K) {
	in := strings.Contains
	gitlog, ll, hclog := k.T+"/git.log", k.T+"/lane.log", k.T+"/hc.log"

	// 4. 선행 조건: origin/<기본>의 커밋 메시지에 레인 이름(단어 경계)
	k.fresh()
	k.add("pa")
	k.add("pb", "--after", "pa,pc", "--no-early")
	write(k.t, k.T+"/commits", "1111aaaa feat: pa-extra 다른 레인\n4444dddd docs(x): pz 문서 번호 (main에 pa가 먼저 착지)\n5555eeee test: 주입 (pa 감사 반영)\n")
	k.run("run")
	k.check("선행: 제목 머리가 아닌 언급(괄호·문장 안)은 안 셈", k.state("pb") == "wait" && in(k.out, "선행 pb ← pa: 아직") && in(k.out, "선행 pb ← pc: 아직"))
	k.check("선행: fetch는 local 기계 저장소에서 한 번", countF(gitlog, "fetch -q origin") == 1 && hasE(gitlog, "^-C /Users/me/src/app fetch"))
	appendFile(k.t, k.T+"/commits", "2222bbbb feat(x): pa 끝\n3333cccc pc: 고침\n")
	k.run("run")
	k.check("선행: 둘 다 들어가면 띄움, 판정 줄에 커밋", k.state("pb") == "up" && in(k.out, "선행 pb ← pa: 들어감 (2222bbbb"))
	k.fresh()
	k.add("pd", "--after", "pa")
	k.set("HC_FAKE_FETCH_FAIL=1")
	write(k.t, k.T+"/commits", "2222bbbb pa\n")
	k.run("run")
	k.check("선행: fetch 실패면 띄우지 않음", k.state("pd") == "wait" && in(k.out, "fetch 실패"))
	k.unset("HC_FAKE_FETCH_FAIL")

	// 3c. 설정 읽기: 키 혼자면 빈 값, 같은 키 두 번은 멈춤
	k.fresh()
	k.confSed(`^coord .*`, "coord")
	k.add("cf", "--only", "here")
	k.run("run")
	k.check("설정: coord 키 혼자면 빈 값 → 붙이지 않음", k.where("cf") == "up here cf" && !exists(k.T+"/balance.log"))
	k.fresh()
	k.add("cn", "--only", "here")
	k.runE([]string{"HC_FAKE_COORD="}, "run")
	k.check("지휘 정보가 없거나 안 맞으면(hc-lane coord 실패) HC_LANE_COORD를 안 넘기고 「지휘 없음」 한 줄(레인은 띄움)", k.where("cn") == "up here cn" && lastLine(read(k.T+"/coord.log")) == "없음" && in(k.out, "지휘 없음"))
	k.cpConf()
	k.confAppend("local    other")
	k.fresh()
	k.add("cg")
	k.run("run")
	k.check("설정: 같은 키 두 줄이면 종료 1, 키 이름, 띄우지 않음", k.rc == 1 && grep(k.out, "local 줄이 두 번") && !exists(ll))
	k.cpConf()

	// 3d. hc ls에 설정한 agent 종류가 하나도 없으면 그 바퀴는 띄우지 않음
	k.fresh()
	k.add("ag")
	k.set(`HC_FAKE_LS={"terminals":[{"address":"box-1/p","agent":"env","name":"proxy","state":"live","status":null,"cwd":"/"}]}`)
	k.run("run")
	k.check("agent 종류 0개: 경고, 종료 1, 띄우지 않음", k.rc == 1 && grep(k.out, "claude-code 종류가 하나도 없다") && !exists(ll))

	// 5. 두 번 띄우지 않음
	k.fresh()
	k.lj("slow/k1:claude-code:dup:working:/Users/other/src/app.wt/dup")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"slow/k1","state":"live","cwd":"/"}]}`)
	k.add("dup")
	k.run("run")
	k.check("같은 이름 살아 있음(hc-lane 레인·그 프로젝트 폴더): hc-lane 안 부르고 떠 있음으로 표시", !exists(ll) && k.where("dup") == "up slow slow/k1" && grep(k.out, "이미 떠 있음 dup"))
	k.fresh()
	k.lj("slow/k1:claude-code:dup:working:/Users/other/src/app.wt/dup", "box-2/k2:claude-code:dup2:working:/elsewhere/dup2")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-2/k2","state":"live","cwd":"/"}]}`)
	k.add("dup")
	k.add("dup2")
	k.run("run")
	k.check("같은 이름이어도 hc-lane 레인(라벨 hc-lane=1)이 아니면 채택하지 않고 띄움", k.where("dup") == "up box-1 box-1/dup")
	k.check("같은 이름 hc-lane 레인이어도 그 프로젝트 폴더가 아니면 채택하지 않고 띄움", k.where("dup2") == "up box-1 box-1/dup2" && !grep(k.out, "이미 떠 있음"))

	// 6. 연결 안 된 기계는 건너뜀
	k.fresh()
	k.mj("box-2", "shared", "slow")
	k.add("c1")
	k.run("run")
	k.check("연결 안 됨: box-1 건너뛰고 box-2", k.where("c1") == "up box-2 box-2/c1" && grep(k.out, "건너뜀 box-1: 연결 안 됨"))

	// 7. --only
	k.fresh()
	k.add("o1", "--only", "slow,here")
	k.run("run")
	k.check("--only: 고른 기계만(차례 안에서)", k.where("o1") == "up slow slow/o1")
	k.fresh()
	k.fill(nil, "add", "--name", "o2", "--project", "tools", "--brief", k.brief("o2"))
	k.run("run")
	k.check("프로젝트 경로가 있는 기계만: tools는 here", k.where("o2") == "up here o2")

	// 8. 띄운 뒤 status가 안 잡히면 그 기계를 빼고 다음 기계로
	k.fresh()
	k.set("HC_FAKE_NOSTATUS=box-1/st")
	k.add("st")
	k.run("run")
	k.check("status 안 잡힘: box-2로 다시, 남긴 것 기록", k.where("st") == "up box-2 box-2/st" && in(k.bQline("st"), "box-1=box-1/st") && grep(k.out, "상태 안 잡힘 st"))
	k.check("status 안 잡힘: hc kill 등 부르지 않음", !nonEmpty(hclog) || len(grepLines(read(hclog), "^exec ")) == len(lines(read(hclog))))
	k.fresh()
	k.set("HC_FAKE_LANE_FAIL=box-1/lf")
	k.add("lf")
	k.run("run")
	k.check("hc-lane 실패: 다음 기계로", k.where("lf") == "up box-2 box-2/lf" && grep(k.out, "실패 lf @box-1"))
	k.unset("HC_FAKE_LANE_FAIL")
	k.fresh()
	k.set("HC_FAKE_LANE_HELD=lh")
	k.add("lh")
	k.run("run")
	k.unset("HC_FAKE_LANE_HELD")
	k.check("hc-lane 종료 10(이미 돎 — 임대 남음): 못 띄움으로 읽어 다음 기계도 10이면 이번 바퀴 포기(줄은 wait 그대로)", k.where("lh") == "wait - -" && countE(ll, "^lh$") >= 2 && grep(k.out, "실패 lh @box-1"))
	k.fresh()
	k.set("HC_FAKE_LANE_START=box-1/s6=6")
	k.add("s6")
	k.run("run")
	k.check("hc-lane 시작 확인 실패(종료 5·6·7): 남은 터미널을 stuck으로 기록하고 그 기계 빼고 다음 기계로(감사 [중간] 1)", k.where("s6") == "up box-2 box-2/s6" && in(k.bQline("s6"), "box-1=box-1/s6") && grep(k.out, "상태 안 잡힘 s6 @box-1 box-1/s6") && grep(k.out, "가짜 로그인 막힘"))

	k.fresh()
	k.mj("box-1")
	k.set("HC_FAKE_NOSTATUS=box-1/rv")
	k.add("rv", "--only", "box-1,box-2")
	k.run("run")
	k.check("남김: 다른 자리가 없으면 대기로 남음", k.where("rv") == "wait - -")
	k.unset("HC_FAKE_NOSTATUS")
	k.mj("box-1", "box-2")
	k.lj("box-1/rv:claude-code:rv")
	k.run("run")
	k.check("남긴 터미널이 살아나면 중복 위험 찍고 그 레인은 더 띄우지 않음", grep(k.out, "중복 위험 rv box-1/rv") && k.where("rv") == "wait - -" && countE(ll, "^--name$") == 2)

	k.fresh()
	k.set("HC_FAKE_NOSTATUS=box-1/ru")
	k.add("ru")
	k.run("run")
	k.unset("HC_FAKE_NOSTATUS")
	k.lj("box-1/ru:claude-code:ru", "box-2/ru:claude-code:ru")
	k.run("run")
	k.check("떠 있는 레인도 남긴 터미널이 살아나면 중복 위험(둘 다 주소)", grep(k.out, "중복 위험 ru box-1/ru: .*지금 떠 있는 box-2/ru"))

	// 9. --dry-run은 찍기만
	k.fresh()
	k.add("d1")
	k.add("d2")
	k.add("d3")
	before := strings.TrimRight(read(k.Q), "\n")
	k.run("run", "--dry-run")
	k.check("dry-run: 무엇을 어디에(상한 반영)", k.rc == 0 && grep(k.out, `\[dry-run\] 띄울 d1 → box-1`) && grep(k.out, `\[dry-run\] 띄울 d3 → box-2`))
	k.check("dry-run: hc-lane·hc exec·herdr-balance·대기열 변화 없음, fetch 없음", !exists(ll) && !exists(hclog) && !exists(k.T+"/balance.log") && strings.TrimRight(read(k.Q), "\n") == before && !hasE(gitlog, "fetch"))

	// 9b. hc가 실패하거나 엉뚱한 출력이면 빈 표로 읽지 않고 멈춘다
	k.fresh()
	k.add("h1")
	k.set("HC_FAKE_LS_FAIL=1")
	k.run("run")
	k.unset("HC_FAKE_LS_FAIL")
	k.check("hc ls 실패: 종료 1, 띄우지 않음", k.rc == 1 && !exists(ll) && k.where("h1") == "wait - -")
	k.runE([]string{"HC_FAKE_LABEL_RC=1"}, "run")
	k.check("라벨 조회 실패: 출력이 빈 목록이어도 대기 줄을 띄우지 않음", k.rc == 1 && k.where("h1") == "wait - -" && !exists(ll))
	k.runE([]string{"HC_FAKE_LS_LANE={}"}, "run")
	k.check("라벨 응답에 terminals가 없으면 이번 바퀴 보류", k.rc == 1 && !exists(ll))
	k.set(`HC_FAKE_LS={"error":{"kind":"usage"}}`)
	k.run("run")
	k.unset("HC_FAKE_LS")
	k.check("hc ls 출력에 terminals 없음: 종료 1, 띄우지 않음", k.rc == 1 && !exists(ll))
	k.set("HC_FAKE_MACHINES=")
	k.run("run")
	k.check("hc machine ls 빈 출력: 종료 1, 띄우지 않음", k.rc == 1 && !exists(ll))

	// 10. ls: 떠 있음 · 끝남
	k.fresh()
	k.add("e1")
	k.add("e2")
	k.run("run")
	k.lj("box-1/e1:claude-code:e1")
	k.run("ls")
	k.check("ls: 살아 있으면 떠 있음, 사라졌으면 끝남", grep(k.out, "e1.*떠 있음") && grep(k.out, "e2.*끝남"))

	// 10b. 끝난 레인 정리
	gcl, last := k.T+"/gc.log", k.DATA+"/gc.last"
	k.fresh()
	k.mj("box-1", "shared")
	k.run("run")
	k.check("gc: 대기열이 비어도 첫 바퀴에 돈다", k.rc == 0 && nonEmpty(gcl), read(gcl))
	k.check("gc: local 저장소는 --on 없이 --integ, 프로젝트마다", hasFx(gcl, "gc --integ --repo /Users/me/src/app") && hasFx(gcl, "gc --integ --repo /Users/me/src/tools"))
	k.check("gc: 연결된 다른 기계는 --on, --integ 없이", hasFx(gcl, "gc --on box-1 --repo /home/me/src/app") && hasFx(gcl, "gc --on shared --repo /srv/me/app"))
	k.check("gc: 연결 안 된 기계는 건너뜀과 알림", !hasF(gcl, "box-2") && hasF(k.GCL, "gc 건너뜀 app @box-2: 연결 안 됨"))
	k.check("gc: 출력 줄은 바퀴 출력이 아니라 gc.log에, 프로젝트·기계와 함께", hasF(k.GCL, "gc app @box-1: 지움 0 · 남김 0") && !in(k.out, "gc app @box-1:") && in(k.out, "gc: 배경으로 띄움(pid "))
	rm(gcl)
	k.add("g1")
	k.run("run")
	k.check("gc: 간격(60분) 안의 다음 바퀴는 안 돈다", !exists(gcl))
	write(k.t, last, "0\n")
	k.runE([]string{"HC_FAKE_GC_RC=1"}, "run")
	k.check("gc: 간격이 지나면 다시, 실패해도 바퀴는 종료 0과 알림", k.rc == 0 && nonEmpty(gcl) && hasF(k.GCL, "gc 실패 app @here (종료 1)"))
	// fill-gc-async: 정리가 오래 걸려도 바퀴는 기다리지 않고, 다음 바퀴의 띄우기도 막히지 않는다
	k.fresh()
	k.mj("box-1", "shared")
	k.add("gs1")
	s0 := time.Now()
	k.out, k.rc = k.exe(k.T, []string{"HC_FAKE_GC_SLEEP=8"}, k.FILL, "run") // gcwait 없이(셸의 out=$(…))
	el := int(time.Since(s0).Seconds())
	k.check("gc 배경: 정리가 8초 걸려도 바퀴는 기다리지 않음(띄우기는 끝남)", k.rc == 0 && el < 5 && k.state("gs1") == "up", "걸린 초 "+strconv.Itoa(el))
	gslow := k.out
	write(k.t, last, "0\n")
	k.add("gs2")
	s0 = time.Now()
	k.run("run")
	el = int(time.Since(s0).Seconds())
	k.check("gc 배경: 앞 정리가 돌면 다음 바퀴는 안 띄우고 막히지도 않음, 시각 그대로", k.rc == 0 && el < 5 && k.state("gs2") == "up" && in(k.out, "gc: 앞 정리가 아직 돈다") && strings.TrimRight(read(last), "\n") == "0", "걸린 초 "+strconv.Itoa(el))
	// 둘째 바퀴가 앞 정리가 도는 것을 봤으니 놓아준다 — 가짜 gc는 대상마다 8초(넷이면 32초)를 끝까지 자지 않고 바로 끝난다
	// (2026-11-24 tests-wait-trim). 아래 확인은 끝난 뒤 남은 출력·잠금만 본다
	write(k.t, k.T+"/gc.release", "")
	k.out = gslow
	k.gcwait()
	rm(k.T + "/gc.release")
	_, lrc := k.exe(k.T, nil, "lockf", "-k", "-t", "0", k.DATA+"/gc.lock", "true")
	k.check("gc 배경: 끝나면 출력이 gc.log에, 잠금이 풀려 다음 정리를 띄울 수 있음", hasF(k.GCL, "gc app @box-1: 지움 0 · 남김 0") && lrc == 0)
	k.fresh()
	k.bConfSedBak(`^gc .*`, "gc       0")
	k.run("run")
	k.check("gc: 설정 gc 0이면 끔", k.rc == 0 && !exists(gcl))
	k.bConfSedBak(`^gc .*`, "gc       60")
	k.run("run", "--dry-run")
	k.check("gc: dry-run은 --dry-run을 넘기고 시각을 적지 않음", hasFx(gcl, "gc --integ --dry-run --repo /Users/me/src/app") && !exists(last))
}
