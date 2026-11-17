#!/bin/bash
# worker-mac-setup mesh(워커끼리 ssh·hc 연결) 시험: 가짜 ssh·ssh-keyscan·tailscale·hc, 기계마다 임시 HOME($T/h/<기계>).
# ssh-keygen은 진짜를 쓴다(임시 키). 실제 기계·네트워크는 쓰지 않는다.
# 실행: bash tests/worker-mesh.test.sh   (다른 사본은 WORKER_MAC_BIN=<bin 폴더>)
set -u
here=$(cd "$(dirname "$0")" && pwd)
BIN=${WORKER_MAC_BIN:-$here/../bin}
T=$(mktemp -d "${TMPDIR:-/tmp}/worker-mesh-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
trap 'rm -rf "$T"' EXIT
export T T_LOG=$T/log HOME=$T/mac
mkdir -p "$T/bin" "$T_LOG" "$HOME/.ssh" "$T/ip" "$T/conf"
ln -s "$BIN/herdr-tools-config" "$T/bin/herdr-tools-config"
export HERDR_TOOLS_CONFIG=$T/conf PATH="$T/bin:$PATH"

# 가짜 ssh. 맥(HOME=$T/mac)에서: 기계 이름을 ssh.log에, 그 기계 HOME에서 sh -c. 없는 기계는 255.
# 워커에서: -G <이름>은 ~/.ssh/config Host 블록의 hostname·user, 접속은 config로 주소를 풀어
# 그 기계 known_hosts에 주소가 있고 받는 기계 authorized_keys에 내 공개 키가 있을 때만 0(conn.log에 「보낸→받은」).
cat >"$T/bin/ssh" <<'EOF'
#!/bin/bash
G=0
while [ $# -gt 0 ]; do case $1 in -o) shift 2 ;; -n|-t) shift ;; -G) G=1; shift ;; *) break ;; esac; done
cfg() { awk -v h="$1" -v k="$2" 'tolower($1)=="host"{m=0; for(i=2;i<=NF;i++) if($i==h) m=1; next} m && tolower($1)==k {print $2; exit}' "$HOME/.ssh/config" 2>/dev/null; }
if [ "$HOME" = "$T/mac" ]; then
  echo "$1" >>"$T_LOG/ssh.log"
  [ -d "$T/h/$1" ] || exit 255
  m=$1; shift
  cd "$T/h/$m" && HOME=$T/h/$m PATH=$T/h/$m/.local/bin:/usr/bin:/bin exec /bin/sh -c "$*"
fi
h=$(cfg "$1" hostname); u=$(cfg "$1" user)
if [ $G = 1 ]; then echo "hostname ${h:-$1}"; echo "user ${u:-$(id -un)}"; exit 0; fi
[ -n "$h" ] && [ -f "$T/ip/$h" ] || exit 255
to=$(cat "$T/ip/$h")
ssh-keygen -F "$h" -f "$HOME/.ssh/known_hosts" >/dev/null 2>&1 || exit 255
body=$(awk '{print $2}' "$HOME/.ssh/id_ed25519.pub")
grep -qF "$body" "$T/h/$to/.ssh/authorized_keys" 2>/dev/null || exit 255
echo "${HOME##*/}→$to" >>"$T_LOG/conn.log"
EOF
# 가짜 ssh-keyscan: 마지막 인자 주소의 호스트 키 줄($T/hk/<주소>)
cat >"$T/bin/ssh-keyscan" <<'EOF'
#!/bin/bash
for a; do :; done
cat "$T/hk/$a" 2>/dev/null
EOF
# 가짜 tailscale: ip -4 / ip -6 → ~/.ts4 / ~/.ts6
cat >"$T/bin/tailscale" <<'EOF'
#!/bin/bash
[ "$1" = ip ] && cat "$HOME/.ts${2#-}"
EOF
# 가짜 hc(워커 쪽): machine ls는 ~/.hcm 줄, machine add <대상> --name <이름>은 ~/.hcm에 더함
cat >"$T/bin/hc" <<'EOF'
#!/bin/bash
f=$HOME/.hcm
case "$1 $2" in
  "machine ls") printf '{"machines":[%s]}\n' "$(paste -sd, "$f" 2>/dev/null)" ;;
  "machine add") echo "${HOME##*/} $*" >>"$T_LOG/hc-add.log"; printf '{"name":"%s","state":"connected"}\n' "$5" >>"$f" ;;
  *) exit 2 ;;
