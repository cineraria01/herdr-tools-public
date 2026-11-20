# hc-lane 함수: watch (bin/hc-lane가 읽는다. 함수는 bin/hc-lane 안 원래 정의 순서 그대로)

# emit <사건 줄들>: 커서에 적고 찍는다. watch는 먼저 지휘 터미널에 한 줄로 밀어 주고(hc send), 보낸 뒤에만 커서에 적는다
# (못 보냈으면 1 — 다음 바퀴에 다시. 지휘가 바빠도 hc send는 입력에 쌓인다)
emit() {
  if [ $ec = watch ]; then
    set -- "$(wfilter "$1")"
    [ -n "$1" ] || return 3   # 이 지휘의 사건이 없음(남의 것은 커서에 안 적어 그 지휘·wait 몫으로 남긴다)
    m_=$(printf '%s\n' "$1" | LC_ALL=C awk -F'\t' '{ s = $4 " " $5 ($6 != "-" ? " " $6 : "") " " $7 " (" $3 ")"; o = o (NR > 1 ? " ‖ " : "") s }
      END { print "hc-lane: " o }' | LC_ALL=C tr -d '\000-\037\177')   # 제어 문자(ESC·Ctrl-C 등)는 지휘 입력에 넣지 않는다
    wsend "$m_" || return 1
  fi
  k_=$(printf '%s\n' "$1" | LC_ALL=C awk -F'\t' "$evkey$nkf"'{ print nk(key()) }')
  printf '%s\n' "$k_" >>"$cur" || die "$ec: 커서 파일을 쓰지 못했다"
  # 「보냄」 표시(<evdir>/sent)는 watch만 쓴다 — 사건 하나에 한 번, 주인 watch가 보냈으면 끝(4차 [중간] 1). 끌어오는 wait는
  # 자기 커서만: 남의 지휘 wait가 본 사건도 주인 watch는 보낸다(5차 [중간] 1 — wait가 sent에 적어 주인이 못 보냈다)
  # ponytail: 늘기만 하는 파일(사건마다 한 줄) — 커지면 since 바닥보다 오래된 줄을 잘라 낸다
  # sent에는 이 watch가 주인인 사건만 적는다: 주인 모름(-)·주인 사라짐 사건은 다른 지휘 watch도 자기 커서로 한 번씩 보내게(wfilter)
  if [ $ec = watch ]; then
    s_=$(printf '%s\n' "$1" | while IFS= read -r l_; do [ "$(owner "$(printf '%s\n' "$l_" | cut -f3)")" != "${wto##*/}" ] || printf '%s\n' "$l_"; done |
      LC_ALL=C awk -F'\t' "$evkey$nkf"'{ print nk(key()) }')
    [ -z "$s_" ] || printf '%s\n' "$s_" >>"$evdir/sent" || echo "hc-lane $ec: 보냄 표시를 쓰지 못했다: $evdir/sent" >&2
  fi
  printf '%s\n' "$1"
}
# wfilter <사건 줄들>: 이 watch(--to 지휘)가 보낼 줄만(3차 감사 [중간] 1 — 지휘마다 나눈다). 레인의 지휘(owner)가
#  이 지휘면 보냄 · 다른 지휘이고 그 지휘가 살아 있거나(0) 모르면(2) 안 보냄(그 지휘 몫) ·
#  주인을 모르거나(-: 주인 없는 hc-fill 줄·터미널 밖 report·옛 레인) 그 지휘가 사라졌으면(1) 살아 있는 모든 지휘 watch가 보냄 —
#  놓치는 것보다 겹치는 게 낫다(2026-11-17 hcfill-owner: 가장 최근 지휘 한 곳이 넘겨받아 영구 주인이 되어, 레인을 넣은 지휘가
#  2시간 못 받음 · 감사 [중간] 1: 주인이 사라진 레인도 같은 꼴). 넘겨받지 않고(레인 기록에 안 더함) sent에도 안 적어 지휘마다 자기
#  커서로 한 번. 주인이 사라진 사건은 옛 주인이 자기 wait로 이미 본 것(prevseen)을 뺀다.
#  보낼지는 「주인이 나(또는 모름·사라짐) + sent·내 커서에 없음」(앞의 사건 awk가 거름) watch 자신의 알림 줄(hc-lane-wait)은 늘 보냄.
#  확인은 이 부름 안에서 지휘마다 한 번(hc get)
wfilter() {
  w0_=${wto##*/} occ_=
  printf '%s\n' "$1" | while IFS= read -r l_; do
    [ -n "$l_" ] || continue
    a_=$(printf '%s\n' "$l_" | cut -f3) n_=$(printf '%s\n' "$l_" | cut -f4)
    [ "$n_" != hc-lane-wait ] || { printf '%s\n' "$l_"; continue; }
    # 아홉째 칸(report --to): 받을 지휘를 정한 사건은 그 지휘들의 watch만(주인 판정 안 함)
    t_=$(printf '%s\n' "$l_" | cut -f9)
    if [ -n "$t_" ] && [ "$t_" != - ]; then case ",$t_," in *",$w0_,"* | *"/$w0_,"*) printf '%s\n' "$l_" ;; esac; continue; fi
    o_=$(owner "$a_")
    [ "$o_" != - ] || { printf '%s\n' "$l_"; continue; }
    [ "$o_" != "$w0_" ] || { prevseen "$a_" "$l_" "$o_" || printf '%s\n' "$l_"; continue; }
    case " $occ_ " in *" $o_=0 "*) r_=0 ;; *" $o_=1 "*) r_=1 ;; *" $o_=2 "*) r_=2 ;; *) coord_check "$o_"; r_=$?; occ_="$occ_ $o_=$r_" ;; esac
    [ $r_ = 1 ] || continue
    # 주인이 사라짐: 모든 지휘가 보냄. 옛 주인이 자기 wait로 이미 본 사건은 그 주인에게 간 것이라 보내지 않는다
    prevseen "$a_" "$l_" "$o_" || printf '%s\n' "$l_"
  done
}
# prevseen <레인 주소> <사건 줄> <지금 owner>: 「보낸 것」 = sent ∪ 그 레인의 옛 주인 seen(6차 감사 — 주인이 사라진 뒤 옛 주인이
# wait로 받은 사건을 다시 보냈다). 옛 주인 = 레인 기록의 그 레인 지휘들과 지금 owner 가운데 이 watch가 아닌 것(seen 파일이 없으면 sent만).
# 있으면 0 — 받은 것으로 치고 이 watch 커서에도 적어 다음 바퀴에 다시 거르지 않는다
prevseen() {
  k_=$(printf '%s\n' "$2" | LC_ALL=C awk -F'\t' "$evkey$nkf"'{ print nk(key()) }')
  for p_ in $( { awk -F'\t' -v a="$1" '$1 == a { print $3 }' "$evdir/lanes.tsv" 2>/dev/null; echo "$3"; } | sed 's|.*/||' | sort -u); do
    [ "$p_" != - ] && [ "$p_" != "$w0_" ] || continue
    if inseen "$k_" "$evdir/seen.$(ckey "$p_")"; then printf '%s\n' "$k_" >>"$cur"; return 0; fi
  done
  return 1
}
# lkinds: 레인 기록에서 「주소(끝의 t…)<탭>레인<탭>종류」(넷째 칸 read·fill이 있는 줄만). 구현 레인 줄은 3칸 그대로
lkinds() { awk -F'\t' 'NF >= 4 && ($4 == "read" || $4 == "fill") { a = $1; sub(/^.*\//, "", a); print a "\t" $2 "\t" $4 }' "$evdir/lanes.tsv" 2>/dev/null; }
# owner <레인 주소>: 그 레인을 띄운 지휘 — 레인 기록(<evdir>/lanes.tsv, hc-lane이 띄울 때 적음)의 마지막 줄, 없으면 hc ls parent,
# 없으면 -. <기계>/t… 꼴은 끝의 t…(watch·지휘 정보는 지휘 기계 것이다).
# ponytail: 다른 기계 지휘의 parent가 이 기계 지휘와 같은 t 이름이면 섞인다 — 잦으면 기계 이름까지 맞춘다
owner() {
  o_=$(awk -F'\t' -v a="$1" '$1 == a { o = $3 } END { print o }' "$evdir/lanes.tsv" 2>/dev/null)
  # 기록이 -(띄울 때 지휘 확인 실패 등)여도 hc parent가 있으면 그것(4차 감사 [낮음] 1. hc-fill 레인은 parent도 없다)
  [ -n "$o_" ] && [ "$o_" != - ] || o_=$(printf '%s\n' "$pmap" | awk -F'\t' -v a="$1" '$1 == a { print $2; exit }')
  o_=${o_:--}; [ "$o_" = - ] || o_=${o_##*/}
  printf %s "$o_"
}
# wsend <글>: 지휘 터미널에 붙여 넣고 Enter. 사람이 쓰던 초안이 있으면 hc가 기다리니 60초 상한(넘으면 실패 — 다음 바퀴에 다시)
# 보내기 전마다 coord_check: 지휘 정보와 다르거나 없으면 보내지 않고(남의 터미널 send 0) 「지휘 없음」을 남기고 끝낸다 — 커서에 안
# 적었으니 사건은 events에 남아 지휘가 돌아와 wait/watch를 걸면 받는다. 코어 연결 실패(모름)는 다음 바퀴에 다시
wsend() { wcheck || return 1; ! wheld || return 1; capt 60 hc send "$wto" "$1" </dev/null >/dev/null 2>&1 || { echo "hc-lane watch: hc send $wto 실패 — 다음 바퀴에 다시" >&2; return 1; }; }
# wheld: 지휘가 질문·권한 창을 띄웠으면 0(보내지 않음 — 다음 바퀴에 다시, 사건은 쌓였다가 창이 닫힌 뒤 한 번에). watch-hold 2026-11-19:
# 창이 떠 있을 때 보낸 알림 글이 그 창의 자유 입력 칸에 들어가 사용자 답으로 제출됐다. 막힘 = hc get status.state가 blocked, 또는
# status가 null(하단 줄 실행기 안의 Claude처럼 상태를 안 알림)이고 화면이 창(wdialog).
# hc get·hc read를 못 읽으면 아님(알림을 잃지 않는 쪽). lane.watch_hold_max분(0이면 막지 않음)은 status blocked에만: 넘으면 보낸다
# (hc가 blocked면 스스로 거절해 — --force를 안 쓴다 — 창이 닫힐 때까지 실패로 다시 돈다). 화면으로 잡은 창은 상한이 지나도 보내지
# 않는다(감사 M1: hc가 막지 않아 창에 답으로 들어간다). 미룬 사건은 커서에 안 적혀 지휘의 hc-lane wait로도 받으니 잃지 않는다.
# 상한을 넘으면 한 줄만(stderr·watch.log — 바퀴마다 되풀이하지 않음)
# 「"state":"blocked"」는 status 안에만 나온다(맨 위 state는 live·finished, msg 안 따옴표는 \"로 이스케이프돼 안 걸림)
wheld() {
  [ "$whmax" -gt 0 ] || return 1
  hg_=$(capt 10 hc get "$wto" --json </dev/null 2>/dev/null) || hg_=
  if printf '%s\n' "$hg_" | grep -Eq '"state" *: *"blocked"'; then hw_=blocked
  elif printf '%s\n' "$hg_" | grep -Eq '"status" *: *null' && capt 10 hc read "$wto" </dev/null 2>/dev/null | wdialog; then hw_=화면
  else wh0= wlg_=; return 1; fi
  now_=$(date +%s)
  [ -n "$wh0" ] || { wh0=$now_ wlg_=; echo "hc-lane watch: 지휘 $wto 질문·권한 창($hw_) — 보내지 않고 창이 닫히면 한 번에 보낸다" >&2; }
  [ $((now_ - wh0)) -ge "$whmax" ] || return 0
  if [ "$hw_" = 화면 ]; then
    [ -n "$wlg_" ] || { wlg_=1; l_="$(utc) hc-lane watch: $wto 질문 창 $(((now_ - wh0) / 60))분째 — 보류 중(화면으로 잡은 창은 보내지 않는다, 사건은 hc-lane wait로도 받음)"
      printf '%s\n' "$l_" >&2; printf '%s\n' "$l_" >>"$evdir/watch.log"; }
    return 0
  fi
  [ -n "$wlg_" ] || { wlg_=1; echo "hc-lane watch: 지휘 $wto 질문·권한 창이 $((whmax / 60))분 넘게 — 보낸다(blocked면 hc가 거절해 창이 닫힐 때까지 다시)" >&2; }
  return 1
}
# wdialog: 화면(stdin)이 Claude Code 질문·권한 창이면 0(감사 M2 — 대화 글에 인용된 창 글자에 걸리지 않게 화면 맨 아래 덩어리만).
# 가로줄 = 앞 빈칸 뒤 ─·╌ 셋 이상으로 시작하는 줄. 마지막 두 가로줄이 평소 입력 칸(가로줄 바로 아래 ❯ 줄, 그 아래 다시 가로줄)이면
# 창 아님 — 화면 위쪽(지휘가 hc read로 본 남의 입력 칸 등)은 안 본다(재감사 R-M1: 한 번 본 입력 칸이 맨 아래 진짜 창을 가렸다).
# 아니면 마지막 가로줄 아래 줄에서만 lane.watch_dialog 글자를 찾는다. 가로줄이 없으면 아님.
# 실측 꼴(Claude Code 2.1.328): 질문 창은 「─ / ☐ 머리 / 질문 / ❯ 1. … / Type something. / ─ / 4. Chat about this / Enter to select …」,
# 권한 창은 「╌ / 설명 / Do you want to proceed? / ❯ 1. Yes / 2. No / Esc to cancel …」 — 둘 다 입력 칸이 없다
wdialog() {
  LC_ALL=C awk '
    function rule(s) { return s ~ /^[ \t]*(─|╌)(─|╌)(─|╌)/ }
    { l[NR] = $0; if (rule($0)) { box = (lr && lr + 1 < NR && l[lr + 1] ~ /^[ \t]*❯/); lr = NR } }
    END { if (box || !lr) exit 1; for (i = lr + 1; i <= NR; i++) print l[i] }' | grep -Eq -e "$wdlg"
}
# wcheck: 이 watch의 지휘 정보가 맞나. 다르거나 없으면 「지휘 없음」을 남기고 끝낸다(exit 0), 모르면 1, 맞으면 0.
# 바퀴마다 앞에서도 부른다(남의 사건만 있어 보낼 일이 없어도 지휘가 사라졌으면 끝나게)
wcheck() {
  coord_check "$wto"; c_=$?
  # 손으로 건 watch가 시작 때 지휘 정보를 못 적었으면(hc get 잠깐 실패) 여기서 다시 적어 본다(3차 감사 [낮음] 2)
  if [ $c_ = 1 ] && [ "${wpar-0}" = 1 ] && [ ! -f "$evdir/coord.$(ckey "$wto")" ]; then coord_set "$wto"; coord_check "$wto"; c_=$?; fi
  [ $c_ != 2 ] || { echo "hc-lane watch: 지휘 $wto 확인 못 함(hc get 실패) — 다음 바퀴에 다시" >&2; return 1; }
  if [ $c_ != 0 ]; then
    l_="$(utc) hc-lane watch: 지휘 없음 $wto — $cwhy. 보내지 않고 끝낸다(사건은 events에 남음, 지휘가 wait/watch를 걸면 받음)"
    printf '%s\n' "$l_" >&2; printf '%s\n' "$l_" >>"$evdir/watch.log"
    exit 0
  fi
}
# wsleep: 끄면(TERM·INT·HUP) 바로 끝나게 sleep을 배경으로 두고 기다린다(트랩이 그 sleep도 끈다)
wsleep() { sleep "$tick" & sp_=$!; wait $sp_; sp_=; }
# watch_lock: 같은 지휘 주소로 watch는 하나만(뒤 것은 종료 75). 잠금 폴더 <evdir>/watch.<주소>/pid.
# 남은 잠금(그 pid가 없거나 hc-lane이 아님)은 넘겨받는다. 끝날 때(지휘 사라짐·TERM·INT·HUP) 자기 잠금만 지운다
# ---- 지휘 정보(2차 감사 [중간] B, 3차 [중간] 1): 지휘 주소를 설정에 박지 않는다. 지휘가 레인을 띄우거나 wait·watch를 걸 때
# 자기 정보 「주소<탭>pid<탭>이름<탭>마지막으로 레인을 띄운 시각(초, 없으면 0)」을 지휘마다 <evdir>/coord.<주소>에 남기고,
# watch는 보낼 때마다 자기 지휘의 것과 그 주소의 hc get pid·이름이 같은지 본다. 코어가 다시 떠 주소가 남에게 가면 pid가 달라
# 보내지 않는다. 지휘가 다음 레인을 띄우면 새로 써진다.
# ponytail: coord.* 는 지우지 않는다(지휘 세션마다 하나) — 많아지면 coord_default가 확인 못 하는(없음) 것을 지운다
ckey() { printf %s "$1" | tr -c 'A-Za-z0-9_-' _; }
# tinfo <주소>: 그 터미널의 맨 위 pid·이름(tp_·tn_, 이름 없으면 -)과 레인·감시 라벨 여부(tl_ 1/0). 0 살아 있음,
# 1 없음·끝남·pid 모름, 2 모름(코어 연결 실패 등 — JSON이 아님). foreground·status 안의 pid·name은 빼고 본다
tinfo() {
  g_=$(capt 10 hc get "$1" --json </dev/null 2>&1)
  printf '%s\n' "$g_" | grep -Eq '"kind" *: *"not_found"|"address" *:' || return 2
  g_=$(printf '%s\n' "$g_" | sed 's/"foreground" *: *{[^}]*}//; s/"status" *: *{[^}]*}//')
  printf '%s\n' "$g_" | grep -q '"state" *: *"live"' || return 1
  tp_=$(printf '%s\n' "$g_" | sed -n 's/.*"pid" *: *\([0-9][0-9]*\).*/\1/p' | tail -n 1)
  tn_=$(printf '%s\n' "$g_" | sed -n 's/.*"name" *: *"\([^"]*\)".*/\1/p' | tail -n 1); tn_=${tn_:--}
  tl_=0; ! printf '%s\n' "$g_" | grep -Eq '"hc-lane(-watch)?" *: *"1"' || tl_=1
  [ -n "$tp_" ]
}
# coord_is <주소>: 그 터미널이 살아 있고 레인·감시 터미널이 아니면 0(지휘로 받음). 아무것도 안 쓴다. 1 아님, 2 모름(hc get 실패)
coord_is() { tinfo "$1" || return; [ "$tl_" = 0 ]; }
# coord_set <주소> [launch]: 그 터미널이 살아 있고 레인·감시 터미널이 아니면 지휘 정보로 적는다(레인이 하위 레인을 띄워도 덮지
# 않게 — 그때 1). launch면 띄운 시각을 지금으로, 아니면 같은 pid의 옛 시각 그대로
coord_set() {
  coord_is "$1" || return 1
  f_=$evdir/coord.$(ckey "$1") ts_=0
  if IFS=$tab read -r a_ p_ n_ t_ 2>/dev/null <"$f_" && [ "$p_" = "$tp_" ]; then case $t_ in ''|*[!0-9]*) ;; *) ts_=$t_ ;; esac; fi
  [ "${2-}" != launch ] || ts_=$(date +%s)
  mkdir -p "$evdir" && printf '%s\t%s\t%s\t%s\n' "$1" "$tp_" "$(flat "$tn_")" "$ts_" >"$f_.$$" && mv -f "$f_.$$" "$f_"
}
# coord_check <주소>: 그 지휘의 정보(coord.<주소>)가 있고 지금 hc get pid·이름이 같으면 0(주소는 cad_, 띄운 시각은 cts_).
# 1 다름·없음(까닭은 cwhy), 2 모름(hc get 연결 실패)
coord_check() {
  IFS=$tab read -r cad_ cpi_ cnm_ cts_ 2>/dev/null <"$evdir/coord.$(ckey "$1")" ||
    { cwhy="지휘 정보 없음($evdir/coord.$(ckey "$1") — 지휘가 레인을 띄우거나 wait를 걸면 생김)"; return 1; }
  tinfo "$cad_"; r_=$?
  [ $r_ != 2 ] || { cwhy="hc get $cad_ 실패"; return 2; }
  [ $r_ = 0 ] || { cwhy="$cad_ 터미널이 없거나 끝남"; return 1; }
  [ "$tp_" = "$cpi_" ] && [ "$(flat "$tn_")" = "$cnm_" ] || { cwhy="$cad_ 의 pid·이름($tp_ $tn_)이 지휘 정보($cpi_ $cnm_)와 다름"; return 1; }
}
# coord_default: 살아 있고 pid·이름이 맞는 지휘 가운데 가장 최근에 레인을 띄운 곳(cad_). hc-lane coord가 찍고, 주인 없는(또는 주인이
# 사라진) hc-fill 줄을 띄울 때 hc-fill이 watch 대상으로 넘긴다(사건은 wfilter가 모든 지휘에게). 1 그런 지휘 없음, 2 그보다 최근 지휘를 확인 못 함(hc get 실패 — 남에게 보내지 않게)
coord_default() {
  bt_=-1 ba_= ut_=-1
  for cf_ in "$evdir"/coord.*; do
    case ${cf_##*/} in coord.*.*) continue ;; esac   # 쓰는 중인 coord.<주소>.<pid>
    IFS=$tab read -r ca_ cp_ cn_ ct_ 2>/dev/null <"$cf_" || continue
    case $ct_ in ''|*[!0-9]*) ct_=0 ;; esac
    coord_check "$ca_"; r_=$?
    if [ $r_ = 0 ]; then [ "$ct_" -le "$bt_" ] || { bt_=$ct_ ba_=$ca_; }
    elif [ $r_ = 2 ] && [ "$ct_" -gt "$ut_" ]; then ut_=$ct_; fi
  done
  [ -n "$ba_" ] || { cwhy="살아 있고 pid·이름이 맞는 지휘가 없다(지휘가 레인을 띄우거나 wait를 걸면 생김)"; return 1; }
  [ "$ut_" -le "$bt_" ] || { cwhy="더 최근 지휘를 확인 못 했다(hc get 실패)"; return 2; }
  cad_=$ba_
}
# fillq: hc-fill 대기열(${HC_FILL_DATA:-~/.local/share/hc-fill}/queue.tsv, 여섯째 칸 상태)에 아직 안 띄운 요청(wait)이 있으면 0.
# up은 세지 않는다: 살아 있는 up 레인은 hc ls(라벨 hc-lane=1)가 세고, 끝난 up 줄(hc-fill add 줄은 rm할 때까지 남음)을 세면
# 쉼 종료가 영영 안 온다(2차 감사 [중간] A). 받은편지 요청은 hc-fill이 바퀴마다 이 대기열로 옮기므로 함께 보인다
fillq() { awk -F'\t' '$6 == "wait" { f = 1; exit } END { exit !f }' "${HC_FILL_DATA:-$HOME/.local/share/hc-fill}/queue.tsv" 2>/dev/null; }
# watch_alive <잠금 폴더>: 그 잠금의 pid가 살아 있는 hc-lane watch면 0(그 pid를 op에)
watch_alive() {
  op=$(cat "$1/pid" 2>/dev/null)
  case $op in ''|*[!0-9]*) return 1 ;; esac
  kill -0 "$op" 2>/dev/null && ps -p "$op" -o command= 2>/dev/null | grep -q 'hc-lane.*watch'
}
watch_lock() {
  lk=$evdir/watch.${cur##*/seen.}
  if ! mkdir "$lk" 2>/dev/null; then
    ! watch_alive "$lk" || { echo "hc-lane watch: $wto 감시가 이미 돈다(pid $op)" >&2; exit 75; }
    rm -rf "$lk"; mkdir "$lk" 2>/dev/null || { echo "hc-lane watch: $wto 감시가 막 떴다" >&2; exit 75; }
  fi
  echo $$ >"$lk/pid" || die "watch: 잠금을 쓰지 못했다: $lk"
  sp_=
  trap '[ -z "$sp_" ] || kill "$sp_" 2>/dev/null; [ "$(cat "$lk/pid" 2>/dev/null)" != $$ ] || rm -rf "$lk"' EXIT
  trap 'exit 143' TERM; trap 'exit 130' INT; trap 'exit 129' HUP
}
# send --all [--audits] [--from <지휘 주소>] <글…>: 이 지휘가 띄운 살아 있는 레인 전부에 hc send(2026-11-17 지휘 「실수는 바로
# 코드로」 — 착지 마감 연장을 원격 레인에만 보내 맥 레인이 멈췄다). 주인은 owner()(레인 기록 마지막 지휘, 없으면 hc parent)의 끝 t….
# 감사(레인 기록 read·이름 audit-*)는 --audits일 때만. 한 줄씩 「보냄|실패 <주소> <레인>」, 끝 줄 「보냄 n · 실패 m」.
# 종료 0 다 보냄(대상 없음 포함), 1 보내기·hc ls 실패, 2 인자
send_cmd() {
  sall=0 saud=0 sfrom=${HC_TERMINAL_ID-} smsg=
  while [ $# -gt 0 ]; do
    case $1 in
      --all) sall=1 ;;
      --audits) saud=1 ;;
      --from) val "$@"; sfrom=$2; shift ;;
      -h|--help) usage ;;
      --) shift; smsg="${smsg:+$smsg }$*"; break ;;   # 끝 표시: 뒤 낱말은 옵션이어도 글(감사 L4)
      *) smsg="${smsg:+$smsg }$1" ;;
    esac
    shift
  done
  [ $sall = 1 ] || { echo "hc-lane send: --all 을 준다(이 지휘의 레인 전부에 보낸다. 한 레인만이면 hc send <주소>)" >&2; exit 2; }
  [ -n "$sfrom" ] || { echo "hc-lane send: 지휘 주소를 모른다(지휘 터미널에서 부르거나 --from <주소>)" >&2; exit 2; }
  [ -n "$smsg" ] || { echo "hc-lane send: 보낼 글이 없다" >&2; exit 2; }
  tl=$(capt 30 hc ls --all --json --label hc-lane=1 </dev/null 2>/dev/null) ||
    { echo "hc-lane send: hc ls 실패 — 아무에게도 안 보냈다" >&2; exit 1; }
  pmap=$(printf '%s\n' "$tl" | jrec address parent) km_=$(lkinds) ok_=0 bad_=0
  while IFS=$tab read -r a_ n_; do
    [ -n "$a_" ] && [ "$(owner "$a_")" = "${sfrom##*/}" ] || continue
    if [ $saud = 0 ]; then
      case $n_ in audit-*) continue ;; esac
      ! printf '%s\n' "$km_" | A=${a_##*/} N=$n_ awk -F'\t' '$1 == ENVIRON["A"] && $2 == ENVIRON["N"] && $3 == "read" { f = 1 } END { exit !f }' || continue
    fi
    if e_=$(capt 60 hc send "$a_" "$smsg" </dev/null 2>&1 >/dev/null); then echo "보냄 $a_ $n_"; ok_=$((ok_ + 1))
    else echo "실패 $a_ $n_ — $(flat "${e_:-hc send 실패}")"; bad_=$((bad_ + 1)); fi
  done <<EOF
$(printf '%s\n' "$tl" | jrec address state name | awk -F'\t' '$2 == "live" { print $1 "\t" $3 }')
EOF
  echo "보냄 $ok_ · 실패 $bad_"
  [ $bad_ = 0 ]
}
# early-wait <선행…>: 먼저 띄움 레인의 기다림 블록이 기한에 닿았을 때. hc-fill이 바퀴마다 이 기계에 써 주는 선행 상태
# (~/.local/share/hc-fill/after.tsv: 선행 go|stop 까닭 쓴시각(초))를 읽어 선행마다 「<선행>: <까닭>」. 모두 go이고 기록이 15분 안이면 0
# (계속 기다림), 기록 없음·묵음(hc-fill이 안 돎)·stop이 하나라도 있으면 1(예전처럼 멈춤), 인자 없으면 2
early_wait_cmd() {
  [ $# -gt 0 ] || { echo "사용법: hc-lane early-wait <선행 레인>…" >&2; return 2; }
  af_=$HOME/.local/share/hc-fill/after.tsv now_=$(date +%s) bad_=0
  for p_ in "$@"; do
    l_=$(awk -F'\t' -v n="$p_" '$1 == n { l = $0 } END { print l }' "$af_" 2>/dev/null)
    IFS=$'\t' read -r _ s_ w_ t_ <<<"$l_"
    case $t_ in '' | *[!0-9]*) t_= ;; esac
    if [ -z "$l_" ]; then echo "$p_: 기록 없음(선행 상태 모름 — $af_)"; bad_=1
    elif [ -z "$t_" ] || [ $((now_ - t_)) -gt 900 ]; then echo "$p_: 기록이 $(( (now_ - ${t_:-0}) / 60 ))분 묵음(hc-fill이 안 돎) — $w_"; bad_=1
    elif [ "$s_" = go ]; then echo "$p_: $w_"
    else echo "$p_: $w_"; bad_=1; fi
  done
  [ $bad_ = 0 ] || return 1
  echo "선행이 살아 진행 중 — 기한이 지나도 계속 기다린다"
}
