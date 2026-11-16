#!/bin/bash
# herdr-sync 시험: 임시 HOME 셋(맥, 워커 맥 w1, 리눅스 lx — lx는 uname이 Linux이고 원격 조각을 dash로 돌린다),
# 가짜 hc(PATH 앞, machine ls·exec [-i] --on만), 임시 저장소 둘(범용 repo·개인 conf)과 그 bare origin. 실제 기계·네트워크는 쓰지 않는다.
# 실행: bash tests/herdr-sync.test.sh   (다른 사본은 HERDR_SYNC=<경로>)
set -u
here=$(cd "$(dirname "$0")" && pwd)
SYNC=${HERDR_SYNC:-$here/../bin/herdr-sync}
[ -f "$SYNC" ] || SYNC=$here/../herdr-sync
MERGE=${HERDR_SETTINGS_MERGE:-$here/../bin/herdr-settings-merge}   # 진짜 herdr-settings-merge를 기계에 깔아 쓴다
[ -f "$MERGE" ] || MERGE=$here/../herdr-settings-merge
[ -f "$MERGE" ] || { echo "FAIL - herdr-settings-merge를 찾지 못했다 (HERDR_SETTINGS_MERGE=<경로>)"; exit 1; }
CFGTOOL=${HERDR_TOOLS_CONFIG_BIN:-$here/../bin/herdr-tools-config}   # 개인 값 검사·prebuild 읽기에 진짜 도우미를 쓴다
[ -f "$CFGTOOL" ] || CFGTOOL=$here/../herdr-tools-config
[ -f "$CFGTOOL" ] || { echo "FAIL - herdr-tools-config를 찾지 못했다 (HERDR_TOOLS_CONFIG_BIN=<경로>)"; exit 1; }
T=$(mktemp -d "${TMPDIR:-/tmp}/herdr-sync-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
trap 'rm -rf "$T"' EXIT
MAC=$T/mac W1=$T/m/w1 LX=$T/m/lx
mkdir -p "$T/bin" "$MAC/Applications/Herdr.app" "$W1" "$LX/.fakebin"
cat > "$T/bin/hc" << 'EOF'
#!/bin/bash
case $1 in
  machine) printf '{"machines":[%s]}\n' "$HC_FAKE_MACHINES" ;;
  exec) shift; in=0
    while [ "$1" != -- ]; do case $1 in -i) in=1 ;; --on) m=$2; shift ;; esac; shift; done; shift
    h=$HCT/m/$m
    { [ -d "$h" ] && [ ! -e "$h/OFFLINE" ]; } || { echo "connect failed: $m" >&2; exit 1; }
    [ $in = 1 ] || exec < /dev/null
    if [ $in = 1 ] && [ "${HC_FAKE_CORRUPT-}" = "$m" ]; then head -c 3 | PATH="$h/.fakebin:$PATH" HOME=$h "$@"; exit; fi
    PATH="$h/.fakebin:$PATH" HOME=$h exec "$@" ;;
  *) echo "fake hc: $*" >&2; exit 2 ;;
esac
EOF
chmod +x "$T/bin/hc"
printf '#!/bin/sh\necho Linux\n' > "$LX/.fakebin/uname" && chmod +x "$LX/.fakebin/uname"
ln -s /bin/dash "$LX/.fakebin/sh"
mkdir -p "$T/tmp"
export TMPDIR=$T/tmp PATH="$T/bin:$PATH" HCT=$T HC_FAKE_MACHINES='{"name":"w1"},{"name":"lx"}' HERDR_TOOLS_REPO=$T/repo HERDR_TOOLS_CONFIG=$T/conf HOME=$MAC
export HERDR_SYNC_SYSAPPS=$T/nosys   # 이 기계의 /Applications/Herdr.app 유무가 시험에 끼지 않게
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t GIT_CONFIG_NOSYSTEM=1

n=0 fails=0
t() { n=$((n + 1)); if eval "$2"; then echo "ok $n - $1"; else fails=$((fails + 1)); echo "FAIL $n - $1"; printf '%s\n' "$out" | sed 's/^/    | /' | tail -30; fi; }
put() { mkdir -p "$(dirname "$1")" && printf '%b' "$2" > "$1"; }
is() { [ "$(cat "$1" 2> /dev/null)" = "$(printf '%b' "$2")" ]; }
has() { printf '%s\n' "$out" | grep -Fxq -- "$1"; }
run() { out=$("$SYNC" "$@" 2>&1); rc=$?; }
snap() { (cd "$T" && find mac m -type f ! -name lock -exec cksum {} + | sort); }
origin() { git -C "$T/origin.git" "$@"; }    # 범용 저장소 origin
corigin() { git -C "$T/conf.git" "$@"; }     # 개인 저장소 origin
heads() { echo "$(origin rev-parse main) $(corigin rev-parse main) $(git -C "$T/repo" rev-parse HEAD) $(git -C "$T/conf" rev-parse HEAD)"; }

