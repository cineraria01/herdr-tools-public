package main

// 착지 뒤 명령(add --on-land <이름>, 레인 go-land-build 2026-11-22 — 사용자가 「고쳐지면 바로 개발 앱 빌드해서 보여줘」를 하루 다섯 번,
// 지휘가 매번 fetch·ff·빌드 감시를 손으로 걸었다). 설정 줄 「on-land <이름> <프로젝트> <명령…>」의 명령을 그 레인이 착지하면
// 이 기계(hc-fill이 도는 지휘 기계)에서 한 번 돌리고 결과를 주인 지휘에 사건 한 줄로.
//
//   DATA/onland.tsv          「레인 이름 프로젝트 주인 접수초」 — add가 쓰고 바퀴(onlandTick)가 착지를 보면 지운다
//   DATA/onland/<이름>.pending  「레인 주인 sha」 줄(착지마다 하나) — 실행기가 한 번에 걷는다(돌고 있는 동안 쌓인 것은 끝난 뒤 한 번 더)
//   DATA/onland/<이름>.running  실행기가 걷은 묶음(pending을 rename) — 명령과 사건이 끝난 뒤에만 지운다. 실행기가 그 사이 죽으면
//                               다음 실행기가 이 묶음부터 다시 돌린다. 바퀴는 남은 pending·running마다 실행기를 띄운다(재감사 2바퀴 fill R-212)
//   DATA/onland/<이름>.lock    같은 이름 명령은 하나씩(flock)
//   DATA/onland/<이름>.child    도는 명령의 「pid 시작 시각」(proc 자식 기록) — 실행기가 죽어 잠금이 풀려도 다음 실행기가 앞 명령이
//                               끝났는지 본다(재감사 2바퀴 fill R-213)
//   DATA/onland/<이름>-<시각>.log 명령 출력 · <이름>.out 실행기 자신의 결과 줄·오류

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/lockx"
	"herdr-tools/internal/proc"
)

const (
	onlandKeep = 14 * 24 * 3600 // ponytail: 착지 못 본 줄(rm·이름만 남음)은 14일 뒤 버린다 — rm이 같이 지우게 할 일이 생기면 그때
	onlandMax  = time.Hour      // 명령 상한 — 넘으면 무리째 끄고 실패(같은 이름 다음 착지가 막히지 않게)
)

func onlandDir() string { return DATA + "/onland" }

// onlandPoll: 앞 실행기의 명령이 끝나기를 볼 때 쉬는 틈(시험이 줄인다)
var onlandPoll = time.Second

// onlandPrevDone: 앞 실행기(죽어 잠금을 놓은 것)가 띄운 같은 이름 명령이 끝났는가 — 기록한 그 프로세스가 돌거나, 우두머리가 끝났는데
// 무리가 남았거나, 모르면(proc.ChildBusy) 명령 상한만큼 기다린다. 그래도 남으면 거짓(pending은 그대로 — 다음 착지 때 다시)
func onlandPrevDone(name string) bool {
	rec := onlandDir() + "/" + name + ".child"
	for end := time.Now().Add(onlandMax); proc.ChildBusy(rec, proc.Grace); time.Sleep(onlandPoll) {
		if !time.Now().Before(end) {
			return false
		}
	}
	return true
}

// onlandConf <이름>: 설정 on-land 줄의 프로젝트·명령(없으면 빈 값)
func onlandConf(name string) (string, string) {
	for _, l := range confLines {
		if f := fields(l); len(f) >= 4 && f[0] == "on-land" && f[1] == name {
			return f[2], restAfter(l, 3)
		}
	}
	return "", ""
}

// onlandCheck: add --on-land 검사. 이름은 사건 이름 on-land-<이름>이 레인 이름 규칙(32자)에 들게
func onlandCheck(name, prj string) {
	if !okname(name) || len(name) > 24 {
		die("--on-land 이름은 a-z 0-9 - _ 24자까지: %s", name)
	}
	p, c := onlandConf(name)
	if c == "" {
		die("--on-land: 설정에 「on-land %s <프로젝트> <명령…>」 줄이 없다: %s", name, CONF)
	}
	if p != prj {
		die("--on-land %s 는 프로젝트 %s 용이다(이 레인은 %s)", name, p, prj)
	}
	if repoOf(p, LOCAL) == "" {
		die("--on-land %s: 설정에 %s 의 %s repo 줄이 없다(명령을 돌릴 폴더)", name, p, LOCAL)
	}
}

