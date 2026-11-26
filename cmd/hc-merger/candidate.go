package main

// 후보 작업 트리에서 묶음 쌓기와 착지 부르기(bash 판 mround의 뒤쪽·mrun_candidate).

import (
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/proc"
)

// stack: 묶음 뒤쪽 구성원을 앞 구성원 위에 차례로 얹는다(그 제출이 기준에서 갈라진 뒤의 커밋만). 충돌하는 구성원은 이번 묶음에서
// 빼고 큐에 그대로. 착지를 부를 수 있으면 true
func (m *M) stack() bool {
	// 뒤 제출이 더 새 기준에서 되돌린 변경을 옛 트리에 얹으면 빈 커밋으로 사라진다 — 첫 후보를 이번 기준으로 맞춘다.
	// 충돌하면 원래 제출 하나로 착지의 rerere 절차를 탄다
	if inLog(m.mwt, m.lgf, "rebase", "-q", "--reapply-cherry-picks", m.B) != 0 {
		m.jround(jFront, "맨 앞 "+m.b[0].l+" 를 기준 위로 다시 얹으면 충돌 — 착지 없이 묶음 크기 1로", ", 로그 "+m.lgf)
		m.writeCap(1)
		m.fin()
		return false
	}
	// 승인 구성원은 얹은 결과가 승인 원본의 순변경과 같을 때만 묶음에 남는다(R-H01). 맨 앞이 증명되지 않으면 혼자(착지가 판정을 본다)
	k := []int{0}
	h0, rc := out("rev-parse", "--verify", "refs/heads/"+m.br)
	if rc != 0 {
		m.defer_("후보 가지 조회 실패")
		return false
	}
	front := m.applied(m.b[0], m.B, h0)
	if front == apUnknown {
		m.defer_("승인 구성원의 후보 트리 대조 실패")
		return false
	}
	for i := 1; front == apOK && i < len(m.b); i++ { // 맨 앞이 다르면 뒤를 얹지 않는다 — 아래에서 원본 제출 하나로 줄인다
		mf, rc := out("merge-base", m.b[i].s, m.B)
		if rc != 0 {
			dieGit(rc, "merger: git merge-base 실패(%s)", m.b[i].l)
		}
		prev, rc := out("rev-parse", "--verify", "refs/heads/"+m.br)
		if rc != 0 {
			m.defer_("후보 가지 조회 실패")
			return false
		}
		if inLog(m.mwt, m.lgf, "rebase", "-q", "--onto", m.br, mf, m.b[i].s) == 0 {
			got, rc := gitx.Out(m.mwt, os.Stderr, "rev-parse", "--verify", "HEAD")
			ap := apUnknown
			if rc == 0 {
				ap = m.applied(m.b[i], prev, got)
			}
			switch ap {
			case apUnknown:
				m.defer_("승인 구성원의 후보 트리 대조 실패")
				return false
			case apDiffer: // 앞 구성원까지의 후보로 되돌리고 이 구성원은 큐에 그대로
				if inLog(m.mwt, m.lgf, "checkout", "-q", "-f", m.br) != 0 {
					m.defer_("앞 구성원의 후보 작업 트리를 복구하지 못했다")
					return false
				}
				continue
			}
			if inLog(m.mwt, m.lgf, "checkout", "-q", "-B", m.br) != 0 {
				m.defer_("후보 가지 전환 실패")
				return false
			}
			k = append(k, i)
		} else {
			if inLog(m.mwt, m.lgf, "rebase", "--abort") != 0 || inLog(m.mwt, m.lgf, "checkout", "-q", "-f", m.br) != 0 {
				m.defer_("앞 구성원의 후보 작업 트리를 복구하지 못했다")
				return false
			}
			fmt.Fprintf(os.Stderr, "hc-lane merger: 묶음에서 뺌 — %s(앞 구성원 위에 얹으면 충돌, 큐에 그대로)\n", m.b[i].l)
		}
	}
	if len(k) < len(m.b) {
		// 하나만 남으면 혼자 착지와 같게 원본 제출 그대로 — 승인 착지는 HEAD가 제출 SHA(HERDR_LAND_SHA)여야 한다
		if len(k) == 1 && inLog(m.mwt, m.lgf, "checkout", "-q", "-B", m.br, m.C0) != 0 {
			m.defer_("후보 가지를 원본 제출로 되돌리지 못했다")
			return false
		}
		m.keep(k...)
		m.jwrite()
		m.exbuild()
	}
	return true
}

