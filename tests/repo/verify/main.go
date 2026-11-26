//go:build !windows

// Command verify: herdr-tools 저장소 검증 본문 — .claude/herdr-verify.sh가 저장소 뿌리에서 짓고 exec 한다(2026-11-22 go-shell-remove:
// 옛 셸 검증의 병렬 실행기·단계·셸 시험 목록을 걷고 go 명령 몇 개로). 표준 라이브러리만 쓴다(레인이 internal을 깨도 검증은 지어지게).
//
//	verify [--cheap-only | --flaky]
//
// 싼 단계: go vet ./... · go test ./tests/repo/(gofmt·셸 문법·줄 수 예산·개인 값·비밀 꼴·싼 검사 빠짐). --cheap-only면 여기까지
// (.claude/herdr-precheck.sh — 병합기 제출 직전). 싼 단계가 실패하면 무거운 단계를 건너뛰고 1(HERDR_VERIFY_ALL=1이면 끝까지).
// 무거운 단계 앞에서 기계 단위 검증 잠금(사용자 캐시 폴더 herdr-tools/verify.lock, flock)을 얻는다 — 다른 검증이 쥐고 있으면 기다리고
// (기다린 초 한 줄, 검증 시간에서 뺀다), 30분을 넘기면 경고 한 줄 뒤 그대로 진행. HERDR_VERIFY_NO_LOCK=1이면 잠금 없이(2026-11-23
// verify-shard-tune: 조각 자리가 코어 수라 검증 둘이 겹치면 코어의 몇 배로 떠 시간에 민감한 시험이 실패했다).
// HERDR_VERIFY_GATE=1(병합기가 병합 검증 자리를 받은 기계에 띄운 검증)이면 그 잠금 대신 같은 폴더의 verify-gate.lock(lockFor)을 쥐어
// 레인 검증과 함께 돌고, 1분 부하를 기다리는 홀로 단계를 조각 뒤로 미룬다(docs/hc-lane/merger-spec-verify.md 「병합 검증 자리」).
// 무거운 단계: go test -race(Go 계약 시험 전부). TestCotenantGuard는 1분 부하를 재는 시험이라 먼저 홀로 돈다
// (2026-11-22 verify-go-timeout — 다른 꾸러미와 함께 돌면 10분을 넘겼다). 그다음 조각(go test 여럿)을 함께 돌린다
// (2026-11-23 verify-shard — 꾸러미 안 시험은 차례로 돌아 검증 전체가 tests/hclane 하나의 시간이었다: 병합기 970초 중 927초):
// splits의 꾸러미는 part마다 조각 하나(-run <part>, -skip <앞 part 전부>)와 나머지 조각(-skip <part 전부>)으로, 그 밖 꾸러미는 모아
// 한 조각(-skip ^TestCotenantGuard$ <꾸러미…>)으로 — 시험(하위 시험)마다 처음 고른 조각 하나에서만 돈다.
// 조각 출력은 끝날 때 한 덩이로(「-- [i/n] … 끝」 다음) 찍는다. 함께 도는 자리 수는 HERDR_VERIFY_JOBS=<수>(없으면 코어 수)다:
// 조각은 한 자리, 나머지 조각은 go test -p <그 1/3>로 그만큼 자리를 쓴다(그 값을 주면 cotenant 단계의 -p에도).
// go list가 실패하면 옛 꼴 한 번(-skip ^TestCotenantGuard$ ./...).
// 홀로 단계·조각이 실패하면 조각이 다 끝난 뒤 혼자 한 번 다시 돌린다(retry.go — HERDR_VERIFY_RETRY=0 끔·=<수> 상한, 기본 3).
// --flaky는 이 기계의 흔들림 기록을 시험별로 보이고 끝난다.
// macOS면 조각 단계 앞에 메모리 디스크를 만들어 조각의 TMPDIR로 주고 끝에 뗀다(ramtmp.go — HERDR_VERIFY_RAMTMP=0 끔·=<MB> 크기,
// 못 만들면 한 줄 알림 뒤 보통 임시 폴더로).
//
// 고아 검사: 단계마다 go 명령을 새 프로세스 무리로 띄운다(함께 도는 조각들은 한 단계 — 무리는 조각이 끝나자마자, 떼어 띄운 것은
// 다 끝난 뒤 한 번). 끝난 뒤 그 무리에 남은
// 프로세스, 또는 그 단계 중 새로 생겨 부모가 1이 된 프로세스 가운데 명령 줄이나 cwd에 이 폴더 경로가 든 것(과 그 자손)은 고아다:
// 목록(pid·cwd·명령)을 찍고 끄고(TERM, 3초 뒤 KILL) 실패. cwd로만 걸리고 명령 줄에 /.claude/가 든 것(이 폴더에서 도는 Claude 세션의
// 훅·플러그인이 떼어 띄운 것)은 뺀다. INT·TERM·HUP을 받으면 도는 무리에 TERM(10초 뒤 KILL)을 보내고 128+신호로 끝난다.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	heavyTimeout = "40m"          // 꾸러미 시험 바이너리 하나의 상한(go test 기본 10분은 tests/hclane·hcfill에 짧다)
	mod          = "herdr-tools/" // go.mod의 모듈 — splits의 꾸러미는 가져오기 경로
	cotenant     = "^TestCotenantGuard$"
)

// part: 오래 걸리는 꾸러미에서 따로 떼는 시험(-run 꼴 — 하위 시험 묶음은 '^TestX$/^(a|b)$'). est는 실측 초(띄우는 차례에만 쓴다)
type part struct {
	run string
	est int
}

// split: 꾸러미와 part들. rest는 나머지 조각의 실측 초
type split struct {
	pkg   string
	parts []part
	rest  int
}

