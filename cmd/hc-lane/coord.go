// coord.go: 지휘 정보(<evdir>/coord.<주소> 「주소<탭>pid<탭>이름<탭>마지막으로 레인을 띄운 시각」)와 hc-lane coord
// (셸 판 lib/hc-lane/watch.sh tinfo·coord_is·coord_set·coord_check·coord_default, bin/hc-lane coord 분기)
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/tri"
)

func init() { register("coord", coordCmd, noHelp) }

// hcGet: hc get <addr> --json 한 번(sec초 상한)을 hcjson.ParseGet으로 판정 — 표준 출력은 터미널 객체, 종료 3의 오류 객체는 표준 오류에 온다.
// (t, true, nil) 있음 · (_, false, nil) 확인된 없음(완전한 not_found) · 오류는 모름(연결 실패·시간 넘김·잘림·찌꺼기·모르는 state·주소 다름)
func hcGet(sec int, addr string) (hcjson.Terminal, bool, error) {
	out, rc := hcGetRaw(sec, addr)
	return hcjson.ParseGet([]byte(out), rc, addr)
}

// hcGetRaw: hcGet이 판정하는 글과 종료 코드 — 0 아닌 종료면 표준 오류(오류 객체), 비었으면 표준 출력
func hcGetRaw(sec int, addr string) (string, int) {
	var eb strings.Builder
	out, rc := captErr(sec, &eb, "hc", "get", addr, "--json")
	return hcjson.GetOut(out, eb.String(), rc), rc
}

// exitOf: 세 값 판정을 바깥 종료 코드로(명령 계약 — 종료 0 있음·맞음, 1 없음·다름, 종료 2(모름))
func exitOf(s tri.State) int {
	switch s {
	case tri.Yes:
		return 0
	case tri.No:
		return 1
	default:
		return 2
	}
}

// getState: hc get 판정. 있음 살아 있음(state live), 없음 완전한 not_found·확인된 finished, 모름 hcGet 오류(gofix L04, 재감사 R-13)
func getState(t hcjson.Terminal, found bool, err error) tri.State {
	switch {
	case err != nil:
		return tri.Unknown
	case !found || t.State != "live":
		return tri.No
	}
	return tri.Yes
}

// tinfo: 그 터미널의 맨 위 pid·이름(없으면 -)·레인/감시 라벨. 있음 살아 있음, 없음 확인된 없음·끝남, 모름 — getState의 모름에 더해
// live인데 신원 칸(pid·name)이 응답에 없거나 pid가 null·0이면 모름(실행 신원을 확인 못 한 것은 「없음」이 아니다 — 재감사2 lane/R-208, fill/R-206)
func tinfo(addr string) (pid, name string, laneLabel bool, st tri.State) {
	return tinfoOf(hcGet(10, addr))
}

// tinfoOf: tinfo의 판정을 이미 받은 hc get 결과로
func tinfoOf(t hcjson.Terminal, found bool, err error) (pid, name string, laneLabel bool, st tri.State) {
	if r := getState(t, found, err); r != tri.Yes {
		return "", "", false, r
	}
	_, hasPid := t.Raw["pid"]
	_, hasName := t.Raw["name"]
	if !hasPid || !hasName || t.PID <= 0 {
		return "", "", false, tri.Unknown
	}
	name = t.Name
	if name == "" {
		name = "-"
	}
	laneLabel = t.Labels["hc-lane"] == "1" || t.Labels["hc-lane-watch"] == "1"
	return strconv.Itoa(t.PID), name, laneLabel, tri.Yes
}

// agentState: 받는 터미널에 에이전트가 있나 — 판정은 hcjson.AgentState 한 곳(herdr-hooks의 hc send·keys 막기와 같은 판정)
var agentState = hcjson.AgentState

func coordFile(addr string) string { return evdir() + "/coord." + ckey(addr) }

// readCoord: 「주소 pid 이름 시각」(셸 판 IFS=탭 read — 빈 칸은 겹쳐 하나)
func readCoord(path string) ([]string, bool) {
	c, r := coordRead(path)
	return c, r == tri.Yes
}

