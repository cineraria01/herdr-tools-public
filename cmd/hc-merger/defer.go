package main

// 쉼(다음 바퀴에)·그 막힘 사건과 원격 목록(ls-remote) 읽기 — round.go에서 나눔(파일 줄 한도)

import (
	"fmt"
	"hash/fnv"
	"os"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/landing"
	"herdr-tools/internal/proc"
)

// defer_: 큐를 두고 다음 바퀴에(bash mdefer)
func (m *M) defer_(why string) {
	fmt.Fprintf(os.Stderr, "hc-lane merger: %s — 큐를 두고 다음 바퀴에\n", why)
	m.mer, m.mq = true, ""
	m.deferred = why
}

// deferNote: 같은 까닭의 「다음 바퀴에」가 손잡이 merger_defer_alarm(기본 deferAlarm) 바퀴 이어지면 막힘 사건을 한 번 낸다(G4 — 영구
// 실패가 사건 없이 75만 되풀이하지 않게). 바퀴가 쉬지 않고 끝나면(다른 까닭 포함) 센 것을 지운다. 시작 단계(startWait)는 손잡이를 읽기 전이라 기본
const deferAlarm = 10

func (m *M) deferNote() {
	f := m.mj + "/defer"
	// 파일: 「<횟수>\t<까닭>[\t<처음 쉰 초>\t<sent>]」 — 옛 꼴(두 칸)도 읽는다. 횟수와 사건 전달을 따로 적어, 임계에 이른 사건 쓰기가
	// 실패하면 다음 바퀴에 같은 열쇠로 다시 낸다(R-C08 — 10번째 사건 쓰기 실패 뒤 다시 내지 않았다)
	var c []string
	if b, err := os.ReadFile(f); err == nil {
		c = strings.Split(strings.TrimRight(string(b), "\n"), "\t")
	}
	// 알린 쉼이 끝남(쉬지 않은 바퀴·다른 까닭) → 같은 열쇠 + :up 한 번(ht-alert-sources). 못 적으면 기록을 두고 다음 바퀴에 다시
	if len(c) >= 4 && c[3] == "sent" && c[1] != m.deferred && digits.MatchString(c[2]) {
		msg := fmt.Sprintf("병합기 쉼 끝 — 같은 까닭 %s바퀴 쉼이 풀림: %s (클론 %s)", c[0], c[1], m.top)
		if !reportKey("중간", msg, deferKey(m.top, c[1], c[2])+":up") {
			return
		}
		c = nil
	}
	if m.deferred == "" {
		removeQuiet(f)
		return
	}
	n, since, sent := 0, "", false
	if len(c) >= 2 && c[1] == m.deferred {
		n, _ = strconv.Atoi(c[0])
		if len(c) >= 4 && digits.MatchString(c[2]) {
			since, sent = c[2], c[3] == "sent"
		}
	}
	n++
	if since == "" {
		since = strconv.FormatInt(time.Now().Unix(), 10)
	}
	if n >= intSetting("merger_defer_alarm", rk["merger_defer_alarm"], deferAlarm, 1) && !sent {
		sent = reportBlockedKey(fmt.Sprintf("병합기가 같은 까닭으로 %d바퀴째 쉼: %s (클론 %s)", n, m.deferred, m.top), deferKey(m.top, m.deferred, since))
	}
	st := "-"
	if sent {
		st = "sent"
	}
	if err := os.WriteFile(f, []byte(fmt.Sprintf("%d\t%s\t%s\t%s\n", n, m.deferred, since, st)), 0o666); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 쉰 횟수를 쓰지 못했다: %v\n", err)
	}
}

