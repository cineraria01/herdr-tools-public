package main

// herdr-settings-merge 계약 시험(옛 tests/herdr-settings-merge.test.sh, 2026-11-21 go-tests-ops): 임시 HOME마다 settings.json·base를
// 두고 herdr-settings-merge Go 바이너리(clitest.Bin)를 밖에서 부른다. 실제 ~/.claude·hc·네트워크는 쓰지 않는다.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ct "herdr-tools/internal/clitest"
	"herdr-tools/internal/ojson"
)

// js: Go 글을 JSON 글자열로(jq --arg)
func js(s string) string { b, _ := json.Marshal(s); return string(b) }

// canon: JSON 글 → jq -cS 한 줄
func canon(s string) string {
	v, err := ojson.Parse([]byte(s))
	if err != nil {
		return "!깨짐 " + s
	}
	return ojson.Canon(v)
}

// ckj: herdr-settings-merge base 값(셸 ck — JSON 글을 jq -cS로 고친 뒤 cksum)
func ckj(s string) string { return ct.Ck(canon(s)) }

// at: jq 경로(.키·[n]) — 없으면 nil(null)
func at(v any, path ...any) any {
	for _, p := range path {
		switch k := p.(type) {
		case string:
			o, ok := v.(*ojson.Object)
			if !ok {
				return nil
			}
			v = o.M[k]
		case int:
			a, ok := v.([]any)
			if !ok || k >= len(a) {
				return nil
			}
			v = a[k]
		}
	}
	return v
}

// dirNames: 폴더 항목 이름 전부(점 파일 포함, 정렬, 빈칸으로 이음)
func dirNames(d string) string {
	es, err := os.ReadDir(d)
	if err != nil {
		return "!" + err.Error()
	}
	n := make([]string, 0, len(es))
	for _, e := range es {
		n = append(n, e.Name())
	}
	return strings.Join(n, " ") // ReadDir는 이름순
}

// load: JSON 파일(못 읽으면 nil과 false)
func load(p string) (any, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	v, err := ojson.Parse(b)
	return v, err == nil
}

