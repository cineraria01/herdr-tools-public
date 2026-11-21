#!/bin/bash
# cotenant-guard — Claude Code PreToolUse guard (Edit|Write|MultiEdit|NotebookEdit|Bash) against two agents editing
# the same files of one git working tree (2026-11-04 1단계; 감사 두 번과 10개 동시 시험 결과 반영). Exit 0 = allow,
# 2 = block (stderr goes to Claude); any other failure blocks too because the settings entry sets "onFailure": "block". The entry runs
#   /bin/bash -c '[ -r "$HOME/.claude/hooks/cotenant-guard.sh" ] || exit 0; exec /bin/bash "$HOME/.claude/hooks/cotenant-guard.sh"'
# so a missing file allows while a broken one blocks.
#
# Lease: <git-dir>/agent-leases/<md5 of path relative to the tree root>/owner, one line split by \037:
#   session pid procStart agent_id name since relpath
# Every edit records a lease (atomic mkdir). An edit is refused when another live process owns the lease and the
# file has uncommitted changes (or it touched the file in the last 10 minutes, ignored files excepted), or when
# nobody owns an already-dirty file while another agent shares the tree. A dead owner (pid gone or procStart differs
# = pid reused) hands over. Leases on clean files that anyone could take over (dead owner, live owner idle for 10
# minutes, ignored file) are deleted when the leases are scanned, by one scanner per tree at a time.
# Bash: git commands that mix or drop other people's changes (also a commit without paths, --amend) are refused while another agent shares the tree;
# writes (sed -i, >, cp, mv, rm, tee, git rm/mv) to a file or folder holding someone else's uncommitted file are
# refused, other file targets are leased. bash/sh/zsh -c, eval, timeout, nice and xargs are looked through.
# Force push (-f, --force, bare --force-with-lease or one without ":<expect>", --mirror, a "+" refspec, --force-if-includes
# without an expect lease) is refused always, shared tree or not (2026-11-17); push --delete, fetch "+refspec" and
# --force-with-lease=<ref>:<expect> (compare-and-swap: refs/claims) are not. No agent bypass: the off switch below only.
#
# Off switch, people only: "AGENT_LEASE_OFF": "1" in the env block of ~/.claude/settings.json (read from the file,
# an environment variable alone does nothing). Agents may not write that switch, disableAllHooks or touch
# .git/agent-leases. Change this file only as: temp copy -> bash -n -> test passes -> mv over it.
# Test (repo): go test ./claude/hooks/tests/ -run CotenantGuard (fake HOME = registry + settings, fake hc on PATH; go·bash both).
# Go 판(cmd/herdr-hooks, 같은 판정)으로 — HERDR_HOOKS_IMPL=bash(또는 HERDR_IMPL=bash)면 셸 판, 바이너리가 없거나 낡으면 이번은 셸 판·짓기는 뒤에서(lib/go-run.sh GO_RUN_BUILD=bg, 2026-11-21 hooks-go)
for _hg in "${BASH_SOURCE[0]%/*}/../../lib/go-run.sh" "${BASH_SOURCE[0]%/*}/../../.local/lib/go-run.sh"; do
  [ -r "$_hg" ] && { . "$_hg" && GO_RUN_BUILD=bg go_run herdr-hooks cotenant-guard || exit 1; break; }
done
CFG=${CLAUDE_CONFIG_DIR:-$HOME/.claude}
US=$'\037'
# absolute tools: a GNU stat, a PATH without /sbin or a stray jq must not change the verdict
JQ=/usr/bin/jq MD5=/sbin/md5 STAT=/usr/bin/stat AWK_BIN=/usr/bin/awk DATE=/bin/date PS=/bin/ps PERL=/usr/bin/perl
TOUCH=/usr/bin/touch MKDIR=/bin/mkdir RMDIR=/bin/rmdir MV=/bin/mv RM=/bin/rm XARGS=/usr/bin/xargs
GIT=/var/db/xcode_select_link/usr/bin/git; [ -x "$GIT" ] || GIT=/usr/bin/git   # the git behind the /usr/bin/git shim (no xcrun lookup)
NOW=
now() { [ -n "$NOW" ] || NOW=$($DATE +%s); }   # one clock read per verdict
SECONDS=0   # whole seconds this verdict has run, read without a fork (R2); the environment can carry another value

settings=; [ -r "$CFG/settings.json" ] && settings=$(< "$CFG/settings.json")
[[ $settings =~ \"AGENT_LEASE_OFF\"[[:space:]]*:[[:space:]]*\"?1 ]] && exit 0

input=$($JQ -r '(.tool_input.file_path // .tool_input.notebook_path // "") as $fp
  | @sh "SID=\(.session_id // "") AID=\(.agent_id // "") HCWD=\(.cwd // "") TOOL=\(.tool_name // "") FP=\($fp) CMD=\(.tool_input.command // "") NEW=\(if ($fp | test("/\\.claude/settings[^/]*\\.json$")) then ([.tool_input.content, .tool_input.new_string, (.tool_input.edits[]?.new_string)] | map(select(type == "string")) | join("\n")) else "" end)"') ||
  { echo "cotenant-guard: 훅 입력 JSON을 읽지 못했다" >&2; exit 1; }
eval "$input"
[ -n "$HCWD" ] || HCWD=$PWD

block() { printf 'cotenant-guard: %s\n' "$1" >&2; exit 2; }
WARN=
warn() { WARN="$WARN${WARN:+ }$1"; }

OFF="훅·가드를 끄거나 임대를 지우는 일(disableAllHooks·AGENT_LEASE_OFF·.git/agent-leases)은 사람만 한다. 막혔으면 다른 일을 먼저 하고 나중에 다시 시도하거나 지휘에 알리거나 git worktree add …에서 하라."
case $TOOL in
  Bash) case $CMD in *disableAllHooks*|*AGENT_LEASE_OFF*|*.git/agent-leases*) block "$OFF" ;; esac ;;
  *) case $NEW in *disableAllHooks*|*AGENT_LEASE_OFF*) block "$OFF" ;; esac ;;
esac

# R = $1 with runs of blanks collapsed (ps pads lstart, the registry does not)
norm() { local IFS=' '; set -f; set -- $1; set +f; R="$*"; }

