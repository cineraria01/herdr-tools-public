package main

// 바이너리: herdr-tools 도구는 Go 바이너리 하나다(2026-11-22 사용자 「셸 판 그냥 지금 즉시 지우고 go 로 된것만 집중 개선 수정하자」 — go-shell-remove).
// sync.list 바이너리 줄(`<묶음> .local/bin/<도구> tools:cmd/<도구> <대상>`)마다 그 대상 기계의 ~/.local/bin/<도구>에 원자 교체로 깐다.
//   - 짓기·올리기(release — 2026-11-21 사용자 「각 컴퓨터 마다 빌드 하지말고 github 에다가 빌드 한것도 처리해서 … 각 컴퓨터에서는 받아서」):
//     herdr-tools-publish가 개인 값·비밀 꼴을 검사하고 내보낸 바로 그 비공개 커밋(HERDR_TOOLS_PUBLISH_RESULT)을 원본 blob 그대로(exportTree) 꺼내
//     바이너리 줄의 도구를 darwin/arm64로, 리눅스 기계에 가는 도구는 linux/amd64로도 짓는다(-trimpath, CGO_ENABLED=0, GOTOOLCHAIN=local,
//     커밋을 -ldflags -X로 새김, 상한 HERDR_TIMEOUT_BUILD 600초). 그 커밋의 트리가 범용 저장소 HEAD와 다르면 올리지 않는다(gofix-ops A02).
//     publish.repo 릴리스 bin-<커밋 12자>에 <도구>·<도구>-linux-amd64·SHA256SUMS. 이미 올린 자산은 덮지 않는다(A05)
//   - 깔기(installSh — 기계마다, 맥도): 받은 것의 sha256이 맥이 계산한 값과 같으면 그것을, 못 받으면(릴리스 없음·통신·해시·다른 CPU)
//     그 기계 ~/.local/lib/herdr-go(Go 소스 나무 줄이 깐다)의 소스 판이 범용 저장소와 같을 때만 그 기계에서 짓는다.
//     어느 쪽이든 같은 폴더 임시 파일 → mv 한 번(본체 하나가 한 세대 — 옛 「.bin/<도구> + .src 두 파일」을 따로 바꾸던 길은 없다, R-01·R-11).
//     옛 판으로 되돌릴 일도 없다: mv가 안 되면 옛 본체 그대로다. 기록(~/.local/state/herdr-sync/bin.tsv: 도구·본체 sha256·소스 판)은
//     「이미 이 판인가」를 알아보는 데만 쓴다 — 실행은 기록을 보지 않는다. 기록이 본체와 어긋나면(쓰다 죽음) 판 모름으로 보고 다시 깐다.
//     받기·짓기 모두 안 되면 그 기계는 「다름 (바이너리)」, 크게 경고하고 그 기계의 설정·지움은 미룬다(옛 포장으로 떨어지지 않는다)
//   - 쓰는 쪽은 herdr-sync 하나(맥 잠금 안)다 — 「부를 때 짓는」 실행기(옛 lib/go-run.sh)는 없다(R-10·R-40)
// 로그에는 publish.repo 주소 원문을 찍지 않는다(설정 키 이름만 — 주소에 인증값이 들 수 있다, A12).
// 시험용: HERDR_SYNC_RELEASE_URL(받는 주소 앞부분, 기본 https://github.com/<주인/이름>/releases/download — file://도 됨)

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/proc"
)

// binUnit: sync.list 바이너리 줄 하나
type binUnit struct{ tool, t string }

// BINS: 바이너리 줄들(parseList)
var BINS []binUnit

