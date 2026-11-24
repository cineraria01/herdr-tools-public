package main

// 띄우기 — 셸 판 lib/hc-fill/launch.sh 뒷부분(launch · place · early_sec · resume_sec). 지휘 기계 파일 붙이기는 paste.go

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/proc"
	"herdr-tools/internal/tri"
)

// launch <레인> <지시문> <기계> <저장소> <되돌릴곳> <주인 지휘|-> → ADDR. 0 띄움 · 1 실패(다음 기계) · 2 이 레인은 이번 바퀴에 그만 ·
// 3 미확정(주소 모름) · 4 입력 거절(hc-lane 종료 11 — 다시 해도 같음, 까닭 REJWHY) · 5 띄웠지만 시작 확인 실패(터미널은 남음) ·
// 6 띄웠고 주소는 있으나 시작 확인 미확정(hc-lane 종료 8)
func launch(lane, brief, m, repo, dest, owner string) int {
	LSSNAP = false // 새 주소가 생긴다 — 터미널 표의 「없음」을 더 믿지 않는다(lsGone)
	LFWHY = ""
	// 본문을 먼저 읽어 둔다: 못 읽으면 착지 줄만 든 빈 레인을 띄우지 않는다
	raw, err := os.ReadFile(brief)
	body := sub(string(raw))
	if err != nil || body == "" {
		why := "빈 파일"
		if err != nil {
			why = errText(err)
		}
		say("실패 %s: 지시문을 읽지 못함(%s): %s", lane, why, brief)
		return 2
	}
	on := ""
	if m != LOCAL {
		on = m
	}
	// 다른 기계에 띄울 때 본문이 가리키는 이 기계 붙일 폴더(설정 paste_dirs) 아래 파일 원문을 붙인다(Go 판 — 셸 판과 다름).
	// 받은편지 줄은 본문을 다른 기계가 썼다 — 이 기계 파일을 붙이지 않는다(gofix-fill F07)
	paste := ""
	if on != "" && dash(dest) == "-" {
		p, ok := lcPasteHome(body, dash(owner), lane)
		if !ok {
			return 2
		}
		paste = p
	}
	mm := dash(on)
	if on == "" {
		mm = LOCAL
	}
	// 받은편지 줄: 보고 경로에 요청 ID(되돌릴곳 #<ID>). 옛 줄(# 없음)은 <레인>.md
	rid := ""
	if i := strings.LastIndexByte(dest, '#'); i >= 0 && dash(dest) != "-" {
		rid = dest[i+1:]
	}
	rsec := ""
	if dash(dest) != "-" {
		rp := lane
		if rid != "" {
			rp += "." + rid
		}
		rsec = "\n## 결과 보고 (hc-fill 받은편지 레인)\n" +
			"끝나면(착지했든 못 했든) 이 기계 `~/.local/share/hc-fill/reports/" + rp + ".md`에 결과를 쓴다 — 같은 폴더의 다른 이름에 다 쓴 뒤\n" +
			"`mv`로 그 이름에 둔다(반쯤 쓴 파일이 보이지 않게). 마지막 줄은 셋 중 하나다:\n" +
			"`결과: 고침 <sha>` · `결과: 넘김 <사유>` · `결과: 실패 <사유>`. hc-fill은 마지막 줄이 이 꼴일 때만 요청한 기계로 되돌려 준다(착지·터미널 상태와 상관없이).\n"
	}
	// 자동 감사(설정 audit): 감사 줄은 --read, 쓰기 줄은 「감사·착지」 절을 붙이고 감사 칸 auto. 받은편지 줄은 지시문이 「hc-fill 감사」를
	// 요구하거나 설정 inbox_audit on일 때만(2026-11-21 — 받은편지 레인이 감사를 기다리며 9시간 멈춤)
	rd, asec := "", ""
	LAUTO = ""
	switch dash(AK) {
	case "-", "auto":
		if AUD != "" && (dash(dest) == "-" || strings.Contains(string(raw), "hc-fill 감사") || conf("inbox_audit") == "on") {
			asec, LAUTO = ASEC, "auto"
		}
	default:
		if strings.HasPrefix(AK, "audit=") {
			rd = "--read"
		}
	}
	sa := []string{"--print-steps"}
	if rd != "" {
		sa = append(sa, rd)
	}
	if on != "" {
		sa = append(sa, "--on", on)
	}
	so, rc := Cmd{Name: "hc-lane", Args: append(sa, "--name", lane, "--repo", repo), Err: errPass}.Run()
	if rc != 0 {
		say("실패 %s @%s: hc-lane --print-steps", lane, m)
		return 1
	}
	steps := sub(so)
	if rsec != "" && rid == "" && oldReport(m, lane) != 0 {
		say("실패 %s @%s: 그 기계의 옛 보고를 비켜 두지 못했다", lane, m)
		return 1
	}
	runmd := body + "\n\n" + steps + "\n" + rsec + asec + ESEC + RSEC + paste
	if AK == "no-audit" { // 2026-11-22 지휘 — 감사 없는 레인이 「감사 판정」을 기다리며 멈춘 일 넷. 새로·먼저·이어·다시 맞춤 모두 여기를 지난다
		runmd = strings.TrimRight(runmd, "\n") + "\n\n자동 감사 없음 — 완료 뒤 감사 판정을 기다리지 말고 바로 착지(hc-lane land)한다\n"
	}
	sz := strconv.Itoa(len(runmd)) // 쓴 run 지시문의 크기가 이것과 같아야 한다(본문이 다 들어갔는지)
	var rf string
	if on == "" {
		rf = HOME + "/.local/share/hc-fill/runs/" + lane + "-run.md"
		ok := os.MkdirAll(filepath.Dir(rf), 0o755) == nil && writeAtomic(rf, runmd) == nil
		if ok {
			st, err := os.Stat(rf)
			ok = err == nil && strconv.FormatInt(st.Size(), 10) == sz
		}
		if !ok {
			say("실패 %s @%s: run 지시문 쓰기", lane, m)
			return 1
		}
	} else {
		o, rc := Cmd{Name: "hc", Args: []string{"exec", "-i", "--on", on, "--", "sh", "-c",
			`d=$HOME/.local/share/hc-fill/runs && mkdir -p "$d" && cat >"$d/$1" && [ "$(wc -c <"$d/$1" | tr -d " ")" = "$2" ] && printf %s "$d/$1"`,
			"_", lane + "-run.md", sz}, In: strings.NewReader(runmd), Err: errPass}.Run()
		rf = sub(o)
		if rc != 0 || rf == "" {
			say("실패 %s @%s: run 지시문을 그 기계에 두지 못했다", lane, m)
			return 1
		}
	}
	own := dash(owner)
	if own == "-" {
		own = ""
	}
	// 주인 지휘가 지금 살아 있나: 그 사이 지휘가 끝났으면(없음이 확인됨 — ownerCheck) 주인 없이(-, 사건은 모든 지휘에게) 띄우고 watch는
	// 살아 있는 지휘에게. 그 밖(시간 넘김·연결 실패·살아 있으나 pid 없음)은 모름 — 주인을 풀지 않고 이번 바퀴는 미룬다(gofix-fill F06·L8, 재감사 R-09)
	if own != "" {
		switch st, why := ownerCheck(own); st {
		case tri.No:
			say("알림: %s 의 주인 지휘 %s 가 지금 없다 — 주인 없이 띄움(사건은 모든 지휘에게) (%s)", lane, own, why)
			own = ""
		case tri.Unknown:
			say("미룸 %s: 주인 지휘 %s 확인을 못 함 — 다음 바퀴에 (%s)", lane, own, why)
			return 2
		case tri.Yes:
		}
	}
	task := strings.ReplaceAll(TASK, "{run}", rf)
	args, ok := lcLaunchArgs(lane, brief, m, repo, dest, owner, task)
	if !ok {
		say("실패 %s @%s: 설정 launch 틀을 인자로 나누지 못함", lane, mm)
		return 1
	}
	// 지휘 주소: 주인 지휘가 있으면 그 지휘, 없으면 hc-lane coord가 확인한 지휘. 안 맞으면 넘기지 않고 한 줄
	wco := own
	if own == "" {
		o, rc := Cmd{Name: "hc-lane", Args: []string{"coord"}, Err: errMerge}.Run()
		wco = sub(o)
		// 지휘가 있는지 모름(시간 넘김·종료 1 밖·「확인 못 했다」) — 감시 없이 띄우지 않고 다음 바퀴에(재감사 R-09). 없음은 종료 1의 「지휘 없음」만
		if rc == proc.TimeoutCode || (rc != 0 && (rc != 1 || strings.Contains(wco, "확인 못"))) {
			say("미룸 %s: 지휘 확인(hc-lane coord)을 못 함(종료 %d) — 다음 바퀴에 (%s)", lane, rc, lastLine(wco))
			return 2
		}
		if rc != 0 {
			say("알림: 지휘 없음 — 감시(hc-lane watch)를 띄우지 않음, %s 사건은 events에 남아 지휘가 wait/watch를 걸면 받는다 (%s)", lane, lastLine(wco))
			wco = ""
		}
	}
	// 시도 기록: 이름·줄 ID·기계·먼저·감사를 띄우기 전에. 채택이 같은 줄 ID·기계면 이것을 쓴다
	if RESUME {
		EARLYF = "resumed"
	}
	// 주소 칸 ?: 미확정 — 띄우기 결과를 알기 전에 죽거나 시간을 넘기면 그대로 남아 다음 바퀴가 이 기계만 본다(tryUnsure)
	if !dry && tryput(lane, RID, mm, dash(EARLYF), dash(LAUTO), "?", "") != 0 {
		return 2
	}
	if rd == "" {
		ownAdd(lane, proj, mm, "pending:"+mm, pst_, SB, PAFT, PSOLO)
		ownStore()
	}
	la := []string{}
	if rd != "" {
		la = append(la, rd)
	}
	if RESUME {
		la = append(la, "--resume")
	}
	if on != "" {
		la = append(la, "--on", on)
	}
	la = append(append(la, "--name", lane, "--repo", repo, "--"), args...)
	env := []string{"HC_LANE_COORD=" + wco, "HC_LANE_OWNER=" + own, "HC_LANE_FILL=1", "HERDR_LAND_PRIORITY=" + strconv.Itoa(wcnt(lane))}
	// 제출 우선: 대기열 줄의 우선을 레인 환경 HC_LANE_PRIORITY로(2026-11-22 fill-prio-pass — 긴급 줄이 보통으로 제출돼 큐 뒤에 섰다).
	// 레인이 land 줄에 따로 주면 그 값. 감사 줄(--read)은 착지하지 않으니 넘기지 않는다
	if rd == "" {
		env = append(env, "HC_LANE_PRIORITY="+plabel(PRIO))
	}
	out, lr := Cmd{Name: "hc-lane", Args: la, Err: errMerge, Env: env}.Run()
	out = sub(out)
	crash("after-launch")
	// 5·6·7: 띄웠지만 시작 확인 실패(터미널은 남음) → 주소를 받아 「상태 안 잡힘」 길로. 8: 시작 확인 미확정 → 주소를 받아 미확정 길로(startState 2와 같게).
	// 시간 넘김은 모름 — 이미 시작했을 수 있다: 다른 기계에 또 띄우지 않고 시도 기록(?)·pending 소유를 남긴다(gofix-fill F01·F2, 재감사2 fill/R-201)
	switch lr {
	case 0, 5, 6, 7, 8:
	case proc.TimeoutCode:
		say("모름 %s @%s: hc-lane 띄우기 시간 넘김 — 시작했는지 몰라 다른 기계에 띄우지 않음(다음 바퀴에 그 기계를 본다)", lane, mm)
		LFWHY = "hc-lane 띄우기 시간 넘김 @" + mm
		return 3
	case rejHC: // 입력 거절 — 아무것도 안 만들었다(확정 실패). 다른 기계·다음 바퀴에 되풀이하지 않게 place가 줄을 멈춘다
		tryMark(lane, mm, "")
		REJWHY = runeCut(strings.TrimPrefix(lastLine(out), "hc-lane: "), 200)
		return 4
	default:
		LFWHY = runeCut(fmt.Sprintf("hc-lane 종료 %d @%s: %s", lr, mm, strings.TrimSpace(lcTail3(out))), 200)
		if launchUnsure(lr, out) { // 발사가 나갔을 수 있다 — 확정 실패가 아니라 미확정(?)(재감사 R-01)
			say("모름 %s @%s: hc-lane 띄우기 결과 미확정(종료 %d) — 시작했는지 몰라 다른 기계에 띄우지 않음(다음 바퀴에 그 기계를 본다): %s", lane, mm, lr, lcTail3(out))
			return 3
		}
		say("실패 %s @%s: hc-lane: %s", lane, mm, lcTail3(out))
		tryMark(lane, mm, "")
		return 1
	}
	SWHY = ""
	if lr != 0 {
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, "hc-lane: 시작 안 됨") {
				SWHY = runeCut(l, 200)
				break
			}
		}
	}
	last := lastLine(out)
	ADDR = ""
	// 끝 줄 전체가 JSON 객체 하나이고 주소가 빈 글이 아니며 이름이 이 레인일 때만(잘린 뒷부분·다른 꼴은 못 읽음 — 재감사 R-05)
	if vs, ok := decodeAll(last); ok && len(vs) == 1 && jget(vs[0], "name") == any(lane) {
		if a, isStr := jget(vs[0], "lane").(string); isStr {
			ADDR = a
		}
	}
	if ADDR == "" { // 띄웠다고 답했는데 주소가 없다 — 시작했을 수 있으니 시간 넘김과 같이 미확정
		say("모름 %s @%s: hc-lane 출력에서 주소를 못 읽음 — 다른 기계에 띄우지 않음: %s", lane, mm, last)
		return 3
	}
	// 원격은 hc-lane이 맥 창의 그 기계 공간에 탭으로 붙인다. 못 붙였으면 그 알림 줄을 그대로
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "hc-lane: Herdr 창") {
			say("알림: %s", strings.TrimPrefix(l, "hc-lane: "))
		}
	}
	bco := own // 로컬 레인은 주인 지휘 옆에(없으면 설정 coord)
	if bco == "" {
		bco = COORD
	}
	if on == "" && bco != "" {
		if _, err := exec.LookPath("herdr-balance"); err == nil {
			for i := 0; ; {
				// 상한 안에서만(proc.Limit이 모르는 명령 — Fable F3·아스트라 E01: 멈추면 run.lock을 쥔 채 끝없이 기다렸다).
				// RunMax: 고아 기록이 쓰인 뒤에야 시작해 이 바퀴가 SIGKILL로 죽어도 다음 바퀴가 거둔다(재감사 fill R-06)
				tickCall("herdr-balance", nil)
				bc := exec.Command("herdr-balance", "--main", bco, "--add", ADDR)
				if proc.Code(proc.RunMax(bc, balanceMax())) == 0 {
					break
				}
				if i++; i >= 5 {
					say("알림: 분할 붙이기 실패 %s (herdr-balance --main %s --add %s)", ADDR, bco, ADDR)
					break
				}
				time.Sleep(2 * time.Second)
			}
		}
	}
	switch lr {
	case 0:
		return 0
	case 8:
		return 6 // 띄운 주소는 있고 시작 확인이 미확정
	}
	return 5
}