// splits: 2026-11-23 워커 맥(10코어) 실측(-json, 조각 없이 돈 시험 시간 — 병합기 기계도 비슷). hc-lane·hc-fill 계약 시험은 절마다
// 하위 시험이고 절은 저마다 새 상태에서 돈다(2026-11-24 contract-subtests) — 묶음은 시간만 보고 나눈다(옛 판은 앞 절 상태에 기대는
// s20c←s20b·s21a2~4←s21a1을 못 뗐고, hc-fill은 HCFILL_SEC 환경 변수로 절 묶음을 골랐다).
// 꾸러미 이름이 바뀌면 그 split은 그냥 안 쓰인다(그 꾸러미는 나머지 한 조각에서 통째로 돈다).
// est는 조각들이 자리 10(코어 수)에서 함께 돌 때 잰 초(2026-11-23 verify-shard-tune) — 긴 것부터 띄우고, 짧은 조각이 끝물의 빈자리를
// 채운다. 이름 범위 part(^TestRecovery[A-C] 등)는 새 시험이 이름대로 저절로 들고, 어디에도 안 맞으면 그 꾸러미 나머지 조각에 든다.
// 병합기·herdr-sync 계약 시험은 묶음마다 최상위 시험(TestMergerContractA~E·TestContractA~E, 2026-11-23 contract-split)이라 part
// 하나씩 — 새 묶음은 이름대로 part를 더한다(안 더하면 그 꾸러미 나머지 조각에 든다). s21b1은 절 하나라 표로는 못 나누지만 나눌
// 까닭도 없어졌다: 약 290초가 디스크가 아니라 기다림 — 레인 띄우기 넷이 가짜 hc에서 시작 확인 상한(lane.start_wait 60초)을 끝까지
// 돌았다 — 이라 --start-wait 0을 주어 약 50~70초다(2026-11-24 s21b1-trim, 남은 기다림은 「다시 안 감」을 보는 고정 pause 약 29초).
// 나눈 뒤 실측(워커 맥 10코어 3회): 가장 긴 조각 538→305초(s21b1)지만 검증은 557~574초로 그대로 — 조각 초의 합(약 4800초)을
// 자리 10으로 나눈 값이 바닥이다. 12코어 병합기 기계에서도 같다(나누기 전 493·500초 → 나눈 뒤 512초, 2026-11-24 첫 착지 — 가장 긴
// 조각은 472→334초로 줄었으나 전체는 그대로). 더 줄이려면 시험 일 자체(합)를 줄이거나 자리 수를 늘려 잰다.
// 메모리 디스크·s21b1 줄인 뒤(2026-11-24 s21b1-trim, 워커 맥 10코어): 검증 324초(가장 긴 조각 s21b1 289초, 합 2181초) → 311·327초
// (s21b1 47~51초, 가장 긴 조각은 나머지 조각 159~179초, 합 2286~2467초) — 다시 합 ÷ 자리(약 230~250초)가 바닥이다.
// 시험의 기다림 걷은 뒤(2026-11-24 tests-wait-trim): 병합기 상주 시험(일꾼 재시작 쉼 30초 × 4 → PATH의 가짜 sleep)이 122→1초라
// 따로 떼지 않고 hc-merger 나머지 조각에 든다. est는 그 전 실측(워커 맥 10코어·메모리 디스크 3회 평균, hc-fill a,b,c는 gc 배경 32초를
// 뺀 값)으로 맞췄다 — 옛 est(s15s 165·Spec|Journal 155·hc-fill 묶음마다 150)가 실제(87·84·10~65)보다 커 긴 것부터의 차례가 틀렸다.
// hc-fill은 묶음마다 같은 est(part est ÷ 묶음 수)라 평균값이다. 실측(워커 맥 10코어, 다른 레인이 함께 돌아 전·후를 번갈아 3쌍):
// 조각 초 합 2680·2724·2347 → 2508·2481·2191초, 조각 단계 326·335·289 → 307·302·279초. 바닥은 가중 합(나머지 조각은 자리 3)
// ÷ 자리이고 차례 모의도 그와 맞는다 — 나머지 조각에서 가장 긴 cmd/hc-fill(약 70초)을 떼면 모의로 약 3초라 떼지 않았다.
// 전체 검증 시간은 바깥 부하에 TestCotenantGuard(부하를 재는 시험, 홀로)가 17~244초로 흔들리는 쪽이 더 크다.
// part가 없는 split은 그 꾸러미 통째 한 조각(나머지 조각에서 뺀다).
// 자리의 빈 몫을 잰 뒤(2026-11-24 verify-pack-measure, 워커 맥 10코어·메모리 디스크, 「끝」 줄의 +시작~+끝초와 「조각 단계 … 빈 자리·초」):
// 조각 단계 274~285초에 빈 자리·초 8~9%(210~252) — 끝 40초에 자리가 10→0으로 빠지는 꼬리였다. 나머지 조각(자리 3, -p 3)이 257~271초로
// 단계 전체를 묶었다: 그 안의 tests/repo/verify(150초)가 이름순으로 늦게 시작했다(꾸러미 합 530초 ÷ 3 = 177초). 그래서 tests/repo/verify를
// 통째 한 조각으로 떼고, est를 그 실측(3번 평균)으로 맞췄다. 그것만으로는 조각 단계 266초·빈 몫 9% 그대로였다 — 이번에는 hc-fill
// k,k2,q,r 묶음(52초)이 고른 est(합 ÷ 묶음 수 32초) 탓에 215초째에 떠 꼬리가 됐다. 그래서 묶음마다 실측 est(secEst).
// 뒤(5번): 조각 단계 251~257초, 빈 자리·초 101~116(4~5%), 검증 275~280초(한 번은 흔들림 다시 돌기로 306초) — 바닥(쥔 자리·초 ÷ 자리)
// 241~246초에 10초 안팎. 앞 단계(vet·tests/repo·TestCotenantGuard 홀로·메모리 디스크)는 24초 안팎이라 그대로 둔다.
// 절을 하위 시험으로 나눈 뒤(2026-11-24 contract-subtests, 워커 맥 10코어): 절마다 새 상태를 만들어도 TestHcFill 전체는 옛 판과 같았다
// (같은 때 함께 돌려 542 → 545초). 그래서 hc-fill은 가장 긴 k를 따로 떼 7묶음, hc-lane은 s21a4(약 75초)를 s21a1~3에서 뗐다. est는 그 검증
// 한 번의 실측 × 0.8(그 판은 바깥 부하로 hc-merger 나머지 조각이 175 → 360초라 다른 est와 눈금을 맞췄다) — hc-fill 묶음 실측 61·60·54·47·
// 42·37·29초·나머지 19초, s21a4 75·s21a1~3 52초. 검증 rc 0, 조각 단계 361초(빈 몫 8%).
var splits = []split{
	{mod + "tests/hclane", []part{
		{run: "^TestHcLane$/^s21b1$", est: 48},
		{run: "^TestHcLane$/^s21a4$", est: 60},
		{run: "^TestHcLane$/^(s21a1|s21a2|s21a3)$", est: 42},
		{run: "^TestHcLane$/^(s15s-16|s15-15r)$", est: 67},
		{run: "^TestHcLane$/^(s13-14|s19)$", est: 42},
		{run: "^TestHcLane$/^(s22-23|s08-11|s17-18|s24-keepoff)$", est: 41},
		{run: "^TestHcLane$", est: 71},
		{run: "^TestGofix", est: 77},
	}, 47},
	{mod + "tests/hcfill", []part{
		{run: "^TestHcFill$/^k$", est: 49},
		{run: "^TestHcFill$/^(e2|f|j)$", est: 48},
		{run: "^TestHcFill$/^(f2|k2|q|r)$", est: 43},
		{run: "^TestHcFill$/^(a|b|c)$", est: 38},
		{run: "^TestHcFill$/^(c2|d|d2)$", est: 34},
		{run: "^TestHcFill$/^(g|h)$", est: 30},
		{run: "^TestHcFill$/^(e|i)$", est: 23},
	}, 15},
	{mod + "tests/merger", []part{
		{run: "^TestMergerContractA$", est: 32},
		{run: "^TestMergerContractB$", est: 31},
		{run: "^TestMergerContractC$", est: 41},
		{run: "^TestMergerContractD$", est: 45},
		{run: "^TestMergerContractE$", est: 41},
		{run: "^TestR4PipeHeldExitZero$", est: 57},
		{run: "^TestRecovery[A-C]", est: 43},
		{run: "^TestRecovery[D-N]", est: 43},
		{run: "^TestRecovery[O-R]", est: 46},
	}, 24},
	{mod + "cmd/herdr-sync", []part{{run: "^TestContractA$", est: 38}, {run: "^TestContractB$", est: 56}, {run: "^TestContractC$", est: 35},
		{run: "^TestContractD$", est: 36}, {run: "^TestContractE$", est: 34}, {run: "^TestContractF$", est: 40}}, 26},
	{mod + "cmd/hc-merger", []part{{run: "^Test(Spec|Journal)", est: 82}}, 175},
	{mod + "tests/util", nil, 104},
	{mod + "cmd/herdr-tools-publish", nil, 52},
	{mod + "tests/repo/verify", nil, 150},
}

