package main

// 워커 연결(worker-mac-setup mesh, 2026-11-17 사용자 「워커 맥끼리 모두 연결 (추천)」) — help.txt 「워커 연결」.
// 원격 조각(mesh.sh)은 bash로 보내고, 기계 정보 모으기·지문 대조·방향 판정·개수는 여기.

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"regexp"
	"strings"

	wm "herdr-tools/internal/workermac"
)

//go:embed mesh.sh
var meshSrc string

// piece: mesh.sh의 「#@ name」 조각
func piece(name string) string {
	var b strings.Builder
	on := false
	for _, l := range strings.SplitAfter(meshSrc, "\n") {
		if strings.HasPrefix(l, "#@ ") {
			on = strings.TrimSpace(l[3:]) == name
			continue
		}
		if on {
			b.WriteString(l)
		}
	}
	return b.String()
}

var rrunSec = 90 // 원격 조각 한 번의 상한 초(시험이 줄인다)

// rrun: 기계 m에서 「<앞붙임>」 + head + 조각을 CHECK·날짜 인자로(rrunSec 상한). 표준 출력과 종료 코드, 표준 오류는 화면으로
func rrun(ctx context.Context, m, pre, name, check, d string) (string, int) {
	return wm.Output(ctx, rrunSec, os.Stderr, "ssh", ssh(m, remoteCmd(pre+piece("head")+piece(name), check, d))...)
}

type node struct {
	w, user, v4, v6, pub, fp string
	ok                       bool
}

var badRe = regexp.MustCompile(`(없음|다름|실패|안 됨)`)

func b01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// macFP: 이 맥 known_hosts에 적힌 그 주소의 ed25519 지문(첫 줄). 없거나 못 읽으면 빈 값 — 대조할 수 없어 부른 쪽이 멈춘다
func macFP(ctx context.Context, v4 string) string {
	kh, _ := wm.Output(ctx, 15, nil, "ssh-keygen", "-F", v4, "-f", os.Getenv("HOME")+"/.ssh/known_hosts")
	var keep []string
	for _, l := range strings.Split(kh, "\n") {
		f := strings.Fields(l)
		if !strings.HasPrefix(strings.TrimLeft(l, " \t\r\v\f"), "#") && len(f) >= 2 && f[1] == "ssh-ed25519" {
			keep = append(keep, l)
		}
	}
	if len(keep) == 0 {
		return ""
	}
	var out strings.Builder
	c := wm.Command(ctx, "ssh-keygen", "-lf", "-")
	c.Stdin, c.Stdout = strings.NewReader(strings.Join(keep, "\n")+"\n"), &out
	if wm.Run(ctx, c) != 0 {
		return ""
	}
	first, _, _ := strings.Cut(out.String(), "\n")
	if f := strings.Fields(first); len(f) >= 2 {
		return f[1]
	}
	return ""
}

