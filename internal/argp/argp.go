// Package argp: 파이썬 argparse를 쓰던 도구를 Go로 옮길 때 인자 꼴을 같게 — 긴 옵션(--이름, --이름=값, 하나뿐인 앞부분 줄임),
// -h·--help, 값 개수(없음·하나·0개 이상·1개 이상), 같은 옵션을 두 번 주면 뒤 것, 오류는 usage + 「<도구>: error: …」 + 종료 2
// (2026-11-21 사용자 「모두 고로 옮기고 싶어」). usage·help 글은 파이썬 판이 80열에서 찍던 것을 도구가 그대로 준다.
// ponytail: 파이썬 판의 오류 문구는 파이썬 판마다도 다르다(3.15는 이상한 인자 뒤 전부를 unrecognized로 찍음) — 꼴과 종료 코드만 맞춘다.
package argp

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Kind: 옵션이 받는 값 개수
type Kind int

const (
	Flag Kind = iota // 값 없음(store_true)
	One              // 값 하나
	Star             // nargs="*"
	Plus             // nargs="+"
)

// Opt: 긴 옵션 하나(이름은 -- 뺀 것)
type Opt struct {
	Name string
	Kind Kind
}

// Parser: 도구 하나의 인자 규칙
type Parser struct {
	Prog, Usage, Help string // Usage는 「usage: …」 줄(들), Help는 -h가 찍는 전부. 둘 다 끝 줄바꿈 포함
	Opts              []Opt
	Stdout, Stderr    io.Writer // 비면 os.Stdout·os.Stderr
	Exit              func(int) // 비면 os.Exit
}

// Result: 받은 옵션. Has는 준 옵션, Vals는 그 값(Flag는 빈 목록)
type Result struct {
	Has  map[string]bool
	Vals map[string][]string
}

// Get: One 옵션의 값(없으면 빈 값)
func (r Result) Get(name string) string {
	if v := r.Vals[name]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func (p *Parser) out(w io.Writer, std *os.File) io.Writer {
	if w != nil {
		return w
	}
	return std
}

func (p *Parser) exit(code int) {
	if p.Exit != nil {
		p.Exit(code)
		return
	}
	os.Exit(code)
}

// Error: usage와 오류 줄을 stderr에 찍고 종료 2
func (p *Parser) Error(msg string) {
	w := p.out(p.Stderr, os.Stderr)
	fmt.Fprint(w, p.Usage)
	fmt.Fprintf(w, "%s: error: %s\n", p.Prog, msg)
	p.exit(2)
}

// 옵션처럼 보이는 낱말: -로 시작하고 「-」 하나나 음수가 아님
func optLike(s string) bool {
	if len(s) < 2 || s[0] != '-' {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err != nil
}

func (p *Parser) find(name string) (Opt, bool, string) {
	all := append([]Opt{{Name: "help", Kind: Flag}}, p.Opts...)
	for _, o := range all {
		if o.Name == name {
			return o, true, ""
		}
	}
	var hit []string
	var got Opt
	for _, o := range all {
		if strings.HasPrefix(o.Name, name) {
			hit = append(hit, "--"+o.Name)
			got = o
		}
	}
	if len(hit) == 1 {
		return got, true, ""
	}
	if len(hit) > 1 {
		return Opt{}, false, fmt.Sprintf("ambiguous option: --%s could match %s", name, strings.Join(hit, ", "))
	}
	return Opt{}, false, ""
}

// Parse: 인자를 읽는다. 오류·도움말이면 Exit을 부르고(시험에서 Exit이 돌아오면) ok=false
func (p *Parser) Parse(args []string) (r Result, ok bool) {
	r = Result{Has: map[string]bool{}, Vals: map[string][]string{}}
	var extra []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			extra = append(extra, args[i+1:]...)
			break
		}
		if a == "-h" {
			a = "--help"
		}
		if !strings.HasPrefix(a, "--") || len(a) == 2 {
			extra = append(extra, a)
			continue
		}
		name, val, eq := strings.Cut(a[2:], "=")
		o, found, amb := p.find(name)
		if amb != "" {
			p.Error(amb)
			return r, false
		}
		if !found {
			extra = append(extra, a)
			continue
		}
		var vals []string
		switch o.Kind {
		case Flag:
			if eq {
				p.Error(fmt.Sprintf("argument --%s: ignored explicit argument '%s'", o.Name, val))
				return r, false
			}
		case One:
			if eq {
				vals = []string{val}
			} else if i+1 < len(args) && !optLike(args[i+1]) {
				i++
				vals = []string{args[i]}
			} else {
				p.Error(fmt.Sprintf("argument --%s: expected one argument", o.Name))
				return r, false
			}
		case Star, Plus:
			if eq {
				vals = []string{val}
			} else {
				for i+1 < len(args) && !optLike(args[i+1]) && args[i+1] != "--" {
					i++
					vals = append(vals, args[i])
				}
			}
			if o.Kind == Plus && len(vals) == 0 {
				p.Error(fmt.Sprintf("argument --%s: expected at least one argument", o.Name))
				return r, false
			}
		}
		if o.Name == "help" {
			fmt.Fprint(p.out(p.Stdout, os.Stdout), p.Help)
			p.exit(0)
			return r, false
		}
		r.Has[o.Name] = true
		r.Vals[o.Name] = vals // 두 번 주면 뒤 것(argparse store)
	}
	if len(extra) > 0 {
		p.Error("unrecognized arguments: " + strings.Join(extra, " "))
		return r, false
	}
	return r, true
}
