//go:build !windows

package main

// 흔들림 다시 돌리기(flaky-retry, 2026-11-24): 병합기 반송 가운데 검증 실패의 절반 남짓이 부하에 흔들린 시험이었다(홀로 도는
// TestCotenantGuard의 시간 단언, 계약 시험의 띄우기 시간 넘김 등). 무거운 단계(홀로 단계·조각)에서 실패한 것을 다른 조각이 모두 끝난 뒤
// 하나씩 혼자 한 번 다시 돌린다 — 통과하면 「흔들림:」 줄·기록(사용자 캐시 herdr-tools/verify-flaky.log)을 남기고 통과로 친다.
// 다시 안 돎: 끔(HERDR_VERIFY_RETRY=0) · 실패한 것이 상한(HERDR_VERIFY_RETRY=<수>, 없으면 3)을 넘음 · 하나라도 경합(WARNING: DATA RACE)·
// 빌드 실패·[setup failed]·go 시간 넘김·실패한 시험 이름이 없음(고아·실행 실패) — 그때는 다시 돌려도 결과가 못 바뀌어 아무것도 다시 안 돈다.
// 다시 돌리기는 한 번뿐이다(통과할 때까지 되풀이하지 않는다). 흔들려 통과한 조각은 통과 기록(scope)을 남기지 않는다.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	retryDefault = 3 // 다시 돌릴 수 있는 실패 수의 기본 상한 — 넘으면 흔들림이 아니라 깨진 것
	flakyDays    = 7 // 검증 끝 요약 줄이 세는 기간(일)
)

// retryCand: 다시 돌릴 후보 — 무거운 단계에서 실패한 홀로 단계(i -1)나 조각(i), 그 첫 출력
type retryCand struct {
	i     int
	label string
	argv  []string
	env   []string
	out   string
}

// retryLimit: HERDR_VERIFY_RETRY — 없으면 retryDefault, 0이면 끔, 양의 정수면 상한. 틀린 값은 한 줄 알림 뒤 기본
func retryLimit(w io.Writer) int {
	v := os.Getenv("HERDR_VERIFY_RETRY")
	if v == "" {
		return retryDefault
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		fmt.Fprintf(w, "verify: HERDR_VERIFY_RETRY=%q 는 0 이상 정수가 아니라 기본 %d로 본다\n", v, retryDefault)
		return retryDefault
	}
	return n
}

var failLine = regexp.MustCompile(`(?m)^\s*--- FAIL: (\S+)`)

// failedTests: 출력의 「--- FAIL: <이름>」 가운데 잎(하위 시험이 따로 찍힌 부모는 뺀다), 처음 나온 차례
func failedTests(out string) []string {
	var all []string
	seen := map[string]bool{}
	for _, m := range failLine.FindAllStringSubmatch(out, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			all = append(all, m[1])
		}
	}
	var leaf []string
	for _, a := range all {
		parent := false
		for _, b := range all {
			if strings.HasPrefix(b, a+"/") {
				parent = true
				break
			}
		}
		if !parent {
			leaf = append(leaf, a)
		}
	}
	return leaf
}

// noRetry: 이 실패를 다시 돌리지 않을 까닭(빈 값이면 다시 돌 수 있다)
func noRetry(out string) string {
	switch {
	case strings.Contains(out, "WARNING: DATA RACE"):
		return "경합(WARNING: DATA RACE)"
	case strings.Contains(out, "[build failed]"):
		return "빌드 실패"
	case strings.Contains(out, "[setup failed]"):
		return "[setup failed]"
	case strings.Contains(out, "panic: test timed out"):
		return "go 시간 넘김(panic: test timed out)"
	case len(failedTests(out)) == 0:
		return "실패한 시험 이름이 없음(고아·실행 실패 등)"
	case strings.Contains(out, "실패 고아를 남김"):
		return "고아를 남김"
	}
	return ""
}

// retryPhase: 실패한 것들(cs)을 규칙대로 하나씩 혼자 한 번 다시 돌린다. run은 하나를 혼자 돌려 (종료 코드, 출력)을 준다 — 부르는 쪽이
// 다른 조각이 모두 끝난 뒤에 부른다. 돌려주는 것: 아직 실패인가 · 흔들려 통과한 것의 i · 끊김(128+신호, 아니면 0)
func retryPhase(root string, cs []retryCand, limit int, run func(retryCand) (int, string), w io.Writer) (fail bool, flaky []int, rc int) {
	if len(cs) == 0 {
		return false, nil, 0
	}
	if limit == 0 {
		return true, nil, 0 // 끔(HERDR_VERIFY_RETRY=0) — 지금까지처럼
	}
	if len(cs) > limit {
		fmt.Fprintf(w, "다시 안 돎: 실패 %d개가 상한 %d를 넘음 — 흔들림이 아니라 깨진 것으로 본다 (상한 HERDR_VERIFY_RETRY=<수>)\n", len(cs), limit)
		return true, nil, 0
	}
	stuck := false
	for _, c := range cs {
		if why := noRetry(c.out); why != "" {
			fmt.Fprintf(w, "다시 안 돎: %s — %s\n", c.label, why)
			stuck = true
		}
	}
	if stuck {
		return true, nil, 0 // 다시 돌려도 결과가 바뀌지 않는다
	}
	twice := 0
	for _, c := range cs {
		tests := strings.Join(failedTests(c.out), ", ")
		fmt.Fprintf(w, "== 다시 %s — 다른 조각이 다 끝난 뒤 혼자 한 번 (처음 실패: %s)\n", c.label, tests)
		r, _ := run(c)
		if r >= 128 {
			return true, flaky, r
		}
		if r != 0 {
			fmt.Fprintf(w, "두 번 실패: %s — %s\n", c.label, tests)
			twice++
			continue
		}
		fmt.Fprintf(w, "흔들림: %s — %s · 혼자 다시 돌려 통과\n", c.label, tests)
		flaky = append(flaky, c.i)
		if err := flakyRecord(root, c.label, failedTests(c.out)); err != nil {
			fmt.Fprintf(w, "경고 흔들림 기록을 못 남겼다: %v\n", err)
		}
	}
	if twice > 0 {
		fmt.Fprintf(w, "실패 두 번 실패 %d개 (위 「두 번 실패:」 줄)\n", twice)
	}
	return twice > 0, flaky, 0
}

