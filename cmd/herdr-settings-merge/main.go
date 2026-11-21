// herdr-settings-merge: 이 기계 ~/.claude/settings.json의 정한 키만 맥 값으로 맞춘다. 키마다 3-way(CONTRACT.md 「설정」).
// Go 판(2026-11-21 사용자 「모두 고로 옮기고 싶어」 — ops-go). 표준 입력·인자·출력·종료 코드·base 꼴은 셸 판 bin/herdr-settings-merge와 같다
// (base·refs의 cksum은 `jq -cS` 한 줄과 같은 바이트로 계산 — internal/ojson). 되돌리기 HERDR_SETTINGS_MERGE_IMPL=bash.
// keys.json 자리는 HERDR_KEYS(포장 bin/herdr-settings-merge가 곁의 ../settings/keys.json을 넣어 준다), 없으면 ~/.local/share/herdr-sync/keys.json.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/cksum"
	"herdr-tools/internal/ojson"
	"herdr-tools/internal/version"
)

const me = "herdr-settings-merge"

func die2(f string, a ...any) {
	fmt.Fprintf(os.Stderr, me+": "+f+"\n", a...)
	os.Exit(2)
}

func die1(f string, a ...any) {
	fmt.Fprintf(os.Stderr, me+": "+f+"\n", a...)
	os.Exit(1)
}

// fail: 도중 실패 — 모은 판정 줄은 버린다(쓰지 않은 것을 「씀」이라 하지 않게)
func fail(f string, a ...any) { die1(f+", 아무것도 쓰지 않음", a...) }

func ck(canon string) string { return cksum.Space([]byte(canon + "\n")) }

func path(k string) []string { return strings.Split(strings.TrimPrefix(k, "."), ".") }

