#!/bin/bash
# 병합기와 착지의 중단·재시도 시험. 인자 하나로 해당 사례만 돌릴 수 있다.
set -eu
here=$(cd "$(dirname "$0")" && pwd -P)
lane_bin=${HC_LANE_BIN:-$here/../bin/hc-lane}
T=$(mktemp -d "${TMPDIR:-/tmp}/hc-merger-recovery.XXXXXX")
T=$(cd "$T" && pwd -P)
trap 'rm -rf "$T"' EXIT
. "$here/isolate.sh"
unset HC_LANE HC_TERMINAL_ID HC_LANE_OWNER HC_LANE_JOB HC_LANE_LEASE HERDR_MERGER HERDR_LAND_PUSH_EXTRA HC_FILL_VERDICTS
unset HC_LANE_MERGER_BATCH HERDR_LAND_PATCHID HERDR_LAND_SHA HERDR_LAND_VERDICT HERDR_LAND_AFTER
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.org GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.org
export HC_LANE_MACHINE=box LC_ALL=C
real_git=$(command -v git)
export real_git
mkdir "$T/bin"
printf '#!/bin/sh\ncase $1 in ls) echo '\''{"terminals":[]}'\'';; *) exit 3;; esac\n' >"$T/bin/hc"
chmod +x "$T/bin/hc"
export PATH="$T/bin:$PATH"

fixture() {
  mkdir "$T/$1"
  cd "$T/$1"
  export HOME=$PWD TMPDIR=$PWD
  origin=$PWD/origin.git writer=$PWD/writer merger=$PWD/merger
  git init -q --bare -b main "$origin"
  git init -q -b main "$writer"
  cd "$writer"
  mkdir .claude
  echo 'exit 0' >.claude/herdr-verify.sh
  echo base >shared
  git add -- .claude shared
  git commit -qm base
  git remote add origin "$origin"
  git push -q origin main
  git clone -q "$origin" "$merger"
  git checkout -qb lane
}
commit_file() {
  printf '%s\n' "$2" >"$1"
  git add -- "$1"
  git commit -qm "$1"
}
og() { git --git-dir="$origin" "$@"; }
enable() { og update-ref refs/herdr/merger/box main; }
submit() { HC_LANE=$1 "$lane_bin" land main >"$HOME/submit.log" 2>&1; }
tick() { "$lane_bin" merger --repo "$merger" --once main >"$HOME/tick.log" 2>&1; }
launch_response() {
  fixture launch-response
  mkdir "$HOME/stub"
  cat >"$HOME/stub/hc" <<'SH'
#!/bin/sh
case $1 in
  ls) echo '{"terminals":[]}' ;;
  get) exit 3 ;;
  run)
    if [ "$mode" = refused ]; then echo '{"error":{"kind":"name_taken"}}' >&2; exit 1; fi
    while [ "$1" != --cwd ]; do shift; done
    echo 작성중 >"$2/draft"
    echo '응답이 끊겼다' >&2
    exit 1 ;;
  *) exit 1 ;;
