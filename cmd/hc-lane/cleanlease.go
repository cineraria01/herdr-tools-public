package main

// clean의 레인 주인·임대(셸 판 lib/hc-lane/clean.sh의 clean_owner·clean_lease와 같은 판정)

import (
	"fmt"
	"os"
	"strings"
)

// legacyParked: 이 기계 사건(events.tsv)에서 그 레인의 마지막 주차·제출 사건이 그 커밋(sha, 7자 이상 앞부분도)의 「주차」인가 —
// 옛 판 주차(원격 lane/<레인>)를 반송된 제출 가지와 가른다(gofix L07). 못 읽으면 false(멈춤 쪽)
func legacyParked(lane, sha string) bool {
	ls, err := readLines(evdir() + "/events.tsv")
	if err != nil {
		return false
	}
	ok := false
	for _, l := range ls {
		f := strings.Split(l, "\t")
		if len(f) < 6 || f[3] != lane || (f[4] != "주차" && f[4] != "제출") {
			continue
		}
		ok = f[4] == "주차" && len(f[5]) >= 7 && strings.HasPrefix(sha, f[5])
	}
	return ok
}

// cleanOwner: 가지 별칭으로 정리할 때도 띄울 때 기록한 레인 이름과 임대 커밋을 쓴다(owner·saved). 조회 실패면 false
func cleanOwner() bool {
	owner = name
	if owner == "" {
		owner = branch
	}
	if name == "" && os.Getenv("LANE_BRANCH") == branch {
		owner = os.Getenv("HC_LANE")
		if owner == "" {
			owner = branch
		}
	}
	var r int
	saved, r = xout("git", "-C", repo, "rev-parse", "-q", "--verify", "refs/hc-lease-owners/"+branch)
	if r != 0 && r != 1 {
		return false
	}
	if saved != "" {
		title, r := xout("git", "-C", repo, "log", "-1", "--format=%s", saved)
		if r != 0 || !strings.HasPrefix(title, "hc-lane lease ") {
			return false
		}
		owner, _, _ = strings.Cut(strings.TrimPrefix(title, "hc-lane lease "), " ")
	}
	return true
}

// cleanLease: 정리가 끝나면 이 실행의 임대만 놓는다
func cleanLease() {
	if !remote {
		return
	}
	if !cleanOwner() {
		errf("hc-lane clean: 임대 기록 조회 실패 — 임대를 보존한다\n")
		return
	}
	token := saved
	if os.Getenv("HC_LANE") == owner && os.Getenv("HC_LANE_LEASE") != "" {
		token = os.Getenv("HC_LANE_LEASE")
	}
	if token == "" {
		return
	}
	if dry {
		fmt.Printf("[dry-run] 이 실행의 임대 refs/hc-lanes/%s 확인\n", owner)
		return
	}
	_, r := leaseX("drop", owner, token)
	switch r {
	case 0:
		fmt.Printf("임대 확인: refs/hc-lanes/%s 없음(놓음)\n", owner)
	case 3:
		fmt.Printf("임대 보존: refs/hc-lanes/%s 는 다른 실행의 것\n", owner)
	default:
		errf("hc-lane clean: 임대를 확인하지 못했다 — 같은 clean으로 다시 시도\n")
		return
	}
	if token == saved && xrun(os.Stdout, os.Stderr, "git", "-C", repo, "update-ref", "-d", "refs/hc-lease-owners/"+branch, saved) != 0 {
		errf("hc-lane clean: 이전 임대 기록 정리 실패\n")
	}
}
