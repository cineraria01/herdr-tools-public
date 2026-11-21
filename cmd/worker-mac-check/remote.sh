# worker-mac-check 원격 부분: 그 기계에서 SSH 문맥으로 bash가 돈다(원격에는 Go가 없다). 셸 판 lib/worker-mac-check/worker-mac-check.sh의
# ver·REMOTE와 같은 내용. 앞에 설정 값(REPOS·TRUST·SLOTS_FILE·TOOLS)이 붙는다. 줄 꼴: R<TAB>상태<TAB>항목<TAB>세부 / V<TAB>도구<TAB>판
ver() { case $1 in go) go version | awk '{print $3}';; wails3) wails3 version 2>&1 | head -1;; git) git --version | awk '{print $3}';; *) "$1" --version | head -1;; esac 2>/dev/null; }
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
