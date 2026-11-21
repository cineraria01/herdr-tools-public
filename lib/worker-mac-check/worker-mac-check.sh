#!/bin/bash
# worker-mac-check — 워커 맥 하나를 맥에서 SSH·hc로 점검한다. 읽기 전용(아무것도 고치지 않는다).
# 2026-11-15 사용자 「관리권한들때문에 mac 에서 진행 안되는것들이 있을것 같은데 초기 설치하면 권한 지정하는 부분들 미리 되도록 처리」 → 「둘 다」.
# 사용: worker-mac-check <기계> [<기계>...]   (기계 = ssh 별칭이자 hc 기계 이름. 예: w1 w2)
# 줄 꼴: <통과|실패|모름>  <항목>  <세부>. 기계마다 끝에 개수 한 줄. 종료 0 = 실패 없음, 1 = 실패 있음, 2 = 연결 안 됨·설정 오류.
# 설정(herdr-tools-config get, 경로는 그 기계 $HOME 기준, 목록은 빈칸으로):
#   worker.repo        저장소 후보, 처음 있는 것을 본다. Claude 신뢰도 그 저장소(없으면 첫 후보)와 부모로 본다
#   worker.trust       worker.repo가 없을 때 Claude 신뢰를 볼 경로, 모두 신뢰여야 통과 (없으면 Documents/app)
#   worker.slots_file  검증 자리 파일
#   worker.tools       맥과 판을 견줄 도구 (없으면 git node)
#   worker.proxy       hc 문맥에서 「<값> status」의 usable now를 볼 명령
#   repo·slots_file·proxy 키가 없으면(설정 파일이 없을 때도) 그 항목은 「모름 … 설정 없음(<키>)」. 값 없이 키만 두면 일부러 끈 것이라 줄을 안 찍는다.
# 비밀은 찍지 않는다: 키체인 값·토큰·전체 argv를 읽거나 출력하지 않는다(gh는 계정 이름, 프록시는 상태 줄만).
# TCC는 시스템 TCC.db를 sqlite3 -readonly로 조회만 한다(원격 로그인에 전체 디스크 접근이 없으면 「모름」).
set -u
BREW_DIR=${WORKER_MAC_BREW_DIR:-/opt/homebrew/bin}  # 시험이 바꾼다
TOOLPATH='$HOME/.local/bin:$HOME/go/bin:$HOME/.bun/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin'

# 도구 판 읽기(원격·맥 같은 함수). go·wails3·git은 정한 꼴, 그 밖은 --version 첫 줄.
ver() { case $1 in go) go version | awk '{print $3}';; wails3) wails3 version 2>&1 | head -1;; git) git --version | awk '{print $3}';; *) "$1" --version | head -1;; esac 2>/dev/null; }

# 원격(SSH 문맥)에서 도는 부분. 앞에 설정 값(REPOS·TRUST·SLOTS_FILE·TOOLS)과 ver가 붙는다.
# 줄 꼴: R<TAB>상태<TAB>항목<TAB>세부 / V<TAB>도구<TAB>판
read -r -d '' REMOTE <<'EOS'
export PATH="$HOME/.local/bin:$HOME/go/bin:$HOME/.bun/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin"
r() { printf 'R\t%s\t%s\t%s\n' "$1" "$2" "$3"; }
u=$(id -un); uid=$(id -u)

a=$(defaults read /Library/Preferences/com.apple.loginwindow autoLoginUser 2>/dev/null)
if [ "$a" = "$u" ]; then r 통과 "자동 로그인" "$a"; elif [ -n "$a" ]; then r 실패 "자동 로그인" "다른 계정 $a"; else r 실패 "자동 로그인" "꺼짐 (재부팅하면 코어가 안 뜬다)"; fi

pm=$(pmset -g 2>/dev/null); sd=$(echo "$pm" | awk '/SleepDisabled/{print $2}'); sl=$(echo "$pm" | awk '$1=="sleep"{print $2}')
if [ "$sd" = 1 ]; then r 통과 "잠자기 끔" "SleepDisabled 1 (덮개 닫아도 안 잠듦)"
elif [ "$sl" = 0 ]; then r 통과 "잠자기 끔" "sleep 0 (덮개를 닫으면 잠듦)"
else r 실패 "잠자기 끔" "SleepDisabled ${sd:-?}, sleep ${sl:-?}"; fi

