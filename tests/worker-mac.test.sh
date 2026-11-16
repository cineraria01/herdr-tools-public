#!/bin/bash
# worker-mac-check·worker-mac-setup 시험: 가짜 ssh·hc(PATH 앞)·임시 HOME 둘(맥·원격)·config.example 사본으로.
# 원격 쪽 시스템 명령(launchctl·pmset·sudo 등)은 가짜 원격 HOME의 ~/.local/bin에 둔 가짜가 받는다. 실제 기계·네트워크는 쓰지 않는다.
# 실행: bash tests/worker-mac.test.sh   (다른 사본은 WORKER_MAC_BIN=<bin 폴더>, 예시 설정은 WORKER_MAC_EXAMPLE=<config.example 폴더>)
set -u
here=$(cd "$(dirname "$0")" && pwd)
BIN=${WORKER_MAC_BIN:-$here/../bin}
EX=${WORKER_MAC_EXAMPLE:-$here/../config.example}
[ -f "$EX/config" ] || { echo "FAIL - 예시 설정이 없다: $EX/config"; exit 1; }
T=$(mktemp -d "${TMPDIR:-/tmp}/worker-mac-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
trap 'rm -rf "$T"' EXIT
export T_LOG=$T/log RHOME=$T/rh HOME=$T/mac GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t GIT_CONFIG_NOSYSTEM=1
mkdir -p "$T/bin" "$T_LOG" "$RHOME/.local/bin" "$HOME/.local/bin" "$RHOME/Library/Application Support/Google/Chrome"
ln -s "$BIN/herdr-tools-config" "$T/bin/herdr-tools-config"
export PATH="$T/bin:$PATH"

# 가짜 ssh: 옵션을 건너뛰고 기계 이름을 ssh.log에, 나머지 명령은 원격 HOME에서 sh -c로 돈다. SSH_FAKE_FAIL이면 연결 안 됨.
cat >"$T/bin/ssh" <<'EOF'
#!/bin/bash
while [ $# -gt 0 ]; do case $1 in -o) shift 2 ;; -n|-t) shift ;; *) break ;; esac; done
echo "$1" >>"$T_LOG/ssh.log"; shift
[ -z "${SSH_FAKE_FAIL-}" ] || exit 255
cd "$RHOME" && HOME=$RHOME exec /bin/sh -c "$*"
EOF
# 가짜 hc(맥 쪽): machine ls는 w1 연결됨, exec는 -- 뒤 명령을 원격 HOME에서. 그 밖은 hc.log에.
cat >"$T/bin/hc" <<'EOF'
#!/bin/bash
case $1 in
  machine) [ "$2" = ls ] && { echo '{"name":"w1","state":"connected"}'; exit 0; }; echo "$*" >>"$T_LOG/hc.log" ;;
  exec) while [ "$1" != -- ]; do shift; done; shift; cd "$RHOME" && HOME=$RHOME exec "$@" ;;
  *) echo "$*" >>"$T_LOG/hc.log" ;;
esac
EOF
chmod +x "$T/bin/ssh" "$T/bin/hc"
# 원격 가짜 시스템 명령. Dock pid 123, 코어 pid 77, 둘 다 asid 5(코어 GUI 문맥).
w() { printf '#!/bin/bash\n%s\n' "$2" >"$1"; chmod +x "$1"; }
R=$RHOME/.local/bin
w "$R/pgrep" 'case $1 in -x) echo 123 ;; *) echo 77 ;; esac'
w "$R/launchctl" 'case "$1 $2" in "print pid/"*) echo "asid = 5"; echo "name = dev.herdr.core.t" ;; "print gui/"*) echo "0 - com.t.herdr-core" ;; *) echo "$*" >>"$T_LOG/launchctl.log" ;; esac'
w "$R/defaults" 'id -un'
w "$R/pmset" 'echo " SleepDisabled 1"'
w "$R/fdesetup" 'echo "FileVault is Off."'
w "$R/sudo" 'echo "sudo: a password is required"; exit 1'
w "$R/sqlite3" 'exit 1'
w "$R/xcode-select" 'echo /nonexistent'
w "$R/pkgutil" 'exit 1'
w "$R/security" 'exit 0'
w "$R/gh" 'echo "Logged in to github.com account tester (keyring)"'
w "$R/herdr-core" 'echo "{\"live\":0}"'
w "$R/hc" 'echo "$*" >>"$T_LOG/remote-hc.log"'
for d in "$R" "$HOME/.local/bin"; do
  w "$d/git" '[ "${1-}" = --version ] && { echo "git version 9.9.9"; exit 0; }; exec /usr/bin/git "$@"'
  w "$d/node" 'echo v99.0.0'
  w "$d/fooz" 'echo "fooz 1.2.3"; echo "둘째 줄"'
