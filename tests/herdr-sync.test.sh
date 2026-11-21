#!/bin/bash
# herdr-sync 시험: 임시 HOME 셋(맥, 워커 맥 w1, 리눅스 lx — lx는 uname이 Linux이고 원격 조각을 dash로 돌린다),
# 가짜 hc(PATH 앞, machine ls·exec [-i] --on만), 임시 저장소 둘(범용 repo·개인 conf)과 그 bare origin. 실제 기계·네트워크는 쓰지 않는다.
# 실행: bash tests/herdr-sync.test.sh   (다른 사본은 HERDR_SYNC=<경로>)
set -u
here=$(cd "$(dirname "$0")" && pwd)
SYNC=${HERDR_SYNC:-$here/../bin/herdr-sync}
# 판(go·bash): herdr-sync는 기본 Go 판으로 못박는다(tests/ops-go-bash.test.sh가 HERDR_IMPL=bash로 같은 것을). 기계에 까는 도구 사본은
# 곁에 공통 실행기가 없어 셸 판으로 돈다(Go 소스를 안 깐 기계와 같다)
export HERDR_SYNC_IMPL=${HERDR_SYNC_IMPL:-${HERDR_IMPL:-go}}
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
    echo "$m" >> "$HCT/calls"   # 기계마다 hc 호출 수를 센다
    { [ -d "$h" ] && [ ! -e "$h/OFFLINE" ]; } || { echo "connect failed: $m" >&2; exit 1; }
    # NOSH: PATH에 sh가 없는 기계(Windows 등) — 진짜 hc의 오류 글 그대로
    [ ! -e "$h/NOSH" ] || { echo '{"error":{"hint":"","kind":"spawn_failed","message":"cannot start sh: program not found"}}' >&2; exit 1; }
    [ $in = 1 ] || exec < /dev/null
    if [ $in = 1 ]; then   # 진짜 hc처럼 입력은 8MB까지만 받는다
      cat > "$HCT/in.$$"
      [ "$(wc -c < "$HCT/in.$$")" -le 8388608 ] || { rm -f "$HCT/in.$$"; echo '{"error":{"kind":"invalid_params","message":"at most 8 MB piped in"}}' >&2; exit 6; }
      exec < "$HCT/in.$$" && rm -f "$HCT/in.$$"
    fi
    if [ $in = 1 ] && [ "${HC_FAKE_CORRUPT-}" = "$m" ]; then head -c 3 | PATH="$h/.fakebin:$PATH" HOME=$h "$@"; exit; fi
    # 출력도 8MB에서 자른다
    PATH="$h/.fakebin:$PATH" HOME=$h "$@" | head -c 8388608; exit "${PIPESTATUS[0]}" ;;
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
t "처음 --check: 기계마다 한 줄" 'printf "%s\n" "$out" | grep -q "^w1: .*충돌 1건" && printf "%s\n" "$out" | grep -q "^lx: .*충돌 2건" && printf "%s\n" "$out" | grep -q "^맥: 저장소가 새로움 3개 (bin/hc-lane bin/herdr-settings-merge settings/keys.json) — herdr-sync로 깐다 · 저장소보다 새로움 7개 (claude/CLAUDE.body.md claude/docs/a.md claude/docs/b.md …) — 커밋 안 됨? · 다름 (지움)$"'

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
t "다 맞은 뒤 --check: 종료 0, 모두 같음" '[ $rc = 0 ] && printf "%s\n" "$out" | grep -qx "w1: 같음" && printf "%s\n" "$out" | grep -qx "lx: 같음" && printf "%s\n" "$out" | grep -qx "맥: 저장소와 같음"'

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
# 착지 직후 --check: 맥이 저장소보다 뒤인 것을 보인다. 기계 줄 「같음」은 맥 옛 판과 같다는 뜻이다 (2026-11-19 「이미 퍼졌다」 오판)
run --check
t "맥이 저장소보다 뒤 --check: 맥 줄에 보이고 종료 1" '[ $rc = 1 ] && has "맥: 저장소가 새로움 1개 (bin/hc-lane) — herdr-sync로 깐다" && is "$MAC/.local/bin/hc-lane" "#!/bin/sh\necho lane\n"'
t "맥이 저장소보다 뒤 --check: 기계 줄 같음은 맥 판 기준" 'has "w1: 같음 (맥 판 기준 — 저장소 새 판은 아직)" && has "lx: 같음 (맥 판 기준 — 저장소 새 판은 아직)" && [ "$(printf "%s\n" "$out" | grep -n "^맥: " | cut -d: -f1)" -lt "$(printf "%s\n" "$out" | grep -n "^w1: " | cut -d: -f1)" ]'
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
t "저장소에서 지움: 맥에서 지우고(맥 지움 줄) 기계는 둔다" '[ $rc = 0 ] && [ ! -e "$MAC/.claude/docs/b.md" ] && grep -rqx b1 "$MAC/.local/state/herdr-sync/bak" && [ -e "$W1/.claude/docs/b.md" ] && has "맥 지움 .claude/docs/b.md (저장소에서 지워짐)"'
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

