package main

// clean의 레인 주인·임대(셸 판 lib/hc-lane/clean.sh의 clean_owner·clean_lease와 같은 판정)

import (
	"fmt"
	"os"
	"strings"
)

// legacyParked: 대상 기계(--on이면 그 기계, 아니면 이 기계) 사건(events.tsv)에서 그 레인의 마지막 주차·제출 사건이 그 커밋(sha,
// 7자 이상 앞부분도)의 「주차」인가 — 옛 판 주차(원격 lane/<레인>)를 반송된 제출 가지와 가른다(gofix L07). 주차 사건은 레인이 돈
// 기계에 적히니 --on이면 그 기계 것을 읽는다(재감사 R-24). 못 읽으면 false(멈춤 쪽)
func legacyParked(lane, sha string) bool {
	var ls []string
	if on != "" {
		out, r := xoutQ("sh", "-c", evrd, "hc-lane", "0")
		if r != 0 {
			return false
		}
		ls = lines(out)
	} else {
		var err error
		if ls, err = readLines(evdir() + "/events.tsv"); err != nil {
			return false
		}
	}
	return lastParkIs(ls, lane, sha)
}

// lastParkIs: 사건 줄들에서 그 레인의 마지막 주차·제출 사건이 그 커밋의 「주차」인가
func lastParkIs(ls []string, lane, sha string) bool {
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

// keepOffSelf: 자기 레인(HC_LANE이 이 가지·이름)을 정리했으면 자기 터미널(HC_TERMINAL_ID)의 hc keep을 끈다. Herdr 창에 붙인
// 터미널은 keep이 켜져 에이전트가 끝나면 셸로 남아 그 기계 hc-fill 자리를 붙잡았다(2026-11-21 mini-m4 land-claim-net 13시간).
// 다른 레인 정리(hc-sweep·gc)·dry-run은 건드리지 않는다. 실패는 알리기만 한다(정리는 이미 끝남)
func keepOffSelf() {
	self, ln := os.Getenv("HC_TERMINAL_ID"), os.Getenv("HC_LANE")
	if dry || self == "" || ln == "" || (ln != branch && ln != name) {
		return
	}
	if _, r := hcRun(30, errDrop, "keep", self, "--off"); r != 0 {
		errf("hc-lane clean: 자기 터미널 %s keep 끄기 실패(종료 %d) — hc-sweep이 착지 증거로 정리한다\n", self, r)
	}
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
