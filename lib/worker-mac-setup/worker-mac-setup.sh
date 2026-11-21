#!/bin/bash
# worker-mac-setup — 새 워커 맥에 처음 한 번 돌리는 권한·문맥 셋업. 맥에서 실행한다.
# 2026-11-15 사용자 「관리권한들때문에 mac 에서 진행 안되는것들이 있을것 같은데 초기 설치하면 권한 지정하는 부분들 미리 되도록 처리」 → 「둘 다」.
# 사용: worker-mac-setup <기계> [--slots N] [--dry-run]
#       worker-mac-setup mesh [--check] [<기계>]   워커 연결만(아래 「워커 연결」)
#   <기계> = 맥 ~/.ssh/config 별칭(키 로그인이 되어 있어야 한다). hc 기계 이름도 같게 쓴다.
#   --slots N  검증 자리 파일(worker.slots_file) 값(파일이 없을 때만 쓴다, 기본 1). Mac Studio 급은 4.
#   --dry-run  바꾸지 않고 할 일만 찍는다(원격은 읽기만, 워커 연결은 --check).
# 워커 연결(2026-11-17 사용자 「워커 맥끼리 모두 연결 (추천)」): worker.mesh에 적은 워커끼리 서로 ssh·hc로 닿게 한다.
#   방향 A→B마다: A ~/.ssh/id_ed25519(없으면 만듦, 주석 <A>-workers-<날짜>) → B authorized_keys에
#   from="<A Tailscale IPv4>,<IPv6>",no-agent-forwarding,no-X11-forwarding → A known_hosts(ssh-keyscan ed25519 지문이
#   이 맥 known_hosts와 같을 때만) → A ~/.ssh/config Host B(HostName Tailscale, User B 계정) → A hc machine add B → 접속 시험.
#   있는 것은 건너뛰고, 고치는 파일은 그날 처음 판을 .bak-mesh-<날짜>로 남긴다. <기계>를 주면 그 기계와 얽힌 방향만.
#   셋업(<기계>가 worker.mesh에 있을 때)도 끝에 이 단계를 돈다. 종료: 0 모두 있음 · 1 없거나 실패 · 4 지문 대조 실패(멈춤).
#   되돌리기: 각 기계 ~/.ssh/{authorized_keys,known_hosts,config}.bak-mesh-<날짜>, hc machine rm <이름>, 만든 키.
# 설정(herdr-tools-config get, 경로는 그 기계 $HOME 기준):
#   worker.slots_file  검증 자리 파일 (없으면 그 단계 없음·체크리스트 앞에 「설정 없음」, --slots를 주면 종료 64)
#   worker.trust       Claude 폴더 신뢰를 넣을 경로들, 첫 경로는 폴더도 만든다 (없으면 Documents/app)
#   worker.ssh_key     SSH 안내에 쓸 맥 개인 키 (없으면 ~/.ssh/id_ed25519)
#   worker.mesh        서로 잇는 워커 hc 기계 이름들(맥 ~/.ssh/config 별칭과 같게). 루프·리눅스 기계·이 맥은 넣지 않는다
#                      (없으면 셋업의 워커 연결 단계를 건너뛰고, mesh는 종료 2)
#   설정 폴더의 worker-checklist.txt  끝 체크리스트에 붙일 개인 단계 (없으면 안 붙이고 「설정 없음」, 붙일 것이 없으면 빈 파일). 빈칸으로 시작하지 않는 줄이 새 번호,
#     빈칸으로 시작하는 줄은 이어지는 줄, # 줄은 주석, {M}은 기계 이름. 예: config.example/worker-checklist.txt
# 자동으로 하는 것(모두 되돌릴 수 있고 sudo가 필요 없다):
#   hc 기계 등록, 폴더, 검증 자리 파일, Claude 첫 실행 안내·폴더 신뢰(~/.claude.json, 백업),
#   herdr-core launchd 작업과 코어를 GUI(Aqua) 문맥으로 다시 띄우기(코어에 살아 있는 터미널이 0개일 때만),
#   Chrome 첫 실행(SSH에서 한 번), 관리자 단계 스크립트 사본, 문서 폴더 접근 질문 띄우기.
# 하지 않는 것: sudo, 비밀번호 받기·저장, 로그인 파일·풀 계정 파일 복사(그 기계에서 따로 로그인), 회전형 풀 설치,
#   TCC DB 수정·tccutil. 사람 몫은 끝에 순서 있는 체크리스트로 찍는다.
set -u
M=""; SLOTS=1; SS=0; DRY=0; MESH_ONLY=0; CHECK=0
[ "${1-}" = mesh ] && { MESH_ONLY=1; shift; }
while [ $# -gt 0 ]; do case $1 in --slots) SLOTS=${2-}; SS=1; shift 2 || shift;; --dry-run) DRY=1; shift;; --check) CHECK=1; shift;; -h|--help) sed -n '2,29p' "$0"; exit 0;; *) M=$1; shift;; esac; done
[ -n "$M" ] || [ $MESH_ONLY = 1 ] || { echo "사용: worker-mac-setup <기계> [--slots N] [--dry-run] | mesh [--check] [<기계>]" >&2; exit 64; }
# cfg <변수> <키> [<기본값>]: 키가 없으면 기본값, 기본값도 없으면 빈 값으로 두고 MISS에 키를 더한다. 설정 오류는 종료 2
MISS=""
cfg() {
  local v; v=$(herdr-tools-config get "$2" 2>/dev/null)
  case $? in
    0) ;;
    3) if [ $# -ge 3 ]; then v=$3; else v=""; MISS="$MISS $2"; fi ;;
    *) herdr-tools-config get "$2" >/dev/null; exit 2 ;;
  esac
  printf -v "$1" '%s' "$v"
}
D=$(date +%Y%m%d)
tmo() { local s=$1; shift; perl -e 'alarm shift; exec @ARGV or exit 127' "$s" "$@"; }
say() { printf '▸ %s\n' "$*"; }
cfg MESH worker.mesh
# ── 워커 연결 (머리말 「워커 연결」) ──
# 원격 스크립트는 base64로 넘기고, 빈칸이 든 값(공개 키 등)은 스크립트 앞에 %q로 붙인다. 공개 키·키 내용은 화면에 찍지 않는다.
rrun() { local m=$1 s=$2 b; shift 2; b=$(printf '%s' "$s" | base64 | tr -d '\n'); tmo 90 ssh -n -o BatchMode=yes -o ConnectTimeout=8 "$m" "bash -c \"\$(echo $b | base64 -D)\" _ $*"; }
MESH_HEAD='set -u; CHECK=$1; D=$2
export PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
nl() { [ -s "$1" ] && [ -n "$(tail -c1 "$1")" ] && echo >>"$1"; true; }
bak() { [ -f "$1" ] && [ ! -e "$1.bak-mesh-$D" ] && cp -p "$1" "$1.bak-mesh-$D"; true; }
fpof() { awk '\''!/^[[:space:]]*#/ && $2=="ssh-ed25519"'\'' | ssh-keygen -lf - 2>/dev/null | awk '\''{print $2}'\''; }
'
# 기계 하나: 키(--check가 아니면 없을 때 만듦), 계정, Tailscale 주소, 공개 키
MESH_INFO='k=$HOME/.ssh/id_ed25519
ts() { local t; for t in tailscale /Applications/Tailscale.app/Contents/MacOS/Tailscale; do command -v "$t" >/dev/null 2>&1 && { "$t" ip "$1" 2>/dev/null | head -1; return; }; done; }
if [ "$CHECK" = 0 ]; then
  mkdir -p "$HOME/.ssh" && chmod 700 "$HOME/.ssh"
  if [ -f "$k" ] && [ ! -f "$k.pub" ]; then ssh-keygen -y -f "$k" >"$k.pub" </dev/null
  elif [ ! -f "$k" ]; then ssh-keygen -q -t ed25519 -N "" -C "$NAME-workers-$D" -f "$k" </dev/null && echo new=1; fi
fi
echo "user=$(id -un)"; echo "ts4=$(ts -4)"; echo "ts6=$(ts -6)"
[ -f "$k.pub" ] && echo "pub=$(head -1 "$k.pub")"; true'
# 받는 쪽 B: 보낸 쪽 공개 키(PUB)를 from=FROM으로
MESH_AUTH='f=$HOME/.ssh/authorized_keys; body=$(printf "%s" "$PUB" | awk "{print \$2}")
if [ -z "$PUB" ]; then echo "키 없음"
elif [ -f "$f" ] && awk -v b="$body" '\''!/^[[:space:]]*#/ { for (i = 1; i <= NF; i++) if ($i == b) x = 1 } END { exit !x }'\'' "$f"; then echo 있음
elif [ "$CHECK" = 1 ]; then echo 없음
else mkdir -p "$HOME/.ssh" && chmod 700 "$HOME/.ssh"; bak "$f"; nl "$f"
  printf "from=\"%s\",no-agent-forwarding,no-X11-forwarding %s\n" "$FROM" "$PUB" >>"$f" && chmod 600 "$f" && echo 넣음; fi'
# 보내는 쪽 A: B(H4·BU·FP)로 가는 known_hosts·config·hc·접속. 지문이 FP와 다르면 종료 4
MESH_CLI='kf=$HOME/.ssh/known_hosts; cf=$HOME/.ssh/config
cur=$(ssh-keygen -F "$H4" -f "$kf" 2>/dev/null | fpof)
if [ -n "$cur" ]; then
  bad=$(printf "%s\n" "$cur" | grep -vxF -- "$FP" | head -1)
  [ -z "$bad" ] || { echo "kh=지문 다름(있는 줄 $bad)"; exit 4; }; echo kh=있음
elif [ "$CHECK" = 1 ]; then echo kh=없음
else line=$(ssh-keyscan -T 8 -t ed25519 "$H4" 2>/dev/null | awk '\''!/^#/ && $2=="ssh-ed25519"'\'' | head -1); got=$(printf "%s\n" "$line" | fpof)
  [ -n "$got" ] && [ "$got" = "$FP" ] || { echo "kh=지문 다름(ssh-keyscan ${got:-읽지 못함})"; exit 4; }
  mkdir -p "$HOME/.ssh" && chmod 700 "$HOME/.ssh"; bak "$kf"; nl "$kf"; printf "%s\n" "$line" >>"$kf"; echo kh=넣음
fi
g() { ssh -G "$B" 2>/dev/null | awk -v k="$1" '\''$1 == k { print $2; exit }'\''; }
gh=$(g hostname); gu=$(g user)
if [ "$gh" = "$H4" ] && [ "$gu" = "$BU" ]; then echo cfg=있음
elif [ "$gh" != "$B" ]; then echo "cfg=다름(HostName $gh, User $gu — 손으로 본다)"
elif [ "$CHECK" = 1 ]; then echo cfg=없음
else bak "$cf"; nl "$cf"; printf "\nHost %s\n    HostName %s\n    User %s\n" "$B" "$H4" "$BU" >>"$cf" && chmod 600 "$cf" && echo cfg=넣음; fi
if hc machine ls 2>/dev/null | grep -q "\"name\":\"$B\""; then echo hc=있음
elif [ "$CHECK" = 1 ]; then echo hc=없음
elif o=$(perl -e "alarm 60; exec @ARGV" hc machine add "$B" --name "$B" 2>&1 </dev/null); then echo hc=넣음
else echo "hc=실패($(printf "%s" "$o" | tail -1))"; fi
ssh -n -o BatchMode=yes -o ConnectTimeout=8 "$B" true </dev/null >/dev/null 2>&1 && echo conn=됨 || echo "conn=안 됨"'

# mesh_run <기계|빈 값>: 빈 값이면 모든 짝. CHECK=1이면 읽기만
mesh_run() {
  local f=$1 w i j a b n=0 bad=0 add=0 out r line rc
  local -a W U V4 V6 PUB FP OK
  set -f; W=($MESH); set +f
  if [ -n "$f" ]; then case " ${W[*]} " in *" $f "*) ;; *) echo "✗ $f 은(는) worker.mesh(${W[*]})에 없다" >&2; return 64;; esac; fi
  say "워커 연결$([ "$CHECK" = 1 ] && echo ' (--check: 읽기만)'): ${W[*]}${f:+ — $f 와 얽힌 방향만}"
  # 1) 기계마다 정보(읽기만)와 이 맥 known_hosts의 ed25519 지문. 지문을 못 대조하면 아무것도 바꾸기 전에 멈춘다
  for i in "${!W[@]}"; do
    w=${W[$i]}; OK[$i]=0
    out=$(rrun "$w" "$(printf 'NAME=%q;' "$w")$MESH_HEAD$MESH_INFO" 1 "$D") || { echo "✗ $w: 접속 안 됨"; bad=$((bad+1)); continue; }
    U[$i]=$(sed -n 's/^user=//p' <<<"$out"); V4[$i]=$(sed -n 's/^ts4=//p' <<<"$out"); V6[$i]=$(sed -n 's/^ts6=//p' <<<"$out"); PUB[$i]=$(sed -n 's/^pub=//p' <<<"$out")
    [ -n "${V4[$i]}" ] || { echo "✗ $w: Tailscale 주소를 못 읽음"; bad=$((bad+1)); continue; }
    FP[$i]=$(ssh-keygen -F "${V4[$i]}" -f "$HOME/.ssh/known_hosts" 2>/dev/null | awk '!/^[[:space:]]*#/ && $2=="ssh-ed25519"' | ssh-keygen -lf - 2>/dev/null | awk '{print $2}' | head -1)
    [ -n "${FP[$i]}" ] || { echo "✗ $w: 이 맥 known_hosts에 ${V4[$i]} ed25519가 없어 지문을 대조할 수 없다 — 멈춤 (ssh $w 로 한 번 붙어 맥 known_hosts에 넣은 뒤 다시)"; return 4; }
    OK[$i]=1
  done
  # 키가 없는 기계는 여기서 만든다(--check가 아니고 그 기계가 얽힌 방향이 있을 때)
  [ "$CHECK" = 1 ] || for i in "${!W[@]}"; do
    w=${W[$i]}; [ "${OK[$i]}" = 1 ] && [ -z "${PUB[$i]}" ] || continue
    out=$(rrun "$w" "$(printf 'NAME=%q;' "$w")$MESH_HEAD$MESH_INFO" 0 "$D") && PUB[$i]=$(sed -n 's/^pub=//p' <<<"$out")
    grep -q '^new=1' <<<"$out" && say "$w: 키 만듦 ~/.ssh/id_ed25519 ($w-workers-$D)"
  done
  # 2) 방향마다: 받는 쪽 authorized_keys → 보내는 쪽 known_hosts·config·hc·접속
  for i in "${!W[@]}"; do for j in "${!W[@]}"; do
    [ "$i" = "$j" ] && continue; a=${W[$i]}; b=${W[$j]}
    [ -z "$f" ] || [ "$a" = "$f" ] || [ "$b" = "$f" ] || continue
    n=$((n+1))
    [ "${OK[$i]}" = 1 ] && [ "${OK[$j]}" = 1 ] || { echo "✗ $a → $b: 위 기계 오류로 건너뜀"; bad=$((bad+1)); continue; }
    r=$(rrun "$b" "$(printf 'PUB=%q FROM=%q;' "${PUB[$i]}" "${V4[$i]}${V6[$i]:+,${V6[$i]}}")$MESH_HEAD$MESH_AUTH" "$CHECK" "$D") || r="실패"
    out=$(rrun "$a" "$(printf 'B=%q H4=%q BU=%q FP=%q;' "$b" "${V4[$j]}" "${U[$j]}" "${FP[$j]}")$MESH_HEAD$MESH_CLI" "$CHECK" "$D"); rc=$?
    line="$a → $b: authorized_keys $r · known_hosts $(sed -n 's/^kh=//p' <<<"$out") · config $(sed -n 's/^cfg=//p' <<<"$out") · hc $(sed -n 's/^hc=//p' <<<"$out") · 접속 $(sed -n 's/^conn=//p' <<<"$out")"
    if [ $rc = 4 ]; then echo "✗ $line"; echo "✗ $a 가 본 $b 호스트 키 지문이 이 맥 known_hosts(${FP[$j]})와 다르다 — 멈춤. 넣지 않았다."; return 4; fi
    add=$((add + $(grep -o '넣음' <<<"$line $r" | wc -l)))
    if [ $rc = 0 ] && [ "$r" != 실패 ] && ! grep -Eq '(없음|다름|실패|안 됨)' <<<"$line" && grep -q '접속 됨' <<<"$line"; then echo "  $line"
    else echo "✗ $line"; bad=$((bad+1)); fi
  done; done
  if [ $bad = 0 ]; then
    if [ "$add" = 0 ]; then say "워커 연결: 모두 있음 (${n}방향)"; else say "워커 연결: 끝 (${n}방향, 새로 넣은 것 $add)"; fi
    return 0
  fi
  say "워커 연결: 문제 $bad (${n}방향 중, ✗ 줄)"; return 1
}
if [ $MESH_ONLY = 1 ]; then
  [ -n "$MESH" ] || { echo "설정 worker.mesh가 없다(서로 잇는 워커 이름들, 예: config.example/config)" >&2; exit 2; }
  mesh_run "$M"; exit $?