esac
EOF
chmod +x "$T/bin/"*

# 기계 하나: HOME, Tailscale 주소, 호스트 키(진짜 ed25519), 가짜 명령 링크. 맥 known_hosts에 그 호스트 키
host() { # host <이름> <ip4> <ip6>
  local d=$T/h/$1
  mkdir -p "$d/.local/bin" "$T/hk"
  for c in ssh ssh-keyscan tailscale hc; do ln -sf "$T/bin/$c" "$d/.local/bin/$c"; done
  echo "$2" >"$d/.ts4"; echo "$3" >"$d/.ts6"; echo "$1" >"$T/ip/$2"
  ssh-keygen -q -t ed25519 -N '' -C hostkey -f "$T/hk/$1" </dev/null
  printf '%s %s\n' "$2" "$(cut -d' ' -f1,2 "$T/hk/$1.pub")" >"$T/hk/$2"
  cat "$T/hk/$2" >>"$HOME/.ssh/known_hosts"
}
fresh() {
  rm -rf "$T/h" "$T/hk" "$T/ip" "$T_LOG"; mkdir -p "$T/h" "$T/ip" "$T_LOG"; : >"$HOME/.ssh/known_hosts"
  host wa 192.0.2.1 2001:db8::1; host wb 192.0.2.2 2001:db8::2; host wc 192.0.2.3 2001:db8::3
  host loopbox 192.0.2.8 2001:db8::8; host linbox 192.0.2.9 2001:db8::9
  printf 'worker.mesh  wa wb wc\n' >"$T/conf/config"
}

n=0 fails=0
t() { n=$((n + 1)); if eval "$2"; then echo "ok $n - $1"; else fails=$((fails + 1)); echo "FAIL $n - $1"; echo "  rc=$rc"; sed 's/^/  | /' "$T/out"; fi; }
has() { grep -Fq -- "$1" "$T/out"; }
mesh() { bash "$BIN/worker-mac-setup" mesh "$@" >"$T/out" 2>&1; rc=$?; }
pubbody() { awk '{print $2}' "$T/h/$1/.ssh/id_ed25519.pub"; }
snap() { (cd "$T/h" && find . -path '*/.ssh/*' -type f -exec cksum {} + | sort) >"$T/snap$1"; }

fresh
mesh --check
t "mesh --check: 아무것도 없으면 종료 1, 바꾸지 않음" '[ $rc = 1 ] && [ ! -e "$T/h/wa/.ssh/id_ed25519" ] && [ ! -e "$T/h/wb/.ssh/authorized_keys" ] && has "wa → wb"'

mesh
t "mesh: 성공" '[ $rc = 0 ] && has "6방향"'
t "mesh: 기계마다 키를 만들고 주석은 <기계>-workers-<날짜>" '[ -f "$T/h/wa/.ssh/id_ed25519" ] && grep -q " wa-workers-[0-9]\{8\}$" "$T/h/wa/.ssh/id_ed25519.pub"'
t "mesh: 받는 쪽 authorized_keys에 from=보낸 기계 Tailscale 주소" 'grep -Fq "from=\"192.0.2.1,2001:db8::1\",no-agent-forwarding,no-X11-forwarding ssh-ed25519 $(pubbody wa) " "$T/h/wb/.ssh/authorized_keys"'
t "mesh: ~/.ssh/config Host 블록(HostName Tailscale, User 그 기계 계정)" '[ "$(HOME=$T/h/wa ssh -G wc | tr "\n" " ")" = "hostname 192.0.2.3 user $(id -un) " ]'
t "mesh: known_hosts는 맥과 같은 지문" '[ "$(ssh-keygen -F 192.0.2.2 -f "$T/h/wa/.ssh/known_hosts" | grep -v "^#" | ssh-keygen -lf - | cut -d" " -f2)" = "$(ssh-keygen -lf "$T/hk/wb.pub" | cut -d" " -f2)" ]'
t "mesh: 워커마다 hc machine add <이름> --name <이름>" 'grep -Fxq "wa machine add wb --name wb" "$T_LOG/hc-add.log" && [ "$(wc -l <"$T_LOG/hc-add.log")" -eq 6 ]'
t "mesh: 6방향 접속 시험" '[ "$(sort -u "$T_LOG/conn.log" | wc -l)" -eq 6 ]'
t "mesh: 키 내용은 찍지 않음" '! grep -Fq "$(pubbody wa)" "$T/out" && ! grep -Fq "$(pubbody wc)" "$T/out"'
t "mesh: 제외 기계(루프·리눅스)는 건드리지 않음" '! grep -Eq "loopbox|linbox" "$T_LOG/ssh.log" "$T_LOG/hc-add.log" "$T_LOG/conn.log" && [ ! -d "$T/h/loopbox/.ssh" ] && [ ! -d "$T/h/linbox/.ssh" ] && ! grep -q loopbox "$T/h/wa/.ssh/config"'

