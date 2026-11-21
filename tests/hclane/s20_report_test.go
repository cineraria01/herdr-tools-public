package hclane_test

// tests/hc-lane.test.sh 20절 앞(1272–1435줄): hc-lane report(칸·이름·긴 줄·동시·--key), verdict, land --verdict.
// 20절 뒤는 s20_wait_test.go(events·wait)·s20_stop_test.go(멈춤 유예·프로세스·끝남·notify·커서·print-steps).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

func init() { reg(80, "s20a", s20Report) }

// s20ev: 셸 $EV.
func (k *K) s20ev() string { return filepath.Join(k.T, ".local/state/hc-lane/events.tsv") }

// s20lines: 파일의 빈 줄 아닌 줄(grep .).
func s20lines(p string) []string {
	var l []string
	for _, s := range strings.Split(read(p), "\n") {
		if s != "" {
			l = append(l, s)
		}
	}
	return l
}

// s20tail: tail -n 1 "$EV"(끝 줄바꿈 뗌).
func (k *K) s20tail() string { return lastLine(strings.TrimRight(read(k.s20ev()), "\n")) }

// s20cut: cut -f<a>-<b> 한 줄(탭이 없으면 줄 그대로 — cut 꼴).
func s20cut(line string, a, b int) string {
	if !strings.Contains(line, "\t") {
		return line
	}
	f := strings.Split(line, "\t")
	var o []string
	for i := a; i <= b && i <= len(f); i++ {
		o = append(o, f[i-1])
	}
	return strings.Join(o, "\t")
}

// s20fld: 셸 fld <줄> <칸>.
func s20fld(line string, n int) string { return s20cut(line, n, n) }

// s20nf: awk -F"\t" '{print NF}' 한 줄.
func s20nf(line string) int {
	if line == "" {
		return 0
	}
	return strings.Count(line, "\t") + 1
}

// s20cutFile: cut -f<n> 파일(줄마다, $(…) 꼴로 끝 줄바꿈 뗌).
func s20cutFile(p string, n int) string {
	var o []string
	for _, l := range strings.Split(strings.TrimRight(read(p), "\n"), "\n") {
		o = append(o, s20fld(l, n))
	}
	return strings.Join(o, "\n")
}

// s20key: awk -F"\t" '$12 == "<키>"' "$EV" | grep -c .
func (k *K) s20key(key string) int {
	n := 0
	for _, l := range s20lines(k.s20ev()) {
		if f := strings.Split(l, "\t"); len(f) >= 12 && f[11] == key {
			n++
		}
	}
	return n
}

// s20comb: (cd dir && VAR=값 "$LANE" … >파일 2>&1) — stdout·stderr를 한 파일로.
func (k *K) s20comb(dir, path string, extra []string, args ...string) {
	cmd := exec.Command(k.LANE, args...)
	cmd.Dir, cmd.Env = dir, k.environ(extra)
	b, _ := cmd.CombinedOutput()
	if err := os.WriteFile(path, b, 0o644); err != nil {
		k.t.Error(err)
	}
}

