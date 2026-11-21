// Package ojson: 키 순서를 지키는 JSON 값과 jq(1.7) 꼴 출력. herdr-settings-merge·herdr-sync Go 판이 jq 대신 쓴다.
//   - Canon은 `jq -cS`(키 정렬·한 줄), Pretty는 `jq .`(두 칸 들여쓰기, 키 순서 그대로)와 같은 바이트를 낸다 — 셸 판이 쓴
//     base·refs의 cksum(`jq -cS` 한 줄)과 Go 판이 계산한 값이 같아야 두 판이 같은 상태 파일을 번갈아 읽는다.
//   - 숫자는 글자 그대로 받아 jq 1.7처럼 decNumber 꼴로 낸다(1.0 → 1.0, 1e2 → 1E+2).
//   - 값: nil · bool · json.Number · string · []any · *Object
package ojson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Object: 키 순서를 지키는 JSON 객체(같은 키를 다시 쓰면 첫 자리에 새 값 — jq와 같음)
type Object struct {
	Keys []string
	M    map[string]any
}

// NewObject: 빈 객체
func NewObject() *Object { return &Object{M: map[string]any{}} }

// Get: 키 값과 있음
func (o *Object) Get(k string) (any, bool) {
	v, ok := o.M[k]
	return v, ok
}

// Set: 키 값(없으면 끝에 더함)
func (o *Object) Set(k string, v any) {
	if _, ok := o.M[k]; !ok {
		o.Keys = append(o.Keys, k)
	}
	o.M[k] = v
}

// Del: 키 지움
func (o *Object) Del(k string) {
	if _, ok := o.M[k]; !ok {
		return
	}
	delete(o.M, k)
	for i, x := range o.Keys {
		if x == k {
			o.Keys = append(o.Keys[:i:i], o.Keys[i+1:]...)
			break
		}
	}
}

// Parse: JSON 값 하나(앞뒤 빈칸만 허용 — 값이 둘 이상이면 오류)
func Parse(b []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	v, err := parseValue(d)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("JSON 값 뒤에 남은 것이 있다")
	}
	return v, nil
}

func parseValue(d *json.Decoder) (any, error) {
	t, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch x := t.(type) {
	case json.Delim:
		switch x {
		case '{':
			o := NewObject()
			for d.More() {
				kt, err := d.Token()
				if err != nil {
					return nil, err
				}
				v, err := parseValue(d)
				if err != nil {
					return nil, err
				}
				o.Set(kt.(string), v)
			}
			if _, err := d.Token(); err != nil {
				return nil, err
			}
			return o, nil
		case '[':
			a := []any{}
			for d.More() {
				v, err := parseValue(d)
				if err != nil {
					return nil, err
				}
				a = append(a, v)
			}
			if _, err := d.Token(); err != nil {
				return nil, err
			}
			return a, nil
		}
		return nil, fmt.Errorf("뜻밖의 %v", x)
	default:
		return x, nil // nil·bool·json.Number·string
	}
}

// Type: jq type 이름
func Type(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case json.Number:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case *Object:
		return "object"
	}
	return "?"
}

// Canon: `jq -cS` 한 줄(줄바꿈 없음)
func Canon(v any) string {
	var b strings.Builder
	write(&b, v, true, "", "")
	return b.String()
}

// Compact: `jq -c` 한 줄(키 순서 그대로)
func Compact(v any) string {
	var b strings.Builder
	write(&b, v, false, "", "")
	return b.String()
}

// Pretty: `jq .` 꼴(끝 줄바꿈 포함)
func Pretty(v any) string {
	var b strings.Builder
	write(&b, v, false, "  ", "")
	b.WriteByte('\n')
	return b.String()
}

// Sorted: 객체 키를 모두 정렬한 깊은 사본(`jq -cS` 결과를 다시 읽은 것과 같음)
func Sorted(v any) any {
	switch x := v.(type) {
	case *Object:
		o := NewObject()
		ks := append([]string(nil), x.Keys...)
		sort.Strings(ks)
		for _, k := range ks {
			o.Set(k, Sorted(x.M[k]))
		}
		return o
	case []any:
		a := make([]any, len(x))
		for i, e := range x {
			a[i] = Sorted(e)
		}
		return a
	}
	return v
}

func write(b *strings.Builder, v any, sorted bool, ind, cur string) {
	nl := func(c string) {
		if ind != "" {
			b.WriteByte('\n')
			b.WriteString(c)
		}
	}
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case json.Number:
		b.WriteString(Number(string(x)))
	case string:
		writeString(b, x)
	case []any:
		if len(x) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			nl(cur + ind)
			write(b, e, sorted, ind, cur+ind)
		}
		nl(cur)
		b.WriteByte(']')
	case *Object:
		if len(x.Keys) == 0 {
			b.WriteString("{}")
			return
		}
		ks := x.Keys
		if sorted {
			ks = append([]string(nil), ks...)
			sort.Strings(ks)
		}
		b.WriteByte('{')
		for i, k := range ks {
			if i > 0 {
				b.WriteByte(',')
			}
			nl(cur + ind)
			writeString(b, k)
			b.WriteByte(':')
			if ind != "" {
				b.WriteByte(' ')
			}
			write(b, x.M[k], sorted, ind, cur+ind)
		}
		nl(cur)
		b.WriteByte('}')
	}
}

