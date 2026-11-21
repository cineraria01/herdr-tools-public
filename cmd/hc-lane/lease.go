package main

// 레인 임대(셸 land.sh lease_core·lease_x·lease_cmd): 프로젝트 원격 ref refs/hc-lanes/<레인>(빈 트리 커밋, 제목 「hc-lane lease <레인>
// <기계> <UTC 시각>」) 하나가 그 이름의 실행 하나다. 띄울 때 잡고(take — 띄우기 몫), land 성공·clean에서 놓는다.
//   take: 0 잡음(찍는 값 = 임대 커밋) · 3 누가 쥐고 있다(「값 제목」) · 4 원격 없음(「no-origin」) · 1 못 잡음·조회 실패 · 5 모름
//   show: 「값 제목」 또는 「-」. 0 · 1 조회 실패 · 4 원격 없음
//   drop: 기대값을 주면 그 값일 때만(다르면 3, 지금 값을 찍음), 안 주면 지금 값을 lease로 지움. 0 지움(없음·원격 없음 포함) · 1 실패
// --on(다른 기계)이면 그 기계의 로그인 셸(git 자격 도우미 PATH)에서 셸 판 lease_core를 그대로 돌린다(그 기계 hc-lane 판과 상관없이).

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"strings"
)

func init() { register("lease", leaseMain, can("lease")) }

var leaseBranch string // --branch(take의 임대 주인 표식 이름)

func leaseMain(a []string) int {
	o := mustParse("lease", a)
	leaseBranch = o.branch
	return leaseCmd(o.act, o.name)
}

func leaseCmd(act, name string) int {
	lo, r := leaseX(act, name, "")
	switch {
	case act == "show" && r == 0:
		fmt.Println(lo)
	case act == "show" && r == 4:
		if lo != "no-origin" {
			eprintf("hc-lane lease: 조회하지 못했다(종료 4)\n")
			return 1
		}
		fmt.Println("- (원격 없음 — 임대 없음)")
	case act == "drop" && r == 0:
		fmt.Printf("hc-lane lease: 놓음 refs/hc-lanes/%s\n", name)
	default:
		w := repo
		if on != "" {
			w = on + ":" + repo
		}
		eprintf("hc-lane lease: %s하지 못했다(git ls-remote·push origin, 종료 %d): %s\n", act, r, w)
		return 1
	}
	return 0
}

// leaseX <take|show|drop> <레인> [<기계|기대값>]: 대상 기계($on)의 repo에서. 출력(끝 줄바꿈 뺌)과 종료 코드
func leaseX(op, name, extra string) (string, int) {
	if on != "" || core != "" {
		var o bytes.Buffer
		rc := runCmdArgv(cmdOpt{stdout: &o, stderr: os.Stderr}, zxArgs(repo, "bash", "-c", leaseCoreSh+`; lease_core "$@"`, "hc-lane-lease", op, repo, name, extra, leaseBranch))
		return trim(o.String()), rc
	}
	if st, err := os.Stat(repo); err != nil || !st.IsDir() {
		eprintf("sh: cd: %s: No such file or directory\n", repo)
		return "", 1
	}
	var o bytes.Buffer
	rc := leaseCore(&o, op, repo, name, extra, leaseBranch)
	return trim(o.String()), rc
}

func runCmdArgv(o cmdOpt, argv []string) int { return runCmd(o, argv[0], argv[1:]...) }

