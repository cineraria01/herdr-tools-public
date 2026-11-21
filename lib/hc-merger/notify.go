package main

// 보낼 함(out/<큐 이름>)과 사건·레인 알림(bash moutbox·msend·mflush). 원격을 고친 뒤 사건을 쓰기 전에 죽어도 다음 바퀴가 보낸다.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// outbox: 보낼 사건을 out/<이름>에 — 상태·sha·글·레인·지휘(--to)·기계·터미널·결과 열쇠·판정 열쇠·터미널 실행 시각·지휘 실행
func (m *M) outbox(mt meta, n, st, sha, msg, ln string) bool {
	vk := ""
	if st == "착지" && mt.verdict != "" {
		vk = "hf:" + mt.verdict + ":landed"
	}
	body := strings.Join([]string{st, sha, msg, ln, mt.owner, mt.host, mt.term, mt.key(n), vk, mt.born, mt.ownerRun}, "\n") + "\n"
	if err := writeVia(m.mj+"/out/"+n, body); err != nil {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 보낼 사건을 적지 못했다")
		return false
	}
	return true
}

// flush: 보낼 함에 남은 것을 모두 보낸다(바퀴 처음에)
func (m *M) flush() {
	des, err := os.ReadDir(m.mj + "/out")
	if err != nil {
		return
	}
	names := []string{}
	for _, d := range des {
		if n := d.Name(); d.Type().IsRegular() && !strings.HasSuffix(n, ".tmp") && !strings.HasSuffix(n, ".try") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		m.send(n)
	}
}

// haveEvent: 이 기계 사건 파일에 같은 레인·상태로 같은 결과 열쇠가 든 사건이 이미 있나(열쇠가 없으면 같은 글)
func haveEvent(ln, st, key, msg string) bool {
	f, err := os.Open(gEvdir + "/events.tsv")
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		c := strings.Split(sc.Text(), "\t")
		for len(c) < 7 {
			c = append(c, "")
		}
		if c[3] != ln || c[4] != st {
			continue
		}
		if key == "" {
			if c[6] == msg {
				return true
			}
			continue
		}
		// 열쇠 뒤 글자가 ID 글자면 다른 열쇠(box-123 대 box-1234)
		if p := strings.Index(c[6], key); p >= 0 {
			if rest := c[6][p+len(key):]; rest == "" || !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_.~-", rune(rest[0])) {
				return true
			}
		}
	}
	return false
}

// hcRun: hc를 시간 상한 안에서. 표준 출력·종료 코드·표준 오류
func hcRun(limit time.Duration, args ...string) (string, int, string) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	var o, e bytes.Buffer
	c := exec.CommandContext(ctx, "hc", args...)
	c.Stdout, c.Stderr = &o, &e
	err := c.Run()
	if ctx.Err() != nil {
		return o.String(), 142, e.String() // bash 판의 perl alarm(SIGALRM)과 같은 종료
	}
	return o.String(), code(err), e.String()
}

// sameRun: hc get 결과가 그 레인의 같은 실행인가 — 최상위 state live·name = 레인·created_ms = 제출 때 적은 값
func sameRun(js, ln, born string) bool {
	d := json.NewDecoder(strings.NewReader(js))
	d.UseNumber()
	var v map[string]any
	if err := d.Decode(&v); err != nil {
		return false
	}
	cm := ""
	switch x := v["created_ms"].(type) {
	case json.Number:
		cm = x.String()
	case string:
		cm = x
	}
	return v["state"] == "live" && v["name"] == ln && cm == born
}

// send: 사건(--name <레인>, 제출 때 적은 지휘가 있으면 --to)을 — 같은 열쇠의 사건이 아직 없을 때만 — 적고, 제출한 레인 터미널이
// 살아 있고 같은 실행이면 hc send로도 알린다(이 기계면 <터미널>, 다른 기계면 <기계>/<터미널>). 알림이 끊기면 보낼 함에 두고 다음
// 바퀴에 다시, 세 번까지. 터미널이 없거나(not_found) 이 기계가 그 기계를 모르면(machine_unknown — 워커 기계는 지휘 맥을 모른다,
// 요구 6) 사건만 남기고 다시 하지 않는다
// ponytail: hc get과 hc send 사이에 주소가 다른 실행에 다시 쓰이는 것은 막지 못한다(확실한 결과는 제출 ID가 든 사건)
func (m *M) send(n string) {
	of := m.mj + "/out/" + n
	if _, err := os.Stat(of); err != nil {
		return
	}
	v := readLines(of, 11)
	st, sha, msg, ln, to, h, t, k, vk, born, ownerRun := v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8], v[9], v[10]
	if !haveEvent(ln, st, k, msg) {
		owner := to
		if owner == "" {
			owner = "*"
		}
		args := []string{"report", st, sha, msg, "--name", ln}
		if to != "" {
			args = append(args, "--to", to)
		}
		if vk != "" {
			args = append(args, "--key", vk)
		}
		c := exec.Command(gSelf, args...)
		c.Stderr = os.Stderr
		c.Env = append(os.Environ(), "HC_LANE_OWNER="+owner, "HC_LANE_OWNER_RUN="+ownerRun)
		if err := c.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "hc-lane merger: %s 사건을 쓰지 못했다 — 다음 바퀴에 다시\n", st)
			return
		}
	}
	// 실행 시각이 없는 옛 제출도 결과 사건은 남는다. 직접 알림에는 같은 실행이라는 근거가 필요하다
	if t != "" && t != "-" && digits.MatchString(born) {
		ad := t
		if h != m.mme {
			ad = h + "/" + t
		}
		retry := false
		js, gr, gerr := hcRun(3*time.Second, "get", ad, "--json")
		switch {
		case gr == 0:
			if sameRun(js, ln, born) {
				if _, sr, _ := hcRun(5*time.Second, "send", ad, fmt.Sprintf("hc-lane merger: %s %s %s — %s", ln, st, strings.TrimPrefix(sha, "-"), msg)); sr != 0 {
					retry = true
				}
			}
		case gr == 3: // 그 터미널 없음 — 끝난 레인
		case strings.Contains(gerr+js, `"machine_unknown"`):
			fmt.Fprintf(os.Stderr, "hc-lane merger: %s 레인 터미널(%s)의 기계를 이 기계가 모른다 — 사건만 남김\n", ln, ad)
		default:
			retry = true
		}
		if retry {
			tr := 0
			if b, err := os.ReadFile(of + ".try"); err == nil {
				if x, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
					tr = x
				}
			}
			tr++
			if tr < 3 {
				if err := os.WriteFile(of+".try", []byte(fmt.Sprintf("%d\n", tr)), 0o666); err != nil {
					fmt.Fprintf(os.Stderr, "hc-lane merger: 알림 시도 횟수를 쓰지 못했다: %v\n", err)
				}
				fmt.Fprintf(os.Stderr, "hc-lane merger: %s 레인 터미널(%s)에 알리지 못했다 — 다음 바퀴에 다시(%d/3)\n", ln, ad, tr)
				return
			}
			fmt.Fprintf(os.Stderr, "hc-lane merger: %s 레인 터미널(%s) 알림을 세 번 못 보냈다 — 사건만 남김\n", ln, ad)
		}
	}
	for _, p := range []string{of, of + ".try"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "hc-lane merger: %s 를 지우지 못했다: %v\n", p, err)
		}
	}
}
