// logs.go: hc-lane logs — 모든 기계의 도구 기록에서 지난번 뒤에 새로 생긴 줄만 모은다(hc-lane-logs, 2026-11-23 사용자 「허더 툴스가 지금
// 계속 작동하면서 로그를 좀 남겨야 할듯해 … 서버 전체의 로그를 파악해서 … 계속 보완」). 기계 고르기·원격 실행·「끝 표식이 안 맞으면 그 기계를
// 통째로 버림」은 stats의 길(ev.pick·rexec) 그대로. 이 기계도 같은 수집 글(lgScript)을 sh로 돌린다. 읽기만 한다. 쓰는 법은 docs/hc-lane/logs.md
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"herdr-tools/internal/proc"
)

func init() {
	register("logs", thenExit(func(a []string) int { return logsRun(os.Stdout, a) }), noHelp)
}

// lgFiles: 기계마다 새 바이트만 받는 기록(집 아래 일반 경로 — 개인 값 아님). 없는 파일은 빈 것
var lgFiles = []string{
	".local/state/herdr-tools/ops.tsv",
	".local/state/hc-lane/events.tsv",
	".local/state/hc-lane/merger-journal.tsv",
	".local/state/hc-lane/watch.log",
	"Library/Logs/hc-sweep.log",
	".local/state/hc-fill/watch.log",
	".local/share/hc-fill/gc.log",
	".local/share/hc-fill/overlap-warn.log",
	".local/state/herdr-balance/watch.log",
}

const (
	lgMergerKey = "merger-logs" // 위치 파일에서 병합기 로그 폴더 줄 — 위치는 바이트가 아니라 본 파일의 가장 늦은 mtime(초)
	lgStatus    = "(상태)"
)

