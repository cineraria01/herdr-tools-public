package main

// 설정(hc-fill.tsv)·이름 규칙·지시문 검사·기본 가지·착지 판정 — 셸 판 lib/hc-fill/common.sh의 같은 이름 함수

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"syscall"
	"unicode"
)

var confLines []string // 설정 파일 줄(main이 한 번 읽음)

// fields: awk 기본 FS(공백·탭)로 나눔
func fields(l string) []string { return strings.Fields(l) }

// restAfter: 앞 n칸과 그 뒤 공백을 뗀 나머지(끝 공백 뗌) — sub(/^[ \t]*[^ \t]+[ \t]+…/, "")
func restAfter(l string, n int) string {
	s := strings.TrimLeft(l, " \t")
	for i := 0; i < n; i++ {
		j := strings.IndexAny(s, " \t")
		if j < 0 {
			return ""
		}
		s = strings.TrimLeft(s[j:], " \t")
	}
	return strings.TrimRight(s, " \t")
}

// conf <키>: 그 키 첫 줄의 나머지(키 혼자면 빈 값). 살아 있는 손잡이(hc-fill knob)가 있으면 그 값이 앞선다(knob.go)
func conf(k string) string {
	if v, ok := knobConf(k); ok {
		return v
	}
	return confRaw(k)
}

// confRaw: 손잡이를 보지 않은 설정 줄 값
func confRaw(k string) string {
	for _, l := range confLines {
		if f := fields(l); len(f) > 0 && f[0] == k {
			return restAfter(l, 1)
		}
	}
	return ""
}

// Machine: machine 줄 — 이름 상한 레인상한(-) 셈(-)
type Machine struct{ Name, Cap, LCap, All string }

func machines() []Machine {
	var ms []Machine
	for _, l := range confLines {
		f := fields(l)
		if len(f) == 0 || f[0] != "machine" {
			continue
		}
		m := Machine{F1(f, 1), F1(f, 2), dash(F1(f, 3)), dash(F1(f, 4))}
		if v, ok := knobConf("seats@" + m.Name); ok { // 실행 중 자리 조정(hc-fill machine <기계> --seats, loadsteer.go)
			m.Cap = v
		}
		ms = append(ms, m)
	}
	return ms
}

// F1: 0부터 센 칸(없으면 빈 값)
func F1(f []string, i int) string {
	if i < len(f) {
		return f[i]
	}
	return ""
}

func machineKnown(m string) bool {
	for _, x := range machines() {
		if x.Name == m {
			return true
		}
	}
	return false
}

// repo_of <프로젝트> <기계>: 그 기계 저장소 경로
func repoOf(p, m string) string {
	for _, l := range confLines {
		f := fields(l)
		if len(f) >= 3 && f[0] == "repo" && f[1] == p && f[2] == m {
			return restAfter(l, 3)
		}
	}
	return ""
}

func projectKnown(p string) bool {
	for _, l := range confLines {
		if f := fields(l); len(f) >= 2 && f[0] == "repo" && f[1] == p {
			return true
		}
	}
	return false
}

// confRows <종류>: 그 종류 줄의 칸들
func confRows(kind string) [][]string {
	var o [][]string
	for _, l := range confLines {
		if f := fields(l); len(f) > 0 && f[0] == kind {
			o = append(o, f)
		}
	}
	return o
}

// kpre <프로젝트>: 그 프로젝트 repo 줄 경로(모든 기계, 끝 / 뗌)
func kpre(p string) []string {
	var o []string
	for _, l := range confLines {
		f := fields(l)
		if len(f) >= 2 && f[0] == "repo" && f[1] == p {
			o = append(o, strings.TrimRight(restAfter(l, 3), "/"))
		}
	}
	return o
}

// 우선순위
func pnorm(s string) (string, bool) {
	switch s {
	case "긴급", "urgent":
		return "urgent", true
	case "높음", "high":
		return "high", true
	case "보통", "normal":
		return "normal", true
	case "낮음", "low":
		return "low", true
	}
	return "", false
}

func plabel(s string) string {
	switch s {
	case "urgent":
		return "긴급"
	case "high":
		return "높음"
	case "low":
		return "낮음"
	}
	return "보통"
}

var nameRe = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

func okname(s string) bool { return nameRe.MatchString(s) }

// kindwhy <이름>: 커밋 종류 낱말과 같은 이름이면 사유
func kindwhy(s string) (string, bool) {
	switch s {
	case "feat", "fix", "docs", "style", "refactor", "perf", "test", "build", "ci", "chore", "revert":
		return fmt.Sprintf("레인 이름이 커밋 종류(%s)와 같으면 착지·선행 판정이 아무 %s: 커밋에나 맞는다 — 다른 이름을 쓴다", s, s), true
	}
	return "", false
}

var (
	needLineRe = regexp.MustCompile(`^[[:space:]]*([-*][[:space:]]*)?필요:[[:space:]]*`)
	needOkRe   = regexp.MustCompile(`^[a-z0-9_-]+(,[a-z0-9_-]+)*$`)
	needSepRe  = regexp.MustCompile(`[ \t]*,[ \t]*`)
)

