package main

// 공통 도우미: 탭 줄(Row — awk -F'\t' 꼴), 파일 읽고 쓰기, 외부 명령, 셸 판과 같은 ID·해시

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/proc"
)

// Row: 탭으로 나눈 한 줄. F(i)는 awk $i와 같다(1부터, 없는 칸은 빈 값). 빈 줄은 칸 0개(NF 0)
type Row []string

func splitRow(line string) Row {
	if line == "" {
		return nil
	}
	return Row(strings.Split(line, "\t"))
}

func (r Row) F(i int) string {
	if i < 1 || i > len(r) {
		return ""
	}
	return r[i-1]
}

func (r Row) NF() int { return len(r) }

// Set: awk의 $i = v — 칸이 모자라면 빈 칸으로 늘린다. 새 Row를 돌려준다
func (r Row) Set(i int, v string) Row {
	n := append(Row(nil), r...)
	for len(n) < i {
		n = append(n, "")
	}
	n[i-1] = v
	return n
}

func (r Row) Line() string { return strings.Join(r, "\t") }

// dash: 빈 값이면 -(셸 판 ${x:--})
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// lines: 글을 줄로(끝 줄바꿈 하나는 줄이 아님). 빈 글은 줄 없음
func lines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// readFile: 파일 내용. ok=false면 없거나 못 읽음(셸 판 2>/dev/null — 빈 것으로 본다)
func readFile(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(b), true
}

func readLines(path string) []string {
	s, _ := readFile(path)
	return lines(s)
}

// readRows: 파일의 줄을 Row로(빈 줄은 칸 0개 Row로 남긴다 — awk NR과 같은 차례)
func readRows(path string) []Row {
	var rs []Row
	for _, l := range readLines(path) {
		rs = append(rs, splitRow(l))
	}
	return rs
}

func rowsText(rs []Row) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteString(r.Line())
		b.WriteByte('\n')
	}
	return b.String()
}

// nonEmpty: 파일이 있고 비어 있지 않은가([ -s ])
func nonEmpty(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Size() > 0
}

func exists(path string) bool { _, err := os.Lstat(path); return err == nil }

func isFile(path string) bool { st, err := os.Stat(path); return err == nil && st.Mode().IsRegular() }

func isDir(path string) bool { st, err := os.Stat(path); return err == nil && st.IsDir() }

// writeAtomic: 같은 폴더 임시 파일 → fsync → rename(요구 2). 셸 판의 「> x.tmp && mv x.tmp x」 자리
func writeAtomic(path, data string) error {
	return atomic.WriteFile(path, []byte(data), 0o644)
}

// appendLine: 한 번의 O_APPEND 쓰기로 한 줄(줄바꿈은 붙인다)
func appendLine(path, line string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("열기 %s: %w", path, err)
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		f.Close()
		return fmt.Errorf("쓰기 %s: %w", path, err)
	}
	return f.Close()
}

// ---- 외부 명령 ----

// 표준 오류를 어디로: 버림(2>/dev/null)·물려줌·표준 출력에 합침(2>&1)
const (
	errDrop = iota
	errPass
	errMerge
)

// Cmd: 외부 명령 하나. Env는 덧붙일 「K=V」(빈 값도), Unset은 지울 이름. In이 nil이면 /dev/null
type Cmd struct {
	Name  string
	Args  []string
	Env   []string
	Unset []string
	In    io.Reader
	Err   int
	Max   time.Duration // 0이 아니면 이 상한(proc.Limit이 모르는 명령 — 로컬 sh 등)
}

