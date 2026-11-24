package main

// 2차 이상 감사가 앞 차수의 지적과 지휘 지시를 알고 시작하게 한다(2026-11-24 prev-findings-recheck — 2차 감사가 앞 판정을 모른 채
// 전체 diff를 처음부터 다시 봤다). 감사 틀 자리 {round} {prev_sha} {prev_report} {prev_verdict} {coord_notes}와 hc-fill note.

import (
	"fmt"
	"os"
	"strings"
)

// coordNotes <레인>: 지휘 지시 파일 — 레인 기계에서 판정 파일 옆.
// ponytail: 레인 정리가 지우지 않는다 — 같은 이름을 다시 쓰면 옛 지시도 보인다(글마다 시각 머리 줄). 잦아지면 clean에서 함께 지운다
func coordNotes(lane string) string {
	return "~/.local/share/hc-fill/verdicts/" + lane + "/coord-notes.md"
}

// hmlText: 감사 기록 16칸의 hml=<높음>.<중간>.<낮음> → 「높음 n·중간 n·낮음 n」(없으면 빈 값)
func hmlText(afl string) string {
	for _, f := range strings.Split(afl, ",") {
		if v, ok := strings.CutPrefix(f, "hml="); ok {
			if p := strings.Split(v, "."); len(p) == 3 && isDigits(p[0]) && isDigits(p[1]) && isDigits(p[2]) {
				return "높음 " + p[0] + "·중간 " + p[1] + "·낮음 " + p[2]
			}
		}
	}
	return ""
}

// aprev <aid>: 그 차수의 몇 차와, 같은 레인·같은 부모 실행에서 그보다 앞서 판정을 받은 마지막 차수(판정 없이 superseded 된 차수는 건너뜀)의
// sha·보고 경로·판정 수. 1차거나 앞 차수가 없으면 앞 값은 빈 값
func aprev(aid string) (rnd, sha, rep, hml string) {
	rows := readRows(AT)
	var cur Row
	for _, r := range rows {
		if r.NF() >= 17 && r.F(1) == aid {
			cur = r
		}
	}
	if cur == nil {
		return
	}
	rnd = cur.F(3)
	for _, r := range rows {
		if r.F(1) == aid {
			break
		}
		if r.NF() >= 17 && r.F(2) == cur.F(2) && r.F(4) == cur.F(4) {
			if h := hmlText(r.F(16)); h != "" {
				sha, rep, hml = r.F(5), "~/.local/share/hc-fill/reports/audit-"+r.F(1)+".md", h
			}
		}
	}
	return
}

// aprevFill: 틀 t의 앞 차수 자리를 채운다. {coord_notes}는 레인 기계에 그 파일이 있을 때만 경로(못 보면 거짓 — 다음 바퀴에)
func aprevFill(t, aid, lane, m string) (string, bool) {
	rnd, sha, rep, hml := aprev(aid)
	t = strings.ReplaceAll(t, "{round}", rnd)
	t = strings.ReplaceAll(t, "{prev_sha}", sha)
	t = strings.ReplaceAll(t, "{prev_report}", rep)
	t = strings.ReplaceAll(t, "{prev_verdict}", hml)
	if !strings.Contains(t, "{coord_notes}") {
		return t, true
	}
	o, rc := onm(nil, m, `if [ -f "$d" ]; then echo Y; else echo N; fi`, coordNotes(lane))
	switch sub(o) {
	case "Y":
		return strings.ReplaceAll(t, "{coord_notes}", coordNotes(lane)), rc == 0
	case "N":
		return strings.ReplaceAll(t, "{coord_notes}", ""), rc == 0
	}
	return t, false
}

func init() { register("note", noteCmd) }

// hc-fill note <레인> <글|파일>: 지휘 지시를 레인 기계의 coord-notes.md에 덧붙이고(다음 감사가 {coord_notes}로 본다) 레인에도 hc send
func noteCmd(args []string) {
	if len(args) != 2 {
		usage()
	}
	lane, text := args[0], args[1]
	if isFile(text) {
		b, err := os.ReadFile(text)
		if err != nil {
			die("지시 파일을 못 읽음: %v", err)
		}
		text = string(b)
	}
	if text = strings.TrimSpace(text); text == "" {
		die("지시가 비었다")
	}
	r := qrow(lane)
	if r == nil {
		noLane(lane)
	}
	m, addr := r.F(7), r.F(8)
	if r.F(6) != "up" || m == "" || m == "-" || addr == "" || addr == "-" {
		die("떠 있지 않은 레인이다(상태 %s): %s", r.F(6), lane)
	}
	nf := coordNotes(lane)
	in := "## " + utcnow() + " 지휘\n" + text + "\n\n"
	if _, rc := onm(strings.NewReader(in), m, `mkdir -p "${d%/*}" && cat >>"$d"`, nf); rc != 0 {
		die("%s 에 지휘 지시를 쓰지 못함: %s", m, nf)
	}
	say("지휘 지시 남김 %s @%s %s", lane, m, nf)
	if !auSend(addr, "hc-fill 지휘 지시(다음 감사도 "+nf+" 로 본다): "+text) {
		fmt.Fprintf(os.Stderr, "hc-fill: 레인 %s 에 보내지 못함 — 파일에는 남았다\n", addr)
		exit(1)
	}
	say("레인에 보냄 %s", addr)
}