// lgScript: 수집(sh, 인자 「<상한> <병합기 mtime> (<파일> <위치>)…」). 구획마다 「== <이름> <위치> <시작> <바이트>」, 그 바이트, 「\nEND <이름>」.
// 끝은 「ALL <병합기 mtime>」. 위치보다 파일이 작으면(돌림) 처음부터. 새 바이트가 파일당·기계당 남은 상한을 넘으면 뒤쪽만.
// 시작이 0보다 크면 새 바이트 바로 앞 1바이트부터 보낸다(그 바이트가 줄바꿈이 아니면 줄 중간 — logsTake가 첫 줄바꿈까지 버린다).
// 병합기 로그는 mtime이 지난번보다 늦은 파일의 실패 줄만(위치 0, 시작 = 상한에 잘려 건너뛴 곳). 임시 파일은 어느 길로 끝나도 trap이 지운다.
// 상태는 launchd 작업(이름에 herdr·.hc-)의
// 마지막 종료와 실행 파일(/bin/ 든 절대 경로 낱말)이 있나, herdr-sync/bak 크기
// ponytail: 돌림은 크기로만 안다 — 돌린 새 파일이 지난 위치보다 커진 뒤에 보면 앞쪽을 놓친다. 그러면 위치 파일에 inode를 더한다
// ponytail: 병합기 로그는 mtime 초 단위 — 가장 늦은 파일과 같은 초에 처음 생긴 파일은 놓칠 수 있고, 다시 고친 파일은 실패 줄을 처음부터 또 받는다
const lgScript = `max=$1 wm=$2; shift 2; left=$max t2=; t=$(mktemp) || exit 1; trap 'rm -f "$t" ${t2:+"$t2"}' EXIT; t2=$(mktemp) || exit 1
sec() { n=$(wc -c < "$t" | tr -d ' ') || exit 1; echo "== $1 $2 $3 $n"; cat "$t" || exit 1; echo; echo "END $1"; }
while [ $# -ge 2 ]; do
  rel=$1 pos=$2; shift 2; f=$HOME/$rel; s=0 p0=0; : > "$t" || exit 1
  if [ -f "$f" ]; then
    [ -r "$f" ] || exit 1
    sz=$(wc -c < "$f" | tr -d ' ') || exit 1
    [ "$sz" -lt "$pos" ] && pos=0
    lim=$max; [ "$left" -lt "$lim" ] && lim=$left; [ "$lim" -lt 0 ] && lim=0
    s=$pos; [ $((sz - s)) -gt "$lim" ] && s=$((sz - lim))
    [ "$s" -gt 0 ] && s=$((s - 1))
    if [ "$sz" -gt "$s" ]; then tail -c +$((s + 1)) "$f" | head -c $((sz - s)) > "$t" || exit 1; fi
    p0=$pos
  fi
  sec "$rel" "$p0" "$s"; left=$((left - n))
done
d=$HOME/.local/state/hc-lane/merger-logs; top=$wm
if [ -d "$d" ]; then
  case $(uname) in Darwin) st='-f %m';; *) st='-c %Y';; esac
  tb=$(printf '\t')
  for f in "$d"/*; do
    [ -f "$f" ] || continue
    m=$(stat $st "$f") || exit 1
    [ "$m" -gt "$wm" ] || continue
    [ "$m" -gt "$top" ] && top=$m
    grep -F -e '--- FAIL' -e "FAIL$tb" -e 'panic:' "$f" > "$t2"; [ $? -le 1 ] || exit 1
    n=$(wc -c < "$t2" | tr -d ' ') || exit 1; [ "$n" -gt 0 ] || continue
    lim=$max; [ "$left" -lt "$lim" ] && lim=$left; [ "$lim" -lt 0 ] && lim=0; s=0
    if [ "$n" -gt "$lim" ]; then s=$((n - lim - 1)); tail -c $((lim + 1)) "$t2" > "$t" || exit 1; else cat "$t2" > "$t" || exit 1; fi
    sec "merger-logs/${f##*/}" 0 "$s"; left=$((left - n))
  done
fi
: > "$t" || exit 1
if [ "$(uname)" = Darwin ] && command -v launchctl >/dev/null 2>&1; then
  u=$(id -u)
  launchctl list | awk 'NR > 1 && $3 ~ /herdr|[.]hc-/ { print $2, $3 }' | while read -r c l; do
    if ! pr=$(launchctl print "gui/$u/$l" 2>/dev/null); then echo "launchd $l 마지막 종료 $c · 실행 파일 모름(launchctl print 실패)"; continue; fi
    miss=
    for p in $(printf '%s\n' "$pr" | awk '/^\tprogram = /{print $3} /^\targuments = [{]/{a=1; next} a && /^\t[}]/{a=0} a{for(i=1;i<=NF;i++) print $i}' |
      grep -E '^(/|[$]HOME/)[^ ]*/bin/' | tr -d "\"';"); do
      case $p in '$HOME'/*) p=$HOME/${p#'$HOME/'};; esac
      [ -e "$p" ] || miss="$miss $p"
    done
    if [ -n "$miss" ]; then echo "launchd $l 마지막 종료 $c · 실행 파일 없음$miss"; else echo "launchd $l 마지막 종료 $c · 실행 파일 있음"; fi
  done >> "$t"
fi
b=$HOME/.local/state/herdr-sync/bak
if [ -d "$b" ]; then echo "herdr-sync/bak $(du -sk "$b" | awk '{print $1}')KB" >> "$t"; else echo "herdr-sync/bak 없음" >> "$t"; fi
sec "` + lgStatus + `" 0 0
echo "ALL $top"`

// lgSec: 수집 구획 하나
type lgSec struct {
	name              string
	pos, start, bytes int64
	body              string
}

func logsMax(v string) (int64, bool) {
	n, err := strconv.ParseInt(v, 10, 64)
	return n, err == nil && n > 0
}

