package main

// 꺼져 죽은 레인 이어 띄우기(ht-revive-cmd, 2026-11-28, docs/hc-fill/revive.md): 워커 기계 전원이 끊겨 18시간 일한 레인의 터미널만 죽었다 —
// 대기열 줄은 옛 주소에 묶여 「끝남」이었고(멈춤 알림에서도 빠져 아무 알림 없이 묻힘), 지휘가 손으로 띄운 새 세션의 완료는 그 줄 것으로
// 읽히지 않아 감사가 열리지 않았다. 업계의 「같은 이름·같은 디스크로 다시 붙이기」: 줄(일)은 그대로 두고 죽은 실행만 같은 작업 트리에
// 새로 띄우거나(hc-lane --revive — 다시 띄울 출처 srcTree, relaunch.go) 그 작업 트리에 이미 뜬 같은 이름 세션에 묶는다(bindRun).
// 판정·거절은 reviveRow 한 곳(명령 hc-fill revive). 꺼져 죽음 알림은 deadTick.
// 기록 DATA/revive.tsv(cols.go rv…) — ls·status 표시(꺼져 죽음·이어 띄움 n번째)와 감사 차수의 「부모 실행이 바뀜」(rvRebound)이 읽는다

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/tri"
)

// 종료 — hc-lane --revive와 같은 뜻
const (
	rvDone = 0  // 이어 띄움·묶기만(--dry-run이면 할 일을 말함)
	rvUnk  = 1  // 모름 — 아무것도 안 함, 다시 해 볼 것
	rvLive = 10 // 살아 있음 — 그 줄 터미널이나 같은 작업 트리를 쓰는 다른 터미널(새 세션 0)
	rvNo   = 11 // 거절 — 다시 해도 같다
)

func init() { register("revive", reviveCmd) }

// reviveCmd: hc-fill revive <레인> [--dry-run] — run 바퀴와 같은 잠금·설정·표(터미널·연결·사건) 위에서 reviveRow 한 번
func reviveCmd(args []string) {
	n := ""
	for _, a := range args {
		switch {
		case a == "--dry-run":
			dry, P = true, "[dry-run] "
		case n == "" && okname(a):
			n = a
		default:
			usage()
		}
	}
	if n == "" {
		usage()
	}
	if err := os.MkdirAll(DATA, 0o755); err != nil {
		die("%v", err)
	}
	wait := 600
	if v := os.Getenv("HC_FILL_THEN_WAIT"); isDigits(v) {
		wait = atoi10(v)
	}
	rf := lockWait(DATA+"/run.lock", time.Duration(wait)*time.Second)
	if rf == nil {
		fmt.Fprintln(os.Stderr, "hc-fill: run 바퀴가 끝나지 않아 그만뒀다, 다시 부른다")
		exit(75)
	}
	heldLocks = append(heldLocks, rf)
	launchConf()
	readTables() // 못 읽으면 여기서 멈춘다(목록 모름 = 아무것도 안 함)
	aconf()
	EV, EVBAD = nil, false
	evTick()
	ASEC = asecText
	if AFIN != "" {
		afinSec()
	}
	rc, line := reviveRow(n, "명령")
	fmt.Println(line)
	exit(rc)
}

// reviveEnded <사건 줄>: 끝난 실행의 마지막 보고인가 — 착지·정리됨·주차·제출, 커밋 없는 완료(「완료 -」·「커밋 0」 — 이을 일이 없다)
func reviveEnded(l string) bool {
	switch evF(l, evState) {
	case "착지", "정리됨", "주차", "제출":
		return true
	case "완료":
		return dash(evF(l, evSHA)) == "-" || strings.Contains(evF(l, evMsg), "커밋 0")
	}
	return false
}

