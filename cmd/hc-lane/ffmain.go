package main

// ffmain <메인 폴더> <기본>: 메인 폴더를 origin/<기본>으로 fast-forward(셸 land.sh ffmain·mainbusy). 기본 브랜치에 있고 추적 파일
// 변경·진행 중인 merge·rebase·skip-worktree 표시가 없고, 메인 폴더를 cwd로 쓰는 일하는 터미널(자신 빼고)이 없을 때만
// merge --ff-only --no-squash --no-overwrite-ignore. 결과 한 줄(앞당김·최신은 표준 출력, 건너뜀은 오류 출력), 늘 계속(착지는 막지 않음).
// --on(다른 기계)이면 그 기계의 hc exec에 셸 판과 같은 스크립트를 보낸다.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"herdr-tools/internal/hcjson"
)

// jrec: JSON을 { 마다(와 줄마다) 끊어 조각마다 키의 문자열·정수 값(없으면 -)을 — 첫 키가 없는 조각은 버린다(셸 jrec, jq 없이).
// 판정(있음·없음)에는 쓰지 않는다 — hc 응답은 internal/hcjson으로(남은 쓰임은 시험)
func jrec(js string, keys ...string) [][]string {
	var o [][]string
	for _, frag := range strings.FieldsFunc(js, func(r rune) bool { return r == '{' || r == '\n' }) {
		row := make([]string, len(keys))
		skip := false
		for i, k := range keys {
			v := "-"
			re := regexp.MustCompile(`"` + regexp.QuoteMeta(k) + `" *: *("[^"]*"|-?[0-9]+)`)
			if m := re.FindStringSubmatch(frag); m != nil {
				v = strings.TrimSuffix(strings.TrimPrefix(m[1], `"`), `"`)
			}
			if i == 0 && v == "-" {
				skip = true
				break
			}
			row[i] = v
		}
		if !skip {
			o = append(o, row)
		}
	}
	return o
}

// mainbusy: 그 기계에서 그 폴더(아래)를 cwd로 쓰는 일하는 터미널 주소들(끝에 빈칸). hc ls 실패·꼴을 못 읽으면 false.
// 일하는 터미널: status가 working·blocked, 또는 status가 없는데 hc get의 foreground pid가 터미널 pid와 다름(셸 위에 무엇이 돎)
func mainbusy(dir string) (string, bool) {
	var b bytes.Buffer
	if runCmd(cmdOpt{stdout: &b}, "hc", "ls", "--json") != 0 {
		return "", false
	}
	l, err := hcjson.ParseList(b.Bytes()) // 주소 없는 항목이 있으면 못 읽음, 모름 항목은 일하는 것으로(부분 목록으로 「아무도 안 씀」을 정하지 않는다)
	if err != nil {
		return "", false
	}
	pre := ""
	if on != "" {
		pre = on + "/"
	}
	var out strings.Builder
	for _, t := range l.Terminals {
		a, s, ss, c := t.Address, t.State, orHyph(t.Status), t.Cwd
		if !strings.HasPrefix(a, pre) {
			continue
		}
		a = strings.TrimPrefix(a, pre)
		if strings.Contains(a, "/") {
			continue
		}
		if on == "" && a == os.Getenv("HC_TERMINAL_ID") {
			continue
		}
		if t.Unknown != "" { // 그 기계의 모름 항목(R-3M01) — cwd를 몰라 그 폴더를 쓴다고 본다
			out.WriteString(pre + a + " ")
			continue
		}
		if s != "live" || !strings.HasPrefix(c+"/", dir+"/") {
			continue
		}
		switch ss {
		case "working", "blocked":
		case "-":
			// 셸만 떠 있음(foreground pid = 터미널 pid)이 확인될 때만 안 바쁨 — 조회 실패·잘림은 바쁨으로 둔다(앞당기지 않는 쪽)
			if gt, found, err := hcGet(10, pre+a); err == nil && found && gt.PID > 0 {
				var fg struct {
					PID int `json:"pid"`
				}
				if json.Unmarshal(gt.Raw["foreground"], &fg) == nil && fg.PID == gt.PID {
					continue
				}
			}
		default:
			continue
		}
		out.WriteString(pre + a + " ")
	}
	return out.String(), true
}