// restEst: 표에 없는 꾸러미를 모은 나머지 조각(-p n/3)의 실측 초
const restEst = 131

// shard: 함께 도는 go test 하나. w는 차지하는 자리(나머지 조각은 -p 수만큼 — 시험 바이너리를 그만큼 함께 띄운다)
type shard struct {
	argv []string
	env  []string
	est  int
	w    int
	// 범위(scope.go): skip이면 띄우지 않고 「건너뜀」 한 줄, note는 띄우는 줄에 덧붙임, done은 끝난 뒤(고아 검사까지 본 종료 코드·출력).
	// done이 참이면 그 실패는 부르는 쪽이 맡는다(retry.go — 다 끝난 뒤 다시 돌려 정한다) — runShards의 종료 코드에 안 센다
	skip bool
	note string
	done func(label string, rc, secs int, out string) bool
}

// vt0: 검증 시작 시각 — 단계·조각의 「끝」 줄에 그 시작·끝을 이 시각부터의 초로 붙인다(2026-11-24 verify-pack-measure: 앞 단계와
// 조각 단계의 초, 자리의 빈 몫을 로그로 재게). verify가 시작할 때 다시 정한다
var vt0 = time.Now()

// span: t부터 지금까지를 「 · +<시작>~+<끝>초」(vt0부터의 초)로
func span(t time.Time) string {
	return fmt.Sprintf(" · +%.1f~+%.1f초", t.Sub(vt0).Seconds(), time.Since(vt0).Seconds())
}

func main() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify:", err)
		os.Exit(1)
	}
	os.Exit(verify(root, os.Args[1:], stop, os.Stdout))
}

// plan: 싼 단계·홀로 도는 cotenant 단계의 go 인자, 조각의 바탕 인자
func plan(jobs string) (cheap [][]string, alone, base []string) {
	cheap = [][]string{{"go", "vet", "./..."}, {"go", "test", "-count=1", "./tests/repo/"}}
	base = []string{"go", "test", "-race", "-count=1", "-timeout", heavyTimeout}
	if jobs != "" {
		base = append(base, "-p", jobs)
	}
	alone = append(append([]string{}, base...), "-run", cotenant, "./claude/hooks/tests/")
	return cheap, alone, base
}

// shards: 꾸러미 목록(go list ./...)을 sp대로 조각으로. 길다고 잰 것부터. n은 함께 도는 자리 수 — 나머지 조각은 그 1/3을
// go test -p로 쓴다(2026-11-23 실측: 시험 바이너리가 코어 수보다 많이 함께 돌면 시험마다 1.5~2배 느려져 시간 기대 시험이 넘쳤다)
func shards(root string, pkgs, base []string, sp []split, n int) []shard {
	byPkg := map[string]split{}
	for _, s := range sp {
		byPkg[s.pkg] = s
	}
	var out []shard
	var rest []string
	for _, p := range pkgs {
		if _, ok := byPkg[p]; !ok {
			rest = append(rest, p)
		}
	}
	one := base[:len(base):len(base)]
	if i := indexOf(base, "-p"); i >= 0 { // 조각의 -p는 여기서 정한다
		one = append(append([]string{}, base[:i]...), base[i+2:]...)
	}
	if len(rest) > 0 {
		w := max(1, n/3)
		a := append(append([]string{}, one...), "-p", strconv.Itoa(w), "-skip", cotenant)
		out = append(out, shard{argv: append(a, rest...), est: restEst, w: w})
	}
	for _, p := range pkgs {
		s, ok := byPkg[p]
		if !ok {
			continue
		}
		var prev []string
		for _, pt := range s.parts {
			a := append(append([]string{}, one...), "-run", pt.run)
			if len(prev) > 0 {
				a = append(a, "-skip", strings.Join(prev, "|"))
			}
			out = append(out, shard{argv: append(a, p), est: pt.est})
			prev = append(prev, pt.run)
		}
		a := append([]string{}, one...)
		if len(prev) > 0 {
			a = append(a, "-skip", strings.Join(prev, "|"))
		}
		out = append(out, shard{argv: append(a, p), est: s.rest})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].est > out[j].est })
	return out
}

func indexOf(a []string, s string) int {
	for i, x := range a {
		if x == s {
			return i
		}
	}
	return -1
}