# 범용 저장소(repo): 깔 도구 하나(hc-lane), herdr-settings-merge, herdr-tools-config.
# 개인 저장소(conf): sync.list(tools: 줄은 repo), keys.json, config(prebuild), 개인 값 패턴. push는 둘 다 HERDR_LAND=1만 받는다
for r in origin conf; do git init -q --bare -b main "$T/$r.git"; done
git clone -q "$T/origin.git" "$T/repo" 2> /dev/null
git clone -q "$T/conf.git" "$T/conf" 2> /dev/null
cat > "$T/conf/sync.list" << 'EOF'
# 시험용
지침본문 .claude/CLAUDE.md#body          claude/CLAUDE.body.md      all
주제지침 .claude/docs/*.md               claude/docs/               all
Codex공통 .codex/AGENTS.md#codex-shared  codex/AGENTS.shared.md     all
스킬     .agents/skills/herdr-desktop/SKILL.md  skills/herdr-desktop/SKILL.md  macos
스킬     .claude/skills/herdr-desktop@   .agents/skills/herdr-desktop   macos
도구     .local/bin/hc-lane              tools:bin/hc-lane                macos
도구     .local/bin/herdr-settings-merge tools:bin/herdr-settings-merge   all
설정     .local/share/herdr-sync/keys.json settings/keys.json       all
맥도구   .local/bin/herdr-balance        tools:bin/herdr-balance          local
맥도구   .local/share/claude-md/*.py     share/claude-md/           local
지움     .agents/skills/herdr-ovis       -                          macos
지움     .local/bin/claude-md-sync       -                          local
EOF
put "$T/repo/bin/hc-lane" '#!/bin/sh\necho lane\n' && chmod +x "$T/repo/bin/hc-lane"
cp "$MERGE" "$T/repo/bin/herdr-settings-merge" && chmod +x "$T/repo/bin/herdr-settings-merge"
cp "$CFGTOOL" "$T/repo/bin/herdr-tools-config" && chmod +x "$T/repo/bin/herdr-tools-config"
# 가짜 공개 내보내기: 부른 것을 적는다(PUB_FAKE_RC로 실패시킴)
put "$T/repo/bin/herdr-tools-publish" '#!/bin/sh\necho "$HERDR_TOOLS_REPO $HERDR_TOOLS_CONFIG $*" >> "$HCT/published"\nexit ${PUB_FAKE_RC:-0}\n'
put "$T/conf/settings/keys.json" '{"values":[".language",".model",".effortLevel",".nope"],"hooks":{"PreToolUse":["cotenant-guard.sh"]},"refs":[".permissions"],"exceptions":{"lx":[".effortLevel"]}}\n'
put "$T/conf/config" 'sync.prebuild  python3 ~/.local/share/claude-md/build_codex.py\n'
put "$T/conf/private-patterns.txt" 'secret-word\n'
for r in repo conf; do
  (cd "$T/$r" && git add -A && git commit -qm init && git push -q origin main)
  printf '#!/bin/sh\n[ "$HERDR_LAND" = 1 ]\n' > "$T/$r/.git/hooks/pre-push" && chmod +x "$T/$r/.git/hooks/pre-push"
done

# 맥: 지침·주제 지침·Codex·스킬·링크·맥 전용 도구·지울 것·설정
put "$MAC/.claude/CLAUDE.md" 'OMC mac\n<!-- User customizations -->\n본문 1\n'
put "$MAC/.claude/docs/a.md" 'a1\n'
put "$MAC/.claude/docs/b.md" 'b1\n'
put "$MAC/.codex/AGENTS.md" 'mac 전용\n<!-- codex-shared:start -->\ncodex 0\n<!-- codex-shared:end -->\n'
put "$MAC/.agents/skills/herdr-desktop/SKILL.md" 'skill 1\n'
mkdir -p "$MAC/.claude/skills" && ln -s "$MAC/.agents/skills/herdr-desktop" "$MAC/.claude/skills/herdr-desktop"
put "$MAC/.local/bin/herdr-balance" '#!/bin/sh\necho bal\n' && chmod +x "$MAC/.local/bin/herdr-balance"
put "$MAC/.local/bin/claude-md-sync" 'old\n'
put "$MAC/.local/share/claude-md/build_codex.py" 'import os
p = os.path.expanduser("~/.codex/AGENTS.md"); s = open(p).read()
a, rest = s.split("<!-- codex-shared:start -->\\n", 1); _, b = rest.split("<!-- codex-shared:end -->\\n", 1)
open(p, "w").write(a + "<!-- codex-shared:start -->\\ncodex built\\n<!-- codex-shared:end -->\\n" + b)
'
put "$MAC/.claude/settings.json" '{"language":"한국어","model":"opus","effortLevel":"xhigh","permissions":{"allow":["x"]},
"hooks":{"PreToolUse":[{"matcher":"Edit|Write","hooks":[{"type":"command","command":"'"$MAC"'/.claude/hooks/cotenant-guard.sh"}]},
{"matcher":"x","hooks":[{"type":"command","command":"moshi.sh"}]}]}}\n'
# w1: 본문은 같고 OMC 블록만 다름, b.md는 base 없이 다름(첫 실행 충돌), 지울 옛 스킬 있음, Codex 지침 없음
put "$W1/.claude/CLAUDE.md" 'OMC w1\n<!-- User customizations -->\n본문 1\n'
put "$W1/.claude/docs/a.md" 'a1\n'
put "$W1/.claude/docs/b.md" 'b-old\n'
put "$W1/.agents/skills/herdr-ovis/SKILL.md" 'ovis\n'
put "$W1/.claude/settings.json" '{"language":"한국어","model":"opus","effortLevel":"xhigh","theme":"w1"}\n'
# lx: 본문이 다름(첫 실행 충돌), 주제 지침 없음, 표식 없는 Codex 지침, 옛 스킬(리눅스는 mac7을 받지 않는다)
put "$LX/.claude/CLAUDE.md" 'OMC lx\n<!-- User customizations -->\n본문 0\n'
put "$LX/.codex/AGENTS.md" 'lx 머리\n<!-- codebase-memory-mcp:start -->\ncbm\n<!-- codebase-memory-mcp:end -->\n꼬리\n'
put "$LX/.agents/skills/herdr-ovis/SKILL.md" 'ovis\n'
put "$LX/.claude/settings.json" '{"language":"한국어","model":"opus","effortLevel":"medium","permissions":{"allow":["y"]},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"moshi-lx.sh"}]}]}}\n'

# 1. 맥에서만
out=$(HOME=$T/m/w1 "$SYNC" --check 2>&1); rc=$?
t "맥 아님: 종료 2와 「맥에서만」" '[ $rc = 2 ] && printf "%s" "$out" | grep -Fq "맥에서만"'
out=$(HC_FAKE_MACHINES= "$SYNC" --check 2>&1); rc=$?
t "기계를 모르는 hc: 종료 2" '[ $rc = 2 ]'
run --check --take w1:x
t "잘못 부름(--check와 --take): 종료 2" '[ $rc = 2 ]'
run --take w1:.claude/docs/zzz.md
t "sync.list에 없는 경로: 종료 1" '[ $rc = 1 ] && printf "%s" "$out" | grep -Fq "없는 경로"'
mkdir -p "$T/sys/Herdr.app"
out=$(HOME=$T/m/w1 HERDR_SYNC_SYSAPPS=$T/sys HERDR_TOOLS_CONFIG=$T/none "$SYNC" --check 2>&1); rc=$?
t "맥 확인: /Applications/Herdr.app도 받는다 (다음 관문 「개인 저장소」까지 감)" '[ $rc = 2 ] && printf "%s" "$out" | grep -Fq "개인 저장소가 없다"'

# 1-2. 개인 저장소가 없으면 종료 2, 아무 데도 쓰지 않는다(범용 저장소 pull도 하지 않는다)
before=$(snap) h0=$(heads)
for a in --check ""; do
  out=$(HERDR_TOOLS_CONFIG=$T/none "$SYNC" $a 2>&1); rc=$?
  t "개인 저장소 없음 (${a:-맞추기}): 종료 2와 안내" '[ $rc = 2 ] && printf "%s" "$out" | grep -Fq "개인 저장소가 없다: $T/none" && printf "%s" "$out" | grep -Fq config.example'
done
t "개인 저장소 없음: 기계·맥·두 저장소 그대로" '[ "$(snap)" = "$before" ] && [ "$(heads)" = "$h0" ]'

# 1-3. 옛 묶음 이름(all9·mac7·mac-only)은 종료 2 (옛 코드는 기계 이름으로 읽어 「같음」으로 보였다)
for o in all9 mac7 mac-only; do
  rm -rf "$T/conf-old" && cp -R "$T/conf" "$T/conf-old"
  sed -i '' "s/ all\$/ $o/; s/ macos\$/ $o/; s/ local\$/ $o/" "$T/conf-old/sync.list"
  git -C "$T/conf-old" commit -qam old
  out=$(HERDR_TOOLS_CONFIG=$T/conf-old "$SYNC" 2>&1); rc=$?
  t "옛 묶음 이름 $o: 종료 2와 새 이름 안내, 아무 데도 안 씀" '[ $rc = 2 ] && printf "%s" "$out" | grep -Fq "옛 묶음 이름 $o" && [ "$(snap)" = "$before" ] && [ "$(heads)" = "$h0" ]'
done
rm -rf "$T/conf-old"

# 1-4. prebuild는 개인 설정 sync.prebuild를 sh -c로 돌린다. 실패하면 맞추지 않고 멈춘다
rm -rf "$T/conf-bad" && cp -R "$T/conf" "$T/conf-bad"
put "$T/conf-bad/config" 'sync.prebuild  echo built > "$HOME/prebuilt"; exit 3\n'
git -C "$T/conf-bad" commit -qam bad
out=$(HERDR_TOOLS_CONFIG=$T/conf-bad "$SYNC" 2>&1); rc=$?
t "prebuild: sh -c로 돌고, 실패하면 종료 1·저장소에 안 씀" '[ $rc = 1 ] && printf "%s" "$out" | grep -Fq "sync.prebuild 실패" && is "$MAC/prebuilt" "built\n" && [ "$(origin rev-parse main) $(corigin rev-parse main)" = "${h0% * *}" ]'
rm -rf "$T/conf-bad" "$MAC/prebuilt"

# 2. 처음 --check: 아무것도 고치지 않는다
before=$(snap) h0=$(heads)
run --check
t "처음 --check: 종료 1" '[ $rc = 1 ]'
t "처음 --check: 기계·맥·저장소 그대로" '[ "$(snap)" = "$before" ] && [ "$(heads)" = "$h0" ] && [ -z "$(git -C "$T/repo" status --porcelain)$(git -C "$T/conf" status --porcelain)" ]'
t "처음 --check: 저장소로 모을 것" 'has "저장소 다름:모음 .claude/docs/a.md" && has "저장소 다름:깖 .local/bin/hc-lane"'
t "처음 --check: base 없이 다르면 충돌" 'has "충돌 w1 .claude/docs/b.md (2줄 다름)" && has "충돌 lx .claude/CLAUDE.md#body (2줄 다름)"'
t "처음 --check: 표식 없는 Codex 지침도 충돌" 'printf "%s\n" "$out" | grep -q "^충돌 lx .codex/AGENTS.md#codex-shared "'
t "처음 --check: 기계마다 한 줄" 'printf "%s\n" "$out" | grep -q "^w1: .*충돌 1건" && printf "%s\n" "$out" | grep -q "^lx: .*충돌 2건" && printf "%s\n" "$out" | grep -q "^맥: 다름"'

# 3. 처음 맞추기
run
t "처음: 충돌이 남아 종료 1" '[ $rc = 1 ]'
t "모음: 맥 → 개인 저장소 커밋·push (HERDR_LAND=1)" 'corigin log -1 --format=%s main | grep -Fq "herdr-sync 모음" && [ "$(corigin show main:claude/docs/a.md)" = a1 ] && [ "$(corigin show main:claude/CLAUDE.body.md)" = "본문 1" ]'
t "모음: Codex 구역은 prebuild로 다시 만든 값" '[ "$(corigin show main:codex/AGENTS.shared.md)" = "codex built" ]'
t "모음: tools: 줄은 범용 저장소로 따로 커밋, 실행 권한도" 'origin log -1 --format=%s main | grep -Fq "herdr-sync 모음: bin/herdr-balance" && origin ls-tree main bin/herdr-balance | grep -q "^100755"'
t "모음: 두 저장소가 섞이지 않는다" '! origin cat-file -e main:claude/docs/a.md 2> /dev/null && ! corigin cat-file -e main:bin/herdr-balance 2> /dev/null'
t "깖: 저장소 → 맥, 실행 권한 따름" 'is "$MAC/.local/bin/hc-lane" "#!/bin/sh\necho lane\n" && [ -x "$MAC/.local/bin/hc-lane" ]'
t "씀: 워커 맥에 도구·스킬, 실행 권한" '[ -x "$W1/.local/bin/hc-lane" ] && is "$W1/.agents/skills/herdr-desktop/SKILL.md" "skill 1\n" && has "w1 씀 .local/bin/hc-lane"'
t "씀: 링크를 만든다" '[ "$(readlink "$W1/.claude/skills/herdr-desktop")" = "$W1/.agents/skills/herdr-desktop" ]'
t "같음: OMC 블록은 그 기계 것 그대로" 'is "$W1/.claude/CLAUDE.md" "OMC w1\n<!-- User customizations -->\n본문 1\n"'
t "충돌: 쓰지 않는다" 'is "$W1/.claude/docs/b.md" "b-old\n" && is "$LX/.claude/CLAUDE.md" "OMC lx\n<!-- User customizations -->\n본문 0\n"'
t "리눅스: all9만 받는다" 'is "$LX/.claude/docs/a.md" "a1\n" && [ -x "$LX/.local/bin/herdr-settings-merge" ] && [ ! -e "$LX/.local/bin/hc-lane" ] && [ ! -e "$LX/.agents/skills/herdr-desktop" ]'
t "Codex: 표식 없는 지침은 쓰지 않는다(충돌)" 'is "$LX/.codex/AGENTS.md" "lx 머리\n<!-- codebase-memory-mcp:start -->\ncbm\n<!-- codebase-memory-mcp:end -->\n꼬리\n"'
t "Codex: 지침 없는 기계는 건너뛴다" '[ ! -e "$W1/.codex" ]'
t "맥 전용은 기계에 안 간다" '[ ! -e "$W1/.local/bin/herdr-balance" ] && [ ! -e "$W1/.local/share/claude-md" ]'
t "지움: 워커 맥은 백업으로 옮긴다, 리눅스는 mac7이라 둔다" '[ ! -e "$W1/.agents/skills/herdr-ovis" ] && is "$(echo "$W1"/.local/state/herdr-sync/bak/*/.agents/skills/herdr-ovis/SKILL.md)" "ovis\n" && [ -e "$LX/.agents/skills/herdr-ovis" ]'
t "지움: 맥도" '[ ! -e "$MAC/.local/bin/claude-md-sync" ] && [ -n "$(echo "$MAC"/.local/state/herdr-sync/bak/*/.local/bin/claude-md-sync)" ] && has "맥 지움 .local/bin/claude-md-sync"'
t "설정: 예외 키는 그 기계 값, 정한 키 밖은 그대로" '[ "$(jq -r .effortLevel "$LX/.claude/settings.json")" = medium ] && has "lx 예외 .effortLevel" && [ "$(jq -r .theme "$W1/.claude/settings.json")" = w1 ]'
t "설정: 정한 훅만 더하고 맥 \$HOME은 대상 \$HOME으로, 다른 훅은 그대로" '[ "$(jq -c "[.hooks.PreToolUse[].hooks[].command]" "$LX/.claude/settings.json")" = "[\"moshi-lx.sh\",\"$LX/.claude/hooks/cotenant-guard.sh\"]" ] && has "lx 씀 hook:PreToolUse:cotenant-guard.sh"'
t "설정: 참고 차이는 보인다" 'has "lx 참고 .permissions"'
t "base: 기계마다 남는다" 'grep -q "^.claude/CLAUDE.md#body	" "$W1/.local/state/herdr-sync/base.tsv" && grep -q "^.claude/docs/a.md	" "$LX/.local/state/herdr-sync/base.tsv"'