// onlandPut: 대기열 잠금 안에서 그 레인 줄을 새로(같은 레인 옛 줄은 바꿈)
func onlandPut(lane, name, prj, owner string) {
	f := DATA + "/onland.tsv"
	var out []Row
	for _, r := range readRows(f) {
		if r.F(1) != lane {
			out = append(out, r)
		}
	}
	out = append(out, Row{lane, name, prj, dash(owner), strconv.FormatInt(nowUnix(), 10)})
	if err := writeAtomic(f, rowsText(out)); err != nil {
		die("착지 뒤 명령 기록을 못 씀: %v", err)
	}
}

// onlandHit <레인> <접수초>: FR·FB(fetchProj 뒤)에서 접수 뒤의 착지 — 제목 머리가 그 레인인 커밋(커밋 시각이 접수 뒤) 또는
// 병합기 표식 refs/hc-landed/<레인>/<초>. 0 착지(그 sha) · 1 아직 · 2 모름
func onlandHit(lane string, since int64) (string, int) {
	o, rc := run("git", "-C", FR, "log", FB, "--format=%H %ct %s", fmt.Sprintf("--since=%d", since-3600))
	if rc != 0 {
		return "", 2
	}
	re := regexp.MustCompile(`^([a-z]+(\([^)]*\))?!?: *)?` + regexp.QuoteMeta(lane) + `([^a-z0-9_-]|$)`)
	for _, l := range lines(o) {
		f := strings.SplitN(l, " ", 3)
		if len(f) == 3 && int64(num(f[1])) >= since && re.MatchString(f[2]) {
			return f[0], 0
		}
	}
	switch lmark(lane, strconv.FormatInt(since, 10)) {
	case 0:
		return LMS, 0
	case 2:
		return "", 2
	}
	return "", 1
}

// onlandLeft: 걷지 못한 차례(남은 pending·running)가 있는 이름들 — 실행기를 못 띄웠거나 실행기가 죽은 뒤(fill R-212).
// 폴더를 못 읽으면(없음 말고) 빈 목록(모름 — 다음 바퀴에)
func onlandLeft() []string {
	es, err := os.ReadDir(onlandDir())
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			say("착지 뒤 명령: 차례 폴더를 못 읽음(%v) — 다음 바퀴에", err)
		}
		return nil
	}
	var out []string
	for _, e := range es {
		for _, ext := range []string{".pending", ".running"} {
			if b, ok := strings.CutSuffix(e.Name(), ext); ok && okname(b) && nonEmpty(onlandDir()+"/"+e.Name()) {
				out = append(out, b)
			}
		}
	}
	return out
}

// onlandTick: 바퀴마다 — 착지한 줄을 그 이름 pending에 넣고 실행기(<SELF> _onland <이름>)를 배경으로 띄운다. 남은 차례도 다시 띄운다
// (실행기가 이미 돌면 새 실행기는 잠금을 못 잡아 바로 끝 — 도는 쪽이 걷는다)
func onlandTick() {
	fire := map[string]bool{}
	if !dry {
		for _, n := range onlandLeft() {
			fire[n] = true
		}
	}
	defer func() {
		for name := range fire {
			onlandSpawn(name)
		}
	}()
	onlandEvery(fire) // 설정 land-every — 레인마다 --on-land 없이 그 프로젝트 착지마다(onland_every.go)
	f := DATA + "/onland.tsv"
	rows := readRows(f)
	if len(rows) == 0 {
		return
	}
	now := nowUnix()
	drop := map[string]bool{} // 「레인 접수초」
	for _, r := range rows {
		lane, name, p, own, ts := r.F(1), r.F(2), r.F(3), r.F(4), int64(num(r.F(5)))
		if now-ts > onlandKeep {
			say("착지 뒤 %s: %s 가 %d일 넘게 착지하지 않아 버림", name, lane, onlandKeep/86400)
			drop[lane+" "+r.F(5)] = true
			continue
		}
		if fetchProj(p) != 0 {
			say("착지 뒤 %s ← %s: 판정 못 함(%s fetch·repo 줄) — 다음 바퀴에", name, lane, p)
			continue
		}
		sha, rc := onlandHit(lane, ts)
		if rc == 2 {
			say("착지 뒤 %s ← %s: 판정 못 함(커밋·착지 표식 조회 실패) — 다음 바퀴에", name, lane)
		}
		if rc != 0 {
			continue
		}
		if dry {
			say("%s착지 뒤 %s 를 돌릴 %s (%s)", P, name, lane, head(sha, 8))
			continue
		}
		if err := onlandPend(name, lane, own, sha); err != nil {
			say("착지 뒤 %s ← %s: 차례를 못 적음(%v) — 다음 바퀴에", name, lane, err)
			continue
		}
		say("착지 뒤 %s ← %s 착지 %s — 명령 차례에 넣음", name, lane, head(sha, 8))
		drop[lane+" "+r.F(5)], fire[name] = true, true
	}
	if len(drop) > 0 && !dry {
		if withQ(func() {
			var keep []Row
			for _, r := range readRows(f) {
				if !drop[r.F(1)+" "+r.F(5)] {
					keep = append(keep, r)
				}
			}
			if err := writeAtomic(f, rowsText(keep)); err != nil {
				say("착지 뒤 명령 기록을 못 고침(%v) — 다음 바퀴에 같은 착지를 또 넣을 수 있다(실행기가 하나로 합친다)", err)
			}
		}) != 0 {
			say("착지 뒤 명령 기록: 대기열 잠금을 못 잡음 — 다음 바퀴에")
		}
	}
}

