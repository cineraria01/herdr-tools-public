package main

// 작업별 임시 폴더 지우기(ht-job-tmpdir, docs/hc-sweep/job-tmp.md): hc-lane·병합기가 띄운 작업은 <뿌리>/<6글자>를 TMPDIR로 받고 안에 표식
// .hc-job(주소·이름·유닉스 초 세 줄)을 둔다. hc-sweep은 (1) 닫은 터미널(닫힘)의 폴더와 (2) 주인이 이번 hc ls에 살아 있지 않고
// 유예가 지난 폴더를 통째로 지운다. 이름 맞추기·lsof는 쓰지 않는다. 모름(목록을 못 믿음·표식 못 읽음·맨 위가 막 바뀜)은 지우지 않는다

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/jobtmp"
)

// jobRootDir: 이번 바퀴의 작업 임시 폴더 뿌리(끔·Windows·설정 못 읽음이면 "") — sweepMain이 hc ls 뒤에 정한다
var jobRootDir string

// jobRoot: 설정 jobtmp.root를 펼친 뿌리
func jobRoot() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	v, ok := conf(jobtmp.Key, jobtmp.Default)
	if !ok {
		return ""
	}
	return jobtmp.Expand(v, home)
}

// jobGrace: 유예(설정 jobtmp.grace_min, 기본 10분). 숫자가 아니거나 음수면 ok false(그 바퀴 고아 지우기를 안 함)
func jobGrace() (time.Duration, bool) {
	v, ok := conf("jobtmp.grace_min", "10")
	f, err := strconv.ParseFloat(v, 64)
	if !ok || err != nil || f < 0 {
		fmt.Printf("작업 임시 폴더: jobtmp.grace_min 값을 못 씀(%q) — 고아 지우기 건너뜀\n", v)
		return 0, false
	}
	return time.Duration(f * float64(time.Minute)), true
}

// liveOwners: hc ls 목록에서 이 기계의 살아 있는 터미널 「주소<탭>이름」. 안전띠 ①: 목록이 이 기계를 다 담았다고 볼 수 없으면 ok false —
// 오류 칸(error·errors)이나 부분 표시(partial·truncated)가 있음, terminals가 목록이 아님, 주소·state가 글이 아닌 줄이 있음,
// 이 기계 줄(주소에 「/」 없음)이 하나도 없음(지킴이로 도는 hc-sweep은 제 터미널이 늘 이 기계 줄에 있다 — 통째로 빠진 목록을 가른다)
func liveOwners(ls map[string]any) (map[string]bool, string) {
	for _, k := range []string{"error", "errors", "partial", "truncated"} {
		if truthy(ls[k]) {
			return nil, "목록에 " + k + " 칸"
		}
	}
	list, ok := ls["terminals"].([]any)
	if !ok {
		return nil, "terminals 목록 없음"
	}
	live, local := map[string]bool{}, 0
	for _, x := range list {
		t, ok := x.(map[string]any)
		a, aok := t["address"].(string)
		s, sok := t["state"].(string)
		if !ok || !aok || !sok || a == "" {
			return nil, "주소·state를 못 읽는 줄"
		}
		if strings.Contains(a, "/") {
			continue
		}
		local++
		if s != "finished" {
			live[a+"\t"+str(t, "name")] = true
		}
	}
	if local == 0 {
		return nil, "이 기계 터미널이 하나도 없음"
	}
	return live, ""
}

// touchedSince: 안전띠 ②: 폴더 자신이나 맨 위 항목 가운데 cutoff 뒤에 바뀐 것이 있나(안쪽 전체는 훑지 않는다 — 싸게). 못 읽으면 true
func touchedSince(dir string, cutoff time.Time) bool {
	st, err := os.Lstat(dir)
	if err != nil || st.ModTime().After(cutoff) {
		return true
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return true
	}
	for _, e := range ents {
		if fi, err := os.Lstat(filepath.Join(dir, e.Name())); err != nil || fi.ModTime().After(cutoff) {
			return true
		}
	}
	return false
}

