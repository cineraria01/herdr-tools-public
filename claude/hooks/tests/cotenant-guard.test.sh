#!/bin/bash
# Tests for ~/.claude/hooks/cotenant-guard.sh (PreToolUse lease guard). Temp git repos, a fake session registry (fake HOME), a fake `hc` on PATH, and
# `sleep` processes standing in for agents. Run: bash ~/.claude/hooks/tests/cotenant-guard.test.sh  (exit 0 = pass)
HOOK=${HOOK:-$(cd "$(dirname "$0")/.." && pwd)/cotenant-guard.sh}   # the hook beside this tests folder (repo or ~/.claude/hooks)
. "$(dirname "$0")/hooks-impl.sh"   # 판: HERDR_HOOKS_IMPL=go(기본)·bash — 셸 판을 sed로 고치던 세 곳은 Go 판에서 시험 손잡이(HERDR_HOOKS_TEST_*)
T=$(mktemp -d "${TMPDIR:-/tmp}/cotenant-test.XXXXXX") && T=$(cd -P "$T" && pwd) || exit 1
H=$T/home
mkdir -p "$H/.claude/sessions" "$T/bin"
printf '{"env":{}}\n' > "$H/.claude/settings.json"
printf '#!/bin/sh\ncat "%s/hc.json" 2>/dev/null || echo %s\n' "$T" "'{\"terminals\":[]}'" > "$T/bin/hc"
chmod +x "$T/bin/hc"
KIDS=
trap '{ kill $KIDS; wait; } 2>/dev/null; rm -rf "$T"' EXIT
pass=0 fail=0

agent() { sleep 900 & KIDS="$KIDS $!"; }   # pid in $!
# reg PID SESSION CWD NAME [PROCSTART]
reg() {
  local st=${5-$(LC_ALL=C TZ=UTC ps -o lstart= -p "$1" | sed 's/ *$//')}
  # like Claude: compact JSON with no trailing newline
  printf '%s' "$(jq -cn --argjson p "$1" --arg s "$2" --arg c "$3" --arg n "$4" --arg st "$st" \
    '{pid: $p, sessionId: $s, cwd: $c, name: $n, procStart: $st, status: "busy"}')" > "$H/.claude/sessions/$1.json"
}
# hook SESSION TOOL PATH-OR-COMMAND [AGENT_ID] [CWD]  -> RC OUT ERR
hook() {
  local inp
  if [ "$2" = Bash ]; then
    inp=$(jq -cn --arg s "$1" --arg c "$3" --arg a "${4-}" --arg w "${5:-$R}" \
      '{session_id: $s, agent_id: (if $a == "" then null else $a end), cwd: $w, tool_name: "Bash", tool_input: {command: $c}}')
  else
    inp=$(jq -cn --arg s "$1" --arg t "$2" --arg f "$3" --arg a "${4-}" --arg w "${5:-$R}" \
      '{session_id: $s, agent_id: (if $a == "" then null else $a end), cwd: $w, tool_name: $t, tool_input: {file_path: $f}}')
  fi
  OUT=$(printf '%s' "$inp" | env -u CLAUDE_CONFIG_DIR -u HC_TERMINAL_ID HOME="$H" PATH="$T/bin:$PATH" ${HOOK_ENV-} bash "$HOOK" 2>"$T/err")
  RC=$? ERR=$(cat "$T/err")
}
check() {   # NAME WANT_RC [GREP_PATTERN in stderr+stdout]
  if [ "$RC" = "$2" ] && { [ -z "${3-}" ] || printf '%s\n%s\n' "$ERR" "$OUT" | grep -q -- "$3"; }; then
    pass=$((pass + 1)); echo "ok   $1"
  else
    fail=$((fail + 1)); echo "FAIL $1 (rc=$RC, want $2${3:+, /$3/}) err=[$ERR] out=[$OUT]"
  fi
}
mkrepo() { git init -q -b main "$1" && (cd "$1" && printf 'a\n' > a.txt && printf 'b\n' > b.txt && mkdir sub && printf 'c\n' > sub/c.txt &&
  for f in d e f g h; do printf '%s\n' $f > $f.txt; done && git add -A && git -c user.name=t -c user.email=t@t commit -qm init); }
leasefile() { echo "$(git -C "$1" rev-parse --absolute-git-dir)/agent-leases/$(md5 -q -s "$2")/owner"; }

# ---------- repo R: agent A alone, then B joins ----------
R=$T/r; mkrepo "$R"
agent; A=$!; reg $A sa "$R" agent-a
hook sa Edit "$R/a.txt"; check "동거 없음: 편집 허용" 0
[ -f "$(leasefile "$R" a.txt)" ] && cut -d $'\037' -f2 "$(leasefile "$R" a.txt)" | grep -qx "$A"; RC=$?; check "동거 없음: 임대는 기록(pid A)" 0
cut -d $'\037' -f5 "$(leasefile "$R" a.txt)" | grep -qx agent-a; RC=$?; check "임대에 레지스트리 이름 기록" 0
echo more >> "$R/a.txt"
hook sa Bash "git add -A && git status"; check "동거 없음: git add -A 차단 없음" 0
hook sa Edit "$R/a.txt"; check "내 임대는 계속 허용" 0
# force push is refused always, cotenant or not (2026-11-17: the coordinator force-pushed a shared temp ref)
for c in "git push -f origin main" "git push --force origin main" "git push -qf origin main" "git push --force-with-lease origin main" \
  "git push --force-with-lease=main origin main" "git push -f --force-with-lease=main:abc origin main" \
  "git push --force-with-lease=main:abc origin +main" "git push --force-with-lease --force-with-lease=main:abc origin main" \
  "git push --force-if-includes origin main" "git push --force-if-includes --force-with-lease origin main" "git push --mirror origin" \
  "git push -q origin +HEAD:refs/integ/x" "git push origin '+HEAD:refs/integ/x'" "git push origin \"+main\"" \
  "git -C $R push -f origin main" "env HERDR_LAND=1 git push --force origin main" "git -c x.y=1 push -f origin main" \
  "git fetch && git push -f origin main" "true; git push -f origin main" "echo x | git push -f origin main" \
  "bash -c 'git push -f origin main'" "x=\$(git push -f origin main)" "git -C /nonexistent push -f origin main"; do
  hook sa Bash "$c"; check "강제 push 차단(동거 없음): $c" 2 "강제 push.*기대값을 적은 --force-with-lease"
done
# audit M1: remote wrappers (hc exec/run --on … --, ssh <host> …) and nested shells; L1: git -c alias/mirror/+push
for c in "hc exec --on mini -- git push -f origin main" "hc exec --on mini --timeout 60 -- bash -lc 'git push -f origin main'" \
  "ssh mini 'cd x && git push -f origin main'" "ssh -p 22 -o BatchMode=yes mini git push --force origin main" \
  "ssh mini -- git push origin +main" "hc run --bg --on mini --name x -- git push --mirror origin" \
  "sh -c \"bash -lc 'zsh -c \\\"git push -f origin main\\\"'\"" "ssh mini \"hc exec --on m2 -- sh -c 'git push -f origin main'\"" \
  "git -c alias.p='push -f' p origin main" "git -c remote.origin.mirror=true push origin" "git -c remote.origin.push=+refs/heads/*:refs/heads/* push origin"; do
  hook sa Bash "$c"; check "감싼 강제 push 차단: $c" 2 "강제 push"
done
for c in "hc exec --on mini -- git push origin main" "ssh mini 'git push --force-with-lease=r:abc origin HEAD:r'" \
  "hc exec --on mini -- git fetch origin +refs/heads/*:refs/remotes/origin/*" "ssh mini 'echo git push -f'" \
  "hc exec --on mini -- bash -lc 'git push -q origin \"\${c}:refs/claims/\$n\" --force-with-lease=\"refs/claims/\$n:\"'" \
  "hc send t1 'git push -f 금지'" "git -c alias.st=status st" "ssh -t mini" "hc exec --on mini -- ls"; do
  hook sa Bash "$c"; check "감싼 정상 명령은 허용: $c" 0