# PD = physical path of the nearest existing directory of $1, PREST = the part below it that does not exist yet
phys() {
  local p=$1 rest=
  case $p in /*) ;; *) p=$HCWD/$p ;; esac
  while [ ! -d "$p" ]; do
    rest=${p##*/}${rest:+/$rest}
    p=${p%/*}; [ -n "$p" ] || p=/
  done
  cd -P -- "$p" 2>/dev/null || return 1
  PD=$PWD PREST=$rest
}

# ROOT GITDIR LEASES for physical dir $1; fails outside a work tree (also inside .git)
tree() {
  [ "$1" = "$T_DIR" ] && return $T_RC
  T_DIR=$1 T_RC=1
  local o rc; o=$($GIT -C "$1" rev-parse --show-toplevel --absolute-git-dir 2>/dev/null); rc=$?
  [ $rc = 128 ] && return 1   # git's answer for "not in a work tree"; anything else means git itself failed (S7)
  [ $rc = 0 ] || block "git을 실행하지 못했다($GIT, 종료 $rc). 판정할 수 없어 막았다. 사람에게 알려라."
  ROOT=${o%%$'\n'*} GITDIR=${o#*$'\n'} LEASES=${o#*$'\n'}/agent-leases T_RC=0
}

# REL = path of $1 relative to ROOT (fails for ROOT itself or outside it); FULL = its physical path
relof() {
  phys "$1" || return 1
  tree "$PD" || return 1
  FULL=$PD${PREST:+/$PREST}
  REL=${FULL#"$ROOT"/}
  [ "$REL" != "$FULL" ] && [ -n "$REL" ]
}

# true when dir $1 is ROOT or below it, but not inside a nested work tree (e.g. .claude/worktrees/x)
intree() {
  cd -P -- "$1" 2>/dev/null || return 1
  local p=$PWD
  case $p in "$ROOT") return 0 ;; "$ROOT"/*) ;; *) return 1 ;; esac
  while [ "$p" != "$ROOT" ]; do [ -e "$p/.git" ] && return 1; p=${p%/*}; done
}

# Session registry Claude writes itself: one line per live-or-stale Claude process
reg() {
  [ -n "$REG_DONE" ] && return; REG_DONE=1
  # Claude writes each file without a trailing newline: read them as a JSON stream, file by file if one is broken
  local f q='[.pid, .sessionId, .procStart, .cwd, .name] | map(. // "" | tostring) | join("\u001f")'
  REG=$($JQ -r "$q" "$CFG"/sessions/*.json 2>/dev/null) ||
    REG=$(for f in "$CFG"/sessions/*.json; do $JQ -r "$q" "$f" 2>/dev/null; done)
}

# MYPID MYSTART MYNAME of the Claude process that runs this hook (hook's parent; the registry by session id)
me() {
  [ -n "$ME_DONE" ] && return; ME_DONE=1
  reg
  local pid sid st cwd name
  while IFS=$US read -r pid sid st cwd name; do
    [ "$sid" = "$SID" ] || continue
    MYPID=$pid MYSTART=$st MYNAME=$name
    [ "$pid" = "$PPID" ] && break   # duplicate resume: two processes share the id, ours is the hook's parent
  done <<EOF
$REG
EOF
  if [ -z "$MYPID" ]; then   # not registered (yet): the hook's parent is the Claude process
    MYPID=$PPID; norm "$(LC_ALL=C TZ=UTC $PS -o lstart= -p "$PPID" 2>/dev/null)"; MYSTART=$R
  fi
  MYNAME=${MYNAME:-claude}${HC_TERMINAL_ID:+[$HC_TERMINAL_ID]}
}

# alive PID PROCSTART: pid exists and started when recorded (procStart guards against pid reuse)
PSC= AOK=
alive() {
  [ -n "$1" ] && kill -0 "$1" 2>/dev/null || return 1   # dead pids cost nothing and are not cached
  case $AOK in *"|$1 $2|"*) return 0 ;; esac   # this pid and record checked already (a scan meets it per lease)
  local ls
  case $PSC in
    *" $1="*) ls=${PSC#*" $1="}; ls=${ls%%|*} ;;
    *) norm "$(LC_ALL=C TZ=UTC $PS -o lstart= -p "$1" 2>/dev/null)"; ls=$R; PSC="$PSC $1=$ls|" ;;
  esac
  [ -n "$ls" ] || return 1
  [ -z "$2" ] && return 0
  norm "$2"; [ "$ls" = "$R" ] && AOK="$AOK|$1 $2|"
}

dirty() { [ -n "$(GIT_OPTIONAL_LOCKS=0 $GIT -C "$ROOT" status --porcelain -uall -- ":(literal)$1" 2>/dev/null)" ]; }

owner() {
  O_SID= O_PID= O_START= O_AID= O_NAME= O_SINCE= O_PATH=
  [ -f "$1/owner" ] && IFS=$US read -r O_SID O_PID O_START O_AID O_NAME O_SINCE O_PATH < "$1/owner"
}
mine() {
  [ "$O_PID" = "$MYPID" ] || return 1
  [ -z "$O_START" ] || [ "$O_START" = "$MYSTART" ] && return 0   # own() writes MYSTART as is
  norm "$O_START"; local a=$R; norm "$MYSTART"; [ "$a" = "$R" ]
}
own() {
  me
  printf '%s\n' "$SID$US$MYPID$US$MYSTART$US$AID$US$MYNAME$US$($DATE '+%F %T')$US$2" > "$1/owner.$$" &&
    $MV -f "$1/owner.$$" "$1/owner"
}
# claim FILE: create it exclusively (O_EXCL, no fork); fails when it exists (also as a directory)
claim() { local rc; set -C; { : > "$1"; } 2>/dev/null; rc=$?; set +C; return $rc; }
# claimgen FILE: claim one owner generation. A claim older than 60 s is a leftover (its taker or the cleanup died
# before finishing; a hook lives 5 s at most). Whoever finds it stale re-arms it first (mtime = now, no fork), then
# only the caller that creates FILE.steal-<the stale mtime> owns it (S3: "remove, then claim" let several callers each
# remove the claim another had just made). Dying at any point leaves it stale as it was or fresh again, so 60 s on it
# goes stale under a new marker name and frees itself (R1: a taker killed between its marker and rm + claim left the
# claim stale under the same name for good). CLAIM_AGE is the age that refused us.
claimgen() {
  claim "$1" && return 0
  local m; m=$($STAT -f %m "$1" 2>/dev/null) || { CLAIM_AGE=0; return 1; }   # gone meanwhile: someone else is on it
  now; CLAIM_AGE=$(( NOW - m ))
  [ "$CLAIM_AGE" -ge 60 ] || return 1
  # gone already (a late loser: the winner has finished and removed .cleaning): do not bring it back unowned
  [ ! -e "$1" ] || { : >| "$1"; } 2>/dev/null || $TOUCH "$1" 2>/dev/null   # touch: a claim folder of an older version
  claim "$1.steal-$m" || { CLAIM_AGE=0; return 1; }   # another caller took over that leftover right now
}
# take over the current owner generation; one claim per generation makes takers (and the cleanup) exclusive. A lease
# folder without owner has one fixed claim name (S4: a name from the folder mtime changed with the first claim).
takeover() {
  local c=$1/claim-${O_PID}-${O_SINCE//[^0-9]/}
  [ -n "$O_PID$O_SINCE" ] || c=$1/claim-orphan
  claimgen "$c" || block "$2: 다른 에이전트가 ${CLAIM_AGE}초 전 이 파일 임대를 넘겨받기 시작했다. 같은 편집을 곧바로 되풀이하지 말고 다른 일을 먼저 하라. 그 넘겨받기가 끝나지 않았다면 60초가 지나면 저절로 풀린다. 그 뒤에도 막히면 지휘에 알려라."
  own "$1" "$2"
}

# refuse_live LD REL: a live owner keeps a dirty file, and a clean one while it touched it in the last 10 minutes
# (else a long-lived coordinator would hold every file it ever edited until it exits). Ignored files (.qa reports,
# logs) are scratch and hand over at once.
refuse_live() {
  local busy="$2 은 $O_NAME(pid $O_PID, since $O_SINCE)이 고치는 중이다(임대는 그 프로세스가 끝나면 풀린다)"
  local out="다른 일을 먼저 하고 나중에 다시 시도하거나 지휘에 알리거나, 꼭 지금이면 git worktree add -b <슬러그> ../${ROOT##*/}.wt/<슬러그> origin/main 에서 하라."
  dirty "$2" && block "$busy(미커밋 변경 있음). $out"
  $GIT -C "$ROOT" check-ignore -q -- "$2" 2>/dev/null && return 0
  now; [ $(( NOW - $($STAT -f %m "$1/owner") )) -lt 600 ] && block "$busy(마지막 편집 10분 안, 커밋됐거나 아직 안 씀). $out"
  return 0
}