fv=$(fdesetup status 2>/dev/null | head -1)
case "$fv" in *"is Off"*) r 통과 "FileVault" "꺼짐 (자동 로그인 워커 기준)";; *"is On"*) r 실패 "FileVault" "켜짐 — 자동 로그인이 안 돼 재부팅 뒤 코어가 안 뜬다";; *) r 모름 "FileVault" "${fv:-읽지 못함}";; esac

sp=$(pgrep -f '(herdr-core|/hc) (serve|--hc-launchd)' | head -1)
if [ -z "$sp" ]; then r 실패 "코어 문맥(Aqua)" "herdr-core serve 없음"
else
  ci=$(launchctl print "pid/$sp" 2>/dev/null); ca=$(echo "$ci" | awk '/asid =/{print $3; exit}')
  co=$(echo "$ci" | awk '/name = /{print $3; exit}')
  dp=$(pgrep -x Dock | head -1); da=""
  [ -n "$dp" ] && da=$(launchctl print "pid/$dp" 2>/dev/null | awk '/asid =/{print $3; exit}')
  st=$(ps -o lstart= -p "$sp" | sed 's/^ *//')
  if [ -z "$dp" ]; then r 실패 "코어 문맥(Aqua)" "GUI 로그인 세션 없음(Dock 없음)"
  elif [ -n "$ca" ] && [ "$ca" = "$da" ]; then r 통과 "코어 문맥(Aqua)" "serve asid $ca = GUI 세션, 시작 $st"
  elif [ "${co#dev.herdr.core.}" != "$co" ]; then
    # hc 0.1.21 --hc-launchd: launchd(gui 도메인)가 따로 띄운 작업이라 asid가 Dock과 달라도 키체인이 열린다(2026-11-16 워커 맥 실측).
    r 통과 "코어 문맥(Aqua)" "launchd 작업 $co (gui 도메인), 시작 $st"
  else
    case "$co" in com.openssh.sshd*) why="SSH 접속에서 떴다";; *) why="묶음 ${co:-?}";; esac
    r 실패 "코어 문맥(Aqua)" "serve asid ${ca:-?} ≠ GUI ${da:-?} — $why, 시작 $st"
  fi
fi
lab=$(launchctl print "gui/$uid" 2>/dev/null | awk '{print $NF}' | grep -E 'herdr-core$' | head -1)
if [ -n "$lab" ]; then r 통과 "코어 launchd 작업" "gui/$uid/$lab 실려 있음"; else r 실패 "코어 launchd 작업" "gui/$uid 에 *.herdr-core 없음"; fi