# 10-2. sh를 못 띄우는 기계(Windows 등)는 건너뛰고 종료 코드에 넣지 않는다.
# uname이 Darwin·Linux가 아닌 기계(Git sh의 MINGW64_NT 등)에는 --take·--pull로 집어도 아무것도 쓰지 않는다
NS=$T/m/ns WG=$T/m/wg
mkdir -p "$NS" "$WG/.fakebin" && touch "$NS/NOSH"
printf '#!/bin/sh\necho MINGW64_NT-10.0\n' > "$WG/.fakebin/uname" && chmod +x "$WG/.fakebin/uname"
put "$WG/.claude/docs/a.md" 'wg-own\n'
wgsnap() { (cd "$WG" && find . \( -type f -o -type l \) -exec cksum {} + 2> /dev/null | sort); }
wg0=$(wgsnap) mac0=$(cat "$MAC/.claude/docs/a.md")
put "$MAC/.claude/docs/a.md" 'a6\n'
export HC_FAKE_MACHINES='{"name":"w1"},{"name":"lx"},{"name":"ns"},{"name":"wg"}'
run --check
t "--check: sh 없는 기계·다른 OS는 건너뜀 한 줄씩" 'has "ns: 건너뜀 (sh 없음 — Windows 등)" && has "wg: 건너뜀 (OS MINGW64_NT-10.0)" && ! printf "%s" "$out" | grep -q "spawn_failed"'
run
t "sh 없는 기계: 건너뜀 한 줄, 종료 0, 다른 기계는 맞춘다" '[ $rc = 0 ] && has "ns: 건너뜀 (sh 없음 — Windows 등)" && ! printf "%s" "$out" | grep -q "ns 연결 실패" && is "$W1/.claude/docs/a.md" "a6\n" && is "$LX/.claude/docs/a.md" "a6\n"'
t "uname MINGW64_NT: 건너뜀 한 줄, 아무 파일도 안 씀" 'has "wg: 건너뜀 (OS MINGW64_NT-10.0)" && [ "$(wgsnap)" = "$wg0" ]'
run --check
t "--check: 다른 기계가 같으면 건너뛴 기계가 있어도 종료 0" '[ $rc = 0 ] && has "ns: 건너뜀 (sh 없음 — Windows 등)" && has "wg: 건너뜀 (OS MINGW64_NT-10.0)" && printf "%s\n" "$out" | grep -qx "w1: 같음"'
run --take 'wg:*'
t "다른 OS: --take로 집어도 쓰지 않는다" '[ $rc = 0 ] && [ "$(wgsnap)" = "$wg0" ]'
run --pull wg:.claude/docs/a.md
t "다른 OS: --pull로 집어도 가져오지 않는다" '[ $rc = 0 ] && is "$MAC/.claude/docs/a.md" "a6\n" && [ "$(wgsnap)" = "$wg0" ]'
run --pull ns:.claude/docs/a.md
t "sh 없는 기계: --pull은 건너뛰고 종료 0" '[ $rc = 0 ] && is "$MAC/.claude/docs/a.md" "a6\n"'
export HC_FAKE_MACHINES='{"name":"w1"},{"name":"lx"}'
rm -rf "$NS" "$WG"
put "$MAC/.claude/docs/a.md" "$mac0\n"
run
t "10-2 뒤 되돌림: 다시 같음" '[ $rc = 0 ] && is "$W1/.claude/docs/a.md" "$mac0\n"'

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
t "맥이 저장소보다 새로움 --check: 맥 줄에 보이고, 기계 줄에 맥 판 표시 없음" 'has "맥: 저장소보다 새로움 1개 (claude/docs/d.md) — 커밋 안 됨? · 충돌 1건" && printf "%s\n" "$out" | grep -q "^w1: " && ! printf "%s" "$out" | grep -Fq "맥 판 기준"'
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

