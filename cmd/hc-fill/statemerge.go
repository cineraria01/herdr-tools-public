package main

// state의 병합 줄 칸(2026-11-24 ops-state-trim): 병합기가 이 기계(LOCAL)에 있는 프로젝트는 이 기계 병합기 일지
// (<XDG_STATE_HOME, 없으면 ~/.local/state>/hc-lane/merger-journal.tsv — hc-merger result.go jrec)에서 마지막 바퀴·최근 검증·건너뜀·맡김을
// 읽는다(읽기만). 다른 기계 병합기는 모름+까닭(그 일지를 원격으로 읽지 않는다). 원격 호출은 하나뿐 — 이 기계 병합기가 맨 앞 추측 검증을
// 기다리는 중이면 그 검증 기계에 find 한 번(hc exec, 상한 proc.Limit — stateSpecWait).

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
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		dir = HOME + "/.local/state"
	}
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

// specFreshSh: 추측 검증 터미널마다(인자 「주소 클론 아이디8」 셋씩, 첫 인자 = 분) 그 로그 <클론>.spec-wt/<아이디8>.log나 작업 트리
// <클론>.spec-wt/<아이디8>(node_modules·.git 빼고)에 그 분 안에 쓴 파일이 있으면 그 주소 한 줄. 없는 경로의 find 오류는 「갱신 없음」 —
// 끝까지 오면 종료 0이라, 종료가 0이 아니면 그 기계에서 못 돌았다(모름). <클론>은 일꾼처럼 git 최상위(링크를 푼 경로, hc-merger
// specverify.go top), 못 구하면 터미널 폴더 그대로(감사 atp1onr 낮음 4 — 링크 폴더면 늘 「갱신 없음」이었다)
// ponytail: 멈춘 검증이면 작업 트리 전체를 훑는다(갱신 중이면 첫 파일에서 -quit) — 너무 느려지면 검증 스크립트가 진행 표식을 한 곳에
const specFreshSh = `n=$d; while [ $# -ge 3 ]; do c=$(cd "$2" 2>/dev/null && git rev-parse --show-toplevel 2>/dev/null) || c=$2; w="$c.spec-wt/$3"; ` +
	`if [ -n "$(find "$w.log" "$w" \( -name node_modules -o -name .git \) -prune -o -type f -mmin -"$n" -print -quit 2>/dev/null)" ]; then echo "$1"; exit 0; fi; ` +
	`shift 3; done; exit 0`

var specIDRe = regexp.MustCompile(`^spec-([0-9a-f]{8})$`)

// stateSpecWait <병합기 기록 폴더> <살아 있는 터미널> <분>: 병합기가 맨 앞 추측 검증을 기다리는 중이면(specwait 첫 줄 「첫 마지막 횟수
// [기계[/사슬]]」의 마지막 기다림 바퀴가 분 안 — 병합기는 기다리는 바퀴마다 고쳐 쓰고 기다림이 끝나면 지운다, hc-merger jround.go
// specWaitNote) 그 검증 기계, 그 기계의 살아 있는 spec-<아이디8> 터미널 주소, 그 추측이 분 안에 갱신됐나. 기다림이 아니거나 맡길 기계를
// 기다림(-)이면 빈 값(지금 그대로 안 움직임을 본다). 갱신을 못 읽으면 why에 까닭 — 갱신 없음과 같이 안 움직임 쪽(놓치지 않는 쪽)
// ponytail: 한 기계에 추측 하나(hc-merger spec.go)라 그 기계의 spec 터미널을 다 본다 — 아이디를 specwait에 적으면 하나로
func stateSpecWait(dir string, terms []hcjson.Terminal, min int, now time.Time) (mach, term string, fresh bool, why string) {
	b, err := os.ReadFile(dir + "/specwait")
	if err != nil || min <= 0 {
		return
	}
	h := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
	if len(h) != 4 {
		return
	}
	last, err := strconv.ParseInt(h[1], 10, 64)
	if err != nil || now.Unix()-last > int64(min)*60 {
		return
	}
	mach, _, _ = strings.Cut(h[3], "/")
	if mach == "-" {
		return "", "", false, ""
	}
	args := []string{strconv.Itoa(min)}
	for _, t := range terms {
		m, id := LOCAL, specIDRe.FindStringSubmatch(t.Name)
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
	out, rc := onm(nil, mach, specFreshSh, args...)
	if rc != 0 {
		return mach, term, false, fmt.Sprintf("추측 검증 갱신 못 읽음(%s 종료 %d)", mach, rc)
	}
	if a := strings.TrimSpace(out); a != "" {
		return mach, a, true, ""
	}
	return
}
