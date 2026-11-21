# hc-fill 함수: tick (bin/hc-fill가 읽는다. 함수는 bin/hc-fill 안 원래 정의 순서 그대로)

# 끝난 레인 정리: 설정 gc 분(기본 60, 0이면 끔)마다 한 번, repo 줄의 기계·저장소마다 hc-lane gc. 원격 중간 참조(--integ)는
# 프로젝트마다 local 기계의 저장소에서 한 번. 정리는 바퀴 밖 배경(hc-fill _gc)에서 돈다 — 바퀴 안에서 차례로 돌 때 큰 저장소 한 번이
# 9분 넘게 걸려 그동안 감사 수거·띄우기가 멈췄다(2026-11-21 fill-gc-async). 잠금 gc.lock(lockf — 프로세스가 끝나면 풀림)이 쥐여
# 있으면 앞 정리가 아직 도는 것이라 이번 바퀴는 띄우지 않는다. 시각은 띄울 때 적는다(실패·멈춤이 바퀴마다 되풀이되지 않게).
# 출력은 $DATA/gc.log에(바퀴 출력과 줄이 섞이지 않게). dry-run은 지금처럼 바퀴 안에서 --dry-run으로 돌고 시각을 적지 않는다
gc_tick() {
  gm=$(conf gc); gm=${gm:-60}
  case $gm in *[!0-9]*) echo "알림: 설정 gc는 분(숫자)이다, 정리 건너뜀: $gm"; return ;; esac
  gm=$((10#$gm))   # 08·09도 십진수로(감사 [낮음] 2)
  [ "$gm" != 0 ] || return 0
  gl=$(cat "$DATA/gc.last" 2>/dev/null); gn=$(date +%s)
  case $gl in '' | *[!0-9]*) gl=0 ;; esac
  [ $((gn - gl)) -ge $((gm * 60)) ] || return 0
  [ $dry = 0 ] || { gc_all; return 0; }
  lockf -k -t 0 "$DATA/gc.lock" true 2>/dev/null; glr=$?
  [ $glr != 75 ] || { echo "gc: 앞 정리가 아직 돈다($DATA/gc.lock) — 이번 바퀴는 안 띄움"; return 0; }
  [ $glr = 0 ] || { echo "gc 잠금 실패(종료 $glr, $DATA/gc.lock) — 이번 바퀴는 안 띄움"; return 0; }
  echo "$gn" >"$DATA/gc.last"
  lockf -k -t 0 "$DATA/gc.lock" "$0" _gc </dev/null >>"$DATA/gc.log" 2>&1 &
  echo "gc: 배경으로 띄움(pid $!) — 출력 $DATA/gc.log"
}

gc_all() {   # repo 줄마다 hc-lane gc(출력은 표준 출력). dry=1이면 --dry-run
  gcon=$(connected)
  awk '$1=="repo" {print $2, $3}' "$CONF" | while read -r gp gmach; do
    gr=$(repo_of "$gp" "$gmach"); [ -n "$gr" ] || continue
    if [ "$gmach" = "$LOCAL" ]; then set -- --integ
    elif printf '%s\n' "$gcon" | grep -Fxq -- "$gmach"; then set -- --on "$gmach"
    else echo "gc 건너뜀 $gp @$gmach: 연결 안 됨"; continue; fi
    [ $dry = 0 ] || set -- "$@" --dry-run
    go=$(hc-lane gc "$@" --repo "$gr" 2>&1 </dev/null); grc=$?
    printf '%s\n' "$go" | sed "s|^|gc $gp @$gmach: |"
    [ $grc = 0 ] || echo "gc 실패 $gp @$gmach (종료 $grc)"
  done
}

connected() { hc machine ls --json 2>/dev/null | jq -r '.machines | arrays | map(select(.state == "connected") | .name) | join("\n")' 2>/dev/null; }
up_ok() { [ "$1" = "$LOCAL" ] || printf '%s\n' "$ICON" | grep -Fxq -- "$1"; }   # ICON: 연결된 기계 목록

onm() {  # onm [-i] <기계> <sh 본문> <경로> [인자…]: 그 기계에서(local이면 바로) sh로. -i 없으면 표준 입력은 /dev/null
  i=; [ "$1" != -i ] || { i=1; shift; }
  om=$1 ob=$X$2; shift 2
  if [ "$om" = "$LOCAL" ]; then
    if [ -n "$i" ]; then sh -c "$ob" _ "$@"; else sh -c "$ob" _ "$@" </dev/null; fi
  elif [ -n "$i" ]; then hc exec -i --on "$om" -- sh -c "$ob" _ "$@"
  else hc exec --on "$om" -- sh -c "$ob" _ "$@" </dev/null; fi
}