// meshRun: worker.mesh 기계끼리 모든 방향(f가 있으면 f와 얽힌 방향만). check면 읽기만. 0 모두 됨 · 1 문제 · 4 지문 대조 실패 · 64 f가 목록 밖
func meshRun(ctx context.Context, mesh, f string, check bool, d string) int {
	ws := strings.Fields(mesh)
	if f != "" && !inList(ws, f) {
		fmt.Fprintf(os.Stderr, "✗ %s 은(는) worker.mesh(%s)에 없다\n", f, strings.Join(ws, " "))
		return 64
	}
	head := ""
	if check {
		head = " (--check: 읽기만)"
	}
	if f != "" {
		say("워커 연결%s: %s — %s 와 얽힌 방향만", head, strings.Join(ws, " "), f)
	} else {
		say("워커 연결%s: %s", head, strings.Join(ws, " "))
	}
	ns := make([]node, len(ws))
	bad, add, n := 0, 0, 0
	// 1) 기계마다 정보(읽기만)와 이 맥 known_hosts의 ed25519 지문. 지문을 못 대조하면 아무것도 바꾸기 전에 멈춘다
	for i, w := range ws {
		ns[i].w = w
		out, rc := rrun(ctx, w, "NAME="+wm.Quote(w)+";", "info", "1", d)
		if rc != 0 {
			fmt.Printf("✗ %s: 접속 안 됨\n", w)
			bad++
			continue
		}
		ns[i].user, ns[i].v4, ns[i].v6, ns[i].pub = wm.Field(out, "user="), wm.Field(out, "ts4="), wm.Field(out, "ts6="), wm.Field(out, "pub=")
		if ns[i].v4 == "" {
			fmt.Printf("✗ %s: Tailscale 주소를 못 읽음\n", w)
			bad++
			continue
		}
		if ns[i].fp = macFP(ctx, ns[i].v4); ns[i].fp == "" {
			fmt.Printf("✗ %s: 이 맥 known_hosts에 %s ed25519가 없어 지문을 대조할 수 없다 — 멈춤 (ssh %s 로 한 번 붙어 맥 known_hosts에 넣은 뒤 다시)\n", w, ns[i].v4, w)
			return 4
		}
		ns[i].ok = true
	}
	// 키가 없는 기계는 여기서 만든다(--check가 아닐 때)
	for i := range ns {
		if check || !ns[i].ok || ns[i].pub != "" {
			continue
		}
		out, rc := rrun(ctx, ns[i].w, "NAME="+wm.Quote(ns[i].w)+";", "info", "0", d)
		if rc == 0 {
			ns[i].pub = wm.Field(out, "pub=")
		}
		if strings.Contains("\n"+out, "\nnew=1") {
			say("%s: 키 만듦 ~/.ssh/id_ed25519 (%s-workers-%s)", ns[i].w, ns[i].w, d)
		}
	}
	// 2) 방향마다: 받는 쪽 authorized_keys → 보내는 쪽 known_hosts·config·hc·접속
	for i, a := range ns {
		for j, b := range ns {
			if i == j || (f != "" && a.w != f && b.w != f) {
				continue
			}
			n++
			if !a.ok || !b.ok {
				fmt.Printf("✗ %s → %s: 위 기계 오류로 건너뜀\n", a.w, b.w)
				bad++
				continue
			}
			from := a.v4
			if a.v6 != "" {
				from += "," + a.v6
			}
			r := "실패"
			if o, rc := rrun(ctx, b.w, "PUB="+wm.Quote(a.pub)+" FROM="+wm.Quote(from)+";", "auth", b01(check), d); rc == 0 {
				r = wm.Trim(o)
			}
			out, rc := rrun(ctx, a.w, "B="+wm.Quote(b.w)+" H4="+wm.Quote(b.v4)+" BU="+wm.Quote(b.user)+" FP="+wm.Quote(b.fp)+";", "cli", b01(check), d)
			line := fmt.Sprintf("%s → %s: authorized_keys %s · known_hosts %s · config %s · hc %s · 접속 %s", a.w, b.w, r,
				wm.Field(out, "kh="), wm.Field(out, "cfg="), wm.Field(out, "hc="), wm.Field(out, "conn="))
			if rc == 4 {
				fmt.Printf("✗ %s\n✗ %s 가 본 %s 호스트 키 지문이 이 맥 known_hosts(%s)와 다르다 — 멈춤. 넣지 않았다.\n", line, a.w, b.w, b.fp)
				return 4
			}
			// 셸 판은 authorized_keys 「넣음」을 두 번 셌다(「$line $r」) — 여기는 한 번
			add += strings.Count(line, "넣음")
			if rc == 0 && r != "실패" && !badRe.MatchString(line) && strings.Contains(line, "접속 됨") {
				fmt.Printf("  %s\n", line)
			} else {
				fmt.Printf("✗ %s\n", line)
				bad++
			}
		}
	}
	switch {
	case bad == 0 && add == 0:
		say("워커 연결: 모두 있음 (%d방향)", n)
	case bad == 0:
		say("워커 연결: 끝 (%d방향, 새로 넣은 것 %d)", n, add)
	default:
		say("워커 연결: 문제 %d (%d방향 중, ✗ 줄)", bad, n)
		return 1
	}
	return 0
}
