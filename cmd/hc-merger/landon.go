package main

// 착지 실행을 워커에(ht-merger-land-offload, 2026-11-26 사용자 「병합기도 지휘에서만 하지 말고 지휘가 병합을 워커들에게 지시 할수 있도록
// 하느네 맞겠다 지휘 컴퓨터가 너무 지금 부하가 심하니까 진행이 안되는것 같아서」). 모으기·차례·rebase·판정·넣기 결정은 그대로 병합기
// 기계가 하고(11-24 「병합은 지휘에서 모와서」), 맨 앞을 직접 검증으로 넣는 바퀴(추측 통과 없음 — 착지 스크립트 저장소의 맨 앞 실패·모름,
// 맡길 한가한 기계 없음 포함)의 **착지 실행**(준비 훅 → hc-lane land: rebase·검증·push, 또는 착지 스크립트)만 고른 워커의 클론에서
// 돌린다(추측 일꾼처럼 hc run --bg --rm --on <워커> --name land-<차례 8>, 결과 파일 <클론>.land-wt/<차례 12>.rc를 짧은 hc exec로 읽으며
// 기다림 — hc exec는 600초가 상한이라 긴 착지를 그대로 못 기다린다). 병합기는 종료 코드와 로그 끝만 받아 지금과 같은
// 판정(judge)·일지·반송·사건 길로 간다.
//
//   - 손잡이 land_on(공통 merger_land_on, 프로젝트별 merger_land_on@<프로젝트>, 옛 길 <일지 폴더>/land_on, HC_LANE_MERGER_LAND_ON):
//     auto(기본) · local(지금처럼 병합기 기계) · 기계 목록(쉼표 — 그 안에서만). 후보는 검증 기계 목록(refs/herdr/verifiers — 기계와 그
//     기계의 클론)의 기계에서 병합기 기계를 뺀 것. 후보를 함께 재어(--spec-idle, verifierpick.go) 도는 저장소 검증이 없는(검증 자리
//     남음) 기계 가운데 코어당 부하가 가장 낮은 기계, 모두 검증 중이면 코어당 부하가 가장 낮은 기계 — 한가함 문턱(idle_load)을 넘었다고
//     병합기 기계로 떨어지지 않는다. 병합기 기계는 손잡이 local·목록에 병합기 기계를 넣었을 때, 또는 닿는 워커가 하나도 없을 때만(그때
//     운영 사건 한 번, 맨 앞 제출마다). 검증 기계 목록이 없거나 제출 기록 없는 손 제출이 든 묶음은 지금 그대로 병합기 기계(사건·일지 줄 없음).
//   - 착지 차례(원격 refs/hc-merger/land/<기본>): 맡기기 전에 후보 커밋을 부모로 한 차례 커밋을 없을 때만(lease) 올리고, 착지의 기본
//     브랜치 push에 그 참조 삭제 + 본 값 lease를 함께 싣는다(HERDR_LAND_PUSH_EXTRA — 큐 표식과 같은 원자 push). 병합기가 차례를 지운
//     실행은 기본 브랜치에 넣지 못한다 — 워커가 죽었다고 보고 다른 워커에 다시 맡겨도 두 기계가 함께 넣지 않는다. 바퀴의 착지 전에
//     차례가 남아 있으면(앞 실행) 그 기계의 실행이 살아 있거나 모르는 동안 기다리고(HERDR_TIMEOUT_LAND까지), 끝났으면 지운다.
//   - 워커가 죽음(못 띄움·결과 없이 터미널이 끝남·결과 읽기가 5분 안 닿음·시간 넘김·워커 쪽에서 착지를 시작 못 함 — 경로·hc-lane
//     없음·옛 판·차례 바뀜은 감싸개가 결과 「nostart <종료>」): 차례를 지우고(본 값 lease) 일지로 끊긴 앞 실행처럼
//     판정한 뒤(recover — 그새 넣었으면 완료), 그 맨 앞 제출에서 그 기계를 빼고 다음 바퀴에 다른 워커로. 뺀 기계가 손잡이 busy_max를
//     넘으면 병합기 기계에서 + 운영 사건 한 번. 워커의 착지 실패(검증 실패·충돌 등 종료 코드)는 지금과 같은 반송·묶음 줄이기·일시 실패.
//   - 보이기: 정할 때 일지에 「착지 실행」 줄(까닭 칸 「<기계>(<까닭>)」) — hc-lane merger --why가 세고, hc-fill status 병합기 줄이 마지막 것을.

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/verified"
)