# 받은편지 걷기: 설정 inbox <기계> <폴더> 줄마다, 그 기계 폴더의 <이름>.req(키<TAB>값 줄: project·after·only)를 검사해
# 대기열에 넣는다(되돌릴곳 <기계>:<폴더>/done/<이름>.md#<요청 ID>). 검사에 걸리면 둘을 rejected/로 옮기고 rejected/<이름>.why에 사유 한 줄.
# 요청 ID(r<UTC 초 36진>-<난수 6자>, 2026-11-20 fill-handoff-integrity)는 접수 때 한 번 정해 대기열 11칸·장부·고정본·레인 보고 경로·
# 요청자 표시에 모두 싣는다. 차례: 접수 표시 <폴더>/.hc-fill/claim/<이름>(요청 ID·.req·지시문 sha256)을 먼저 → 지시문 사본 → 대기열·장부
# (한 잠금) → 지시문 → taken/<이름>.<ID>.brief.md → .req → taken/<이름>.<ID>.req → 접수 표시 지움. 중간에 죽으면 다음 바퀴가 그 표시의
# 해시가 지금 원본과 같을 때만 같은 ID로 남은 단계를 잇는다 — 다르면 새 요청이라 옛 접수가 끝날 때까지 손대지 않는다.
# .req 없이 남은 지시문(옛 차례가 남긴 것·요청자가 지시문을 먼저 둔 중간)은 옮기지 않는다.
# 실행 명령·경로·모델은 받지 않는다(설정이 정한다). 연결 안 된 기계는 건너뛴다(요청은 그 기계에 그대로 쌓인다)
irej() {  # irej <사유>: 받은편지 요청 $n(기계 $im 폴더 $idir)을 rejected/로 옮기고 사유 한 줄
  onm "$im" 'mkdir -p "$d/rejected" && for e in req brief.md; do [ ! -f "$d/$1.$e" ] || mv -f "$d/$1.$e" "$d/rejected/" || exit 1; done &&
    printf "%s\n" "$2" >"$d/rejected/$1.why.tmp" && mv -f "$d/rejected/$1.why.tmp" "$d/rejected/$1.why"' "$idir" "$n" "$1" &&
    echo "거절 $im $n: $1" || echo "받은편지 $im $n: 거절($1)을 쓰지 못했다, 다음 바퀴에"
}
s_ilist() {
  printf %s 'if [ "$1" = 0 ]; then for c in "$d"/.hc-fill/claim/*; do [ -f "$c" ] || continue; n=${c##*/}; case $n in *.tmp) continue ;; esac; [ -f "$d/$n.req" ] || rm -f "$c"; done; fi
for f in "$d"/*.req; do [ -f "$f" ] || continue; f=${f##*/}; printf "%s\n" "${f%.req}"; done'
}
s_iclaim() {
  printf %s 'h() { if [ -f "$1" ]; then shasum -a 256 "$1" | cut -d " " -f 1; else echo -; fi; }
c=$d/.hc-fill/claim/$1; r=- cq=- cb=-; [ ! -f "$c" ] || read -r r cq cb <"$c" || :
if [ "$r" != - ] && [ -f "$d/taken/$1.$r.req" ]; then r=- cq=- cb=-; fi
b=$d/$1.brief.md; [ -f "$b" ] || [ "$r" = - ] || b=$d/taken/$1.$r.brief.md
q=$(h "$d/$1.req"); [ "$q" != - ] || exit 3
echo "${r:--} ${cq:--} ${cb:--} $q $(h "$b")"'
}
inbox_tick() {
  awk '$1=="inbox" {f=1} END {exit !f}' "$CONF" || return 0
  ICON=$(connected)
  while read -r -u 6 _ im idir; do
    [ -n "$idir" ] || { echo "알림: 설정 inbox 줄에 폴더가 없다: $im"; continue; }
    case $idir in *'#'*) echo "알림: 설정 inbox 폴더에 #를 쓰지 않는다(요청 ID 칸 구분): $idir"; continue ;; esac
    up_ok "$im" || { echo "받은편지 건너뜀 $im: 연결 안 됨"; continue; }
    names=$(onm "$im" "$(s_ilist)" "$idir" "$dry") || { echo "받은편지 건너뜀 $im: 목록을 읽지 못했다"; continue; }
    while IFS= read -r -u 7 n; do
      [ -n "$n" ] || continue
      why= p= a=- o=- pr=high rid=
      if ! okname "$n"; then why="이름은 a-z 0-9 - _ 32자까지"
      elif case $n in audit-*) true ;; *) false ;; esac; then why="이름은 audit- 로 시작하지 않는다(자동 감사 줄 이름 자리)"
      elif why=$(kindwhy "$n"); then :
      else
        ci=$(onm "$im" "$(s_iclaim)" "$idir" "$n") || { echo "받은편지 $im $n: 접수 표시를 읽지 못했다, 다음 바퀴에"; continue; }
        read -r crid cq cb hq hb <<<"$ci"
        if [ "$crid" != - ]; then
          if [ "$cq" = "$hq" ] && [ "$cb" = "$hb" ]; then rid=$crid
          else echo "받은편지 $im $n: 접수 중인 요청($crid)과 내용이 다르다 — 그 접수가 끝난 뒤 다음 바퀴에"; continue; fi
        fi
        req=$(onm "$im" 'cat "$d/$1.req"' "$idir" "$n") || { echo "받은편지 $im $n: .req를 읽지 못했다, 다음 바퀴에"; continue; }
        p=$(printf '%s\n' "$req" | awk -F'\t' '$1=="project" {print $2; exit}')
        a=$(printf '%s\n' "$req" | awk -F'\t' '$1=="after" {print $2; exit}'); a=${a:--}
        o=$(printf '%s\n' "$req" | awk -F'\t' '$1=="only" {print $2; exit}'); o=${o:--}
        # 우선순위: 적혀 있으면 그 값, 없으면 높음(회원이 겪는 오류 수정이 기능 개발보다 먼저 — 2026-11-17 지휘 결정)
        x=$(printf '%s\n' "$req" | awk -F'\t' '$1=="priority" {print $2; f=1; exit} END {if (!f) print "-"}')
        [ "$x" = - ] || pr=$(pnorm "$x") || { pr=high; why="모르는 priority: $x (긴급|높음|보통|낮음 또는 urgent|high|normal|low)"; }
        awk -v p="$p" '$1=="repo" && $2==p {f=1} END {exit !f}' "$CONF" || why="설정에 없는 project: ${p:-(없음)}"
        [ "$a" = - ] || for x in $(printf %s "$a" | tr , ' '); do okname "$x" || why="after 레인 이름: $x"; w=$(kindwhy "$x") && why="after: $w"; done
        [ "$o" = - ] || for x in $(printf %s "$o" | tr , ' '); do
          machines | awk -v m="$x" '$1==m {f=1} END {exit !f}' || why="설정에 없는 only 기계: $x"
        done
      fi
      if [ -z "$why" ] && [ -z "$rid" ]; then   # 새 접수: 같은 이름 줄이 있으면 거절, 지시문이 없거나 비었으면 거절
        hst=$(awk -F'\t' -v n="$n" '($1 "")==n {print $6; exit}' "$Q" 2>/dev/null)
        hrp=$(awk -F'\t' -v n="$n" '($1 "")==n {print $11; exit}' "$Q" 2>/dev/null)
        # 옛 차례(접수 표시 없음)가 대기열에 넣고 taken/으로 못 옮긴 요청: 옮긴 옛 줄(요청 ID rl…)이 이 받은편지·이름이면 그 ID로 옮기기만
        case $hrp in "$im:$idir/done/$n.md#rl"*) [ "$hst" = sent ] || { rid=${hrp##*#}; hst=; } ;; esac
        if [ -n "$rid" ]; then :
        elif [ "$hst" = sent ]; then why="이미 끝난 이름(결과를 되돌려 줌) — 새 이름으로 다시 요청"
        elif [ -n "$hst" ]; then why="이미 대기열에 있는 이름 — 새 이름으로 다시 요청"
        elif [ "$hb" = - ]; then why="지시문 $n.brief.md가 없거나 비었다"
        else case $(onm "$im" 'if grep -q "[^[:space:]]" "$d/$1.brief.md" 2>/dev/null; then echo yes; else echo no; fi' "$idir" "$n") in
          yes) ;;
          no) why="지시문 $n.brief.md가 없거나 비었다" ;;
          *) echo "받은편지 $im $n: 지시문을 확인하지 못했다, 다음 바퀴에"; continue ;;
        esac; fi
      fi
      if [ -n "$why" ]; then
        if [ $dry = 1 ]; then echo "${P}거절할 $im $n: $why"; continue; fi
        irej "$why"; continue
      fi
      if [ $dry = 1 ]; then echo "${P}받을 $im $n ($p)"; continue; fi
      if [ -z "$rid" ]; then   # 접수 표시를 먼저(요청 ID를 고정)
        rid=$(newid r); while [ -n "$(lget "$rid")" ] || [ -e "$DATA/replies/$rid.md" ]; do rid=$(newid r); done
        onm "$im" 'mkdir -p "$d/.hc-fill/claim" && printf "%s %s %s\n" "$2" "$3" "$4" >"$d/.hc-fill/claim/$1.tmp" && mv -f "$d/.hc-fill/claim/$1.tmp" "$d/.hc-fill/claim/$1"' \
          "$idir" "$n" "$rid" "$hq" "$hb" || { echo "받은편지 $im $n: 접수 표시를 두지 못했다, 다음 바퀴에"; continue; }
        crash inbox-after-claim
      fi
      bf=$DATA/briefs/$im/$n.$rid.md
      if [ ! -s "$bf" ]; then
        mkdir -p "${bf%/*}" && onm "$im" 'b=$d/$1.brief.md; [ -f "$b" ] || b=$d/taken/$1.$2.brief.md; cat "$b"' "$idir" "$n" "$rid" >"$bf.tmp" &&
          [ "$(shasum -a 256 "$bf.tmp" | cut -d ' ' -f 1)" = "$hb" ] && mv -f "$bf.tmp" "$bf" ||
          { echo "받은편지 $im $n: 지시문을 가져오지 못했다(또는 내용이 접수 때와 다르다), 다음 바퀴에"; continue; }
      fi
      if why=$(brief_bad "$bf"); then rm -f "$bf"; irej "지시문 $n.brief.md: $why"; continue; fi   # 본문 없음(부록만) 등(fill-dep-needs)
      ia=$(qlock _inadd "$n" "$p" "$bf" "$a" "$o" "$pr" "$im:$idir/done/$n.md" "$rid") || continue
      crash inbox-after-add
      [ "$ia" != ADDED ] || echo "받음 $im $n → 대기열 ($p, 우선 $(plabel "$pr"), 요청 $rid)"
      onm "$im" 'mkdir -p "$d/taken" && { [ ! -f "$d/$1.brief.md" ] || mv -f "$d/$1.brief.md" "$d/taken/$1.$2.brief.md"; } &&
        { [ ! -f "$d/$1.req" ] || mv -f "$d/$1.req" "$d/taken/$1.$2.req"; } && rm -f "$d/.hc-fill/claim/$1"' "$idir" "$n" "$rid" ||
        echo "받은편지 $im $n: taken/으로 옮기지 못했다, 다음 바퀴에 다시"
    done 7<<<"$names"
  done 6<<<"$(awk '$1=="inbox"' "$CONF")"
}