// reviveRow <레인> <어떻게(명령·스스로)>: 그 떠 있음 줄의 죽은 실행만 같은 작업 트리에 새로 띄우거나 그 작업 트리에 뜬 같은 이름 세션에
// 묶는다 — (종료, 한 줄). 바퀴 표(TERMS·CONN·LANEADDR·EV)와 설정(launchConf·ASEC)을 읽은 뒤에 부른다
func reviveRow(n, how string) (int, string) {
	q := qrow(n)
	switch {
	case q == nil:
		return rvNo, "이어 띄우지 않음 " + n + ": 대기열에 없다"
	case strings.HasPrefix(q.F(qAudit), "audit=") || strings.HasPrefix(n, "audit-"):
		return rvNo, "이어 띄우지 않음 " + n + ": 감사 줄이다(판정 없음은 자동 감사가 지휘에 넘긴다)"
	case q.F(qState) != "up":
		return rvNo, "이어 띄우지 않음 " + n + ": 떠 있는 줄이 아니다(" + q.F(qState) + " — 대기 줄은 run이 띄운다)"
	}
	p, m, a := q.F(qProj), q.F(qMach), q.F(qAddr)
	if m != LOCAL && !connHas(m) {
		return rvUnk, "모름 " + n + ": 기계 " + m + " 연결 안 됨(또는 연결 확인 못 함) — 아무것도 안 함"
	}
	if EVBAD || has(evUnk(), m) {
		return rvUnk, "모름 " + n + ": " + m + " 레인 사건을 못 읽음 — 아무것도 안 함"
	}
	if l := evLast(n, a); reviveEnded(l) {
		return rvNo, "이어 띄우지 않음 " + n + ": 그 실행의 마지막 보고가 「" + evF(l, evState) + " " + dash(head(evF(l, evSHA), 8)) + "」 — 끝난 실행"
	}
	switch mqueued(p, n) {
	case 0:
		return rvNo, "이어 띄우지 않음 " + n + ": 병합기 줄에 있다(착지는 병합기가 한다)"
	case 2:
		return rvUnk, "모름 " + n + ": 병합기 줄을 못 읽음 — 아무것도 안 함"
	}
	for _, r := range readRows(AT) {
		if r.NF() >= atNF && r.F(atLane) == n && strings.HasPrefix(r.F(atState), "escalate:") {
			return rvNo, "이어 띄우지 않음 " + n + ": 감사 차수 " + r.F(atID) + " 를 지휘에 넘기는 중(" + r.F(atState) + ")"
		}
	}
	shell := false
	if lalive(a, n) {
		if shell = alive(a) == "-" && shellLeft(a, n); !shell {
			return rvLive, "이미 돎 " + n + ": 터미널 " + a + " 이 살아 있다 — 아무것도 안 함"
		}
	}
	if shell { // 재시동 꼴 ② — 같은 주소에 빈 셸로 되살아남: 아래 검사는 그 셸을 빼고 보고, 닫기는 띄우기 바로 앞(reviveShellClose)
		var ts []Term
		for _, t := range TERMS {
			if t.Addr != a {
				ts = append(ts, t)
			}
		}
		TERMS = ts
	}
	switch pget(a) { // 표에 없어도 hc get으로 끝남을 확정한다(목록에 없음 ≠ 끝남 — row-id.md)
	case pUnknown:
		return rvUnk, "모름 " + n + ": 옛 터미널 " + a + " 을 확인하지 못했다(hc get) — 아무것도 안 함"
	case pLive:
		if PNAME == n && !shell { // 표에 없는데 살아 있음 — 빈 셸(표가 그 뒤 바뀜)이면 위와 같이 닫을 차례
			shell = shellLeft(a, n)
		}
		if PNAME == n && !shell {
			return rvLive, "이미 돎 " + n + ": 터미널 " + a + " 이 살아 있다(끊겼다 돌아옴) — 아무것도 안 함"
		}
	case pDone:
	}
	repo := repoOf(p, m)
	if repo == "" {
		return rvNo, "이어 띄우지 않음 " + n + ": 설정에 repo " + p + " " + m + " 줄이 없다"
	}
	wt := strings.TrimSuffix(repo, "/") + ".wt/" + n
	t, rc, why := reviveSeat(n, m, wt)
	if rc != rvDone {
		return rc, why
	}
	sh := ""
	if shell {
		sh = "빈 셸 " + a + " 을 닫고 "
	}
	if t != nil {
		if dry {
			return rvDone, P + sh + "묶을 " + n + ": " + a + " → " + t.Addr
		}
		if rc, why := reviveShellClose(shell, n, a); rc != rvDone {
			return rc, why
		}
		return reviveBind(q, t, how)
	}
	hd, dirty, rc, why := reviveTree(n, m, wt)
	if rc != rvDone {
		return rc, why
	}
	tree := fmt.Sprintf("작업 트리 %s (HEAD %s, 커밋 안 된 파일 %d)", wt, head(hd, 8), dirty)
	secs, rc, why := reviveSecs(q) // 다시 지을 절의 거절·모름도 dry-run·빈 셸 닫기 앞에서
	if rc != rvDone {
		return rc, why
	}
	if dry {
		return rvDone, P + sh + "이어 띄울 " + n + " @" + m + ": " + tree + " — 옛 터미널 " + a
	}
	if rc, why := reviveShellClose(shell, n, a); rc != rvDone {
		return rc, why
	}
	return reviveLaunch(q, repo, tree, how, secs)
}

