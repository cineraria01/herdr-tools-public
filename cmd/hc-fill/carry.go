package main

// 지시문이 가리키는 지휘 기계 자료 싣기(2026-11-26 ht-fill-carry-qa — 지휘가 지시문에 지휘 기계 저장소의 .qa/…(git이 따라가지 않는 자료)와
// 지휘 기계 절대 경로를 적어, 다른 기계 새 작업 트리에서 뜬 레인이 그 파일을 못 읽고 멈췄다).
// add·edit·--then: 저장소 .qa 절대 경로는 상대 .qa/…로, 저장소 밖 파일은 데이터 폴더 carry/<레인>/에 사본을 두고 .qa/carry/<이름>으로 바꿔 쓴다.
// launch: 지시문의 .qa/… 자료를 tar 한 묶음으로 만들어 hc-lane --carry로 넘기면, hc-lane이 작업 트리를 만든 직후 그 자리에 푼다

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// carryMax: 실을 tar 묶음 상한. hc exec -i가 입력을 8MB에서 자르므로 한 번에 보낼 수 있는 만큼(herdr-sync capOf와 같은 값).
// ponytail: 한 묶음 상한 — 큰 시안이 잦으면 묶음을 나눠 보내기
const carryMax = 7340032

// carryToks: 본문에서 경로 후보(나온 차례). 낱말마다 감싼 글자를 벗기고, 조사가 붙었으면 뗀 꼴도 함께
func carryToks(body string) [][]string {
	var o [][]string
	for _, tok := range lcPasteToks(body) {
		for _, w := range strings.Fields(ovMSep.Replace(tok)) {
			x := ovUnwrap(w)
			c := []string{x}
			if loc := ovParticleRe.FindStringIndex(x); loc != nil {
				c = append(c, ovUnwrap(x[:loc[0]+1]))
			}
			o = append(o, c)
		}
	}
	return o
}

// carryRel: .qa/… 상대 경로(./ 앞붙임 허용)를 깨끗한 꼴로. 저장소 밖으로 나가면 ""
func carryRel(x string) string {
	x = strings.TrimPrefix(x, "./")
	if !strings.HasPrefix(x, ".qa/") {
		return ""
	}
	c := filepath.Clean(x)
	if !strings.HasPrefix(c, ".qa/") {
		return ""
	}
	return c
}

// carryAbs: ~/… 또는 /… 를 이 기계 절대 경로로, 아니면 ""
func carryAbs(x string) string {
	switch {
	case strings.HasPrefix(x, "~/") && HOME != "":
		return filepath.Clean(strings.TrimRight(HOME, "/") + x[1:])
	case strings.HasPrefix(x, "/"):
		return filepath.Clean(x)
	}
	return ""
}

// carryInGit: p가 어느 git 저장소 안인가(위 폴더에 .git이 있나) — 다른 저장소 파일은 그 저장소 일이라 싣지 않는다
func carryInGit(p string) bool {
	for d := filepath.Dir(p); ; d = filepath.Dir(d) {
		if _, err := os.Lstat(d + "/.git"); err == nil {
			return true
		}
		if d == "/" || d == "." {
			return false
		}
	}
}

// carryRoot: 실을 수 있는 곳 — 집 아래와 임시 폴더(지휘 scratchpad)뿐. /bin/sh 같은 시스템 파일은 레인 기계에도 있다.
// 실행 파일(도구)은 부르는 경로라 싣지 않는다(carryPrep)
func carryRoot(real string) bool {
	rs := []string{"/private/tmp", "/tmp", "/private/var/folders", "/var/folders"}
	for _, d := range []string{HOME, os.TempDir()} { // 이 프로세스의 TMPDIR(메모리 디스크 등)도
		if d = strings.TrimRight(d, "/"); d != "" {
			if r, err := filepath.EvalSymlinks(d); err == nil {
				d = r
			}
			rs = append(rs, d)
		}
	}
	for _, d := range rs {
		if d != "" && strings.HasPrefix(real, d+"/") {
			return true
		}
	}
	return false
}

// carryDot: 경로 마디 가운데 .으로 시작하는 것이 있나(설정·비밀·hc-fill 데이터 폴더 — 싣지 않는다. ~/.claude-tasks는 붙이기 paste.go 몫)
func carryDot(p string) bool {
	for _, s := range strings.Split(p, "/") {
		if strings.HasPrefix(s, ".") {
			return true
		}
	}
	return false
}

