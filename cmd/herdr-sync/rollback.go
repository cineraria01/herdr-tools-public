package main

// 되돌리기 --at <커밋>(2026-11-24 deploy-log-rollback): 이미 올린 그 커밋의 릴리스(bin-<커밋 12자>) 바이너리를 기계마다 깐다.
// 범위는 도구 바이너리뿐이다 — 파일·나무·설정·지움은 건드리지 않는다(개인 저장소 쪽 지침·설정은 git으로 되돌린다).
//   - 올려 둔 판만: 받지 못하면 그 자리에서 옛 소스를 짓지 않는다(installSh: 소스 판 at:<커밋>이면 짓기 길을 막는다).
//     그 판 릴리스에 없는 도구(뒤에 더한 도구)는 건너뛴다. herdr-sync 자신도 건너뛴다(되돌린 옛 herdr-sync가 다음 배포를 맡지 않게).
//   - 기록 맞춤: 깐 sha와 소스 판 at:<커밋>을 그 기계 bin.tsv에 적는다(installSh rec) — 다음 보통 배포는 그 본체를 「마지막으로 깔아 준
//     판」으로 알아봐 「따로 바뀜」 충돌 없이 새 판으로 바꾼다. 따로 바뀐 본체는 지금처럼 덮지 않고 충돌(--take로 덮음).
//   - --check와 함께면 무엇이 바뀔지만(다름:바이너리) 보인다. 없는 판이면 거절하고 올린 판 목록을 보인다(종료 1).
// 받는 것은 gh release view·download(SHA256SUMS)뿐이다 — 공개 저장소에 아무것도 올리지 않는다.

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"herdr-tools/internal/toolcfg"
	"herdr-tools/internal/tri"
)

var atRef string // --at 값

var hex12 = regexp.MustCompile(`^[0-9a-f]{12,40}$`)

// relSlug: publish.repo의 GitHub 「주인/이름」(없으면 "", 까닭)
func relSlug() (string, string) {
	pub, err := toolcfg.Lookup(CONF, "publish.repo", "", os.Stderr)
	if err != nil {
		return "", "개인 설정을 읽지 못했다 (" + CONF + "/config)"
	}
	if s := githubSlug(pub); s != "" {
		return s, ""
	}
	return "", "설정 publish.repo가 GitHub 저장소가 아니다"
}

// fetchRelease: 올려 둔 bin-<c12> 릴리스의 받을 곳(주소·sha256·리눅스 자산). 있음 · 없음(gh가 「release not found」) · 모름
func fetchRelease(slug, c12, src string) (*relInfo, tri.State, string) {
	tag := "bin-" + c12
	if _, rc, errs := gh("release", "view", tag, "--repo", slug, "--json", "assets", "--jq", ".assets[].name"); rc != 0 {
		if strings.TrimSpace(errs) == "release not found" {
			return nil, tri.No, ""
		}
		return nil, tri.Unknown, "gh release view 실패(인증·통신·시간 넘김)"
	}
	d := TMP + "/relat"
	if err := os.MkdirAll(d, 0o700); err != nil {
		return nil, tri.Unknown, "임시 폴더"
	}
	if _, rc, _ := gh("release", "download", tag, "--repo", slug, "--pattern", "SHA256SUMS", "--dir", d, "--clobber"); rc != 0 {
		return nil, tri.Unknown, "gh release download SHA256SUMS 실패"
	}
	b, err := os.ReadFile(d + "/SHA256SUMS")
	if err != nil {
		return nil, tri.Unknown, "받은 SHA256SUMS를 읽지 못했다"
	}
	rel := &relInfo{c12: c12, src: src, tag: tag, sums: map[string]string{}, lx: map[string]bool{}, at: true,
		base: envOr("HERDR_SYNC_RELEASE_URL", "https://github.com/"+slug+"/releases/download")}
	for _, l := range splitLines(string(b)) {
		f := strings.Fields(l)
		if len(f) != 2 {
			return nil, tri.Unknown, "SHA256SUMS 줄이 깨졌다"
		}
		rel.sums[f[1]] = f[0]
		if t, ok := strings.CutSuffix(f[1], "-linux-amd64"); ok {
			rel.lx[t] = true
		}
	}
	return rel, tri.Yes, ""
}

