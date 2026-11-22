package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSelfTest(t *testing.T) {
	if err := selfTest(); err != nil {
		t.Fatal(err)
	}
}

// landedClean: git 결과를 차례로 돌려주는 가짜로 — 착지·더러움·증거 없음·모름
func TestLandedClean(t *testing.T) {
	dir := t.TempDir()
	ev := filepath.Join(dir, "events.tsv")
	// 옛 8칸 줄: 같은 주소 t1, 이 터미널(created_ms 2026-01-01) 뒤 시각이라 이 실행의 증거
	os.WriteFile(ev, []byte("2026-11-01T00:00:00Z\tm\tt1\tland-x\t제출\tabc\t-\t3\n"), 0o644)
	created := float64(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli())
	seq := func(outs ...result) runner {
		i := 0
		return func(_ time.Duration, _ []string, _ string, args ...string) result {
			if i >= len(outs) {
				t.Fatalf("더 부름: %v", args)
			}
			i++
			return outs[i-1]
		}
	}
	ok := func(s string) result { return result{out: s} }
	term := map[string]any{"cwd": "/r.wt/land-x", "name": "land-x", "address": "t1", "created_ms": created}
	// 차례: status · symbolic-ref · rev-parse · cherry · rev-list --merges(R-201) · [ls-remote · 표식 is-ancestor · HEAD is-ancestor · cherry]
	repo, br, why := landedClean(term, seq(ok(""), ok("land-x\n"), ok("/r/.git\n"), ok("- abc\n"), ok("")), ev)
	if repo != "/r" || br != "land-x" || why != "" {
		t.Fatalf("착지: %q %q %q", repo, br, why)
	}
	// [R-201] main에 없는 병합이 HEAD에 있으면 사건 「제출」은 증거가 아니다(원격 표식도 없음)
	mc := ok(strings.Repeat("a", 40) + "\n") // 병합 커밋 이름(실행 중에 만듦 — 비밀 꼴 검사)
	if _, _, why = landedClean(term, seq(ok(""), ok("land-x\n"), ok("/r/.git\n"), ok(""), mc, ok("")), ev); !strings.HasPrefix(why, "착지 증거 없음") {
		t.Fatal("재작성된 병합을 사건으로 착지라 봄: " + why)
	}
	// [R-201] 병합 조회 출력이 커밋 이름 꼴이 아니면 모름(남김)
	if _, _, why = landedClean(term, seq(ok(""), ok("land-x\n"), ok("/r/.git\n"), ok(""), ok("abc\n")), ev); !strings.HasPrefix(why, "모름") {
		t.Fatal("깨진 rev-list 출력을 병합 없음으로 봄: " + why)
	}
	if _, _, why = landedClean(term, seq(ok("?? x\n")), ev); why != "커밋 안 된 변경" {
		t.Fatal(why)
	}
	if _, _, why = landedClean(term, seq(ok(""), ok("land-x\n"), ok("/r/.git\n"), ok("+ abc\n")), ev); !strings.HasPrefix(why, "착지 전") {
		t.Fatal(why)
	}
	other := map[string]any{"cwd": "/r.wt/land-n", "name": "land-n", "address": "t2", "created_ms": created}
	if _, _, why = landedClean(other, seq(ok(""), ok("land-n\n"), ok("/r/.git\n"), ok(""), ok(""), ok("")), ev); !strings.HasPrefix(why, "착지 증거 없음") {
		t.Fatal(why)
	}
	after := strconv.FormatInt(int64(created/1000)+60, 10)
	mk := ok("abc\trefs/hc-landed/land-n/" + after + "\n")
	if repo, _, _ = landedClean(other, seq(ok(""), ok("land-n\n"), ok("/r/.git\n"), ok(""), ok(""), mk, ok(""), result{rc: 1}, ok("")), ev); repo != "/r" {
		t.Fatal("이 터미널 뒤 원격 표식이면 착지(rebase됨 — 패치로)")
	}
	if repo, _, _ = landedClean(other, seq(ok(""), ok("land-n\n"), ok("/r/.git\n"), ok(""), ok(""), mk, ok(""), ok("")), ev); repo != "/r" {
		t.Fatal("HEAD가 표식 커밋의 조상이면 착지")
	}
	// [R-201] 병합이 있고 HEAD가 표식 커밋의 조상이 아니면(재작성) 패치 대조로 착지라 하지 않는다
	if _, _, why = landedClean(other, seq(ok(""), ok("land-n\n"), ok("/r/.git\n"), ok(""), mc, mk, ok(""), result{rc: 1}), ev); !strings.HasPrefix(why, "착지 증거 없음") {
		t.Fatal("재작성된 병합을 표식으로 착지라 봄: " + why)
	}
	// [B03] 이 터미널보다 먼저 생긴 표식(같은 이름의 옛 레인)·다른 주소의 옛 사건은 증거 아님
	if _, _, why = landedClean(other, seq(ok(""), ok("land-n\n"), ok("/r/.git\n"), ok(""), ok(""), ok("abc\trefs/hc-landed/land-n/1\n")), ev); !strings.HasPrefix(why, "착지 증거 없음") {
		t.Fatal("옛 표식을 증거로 봄: " + why)
	}
	moved := map[string]any{"cwd": "/r.wt/land-x", "name": "land-x", "address": "t9", "created_ms": created}
	if _, _, why = landedClean(moved, seq(ok(""), ok("land-x\n"), ok("/r/.git\n"), ok(""), ok(""), ok("")), ev); !strings.HasPrefix(why, "착지 증거 없음") {
		t.Fatal("다른 주소의 옛 사건을 증거로 봄: " + why)
	}
	if _, _, why = landedClean(term, seq(ok(""), result{rc: 1}), ev); !strings.HasPrefix(why, "모름") {
		t.Fatal(why)
	}
}

// run: 상한 시간이면 그 자식을 끄고 모름(err)으로 곧 돌아온다. 자식은 같은 무리라(B19 — 셸 판과 같게) 쓸기 무리를 끄면 함께 끝난다
// (무리 확인은 sys_unix_test.go). 손자는 거두지 않는다 — 부르는 명령은 손자를 두지 않는다(run 주석)
func TestRunTimeout(t *testing.T) {
	pidf := filepath.Join(t.TempDir(), "pid")
	t0 := time.Now()
	r := run(300*time.Millisecond, nil, "sh", "-c", "echo $$ >"+pidf+"; exec sleep 30")
	if r.err == nil || r.rc != -1 {
		t.Fatalf("시간 초과가 아님: %+v", r)
	}
	if d := time.Since(t0); d > 5*time.Second {
		t.Fatalf("늦게 돌아옴 %v", d)
	}
	b, _ := os.ReadFile(pidf)
	var n int
	for _, c := range strings.TrimSpace(string(b)) {
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		t.Fatal("자식 pid 없음")
	}
	if alive(n) {
		t.Fatalf("자식 %d가 남음", n)
	}
}