// flakyFile: 흔들림 기록(통과 기록 곁, scope.go) — 한 줄 「시각\t트리\t조각\t시험(쉼표)\t부하」(시험이 바꾼다)
var flakyFile = func() string { return filepath.Join(scopeCache(), "verify-flaky.log") }

// ponytail: 기록은 덧붙이기만 한다 — 흔들림은 드물어 작다. 커지면 기간 지난 줄 걷기를 더한다
func flakyRecord(root, label string, tests []string) error {
	p := flakyFile()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tree, err := gitIn(root, "rev-parse", "-q", "--verify", "HEAD^{tree}")
	if err != nil || tree == "" {
		tree = "?" // 저장소 밖·git 실패 — 모름은 모름으로 적는다
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\n", time.Now().Format(time.RFC3339), tree, label, strings.Join(tests, ","), loadAvg())
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// loadAvg: 그때 기계 부하(1·5·15분 평균)와 코어 수. 못 읽으면 "?"
func loadAvg() string {
	s := ""
	if b, err := exec.Command("sysctl", "-n", "vm.loadavg").Output(); err == nil { // macOS: 「{ 3.1 2.0 1.5 }」
		s = strings.Trim(strings.TrimSpace(string(b)), "{} ")
	} else if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if f := strings.Fields(string(b)); len(f) >= 3 {
			s = strings.Join(f[:3], " ")
		}
	}
	if s == "" {
		return "?"
	}
	return fmt.Sprintf("load %s / %d코어", s, runtime.NumCPU())
}

type flakyStat struct {
	test string
	n    int
	last time.Time
}

// flakyStats: since 뒤 기록을 시험마다 센다(많은 것부터, 같으면 이름순). 기록이 없으면 빈 목록
func flakyStats(since time.Time) ([]flakyStat, error) {
	f, err := os.Open(flakyFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := map[string]*flakyStat{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		c := strings.Split(sc.Text(), "\t")
		if len(c) < 4 {
			continue
		}
		t, err := time.Parse(time.RFC3339, c[0])
		if err != nil || t.Before(since) {
			continue
		}
		for _, name := range strings.Split(c[3], ",") {
			if name == "" {
				continue
			}
			s := m[name]
			if s == nil {
				s = &flakyStat{test: name}
				m[name] = s
			}
			s.n++
			if t.After(s.last) {
				s.last = t
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := make([]flakyStat, 0, len(m))
	for _, s := range m {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		return out[i].test < out[j].test
	})
	return out, nil
}

// flakySummary: 검증 끝 한 줄 — 최근 flakyDays일 이 기계의 흔들림(없으면 안 찍는다)
func flakySummary(w io.Writer) {
	st, err := flakyStats(time.Now().AddDate(0, 0, -flakyDays))
	if err != nil {
		fmt.Fprintf(w, "경고 흔들림 기록을 못 읽었다: %v\n", err)
		return
	}
	if len(st) == 0 {
		return
	}
	var parts []string
	for _, s := range st {
		parts = append(parts, fmt.Sprintf("%s %d번", s.test, s.n))
	}
	fmt.Fprintf(w, "이 기계에서 최근 %d일 흔들림: %s (시험별: verify --flaky — 쌓인 시험은 고친다)\n", flakyDays, strings.Join(parts, ", "))
}

// flakyReport: verify --flaky — 기록 전부를 시험마다(횟수·마지막). 종료 0, 기록을 못 읽으면 1
func flakyReport(w io.Writer) int {
	st, err := flakyStats(time.Time{})
	if err != nil {
		fmt.Fprintf(w, "실패 흔들림 기록 %s: %v\n", flakyFile(), err)
		return 1
	}
	if len(st) == 0 {
		fmt.Fprintf(w, "흔들림 기록 없음 (%s)\n", flakyFile())
		return 0
	}
	fmt.Fprintf(w, "흔들림 기록 %s — 시험별 횟수 (쌓인 시험은 고친다)\n", flakyFile())
	for _, s := range st {
		fmt.Fprintf(w, "%4d  %s  (마지막 %s)\n", s.n, s.test, s.last.Local().Format("2006-01-02 15:04"))
	}
	return 0
}