// runCandidate: 준비 훅(.claude/herdr-setup.sh — 실패는 일시 실패 75) 뒤 hc-lane land를 후보 작업 트리에서. 둘 다 새 프로세스 무리로,
// 출력은 로그 뒤에. 준비 훅의 코드 색인은 색인 도구를 PATH에서 빼 건너뛰게 한다(곧 지울 폴더를 색인하면 검증과 CPU를 다툰다)
func (m *M) runCandidate() int {
	lf, err := os.OpenFile(m.lgf, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 착지 로그를 열지 못했다: %v\n", err)
		return 75
	}
	defer lf.Close()
	pf, stop := m.stLandLocal() // 착지하는 동안 상태 한 장을 이어 씀(staterec.go)
	defer stop()
	if rc, _ := setupHook(m.mwt, m.top, m.base, lf, m.mj+"/child", false); rc != 0 {
		return rc
	}
	c := exec.Command(gSelf, "land", m.base)
	c.Dir, c.Stdout, c.Stderr = m.mwt, lf, lf
	c.Env = append(append(os.Environ(), "HERDR_MERGER=1", "HERDR_LAND_PHASE="+pf), m.landEnv(lf, m.ex, "HERDR_VERIFY_PARTS_FROM="+m.sparts)...)
	// 착지에도 상한(재감사 3바퀴 merger/R-3M07 — 검증·착지 훅이 끝나지 않으면 일꾼·상주가 사건 없이 끝없이 기다렸다). 넘으면 무리째 거두고
	// 124(모름 — 일시 실패, judge가 원격부터 본다). keep: 이 일꾼이 죽으면 상한을 걸 쪽이 없지만 그 착지는 끝까지 가게 둔다(다음 바퀴가
	// 기다리다 상한을 넘기면 쉰 바퀴로 센다 — childWait)
	rc, ran := proc.WaitFor(c, m.mj+"/child", landMax(), true)
	if !ran {
		return rcNotRun
	}
	return rc
}

// setupHook: 작업 트리 wt에 준비 훅이 있으면 돌린다(상한 HERDR_TIMEOUT_SETUP, 자식 기록 rec). 0 통과·없음 · 75 실패 · rcNotRun 못 띄움,
// 둘째 값은 훅 자신의 종료 코드. shell이면 그 기계의 셸 환경에서(워커 착지 — 추측 일꾼과 같은 shellCmd, hc 코어 PATH에는 npm·bun 등이 없다)
func setupHook(wt, top, base string, lf *os.File, rec string, shell bool) (int, int) {
	if _, err := os.Stat(wt + "/.claude/herdr-setup.sh"); err != nil {
		return 0, 0
	}
	var c *exec.Cmd
	if shell {
		c = shellCmd(wt, "sh", "-c", noIndexSh, "hc-merger", "bash", ".claude/herdr-setup.sh")
		c.Env = append(os.Environ(), "LANE_REPO="+top, "LANE_BASE="+base)
	} else {
		c = exec.Command("bash", ".claude/herdr-setup.sh")
		c.Dir = wt
		c.Env = append(os.Environ(), "PATH="+pathWithout("codebase-memory-mcp"), "LANE_REPO="+top, "LANE_BASE="+base)
	}
	c.Stdout, c.Stderr = lf, lf
	// 준비 훅에도 상한(E11·G1 — 의존성 설치가 망에서 멈추면 main에 쓰는 유일한 프로세스가 선다). 넘으면 무리째 거두고 큐를 둔 채 쉰다
	rc, ran := proc.WaitFor(c, rec, setupMax(), setupMax() <= 0)
	if !ran {
		return rcNotRun, 0
	}
	if rc != 0 {
		fmt.Fprintf(lf, "hc-lane merger: 준비 훅(.claude/herdr-setup.sh) 실패(종료 %d)\n", rc)
		return 75, rc
	}
	return 0, 0
}