# 목록은 빈칸으로만 나눈다(*·? 펼치지 않음)
set -f; cands=($REPOS); tlist=($TRUST); tools=($TOOLS); set +f
repo=""
if [ ${#cands[@]} -gt 0 ]; then
  for d in "${cands[@]}"; do [ -d "$HOME/$d/.git" ] && { repo=$HOME/$d; break; }; done
  if [ -n "$repo" ]; then
    o=$(git -C "$repo" remote get-url origin 2>/dev/null | sed -E 's#//[^/@]*@#//#'); b=$(git -C "$repo" rev-parse --abbrev-ref HEAD 2>/dev/null)
    r 통과 "저장소" "$repo ($b, $o)"
  else r 실패 "저장소" "~/${cands[0]} 없음"; fi
fi
# Claude 첫 실행 안내·폴더 신뢰 (~/.claude.json 키만 읽음). 경로는 링크를 푼 실제 경로로 견준다.
# 저장소 후보가 있으면 찾은 저장소(없으면 첫 후보)와 그 부모 가운데 하나가 신뢰면 통과 — 워크트리는 원본 저장소의 신뢰를 따른다.
# 후보가 없으면 worker.trust 경로가 모두 신뢰여야 통과.
rpath() { (cd -P "$1" 2>/dev/null && pwd -P) || { d=$(cd -P "${1%/*}" 2>/dev/null && pwd -P) && echo "$d/${1##*/}"; } || echo "$1"; }
tr_() { jq -r --arg a "$1" '.projects[$a].hasTrustDialogAccepted//false' "$HOME/.claude.json" 2>/dev/null; }
ob=$(jq -r '.hasCompletedOnboarding//false' "$HOME/.claude.json" 2>/dev/null)
if [ -z "$ob" ]; then r 모름 "Claude 첫 실행·신뢰" "~/.claude.json 못 읽음"
else
  if [ ${#cands[@]} -gt 0 ]; then
    rp=$(rpath "${repo:-$HOME/${cands[0]}}"); ta=$(tr_ "${rp%/*}"); ts=$(tr_ "$rp")
    tl=" ${rp%/*}=$ta ${rp##*/}=$ts"; ok=0
    if [ "$ta" = true ] || [ "$ts" = true ]; then ok=1; fi
  else
    tl=""; ok=1; [ ${#tlist[@]} -gt 0 ] || ok=0
    for p in ${tlist[@]+"${tlist[@]}"}; do v=$(tr_ "$(rpath "$HOME/$p")"); tl="$tl $p=$v"; [ "$v" = true ] || ok=0; done
  fi
  if [ "$ob" = true ] && [ $ok = 1 ]; then r 통과 "Claude 첫 실행·신뢰" "안내 끝, 신뢰$tl"
  else r 실패 "Claude 첫 실행·신뢰" "안내 끝=$ob, 신뢰$tl (대화형 첫 실행이 「No, exit」에서 멈춘다)"; fi
fi
if [ -n "$SLOTS_FILE" ]; then
  vs="$HOME/$SLOTS_FILE"; vn=${SLOTS_FILE##*/}
  if [ -f "$vs" ]; then v=$(tr -d ' \n' < "$vs"); case "$v" in ''|*[!0-9]*) r 실패 "$vn" "숫자 아님";; *) r 통과 "$vn" "$v";; esac
  else r 실패 "$vn" "파일 없음 (자리 1로 돈다)"; fi
fi

cp=$(xcode-select -p 2>/dev/null); clt=$(pkgutil --pkg-info=com.apple.pkg.CLTools_Executables 2>/dev/null | awk '/^version/{print $2}')
if [ -n "$cp" ] && [ -x "$cp/usr/bin/git" -o -x /usr/bin/clang ]; then r 통과 "명령줄 도구" "$cp ${clt:+(CLT $clt)}"; else r 실패 "명령줄 도구" "없음 (xcode-select --install)"; fi
if ls -d /Applications/Xcode*.app >/dev/null 2>&1; then
  if xcodebuild -license check >/dev/null 2>&1; then r 통과 "Xcode 라이선스" "동의됨"; else r 실패 "Xcode 라이선스" "동의 전 (sudo xcodebuild -license accept)"; fi
else r 통과 "Xcode 라이선스" "해당 없음: Xcode 앱 없음 (iOS 교차 vet은 건너뜀)"; fi

for t in ${tools[@]+"${tools[@]}"}; do v=$(ver "$t"); printf 'V\t%s\t%s\n' "$t" "${v:-없음}"; done

sl=$(sudo -n -l 2>&1)
if echo "$sl" | grep -q 'password is required'; then r 통과 "sudo 비밀번호 없이" "없음 (모든 sudo에 비밀번호)"
else np=$(echo "$sl" | grep NOPASSWD | sed 's/^ *//' | tr '\n' ';')
  if echo "$np" | grep -q 'NOPASSWD: ALL'; then r 실패 "sudo 비밀번호 없이" "모든 명령: $np"; else r 통과 "sudo 비밀번호 없이" "${np:-없음}"; fi; fi

db="/Library/Application Support/com.apple.TCC/TCC.db"
q=$(sqlite3 -readonly "$db" "select service||'|'||client||'|'||auth_value from access where service in ('kTCCServiceSystemPolicyAllFiles','kTCCServiceAccessibility','kTCCServiceScreenCapture')" 2>/dev/null); rc=$?
if [ $rc -ne 0 ]; then
  for s in "전체 디스크 접근" "손쉬운 사용" "화면 기록"; do r 모름 "TCC $s" "시스템 TCC.db를 못 읽음 (원격 로그인에 전체 디스크 접근 없음)"; done
else
  for pair in "kTCCServiceSystemPolicyAllFiles:전체 디스크 접근" "kTCCServiceAccessibility:손쉬운 사용" "kTCCServiceScreenCapture:화면 기록"; do
    sv=${pair%%:*}; nm=${pair#*:}; out=""
    for c in Terminal herdr node sshd-keygen-wrapper; do
      v=$(echo "$q" | awk -F'|' -v s="$sv" -v c="$c" '$1==s && index($2,c){print $3; exit}')
      case "$v" in 2) v=허용;; 0) v=거부;; 3) v=제한;; '') v=없음;; esac; out="$out $c=$v"
    done
    r 통과 "TCC $nm" "기록만:${out} (워커 맥 필수 아님)"
  done
fi
printf 'E\t끝\n' # 끝 표식 — 맥 쪽은 이 줄이 없으면 점검이 끊긴 것으로 본다(E01)
EOS

# 맥(이 기계)의 판
mac_ver() { PATH="$HOME/.local/bin:$HOME/go/bin:$HOME/.bun/bin:/opt/homebrew/bin:$PATH" ver "$1"; }
# 상한 걸고 실행(질문 창 대기·코어 멈춤에 걸려도 점검이 끝나게). perl alarm은 exec 뒤에도 남는다.
tmo() { local s=$1; shift; perl -e 'alarm shift; exec @ARGV or exit 127' "$s" "$@"; }
hcx() { tmo "$1" hc exec --on "$M" -- /bin/sh -c "export PATH=\"$TOOLPATH\"; $2" 2>/dev/null; }

check_one() {
  M=$1; P=0; F=0; U=0
  line() { printf '%s\t%s\t%s\n' "$1" "$2" "$3"; case $1 in 통과) P=$((P+1));; 실패) F=$((F+1));; *) U=$((U+1));; esac; }
  echo "== $M ($(date '+%F %T'))"
  if ! tmo 15 ssh -n -o BatchMode=yes -o ConnectTimeout=8 "$M" true 2>/dev/null; then
    line 실패 "SSH 연결" "ssh $M 안 됨 (Tailscale·원격 로그인·키 확인)"; echo "-- $M: 통과 $P · 실패 $F · 모름 $U"; return 2
  fi
  line 통과 "SSH 연결" "키 로그인"
  for k in $MISS; do
    case $k in worker.repo) i="저장소";; worker.slots_file) i="검증 자리 파일";; worker.proxy) i="프록시 status";; *) i=$k;; esac
    line 모름 "$i" "설정 없음($k) — ${HERDR_TOOLS_CONFIG:-$HOME/.config/herdr-tools}/config, 예: config.example/config"
  done
  b64=$(printf '%s\n%s' "$PRE" "$REMOTE" | base64 | tr -d '\n')
  out=$(tmo 90 ssh -n -o BatchMode=yes "$M" "bash -c \"\$(echo $b64 | base64 -D)\"" 2>/dev/null); orc=$?
  while IFS=$'\t' read -r k a b c; do
    case $k in
      R) line "$a" "$b" "$c";;
      V) m=$(mac_ver "$a"); if [ "$b" = "$m" ]; then line 통과 "판 $a" "$b (맥과 같음)"; else line 실패 "판 $a" "$b (맥 ${m:-없음})"; fi;;
    esac
  done <<< "$out"
  # 받은 줄은 두고, 다 받지 못한 점검은 통과로 세지 않는다(E01 — Go 판 cut과 같은 말)
  if [ $orc = 142 ]; then why="시간 초과(90초)"; elif [ $orc != 0 ]; then why="끊김(종료 $orc)"; elif ! grep -qx $'E\t끝' <<<"$out"; then why="끝 표식 없음"; else why=""; fi
  [ -z "$why" ] || line 실패 "원격 점검" "$why — 받은 줄까지만 봤다"

  # hc 터미널 문맥(코어가 띄운 프로그램) 점검
  if ! hc machine ls 2>/dev/null | grep -q "\"name\":\"$M\",[^}]*\"state\":\"connected\""; then
    for i in "키체인(hc 문맥)" "brew PATH(hc 문맥)" "문서 폴더(hc 문맥)" "gh 로그인" ${PX:+"$PX status"}; do line 모름 "$i" "hc 기계 $M 연결 안 됨"; done
  else
    kc=$(hcx 20 'security show-keychain-info "$HOME/Library/Keychains/login.keychain-db" >/dev/null 2>&1; echo $?')
    case "$kc" in 0) line 통과 "키체인(hc 문맥)" "열림";; 36) line 실패 "키체인(hc 문맥)" "User interaction is not allowed (코어가 SSH 문맥)";; *) line 모름 "키체인(hc 문맥)" "rc ${kc:-시간 초과}";; esac
    # hcx는 PATH를 덮으니 여기는 코어가 준 PATH 그대로 본다
    hp=$(tmo 15 hc exec --on "$M" -- /bin/sh -c "[ -d '$BREW_DIR' ] || { echo -; exit 0; }; echo \"\$PATH\"" 2>/dev/null)
    case ":$hp:" in
      :-:) line 통과 "brew PATH(hc 문맥)" "해당 없음: $BREW_DIR 없음";;
      ::) line 모름 "brew PATH(hc 문맥)" "15초 안 응답";;
      *":$BREW_DIR:"*) line 통과 "brew PATH(hc 문맥)" "$BREW_DIR 있음";;
      *) line 실패 "brew PATH(hc 문맥)" "$BREW_DIR 없음 — 코어가 로그인 환경 없이 떴다 (hc-core-relaunch $M)";;
    esac
    dc=$(hcx 15 'ls "$HOME/Documents" >/dev/null 2>&1 && echo ok || echo no')
    case "$dc" in ok) line 통과 "문서 폴더(hc 문맥)" "~/Documents 읽힘";; no) line 실패 "문서 폴더(hc 문맥)" "읽기 거부 (TCC 파일 및 폴더 → herdr-core → 문서 폴더)";; *) line 실패 "문서 폴더(hc 문맥)" "15초 안 응답 — 그 맥 화면에 폴더 접근 질문이 떠 있을 수 있다";; esac
    gh=$(hcx 20 'gh auth status --hostname github.com 2>&1 | grep -m1 -E "Logged in to github.com|not logged|token .* invalid"')
    acct=$(echo "$gh" | sed -nE 's/.*account ([^ ]+).*/\1/p')
    if [ -n "$acct" ]; then line 통과 "gh 로그인" "$acct"; elif [ -n "$gh" ]; then line 실패 "gh 로그인" "로그인 안 됨"; else line 실패 "gh 로그인" "gh 없음 또는 응답 없음"; fi
    if [ -n "$PX" ]; then
      tc=$(hcx 30 "$(printf %q "$PX")"' status 2>&1 | grep -m2 -i -E "running|stopped|not running|usable now"' | tr '\n' ' ' | tr -s ' ')
      if echo "$tc" | grep -qi 'usable now: *[1-9]'; then line 통과 "$PX status" "$tc"; elif [ -n "$tc" ]; then line 실패 "$PX status" "$tc"; else line 실패 "$PX status" "응답 없음 (프록시·계정 없음?)"; fi
    fi
  fi
  echo "-- $M: 통과 $P · 실패 $F · 모름 $U"
  [ $F -eq 0 ]
}

[ $# -ge 1 ] && ! printf '%s\n' "$@" | grep -q '^-' || { echo "사용: worker-mac-check <기계> [<기계>...]" >&2; exit 64; }  # -로 시작하는 이름은 ssh 옵션(E10)
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
cfg REPOS worker.repo; cfg TRUST worker.trust 'Documents/app'; cfg SLOTS_FILE worker.slots_file
cfg TOOLS worker.tools 'git node'; cfg PX worker.proxy
PRE=$(printf 'REPOS=%q TRUST=%q SLOTS_FILE=%q TOOLS=%q\n' "$REPOS" "$TRUST" "$SLOTS_FILE" "$TOOLS"; declare -f ver)
rc=0
for m in "$@"; do check_one "$m"; r=$?; [ $r -gt $rc ] && rc=$r; echo; done
exit $rc