done
# 2026-11-17 cg-multibyte: awk splits the command into bytes and cuts long words at 1024 bytes, mid-character for
# Korean/emoji; in a UTF-8 locale awk's regex then died (towc: multibyte conversion failure) = "Bash 명령 분석 실패"
# on a plain hc send. Pads 0..3 move the cut across every byte of a 3- and 4-byte character; verdicts stay the same
for loc in ko_KR.UTF-8 C; do
  for p in 0 1 2 3; do
    pad=$(printf '%*s' $p '' | tr ' ' x)
    for body in "$(printf '가나다라 %.0s' $(seq 1 120))" "$(printf '🙂한글 %.0s' $(seq 1 120))" "$(printf '깨�진 %.0s' $(seq 1 150))"; do
      HOOK_ENV="LC_ALL=$loc" hook sa Bash "MSG='$pad$body'; for t in a/t1 a/t2; do hc send \"\$t\" \"\$MSG\" 2>&1 | tail -1; done; hc read b/t3 2>&1 | grep -E \"Model:|effort\" | tail -1"
      check "긴 한국어 글 hc send 허용($loc, 자름 위치 +$p, ${body:0:2})" 0
      HOOK_ENV="LC_ALL=$loc" hook sa Bash "MSG='$pad$body'; git push -f origin main"
      check "긴 한국어 글 뒤 강제 push는 그대로 차단($loc, +$p, ${body:0:2})" 2 "강제 push"
      HOOK_ENV="LC_ALL=$loc" hook sa Bash "bash -c 'git push -f origin main; echo $pad$body'"
      check "긴 한국어 글이 든 bash -c 안 강제 push 차단($loc, +$p, ${body:0:2})" 2 "강제 push"
    done
  done
done
# audit 2 M1-b: wrappers by their whole option grammar (hc exec/run --help, ssh(1)), -- or not, value options before/after,
# nested; one table: verdict<TAB>command
deep='git push origin main'; for i in 1 2 3 4 5 6; do deep="sh -c $(printf %q "$deep")"; done
while IFS=$'\t' read -r want c; do
  [ -n "$c" ] || continue
  hook sa Bash "$c"
  if [ "$want" = 2 ]; then check "감싸기 문법 표 막힘: $c" 2 "강제 push\|겹침"; else check "감싸기 문법 표 허용: $c" 0; fi
done <<EOF
2	hc exec --on mini-m4 git push -f origin main
2	hc exec --on mini-m4 bash -lc 'git push -f origin main'
2	hc exec --on mini-m4 --cwd /x git push --force origin main
2	hc exec --on=mini --timeout=30 -i --env K=V git push -f origin main
2	hc exec --json --on mini --stdin git push origin +main
2	hc run --bg --on mini-m4 --name x 'cd app && git push -f origin main'
2	hc run --on vps "git push --mirror origin"
2	hc run --bg --on m --name x --cwd /w zsh -c 'git push -f origin main'
2	hc run --label a=b --parent t1 --no-parent --no-notify --outlive-parent --rm --keep --timeout 9 --on m git push origin +main
2	ssh -i ~/.ssh/k -J jump -l me -F cfg mini 'git push -f origin main'
2	ssh -tp 22 mini git push -f origin main
2	ssh -oBatchMode=yes -A mini git push --force origin main
2	ssh -p22 -E log -b 1.2.3.4 me@mini "bash -lc 'git push -f origin main'"
2	hc exec --on a ssh b "hc run --on c 'sh -c \"git push -f origin main\"'"
2	ssh a hc exec --on b zsh -c "'git push --mirror origin'"
2	git -c alias.p=push p origin +main
2	git -c alias.p='!git push -f' p origin main
2	$deep
0	hc run --bg --on m --name x claude "git push -f 쓰지 마"
0	hc run reviewer "review src/x.rs"
0	hc exec --on m git push origin main
0	hc exec --on m ls -f
0	hc exec --on m git push -q origin "c:refs/claims/n" --force-with-lease=refs/claims/n:
0	hc run --on m 'git push -q origin --delete refs/claims/n --force-with-lease=refs/claims/n:abc'
0	git -c alias.pf='push --force-with-lease=main:abc' pf origin main
0	hc-lane send --all -- 'git push --force 금지'
0	ssh mini 'git log -f'
0	ssh -f -p 22 mini 'sleep 1'
2	git -c alias.p='push -f' -c alias.q=status p origin main
2	git -c alias.P='push -f' p origin main
2	bash +x -c 'git push -f origin main'
2	hc exec --on m bash +o posix -c 'git push -f origin main'
2	hc --json exec --on m git push -f origin main
0	git -c alias.p='push -f' -c alias.p=status p
0	bash +x script.sh -f
EOF
for c in "git push origin main" "git push -u origin HEAD" "git push origin --delete refs/integ/x" "git push origin :refs/integ/x" \
  "git push -o ci.skip origin main" "git fetch origin +refs/heads/*:refs/remotes/origin/*" "git fetch -f origin" \
  "git commit -m 'git push -f 금지' -- a.txt" "echo 'git push --force'" \
  "git push --force-with-lease=main:abc origin main" "git push --force-with-lease=main:abc --force-if-includes origin main" \
  'git push -q origin "${c}:refs/claims/$n" --force-with-lease="refs/claims/$n:" && echo "$n $c" >> ev/claims.txt' \
  'git push -q origin "${c}:refs/claims/$n" --force-with-lease="refs/claims/$n:0123abcd"' \
  'git push -q origin "${c2}:refs/claims/$n" --force-with-lease="refs/claims/$n:$old"' \
  'git push -q origin --delete "refs/claims/$n" --force-with-lease="refs/claims/$n:$c"' \
  'git push -q origin --delete refs/wt-claims/x --force-with-lease=refs/wt-claims/x:0123abcd'; do
  hook sa Bash "$c"; check "강제 push 아님(기대값 적은 lease·선점 포함)은 허용: $c" 0
done

printf '{"pid": 1, "sessionId": "broken' > "$H/.claude/sessions/1.json"   # a half-written registry file
agent; B=$!; reg $B sb "$R" agent-b
hook sb Edit "$R/a.txt"; check "혼자 고치던 파일을 뒤에 합류한 쪽이 못 가져감" 2 "agent-a"
hook sb Write "$R/a.txt"; check "Write도 같은 판정" 2 "고치는 중"
hook sb Bash "git add -A"; check "동거: git add -A 차단" 2 "git add -- <경로>"
hook sb Bash "cd sub && git add ."; check "동거: git add . 차단" 2
hook sb Bash "git commit -am fix"; check "동거: git commit -am 차단" 2
hook sb Bash "git commit --all -m x"; check "동거: git commit --all 차단" 2
hook sb Bash "git stash"; check "동거: git stash 차단" 2 "지우거나 숨길"
hook sb Bash "ssh mini 'git add -A && git stash'"; check "동거여도 원격(ssh) 명령은 이 트리 동거 판정 안 함" 0
hook sb Bash "hc exec --on mini -- git stash"; check "동거여도 원격(hc exec) 명령은 이 트리 동거 판정 안 함" 0
hook sb Bash "hc exec --on mini -- git push -f origin main"; check "동거 중 원격 강제 push는 막힘" 2 "강제 push"
hook sb Bash "hc exec -- git stash"; check "동거 중 --on 없는 로컬 hc exec는 이 트리 규칙(감사2 L-c)" 2 "지우거나 숨길"
hook sb Bash "git stash list"; check "git stash list는 허용" 0
hook sb Bash "git reset --hard HEAD"; check "동거: git reset --hard 차단" 2
hook sb Bash "git checkout -- b.txt"; check "동거: git checkout -- 차단" 2
hook sb Bash "git checkout ."; check "동거: git checkout . 차단" 2
hook sb Bash "git restore b.txt"; check "동거: git restore 차단" 2
hook sb Bash "git restore --staged b.txt"; check "git restore --staged는 허용" 0
hook sb Bash "git clean -fd"; check "동거: git clean 차단" 2
hook sb Bash "git clean -n"; check "git clean -n은 허용" 0
hook sb Edit "$R/b.txt"; check "남이 안 잡은 깨끗한 파일 편집 허용" 0
echo more >> "$R/b.txt"
hook sb Bash "git add b.txt && git commit -m 'b: git add -A 금지' -- b.txt"; check "git add <내 파일> 허용, 메시지 속 글자는 무시" 0
hook sb Bash "git add a.txt"; check "git add에 남의 임대 파일 차단" 2 "agent-a"
hook sb Bash "git -C $R add -- ."; check "git -C 경로 add . 차단" 2
hook sb Bash "git rebase origin/main"; check "남의 미커밋 파일이 있으면 rebase 차단 + 탈출로" 2 "worktree add --detach ../r.wt/land-"
hook sb Bash "git rebase origin/main"; check "착지 탈출로 문구: origin 기준 + cherry-pick" 2 "origin/<기본 브랜치>.*cherry-pick"
hook sb Bash "git pull --rebase"; check "pull 차단" 2
hook sb Bash "git checkout -b x"; check "checkout -b 차단" 2
hook sb Bash "git merge --abort"; check "merge --abort는 허용" 0
hook sb Bash "sed -i '' 's/a/b/' a.txt"; check "남의 임대 파일 sed -i 차단" 2
hook sb Bash "echo hi > a.txt"; check "남의 임대 파일 > 차단" 2
hook sb Bash "echo hi >> $R/a.txt"; check "남의 임대 파일 절대 경로 >> 차단" 2
hook sb Bash "rm -f ./a.txt"; check "남의 임대 파일 rm 차단" 2
hook sb Bash "cp b.txt a.txt"; check "남의 임대 파일 cp 대상 차단" 2
hook sb Bash "mv a.txt z.txt"; check "남의 임대 파일 mv 차단" 2
hook sb Bash "cat a.txt > /dev/null 2>&1; wc -l a.txt"; check "남의 파일 읽기만은 허용" 0
hook sb Bash "printf 'x\n' > b.txt"; check "내 파일 > 허용" 0
hook sb Bash "$(printf "cat > notes.txt <<'EOF'\ngit add -A\nrm a.txt\nEOF\n")"; check "heredoc 본문은 명령이 아님" 0
hook sb Bash "$(printf 'git commit -m "$(cat <<'"'"'EOF'"'"'\nfix: git add -A 규칙\nEOF\n)" -- b.txt')"; check "commit 메시지 heredoc 속 git add -A 무시" 0
hook sb Bash "echo 'git add -A'"; check "따옴표 속 명령은 무시" 0
hook sb Bash "x=\$(git stash)"; check "\$( ) 안의 명령도 검사" 2

