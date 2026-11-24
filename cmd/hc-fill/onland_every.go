package main

// 착지마다(설정 줄 「land-every <이름>」, 2026-11-24 deploy-log-rollback — 착지 뒤 명령은 레인마다 add --on-land로 켜야 해서 배포
// 알림을 놓쳤다): 그 이름 on-land 줄 프로젝트의 기본 가지가 앞서면(어느 레인이든, --on-land 없이) 그 사이 커밋의 제목 머리(레인)마다
// pending에 한 줄을 넣는다 — 실행기가 한 번에 걷어 명령을 한 번 돌리니 잇단 착지는 한 번으로 합쳐진다(on-land와 같은 잠금·합침).
//   DATA/onland/<이름>.head  마지막으로 본 기본 가지 커밋. 없으면 첫 바퀴는 기준만 적고 돌리지 않는다(켠 뒤의 착지부터)
// 사건 주인은 없음(-) — hc-lane report가 그 저장소를 맡은 지휘에 보낸다.

import (
	"os"
	"strings"

	"herdr-tools/internal/atomic"
)

// onlandEvery: 바퀴마다 — 앞선 land-every 이름을 fire에 넣는다(실행기는 onlandTick이 띄운다)
func onlandEvery(fire map[string]bool) {
	for _, f := range confRows("land-every") {
		if len(f) != 2 || !okname(f[1]) {
			say("착지마다: 설정 줄 꼴이 「land-every <이름>」이 아니다: %s", strings.Join(f, " "))
			continue
		}
		name := f[1]
		p, c := onlandConf(name)
		if c == "" {
			say("착지마다 %s: 설정에 「on-land %s <프로젝트> <명령…>」 줄이 없다", name, name)
			continue
		}
		if fetchProj(p) != 0 {
			say("착지마다 %s: 판정 못 함(%s fetch·repo 줄) — 다음 바퀴에", name, p)
			continue
		}
		h, rc := run("git", "-C", FR, "rev-parse", "--verify", "-q", FB+"^{commit}")
		h = sub(h)
		if rc != 0 || h == "" {
			say("착지마다 %s: 판정 못 함(%s 기본 가지를 읽지 못함) — 다음 바퀴에", name, p)
			continue
		}
		hf := onlandDir() + "/" + name + ".head"
		b, ok, err := atomic.Read(hf)
		if err != nil {
			say("착지마다 %s: 기준을 못 읽음(%v) — 다음 바퀴에", name, err)
			continue
		}
		old := strings.TrimSpace(string(b))
		if old == h {
			continue
		}
		var lanes []string
		if ok && old != "" {
			o, rc := run("git", "-C", FR, "log", "--reverse", "--format=%s", old+".."+h) // 착지 차례대로
			for _, l := range lines(o) {
				lanes = append(lanes, subjLane(l))
			}
			if rc != 0 || len(lanes) == 0 { // 기준이 기본 가지 밖(되감김 등)이면 레인을 모른다
				lanes = []string{"?"}
			}
		}
		if dry {
			if len(lanes) > 0 {
				say("%s착지마다 %s 를 돌릴 %s (%s)", P, name, strings.Join(uniq(lanes), ","), head(h, 8))
			}
			continue
		}
		bad := false
		for _, l := range uniq(lanes) {
			if err := onlandPend(name, l, "-", h); err != nil {
				say("착지마다 %s: 차례를 못 적음(%v) — 다음 바퀴에", name, err)
				bad = true
				break
			}
		}
		if bad {
			continue
		}
		if err := os.MkdirAll(onlandDir(), 0o755); err != nil {
			say("착지마다 %s: 차례 폴더를 못 만듦(%v) — 다음 바퀴에", name, err)
			continue
		}
		if err := writeAtomic(hf, h+"\n"); err != nil {
			say("착지마다 %s: 기준을 못 씀(%v) — 다음 바퀴에 같은 착지를 또 넣을 수 있다(실행기가 하나로 합친다)", name, err)
		}
		if len(lanes) > 0 {
			say("착지마다 %s ← %s 착지 %s — 명령 차례에 넣음", name, strings.Join(uniq(lanes), ","), head(h, 8))
			fire[name] = true
		}
	}
}

// subjLane: 커밋 제목 머리 — 「<레인>: …」의 레인, 없으면 첫 낱말
func subjLane(s string) string {
	if i := strings.Index(s, ":"); i > 0 && !strings.ContainsAny(s[:i], " \t") {
		return s[:i]
	}
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return "?"
}

// onlandTail: 명령 로그에서 명령이 마지막으로 찍은 줄(실행기가 적은 「--- 」·「$ 」 줄 빼고) — 사건 글에 붙인다, 없으면 ""
func onlandTail(b []byte) string {
	last := ""
	for _, l := range lines(string(b)) {
		if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "--- ") && !strings.HasPrefix(t, "$ ") {
			last = t
		}
	}
	if len([]rune(last)) > 300 {
		last = string([]rune(last)[:300]) + "…"
	}
	return last
}
