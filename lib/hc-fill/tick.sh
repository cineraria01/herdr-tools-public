# hc-fill 함수: tick (bin/hc-fill가 읽는다. 함수는 bin/hc-fill 안 원래 정의 순서 그대로)

# 끝난 레인 정리: 설정 gc 분(기본 60, 0이면 끔)마다 한 번, repo 줄의 기계·저장소마다 hc-lane gc. 원격 중간 참조(--integ)는
# 프로젝트마다 local 기계의 저장소에서 한 번. 시각은 돌리기 전에 적는다(실패·멈춤이 바퀴마다 되풀이되지 않게). dry-run은 적지 않는다
gc_tick() {
  gm=$(conf gc); gm=${gm:-60}
  case $gm in *[!0-9]*) echo "알림: 설정 gc는 분(숫자)이다, 정리 건너뜀: $gm"; return ;; esac
  gm=$((10#$gm))   # 08·09도 십진수로(감사 [낮음] 2)
  [ "$gm" != 0 ] || return 0
  gl=$(cat "$DATA/gc.last" 2>/dev/null); gn=$(date +%s)
  case $gl in '' | *[!0-9]*) gl=0 ;; esac
  [ $((gn - gl)) -ge $((gm * 60)) ] || return 0
  [ $dry = 1 ] || echo "$gn" >"$DATA/gc.last"
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
# <이름>.brief.md를 $DATA/briefs/<기계>/<이름>.md로 복사하고 대기열에 넣은 뒤(되돌릴곳 <기계>:<폴더>/done/<이름>.md)
# 그 기계에서 둘을 taken/으로 옮긴다. 검사에 걸리면 둘을 rejected/로 옮기고 rejected/<이름>.why에 사유 한 줄.
# 실행 명령·경로·모델은 받지 않는다(설정이 정한다). 연결 안 된 기계는 건너뛴다(요청은 그 기계에 그대로 쌓인다)
irej() {  # irej <사유>: 받은편지 요청 $n(기계 $im 폴더 $idir)을 rejected/로 옮기고 사유 한 줄
  onm "$im" 'mkdir -p "$d/rejected" && for e in req brief.md; do [ ! -f "$d/$1.$e" ] || mv -f "$d/$1.$e" "$d/rejected/" || exit 1; done &&
    printf "%s\n" "$2" >"$d/rejected/$1.why.tmp" && mv -f "$d/rejected/$1.why.tmp" "$d/rejected/$1.why"' "$idir" "$n" "$1" &&
    echo "거절 $im $n: $1" || echo "받은편지 $im $n: 거절($1)을 쓰지 못했다, 다음 바퀴에"
}
inbox_tick() {
  awk '$1=="inbox" {f=1} END {exit !f}' "$CONF" || return 0
  ICON=$(connected)
  while read -r -u 6 _ im idir; do
    [ -n "$idir" ] || { echo "알림: 설정 inbox 줄에 폴더가 없다: $im"; continue; }
    up_ok "$im" || { echo "받은편지 건너뜀 $im: 연결 안 됨"; continue; }
    names=$(onm "$im" 'for f in "$d"/*.req; do [ -f "$f" ] || continue; f=${f##*/}; printf "%s\n" "${f%.req}"; done' "$idir") ||
      { echo "받은편지 건너뜀 $im: 목록을 읽지 못했다"; continue; }
    while IFS= read -r -u 7 n; do
      [ -n "$n" ] || continue
      why= p= a=- o=- pr=high
      if ! okname "$n"; then why="이름은 a-z 0-9 - _ 32자까지"
      elif case $n in audit-*) true ;; *) false ;; esac; then why="이름은 audit- 로 시작하지 않는다(자동 감사 줄 이름 자리)"
      elif why=$(kindwhy "$n"); then :
      elif ! req=$(onm "$im" 'cat "$d/$1.req"' "$idir" "$n"); then echo "받은편지 $im $n: .req를 읽지 못했다, 다음 바퀴에"; continue
      else
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
      had=$(awk -F'\t' -v n="$n" '($1 "")==n {print ($11 == "" ? "-" : $11); exit}' "$Q" 2>/dev/null)
      hst=$(awk -F'\t' -v n="$n" '($1 "")==n {print $6; exit}' "$Q" 2>/dev/null)
      if [ -z "$why" ] && [ -n "$had" ]; then
        if [ "$had" != "$im:$idir/done/$n.md" ]; then why="이미 대기열에 있는 이름"
        elif [ "$hst" = sent ]; then why="이미 끝난 이름(결과를 되돌려 줌) — 새 이름으로 다시 요청"
        else   # 같은 요청을 넣고 taken/으로 못 옮긴 것인지, 같은 이름의 새 요청인지: taken/에 이미 있으면 새 요청
          case $(onm "$im" 'if [ -f "$d/taken/$1.req" ]; then echo yes; else echo no; fi' "$idir" "$n") in
            yes) why="이미 받아 진행 중인 이름 — 새 이름으로 다시 요청" ;;
            no) ;;
            *) echo "받은편지 $im $n: taken/을 확인하지 못했다, 다음 바퀴에"; continue ;;
          esac
        fi
      fi
      if [ -z "$why" ] && [ -z "$had" ]; then   # 없음(no)만 거절, 확인 못 함은 다음 바퀴에
        case $(onm "$im" 'if grep -q "[^[:space:]]" "$d/$1.brief.md" 2>/dev/null; then echo yes; else echo no; fi' "$idir" "$n") in
          yes) ;;
          no) why="지시문 $n.brief.md가 없거나 비었다" ;;
          *) echo "받은편지 $im $n: 지시문을 확인하지 못했다, 다음 바퀴에"; continue ;;
        esac
      fi
      if [ -n "$why" ]; then
        if [ $dry = 1 ]; then echo "${P}거절할 $im $n: $why"; continue; fi
        irej "$why"; continue
      fi
      if [ $dry = 1 ]; then echo "${P}받을 $im $n ($p)"; continue; fi
      if [ -z "$had" ]; then   # 이미 있으면(지난 바퀴에 넣고 옮기지 못함) 옮기기만
        bf=$DATA/briefs/$im/$n.md
        mkdir -p "${bf%/*}" && onm "$im" 'cat "$d/$1.brief.md"' "$idir" "$n" >"$bf.tmp" && mv -f "$bf.tmp" "$bf" ||
          { echo "받은편지 $im $n: 지시문을 가져오지 못했다, 다음 바퀴에"; continue; }
        if why=$(brief_bad "$bf"); then rm -f "$bf"; irej "지시문 $n.brief.md: $why"; continue; fi   # 본문 없음(부록만) 등
        qlock _add "$n" "$p" "$bf" "$a" "$o" wait - - - "$(date +%Y-%m-%dT%H:%M)" "$im:$idir/done/$n.md" - "$pr" - - || continue
        echo "받음 $im $n → 대기열 ($p, 우선 $(plabel "$pr"))"
      fi
      onm "$im" 'mkdir -p "$d/taken" && mv -f "$d/$1.req" "$d/taken/" && { [ ! -f "$d/$1.brief.md" ] || mv -f "$d/$1.brief.md" "$d/taken/"; }' "$idir" "$n" ||
        echo "받은편지 $im $n: taken/으로 옮기지 못했다, 다음 바퀴에 다시"
    done 7<<<"$names"
  done 6<<<"$(awk '$1=="inbox"' "$CONF")"
}

