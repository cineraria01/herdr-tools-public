# hc-fill 함수: launch (bin/hc-fill가 읽는다. 함수는 bin/hc-fill 안 원래 정의 순서 그대로)

cnt() { printf '%s\n' "$1" | grep -Fxc -- "$2" || true; }

fetch_proj() {  # fetch_proj <프로젝트>: FR(local 저장소)·FB(기본 가지)를 둔다. 0 됨, 1 설정에 local repo 줄 없음, 2 fetch 실패
  FR=$(repo_of "$1" "$LOCAL"); [ -n "$FR" ] || return 1
  FB=$(project_ref "$FR") || return 2
  case $FB in refs/heads/*) return 0 ;; esac
  if ! inlist "$1" "$FETCHED"; then
    FETCHED="$FETCHED,$1"
    if [ $dry = 1 ]; then echo "${P}fetch 건너뜀: 지금 있는 origin 참조로 판정 ($FR)"
    elif ! git -C "$FR" fetch -q origin; then FETCHBAD="$FETCHBAD,$1"; fi
  fi
  inlist "$1" "$FETCHBAD" && return 2
  return 0
}
# 착지 표식(2026-11-20 fill-dup-guard, 지휘 결정 — carousel-save-race가 선행 main-split의 착지를 커밋 제목 머리로 못 알아봐(「feat(frontend):
# main-split-1 …」) 먼저 띄움 기한을 넘겼다): hc-lane land가 main과 같은 원자 push에 refs/hc-landed/<레인>/<UTC 초>(= 착지 커밋)를 싣는다.
# since <선행>: 줄 ID에 기록한 접수 초. ID가 없는 옛 줄만 10칸의 분 시각으로 복원한다.
since() {
  sa_=$(awk -F'\t' -v n="$1" '($1 "") == (n "") {
    if ($17 ~ /^r[0-9]+-[0-9]+-[0-9]+$/) {split(substr($17,2), a, "-"); print a[1]} else print $10
    exit}' "$Q" 2>/dev/null)
  case $sa_ in *[!0-9]*|'') ;; *) printf '%s\n' "$sa_"; return ;; esac
  [ -n "$sa_" ] && [ "$sa_" != - ] || return 1
  date -j -f %Y-%m-%dT%H:%M:%S "$sa_:00" +%s 2>/dev/null || date -d "$sa_:00" +%s 2>/dev/null   # 초를 0으로(-j -f는 빠진 칸을 지금 값으로 채운다)
}
# lmark <선행> <접수 초>: 기준 가지에 든 표식은 0, 없으면 1, 조회 실패는 2.
lmark() {
  local refs candidates sha rc missing=1
  LMS= LMFROM=$2
  case $FB in origin/*) ;; *) return 1 ;; esac
  refs=$(git -C "$FR" ls-remote origin "refs/hc-landed/$1/*" </dev/null 2>/dev/null) || return 2
  candidates=$(printf '%s\n' "$refs" | awk -v p="refs/hc-landed/$1/" -v t="$2" 'index($2,p)==1 {
    s=substr($2,length(p)+1); if (s ~ /^[0-9]+$/ && s+0>=t+0) print $1}') || return 2
  while read -r sha; do
    [ -n "$sha" ] || continue
    git -C "$FR" merge-base --is-ancestor "$sha" "$FB" 2>/dev/null; rc=$?
    case $rc in 0) LMS=$sha; return 0 ;; 1) ;; *) missing=2 ;; esac
  done <<<"$candidates"
  return "$missing"
}
# ls는 원격을 읽지 않고 마지막 바퀴에서 확인한 표식만 로컬 기본 가지와 다시 견준다.
cached_mark() {
  local stamp sha
  [ -s "$DATA/after-landed.tsv" ] && stamp=$(since "$2") || return 1
  sha=$(awk -F'\t' -v p="$1" -v n="$2" -v t="$stamp" '$1==p && ($2 "")==n && $3==t {print $4; exit}' "$DATA/after-landed.tsv")
  [ -n "$sha" ] && git -C "$FR" merge-base --is-ancestor "$sha" "$FB" 2>/dev/null
}
# 선행의 제목이나 표식에서 확인한 착지. 조회 오류는 2로 넘긴다.
landing_hit() {
  local since_epoch
  hit=$(landed "$1") || return 2
  [ -z "$hit" ] || return 0
  since_epoch=$(since "$1") || return 1
  lmark "$1" "$since_epoch" || return $?
  hit="${LMS:0:8} — 착지 표식"
}
after_ok() {  # after_ok <레인> <프로젝트> <쉼표 목록>: 0 다 들어감, 1 아직(기본 가지는 b), 2 판정 못 함
  fetch_proj "$2"
  case $? in
    1) echo "선행 $1: 판정 못 함(설정에 $2 의 $LOCAL repo 줄이 없다)"; return 2 ;;
    2) echo "선행 $1: 판정 못 함(git fetch 실패: $FR)"; return 2 ;;
  esac
  b=$FB ok=0
  for a in $(printf %s "$3" | tr , ' '); do
    # 들어감 = 커밋 제목 머리 규칙(지금 그대로) 또는 그 선행 줄을 넣은 뒤의 착지 표식이 origin/<기본>의 조상(대기열에 그 줄이 있을 때)
    landing_hit "$a"
    case $? in
      0) echo "선행 $1 ← $a: 들어감 ($hit)" ;;
      1) echo "선행 $1 ← $a: 아직 ($b)"; ok=1 ;;
      *) echo "선행 $1 ← $a: 판정 못 함(커밋·착지 표식 조회 실패)"; return 2 ;;
    esac
  done
  return $ok
}
# lane_landed <레인 이름> <cwd>: cwd의 <저장소>.wt/… 앞 폴더 이름과 repo 줄 경로의 이름이 같은 프로젝트에서 착지했으면 0(LHIT=커밋 줄).
# 이름이 레인 꼴이 아니거나 프로젝트를 못 찾거나 fetch 실패면 1(판정 못 함 = 셈). 하위 셸로 부르지 않는다(FETCHED가 바퀴에 남게)
lane_landed() {
  LHIT=; okname "$1" || return 1
  lp=${2%%.wt/*}; lp=${lp##*/}
  lp=$(awk -v n="$lp" '$1=="repo" { x = $0; sub(/^[ \t]*repo[ \t]+[^ \t]+[ \t]+[^ \t]+[ \t]+/, "", x); sub(/[ \t]+$/, "", x); sub(/\/+$/, "", x); sub(/.*\//, "", x)
    if (x == n) { print $2; exit } }' "$CONF")
  [ -n "$lp" ] && fetch_proj "$lp" || return 1
  LHIT=$(landed "$1") && [ -n "$LHIT" ]
}

