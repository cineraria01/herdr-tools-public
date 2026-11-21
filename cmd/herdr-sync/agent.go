package main

// 조각(agent.sh) 부르기·조회·묶음 나누기

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"herdr-tools/internal/cksum"
	"herdr-tools/internal/proc"
)

// macCmd: 맥에서 조각. hsb는 base 파일(HSB) — ""면 조각 기본값(~/.local/state/herdr-sync/base.tsv), "-"면 base를 고치지 않음
func macCmd(hsb string, args ...string) *exec.Cmd {
	c := exec.Command("sh", append([]string{"-c", agent, "_"}, args...)...)
	if hsb != "" {
		c.Env = append(os.Environ(), "HSB="+hsb)
	}
	c.Stderr = os.Stderr
	return c
}

// mac: 맥에서 저장소 base로 (셸 판 mac())
func mac(args ...string) *exec.Cmd { return macCmd(STATE+"/repo-base.tsv", args...) }

// on: 그 기계에서 조각(표준 입력 없음 = hc exec, 있음 = hc exec -i)
func on(m string, input bool, args ...string) *exec.Cmd {
	a := []string{"exec"}
	if input {
		a = append(a, "-i")
	}
	a = append(a, "--timeout", "600", "--on", m, "--", "sh", "-c", agent, "_")
	c := exec.Command("hc", append(a, args...)...)
	c.Stderr = os.Stderr
	return c
}

func run(c *exec.Cmd) int { return proc.Code(proc.Run(c)) } // git·hc·gh 등은 상한 안에서만(proc.Limit)

// runIn: 표준 입력을 주고 돌린다. 표준 출력은 out(nil이면 버림)
func runIn(c *exec.Cmd, in io.Reader, out io.Writer) int {
	c.Stdin, c.Stdout = in, out
	return run(c)
}

func output(c *exec.Cmd) (string, int) {
	out, err := proc.Output(c)
	return string(out), proc.Code(err)
}

// pipeDrain: 소비자가 0으로 끝난 뒤 생산자가 끝나기를 기다리는 상한 — 넘으면 생산자를 끄고 124(모름)
var pipeDrain = 60 * time.Second

// pipeline: prefix + producer의 표준 출력을 consumer의 표준 입력으로(셸 판 `{ cat …; echo .; prod; } | cons`). 두 종료 코드.
// 둘 사이는 OS 파이프 하나(Go가 가운데서 복사하지 않는다): 소비자가 끝나면 이쪽 읽기 끝을 닫아 생산자의 쓰기가 EPIPE로 끝나게 하고,
// 소비자가 실패했으면 생산자를 바로 끈다 — 실패한 소비자 뒤에서 생산자를 끝까지 기다리며 잠금을 쥐지 않는다(gofix-ops A08)
func pipeline(prefix []byte, producer, consumer *exec.Cmd, out io.Writer) (int, int) {
	r, w, err := os.Pipe()
	if err != nil {
		return 1, 1
	}
	consumer.Stdin, consumer.Stdout = r, out
	producer.Stdout = w
	type started struct {
		ok  bool
		err error
	}
	st := make(chan started, 1)
	go func() { // 앞부분을 다 쓴 뒤에 생산자를 띄운다(순서). 소비자가 먼저 죽으면 쓰기가 EPIPE로 돌아온다
		if _, err := w.Write(prefix); err != nil {
			w.Close()
			st <- started{false, nil}
			return
		}
		err := producer.Start()
		w.Close() // 생산자가 제 사본을 쥔다 — 생산자가 끝나면 소비자가 EOF
		st <- started{err == nil, err}
	}()
	crc := run(consumer)
	r.Close()
	s := <-st
	if !s.ok {
		if s.err != nil {
			return proc.Code(s.err), crc
		}
		return 1, crc // 앞부분을 다 넘기지 못했다(소비자가 먼저 끝남)
	}
	done := make(chan error, 1)
	go func() { done <- producer.Wait() }()
	if crc != 0 {
		select {
		case err := <-done:
			return proc.Code(err), crc
		default:
		}
		producer.Process.Kill()
		return proc.Code(<-done), crc
	}
	select {
	case err := <-done:
		return proc.Code(err), crc
	case <-time.After(pipeDrain):
	}
	producer.Process.Kill()
	<-done
	return proc.TimeoutCode, crc
}