# lease REL MODE — 1 (edit tools) also refuses an unleased dirty file while the tree is shared; 0 = Bash write;
# check = refuse only, record nothing (Bash checks every target before it leases any)
lease() {
  local rel=$1 ld
  if [ "$2" = check ]; then
    ld=$LEASES/$($MD5 -q -s "$rel")
    owner "$ld" || return 0
    me; mine && return 0
    alive "$O_PID" "$O_START" && refuse_live "$ld" "$rel"
    return 0
  fi
  [ -d "$LEASES" ] || $MKDIR -p "$LEASES" 2>/dev/null || return 0   # cannot record (read-only git dir): allow
  ld=$LEASES/$($MD5 -q -s "$rel")
  if $MKDIR "$ld" 2>/dev/null; then
    if dirty "$rel"; then
      if [ "$2" != 1 ]; then $RMDIR "$ld"; return 0; fi   # a Bash write does not adopt changes it did not make
      if cotenant; then
        $RMDIR "$ld"
        block "$rel: 임대 기록 없는 미커밋 변경이 있다(같은 작업 트리: $WHO). 누구 것인지 git diff -- '$rel'로 확인하라. 남의 것이면 다른 일을 먼저 하고 나중에 다시 시도하거나 지휘에 알리고, 네 것(Bash로 바꾼 것 등)이면 그 변경을 커밋한 뒤 고치거나 git worktree add -b <슬러그> ../${ROOT##*/}.wt/<슬러그> origin/main 에서 하라."
      fi
    fi
    own "$ld" "$rel"; return 0
  fi
  me
  if ! owner "$ld"; then
    now; [ $(( NOW - $($STAT -f %m "$ld") )) -lt 10 ] && block "$rel: 다른 에이전트가 지금 이 파일 임대를 잡는 중이다. 잠시 뒤 다시 시도하라."
    takeover "$ld" "$rel"; return 0
  fi
  if mine; then
    $TOUCH "$ld/owner"   # mtime = owner's last edit (see the idle hand-over below)
    [ "$O_AID" != "$AID" ] && warn "$rel 은 같은 세션의 다른 에이전트(${O_AID:-메인})가 먼저 고친 파일이다. 서로 덮어쓰지 않게 파일 소유를 정하라."
    return 0
  fi
  if alive "$O_PID" "$O_START"; then
    [ "$O_SID" = "$SID" ] && warn "같은 세션 $SID 를 다른 프로세스(pid $O_PID)도 돌리고 있다(중복 재개). 하나는 즉시 끝내라."
    refuse_live "$ld" "$rel"
    takeover "$ld" "$rel"; return 0
  fi
  takeover "$ld" "$rel"
  [ "$O_SID" != "$SID" ] && dirty "$rel" &&
    warn "$rel 에 끝난 에이전트 $O_NAME(pid $O_PID, since $O_SINCE)의 미커밋 변경이 남아 있다. 고치기 전에 git diff -- '$rel'을 먼저 보라."
  return 0
}