# Bash writes lease too: B's own redirect target is B's afterwards
hook sb Bash "echo new > made-by-b.txt"; check "Bash 쓰기 대상도 임대" 0
printf 'new\n' > "$R/made-by-b.txt"
hook sb Edit "$R/made-by-b.txt"; check "내가 Bash로 만든 파일은 Edit 허용" 0
hook sa Edit "$R/made-by-b.txt"; check "남이 Bash로 만든 파일은 막힘" 2 "agent-b"

# ---------- concurrent first edits of the same new file: exactly one wins ----------
for k in 1 2 3 4 5; do
  ( hook sa Write "$R/new$k.txt"; echo $RC > "$T/rc.a$k" ) & j1=$!
  ( hook sb Write "$R/new$k.txt"; echo $RC > "$T/rc.b$k" ) & j2=$!
  wait $j1 $j2
  RC=$(( $(cat "$T/rc.a$k") + $(cat "$T/rc.b$k") )); check "동시 두 획득 중 하나만 성공 ($k)" 2
done

# ---------- dead owner and pid reuse ----------
agent; C=$!; reg $C sc "$R" agent-c
hook sc Edit "$R/d.txt"; echo more >> "$R/d.txt"
kill $C; wait $C 2>/dev/null
hook sb Edit "$R/d.txt"; check "죽은 소유자 파일은 인수 + 경고" 0 "끝난 에이전트 agent-c"
cut -d $'\037' -f2 "$(leasefile "$R" d.txt)" | grep -qx "$B"; RC=$?; check "인수 뒤 소유자는 B" 0
mkdir -p "$(dirname "$(leasefile "$R" e.txt)")"
printf 'sz\037%s\037Mon Jan  1 00:00:00 2001\037\037reused\0372026-01-01 00:00:00\037e.txt\n' "$A" > "$(leasefile "$R" e.txt)"
echo more >> "$R/e.txt"
hook sb Edit "$R/e.txt"; check "pid 재사용(procStart 불일치)은 죽은 것으로 인수 + 경고" 0 "끝난 에이전트 reused"

# ---------- unknown owner, idle hand-over, subagent ----------
echo more >> "$R/f.txt"
hook sb Edit "$R/f.txt"; check "주인 불명 dirty 파일 차단(동거)" 2 "임대 기록 없는"
hook sa Edit "$R/g.txt"
hook sb Edit "$R/g.txt"; check "남의 최근 임대(깨끗해도) 차단" 2 "10분 안"
touch -t 202601010000 "$(leasefile "$R" g.txt)"
hook sb Edit "$R/g.txt"; check "10분 넘게 손 안 댄 깨끗한 남의 파일은 넘겨받음" 0
hook sa Edit "$R/a.txt" agent-sub1; check "같은 세션 서브에이전트는 경고만" 0 "같은 세션의 다른 에이전트"

# ---------- off switch ----------
printf '{"env":{"AGENT_LEASE_OFF":"1"}}\n' > "$H/.claude/settings.json"
hook sb Edit "$R/a.txt"; check "settings.json env AGENT_LEASE_OFF=1이면 허용" 0
printf '{"env":{}}\n' > "$H/.claude/settings.json"
HOOK_ENV="AGENT_LEASE_OFF=1" hook sb Edit "$R/a.txt"; check "환경 변수만으로는 꺼지지 않음" 2

# ---------- tree boundaries: subfolder, symlink, linked worktree, nested worktree, sibling name ----------
R2=$T/r2; mkrepo "$R2"
agent; X=$!; reg $X sx "$R2" agent-x
hook sx Bash "git add -A" "" "$R2"; check "R2 혼자: git add -A 허용" 0
agent; Y=$!; reg $Y sy "$R2/sub" agent-y
hook sx Bash "git add -A" "" "$R2"; check "하위 폴더 cwd도 같은 트리(동거)" 2 "agent-y"
rm "$H/.claude/sessions/$Y.json"
ln -s "$R2" "$T/r2link"; reg $Y sy "$T/r2link" agent-y
hook sx Bash "git add -A" "" "$R2"; check "심볼릭 링크 cwd도 같은 트리(동거)" 2 "agent-y"
hook sx Edit "$T/r2link/a.txt" "" "$R2"; echo more >> "$R2/a.txt"
hook sy Edit "$R2/a.txt" "" "$R2"; check "심볼릭 링크로 잡은 임대 = 같은 파일" 2 "agent-x"
rm "$H/.claude/sessions/$Y.json"
git -C "$R2" worktree add -q "$T/r2wt" -b wt 2>/dev/null
reg $Y sy "$T/r2wt" agent-y
hook sx Bash "git add -A" "" "$R2"; check "linked worktree는 다른 트리(동거 아님)" 0
hook sy Edit "$T/r2wt/a.txt" "" "$T/r2wt"; check "linked worktree의 같은 경로 파일은 따로" 0
hook sy Bash "cd $T/r2wt && git rebase main" "" "$R2"; check "cd로 다른 워크트리에서 rebase 허용" 0
rm "$H/.claude/sessions/$Y.json"
git -C "$R2" worktree add -q "$R2/.claude/worktrees/w" -b nested 2>/dev/null
reg $Y sy "$R2/.claude/worktrees/w" agent-y
hook sx Bash "git add -A" "" "$R2"; check "트리 안에 둔 worktree는 다른 트리" 0
rm "$H/.claude/sessions/$Y.json"
mkrepo "$T/r2.web"; reg $Y sy "$T/r2.web" agent-y
hook sx Bash "git add -A" "" "$R2"; check "이름이 비슷한 형제 폴더(r2.web)는 다른 트리" 0
hook sx Edit "/etc/hosts" "" "$R2"; check "git 밖 파일은 허용" 0