const (
	landClaims   = "refs/hc-merger/land/" // 착지 차례(+ <기본 브랜치>)
	jLandOn      = "착지 실행"                // 일지 결과 칸 — 착지를 돌릴 기계를 정함
	rcLandRunNot = 70                     // --land-run: 착지를 시작하지 못함(받기·차례 바뀜·작업 트리) — 병합기는 워커가 죽은 것과 같게
	rcLost       = -2                     // landExec: 맡긴 워커가 죽어 일지로 판정했다 — 바퀴는 여기서 끝(round)
	localTag     = "병합기 기계 · "            // 일지 까닭의 머리 — 병합기 기계에서 돌림(--why가 가른다)
)

// landOff: M에 묻는 착지 실행 상태 — 이번 바퀴 검증 기계 목록(specDecide가 채움)
type landOff struct {
	vers   []verified.Verifier
	lclaim string // 이번 바퀴에 올린 착지 차례 커밋
}

func (m *M) claimRef() string { return landClaims + m.base }

// landMode: 손잡이 land_on — "auto" · "local" · "list"(그 기계들)
func (m *M) landMode() (string, map[string]bool) {
	v := m.knobStr("land_on", "HC_LANE_MERGER_LAND_ON")
	switch v {
	case "", "-", "auto":
		return "auto", nil
	case "local":
		return "local", nil
	}
	in := map[string]bool{}
	for _, x := range strings.Split(v, ",") {
		if x = strings.TrimSpace(x); verified.MachineOK(x) {
			in[x] = true
		}
	}
	return "list", in
}

// landDead: 이 맨 앞 제출의 착지 실행이 죽은 기계들(<일지 폴더>/landdead — 「<맨 앞 SHA> <기계>…」, 다른 제출이면 빈 값)
func (m *M) landDead() []string {
	b, err := os.ReadFile(m.mj + "/landdead")
	if err != nil {
		return nil
	}
	f := strings.Fields(string(b))
	if len(f) < 2 || f[0] != m.b[0].s {
		return nil
	}
	return f[1:]
}

func (m *M) landDeadAdd(mach string) {
	d := append(m.landDead(), mach)
	if err := writeVia(m.mj+"/landdead", m.b[0].s+" "+strings.Join(d, " ")+"\n"); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 죽은 착지 기계를 적지 못했다(다음 바퀴에 그 기계를 다시 고를 수 있다): %v\n", err)
	}
}

// landCand: 후보 하나의 재기
type landCand struct {
	v    verified.Verifier
	i    idleInfo
	ok   bool
	note string
	gate bool // 병합 검증 자리가 빔(merger_gate@<기계> 1 · 도는 병합 검증 0)
}