# 15. 나무 줄: <폴더>/** = 그 아래 모든 층의 파일. 링크·숨김·__pycache__·node_modules·제외·50MB 넘음은 걷지 않는다
git -C "$T/lane" pull -q
cat >> "$T/lane/sync.list" << 'EOF'
스킬나무 .agents/skills/**             skills/agents/          macos
스킬나무 .claude/skills/**             skills/claude/          all
제외     .agents/skills/app-managed    -                       -
링크     .claude/skills-alias/beta@    .claude/skills/beta     all
링크     .claude/skills-alias/gone@    .agents/skills/gone     all
EOF
git -C "$T/lane" commit -qam tree && git -C "$T/lane" push -q origin main
git -C "$T/conf" pull -q   # herdr-sync가 처음에 하는 pull을 미리 (아래 heads 비교가 그것을 쓰기로 보지 않게)
A=$MAC/.agents/skills C=$MAC/.claude/skills
put "$A/alpha/SKILL.md" 'alpha 1\n'
put "$A/alpha/scripts/run.sh" '#!/bin/sh\necho run\n' && chmod +x "$A/alpha/scripts/run.sh"
put "$A/alpha/ref/deep/er/notes.md" 'deep 1\n'
put "$A/alpha/한 글.md" 'space 1\n'
put "$A/alpha/.hidden" 'h\n' && put "$A/alpha/.git/config" 'g\n' && put "$A/alpha/__pycache__/x.pyc" 'p\n' && put "$A/alpha/node_modules/m/index.js" 'n\n'
ln -s SKILL.md "$A/alpha/link.md" && put "$MAC/elsewhere/in.md" 'e\n' && ln -s "$MAC/elsewhere" "$A/linked"
put "$A/app-managed/SKILL.md" 'app mac\n'
put "$A/gamma/SKILL.md" 'gamma 1\n'
put "$A/big/ok.md" 'ok\n' && dd if=/dev/zero of="$A/big/huge.bin" bs=1 count=0 seek=7340033 2> /dev/null
put "$C/beta/SKILL.md" 'beta 1\n' && put "$C/beta/sub/x.md" 'x\n'
# w1: 손댄 파일(base 없음, 내용 다름), 내용은 같고 실행 권한만 다른 파일, 그 기계에만 있는 파일, 제외 경로, 링크로 된 폴더
W1A=$W1/.agents/skills
put "$W1A/alpha/SKILL.md" 'alpha w1\n'
put "$W1A/alpha/scripts/run.sh" '#!/bin/sh\necho run\n'
put "$W1A/w1only/SKILL.md" 'mine\n'
put "$W1A/app-managed/SKILL.md" 'app w1\n'
mkdir -p "$W1/elsewhere" && ln -s "$W1/elsewhere" "$W1A/gamma"
before=$(snap) h0=$(heads)
run --check
t "나무 --check: 아무 데도 안 쓴다" '[ $rc = 1 ] && [ "$(snap)" = "$before" ] && [ "$(heads)" = "$h0" ] && has "저장소 다름:모음 .agents/skills/alpha/ref/deep/er/notes.md"'
run
skills() { git -C "$T/conf.git" -c core.quotePath=false ls-tree -r --name-only main skills/ | LC_ALL=C sort | tr '\n' ' '; }
t "나무 모음: 하위 폴더 모든 층, 걷지 않는 것은 빼고, 따로 적은 줄과 겹치면 그 줄이 맡는다" '[ "$(skills)" = "skills/agents/alpha/SKILL.md skills/agents/alpha/ref/deep/er/notes.md skills/agents/alpha/scripts/run.sh skills/agents/alpha/한 글.md skills/agents/big/ok.md skills/agents/gamma/SKILL.md skills/claude/beta/SKILL.md skills/claude/beta/sub/x.md skills/herdr-desktop/SKILL.md " ]'
t "나무 모음: 실행 권한" 'corigin ls-tree main skills/agents/alpha/scripts/run.sh | grep -q "^100755"'
t "7MB 넘음: 건너뜀 한 줄, 아무 데도 안 감" 'has "건너뜀 .agents/skills/big/huge.bin (7MB 넘음)" && [ ! -e "$W1A/big/huge.bin" ] && is "$W1A/big/ok.md" "ok\n"'
t "나무 씀: 새 파일, 여러 층, 빈칸·한글 이름" 'is "$W1A/alpha/ref/deep/er/notes.md" "deep 1\n" && is "$W1A/alpha/한 글.md" "space 1\n" && has "w1 씀 .agents/skills/alpha/한 글.md"'
t "나무 씀: 실행 권한만 다르면 충돌이 아니라 씀" '[ -x "$W1A/alpha/scripts/run.sh" ] && ! has "충돌 w1 .agents/skills/alpha/scripts/run.sh (0줄 다름)" && has "w1 씀 .agents/skills/alpha/scripts/run.sh"'
t "걷지 않음: 숨김·.git·__pycache__·node_modules·링크" '[ ! -e "$W1A/alpha/.hidden" ] && [ ! -e "$W1A/alpha/.git" ] && [ ! -e "$W1A/alpha/__pycache__" ] && [ ! -e "$W1A/alpha/node_modules" ] && [ ! -e "$W1A/alpha/link.md" ] && [ ! -e "$W1A/linked" ]'
t "제외 줄: 그 아래는 안 걷고 기계에 있는 것도 그대로" 'is "$W1A/app-managed/SKILL.md" "app w1\n" && ! corigin cat-file -e main:skills/agents/app-managed/SKILL.md 2> /dev/null'
t "그 기계에만 있는 파일: 그대로, 표에도 안 셈" 'is "$W1A/w1only/SKILL.md" "mine\n" && ! printf "%s" "$out" | grep -Fq w1only'
t "그 기계에서 손댄 파일: 충돌, 덮지 않음" 'has "충돌 w1 .agents/skills/alpha/SKILL.md (2줄 다름)" && is "$W1A/alpha/SKILL.md" "alpha w1\n" && printf "%s\n" "$out" | grep -qx "w1: 충돌 1건"'
t "그 기계에서 링크인 폴더: 따라가지 않고 건너뜀" 'has "건너뜀 w1 .agents/skills/gamma (링크 — 따라가지 않음)" && [ ! -e "$W1/elsewhere/SKILL.md" ]'
t "리눅스: all 나무만, dash로 씀" 'is "$LX/.claude/skills/beta/sub/x.md" "x\n" && [ ! -e "$LX/.agents/skills/alpha" ] && printf "%s\n" "$out" | grep -qx "lx: 같음"'
t "링크: 가리킬 곳을 이번에 썼으면 같은 실행에서 만든다" '[ "$(readlink "$LX/.claude/skills-alias/beta")" = "$LX/.claude/skills/beta" ] && [ "$(readlink "$W1/.claude/skills-alias/beta")" = "$W1/.claude/skills/beta" ]'
t "링크: 가리킬 곳이 없으면 만들지 않고 건너뜀" 'has "건너뜀 lx .claude/skills-alias/gone@ (가리킬 곳 없음)" && [ ! -L "$LX/.claude/skills-alias/gone" ] && [ ! -L "$W1/.claude/skills-alias/gone" ]'
t "나무 base: 파일마다 남는다" 'grep -q "^.agents/skills/alpha/ref/deep/er/notes.md	" "$W1/.local/state/herdr-sync/base.tsv" && grep -q "^.agents/skills/alpha/ref/deep/er/notes.md	" "$MAC/.local/state/herdr-sync/repo-base.tsv"'
run --check
t "나무 --check: 남은 것은 그 충돌 하나" '[ $rc = 1 ] && printf "%s\n" "$out" | grep -qx "w1: 충돌 1건" && printf "%s\n" "$out" | grep -qx "lx: 같음" && printf "%s\n" "$out" | grep -qx "맥: 저장소와 같음"'

# 15-2. 기계당 hc 호출 수는 나무 파일 수와 상관없다
put "$C/few/a.md" 'a\n' && put "$C/few/b/c.md" 'c\n'
rm -f "$T/calls"; run; few="$(grep -cx w1 "$T/calls") $(grep -cx lx "$T/calls")"
for x in $(seq 40); do put "$C/many/d$((x % 4))/f$x.md" "f$x\n"; done
rm -f "$T/calls"; run; many="$(grep -cx w1 "$T/calls") $(grep -cx lx "$T/calls")"
t "hc 호출 수: 새 파일 2개 ($few)와 40개 ($many)가 같다" '[ "$few" = "$many" ] && is "$LX/.claude/skills/many/d1/f9.md" "f9\n"'
put "$C/many/d0/f4.md" 'f4 바뀜\n'
rm -f "$T/calls"; run; one="$(grep -cx w1 "$T/calls") $(grep -cx lx "$T/calls")"
t "hc 호출 수: 바뀐 파일 하나도 같은 꼴 ($one)" '[ "$one" = "$many" ] && is "$W1/.claude/skills/many/d0/f4.md" "f4 바뀜\n"'

# 15-3. --take: 파일 하나, <폴더>/**
run --take 'w1:.nope/**'
t "--take 없는 나무: 종료 1" '[ $rc = 1 ] && printf "%s" "$out" | grep -Fq "없는 경로"'
run --take w1:.agents/skills/alpha/SKILL.md
t "--take 나무 파일 하나: 맥 값, 백업" '[ $rc = 0 ] && is "$W1A/alpha/SKILL.md" "alpha 1\n" && grep -rqx "alpha w1" "$W1/.local/state/herdr-sync/bak"'
put "$W1A/alpha/ref/deep/er/notes.md" 'w1 deep\n' && put "$W1A/alpha/한 글.md" 'w1 space\n'
run
t "손댄 나무 파일 둘: 충돌 2건, 그대로" '[ $rc = 1 ] && printf "%s\n" "$out" | grep -qx "w1: 충돌 2건" && is "$W1A/alpha/한 글.md" "w1 space\n"'
run --take 'w1:.agents/skills/**'
t "--take 기계:<폴더>/**: 그 나무 충돌 전부" '[ $rc = 0 ] && is "$W1A/alpha/ref/deep/er/notes.md" "deep 1\n" && is "$W1A/alpha/한 글.md" "space 1\n"'

# 15-4. --pull: 파일 하나, <폴더>/**
put "$W1A/alpha/SKILL.md" 'alpha from w1\n'
run --pull w1:.agents/skills/alpha/SKILL.md
t "--pull 나무 파일 하나: 맥·저장소로" '[ $rc = 0 ] && is "$A/alpha/SKILL.md" "alpha from w1\n" && [ "$(corigin show main:skills/agents/alpha/SKILL.md)" = "alpha from w1" ]'
put "$W1A/alpha/ref/deep/er/notes.md" 'w1 deep 2\n' && put "$W1A/alpha/한 글.md" 'w1 space 2\n'
run --pull 'w1:.agents/skills/**'
t "--pull 기계:<폴더>/**: 그 나무 충돌 전부를 맥·저장소로" '[ $rc = 0 ] && is "$A/alpha/한 글.md" "w1 space 2\n" && [ "$(corigin show "main:skills/agents/alpha/ref/deep/er/notes.md")" = "w1 deep 2" ] && is "$W1A/w1only/SKILL.md" "mine\n"'

# 15-5. 저장소만 바뀜(레인 착지) → 맥에 깖, 기계까지. 맥만 바뀜 → 저장소로. 실행 권한
git -C "$T/lane" pull -q
put "$T/lane/skills/agents/alpha/ref/deep/er/notes.md" 'lane deep\n' && put "$T/lane/skills/agents/alpha/new/from-lane.md" 'lane new\n'
git -C "$T/lane" add -- skills && git -C "$T/lane" commit -qm lane-tree && git -C "$T/lane" push -q origin main
run
t "나무 깖: 바뀐 파일·새 폴더의 새 파일이 맥과 기계에" '[ $rc = 0 ] && is "$A/alpha/ref/deep/er/notes.md" "lane deep\n" && is "$A/alpha/new/from-lane.md" "lane new\n" && is "$W1A/alpha/new/from-lane.md" "lane new\n" && has "맥 깖 .agents/skills/alpha/new/from-lane.md"'
chmod a-x "$A/alpha/scripts/run.sh" && put "$C/beta/SKILL.md" 'beta 2\n'
run
t "나무: 맥만 바뀐 내용과 실행 권한이 저장소·기계로" '[ $rc = 0 ] && corigin ls-tree main skills/agents/alpha/scripts/run.sh | grep -q "^100644" && [ ! -x "$W1A/alpha/scripts/run.sh" ] && [ "$(corigin show main:skills/claude/beta/SKILL.md)" = "beta 2" ] && is "$LX/.claude/skills/beta/SKILL.md" "beta 2\n"'
rm "$A/gamma/SKILL.md"
run
t "나무: 맥에서 지운 파일은 저장소에서 지우고 기계는 둔다" '[ $rc = 0 ] && ! corigin cat-file -e main:skills/agents/gamma/SKILL.md 2> /dev/null'

# 15-6. 깨진 전송: 나무 파일을 쓰지 않고 임시 폴더도 남기지 않는다
put "$C/beta/SKILL.md" 'beta 3\n'
out=$(HC_FAKE_CORRUPT=w1 "$SYNC" 2>&1); rc=$?
t "나무 깨진 전송: w1은 그대로, lx는 씀" '[ $rc = 1 ] && is "$W1/.claude/skills/beta/SKILL.md" "beta 2\n" && is "$LX/.claude/skills/beta/SKILL.md" "beta 3\n"'
t "나무 깨진 전송: 받는 쪽 임시 폴더 없음" '[ -z "$(find "$T/m" "$MAC" -path "*/herdr-sync/w.*")" ]'
run
t "깨진 뒤 다시: 맞춘다" '[ $rc = 0 ] && is "$W1/.claude/skills/beta/SKILL.md" "beta 3\n"'

