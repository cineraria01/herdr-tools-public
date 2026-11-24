//go:build !windows

package main

// machine-rename(2026-11-24): hc-fill machine --rename <옛> <새>. 견본 — 설정·상태 파일·손잡이·원격 켬 표식에 옛 이름(box-old)이 기계 칸·
// 목록 항목·주소 접두로 있고, 글 칸·레인 이름·앞부분이 같은 다른 기계(box-old-2)에도 같은 낱말이 있다. 견본을 {M} 자리표로 쓰고
// 바꾼 뒤 내용이 {M}=box-new로 채운 것과 바이트까지 같아야 한다(① 옛 이름 0칸 ② 글 칸 그대로 ③ 목록 항목만 ④ 앞부분 같은 이름 그대로,
// 다른 기계 줄 바이트 같음). 고치기 전 코드는 --rename을 사용법으로 거절(종료 2)

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"herdr-tools/internal/lockx"
)

// rnFixture: 경로(HOME 기준) → 내용 틀. {M} 바꿀 기계 이름, {CLONE} 이 기계 클론
var rnFixture = map[string]string{
	"conf/hc-fill.tsv": "# 예시 — box-old 는 주석이라 그대로\nlocal here\ncoord {M}/t5\nmachine  here   2 - -\nmachine  {M}    2 - -\n" +
		"machine  box-old-2 1 - -\nmachine-has {M}  xcode\nrepo app here  {CLONE}\nrepo app {M}   /srv/box-old/app\nrepo app box-old-2 /srv/app\n" +
		"merger app {M}\nmerger_verifiers app box-old-2,{M}\ninbox {M} ~/inbox\nidle 10\n",
	"data/queue.tsv": "lane-up\tapp\t{B}\t-\t{M},box-old-2\tup\t{M}\t{M}/t1\t{M}={M}/t9,box-old-2=box-old-2/t3\tt\t-\t{M}/t5\tnormal\t-\tauto\t-\tr1\n" +
		"audit-a1\tapp\t{B}\t-\t{M}\twait\t-\t-\t-\tt\t-\tt5\turgent\tno-early\taudit=lane-up\t-\tr2\n" +
		"lane-b\tapp\t{B}\t-\tbox-old-2\tup\tbox-old-2\tbox-old-2/t3\t-\tt\tbox-old-2:~/inbox#q1\tt5\tnormal\t-\t-\t-\tr3\n" +
		"box-old\tapp\t{B}\tbox-old\t-\twait\t-\t-\t-\tt\t{M}:~/inbox#q2\t-\tnormal\t-\t-\t-\tr4\n",
	"data/inbox.tsv":                        "q2\tbox-old\t{M}:~/inbox#q2\topen\tresult\t-\t-\tbox-old 비고\tt\nq1\tlane-b\tbox-old-2:~/inbox#q1\topen\tresult\t-\t-\t-\tt\n",
	"data/audit.tsv":                        "a1\tlane-up\t1\trun1\tsha\t{M}\t{M}/t1\tqueued\t0\t-\t0\t0\t-\t-\t0\t-\tbox-old 에서 봄\na2\tlane-b\t1\trun2\tsha\tbox-old-2\tbox-old-2/t3\tqueued\t0\t-\t0\t0\t-\t-\t0\t-\t-\n",
	"data/try.tsv":                          "lane-up\tr1\t{M}\t-\tauto\t{M}/t1\tt1@1\nlane-x\tr9\tbox-old-2\t-\t-\t?\t-\tbox-old-2/t4\t2\n",
	"data/reject.tsv":                       "r5\tlane-r\treject\t1\t2\t-\t{M}\tbox-old 닿지 않음\n",
	"data/own.tsv":                          "lane-up\tapp\t{M}\t{M}/t1\ts\tb\t,-,\t-\nlane-p\tapp\t{M}\tpending:{M}\ts\tb\t,-,\t-\n",
	"data/overlap.tsv":                      "lane-w\tlane-up\tk\tbox-old-2,{M}\tapp\n",
	"data/merger.tsv":                       "app\t{M}\n",
	"data/merger-alive.tsv":                 "app\t{M}\talive\t1\t{M}/t7\t0\t0\t0\t-\tbox-old 살아 있음\n",
	"data/load.tsv":                         "{M}\t1.00\t8\t1\t1\t2\t-\t0\t0\nbox-old-2\t1.00\t8\t1\t1\t1\t-\t0\t0\n",
	"data/reach.tsv":                        "{M}\tup\t1\t0\tbox-old\n",
	"data/idle.tsv":                         "{M}\t1\t0\t0\t1\tbox-old 빔\n",
	"data/held.tsv":                         "{M}/t1\t100\nt9\t100\n",
	"data/seen.tsv":                         "{M}\tlane-up\t{M}/t1\n",
	"data/nudge.tsv":                        "lane-up\t{M}/t1\t100\n",
	"data/onland.tsv":                       "lane-up\tapp-dev\tapp\t{M}/t5\t100\n",
	"data/onland/app-dev.pending":           "lane-up\t{M}/t5\tsha\n",
	"data/reland.tsv":                       "#since\t1\nf1\tlane-r\tmain\t보냄\t{M}/t8\tt\t{M}/t5\tmsg\nf1\tlane-r\tmain\t알림\tbox-old/t8\tt\n",
	"data/machine.log":                      "t\tt5\tbox-old\tpause\t-\t-\n",
	"home/.local/state/herdr-ops/knobs.tsv": "# 머리\nseats@{M}\t3\tt5\t왜\t1\t-\t0\npause@box-old-2\t1\tt5\t왜\t1\t-\t0\n",
}

