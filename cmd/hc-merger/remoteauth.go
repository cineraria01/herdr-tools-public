package main

// 원격 로그인 점검(ht-cordon-remote-auth, 2026-11-27, docs/hc-fill/cordon.md): 워커의 원격 저장소 자격(자격 도우미가 내주는 토큰)이 서버
// 쪽에서 폐기되자 연결·부하·저장소 읽기는 모두 좋아 그 기계에 레인이 계속 갔고 착지(git ls-remote)에서 막혔다. hc-fill이 --spec-idle에
// --remote-auth를 얹으면(좋은 기계는 몇 분에 한 번), --repo-read 폴더 가운데 처음 있는 폴더에서 착지와 같은 원격 읽기 한 번
// (`git -C <폴더> ls-remote --exit-code origin HEAD` — 쓰지 않음·받지 않음, 묻는 창 없게 GIT_TERMINAL_PROMPT=0, 글은 LC_ALL=C)을 하고
// 결과 그대로 「remote-auth <차례> <종료|timeout|none> <호스트> <오류 줄들>」 한 줄을 낸다. 거절인지 모름인지는 hc-fill이 가른다(한 곳).
// 토큰·자격 도우미 출력은 읽지 않는다(git 표준 오류만 — 호스트는 원격 주소에서 자격을 뺀 이름만)

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

// remoteAuth: 처음 있는 폴더(차례는 1부터)에서 원격 읽기 한 번. 폴더가 하나도 없으면 「remote-auth 0 none - -」
func remoteAuth(paths []string) string {
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
		var eb bytes.Buffer
		// shortcut: 공개 저장소는 읽기에 자격을 안 내(서버가 401을 안 줌) 토큰이 폐기돼도 좋음으로 보인다 — 비공개 원격이 첫 repo 줄이어야 가린다,
		// 공개 저장소를 첫 줄로 쓰는 설정이 생기면 비공개 원격 가운데서 고른다
		c := exec.Command("git", "-C", p, "ls-remote", "--exit-code", "origin", "HEAD")
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
		return fmt.Sprintf("remote-auth %d %s %s %s", i+1, rc, host, msg)
	}
	return "remote-auth 0 none - -"
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