# 되돌려 주기(2026-11-20 fill-handoff-integrity — 결과를 쓴 터미널이 idle로 살아 있거나 착지가 없으면 안 나가던 것, 원격 쓰기 뒤 로컬
# 기록 실패·두 바퀴 겹침에 다시 쓰던 것):
#  1) 고정: up이고 되돌릴곳이 있는 줄의 장부가 open이면 레인 기계 보고(reports/<레인>.<요청 ID>.md — 옛 줄은 <레인>.md)가 완성일 때
#     (마지막 줄이 「결과: 고침|넘김|실패 …」) 그 내용을 고정본 replies/<요청 ID>.md에 배타 생성(먼저 확정된 것 — 결과든 취소든 — 이 이긴다).
#     터미널이 끝남 확정인데 완성 보고가 없으면 「결과: 실패 보고 없음」(반쪽이 있으면 「… 보고 불완전」). 터미널이 쉬든 착지가 없든 상관없다
#  2) 공개: 장부 deliver를 요청자 기계에 IPUB로(대상마다 잠금, 크기·sha256 확인, .hc-fill/<요청 ID>.sent 표시). SENT·ALREADY면 장부 sent,
#     CONFLICT·EXISTS면 held + 지휘 한 번(덮지 않음), BADINPUT·실패·응답 없음은 다음 바퀴에 같은 고정본으로
# 레인 기계나 요청한 기계가 연결 안 됨이면 다음 바퀴로
s_ipub() {
  printf %s 'mkdir -p "$d/done/.hc-fill" || exit 1
exec lockf -k -t 30 "$d/done/.hc-fill/$1.lock" sh -c '\''d=$1 n=$2 r=$3 h=$4 z=$5 m=$d/done/.hc-fill f=$d/done/$2.md o=$d/done/.hc-fill/owner/$2
 hs() { shasum -a 256 "$1" | cut -d " " -f 1; }
 setowner() { mkdir -p "${o%/*}" && printf "%s\n" "$r" >"$o.tmp" && mv -f "$o.tmp" "$o"; }
 mark() { printf "%s\n" "$h" >"$m/$r.sent.tmp" && mv -f "$m/$r.sent.tmp" "$m/$r.sent"; }
if [ -f "$m/$r.sent" ]; then if [ "$(cat "$m/$r.sent")" = "$h" ]; then echo ALREADY; else echo CONFLICT; fi; exit 0; fi
ready=$m/$r.ready; intent=$m/$r.publishing
if [ -f "$intent" ]; then
  [ "$(cat "$intent")" = "$h" ] || { echo CONFLICT; exit 0; }
  # ready가 사라지는 연산은 요청자 경로로의 rename뿐이다. 요청자가 이미 수거했어도 다시 공개하지 않는다.
  [ -e "$ready" ] || { mv -f "$intent" "$m/$r.sent" && echo ALREADY; exit; }
fi
t=$m/$r.tmp; cat >"$t" || exit 1
if [ "$(wc -c <"$t" | tr -d " ")" != "$z" ] || [ "$(hs "$t")" != "$h" ]; then rm -f "$t"; echo BADINPUT; exit 0; fi
if [ -f "$f" ]; then
  rm -f "$t"
  if [ -f "$o" ]; then
    owner=$(cat "$o") || exit 1
    [ "$owner" = "$r" ] || { echo EXISTS; exit 0; }
  fi
  if [ "$(hs "$f")" = "$h" ]; then setowner && mark && echo ALREADY; exit; fi
  echo EXISTS; exit 0
fi
setowner &&
  mv -f "$t" "$ready" && printf "%s\n" "$h" >"$intent.tmp" && mv -f "$intent.tmp" "$intent" &&
  mv -f "$ready" "$f" && mv -f "$intent" "$m/$r.sent" && echo SENT'\'' _ "$d" "$@"'
}
rdone() { printf '%s\n' "$1" | tail -n 1 | grep -Eq '^결과: (고침[[:blank:]]+[[:xdigit:]]{4,40}[[:blank:]]*|(넘김|실패)[[:blank:]]+[^[:space:]].*)$'; }
reply_tick() {
  ICON=$CONN
  # 1) 고정
  while IFS=$'\t' read -r -u 6 n _ _ _ _ _ m a _ _ rp _; do
    [ -n "$n" ] || continue
    case $rp in *'#'*) rid=${rp##*#} ;; *) continue ;; esac
    lr_=$(lget "$rid"); [ "$(printf %s "$lr_" | cut -f 4)" = open ] || continue
    if [ -e "$DATA/replies/$rid.md" ]; then [ $dry = 1 ] || rfix "$rid" "$(printf %s "$lr_" | cut -f 5)" -; continue; fi   # 고정 뒤 장부 전에 죽음
    up_ok "$m" || { echo "되돌려 주기 미룸 $n: $m 연결 안 됨"; continue; }
    if [ -n "$EVBAD" ] || printf '%s\n' "$EV" | grep -Fxq -- "모름 $m"; then
      echo "되돌려 주기 미룸 $n: 레인 사건을 못 읽어 제출 여부를 모름"; continue
    fi
    # 병합기에 제출하고 끝난 레인(마지막 사건 「제출」)은 그 제출 ID의 병합기 결과(착지·막힘)가 정해진 뒤에 회신하고, 그 결과를 끝 줄로
    # 붙인다(병합기 2차 높음 6·2단계 — 제출만 하고 끝난 터미널을 「끝」으로 보면 착지 전에 회신했다)
    mline_=
    lx_=$(joblast "$n" "$a" 제출)
    if [ -n "$lx_" ]; then
      sid_=$(printf %s "$lx_" | cut -f 7 | awk '{ print $1 }')
      mr_=$(mgres "$n" "$sid_")
      [ -n "$mr_" ] || { echo "되돌려 주기 미룸 $n: 병합기 결과 기다림(제출 ID $sid_)"; continue; }
      case $(printf %s "$mr_" | cut -f 5) in
        착지) mline_="착지 $(printf %s "$mr_" | cut -f 6 | cut -c 1-12) (병합기 $(printf %s "$mr_" | cut -f 2), 제출 ID $sid_)" ;;
        *) mline_="결과: 실패 병합기 막힘 — $(printf %s "$mr_" | cut -f 7)" ;;
      esac
    fi
    rpth="~/.local/share/hc-fill/reports/$n.$rid.md"; case $(printf %s "$lr_" | cut -f 8) in *legacy*) rpth="~/.local/share/hc-fill/reports/$n.md" ;; esac
    # 종료를 확인한 뒤 보고를 읽어, 마지막 mv와 종료가 같은 바퀴에 일어나도 본문을 놓치지 않는다.
    pget "$a"; endrc=$?
    if [ "$endrc" = 0 ]; then
      started=$(attempt_run "$n" "$a")
      if [ -n "$started" ] && [ "$PRUN" != "$started" ]; then endrc=3
      elif [ -n "$PNAME" ] && [ "$PNAME" != - ] && [ "$PNAME" != "$n" ] && ! lalive "$a" "$n"; then endrc=3; fi
    fi
    rep=$(onm "$m" 'if [ -f "$d" ]; then echo HAVE; cat "$d"; else echo NONE; fi' "$rpth") ||
      { echo "되돌려 주기 미룸 $n: $m 에서 보고를 읽지 못했다"; continue; }
    kind=result body=
    case $rep in
      HAVE*) body=${rep#HAVE}; body=${body#?} ;;
      NONE) ;;
      *) echo "되돌려 주기 미룸 $n: $m 에서 보고를 읽지 못했다"; continue ;;
    esac
    if [ -z "$body" ] || ! rdone "$body"; then
      [ "$endrc" = 3 ] || { lalive "$a" "$n" || echo "되돌려 주기 미룸 $n: 터미널 종료를 확인하지 못했다"; continue; }
      kind=noreport; if [ -n "$body" ]; then body="$body
