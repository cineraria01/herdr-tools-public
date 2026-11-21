# worker-mac-setup mesh 원격 조각: 그 기계에서 bash가 돈다(원격에는 Go가 없다). 셸 판의 MESH_HEAD·MESH_INFO·MESH_AUTH·MESH_CLI와 같은 내용.
# 「#@ 이름」 줄이 조각을 나눈다. 한 번 부를 때 「<설정 값>;」 + head + 조각 하나를 보내고, 인자는 CHECK 날짜다
#@ head
set -u; CHECK=$1; D=$2
export PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
nl() { [ -s "$1" ] && [ -n "$(tail -c1 "$1")" ] && echo >>"$1"; true; }
bak() { [ -f "$1" ] && [ ! -e "$1.bak-mesh-$D" ] && cp -p "$1" "$1.bak-mesh-$D"; true; }
fpof() { awk '!/^[[:space:]]*#/ && $2=="ssh-ed25519"' | ssh-keygen -lf - 2>/dev/null | awk '{print $2}'; }
#@ info
# 기계 하나: 키(--check가 아니면 없을 때 만듦), 계정, Tailscale 주소, 공개 키. 앞에 NAME
k=$HOME/.ssh/id_ed25519
ts() { local t; for t in tailscale /Applications/Tailscale.app/Contents/MacOS/Tailscale; do command -v "$t" >/dev/null 2>&1 && { "$t" ip "$1" 2>/dev/null | head -1; return; }; done; }
if [ "$CHECK" = 0 ]; then
  mkdir -p "$HOME/.ssh" && chmod 700 "$HOME/.ssh"
  if [ -f "$k" ] && [ ! -f "$k.pub" ]; then ssh-keygen -y -f "$k" >"$k.pub" </dev/null
  elif [ ! -f "$k" ]; then ssh-keygen -q -t ed25519 -N "" -C "$NAME-workers-$D" -f "$k" </dev/null && echo new=1; fi
fi
echo "user=$(id -un)"; echo "ts4=$(ts -4)"; echo "ts6=$(ts -6)"
[ -f "$k.pub" ] && echo "pub=$(head -1 "$k.pub")"; true
#@ auth
# 받는 쪽 B: 보낸 쪽 공개 키(PUB)를 from=FROM으로
f=$HOME/.ssh/authorized_keys; body=$(printf "%s" "$PUB" | awk "{print \$2}")
if [ -z "$PUB" ]; then echo "키 없음"
elif [ -f "$f" ] && awk -v b="$body" '!/^[[:space:]]*#/ { for (i = 1; i <= NF; i++) if ($i == b) x = 1 } END { exit !x }' "$f"; then echo 있음
elif [ "$CHECK" = 1 ]; then echo 없음
else mkdir -p "$HOME/.ssh" && chmod 700 "$HOME/.ssh"; bak "$f"; nl "$f"
  printf "from=\"%s\",no-agent-forwarding,no-X11-forwarding %s\n" "$FROM" "$PUB" >>"$f" && chmod 600 "$f" && echo 넣음; fi
#@ cli
# 보내는 쪽 A: B(H4·BU·FP)로 가는 known_hosts·config·hc·접속. 지문이 FP와 다르면 종료 4
kf=$HOME/.ssh/known_hosts; cf=$HOME/.ssh/config
cur=$(ssh-keygen -F "$H4" -f "$kf" 2>/dev/null | fpof)
if [ -n "$cur" ]; then
  bad=$(printf "%s\n" "$cur" | grep -vxF -- "$FP" | head -1)
  [ -z "$bad" ] || { echo "kh=지문 다름(있는 줄 $bad)"; exit 4; }; echo kh=있음
elif [ "$CHECK" = 1 ]; then echo kh=없음
else line=$(ssh-keyscan -T 8 -t ed25519 "$H4" 2>/dev/null | awk '!/^#/ && $2=="ssh-ed25519"' | head -1); got=$(printf "%s\n" "$line" | fpof)
  [ -n "$got" ] && [ "$got" = "$FP" ] || { echo "kh=지문 다름(ssh-keyscan ${got:-읽지 못함})"; exit 4; }
  mkdir -p "$HOME/.ssh" && chmod 700 "$HOME/.ssh"; bak "$kf"; nl "$kf"; printf "%s\n" "$line" >>"$kf"; echo kh=넣음
fi
g() { ssh -G "$B" 2>/dev/null | awk -v k="$1" '$1 == k { print $2; exit }'; }
gh=$(g hostname); gu=$(g user)
if [ "$gh" = "$H4" ] && [ "$gu" = "$BU" ]; then echo cfg=있음
elif [ "$gh" != "$B" ]; then echo "cfg=다름(HostName $gh, User $gu — 손으로 본다)"
elif [ "$CHECK" = 1 ]; then echo cfg=없음
else bak "$cf"; nl "$cf"; printf "\nHost %s\n    HostName %s\n    User %s\n" "$B" "$H4" "$BU" >>"$cf" && chmod 600 "$cf" && echo cfg=넣음; fi
if hc machine ls 2>/dev/null | grep -q "\"name\":\"$B\""; then echo hc=있음
elif [ "$CHECK" = 1 ]; then echo hc=없음
elif o=$(perl -e "alarm 60; exec @ARGV" hc machine add "$B" --name "$B" 2>&1 </dev/null); then echo hc=넣음
else echo "hc=실패($(printf "%s" "$o" | tail -1))"; fi
ssh -n -o BatchMode=yes -o ConnectTimeout=8 "$B" true </dev/null >/dev/null 2>&1 && echo conn=됨 || echo "conn=안 됨"