// childWait: 죽은 앞 실행이 남긴 착지 자식(<일지>/child)·고아 무리(proc 기록) 때문에 이 바퀴를 쉴 까닭(없으면 빈 값 — 무리 번호·기록
// 경로를 담아 같은 기록이면 같은 글). count = 쉰 바퀴로 셀 것(deferNote — deferAlarm 바퀴 이어지면 막힘 사건). 기록한 그 착지가 착지
// 상한 안에서 도는 중이면 세지 않고 끝나기를 기다린다(재감사 3바퀴 merger/R-3M01)
func (m *M) childWait() (why string, count bool) {
	rec := m.mj + "/child"
	switch st, pid := proc.ChildCheck(rec); st {
	case proc.ChildRunning:
		if fi, err := os.Stat(rec); err == nil && (landMax() <= 0 || time.Since(fi.ModTime()) < landMax()+time.Minute) {
			return fmt.Sprintf("앞 실행의 착지 자식 %d 이 아직 돈다(기록 %s)", pid, rec), false
		}
		return fmt.Sprintf("앞 실행의 착지 자식 %d 이 착지 상한을 넘겨 아직 돈다(기록 %s)", pid, rec), true
	case proc.ChildLeft:
		return fmt.Sprintf("앞 실행의 착지 자식 %d 은 끝났는데 그 무리가 남았다 — 소유를 입증할 수 없어 신호 없이 기다림(기록 %s)", pid, rec), true
	case proc.ChildUnknown:
		return fmt.Sprintf("앞 실행의 착지 자식 기록을 확인하지 못했다(모름, 기록 %s)", rec), true
	}
	if l := proc.Left(); len(l) > 0 {
		return fmt.Sprintf("앞 실행이 남긴 자식 무리가 아직 도는지 모른다: %s", strings.Join(l, ", ")), true
	}
	return "", false
}

// deferKey: 같은 까닭 쉼 막힘의 열쇠(풀림은 + :up)
func deferKey(top, why, since string) string {
	h := fnv.New32a()
	h.Write([]byte(top + "\t" + why))
	return fmt.Sprintf("merger:defer:%s:%08x", since, h.Sum32())
}

// reportBlockedKey: 막힘 사건(--name merger)을 결과 열쇠와 함께 상한 안에서 — 같은 열쇠의 사건이 이미 있으면 report가 다시 쓰지 않는다.
// 적었으면(또는 이미 있으면) true
func reportBlockedKey(msg, key string) bool { return reportKey("막힘", msg, key) }

// reportKey: 사건(--name merger)을 열쇠와 함께 상한 안에서 — 막힘과 그 풀림(같은 열쇠 + :up 중간)이 같은 길. 적었으면 true
func reportKey(st, msg, key string) bool {
	if _, rc, e := boundedRun(reportMax, gSelf, nil, "report", st, "-", msg, "--name", "merger", "--key", key); rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: %s 사건을 적지 못했다(종료 %d) — 다음 바퀴에 다시: %s\n", st, rc, strings.TrimSpace(e))
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

// lsRemote <원격> <패턴…>: gitx.LsRemote(--exit-code) — 줄마다 「<SHA 40|64자>\t<참조>」, 이름이 요청 패턴 안, 끝 줄바꿈까지 와야
// 받는다. 하나라도 어긋나면(잘린 줄·요청 끝 글자가 빠진 이름·요청 밖 이름) 목록 전체가 모름(종료 1) — 「그 표식 없음」으로 읽고 일지를
// 지우지 않게(R-H03, 재감사 merger/R-201). 「없음」은 종료 2(맞는 참조 없음)뿐. 돌려주는 목록은 이름순 「<SHA>\t<참조>」 줄(끝 줄바꿈 없음)
func lsRemote(args ...string) (string, int) {
	if len(args) < 2 {
		return "", 1
	}
	refs, st := gitx.LsRemote("", args[0], os.Stderr, args[1:]...)
	if st == gitx.Unknown {
		fmt.Fprintf(os.Stderr, "hc-lane merger: git ls-remote 응답이 온전하지 않다(실패·잘림·요청 밖 이름) — 모름으로 본다: %s\n", strings.Join(args[1:], " "))
		return "", 1
	}
	var l []string
	for _, r := range landing.Refs(refs) {
		l = append(l, r.SHA+"\t"+r.Name)
	}
	return strings.Join(l, "\n"), 0
}