// writeString: jq와 같은 이스케이프 — " \ 와 \b \f \n \r \t, 나머지 0x00–0x1F·0x7F는 \u00xx, 그 밖(비 ASCII 포함)은 그대로
func writeString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 || c == 0x7f {
				fmt.Fprintf(b, `\u%04x`, c)
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
}

// Number: JSON 숫자 글자를 jq 1.7(decNumber to-scientific-string) 꼴로
func Number(s string) string {
	neg := strings.HasPrefix(s, "-")
	t := strings.TrimPrefix(s, "-")
	exp := 0
	if i := strings.IndexAny(t, "eE"); i >= 0 {
		e, err := strconv.Atoi(strings.TrimPrefix(t[i+1:], "+"))
		if err != nil {
			return s
		}
		exp, t = e, t[:i]
	}
	digits := t
	if i := strings.IndexByte(t, '.'); i >= 0 {
		digits = t[:i] + t[i+1:]
		exp -= len(t) - i - 1
	}
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		digits = "0"
	}
	sign := ""
	if neg {
		sign = "-"
	}
	adj := exp + len(digits) - 1
	if exp <= 0 && adj >= -6 {
		if exp == 0 {
			return sign + digits
		}
		if p := len(digits) + exp; p > 0 {
			return sign + digits[:p] + "." + digits[p:]
		}
		return sign + "0." + strings.Repeat("0", -exp-len(digits)) + digits
	}
	r := digits[:1]
	if len(digits) > 1 {
		r += "." + digits[1:]
	}
	es := "+"
	if adj < 0 {
		es = "-"
	}
	if adj < 0 {
		adj = -adj
	}
	return sign + r + "E" + es + strconv.Itoa(adj)
}

// ErrIndex: 객체가 아닌 값을 키로 읽거나 쓰려 했다(jq 「Cannot index」)
var ErrIndex = errors.New("객체가 아닌 값을 키로 다룰 수 없다")

// GetPath: jq getpath(키 목록). null 아래는 null, 객체가 아니면 ErrIndex
func GetPath(v any, p []string) (any, error) {
	for _, k := range p {
		switch x := v.(type) {
		case nil:
			return nil, nil
		case *Object:
			v = x.M[k]
		default:
			return nil, ErrIndex
		}
	}
	return v, nil
}

// SetPath: jq setpath — 없는 중간은 객체로 만든다. 고친 값을 돌려준다
func SetPath(v any, p []string, x any) (any, error) {
	if len(p) == 0 {
		return x, nil
	}
	var o *Object
	switch y := v.(type) {
	case nil:
		o = NewObject()
	case *Object:
		o = y
	default:
		return nil, ErrIndex
	}
	c, err := SetPath(o.M[p[0]], p[1:], x)
	if err != nil {
		return nil, err
	}
	o.Set(p[0], c)
	return o, nil
}

// DelPath: jq delpaths([p]) — 없으면 그대로
func DelPath(v any, p []string) (any, error) {
	if len(p) == 0 {
		return nil, nil
	}
	switch y := v.(type) {
	case nil:
		return nil, nil
	case *Object:
		if len(p) == 1 {
			y.Del(p[0])
			return y, nil
		}
		c, ok := y.M[p[0]]
		if !ok {
			return y, nil
		}
		c, err := DelPath(c, p[1:])
		if err != nil {
			return nil, err
		}
		y.M[p[0]] = c
		return y, nil
	}
	return nil, ErrIndex
}

// Walk: 값 안의 문자열마다 f(객체 키는 그대로 — jq walk와 같음)
func Walk(v any, f func(string) string) any {
	switch x := v.(type) {
	case string:
		return f(x)
	case []any:
		for i, e := range x {
			x[i] = Walk(e, f)
		}
	case *Object:
		for _, k := range x.Keys {
			x.M[k] = Walk(x.M[k], f)
		}
	}
	return v
}

// Copy: 깊은 사본
func Copy(v any) any {
	switch x := v.(type) {
	case []any:
		a := make([]any, len(x))
		for i, e := range x {
			a[i] = Copy(e)
		}
		return a
	case *Object:
		o := &Object{Keys: append([]string(nil), x.Keys...), M: make(map[string]any, len(x.M))}
		for k, e := range x.M {
			o.M[k] = Copy(e)
		}
		return o
	}
	return v
}