// reviveShellClose: 빈 셸로 되살아난 옛 터미널(같은 이름·작업 트리를 쥐고 있다)을 검사가 다 끝난 뒤 그 주소로 닫는다(감사 atp6gby 낮음 N1 —
// 거절될 입력에 셸만 닫지 않게). 빈 셸이 아니면 할 일 없음
func reviveShellClose(shell bool, n, a string) (int, string) {
	if !shell {
		return rvDone, ""
	}
	if _, rc := (Cmd{Name: "hc", Args: []string{"kill", a}, Err: errMerge}).Run(); rc != 0 {
		return rvUnk, fmt.Sprintf("모름 %s: 빈 셸 %s 을 닫지 못했다(hc kill 종료 %d) — 아무것도 안 함", n, a, rc)
	}
	say("빈 셸 닫음 %s (%s — 재시동 뒤 셸만 남음)", a, n)
	return rvDone, ""
}

// reviveSeat <레인> <기계> <작업 트리>: 지금 그 작업 트리의 터미널 — 같은 이름 세션 하나면 그것(묶기만), 같은 이름이 다른 곳이나 다른 이름이
// 그 작업 트리를 쓰면 살아 있음(새로 띄우지 않는다 — 같은 작업 트리에 세션 둘 금지). 이 기계는 디스크로 같은 폴더인지까지(Uses — 다른 철자 cwd),
// 원격은 글자로(Under — Windows 꼴 cwd는 다른 폴더). 띄운 뒤 cd로 들어온 터미널은 hc-lane --revive의 관문이 본다
func reviveSeat(n, m, wt string) (*Term, int, string) {
	under := hcjson.Under
	if m == LOCAL {
		under = hcjson.Uses
	}
	var bind *Term
	for i := range TERMS {
		t := &TERMS[i]
		in := t.M == m && under(t.Cwd, wt)
		switch {
		case t.Name == n && in && bind == nil:
			bind = t
		case t.Name == n && in:
			return nil, rvLive, "이미 돎 " + n + ": 같은 이름 세션이 작업 트리 " + wt + " 에 둘 넘게 있다(" + bind.Addr + ", " + t.Addr + ") — 지휘가 하나를 정리한다"
		case t.Name == n:
			return nil, rvLive, "이미 돎 " + n + ": 같은 이름 터미널 " + t.Addr + " 이 다른 곳(" + dash(t.Cwd) + ")에 살아 있다 — 새로 띄우지 않음"
		case in:
			return nil, rvLive, "이미 돎 " + n + ": 작업 트리 " + wt + " 를 쓰는 다른 터미널 " + t.Addr + "(" + dash(t.Name) + ")이 살아 있다 — 새로 띄우지 않음"
		}
	}
	return bind, rvDone, ""
}

// reviveTreeSh: 작업 트리(onm 머리말 X가 $d로 — ~ 펼침)가 있나·HEAD·가지(진행 중인 rebase는 -)·커밋 안 된 파일 수
const reviveTreeSh = `cd "$d" 2>/dev/null || { echo nodir; exit 0; }
[ "$(git rev-parse --show-toplevel 2>/dev/null)" = "$(pwd -P)" ] || { echo notwt; exit 0; }
h=$(git rev-parse --verify -q HEAD) || exit 2
b=$(git symbolic-ref -q --short HEAD) || b=-
s=$(git status --porcelain) || exit 2
printf 'ok %s %s %s\n' "$h" "$b" "$(printf '%s' "$s" | grep -c .)"`

