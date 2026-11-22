set -u
H=$HOME; U=$(id -un); L="com.$U.herdr-core-restart"; P="$H/Library/LaunchAgents/$L.plist"; S="$H/.local/state/core-restart.sh"
B="$H/.local/bin/herdr-core"; [ -x "$B" ] || { echo "✗ $B 없음"; exit 1; }
mkdir -p "$H/.local/state" "$H/Library/LaunchAgents" "$H/Library/Logs"
cat > "$S" <<'EOF'
#!/bin/bash
pk=$(pgrep -f "herdr-core connect" | tr '\n' ' ')
# 바깥 bootout(10초)이 이 작업을 끊어도 connect를 풀어 둔다
trap '[ -n "$pk" ] && kill -CONT $pk' EXIT
trap 'exit 143' TERM
[ -n "$pk" ] && kill -STOP $pk
# --fresh + 로그인 셸: 그냥 restart는 코어가 처음 뜬 환경을 물려준다. SSH로 처음 뜬 코어면 ~/.zshenv만 읽은 PATH라
# hc 터미널에 brew(~/.zprofile)가 없다(2026-11-16 실측, railway·bunx 종료 127). 로그인 셸에서 --fresh로 새 터미널 환경을 바꾼다.
"${SHELL:-/bin/zsh}" -lc '"$HOME/.local/bin/herdr-core" server restart --fresh'
sleep 4
echo "멈췄다 푼 connect: ${pk:-없음}"
EOF
cat > "$P" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>$L</string>
<key>ProgramArguments</key><array><string>/bin/bash</string><string>$S</string></array>
<key>EnvironmentVariables</key><dict><key>PATH</key><string>$H/.local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin</string><key>LANG</key><string>${LANG:-en_US.UTF-8}</string><key>HOME</key><string>$H</string></dict>
<key>RunAtLoad</key><true/>
<key>StandardOutPath</key><string>$H/Library/Logs/herdr-core-restart.log</string>
<key>StandardErrorPath</key><string>$H/Library/Logs/herdr-core-restart.log</string>
</dict></plist>
EOF
: > "$H/Library/Logs/herdr-core-restart.log"
echo "전: $(pgrep -lf '(herdr-core|/hc) (serve|--hc-launchd)' | cut -c1-70)"
launchctl bootstrap "gui/$(id -u)" "$P"; sleep 10
launchctl bootout "gui/$(id -u)/$L" 2>/dev/null; rm -f "$P"
echo "후: $(pgrep -lf '(herdr-core|/hc) (serve|--hc-launchd)' | cut -c1-70)"
tail -1 "$H/Library/Logs/herdr-core-restart.log"
