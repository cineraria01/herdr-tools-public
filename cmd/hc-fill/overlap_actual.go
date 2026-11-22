package main

// 실제 변경 겹침(go-overlap-actual, Go 판만): 지시문의 「파일:」·「쓰기:」 예약에 더해, 쓰기 소유 레인(떠 있음·제출됨)과 주차한 대기 줄이
// 실제로 바꾼 파일을 겹침 키 「A」로 견준다. 원격은 바퀴마다 프로젝트당 fetch 한 번, 떠 있는 레인의 작업 트리는 기계당 hc exec 한 번(동시).
// 못 읽은 레인은 키 A 없이 지금처럼 예약·언급만으로 판정한다

import (
	"sort"
	"strings"
	"sync"
)

// ACT: 이번 바퀴에 읽은 실제 변경 「프로젝트<TAB>레인」 → 파일(overlap_ignore 뺌). 키가 없으면 못 읽음(빈 목록은 바꾼 것 없음)
var ACT map[string][]string

// ACTPARK: 이번 바퀴의 주차한 대기 줄 「프로젝트<TAB>레인」(실제 변경을 못 읽었어도 — 막힘 문구를 「재개 미룸」으로)
var ACTPARK map[string]bool

// actRefs: 주차·제출 가지를 받아 두는 지휘 기계 로컬 이름(바퀴마다 --prune으로 원격과 맞춤)
const actNS = "refs/hc-fill/act/"

// actScript: 인자 「프로젝트 레인 작업트리 기준」 넷씩. 읽은 레인마다 「p<TAB>n<TAB>+」, 파일마다 「p<TAB>n<TAB>=파일」(못 읽은 레인은 줄 없음).
// 바꾼 파일 = 기준과의 merge-base부터 작업 트리까지(커밋·스테이지·안 올린 것) + 추적 안 하는 새 파일
const actScript = `while [ $# -ge 4 ]; do p=$1 n=$2 w=$3 b=$4; shift 4
case $w in "~/"*) w=$HOME/${w#"~/"} ;; esac
mb=$(git -C "$w" merge-base HEAD "$b" 2>/dev/null) || continue
o=$(git -C "$w" -c core.quotepath=off diff --no-renames --name-only "$mb" 2>/dev/null) || continue
u=$(git -C "$w" -c core.quotepath=off ls-files --others --exclude-standard 2>/dev/null) || continue
printf '%s\t%s\t+\n' "$p" "$n"
printf '%s\n%s\n' "$o" "$u" | while IFS= read -r f; do [ -n "$f" ] && printf '%s\t%s\t=%s\n' "$p" "$n" "$f"; done
done
`

// actParse: actScript 출력 → 「p<TAB>n」 → 파일(읽은 레인만)
func actParse(out string) map[string][]string {
	o := map[string][]string{}
	for _, l := range lines(out) {
		r := splitRow(l)
		if r.NF() != 3 {
			continue
		}
		k := r.F(1) + "\t" + r.F(2)
		switch v := r.F(3); {
		case v == "+":
			if _, ok := o[k]; !ok {
				o[k] = []string{}
			}
		case strings.HasPrefix(v, "=") && len(v) > 1:
			if _, ok := o[k]; ok {
				o[k] = append(o[k], v[1:])
			}
		}
	}
	return o
}

// actDiff <저장소> <기준> <참조>: 기준과의 merge-base부터 그 참조까지 바뀐 파일. ok=false면 못 읽음
func actDiff(repo, base, ref string) ([]string, bool) {
	out, rc := run("git", "-C", repo, "-c", "core.quotepath=off", "diff", "--no-renames", "--name-only", base+"..."+ref)
	if rc != 0 {
		return nil, false
	}
	return lines(sub(out)), true
}

