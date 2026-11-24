// hc-job-server: 자동 작업(Claude 루프)을 Herdr core 터미널 하나로 지키는 감독(셸 판 2026-11-02, Go 판 2026-11-24 hc-job-stale-restart)
//
//	hc-job-server <이름> <job 라벨> <작업 폴더> <재개 스크립트>
//
// launchd(KeepAlive)가 앞에서 돌린다. 틱(HC_JOB_TICK, 60초)마다 `hc get <이름> --json`으로:
//
//	live     → 채택: 아무것도 하지 않는다(이 감독을 kickstart해도 루프 pid·hc 주소 그대로).
//	           단 멈춘 루프 — status.state가 done·idle이고 status_ms가 HC_JOB_STALE초(기본 5400, 0이면 안 봄) 넘게 그대로 — 는
//	           hc kill <주소>(다음 틱에 finished로 같은 세션을 다시 띄운다). Claude /loop가 최대 시간(약 7일)에 닿아 ScheduleWakeup이
//	           거절되면 Claude는 프롬프트에 살아 있는데 회차가 없다(2026-11-23 11시간). ScheduleWakeup 상한이 3600초라 도는 루프는
//	           그 안에 다시 working이 된다. working·blocked·error, status 없음(Claude 아님·권한 대기 중인 hc-loop-start)은 건드리지 않는다.
//	finished → 끝난 기록만 hc rm(세션 로그는 그대로) 뒤 absent와 같이.
//	absent   → hc-loop-start(HC_JOB_LOOP_START, 기본 ~/.local/bin/hc-loop-start)로 같은 세션 재개 스크립트를 띄운다.
//	error    → 코어에 못 붙음: 띄우지 않는다(없음으로 오판하면 중복이 생긴다).
//
// 같은 세션 ID(재개 스크립트에서 처음 나오는 `--resume <ID>`)의 Claude가 hc 밖에서 돌고 있으면 띄우지 않는다.
// 다시 띄우기는 HC_JOB_GAP(기본 600초)에 한 번까지. hc는 HC_JOB_HC(기본 ~/.local/bin/hc), HC_JOB_ONCE면 한 틱만.
// 루프를 일부러 멈추려면: launchctl bootout gui/$(id -u)/<launchd 라벨> 뒤 hc kill <이름>.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"herdr-tools/internal/proc"
	"herdr-tools/internal/selfexec"
	"herdr-tools/internal/version"
)

const me = "hc-job-server"

func main() { os.Exit(run(os.Args[1:])) }

type job struct {
	name, label, dir, script string
	hc, loopStart, home      string
	sid                      string
	gap, tick, stale         int64 // 초
	last                     int64 // 마지막으로 띄운 때(유닉스 초)
}

var resumeRe = regexp.MustCompile(`--resume ([0-9a-f-]*)`)

func run(args []string) int {
	proc.Who = me
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println(version.String(me))
		return 0
	}
	if len(args) != 4 || args[0] == "" || args[1] == "" || args[2] == "" || args[3] == "" {
		fmt.Fprintf(os.Stderr, "쓰는 법: %s <이름> <job 라벨> <작업 폴더> <재개 스크립트>\n", me)
		return 2
	}
	home := os.Getenv("HOME")
	j := &job{name: args[0], label: args[1], dir: args[2], script: args[3], home: home,
		hc:        envOr("HC_JOB_HC", home+"/.local/bin/hc"),
		loopStart: envOr("HC_JOB_LOOP_START", home+"/.local/bin/hc-loop-start"),
		gap:       envInt("HC_JOB_GAP", 600), stale: envInt("HC_JOB_STALE", 5400), tick: envInt("HC_JOB_TICK", 60), last: envInt("HC_JOB_LAST", 0)}
	if b, err := os.ReadFile(j.script); err == nil {
		if m := resumeRe.FindSubmatch(b); m != nil {
			j.sid = string(m[1])
		}
	}
	if os.Getenv("HC_JOB_ONCE") != "" {
		j.step()
		return 0
	}
	sid := j.sid
	if sid == "" {
		sid = "없음"
	}
	j.log("감독 시작(세션 %s)", sid)
	self := selfexec.New(me, func(l string) { j.log("%s", l) })
	for {
		self.Check() // 틱 사이 — 새 판이 깔렸으면 그 판으로 이어 돈다(같은 pid라 launchd KeepAlive가 다시 띄우지 않는다)
		j.step()
		time.Sleep(time.Duration(j.tick) * time.Second)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// envInt: 음 아닌 정수(못 읽으면 경고하고 기본값)
func envInt(k string, def int64) int64 {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			return n
		}
		fmt.Fprintf(os.Stderr, "%s: 경고 — %s=%q 를 읽지 못해 기본 %d\n", me, k, v, def)
	}
	return def
}