fi
cfg SLOTS_FILE worker.slots_file; cfg TRUST worker.trust 'Documents/app'; cfg KEY worker.ssh_key '~/.ssh/id_ed25519'
CL=$(herdr-tools-config file worker-checklist.txt 2>/dev/null) || MISS="$MISS worker-checklist.txt"
if [ $SS = 1 ] && [ -z "$SLOTS_FILE" ]; then echo "--slots를 쓰려면 설정 worker.slots_file이 있어야 한다" >&2; exit 64; fi
case "$SLOTS" in ''|*[!0-9]*|0) echo "--slots는 1 이상 숫자" >&2; exit 64;; esac

if ! tmo 15 ssh -n -o BatchMode=yes -o ConnectTimeout=8 "$M" true 2>/dev/null; then
  cat <<EOF
✗ ssh $M 키 로그인이 안 된다. 먼저 사람이 할 일:
  1. 그 맥: 시스템 설정 → 일반 → 공유 → 원격 로그인 켜기, (i)에서 「원격 사용자에 대한 전체 디스크 접근 허용」 켜기.
  2. 그 맥: Tailscale 로그인, 관리 화면에서 키 만료 끄기.
  3. 이 맥 ~/.ssh/config에 Host $M 블록(HostName=Tailscale 주소, User=그 맥 계정, IdentityFile $KEY)을 넣고
     ! ssh-copy-id -i $KEY.pub $M     (그 맥 계정 암호는 사용자가 직접 친다)
  4. 다시: worker-mac-setup $M