// verify: 종료 코드. 0 통과 · 1 실패 · 2 틀린 인자 · 128+신호 끊김
func verify(root string, args []string, stop <-chan os.Signal, w *os.File) int {
	cheapOnly := len(args) == 1 && args[0] == "--cheap-only"
	if len(args) == 1 && args[0] == "--flaky" {
		return flakyReport(w)
	}
	if len(args) > 0 && !cheapOnly {
		fmt.Fprintf(w, "verify: 모르는 인자 %q (쓰는 법: verify [--cheap-only | --flaky])\n", strings.Join(args, " "))
		return 2
	}
	jobs := os.Getenv("HERDR_VERIFY_JOBS")
	n, err := strconv.Atoi(jobs)
	if jobs != "" && (err != nil || n < 1) {
		fmt.Fprintf(w, "verify: HERDR_VERIFY_JOBS=%q 는 1 이상 정수가 아니라 무시한다\n", jobs)
		jobs = ""
	}
	if jobs == "" {
		n = runtime.NumCPU() // 2026-11-23 실측: 코어 수의 7/10보다 빨랐다(10코어 487 대 542·544초, 8코어 612 대 761초)
		// 자리를 코어 수의 2~3배로 늘려도 그대로다(디스크의 작은 쓰기에 묶임 — 2026-11-24 실측 546·530·527초). 그래서 메모리 디스크(ramtmp.go)
	}
	cheap, alone, base := plan(jobs)
	t0, fail := time.Now(), false
	vt0 = t0
	tree0 := ""
	if !cheapOnly {
		tree0 = cleanTree(root) // 통과 표식(mark.go)은 시작과 끝이 같은 깨끗한 트리일 때만
	}
	for _, a := range cheap {
		rc := runStep(root, a, stop, w)
		if rc >= 128 {
			return rc
		}
		fail = fail || rc != 0
	}
	if cheapOnly {
		return b2i(fail)
	}
	if fail && os.Getenv("HERDR_VERIFY_ALL") != "1" {
		fmt.Fprintln(w, "실패 싼 검사 — 무거운 시험을 건너뛰고 끝 (끝까지 돌리려면 HERDR_VERIFY_ALL=1)")
		return 1
	}
	var sh []shard
	if pkgs, err := goList(root); err != nil {
		fmt.Fprintf(w, "알림 go list 실패(%v) — 조각 없이 한 번에 돈다\n", err)
		sh = []shard{{argv: append(append(append([]string{}, base...), "-skip", cotenant), "./...")}}
	} else {
		sh = shards(root, pkgs, base, splits, n)
	}
	// 범위(scope.go): 0번은 홀로 도는 cotenant 단계, 그 뒤는 조각
	sc := scopePlan(root, append([]shard{{argv: alone}}, sh...), w)
	limit := retryLimit(w)
	var cs []retryCand // 다시 돌릴 후보(retry.go) — 홀로 단계(i -1)와 조각
	var common []string
	runAny := !sc.skip[0]
	for i := range sh {
		sh[i].skip, sh[i].note = sc.skip[i+1], sc.note(i+1)
		sh[i].done = func(label string, rc, secs int, out string) bool {
			sc.done(i+1, label, rc, secs, w) // 흔들려 통과해도 통과 기록은 없다(실패로 적힌다 — 다음에 다시 돈다)
			if rc == 0 || limit == 0 {
				return false
			}
			cs = append(cs, retryCand{i: i, label: label, argv: sh[i].argv, env: append(append([]string{}, sh[i].env...), common...), out: out})
			return true
		}
		runAny = runAny || !sh[i].skip
	}
	gate := os.Getenv("HERDR_VERIFY_GATE") == "1"
	if runAny && os.Getenv("HERDR_VERIFY_NO_LOCK") != "1" {
		tw := time.Now()
		release, rc := verifyLock(lockFor(), lockLimit, stop, w)
		if rc >= 128 {
			return rc
		}
		defer release()
		t0 = t0.Add(time.Since(tw)) // 검증 시간에서 잠금 기다림은 뺀다(기다림은 따로 한 줄)
		vt0 = t0
	}
	// 홀로 단계(cotenant — 1분 부하가 CPU 수 밑으로 내리길 기다리는 시험). 병합 검증 자리(gate)는 레인 검증과 함께 돌 수 있어 조각 뒤로
	// 미룬다 — 먼저 돌면 함께 도는 레인 검증의 부하로 calm이 오래 기다린다(그 단계만 차례로, 조각과는 여전히 따로)
	runAlone := func() int {
		aloneLabel := "[홀로] " + strings.Join(alone, " ")
		if sc.skip[0] {
			fmt.Fprintf(w, "건너뜀 %s — 입력이 통과 기록과 같음\n", aloneLabel)
			return 0
		}
		if s := sc.note(0); s != "" {
			fmt.Fprintf(w, "%s%s\n", aloneLabel, s)
		}
		ta := time.Now()
		fmt.Fprintf(w, "== %s\n", strings.Join(alone, " "))
		rc, out := runOne(root, alone, nil, stop, w, true)
		if rc >= 128 {
			return rc
		}
		fmt.Fprintf(w, "-- %s 끝 %d초 종료 %d%s\n", aloneLabel, int(time.Since(ta).Seconds()), rc, span(ta))
		w.WriteString(out) //nolint:errcheck // 화면·로그
		sc.done(0, aloneLabel, rc, int(time.Since(ta).Seconds()), w)
		if rc != 0 && limit > 0 {
			cs = append(cs, retryCand{i: -1, label: aloneLabel, argv: alone, out: out})
		} else {
			fail = fail || rc != 0
		}
		return 0
	}
	if !gate {
		if rc := runAlone(); rc >= 128 {
			return rc
		}
	}
	var ram *ramDisk
	if runAny {
		ram = ramSetup(w) // 조각들의 TMPDIR(ramtmp.go). 못 만들면 nil — 보통 임시 폴더로 그대로
	}
	defer ram.teardown(w)
	common = ram.env()
	// 조각의 임시 폴더는 이 검증만의 것(메모리 디스크 볼륨, 못 만들었으면 mktemp 폴더) — 끝에 그 아래 남은 프로세스를 거둔다(reapUnder)
	own := ram.root()
	if own == "" && runAny {
		if d, err := os.MkdirTemp("", "herdr-verify-tmp."); err != nil {
			fmt.Fprintf(w, "알림 검증 임시 폴더를 못 만들어 보통 임시 폴더로(그 아래 고아는 못 거둔다): %v\n", err)
		} else {
			own, common = d, []string{"TMPDIR=" + d + "/"}
			defer os.RemoveAll(d) //nolint:errcheck // 시험 임시 파일
		}
	}
	reaped := false
	reap := func() bool { // 한 번만 — 끝 줄 앞과 끊김 길(defer, 떼기·지우기보다 먼저 돈다)
		if reaped {
			return false
		}
		reaped = true
		return reapUnder(own, w) > 0
	}
	defer reap()
	rc := runShards(root, sh, common, n, stop, w)
	if rc >= 128 {
		return rc
	}
	fail = fail || rc != 0
	if gate {
		// 무거운 조각은 레인 검증과 함께 돌았다. 판정을 정하는 시간 시험(홀로 단계)과 혼자 다시 돌기는 레인 잠금을 기다려 쥐고 돈다
		// (감사 atp3qsv 중간 2 — 레인 검증의 조각과 겹친 채 흔들린 판정이 반송으로 이어지지 않게)
		if os.Getenv("HERDR_VERIFY_NO_LOCK") != "1" && (!sc.skip[0] || len(cs) > 0) {
			release, rc := verifyLock(lockPath(), lockLimit, stop, w)
			if rc >= 128 {
				return rc
			}
			defer release()
		}
		if rc := runAlone(); rc >= 128 {
			return rc
		}
	}
	if len(cs) > 0 && fail {
		fmt.Fprintln(w, "다시 안 돎: 다시 돌 수 없는 실패가 함께 있다 — 다시 돌려도 결과가 못 바뀐다")
	} else if len(cs) > 0 { // 다른 조각이 다 끝났다 — 실패한 것을 하나씩 혼자
		retryFail, _, rrc := retryPhase(root, cs, limit, func(c retryCand) (int, string) {
			t := time.Now()
			rc, out := runOne(root, c.argv, c.env, stop, w, true)
			if rc < 128 {
				fmt.Fprintf(w, "-- 다시 %s 끝 %d초 종료 %d\n%s", c.label, int(time.Since(t).Seconds()), rc, out)
			}
			return rc, out
		}, w)
		if rrc >= 128 {
			return rrc
		}
		fail = retryFail
	}
	fail = reap() || fail
	ram.teardown(w)
	sc.finish(w)
	flakySummary(w)
	fmt.Fprintf(w, "검증 시간 %d초 — %s\n", int(time.Since(t0).Seconds()), map[bool]string{false: "통과", true: "실패"}[fail])
	if !fail {
		markPassed(root, tree0, w)
	}
	return b2i(fail)
}

