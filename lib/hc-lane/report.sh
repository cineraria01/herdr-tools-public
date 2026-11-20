# hc-lane 함수: report (bin/hc-lane가 읽는다. 함수는 bin/hc-lane 안 원래 정의 순서 그대로)

report_cmd() {
  rn= st= sha= msg= k=0 to_=
  while [ $# -gt 0 ]; do
    case $1 in --name) val "$@"; rn=$2; shift 2; continue ;; --to) val "$@"; to_=$2; shift 2; continue ;; -h|--help) usage ;; esac
    k=$((k + 1)); case $k in 1) st=$1 ;; 2) sha=$1 ;; 3) msg=$1 ;; *) msg="$msg $1" ;; esac; shift
  done
  case " $evstates " in *" $st "*) [ -n "$st" ] ;; *) false ;; esac ||
    { echo "hc-lane report: 상태는 $evstates 가운데 하나: ${st:-(없음)}" >&2; exit 2; }
  case $to_ in *[!A-Za-z0-9_.,/-]*) echo "hc-lane report: --to 지휘 주소 글자: $to_" >&2; exit 2 ;; esac
  # 대문자 sha도 sha로(소문자로 적음). 7~40자가 아닌 16진 낱말(DB·CAFE)은 한 줄 칸으로 그대로
  case $sha in
    ''|-) sha=- ;;
    *[!0-9A-Fa-f]*) msg="$sha${msg:+ $msg}" sha=- ;;   # hc-lane report 막힘 "이유"
    *) if [ ${#sha} -ge 7 ] && [ ${#sha} -le 40 ]; then sha=$(printf %s "$sha" | tr A-F a-f); else msg="$sha${msg:+ $msg}" sha=-; fi ;;
  esac
  m=${HC_LANE_MACHINE:-$(hostname -s 2>/dev/null)}
  # 줄 전체를 1000바이트 안으로: bash printf는 약 1KB씩 나눠 쓰니, 넘으면 여러 레인이 함께 더할 때 줄이 섞여
  # 두 사건이 다 7칸이 아니게 된다(감사 실측). 앞 칸은 64바이트씩, 남는 자리를 한 줄 칸이 쓴다
  # 여덟째 칸 = 그때의 hc turn(그 턴이 끝나 쉬면 turn+1). wait가 「report 뒤 더 일하고 report 없이 쉼」을 가린다
  # 코어가 응답하지 않아도 report가 멈추지 않게 3초 상한(perl alarm, 없으면 그냥), 넘으면 turn -
  g_=; [ -z "${HC_TERMINAL_ID-}" ] || g_=$(capt 3 hc get "$HC_TERMINAL_ID" --json 2>/dev/null)
  tn=$(printf '%s\n' "$g_" | sed -n 's/.*"turn" *: *\([0-9][0-9]*\).*/\1/p' | tail -n 1)
  # 레인 이름: --name → $HC_LANE → hc 터미널 이름(맨 위 "name" — 줄의 마지막 것, 앞의 foreground.name이 아니다) → 작업 트리 가지.
  # 가지가 기본 가지(origin/HEAD·main·master)면 쓰지 않는다: hc-lane clean으로 작업 트리를 지운 뒤 메인 폴더에서 report하면
  # 레인 칸이 「main」이 됐다(2026-11-16 실측 v5-lb → 「main 착지」, 작업 중 발견해 함께 고침)
  lane=${rn:-${HC_LANE-}}
  [ -n "$lane" ] || lane=$(printf '%s\n' "$g_" | sed -n 's/.*"name" *: *"\([^"]*\)".*/\1/p' | tail -n 1)
  if [ -z "$lane" ]; then
    lane=$(git symbolic-ref -q --short HEAD 2>/dev/null)
    ob=$(git symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null); ob=${ob#origin/}
    case " main master ${ob:-main} " in *" $lane "*) lane= ;; esac
  fi
  # 병합기에 제출만 한 커밋의 「착지」는 적지 않는다(감사 land-merge-train-2 중간 1, 병합기 1차 중간 7 — --name이어도): 원격
  # lane/<레인>(제출 push가 고친 추적 참조)이 그 커밋을 담거나 이 기계 제출 기록(<공용 git 폴더>/hc-submitted — push 응답이 끊겨 추적 참조가
  # 없을 때)에 있고, 기본 브랜치에는 없으면 착지는 병합기가 적는다. 거짓 착지는 겹침 소유를 풀고 지표에 남는다
  # 기본 브랜치는 제출 기록의 셋째 칸(제출 때의 대상), 없으면 origin/HEAD(2차 중간 8 — report는 origin_base 정의 전에 돌아 늘 main이었다)
  if [ "$st" = 착지 ] && [ "$sha" != - ] && [ -n "$lane" ]; then
    sb_=$(awk -v l="$lane" -v s="$sha" '$1 == l && index($2, s) == 1 && $3 != "" { b = $3 } END { print b }' "$(git rev-parse --git-common-dir 2>/dev/null)/hc-submitted" 2>/dev/null)
    [ -n "$sb_" ] || { sb_=$(git symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null); sb_=${sb_#origin/}; }
  fi
  if [ "$st" = 착지 ] && [ "$sha" != - ] && [ -n "$lane" ] && {
      git merge-base --is-ancestor "$sha" "refs/remotes/origin/lane/$lane" 2>/dev/null ||
      awk -v l="$lane" -v s="$sha" '$1 == l && index($2, s) == 1 { f = 1 } END { exit !f }' "$(git rev-parse --git-common-dir 2>/dev/null)/hc-submitted" 2>/dev/null; } &&
    ! git merge-base --is-ancestor "$sha" "refs/remotes/origin/${sb_:-main}" 2>/dev/null; then
    echo "hc-lane report: $sha 는 병합기 큐에 제출만 됐다(lane/$lane) — 착지는 병합기가 사건(착지·막힘 --name $lane)으로 적는다. 아무것도 적지 않음"
    return 0
  fi
  tail_="$tab${tn:--}${to_:+$tab$to_}"
  line="$(utc)$tab$(bcut 64 "$(flat "${m:--}")")$tab$(bcut 64 "$(flat "${HC_TERMINAL_ID:--}")")$tab$(bcut 64 "$(flat "${lane:--}")")$tab$st$tab$(flat "$sha")$tab"
  line=$(bcut $((1000 - ${#tail_})) "$line$(flat "${msg:--}")")$tail_
  mkdir -p "$evdir" || die "report: 폴더를 만들지 못했다: $evdir"
  # 더하기만 한다(O_APPEND 한 번). ponytail: 늘기만 하는 파일 — 수만 줄이 되면 오래된 줄을 옮겨 자르는 단계를 더한다
  printf '%s\n' "$line" >>"$evdir/events.tsv" || die "report: 쓰지 못했다: $evdir/events.tsv"
  echo "$(printf %s "$line" | cut -f4) $st: $sha $(printf %s "$line" | cut -f7)"
}
# pick: --all이면 hc machine ls에서 연결된 기계를 ems에, 아닌 기계를 unk에. --on으로 준 기계는 늘 ems에
pick() {
  ems=$ons unk= pbad=0
  [ $all = 1 ] || return 0
  js=$(hc machine ls --json 2>/dev/null) || { echo "hc-lane $ec: hc machine ls 실패 — 이 기계만 본다" >&2; pbad=1; return 0; }
  recs=$(printf '%s\n' "$js" | jrec name state)
  while IFS=$tab read -r mn ms; do
    [ -n "$mn" ] || continue
    case " $ems " in *" $mn "*) continue ;; esac
    if [ "$ms" = connected ]; then ems="$ems $mn"; else unk="$unk $mn"; fi
  done <<EOF
$recs
EOF
}
# nosh <hc exec 오류 글>: 그 기계가 프로그램을 띄우지 못함(spawn_failed + 「cannot start <프로그램>」 — sh·ps가 없는 Windows 등)이면 0.
# 연결은 멀쩡하고 레인 기록도 있을 수 없는 기계다(lane-nosh). 연결 실패·시간 초과·그 밖의 오류는 1
nosh() { case $1 in *'"spawn_failed"'*) case $1 in *'cannot start '*) return 0 ;; esac ;; esac; return 1; }
# rexec <기계> <명령…>: hc exec --on(60초)으로 돌려 표준 출력을 낸다. 실패했는데 nosh면 종료 9(부른 쪽이 조용히 건너뜀)
rexec() {
  rx_m=$1; shift
  rx_e=$(mktemp "${TMPDIR:-/tmp}/hc-lane-rx.XXXXXX") || { hc exec --on "$rx_m" --timeout 60 -- "$@" </dev/null 2>/dev/null; return; }
  hc exec --on "$rx_m" --timeout 60 -- "$@" </dev/null 2>"$rx_e"; rx_r=$?
  [ $rx_r = 0 ] || ! nosh "$(cat "$rx_e")" || rx_r=9
  rm -f "$rx_e"; return $rx_r
}
# evread <끝 줄 수(0이면 전부)>: 이 기계와 ems 기계의 events.tsv(7칸 이상 줄만 — 옛 줄은 turn 칸이 없다). 원격 줄은 기계·터미널 칸을 hc 이름으로.
# 원격은 기계마다 hc exec 한 번. 못 읽은 기계는 「모름 <기계>」, sh를 못 띄우는 기계(nosh)는 레인 기록이 없는 것으로 보고 0줄
evread() {
  rd='f=${XDG_STATE_HOME:-$HOME/.local/state}/hc-lane/events.tsv; [ -f "$f" ] || exit 0; if [ "$1" = 0 ]; then cat "$f"; else tail -n "$1" "$f"; fi'
  sh -c "$rd" hc-lane "$1" | awk -F'\t' 'NF >= 7'
  for mn in $unk; do echo "모름 $mn"; done
  for mn in $ems; do
    o=$(rexec "$mn" sh -c "$rd" hc-lane "$1") || { [ $? = 9 ] || echo "모름 $mn"; continue; }
    printf '%s\n' "$o" | M=$mn awk -F'\t' -v OFS='\t' 'NF >= 7 { $2 = ENVIRON["M"]; if ($3 != "-" && index($3, "/") == 0) $3 = ENVIRON["M"] "/" $3; print }'
  done
}
# stopped <사건 줄들>: 라벨 hc-lane=1 터미널 가운데 멈춘 것을 멈춤 사건 줄로(한 줄 칸 「<상태> turn <n>」). hc ls 한 번.
# 그 터미널(주소와 레인 이름이 같은)의 마지막 사건으로 가린다: 끝남은 착지·판정 그 턴(report turn+1 이하)이거나 hc-sweep
# 정리 뒤(정리됨은 wait 거름이 같은 규칙으로 판정)면, done·idle(report 없이 쉬는
# 레인 — 2026-11-16 사고 꼴)은 완료·착지·막힘·판정이고 지금 turn이 그 사건 turn+1 이하일 때만(그 턴이 끝나 쉼) 빼고,
# blocked·error는 늘 알린다. 이번 바퀴에 못 읽은 기계(모름·hc machine ls 실패)의 레인은 사건을 모르니 blocked·error만
# 그대로 알리고, 쉼·끝남은 「연결 안 됨 <기계> <날짜>」(레인마다 하루 한 번)로 알린다.
# ponytail: report 뒤 같은 지시 안에서 막혔다 풀려도 turn이 올라 거짓 멈춤이 날 수 있다(놓치는 쪽보다 낫다)
# done·idle은 resting을 거친다(유예). blocked·error·끝남은 바로 알린다.
# resting <주소> <커서 열쇠>: 쉬는 레인이 정말 멈췄나(wait-grace, 2026-11-16 실측 — 배경 셸이 도는 동안 턴을 끝낸 Claude
# 레인을 멈춤으로 알렸다. 배경 작업이 끝나면 스스로 이어 간다). 쉰 지 lane.wait_grace분이 지났고, 화면의 마지막 상태 줄
# (lane.wait_line 또는 lane.wait_busy에 걸리는 줄 가운데 맨 아래 — 옛 턴의 끝 줄·본문은 안 봄)에 lane.wait_busy 표시가 없을 때
# 참. 표시가 있어도 쉰 지 lane.wait_busy_max분이 넘으면 참(셸이 죽어 이어 가지 못한 레인을 영영 붙들지 않게).
# 쉰 시각은 이 열쇠(같은 turn의 쉼)를 처음 본 status_ms를 커서에 「#rest」 줄로 남겨 쓴다(done→idle로 status_ms가 새로 찍혀도).
# status_ms를 못 읽으면(hc get 실패·옛 코어) 바로 참, hc read가 실패하면 표시 없음(놓치지 않는 쪽).
# 주소는 hc ls 그대로(<기계>/t…)라 이 기계 코어가 잇는다.
# ponytail: 상한이 30분이라 (1) 이어 간 턴이 ✻ 줄 없이 끝나면 옛 표시 줄이 최대 30분 붙들고, (2) 정상으로 30분 넘게 배경을
# 기다리는 레인은 같은 turn에 거짓 멈춤이 한 번 난다(재감사 [낮음]). 잦으면 lane.wait_busy_max를 올리거나 꼬리 바의 살아 있는
# 개수(「⏵⏵ … · N shells」)를 본다.
# procbusy <주소> <pid>: 그 pid의 자손 가운데 명령 줄에 lane.wait_procs 글자(빈칸으로 나눈 목록, 기본 shell-snapshots —
# Claude Code Bash 도구 셸 꼴 「/bin/zsh -c source …/.claude/shell-snapshots/snapshot-zsh-….sh …」)가 든 것이 살아 있나.
# 0 있음(배경 작업 중), 1 없음, 2 모름(ps 실패). MCP 서버처럼 늘 사는 자손은 그 꼴이 아니라 세지 않는다.
# ps는 기계마다 바퀴에 한 번(이 기계는 ps, 원격은 hc exec --on 한 번) 받아 psc(「<기계><탭><ps 줄>」)에 둔다 — stopped 한 번이 한 바퀴
# psget <주소>: 그 기계(pm)의 ps를 바퀴에 한 번 psc에 받아 둔다. 0 있음, 2 모름(ps 실패)
psget() {
  case $1 in */*) pm=${1%%/*} ;; *) pm=. ;; esac
  case " $psbad " in *" $pm "*) return 2 ;; esac
  case " $psok " in *" $pm "*) return 0 ;; esac
  if [ "$pm" = . ]; then o_=$(capt 20 ps -A -o pid=,ppid=,command= </dev/null 2>/dev/null)
  else o_=$(rexec "$pm" ps -A -o pid=,ppid=,command=); fi
  pr_=$?
  [ $pr_ != 9 ] || { psok="$psok $pm"; return 0; }   # ps를 못 띄우는 기계(nosh): 레인 프로세스가 없는 것으로
  if [ $pr_ != 0 ] || [ -z "$o_" ]; then psbad="$psbad $pm"; return 2; fi
  psc=$psc$(printf '\n%s' "$o_" | sed "s|^|$pm$tab|")
  psok="$psok $pm"
}
# wtbusy <주소> <cwd>: 레인 작업 트리(<…>.wt/<레인>) 경로가 명령 줄에 든 프로세스가 그 기계에 살아 있나(sweep-rename 6 — 배경 검증을
# 기다리며 차례만 끝낸 레인이 「멈춤 done」으로 왔다. 터미널 자손이 아니어도 센다). 작업 트리가 아니거나 ps를 모르면 1(아님)
# ponytail: 명령 줄만 본다(cwd로만 도는 프로세스는 안 보임 — lsof는 바퀴마다 무겁다). 레인 자신의 명령 줄에 그 경로가 있으면
#           lane.wait_busy_max분까지 멈춤이 늦는다
wtbusy() {
  case $2 in /*/?*.wt/?*) ;; *) return 1 ;; esac
  psget "$1" || return 1
  printf '%s\n' "$psc" | M=$pm C=${2%/} LC_ALL=C awk -F'\t' '$1 == ENVIRON["M"] { c = substr($0, length($1) + 2)
      sub(/^[ \t]*[0-9]+[ \t]+[0-9]+[ \t]*/, "", c); c = c " "
      if (index(c, ENVIRON["C"] "/") || index(c, ENVIRON["C"] " ")) { f = 1; exit } }
    END { exit !f }'
}
procbusy() {
  psget "$1" || return 2
  printf '%s\n' "$psc" | M=$pm P=$2 W=$wprocs LC_ALL=C awk -F'\t' '
    $1 == ENVIRON["M"] { r = substr($0, length($1) + 2); n = split(r, f, " "); if (n < 2) next
      c = r; sub(/^[ \t]*[0-9]+[ \t]+[0-9]+[ \t]*/, "", c); par[f[1]] = f[2]; cmd[f[1]] = c }
    END { nw = split(ENVIRON["W"], w, " ")
      for (p in cmd) {
        hit = 0; for (i = 1; i <= nw; i++) if (index(cmd[p], w[i])) hit = 1
        if (!hit) continue
        q = par[p]; k = 0
        while (q != "" && q + 0 > 1 && k++ < 10000) { if (q == ENVIRON["P"]) exit 0; q = par[q] }
      }
      exit 1 }'
}
# hc는 원격 레인의 parent를 묻는 쪽 기준 주소로 준다(watch-parent, 2026-11-17 실측 — 레인 기계 hc get은 「<지휘 기계>/t7」,
# 지휘 기계 hc get은 앞부분 없이 「t7」). 그래서 parent는 그대로 이 기계에서 묻는다(앞부분 없음 = 이 기계 터미널).
# 옛 꼴(바꿔 주지 않은 「<이 기계 이름>/t…」)만 machine_unknown으로 알아보고 앞부분을 떼어 다시 묻는다(3차 감사 [중간] N1).
# pnorm <parent>: 「<기계>/t…」의 앞부분이 hc machine ls에 없는 이름(= 이 기계 자신)이면 뗀 주소(pq_). hc machine ls가 실패하면 그대로
pnorm() {
  pq_=$1
  case $pq_ in */*)
    [ -n "${mls_-}" ] || if mj_=$(capt 20 hc machine ls --json </dev/null 2>/dev/null) && printf '%s' "$mj_" | grep -q '"machines"'
      then mls_=" $(printf '%s\n' "$mj_" | jrec name state | cut -f1 | tr '\n' ' ') "; else mls_=fail; fi
    [ "$mls_" = fail ] || case $mls_ in *" ${pq_%%/*} "*) ;; *) pq_=${pq_#*/} ;; esac ;;
  esac
}
# plive <주소>: 그 터미널(레인의 parent)이 지금 살아 있나. 바퀴(stopped 한 번)마다 주소당 hc get 한 번(pl_에 「주소=0|1|3」).
# 0 살아 있음, 3 그 주소의 기계를 모름(machine_unknown — 이 기계 자신의 이름이 붙은 주소), 1 그 밖(끝남·없음·모름)
plive() {
  case " $pl_ " in *" $1=0 "*) return 0 ;; *" $1=1 "*) return 1 ;; *" $1=3 "*) return 3 ;; esac
  g_=$(capt 10 hc get "$1" --json </dev/null 2>&1)
  if printf '%s\n' "$g_" | sed 's/"foreground" *: *{[^}]*}//; s/"status" *: *{[^}]*}//' | grep -q '"state" *: *"live"'; then pl_="$pl_ $1=0"; return 0
  elif printf '%s\n' "$g_" | grep -q 'machine_unknown'; then pl_="$pl_ $1=3"; return 3; fi
  pl_="$pl_ $1=1"; return 1
}
# palive <parent>: parent를 그대로 먼저 묻고(들어온 연결의 상대는 machine ls에 없어도 그대로 풀린다 — 3차 감사 [낮음] L1),
# machine_unknown일 때만 pnorm으로 앞부분을 떼고 다시 묻는다
# ponytail: 앞부분 없는 parent를 레인 기계 터미널로 보던 판(바꿔 주지 않는 hc)은 안 받는다 — 그런 hc가 나오면 not_found일 때 <레인 기계>/t…도 묻는다
palive() {
  plive "$1"; r_=$?
  [ $r_ = 3 ] || return $r_
  pnorm "$1"; [ "$pq_" != "$1" ] || return 1
  plive "$pq_"
}
resting() {
  g_=$(capt 10 hc get "$1" --json </dev/null 2>/dev/null)
  sm=$(printf '%s\n' "$g_" | sed -n 's/.*"status_ms" *: *\([0-9][0-9]*\).*/\1/p' | tail -n 1)
  [ -n "$sm" ] || return 0
  # 옛 꼴(기계 이름 든 주소) #rest 줄도 nk로 견준다(재감사 [낮음] 3)
  r=$(LC_ALL=C K=$2 awk -F'\t' "$nkf"'$1 == "#rest" && NF >= 3 { k = $2; for (i = 3; i < NF; i++) k = k "\t" $i; if (nk(k) == ENVIRON["K"]) r = $NF } END { print r }' "$cur" 2>/dev/null)
  case $r in ''|*[!0-9]*) printf '#rest\t%s\t%s\n' "$2" "$sm" >>"$cur"; r=$sm ;; esac
  [ "$sm" -ge "$r" ] || r=$sm
  ag=$(($(date +%s) - r / 1000))
  [ $ag -ge $((wgrace * 60)) ] || return 1
  [ $ag -lt $((wbmax * 60)) ] || return 0
  # 작업 트리 아래 배경 실행(터미널 자손이 아니어도)이 살아 있으면 바쁨(sweep-rename 6). lane.wait_procs가 빈 값이면 프로세스 판정과 함께 끔
  [ -z "$wprocs" ] || ! wtbusy "$1" "${3-}" || return 1
  # 터미널 프로그램 pid로 프로세스 판정: 배경 셸이 있으면 바쁨. 없거나 모르면(pid 없음·1 이하·ps 실패) 화면 판정도 본다
  # (셸 없는 배경 작업 — 배경 Agent 등 — 은 화면 표시로만 보인다. 감사 [낮음] 4). 맨 위 "pid"는 foreground 뒤라 마지막 것을
  # 쓴다(맨 위가 null이면 foreground.pid — 터미널 프로그램 자신이거나 그 자손이라 Bash 셸은 여전히 그 아래다)
  tp=$(printf '%s\n' "$g_" | sed -n 's/.*"pid" *: *\([0-9][0-9]*\).*/\1/p' | tail -n 1)
  case $tp in 0|1) tp= ;; esac
  if [ -n "$wprocs" ] && [ -n "$tp" ]; then
    ! procbusy "$1" "$tp" || return 1
  fi
  [ -n "$wbusy" ] || return 0
  ! capt 10 hc read "$1" </dev/null 2>/dev/null | grep -Ei -e "$wline" -e "$wbusy" | tail -n 1 | grep -Eiq -e "$wbusy"
}
# mine <parent>: 이 지휘의 레인인가. 거르지 않으면(--mine 없음, 터미널 밖) 늘 참. parent를 모르면(-·빈 값) 참(fail-open).
# 원격 레인의 parent는 hc가 묻는 쪽 기준으로 주어 보통 앞부분 없는 <t…>, 옛 꼴 <이 기계>/<t…>도 끝의 /<t…>로 맞춘다.
# ponytail: 다른 기계의 지휘가 같은 t 이름이면 섞인다 — 잦으면 기계 이름까지 맞춘다
mine() { [ -z "$myp" ] || case $1 in ''|-|"$myp"|*/"$myp") true ;; *) false ;; esac; }
stopped() {  # stopped <사건 줄들> <이번 바퀴 모름 기계들>
  psc= psok= psbad=
  [ "$lsok" = 1 ] || { echo "hc-lane wait: hc ls 실패 — 이번 바퀴는 멈춘 레인을 못 봤다" >&2; echo '#hc-ls-fail'; return 0; }
  me=${HC_LANE_MACHINE:-$(hostname -s 2>/dev/null)}
  km_=$(lkinds) pl_= mls_=
  printf '%s\n' "$tl" | jrec address name state status turn parent reason cwd | while IFS=$tab read -r a nm s ss tn pa rs cw; do
    mine "$pa" || continue
    # uk=1: 이번 바퀴에 그 기계 events를 못 읽음(모름·hc machine ls 실패). blocked·error는 사건과 상관없이 알리고,
    # 사건이 있어야 하는 판정(쉼·끝남)은 「연결 안 됨 <기계> <날짜>」로 바꿔 레인마다 하루 한 번 알린다(다음 바퀴엔 다시 정상 판정)
    uk=0
    case $a in
      */*) [ $all = 1 ] || case " $ons " in *" ${a%%/*} "*) ;; *) continue ;; esac; mn=${a%%/*}
           [ "${pbad:-0}" = 0 ] || uk=1
           case " $2 " in *" $mn "*) uk=1 ;; esac ;;
      *) mn=${me:--} ;;
    esac
    # 정리됨(hc-sweep이 닫음)은 report가 아니라 마지막 report를 가리지 않는다. sw=1이면 그 뒤에 정리됨이 있었다
    # 착지 뒤 그 턴의 같은 sha 완료는 착지로 친다(turn만 그 완료의 것으로) — $landed 참고
    lr=$(printf '%s\n' "$1" | A=$a N=$nm LC_ALL=C awk -F'\t' "$landed"'$3 == ENVIRON["A"] && $4 == ENVIRON["N"] {
      if ($5 == "정리됨") w = 1; else { lrep($5, $6, NF >= 8 ? $8 : "-"); l = lst["x"] "\t" ltn["x"]; w = 0 } }
      END { print l "\t" (w ? 1 : 0) }')
    last=${lr%%"$tab"*} lt=${lr#*"$tab"} sw=${lr##*"$tab"} lt=${lt%"$tab"*}
    why=
    # 끝남: 착지·판정 그 턴(report turn+1 이하. turn을 모르면 알림 — 놓치는 것보다 거짓 알림 한 번)이면 알리지 않고, 아니면(완료 — 착지 전 감사 기다림, 막힘,
    # 중간, report 없음, 착지·판정 뒤 이어 받은 일) 뜻밖의 종료. hc-sweep 정리 뒤면 그 정리됨 사건이 판정한다(아래 wait 거름).
    # lane.notify no면 hc가 지휘에 종료를 알리지 않으니 이것이 유일한 알림
    # 마지막 보고: 착지·판정, --read·hc-fill 레인(레인 기록 넷째 칸 read·fill)은 완료도(sweep-rename 5). 지휘가 hc kill로 닫은 것
    # (reason killed)은 일부러 닫은 것이라 알리지 않는다 — hc-sweep이 닫은 것은 정리됨 사건이 판정한다
    # 「제출」(병합기 큐에 넣고 끝남 — 착지는 병합기가 알린다)도 정상 끝(병합기 3차 중간 10)
    fin=' 착지 판정 제출 주차 '
    case $(printf '%s\n' "$km_" | A=${a##*/} N=$nm awk -F'\t' '$1 == ENVIRON["A"] && $2 == ENVIRON["N"] { k = $3 } END { print k }') in
      read|fill) fin=' 착지 판정 제출 주차 완료 ' ;;
    esac
    # killed가 지휘의 hc kill인지: parent가 없거나(hc-fill 레인) 아직 살아 있을 때만. parent가 끝났거나 없어졌거나 모르면
    # die_with_parent로 함께 끝난 것이라 알린다(재감사 [중간] M1 — 지휘 칸이 죽어 함께 killed된 구현 레인을 놓쳤다)
    if [ "$s" != live ]; then [ "$sw" = 1 ] || case $fin in
        *" $last "*) case $lt$tn in *[!0-9]*|'') why="뜻밖의 종료($s)" ;; *) [ "$tn" -le $((lt + 1)) ] || why="뜻밖의 종료($s)" ;; esac ;;
        *) why="뜻밖의 종료($s)" ;;
      esac
      kq=0; [ -z "$why" ] || [ "$rs" != killed ] || case $pa in ''|-) kq=1 ;; *) ! palive "$pa" || kq=1 ;; esac
      # 조용히 넘긴 killed는 커서(watch는 sent도)에 그 멈춤 열쇠로 적는다: 지휘가 끝난 뒤 다음 판정에서 parent가 죽어 다시 오지 않게(3차 [낮음] 1)
      if [ $kq = 1 ]; then
        k_="멈춤$tab${a##*/}$tab$(flat "$nm")$tab$why turn $tn"
        inseen "$k_" "$cur" $wsf || { printf '%s\n' "$k_" >>"$cur"; [ -z "$wsf" ] || printf '%s\n' "$k_" >>"$wsf"; }
        why=
      fi
    else case $ss in
        blocked|error) why=$ss ;;
        done|idle) case $last in
            완료|착지|막힘|판정) case $lt$tn in *[!0-9]*|'') ;; *) [ "$tn" -le $((lt + 1)) ] || why=$ss ;; esac ;;
            *) why=$ss ;;
          esac ;;
      esac
    fi
    [ -n "$why" ] || continue
    case $uk$why in 1blocked|1error|0*) ;; *) why="연결 안 됨 $mn $(date -u +%Y-%m-%d)" ;; esac
    case $why in done|idle)
      # 이미 알린 쉼(열쇠는 evkey와 같은 꼴)은 다시 볼 것 없다 — 바퀴마다 원격 hc get·read를 부르지 않게
      k_="멈춤$tab${a##*/}$tab$(flat "$nm")${tab}쉼 turn $tn"
      ! inseen "$k_" "$cur" $wsf || continue
      { [ "$wgrace" = 0 ] && [ -z "$wbusy$wprocs" ]; } || resting "$a" "$k_" "$cw" || continue ;;
    esac
    printf '%s\t%s\t%s\t%s\t멈춤\t-\t%s turn %s\n' "$(utc)" "$mn" "$a" "$(flat "$nm")" "$why" "$tn"
  done
}