func ffmain(dir, b string) {
	var fr string
	if fb, ok := mainbusy(dir); !ok {
		fr = "건너뜀: hc ls 실패(메인 폴더에서 일하는 세션을 모름)"
	} else if fb != "" {
		fr = "건너뜀: 메인 폴더에서 일하는 세션 " + strings.TrimSuffix(fb, " ")
	} else if on != "" || core != "" {
		var o bytes.Buffer
		xRun(cmdOpt{stdout: &o}, "sh", "-c", ffScript, "hc-lane", dir, b)
		fr = trim(o.String())
	} else {
		fr = ffLocal(dir, b)
	}
	where := dir
	if on != "" {
		where = on + ":" + dir
	}
	switch {
	case strings.HasPrefix(fr, "앞당김") || strings.HasPrefix(fr, "최신"):
		fmt.Printf("hc-lane: 메인 폴더 %s (%s)\n", fr, where)
	case strings.HasPrefix(fr, "확인불가:"):
		eprintf("hc-lane: 경고 — 메인 폴더 %s (%s)\n", strings.TrimPrefix(fr, "확인불가: "), where)
	default:
		if fr == "" {
			fr = "건너뜀: 결과를 못 읽음"
		}
		eprintf("hc-lane: 경고 — 메인 폴더 앞당김 %s — 그대로 둠 (%s)\n", fr, where)
	}
}

// ffLocal: 이 기계의 메인 폴더에서(ffScript와 같은 판정, git 오류 출력은 버림)
func ffLocal(dir, b string) string {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "건너뜀: 폴더 없음"
	}
	g := func(args ...string) (string, int) {
		var o bytes.Buffer
		rc := runCmd(cmdOpt{dir: dir, stdout: &o}, "git", args...)
		return trim(o.String()), rc
	}
	g2 := func(args ...string) (string, int) {
		var o bytes.Buffer
		rc := runCmd(cmdOpt{dir: dir, stdout: &o, stderr: &o}, "git", args...)
		return trim(o.String()), rc
	}
	gd, rc := g("rev-parse", "--absolute-git-dir")
	if rc != 0 {
		return "건너뜀: git 폴더를 못 찾음"
	}
	if bare, _ := g("rev-parse", "--is-bare-repository"); bare == "true" {
		return "건너뜀: 맨(bare) 저장소"
	}
	for _, f := range []string{"MERGE_HEAD", "rebase-merge", "rebase-apply", "CHERRY_PICK_HEAD", "REVERT_HEAD"} {
		if _, err := os.Stat(gd + "/" + f); err == nil {
			return "건너뜀: 진행 중인 merge/rebase(" + f + ")"
		}
	}
	if h, _ := g("symbolic-ref", "-q", "--short", "HEAD"); h != b {
		if h == "" {
			h = "분리된 HEAD"
		}
		return "건너뜀: 기본 브랜치(" + b + ")가 아니다(" + h + ")"
	}
	// 조회가 실패하면 깨끗함·표시 없음으로 읽지 않는다
	if s, rc := g2("status", "--porcelain", "--untracked-files=no"); rc != 0 {
		return "건너뜀: git status 실패(작업 트리를 모름)"
	} else if s != "" {
		return "건너뜀: 추적 파일에 커밋 안 된 변경"
	}
	l, rc := g("ls-files", "-v")
	if rc != 0 {
		return "건너뜀: git ls-files 실패(색인 표시를 모름)"
	}
	for _, x := range strings.Split(l, "\n") {
		if x != "" && (x[0] >= 'a' && x[0] <= 'z' || x[0] == 'S') {
			return "건너뜀: skip-worktree·assume-unchanged 표시 파일이 있다"
		}
	}
	o, rc := g("rev-parse", "-q", "--verify", "refs/remotes/origin/"+b+"^{commit}")
	if rc != 0 {
		return "건너뜀: origin/" + b + " 없음"
	}
	a, _ := g("rev-parse", "--short", "HEAD")
	n, _ := g("rev-parse", "--short", o)
	if h, _ := g("rev-parse", "HEAD"); h == o {
		return "최신 " + a
	}
	if _, rc := g("merge-base", "--is-ancestor", "HEAD", o); rc != 0 {
		return "건너뜀: 로컬 " + b + " 에 origin/" + b + " 에 없는 커밋이 있다"
	}
	var mo bytes.Buffer // 작업 트리를 바꾸는 merge는 상한 없이(cmdOpt.noLimit — 중간에 끊으면 반쯤 바뀐 메인 폴더가 남는다)
	if rc := runCmd(cmdOpt{dir: dir, stdout: &mo, stderr: &mo, noLimit: true}, "git", "merge", "--ff-only", "--no-squash", "--no-overwrite-ignore", "-q", o); rc != 0 {
		e := trim(mo.String())
		return "건너뜀: git이 거절 — " + strings.SplitN(e, "\n", 2)[0]
	}
	// merge는 이미 성공했다 — HEAD를 못 읽으면 「그대로 둠」이 아니라 확인 불가로
	h2, rc := g("rev-parse", "-q", "--verify", "HEAD")
	if rc != 0 {
		return "확인불가: merge는 성공했는데 최종 HEAD를 확인하지 못했다(git rev-parse 실패) — 폴더가 " + n + "으로 갔을 수 있다"
	}
	if h2 != o {
		switch _, c := g("merge-base", "--is-ancestor", o, h2); c {
		case 0:
			s, rc := g("rev-parse", "--short", h2)
			if rc != 0 {
				s = h2
			}
			return "앞당김 " + a + "→" + s + " (목표 " + n + "을 품음 — 다른 착지가 더 앞당김)"
		case 1:
			return "건너뜀: merge 뒤 HEAD가 origin/" + b + " 를 품지 않는다(저장소 merge 설정?)"
		default:
			return fmt.Sprintf("확인불가: merge는 성공했는데 최종 HEAD가 목표를 품는지 확인하지 못했다(git merge-base 종료 %d)", c)
		}
	}
	return "앞당김 " + a + "→" + n
}