// landPick: 이번 착지를 돌릴 기계와 까닭. 빈 Machine이면 병합기 기계(까닭은 localTag로 시작, 고를 것이 없으면 빈 까닭 — 지금 그대로·
// 일지 줄 없음). ev면 운영 사건 한 번
func (m *M) landPick() (verified.Verifier, string, bool) {
	mode, in := m.landMode()
	if mode == "local" {
		return verified.Verifier{}, localTag + "손잡이 local", false
	}
	for _, e := range m.b {
		if e.d == "" { // 판정이 후보 가지 끝(로컬)을 본다 — 원격에서 rebase하면 못 본다
			return verified.Verifier{}, "", false
		}
	}
	dead := m.landDead()
	if len(dead) > m.busyMax() {
		return verified.Verifier{}, fmt.Sprintf("%s맡긴 워커가 %d번 죽음(%s, 손잡이 busy_max %d)", localTag, len(dead), strings.Join(dead, "·"), m.busyMax()), true
	}
	skip := map[string]bool{}
	for _, d := range dead {
		skip[d] = true
	}
	var cs []*landCand
	listed := false
	for _, v := range m.vers {
		if v.Machine == m.mme || mode == "list" && !in[v.Machine] {
			continue
		}
		listed = true
		if !skip[v.Machine] {
			cs = append(cs, &landCand{v: v})
		}
	}
	switch {
	case !listed && mode == "auto": // 검증 기계 목록 없음 — 지금 그대로
		return verified.Verifier{}, "", false
	case !listed && in[m.mme]:
		return verified.Verifier{}, localTag + "손잡이 목록", false
	case !listed:
		return verified.Verifier{}, localTag + "손잡이 목록의 기계가 검증 기계 목록에 없음", true
	}
	var wg sync.WaitGroup
	for _, c := range cs {
		wg.Add(1)
		go func(c *landCand) {
			defer wg.Done()
			c.i, c.ok, c.note = probeRemote(c.v.Machine)
			c.gate = c.ok && gateOn(c.v.Machine) && c.i.gateFree()
		}(c)
	}
	wg.Wait()
	best := landBest(cs)
	if best == nil {
		var notes []string
		for _, c := range cs {
			notes = append(notes, c.v.Machine+" "+c.note)
		}
		for _, d := range dead {
			notes = append(notes, d+" 죽음")
		}
		if in[m.mme] {
			return verified.Verifier{}, localTag + "손잡이 목록 — 닿는 워커 없음", false
		}
		return verified.Verifier{}, localTag + "닿는 워커 없음(" + strings.Join(notes, " · ") + ")", true
	}
	why := "검증 자리 남음"
	switch {
	case best.i.verifies == 0:
	case best.gate:
		why = "병합 검증 자리 — 레인 검증과 함께"
	default:
		why = "모두 검증 중 — 가장 덜 바쁨"
	}
	return best.v, fmt.Sprintf("%s · 코어당 부하 %.2f", why, best.i.load/float64(best.i.cpus)), false
}

// landBest: 잰 후보 가운데 도는 검증이 없는 기계 먼저, 다음 병합 검증 자리가 빈 기계(추측 검증 고르기와 같은 기준 — verifierpick.go),
// 그 안에서 코어당 부하가 가장 낮은 기계(같으면 목록 차례). 닿은 것이 없으면 nil
func landBest(cs []*landCand) *landCand {
	var ok []*landCand
	for _, c := range cs {
		if c.ok {
			ok = append(ok, c)
		}
	}
	sort.SliceStable(ok, func(a, b int) bool {
		x, y := ok[a].i, ok[b].i
		if (x.verifies == 0) != (y.verifies == 0) {
			return x.verifies == 0
		}
		if ok[a].gate != ok[b].gate {
			return ok[a].gate
		}
		return x.load/float64(x.cpus) < y.load/float64(y.cpus)
	})
	if len(ok) == 0 {
		return nil
	}
	return ok[0]
}

// landPrep: 착지 직전(착지 중 표식 뒤) — 착지 차례가 남아 있으면 기다리거나 치운다. 쉬면 false(후보를 치우고 큐는 그대로)
func (m *M) landPrep() bool {
	sha := refsha(m.qs, m.claimRef())
	if sha == "" {
		return true
	}
	why, wait := m.claimHeld(sha)
	if !wait && m.unclaim(sha) {
		fmt.Printf("hc-lane merger: 남은 착지 차례를 치웠다(%s) — 그 실행은 이제 %s 에 넣지 못한다\n", why, m.base)
		return true
	}
	if m.fin() {
		m.defer_("착지 차례를 앞 실행이 쥐고 있다(" + why + ")")
	}
	return false
}

