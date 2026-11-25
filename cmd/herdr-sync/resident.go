package main

// 배포 뒤 상주 확인(2026-11-25 ht-resident-reexec — 실행 파일은 아홉 대 모두 새 판인데 오래 도는 고리 몇이 시작 때 읽은 옛 판으로 돌아,
// 배포가 「됐다」고 보고된 뒤에도 일부만 먹고 있었다): 기계마다 그 기계 ~/.local/bin의 실행 파일로 도는 herdr-tools 상주(resident.Kinds)
// 가운데 판 표시(selfexec.New가 남김)가 지금 깔린 파일과 다른 것(옛 판)·표시가 없는 것(판 모름 — 스스로 못 넘어감)을 센다.
// 판 표시의 커밋이 이번 판(범용 저장소 HEAD)과 같으면 실행 파일만 다시 쓰인 것이라 세지 않는다.
//   - 배포: 깐 직후라 유예 없이 「넘어가는 중 n개」를 보인다(다름 아님 — 상주가 바퀴 사이에 스스로 넘어간다).
//   - --check: 유예(손잡이 sweep_resident_grace, 기본 5분)를 넘긴 것만 「다름:상주」(종료 1). 몇 바퀴 안에 못 넘어간 것의 운영 사건은
//     그 기계 hc-sweep 상주 판 점검(hc-stale-resident, 같은 프로세스·같은 판에 한 번)이 낸다 — 여기서 또 내지 않는다.
// 조회는 기계마다 hc exec 한 번(ps 한 번·표시 파일·실행 파일 stat). 고치지 않는다.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/knob"
	"herdr-tools/internal/resident"
)

// residentSh: H(HOME)·P(ps 줄)·M(판 표시 한 장)·B(실행 파일 경로·크기·수정 초), 끝에 END. 인자 = 상주 실행 파일 이름들.
// stat은 GNU -c 먼저(BSD는 -c에서 출력 없이 실패) — GNU의 -f는 파일 시스템 정보를 여러 줄 찍고 실패해 B 줄을 깬다(감사 atp2cjf 중간 1)
const residentSh = `h=$HOME; d=${XDG_STATE_HOME:-$h/.local/state}/herdr-tools/resident/pids
printf 'H\t%s\n' "$h"
ps_=$(LC_ALL=C ps -axww -o pid=,lstart=,command=) || exit 1
printf '%s\n' "$ps_" | awk '{print "P\t" $0}'
for f in "$d"/*.json; do [ -f "$f" ] || continue; printf 'M\t%s\n' "$(tr -d '\n' < "$f")"; done
for t in "$@"; do f=$h/.local/bin/$t; [ -f "$f" ] || continue
  s=$(stat -c '%s %Y' "$f" 2>/dev/null || stat -f '%z %m' "$f" 2>/dev/null) || continue
  printf 'B\t%s\t%s\n' "$f" "$s"; done
echo END
`

// residentBins: 상주 실행 파일 이름(겹침 없이)
func residentBins() []string {
	var o []string
	for _, k := range resident.Kinds {
		if !strings.Contains(" "+strings.Join(o, " ")+" ", " "+k.Bin+" ") {
			o = append(o, k.Bin)
		}
	}
	return o
}

type resProbe struct {
	home  string
	procs []resident.Proc
	marks map[int]resident.Mark
	bins  map[string]resident.Bin
}

