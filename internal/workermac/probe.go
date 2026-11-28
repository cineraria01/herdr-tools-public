package workermac

// 기계 점검 판정 가운데 여러 도구가 같은 것을 재는 것(ht-probe-cordon — 두 곳에서 따로 재지 않는다): worker-mac-check(손으로)·hc-sweep
// 건강 점검(바퀴마다 그 기계에서)·hc-lane merger --spec-idle(hc-fill 격리 점검 조각, docs/hc-fill/cordon.md)

import (
	"path/filepath"
	"regexp"
	"strconv"
)

// KeychainLocked: security show-keychain-info의 「User interaction is not allowed」 종료 코드(코어가 SSH 문맥으로 떠 키체인이 막힘).
// 0은 열림, 그 밖은 모름
const KeychainLocked = 36

// KeychainArgs: 로그인 키체인이 열려 있는지만 묻는 명령(암호·항목은 안 다룬다)
func KeychainArgs(home string) []string {
	return []string{"/usr/bin/security", "show-keychain-info", filepath.Join(home, "Library/Keychains/login.keychain-db")}
}

var poolUsableRe = regexp.MustCompile(`(?i)usable now:[ \t]*([0-9]+)`)

// PoolUsable: 풀 프록시 `<명령> status` 출력의 「Usable now: <n>」(지금 쓸 수 있는 계정 수). 줄이 없으면 ok 거짓(모름) — 계정 이름은 읽지 않는다
func PoolUsable(out string) (n int, ok bool) {
	m := poolUsableRe.FindStringSubmatch(out)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}