// reviveTree <레인> <기계> <작업 트리>: (HEAD, 커밋 안 된 파일 수, 종료, 까닭) — 없음·작업 트리 아님·가지가 다름은 거절, 조회 실패는 모름
func reviveTree(n, m, wt string) (string, int, int, string) {
	o, rc := onm(nil, m, reviveTreeSh, wt)
	f := strings.Fields(sub(o))
	switch {
	case rc == 0 && len(f) == 1 && f[0] == "nodir":
		return "", 0, rvNo, "이어 띄우지 않음 " + n + ": 작업 트리가 없다(" + m + " " + wt + ") — 착지·clean으로 끝났거나 지웠다(새로 하려면 대기열에 다시 넣는다)"
	case rc == 0 && len(f) == 1 && f[0] == "notwt":
		return "", 0, rvNo, "이어 띄우지 않음 " + n + ": " + wt + " 는 git 작업 트리(맨 위)가 아니다"
	case rc != 0 || len(f) != 4 || f[0] != "ok":
		return "", 0, rvUnk, fmt.Sprintf("모름 %s: 작업 트리 %s 를 확인하지 못했다(%s, 종료 %d) — 아무것도 안 함", n, wt, m, rc)
	case f[2] != n && f[2] != "-": // 진행 중인 rebase(떨어진 HEAD)는 hc-lane이 rebase의 head-name으로 본다
		return "", 0, rvNo, "이어 띄우지 않음 " + n + ": 작업 트리 " + wt + " 의 가지가 " + f[2] + " 다(레인 가지 " + n + " 아님)"
	}
	return f[1], atoi10(f[3]), rvDone, ""
}

// reviveBind <줄> <같은 이름 세션>: 사람이 손으로 띄운(라벨 hc-lane=1이 없어도) 그 작업 트리의 세션에 줄을 묶기만 한다
func reviveBind(q Row, t *Term, how string) (int, string) {
	n, a := q.F(qName), q.F(qAddr)
	if pget(t.Addr) != pLive || PNAME != n {
		return rvUnk, "모름 " + n + ": 같은 이름 세션 " + t.Addr + " 을 확인하지 못했다(hc get) — 아무것도 안 함"
	}
	if dry {
		return rvDone, P + "묶을 " + n + ": " + a + " → " + t.Addr
	}
	stuck = q.F(qStuck)
	if bindRun(n, q.F(qRID), t.M, t.Addr, PRUN, rowMode(q).val(), q.F(qAudit)) != 0 { // 14·15칸은 같은 값(시도 기록에도 그대로)
		return rvUnk, "모름 " + n + ": 줄을 묶지 못했다(시도 기록·대기열 — 줄이 바뀌었을 수 있다)"
	}
	rvAdd(n, q.F(qRID), a, "묶기만", t.Addr, how)
	s := "묶기만 " + n + ": " + a + " → " + t.Addr
	if !has(LANEADDR, t.Addr) {
		s += " (hc-lane 라벨 없음 — 손으로 띄운 세션이라 레인 임대 값이 없을 수 있다: 임대를 손으로 풀어야 할 수 있음)"
	}
	return rvDone, s
}