// installSh: 기계에서 바이너리를 까는 조각(POSIX sh — 리눅스 dash도). 인자: <모드 set|check> <커밋12> <소스판> <CPU(uname -m)>, 그다음 (도구 주소 sha256)…
// (주소·sha256이 -이면 받지 않고 짓기만). 도구마다 정확히 한 줄: S(이미 이 판)·W(받음)·B(지음 — 까닭)·E(못 깖 — 까닭, 옛 본체 그대로)·
// D(--check: 다름 — 까닭). W·B 셋째 칸이 있으면 기록을 못 씀(다음 실행이 다시 깐다).
// 이 기계 소스 판(cur)은 srcDigest와 같은 계산(TestInstallShSourceDigest가 본다)
// budget(조각 앞에 붙인 변수, 초 — 없거나 0이면 상한 없음): 그 시각이 지나면 남은 도구는 받거나 짓지 않고 E 「시간 상한」(옛 본체 그대로,
// 다음 herdr-sync가 잇는다), 받기(curl)도 남은 시간까지만.
// ponytail: 이미 시작한 짓기 하나는 끊지 않는다 — hc 상한(hcExecMax)과의 차이(120초)가 그 몫. 짓기가 그보다 길어지면 hc run --bg로 나눈다
const installBudget = hcExecMax - 120

// budgetSh: 끝 시각(secs초 뒤)을 지키는 installSh
func budgetSh(secs int) string { return "budget=" + strconv.Itoa(secs) + "\n" + installSh }

const installSh = `mode=$1 c12=$2 src=$3 arch=$4; shift 4
end=; [ "${budget:-0}" -le 0 ] || end=$(( $(date +%s) + budget ))
left() { if [ -z "$end" ]; then echo 300; else l=$(( end - $(date +%s) )); [ "$l" -lt 300 ] || l=300; echo "$l"; fi; }
b=$HOME/.local/bin
st=$HOME/.local/state/herdr-sync/bin.tsv
g=$HOME/.local/lib/herdr-go
tab=$(printf '\t')
sha() { if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1"; else sha256sum "$1"; fi | awk '{print $1}'; }
gob() {
  command -v go 2>/dev/null && return 0
  for x in /opt/homebrew/bin/go /usr/local/go/bin/go /usr/local/bin/go; do [ -x "$x" ] && { echo "$x"; return 0; }; done
  return 0
}
um=$(uname -m); [ "$um" != amd64 ] || um=x86_64
cur=
[ ! -f "$g/go.mod" ] || cur=$(cd "$g" && find go.mod cmd internal -type f ! -path '*/.*' 2>/dev/null | LC_ALL=C sort | tr '\n' '\0' | xargs -0 cksum | cksum | awk '{print $1"."$2}')
[ "$mode" = check ] || mkdir -p "$b" "${st%/*}" || exit 1
rec() {
  { [ ! -f "$st" ] || awk -F'\t' -v t="$1" '$1 != t' "$st"; printf '%s\t%s\t%s\n' "$1" "$2" "$src"; } > "$st.$$" && mv -f "$st.$$" "$st" && return 0
  rm -f "$st.$$"; return 1
}
while [ $# -ge 3 ]; do
  t=$1 url=$2 sum=$3; shift 3
  have=
  [ ! -f "$b/$t" ] || have=$(sha "$b/$t")
  if [ -n "$have" ] && grep -qxF "$t$tab$have$tab$src" "$st" 2>/dev/null; then printf 'S\t%s\n' "$t"; continue; fi
  if [ -n "$have" ] && [ "$have" = "$sum" ]; then
    [ "$mode" = check ] || rec "$t" "$have" || :
    printf 'S\t%s\n' "$t"; continue
  fi
  if [ "$mode" = check ]; then
    if [ -n "$have" ]; then printf 'D\t%s\t판이 다르거나 모름\n' "$t"; else printf 'D\t%s\t없음\n' "$t"; fi
    continue
  fi
  tl=$(left)
  if [ "$tl" -le 0 ]; then printf 'E\t%s\t시간 상한 — 다음 herdr-sync에서\n' "$t"; continue; fi
  n=$b/.$t.new.$$ why= how=W
  rm -f "$n" "$n.log"
  if [ "$url" = - ]; then why='릴리스 없음'
  elif [ "$um" != "$arch" ]; then why="CPU가 $um"
  elif ! curl -fsSL --max-time "$tl" -o "$n" "$url" 2>/dev/null; then why='받지 못했다'
  elif [ "$(sha "$n")" != "$sum" ]; then why='받은 것의 해시가 다르다'
  fi
  if [ -n "$why" ]; then
    rm -f "$n"; how=B
    go=$(gob)
    if [ -z "$go" ]; then printf 'E\t%s\t%s · 그 기계에 go가 없다\n' "$t" "$why"; continue; fi
    if [ -z "$cur" ] || [ "$cur" != "$src" ]; then printf 'E\t%s\t%s · 그 기계 Go 소스 판이 범용 저장소와 다르다\n' "$t" "$why"; continue; fi
    if ! (cd "$g" && GOTOOLCHAIN=local CGO_ENABLED=0 GOOS= GOARCH= GOFLAGS= \
        "$go" build -trimpath -buildvcs=false -ldflags "-X herdr-tools/internal/version.Commit=$c12" -o "$n" "./cmd/$t" </dev/null >"$n.log" 2>&1); then
      printf 'E\t%s\t%s · 그 기계에서 짓지 못했다: %s\n' "$t" "$why" "$(tail -n 1 "$n.log" 2>/dev/null)"; rm -f "$n" "$n.log"; continue
    fi
    rm -f "$n.log"
  fi
  if chmod 755 "$n" && s=$(sha "$n") && [ -n "$s" ] && mv -f "$n" "$b/$t"; then
    note=$why
    rec "$t" "$s" || note="$why${why:+ · }기록을 쓰지 못했다"
    if [ -n "$note" ]; then printf '%s\t%s\t%s\n' "$how" "$t" "$note"; else printf '%s\t%s\n' "$how" "$t"; fi
  else
    rm -f "$n"; printf 'E\t%s\t바꿔 넣지 못했다 — 옛 본체 그대로\n' "$t"
  fi
done
`