func strList(v any) []string {
	a, _ := v.([]any)
	var r []string
	for _, e := range a {
		if s, ok := e.(string); ok {
			r = append(r, s)
		}
	}
	return r
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func obj(v any) *ojson.Object {
	o, _ := v.(*ojson.Object)
	return o
}

// markers: keys.json hooks[ev] 판별 글자 가운데 command에 든 것
func markers(keys *ojson.Object, ev, command string) []string {
	var r []string
	if h := obj(keys.M["hooks"]); h != nil {
		for _, m := range strList(h.M[ev]) {
			if strings.Contains(command, m) {
				r = append(r, m)
			}
		}
	}
	return r
}

var nameChar = regexp.MustCompile(`^[A-Za-z0-9._-]`)

// rh: 문자열 안의 from을 to로. 뒤에 이름 글자가 이어지면 그대로(/Users/a가 /Users/ab를 건드리지 않게)
func rh(from, to string) func(string) string {
	return func(s string) string {
		p := strings.Split(s, from)
		r := p[0]
		for _, x := range p[1:] {
			if nameChar.MatchString(x) {
				r += from + x
			} else {
				r += to + x
			}
		}
		return r
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version.String(me))
		return
	}
	check, takeAll := false, false
	takes := map[string]bool{}
	a := os.Args[1:]
	for i := 0; i < len(a); i++ {
		switch a[i] {
		case "--check":
			check = true
		case "--take":
			if i+1 >= len(a) || a[i+1] == "" {
				die2("--take 뒤에 키가 없음")
			}
			i++
			takes[a[i]] = true
			takeAll = takeAll || a[i] == "*"
		default:
			die2("모르는 인자: %s", a[i])
		}
	}
	machine := os.Getenv("HERDR_MACHINE")
	if machine == "" {
		die2("HERDR_MACHINE 없음")
	}
	home := os.Getenv("HOME")
	kf := os.Getenv("HERDR_KEYS")
	if kf == "" {
		kf = home + "/.local/share/herdr-sync/keys.json"
	}
	kb, err := os.ReadFile(kf)
	kv, perr := ojson.Parse(kb)
	keys := obj(kv)
	if err != nil || perr != nil || keys == nil {
		die2("keys.json을 읽을 수 없음: %s", kf)
	}
	inb, err := io.ReadAll(os.Stdin)
	if err != nil {
		die1("표준 입력을 읽지 못했다: %v", err)
	}
	inv, err := ojson.Parse(inb)
	in := obj(inv)
	if err != nil || in == nil || !valid(in, keys) {
		die2("표준 입력이 약속한 JSON이 아님(또는 keys.json 목록 밖의 키·훅)")
	}
	mh, _ := in.M["home"].(string)
	want := obj(ojson.Copy(in))
	if mh != "" && mh != home {
		for _, k := range []string{"values", "hooks"} {
			if v, ok := want.M[k]; ok {
				want.M[k] = ojson.Walk(v, rh(mh, home))
			}
		}
	}

	S := home + "/.claude/settings.json"
	var work any = ojson.NewObject()
	sexists := false
	if sb, err := os.ReadFile(S); err == nil || !os.IsNotExist(err) {
		v, perr := ojson.Parse(sb)
		if err != nil || perr != nil || obj(v) == nil {
			die1("%s 이 올바른 JSON 객체가 아니라 손대지 않음", S)
		}
		work, sexists = v, true
	}
	ST := home + "/.local/state/herdr-sync"
	B := ST + "/settings-base.json"
	base0 := []byte("{}\n")
	base := ojson.NewObject()
	if bb, err := os.ReadFile(B); err == nil {
		if v, err := ojson.Parse(bb); err == nil && obj(v) != nil {
			base, base0 = obj(v), bb
		}
	}
	baseSet := false
	setbase := func(k, canon string) { base.Set(k, ck(canon)); baseSet = true }

	var out bytes.Buffer
	changed, bad := false, false
	// decide: 판정을 찍는다. 씀이면 참(쓰는 것은 부른 쪽)
	decide := func(k, cur, wantc string) bool {
		if cur == wantc {
			fmt.Fprintf(&out, "같음 %s\n", k)
			setbase(k, wantc)
			return false
		}
		b, _ := base.M[k].(string)
		if (b != "" && ck(cur) == b) || takes[k] || takeAll {
			fmt.Fprintf(&out, "씀 %s\n", k)
			setbase(k, wantc)
			bad = bad || check
			return true
		}
		fmt.Fprintf(&out, "충돌 %s\n", k)
		bad = true
		return false
	}

	exc := []string(nil)
	if e := obj(keys.M["exceptions"]); e != nil {
		exc = strList(e.M[machine])
	}
	values := obj(want.M["values"])
	for _, k := range values.Keys {
		if has(exc, k) {
			fmt.Fprintf(&out, "예외 %s\n", k)
			continue
		}
		p := path(k)
		cv, err := ojson.GetPath(work, p)
		if err != nil {
			fail("%s 를 읽을 수 없음(중간 키가 객체가 아님)", k)
		}
		wv := ojson.Sorted(values.M[k])
		if decide(k, ojson.Canon(cv), ojson.Canon(wv)) {
			if wv == nil {
				work, err = ojson.DelPath(work, p)
			} else {
				work, err = ojson.SetPath(work, p, wv)
			}
			if err != nil {
				fail("jq로 고치기 실패")
			}
			changed = true
		}
	}

	hooks, _ := want.M["hooks"].([]any)
	for _, hv := range hooks {
		h := obj(hv)
		ev, _ := h.M["event"].(string)
		command, _ := h.M["command"].(string)
		mk := markers(keys, ev, command)[0]
		k := "hook:" + ev + ":" + mk
		hk := ojson.NewObject()
		hk.Set("type", "command")
		for _, f := range h.Keys {
			if f != "event" && f != "matcher" {
				hk.Set(f, h.M[f])
			}
		}
		d := ojson.NewObject()
		d.Set("matcher", h.M["matcher"])
		d.Set("hook", hk)
		dv := obj(ojson.Sorted(d))
		wantc := ojson.Canon(dv)
		cur, err := curHook(work, ev, mk)
		if err != nil {
			fail("hooks.%s 를 읽을 수 없음", ev)
		}
		w := false
		if cur == nil {
			fmt.Fprintf(&out, "씀 %s\n", k)
			setbase(k, wantc)
			bad = bad || check
			w = true
		} else {
			w = decide(k, ojson.Canon(cur), wantc)
		}
		if w {
			if work, err = putHook(work, ev, mk, dv); err != nil {
				fail("jq로 고치기 실패")
			}
			changed = true
		}
	}

	if refs := obj(in.M["refs"]); refs != nil {
		for _, k := range refs.Keys {
			cv, err := ojson.GetPath(work, path(k))
			if err != nil {
				fail("%s 를 읽을 수 없음", k)
			}
			cv = ojson.Copy(cv)
			if mh != "" && mh != home {
				cv = ojson.Walk(cv, rh(home, mh))
			}
			if ck(ojson.Canon(cv)) != raw(refs.M[k]) {
				fmt.Fprintf(&out, "참고 %s\n", k)
			}
		}
	}

	code := 0
	if bad {
		code = 1
	}
	if check {
		os.Stdout.Write(out.Bytes())
		os.Exit(code)
	}
	if changed {
		writeSettings(S, ST, sexists, work)
	}
	os.Stdout.Write(out.Bytes()) // settings를 다 썼으니 판정 줄은 참이다
	if baseSet {
		if nb := []byte(ojson.Pretty(base)); !bytes.Equal(nb, base0) {
			if err := os.MkdirAll(ST, 0o755); err != nil {
				die1("base 쓰기 실패")
			}
			if err := atomic.WriteFile(B, nb, 0o644); err != nil {
				die1("base 쓰기 실패")
			}
		}
	}
	os.Exit(code)
}