// launchUnsure: hc-lane 띄우기의 실패 종료 가운데 발사가 나갔을 수 있는 것(참). 확정 실패는 hc run 전의 거절·실패
// (종료 1·2·10, 못 띄움 127)뿐이고, hc-lane이 hc run 뒤 결과를 모른다고 한 1(「hc run 결과를 확인하지 못했다」)·바깥 상한(「결과 모름」)·
// 갑작스런 끝(panic·신호·그 밖의 종료)은 미확정이다
func launchUnsure(rc int, out string) bool {
	switch rc {
	case 1, 2, 10, 127:
	default:
		return true
	}
	return strings.Contains(out, "hc run 결과를 확인하지 못했다") || strings.Contains(out, "결과 모름") || strings.Contains(out, "panic:")
}

// ownerCheck <주인 지휘>: 있음 살아 있음, 없음 확인, 모름(까닭 why). hc-lane coord --check 1은 「없거나 끝남」과 「살아 있으나 pid 없음」이
// 같은 꼴이라 그대로 믿지 않는다 — 다른 실행(pid·이름 다름)이거나 hc get이 끝남(not_found·finished)을 확인했을 때만 없음(재감사 R-09)
func ownerCheck(own string) (tri.State, string) {
	ce, rc := Cmd{Name: "hc-lane", Args: []string{"coord", "--check", own}, Err: errMerge}.Run()
	why := lastLine(sub(ce))
	switch {
	case rc == 0:
		return tri.Yes, ""
	case rc != 1:
		return tri.Unknown, fmt.Sprintf("종료 %d %s", rc, why)
	case strings.Contains(why, "지휘 정보(") && strings.Contains(why, "와 다름"),
		strings.Contains(why, "에이전트가 끝나 셸만 남음"): // 대화형 셸만 남은 지휘(hc-lane coordCheck — watch-agent-guard)
		return tri.No, why
	}
	pr, pn := PRUN, PNAME // pget이 채우는 실행 키는 부른 쪽 것 그대로
	gone := pget(own) == pDone
	PRUN, PNAME = pr, pn
	if gone {
		return tri.No, why
	}
	return tri.Unknown, why + " — hc get은 끝남을 확인하지 못함"
}

