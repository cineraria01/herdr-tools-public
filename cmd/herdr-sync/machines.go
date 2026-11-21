package main

// 기계마다 맞추기(파일·나무·링크·base·설정·지움), 맥 지움, 표, 공개 내보내기

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"herdr-tools/internal/cksum"
	"herdr-tools/internal/ojson"
	"herdr-tools/internal/toolcfg"
)

var (
	sinCache string
	sinDone  bool
	machOS   = map[string]string{} // 맞춘 기계의 uname(릴리스 받기에 쓴다)
)

func path(k string) []string { return strings.Split(strings.TrimPrefix(k, "."), ".") }

// sin: herdr-settings-merge의 표준 입력. 기계마다 같다(예외 키·$HOME 바꿔 넣기는 herdr-settings-merge가 한다)
func sin() (string, bool) {
	if sinDone {
		return sinCache, sinCache != ""
	}
	sinDone = true
	keys, err := readJSONFile(KEYS)
	if err != nil {
		return "", false
	}
	sv, serr := readJSONFile(home + "/.claude/settings.json")
	refs := ojson.NewObject()
	for _, k := range strList(objGet(keys, "refs")) {
		c := ""
		if serr == nil {
			if v, err := ojson.GetPath(sv, path(k)); err == nil {
				c = ojson.Canon(v)
			}
		}
		refs.Set(k, cksum.Space([]byte(c+"\n")))
	}
	if serr == nil {
		if _, ok := sv.(*ojson.Object); !ok {
			serr = ojson.ErrIndex
		}
	}
	if serr != nil {
		return "", false
	}
	values := ojson.NewObject()
	for _, p := range strList(objGet(keys, "values")) {
		v, err := ojson.GetPath(sv, path(p))
		if err != nil {
			return "", false
		}
		values.Set(p, v)
	}
	hooks := []any{}
	if kh, ok := objGet(keys, "hooks").(*ojson.Object); ok {
		for _, ev := range kh.Keys {
			marks := strList(kh.M[ev])
			entries, _ := objGet(objGet(sv, "hooks"), ev).([]any)
			for _, e := range entries {
				eo, ok := e.(*ojson.Object)
				if !ok {
					continue
				}
				items, _ := eo.M["hooks"].([]any)
				for _, h := range items {
					ho, ok := h.(*ojson.Object)
					if !ok {
						return "", false
					}
					c, isStr := ho.M["command"].(string)
					hit := false
					for _, mk := range marks {
						if !isStr {
							return "", false
						}
						if strings.Contains(c, mk) {
							hit = true
						}
					}
					if !hit {
						continue
					}
					o := ojson.NewObject()
					o.Set("event", ev)
					mt := eo.M["matcher"]
					if mt == nil || mt == false {
						mt = ""
					}
					o.Set("matcher", mt)
					for _, f := range ho.Keys {
						o.Set(f, ho.M[f])
					}
					hooks = append(hooks, o)
				}
			}
		}
	}
	in := ojson.NewObject()
	in.Set("values", values)
	in.Set("hooks", hooks)
	in.Set("refs", refs)
	in.Set("home", home)
	sinCache = ojson.Compact(in)
	return sinCache, true
}

// goRunWarn: 공통 실행기의 「Go 판을 쓸 수 없다」 경고 줄(그 기계에 go가 없음) — 도구 문제 판정에서 뺀다
func goRunWarn(l string) bool {
	return strings.Contains(l, "경고: Go 판을 쓸 수 없다") || strings.Contains(l, "고치기: 그 기계에 go를")
}

