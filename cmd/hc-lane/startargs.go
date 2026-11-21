package main

// 띄우기·clean·gc 인자 읽기·저장소 정하기(hc-lane-go-start, land·guard·lease·fetch는 args.go) — 셸 판 bin/hc-lane의 while 줄·검사·저장소 블록과 같은 순서·같은 글

import (
	"fmt"
	"os"
	"strings"
)

var (
	gcSecs      int64
	gcMins      int64
	nameCharsOK = func(s string) bool {
		for _, c := range s {
			if !(c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z') {
				return false
			}
		}
		return true
	}
)

func valS(args []string) string {
	if len(args) < 2 {
		die("%s 값이 없다", args[0])
	}
	return args[1]
}

// parseStart: 옵션을 전역에 읽고, 남은 것(-- 뒤 실행 명령, fetch의 참조)을 돌려준다
func parseStart(cmd string, args []string) []string {
	cmdName = cmd
	for len(args) > 0 {
		a := args[0]
		switch a {
		case "--on":
			on = valS(args)
			args = args[2:]
			continue
		case "--branch":
			branch = valS(args)
			args = args[2:]
			continue
		case "--name":
			name = valS(args)
			args = args[2:]
			continue
		case "--repo":
			repo = valS(args)
			args = args[2:]
			continue
		case "--brief":
			brief = valS(args)
			args = args[2:]
			continue
		case "--start-wait":
			swarg = valS(args)
			args = args[2:]
			continue
		case "--min-age":
			minage = valS(args)
			args = args[2:]
			continue
		case "--verdict":
			v := valS(args)
			if cmd != "land" {
				die("--verdict는 land에서만 쓴다")
			}
			if v == "" || strings.Trim(v, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-") != "" {
				die("--verdict 판정 ID 글자: %s", v)
			}
			_ = v // --verdict는 land 몫(이 판은 검사만)
			args = args[2:]
			continue
		case "--read":
			read = true
		case "--direct":
			direct = true
		case "--resume":
			resume = true
		case "--dry-run":
			dry = true
		case "--print-steps":
			steps = true
		case "--integ":
			integ = "refs/integ/"
		case "--remote-branches":
			rbFlag = true
		case "--archive":
			archive = true
		case "-h", "--help":
			usage()
		case "--":
			return args[1:]
		default:
			if strings.HasPrefix(a, "--integ=") {
				integ = strings.TrimPrefix(a, "--integ=")
				if integ == "" {
					die("--integ= 뒤에 접두가 없다")
				}
			} else if strings.HasPrefix(a, "-") {
				die("알 수 없는 인자: %s", a)
			} else {
				if cmd == "fetch" {
					return args
				}
				die("알 수 없는 인자: %s (실행 명령 앞에는 -- 를 둔다)", a)
			}
		}
		args = args[1:]
	}
	return nil
}