// balanceMax: herdr-balance 한 번의 상한 — HC_FILL_BALANCE_TIMEOUT초(빼면 30)
func balanceMax() time.Duration {
	if v := os.Getenv("HC_FILL_BALANCE_TIMEOUT"); isDigits(v) {
		return time.Duration(atoi10(v)) * time.Second
	}
	return 30 * time.Second
}

// tryMark: 이번 시도 기록의 주소 칸을 고친다(""=확정 실패 -, 주소=떴지만 상태 안 잡힘)
func tryMark(lane, m, addr string) {
	if !dry && tryput(lane, RID, m, dash(EARLYF), dash(LAUTO), addr, "") != 0 {
		say("경고 %s: 시도 기록을 고치지 못함", lane)
	}
}

// lcTail3: printf '%s\n' "$out" | tail -n 3 | tr '\n' ' '
func lcTail3(s string) string {
	l := strings.Split(s, "\n")
	if len(l) > 3 {
		l = l[len(l)-3:]
	}
	return strings.Join(l, " ") + " "
}

// lcLaunchArgs: 설정 launch 틀의 {task}를 "$task"로 바꿔 셸이 낱말을 나누게 한다(설정 파일은 믿는다 — 셸 판 eval "set -- …"와 같은 argv).
// $1~$6은 셸 판 launch의 위치 인자와 같다
func lcLaunchArgs(lane, brief, m, repo, dest, owner, task string) ([]string, bool) {
	tmpl := strings.ReplaceAll(LAUNCH, "{task}", `"$task"`)
	out, rc := Cmd{Name: "sh", Args: []string{"-c", `task=$7; eval "set -- $8" || exit 1; [ $# -eq 0 ] || printf '%s\0' "$@"`,
		"_", lane, brief, m, repo, dest, owner, task, tmpl}, Err: errPass}.Run()
	if rc != 0 {
		return nil, false
	}
	if out == "" {
		return nil, true
	}
	return strings.Split(strings.TrimSuffix(out, "\x00"), "\x00"), true
}