type rnK struct {
	fillKit
	root, clone, bare string
}

func rnBin(t *testing.T) string {
	t.Helper()
	b := t.TempDir() + "/hc-fill"
	if out, err := exec.Command("go", "build", "-o", b, ".").CombinedOutput(); err != nil {
		t.Fatalf("빌드: %v %s", err, out)
	}
	return b
}

func rnGit(t *testing.T, dir string, a ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, a...)...)
	c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", a, err, out)
	}
	return strings.TrimSpace(string(out))
}

// rnSetup: 견본을 옛 이름으로 깔고, 원격(맨 저장소)에 refs/herdr/merger/box-old
func rnSetup(t *testing.T, bin string) *rnK {
	t.Helper()
	d := t.TempDir()
	k := &rnK{root: d, clone: d + "/clone", bare: d + "/origin.git"}
	k.bin, k.home, k.data = bin, d+"/home", d+"/data"
	rnGit(t, d, "init", "-q", "--bare", k.bare)
	rnGit(t, d, "init", "-q", k.clone)
	rnGit(t, k.clone, "commit", "-q", "--allow-empty", "-m", "c")
	rnGit(t, k.clone, "remote", "add", "origin", k.bare)
	rnGit(t, k.clone, "push", "-q", "origin", "HEAD:refs/herdr/merger/box-old")
	for p, s := range rnFixture {
		f := d + "/" + p
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte(k.fill(s, "box-old")), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	k.env = append(os.Environ(), "HOME="+k.home, "HERDR_TOOLS_CONFIG="+d+"/conf", "HC_FILL_DATA="+k.data, "HERDR_TOOLS_OPLOG="+d+"/ops.tsv",
		"HERDR_TOOLS_JOURNAL="+d+"/journal.jsonl", "HC_TERMINAL_ID=", "GIT_CONFIG_GLOBAL=/dev/null", "HC_FILL_THEN_WAIT=0")
	return k
}

// tickEnv: 바퀴 한 번을 돌릴 가짜 hc(그 기계를 box-new로 보임 — lane-up은 box-new/t1에서 일하는 중)·hc-lane·herdr-balance와
// 띄우기·감사 설정 줄. hc·hc-lane 부름은 calls.log에
func (k *rnK) tickEnv(t *testing.T) {
	t.Helper()
	b := k.root + "/bin"
	ts := `{"terminals":[{"address":"t5","agent":"claude-code","name":"coord","cwd":"/","state":"live","status":{"state":"working"}},` +
		`{"address":"box-new/t1","agent":"claude-code","name":"lane-up","cwd":"/srv/box-old/app.wt/lane-up","state":"live","status":{"state":"working"}}]}`
	files := map[string]string{
		b + "/hc": "#!/bin/sh\necho \"hc $*\" >>\"$RN_LOG\"\ncase \"$1 $2\" in\n" +
			"'machine ls') echo '{\"machines\":[{\"name\":\"box-new\",\"state\":\"connected\"},{\"name\":\"box-old-2\",\"state\":\"connected\"}]}' ;;\n" +
			"'ls --json') echo '" + ts + "' ;;\n'ls --label') echo '{\"terminals\":[{\"address\":\"box-new/t1\",\"agent\":\"claude-code\",\"name\":\"lane-up\",\"state\":\"live\",\"status\":{\"state\":\"working\"}}]}' ;;\n" +
			"get*) echo '{\"address\":\"'$2'\",\"created_ms\":1,\"cwd\":\"/\",\"name\":\"lane-up\",\"state\":\"live\",\"status\":{\"app\":\"claude-code\",\"state\":\"working\"}}' ;;\n" +
			"*) exit 1 ;;\nesac\n",
		b + "/hc-lane":       "#!/bin/sh\necho \"hc-lane $*\" >>\"$RN_LOG\"\nexit 0\n",
		b + "/herdr-balance": "#!/bin/sh\nexit 0\n",
		k.root + "/audit.md": "# 감사 {lane}\n본문 {audit_id}\n",
		k.root + "/b.md":     "# 일\n본문\n",
	}
	for p, s := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.OpenFile(k.root+"/conf/hc-fill.tsv", os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("gc 0\nagent claude-code\nlaunch sh -c 'true' _ {task}\naudit " + k.root + "/audit.md\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}
	k.env = append(k.env, "PATH="+b+":/usr/bin:/bin", "RN_LOG="+k.root+"/calls.log")
}

func (k *rnK) fill(s, m string) string {
	return strings.NewReplacer("{M}", m, "{CLONE}", k.clone, "{B}", k.root+"/b.md").Replace(s)
}

// want: 견본을 그 이름으로 채운 파일들
func (k *rnK) want(m string) map[string]string {
	w := map[string]string{}
	for p, s := range rnFixture {
		w[k.root+"/"+p] = k.fill(s, m)
	}
	return w
}

// snap: 견본 파일들의 지금 내용(일지·운영 기록 등 견본 밖 파일은 빼고)
func (k *rnK) snap(t *testing.T) map[string]string {
	t.Helper()
	s := map[string]string{}
	for p := range rnFixture {
		b, err := os.ReadFile(k.root + "/" + p)
		if err != nil {
			t.Fatal(err)
		}
		s[k.root+"/"+p] = string(b)
	}
	return s
}

func rnSame(t *testing.T, what string, got, want map[string]string) {
	t.Helper()
	for p, w := range want {
		if got[p] != w {
			t.Errorf("%s: %s\n  지금 %q\n  바람 %q", what, p, got[p], w)
		}
	}
}

// refs: 원격의 병합기 켬 표식 「이름=sha」
func (k *rnK) refs(t *testing.T) string {
	t.Helper()
	return rnGit(t, k.clone, "for-each-ref", "--format=%(refname)", "refs/herdr/") + "|" + rnGit(t, k.bare, "for-each-ref", "--format=%(refname)=%(objectname)", "refs/herdr/merger/")
}

func TestMachineRename(t *testing.T) {
	bin := rnBin(t)

	// ①~④ 바꾼 뒤 모든 견본이 새 이름으로 채운 것과 바이트까지 같다 · ⑥(앞) 표식이 같은 커밋으로 옮겨짐 · ⑩ 일지 · ⑨ 되돌리기로 처음과 같음
	t.Run("rename", func(t *testing.T) {
		k := rnSetup(t, bin)
		orig := k.snap(t)
		sha := rnGit(t, k.bare, "rev-parse", "refs/herdr/merger/box-old")
		out, se, rc := k.run(t, "machine", "--rename", "box-old", "box-new", "--why", "망 이름과 같게")
		if rc != 0 {
			t.Fatalf("rename: 종료 0이어야 — %d %q %q", rc, out, se)
		}
		rnSame(t, "바꾼 뒤", k.snap(t), k.want("box-new"))
		if got := rnGit(t, k.bare, "for-each-ref", "--format=%(refname)=%(objectname)", "refs/herdr/merger/"); got != "refs/herdr/merger/box-new="+sha {
			t.Fatalf("켬 표식이 같은 커밋으로 옮겨져야: %q", got)
		}
		for _, s := range []string{"hc-fill merger restart app", "설정 파일을 고쳤다: " + k.root + "/conf/hc-fill.tsv 7줄 — 커밋은 직접", "떠 있는 줄 lane-up"} {
			if !strings.Contains(out, s) {
				t.Errorf("다음 명령에 %q 가 없다:\n%s", s, out)
			}
		}
		js := ehJournal(t, k.fillKit, "machine rename")
		if len(js) != 1 || js[0]["undo"] != "hc-fill machine --rename box-new box-old" || js[0]["why"] != "망 이름과 같게" || js[0]["target"] != "box-old → box-new" {
			t.Fatalf("일지: %v", js)
		}
		if a, err := os.ReadFile(k.data + "/machine-alias.tsv"); err != nil || !strings.HasPrefix(string(a), "box-old\tbox-new\t") || strings.Count(string(a), "\n") != 1 {
			t.Fatalf("옛 이름 기록 「box-old box-new 초」 한 줄이어야: %q %v", a, err)
		}
		if _, se, rc := k.run(t, "machine", "--rename", "box-new", "box-old"); rc != 0 {
			t.Fatalf("되돌리기: %d %q", rc, se)
		}
		if a, _ := os.ReadFile(k.data + "/machine-alias.tsv"); !strings.HasPrefix(string(a), "box-new\tbox-old\t") || strings.Count(string(a), "\n") != 1 {
			t.Fatalf("되돌린 뒤 옛 이름 기록은 「box-new box-old」 한 줄(앞 줄은 빠짐): %q", a)
		}
		rnSame(t, "되돌린 뒤", k.snap(t), orig)
		if got := rnGit(t, k.bare, "for-each-ref", "--format=%(refname)=%(objectname)", "refs/herdr/merger/"); got != "refs/herdr/merger/box-old="+sha {
			t.Fatalf("되돌린 뒤 표식: %q", got)
		}
	})

	// ⑤ 떠 있는 레인·감사 기다리는 줄: hc가 그 기계를 새 이름으로 보인 뒤 바꾸면 다음 바퀴가 레인을 다시 띄우지 않고(살아 있는 그 주소를 이어 봄,
	// 끝난 것으로 치지 않음) 감사 줄은 새 이름 기계에 놓는다
	t.Run("tick", func(t *testing.T) {
		tick := func(rename bool) (calls string, up Row) {
			k := rnSetup(t, bin)
			k.tickEnv(t)
			if rename {
				if _, se, rc := k.run(t, "machine", "--rename", "box-old", "box-new"); rc != 0 {
					t.Fatalf("rename: %d %q", rc, se)
				}
			}
			out, se, rc := k.run(t, "run")
			b, err := os.ReadFile(k.root + "/calls.log")
			if rc != 0 || err != nil {
				t.Fatalf("run: %d %v\n%s\n%s", rc, err, out, se)
			}
			for _, r := range readRowsT(t, k.data+"/queue.tsv") {
				if r.F(qName) == "lane-up" {
					up = r
				}
			}
			return string(b), up
		}
		// 바꾸지 않으면(설정·상태는 옛 이름, hc는 새 이름) 감사가 그 기계에 안 뜬다 — 시험이 헛돌지 않는다
		if c, _ := tick(false); strings.Contains(c, "--on box-new --name audit-a1") {
			t.Fatalf("바꾸기 전: 감사가 새 이름 기계에 놓였다 — 시험이 헛돈다\n%s", c)
		}
		c, up := tick(true)
		if !strings.Contains(c, "hc-lane --print-steps --read --on box-new --name audit-a1 ") {
			t.Errorf("감사 줄이 새 이름 기계에 놓여야:\n%s", c)
		}
		if strings.Contains(c, "--name lane-up") {
			t.Errorf("떠 있는 lane-up을 다시 띄웠다:\n%s", c)
		}
		if up.F(qState) != "up" || up.F(qMach) != "box-new" || up.F(qAddr) != "box-new/t1" || up.F(qRID) != "r1" {
			t.Errorf("lane-up은 같은 줄 ID로 새 이름 주소에 떠 있어야(끝난 것으로 치지 않음): %q", up)
		}
	})

	// ⑦ dry-run: 바꿀 자리마다 파일·줄 수·본보기, 표식, 떠 있는 것 — 아무것도 안 바꿈. --json도 같은 사실
	t.Run("dry-run", func(t *testing.T) {
		k := rnSetup(t, bin)
		orig, refs := k.snap(t), k.refs(t)
		out, se, rc := k.run(t, "machine", "--rename", "box-old", "box-new", "--dry-run")
		if rc != 0 || !strings.Contains(out, "미리 보기") || !strings.Contains(out, k.data+"/queue.tsv  3줄  예: lane-up") ||
			!strings.Contains(out, "병합기 app  refs/herdr/merger/box-old → refs/herdr/merger/box-new") || !strings.Contains(out, "떠 있는 줄: lane-up") {
			t.Fatalf("dry-run 출력: %d %q %q", rc, out, se)
		}
		rnSame(t, "dry-run 뒤", k.snap(t), orig)
		if k.refs(t) != refs {
			t.Fatal("dry-run이 원격을 바꿨다")
		}
		out, _, rc = k.run(t, "machine", "--rename", "box-old", "box-new", "--dry-run", "--json")
		var x struct {
			DryRun bool `json:"dry_run"`
			Files  []struct {
				Path  string `json:"path"`
				Lines int    `json:"lines"`
			} `json:"files"`
			Refs []struct{ Project string } `json:"merger_refs"`
		}
		if rc != 0 || json.Unmarshal([]byte(out), &x) != nil || !x.DryRun || len(x.Refs) != 1 || len(x.Files) != 21 {
			t.Fatalf("dry-run --json: %d %q %+v", rc, out, x)
		}
		rnSame(t, "dry-run --json 뒤", k.snap(t), orig)
		if _, err := os.Stat(k.data + "/machine-alias.tsv"); err == nil {
			t.Fatal("dry-run이 옛 이름 기록을 썼다")
		}
		// 감사 낮음: 상태 파일에 새 이름의 묵은 줄(설정에서 지운 기계)이 있으면 알린다
		f, err := os.OpenFile(k.data+"/load.tsv", os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString("box-new\t1.00\t8\t1\t1\t1\t-\t0\t0\n")
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			t.Fatal(err)
		}
		if out, _, _ := k.run(t, "machine", "--rename", "box-old", "box-new", "--dry-run"); !strings.Contains(out, "이미 box-new 이 기계 칸에 든 줄이 1줄") {
			t.Fatalf("묵은 새 이름 줄 알림이 없다:\n%s", out)
		}
	})

	// ⑥ 원격 쓰기 실패(받는 쪽이 거절) → 파일도 원래대로, 표식 그대로
	t.Run("remote-fail", func(t *testing.T) {
		k := rnSetup(t, bin)
		if err := os.WriteFile(k.bare+"/hooks/pre-receive", []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		orig, refs := k.snap(t), k.refs(t)
		_, se, rc := k.run(t, "machine", "--rename", "box-old", "box-new")
		if rc != 1 || !strings.Contains(se, "push 실패") {
			t.Fatalf("원격 실패: 종료 1이어야 — %d %q", rc, se)
		}
		rnSame(t, "원격 실패 뒤", k.snap(t), orig)
		if k.refs(t) != refs {
			t.Fatal("원격 실패 뒤 표식이 바뀌었다")
		}
		if js := ehJournal(t, k.fillKit, "machine rename"); len(js) != 0 {
			t.Fatalf("실패한 바꿈이 일지에: %v", js)
		}
		if _, err := os.Stat(k.data + "/machine-alias.tsv"); err == nil {
			t.Fatal("원격 실패 뒤 옛 이름 기록이 남았다(새로 만든 파일은 지워야)")
		}
	})

	// 도중 쓰기 실패(뒤쪽 파일의 폴더가 읽기 전용) → 앞서 쓴 파일도 원래대로
	t.Run("write-fail", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root는 읽기 전용 폴더에도 쓴다")
		}
		k := rnSetup(t, bin)
		ro := k.data + "/onland"
		if err := os.Chmod(ro, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(ro, 0o755) })
		orig, refs := k.snap(t), k.refs(t)
		_, se, rc := k.run(t, "machine", "--rename", "box-old", "box-new")
		if rc != 1 || !strings.Contains(se, "원본으로 되돌렸다") {
			t.Fatalf("쓰기 실패: 종료 1이어야 — %d %q", rc, se)
		}
		rnSame(t, "쓰기 실패 뒤", k.snap(t), orig)
		if k.refs(t) != refs {
			t.Fatal("쓰기 실패 뒤 표식이 바뀌었다")
		}
	})

	// ⑧ 거절 넷: 새 이름이 이미 기계 줄에 · 옛 이름이 설정에 없음 · 이름 꼴 · 잠금을 못 쥠 — 모두 아무것도 안 바꿈
	t.Run("refuse", func(t *testing.T) {
		k := rnSetup(t, bin)
		orig, refs := k.snap(t), k.refs(t)
		for _, c := range []struct {
			a    []string
			rc   int
			want string
		}{
			{[]string{"box-old", "box-old-2"}, 2, "이미 있다"},
			{[]string{"box-old", "here"}, 2, "이미 있다"},
			{[]string{"nope", "box-new"}, 2, "없는 기계"},
			{[]string{"box-old", "Box_New"}, 2, "이름 꼴"},
			{[]string{"box-old", "-x"}, 2, "사용법"},
		} {
			_, se, rc := k.run(t, append([]string{"machine", "--rename"}, c.a...)...)
			if rc != c.rc || !strings.Contains(se, c.want) {
				t.Errorf("%v: 종료 %d·%q 바람, 받음 %d %q", c.a, c.rc, c.want, rc, se)
			}
		}
		f, busy, err := lockx.TryLock(k.data + "/run.lock")
		if err != nil || busy {
			t.Fatal(err, busy)
		}
		_, se, rc := k.run(t, "machine", "--rename", "box-old", "box-new")
		f.Close()
		if rc != 75 {
			t.Errorf("잠금을 못 쥠: 종료 75 바람, 받음 %d %q", rc, se)
		}
		rnSame(t, "거절 뒤", k.snap(t), orig)
		if k.refs(t) != refs {
			t.Fatal("거절 뒤 표식이 바뀌었다")
		}
	})
}