// tarCreate: 맥 tar(bsdtar) — 확장 속성·ACL·맥 메타데이터를 빼고, 이름을 바이트 그대로(hdrcharset=BINARY). list 파일의 경로를 dir 아래에서
func tarCreate(dir, list string) *exec.Cmd {
	c := exec.Command("tar", "--no-xattrs", "--no-acls", "--no-mac-metadata", "--format=pax", "--options=hdrcharset=BINARY", "-cf", "-", "-T", list)
	c.Dir = dir
	c.Env = append(os.Environ(), "COPYFILE_DISABLE=1")
	c.Stderr = os.Stderr
	return c
}

func writeLines(name string, lines []string) string {
	p := TMP + "/" + name
	if err := os.WriteFile(p, joinLines(lines), 0o600); err != nil {
		die("임시 파일을 쓰지 못했다: %s", p)
	}
	return p
}

func joinLines(lines []string) []byte {
	var b bytes.Buffer
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	return b.Bytes()
}

func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// nosh: hc 오류 글이 그 기계에 sh가 없다는 뜻인가(hc-lane nosh()와 같은 글자)
func nosh(e string) bool {
	return strings.Contains(e, `"spawn_failed"`) && strings.Contains(e, "cannot start ")
}

// query: where가 ""면 맥(저장소 base), 아니면 그 기계에서 q를 돌려 단위마다 현재·base·덮은 링크를 채운다.
// 나무 파일은 F·L·G·B 줄을 단위 목록에 맞춘다: 없으면 -, 그 파일이나 위 폴더가 링크면 L, 7MB 넘으면 G
func query(where string) (*qres, bool) {
	args := append([]string{"q"}, PK...)
	args = append(append(args, "--"), RMK...)
	args = append(args, "--")
	for _, t := range TR {
		args = append(args, t.k)
	}
	args = append(append(args, "--"), EXK...)
	args = append(append(args, "--"), LKT...)
	var c *exec.Cmd
	if where == "" {
		c = mac(args...)
	} else {
		c = on(where, false, args...)
	}
	var qe bytes.Buffer
	c.Stderr = &qe
	out, rc := output(c)
	q := &qres{RM: map[string]bool{}, Y: map[string]bool{}}
	if rc != 0 {
		e := strings.TrimRight(qe.String(), "\n")
		if nosh(e) {
			q.nosh = true
		} else if e != "" {
			fmt.Fprintln(os.Stderr, e)
		}
		return q, false
	}
	os.Stderr.Write(qe.Bytes())
	type ub struct{ s, b string }
	var us []ub
	fs, gs, bs, lk := map[string]string{}, map[string]bool{}, map[string]string{}, map[string]bool{}
	for _, l := range splitLines(out) {
		a := strings.Split(l, "\t")
		get := func(i int) string {
			if i < len(a) {
				return a[i]
			}
			return ""
		}
		switch a[0] {
		case "U":
			us = append(us, ub{get(1), get(2)})
		case "F":
			fs[get(1)] = get(2)
		case "L":
			lk[get(1)] = true
		case "G":
			gs[get(1)] = true
		case "B":
			bs[get(1)] = get(2)
		case "R":
			q.RM[get(1)] = true
		case "Y":
			q.Y[get(1)] = true
		case "H":
			q.H = get(1)
		case "O":
			q.O = get(1)
		}
	}
	iu := 0
	for _, u := range U {
		if !u.tree {
			if iu >= len(us) {
				return q, false
			}
			q.C, q.B, q.L = append(q.C, us[iu].s), append(q.B, us[iu].b), append(q.L, "")
			iu++
			continue
		}
		s, ok := fs[u.k]
		if !ok {
			s = "-"
		}
		if gs[u.k] {
			s = "G"
		}
		w := ""
		for p := u.k; ; {
			if lk[p] {
				s, w = "L", p
				break
			}
			i := strings.LastIndexByte(p, '/')
			if i < 0 {
				break
			}
			p = p[:i]
		}
		b, ok := bs[u.k]
		if !ok {
			b = "-"
		}
		q.C, q.B, q.L = append(q.C, s), append(q.B, b), append(q.L, w)
	}
	// 끝 줄(H·O)까지 와야 온전한 조회다 — 잘린 조회를 「OS 알 수 없음」 건너뜀(종료 0)으로 읽지 않는다(gofix-ops B10)
	return q, iu == len(us) && q.H != "" && q.O != ""
}