// claimHeld: 남은 차례 커밋 sha의 앞 실행이 아직 넣을 수 있나 — 살아 있거나 모르면(HERDR_TIMEOUT_LAND 안) wait. 까닭은 바퀴마다 같은 글
func (m *M) claimHeld(sha string) (string, bool) {
	if fetchr(true, "-q", "origin", m.claimRef()) != 0 {
		return "차례를 못 받음", true
	}
	subj, rc := out("log", "-1", "--format=%s", sha)
	f := map[string]string{}
	head, path, _ := strings.Cut(subj, " path=")
	for _, kv := range strings.Fields(head) {
		if k, v, ok := strings.Cut(kv, "="); ok {
			f[k] = v
		}
	}
	at, err := strconv.ParseInt(f["at"], 10, 64)
	mach := f["machine"]
	if rc != 0 || err != nil || !addrOK(mach) || path == "" {
		return "차례 꼴 모름", true
	}
	if time.Now().Unix()-at > landWait() {
		return mach + " 실행이 착지 상한을 넘김", false
	}
	switch landLive(mach, sha) { // 실행이 살아 있나 — 그 기계 터미널 land-<차례 8>(--rm: 끝나면 없어진다)
	case idIdle:
		return mach + " 실행 끝남", false
	case idBusy:
		return mach + " 실행 도는 중", true
	}
	return mach + " 실행 살아 있음 모름", true
}

// landWait: 워커 착지 한 번을 기다리는 상한(초) — 준비 훅 + 착지 + 5분. 둘 중 하나라도 상한 없음(0)이면 하루
// ponytail: 상한 없음도 하루로 — 차례가 끝없이 남지 않게
func landWait() int64 {
	if setupMax() <= 0 || landMax() <= 0 {
		return 86400
	}
	return int64((setupMax()+landMax())/time.Second) + 300
}

// landLive: 워커 터미널 land-<차례 8>이 살아 있나 — idBusy 살아 있음 · idIdle 없음(끝남) · idUnknown 못 읽음
func landLive(mach, cm string) int {
	t, found, err := hcGet(mach+"/land-"+cm[:8], "")
	switch {
	case err != nil:
		return idUnknown
	case found && t.State == "live":
		return idBusy
	}
	return idIdle
}

// unclaim: 차례 sha를 본 값 lease로 지운다. 이미 없거나 다른 값이면(그 실행이 넣으며 지웠다) true. 모르면 false
func (m *M) unclaim(sha string) bool {
	h, rc := lsRemote("origin", m.claimRef())
	if rc != 0 {
		return false
	}
	if f := strings.Fields(h); len(f) == 0 || f[0] != sha {
		return true
	}
	return runO("push", "-q", "--force-with-lease="+m.claimRef()+":"+sha, "origin", ":"+m.claimRef()) == 0
}

// landExec: 착지 실행 — 직접 검증이면 기계를 골라(landPick) 일지에 한 줄, 워커면 그 클론에서(runRemote), 아니면 지금처럼(runCandidate)
func (m *M) landExec() int {
	if m.vw.id != "" { // 맨 앞 추측 통과 — 착지가 검증을 건너뛴다(병합기 기계에서 가볍게)
		return m.runCandidate()
	}
	v, why, ev := m.landPick()
	if why == "" {
		return m.runCandidate()
	}
	mach := v.Machine
	if mach == "" {
		mach = m.mme
	}
	fmt.Printf("hc-lane merger: 착지 실행 — %s(%s)\n", mach, why)
	m.jround(jLandOn, mach+"("+why+")", "")
	if ev {
		m.landEvent(why)
	}
	if v.Machine == "" {
		return m.runCandidate()
	}
	rc, lost := m.runRemote(v)
	if lost == "" {
		return rc
	}
	m.landDeadAdd(v.Machine)
	m.ls1 = time.Now().Unix()
	m.jround(jLandOn, fmt.Sprintf("%s 착지 실행 끊김(%s) — 일지로 판정하고 다음 바퀴에 다른 워커로", v.Machine, lost), ", 로그 "+m.lgf)
	if !m.unclaim(m.lclaim) { // 차례를 못 지움 — 그 실행이 아직 넣을 수 있다. 일지를 두고 다음 바퀴에(landPrep이 기다리거나 치운 뒤 recover)
		m.mer, m.mq = true, ""
		return rcLost
	}
	if !m.recover() { // 그새 넣었으면 완료, 아니면 처음부터
		m.mer, m.mq = true, ""
	}
	return rcLost
}

