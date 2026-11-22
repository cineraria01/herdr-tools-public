package hclane_test

// gofix3-hcjson(2026-11-22 Go 전환 재감사 2바퀴 — 아스트라 6.2 max, 감사 HEAD 1eccd73): hc 응답 판독을 internal/hcjson으로 모은 뒤
// 지적 ID마다 고치기 전 판에서 실패하던 시험. 기대값은 지적 원문과 hcjson 계약으로 정했다.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGofix3R2(t *testing.T) {
	withK(t, func(k *K) {
		for _, c := range []struct {
			name string
			f    func(*K)
		}{
			{"R202", g3LiveCwd}, {"R208", g3CoordIdentity}, {"R221", g3CoordMode}, {"R209", g3SendOwner}, {"R210", g3SendName}, {"R211", g3StartPartial},
		} {
			k.t.Run(c.name, func(t *testing.T) {
				pt := k.t
				k.t = t
				defer func() { k.t = pt }()
				c.f(k)
			})
		}
	})
}

// R-202: 살아 있는 터미널의 cwd가 빈 글·「.」·상대 경로면 그 터미널의 작업 폴더를 모른다 — gc·clean은 작업 트리를 지우지 않는다
func g3LiveCwd(k *K) {
	k.mk("g3202")
	k.land("g3202l")
	os.Remove(k.wt + "/.omc/state/s.json")
	k.runE([]string{`HC_FAKE_LS={"terminals":[]}`}, "gc", "--repo", k.R, "--dry-run", "--min-age", "0")
	k.check("R-202 준비: 올바른 빈 목록이면 지울 후보", k.rc == 0 && s19lineHas(k, k.wt, "지움"))
	for _, c := range []string{`""`, `"."`, `"rel/x"`} {
		ls := `{"terminals":[{"address":"t5","agent":"claude-code","cwd":` + c + `,"state":"live"}]}`
		k.runE([]string{"HC_FAKE_LS=" + ls}, "gc", "--repo", k.R, "--dry-run", "--min-age", "0")
		// 그 항목만 모름(R-3M01) — 목록은 쓰되 이 기계의 작업 트리는 하나도 고르지 않는다
		k.check("R-202 gc: live cwd "+c+" 이면 모름 때문에 건너뜀, 아무것도 안 고름", k.rc == 0 && strings.Contains(k.out, "모름 때문에 건너뜀") && !strings.Contains(k.out, "지움 /"))
		k.runE([]string{"HC_FAKE_LS=" + ls}, "clean", "--repo", k.R, "--branch", "g3202l", "--dry-run")
		k.check("R-202 clean: live cwd "+c+" 이면 멈추고 작업 트리를 남긴다", k.rc != 0 && k.kept("g3202l") && !strings.Contains(k.out, "정리됨"))
	}
}

// g3Get: hc get에 body(JSON 한 줄, 표준 출력)와 종료 rc로 답하는 가짜. rc 3이면 body는 표준 오류에(진짜 hc 꼴)
func (k *K) g3Get(body, rc string) string {
	to := ""
	if rc == "3" {
		to = " >&2"
	}
	return k.gfBin(map[string]string{"hc": `[ "$1" = get ] && { printf '%s\n' ` + sq(body) + to + `; exit ` + rc + `; }; exit 3`})
}