# 되돌려 주기: up이고 되돌릴곳이 있는 줄. 레인 기계에 보고(~/.local/share/hc-fill/reports/<레인>.md)가 있고 터미널이 사라졌거나
# done·idle이면 그 보고를, 보고 없이 터미널이 사라졌으면 「결과: 실패 보고 없음」을 요청한 기계에 쓰고 상태 sent.
# 레인 기계나 요청한 기계가 연결 안 됨이면 다음 바퀴로(연결 안 된 기계의 터미널은 hc ls에 없어 사라진 것으로 보이므로)
reply_tick() {
  ICON=$CONN
  while IFS=$'\t' read -r -u 6 n _ _ _ _ _ m a stuck _ rp _; do
    [ -n "$n" ] || continue
    rq=${rp%%:*} rpath=${rp#*:}
    if ! up_ok "$m" || ! up_ok "$rq"; then echo "되돌려 주기 미룸 $n: $m 또는 $rq 연결 안 됨"; continue; fi
    st=$(printf '%s\n' "$TERMS" | awk -F'\t' -v a="$a" '$2==a {print $5; f=1; exit} END {if (!f) print "gone"}')
    case $st in gone | done | idle) ;; *) continue ;; esac
    # 병합기에 제출하고 끝난 레인(마지막 사건 「제출」)은 그 제출 ID의 병합기 결과(착지·막힘)가 정해진 뒤에 회신하고, 그 결과를 끝 줄로
    # 붙인다(병합기 2차 높음 6·2단계 — 제출만 하고 끝난 터미널을 「끝」으로 보면 착지 전에 회신했다)
    mline_=
    lx_=$(lastev "$n" "$a")
    if [ "$(printf %s "$lx_" | cut -f 5)" = 제출 ]; then
      [ -z "$EVBAD" ] || { echo "되돌려 주기 미룸 $n: 레인 사건을 못 읽어 병합기 결과를 모름"; continue; }
      sid_=$(printf %s "$lx_" | cut -f 7 | awk '{ print $1 }')
      mr_=$(mgres "$n" "$sid_")
      [ -n "$mr_" ] || { echo "되돌려 주기 미룸 $n: 병합기 결과 기다림(제출 ID $sid_)"; continue; }
      case $(printf %s "$mr_" | cut -f 5) in
        착지) mline_="착지 $(printf %s "$mr_" | cut -f 6 | cut -c 1-12) (병합기 $(printf %s "$mr_" | cut -f 2), 제출 ID $sid_)" ;;
        *) mline_="결과: 실패 병합기 막힘 — $(printf %s "$mr_" | cut -f 7)" ;;
      esac
    fi
    rep=$(onm "$m" 'if [ -f "$d" ]; then echo HAVE; cat "$d"; else echo NONE; fi' "~/.local/share/hc-fill/reports/$n.md") ||
      { echo "되돌려 주기 미룸 $n: $m 에서 보고를 읽지 못했다"; continue; }
    case $rep in
      HAVE*) body=${rep#HAVE}; body=${body#?} ;;
      NONE) [ "$st" = gone ] || continue
        hc get "$a" --json 2>&1 </dev/null | grep -Eq '"not_found"|"state" *: *"finished"' ||   # 없어졌거나 프로세스가 끝남
          { echo "되돌려 주기 미룸 $n: hc ls에 없지만 hc get이 없음(not_found)·끝남(finished)이라 하지 않는다"; continue; }
        body="결과: 실패 보고 없음" ;;
      *) echo "되돌려 주기 미룸 $n: $m 에서 보고를 읽지 못했다"; continue ;;
    esac
    [ -z "$mline_" ] || body="$body
