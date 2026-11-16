#!/bin/bash
# herdr-tools-config 시험: 임시 HOME·가짜 설정 폴더로 get·file·lint를 돌린다. 실제 ~/.config·네트워크는 쓰지 않는다.
# 실행: bash tests/herdr-tools-config.test.sh   (다른 사본은 HERDR_TOOLS_CONFIG_BIN=<경로>)
set -u
here=$(cd "$(dirname "$0")" && pwd)
TOOL=${HERDR_TOOLS_CONFIG_BIN:-$here/../bin/herdr-tools-config}
EX=$here/../config.example
T=$(mktemp -d "${TMPDIR:-/tmp}/htc-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
trap 'rm -rf "$T"' EXIT
export HOME="$T/home" GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t GIT_CONFIG_NOSYSTEM=1
unset HERDR_TOOLS_CONFIG
mkdir -p "$HOME" "$T/c1" "$T/c2" "$T/공백 폴더"

n=0 fails=0
t() { n=$((n + 1)); if eval "$2"; then echo "ok $n - $1"; else fails=$((fails + 1)); echo "FAIL $n - $1"; echo "  rc=$rc out=[$out] err=[$(cat "$T/err")]"; fi; }
# run <설정 폴더> <인자…>: $out·$rc. 설정 폴더가 -이면 HERDR_TOOLS_CONFIG를 두지 않는다
run() { local c=$1; shift
  if [ "$c" = - ]; then out=$(bash "$TOOL" "$@" 2>"$T/err"); else out=$(HERDR_TOOLS_CONFIG=$c bash "$TOOL" "$@" 2>"$T/err"); fi; rc=$?; }
err1() { [ "$(wc -l <"$T/err" | tr -d ' ')" = 1 ]; }

cat >"$T/c1/config" <<'EOF'
# 주석 줄
  # 들여쓴 주석
sync.prebuild   make -C ~/src  prebuild
balance.remote	w1/ w2/
worker.proxy
worker.ssh_key ~/.ssh/id_test

Bad.Key 무시된다
EOF
printf 'balance.remote x/\n' >"$T/c2/config"

# get
run "$T/c1" get sync.prebuild
t "get: 값은 줄 끝까지, 앞뒤 빈칸만 뺌" '[ $rc = 0 ] && [ "$out" = "make -C ~/src  prebuild" ]'
run "$T/c1" get balance.remote
t "get: 탭으로 나눠도 됨" '[ $rc = 0 ] && [ "$out" = "w1/ w2/" ]'
run "$T/c1" get worker.proxy
t "get: 키 혼자 있으면 빈 값, 종료 0" '[ $rc = 0 ] && [ -z "$out" ]'
run "$T/c1" get worker.proxy fallback
t "get: 빈 값도 값이라 기본값을 쓰지 않음" '[ $rc = 0 ] && [ -z "$out" ]'
run "$T/c1" get worker.tools "git node"
t "get: 없는 키 + 기본값 → 기본값" '[ $rc = 0 ] && [ "$out" = "git node" ]'
run "$T/c1" get worker.tools ""
t "get: 없는 키 + 빈 기본값 → 빈 값, 종료 0" '[ $rc = 0 ] && [ -z "$out" ]'
run "$T/c1" get worker.tools
t "get: 없는 키, 기본값 없음 → 종료 3 + 한 줄 안내" '[ $rc = 3 ] && [ -z "$out" ] && err1 && grep -Fq "설정 없음 worker.tools ($T/c1/config — 예: config.example/config)" "$T/err"'
run "$T/c2" get balance.remote
t "get: HERDR_TOOLS_CONFIG를 바꾸면 그 폴더를 읽음" '[ $rc = 0 ] && [ "$out" = "x/" ]'
mkdir -p "$HOME/.config/herdr-tools"; printf 'worker.repo src/p\n' >"$HOME/.config/herdr-tools/config"
run - get worker.repo
t "get: 기본 폴더는 ~/.config/herdr-tools" '[ $rc = 0 ] && [ "$out" = "src/p" ]'
run "$T/없음" get worker.repo
t "get: 설정 파일 없음 → 종료 3" '[ $rc = 3 ] && err1 && grep -Fq "설정 없음 worker.repo ($T/없음/config" "$T/err"'
run "$T/없음" get worker.repo dflt
t "get: 설정 파일 없음 + 기본값 → 기본값" '[ $rc = 0 ] && [ "$out" = dflt ]'
printf 'worker.repo a\nworker.tools git\nworker.repo b\n' >"$T/c2/config"
run "$T/c2" get worker.tools
t "get: 같은 키 두 번은 다른 키를 물어도 종료 2" '[ $rc = 2 ] && err1 && grep -Fq "worker.repo" "$T/err"'
printf 'worker.proxy\r\nworker.repo a b\r\n\r\n# c\r\n' >"$T/c2/config"
run "$T/c2" get worker.proxy fallback
t "get: CRLF config의 키 혼자 줄도 빈 값" '[ $rc = 0 ] && [ -z "$out" ]'
run "$T/c2" get worker.repo
t "get: CRLF config의 값 끝 CR은 뺌" '[ $rc = 0 ] && [ "$out" = "a b" ]'
run "$T/c1" get Bad.Key
t "get: 키 이름 꼴이 틀리면 종료 2" '[ $rc = 2 ] && err1'
run "$T/c1" get
t "get: 키 없이 부르면 종료 2" '[ $rc = 2 ]'
run "$T/c1" get a.b c d
t "get: 인자가 넘치면 종료 2" '[ $rc = 2 ]'

# file
printf 'x\n' >"$T/공백 폴더/worker-checklist.txt"; mkdir -p "$T/공백 폴더/settings"; printf '{}\n' >"$T/공백 폴더/settings/keys.json"
run "$T/공백 폴더" file worker-checklist.txt
t "file: 있으면 절대 경로, 종료 0" '[ $rc = 0 ] && [ "$out" = "$T/공백 폴더/worker-checklist.txt" ]'
run "$T/공백 폴더" file settings/keys.json
t "file: 하위 폴더 이름도 됨" '[ $rc = 0 ] && [ "$out" = "$T/공백 폴더/settings/keys.json" ]'
(cd "$T" && HERDR_TOOLS_CONFIG=c1 bash "$TOOL" file config >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "file: 상대 경로 설정 폴더도 절대 경로로" '[ $rc = 0 ] && [ "$out" = "$T/c1/config" ]'
run "$T/공백 폴더" file sync.list
t "file: 없으면 종료 3 + 한 줄 안내" '[ $rc = 3 ] && [ -z "$out" ] && err1 && grep -Fq "sync.list" "$T/err"'
run "$T/공백 폴더" file ../c1/config
t "file: ..로 폴더 밖은 종료 2" '[ $rc = 2 ] && [ -z "$out" ]'
run "$T/공백 폴더" file /etc/hosts
t "file: 절대 경로는 종료 2" '[ $rc = 2 ] && [ -z "$out" ]'
run "$T/공백 폴더" file settings
t "file: 폴더는 파일이 아니라 종료 3" '[ $rc = 3 ]'

# lint
printf '# 주석\n\nsecret-host-[0-9]+\n/Users/bob/\n' >"$T/c1/private-patterns.txt"
R="$T/repo"; mkdir -p "$R/sub"
printf 'ok\nssh secret-host-7 here\n' >"$R/a.txt"
printf 'cd /Users/bob/x\nsecret-host-1\n' >"$R/sub/b c.sh"
printf 'clean\n' >"$R/clean.txt"
printf 'secret-host-[0-9]+\nsecret-host-9\n' >"$R/sub/private-patterns.txt"
printf 'secret-host-3\n' >"$R/untracked.txt"
(cd "$R" && git init -q && git add a.txt "sub/b c.sh" clean.txt sub/private-patterns.txt && git commit -qm i)
(cd "$R" && HERDR_TOOLS_CONFIG=$T/c1 bash "$TOOL" lint >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "lint: 걸리면 종료 1, 파일:줄: 패턴" '[ $rc = 1 ] && printf "%s\n" "$out" | grep -Fxq "a.txt:2: secret-host-[0-9]+" && printf "%s\n" "$out" | grep -Fxq "sub/b c.sh:1: /Users/bob/" && printf "%s\n" "$out" | grep -Fxq "sub/b c.sh:2: secret-host-[0-9]+"'
t "lint: 인자 없으면 git ls-files만(추적 안 하는 파일은 뺌)" '! printf "%s\n" "$out" | grep -q "untracked" && [ "$(printf "%s\n" "$out" | wc -l | tr -d " ")" = 4 ]'
t "lint: 쓰는 패턴 파일이 아닌 다른 private-patterns.txt는 검사함" 'printf "%s\n" "$out" | grep -Fxq "sub/private-patterns.txt:2: secret-host-[0-9]+"'
(cd "$R" && HERDR_TOOLS_CONFIG=$T/c1 bash "$TOOL" lint clean.txt sub/private-patterns.txt >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "lint: 파일을 주면 그 파일만(패턴 파일도 준 대로 검사)" '[ $rc = 1 ] && [ "$out" = "sub/private-patterns.txt:2: secret-host-[0-9]+" ]'
(cd "$R" && HERDR_TOOLS_CONFIG=$T/c1 bash "$TOOL" lint clean.txt >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "lint: 안 걸리면 종료 0, 출력 없음" '[ $rc = 0 ] && [ -z "$out" ]'
(cd "$R" && HERDR_TOOLS_CONFIG=$T/c2 bash "$TOOL" lint >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "lint: 패턴 파일이 둘 다 없으면 종료 0 + 표준 오류 한 줄" '[ $rc = 0 ] && [ -z "$out" ] && err1'
mkdir -p "$HOME/.local/share/herdr-tools"; printf 'clean\n' >"$HOME/.local/share/herdr-tools/private-patterns.txt"
(cd "$R" && HERDR_TOOLS_CONFIG=$T/c2 bash "$TOOL" lint >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "lint: 설정 폴더에 없으면 ~/.local/share/herdr-tools 패턴" '[ $rc = 1 ] && [ "$out" = "$(printf "clean.txt:0: 경로 clean\nclean.txt:1: clean")" ]'
printf 'ok(\n' >"$T/c2/private-patterns.txt"
(cd "$R" && HERDR_TOOLS_CONFIG=$T/c2 bash "$TOOL" lint a.txt >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "lint: 틀린 패턴은 종료 2" '[ $rc = 2 ] && grep -Fq "ok(" "$T/err"'
(cd "$T" && HERDR_TOOLS_CONFIG=$T/c1 bash "$TOOL" lint >"$T/out" 2>"$T/err"); rc=$?
t "lint: 인자 없고 git 저장소 밖이면 종료 2" '[ $rc = 2 ]'
# 패턴 폴더가 저장소 안(상대 경로로 줌): 쓰는 그 패턴 파일만 뺀다
R2="$T/repo2"; mkdir -p "$R2/conf" "$R2/ex"
printf '# 예: secret-host-5\nsecret-host-[0-9]+\n' >"$R2/conf/private-patterns.txt"
printf '# 예: secret-host-6\n' >"$R2/ex/private-patterns.txt"
printf 'secret-host-2\n' >"$R2/x:y.txt"
printf 'secret-host-4\n' >"$R2/target.txt"
ln -s /Users/bob/private "$R2/lnk"; ln -s target.txt "$R2/lnk2"
(cd "$R2" && git init -q && git add conf ex "x:y.txt" target.txt lnk lnk2 && git commit -qm i)
(cd "$R2" && HERDR_TOOLS_CONFIG=conf bash "$TOOL" lint >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "lint: 저장소 안 패턴 파일은 그 하나만 빠지고 다른 패턴 파일은 걸림" '[ $rc = 1 ] && ! printf "%s\n" "$out" | grep -q "^conf/" && printf "%s\n" "$out" | grep -Fxq "ex/private-patterns.txt:1: secret-host-[0-9]+"'
t "lint: 파일 이름에 :가 있어도 파일:줄: 패턴" 'printf "%s\n" "$out" | grep -Fxq "x:y.txt:1: secret-host-[0-9]+"'
t "lint: 심볼릭 링크는 대상 내용을 훑지 않음" '! printf "%s\n" "$out" | grep -q "^lnk2:" && printf "%s\n" "$out" | grep -Fxq "target.txt:1: secret-host-[0-9]+"'
printf '/Users/bob/\n' >"$T/c2/private-patterns.txt"
(cd "$R2" && HERDR_TOOLS_CONFIG=$T/c2 bash "$TOOL" lint >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "lint: 심볼릭 링크는 대상 경로 글자를 검사" '[ $rc = 1 ] && [ "$out" = "lnk:1: /Users/bob/" ]'
ln -s /Users/bob/leak "$R2/-lnk3"; (cd "$R2" && git add -- -lnk3 && git commit -qm l)
(cd "$R2" && HERDR_TOOLS_CONFIG=$T/c2 bash "$TOOL" lint >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "lint: 이름이 -로 시작하는 링크도 검사" '[ $rc = 1 ] && printf "%s\n" "$out" | grep -Fxq -- "-lnk3:1: /Users/bob/" && [ ! -s "$T/err" ]'
mkdir -p "$T/c3"; ln -s "$R2/conf/private-patterns.txt" "$T/c3/private-patterns.txt"
(cd "$R2" && HERDR_TOOLS_CONFIG=$T/c3 bash "$TOOL" lint >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "lint: 설정 폴더의 패턴 파일이 저장소 파일로 가는 링크여도 그 파일을 뺌" '[ $rc = 1 ] && ! printf "%s\n" "$out" | grep -q "^conf/" && printf "%s\n" "$out" | grep -Fxq "ex/private-patterns.txt:1: secret-host-[0-9]+"'
printf 'clean\r\n' >"$T/c2/private-patterns.txt"
(cd "$R" && HERDR_TOOLS_CONFIG=$T/c2 bash "$TOOL" lint >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "lint: CRLF 패턴 파일도 줄 끝 CR을 빼고 잡음" '[ $rc = 1 ] && [ "$out" = "$(printf "clean.txt:0: 경로 clean\nclean.txt:1: clean")" ]'

# UTF-8 아닌 바이트·NUL이 든 파일도 검사한다(macOS grep -I는 통째로 건너뛰었다 — 2026-11-16 publish-public 감사)
printf 'caf\351 secret-host-1\n' >"$R/latin1.txt"
printf 'a\000b\nsecret-host-2\n' >"$R/blob.bin"
(cd "$R" && HERDR_TOOLS_CONFIG=$T/c1 bash "$TOOL" lint latin1.txt blob.bin >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "lint: UTF-8 아닌 바이트·NUL 파일도 잡음" '[ $rc = 1 ] && printf "%s\n" "$out" | grep -Fxq "latin1.txt:1: secret-host-[0-9]+" && printf "%s\n" "$out" | grep -Fxq "blob.bin:2: secret-host-[0-9]+"'
printf 'clean\n' >"$R/secret-host-8.md"
(cd "$R" && HERDR_TOOLS_CONFIG=$T/c1 bash "$TOOL" lint secret-host-8.md "$R/clean.txt" >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "lint: 상대 경로로 준 파일은 경로 글자도 검사(절대 경로는 내용만)" '[ $rc = 1 ] && [ "$out" = "secret-host-8.md:0: 경로 secret-host-[0-9]+" ]'
(cd "$R" && HERDR_TOOLS_CONFIG=$T/c1 bash "$TOOL" lint clean.txt no-such.txt >"$T/out" 2>"$T/err"); rc=$?
t "lint: 못 읽는 파일이 있으면 검사 못 함, 종료 2" '[ $rc = 2 ] && grep -Fq "검사 못 함" "$T/err"'
printf 'SECRET-HOST-3 up\n' >"$R/upper.txt"
(cd "$R" && HERDR_TOOLS_CONFIG=$T/c1 bash "$TOOL" lint upper.txt >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "lint: 대소문자를 무시한다" '[ $rc = 1 ] && [ "$out" = "upper.txt:1: secret-host-[0-9]+" ]'
rm -f "$R/latin1.txt" "$R/blob.bin" "$R/secret-host-8.md" "$R/upper.txt"

# 부름
run "$T/c1"
t "인자 없으면 종료 2" '[ $rc = 2 ]'
run "$T/c1" nope
t "모르는 명령은 종료 2" '[ $rc = 2 ]'

# config.example: 계약 §3의 키가 모두 있고, 가짜 패턴에 걸리지 않음
for k in sync.prebuild balance.remote worker.repo worker.trust worker.slots_file worker.tools worker.ssh_key worker.proxy; do
  run "$EX" get "$k"
  t "config.example: $k 있음" '[ $rc = 0 ] && [ -n "$out" ]'
done
(cd "$EX/.." && HERDR_TOOLS_CONFIG=$EX bash "$TOOL" lint config.example/config >"$T/out" 2>"$T/err"); rc=$?; out=$(cat "$T/out")
t "config.example: 예시 config는 예시 패턴에 안 걸림" '[ $rc = 0 ] && [ -z "$out" ]'

echo "# ${n}개 중 실패 ${fails}"
[ "$fails" = 0 ]