# ---------- hc ls as a second signal ----------
printf '{"terminals":[{"address":"t9","agent":"codex","cwd":"%s","state":"live","name":"codex-lane"}]}\n' "$R2" > "$T/hc.json"
hook sx Bash "git stash" "" "$R2"; check "hc ls의 codex 터미널도 동거" 2 "codex-lane"
printf '{"terminals":[{"address":"t9","agent":"zsh","cwd":"%s","state":"live"},{"address":"w1/t1","agent":"codex","cwd":"%s","state":"live"}]}\n' "$R2" "$R2" > "$T/hc.json"
hook sx Bash "git stash" "" "$R2"; check "셸·다른 기계 터미널은 동거 아님" 0
rm "$T/hc.json"

# ---------- 수정 1차 (감사 2026-11-04: H1 H2 M1 M2 M3 L1 L2 L3 L4 L6 L8) ----------
hookw() {   # SESSION TOOL PATH CONTENT  — Write/Edit carrying file content
  local inp
  inp=$(jq -cn --arg s "$1" --arg t "$2" --arg f "$3" --arg c "$4" --arg w "$R" \
    '{session_id: $s, cwd: $w, tool_name: $t, tool_input: ({file_path: $f} + (if $t == "Write" then {content: $c} else {old_string: "x", new_string: $c} end))}')
  OUT=$(printf '%s' "$inp" | env -u CLAUDE_CONFIG_DIR -u HC_TERMINAL_ID HOME="$H" PATH="$T/bin:$PATH" bash "$HOOK" 2>"$T/err")
  RC=$? ERR=$(cat "$T/err")
}
R3=$T/r3; mkrepo "$R3"
(cd "$R3" && printf '.qa/\nlogs/\n' > .gitignore && mkdir -p src/pkg && echo p > src/pkg/x.go && git add -A && git -c user.name=t -c user.email=t@t commit -qm more)
agent; A3=$!; reg $A3 s3a "$R3" agent-a3
agent; B3=$!; reg $B3 s3b "$R3" agent-b3
hook s3a Edit "$R3/src/pkg/x.go" "" "$R3"; echo y >> "$R3/src/pkg/x.go"

for c in "rm -rf src" "rm -rf src/pkg/" "mv src src2" "cp -r /tmp src/pkg" "rm -rf ./*" "rm -rf $R3/src"; do
  hook s3b Bash "$c" "" "$R3"; check "H1 남의 미커밋 파일이 든 폴더 대상 쓰기 차단: $c" 2 "agent-a3"
done
hook s3b Bash "rm -rf sub" "" "$R3"; check "H1 남의 미커밋 파일 없는 폴더는 허용" 0
[ -d "$(dirname "$(leasefile "$R3" e.txt)")" ]; RC=$?; check "막힌 Bash 명령은 앞쪽 대상에도 임대를 남기지 않음(rm -rf ./* 의 e.txt)" 1
hook s3b Bash "cp a.txt sub/" "" "$R3"; [ -d "$(dirname "$(leasefile "$R3" sub)")" ]; RC=$?; check "L6 폴더 대상은 임대를 잡지 않음" 1
echo z >> "$R3/sub/c.txt"
hook s3a Bash "git pull --rebase" "" "$R3"; check "L6 폴더 임대 탓에 남의 rebase가 막히지 않음" 0
git -C "$R3" checkout -q -- sub/c.txt

hook s3a Edit "$R3/a.txt" "" "$R3"; echo y >> "$R3/a.txt"; git -C "$R3" add a.txt
hook s3b Edit "$R3/b.txt" "" "$R3"; echo y >> "$R3/b.txt"
hook s3b Bash "git commit -m 'b fix'" "" "$R3"; check "M1 경로 없는 commit에 남이 stage한 파일이 있으면 차단" 2 "agent-a3"
hook s3b Bash "git commit --amend --no-edit" "" "$R3"; check "M1 commit --amend도 같은 판정" 2
hook s3b Bash "git commit -m 'b fix' -- b.txt" "" "$R3"; check "M1 경로 지정 commit은 허용" 0
for c in "git add *.txt" "git add '*.txt'" "git add ':(glob)**/*.go'" "git add \$(git ls-files -m)" "git ls-files -m | xargs git add" "for f in a b; do git add \"\$f\"; done"; do
  hook s3b Bash "$c" "" "$R3"; check "M1 글롭·판정 못 하는 git add 차단: $c" 2
done
hook s3b Bash "git add 'b*.txt'" "" "$R3"; check "M1 남의 파일과 안 겹치는 글롭은 허용" 0
hook sx Bash "git add \$(git ls-files -m)" "" "$R2"; check "M1 혼자면 판정 못 하는 git add도 허용" 0

for ag in bun node go vite make; do
  printf '{"terminals":[{"address":"t9","agent":"%s","cwd":"%s","state":"live","name":"dev"}]}\n' "$ag" "$R2" > "$T/hc.json"
  hook sx Bash "git stash" "" "$R2"; check "M2 hc의 $ag 터미널은 에이전트 아님" 0
done
for ag in claude-code codex agy grok cursor-agent gemini opencode; do
  printf '{"terminals":[{"address":"t9","agent":"%s","cwd":"%s","state":"live","name":"dev"}]}\n' "$ag" "$R2" > "$T/hc.json"
  hook sx Bash "git stash" "" "$R2"; check "M2 hc의 $ag 터미널은 에이전트" 2 "dev"
done
rm -f "$T/hc.json"

CMDLINE=$(jq -r '.hooks.PreToolUse[]?.hooks[]? | select(.command | test("cotenant-guard")) | .command' "$HOME/.claude/settings.json" | head -1)
case $CMDLINE in *'[ -r '*'|| exit 0'*) RC=0 ;; *) RC=1 ;; esac; check "M3 이 기계 설정 등록 명령은 훅 파일이 없을 때만 허용하는 꼴" 0
mkdir -p "$T/h2/.claude/hooks"
printf '{}' | env HOME="$T/h2" /bin/sh -c "$CMDLINE" >/dev/null 2>&1; RC=$?; check "M3 훅 파일이 없으면 허용" 0
printf '#!/bin/bash\nexit 2\n' > "$T/h2/.claude/hooks/cotenant-guard.sh"
printf '{}' | env HOME="$T/h2" /bin/sh -c "$CMDLINE" >/dev/null 2>&1; RC=$?; check "M3 훅 파일이 있으면 그 판정(2)" 2
printf '#!/bin/bash\nexit 1\n' > "$T/h2/.claude/hooks/cotenant-guard.sh"
printf '{}' | env HOME="$T/h2" /bin/sh -c "$CMDLINE" >/dev/null 2>&1; RC=$?; check "M3 훅 내부 오류는 그대로 0이 아님(차단)" 1

mkdir -p "$T/badbin"
for c in stat md5 jq awk date ps touch mkdir rmdir mv rm; do printf '#!/bin/sh\necho broken\nexit 1\n' > "$T/badbin/$c"; chmod +x "$T/badbin/$c"; done
hook s3a Edit "$R3/g.txt" "" "$R3"
OUT=$(jq -cn --arg w "$R3" --arg f "$R3/g.txt" '{session_id:"s3b",cwd:$w,tool_name:"Edit",tool_input:{file_path:$f}}' |
  env -u CLAUDE_CONFIG_DIR -u HC_TERMINAL_ID HOME="$H" PATH="$T/badbin:$T/bin:$PATH" bash "$HOOK" 2>"$T/err"); RC=$? ERR=$(cat "$T/err")
check "L3 PATH 앞의 가짜 stat·md5·jq·awk·date·ps 등을 쓰지 않음" 2 "10분 안"

cat > "$T/self.sh" <<'IN'
# acts as the Claude process: owns a lease recorded with other spacing, then runs the hook as its direct child
H=$1 HOOK=$2 R=$3 T=$4
st=$(LC_ALL=C TZ=UTC /bin/ps -o lstart= -p $$ | sed 's/ *$//; s/ /  /g')
ld=$(git -C "$R" rev-parse --absolute-git-dir)/agent-leases/$(/sbin/md5 -q -s h.txt); mkdir -p "$ld"
printf 'old-session\037%s\037%s\037\037me\0372026-01-01 00:00:00\037h.txt\n' $$ "$st" > "$ld/owner"
echo z >> "$R/h.txt"
printf '{"session_id":"unregistered","cwd":"%s","tool_name":"Edit","tool_input":{"file_path":"%s/h.txt"}}' "$R" "$R" |
  env -u CLAUDE_CONFIG_DIR -u HC_TERMINAL_ID HOME="$H" PATH="$T/bin:$PATH" /bin/bash "$HOOK"