// relInfo: 이번 실행의 바이너리 판 — 받을 곳(릴리스가 이 빌드와 같다고 확인됐을 때만 tag)과 짓는 판
type relInfo struct {
	c12, src  string            // 새길 커밋 12자 · 소스 판(srcDigest)
	base, tag string            // 받는 주소 앞부분 · 릴리스 태그 — 비면 받지 않고 기계마다 짓는다
	sums      map[string]string // 자산 이름 → sha256
	lx        map[string]bool   // linux/amd64 자산이 있는 도구
}

// binTools: 그 기계에 갈 바이너리 도구(이름 순, 겹침 없이)
func binTools(m, osname string) []string {
	seen := map[string]bool{}
	var r []string
	for _, b := range BINS {
		if targets(b.t, m, osname) && !seen[b.tool] {
			seen[b.tool] = true
			r = append(r, b.tool)
		}
	}
	sort.Strings(r)
	return r
}

// linuxTools: tools 가운데 linux/amd64로도 지을 도구 — 바이너리 줄 대상이 all이거나 osOf가 Linux라고 아는 기계 이름을 담을 때
func linuxTools(tools []string, bins []binUnit, osOf map[string]string) []string {
	want := map[string]bool{}
	for _, b := range bins {
		if b.t == "all" {
			want[b.tool] = true
			continue
		}
		for m, o := range osOf {
			if o == "Linux" && b.t != "macos" && b.t != "local" && targets(b.t, m, "Linux") {
				want[b.tool] = true
			}
		}
	}
	var r []string
	for _, t := range tools {
		if want[t] {
			r = append(r, t)
		}
	}
	return r
}

// unknownTargets: 바이너리 줄 대상으로 이름을 댄 기계 가운데 이번 실행에서 OS를 모르는 것(이름 순, 겹침 없이)
func unknownTargets(bins []binUnit, osOf map[string]string) []string {
	seen := map[string]bool{}
	var r []string
	for _, b := range bins {
		if b.t == "all" || b.t == "macos" || b.t == "local" {
			continue
		}
		for _, m := range strings.Split(b.t, ",") {
			if osOf[m] == "" && !seen[m] {
				seen[m] = true
				r = append(r, m)
			}
		}
	}
	sort.Strings(r)
	return r
}

