# hc-fill 함수: audit (bin/hc-fill가 읽는다. 함수는 bin/hc-fill 안 원래 정의 순서 그대로)

aget() { awk -F'\t' -v n="$1" '($1 "")==n' "$AT" 2>/dev/null | tail -n 1; }
aput() {  # aput <레인> <sha> <몇번째> <지적수> <상태> <실패> [<판정없음>]: 그 레인 줄을 바꾼다(dry-run은 안 씀)
  [ $dry = 0 ] || return 0
  { awk -F'\t' -v n="$1" '($1 "")!=n' "$AT" 2>/dev/null; printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" "$5" "$6" "${7:-0}"; } >"$AT.tmp" && mv -f "$AT.tmp" "$AT"
}
adel() {  # adel <레인>: 그 레인 줄을 지운다(없으면 그대로)
  [ $dry = 0 ] && [ -f "$AT" ] && awk -F'\t' -v n="$1" '($1 "")==n {f=1} END {exit !f}' "$AT" || return 0
  awk -F'\t' -v n="$1" '($1 "")!=n' "$AT" >"$AT.tmp" && mv -f "$AT.tmp" "$AT"
}
lalive() { printf '%s\n' "$TERMS" | awk -F'\t' -v a="$1" -v n="$2" '$2==a && ($4 "")==n {f=1} END {exit !f}'; }   # 그 주소에 그 이름 터미널
# audname <레인>: 감사 줄 이름. audit-<레인>이 32자를 넘으면 audit-<앞 21자>-<레인 cksum 아래 16비트 4자리 16진>(32자)
audname() {
  an=audit-$1; [ ${#an} -le 32 ] || an=audit-$(printf %s "$1" | cut -c 1-21)-$(printf %s "$1" | cksum | awk '{printf "%04x", $1 % 65536}')
  echo "$an"
}
alive() { printf '%s\n' "$TERMS" | awk -F'\t' -v a="$1" '$2==a {print $5; f=1; exit} END {if (!f) print "gone"}'; }   # 상태 또는 gone
lastev() {  # lastev <이름> <터미널> [<상태>]: 그 터미널이 그 이름으로 적은 마지막 사건 줄(events는 시각 순)
  printf '%s\n' "$EV" | awk -F'\t' -v n="$1" -v a="$2" -v s="${3-}" '($4 "")==n && $3==a && (s == "" || $5==s) {l=$0} END {print l}'
}
aesc() {  # aesc <레인> <주인|-> <글>: 그 레인 주인 지휘(없으면 모든 지휘)에 막힘 사건. 0 남김(dry-run 포함)
  if [ $dry = 1 ]; then echo "${P}지휘로 넘길 $1: $3"; return 0; fi
  ao=$2; [ "$ao" != - ] || ao=
  if env -u HC_TERMINAL_ID -u HC_LANE hc-lane report 막힘 - "$3" --name "$1" ${ao:+--to "$ao"} >/dev/null 2>&1 </dev/null; then
    echo "지휘로 넘김 $1 → ${ao:-모든 지휘}: $3"; return 0
  fi
  echo "지휘로 넘기기 실패 $1(hc-lane report) — 다음 바퀴에 다시"; return 1
}
audit_tick() {
  [ -n "$AUD" ] && [ -s "$Q" ] || return 0
  # 볼 줄: 쓰기 줄(up·auto)은 터미널이 살아 있을 때, 감사 줄(up·audit=)은 그 레인이 감사에 넘겨진(queued) 동안
  LROWS= AROWS= ams=
  while IFS=$'\t' read -r -u 6 n _ _ _ _ _ m a _ _ _ _ _ _ k _; do
    case $k in
      auto) [ "$(alive "$a")" != gone ] || continue; LROWS="$LROWS$n
" ;;
      audit=*) [ "$(aget "${k#audit=}" | cut -f 5)" = queued ] || continue; AROWS="$AROWS$n
" ;;
      *) continue ;;
    esac
    [ "$m" = "$LOCAL" ] || inlist "$m" "$ams" || ams="$ams,$m"
  done 6<<<"$(awk -F'\t' '$6=="up" && ($15=="auto" || $15 ~ /^audit=/)' "$Q")"
  [ -n "$LROWS$AROWS" ] || return 0
  for m in $(printf %s "$ams" | tr , ' '); do printf '%s\n' "$CONN" | grep -Fxq -- "$m" || echo "자동 감사 미룸 @$m: 연결 안 됨"; done
  [ -z "$EVBAD" ] || { echo "자동 감사 미룸: hc-lane events 실패, 다음 바퀴에"; return 0; }   # 사건은 ev_tick이 읽었다
  AUNK=$(printf '%s\n' "$EV" | sed -n 's/^모름 //p')
  aok() { [ "$1" = "$LOCAL" ] || { printf '%s\n' "$CONN" | grep -Fxq -- "$1" && ! printf '%s\n' "$AUNK" | grep -Fxq -- "$1"; }; }
  qrow() { awk -F'\t' -v n="$1" '($1 "")==n {print; exit}' "$Q"; }
  # 1) 판정: 감사 줄마다 그 감사 터미널의 마지막 「판정」에서 높음·중간·낮음 수를 뽑아 레인에 통과·지적(또는 지휘로)
  while IFS= read -r -u 6 an; do
    [ -n "$an" ] || continue
    IFS=$'\t' read -r _ _ _ _ _ _ m aa _ _ _ _ _ _ k _ <<<"$(qrow "$an")"
    n=${k#audit=}
    IFS=$'\t' read -r _ _ _ _ _ _ lm la _ _ _ lo _ <<<"$(qrow "$n")"
    IFS=$'\t' read -r _ ash ar af ast afl anv <<<"$(aget "$n")"
    aok "$m" || continue
    rep="~/.local/share/hc-fill/reports/$an.md"
    v=$(lastev "$an" "$aa" 판정)
    if [ -z "$v" ]; then
      # 판정 없이 감사 터미널이 끝났거나(done·idle — 완료로 잘못 적음·판정 빠뜨림) 사라졌으면 세 바퀴 이어질 때 지휘로(재감사 [중간] 2)
      ats=$(alive "$aa"); [ "$aa" != - ] || ats=gone
      case $ats in
        gone | done | idle)
          anv=$((${anv:-0} + 1))
          if [ $anv -ge 3 ]; then
            lx=$(lastev "$an" "$aa" | cut -f 5,7 | tr '\t' ' ')
            aesc "$n" "$lo" "$n 감사 ${ar}번째 판정 없음 — 지휘 판단 (감사 터미널 $aa ${ats}, 마지막 사건 「${lx:-없음}」, $an @$m $rep)" &&
              aput "$n" "$ash" "$ar" "$af" coord "$afl"
          else aput "$n" "$ash" "$ar" "$af" queued "$afl" "$anv"; echo "감사 판정 없음 $an $aa ${ats} — ${anv}/3 바퀴"; fi ;;
        *) [ "${anv:-0}" = 0 ] || aput "$n" "$ash" "$ar" "$af" queued "$afl" 0 ;;
      esac
      continue
    fi
    msg=$(printf %s "$v" | cut -f 7)
    # 판정 줄의 sha가 지금 감사에 넘긴 sha가 아니면(지난 바퀴 감사의 판정) 건너뜀. sha가 없으면 받는다
    vs=$(printf %s "$msg" | sed -n 's/.*감사 완료 \([0-9a-f]\{7,40\}\).*/\1/p')
    case $vs in '' | "$ash"* ) ;; *) case $ash in "$vs"*) ;; *) continue ;; esac ;; esac
    # 레인이 그 사이 다른 sha로 완료를 다시 적었으면 이 판정은 옛 커밋 것 — 보내지 않고 아래 2)가 새 감사를 넣는다
    le=$(lastev "$n" "$la")
    if [ "$(printf %s "$le" | cut -f 5)" = 완료 ] && [ "$(printf %s "$le" | cut -f 6)" != "$ash" ]; then
      echo "판정 건너뜀 $an: $n 이 새 완료($(printf %s "$le" | cut -f 6))를 적었다 — 새 감사로"; continue
    fi
    h=$(printf %s "$msg" | sed -n 's/.*높음 *\([0-9][0-9]*\).*/\1/p') md=$(printf %s "$msg" | sed -n 's/.*중간 *\([0-9][0-9]*\).*/\1/p')
    lw=$(printf %s "$msg" | sed -n 's/.*낮음 *\([0-9][0-9]*\).*/\1/p')
    if [ -z "$h" ] || [ -z "$md" ]; then
      aesc "$n" "$lo" "$n 감사 ${ar}번째 판정 줄을 못 읽음(「${msg}」) — 지휘 판단 ($an @$m $rep)" && aput "$n" "$ash" "$ar" "$af" coord "$afl"
      continue
    fi
    if [ "$h" = 0 ] && [ "$md" = 0 ]; then
      ns=pass txt="감사 통과 ($ash, 낮음 ${lw:-?} — $rep). run 지시문의 착지 절차대로 main에 넣어라."
    elif [ "$af" -lt "$AMAX" ]; then
      ns=fix txt="감사 지적 ($ash, 높음 ${h}·중간 $md) — ${rep}를 읽고 높음·중간을 고친 뒤 다시 검증하고 hc-lane report 완료 <새 sha>."
    else
      aesc "$n" "$lo" "$n 감사 ${ar}번째 높음 ${h}·중간 $md — 지휘 판단 (지적 ${af}번, 상한 audit_max $AMAX, $an @$m $rep)" &&
        aput "$n" "$ash" "$ar" "$af" coord "$afl"
      continue
    fi
    if [ $dry = 1 ]; then echo "${P}보낼 $n $la: $txt"; continue; fi
    if ! lalive "$la" "$n"; then sw="레인 터미널 $la 없음"   # 그 주소에 그 레인 이름이 아니면(코어 재시작 등) 남에게 안 보냄
    elif hc send "$la" "$txt" >/dev/null 2>&1 </dev/null; then
      [ $ns = pass ] || af=$((af + 1))
      aput "$n" "$ash" "$ar" "$af" "$ns" 0
      if [ $ns = pass ]; then echo "감사 통과 보냄 $n $la ($ash, 낮음 ${lw:-?})"; else echo "감사 지적 보냄 $n $la ($ash, 높음 ${h}·중간 $md, ${af}번째)"; fi
      continue
    else sw="hc send 실패"; fi
    afl=$((afl + 1))
    if [ $afl -ge 3 ]; then
      aesc "$n" "$lo" "$n 감사 판정(높음 ${h}·중간 ${md})을 레인에 세 번 못 보냄($sw) — 지휘 판단 ($an @$m $rep)" &&
        aput "$n" "$ash" "$ar" "$af" coord "$afl" || aput "$n" "$ash" "$ar" "$af" queued "$afl"
    else aput "$n" "$ash" "$ar" "$af" queued "$afl"; echo "감사 판정 보내기 미룸 $n: $sw — 다음 바퀴에 다시(${afl}/3)"; fi
  done 6<<<"$AROWS"
  # 2) 완료: 쓰기 레인 터미널의 마지막 사건이 「완료 <sha>」이고 그 sha를 아직 감사에 넘기지 않았으면 그 기계에 감사 줄
  while IFS= read -r -u 6 n; do
    [ -n "$n" ] || continue
    IFS=$'\t' read -r _ proj _ _ _ _ m la _ _ _ lo _ <<<"$(qrow "$n")"
    aok "$m" || continue
    le=$(lastev "$n" "$la")
    [ "$(printf %s "$le" | cut -f 5)" = 완료 ] || continue
    sha=$(printf %s "$le" | cut -f 6)
    IFS=$'\t' read -r _ ash ar af ast afl _ <<<"$(aget "$n")"
    [ "$sha" != "${ash-}" ] || continue
    if [ "$sha" = - ]; then   # 완료에 sha가 없으면 감사할 커밋을 모른다 — 지휘에 한 번(sha 칸 -로 적어 되풀이 안 함)
      aesc "$n" "$lo" "자동 감사 못 함 $n: 완료에 sha 없음 — 지휘 판단 ($la)" && aput "$n" - "${ar:-0}" "${af:-0}" nosha 0
      continue
    fi
    an=$(audname "$n")
    if [ "${ast-}" = queued ]; then   # 지난 감사가 아직 돌면 그 판정 뒤에(두 감사가 함께 돌지 않게)
      IFS=$'\t' read -r _ _ _ _ _ ost _ oa _ <<<"$(qrow "$an")"
      if [ "${ost-}" = up ]; then case $(alive "$oa") in gone | done | idle) ;; *) echo "자동 감사 미룸 $n: 새 완료 $sha — 지난 감사($ash)가 아직 돈다"; continue ;; esac; fi
    fi
    repo=$(repo_of "$proj" "$m"); [ -n "$repo" ] || { echo "자동 감사 못 함 $n: 설정에 $proj @$m repo 줄이 없다"; continue; }
    base=$(git -C "$(repo_of "$proj" "$LOCAL")" symbolic-ref --short -q refs/remotes/origin/HEAD 2>/dev/null); base=${base#origin/}; base=${base:-main}
    nr=$((${ar:-0} + 1))
    if [ $dry = 1 ]; then echo "${P}넣을 감사 $an ← $n $sha (${nr}번째, $m)"; continue; fi
    t=$(cat -- "$AUD") || { echo "자동 감사 못 함 $n: 틀을 읽지 못함 $AUD — 다음 바퀴에"; continue; }
    t=${t//\{lane\}/$n} t=${t//\{machine\}/$m} t=${t//\{worktree\}/${repo%/}.wt/$n} t=${t//\{branch\}/$n} t=${t//\{sha\}/$sha}
    t=${t//\{base\}/$base} t=${t//\{brief\}/"~/.local/share/hc-fill/runs/$n-run.md"} t=${t//\{report\}/"~/.local/share/hc-fill/reports/$an.md"}
    bf=$DATA/briefs/audit/$an.md
    mkdir -p "${bf%/*}" && printf '%s\n' "$t" >"$bf.tmp" && mv -f "$bf.tmp" "$bf" || { echo "자동 감사 못 함 $n: 감사 지시문을 쓰지 못함 $bf"; continue; }
    qlock _put "$an" "$proj" "$bf" - "$m" wait - - - "$(date +%Y-%m-%dT%H:%M)" - "${lo:--}" urgent no-early "audit=$n" || continue
    aput "$n" "$sha" "$nr" "${af:-0}" queued 0
    echo "감사 넣음 $an ← $n $sha (${nr}번째, $m)"
  done 6<<<"$LROWS"
}
# audit_reap: 판정을 레인에 보낸(pass·fix) 감사 터미널을 끈다(hc kill <정확한 주소>) — 감사 터미널이 레인 작업 트리를 cwd로 쓰고
# 남으면 레인의 hc-lane clean이 멈춘다(2026-11-19 지휘 실측: 선행 착지 뒤 감사 터미널 때문에 정리 멈춤). 끄는 것은 이름이 그 감사
# 줄 이름이고 done·idle이며 레인 기록(hc-lane lanes.tsv) 종류가 read인 터미널만. working·blocked면 다음 바퀴에. 같은 이름의 지난
# 바퀴 감사 터미널(queued인데 지금 감사 줄 주소가 아님)도 끈다. 「지휘로 넘김」(coord)은 지휘가 보도록 남긴다
audit_reap() {
  [ -n "$AUD" ] && [ -s "$Q" ] || return 0
  lt=${XDG_STATE_HOME:-$HOME/.local/state}/hc-lane/lanes.tsv
  while IFS=$'\t' read -r -u 6 an am aa k; do   # am: 감사 줄 기계(5칸 --only) — 다른 기계의 같은 이름 터미널은 안 끔
    n=${k#audit=}
    ast=$(aget "$n" | cut -f 5)
    printf '%s\n' "$TERMS" | awk -F'\t' -v x="$an" -v m="$am" '$1==m && ($4 "")==x &&($5 == "done" || $5 == "idle") {print $2}' | while read -r ta; do
      case $ast in pass | fix) ;; queued) [ "$ta" != "$aa" ] || continue ;; *) continue ;; esac
      [ "$(awk -F'\t' -v a="$ta" -v x="$an" '$1==a && ($2 "")==x {k=$4} END {print k}' "$lt" 2>/dev/null)" = read ] || continue   # 주소+이름(주소 재사용)
      if [ $dry = 1 ]; then echo "${P}끌 감사 터미널 $an $ta"
      elif hc kill "$ta" >/dev/null 2>&1 </dev/null; then echo "감사 터미널 끔 $an $ta (판정 보낸 뒤)"
      else echo "감사 터미널 끄기 실패 $an $ta — 다음 바퀴에 다시"; fi
    done
  done 6<<<"$(awk -F'\t' -v OFS='\t' '$15 ~ /^audit=/ {print $1, $5, $8, $15}' "$Q")"
}