// landEnv: 착지(hc-lane land)에 줄 환경(HERDR_MERGER 빼고) — ex는 HERDR_LAND_PUSH_EXTRA, parts는 통과 조각 「키=값」(병합기 기계는
// HERDR_VERIFY_PARTS_FROM, 워커는 HC_MERGER_LAND_SPEC — landon.go, 빈 값이면 안 넣음). 워커에는 이 값이 env 인자로 간다
func (m *M) landEnv(lf *os.File, ex, parts string) []string {
	// 혼자면 착지가 그 제출의 승인 patch-id를 대조한다. 묶음 HEAD는 어느 한 승인 변경도 아니라 묶음 전체에 대한 판정은 넘기지 않는다 —
	// 판정이 붙은 구성원은 filterBatch(approved)가 하나씩 확인해야만 묶음에 들었다
	v, p := m.bv, m.bp
	if len(m.b) > 1 {
		v, p = "", ""
		lease := false
		for _, e := range m.b {
			if e.v != "" {
				fmt.Fprintf(lf, "hc-lane merger: 묶음 구성원 승인 확인됨 — %s 제출 ID %s 판정 %s patch-id %s\n", e.l, e.d, e.v, e.p)
				lease = true
			}
		}
		// 승인 구성원의 결과 트리는 기준 m.B 위에서 확인했다(stack·applied) — 착지가 더 새 기준으로 다시 rebase하면 그 확인이 무효라
		// 기본 브랜치가 m.B일 때만 올린다(lease, 바뀌었으면 착지가 종료 6 → 다음 바퀴에 새 기준으로 다시 확인. R-H01)
		if lease {
			ex += " --force-with-lease=refs/heads/" + m.base + ":" + m.B
		}
	}
	env := []string{"HC_LANE=" + m.br, "HERDR_LAND_PUSH_EXTRA=" + strings.TrimSpace(ex),
		"HERDR_LAND_VERDICT=" + v, "HERDR_LAND_PATCHID=" + p, "HERDR_LAND_SHA=" + m.C0, "HERDR_SPEC_TREE=" + m.stree}
	if parts != "" {
		env = append(env, parts)
	}
	env = append(env, m.ownerEnv()...)
	if h := m.prHeads(); h != "" { // PR 거울의 머리 가지가 있는 구성원만(prhead.go) — 없으면 환경이 전과 같다
		env = append(env, "HERDR_LAND_PR_HEADS="+h)
	}
	return env
}

// rcNotRun: runCandidate가 준비 훅·착지를 띄우지 못했다(고아 기록·시작 시각을 확정 못 함, 실행 파일 없음 — 명령은 한 줄도 안 돎).
// 착지 실패가 아니라 모름: 제출을 반송하지 않고 이 바퀴를 쉰다(재감사 3바퀴 merger/R-3M02 — 옛 판은 127을 착지 실패로 읽어 반송했다)
const rcNotRun = -1

// ownerEnv: 착지가 내는 중간 사건(「land 실패」 — hc-lane report)의 주인. 묶음 구성원이 모두 같은 지휘면 제출 기록의 owner(실행이
// 모두 같으면 owner_run도), 아니면 빈 값 = 사건 주인 '-'(모든 지휘에게). 병합기 자신의 환경 값은 늘 덮는다
func (m *M) ownerEnv() []string {
	o, r := "", ""
	for i, e := range m.b {
		if i == 0 {
			o, r = e.ow, e.owr
		} else if e.ow != o {
			o = ""
		} else if e.owr != r {
			r = ""
		}
	}
	if o == "" {
		r = ""
	}
	return []string{"HC_LANE_OWNER=" + o, "HC_LANE_OWNER_RUN=" + r}
}

// setupMax: 준비 훅 상한 HERDR_TIMEOUT_SETUP(초, 기본 1800 — 큰 저장소의 의존성 설치·빌드). 0이면 상한 없음
func setupMax() time.Duration {
	return time.Duration(intSetting("HERDR_TIMEOUT_SETUP", os.Getenv("HERDR_TIMEOUT_SETUP"), 1800, 0)) * time.Second
}

// landMax: 착지(hc-lane land) 상한 HERDR_TIMEOUT_LAND(초, 기본 10800 — 검증 자리·착지 차례 기다림과 전체 검증을 넉넉히). 0이면 상한 없음
func landMax() time.Duration {
	return time.Duration(intSetting("HERDR_TIMEOUT_LAND", os.Getenv("HERDR_TIMEOUT_LAND"), 10800, 0)) * time.Second
}

// ponytail: 도구 이름 하나로 거름, 준비 훅이 다른 색인 길을 쓰면 그때 LANE_MERGER 같은 표시로
func pathWithout(tool string) string {
	var keep []string
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if st, err := os.Stat(filepath.Join(d, tool)); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			continue
		}
		keep = append(keep, d)
	}
	return strings.Join(keep, ":")
}

// mktempDir: <폴더>/c-XXXXXX(6글자 — 남은 빈 폴더를 wtclean이 이 꼴로 거둔다)
func mktempDir(parent string) (string, error) {
	const abc = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	for try := 0; try < 100; try++ {
		var r [6]byte
		if _, err := rand.Read(r[:]); err != nil {
			return "", err
		}
		for i := range r {
			r[i] = abc[int(r[i])%len(abc)]
		}
		p := parent + "/c-" + string(r[:])
		err := os.Mkdir(p, 0o700)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("임시 폴더 이름이 계속 겹친다: %s", parent)
}

// tail5: 로그 끝 다섯 줄을 화면에
func tail5(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > 5 {
			lines = lines[1:]
		}
	}
	for _, l := range lines {
		fmt.Println(l)
	}
}
