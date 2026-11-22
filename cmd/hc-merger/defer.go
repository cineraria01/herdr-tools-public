package main

// 쉼(다음 바퀴에)·그 막힘 사건과 원격 목록(ls-remote) 읽기 — round.go에서 나눔(파일 줄 한도)

import (
	"fmt"
	"hash/fnv"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// defer_: 큐를 두고 다음 바퀴에(bash mdefer)
func (m *M) defer_(why string) {
	fmt.Fprintf(os.Stderr, "hc-lane merger: %s — 큐를 두고 다음 바퀴에\n", why)
	m.mer, m.mq = true, ""
	m.deferred = why
}

// deferNote: 같은 까닭의 「다음 바퀴에」가 deferAlarm 바퀴 이어지면 막힘 사건을 한 번 낸다(G4 — 영구 실패가 사건 없이 75만 되풀이하지 않게).
// 바퀴가 쉬지 않고 끝나면(다른 까닭 포함) 센 것을 지운다
const deferAlarm = 10

func (m *M) deferNote() {
	f := m.mj + "/defer"
	if m.deferred == "" {
		removeQuiet(f)
		return
	}
	// 파일: 「<횟수>\t<까닭>[\t<처음 쉰 초>\t<sent>]」 — 옛 꼴(두 칸)도 읽는다. 횟수와 사건 전달을 따로 적어, 임계에 이른 사건 쓰기가
	// 실패하면 다음 바퀴에 같은 열쇠로 다시 낸다(R-C08 — 10번째 사건 쓰기 실패 뒤 다시 내지 않았다)
	n, since, sent := 0, "", false
	if b, err := os.ReadFile(f); err == nil {
		if c := strings.Split(strings.TrimRight(string(b), "\n"), "\t"); len(c) >= 2 && c[1] == m.deferred {
			n, _ = strconv.Atoi(c[0])
			if len(c) >= 4 && digits.MatchString(c[2]) {
				since, sent = c[2], c[3] == "sent"
			}
		}
	}
	n++
	if since == "" {
		since = strconv.FormatInt(time.Now().Unix(), 10)
	}
	if n >= deferAlarm && !sent {
		h := fnv.New32a()
		h.Write([]byte(m.top + "\t" + m.deferred))
		sent = reportBlockedKey(fmt.Sprintf("병합기가 같은 까닭으로 %d바퀴째 쉼: %s (클론 %s)", n, m.deferred, m.top),
			fmt.Sprintf("merger:defer:%s:%08x", since, h.Sum32()))
	}
	st := "-"
	if sent {
		st = "sent"
	}
	if err := os.WriteFile(f, []byte(fmt.Sprintf("%d\t%s\t%s\t%s\n", n, m.deferred, since, st)), 0o666); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 쉰 횟수를 쓰지 못했다: %v\n", err)
	}
}

// reportBlockedKey: 막힘 사건(--name merger)을 결과 열쇠와 함께 상한 안에서 — 같은 열쇠의 사건이 이미 있으면 report가 다시 쓰지 않는다.
// 적었으면(또는 이미 있으면) true
func reportBlockedKey(msg, key string) bool {
	if _, rc, e := boundedRun(reportMax, gSelf, nil, "report", "막힘", "-", msg, "--name", "merger", "--key", key); rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 막힘 사건을 적지 못했다(종료 %d) — 다음 바퀴에 다시: %s\n", rc, strings.TrimSpace(e))
		return false
	}
	return true
}

// refsha: ls-remote 목록에서 그 참조의 SHA(없으면 빈 값)
func refsha(list, ref string) string {
	for _, line := range strings.Split(list, "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == ref {
			return f[0]
		}
	}
	return ""
}

// lsRemote: git ls-remote <인자…> — 종료 0이어도 줄마다 「<SHA 40|64자>\t<참조>」 꼴이어야 받는다. 잘린 줄(SHA 앞 12자 등)이 하나라도
// 있으면 목록 전체가 모름(종료 1)이다 — 잘린 줄을 「그 표식 없음」으로 읽고 일지를 지우지 않게(R-H03). 정상 빈 출력만 「없음」
func lsRemote(args ...string) (string, int) {
	l, rc := out(append([]string{"ls-remote"}, args...)...)
	if rc != 0 || l == "" {
		return l, rc
	}
	for _, line := range strings.Split(l, "\n") {
		sha, ref, ok := strings.Cut(line, "\t")
		if !ok || !objName.MatchString(sha) || ref == "" || strings.ContainsAny(ref, " \t") {
			fmt.Fprintf(os.Stderr, "hc-lane merger: git ls-remote 출력 줄 꼴이 틀렸다(잘림?) — 모름으로 본다: %q\n", line)
			return "", 1
		}
	}
	return l, 0
}

var objName = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