func logsRun(w io.Writer, a []string) int {
	e := &ev{ec: "logs", ed: evdir()}
	max := int64(1 << 20)
	for i := 0; i < len(a); i++ {
		switch a[i] {
		case "--all":
			e.all = true
		case "--on":
			v := val(a, i)
			if v == "" || strings.ContainsAny(v, "/ ") {
				dieArg("--on 기계 이름이 이상하다: %s", v)
			}
			e.ons = append(e.ons, v)
			i++
		case "--max":
			n, ok := logsMax(val(a, i))
			if !ok {
				dieArg("--max는 1 이상 바이트 수: %s", a[i+1])
			}
			max = n
			i++
		default:
			unknownArg(a[i], "", "--all", "--on", "--max")
		}
	}
	e.pick()
	cp := logsCursorPath()
	cur, err := logsCursorRead(cp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane logs: 위치 파일을 못 읽었다 — 아무것도 받지 않는다: %v\n", err)
		return 1
	}
	me := machineName()
	var bad, lines []string
	got, want := 0, 1
	next := map[string]int64{}
	take := func(m string, o string, rc int, why string) {
		if rc != 0 {
			bad = append(bad, m+"("+why+")")
			return
		}
		secs, wm, err := logsParse(o)
		if err != nil {
			bad = append(bad, m+"("+err.Error()+")")
			return
		}
		got++
		lines = append(lines, logsTake(m, secs, wm, next)...)
	}
	o, rc := captTo(90, false, nil, "sh", logsArgs(cur, me, max)...)
	why := fmt.Sprintf("sh 종료 %d", rc) // 이 기계는 hc exec를 거치지 않는다
	if rc == proc.TimeoutCode {
		why = "sh 시간 넘김(90초)"
	}
	take(me, o, rc, why)
	if e.pbad {
		bad = append(bad, "기계 목록(hc machine ls 실패)")
	}
	for _, m := range e.unk {
		want++
		bad = append(bad, m+"(연결 안 됨)")
	}
	for _, m := range e.ems {
		if m == me {
			continue // --on 이 기계 — 위에서 받았다(같은 위치 줄을 두 번 앞당기지 않게)
		}
		o, rc := rexec(m, append([]string{"sh"}, logsArgs(cur, m, max)...)...)
		if rc == 9 { // sh를 못 띄우는 기계 — 대수에 넣지 않는다(stats와 같다)
			bad = append(bad, m+"(sh 없음 — 대수 밖)")
			continue
		}
		want++
		take(m, o, rc, fmt.Sprintf("hc exec 종료 %d", rc))
	}
	bw := bufio.NewWriter(w)
	line := fmt.Sprintf("수집: %d/%d대", got, want)
	if len(bad) > 0 {
		line += " · 수집 못 한 기계: " + strings.Join(bad, " ") + " — 그 기계 것은 하나도 받지 않았고 위치도 그대로"
	}
	fmt.Fprintln(bw, line)
	for _, l := range lines {
		fmt.Fprintln(bw, l)
	}
	// 위치는 출력을 다 쓴 뒤에만 앞당긴다(도중에 죽으면 다음에 다시 받는다 — 잃는 것보다 두 번이 낫다)
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane logs: 출력 실패 — 위치를 옮기지 않는다: %v\n", err)
		return 1
	}
	for k, v := range next {
		cur[k] = v
	}
	if err := logsCursorWrite(cp, cur); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane logs: 위치 파일을 못 썼다 — 다음에 같은 줄을 다시 받는다: %v\n", err)
		return 1
	}
	if got < want || e.pbad {
		return 1
	}
	return 0
}

// logsArgs: sh 인자(-c 수집 글 $0 상한 병합기 mtime (파일 위치)…)
func logsArgs(cur map[string]int64, m string, max int64) []string {
	a := []string{"-c", lgScript, "hc-lane", strconv.FormatInt(max, 10), strconv.FormatInt(cur[m+tab+lgMergerKey], 10)}
	for _, f := range lgFiles {
		a = append(a, f, strconv.FormatInt(cur[m+tab+f], 10))
	}
	return a
}