# 4. 다시 돌려도 충돌은 그대로, --take로 맥 값
run
t "다시: 충돌은 쓰지 않는다" '[ $rc = 1 ] && is "$W1/.claude/docs/b.md" "b-old\n"'
run --take w1:.claude/docs/b.md
t "--take: 맥 값으로 덮고 백업" 'is "$W1/.claude/docs/b.md" "b1\n" && grep -rqx "b-old" "$W1/.local/state/herdr-sync/bak" && printf "%s\n" "$out" | grep -qx "w1: 같음"'

run --take 'lx:.codex/AGENTS.md#codex-shared'
t "--take: 표식 없는 Codex 지침에 구역을 넣고 cbm 블록은 남긴다(백업)" 'is "$LX/.codex/AGENTS.md" "<!-- codebase-memory-mcp:start -->\ncbm\n<!-- codebase-memory-mcp:end -->\n\n<!-- codex-shared:start -->\ncodex built\n<!-- codex-shared:end -->\n" && grep -rqx "꼬리" "$LX/.local/state/herdr-sync/bak"'

# 5. --pull: 그 기계 값을 맥으로 가져와 저장소·다른 기계까지
run --pull lx:.claude/CLAUDE.md#body
t "--pull: 종료 0" '[ $rc = 0 ]'
t "--pull: 맥 본문이 lx 값, 맥 OMC는 그대로" 'is "$MAC/.claude/CLAUDE.md" "OMC mac\n<!-- User customizations -->\n본문 0\n"'
t "--pull: 저장소와 w1까지" '[ "$(corigin show main:claude/CLAUDE.body.md)" = "본문 0" ] && is "$W1/.claude/CLAUDE.md" "OMC w1\n<!-- User customizations -->\n본문 0\n"'
run --check
t "다 맞은 뒤 --check: 종료 0, 모두 같음" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -qx "w1: 같음" && printf "%s\n" "$out" | grep -qx "lx: 같음" && printf "%s\n" "$out" | grep -qx "맥: 같음"'