esac
SH
  chmod +x "$HOME/stub/hc"
  for mode in lost refused; do
    export mode
    if PATH="$HOME/stub:$PATH" "$lane_bin" --name "$mode" --repo "$writer" --start-wait 0 -- true >"$HOME/log" 2>&1; then return 1; fi
    if [ "$mode" = lost ]; then
      test -f "$writer.wt/$mode/draft"
      test "$(cat "$writer.wt/$mode/draft")" = 작성중
      test -n "$(og for-each-ref "refs/hc-lanes/$mode")"
    else
      test ! -e "$writer.wt/$mode"
      test -z "$(git for-each-ref "refs/heads/$mode")"
    fi
  done
}
installed_test_paths() {
  fixture installed
  prefix=$HOME/tools
  mkdir -p "$prefix/bin/tests" "$prefix/lib"
  cp -R "$here/../lib/hc-lane" "$prefix/lib/"
  cp "$lane_bin" "$prefix/bin/hc-lane"
  for name in hc-lane hc-lane-merger; do
    # 배포된 시험의 준비 부분만 실행한다. 첫 임시 저장소를 만들기 전에 경로와 격리를 확인한다.
    sed '/^W=/,$d' "$here/$name.test.sh" >"$prefix/bin/tests/$name.test.sh"
    cat >>"$prefix/bin/tests/$name.test.sh" <<'SH'
test "$LANE" -ef "$here/../hc-lane" && test "$PWD" = "$T" && test -z "${GIT_DIR+x}" || exit 1
if ( : >&18 ) 2>/dev/null || ( : >&19 ) 2>/dev/null; then exit 1; fi
SH
    (unset HC_LANE_BIN; GIT_DIR="$writer/.git" bash "$prefix/bin/tests/$name.test.sh" 18>/dev/null 19>/dev/null)
  done
}
approval_location() {
  for state in same advanced; do
    fixture "location-$state"
    git checkout -q main
    commit_file .claude/herdr-verify.sh 'test ! -e "$HOME/failverify"'
    file='작업 규칙'
    printf '%s\n' 첫째 이전 사이1 사이2 사이3 사이4 둘째 이전 끝1 끝2 끝3 끝4 >"$file"
    git add -- "$file"
    git commit -qm 규칙
    git push -q origin main
    git checkout -q -B lane main
    sed '2s/이전/이후/' "$file" >edit && mv edit "$file"
    git commit -qam '첫째 수정 승인'
    approved=$(git rev-parse HEAD)
    export HC_FILL_VERDICTS=$HOME/verdicts HC_LANE=lane HC_LANE_JOB=run-1
    mkdir -p "$HC_FILL_VERDICTS/lane"
    printf 'aid\ta1\nlane\tlane\nrun\trun-1\nsha\t%s\nverdict\tpass\nbase\tmain\n' "$approved" >"$HC_FILL_VERDICTS/lane/a1.md"
    if [ "$state" = advanced ]; then
      git checkout -q main
      echo 새끝 >>"$file"
      git commit -qam '같은 파일의 독립 변경'
      git push -q origin main
      git checkout -q lane
    fi
    base=$(og rev-parse main)
    touch "$HOME/failverify"
    if "$lane_bin" land main --verdict a1 >"$HOME/log" 2>&1; then return 1; fi
    grep -q '검증 실패' "$HOME/log"
    good=$(git rev-parse HEAD)
    sed -e '2s/이후/이전/' -e '8s/이전/이후/' "$file" >edit && mv edit "$file"
    git commit -qam '둘째 수정으로 이동'
    rm "$HOME/failverify"
    rc=0; "$lane_bin" land main --verdict a1 >>"$HOME/log" 2>&1 || rc=$?
    test "$rc" = 2
    test "$(og rev-parse main)" = "$base"
    git checkout -q -B lane "$good"
    "$lane_bin" land main --verdict a1 >>"$HOME/log" 2>&1
    test "$(og rev-parse main)" = "$good"
  done
}
clean_submission_owner() {
  fixture clean-submission
  mkdir "$HOME/stub"
  cat >"$HOME/stub/hc" <<'SH'
#!/bin/sh
case $1 in run) echo '{"address":"t11"}' ;; ls) echo '{"terminals":[]}' ;; *) exit 3 ;; esac
SH
  chmod +x "$HOME/stub/hc"
  PATH="$HOME/stub:$PATH" "$lane_bin" --name submitted --branch alias --repo "$writer" --start-wait 0 -- true >"$HOME/log" 2>&1
  wt=$writer.wt/alias
  (cd "$wt" && commit_file 작업 보존)
  sha=$(git -C "$wt" rev-parse HEAD)
  git push -q origin "$sha:refs/heads/lane/other"
  if "$lane_bin" clean --repo "$writer" --branch alias >>"$HOME/log" 2>&1; then return 1; fi
  test "$(git -C "$wt" rev-parse HEAD)" = "$sha"
  test "$(cat "$wt/작업")" = 보존
  lease=$(git rev-parse refs/hc-lease-owners/alias)
  (cd "$wt" && HC_LANE=submitted HC_LANE_LEASE=$lease "$lane_bin" park >>"$HOME/log" 2>&1)
  "$lane_bin" clean --repo "$writer" --branch alias >>"$HOME/log" 2>&1
  test ! -e "$wt"
  test "$(og rev-parse refs/heads/lane/submitted)" = "$sha"
}
clean_rebased_receipt() {
  fixture clean-rebased
  enable
  wt=$writer.wt/feature
  git worktree add -qb feature "$wt" main
  (cd "$wt" && commit_file 기능 보존 && submit feature)
  original=$(git -C "$wt" rev-parse HEAD)
  git checkout -q main
  commit_file independent yes
  git push -q origin main
  tick
  test "$(og show main:기능)" = 보존
  test "$(og rev-parse refs/merge-done/feature^)" = "$original"
  test -z "$(og for-each-ref refs/heads/lane/feature)"
  if og merge-base --is-ancestor "$original" main; then return 1; fi
  receipt=$(og rev-parse refs/merge-done/feature)
  og update-ref refs/merge-done/feature main
  if "$lane_bin" clean --repo "$writer" --branch feature >"$HOME/log" 2>&1; then return 1; fi
  test -f "$wt/기능"
  og update-ref refs/merge-done/feature "$receipt"
  "$lane_bin" clean --repo "$writer" --branch feature >>"$HOME/log" 2>&1
  test ! -e "$wt"
  test -z "$(git for-each-ref refs/heads/feature)"
}
journal_retry() {
  for outcome in accept reject; do
    fixture "journal-$outcome"
    enable
    if [ "$outcome" = reject ]; then commit_file .claude/herdr-verify.sh 'exit 1'; else commit_file payload yes; fi
    submit lane
    journal=$HOME/.local/state/hc-lane/merger-journal.tsv
    mkdir "$journal"
    rc=0; tick || rc=$?
    test "$rc" = 75
    if [ "$outcome" = accept ]; then
      test "$(og show main:payload)" = yes
      test -s "$merger/.git/hc-merger/cur"
      result=착지
    else
      test -n "$(og for-each-ref refs/merge-failed)"
      test -s "$merger"/.git/hc-merger/fail/*-lane
      result=돌려보냄
    fi
    rmdir "$journal"
    tick
    tick
    awk -F'\t' -v r="$result" '$3=="lane" && $10==r {n++} END {exit n!=1}' "$journal"
  done
}
guard_queries() {
  fixture guard
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
case "$fault:$*" in origin:*" remote"|hooks:*" config --get core.hooksPath") touch "$HOME/hit"; exit 128 ;; esac
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  for fault in origin hooks; do
    export fault
    if PATH="$HOME/stub:$PATH" "$lane_bin" guard install --repo "$writer" >"$HOME/log" 2>&1; then return 1; fi
    test -f "$HOME/hit"
    test ! -e .git/hooks/pre-push
    rm "$HOME/hit"
  done
  "$lane_bin" guard install --repo "$writer" >"$HOME/log" 2>&1
  "$lane_bin" guard status --repo "$writer" >>"$HOME/log" 2>&1
  test -x .git/hooks/pre-push
}
verdict_identity() {
  fixture identity
  commit_file payload yes
  sha=$(git rev-parse HEAD)
  base=$(og rev-parse main)
  export HC_LANE=lane HC_TERMINAL_ID=t11 HC_FILL_VERDICTS=$HOME/verdicts
  mkdir -p "$HC_FILL_VERDICTS/lane" "$HOME/stub"
  cat >"$HOME/stub/hc" <<'SH'
#!/bin/sh
case $1 in
  get) echo '{"state":"live","created_ms":11,"foreground":{"created_ms":22},"turn":1}'
       test "$fault" = ok ;;
  ls) echo '{"terminals":[]}' ;;
  *) exit 3 ;;
esac
SH
  chmod +x "$HOME/stub/hc"
  for fault in failed ok; do
    export fault
    printf 'aid\ta1\nlane\tlane\nrun\tt11@22\nsha\t%s\nverdict\tpass\nbase\tmain\n' "$sha" >"$HC_FILL_VERDICTS/lane/a1.md"
    if PATH="$HOME/stub:$PATH" "$lane_bin" land main --verdict a1 >"$HOME/log" 2>&1; then return 1; fi
    test ! -e "$HC_FILL_VERDICTS/lane/a1.ack"
    test "$(og rev-parse main)" = "$base"
    PATH="$HOME/stub:$PATH" "$lane_bin" report 완료 "$sha" >/dev/null 2>&1
    if [ "$fault" = failed ]; then expected=-; else expected=t11@11; fi
    test "$(tail -1 "$HOME/.local/state/hc-lane/events.tsv" | cut -f10)" = "$expected"
  done
  sed 's/t11@22/t11@11/' "$HC_FILL_VERDICTS/lane/a1.md" >"$HOME/right"
  mv "$HOME/right" "$HC_FILL_VERDICTS/lane/a1.md"
  PATH="$HOME/stub:$PATH" "$lane_bin" land main --verdict a1 >"$HOME/log" 2>&1
  test "$(og rev-parse main)" = "$sha"
}
postpush_cleanup() {
  fixture cleanup-push
  enable
  commit_file payload yes
  submit lane
  state=$HOME/.local/state/hc-lane
  journal=$merger/.git/hc-merger/cur
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
case "$1 $*" in "push "*:refs/hc-landed/lane/*) touch "$HOME/hit"; exit 128 ;; esac
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  for attempt in 1 2; do
    rc=0; PATH="$HOME/stub:$PATH" tick || rc=$?
    test -f "$HOME/hit"
    test "$rc" = 75
    test -n "$(og for-each-ref refs/merge-done/lane)"
    test -z "$(og for-each-ref refs/hc-landed/lane/)"
    test "$(og show main:payload)" = yes
    test -s "$journal"
    awk -F'\t' '$4=="lane" && $5=="착지" {exit 1}' "$state/events.tsv"
    rm "$HOME/hit"
  done
  cp "$journal" "$HOME/before-cleanup"
  tick
  cp "$HOME/before-cleanup" "$journal"
  tick
  test ! -f "$journal"
  test -z "$(og for-each-ref refs/heads/lane refs/merge-queue refs/merge-queue-meta)"
  awk -F'\t' '$4=="lane" && $5=="착지" {n++} END {exit n!=1}' "$state/events.tsv"
  awk -F'\t' '$3=="lane" && $10=="착지" {n++} END {exit n!=1}' "$state/merger-journal.tsv"
}
proof_ref() {
  fixture proof-ref
  enable
  commit_file payload yes
  HC_LANE_JOB=run-7 submit lane
  receipt=$(og for-each-ref --format='%(objectname)' refs/merge-queue-meta)
  sid=$(og show -s --format=%s "$receipt" | tr ' ' '\n' | sed -n 's/^id=//p')
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
[ "$1 $2" != "check-ref-format refs/hc-landed/$fault" ] || { touch "$HOME/hit"; exit 128; }
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  for fault in "$sid" run-7; do
    export fault
    rc=0; PATH="$HOME/stub:$PATH" tick || rc=$?
    test -f "$HOME/hit"
    test "$rc" = 75
    test "$(og rev-parse main)" = "$(git rev-parse main)"
    test -n "$(og for-each-ref refs/merge-queue)"
    rm "$HOME/hit"
  done
  tick
  for id in "$sid" run-7; do test "$(og rev-parse --verify "refs/hc-landed/$id")" = "$(og rev-parse main)"; done
  awk -F'\t' '$4=="lane" && $5=="착지" {n++} END {exit n!=1}' "$HOME/.local/state/hc-lane/events.tsv"
}
merge_resolution() {
  for mode in ahead diverged batch automatic; do
    fixture "merge-$mode"
    commit_file left yes
    git checkout -qb side main
    commit_file right yes
    git checkout -q lane
    git merge -q --no-commit --no-ff side
    if [ "$mode" != automatic ]; then echo resolved >resolution; git add -- resolution; fi
    git commit -qm '두 갈래 병합'
    original=$(git rev-parse HEAD)
    if [ "$mode" = ahead ]; then
      "$lane_bin" land main >"$HOME/log" 2>&1
      test "$(og rev-parse main)" = "$original"
      continue
    fi
    git checkout -q main
    commit_file independent yes
    git push -q origin main
    enable
    if [ "$mode" = batch ]; then git checkout -qb alpha; commit_file companion yes; submit alpha; fi
    git checkout -q lane
    submit zulu
    tick
    if [ "$mode" = automatic ]; then
      test "$(og show main:left)" = yes && test "$(og show main:right)" = yes
      test -n "$(og for-each-ref refs/merge-done/zulu)"
    else
      test "$(og rev-parse refs/heads/lane/zulu)" = "$original"
      test -z "$(og for-each-ref refs/merge-done/zulu refs/hc-landed/zulu/)"
      test -n "$(og for-each-ref 'refs/merge-failed/*-zulu')"
      if [ "$mode" = batch ]; then test "$(og show main:companion)" = yes; fi
    fi
  done
}
reverted_upstream() {
  for mode in single batch; do
    fixture "reapplied-$mode"
    enable
    commit_file shared wanted
    submit alpha
    git checkout -q main
    echo wanted >shared
    git commit -qam '다른 레인의 같은 변경'
    git revert --no-edit HEAD >/dev/null
    git push -q origin main
    if [ "$mode" = batch ]; then
      git checkout -qb zulu
      commit_file companion yes
      submit zulu
    fi
    tick
    test "$(og show main:shared)" = wanted
    test -z "$(og for-each-ref refs/merge-queue refs/merge-failed)"
    if [ "$mode" = batch ]; then test "$(og show main:companion)" = yes; fi
  done
}
resident_sleep() {
  fixture idle
  enable
  commit_file payload yes
  submit lane
  mkdir "$HOME/stub"
  cat >"$HOME/stub/sleep" <<'SH'
#!/bin/sh
# 실제 착지와 큐 삭제가 끝난 뒤의 첫 sleep 인자를 전달한다.
bare="$HOME/origin.git"
if [ "$("$real_git" --git-dir="$bare" show main:payload 2>/dev/null)" = yes ] &&
   [ -z "$("$real_git" --git-dir="$bare" for-each-ref refs/merge-queue)" ]; then
  printf '%s\n' "$*" >"$HOME/delay.tmp" && mv "$HOME/delay.tmp" "$HOME/delay"
  exec perl -e 'for (1..1200) { last if -e "$ENV{HOME}/release"; select undef,undef,undef,0.05 }'
fi
exec /bin/sleep "$@"
SH
  chmod +x "$HOME/stub/sleep"
  PATH="$HOME/stub:$PATH" "$lane_bin" merger --repo "$merger" --tick 17 >"$HOME/log" 2>&1 & resident=$!
  trap 'if jobs -pr | grep -Fxq "$resident"; then kill "$resident" 2>/dev/null || :; fi; touch "$HOME/release"; wait "$resident" 2>/dev/null || :' EXIT
  for ((i=0;i<1200;i++)); do
    [ ! -s "$HOME/delay" ] && kill -0 "$resident" 2>/dev/null || break
    sleep 0.05
  done
  test -s "$HOME/delay"
  test "$(cat "$HOME/delay")" = 17
}
receipt_race() {
  for phase in before after; do
    fixture "receipt-$phase"
    enable
    commit_file payload yes
    submit lane
    qref=$(og for-each-ref --format='%(refname)' refs/merge-queue)
    meta=refs/merge-queue-meta/${qref##*/}
    old=$(og rev-parse "$meta")
    original=$(og show -s --format=%s "$old" | tr ' ' '\n' | sed -n 's/^id=//p')
    sha=$(git rev-parse HEAD)
    next=$(og show -s --format=%s "$old" | sed "s/id=$original/id=new-receipt/" | og commit-tree "$old^{tree}" -p "$sha")
    export qref meta next sha phase
    mkdir "$HOME/stub"
    cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