결과: 실패 보고 불완전"; else body="결과: 실패 보고 없음"; fi
    fi
    [ -z "$mline_" ] || body="$body
$mline_"
    if [ $dry = 1 ]; then echo "${P}되돌려 줄 $n → ${rp%%#*} ($(printf '%s\n' "$body" | tail -n 1))"; continue; fi
    rfix "$rid" "$kind" "$body" || { echo "되돌려 주기 미룸 $n: 고정본을 두지 못했다"; continue; }
    crash after-reply-ledger
  done 6<<<"$(awk -F'\t' '$6=="up" && $11!="" && $11!="-"' "$Q" 2>/dev/null)"
  # 2) 공개
  while IFS=$'\t' read -r -u 6 rid n dest st _ h z _; do
    [ "$st" = deliver ] || continue
    rq=${dest%%:*} rpath=${dest#*:}; rdir=${rpath%/done/*}; rnm=${rpath##*/}; rnm=${rnm%.md}
    up_ok "$rq" || { echo "되돌려 주기 미룸 $n: $rq 연결 안 됨"; continue; }
    rf_=$DATA/replies/$rid.md
    [ -f "$rf_" ] || { echo "되돌려 주기 미룸 $n: 고정본이 없다 $rf_"; continue; }
    if [ $dry = 1 ]; then echo "${P}되돌려 줄 $n → $dest ($(tail -n 1 "$rf_"))"; continue; fi
    po_=$(onm -i "$rq" "$(s_ipub)" "$rdir" "$rnm" "$rid" "$h" "$z" <"$rf_") || { echo "되돌려 주기 실패 $n → $dest, 다음 바퀴에"; continue; }
    crash after-publish
    case $po_ in
      SENT | ALREADY)
        lset "$rid" 4 sent || { echo "되돌려 줌 $n → $dest — 장부 기록 실패, 다음 바퀴에 같은 내용으로 다시(요청자는 ALREADY)"; continue; }
        echo "되돌려 줌 $n → $dest ($(tail -n 1 "$rf_"))" ;;
      CONFLICT | EXISTS)
        aesc "$n" - "받은편지 결과를 못 둠 $n → $dest: 그 자리에 다른 결과가 있다($po_ — 덮지 않음). 지휘 판단(고정본 $rf_)" "hf:$rid:held" && lset "$rid" 4 held ;;
      *) echo "되돌려 주기 미룸 $n → $dest: 「${po_}」, 다음 바퀴에" ;;
    esac
  done 6<<<"$(cat "$LED" 2>/dev/null)"
  # 3) 장부 sent인데 대기열 줄이 아직 up이면 sent로(공개·장부 뒤 대기열 기록 전에 죽음)
  [ $dry = 0 ] || return 0
  awk -F'\t' 'NR==FNR { if ($4 == "sent") s[$1] = 1; next } $6=="up" && index($11, "#") { r = $11; sub(/^.*#/, "", r); if (r in s) print $1 "\t" $7 "\t" $8 "\t" $9 "\t" $17 }' "$LED" "$Q" 2>/dev/null |
    while IFS=$'\t' read -r n m a stuck row; do QRID=$row qset "$n" sent "$m" "$a" "$stuck"; done
}