// jobSweep: 뿌리 아래 작업 폴더 가운데 gone(주소, 이름, 표식 초)이 참이고 맨 위가 grace 안에 안 바뀐 것을 지운다. 지운(dry면 지울) 개수
func jobSweep(root string, gone func(addr, name string, at int64) bool, grace time.Duration, dry bool, why string) int {
	ents, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	cutoff := time.Now().Add(-grace)
	n := 0
	for _, e := range ents {
		p := filepath.Join(root, e.Name())
		st, err := os.Lstat(p)
		if err != nil || !st.IsDir() || !mine(st) || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		addr, name, at, ok := jobtmp.Owner(p)
		switch {
		case !ok:
			if dry {
				fmt.Println("작업 임시 남김(표식 못 읽음):", p)
			}
			continue
		case !gone(addr, name, at):
			continue
		case touchedSince(p, cutoff):
			fmt.Printf("작업 임시 미룸(맨 위가 %s 안에 바뀜): %s %s %s\n", grace, p, addr, name)
			continue
		}
		if dry {
			fmt.Printf("지울 작업 임시(%s): %s %s %s\n", why, p, addr, name)
		} else if err := removeTree(p); err != nil {
			fmt.Printf("작업 임시 못 지움: %s %v\n", p, err)
			continue
		} else {
			fmt.Printf("지운 작업 임시(%s): %s %s %s\n", why, p, addr, name)
			record(addr, name, "작업 임시 폴더 지움("+why+")", p, "-", "-")
		}
		n++
	}
	return n
}

// jobDrop: 닫은 터미널(주소·이름)의 작업 폴더를 지운다(닫는 길). 맨 위가 유예 안에 바뀐 것은 고아 바퀴로 미룬다
func jobDrop(addr, name string) {
	if jobRootDir == "" {
		return
	}
	grace, ok := jobGrace()
	if !ok {
		return
	}
	jobSweep(jobRootDir, func(a, n string, _ int64) bool { return a == addr && n == name }, grace, false, "터미널 닫음")
}

// osTmp: 사용자 임시 폴더 맨 위의 운영체제 것(실제 폴더를 보고 — 2026-11-27 워커 실측). 한 번 치우기가 건드리지 않는다
var osTmp = []string{"com.apple.", "TemporaryItems", "Cleanup At Startup", "xcrun_db", ".AddressBookLocks", ".LINKS", ".screen"}