// lockBlocked: 시험 고리 — verifyLock이 남이 쥔 잠금에 막혀 기다리기 직전마다 부른다(시험이 「둘째가 막힌 뒤에만 놓기」로 순서를 묶게)
var lockBlocked = func() {}

// lockLimit: 검증 잠금을 기다리는 상한 — 넘기면 경고 한 줄 뒤 잠금 없이 진행(잠금이 검증을 영영 막지 않게)
const lockLimit = 30 * time.Minute

// lockPath: 기계(사용자) 단위 검증 잠금 파일 — 사용자 캐시 폴더(macOS ~/Library/Caches, 리눅스 ~/.cache), 없으면 임시 폴더
func lockPath() string {
	d, err := os.UserCacheDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "herdr-tools", "verify.lock")
}

// lockFor: 이 검증이 쥘 잠금 — HERDR_VERIFY_GATE=1(병합기가 병합 검증 자리를 받은 기계에 띄운 검증)이면 같은 폴더의 병합 검증 잠금
// verify-gate.lock(기계마다 병합 검증 하나), 아니면 레인 검증 잠금(docs/hc-lane/merger-spec-verify.md 「병합 검증 자리」)
func lockFor() string {
	if os.Getenv("HERDR_VERIFY_GATE") == "1" {
		return filepath.Join(filepath.Dir(lockPath()), "verify-gate.lock")
	}
	return lockPath()
}

// verifyLock: path에 flock(배타)을 얻는다. 다른 검증이 쥐고 있으면 1초마다 다시 보며 limit까지 기다리고, 기다렸으면 그 초를 한 줄로
// 찍는다. limit을 넘기거나 파일을 못 열면 경고 한 줄 뒤 잠금 없이 진행한다. 기다리다 신호를 받으면 128+신호.
// 돌려주는 함수가 잠금을 푼다(프로세스가 끝나도 풀린다). os.OpenFile은 O_CLOEXEC라 조각(go test)에 물려주지 않는다
func verifyLock(path string, limit time.Duration, stop <-chan os.Signal, w *os.File) (func(), int) {
	nop := func() {}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(w, "경고 검증 잠금 폴더를 못 만들어 잠금 없이 진행한다: %v\n", err)
		return nop, 0
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		fmt.Fprintf(w, "경고 검증 잠금 %s를 못 열어 잠금 없이 진행한다: %v\n", path, err)
		return nop, 0
	}
	t0 := time.Now()
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		waited := int(time.Since(t0).Seconds())
		switch {
		case err == nil:
			if waited > 0 {
				fmt.Fprintf(w, "검증 잠금: 같은 기계의 다른 검증을 %d초 기다렸다 (%s)\n", waited, path)
			}
			return func() { f.Close() }, 0
		case !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR):
			f.Close()
			fmt.Fprintf(w, "경고 검증 잠금 %s 실패(%v) — 잠금 없이 진행한다\n", path, err)
			return nop, 0
		case time.Since(t0) >= limit:
			f.Close()
			fmt.Fprintf(w, "경고 검증 잠금: 다른 검증을 %d초 기다려 상한을 넘겼다 — 잠금 없이 진행한다 (%s)\n", waited, path)
			return nop, 0
		}
		lockBlocked()
		select {
		case s := <-stop:
			f.Close()
			fmt.Fprintf(w, "끊김(%v) — 검증 잠금을 기다리다\n", s)
			return nop, sigRC(s)
		case <-time.After(time.Second):
		}
	}
}

