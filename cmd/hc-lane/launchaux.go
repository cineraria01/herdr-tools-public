package main

// 레인 띄우기 곁일: 실행 명령 틀 나누기·hc 출력 읽기·되돌리기·감시 띄우기·원격 공간 이름(셸 판 lib/hc-lane/start.sh와 같은 글)

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"herdr-tools/internal/oplog"
	"herdr-tools/internal/proc"
)

// rcRefuse: 띄우기의 입력 거절 — 다시 해도 같은 결과(가지·작업 트리가 이미 있음, 지시문 없음, 주차 가지 없음 등). hc-fill은 이 코드를
// 받으면 그 대기열 줄을 멈춰 두고 주인 지휘에 한 번 알린다(2026-11-24 fill-reject-hold — 「원격 브랜치가 이미 있다」로 2시간 18분
// 동안 278번 다시 띄웠다). 일시 실패(git·hc·연결·시간 넘김)는 지금처럼 1
const rcRefuse = 11

// refuse: 「hc-lane: <글>」 한 줄 뒤 종료 11(die와 같은 기록). 확정된 답 뒤에만 부른다 — 시간 넘김 바꿈(die의 75)이 없다
func refuse(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(os.Stderr, "hc-lane: %s\n", msg)
	oplog.Fatal("hc-lane", "", oplog.Die, rcRefuse, msg)
	exit(rcRefuse)
}

// shellWords: 설정 lane.run 틀을 셸 문법으로 나눈다(셸 판 eval "set -- $tpl" — 설정 파일은 믿는다)
func shellWords(tpl string) []string {
	var b bytes.Buffer
	c := exec.Command("bash", "-c", `eval "set -- $1" || exit 1; printf '%s\0' "$@"`, "hc-lane", tpl)
	c.Stdout = &b
	if c.Run() != nil {
		die("설정 lane.run을 셸 문법으로 나누지 못했다: %s", tpl)
	}
	s := strings.TrimSuffix(b.String(), "\x00")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}

// runRefused: hc run 오류가 JSON 하나이고 kind가 name_taken·spawn_failed(아무것도 안 띄움이 확실)
func runRefused(out string) bool {
	d := json.NewDecoder(strings.NewReader(out))
	var vs []map[string]any
	for {
		var v map[string]any
		err := d.Decode(&v)
		if err == io.EOF {
			break
		}
		if err != nil {
			return false
		}
		vs = append(vs, v)
	}
	if len(vs) != 1 {
		return false
	}
	e, _ := vs[0]["error"].(map[string]any)
	k, _ := e["kind"].(string)
	return k == "name_taken" || k == "spawn_failed"
}

// firstAddr: hc run 표준 출력이 JSON 객체 정확히 하나이고 맨 위 address가 비어 있지 않은 문자열일 때 그 주소, 아니면 ""(잘린 응답에서
// 주소를 뽑아 등록하지 않는다 — 재감사 R-17)
func firstAddr(out string) string {
	m := jone(out)
	if m == nil {
		return ""
	}
	a, ok := m["address"].(string)
	if !ok || strings.ContainsAny(a, " \t\n") {
		return ""
	}
	return a
}

// rollback: 띄우기 전에 실패하면 방금 만든 작업 트리·가지를 지운다 — 고유 커밋이 없을 때만. 돌려준 값은 die에 붙일 한 줄
func rollback() string {
	k, r := xoutQ("git", "-C", repo, "rev-list", "--count", bref+"..refs/heads/"+branch)
	if r != 0 {
		k = "?"
	}
	if k == "0" && xrun(nil, nil, "git", "-C", repo, "worktree", "remove", "--force", wt) == 0 {
		if xrun(nil, nil, "git", "-C", repo, "branch", "-q", "-d", branch) == 0 {
			return fmt.Sprintf("방금 만든 워크트리 %s·브랜치 %s 는 지웠다(고유 커밋 없음). 원인을 고친 뒤 같은 명령을 다시 돌린다", wt, branch)
		}
		return fmt.Sprintf("방금 만든 워크트리 %s 는 지웠고 브랜치 %s 는 못 지웠다. 지우려면: %sgit -C %s branch -d %s", wt, branch, rx, q(repo), branch)
	}
	lg := ""
	if logPath != "" {
		lg = ", 기록 " + logPath
	}
	return fmt.Sprintf("워크트리 %s%s 는 남겨 둠(고유 커밋 %s개 또는 지우기 실패)%s. 지우려면: %s", wt, onNote(), k, lg, undo)
}

// autoWatch: 레인·감사를 띄운 지휘에게 도는 watch가 없으면 이 기계에 하나 띄운다
func autoWatch() {
	co := os.Getenv("HC_LANE_COORD")
	if co == "" {
		co = os.Getenv("HC_TERMINAL_ID")
	}
	if co == "" {
		return
	}
	watchFor(co)
}