case "$1 $*" in "push "*":refs/heads/main "*":refs/merge-queue/"*)
  if mkdir "$HOME/once" 2>/dev/null; then
    if [ "$phase" = after ]; then "$real_git" "$@" || exit 1; fi
    og() { "$real_git" --git-dir="$HOME/origin.git" "$@"; }
    og update-ref "$qref" "$sha" && og update-ref "$meta" "$next" && og update-ref refs/heads/lane/lane "$sha" || exit 1
    if [ "$phase" = after ]; then exit 0; fi
  fi ;;
esac
exec "$real_git" "$@"
SH
    chmod +x "$HOME/stub/git"
    PATH="$HOME/stub:$PATH" tick
    test -d "$HOME/once"
    test "$(og rev-parse refs/merge-done/lane)" = "$next"
    test "$(og rev-parse refs/hc-landed/new-receipt)" = "$sha"
    expect=0; [ "$phase" = before ] || expect=1
    awk -F'\t' -v id="$original" -v expected="$expect" '$5=="착지" && index($7,id) {n++} END {exit n+0!=expected}' "$HOME/.local/state/hc-lane/events.tsv"
    test -z "$(og for-each-ref refs/merge-queue refs/merge-queue-meta refs/heads/lane)"
  done
}
landed_lease() {
  for mode in direct merger approved; do
    fixture "landed-$mode"
    old=$(git rev-parse main)
    commit_file payload yes
    landed=$(git rev-parse HEAD)
    git push -q origin "$old:refs/hc-lanes/lane" HEAD:main
    [ "$mode" != merger ] || enable
    flags=()
    if [ "$mode" = approved ]; then
      export HC_FILL_VERDICTS=$HOME/verdicts HC_LANE_JOB=run-1
      mkdir -p "$HC_FILL_VERDICTS/lane"
      printf 'aid\ta1\nlane\tlane\nrun\trun-1\nsha\t%s\nverdict\tpass\nbase\tmain\n' "$landed" >"$HC_FILL_VERDICTS/lane/a1.md"
      flags=(--verdict a1)
    fi
    HC_LANE=lane HC_LANE_LEASE=$old "$lane_bin" land main ${flags[0]+"${flags[@]}"} >"$HOME/log" 2>&1
    test -z "$(og for-each-ref refs/hc-lanes/lane)"
    test "$(og for-each-ref --format='%(objectname)' 'refs/hc-landed/lane/*')" = "$landed"
    git push -q origin "$landed:refs/hc-lanes/lane"
    HC_LANE=lane HC_LANE_LEASE=$old "$lane_bin" land main ${flags[0]+"${flags[@]}"} >>"$HOME/log" 2>&1
    test "$(og rev-parse refs/hc-lanes/lane)" = "$landed"
  done
}
default_query() {
  fixture default
  git push -q origin main:refs/heads/release
  git symbolic-ref refs/remotes/origin/HEAD refs/remotes/origin/release
  commit_file payload yes
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
case "$fault:$*" in
  default:*symbolic-ref*"refs/remotes/origin/HEAD"|default:*for-each-ref*"refs/remotes/origin/HEAD"|origin:*"remote get-url origin")
    touch "$HOME/hit"; exit 128 ;;