# 자리를 차지한 에이전트(기계 한 줄에 하나). working·blocked 등은 늘 차지. done·idle은 hc-lane 작업 트리(<저장소>.wt/…)가
# 그 기계에 남은 쓰기 레인(감사·착지를 기다림)만. 읽기 전용(--read)·작업 트리가 정리된 레인은 아니다. 셈 칸이 all인 기계는 다 센다.
wt_there() {  # wt_there <기계> <폴더> → yes|no|? (확인 못 하면 ?)
  if [ "$1" = "$LOCAL" ]; then [ -d "$2" ] && echo yes || echo no; return; fi
  printf '%s\n' "$CONN" | grep -Fxq -- "$1" || { echo '?'; return; }
  v=$(hc exec --on "$1" -- sh -c '[ -d "$1" ] && echo yes || echo no' _ "$2" 2>/dev/null)
  case $v in yes | no) echo "$v" ;; *) echo '?' ;; esac
}

agents_on() { echo $(( $(cnt "$SEATS" "$1") + $(cnt "$NOW" "$1") )); }
lanes_on() {  # hc-fill이 그 기계에 띄워 살아 있는 레인 수. 자동 감사 줄은 빼고 센다(감사 줄은 레인상한도 안 봄 — 교착, 재감사
  k=0          # [높음] 1). 감사를 기다리는 쓰기 레인은 센다: 지적 뒤 다시 일하니 착지 전까지 레인상한 자리를 쥔다(재감사 [중간] 1)
  while IFS=$'\t' read -r a an; do   # 주소+이름(주소 재사용)
    [ -n "$a" ] || continue
    printf '%s\n' "$TERMS" | awk -F'\t' -v a="$a" -v n="$an" '$2==a && ($4 "")==n {f=1} END {exit !f}' && k=$((k + 1))
  done <<<"$(printf '%s\n' "$ROWS" | awk -F'\t' -v m="$1" '$6=="up" && $7==m && $15 !~ /^audit=/ {print $8 "\t" $1}')"
  echo $((k + $(cnt "$NOWL" "$1")))
}