// raw: `jq -r` 꼴 글자(문자열은 그대로, 그 밖은 JSON)
func raw(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ojson.Compact(v)
}

// valid: 셸 판 jq -e 검사와 같다 — keys.json 목록 밖의 키·참고·훅은 받지 않는다
func valid(in, keys *ojson.Object) bool {
	values := obj(in.M["values"])
	if values == nil {
		return false
	}
	kv := strList(keys.M["values"])
	for _, k := range values.Keys {
		if !has(kv, k) {
			return false
		}
	}
	if r, ok := in.M["refs"]; ok && r != nil && r != false {
		refs := obj(r)
		if refs == nil {
			return false
		}
		kr := strList(keys.M["refs"])
		for _, k := range refs.Keys {
			if !has(kr, k) {
				return false
			}
		}
	}
	if h, ok := in.M["home"]; ok && h != nil && h != false {
		if _, ok := h.(string); !ok {
			return false
		}
	}
	if hv, ok := in.M["hooks"]; ok && hv != nil && hv != false {
		hooks, ok := hv.([]any)
		if !ok {
			return false
		}
		seen := map[string]bool{}
		for _, e := range hooks {
			h := obj(e)
			if h == nil {
				return false
			}
			ev, ok1 := h.M["event"].(string)
			_, ok2 := h.M["matcher"].(string)
			c, ok3 := h.M["command"].(string)
			if !ok1 || !ok2 || !ok3 {
				return false
			}
			m := markers(keys, ev, c)
			if len(m) != 1 || seen[ev+"\x00"+m[0]] {
				return false
			}
			seen[ev+"\x00"+m[0]] = true
		}
	}
	return true
}

// each: jq `.[]?`가 아닌 `(x // [])[]` — null은 없음, 배열·객체는 원소, 그 밖은 오류
func each(v any) ([]any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case bool:
		if !x {
			return nil, nil
		}
	case []any:
		return x, nil
	case *ojson.Object:
		r := make([]any, 0, len(x.Keys))
		for _, k := range x.Keys {
			r = append(r, x.M[k])
		}
		return r, nil
	}
	return nil, ojson.ErrIndex
}

// field: jq `.k` — null은 null, 객체가 아니면 오류
func field(v any, k string) (any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case *ojson.Object:
		return x.M[k], nil
	}
	return nil, ojson.ErrIndex
}

// mine: 훅 항목의 command(없으면 "")에 판별 글자가 드나
func mine(hook any, mk string) (bool, error) {
	c, err := field(hook, "command")
	if err != nil {
		return false, err
	}
	switch x := c.(type) {
	case nil:
		return false, nil
	case bool:
		if !x {
			return false, nil
		}
	case string:
		return strings.Contains(x, mk), nil
	}
	return false, ojson.ErrIndex
}