// installBins: 그 기계(맥은 "맥")에 바이너리를 깐다(--check면 견주기만). 다름 묶음과 「다 깔렸나」(거짓이면 설정·지움을 미룬다)
func installBins(m, osname string, rel *relInfo) ([]string, bool) {
	tools := binTools(m, osname)
	if len(tools) == 0 || rel == nil {
		return nil, true
	}
	mode, arch := "set", "arm64"
	if check {
		mode = "check"
	}
	if osname == "Linux" {
		arch = "x86_64"
	}
	args := []string{mode, rel.c12, rel.src, arch}
	for _, t := range tools {
		a := t
		if osname == "Linux" {
			a = t + "-linux-amd64"
		}
		if rel.tag != "" && (osname != "Linux" || rel.lx[t]) {
			args = append(args, t, rel.base+"/"+rel.tag+"/"+a, rel.sums[a])
		} else {
			args = append(args, t, "-", "-")
		}
	}
	var c *exec.Cmd
	var out string
	var rc int
	var errb bytes.Buffer
	if m == "맥" {
		// 맥 대체 빌드도 상한·무리 소유(ops/R-205): 조각 전체가 HERDR_TIMEOUT_BUILD 안에(남은 도구는 그 시각부터 E 「시간 상한」), 넘기면
		// proc가 무리째(짓던 go·컴파일러 자손까지) 거둔 뒤 돌아오고 결과는 모름 — 돌아온 뒤 본체·bin.tsv를 쓰는 자손이 남지 않는다.
		// herdr-sync가 신호를 받으면 onSignals의 StopCurrent가 이 무리를 다 거둔 뒤에 끝낸다(잠금은 그 뒤에 놓인다)
		bm := buildMax()
		c = exec.Command("sh", append([]string{"-c", budgetSh(int(bm / time.Second)), "_"}, args...)...)
		c.Stderr = &errb
		b, err := proc.OutputMax(c, bm)
		out, rc = string(b), proc.Code(err)
	} else {
		// 조각이 installBudget 안에 끝내고(남은 도구는 E 「시간 상한」), hc 상한까지 남은 몫은 마지막 짓기·보고 몫
		c = exec.Command("hc", append([]string{"exec", "--timeout", strconv.Itoa(hcExecMax), "--on", m, "--", "sh", "-c", budgetSh(installBudget), "_"}, args...)...)
		c.Stderr = &errb
		out, rc = output(c)
	}
	ok := rc == 0
	for _, p := range putNotes(m, tools, out, rc, errb.String()) {
		switch {
		case p.bad:
			ok = false
			fmt.Fprintf(os.Stderr, "!!! herdr-sync: %s !!!\n", p.s)
		case p.warn:
			warn("%s", p.s)
		default:
			say("%s", p.s)
		}
		if p.diff {
			ok = false
		}
	}
	if !ok {
		if !check {
			fmt.Fprintf(os.Stderr, "!!! herdr-sync: %s 바이너리를 다 깔지 못했다 — 그 기계의 설정·지움은 미룬다(옛 도구가 남아 있게). 다음 herdr-sync에서 !!!\n", m)
		}
		return []string{"바이너리"}, false
	}
	return nil, true
}

// putNotes: 깔기 한 번(맥·기계)의 알릴 줄. 전송이 0으로 끝나지 않았으면 응답 줄은 잘렸거나 끝나지 않은 것일 수 있어 확정(받음)으로
// 찍지 않는다 — 깔렸는지 모른다는 못 깖 한 줄만(다음 herdr-sync가 판을 보고 다시 깔거나 같음으로 넘어간다, R-19)
func putNotes(m string, tools []string, out string, rc int, errs string) []note {
	if rc != 0 {
		return []note{{s: fmt.Sprintf("%s 바이너리 깔기 실패 (종료 %d — 깔렸는지 모름, 다음 herdr-sync에서 확인) %s", m, rc, strings.TrimSpace(errs)), bad: true}}
	}
	return installReport(m, tools, out)
}

