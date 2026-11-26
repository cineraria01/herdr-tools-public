package main

// state의 병합 줄 칸(2026-11-24 ops-state-trim): 병합기가 이 기계(LOCAL)에 있는 프로젝트는 이 기계 병합기 일지
// (<XDG_STATE_HOME, 없으면 ~/.local/state>/hc-lane/merger-journal.tsv — hc-merger result.go jrec)에서 마지막 바퀴·최근 검증·건너뜀·맡김을
// 읽는다(읽기만). 다른 기계 병합기는 모름+까닭(그 일지를 원격으로 읽지 않는다). 원격 호출은 둘뿐 — 이 기계 병합기가 맨 앞 추측 검증을
// 기다리는 중이면 그 검증 기계에, 맨 앞 착지를 워커에서 돌리는 중이면 그 워커에 find 한 번씩(hc exec, 상한 proc.Limit — stateSpecWait·
// stateLandOn).

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/opsstate"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/verified"
)

// stateJournalN: 최근 착지·맡김을 셀 수(hc-lane merger --why 기본과 같은 20)
const stateJournalN = 20

// stateMergeJournal: 병합 줄 하나를 채우는 함수. 일지는 한 번만 읽는다. 이 기계 병합기 프로젝트가 둘 이상이면 일지 줄(프로젝트 칸 없음)을
// 대기열 줄의 레인 → 프로젝트로 가른다
// ponytail: 대기열에서 정리된 레인의 일지 줄은 프로젝트를 몰라 둘 이상일 때 빠진다(덜 셈) — 일지에 프로젝트 칸이 생기면 그것으로
func stateMergeJournal(mr map[string]maRow, rows []Row, cwd map[string]string, terms []hcjson.Terminal) func(g *opsstate.MergeLine, x maRow, now time.Time) {
	local := 0
	for _, x := range mr {
		if x.M == LOCAL {
			local++
		}
	}
	proj := map[string]string{}
	for _, r := range rows {
		proj[r.F(qName)] = r.F(qProj)
	}
	var ls []string
	bad := ""
	dir := stateDir()
	if local > 0 {
		b, err := os.ReadFile(dir + "/hc-lane/merger-journal.tsv")
		switch {
		case os.IsNotExist(err):
			bad = "이 기계 병합기 일지 없음(merger-journal.tsv)"
		case err != nil:
			bad = "이 기계 병합기 일지 못 읽음: " + err.Error()
		default:
			ls = lines(string(b))
		}
	}
	return func(g *opsstate.MergeLine, x maRow, now time.Time) {
		if x.M != LOCAL {
			m := dash0(x.M)
			if m == "" {
				m = "모름"
			}
			g.Why = "병합기 바퀴 시각·검증 시간·건너뜀은 병합기 기계(" + m + ") 일지에만 — 원격으로 안 읽음"
			return
		}
		if bad != "" {
			g.Why = bad
			return
		}
		keep := func(string) bool { return true }
		if local > 1 {
			keep = func(l string) bool { return proj[l] == g.Project }
		}
		f := opsstate.MergeJournal(ls, stateJournalN, keep)
		g.Lands, g.Skipped, g.Delegated = ptrI(f.Lands), ptrI(f.Skipped), ptrI(f.Delegated)
		g.RecentVerifyS = f.VerifyS
		if f.Lands > 0 {
			g.SkipRatio = ptrF(float64(f.Skipped) / float64(f.Lands))
		}
		if c := cwd[x.Addr]; x.St == "alive" && c != "" {
			if ln, at, ok := stateLanding(c, dir+"/hc-lane/merger-logs"); ok {
				g.Landing, g.LandingAgeS = ln, ptrI(max(0, int(now.Sub(at).Seconds())))
			}
			if md, ok := mergerGitDir(c); ok && g.Length != nil && *g.Length > 0 {
				g.SpecMachine, g.SpecTerm, g.SpecFresh, g.SpecWhy = stateSpecWait(md, terms, knobInt("advise_spec_fresh"), now)
			}
		}
		if x.St == "alive" && g.Landing == "" && g.Length != nil && *g.Length > 0 {
			stateLandOn(g, f, terms, knobInt("advise_spec_fresh"), now)
		}
		if f.LastAt.IsZero() {
			g.Why = "이 기계 병합기 일지에 이 프로젝트 줄 없음"
			return
		}
		g.LastRoundAgeS = ptrI(int(now.Sub(f.LastAt).Seconds()))
	}
}