// ffScript: --on·코어 문맥용(셸 판 ffmain의 스크립트 그대로). 결과 한 줄을 표준 출력에
const ffScript = `cd "$1" 2>/dev/null || { echo "건너뜀: 폴더 없음"; exit 1; }
    b=$2 gd=$(git rev-parse --absolute-git-dir 2>/dev/null) || { echo "건너뜀: git 폴더를 못 찾음"; exit 1; }
    [ "$(git rev-parse --is-bare-repository 2>/dev/null)" != true ] || { echo "건너뜀: 맨(bare) 저장소"; exit 1; }
    for f in MERGE_HEAD rebase-merge rebase-apply CHERRY_PICK_HEAD REVERT_HEAD; do
      [ ! -e "$gd/$f" ] || { echo "건너뜀: 진행 중인 merge/rebase($f)"; exit 1; }
    done
    h=$(git symbolic-ref -q --short HEAD)
    [ "$h" = "$b" ] || { echo "건너뜀: 기본 브랜치($b)가 아니다(${h:-분리된 HEAD})"; exit 1; }
    s=$(git status --porcelain --untracked-files=no 2>&1) || { echo "건너뜀: git status 실패(작업 트리를 모름)"; exit 1; }
    [ -z "$s" ] || { echo "건너뜀: 추적 파일에 커밋 안 된 변경"; exit 1; }
    l=$(git ls-files -v 2>/dev/null) || { echo "건너뜀: git ls-files 실패(색인 표시를 모름)"; exit 1; }
    ! printf "%s\n" "$l" | grep -q "^[a-zS]" || { echo "건너뜀: skip-worktree·assume-unchanged 표시 파일이 있다"; exit 1; }
    o=$(git rev-parse -q --verify "refs/remotes/origin/$b^{commit}") || { echo "건너뜀: origin/$b 없음"; exit 1; }
    a=$(git rev-parse --short HEAD) n=$(git rev-parse --short "$o")
    [ "$(git rev-parse HEAD)" != "$o" ] || { echo "최신 $a"; exit 0; }
    git merge-base --is-ancestor HEAD "$o" || { echo "건너뜀: 로컬 $b 에 origin/$b 에 없는 커밋이 있다"; exit 1; }
    e=$(git merge --ff-only --no-squash --no-overwrite-ignore -q "$o" 2>&1) || { echo "건너뜀: git이 거절 — $(printf %s "$e" | head -n 1)"; exit 1; }
    h2=$(git rev-parse -q --verify HEAD) || { echo "확인불가: merge는 성공했는데 최종 HEAD를 확인하지 못했다(git rev-parse 실패) — 폴더가 ${n}으로 갔을 수 있다"; exit 1; }
    if [ "$h2" != "$o" ]; then
      git merge-base --is-ancestor "$o" "$h2"; c=$?
      case $c in
        0) echo "앞당김 ${a}→$(git rev-parse --short "$h2" 2>/dev/null || echo "$h2") (목표 ${n}을 품음 — 다른 착지가 더 앞당김)"; exit 0 ;;
        1) echo "건너뜀: merge 뒤 HEAD가 origin/$b 를 품지 않는다(저장소 merge 설정?)"; exit 1 ;;
        *) echo "확인불가: merge는 성공했는데 최종 HEAD가 목표를 품는지 확인하지 못했다(git merge-base 종료 $c)"; exit 1 ;;
      esac
    fi
    echo "앞당김 ${a}→${n}"`