// actTick <대기열 줄>: ACT를 채운다 — 쓰기 소유(OREC) 레인은 떠 있으면 그 기계 작업 트리(못 읽으면 원격 가지), 아니면 원격 제출 가지
// lane/<레인>(없으면 주차 가지), 주차한 대기 줄은 원격 주차 가지 refs/park/<레인>(없으면 옛 판 lane/<레인>). 쓰기 소유가 있는 프로젝트만
func actTick(rows []Row) {
	ACT = map[string][]string{}
	type ent struct{ p, n, m string } // m = 작업 트리를 읽을 기계(없으면 원격 가지만)
	var own []ent
	seen := map[string]bool{}
	ps := map[string]bool{}
	for _, o := range OREC {
		n, p, m, a := o.F(1), o.F(2), o.F(3), o.F(4)
		if n == "" || p == "" || seen[p+"\t"+n] {
			continue
		}
		seen[p+"\t"+n] = true
		ps[p] = true
		if !lalive(a, n) || m != LOCAL && !connHas(m) {
			m = ""
		}
		own = append(own, ent{p, n, m})
	}
	// 주차 줄은 쓰기 소유가 없는 프로젝트도 읽는다 — 같은 바퀴에 먼저 재개한 주차 줄의 키 A가 뒤 주차 줄을 막는다(감사 atow8kz-rpid2c)
	var parked []ent
	ACTPARK = map[string]bool{}
	for _, r := range rows {
		if r.F(6) == "wait" && dash(r.F(14)) == "parked" && !strings.HasPrefix(r.F(15), "audit=") && !seen[r.F(2)+"\t"+r.F(1)] {
			parked = append(parked, ent{r.F(2), r.F(1), ""})
			ACTPARK[r.F(2)+"\t"+r.F(1)] = true
			ps[r.F(2)] = true
		}
	}
	if len(own) == 0 && len(parked) == 0 {
		return
	}
	// 프로젝트마다: 기준 가지(lcFetch — 바퀴에 한 번 fetch)와 주차·제출 가지 한 번에 받기
	base, repo, refs := map[string]string{}, map[string]string{}, map[string]map[string]bool{}
	var pl []string
	for p := range ps {
		pl = append(pl, p)
	}
	sort.Strings(pl)
	for _, p := range pl {
		if lcFetch(p, true) != 0 {
			say("실제 변경 못 읽음 %s: 기준 가지 조회 실패 — 예약만으로 판정", p)
			continue
		}
		base[p], repo[p] = FB, FR
		if !strings.HasPrefix(FB, "origin/") {
			continue // 원격 없는 저장소: 주차·제출 가지 없음(작업 트리만)
		}
		if !dry {
			if _, rc := run("git", "-C", FR, "fetch", "-q", "--prune", "origin",
				"+refs/park/*:"+actNS+"park/*", "+refs/heads/lane/*:"+actNS+"lane/*"); rc != 0 {
				say("실제 변경 못 읽음 %s: 주차·제출 가지 fetch 실패 — 그 가지는 예약만으로 판정", p)
				continue
			}
		}
		out, rc := run("git", "-C", FR, "for-each-ref", "--format=%(refname)", actNS)
		if rc != 0 {
			continue
		}
		refs[p] = map[string]bool{}
		for _, l := range lines(sub(out)) {
			refs[p][l] = true
		}
	}
	// 기계마다 작업 트리 한 번(동시)
	byM := map[string][]string{}
	for _, e := range own {
		if e.m == "" || base[e.p] == "" {
			continue
		}
		r := strings.TrimRight(repoOf(e.p, e.m), "/")
		if r == "" {
			continue
		}
		byM[e.m] = append(byM[e.m], e.p, e.n, r+".wt/"+e.n, base[e.p])
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for m, args := range byM {
		wg.Add(1)
		go func(m string, args []string) {
			defer wg.Done()
			out, rc := onm(nil, m, actScript, append([]string{"-"}, args...)...)
			mu.Lock()
			defer mu.Unlock()
			if rc != 0 {
				say("실제 변경 못 읽음 @%s: 작업 트리 조회 실패 — 원격 가지나 예약만으로 판정", m)
				return
			}
			for k, v := range actParse(out) {
				ACT[k] = v
			}
		}(m, args)
	}
	wg.Wait()
	// 원격 가지: 작업 트리를 못 읽은 쓰기 소유는 제출 → 주차 순, 주차 대기 줄은 주차 → 옛 판 lane 순
	fromRefs := func(e ent, kinds ...string) {
		if _, ok := ACT[e.p+"\t"+e.n]; ok || refs[e.p] == nil {
			return
		}
		for _, k := range kinds {
			ref := actNS + k + "/" + e.n
			if !refs[e.p][ref] {
				continue
			}
			if fs, ok := actDiff(repo[e.p], base[e.p], ref); ok {
				ACT[e.p+"\t"+e.n] = fs
			}
			return
		}
	}
	for _, e := range own {
		fromRefs(e, "lane", "park")
	}
	for _, e := range parked {
		fromRefs(e, "park", "lane")
	}
	var miss []string
	for _, e := range own {
		if _, ok := ACT[e.p+"\t"+e.n]; !ok && e.m != "" && base[e.p] != "" { // 기준 가지 실패는 위에서 프로젝트로 알림
			miss = append(miss, e.n)
		}
	}
	if len(miss) > 0 {
		say("실제 변경 못 읽음 %s — 그 레인은 예약만으로 판정", strings.Join(miss, ","))
	}
	// overlap_ignore 빼기
	for k, fs := range ACT {
		p, _, _ := strings.Cut(k, "\t")
		igs := ovIgnores(p)
		var keep []string
		for _, f := range fs {
			if f != "" && !ovIgnored(igs, f) {
				keep = append(keep, f)
			}
		}
		sort.Strings(keep)
		ACT[k] = keep
	}
}

// actKeys <프로젝트> <레인>: 그 레인 실제 변경 키 「A<TAB>파일」(못 읽었으면 없음)
func actKeys(p, n string) []Row {
	var o []Row
	for _, f := range ACT[p+"\t"+n] {
		o = append(o, Row{"A", f})
	}
	return o
}

// candKeys: 후보 레인의 겹침 키 — 지시문 키(lkeys) + 실제 변경(주차한 줄이면)
func candKeys(p, n, brief string) []Row { return append(lkeys(p, brief), actKeys(p, n)...) }
