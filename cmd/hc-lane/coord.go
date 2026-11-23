// coord.go: 지휘 정보(<evdir>/coord.<주소> 「주소<탭>pid<탭>이름<탭>마지막으로 레인을 띄운 시각」)와 hc-lane coord
// (셸 판 lib/hc-lane/watch.sh tinfo·coord_is·coord_set·coord_check·coord_default, bin/hc-lane coord 분기)
package main

import (
	"encoding/json"
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
	if rc != 0 && strings.TrimSpace(eb.String()) != "" {
		out = eb.String()
	}
	return out, rc
}

// getState: hc get 판정. 0 살아 있음(state live), 1 없음(완전한 not_found)·끝남(확인된 finished), 2 모름(hcGet 오류 — gofix L04, 재감사 R-13)
func getState(t hcjson.Terminal, found bool, err error) int {
	switch {
	case err != nil:
		return 2
	case !found || t.State != "live":
		return 1
	}
	return 0
}

// tinfo: 그 터미널의 맨 위 pid·이름(없으면 -)·레인/감시 라벨. rc 0 살아 있음, 1 확인된 없음·끝남, 2 모름 — getState의 모름에 더해
// live인데 신원 칸(pid·name)이 응답에 없거나 pid가 null·0이면 모름(실행 신원을 확인 못 한 것은 「없음」이 아니다 — 재감사2 lane/R-208, fill/R-206)
func tinfo(addr string) (pid, name string, laneLabel bool, rc int) {
	return tinfoOf(hcGet(10, addr))
}

// tinfoOf: tinfo의 판정을 이미 받은 hc get 결과로
func tinfoOf(t hcjson.Terminal, found bool, err error) (pid, name string, laneLabel bool, rc int) {
	if r := getState(t, found, err); r != 0 {
		return "", "", false, r
	}
	_, hasPid := t.Raw["pid"]
	_, hasName := t.Raw["name"]
	if !hasPid || !hasName || t.PID <= 0 {
		return "", "", false, 2
	}
	name = t.Name
	if name == "" {
		name = "-"
	}
	laneLabel = t.Labels["hc-lane"] == "1" || t.Labels["hc-lane-watch"] == "1"
	return strconv.Itoa(t.PID), name, laneLabel, 0
}

// agentState: 그 터미널에 지금 터미널 입력을 받는 프로그램(에이전트)이 있나 — hc get 한 번(t)으로. 0 있음,
// 1 없음(대화형 셸 자신이 전경이고 status 없음 — 붙여 넣은 글을 셸이 명령으로 실행한다), 2 모름(까닭). 근거는 2026-11-23 실측
// (hc 0.1.21·0.1.22, 견본 testdata/agent, docs/hc-lane/watch.md 「받는 쪽 에이전트 확인」):
// 에이전트가 돌면 전경 pid가 셸과 다르다(job control). zsh -c 꼴은 자식이 돌아도 전경 pid가 셸과 같지만 그 셸은 터미널을 읽지 않으니 있음.
// 판정이 헷갈리면 모름(다시 봄)이지 없음(끝냄)이 아니다 — 도는 지휘에게 사건이 안 가는 것이 가장 비싼 실수다
func agentState(t hcjson.Terminal) (int, string) {
	var fg struct {
		PID  *int     `json:"pid"`
		Argv []string `json:"argv"`
	}
	var argv []string
	if r, ok := t.Raw["foreground"]; ok && string(r) != "null" {
		if json.Unmarshal(r, &fg) != nil || fg.PID == nil || *fg.PID <= 0 {
			return 2, "hc get의 전경 프로세스(foreground pid)를 못 읽음"
		}
		// 전경이 다른 프로그램(에이전트·그 배경 명령·하단 줄 실행기)이면 있음. 인자 없는 중첩 대화형 셸(bash·zsh -i 등)이면 아래 같은
		// 판정. nix-shell의 `bash --rcfile <경로>` 꼴은 스크립트 인자로 보아 있음으로 읽는다(안전한 쪽)
		if *fg.PID != t.PID && (len(fg.Argv) == 0 || !interactiveShell(fg.Argv)) {
			return 0, ""
		}
		if len(fg.Argv) == 0 {
			return 2, "전경이 맨 위 프로세스인데 hc get에 전경 argv가 없다"
		}
		argv = fg.Argv
	} else {
		r, ok := t.Raw["argv"]
		if !ok {
			// ponytail: 판정 칸(foreground·argv)이 둘 다 없는 응답은 가를 근거가 없어 이 확인을 건너뛴다(옛 동작) — 잰 hc 0.1.21·0.1.22는 둘 다 준다
			return 0, ""
		}
		// 전경 칸이 없다(옛 hc·막 뜸) — 맨 위 argv가 대화형 셸일 때만 모름, 셸이 아닌 프로그램이면 그 프로그램이 터미널을 받는다
		if json.Unmarshal(r, &argv) != nil || len(argv) == 0 {
			return 2, "hc get의 argv를 못 읽음"
		}
		if interactiveShell(argv) {
			return 2, "맨 위가 대화형 셸인데 hc get에 전경 프로세스가 없다"
		}
		return 0, ""
	}
	if !interactiveShell(argv) {
		return 0, ""
	}
	if _, ok := t.Raw["status"]; !ok {
		return 2, "전경이 대화형 셸인데 hc get에 status 칸이 없다"
	}
	if t.Status != "" {
		return 2, "전경이 대화형 셸인데 status가 " + t.Status
	}
	return 1, "에이전트가 끝나 셸만 남음(전경 " + flat(strings.Join(argv, " ")) + ")"
}