# 15-7. hc는 입력·출력을 8MB에서 자른다: 나무 쓰기·충돌 읽기·가져오기를 7MB 아래 묶음 여러 번으로 나눈다
heavy() { awk -v id="$2" 'BEGIN { for (i = 0; i < 100000; i++) printf "%s line %07d xxxxxxxxxxxxxxxxxxxx\n", id, i }' > "$1"; }
mkdir -p "$C/heavy" && for x in 1 2 3; do heavy "$C/heavy/h$x.md" "h$x"; done   # 3.4MB씩, 합 10MB
rm -f "$T/calls"; run
t "큰 묶음(10MB): 나눠 써서 w1·lx에 다 간다" '[ $rc = 0 ] && cmp -s "$C/heavy/h3.md" "$W1/.claude/skills/heavy/h3.md" && cmp -s "$C/heavy/h1.md" "$LX/.claude/skills/heavy/h1.md" && [ "$(grep -cx w1 "$T/calls")" -ge 4 ]'
for x in 1 2 3; do echo "w1 덧붙임" >> "$W1/.claude/skills/heavy/h$x.md"; done
run
t "큰 충돌 내용 읽기도 나눈다: 줄 수가 맞다" '[ $rc = 1 ] && has "충돌 w1 .claude/skills/heavy/h1.md (1줄 다름)" && has "충돌 w1 .claude/skills/heavy/h3.md (1줄 다름)"'
run --pull 'w1:.claude/skills/**'
t "큰 --pull도 나눠 받는다" '[ $rc = 0 ] && cmp -s "$W1/.claude/skills/heavy/h3.md" "$C/heavy/h3.md" && tail -1 "$C/heavy/h2.md" | grep -qx "w1 덧붙임" && cmp -s "$C/heavy/h2.md" "$LX/.claude/skills/heavy/h2.md"'

