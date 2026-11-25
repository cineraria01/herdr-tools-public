//go:build !windows

package main

// 범위 모드(2026-11-26 ht-verify-scope-on): HERDR_VERIFY_SCOPE > 손잡이 verify_scope > 기본 on. 못 읽으면 observe(전부 돈다 — 안전한 쪽).
// 손잡이 꼴은 internal/knob(저장 파일 knobs.tsv의 7칸 줄, 원격 참조 refs/herdr/knobs 커밋 메시지)이지만 여기서는 표준 라이브러리로
// 읽는다 — 검증 본문은 internal을 가져오지 않는다(main.go 머리말: 레인이 internal을 깨도 검증은 지어지게). 꼴은 scope_mode_test.go가
// internal/knob으로 만든 입력으로 맞춘다.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	knobName    = "verify_scope"
	knobDefault = "on" // internal/knob 등록 표의 기본과 같게
	knobRef     = "refs/herdr/knobs"
	knobGitMax  = 30 * time.Second // git 한 번의 상한(원격 조회)
)

func scopeModeOK(m string) bool { return m == "observe" || m == "on" || m == "off" }

// scopeMode: 저장 파일(이 기계 ~/.local/state/herdr-ops/knobs.tsv — 지휘 기계)에 살아 있는 verify_scope가 있으면 그 값,
// 없으면 이 클론 origin의 refs/herdr/knobs(원격 손잡이의 정본 — 지휘 기계 hc-fill이 손잡이 > 설정 파일로 푼 값을 올린다),
// 참조에 값이 없으면 이 기계의 손잡이 설정 파일, 그것도 없으면 기본 on
func scopeMode(root string, w io.Writer) string {
	if m := os.Getenv("HERDR_VERIFY_SCOPE"); m != "" {
		if !scopeModeOK(m) {
			fmt.Fprintf(w, "verify: HERDR_VERIFY_SCOPE=%q 는 observe·on·off가 아니라 observe로 본다\n", m)
			return "observe"
		}
		return m
	}
	now := time.Now()
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		b, err := os.ReadFile(filepath.Join(home, ".local/state/herdr-ops/knobs.tsv"))
		switch {
		case errors.Is(err, os.ErrNotExist):
		case err != nil:
			fmt.Fprintf(w, "알림 범위: 손잡이를 못 읽어 observe로 돈다(%v)\n", err)
			return "observe"
		default:
			v, err := knobFileValue(string(b), now)
			if err != nil {
				fmt.Fprintf(w, "알림 범위: 손잡이를 못 읽어 observe로 돈다(%v)\n", err)
				return "observe"
			}
			if v != "" {
				return v
			}
		}
	}
	v, ok := knobRemoteValue(root, now)
	if !ok {
		fmt.Fprintf(w, "알림 범위: 손잡이를 못 읽어 observe로 돈다(%s origin의 %s)\n", root, knobRef)
		return "observe"
	}
	if v == "" {
		return knobConfValue()
	}
	return v
}

// knobConfValue: 손잡이 설정 파일(개인 설정 폴더 knobs.tsv — internal/knob ConfName·ParseConf 꼴 「이름 값 # 까닭」)의 verify_scope.
// 원격 참조에는 지휘 기계가 설정 파일 값도 실으니 이 길은 참조가 없을 때만 먹는다. 없거나 범위 밖이면 기본
func knobConfValue() string {
	dir := os.Getenv("HERDR_TOOLS_CONFIG")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config/herdr-tools")
	}
	b, err := os.ReadFile(filepath.Join(dir, "knobs.tsv"))
	if err != nil {
		return knobDefault
	}
	for _, l := range strings.Split(string(b), "\n") {
		body, _, _ := strings.Cut(l, "#")
		if f := strings.Fields(body); len(f) == 2 && f[0] == knobName && scopeModeOK(f[1]) {
			return f[1]
		}
	}
	return knobDefault
}

// knobLive: 값이 범위 안이고 기한(초, "-" = 계속)이 안 지났는가
func knobLive(v, until string, now time.Time) bool {
	if !scopeModeOK(v) {
		return false
	}
	if until == "-" {
		return true
	}
	u, err := strconv.ParseInt(until, 10, 64)
	return err == nil && now.Unix() < u
}

// knobFileValue: 저장 파일에서 살아 있는 verify_scope 값(없으면 ""). 꼴이 틀린 줄이 하나라도 있으면 오류 — knob.Load와 같다
func knobFileValue(s string, now time.Time) (string, error) {
	v := ""
	for i, l := range strings.Split(s, "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Split(l, "\t")
		if len(f) != 7 || f[0] == "" || (f[6] != "0" && f[6] != "1") {
			return "", fmt.Errorf("손잡이 파일 %d줄 꼴이 틀렸다", i+1)
		}
		if f[0] == knobName && knobLive(f[1], f[5], now) {
			v = f[1]
		}
	}
	return v, nil
}

// knobRemoteValue: origin의 refs/herdr/knobs에서 살아 있는 verify_scope 값. 참조가 없거나 값이 없으면 ("", true), 못 읽으면 ok=false
func knobRemoteValue(root string, now time.Time) (string, bool) {
	out, rc := knobGit(root, "ls-remote", "--exit-code", "origin", knobRef)
	if rc == 2 && out == "" {
		return "", true // 참조 없음
	}
	sha, name, cut := strings.Cut(strings.TrimRight(out, "\n"), "\t")
	if rc != 0 || !cut || name != knobRef || strings.Contains(name, "\n") || len(sha) < 40 || strings.Trim(sha, "0123456789abcdef") != "" {
		return "", false
	}
	if _, rc := knobGit(root, "cat-file", "-e", sha+"^{commit}"); rc != 0 {
		if _, rc := knobGit(root, "fetch", "-q", "origin", "+"+knobRef+":"+knobRef); rc != 0 {
			return "", false
		}
	}
	body, rc := knobGit(root, "cat-file", "commit", sha)
	if rc != 0 {
		return "", false
	}
	_, msg, _ := strings.Cut(body, "\n\n")
	ls := strings.Split(strings.TrimRight(msg, "\n"), "\n")
	if ls[0] != "hc-knobs" {
		return "", true // 꼴 모름 — knob.ParseRemote처럼 빈 값(기본)
	}
	v := ""
	for _, l := range ls[1:] {
		if f := strings.Split(l, "\t"); len(f) == 3 && f[0] == knobName && knobLive(f[1], f[2], now) {
			v = f[1]
		}
	}
	return v, true
}

// knobGit: git -C root …의 표준 출력과 종료 코드(상한을 넘기거나 못 띄우면 -1)
func knobGit(root string, args ...string) (string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), knobGitMax)
	defer cancel()
	c := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	out, err := c.Output()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return string(out), 0
	case ctx.Err() != nil:
		return "", -1
	case errors.As(err, &ee):
		return string(out), ee.ExitCode()
	}
	return "", -1
}