// landEvent: 병합기 기계에서 돌리는 까닭(닿는 워커 없음 등)을 운영 사건 한 번(맨 앞 제출마다 — 사건 열쇠)
func (m *M) landEvent(why string) {
	msg := fmt.Sprintf("병합기 맨 앞 %s 착지를 워커에 맡기지 못해 병합기 기계(%s)에서 돌린다 — %s (손잡이 merger_land_on, 맨 앞 %s)", m.b[0].l, m.mme, strings.TrimPrefix(why, localTag), m.b[0].s)
	if _, rc, e := boundedRun(reportMax, gSelf, nil, "report", "막힘", "-", msg, "--name", "merger", "--key", "hc-merger-land-local:"+m.b[0].s); rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 착지 실행 사건을 적지 못했다(종료 %d): %s\n", rc, strings.TrimSpace(e))
	}
}

// runRemote: 워커 v의 클론에서 착지(hc-lane merger --land-run). 착지 종료 코드, 또는 lost(워커가 죽음 — 까닭)
func (m *M) runRemote(v verified.Verifier) (int, string) {
	lf, err := os.OpenFile(m.lgf, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 착지 로그를 열지 못했다: %v\n", err)
		return 75, ""
	}
	defer lf.Close()
	cand, rc1 := out("rev-parse", "--verify", "refs/heads/"+m.br)
	tree, rc2 := out("rev-parse", "--verify", m.br+"^{tree}")
	if rc1 != 0 || rc2 != 0 {
		return rcNotRun, ""
	}
	cm, ok := commitTree("", tree, cand, fmt.Sprintf("hc-merger-land machine=%s at=%d lane=%s path=%s", v.Machine, time.Now().Unix(), m.br, v.Path))
	ref := m.claimRef()
	if !ok || runO("push", "-q", "--force-with-lease="+ref+":", "origin", cm+":"+ref) != 0 {
		fmt.Fprintln(lf, "hc-lane merger: 착지 차례를 올리지 못했다 — 착지하지 않았다")
		return rcNotRun, ""
	}
	m.lclaim = cm
	parts := ""
	if m.sparts != "" { // 통과 조각은 워커가 원격에서 다시 받는다(병합기 파일 경로는 그 기계에 없다)
		parts = "HC_MERGER_LAND_SPEC=" + m.sfid
	}
	env := m.landEnv(lf, m.ex+" :"+ref+" --force-with-lease="+ref+":"+cm, parts)
	// 추측 일꾼과 같은 꼴(hc run --bg --rm — hc exec는 600초가 상한이라 긴 착지를 못 기다린다). 결과는 그 기계 파일 <클론>.land-wt/<차례 12>.rc
	dir := v.Path + ".land-wt"
	res, lg := dir+"/"+cm[:12]+".rc", dir+"/"+cm[:12]+".log"
	args := append([]string{"run", "--bg", "--json", "--no-parent", "--rm", "--on", v.Machine, "--name", "land-" + cm[:8],
		"--label", "hc-merger-land=1", "--cwd", v.Path, "env", "HC_MERGER_LAND_DIR=" + dir, "HC_MERGER_LAND_RESULT=" + res}, append(gateEnv(v.Machine), env...)...)
	args = append(args, "sh", "-c", landWrap, "hc-merger-land", "hc-lane", "merger", "--land-run", m.base, cm)
	fmt.Fprintf(lf, "hc-lane merger: 착지를 %s(%s)에서 돌린다 — 차례 %s\n", v.Machine, v.Path, first8(cm))
	if _, rc, e := boundedRun(60*time.Second, "hc", nil, args...); rc != 0 {
		fmt.Fprintf(lf, "hc-lane merger: 워커에 착지를 띄우지 못했다(종료 %d): %s\n", rc, strings.TrimSpace(e))
		return 0, "못 띄움"
	}
	rc, lost := landAwait(v.Machine, cm, res)
	landLogTail(lf, v.Machine, lg)
	if lost == "" {
		m.unclaim(cm) // 실패로 끝난 실행의 차례 — 못 지우면 다음 바퀴 landPrep이 끝남을 보고 치운다
	}
	return rc, lost
}