// Run: 표준 출력(그대로 — 끝 줄바꿈 포함)과 셸 종료 코드
func (c Cmd) Run() (string, int) {
	x := exec.Command(c.Name, c.Args...)
	x.Env = envWith(c.Env, c.Unset)
	x.Stdin = c.In
	var out bytes.Buffer
	x.Stdout = &out
	switch c.Err {
	case errPass:
		x.Stderr = os.Stderr
	case errMerge:
		x.Stderr = &out
	}
	max := c.Max
	if max <= 0 {
		max = proc.Limit(x.Args)
	}
	err := proc.RunMax(x, max) // git·hc·hc-lane 등은 상한 안에서만 기다린다(proc.Limit) — 넘으면 「시간 넘김」 한 줄과 124(모름)
	var ee *exec.ExitError
	var te *proc.TimeoutError
	if errors.As(err, &te) {
		return "", proc.TimeoutCode // 반쪽 출력은 믿지 않는다
	}
	if err != nil && !errors.As(err, &ee) {
		// 못 띄움(PATH에 없음 등) — 셸처럼 127, 이유는 표준 오류에
		fmt.Fprintf(os.Stderr, "hc-fill: %s: %v\n", c.Name, err)
		return out.String(), 127
	}
	return out.String(), proc.Code(err)
}

func envWith(add, unset []string) []string {
	if len(add) == 0 && len(unset) == 0 {
		return nil
	}
	drop := map[string]bool{}
	for _, u := range unset {
		drop[u] = true
	}
	for _, kv := range add {
		drop[kv[:strings.IndexByte(kv, '=')]] = true
	}
	var e []string
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 && drop[kv[:i]] {
			continue
		}
		e = append(e, kv)
	}
	return append(e, add...)
}

// run: 이름·인자로(표준 오류 버림, 입력 /dev/null). 표준 출력과 종료 코드
func run(name string, args ...string) (string, int) { return Cmd{Name: name, Args: args}.Run() }

// sub: $(…) — 끝 줄바꿈을 모두 뗀다
func sub(s string) string { return strings.TrimRight(s, "\n") }

// lastLine: printf '%s\n' "$x" | tail -n 1
func lastLine(s string) string {
	l := lines(s)
	if len(l) == 0 {
		return ""
	}
	return l[len(l)-1]
}

// ---- 목록 ----

// inlist <값> <쉼표 목록>
func inlist(v, list string) bool { return strings.Contains(","+list+",", ","+v+",") }

// words: $(printf %s "$x" | tr , ' ') 낱말 나누기 — 쉼표·공백·탭·줄바꿈으로 나누고 빈 것은 버림
func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
}

// has: printf '%s\n' "$list" | grep -Fxq -- "$x"
func has(list []string, x string) bool {
	for _, l := range list {
		if l == x {
			return true
		}
	}
	return false
}

// cnt: printf '%s\n' "$1" | grep -Fxc -- "$2"
func cnt(list []string, x string) int {
	k := 0
	for _, l := range list {
		if l == x {
			k++
		}
	}
	return k
}

// uniq: awk 'NF && !s[$0]++'
func uniq(xs []string) []string {
	seen := map[string]bool{}
	var o []string
	for _, x := range xs {
		if strings.TrimSpace(x) == "" || seen[x] {
			continue
		}
		seen[x] = true
		o = append(o, x)
	}
	return o
}

// ---- 글 ----

// flat1: 탭·줄바꿈을 빈칸으로
func flat1(s string) string { return strings.NewReplacer("\t", " ", "\n", " ").Replace(s) }

// runeCut: cut -c 1-n(UTF-8 글자 단위)
func runeCut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	k := 0
	for i := range s {
		if k == n {
			return s[:i]
		}
		k++
	}
	return s
}