// rsig: 저장소 파일 서명(맥 서명과 같은 꼴 crc.크기.x|-), 없으면(일반 파일이 아니면) -.
// 있는지 모르거나 못 읽으면 멈춘다 — 「없음」으로 읽으면 깖이 「저장소에서 지워짐」으로 맥 파일을 치운다(gofix-ops A01·O-08)
func rsig(f, k string) string {
	st, err := os.Stat(f)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return "-"
		}
		die("저장소 파일을 읽지 못했다: %s (%v)", f, err)
	}
	if !st.Mode().IsRegular() {
		return "-"
	}
	x := "-"
	if !strings.Contains(k, "#") && canExec(f) {
		x = "x"
	}
	fh, err := os.Open(f)
	if err != nil {
		die("저장소 파일을 읽지 못했다: %s (%v)", f, err)
	}
	defer fh.Close()
	c, n, err := cksum.Reader(fh)
	if err != nil {
		die("저장소 파일을 읽지 못했다: %s (%v)", f, err)
	}
	return fmt.Sprintf("%d.%d.%s", c, n, x)
}

// lines: 두 파일의 diff에서 바뀐 줄 수(<·> 줄). 둘째가 파일이 아니면 빈 파일과
func lines(a, b string) int {
	if !isFile(b) {
		b = "/dev/null"
	}
	c := exec.Command("diff", a, b)
	c.Stderr = os.Stderr
	out, _ := c.Output()
	n := 0
	for _, l := range splitLines(string(out)) {
		if strings.HasPrefix(l, "<") || strings.HasPrefix(l, ">") {
			n++
		}
	}
	return n
}

// macval: 맥 값(단위 번호) → TMP/v<번호>
func macval(i int) string {
	p := fmt.Sprintf("%s/v%d", TMP, i)
	if !exists(p) {
		f, err := os.Create(p)
		if err != nil {
			die("임시 파일을 쓰지 못했다: %s", p)
		}
		c := mac("r", U[i].k)
		c.Stdout = f
		run(c)
		f.Close()
	}
	return p
}

// mpath: 맥 값이 든 파일
func mpath(i int) string {
	if U[i].tree {
		return home + "/" + U[i].k
	}
	return macval(i)
}

// capOf: hc exec는 입력·출력을 8MB에서 자른다. 나무 파일을 보내고 받을 때는 7MB 아래 묶음 여러 번으로 나눈다
const capOf = 7340032

// chunks: 줄을 묶음으로(파일 크기+tar 머리 합이 7MB 아래, 묶음마다 적어도 한 줄). sigf는 서명 칸(1부터)
func chunks(ls []string, sigf int) [][]string {
	var r [][]string
	u := 0
	for _, l := range ls {
		size := 0
		if p := strings.Split(field(l, sigf), "."); len(p) > 1 {
			fmt.Sscanf(p[1], "%d", &size)
		}
		e := (size+511)/512*512 + 1536 + len(l) + 1
		if len(r) == 0 || u+e > capOf {
			r = append(r, nil)
			u = 0
		}
		u += e
		r[len(r)-1] = append(r[len(r)-1], l)
	}
	return r
}

func field(l string, i int) string {
	f := strings.Split(l, "\t")
	if i-1 < len(f) {
		return f[i-1]
	}
	return ""
}

func sigx(s string) string { // 서명 끝 x|-
	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		return s[i+1:]
	}
	return s
}

func sigBody(s string) string { // 서명의 내용 부분(crc.크기)
	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		return s[:i]
	}
	return s
}

// wpack: 조각 W의 입력 앞부분(목록, 「.」 줄)과 tar 생산자(그 폴더 아래 원본들)
func wpack(dir string, list []string, name string) ([]byte, *exec.Cmd) {
	var src []string
	for _, l := range list {
		src = append(src, field(l, 4))
	}
	return append(joinLines(list), ".\n"...), tarCreate(dir, writeLines(name, src))
}

// wres: 목록 줄마다 (ok, 키, 까닭)
func wres(list []string, wout string) [][3]string {
	ok, why := map[string]bool{}, map[string]string{}
	for _, l := range splitLines(wout) {
		a := strings.Split(l, "\t")
		switch {
		case a[0] == "W" && len(a) > 1:
			ok[a[1]] = true
		case a[0] == "E" && len(a) > 2:
			why[a[1]] = a[2]
		}
	}
	var r [][3]string
	for _, l := range list {
		k := field(l, 1)
		switch {
		case ok[k]:
			r = append(r, [3]string{"ok", k, ""})
		case why[k] != "":
			r = append(r, [3]string{"fail", k, why[k]})
		default:
			r = append(r, [3]string{"fail", k, "답이 없다"})
		}
	}
	return r
}
