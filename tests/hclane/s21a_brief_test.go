package hclane_test

// tests/hc-lane.test.sh 21절 앞(1803–1935줄): --brief(지시문 사본·{brief}), 신뢰 질문 자동 넘김, 외부 가져오기 질문,
// --read 하위 폴더·저장소 아닌 폴더, 시작 확인(종료 5·6·7), --start-wait·lane.start_wait.

import (
	"os"
	"strings"
)

func init() { reg(100, "s21a1", s21aBrief) }

// s21aBD: 셸 $BD(지시문 폴더).
func (k *K) s21aBD() string { return k.W + "/지시 폴더" }

// s21aKeys: $(cat "$HCLOG.keys").
func (k *K) s21aKeys() string { return strings.TrimRight(read(k.HCLOG+".keys"), "\n") }

// s21aRmKeys: rm -f "$HCLOG.keys".
func (k *K) s21aRmKeys() { os.Remove(k.HCLOG + ".keys") }

// s21aLine: 그 줄이 그대로 있다(grep -Fx).
func s21aLine(content, l string) bool { return contains(strings.Split(content, "\n"), l) }

// s21aPairIn: 줄 목록에서 옵션 다음 줄이 그 값(grep -Fx -A1 opt | grep -Fx val).
func s21aPairIn(l []string, opt, val string) bool {
	for i, s := range l {
		if s == opt && i+1 < len(l) && l[i+1] == val {
			return true
		}
	}
	return false
}

// s21aCountF: 그 글이 든 줄 수(grep -cF).
func s21aCountF(content, sub string) int {
	n := 0
	for _, l := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		if content != "" && strings.Contains(l, sub) {
			n++
		}
	}
	return n
}

// s21aCountRe: 정규식이 걸린 줄 수(grep -c).
func s21aCountRe(content, re string) int {
	n := 0
	for _, l := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		if content != "" && grep(l, re) {
			n++
		}
	}
	return n
}

// s21aCut: cut -f<from>-<to>(줄마다, 탭 없는 줄은 그대로 — cut 꼴). 결과는 $(…) 꼴(끝 줄바꿈 없음).
func s21aCut(content string, from, to int) string {
	if content == "" {
		return ""
	}
	var o []string
	for _, l := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		f := strings.Split(l, "\t")
		if len(f) == 1 {
			o = append(o, l)
			continue
		}
		var g []string
		for i := from; i <= to && i <= len(f); i++ {
			g = append(g, f[i-1])
		}
		o = append(o, strings.Join(g, "\t"))
	}
	return strings.Join(o, "\n")
}

// s21aTrust: 셸 trust_screen <No 줄 앞> <Yes 줄 앞>(2026-11-16 실제 Claude 2.1 화면 글자).
func s21aTrust(sc, c1, c2 string) {
	os.WriteFile(sc, []byte(strings.Join([]string{" Accessing workspace:", " /x",
		" Quick safety check: Is this a project you created or one you trust? (Like your own code)",
		" Security guide", " " + c1 + " No, exit", " " + c2 + " Yes, I trust this folder", " Enter to confirm · Esc to cancel"}, "\n")+"\n"), 0o644)
}

// s21aImp: 셸 imp_screen <No 줄 앞> <Yes 줄 앞> <경로 줄…>(2026-11-17 Claude 2.1.326 화면 글자).
func s21aImp(sc, c1, c2 string, paths ...string) {
	var b strings.Builder
	for _, l := range []string{" Allow external CLAUDE.md file imports?", "",
		" This project's CLAUDE.md or .claude/rules imports files outside the current working directory. Never allow this for third-party repositories.",
		"", " External imports:"} {
		b.WriteString(l + "\n")
	}
	for _, p := range paths {
		b.WriteString("   " + p + "\n")
	}
	for _, l := range []string{"", " Important: Only use Claude Code with files you trust. Accessing untrusted files may pose security risks", "",
		" " + c1 + " No, disable external imports", " " + c2 + " Yes, allow external imports"} {
		b.WriteString(l + "\n")
	}
	os.WriteFile(sc, []byte(b.String()), 0o644)
}