// landLogRe: 병합기 착지 로그 이름 <맨 앞 레인>-<YYYYmmdd-HHMMSS 이 기계 시각>-<pid>.log(hc-merger round.go m.lgf). 미리 훑기·충돌 풀기
// 로그(<레인>-prescan-…·<레인>-resolve-…)와 추측 미리 줄 로그(pre-<날짜>.log)는 착지가 아니다
var landLogRe = regexp.MustCompile(`^(.+)-([0-9]{8}-[0-9]{6})-([0-9]+)\.log$`)

// stateLanding <병합기 클론> <착지 로그 폴더>: 이 기계 병합기가 지금 착지 명령(준비 훅·hc-lane land)을 돌리는 중이면 그 맨 앞 레인과
// 착지 시작 시각(advise-merge-stall-age). 착지 중 = 클론의 <git-path hc-merger>/child 기록(병합기가 착지 자식을 띄울 때 「pid<탭>시작
// 시각」을 쓰고 끝나면 지운다 — internal/proc WaitFor)의 그 pid가 그 시작 시각으로 살아 있다. 읽기만 한다(proc.ChildCheck는 남은 기록을
// 지워 쓰지 않는다). 기록·ps를 못 읽으면 착지 중이 아니라고 하지 않고 ok=false — advise는 그때 지금처럼 안 움직임을 본다(놓치지 않는 쪽).
// 레인·시각은 그 자식의 부모(= 병합기 프로세스) pid로 끝나는 착지 로그 가운데 가장 늦은 것 — 로그 이름 끝 pid가 병합기 pid라 한 기계에
// 병합기가 여럿이어도, 그 착지가 대기열 줄 없는 레인이어도 지난 착지 로그를 고르지 않는다(감사 atp07wf 중간 1). 부모 pid·로그를 못 찾으면
// 레인 「모름」·기록을 쓴 시각(착지 자식이 뜬 때)
// ponytail: 병합기 기계에서만 잰다 — 다른 기계 병합기는 원격으로 읽지 않는다(일지와 같음)
func stateLanding(clone, logs string) (lane string, at time.Time, ok bool) {
	rec, ok := mergerGitDir(clone)
	if !ok {
		return "", time.Time{}, false
	}
	rec += "/child"
	b, err := os.ReadFile(rec)
	if err != nil {
		return "", time.Time{}, false
	}
	f := strings.SplitN(strings.TrimRight(string(b), "\n"), "\t", 2)
	pid, err := strconv.Atoi(f[0])
	if err != nil || pid <= 1 || len(f) < 2 || !proc.Alive(pid) {
		return "", time.Time{}, false
	}
	if st, err := proc.StartTime(pid); err != nil || st != f[1] {
		return "", time.Time{}, false
	}
	lane = "모름"
	if fi, err := os.Stat(rec); err == nil {
		at = fi.ModTime()
	}
	pp, rc := run("ps", "-o", "ppid=", "-p", f[0])
	mpid := strings.TrimSpace(pp)
	if rc != 0 || mpid == "" || mpid == "1" {
		return lane, at, !at.IsZero()
	}
	var best time.Time
	es, _ := os.ReadDir(logs) // 못 읽으면 레인 모름·기록 시각
	for _, e := range es {
		m := landLogRe.FindStringSubmatch(e.Name())
		if m == nil || m[3] != mpid || strings.HasSuffix(m[1], "-prescan") || strings.HasSuffix(m[1], "-resolve") {
			continue
		}
		if t, err := time.ParseInLocation("20060102-150405", m[2], time.Local); err == nil && t.After(best) {
			lane, best = m[1], t
		}
	}
	if !best.IsZero() {
		at = best
	}
	return lane, at, !at.IsZero()
}