// place <레인> <프로젝트> <지시문> <기계제한> <되돌릴곳> <주인> [<선행 — 먼저 띄움일 때>]: 자리가 남는 기계 가운데 코어당 부하가
// 낮은 기계부터(모르거나 비슷하면 machine 차례 — steer, loadsteer.go. 자동 감사 줄은 machine 차례 그대로).
// placed 띄움 · bad 지시문을 못 읽어 이 바퀴엔 그만. stuck을 고쳐 적는다. 쓰기 줄은 터미널 주소를 받은 때 실행 기록에 적고 GOTADDR
func place(lane, p, brief, only, reply, owner, after string) {
	placed, bad, GOTADDR = false, false, false
	if why := rejHeld(lane, owner, reply); why != "" { // 띄우기 거절로 멈춘 줄 — 지휘가 retry·edit·rm 할 때까지(reject.go)
		say("멈춤 %s: 띄우기 거절 — %s (hc-fill retry %s)", lane, why, lane)
		bad = true
		return
	}
	isa := strings.HasPrefix(dash(AK), "audit=")
	failed, full, n0 := false, "", len(NOW)
	defer func() {
		if placed || GOTADDR {
			rejOK(lane)
		} else if failed {
			rejFail(lane, LFWHY, owner, reply)
		}
		if isa && len(NOW) > n0 { // 감사가 떴다(또는 미확정으로 그 기계 자리를 찼다) — 잡아 둔 몫을 놓는다(겹쳐 세지 않음)
			delete(RSV, lane)
		} else if isa && full != "" && !placed && !bad && !failed {
			aseatWait(lane, full)
		}
	}()
	// 자리가 남는 후보(machine 차례) → 부하 차례·모두 바쁨 관문(자동 감사 줄은 빼고) → 차례대로 띄우기(placeCands, loadsteer.go)
	cands, repos, fm := placeCands(lane, p, only, isa)
	full = fm
	for _, mc := range cands {
		m, repo := mc.Name, repos[mc.Name]
		mwhy(m, "")
		if dry {
			pre, suf := "", ""
			if after != "" {
				pre, suf = "먼저 ", " 선행 "+after
			}
			say("%s%s띄울 %s → %s (%s)%s", P, pre, lane, m, repo, suf)
			NOW = append(NOW, m)
			placed, UPNOW = true, UPNOW+","+lane
			if !isa {
				NOWL = append(NOWL, m)
				ownAdd(lane, p, m, "dry:"+m+"/"+lane, utcnow(), brief, dash(PAFT), dash(PSOLO))
			}
			break
		}
		ADDR, pst_, EARLYF = "", utcnow(), ""
		if after != "" {
			EARLYF = "early"
		}
		lr := launch(lane, brief, m, repo, reply, owner)
		if lr == 4 { // 입력 거절: 다른 기계로 넘기지 않고 줄을 멈춘다
			rejHold(lane, m, REJWHY, owner, reply)
			bad = true
			break
		}
		failed = failed || lr == 1 || lr == 2 || lr == 3
		if lr == 2 {
			bad = true
			break
		}
		if lr == 3 { // 미확정: 그 기계 자리는 찬 것으로 보고 이 레인은 이번 바퀴에 그만
			NOW = append(NOW, m)
			bad = true
			break
		}
		if lr != 0 && lr != 5 && lr != 6 {
			continue
		}
		ss := tri.No // 5: 띄웠지만 시작 확인 실패
		switch lr {
		case 0:
			ss = startState(ADDR, lane)
		case 6: // 안쪽 hc-lane이 시작 확인 미확정(종료 8) — 확인된 시작 실패가 아니다(재감사2 fill/R-201)
			ss = tri.Unknown
		}
		if ss == tri.Unknown { // 시작 확인(hc get)을 못 함 — 띄운 주소가 있으니 시작했을 수 있다: 미확정(?)으로 두고 다른 기계에 또 띄우지 않는다(재감사 R-01·R-05)
			say("모름 %s @%s %s: 시작 확인(hc get)을 못 함 — 다른 기계에 띄우지 않음(다음 바퀴에 그 기계를 본다)", lane, m, ADDR)
			// 받은 주소를 시도 기록 8칸에 — 다음 바퀴가 hc get으로 확정한다(fill/R-4M01). 못 적으면 주소 모름 미확정 그대로
			if !dry && tryEdit(lane, func(r Row) Row {
				if r.F(trRID) == RID && r.F(trMach) == m && r.F(trAddr) == "?" {
					return r.Set(trGot, ADDR)
				}
				return r
			}) != 0 {
				say("경고 %s: 시도 기록에 받은 주소를 적지 못함", lane)
			}
			NOW = append(NOW, m)
			bad = true
			break
		}
		if !isa {
			ownBind(lane, "pending:"+m, ADDR)
			GOTADDR = true
		}
		NOW = append(NOW, m) // 상태가 안 잡혀도 자리는 차 있다
		if ss == tri.Yes {
			if !isa {
				NOWL = append(NOWL, m)
			}
			pe := ""
			if after != "" {
				pe = "early"
			}
			if RESUME {
				pe = "resumed" // 주차에서 이어 띄움은 resumed(다시 먼저 띄움 후보가 아님)
			}
			if tryput(lane, RID, m, pe, LAUTO, ADDR, START_RUN) != 0 {
				die("실행 기록 저장 실패")
			}
			if qset(true, RID, lane, "up", m, ADDR, stuck, pe, LAUTO) == 0 {
				if after != "" {
					say("먼저 띄움 %s → %s %s (%s, 선행 %s)", lane, m, ADDR, repo, after)
				} else {
					rs := ""
					if RESUME {
						rs = ", 주차한 가지 lane/" + lane + " 에서 이어"
					}
					say("띄움 %s → %s %s (%s%s)", lane, m, ADDR, repo, rs)
				}
			} else {
				say("줄 바뀜 %s: 띄우는 동안 대기열 줄이 빠지거나 다시 들어왔다 — 띄운 %s %s 은 옛 줄의 실행이라 새 줄에 적지 않음(지휘가 정리)", lane, m, ADDR)
			}
			placed, UPNOW = true, UPNOW+","+lane
			break
		}
		tryMark(lane, m, ADDR) // 미확정(?) 아님 — 터미널은 stuck으로 다룬다
		if stuck == "-" {
			stuck = m + "=" + ADDR
		} else {
			stuck += "," + m + "=" + ADDR
		}
		if qset(true, RID, lane, "wait", "-", "-", stuck, "", "") != 0 {
			say("줄 바뀜 %s: 상태 안 잡힘 %s %s 을 새 줄에 적지 않음(지휘가 정리)", lane, m, ADDR)
			break
		}
		why := SWHY
		if why == "" {
			why = fmt.Sprintf("%d초 안에 hc get status 없음", WAIT)
		}
		say("상태 안 잡힘 %s @%s %s: %s. 그 레인에서 %s 빼고 다음 기계로(그 터미널은 남겨 둠, 확인 뒤 지휘가 정리)", lane, m, ADDR, why, m)
	}
}

