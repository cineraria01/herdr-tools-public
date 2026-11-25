package main

// tell(2026-11-26 ht-fill-tell): 레인에 글 보내기. 지휘가 「hc ls | 이름 찾기 | hc send」로 같은 이름의 앞 run(끝난 터미널)에 보내
// 한 바퀴 늦었다 — 대기열 줄이 기록한 지금 run 터미널(laneTerm) 하나에만 보낸다. --audit이면 그 레인의 지금 감사 줄 터미널로

import "herdr-tools/internal/hcjson"

func init() { register("tell", tellCmd) }

// tell [--audit] <레인> <글>. 0 보냄 · 1 안 보냄(까닭)·모름·보내기 실패 · 2 잘못 부름
func tellCmd(args []string) {
	audit := false
	var rest []string
	for _, a := range args {
		if a == "--audit" {
			audit = true
		} else {
			rest = append(rest, a)
		}
	}
	if len(rest) != 2 || rest[0] == "" || rest[0][0] == '-' || rest[1] == "" {
		usage()
	}
	name, msg := rest[0], rest[1]
	var r Row
	if audit {
		for _, q := range qRowsW() {
			if q.F(qAudit) == "audit="+name && q.F(qState) == "up" {
				r = q
			}
		}
		if r == nil {
			die("%s: 지금 도는 감사가 없다 — 보내지 않음", name)
		}
	} else if r = qrowW(name); r == nil {
		noLane(name)
	}
	t, found, err := laneTerm(r)
	to, addr := r.F(qName), r.F(qAddr)
	if b := hcjson.SendBlocked(t, found, err); b != "" {
		die("%s: 보내지 않음 — 받는 쪽 %s", to, b)
	}
	if !auSendTo(addr, msg, t, found, err) {
		die("%s: hc send %s 실패 — 보냈는지 모른다", to, addr)
	}
	say("보냄 %s → %s", to, addr)
}
