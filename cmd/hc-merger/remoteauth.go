package main

// 원격 로그인 점검(ht-cordon-remote-auth, 2026-11-27, docs/hc-fill/cordon.md): 워커의 원격 저장소 자격(자격 도우미가 내주는 토큰)이 서버
// 쪽에서 폐기되자 연결·부하·저장소 읽기는 모두 좋아 그 기계에 레인이 계속 갔고 착지(git ls-remote)에서 막혔다. hc-fill이 --spec-idle에
// --remote-auth를 얹으면(좋은 기계는 몇 분에 한 번), --repo-read 폴더 가운데 처음 있는 폴더에서 착지와 같은 원격 읽기 한 번
// (`git -C <폴더> ls-remote --exit-code origin HEAD` — 쓰지 않음·받지 않음, 묻는 창 없게 GIT_TERMINAL_PROMPT=0, 글은 LC_ALL=C)을 하고
// 결과 그대로 「remote-auth <차례> <종료|timeout|none> <호스트> <오류 줄들>」 한 줄을 낸다. 거절인지 모름인지는 hc-fill이 가른다(한 곳).
// 토큰·자격 도우미 출력은 읽지 않는다(git 표준 오류만 — 호스트는 원격 주소에서 자격을 뺀 이름만).
// 같은 호출에 쓰기 자격 한 줄 「remote-write …」(ht-probe-cordon — push --dry-run, 원격에 아무것도 안 씀)

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"herdr-tools/internal/proc"
)

// remoteAuthMax: 원격 읽기 상한(시험이 줄인다). 보통 1초 안 — 넘으면 timeout(모름: 원격이 느린 것은 자격 거절이 아니다)
var remoteAuthMax = 8 * time.Second

// probeRef: 쓰기 자격 점검이 --dry-run으로만 미는 참조(원격에 생기지 않는다 — 2026-11-28 실제 확인 ls-remote 앞뒤 0개)
const probeRef = "refs/herdr/probe-write"

// remoteAuth: 처음 있는 폴더(차례는 1부터)에서 원격 읽기 한 번과 쓰기 자격 한 번(동시에 — 조각 시간을 늘리지 않음). 줄 둘
// 「remote-auth …」·「remote-write …」(ht-probe-cordon). 폴더가 하나도 없으면 「remote-auth 0 none - -」·「remote-write 0 none - -」
func remoteAuth(paths []string) []string {
	for i, p := range paths {
		if rest, ok := strings.CutPrefix(p, "~/"); ok {
			p = filepath.Join(os.Getenv("HOME"), rest)
		}
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			continue
		}
		host := "-"
		if u, err := exec.Command("git", "-C", p, "config", "--get", "remote.origin.url").Output(); err == nil {
			host = remoteHost(strings.TrimSpace(string(u)))
		}
		// 쓰기 자격: 착지와 같은 push 길(pushurl·자격 도우미)로 receive-pack까지 가되 --dry-run이라 아무것도 보내지 않는다. 공개 저장소도
		// push에는 자격을 내 위 읽기 점검의 한계(공개 저장소는 폐기된 토큰도 좋음)를 가린다. 훅은 안 돌린다(--no-verify — push 관문·LFS)
		w := make(chan string, 1)
		go func() {
			w <- gitProbe(p, "push", "--dry-run", "--no-verify", "--porcelain", "origin", "HEAD:"+probeRef)
		}()
		// shortcut: 공개 저장소는 읽기에 자격을 안 내(서버가 401을 안 줌) 토큰이 폐기돼도 좋음으로 보인다 — 비공개 원격이 첫 repo 줄이어야 가린다,
		// 공개 저장소를 첫 줄로 쓰는 설정이 생기면 비공개 원격 가운데서 고른다(쓰기 자격 줄은 공개 저장소에서도 거절을 본다)
		a := gitProbe(p, "ls-remote", "--exit-code", "origin", "HEAD")
		return []string{fmt.Sprintf("remote-auth %d %s", i+1, strings.Replace(a, " ", " "+host+" ", 1)),
			fmt.Sprintf("remote-write %d %s", i+1, strings.Replace(<-w, " ", " "+host+" ", 1))}
	}
	return []string{"remote-auth 0 none - -", "remote-write 0 none - -"}
}

// gitProbe: git -C <폴더> <인자…> 한 번(상한 remoteAuthMax, 묻는 창 없게) → 「<종료|timeout|-1> <표준 오류 줄들을 | 로>」(없으면 「-」)
func gitProbe(p string, args ...string) string {
	var eb bytes.Buffer
	c := exec.Command("git", append([]string{"-C", p}, args...)...)
	c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	c.Stderr = &eb
	err := proc.RunMax(c, remoteAuthMax)
	rc := "0"
	var ee *exec.ExitError
	switch {
	case errors.Is(err, proc.ErrTimeout):
		rc = "timeout"
	case errors.As(err, &ee):
		rc = fmt.Sprint(ee.ExitCode())
	case err != nil:
		rc = "-1"
	}
	var ls []string
	for _, l := range strings.Split(eb.String(), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			ls = append(ls, l)
		}
	}
	msg := bcut(600, flat(strings.Join(ls, " | ")))
	if msg == "" {
		msg = "-"
	}
	return rc + " " + msg
}

// remoteHost: 원격 주소의 호스트만(https://[자격@]호스트/… · ssh://[사용자@]호스트[:포트]/… · [사용자@]호스트:경로). 모르면 「-」
func remoteHost(u string) string {
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		return p.Hostname()
	}
	if strings.Contains(u, "://") { // file:// 등 호스트 없는 주소
		return "-"
	}
	if at := strings.LastIndex(u, "@"); at >= 0 {
		u = u[at+1:]
	}
	if h, _, ok := strings.Cut(u, ":"); ok && h != "" && !strings.ContainsAny(h, "/ ") {
		return h
	}
	return "-"
}
