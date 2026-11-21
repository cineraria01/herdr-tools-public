package main

// cotenant-guard 셸 낱말 가르기 — sed -i·git 하위 명령 판정 (cgtok.go에서 옮김, awk 원문 그대로)

import (
	"regexp"
	"strings"
)

var (
	awkSedI      = regexp.MustCompile(`^-[A-Za-z]*i`)
	awkMirror    = regexp.MustCompile(`(?s)^remote\..*\.mirror=`)
	awkMirrorOff = regexp.MustCompile(`=(false|no|off|0)$`)
	awkPushPlus  = regexp.MustCompile(`(?s)^remote\..*\.push=\+`)
	awkAlias     = regexp.MustCompile(`^alias\.[^=]+=`)
	awkShortOpt  = regexp.MustCompile(`(?s)^-[^-]`)
	awkAmend     = regexp.MustCompile(`^--am(e(n(d)?)?)?$`)
	awkCommitVal = regexp.MustCompile(`^--(message|file|author|date|fixup|squash|cleanup|trailer|template|reuse-message|reedit-message|pathspec-from-file)$`)
	awkTopColon  = regexp.MustCompile(`^:(/|\(top\))?$`)
	awkLeaseCAS  = regexp.MustCompile(`^--force-with-lease=[^:]*:`)
	awkForce     = regexp.MustCompile(`^--(force|force-with-lease|mirror)(=|$)`)
	awkAnyOpt    = regexp.MustCompile(`(?s)^-.`)
	awkPlusOpt   = regexp.MustCompile(`(?s)^\+.`)
	awkSq        = regexp.MustCompile(`^[A-Za-z0-9_./:=@%+,-]+$`)
	awkSkipWord  = regexp.MustCompile(`^(if|then|else|elif|do|while|until|time|!|\{|\}|exec|command|nohup|sudo|builtin)$`)
	awkAssign    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	awkXargsV    = regexp.MustCompile(`^-[IJLnPsEd]$`)
	awkShellC    = regexp.MustCompile(`^-[A-Za-z]*c[A-Za-z]*$`)
	awkShells    = regexp.MustCompile(`^(bash|sh|zsh|dash|ksh)$`)
)

func (k *cgTok) sedi(at int) {
	ip := false
	for j := at + 1; j <= k.nt; j++ {
		if awkSedI.MatchString(k.t[j]) || strings.HasPrefix(k.t[j], "--in-place") {
			ip = true
		}
	}
	if !ip {
		return
	}
	script := true
	for j := at + 1; j <= k.nt; j++ {
		a := k.t[j]
		if a == "-e" || a == "-f" || a == "--expression" || a == "--file" {
			j++
			script = false
			continue
		}
		if strings.HasPrefix(a, "-") || a == "" {
			continue
		}
		if script {
			script = false
			continue
		}
		k.emit("WRITE", k.cwd, k.tilde(a))
	}
}