func TestContract(t *testing.T) {
	merge := ct.Bin(t, "herdr-settings-merge")
	keys := filepath.Join(ct.Repo(t), "config.example/settings/keys.json") // 가짜 예시 목록(진짜는 개인 설정)
	T := ct.TempDir(t)

	var r ct.Res
	var H string
	c := &ct.C{T: t, Detail: func() string { return fmt.Sprintf("  out: %s\n  err: %s", r.Out, r.Err) }}
	homes := 0
	// fresh <settings JSON> [base JSON]: 새 임시 HOME
	fresh := func(settings string, base ...string) {
		homes++
		H = fmt.Sprintf("%s/home%d.%d", T, c.N, homes)
		ct.Put(t, H+"/.claude/settings.json", settings+"\n")
		if len(base) > 0 {
			ct.Put(t, H+"/.local/state/herdr-sync/settings-base.json", base[0]+"\n")
		}
	}
	// runM <기계> <입력 JSON> [인자…]
	runM := func(m, in string, args ...string) {
		r = ct.Cmd{Stdin: in, Env: []string{"HOME=" + H, "HERDR_MACHINE=" + m, "HERDR_KEYS=" + keys}}.Run(merge, args...)
	}
	run := func(in string, args ...string) { runM("w1", in, args...) }
	settings := func() any { v, _ := load(H + "/.claude/settings.json"); return v }
	S := func(path ...any) string { return ojson.Canon(at(settings(), path...)) }
	baseRaw := func(k string) string {
		v, _ := load(H + "/.local/state/herdr-sync/settings-base.json")
		s, _ := at(v, k).(string)
		return s
	}
	has := func(l string) bool { return ct.HasLine(r.Out, l) }
	bakDir := func() string { return H + "/.local/state/herdr-sync/bak" }
	nobak := func() bool { return !ct.IsDir(bakDir()) }
	bakAll := func() string {
		m, _ := filepath.Glob(bakDir() + "/*/settings.json")
		var b strings.Builder
		for _, p := range m {
			b.WriteString(ct.Read(p))
		}
		return strings.TrimRight(b.String(), "\n")
	}
	commands := func() string { // jq -cS "[.hooks.PreToolUse[].hooks[].command]"
		var cs []any
		ps, _ := at(settings(), "hooks", "PreToolUse").([]any)
		for _, p := range ps {
			hs, _ := at(p, "hooks").([]any)
			for _, h := range hs {
				cs = append(cs, at(h, "command"))
			}
		}
		if cs == nil {
			cs = []any{}
		}
		return ojson.Canon(cs)
	}
	guard := `/bin/bash -c 'exec /bin/bash "$HOME/.claude/hooks/cotenant-guard.sh"'`
	moshi := `{"matcher":"Bash","hooks":[{"type":"command","command":"moshi-hook.sh"}]}`

	suite := func() {
		// 같음: base가 없어도 같으면 같음, base를 남긴다
		fresh(`{"language":"한국어","theme":"dark"}`)
		run(`{"values":{".language":"한국어"}}`)
		c.Ok("같음: 종료 0·같음 줄", r.RC == 0 && r.Out == "같음 .language")
		c.Ok("같음: base에 cksum, settings·백업은 그대로", baseRaw(".language") == ckj(`"한국어"`) && nobak() && S("theme") == `"dark"`)

		// 씀: 현재 = base → 맥 값으로, 백업·권한·base
		fresh(`{"model":"old","theme":"dark"}`, `{".model":"`+ckj(`"old"`)+`"}`)
		ct.Chmod(t, H+"/.claude/settings.json", 0o640)
		ct.Put(t, H+"/.claude/.keep", "x\n") // 원래 있던 점 파일은 그대로 남아야 한다
		before := strings.TrimRight(ct.Read(H+"/.claude/settings.json"), "\n")
		entries0 := dirNames(H + "/.claude")
		run(`{"values":{".model":"new"}}`)
		c.Ok("씀: 종료 0·씀 줄", r.RC == 0 && r.Out == "씀 .model")
		c.Ok("씀: 값이 바뀌고 다른 키는 그대로", S("model") == `"new"` && S("theme") == `"dark"`)
		st, err := os.Stat(H + "/.claude/settings.json")
		c.Ok("씀: 권한 그대로(640)", err == nil && st.Mode().IsRegular() && st.Mode().Perm() == 0o640)
		c.Ok("씀: bak/<시각>/settings.json에 원본", bakAll() == before)
		c.Ok("씀: base가 새 값으로", baseRaw(".model") == ckj(`"new"`))
		// 쓰기 전후 폴더 항목 전체(점 파일 포함)가 같아야 한다 — 숨긴 임시 이름도 남으면 실패
		c.Ok("씀: 임시 파일을 남기지 않음", dirNames(H+"/.claude") == entries0 && entries0 == ".keep settings.json")
		run(`{"values":{".model":"new"}}`)
		c.Ok("씀 뒤 다시: 같음", r.RC == 0 && r.Out == "같음 .model")

		// 충돌: 현재 ≠ base ≠ 맥
		fresh(`{"model":"mine"}`, `{".model":"`+ckj(`"old"`)+`"}`)
		run(`{"values":{".model":"new"}}`)
		c.Ok("충돌: 종료 1·충돌 줄·쓰지 않음", r.RC == 1 && r.Out == "충돌 .model" && S("model") == `"mine"` && nobak())
		// 처음(base 없음)이면서 다름 → 충돌. 대상에 키가 없어도(null) 같다
		fresh(`{"model":"mine"}`)
		run(`{"values":{".model":"new",".language":"한국어"}}`)
		c.Ok("처음·다름: 충돌(키가 없는 것도)", r.RC == 1 && has("충돌 .model") && has("충돌 .language") && S("model") == `"mine"`)
		c.Ok("처음·다름: base를 만들지 않음", !ct.Exists(H+"/.local/state/herdr-sync/settings-base.json"))
		run(`{"values":{".model":"new",".language":"한국어"}}`, "--take", ".model")
		c.Ok("--take <키>: 그 키만 씀, 나머지는 충돌", r.RC == 1 && has("씀 .model") && has("충돌 .language") && S("model") == `"new"` && S("language") == "null")
		run(`{"values":{".model":"new",".language":"한국어"}}`, "--take", "*")
		c.Ok("--take *: 전부 씀, 종료 0", r.RC == 0 && has("같음 .model") && has("씀 .language") && S("language") == `"한국어"`)

		// 예외: lx(예시 keys.json의 exceptions)의 .modelSettings·.effortLevel은 --take *로도 손대지 않는다
		fresh(`{"effortLevel":"medium","modelSettings":{"a":1},"model":"x"}`)
		runM("lx", `{"values":{".effortLevel":"xhigh",".modelSettings":{"a":2},".model":"x"}}`, "--take", "*")
		c.Ok("예외: 예외 줄, 값 그대로, 종료 0", r.RC == 0 && has("예외 .effortLevel") && has("예외 .modelSettings") && has("같음 .model") && S("effortLevel") == `"medium"` && S("modelSettings", "a") == "1")
		runM("w2", `{"values":{".effortLevel":"xhigh"}}`, "--take", "*")
		c.Ok("예외: 다른 기계는 맞춘다", r.RC == 0 && r.Out == "씀 .effortLevel")

		// 참고: 다르면 참고 줄만, 같으면 없음, refs가 없으면 없음. 이 기계 HOME은 맥 HOME으로 되돌려 비교
		fresh(`{}`)
		ct.Put(t, H+"/.claude/settings.json", `{"statusLine":{"command":"`+H+`/.claude/sl.sh"},"permissions":{"allow":["x"]}}`+"\n")
		run(`{"values":{},"home":"/Users/mac","refs":{".statusLine":"` + ckj(`{"command":"/Users/mac/.claude/sl.sh"}`) + `",".permissions":"` + ckj(`{"allow":["y"]}`) + `",".enabledPlugins":"` + ck("null") + `"}}`)
		c.Ok("참고: 다른 것만 참고 줄, 종료 0, 쓰지 않음", r.RC == 0 && r.Out == "참고 .permissions" && nobak())
		run(`{"values":{}}`)
		c.Ok("참고: refs가 없으면 찍지 않음", r.RC == 0 && r.Out == "")

		// 훅: 없으면 더하고, 다른 훅 항목은 그대로, 다시 돌리면 같음
		fresh(`{"hooks":{"PreToolUse":[` + moshi + `],"Stop":[` + moshi + `]}}`)
		hin := `{"values":{},"hooks":[{"event":"PreToolUse","matcher":"Edit|Bash","command":` + js(guard) + `,"timeout":5,"onFailure":"block"}]}`
		run(hin)
		c.Ok("훅 더하기: 씀 줄, 종료 0", r.RC == 0 && r.Out == "씀 hook:PreToolUse:cotenant-guard")
		exp := canon(`{"matcher":"Edit|Bash","hooks":[{"type":"command","command":` + js(guard) + `,"timeout":5,"onFailure":"block"}]}`)
		c.Ok("훅 더하기: 끝에 붙고 필드 그대로", S("hooks", "PreToolUse", 1) == exp)
		c.Ok("훅 더하기: 다른 훅 항목은 그대로", S("hooks", "PreToolUse", 0) == canon(moshi) && S("hooks", "Stop") == canon("["+moshi+"]"))
		run(hin)
		n, _ := at(settings(), "hooks", "PreToolUse").([]any)
		c.Ok("훅: 다시 돌리면 같음, 하나만 있음", r.RC == 0 && r.Out == "같음 hook:PreToolUse:cotenant-guard" && len(n) == 2)
		// 훅 3-way: 옛 command가 base와 같으면 그 항목만 바꾸고, 같은 묶음의 다른 훅은 남긴다
		old := `{"matcher":"Edit","hook":{"command":"old cotenant-guard.sh","type":"command"}}`
		fresh(`{"hooks":{"PreToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"old cotenant-guard.sh"},{"type":"command","command":"moshi-hook.sh"}]}]}}`, `{"hook:PreToolUse:cotenant-guard":"`+ckj(old)+`"}`)
		run(hin)
		c.Ok("훅 바꾸기: 씀, 같은 묶음의 다른 훅은 남김, 옛 항목은 없음", r.RC == 0 && r.Out == "씀 hook:PreToolUse:cotenant-guard" && commands() == canon(`["moshi-hook.sh",`+js(guard)+`]`))
		fresh(`{"hooks":{"PreToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"mine cotenant-guard.sh"}]}]}}`)
		run(hin)
		c.Ok("훅: 처음이면서 다르면 충돌, 쓰지 않음", r.RC == 1 && r.Out == "충돌 hook:PreToolUse:cotenant-guard" && nobak())
		run(`{"values":{},"hooks":[{"event":"PreToolUse","matcher":"Bash","command":"other.sh"}]}`)
		c.Ok("훅: 맞출 목록에 없는 훅을 주면 종료 2", r.RC == 2 && r.Out == "" && nobak())

		// 대상에서 지운 훅은 base가 있어도 다시 더한다. 훅 command 안의 맥 HOME도 바꾼다
		fresh(`{"hooks":{"PreToolUse":[]}}`, `{"hook:PreToolUse:loop-no-question":"`+ckj(`{"matcher":"x"}`)+`"}`)
		run(`{"home":"/Users/mac","values":{},"hooks":[{"event":"PreToolUse","matcher":"Ask","command":"/Users/mac/.claude/hooks/loop-no-question.sh"}]}`)
		c.Ok("훅: 지운 훅을 다시 더하고 HOME을 바꿈", r.RC == 0 && r.Out == "씀 hook:PreToolUse:loop-no-question" && S("hooks", "PreToolUse", 0, "hooks", 0, "command") == js(H+"/.claude/hooks/loop-no-question.sh"))

		// 목록 밖 키·참고·이벤트는 받지 않는다(종료 2, 쓰지 않음)
		fresh(`{"hooks":{"Stop":[]},"permissions":{"allow":["x"]}}`)
		run(`{"values":{".hooks":{},".permissions":{}}}`, "--take", "*")
		c.Ok("목록 밖 값 키: 종료 2, 그대로", r.RC == 2 && r.Out == "" && S("permissions", "allow", 0) == `"x"` && nobak())
		run(`{"values":{},"refs":{".hooks":"1 2"}}`)
		c.Ok("목록 밖 참고 키: 종료 2", r.RC == 2 && r.Out == "")
		run(`{"values":{},"hooks":[{"event":"Stop","matcher":"","command":"cotenant-guard.sh"}]}`)
		c.Ok("keys.json에 없는 이벤트의 훅: 종료 2", r.RC == 2 && r.Out == "" && nobak())

		// 중간 키가 객체가 아니면(env가 문자열) 「씀」이라 하지 않고 종료 1, 아무것도 쓰지 않음
		fresh(`{"env":"oops"}`)
		run(`{"values":{".env.ENABLE_TOOL_SEARCH":"1"}}`, "--take", "*")
		c.Ok("읽을 수 없는 경로: 종료 1, 씀 없음, 오류 한 줄, 그대로", r.RC == 1 && !has("씀 .env.ENABLE_TOOL_SEARCH") && ct.NewlineCount(r.Err) == 1 && S("env") == `"oops"` && nobak() && !ct.Exists(H+"/.local/state/herdr-sync/settings-base.json"))

		fresh(`{"model":"a","env":"oops"}`)
		run(`{"values":{".model":"b",".env.ENABLE_TOOL_SEARCH":"1"}}`, "--take", "*")
		c.Ok("읽기 실패: 앞에서 판정한 키의 씀 줄도 내지 않음, 그대로", r.RC == 1 && r.Out == "" && S("model") == `"a"` && nobak())
		run(`{"values":{},"hooks":[{"event":"PreToolUse","matcher":"A","command":"loop-no-question.sh"},{"event":"PreToolUse","matcher":"B","command":"x loop-no-question.sh"}]}`)
		c.Ok("같은 판별 글자의 훅 둘: 종료 2", r.RC == 2 && r.Out == "" && nobak())
		run(`{"values":{},"hooks":[{"event":"PreToolUse","matcher":"A","command":"cotenant-guard.sh && loop-no-question.sh"}]}`)
		c.Ok("판별 글자 둘이 든 훅: 종료 2", r.RC == 2 && r.Out == "" && nobak())

		// 연달아 두 번 써도 백업이 둘 남는다(같은 초면 -<pid>)
		fresh(`{"model":"orig"}`)
		run(`{"values":{".model":"v1"}}`, "--take", "*")
		run(`{"values":{".model":"v2"}}`, "--take", "*")
		bs, _ := os.ReadDir(bakDir())
		c.Ok("백업: 두 번 쓰면 둘, 처음 값이 남음", len(bs) == 2 && strings.Contains(bakAll(), "orig"))

		// HOME 바꾸기: 맥 HOME과 그 아래만, /Users/macx는 그대로. 중첩 env 키는 다른 env를 남긴다
		fresh(`{"env":{"KEEP":"1"}}`)
		run(`{"home":"/Users/mac","values":{".env.ENABLE_TOOL_SEARCH":"/Users/mac/a:/Users/macx/b",".tui":"/Users/mac",".autoMode":{"x":["at /Users/mac, here /Users/mac.d"]}}}`, "--take", "*")
		c.Ok("HOME: 맥 HOME을 이 기계 HOME으로", r.RC == 0 && S("env", "ENABLE_TOOL_SEARCH") == js(H+"/a:/Users/macx/b") && S("tui") == js(H) && S("env", "KEEP") == `"1"`)
		c.Ok("HOME: 글 속(뒤에 쉼표)도 바꾸고 /Users/mac.d는 그대로", S("autoMode", "x", 0) == js("at "+H+", here /Users/mac.d"))
		run(`{"home":"/Users/mac","values":{".env.ENABLE_TOOL_SEARCH":"/Users/mac/a:/Users/macx/b"}}`)
		c.Ok("HOME: 바꾼 값과 같으면 같음", r.RC == 0 && r.Out == "같음 .env.ENABLE_TOOL_SEARCH")
		run(`{"values":{".env.ENABLE_TOOL_SEARCH":null}}`)
		env, _ := at(settings(), "env").(*ojson.Object)
		_, kept := env.Get("ENABLE_TOOL_SEARCH")
		c.Ok("null 값: base와 같으면 키를 지움", r.RC == 0 && r.Out == "씀 .env.ENABLE_TOOL_SEARCH" && env != nil && !kept)

		// 깨진 settings.json: 손대지 않고 종료 1, 표준 오류 한 줄
		fresh(`{"model": "x",`)
		run(`{"values":{".model":"y"}}`, "--take", "*")
		c.Ok("깨짐: 종료 1, 출력 없음, 오류 한 줄, 파일 그대로", r.RC == 1 && r.Out == "" && ct.NewlineCount(r.Err) == 1 && ct.Is(H+"/.claude/settings.json", `{"model": "x",`) && nobak())

		// --check: 아무것도 쓰지 않는다
		fresh(`{"model":"old"}`, `{".model":"`+ckj(`"old"`)+`"}`)
		baseBefore, _ := os.ReadFile(H + "/.local/state/herdr-sync/settings-base.json")
		run(`{"values":{".model":"new",".language":"x"},"hooks":[{"event":"PreToolUse","matcher":"B","command":"loop-no-question.sh"}]}`, "--check", "--take", "*")
		c.Ok("--check: 씀을 보이고 종료 1", r.RC == 1 && has("씀 .model") && has("씀 .language") && has("씀 hook:PreToolUse:loop-no-question"))
		baseAfter, err := os.ReadFile(H + "/.local/state/herdr-sync/settings-base.json")
		c.Ok("--check: settings·base·백업 모두 그대로", S("model") == `"old"` && err == nil && bytes.Equal(baseBefore, baseAfter) && nobak())
		fresh(`{"model":"x"}`)
		run(`{"values":{".model":"x"}}`, "--check")
		c.Ok("--check: 다 같으면 종료 0, base도 만들지 않음", r.RC == 0 && !ct.Exists(H+"/.local/state/herdr-sync"))

		// 심볼릭 링크 settings.json은 링크를 살리고 대상에 쓴다
		fresh(`{}`)
		os.Remove(H + "/.claude/settings.json")
		ct.Put(t, H+"/real.json", `{"model":"old"}`+"\n")
		os.Symlink(H+"/real.json", H+"/.claude/settings.json")
		run(`{"values":{".model":"new"}}`, "--take", ".model")
		rv, _ := load(H + "/real.json")
		c.Ok("링크: 링크는 그대로, 대상 파일이 바뀜", r.RC == 0 && ct.IsLink(H+"/.claude/settings.json") && at(rv, "model") == "new")

		// 잘못 부름: 종료 2
		fresh(`{}`)
		run(`{"values":{}}`, "--bogus")
		c.Ok("잘못 부름: 모르는 인자", r.RC == 2)
		run(`{"values":{}}`, "--take")
		c.Ok("잘못 부름: --take 뒤 키 없음", r.RC == 2)
		run(`not json`)
		c.Ok("잘못 부름: 입력이 JSON이 아님", r.RC == 2 && nobak())
		run(`{"hooks":[]}`)
		c.Ok("잘못 부름: values 없음", r.RC == 2)
		runM("", `{"values":{}}`)
		c.Ok("잘못 부름: HERDR_MACHINE 없음", r.RC == 2)
	}

	suite()
	c.Done()
}