IN
OUT=$(bash "$T/self.sh" "$H" "$HOOK" "$R3" "$T" 2>&1); RC=$?; check "L1 레지스트리에 없는 세션도 자기 임대(공백 표기만 다름)는 허용" 0

mkdir -p "$R3/.qa" "$R3/logs"
hook s3a Write "$R3/.qa/report.md" "" "$R3"; printf 'r\n' > "$R3/.qa/report.md"
hook s3b Edit "$R3/.qa/report.md" "" "$R3"; check "L2 무시된 파일은 10분 유예 없이 넘겨받음" 0
hook s3a Bash "echo step1 >> logs/progress.log" "" "$R3"
hook s3b Bash "echo step2 >> logs/progress.log" "" "$R3"; check "L2 무시된 공유 로그 >> 허용" 0

for c in "bash -c 'git stash'" "sh -c \"git add -A\"" "zsh -lc 'cd sub && git reset --hard'" "eval git stash" "eval 'git add -A'" "timeout 30 git stash" "timeout -s KILL 5m git add -A" "nice git reset --hard" "nice -n 5 git stash" "xargs git add < list"; do
  hook s3b Bash "$c" "" "$R3"; check "L4 감싼 명령도 검사: $c" 2
done
hook s3b Bash "bash -c 'echo hi; ls'" "" "$R3"; check "L4 감싼 무해한 명령은 허용" 0
c=$(printf 'git commit -m "$(cat <<'"'"'EOF'"'"'\nfix: 27\" 모니터 대응\nrm 금지 규칙 문서화\ngit stash 언급\nEOF\n)" -- b.txt')
hook s3b Bash "$c" "" "$R3"; check "L4 heredoc 본문의 홀수 따옴표에 짝이 뒤집히지 않음" 0
[ -d "$(dirname "$(leasefile "$R3" 금지)")" ]; RC=$?; check "L4 heredoc 본문 낱말로 유령 임대가 생기지 않음" 1

hook s3b Bash "printf '{\"disableAllHooks\":true}' > .claude/settings.local.json" "" "$R3"; check "L8 Bash로 disableAllHooks 쓰기 차단" 2 "사람만"
hook sx Bash "rm -rf .git/agent-leases" "" "$R2"; check "L8 임대 폴더 지우기 차단(혼자여도)" 2 "사람만"
hook sx Bash "echo AGENT_LEASE_OFF" "" "$R2"; check "L8 AGENT_LEASE_OFF 글자가 든 Bash 차단" 2
hookw s3b Write "$R3/.claude/settings.local.json" '{"disableAllHooks": true}'; check "L8 Write로 disableAllHooks 넣기 차단" 2 "사람만"
hookw s3b Edit "$H/.claude/settings.json" '"AGENT_LEASE_OFF": "1"'; check "L8 Edit로 AGENT_LEASE_OFF 넣기 차단" 2
hookw s3b Write "$R3/.claude/settings.local.json" '{"permissions": {}}'; check "L8 다른 설정 편집은 허용" 0

L3D=$(git -C "$R3" rev-parse --absolute-git-dir)/agent-leases
for k in $(seq 1 300); do
  mkdir -p "$L3D/dead$k"
  printf 'sz\037%s\037Mon Jan  1 00:00:00 2001\037\037gone\0372026-01-01 00:00:00\037ghost%s.txt\n' $((90000 + k)) $k > "$L3D/dead$k/owner"
done
mkdir -p "$L3D/deaddirty"; printf 'sz\03799999\037Mon Jan  1 00:00:00 2001\037\037gone\0372026-01-01 00:00:00\037d.txt\n' > "$L3D/deaddirty/owner"
echo q >> "$R3/d.txt"
n0=$(ls "$L3D" | wc -l)
hook s3b Bash "git add b.txt" "" "$R3"; check "H2 임대를 정리하는 판정도 통과" 0
n1=$(ls "$L3D" | wc -l)
[ $((n0 - n1)) -eq 300 ] && [ -d "$L3D/deaddirty" ] && [ -f "$(leasefile "$R3" src/pkg/x.go)" ]; RC=$?
check "H2 죽은 소유자의 깨끗한 임대 300개만 지움(n0=$n0 n1=$n1, 미커밋·산 임대는 남김)" 0

# ---------- 수정 2차 (재감사: N1 남은 claim, N3 큰따옴표 안 heredoc, N5 git status 실패) ----------
R4=$T/r4; mkrepo "$R4"
agent; A4=$!; reg $A4 s4a "$R4" agent-a4
agent; B4=$!; reg $B4 s4b "$R4" agent-b4
L4D=$(git -C "$R4" rev-parse --absolute-git-dir)/agent-leases; mkdir -p "$L4D"
deadlease() {   # REL [old|new|olddir] — a dead owner's lease on REL, optionally with a claim left behind
  local d c; d=$L4D/$(md5 -q -s "$1"); c=$d/claim-99991-20260101000000; mkdir -p "$d"
  printf 'sz\03799991\037Mon Jan  1 00:00:00 2001\037\037gone\0372026-01-01 00:00:00\037%s\n' "$1" > "$d/owner"
  case ${2-} in
    old) : > "$c"; touch -t 202601010000 "$c" ;;
    new) : > "$c" ;;
    olddir) mkdir "$c"; touch -t 202601010000 "$c" ;;
  esac
  echo "$d"
}
d=$(deadlease b.txt old)
hook s4b Edit "$R4/b.txt" "" "$R4"; check "N1 60초 넘게 묵은 claim은 지우고 넘겨받음" 0
cut -d $'\037' -f2 "$d/owner" | grep -qx "$B4"; RC=$?; check "N1 넘겨받은 뒤 소유자는 B" 0
deadlease d.txt new > /dev/null
hook s4b Edit "$R4/d.txt" "" "$R4"; check "N1 방금 생긴 claim은 막되 곧바로 되풀이하지 말라고 알림" 2 "60초"
deadlease e.txt olddir > /dev/null
hook s4b Edit "$R4/e.txt" "" "$R4"; check "N1 옛 판의 claim 폴더도 묵었으면 넘겨받음" 0
d=$(deadlease f.txt old)
hook s4b Bash "git add a.txt" "" "$R4"; [ -d "$d" ]; RC=$?; check "N1 정리도 묵은 claim을 지우고 그 임대를 치움" 1
for k in $(seq 1 200); do deadlease "ghost$k.txt" > /dev/null; done
hook s4b Bash "git add a.txt" "" "$R4"
left=0; for k in $(seq 1 200); do [ -d "$L4D/$(md5 -q -s ghost$k.txt)" ] && left=$((left + 1)); done
RC=$left; check "N1 정리한 임대는 훅이 끝나기 전에 제자리에서 사라짐(claim만 남는 틈 없음)" 0

hook s4a Edit "$R4/g.txt" "" "$R4"; echo y >> "$R4/g.txt"
c=$(printf 'git commit -m "$(cat <<'"'"'END-MSG'"'"'\nmsg\nEND-MSG\n)" -- b.txt && git stash')
hook s4b Bash "$c" "" "$R4"; check "N3 하이픈 든 heredoc 구분자 뒤 git stash도 검사" 2
hook s4b Bash 'echo "$(date) <<EOF" ; git stash' "" "$R4"; check "N3 heredoc이 아닌 <<낱말 뒤 git stash도 검사" 2
c=$(printf 'git commit -m "$(cat <<'"'"'END-MSG'"'"'\nfix: 27\" 모니터\ngit stash 언급\nEND-MSG\n)" -- b.txt')
hook s4b Bash "$c" "" "$R4"; check "N3 하이픈 구분자 heredoc 본문은 여전히 건너뜀" 0

