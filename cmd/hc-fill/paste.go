package main

// 지휘 기계 파일 붙이기(Go 판 — 지휘가 지시문에 맥 경로를 적어 워커 기계가 못 읽었다). launch가 다른 기계에 띄울 때 부른다

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	lcPasteFileMax = 64 << 10
	lcPasteAllMax  = 256 << 10
)

// 비밀 꼴 판정은 secret.go(lcSecretHit) — 실제 값 모양만 본다(문서 속 「API_KEY」 같은 이름만으로는 막지 않는다)

func lcBlank(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f'
}

// lcPasteScan: 본문이 가리키는 이 기계 붙일 폴더(lcPasteDirs) 아래 일반 파일(<HOME>/… 또는 ~/…)의 원문 절. 막힐 때는 shown(지시문에 적힌 꼴)·abs·why.
// 읽히지 않거나 일반 파일이 아닌 경로는 그냥 건너뛴다. 64KiB 넘는 파일·바이너리(NUL)·합계 256KiB를 넘기는 파일은 경로만 적는다
// (막는 것은 비밀 꼴뿐). 같은 경로는 한 번만
func lcPasteScan(body string) (paste, shown, abs, why string) {
	home := strings.TrimRight(HOME, "/")
	if home == "" {
		return "", "", "", ""
	}
	dirs := lcPasteDirs(home)
	seen := map[string]bool{}
	var b strings.Builder
	total := 0
	for _, tok := range lcPasteToks(body) {
		x := ovUnwrap(tok)
		cands := []string{x}
		if loc := ovParticleRe.FindStringIndex(x); loc != nil {
			cands = append(cands, ovUnwrap(x[:loc[0]+1]))
		}
		for _, c := range cands {
			var p string
			switch {
			case strings.HasPrefix(c, "~/"):
				p = home + c[1:]
			case strings.HasPrefix(c, home+"/"):
				p = c
			default:
				continue
			}
			p = filepath.Clean(p)
			// hc-fill 데이터 폴더(runs·reports·verdicts…)는 레인 기계 자기 파일을 가리킨다 — 지휘 기계 것을 붙이지 않는다
			if !strings.HasPrefix(p, home+"/") || strings.HasPrefix(p, home+"/.local/share/hc-fill/") || DATA != "" && strings.HasPrefix(p, DATA+"/") {
				continue
			}
			// 붙일 폴더 안인지는 링크를 다 푼 실제 경로로 본다(폴더 안 링크가 ~/.netrc 등 밖을 가리키면 붙이지 않는다 — gofix-fill F07)
			real, err := filepath.EvalSymlinks(p)
			if err != nil || !lcUnder(real, dirs) {
				continue
			}
			st, err := os.Stat(real)
			if err != nil || !st.Mode().IsRegular() {
				continue
			}
			if seen[p] {
				break
			}
			seen[p] = true
			// 큰 파일·바이너리(설명으로 적은 Go 바이너리 경로 등)는 붙이지 않고 경로만 — 레인은 띄운다(2026-11-21 go-launchd-tcc가
			// 5.5MB 바이너리 경로에 걸려 아예 안 뜸). 비밀 꼴만 띄우지 않는 까닭이다
			pathOnly := "\n## (붙임) 지휘 기계 파일 — " + c + "\n(붙이지 않음: 큼/바이너리 — 경로만)\n"
			if st.Size() > lcPasteFileMax {
				b.WriteString(pathOnly)
				break
			}
			data, err := os.ReadFile(real)
			if err != nil {
				break // 읽히지 않으면 붙이지 않는다
			}
			if bytes.IndexByte(data, 0) >= 0 || total+len(data) > lcPasteAllMax {
				b.WriteString(pathOnly)
				break
			}
			total += len(data)
			if k, n, nm := lcSecretHit(data); k != "" {
				return "", c, p, lcSecretWhy(k, n, nm)
			}
			b.WriteString("\n## (붙임) 지휘 기계 파일 원문 — " + c + "\n" + string(data))
			break
		}
	}
	return b.String(), "", "", ""
}

var lcQuotedRe = regexp.MustCompile("`[^`\n]+`|\"[^\"\n]+\"|'[^'\n]+'")

// lcPasteToks: 본문의 낱말(나온 차례대로) — 백틱·따옴표로 묶은 것은 공백이 들었으면 묶음 전체를 한 항목으로 먼저(「감사 자료.md」, R-25),
// 이어 그 안 낱말도. 묶지 않은 곳은 공백으로 나눈다
func lcPasteToks(body string) []string {
	var o []string
	split := func(s string) { o = append(o, strings.FieldsFunc(strings.ReplaceAll(s, "`", " "), lcBlank)...) }
	last := 0
	for _, m := range lcQuotedRe.FindAllStringIndex(body, -1) {
		split(body[last:m[0]])
		in := strings.TrimFunc(body[m[0]+1:m[1]-1], lcBlank)
		if strings.IndexFunc(in, lcBlank) >= 0 {
			o = append(o, in)
		}
		split(in)
		last = m[1]
	}
	split(body[last:])
	return o
}

// lcPasteDirs: 붙일 수 있는 폴더(링크를 푼 실제 경로) — 설정 `paste_dirs <폴더…>`(~/ 꼴 가능), 없으면 ~/.claude-tasks(지휘가 보고·지시를 두는 곳).
// 없는 폴더는 뺀다. HOME 밖 폴더는 쓰지 않는다
func lcPasteDirs(home string) []string {
	v := conf("paste_dirs")
	if v == "" {
		v = "~/.claude-tasks"
	}
	var ds []string
	for _, d := range words(v) {
		if strings.HasPrefix(d, "~/") || d == "~" {
			d = home + d[1:]
		}
		d = filepath.Clean(d)
		if d != home && !strings.HasPrefix(d, home+"/") {
			continue
		}
		if r, err := filepath.EvalSymlinks(d); err == nil {
			ds = append(ds, r)
		}
	}
	return ds
}

// lcUnder: p가 dirs 가운데 하나 아래(그 폴더 자신은 아님)
func lcUnder(p string, dirs []string) bool {
	for _, d := range dirs {
		if strings.HasPrefix(p, strings.TrimRight(d, "/")+"/") {
			return true
		}
	}
	return false
}

// lcPasteHome <본문> <주인|-> <레인>: 붙일 글. 상한·비밀 꼴이면 한 줄과 지휘 알림 한 번(키 hf:<레인>:paste:<경로 cksum>), ok=false
func lcPasteHome(body, owner, lane string) (string, bool) {
	paste, shown, abs, why := lcPasteScan(body)
	if why == "" {
		return paste, true
	}
	msg := fmt.Sprintf("실패 %s: 지시문이 가리키는 지휘 기계 파일 %s가 %s — 원문을 붙이지 못해 띄우지 않음", lane, shown, why)
	say("%s", msg)
	aesc(lane, owner, msg, fmt.Sprintf("hf:%s:paste:%d", lane, cksum([]byte(abs))))
	return "", false
}