// goList: 이 모듈의 꾸러미(가져오기 경로)
func goList(root string) ([]string, error) {
	c := exec.Command("go", "list", "./...")
	c.Dir = root
	out, err := c.Output()
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ctlVar: 이 검증 실행을 조종하는 변수(HERDR_VERIFY_*·HERDR_SPEC_*·HERDR_VERIFIED_TREE)인가 — 자식(시험)에게 물려주지 않는다(start).
// 시험이 검증 도구 자신을 시험하면 그 값을 보고 다른 동작을 한다(2026-11-26 ht-verify-env-scrub — 병합 검증 자리 기계의
// HERDR_VERIFY_GATE=1이 이 꾸러미 시험에 새 그 기계에서만 거짓 실패, 앞서 HERDR_VERIFY_PARTS는 픽스처 verify()가 이 실행의 조각 파일을
// 덮어쓸 뻔했다 — parts.go). 자식이 일부러 받는 것은 childKeep, 검증이 정해 주는 것(TMPDIR·HERDR_VERIFY_BOOT_TMPDIR)은 start의 env 인자
func ctlVar(k string) bool {
	return !childKeep[k] && (strings.HasPrefix(k, "HERDR_VERIFY_") || strings.HasPrefix(k, "HERDR_SPEC_") || k == "HERDR_VERIFIED_TREE")
}

// childKeep: 자식이 일부러 받는 조종 변수 — HERDR_VERIFY_NO_LINT는 tests/repo의 개인 값 검사가 읽는다(공개 사본에서 건너뛰기, secrets_test.go)
var childKeep = map[string]bool{"HERDR_VERIFY_NO_LINT": true}

// start: 새 무리로 띄운다(출력은 out 파일 — *os.File이라 그대로 물려준다: 복사 고루틴이 출력을 붙든 자손을 기다리지 않게)
func start(root string, argv, env []string, out *os.File) (*exec.Cmd, error) {
	c := exec.Command(argv[0], argv[1:]...)
	c.Dir, c.Stdout, c.Stderr = root, out, out
	c.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool { k, _, _ := strings.Cut(kv, "="); return ctlVar(k) })
	c.Env = append(c.Env, env...)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c, c.Start()
}

// exitCode: Wait 결과 → 종료 코드(신호로 죽었으면 128+신호)
func exitCode(err error, w *os.File, name string) int {
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ee):
		if st, ok := ee.Sys().(syscall.WaitStatus); ok && st.Signaled() {
			return 128 + int(st.Signal())
		}
		return ee.ExitCode()
	default:
		fmt.Fprintf(w, "실패 %s: %v\n", name, err)
		return 1
	}
}

// stopGroups: 무리들에 TERM, 10초 안에 다 안 끝나면 KILL. done은 무리마다 한 번 오는 끝남
func stopGroups(pgs []int, done <-chan struct{}) {
	for _, pg := range pgs {
		_ = syscall.Kill(-pg, syscall.SIGTERM) // 이미 끝난 무리면 ESRCH — 볼 것 없다
	}
	deadline := time.After(10 * time.Second)
	for left := len(pgs); left > 0; {
		select {
		case <-done:
			left--
		case <-deadline:
			for _, pg := range pgs {
				_ = syscall.Kill(-pg, syscall.SIGKILL)
			}
			for ; left > 0; left-- {
				<-done
			}
		}
	}
}

func sigRC(s os.Signal) int {
	if n, ok := s.(syscall.Signal); ok {
		return 128 + int(n)
	}
	return 130
}

// runStep: 한 명령을 새 무리로 돌리고 고아 검사. 종료 코드(고아가 있으면 0이라도 1, 신호로 끊기면 128+신호)
func runStep(root string, argv []string, stop <-chan os.Signal, w *os.File) int {
	fmt.Fprintf(w, "== %s\n", strings.Join(argv, " "))
	t := time.Now()
	rc, _ := runOne(root, argv, nil, stop, w, false)
	if rc < 128 {
		fmt.Fprintf(w, "-- %s 끝 %d초 종료 %d%s\n", strings.Join(argv, " "), int(time.Since(t).Seconds()), rc, span(t))
	}
	return rc
}

// runOne: runStep의 몸. capture면 출력을 임시 파일에 받아 돌려준다(w에는 안 쓴다 — 부르는 쪽이 옮긴다, 끊기면 여기서 옮긴다).
// 고아가 있으면 돌려주는 출력 끝에 「실패 고아를 남김」(다시 돌리기 판정 — retry.go)
func runOne(root string, argv, env []string, stop <-chan os.Signal, w *os.File, capture bool) (int, string) {
	before, err := procs()
	if err != nil {
		fmt.Fprintf(w, "실패 고아 검사 준비: %v\n", err)
		return 1, ""
	}
	out := w
	if capture {
		if out, err = os.CreateTemp("", "herdr-verify-step."); err != nil {
			fmt.Fprintf(w, "실패 단계 출력 파일: %v\n", err)
			return 1, ""
		}
		defer os.Remove(out.Name()) //nolint:errcheck // 내용은 읽어 돌려준다
	}
	text := func() string {
		if !capture {
			return ""
		}
		return dump(out, nil)
	}
	c, err := start(root, argv, env, out)
	if err != nil {
		text()
		fmt.Fprintf(w, "실패 %s: %v\n", argv[0], err)
		return 127, ""
	}
	pg := c.Process.Pid
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err = <-done:
	case s := <-stop:
		ended := make(chan struct{}, 1)
		go func() { <-done; ended <- struct{}{} }()
		stopGroups([]int{pg}, ended)
		w.WriteString(text()) //nolint:errcheck // 화면·로그
		fmt.Fprintf(w, "끊김(%v) — 도는 시험 무리를 껐다\n", s)
		return sigRC(s), ""
	}
	rc := exitCode(err, w, argv[0])
	s := text()
	rc, orph := checkOrphans(root, []int{pg}, before, rc, strings.Join(argv, " "), w)
	if orph {
		s += "실패 고아를 남김\n"
	}
	return rc, s
}

// checkOrphans: 고아를 찍고 끄고, 있으면 rc를 실패로. 둘째 값은 고아가 있었나(검사를 못 했어도 참 — 모름을 깨끗함으로 읽지 않는다)
func checkOrphans(root string, pgs []int, before map[int]pinfo, rc int, what string, w *os.File) (int, bool) {
	lines, err := orphans(root, pgs, before)
	if err != nil {
		fmt.Fprintf(w, "실패 고아 검사 못 함: %v\n", err)
		return max(rc, 1), true
	}
	if len(lines) > 0 {
		fmt.Fprintln(w, strings.Join(lines, "\n"))
		fmt.Fprintf(w, "실패 고아를 남김(위 목록, 껐음) — %s\n", what)
		if rc == 0 {
			rc = 1
		}
	}
	return rc, len(lines) > 0
}