status_ok() {  # 띄운 터미널의 hc get status가 WAIT초 안에 잡히는지
  end=$(( $(date +%s) + WAIT ))
  while :; do
    if created_=$(hc get "$1" --json 2>/dev/null | jq -er 'select((.status | type) == "object") | .created_ms // "-"'); then
      case $created_ in ''|*[!0-9]*) START_RUN=- ;; *) START_RUN=${1##*/}@$created_ ;; esac
      return 0
    fi
    [ "$(date +%s)" -lt "$end" ] || return 1
    sleep "$POLL"
  done
}

# 시도 기록(이름·줄 ID·기계·먼저·감사·확정 주소). 대기열에서 빠져도 그 터미널이 끝날 때까지 보존한다.
tryput() {
  [ $dry = 0 ] || return 0
  local file=$DATA/try.tsv
  { { [ ! -e "$file" ] || awk -F'\t' -v n="$1" '($1 "")!=n' "$file"; } &&
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "${4:--}" "${5:--}" "${6:--}" "${7:--}"; } >"$file.tmp" && mv -f "$file.tmp" "$file"
}
# 줄을 교체했어도 이전 시도의 기계가 끊겼거나 그 이름의 터미널이 아직 있으면 기다린다.
try_old_busy() {
  local machine addr
  read -r machine addr <<<"$(awk -F'\t' -v n="$1" -v r="$2" '($1 "")==n && ($2 "")!=r {print $3, $6; exit}' "$DATA/try.tsv" 2>/dev/null)"
  [ -n "$machine" ] || return 1
  [ "$machine" = "$LOCAL" ] || printf '%s\n' "$CONN" | grep -Fxq -- "$machine" || return 0
  printf '%s\n' "$TERMS" | awk -F'\t' -v m="$machine" -v a="$addr" -v n="$1" '$1==m && ($4 "")==n && (a=="" || a=="-" || $2==a) {f=1} END {exit !f}'
}
try_prune() {
  local file=$DATA/try.tsv
  [ $dry = 0 ] && [ -s "$file" ] || return 0
  printf '%s\n' "$TERMS" | CONN_="$CONN" awk -F'\t' -v loc="$LOCAL" '
    BEGIN {m[loc]=1; k=split(ENVIRON["CONN_"],x,"\n"); for(i=1;i<=k;i++) m[x[i]]=1}
    FILENAME=="-" {seen[$1 SUBSEP $4]=1; next}
    FILENAME==ARGV[2] {q[$1]=1; next}
    ($1 in q) || !($3 in m) || (($3 SUBSEP $1) in seen)' - "${Q}" "$file" >"$file.tmp" && mv -f "$file.tmp" "$file"
}
tryget() { awk -F'\t' -v n="$1" -v r="$2" -v m="$3" '($1 "")==n && ($2 "")==r && $3==m {print $4 "\t" $5; exit}' "$DATA/try.tsv" 2>/dev/null; }   # 먼저<TAB>감사
# 주차한 줄에서 채택할 수 있는 것은 같은 줄의 --resume 시도뿐이다. 주차를 보고한 터미널은 정리가 끝날 때까지 기다린다.
park_candidate() {
  local attempt
  attempt=$(tryget "$1" "$2" "$3")
  if [ "${attempt%%	*}" = resumed ]; then
    [ -z "$EVBAD" ] && ! printf '%s\n' "$EV" | grep -Fxq -- "모름 $3" && pget "$4" || return 2
    [ -n "$(runlast "$1" "$4" "$PRUN" 주차)" ] || return 0
  fi
  case $5 in done|idle) return 1 ;; *) return 2 ;; esac
}
# inproj <프로젝트> <기계> <cwd>: cwd가 그 기계 그 프로젝트 저장소(또는 <저장소>.wt/ 아래)면 0
inproj() { ip_=$(repo_of "$1" "$2"); ip_=${ip_%/}; [ -n "$ip_" ] || return 1; case $3 in "$ip_" | "$ip_"/* | "$ip_".wt/*) return 0 ;; esac; return 1; }
old_report() {  # old_report <기계> <레인>: 같은 이름의 옛 보고가 새 레인 결과로 나가지 않게 .old-<시각>으로 비켜 둔다(지우지 않음)
  onm "$1" '[ ! -f "$d" ] || mv -f "$d" "$d.old-$1"' "~/.local/share/hc-fill/reports/$2.md" "$(date +%Y%m%d%H%M%S)"
}

# 설정 audit_final <안내>(2026-11-20 지휘 — 레인이 hc-fill 「감사 통과」만 보고 최종 감사 전에 착지했다): 그 뒤에 최종 감사가 따로 있으면
# 3번을 「통과여도 착지하지 않는다 — 최종 감사 통과 알림 뒤에만」으로. 이 절이 본문보다 앞서므로 본문의 최종 감사 규칙을 덮지 않게
afin_sec() {
  ASEC=${ASEC/"3. 출력이 「감사 통과 …」면 그 출력의 착지 줄"/"3. 출력이 「감사 통과 …」여도 착지하지 않는다 — 최종 감사($AFIN)의 통과 알림을 기다린다. 그 전에는 hc-lane report도 하지 않는다. 최종 통과를 받으면 그 출력의 착지 줄"}
  ASEC=${ASEC/"4. 출력이 「감사 지적"/"최종 감사가 지적하면 4번처럼 고친다.
4. 출력이 「감사 지적"}
}

launch() {  # launch <레인> <지시문> <기계> <저장소> <되돌릴곳> <주인 지휘|-> → ADDR. 실패면 1
  # 본문을 먼저 읽어 둔다: 못 읽으면 착지 줄만 든 빈 레인을 띄우지 않는다(종료 2 = 이 레인은 이번 바퀴에 그만)
  body=$(cat -- "$2" 2>&1) && [ -n "$body" ] || { body=${body##*: }; echo "실패 $1: 지시문을 읽지 못함(${body:-빈 파일}): $2"; return 2; }
  on=; [ "$3" = "$LOCAL" ] || on=$3
  # 받은편지 줄: 보고 경로에 요청 ID(되돌릴곳 #<ID>) — 같은 이름의 옛 보고가 이 요청 결과로 나가지 않게. 옛 줄(# 없음)은 <레인>.md
  rid=; case ${5:--} in *'#'*) rid=${5##*#} ;; esac
  rsec=; [ "${5:--}" = - ] || rsec="
## 결과 보고 (hc-fill 받은편지 레인)
끝나면(착지했든 못 했든) 이 기계 \`~/.local/share/hc-fill/reports/$1${rid:+.$rid}.md\`에 결과를 쓴다 — 같은 폴더의 다른 이름에 다 쓴 뒤
\`mv\`로 그 이름에 둔다(반쯤 쓴 파일이 보이지 않게). 마지막 줄은 셋 중 하나다:
\`결과: 고침 <sha>\` · \`결과: 넘김 <사유>\` · \`결과: 실패 <사유>\`. hc-fill은 마지막 줄이 이 꼴일 때만 요청한 기계로 되돌려 준다(착지·터미널 상태와 상관없이).
"
  # 자동 감사(설정 audit): 감사 줄은 --read, 쓰기 줄(받은편지·--no-audit 빼고)은 「감사·착지」 절을 붙이고 감사 칸 auto
  rd= asec= LAUTO=
  case ${AK:--} in audit=*) rd=--read ;; -|auto) [ -z "$AUD" ] || [ "${5:--}" != - ] || { asec=$ASEC LAUTO=auto; } ;; esac
  steps=$(hc-lane --print-steps $rd ${on:+--on "$on"} --name "$1" --repo "$4") || { echo "실패 $1 @$3: hc-lane --print-steps"; return 1; }
  [ -z "$rsec" ] || [ -n "$rid" ] || old_report "$3" "$1" || { echo "실패 $1 @$3: 그 기계의 옛 보고를 비켜 두지 못했다"; return 1; }
  runmd() { printf '%s\n\n%s\n' "$body" "$steps"; printf %s "$rsec"; printf %s "$asec"; printf %s "${ESEC-}"; printf %s "${RSEC-}"; }
  sz=$(runmd | wc -c | tr -d ' ')   # 쓴 run 지시문의 크기가 이것과 같아야 한다(본문이 다 들어갔는지)
  if [ -z "$on" ]; then
    rf=$HOME/.local/share/hc-fill/runs/$1-run.md
    mkdir -p "${rf%/*}" && runmd >"$rf" && [ "$(wc -c <"$rf" | tr -d ' ')" = "$sz" ] || { echo "실패 $1 @$3: run 지시문 쓰기"; return 1; }
  else
    rf=$(runmd | hc exec -i --on "$on" -- sh -c \
      'd=$HOME/.local/share/hc-fill/runs && mkdir -p "$d" && cat >"$d/$1" && [ "$(wc -c <"$d/$1" | tr -d " ")" = "$2" ] && printf %s "$d/$1"' _ "$1-run.md" "$sz") && [ -n "$rf" ] ||
      { echo "실패 $1 @$3: run 지시문을 그 기계에 두지 못했다"; return 1; }
  fi
  own=${6:--}; [ "$own" != - ] || own=
  # 주인 지휘가 지금 살아 있나: 대기열 레인은 --after 사슬로 몇 시간 뒤에 뜬다 — 그 사이 지휘가 끝났으면 주인 없이(-, 사건은 모든 지휘에게)
  # 띄우고 watch는 살아 있는 지휘(hc-lane coord)에게(감사 [중간] 1 — 죽은 주소를 넘겨 아무 watch도 안 떴다)
  if [ -n "$own" ] && ! ce=$(hc-lane coord --check "$own" 2>&1 </dev/null); then
    echo "알림: $1 의 주인 지휘 $own 가 지금 없다 — 주인 없이 띄움(사건은 모든 지휘에게) ($(printf '%s\n' "$ce" | tail -n 1))"; own=
  fi
  task=${TASK//\{run\}/$rf}
  eval "set -- \"\$1\" \"\$4\" ${LAUNCH//\{task\}/\"\$task\"}"   # 설정의 launch 틀을 인자로(설정 파일은 믿는다)
  lane=$1 repo=$2; shift 2
  # 지휘 주소를 넘겨 그 지휘의 hc-lane watch가 없으면 hc-lane이 띄우게 한다(launchd에는 $HC_TERMINAL_ID가 없다 — lane-push 감사 [중간] 1).
  # 주소는 설정 coord가 아니라 hc-lane coord(지휘마다 남긴 주소·pid·이름을 지금 hc get과 맞춰 본 것 가운데 가장 최근에 레인을 띄운 지휘 —
  # 2차 감사 [중간] B: 박힌 주소는 코어가 다시 뜨면 남의 터미널이 된다). 안 맞으면 넘기지 않고 한 줄 — 사건은 events에 남는다
  # 대기열에 주인 지휘가 있으면(add한 지휘) 그 지휘가 watch 대상이자 레인 주인(HC_LANE_OWNER → 레인 기록) — 사건은 그 지휘에게만.
  # (2026-11-17 hcfill-owner: 주인 없이 띄운 레인 사건을 다른 지휘가 넘겨받아 넣은 지휘가 2시간 못 받음)
  if [ -n "$own" ]; then wco=$own
  else
    wco=$(hc-lane coord 2>&1 </dev/null) || { echo "알림: 지휘 없음 — 감시(hc-lane watch)를 띄우지 않음, $lane 사건은 events에 남아 지휘가 wait/watch를 걸면 받는다 ($(printf %s "$wco" | tail -n 1))"; wco=; }
  fi
  # 시도 기록(아스트라 2차 ⑤⑥ — 띄운 뒤 up을 적기 전에 run이 죽으면 다음 바퀴의 「이미 떠 있음」 채택이 먼저 띄움·자동 감사 표시를 잃고
  # 이번 실행이 쓴 받은편지 보고를 .old-로 치웠다): 이름·줄 ID·기계·먼저·감사를 띄우기 전에. 채택이 같은 줄 ID·기계면 이것을 쓴다
  [ -z "${RESUME-}" ] || EARLYF=resumed
  [ $dry = 1 ] || tryput "$lane" "${RID-}" "${on:-$LOCAL}" "${EARLYF:--}" "${LAUTO:--}" || return 2
  # HC_LANE_FILL=1: hc-lane이 레인 기록에 fill로 적는다(hc-lane wait가 「완료」를 마지막 보고로 봄)
  if [ -z "$rd" ]; then
    own_add "$lane" "$proj" "${on:-$LOCAL}" "pending:${on:-$LOCAL}" "$pst_" "$SB" "$PAFT" "$PSOLO"
    own_store
  fi
  out=$(HC_LANE_COORD=$wco HC_LANE_OWNER=$own HC_LANE_FILL=1 HERDR_LAND_PRIORITY=$(wcnt "$lane") hc-lane $rd ${RESUME:+--resume} ${on:+--on "$on"} --name "$lane" --repo "$repo" -- "$@" 2>&1); lr=$?
  crash after-launch
  # 5·6·7: 띄웠지만 시작 확인 실패(터미널은 남음) → 주소를 받아 「상태 안 잡힘」 길로(stuck 기록, 그 기계 빼고 다음으로)
  case $lr in 0|5|6|7) ;; *) echo "실패 $lane @${on:-$LOCAL}: hc-lane: $(printf '%s\n' "$out" | tail -n 3 | tr '\n' ' ')"; return 1 ;; esac
  SWHY=; [ $lr = 0 ] || SWHY=$(printf '%s\n' "$out" | grep -m 1 '^hc-lane: 시작 안 됨' | cut -c 1-200)
  ADDR=$(printf '%s\n' "$out" | tail -n 1 | jq -r '.lane // empty' 2>/dev/null)
  [ -n "$ADDR" ] || { echo "실패 $lane @${on:-$LOCAL}: hc-lane 출력에서 주소를 못 읽음: $(printf '%s\n' "$out" | tail -n 1)"; return 1; }
  # 원격은 hc-lane이 맥 창의 그 기계 공간에 탭으로 붙인다(herdr-balance는 원격 터미널을 맥 탭에 못 넣는다). 못 붙였으면 그 알림 줄을 그대로
  printf '%s\n' "$out" | sed -n 's/^hc-lane: \(Herdr 창.*\)/알림: \1/p'
  bco=${own:-$COORD}   # 로컬 레인은 주인 지휘 옆에(없으면 설정 coord)
  if [ -z "$on" ] && [ -n "$bco" ] && command -v herdr-balance >/dev/null 2>&1; then
    i=0; until herdr-balance --main "$bco" --add "$ADDR" >/dev/null 2>&1; do
      i=$((i + 1)); [ $i -lt 5 ] || { echo "알림: 분할 붙이기 실패 $ADDR (herdr-balance --main $bco --add $ADDR)"; break; }; sleep 2
    done
  fi
  [ $lr = 0 ] || return 5
}

# place <레인> <프로젝트> <지시문> <기계제한> <되돌릴곳> <주인> [<선행 — 먼저 띄움일 때>]: machine 차례에서 처음 자리가 남는 기계에.
# placed=1 띄움 · bad=1 지시문을 못 읽어 이 바퀴엔 그만. stuck(그 줄의 남긴것)을 고쳐 적는다. 먼저 띄움이면 ESEC를 run 지시문 끝에 붙인다.
# 쓰기 줄은 터미널 주소를 받은 때(상태 안 잡힘 포함) 실행 기록에 적고 GOTADDR=1(아스트라 2차 높음 1) — 선행·겹침칸은 PAFT·PSOLO
place() {
  placed= bad= GOTADDR=
  while read -r -u 4 m cap lcap _; do
    [ "$4" = - ] || inlist "$m" "$4" || continue
    mhas "$m" "${NEED-}" || continue   # 지시문 「필요:」 능력이 없는 기계
    stk=$(printf '%s\n' "$stuck" | tr , '\n' | grep -c "^$m=")
    [ "$stk" = 0 ] || continue
    repo=$(repo_of "$2" "$m"); [ -n "$repo" ] || continue
    case $AK in audit=*) repo=${repo%/}.wt/${AK#audit=} ;; esac   # 자동 감사: 그 레인 작업 트리에서 --read
    if [ "$m" != "$LOCAL" ] && ! printf '%s\n' "$CONN" | grep -Fxq -- "$m"; then
      inlist "$m" "$OFF" || { echo "건너뜀 $m: 연결 안 됨"; OFF="$OFF,$m"; }
      continue
    fi
    [ "$(agents_on "$m")" -lt "$cap" ] || continue
    # 자동 감사 줄은 레인상한을 보지 않고 레인 수에도 안 든다(그 레인의 자리를 이어 쓴다 — 재감사 [높음] 1)
    case ${AK:--} in audit=*) isa=1 ;; *) isa= ;; esac
    [ -n "$isa" ] || [ "$lcap" = - ] || [ "$(lanes_on "$m")" -lt "$lcap" ] || continue
    if [ $dry = 1 ]; then
      echo "${P}${7:+먼저 }띄울 $1 → $m ($repo)${7:+ 선행 $7}"
      NOW=$(printf '%s\n%s' "$NOW" "$m") placed=1 UPNOW="$UPNOW,$1"
      [ -n "$isa" ] || { NOWL=$(printf '%s\n%s' "$NOWL" "$m"); own_add "$1" "$2" "$m" "dry:$m/$1" "$(utcnow)" "$3" "${PAFT:--}" "${PSOLO:--}"; }; break
    fi
    ADDR= pst_=$(utcnow) EARLYF=; [ -z "${7-}" ] || EARLYF=early
    launch "$1" "$3" "$m" "$repo" "$5" "$6"; lr=$?
    [ $lr != 2 ] || { bad=1; break; }
    [ $lr = 0 ] || [ $lr = 5 ] || continue
    [ -n "$isa" ] || { own_bind "$1" "pending:$m" "$ADDR"; GOTADDR=1; }
    NOW=$(printf '%s\n%s' "$NOW" "$m")   # 상태가 안 잡혀도 자리는 차 있다
    if [ $lr = 0 ] && status_ok "$ADDR"; then
      [ -n "$isa" ] || NOWL=$(printf '%s\n%s' "$NOWL" "$m")
      pe_=; [ -z "${7-}" ] || pe_=early; [ -z "${RESUME-}" ] || pe_=resumed   # 주차에서 이어 띄움은 resumed(다시 먼저 띄움 후보가 아님)
      tryput "$1" "$RID" "$m" "$pe_" "$LAUTO" "$ADDR" "$START_RUN" || die "실행 기록 저장 실패"
      if QRID=$RID qset "$1" up "$m" "$ADDR" "$stuck" "$pe_" "$LAUTO"; then
        if [ -n "${7-}" ]; then echo "먼저 띄움 $1 → $m $ADDR ($repo, 선행 $7)"; else echo "띄움 $1 → $m $ADDR ($repo${RESUME:+, 주차한 가지 lane/$1 에서 이어})"; fi
      else echo "줄 바뀜 $1: 띄우는 동안 대기열 줄이 빠지거나 다시 들어왔다 — 띄운 $m $ADDR 은 옛 줄의 실행이라 새 줄에 적지 않음(지휘가 정리)"; fi
      placed=1 UPNOW="$UPNOW,$1"; break
    fi
    stuck=$([ "$stuck" = - ] && echo "$m=$ADDR" || echo "$stuck,$m=$ADDR")
    QRID=$RID qset "$1" wait - - "$stuck" || { echo "줄 바뀜 $1: 상태 안 잡힘 $m $ADDR 을 새 줄에 적지 않음(지휘가 정리)"; break; }
    echo "상태 안 잡힘 $1 @$m $ADDR: ${SWHY:-${WAIT}초 안에 hc get status 없음}. 그 레인에서 $m 빼고 다음 기계로(그 터미널은 남겨 둠, 확인 뒤 지휘가 정리)"
  done 4<<<"$(machines)"
}

# early_sec <선행 목록> <기본 가지>: 먼저 띄움 레인의 run 지시문 끝에 붙일 절. 선행마다 판정 줄이 따로다
# (2026-11-18 지휘가 손으로 붙일 때 셸 낱말 나누기로 두 선행을 한 줄로 이어 붙였다). 판정은 after_ok와 같은 커밋 제목 머리 규칙.
# 기한은 고정 시각(지금 + 설정 early_wait시간) — 명령 시간 한도로 끊겨 다시 돌려도 늘지 않게
early_sec() {
  dl=$(( $(date +%s) + EWAIT * 3600 ))
  dls=$(date -r "$dl" '+%Y-%m-%d %H:%M' 2>/dev/null || date -d "@$dl" '+%Y-%m-%d %H:%M')
  re1='^([a-z]+(\([^)]*\))?!?: *)?' re2='([^a-z0-9_-]|$)'
  local fetch=
  case $2 in origin/*) fetch='git fetch -q origin && ' ;; esac
  w="until ${fetch}subjects=\$(git log $2 --format=%s)"
  for a in $(printf %s "$1" | tr , ' '); do
    # after_ok와 같은 판정: 제목 머리, 또는 그 선행 줄을 넣은 뒤의 착지 표식 sha가 origin/<기본>의 조상(ls-remote·merge-base 오류면 아직)
    # 파이프 끝은 ( … ) 하위 셸 — { …; exit; }면 zsh(파이프 끝을 지금 셸에서 돈다)에서 그 exit가 기다림 셸째 끝낸다(2026-11-20)
    if [ -n "$fetch" ] && st_=$(since "$a"); then w="$w &&
  { printf '%s\\n' \"\$subjects\" | grep -Eq '$re1$a$re2' || { o=\$(git ls-remote origin 'refs/hc-landed/$a/*') && printf '%s\\n' \"\$o\" | awk '{ s = \$2; sub(/^refs\\/hc-landed\\/$a\\//, \"\", s); if (s ~ /^[0-9]+\$/ && s + 0 >= $st_) print \$1 }' | ( while read -r s; do git merge-base --is-ancestor \"\$s\" $2 && exit 0; done; exit 1 ); }; }"
    else w="$w &&
  printf '%s\\n' \"\$subjects\" | grep -Eq '$re1$a$re2'"; fi
  done
  cat <<EOF

## 먼저 띄움 — 선행 착지 전에 띄웠다 (hc-fill, $(date '+%Y-%m-%d %H:%M'))
자리가 비어 hc-fill이 이 레인을 선행 레인($(printf %s "$1" | sed 's/,/, /g'))이 $2에 들어가기 **전에** 띄웠다. 위 지시문의 본 작업은 선행이 들어간 뒤에 한다.
1. 지금은 준비만: 지시문·계약 읽기, 이 레인 몫 파일의 실패 시험·골든 고정(커밋해도 된다), 선행 레인이 무엇을 바꾸는지 미리 읽기. 선행 레인이 고칠 파일은 아직 고치지 않는다.
2. 준비가 끝나면 자리를 쥐고 기다리지 않는다(2026-11-20 사용자 「노는 워크트리 없이」): 준비를 커밋한 뒤(커밋할 것이 없어도 된다) 이 작업 폴더에서 \`hc-lane park "<준비 한 줄>"\` → 위 착지·정리 절차의 \`hc-lane clean\` 줄 → 턴을 끝낸다. hc-lane report 완료는 적지 않는다. 선행이 들어가면 hc-fill이 주차한 가지(원격 lane/<레인>)에서 이 레인을 다시 띄우고 지시문 끝에 「이어 가기」 절을 붙인다.
3. park가 종료 3(원격 없는 저장소)일 때만 아래 블록을 그대로 한 번에 돌려 선행을 기다린다. 배경 실행 금지(\`&\`·nohup도, 도구의 배경 실행 run_in_background도). 명령 시간 한도는 도구가 허락하는 최대로 준다(예: Bash 도구 timeout 600000). 선행마다 판정 줄이 따로다 — 합치거나 줄을 고치지 않는다. 그래도 한도로 끊기면 같은 블록을 다시 돌린다(기한은 고정 시각이다).
   기한($dls)이 지나면 블록이 \`hc-lane early-wait\`로 hc-fill이 이 기계에 바퀴마다 써 주는 선행 상태를 본다 — 선행이 살아 진행 중이면(작업 중·감사·착지 기다림·대기열에서 기다림) 멈추지 않고 계속 기다린다.
\`\`\`sh
$w
do [ "\$(date +%s)" -lt $dl ] || hc-lane early-wait $(printf %s "$1" | tr , ' ') || { echo "선행 대기 초과"; break; }; sleep 180; done
\`\`\`
4. 「선행 대기 초과」가 찍혔으면(기한 ${dls}·${EWAIT}시간이 지났고 선행이 막힘·끝남·사라짐이거나 상태를 모름) 그 바로 앞 줄들(\`<선행>: <까닭>\`)을 붙여 \`hc-lane report 막힘 - "선행 대기 초과: <선행>: <까닭>"\`을 부르고 멈춘다. 이 레인의 대기열 줄은 「떠 있음」으로 남아 hc-fill이 다시 띄우지 않으니, 지휘가 \`hc-fill rm <레인>\` 뒤 다시 넣는다.
5. 블록으로 기다려 선행이 모두 들어갔으면 \`git rebase $2\` 뒤 본 작업을 한다(준비 커밋은 그대로 둔다).
EOF
}

# resume_sec <레인> <선행 목록> <기본 가지>: 주차한 레인을 그 가지에서 다시 띄울 때 run 지시문 끝 절(본문의 「먼저 띄움」 절은 안 붙는다)
resume_sec() {
  cat <<EOF

## 이어 가기 — 주차한 가지에서 다시 띄웠다 (hc-fill, $(date '+%Y-%m-%d %H:%M'))
이 레인은 먼저 띄워 준비한 뒤 주차했다(hc-lane park). 선행($(printf %s "$2" | sed 's/,/, /g'))이 $3에 들어가 hc-fill이 주차한 가지 origin/lane/$1 에서 이 작업 트리를 만들었다(준비 커밋이 들어 있다).
1. 먼저 \`git fetch origin && git rebase $3\` — 충돌은 양쪽 내용을 살려 푼다. 준비 커밋은 그대로 둔다.
2. 그다음 위 지시문의 본 작업을 끝까지 한다. 다시 주차하지 않는다.
EOF
}