# 6. base가 있는 뒤 그 기계에서 따로 고치면 충돌, --take <기계>:*
put "$W1/.claude/docs/a.md" 'w1이 고침\n'
run
t "따로 고친 기계: 충돌" '[ $rc = 1 ] && has "충돌 w1 .claude/docs/a.md (2줄 다름)" && is "$W1/.claude/docs/a.md" "w1이 고침\n"'
run --take 'w1:*'
t "--take w1:*: 그 기계 충돌 전부" '[ $rc = 0 ] && is "$W1/.claude/docs/a.md" "a1\n"'

# 6-2. --pull <기계>:*는 그 기계의 대상만: 맥 전용 도구의 옛 사본은 가져오지 않는다
put "$W1/.local/bin/herdr-balance" 'stale\n'
run --pull 'w1:*'
t "--pull 기계:*: 대상 밖 파일은 가져오지 않는다" 'is "$MAC/.local/bin/herdr-balance" "#!/bin/sh\necho bal\n" && ! printf "%s" "$out" | grep -Fq "가져옴 .local/bin/herdr-balance"'
run --pull 'w1:.codex/AGENTS.md#codex-shared'
t "--pull Codex 구역: 거절" '[ $rc = 1 ] && printf "%s" "$out" | grep -Fq "Codex 구역"'