// reviveLaunch <줄> <저장소> <작업 트리 한 줄> <어떻게> <다시 지은 절>: hc-lane --revive로 같은 작업 트리에 새 터미널(launch — 출처 srcTree),
// 시작이 잡히면 묶는다. 한 번 띄우기용 전역은 여기서 채우고 defer로 비운다
func reviveLaunch(q Row, repo, tree, how string, s relSecs) (int, string) {
	n, m, a := q.F(qName), q.F(qMach), q.F(qAddr)
	RID, proj, stuck = q.F(qRID), q.F(qProj), q.F(qStuck)
	AK, PAFT, PSOLO, PRIO, PADDED = dash(q.F(qAudit)), q.F(qAfter), dash(q.F(qOverlap)), q.F(qPrio), q.F(qAdded)
	defer relaunchClear() // 다음 레인에 이 실행의 절·쌓음 환경을 넘기지 않는다(relaunch.go)
	RSEC, ESEC, STACK, STREF, EARLYF, ADDR, pst_ = "", s.esec, s.stack, s.stref, rowMode(q).val(), "", utcnow()
	RSRC = srcTree
	lr := launch(n, q.F(qBrief), m, repo, dash(q.F(qReply)), dash(q.F(qOwner)))
	RSRC = srcNew
	switch {
	case lr == 4:
		return rvNo, "이어 띄우지 않음 " + n + ": " + REJWHY
	case strings.HasPrefix(LFWHY, "hc-lane 종료 10 "):
		return rvLive, "이미 돎 " + n + ": " + LFWHY
	case ADDR == "":
		return rvUnk, "못 띄움 " + n + " @" + m + ": " + dash(LFWHY)
	}
	if lr != 0 || startState(ADDR, n) != tri.Yes {
		return rvUnk, "모름 " + n + ": 띄웠지만 시작 확인을 못 함 " + ADDR + " — 줄은 옛 주소 그대로(잡히면 hc-fill revive " + n + " 가 그 세션에 묶는다)"
	}
	if bindRun(n, RID, m, ADDR, START_RUN, rowMode(q).val(), q.F(qAudit)) != 0 {
		return rvUnk, "모름 " + n + ": 띄운 " + ADDR + " 에 줄을 묶지 못했다(줄이 바뀌었을 수 있다 — hc-fill revive " + n + " 로 다시)"
	}
	rvAdd(n, RID, a, "이어 띄움", ADDR, how)
	return rvDone, "이어 띄움 " + n + " @" + m + ": " + tree + " — 새 터미널 " + ADDR
}

// reviveSecs <줄>: launch가 run 지시문을 새로 쓰니 그 줄의 실행이 띄울 때 받은 절을 줄 모드대로 다시 짓는다(감사 atp6fql M2 — 빠지면 쌓인
// 레인은 HC_LANE_STACK 없이 merge-base 범위로 제출해 감사 범위와 어긋났다). 쌓아 띄움은 stack.tsv의 선행·P(waitRow의 stackSec·stackEnvs와
// 같은 값), 먼저 띄움은 earlySec. 기본 가지를 모르면 모름, 쌓인 줄인데 그 줄 기록이 없으면 거절(다시 지을 수 없다).
// 전역은 쓰지 않고 값만 돌려준다 — reviveRow가 dry-run·빈 셸 닫기 앞에서 검사하고, reviveLaunch가 띄우기 바로 앞에 채운다(감사 atp6hkn L6)
func reviveSecs(q Row) (relSecs, int, string) {
	n, em := q.F(qName), rowMode(q)
	if em != mStacked && em != mEarly {
		return relSecs{}, rvDone, ""
	}
	if fetchProj(q.F(qProj)) != 0 {
		return relSecs{}, rvUnk, "모름 " + n + ": 기본 가지를 모름(" + q.F(qProj) + " fetch·repo 줄) — 먼저 띄움·쌓아 띄움 절을 다시 짓지 못해 아무것도 안 함"
	}
	if em == mEarly {
		if EWAIT == 0 { // earlyRows가 정하는 값 — 명령에서는 아직 없다
			EWAIT = 5
			if v := conf("early_wait"); isDigits(v) && v != "0" {
				EWAIT = atoi10(v)
			}
		}
		return relSecs{esec: earlySec(q.F(qAfter), FB)}, rvDone, ""
	}
	r := stackRec(n)
	if r == nil || r.F(skRID) != q.F(qRID) {
		return relSecs{}, rvNo, "이어 띄우지 않음 " + n + ": 쌓아 띄운 줄인데 그 줄의 쌓음 기록(stack.tsv)이 없다 — 절·HC_LANE_STACK을 다시 짓지 못한다(손으로 hc-lane --revive --brief)"
	}
	if r.F(skState) == "rebased-main" { // 기본 가지로 다시 쌓인 뒤면 보통 줄과 같다(stackRef)
		return relSecs{}, rvDone, ""
	}
	s := relSecs{stack: r.F(skPred) + "@" + r.F(skP), stref: r.F(skQueue)}
	old := STACK // stackSec은 STACK에서 선행·P를 읽는다
	STACK = s.stack
	s.esec = stackSec(n, em, FB)
	STACK = old
	return s, rvDone, ""
}

// relSecs: 이어 띄울 실행에 다시 지은 절과 쌓음 환경(reviveSecs → reviveLaunch가 ESEC·STACK·STREF에)
type relSecs struct{ esec, stack, stref string }

