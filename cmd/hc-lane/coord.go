// coord.go: 지휘 정보(<evdir>/coord.<주소> 「주소<탭>pid<탭>이름<탭>마지막으로 레인을 띄운 시각」)와 hc-lane coord
// (셸 판 lib/hc-lane/watch.sh tinfo·coord_is·coord_set·coord_check·coord_default, bin/hc-lane coord 분기)
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
)

func init() { register("coord", coordCmd, noHelp) }

var (
	reGetKnown = regexp.MustCompile(`"kind" *: *"not_found"|"address" *:`)
	reNotFound = regexp.MustCompile(`"kind" *: *"not_found"`)
)

// tinfo: 그 터미널의 맨 위 pid·이름(없으면 -)·레인/감시 라벨. rc 0 살아 있음, 1 없음·끝남·pid 모름, 2 모름(코어 연결 실패 등)
func tinfo(addr string) (pid, name string, laneLabel bool, rc int) {
	out, hrc := hcRun(10, errMerge, "get", addr, "--json")
	if !reGetKnown.MatchString(out) {
		return "", "", false, 2
	}
	if hrc != 0 { // 실패한 hc get의 출력(잘린 응답 등)으로 「끝남」을 정하지 않는다 — 확인된 not_found만 1(gofix L04)
		if reNotFound.MatchString(out) {
			return "", "", false, 1
		}
		return "", "", false, 2
	}
	m := jparse(out)
	if m == nil {
		return "", "", false, 2
	}
	if m["state"] != "live" {
		return "", "", false, 1
	}
	pid = topNum(m, "pid", "")
	name, _ = jget(m, "name")
	if name == "" {
		name = "-"
	}
	if lb, ok := m["labels"].(map[string]any); ok {
		laneLabel = lb["hc-lane"] == "1" || lb["hc-lane-watch"] == "1"
	}
	if pid == "" {
		return pid, name, laneLabel, 1
	}
	return pid, name, laneLabel, 0
}

func coordFile(addr string) string { return evdir() + "/coord." + ckey(addr) }

// readCoord: 「주소 pid 이름 시각」(셸 판 IFS=탭 read — 빈 칸은 겹쳐 하나)
func readCoord(path string) ([]string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	l, _, nl := strings.Cut(string(b), "\n")
	if !nl {
		return nil, false
	}
	f := strings.FieldsFunc(l, func(r rune) bool { return r == '\t' })
	if len(f) > 4 {
		f = append(f[:3], strings.Join(f[3:], tab))
	}
	for len(f) < 4 {
		f = append(f, "")
	}
	return f, true
}

// coordSet: 살아 있고 레인·감시 터미널이 아니면 지휘 정보로 적는다. launch면 띄운 시각을 지금으로, 아니면 같은 pid의 옛 시각 그대로
func coordSet(addr string, launch bool) bool {
	pid, name, ll, rc := tinfo(addr)
	if rc != 0 || ll {
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
	return atomic.WriteFile(f, []byte(addr+tab+pid+tab+flat(name)+tab+ts+"\n"), 0o666) == nil
}

// coordCheck: 그 지휘의 정보가 있고 지금 hc get pid·이름이 같으면 0(주소 addr). 1 다름·없음(까닭 why), 2 모름
func coordCheck(addr string) (cad, why string, rc int) {
	f := coordFile(addr)
	c, ok := readCoord(f)
	if !ok {
		return "", fmt.Sprintf("지휘 정보 없음(%s — 지휘가 레인을 띄우거나 wait를 걸면 생김)", f), 1
	}
	cad = c[0]
	pid, name, _, r := tinfo(cad)
	if r == 2 {
		return cad, fmt.Sprintf("hc get %s 실패", cad), 2
	}
	if r != 0 {
		return cad, cad + " 터미널이 없거나 끝남", 1
	}
	if pid != c[1] || flat(name) != c[2] {
		return cad, fmt.Sprintf("%s 의 pid·이름(%s %s)이 지휘 정보(%s %s)와 다름", cad, pid, name, c[1], c[2]), 1
	}
	return cad, "", 0
}

// coordDefault: 살아 있고 맞는 지휘 가운데 가장 최근에 레인을 띄운 곳. 1 없음, 2 그보다 최근 지휘를 확인 못 함
func coordDefault() (string, string, int) {
	fs, _ := filepath.Glob(evdir() + "/coord.*")
	sort.Strings(fs)
	var bt, ut int64 = -1, -1
	ba := ""
	for _, cf := range fs {
		if strings.Count(filepath.Base(cf), ".") >= 2 {
			continue // 쓰는 중인 coord.<주소>.<pid>
		}
		c, ok := readCoord(cf)
		if !ok {
			continue
		}
		ct := int64(0)
		if digits(c[3]) {
			ct = atoi(c[3])
		}
		_, _, r := coordCheck(c[0])
		if r == 0 && ct > bt {
			bt, ba = ct, c[0]
		} else if r == 2 && ct > ut {
			ut = ct
		}
	}
	if ba == "" {
		return "", "살아 있고 pid·이름이 맞는 지휘가 없다(지휘가 레인을 띄우거나 wait를 걸면 생김)", 1
	}
	if ut > bt {
		return "", "더 최근 지휘를 확인 못 했다(hc get 실패)", 2
	}
	return ba, "", 0
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
		_, why, rc := coordCheck(a[1])
		if rc != 0 {
			fmt.Fprintf(os.Stderr, "hc-lane coord --check: %s\n", why)
		}
		return rc
	}
	cad, why, rc := coordDefault()
	if rc == 0 {
		fmt.Println(cad)
		return 0
	}
	fmt.Fprintf(os.Stderr, "hc-lane coord: 지휘 없음 — %s\n", why)
	return 1
}