d=$(deadlease h.txt); echo z >> "$R4/h.txt"
cp "$R4/.git/index" "$T/index.bak"; printf 'garbage' > "$R4/.git/index"
hook s4b Bash "git add a.txt" "" "$R4"
[ -d "$d" ]; RC=$?; check "N5 git status가 실패하면 정리를 건너뜀(죽은 소유자의 미커밋 임대 보존)" 0
cp "$T/index.bak" "$R4/.git/index"

# ---------- 수정 3차 (10개 동시 시험 stress10: S1~S7, 막힘 문구) ----------
# race N PREFIX TOOL ARG CWD -> PASSED (how many got rc 0), RACE_MS (wall time): agents PREFIX1..N call the hook at one instant
race() {
  local k js= t0; t0=$(perl -MTime::HiRes=time -e 'printf "%.3f", time + 0.5'); rm -f "$T"/race.*
  for k in $(seq 1 "$1"); do
    ( inp=$(jq -cn --arg s "$2$k" --arg t "$3" --arg v "$4" --arg w "$5" \
        '{session_id: $s, cwd: $w, tool_name: $t, tool_input: (if $t == "Bash" then {command: $v} else {file_path: $v} end)}')
      perl -MTime::HiRes=time,sleep -e 'my $w = shift() - time; sleep $w if $w > 0' "$t0"
      printf '%s' "$inp" | env -u CLAUDE_CONFIG_DIR -u HC_TERMINAL_ID HOME="$H" PATH="$T/bin:$PATH" bash "$HOOK" >/dev/null 2>&1
      echo $? > "$T/race.$k" ) & js="$js $!"
  done
  wait $js
  RACE_MS=$(perl -MTime::HiRes=time -e 'printf "%d", (time - shift) * 1000' "$t0")
  PASSED=$(cat "$T"/race.* | grep -cx 0)
}
rec() {   # LEASES REL PID PROCSTART [old] — a lease record (old: owner untouched since 2026-01-01); prints its folder
  local d=$1/$(md5 -q -s "$2"); mkdir -p "$d"
  printf 's\037%s\037%s\037\037owner-%s\0372026-01-01 00:00:00\037%s\n' "$3" "$4" "$3" "$2" > "$d/owner"
  [ "${5-}" = old ] && touch -t 202601010000 "$d/owner"
  echo "$d"
}
DEADST="Mon Jan  1 00:00:00 2001"
R5=$T/r5; mkrepo "$R5"
(cd "$R5" && printf '.qa/\n' > .gitignore && git add .gitignore && git -c user.name=t -c user.email=t@t commit -qm ignore)
agent; A5=$!; reg $A5 s5a "$R5" agent-a5
agent; B5=$!; reg $B5 s5b "$R5" agent-b5
ST5=$(LC_ALL=C TZ=UTC ps -o lstart= -p $A5 | sed 's/ *$//')
L5D=$(git -C "$R5" rev-parse --absolute-git-dir)/agent-leases; mkdir -p "$L5D"

g=$(rec "$L5D" ghost1.txt 99992 "$DEADST")
: > "$L5D/.cleaning"
hook s5b Bash "git add b.txt" "" "$R5"; [ -d "$g" ]; RC=$?; check "S1 다른 쪽이 정리 중(정리 잠금 60초 안)이면 정리를 건너뜀" 0
touch -t 202601010000 "$L5D/.cleaning"
hook s5b Bash "git add b.txt" "" "$R5"; [ -d "$g" ]; RC=$?; check "S1 묵은 정리 잠금(60초 넘음)은 치우고 정리" 1
[ -e "$L5D/.cleaning" ]; RC=$?; check "S1 정리가 끝나면 정리 잠금을 남기지 않음" 1

i1=$(rec "$L5D" idle.txt $A5 "$ST5" old)
i2=$(rec "$L5D" recent.txt $A5 "$ST5")
i3=$(rec "$L5D" d.txt $A5 "$ST5" old); echo y >> "$R5/d.txt"
mkdir -p "$R5/.qa"; echo r > "$R5/.qa/r.md"; i4=$(rec "$L5D" .qa/r.md $A5 "$ST5")
hook s5b Bash "git add b.txt" "" "$R5"
[ -d "$i1" ]; RC=$?; check "S2 산 소유자의 깨끗하고 10분 넘게 안 쓴 임대는 정리(넘겨받기와 같은 조건)" 1
[ -d "$i2" ] && [ -d "$i3" ]; RC=$?; check "S2 산 소유자의 최근 임대와 미커밋 임대는 남김" 0
[ -d "$i4" ]; RC=$?; check "S2 산 소유자의 무시된 파일 임대는 10분 안이어도 정리(유예 없음 규칙과 같음)" 1

d=$(rec "$L5D" s3.txt 99993 "$DEADST"); c=$d/claim-99993-20260101000000; : > "$c"; touch -t 202601010000 "$c"
: > "$c.steal-$(stat -f %m "$c")"
hook s5b Edit "$R5/s3.txt" "" "$R5"; check "S3 묵은 claim을 다른 쪽이 이미 치우는 중(steal 표식)이면 막힘" 2 "넘겨받기"
o=$L5D/$(md5 -q -s s4.txt); mkdir -p "$o"; : > "$o/claim-orphan"; touch -t 202601010000 "$o"
hook s5b Write "$R5/s4.txt" "" "$R5"; check "S4 주인 기록 없는 임대 폴더는 고정 이름 claim(새것이 있으면 막힘)" 2 "넘겨받기"

echo y >> "$R5/b.txt"
hook s5b Bash "git commit -m 'b'" "" "$R5"; check "S5 동거 중 경로 없는 git commit은 stage에 남의 파일이 없어도 막고 경로를 안내" 2 "git commit -m … -- <경로>"
hook s5b Bash "git commit -m b --" "" "$R5"; check "S5 -- 뒤에 경로가 없어도 경로 없는 commit" 2
hook s5b Bash "git commit -m b -- b.txt" "" "$R5"; check "S5 경로 지정 commit은 허용" 0
hook sx Bash "git commit -m x" "" "$R2"; check "S5 혼자면 경로 없는 git commit 허용" 0
hook s5b Bash "git commit --amend -m b -- b.txt" "" "$R5"; check "S6 동거 중 --amend는 경로가 있어도 막힘" 2 "amend"
hook s5b Bash "git commit --amen --no-edit" "" "$R5"; check "S6 --amend 줄임(--amen)도 막힘" 2
hook sx Bash "git commit --amend --no-edit" "" "$R2"; check "S6 혼자면 --amend 허용" 0

hook s5a Edit "$R5/e.txt" "" "$R5"; echo y >> "$R5/e.txt"
OUT=$(jq -cn --arg w "$R5" --arg f "$R5/e.txt" '{session_id:"s5b",cwd:$w,tool_name:"Edit",tool_input:{file_path:$f}}' |
  env -u CLAUDE_CONFIG_DIR -u HC_TERMINAL_ID HOME="$H" PATH="$T/bin" /bin/bash "$HOOK" 2>"$T/err"); RC=$? ERR=$(cat "$T/err")
check "S7 PATH에 git이 없어도 같은 판정(남의 미커밋 파일 차단)" 2 "agent-a5"
printf '#!/bin/sh\necho broken >&2\nexit 1\n' > "$T/badgit"; chmod +x "$T/badgit"
if [ "$HERDR_HOOKS_IMPL" = go ]; then BADGIT_HOOK=$HOOK BADGIT_ENV="HERDR_HOOKS_TEST_GIT=$T/badgit"
else sed "s|^GIT=.*|GIT=$T/badgit|" "$HOOK" > "$T/hook-badgit.sh"; BADGIT_HOOK=$T/hook-badgit.sh BADGIT_ENV=; fi
OUT=$(jq -cn --arg w "$R2" --arg f "$R2/a.txt" '{session_id:"sx",cwd:$w,tool_name:"Edit",tool_input:{file_path:$f}}' |
  env -u CLAUDE_CONFIG_DIR -u HC_TERMINAL_ID HOME="$H" PATH="$T/bin:$PATH" $BADGIT_ENV /bin/bash "$BADGIT_HOOK" 2>"$T/err"); RC=$? ERR=$(cat "$T/err")