func (j *job) log(f string, a ...any) {
	fmt.Printf("%s %s: %s\n", time.Now().Format("2006-01-02 15:04:05"), j.name, fmt.Sprintf(f, a...))
}

// got: hc get --json에서 쓰는 칸
type got struct {
	Address string `json:"address"`
	State   string `json:"state"`
	Status  *struct {
		State string `json:"state"`
	} `json:"status"`
	StatusMs int64 `json:"status_ms"`
}

// stuck: 멈춘 루프면 그 까닭(아니면 "")
func (j *job) stuck(g got) string {
	if j.stale == 0 || g.Status == nil || (g.Status.State != "done" && g.Status.State != "idle") || g.StatusMs <= 0 {
		return ""
	}
	if age := time.Now().UnixMilli()/1000 - g.StatusMs/1000; age > j.stale {
		return fmt.Sprintf("status %s가 %d초째 그대로(HC_JOB_STALE %d)", g.Status.State, age, j.stale)
	}
	return ""
}

func (j *job) step() {
	out, err := proc.Output(exec.Command(j.hc, "get", j.name, "--json"))
	st, g := "", got{}
	switch rc := proc.Code(err); {
	case rc == 3:
		st = "absent"
	case rc != 0 || json.Unmarshal(out, &g) != nil:
		st = "error"
	default:
		st = g.State
	}
	switch st {
	case "live":
		why := j.stuck(g)
		if why == "" || g.Address == "" {
			return
		}
		if time.Now().Unix()-j.last < j.gap {
			return // 다시 띄울 수 없는 동안은 끄지 않는다
		}
		if err := proc.Run(exec.Command(j.hc, "kill", g.Address)); err != nil {
			j.log("멈춘 루프(%s) → hc kill %s 실패(종료 %d), 다음 틱에 다시 확인", why, g.Address, proc.Code(err))
		} else {
			j.log("멈춘 루프(%s) → hc kill %s, 다음 틱에 다시 띄움", why, g.Address)
		}
		return
	case "finished":
		proc.Run(exec.Command(j.hc, "rm", g.Address)) //nolint:errcheck // 지울 것이 없어도 다음 hc run이 같은 이름으로 뜬다(셸 판과 같게)
	case "absent":
	default:
		return // error·모르는 상태: 띄우지 않는다
	}
	if j.sid != "" && proc.Run(exec.Command("/usr/bin/pgrep", "-f", "--", "--resume "+j.sid)) == nil {
		j.log("hc 밖에서 같은 세션(%s)이 돌고 있어 띄우지 않음", j.sid)
		return
	}
	now := time.Now().Unix()
	if now-j.last < j.gap {
		return
	}
	c := exec.Command(j.hc, "run", "--bg", "--json", "--outlive-parent", "--no-parent", "--name", j.name, "--label", "job="+j.label,
		"--cwd", j.home+"/.claude-tasks", "--", "/bin/bash", j.loopStart, j.dir, j.script)
	if err := proc.Run(c); err == nil {
		j.last = now
		j.log("%s → 다시 띄움(hc-loop-start %s %s)", st, j.dir, j.script)
	} else {
		j.log("%s → hc run 실패, 다음 틱에 다시 확인", st)
	}
}
