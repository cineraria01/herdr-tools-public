package hcfill_test

import (
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ht-relaunch-unify(2026-11-28) 성격 시험: 같은 줄을 다른 실행으로 다시 띄우는 길(주차 → 이어 띄움, 반송 → 다시 띄움, 채택)과
// 14칸(먼저 띄움) 일곱 값(-·no-early·early·stacked·parked·resumed·bounced)이 줄 값·터미널 닫기·띄우기 인자·화면 글로 어떻게 나오는지를
// 그대로 적어 testdata/relaunch.golden과 한 글자까지 견준다. 구조를 합치기 전에 찍었다 — 기대값을 고쳐 통과시키지 않는다
// (바꿔야 하면 지휘에 보고). 새로 찍기: 골든 파일을 비우고 돌리면 지금 값을 써 두고 실패한다(시험은 환경을 지워 손잡이를 못 받는다 — sandbox)
func init() { reg(106, "relaunch", secRelaunch) }

type rlT struct {
	k     *K
	b     strings.Builder
	rids  map[string]string
	lanes int // lane.log·hc.log에서 이미 적은 길이
	hcs   int
}

var rlTime = regexp.MustCompile(`\d{4}-\d\d-\d\d[T ]\d\d:\d\d(:\d\d(\.\d+)?Z?)?|\b\d\d:\d\d\b|\b\d{8}T?\d{6}\b|\b\d+m\b|\d+분`)

// rlCores: 기계 줄의 코어 수 — 부하 재기가 시험을 도는 기계의 진짜 코어 수를 읽는다(2026-11-28 ht-revive-cmd: 12코어에서 찍은 골든이 10코어 워커에서 깨졌다)
var rlCores = regexp.MustCompile(`/\d+\(코어당`)

// norm: 실행마다 바뀌는 값(임시 폴더·줄 ID·시각·코어 수)을 이름으로
func (s *rlT) norm(x string) string {
	x = strings.ReplaceAll(x, s.k.T, "$T")
	for _, r := range rows(s.k.Q) {
		if id := col(r, 17); id != "" && s.rids[id] == "" {
			s.rids[id] = "<줄" + strconv.Itoa(len(s.rids)+1) + ">"
		}
	}
	ids := make([]string, 0, len(s.rids))
	for id := range s.rids {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return len(ids[i]) > len(ids[j]) })
	for _, id := range ids {
		x = strings.ReplaceAll(x, id, s.rids[id])
	}
	return rlTime.ReplaceAllString(rlCores.ReplaceAllString(x, "/<코어>(코어당"), "<시각>")
}

// step: 한 걸음 — hc-fill <인자> 출력, 대기열 줄(이름·6·7·8·14칸), 시도 기록, 새로 쌓인 hc-lane 호출·hc 기록
func (s *rlT) step(title string, args ...string) {
	k := s.k
	k.run(args...)
	s.b.WriteString("## " + title + "\n$ hc-fill " + strings.Join(args, " ") + " (종료 " + strconv.Itoa(k.rc) + ")\n")
	for _, l := range strings.Split(s.norm(k.out), "\n") {
		if l != "" && !strings.HasPrefix(l, "바퀴 계측:") && !strings.HasPrefix(l, "gc: 배경으로 띄움") { // 걸린 시간·pid
			s.b.WriteString(l + "\n")
		}
	}
	s.state()
}

