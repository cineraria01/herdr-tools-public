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

	"herdr-tools/internal/proc"
)

// stack: 묶음 뒤쪽 구성원을 앞 구성원 위에 차례로 얹는다(그 제출이 기준에서 갈라진 뒤의 커밋만). 충돌하는 구성원은 이번 묶음에서
// 빼고 큐에 그대로. 착지를 부를 수 있으면 true
func (m *M) stack() bool {
	// 뒤 제출이 더 새 기준에서 되돌린 변경을 옛 트리에 얹으면 빈 커밋으로 사라진다 — 첫 후보를 이번 기준으로 맞춘다.
	// 충돌하면 원래 제출 하나로 착지의 rerere 절차를 탄다
	if inLog(m.mwt, m.lgf, "rebase", "-q", "--reapply-cherry-picks", m.B) != 0 {
		m.writeCap(1)
		m.fin()
		return false
	}
	k := []int{0}
	for i := 1; i < len(m.b); i++ {
		mf, rc := out("merge-base", m.b[i].s, m.B)
		if rc != 0 {
			dieGit(rc, "merger: git merge-base 실패(%s)", m.b[i].l)
		}
		if inLog(m.mwt, m.lgf, "rebase", "-q", "--onto", m.br, mf, m.b[i].s) == 0 {
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
	if _, err := os.Stat(m.mwt + "/.claude/herdr-setup.sh"); err == nil {
		c := exec.Command("bash", ".claude/herdr-setup.sh")
		c.Dir, c.Stdout, c.Stderr = m.mwt, lf, lf
		c.Env = append(os.Environ(), "PATH="+pathWithout("codebase-memory-mcp"), "LANE_REPO="+m.top, "LANE_BASE="+m.base)
		// 준비 훅에도 상한(E11·G1 — 의존성 설치가 망에서 멈추면 main에 쓰는 유일한 프로세스가 선다). 넘으면 무리째 거두고 큐를 둔 채 쉰다
		if rc := proc.WaitMax(c, m.mj+"/child", setupMax()); rc != 0 {
			fmt.Fprintf(lf, "hc-lane merger: 준비 훅(.claude/herdr-setup.sh) 실패(종료 %d)\n", rc)
			return 75
		}
	}
	// 혼자면 착지가 그 제출의 승인 patch-id를 대조한다. 묶음 HEAD는 어느 한 승인 변경도 아니라 묶음 전체에 대한 판정은 넘기지 않는다 —
	// 판정이 붙은 구성원은 filterBatch(approved)가 하나씩 확인해야만 묶음에 들었다
	v, p := m.bv, m.bp
	if len(m.b) > 1 {
		v, p = "", ""
		for _, e := range m.b {
			if e.v != "" {
				fmt.Fprintf(lf, "hc-lane merger: 묶음 구성원 승인 확인됨 — %s 제출 ID %s 판정 %s patch-id %s\n", e.l, e.d, e.v, e.p)
			}
		}
	}
	c := exec.Command(gSelf, "land", m.base)
	c.Dir, c.Stdout, c.Stderr = m.mwt, lf, lf
	c.Env = append(os.Environ(), "HERDR_MERGER=1", "HC_LANE="+m.br, "HERDR_LAND_PUSH_EXTRA="+m.ex,
		"HERDR_LAND_VERDICT="+v, "HERDR_LAND_PATCHID="+p, "HERDR_LAND_SHA="+m.C0, "HERDR_SPEC_TREE="+m.stree)
	return proc.Wait(c, m.mj+"/child")
}

// setupMax: 준비 훅 상한 HERDR_TIMEOUT_SETUP(초, 기본 1800 — 큰 저장소의 의존성 설치·빌드). 0이면 상한 없음
func setupMax() time.Duration {
	return time.Duration(intSetting("HERDR_TIMEOUT_SETUP", os.Getenv("HERDR_TIMEOUT_SETUP"), 1800, 0)) * time.Second
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