func rvFile() string { return DATA + "/revive.tsv" }

// rvAdd: 기록 한 줄(못 쓰면 알림 한 줄 — 표시·횟수만 잃는다)
func rvAdd(n, rid, old, what, nw, how string) {
	if dry {
		return
	}
	if err := appendLine(rvFile(), strings.Join([]string{n, dash(rid), old, what, dash(nw), strconv.FormatInt(nowUnix(), 10), dash(how)}, "\t")); err != nil {
		say("이어 띄움 기록을 못 씀(%s %s): %v", n, what, err)
	}
}

// rvHas: 그 레인·줄 ID·옛 주소에 무엇(머리 글자)이 적혔나
func rvHas(led []Row, n, rid, old, what string) bool {
	for _, r := range led {
		if r.F(rvLane) == n && r.F(rvRID) == dash(rid) && r.F(rvOld) == old && strings.HasPrefix(r.F(rvWhat), what) {
			return true
		}
	}
	return false
}

// rvNote <기록> <줄> <터미널 상태(없으면 빈 값)>: ls·status 덧말 — 터미널이 없고 바퀴가 그 주소를 꺼져 죽음으로 적었으면 「꺼져 죽음」,
// 지금 주소가 이어 띄운(묶은) 실행이면 「이어 띄움(재시동 뒤, n번째)」, 아니면 빈 값
func rvNote(led []Row, q Row, live string) string {
	n, rid, a := q.F(qName), q.F(qRID), q.F(qAddr)
	if (live == "" || live == "-") && rvHas(led, n, rid, a, "죽음") { // -: 같은 주소에 빈 셸로 되살아남(deadCand)
		return "꺼져 죽음 — hc-fill revive " + n
	}
	if k := rvNth(led, n, rid, a); k > 0 {
		return fmt.Sprintf("이어 띄움(재시동 뒤, %d번째)", k)
	}
	return ""
}

// rvNth: 그 줄이 지금 주소로 몇 번째 이어 띄운(묶은) 실행인가(지금 주소가 마지막 이어 띄움의 새 주소일 때만, 아니면 0)
func rvNth(led []Row, n, rid, addr string) int {
	k, last := 0, ""
	for _, r := range led {
		if r.F(rvLane) == n && r.F(rvRID) == dash(rid) && (r.F(rvWhat) == "이어 띄움" || r.F(rvWhat) == "묶기만") {
			k, last = k+1, r.F(rvNew)
		}
	}
	if last != addr {
		return 0
	}
	return k
}

// rvRebound: 그 레인 줄이 옛 실행 주소에서 이어 띄움·묶기로 옮겨 갔나 — 감사 차수의 부모 실행이 끝난 까닭(audit2.go)
func rvRebound(n, old string) bool {
	for _, r := range readRows(rvFile()) {
		if r.F(rvLane) == n && r.F(rvOld) == old && (r.F(rvWhat) == "이어 띄움" || r.F(rvWhat) == "묶기만") {
			return true
		}
	}
	return false
}

// auRebound <말머리>(audit2.go aQueued·aDeliver): 감사 차수의 부모 실행이 끝난 까닭이 이어 띄움·묶기(줄이 새 실행으로 옮겨 감)면 「새 완료면
// superseded」와 같이 끝낸다 — 새 실행이 완료를 다시 적으면 aOpen이 새 차수를 연다(지휘에 넘기지 않음, 참)
func auRebound(what string) bool {
	if !rvRebound(auLn, auPa) {
		return false
	}
	if !dry {
		aupd(auAid, cv{atState, "superseded"})
	}
	fmt.Println(what + " " + auAn + ": " + auLn + " 이 " + auPa + " 에서 이어 띄움으로 옮겨 갔다 — 새 실행의 완료로 새 차수")
	return true
}