// R-208·fill/R-206: 지휘의 실행 신원을 확인하지 못한 응답은 「없음」(1)이 아니라 모름(2). 확인된 끝남·완전한 not_found만 1
func g3CoordIdentity(k *K) {
	st := filepath.Join(k.T, "st208")
	x := "XDG_STATE_HOME=" + st
	full := `{"address":"t1","cwd":"/","name":"boss","pid":1234,"state":"live","status":null}`
	k.runE([]string{k.g3Get(full, "0"), x}, "coord", "--set", "t1")
	k.check("R-208 준비: coord --set t1", k.rc == 0)
	k.runE([]string{k.g3Get(full, "0"), x}, "coord", "--check", "t1")
	k.check("R-208 준비: 같은 신원이면 0", k.rc == 0)
	for _, c := range []struct{ what, body, rc string }{
		{"live·pid 없음", `{"address":"t1","cwd":"/","name":"boss","state":"live"}`, "0"},
		{"live·pid null", `{"address":"t1","cwd":"/","name":"boss","pid":null,"state":"live"}`, "0"},
		{"state starting", `{"address":"t1","cwd":"/","name":"boss","pid":1234,"state":"starting"}`, "0"},
		{"not_found·종료 124", `{"error":{"kind":"not_found"}}`, "124"},
		{"이름 칸 없음(같은 pid) — fill/R-206", `{"address":"t1","cwd":"/","pid":1234,"state":"live"}`, "0"},
		{"주소 다름", `{"address":"t2","cwd":"/","name":"boss","pid":1234,"state":"live"}`, "0"},
	} {
		k.runE([]string{k.g3Get(c.body, c.rc), x}, "coord", "--check", "t1")
		k.check("R-208 coord --check: "+c.what+" 은 모름(2)", k.rc == 2)
	}
	k.runE([]string{k.g3Get(`{"address":"t1","cwd":"/","name":"boss","pid":1234,"state":"finished"}`, "0"), x}, "coord", "--check", "t1")
	k.check("R-208 coord --check: 확인된 finished는 1", k.rc == 1)
	k.runE([]string{k.g3Get(`{"error":{"kind":"not_found","message":"no terminal"}}`, "3"), x}, "coord", "--check", "t1")
	k.check("R-208 coord --check: 완전한 not_found(종료 3)는 1", k.rc == 1)
	k.runE([]string{k.g3Get(`{"address":"t1","cwd":"/","name":"boss2","pid":99,"state":"live"}`, "0"), x}, "coord", "--check", "t1")
	k.check("R-208 coord --check: 완전한 다른 신원(pid·이름)은 1", k.rc == 1)
}

// R-221·merger/R-206: 구판이 0666으로 만든 지휘 기록도 갱신하면 0600으로 좁힌다
func g3CoordMode(k *K) {
	st := filepath.Join(k.T, "st221")
	f := st + "/hc-lane/coord.t1"
	write(k.t, f, "t1\t1234\tboss\t0\n")
	if err := os.Chmod(f, 0o666); err != nil {
		k.t.Fatal(err)
	}
	full := `{"address":"t1","cwd":"/","name":"boss","pid":1234,"state":"live"}`
	k.runE([]string{k.g3Get(full, "0"), "XDG_STATE_HOME=" + st}, "coord", "--set", "t1")
	fi, err := os.Stat(f)
	k.check("R-221 준비: coord --set 성공·기록 갱신", k.rc == 0 && err == nil && strings.HasPrefix(read(f), "t1\t1234\tboss\t"))
	k.check("R-221 coord --set: 기존 0666 기록이 0600으로", err == nil && fi.Mode().Perm() == 0o600)
}

// g3Send: hc ls에 ls로 답하고 send를 $HCLOG.send에 적는 가짜
func (k *K) g3Send(ls string) string {
	return k.gfBin(map[string]string{"hc": `case $1 in ls) printf '%s\n' ` + sq(ls) + `; exit 0 ;; send) printf '%s\n' "$2" >>"$HCLOG.send"; exit 0 ;; esac; exit 3`})
}