# 15-8. 줄을 옮겨도 맥 파일을 지우지 않는다 (repo-base에 저장소 경로를 함께 적는다)
git -C "$T/lane" pull -q
grep -v '^스킬     .agents/skills/herdr-desktop/SKILL.md' "$T/lane/sync.list" > "$T/sl" && cat "$T/sl" > "$T/lane/sync.list"
git -C "$T/lane" commit -qam s1 && git -C "$T/lane" push -q origin main
run
t "S1 따로 적은 줄을 지움: 나무가 맡아 저장소로 모은다, 맥 파일 그대로" '[ $rc = 0 ] && is "$A/herdr-desktop/SKILL.md" "skill 1\n" && has "맥 모음 .agents/skills/herdr-desktop/SKILL.md" && [ "$(corigin show main:skills/agents/herdr-desktop/SKILL.md)" = "skill 1" ] && ! printf "%s" "$out" | grep -q "^맥 지움"'
git -C "$T/lane" pull -q
sed 's|skills/agents/  |skills/agents2/ |' "$T/lane/sync.list" > "$T/sl" && cat "$T/sl" > "$T/lane/sync.list"
git -C "$T/lane" commit -qam s2 && git -C "$T/lane" push -q origin main
run
t "S2 나무의 저장소 폴더를 바꿈: 새 폴더로 모은다, 맥 파일 그대로" '[ $rc = 0 ] && is "$A/alpha/한 글.md" "w1 space 2\n" && [ -f "$A/alpha/new/from-lane.md" ] && [ "$(corigin show main:skills/agents2/alpha/SKILL.md)" = "alpha from w1" ] && ! printf "%s" "$out" | grep -q "^맥 지움"'