func (s *rlT) state() {
	k := s.k
	s.b.WriteString("-- 대기열\n")
	for _, r := range rows(k.Q) {
		s.b.WriteString(s.norm(strings.Join([]string{col(r, 1), col(r, 6), col(r, 7), col(r, 8), col(r, 14), col(r, 17)}, " ")) + "\n")
	}
	s.b.WriteString("-- 시도 기록\n" + s.norm(read(k.DATA+"/try.tsv")))
	ln, hc := read(k.T+"/lane.log"), read(k.T+"/hc.log")
	if len(ln) < s.lanes {
		s.lanes = 0
	}
	if len(hc) < s.hcs {
		s.hcs = 0
	}
	var hl []string
	for _, l := range strings.Split(hc[s.hcs:], "\n") {
		if f, _, _ := strings.Cut(l, " "); f == "kill" || f == "keep" || f == "read" { // exec(기계마다 나란히 도는 조회)는 차례가 흔들린다
			hl = append(hl, l)
		}
	}
	sort.Strings(hl)
	s.b.WriteString("-- 지시문 절\n")
	if es, err := os.ReadDir(k.DATA + "/runs"); err == nil {
		for _, e := range es {
			for _, l := range strings.Split(read(k.DATA+"/runs/"+e.Name()), "\n") {
				if strings.HasPrefix(l, "## ") || strings.HasPrefix(l, "0. ") || strings.HasPrefix(l, "1. git fetch") {
					s.b.WriteString(e.Name() + ": " + s.norm(l) + "\n")
				}
			}
		}
	}
	s.b.WriteString("-- hc-lane\n" + s.norm(strings.ReplaceAll(ln[s.lanes:], "\n", " ")) + "\n-- hc\n" + s.norm(strings.Join(hl, "\n")) + "\n")
	s.lanes, s.hcs = len(ln), len(hc)
}

