// reporoute.go: 주인 없는 레인 사건을 그 저장소 지휘에게 — watch의 wfilter가 부르는 repoSend와 그 도우미(watch-project-route, 2026-11-24)
package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/tri"
)

// repoSend: 주인 없는·주인이 사라진 사건을 이 watch가 보내나. 사건의 저장소를 모름·이 지휘가 그 저장소를 다룸·이 지휘의 저장소를 모름
// (모든 저장소를 다룬다)이면 보낸다. 아니면 그 저장소 지휘(다른 지휘 가운데 그 저장소를 다루거나 저장소를 모르는 지휘)에게 미루되
// 「그 지휘가 보낼 수 있을 것」을 추론하지 않는다(지휘 지시 — watch 없음·커서 바닥 뒤 같은 구멍이 감사 1·2차에 나왔다): 그 지휘의
// seen.<지휘>에 이 사건이 있으면 받은 것으로 치고 커서에 적는다(다시 안 봄), 없으면 이 watch가 처음 미룬 때부터 유예
// (lane.watch_repo_grace, e.rgrace) 동안만 미루고 그 뒤에는 보낸다. 그 지휘 커서를 못 읽으면 보낸다.
// 그래서 「아무도 안 받음」은 없고 최악은 유예 뒤 겹침이다. 확인된 끝남(coordCheck 없음)인 지휘에게는 미루지 않는다(빠른 길 — 받았는지는 본다)
func (e *ev) repoSend(l string) bool {
	r := e.evrepo(l)
	if r == "" {
		return true
	}
	if rs, ok := coordRepos(e.wto); !ok || rs[r] {
		return true
	}
	cfs, _ := filepath.Glob(e.ed + "/coord.*")
	k := lineKey(l)
	wait, bad := false, false
	for _, cf := range cfs {
		ck := strings.TrimPrefix(filepath.Base(cf), "coord.")
		if strings.Contains(ck, ".") || ck == ckey(e.wto) {
			continue // 쓰는 중인 coord.<주소>.<pid>, 이 watch 자신
		}
		c, ok := readCoord(cf)
		if !ok {
			continue
		}
		if rs, known := coordRepos(c[0]); known && !rs[r] {
			continue
		}
		s, sr := e.seenOf(c[0])
		if s[k] {
			delete(e.rdefer, k)
			e.appendCur(k)
			return false
		}
		if sr == tri.Unknown {
			bad = true
			continue
		}
		cr, ok := e.occ[lastSeg(c[0])]
		if !ok {
			_, _, cr = coordCheck(c[0])
			e.occ[lastSeg(c[0])] = cr
		}
		if cr != tri.No {
			wait = true
		}
	}
	if bad || !wait {
		delete(e.rdefer, k)
		return true
	}
	// 유예는 이 watch가 이 열쇠를 처음 미룬 때부터(사건 1칸 시각이 아니라 — 멈춤은 바퀴마다 「지금」으로 지어 유예가 안 끝났고, 다른 기계
	// 시계가 앞서도 같았다: 감사 3차 높음 1). watch가 다시 뜨면 표가 비어 유예가 한 번 더 돈다(늦을 뿐)
	t0, ok := e.rdefer[k]
	if !ok {
		t0 = time.Now()
		e.rdefer[k] = t0
	}
	if time.Since(t0) < e.rgrace {
		return false
	}
	delete(e.rdefer, k)
	return true
}

// pruneDefer: 미룬 표에서 유예보다 10분 넘게 묵은 열쇠(창을 벗어나 다시 안 오는 것)를 걷는다 — 미룬 열쇠는 창에 있는 동안 바퀴마다
// 다시 와 유예가 지나면 보내져 지워지니, 그보다 묵은 것은 창 밖이다
func (e *ev) pruneDefer() {
	if e.rdefer == nil {
		e.rdefer = map[string]time.Time{}
	}
	for k, t := range e.rdefer {
		if time.Since(t) > e.rgrace+10*time.Minute {
			delete(e.rdefer, k)
		}
	}
}

// seenOf: 그 지휘 커서 seen.<지휘>의 사건 열쇠 — 바퀴마다 지휘 하나에 한 번 읽는다(e.seenc, wfilter가 비움). 있음 읽음, 없음 파일 없음(빈 모음), 모름 못 읽음
func (e *ev) seenOf(addr string) (map[string]bool, tri.State) {
	if s, ok := e.seenc[addr]; ok {
		return s.k, s.rc
	}
	s := seenCache{k: map[string]bool{}}
	ls, err := readLines(e.ed + "/seen." + ckey(addr))
	switch {
	case err == nil:
		s.rc = tri.Yes
	case os.IsNotExist(err):
		s.rc = tri.No
	default:
		s.rc = tri.Unknown
	}
	for _, x := range ls {
		if !strings.HasPrefix(x, "#since"+tab) {
			s.k[nk(x)] = true
		}
	}
	e.seenc[addr] = s
	return s.k, s.rc
}

type seenCache struct {
	k  map[string]bool
	rc tri.State
}

// evrepo: 사건 레인의 저장소 — 13칸, 없으면 이 바퀴에 읽은 사건에서 그 레인 이름의 13칸(병합기·hc-fill·멈춤처럼 --name으로 적은 줄).
// 레인 이름에 저장소가 둘 이상 걸리거나 모르면 ""
func (e *ev) evrepo(l string) string {
	x := events.Cols(l)
	if repoIDok(x.Repo) {
		return x.Repo
	}
	return e.lrepo[x.Lane]
}

// laneRepos: 사건들에서 레인 이름 → 13칸 저장소(둘 이상이면 "" — 모름)
func laneRepos(ls []string) map[string]string {
	m := map[string]string{}
	for _, l := range ls {
		x := events.Cols(l)
		ln, r := x.Lane, x.Repo
		if ln == "" || ln == "-" || !repoIDok(r) {
			continue
		}
		if p, ok := m[ln]; ok && p != r {
			r = ""
		}
		m[ln] = r
	}
	return m
}
