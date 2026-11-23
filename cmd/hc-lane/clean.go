package main

// clean(셸 판 lib/hc-lane/clean.sh)과 park: 지우기 전에 다 확인하고, 하나라도 걸리면 아무것도 지우지 않는다.
// 「제출됨」은 큐 표식 refs/merge-queue/<UTC초>-<레인>이 그 커밋일 때만, 「주차됨」은 원격 refs/park/<레인>이 그 커밋일 때만
// (2026-11-21 hc-lane-go-start — 예전엔 원격 lane/<레인>을 봐서, 반송돼 큐에서 빠진 제출 가지·주차 가지가 같은 이름으로 섞였다).
// 정리가 중간에 끊긴 작업 폴더(추적 파일이 지워지기만 함)는 다시 부르면 이어서 지운다.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func init() {
	register("clean", cleanMain, nil)
	register("park", parkMain, noHelp) // --help·-h는 사용법(2026-11-23 park-help-guard — 예전엔 주차 글로 읽어 실제로 주차했다)
}

var (
	td, did         string
	owner, saved    string
	cleanSubmitTell = regexp.MustCompile(`^refs/merge-queue/[0-9]+-(.+)$`)
)

// cj: clean 결과 JSON 한 줄
func cj(state, reason string) {
	t := td
	if t == "" {
		t = "none"
	}
	fmt.Printf(`{"clean":"%s","reason":"%s","dry_run":%s,"on":"%s","repo":"%s","worktree":"%s","branch":"%s","base":"%s","teardown":"%s"}`+"\n",
		state, j(reason), tf(dry), j(on), j(repo), j(wt), j(branch), j(base), t)
}

// stop: 오류 출력에 알리고(자세한 목록은 30줄까지) JSON, 종료 1
func stop(msg string, detail ...string) {
	errf("hc-lane clean: %s\n", msg)
	if len(detail) > 0 && detail[0] != "" {
		ls := strings.Split(detail[0], "\n")
		for i, l := range ls {
			if i == 30 {
				break
			}
			errf("    %s\n", l)
		}
		if len(ls) > 30 {
			errf("    … (모두 %d줄)\n", len(ls))
		}
	}
	if did != "" {
		errf("hc-lane clean: 여기까지 했다: %s. 원인을 고친 뒤 같은 명령을 다시 돌린다\n", did)
	} else {
		errf("hc-lane clean: 아무것도 지우지 않았다. 원인을 고친 뒤 같은 명령을 다시 돌린다(폴더를 rm -rf로 직접 지우지 않는다)\n")
	}
	cj("stopped", msg)
	os.Exit(1)
}

// termGate: 작업 폴더에서 도는 살아 있는 다른 터미널이 없음을 확정한다 — 모르거나(hc ls 실패·판독 오류·기계 연결 모름) 있으면 멈춘다.
// 처음·보관 뒤·teardown 뒤(제거 직전)에 부른다 — 확인과 지우기 사이에 다시 붙은 터미널을 놓치지 않게(재감사2 lane/R-207)
func termGate(when string) {
	ps, r := termPairs()
	if r != 0 {
		stop(termWhy(r) + ": " + when + "이 폴더에서 도는 터미널을 확인하지 못했다")
	}
	if busy := busyTerms(ps, true); len(busy) > 0 {
		stop(when+"살아 있는 터미널이 이 폴더에서 돈다(끝났으면 hc kill <주소> 뒤 다시): "+wt, strings.Join(busy, "\n"))
	}
}

func addDid(s string) {
	if did != "" {
		did += ", "
	}
	did += s
}

// exists: 있음·없음만 믿고, 그 밖(hc 연결 끊김 등)은 멈춘다
func exists() bool {
	switch xrun(nil, os.Stderr, "sh", "-c", `[ -e "$1" ] || [ -L "$1" ]`, "hc-lane", wt) {
	case 0:
		return true
	case 1:
		return false
	}
	stop("폴더를 확인하지 못했다(hc·연결?): " + wt)
	return false
}

func hasBranch() bool {
	switch xrun(nil, os.Stderr, "git", "-C", repo, "show-ref", "--verify", "-q", "refs/heads/"+branch) {
	case 0:
		return true
	case 1:
		return false
	}
	stop("브랜치를 확인하지 못했다: " + branch)
	return false
}

