// repoid.go: 레인의 저장소를 기계와 무관하게 가리키는 식별자 — origin 주소를 정규화한 「호스트/경로」(watch-project-route, 2026-11-24).
// 사건 13칸과 지휘 저장소 기록(coordrepo.<주소>)이 이것으로 견준다. 클론 경로는 기계마다 달라 쓰지 않는다
package main

import (
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// repoID: origin 주소 → 「호스트/경로」(소문자, 사용자·포트·끝 .git·/ 뗌). scp 꼴(git@h:o/r.git)·ssh://·https:// 모두 같은 값.
// 로컬 경로·file:·빈 값·이상한 글자·200바이트 넘음은 ""(모름 — 받는 지휘를 좁히지 않는다)
func repoID(u string) string {
	u = strings.TrimSpace(u)
	host, path := "", ""
	if strings.Contains(u, "://") {
		p, err := url.Parse(u)
		if err != nil || p.Scheme == "file" {
			return ""
		}
		host, path = p.Hostname(), p.Path
	} else if h, p, ok := strings.Cut(u, ":"); ok && !strings.Contains(h, "/") && len(h) > 1 { // 한 글자는 드라이브 문자(C:/…)
		if i := strings.LastIndex(h, "@"); i >= 0 {
			h = h[i+1:]
		}
		host, path = h, p
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	id := strings.ToLower(host + "/" + strings.Trim(path, "/"))
	if host == "" || path == "" || !repoIDok(id) {
		return ""
	}
	return id
}

// repoIDok: 사건 칸·파일 줄에 실을 수 있는 식별자 꼴인가(옛 판·손으로 쓴 줄의 13칸도 이것으로 거른다)
func repoIDok(s string) bool {
	return s != "" && s != "-" && len(s) <= 200 && strings.Contains(s, "/") && onlyChars(s, "abcdefghijklmnopqrstuvwxyz0123456789._~+/-")
}

// repoHere: 지금 폴더 저장소의 식별자 — insteadOf를 풀지 않은 origin 원문(git config, 기계마다 다른 insteadOf가 끼지 않게). 실패·없음은 ""
func repoHere() string {
	u, rc := capt(10, errDrop, "git", "config", "--get", "remote.origin.url")
	if rc != 0 {
		return ""
	}
	return repoID(u)
}

// repoGrace: 남의 저장소 주인 없는 사건을 미루는 유예(repoSend) — HC_LANE_WATCH_REPO_GRACE → lane.watch_repo_grace(초), 기본 바퀴 간격의
// 3배(그 저장소 지휘 watch가 두세 바퀴 안에 받지 못한 사건은 이 watch가 보낸다)
func repoGrace(tick int64) time.Duration {
	g := os.Getenv("HC_LANE_WATCH_REPO_GRACE")
	if g == "" {
		g = cfg("lane.watch_repo_grace", strconv.FormatInt(3*tick, 10))
	}
	if !digits(g) {
		die("유예(lane.watch_repo_grace·HC_LANE_WATCH_REPO_GRACE)는 0 이상 정수(초): %s", g)
	}
	return time.Duration(atoi(g)) * time.Second
}

// reportRepo: report가 13칸에 실을 저장소 — HC_LANE_REPO_ID(제출처럼 남의 이름으로 적되 그 저장소 안에서 부르는 길이 준다), 아니면
// --name 없이 부를 때만 지금 폴더의 저장소(--name은 hc-fill·hc-sweep처럼 다른 폴더에서 남의 레인을 적는 길이라 모름)
func reportRepo(named bool) string {
	if v, ok := os.LookupEnv("HC_LANE_REPO_ID"); ok {
		if repoIDok(v) {
			return v
		}
		return ""
	}
	if named {
		return ""
	}
	return repoHere()
}