// runShards: 조각들을 차례대로, 도는 조각의 자리(w, 없으면 1) 합이 n을 넘지 않게 함께 돌린다(아무것도 안 돌면 하나는 띄운다).
// 빈자리가 앞 조각에 모자라면 그 뒤에서 들어가는 조각을 먼저 띄운다.
// 조각마다 띄울 때 「== [i/n] …」, 끝나면 「-- [i/n] … 끝」과 그 출력. common은 모든 조각에 더하는 환경(메모리 디스크 TMPDIR 등)
// — 띄울 때만 넘기고 이름표에는 조각마다 다른 env만 찍는다(단계 앞 「메모리 디스크 …」 한 줄이 이미 알린다).
// 고아 검사: 조각마다 끝나자마자 그 무리, 다 끝난 뒤 떼어 띄운 것(부모 1) 한 번. 종료 코드: 0 통과 · 1 실패(조각 하나라도·고아) · 128+신호 끊김
func runShards(root string, sh []shard, common []string, n int, stop <-chan os.Signal, w *os.File) int {
	before, err := procs()
	if err != nil {
		fmt.Fprintf(w, "실패 고아 검사 준비: %v\n", err)
		return 1
	}
	d, err := os.MkdirTemp("", "herdr-verify-shard.")
	if err != nil {
		fmt.Fprintf(w, "실패 조각 출력 폴더: %v\n", err)
		return 1
	}
	defer os.RemoveAll(d) //nolint:errcheck // 조각 출력은 이미 w에 옮겼다
	type end struct {
		i   int
		err error
	}
	ends := make(chan end, len(sh))
	label := func(i int) string {
		return fmt.Sprintf("[%d/%d] %s", i+1, len(sh), strings.TrimSpace(strings.Join(sh[i].env, " ")+" "+strings.Join(sh[i].argv, " ")))
	}
	outs, t0s := make([]*os.File, len(sh)), make([]time.Time, len(sh))
	running := map[int]int{} // 조각 → 무리
	fail, used := false, 0
	seat := func(i int) int { return max(sh[i].w, 1) }
	// 자리·초: 쥔 자리(seat 합)를 시간으로 적분 — 끝에 「조각 단계 … 빈 자리·초」 한 줄(빈 몫 = 자리 n × 단계 초 − 쥔 자리·초)
	ts, tl, held := time.Now(), time.Now(), 0.0
	tick := func() { held += float64(used) * time.Since(tl).Seconds(); tl = time.Now() }
	wait := make([]int, len(sh)) // 아직 안 띄운 조각(차례대로)
	for i := range wait {
		wait[i] = i
	}
	for len(wait) > 0 || len(running) > 0 {
		// 들어가는 조각을 먼저: 앞 조각이 자리 여럿을 원해 못 들어가도 그 뒤의 들어가는 조각은 띄운다
		for k := 0; k < len(wait); {
			i := wait[k]
			if len(running) > 0 && used+seat(i) > n {
				k++
				continue
			}
			wait = append(wait[:k], wait[k+1:]...)
			if sh[i].skip {
				fmt.Fprintf(w, "건너뜀 %s — 입력이 통과 기록과 같음\n", label(i))
				continue
			}
			f, err := os.Create(filepath.Join(d, strconv.Itoa(i)))
			if err != nil {
				fmt.Fprintf(w, "실패 조각 출력 파일: %v\n", err)
				fail = true
				continue
			}
			fmt.Fprintf(w, "== %s%s\n", label(i), sh[i].note)
			c, err := start(root, sh[i].argv, append(sh[i].env[:len(sh[i].env):len(sh[i].env)], common...), f)
			if err != nil {
				fmt.Fprintf(w, "실패 %s: %v\n", label(i), err)
				f.Close()
				fail = true
				continue
			}
			outs[i], t0s[i], running[i] = f, time.Now(), c.Process.Pid
			tick()
			used += seat(i)
			go func() { ends <- end{i, c.Wait()} }()
		}
		if len(running) == 0 {
			break
		}
		select {
		case e := <-ends:
			pg := running[e.i]
			delete(running, e.i)
			tick()
			used -= seat(e.i)
			rc := exitCode(e.err, w, label(e.i))
			secs := int(time.Since(t0s[e.i]).Seconds())
			fmt.Fprintf(w, "-- %s 끝 %d초 종료 %d%s 자리 %d\n", label(e.i), secs, rc, span(t0s[e.i]), seat(e.i))
			out := dump(outs[e.i], w)
			// 그 무리는 끝나자마자 본다 — 다 끝난 뒤면 pid가 한 바퀴 돌아 남의 새 무리를 끌 수 있다(검증 한 번에 프로세스 수만 개)
			rc, orph := checkOrphans(root, []int{pg}, nil, rc, label(e.i), w)
			if orph {
				out += "실패 고아를 남김\n"
			}
			taken := false
			if sh[e.i].done != nil {
				taken = sh[e.i].done(label(e.i), rc, secs, out)
			}
			fail = fail || (rc != 0 && !taken)
		case s := <-stop:
			var live []int
			for _, pg := range running {
				live = append(live, pg)
			}
			ended := make(chan struct{}, len(live))
			go func() {
				for range live {
					e := <-ends
					dump(outs[e.i], w)
					ended <- struct{}{}
				}
			}()
			stopGroups(live, ended)
			fmt.Fprintf(w, "끊김(%v) — 도는 시험 무리를 껐다\n", s)
			return sigRC(s)
		}
	}
	if d := time.Since(ts).Seconds(); d > 0 {
		fmt.Fprintf(w, "조각 단계 %.1f초%s — 자리 %d, 쥔 자리·초 %.0f, 빈 자리·초 %.0f (%.0f%%)\n", d, span(ts), n, held, float64(n)*d-held, 100*(1-held/(float64(n)*d)))
	}
	rc, _ := checkOrphans(root, nil, before, b2i(fail), "무거운 시험 조각", w) // 떼어 띄워 부모가 1이 된 것은 다 끝난 뒤 한 번
	return rc
}

// dump: 출력 파일을 w로 옮기고(w가 nil이면 안 옮김) 닫는다. 그 내용
func dump(f *os.File, w *os.File) string {
	b, err := os.ReadFile(f.Name())
	if err == nil && w != nil {
		w.Write(b) //nolint:errcheck // 화면·로그
	}
	f.Close()
	return string(b)
}

type pinfo struct {
	pgid, ppid int
	cmd        string
}

// procs: 이 사용자의 프로세스(ps -x). ps가 실패하면 오류 — 모르면 「고아 없음」으로 읽지 않는다
func procs() (map[int]pinfo, error) {
	out, err := exec.Command("ps", "-x", "-o", "pid=,pgid=,ppid=,command=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	m := map[int]pinfo{}
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) < 3 {
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		pg, e2 := strconv.Atoi(f[1])
		pp, e3 := strconv.Atoi(f[2])
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		cmd := ""
		if len(f) > 3 {
			cmd = strings.Join(f[3:], " ")
		}
		m[pid] = pinfo{pg, pp, cmd}
	}
	return m, nil
}