// carryReplace: 본문에서 old 경로를 new로 — 앞뒤가 경로 글자가 아닌 자리만(/a/x.py가 /a/x.py.orig 안에서 바뀌지 않게)
func carryReplace(body, old, nw string) string {
	pc := func(b byte) bool {
		return b == '/' || b == '.' || b == '_' || b == '-' || b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
	}
	var b strings.Builder
	for {
		i := strings.Index(body, old)
		if i < 0 {
			b.WriteString(body)
			return b.String()
		}
		j := i + len(old)
		if (i > 0 && pc(body[i-1])) || (j < len(body) && pc(body[j]) && !(body[j] == '.' && (j+1 == len(body) || !pc(body[j+1])))) {
			b.WriteString(body[:j])
		} else {
			b.WriteString(body[:i] + nw)
		}
		body = body[j:]
	}
}

// carryPrep <레인> <프로젝트> <지시문 글> <이미 쓴 이름> <add 때>: 경고를 찍고 바꿔 쓴 글과 실을 저장소 밖 파일(이 기계 경로 → carry/<레인>/ 안 이름).
// taken: 데이터 폴더에 이미 있는 사본 이름(--then·띄울 때 — 같은 내용이면 그 이름을 다시 쓴다). 없는 .qa 경고는 add 때만(띄울 때는 carryTar가 알림)
func carryPrep(lane, prj string, b []byte, taken map[string]bool, atAdd bool) ([]byte, map[string]string) {
	repo := strings.TrimRight(repoOf(prj, LOCAL), "/")
	body := string(b)
	ext := map[string]string{}
	mine := map[string]string{} // 이번에 붙인 이름 → 이 기계 경로
	seen := map[string]bool{}
	for _, cands := range carryToks(body) {
		for _, c := range cands {
			if seen[c] {
				break
			}
			last := c == cands[len(cands)-1] // 없으면 조사를 뗀 꼴(다음 후보)을 본다(감사 atp3u25 낮음 1)
			if rel := carryRel(c); rel != "" {
				if repo != "" && !lcE(repo+"/"+rel) && !strings.HasPrefix(rel, ".qa/carry/") {
					if !last {
						continue
					}
					if atAdd {
						say("경고: 지시문의 %s 가 %s 에 없다 — 레인 작업 트리에 실리지 않는다", c, repo)
					}
				}
				seen[c] = true
				break
			}
			p := carryAbs(c)
			if p == "" {
				continue
			}
			if repo != "" && strings.HasPrefix(p, repo+"/.qa/") {
				if !lcE(p) {
					if !last {
						continue
					}
					if atAdd {
						say("경고: 지시문의 %s 가 없다 — 레인 작업 트리에 실리지 않는다", c)
					}
				}
				seen[c] = true
				body = carryReplace(body, c, strings.TrimPrefix(p, repo+"/"))
				break
			}
			real, err := filepath.EvalSymlinks(p)
			if err != nil {
				continue
			}
			st, err := os.Stat(real)
			if err != nil || !st.Mode().IsRegular() || st.Mode()&0o111 != 0 || !carryRoot(real) || (repo != "" && strings.HasPrefix(real, repo+"/")) ||
				carryDot(real) || carryDot(p) || carryInGit(real) {
				continue
			}
			seen[c] = true
			if st.Size() > carryMax {
				say("경고: %s — 다른 기계 레인은 이 파일을 못 읽는다, 실어 보내기 상한(%d바이트)을 넘어 싣지 않음 — .qa로 옮겨라", c, carryMax)
				break
			}
			data, err := os.ReadFile(real)
			if err != nil {
				break
			}
			if k, n, nm := lcSecretHit(data); k != "" {
				say("경고: %s — 다른 기계 레인은 이 파일을 못 읽는다, %s라 싣지 않음", c, lcSecretWhy(k, n, nm))
				break
			}
			nm := filepath.Base(real)
			for i := 2; ; i++ {
				if o, ok := mine[nm]; ok && o == real || !ok && (!taken[nm] || carrySame(DATA+"/carry/"+lane+"/"+nm, data)) {
					break
				}
				nm = strconv.Itoa(i) + "-" + filepath.Base(real)
			}
			mine[nm], ext[real] = real, nm
			say("경고: %s — 다른 기계 레인은 이 파일을 못 읽는다 — .qa로 옮겨라 (이번에는 사본을 .qa/carry/%s 로 실어 보냄)", c, nm)
			body = carryReplace(body, c, ".qa/carry/"+nm)
			break
		}
	}
	return []byte(body), ext
}

// carrySame: 그 사본이 data와 같은가(띄울 때마다 같은 파일에 새 이름을 붙이지 않게)
func carrySame(p string, data []byte) bool {
	b, err := os.ReadFile(p)
	return err == nil && bytes.Equal(b, data)
}