func osOwned(name string) bool {
	for _, p := range osTmp {
		if name == p || strings.HasSuffix(p, ".") && strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// tmpKind: 이름 꼴 — dry-run이 보이고 --only가 글자 그대로 고르는 종류. Test*, 그 밖은 끝 숫자·_를 떼고 마지막 . - _ 뒤 마디를 *로
// (같은 머리 — com.google.Chrome.·lane-dev-app- — 는 꼬리가 소문자뿐이어도 한 줄). 숫자가 든 마디 뒤의 4글자 이하 소문자 확장자
// (uv-<꼬리>.lock)는 떼었다 붙인다. 이름 하나로만 정해진다(함께 있는 것과 상관없음 — dry-run과 지우기가 같은 종류를 본다)
func tmpKind(n string) string {
	if strings.HasPrefix(n, "Test") {
		return "Test*"
	}
	stem, ext := n, ""
	if i := strings.LastIndex(n, "."); i > 0 && len(n)-i-1 <= 4 && len(n)-i-1 > 0 && strings.Trim(n[i+1:], "abcdefghijklmnopqrstuvwxyz") == "" {
		seg := n[:i]
		seg = seg[strings.LastIndexAny(seg, ".-_")+1:]
		if strings.ContainsAny(seg, "0123456789") {
			stem, ext = n[:i], n[i:]
		}
	}
	s := strings.TrimRight(stem, "0123456789_")
	if s == "" {
		s = stem
	}
	if i := strings.LastIndexAny(s, ".-_"); i > 0 {
		return s[:i+1] + "*" + ext
	}
	if s != stem {
		return s + "*" + ext
	}
	return n
}

// unpicked: --only에 붙일 줄에서 뺄 종류 — 꼴이 아닌 이름 하나(데몬 폴더·kw 같은 정체 모름), 한 개뿐이고 0.0MB, 붙여 넣기를 깨는 글자
func unpicked(kind string, n int, size int64) bool {
	return !strings.Contains(kind, "*") || n == 1 && size < 50000 || strings.ContainsAny(kind, ",'")
}

// onlySet: --only 글(쉼표로 나눔, 앞뒤 빈칸 뺌). 고른 것이 없으면 nil
func onlySet(v string) map[string]bool {
	var m map[string]bool
	for _, k := range strings.Split(v, ",") {
		if k = strings.TrimSpace(k); k != "" {
			if m == nil {
				m = map[string]bool{}
			}
			m[k] = true
		}
	}
	return m
}

// backlogRun: 이미 쌓인 임시 찌꺼기 한 번 치우기(--tmp-backlog <시간>). 사용자 임시 폴더 맨 위 내 항목 가운데 안쪽 모두가 hours시간 넘게
// 안 바뀌었고 lsof로 아무도 열지 않은 것 — 운영체제 것은 뺀다. only가 있으면 그 종류(tmpKind 글자 그대로)만 — 지우기는 only가 꼭 있다(sweepMain).
// dry면 종류별 개수·크기(큰 차례)와 --only에 붙일 한 줄. lsof를 못 읽으면 하나도 안 지운다(종료 1)
func backlogRun(hours float64, dry bool, only map[string]bool, r runner) int {
	base := tmpDir(r)
	ents, err := os.ReadDir(base)
	if err != nil {
		fmt.Println("한 번 치우기: 임시 폴더를 못 읽음:", base, err)
		return 1
	}
	base = realpath(base)
	cutoff := time.Now().Add(-time.Duration(hours * float64(time.Hour)))
	type cand struct {
		p, kind string
		size    int64
	}
	var cands []cand
	osN, newN, skipN := 0, 0, 0
	for _, e := range ents {
		p := filepath.Join(base, e.Name())
		st, err := os.Lstat(p)
		switch {
		case err != nil || !mine(st):
			continue
		case osOwned(e.Name()):
			osN++
			continue
		case only != nil && !only[tmpKind(e.Name())]:
			skipN++
			continue
		}
		if size, aged, err := scan(p, cutoff); err == nil && aged {
			cands = append(cands, cand{p, tmpKind(e.Name()), size})
		} else {
			newN++
		}
	}
	held := map[string]bool{}
	if len(cands) > 0 {
		if held = openPaths(r, lsofLimit(conf("sweep.lsof_secs", "60"))); held == nil {
			fmt.Println("한 번 치우기: lsof 실패 — 하나도 안 지움")
			return 1
		}
	}
	type sum struct {
		n    int
		size int64
	}
	// 지운 이름 낱낱이(2026-11-27 — 합계만 남아 무엇을 지웠는지 나중에 못 봤다): 실행마다 한 파일, 못 열면 하나도 안 지운다
	logDir := filepath.Join(stateHome(), "hc-sweep", "tmp-backlog")
	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%d.log", time.Now().Format("20060102-150405"), os.Getpid()))
	var lf *os.File
	if dry {
		fmt.Printf("지우면 지운 이름을 낱낱이 %s/<시각>-<pid>.log에 남긴다\n", logDir)
	} else if len(cands) > 0 {
		if err = os.MkdirAll(logDir, 0o755); err == nil {
			lf, err = os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		}
		if err != nil {
			fmt.Println("한 번 치우기: 지운 이름 기록 파일을 못 엶 — 하나도 안 지움:", err)
			return 1
		}
		defer lf.Close()
	}
	kinds, busy, n, total := map[string]*sum{}, 0, 0, int64(0)
	for _, c := range cands {
		in := false
		for h := range held {
			if under(h, c.p) {
				in = true
				break
			}
		}
		if in {
			busy++
			continue
		}
		if !dry {
			if err := removeTree(c.p); err != nil {
				fmt.Printf("못 지움: %s %v\n", c.p, err)
				continue
			}
			if _, err := fmt.Fprintf(lf, "%s\t%s\t%d\n", time.Now().Format("2006-01-02T15:04:05-0700"), flat(c.p), c.size); err != nil {
				fmt.Printf("지운 이름 기록을 못 씀 — 여기서 멈춤(방금 지운 것: %s): %v\n", c.p, err)
				return 1
			}
		}
		if kinds[c.kind] == nil {
			kinds[c.kind] = &sum{}
		}
		kinds[c.kind].n++
		kinds[c.kind].size += c.size
		n, total = n+1, total+c.size
	}
	var ks []string
	for k := range kinds {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		return kinds[ks[i]].size > kinds[ks[j]].size || kinds[ks[i]].size == kinds[ks[j]].size && ks[i] < ks[j]
	})
	word := "지움"
	if dry {
		word = "지울 것"
	}
	skipped := ""
	if only != nil {
		skipped = fmt.Sprintf(" · 고르지 않은 것 %d개 남김", skipN)
	}
	fmt.Printf("한 번 치우기(%s, %s시간 넘음): %s %d개 %s · 열린 것 %d개·새 것 %d개 남김%s · 운영체제 것 %d개 뺌\n", base, pyFloat(hours), word, n, human(total), busy, newN, skipped, osN)
	var pick, rest []string
	for _, k := range ks {
		fmt.Printf("  %s\t%d개\t%s\n", k, kinds[k].n, human(kinds[k].size))
		if unpicked(k, kinds[k].n, kinds[k].size) {
			rest = append(rest, k)
		} else {
			pick = append(pick, k)
		}
	}
	if dry && len(pick) > 0 { // 따옴표 — 종류의 *를 셸이 펼치지 않게
		fmt.Printf("고르기(hc-sweep --tmp-backlog %s 뒤에 붙임):\n--only '%s'\n", pyFloat(hours), strings.Join(pick, ","))
	}
	if dry && len(rest) > 0 {
		fmt.Printf("그 밖(고르지 않음): %s\n", strings.Join(rest, ","))
	}
	if lf != nil {
		fmt.Fprintf(lf, "합계\t%d개\t%d\n", n, total)
		fmt.Println("지운 이름:", logPath)
	}
	if n > 0 && !dry {
		record("임시:"+base, "-", "임시 찌꺼기 한 번 치우기("+pyFloat(hours)+"시간 넘음)", base, "-", fmt.Sprintf("%d개 %s", n, human(total)))
	}
	return 0
}

func human(b int64) string {
	if b >= 1e9 {
		return fmt.Sprintf("%.1fGB", float64(b)/gb)
	}
	return fmt.Sprintf("%.1fMB", float64(b)/1e6)
}

// jobOrphans: 주인 터미널(주소+이름)이 이번 hc ls에 살아 있지 않고 표식이 유예보다 오래된 폴더를 지운다(놓친 길 — 충돌·KILL·재부팅·다른 도구가 닫음)
func jobOrphans(ls map[string]any, dry bool) {
	if jobRootDir == "" || !lexists(jobRootDir) {
		return
	}
	live, why := liveOwners(ls)
	if why != "" {
		fmt.Printf("작업 임시 폴더: hc ls 목록을 믿을 수 없음(%s) — 지우지 않음\n", why)
		return
	}
	grace, ok := jobGrace()
	if !ok {
		return
	}
	now := time.Now().Unix()
	n := jobSweep(jobRootDir, func(a, name string, at int64) bool {
		return !live[a+"\t"+name] && now-at >= int64(grace/time.Second)
	}, grace, dry, "주인 터미널 없음")
	if n > 0 {
		fmt.Printf("작업 임시 폴더: 주인 없는 것 %d개 %s\n", n, map[bool]string{true: "지울 것", false: "지움"}[dry])
	}
}