type note struct {
	s               string
	warn, bad, diff bool // 경고 · 못 깖(크게) · 다름(--check)
}

// installReport: installSh 출력 → 알릴 줄. 보낸 도구마다 S·W·B·E·D 가운데 정확히 한 줄이어야 한다 — 빠짐·겹침·모르는 도구는 못 깖(C03)
func installReport(m string, tools []string, out string) []note {
	want := map[string]bool{}
	for _, t := range tools {
		want[t] = true
	}
	seen := map[string]int{}
	var r []note
	for _, l := range splitLines(out) {
		a := strings.Split(l, "\t")
		if len(a) < 2 || !want[a[1]] {
			r = append(r, note{s: fmt.Sprintf("%s 바이너리 깔기: 알 수 없는 응답 %q", m, l), bad: true})
			continue
		}
		seen[a[1]]++
		why := ""
		if len(a) > 2 {
			why = a[2]
		}
		switch {
		case a[0] == "S" && why == "":
		case a[0] == "W" && why == "":
			r = append(r, note{s: fmt.Sprintf("%s 받음 %s", m, a[1])})
		case a[0] == "B" && why != "" && !strings.Contains(why, "기록을 쓰지 못했다"):
			r = append(r, note{s: fmt.Sprintf("%s 지음 %s (릴리스를 못 받아 그 기계에서 — %s)", m, a[1], why)})
		case (a[0] == "W" || a[0] == "B") && strings.Contains(why, "기록을 쓰지 못했다"):
			r = append(r, note{s: fmt.Sprintf("%s 깖 %s — %s(다음 실행이 다시 깐다)", m, a[1], why), warn: true})
		case a[0] == "E" && why != "":
			r = append(r, note{s: fmt.Sprintf("%s %s 바이너리를 깔지 못했다: %s", m, a[1], why), bad: true})
		case a[0] == "D" && why != "" && check:
			r = append(r, note{s: fmt.Sprintf("%s 다름:바이너리 %s (%s)", m, a[1], why), diff: true})
		default:
			r = append(r, note{s: fmt.Sprintf("%s 바이너리 깔기: 알 수 없는 응답 %q", m, l), bad: true})
		}
	}
	for _, t := range tools {
		switch {
		case seen[t] == 0:
			r = append(r, note{s: fmt.Sprintf("%s %s 바이너리 깔기 응답이 없다 — 다음 herdr-sync에서", m, t), bad: true})
		case seen[t] > 1:
			r = append(r, note{s: fmt.Sprintf("%s %s 바이너리 깔기 응답이 %d번 — 다음 herdr-sync에서 다시 본다", m, t, seen[t]), bad: true})
		}
	}
	return r
}

var slugRe = regexp.MustCompile(`^(?:https://github\.com/|git@github\.com:|ssh://git@github\.com/)([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+?)(?:\.git)?/?$`)

// githubSlug: publish.repo가 GitHub 저장소면 「주인/이름」
func githubSlug(url string) string {
	if m := slugRe.FindStringSubmatch(url); m != nil {
		return m[1]
	}
	return ""
}

// gh: 표준 출력·종료 코드·표준 오류(그대로도 흘린다). 상한(proc.Limit — gh는 HERDR_TIMEOUT_REMOTE)에 걸리면 124 = 모름
func gh(args ...string) (string, int, string) {
	var eb bytes.Buffer
	c := exec.Command("gh", args...)
	c.Stderr = io.MultiWriter(os.Stderr, &eb)
	out, rc := output(c)
	return out, rc, eb.String()
}

