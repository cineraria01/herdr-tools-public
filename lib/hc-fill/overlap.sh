# hc-fill 함수: overlap (bin/hc-fill가 읽는다. 함수는 bin/hc-fill 안 원래 정의 순서 그대로)

# kpre <프로젝트>: 그 프로젝트 repo 줄 경로(모든 기계, 끝 / 뗌) 한 줄에 하나
kpre() { awk -v p="$1" '$1=="repo" && $2==p { x = $0; sub(/^[ \t]*repo[ \t]+[^ \t]+[ \t]+[^ \t]+[ \t]+/, "", x); sub(/[ \t]+$/, "", x); sub(/\/+$/, "", x); print x }' "$CONF"; }
# ktok <R|M>: 표준 입력 글에서 키 후보 「R|M<TAB>id<TAB>FR ID」 · 「R<TAB>p<TAB>경로」 · 「M<TAB>m<TAB>경로<TAB>조사 뗀 경로」(정규화한 저장소
# 상대 경로, 있는지는 lkeys가 본다). 환경 KPRE_ = kpre 출력
ktok() {
  KK_=$1 awk '
    function norm(t,   abs, dir, n, a, i, o, b, r, k, m, P, p, hit) {
      if (t == "" || t ~ /^~/ || index(t, "://")) return ""
      abs = (substr(t, 1, 1) == "/"); dir = (t ~ /\/$/)
      n = split(t, a, "/"); o = 0
      for (i = 1; i <= n; i++) { if (a[i] == "" || a[i] == ".") continue
        if (a[i] == "..") { if (o == 0) return ""; o--; continue }
        b[++o] = a[i] }
      r = ""; for (i = 1; i <= o; i++) r = r (i > 1 ? "/" : "") b[i]
      if (abs) { r = "/" r; hit = 0; m = split(ENVIRON["KPRE_"], P, "\n")
        for (k = 1; k <= m; k++) { p = P[k]; if (p == "") continue
          if (index(r "/", p "/") == 1) { r = substr(r, length(p) + 2); hit = 1; break }
          if (index(r, p ".wt/") == 1) { r = substr(r, length(p) + 5); if (!sub(/^[^\/]+\//, "", r)) return ""; hit = 1; break } }
        if (!hit) return "" }
      if (r == "" || r ~ /^[-._\/]+$/) return ""
      return r (dir ? "/" : "") }
    function unwrap(t) {   # 앞뒤 괄호·따옴표·문장부호를 뗌(바이트 단위 — 「」『』。는 여러 바이트라 따로)
      while (1) { if (sub(/^[`"\047(\[{<]/, "", t) || sub(/^(「|『|“|‘)/, "", t)) continue
        if (sub(/[`"\047)\]}>,;:!?]$/, "", t) || sub(/(」|』|。|”|’|，)$/, "", t)) continue
        if (t ~ /[^.]\.$/ && t !~ /\/\.$/) { sub(/\.$/, "", t); continue }
        break }
      return t }
    function frid(t) { return match(t, /^FR-[A-Z0-9]+(-[A-Z0-9]+)*/) ? substr(t, 1, RLENGTH) : "" }
    # 예약: 괄호 안(설명·참고 경로)을 먼저 지우고(겹 괄호는 되풀이) 항목으로 나눈 뒤, 항목 첫 경로만 — 백틱으로 시작하면 그 안, 아니면
    # 첫 낱말. 낱말 하나뿐인 항목은 p(새 파일도 받음), 뒤에 설명이 붙은 항목은 q(lkeys가 더 좁게 받음) — 아스트라 2차 중간 5·6
    ENVIRON["KK_"] == "R" {
      x0 = $0; while (gsub(/\([^()]*\)/, "", x0)) ;
      n = split(x0, it, /,|·|、|;|，/)
      for (i = 1; i <= n; i++) { x = it[i]; sub(/^[ \t]+/, "", x); sub(/[ \t]+$/, "", x); if (x == "") continue
        if (match(x, /^`[^`]+`/)) { x = substr(x, 2, RLENGTH - 2); ty = "p" }
        else { ty = (split(x, w, /[ \t]+/) == 1) ? "p" : "q"; x = unwrap(w[1]) }
        if ((f = frid(x)) != "" && f == x) { print "R\tid\t" f; continue }
        if ((p = norm(x)) != "") print "R\t" ty "\t" p }
      next }
    { gsub(/`/, " "); gsub(/·|、|，|,|;/, " ")   # `경로`을 → 경로 · 을, 가운뎃점·쉼표 목록은 낱말마다(2차 중간 6)
      for (i = 1; i <= NF; i++) { x = unwrap($i)
        if ((f = frid(x)) != "") { print "M\tid\t" f; continue }
        if (x !~ /[A-Za-z0-9]/) continue
        s = x; if (match(s, /[A-Za-z0-9_.\/)-](에서|으로|까지|부터|처럼|에게|을|를|이|가|은|는|에|의|와|과|도|로|만)$/)) s = unwrap(substr(s, 1, RSTART))
        p = norm(x); q = norm(s); if (q == "") q = p
        if (p != "" || q != "") print "M\tm\t" (p == "" ? q : p) "\t" q } }'
}
# lkeys <프로젝트> <지시문>: 그 레인의 키 「R|M<TAB>키」(같은 키는 R이 앞)
lkeys() {
  lr_=$(repo_of "$1" "$LOCAL"); lr_=${lr_%/}
  kp_=$(kpre "$1")
  igs_=$(awk -v p="$1" '$1=="overlap_ignore" && $2==p { for (i = 3; i <= NF; i++) print $i }' "$CONF")
  ex_=$(sed -n -E 's/^[[:space:]]*([-*][[:space:]]*)?(파일|쓰기):(.*)/\3/p' "$2" 2>/dev/null)
  { [ -z "$ex_" ] || printf '%s\n' "$ex_" | KPRE_=$kp_ ktok R; KPRE_=$kp_ ktok M <"$2"; } |
    while IFS=$'\t' read -r k_ y_ v_ w_; do
      vs_=   # 맨 이름이 확장자 붙은 맨 위 파일로 풀리면 그 이름들(README → README.md, 2차 낮음 1) — 아래 overlap_ignore를 같이 거친다
      case $y_ in
        p | q)   # 예약 경로: / 가 있으면 그대로, 맨 위 실제 파일·폴더면 그것(폴더면 끝 /), 없으면 확장자 붙은 맨 위 파일, 그것도 없으면
                 # p(낱말 하나뿐인 항목)는 새 파일(main.c·.newconfig·NewTool — 2차 중간 5), q(뒤에 설명이 붙은 항목의 첫 낱말)는 확장자나
                 # 점 파일 꼴일 때만(설명 낱말 e.g·see가 키가 되지 않게 — hc-fill 감사 낮음 1)
          case $v_ in */*) ;; *)
            if [ -z "$lr_" ] || [ ! -e "$lr_/$v_" ]; then
              [ -z "$lr_" ] || for f_ in "$lr_/$v_".*; do [ ! -f "$f_" ] || vs_="$vs_${f_##*/}
"; done
              [ -n "$vs_" ] || [ "$y_" = p ] || case $v_ in ??*.[A-Za-z0-9]* | .[A-Za-z0-9]?*) ;; *) continue ;; esac
            fi ;; esac
          [ -z "$lr_" ] || [ ! -d "$lr_/$v_" ] || v_=${v_%/}/ ;;
        m)   # 언급: 있는 것(조사 붙은 그대로 → 뗀 것), 없으면 첫 폴더가 있는 새 경로. 맨 위 낱말은 실제 파일만(Makefile)
          [ -n "$lr_" ] || continue
          if [ -e "$lr_/$v_" ]; then :; elif [ -e "$lr_/$w_" ]; then v_=$w_
          else v_=$w_; case $v_ in */?*) [ -d "$lr_/${v_%%/*}" ] || continue ;; *) continue ;; esac; fi
          case $v_ in */*) ;; *) [ -f "$lr_/$v_" ] || continue ;; esac
          [ ! -d "$lr_/$v_" ] || v_=${v_%/}/ ;;
      esac
      printf '%s\n' "${vs_:-$v_}" | while IFS= read -r v_; do
        [ -n "$v_" ] || continue
        ig_=; while IFS= read -r g_; do [ -n "$g_" ] || continue; case $v_ in $g_) ig_=1; break ;; esac; done <<<"$igs_"
        [ -n "$ig_" ] || printf '%s\t%s\n' "$k_" "$v_"
      done
    done | awk -F'\t' '!s[$2]++'
}
# ovcmp <소유 줄(레인 R|M 키)>: 표준 입력의 후보 키와 견줘 앞 레인마다 「앞레인<TAB>예약|언급<TAB>키 둘까지」(예약 = 둘 다 R인 겹침)
ovcmp() {
  OWN_=$1 awk -F'\t' '
    function hit(a, b) { return a == b || (a ~ /\/$/ && index(b, a) == 1) || (b ~ /\/$/ && index(a, b) == 1) }
    $2 == "" { next }   # 빈 키(키 없는 지시문의 빈 줄)는 겹침이 아니다
    { ck[++n] = $2; cr[n] = ($1 == "R") }
    END { m = split(ENVIRON["OWN_"], L, "\n")
      for (i = 1; i <= m; i++) { if (split(L[i], f, "\t") < 3 || f[3] == "") continue
        for (j = 1; j <= n; j++) { if (!hit(f[3], ck[j])) continue
          o = f[1]; if (!(o in seen)) { seen[o] = 1; ord[++no] = o }
          if (f[2] == "R" && cr[j]) { r = 1; res[o] = 1 } else r = 0
          if (!index("," ks[r, o] ",", "," ck[j] ",") && c[r, o] < 2) { ks[r, o] = ks[r, o] (c[r, o] ? "," : "") ck[j]; c[r, o]++ } } }
      for (k = 1; k <= no; k++) { o = ord[k]; print o "\t" (res[o] ? "예약" : "언급") "\t" (res[o] ? ks[1, o] : ks[0, o]) } }'
}
# 쓰기 소유 = 실행 기록 OREC(own.tsv, 위에서 불러 옴). 이름·주소로 소유를 추정하지 않고, 모르면 유지한다(2026-11-20 지휘).
# 적음: 쓰기 터미널 주소를 받은 때(place — 상태 안 잡힘으로 남긴 터미널도, 아스트라 2차 높음 1) · 대기열 쓰기 줄(up·wait·sent)과 같은 이름의
#   살아 있는 터미널(손으로 띄움 — 레인 기록이 read인 주소 빼고, 2차 중간 4) · own.tsv가 없거나 옛 꼴이면 up 줄 주소와 끊긴 기계의 남긴
#   주소(hc-fill 감사 낮음 2). 시작 시각은 받은 때, 입양·채움은 그 줄을 넣은 시각.
# 지움: 그 기계가 연결된 채 그 주소에 그 이름 터미널이 없음(병합기 큐에 있거나 큐를 못 읽으면 유지) · 그 터미널이 시작 뒤 착지를 적음
#   (그 전 시각의 착지는 같은 이름·주소의 지난 실행 — 2차 중간 2) · 병합기 기계가 그 이름으로 착지를 적음. 기계가 연결 안 됨이면 유지.
#   대기열 rm·sent와 상관없다. overlap off여도 맞춘다(다시 켤 때 낡은 기록으로 넘어가지 않게)
utcnow() { date -u +%Y-%m-%dT%H:%M:%SZ; }
utc_of() {  # utc_of <대기열 더한시각(이 기계 분)>: UTC. 못 읽으면 ~(어떤 사건 시각보다 뒤 — 착지로 풀지 않음)
  s_=$(date -j -f %Y-%m-%dT%H:%M:%S "$1:00" +%s 2>/dev/null) && date -u -r "$s_" +%Y-%m-%dT%H:%M:%SZ || echo '~'
}
own_rel() {  # own_rel <레인> <주소>: 그 기록과 지시문 사본을 지운다(dry-run은 사본을 안 지움)
  x_=$(printf '%s\n' "$OREC" | awk -F'\t' -v n="$1" -v a="$2" '($1 "")==n && $4==a {print $6}')
  OREC=$(printf '%s\n' "$OREC" | awk -F'\t' -v n="$1" -v a="$2" 'NF && !(($1 "")==n && $4==a)')
  [ $dry = 0 ] || return 0
  printf '%s\n' "$x_" | while IFS= read -r f_; do case $f_ in "$OWND"/*) rm -f -- "$f_" ;; esac; done
}
own_add() {  # own_add <레인> <프로젝트> <기계> <주소> <시작UTC> <지시문> <선행|-> <겹침칸>: 지시문을 $OWND에 복사해 기록(dry-run은 복사 없이)
  c_=$6
  if [ $dry = 0 ]; then
    if mkdir -p "$OWND" && c_=$(mktemp "$OWND/$1.XXXXXX") && cat -- "$6" >"$c_" 2>/dev/null; then :
    else [ -z "$c_" ] || [ "$c_" = "$6" ] || rm -f -- "$c_"; c_=$6; echo "알림: 쓰기 소유 $1 지시문 사본을 못 만듦 — 원본으로 봄($6)"; fi
  fi
  own_rel "$1" "$4"
  OREC="$OREC${OREC:+
}$1	$2	$3	$4	$5	$c_	,$7,	${8:--}"
}
own_done() {  # own_done <레인> <주소> <시작UTC>: 그 실행이 착지했으면 0. 사건을 못 읽었으면 1(유지)
  [ -z "$EVBAD" ] || return 1
  lx_=$(lastev "$1" "$2")
  [ "$(printf %s "$lx_" | cut -f 5)" = 착지 ] && ! [ "$(printf %s "$lx_" | cut -f 1)" \< "$3" ] && return 0
  # 그 터미널이 아직 사건을 하나도 안 적었으면(비교 시각 없음) 같은 이름의 옛 착지로 풀지 않는다(병합기 2차 중간 6).
  # 마지막 사건이 「제출」이면 그 제출 ID(한 줄 첫 낱말)가 든 병합기 착지로도 푼다 — 시각과 상관없이(3차 중간 11 — 같은 초·접수
  # 확인 전에 병합기가 끝나면 「뒤」가 아니라 소유가 남았다)
  printf '%s\n' "$EV" | awk -F'\t' -v n="$1" -v t="$(printf %s "$lx_" | cut -f 1)" -v M="$MGM" \
    -v sid="$(printf %s "$lx_" | awk -F'\t' '$5 == "제출" { split($7, w, " "); print w[1] }')" \
    'BEGIN { k = split(M, a, " "); for (i = 1; i <= k; i++) m[a[i]] = 1 }
     t != "" && ($4 "") == n && $5 == "착지" && ($2 in m) && ($1 > t || (sid != "" && index($7, "제출 ID " sid))) { f = 1 } END { exit !f }'
}

tmap() { printf '%s\n' "$THENMAP" | awk -F'\t' -v n="$1" '($1 "")==n {print $2; f = 1; exit} END {if (!f) print n}'; }   # 이어 붙인 이름이면 받은 줄
# owners <후보> <프로젝트>: 같은 프로젝트의 쓰기 소유 레인 가운데 자기 아니고 이 후보를 (--after 사슬·--then을 건너서라도) 기다리지 않는 것
# 「레인<TAB>겹침칸」 — 기다리는 소유 레인은 후보 착지 뒤에 일하니 막지 않는다(먼저 띄운 후속이 사슬 끝 선행을 막아 서로 기다리지 않게)
owners() {
  printf '%s\n' "$OREC" | QE_=$QEDGES TM_=$THENMAP awk -F'\t' -v c="$1" -v p="$2" '
    function waits(o,   q, h, t, x, a, m, i, k, seen) {
      h = 1; t = 1; q[1] = o; seen[o] = 1
      while (h <= t) { x = q[h++]; m = split(A[x], a, ",")
        for (i = 1; i <= m; i++) { k = a[i]; if (k == "" || k == "-") continue; if (k in T) k = T[k]
          if (k == c) return 1
          if (!(k in seen)) { seen[k] = 1; q[++t] = k } } }
      return 0 }
    BEGIN { n = split(ENVIRON["QE_"], L, "\n"); for (i = 1; i <= n; i++) if (split(L[i], f, "\t") >= 2) A[f[1]] = A[f[1]] f[2]
      n = split(ENVIRON["TM_"], L, "\n"); for (i = 1; i <= n; i++) if (split(L[i], f, "\t") >= 2) T[f[1]] = f[2] }
    NF >= 7 { A[$1] = A[$1] $7; R[++nr] = $1 "\t" $2 "\t" $8 }
    END { for (i = 1; i <= nr; i++) { split(R[i], f, "\t"); if (f[2] != p || f[1] == c || (f[1] in S)) continue; S[f[1]] = 1
        if (!waits(f[1])) print f[1] "\t" f[3] } }'
}

# ovl_check <레인> <프로젝트> <지시문> <선행|-> <감사칸> <겹침칸> <기계제한>: 0 띄워도 됨(경고는 OVWARN에 — 띄운 뒤 ovl_placed가 찍음),
# 1 겹침 대기. 견줄 소유: owners(같은 프로젝트, 자기 아님, 이 후보를 기다리지 않음). 단독(겹침칸 solo — 2026-11-20 지휘 t1cj): 단독 후보는
# 다른 쓰기 소유가 있으면, 다른 후보는 단독 소유가 있으면 파일과 상관없이 대기(설정 overlap과 상관없이, --overlap-ok 줄도)
ovl_check() {
  OVWARN= OVKEYS=
  case ${5:--} in audit=*) return 0 ;; esac   # 읽기 감사는 통과
  ovo_=$(owners "$1" "$2")
  if [ "${6:--}" = solo ]; then so_=$(printf '%s\n' "$ovo_" | awk -F'\t' 'NF {print $1; exit}')
  else so_=$(printf '%s\n' "$ovo_" | awk -F'\t' '$2=="solo" {print $1; exit}'); fi
  if [ -n "$so_" ]; then
    echo "단독 대기 $1: $so_ — 앞 레인이 끝나면 뜬다"
    OVWAIT="$OVWAIT$1	$so_	단독	${7:--}	$2
"; return 1
  fi
  [ "$OVM" != off ] && [ "${6:--}" = - ] && [ -n "$ovo_" ] || return 0
  ow_=$(printf %s "$OWN" | OVO_=$ovo_ awk -F'\t' -v OFS='\t' -v p="$2" '
    BEGIN { n = split(ENVIRON["OVO_"], L, "\n"); for (i = 1; i <= n; i++) if (split(L[i], f, "\t") >= 1 && f[1] != "") ok[f[1]] = 1 }
    $1==p && ($2 in ok) {print $2, $3, $4}')
  [ -n "$ow_" ] || return 0
  OVKEYS=$(lkeys "$2" "$3")
  ov_=$(printf '%s\n' "$OVKEYS" | ovcmp "$ow_")
  [ -n "$ov_" ] || return 0
  if [ "$OVM" = block ] && bl_=$(printf '%s\n' "$ov_" | awk -F'\t' '$2=="예약" {print $1 "\t" $3; exit}') && [ -n "$bl_" ]; then
    echo "겹침 대기 $1: ${bl_%%	*} (${bl_#*	}) — 앞 레인이 착지하면 뜬다"
    OVWAIT="$OVWAIT$1	$bl_	${7:--}	$2
"; return 1
  fi
  OVWARN=$ov_
}
# ovl_placed <레인> <프로젝트> <지시문> <선행|-> <감사칸>: 주소를 받은 쓰기 줄(실행 기록은 place가 적었다)의 키를 소유에 더하고(감사 줄은
# 아님) 경고를 찍고 적는다(dry-run은 안 적음)
ovl_placed() {
  [ "$OVM" != off ] || return 0
  case ${5:--} in audit=*) return 0 ;; esac
  [ -n "$OVKEYS" ] || OVKEYS=$(lkeys "$2" "$3")
  [ -z "$OVKEYS" ] || OWN="$OWN$(printf '%s\n' "$OVKEYS" | sed "s|^|$2	$1	|")
"
  [ -n "$OVWARN" ] || return 0
  printf '%s\n' "$OVWARN" | while IFS=$'\t' read -r wo wk wf; do
    echo "겹침 경고 $1: $wo $wk ($wf)"
    [ $dry = 1 ] || printf '%s\t%s\t%s\t%s\t%s\n' "$(date +%Y-%m-%dT%H:%M)" "$1" "$wo" "$wk" "$wf" >>"$DATA/overlap-warn.log"
  done
}

nd_block() {
  [ -n "$NEED" ] || return 1
  for nm_ in $(machines | awk '{print $1}'); do
    [ "$2" = - ] || inlist "$nm_" "$2" || continue
    mhas "$nm_" "$NEED" && return 1
  done
  nl_="그 능력을 가진 기계가 없다(설정 machine-has)"; [ "$2" = - ] || nl_="기계 제한 $2 안에 $nl_"
  echo "막힘 $1: 필요 $NEED — $nl_. 대기로 둠"
  ns_=0
  if printf '%s\n' "$NDOLD" | grep -Fxq -- "$1	$NEED	1"; then ns_=1
  elif [ $dry = 0 ] && aesc "$1" "$3" "$1: 필요 $NEED — $nl_. 설정에 능력을 더하거나 지시문의 「필요:」 줄을 고친다(hc-fill ls)"; then ns_=1; fi
  NDNOW="$NDNOW$1	$NEED	$ns_
"
  return 0
}
# snap <레인> <지시문>: 바퀴 처음에 읽은 줄(ROWS)과 지금 대기열 줄을 견줘(1-5칸·10칸부터 — 상태·주소 칸은 이 바퀴가 바꾼다) 같으면
# 지시문을 이번 바퀴 사본 $SNAPD/<레인>.md로 떠 SB에 — 겹침 검사·띄우기·실행 기록이 모두 이 사본을 쓴다. 바퀴 도중 rm·add로 줄이
# 바뀌었으면 1(다음 바퀴에 새 줄로 다시 판정 — 아스트라 2차 중간 1)
snap() {
  o_=$(printf '%s\n' "$ROWS" | awk -F'\t' -v n="$1" '($1 "")==n {print; exit}' | cut -f 1-5,10-)
  c_=$(awk -F'\t' -v n="$1" '($1 "")==n {print; exit}' "$Q" 2>/dev/null | cut -f 1-5,10-)
  [ "$o_" = "$c_" ] || { echo "대기열 줄이 바뀌었다 $1 — 다음 바퀴에 다시 판정"; return 1; }
  [ -n "$SNAPD" ] || SNAPD=$(mktemp -d "${TMPDIR:-/tmp}/hc-fill-snap.XXXXXX") || { echo "실패 $1: 지시문 사본 폴더를 못 만듦"; return 1; }
  SB=$SNAPD/$1.md
  cat -- "$2" >"$SB" 2>/dev/null && grep -q '[^[:space:]]' -- "$SB" || { echo "실패 $1: 지시문 사본을 못 뜸: $2"; return 1; }
}

# 선행 상태(2026-11-19 fill-dep-needs 사용자 「① 선행 사슬 기다림 (추천)」 — 먼저 띄움 레인은 선행이 살아 진행 중이어도 기한에 「선행 대기
# 초과」로 멈춰 지휘를 불렀다, 막힘 31건 중 11건): 떠 있는 먼저 띄움 줄(up·early)의 기계마다 그 줄 선행의 판정을
# ~/.local/share/hc-fill/after.tsv(선행 go|stop 까닭 쓴시각(초))로 원자 쓰기 — 대기열은 이 기계에만 있어 레인 기계의 hc-lane early-wait는
# 이 파일을 본다. 판정 차례(pred_state → PSV): 들어감 go · 대기열에 없음 stop(착지 안 하고 끝남 — 지운 줄) · 대기 go · up 아님(sent 등) stop ·
# 기계 연결 안 됨 go(모름) · 그 주소에 그 이름 터미널 없음 stop · done·idle 아님(working 등) go · 자동 감사 queued|pass|fix go ·
# 그 터미널의 마지막 사건 완료·착지·제출 go, 막힘 stop · 사건 없이 쉼: hold분 안 go, 넘으면 stop. 생존은 이 바퀴 hc ls(TERMS — 이름 대조)로
# 본다(원격 임대 refs/hc-lanes/<레인>은 끝난 뒤 clean 전까지 남아 「살아 있음」으로 못 쓴다). fetch 실패면 들어감 판정만 건너뛴다. dry-run은 찍기만
pred_state() {  # pred_state <선행> <기다리는 줄 프로젝트>: PSV=「go|stop<TAB>까닭」
  if fetch_proj "$2" >/dev/null; then
    h_=$(landed "$1")
    [ -n "$h_" ] || ! st_=$(since "$1") || ! lmark "$1" "$st_" || h_="$(printf %s "$LMS" | cut -c 1-8) 착지 표식"
    [ -z "$h_" ] || { PSV="go	들어감(${h_%% *})"; return; }
  fi
  r_=$(awk -F'\t' -v n="$1" '($1 "")==n {print; exit}' "$Q")
  [ -n "$r_" ] || { tf_=$(thenof "$1"); [ -z "$tf_" ] || r_=$(awk -F'\t' -v n="$tf_" '($1 "")==n {print; exit}' "$Q"); }   # --then 이름
  [ -n "$r_" ] || { PSV="stop	대기열에 없음(착지 안 하고 끝남)"; return; }
  IFS=$'\t' read -r pn_ _ _ _ _ pq_ pm_ pa_ _ <<<"$r_"
  case $pq_ in wait) PSV="go	대기열에서 기다림(아직 안 뜸)"; return ;; up) ;; *) PSV="stop	대기열 상태 $pq_(떠 있지 않음)"; return ;; esac
  [ "$pm_" = "$LOCAL" ] || printf '%s\n' "$CONN" | grep -Fxq -- "$pm_" || { PSV="go	모름($pm_ 연결 안 됨)"; return; }
  lalive "$pa_" "$pn_" || { PSV="stop	터미널 없음($pa_)"; return; }
  ts_=$(alive "$pa_")
  case $ts_ in done | idle) ;; *) PSV="go	진행 중($ts_)"; return ;; esac
  case $(aget "$pn_" | cut -f 5) in
    queued) PSV="go	자동 감사 중"; return ;; pass) PSV="go	감사 통과 — 착지 중"; return ;; fix) PSV="go	감사 지적 고치는 중"; return ;;
  esac
  if [ -z "$EVBAD" ]; then
    l_=$(lastev "$pn_" "$pa_")
    case $(printf %s "$l_" | cut -f 5) in
      완료) PSV="go	완료 보고(감사·착지 기다림)"; return ;; 착지 | 제출) PSV="go	$(printf %s "$l_" | cut -f 5) 보고"; return ;;
      막힘) PSV="stop	막힘: $(printf %s "$l_" | cut -f 7)"; return ;;
    esac
  fi
  hs_=$(printf '%s' "$HNEW" | awk -F'\t' -v a="$pa_" '$1==a {print $2; exit}'); case $hs_ in '' | *[!0-9]*) hs_=$HNOW ;; esac
  hm_=$(((HNOW - hs_) / 60))
  if [ "$HOLD" != 0 ] && [ $((HNOW - hs_)) -ge $((HOLD * 60)) ]; then PSV="stop	쉰 지 ${hm_}분 — 사건 없음(hold ${HOLD}분)"
  else PSV="go	쉼(${hm_}분, 사건 없음)"; fi
}
after_tick() {
  [ -s "$Q" ] || return 0
  atn_=$(date +%s)
  for am_ in $(awk -F'\t' '$6=="up" && $14=="early" {print $7}' "$Q" | sort -u); do
    [ "$am_" = "$LOCAL" ] || printf '%s\n' "$CONN" | grep -Fxq -- "$am_" || { echo "선행 상태 미룸 @$am_: 연결 안 됨"; continue; }
    ab_=
    while IFS=$'\t' read -r ap_ aa_; do
      for x_ in $(printf %s "$aa_" | tr , ' '); do
        printf '%s' "$ab_" | cut -f 1 | grep -Fxq -- "$x_" && continue
        pred_state "$x_" "$ap_"
        ab_="$ab_$x_	$PSV	$atn_
"
      done
    done <<<"$(awk -F'\t' -v OFS='\t' -v m="$am_" '$6=="up" && $14=="early" && $7==m && $4!="-" {print $2, $4}' "$Q")"
    [ -n "$ab_" ] || continue
    if [ $dry = 1 ]; then printf '%s' "$ab_" | cut -f 1-3 | sed "s|^|${P}선행 상태 @$am_: |"; continue; fi
    printf '%s' "$ab_" | onm -i "$am_" 'mkdir -p "${d%/*}" && cat >"$d.tmp" && mv -f "$d.tmp" "$d"' "~/.local/share/hc-fill/after.tsv" ||
      echo "선행 상태 쓰기 실패 @$am_ — 다음 바퀴에"
  done
}
