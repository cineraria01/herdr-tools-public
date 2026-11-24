package main

// state의 배포 칸(2026-11-24 ops-state): 기계마다 「기록상」 깔린 판 — herdr-sync 배포 기록(<HOME>/.local/state/herdr-sync/deploy.tsv,
// docs/herdr-sync/deploy.md)에서 그 기계 결과가 같음·씀인 마지막 줄의 범용 커밋 — 과 origin/main에서 밀린 커밋 수(범용 저장소에서
// git rev-list, fetch 없이 마지막으로 받은 origin/main 기준). 기계에 닿아 실제 바이너리를 보지 않는다(그건 herdr-sync --status) —
// 기록 뒤 기계에서 따로 바뀐 것은 여기서 안 보인다. 기록이 없거나 저장소를 못 읽으면 모름.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/opsstate"
	"herdr-tools/internal/proc"
)

// stateRepo: 범용 저장소(herdr-sync와 같은 기본 — HERDR_TOOLS_REPO, 없으면 그 기본 자리)
func stateRepo() string {
	if r := os.Getenv("HERDR_TOOLS_REPO"); r != "" {
		return r
	}
	return HOME + "/Documents/app/herdr-tools"
}

// deployMachine: 배포 기록의 기계 이름 — herdr-sync는 이 기계를 설정 local 값으로 적고(sync-local-name), 그 전 기록과 local 줄이 없는
// 기계는 「맥」으로 적었다. 둘을 한 기계(local)로 읽는다(기록은 고쳐 쓰지 않는다)
func deployMachine(m string) string {
	if m == "맥" && LOCAL != "" {
		return LOCAL
	}
	return m
}

// deployInstalled: 배포 기록 줄들 → 기계마다 마지막으로 같음·씀이 된 커밋 12자와 그 줄 시각, 기계 차례(처음 본 차례), 마지막 줄 시각
func deployInstalled(lines []string) (ver map[string]string, order []string, lastAt time.Time) {
	ver = map[string]string{}
	for _, l := range lines {
		f := strings.Split(l, "\t")
		if len(f) < 8 {
			continue
		}
		if t, err := time.Parse(time.RFC3339, f[0]); err == nil {
			lastAt = t
		}
		for _, mr := range strings.Fields(f[7]) {
			m, r, ok := strings.Cut(mr, "=")
			if !ok {
				continue
			}
			m = deployMachine(m)
			if _, seen := ver[m]; !seen {
				order = append(order, m)
				ver[m] = ""
			}
			if (r == "같음" || r == "씀") && f[3] != "?" {
				ver[m] = f[3]
			}
		}
	}
	return ver, order, lastAt
}

// deploySkipOnly: 배포 기록에 결과가 건너뜀뿐인 기계(이 기계 판의 배포 대상이 아님 — ops-state-trim)
func deploySkipOnly(lines []string) map[string]bool {
	o := map[string]bool{}
	for _, l := range lines {
		f := strings.Split(l, "\t")
		if len(f) < 8 {
			continue
		}
		for _, mr := range strings.Fields(f[7]) {
			if m, r, ok := strings.Cut(mr, "="); ok {
				m = deployMachine(m)
				_, seen := o[m]
				o[m] = r == "건너뜀" && (!seen || o[m])
			}
		}
	}
	return o
}