// onlandPend: pending에 한 줄(짧은 잠금 — 실행기가 걷는 중에 끼어 잃지 않게)
func onlandPend(name, lane, own, sha string) error {
	d := onlandDir()
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	l, err := lockx.Lock(d + "/" + name + ".plock")
	if err != nil {
		return err
	}
	defer l.Close()
	return appendLine(d+"/"+name+".pending", strings.Join([]string{lane, dash(own), sha}, "\t"))
}

// onlandTake: 다음 묶음 — 앞 실행기가 끝내지 못한 running이 있으면 그것, 없으면 pending을 running으로 옮겨(rename) 그 줄들.
// 묶음은 onlandDone이 지운다. 읽기 오류(없음 말고)는 오류(모름 — 아무것도 옮기지 않는다)
func onlandTake(name string) ([]Row, error) {
	d := onlandDir()
	l, err := lockx.Lock(d + "/" + name + ".plock")
	if err != nil {
		return nil, err
	}
	defer l.Close()
	r := d + "/" + name + ".running"
	for moved := false; ; moved = true {
		b, ok, err := atomic.Read(r)
		if err != nil {
			return nil, err
		}
		if ok {
			var rs []Row
			for _, l := range lines(string(b)) {
				rs = append(rs, splitRow(l))
			}
			if len(rs) > 0 {
				return rs, nil
			}
			if err := os.Remove(r); err != nil { // 빈 묶음
				return nil, err
			}
		}
		if moved {
			return nil, nil
		}
		if err := os.Rename(d+"/"+name+".pending", r); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, nil
			}
			return nil, err
		}
		if err := atomic.SyncDir(d); err != nil {
			return nil, err
		}
	}
}

