package merger_test

// 바퀴 일꾼이 뜻밖에 끝나도 병합기가 멈추지 않는다(2026-11-22 merger-worker-retry): 옛 바이너리 자리를 지우자 상주 병합기가 시작 때 잡은
// 경로의 hc-merger를 exec하려다 127 → 「바퀴 일꾼이 뜻밖에 끝났다」로 섰다. 바퀴마다 일꾼 경로를 새로 찾고(자기 곁 → PATH), 못 뜨거나
// 신호로 죽으면 「일꾼 다시」 사건 + 30초 쉼 뒤 다시, 같은 실패가 5번 이어질 때만 막힘·멈춤. herdr-sync의 원자 교체(rename)는 도는 바퀴를
// 건드리지 않고 다음 바퀴가 새 판을 쓴다.
//
// 상주 병합기를 옮겨 둔 폴더(inst)의 hc-lane으로 띄우고, PATH의 가짜 sleep이 바퀴 사이(쉼)마다 불려 inst의 hc-merger를 치우거나 되돌리며
// 정해 둔 횟수에서 병합기(sleep의 부모)에 TERM을 보낸다.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// wr: 시험 하나의 자리 — inst(옮긴 hc-lane·hc-merger)·stub(가짜 sleep 등)·sleep 기록
type wr struct {
	*F
	inst, stub, slog string
}

// newWR: inst에 hc-lane·hc-merger를 복사하고, PATH에는 hc-merger가 없게(stub → T/bin → herdr-tools-config만 둔 폴더 → hc-merger
// 없는 바깥 PATH). withPath면 PATH에도 hc-merger(기록을 남기고 지은 것을 exec하는 대역)를 둔다
func newWR(t *testing.T, name string, withPath bool) *wr {
	k := newRK(t)
	f := k.fixture(name)
	w := &wr{F: f, inst: f.home + "/inst", stub: f.home + "/stub", slog: f.home + "/sleep.log"}
	mkdirs(t, w.inst, w.stub, f.home+"/tools")
	f.sh(f.home, `cp "$1" "$3/hc-lane" && cp "$2" "$3/hc-merger" && ln -s "$(dirname "$1")/herdr-tools-config" "$4/herdr-tools-config"`,
		k.LANE, k.MERGER, w.inst, f.home+"/tools")
	var keep []string
	for _, d := range filepath.SplitList(k.env["PATH"]) {
		if d != "" && !exists(filepath.Join(d, "hc-merger")) && d != filepath.Dir(k.LANE) {
			keep = append(keep, d)
		}
	}
	p := w.stub + ":" + k.T + "/bin:" + f.home + "/tools"
	if withPath {
		mkdirs(t, f.home+"/pathbin")
		f.stub(f.home+"/pathbin/hc-merger", "#!/bin/sh\necho path >>\""+f.home+"/pathbin.log\"\nexec '"+k.MERGER+"' \"$@\"\n")
		p += ":" + f.home + "/pathbin"
	}
	k.set("PATH=" + p + ":" + strings.Join(keep, ":"))
	return w
}

// sleepStub: n번째 쉼마다 할 셸 한 줄(acts[n-1], 1부터). 쉼마다 「n 초」를 기록하고, acts보다 많이 불리면(25번에서) 병합기에 TERM
func (w *wr) sleepStub(acts ...string) {
	body := "#!/bin/sh\nn=$(($(wc -l <\"" + w.slog + "\" 2>/dev/null || echo 0) + 1))\necho \"$n $1\" >>\"" + w.slog + "\"\ncase $n in\n"
	for i, a := range acts {
		body += "  " + strconv.Itoa(i+1) + ") " + a + " ;;\n"
	}
	body += "esac\n[ \"$n\" -lt 25 ] || kill -TERM $PPID\nexit 0\n"
	w.stub2("sleep", body)
}

func (w *wr) stub2(name, body string) { w.t.Helper(); w.F.stub(w.stub+"/"+name, body) }

// run: inst의 hc-lane으로 상주 병합기(--tick 7)를 띄워 끝날 때까지
func (w *wr) run() R {
	return w.exe(w.writer, nil, 2*time.Minute, w.inst+"/hc-lane", "merger", "--repo", w.mclone, "--tick", "7", "main")
}

func (w *wr) sleeps() []string { return lines(read(w.slog)) }

func (w *wr) evText(state string) []string {
	return evs(w.home+"/.local/state/hc-lane/events.tsv", "merger", state)
}

const term = "kill -TERM $PPID"