done
w "$R/example-proxy" '[ "$1" = status ] && { echo "proxy running"; echo "usable now: 2"; }'
w "$R/other-proxy" '[ "$1" = status ] && echo "usable now: 0"'

n=0 fails=0
t() { n=$((n + 1)); if eval "$2"; then echo "ok $n - $1"; else fails=$((fails + 1)); echo "FAIL $n - $1"; echo "  rc=$rc"; sed 's/^/  | /' "$T/out"; fi; }
has() { grep -Fq -- "$1" "$T/out"; }
# 설정 폴더: config.example 사본. cfg_del <키>: 그 줄 지움. cfg_set <키> <값>: 바꿈(없으면 더함)
newcfg() { rm -rf "$T/conf"; cp -R "$EX" "$T/conf"; rm -f "$T/conf/worker-checklist.txt"; export HERDR_TOOLS_CONFIG=$T/conf; }
cfg_del() { grep -v -E "^$1([[:space:]]|\$)" "$T/conf/config" >"$T/conf/c" && mv "$T/conf/c" "$T/conf/config"; }
cfg_set() { cfg_del "$1"; printf '%s %s\n' "$1" "$2" >>"$T/conf/config"; }
newhome() { rm -rf "$RHOME/src" "$RHOME/work" "$RHOME/Documents" "$RHOME/.config" "$RHOME/.claude.json"* "$RHOME/.local/state"; : >"$T_LOG/ssh.log"; }
check() { bash "$BIN/worker-mac-check" "$@" >"$T/out" 2>&1; rc=$?; }
setup() { bash "$BIN/worker-mac-setup" "$@" >"$T/out" 2>&1; rc=$?; }
git_repo() { mkdir -p "$RHOME/$1" && /usr/bin/git -C "$RHOME/$1" init -q -b main && /usr/bin/git -C "$RHOME/$1" commit -q --allow-empty -m t \
  && /usr/bin/git -C "$RHOME/$1" remote add origin https://example.invalid/p.git; }
trust_json() { jq -n --args '{hasCompletedOnboarding:true, projects:(reduce $ARGS.positional[] as $p ({}; .[$p]={hasTrustDialogAccepted:true}))}' "$@" >"$RHOME/.claude.json"; }

# ── worker-mac-check ──
newcfg; newhome; git_repo src/example-app; trust_json "$RHOME/src/example-app"
mkdir -p "$RHOME/.config/example-app" "$RHOME/Documents"; echo 2 >"$RHOME/.config/example-app/verify-slots"
check w1
t "check: worker.repo 첫 경로의 저장소" '[ $rc = 0 ] && has "통과	저장소	$RHOME/src/example-app (main, https://example.invalid/p.git)"'
t "check: 신뢰는 찾은 저장소와 그 부모로" 'has "통과	Claude 첫 실행·신뢰	안내 끝, 신뢰 $RHOME/src=false example-app=true"'
t "check: worker.slots_file 파일" 'has "통과	verify-slots	2"'
t "check: worker.tools 도구만 판 비교" 'has "통과	판 git	9.9.9 (맥과 같음)" && has "통과	판 node	v99.0.0 (맥과 같음)" && ! has "판 bun" && ! has "판 go"'
t "check: worker.proxy 상태 줄" 'has "통과	example-proxy status	proxy running usable now: 2"'

newhome; git_repo work/example-app; echo "{}" >"$RHOME/.claude.json"
check w1
t "check: worker.repo는 처음 있는 것" 'has "통과	저장소	$RHOME/work/example-app"'
t "check: 신뢰 없으면 실패" '[ $rc = 1 ] && has "실패	Claude 첫 실행·신뢰	안내 끝=false, 신뢰 $RHOME/work=false example-app=false"'
t "check: slots 파일이 없으면 실패" 'has "실패	verify-slots	파일 없음"'
trust_json "$RHOME/work"
check w1
t "check: 둘째 후보(work/…) 저장소는 그 부모 신뢰로 통과" 'has "통과	Claude 첫 실행·신뢰	안내 끝, 신뢰 $RHOME/work=true example-app=false"'
trust_json "$RHOME/src" "$RHOME/src/example-app"
check w1
t "check: 찾은 저장소와 상관없는 worker.trust 경로 신뢰로는 통과 안 함" 'has "실패	Claude 첫 실행·신뢰	안내 끝=true, 신뢰 $RHOME/work=false example-app=false"'
newhome; git_repo src/example-app; trust_json "$RHOME/src/example-web"
check w1
t "check: 형제 폴더 신뢰로는 통과 안 함" 'has "실패	Claude 첫 실행·신뢰	안내 끝=true, 신뢰 $RHOME/src=false example-app=false"'

newhome; trust_json "$RHOME/src"
check w1
t "check: worker.repo 경로가 다 없으면 첫 경로로 실패" 'has "실패	저장소	~/src/example-app 없음"'
t "check: 저장소가 없으면 첫 후보와 그 부모의 신뢰" 'has "통과	Claude 첫 실행·신뢰	안내 끝, 신뢰 $RHOME/src=true example-app=false"'
newcfg; cfg_set worker.repo "x*"; newhome; git_repo xa
check w1
t "check: 목록의 *는 원격에서 펼치지 않음" 'has "실패	저장소	~/x* 없음"'

newcfg; cfg_set worker.tools fooz; cfg_set worker.proxy other-proxy; newhome
check w1
t "check: worker.tools를 바꾸면 그 도구(--version 첫 줄)" 'has "통과	판 fooz	fooz 1.2.3 (맥과 같음)" && ! has "판 git"'
t "check: worker.proxy를 바꾸면 그 명령의 status" 'has "실패	other-proxy status	usable now: 0"'

newcfg; for k in worker.repo worker.trust worker.slots_file worker.tools worker.proxy; do cfg_del $k; done; newhome
mkdir -p "$RHOME/Documents/app"; trust_json "$RHOME/Documents/app"
check w1
t "check: worker.repo 없음 → 모름 설정 없음" 'has "모름	저장소	설정 없음(worker.repo)"'
t "check: worker.trust 없음 → Documents/app" 'has "통과	Claude 첫 실행·신뢰	안내 끝, 신뢰 Documents/app=true"'
t "check: worker.slots_file 없음 → 모름 설정 없음" 'has "모름	검증 자리 파일	설정 없음(worker.slots_file)"'
t "check: worker.tools 없음 → git node" 'has "판 git	" && has "판 node	" && [ "$(grep -c "	판 " "$T/out")" = 2 ]'
t "check: worker.proxy 없음 → 모름 설정 없음" 'has "모름	프록시 status	설정 없음(worker.proxy)"'
cfg_set worker.trust "src src/example-app"; trust_json "$RHOME/src"
check w1
t "check: worker.repo 없음 → worker.trust 경로가 모두 신뢰여야 통과" 'has "실패	Claude 첫 실행·신뢰	안내 끝=true, 신뢰 src=true src/example-app=false"'
rm -f "$T/conf/config"
check w1
t "check: 설정 파일이 없으면 건너뛰는 항목마다 모름" 'has "설정 없음(worker.repo)" && has "설정 없음(worker.slots_file)" && has "설정 없음(worker.proxy)" && grep -Eq "^-- w1: .*모름 [1-9]" "$T/out"'
newcfg; cfg_del worker.proxy; printf 'worker.proxy\nworker.repo\nworker.slots_file\n' >>"$T/conf/config"; cfg_del worker.repo; cfg_del worker.slots_file; printf 'worker.repo\nworker.slots_file\n' >>"$T/conf/config"
check w1
t "check: 빈 값(키 혼자)은 일부러 끈 것 → 줄 없음" '! has "설정 없음" && ! has "저장소" && ! has "verify-slots" && ! has " status	"'

newcfg; printf 'worker.tools git\n' >>"$T/conf/config"; newhome
check w1
t "check: 같은 키 두 번이면 종료 2, 접속 안 함" '[ $rc = 2 ] && [ ! -s "$T_LOG/ssh.log" ]'

# ── worker-mac-setup ──
newcfg; newhome
setup w1 --slots 3
t "setup: 성공" '[ $rc = 0 ]'
t "setup: --slots는 worker.slots_file에" '[ "$(cat "$RHOME/.config/example-app/verify-slots" 2>/dev/null)" = 3 ]'
t "setup: worker.trust 경로를 신뢰로" '[ "$(jq -c "[.hasCompletedOnboarding, (.projects|keys)]" "$RHOME/.claude.json")" = "[true,[\"$RHOME/src\",\"$RHOME/src/example-app\"]]" ]'
t "setup: worker.trust 첫 경로 폴더를 만듦" '[ -d "$RHOME/src" ]'
t "setup: 체크리스트 파일 없음 → 8번이 Xcode" 'grep -Eq "^ 8\. \[그 맥\] \(Xcode" "$T/out" && grep -Eq "^ 9\. \[이 맥\] 점검: worker-mac-check w1" "$T/out"'
t "setup: 체크리스트 파일 없음을 알림" 'has "설정 없음(worker-checklist.txt)"'

newcfg; newhome
printf '# 주석\n[이 맥] 예시 로그인 {M}:\n    ! ssh -t {M} 예시\n\n[그 맥] 둘째 단계\n' >"$T/conf/worker-checklist.txt"
setup w1
t "setup: worker-checklist.txt를 8번부터 붙임, {M}은 기계 이름" 'grep -Fxq " 8. [이 맥] 예시 로그인 w1:" "$T/out" && grep -Fxq "    ! ssh -t w1 예시" "$T/out" && grep -Fxq " 9. [그 맥] 둘째 단계" "$T/out"'
t "setup: 설정이 다 있으면 설정 없음 줄 없음" '! has "설정 없음"'
t "setup: 붙인 뒤 번호가 이어짐" 'grep -Eq "^10\. \[그 맥\] \(Xcode" "$T/out" && grep -Eq "^11\. \[이 맥\] 점검" "$T/out" && ! grep -Fq "# 주석" "$T/out"'
t "setup: --slots 없으면 1" '[ "$(cat "$RHOME/.config/example-app/verify-slots" 2>/dev/null)" = 1 ]'

newcfg; cfg_del worker.slots_file; cfg_del worker.trust; newhome
setup w1 --slots 3
t "setup: worker.slots_file 없음 + --slots → 종료 64, 접속 안 함" '[ $rc = 64 ] && [ ! -s "$T_LOG/ssh.log" ]'
setup w1
t "setup: worker.slots_file 없음 → slots 단계 없음" '[ $rc = 0 ] && ! has "verify-slots" && [ ! -d "$RHOME/.config" ]'
t "setup: worker.slots_file 없음을 알림" 'has "설정 없음(worker.slots_file)"'
t "setup: worker.trust 없음 → Documents/app" '[ "$(jq -c "(.projects|keys)" "$RHOME/.claude.json")" = "[\"$RHOME/Documents/app\"]" ]'

newcfg; cfg_set worker.trust "it's src"; cfg_set worker.slots_file ".config/it's/slots"; newhome
setup w1
t "setup: 경로에 따옴표가 있어도 폴더·자리 파일·뒤 단계" '[ $rc = 0 ] && [ -d "$RHOME/it'"'"'s" ] && [ "$(cat "$RHOME/.config/it'"'"'s/slots" 2>/dev/null)" = 1 ] && [ -x "$RHOME/.local/state/worker-mac-setup/root-setup.sh" ]'

newcfg; newhome
SSH_FAKE_FAIL=1 setup w1
t "setup: SSH 안내는 worker.ssh_key" '[ $rc = 2 ] && has "IdentityFile ~/.ssh/id_ed25519_workers" && has "ssh-copy-id -i ~/.ssh/id_ed25519_workers.pub w1"'
cfg_del worker.ssh_key
SSH_FAKE_FAIL=1 setup w1
t "setup: worker.ssh_key 없음 → ~/.ssh/id_ed25519" 'has "IdentityFile ~/.ssh/id_ed25519)" && has "ssh-copy-id -i ~/.ssh/id_ed25519.pub w1"'

newcfg; newhome
setup w1 --dry-run --slots 2
t "setup --dry-run: 원격에 쓰지 않음" '[ $rc = 0 ] && [ ! -e "$RHOME/.claude.json" ] && [ ! -d "$RHOME/src" ] && has "(dry-run) ~/.claude.json: hasCompletedOnboarding=true, 신뢰 src src/example-app"'

echo "# ${n}개 중 실패 ${fails}"
[ $fails = 0 ]
