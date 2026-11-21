# worker-mac-setup 원격 자동 단계: 그 기계에서 SSH 문맥으로 bash가 돈다(원격에는 Go가 없다). 셸 판 lib/worker-mac-setup/worker-mac-setup.sh의
# REMOTE와 같은 내용. 앞에 설정 값(TRUST·SLOTS_FILE)이 붙는다. 인자: DRY SLOTS 날짜. 종료 3 = GUI 로그인 세션 없음
set -u
DRY=$1; SLOTS=$2; D=$3
export PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin"
do_() { if [ "$DRY" = 1 ]; then echo "  (dry-run) $*"; else eval "$@"; fi; }
u=$(id -un); uid=$(id -u); ST="$HOME/.local/state/worker-mac-setup"
set -f; tl=($TRUST); set +f   # 빈칸으로만 나눈다(*·? 펼치지 않음)
mk=(); [ ${#tl[@]} -gt 0 ] && mk+=("$HOME/${tl[0]}")
[ -n "$SLOTS_FILE" ] && mk+=("$(dirname "$HOME/$SLOTS_FILE")")
mk+=("$HOME/.local/bin" "$HOME/Library/LaunchAgents" "$HOME/.claude-tasks" "$ST")
echo "▸ 폴더"; do_ "mkdir -p $(printf '%q ' "${mk[@]}")"

if [ -n "$SLOTS_FILE" ]; then
  vs="$HOME/$SLOTS_FILE"; vn=${SLOTS_FILE##*/}
  if [ -f "$vs" ]; then echo "▸ $vn: 이미 $(cat "$vs") (그대로 둔다)"; else echo "▸ $vn = $SLOTS"; do_ "echo $SLOTS > $(printf %q "$vs")"; fi
fi

# Claude 첫 실행 안내·폴더 신뢰. 워크트리는 원본 저장소의 신뢰를 따른다(그래서 저장소 폴더와 그 부모를 넣는다).
cj="$HOME/.claude.json"
if ! command -v jq >/dev/null; then echo "▸ Claude 신뢰: jq 없음 — 사람 몫 체크리스트 참고"
else
  [ -f "$cj" ] || do_ "echo '{}' > '$cj'; chmod 600 '$cj'"
  if [ "$DRY" = 1 ]; then echo "  (dry-run) ~/.claude.json: hasCompletedOnboarding=true, 신뢰 $TRUST"
  else
    cp -p "$cj" "$cj.bak-worker-setup-$D"
    # 링크를 푼 실제 경로로 넣는다(없는 폴더는 부모까지 풀고 이름을 붙인다)
    rpath() { (cd -P "$1" 2>/dev/null && pwd -P) || { d=$(cd -P "${1%/*}" 2>/dev/null && pwd -P) && echo "$d/${1##*/}"; } || echo "$1"; }
    tp=(); for p in ${tl[@]+"${tl[@]}"}; do tp+=("$(rpath "$HOME/$p")"); done
    jq '.hasCompletedOnboarding=true
      | reduce $ARGS.positional[] as $p (.; .projects[$p].hasTrustDialogAccepted=true)' "$cj.bak-worker-setup-$D" --args ${tp[@]+"${tp[@]}"} > "$cj.tmp" \
      && mv "$cj.tmp" "$cj" && chmod 600 "$cj" && echo "▸ Claude 첫 실행 안내·신뢰 넣음 (백업 ~/.claude.json.bak-worker-setup-$D)"
  fi
fi

# Chrome 첫 실행: 코어(launchd) 문맥에서 처음 뜨면 사용자 자료 폴더를 못 만들어 죽는다(2026-11-06 워커 맥 실측). SSH에서 한 번 띄워 둔다.
C="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
if [ -x "$C" ] && [ ! -d "$HOME/Library/Application Support/Google/Chrome" ]; then
  echo "▸ Chrome 첫 실행(헤드리스 한 번)"; do_ "perl -e 'alarm 40; exec @ARGV' \"\$C\" --headless=new --disable-gpu --dump-dom about:blank >/dev/null 2>&1; true"
elif [ -x "$C" ]; then echo "▸ Chrome: 자료 폴더 있음"; else echo "▸ Chrome: 없음 (브라우저 점검에 필요, 설치는 도구 단계)"; fi

# 관리자 단계 스크립트 사본(사람이 sudo로 한 번 실행)
echo "▸ 관리자 단계 스크립트 사본 $ST/root-setup.sh$([ "$DRY" = 1 ] && echo " (dry-run: 쓰지 않음)")"
[ "$DRY" = 1 ] || cat > "$ST/root-setup.sh" <<'ROOT'
#!/bin/bash
# 워커 맥 관리자 단계 (worker-mac-setup). 사람이 sudo 암호를 넣어 한 번 실행한다.
#  1) 덮개를 닫아도·배터리에서도 잠들지 않게: pmset -a disablesleep 1   (가방에 넣을 땐 sudo pmset -a disablesleep 0)
#  2) 원격 로그인 키 전용: /etc/ssh/sshd_config.d/050-keyonly.conf   (authorized_keys에 키가 있을 때만)
#  3) Xcode 앱이 있으면 라이선스 동의
# 되돌리기: sudo pmset -a disablesleep 0 ; sudo rm /etc/ssh/sshd_config.d/050-keyonly.conf
set -u
[ "$(id -u)" -eq 0 ] || { echo "sudo로 실행해야 합니다"; exit 1; }
U=${SUDO_USER:-}; [ -n "$U" ] || { echo "sudo로 실행해야 합니다(SUDO_USER 없음)"; exit 1; }
echo "[1/3] 잠자기 끄기"; /usr/bin/pmset -a disablesleep 1; /usr/bin/pmset -g | grep -i SleepDisabled
echo "[2/3] 원격 로그인 키 전용"
AK=$(eval echo "~$U")/.ssh/authorized_keys
if [ -s "$AK" ] && grep -q '^[^#]*ssh-' "$AK"; then
  F=/etc/ssh/sshd_config.d/050-keyonly.conf
  printf '# 워커 맥(worker-mac-setup): 원격 로그인은 키로만. 되돌리기: 이 파일을 지운다.\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nPermitRootLogin no\n' > "$F"; chmod 644 "$F"
  if /usr/sbin/sshd -t; then /usr/sbin/sshd -T 2>/dev/null | grep -iE '^(passwordauthentication|kbdinteractiveauthentication) '
  else echo "sshd 설정 검사 실패 — 파일을 지우고 원래대로 둡니다"; rm -f "$F"; fi
else echo "authorized_keys에 키가 없어 건너뜀(잠김 방지)"; fi
echo "[3/3] Xcode 라이선스"
if ls -d /Applications/Xcode*.app >/dev/null 2>&1; then xcodebuild -license accept && echo "동의함"; else echo "Xcode 앱 없음 — 건너뜀"; fi
echo ROOT_SETUP_DONE
ROOT
[ "$DRY" = 1 ] || chmod 755 "$ST/root-setup.sh"

# herdr-core launchd 작업(로그인 때와 60초마다 server start)과 GUI 문맥 코어
lab="com.$u.herdr-core"; ex=$(ls "$HOME/Library/LaunchAgents/" 2>/dev/null | grep -E 'herdr-core\.plist$' | head -1)
[ -n "$ex" ] && lab=${ex%.plist}
pl="$HOME/Library/LaunchAgents/$lab.plist"
if [ -f "$pl" ]; then echo "▸ 코어 launchd plist: 있음 ($lab)"
else echo "▸ 코어 launchd plist 만들기 ($lab)"
  [ "$DRY" = 1 ] || cat > "$pl" <<PL
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>$lab</string>
  <key>ProgramArguments</key><array><string>$HOME/.local/bin/herdr-core</string><string>server</string><string>start</string></array>
  <key>EnvironmentVariables</key><dict>
    <key>PATH</key><string>$HOME/.local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    <key>LANG</key><string>en_US.UTF-8</string><key>LC_ALL</key><string>en_US.UTF-8</string>
  </dict>
  <key>RunAtLoad</key><true/><key>StartInterval</key><integer>60</integer>
  <key>AbandonProcessGroup</key><true/><key>ProcessType</key><string>Interactive</string>
  <key>StandardOutPath</key><string>$HOME/Library/Logs/herdr-core-launchd.log</string>
  <key>StandardErrorPath</key><string>$HOME/Library/Logs/herdr-core-launchd.log</string>
</dict></plist>
PL
fi
if ! pgrep -x Dock >/dev/null; then
  echo "✗ GUI 로그인 세션이 없다(그 맥에 로그인 전). 자동 로그인을 켜고 로그인한 뒤 다시 돌린다."; exit 3
fi
launchctl print "gui/$uid/$lab" >/dev/null 2>&1 || { echo "▸ launchctl bootstrap gui/$uid $lab"; do_ "launchctl bootstrap gui/$uid '$pl'"; }
sp=$(pgrep -f 'herdr-core serve' | head -1); da=$(launchctl print "pid/$(pgrep -x Dock | head -1)" | awk '/asid =/{print $3; exit}')
ca=""; [ -n "$sp" ] && ca=$(launchctl print "pid/$sp" 2>/dev/null | awk '/asid =/{print $3; exit}')
if [ -n "$sp" ] && [ "$ca" = "$da" ]; then echo "▸ 코어: 이미 GUI 문맥"
else
  live=$(herdr-core server status --json 2>/dev/null | sed -nE 's/.*"live":([0-9]+).*/\1/p')
  if [ -n "$sp" ] && [ "${live:-1}" != 0 ]; then
    echo "✗ 코어가 SSH 문맥인데 살아 있는 터미널이 ${live:-?}개라 다시 띄우지 않는다(터미널이 끊긴다). 지휘가 정한다."
  else
    echo "▸ 코어를 GUI 문맥으로 다시 띄움 (hc server stop; launchctl kickstart — 한 명령으로 붙여야 SSH 재접속이 먼저 띄우지 않는다)"
    do_ "hc server stop >/dev/null 2>&1; launchctl kickstart -k gui/$uid/$lab; sleep 3"
    [ "$DRY" = 1 ] || { sp=$(pgrep -f 'herdr-core serve' | head -1); ca=$(launchctl print "pid/$sp" 2>/dev/null | awk '/asid =/{print $3; exit}'); [ "$ca" = "$da" ] && echo "  코어 GUI 문맥 확인" || echo "  ✗ 아직 GUI 문맥 아님(asid ${ca:-?} ≠ ${da:-?})"; }
  fi
fi
