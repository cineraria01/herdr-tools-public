package main

// --status(2026-11-24 deploy-log-rollback — 지휘 에이전트가 「지금 배포할지」를 한 명령으로 판단하게): 기계마다 깔린 판(커밋),
// origin/main과의 차이(밀린 커밋 수·제목 머리), 병합 줄에 남은 제출 수, 지금 도는 배포. 고치지 않는다 — 쓰는 것은 범용 저장소
// git fetch(origin/main 참조)뿐이고 잠금도 쥐지 않는다(도는 배포를 막지 않게).
// 기계의 판은 그 기계 bin.tsv의 소스 판 칸을 배포 기록(deploy.tsv)의 같은 소스 판 줄로 풀어 읽는다(되돌림은 at:<커밋>). 기록에 없는 판
// (기록 전 배포)·닿지 못한 기계·읽지 못한 것은 모름. 본체가 기록의 sha와 다르면 「따로 바뀜」을 붙인다.

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/resident"
)

// statusResident: 이 기계(맥) 상주 판 점검 결과 줄 — hc-sweep이 쓴 한 장, 있을 때만(stale-resident-check, docs/hc-sweep/resident.md).
// 배포했는데 옛 판으로 도는 상주는 배포가 닿지 않은 것이라 배포 상태 곁에 둔다. 다른 기계는 그 기계 사건(hc-stale-resident)으로 온다
func statusResident(dir string, now time.Time) []string {
	var o []string
	for _, l := range resident.Show(dir, now) {
		o = append(o, "맥 "+l)
	}
	return o
}

// binStateSh: 그 기계의 bin.tsv 줄마다 「T 도구 소스판 같음|없음|따로바뀜」, 기록이 없으면 N, 끝에 END(출력이 온전한지)
const binStateSh = `st=$HOME/.local/state/herdr-sync/bin.tsv
sha() { if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1"; else sha256sum "$1"; fi 2>/dev/null | awk '{print $1}'; }
if [ -f "$st" ]; then
  tab=$(printf '\t')
  while IFS="$tab" read -r t s src; do
    [ -n "$t" ] || continue
    h=; [ ! -e "$HOME/.local/bin/$t" ] || h=$(sha "$HOME/.local/bin/$t")
    k=같음; if [ -z "$h" ]; then k=없음; elif [ "$h" != "$s" ]; then k=따로바뀜; fi
    printf 'T\t%s\t%s\t%s\n' "$t" "$src" "$k"
  done < "$st" || exit 1
else
  echo N
fi
echo END
`

// machineBins: 기계(맥은 "맥")의 bin.tsv 상태. ok 거짓 = 모름(닿지 못함·출력이 온전하지 않음)
func machineBins(m string) (rows [][3]string, ok bool) {
	var c *exec.Cmd
	if m == "맥" {
		c = exec.Command("sh", "-c", binStateSh)
	} else {
		c = exec.Command("hc", "exec", "--on", m, "--", "sh", "-c", binStateSh)
	}
	out, rc := output(c)
	ls := splitLines(out)
	if rc != 0 || len(ls) == 0 || ls[len(ls)-1] != "END" {
		return nil, false
	}
	for _, l := range ls[:len(ls)-1] {
		f := strings.Split(l, "\t")
		switch {
		case len(f) == 4 && f[0] == "T":
			rows = append(rows, [3]string{f[1], f[2], f[3]})
		case l == "N":
		default:
			return nil, false
		}
	}
	return rows, true
}

// srcCommit: 소스 판 → 커밋 12자(되돌림 at:<커밋>, 아니면 배포 기록의 마지막 같은 소스 판 줄), 모르면 ""
func srcCommit(src string, log []logRow) string {
	if c, ok := strings.CutPrefix(src, "at:"); ok {
		return c
	}
	for i := len(log) - 1; i >= 0; i-- {
		if log[i].src == src && log[i].c12 != "?" {
			return log[i].c12
		}
	}
	if hs, hc := headSrc(); src == hs && hc != "" { // 기록 전 판이라도 지금 범용 저장소 HEAD와 같은 소스면 그 커밋
		return hc
	}
	return ""
}

var headSrcV, headC12V *string

// headSrc: 범용 저장소 작업 트리의 소스 판과 HEAD 커밋 12자(한 번만 잰다, 못 재면 빈 값)
func headSrc() (string, string) {
	if headSrcV == nil {
		s, err := srcDigest(REPO)
		c := head12(REPO)
		if err != nil || c == "?" {
			s, c = "", ""
		}
		headSrcV, headC12V = &s, &c
	}
	return *headSrcV, *headC12V
}