// logsParse: 수집 출력 → 구획들·병합기 mtime. 구획마다 머리의 바이트 수만큼 읽고 바로 뒤 「\nEND <이름>\n」, 맨 끝 「ALL <n>」이 있어야 한다
// (출력 상한에 잘렸거나 반쪽이면 오류 — 그 기계는 통째로 버린다)
func logsParse(o string) (secs []lgSec, wm int64, err error) {
	bad := fmt.Errorf("수집 출력이 끝까지 오지 않음(끝 표식이 맞지 않음)")
	for {
		if rest, ok := strings.CutPrefix(o, "ALL "); ok {
			wm, err := strconv.ParseInt(rest, 10, 64)
			if err != nil {
				return nil, 0, bad
			}
			return secs, wm, nil
		}
		nl := strings.IndexByte(o, '\n')
		if nl < 0 || !strings.HasPrefix(o, "== ") {
			return nil, 0, bad
		}
		f := strings.Fields(o[3:nl])
		if len(f) < 4 {
			return nil, 0, bad
		}
		k := len(f) - 3
		var n [3]int64
		for i := range n {
			if n[i], err = strconv.ParseInt(f[k+i], 10, 64); err != nil || n[i] < 0 {
				return nil, 0, bad
			}
		}
		s := lgSec{name: strings.Join(f[:k], " "), pos: n[0], start: n[1], bytes: n[2]}
		o = o[nl+1:]
		end := "\nEND " + s.name + "\n"
		if int64(len(o)) < s.bytes || !strings.HasPrefix(o[s.bytes:], end) {
			return nil, 0, bad
		}
		s.body, o = o[:s.bytes], o[s.bytes+int64(len(end)):]
		secs = append(secs, s)
	}
}

// logsTake: 한 기계의 구획 → 출력 줄(「기계<TAB>파일<TAB>줄」), 다음 위치는 next에. 파일 구획은 끝 줄바꿈까지만 받고 그 뒤 반쪽 줄은 다음에.
// 시작이 0보다 크면 본문 첫 바이트는 새 바이트 바로 앞 것 — 첫 줄바꿈까지(없으면 전부) 버리고, 위치보다 더 간 만큼이 건너뜀이다.
// 그래서 잘린 뒤 첫 줄도, 줄 중간에 남은 위치(0바이트로 잘림) 다음 실행 첫 줄도 조각이 아니다
func logsTake(m string, secs []lgSec, wm int64, next map[string]int64) []string {
	var out []string
	add := func(name, body string) {
		if body == "" {
			return
		}
		for _, l := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
			out = append(out, m+tab+name+tab+l)
		}
	}
	for _, s := range secs {
		file := s.name != lgStatus && !strings.HasPrefix(s.name, lgMergerKey+"/")
		body, off := s.body, s.start
		if s.start > 0 {
			d := strings.IndexByte(body, '\n') + 1
			if d == 0 {
				d = len(body)
			}
			body, off = body[d:], off+int64(d)
		}
		if skip := off - s.pos; skip > 0 {
			out = append(out, fmt.Sprintf("%s%s%s(잘림 — 앞 %d바이트 건너뜀)", m+tab, s.name, tab, skip))
		}
		if !file {
			add(s.name, body)
			continue
		}
		i := strings.LastIndexByte(body, '\n') + 1
		next[m+tab+s.name] = off + int64(i)
		add(s.name, body[:i])
	}
	next[m+tab+lgMergerKey] = wm
	return out
}

func logsCursorPath() string {
	s := os.Getenv("XDG_STATE_HOME")
	if s == "" {
		s = filepath.Join(os.Getenv("HOME"), ".local/state")
	}
	return filepath.Join(s, "herdr-tools", "logs-cursor.tsv")
}

// logsCursorRead: 「기계<TAB>파일<TAB>위치」(없으면 빈 것). 꼴이 틀린 줄은 버린다(그 파일은 처음부터)
func logsCursorRead(p string) (map[string]int64, error) {
	cur := map[string]int64{}
	ls, err := readLines(p)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, l := range ls {
		f := strings.Split(l, tab)
		if len(f) != 3 {
			continue
		}
		if n, err := strconv.ParseInt(f[2], 10, 64); err == nil && n >= 0 {
			cur[f[0]+tab+f[1]] = n
		}
	}
	return cur, nil
}

// logsCursorWrite: 임시 파일에 다 쓰고 이름 바꾸기(반쪽 위치 파일이 남지 않게)
func logsCursorWrite(p string, cur map[string]int64) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o777); err != nil {
		return err
	}
	keys := make([]string, 0, len(cur))
	for k := range cur {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s\t%d\n", k, cur[k])
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".logs-cursor-*")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