// onlandDone: 걷은 묶음의 명령·사건이 끝났다 — running을 지운다
func onlandDone(name string) error {
	d := onlandDir()
	l, err := lockx.Lock(d + "/" + name + ".plock")
	if err != nil {
		return err
	}
	defer l.Close()
	if err := os.Remove(d + "/" + name + ".running"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func onlandSpawn(name string) {
	out, err := os.OpenFile(onlandDir()+"/"+name+".out", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		say("착지 뒤 %s: 실행기를 못 띄움(%v) — pending은 남아 다음 착지 때 함께", name, err)
		return
	}
	defer out.Close()
	c := exec.Command(SELF, "_onland", name)
	c.Stdout, c.Stderr = out, out // 실행기 자신의 결과 줄·사건 실패(명령 출력은 따로 <이름>-<시각>.log)
	proc.Group(c)                 // 바퀴와 따로(빌드는 바퀴보다 길다)
	if err := c.Start(); err != nil {
		say("착지 뒤 %s: 실행기를 못 띄움(%v) — pending은 남아 다음 착지 때 함께", name, err)
		return
	}
	pid := c.Process.Pid
	c.Process.Release()
	say("착지 뒤 %s: 실행기 띄움(pid %d) — 로그 %s/%s-*.log · 실행기 %s/%s.out", name, pid, onlandDir(), name, onlandDir(), name)
}

// onlandRun <이름>: 실행기 — 잠금을 쥐고 pending이 빌 때까지 한 번씩(쌓인 착지는 한 번으로). 이미 도는 실행기가 있으면 그쪽이 걷는다.
// 잠금을 놓은 뒤에도 다시 본다(놓기 직전에 들어온 줄을 잃지 않게). 종료 0 · 1 잠금 오류 · 75 앞 실행기의 명령이 아직 돎(모름)
func onlandRun(name string) int {
	d := onlandDir()
	if err := os.MkdirAll(d, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		return 1
	}
	for {
		f, busy, err := lockx.TryLock(d + "/" + name + ".lock")
		if err != nil {
			fmt.Fprintf(os.Stderr, "hc-fill: 착지 뒤 %s 잠금: %v\n", name, err)
			return 1
		}
		if busy {
			return 0
		}
		// 잠금은 실행기가 죽으면 풀리지만 그 명령은 살아 있을 수 있다 — 같은 이름을 겹쳐 돌리지 않게 앞 명령이 끝난 뒤에만(fill R-213)
		if !onlandPrevDone(name) {
			fmt.Fprintf(os.Stderr, "hc-fill: 착지 뒤 %s: 앞 실행기의 명령이 아직 돈다(또는 모름) — pending은 남아 다음 착지 때 함께\n", name)
			f.Close()
			return 75
		}
		for {
			rs, err := onlandTake(name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "hc-fill: 착지 뒤 %s 차례 읽기: %v\n", name, err)
				f.Close()
				return 1
			}
			if len(rs) == 0 {
				break
			}
			onlandOnce(name, rs)
			if err := onlandDone(name); err != nil {
				fmt.Fprintf(os.Stderr, "hc-fill: 착지 뒤 %s 묶음을 끝냄으로 못 적음(%v) — 다음 실행기가 한 번 더 돌릴 수 있다\n", name, err)
				f.Close()
				return 1
			}
		}
		f.Close()
		if !nonEmpty(d+"/"+name+".pending") && !nonEmpty(d+"/"+name+".running") {
			return 0
		}
	}
}

// onlandOnce: 명령 한 번(설정 repo 폴더에서 sh -c) → 주인마다 사건 한 줄(성공 중간 · 실패 막힘)
func onlandOnce(name string, rs []Row) {
	var lanes, owners []string
	sha := ""
	for _, r := range rs {
		lanes, owners, sha = append(lanes, r.F(1)), append(owners, r.F(2)), r.F(3)
	}
	lanes, owners = uniq(lanes), uniq(owners)
	p, cmdline := onlandConf(name)
	repo := repoOf(p, LOCAL)
	lg := fmt.Sprintf("%s/%s-%s.log", onlandDir(), name, time.Now().Format("20060102-150405"))
	rc, why := 0, ""
	switch {
	case cmdline == "":
		rc, why = 1, "설정에 on-land "+name+" 줄이 없어짐"
	case repo == "":
		rc, why = 1, "설정에 "+p+" 의 "+LOCAL+" repo 줄이 없어짐"
	default:
		out, err := os.OpenFile(lg, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			rc, why = 1, "로그를 못 엶: "+err.Error()
			break
		}
		fmt.Fprintf(out, "--- %s %s ← %s (%s)\n$ %s\n", stampLog(), name, strings.Join(lanes, ","), head(sha, 8), cmdline)
		c := exec.Command("sh", "-c", cmdline)
		c.Dir = repo
		c.Stdout, c.Stderr = out, out
		c.Env = append(os.Environ(), "HC_ONLAND_NAME="+name, "HC_ONLAND_LANES="+strings.Join(lanes, ","), "HC_ONLAND_SHA="+sha)
		rc = proc.WaitMax(c, onlandDir()+"/"+name+".child", onlandMax) // 명령 무리를 자식 기록에(다음 실행기가 본다)
		fmt.Fprintf(out, "--- 종료 %d\n", rc)
		out.Close()
		if rc == proc.TimeoutCode {
			why = fmt.Sprintf("상한 %s 넘김", onlandMax)
		}
	}
	tail := "" // 명령의 끝 줄을 사건 글에(예: herdr-sync --landed의 「배포할 것 …」)
	if b, err := os.ReadFile(lg); err == nil {
		if t := onlandTail(b); t != "" {
			tail = ": " + t
		}
	}
	st, msg := "중간", fmt.Sprintf("착지 뒤 %s 성공%s — %s 착지(%s) · 로그 %s", name, tail, strings.Join(lanes, ","), head(sha, 8), lg)
	if rc != 0 {
		if why == "" {
			why = "종료 " + strconv.Itoa(rc)
		}
		st, msg = "막힘", fmt.Sprintf("착지 뒤 %s 실패(%s)%s — %s 착지(%s) · 로그 %s", name, why, tail, strings.Join(lanes, ","), head(sha, 8), lg)
	}
	fmt.Println(msg)
	for _, o := range owners {
		to := o
		if to == "-" {
			to = ""
		}
		if hcLaneReport(st, msg, "on-land-"+name, to, "") != 0 {
			fmt.Fprintf(os.Stderr, "hc-fill: 착지 뒤 %s 사건을 못 남김(hc-lane report, 주인 %s) — 로그 %s\n", name, o, lg)
		}
	}
}

func init() { register("_onland", onlandMain) }

// onlandMain: _onland <이름>
func onlandMain(args []string) {
	if len(args) != 1 || !okname(args[0]) {
		usage()
	}
	dry = false
	exit(onlandRun(filepath.Base(args[0])))
}