// watchFor: 그 지휘(지휘 확인·에이전트 확인을 통과한 것만)의 watch가 돌고 있지 않으면 이 기계에 띄운다 — autoWatch와
// hc-lane coord --watch(hc-fill 바퀴). 0 돈다·띄움, 1 지휘 없음(띄우지 않음), 2 지휘 모름, 3 띄우기 실패
func watchFor(co string) int {
	wk := ckey(co)
	// 돌고 있으면 지휘 확인(hc get) 없이 — 돌고 있는 watch는 바퀴마다 스스로 지휘 확인을 한다(hc-fill이 바퀴마다 부른다, 감사 낮음 2)
	if _, alive := watchAlive(evdir() + "/watch." + wk); alive {
		return 0
	}
	if _, why, r := coordCheck(co); r != 0 {
		errf("hc-lane: 지휘 없음 — %s. 감시(watch)를 띄우지 않음(사건은 events에 남는다)\n", why)
		return r
	}
	wn := strings.ToLower(wk)
	if len(wn) > 21 {
		wn = wn[:21]
	}
	wn = "lane-watch-" + wn
	home := os.Getenv("HOME")
	if dry {
		fmt.Printf("[dry-run] hc run --bg --no-notify --rm --name %s --label hc-lane-watch=1 --cwd %s %s watch --all --to %s\n", wn, q(home), q(selfPath), co)
		return 0
	}
	if proc.Run(exec.Command("hc", "run", "--bg", "--json", "--no-notify", "--rm", "--name", wn, "--label", "hc-lane-watch=1", "--cwd", home, selfPath, "watch", "--all", "--to", co)) == nil {
		fmt.Printf("hc-lane: 감시 %s 띄움(지휘 %s 에게 레인 사건을 hc send로 밀어 줌)\n", wn, co)
		return 0
	}
	errf("hc-lane: 감시 %s 를 띄우지 못했다(이미 그 이름이 살아 있으면 괜찮다) — 지휘는 hc-lane wait --all로 볼 것\n", wn)
	return 3
}

func sleepPoll() { sleepFor(poll) }

// uiLimit: 창 조회·이름 붙이기 한 번의 상한 — 원격 상한(HERDR_TIMEOUT_REMOTE)과 10초 가운데 짧은 쪽. 결과가 부수적인 단계라
// 상한 없음(0)이어도 10초(재감사 2바퀴 P7·lane/R-218)
func uiLimit() time.Duration {
	d := proc.Limit([]string{"ssh"}) // = HERDR_TIMEOUT_REMOTE
	if d <= 0 || d > 10*time.Second {
		d = 10 * time.Second
	}
	return d
}

// showRemote: 띄운 뒤 그 공간이 이름 없이 생겼으면 기계 이름을 붙인다(창이 터미널을 보는 데 몇 초 — poll초씩 5번까지).
// 조회·이름 붙이기는 uiLimit 안에서만 — 결과가 모름이어도 이미 받은 터미널 주소(addr)·시작 결과는 그대로다(부른 쪽이 돌려준다)
func showRemote() {
	if hsp == "" || addr == "" {
		return
	}
	id := on + "/" + hsp
	for i := 0; ; {
		ob, err := proc.OutputMax(exec.Command("herdr-app", "ls"), uiLimit())
		out, r := trim(string(ob)), code(err)
		if r != 0 {
			errf("hc-lane: Herdr 창이 없어 %s 공간 이름은 못 붙였다(창을 열면 %s 가 보인다)\n", on, addr)
			return
		}
		var v struct {
			Spaces []struct {
				ID   string  `json:"id"`
				Name *string `json:"name"`
			} `json:"spaces"`
		}
		found, nm := false, ""
		if json.Unmarshal([]byte(out), &v) == nil {
			for _, s := range v.Spaces {
				if s.ID == id {
					found = true
					if s.Name != nil {
						nm = *s.Name
					}
					break
				}
			}
		}
		if found {
			if nm == on {
				return
			}
			if proc.RunMax(exec.Command("herdr-app", "do", "space.rename", "--id", id, "--name", on), uiLimit()) == nil {
				return
			}
			errf("hc-lane: Herdr 창의 %s 공간 이름을 붙이지 못했다(herdr-app do space.rename --id %s --name %s)\n", on, id, on)
			return
		}
		i++
		if i >= 5 {
			errf("hc-lane: Herdr 창에 %s 공간(%s)이 아직 안 보인다 — 창을 확인\n", addr, id)
			return
		}
		sleepPoll()
	}
}