# FDIRTY from the leases of the changed paths and of the folders above them only: one tree status, one perl
changed_leases() {
  [ -x "$PERL" ] || block "perl을 찾지 못했다($PERL). 판정할 수 없어 막았다. 사람에게 알려라."   # else FDIRTY would come out empty
  local -a fd=()
  while IFS=$US read -r O_SID O_PID O_START O_AID O_NAME O_SINCE O_PATH; do
    [ "$O_SID" = $'\001' ] && block "git status가 실패했다($ROOT). 남의 미커밋 파일을 판정할 수 없어 막았다. 색인·권한을 사람에게 알려라."   # else FDIRTY would come out empty
    [ "$O_PID" = "$MYPID" ] && mine && continue
    alive "$O_PID" "$O_START" && fd+=("$O_PATH$US$O_NAME$US$O_PID$US$O_SINCE")
  done < <({ GIT_OPTIONAL_LOCKS=0 $GIT -C "$ROOT" status --porcelain -z -uall 2>/dev/null || printf '\001\0'; } | L=$LEASES $PERL -MDigest::MD5=md5_hex -0ne '
    chomp; if ($_ eq "\001") { print "\001\n"; next }   # git status failed (no "XY path" entry is this)
    if ($skip) { $skip = 0; next }
    if ($src) { $src = 0 } else { my $st = substr($_, 0, 2); $_ = substr($_, 3); $src = $st =~ /^R/; $skip = $st =~ /^C/ }
    for (my $p = $_; length $p; $p =~ s{/?[^/]*$}{}) {   # the path, then each folder above it
      next if $seen{$p}++;
      open(my $h, "<", "$ENV{L}/" . md5_hex($p) . "/owner") or next; my $l = <$h>; chomp $l; print "$l\n";
    }')
  FDIRTY=; [ ${#fd[@]} -gt 0 ] && printf -v FDIRTY '%s\n' "${fd[@]}"
  return 0
}

# FDIRTY: leases in ROOT held by other live processes on files with uncommitted changes, lines rel\037name\037pid\037since.
# One scanner per tree at a time ($LEASES/.cleaning; S1: all at once overran the 5 s limit) reads every lease and deletes
# those on clean files that a take-over would hand over anyway: a dead owner's, a live owner's untouched for 10 minutes,
# any on an ignored file (H2, S2: they pile up). The others read only the leases of the changed paths and of the folders
# above them (S2: the shell work per lease is the cost). Arrays and one awk join, not strings grown per lease (n^2).
# The scanner stops reading after about 1-2 s and deleting after 2-3 s (R2: under load one scanner of 1000 leases ran
# past the 5 s limit); it then deletes only what it read and takes FDIRTY the changed-paths way, the next scanner goes on.
scan_foreign() {
  [ "$F_ROOT" = "$ROOT" ] && return; F_ROOT=$ROOT FDIRTY=
  [ -d "$LEASES" ] || return 0
  me
  local f e i j k c st sfail= part=; local -a lr=() lw=() ld=() lp=() dirty=() hit=() fd=() gone=() cr=() ci=() ign=()
  claimgen "$LEASES/.cleaning" || { changed_leases; return 0; }
  for f in "$LEASES"/*/owner; do   # lr rel, lw "name\037pid\037since" of a live owner ("" = dead), ld folder, lp pid\037since
    # also at most 5000 (their paths go to one git status as arguments, under ARG_MAX while they average < ~190 bytes)
    [ $SECONDS -lt 2 ] && [ ${#lr[@]} -lt 5000 ] || { part=1; break; }   # ponytail: same glob order each scan; that many live leases ahead of the dead ones stall the drain
    { IFS=$US read -r O_SID O_PID O_START O_AID O_NAME O_SINCE O_PATH < "$f"; } 2>/dev/null || continue
    [ "$O_PID" = "$MYPID" ] && mine && continue
    case $AOK in   # the loop body runs per lease: no function call when this owner was already found alive
      *"|$O_PID $O_START|"*) lw+=("$O_NAME$US$O_PID$US$O_SINCE") ;;
      *) if alive "$O_PID" "$O_START"; then lw+=("$O_NAME$US$O_PID$US$O_SINCE"); else lw+=(""); fi ;;
    esac
    lr+=("$O_PATH") ld+=("${f%/owner}") lp+=("$O_PID$US$O_SINCE")
  done
  [ ${#lr[@]} -gt 0 ] || { $RM -f "$LEASES/.cleaning"; [ -z "$part" ] || changed_leases; return 0; }
  while IFS= read -r -d '' e; do
    [ "$e" = status-failed ] && { sfail=1; continue; }   # no "XY path" entry looks like this
    st=${e:0:2}; dirty+=("${e:3}")
    case $st in R*) IFS= read -r -d '' e; dirty+=("$e") ;; C*) IFS= read -r -d '' e ;; esac   # the source path follows
  done < <(GIT_OPTIONAL_LOCKS=0 $GIT -C "$ROOT" status --porcelain -z -uall -- "${lr[@]/#/:(literal)}" 2>/dev/null || printf 'status-failed\0')
  # leases on a changed path or on a folder holding one (folder leases of older versions)
  [ ${#dirty[@]} -gt 0 ] && for i in $( { printf '%s\n' "${dirty[@]}"; printf '\001\n'; printf '%s\n' "${lr[@]}"; } |
      LC_ALL=C $AWK_BIN 's { if ($0 in d) print NR - s - 1; next } $0 == "\001" { s = NR; next } { d[$0]; while (sub(/\/[^\/]*$/, "")) d[$0] }'); do
    hit[i]=1; [ -n "${lw[i]}" ] && fd+=("${lr[i]}$US${lw[i]}")
  done
  # no status of the leased paths: unknown which files are clean, delete nothing this time (lock released first: the
  # changed-paths way blocks when git status fails there too)
  [ -n "$sfail" ] && { $RM -f "$LEASES/.cleaning"; changed_leases; return 0; }
  if [ -n "$part" ]; then changed_leases   # not every lease read
  else [ ${#fd[@]} -gt 0 ] && printf -v FDIRTY '%s\n' "${fd[@]}"
  fi
  : > "$LEASES/.idle" && $TOUCH -A -1000 "$LEASES/.idle" || $RM -f "$LEASES/.idle"   # mtime 10 minutes ago, or no idle rule
  for i in "${!lr[@]}"; do   # ponytail: at most 1000 deletions per scan (a pile-up drains over a few scans)
    [ ${#gone[@]} -ge 1000 ] || [ $SECONDS -ge 3 ] && break
    [ -n "${hit[i]}" ] && continue   # keeps uncommitted work, a dead owner's too
    if [ -n "${lw[i]}" ] && ! [ "${ld[i]}/owner" -ot "$LEASES/.idle" ]; then cr+=("${lr[i]}") ci+=("$i"); continue; fi
    # the take-over's claim, so never both; a refused claim is skipped without a fork (S1) unless 10 minutes old (leftover)
    c=${lp[i]#*$US}; c=${ld[i]}/claim-${lp[i]%%$US*}-${c//[^0-9]/}
    claim "$c" || { [ "$c" -ot "$LEASES/.idle" ] && claimgen "$c"; } || continue
    gone+=("${ld[i]}")
  done
  if [ ${#cr[@]} -gt 0 ] && [ ${#gone[@]} -lt 1000 ] && [ $SECONDS -lt 3 ]; then   # a live owner's recent lease goes only when the file is ignored
    while IFS= read -r -d '' e; do ign+=("$e"); done < <(printf '%s\0' "${cr[@]}" | $GIT -C "$ROOT" check-ignore -z --stdin 2>/dev/null)
    j=0
    for k in "${!cr[@]}"; do   # check-ignore prints the ignored ones in input order
      [ "${cr[k]}" = "${ign[j]}" ] || continue
      j=$((j + 1)) i=${ci[k]}
      c=${lp[i]#*$US}; claim "${ld[i]}/claim-${lp[i]%%$US*}-${c//[^0-9]/}" && gone+=("${ld[i]}")
    done
  fi
  [ ${#gone[@]} -gt 0 ] || { $RM -f "$LEASES/.cleaning"; return 0; }
  # One rename moves them all (and the cleaning claim) off their paths now, so nothing is left as a claimed-but-undeleted
  # lease if the deletion dies; only the trash folder (not scanned: dot name) is deleted in the background.
  local trash=$LEASES/.trash-$$
  $MKDIR -p "$trash" && $MV "${gone[@]}" "$LEASES/.cleaning" "$trash/" 2>/dev/null
  $RM -rf "$trash" </dev/null >/dev/null 2>&1 &
}

# hc ls: live agent CLIs of this machine in ROOT (allow-list; dev servers, builds and shells are not agents)
hc_who() {
  command -v hc >/dev/null 2>&1 || return 0
  local addr agent cwd name
  while IFS=$US read -r addr agent cwd name; do
    [ -n "$addr" ] && [ "$addr" != "$HC_TERMINAL_ID" ] || continue
    case $agent in claude*|codex*|agy|grok*|cursor-agent|gemini*|opencode) ;; *) continue ;; esac
    intree "$cwd" && WHO="$WHO, ${name:-$agent}[$addr]"
  done <<EOF
$($PERL -e 'alarm 2; exec @ARGV' hc ls --json 2>/dev/null | $JQ -r '.terminals[]? | select(.state == "live" and (.address | contains("/") | not)) | [.address, .agent, .cwd, .name] | map(. // "" | tostring) | join("\u001f")' 2>/dev/null)
EOF
}

# cotenant: another live agent shares ROOT — a registered Claude whose cwd is in it, an hc agent terminal there,
# or a live lease holder with uncommitted changes in it. WHO names them.
cotenant() {
  [ "$C_ROOT" = "$ROOT" ] && return $C_RC
  C_ROOT=$ROOT WHO=
  me
  local pid sid st cwd name rel rest
  while IFS=$US read -r pid sid st cwd name; do
    [ -n "$pid" ] && [ "$pid" != "$MYPID" ] || continue
    intree "$cwd" && alive "$pid" "$st" && WHO="$WHO, $name(pid $pid)"
  done <<EOF
$REG
EOF
  [ -n "$WHO" ] || hc_who
  if [ -z "$WHO" ]; then
    scan_foreign
    while IFS=$US read -r rel name pid rest; do [ -n "$rel" ] && WHO="$WHO, $name(pid $pid)"; done <<EOF
$FDIRTY
EOF
  fi
  WHO=${WHO#, }
  [ -n "$WHO" ]; C_RC=$?
  return $C_RC
}

lines() { local l out=; while IFS=$US read -r l rest; do [ -n "$l" ] && out="$out, $l(${rest%%$US*})"; done <<EOF
$1
EOF
  R=${out#, }; }

# foreign_under PREFIX [PATTERN]: block with MSG when someone else's uncommitted leased file is PREFIX, below it,
# or (PATTERN=1) matches PREFIX as a glob (* also crosses /, like git pathspecs). Empty PREFIX = the whole tree.
foreign_under() {
  scan_foreign
  local f rest
  while IFS=$US read -r f rest; do
    [ -n "$f" ] || continue
    if [ -z "$1" ]; then :
    elif [ "$2" = 1 ]; then case $f in $1|$1/*) ;; *) continue ;; esac
    else case $f in "$1"|"$1"/*) ;; *) continue ;; esac
    fi
    block "$f 는 ${rest%%$US*}이 고치는 중인 파일(미커밋)이다. $MSG"
  done <<EOF
$FDIRTY
EOF
}

bash_git() {   # KIND DIR ARG
  [ "$1" = FORCEPUSH ] && block "'$3' 은 강제 push다. 원격에 있던 남의 커밋을 지울 수 있어 동거와 상관없이 늘 막는다. 대신 새 이름의 참조로 push하거나(git push origin HEAD:refs/<새 이름>), 그 참조를 git push origin --delete <참조> 로 지운 뒤 다시 push하라. 기대값을 적은 --force-with-lease=<참조>:<sha>(없을 때만 만들기는 <참조>: 로 빈 값)는 비교 후 교체라 된다. 그 참조를 다른 레인·사람이 쓰거나 판단이 서지 않으면 지휘(또는 사람)에게 알려라. 이 규칙을 끄는 것은 사람만 한다."
  phys "$2" || return 0
  tree "$PD" || return 0
  local wt="../${ROOT##*/}.wt/land-<슬러그>" MSG
  case $1 in
    ADDALL|COMMITALL|COMMITSTAGED|AMEND|DISCARD|STASH|RESETHARD|CLEAN|DYN)
      cotenant || return 0
      case $1 in
        ADDALL|COMMITALL) block "'$3'은 같은 작업 트리의 다른 에이전트($WHO) 변경까지 담는다. 네 파일만 경로로 지정하라(git add -- <경로>), 커밋 전 git diff --cached로 확인." ;;
        # S5: checking the stage first left a gap until the commit ran, in which another agent could stage its file
        COMMITSTAGED) block "경로 없는 '$3'은 같은 작업 트리의 다른 에이전트($WHO)가 그 사이 stage한 파일까지 커밋할 수 있어 막았다. 네 파일만 경로로 지정하라: git commit -m … -- <경로>" ;;
        AMEND) block "'$3'은 같은 작업 트리의 다른 에이전트($WHO)가 그 사이 만든 커밋을 고쳐 쓸 수 있어 막았다. 새 커밋을 만들어라: git commit -m … -- <경로>" ;;
        DYN) block "'$3'은 무엇을 담을지 미리 알 수 없어 같은 작업 트리에 다른 에이전트($WHO)가 있는 동안 막는다. 네 파일 경로를 글자로 적어라(git add -- <경로>)." ;;
        *) block "'$3'은 같은 작업 트리의 다른 에이전트($WHO) 미커밋 변경을 지우거나 숨길 수 있어 막았다. 네 파일만 되돌리려면 그 파일을 Edit로 고치고, 트리 전체를 다루는 일은 자기 워크트리(git worktree add --detach $wt HEAD)에서 하라." ;;
      esac ;;
    ADD|DISCARDPATH)
      local a=$3 top= glob= base
      local verb=add; [ "$1" = DISCARDPATH ] && verb=checkout
      case $a in *'$'*) bash_git DYN "$2" "git $verb $a"; return 0 ;; esac
      case $a in   # pathspec magic: :(glob|top|literal|exclude…)x, :/x, :!x
        ':('*')'*) local m=${a%%\)*}; m=${m#:\(}; a=${a#*\)}
                   case $m in *exclude*) return 0 ;; esac
                   case $m in *top*) top=1 ;; esac
                   case $m in *literal*) ;; *) case $a in *'*'*|*'?'*|*'['*) glob=1 ;; esac ;; esac ;;
        ':!'*|':^'*) return 0 ;;
        ':/'*) a=${a#:/}; top=1 ;;
        *'*'*|*'?'*|*'['*) glob=1 ;;
      esac
      MSG="git add·checkout·commit에 넣지 말고 네 파일만 경로로 지정하라. 그 파일을 커밋하려면 그쪽이 직접 커밋하게 하거나, 그 에이전트 프로세스가 끝나 임대가 풀린 뒤에 하라."
      if [ -n "$glob" ]; then
        base=; [ -n "$top" ] || { base=${PD#"$ROOT"}; base=${base#/}; }
        a=${a#./}
        foreign_under "${base:+$base/}$a" 1
        return 0
      fi
      [ -n "$top" ] && a=$ROOT/$a
      case $a in /*) ;; *) a=$PD/$a ;; esac
      relof "$a" || return 0
      foreign_under "$REL" ;;
    LAND)
      scan_foreign
      [ -n "$FDIRTY" ] || return 0
      lines "$FDIRTY"
      block "다른 에이전트의 미커밋 파일($R)이 있는 작업 트리라 git $3 을 막았다. 커밋·rebase·검증·push(착지)는 따로 연 폴더에서 하라. 내 커밋만 올리려면 git worktree add --detach $wt origin/<기본 브랜치> 뒤 그 폴더에서 git cherry-pick <내 커밋…> → 검증 → push. HEAD 기준(…add --detach $wt HEAD)은 이 폴더의 미push 커밋을 모두 올리니 지휘가 모아 착지할 때만." ;;
  esac
}

bash_write() {   # DIR ARG [noglob — 글롭이 펼친 구체 경로: 다시 펼치지 않는다]
  local a=$2 f MSG
  case $a in ''|*'$'*|-) return 0 ;; esac
  case $a in /*) ;; *) a=$1/$a ;; esac
  [ "${3-}" = noglob ] || case $a in
    *'*'*|*'?'*|*'['*)
      local -a m; shopt -s nullglob; m=($a); shopt -u nullglob
      for f in "${m[@]}"; do bash_write / "$f" noglob; done; return 0 ;;
  esac
  phys "$a" || return 0
  tree "$PD" || return 0
  FULL=$PD${PREST:+/$PREST}
  if [ "$FULL" = "$ROOT" ]; then REL=
  elif [ "${FULL#"$ROOT"/}" != "$FULL" ]; then REL=${FULL#"$ROOT"/}
  else return 0
  fi
  if [ -d "$FULL" ]; then   # a folder (or the whole tree): no lease, refuse when someone else's dirty file is in it
    MSG="'$2' 는 ${REL:-작업 트리 전체} 폴더를 바꾸므로 막았다. 그 파일을 빼고 네 파일만 경로로 지정하라."
    foreign_under "$REL"
    return 0
  fi
  lease "$REL" check
  PEND="$PEND$PD$US$REL"$'\n'   # leased by check_bash once the whole command has passed
}

# analyze CMD CWD DEPTH [REMOTE]: findings of the tokenizer below; SUB = a string run by bash -c / eval, analyzed again;
# RSUB = a command run on another machine (hc exec|run … --, ssh <host> …): only the force-push rule applies there
# (its paths are not this machine's trees), REMOTE=1 for it and everything nested in it
analyze() {
  local out kind dir arg
  out=$(CG_CWD=$2 LC_ALL=C $AWK_BIN "$AWK" <<<"$1") || { echo "cotenant-guard: Bash 명령 분석 실패" >&2; exit 1; }
  while IFS=$US read -r kind dir arg; do
    case $kind in
      FORCEPUSH) bash_git "$kind" "$dir" "$arg" ;;
      SUB|RSUB) [ "$3" -lt 5 ] || block "명령 겹침이 5겹을 넘어(bash -c·ssh·hc exec 등) 강제 push 여부를 판정할 수 없어 막았다. 겹침을 줄여 다시 하라."
        analyze "${arg//$'\036'/$'\n'}" "$dir" $(( $3 + 1 )) "$([ $kind = RSUB ] && echo 1 || echo "${4-}")" ;;
      WRITE) [ "${4-}" = 1 ] || bash_write "$dir" "$arg" ;;
      ?*) [ "${4-}" = 1 ] || bash_git "$kind" "$dir" "$arg" ;;
    esac
  done <<EOF
$out
EOF
}

check_bash() {
  local re='git|sed|cp|mv|rm|tee|unlink|>'
  [[ $CMD =~ $re ]] || return 0
  PEND=
  analyze "$CMD" "$HCWD" 0
  local pd rel   # nothing refused the command: record leases for its file targets
  while IFS=$US read -r pd rel; do [ -n "$rel" ] && tree "$pd" && lease "$rel" 0; done <<EOF
$PEND
EOF
}

# Shell tokenizer: quotes, escapes, $( ), separators, redirections and heredoc bodies (also a heredoc inside
# "$( )"); follows cd and git -C; looks through env, timeout, nice, xargs, bash/sh/zsh -c and eval.
# Prints KIND\037DIR\037ARG per finding (newlines in ARG as \036). Long quoted words are cut at 1024 bytes.
# Bytes, not characters (split(s, ch, "") is per byte): run under LC_ALL=C, or a cut mid-character makes the UTF-8 regex
# fail (towc: multibyte conversion failure) and every long Korean command is refused. Every pattern here is ASCII.
AWK='
function word() {
  if (!intok) return
  if (mode == "w") emit("WRITE", cwd, tilde(tok))
  else if (mode == "d") { if (tok !~ /^[0-9]*-?$/) emit("WRITE", cwd, tilde(tok)) }
  else if (mode == "h") hd[++nhd] = tok
  else if (mode != "s") t[++nt] = tok
  mode = ""; tok = ""; intok = 0; tl = 0
}
function add(c) { if (tl < 1024) { tok = tok c; tl++ } intok = 1 }
function emit(k, d, a) { gsub(/\n/, "\036", a); printf "%s\037%s\037%s\n", k, d, a }
function join(b, p) { return (p ~ /^\//) ? p : b "/" p }
function tilde(p) { if (p == "~") return home; if (substr(p, 1, 2) == "~/") return home substr(p, 2); return p }
function redir(   c) {
  c = ch[i]
  if (intok && mode == "" && tok ~ /^[0-9]+$/) { tok = ""; intok = 0; tl = 0 } else word()
  if (c == ">") {
    if (ch[i+1] == ">" || ch[i+1] == "|") i++
    if (ch[i+1] == "&") { i++; mode = "d" } else mode = "w"
  } else if (ch[i+1] == "<") {
    i++
    if (ch[i+1] == "<") { i++; mode = "s" } else { if (ch[i+1] == "-") i++; mode = "h" }
  } else if (ch[i+1] == "&") { i++; mode = "s" }
  else if (ch[i+1] == ">") { i++; mode = "w" }
  else mode = "s"
}
function skipto(p, w,   s, e, j, line) {   # from the newline at p, skip body lines through the line equal to w
  for (;;) {
    s = p + 1; if (s > n) return -1
    e = s; while (e <= n && ch[e] != "\n") e++
    p = e
    if (e - s <= length(w) + 16) {
      line = ""; for (j = s; j < e; j++) line = line ch[j]
      sub(/^[ \t]+/, "", line); sub(/[ \t]+$/, "", line)
      if (line == w) return p
    }
  }
}
function heredocs(p,   k) {
  for (k = 1; k <= nhd; k++) { p = skipto(p, hd[k]); if (p < 0) { p = n; break } }   # unterminated: body to the end, as bash
  nhd = 0
  return p
}
function qheredoc(p,   j, w, e, c) {   # "<<" inside "$( )": skip the body (odd quotes in it do not count); -1 = no heredoc here
  j = p + 2; if (ch[j] == "-") j++
  while (ch[j] == " " || ch[j] == "\t") j++
  if (ch[j] == "\047" || ch[j] == "\"") j++
  w = ""
  while (j <= n) {   # the delimiter runs to a blank, quote or one of <>;&|()
    c = ch[j]
    if (c == " " || c == "\t" || c == "\n" || c == "\047" || c == "\"" || index("<>;&|()", c)) break
    w = w c; j++
  }
  if (w == "") return -1
  e = j; while (e <= n && ch[e] != "\n") e++
  if (e > n) return -1
  return skipto(e, w)   # -1 when the delimiter line never comes: then it was not a heredoc
}
function nonopt(k, last,   j, a, dd) {
  for (j = k + 1; j <= nt; j++) {
    a = t[j]
    if (!dd && a == "--") { dd = 1; continue }
    if (!dd && a ~ /^-/) continue
    if (last && j < nt) continue
    emit("WRITE", cwd, tilde(a))
  }
}
function sedi(k,   j, a, ip, script) {
  for (j = k + 1; j <= nt; j++) if (t[j] ~ /^-[A-Za-z]*i/ || t[j] ~ /^--in-place/) ip = 1
  if (!ip) return
  script = 1
  for (j = k + 1; j <= nt; j++) {
    a = t[j]
    if (a == "-e" || a == "-f" || a == "--expression" || a == "--file") { j++; script = 0; continue }
    if (a ~ /^-/ || a == "") continue
    if (script) { script = 0; continue }
    emit("WRITE", cwd, tilde(a))
  }
}
function git(k,   j, d, o, s, a, q, c2, dd, wt, st, dry, x, all, np, lz, fi, an, ab) {
  d = cwd; j = k + 1; split("", AL)
  while (j <= nt && t[j] ~ /^-/) {
    o = t[j]
    if (o == "-C") { d = join(d, tilde(t[j+1])); j += 2; continue }
    if (o == "-c") {   # a one-off remote config that makes a plain push forced; a one-off alias is expanded below
      x = t[j+1]
      if ((x ~ /^remote\..*\.mirror=/ && x !~ /=(false|no|off|0)$/) || x ~ /^remote\..*\.push=\+/) { emit("FORCEPUSH", d, "git -c " x); return }
      if (x ~ /^alias\.[^=]+=/) { an = substr(x, 7); sub(/=.*/, "", an); AL[tolower(an)] = substr(x, index(x, "=") + 1) }   # git keys ignore case, the last wins
    }
    if (o == "-c" || o == "--git-dir" || o == "--work-tree" || o == "--namespace" || o == "--config-env") { j += 2; continue }
    j++
  }
  if (j > nt) return
  s = t[j]
  if (tolower(s) in AL) {   # git -c alias.<s>=<body> <s> args: analyze "git <body> args" (or the "!" shell line)
    ab = AL[tolower(s)]; x = (ab ~ /^!/) ? substr(ab, 2) : "git " ab
    for (a = j + 1; a <= nt; a++) x = x " " sq(t[a])
    emit("SUB", d, x); return
  }
  if (s == "add") {
    if (subnext || viax) emit("DYN", d, "git add " (viax ? "(xargs)" : "$(…)"))
    for (a = j + 1; a <= nt; a++) {
      o = t[a]
      if (!dd && o == "--") { dd = 1; continue }
      if (!dd && o ~ /^-/) {
        if (o == "--all" || o == "--update" || o == "--no-ignore-removal" || (o ~ /^-[^-]/ && o ~ /[Au]/)) emit("ADDALL", d, "git add " o)
        continue
      }
      if (o == "." || o == "*" || o ~ /^:(\/|\(top\))?$/) emit("ADDALL", d, "git add " o)
      else emit("ADD", d, tilde(o))
    }
  } else if (s == "commit") {
    for (a = j + 1; a <= nt; a++) {
      o = t[a]
      if (!dd && o == "--") { dd = 1; continue }
      if (!dd && o ~ /^--/) {
        if (o == "--all") { all = 1; emit("COMMITALL", d, "git commit --all") }
        if (o ~ /^--am(e(n(d)?)?)?$/) emit("AMEND", d, "git commit --amend")
        if (o ~ /^--(message|file|author|date|fixup|squash|cleanup|trailer|template|reuse-message|reedit-message|pathspec-from-file)$/) a++
        continue
      }
      if (!dd && o ~ /^-./) {
        for (q = 2; q <= length(o); q++) {
          c2 = substr(o, q, 1)
          if (c2 == "a") { all = 1; emit("COMMITALL", d, "git commit " o); break }
          if (index("mFCct", c2)) { if (q == length(o)) a++; break }
        }
        continue
      }
      np++; emit("ADD", d, tilde(o))
    }
    if (!all && !np) emit("COMMITSTAGED", d, "git commit")
  } else if (s == "checkout") {
    for (a = j + 1; a <= nt; a++) {
      o = t[a]
      if (o == "--") { emit("DISCARD", d, "git checkout --"); return }
      if (o ~ /^-/) {
        if (o == "-f" || o == "--force" || o == "-p" || o == "--patch") emit("DISCARD", d, "git checkout " o)
        if (o == "-b" || o == "-B" || o == "--orphan") { emit("LAND", d, "checkout " o); a++ }
        continue
      }
      if (o == ".") { emit("DISCARD", d, "git checkout ."); continue }
      emit("DISCARDPATH", d, tilde(o)); emit("LAND", d, "checkout " o)
    }
  } else if (s == "switch") {
    for (a = j + 1; a <= nt; a++) if (t[a] == "-f" || t[a] == "--force" || t[a] == "--discard-changes") { emit("DISCARD", d, "git switch " t[a]); return }
    emit("LAND", d, "switch")
  } else if (s == "restore") {
    for (a = j + 1; a <= nt; a++) {
      o = t[a]
      if (o == "--") break
      if (o == "--staged") st = 1
      else if (o == "--worktree") wt = 1
      else if (o ~ /^-[^-]/) { if (o ~ /S/) st = 1; if (o ~ /W/) wt = 1 }
    }
    if (wt || !st) emit("DISCARD", d, "git restore")
  } else if (s == "stash") {
    a = j + 1; while (a <= nt && t[a] ~ /^-/) a++
    x = t[a]
    if (x != "list" && x != "show") emit("STASH", d, "git stash" (x == "" ? "" : " " x))
  } else if (s == "reset") {
    for (a = j + 1; a <= nt; a++) if (t[a] == "--hard") emit("RESETHARD", d, "git reset --hard")
  } else if (s == "clean") {
    for (a = j + 1; a <= nt; a++) if (t[a] == "--dry-run" || (t[a] ~ /^-[^-]/ && t[a] ~ /n/)) dry = 1
    if (!dry) emit("CLEAN", d, "git clean")
  } else if (s == "rebase" || s == "merge" || s == "pull") {
    for (a = j + 1; a <= nt; a++) if (t[a] == "--abort" || t[a] == "--quit") return
    emit("LAND", d, s)
  } else if (s == "rm" || s == "mv") {
    x = cwd; cwd = d; nonopt(j, 0); cwd = x
  } else if (s == "push") {   # force forms (also a "+" refspec); --delete and :ref are not force
    # --force-with-lease=<ref>:<expect> (expect may be empty: "only if absent") is compare-and-swap, as --delete then push:
    # allowed (claims). Bare lease or <ref> without ":" leases against the tracking ref = blind. --force-if-includes only with it
    lz = 0; fi = ""
    for (a = j + 1; a <= nt; a++) {
      o = t[a]
      if (o ~ /^--force-with-lease=[^:]*:/) { lz = 1; continue }
      if (o == "--force-if-includes") { fi = o; continue }
      if (o ~ /^--(force|force-with-lease|mirror)(=|$)/) { emit("FORCEPUSH", d, "git push " o); return }
      if (o == "-o" || o == "--push-option" || o == "--repo" || o == "--receive-pack" || o == "--exec") { a++; continue }
      if (o ~ /^-[^-]/) {
        for (q = 2; q <= length(o); q++) {
          c2 = substr(o, q, 1)
          if (c2 == "f") { emit("FORCEPUSH", d, "git push " o); return }
          if (c2 == "o") { if (q == length(o)) a++; break }
        }
        continue
      }
      if (o ~ /^\+/) { emit("FORCEPUSH", d, "git push … " o); return }
    }
    if (fi != "" && !lz) emit("FORCEPUSH", d, "git push " fi " (기대값 적은 --force-with-lease 없이)")
  }
}
# optend(J, SHORTV, LONGV): getopt walk of the options of a wrapper from t[J]; returns the index of the first operand (the
# inner command). SHORTV = short options taking a value (getopt "p:"; -p22 and -tp 22 both), LONGV = " --long … " taking a
# value (--long=value is one word); "--" ends options. OPTON = --on seen, OPTCWD = --cwd value. Every wrapper (hc exec|run,
# ssh, sh -c) goes through this one walk, and analyze() repeats it for each nesting level
function optend(j, shortv, longv, plus,   o, c, q) {
  OPTON = 0; OPTCWD = ""
  for (; j <= nt; j++) {
    o = t[j]
    if (o == "--") return j + 1
    if (o !~ /^-./ && !(plus && o ~ /^\+./)) return j   # PLUS: +x +o v also options (sh family)
    if (o ~ /^--/) {
      c = o; sub(/=.*/, "", c)
      if (c == "--on") OPTON = 1
      if (c == "--cwd") OPTCWD = (o ~ /=/) ? substr(o, 7) : t[j+1]
      if (o !~ /=/ && index(longv, " " o " ")) j++
      continue
    }
    for (q = 2; q <= length(o); q++) if (index(shortv, substr(o, q, 1))) { if (q == length(o)) j++; break }
  }
  return j
}
function sq(s) {   # quote one argv word for re-parsing
  if (s ~ /^[A-Za-z0-9_.\/:=@%+,-]+$/) return s
  gsub(/\047/, "\047\\\047\047", s); return "\047" s "\047"
}
function cmd(   k, w, name, j, x, q) {
  mode = ""
  k = 1
  while (k <= nt) {
    w = t[k]
    if (w ~ /^(if|then|else|elif|do|while|until|time|!|\{|\}|exec|command|nohup|sudo|builtin)$/ || w ~ /^[A-Za-z_][A-Za-z0-9_]*=/) { k++; continue }
    if (w == "env") {
      k++
      while (k <= nt && (t[k] ~ /^-/ || t[k] ~ /^[A-Za-z_][A-Za-z0-9_]*=/)) { if (t[k] == "-u" || t[k] == "-C" || t[k] == "-S" || t[k] == "-P") k++; k++ }
      continue
    }
    if (w == "timeout" || w == "gtimeout") { k++; while (k <= nt && t[k] ~ /^-/) { if (t[k] == "-s" || t[k] == "-k") k++; k++ } k++; continue }
    if (w == "nice") { k++; while (k <= nt && t[k] ~ /^-/) { if (t[k] == "-n") k++; k++ } continue }
    if (w == "xargs") { k++; while (k <= nt && t[k] ~ /^-/) { if (t[k] ~ /^-[IJLnPsEd]$/) k++; k++ } viax = 1; continue }
    break
  }
  if (k <= nt) {
    name = t[k]; sub(/.*\//, "", name)
    if (name == "cd" || name == "pushd") { if (k == nt) cwd = home; else if (t[k+1] !~ /^-/) cwd = join(cwd, tilde(t[k+1])) }
    else if (name == "git") git(k)
    else if (name == "sed") sedi(k)
    else if (name == "cp") nonopt(k, 1)
    else if (name == "mv" || name == "rm" || name == "rmdir" || name == "unlink" || name == "tee") nonopt(k, 0)
    else if (name ~ /^(bash|sh|zsh|dash|ksh)$/) {   # [opts] -c <line>: the first operand is the line
      j = optend(k + 1, "oO", " --rcfile --init-file ", 1)
      for (q = k + 1; q < j; q++) if (t[q] ~ /^-[A-Za-z]*c[A-Za-z]*$/) { if (j <= nt) emit("SUB", cwd, t[j]); break }
    } else if (name == "eval") { x = ""; for (j = k + 1; j <= nt; j++) x = x (j > k + 1 ? " " : "") t[j]; emit("SUB", cwd, x) }
    else if (name == "hc" && (t[k+1] == "exec" || t[k+1] == "run" || (t[k+1] == "--json" && (t[k+2] == "exec" || t[k+2] == "run")))) {   # hc [--json] exec|run [OPTIONS] <ARGV>… (hc … --help)
      if (t[k+1] == "--json") k++
      j = optend(k + 2, "", " --on --cwd --env --timeout --name --label --parent ")
      if (j > nt) x = ""
      else if (t[k+1] == "run" && j == nt) x = t[j]   # hc run with one word runs it as a shell line
      else { x = ""; for (q = j; q <= nt; q++) x = x (q > j ? " " : "") sq(t[q]) }   # argv as given, no shell
      if (x != "") emit(OPTON ? "RSUB" : "SUB", OPTCWD == "" ? cwd : join(cwd, tilde(OPTCWD)), x)
    } else if (name == "ssh") {   # ssh [options] destination [command…] (ssh(1)): the remote shell parses the words joined by blanks
      j = optend(k + 1, "BbcDEeFIiJLlmOoPpQRSWw", "") + 1
      if (t[j] == "--") j++
      x = ""; for (q = j; q <= nt; q++) x = x (q > j ? " " : "") t[q]
      if (x != "") emit("RSUB", cwd, x)
    }
  }
  nt = 0; viax = 0; split("", t)
}
BEGIN { RS = "\001" }
{ src = src $0 }
END {
  n = split(src, ch, "")
  cwd = ENVIRON["CG_CWD"]; home = ENVIRON["HOME"]
  for (i = 1; i <= n; i++) {
    c = ch[i]
    if (c == "\\") { if (i < n) { i++; if (ch[i] != "\n") add(ch[i]) } continue }
    if (c == "\047") { i++; while (i <= n && ch[i] != "\047") { add(ch[i]); i++ } intok = 1; continue }
    if (c == "\"") {
      i++; dsub = 0
      while (i <= n && ch[i] != "\"") {
        if (ch[i] == "\\" && i < n) { i++; add(ch[i]); i++; continue }
        if (ch[i] == "$" && ch[i+1] == "(") dsub = 1
        if (dsub && ch[i] == "<" && ch[i+1] == "<" && ch[i+2] != "<") { r = qheredoc(i); if (r > 0) { i = r; continue } }
        add(ch[i]); i++
      }
      intok = 1; continue
    }
    if (c == "$" && ch[i+1] == "(") { word(); subnext = 1; cmd(); subnext = 0; i++; continue }
    if (c == "$" && ch[i+1] == "{") { while (i <= n && ch[i] != "}") { add(ch[i]); i++ } add("}"); continue }
    if (c == " " || c == "\t") { word(); continue }
    if (c == "#" && !intok) { while (i < n && ch[i+1] != "\n") i++; continue }
    if (c == "\n") { word(); cmd(); if (nhd) i = heredocs(i); continue }
    if (c == ">" || c == "<") { redir(); continue }
    if (c == "&" && ch[i+1] == ">") { word(); i++; if (ch[i+1] == ">") i++; mode = "w"; continue }
    if (c == "`") { word(); bt = !bt; subnext = bt; cmd(); subnext = 0; continue }
    if (c == ";" || c == "&" || c == "|" || c == "(" || c == ")") { word(); cmd(); continue }
    add(c)
  }
  word(); cmd()
}'

case $TOOL in
  Edit|Write|MultiEdit|NotebookEdit) [ -n "$FP" ] && relof "$FP" && lease "$REL" 1 ;;
  Bash) [ -n "$CMD" ] && check_bash ;;
esac
if [ -n "$WARN" ]; then
  $JQ -cn --arg c "cotenant-guard: $WARN" '{hookSpecificOutput: {hookEventName: "PreToolUse", additionalContext: $c}}'
fi
exit 0