// head: ${s:0:n}(ASCII 값 — sha 등)
func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// padB: printf '%-Ns' — /bin/bash 3.2 printf처럼 바이트로 센다
func padB(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

// isDigits: case $x in ”|*[!0-9]*) 의 반대(빈 값은 거짓)
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// atoi10: $((10#$x)) — 숫자만 든 값(08·09도 십진). 아니면 0
func atoi10(s string) int {
	n, err := strconv.Atoi(strings.TrimLeft(s, "0"))
	if err != nil {
		return 0
	}
	return n
}

// num: awk의 x+0(앞 숫자만, 아니면 0)
func num(s string) float64 {
	s = strings.TrimLeft(s, " \t")
	end := 0
	for end < len(s) && (s[end] >= '0' && s[end] <= '9' || (end == 0 && (s[end] == '-' || s[end] == '+')) || s[end] == '.') {
		end++
	}
	f, _ := strconv.ParseFloat(s[:end], 64)
	return f
}

// ---- 시각 ----

func nowMin() string   { return time.Now().Format("2006-01-02T15:04") } // date +%Y-%m-%dT%H:%M
func nowUnix() int64   { return time.Now().Unix() }
func utcnow() string   { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }
func stampLog() string { return time.Now().Format("2006-01-02 15:04:05") } // date '+%F %T'

// utc_of <대기열 더한시각(이 기계 분)>: UTC. 못 읽으면 ~(어떤 사건 시각보다 뒤 — 착지로 풀지 않음)
func utcOf(s string) string {
	t, err := time.ParseInLocation("2006-01-02T15:04:05", s+":00", time.Local)
	if err != nil {
		return "~"
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// ---- ID·해시(셸 판과 같은 값) ----

func b36(n int64) string { return strconv.FormatInt(n, 36) }

func rand6() string {
	const a = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		k, err := rand.Int(rand.Reader, big.NewInt(int64(len(a))))
		if err != nil {
			panic(fmt.Sprintf("난수: %v", err)) // 커널 난수가 없으면 이어 갈 수 없다
		}
		b[i] = a[k.Int64()]
	}
	return string(b)
}

// newid <머리 글자>: <머리><UTC 초 36진>-<난수 6자>
func newid(p string) string { return p + b36(nowUnix()) + "-" + rand6() }

// rowid: 줄 ID r<초>-<pid>-<난수>
func rowid() string {
	k, err := rand.Int(rand.Reader, big.NewInt(32768))
	if err != nil {
		panic(fmt.Sprintf("난수: %v", err))
	}
	return fmt.Sprintf("r%d-%d-%d", nowUnix(), os.Getpid(), k.Int64())
}

// cksum: POSIX cksum(1)의 CRC(셸 판이 결정적 ID에 쓴다 — 같은 값이어야 두 판이 같은 ID를 만든다)
func cksum(data []byte) uint32 {
	var crc uint32
	upd := func(b byte) {
		crc ^= uint32(b) << 24
		for i := 0; i < 8; i++ {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
		}
	}
	for _, b := range data {
		upd(b)
	}
	for n := len(data); n > 0; n >>= 8 {
		upd(byte(n))
	}
	return ^crc
}

func sha256hex(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// ---- 끝내기 ----

// exitHooks: run 바퀴의 EXIT trap(held.tsv 비움·사본 지움). die·exit가 돈다
var exitHooks []func()

// 잠금 안(셸 판의 qlock 자식 프로세스)에서 끝내기 — withQ가 받아 그 자식의 종료 코드로 돌려준다
type lockExit struct{ code int }

func exit(code int) {
	if qHeld {
		panic(lockExit{code})
	}
	runExitHooks()
	os.Exit(code)
}

var exitOnce sync.Once

// runExitHooks: 끝내기 훅(나중에 단 것부터) — exit와 run의 신호 처리(셸 판 EXIT trap은 TERM에도 돈다, Fable L2).
// 정상 종료와 신호가 겹쳐도 한 번만(뒤에 온 쪽은 앞의 정리가 끝날 때까지 기다린다, R-20). 훅은 신호 고루틴을 띄우기 전에 단다
func runExitHooks() {
	exitOnce.Do(func() {
		for i := len(exitHooks) - 1; i >= 0; i-- {
			exitHooks[i]()
		}
	})
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "hc-fill: %s\n", fmt.Sprintf(format, a...))
	exit(DRC)
}

func refuse(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "hc-fill: %s\n", fmt.Sprintf(format, a...))
	exit(2)
}

func say(format string, a ...any) { fmt.Printf(format+"\n", a...) }

// crash <지점>: 시험의 중단 주입(HC_FILL_CRASH_AT). 잠금 안이면 그 잠금 일(셸 판의 자식)만 죽은 것처럼, 밖이면 이 프로세스를 SIGKILL
func crash(at string) {
	if os.Getenv("HC_FILL_CRASH_AT") != at {
		return
	}
	if err := appendLine(DATA+"/crash.log", at); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
	}
	if qHeld {
		panic(lockExit{137})
	}
	if p, err := os.FindProcess(os.Getpid()); err == nil {
		p.Kill() // SIGKILL(Windows는 TerminateProcess)
	}
	select {}
}