// interactiveShell: 터미널을 읽는 셸인가 — 셸 이름(로그인 셸의 앞 「-」 무시)이고 -c·스크립트 인자가 없다
func interactiveShell(argv []string) bool {
	switch filepath.Base(strings.TrimPrefix(argv[0], "-")) {
	case "sh", "bash", "zsh", "fish", "dash", "ksh", "mksh", "tcsh", "csh":
	default:
		return false
	}
	for _, a := range argv[1:] {
		if !strings.HasPrefix(a, "-") || !strings.HasPrefix(a, "--") && strings.ContainsRune(a, 'c') {
			return false
		}
	}
	return true
}

func coordFile(addr string) string { return evdir() + "/coord." + ckey(addr) }

// readCoord: 「주소 pid 이름 시각」(셸 판 IFS=탭 read — 빈 칸은 겹쳐 하나)
func readCoord(path string) ([]string, bool) {
	c, r := coordRead(path)
	return c, r == 0
}

// coordRead: readCoord와 같되 0 있음, 1 없음(ENOENT)·꼴이 아님(줄바꿈 없음 — 쓰는 중), 2 읽기 실패(권한·폴더·입출력 — 모름)
func coordRead(path string) ([]string, int) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 1
	} else if err != nil {
		return nil, 2
	}
	l, _, nl := strings.Cut(string(b), "\n")
	if !nl {
		return nil, 1
	}
	f := strings.FieldsFunc(l, func(r rune) bool { return r == '\t' })
	if len(f) > 4 {
		f = append(f[:3], strings.Join(f[3:], tab))
	}
	for len(f) < 4 {
		f = append(f, "")
	}
	return f, 0
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
	// 사용자 전용 장부 — 구판이 0666으로 만든 기존 파일도 갈 때 0600으로 좁힌다(재감사2 lane/R-221·merger/R-206)
	return atomic.WriteFileMode(f, []byte(addr+tab+pid+tab+flat(name)+tab+ts+"\n"), 0o600) == nil
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

// coordCheck: 그 지휘의 정보가 있고 지금 hc get pid·이름이 같으면 0(주소 addr). 1 다름·없음(까닭 why), 2 모름
func coordCheck(addr string) (cad, why string, rc int) {
	f := coordFile(addr)
	c, cr := coordRead(f)
	if cr == 2 {
		return "", fmt.Sprintf("지휘 정보를 읽지 못했다(%s)", f), 2
	}
	if cr != 0 {
		return "", fmt.Sprintf("지휘 정보 없음(%s — 지휘가 레인을 띄우거나 wait를 걸면 생김)", f), 1
	}
	cad = c[0]
	t, found, err := hcGet(10, cad)
	pid, name, _, r := tinfoOf(t, found, err)
	if r == 2 {
		return cad, fmt.Sprintf("hc get %s 로 지휘의 신원(state·pid·이름)을 확인 못 함", cad), 2
	}
	if r != 0 {
		return cad, cad + " 터미널이 없거나 끝남", 1
	}
	if pid != c[1] || flat(name) != c[2] {
		return cad, fmt.Sprintf("%s 의 pid·이름(%s %s)이 지휘 정보(%s %s)와 다름", cad, pid, name, c[1], c[2]), 1
	}
	// pid·이름이 같아도 대화형 셸에서 띄운 지휘는 에이전트가 끝나면 셸만 남는다(watch-agent-guard) — 받는 쪽에 에이전트가 있어야 지휘
	switch a, why := agentState(t); a {
	case 1:
		return cad, cad + " 의 " + why, 1
	case 2:
		return cad, fmt.Sprintf("%s 에이전트 확인 못 함 — %s", cad, why), 2
	}
	return cad, "", 0
}

// coordDefault: 살아 있고 맞는 지휘 가운데 가장 최근에 레인을 띄운 곳. 1 없음, 2 그보다 최근 지휘를 확인 못 함
func coordDefault() (string, string, int) {
	cfs, _ := filepath.Glob(evdir() + "/coord.*")
	sort.Strings(cfs)
	var bt, ut int64 = -1, -1
	ba := ""
	for _, cf := range cfs {
		if strings.Count(filepath.Base(cf), ".") >= 2 {
			continue // 쓰는 중인 coord.<주소>.<pid>
		}
		c, cr := coordRead(cf)
		if cr == 2 { // 읽지 못한 후보는 「없음」이 아니다 — 시각을 모르니 어느 확인된 지휘보다 최근일 수 있다
			ut = 1<<62 - 1
			continue
		}
		if cr != 0 {
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
		if ut >= 0 { // 확인된 지휘는 없지만 조회 못 한 후보가 있다 — 「없음」이 아니다(R-13)
			return "", "확인 못 한 지휘 정보가 있다(hc get 실패·잘린 응답·읽기 실패)", 2
		}
		return "", "살아 있고 pid·이름이 맞는 지휘가 없다(지휘가 레인을 띄우거나 wait를 걸면 생김)", 1
	}
	if ut > bt {
		return "", "더 최근 지휘를 확인 못 했다(hc get 실패·읽기 실패)", 2
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
	if rc == 2 {
		fmt.Fprintf(os.Stderr, "hc-lane coord: 지휘 모름 — %s\n", why)
		return 2
	}
	fmt.Fprintf(os.Stderr, "hc-lane coord: 지휘 없음 — %s\n", why)
	return 1
}