// residentParse: 조회 출력 → 그 기계 상태. ok 거짓 = 모름(끝이 잘림·모르는 줄·H 없음)
func residentParse(out string) (resProbe, bool) {
	p := resProbe{marks: map[int]resident.Mark{}, bins: map[string]resident.Bin{}}
	ls := splitLines(out)
	if len(ls) == 0 || ls[len(ls)-1] != "END" {
		return p, false
	}
	var ps []string
	for _, l := range ls[:len(ls)-1] {
		k, v, _ := strings.Cut(l, "\t")
		switch k {
		case "H":
			p.home = v
		case "P":
			ps = append(ps, v)
		case "M":
			var m resident.Mark
			if json.Unmarshal([]byte(v), &m) == nil && m.Pid > 0 {
				m.Mtime = m.Mtime.Truncate(time.Second) // stat은 초까지라 초로 견준다
				p.marks[m.Pid] = m
			}
		case "B":
			path, sz, _ := strings.Cut(v, "\t")
			f := strings.Fields(sz)
			if len(f) != 2 {
				return p, false
			}
			n, err1 := strconv.ParseInt(f[0], 10, 64)
			s, err2 := strconv.ParseInt(f[1], 10, 64)
			if err1 != nil || err2 != nil {
				return p, false
			}
			p.bins[path] = resident.Bin{Size: n, Mtime: time.Unix(s, 0)}
		default:
			return p, false
		}
	}
	// ponytail: ps 시작 시각은 맥의 시간대로 푼다 — 기계마다 시간대가 다르면 유예가 그만큼 어긋난다
	p.procs = resident.ParsePS(strings.Join(ps, "\n"), time.Local)
	return p, p.home != "" && len(p.procs) > 0
}

// residentStale: 그 기계 HOME 아래 실행 파일로 도는 상주 가운데 옛 판·판 모름(유예를 넘긴 것, 판 표시 커밋이 cur인 것은 뺌)
func residentStale(p resProbe, now time.Time, grace time.Duration, cur string) []resident.Finding {
	var mine []resident.Proc
	for _, x := range p.procs {
		path := x.Args[0]
		if m, ok := resident.Marked(x, p.marks); ok {
			path = m.Path
		}
		// ponytail: 맨 이름으로 띄운(argv[0]에 경로 없는) 표시 없는 상주는 그 기계 것인지 몰라 안 센다
		if strings.HasPrefix(path, p.home+"/") {
			mine = append(mine, x)
		}
	}
	stat := func(path string) (resident.Bin, bool) { b, ok := p.bins[path]; return b, ok }
	var o []resident.Finding
	for _, f := range resident.Scan(mine, p.marks, stat, now, grace) {
		if m := p.marks[f.Pid]; f.State == "old" && cur != "" && cur != "?" && m.Commit == cur {
			continue
		}
		o = append(o, f)
	}
	return o
}

// residentGrace: sweep_resident_grace(분 — 이 기계 손잡이 > 설정 파일 > 기본, 손잡이 파일이 깨졌으면 설정 파일·기본)
func residentGrace() time.Duration {
	v, _ := knob.Local(home, "sweep_resident_grace", os.Stderr, time.Now())
	n, _ := strconv.Atoi(v)
	return time.Duration(n) * time.Minute
}

// residentNote: 기계 m(이 기계는 LOCAL) 상주를 재어 줄을 찍는다. --check면 유예를 넘긴 것을 다름으로(diffs에 「상주」)
func residentNote(m string, diffs *[]string) {
	args := append([]string{"-c", residentSh, "_"}, residentBins()...)
	c := exec.Command("sh", args...)
	if m != LOCAL {
		c = exec.Command("hc", append([]string{"exec", "--timeout", "60", "--on", m, "--", "sh"}, args...)...)
	}
	out, rc := output(c)
	p, ok := residentParse(out)
	if rc != 0 || !ok {
		fmt.Fprintf(os.Stderr, "herdr-sync: %s 상주 판을 못 셈(종료 %d) — 모름, 다름으로 세지 않는다\n", m, rc)
		return
	}
	grace := time.Duration(0)
	if check {
		grace = residentGrace()
	}
	fs := residentStale(p, time.Now(), grace, head12(REPO))
	if len(fs) == 0 {
		return
	}
	if check {
		for _, f := range fs {
			say("%s 다름:상주 %s", m, f.Text())
		}
		*diffs = append(*diffs, "상주")
		return
	}
	var who []string
	for _, f := range fs {
		w := fmt.Sprintf("%s(pid %d)", f.Who, f.Pid)
		if f.State == "unknown" {
			w += " 판 모름 — 스스로 못 넘어감, 다시 띄우기: " + f.Restart
		}
		who = append(who, w)
	}
	say("상주 %s: 아직 옛 판 %d개 — %s (바퀴 사이에 새 판으로 스스로 넘어간다 · 남으면 herdr-sync --check)", m, len(fs), strings.Join(who, " · "))
}