EOF
  exit 2
fi

U=$(ssh -n -o BatchMode=yes "$M" 'id -un'); H=$(ssh -n -o BatchMode=yes "$M" 'echo $HOME')
say "대상 $M (계정 $U, 홈 $H)$([ $DRY = 1 ] && echo ' — dry-run')"

# 1. hc 기계 등록(코어 설치). 설치 직후 코어는 SSH 문맥이라 아래 2에서 launchd로 옮긴다.
if hc machine ls 2>/dev/null | grep -q "\"name\":\"$M\""; then say "hc 기계 $M: 이미 있음"
elif [ $DRY = 1 ]; then say "(dry-run) hc machine add $M --install --name $M"
else say "hc 기계 등록"; hc machine add "$M" --install --name "$M" || { echo "✗ hc machine add 실패" >&2; exit 1; }; fi

# 2. 원격 자동 단계(SSH 문맥). 결과 줄만 찍는다. 앞에 설정 값(TRUST·SLOTS_FILE)이 붙는다.
read -r -d '' REMOTE <<'EOS'
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
EOS
PRE=$(printf 'TRUST=%q SLOTS_FILE=%q' "$TRUST" "$SLOTS_FILE")
b64=$(printf '%s\n%s' "$PRE" "$REMOTE" | base64 | tr -d '\n')
tmo 180 ssh -n -o BatchMode=yes "$M" "bash -c \"\$(echo $b64 | base64 -D)\" _ $DRY $SLOTS $D"; rrc=$?
[ $rrc -eq 3 ] && exit 3