// carryKeep <레인> <파일들> <새로>: 저장소 밖 파일 사본을 데이터 폴더 carry/<레인>/에. 새로면 옛 사본을 먼저 비운다(add·edit)
func carryKeep(lane string, ext map[string]string, fresh bool) {
	d := DATA + "/carry/" + lane
	if fresh {
		os.RemoveAll(d)
	}
	if len(ext) == 0 {
		return
	}
	for src, nm := range ext {
		b, err := os.ReadFile(src)
		if err == nil {
			err = os.MkdirAll(d, 0o755)
		}
		if err == nil {
			err = writeAtomic(d+"/"+nm, string(b))
		}
		if err != nil {
			say("경고: %s 사본을 두지 못했다(%s) — 레인 작업 트리에 실리지 않는다", src, errText(err))
		}
	}
}

// carryWrite <사본> <레인> <프로젝트> <글>: 경로를 바꿔 쓴 지시문 사본을 두고 저장소 밖 파일 사본을 새로(edit)
func carryWrite(bf, lane, prj string, b []byte) error {
	b, ext := carryPrep(lane, prj, b, map[string]bool{}, true)
	carryCheck(lane, prj, b, ext, true) // 앞 지시문을 덮기 전에
	if err := writeAtomic(bf, string(b)); err != nil {
		return err
	}
	carryKeep(lane, ext, true)
	return nil
}

// carryTaken <레인>: 이미 둔 사본 이름(--then이 이어 더할 때)
func carryTaken(lane string) map[string]bool {
	t := map[string]bool{}
	es, _ := os.ReadDir(DATA + "/carry/" + lane)
	for _, e := range es {
		t[e.Name()] = true
	}
	return t
}

// carryGroup: 지시문에 적힌 경로 하나가 실을 몫(넣을 때 거절 글의 큰 것 목록)
type carryGroup struct {
	path string
	n    int
	est  int64
}

// carrySet <레인> <프로젝트> <본문> <미룬 사본> <새로>: 본문의 .qa/… 자료(작업 트리 상대 이름 → 이 기계 경로)와 묶음 어림 크기·경로별 몫·지휘 기계에 없는 경로.
// 넣을 때(carryCheck)와 띄울 때(carryTar)가 이 한 셈을 쓴다 — 두 판정이 어긋나지 않게(2026-11-28 ht-carry-add-check).
// pend: 아직 데이터 폴더에 두지 않은 사본(이름 → 이 기계 경로, add·edit·--then 때). fresh: 데이터 폴더 옛 사본을 보지 않음(add·edit는 새로 비운다)
func carrySet(lane, prj, body string, pend map[string]string, fresh bool) (map[string]string, int64, []carryGroup, []string) {
	repo := strings.TrimRight(repoOf(prj, LOCAL), "/")
	files := map[string]string{}
	var gs []carryGroup
	var miss []string
	est := int64(1024) // 읽기 전에 크기로 어림(머리 PAX 포함 넉넉히 1,536바이트 + 512 단위 올림, 끝 1,024)
	for _, cands := range carryToks(body) {
		for _, c := range cands {
			rel := carryRel(c)
			if rel == "" {
				continue
			}
			src, label := "", rel
			nm, isc := strings.CutPrefix(rel, ".qa/carry/")
			if k := DATA + "/carry/" + lane + "/" + nm; isc && pend[nm] != "" {
				src, label = pend[nm], pend[nm]
			} else if isc && !fresh && lcE(k) {
				src = k
			} else if repo != "" && lcE(repo+"/"+rel) {
				src = repo + "/" + rel
			}
			if src == "" {
				if c == cands[len(cands)-1] {
					miss = append(miss, rel)
				}
				continue
			}
			g := carryGroup{path: label}
			filepath.WalkDir(src, func(p string, e fs.DirEntry, err error) error {
				if err == nil && e.Type().IsRegular() {
					if fi, err := e.Info(); err == nil && files[rel+strings.TrimPrefix(p, src)] == "" {
						files[rel+strings.TrimPrefix(p, src)] = p
						g.n++
						g.est += 1536 + (fi.Size()+511)/512*512
					}
				}
				return nil
			})
			if g.n > 0 {
				est += g.est
				gs = append(gs, g)
			}
			break
		}
	}
	return files, est, gs, miss
}

// carryOver: 상한을 넘었을 때의 까닭(넣을 때·띄울 때 같은 글), 아니면 ""
func carryOver(n int, est int64) string {
	if est <= carryMax {
		return ""
	}
	return fmt.Sprintf("실어 보낼 자료가 상한 %d바이트를 넘음(파일 %d개 약 %d바이트)", carryMax, n, est)
}