// landWrap: 워커 터미널의 감싸개 — land-run이 결과 파일을 못 썼으면(시작 못 함: hc-lane 없음 127·옛 판 「알 수 없는 인자」 1·폴더 못
// 만듦 70 등) 「nostart <종료>」를 적는다. 병합기는 그것을 착지 실패가 아니라 워커가 죽은 것으로 본다(반송하지 않음).
// 터미널이 신호로 끊기면(hc kill — 무리에 HUP·TERM, 약 1초 뒤 KILL: hc 0.1.22 실측) 그 자리에서 결과가 없으면 「killed」를 적는다
// — 덫이 없으면 셸이 먼저 죽어 결과 없이 터미널만 사라졌다(2026-11-26 ht-land-run-killed). 착지는 뒤로 띄워 wait로 기다린다:
// 앞일이면 셸이 착지가 거두고 끝날 때까지 덫을 미뤄 1초 뒤 KILL에 잘렸다(감사 atp2sd0 중간 1). 착지는 같은 무리라 신호를 따로
// 받는다 — 그사이 넣었으면 차례가 이미 지워져 병합기 recover가 완료로, 넣는 중이면 차례 lease가 두 번 넣기를 막는다.
// 처음부터 KILL이면 못 적는다(그때는 병합기가 터미널 없음으로 본다)
const landWrap = `R=$HC_MERGER_LAND_RESULT; mkdir -p "$HC_MERGER_LAND_DIR" || exit 70; rm -f "$R"; ` +
	`trap '[ -s "$R" ] || echo killed >"$R"; exit 129' HUP TERM INT; "$@" & p=$!; wait $p; r=$?; [ -s "$R" ] || echo "nostart $r" >"$R"; exit $r`

// landPoll: 워커 착지 결과를 읽는 간격(시험이 줄인다) · landMiss: 결과 읽기가 이만큼 잇달아 안 닿으면 끊김(기본 20번 ≈ 5분)
var (
	landPoll = 15 * time.Second
	landMiss = 20
)

// landAwait: 워커 착지의 결과 파일을 짧은 hc exec로 읽어 기다린다. 착지 종료 코드, 또는 lost 까닭(시작 못 함·결과 없이 끝남·닿지 않음·살아 있음 모름·
// 시간 넘김 — landWait)
func landAwait(mach, cm, res string) (int, string) {
	end := time.Now().Add(time.Duration(landWait()) * time.Second)
	for miss := 0; ; time.Sleep(landPoll) {
		r, ok := landResult(mach, res)
		switch {
		case !ok:
			if miss++; miss >= landMiss {
				return 0, "닿지 않음"
			}
		case r != "":
			return landParse(r)
		default:
			switch landLive(mach, cm) {
			case idIdle: // 터미널이 끝났는데 결과가 없다 — 그사이 적었을 수 있어 한 번 더
				if r, ok := landResult(mach, res); ok && r != "" {
					return landParse(r)
				}
				return 0, "결과 없이 끝남"
			case idUnknown: // 살아 있는지 못 읽음이 이어지면 결과 못 읽음과 같이 끊김(차례 lease가 두 번 넣기를 막는다)
				if miss++; miss >= landMiss {
					return 0, "살아 있음 모름"
				}
			default:
				miss = 0
			}
		}
		if time.Now().After(end) {
			return 0, "시간 넘김"
		}
	}
}

// landParse: 결과 파일 — 「<종료>」(착지가 돌았다) 또는 「nostart <종료>」(시작 못 함 → lost)
func landParse(r string) (int, string) {
	if n, err := strconv.Atoi(r); err == nil {
		return n, ""
	}
	if r == "killed" {
		return 0, "워커 착지가 신호로 끊김(hc kill 등)"
	}
	if c, ok := strings.CutPrefix(r, "nostart "); ok {
		return 0, "워커가 착지를 시작 못 함(종료 " + c + ")"
	}
	return 0, "결과 꼴 모름: " + bcut(40, flat(r))
}

// landResult: 그 기계 결과 파일(없으면 빈 값). ok=false면 못 읽음(hc 실패·시간 넘김)
func landResult(mach, res string) (string, bool) {
	o, rc, e := boundedRun(40*time.Second, "hc", nil, "exec", "--on", mach, "--json", "--timeout", "30", "--", "sh", "-c", `cat "$1" 2>/dev/null; exit 0`, "_", res)
	x, err := hcjson.ParseExec([]byte(o), []byte(e), rc)
	if err != nil || x.Code != 0 || x.TimedOut {
		return "", false
	}
	return strings.TrimSpace(string(x.Stdout)), true
}