func leftCheck(what, common string) {
	g, r := xout("git", "-C", repo, "status", "--porcelain", "--untracked-files=all", "--", ":(glob)**/.gitignore")
	if r != 0 {
		stop("메인 폴더 git status 실패: " + repo)
	}
	if g != "" {
		stop("메인 폴더의 .gitignore에 커밋 안 된 변경이 있어 "+what+" 폴더의 무시 판정을 믿을 수 없다: "+repo, g)
	}
	left, ok := leftover(repo, wt, common)
	if !ok {
		stop(what + " 폴더를 검사하지 못했다: " + wt)
	}
	if left != "" {
		stop(what+" 폴더에 .gitignore로 무시되지 않는 것(추적 파일 포함)이 있다: "+wt, left)
	}
}

// qaScript: 무시된 보관물 .qa를 메인 폴더의 같은 자리로 복사(덮지 않음) — 「c|s|d|f|n <경로>」 줄. 3 메인 폴더 없음.
// 옆 임시 파일에 다 복사한 뒤 ln -P(이미 있으면 실패 — 덮지 않는 원자 넣기)로 넣는다: 두 작업 트리가 같은 경로를 동시에 보관해도
// 한쪽이 다른 쪽을 덮지 않고, 진 쪽은 다시 견줘 다르면 d(그 작업 트리 보존, gofix E04)
const qaScript = `cd "$1" || exit 2
      r=$(find . -path ./.git -prune -o -type d -name .qa -print -prune) || exit 2
      [ -n "$r" ] || exit 0
      f=$(printf "%s\n" "$r" | while IFS= read -r d; do find "$d" ! -type d || exit 2; done) || exit 2
      f=$(printf "%s\n" "$f" | sed "/^$/d; s|^\./||") || exit 2
      [ -n "$f" ] || exit 0
      i=$(printf "%s\n" "$f" | git -c core.quotePath=false -C "$3" check-ignore --stdin); [ $? -le 1 ] || exit 2
      [ -n "$i" ] || exit 0
      [ -n "$2" ] && [ -d "$2" ] || exit 3
      [ "$4" = 0 ] || { printf "%s\n" "$i" | sed "s/^/n /"; exit 0; }
      printf "%s\n" "$i" | while IFS= read -r p; do
        if [ -e "$2/$p" ] || [ -L "$2/$p" ]; then k=s; cmp -s "$p" "$2/$p" || k=d
        else
          t= k=f
          if mkdir -p "$2/$(dirname "$p")" && t=$(mktemp "$2/$(dirname "$p")/.hc-lane-qa.XXXXXX") && cp -pP "$p" "$t" && cmp -s "$p" "$t"; then
            if ln -P "$t" "$2/$p" 2>/dev/null; then
              # 그사이 목적지가 폴더가 됐으면 ln은 그 안에 넣고 0 — 목적지 자체가 이 파일일 때만 보관됨(R-12)
              if [ ! -d "$2/$p" ] && [ "$t" -ef "$2/$p" ]; then k=c
              else rm -f "$2/$p/${t##*/}"; k=f; fi
            elif [ -d "$2/$p" ]; then k=f
            elif [ -e "$2/$p" ] || [ -L "$2/$p" ]; then k=s; cmp -s "$p" "$2/$p" || k=d
            fi
          fi
          [ -z "$t" ] || rm -f "$t"
        fi
        echo "$k $p"
      done`

var qaDirRe = regexp.MustCompile(`^(.*\.qa)/.*`)