$mline_"
    last=$(printf '%s\n' "$body" | tail -n 1)
    if [ $dry = 1 ]; then echo "${P}되돌려 줄 $n → $rp ($last)"; continue; fi
    printf '%s\n' "$body" | onm -i "$rq" 'mkdir -p "${d%/*}" && cat >"$d.tmp" && mv -f "$d.tmp" "$d"' "$rpath" ||
      { echo "되돌려 주기 실패 $n → $rp, 다음 바퀴에"; continue; }
    qset "$n" sent "$m" "$a" "$stuck"
    echo "되돌려 줌 $n → $rp ($last)"
  done 6<<<"$(awk -F'\t' '$6=="up" && $11!="" && $11!="-"' "$Q" 2>/dev/null)"
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
    if [ $# -gt 0 ]; then
      if [ $dry = 1 ]; then echo "${P}병합기 $p_: git push origin $*"
      elif git -C "$r_" push -q origin "$@" 2>/dev/null; then echo "병합기 $p_: ${w_:+켬 → $w_}${w_:-끔}"
      else echo "병합기 $p_: 켬 표식 push 실패 — 다음 바퀴에"; continue; fi
    fi
    [ $dry = 1 ] && continue
    if [ -n "$w_" ]; then awk -F'\t' -v p="$p_" '$1 != p' "$mgf_" 2>/dev/null >"$mgf_.tmp"; printf '%s\t%s\n' "$p_" "$w_" >>"$mgf_.tmp"; mv -f "$mgf_.tmp" "$mgf_"
    else awk -F'\t' -v p="$p_" '$1 != p' "$mgf_" >"$mgf_.tmp" && mv -f "$mgf_.tmp" "$mgf_"; fi
  done
  MQP= MQS= MQBAD=   # 켬 표식을 고쳤을 수 있으니 이 바퀴의 나머지는 다시 읽는다
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