// settings: 대상 기계의 herdr-settings-merge에 넘기고 판정 줄을 옮긴다. 충돌 수를 더하고 다름 묶음(설정·도구문제:설정)을 돌려준다
func settings(m string, conf *int) string {
	if !isFile(KEYS) {
		return ""
	}
	in, ok := sin()
	if !ok {
		warn("설정 입력을 만들지 못했다 (%s·~/.claude/settings.json)", KEYS)
		return "도구문제:설정"
	}
	var opts []string
	if check {
		opts = append(opts, "--check")
	}
	for _, s := range takes {
		if s == m+":*" {
			opts = append(opts, "--take", "*")
		} else if strings.HasPrefix(s, m+":") {
			if k := s[len(m)+1:]; !unitKey(k) && !treespec(k) {
				opts = append(opts, "--take", k)
			}
		}
	}
	args := append([]string{"exec", "-i", "--on", m, "--", "env", "HERDR_MACHINE=" + m,
		"sh", "-c", `x=$HOME/.local/bin/herdr-settings-merge; [ -x "$x" ] || exit 127; exec "$x" "$@"`, "_"}, opts...)
	c := exec.Command("hc", args...)
	var serr bytes.Buffer
	c.Stdin, c.Stderr = strings.NewReader(in+"\n"), &serr
	out, r := output(c)
	c0, wrote, sdiff := *conf, false, ""
	for _, l := range splitLines(out) {
		l = strings.TrimLeft(l, " \t")
		s, rest := l, ""
		if i := strings.IndexAny(l, " \t"); i >= 0 {
			s, rest = l[:i], strings.TrimLeft(l[i:], " \t")
		}
		switch s {
		case "충돌":
			say("충돌 %s %s", m, rest)
			*conf++
		case "씀":
			if check {
				say("%s 다름 %s", m, rest)
			} else {
				say("%s 씀 %s", m, rest)
			}
			wrote = true
		case "예외", "참고":
			say("%s %s %s", m, s, rest)
		}
	}
	if check && wrote {
		sdiff = "설정"
	}
	realErr := false
	for _, l := range splitLines(serr.String()) {
		if !goRunWarn(l) {
			realErr = true
		}
	}
	bad := false
	switch r {
	case 0:
	case 1: // 충돌이 아니고, --check의 다름(표준 오류 없음)도 아니면 도구 문제
		if *conf > c0 || (check && wrote && !realErr) {
			break
		}
		bad = true
	case 127:
		warn("%s에 ~/.local/bin/herdr-settings-merge가 없다 (설정을 맞추지 않음)", m)
		sdiff = "도구문제:설정"
	default:
		bad = true
	}
	if bad {
		sdiff = "도구문제:설정"
	}
	for _, l := range splitLines(serr.String()) {
		fmt.Fprintf(os.Stderr, "herdr-sync: %s: %s\n", m, l)
	}
	return sdiff
}

