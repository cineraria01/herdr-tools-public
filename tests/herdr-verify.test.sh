#!/bin/bash
# .claude/herdr-verify.sh 고아 검사: 시험이 배경 프로세스를 남기면 목록을 찍고 끄고 검증 실패.
# 남기지 않으면 통과. 검증 전부터 돌던 프로세스는 건드리지 않는다. 이 시험도 끝에 고아 0을 확인한다.
set -u
unset HERDR_VERIFY_ALL   # 바깥 검증(HERDR_VERIFY_ALL=1)이 주는 값을 안쪽 검증이 물려받으면 8절 「싼 검사에서 멈춤」이 깨진다
src=$(cd "$(dirname "$0")/.." && pwd)/.claude/herdr-verify.sh
cfgtool=$(cd "$(dirname "$0")/.." && pwd)/bin/herdr-tools-config   # 검증의 개인 값 검사 단계가 부른다
T=$(mktemp -d "${TMPDIR:-/tmp}/hv-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
P=$T/pids
mkdir -p "$P"
pass=0 fail=0
t() { if eval "$2"; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "실패: $1"; fi; }
alive() { kill -0 "$1" 2>/dev/null; }
cleanup() {
  for f in "$P"/*; do [ -f "$f" ] && kill -KILL "$(cat "$f")" 2>/dev/null; done
  rm -rf "$T"
}
trap cleanup EXIT
# 개인 값 패턴은 시험 것만 (이 기계의 진짜 패턴·임시 폴더 경로에 흔들리지 않게)
mkdir -p "$T/conf" && printf 'secret-word\n' >"$T/conf/private-patterns.txt"
export HERDR_TOOLS_CONFIG=$T/conf

# 가짜 저장소(git, 파일은 index에): 검증 스크립트 + herdr-tools-config + 시험 하나.
# bin/idle은 자식 없이 잠자는 스크립트(명령 줄에 저장소 경로가 든다)
mk() {
  R=$T/$1
  mkdir -p "$R/.claude" "$R/bin" "$R/tests"
  cp "$src" "$R/.claude/herdr-verify.sh"
  cp "$cfgtool" "$R/bin/herdr-tools-config"
  printf '#!/usr/bin/env python3\nimport time\ntime.sleep(300)\n' >"$R/bin/idle"
  chmod +x "$R/bin/idle"
  printf '#!/bin/bash\n%s\n' "$2" >"$R/tests/x.test.sh"
  git -C "$R" init -q && git -C "$R" add -- .claude bin tests
}
verify() { out=$(bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?; }
gone() { for _ in 1 2 3 4 5 6 7 8 9 10; do alive "$1" || return 0; sleep 0.2; done; return 1; }
allgone() { for f in "$P"/*; do gone "$(cat "$f")" || return 1; done; }

# 1. 남기지 않는 시험: 통과
mk ok 'exit 0'
verify
t "깨끗한 시험: 통과(종료 0)" '[ $rc = 0 ] && printf "%s" "$out" | grep -q "통과 tests/x.test.sh"'

# 2. 같은 프로세스 무리에 배경 sleep을 남김: 목록·끔·실패
mk grp "sleep 300 </dev/null >/dev/null 2>&1 & echo \$! >'$P/grp'"
verify
t "배경 sleep 남김: 검증 실패" '[ $rc != 0 ]'
t "배경 sleep 남김: 고아 목록에 pid" 'printf "%s" "$out" | grep -q "고아 pid $(cat "$P/grp")"'
t "배경 sleep 남김: 껐다" 'gone "$(cat "$P/grp")"'

# 3. 무리를 벗어나(setsid) 부모가 1이 된 저장소 경로 프로세스: 목록·끔·실패
mk sid "python3 -c 'import os,sys; os.setsid(); os.execv(sys.argv[1], sys.argv[1:])' \"\$(dirname \"\$0\")/../bin/idle\" </dev/null >/dev/null 2>&1 & echo \$! >'$P/sid'"
verify
t "setsid로 빠진 저장소 경로 프로세스: 실패·끔" '[ $rc != 0 ] && printf "%s" "$out" | grep -q "고아 pid $(cat "$P/sid")" && gone "$(cat "$P/sid")"'

# 3b. 명령 줄에 저장소 경로가 없고 cwd로만 저장소에 있는 떼어 띄운 프로세스(sleep): 여전히 잡음, 고아 줄에 cwd
mk cwdonly "cd \"\$(dirname \"\$0\")/..\" && python3 -c 'import os,sys; os.setsid(); os.execvp(\"sleep\", [\"sleep\", \"300\"])' </dev/null >/dev/null 2>&1 & echo \$! >'$P/cwdonly'"
verify
t "cwd로만 저장소인 떼어 띄운 sleep: 실패·끔, 줄에 cwd" '[ $rc != 0 ] && printf "%s" "$out" | grep -q "고아 pid $(cat "$P/cwdonly") (cwd $R)" && gone "$(cat "$P/cwdonly")"'

# 3c. 저장소를 cwd로 떼어 띄운 Claude Code 플러그인·훅 프로세스(명령 줄에 /.claude/, 저장소 경로 없음):
# 레인의 Claude 세션 소속이라 고아가 아니다 — 안 잡고 안 끔(2026-11-19 mini-m4 fill-name-guard 착지가 OMC node를 잡음)
mkdir -p "$T/home/.claude/plugins/fake"
printf '#!/usr/bin/env python3\nimport time\ntime.sleep(300)\n' >"$T/home/.claude/plugins/fake/idle"
chmod +x "$T/home/.claude/plugins/fake/idle"
mk plug "python3 -c 'import os,sys; os.setsid(); os.execv(sys.argv[1], sys.argv[1:])' '$T/home/.claude/plugins/fake/idle' </dev/null >/dev/null 2>&1 & echo \$! >'$P/plug'"
verify
t "cwd가 저장소인 /.claude/ 플러그인 프로세스: 통과, 그대로 삶" '[ $rc = 0 ] && ! printf "%s" "$out" | grep -q "고아 pid" && alive "$(cat "$P/plug")"'
kill "$(cat "$P/plug")"

# 3d. 명령 줄에 /.claude/가 들어도 저장소 경로가 같이 들면(저장소의 .claude/ 스크립트) 여전히 잡음
printf '#!/usr/bin/env python3\nimport time\ntime.sleep(300)\n' >"$T/dot-idle"
mk dot "cp '$T/dot-idle' .claude/idle && chmod +x .claude/idle && python3 -c 'import os,sys; os.setsid(); os.execv(sys.argv[1], sys.argv[1:])' \"\$PWD/.claude/idle\" </dev/null >/dev/null 2>&1 & echo \$! >'$P/dot'"
verify
t "저장소 .claude/ 아래 프로세스: 실패·끔" '[ $rc != 0 ] && printf "%s" "$out" | grep -q "고아 pid $(cat "$P/dot")" && gone "$(cat "$P/dot")"'

# 4. 검증 전부터 돌던 저장소 경로 프로세스(부모 1)는 건드리지 않음
mk pre 'exit 0'
( "$R/bin/idle" </dev/null >/dev/null 2>&1 & echo $! >"$P/pre" )
for _ in 1 2 3 4 5 6 7 8 9 10; do [ "$(ps -o ppid= -p "$(cat "$P/pre")" | tr -d ' ')" = 1 ] && break; sleep 0.2; done
verify
t "검증 전부터 돌던 프로세스: 통과, 그대로 삶" '[ $rc = 0 ] && alive "$(cat "$P/pre")"'
kill "$(cat "$P/pre")"

# 5. 검증이 TERM으로 끝나면 돌던 시험 무리도 같이 끔
mk int "echo \$\$ >'$P/int-sh'
sleep 300 </dev/null >/dev/null 2>&1 & echo \$! >'$P/int'
wait"
bash "$R/.claude/herdr-verify.sh" >/dev/null 2>&1 &
vp=$!
for _ in $(seq 50); do [ -s "$P/int" ] && break; sleep 0.1; done
kill -TERM "$vp"
{ wait "$vp"; } 2>/dev/null
vrc=$?
t "검증이 TERM으로 끝남: 종료 143, 시험 무리의 배경 sleep도 끔" '[ $vrc = 143 ] && [ -s "$P/int" ] && gone "$(cat "$P/int")" && gone "$(cat "$P/int-sh")"'

# 6. 개인 값 검사 단계: 저장소 파일이 패턴에 걸리면 실패, 안 걸리면 통과 줄
mk ok2 'exit 0'
verify
t "개인 값 없음: 통과 개인 값 검사" '[ $rc = 0 ] && printf "%s" "$out" | grep -q "통과 개인 값 검사"'
mk leak 'echo secret-word'
verify
t "개인 값 있음: 검증 실패, 걸린 줄" '[ $rc != 0 ] && printf "%s" "$out" | grep -q "실패 개인 값 검사" && printf "%s" "$out" | grep -q "tests/x.test.sh:2: secret-word"'

# 7. 패턴 파일이 없는 기계: 건너뛰지 않고 실패(검사 없이 착지 못 하게). 명시적 HERDR_VERIFY_NO_LINT=1만 건너뜀
mk nopat 'exit 0'
out=$(HOME=$T/home HERDR_TOOLS_CONFIG=$T/없음 bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?
t "패턴 파일 없음: 실패 개인 값 검사, herdr-sync 안내" '[ $rc != 0 ] && printf "%s" "$out" | grep -q "실패 개인 값 검사" && printf "%s" "$out" | grep -q "herdr-sync"'
out=$(HC_LANE= HOME=$T/home HERDR_TOOLS_CONFIG=$T/없음 HERDR_VERIFY_NO_LINT=1 bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?
t "패턴 파일 없음 + HERDR_VERIFY_NO_LINT=1: 건너뜀 줄, 통과" '[ $rc = 0 ] && printf "%s" "$out" | grep -q "건너뜀 개인 값 검사" && ! printf "%s" "$out" | grep -q "통과 개인 값 검사"'
# 레인 안(HC_LANE)에서는 끌 수 없다: 무인 레인이 막힘을 우회해 착지하지 않게
out=$(HC_LANE=x HOME=$T/home HERDR_TOOLS_CONFIG=$T/없음 HERDR_VERIFY_NO_LINT=1 bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?
t "레인 안 + HERDR_VERIFY_NO_LINT=1: 무시 줄, 실패 개인 값 검사" '[ $rc != 0 ] && printf "%s" "$out" | grep -q "레인 안이라 HERDR_VERIFY_NO_LINT 무시" && printf "%s" "$out" | grep -q "실패 개인 값 검사" && ! printf "%s" "$out" | grep -q "건너뜀 개인 값 검사"'

# 8. 싼 검사 먼저: 줄 수 한도를 넘긴 파일이 있으면 몇 초 안에 실패하고 무거운 시험(가짜 느린 시험)을 부르지 않는다.
# HERDR_VERIFY_ALL=1이면 무거운 시험까지 다 돈다. 싼 검사는 한 번만 돈다
here=$(cd "$(dirname "$0")" && pwd)
mk cheap "touch '$T/slow-ran'; sleep 30"
mkdir -p "$R/lib/hc-lane"
cp "$here/line-budget.test.sh" "$here/isolate.sh" "$R/tests/"
cp "$here/../lib/hc-lane/test-isolate.sh" "$R/lib/hc-lane/"
: >"$R/tests/line-budget.tsv"
seq 601 >"$R/big"
git -C "$R" add -- big lib tests
git -C "$R" -c user.name=t -c user.email=t@t -c commit.gpgsign=false commit -qm t   # line-budget이 HEAD를 본다
s=$(date +%s); out=$(HC_LANE= bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?; e=$(( $(date +%s) - s ))
t "줄 수 한도 넘김: 실패, 싼 검사 줄, 몇 초 안(${e}초)" '[ $rc = 1 ] && printf "%s" "$out" | grep -q "실패 tests/line-budget.test.sh" && printf "%s" "$out" | grep -q "새 거대 파일 big 601줄" && printf "%s" "$out" | grep -q "실패 싼 검사" && [ $e -lt 20 ]'
t "줄 수 한도 넘김: 무거운 시험은 안 부름" '[ ! -e "$T/slow-ran" ] && ! printf "%s" "$out" | grep -q "tests/x.test.sh"'
printf '#!/bin/bash\ntouch "%s/slow-ran"\n' "$T" >"$R/tests/x.test.sh"
out=$(HC_LANE= HERDR_VERIFY_ALL=1 bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?
t "HERDR_VERIFY_ALL=1: 무거운 시험까지 돌고 실패, 싼 검사는 한 번" '[ $rc = 1 ] && [ -e "$T/slow-ran" ] && printf "%s" "$out" | grep -q "통과 tests/x.test.sh" && [ "$(printf "%s\n" "$out" | grep -c "tests/line-budget.test.sh$")" = 1 ]'
seq 10 >"$R/big"
out=$(HC_LANE= bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?
t "한도 안: 싼 검사 통과 뒤 무거운 시험, line-budget은 한 번" '[ $rc = 0 ] && printf "%s" "$out" | grep -q "통과 tests/x.test.sh" && [ "$(printf "%s\n" "$out" | grep -c "통과 tests/line-budget.test.sh")" = 1 ]'
# 9. --cheap-only(제출 전 사전 검사 .claude/herdr-precheck.sh가 부른다, go-followups): 싼 검사만 돌고 무거운 시험은 안 부름
cp "$src" "$R/.claude/herdr-verify.sh"; cp "${src%/*}/herdr-precheck.sh" "$R/.claude/herdr-precheck.sh"
rm -f "$T/slow-ran"
out=$(HC_LANE= bash "$R/.claude/herdr-precheck.sh" 2>&1); rc=$?
t "사전 검사(--cheap-only): 싼 검사 통과, 무거운 시험 안 부름, 종료 0" '[ $rc = 0 ] && printf "%s" "$out" | grep -q "통과 tests/line-budget.test.sh" && [ ! -e "$T/slow-ran" ] && ! printf "%s" "$out" | grep -q "tests/x.test.sh" || { printf "%s\n" "$out"; false; }'
seq 601 >"$R/big"
out=$(HC_LANE= bash "$R/.claude/herdr-precheck.sh" 2>&1); rc=$?
t "사전 검사(--cheap-only): 줄 수 한도 넘김이면 종료 1" '[ $rc = 1 ] && printf "%s" "$out" | grep -q "새 거대 파일 big 601줄" && [ ! -e "$T/slow-ran" ]'

# 10. 병렬 실행기(2026-11-22 verify-parallel): 동시에 돌고, 결과는 원래(이름) 순서로, 하나가 실패해도 끝까지, 종료 1
mk par 'exit 0'
rm "$R/tests/x.test.sh"
mkdir -p "$T/par"
# a·b·c는 셋 다 시작해야 끝난다(동시가 아니면 5초 뒤 실패). b가 먼저 끝나도 a 다음에 찍힌다
for n in a b c; do
  printf '#!/bin/bash\ntouch "%s/par/%s"; [ %s = a ] && sleep 1\nfor i in $(seq 50); do [ -e "%s/par/a" ] && [ -e "%s/par/b" ] && [ -e "%s/par/c" ] && exit 0; sleep 0.1; done; exit 1\n' \
    "$T" "$n" "$n" "$T" "$T" "$T" >"$R/tests/$n.test.sh"
done
printf '#!/bin/bash\necho "FAIL - 일부러"; echo 끝줄; exit 3\n' >"$R/tests/f.test.sh"   # 이름은 글자 하나 — 로캘 정렬이 점을 무시해도 차례가 같게
printf '#!/bin/bash\ntouch "%s/par/g"\n' "$T" >"$R/tests/g.test.sh"
order() { printf '%s\n' "$out" | grep -E '^(통과|실패) tests/' | tr '\n' ' '; }
out=$(HC_LANE= HERDR_VERIFY_JOBS=3 bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?
t "병렬: 셋이 함께 돌고 원래 순서로, 실패 하나면 종료 1, 그 뒤 시험도 돎" \
  '[ $rc = 1 ] && [ "$(order)" = "통과 tests/a.test.sh 통과 tests/b.test.sh 통과 tests/c.test.sh 실패 tests/f.test.sh 통과 tests/g.test.sh " ] && [ -e "$T/par/g" ] || { printf "%s\n" "$out"; false; }'
t "병렬: 실패 시험의 FAIL 줄과 끝 줄" 'printf "%s" "$out" | grep -q "^FAIL - 일부러" && printf "%s" "$out" | grep -q "^끝줄"'
t "병렬: 끝에 검증 시간 줄(동시 3)" 'printf "%s" "$out" | grep -q "^검증 시간 [0-9]*초 · 동시 3 — 긴 단계: "'
rm -f "$R/tests/f.test.sh"
rm -f "$T/par/a" "$T/par/b" "$T/par/c"
out=$(HC_LANE= HERDR_VERIFY_JOBS=1 HERDR_VERIFY_STEP_SECS=60 bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?
t "HERDR_VERIFY_JOBS=1: 하나씩이라 서로 기다리는 a가 실패" '[ $rc = 1 ] && printf "%s" "$out" | grep -q "^실패 tests/a.test.sh" && printf "%s" "$out" | grep -q "동시 1"'

# 11. 시간 넘김: 상한(HERDR_VERIFY_STEP_SECS)을 넘으면 무리째(배경 자식까지) 끄고 「시간 넘김」, 다른 시험은 그대로 통과
mk slow "sleep 300 </dev/null >/dev/null 2>&1 & echo \$! >'$P/slow'; sleep 300"
printf '#!/bin/bash\nexit 0\n' >"$R/tests/y.test.sh"
s=$(date +%s); out=$(HC_LANE= HERDR_VERIFY_STEP_SECS=2 bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?; e=$(( $(date +%s) - s ))
t "시간 넘김: 실패 줄, 무리째 끔, 몇 초 안(${e}초), 다른 시험 통과" \
  '[ $rc = 1 ] && printf "%s" "$out" | grep -q "^실패 tests/x.test.sh: 시간 넘김(2초)" && gone "$(cat "$P/slow")" && [ $e -lt 15 ] && printf "%s" "$out" | grep -q "^통과 tests/y.test.sh"'

# 12. 단계: 「# herdr-verify: stages」 파일은 --stages의 단계를 따로(동시에) 돌리고 한 줄로 결과. 「!」 단계는 홀로 — 다른 것과 겹치지 않음
mk stgrepo 'touch "'"$T"'/stg-x"; exit 0'
mkdir -p "$T/stg"
cat >"$R/tests/s.test.sh" <<EOF
#!/bin/bash
# herdr-verify: stages
case "\${1-}" in
  --stages) printf '%s\n' '!solo' p1 p2 ;;
  --stage) case "\$2" in
    solo) touch "$T/stg/solo"; sleep 1; ls "$T/stg" >"$T/stg-during"; [ ! -e "$T/stg-x" ] ;;
    p1|p2) touch "$T/stg/\$2"; for i in \$(seq 50); do [ -e "$T/stg/p1" ] && [ -e "$T/stg/p2" ] && break; sleep 0.1; done
      [ -e "$T/stg/p1" ] && [ -e "$T/stg/p2" ] && [ "\$2" != "\${STG_FAIL-}" ] ;;
  esac ;;
  *) exit 9 ;;
esac
EOF
out=$(HC_LANE= HERDR_VERIFY_JOBS=4 bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?
t "단계: 한 파일 한 줄 통과, p1·p2 동시, 홀로 단계는 맨 먼저 혼자" \
  '[ $rc = 0 ] && [ "$(order)" = "통과 tests/s.test.sh 통과 tests/x.test.sh " ] && [ "$(cat "$T/stg-during")" = solo ] || { printf "%s\n" "$out"; false; }'
rm -f "$T/stg/p1" "$T/stg/p2" "$T/stg-x"
out=$(HC_LANE= HERDR_VERIFY_JOBS=4 STG_FAIL=p2 bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?
t "단계: 한 단계 실패면 그 파일 실패, 실패한 단계 이름" \
  '[ $rc = 1 ] && printf "%s" "$out" | grep -q "^실패 tests/s.test.sh" && printf "%s" "$out" | grep -q "^  단계 p2: 실패(종료 1)" && ! printf "%s" "$out" | grep -q "단계 p1"'
printf '#!/bin/bash\n# herdr-verify: stages\nexit 1\n' >"$R/tests/s.test.sh"
out=$(HC_LANE= bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?
t "단계: 목록을 못 읽으면 그 파일 실패" '[ $rc = 1 ] && printf "%s" "$out" | grep -q "^실패 tests/s.test.sh: 단계 목록(--stages)을 못 읽음"'

# 13. 병렬에서 부모가 1이 된 고아(setsid): 동시에 돈 시험 목록과 함께 잡고 끔
mk psid "python3 -c 'import os,sys; os.setsid(); os.execv(sys.argv[1], sys.argv[1:])' \"\$(dirname \"\$0\")/../bin/idle\" </dev/null >/dev/null 2>&1 & echo \$! >'$P/psid'"
printf '#!/bin/bash\nsleep 1\n' >"$R/tests/z.test.sh"
out=$(HC_LANE= HERDR_VERIFY_JOBS=2 bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?
t "병렬 고아: 실패, 동시에 돈 시험 목록, 끔" \
  '[ $rc = 1 ] && printf "%s" "$out" | grep -q "고아 pid $(cat "$P/psid")" && printf "%s" "$out" | grep -q "동시에 돈 시험 가운데: tests/x.test.sh tests/z.test.sh" && gone "$(cat "$P/psid")"'

# 14. 셸 판 계약 시험(tests/*-bash.test.sh)은 HERDR_TEST_BASH=1일 때만 — 아니면 「건너뜀」 줄(통과로 찍지 않음)
mk sh 'exit 0'
printf '#!/bin/bash\ntouch "%s/bash-ran"\n' "$T" >"$R/tests/x-bash.test.sh"
out=$(HC_LANE= HERDR_TEST_BASH= bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?
t "셸 판 시험: 기본은 건너뜀 줄, 안 돎" '[ $rc = 0 ] && printf "%s" "$out" | grep -q "^건너뜀 tests/x-bash.test.sh" && ! printf "%s" "$out" | grep -q "통과 tests/x-bash" && [ ! -e "$T/bash-ran" ]'
out=$(HC_LANE= HERDR_TEST_BASH=1 bash "$R/.claude/herdr-verify.sh" 2>&1); rc=$?
t "셸 판 시험: HERDR_TEST_BASH=1이면 돎" '[ $rc = 0 ] && printf "%s" "$out" | grep -q "^통과 tests/x-bash.test.sh" && [ -e "$T/bash-ran" ]'

# 끝: 이 시험이 띄운 것 0
t "이 시험의 고아 0" allgone

echo "통과 $pass · 실패 $fail"
[ $fail = 0 ]