// coordRead: readCoord와 같되 있음, 없음(ENOENT)·꼴이 아님(줄바꿈 없음 — 쓰는 중), 모름(읽기 실패 — 권한·폴더·입출력)
func coordRead(path string) ([]string, tri.State) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, tri.No
	} else if err != nil {
		return nil, tri.Unknown
	}
	l, _, nl := strings.Cut(string(b), "\n")
	if !nl {
		return nil, tri.No
	}
	f := strings.FieldsFunc(l, func(r rune) bool { return r == '\t' })
	if len(f) > 4 {
		f = append(f[:3], strings.Join(f[3:], tab))
	}
	for len(f) < 4 {
		f = append(f, "")
	}
	return f, tri.Yes
}

// coordSet: 살아 있고 레인·감시 터미널이 아니면 지휘 정보로 적는다. launch면 띄운 시각을 지금으로, 아니면 같은 pid의 옛 시각 그대로
func coordSet(addr string, launch bool) bool {
	t, found, err := hcGet(10, addr)
	pid, name, ll, rc := tinfoOf(t, found, err)
	if rc != tri.Yes || ll {
		return false
	}
	f := coordFile(addr)
	ts := "0"
	if c, ok := readCoord(f); ok && c[1] == pid && digits(c[3]) {
		ts = c[3]
	}
	if launch {
		ts = strconv.FormatInt(time.Now().Unix(), 10)
	}
	if os.MkdirAll(evdir(), 0o777) != nil {
		return false
	}
	// 사용자 전용 장부 — 구판이 0666으로 만든 기존 파일도 갈 때 0600으로 좁힌다(재감사2 lane/R-221·merger/R-206)
	if atomic.WriteFileMode(f, []byte(addr+tab+pid+tab+flat(name)+tab+ts+"\n"), 0o600) != nil {
		return false
	}
	coordCwdRepo(addr, t)
	return true
}

// coordRepoFile: 지휘가 다루는 저장소 기록 <evdir>/coordrepo.<주소> 「pid<탭>저장소」(watch-project-route) — coord.<주소> 꼴은 그대로 두려고
// 옆 파일(옛 판 readCoord는 다섯째 칸을 시각에 붙여 읽는다). pid가 지금 지휘 정보와 같은 줄만 지금 지휘의 것
func coordRepoFile(addr string) string { return evdir() + "/coordrepo." + ckey(addr) }

// coordRepoAdd: 그 지휘(지휘 정보가 이 기계에 있을 때만)가 그 저장소를 다룬다고 적는다. 이미 있으면 그대로
func coordRepoAdd(addr, rid string) {
	c, ok := readCoord(coordFile(addr))
	if !ok || !repoIDok(rid) || !digits(c[1]) {
		return
	}
	l := c[1] + tab + rid
	f := coordRepoFile(addr)
	if ls, _ := readLines(f); has(ls, l) {
		return
	}
	if err := appendLines(f, l); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane: 경고 — 지휘 저장소 기록을 쓰지 못했다(그 지휘는 모든 저장소 사건을 받는다): %s\n", f)
	}
}

// coordCwdRepo: 지휘 터미널이 서 있는 폴더(띄운 폴더 cwd·전경 폴더 foreground.cwd)의 저장소도 그 지휘가 다룬다고 적는다(coord-cwd-repo —
// 저장소 A 폴더에서 B에만 레인을 넣은 지휘의 기록에 A가 없어 A 받은편지 레인 사건이 모든 지휘에게 갔다). coordSet·coordCheck(watch 바퀴마다
// wcheck)가 부르니 옛 판이 적은 지휘도 다음 바퀴에 채워진다. 넓히기만: 기록이 없는 지휘(모름 = 모든 저장소)는 그대로, 저장소 아님·원격
// 없음·읽기 실패는 안 넣음, 다른 기계 터미널(주소에 /)은 그 폴더가 이 기계 것이 아니라 안 봄.
// ponytail: 첫 레인을 띄운 순간(기록 모름 → 그 저장소)부터 다음 바퀴까지는 작업 폴더 저장소가 빠진다 — 그사이 그 사건은 겹칠 수는 있어도 안 빠진다
func coordCwdRepo(addr string, t hcjson.Terminal) {
	if strings.Contains(addr, "/") {
		return
	}
	if _, known := coordRepos(addr); !known {
		return
	}
	for _, d := range []string{t.Cwd, t.NowCwd} {
		if d == "" {
			continue
		}
		if u, rc := capt(10, errDrop, "git", "-C", d, "config", "--get", "remote.origin.url"); rc == 0 {
			coordRepoAdd(addr, repoID(u))
		}
	}
}