func syncMachines(machines []string, mq *qres) {
	MS, MRM := mq.C, mq.RM
	for _, m := range machines {
		q, ok := query(m)
		if !ok {
			if q.nosh { // 종료 코드에 넣지 않는다
				rows += m + ": 건너뜀 (sh 없음 — Windows 등)\n"
				continue
			}
			warn("%s 연결 실패", m)
			row(m, []string{"연결"}, 0)
			continue
		}
		// Darwin·Linux가 아니면 쓰기·지움·링크·base·설정을 하나도 하지 않는다 (--take로 집어도)
		if q.O != "Darwin" && q.O != "Linux" {
			rows += m + ": 건너뜀 (OS " + orUnknown(q.O) + ")\n"
			continue
		}
		machOS[m] = q.O
		var diffs, bp, wm, cf, wk []string
		var wi, links []int
		conf := 0
		shown := map[string]bool{}
		for i, u := range U {
			if !targets(u.t, m, q.O) {
				continue
			}
			k, mv, c, b := u.k, MS[i], q.C[i], q.B[i]
			if strings.HasSuffix(k, "@") { // 링크는 쓰기 뒤에 (가리킬 곳을 이번에 쓸 수 있다)
				links = append(links, i)
				continue
			}
			switch mv {
			case "-", "~", "?", "L", "G": // 맥에 없는 것은 맞추지 않는다 (지울 것은 지움 줄로)
				continue
			}
			switch c {
			case "~": // Codex 지침이 없는 기계
				continue
			case "!":
				warn("%s ~/%s에 '<!-- User customizations -->' 줄이 없다", m, strings.SplitN(k, "#", 2)[0])
				diffs = append(diffs, u.b)
				continue
			case "L":
				if !shown[q.L[i]] {
					say("건너뜀 %s %s (링크 — 따라가지 않음)", m, q.L[i])
					shown[q.L[i]] = true
				}
				continue
			case "G":
				say("건너뜀 %s %s (7MB 넘음)", m, k)
				continue
			}
			if c == mv {
				if b != mv {
					bp = append(bp, k+"\t"+mv)
				}
				continue
			}
			// 3-way는 내용(crc.크기)으로 본다: 내용이 맥과 같고 실행 권한만 다르면 충돌이 아니라 씀이다
			if sigBody(c) != sigBody(mv) && c != b && !spec(takes, m, k) {
				if u.tree {
					cf = append(cf, k+"\t"+c)
				} else {
					v := macval(i)
					cur := TMP + "/cur"
					if f, err := os.Create(cur); err == nil {
						r := on(m, false, "r", k)
						r.Stdout, r.Stderr = f, nil
						run(r)
						f.Close()
					}
					say("충돌 %s %s (%d줄 다름)", m, k, lines(v, cur))
				}
				conf++
				continue
			}
			wk = append(wk, k)
			if check {
				say("%s 다름 %s", m, k)
				diffs = append(diffs, u.b)
				continue
			}
			if u.tree {
				wm = append(wm, k+"\t"+sigx(mv)+"\t"+mv+"\t"+k)
				wi = append(wi, i)
				continue
			}
			in, err := os.Open(macval(i))
			if err == nil && runIn(on(m, true, "w", k, sigx(mv), mv, STAMP), in, nil) == 0 {
				say("%s 씀 %s", m, k)
			} else {
				warn("%s %s 쓰기 실패", m, k)
				diffs = append(diffs, u.b)
			}
			if in != nil {
				in.Close()
			}
		}
		// 나무 파일: 충돌 내용 읽기와 쓰기는 7MB 아래 묶음마다 hc 한 번 (파일 수가 아니라 바이트에 비례, 평소 한 번)
		if len(cf) > 0 {
			cd := TMP + "/cur.d"
			os.RemoveAll(cd)
			os.Mkdir(cd, 0o700)
			for _, ch := range chunks(cf, 2) {
				var keys []string
				for _, l := range ch {
					keys = append(keys, field(l, 1))
				}
				t := on(m, true, "T")
				t.Stdin = bytes.NewReader(joinLines(keys))
				x := exec.Command("tar", "-xf", "-")
				x.Dir = cd
				pipeline(nil, t, x, nil)
			}
			for _, l := range cf {
				k := field(l, 1)
				say("충돌 %s %s (%d줄 다름)", m, k, lines(home+"/"+k, cd+"/"+k))
			}
		}
		if len(wm) > 0 {
			var wo bytes.Buffer
			for n, ch := range chunks(wm, 3) {
				pre, tar := wpack(home, ch, fmt.Sprintf("wm%d.src", n))
				pipeline(pre, tar, on(m, true, "W", STAMP), &wo)
			}
			for j, r := range wres(wm, wo.String()) {
				if r[0] == "ok" {
					say("%s 씀 %s", m, r[1])
				} else {
					warn("%s %s 쓰기 실패 (%s)", m, r[1], r[2])
					diffs = append(diffs, U[wi[j]].b)
				}
			}
		}
		there := func(p string) bool { // 그 기계에 있거나, 이번에 그 아래에 쓸 파일이 있다 (링크 대상)
			if q.Y[p] {
				return true
			}
			for _, k := range wk {
				if k == p || strings.HasPrefix(k, p+"/") {
					return true
				}
			}
			return false
		}
		for _, i := range links {
			u := U[i]
			k, c, want := u.k, q.C[i], q.H+"/"+u.r
			if c == want {
				continue
			}
			if c != "-" && !spec(takes, m, k) {
				say("충돌 %s %s (링크가 아니거나 다른 곳을 가리킨다: %s)", m, k, c)
				conf++
				continue
			}
			if !there(u.r) {
				say("건너뜀 %s %s (가리킬 곳 없음)", m, k)
				continue
			}
			if check {
				say("%s 다름:링크 %s", m, k)
				diffs = append(diffs, u.b)
				continue
			}
			if c == "-" || runIn(on(m, false, "d", k, STAMP), nil, nil) == 0 {
				switch runIn(on(m, false, "l", k, u.r), nil, nil) {
				case 0:
					say("%s 씀 %s", m, k)
				case 3:
					say("건너뜀 %s %s (가리킬 곳 없음)", m, k)
				default:
					warn("%s %s 링크를 만들지 못했다", m, k)
					diffs = append(diffs, u.b)
				}
			} else {
				warn("%s %s 링크를 만들지 못했다", m, k)
				diffs = append(diffs, u.b)
			}
		}
		if !check && len(bp) > 0 && runIn(on(m, true, "B"), bytes.NewReader(joinLines(bp)), nil) != 0 {
			warn("%s base를 쓰지 못했다", m)
		}
		diffs = append(diffs, settings(m, &conf))
		for j, k := range RMK {
			if !targets(RMT[j], m, q.O) || !q.RM[k] {
				continue
			}
			switch {
			case check:
				say("%s 다름:지움 %s", m, k)
				diffs = append(diffs, "지움")
			case runIn(on(m, false, "d", k, STAMP), nil, nil) == 0:
				say("%s 지움 %s", m, k)
			default:
				warn("%s %s 지우기 실패", m, k)
				diffs = append(diffs, "지움")
			}
		}
		row(m, diffs, conf)
	}
	// 맥에서 지울 것 (지움 줄은 대상이 무엇이든 맥을 포함한다)
	for _, k := range RMK {
		if !MRM[k] {
			continue
		}
		switch {
		case check:
			say("맥 다름:지움 %s", k)
			rdiff = append(rdiff, "지움")
		case run(mac("d", k, STAMP)) == 0:
			say("맥 지움 %s", k)
		default:
			warn("맥 %s 지우기 실패", k)
			rdiff = append(rdiff, "지움")
		}
	}
	saved := rows
	rows = ""
	if check {
		macrow()
	} else {
		row("맥", rdiff, rconf)
	}
	fmt.Print(rows + saved)
	if !check {
		publish(machines)
	}
}

