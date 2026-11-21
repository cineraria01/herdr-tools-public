// Package pyjson: 파이썬 json.dumps 기본 꼴(구분 「, 」「: 」, 키 순서 그대로)로 찍는다 — 파이썬 판 도구의 출력·상태 파일·
// 인자 글자를 Go 판에서 같게(2026-11-21 사용자 「모두 고로 옮기고 싶어」). ascii=true는 ensure_ascii(기본), false는 한글 그대로.
// 읽기는 Decode(숫자는 json.Number로 — 받은 글자 그대로 다시 찍게).
package pyjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"
)

// KV·Obj: 순서가 있는 객체(파이썬 dict 넣은 순서)
type KV struct {
	K string
	V any
}

// Obj: 순서가 있는 객체
type Obj []KV

// Decode: JSON 하나를 any로(숫자는 json.Number). 뒤에 JSON 공백(빈칸·탭·줄바꿈·CR) 말고 무엇이든 남으면 오류(파이썬 json.loads 「Extra data」)
func Decode(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := d.Decode(v); err != nil {
		return err
	}
	if rest := bytes.TrimLeft(b[d.InputOffset():], " \t\n\r"); len(rest) > 0 {
		return fmt.Errorf("JSON 뒤에 다른 값이 있다(%d바이트째)", len(b)-len(rest))
	}
	return nil
}

// Dumps: json.dumps(v, ensure_ascii=ascii)와 같은 글자
func Dumps(v any, ascii bool) string {
	var b strings.Builder
	dump(&b, v, ascii)
	return b.String()
}

func dump(b *strings.Builder, v any, ascii bool) {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		Quote(b, x, ascii)
	case json.Number:
		b.WriteString(x.String())
	case int:
		fmt.Fprint(b, x)
	case float64:
		fmt.Fprint(b, x)
	case []string:
		b.WriteByte('[')
		for i, s := range x {
			if i > 0 {
				b.WriteString(", ")
			}
			Quote(b, s, ascii)
		}
		b.WriteByte(']')
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteString(", ")
			}
			dump(b, e, ascii)
		}
		b.WriteByte(']')
	case Obj:
		b.WriteByte('{')
		for i, kv := range x {
			if i > 0 {
				b.WriteString(", ")
			}
			Quote(b, kv.K, ascii)
			b.WriteString(": ")
			dump(b, kv.V, ascii)
		}
		b.WriteByte('}')
	case map[string]any: // 받은 객체를 다시 찍을 때 — 파이썬은 받은 순서지만 Go 맵은 순서가 없어 키 순서로
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		o := make(Obj, 0, len(keys))
		for _, k := range keys {
			o = append(o, KV{k, x[k]})
		}
		dump(b, o, ascii)
	default:
		panic(fmt.Sprintf("pyjson: 모르는 형 %T", v))
	}
}

// Quote: 파이썬 json 문자열 꼴
func Quote(b *strings.Builder, s string, ascii bool) {
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20:
			fmt.Fprintf(b, `\u%04x`, r)
		case ascii && r > 0xffff:
			a, c := utf16.EncodeRune(r)
			fmt.Fprintf(b, `\u%04x\u%04x`, a, c)
		case ascii && r >= 0x7f: // 파이썬 ensure_ascii는 0x7f(DEL)도 \u007f
			fmt.Fprintf(b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}