// needs <지시문>: 「필요: <능력>, …」 줄의 능력을 쉼표 목록으로(여러 줄이면 합침)
func needs(path string) string {
	var caps []string
	for _, l := range readLines(path) {
		loc := needLineRe.FindStringIndex(l)
		if loc == nil {
			continue
		}
		g := needSepRe.ReplaceAllString(l[loc[1]:], ",")
		g = strings.TrimRight(g, " \t")
		if needOkRe.MatchString(g) {
			caps = append(caps, strings.Split(g, ",")...)
		}
	}
	return strings.Join(uniq(caps), ",")
}

// mhas <기계> <능력 쉼표 목록|빈 값>: 설정 machine-has에 다 있으면 참
func mhas(m, caps string) bool {
	if caps == "" {
		return true
	}
	var hv []string
	for _, f := range confRows("machine-has") {
		if len(f) >= 2 && f[1] == m {
			for _, x := range f[2:] {
				hv = append(hv, strings.Split(x, ",")...)
			}
		}
	}
	for _, c := range words(caps) {
		if !has(hv, c) {
			return false
		}
	}
	return true
}

var appendixRe = regexp.MustCompile(`^#+[ \t]*(착지·정리|감사·착지|먼저 띄움|결과 보고)`)

func blank(s string) bool { return strings.TrimLeft(s, " \t\n\v\f\r") == "" }

// brief_bad <지시문>: 실제로 열어 읽어 본다. 못 읽거나 비었으면 (까닭, 참)
func briefBad(path string) (string, bool) {
	if !isFile(path) {
		return "없음", true
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return errText(err), true
	}
	s := string(b)
	if blank(s) {
		return "빈 파일", true
	}
	a, f := false, false
	for _, l := range lines(s) {
		if appendixRe.MatchString(l) {
			a = true
			break
		}
		if !blank(l) && !strings.HasPrefix(strings.TrimLeft(l, " \t"), "#") {
			f = true
			break
		}
	}
	if a && !f {
		return "본문 없음(제목·부록 절만)", true
	}
	return "", false
}

// errText: cat 오류의 끝 부분(「Permission denied」 꼴 — C strerror처럼 첫 글자 대문자)
func errText(err error) string {
	var en syscall.Errno
	if errors.As(err, &en) {
		s := en.Error()
		r := []rune(s)
		if len(r) > 0 {
			r[0] = unicode.ToUpper(r[0])
		}
		return string(r)
	}
	return "읽기 실패"
}

// project_ref <저장소>: 기본 가지의 실제 참조. 원격 부재(2)와 Git 오류를 구분한다. ok=false면 모름
func projectRef(repo string) (string, bool) {
	if r, hit := pcRef[repo]; hit { // 바퀴에 저장소마다 한 번(aftercost.go)
		return r.out, r.ok
	}
	ref, ok := projectRefRead(repo)
	pcRef[repo] = pcRes{ref, ok}
	return ref, ok
}

func projectRefRead(repo string) (string, bool) {
	var ref string
	_, rc := run("git", "-C", repo, "remote", "get-url", "origin")
	switch rc {
	case 0:
		o, rc2 := run("git", "-C", repo, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD")
		if rc2 == 0 {
			ref = sub(o)
		} else {
			if _, rc3 := run("git", "-C", repo, "show-ref", "--exists", "refs/remotes/origin/HEAD"); rc3 != 2 {
				return "", false
			}
			ref = "origin/main"
		}
	case 2:
		rc3 := 0
		for _, r := range []string{"refs/heads/main", "refs/heads/master"} {
			ref = r
			_, rc3 = run("git", "-C", repo, "show-ref", "--exists", r)
			if rc3 == 0 {
				break
			}
			if rc3 != 2 {
				return "", false
			}
		}
		if rc3 != 0 {
			return "", false
		}
	default:
		return "", false
	}
	if ref == "" || strings.Trim(ref, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/_.-") != "" {
		return "", false
	}
	return ref, true
}

// projectRefSh: 원격 기계에서 돌릴 같은 함수(셸 판 declare -f project_ref 자리)
const projectRefSh = `project_ref() {
  local ref rc
  git -C "$1" remote get-url origin >/dev/null 2>&1; rc=$?
  case $rc in
    0) if ! ref=$(git -C "$1" symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null); then
         git -C "$1" show-ref --exists refs/remotes/origin/HEAD >/dev/null 2>&1
         [ $? = 2 ] || return 1
         ref=origin/main
       fi ;;
    2) for ref in refs/heads/main refs/heads/master; do
         git -C "$1" show-ref --exists "$ref" >/dev/null 2>&1; rc=$?
         [ $rc != 0 ] || break
         [ $rc = 2 ] || return 1
       done
       [ $rc = 0 ] || return 1 ;;
    *) return 1 ;;
  esac
  case $ref in ''|*[!a-zA-Z0-9/_.-]*) return 1 ;; esac
  printf '%s\n' "$ref"
}`

// landed <레인>: FR 저장소 FB에서 제목 머리가 그 레인인 첫 커밋 줄(「%h %s」). ok=false면 git 실패(모름 — 반쪽 출력은 버림)
func landed(lane string) (string, bool) {
	o, ok := pcSubjects(FR, FB) // 바퀴에 저장소·가지마다 한 번(aftercost.go)
	if !ok {
		return "", false
	}
	re := regexp.MustCompile(`^[0-9a-f]+ ([a-z]+(\([^)]*\))?!?: *)?` + regexp.QuoteMeta(lane) + `([^a-z0-9_-]|$)`)
	for _, l := range lines(o) {
		if strings.Contains(l, lane) && re.MatchString(l) {
			return l, true
		}
	}
	return "", true
}