func s20Report(k *K) {
	// 20. 레인 사건: hc-lane report·events·wait (지휘는 사건만 읽는다)
	k.mk("t20")
	EV := k.s20ev()
	tl := k.s20tail
	k.runE([]string{"HC_LANE_MACHINE=mac", "HC_TERMINAL_ID=t5", "HC_LANE=lane-q"}, "report", "완료", "abc1234", "요약\t탭\n줄")
	ln1 := strings.TrimRight(read(EV), "\n")
	k.check("report: 종료 0, events.tsv에 한 줄", k.rc == 0 && len(s20lines(EV)) == 1)
	f := func(n int) string { return s20fld(ln1, n) }
	k.check("report: 열두 칸(시각 UTC·기계·터미널·레인·상태·sha·한 줄·turn·--to·발행 실행·수신 지휘·키)", !strings.Contains(ln1, "\n") && s20nf(ln1) == 12 &&
		f(9)+"|"+f(12) == "-|-" && regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$`).MatchString(f(1)) &&
		f(2)+"|"+f(3)+"|"+f(4)+"|"+f(5)+"|"+f(6) == "mac|t5|lane-q|완료|abc1234")
	k.check("report: 한 줄의 탭·줄바꿈은 지움", f(7) == "요약 탭 줄")
	k.check("report: 화면에도 같은 사건", strings.Contains(k.out, "lane-q 완료") && strings.Contains(k.out, "abc1234"))
	k.runE([]string{"HC_LANE=lane-q"}, "report", "막힘", "이유만 있음")
	k.check("report: sha 없이 한 줄만 주면 sha는 -", k.rc == 0 && s20cut(tl(), 5, 7) == "막힘\t-\t이유만 있음")
	k.runE([]string{"HC_LANE=lane-q"}, "report", "판정", "-", "감사 완료")
	k.check("report: 판정 - <한 줄>", k.rc == 0 && s20cut(tl(), 5, 7) == "판정\t-\t감사 완료")
	k.runE([]string{"HC_LANE=lane-q"}, "report", "끝남", "abc")
	k.check("report: 모르는 상태는 거절, 아무것도 안 씀", k.rc == 2 && len(s20lines(EV)) == 3)
	k.runE([]string{"HC_LANE=lane-q"}, "report", "완료", "abc", "--name", "lane-n")
	k.check("report: 레인 이름은 --name이 $HC_LANE보다 앞", s20fld(tl(), 4) == "lane-n")
	k.runE([]string{"HC_LANE=lane-q"}, "report", "착지", "6D295B6F", "끝")
	k.check("report: 대문자 sha는 소문자로 적음(한 줄 칸으로 보내지 않음 — 감사 wait-landed [낮음] 3)", k.rc == 0 && s20cut(tl(), 5, 7) == "착지\t6d295b6f\t끝")
	k.runE([]string{"HC_LANE=lane-q"}, "report", "막힘", "DB", "연결", "실패")
	k.check("report: 7자 미만 16진 낱말은 한 줄 칸에 대문자 그대로(재감사 [낮음] 1)", k.rc == 0 && s20cut(tl(), 5, 7) == "막힘\t-\tDB 연결 실패")
	k.sh(k.R, "git checkout -q -b lane-br")
	k.runIn(k.R, nil, "report", "중간")
	k.check("report: --name·$HC_LANE 없으면 작업 트리 가지 이름", k.rc == 0 && s20fld(tl(), 4) == "lane-br")
	k.sh(k.R, "git checkout -q main")
	k.runIn(k.R, []string{"HC_LANE=lane-h"}, "report", "중간")
	k.check("report: $HC_LANE이 가지 이름보다 앞", s20fld(tl(), 4) == "lane-h")
	host, _ := k.exe(k.T, "", nil, "hostname", "-s")
	k.check("report: 기계 이름 기본은 hostname", s20fld(tl(), 2) == host)
	// 작업 중 발견해 함께 고침(지휘 실측: 워커 기계 레인 t10 v5-lb → 「main 착지」): hc-lane clean으로 작업 트리를 지운 뒤 report하면
	// 지금 폴더 가지(main)가 레인 칸에 찍혔다. 레인 이름은 가지보다 hc 터미널 이름이 먼저, 기본 가지 이름은 레인으로 쓰지 않는다
	tn := []string{"HC_TERMINAL_ID=t10", "HC_FAKE_GET_TURN=3", "HC_FAKE_GET_NAME=v5-lb"}
	k.runIn(k.R, tn, "report", "착지", "abc1234")
	k.check("report: --name·$HC_LANE 없으면 가지(main)보다 hc 터미널 이름", k.rc == 0 && s20fld(tl(), 4) == "v5-lb")
	k.sh(k.R, "git checkout -q -b lane-br2")
	k.runIn(k.R, tn, "report", "중간")
	k.check("report: hc 터미널 이름이 가지 이름보다 앞", s20fld(tl(), 4) == "v5-lb")
	k.sh(k.R, "git checkout -q main")
	k.runIn(k.R, nil, "report", "중간")
	k.check("report: 이름이 없고 가지가 기본 가지(main)면 레인 칸은 -", k.rc == 0 && s20fld(tl(), 4) == "-")

	// 감사 반영 1·7: 긴 줄을 여럿이 함께 report해도 줄이 섞이지 않는다(줄 전체 1000바이트 안, 끝 반쪽 UTF-8 버림)
	write(k.t, EV, "")
	long := strings.Repeat("가", 450)
	var wg sync.WaitGroup
	for p := 1; p <= 4; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				k.exe(k.T, "", []string{fmt.Sprintf("HC_LANE=lane-c%d", p)}, k.LANE, "report", "중간", "-", long)
			}
		}(p)
	}
	wg.Wait()
	all12, short, evl := true, true, s20lines(EV)
	for _, l := range evl {
		all12 = all12 && s20nf(l) == 12
	}
	for _, l := range strings.Split(read(EV), "\n") {
		short = short && len(l) <= 1000
	}
	k.check("report: 긴 줄 동시 100건이 모두 12칸 한 줄", len(evl) == 100 && all12)
	k.check("report: 줄은 1000바이트 안, 끝 UTF-8 글자 온전", short && utf8.ValidString(read(EV)) && len(s20fld(tl(), 7))+1 > 600)
	k.runE([]string{"HC_LANE=" + strings.Repeat("n", 300), "HC_LANE_MACHINE=" + strings.Repeat("m", 300)}, "report", "완료", "abc1234", long)
	k.check("report: 긴 이름·기계 칸도 잘려 12칸·1000바이트 안", k.rc == 0 && s20nf(tl()) == 12 && len(tl())+1 <= 1001 && s20fld(tl(), 6) == "abc1234")
	// 10칸 발행 실행(HC_LANE_JOB을 주면 그 값, 없으면 <터미널>@<created_ms>), 11칸 수신 지휘(HC_LANE_OWNER@HC_LANE_OWNER_RUN — fill-handoff-integrity), 12칸 키
	write(k.t, EV, "")
	k.runE([]string{"HC_TERMINAL_ID=t5", "HC_FAKE_GET_TURN=2", "HC_LANE_JOB=j1120-a1b2", "HC_LANE_OWNER=tq9", "HC_LANE=lane-q"}, "report", "중간", "-", "실행칸")
	k.check("report: 10칸 작업 ID, 11칸 수신 지휘 tq9, 12칸 -", k.rc == 0 && s20cut(tl(), 10, 12) == "j1120-a1b2\ttq9\t-")
	k.runE([]string{"HC_TERMINAL_ID=t5", "HC_FAKE_GET_TURN=2", "HC_LANE_JOB=j1120-a1b2~a2", "HC_LANE_OWNER=tq9", "HC_LANE_OWNER_RUN=1700000000555", "HC_LANE=lane-q"}, "report", "중간", "-", "실행아이디")
	k.check("report: 물결(~) 든 작업 ID도 10칸, 11칸은 <지휘>@<지휘 실행>", s20cut(tl(), 10, 11) == "j1120-a1b2~a2\ttq9@1700000000555")
	k.runE([]string{"HC_TERMINAL_ID=t5", "HC_FAKE_GET_TURN=2", "HC_LANE=lane-q"}, "report", "완료", "abc1234", "작업없음")
	k.check("report: HC_LANE_JOB이 없고 created_ms도 모르면 10칸 -, 경고", k.rc == 0 && s20fld(tl(), 10) == "-" && strings.Contains(k.errf(), "발행 실행(10칸)을 못 적었다"))
	k.runE([]string{"HC_TERMINAL_ID=box-1/t5", "HC_FAKE_GET_TURN=2", "HC_FAKE_GET_CREATED=1700000000123", "HC_LANE=lane-q"}, "report", "완료", "abc1234", "실행키")
	k.check("report: HC_LANE_JOB이 없으면 10칸 = <기계 접두 없는 터미널>@<created_ms>(작업 ID 장치는 없다 — fill-dup-guard가 범위를 좁힘)", k.rc == 0 && s20fld(tl(), 10) == "t5@1700000000123")
	k.exe(k.T, "", []string{"HC_LANE=lane-q"}, k.LANE, "report", "중간", "-", "터미널밖") // env -u HC_TERMINAL_ID -u HC_LANE_OWNER(k.env에 없음)
	k.check("report: 터미널 밖이면 10·11칸 -", s20cut(tl(), 10, 11) == "-\t-")
	k.runE([]string{"HC_LANE=lane-q"}, "report", "막힘", "-", "넘김 글", "--name", "lane-x", "--to", "tq1", "--key", "hf:a1:coord")
	k.runE([]string{"HC_LANE=lane-q"}, "report", "막힘", "-", "넘김 글 다시", "--name", "lane-x", "--to", "tq1", "--key", "hf:a1:coord")
	k.check("report --key: 같은 키 두 번째는 더하지 않고 「이미 있음」(종료 0)", k.rc == 0 && k.s20key("hf:a1:coord") == 1 && strings.Contains(k.out, "이미 있음"))
	for p := 1; p <= 6; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			k.exe(k.T, "", []string{"HC_LANE=lane-q"}, k.LANE, "report", "막힘", "-", "동시", "--key", "hf:a2:coord")
		}()
	}
	wg.Wait()
	k.check("report --key: 동시 여섯 번도 한 줄", k.s20key("hf:a2:coord") == 1)
	k.runE([]string{"HC_LANE=lane-q"}, "report", "중간", "-", "본문에 hf:a3:coord 인용")
	k.runE([]string{"HC_LANE=lane-q"}, "report", "막힘", "-", "키는칸만", "--key", "hf:a3:coord")
	k.check("report --key: 본문에 같은 글이 있어도 12칸만 견줌(더함)", k.rc == 0 && k.s20key("hf:a3:coord") == 1)
	os.Chmod(EV, 0o000)
	k.runE([]string{"HC_LANE=lane-q"}, "report", "막힘", "-", "못읽음", "--key", "hf:a4:coord")
	os.Chmod(EV, 0o644)
	k.check("report --key: events.tsv를 못 읽으면 종료 1, 안 더함", k.rc == 1 && !strings.Contains(read(EV), "못읽음"))
	k.runE([]string{"HC_LANE=lane-q"}, "report", "막힘", "-", "x", "--key", "a;b")
	k.check("report --key: 키 글자가 아니면 종료 2", k.rc == 2)

	s20Verdict(k)
	s20LandVerdict(k)
}

// s20vput: 셸 vput <ID> <실행> <sha> <판정>.
func s20vput(k *K, vd, id, run, sha, verdict string) {
	write(k.t, filepath.Join(vd, id+".md"), fmt.Sprintf("aid\t%s\nlane\tlane-v\nrun\t%s\nsha\t%s\nverdict\t%s\nreport\t~/rep/%s.md\nround\t1\nbase\tmain\n", id, run, sha, verdict, id))
}

func s20Verdict(k *K) {
	// fill-handoff-integrity: verdict <ID> — hc-fill이 둔 판정 하나를 그 작업(HC_LANE_JOB)이 HEAD = 승인 sha일 때 연다.
	// 통과는 아무것도 안 쓰고 착지 줄(land --verdict)만, 지적은 <ID>.ack taken을 한 번. 표시 하나를 수거·착지·취소가 배타적으로 다툼
	VD := filepath.Join(k.T, ".local/share/hc-fill/verdicts/lane-v")
	mkdirs(k.t, VD)
	vh := k.git(k.R, "rev-parse", "HEAD")
	vput := func(id, run, sha, v string) { s20vput(k, VD, id, run, sha, v) }
	VW := filepath.Join(k.T, "vwt-lane-v")
	k.sh(k.R, `git worktree add -q --detach "$1" HEAD`, VW) // 레인은 늘 연결 작업 트리(hc-lane이 만든 것)에서 부른다
	venv := []string{"HC_TERMINAL_ID=t9vv", "HC_LANE=lane-v", "HC_LANE_JOB=rv-1"}
	vrun := func(a ...string) { k.runIn(VW, venv, append([]string{"verdict"}, a...)...) }
	ack := func(id string) string { return filepath.Join(VD, id+".ack") }
	zero := strings.Repeat("0", 40) // 40자 0을 이어 만든다(공개 검사의 16진 꼴에 안 걸리게)
	// fill-park-readopt(지휘 t30x, 2026-11-21 travel-camera-moves): 병합기 제출 뒤 작업 트리를 지운 레인이 메인 폴더에서 verdict를 부르니 메인
	// HEAD를 레인 커밋으로 읽어 「착지·고침 안 함 → report 완료 <메인 sha>」 엉뚱한 지시를 냈다 → 작업 트리 없음으로 끝낸다(아무것도 안 씀)
	vput("a6gone", "rv-1", zero, "pass")
	k.runIn(k.R, venv, "verdict", "a6gone")
	k.check("verdict: 레인 작업 트리가 아닌 곳(메인 폴더)이면 메인 HEAD로 짐작하지 않고 「작업 트리 없음 — 제출·착지했으면 수거 불필요」, 표시 없음",
		k.rc == 0 && !exists(ack("a6gone")) && strings.Contains(k.out, "작업 트리 없음 — 제출·착지했으면 수거 불필요") && !strings.Contains(k.out, "report 완료"))
	k.runIn(k.T, venv, "verdict", "a6gone")
	k.check("verdict: git 밖(지운 작업 트리 자리 등)이어도 「작업 트리 없음」(종료 0, 다시 부르라 하지 않음)", k.rc == 0 && strings.Contains(k.out, "작업 트리 없음"))
	vput("a1pass", "rv-1", vh, "pass")
	vput("a0other", "rv-0", vh, "pass")
	vrun("a1pass")
	k.check("verdict 통과: 착지 줄(land main --verdict a1pass)만 찍고 아무것도 안 씀", k.rc == 0 && !exists(ack("a1pass")) && strings.Contains(k.out, "hc-lane land main --verdict a1pass"))
	vrun("a0other")
	k.check("verdict: 다른 실행의 판정은 손대지 않음(종료 1, 표시 없음)", k.rc == 1 && !exists(ack("a0other")) && strings.Contains(k.out, "다른 실행"))
	vrun()
	k.check("verdict: 판정 ID 없이는 거절(종료 2 — 지시 폴더를 훑지 않는다)", k.rc == 2)
	vrun("a9none")
	k.check("verdict: 없는 판정은 종료 1", k.rc == 1)
	vput("a2stale", "rv-1", zero, "fix")
	vrun("a2stale")
	k.check("verdict: HEAD ≠ 승인 sha → 아무것도 안 쓰고 「지금 커밋으로 완료」", k.rc == 0 && !exists(ack("a2stale")) && strings.Contains(k.out, "report 완료"))
	vput("a3fix", "rv-1", vh, "fix")
	k.runIn(VW, []string{"HC_TERMINAL_ID=t9vv", "HC_LANE=lane-v"}, "verdict", "a3fix")
	k.check("verdict: HC_LANE_JOB 없고 hc get created_ms도 없으면(모름) 종료 75, 표시 없음", k.rc == 75 && !exists(ack("a3fix")))
	var wg sync.WaitGroup
	for p := 1; p <= 5; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			k.s20comb(VW, filepath.Join(k.T, fmt.Sprintf("vpar.%d", p)), venv, "verdict", "a3fix")
		}(p)
	}
	wg.Wait()
	same, tmp := 0, false
	for p := 1; p <= 5; p++ {
		for _, l := range strings.Split(read(filepath.Join(k.T, fmt.Sprintf("vpar.%d", p))), "\n") {
			if strings.Contains(l, "감사 지적 1번째 a3fix") {
				same++
			}
		}
	}
	if es, err := os.ReadDir(VD); err == nil {
		for _, e := range es {
			tmp = tmp || strings.HasPrefix(e.Name(), ".ack.")
		}
	}
	k.check("verdict 지적: 동시 다섯 번 → 표시 하나(taken), 모두 같은 지적 지시, 임시 파일 안 남음", s20cutFile(ack("a3fix"), 1) == "taken" && same == 5 && !tmp)
	a3 := strings.TrimRight(read(ack("a3fix")), "\n")
	vrun("a3fix")
	k.check("verdict 지적: 다시 부르면 「이미 받음」·같은 지시, 표시 그대로", k.rc == 0 && strings.TrimRight(read(ack("a3fix")), "\n") == a3 &&
		strings.Contains(k.out, "이미 받음 a3fix") && strings.Contains(k.out, "감사 지적"))
	vput("a4void", "rv-1", vh, "fix")
	write(k.t, ack("a4void"), "void\t2026\n")
	vrun("a4void")
	k.check("verdict: hc-fill이 취소(void)한 판정은 아무것도 안 함", k.rc == 0 && s20cutFile(ack("a4void"), 1) == "void" && strings.Contains(k.out, "취소") && !strings.Contains(k.out, "감사 지적"))
}

func s20LandVerdict(k *K) {
	// fill-handoff-integrity: land --verdict <ID> — 승인 변경(patch-id)만, 차이가 rebase뿐일 때 push. 응답 유실은 다시 부르면 끝냄
	oD, oR, oB := k.D, k.R, k.B // 아래 절은 t20 저장소를 쓰니 이 시험 뒤 되돌린다
	k.mk("tvd")
	lv := func(id string) {
		k.runIn(k.R, []string{"HC_LANE=lane-v", "HC_LANE_JOB=rv-1"}, "land", "main", "--verdict", id)
	}
	VD := filepath.Join(k.T, ".local/share/hc-fill/verdicts/lane-v")
	os.RemoveAll(VD)
	mkdirs(k.t, VD)
	vput := func(id, run, sha, v string) { s20vput(k, VD, id, run, sha, v) }
	ack := func(id string) string { return filepath.Join(VD, id+".ack") }
	og := k.D + "/origin.git"
	omain := func() string { return k.git(og, "rev-parse", "main") }
	k.sh(k.R, `mkdir -p .claude && printf '#!/bin/bash\n[ ! -e "$HOME/verify.fail" ]\n' >.claude/herdr-verify.sh && printf 'lane\n' >lane.txt &&
  git add .claude/herdr-verify.sh lane.txt && git commit -qm "lane-v: 일"`)
	S1 := k.git(k.R, "rev-parse", "HEAD")
	k.seed("other.txt", "남의착지") // 기준이 앞서 나감 — rebase가 필요(내용 충돌 없음)
	vput("a5", "rv-1", S1, "pass")
	lv("a5")
	k.check("land --verdict: 차이가 rebase뿐이면 push, <ID>.done, 시작 표시 landing", k.rc == 0 && k.gitOK(og, "show", "main:lane.txt") && k.gitOK(og, "show", "main:other.txt") &&
		exists(filepath.Join(VD, "a5.done")) && s20cutFile(ack("a5"), 1) == "landing")
	os.Remove(filepath.Join(VD, "a5.done"))
	lv("a5")
	k.check("land --verdict: 끝 표시 전 응답 유실 꼴(이미 기준에 들어감)은 다시 부르면 끝 표시만", k.rc == 0 && exists(filepath.Join(VD, "a5.done")) && strings.Contains(k.out, "이미"))
	lv("a5")
	k.check("land --verdict: 끝난 판정을 다시 부르면 「이미 착지」(종료 0)", k.rc == 0 && strings.Contains(k.out, "이미 착지"))
	k.sh(k.R, `printf 'lane2\n' >lane2.txt && git add lane2.txt && git commit -qm "lane-v: 이어서"`)
	S2, o1 := k.git(k.R, "rev-parse", "HEAD"), omain()
	vput("a6", "rv-1", S2, "pass")
	write(k.t, filepath.Join(k.T, "verify.fail"), "")
	lv("a6")
	os.Remove(filepath.Join(k.T, "verify.fail"))
	k.check("land --verdict: 검증 실패면 push 안 함(시작 표시는 남음)", k.rc == 1 && s20cutFile(ack("a6"), 1) == "landing" && omain() == o1)
	k.sh(k.R, `printf 'fixed\n' >>lane2.txt && git commit -qam "lane-v: 검증 고침"`)
	lv("a6")
	k.check("land --verdict: 시작 뒤 내용을 바꾼 커밋(고침)은 patch-id가 달라 push 안 함(종료 2, 새 감사)", k.rc == 2 && strings.Contains(k.errf(), "승인 내용과 다르다") && omain() == o1)
	vput("a7", "rv-1", S2, "pass")
	lv("a7")
	k.check("land --verdict: 처음 부를 때 HEAD가 승인 커밋이 아니면 종료 2, 표시 없음", k.rc == 2 && strings.Contains(k.errf(), "승인 커밋이 아니다") && !exists(ack("a7")))
	S3 := k.git(k.R, "rev-parse", "HEAD")
	vput("a8", "rv-0", S3, "pass")
	lv("a8")
	k.check("land --verdict: 다른 실행의 판정이면 종료 2, 표시 없음", k.rc == 2 && !exists(ack("a8")))
	vput("a9", "rv-1", S3, "pass")
	write(k.t, ack("a9"), "void\tx\n")
	lv("a9")
	k.check("land --verdict: 취소된(void) 판정이면 종료 2, push 없음", k.rc == 2 && strings.Contains(k.errf(), "취소") && omain() == o1)
	vput("a10", "rv-1", S3, "fix")
	lv("a10")
	k.check("land --verdict: 지적 판정으로는 착지 안 함(종료 2)", k.rc == 2)
	// 위임 경로(.claude/herdr-land.sh — 문서 접기처럼 착지 스크립트가 내용을 더 바꿀 수 있다): 위임 전에 같은 기준으로 rebase해 대조하고
	// 맞으면 맡긴다(스크립트의 접기는 대조 뒤라 막지 않는다). 시작 뒤 승인 밖 커밋이 끼면 위임 전에 종료 2(스크립트 안 부름)
	k.sh(k.R, `printf '#!/bin/bash\n: >"$HOME/land.called"\n[ ! -e "$HOME/land.fail" ] || exit 1\ngit fetch -q origin && git rebase -q "origin/$1" && echo 접음 >>lane.txt && git commit -qam 접기 && git push -q origin "HEAD:refs/heads/$1"\n' >.claude/herdr-land.sh && git add .claude/herdr-land.sh && git commit -qm "lane-v: 위임"`)
	S4 := k.git(k.R, "rev-parse", "HEAD")
	vput("a11", "rv-1", S4, "pass")
	// 작업 중 발견해 함께 고침: 셸 원본 `seed other2.txt 또남의착지`는 seed 클론이 뒤처져 push가 거절됐다(셸은 실패를 안 봐 원격이
	// 안 움직였고, 위임 전 rebase를 실제로는 시험하지 않았다). 당겨 온 뒤 올려 진짜 「남의 착지」를 만든다
	k.seedp("other2.txt", "또남의착지")
	called := filepath.Join(k.T, "land.called")
	os.Remove(called)
	lv("a11")
	lane, _ := k.exe(k.T, "", nil, "git", "-C", og, "show", "main:lane.txt")
	k.check("land --verdict 위임: 위임 전 rebase·대조가 맞으면 맡김(스크립트의 접기 포함 push), <ID>.done", k.rc == 0 && exists(called) && strings.Contains(lane, "접음") && exists(filepath.Join(VD, "a11.done")))
	k.sh(k.R, `git fetch -q origin && git reset -q --hard origin/main && printf 'x\n' >w.txt && git add w.txt && git commit -qm "lane-v: 다음"`)
	S5 := k.git(k.R, "rev-parse", "HEAD")
	vput("a12", "rv-1", S5, "pass")
	o2 := omain()
	write(k.t, filepath.Join(k.T, "land.fail"), "")
	lv("a12")
	os.Remove(filepath.Join(k.T, "land.fail"))
	k.sh(k.R, `printf 'y\n' >>w.txt && git commit -qam "lane-v: 승인 뒤 고침"`)
	os.Remove(called)
	lv("a12")
	k.check("land --verdict 위임: 시작 뒤 승인 밖 커밋이 끼면 위임 전에 종료 2(스크립트 안 부름, push 없음)", k.rc == 2 && !exists(called) && omain() == o2)
	os.RemoveAll(filepath.Join(k.T, ".local/share/hc-fill"))
	k.D, k.R, k.B = oD, oR, oB
}
