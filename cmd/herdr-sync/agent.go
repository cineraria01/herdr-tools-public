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
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"herdr-tools/internal/cksum"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/shx"
)

// hcExecMax: hc exec --timeout 상한(초). hc는 1~600만 받고 넘으면 invalid_params·종료 6으로 거절한다(2026-11-22 sync-hc-timeout).
// 더 오래 걸릴 수 있는 일은 기계 쪽 조각이 스스로 끝 시각을 지킨다(installSh의 budget)
const hcExecMax = 600

// agentQ: 조회(q)용 조각 — 끝 표식(shx.Tail)이 찍혀야 응답이 온전하다(잘린 응답은 모름, ops/R-207). 엄격 머리(shx.Script)는
// 쓰지 않는다: 리눅스 기계의 sh(dash 0.5.12)가 pipefail을 받지 않는다 — q는 결과를 바꾸는 단계를 파이프에 두지 않고 || exit 1로 본다.
// 다른 하위 명령(r·T는 내용·tar 바이트를 그대로 낸다)은 표식 없는 agent 그대로
var agentQ = shx.Tail(agent)

// script: 하위 명령에 맞는 조각
func script(args []string) string {
	if len(args) > 0 && args[0] == "q" {
		return agentQ
	}
	return agent
}

// macCmd: 맥에서 조각. hsb는 base 파일(HSB) — ""면 조각 기본값(~/.local/state/herdr-sync/base.tsv), "-"면 base를 고치지 않음
func macCmd(hsb string, args ...string) *exec.Cmd {
	c := exec.Command("sh", append([]string{"-c", script(args), "_"}, args...)...)
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
	a = append(a, "--timeout", strconv.Itoa(hcExecMax), "--on", m, "--", "sh", "-c", script(args), "_")
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

// pipeMax: 묶음 하나(생산자 | 소비자)의 전체 상한 — 두 명령의 proc.Limit 가운데 큰 것, 둘 다 없으면(tar·sh·hc exec --timeout) 이 값.
// hc exec의 --timeout 600에 여유를 둔다. 넘으면 두 무리를 다 끄고 둘 다 124(모름)
var pipeMax = 660 * time.Second

// pipeline: prefix + producer의 표준 출력을 consumer의 표준 입력으로(셸 판 `{ cat …; echo .; prod; } | cons`). 두 종료 코드.
// 둘 사이는 OS 파이프 하나(Go가 가운데서 복사하지 않는다): 소비자가 끝나면 이쪽 읽기 끝을 닫아 생산자의 쓰기가 EPIPE로 끝나게 하고,
// 소비자가 실패했으면 생산자를 바로 끈다 — 실패한 소비자 뒤에서 생산자를 끝까지 기다리며 잠금을 쥐지 않는다(gofix-ops A08)
//
// 둘 다 새 무리로 띄워 proc에 올리고(proc.Start — 신호·SIGKILL 뒤에도 남지 않게), 전송 시작부터 한 기한(pipeMax 등) 안에서 기다린다.
// 소비자가 실패하거나 기한·생산자 기다림을 넘기면 PID 하나가 아니라 그 무리째(자손까지) 거둔다 — 남은 자손이 완료표를 쓰지 않게(R-08)
func pipeline(prefix []byte, producer, consumer *exec.Cmd, out io.Writer) (int, int) {
	r, w, err := os.Pipe()
	if err != nil {
		return 1, 1
	}
	consumer.Stdin, consumer.Stdout = r, out
	producer.Stdout = w
	proc.Group(producer)
	proc.Group(consumer)
	producer.WaitDelay, consumer.WaitDelay = proc.Grace, proc.Grace // 출력 관을 쥔 손자를 끝없이 기다리지 않게
	d := max(proc.Limit(producer.Args), proc.Limit(consumer.Args))
	if d <= 0 {
		d = pipeMax
	}
	limit := time.NewTimer(d)
	defer limit.Stop()
	relC, err := proc.Start(consumer)
	r.Close() // 소비자가 제 사본을 쥔다 — 소비자가 끝나면 생산자의 쓰기가 EPIPE
	if err != nil {
		w.Close()
		return 1, proc.Code(err)
	}
	cdone := make(chan error, 1)
	go func() { e := consumer.Wait(); relC(); cdone <- e }()
	type started struct {
		rel func()
		err error
	}
	pst := make(chan started, 1)
	go func() { // 앞부분을 다 쓴 뒤에 생산자를 띄운다(순서). 소비자가 먼저 죽으면 쓰기가 EPIPE로 돌아온다
		if _, err := w.Write(prefix); err != nil {
			w.Close()
			pst <- started{nil, err}
			return
		}
		rel, err := proc.Start(producer)
		w.Close() // 생산자가 제 사본을 쥔다 — 생산자가 끝나면 소비자가 EOF
		pst <- started{rel, err}
	}()
	stop := func(c *exec.Cmd) { go proc.StopGroup(c.Process.Pid, proc.Grace) }
	pdone := make(chan error, 1)
	var drain <-chan time.Time
	prc, crc := -1, -1
	pup, timedOut := false, false
	for pst != nil || prc < 0 || crc < 0 {
		select {
		case s := <-pst:
			pst = nil
			if s.rel == nil {
				prc = 1 // 앞부분을 다 넘기지 못했거나(소비자가 먼저 끝남) 생산자를 못 띄웠다
				if s.err != nil && !errors.Is(s.err, syscall.EPIPE) {
					prc = proc.Code(s.err)
				}
				break
			}
			pup = true
			go func() { e := producer.Wait(); s.rel(); pdone <- e }()
			if timedOut || crc > 0 {
				stop(producer)
			}
		case e := <-cdone:
			crc = proc.Code(e)
			switch {
			case timedOut:
			case crc != 0 && pup:
				stop(producer) // 실패한 소비자 뒤에서 생산자를 끝까지 기다리며 잠금을 쥐지 않는다(gofix-ops A08)
			case crc == 0 && prc < 0:
				t := time.NewTimer(pipeDrain)
				defer t.Stop()
				drain = t.C
			}
		case e := <-pdone:
			prc = proc.Code(e)
		case <-drain:
			drain = nil
			if prc < 0 {
				fmt.Fprintf(os.Stderr, "herdr-sync: 소비자가 끝난 뒤 생산자가 %v 안에 끝나지 않았다 — 끈다\n", pipeDrain)
				timedOut = true
				stop(producer)
			}
		case <-limit.C:
			fmt.Fprintf(os.Stderr, "herdr-sync: 시간 넘김 %s | %s %d — 둘 다 끈다\n", producer.Path, consumer.Path, int(d/time.Second))
			timedOut = true
			if pup && prc < 0 {
				stop(producer)
			}
			if crc < 0 {
				stop(consumer)
			}
		}
	}
	if timedOut {
		if crc == 0 { // 소비자는 다 받고 끝났다 — 생산자만 모름
			return proc.TimeoutCode, crc
		}
		return proc.TimeoutCode, proc.TimeoutCode
	}
	return prc, crc
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
	return parseQ(out)
}

// qcols: 조회 응답 줄마다 칸 수(B는 base 줄 통째 — 저장소 base는 칸이 더 있다: 그 이상)
var qcols = map[string]int{"U": 3, "F": 3, "L": 2, "G": 2, "B": 3, "R": 2, "Y": 2, "H": 2, "O": 2}

// parseQ: 조회 응답(q의 출력) → 단위마다 현재·base·덮은 링크. 끝 표식(shx.Cut)이 있고, 줄마다 아는 꼴·칸 수·빈 칸 없음이고,
// H·O가 한 번씩 O가 맨 끝일 때만 온전하다 — 잘린 응답(「O<TAB>Darw」)을 「비지원 OS 건너뜀」으로 읽지 않는다(ops/R-207)
func parseQ(out string) (*qres, bool) {
	q := &qres{RM: map[string]bool{}, Y: map[string]bool{}}
	body, ok := shx.Cut(out, 0)
	if !ok {
		return q, false
	}
	type ub struct{ s, b string }
	var us []ub
	fs, gs, bs, lk := map[string]string{}, map[string]bool{}, map[string]string{}, map[string]bool{}
	ls := splitLines(body)
	nh, no := 0, 0
	for i, l := range ls {
		a := strings.Split(l, "\t")
		n, known := qcols[a[0]]
		if !known || len(a) < n || a[0] != "B" && len(a) != n || slices.Contains(a[1:n], "") {
			fmt.Fprintf(os.Stderr, "herdr-sync: 조회 응답의 줄이 깨졌다: %q\n", l)
			return q, false
		}
		switch a[0] {
		case "H":
			nh++
		case "O":
			if no++; i != len(ls)-1 {
				return q, false
			}
		}
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
	return q, iu == len(us) && nh == 1 && no == 1
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