# 15-9. 저장소 .gitignore에 걸리는 나무 파일은 맥·저장소·기계 모두에서 뺀다
git -C "$T/lane" pull -q && printf '*.bak-*\n' > "$T/lane/.gitignore" && git -C "$T/lane" add .gitignore && git -C "$T/lane" commit -qm ign && git -C "$T/lane" push -q origin main
put "$A/alpha/SKILL.md.bak-1" 'old\n'
run
t ".gitignore: 건너뜀 줄, 저장소 깨끗, 기계에 안 감" '[ $rc = 0 ] && has "건너뜀 .agents/skills/alpha/SKILL.md.bak-1 (저장소 .gitignore)" && [ -z "$(git -C "$T/conf" status --porcelain)" ] && [ ! -e "$W1A/alpha/SKILL.md.bak-1" ]'
# 커밋이 실패하면 이번에 스테이징·쓴 것을 되돌리고, 「맥 모음」 줄을 찍지 않는다. 다음 실행은 막히지 않는다
printf '#!/bin/sh\nexit 1\n' > "$T/conf/.git/hooks/pre-commit" && chmod +x "$T/conf/.git/hooks/pre-commit"
put "$A/alpha/SKILL.md" 'alpha 2\n' && put "$A/zeta/new.md" 'zeta\n'
run
t "커밋 실패: 종료 1, 저장소 깨끗, 「맥 모음」 없음" '[ $rc = 1 ] && [ -z "$(git -C "$T/conf" status --porcelain)" ] && ! printf "%s" "$out" | grep -q "^맥 모음"'
rm -f "$T/conf/.git/hooks/pre-commit"
run
t "커밋 실패 뒤 다시: 모은다" '[ $rc = 0 ] && [ "$(corigin show main:skills/agents2/zeta/new.md)" = zeta ] && is "$W1A/alpha/SKILL.md" "alpha 2\n"'

# 15-10. 받는 쪽 W는 끝난 pid의 w.* 임시 폴더를 지운다
mkdir -p "$W1/.local/state/herdr-sync/w.99999999/x" && put "$A/zeta/new.md" 'zeta 2\n'
run
t "묵은 w.* 임시 폴더를 지운다" '[ $rc = 0 ] && [ ! -e "$W1/.local/state/herdr-sync/w.99999999" ] && is "$W1A/zeta/new.md" "zeta 2\n"'

# 15-11. 옛 꼴 repo-base(셋째 칸 없음)에서 줄을 옮긴 첫 실행: 저장소 쪽이 없다고 맥 파일을 지우지 않고 충돌로 멈춘다
RB=$MAC/.local/state/herdr-sync/repo-base.tsv
cut -f1,2 "$RB" > "$T/rb" && cat "$T/rb" > "$RB"
git -C "$T/lane" pull -q
sed 's|skills/agents2/ |skills/agents3/ |' "$T/lane/sync.list" > "$T/sl" && cat "$T/sl" > "$T/lane/sync.list"
git -C "$T/lane" commit -qam legacy && git -C "$T/lane" push -q origin main
run
t "옛 꼴 base + 줄 옮김: 충돌로 멈추고 맥 파일 그대로" '[ $rc = 1 ] && is "$A/alpha/SKILL.md" "alpha 2\n" && has "충돌 저장소 .agents/skills/alpha/SKILL.md (저장소에 없음 — 옛 base라 지운 것인지 모른다)" && ! printf "%s" "$out" | grep -q "^맥 지움"'
run --take 'repo:.agents/skills/**'
t "옛 꼴 base 충돌: --take repo:<폴더>/**로 새 자리에 모은다" '[ $rc = 0 ] && [ "$(corigin show main:skills/agents3/alpha/SKILL.md)" = "alpha 2" ] && is "$A/alpha/SKILL.md" "alpha 2\n"'

# 15-12. 모으기 실패 되돌리기는 이번에 쓴 경로만: 그 사이 남이 같은 저장소에 쓴 새 파일·고친 줄은 남는다
printf '#!/bin/sh\necho other > other-agent.txt\necho "edit by other" >> private-patterns.txt\nexit 1\n' > "$T/conf/.git/hooks/pre-commit" && chmod +x "$T/conf/.git/hooks/pre-commit"
put "$A/alpha/SKILL.md" 'alpha 3\n' && put "$A/eta/new.md" 'eta\n'
run
t "되돌리기 범위: 남의 새 파일·남의 줄은 남고, 이번에 쓴 것만 되돌린다" '[ $rc = 1 ] && [ -e "$T/conf/other-agent.txt" ] && grep -qx "edit by other" "$T/conf/private-patterns.txt" && [ "$(git -C "$T/conf" status --porcelain --untracked-files=all | sort | tr "\n" "|")" = " M private-patterns.txt|?? other-agent.txt|" ]'
rm -f "$T/conf/.git/hooks/pre-commit" "$T/conf/other-agent.txt" && git -C "$T/conf" checkout -q -- private-patterns.txt
run
t "되돌린 뒤 다시: 모은다" '[ $rc = 0 ] && [ "$(corigin show main:skills/agents3/eta/new.md)" = eta ]'

# 13. 잠금: 한 번에 하나
lock=$MAC/.local/state/herdr-sync/lock
lockf -s -t 0 "$lock" sh -c 'touch "$1"; while [ ! -e "$2" ]; do sleep 0.1; done' _ "$T/held" "$T/release" &
pid=$!
for _ in $(seq 50); do [ -e "$T/held" ] && break; sleep 0.1; done
run --check
touch "$T/release"; wait "$pid"
t "잠금: 다른 herdr-sync가 돌면 바로 끝" '[ $rc = 75 ] && printf "%s" "$out" | grep -Fq "다른 herdr-sync"'