esac
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  rc=0; fault=default PATH="$HOME/stub:$PATH" "$lane_bin" land >"$HOME/log" 2>&1 || rc=$?
  test -f "$HOME/hit" && test "$rc" != 0
  test "$(og rev-parse main)" = "$(git rev-parse main)"
  rm "$HOME/hit"
  rc=0; fault=origin PATH="$HOME/stub:$PATH" "$lane_bin" clean --repo "$writer" --name absent >"$HOME/log" 2>&1 || rc=$?
  test -f "$HOME/hit" && test "$rc" != 0
  "$lane_bin" land >"$HOME/log" 2>&1
  test "$(og rev-parse release)" = "$(git rev-parse HEAD)"
  test "$(og rev-parse main)" = "$(git rev-parse main)"
}
rebase_retry() {
  fixture index-lock
  enable
  commit_file shared lane
  submit lane
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
if [ "$1" = rebase ] && [ "$2" = -q ]; then touch "$HOME/hit"; exit 128; fi
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  rc=0; PATH="$HOME/stub:$PATH" tick || rc=$?
  test -f "$HOME/hit"
  test "$rc" = 75
  test -z "$(og for-each-ref refs/merge-failed)"
  test -n "$(og for-each-ref refs/merge-queue)"
  git checkout -q main
  commit_file shared remote
  git push -q origin main
  tick
  test -n "$(og for-each-ref refs/merge-failed)"
  test "$(og show main:shared)" = remote
}
report_queries() {
  fixture report
  enable
  commit_file payload yes
  submit lane
  sha=$(git rev-parse HEAD)
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
case "$1 $2 ${4-}" in "merge-base --is-ancestor $fault") touch "$HOME/hit"; exit 128 ;; esac
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  for fault in refs/remotes/origin/main refs/remotes/origin/lane/lane; do
    export fault
    if [ "$fault" != refs/remotes/origin/main ]; then rm .git/hc-submitted; fi
    rc=0; PATH="$HOME/stub:$PATH" "$lane_bin" report 착지 "$sha" --name lane >"$HOME/log" 2>&1 || rc=$?
    test -f "$HOME/hit"
    test "$rc" != 0
    awk -F'\t' '$5=="착지"{exit 1}' "$HOME/.local/state/hc-lane/events.tsv"
    rm "$HOME/hit"
  done
  "$lane_bin" report 착지 "$sha" --name lane >"$HOME/log" 2>&1
  grep -q '제출만' "$HOME/log"
  git push -q origin HEAD:main
  "$lane_bin" report 착지 "$sha" --name lane >"$HOME/log" 2>&1
  awk -F'\t' '$5=="착지"{n++} END {exit n!=1}' "$HOME/.local/state/hc-lane/events.tsv"
}
notify_lock_fd() {
  fixture notify-fd
  enable
  commit_file payload yes
  mkdir "$HOME/stub"
  cat >"$HOME/stub/hc" <<'SH'
#!/bin/bash
case $1 in
  get) if ( : >&19 ) 2>/dev/null; then touch "$HOME/inherited"; fi
       touch "$HOME/checked"; echo '{"name":"lane","state":"live","created_ms":123}' ;;
  ls) echo '{"terminals":[]}' ;;
  send) : ;;
  *) exit 3 ;;
esac
SH
  chmod +x "$HOME/stub/hc"
  PATH="$HOME/stub:$PATH" HC_TERMINAL_ID=t11 submit lane
  rm "$HOME/checked"
  PATH="$HOME/stub:$PATH" tick
  test -f "$HOME/checked"
  test ! -e "$HOME/inherited"
}
notify_run() {
  for mode in reused same unknown; do
    fixture "notify-$mode"
    enable
    commit_file payload yes
    mkdir "$HOME/stub"
    cat >"$HOME/stub/hc" <<'SH'
#!/bin/sh
case $1 in
  get) born=$(cat "$HOME/created") || exit 1
       echo "{\"name\":\"lane\",\"state\":\"live\",\"created_ms\":$born}"
       test -z "$born" && exit 1; exit 0 ;;
  ls) echo '{"terminals":[]}' ;;
  send) printf '%s\n' "$2" >>"$HOME/sent" ;;
  *) exit 3 ;;
esac
SH
    chmod +x "$HOME/stub/hc"
    if [ "$mode" = unknown ]; then : >"$HOME/created"; else echo 11 >"$HOME/created"; fi
    PATH="$HOME/stub:$PATH" HC_TERMINAL_ID=t11 submit lane
    if [ "$mode" = same ]; then echo 11 >"$HOME/created"; else echo 22 >"$HOME/created"; fi
    PATH="$HOME/stub:$PATH" tick
    if [ "$mode" = same ]; then test "$(cat "$HOME/sent")" = t11; else test ! -e "$HOME/sent"; fi
    awk -F'\t' '$4=="lane" && $5=="착지" {n++} END {exit n!=1}' "$HOME/.local/state/hc-lane/events.tsv"
  done
}
wait_submit_owner() {
  fixture wait-owner
  enable
  commit_file payload yes
  HC_LANE_OWNER=box/t1 HC_LANE_OWNER_RUN=11 submit lane
  mkdir "$HOME/stub"
  cat >"$HOME/stub/hc" <<'SH'
#!/bin/sh
case $1 in
  get) echo "{\"address\":\"t1\",\"state\":\"live\",\"pid\":123,\"name\":\"owner\",\"created_ms\":$created}"; test "$fault" = ok ;;
  ls) echo '{"terminals":[]}' ;;
  *) exit 3 ;;