// deadCand: 떠 있음 줄의 터미널이 이번 표에 없다(주소가 없거나 다른 이름 — 재사용) — 표를 믿을 수 있고 그 기계가 연결·사건을 읽었을 때만
func deadCand(q Row) bool {
	n, m, a := q.F(qName), q.F(qMach), q.F(qAddr)
	if q.F(qState) != "up" || n == "" || strings.HasPrefix(q.F(qAudit), "audit=") || strings.HasPrefix(n, "audit-") || !LSSNAP {
		return false
	}
	if m != LOCAL && (!connHas(m) || has(evUnk(), m)) {
		return false
	}
	if lalive(a, n) { // 목록에 status가 없으면(-) hc get으로 — 재시동 뒤 같은 주소에 빈 셸로 되살아난 꼴 ②
		return alive(a) == "-" && shellLeft(a, n)
	}
	return lsGone(a) || alive(a) != "gone"
}

// shellLeft <주소> <이름>: 그 주소의 그 이름 터미널이 살아 있는데 안에 에이전트 없이 대화형 셸만 남았나(hc get의 전경이 맨 위 셸·status 없음 —
// hcjson.AgentState). 2026-11-28 워커 기계 정상 재부팅 실측: 코어가 레인 터미널을 같은 주소·이름·라벨(hc-lane=1)로 빈 로그인 셸(argv zsh -l)로
// 되살려 「작업 중(-)」으로 보였다. 모르면 거짓(살아 있음 쪽)
func shellLeft(a, n string) bool {
	t, found, err := hcGet(a)
	if err != nil || !found || t.State == "finished" || !t.HasName || t.Name != n {
		return false
	}
	st, _ := hcjson.AgentState(t)
	return st == tri.No
}

// REACHED: 이 바퀴에 다시 닿은 기계(앞 바퀴 reach.tsv 끊김 → 지금 연결 — unreachTick이 채운다)
var REACHED = map[string]bool{}

// reviveAutoTick(ht-revive-auto, launchRows 머리 — 띄우기 전·표를 믿을 수 있는 동안): 기록 「죽음(재시동)」(기계가 다시 닿은 바퀴에 꺼져 죽음)이고
// 아직 꺼져 죽은 줄마다 명령과 같은 reviveRow(판정·거절 모두 같다). 됨 → 주인 지휘에 중간 한 줄, 모름 → 기록 없이 다음 바퀴에(재시동 뒤 코어가
// 덜 뜸), 거절·살아 있음 → 「못 띄움」과 막힘 한 번(까닭·손으로 칠 명령), 줄당 스스로 이어 띄움이 revive_max번이면 「상한」과 막힘 한 번.
// 기록만 읽으니 다시 닿은 바퀴(와 모름으로 미룬 줄)에만 일한다
func reviveAutoTick() {
	lim := knobInt("revive_max")
	if lim == 0 || !nonEmpty(rvFile()) || !nonEmpty(Q) {
		return
	}
	led := readRows(rvFile())
	var cands []Row // 이번 표로 먼저 고른다 — 띄우면 launch가 표의 「없음」을 더 믿지 않는다(LSSNAP)
	for _, q := range qRows() {
		n, rid, a := q.F(qName), q.F(qRID), q.F(qAddr)
		if rvHas(led, n, rid, a, "죽음(재시동)") && !rvHas(led, n, rid, a, "못 띄움") && !rvHas(led, n, rid, a, "상한") && deadCand(q) {
			cands = append(cands, q)
		}
	}
	for _, q := range cands {
		n, rid, m, a := q.F(qName), q.F(qRID), q.F(qMach), q.F(qAddr)
		own, to := dash(q.F(qOwner)), dash(q.F(qOwner))
		if to == "-" {
			to = ""
		}
		k := 0
		for _, r := range led {
			if r.F(rvLane) == n && r.F(rvRID) == dash(rid) && r.F(rvHow) == "스스로" && (r.F(rvWhat) == "이어 띄움" || r.F(rvWhat) == "묶기만") {
				k++
			}
		}
		if k >= lim {
			msg := fmt.Sprintf("기계 %s 재시동 — 레인 %s 스스로 이어 띄우기 상한 revive_max %d번 — 손으로: hc-fill revive %s", m, n, lim, n)
			if aesc(n, own, msg, "hf:"+n+":revive-max:"+a) == 0 {
				rvAdd(n, rid, a, "상한", "-", "스스로")
			}
			continue
		}
		rc, line := reviveRow(n, "스스로")
		say("%s", line)
		switch rc {
		case rvDone:
			NOW = append(NOW, m) // 자리는 찬다
			nw := dash(qrow(n).F(qAddr))
			msg := fmt.Sprintf("기계 %s 재시동 — 레인 %s 같은 작업 트리에서 이어 띄움(%s → %s)", m, n, a, nw)
			if !dry && hcLaneReport("중간", msg, n, to, "hf:"+n+":revived:"+a) != 0 {
				say("이어 띄움 알림 실패 %s — 줄은 묶였다", n)
			}
		case rvUnk:
		default:
			msg := fmt.Sprintf("기계 %s 재시동 — 레인 %s 이어 띄우지 못함: %s — 손으로: hc-fill revive %s", m, n, line, n)
			if aesc(n, own, msg, "hf:"+n+":revive-fail:"+a) == 0 {
				rvAdd(n, rid, a, "못 띄움", "-", "스스로")
			}
		}
	}
}

