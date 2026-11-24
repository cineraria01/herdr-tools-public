package main

// 상주 판 점검(2026-11-24 stale-resident-check, internal/resident, docs/hc-sweep/resident.md): 건강 점검 길에서 이 기계의 상주 도구가
// 지금 깔린 판으로 도는지 보고, 옛 판·판 모름이 유예(손잡이 sweep_resident_grace)를 넘기면 운영 사건 hc-stale-resident 한 번.
// 이 기계만 본다(다른 기계는 그 기계 hc-sweep이). 프로세스를 끄거나 다시 띄우지 않는다 — 사건의 「다시 띄우기」 줄은 지휘가 부른다.
// 재기는 ps 한 번(+ 표시 없는 상주마다 그 환경 ps 한 번)과 표시 파일 읽기뿐이다(열린 파일 조회 없음).

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"herdr-tools/internal/resident"
)

// residentCheck: 한 바퀴. report(msg, key)는 사건 한 줄. 찍을 줄들을 돌려준다
func residentCheck(machine, dir string, dry bool, r runner, report func(msg, key string) bool, now time.Time, grace time.Duration) []string {
	env := append(os.Environ(), "LC_ALL=C")
	rp := resident.ReportPath(dir)
	prev, err := resident.LoadReport(rp)
	if err != nil {
		prev = resident.Report{} // 깨진 한 장은 없는 것으로(보낸 키를 잊어 한 번 더 알릴 수 있다 — 놓치지 않는 쪽)
	}
	unknown := func(why string) []string {
		if !dry {
			prev.Unknown = why
			if err := resident.SaveReport(rp, prev); err != nil {
				return []string{"상주 판 점검: 모름(" + why + ") · 결과를 못 씀: " + err.Error()}
			}
		}
		return []string{"상주 판 점검: 모름(" + why + ") — 다음 바퀴에"}
	}
	res := r(15*time.Second, env, "/bin/ps", "-axww", "-o", "pid=,lstart=,command=")
	if res.err != nil || res.rc != 0 {
		return unknown("ps 실패 " + rcText(res))
	}
	procs := resident.ParsePS(res.out, time.Local)
	if len(procs) == 0 {
		return unknown("ps 출력을 풀지 못함")
	}
	marks, err := resident.Load(dir)
	if err != nil {
		return unknown("판 표시를 못 읽음: " + err.Error())
	}
	alive := map[int]bool{}
	for i, p := range procs {
		alive[p.Pid] = true
		if _, ok := resident.KindOf(p.Args); !ok || now.Sub(p.Start) < grace {
			continue
		}
		if _, ok := resident.Marked(p, marks); ok {
			continue
		}
		// 표시 없는 상주: 다시 띄우기 줄을 쓰려고 그 환경의 hc 터미널·launchd 라벨만(못 읽으면 빈 값 — 줄이 「kill <pid>」로)
		if e := r(10*time.Second, env, "/bin/ps", "-wwwE", "-p", strconv.Itoa(p.Pid), "-o", "command="); e.err == nil && e.rc == 0 {
			procs[i].Term, procs[i].Label = resident.EnvOf(e.out)
		}
	}
	stat := func(p string) (resident.Bin, bool) {
		fi, err := os.Stat(p)
		if err != nil {
			return resident.Bin{}, false
		}
		return resident.Bin{Size: fi.Size(), Mtime: fi.ModTime()}, true
	}
	fs := resident.Scan(procs, marks, stat, now, grace)
	var lines []string
	sent := map[string]string{}
	for _, f := range fs {
		key := f.Key(machine)
		if at, ok := prev.Sent[key]; ok {
			sent[key] = at // 같은 프로세스·같은 깔린 판에는 한 번
			continue
		}
		msg := f.Text() + " (" + machine + ")"
		switch {
		case dry:
			lines = append(lines, "상주 판 점검 (보낼 것): "+msg)
		case report(msg, key):
			sent[key] = now.Format(time.RFC3339)
			lines = append(lines, "상주 판 점검: "+msg)
		default:
			lines = append(lines, "상주 판 점검: "+msg+" — 알림 실패(hc-lane report), 다음 바퀴에 같은 키로 다시")
		}
	}
	if dry {
		return lines
	}
	if n := resident.Prune(dir, marks, alive); n > 0 {
		lines = append(lines, fmt.Sprintf("상주 판 점검: 끝난 프로세스의 판 표시 %d장 지움", n))
	}
	if err := resident.SaveReport(rp, resident.Report{Checked: now, Findings: fs, Sent: sent}); err != nil {
		lines = append(lines, "상주 판 점검 결과를 못 씀: "+err.Error())
	}
	return lines
}