// coordRepos: 그 지휘가 다루는 저장소(지금 지휘 정보의 pid 줄만). known 거짓 = 모름(정보 없음·옛 판·읽기 실패) — 모든 저장소를 다룬다로 본다
func coordRepos(addr string) (map[string]bool, bool) {
	c, ok := readCoord(coordFile(addr))
	if !ok {
		return nil, false
	}
	ls, err := readLines(coordRepoFile(addr))
	if err != nil {
		return nil, false
	}
	rs := map[string]bool{}
	for _, l := range ls {
		if p, r, _ := strings.Cut(l, tab); p == c[1] && repoIDok(r) {
			rs[r] = true
		}
	}
	return rs, len(rs) > 0
}

// seedSeen: 새로 적은 지휘(hc-fill add → coord --set)의 커서 seen.<지휘>가 없을 때만 「#since 지금」으로 만든다 — 뒤에 뜨는 그 지휘
// watch가 등록 전 남의 사건(새 커서 기본 2시간 치)을 보내지 않게. 이미 있는 커서는 그대로(O_EXCL)
func seedSeen(addr string) error {
	if err := os.MkdirAll(evdir(), 0o777); err != nil {
		return err
	}
	f, err := os.OpenFile(evdir()+"/seen."+ckey(addr), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := f.WriteString("#since" + tab + utcNow() + "\n"); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// coordCheck: 그 지휘의 정보가 있고 지금 hc get pid·이름이 같으면 있음(주소 addr). 없음 다름·없음(까닭 why), 모름
func coordCheck(addr string) (cad, why string, st tri.State) {
	f := coordFile(addr)
	c, cr := coordRead(f)
	switch cr {
	case tri.Unknown:
		return "", fmt.Sprintf("지휘 정보를 읽지 못했다(%s)", f), tri.Unknown
	case tri.No:
		return "", fmt.Sprintf("지휘 정보 없음(%s — 지휘가 레인을 띄우거나 wait를 걸면 생김)", f), tri.No
	case tri.Yes:
	}
	cad = c[0]
	t, found, err := hcGet(10, cad)
	pid, name, _, r := tinfoOf(t, found, err)
	switch r {
	case tri.Unknown:
		return cad, fmt.Sprintf("hc get %s 로 지휘의 신원(state·pid·이름)을 확인 못 함", cad), tri.Unknown
	case tri.No:
		return cad, cad + " 터미널이 없거나 끝남", tri.No
	case tri.Yes:
	}
	if pid != c[1] || flat(name) != c[2] {
		return cad, fmt.Sprintf("%s 의 pid·이름(%s %s)이 지휘 정보(%s %s)와 다름", cad, pid, name, c[1], c[2]), tri.No
	}
	// pid·이름이 같아도 대화형 셸에서 띄운 지휘는 에이전트가 끝나면 셸만 남는다(watch-agent-guard) — 받는 쪽에 에이전트가 있어야 지휘
	switch a, why := agentState(t); a {
	case tri.No:
		return cad, cad + " 의 " + why, tri.No
	case tri.Unknown:
		return cad, fmt.Sprintf("%s 에이전트 확인 못 함 — %s", cad, why), tri.Unknown
	case tri.Yes:
	}
	coordCwdRepo(cad, t)
	return cad, "", tri.Yes
}

// coordDefault: 살아 있고 맞는 지휘 가운데 가장 최근에 레인을 띄운 곳. 없음, 모름(그보다 최근 지휘를 확인 못 함)
func coordDefault() (string, string, tri.State) {
	cfs, _ := filepath.Glob(evdir() + "/coord.*")
	sort.Strings(cfs)
	var bt, ut int64 = -1, -1
	ba := ""
	for _, cf := range cfs {
		if strings.Count(filepath.Base(cf), ".") >= 2 {
			continue // 쓰는 중인 coord.<주소>.<pid>
		}
		c, cr := coordRead(cf)
		switch cr {
		case tri.Unknown: // 읽지 못한 후보는 「없음」이 아니다 — 시각을 모르니 어느 확인된 지휘보다 최근일 수 있다
			ut = 1<<62 - 1
			continue
		case tri.No:
			continue
		case tri.Yes:
		}
		ct := int64(0)
		if digits(c[3]) {
			ct = atoi(c[3])
		}
		_, _, r := coordCheck(c[0])
		if r == tri.Yes && ct > bt {
			bt, ba = ct, c[0]
		} else if r == tri.Unknown && ct > ut {
			ut = ct
		}
	}
	if ba == "" {
		if ut >= 0 { // 확인된 지휘는 없지만 조회 못 한 후보가 있다 — 「없음」이 아니다(R-13)
			return "", "확인 못 한 지휘 정보가 있다(hc get 실패·잘린 응답·읽기 실패)", tri.Unknown
		}
		return "", "살아 있고 pid·이름이 맞는 지휘가 없다(지휘가 레인을 띄우거나 wait를 걸면 생김)", tri.No
	}
	if ut > bt {
		return "", "더 최근 지휘를 확인 못 했다(hc get 실패·읽기 실패)", tri.Unknown
	}
	return ba, "", tri.Yes
}

func coordCmd(a []string) int {
	if len(a) > 0 && a[0] == "--set" {
		sa := os.Getenv("HC_TERMINAL_ID")
		if len(a) > 1 && a[1] != "" {
			sa = a[1]
		}
		if sa == "" {
			fmt.Fprintln(os.Stderr, "hc-lane coord --set: 주소가 없다(지휘 터미널에서 부르거나 주소를 준다)")
			return 2
		}
		if coordSet(sa, true) {
			if err := seedSeen(sa); err != nil {
				fmt.Fprintf(os.Stderr, "hc-lane coord --set: 경고 — 새 지휘 커서를 쓰지 못했다(watch가 2시간 전부터 본다): %v\n", err)
			}
			fmt.Println(sa)
			return 0
		}
		fmt.Fprintf(os.Stderr, "hc-lane coord --set: %s 는 살아 있는 지휘 터미널이 아니다(레인·감시 터미널이거나 없음)\n", sa)
		return 1
	}
	if len(a) > 0 && a[0] == "--check" {
		if len(a) < 2 || a[1] == "" {
			fmt.Fprintln(os.Stderr, "hc-lane coord --check: 주소를 준다")
			return 2
		}
		_, why, st := coordCheck(a[1])
		if st != tri.Yes {
			fmt.Fprintf(os.Stderr, "hc-lane coord --check: %s\n", why)
		}
		return exitOf(st)
	}
	if len(a) > 0 && a[0] == "--watch" {
		if len(a) < 2 || a[1] == "" || strings.HasPrefix(a[1], "-") || !onlyChars(a[1], alnum+"_./-") {
			fmt.Fprintln(os.Stderr, "hc-lane coord --watch: 지휘 주소를 준다")
			return 2
		}
		startSetup()
		return watchFor(a[1])
	}
	cad, why, st := coordDefault()
	if st == tri.Yes {
		fmt.Println(cad)
		return 0
	}
	if st == tri.Unknown {
		fmt.Fprintf(os.Stderr, "hc-lane coord: 지휘 모름 — %s\n", why)
		return 2
	}
	fmt.Fprintf(os.Stderr, "hc-lane coord: 지휘 없음 — %s\n", why)
	return 1
}