// checkArgs: 셸 판의 인자 검사(띄우기·clean·gc 몫)
func checkArgs(cmd string, rest []string) {
	launch := cmd == "launch"
	if !launch && brief+swarg != "" {
		die("--brief·--start-wait는 레인을 띄울 때만 쓴다")
	}
	if !launch && direct {
		die("--direct는 레인을 띄울 때만 쓴다")
	}
	if !(launch && !read) && resume {
		die("--resume은 쓰기 레인을 띄울 때만 쓴다")
	}
	if read && direct {
		errf("hc-lane: --read와 --direct는 함께 쓰지 않는다(--direct는 쓰기 레인만)\n")
		os.Exit(2)
	}
	switch cmd {
	case "clean":
		if repo == "" || name+branch == "" {
			usage()
		}
		if read || steps || len(rest) > 0 {
			die("clean에는 --read·--print-steps·실행 명령을 쓰지 않는다")
		}
	case "gc":
		if repo == "" {
			usage()
		}
		if name+branch != "" || read || steps || len(rest) > 0 {
			die("gc에는 --on·--repo·--dry-run·--integ·--min-age·--remote-branches만 쓴다")
		}
		switch {
		case integ == "":
		case strings.HasPrefix(integ, "refs/heads/"), strings.HasPrefix(integ, "refs/tags/"), strings.HasPrefix(integ, "refs/claims/"), strings.HasPrefix(integ, "refs/remotes/"):
			die("--integ 접두로 가지·태그·선점 참조는 쓰지 않는다: %s", integ)
		case strings.Trim(integ, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789/_.-") != "":
			die("--integ 접두에 쓸 수 없는 글자가 있다: %s", integ)
		case strings.HasPrefix(integ, "refs/") && strings.HasSuffix(integ, "/") && len(integ) > len("refs//"):
		default:
			die("--integ 접두는 refs/<이름>/ 꼴: %s", integ)
		}
		m := minage
		if m == "" {
			m = "24h"
		}
		k, u := m, "h"
		if n := len(m); n > 0 && strings.ContainsRune("smhd", rune(m[n-1])) {
			k, u = m[:n-1], m[n-1:]
		}
		if k == "" || strings.Trim(k, "0123456789") != "" {
			die("--min-age는 숫자에 s·m·h·d(단위 없으면 시간): %s", m)
		}
		var n int64
		fmt.Sscan(k, &n)
		switch u {
		case "s":
			gcSecs = n
		case "m":
			gcSecs = n * 60
		case "d":
			gcSecs = n * 86400
		default:
			gcSecs = n * 3600
		}
		gcMins = (gcSecs + 59) / 60
	case "launch":
		if name == "" || repo == "" {
			usage()
		}
	}
	if cmd != "clean" && archive {
		die("--archive는 clean에서만 쓴다")
	}
	if cmd != "gc" && (integ+minage != "" || rbFlag) {
		die("--integ·--min-age·--remote-branches는 gc에서만 쓴다")
	}
	if !nameCharsOK(name) {
		die("--name은 a-z 0-9 - _ 만 쓴다(hc 규칙): %s", name)
	}
	if len(name) > 32 {
		die("--name은 32자까지: %s", name)
	}
	if launch && !steps && len(rest) == 0 && brief == "" {
		die("-- 뒤에 실행 명령이 없다")
	}
}

// resolveRepo: --read 폴더(rdir)·메인 폴더(top)·원격·기본 가지·작업 폴더 자리를 정한다. 돌려준 값은 --read 폴더
func resolveRepo(cmd string) string {
	rdir := ""
	if cmd == "launch" && read {
		d, r := xout("sh", "-c", `cd "$1" && pwd -P`, "hc-lane", repo)
		if r != 0 || d == "" {
			die("폴더가 없다: %s%s", repo, onNote())
		}
		rdir = d
		if xrun(nil, nil, "git", "-C", rdir, "rev-parse", "--git-dir") != 0 {
			norepo = true
		}
	}
	if norepo {
		repo, remote, base, bref, blab = rdir, false, "", "", ""
		return rdir
	}
	wl, _ := xout("git", "-C", repo, "worktree", "list", "--porcelain")
	top = ""
	if first, _, _ := strings.Cut(wl, "\n"); strings.HasPrefix(first, "worktree ") {
		top = strings.TrimPrefix(first, "worktree ")
	}
	if top == "" {
		die("git 저장소가 아니다: %s%s", repo, onNote())
	}
	repo = top
	var ok bool
	if remote, base, ok = repoDefaults(repo); !ok {
		die("원격·기본 브랜치 조회 실패: %s", repo)
	}
	if remote {
		bref, blab = "refs/remotes/origin/"+base, "origin/"+base
	} else {
		bref, blab = "refs/heads/"+base, "로컬 "+base
	}
	if !plain(base) {
		die("기본 브랜치 이름에 쓸 수 없는 글자가 있다: %s", base)
	}
	if cmd != "gc" {
		if branch == "" {
			branch = name
		}
		if !plain(branch) || xrun(nil, nil, "git", "-C", repo, "check-ref-format", "--branch", branch) != 0 {
			die("브랜치 이름이 올바르지 않다(영문·숫자·/ _ . - 만): %s", branch)
		}
		i := strings.LastIndex(repo, "/")
		wt = repo[:i] + "/" + repo[i+1:] + ".wt/" + strings.ReplaceAll(branch, "/", "-")
		rx = ""
		if on != "" {
			rx = "hc exec --on " + on + " -- "
		}
		undo = fmt.Sprintf("%sgit -C %s worktree remove --force %s && %sgit -C %s branch -d %s", rx, q(repo), q(wt), rx, q(repo), branch)
	}
	return rdir
}

func onNote() string {
	if on != "" {
		return " (기계 " + on + ")"
	}
	return ""
}

func dryP() string {
	if dry {
		return "[dry-run] "
	}
	return ""
}