func cleanMain(args []string) int {
	startSetup()
	dieHook = func(msg string) { cj("stopped", msg) } // 셸 판 die: clean이면 결과 JSON도
	rest := parseStart("clean", args)
	checkArgs("clean", rest)
	resolveRepo("clean")
	selfTerm = os.Getenv("HC_TERMINAL_ID")
	P := dryP()
	td, did = "none", ""
	if branch == base {
		die("기본 브랜치(%s)는 정리하지 않는다", base)
	}
	here, _ := os.Getwd()
	if h, err := filepath.EvalSymlinks(here); err == nil {
		here = h
	}
	common, r := xout("git", "-C", repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if r != 0 {
		stop("git 관리 폴더를 찾지 못했다: " + repo)
	}
	wl, r := xout("git", "-C", repo, "worktree", "list", "--porcelain")
	if r != 0 {
		stop("git worktree list 실패: " + repo)
	}
	var blk []string
	reg := false
	f := false
	for _, ln := range strings.Split(wl, "\n") {
		if ln == "worktree "+wt {
			reg, f = true, true
			continue
		}
		if f && strings.HasPrefix(ln, "worktree ") {
			f = false
		}
		if f {
			blk = append(blk, ln)
		}
	}
	dir := exists()
	br := hasBranch()
	if !reg && !dir && !br {
		fmt.Printf("정리할 것 없음: 워크트리 %s 와 브랜치 %s 가 이미 없다\n", wt, branch)
		cleanLease()
		keepOffSelf()
		cj("nothing", "")
		return 0
	}
	blkVal := func(k string) string {
		for _, ln := range blk {
			if strings.HasPrefix(ln, k) {
				return strings.TrimPrefix(ln, k)
			}
		}
		return ""
	}
	// 0) 이 브랜치의 작업 폴더인지, 잠기지 않았는지, 살아 있는 다른 터미널이 그 폴더에서 돌지 않는지
	if reg {
		wb := blkVal("branch ")
		if wb != "" && wb != "refs/heads/"+branch {
			stop("이 폴더는 다른 브랜치(" + strings.TrimPrefix(wb, "refs/heads/") + ")의 워크트리다: " + wt)
		}
		if wb == "" && br {
			stop("이 폴더는 분리 HEAD 워크트리인데 브랜치 " + branch + " 는 따로 있다(다른 레인의 폴더일 수 있다): " + wt)
		}
		for _, ln := range blk {
			if strings.HasPrefix(ln, "locked") {
				stop("잠긴 워크트리다(git worktree unlock 뒤 다시): " + wt)
			}
		}
	}
	if dir {
		termGate("")
	}

	// 1) 브랜치와 작업 폴더 HEAD가 기준에 들어갔는지(폴더가 없어도 등록의 HEAD로 본다)
	if remote {
		if dry {
			fmt.Printf("[dry-run] git fetch 건너뜀: 지금 있는 origin/%s 로 판정\n", base)
		} else if xfetch(repo, "-q", "origin") != 0 {
			stop("git fetch 실패: " + repo)
		}
	}
	if xrun(nil, os.Stderr, "git", "-C", repo, "rev-parse", "-q", "--verify", bref+"^{commit}") != 0 {
		stop(blab + " 가 없다: " + repo)
	}
	var targets []string
	if br {
		targets = append(targets, "refs/heads/"+branch)
	}
	if reg {
		h := blkVal("HEAD ")
		if h == "" {
			stop("워크트리 HEAD를 읽지 못했다: " + wt)
		}
		targets = append(targets, h)
	}
	var arch []string
	btip, subm := "", false
	if br {
		if btip, r = xout("git", "-C", repo, "rev-parse", "-q", "--verify", "refs/heads/"+branch+"^{commit}"); r != 0 {
			stop("브랜치를 읽지 못했다: " + branch)
		}
	}
	var lrr []string
	looked := false
	receipt, receiptInfo, doneRef, parkRef, sub := "", "", "", "", ""
	for _, c := range targets {
		lab := "워크트리 HEAD " + c[:min(12, len(c))]
		if strings.HasPrefix(c, "refs/heads/") {
			lab = "브랜치 " + branch
		}
		ia := xrun(nil, os.Stderr, "git", "-C", repo, "merge-base", "--is-ancestor", c, bref)
		if ia == 0 {
			continue
		}
		// 제출·주차한 레인 자신의 원격 표식이 원본을 쥘 때만 정리한다
		if ia == 1 && remote && br {
			if !looked {
				looked = true
				if !cleanOwner() {
					stop("레인 소유 기록 조회 실패: " + branch)
				}
				// 표식 이름: land·park가 가지 설정에 남긴 낸 이름(HC_LANE=<원래>로 낸 다시 맞춤·고침 레인), 없으면 레인 이름
				sub = owner
				switch v, r := xout("git", "-C", repo, "config", "--get", "branch."+branch+".hcLane"); {
				case r == 0 && lnameOK(v):
					sub = v
				case r != 1:
					stop("가지 설정 branch." + branch + ".hcLane 조회 실패(또는 꼴이 틀림): " + v)
				}
				doneRef, parkRef = "refs/merge-done/"+sub, "refs/park/"+sub
				out, r := xout("git", "-C", repo, "ls-remote", "origin", "refs/merge-queue/*", parkRef, doneRef, "refs/heads/lane/"+sub)
				if r != 0 {
					stop("git ls-remote 실패: " + lab + " · refs/merge-queue/*-" + sub)
				}
				lrr = strings.Split(out, "\n")
				for _, ln := range lrr {
					if fs := strings.Fields(ln); len(fs) == 2 && fs[1] == doneRef {
						receipt = fs[0]
					}
				}
			}
			s, r := xout("git", "-C", repo, "rev-parse", "-q", "--verify", c+"^{commit}")
			if r != 0 {
				stop("커밋을 읽지 못했다: " + lab)
			}
			hit := ""
			for _, ln := range lrr {
				fs := strings.Fields(ln)
				if len(fs) != 2 || fs[0] != s {
					continue
				}
				if m := cleanSubmitTell.FindStringSubmatch(fs[1]); m != nil && m[1] == sub && hit == "" {
					hit = fs[1]
				}
			}
			if hit != "" {
				fmt.Printf("%s제출됨: %s 은 큐 표식 %s 에 그대로 있다(병합기가 넣는다)\n", P, lab, hit)
				subm = true
				continue
			}
			if contains(lrr, s+"\t"+parkRef) {
				fmt.Printf("%s주차됨: %s 은 원격 %s 에 그대로 있다(선행이 들어가면 hc-fill이 이어 띄운다)\n", P, lab, parkRef)
				subm = true
				continue
			}
			// 옛 판 주차(원격 lane/<레인>): 이 레인의 마지막 주차·제출 사건이 같은 SHA의 「주차」일 때만 — 반송된 제출 가지와 가른다.
			// --resume은 refs/park가 없으면 lane/<레인>을 받는다(gofix L07)
			if contains(lrr, s+"\trefs/heads/lane/"+sub) && legacyParked(sub, s) {
				fmt.Printf("%s주차됨: %s 은 원격 refs/heads/lane/%s(옛 판 주차)에 그대로 있다\n", P, lab, sub)
				subm = true
				continue
			}
			// 병합기가 먼저 rebase해 넣으면 원본은 완료 기록의 부모로 남는다
			if receipt != "" {
				if receiptInfo == "" {
					if xrun(nil, nil, "git", "-C", repo, "cat-file", "-e", receipt+"^{commit}") != 0 {
						if dry {
							stop("원격 완료 기록을 받아야 판정할 수 있다: " + doneRef)
						}
						if xfetch(repo, "-q", "--no-tags", "--no-write-fetch-head", "origin", doneRef) != 0 {
							stop("완료 기록 받기 실패: " + doneRef)
						}
					}
					if receiptInfo, r = xout("git", "-C", repo, "show", "-s", "--format=%P %s", receipt); r != 0 {
						stop("완료 기록 조회 실패: " + receipt)
					}
				}
				if strings.HasPrefix(receiptInfo, s+" merge-queue lane="+sub+" ") && strings.Contains(receiptInfo, " orig="+s+" ") {
					fmt.Printf("%s착지됨: %s 에 %s 의 원본이 있다\n", P, doneRef, lab)
					subm = true
					continue
				}
			}
		}
		if ia == 1 && archive {
			if !remote {
				stop("원격(origin)이 없어 --archive로 보관할 수 없다: " + repo)
			}
			s, r := xout("git", "-C", repo, "rev-parse", "-q", "--verify", c+"^{commit}")
			if r != 0 {
				stop("커밋을 읽지 못했다: " + lab)
			}
			if !contains(arch, s) {
				arch = append(arch, s)
			}
			continue
		}
		lg, _ := xout("git", "-C", repo, "log", "--oneline", "-n", "10", bref+".."+c)
		stop(lab+" 이(가) "+blab+" 에 들어가지 않았다(착지 전이거나 다른 커밋). 착지 절차의 hc-lane land부터 다시 한다(다른 곳에서 rebase돼 착지했으면 --archive)", lg)
	}

	// 2) 지울 폴더에 추적 변경·무시되지 않은 untracked가 없는지. 추적 파일이 지워지기만 했으면(정리가 중간에 끊김) 이어서 지운다
	delOnly := false
	if reg && dir {
		st, r := xout("git", "-c", "core.quotePath=false", "-C", wt, "status", "--porcelain", "--untracked-files=all", "--ignore-submodules=none")
		if r != 0 {
			stop("git status 실패: " + wt)
		}
		if st != "" {
			n := 0
			for _, ln := range strings.Split(st, "\n") {
				if strings.HasPrefix(ln, " D ") {
					n++
				}
			}
			if n != len(strings.Split(st, "\n")) {
				stop("워크트리에 커밋 안 된 변경이나 무시되지 않은 untracked가 있다: "+wt, st)
			}
			delOnly = true
			fmt.Printf("%s정리가 끊긴 자리: 추적 파일 %d개가 이미 지워졌다(변경은 지움뿐) — 이어서 지운다\n", P, n)
		}
		igs, _ := xout("git", "-c", "core.quotePath=false", "-C", wt, "status", "--porcelain", "--ignored", "--untracked-files=normal")
		ig := ""
		for _, ln := range strings.Split(igs, "\n") {
			if strings.HasPrefix(ln, "!! ") {
				ig += strings.TrimPrefix(ln, "!! ") + " "
			}
		}
		if ig != "" {
			fmt.Printf("%s무시된 것도 같이 지운다: %s\n", P, ig)
		}
	} else if dir {
		leftCheck("등록이 지워진", common)
	}

	// 2.5) --archive: 안 들어간 커밋을 원격 refs/archive/<가지>-<sha8>에 보관. 하나라도 실패하면 아무것도 지우지 않는다
	for _, s := range arch {
		ar := "refs/archive/" + branch + "-" + s[:min(8, len(s))]
		if dry {
			fmt.Printf("[dry-run] git -C %s push origin %s:%s\n", q(repo), s, ar)
			continue
		}
		if zx(os.Stderr, os.Stderr, repo, "git", "push", "-q", "origin", s+":"+ar) != 0 {
			stop("보관 push 실패: " + ar + " (" + s + ")")
		}
		fmt.Printf("보관 %s (%s) — 되살리기: git fetch origin %s\n", ar, s, ar)
	}

	// 2.7) 무시된 보관물 .qa: 지우기 전 메인 폴더(첫 작업 트리)의 같은 자리로 복사(덮지 않음), 모두 같을 때만 지운다
	if dir {
		mw := ""
		for i, ln := range strings.Split(wl, "\n") {
			if ln == "" {
				break
			}
			if i == 0 && len(ln) >= 9 {
				mw = ln[9:]
			}
			if strings.HasPrefix(ln, "bare") {
				mw = ""
			}
		}
		if mw == wt {
			mw = ""
		}
		gd := repo
		if reg {
			gd = wt
		}
		d := "0"
		if dry {
			d = "1"
		}
		qa, r := xout("sh", "-c", qaScript, "hc-lane", wt, mw, gd, d)
		switch r {
		case 0:
		case 3:
			stop("메인 폴더를 찾지 못해 무시된 .qa를 옮길 수 없다(손으로 옮긴 뒤 다시): " + wt)
		default:
			m := mw
			if m == "" {
				m = "?"
			}
			stop("무시된 .qa를 메인 폴더로 옮기지 못했다: " + wt + " → " + m)
		}
		if qa != "" {
			ls := strings.Split(qa, "\n")
			seen := map[string]bool{}
			var ds []string
			nc, ns := 0, 0
			var dq []string
			for _, l := range ls {
				if len(l) < 3 || l[1] != ' ' || !strings.ContainsRune("csdfn", rune(l[0])) { // 줄 꼴이 틀리면 보관 결과를 모른다(R-11)
					stop("무시된 .qa 보관 결과를 읽지 못했다(줄 꼴): "+wt, l)
				}
				p := l[2:]
				if m := qaDirRe.FindStringSubmatch(p); m != nil {
					p = m[1]
				}
				if !seen[p] {
					seen[p] = true
					ds = append(ds, p)
				}
				switch {
				case strings.HasPrefix(l, "c "):
					nc++
				case strings.HasPrefix(l, "s "):
					ns++
				case strings.HasPrefix(l, "d "):
					dq = append(dq, "다름(메인 것 둠) "+l[2:])
				case strings.HasPrefix(l, "f "):
					dq = append(dq, "복사 실패 "+l[2:])
				}
			}
			sort.Strings(ds)
			var qp []string
			for _, p := range ds {
				qp = append(qp, wt+"/"+p+" → "+mw+"/"+p)
			}
			if dry {
				fmt.Printf("[dry-run] 무시된 보관물 복사(덮지 않음) 뒤 지움: %s (파일 %d개)\n", strings.Join(qp, ", "), len(ls))
			} else {
				if len(dq) > 0 {
					stop("무시된 .qa 파일 일부를 메인 폴더로 옮기지 못했다(같은 이름의 다른 파일은 덮지 않는다) — 비교·정리한 뒤 다시. 아무것도 안 지웠다: "+wt, strings.Join(dq, "\n"))
				}
				fmt.Printf("옮김 %s (파일 %d개 — 새로 %d, 이미 같음 %d)\n", strings.Join(qp, ", "), len(ls), nc, ns)
				addDid(".qa 메인 폴더로 복사")
			}
		}
	}

	// 2.9) 보관(.qa 복사·archive push)하는 사이 그 폴더에 터미널이 다시 붙었을 수 있다 — 지우기 전에 다시 확인(R-207)
	if dir && !dry {
		termGate("보관 뒤 ")
	}
	// 3) teardown(있으면, 실패해도 계속) → worktree remove → 남은 무시 파일 폴더 → branch -d
	if reg && dir && xtest("-f", wt+"/.claude/herdr-teardown.sh") {
		if dry {
			fmt.Println("[dry-run] 워크트리에서 bash .claude/herdr-teardown.sh")
			td = "dry-run"
		} else if r, lg := hook("teardown"); r == 0 {
			td, did = "ran", "teardown"
		} else {
			tailLog(lg)
			errf("hc-lane clean: herdr-teardown.sh 실패(위는 기록 끝 20줄), 정리는 계속한다\n")
			td, did = "failed", "teardown"
		}
		if !dry {
			termGate("teardown 뒤 제거 직전 ")
		}
	}
	if reg {
		if dry {
			fmt.Printf("[dry-run] git -C %s worktree remove %s\n", q(repo), q(wt))
		} else {
			// 무시된 파일 때문에 "Directory not empty"로 끝나도 등록은 지워질 수 있다. 등록으로 판정한다.
			// 변경이 지움뿐(끊긴 정리)이면 git이 「modified」로 거절하니 --force — 위에서 지움뿐임을 확인했다
			ra := []string{"git", "-C", repo, "worktree", "remove", wt}
			if delOnly {
				ra = []string{"git", "-C", repo, "worktree", "remove", "--force", wt}
			}
			xrun(os.Stderr, os.Stderr, ra...)
			wl2, r := xout("git", "-C", repo, "worktree", "list", "--porcelain")
			if r != 0 {
				stop("git worktree list 실패: " + repo)
			}
			if contains(strings.Split(wl2, "\n"), "worktree "+wt) {
				stop("git worktree remove 실패: " + wt)
			}
			addDid("워크트리 등록 제거")
		}
	}
	if dry {
		if !reg && dir {
			fmt.Printf("[dry-run] rm -rf -- %s  # 무시된 파일만 남은 폴더\n", q(wt))
		}
	} else if exists() {
		leftCheck("남은", common)
		if xrun(os.Stdout, os.Stderr, "rm", "-rf", "--", wt) != 0 {
			stop("남은 폴더를 지우지 못했다: " + wt)
		}
		addDid("남은 무시 파일 폴더 삭제")
	}
	if br {
		if dry {
			fmt.Printf("[dry-run] git -C %s branch -d %s\n", q(repo), branch)
		} else {
			if len(arch) > 0 || subm { // 보관·제출했으니 병합 여부 대신 확인한 값 그대로일 때만 지운다
				if xrun(os.Stderr, os.Stderr, "git", "-C", repo, "update-ref", "-d", "refs/heads/"+branch, btip) != 0 {
					stop("브랜치를 지우지 못했다(보관 뒤 값이 바뀌었나?): " + branch)
				}
			} else if xrun(os.Stderr, os.Stderr, "git", "-C", repo, "branch", "-q", "-d", branch) != 0 {
				stop("git branch -d 실패: " + branch)
			}
			addDid("브랜치 삭제")
		}
	}
	tdk := "돌릴 예정"
	switch td {
	case "ran":
		tdk = "통과"
	case "none":
		tdk = "없음(건너뜀)"
	case "failed":
		tdk = "실패(정리는 계속)"
	}
	if dry {
		fmt.Printf("[dry-run] 정리할 것: 워크트리 %s, 브랜치 %s (teardown: %s)\n", wt, branch, tdk)
	} else {
		at := ""
		if on != "" {
			at = " @" + on
		}
		fmt.Printf("정리 끝: 워크트리 %s, 브랜치 %s%s (teardown: %s)\n", wt, branch, at, tdk)
	}
	if on == "" && !dry {
		if strings.HasPrefix(os.Getenv("PWD")+"/", wt+"/") || strings.HasPrefix(here+"/", wt+"/") {
			fmt.Printf("알림: 지금 셸 위치는 지운 폴더 안이었다. 다음 명령 전에 cd %s\n", q(repo))
		}
	}
	cleanLease()
	keepOffSelf()
	if dry {
		cj("planned", "")
	} else {
		cj("done", "")
	}
	return 0
}