esac
SH
  chmod +x "$HOME/stub/hc"
  for mode in failed reused same; do
    fault=ok created=11
    case $mode in failed) fault=error ;; reused) created=22 ;; esac
    rc=0; fault=$fault created=$created PATH="$HOME/stub:$PATH" HC_TERMINAL_ID=t1 \
      "$lane_bin" wait --mine --state 제출 --timeout 0 >"$HOME/wait.log" 2>&1 || rc=$?
    if [ "$mode" = same ]; then
      test "$rc" = 0
      grep -Fq $'\tlane\t제출\t' "$HOME/wait.log"
    else
      test "$rc" = 75
      if grep -Fq $'\tlane\t제출\t' "$HOME/wait.log"; then return 1; fi
    fi
  done
}
result_owner() {
  mkdir "$T/watch-bin"
  printf '%s\n' '#!/bin/sh' 'echo "{\"address\":\"t1\",\"state\":\"live\",\"created_ms\":$watch_created}"' >"$T/watch-bin/hc"
  chmod +x "$T/watch-bin/hc"
  . "$here/../lib/hc-lane/common.sh"; . "$here/../lib/hc-lane/watch.sh"
  for mode in landed returned legacy unowned; do
    fixture "owner-$mode"
    enable
    if [ "$mode" = returned ]; then commit_file .claude/herdr-verify.sh 'exit 1'; else commit_file payload yes; fi
    case $mode in
      legacy) HC_LANE_OWNER=box/t1 HC_LANE_OWNER_RUN= submit lane ;;
      unowned) HC_LANE_OWNER='*' HC_LANE_OWNER_RUN= submit lane ;;
      *) HC_LANE_OWNER=box/t1 HC_LANE_OWNER_RUN=11 submit lane ;;
    esac
    HC_LANE_OWNER=t9 HC_LANE_OWNER_RUN=99 tick
    event=착지 owner=box/t1@11 to=box/t1
    case $mode in
      returned) event=막힘 ;;
      legacy) owner=box/t1 ;;
      unowned) owner='*' to=- ;;
    esac
    row=$(awk -F'\t' -v s="$event" '$4=="lane" && $5==s' "$HOME/.local/state/hc-lane/events.tsv")
    test "$(cut -f9,11 <<<"$row")" = "$to"$'\t'"$owner"
    if [ "$owner" = box/t1@11 ]; then
      test "$(PATH="$T/watch-bin:$PATH" wto=t1 watch_created=11 wfilter "$row")" = "$row"
      test -z "$(PATH="$T/watch-bin:$PATH" wto=t1 watch_created=22 wfilter "$row")"
    fi
  done
}
reserved_queue() {
  fixture reserved
  enable
  old=$(git rev-parse main)
  commit_file payload yes
  # 제출 기록 없이 ref로 넣은 옛 큐도 병합기의 입력이다.
  git push -q origin HEAD:refs/merge-queue/1000000000-main
  tick
  test "$(git -C "$merger" rev-parse --verify refs/heads/main)" = "$old"
  test "$(og rev-parse main)" = "$old"
  test "$(og rev-parse refs/merge-failed/1000000000-main)" = "$(git rev-parse HEAD)"
}
queue_name() {
  fixture queue-name
  enable
  commit_file payload yes
  submit lane
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
case "$*" in 'check-ref-format --branch lane') exit 128 ;; esac
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  PATH="$HOME/stub:$PATH" tick
  test -z "$(og for-each-ref refs/merge-failed)"
  test "$(og show main:payload)" = yes
}
approved_in_main() {
  fixture already-approved
  enable
  commit_file payload yes
  approved=$(git rev-parse HEAD)
  git push -q origin HEAD:main
  export HC_FILL_VERDICTS=$HOME/verdicts HC_LANE=lane HC_LANE_JOB=run-1
  mkdir -p "$HC_FILL_VERDICTS/lane"
  printf 'aid\ta1\nlane\tlane\nrun\trun-1\nsha\t%s\nverdict\tpass\nbase\tmain\n' "$approved" >"$HC_FILL_VERDICTS/lane/a1.md"
  "$lane_bin" land main --verdict a1 >"$HOME/log" 2>&1
  test "$(cut -f2 "$HC_FILL_VERDICTS/lane/a1.done")" = "$approved"
  test -z "$(og for-each-ref refs/merge-queue)"
}
clone_state() {
  fixture clone-state
  enable
  tick
  commit_file payload yes
  submit lane
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
case "$fault:$*" in
  "head:symbolic-ref -q HEAD"|"path:rev-parse --git-path rebase-merge"|"abort:rebase --abort") touch "$HOME/hit"; exit 128 ;;
esac
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  for fault in head path abort; do
    export fault
    [ "$fault" != abort ] || mkdir "$merger/.git/rebase-merge"
    rc=0; PATH="$HOME/stub:$PATH" tick || rc=$?
    test -f "$HOME/hit"
    test "$rc" = 75
    test "$(og rev-parse main)" = "$(git rev-parse main)"
    rm "$HOME/hit"
  done
  rmdir "$merger/.git/rebase-merge"
  tick
  test "$(og show main:payload)" = yes
}
cleanup_retry() {
  fixture cleanup
  enable
  commit_file payload yes
  submit lane
  root=$merger.merger-wt
  mkdir -p "$root/c-123456" "$HOME/stub"
  git -C "$merger" worktree add -q --detach "$root/c-abcdef" HEAD
  cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
case "$fault $1 $2" in "remove worktree remove"|"prune worktree prune") touch "$HOME/hit"; exit 128;; esac
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  for fault in remove prune; do
    export fault
    rc=0; PATH="$HOME/stub:$PATH" tick || rc=$?
    test -f "$HOME/hit"
    test "$rc" = 75
    test "$(og rev-parse main)" = "$(git rev-parse main)"
    rm "$HOME/hit"
  done
  tick
  test "$(og show main:payload)" = yes
  test -z "$(ls -A "$root")"
}
return_proof() {
  for state in withdrawn response_lost; do
    fixture "return-$state"
    enable
    commit_file .claude/herdr-verify.sh 'exit 1'
    submit lane
    mkdir "$HOME/stub"
    cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
case "$1 $*" in
  "push "*":refs/merge-failed/"*)
    touch "$HOME/attempt"
    if [ "$state" = response_lost ]; then "$real_git" "$@" || exit 128
    else
      for ref in $("$real_git" --git-dir="$HOME/origin.git" for-each-ref --format='%(refname)' refs/merge-queue refs/merge-queue-meta); do
        "$real_git" --git-dir="$HOME/origin.git" update-ref -d "$ref" || exit 128
      done
    fi
    exit 128 ;;