// buildMax: 릴리스 빌드 하나의 상한(A09) — 넘으면 그 무리째(컴파일러 자손까지) 끄고 이번 릴리스를 보류한다
func buildMax() time.Duration {
	if n, err := strconv.Atoi(os.Getenv("HERDR_TIMEOUT_BUILD")); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 600 * time.Second
}

var hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

// binsPrepare: 이번 실행의 바이너리 판(바이너리 줄이 없으면 nil). --check가 아니면 공개 내보내기(publish) 뒤 릴리스를 짓고 올린다 —
// 기계 맞추기보다 먼저(새 본체가 깔린 뒤에 설정·지움이 옛 포장을 걷게)
func binsPrepare(pubwarn func(string)) *relInfo {
	checked, pub := "", ""
	if !check {
		checked, pub = publish(pubwarn)
	}
	if len(BINS) == 0 {
		return nil
	}
	head, rc := gitq("-C", REPO, "rev-parse", "HEAD")
	src, err := srcDigest(REPO)
	if rc != 0 || len(head) < 12 || err != nil {
		pubwarn("바이너리 판을 읽지 못했다(범용 저장소 HEAD·소스) — 바이너리를 깔지 않는다")
		return &relInfo{c12: "unknown", src: "unknown"}
	}
	rel := &relInfo{c12: head[:12], src: src}
	if !check && pub != "" {
		release(rel, pub, checked, pubwarn)
	}
	return rel
}