# mgres <레인> <제출 ID>: 병합기 기계(MGM)가 그 레인 이름·그 제출 ID로 적은 마지막 결과(착지·막힘) 사건 줄, 없으면 빈 값
mgres() {
  [ -n "$2" ] || return 0
  printf '%s\n' "$EV" | awk -F'\t' -v n="$1" -v M="$MGM" -v sid="$2" \
    'BEGIN { k = split(M, a, " "); for (i = 1; i <= k; i++) m[a[i]] = 1 }
     ($4 "") == n && ($5 == "착지" || $5 == "막힘") && ($2 in m) && index($7, "제출 ID " sid) { r = $0 } END { print r }'
}
ev_tick() {
  # 쓰기 소유 실행 기록(OREC)의 기계도 읽는다 — rm·sent 뒤에도 살아 있는 레인의 착지를 보려고(아스트라 2차 중간 4)
  eups=$(awk -F'\t' -v OFS='\t' '$6=="up" {print $1, $7, $8, $17}' "$Q" 2>/dev/null) eoms=$(printf '%s\n' "$OREC" | cut -f 3)
  [ -n "$eups$eoms" ] || return 0
  set --
  # 병합기 기계도 읽는다: 제출한 레인의 착지·막힘은 병합기 터미널이 --name <레인>으로 적는다(병합기 1차 중간 5)
  MGM=
  for p_ in $( { awk -F'\t' '$6=="up" {print $2}' "$Q" 2>/dev/null; printf '%s\n' "$OREC" | cut -f 2; } | awk 'NF' | sort -u); do MGM="$MGM $(mgmach "$p_")"; done
  for m in $( { printf '%s\n' "$eups" | cut -f 2; printf '%s\n' "$eoms" $MGM; } | awk 'NF' | sort -u); do
    [ "$m" != "$LOCAL" ] && printf '%s\n' "$CONN" | grep -Fxq -- "$m" && set -- "$@" --on "$m"
  done
  EV=$(hc-lane events "$@" 2>/dev/null </dev/null) || { EVBAD=1; echo "레인 사건 읽기 실패(hc-lane events) — 다음 바퀴에"; return 0; }
  [ $dry = 0 ] || return 0
  eunk=$(printf '%s\n' "$EV" | sed -n 's/^모름 //p') enow=$(date +%H:%M) LRT=$DATA/last.tsv
  # 여덟째 칸 = 대기열 줄 ID(기존 6 — 이름만으로 지난 줄을 남기면 rm·다시 add한 같은 이름 새 줄에 옛 실행의 착지 sha가 붙었다):
  # 끊긴 기계의 지난 값은 줄 ID가 같을 때만 남긴다. ponytail: 같은 주소·같은 이름의 옛 사건은 여전히 lastev가 고른다(주소 재사용 — 드묾)
  while IFS=$'\t' read -r n m a r; do
    [ -n "$n" ] || continue   # up 줄이 없고 실행 기록만 있는 바퀴
    if [ "$m" != "$LOCAL" ] && { ! printf '%s\n' "$CONN" | grep -Fxq -- "$m" || printf '%s\n' "$eunk" | grep -Fxq -- "$m"; }; then
      awk -F'\t' -v OFS='\t' -v n="$n" -v r="$r" '($1 "")==n && ($8 "")==r { $7 = "old"; print; exit }' "$LRT" 2>/dev/null; continue
    fi
    l=$(lastev "$n" "$a")
    if [ -z "$l" ]; then printf '%s\t-\t-\t-\t-\t%s\tfresh\t%s\n' "$n" "$enow" "$r"; continue; fi
    # 제출한 레인(마지막 사건 「제출」)은 병합기 기계가 그 제출 ID로 적은 결과(착지·막힘)가 있으면 그것을 보인다(병합기 3차 중간 10 —
    # ls가 제출자 터미널만 읽어 최종 결과를 놓쳤다)
    if [ "$(printf %s "$l" | cut -f 5)" = 제출 ]; then
      r_=$(mgres "$n" "$(printf %s "$l" | cut -f 7 | awk '{ print $1 }')")
      [ -z "$r_" ] || l=$r_
    fi
    es=$(date -j -u -f %Y-%m-%dT%H:%M:%SZ "$(printf %s "$l" | cut -f 1)" +%s 2>/dev/null) && eh=$(date -r "$es" +%H:%M) || eh=?
    est=$(printf %s "$l" | cut -f 5) esha=$(printf %s "$l" | cut -f 6 | cut -c 1-8)
    emsg=$(printf %s "$l" | cut -f 7 | LC_ALL=en_US.UTF-8 cut -c 1-40)   # UTF-8 글자 단위(launchd처럼 LANG이 없어도 글자를 쪼개지 않게)
    printf '%s\t%s\t%s\t%s\t%s\t%s\tfresh\t%s\n' "$n" "${est:--}" "${esha:--}" "$eh" "${emsg:--}" "$enow" "$r"
  done <<<"$eups" >"$LRT.tmp" && mv -f "$LRT.tmp" "$LRT"
}

