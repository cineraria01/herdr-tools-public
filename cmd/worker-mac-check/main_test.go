package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	wm "herdr-tools/internal/workermac"
)

// 원격 줄 파싱: bash 「IFS=$'\t' read -r k a b c」과 같은 칸
func TestReadTab(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"R\t통과\t저장소\t/x (main, o)", []string{"R", "통과", "저장소", "/x (main, o)"}},
		{"R\t통과\t항목\t", []string{"R", "통과", "항목", ""}},
		{"V\tgit\t9.9.9", []string{"V", "git", "9.9.9", ""}},
		{"R\t실패\t항목\t세부\t탭 든 나머지\t", []string{"R", "실패", "항목", "세부\t탭 든 나머지"}},
		{"\tR\t\t통과\tx\ty", []string{"R", "통과", "x", "y"}},
		{"", []string{"", "", "", ""}},
	} {
		if got := readTab(c.in, 4); !reflect.DeepEqual(got, c.want) {
			t.Errorf("readTab(%q) = %q, 바람 %q", c.in, got, c.want)
		}
	}
	// 셸 판과 견준다
	for _, in := range []string{"R\t통과\t항목\t세부\t꼬리\t", "\t\tV\tgo\t", "R\t\t\tx"} {
		out, err := exec.Command("bash", "-c", `IFS=$'\t' read -r k a b c <<<"$1"; printf '%s|%s|%s|%s' "$k" "$a" "$b" "$c"`, "_", in).Output()
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(readTab(in, 4), "|"); got != string(out) {
			t.Errorf("readTab(%q) = %q, bash %q", in, got, out)
		}
	}
}

// 원격 줄을 받아 판정·개수: R은 그대로, V는 맥과 견줌, 그 밖은 버림
func TestRemoteLines(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".local/bin"), 0o755)
	os.WriteFile(filepath.Join(home, ".local/bin/fooz"), []byte("#!/bin/sh\necho 'fooz 1.2.3'; echo 둘째\n"), 0o755)
	os.WriteFile(filepath.Join(home, ".local/bin/git"), []byte("#!/bin/sh\necho 'git version 9.9.9'\n"), 0o755)
	t.Setenv("HOME", home)
	var b bytes.Buffer
	c := &checker{w: &b}
	c.remoteLines(context.Background(), "R\t통과\t자동 로그인\tt\nR\t모름\tFileVault\t?\nV\tfooz\tfooz 1.2.3\nV\tgit\t1.0\nV\tnope-xyz\t없음\n잡음\n")
	want := "통과\t자동 로그인\tt\n모름\tFileVault\t?\n통과\t판 fooz\tfooz 1.2.3 (맥과 같음)\n실패\t판 git\t1.0 (맥 9.9.9)\n실패\t판 nope-xyz\t없음 (맥 없음)\n"
	if b.String() != want {
		t.Errorf("줄\n%s바람\n%s", b.String(), want)
	}
	if c.p != 2 || c.f != 2 || c.u != 1 {
		t.Errorf("개수 %d %d %d", c.p, c.f, c.u)
	}
}

// 셸 인용: 원격 앞붙임을 bash가 읽으면 값 그대로(따옴표·$·*가 들어도)
func TestPreQuoting(t *testing.T) {
	s := settings{repos: "src/it's x*", trust: `$HOME "q"`, slots: "", tools: "git node"}
	out, err := exec.Command("bash", "-c", s.pre()+"\n"+`printf '%s|%s|%s|%s' "$REPOS" "$TRUST" "$SLOTS_FILE" "$TOOLS"`).Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := "src/it's x*|$HOME \"q\"||git node"; string(out) != want {
		t.Errorf("%q, 바람 %q", out, want)
	}
}

func TestHelpers(t *testing.T) {
	if ghAccount("✓ Logged in to github.com account tester (keyring)") != "tester" || ghAccount("not logged in") != "" {
		t.Error("ghAccount")
	}
	if squeeze("proxy running\nusable now: 2\n") != "proxy running usable now: 2 " || squeeze("a   b") != "a b" {
		t.Errorf("squeeze %q", squeeze("proxy running\nusable now: 2\n"))
	}
	re := connectedRe("w.1")
	if !re.MatchString(`{"name":"w.1","state":"connected"}`) || re.MatchString(`{"name":"wx1","state":"connected"}`) ||
		re.MatchString(`{"name":"w.1","state":"down"}`) || re.MatchString("{\"name\":\"w.1\",\n\"state\":\"connected\"}") {
		t.Error("connectedRe")
	}
	if n, ok := wm.PoolUsable("Usable Now: 3"); n != 3 || !ok {
		t.Error("PoolUsable") // 꼴 전부는 internal/workermac 시험
	}
}

