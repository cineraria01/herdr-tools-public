package rest

import (
	"strconv"
	"strings"
	"time"
)

// 묻기: 판정은 그 터미널이 있는 기계에서만 할 수 있다(그 기계의 ~/.claude·임시 뿌리). 다른 도구·다른 기계는 그 기계의
// `hc-sweep --rest <주소>`가 찍는 한 줄 「REST<탭>상태<탭>세션 id<탭>까닭<탭>end」를 읽는다. 그 줄이 없거나 꼴이 틀리면(옛 판 hc-sweep은
// 모르는 인자로 종료 2·연결 안 됨·시간 넘김·잘린 출력) 모름 — 배포 사이 옛 판 워커는 늘 모름으로 읽힌다. 경고 줄이 섞여도 그 줄만 본다

// AskMax: 한 번 묻기의 상한(원격 hc exec --timeout). 부르는 쪽 상한은 이보다 30초 길게
const AskMax = 60 * time.Second

// Runner: argv를 돌려 표준 출력과 종료 코드(못 띄움·시간 넘김은 0이 아닌 값)
type Runner func(argv []string) (string, int)

// Line: hc-sweep --rest가 찍는 한 줄
func Line(v Verdict) string {
	flat := func(s string) string {
		if s = strings.Join(strings.Fields(s), " "); s == "" {
			return "-"
		}
		return s
	}
	return strings.Join([]string{"REST", v.State.String(), flat(v.Sid), flat(v.Sig), "end"}, "\t")
}

// Parse: hc-sweep --rest의 출력(종료 0일 때만)에서 마지막 REST 줄. 없거나 꼴이 틀리면 모름(Sig에 까닭)
func Parse(out string, rc int) Verdict {
	if rc != 0 {
		return Verdict{Sig: "묻기 실패(종료 " + strconv.Itoa(rc) + " — 옛 판·연결 안 됨·시간 넘김)"}
	}
	var line string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "REST\t") {
			line = strings.TrimSuffix(l, "\r")
		}
	}
	f := strings.Split(line, "\t")
	if len(f) != 5 || f[4] != "end" {
		return Verdict{Sig: "묻기 답 꼴이 틀림(REST 줄 없음·잘림)"}
	}
	undash := func(s string) string {
		if s == "-" {
			return ""
		}
		return s
	}
	v := Verdict{Sid: undash(f[2]), Sig: undash(f[3])}
	switch f[1] {
	case "quiet":
		v.State = Quiet
	case "busy":
		v.State = Busy
	case "unknown":
	default:
		return Verdict{Sig: "묻기 답 꼴이 틀림(상태 " + f[1] + ")"}
	}
	if v.State != Quiet && v.Sig == "" {
		v.Sig = "까닭 없음"
	}
	return v
}

// Ask: 주소 addr(다른 기계는 「기계/tN」)의 터미널이 정말 쉬는가를 그 기계의 hc-sweep에 묻는다. local은 이 기계 이름(접두가 이것이거나
// 없으면 이 기계에서 바로 hc-sweep --rest tN). 닫거나 손을 뗀 것으로 치기 직전, 그 터미널 하나에만 부른다(바퀴마다 다 묻지 않는다)
func Ask(run Runner, addr, local string) Verdict {
	on, term := "", addr
	if i := strings.LastIndexByte(addr, '/'); i >= 0 {
		on, term = addr[:i], addr[i+1:]
	}
	argv := []string{"hc-sweep", "--rest", term}
	if on != "" && on != local {
		argv = append([]string{"hc", "exec", "--on", on, "--timeout", strconv.Itoa(int(AskMax / time.Second)), "--"}, argv...)
	}
	return Parse(run(argv))
}

// Why: 조치하지 않은 까닭 한 줄(쉼이면 빈 값) — 「안에서 일이 도는 중(…)」·「쉼 모름(…)」
func Why(v Verdict) string {
	switch v.State {
	case Quiet:
		return ""
	case Busy:
		return "안에서 일이 도는 중(" + v.Sig + ")"
	default: // Unknown·모르는 값
		return "쉼 모름(" + v.Sig + ")"
	}
}