# 3. 문서 폴더 접근 질문을 지금 띄운다(그 맥 화면에서 허용). 허용 전엔 hc 터미널의 ~/Documents 작업이 멈춘다.
if [ $DRY = 0 ]; then
  sleep 3; r=$(tmo 12 hc exec --on "$M" -- /bin/sh -c 'ls "$HOME/Documents" >/dev/null 2>&1 && echo ok || echo no' 2>/dev/null)
  case "$r" in ok) say "문서 폴더(hc 문맥): 이미 허용";; no) say "문서 폴더(hc 문맥): 거부됨 — 아래 체크리스트 5";; *) say "문서 폴더(hc 문맥): 응답 없음 — 그 맥 화면에 「herdr-core … 문서 폴더」 질문이 떠 있다. 허용한다(체크리스트 5)";; esac
fi

# 4. 워커 연결: 이 기계가 worker.mesh에 있으면 다른 워커들과 서로 잇는다(dry-run은 --check)
if [ -n "$MESH" ]; then
  case " $MESH " in
    *" $M "*) CHECK=$DRY; mesh_run "$M"; mrc=$?; [ $mrc = 4 ] && exit 4 ;;
    *) say "워커 연결: $M 은(는) worker.mesh에 없어 건너뜀 (워커면 개인 설정 worker.mesh에 더하고 worker-mac-setup mesh $M)" ;;
  esac