// deployHold: 배포 밀림을 아직 따지지 않을 까닭(opsstate.Facts.DeployHold) — 배포가 지금 돈다(herdr-sync 표식 deploying의 pid가 살아 있음) ·
// 착지 뒤 명령 차례(onland pending·running)가 남음 · 마지막 착지(착지마다 기준 onland/*.head가 바뀐 때, 범용 저장소 origin/main 커밋 시각
// 가운데 늦은 것)가 손잡이 deploy_grace분 안. 없으면 빈 값(2026-11-24 지휘 — 착지 뒤 배포가 도는 1~2분에 밀림 사건이 거짓으로 나갔다)
func deployHold(now time.Time) string {
	if b, err := os.ReadFile(HOME + "/.local/state/herdr-sync/deploying"); err == nil {
		f := strings.Split(strings.TrimSpace(string(b)), "\t")
		if pid, _ := strconv.Atoi(f[0]); pid > 0 && proc.Alive(pid) {
			return "배포 도는 중(herdr-sync pid " + f[0] + ")"
		}
	}
	// 착지 뒤 명령은 배포(herdr-sync를 부르는 on-land 줄)만 — 다른 프로젝트의 빌드 등은 배포 밀림과 무관하다(감사 atozuyd 낮음)
	deploys := func(name string) bool { _, c := onlandConf(name); return strings.Contains(c, "herdr-sync") }
	for _, n := range onlandLeft() {
		if deploys(n) {
			return "착지 뒤 배포 차례 남음(" + n + ")"
		}
	}
	g := knobInt("deploy_grace")
	if g == 0 {
		return ""
	}
	var last time.Time
	if hs, err := filepath.Glob(onlandDir() + "/*.head"); err == nil {
		for _, h := range hs {
			if st, err := os.Stat(h); err == nil && st.ModTime().After(last) && deploys(strings.TrimSuffix(filepath.Base(h), ".head")) {
				last = st.ModTime()
			}
		}
	}
	if out, rc := run("git", "-C", stateRepo(), "log", "-1", "--format=%ct", "origin/main"); rc == 0 {
		if s, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64); err == nil && time.Unix(s, 0).After(last) {
			last = time.Unix(s, 0)
		}
	}
	if a := now.Sub(last); !last.IsZero() && a < time.Duration(g)*time.Minute { // now 뒤(음수)도 방금 착지
		return fmt.Sprintf("마지막 착지 %d분 전(손잡이 deploy_grace %d분 안)", max(0, int(a.Minutes())), g)
	}
	return ""
}

// deployHoldIf: 밀린 기계가 있을 때만 보류 까닭을 잰다(deployHold), 없으면 빈 값
func deployHoldIf(ds []opsstate.Deploy, now time.Time) string {
	for _, d := range ds {
		if d.Behind != nil && *d.Behind > 0 {
			return deployHold(now)
		}
	}
	return ""
}

// stateDeploy: s.Deploy를 채우고 재료 한 줄을 돌려준다
func stateDeploy(s *opsstate.State, now time.Time) opsstate.Source {
	src := opsstate.Source{Name: "deploy"}
	b, ok := readFile(HOME + "/.local/state/herdr-sync/deploy.tsv")
	if !ok {
		src.Why = "배포 기록 없음(herdr-sync deploy.tsv) — 이 기계에서 배포한 적 없음"
		return src
	}
	ver, order, at := deployInstalled(lines(b))
	skip := deploySkipOnly(lines(b))
	if !at.IsZero() {
		src.AgeS = ptrI(int(now.Sub(at).Seconds()))
	}
	t0 := time.Now()
	behind := map[string]*int{}
	repo := stateRepo()
	for _, m := range order {
		d := opsstate.Deploy{Machine: m, Version: ver[m], Why: "배포 기록상 판"}
		if d.Version == "" {
			d.Why = "기록에 깐 판 없음(실패·충돌만)"
			if skip[m] {
				d.Why = opsstate.DeployNotTarget
			}
			s.Deploy = append(s.Deploy, d)
			continue
		}
		n, seen := behind[d.Version]
		if !seen {
			src.Calls++
			if out, rc := run("git", "-C", repo, "rev-list", "--count", d.Version+"..origin/main"); rc == 0 {
				if k, err := strconv.Atoi(strings.TrimSpace(out)); err == nil {
					n = ptrI(k)
				}
			}
			behind[d.Version] = n
		}
		if n == nil {
			d.Why = "밀린 커밋 수 모름(범용 저장소 git rev-list 실패)"
		}
		d.Behind = n
		s.Deploy = append(s.Deploy, d)
	}
	src.Secs = time.Since(t0).Seconds()
	return src
}