// landLogTail: 워커 착지 로그의 끝(256KB)을 병합기 착지 로그 뒤에 — judge·runTail이 실패 줄을 거기서 읽는다
func landLogTail(lf *os.File, mach, lg string) {
	o, rc, e := boundedRun(70*time.Second, "hc", nil, "exec", "--on", mach, "--json", "--timeout", "60", "--", "sh", "-c", `tail -c 262144 "$1"`, "_", lg)
	if x, err := hcjson.ParseExec([]byte(o), []byte(e), rc); err == nil {
		lf.Write(x.Stdout)
	}
	fmt.Fprintf(lf, "\nhc-lane merger: 워커 착지 로그 %s:%s\n", mach, lg)
}

// landRunMain: hc-lane merger --land-run <기본 브랜치> <차례 커밋> — 워커 쪽(병합기가 hc run --on으로, 그 기계 클론에서 landWrap 안에).
// 차례 커밋의 부모(후보)를 <폴더>/<차례 12>(폴더 = HC_MERGER_LAND_DIR, 없으면 <클론>.land-wt)에 hc-merger-land/<레인>-<차례 12> 가지로 꺼내
// 준비 훅 → hc-lane land(병합기와 같은 환경 — 부른 쪽이 env로). 로그는 <폴더>/<차례 12>.log. 착지 단계에 들어섰으면 종료 코드를
// HC_MERGER_LAND_RESULT 파일에 적는다(못 적은 끝은 감싸개가 「nostart」). 종료: 착지 종료 코드 · 75 준비 훅 실패 · 70 시작 못 함 · 2 인자
func landRunMain(a []string) int {
	rc, started := landRun(a)
	if r := os.Getenv("HC_MERGER_LAND_RESULT"); started && r != "" {
		if err := os.WriteFile(r, []byte(strconv.Itoa(rc)+"\n"), 0o666); err != nil {
			fmt.Fprintf(os.Stderr, "hc-lane merger: --land-run — 결과 파일을 못 썼다(병합기는 시작 못 함으로 본다): %v\n", err)
		}
	}
	return rc
}