// seenHas: 앞 바퀴 seen.tsv에 그 터미널(기계·이름·주소)
func seenHas(seen []Row, m, n, a string) bool {
	for _, s := range seen {
		if s.F(snMach) == m && s.F(snName) == n && s.F(snAddr) == a {
			return true
		}
	}
	return false
}

// deadTick(stallTick — readTables·사건 뒤): 떠 있음 줄의 터미널이 없어졌고 그 실행의 마지막 보고가 끝이 아니면 꺼져 죽음 — 앞 바퀴 seen.tsv에
// 그 터미널이 있었던 바퀴(없어진 바퀴)에만 기록에 「죽음」을 적고(오래전에 죽은 옛 줄은 「끝남」 그대로 — 이미 쌓인 상태에 처음 돌 때 수백
// 줄을 알리지 않게) 주인 지휘에 막힘 한 번(키 hf:<레인>:dead:<옛 주소>, 기록 「알림」). 기계가 다시 닿은 바퀴(REACHED)면 「죽음(재시동)」 —
// 스스로 이어 띄우기가 켜졌으면(revive_max > 0) 막힘 대신 reviveAutoTick의 결과가 알림이다
func deadTick() {
	if EVBAD || !nonEmpty(Q) {
		return
	}
	led, seen := readRows(rvFile()), readRows(DATA+"/seen.tsv")
	auto := knobInt("revive_max") > 0
	for _, q := range qRows() {
		n, rid, m, a := q.F(qName), q.F(qRID), q.F(qMach), q.F(qAddr)
		// 이미 알렸거나 스스로 이어 띄우기가 맡은 주소는 다시 보지 않는다 — 빈 셸 판정의 hc get을 바퀴마다 부르지 않게(감사 atp6gby 낮음 N2)
		if rvHas(led, n, rid, a, "알림") || (auto && rvHas(led, n, rid, a, "죽음(재시동)")) || !deadCand(q) {
			continue
		}
		if !rvHas(led, n, rid, a, "죽음") {
			l := evLast(n, a)
			if !seenHas(seen, m, n, a) || reviveEnded(l) {
				continue
			}
			what := "죽음"
			if REACHED[m] { // 기계가 다시 닿은 바퀴에 없어졌다 — 재시동(reviveAutoTick이 스스로 이어 띄운다)
				what = "죽음(재시동)"
			}
			rvAdd(n, rid, a, what, "-", "-")
			led = append(led, Row{n, dash(rid), a, what})
			say("꺼져 죽음 %s @%s: 터미널 %s 이 없다(마지막 보고 %s)", n, m, a, dash(evF(l, evState)))
		}
		if auto && rvHas(led, n, rid, a, "죽음(재시동)") { // 재시동은 reviveAutoTick의 결과가 알림
			continue
		}
		msg := fmt.Sprintf("%s 꺼져 죽음 — @%s 터미널 %s 이 없다(마지막 보고 %s), 작업 트리 %s 는 남았을 수 있다 — 이어 띄우기: hc-fill revive %s",
			n, m, a, dash(evF(evLast(n, a), evState)), strings.TrimSuffix(repoOf(q.F(qProj), m), "/")+".wt/"+n, n)
		if aesc(n, dash(q.F(qOwner)), msg, "hf:"+n+":dead:"+a) == 0 {
			rvAdd(n, rid, a, "알림", "-", "-")
			led = append(led, Row{n, dash(rid), a, "알림"})
		}
	}
}