snap 1
mesh
snap 2
t "mesh: 두 번째는 바꾸지 않음(멱등)" '[ $rc = 0 ] && cmp -s "$T/snap1" "$T/snap2" && [ "$(wc -l <"$T_LOG/hc-add.log")" -eq 6 ]'
mesh --check
t "mesh --check: 다 있으면 「모두 있음」 종료 0" '[ $rc = 0 ] && has "모두 있음 (6방향)"'

# 새 워커 wd를 더함: 처음 있던 파일은 .bak-mesh-<날짜>로 한 번 남기고, wd와 얽힌 방향만 더한다
host wd 192.0.2.4 2001:db8::4; printf 'worker.mesh  wa wb wc wd\n' >"$T/conf/config"
echo "# 손으로 둔 줄" >>"$T/h/wa/.ssh/config"; cp "$T/h/wa/.ssh/config" "$T/wa-config-before"
: >"$T_LOG/hc-add.log"
mesh wd
t "mesh <기계>: 그 기계와 얽힌 6방향만" '[ $rc = 0 ] && has "6방향" && [ "$(wc -l <"$T_LOG/hc-add.log")" -eq 6 ] && ! grep -Fq "wa machine add wb" "$T_LOG/hc-add.log"'
t "mesh: 고친 파일의 처음 판을 .bak-mesh-<날짜>로" 'ls "$T/h/wa/.ssh/" | grep -q "^config.bak-mesh-[0-9]\{8\}$" && ls "$T/h/wa/.ssh/" | grep -q "^authorized_keys.bak-mesh-"'
mesh --check
t "mesh --check: 넷이면 12방향 모두 있음" '[ $rc = 0 ] && has "모두 있음 (12방향)"'

# 지문 불일치: 맥이 아는 wb 호스트 키와 실제(ssh-keyscan)가 다르면 known_hosts에 넣지 않고 멈춘다
fresh
ssh-keygen -q -t ed25519 -N '' -f "$T/other" </dev/null
printf '192.0.2.2 %s\n' "$(cut -d' ' -f1,2 "$T/other.pub")" >"$T/hk/192.0.2.2"
mesh
t "mesh: 지문이 다르면 종료 4로 멈춤" '[ $rc = 4 ] && has "지문"'
t "mesh: 다른 지문은 known_hosts에 넣지 않음" '! grep -rqF "$(cut -d" " -f2 "$T/other.pub")" "$T/h"/*/.ssh/known_hosts 2>/dev/null'

# 맥 known_hosts에 그 기계가 없으면 대조할 수 없으니 바꾸기 전에 멈춘다
fresh
grep -v "^192.0.2.3 " "$HOME/.ssh/known_hosts" >"$T/kh" && mv "$T/kh" "$HOME/.ssh/known_hosts"
mesh
t "mesh: 맥 known_hosts에 없으면 종료 4, 아무것도 바꾸지 않음" '[ $rc = 4 ] && [ ! -e "$T/h/wa/.ssh/authorized_keys" ] && [ ! -e "$T/h/wa/.ssh/id_ed25519" ]'

# 설정·인자
rm -f "$T/conf/config"
mesh --check
t "mesh: worker.mesh 설정이 없으면 종료 2" '[ $rc = 2 ] && has "worker.mesh"'
fresh
mesh loopbox
t "mesh: 목록 밖 기계를 주면 종료 64, 접속 안 함" '[ $rc = 64 ] && [ ! -s "$T_LOG/ssh.log" ]'

echo "# ${n}개 중 실패 ${fails}"
[ $fails = 0 ]