func landRun(a []string) (int, bool) {
	if len(a) != 2 || !plain(a[0]) || len(a[1]) != 40 || strings.Trim(a[1], "0123456789abcdef") != "" {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --land-run <기본 브랜치> <차례 커밋 40자>")
		return 2, false
	}
	base, cm := a[0], a[1]
	ln := os.Getenv("HC_LANE")
	top, rc := outQ("rev-parse", "--show-toplevel")
	if !laneOK(ln) || rc != 0 || os.Chdir(top) != nil {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --land-run — git 클론 안에서 HC_LANE과 함께 부른다")
		return rcLandRunNot, false
	}
	if orphansDefer("착지 작업 트리") {
		return rcLandRunNot, false
	}
	dir := os.Getenv("HC_MERGER_LAND_DIR")
	if dir == "" {
		dir = top + ".land-wt"
	}
	landSweep(dir)
	ref := landClaims + base
	if fetchr(false, "-q", "origin", "+"+ref+":"+ref, "+refs/heads/"+base+":refs/remotes/origin/"+base) != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --land-run — 착지 차례를 받지 못했다")
		return rcLandRunNot, false
	}
	// 착지 push가 함께 싣는 제출 기록 커밋(<기록>:refs/merge-done/<레인> — exbuild)을 이 클론에도 — 그 push가 지우는 기록 참조를 받기만
	// (FETCH_HEAD, 참조는 남기지 않음)
	metas := []string{"-q", "origin"}
	for _, f := range strings.Fields(os.Getenv("HERDR_LAND_PUSH_EXTRA")) {
		if r, ok := strings.CutPrefix(f, ":refs/merge-queue-meta/"); ok {
			metas = append(metas, "refs/merge-queue-meta/"+r)
		}
	}
	if len(metas) > 2 && fetchr(true, metas...) != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --land-run — 제출 기록을 받지 못했다")
		return rcLandRunNot, false
	}
	defer runQ("update-ref", "-d", ref) // 이 클론에 차례 참조를 남기지 않는다
	if got, _ := outQ("rev-parse", "-q", "--verify", ref); got != cm {
		fmt.Fprintf(os.Stderr, "hc-lane merger: --land-run — 착지 차례가 바뀌었다(%s ≠ %s), 착지하지 않는다\n", first8(got), first8(cm))
		return rcLandRunNot, false
	}
	wt, br := dir+"/"+cm[:12], landBranch(ln, cm)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --land-run — 작업 폴더를 만들지 못했다")
		return rcLandRunNot, false
	}
	lg := wt + ".log"
	defer runQ("branch", "-q", "-D", br)
	landRmWT(wt)
	if inLog("", lg, "worktree", "add", "-q", "-f", "-B", br, wt, cm+"^") != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --land-run — 작업 트리를 만들지 못했다")
		return rcLandRunNot, false
	}
	defer landRmWT(wt)
	lf, err := os.OpenFile(lg, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return rcLandRunNot, false
	}
	defer lf.Close()
	env := append(os.Environ(), "HERDR_MERGER=1")
	if id := os.Getenv("HC_MERGER_LAND_SPEC"); id != "" {
		if line, _, ok := specPartsGet(id, ""); ok && len(strings.Fields(line)) >= 2 && os.WriteFile(wt+".parts", []byte(line+"\n"), 0o666) == nil {
			env = append(env, "HERDR_VERIFY_PARTS_FROM="+wt+".parts")
			defer os.Remove(wt + ".parts")
		}
	}
	switch rc := setupHook(wt, top, base, lf, ""); rc {
	case rcNotRun:
		return rcLandRunNot, false
	case 0:
	default:
		return rc, true
	}
	c := exec.Command(gSelf, "land", base)
	c.Dir, c.Stdout, c.Stderr, c.Env = wt, lf, lf, env
	rc, ran := proc.WaitFor(c, "", landMax(), landMax() <= 0)
	if !ran {
		return rcLandRunNot, false
	}
	return rc, true
}

// landBranch: 착지 작업 트리의 가지 — 실행(차례)마다 다른 이름. 레인 이름만이면 신호로 끊겨 defer가 못 돈 앞 실행의 작업 트리가 그
// 가지를 쥐어 같은 레인의 다음 실행이 시작도 못 했다(2026-11-26 ht-land-run-killed — 「used by worktree」 → nostart 70). 남은 것은
// landSweep이 하루 뒤 치운다. 「/」가 아니라 「-」로 잇는다 — 옛 이름 hc-merger-land/<레인> 가지와 참조 폴더·파일로 부딪히지 않게
func landBranch(ln, cm string) string { return "hc-merger-land/" + ln + "-" + cm[:12] }

// landRmWT: 착지 작업 트리를 지운다(git이 못 지우면 폴더째 — <폴더> 아래만), 등록은 prune
func landRmWT(wt string) {
	if _, err := os.Lstat(wt); err == nil && runQ("worktree", "remove", "-f", "-f", wt) != 0 {
		os.RemoveAll(wt)
	}
	runQ("worktree", "prune")
}

// landSweep: 시간 넘김(무리째 KILL)으로 defer가 못 돈 앞 실행의 잔재 — <폴더> 아래 하루 넘은 항목과, 어느 작업 트리도 쓰지 않는
// hc-merger-land/* 가지를 치운다(감사 atp2f36 낮음 1)
func landSweep(dir string) {
	es, _ := os.ReadDir(dir) // 없으면 치울 것 없음
	for _, e := range es {
		if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) > 24*time.Hour {
			if e.IsDir() {
				landRmWT(dir + "/" + e.Name())
			} else {
				os.Remove(dir + "/" + e.Name())
			}
		}
	}
	runQ("worktree", "prune")
	bs, _ := outQ("for-each-ref", "--format=%(refname:short)", "refs/heads/hc-merger-land/")
	for _, b := range strings.Fields(bs) {
		runQ("branch", "-q", "-D", b) // 작업 트리가 쓰는 가지는 git이 거절한다(도는 실행)
	}
}
