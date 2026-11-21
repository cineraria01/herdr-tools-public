#!/bin/bash
# Tests for ~/.claude/hooks/lane-no-shell-edit.sh. Feeds PreToolUse JSON through the same wrapper settings.json uses
# (a missing hook file = exit 0, so HOOK=/nonexistent shows the state before the hook). Run: bash <this file>  (exit 0 = pass)
HOOK=${HOOK:-$(cd "$(dirname "$0")/.." && pwd)/lane-no-shell-edit.sh}   # the hook beside this tests folder (repo or ~/.claude/hooks)
. "$(dirname "$0")/hooks-impl.sh"   # 판: HERDR_HOOKS_IMPL=go(기본)·bash
pass=0 fail=0
# case NAME WANT(0 | write | commit | zsh) COMMAND [LANE — 기본 lane-x, 빈 값이면 HC_LANE 없음]
case_() {
  local inp err rc lane=${4-lane-x} want=$2 re
  inp=$(jq -cn --arg c "$3" '{tool_name: "Bash", tool_input: {command: $c}}')
  if [ -n "$lane" ]; then
    err=$(printf '%s' "$inp" | HC_LANE=$lane HOOK="$HOOK" /bin/bash -c '[ -r "$HOOK" ] || exit 0; exec /bin/bash "$HOOK"' 2>&1 >/dev/null)
  else
    err=$(printf '%s' "$inp" | env -u HC_LANE HOOK="$HOOK" /bin/bash -c '[ -r "$HOOK" ] || exit 0; exec /bin/bash "$HOOK"' 2>&1 >/dev/null)
  fi
  rc=$?
  case $want in write) re='Edit·Write 도구로' ;; commit) re='git commit -F' ;; zsh) re='경로를 글자로' ;; *) re= ;; esac
  if { [ "$want" = 0 ] && [ "$rc" = 0 ]; } || { [ "$want" != 0 ] && [ "$rc" = 2 ] && printf '%s' "$err" | grep -qF "$re"; }; then
    pass=$((pass + 1)); echo "ok   $1"
  else
    fail=$((fail + 1)); echo "FAIL $1 (rc=$rc, want $want) err=[$err]"
  fi
}
# 막아야 할 것: 셸로 파일 고치기
case_ "python heredoc open w"        write $'python3 - <<\'EOF\'\np = "bin/hc-lane"\ns = open(p).read().replace("a", "b")\nopen(p, "w").write(s)\nEOF'
case_ "python heredoc write_text"    write $'python3 <<EOF\nfrom pathlib import Path\nPath("README.md").write_text("x")\nEOF'
case_ "python heredoc open a"        write $'cd src && python3 - <<\'PY\'\nwith open("notes.md", \'a\') as f: f.write("x")\nPY'
case_ "node heredoc writeFileSync"   write $'node - <<\'EOF\'\nconst fs = require("fs"); fs.writeFileSync("a.json", "{}")\nEOF'
case_ "python -c open w"             write "python3 -c 'open(\"x.go\",\"w\").write(\"package x\")'"
case_ "sed -i ''"                    write "sed -i '' 's/foo/bar/' bin/hc-lane"
case_ "sed -E -i"                    write 'sed -E -i "s/a/b/g" docs/x.md'
case_ "sed --in-place"               write 'sed --in-place s/a/b/ main.go'
case_ "perl -pi -e"                  write "perl -pi -e 's/a/b/' README.md"
case_ "> 소스 파일"                  write 'echo "package main" > cmd/main.go'
case_ ">> 소스 파일"                 write 'printf "x\n" >> docs/CHANGELOG.md'
case_ '> "따옴표 파일"'              write 'jq . a.json > "settings.json"'
case_ "cat > 파일 heredoc"           write $'cat > CONTRACT.md <<\'EOF\'\n# x\nEOF'
case_ "tee 소스 파일"                write 'echo x | tee -a src/app.ts'
case_ "앞 명령 뒤 && sed -i"         write "cd frontend && sed -i '' s/a/b/ src/App.tsx"
# 막아야 할 것: heredoc 커밋 메시지
case_ 'commit -m "$(cat <<EOF'       commit $'git commit -m "$(cat <<\'EOF\'\nfix: x\n\nCo-Authored-By: y\nEOF\n)" -- a.sh'
case_ "commit -F - <<EOF"            commit $'git commit -F - -- a.sh <<\'EOF\'\nfix: x\nEOF'
case_ "git -C 경로 commit heredoc"   commit $'git -C /repo commit -F - <<EOF\nfix\nEOF'
# 막아야 할 것: zsh 배열·확장 플래그
case_ 'files=(${(f)…}) git add'      zsh 'files=(${(f)"$(git status --porcelain | cut -c4-)"}); git add -- $files'
case_ '${(@)…}'                      zsh 'for f in ${(@f)$(git diff --name-only)}; do echo $f; done'
# 통과할 것: 읽기·검사·시험·임시 폴더·.qa/
case_ "python heredoc 읽기"          0 $'python3 - <<\'EOF\'\nimport json\nprint(json.load(open("a.json"))["k"])\nEOF'
case_ "python heredoc open r"        0 $'python3 - <<\'EOF\'\nprint(open("a.md", "r").read()[:100])\nEOF'
case_ "python heredoc /tmp 쓰기"     0 $'python3 - <<\'EOF\'\nopen("/tmp/x.json", "w").write("{}")\nEOF'
case_ "python heredoc .qa/ 쓰기"     0 $'python3 - <<\'EOF\'\nopen(".qa/2026-11-18/out.md", "w").write("x")\nEOF'
case_ "sed -n | grep"                0 "sed -n '1,40p' bin/hc-lane | grep -n report"
case_ "sed 출력만"                   0 "sed 's/a/b/' a.md"
case_ "sed -i /tmp"                  0 "sed -i '' s/a/b/ /tmp/x.md"
case_ "> /tmp 파일"                  0 'jq . a.json > /tmp/b.json'
case_ "> .qa/ 로그"                  0 'bash .claude/herdr-verify.sh > .qa/2026-11-18/verify.md 2>&1'
case_ '> $TMPDIR'                    0 'go test ./... > "$TMPDIR/out.json"'
case_ "> 변수 대상"                  0 'echo x > "$out"'
case_ "> /dev/null·2>&1·>&2"         0 'bash tests/hc-lane.test.sh >/dev/null 2>&1; echo done >&2'
case_ "> 소스 아닌 파일(.log)"       0 'make > build.log'
case_ "시험 실행"                    0 'bash claude/hooks/tests/lane-no-shell-edit.test.sh'
case_ "go test"                      0 'go test -race ./...'
case_ "commit -m 글자"               0 'git commit -m "fix: sed -i · > a.md 막기" -- a.sh'
case_ "commit -F 파일"               0 'git commit -F /tmp/msg.txt -- a.sh'
case_ "git add 글자 경로"            0 'git add -- bin/hc-lane claude/hooks/x.sh'
case_ "grep 패턴"                    0 "grep -rn 'open(p, \"w\")' bin/ | head"
case_ 'echo 설명 안 ${(f)'           0 "echo '\${(f)…} 대신 글자 경로'"
case_ "cat heredoc 읽기만"           0 $'cat <<\'EOF\'\n> a.md 는 예시\nEOF'
case_ "명령 없음"                    0 ''
# HC_LANE 없으면(지휘·사람 세션) 늘 통과
case_ "HC_LANE 없음: sed -i"         0 "sed -i '' s/a/b/ bin/hc-lane" ''
case_ "HC_LANE 없음: heredoc 커밋"   0 $'git commit -F - <<EOF\nfix\nEOF' ''
case_ "HC_LANE 없음: python 쓰기"    0 $'python3 - <<EOF\nopen("a.go","w").write("x")\nEOF' ''
case_ 'HC_LANE 없음: ${(f)…}'        0 'files=(${(f)"$(git status)"}); git add -- $files' ''
echo "pass=$pass fail=$fail"
[ "$fail" = 0 ]