func secRelaunch(k *K) {
	s := &rlT{k: k, rids: map[string]string{}}
	k.fresh()
	k.cpConf()
	k.confAppend("audit " + k.EXA)

	// 1. 먼저 띄움(early)·먼저 띄움 끔(no-early)·선행 없는 줄(-) — 띄운 값과 ls·status 글
	k.add("ea", "--after", "pz", "--only", "box-1")
	k.add("ne", "--after", "pz", "--no-early", "--only", "box-1")
	k.add("pl", "--only", "box-1")
	s.step("먼저 띄움·끔·선행 없음 첫 바퀴", "run")
	s.step("ls", "ls")
	s.step("status", "status")
	s.step("두 번째 바퀴(같은 상태)", "run")

	// 2. 채택: 대기 줄과 같은 이름의 hc-lane 레인 터미널이 이미 살아 있음
	k.add("ad", "--only", "box-2")
	k.lj("box-1/ea:claude-code:ea:working:/home/me/src/app.wt/ea", "box-1/pl:claude-code:pl:working:/home/me/src/app.wt/pl",
		"box-2/ad:claude-code:ad:working:/home/me/src/app.wt/ad")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/ea","state":"live","cwd":"/"},{"address":"box-1/pl","state":"live","cwd":"/"},{"address":"box-2/ad","state":"live","cwd":"/"}]}`)
	s.step("채택", "run")

	// 3. 주차 → 이어 띄움
	write(k.t, k.T+"/events", "2026-11-20T01:00:00Z\tm\tbox-1/ea\tea\t주차\tabcdef12\tlane/ea 준비\t1\n")
	s.step("주차 사건(터미널 working)", "run")
	s.step("주차 줄 dry-run", "run", "--dry-run")
	write(k.t, k.T+"/commits", "aaaa1111 pz: 끝\n")
	s.step("선행 착지 — 옛 주차 터미널 working이라 미룸", "run")
	k.lj("box-1/pl:claude-code:pl:working:/home/me/src/app.wt/pl", "box-2/ad:claude-code:ad:working:/home/me/src/app.wt/ad")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/pl","state":"live","cwd":"/"},{"address":"box-2/ad","state":"live","cwd":"/"}]}`)
	s.step("옛 주차 터미널 없어짐 — 이어 띄움", "run")
	s.step("ls 이어 띄운 뒤", "ls")
	s.step("두 번째 바퀴(이어 띄운 뒤)", "run")

	// 4. 반송 → 다시 띄움(반송 절 없음 → 옛 실행 남음 → 없어짐)
	k.add("bq", "--only", "box-2")
	k.hSetQ("bq", map[int]string{14: "bounced"})
	s.step("반송됨 — 절 없음", "run")
	if err := os.MkdirAll(k.DATA+"/bounce", 0o755); err != nil {
		k.t.Fatal(err)
	}
	write(k.t, k.DATA+"/bounce/bq.md", "## 반송 — 병합기가 이 레인의 제출을 돌려보냈다 (hc-fill, 시험)\n1. git fetch origin refs/merge-failed/900-bq 뒤 git reset --keep FETCH_HEAD\n")
	k.lj("box-1/pl:claude-code:pl:working:/home/me/src/app.wt/pl", "box-2/ad:claude-code:ad:working:/home/me/src/app.wt/ad",
		"box-2/t-old:claude-code:bq:idle:/home/me/src/app.wt/bq")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/pl","state":"live","cwd":"/"},{"address":"box-2/ad","state":"live","cwd":"/"},{"address":"box-2/t-old","state":"live","cwd":"/"}]}`)
	s.step("반송됨 — 옛 실행 남음", "run")
	s.step("ls 반송됨", "ls")
	k.set("HC_FAKE_GONE=box-2/t-old")
	s.step("반송됨 — 옛 실행 없어짐", "run")
	k.unset("HC_FAKE_GONE")
	s.step("두 번째 바퀴(반송 다시 띄운 뒤)", "run")

	// 5. 이미 쌓인 상태: 일곱 값이 섞인 옛 꼴 대기열(16칸 줄·17칸 줄) — 첫 바퀴 dry-run·ls·status가 줄을 바꾸지 않는다
	k.fresh()
	k.cpConf()
	q := strings.Join([]string{
		"o1\tapp\t" + k.brief("o1") + "\t-\t-\twait\t-\t-\t-\tt\t-\t-\tnormal\t-\t-\t-",
		"o2\tapp\t" + k.brief("o2") + "\tpz\tbox-1\twait\t-\t-\t-\tt\t-\t-\tnormal\tno-early\t-\t-\tr2",
		"o3\tapp\t" + k.brief("o3") + "\tpz\tbox-1\tup\tbox-1\tbox-1/o3\t-\tt\t-\t-\tnormal\tearly\t-\t-\tr3",
		"o4\tapp\t" + k.brief("o4") + "\tpz\tbox-1\twait\t-\t-\t-\tt\t-\t-\tnormal\tparked\t-\t-\tr4",
		"o5\tapp\t" + k.brief("o5") + "\tpz\tbox-1\tup\tbox-1\tbox-1/o5\t-\tt\t-\t-\tnormal\tresumed\t-\t-\tr5",
		"o6\tapp\t" + k.brief("o6") + "\t-\tbox-2\twait\t-\t-\t-\tt\t-\t-\tnormal\tbounced\t-\t-\tr6",
		"o7\tapp\t" + k.brief("o7") + "\tpz\tbox-1\tup\tbox-1\tbox-1/o7\t-\tt\t-\t-\tnormal\tstacked\t-\t-\tr7",
		"o8\tapp\t" + k.brief("o8") + "\t-\tbox-2\tup\tbox-2\tbox-2/o8\t-\tt\t-\t-\tnormal\tparked\t-\t-\tr8",
	}, "\n") + "\n"
	mkdirs(k.t, k.DATA)
	write(k.t, k.Q, q)
	k.lj("box-1/o3:claude-code:o3:working:/home/me/src/app.wt/o3", "box-1/o5:claude-code:o5:working:/home/me/src/app.wt/o5",
		"box-1/o7:claude-code:o7:working:/home/me/src/app.wt/o7", "box-2/o8:claude-code:o8:working:/home/me/src/app.wt/o8")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/o3","state":"live","cwd":"/"},{"address":"box-1/o5","state":"live","cwd":"/"},{"address":"box-1/o7","state":"live","cwd":"/"},{"address":"box-2/o8","state":"live","cwd":"/"}]}`)
	s.lanes, s.hcs = 0, 0
	s.step("옛 꼴 섞인 대기열 — ls", "ls")
	s.step("옛 꼴 섞인 대기열 — status", "status")
	s.step("옛 꼴 섞인 대기열 — run --dry-run", "run", "--dry-run")
	k.check("dry-run은 섞인 대기열을 한 바이트도 바꾸지 않음", read(k.Q) == q, read(k.Q))
	s.step("옛 꼴 섞인 대기열 — run", "run")

	// 6. 바퀴 순서 반대·외부 가정: 주차 사건이 줄을 띄우기 전에 옴 · 터미널 목록을 못 읽음 · hc get 낱말 finished·not_found
	k.fresh()
	k.cpConf()
	s.lanes, s.hcs = 0, 0
	k.add("rv", "--after", "pz", "--only", "box-1")
	write(k.t, k.T+"/events", "2026-11-20T01:00:00Z\tm\tbox-1/rv\trv\t주차\tabcdef12\tlane/rv 준비\t1\n")
	s.step("주차 사건이 먼저(줄은 아직 대기)", "run")
	k.hSetQ("rv", map[int]string{6: "wait", 7: "-", 8: "-", 14: "parked"})
	write(k.t, k.T+"/commits", "aaaa1111 pz: 끝\n")
	k.set("HC_FAKE_LS_FAIL=1")
	s.step("주차 줄 — 터미널 목록 못 읽음", "run")
	k.unset("HC_FAKE_LS_FAIL")
	k.lj("box-1/old/rv:claude-code:rv:idle:/home/me/src/app.wt/rv")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/old/rv","state":"live","cwd":"/"}]}`, "HC_FAKE_FINISHED=box-1/old/rv")
	s.step("주차 줄 — 같은 이름 옛 터미널 hc get finished", "run")
	k.unset("HC_FAKE_FINISHED", "HC_FAKE_LS_LANE")
	k.lj()
	k.add("bx", "--only", "box-2")
	k.hSetQ("bx", map[int]string{14: "bounced"})
	write(k.t, k.DATA+"/bounce/bx.md", "## 반송 — 병합기가 이 레인의 제출을 돌려보냈다 (hc-fill, 시험)\n1. git fetch origin refs/merge-failed/901-bx 뒤 git reset --keep FETCH_HEAD\n")
	k.lj("box-2/old/bx:claude-code:bx:idle:/home/me/src/app.wt/bx")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-2/old/bx","state":"live","cwd":"/"}]}`, "HC_FAKE_GONE=box-2/old/bx")
	s.step("반송 줄 — 같은 이름 옛 터미널 hc get not_found", "run")
	k.unset("HC_FAKE_GONE", "HC_FAKE_LS_LANE")
	k.lj()

	got := s.b.String()
	gp := repo + "/tests/hcfill/testdata/relaunch.golden"
	want := read(gp)
	if want == "" {
		write(k.t, gp, got)
		k.check("성격 시험: 골든이 비어 지금 값을 써 둠 — 확인 뒤 다시 돌린다", false, gp)
		return
	}
	if got != want {
		gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
		i := 0
		for i < len(gl) && i < len(wl) && gl[i] == wl[i] {
			i++
		}
		lo := max(0, i-3)
		k.check("성격 시험: 다시 띄우기 길의 출력·줄 값이 골든과 같음", false,
			"첫 다른 줄 "+strconv.Itoa(i+1)+"\n--- 골든\n"+strings.Join(wl[lo:min(len(wl), i+4)], "\n")+"\n--- 지금\n"+strings.Join(gl[lo:min(len(gl), i+4)], "\n"))
		return
	}
	k.check("성격 시험: 다시 띄우기 길의 출력·줄 값이 골든과 같음", true)
}
