# hc-fill 함수: audit (bin/hc-fill가 읽는다. 함수는 bin/hc-fill 안 원래 정의 순서 그대로)

arow() { awk -F'\t' -v a="$1" 'NF >= 17 && $1==a' "$AT" 2>/dev/null | tail -n 1; }
aupd() {  # aupd <aid> <칸> <값> [<칸> <값>…]: 그 차수 줄의 칸을 바꾼다(dry-run은 안 씀). 값의 탭·줄바꿈은 빈칸
  [ $dry = 0 ] || return 0
  au_=$1; shift; kv_=
  while [ $# -ge 2 ]; do kv_="$kv_$1	$(flat1 "${2:--}")	"; shift 2; done   # 빈 값은 -(탭 read가 빈 칸을 뭉개 칸이 밀리지 않게)
  awk -F'\t' -v OFS='\t' -v a="$au_" -v kv="$kv_" 'BEGIN { n = split(kv, x, "\t") } NF >= 17 && $1==a { for (i = 1; i + 1 <= n; i += 2) $(x[i]) = x[i+1] } 1' "$AT" >"$AT.tmp" &&
    mv -f "$AT.tmp" "$AT"
}
aadd() {  # aadd <17칸>: 빈 칸은 -
  [ $dry = 0 ] || return 0
  ad_=; for x_ in "$@"; do ad_="$ad_${ad_:+	}$(flat1 "${x_:--}")"; done
  printf '%s\n' "$ad_" >>"$AT"
}
# 옛 감사 기록 옮기기: 끝 상태(pass·fix·coord·nosha)는 legacy(같은 레인의 같은 sha로 다시 열지 않음), queued는 판정 묶음이 없으니
# 소비하지 않고 지휘에(escalate:legacy). aid는 원본 줄의 해시(결정적 — 다시 돌아도 같다)
amigrate() {
  [ $dry = 0 ] && [ -s "$AT" ] && awk -F'\t' 'NF >= 5 && NF < 17 {f=1} END {exit !f}' "$AT" || return 0
  while IFS= read -r l_; do
    nf_=$(printf '%s\n' "$l_" | awk -F'\t' '{print NF}')
    if [ "$nf_" -ge 17 ] || [ "$nf_" -lt 5 ]; then printf '%s\n' "$l_"; continue; fi
    printf '%s\n' "$l_" | awk -F'\t' -v OFS='\t' -v id="al$(printf %s "$l_" | cksum | awk '{printf "%08x", $1}')" '{
      st = ($5 == "queued") ? "escalate:legacy" : "legacy"
      nt = ($5 == "queued") ? $1 " 옛 감사 기록(" $2 " " $3 "번째) — 판정을 차수에 묶지 못한 채 hc-fill이 바뀌었다. 지휘 판단(옛 감사 줄 정리 포함)" : "-"
      print id, $1, $3, "-", $2, "-", "-", st, ($4 == "" ? 0 : $4), "-", 0, 0, "-", "-", 0, "-", nt }'
  done <"$AT" >"$AT.tmp" && mv -f "$AT.tmp" "$AT" && echo "감사 기록 옮김: 옛 줄을 차수 기록으로(legacy·escalate:legacy)"
}
lalive() { printf '%s\n' "$TERMS" | awk -F'\t' -v a="$1" -v n="$2" '$2==a && ($4 "")==n {f=1} END {exit !f}'; }   # 그 주소에 그 이름 터미널
# audname <레인>: 옛 감사 줄 이름(옮긴 기록의 감사 줄 정리용). audit-<레인>이 32자를 넘으면 audit-<앞 21자>-<cksum 4자리 16진>
audname() {
  an=audit-$1; [ ${#an} -le 32 ] || an=audit-$(printf %s "$1" | cut -c 1-21)-$(printf %s "$1" | cksum | awk '{printf "%04x", $1 % 65536}')
  echo "$an"
}
alive() { printf '%s\n' "$TERMS" | awk -F'\t' -v a="$1" '$2==a {print $5; f=1; exit} END {if (!f) print "gone"}'; }   # 상태 또는 gone
lastev() {  # lastev <이름> <터미널> [<상태>]: 그 터미널이 그 이름으로 적은 마지막 사건 줄(events는 시각 순)
  printf '%s\n' "$EV" | awk -F'\t' -v n="$1" -v a="$2" -v s="${3-}" '($4 "")==n && $3==a && (s == "" || $5==s) {l=$0} END {print l}'
}
# 확정한 실행 키는 터미널이 사라진 뒤에도 같은 주소의 옛 사건을 거른다.
attempt_run() { awk -F'\t' -v n="$1" -v a="$2" '($1 "")==n && $6==a && $7 ~ /@[0-9]+$/ {print $7; exit}' "$DATA/try.tsv" 2>/dev/null; }
joblast() {
  local key
  key=$(attempt_run "$1" "$2")
  if [ -n "$key" ]; then runlast "$1" "$2" "$key" "${3-}"
  elif pget "$2" && [ "$PNAME" = "$1" ]; then runlast "$1" "$2" "$PRUN" "${3-}"
  else lastev "$@"; fi
}
# pget <주소>: PRUN(<기계 접두 없는 터미널>@<created_ms> — 레인 report 10칸과 같은 꼴)·PNAME. 0 살아 있음, 3 끝남 확정(not_found·finished), 1 모름
pget() {
  PRUN= PNAME=
  pg_=$(hc get "$1" --json 2>&1 </dev/null); pr_=$?
  printf '%s\n' "$pg_" | jq -e '(.error.kind // .kind) == "not_found"' >/dev/null 2>&1 && return 3
  [ $pr_ = 0 ] || return 1
  printf '%s\n' "$pg_" | jq -se 'map(select(type == "object")) | .[0].state == "finished"' >/dev/null 2>&1 && return 3
  pc_=$(printf '%s\n' "$pg_" | jq -sr 'map(select(type == "object")) | .[0].created_ms // empty' 2>/dev/null)
  case $pc_ in '' | *[!0-9]*) return 1 ;; esac
  PRUN=${1##*/}@$pc_ PNAME=$(printf '%s\n' "$pg_" | jq -sr 'map(select(type == "object")) | .[0].name // "-"' 2>/dev/null)
}
# runlast <레인> <주소> <실행> [<상태>]: 그 실행이 그 이름으로 적은 마지막 사건. 10칸(발행 실행)이 있으면 그 값이 같은 것, 없거나 -인
# 옛 사건은 같은 주소·이름이고 시각이 그 실행 시작 이후인 것(그 주소는 한 번에 한 실행만 쥔다)
runlast() {
  rs_=$(date -u -r $(( ${3#*@} / 1000 )) +%Y-%m-%dT%H:%M:%SZ 2>/dev/null) || rs_=9
  printf '%s\n' "$EV" | awk -F'\t' -v n="$1" -v a="$2" -v r="$3" -v t="$rs_" -v s="${4-}" \
    '($4 "")==n && $3==a && (s == "" || $5==s) && ((NF >= 10 && $10 != "-") ? ($10 == r) : ($1 >= t)) {l=$0} END {print l}'
}
aesc() {  # aesc <레인> <주인|-> <글> [<키>]: 그 레인 주인 지휘(없으면 모든 지휘)에 막힘 사건(키가 있으면 한 번만). 0 남김(dry-run 포함)
  if [ $dry = 1 ]; then echo "${P}지휘로 넘길 $1: $3"; return 0; fi
  ao=$2; [ "$ao" != - ] || ao=
  if env -u HC_TERMINAL_ID -u HC_LANE hc-lane report 막힘 - "$3" --name "$1" ${ao:+--to "$ao"} ${4:+--key "$4"} >/dev/null 2>&1 </dev/null; then
    echo "지휘로 넘김 $1 → ${ao:-모든 지휘}: $3"; return 0
  fi
  echo "지휘로 넘기기 실패 $1(hc-lane report) — 다음 바퀴에 다시"; return 1
}
qrow() { awk -F'\t' -v n="$1" '($1 "")==n {print; exit}' "$Q" 2>/dev/null; }
qown() { awk -F'\t' -v n="$1" '($1 "")==n {print ($12 == "" ? "-" : $12); f=1; exit} END {if (!f) print "-"}' "$Q" 2>/dev/null; }   # 그 줄 주인(없으면 -)
aterm() { case $1 in taken | superseded | coord | nosha | legacy) return 0 ;; esac; return 1; }   # 끝 상태
# a_esc <aid> <까닭> <글>: 기록을 먼저(escalate:<까닭> + 글) — 사건은 아래 a_send_esc가(키로 한 번). 끝나면 coord(nosha는 nosha)
a_esc() { aupd "$1" 8 "escalate:$2" 17 "$3"; crash after-escalate-record; a_send_esc "$1"; }
a_send_esc() {
  IFS=$'\t' read -r se_a se_l _ _ _ _ _ se_s _ _ _ _ _ _ _ _ se_t <<<"$(arow "$1")"
  [ -n "$se_a" ] || return 0
  aesc "$se_l" "$(qown "$se_l")" "$se_t" "hf:$se_a:${se_s#escalate:}" || return 0
  crash after-escalate-event
  if [ "$se_s" = escalate:nosha ]; then aupd "$1" 8 nosha; else aupd "$1" 8 coord; fi
}
# 레인 기계의 영속 지시·수거 표시(~/.local/share/hc-fill/verdicts/<레인>/). VPUT <aid> <sha256> <크기>(표준 입력 = 지시): 없으면 같은 내용으로
# 배타 생성(ln), 있으면 해시 대조. 찍는 것 「WROTE|ALREADY <표시> <끝>」(표시 none|taken|landing|void, 끝 yes|no) · CONFLICT · BADINPUT
s_vput() {
  printf %s 'mkdir -p "$d" || exit 1; v=$d/$1.md; x=$d/$1.ack; t=$(mktemp "$d/.put.XXXXXX") || exit 1
cat >"$t" || { rm -f "$t"; exit 1; }
 hs() { shasum -a 256 "$1" | cut -d " " -f 1; }
if [ "$(wc -c <"$t" | tr -d " ")" != "$3" ] || [ "$(hs "$t")" != "$2" ]; then rm -f "$t"; echo BADINPUT; exit 0; fi
if ln "$t" "$v" 2>/dev/null; then r=WROTE; elif [ -f "$v" ] && [ "$(hs "$v")" = "$2" ]; then r=ALREADY; else rm -f "$t"; echo CONFLICT; exit 0; fi
rm -f "$t"; a=none; [ ! -f "$x" ] || a=$(cut -f 1 "$x" | head -n 1); e=no; [ ! -f "$d/$1.done" ] || e=yes; echo "$r ${a:-none} $e"'
}
# VVOID <aid> <시각>: 표시 void를 배타 생성 — 이미 있으면(레인이 먼저 수거·착지 시작) 그대로. 찍는 것 = 지금 표시
s_vvoid() {
  printf %s 'mkdir -p "$d" || exit 1; x=$d/$1.ack; t=$(mktemp "$d/.void.XXXXXX") || exit 1
printf "void\t%s\n" "$2" >"$t" && { ln "$t" "$x" 2>/dev/null; rm -f "$t"; }; [ -f "$x" ] || exit 1; cut -f 1 "$x" | head -n 1'
}
# a_void <aid> <레인> <기계>: 0 void를 잡음(넘기거나 버려도 됨), 2 레인이 이미 수거·착지 시작(→ taken), 1 모름(다음 바퀴)
a_void() {
  aok "$3" || return 1
  [ $dry = 0 ] || return 0
  vo_=$(onm "$3" "$(s_vvoid)" "~/.local/share/hc-fill/verdicts/$2" "$1" "$(date -u +%Y-%m-%dT%H:%M:%SZ)") || return 1
  case $vo_ in void) return 0 ;; taken | landing) aupd "$1" 8 taken; echo "감사 $1: 레인이 이미 판정을 받음($vo_) — 끝"; return 2 ;; esac
  return 1
}
abase() {
  local path
  path=$(repo_of "$1" "$LOCAL")
  if [ -n "$path" ]; then project_ref "$path"; return; fi
  path=$(repo_of "$1" "$2"); [ -n "$path" ] || return 1
  onm "$2" "$(declare -f project_ref)
project_ref \"\$d\"" "$path"
}
# abrief <aid> <레인> <기계> <sha40> <프로젝트>: 감사 지시문 briefs/audit/<aid>.md를 확정(이미 있으면 그대로 — 처음 확정본으로 이어 감)
abrief() {
  bf=$DATA/briefs/audit/$1.md
  [ ! -s "$bf" ] || return 0
  repo=$(repo_of "$5" "$3"); [ -n "$repo" ] || { echo "자동 감사 못 함 $2: 설정에 $5 @$3 repo 줄이 없다"; return 1; }
  t=$(cat -- "$AUD") || { echo "자동 감사 못 함 $2: 틀을 읽지 못함 $AUD — 다음 바퀴에"; return 1; }
  case $t in *'{audit_id}'*) ;; *)   # 옛 설치 틀: 새 판정 줄 꼴을 덧붙인다(판정이 차수·커밋에 묶이게)
    t="$t

## 판정 줄 (hc-fill — 이 절이 위 판정 줄보다 앞선다)
마지막에 정확히 이 꼴로 한 번 부른다(숫자만 바꾼다. 차수 ID와 40자 sha가 맞아야 hc-fill이 판정을 받는다):
\`hc-lane report 판정 {sha} \"감사 완료 {audit_id} 높음 <n>·중간 <n>·낮음 <n>\"\`" ;;
  esac
  t=${t//\{lane\}/$2} t=${t//\{machine\}/$3} t=${t//\{worktree\}/${repo%/}.wt/$2} t=${t//\{branch\}/$2} t=${t//\{sha\}/$4} t=${t//\{audit_id\}/$1}
  local ref branch
  ref=$(abase "$5" "$3") || { echo "자동 감사 미룸 $2: 기본 가지 조회 실패"; return 1; }
  branch=${ref#refs/heads/}; branch=${branch#origin/}
  t=${t//origin\/\{base\}/$ref} t=${t//\{base\}/$branch} t=${t//\{brief\}/"~/.local/share/hc-fill/runs/$2-run.md"} t=${t//\{report\}/"~/.local/share/hc-fill/reports/audit-$1.md"}
  mkdir -p "${bf%/*}" && printf '%s\n' "$t" >"$bf.tmp" && mv -f "$bf.tmp" "$bf" || { echo "자동 감사 못 함 $2: 감사 지시문을 쓰지 못함 $bf"; return 1; }
}
# a_put <aid> <레인> <기계> <프로젝트>: 감사 줄 audit-<aid>를 대기열에(이미 있으면 그대로 — 멱등). 주인은 레인 줄 주인
a_put() {
  [ -z "$(qrow "audit-$1")" ] || return 0
  qlock _add "audit-$1" "$4" "$DATA/briefs/audit/$1.md" - "$3" wait - - - "$(date +%Y-%m-%dT%H:%M)" - "$(qown "$2")" urgent no-early "audit=$2" - >/dev/null
}
aproj() { awk -F'\t' -v a="$1" '$1==a {print $2; exit}' "$DATA/audit-proj.tsv" 2>/dev/null; }
# 차수 하나를 본다(상태별). 줄 칸을 읽어 둔 변수: aid ln rnd prun sha am pa ast afx av awk awt alc alw anv afl
a_round() {
  an=audit-$aid
  case $ast in
    opening)   # 지시문 확정 → 대기열 → queued. 중간에 죽었으면 남은 단계만
      [ $dry = 0 ] || { echo "${P}이을 감사 $an (opening)"; return 0; }
      pj_=$(aproj "$aid")
      abrief "$aid" "$ln" "$am" "$sha" "$pj_" || return 0
      a_put "$aid" "$ln" "$am" "$pj_" || return 0
      aupd "$aid" 8 queued; echo "감사 넣음 $an ← $ln ${sha:0:8} (${rnd}번째, $am)" ;;
    queued) a_queued ;;
    deliver) a_deliver ;;
    escalate:*) a_send_esc "$aid" ;;
    *) a_cleanup ;;
  esac
}
a_queued() {
  qr_=$(qrow "$an")
  if [ -z "$qr_" ]; then a_esc "$aid" cancelled "$ln 감사 ${rnd}번째($aid, ${sha:0:8}) 감사 줄이 대기열에서 빠졌다(hc-fill rm) — 지휘 판단"; return 0; fi
  IFS=$'\t' read -r _ _ _ _ _ qs_ qm_ qa_ _ <<<"$qr_"
  aok "$qm_" || return 0
  rep="~/.local/share/hc-fill/reports/$an.md"
  # 판정: 이 차수 감사 줄 이름 · 「판정」 · 6칸이 이 차수 40자 sha · 본문에 「감사 완료 <aid> 」(다른 차수·옛 꼴 판정은 맞을 수 없다)
  v_=$(printf '%s\n' "$EV" | awk -F'\t' -v n="$an" -v a="$aid" -v s="$sha" '($4 "")==n && $5=="판정" && $6==s && index($7 " ", "감사 완료 " a " ") {l=$0} END {print l}')
  if [ -z "$v_" ]; then
    ats_=$(alive "$qa_"); [ "$qs_" = up ] || ats_=-
    case $ats_ in
      gone | done | idle)
        anv=$((${anv:-0} + 1)); aupd "$aid" 15 "$anv"
        if [ $anv -ge 3 ]; then
          lx_=$(printf '%s\n' "$EV" | awk -F'\t' -v n="$an" '($4 "")==n {l=$5 " " $7} END {print l}')
          a_esc "$aid" noverdict "$ln 감사 ${rnd}번째 판정 없음 — 지휘 판단 (감사 터미널 $qa_ ${ats_}, 마지막 사건 「${lx_:-없음}」 — 옛 꼴 판정(차수 ID·40자 sha 없음)은 받지 않는다, $an @$qm_ $rep)"
        else echo "감사 판정 없음 $an $qa_ ${ats_} — ${anv}/3 바퀴"; fi ;;
      *) [ "${anv:-0}" = 0 ] || aupd "$aid" 15 0 ;;
    esac
    return 0
  fi
  # 부모 실행이 그대로인가(같은 주소·같은 created_ms). 끝남 확정이면 지휘로, 모르면 다음 바퀴
  pget "$pa"; pr_=$?
  [ $pr_ != 1 ] || { echo "감사 판정 미룸 $an: 부모 $pa 를 확인하지 못했다(hc get)"; return 0; }
  if [ $pr_ = 3 ] || [ "$PRUN" != "$prun" ]; then
    a_esc "$aid" parent-ended "$ln 감사 ${rnd}번째 판정이 왔는데 부모 실행($prun)이 끝났다 — 지휘 판단 (${sha:0:8}, $an @$qm_ $rep)"; return 0
  fi
  le_=$(runlast "$ln" "$pa" "$prun" 완료); ls_=$(printf %s "$le_" | cut -f 6)
  [ -n "$le_" ] || { echo "감사 판정 미룸 $an: 부모 실행의 완료 사건을 못 찾았다"; return 0; }
  case $sha in "$ls_"*) [ ${#ls_} -ge 7 ] || ls_=x ;; *) ls_=x ;; esac
  if [ "$ls_" = x ]; then aupd "$aid" 8 superseded; echo "판정 버림 $an: $ln 이 새 완료($(printf %s "$le_" | cut -f 6))를 적었다 — 새 차수로"; return 0; fi
  msg_=$(printf %s "$v_" | cut -f 7)
  local format="^감사 완료 $aid 높음 ([0-9]+)·중간 ([0-9]+)·낮음 ([0-9]+)$"
  if [[ $msg_ =~ $format ]]; then
    h_=${BASH_REMATCH[1]} md_=${BASH_REMATCH[2]} lw_=${BASH_REMATCH[3]}
  else
    a_esc "$aid" unreadable "$ln 감사 ${rnd}번째 판정 줄을 못 읽음(「${msg_}」) — 지휘 판단 ($an @$qm_ $rep)"; return 0
  fi
  if [[ $h_$md_ =~ ^0+$ ]]; then av=pass
  elif [ "${afx:-0}" -lt "$AMAX" ]; then av=fix afx=$((${afx:-0} + 1))
  else a_esc "$aid" limit "$ln 감사 ${rnd}번째 높음 ${h_}·중간 $md_ — 지휘 판단 (지적 ${afx}번, 상한 audit_max $AMAX, $an @$qm_ $rep)"; return 0; fi
  [ $dry = 0 ] || { echo "${P}넘길 판정 $an → $ln $pa ($av, 높음 ${h_}·중간 ${md_}·낮음 ${lw_:-?})"; return 0; }
  aupd "$aid" 8 deliver 9 "$afx" 10 "$av" 17 "$rep"
  crash after-consume
  echo "판정 받음 $an → $ln ($av, 높음 ${h_}·중간 ${md_}·낮음 ${lw_:-?})"
  ast=deliver alc= alw=; a_deliver
}
a_deliver() {
  if ! aok "$am"; then [ -z "$alc" ] || [ "$alc" = - ] || aupd "$aid" 13 -; return 0; fi   # 끊긴 동안은 기다린 시간을 세지 않는다
  [ $dry = 0 ] || { echo "${P}둘 판정 $aid → $am:verdicts/$ln/"; return 0; }
  IFS=$'\t' read -r _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ rep <<<"$(arow "$aid")"
  # 파일 공개 전에 부모 실행·최신 완료를 재확인한다. 이미 수거했으면 a_void가 그 표시를 보존한다.
  pget "$pa"; pr_=$?
  [ $pr_ != 1 ] || return 0
  if [ $pr_ = 3 ] || [ "$PRUN" != "$prun" ]; then
    a_void "$aid" "$ln" "$am"; case $? in 0) a_esc "$aid" parent-ended "$ln 감사 ${rnd}번째 판정($av)을 수거하기 전에 부모 실행($prun)이 끝났다 — 지휘 판단 ($an $rep)" ;; esac
    return 0
  fi
  le_=$(runlast "$ln" "$pa" "$prun" 완료); ls_=$(printf %s "$le_" | cut -f 6)
  [ -n "$le_" ] || { echo "판정 두기 미룸 $aid: 부모 완료 사건을 확인하지 못했다"; return 0; }
  case $sha in "$ls_"*) [ ${#ls_} -ge 7 ] || ls_=x ;; *) ls_=x ;; esac
  if [ "$ls_" = x ]; then
    a_void "$aid" "$ln" "$am"; case $? in 0) aupd "$aid" 8 superseded; echo "판정 취소 $an: $ln 이 수거 전에 새 완료를 적었다 — 새 차수로" ;; esac
    return 0
  fi
  local ref branch
  ref=$(abase "$(aproj "$aid")" "$am") || { echo "판정 두기 미룸 $aid: 기본 가지 조회 실패"; return 0; }
  branch=${ref#refs/heads/}; branch=${branch#origin/}
  vb_=$(printf 'aid\t%s\nlane\t%s\nrun\t%s\nsha\t%s\nverdict\t%s\nreport\t%s\nround\t%s\nbase\t%s\n' "$aid" "$ln" "$prun" "$sha" "$av" "$rep" "$rnd" \
    "$branch")
  vh_=$(printf '%s\n' "$vb_" | shasum -a 256 | cut -d ' ' -f 1) vz_=$(printf '%s\n' "$vb_" | wc -c | tr -d ' ')
  vo_=$(printf '%s\n' "$vb_" | onm -i "$am" "$(s_vput)" "~/.local/share/hc-fill/verdicts/$ln" "$aid" "$vh_" "$vz_") ||
    { echo "판정 두기 미룸 $aid → $am: 원격 쓰기 실패, 다음 바퀴에"; return 0; }
  crash after-verdict-file
  case $vo_ in
    CONFLICT) a_esc "$aid" conflict "$ln 감사 ${rnd}번째 판정 파일 자리에 다른 내용이 있다($am ~/.local/share/hc-fill/verdicts/$ln/$aid.md) — 지휘 판단"; return 0 ;;
    BADINPUT) echo "판정 두기 미룸 $aid → $am: 받은 내용이 잘렸다(크기·해시 다름), 다음 바퀴에 다시"; return 0 ;;
    WROTE* | ALREADY*) ;;
    *) echo "판정 두기 미룸 $aid → $am: 알 수 없는 응답 「${vo_}」"; return 0 ;;
  esac
  set -- $vo_
  case $2 in taken | landing) aupd "$aid" 8 taken; echo "판정 수거됨 $an → $ln ($2)"; a_cleanup_now; return 0 ;; void) aupd "$aid" 8 superseded; return 0 ;; esac
  now_=$(date +%s)
  case $alc in '' | - | *[!0-9]*) ;; *) d_=$((now_ - alc)); [ $d_ -le 600 ] || d_=600; awt=$((${awt:-0} + d_)) ;; esac   # 한 바퀴 몫은 10분까지
  aupd "$aid" 12 "${awt:-0}" 13 "$now_"
  case $alw in '' | - | *[!0-9]*) alw=0 ;; esac
  if [ $((now_ - alw)) -ge $((WAKE * 60)) ]; then
    if ! lalive "$pa" "$ln"; then echo "깨우기 미룸 $an: 레인 터미널 $pa 이 이 바퀴 목록에 없다"
    elif perl -e 'alarm shift; exec @ARGV' 20 hc send "$pa" "hc-fill: 감사 판정이 왔다 — 이 작업 트리에서 hc-lane verdict $aid 를 돌리고 그 출력대로 한다 (판정 $aid)" >/dev/null 2>&1 </dev/null; then
      awk_=$((${awk:-0} + 1)); aupd "$aid" 11 "$awk_" 14 "$now_"; echo "깨움 $ln $pa — 판정 $aid ($av, ${awk_}번째)"
    else echo "깨우기 실패 $ln $pa — 다음 바퀴에(판정은 파일에 있다)"; fi
  fi
  if [ "${awt:-0}" -ge $((ACKMAX * 60)) ] && ! inlist noack "${afl:--}"; then
    aesc "$ln" "$(qown "$ln")" "$ln 감사 ${rnd}번째 판정($av)을 ${ACKMAX}분 넘게 수거하지 않는다 — 지휘 판단 ($pa: hc-lane verdict $aid)" "hf:$aid:noack" &&
      aupd "$aid" 16 "${afl:+$afl,}noack"
  fi
}
# a_cleanup: 끝난 차수의 감사 줄을 대기열에서 뺀다 — 안 뜬 줄(wait)은 바로, 뜬 줄은 그 터미널이 쉬거나 없으면(정리 주소를 지휘에 한 번)
a_cleanup_now() { ast=taken; a_cleanup; }
a_cleanup() {
  cn_=$an; [ "$prun" != - ] || cn_=$(audname "$ln")   # 옮긴 옛 기록은 옛 이름 줄
  qr_=$(qrow "$cn_"); [ -n "$qr_" ] || return 0
  IFS=$'\t' read -r _ _ _ _ _ qs_ qm_ qa_ _ _ _ _ _ _ qk_ _ <<<"$qr_"
  [ "$qk_" = "audit=$ln" ] || return 0
  [ $dry = 0 ] || { echo "${P}정리할 감사 줄 $cn_ ($qs_)"; return 0; }
  if [ "$qs_" = up ]; then
    aok "$qm_" || return 0
    if lalive "$qa_" "$cn_"; then case $(alive "$qa_") in
      done | idle)
        aesc "$ln" "$(qown "$ln")" "감사 터미널 $qa_ 정리 필요($cn_, 차수 $aid ${ast}) — 정리할 때 이름과 차수를 다시 확인한다" "hf:$aid:cleanup" || return 0 ;;
      *) return 0 ;;
    esac; fi
  fi
  qlock _rmx "$cn_" "$qs_" >/dev/null 2>&1 && echo "감사 줄 뺌 $cn_ (차수 $aid ${ast})"
}
# 새 차수: 쓰기 레인(up·auto)이 그 실행으로 적은 최신 「완료 <sha>」가 아직 감사되지 않았으면(완료 뒤 중간 보고가 끼어도 — main-split 434c5495)
a_open() {
  n=$1 proj=$2 m=$3 la=$4 lo=$5
  l0_=$(lastev "$n" "$la" 완료); [ -n "$l0_" ] || return 0
  pget "$la" || return 0
  lc_=$(runlast "$n" "$la" "$PRUN" 완료); [ -n "$lc_" ] || return 0
  s_=$(printf %s "$lc_" | cut -f 6)
  awk -F'\t' -v n="$n" -v r="$PRUN" -v a="$la" -v s="$s_" 'NF >= 17 && ($2 "")==n && $4==r && $7==a && (s == "-" ? $5 == "-" : index($5, s) == 1) {f=1} END {exit !f}' "$AT" 2>/dev/null && return 0
  # 옮긴 옛 기록이 같은 sha를 이미 다뤘으면 다시 열지 않는다(이미 보낸 판정)
  [ "$s_" = - ] || ! awk -F'\t' -v n="$n" -v s="$s_" 'NF >= 17 && ($2 "")==n && $4=="-" && (index($5, s) == 1 || index(s, $5) == 1) {f=1} END {exit !f}' "$AT" 2>/dev/null || return 0
  # 이 레인의 끝나지 않은 차수: 감사가 아직 돌면 미룸(감사 둘이 함께 돌지 않게), 아니면 그 차수를 끝낸다(deliver는 void를 먼저)
  while IFS=$'\t' read -r -u 8 oa_ _ _ _ osh_ om_ _ ost_ _; do
    [ -n "$oa_" ] || continue
    case $ost_ in
      opening | queued)
        IFS=$'\t' read -r _ _ _ _ _ oqs_ _ oqa_ _ <<<"$(qrow "audit-$oa_")"
        if [ "${oqs_-}" = up ]; then case $(alive "$oqa_") in gone | done | idle) ;; *) echo "자동 감사 미룸 $n: 새 완료 $s_ — 지난 감사($oa_ ${osh_:0:8})가 아직 돈다"; return 0 ;; esac; fi
        [ $dry = 1 ] || aupd "$oa_" 8 superseded ;;
      deliver)
        [ $dry = 0 ] || continue
        a_void "$oa_" "$n" "$om_"; case $? in 0) aupd "$oa_" 8 superseded ;; 2) ;; *) echo "자동 감사 미룸 $n: 지난 판정 $oa_ 취소를 확인하지 못했다"; return 0 ;; esac ;;
      escalate:*) echo "자동 감사 미룸 $n: 지난 차수 $oa_ 를 지휘에 넘기는 중"; return 0 ;;
    esac
  done 8<<<"$(awk -F'\t' -v n="$n" 'NF >= 17 && ($2 "")==n' "$AT" 2>/dev/null)"
  aid=$(newid a); while [ -n "$(arow "$aid")" ] || [ -e "$DATA/briefs/audit/$aid.md" ]; do aid=$(newid a); done
  rnd=$(( $(awk -F'\t' -v n="$n" -v r="$PRUN" 'NF >= 17 && ($2 "")==n && $4==r {k++} END {print k + 0}' "$AT" 2>/dev/null) + 1 ))
  afx=$(awk -F'\t' -v n="$n" -v r="$PRUN" 'NF >= 17 && ($2 "")==n && $4==r {f=$9} END {print f + 0}' "$AT" 2>/dev/null); afx=${afx:-0}   # 기록이 아직 없으면 0
  if [ "$s_" = - ]; then   # 완료에 sha가 없으면 감사할 커밋을 모른다 — 지휘에 한 번
    [ $dry = 0 ] || { echo "${P}지휘로 넘길 $n: 완료에 sha 없음"; return 0; }
    aadd "$aid" "$n" "$rnd" "$PRUN" - "$m" "$la" escalate:nosha "$afx" - 0 0 - - 0 - "자동 감사 못 함 $n: 완료에 sha 없음 — 지휘 판단 ($la)"
    a_send_esc "$aid"; return 0
  fi
  # 40자 sha: 부모 기계·부모 작업 트리에서. 「없음」은 rev-parse 종료 1에 오류 출력이 없을 때만, 그 밖은 모름
  cw_=$(printf '%s\n' "$TERMS" | awk -F'\t' -v a="$la" '$2==a {print $6; exit}')
  case $cw_ in '' | -) cw_=$(repo_of "$proj" "$m"); cw_=${cw_%/}.wt/$n ;; esac
  so_=$(onm "$m" 'e=$(git -C "$d" rev-parse --verify --quiet "$1^{commit}" 2>&1); r=$?; if [ $r = 0 ]; then echo "OK $e"; elif [ $r = 1 ] && [ -z "$e" ]; then echo NONE; else exit 2; fi' "$cw_" "$s_") ||
    { echo "자동 감사 미룸 $n: $m 에서 완료 sha $s_ 를 풀지 못했다"; return 0; }
  case $so_ in
    "OK "*) s40_=${so_#OK } ;;
    NONE) [ $dry = 0 ] || return 0
      aadd "$aid" "$n" "$rnd" "$PRUN" "$s_" "$m" "$la" escalate:nosha "$afx" - 0 0 - - 0 - "자동 감사 못 함 $n: 완료 sha $s_ 가 작업 트리($cw_)에 없다 — 지휘 판단 ($la)"
      a_send_esc "$aid"; return 0 ;;
    *) echo "자동 감사 미룸 $n: 완료 sha 응답 「${so_}」"; return 0 ;;
  esac
  case $s40_ in *[!0-9a-f]*) echo "자동 감사 미룸 $n: sha 꼴 「${s40_}」"; return 0 ;; esac
  [ ${#s40_} = 40 ] || { echo "자동 감사 미룸 $n: sha 꼴 「${s40_}」"; return 0; }
  if [ $dry = 1 ]; then echo "${P}넣을 감사 audit-<새 차수> ← $n ${s40_:0:8} (${rnd}번째, $m)"; return 0; fi
  # 지시문을 확정하기 전에 차수를 기록해야 재시작해도 같은 ID로 이어 간다.
  printf '%s\t%s\n' "$aid" "$proj" >>"$DATA/audit-proj.tsv" || return 1
  aadd "$aid" "$n" "$rnd" "$PRUN" "$s40_" "$m" "$la" opening "$afx" - 0 0 - - 0 - - || return 1
  crash after-opening
  abrief "$aid" "$n" "$m" "$s40_" "$proj" || return 0
  crash after-brief
  a_put "$aid" "$n" "$m" "$proj" || return 0
  crash after-put
  aupd "$aid" 8 queued
  echo "감사 넣음 audit-$aid ← $n ${s40_:0:8} (${rnd}번째, $m)"
}
audit_tick() {
  [ -n "$AUD" ] || return 0
  [ -s "$Q" ] || [ -s "$AT" ] || return 0
  amigrate
  [ -z "$EVBAD" ] || { echo "자동 감사 미룸: hc-lane events 실패, 다음 바퀴에"; return 0; }   # 사건은 ev_tick이 읽었다
  AUNK=$(printf '%s\n' "$EV" | sed -n 's/^모름 //p')
  aok() { [ "$1" = "$LOCAL" ] || { printf '%s\n' "$CONN" | grep -Fxq -- "$1" && ! printf '%s\n' "$AUNK" | grep -Fxq -- "$1"; }; }
  # 1) 있는 차수: 끝나지 않은 것을 잇고, 끝난 것의 감사 줄을 정리
  while IFS=$'\t' read -r -u 6 aid ln rnd prun sha am pa ast afx av awk awt alc alw anv afl _; do
    [ -n "$aid" ] || continue
    a_round
  done 6<<<"$(awk -F'\t' 'NF >= 17' "$AT" 2>/dev/null)"
  # 2) 새 차수: 쓰기 줄(up·auto)의 터미널이 살아 있는 것
  while IFS=$'\t' read -r -u 6 n proj _ _ _ _ m la _ _ _ lo _ _ k _; do
    [ "$k" = auto ] && [ "$(alive "$la")" != gone ] || continue
    aok "$m" || { echo "자동 감사 미룸 @$m: 연결 안 됨"; continue; }
    a_open "$n" "$proj" "$m" "$la" "${lo:--}"
  done 6<<<"$(awk -F'\t' '$6=="up" && $15=="auto"' "$Q" 2>/dev/null)"
}

# mgres <레인> <제출 ID>: 병합기 기계(MGM)가 그 레인 이름·그 제출 ID로 적은 마지막 결과(착지·막힘) 사건 줄, 없으면 빈 값
mgres() {
  [ -n "$2" ] || return 0
  printf '%s\n' "$EV" | awk -F'\t' -v n="$1" -v M="$MGM" -v sid="$2" \
    'BEGIN { k = split(M, a, " "); for (i = 1; i <= k; i++) m[a[i]] = 1 }
     ($4 "") == n && ($5 == "착지" || $5 == "막힘") && ($2 in m) {
       msg = $7; gsub(/,/, " ", msg); if (index(" " msg " ", " 제출 ID " sid " ")) r = $0
     } END { print r }'
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
    l=$(joblast "$n" "$a")
    if [ -z "$l" ]; then printf '%s\t-\t-\t-\t-\t%s\tfresh\t%s\n' "$n" "$enow" "$r"; continue; fi
    # 제출 뒤 정리 중간 보고가 끼어도 같은 실행의 병합 결과를 보인다.
    case $(printf %s "$l" | cut -f 5) in
      제출) submission=$l ;;
      중간) submission=$(joblast "$n" "$a" 제출) ;;
      *) submission= ;;
    esac
    if [ -n "$submission" ]; then
      r_=$(mgres "$n" "$(printf %s "$submission" | cut -f 7 | awk '{ print $1 }')")
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
    l=$(joblast "$n" "$a"); [ "$(printf %s "$l" | cut -f 5)" = 주차 ] || continue
    if [ $dry = 1 ]; then echo "${P}주차로 되돌릴 $n ($a)"; continue; fi
    QRID=$r qset "$n" wait - - "$st" parked && echo "주차 $n: $a 가 lane/$n ($(printf %s "$l" | cut -f 6 | cut -c 1-8))에 두고 끝냄 — 자리 비움, 선행 착지 뒤 그 가지에서 이어 띄움"
  done 6<<<"$(awk -F'\t' -v OFS='\t' '$6=="up" && $14=="early" {print $1, $7, $8, $9, $17}' "$Q")"
}

wcnt() { printf '%s\n' "$WC" | awk -F'\t' -v n="$1" '($1 "")==n {print $2; f=1; exit} END {if (!f) print 0}'; }