# 7. 맥 ↔ 저장소: 저장소만 바뀜(레인 착지) → 맥에 깔고 기계까지
git clone -q "$T/conf.git" "$T/lane" 2> /dev/null
(cd "$T/lane" && printf 'a2\n' > claude/docs/a.md && git commit -qam lane && git push -q origin main)
run
t "저장소만 바뀜: 맥에 깖(백업)·기계에 씀" '[ $rc = 0 ] && is "$MAC/.claude/docs/a.md" "a2\n" && grep -rqx a1 "$MAC/.local/state/herdr-sync/bak" && is "$W1/.claude/docs/a.md" "a2\n" && is "$LX/.claude/docs/a.md" "a2\n"'
# 둘 다 바뀜 → 멈춘다
put "$MAC/.claude/docs/a.md" 'a3-mac\n'
(cd "$T/lane" && git pull -q && printf 'a3-repo\n' > claude/docs/a.md && git commit -qam lane2 && git push -q origin main)
head1=$(corigin rev-parse main)
run
t "둘 다 바뀜: 충돌로 멈추고 아무 데도 안 쓴다" '[ $rc = 1 ] && has "충돌 저장소 .claude/docs/a.md (2줄 다름)" && [ "$(corigin rev-parse main)" = "$head1" ] && is "$MAC/.claude/docs/a.md" "a3-mac\n" && is "$W1/.claude/docs/a.md" "a2\n"'
run --take repo:.claude/docs/a.md
t "--take repo: 맥 값을 저장소와 기계에" '[ $rc = 0 ] && [ "$(corigin show main:claude/docs/a.md)" = a3-mac ] && is "$W1/.claude/docs/a.md" "a3-mac\n"'