# 13-1. 범용 저장소에 lib/<도구>/가 생겼는데 개인 sync.list에 bin 줄만 있으면(2026-11-20 herdr-tools-split) 아무 데도 쓰지 않고 멈춘다.
# bin만 깔리면 모든 기계의 그 도구가 「함수 파일을 못 읽었다」로 멈추기 때문. --check도 같은 판정
put "$T/repo/lib/hc-lane/a.sh" 'a=1\n'
git -C "$T/repo" add lib && git -C "$T/repo" commit -qm lib && HERDR_LAND=1 git -C "$T/repo" push -q origin main
put "$T/repo/bin/hc-lane" '#!/bin/sh\necho lib-lane\n' && git -C "$T/repo" commit -qam lib-lane && HERDR_LAND=1 git -C "$T/repo" push -q origin main
before=$(snap)
run --check
t "lib 줄 없음 --check: 멈추고 고칠 줄을 찍는다" '[ $rc = 2 ] && has "도구  .local/lib/hc-lane/**  tools:lib/hc-lane/  macos"'
run
t "lib 줄 없음: 아무 기계에도 쓰지 않고 멈춘다 (새 bin도 안 깖)" '[ $rc = 2 ] && [ "$(snap)" = "$before" ] && ! grep -q lib-lane "$W1/.local/bin/hc-lane" && has "도구  .local/lib/hc-lane/**  tools:lib/hc-lane/  macos"'
printf '도구     .local/lib/hc-lane/**       tools:lib/hc-lane/             macos\n' >> "$T/conf/sync.list"
git -C "$T/conf" commit -qam lib && HERDR_LAND=1 git -C "$T/conf" push -q origin main
run
t "lib 줄을 더하면: bin과 lib를 함께 깐다" '[ $rc = 0 ] && is "$W1/.local/lib/hc-lane/a.sh" "a=1\n" && is "$W1/.local/bin/hc-lane" "#!/bin/sh\necho lib-lane\n"'