// releaseList: 올린 판(bin-…) 목록 — 거절할 때 보인다
func releaseList(slug string) string {
	out, rc, _ := gh("release", "list", "--repo", slug, "--limit", "30", "--json", "tagName", "--jq", ".[].tagName")
	if rc != 0 {
		return "(목록을 읽지 못했다 — gh release list)"
	}
	var r []string
	for _, l := range splitLines(out) {
		if c, ok := strings.CutPrefix(l, "bin-"); ok {
			r = append(r, c)
		}
	}
	if len(r) == 0 {
		return "(올린 판 없음)"
	}
	sort.Strings(r)
	return strings.Join(r, " ")
}

// atTools: 되돌릴 때 그 기계에 깔 도구 — 그 판 릴리스에 자산이 있는 것만, herdr-sync 자신은 빼고(건너뜀을 알린다)
func atTools(m, osname string, rel *relInfo, tools []string) []string {
	var r []string
	for _, t := range tools {
		switch {
		case t == "herdr-sync":
			say("건너뜀 %s %s (되돌리기는 herdr-sync 자신을 바꾸지 않는다)", m, t)
		case osname == "Linux" && !rel.lx[t], osname != "Linux" && rel.sums[t] == "":
			say("건너뜀 %s %s (그 판 릴리스에 없음)", m, t)
		default:
			r = append(r, t)
		}
	}
	return r
}

// rollback: --at <커밋> 본체 — 맥 다음 기계마다 바이너리만. 표는 보통 배포와 같은 꼴(맥 줄 먼저)
func rollback(machines []string) {
	if len(pulls) > 0 {
		die("--at은 --pull과 함께 쓰지 않는다")
	}
	c12 := ""
	if full, rc := gitq("-C", REPO, "rev-parse", "-q", "--verify", atRef+"^{commit}"); rc == 0 && len(full) >= 12 {
		c12 = full[:12]
	} else if hex12.MatchString(atRef) {
		c12 = atRef[:12] // 범용 저장소에 없는 커밋이어도 올린 판이면 받는다
	}
	if c12 != "" {
		deployC12, deploySrc = c12, "at:"+c12 // 거절로 끝나도 기록에 그 커밋으로
	}
	slug, why := relSlug()
	if slug == "" {
		die("되돌리기: %s — 받을 릴리스를 모른다", why)
	}
	if c12 == "" {
		die("되돌리기: 커밋을 모른다: %s — 올린 판: %s", atRef, releaseList(slug))
	}
	rel, st, why := fetchRelease(slug, c12, "at:"+c12)
	switch st {
	case tri.No:
		die("되돌리기: 그 판의 릴리스가 없다 (bin-%s) — 올린 판만 깐다. 올린 판: %s", c12, releaseList(slug))
	case tri.Unknown:
		die("되돌리기: 릴리스를 읽지 못했다 bin-%s (%s) — 모름, 아무것도 깔지 않음", c12, why)
	case tri.Yes:
	}
	deployC12, deploySrc, deployReached = c12, rel.src, true
	say("되돌리기 bin-%s (바이너리만)", c12)
	mbd, _ := installBins("맥", "Darwin", rel, &rconf)
	rdiff = append(rdiff, mbd...)
	for _, m := range machines {
		q, ok := query(m)
		switch {
		case !ok && q.nosh:
			rows += m + ": 건너뜀 (sh 없음 — Windows 등)\n"
			result(m, "건너뜀")
			continue
		case !ok:
			warn("%s 연결 실패", m)
			row(m, []string{"연결"}, 0)
			continue
		case q.O != "Darwin" && q.O != "Linux":
			rows += m + ": 건너뜀 (OS " + orUnknown(q.O) + ")\n"
			result(m, "건너뜀")
			continue
		}
		conf := 0
		bd, _ := installBins(m, q.O, rel, &conf)
		row(m, bd, conf)
	}
	saved := rows
	rows = ""
	row("맥", rdiff, rconf)
	fmt.Print(rows + saved)
}