# 8. 실행 권한은 맥 값을 따른다
chmod a-x "$MAC/.local/bin/hc-lane"
run
t "실행 권한: 맥에서 뺀 x가 저장소·기계로" '[ $rc = 0 ] && origin ls-tree main bin/hc-lane | grep -q "^100644" && [ -f "$W1/.local/bin/hc-lane" ] && [ ! -x "$W1/.local/bin/hc-lane" ]'

# 7-2. 범용 저장소만 바뀜(범용 레인 착지) → tools: 줄을 맥에 깖
git clone -q "$T/origin.git" "$T/tlane" 2> /dev/null
(cd "$T/tlane" && printf '#!/bin/sh\necho lane2\n' > bin/hc-lane && git commit -qam tlane && git push -q origin main)
run
t "범용 저장소만 바뀜: 맥에 깖·기계에 씀" '[ $rc = 0 ] && has "맥 깖 .local/bin/hc-lane" && is "$MAC/.local/bin/hc-lane" "#!/bin/sh\necho lane2\n" && is "$W1/.local/bin/hc-lane" "#!/bin/sh\necho lane2\n"'

# 8-2. push가 거절되면 멈추고, 다음 실행에서 그 커밋을 다시 보낸다
printf '#!/bin/sh\n[ "$HERDR_LAND" = 1 ] && [ ! -e "%s/nopush" ]\n' "$T" > "$T/conf/.git/hooks/pre-push"
put "$MAC/.claude/docs/c.md" 'c1\n'
touch "$T/nopush"
run
t "push 거절: 종료 1, origin에 없음" '[ $rc = 1 ] && ! corigin cat-file -e main:claude/docs/c.md 2> /dev/null'
rm -f "$T/nopush"
run
t "push 거절 뒤 다시: 그 커밋을 보낸다" '[ $rc = 0 ] && [ "$(corigin show main:claude/docs/c.md)" = c1 ]'

# 9. 반쯤 쓴 파일이 없음: 전송이 깨지면 쓰지 않고 임시 파일도 남지 않는다
put "$MAC/.claude/docs/a.md" 'a4 긴 내용\n'
out=$(HC_FAKE_CORRUPT=w1 "$SYNC" 2>&1); rc=$?
t "깨진 전송: 쓰지 않음, 다름으로 보임 (설정 입력도 깨져 도구가 거절)" '[ $rc = 1 ] && is "$W1/.claude/docs/a.md" "a3-mac\n" && printf "%s\n" "$out" | grep -qx "w1: 다름 (주제지침·도구문제:설정)"'
t "깨진 전송: 임시 파일 없음" '[ -z "$(find "$T/mac" "$T/m" "$T/repo" "$T/conf" -name "*.hsync.*")" ]'
run
t "깨진 뒤 다시: 맞춘다" '[ $rc = 0 ] && is "$W1/.claude/docs/a.md" "a4 긴 내용\n"'