// publish: 다 맞춘 뒤, 설정 publish.repo가 있으면 범용 저장소를 공개 저장소로 내보낸다(herdr-tools-publish, 기록 없이·검사 뒤).
// 내보내기가 되면 Go 판 도구 바이너리를 그 공개 저장소 릴리스로 올리고 기계마다 받게 한다(release.go).
// 기계 맞추기와 별개라, 실패해도 크게 경고만 하고 종료 코드는 바꾸지 않는다
func publish(machines []string) {
	pubwarn := func(s string) { fmt.Fprintf(os.Stderr, "!!! herdr-sync: %s !!!\n", s) }
	pub, err := toolcfg.Lookup(CONF, "publish.repo", "", os.Stderr)
	if err != nil {
		pubwarn(fmt.Sprintf("개인 설정을 읽지 못해 공개 저장소 내보내기를 건너뜀 (%s/config)", CONF))
		return
	}
	if pub == "" {
		return
	}
	c := exec.Command("bash", REPO+"/bin/herdr-tools-publish")
	c.Env = append(os.Environ(), "HERDR_TOOLS_CONFIG="+CONF, "HERDR_TOOLS_REPO="+REPO)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if run(c) != 0 {
		pubwarn("공개 저장소 내보내기 실패 (맞추기는 끝났다) — herdr-tools-publish 출력을 보고 고친 뒤 herdr-tools-publish를 다시")
		return
	}
	release(pub, machines, pubwarn)
}