// R-209: 같은 주소의 옛 실행(다른 이름) 기록이나, 지금 부모와 어긋나는 기록으로 남의 레인에 보내지 않는다
func g3SendOwner(k *K) {
	st := filepath.Join(k.T, "st209")
	x := "XDG_STATE_HOME=" + st
	cur := `{"terminals":[{"address":"t8","cwd":"/","name":"new-lane","parent":"B","state":"live"}]}`
	write(k.t, st+"/hc-lane/lanes.tsv", "")
	os.Remove(k.HCLOG + ".send")
	k.runE([]string{k.g3Send(cur), x}, "send", "--all", "--from", "B", "확인")
	k.check("R-209 준비: 지금 부모 B에게는 보낸다", k.rc == 0 && strings.TrimSpace(read(k.HCLOG+".send")) == "t8")
	os.Remove(k.HCLOG + ".send")
	k.runE([]string{k.g3Send(cur), x}, "send", "--all", "--from", "A", "확인")
	k.check("R-209 준비: 기록 없으면 A에게는 안 보낸다", k.rc == 0 && read(k.HCLOG+".send") == "")
	write(k.t, st+"/hc-lane/lanes.tsv", "t8\told-lane\tA\n")
	k.runE([]string{k.g3Send(cur), x}, "send", "--all", "--from", "A", "확인")
	k.check("R-209 send --all: 같은 주소의 옛 레인(다른 이름) 기록으로 A가 보내지 않는다", read(k.HCLOG+".send") == "")
	write(k.t, st+"/hc-lane/lanes.tsv", "t8\tnew-lane\tA\n")
	k.runE([]string{k.g3Send(cur), x}, "send", "--all", "--from", "A", "확인")
	k.check("R-209 send --all: 기록 주인 A와 지금 부모 B가 어긋나면 보류(안 보냄, 종료 1)", k.rc == 1 && read(k.HCLOG+".send") == "" &&
		strings.Contains(k.out, "보류 t8"))
}

// R-210: 살아 있는 레인의 이름이 빠지면 감사·읽기 전용 제외를 가를 수 없다 — 어디에도 보내기 전에 실패
func g3SendName(k *K) {
	st := filepath.Join(k.T, "st210")
	x := "XDG_STATE_HOME=" + st
	write(k.t, st+"/hc-lane/lanes.tsv", "t8\taudit-case\tA\tread\n")
	os.Remove(k.HCLOG + ".send")
	named := `{"terminals":[{"address":"t7","cwd":"/","name":"impl","parent":"A","state":"live"},{"address":"t8","cwd":"/","name":"audit-case","parent":"A","state":"live"}]}`
	k.runE([]string{k.g3Send(named), x}, "send", "--all", "--from", "A", "확인")
	k.check("R-210 준비: 이름이 있으면 감사 레인만 빼고 보낸다", k.rc == 0 && strings.TrimSpace(read(k.HCLOG+".send")) == "t7")
	os.Remove(k.HCLOG + ".send")
	noname := `{"terminals":[{"address":"t7","cwd":"/","name":"impl","parent":"A","state":"live"},{"address":"t8","cwd":"/","parent":"A","state":"live"}]}`
	k.runE([]string{k.g3Send(noname), x}, "send", "--all", "--from", "A", "확인")
	k.check("R-210 send --all: 이름 빠진 레인이 있으면 아무에게도 안 보내고 실패", k.rc != 0 && read(k.HCLOG+".send") == "")
}

// R-211·fill/R-201: 시작 확인 hc get의 잘린 객체·status 칸 없는 응답은 성공(0)도 확인된 실패(5)도 아닌 미확정(8)
func g3StartPartial(k *K) {
	real := "hc"
	for _, d := range filepath.SplitList(k.env["PATH"]) {
		if exists(filepath.Join(d, "hc")) {
			real = filepath.Join(d, "hc")
			break
		}
	}
	k.mk("g3211")
	for i, body := range []string{`{"address":"t9zz","status":{`, `{"address":"t9zz","cwd":"/","name":"lane-p1","state":"live","turn":0}`} {
		n := []string{"lane-p0", "lane-p1"}[i]
		p := k.gfBin(map[string]string{"hc": `[ "$1" = get ] && case $2 in *t9zz) printf '%s\n' ` + sq(body) + `; echo x >>"$HCLOG.g3get"; exit 0 ;; esac; exec ` + sq(real) + ` "$@"`})
		os.Remove(k.HCLOG + ".g3get")
		k.runE([]string{p}, "--name", n, "--repo", k.R, "--start-wait", "1", "--", "claude")
		k.check("R-211 준비: 대역이 시작 확인 조회에 답했다("+n+")", strings.Count(read(k.HCLOG+".g3get"), "x") >= 1)
		k.check("R-211 시작 확인: "+body+" 는 미확정(종료 8, start unknown)", k.rc == 8 && strings.Contains(lastLine(k.out), `"start":"unknown"`))
	}
}