# 주차(2026-11-20 fill-existing-defects, 사용자 「노는 워크트리 없이」): 먼저 띄운(early) up 줄의 터미널이 그 이름으로 마지막에 「주차」를
# 적었으면(hc-lane park — 가지는 원격 lane/<레인>, 임대는 놓음) 그 줄을 대기(wait)·먼저 칸 parked로 되돌린다 — 자리를 비우고, 선행이
# 들어가면 아래 바퀴가 hc-lane --resume으로 그 가지에서 다시 띄운다. 끊긴·못 읽은 기계는 다음 바퀴에(사건을 모름)
park_tick() {
  [ -z "$EVBAD" ] && [ -s "$Q" ] || return 0
  punk=$(printf '%s\n' "$EV" | sed -n 's/^모름 //p')
  while IFS=$'\t' read -r -u 6 n m a st r; do
    [ -n "$n" ] || continue
    [ "$m" = "$LOCAL" ] || { printf '%s\n' "$CONN" | grep -Fxq -- "$m" && ! printf '%s\n' "$punk" | grep -Fxq -- "$m"; } || continue
    l=$(lastev "$n" "$a"); [ "$(printf %s "$l" | cut -f 5)" = 주차 ] || continue
    if [ $dry = 1 ]; then echo "${P}주차로 되돌릴 $n ($a)"; continue; fi
    QRID=$r qset "$n" wait - - "$st" parked && echo "주차 $n: $a 가 lane/$n ($(printf %s "$l" | cut -f 6 | cut -c 1-8))에 두고 끝냄 — 자리 비움, 선행 착지 뒤 그 가지에서 이어 띄움"
  done 6<<<"$(awk -F'\t' -v OFS='\t' '$6=="up" && $14=="early" {print $1, $7, $8, $9, $17}' "$Q")"
}

wcnt() { printf '%s\n' "$WC" | awk -F'\t' -v n="$1" '($1 "")==n {print $2; f=1; exit} END {if (!f) print 0}'; }