// carryCheck <레인> <프로젝트> <바꿔 쓴 글> <저장소 밖 사본(carryPrep)> <새로>: 띄울 때 실릴 자료가 상한을 넘으면 넣기 전에 거절(die — 아무것도 쓰기 전).
// 띄울 때의 검사는 그대로 남는다(넣은 뒤 .qa가 커질 수 있다)
func carryCheck(lane, prj string, b []byte, ext map[string]string, fresh bool) {
	pend := map[string]string{}
	for real, nm := range ext {
		pend[nm] = real
	}
	files, est, gs, _ := carrySet(lane, prj, string(b), pend, fresh)
	why := carryOver(len(files), est)
	if why == "" {
		return
	}
	sort.SliceStable(gs, func(i, j int) bool { return gs[i].est > gs[j].est })
	msg := why + " — 넣지 않음"
	for i, g := range gs {
		if i == 3 {
			break
		}
		msg += fmt.Sprintf("\n  %s — 파일 %d개 약 %d바이트", g.path, g.n, g.est)
	}
	die("%s\n폴더 통째 대신 레인이 실제로 읽을 파일만 적어라(큰 자료는 저장소에 두거나 레인이 그 기계에서 만들게)", msg)
}

// carryTar <레인> <프로젝트> <본문>: 본문의 .qa/… 자료를 tar 한 묶음으로(작업 트리 상대 이름). 없으면 nil.
// .qa/carry/<이름>은 데이터 폴더 사본이 먼저, 없으면 저장소. 없는 자료는 알림 한 줄. 상한을 넘으면 why
func carryTar(lane, prj, body string) ([]byte, string) {
	files, est, _, miss := carrySet(lane, prj, body, nil, false)
	for _, rel := range miss {
		say("알림 %s: 지시문의 %s 가 지휘 기계에 없어 싣지 않음", lane, rel)
	}
	if len(files) == 0 {
		return nil, ""
	}
	// 읽기 전에 크기로 판정 — 큰 폴더를 가리켜도 통째로 메모리에 올리지 않는다
	if why := carryOver(len(files), est); why != "" {
		return nil, why
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, n := range names {
		b, err := os.ReadFile(files[n])
		if err != nil {
			say("알림 %s: %s 를 읽지 못해 싣지 않음(%s)", lane, files[n], errText(err))
			continue
		}
		mode := int64(0o644)
		if st, err := os.Stat(files[n]); err == nil && st.Mode()&0o111 != 0 {
			mode = 0o755
		}
		if tw.WriteHeader(&tar.Header{Name: n, Mode: mode, Size: int64(len(b)), Format: tar.FormatPAX}) != nil || func() error { _, e := tw.Write(b); return e }() != nil {
			return nil, "tar 묶음을 만들지 못함"
		}
		if buf.Len() > carryMax {
			return nil, fmt.Sprintf("실어 보낼 자료가 상한 %d바이트를 넘음(%s 까지 %d바이트)", carryMax, n, buf.Len())
		}
	}
	if tw.Close() != nil {
		return nil, "tar 묶음을 만들지 못함"
	}
	if buf.Len() > carryMax {
		return nil, fmt.Sprintf("실어 보낼 자료가 상한 %d바이트를 넘음(%d바이트)", carryMax, buf.Len())
	}
	return buf.Bytes(), ""
}

// lcCarry <레인> <프로젝트> <본문> <주인|->: hc-lane --carry에 넘길 tar 파일 경로("" 실을 것 없음). 상한 넘김이면 한 줄과 막힘 사건
// 한 번(키 hf:<레인>:carry:<크기>), ok=false — 띄우지 않는다
func lcCarry(lane, prj, body, owner string) (string, string, bool) {
	// 깔리기 전에 add된 줄은 사본이 옛 꼴 그대로다 — 띄울 때도 바꿔 쓴다(감사 atp3u25 중간 3. 새 줄에는 바꿀 것이 없다)
	nb, ext := carryPrep(lane, prj, []byte(body), carryTaken(lane), false)
	carryKeep(lane, ext, false)
	body = string(nb)
	fail := func(why string) (string, string, bool) {
		msg := fmt.Sprintf("실패 %s: %s — 레인을 띄우지 않음", lane, why)
		say("%s", msg)
		aesc(lane, owner, msg, fmt.Sprintf("hf:%s:carry:%d", lane, cksum([]byte(why))))
		return body, "", false
	}
	tb, why := carryTar(lane, prj, body)
	if why != "" {
		return fail(why + "(.qa 자료를 줄이거나 나눠라)")
	}
	if tb == nil {
		return body, "", true
	}
	if err := os.MkdirAll(DATA+"/carry", 0o755); err != nil {
		return fail("실어 보낼 묶음 폴더를 만들지 못함(" + errText(err) + ")")
	}
	f, err := os.CreateTemp(DATA+"/carry", lane+".*.tar")
	if err == nil {
		_, err = f.Write(tb)
		if e := f.Close(); err == nil {
			err = e
		}
		if err != nil {
			os.Remove(f.Name())
		}
	}
	if err != nil {
		return fail("실어 보낼 묶음을 쓰지 못함(" + errText(err) + ")")
	}
	return body, f.Name(), true
}
