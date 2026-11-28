package main

// 원격 로그인 거절 격리(ht-cordon-remote-auth, 2026-11-27, docs/hc-fill/cordon.md 「원격 로그인 점검」): 워커의 원격 저장소 토큰이 서버
// 쪽에서 폐기됐는데 연결·부하·저장소 읽기는 좋아 대기열이 그 기계에 레인을 계속 보냈고, 레인은 구현·감사까지 한 뒤 착지(git ls-remote)에서
// 막혔다(사람이 다른 지휘의 제보로 알아챔). 저장소 읽기 격리(cordon.go)와 같은 길에 점검 하나(cdAuth)를 더한다.
//   - 점검: 물을 차례인 기계만 바퀴마다 이미 부르는 조각(loadSnip)에 --remote-auth(cmd/hc-merger/remoteauth.go — 착지와 같은 원격 읽기 한 번).
//     좋은 기계는 cdAuth.every마다, 나쁘거나 격리 중·회복 알림이 남은 기계는 바퀴마다.
//   - 나쁨은 자격이 거절된 글뿐(authBad — 인증 실패·사용자 이름을 못 읽음·나쁜 자격·공개 키 거절·401·403). 그 밖은 모두 모름:
//     안 끝남·DNS·연결 실패·5xx·속도 제한(403이어도)·폴더 없음·옛 판 워커. 저장소 읽기와 달리 안 끝남은 나쁨이 아니다 — 원격 서비스가
//     느리거나 죽었을 때 워커 전부를 격리하면 안 된다.
//   - 오류 글은 받는 자리에서 주소 안의 자격·토큰 꼴을 지운다(authRedact) — 기록·알림·기계 줄·로그에 남지 않게.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"herdr-tools/internal/opsstate"
)

// cdAuth: 원격 로그인 — 좋은 기계는 7분마다(합격 기준 15분: 간격 7분 + 한 바퀴 + 문턱 5분 + 한 바퀴 = 14분, 바퀴 실행·cordon@ push 지연 몫 1분)
var cdAuth = &cordonCk{"-auth", "원격 로그인", "거절", opsstate.CordonAuthPre, opsstate.CordonAuth.Look, 420, authVerdict, false}

// AUTHCK: 이번 바퀴에 물은 원격 로그인 점검(기계 → remote-auth 줄, 물었는데 줄이 없으면 빈 값 — 옛 판 워커). 없는 기계는 못 물음
var AUTHCK map[string]string

// authDue: 그 기계에 원격 로그인을 물을 차례(loadSnip·cordonTick — 물었는지는 셈하지 않고 조각 출력의 표식 authAsked로만 본다)
// 쓰기 자격(cdWrite)은 같은 호출에 실려 그 점검이 나쁘거나 회복 알림이 남아도 바퀴마다 묻는다
func authDue(m string) bool {
	now := nowUnix()
	return len(repoPaths(m)) > 0 && (cdAuth.due(cdAuth.load(), m, now) || cdWrite.due(cdWrite.load(), m, now))
}

// authAsked: 원격 로그인을 물은 조각이 먼저 내는 표식 줄(loadSnip → repoTake)
const authAsked = "remote-auth-asked"

// authLine: 출력에서 첫 「remote-auth …」 줄(없으면 빈 값)
func authLine(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, "remote-auth ") {
			return l
		}
	}
	return ""
}

var (
	// authBad: 자격이 거절된 글(git 판·호스트마다 다름 — 2026-11-27 실제 확인 git 2.56.1의 글은 cordonauth_test.go 고정 자료)
	authBad = regexp.MustCompile(`(?i)authentication failed|could not read (username|password)|bad credentials|invalid (username or (token|password)|credentials)|` +
		`permission denied \(publickey|permission to \S+ denied|returned error: 40[13]\b|\bhttp[/0-9.]* 40[13]\b`)
	// authUnsure: 거절 글이 섞여도 모름(속도 제한은 403으로도 온다)
	authUnsure = regexp.MustCompile(`(?i)rate limit|too many requests|returned error: (429|5\d\d)\b|\bhttp[/0-9.]* (429|5\d\d)\b`)
	reURLCred  = regexp.MustCompile(`(://)[^/@\s'"]+@`)
)

// authRedact: 주소 안의 자격(https://user:token@…)과 알려진 토큰 꼴(secret.go lcSecretHeads)을 지운다
func authRedact(s string) string {
	s = reURLCred.ReplaceAllString(s, "$1<자격>@")
	for _, h := range lcSecretHeads {
		s = h.re.ReplaceAllString(s, "<가림>")
	}
	return s
}

// authVerdict: 「remote-auth <차례> <종료|timeout|none> <호스트> <오류 줄들(| 로 이음)>」 → 1 좋음(종료 0·2 — 2는 빈 원격) ·
// 2 나쁨(「<프로젝트> <호스트>: <거절 줄>」) · 3 물었는데 모름 · 0 못 물음
func authVerdict(m string) (int, string) {
	l, asked := AUTHCK[m]
	if !asked {
		return 0, ""
	}
	return remoteVerdict(m, l, "remote-auth")
}

// remoteVerdict: 원격 점검 줄 하나(머리 head — remote-auth·remote-write, 같은 꼴·같은 거절 글)의 판정
func remoteVerdict(m, l, head string) (int, string) {
	f := strings.SplitN(l, " ", 5)
	ps := repoPaths(m)
	if len(f) < 5 || f[0] != head {
		return 3, ""
	}
	i, err := strconv.Atoi(f[1])
	if err != nil || i < 1 || i > len(ps) {
		return 3, ""
	}
	if f[2] == "0" || f[2] == "2" {
		return 1, ""
	}
	msg := authRedact(f[4])
	if _, err := strconv.Atoi(f[2]); err != nil || authUnsure.MatchString(msg) || !authBad.MatchString(msg) {
		return 3, ""
	}
	line := msg
	for _, part := range strings.Split(msg, " | ") {
		if authBad.MatchString(part) {
			line = part
			break
		}
	}
	return 2, fmt.Sprintf("%s %s: %s", ps[i-1].p, f[3], bcutS(line, 200))
}