fi

echo
for k in $MISS; do
  case $k in worker.slots_file) w="검증 자리 파일 단계를 건너뜀";; worker-checklist.txt) w="개인 단계를 붙이지 않음";; worker.mesh) w="워커 연결 단계를 건너뜀";; *) w="";; esac
  echo "※ 모름 — 설정 없음($k): $w (${HERDR_TOOLS_CONFIG:-$HOME/.config/herdr-tools}, 예: config.example/)"
done
cat <<EOF
━━ 사람 몫 (순서대로, 그 맥 화면 또는 이 맥에서 \`!\`로) ━━
 1. [그 맥] 시스템 설정 → 일반 → 공유 → 원격 로그인 → (i) → 「원격 사용자에 대한 전체 디스크 접근 허용」 켜기
    (worker-mac-check가 TCC 기록을 읽고, SSH 문맥 작업이 폴더 질문에 안 막힌다)
 2. [그 맥] 시스템 설정 → 개인정보 보호 및 보안 → FileVault → 끄기 (복호화 끝날 때까지 기다림. 켜져 있으면 자동 로그인 메뉴가 없다)
 3. [그 맥] 시스템 설정 → 사용자 및 그룹 → 자동으로 로그인 → $U
 4. [이 맥] 관리자 단계(암호는 그 맥 계정 것, 사용자가 직접 친다):
    ! ssh -t $M 'sudo /bin/bash ~/.local/state/worker-mac-setup/root-setup.sh'     → ROOT_SETUP_DONE
 5. [그 맥] 「herdr-core이(가) 문서 폴더의 파일에 접근하려고 합니다」 → 허용.
    닫혔으면 시스템 설정 → 개인정보 보호 및 보안 → 파일 및 폴더 → herdr-core → 문서 폴더 켜기
 6. [이 맥] gh 로그인(그 맥에서 따로, 토큰 복사 안 함):
    ! ssh -t $M 'zsh -lic "gh auth login --hostname github.com --git-protocol https --web && gh auth setup-git"'
 7. [이 맥] Claude 로그인(그 맥에서 따로): ! ssh -t $M 'zsh -lic "claude auth login"'
EOF
n=8
# 개인 단계(설정 폴더의 worker-checklist.txt)를 8번부터 붙인다
if [ -n "$CL" ]; then
  while IFS= read -r l || [ -n "$l" ]; do
    l=${l%$'\r'}; l=${l//\{M\}/$M}
    case $l in ''|'#'*) ;; [[:space:]]*) printf '%s\n' "$l";; *) printf '%2d. %s\n' $n "$l"; n=$((n+1));; esac
  done < "$CL"
fi
printf '%2d. %s\n' $n "[그 맥] (Xcode 앱을 깐 경우만) 4의 root-setup이 라이선스를 동의한다. 깐 뒤 4를 다시 돌린다."; n=$((n+1))
printf '%2d. %s\n' $n "[이 맥] 점검: worker-mac-check $M  — 실패 0이면 끝. 코어 문맥이 실패면 그 맥 로그인 뒤 worker-mac-setup $M 을 다시 돌린다."
echo " 가방에 넣을 땐: ! ssh -t $M 'sudo pmset -a disablesleep 0'"
exit 0
