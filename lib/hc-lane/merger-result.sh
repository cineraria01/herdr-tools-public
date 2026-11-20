# hc-lane 함수: merger-result (bin/hc-lane가 읽는다. 함수는 bin/hc-lane 안 원래 정의 순서 그대로)

# mjrec <큐 이름> <레인> <결과> <후보|-> <까닭>: 병합기 일지 …/hc-lane/merger-journal.tsv에 제출마다 한 줄(2026-11-20 지휘 — 효과 수치는
# 병합기 자기 일지 하나에서, 착지 시도 기록 대신). 칸(탭): 적은 시각 · 큐 이름 · 레인 · 제출 ID · 작업 ID(HC_LANE_JOB) · 제출 시각 ·
# 묶음 크기(그 착지에 든 제출 수) · 착지 시작 · 착지 끝(push 응답을 받은 뒤 — 시각은 이 병합기가 잰 것) · 결과(착지|돌려보냄) ·
# 후보 SHA(40자 — 기본 브랜치가 담은 것, 돌려보냄은 -) · 까닭. 모르는 칸은 -. 한 번 O_APPEND(1000바이트 안)
# ponytail: 끊긴 실행을 다음 바퀴가 다시 정리하면 같은 큐 이름 줄이 둘일 수 있다 — 읽는 쪽이 큐 이름으로 하나만 센다
mjrec() {
  mt0_=-; [ -z "${ls0_-}" ] || mt0_=$(utc "$ls0_"); mt1_=-; [ -z "${ls1_-}" ] || mt1_=$(utc "$ls1_")
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$(utc)" "$1" "$2" "${mid_:--}" "${mjb_:--}" "$(utc "${1%%-*}")" "${mbn_:--}" \
    "$mt0_" "$mt1_" "$3" "$4" "$(bcut 300 "$(flat "$5")")" >>"$evdir/merger-journal.tsv" || echo "hc-lane merger: 병합기 일지를 쓰지 못했다" >&2
}
# mkey <큐 이름>: 그 제출의 결과를 가리는 열쇠 — 제출 기록이 있으면 「제출 ID <ID>」(제출마다 하나), 없으면 「큐 <이름>」. 결과 사건의 한 줄과
# 병합기 일지에 이 열쇠를 넣고, 같은 열쇠가 이미 적혔으면 다시 적지 않는다(3차 중간 6 — 사건·일지를 쓴 뒤 표시를 따로 써 그 사이에 죽으면
# 겹쳤다, 2차 중간 5). mmeta 뒤에 부른다
mkey() { if [ -n "${mid_-}" ]; then printf '제출 ID %s' "$mid_"; else printf '큐 %s' "$1"; fi; }
# mjonce <mjrec 인자…>: 같은 큐 이름·제출 ID·결과의 일지 줄이 없을 때만 적는다
mjonce() {
  LC_ALL=C awk -F'\t' -v n="$1" -v i="${mid_:--}" -v r="$3" '$2 == n && $4 == i && $10 == r { f = 1 } END { exit !f }' "$evdir/merger-journal.tsv" 2>/dev/null ||
    mjrec "$@"
}
# mdone <큐> <제출 SHA> <레인> <기록 SHA|-> <들어간 후보 C> <까닭>: 기본 브랜치가 C를 담은 뒤의 정리 — 한 번의 원자 push로, 원격에 지금 남은
# 것만: 제출 기록을 refs/merge-done/<레인>으로(접수 확인·결과 조회용, 기록 없는 큐면 C), 큐 표식·lane 가지·기록은 그 제출 값일 때만 lease로
# 지우고(3차 높음 3 — 옛 병합기가 새 제출을 지울 수 없다), 작업 표식이 없으면 C로(3차 중간 4). 정리가 확인된 뒤에만 결과 사건을 보낼 함에
# 쓴다(3차 중간 5 — 지우기가 계속 실패해도 사건이 나갔다). 정리 뒤 사건 전에 죽으면 일지(cur)가 남아 다음 바퀴가 같은 정리(할 것 없음)·사건을
# 한다. 제출 기록이 있는데 못 읽으면 쉰다(3차 중간 7). 망 오류·거절이면 1(일지를 두고 다음 바퀴가 같은 판정)
mdone() {
  q_=$1 s_=$2 ln_=$3 m_=${4#-} C_=$5 why_=$6 n_=${1#refs/merge-queue/}
  mmeta "$m_" "$s_" 2>/dev/null || { echo "hc-lane merger: 제출 기록($n_)을 읽지 못했다 — 다음 바퀴에" >&2; return 1; }
  dn_=${m_:-$C_} jk_=
  [ -z "$mjb_" ] || [ "$mjb_" = - ] || ! git check-ref-format "refs/hc-landed/$mjb_" || jk_=refs/hc-landed/$mjb_
  rr_=$(git ls-remote origin "$q_" "refs/heads/lane/$ln_" "refs/merge-queue-meta/$n_" "refs/merge-done/$ln_" ${jk_:+"$jk_"}) ||
    { echo "hc-lane merger: 정리할 참조를 보지 못했다(git ls-remote) — 다음 바퀴에 일지로 판정" >&2; return 1; }
  rv_() { printf '%s\n' "$rr_" | awk -v r="$1" '$2 == r { print $1 }'; }
  set --
  [ "$(rv_ "refs/merge-done/$ln_")" = "$dn_" ] || set -- "$@" "+$dn_:refs/merge-done/$ln_"
  [ "$(rv_ "$q_")" != "$s_" ] || set -- "$@" ":$q_" "--force-with-lease=$q_:$s_"
  [ "$(rv_ "refs/heads/lane/$ln_")" != "$s_" ] || set -- "$@" ":refs/heads/lane/$ln_" "--force-with-lease=refs/heads/lane/$ln_:$s_"
  [ -z "$m_" ] || [ "$(rv_ "refs/merge-queue-meta/$n_")" != "$m_" ] || set -- "$@" ":refs/merge-queue-meta/$n_" "--force-with-lease=refs/merge-queue-meta/$n_:$m_"
  [ -z "$jk_" ] || [ -n "$(rv_ "$jk_")" ] || set -- "$@" "$C_:$jk_"
  if [ $# -gt 0 ] && ! git push -q --atomic --porcelain origin "$@" >/dev/null 2>&1; then
    echo "hc-lane merger: $ln_ 의 큐·lane·기록 참조를 정리하지 못했다(git push) — 다음 바퀴에 일지로 판정" >&2; return 1
  fi
  rm -f "$mj_/$ln_.tr"
  mjonce "$n_" "$ln_" 착지 "$C_" "$why_"
  moutbox "$n_" 착지 "$C_" "$why_ — 제출 $(printf %.8s "$s_"), $(mkey "$n_")${mi_:+ (${mi_#*, })}" "$ln_" || return 1
  msend "$n_"
  echo "hc-lane merger: $ln_ 착지 $(printf %.8s "$C_") ($why_)"
}
# mfail <큐> <제출 SHA> <레인> <기록 SHA|-> <로그|-> <까닭>: 큐 표식을 refs/merge-failed/<이름>, 기록을 refs/merge-failed-meta/<이름>으로
# 한 번의 원자 push로 옮기고(lane 가지는 둠) 막힘 사건. 옮기다 원격을 못 보면 멈추지 않고 fail/<이름>에 적어 다음 바퀴에 다시(1차 중간 3)
# — 그동안 그 제출만 건너뛰고 다른 제출은 처리한다. 1 = 아직 못 옮김
mfail() {
  q_=$1 s_=$2 ln_=$3 m_=${4#-} lg__=${5#-} why_=$6 n_=${1#refs/merge-queue/} fr_=refs/merge-failed/${1#refs/merge-queue/}
  # 제출 기록이 있는데 못 읽으면 반송하지 않고 쉰다(3차 중간 7). 이름 꼴이 틀린 항목은 기록이 맞지 않아도 반송한다(그 큐만 막히지 않게)
  mmeta "$m_" "$s_" 2>/dev/null || { case $why_ in "큐 이름 꼴이 아니다"*) mi_= mid_= ;; *) echo "hc-lane merger: 제출 기록($n_)을 읽지 못했다 — 다음 바퀴에" >&2; return 1 ;; esac; }
  # 옮기기 전에 할 일을 적고(fail/), 옮긴 것을 확인한 뒤에 사건을 보낼 함에 — 못 옮겼는데 막힘이 나가지 않게
  # 다 쓴 뒤 바꿔 넣는다(2차 중간 4 — 덮어쓰다 끊겨 빈 파일이 남으면 다시 하기가 빈 이름으로 멈췄다)
  printf '%s\n' "$q_" "$s_" "$ln_" "${m_:--}" "${lg__:--}" "$(flat "$why_")" >"$mj_/fail/$n_.tmp" && mv -f "$mj_/fail/$n_.tmp" "$mj_/fail/$n_" ||
    die "merger: 돌려보낼 일을 적지 못했다"
  ob__="병합기: $why_ — 원본 $(printf %.8s "$s_"), 기준 $(printf %.8s "${B_:--}"), $(mkey "$n_")${mi_:+ (${mi_#*, })}, $fr_ 로 옮김(lane/$ln_ 는 그대로)${lg__:+, 로그 $lg__}"
  # 옮길 제출 객체가 이 클론에 아직 없으면(큐를 받기 전에 반송 — 이름 꼴) 그 큐 참조를 받는다
  git cat-file -e "$s_^{commit}" 2>/dev/null || fetchr git fetch -q origin "+$q_:refs/hc-merger/q/$n_" 2>/dev/null ||
    { echo "hc-lane merger: 실패한 제출 $q_ 를 받지 못했다(git fetch) — 다음 바퀴에 다시" >&2; return 1; }
  set -- "$s_:$fr_" ":$q_" "--force-with-lease=$fr_:" "--force-with-lease=$q_:$s_"
  [ -z "$m_" ] || set -- "$@" "$m_:refs/merge-failed-meta/$n_" ":refs/merge-queue-meta/$n_" "--force-with-lease=refs/merge-failed-meta/$n_:" "--force-with-lease=refs/merge-queue-meta/$n_:$m_"
  git push -q --atomic --porcelain origin "$@" >/dev/null 2>&1 || {
    rb_=$(git ls-remote origin "$q_" "$fr_") || { echo "hc-lane merger: 실패한 제출을 옮겼는지 확인하지 못했다($q_) — 다음 바퀴에 다시" >&2; return 1; }
    case $(printf '%s\n' "$rb_" | awk -v a="$q_" -v b="$fr_" -v s="$s_" '$2 == a { q = 1 } $2 == b && $1 == s { f = 1 } END { print q ? "left" : (f ? "ok" : "gone") }') in
      ok) ;;   # 응답만 끊김
      gone) ob__="병합기: $why_ (그새 다시 제출해 옛 표식이 없어 옮기지 않음 — 새 제출은 큐에) — 원본 $(printf %.8s "$s_"), $(mkey "$n_")${lg__:+, 로그 $lg__}" ;;
      *) echo "hc-lane merger: 실패한 제출 $q_ 를 $fr_ 로 옮기지 못했다 — 다음 바퀴에 다시" >&2; return 1 ;;
    esac
  }
  moutbox "$n_" 막힘 - "$ob__" "$ln_" || return 1
  rm -f "$mj_/fail/$n_" "$mj_/$ln_.tr"
  mjonce "$n_" "$ln_" 돌려보냄 - "$why_"
  msend "$n_"
  echo "hc-lane merger: $ln_ 실패 — $why_ → $fr_" >&2
}
mfretry() {  # 옮기다 멈춘 돌려보내기를 다시(못 하면 그 제출만 계속 건너뜀)
  for f_ in "$mj_"/fail/*; do
    [ -f "$f_" ] || continue
    case $f_ in *.tmp) continue ;; esac
    q_= s_= ln_= m_= lg_f= why_=
    { read -r q_; read -r s_; read -r ln_; read -r m_; read -r lg_f; read -r why_; } <"$f_"
    # 읽은 칸이 꼴에 맞지 않으면(잘린 파일) 버린다 — 그 제출은 큐에 그대로라 다음 바퀴에 다시 판정된다(2차 중간 4)
    case $q_ in refs/merge-queue/?*) ;; *) q_= ;; esac
    if [ -z "$q_" ] || [ "${q_#refs/merge-queue/}" != "${f_##*/}" ] || ! printf %s "$s_" | grep -Eq '^[0-9a-f]{40}$' || [ -z "$ln_" ] || [ -z "$why_" ]; then
      echo "hc-lane merger: 돌려보낼 기록 ${f_##*/} 이 깨졌다 — 버리고 큐에서 다시 판정" >&2; rm -f "$f_"; continue
    fi
    mfail "$q_" "$s_" "$ln_" "$m_" "$lg_f" "$why_" || mer_=1
  done
}
# moutbox <이름> <상태> <sha|-> <글> <레인>: 보낼 사건을 out/<이름>에(지휘 --to·레인 터미널·결과 열쇠 함께). msend가 보내고 지운다
moutbox() {
  printf '%s\n' "$2" "$3" "$4" "$5" "${mo__-}" "${mh_-}" "${mtm_-}" "$(mkey "$1")" >"$mj_/out/$1.tmp" && mv -f "$mj_/out/$1.tmp" "$mj_/out/$1" ||
    { echo "hc-lane merger: 보낼 사건을 적지 못했다" >&2; return 1; }
}
# msend <이름>: 사건(--name <레인>, 제출 때 적은 지휘가 있으면 --to)을 — 같은 레인·상태에 같은 결과 열쇠(mkey)가 든 사건이 이 기계 사건
# 파일에 아직 없을 때만 — 적고(3차 중간 6·2차 중간 5), 제출한 레인 터미널이 살아 있고(최상위 state live) 그 이름(최상위 name)이 레인이면
# hc send로도 알린다 — 이 기계면 <터미널>, 다른 기계면 <기계>/<터미널>(2차 중간 15·3차 중간 12). 알림이 실패하면(끊김) 보낼 함에 두고
# 다음 바퀴에 다시, 세 번까지. 터미널이 없으면(not_found) 사건만. ponytail: hc get과 hc send 사이에 주소가 다른 실행에 다시 쓰이는 것은
# 막지 못한다(모델 밖 — 확실한 결과는 제출 ID가 든 사건)
msend() {
  of_=$mj_/out/$1
  [ -f "$of_" ] || return 0
  o_k=
  { read -r o_st; read -r o_sh; read -r o_msg; read -r o_ln; read -r o_to; read -r o_h; read -r o_t; read -r o_k; } <"$of_"
  if ! LC_ALL=C awk -F'\t' -v l="$o_ln" -v s="$o_st" -v k="${o_k:-$o_msg}" '$4 == l && $5 == s && index($7, k) { f = 1 } END { exit !f }' "$evdir/events.tsv" 2>/dev/null; then
    (mgrun_=; report_cmd "$o_st" "$o_sh" "$o_msg" --name "$o_ln" ${o_to:+--to "$o_to"}) >/dev/null ||
      { echo "hc-lane merger: $o_st 사건을 쓰지 못했다 — 다음 바퀴에 다시" >&2; return 0; }
  fi
  if [ -n "$o_t" ] && [ "$o_t" != - ]; then
    ad_=$o_t; [ "$o_h" = "$mme_" ] || ad_=$o_h/$o_t
    gj_=$(capt 3 hc get "$ad_" --json 2>/dev/null); gr_=$?
    nr_=0   # 다시 할까
    if [ $gr_ = 0 ]; then
      [ "$(printf '%s\n' "$gj_" | jq -r 'select(.state == "live") | .name // empty' 2>/dev/null)" != "$o_ln" ] ||
        capt 5 hc send "$ad_" "hc-lane merger: $o_ln $o_st ${o_sh#-} — $o_msg" >/dev/null 2>&1 || nr_=1
    elif [ $gr_ != 3 ]; then nr_=1; fi   # 3 = 그 터미널 없음(not_found) — 끝난 레인
    if [ $nr_ = 1 ]; then
      tr_=$(cat "$of_.try" 2>/dev/null); tr_=$(( ${tr_:-0} + 1 ))
      if [ $tr_ -lt 3 ]; then
        echo "$tr_" >"$of_.try"; echo "hc-lane merger: $o_ln 레인 터미널($ad_)에 알리지 못했다 — 다음 바퀴에 다시($tr_/3)" >&2; return 0
      fi
      echo "hc-lane merger: $o_ln 레인 터미널($ad_) 알림을 세 번 못 보냈다 — 사건만 남김" >&2
    fi
  fi
  rm -f "$of_" "$of_.try"
}
mflush() { for f_ in "$mj_"/out/*; do case $f_ in *.tmp|*.try) continue ;; esac; [ ! -f "$f_" ] || msend "${f_##*/}"; done; }