mqload() {
  inlist "$1" "$MQP" && return 0
  MQP="$MQP,$1"
  r_=$(repo_of "$1" "$LOCAL")
  if [ -z "$r_" ]; then
    { awk -v p="$1" '$1 == "merger" && $2 == p { f = 1 } END { exit !f }' "$CONF" || cut -f 1 "$DATA/merger.tsv" 2>/dev/null | grep -Fxq -- "$1"; } &&
      MQBAD="$MQBAD,$1"
    return 0
  fi
  git -C "$r_" remote get-url origin >/dev/null 2>&1
  case $? in 0) ;; 2) return 0 ;; *) MQBAD="$MQBAD,$1"; return 0 ;; esac
  o_=$(git -C "$r_" ls-remote origin 'refs/merge-queue/*' 'refs/herdr/merger/*' 2>/dev/null) || { MQBAD="$MQBAD,$1"; return 0; }
  MQS="$MQS$(printf '%s\n' "$o_" | awk -v p="$1" 'NF == 2 { print p "\t" $2 }')
"
}
# mqueued <프로젝트> <레인>: 그 레인 제출이 큐에 있으면 0, 없으면 1, 못 읽으면 2(감사 land-merge-train-2 중간 1·병합기 1차 중간 5 —
# 제출한 레인은 바로 끝나 터미널이 사라지지만 변경은 병합기가 넣기 전이다. 모르면 소유를 쥔다)
mqueued() {
  mqload "$1"; inlist "$1" "$MQBAD" && return 2
  printf '%s' "$MQS" | awk -F'\t' -v p="$1" -v l="$2" '$1 == p { n = $2; if (sub(/^refs\/merge-queue\/[0-9]+-/, "", n) && n == l) f = 1 } END { exit !f }'
}
mgmach() { mqload "$1"; printf '%s' "$MQS" | awk -F'\t' -v p="$1" '$1 == p && sub(/^refs\/herdr\/merger\//, "", $2) { print $2 }'; }   # 켜진 병합기 기계
# merger_tick: 설정 줄 「merger <프로젝트> <기계>」 하나가 그 프로젝트의 병합기를 켜고 끈다(병합기 1차 중간 10 — 지시문 「설정 한 곳」).
# 레인 기계는 개인 설정을 못 읽으니 hc-fill이 그 줄을 원격 켬 표식 refs/herdr/merger/<기계>로 옮기고(레인의 hc-lane land는 그 표식만 본다),
# 줄을 지우면 표식을 지운다. 손대는 것은 이 설정이 켠 적이 있는 프로젝트뿐($DATA/merger.tsv). dry-run은 할 일만 찍는다.
# 대기열이 비어 일찍 끝나는 바퀴에도 돈다(병합기 2차 중간 9 — 빈 대기열에서 켜고 끈 설정이 원격에 안 갔다)
merger_tick() {
  mgf_=$DATA/merger.tsv
  want_=$(awk '$1 == "merger" && NF >= 3 { print $2 "\t" $3 }' "$CONF")
  for p_ in $( { printf '%s\n' "$want_" | cut -f 1; cut -f 1 "$mgf_" 2>/dev/null; } | awk 'NF && !s[$0]++'); do
    w_=$(printf '%s\n' "$want_" | awk -F'\t' -v p="$p_" '$1 == p { print $2; exit }')
    r_=$(repo_of "$p_" "$LOCAL"); [ -n "$r_" ] || { echo "병합기 $p_: 설정에 $LOCAL repo 줄이 없다 — 켬 표식을 못 고침"; continue; }
    mqload "$p_"; inlist "$p_" "$MQBAD" && { echo "병합기 $p_: 원격을 못 읽음 — 켬 표식은 다음 바퀴에"; continue; }
    cur_=$(mgmach "$p_")
    set --
    for c_ in $cur_; do [ "$c_" = "$w_" ] || set -- "$@" ":refs/herdr/merger/$c_"; done
    [ -z "$w_" ] || printf '%s\n' "$cur_" | grep -Fxq -- "$w_" || set -- "$@" "+refs/remotes/origin/HEAD:refs/herdr/merger/$w_"
    if [ $dry = 0 ] && [ -n "$w_" ]; then
      # 켬 push의 응답을 잃더라도 설정 줄을 지우면 다음 바퀴가 끌 수 있어야 한다.
      if ! { { [ ! -e "$mgf_" ] || awk -F'\t' -v p="$p_" '$1 != p' "$mgf_"; } &&
        printf '%s\t%s\n' "$p_" "$w_"; } >"$mgf_.tmp" || ! mv -f "$mgf_.tmp" "$mgf_"; then
        echo "병합기 $p_: 관리 기록을 못 써 켬 표식을 보류"; continue
      fi
    fi
    if [ $# -gt 0 ]; then
      if [ $dry = 1 ]; then echo "${P}병합기 $p_: git push origin $*"
      elif git -C "$r_" push -q --atomic origin "$@" 2>/dev/null; then echo "병합기 $p_: ${w_:+켬 → $w_}${w_:-끔}"
      else echo "병합기 $p_: 켬 표식 push 실패 — 다음 바퀴에"; continue; fi
    fi
    [ $dry = 1 ] && continue
    [ -n "$w_" ] || { awk -F'\t' -v p="$p_" '$1 != p' "$mgf_" >"$mgf_.tmp" && mv -f "$mgf_.tmp" "$mgf_"; }
  done
  MQP= MQS= MQBAD=   # 켬 표식을 고쳤을 수 있으니 이 바퀴의 나머지는 다시 읽는다
}

# 자동 감사(설정 audit — 2026-11-19 fill-auto-audit: 완료 → 지휘가 감사를 띄움 → 판정 → 착지 지시 왕복이 지휘 한 사람에게 줄을 섰다).
# 감사·착지 절을 붙여 띄운 쓰기 레인(감사 칸 auto)이 「완료 <sha>」를 적으면 그 기계에 긴급 감사 줄을 넣고, 판정을 레인에 넘긴다.
# 넘기기는 빠짐없이·한 번만(2026-11-20 fill-handoff-integrity — 판정이 다른 커밋·다른 실행·지난 감사에 묶이거나 hc send 한 번에 기대던 것):
#  · 감사 하나 = 차수 하나 = 차수 ID aid(a<UTC 초 36진>-<난수 6자>). 감사 줄·터미널 이름 audit-<aid>, 지시문 briefs/audit/<aid>.md,
#    run 사본 runs/audit-<aid>-run.md, 보고서 reports/audit-<aid>.md — 차수마다 따로(이름만으로 덮어쓰지 않음).
#  · 차수는 (부모 실행 <터미널>@<created_ms>, 40자 sha, aid)에 묶인다. 판정은 본문에 「감사 완료 <aid> 」, 6칸이 그 40자 sha일 때만 받고,
#    부모 실행이 그대로이고 그 실행의 최신 완료가 같은 sha일 때만 소비한다. 부모가 끝났으면 지휘로, 새 sha를 보고했으면 버리고 새 차수.
#  · 소비한 판정은 레인 기계 verdicts/<레인>/<aid>.md(영속 지시, 크기·sha256 확인)로 두고 hc send는 깨우기만(몇 번 가도 무해).
#    레인이 hc-lane verdict <aid>로 수거하면 <aid>.ack(taken·landing)가 생기고 그것으로 끝. 취소는 같은 표시를 void로 먼저 잡는다.
#  · 감사 터미널은 끄지 않는다(사용자 결정) — 차수가 끝나면 정리할 주소를 지휘에 사건 한 번.
#  · 사건(지휘 넘김·정리)은 hc-lane report --key hf:<aid>:<까닭>으로 한 번. 기록이 곧 의도라 중간에 죽어도 다음 바퀴가 남은 단계만 잇는다.
# 기록 $DATA/audit.tsv — 한 줄 = 한 차수(17칸, 탭): aid 레인 차수 부모실행 sha(40자) 기계 부모주소 상태 지적수 판정(pass|fix|-)
#   깨움수 기다린초(연결된 채) 마지막확인(초) 마지막깨움(초) 판정없음(바퀴) 표시(noack 보냄 등 쉼표, -) 비고(지휘 글)
# 상태: opening → queued → deliver → taken(끝) · superseded(끝, 새 완료) · escalate:<까닭> → coord(끝) · escalate:nosha → nosha(끝) ·
#   legacy(옛 7칸 기록을 옮긴 끝 상태 — 같은 sha로 다시 열지 않음)
# 옛 기록(7칸 레인 sha 몇번째 지적수 상태 실패 판정없음)은 바퀴 처음에 한 번 옮긴다(amigrate)
aconf() {  # 깨움 간격·수거 상한(분)
  WAKE=$(conf wake_every); WAKE=${WAKE:-10}; case $WAKE in '' | *[!0-9]*) echo "알림: 설정 wake_every는 분(숫자)이다, 10으로: $WAKE"; WAKE=10 ;; esac
  ACKMAX=$(conf ack_max); ACKMAX=${ACKMAX:-30}; case $ACKMAX in '' | *[!0-9]*) echo "알림: 설정 ack_max는 분(숫자)이다, 30으로: $ACKMAX"; ACKMAX=30 ;; esac
  WAKE=$((10#$WAKE)) ACKMAX=$((10#$ACKMAX))
}

# 놀고 있음: 바퀴 끝에도 자리가 남은 기계(연결됨, 그 기계 repo 줄이 있음)가 설정 idle분 넘게 이어지면 사건 한 번.
# 사유가 「대기열에 띄울 것 없음」이면 비기 시작한 뒤 그 한 번뿐(2026-11-19 idle-once, 사용자 「알림 대기열 빌 때 한 번만 오게 해줘」 —
# 빈 대기열에서 기계 7대가 30분·2시간마다 되풀이). 「먼저 띄움 상한에 걸려 남김」(지휘가 손쓸 일)은 계속 비면 30분마다 세 번, 그 뒤 2시간마다
# (감사 [낮음] 3). 「겹침 대기 n줄」(앞 레인이 착지하면 저절로 풀림 — lane-overlap-guard)은 대기열 빔처럼 한 번.
# 지난번과 사유가 바뀌면 바로 다시 알린다(같은 사유 되풀이만 막음).
# 받는 지휘: 대기 줄의 주인 가운데 살아 있는 것(hc-lane coord --check가 「다름·없음」이 아닌 것). 주인 없는 대기 줄이 있거나 대기 줄이
# 없거나 살아 있는 주인이 없으면 모든 지휘(--to 없이 — 주인 모르는 사건은 모든 지휘 watch가 보낸다).
# 기록 $DATA/idle.tsv(기계 비기시작 보낸시각 보낸횟수 빈자리수 보낸사유(none·cap·ovl) — 빈 자리 수가 줄면 처음부터. 옛 5칸 줄은 사유 none으로 읽음)
idle_tick() {
  [ "$IDLE" != 0 ] && [ $dry = 0 ] || return 0
  inow=$(date +%s) inew= idl=
  while read -r -u 4 m cap lcap _; do
    # 사유는 기계마다: 먼저 띄움 상한 · 그 기계가 받을 수 있는(기계 제한·repo 줄) 겹침 대기 줄 수 · 대기열에 띄울 것 없음(아스트라 감사 중간 5)
    ik=none ion=0
    if [ -n "$ECAPHIT" ] && { inlist - "$ECAPM" || inlist "$m" "$ECAPM"; }; then ik=cap   # 상한에 걸린 후보를 받을 기계만(2차 낮음 1)
    else
      while IFS=$'\t' read -r _ _ _ oo_ op_; do
        [ -n "$op_" ] || continue; [ "$oo_" = - ] || inlist "$m" "$oo_" || continue
        [ -n "$(repo_of "$op_" "$m")" ] && ion=$((ion + 1))
      done <<<"$OVWAIT"
      [ $ion = 0 ] || ik=ovl
    fi
    awk -v m="$m" '$1=="repo" && $3==m {f=1} END {exit !f}' "$CONF" || continue
    [ "$m" = "$LOCAL" ] || printf '%s\n' "$CONN" | grep -Fxq -- "$m" || continue
    fr=$((cap - $(agents_on "$m")))
    [ "$lcap" = - ] || { l=$((lcap - $(lanes_on "$m"))); [ $l -ge $fr ] || fr=$l; }
    [ $fr -gt 0 ] || continue
    read -r isince isent icnt ifr ikind <<<"$(awk -F'\t' -v m="$m" '$1==m {print $2, $3, $4, $5, $6; exit}' "$DATA/idle.tsv" 2>/dev/null)"
    case $isince in '' | *[!0-9]*) isince=$inow isent=0 icnt=0 ;; esac
    case $isent in '' | *[!0-9]*) isent=0 ;; esac
    case $icnt in '' | *[!0-9]*) icnt=0 ;; esac
    case $ifr in '' | *[!0-9]*) ;; *) [ "$fr" -ge "$ifr" ] || isince=$inow isent=0 icnt=0 ;; esac   # 자리가 하나라도 찼으면 처음부터
    case $ikind in cap | ovl) ;; *) ikind=none ;; esac
    [ "$isent" != 0 ] || ikind=
    inew="$inew$m	$isince	$isent	$icnt	$fr	$ikind