// leaseCore: 이 기계에서(셸 lease_core와 같은 git 호출·출력)
func leaseCore(out *bytes.Buffer, op, r, n, x, owner string) int {
	ref := "refs/hc-lanes/" + n
	env := []string{"GIT_AUTHOR_NAME=hc-lane", "GIT_AUTHOR_EMAIL=hc-lane@localhost", "GIT_COMMITTER_NAME=hc-lane", "GIT_COMMITTER_EMAIL=hc-lane@localhost"}
	g := func(stdout, stderr *bytes.Buffer, stdin string, args ...string) int {
		o := cmdOpt{env: env}
		if stdout != nil {
			o.stdout = stdout
		}
		if stderr != nil {
			o.stderr = stderr
		} else {
			o.stderr = os.Stderr
		}
		if stdin != "" {
			o.stdin = strings.NewReader(stdin)
		}
		return runCmd(o, "git", append([]string{"-C", r}, args...)...)
	}
	quiet := &bytes.Buffer{}
	gout := func(args ...string) (string, int) {
		var b bytes.Buffer
		rc := g(&b, nil, "", args...)
		return trim(b.String()), rc
	}
	cur := func() (string, bool) {
		l, rc := gout("ls-remote", "origin", ref)
		if rc != 0 {
			return "", false
		}
		return firstWord(l), true
	}
	sub := func(v string) string {
		if g(nil, quiet, "", "cat-file", "-e", v+"^{commit}") != 0 {
			g(nil, quiet, "", "fetch", "-q", "--no-tags", "--no-write-fetch-head", "origin", ref)
		}
		var b bytes.Buffer
		if g(&b, quiet, "", "log", "-1", "--format=%s", v) != 0 {
			return "?"
		}
		return trim(b.String())
	}
	switch g(nil, quiet, "", "remote", "get-url", "origin") {
	case 0:
	case 2:
		out.WriteString("no-origin\n")
		if op == "drop" {
			return 0
		}
		return 4
	default:
		return 1
	}
	switch op {
	case "take":
		t, rc := gout("hash-object", "-t", "tree", "-w", "/dev/null")
		if rc != 0 {
			return 1
		}
		// 본문의 고유값: 같은 기계·같은 초·같은 이름이면 커밋 해시가 같아져 남의 임대를 자기 것으로 읽는다
		xx := x
		if xx == "" {
			xx = "-"
		}
		body := fmt.Sprintf("hc-lane lease %s %s %s\n\n%d-%d-%d\n", n, xx, utc(), os.Getpid(), rand.Intn(32768), rand.Intn(32768))
		var cb bytes.Buffer
		if g(&cb, nil, body, "commit-tree", t) != 0 {
			return 1
		}
		c := trim(cb.String())
		on := owner
		if on == "" {
			on = n
		}
		if g(nil, nil, "", "update-ref", "refs/hc-lease-owners/"+on, c) != 0 {
			return 1
		}
		if g(quiet, quiet, "", "push", "-q", "--porcelain", "origin", "--force-with-lease="+ref+":", c+":"+ref) == 0 {
			out.WriteString(c + "\n")
			return 0
		}
		v, ok := cur() // push 실패: 원격 값으로 판정(응답만 잃었을 수 있다)
		if !ok {
			return 5
		}
		if v == c {
			out.WriteString(c + "\n")
			return 0
		}
		if v != "" {
			out.WriteString(v + " " + sub(v) + "\n")
			return 3
		}
		return 1
	case "show":
		v, ok := cur()
		if !ok {
			return 1
		}
		if v != "" {
			out.WriteString(v + " " + sub(v) + "\n")
		} else {
			out.WriteString("-\n")
		}
		return 0
	case "drop":
		v, ok := cur()
		if !ok {
			return 1
		}
		if v == "" {
			return 0
		}
		if x != "" && x != v {
			out.WriteString(v + "\n")
			return 3
		}
		if g(quiet, quiet, "", "push", "-q", "--porcelain", "origin", "--force-with-lease="+ref+":"+v, ":"+ref) == 0 {
			return 0
		}
		if v, ok := cur(); ok && v == "" { // 지운 응답만 잃었는지
			return 0
		}
		return 1
	}
	return 1
}

// leaseCoreSh: --on용 — 셸 판 lease_core 그대로(그 기계의 bash -c로 보낸다)
const leaseCoreSh = `lease_core() {
  lc_op=$1 lc_r=$2 lc_n=$3 lc_x=${4-} lc_ref=refs/hc-lanes/$3
  lc_g() { GIT_AUTHOR_NAME=hc-lane GIT_AUTHOR_EMAIL=hc-lane@localhost GIT_COMMITTER_NAME=hc-lane GIT_COMMITTER_EMAIL=hc-lane@localhost git -C "$lc_r" "$@"; }
  lc_cur() { lc_l=$(lc_g ls-remote origin "$lc_ref" </dev/null) || return 1; printf '%s' "${lc_l%%[[:space:]]*}"; }
  lc_sub() {
    lc_g cat-file -e "$1^{commit}" 2>/dev/null || lc_g fetch -q --no-tags --no-write-fetch-head origin "$lc_ref" </dev/null 2>/dev/null
    lc_g log -1 --format=%s "$1" 2>/dev/null || echo '?'
  }
  lc_g remote get-url origin >/dev/null 2>&1; lc_o=$?
  if [ $lc_o = 2 ]; then echo no-origin; [ "$lc_op" != drop ] || return 0; return 4; fi
  [ $lc_o = 0 ] || return 1
  case $lc_op in
    take)
      lc_t=$(lc_g hash-object -t tree -w /dev/null) &&
        lc_c=$(printf 'hc-lane lease %s %s %s\n\n%s-%s-%s\n' "$lc_n" "${lc_x:--}" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$$" "$RANDOM" "$RANDOM" | lc_g commit-tree "$lc_t") || return 1
      lc_g update-ref "refs/hc-lease-owners/${5:-$lc_n}" "$lc_c" || return 1
      lc_g push -q --porcelain origin "--force-with-lease=$lc_ref:" "$lc_c:$lc_ref" </dev/null >/dev/null 2>&1 && { echo "$lc_c"; return 0; }
      lc_v=$(lc_cur) || return 5
      [ "$lc_v" != "$lc_c" ] || { echo "$lc_c"; return 0; }
      [ -z "$lc_v" ] || { echo "$lc_v $(lc_sub "$lc_v")"; return 3; }
      return 1 ;;
    show) lc_v=$(lc_cur) || return 1; if [ -n "$lc_v" ]; then echo "$lc_v $(lc_sub "$lc_v")"; else echo -; fi; return 0 ;;
    drop)
      lc_v=$(lc_cur) || return 1
      [ -n "$lc_v" ] || return 0
      [ -z "$lc_x" ] || [ "$lc_x" = "$lc_v" ] || { echo "$lc_v"; return 3; }
      lc_g push -q --porcelain origin "--force-with-lease=$lc_ref:$lc_v" ":$lc_ref" </dev/null >/dev/null 2>&1 && return 0
      lc_v=$(lc_cur) && [ -z "$lc_v" ] && return 0
      return 1 ;;
  esac
  return 1
}`