check "S7 git을 실행하지 못하면(종료 128 밖) 허용하지 않고 막음" 2 "git을 실행하지 못했다"
hook s5b Edit "$R5/e.txt" "" "$R5"; check "문구: 기다리라 대신 다른 일을 먼저 하고 나중에 다시 시도하라" 2 "다른 일을 먼저 하고 나중에 다시 시도"
: > "$L5D/.cleaning"
hook s5b Bash "git add e.txt" "" "$R5"; check "S2 정리 잠금을 남이 쥔 동안(바뀐 경로의 임대만 읽음)도 남의 미커밋 파일 git add 차단" 2 "agent-a5"
rm -f "$L5D/.cleaning"

R6=$T/r6; mkrepo "$R6"; L6D=$(git -C "$R6" rev-parse --absolute-git-dir)/agent-leases; mkdir -p "$L6D"
for k in $(seq 1 10); do agent; reg $! s6c$k "$R6" agent-6c$k; [ $k = 1 ] && P6=$!; done
ST6=$(LC_ALL=C TZ=UTC ps -o lstart= -p $P6 | sed 's/ *$//')
f6=$(rec "$L6D" sub $P6 "$ST6"); echo z >> "$R6/sub/c.txt"
hook s6c2 Bash "git rebase main" "" "$R6"; check "S2 옛 판의 폴더 임대 아래 미커밋 파일도 남의 것(모든 임대를 읽을 때)" 2 "sub("
: > "$L6D/.cleaning"
hook s6c2 Bash "git rebase main" "" "$R6"; check "S2 같은 것을 정리 잠금을 남이 쥔 동안(바뀐 경로·그 위 폴더의 임대만 읽음)에도" 2 "sub("
rm -f "$L6D/.cleaning"; git -C "$R6" checkout -q -- sub/c.txt; rm -rf "$f6"
for rep in 1 2 3; do
  d=$(rec "$L6D" "s3-$rep.txt" 99994 "$DEADST"); c=$d/claim-99994-20260101000000; : > "$c"; touch -t 202601010000 "$c"
  race 10 s6c Write "$R6/s3-$rep.txt" "$R6"; RC=$PASSED; check "S3 묵은 claim이 남은 죽은 임대를 10개가 동시에 넘겨받으면 하나만 ($rep)" 1
done
for rep in 1 2 3; do
  o=$L6D/$(md5 -q -s "s4-$rep.txt"); mkdir -p "$o"; touch -t 202601010000 "$o"
  race 10 s6c Write "$R6/s4-$rep.txt" "$R6"; RC=$PASSED; check "S4 주인 기록 없는 임대 폴더를 10개가 동시에 넘겨받으면 하나만 ($rep)" 1
done
mkdir -p $(for k in $(seq 1 1000); do printf '%s/g%s ' "$L6D" $k; done)
for k in $(seq 1 1000); do printf 'sz\03799995\037%s\037\037gone\0372026-01-01 00:00:00\037ghost/g%s.txt\n' "$DEADST" $k > "$L6D/g$k/owner"; done
race 5 s6c Bash "git add a.txt" "$R6"; RC=$PASSED; check "S1 죽은 임대 1000개를 5개가 동시에 정리해도 모두 통과" 5
[ "$RACE_MS" -lt 4000 ]; RC=$?; check "S1 그때 가장 늦은 판정도 4초 안(${RACE_MS}ms, 훅 제한 5초)" 0
# 시간 시험은 맥 부하에 기대지 않게(2026-11-18 hcfill-early 착지: 레인들의 Go 시험으로 1분 부하 50~65, CPU 18개에서 S2·R2가 흔들림).
# 시간에 기대는 판정 앞에서 calm이 1분 부하가 CPU 수 아래로 내리기를 10초마다 기다린다(이 시험 파일 전체에서 합쳐 최대
# COTENANT_CALM_MAX초, 기본 600). 그래도 부하가 CPU 수 이상이면 시간 검사(ms 상한)만 「부하로 건너뜀」 한 줄로 남기고 통과시키고,
# 동작 검사(판정 결과·지움·차단)는 그대로 본다. 부하가 낮으면 원래 상한(1.5초·3.5초) 그대로
msince() { perl -MTime::HiRes=time -e 'printf "%d", (time - shift) * 1000' "$1"; }
CALMW=0 BUSY=
calm() {
  local nc l mx=${COTENANT_CALM_MAX:-600}; nc=$(sysctl -n hw.ncpu 2>/dev/null || echo 4)
  while :; do
    l=$(sysctl -n vm.loadavg 2>/dev/null | awk '{ print int($2) }')
    if [ -z "$l" ] || [ "$l" -lt "$nc" ]; then BUSY=; return 0; fi
    [ $CALMW -lt "$mx" ] || { BUSY="1분 부하 $l ≥ CPU $nc"; echo "calm: $BUSY — 합쳐 ${CALMW}초 기다려도 안 내려 그대로 잰다"; return 0; }
    sleep 10; CALMW=$((CALMW + 10))
  done
}
timeok() {  # timeok <ms> <상한 ms>: 상한 안이면 0. 부하(BUSY)면 시간 검사만 건너뛰고 0, 한 줄
  [ -z "$BUSY" ] || { echo "부하로 건너뜀: 시간 검사(${1}ms, 상한 ${2}ms) — $BUSY"; return 0; }
  [ "$1" -lt "$2" ]
}
calm
mkdir -p $(for k in $(seq 1 3000); do printf '%s/v%s ' "$L6D" $k; done)
for k in $(seq 1 3000); do printf 's\037%s\037%s\037\037agent-6c1\0372026-11-04 23:00:00\037live/x%s.txt\n' "$P6" "$ST6" $k > "$L6D/v$k/owner"; done
calm; t0=$(perl -MTime::HiRes=time -e 'printf "%.3f", time')
hook s6c2 Bash "git add a.txt" "" "$R6"; ms=$(msince "$t0")
[ "$RC" = 0 ] && timeok "$ms" 1500; RC=$?; check "S2 남의 산 임대 3000개가 있어도 git add 판정 1.5초 안(${ms}ms)" 0
[ "$(ls "$L6D" | grep -c '^v')" = 3000 ]; RC=$?; check "S2 최근에 쓴 산 임대 3000개는 정리하지 않음" 0