func (k *cgTok) git(at int) {
	d := k.cwd
	j := at + 1
	k.al = map[string]string{}
	for j <= k.nt && strings.HasPrefix(k.t[j], "-") {
		o := k.t[j]
		if o == "-C" {
			d = awkJoin(d, k.tilde(k.tt(j+1)))
			j += 2
			continue
		}
		if o == "-c" {
			x := k.tt(j + 1)
			if (awkMirror.MatchString(x) && !awkMirrorOff.MatchString(x)) || awkPushPlus.MatchString(x) {
				k.emit("FORCEPUSH", d, "git -c "+x)
				return
			}
			if awkAlias.MatchString(x) {
				an := x[6:]
				if e := strings.IndexByte(an, '='); e >= 0 {
					an = an[:e]
				}
				k.al[asciiLower(an)] = x[strings.IndexByte(x, '=')+1:]
			}
		}
		if o == "-c" || o == "--git-dir" || o == "--work-tree" || o == "--namespace" || o == "--config-env" {
			j += 2
			continue
		}
		j++
	}
	if j > k.nt {
		return
	}
	s := k.t[j]
	if ab, ok := k.al[asciiLower(s)]; ok {
		x := "git " + ab
		if strings.HasPrefix(ab, "!") {
			x = ab[1:]
		}
		for a := j + 1; a <= k.nt; a++ {
			x += " " + awkSqWord(k.t[a])
		}
		k.emit("SUB", d, x)
		return
	}
	dd := false
	switch s {
	case "add":
		if k.subnext || k.viax {
			if k.viax {
				k.emit("DYN", d, "git add (xargs)")
			} else {
				k.emit("DYN", d, "git add $(…)")
			}
		}
		for a := j + 1; a <= k.nt; a++ {
			o := k.t[a]
			if !dd && o == "--" {
				dd = true
				continue
			}
			if !dd && strings.HasPrefix(o, "-") {
				if o == "--all" || o == "--update" || o == "--no-ignore-removal" || (awkShortOpt.MatchString(o) && strings.ContainsAny(o, "Au")) {
					k.emit("ADDALL", d, "git add "+o)
				}
				continue
			}
			if o == "." || o == "*" || awkTopColon.MatchString(o) {
				k.emit("ADDALL", d, "git add "+o)
			} else {
				k.emit("ADD", d, k.tilde(o))
			}
		}
	case "commit":
		all, np := false, 0
		for a := j + 1; a <= k.nt; a++ {
			o := k.t[a]
			if !dd && o == "--" {
				dd = true
				continue
			}
			if !dd && strings.HasPrefix(o, "--") {
				if o == "--all" {
					all = true
					k.emit("COMMITALL", d, "git commit --all")
				}
				if awkAmend.MatchString(o) {
					k.emit("AMEND", d, "git commit --amend")
				}
				if awkCommitVal.MatchString(o) {
					a++
				}
				continue
			}
			if !dd && awkAnyOpt.MatchString(o) {
				for q := 1; q < len(o); q++ {
					c2 := o[q]
					if c2 == 'a' {
						all = true
						k.emit("COMMITALL", d, "git commit "+o)
						break
					}
					if strings.IndexByte("mFCct", c2) >= 0 {
						if q == len(o)-1 {
							a++
						}
						break
					}
				}
				continue
			}
			np++
			k.emit("ADD", d, k.tilde(o))
		}
		if !all && np == 0 {
			k.emit("COMMITSTAGED", d, "git commit")
		}
	case "checkout":
		for a := j + 1; a <= k.nt; a++ {
			o := k.t[a]
			if o == "--" {
				k.emit("DISCARD", d, "git checkout --")
				return
			}
			if strings.HasPrefix(o, "-") {
				if o == "-f" || o == "--force" || o == "-p" || o == "--patch" {
					k.emit("DISCARD", d, "git checkout "+o)
				}
				if o == "-b" || o == "-B" || o == "--orphan" {
					k.emit("LAND", d, "checkout "+o)
					a++
				}
				continue
			}
			if o == "." {
				k.emit("DISCARD", d, "git checkout .")
				continue
			}
			k.emit("DISCARDPATH", d, k.tilde(o))
			k.emit("LAND", d, "checkout "+o)
		}
	case "switch":
		for a := j + 1; a <= k.nt; a++ {
			if o := k.t[a]; o == "-f" || o == "--force" || o == "--discard-changes" {
				k.emit("DISCARD", d, "git switch "+o)
				return
			}
		}
		k.emit("LAND", d, "switch")
	case "restore":
		st, wt := false, false
		for a := j + 1; a <= k.nt; a++ {
			o := k.t[a]
			if o == "--" {
				break
			}
			if o == "--staged" {
				st = true
			} else if o == "--worktree" {
				wt = true
			} else if awkShortOpt.MatchString(o) {
				if strings.Contains(o, "S") {
					st = true
				}
				if strings.Contains(o, "W") {
					wt = true
				}
			}
		}
		if wt || !st {
			k.emit("DISCARD", d, "git restore")
		}
	case "stash":
		a := j + 1
		for a <= k.nt && strings.HasPrefix(k.t[a], "-") {
			a++
		}
		x := k.tt(a)
		if x != "list" && x != "show" {
			if x == "" {
				k.emit("STASH", d, "git stash")
			} else {
				k.emit("STASH", d, "git stash "+x)
			}
		}
	case "reset":
		for a := j + 1; a <= k.nt; a++ {
			if k.t[a] == "--hard" {
				k.emit("RESETHARD", d, "git reset --hard")
			}
		}
	case "clean":
		dry := false
		for a := j + 1; a <= k.nt; a++ {
			if k.t[a] == "--dry-run" || (awkShortOpt.MatchString(k.t[a]) && strings.Contains(k.t[a], "n")) {
				dry = true
			}
		}
		if !dry {
			k.emit("CLEAN", d, "git clean")
		}
	case "rebase", "merge", "pull":
		for a := j + 1; a <= k.nt; a++ {
			if k.t[a] == "--abort" || k.t[a] == "--quit" {
				return
			}
		}
		k.emit("LAND", d, s)
	case "rm", "mv":
		x := k.cwd
		k.cwd = d
		k.nonopt(j, false)
		k.cwd = x
	case "push":
		lz, fi := false, ""
		for a := j + 1; a <= k.nt; a++ {
			o := k.t[a]
			if awkLeaseCAS.MatchString(o) {
				lz = true
				continue
			}
			if o == "--force-if-includes" {
				fi = o
				continue
			}
			if awkForce.MatchString(o) {
				k.emit("FORCEPUSH", d, "git push "+o)
				return
			}
			if o == "-o" || o == "--push-option" || o == "--repo" || o == "--receive-pack" || o == "--exec" {
				a++
				continue
			}
			if awkShortOpt.MatchString(o) {
				for q := 1; q < len(o); q++ {
					c2 := o[q]
					if c2 == 'f' {
						k.emit("FORCEPUSH", d, "git push "+o)
						return
					}
					if c2 == 'o' {
						if q == len(o)-1 {
							a++
						}
						break
					}
				}
				continue
			}
			if strings.HasPrefix(o, "+") {
				k.emit("FORCEPUSH", d, "git push … "+o)
				return
			}
		}
		if fi != "" && !lz {
			k.emit("FORCEPUSH", d, "git push "+fi+" (기대값 적은 --force-with-lease 없이)")
		}
	}
}