// early_sec <선행 목록> <기본 가지>: 먼저 띄움 레인의 run 지시문 끝에 붙일 절(셸 판 $(early_sec …) — 끝 줄바꿈 없음). 선행마다 판정 줄이 따로다.
// 기한은 고정 시각(지금 + 설정 early_wait시간). 제목 판정은 grep -q 대신 끝까지 읽고 >/dev/null — 이력이 파이프 버퍼보다 길면
// pipefail 셸에서 printf가 SIGPIPE로 끝나 들어간 커밋을 아직으로 봤다(go-followups)
func earlySec(after, base string) string {
	dl := time.Now().Unix() + int64(EWAIT)*3600
	dls := time.Unix(dl, 0).Format("2006-01-02 15:04")
	re1, re2 := `^([a-z]+(\([^)]*\))?!?: *)?`, `([^a-z0-9_-]|$)`
	fetch := ""
	if strings.HasPrefix(base, "origin/") {
		fetch = "git fetch -q origin && "
	}
	w := "until " + fetch + "subjects=$(git log " + base + " --format=%s)"
	for _, a := range words(after) {
		if _, op, at := strings.Cut(a, "@"); at && op != "" {
			// 다른 프로젝트 선행(<이름>@<프로젝트>, afterAt): 이 레인 저장소 이력에는 없다 — hc-fill이 일반 배치와 같은 해석으로 판정해
			// 이 기계에 써 주는 after.tsv의 「go 들어감」을 본다(재감사2 fill/R-217 — 이 저장소 기본 가지만 보아 영영 기다렸다)
			w += " &&\n  awk -F'\\t' '$1 == \"" + a + "\" && $2 == \"go\" && index($3, \"들어감(\") == 1 { f = 1 } END { exit !f }' \"$HOME/.local/share/hc-fill/after.tsv\""
			continue
		}
		st, ok := "", false
		if fetch != "" {
			st, ok = since(a)
		}
		if ok {
			w += " &&\n  { printf '%s\\n' \"$subjects\" | grep -E '" + re1 + a + re2 + "' >/dev/null || { o=$(git ls-remote origin 'refs/hc-landed/" + a +
				"/*') && printf '%s\\n' \"$o\" | awk '{ s = $2; sub(/^refs\\/hc-landed\\/" + a + "\\//, \"\", s); if (s ~ /^[0-9]+$/ && s + 0 >= " + st +
				") print $1 }' | ( while read -r s; do git merge-base --is-ancestor \"$s\" " + base + " && exit 0; done; exit 1 ); }; }"
		} else {
			w += " &&\n  printf '%s\\n' \"$subjects\" | grep -E '" + re1 + a + re2 + "' >/dev/null"
		}
	}
	now := time.Now().Format("2006-01-02 15:04")
	return "\n## 먼저 띄움 — 선행 착지 전에 띄웠다 (hc-fill, " + now + ")\n" +
		"자리가 비어 hc-fill이 이 레인을 선행 레인(" + strings.ReplaceAll(after, ",", ", ") + ")이 " + base + "에 들어가기 **전에** 띄웠다. 위 지시문의 본 작업은 선행이 들어간 뒤에 한다.\n" +
		"1. 지금은 준비만: 지시문·계약 읽기, 이 레인 몫 파일의 실패 시험·골든 고정(커밋해도 된다), 선행 레인이 무엇을 바꾸는지 미리 읽기. 선행 레인이 고칠 파일은 아직 고치지 않는다.\n" +
		"2. 준비가 끝나면 자리를 쥐고 기다리지 않는다(2026-11-20 사용자 「노는 워크트리 없이」): 준비를 커밋한 뒤(커밋할 것이 없어도 된다) 이 작업 폴더에서 `hc-lane park \"<준비 한 줄>\"` → 위 착지·정리 절차의 `hc-lane clean` 줄 → 턴을 끝낸다. hc-lane report 완료는 적지 않는다. 선행이 들어가면 hc-fill이 주차한 가지(원격 lane/<레인>)에서 이 레인을 다시 띄우고 지시문 끝에 「이어 가기」 절을 붙인다.\n" +
		"3. park가 종료 3(원격 없는 저장소)일 때만 아래 블록을 그대로 한 번에 돌려 선행을 기다린다. 배경 실행 금지(`&`·nohup도, 도구의 배경 실행 run_in_background도). 명령 시간 한도는 도구가 허락하는 최대로 준다(예: Bash 도구 timeout 600000). 선행마다 판정 줄이 따로다 — 합치거나 줄을 고치지 않는다. 그래도 한도로 끊기면 같은 블록을 다시 돌린다(기한은 고정 시각이다).\n" +
		"   기한(" + dls + ")이 지나면 블록이 `hc-lane early-wait`로 hc-fill이 이 기계에 바퀴마다 써 주는 선행 상태를 본다 — 선행이 살아 진행 중이면(작업 중·감사·착지 기다림·대기열에서 기다림) 멈추지 않고 계속 기다린다.\n" +
		"```sh\n" +
		w + "\n" +
		"do [ \"$(date +%s)\" -lt " + strconv.FormatInt(dl, 10) + " ] || hc-lane early-wait " + strings.ReplaceAll(after, ",", " ") + " || { echo \"선행 대기 초과\"; break; }; sleep 180; done\n" +
		"```\n" +
		"4. 「선행 대기 초과」가 찍혔으면(기한 " + dls + "·" + strconv.Itoa(EWAIT) + "시간이 지났고 선행이 막힘·끝남·사라짐이거나 상태를 모름) 그 바로 앞 줄들(`<선행>: <까닭>`)을 붙여 `hc-lane report 막힘 - \"선행 대기 초과: <선행>: <까닭>\"`을 부르고 멈춘다. 이 레인의 대기열 줄은 「떠 있음」으로 남아 hc-fill이 다시 띄우지 않으니, 지휘가 `hc-fill rm <레인>` 뒤 다시 넣는다.\n" +
		"5. 블록으로 기다려 선행이 모두 들어갔으면 `git rebase " + base + "` 뒤 본 작업을 한다(준비 커밋은 그대로 둔다)."
}

// resume_sec <레인> <선행 목록> <기본 가지>: 주차한 레인을 그 가지에서 다시 띄울 때 run 지시문 끝 절(끝 줄바꿈 없음)
func resumeSec(lane, after, base string) string {
	return "\n## 이어 가기 — 주차한 가지에서 다시 띄웠다 (hc-fill, " + time.Now().Format("2006-01-02 15:04") + ")\n" +
		"이 레인은 먼저 띄워 준비한 뒤 주차했다(hc-lane park). 선행(" + strings.ReplaceAll(after, ",", ", ") + ")이 " + base + "에 들어가 hc-fill이 주차한 가지 origin/lane/" + lane + " 에서 이 작업 트리를 만들었다(준비 커밋이 들어 있다).\n" +
		"1. 먼저 `git fetch origin && git rebase " + base + "` — 충돌은 양쪽 내용을 살려 푼다. 준비 커밋은 그대로 둔다.\n" +
		"2. 그다음 위 지시문의 본 작업을 끝까지 한다. 다시 주차하지 않는다."
}
