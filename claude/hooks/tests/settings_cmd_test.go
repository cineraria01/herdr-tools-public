package tests

// settings.json 명령 줄(hookCmds) 자체의 계약 — 셸 포장 claude/hooks/*.sh·lib/go-run.sh를 걷으며(2026-11-22 go-shell-remove) 재감사 지적
// R-H07·R-M02·R-L01·R-41이 같은 꼴로 다시 생기지 않게. 옛 꼴에서는 모두 실패한다(주석의 「옛 꼴」).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
)

// hookCase: 훅마다 「막을 만한」 입력과 환경 — 바이너리가 있으면 그 훅이 막았을 입력이다
var hookCases = []struct {
	name string
	in   []byte
	env  []string
}{
	{"cotenant-guard", []byte(`{"session_id":"s","cwd":"/tmp","tool_name":"Edit","tool_input":{"file_path":"/tmp/x"}}`), nil},
	{"loop-no-question", []byte(`{"tool_name":"AskUserQuestion","tool_input":{}}`), []string{"HC_TERMINAL_ID=t1"}},
	{"hc-lane-wait-guard", bashInput("hc-lane wait --mine &"), nil},
	{"kill-by-lookup-guard", bashInput("pkill socat"), nil},
	{"lane-no-shell-edit", bashInput("sed -i '' s/a/b/ main.go"), []string{"HC_LANE=lane-x"}},
}

// herdr-sync·herdr-settings-merge는 훅 항목을 keys.json hooks.PreToolUse의 판별 글자가 command에 「든」 것으로 짝짓는다(하나여야 한다).
// 명령 줄마다 판별 글자가 정확히 하나 = 그 훅 이름이어야 hook:PreToolUse:<이름> 열쇠가 선다(옛 포장 명령 …/<이름>.sh에도 그 글자가 들어
// 같은 항목을 갈아 끼운다 — 둘이 함께 남지 않는다)
func TestHookCmdSyncMarker(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "config.example", "settings", "keys.json"))
	if err != nil {
		t.Fatal(err)
	}
	var keys struct {
		Hooks map[string][]string `json:"hooks"`
	}
	if err := json.Unmarshal(b, &keys); err != nil {
		t.Fatal(err)
	}
	marks := keys.Hooks["PreToolUse"]
	for name, cmd := range hookCmds {
		var hit []string
		for _, m := range marks {
			if strings.Contains(cmd, m) {
				hit = append(hit, m)
			}
		}
		if len(hit) != 1 || hit[0] != name {
			t.Errorf("%s 명령 줄의 판별 글자 %v(want [%s]만)", name, hit, name)
		}
	}
	if len(marks) != len(hookCmds) {
		t.Errorf("예시 keys.json 판별 글자 %d개, 훅 %d개", len(marks), len(hookCmds))
	}
}