// 일꾼이 못 뜨면(127) 멈추지 않고 「일꾼 다시」 + 30초 쉼 뒤 다시 — 되살아나면 그대로 이어 간다
func TestWorkerRetryExecFailure(t *testing.T) {
	w := newWR(t, "wr-exec", false)
	w.sleepStub(`mv "$HOME/inst/hc-merger" "$HOME/inst/hc-merger.gone"`, `mv "$HOME/inst/hc-merger.gone" "$HOME/inst/hc-merger"`, term)
	r := w.run()
	d := "rc=" + strconv.Itoa(r.RC) + "\n" + r.Err + "\nsleep:\n" + read(w.slog)
	w.check("일꾼이 못 떠도 병합기는 멈추지 않고 TERM으로만 끝났다(143)", r.RC == 143, d)
	w.check("막힘 사건 없음", len(w.evText("막힘")) == 0, strings.Join(w.evText("막힘"), "\n"))
	mid := w.evText("중간")
	w.check("「일꾼 다시」 중간 사건 하나", len(mid) == 1 && anyHas(mid, "일꾼 다시"), strings.Join(mid, "\n"), d)
	s := w.sleeps()
	w.check("쉼 순서: 빈 바퀴 7 → 일꾼 다시 30 → 빈 바퀴 7", len(s) == 3 && s[0] == "1 7" && s[1] == "2 30" && s[2] == "3 7", d)
	w.check("경고 한 줄(종료 127)", strings.Contains(r.Err, "127") && strings.Contains(r.Err, "일꾼 다시"), d)
}

// 같은 실패가 5번 이어지면 그때 막힘 사건 + 멈춤(일꾼 다시 사건은 4번)
func TestWorkerRetryGivesUp(t *testing.T) {
	w := newWR(t, "wr-giveup", false)
	w.sleepStub(`mv "$HOME/inst/hc-merger" "$HOME/inst/hc-merger.gone"`)
	r := w.run()
	d := r.Err + "\nsleep:\n" + read(w.slog)
	w.check("다섯 번째 실패에 멈춤(종료 1)", r.RC == 1, d)
	b := w.evText("막힘")
	w.check("막힘 사건 하나(병합기 멈춤)", len(b) == 1 && anyHas(b, "병합기 멈춤"), strings.Join(b, "\n"), d)
	w.check("「일꾼 다시」 사건 넷", countHas(w.evText("중간"), "일꾼 다시") == 4, strings.Join(w.evText("중간"), "\n"))
	w.check("쉼: 빈 바퀴 하나 + 30초 넷", len(w.sleeps()) == 5 && countHas(w.sleeps(), " 30") == 4, d)
}

// 시작 때 잡은 경로가 사라져도 바퀴마다 다시 찾아 PATH의 hc-merger로 이어 간다(실패도 사건도 없이)
func TestWorkerRetryPathFallback(t *testing.T) {
	w := newWR(t, "wr-path", true)
	w.sleepStub(`rm "$HOME/inst/hc-merger"`, term)
	r := w.run()
	d := r.Err + "\nsleep:\n" + read(w.slog)
	w.check("TERM으로만 끝났다(143)", r.RC == 143, d)
	w.check("둘째 바퀴는 PATH의 hc-merger", strings.Count(read(w.home+"/pathbin.log"), "path") == 1, d)
	w.check("사건 없음", len(w.evText("막힘"))+len(w.evText("중간")) == 0, d)
	w.check("쉼 둘 다 빈 바퀴(7)", strings.Join(w.sleeps(), ",") == "1 7,2 7", d)
}

// 바퀴 도중 hc-merger를 rename으로 바꿔도 그 바퀴는 끝나고(빈 바퀴 쉼), 다음 바퀴는 새 판이다
func TestWorkerRetryAtomicSwap(t *testing.T) {
	w := newWR(t, "wr-swap", false)
	w.F.stub(w.inst+"/hc-merger.new", "#!/bin/sh\necho new >>\""+w.home+"/new.log\"\nexec '"+w.MERGER+"' \"$@\"\n")
	// 바퀴 안의 첫 ls-remote(큐 읽기) 자리에서 바꿔 넣는다 — 상주 시작은 ls-remote를 부르지 않는다(기본 브랜치를 줬다)
	w.stub2("git", "#!/bin/sh\nif [ \"$1\" = ls-remote ] && [ ! -e \"$HOME/swapped\" ]; then mv \"$HOME/inst/hc-merger.new\" \"$HOME/inst/hc-merger\" && : >\"$HOME/swapped\"; fi\nexec \"$real_git\" \"$@\"\n")
	w.sleepStub(":", term)
	r := w.run()
	d := r.Err + "\nsleep:\n" + read(w.slog)
	w.check("바퀴 중에 바꿔 넣었다", exists(w.home+"/swapped"), d)
	w.check("TERM으로만 끝났다(143)", r.RC == 143, d)
	w.check("사건 없음", len(w.evText("막힘"))+len(w.evText("중간")) == 0, d)
	w.check("첫 바퀴는 끝까지(빈 바퀴 쉼 7), 둘째 바퀴도", strings.Join(w.sleeps(), ",") == "1 7,2 7", d)
	w.check("둘째 바퀴만 새 판", strings.Count(read(w.home+"/new.log"), "new") == 1, d)
	if _, err := os.Stat(w.inst + "/hc-merger.new"); err == nil {
		w.t.Errorf("hc-merger.new가 남았다")
	}
}