// mergerGitDir <병합기 클론>: 병합기 기록 폴더 <git-path hc-merger>(child·specwait가 있는 곳 — hc-merger m.mj)
func mergerGitDir(clone string) (string, bool) {
	gp, rc := run("git", "-C", clone, "rev-parse", "--git-path", "hc-merger")
	if rc != 0 || sub(gp) == "" {
		return "", false
	}
	d := sub(gp)
	if !filepath.IsAbs(d) {
		d = filepath.Join(clone, d)
	}
	return d, true
}

var (
	specIDRe = regexp.MustCompile(`^spec-([0-9a-f]{8})$`)
	landIDRe = regexp.MustCompile(`^land-([0-9a-f]{8})$`) // 워커 착지 터미널 land-<차례 8>(hc-merger landon.go runRemote)
)

// stateFresh <기계> <폴더 종류 spec·land> <터미널 이름 꼴> <살아 있는 터미널> <분>: 그 기계의 살아 있는 <종류>-<아이디8> 터미널(첫
// 주소, 없으면 빈 값)과 그 로그·작업 트리가 분 안에 갱신됐나. 판정 본문은 병합기의 맨 앞 기다림 상한과 같은 verified.FreshSh(한 곳 —
// 터미널마다 인자 「주소 클론 아이디8」 셋씩, 첫 인자 = 분 — onm의 머리말이 $d로). 그 기계를 못 읽으면 why에 까닭(갱신 아님 쪽)
func stateFresh(mach, kind, label string, re *regexp.Regexp, terms []hcjson.Terminal, min int) (term string, fresh bool, why string) {
	args := []string{strconv.Itoa(min)}
	for _, t := range terms {
		m, id := LOCAL, re.FindStringSubmatch(t.Name)
		if a, _, ok := strings.Cut(t.Address, "/"); ok {
			m = a
		}
		if t.State != "live" || id == nil || m != mach || t.Cwd == "" {
			continue
		}
		if term == "" {
			term = t.Address
		}
		args = append(args, t.Address, t.Cwd, id[1])
	}
	if term == "" {
		return
	}
	out, rc := onm(nil, mach, "n=$d; k="+kind+"; "+verified.FreshSh, args...)
	if rc != 0 {
		return term, false, fmt.Sprintf("%s 갱신 못 읽음(%s 종료 %d)", label, mach, rc)
	}
	if a := strings.TrimSpace(out); a != "" {
		return a, true, ""
	}
	return term, false, ""
}

// stateLandOn: 이 기계 병합기 일지의 마지막 줄이 워커 착지 실행 결정(f.LandOn — opsstate.LandRemote)이면 그 워커·나이·land 터미널·갱신을
// g에 채운다(ht-advise-land-remote — 워커 착지를 기다리는 동안 일지는 그 한 줄뿐이라 안 움직임이 올라갔다). 갱신 중이면 병합기가 그 레인을
// 착지 중(Landing — advise는 안 움직임이 아니라 느림 문턱과 견준다). 손잡이 advise_spec_fresh 0이면 보지 않는다(추측과 같은 손잡이)
func stateLandOn(g *opsstate.MergeLine, f opsstate.JournalFacts, terms []hcjson.Terminal, min int, now time.Time) {
	if f.LandOn == "" || min <= 0 {
		return
	}
	g.LandOn, g.LandOnAgeS = f.LandOn, ptrI(max(0, int(now.Sub(f.LandOnAt).Seconds())))
	g.LandOnTerm, g.LandOnFresh, g.LandOnWhy = stateFresh(f.LandOn, "land", "워커 착지", landIDRe, terms, min)
	if g.LandOnFresh {
		g.Landing, g.LandingAgeS = f.LandOnLane, g.LandOnAgeS
	}
}