// curHook: 대상에서 판별 글자가 든 첫 훅을 {matcher, hook}으로(없으면 nil)
func curHook(work any, ev, mk string) (any, error) {
	hs, err := field(work, "hooks")
	if err != nil {
		return nil, err
	}
	evs, err := field(hs, ev)
	if err != nil {
		return nil, err
	}
	entries, err := each(evs)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		mt, err := field(e, "matcher")
		if err != nil {
			return nil, err
		}
		inner, err := field(e, "hooks")
		if err != nil {
			return nil, err
		}
		items, err := each(inner)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			ok, err := mine(it, mk)
			if err != nil {
				return nil, err
			}
			if ok {
				r := ojson.NewObject()
				r.Set("matcher", mt)
				r.Set("hook", it)
				return r, nil
			}
		}
	}
	return nil, nil
}

// putHook: 판별 글자가 든 항목만 빼고(그래서 빈 묶음만 지우고) 맥 항목을 끝에 붙인다. 다른 훅 항목은 그대로
func putHook(work any, ev, mk string, d *ojson.Object) (any, error) {
	hs, err := field(work, "hooks")
	if err != nil {
		return nil, err
	}
	evs, err := field(hs, ev)
	if err != nil {
		return nil, err
	}
	entries, err := each(evs)
	if err != nil {
		return nil, err
	}
	next := []any{}
	for _, e := range entries {
		inner, err := field(e, "hooks")
		if err != nil {
			return nil, err
		}
		items, err := each(inner)
		if err != nil {
			return nil, err
		}
		any_ := false
		var keep []any
		for _, it := range items {
			ok, err := mine(it, mk)
			if err != nil {
				return nil, err
			}
			if ok {
				any_ = true
			} else {
				keep = append(keep, it)
			}
		}
		if !any_ {
			next = append(next, e)
			continue
		}
		if len(keep) == 0 {
			continue
		}
		eo := obj(e) // any_가 참이면 e는 객체다(null의 hooks는 비었다)
		eo.Set("hooks", keep)
		next = append(next, eo)
	}
	add := ojson.NewObject()
	add.Set("matcher", d.M["matcher"])
	add.Set("hooks", []any{d.M["hook"]})
	return ojson.SetPath(work, []string{"hooks", ev}, append(next, add))
}

// writeSettings: 백업(bak/<시각>/settings.json, 같은 초면 -<pid>) 뒤 원본 권한으로 임시 → fsync → rename. 링크면 링크 대상에 쓴다
func writeSettings(S, ST string, exists bool, work any) {
	data := []byte(ojson.Pretty(work))
	if obj(work) == nil {
		die1("고친 결과가 올바른 JSON이 아니라 쓰지 않음")
	}
	if err := os.MkdirAll(filepath.Dir(S), 0o755); err != nil {
		os.Exit(1)
	}
	if exists {
		bak := ST + "/bak/" + time.Now().Format("20060102-150405")
		if _, err := os.Lstat(bak); err == nil {
			bak = fmt.Sprintf("%s-%d", bak, os.Getpid())
		}
		if err := os.MkdirAll(bak, 0o755); err != nil || copyPreserve(S, bak+"/settings.json") != nil {
			die1("백업 실패, 쓰지 않음")
		}
	}
	if fi, err := os.Lstat(S); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		f, err := os.OpenFile(S, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
		if err != nil {
			die1("임시 파일 쓰기 실패, 쓰지 않음")
		}
		_, werr := f.Write(data)
		if cerr := f.Close(); werr != nil || cerr != nil {
			die1("임시 파일 쓰기 실패, 쓰지 않음")
		}
		return
	}
	perm := os.FileMode(0o600)
	if fi, err := os.Stat(S); err == nil {
		perm = fi.Mode().Perm()
	}
	if err := atomic.WriteFile(S, data, perm); err != nil {
		die1("임시 파일 쓰기 실패, 쓰지 않음")
	}
}

// copyPreserve: cp -p (권한·수정 시각)
func copyPreserve(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, fi.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Chmod(dst, fi.Mode().Perm()); err != nil {
		return err
	}
	return os.Chtimes(dst, fi.ModTime(), fi.ModTime())
}