func s21aBrief(k *K) {
	// 21. --brief(지시문 사본·{brief}), 신뢰 질문 자동 넘김, 시작 확인(종료 5·6·7), fetch(로그인 셸 꼴)
	k.mk("t18")
	R, T := k.R, k.T
	BD := k.s21aBD()
	write(k.t, BD+"/BRIEF.md", "# 할 일\n본문 줄\n")
	k.run("--brief", BD+"/BRIEF.md", "--name", "lane-b", "--repo", R, "--", "echo", "{brief} 를 읽어라")
	k.check("--brief: 종료 0", k.rc == 0)
	run := read(BD + "/BRIEF-run.md")
	k.check("--brief: 사본은 지시문 옆 BRIEF-run.md, 본문+착지 절차(작업 폴더)", s21aLine(run, "본문 줄") && strings.Contains(run, "### 착지·정리") && strings.Contains(run, R+".wt/lane-b"))
	k.check("--brief: 실행 명령의 {brief} 자리에 사본 경로", k.has(BD+"/BRIEF-run.md 를 읽어라"))
	k.check("--brief: 레인 환경 HC_LANE·라벨도 그대로", k.has("HC_LANE=lane-b") && k.pair("--label", "hc-lane=1"))
	k.check("--brief: 지시문 원본은 그대로", strings.TrimRight(read(BD+"/BRIEF.md"), "\n") == "# 할 일\n본문 줄")
	k.conf("lane.brief_dir ~/briefs")
	k.run("--on", "box", "--brief", BD+"/BRIEF.md", "--name", "lane-bo", "--repo", R, "--", "echo", "{brief}")
	ex := k.hclog(".exec")
	k.check("--brief --on: 설정 lane.brief_dir(그 기계 ~)에 <레인>-run.md, hc exec -i --on box로", k.rc == 0 &&
		strings.Contains(read(T+"/briefs/lane-bo-run.md"), "본문 줄") && s21aPairIn(ex, "--on", "box") && contains(ex, "-i"))
	k.check("--brief --on: 실행 명령에 그 기계 경로", k.has(T+"/briefs/lane-bo-run.md"))
	k.conf(`lane.run printf "%s\n" "읽고 진행: {brief}" x`)
	k.run("--brief", BD+"/BRIEF.md", "--name", "lane-br", "--repo", R)
	k.check("--brief: -- 뒤 명령이 없으면 설정 lane.run 틀", k.rc == 0 && k.has("읽고 진행: "+BD+"/BRIEF-run.md") && k.has(`%s\n`) && k.has("x"))
	k.noconf()
	k.run("--brief", BD+"/BRIEF.md", "--name", "lane-bn", "--repo", R)
	k.check("--brief: 명령도 lane.run도 없으면 오류, 아무것도 안 만듦", k.rc != 0 && strings.Contains(k.errf(), "lane.run") && !exists(R+".wt/lane-bn"))
	k.run("--brief", BD+"/BRIEF.md", "--name", "lane-bx", "--repo", R, "--", "echo", "hi")
	k.check("--brief: 실행 명령에 {brief} 자리가 없으면 오류, 아무것도 안 만듦", k.rc != 0 && strings.Contains(k.errf(), "{brief}") && !exists(R+".wt/lane-bx"))
	k.run("--name", "lane-by", "--repo", R, "--", "echo", "{brief}")
	k.check("{brief}가 있는데 --brief가 없으면 오류", k.rc != 0 && strings.Contains(k.errf(), "--brief") && !exists(R+".wt/lane-by"))
	k.run("--brief", BD+"/없음.md", "--name", "lane-bz", "--repo", R, "--", "echo", "{brief}")
	k.check("--brief: 지시문 파일이 없으면 오류", k.rc != 0 && !exists(R+".wt/lane-bz"))
	write(k.t, BD+"/AUDIT.md", "# 감사\n")
	k.run("--read", "--brief", BD+"/AUDIT.md", "--name", "lane-ba", "--repo", R, "--", "echo", "{brief}")
	au := read(BD + "/AUDIT-run.md")
	k.check("--read --brief: 사본에 착지 절차 대신 판정 알림, 경로가 명령에", k.rc == 0 && s21aLine(au, "# 감사") && !strings.Contains(au, "착지·정리") &&
		strings.Contains(au, "판정 알림") && k.has(BD+"/AUDIT-run.md"))
	write(k.t, BD+"/DRY.md", "# 마른\n")
	k.run("--dry-run", "--brief", BD+"/DRY.md", "--name", "lane-bd", "--repo", R, "--", "echo", "{brief}")
	k.check("--brief --dry-run: 사본을 쓰지 않고 할 일만", k.rc == 0 && !exists(BD+"/DRY-run.md") && strings.Contains(k.out, "지시문 사본") && !exists(R+".wt/lane-bd"))

	// 신뢰 질문(2026-11-16 실제 Claude 2.1 화면 글자)
	SC := T + "/screen"
	s21aTrust(SC, "❯", " ")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=untilkeys"}, "--name", "lane-tr", "--repo", R, "--", "claude")
	k.check("신뢰 질문: 아래·엔터로 넘기고 status가 잡혀 종료 0", k.rc == 0 && k.s21aKeys() == "t9zz down enter" && strings.Contains(k.out, "신뢰 질문 넘김"))
	s21aTrust(SC, " ", "❯")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=untilkeys"}, "--name", "lane-tr2", "--repo", R, "--", "claude")
	k.check("신뢰 질문: 이미 신뢰에 커서면 엔터만", k.rc == 0 && k.s21aKeys() == "t9zz enter")
	// 2026-11-28 ht-revive-cmd 실측: Claude가 신뢰 창에서도 status(blocked)를 내 「시작 확인」이 바로 통과해 창을 넘기지 않았다(레인이 창에서 멈춤)
	s21aTrust(SC, "❯", " ")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC}, "--name", "lane-trs", "--repo", R, "--", "claude")
	k.check("신뢰 질문: status가 이미 잡혀도 화면이 신뢰 질문이면 넘긴다", k.rc == 0 && k.s21aKeys() == "t9zz down enter" && strings.Contains(k.out, "신뢰 질문 넘김"))
	s21aTrust(SC, " ", " ")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=untilkeys"}, "--start-wait", "1", "--name", "lane-tr3", "--repo", R, "--", "claude")
	k.check("신뢰 질문: 어느 줄에도 커서(❯)가 없으면 키를 안 보내고 「모름」(감사 [낮음] 3)", k.rc == 5 && !exists(k.HCLOG+".keys") && strings.Contains(k.errf(), "모름 — 시작 화면 확인"))
	k.runE([]string{"HC_FAKE_NOSTATUS=1"}, "--start-wait", "0", "--name", "lane-sw0", "--repo", R, "--", "./script.sh")
	k.check("--start-wait 0: 이 호출만 시작 확인 끔(설정보다 앞섬, status 없는 명령용)", k.rc == 0 && strings.Contains(lastLine(k.out), `"start":"off"`))
	k.run("--start-wait", "1분", "--name", "lane-swx", "--repo", R, "--", "claude")
	k.check("--start-wait가 숫자가 아니면 띄우기 전에 오류(알 수 없는 인자가 아니라 값 검사)", k.rc != 0 && strings.Contains(k.errf(), "--start-wait는 0 이상 정수") && !exists(R+".wt/lane-swx"))
	k.conf("lane.start_wait 1")
	write(k.t, SC, " Accessing workspace:\n Do you trust the files in this folder?\n ❯ 1. Yes, proceed\n   2. No\n")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=1"}, "--name", "lane-tu", "--repo", R, "--", "claude")
	k.check("신뢰 질문 글자가 다르면 안 건드리고 「모름 — 시작 화면 확인」, 종료 5", k.rc == 5 && !exists(k.HCLOG+".keys") && strings.Contains(k.errf(), "모름 — 시작 화면 확인"))
	k.check("시작 실패에도 JSON 끝 줄에 주소와 start", strings.Contains(lastLine(k.out), `"lane":"t9zz"`) && strings.Contains(lastLine(k.out), `"start":"no_status"`))
	write(k.t, SC, "Welcome\n> \n")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=1"}, "--name", "lane-ns", "--repo", R, "--", "claude")
	k.check("status가 안 잡히면 종료 5, 키는 안 보냄", k.rc == 5 && !exists(k.HCLOG+".keys") && strings.Contains(k.errf(), "status"))
	write(k.t, SC, "Claude Code\n\n  Login expired · Please run /login\n\n")
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=1"}, "--on", "box", "--name", "lane-lg", "--repo", R, "--", "claude")
	k.check("로그인 만료: 종료 6, 그 줄을 알림", k.rc == 6 && strings.Contains(k.errf(), "로그인") && strings.Contains(k.errf(), "Login expired") && strings.Contains(lastLine(k.out), `"start":"login"`))
	write(k.t, SC, "security: SecKeychainCopyDefault: User interaction is not allowed.\nNot logged in\n")
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=1"}, "--name", "lane-kc", "--repo", R, "--", "claude")
	k.check("키체인: 종료 7", k.rc == 7 && strings.Contains(k.errf(), "키체인"))
	k.runE([]string{"HC_FAKE_NOSTATUS=finished"}, "--name", "lane-fn", "--repo", R, "--", "claude")
	k.check("터미널이 끝나면 기다리지 않고 종료 5", k.rc == 5 && strings.Contains(k.errf(), "끝났다"))
	k.conf("lane.start_wait 1", "lane.auto_trust off")
	s21aTrust(SC, "❯", " ")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=1"}, "--name", "lane-to", "--repo", R, "--", "claude")
	k.check("lane.auto_trust off: 신뢰 질문을 안 건드리고 종료 5", k.rc == 5 && !exists(k.HCLOG+".keys") && strings.Contains(k.errf(), "신뢰 질문"))

	// 외부 가져오기 질문(2026-11-17 Claude 2.1.326 화면 글자: 경로 줄은 두 칸 들여쓰기, 기본 커서는 No)
	k.conf("lane.start_wait 1")
	s21aImp(SC, "❯", " ", R+".wt/lane-im/AGENTS.md", R+".wt/lane-im/docs/x.md")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=untilkeys"}, "--name", "lane-im", "--repo", R, "--", "claude")
	k.check("외부 가져오기: 경로가 모두 작업 폴더 저장소 안이면 아래·엔터로 허용, 종료 0", k.rc == 0 && k.s21aKeys() == "t9zz down enter" && strings.Contains(k.out, "외부 가져오기 허용"))
	s21aImp(SC, " ", "❯", "~/"+strings.TrimPrefix(R, T+"/")+".wt/lane-im2/AGENTS.md")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=untilkeys"}, "--name", "lane-im2", "--repo", R, "--", "claude")
	k.check("외부 가져오기: ~ 줄임 경로도 펼쳐 보고, 이미 Yes에 커서면 엔터만", k.rc == 0 && k.s21aKeys() == "t9zz enter")
	s21aImp(SC, " ", "❯", R+".wt/lane-imk/AGENTS.md")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_KEYS_SLEEP=2", "HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=untilkeys"}, "--name", "lane-imk", "--repo", R, "--", "claude")
	k.check("시작 확인: 키를 보낸 바퀴에 start_wait가 지나도 status를 한 번 더 본다(부하 흔들림, 종료 0)", k.rc == 0 && k.s21aKeys() == "t9zz enter")
	s21aImp(SC, "❯", " ", R+".wt/lane-im3/AGENTS.md", R+"/AGENTS.md")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=1"}, "--name", "lane-im3", "--repo", R, "--", "claude")
	imq := false // grep -F "외부 가져오기 질문(" err | grep -Fq "lane-im3/AGENTS.md"
	for _, l := range strings.Split(k.errf(), "\n") {
		if strings.Contains(l, "외부 가져오기 질문(") && strings.Contains(l, "lane-im3/AGENTS.md") {
			imq = true
		}
	}
	k.check("외부 가져오기: 밖 경로가 하나라도 있으면 안 고르고 「모름 — 외부 가져오기 질문(<경로>)」, 종료 5", k.rc == 5 && !exists(k.HCLOG+".keys") &&
		strings.Contains(k.errf(), "모름 — 외부 가져오기 질문("+R+"/AGENTS.md)") && !imq)
	s21aImp(SC, "❯", " ", R+".wt/lane-im4/AGENTS.md", "… +3 imports not shown.", "Yes covers those too, plus any this project adds later.")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=1"}, "--name", "lane-im4", "--repo", R, "--", "claude")
	k.check("외부 가져오기: 다 보이지 않는 목록(… +N not shown)은 안 고름, 종료 5", k.rc == 5 && !exists(k.HCLOG+".keys") && strings.Contains(k.errf(), "모름 — 외부 가져오기 질문("))
	s21aImp(SC, "❯", " ", R+".wt/lane-im5/../../../etc/x.md")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=1"}, "--name", "lane-im5", "--repo", R, "--", "claude")
	k.check("외부 가져오기: .. 로 빠져나가는 경로는 밖, 안 고름", k.rc == 5 && !exists(k.HCLOG+".keys"))
	k.conf("lane.start_wait 1", "lane.auto_trust off")
	s21aImp(SC, "❯", " ", R+".wt/lane-im6/AGENTS.md")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=1"}, "--name", "lane-im6", "--repo", R, "--", "claude")
	k.check("외부 가져오기: lane.auto_trust off면 안 고름, 종료 5", k.rc == 5 && !exists(k.HCLOG+".keys") && strings.Contains(k.errf(), "외부 가져오기 질문(lane.auto_trust off)"))
	k.conf("lane.start_wait 1")

	// --read: 저장소 안 하위 폴더(.qa/…)·저장소가 아닌 폴더도 그 폴더에서, 시작 확인도 받는다
	mkdirs(k.t, R+"/.qa/감사 1")
	s21aImp(SC, "❯", " ", R+"/AGENTS.md")
	os.Remove(k.HCLOG)
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=untilkeys"}, "--read", "--name", "lane-iq", "--repo", R+"/.qa/감사 1", "--", "claude")
	k.check("--read 하위 폴더: 그 폴더에서 띄우고 저장소 루트 가져오기는 허용", k.rc == 0 && k.pair("--cwd", R+"/.qa/감사 1") && k.has("LANE_REPO="+R) && k.s21aKeys() == "t9zz down enter")
	P := T + "/저장소 아닌 폴더"
	mkdirs(k.t, P)
	os.Remove(k.HCLOG)
	k.s21aRmKeys()
	k.run("--read", "--name", "lane-ip", "--repo", P, "--", "claude")
	k.check("--read 저장소 아닌 폴더: 그 폴더에서 띄움, LANE_BASE 빈 값, 종료 0", k.rc == 0 && k.pair("--cwd", P) && k.has("LANE_REPO="+P) && k.has("LANE_BASE="))
	s21aImp(SC, "❯", " ", P+"/AGENTS.md")
	k.s21aRmKeys()
	k.runE([]string{"HC_FAKE_SCREEN=" + SC, "HC_FAKE_NOSTATUS=1"}, "--read", "--name", "lane-ip2", "--repo", P, "--", "claude")
	k.check("--read 저장소 아닌 폴더: 가져오기 질문은 안 고름, 종료 5", k.rc == 5 && !exists(k.HCLOG+".keys") && strings.Contains(k.errf(), "저장소 아님"))
	k.run("--read", "--name", "lane-ix", "--repo", T+"/없는 폴더", "--", "claude")
	k.check("--read: 없는 폴더는 띄우기 전에 오류", k.rc == 1 && strings.Contains(k.errf(), "폴더가 없다"))
	k.conf("lane.start_wait 0")
	k.runE([]string{"HC_FAKE_NOSTATUS=1"}, "--name", "lane-s0", "--repo", R, "--", "claude")
	k.check("lane.start_wait 0: 시작 확인 끔, 종료 0", k.rc == 0 && strings.Contains(lastLine(k.out), `"start":"off"`))
	k.conf("lane.start_wait 다섯")
	k.run("--name", "lane-sx", "--repo", R, "--", "claude")
	k.check("lane.start_wait가 숫자가 아니면 띄우기 전에 오류", k.rc != 0 && strings.Contains(k.errf(), "lane.start_wait") && !exists(R+".wt/lane-sx"))
}
