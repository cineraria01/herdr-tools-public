package main

// 보낼 함(out/<큐 이름>)과 사건·레인 알림(bash moutbox·msend·mflush). 원격을 고친 뒤 사건을 쓰기 전에 죽어도 다음 바퀴가 보낸다.

import (
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

	"herdr-tools/internal/events"
	"herdr-tools/internal/proc"
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

// haveEvent: 이 기계 사건 파일에 같은 레인·상태로 같은 결과 열쇠가 든 사건이 이미 있나(열쇠가 없으면 같은 글).
// 사건 파일을 끝까지 읽지 못하면 오류 — 모르는 채 「없음」으로 사건을 또 쓰지 않는다(E12)
func haveEvent(ln, st, key, msg string) (bool, error) {
	found := false
	err := events.ScanFile(gEvdir+"/events.tsv", func(e events.Event) bool {
		if e.Lane != ln || e.State != st {
			return true
		}
		if key == "" {
			found = e.Msg == msg
			return !found
		}
		// 열쇠 뒤 글자가 ID 글자면 다른 열쇠(box-123 대 box-1234)
		if p := strings.Index(e.Msg, key); p >= 0 {
			if rest := e.Msg[p+len(key):]; rest == "" || !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_.~-", rune(rest[0])) {
				found = true
			}
		}
		return !found
	})
	if found {
		return true, nil // 찾았으면 뒤를 못 읽었어도 있음
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

// 알림 상한 — 사건 쓰기(hc-lane report)
const reportMax = 60 * time.Second

// boundedRun: 명령을 새 무리로 띄워 limit 안에서 기다린다(E05·F9) — 넘거나 자식이 끝났는데 출력 관이 안 닫히면 그 무리째 거두고 142
// (bash 판 perl alarm과 같은 종료, 결과는 모름). 표준 출력·종료 코드·표준 오류. env는 os.Environ() 뒤에 더할 것
func boundedRun(limit time.Duration, name string, env []string, args ...string) (string, int, string) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	var o, e bytes.Buffer
	c := exec.CommandContext(ctx, name, args...)
	c.Stdout, c.Stderr = &o, &e
	if env != nil {
		c.Env = append(os.Environ(), env...)
	}
	proc.Group(c)
	c.Cancel = func() error { proc.StopGroup(c.Process.Pid, time.Second); return nil }
	c.WaitDelay = time.Second
	err := c.Run()
	if errors.Is(err, exec.ErrWaitDelay) && c.Process != nil {
		proc.StopGroup(c.Process.Pid, time.Second) // 관을 쥔 손자
	}
	if ctx.Err() != nil || errors.Is(err, exec.ErrWaitDelay) {
		return "", 142, e.String()
	}
	return o.String(), proc.Code(err), e.String()
}

// hcRun: hc를 시간 상한 안에서. 표준 출력·종료 코드·표준 오류
func hcRun(limit time.Duration, args ...string) (string, int, string) {
	return boundedRun(limit, "hc", nil, args...)
}

// addrOK: hc에 넘길 주소 조각(기계·터미널) — 옵션으로 읽히지 않게(X1)
func addrOK(s string) bool {
	return s != "" && s[0] != '-' && strings.Trim(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_.-") == ""
}

// sameRun: hc get 결과가 그 레인의 같은 실행인가 — 최상위 state live·name = 레인·created_ms = 제출 때 적은 값.
// ok 거짓 = 응답을 해석하지 못함(잘린 JSON 등 — 다른 실행이라는 근거가 아니다, E06)
func sameRun(js, ln, born string) (same, ok bool) {
	d := json.NewDecoder(strings.NewReader(js))
	d.UseNumber()
	var v map[string]any
	if err := d.Decode(&v); err != nil {
		return false, false
	}
	cm := ""
	switch x := v["created_ms"].(type) {
	case json.Number:
		cm = x.String()
	case string:
		cm = x
	}
	if _, has := v["state"]; !has {
		return false, false
	}
	return v["state"] == "live" && v["name"] == ln && cm == born, true
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
	v, rerr := readLinesE(of, 11)
	if rerr != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 보낼 사건 %s 를 읽지 못했다 — 두고 다음 바퀴에: %v\n", n, rerr)
		return
	}
	st, sha, msg, ln, to, h, t, k, vk, born, ownerRun := v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8], v[9], v[10]
	have, herr := haveEvent(ln, st, k, msg)
	if herr != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 사건 파일을 끝까지 읽지 못했다(같은 사건이 있는지 모름) — 보낼 함을 두고 다음 바퀴에: %v\n", herr)
		return
	}
	if !have {
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
		if _, rc, e := boundedRun(reportMax, gSelf, []string{"HC_LANE_OWNER=" + owner, "HC_LANE_OWNER_RUN=" + ownerRun}, args...); rc != 0 {
			os.Stderr.WriteString(e)
			fmt.Fprintf(os.Stderr, "hc-lane merger: %s 사건을 쓰지 못했다(종료 %d) — 다음 바퀴에 다시\n", st, rc)
			return
		}
	}
	// 실행 시각이 없는 옛 제출도 결과 사건은 남는다. 직접 알림에는 같은 실행이라는 근거가 필요하다
	if t != "" && t != "-" && digits.MatchString(born) && addrOK(t) && (h == m.mme || addrOK(h)) {
		ad := t
		if h != m.mme {
			ad = h + "/" + t
		}
		retry := false
		js, gr, _ := hcRun(3*time.Second, "get", ad, "--json")
		switch gr {
		case 0:
			same, ok := sameRun(js, ln, born)
			switch {
			case !ok: // 응답을 해석하지 못함 — 다른 실행이라는 근거가 아니다(E06)
				retry = true
			case same:
				if _, sr, _ := hcRun(5*time.Second, "send", ad, fmt.Sprintf("hc-lane merger: %s %s %s — %s", ln, st, strings.TrimPrefix(sha, "-"), msg)); sr != 0 {
					retry = true
				}
			}
		case 3: // 그 터미널 없음 — 끝난 레인
		default: // 끊김·시간 넘김·그 기계를 모름(machine_unknown — 잠깐 끊긴 것일 수 있다) — bash 판처럼 세 번까지 다시(E06)
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