esac
exec "$real_git" "$@"
SH
    chmod +x "$HOME/stub/git"
    export state
    PATH="$HOME/stub:$PATH" tick
    test -f "$HOME/attempt"
    expect=0; [ "$state" = withdrawn ] || expect=1
    awk -F'\t' -v expected="$expect" '$4=="lane" && $5=="막힘" {n++} END {exit n+0!=expected}' "$HOME/.local/state/hc-lane/events.tsv"
    test "$(og for-each-ref --format=x refs/merge-failed-meta | wc -l)" -eq "$expect"
  done
}
approval_space() {
  fixture approval-space
  commit_file .claude/herdr-verify.sh 'test ! -e "$HOME/failverify"'
  commit_file '안내 문구' '두 단어'
  approved=$(git rev-parse HEAD)
  export HC_FILL_VERDICTS=$HOME/verdicts HC_LANE=lane HC_LANE_JOB=run-1
  mkdir -p "$HC_FILL_VERDICTS/lane"
  printf 'aid\ta1\nlane\tlane\nrun\trun-1\nsha\t%s\nverdict\tpass\nbase\tmain\n' "$approved" >"$HC_FILL_VERDICTS/lane/a1.md"
  touch "$HOME/failverify"
  "$lane_bin" land main --verdict a1 >"$HOME/log" 2>&1 && return 1
  grep -q '검증 실패' "$HOME/log"
  rm "$HOME/failverify"
  commit_file '안내 문구' '두단어'
  rc=0; "$lane_bin" land main --verdict a1 >>"$HOME/log" 2>&1 || rc=$?
  test "$rc" = 2
  test "$(og rev-parse main)" = "$(git rev-parse main)"
  test ! -e "$HC_FILL_VERDICTS/lane/a1.done"
}
hook_lease() {
  fixture hook-lease
  cat >.claude/herdr-land.sh <<'SH'
git --git-dir="$HOME/origin.git" update-ref refs/hc-lanes/lane "$replacement" "$HC_LANE_LEASE" || exit 1
git push --atomic origin HEAD:main $HERDR_LAND_PUSH_EXTRA
SH
  git add -- .claude/herdr-land.sh
  git commit -qm '전용 착지'
  old=$(git rev-parse main)
  replacement=$(git rev-parse HEAD)
  export replacement
  git push -q origin "$old:refs/hc-lanes/lane" "$replacement:refs/heads/other"
  rc=0; HC_LANE=lane HC_LANE_LEASE=$old "$lane_bin" land main >"$HOME/log" 2>&1 || rc=$?
  test "$(og rev-parse --verify refs/hc-lanes/lane)" = "$replacement"
  test "$rc" != 0
  test "$(og rev-parse main)" = "$old"
}
hook_lease_query() {
  fixture hook-lease-query
  base=$(git rev-parse main)
  cat >.claude/herdr-land.sh <<'SH'
touch "$HOME/hook-ran"
git push -q --atomic origin HEAD:main $HERDR_LAND_PUSH_EXTRA
SH
  git add -- .claude/herdr-land.sh
  git commit -qm '원자 착지 훅'
  git push -q origin "$base:refs/hc-lanes/lane"
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
if [ "$*" = 'ls-remote origin refs/hc-lanes/lane' ]; then touch "$HOME/hit"; exit 128; fi
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  export HC_LANE=lane HC_LANE_LEASE=$base
  rc=0; PATH="$HOME/stub:$PATH" "$lane_bin" land main >"$HOME/log" 2>&1 || rc=$?
  test -f "$HOME/hit"
  test "$rc" = 75
  test ! -e "$HOME/hook-ran"
  test "$(og rev-parse main)" = "$base"
  "$lane_bin" land main >>"$HOME/log" 2>&1
  test "$(og rev-parse main)" = "$(git rev-parse HEAD)"
  test -z "$(og for-each-ref refs/hc-lanes/lane)"
}
approval_retry() {
  for state in fast_forward unrelated; do
    fixture "$state"
    commit_file .claude/herdr-verify.sh 'test ! -e "$HOME/failverify"'
    commit_file payload yes
    approved=$(git rev-parse HEAD)
    export HC_FILL_VERDICTS=$HOME/verdicts HC_LANE=lane HC_LANE_JOB=run-1
    mkdir -p "$HC_FILL_VERDICTS/lane"
    printf 'aid\ta1\nlane\tlane\nrun\trun-1\nsha\t%s\nverdict\tpass\nbase\tmain\n' "$approved" >"$HC_FILL_VERDICTS/lane/a1.md"
    donefile=$HC_FILL_VERDICTS/lane/a1.done
    if [ "$state" = fast_forward ]; then
      "$lane_bin" land main --verdict a1 >"$HOME/log" 2>&1
      rm "$donefile"
      "$lane_bin" land main --verdict a1 >>"$HOME/log" 2>&1
      test "$(cut -f2 "$donefile")" = "$approved"
    else
      touch "$HOME/failverify"
      "$lane_bin" land main --verdict a1 >"$HOME/log" 2>&1 && return 1
      git checkout -q -B lane main
      "$lane_bin" land main --verdict a1 >>"$HOME/log" 2>&1 && return 1
      test ! -e "$donefile"
    fi
  done
}
rerere_content() {
  fixture "resolution-${1:-same}"
  export GIT_CONFIG_COUNT=2 GIT_CONFIG_KEY_0=rerere.enabled GIT_CONFIG_VALUE_0=true GIT_CONFIG_KEY_1=rerere.autoUpdate GIT_CONFIG_VALUE_1=true
  git checkout -q main
  commit_file .claude/herdr-verify.sh 'test ! -e "$HOME/failverify"'
  git checkout -q -B lane main
  commit_file shared lane
  approved=$(git rev-parse HEAD)
  git checkout -q main
  commit_file shared other
  git push -q origin main
  git checkout -q lane
  git rebase main >"$HOME/prepare" 2>&1 && return 1
  printf 'lane\nother\n' >shared
  git add -- shared
  GIT_EDITOR=true git rebase --continue >>"$HOME/prepare" 2>&1
  git checkout -q -B lane "$approved"
  export HC_FILL_VERDICTS=$HOME/verdicts HC_LANE=lane HC_LANE_JOB=run-1
  mkdir -p "$HC_FILL_VERDICTS/lane"
  printf 'aid\ta1\nlane\tlane\nrun\trun-1\nsha\t%s\nverdict\tpass\nbase\tmain\n' "$approved" >"$HC_FILL_VERDICTS/lane/a1.md"
  touch "$HOME/failverify"
  rc=0; "$lane_bin" land main --verdict a1 >"$HOME/log" 2>&1 || rc=$?
  test "$rc" = 1
  grep -q '검증 실패' "$HOME/log"
  resolved=$(git rev-parse HEAD)
  rm "$HOME/failverify"
  if [ "${1-}" = advanced ]; then
    git checkout -q main
    echo '새 기준 내용' >>shared
    git commit -qam '기본 가지에서 같은 파일을 다시 고침'
    git push -q origin main
    base=$(og rev-parse main)
    git checkout -q -B lane main
    git show "$resolved:shared" >shared
    git commit -qam '옛 충돌 풀이를 적용'
    rc=0; "$lane_bin" land main --verdict a1 >>"$HOME/log" 2>&1 || rc=$?
    test "$rc" = 2
    test "$(og rev-parse main)" = "$base"
    test ! -e "$HC_FILL_VERDICTS/lane/a1.done"
    return
  fi
  commit_file shared later-edit
  rc=0; "$lane_bin" land main --verdict a1 >"$HOME/log" 2>&1 || rc=$?
  test "$rc" = 2
  test "$(og show main:shared)" = other
  git checkout -q -B lane "$resolved"
  "$lane_bin" land main --verdict a1 >"$HOME/log" 2>&1
  test "$(og rev-parse main)" = "$resolved"
}
rerere_base() { rerere_content advanced; }
isolate_root() {
  fixture ceiling
  mkdir -p "$writer/build/test/sub"
  (
    T=$writer/build/test
    . "$here/isolate.sh"
    if git rev-parse --git-dir >/dev/null 2>&1; then exit 1; fi
    if git -C sub rev-parse --git-dir >/dev/null 2>&1; then exit 1; fi
    git init -q -b main sub
    test "$(git -C sub rev-parse --show-toplevel)" = "$T/sub"
  )
}
parked_branch() {
  fixture parked
  commit_file prepared yes
  git push -q origin HEAD:refs/heads/lane/parked
  git fetch -q origin
  og update-ref -d refs/heads/lane/parked
  mkdir "$HOME/stub"
  cat >"$HOME/stub/hc" <<'SH'
#!/bin/sh
case $1 in run) touch "$HOME/launched"; echo '{"address":"t11"}';; ls) echo '{"terminals":[]}';; *) exit 3;; esac
SH
  chmod +x "$HOME/stub/hc"
  PATH="$HOME/stub:$PATH" "$lane_bin" --resume --name parked --repo "$writer" --start-wait 0 -- true >"$HOME/log" 2>&1 && return 1
  test ! -e "$HOME/launched"
  test ! -e "$writer.wt/parked"
  git push -q origin HEAD:refs/heads/lane/parked
  PATH="$HOME/stub:$PATH" "$lane_bin" --resume --name parked --branch alt --repo "$writer" --start-wait 0 -- true >"$HOME/log" 2>&1
  test -f "$HOME/launched"
  test "$(git -C "$writer.wt/alt" rev-parse HEAD)" = "$(git rev-parse HEAD)"
}
event_id() {
  fixture ids
  enable
  tick
  events=$HOME/.local/state/hc-lane/events.tsv
  box=$merger/.git/hc-merger/out
  printf '%s\n' 착지 - '완료 — 제출 ID box-1234' lane '' '' '' '제출 ID box-1234' >"$box/first"
  tick
  printf '%s\n' 착지 - '완료 — 제출 ID box-123' lane '' '' '' '제출 ID box-123' >"$box/second"
  cp "$box/second" "$HOME/replay"
  tick
  cp "$HOME/replay" "$box/second"
  tick
  awk -F'\t' '$7=="완료 — 제출 ID box-123" {n++} END {exit n!=1}' "$events"
}
lease_owner() {
  fixture leases
  old=$(git rev-parse main)
  commit_file next new
  other=$(git rev-parse HEAD)
  git push -q origin "$other:refs/hc-lanes/retired"
  # 옛 클론의 clean 재시도. 같은 이름의 새 실행은 다른 기계에서 이미 임대를 잡았다.
  HC_LANE=retired HC_LANE_LEASE=$old "$lane_bin" clean --repo "$writer" --name retired >"$HOME/log" 2>&1
  test "$(og rev-parse --verify refs/hc-lanes/retired)" = "$other"
  if HC_LANE=retired HC_LANE_LEASE=$old "$lane_bin" park >"$HOME/log" 2>&1; then return 1; fi
  test -z "$(og for-each-ref refs/heads/lane/retired)"
  test "$(og rev-parse --verify refs/hc-lanes/retired)" = "$other"
  . "$here/../lib/hc-lane/land.sh"
  own=$(lease_core take "$writer" owned box branch-alias)
  test "$(og rev-parse refs/hc-lanes/owned)" = "$own"
  "$lane_bin" clean --repo "$writer" --branch branch-alias >"$HOME/log" 2>&1
  test -z "$(og for-each-ref refs/hc-lanes/owned)"
}
candidate_checkout() {
  fixture checkout
  enable
  for name in alpha zulu; do
    git checkout -q -B "$name" main
    commit_file "$name" yes
    submit "$name"
  done
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
case "$*" in 'checkout -q -B alpha'|'checkout -q -f alpha') touch "$HOME/injected"; exit 128;; esac
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  rc=0; PATH="$HOME/stub:$PATH" tick || rc=$?
  test -f "$HOME/injected"
  test "$rc" = 75
  test "$(og rev-parse main)" = "$(git rev-parse main)"
  test "$(og for-each-ref --format=x refs/merge-queue | wc -l)" -eq 2
  tick
  test "$(og show main:zulu)" = yes
}
live_candidate() {
  fixture live
  enable
  cat >.claude/herdr-verify.sh <<'SH'
if mkdir "$HOME/first" 2>/dev/null; then
  pwd -P >"$HOME/candidate"
  perl -e 'for (1..600) { last if -e "$ENV{HOME}/release"; select undef,undef,undef,0.05 }'
  test -f .git
fi
SH
  git commit -qam '검증 대기'
  submit lane
  interrupt_candidate
  awk -F'\t' '$5=="착지" && index($7,"앞 병합기 실행"){n++} END {exit n!=1}' "$HOME/.local/state/hc-lane/events.tsv"
}
live_worktree() {
  fixture live-worktree
  enable
  commit_file payload yes
  submit lane
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/bash
if [ "$1 $2" = 'worktree add' ] && mkdir "$HOME/first" 2>/dev/null; then
  "$real_git" "$@" || exit 1
  printf '%s\n' "${@: -2:1}" >"$HOME/candidate"
  perl -e 'for (1..600) { last if -e "$ENV{HOME}/release"; select undef,undef,undef,0.05 }'
  exit 0
fi
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  PATH="$HOME/stub:$PATH" interrupt_candidate
}
interrupt_candidate() {
  "$lane_bin" merger --repo "$merger" --once main >"$HOME/first.log" 2>&1 & leader=$!
  trap 'touch "$HOME/release"; [ -z "${leader-}" ] || wait "$leader" 2>/dev/null || :' EXIT
  for ((i=0;i<300;i++)); do [ ! -s "$HOME/candidate" ] || break; sleep 0.05; done
  test -s "$HOME/candidate"
  kill -KILL "$leader"
  wait "$leader" 2>/dev/null || :
  leader=
  free=0; perl -MFcntl=:flock -e 'open my $f, ">>", $ARGV[0] or die; flock($f, LOCK_EX | LOCK_NB) or exit 1' "$HOME"/.local/state/hc-lane/merger.*.lock || free=$?
  rc=0; tick || rc=$?
  present=0; [ ! -f "$(cat "$HOME/candidate")/.git" ] || present=1
  touch "$HOME/release"
  for ((i=0;i<300;i++)); do
    retry=0; tick || retry=$?
    [ "$retry" = 75 ] || break
    sleep 0.05
  done
  test "$rc" = 75
  test "$free" = 0
  test "$present" = 1
  test "$retry" = 0
  test -z "$(og for-each-ref refs/merge-queue)"
}
git_lock_fd() {
  fixture fd
  enable
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/bash
if [ "$1" = ls-remote ]; then
  touch "$HOME/called"
  if ( : >&19 ) 2>/dev/null; then touch "$HOME/inherited"; fi
fi
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  PATH="$HOME/stub:$PATH" tick
  test -f "$HOME/called"
  test ! -e "$HOME/inherited"
}
submit_retry() {
  for state in queued landed; do
    fixture "submit-$state"
    enable
    commit_file payload yes
    mkdir "$HOME/stub"
    cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
case "$1 $*" in "ls-remote "*refs/merge-failed-meta/*)
  touch "$HOME/interrupted"; exit 128 ;;
esac
exec "$real_git" "$@"
SH
    chmod +x "$HOME/stub/git"
    if PATH="$HOME/stub:$PATH" submit lane; then return 1; fi
    test -f "$HOME/interrupted"
    meta=$(og for-each-ref --format='%(objectname)' refs/merge-queue-meta)
    id=$(og show -s --format=%s "$meta" | tr ' ' '\n' | sed -n 's/^id=//p')
    test -n "$id"
    [ "$state" = queued ] || tick
    submit lane
    submit lane
    grep -Fx "lane $(git rev-parse HEAD) main" .git/hc-submitted
    awk -F'\t' -v id="$id" '$4=="lane" && $5=="제출" && index($7,id " ")==1 {n++} END {exit n!=1}' "$HOME/.local/state/hc-lane/events.tsv"
    test "$(og for-each-ref --format='%(objectname)' refs/merge-queue-meta refs/merge-done)" = "$meta"
  done
}
stale_hook_query() {
  fixture stale
  git checkout -q main
  commit_file .claude/herdr-land.sh 'touch "$HOME/old"'
  git push -q origin main
  git checkout -q -B lane main
  commit_file work yes
  git checkout -q main
  commit_file .claude/herdr-land.sh 'touch "$HOME/new"'
  git push -q origin main
  git checkout -q lane
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
case "$*" in "merge-base HEAD "*) touch "$HOME/injected"; exit 128;; esac
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  if PATH="$HOME/stub:$PATH" "$lane_bin" land main >"$HOME/log" 2>&1; then return 1; fi
  test -f "$HOME/injected"
  test ! -e "$HOME/old"
  "$lane_bin" land main >"$HOME/log" 2>&1
  test -f "$HOME/new"
}
batch_queries() {
  fixture policy
  git checkout -q main
  commit_file .claude/herdr-merge-solo fragments
  git push -q origin main
  enable
  for name in alpha zulu; do
    git checkout -q -B "$name" main
    mkdir -p fragments
    commit_file "fragments/$name" yes
    submit "$name"
  done
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
case "$fault $1 $*" in
  "policy show "*:.claude/herdr-merge-solo|"history rev-list "*" -- fragments"|"tree merge-tree "*)
    touch "$HOME/injected"; exit 128 ;;
esac
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  for fault in policy history tree; do
    export fault
    rc=0; PATH="$HOME/stub:$PATH" tick || rc=$?
    test -f "$HOME/injected"
    test "$rc" = 75
    test "$(og rev-parse main)" = "$(git rev-parse main)"
    test "$(og for-each-ref --format=x refs/merge-queue | wc -l)" -eq 2
    rm "$HOME/injected"
  done
  tick
  test "$(og show main:fragments/zulu)" = yes
}
withdrawn_candidate() {
  fixture withdrawn
  enable
  commit_file work first
  submit lane
  # 다른 작성자가 같은 커밋을 넣고 레인이 제출을 물린 직후, 병합기의 원자 push가 거절된다.
  mkdir "$HOME/stub"
  cat >"$HOME/stub/git" <<'SH'
#!/bin/sh
case "$1 $*" in
  "push "*":refs/heads/main "*":refs/merge-queue/"*)
    "$real_git" push -q origin HEAD:main || exit 1
    for ref in $("$real_git" --git-dir="$HOME/origin.git" for-each-ref --format='%(refname)' refs/merge-queue refs/merge-queue-meta); do
      "$real_git" --git-dir="$HOME/origin.git" update-ref -d "$ref" || exit 1
    done
    touch "$HOME/withdrawn"
    exit 1 ;;
esac
exec "$real_git" "$@"
SH
  chmod +x "$HOME/stub/git"
  PATH="$HOME/stub:$PATH" tick
  test -f "$HOME/withdrawn"
  test -z "$(og for-each-ref refs/merge-done refs/hc-landed/lane)"
  awk -F'\t' '$4=="lane" && $5=="착지" {exit 1}' "$HOME/.local/state/hc-lane/events.tsv"
}
interrupted_cleanup() {
  fixture already
  enable
  commit_file done yes
  submit lane
  git push -q origin HEAD:main
  mkdir "$HOME/stub"
  cat >"$HOME/stub/mv" <<'SH'
#!/bin/sh
case $* in *hc-merger/out/*) touch "$HOME/interrupted"; exit 1;; esac
exec /bin/mv "$@"
SH
  chmod +x "$HOME/stub/mv"
  rc=0; PATH="$HOME/stub:$PATH" tick || rc=$?
  test -f "$HOME/interrupted" && test "$rc" = 75
  test -z "$(og for-each-ref refs/merge-queue)"
  tick
  test -n "$(og for-each-ref refs/hc-landed/lane/)"
  awk -F'\t' '$4=="lane" && $5=="착지" {n++} END {exit n!=1}' "$HOME/.local/state/hc-lane/events.tsv"
}
approved_submit() {
  fixture approved
  enable
  commit_file approved yes
  export HC_FILL_VERDICTS=$HOME/verdicts HC_LANE_JOB=run-1
  mkdir -p "$HC_FILL_VERDICTS/lane"
  approved=$(git rev-parse HEAD)
  for verdict in fix pass; do
    printf 'aid\ta1\nlane\tlane\nrun\trun-1\nsha\t%s\nverdict\t%s\nbase\tmain\n' "$approved" "$verdict" >"$HC_FILL_VERDICTS/lane/a1.md"
    if [ "$verdict" = fix ]; then
      if HC_LANE=lane "$lane_bin" land main --verdict a1 >"$HOME/log" 2>&1; then return 1; fi
      test -z "$(og for-each-ref refs/merge-queue)"
    fi
  done
  git checkout -qb another main
  commit_file other yes
  submit alpha
  git checkout -q lane
  HC_LANE=lane "$lane_bin" land main --verdict a1 >"$HOME/log" 2>&1
  grep -q '^landing' "$HC_FILL_VERDICTS/lane/a1.ack"
  receipt=$(og for-each-ref --format='%(objectname)' 'refs/merge-queue-meta/*-lane')
  HC_LANE=lane "$lane_bin" land main --verdict a1 >"$HOME/log" 2>&1
  test "$(og for-each-ref --format='%(objectname)' 'refs/merge-queue-meta/*-lane')" = "$receipt"
  tick
  test "$(og show main:approved)" = yes
  test "$(og show main:other)" = yes
  grep -q '승인 착지(판정 a1)' "$HOME"/.local/state/hc-lane/merger-logs/lane-*.log
  awk -F'\t' '$4=="lane" && $5=="착지" && $12=="hf:a1:landed" {n++} END {exit n!=1}' "$HOME/.local/state/hc-lane/events.tsv"
}
batch_base() {
  fixture batch
  enable
  commit_file alpha first
  submit alpha
  git checkout -q main
  commit_file shared advanced
  git push -q origin main
  git checkout -qb second
  commit_file shared base
  submit zulu
  tick
  test "$(og show main:alpha)" = first
  test "$(og show main:shared)" = base
  test -z "$(og for-each-ref refs/merge-queue/)"
}
batch_default() {
  fixture batch-default
  enable
  for name in alpha beta gamma omega; do
    git checkout -q -b "$name" main
    commit_file "$name" yes
    submit "$name"
  done
  tick
  awk -F'\t' '$10=="착지" {n++; sizes[$7]++} END {exit !(n==4 && sizes[3]==3 && sizes[1]==1)}' "$HOME/.local/state/hc-lane/merger-journal.tsv"
}

untracked_hooks() {
  for hook in herdr-verify herdr-land; do
    fixture "$hook"
    git rm -q .claude/herdr-verify.sh
    git commit -qm '검증 훅 제거'
    mkdir -p .claude
    echo 'touch "$HOME/ran"' >".claude/$hook.sh"
    if "$lane_bin" land main >"$HOME/log" 2>&1; then return 1; fi
    test ! -e "$HOME/ran"
    test "$(git --git-dir="$origin" rev-parse main)" = "$(git rev-parse main)"
  done
}

for scenario in ${1:-untracked_hooks batch_base approved_submit interrupted_cleanup withdrawn_candidate batch_queries stale_hook_query submit_retry live_candidate git_lock_fd candidate_checkout lease_owner event_id parked_branch isolate_root rerere_content approval_retry hook_lease approval_space return_proof cleanup_retry clone_state approved_in_main queue_name reserved_queue notify_lock_fd report_queries rebase_retry default_query landed_lease receipt_race live_worktree resident_sleep reverted_upstream merge_resolution proof_ref postpush_cleanup verdict_identity guard_queries journal_retry installed_test_paths launch_response clean_submission_owner approval_location hook_lease_query rerere_base clean_rebased_receipt batch_default notify_run wait_submit_owner result_owner}; do
  ( "$scenario" )
  echo "통과 $scenario"
done