// stateSpecWait <병합기 기록 폴더> <살아 있는 터미널> <분>: 병합기가 맨 앞 추측 검증을 기다리는 중이면(specwait 첫 줄 「첫 마지막 횟수
// [기계[/사슬]]」의 마지막 기다림 바퀴가 분 안 — 병합기는 기다리는 바퀴마다 고쳐 쓰고 기다림이 끝나면 지운다, hc-merger jround.go
// specWaitNote) 그 검증 기계, 그 기계의 살아 있는 spec-<아이디8> 터미널 주소, 그 추측이 분 안에 갱신됐나. 기다림이 아니거나 맡길 기계를
// 기다림(-)이면 빈 값(지금 그대로 안 움직임을 본다). 갱신을 못 읽으면 why에 까닭 — 갱신 없음과 같이 안 움직임 쪽(놓치지 않는 쪽)
// ponytail: 한 기계에 추측 하나(hc-merger spec.go)라 그 기계의 spec 터미널을 다 본다 — 아이디를 specwait에 적으면 하나로
func stateSpecWait(dir string, terms []hcjson.Terminal, min int, now time.Time) (mach, term string, fresh bool, why string) {
	if mach, _, _ = specWaitRec(dir, min, now); mach == "" {
		return
	}
	term, fresh, why = stateFresh(mach, "spec", "추측 검증", specIDRe, terms, min)
	return
}

// specWaitRec <병합기 기록 폴더> <분> <지금>: specwait(hc-merger jround.go specWaitRead와 같은 꼴 — 첫 줄 「첫 마지막 횟수 [기계[/사슬]]」,
// 구성원마다 「큐 참조 제출 레인 기록」)의 맡긴 검증 기계·기다림 첫 초·구성원 레인. 기다림 아님·마지막 바퀴가 분 넘음·맡길 기계를 기다림(-)·
// 손잡이 0이면 빈 값. 읽기만 한다
func specWaitRec(dir string, min int, now time.Time) (mach string, first int64, lanes []string) {
	b, err := os.ReadFile(dir + "/specwait")
	if err != nil || min <= 0 {
		return
	}
	ls := strings.Split(strings.TrimSpace(string(b)), "\n")
	h := strings.Fields(ls[0])
	if len(h) != 4 {
		return
	}
	first, err1 := strconv.ParseInt(h[0], 10, 64)
	last, err2 := strconv.ParseInt(h[1], 10, 64)
	if err1 != nil || err2 != nil || now.Unix()-last > int64(min)*60 {
		return "", 0, nil
	}
	if mach, _, _ = strings.Cut(h[3], "/"); mach == "-" {
		return "", 0, nil
	}
	for _, l := range ls[1:] {
		if f := strings.Fields(l); len(f) == 4 {
			lanes = append(lanes, f[2])
		}
	}
	return mach, first, lanes
}

func stateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return d
	}
	return HOME + "/.local/state"
}

// mqBatch <병합기 살아 있음 기록> <살아 있는 터미널 주소 → 폴더> <지금>: 이 기계 병합기가 지금 도는 묶음의 구성원 → 단계 덧말 —
// 추측 검증을 기다리는 묶음(specwait)은 「묶음 검증 중 <기계> n분째」, 착지 중인 맨 앞 레인(stateLanding)은 「착지 중 n분째」.
// 다른 기계 병합기·살아 있지 않음·클론 모름이면 nil(원격으로 읽지 않는다 — 차례만 보인다)
// ponytail: 착지는 맨 앞 레인만(착지 로그 이름) — 묶음 착지의 뒤 구성원은 차례만 보인다. 워커 착지(LandOn)도 차례만
func mqBatch(x maRow, cwd map[string]string, now time.Time) map[string]string {
	c := cwd[x.Addr]
	if x.M != LOCAL || x.St != "alive" || c == "" {
		return nil
	}
	o := map[string]string{}
	if md, ok := mergerGitDir(c); ok {
		if mach, first, lanes := specWaitRec(md, knobInt("advise_spec_fresh"), now); mach != "" {
			for _, l := range lanes {
				o[l] = fmt.Sprintf("묶음 검증 중 %s %d분째", mach, max(0, now.Unix()-first)/60)
			}
		}
	}
	if ln, at, ok := stateLanding(c, stateDir()+"/hc-lane/merger-logs"); ok && ln != "모름" {
		o[ln] = fmt.Sprintf("착지 중 %d분째", max(0, int(now.Sub(at).Minutes())))
	}
	return o
}