# ---------- 수정 4차 (재확인 1: R1 steal 표식만 남은 claim, R2 정리자 시간 상한) ----------
cat > "$T/date61" <<'IN'
#!/bin/sh
# the hook's clock 61 s on: a claim re-armed just now has gone stale again
[ "$1" = +%s ] && { echo $(( $(/bin/date +%s) + 61 )); exit 0; }
exec /bin/date "$@"
IN
chmod +x "$T/date61"
if [ "$HERDR_HOOKS_IMPL" = go ]; then hook61() { HOOK_ENV=HERDR_HOOKS_TEST_NOW_OFFSET=61 hook "$@"; }
else sed "s|DATE=/bin/date|DATE=$T/date61|" "$HOOK" > "$T/hook-61.sh"; hook61() { local HOOK=$T/hook-61.sh; hook "$@"; }; fi
d=$(rec "$L5D" r1.txt 99996 "$DEADST"); c=$d/claim-99996-20260101000000; : > "$c"; touch -t 202601010000 "$c"
m=$(stat -f %m "$c"); : > "$c.steal-$m"   # a taker killed right after its marker: the claim and the marker are left
hook s5b Edit "$R5/r1.txt" "" "$R5"; check "R1 steal 표식만 남은 묵은 claim: 지금은 막고 60초 뒤 풀린다고 알림" 2 "60초가 지나면"
[ "$(stat -f %m "$c")" -gt "$m" ]; RC=$?; check "R1 묵은 claim을 만난 판정이 그 claim을 다시 무장(mtime = 지금)" 0
hook61 s5b Edit "$R5/r1.txt" "" "$R5"; check "R1 60초 뒤에는 누군가 넘겨받을 수 있음(문구의 '60초 뒤 풀림'이 참)" 0
cut -d $'\037' -f2 "$d/owner" | grep -qx "$B5"; RC=$?; check "R1 넘겨받은 쪽이 주인" 0
for k in $(seq 1 50); do rec "$L5D" "ghost/r1-$k.txt" 99997 "$DEADST" > /dev/null; done
gl() { local k n=0; for k in $(seq 1 50); do [ -d "$L5D/$(md5 -q -s "ghost/r1-$k.txt")" ] && n=$((n + 1)); done; echo $n; }
: > "$L5D/.cleaning"; touch -t 202601010000 "$L5D/.cleaning"; m=$(stat -f %m "$L5D/.cleaning"); : > "$L5D/.cleaning.steal-$m"
hook s5b Bash "git add b.txt" "" "$R5"; check "R1 steal 표식만 남은 묵은 정리 잠금: 판정은 그대로" 0
hook61 s5b Bash "git add b.txt" "" "$R5"; RC=$(gl); check "R1 정리 잠금도 60초 뒤 누군가 다시 잡아 정리(죽은 임대 50개 남은 수)" 0
[ -e "$L5D/.cleaning" ]; RC=$?; check "R1 그 정리도 잠금을 남기지 않음" 1
for k in $(seq 1 5); do rec "$L5D" "ghost/r1-$k.txt" 99997 "$DEADST" > /dev/null; done
HOOK_ENV=SECONDS=99 hook s5b Bash "git add b.txt" "" "$R5"; RC=$(gl); check "R2 환경의 SECONDS가 시간 상한을 앞당기지 않음(남은 죽은 임대 수)" 0
cp "$R5/.git/index" "$T/index5.bak"; printf 'garbage' > "$R5/.git/index"
: > "$L5D/.cleaning"   # someone else cleans: the changed-paths way
hook s5b Bash "git add e.txt" "" "$R5"; check "작업 중 발견: git status가 실패하면 남의 미커밋 파일 git add를 허용하지 않고 막음(바뀐 경로만 읽는 판정)" 2 "git status가 실패"
rm -f "$L5D/.cleaning"
hook s5b Bash "git add e.txt" "" "$R5"; check "작업 중 발견: 같은 것을 정리자가 판정할 때도 막음" 2 "git status가 실패"
[ -e "$L5D/.cleaning" ]; RC=$?; check "작업 중 발견: 그때 정리 잠금을 남기지 않음" 1
cp "$T/index5.bak" "$R5/.git/index"

R7=$T/r7; mkrepo "$R7"; L7D=$(git -C "$R7" rev-parse --absolute-git-dir)/agent-leases; mkdir -p "$L7D"
agent; A7=$!; reg $A7 s7a "$R7" agent-a7
agent; B7=$!; reg $B7 s7b "$R7" agent-b7
hook s7a Edit "$R7/h.txt" "" "$R7"; echo y >> "$R7/h.txt"   # its lease folder (md5 f54c…) sorts after the 0g* ones below
(cd "$L7D" && seq 1 20000 | sed 's/^/0g/' | xargs mkdir)
for k in $(seq 1 20000); do printf 'sz\03799998\037%s\037\037gone\0372026-01-01 00:00:00\037ghost/g%s.txt\n' "$DEADST" $k > "$L7D/0g$k/owner"; done
calm; t0=$(perl -MTime::HiRes=time -e 'printf "%.3f", time')
hook s7b Bash "git add b.txt" "" "$R7"; ms=$(msince "$t0")
[ "$RC" = 0 ] && timeok "$ms" 3500; RC=$?; check "R2 죽은 임대 2만 개(옛 판 맥 4.5초)도 정리 판정이 상한 안에 끝남(${ms}ms, 훅 제한 5초)" 0
# 부하가 크면 준비에 2초를 다 써 한 판정이 하나도 못 읽을 수 있다(설계대로) — 판정을 5번까지 거듭해 정리가 나아가는지 본다
g7() { ls "$L7D" | grep -c '^0g'; }
n1=$(g7); i=1; while [ "$n1" -ge 20000 ] && [ $i -lt 5 ]; do calm; hook s7b Bash "git add b.txt" "" "$R7"; n1=$(g7); i=$((i + 1)); done
[ "$n1" -lt 20000 ]; RC=$?; check "R2 상한까지 읽은 것은 지움(남은 $n1, 판정 ${i}번)" 0
[ -e "$L7D/.cleaning" ]; RC=$?; check "R2 상한에 걸려도 정리 잠금을 풀고 끝냄" 1
calm; t0=$(perl -MTime::HiRes=time -e 'printf "%.3f", time')
hook s7b Bash "git add h.txt" "" "$R7"; ms=$(msince "$t0"); timeok "$ms" 3500 || RC="$RC(${ms}ms)"
check "R2 상한에 걸린 정리 판정도 못 읽은 임대의 남의 미커밋 파일 git add 차단(${ms}ms)" 2 "agent-a7"
n2=$(g7); i=1; while [ "$n2" -ge "$n1" ] && [ $i -lt 5 ]; do calm; hook s7b Bash "git add b.txt" "" "$R7"; n2=$(g7); i=$((i + 1)); done
[ "$n2" -lt "$n1" ]; RC=$?; check "R2 다음 판정이 이어서 정리(남은 $n1 → $n2, 판정 ${i}번)" 0
rm -f "$L7D/.idle"   # written by a scanner that has read some leases
if [ "$HERDR_HOOKS_IMPL" = go ]; then HOOK_ENV=HERDR_HOOKS_TEST_SECONDS=5 hook s7b Bash "git add h.txt" "" "$R7"   # a verdict that reaches the scan already past the limit
else sed 's|^SECONDS=0|SECONDS=5|' "$HOOK" > "$T/hook-late.sh"; HOOK=$T/hook-late.sh hook s7b Bash "git add h.txt" "" "$R7"; fi
check "R2 늦게 시작해 하나도 못 읽은 정리 판정도 남의 미커밋 파일 git add 차단" 2 "agent-a7"
[ -e "$L7D/.idle" ]; RC=$?; check "R2 시간 상한이 지난 정리 판정은 임대를 읽지 않음" 1
[ -e "$L7D/.cleaning" ]; RC=$?; check "R2 그때도 정리 잠금을 남기지 않음" 1
R8=$T/r8; mkrepo "$R8"; L8D=$(git -C "$R8" rev-parse --absolute-git-dir)/agent-leases; mkdir -p "$L8D"
agent; reg $! s8a "$R8" agent-8a
long=ghost/$(printf 'x%.0s' $(seq 1 120))
(cd "$L8D" && seq 1 8000 | sed 's/^/0g/' | xargs mkdir)
for k in $(seq 1 8000); do printf 'sz\03799998\037%s\037\037gone\0372026-01-01 00:00:00\037%s/g%s.txt\n' "$DEADST" "$long" $k > "$L8D/0g$k/owner"; done
calm; hook s8a Bash "git add b.txt" "" "$R8"; n=$(ls "$L8D" | grep -c '^0g')
[ "$RC" = 0 ] && [ "$n" -lt 8000 ]; RC=$?; check "R2 경로가 긴 죽은 임대 8000개(한 status 인자 1.2MB)도 정리가 진행(남은 $n)" 0

# 작업 중 발견(2026-11-21 hooks-go 두 판 대조): 글롭이 펼친 결과를 다시 펼쳐, 닫히지 않은 [ 가 든 대상(펼쳐도 그대로)이나
# 저를 다시 맞추는 이름(q*·q*b)이면 bash_write가 끝없이 되돌아 셸 판이 죽었다(신호로 끝남 → onFailure로 막힘). 펼친 결과는 구체 경로다
touch "$R/q*" "$R/q*b"
for c in "echo x >> build.log[" "rm -f q*" "printf y > 'q*'"; do
  RC=; hook sb Bash "$c"; check "작업 중 발견: 글롭 결과를 다시 펼치지 않음(끝없이 안 돎): $c" 0
done
[ -f "$(leasefile "$R" 'build.log[')" ]; RC=$?; check "작업 중 발견: 닫히지 않은 [ 대상도 보통 파일로 임대" 0
[ -f "$(leasefile "$R" 'q*b')" ]; RC=$?; check "글롭으로 맞은 파일도 이미 미커밋이면 Bash 쓰기가 임대하지 않음(남의 변경을 떠안지 않게)" 1
rm -f "$R/q*" "$R/q*b"

echo "pass $pass, fail $fail"
[ "$fail" = 0 ]