"
    [ $((inow - isince)) -ge $((IDLE * 60)) ] || continue
    igap=1800; [ "$icnt" -lt 3 ] || igap=7200
    # 처음이거나 사유가 바뀌면 보냄, 같은 사유면 대기열 빔은 다시 안 보내고 상한은 간격대로
    [ "$isent" = 0 ] || [ "$ikind" != "$ik" ] || { [ $ik = cap ] && [ $((inow - isent)) -ge $igap ]; } || continue
    imx="$m 자리 ${fr}개 $(((inow - isince) / 60))분째 빔"; [ $ik != ovl ] || imx="$imx(겹침 대기 ${ion}줄)"
    idl="$idl$ik	$m	$imx
"
  done 4<<<"$(machines)"
  # 사유마다 사건 하나(그 사유의 기계만)
  if [ -n "$idl" ]; then
    ito= iall=
    for o in $(awk -F'\t' '$6=="wait" {print ($12 == "" ? "-" : $12)}' "$Q" 2>/dev/null | sort -u); do
      if [ "$o" = - ]; then iall=1; continue; fi
      hc-lane coord --check "$o" >/dev/null 2>&1 </dev/null; [ $? = 1 ] || ito="$ito${ito:+,}$o"
    done
    [ -z "$iall" ] || ito=
  fi
  for ik in cap ovl none; do
    idue=$(printf %s "$idl" | awk -F'\t' -v k="$ik" '$1==k {printf " %s", $2}')
    [ -n "$idue" ] || continue
    imsg=$(printf %s "$idl" | awk -F'\t' -v k="$ik" '$1==k {printf "%s%s", (n++ ? " · " : ""), $3}')
    case $ik in
      cap) iwhy="먼저 띄움 상한(early ${ECAP})에 걸려 남김 — 상한을 올리거나 일을 더 넣는다" ;;
      ovl) iwhy="겹침 대기 — 앞 레인이 착지하면 뜬다(hc-fill ls)" ;;
      *) iwhy="대기열에 띄울 것 없음" ;;
    esac
    if env -u HC_TERMINAL_ID -u HC_LANE hc-lane report 중간 - "$imsg — $iwhy" --name hc-fill-idle ${ito:+--to "$ito"} >/dev/null 2>&1 </dev/null; then
      echo "놀고 있음 알림 → ${ito:-모든 지휘}: $imsg — $iwhy"
      inew=$(printf %s "$inew" | awk -F'\t' -v OFS='\t' -v d="$idue " -v t="$inow" -v k="$ik" 'index(" " d, " " $1 " ") {$3 = t; $4 = ($6 == k ? $4 + 1 : 1); $6 = k} 1')   # 사유가 바뀌면 횟수도 처음부터(감사 낮음 1)
    else echo "놀고 있음 알림 실패(hc-lane report) — 다음 바퀴에 다시"; fi
  done
  printf '%s' "$inew" >"$DATA/idle.tsv.tmp" && mv -f "$DATA/idle.tsv.tmp" "$DATA/idle.tsv"
}