// release: checked = herdr-tools-publish가 검사하고 내보낸 비공개 커밋(40자). 되면 rel에 받을 곳(tag·sums·lx)을 채운다
func release(rel *relInfo, pub, checked string, pubwarn func(string)) {
	if !isFile(REPO + "/go.mod") {
		return
	}
	slug := githubSlug(pub)
	if slug == "" {
		warn("릴리스 건너뜀 — 설정 publish.repo가 GitHub 저장소가 아니다. 기계는 각자 짓는다")
		return
	}
	gob := goBin()
	if gob == "" {
		pubwarn("릴리스 건너뜀 — 맥에 go가 없다(brew install go). 기계는 각자 짓는다")
		return
	}
	if !hex40.MatchString(checked) {
		pubwarn("릴리스 건너뜀 — herdr-tools-publish가 검사한 커밋을 알려 주지 않았다(HERDR_TOOLS_PUBLISH_RESULT). 기계는 각자 짓는다")
		return
	}
	ht, rc1 := gitq("-C", REPO, "rev-parse", "HEAD^{tree}")
	ct, rc2 := gitq("-C", REPO, "rev-parse", checked+"^{tree}")
	if rc1 != 0 || rc2 != 0 {
		pubwarn("릴리스 건너뜀 — 범용 저장소 커밋을 읽지 못했다")
		return
	}
	if ht != ct {
		pubwarn("릴리스 건너뜀 — 공개 검사한 커밋이 범용 저장소 HEAD(기계에 깐 소스)와 다르다(그사이 origin/main이 앞섬) — 다음 herdr-sync에서")
		return
	}
	if u := unknownTargets(BINS, machOS); len(u) > 0 { // 자산 목록(리눅스 자산 유무)이 그 기계 접속에 따라 갈리지 않게(R-218)
		pubwarn("릴리스 보류 — 바이너리 줄 대상 기계의 OS를 모른다(" + strings.Join(u, " ") + " — 연결 안 됨). 다음 herdr-sync에서, 기계는 각자 짓는다")
		return
	}
	c12, tag := checked[:12], "bin-"+checked[:12]
	srcd := TMP + "/relsrc"
	if err := os.MkdirAll(srcd, 0o700); err != nil {
		pubwarn("릴리스 건너뜀 — 임시 폴더")
		return
	}
	if err := exportTree(REPO, checked, srcd); err != nil {
		pubwarn("릴리스 건너뜀 — 검사한 커밋을 꺼내지 못했다 (" + err.Error() + ")")
		return
	}
	var tools []string
	seen := map[string]bool{}
	for _, b := range BINS {
		if !seen[b.tool] {
			seen[b.tool] = true
			tools = append(tools, b.tool)
		}
	}
	sort.Strings(tools)
	out := TMP + "/rel"
	if err := os.MkdirAll(out, 0o700); err != nil {
		pubwarn("릴리스 건너뜀 — 임시 폴더")
		return
	}
	cache := os.Getenv("GOCACHE")
	if cache == "" {
		cache = STATE + "/go-build" // 맥 한 곳에만 쌓인다
	}
	sums := map[string]string{} // 자산 이름 → sha256
	build := func(goos, goarch, t, name string) bool {
		b := exec.Command(gob, "build", "-trimpath", "-buildvcs=false", "-ldflags", "-X herdr-tools/internal/version.Commit="+c12,
			"-o", out+"/"+name, "./cmd/"+t)
		b.Dir = srcd
		b.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0", "GOTOOLCHAIN=local", "GOCACHE="+cache,
			"GOPATH="+STATE+"/gopath", "GOFLAGS=")
		b.Stdin, b.Stdout, b.Stderr = nil, os.Stderr, os.Stderr
		switch proc.WaitMax(b, "", buildMax()) {
		case 0:
		case proc.TimeoutCode:
			pubwarn("릴리스 빌드 시간 넘김 (" + name + ") — 이번 릴리스는 보류, 기계는 각자 짓는다")
			return false
		default:
			pubwarn("릴리스 빌드 실패 (" + name + ") — 기계는 각자 짓는다")
			return false
		}
		s, err := sha256File(out + "/" + name)
		if err != nil {
			pubwarn("릴리스 빌드 실패 (" + name + " sha256)")
			return false
		}
		sums[name] = s
		return true
	}
	assets := append([]string(nil), tools...)
	for _, t := range tools {
		if !build("darwin", "arm64", t, t) {
			return
		}
	}
	lxt := linuxTools(tools, BINS, machOS)
	lx := map[string]bool{}
	for _, t := range lxt {
		if !build("linux", "amd64", t, t+"-linux-amd64") {
			return
		}
		assets = append(assets, t+"-linux-amd64")
		lx[t] = true
	}
	var sb strings.Builder
	for _, a := range assets {
		fmt.Fprintf(&sb, "%s  %s\n", sums[a], a)
	}
	if os.WriteFile(out+"/SHA256SUMS", []byte(sb.String()), 0o644) != nil {
		pubwarn("릴리스 건너뜀 — SHA256SUMS를 쓰지 못했다")
		return
	}
	if !upload(slug, tag, c12, out, assets, sb.String(), pubwarn) {
		return
	}
	rel.c12, rel.base, rel.tag, rel.sums, rel.lx = c12, envOr("HERDR_SYNC_RELEASE_URL", "https://github.com/"+slug+"/releases/download"), tag, sums, lx
}