// R-H07: ~/.local/bin/herdr-hooks가 없거나 실행할 수 없으면 안전 장치(cotenant-guard)는 막고(2·까닭 한 줄), 편의 막·알림 넷은 통과(0).
// 옛 꼴: 등록 명령 `[ -r ~/.claude/hooks/cotenant-guard.sh ] || exit 0`이 파일이 없으면 cotenant-guard도 통과시켰고, 포장이 바이너리를
// 못 쓰면 자동으로 셸 판으로 떨어졌다(launchd 문맥·설치 안 됨에서 임대 판정이 셸 판으로 — 미커밋·소유권 불명 차단이 빠짐)
func TestHookCmdNoBinary(t *testing.T) {
	for _, layout := range []string{"없음", "실행 권한 없음"} {
		for _, h := range hookCases {
			t.Run(layout+"/"+h.name, func(t *testing.T) {
				H := t.TempDir()
				if layout == "실행 권한 없음" {
					p := filepath.Join(H, ".local", "bin", "herdr-hooks")
					if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				rc, _, errOut := runOut(t, hookExec(h.name, h.in, envWith([]string{"HC_LANE", "HC_TERMINAL_ID"}, append(h.env, "HOME="+H)...), ""))
				if h.name == "cotenant-guard" {
					if rc != 2 || !strings.Contains(errOut, "herdr-hooks") || strings.Count(strings.TrimSpace(errOut), "\n") != 0 {
						t.Errorf("안전 장치: rc=%d(want 2, 까닭 한 줄) err=[%s]", rc, errOut)
					}
				} else if rc != 0 {
					t.Errorf("알림 훅: rc=%d(want 0 — 막지 않는다) err=[%s]", rc, errOut)
				}
			})
		}
	}
}

// R-L01·R-41: 운영 빌드(태그 없음 — clitest.Bin)는 실행 파일·판정을 바꿔 끼우는 환경을 받지 않는다. 명령 줄은 고정 자리 하나만 exec하고
// (HERDR_HOOKS_BIN·HERDR_HOOKS_IMPL·HERDR_IMPL을 안 봄), 바이너리는 시험 손잡이(HERDR_HOOKS_TEST_*)를 시험 빌드(-tags hookstest)에서만 먹는다.
// 옛 꼴: lib/go-run.sh가 HERDR_HOOKS_BIN을 운영 훅에서도 받아 그 파일을 판 대조 없이 exec했다
func TestHookCmdIgnoresSwapEnv(t *testing.T) {
	prod := clitest.Bin(t, "herdr-hooks")
	for _, b := range []struct {
		name, bin string
		want      int
	}{{"운영 빌드", prod, 0}, {"시험 빌드(대조 — 손잡이가 먹으면 막음)", buildHooks(t), 2}} {
		t.Run(b.name, func(t *testing.T) {
			c := newCG(t)
			c.fatal(os.Remove(c.H + "/.local/bin/herdr-hooks"))
			putBin(t, c.H, b.bin)
			R, _, _ := repoAB(c)
			marker := c.T + "/swapped"
			c.write(c.T+"/swap", "#!/bin/sh\n: > \""+marker+"\"\nexit 2\n")
			c.fatal(os.Chmod(c.T+"/swap", 0o755))
			c.write(c.T+"/badgit", "#!/bin/sh\necho broken >&2\nexit 1\n")
			c.fatal(os.Chmod(c.T+"/badgit", 0o755))
			c.hookEnv = []string{"HERDR_HOOKS_BIN=" + c.T + "/swap", "HERDR_HOOKS_IMPL=go", "HERDR_IMPL=go",
				"HERDR_HOOKS_TEST_GIT=" + c.T + "/badgit", "HERDR_HOOKS_TEST_NOW_OFFSET=61", "HERDR_HOOKS_TEST_SECONDS=5"}
			c.run("sa", "Edit", R+"/a.txt")
			if c.RC != b.want {
				t.Errorf("cotenant-guard rc=%d(want %d) err=[%s]", c.RC, b.want, c.ERR)
			}
			c.hook = "kill-by-lookup-guard"
			c.run("sa", "Bash", "kill 12345")
			if c.RC != 0 {
				t.Errorf("kill-by-lookup-guard rc=%d(want 0) err=[%s]", c.RC, c.ERR)
			}
			if exists(marker) {
				t.Error("HERDR_HOOKS_BIN으로 건 실행 파일이 불렸다")
			}
		})
	}
}

// R-M02: 명령 줄은 첫 호출 준비(--version 확인·go build·판 대조)를 하지 않고 바로 판정한다 — 첫 호출도 cotenant 훅 제한 5초 안.
// 옛 꼴: go-run.sh가 판정 앞에 --version(최대 10초)·소스 판 계산·뒤 짓기를 먼저 해, 첫 --version이 5.3초면 전체 5.55초였다
func TestHookCmdFirstCall(t *testing.T) {
	for name, cmd := range hookCmds {
		for _, bad := range []string{"--version", "go build", "go-run", "go_run", "GO_RUN", ".sh"} {
			if strings.Contains(cmd, bad) {
				t.Errorf("%s 명령 줄에 %q", name, bad)
			}
		}
		if !strings.HasSuffix(cmd, `exec "$HOME/.local/bin/herdr-hooks" `+name) {
			t.Errorf("%s 명령 줄이 고정 자리 바이너리를 바로 exec하지 않는다: %s", name, cmd)
		}
	}
	// --version에 6초 잠드는 가짜 바이너리·불리면 적는 가짜 go: 명령 줄은 그 훅 하나만 부르고 go를 부르지 않는다
	t.Run("준비 호출 없음", func(t *testing.T) {
		T := t.TempDir()
		log := T + "/calls"
		for p, body := range map[string]string{
			T + "/home/.local/bin/herdr-hooks": "#!/bin/sh\necho \"$*\" >> \"" + log + "\"\n[ \"$1\" = --version ] && sleep 6\nexit 0\n",
			T + "/bin/go":                      "#!/bin/sh\necho \"go $*\" >> \"" + log + "\"\n",
		} {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		var want []string
		for _, h := range hookCases {
			t0 := time.Now()
			rc, _, errOut := runOut(t, hookExec(h.name, h.in, envWith(nil, "HOME="+T+"/home", "PATH="+T+"/bin:/usr/bin:/bin"), ""))
			if d := time.Since(t0); rc != 0 || d > 5*time.Second {
				t.Errorf("%s: rc=%d(want 0), %v(want 5초 안) err=[%s]", h.name, rc, d, errOut)
			}
			want = append(want, h.name)
		}
		got, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		if g := strings.TrimSpace(string(got)); g != strings.Join(want, "\n") {
			t.Errorf("불린 것 [%s], want 훅 이름만 차례대로 [%s]", g, strings.Join(want, " "))
		}
	})
	// 막 깐 운영 바이너리(옆에 .ok·.src 같은 표식 없음)의 첫 cotenant 판정이 5초 안 — 부하에 흔들리지 않게 상한은 훅 제한 그대로
	t.Run("막 깐 바이너리 첫 판정", func(t *testing.T) {
		src, err := os.ReadFile(clitest.Bin(t, "herdr-hooks"))
		if err != nil {
			t.Fatal(err)
		}
		c := newCG(t)
		p := c.H + "/.local/bin/herdr-hooks"
		c.fatal(os.Remove(p))
		c.fatal(os.WriteFile(p, src, 0o755))
		R, _, _ := repoAB(c)
		t0 := time.Now()
		c.run("sa", "Edit", R+"/a.txt")
		if d := time.Since(t0); c.RC != 0 || d > 5*time.Second {
			t.Errorf("rc=%d(want 0), %v(want 5초 안) err=[%s]", c.RC, d, c.ERR)
		}
	})
}