// cwds: pid → cwd(lsof). 못 읽은 pid는 빠진다(그 프로세스는 명령 줄로만 판정)
func cwds(pids []int) map[int]string {
	m := map[int]string{}
	if len(pids) == 0 {
		return m
	}
	ps := make([]string, len(pids))
	for i, p := range pids {
		ps[i] = strconv.Itoa(p)
	}
	out, _ := exec.Command("lsof", "-a", "-d", "cwd", "-p", strings.Join(ps, ","), "-Fpn").Output() // 사라진 pid가 있으면 1 — 읽은 만큼 쓴다
	pid := 0
	for _, l := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(l, "p"):
			pid, _ = strconv.Atoi(l[1:])
		case strings.HasPrefix(l, "n") && pid > 0:
			m[pid] = l[1:]
		}
	}
	return m
}

// orphans: pgs 무리들에 남은 것과, before에 없고 부모가 1이며 이 폴더(root) 것인 프로세스와 그 자손을 찍고 끈다. 찍은 줄.
// before가 nil이면 무리만 본다(함께 도는 조각 하나가 끝났을 때 — 다른 조각이 떼어 띄운 것을 고아로 읽지 않게)
func orphans(root string, pgs []int, before map[int]pinfo) ([]string, error) {
	ps, err := procs()
	if err != nil {
		return nil, err
	}
	here := []string{root}
	if r, err := filepath.EvalSymlinks(root); err == nil && r != root {
		here = append(here, r)
	}
	mine := map[int]bool{}
	for _, pg := range pgs {
		mine[pg] = true
	}
	me := os.Getpid()
	var fresh []int
	for p, i := range ps {
		if _, old := before[p]; before != nil && !old && i.ppid == 1 {
			fresh = append(fresh, p)
		}
	}
	cwd := cwds(fresh)
	stray := func(p int) bool {
		c, cmd := cwd[p], ps[p].cmd
		for _, h := range here {
			if strings.Contains(cmd, h+"/") || (c != "" && strings.HasPrefix(c+"/", h+"/") && !strings.Contains(cmd, "/.claude/")) {
				return true
			}
		}
		return false
	}
	bad := map[int]bool{}
	for p, i := range ps {
		if p != me && mine[i.pgid] {
			bad[p] = true
		}
	}
	for _, p := range fresh {
		if p != me && stray(p) {
			bad[p] = true
		}
	}
	for grew := true; grew; { // 자손도 함께
		grew = false
		for p, i := range ps {
			if !bad[p] && p != me && bad[i.ppid] {
				bad[p], grew = true, true
			}
		}
	}
	if len(bad) == 0 {
		return nil, nil
	}
	var pids []int
	for p := range bad {
		pids = append(pids, p)
	}
	sort.Ints(pids)
	var need []int
	for _, p := range pids {
		if _, ok := cwd[p]; !ok {
			need = append(need, p)
		}
	}
	for p, c := range cwds(need) {
		cwd[p] = c
	}
	lines := make([]string, 0, len(pids))
	for _, p := range pids {
		c := cwd[p]
		if c == "" {
			c = "?"
		}
		cmd := []rune(ps[p].cmd)
		if len(cmd) > 120 {
			cmd = cmd[:120]
		}
		lines = append(lines, fmt.Sprintf("  고아 pid %d (cwd %s): %s", p, c, string(cmd)))
	}
	killPids(pids)
	return lines, nil
}

// killPids: TERM, 3초 안에 안 끝난 것은 KILL
func killPids(pids []int) {
	for _, p := range pids {
		_ = syscall.Kill(p, syscall.SIGTERM) // 이미 끝났으면 ESRCH
	}
	for i := 0; i < 30 && anyAlive(pids); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	for _, p := range pids {
		if syscall.Kill(p, 0) == nil {
			_ = syscall.Kill(p, syscall.SIGKILL)
		}
	}
}

// reapUnder: 이 검증만의 임시 폴더 dir(조각의 TMPDIR — 메모리 디스크 볼륨 또는 mktemp 폴더) 아래 경로가 명령 줄에 든 프로세스를
// 찍고 끈다(부모·무리와 상관없이 — 그 폴더는 이 검증 것이라 남의 것이 아니다). 시험 바이너리가 끊겨 죽으면(마감 panic·TERM) 새 무리로
// 띄운 자식이 부모 1로 남는데 그 경로는 저장소 밖이라 orphans가 못 본다 — 메모리 디스크를 그 위에서 도는 채로 떼면 실행 파일이 사라져
// 프로세스가 CPU를 태운다(2026-11-26 ht-test-orphan-leak: 워커 맥의 herdr-sync --check 30시간 275%). 그래서 떼기 전에, 끊길 때도.
// 찾은 수(검사를 못 하면 1 — 모름을 깨끗함으로 읽지 않는다)
func reapUnder(dir string, w *os.File) int {
	if dir == "" {
		return 0
	}
	ps, err := procs()
	if err != nil {
		fmt.Fprintf(w, "실패 검증 임시 폴더 고아 검사 못 함: %v\n", err)
		return 1
	}
	here := []string{strings.TrimSuffix(dir, "/") + "/"}
	if r, err := filepath.EvalSymlinks(dir); err == nil && r+"/" != here[0] {
		here = append(here, r+"/")
	}
	me := os.Getpid()
	var pids []int
	for p, i := range ps {
		for _, h := range here {
			if p != me && strings.Contains(i.cmd, h) {
				pids = append(pids, p)
				break
			}
		}
	}
	if len(pids) == 0 {
		return 0
	}
	sort.Ints(pids)
	for _, p := range pids {
		cmd := []rune(ps[p].cmd)
		if len(cmd) > 160 {
			cmd = cmd[:160]
		}
		fmt.Fprintf(w, "  고아 pid %d (검증 임시 폴더): %s\n", p, string(cmd))
	}
	killPids(pids)
	fmt.Fprintf(w, "실패 고아를 남김(위 목록, 껐음) — 검증 임시 폴더 %s\n", dir)
	return len(pids)
}

func anyAlive(pids []int) bool {
	for _, p := range pids {
		if syscall.Kill(p, 0) == nil {
			return true
		}
	}
	return false
}