// 설정: 키 없음은 기본값이나 miss, 오류는 err(종료 2 몫)
func TestLoad(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "herdr-tools-config"), []byte(`#!/bin/bash
case $2 in worker.repo) echo src/a ;; worker.tools) [ -n "${BAD-}" ] && { echo "같은 키 두 번" >&2; exit 2; }; exit 3 ;; *) exit 3 ;; esac
`), 0o755)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	t.Setenv("WORKER_MAC_BREW_DIR", "")
	s, err := load(context.Background())
	if err != nil || s.repos != "src/a" || s.trust != "Documents/app" || s.tools != "git node" || s.brew != "/opt/homebrew/bin" {
		t.Fatalf("%+v %v", s, err)
	}
	if want := []string{"worker.slots_file", "worker.proxy"}; !reflect.DeepEqual(s.miss, want) {
		t.Errorf("miss %v", s.miss)
	}
	t.Setenv("BAD", "1")
	if _, err := load(context.Background()); err == nil || !strings.Contains(err.Error(), "같은 키 두 번") {
		t.Errorf("오류 %v", err)
	}
}

// 상한 시간: ssh가 멈추면 SSH 연결 상한에 걸려 실패 줄·종료 2. 연결은 되는데 원격 점검이 멈추면 받은 줄까지 보고 hc 점검으로 넘어간다
func TestTimeout(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "ssh"), []byte("#!/bin/sh\n[ \"$SSH_HANG\" = connect ] && exec sleep 30\ncase \"$*\" in *'bash -c'*) printf 'R\\t통과\\t앞\\tx\\n'; exec sleep 30 ;; esac\n"), 0o755)
	os.WriteFile(filepath.Join(d, "hc"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	connectSec, remoteSec = 1, 1
	defer func() { connectSec, remoteSec = 15, 90 }()
	var b bytes.Buffer
	t.Setenv("SSH_HANG", "connect")
	start := time.Now()
	if rc := checkOne(context.Background(), &b, settings{}, "w1"); rc != 2 || !strings.Contains(b.String(), "실패\tSSH 연결\tssh w1 안 됨") {
		t.Errorf("rc %d\n%s", rc, b.String())
	}
	t.Setenv("SSH_HANG", "")
	b.Reset()
	// [B27] 받은 앞줄은 보존하되, 끊긴 원격 점검은 성공(0)이 아니다
	if rc := checkOne(context.Background(), &b, settings{}, "w1"); rc != 1 || !strings.Contains(b.String(), "통과\t앞\tx\n") ||
		!strings.Contains(b.String(), "실패\t원격 점검\t") ||
		!strings.Contains(b.String(), "모름\t키체인(hc 문맥)\thc 기계 w1 연결 안 됨") || !strings.Contains(b.String(), "-- w1: 통과 2 · 실패 1 · 모름 4") {
		t.Errorf("rc %d\n%s", rc, b.String())
	}
	if time.Since(start) > 15*time.Second {
		t.Error("상한이 안 먹음")
	}
}

// [E01·B27] 원격 점검 본문이 255(끊김)로 끝나면 받은 줄은 두고 「실패 원격 점검」 한 줄 — 종료 1
func TestRemoteCut(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "ssh"), []byte("#!/bin/sh\ncase \"$*\" in *'bash -c'*) printf 'R\\t통과\\t앞\\tx\\n'; exit 255 ;; esac\nexit 0\n"), 0o755)
	os.WriteFile(filepath.Join(d, "hc"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	var b bytes.Buffer
	if rc := checkOne(context.Background(), &b, settings{}, "w1"); rc != 1 || !strings.Contains(b.String(), "통과\t앞\tx\n") ||
		!strings.Contains(b.String(), "실패\t원격 점검\t") {
		t.Errorf("rc %d\n%s", rc, b.String())
	}
}

// [E01] ssh가 0이어도 끝 표식이 없으면(원격 bash가 중간에 죽음) 끊긴 것 — 표식이 있으면 「원격 점검」 줄 없이 통과. 목적지 앞에 --(E10)
func TestRemoteEndMark(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "ssh"), []byte("#!/bin/sh\necho \"$*\" >>"+d+"/args\ncase \"$*\" in *'bash -c'*) printf 'R\\t통과\\t앞\\tx\\n'; [ -n \"$MARK\" ] && printf 'E\\t끝\\n' ;; esac\nexit 0\n"), 0o755)
	os.WriteFile(filepath.Join(d, "hc"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	var b bytes.Buffer
	t.Setenv("MARK", "")
	if rc := checkOne(context.Background(), &b, settings{}, "w1"); rc != 1 || !strings.Contains(b.String(), "실패\t원격 점검\t끝 표식 없음") {
		t.Errorf("표식 없음: rc %d\n%s", rc, b.String())
	}
	b.Reset()
	t.Setenv("MARK", "1")
	if rc := checkOne(context.Background(), &b, settings{}, "w1"); rc != 0 || strings.Contains(b.String(), "원격 점검") || !strings.Contains(b.String(), "-- w1: 통과 2 · 실패 0 · 모름 4") {
		t.Errorf("표식 있음: rc %d\n%s", rc, b.String())
	}
	for _, l := range strings.Split(strings.TrimSpace(readF(d+"/args")), "\n") {
		if !strings.Contains(l, " -- w1 ") {
			t.Errorf("목적지 앞에 -- 없음: %q", l)
		}
	}
	if !strings.HasSuffix(remote, "printf 'E\\t끝\\n' # 끝 표식 — 맥 쪽은 이 줄이 없으면 점검이 끊긴 것으로 본다(E01)\n") {
		t.Error("remote.sh 마지막 줄이 끝 표식이 아님")
	}
	if cut("x\n", 142, 90) != "시간 초과(90초)" || cut("", 255, 90) != "끊김(종료 255)" || cut("E\t끝\n", 0, 90) != "" {
		t.Error("cut")
	}
}

// [E10] -로 시작하는 기계 이름
func TestDashName(t *testing.T) {
	if !dashName([]string{"w1", "-oProxyCommand=x"}) || dashName([]string{"w1", "w-2"}) {
		t.Error("dashName")
	}
}

// [B25] --version에 릴리스 커밋(-ldflags -X …version.Commit)이 보인다
func TestVersionShowsCommit(t *testing.T) {
	bin := t.TempDir() + "/worker-mac-check"
	if b, err := exec.Command("go", "build", "-ldflags", "-X herdr-tools/internal/version.Commit=abc123def456", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("빌드: %v %s", err, b)
	}
	if out, err := exec.Command(bin, "--version").Output(); err != nil || string(out) != "worker-mac-check (Go abc123def456)\n" {
		t.Fatalf("--version: %q %v", out, err)
	}
}

// 화면 기록의 herdr가 허용(2)이 아니면 실패 — UI 레인(ego-browser·화면 캡처)이 그 기계에서 안 된다(2026-11-26 한 워커 맥 herdr=거부가 「통과 … 필수 아님」에 묻힘).
// 다른 서비스·클라이언트는 기록만, TCC.db 못 읽음은 모름. 가짜 sqlite3로 remote.sh TCC 블록만 돌린다(실제 TCC.db는 안 읽음)
func TestTCCScreenCapture(t *testing.T) {
	i := strings.Index(remote, `db="/Library/Application Support/com.apple.TCC/TCC.db"`)
	j := strings.Index(remote, "printf 'E\\t끝\\n'")
	if i < 0 || j < i {
		t.Fatal("remote.sh TCC 블록 못 찾음")
	}
	block := `r() { printf 'R\t%s\t%s\t%s\n' "$1" "$2" "$3"; }` + "\n" + remote[i:j]
	run := func(rows string, rc int) string {
		d := t.TempDir()
		os.WriteFile(filepath.Join(d, "sqlite3"), []byte(fmt.Sprintf("#!/bin/sh\nprintf '%%s' \"$ROWS\"\nexit %d\n", rc)), 0o755)
		cmd := exec.Command("bash", "-c", block)
		cmd.Env = append(os.Environ(), "PATH="+d+":/usr/bin:/bin", "ROWS="+rows)
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	fda := "kTCCServiceSystemPolicyAllFiles|/x/herdr|0\n"
	for _, c := range []struct{ name, rows, want string }{
		{"거부", fda + "kTCCServiceScreenCapture|/Applications/Terminal.app|2\nkTCCServiceScreenCapture|/x/herdr|0\n",
			"R\t실패\tTCC 화면 기록\therdr=거부 — UI 레인(ego-browser·화면 캡처)이 그 기계에서 안 된다 — 그 맥 시스템 설정 › 개인정보 보호 및 보안 › 화면 및 시스템 오디오 녹음에서 herdr 허용 뒤 `hc-core-relaunch <기계>` (Terminal=허용 herdr=거부 node=없음 sshd-keygen-wrapper=없음)\n"},
		{"없음", fda, "R\t실패\tTCC 화면 기록\therdr=없음 — UI 레인"},
		{"제한", "kTCCServiceScreenCapture|/x/herdr|3\n", "R\t실패\tTCC 화면 기록\therdr=제한 — UI 레인"},
		{"허용", fda + "kTCCServiceScreenCapture|/x/herdr|2\n", "R\t통과\tTCC 화면 기록\t기록만: Terminal=없음 herdr=허용 node=없음 sshd-keygen-wrapper=없음 (워커 맥 필수 아님)\n"},
	} {
		out := run(c.rows, 0)
		if !strings.Contains(out, c.want) {
			t.Errorf("%s:\n%s바람 %q", c.name, out, c.want)
		}
		// 다른 서비스는 herdr가 거부·없음이어도 기록만
		if !strings.Contains(out, "R\t통과\tTCC 전체 디스크 접근\t기록만:") || !strings.Contains(out, "R\t통과\tTCC 손쉬운 사용\t기록만:") {
			t.Errorf("%s: 다른 서비스가 기록만이 아님\n%s", c.name, out)
		}
	}
	if out := run("", 1); strings.Count(out, "R\t모름\tTCC ") != 3 || !strings.Contains(out, "R\t모름\tTCC 화면 기록\t시스템 TCC.db를 못 읽음") {
		t.Errorf("못 읽음:\n%s", out)
	}
}

func readF(p string) string { b, _ := os.ReadFile(p); return string(b) }