// upload: 릴리스를 만들거나 빠진 자산을 채운다. 이미 올린 자산은 덮지 않는다(A05). 받기로 넘어가도 되면 참(릴리스가 이 빌드와 같다고 확인됨)
func upload(slug, tag, c12, out string, tools []string, sums string, pubwarn func(string)) bool {
	files := func(names []string) []string {
		var r []string
		for _, n := range names {
			r = append(r, out+"/"+n)
		}
		return r
	}
	all := append(append([]string(nil), tools...), "SHA256SUMS")
	names, rc, errs := gh("release", "view", tag, "--repo", slug, "--json", "assets", "--jq", ".assets[].name")
	if rc != 0 {
		// gh가 그 태그의 릴리스가 없다고 한 줄로 답했을 때(「release not found」)만 없음이 확인됨 — 「proxy host not found」·저장소 404 같은
		// 연결·인증·이름 풀기 오류는 조회 실패(모름)다(R-12)
		if rc != proc.TimeoutCode && strings.TrimSpace(errs) == "release not found" {
			args := append([]string{"release", "create", tag, "--repo", slug, "--title", tag,
				"--notes", "herdr-tools " + c12 + " Go 도구 바이너리(<도구> darwin/arm64, <도구>-linux-amd64) — herdr-sync가 맥에서 지어 올림, 기계는 SHA256SUMS로 확인해 받는다"}, files(all)...)
			if _, rc, _ := gh(args...); rc != 0 {
				pubwarn("릴리스 만들기 실패 " + tag + " (gh release create — 맥 gh 인증·공개 미러 쓰기 권한) — 기계는 각자 짓는다")
				return false
			}
			say("릴리스 올림 %s", tag)
			return true
		}
		pubwarn("릴리스 조회 실패 " + tag + " (gh release view — 인증·통신·시간 넘김, 있는지 모름) — 이번에는 올리지 않는다, 기계는 각자 짓는다")
		return false
	}
	have := map[string]bool{}
	for _, n := range splitLines(names) {
		have[n] = true
	}
	if have["SHA256SUMS"] {
		old := TMP + "/relold"
		os.MkdirAll(old, 0o700)
		if _, rc, _ := gh("release", "download", tag, "--repo", slug, "--pattern", "SHA256SUMS", "--dir", old, "--clobber"); rc != 0 {
			pubwarn("릴리스 확인 실패 " + tag + " (gh release download SHA256SUMS) — 이번에는 받지 않는다, 기계는 각자 짓는다")
			return false
		}
		b, err := os.ReadFile(old + "/SHA256SUMS")
		if err != nil {
			pubwarn("릴리스 확인 실패 " + tag + " (받은 SHA256SUMS를 읽지 못했다)")
			return false
		}
		if string(b) != sums {
			pubwarn("릴리스 " + tag + "의 SHA256SUMS가 이번 빌드와 다르다 — 덮지 않는다(올린 자산을 지우지 않게). 기계는 각자 짓는다")
			return false
		}
	} else { // 명세 없이 자산만 남은 반쪽 릴리스: 남은 자산의 바이트가 이번 빌드와 같을 때만 명세를 올린다(R-13)
		if err := os.MkdirAll(TMP+"/relold", 0o700); err != nil {
			pubwarn("릴리스 확인 실패 " + tag + " (임시 폴더)")
			return false
		}
		for _, n := range tools {
			if !have[n] {
				continue
			}
			got := TMP + "/relold/" + n
			os.Remove(got)
			if _, rc, _ := gh("release", "download", tag, "--repo", slug, "--pattern", n, "--dir", TMP+"/relold", "--clobber"); rc != 0 {
				pubwarn("릴리스 확인 실패 " + tag + " (gh release download " + n + ") — 이번에는 올리지도 받지도 않는다, 다음 herdr-sync에서")
				return false
			}
			a, err1 := sha256File(got)
			w, err2 := sha256File(out + "/" + n)
			if err1 != nil || err2 != nil {
				pubwarn("릴리스 확인 실패 " + tag + " (받은 " + n + "의 sha256)")
				return false
			}
			if a != w {
				pubwarn("릴리스 " + tag + "에 남은 " + n + "이 이번 빌드와 다르다 — 명세를 올리지 않는다(맞지 않는 명세를 공개하지 않게). 기계는 각자 빌드한다")
				return false
			}
		}
	}
	var miss []string
	for _, n := range all {
		if !have[n] {
			miss = append(miss, n)
		}
	}
	if len(miss) == 0 {
		say("릴리스 같음 %s", tag)
		return true
	}
	if _, rc, _ := gh(append([]string{"release", "upload", tag, "--repo", slug}, files(miss)...)...); rc != 0 {
		pubwarn("릴리스 올리기 실패 " + tag + " (gh release upload — 맥 gh 인증·공개 미러 쓰기 권한) — 기계는 각자 짓는다")
		return false
	}
	say("릴리스 채움 %s (%s)", tag, strings.Join(miss, " "))
	return true
}