# 14. 범용 저장소의 config.example/sync.list: 꼴이 맞고, tools: 경로가 모두 있고, bin/*을 모두 깐다
LIST=$here/../config.example/sync.list
if [ -f "$LIST" ]; then
  t "config.example/sync.list: 줄마다 네 칸, 대상이 새 이름" '! grep -v "^#" "$LIST" | awk "NF && (NF != 4 || \$4 !~ /^(all|macos|local|[a-z0-9,-]+)\$/ || \$4 ~ /(^|,)(all9|mac7|mac-only)(,|\$)/)" | grep -q .'
  for p in $(awk '!/^#/ && $3 ~ /^tools:/ {print substr($3, 7)}' "$LIST"); do
    t "config.example/sync.list: tools:$p 있음" '[ -e "$here/../$p" ]'
  done
  for f in "$here"/../bin/*; do b=${f##*/}; t "config.example/sync.list: bin/$b" 'grep -q " tools:bin/$b " "$LIST"'; done
  # lib/<도구>/(본체가 source하는 함수 파일)는 나무 줄로, 대상은 그 도구 줄과 같게(빠지면 그 기계의 도구가 종료 1)
  # 도구가 함께 쓰는 lib(bin 없음, lib/<이름>/OWNER에 쓰는 도구 이름)는 그 도구 줄과 견준다 — 예: hc-merger(hc-lane이 빌드해 부르는 병합기 Go 판)
  # Go 판 도구(go.mod·cmd/<이름>)는 소스로 깐다: Go 줄 넷이 한 번에, 대상이 같게(2026-11-21 go-foundation)
  if [ -f "$here/../go.mod" ]; then
    golines=$(awk '$3 == "tools:go.mod" || $3 == "tools:cmd/" || $3 == "tools:internal/" || $3 == "tools:lib/go-run.sh" { print $3, $4 }' "$LIST" | sort | paste -sd , -)
    t "config.example/sync.list: Go 소스 나무 줄 넷, 대상이 같음" '[ "$golines" = "tools:cmd/ all,tools:go.mod all,tools:internal/ all,tools:lib/go-run.sh all" ]'
  fi
  # 점 파일만 남은 폴더(옮긴 도구의 무시된 빌드 산출물 — 예: 옛 lib/hc-merger/.bin)는 lib가 아니다
  for f in "$here"/../lib/*/; do [ -d "$f" ] && [ -n "$(ls "$f")" ] || continue; b=$(basename "$f"); o=$b
    [ ! -f "$f/OWNER" ] || o=$(head -n 1 "$f/OWNER")
    t "config.example/sync.list: lib/$b/ 나무 줄, 대상은 bin/${o}와 같음" \
      '[ -n "$(awk -v b="$o" "\$3 == \"tools:bin/\" b {print \$4}" "$LIST")" ] && [ "$(awk -v b="$b" "\$3 == \"tools:lib/\" b \"/\" && \$2 ~ /\\/\\*\\*\$/ {print \$4}" "$LIST")" = "$(awk -v b="$o" "\$3 == \"tools:bin/\" b {print \$4}" "$LIST")" ]'
  done
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

# 16. 릴리스(Go 판만, 2026-11-21 사용자 「github 에다가 빌드 한것도 처리해서 … 각 컴퓨터에서는 받아서」): 내보내기가 되면 맥에서 cmd/*를 지어
# publish.repo 릴리스 bin-<커밋 12자>에 올리고(가짜 gh — $T/rel/<태그>/), macOS 기계는 맥이 넘긴 sha256이 맞을 때만 받는다(file:// 주소)
if [ "$HERDR_SYNC_IMPL" = go ]; then
  cat > "$T/bin/gh" << 'EOF'
#!/bin/bash
echo "$*" >> "$HCT/gh.calls"
[ "$1" = release ] || exit 2
op=$2 tag=$3; shift 3
d=$HCT/rel/$tag
case $op in
  view) [ -d "$d" ] ;;
  download) while [ $# -gt 0 ]; do case $1 in --pattern) p=$2; shift ;; --dir) o=$2; shift ;; esac; shift; done
    [ -f "$d/$p" ] && cp "$d/$p" "$o/" ;;
  create | upload) [ -z "${GH_FAKE_FAIL-}" ] || exit 1
    mkdir -p "$d"; while [ $# -gt 0 ]; do case $1 in --repo | --title | --notes) shift ;; --clobber) ;; *) cp "$1" "$d/" ;; esac; shift; done ;;
  *) exit 2 ;;
esac
EOF
  chmod +x "$T/bin/gh"
  export HERDR_SYNC_RELEASE_URL=file://$T/rel GOCACHE=${GOCACHE:-$here/../.bin/go-build}
  put "$T/repo/go.mod" 'module herdr-tools\n\ngo 1.24\n'
  put "$T/repo/cmd/hello/main.go" 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("hello") }\n'
  git -C "$T/repo" add go.mod cmd && git -C "$T/repo" commit -qm go && HERDR_LAND=1 git -C "$T/repo" push -q origin main
  sed -i '' 's|^publish.repo .*|publish.repo  https://github.com/test/pub.git|' "$T/conf/config"
  git -C "$T/conf" commit -qam rel && HERDR_LAND=1 git -C "$T/conf" push -q origin main
  for h in "$MAC" "$W1" "$LX"; do put "$h/.local/lib/herdr-go/go.mod" 'module herdr-tools\n'; done   # Go 소스를 깐 기계(받는 자리)
  run
  c12=$(git -C "$T/repo" rev-parse HEAD | cut -c1-12) tag=bin-$(git -C "$T/repo" rev-parse HEAD | cut -c1-12)
  src=$(go_run_root=$T/repo; . "$here/../lib/go-run.sh"; go_run_root=$T/repo; go_run_src)
  t "릴리스: 맥에서 지어 올림(바이너리 + SHA256SUMS)" '[ $rc = 0 ] && has "릴리스 올림 $tag" && [ -x "$T/rel/$tag/hello" ] && grep -q "  hello$" "$T/rel/$tag/SHA256SUMS"'
  t "릴리스: 커밋을 바이너리에 새김(빌드 입력은 범용 저장소)" '"$T/rel/$tag/hello" >/dev/null && [ "$(shasum -a 256 "$T/rel/$tag/hello" | cut -d" " -f1)" = "$(cut -d" " -f1 "$T/rel/$tag/SHA256SUMS")" ]'
  t "받기: macOS 기계·맥은 해시가 맞으면 받고 소스 판을 곁에" 'has "w1 받음 hello ($tag)" && [ "$("$W1/.local/lib/herdr-go/.bin/hello")" = hello ] && [ "$(cat "$W1/.local/lib/herdr-go/.bin/hello.src")" = "$src" ] && [ -x "$MAC/.local/lib/herdr-go/.bin/hello" ]'
  t "받기: 리눅스는 받지 않는다(darwin/arm64만)" '[ ! -e "$LX/.local/lib/herdr-go/.bin/hello" ]'
  : > "$T/gh.calls"
  run
  t "릴리스: 같은 커밋·같은 해시는 다시 안 올림, 이미 받은 기계는 다시 안 받음" '[ $rc = 0 ] && has "릴리스 같음 $tag" && ! grep -Eq "^release (create|upload)" "$T/gh.calls" && ! printf "%s" "$out" | grep -q " 받음 hello"'
  printf 'x' >> "$T/rel/$tag/hello"; rm -f "$W1/.local/lib/herdr-go/.bin/hello" "$W1/.local/lib/herdr-go/.bin/hello.src"
  run
  t "받기: 받은 것의 해시가 다르면 쓰지 않고 크게 경고, 맞추기 종료 코드는 그대로" '[ $rc = 0 ] && [ ! -e "$W1/.local/lib/herdr-go/.bin/hello" ] && printf "%s" "$out" | grep -q "!!! herdr-sync: w1 hello 바이너리를 받지 않음: 해시가 다르다"'
  t "받기: 실패한 받기는 임시 파일을 남기지 않음" '[ -z "$(find "$W1/.local/lib/herdr-go/.bin" -name ".hello.*")" ]'
  rm -rf "$T/rel/$tag"
  GH_FAKE_FAIL=1 run
  t "릴리스: 올리기 실패는 크게 경고, 받지 않음, 종료 코드 그대로" '[ $rc = 0 ] && has "!!! herdr-sync: 릴리스 만들기 실패 $tag (gh release create — 맥 gh 인증·공개 미러 쓰기 권한) — 기계는 각자 빌드한다 !!!" && [ ! -e "$W1/.local/lib/herdr-go/.bin/hello" ]'
  unset HERDR_SYNC_RELEASE_URL
fi

t "임시 파일이 남지 않는다 (맥·기계 쪽 조각 모두)" '[ -z "$(ls -A "$T/tmp")" ]'

echo "$((n - fails))/$n 통과"
[ $fails = 0 ]