# inseen <열쇠> <커서 파일…>: 그 파일들에 같은 사건(nk로 견줌 — 옛 꼴 줄 포함)이 있으면 0
inseen() { k__=$1; shift; LC_ALL=C K=$k__ awk "$nkf"'nk($0) == ENVIRON["K"] { f = 1; exit } END { exit !f }' "$@" 2>/dev/null; }

ev_cmd() {
  ec=$1; shift
  all=0 ons= since= to=110 sts= mn_=0 wto=
  while [ $# -gt 0 ]; do
    case $1 in
      --all) all=1; shift ;;
      --on) val "$@"; case $2 in ''|*/*|*' '*) die "--on 기계 이름이 이상하다: $2" ;; esac; ons="$ons $2"; shift 2 ;;
      --since) val "$@"; since=$2; shift 2 ;;
      --timeout) [ $ec = wait ] || die "--timeout은 wait에서만 쓴다"; val "$@"; to=$2; shift 2 ;;
      --state) [ $ec != events ] || die "--state는 wait·watch에서만 쓴다"; val "$@"; sts=$2; shift 2 ;;
      --mine) [ $ec != events ] || die "--mine은 wait·watch에서만 쓴다"; mn_=1; shift ;;
      --to) [ $ec = watch ] || die "--to는 watch에서만 쓴다"; val "$@"; wto=$2; shift 2 ;;
      -h|--help) usage ;;
      *) die "알 수 없는 인자: $1" ;;
    esac
  done
  case $to in ''|*[!0-9]*) die "--timeout은 분(0 이상 정수): $to" ;; esac
  [ $ec != watch ] || case $wto in ''|-*|*[!A-Za-z0-9_./-]*) die "watch: --to <지휘 터미널 주소>가 없거나 이상하다: $wto" ;; esac
  for x_ in $(printf %s "$sts" | tr ',' ' '); do
    case " $evstates 멈춤 " in *" $x_ "*) ;; *) die "--state는 $evstates 멈춤 가운데: $x_" ;; esac
  done
  case $since in
    ''|*[!0-9smhd]*|[smhd]*) ;;   # 비었거나 그대로 시각(문자열 비교: 2026-11-16T02:00:00Z 꼴)
    *) k=${since%[smhd]} u=${since#"$k"}
       case $k in ''|*[!0-9]*) die "--since는 시각(UTC 2026-11-16T02:00:00Z) 또는 숫자에 s·m·h·d: $since" ;; esac
       case $u in s) k=$k ;; m) k=$((k * 60)) ;; d) k=$((k * 86400)) ;; *) k=$((k * 3600)) ;; esac
       since=$(utc $(($(date +%s) - k))) ;;
  esac
  # wait는 찍은 사건을 seen에 적어 다시 알리지 않는다: 출력이 /dev/null이면 아무도 못 읽은 사건이 「본 것」이 된다(2026-11-17)
  if [ $ec = wait ] && [ /dev/stdout -ef /dev/null ]; then
    echo "hc-lane wait: 출력이 /dev/null이라 사건을 읽을 사람이 없습니다 — seen에 적지 않고 끝냅니다. 백그라운드로 기다리려면 출력을 파일로 받으세요" >&2
    exit 2
  fi
  if [ $ec = events ]; then
    pick
    out=$(evread 0)
    printf '%s\n' "$out" | grep "$tab" | S=$since awk -F'\t' '$1 >= ENVIRON["S"]' | sort -s -t "$tab" -k1,1
    printf '%s\n' "$out" | grep '^모름 '
    return 0
  fi
  # 커서는 지휘(부른 터미널)마다 따로: 한 기계의 지휘 여럿이 서로의 사건을 먹지 않게. 터미널 밖이면 seen.
  # watch는 --to 지휘의 커서를 쓴다(그 지휘의 wait와 함께 — 한쪽이 알린 사건을 다른 쪽이 다시 안 알림)
  # ponytail: 둘이 같은 바퀴에 커서를 읽으면 한 사건을 둘 다 알릴 수 있다(거짓 알림 한 번, 놓치지는 않음)
  me_=${wto:-${HC_TERMINAL_ID-}}
  cur=$evdir/seen${me_:+.$(printf %s "$me_" | tr -c 'A-Za-z0-9_-' _)}
  # watch는 「보냄」 표시(<evdir>/sent — 누구에게든 보냈거나 wait로 받은 사건)도 본 것으로 친다. wait는 자기 커서만
  wsf=; [ $ec != watch ] || wsf=$evdir/sent
  mkdir -p "$evdir" || die "wait: 폴더를 만들지 못했다: $evdir"
  [ $ec != watch ] || watch_lock
  # 지휘 정보 남기기: wait는 부른 터미널, watch는 자기 터미널의 parent가 --to일 때(지휘가 손으로 건 watch) 그 지휘를
  if [ -n "${HC_TERMINAL_ID-}" ]; then
    if [ $ec = wait ]; then coord_set "$HC_TERMINAL_ID"
    else
      pa_=$(capt 10 hc get "$HC_TERMINAL_ID" --json </dev/null 2>/dev/null | sed -n 's/.*"parent" *: *"\([^"]*\)".*/\1/p' | tail -n 1)
      case $pa_ in "$wto"|*/"$wto") wpar=1; coord_set "$wto" ;; esac
    fi
  fi
  # 새 커서는 바닥 시각(--since, 없으면 2시간 전)을 첫 줄에 적는다: 처음 wait가 옛 사건을 쏟지 않게.
  # 레인을 띄운 뒤 2시간 안에 첫 wait를 걸면 그 사이 사건은 놓치지 않는다
  [ -e "$cur" ] || printf '#since\t%s\n' "${since:-$(utc $(($(date +%s) - 7200)))}" >"$cur" || die "wait: 커서 파일을 쓰지 못했다: $cur"
  wgrace=$(cfg lane.wait_grace 10) || exit 1
  case $wgrace in ''|*[!0-9]*) die "설정 lane.wait_grace는 0 이상 정수(분): $wgrace" ;; esac
  wbusy=$(cfg lane.wait_busy 'shells? still running|monitors? still running') || exit 1
  wbmax=$(cfg lane.wait_busy_max 30) || exit 1
  case $wbmax in ''|*[!0-9]*) die "설정 lane.wait_busy_max는 0 이상 정수(분): $wbmax" ;; esac
  wprocs=$(cfg lane.wait_procs shell-snapshots) || exit 1
  wline=$(cfg lane.wait_line '^[[:space:]]*(✻|✶|✳|✢|✽)') || exit 1
  [ -n "$wline" ] || wline='$.^'   # 빈 값: 상태 줄을 안 고름(표시 줄 가운데 맨 아래만). 아무 줄에도 안 맞는 식
  if [ $ec = watch ]; then
    tick=$(cfg lane.watch_tick 30) || exit 1
    widle=$(cfg lane.watch_idle 60) || exit 1
    case $widle in ''|*[!0-9]*) die "설정 lane.watch_idle는 0 이상 정수(분): $widle" ;; esac
    widle=${HC_LANE_WATCH_IDLE:-$((widle * 60))}   # 시험용 초 단위
    idle0=$(date +%s)
    whmax=$(cfg lane.watch_hold_max 30) || exit 1
    case $whmax in ''|*[!0-9]*) die "설정 lane.watch_hold_max는 0 이상 정수(분): $whmax" ;; esac
    whmax=${HC_LANE_WATCH_HOLD_MAX:-$((whmax * 60))} wh0=   # 시험용 초 단위
    wdlg=$(cfg lane.watch_dialog 'Tab/Arrow keys to navigate|Chat about this|Type something|Do you want to proceed\?|Would you like to proceed\?|tell Claude what to do differently') || exit 1
  else tick=60; fi
  tick=${HC_LANE_WAIT_TICK:-$tick}
  case $tick in ''|*[!0-9]*) die "바퀴 간격(lane.watch_tick·HC_LANE_WAIT_TICK)은 0 이상 정수(초): $tick" ;; esac
  end=$(($(date +%s) + to * 60)) told= lsf=0 lf0= lsa=0
  # 기본은 모든 레인 사건. --mine이면 이 지휘($HC_TERMINAL_ID)가 parent인 레인의 사건만(한 기계에 지휘가 여럿일 때).
  # 기본으로 거르면 지휘가 바뀌거나(옛 parent는 죽은 터미널) wait를 다른 터미널에서 부를 때 사건이 다 가려진다(감사 3차 [높음])
  myp=; [ $mn_ = 0 ] || myp=$me_
  gone=0
  while :; do
    # watch: 지휘 터미널이 없거나 끝났으면 그 바퀴는 건너뛰고, 3바퀴 이어지면 스스로 끝낸다(보낼 곳이 없다)
    if [ $ec = watch ]; then
      # 「없음」(not_found)이나 끝난 터미널일 때만 센다. 코어 연결 실패·시간 초과는 모름이라 세지 않고 그대로 돈다(감사 5)
      g_=$(capt 10 hc get "$wto" --json </dev/null 2>&1)
      if printf '%s\n' "$g_" | grep -q '"state" *: *"live"'; then gone=0; wcheck || { wsleep; continue; }
      elif printf '%s\n' "$g_" | grep -Eq '"kind" *: *"not_found"|"address" *:'; then
        gone=$((gone + 1))
        [ $gone -lt 3 ] || { echo "hc-lane watch: 지휘 터미널 $wto 사라짐(hc get ${gone}바퀴 없음·끝남) — 끝낸다" >&2; return 0; }
        wsleep; continue
      fi
    fi
    pick
    # ponytail: 바퀴마다 기계마다 끝 500줄만 본다 — 한 바퀴 사이에 한 기계에 500건 넘게 쌓이면 앞의 것을 놓친다
    out=$(evread 500)
    mu=$(printf '%s\n' "$out" | sed -n 's/^모름 //p' | tr '\n' ' ')
    # 못 읽은 기계는 끊길 때마다 한 번 알린다(다시 읽히면 told에서 빼 다음 끊김도 알림). 다음 바퀴에 다시 읽어 본다.
    # watch는 지휘에게도 한 줄(보내지 못했으면 다음 바퀴에 다시)
    t_=; for mn in $told; do case " $mu " in *" $mn "*) t_="$t_ $mn" ;; esac; done; told=$t_
    for mn in $mu; do
      case " $told " in *" $mn "*) continue ;; esac
      echo "hc-lane $ec: 모름 $mn (연결되면 그때 본다)" >&2
      [ $ec != watch ] || wsend "hc-lane watch: 연결 안 됨 $mn — 다음 바퀴에 다시 본다" || continue
      told="$told $mn"
    done
    evl=$(printf '%s\n' "$out" | grep "$tab")
    cand=$evl
    st_= tl= lsok=1 pmap=
    # hc ls는 바퀴마다 한 번: 멈춤 판정과 parent 거름이 함께 쓴다
    if [ -n "$myp" ] || [ $ec = watch ] || case ",${sts:-멈춤}," in *,멈춤,*) true ;; *) false ;; esac; then
      tl=$(hc ls --all --json --label hc-lane=1 2>/dev/null) || lsok=0
      [ -z "$myp" ] && [ $ec != watch ] || pmap=$(printf '%s\n' "$tl" | jrec address parent)
    fi
    case ",${sts:-멈춤}," in *,멈춤,*) st_=$(stopped "$evl" "$mu"); cand=$(printf '%s\n%s' "$evl" "$st_") ;; esac
    # watch: 이 지휘의 살아 있는 레인(parent가 --to, 모르면 이 지휘 것으로 침)이 있거나 hc ls를 못 읽었으면 쉬는 시계를 새로
    if [ $ec = watch ] && { [ $lsok = 0 ] || printf '%s\n' "$tl" | jrec address state parent | W=$wto awk -F'\t' '
        $2 == "live" && ($3 == "-" || $3 == ENVIRON["W"] || substr($3, length($3) - length(ENVIRON["W"])) == "/" ENVIRON["W"]) { f = 1 }
        END { exit !f }'; } || fillq; then idle0=$(date +%s); fi
    # hc ls가 바퀴마다 실패하면 멈춤·끝남을 영영 못 본다. 3바퀴 이어지면 사건 한 줄로 끝낸다(배경 wait의 stderr는 안 보인다)
    if printf '%s\n' "$st_" | grep -qx '#hc-ls-fail'; then
      lsf=$((lsf + 1)); [ $lsf != 1 ] || lf0=$(utc)
      if [ $lsf -ge 3 ]; then
        l_="$(utc)$tab${HC_LANE_MACHINE:-$(hostname -s 2>/dev/null)}$tab-${tab}hc-lane-wait${tab}멈춤$tab-${tab}hc ls 실패 ${lsf}바퀴(${lf0}부터) — 레인 멈춤·끝남을 못 봄$tab-"
        # watch는 실패가 이어지는 동안 한 번만(lsa=1), 다시 성공하면 「회복」 한 번(감사 [중간] 2 — 3바퀴마다 새 열쇠로 폭주했다)
        if [ "$lsa" = 0 ] && emit "$l_"; then [ $ec = watch ] || return 0; lsa=1; fi
      fi
    else
      lsf=0
      [ "$lsa" = 0 ] || ! wsend "hc-lane watch: hc ls 회복 — 레인 멈춤·끝남을 다시 본다" || lsa=0
    fi
    # 커서는 BEGIN에서 읽는다(빈 파일이면 NR == FNR가 입력 줄까지 먹는다). 「#since<탭>시각」 줄은 바닥
    # 정리됨(hc-sweep이 닫고 hc rm — 목록에서 사라진다)은 그 터미널(주소·레인)의 바로 앞 report가 착지·판정이면 버리고,
    # 아니면 「뜻밖의 종료(정리됨)」 멈춤으로 바꿔 알린다: wait가 안 돌던 틈에 닫힌 보고 없는 레인도 다음 wait에서 나오게.
    # ponytail: 앞 report는 바퀴마다 보는 끝 500줄 안에서 찾는다 — 못 찾으면 알리는 쪽
    # 착지·판정 뒤라도 정리됨 turn이 그 report turn+1보다 뒤면(이어 받은 일) 또는 turn을 모르면 알린다. 같은 turn의 쉼으로 이미 알린 레인은 다시 안 알림.
    # parent 거름(PM: 「주소<탭>parent」 줄)은 찍을지에만 쓴다 — 마지막 report 추적은 모든 줄로
    new=$(printf '%s\n' "$cand" | grep "$tab" | KM=$(lkinds) F=$cur SF=$wsf S=$since W=",$sts," PM=$pmap MY=$myp LC_ALL=C awk -F'\t' -v OFS='\t' "$evkey$nkf$landed"'
      BEGIN { fl = ""; while ((getline l < ENVIRON["F"]) > 0) { if (l ~ /^#since\t/) fl = substr(l, 8); else seen[nk(l)] = 1 }
              if (ENVIRON["SF"] != "") while ((getline l < ENVIRON["SF"]) > 0) seen[nk(l)] = 1
              nk_ = split(ENVIRON["KM"], kl, "\n"); for (i = 1; i <= nk_; i++) { split(kl[i], kf, "\t"); if (kf[1] != "") kind[kf[1] SUBSEP kf[2]] = kf[3] }
              np = split(ENVIRON["PM"], pl, "\n"); for (i = 1; i <= np; i++) { split(pl[i], pf, "\t"); if (pf[1] != "") par[pf[1]] = pf[2] } }
      function num(x) { return x ~ /^[0-9]+$/ }
      { t2 = $3 SUBSEP $4 }
      $5 == "정리됨" { a3 = $3; sub(/^.*\//, "", a3)
        if ((lst[t2] == "착지" || lst[t2] == "판정" || lst[t2] == "제출" || lst[t2] == "주차" || (lst[t2] == "완료" && kind[a3 SUBSEP $4] ~ /^(read|fill)$/)) && num($8) && num(ltn[t2]) && $8 + 0 <= ltn[t2] + 1) next
        if (nk("멈춤\t" $3 "\t" $4 "\t쉼 turn " $8) in seen) next
        $5 = "멈춤"; $6 = "-"; $7 = "뜻밖의 종료(정리됨) " $7 }
      $5 != "멈춤" { lrep($5, $6, NF >= 8 ? $8 : "-", t2) }
      ENVIRON["MY"] != "" && ($3 in par) && par[$3] != "-" && par[$3] != ENVIRON["MY"] && substr(par[$3], length(par[$3]) - length(ENVIRON["MY"])) != "/" ENVIRON["MY"] { next }
      $1 >= ENVIRON["S"] && $1 >= fl && (ENVIRON["W"] == ",," || index(ENVIRON["W"], "," $5 ",")) && !(nk(key()) in seen) && !(nk(key()) in done) { done[nk(key())] = 1; print }' |
      sort -s -t "$tab" -k1,1)
    if [ -n "$new" ] && emit "$new"; then [ $ec = watch ] || return 0; idle0=$(date +%s); fi
    [ $ec = watch ] || [ "$(date +%s)" -lt $end ] || { echo "hc-lane wait: ${to}분 동안 새 사건 없음" >&2; return 75; }
    # watch: 이 지휘의 살아 있는 레인도 새 사건도 없이 lane.watch_idle분이 지나면 끝낸다(다음 레인을 띄울 때 hc-lane이 다시 띄운다)
    [ $ec = wait ] || [ $(($(date +%s) - idle0)) -lt "$widle" ] ||
      { echo "hc-lane watch: $wto 의 살아 있는 레인·새 사건 없이 $((widle / 60))분 — 끝낸다" >&2; return 0; }
    wsleep
  done
}