// machineCommit: 기계 한 대의 판 — 한 커밋이면 그것, 섞였으면 ""와 풀이, 따로 바뀐 도구
func machineCommit(rows [][3]string, log []logRow) (c12, desc string) {
	cnt := map[string]int{}
	var changed []string
	for _, r := range rows {
		c := srcCommit(r[1], log)
		if c == "" {
			c = "모름"
		}
		cnt[c]++
		if r[2] != "같음" {
			changed = append(changed, r[0]+"("+r[2]+")")
		}
	}
	var keys []string
	for k := range cnt {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	switch {
	case len(rows) == 0:
		desc = "모름 (깐 기록 없음)"
	case len(keys) == 1 && keys[0] == "모름":
		desc = "모름 (배포 기록 전 판)"
	case len(keys) == 1:
		c12, desc = keys[0], keys[0]
	default:
		var p []string
		for _, k := range keys {
			p = append(p, fmt.Sprintf("%s 도구 %d개", k, cnt[k]))
		}
		desc = "섞임 (" + strings.Join(p, " · ") + ")"
	}
	if len(changed) > 0 {
		desc += " · 따로 바뀜 " + strings.Join(changed, " ")
	}
	return c12, desc
}

// behind: from..origin/main 커밋 수와 제목 머리(오래된 것부터, 겹침 없이). from을 모르면 n=-1
func behind(from string) (int, []string) {
	out, rc := gitq("-C", REPO, "log", "--format=%s", from+"..origin/main")
	if rc != 0 {
		return -1, nil
	}
	ls := splitLines(out)
	seen := map[string]bool{}
	var heads []string
	for i := len(ls) - 1; i >= 0; i-- {
		h := subjHead(ls[i])
		if !seen[h] {
			seen[h] = true
			heads = append(heads, h)
		}
	}
	return len(ls), heads
}

// subjHead: 제목 머리 — 「<레인>: …」의 레인, 없으면 첫 낱말
func subjHead(s string) string {
	if i := strings.Index(s, ":"); i > 0 && !strings.ContainsAny(s[:i], " \t") {
		return s[:i]
	}
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return "?"
}

func headsText(h []string) string {
	if len(h) > 8 {
		return strings.Join(h[:8], " ") + " …"
	}
	return strings.Join(h, " ")
}

// queueLeft: 병합 줄에 남은 제출 수(origin refs/merge-queue/*), 모르면 -1
func queueLeft() int {
	refs, st := gitx.LsRemote(REPO, "origin", nil, "refs/merge-queue/*")
	if st == gitx.Unknown {
		return -1
	}
	return len(refs)
}

func countText(n int, unit string) string {
	if n < 0 {
		return "모름"
	}
	return strconv.Itoa(n) + unit
}

// pendingLine: 배포할 것 한 줄(from = 깔린 판) — --status와 --landed가 같은 글을 쓴다. k는 밀린 커밋 수(모르면 -1)
func pendingLine(from string) (string, int) {
	q := "병합 줄 " + countText(queueLeft(), "건") + " 남음"
	if from == "" {
		return "배포할 것 모름 (깔린 판을 모른다 — 맥 bin.tsv·배포 기록) · " + q, -1
	}
	k, heads := behind(from)
	switch {
	case k < 0:
		return "배포할 것 모름 (" + from + "..origin/main을 읽지 못했다) · " + q, -1
	case k == 0:
		return "배포할 것 없음 (깔린 판 " + from + " = origin/main) · " + q, 0
	}
	return fmt.Sprintf("배포할 것 %d커밋: %s · %s", k, headsText(heads), q), k
}

// fetchMain: origin/main 참조를 새로(실패는 경고 — 있는 참조로 판정하고 그렇다고 적는다)
func fetchMain() string {
	if _, rc := gitq("-C", REPO, "fetch", "-q", "origin", "+refs/heads/main:refs/remotes/origin/main"); rc != 0 {
		return " (fetch 실패 — 있는 origin/main 참조로)"
	}
	return ""
}

// macCommit: 맥에 깔린 판(맥이 먼저 깔고 기계는 그 판을 받는다) — 모르면 ""
func macCommit(log []logRow) string {
	rows, ok := machineBins("맥")
	if !ok {
		return ""
	}
	c, _ := machineCommit(rows, log)
	return c
}

// showStatus: --status [<기계>…]. 종료 0(모름은 모름으로 찍는다), 기록을 못 읽으면 1
func showStatus(machines []string) int {
	log, err := readLog()
	if err != nil {
		fmt.Fprintf(os.Stderr, "herdr-sync: 배포 기록을 읽지 못했다 (%s): %v\n", deployLogFile(), err)
		return 1
	}
	note := fetchMain()
	om, rc := gitq("-C", REPO, "rev-parse", "origin/main")
	if rc != 0 || len(om) < 12 {
		om = "모름"
	} else {
		om = om[:12]
	}
	run := running()
	if run == "" {
		run = "없음"
	}
	fmt.Printf("origin/main %s%s · 지금 도는 배포 %s\n", om, note, run)
	var lines []string
	from := ""
	for _, m := range append([]string{"맥"}, machines...) {
		rows, ok := machineBins(m)
		if !ok {
			lines = append(lines, m+": 모름 (닿지 않음·읽지 못함)")
			continue
		}
		c, d := machineCommit(rows, log)
		if m == "맥" {
			from = c
		}
		if c != "" {
			if k, _ := behind(c); k >= 0 {
				d += " · 뒤 " + strconv.Itoa(k) + "커밋"
			}
		}
		lines = append(lines, m+": "+d)
	}
	p, _ := pendingLine(from)
	fmt.Println(p + " (맥 기준)")
	fmt.Println(strings.Join(lines, "\n"))
	for _, l := range statusResident(resident.Dir(), time.Now()) {
		fmt.Println(l)
	}
	if len(log) > 0 {
		l := log[len(log)-1]
		fmt.Printf("마지막 배포 %s %s 종료 %s · %s · 부른 곳 %s\n", l.when, l.kind, l.rc, l.c12, l.by)
	}
	if autoOn() {
		fmt.Println("스스로 배포 켬 (" + autoFile() + ")")
	} else {
		fmt.Println("스스로 배포 끔")
	}
	return 0
}
