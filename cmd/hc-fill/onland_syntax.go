package main

// 착지 뒤 명령 구문 검사(2026-11-24 지휘 지시 — --why 값에 괄호를 넣은 on-land 명령의 첫 실행이 sh 구문 오류(종료 2)로 끝났다).
// 설정 on-land 줄의 명령을 돌리는 셸(sh -c)과 같은 sh로 -n(구문만, 돌리지 않음) 검사한다. 틀린 줄은 status 끝과 바퀴의 운영 사건
// (설정 줄마다 키 하나 — 같은 줄이면 한 번)으로 알린다

import (
	"strconv"
	"strings"
)

// onlandSyntax: 구문이 틀린 on-land 줄마다 「on-land <이름> 명령 구문 오류: <sh의 글>」과 그 설정 줄의 사건 키
func onlandSyntax() (msgs, keys, names []string) {
	for _, l := range confLines {
		f := fields(l)
		if len(f) < 4 || f[0] != "on-land" {
			continue
		}
		out, rc := Cmd{Name: "sh", Args: []string{"-n", "-c", restAfter(l, 3)}, Err: errMerge}.Run()
		if rc == 0 {
			continue
		}
		why := strings.TrimSpace(flat1(sub(out)))
		if why == "" {
			why = "sh -n 종료 " + strconv.Itoa(rc)
		}
		msgs = append(msgs, "on-land "+f[1]+" 명령 구문 오류: "+why)
		keys = append(keys, "hf:onland-syntax:"+f[1]+":"+strconv.FormatUint(uint64(cksum([]byte(l))), 10))
		names = append(names, f[1])
	}
	return msgs, keys, names
}

// onlandSyntaxTick: 바퀴마다 — 틀린 줄을 로그에, 운영 사건은 그 설정 줄에 한 번(hc-lane report --key가 같은 키를 다시 안 적는다)
func onlandSyntaxTick() {
	msgs, keys, names := onlandSyntax()
	for i, m := range msgs {
		say("%s", m)
		if !dry && hcLaneReport("막힘", m+" — 설정 줄을 고친다(명령은 돌리지 않았다)", "on-land-"+names[i], "", keys[i]) != 0 {
			say("착지 뒤 %s: 구문 오류 사건을 못 적음 — 다음 바퀴에", names[i])
		}
	}
}