# 9-2. --pull repo: 저장소 값을 남긴다
put "$MAC/.claude/docs/a.md" 'a5-mac\n'
(cd "$T/lane" && git pull -q && printf 'a5-repo\n' > claude/docs/a.md && git commit -qam lane3 && git push -q origin main)
run --pull repo:.claude/docs/a.md
t "--pull repo: 저장소 값을 맥과 기계에" '[ $rc = 0 ] && is "$MAC/.claude/docs/a.md" "a5-repo\n" && is "$W1/.claude/docs/a.md" "a5-repo\n"'
# 저장소에서 지운 파일은 맥에서 지우고(백업) 기계에는 둔다
(cd "$T/lane" && git pull -q && git rm -q claude/docs/b.md && git commit -qm rm && git push -q origin main)
run
t "저장소에서 지움: 맥에서 지우고 기계는 둔다" '[ $rc = 0 ] && [ ! -e "$MAC/.claude/docs/b.md" ] && grep -rqx b1 "$MAC/.local/state/herdr-sync/bak" && [ -e "$W1/.claude/docs/b.md" ] && has "맥 깖 .claude/docs/b.md"'
# 링크 자리에 폴더가 있으면 충돌, --take로 폴더를 백업 폴더로 옮기고 링크
rm "$W1/.claude/skills/herdr-desktop" && mkdir "$W1/.claude/skills/herdr-desktop" && touch "$W1/.claude/skills/herdr-desktop/mine"
run
t "링크 자리에 폴더: 충돌, 건드리지 않음" '[ $rc = 1 ] && [ -e "$W1/.claude/skills/herdr-desktop/mine" ] && [ ! -L "$W1/.claude/skills/herdr-desktop" ]'
run --take 'w1:.claude/skills/herdr-desktop@'
t "링크 --take: 폴더는 백업, 링크를 만든다" '[ $rc = 0 ] && [ -L "$W1/.claude/skills/herdr-desktop" ] && [ -n "$(find "$W1/.local/state/herdr-sync/bak" -name mine)" ]'

# 10. 연결 실패
touch "$W1/OFFLINE"
run --check
t "연결 실패: 그 기계는 다름, 종료 1" '[ $rc = 1 ] && printf "%s\n" "$out" | grep -qx "w1: 다름 (연결)" && printf "%s\n" "$out" | grep -qx "lx: 같음"'
rm -f "$W1/OFFLINE"

# 11. 설정: 그 기계에서 따로 고친 키는 충돌, --take <기계>:<키>, 도구 문제는 충돌과 구분
jq '.model = "sonnet"' "$LX/.claude/settings.json" > "$T/s" && mv "$T/s" "$LX/.claude/settings.json"
run
t "설정 충돌: 충돌로 세고 쓰지 않는다" '[ $rc = 1 ] && has "충돌 lx .model" && printf "%s\n" "$out" | grep -qx "lx: 충돌 1건" && [ "$(jq -r .model "$LX/.claude/settings.json")" = sonnet ]'
run --take lx:.model
t "설정 --take: 맥 값으로" '[ $rc = 0 ] && has "lx 씀 .model" && [ "$(jq -r .model "$LX/.claude/settings.json")" = opus ]'
cp "$LX/.claude/settings.json" "$T/s.keep" && echo '{깨짐' > "$LX/.claude/settings.json"
run --check
t "설정 도구 문제: 충돌이 아니라 도구문제로" '[ $rc = 1 ] && printf "%s\n" "$out" | grep -qx "lx: 다름 (도구문제:설정)" && ! has "충돌 lx .model"'
mv "$T/s.keep" "$LX/.claude/settings.json"
run --check
t "설정 --check: 다 맞으면 같음" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -qx "lx: 같음"'

# 12. 고른 기계만
put "$MAC/.claude/docs/b.md" 'b2\n'
run lx
t "고른 기계만: lx만 맞추고 w1은 그대로" '[ $rc = 0 ] && is "$LX/.claude/docs/b.md" "b2\n" && is "$W1/.claude/docs/b.md" "b1\n" && ! printf "%s\n" "$out" | grep -q "^w1:"'

# 12-2. 어느 저장소든 커밋하지 않은 변경이 있으면 아무것도 하지 않고 멈춘다
put "$MAC/.claude/docs/b.md" 'b3\n'
before=$(snap)
for f in repo/bin/hc-lane conf/sync.list; do
  printf 'dirty\n' >> "$T/$f"
  run
  t "더러운 저장소 ($f): 멈추고 아무 데도 안 쓴다" '[ $rc = 1 ] && printf "%s" "$out" | grep -Fq "커밋하지 않은 변경" && [ "$(snap)" = "$before" ]'
  git -C "$T/${f%%/*}" checkout -q -- "${f#*/}"
done

# 12-3. 범용 저장소로 모을 맥 값에 개인 값이 있으면 충돌, 아무 데도 안 쓴다 (개인 저장소로 가는 파일은 괜찮다)
run
cp -p "$MAC/.local/bin/herdr-balance" "$T/bal.keep"
put "$MAC/.local/bin/herdr-balance" '#!/bin/sh\necho secret-word\n' && chmod +x "$MAC/.local/bin/herdr-balance"
put "$MAC/.claude/docs/d.md" 'secret-word 는 개인 저장소엔 괜찮다\n'
before=$(snap) h0=$(heads)
run --check
t "개인 값 --check: 충돌로 보인다" '[ $rc = 1 ] && has "충돌 저장소 .local/bin/herdr-balance (개인 값)" && has "저장소 다름:모음 .claude/docs/d.md"'
run
t "개인 값: 종료 1, 두 저장소·맥·기계 그대로" '[ $rc = 1 ] && has "충돌 저장소 .local/bin/herdr-balance (개인 값)" && [ "$(heads)" = "$h0" ] && [ "$(snap)" = "$before" ]'
run --take repo:.local/bin/herdr-balance
t "개인 값: --take repo로도 범용 저장소에 넣지 않는다" '[ $rc = 1 ] && [ "$(heads)" = "$h0" ]'
# 패턴 파일이 없으면(또는 패턴 줄이 없으면) 검사를 못 하니 범용으로 모으지 않는다(fail-closed)
put "$MAC/.local/bin/herdr-balance" '#!/bin/sh\necho bal2\n' && chmod +x "$MAC/.local/bin/herdr-balance"
for pat in '# 주석만\n' -; do
  rm -rf "$T/conf-np" && cp -R "$T/conf" "$T/conf-np"
  if [ "$pat" = - ]; then git -C "$T/conf-np" rm -q private-patterns.txt; else put "$T/conf-np/private-patterns.txt" "$pat"; git -C "$T/conf-np" add private-patterns.txt; fi
  git -C "$T/conf-np" commit -qm np
  out=$(HERDR_TOOLS_CONFIG=$T/conf-np "$SYNC" 2>&1); rc=$?
  t "패턴 없음(${pat%%\\*}): 범용 모음은 충돌, 범용 origin 그대로" '[ $rc = 1 ] && has "충돌 저장소 .local/bin/herdr-balance (개인 값 검사 못 함 — $T/conf-np/private-patterns.txt에 패턴이 없다)" && [ "$(origin rev-parse main)" = "${h0%% *}" ]'
done
rm -rf "$T/conf-np"
cp -p "$T/bal.keep" "$MAC/.local/bin/herdr-balance"
run
t "개인 값을 뺀 뒤: 개인 저장소 파일은 모은다" '[ $rc = 0 ] && corigin show main:claude/docs/d.md | grep -Fq secret-word && ! origin log -p main | grep -Fq secret-word'

# 13. 잠금: 한 번에 하나
lock=$MAC/.local/state/herdr-sync/lock
lockf -s -t 0 "$lock" sh -c 'touch "$1"; while [ ! -e "$2" ]; do sleep 0.1; done' _ "$T/held" "$T/release" &
pid=$!
for _ in $(seq 50); do [ -e "$T/held" ] && break; sleep 0.1; done
run --check
touch "$T/release"; wait "$pid"
t "잠금: 다른 herdr-sync가 돌면 바로 끝" '[ $rc = 75 ] && printf "%s" "$out" | grep -Fq "다른 herdr-sync"'

# 14. 범용 저장소의 config.example/sync.list: 꼴이 맞고, tools: 경로가 모두 있고, bin/*을 모두 깐다
LIST=$here/../config.example/sync.list
if [ -f "$LIST" ]; then
  t "config.example/sync.list: 줄마다 네 칸, 대상이 새 이름" '! grep -v "^#" "$LIST" | awk "NF && (NF != 4 || \$4 !~ /^(all|macos|local|[a-z0-9,-]+)\$/ || \$4 ~ /(^|,)(all9|mac7|mac-only)(,|\$)/)" | grep -q .'
  for p in $(awk '!/^#/ && $3 ~ /^tools:/ {print substr($3, 7)}' "$LIST"); do
    t "config.example/sync.list: tools:$p 있음" '[ -e "$here/../$p" ]'
  done
  for f in "$here"/../bin/*; do b=${f##*/}; t "config.example/sync.list: bin/$b" 'grep -q " tools:bin/$b " "$LIST"'; done
fi

# 15. 공개 내보내기: 설정 publish.repo가 있을 때만, --check는 안 부름, 실패해도 크게 경고만 하고 종료 코드는 그대로
t "publish.repo 없음: 내보내기를 부르지 않는다" '[ ! -e "$T/published" ]'
printf 'publish.repo  /x/public.git\n' >> "$T/conf/config"
git -C "$T/conf" commit -qam pub && HERDR_LAND=1 git -C "$T/conf" push -q origin main
run --check
t "--check: 내보내기를 부르지 않는다" '[ ! -e "$T/published" ]'
run
t "publish.repo 있음: 맞춘 뒤 내보내기를 부른다" '[ $rc = 0 ] && [ "$(cat "$T/published")" = "$T/repo $T/conf " ]'
export PUB_FAKE_RC=1
run
unset PUB_FAKE_RC
t "내보내기 실패: 크게 경고, 맞추기 종료 코드는 그대로" '[ $rc = 0 ] && has "!!! herdr-sync: 공개 저장소 내보내기 실패 (맞추기는 끝났다) — herdr-tools-publish 출력을 보고 고친 뒤 herdr-tools-publish를 다시 !!!"'

t "임시 파일이 남지 않는다 (맥·기계 쪽 조각 모두)" '[ -z "$(ls -A "$T/tmp")" ]'

echo "$((n - fails))/$n 통과"
[ $fails = 0 ]
