# hc-lane 함수: start (bin/hc-lane가 읽는다. 함수는 bin/hc-lane 안 원래 정의 순서 그대로)

# 쓰기·읽기 레인 공통(2026-11-19 무인 레인 셋이 rm -rf $S/… · rm -f out/* 에서 Claude Code 권한 창에 막혀 멈췄다)
rm_rule() {
  printf -- '- 임시 파일은 실행마다 `mktemp -d`로 새 폴더를 만들어 쓰고 지우지 않는다. `rm -rf`·`rm -f`에 변수(`$X/…`)나 와일드카드(`*`) 경로를 쓰지 않는다 — Claude Code가 권한 창을 띄워 무인 레인이 막힌다. 꼭 지워야 하면 글자 그대로의 경로나 `"${X:?}"` 꼴로.\n'
}

print_steps() {
  n=0
  s() { n=$((n + 1)); printf '%s. %s\n' "$n" "$*"; }
  printf '### 착지·정리 (hc-lane이 채움)\n'
  printf -- '- 작업 폴더 %s (브랜치 %s, 기준 %s). 편집·명령은 이 폴더 안에서만 한다.\n' "$wt" "$branch" "$blab"
  printf -- '- 커밋은 경로를 지정한다: git add -- <경로…> && git commit -m "<메시지>" -- <경로…> (add -A·commit -a·stash·reset --hard·강제 push 금지)\n'
  rm_rule
  # rebase·검증·push는 한 줄: 앞 단계가 실패하면 push가 나가지 않는다(2026-11-05 레인이 ;로 이어 써서 검증 실패 뒤 push가
  # 나갔고, 따로 된 줄이면 충돌로 멈춘 rebase의 반쯤 된 HEAD도 나간다). 안내는 # 줄로 따로: 대화형 zsh에 붙여 넣어도
  # 명령이 되지 않게 ; > | & 를 쓰지 않는다.
  # 착지는 hc-lane land 한 줄: 저장소의 .claude/herdr-land.sh(있으면) 또는 fetch·rebase·herdr-verify.sh·push. 검증 스크립트가
  # 없으면 실패하고 push하지 않는다(2026-11-16 전환 안전장치: 옛 줄은 검증이 없으면 「건너뜀」 뒤 push했다).
  printf -- '- 알림: 중간·완료·착지·막힘 때 hc-lane report <상태> [<sha>] "<한 줄>"을 부른다(화면 줄과 보고 파일은 그대로 써도 됨). 지휘는 hc-lane wait로 이 사건만 기다린다\n'
  printf -- '  예: hc-lane report 완료 <sha8> "<요약>" · 아래 1이 착지하면 hc-lane report 착지 <sha8> · 막히면 hc-lane report 막힘 - "<까닭>"\n'
  s "cd $(q "$wt") && hc-lane land $base"
  printf '   # 이 한 줄을 그대로 돌린다. 나누거나 세미콜론으로 이어 쓰지 말 것. 앞 단계가 실패하면 push는 나가지 않는다\n'
  printf '   # 저장소에 .claude/herdr-land.sh가 있으면 그 스크립트가 fetch·rebase·검증·push를 하고, 없으면 hc-lane이 같은 순서로 하며 .claude/herdr-verify.sh가 없으면 push하지 않고 실패한다\n'
  printf '   # rebase가 충돌로 멈추면(종료 3) 양쪽 내용을 살려 풀고 고친 파일을 git add 한 뒤 git rebase --continue, 그다음 이 줄을 처음부터 다시\n'
  printf '   # 검증이 실패하면 고쳐 커밋하고 이 줄을 다시. push 거절 횟수(종료 4)나 잠금 기다림 상한(종료 75)으로 끝나면 이 줄을 다시, 최대 20번\n'
  printf '   # 병합기가 켜진 저장소면 이 줄은 큐에 제출만 하고 끝난다(「제출 … 이 레인은 여기서 끝」) — 착지는 병합기가 알리고, report 착지는 적히지 않는다. 아래 clean은 원격 lane/레인이름 에 그대로 있는 가지를 「제출됨」으로 지워 자리를 바로 비운다\n'
  s "cd $(q "$repo") && hc-lane clean --repo $(q "$repo") --branch $branch"
  printf '   # teardown·검사·워크트리·브랜치 정리를 한 번에. clean이 종료 1로 멈추면 출력된 이유를 고치고 같은 줄을 다시 돈다\n'
  printf '   # 폴더를 rm -rf로 직접 지우지 않는다. 보고는 출력 그대로, 건너뜀은 통과가 아니다\n'
  printf '   # 끝내기 전 이 작업 트리 아래에서 도는 배경 프로세스가 없는지 확인한다(pgrep -fl %s). 자기 셸·에이전트는 빼고, 남은 것은 그 정확한 pid로 kill 한다. cwd로만 도는 것(명령 줄에 경로 없음)은 pgrep에 안 보인다\n' "$(q "$wt/")"
}

read_steps() {  # --read(감사·조사) 레인: 판정은 사건으로
  printf '### 판정 알림 (hc-lane이 채움)\n'
  rm_rule
  printf -- '- 판정이 나오면 보고 파일과 함께 이 한 줄을 부른다: hc-lane report 판정 - "<판정 한 줄(예: 감사 완료 <대상 sha8> 높음 0·중간 0·낮음 n)>"\n'
  printf -- '- 막히면 hc-lane report 막힘 - "<까닭>". 지휘는 hc-lane wait로 이 사건만 기다린다\n'
}

# start_check <주소>: 띄운 직후 Claude 폴더 신뢰 질문을 넘기고 hc get status가 잡히는지 본다.
# start=ok|no_status|login|keychain, 종료 0·5·6·7. 실패여도 터미널은 남긴다(사람이 로그인하면 이어질 수 있다 — hc-fill 「상태 안 잡힘」과 같다).
# 신뢰 질문 글자는 2026-11-16 Claude 2.1 실측: 「❯ No, exit」(기본) 「  Yes, I trust this folder」, 그동안 status는 null.
start_check() {
  end=$(($(date +%s) + swait)) tr= why= im= ik=0 last=
  while :; do
    g=$(hc get "$1" --json 2>/dev/null) || g=
    if printf %s "$g" | grep -Eq '"status" *: *\{'; then
      start=ok
      [ "$fwait" != 0 ] && command -v jq >/dev/null 2>&1 || return 0
      if [ "${HC_LANE_FILL-}" = 1 ]; then
        # hc-fill은 두 출력을 함께 받는다. 감시가 그 파이프를 쥐면 다음 기계 배치 판단이 늦어진다.
        { turn_check "$1" </dev/null >&3 2>&1 3>&- & } 3>>"$evdir/watch.log" ||
          { turn_check "$1" </dev/null >/dev/null 2>&1 & }
      else
        turn_check "$1"
      fi
      return 0
    fi
    if printf %s "$g" | grep -Eq '"state" *: *"finished"'; then why="터미널이 끝났다"; break; fi
    scr=$(hc read "$1" 2>/dev/null)
    case $scr in *'Allow external CLAUDE.md file imports?'*)
      [ "$ik" -ge 3 ] || imports_q "$1" || { why="외부 가져오기 질문에 작업 폴더 저장소 밖 경로가 있다"; break; } ;;
    esac
    if [ -z "$tr" ]; then
      case $scr in *'Accessing workspace'*|*'trust this folder'*|*'Do you trust'*|*'one you trust?'*)
        if [ $strust = off ]; then tr=off
        elif printf '%s\n' "$scr" | grep -Fq 'Is this a project you created or one you trust?' &&
          printf '%s\n' "$scr" | grep -Eq '^ *(❯ | +)No, exit *$' && printf '%s\n' "$scr" | grep -Eq '^ *(❯ | +)Yes, I trust this folder *$'; then
          # 커서(❯)가 「No, exit」(기본)이면 아래로 한 칸, 이미 신뢰에 있으면 엔터만
          # 커서가 어느 줄에도 안 보이면(글리프가 다름 등) 「모름」: 엔터가 「No, exit」에 갈 수 있다
          if printf '%s\n' "$scr" | grep -Eq '^ *❯ No, exit *$'; then set -- "$1" down enter
          elif printf '%s\n' "$scr" | grep -Eq '^ *❯ Yes, I trust this folder *$'; then set -- "$1" enter
          else set -- "$1"; fi
          if [ $# = 1 ]; then tr=unknown; echo "hc-lane: 모름 — 시작 화면 확인($1): 신뢰 질문에 커서(❯)가 안 보여 건드리지 않았다" >&2
          elif hc keys "$@" >/dev/null; then tr=passed; echo "hc-lane: 신뢰 질문 넘김($1)"
          else tr=unknown; echo "hc-lane: 모름 — 시작 화면 확인($1): 신뢰 질문 키를 보내지 못했다" >&2; fi
        else tr=unknown; echo "hc-lane: 모름 — 시작 화면 확인($1): 신뢰 질문 같은데 글자가 달라 건드리지 않았다(hc read $1)" >&2
        fi ;;
      esac
    fi
    # 마감이 지나도 한 번은 더 본다: 이 바퀴에 키를 보냈으면 status는 다음 hc get에야 잡힌다
    # (2026-11-19 부하로 한 바퀴가 start_wait를 넘어 신뢰·가져오기 질문을 넘기고도 종료 5가 났다)
    if [ "$(date +%s)" -ge "$end" ]; then [ -z "$last" ] || break; last=1; fi
    sleep "$poll"
  done
  [ -n "$why" ] || why="${swait}초 안에 hc get status가 안 잡혔다"
  case $tr in unknown) why="$why, 신뢰 질문 모름" ;; off) why="$why, 신뢰 질문(lane.auto_trust off)" ;; esac
  case $im in unknown) why="$why, 외부 가져오기 질문 모름" ;; off) why="$why, 외부 가져오기 질문(lane.auto_trust off)" ;; esac
  scr=$(hc read "$1" 2>/dev/null | grep -v '^[[:space:]]*$' | tail -n 15)
  if hit=$(printf '%s\n' "$scr" | grep -iE -m 1 'keychain|errSec'); then start=keychain rc=7 k=키체인
  elif hit=$(printf '%s\n' "$scr" | grep -iE -m 1 'login expired|/login|not logged in|log ?in again|invalid api key|oauth token|authentication_error|unauthorized|(^|[^0-9])401([^0-9]|$)'); then
    start=login rc=6 k=로그인·인증
  else start=no_status rc=5 k= hit=; fi
  hit=$(printf %s "$hit" | sed 's/^[[:space:]]*//' | cut -c 1-160)
  echo "hc-lane: 시작 안 됨 $1: $why${k:+ — 막힘($k): $hit}. 터미널은 남겼다(hc read $1). 다른 기계로 옮기려면 이 레인 작업 폴더를 정리하고 다시 띄운다" >&2
  return $rc
}

# turn_check <주소>: 첫 턴 감시(알림 전용, launch-submit-check 2026-11-20). g는 방금 읽은 hc get, lo_는 레인 주인 지휘. start=ok|noturn, 늘 종료 0.
# 첫 지시는 입력칸이 아니라 실행 인자로 넘긴다(hc-fill launch 틀의 {task}, --brief의 {brief}, 직접 띄울 때 위치 인자)라 입력칸에 남을 길이 없다.
# 그래도 2026-11-19 원격 자동 감사에서 Claude가 인자 첫 지시를 244초 제출하지 않고 idle로 멈춘 드문 경합이 있어, lane.first_turn_wait초 안에
# turn > 0 또는 status working(실측: 첫 턴 동안 working·turn 0, 턴이 끝나야 turn 1)을 한 번도 못 보면 사건 막힘 「첫 턴 없음 <주소>」 한 줄만 남긴다.
# 키는 보내지 않고 화면도 읽지 않는다. 조회 실패·빈 값은 다음 조회로(마감까지). 놓치거나 늦게 알려도 띄우기 결과에는 영향이 없다.
# lane.first_turn_wait 0·jq 없음이면 보지 않는다.
turn_check() {
  cap=$(($(date +%s) + fwait))
  while :; do
    if printf %s "$g" | jq -e '(.turn | type == "number" and . > 0) or (.status | type == "object" and .state == "working")' >/dev/null 2>&1; then
      start=ok; return 0
    fi
    r_=$((cap - $(date +%s))); [ "$r_" -gt 0 ] || break
    sleep "$(awk -v p="$poll" -v r="$r_" 'BEGIN { print (p < r ? p : r) }')"
    r_=$((cap - $(date +%s))); [ "$r_" -gt 0 ] || break
    g=$(capt "$r_" hc get "$1" --json 2>/dev/null) || g=
    [ "$(date +%s)" -le "$cap" ] || break
  done
  start=noturn
  local event_error=
  lane_event 막힘 "첫 턴 없음 $1${on:+ @$on}" || event_error=" 막힘 사건을 적지 못했다."
  echo "hc-lane: 첫 턴 없음 $1${on:+ @$on}: ${fwait}초 안에 turn > 0·working이 없었다.${event_error} 키는 보내지 않았다(화면은 hc read $1)" >&2
  return 0
}
# lane_event <상태> <한 줄>: 띄운 레인 이름으로 이 기계 사건 기록에 한 줄(터미널 칸 -). 주인 지휘가 있으면 --to로 그 지휘 watch만. 실패면 1
lane_event() {
  to_=; [ "${lo_:--}" = - ] || to_=$lo_
  env -u HC_TERMINAL_ID -u HC_LANE "$(cd "$(dirname "$0")" && pwd -P)/${0##*/}" report "$1" - "$2" --name "$name" ${to_:+--to "$to_"} >/dev/null 2>&1 </dev/null
}

# imports_q <주소>: 「Allow external CLAUDE.md file imports?」(CLAUDE.md의 @가져오기가 작업 폴더 밖 — 2026-11-17 .qa/… 폴더에서
# 띄운 레인이 저장소 루트 AGENTS.md 가져오기 질문에서 10분 멈춤). 화면 글자는 Claude 2.1.326 실측: 「External imports:」 아래
# 줄마다 「  <경로>」(~ 줄임·긴 경로는 가운데 … 생략), 넘치면 「… +N imports not shown.」, 고르기는 「❯ No, disable external
# imports」(기본)·「  Yes, allow external imports」. 경로가 모두 작업 폴더의 git 저장소(rev-parse --show-toplevel) 안일 때만
# 그 터미널에 허용을 고른다. 밖·읽을 수 없는 줄(…·생략)이 하나라도 있으면 안 고르고 1(호출자가 종료 5).
# 질문이 남아 있으면 바퀴마다 커서를 다시 보고 3번까지 보낸다(열린 직후 입력은 Claude가 버린다). im=passed|off|unknown
imports_q() {
  [ $strust = on ] || { im=off; return 0; }
  [ "$im" != unknown ] || return 0
  if [ -z "${itop+x}" ]; then
    itop=$(x git -C "$cwd" rev-parse --show-toplevel 2>/dev/null)
    if [ -n "$on" ]; then ihome=$(x sh -c 'printf %s "$HOME"'); else ihome=$HOME; fi
  fi
  bad=
  ps_=$(printf '%s\n' "$scr" | awk '/External imports:/ { f = 1; next }
    f && (/^[[:space:]]*$/ || /Important:/ || /(Yes, allow|No, disable) external imports/) { exit }
    f { sub(/^[[:space:]]+/, ""); sub(/[[:space:]]+$/, ""); print }')
  [ -n "$ps_" ] || bad="(목록을 읽지 못함)"
  while IFS= read -r p_; do
    [ -n "$p_" ] || continue
    case $p_ in "~") p_=$ihome ;; "~/"*) p_=$ihome/${p_#"~/"} ;; esac
    case $p_ in *…*|*/../*|*/..|*/./*) ok_=0 ;; "$itop"|"$itop"/*) ok_=1 ;; *) ok_=0 ;; esac
    [ -n "$itop" ] && [ $ok_ = 1 ] || bad="${bad:+$bad, }$p_"
  done <<EOF
$ps_
EOF
  if [ -n "$bad" ]; then echo "hc-lane: 모름 — 외부 가져오기 질문($bad): 작업 폴더 저장소(${itop:-저장소 아님}) 밖이라 고르지 않았다($1)" >&2; return 1; fi
  if printf '%s\n' "$scr" | grep -Eq '^ *❯ ([0-9]+\. )?Yes, allow external imports *$'; then set -- "$1" enter
  elif printf '%s\n' "$scr" | grep -Eq '^ *❯ ([0-9]+\. )?No, disable external imports *$'; then
    yl=$(printf '%s\n' "$scr" | grep -nE '^ *( +)([0-9]+\. )?Yes, allow external imports *$' | head -n 1 | cut -d: -f1)
    nl=$(printf '%s\n' "$scr" | grep -nE '^ *❯ ([0-9]+\. )?No, disable external imports *$' | head -n 1 | cut -d: -f1)
    if [ -z "$yl" ]; then set -- "$1"
    elif [ "$yl" -gt "$nl" ]; then set -- "$1" down enter
    else set -- "$1" up enter; fi
  else set -- "$1"; fi
  if [ $# = 1 ]; then im=unknown; echo "hc-lane: 모름 — 외부 가져오기 질문($1): 고르기 줄·커서(❯)가 안 보여 건드리지 않았다" >&2
  elif hc keys "$@" >/dev/null; then im=passed ik=$((ik + 1)); echo "hc-lane: 외부 가져오기 허용($1, 저장소 $itop 안)"
  else im=unknown; echo "hc-lane: 모름 — 외부 가져오기 질문($1): 키를 보내지 못했다" >&2; fi
  return 0
}

# auto_watch: 레인·감사를 띄운 지휘(HC_LANE_COORD — hc-fill이 설정 coord로 넘김 —, 없으면 $HC_TERMINAL_ID, 둘 다 없으면 건너뜀)에게 도는 watch가 없으면 이 기계에 하나 띄운다
# (lane-push 2026-11-16 사용자 「감시를 걸지 않은 이런 문제 없도록 코드적으로」 — 지휘가 손으로 감시를 걸거나 다시 걸 일이 없게).
# watch는 지휘 터미널을 parent로(지휘가 끝나면 같이 끝남), --no-notify, 라벨 hc-lane-watch=1(hc-lane=1이 아니라 레인으로 안 셈).
# 원격 레인 사건도 보게 --all. 레인은 이미 떴으니 실패해도 한 줄 알리고 계속한다
auto_watch() {
  co=${HC_LANE_COORD:-${HC_TERMINAL_ID-}}
  [ -n "$co" ] || return 0
  coord_check "$co" || { echo "hc-lane: 지휘 없음 — $cwhy. 감시(watch)를 띄우지 않음(사건은 events에 남는다)" >&2; return 0; }
  wk=$(printf %s "$co" | tr -c 'A-Za-z0-9_-' _)
  wn=lane-watch-$(printf %s "$wk" | tr 'A-Z' 'a-z' | cut -c 1-21)
  ! watch_alive "$evdir/watch.$wk" || return 0
  self=$(cd "$(dirname "$0")" && pwd -P)/${0##*/}
  if [ $dry = 1 ]; then echo "[dry-run] hc run --bg --no-notify --rm --name $wn --label hc-lane-watch=1 --cwd $(q "$HOME") $(q "$self") watch --all --to $co"; return 0; fi
  if hc run --bg --json --no-notify --rm --name "$wn" --label hc-lane-watch=1 --cwd "$HOME" "$self" watch --all --to "$co" >/dev/null 2>&1 </dev/null
  then echo "hc-lane: 감시 $wn 띄움(지휘 $co 에게 레인 사건을 hc send로 밀어 줌)"
  else echo "hc-lane: 감시 $wn 를 띄우지 못했다(이미 그 이름이 살아 있으면 괜찮다) — 지휘는 hc-lane wait --all로 볼 것" >&2; fi
}

# rollback: 띄우기 전에 실패하면(setup·hc run) 방금 만든 워크트리·브랜치를 지운다 — 고유 커밋이 없을 때만.
# 남기면 다음 시도가 「로컬 브랜치가 이미 있다」로 영영 막힌다(2026-11-17 hc-fill travel-c0). 출력은 die에 붙일 한 줄
rollback() {
  k=$(x git -C "$repo" rev-list --count "$bref..refs/heads/$branch" 2>/dev/null) || k=?
  if [ "$k" = 0 ] && x git -C "$repo" worktree remove --force "$wt" >/dev/null 2>&1; then
    if x git -C "$repo" branch -q -d "$branch" >/dev/null 2>&1; then
      echo "방금 만든 워크트리 ${wt}·브랜치 ${branch} 는 지웠다(고유 커밋 없음). 원인을 고친 뒤 같은 명령을 다시 돌린다"
    else   # 워크트리는 이미 없으니 안내는 브랜치 지우기만(감사 [낮음] 2)
      echo "방금 만든 워크트리 ${wt} 는 지웠고 브랜치 ${branch} 는 못 지웠다. 지우려면: ${rx}git -C $(q "$repo") branch -d $branch"
    fi
  else
    echo "워크트리 $wt${on:+ (기계 $on)} 는 남겨 둠(고유 커밋 ${k}개 또는 지우기 실패)${log:+, 기록 $log}. 지우려면: $undo"
  fi
}

# show_remote: 띄운 뒤 그 공간이 이름 없이 생겼으면 기계 이름을 붙인다. 창이 터미널을 보는 데 몇 초 걸려 HC_LANE_POLL초씩 5번까지
show_remote() {
  [ -n "$hsp" ] && [ -n "$addr" ] || return 0
  i=0
  while :; do
    ls_=$(herdr-app ls 2>/dev/null) || { echo "hc-lane: Herdr 창이 없어 $on 공간 이름은 못 붙였다(창을 열면 $addr 가 보인다)" >&2; return 0; }
    # 공간이 있으면 「=<이름>」. herdr-app ls에는 named 칸이 없어(2026-11-17 실측) 이름이 기계 이름과 다르면 붙인다
    st_=$(printf %s "$ls_" | jq -r --arg id "$on/$hsp" '[.spaces[]? | select(.id == $id) | "=" + (.name // "")][0] // empty' 2>/dev/null)
    case $st_ in
      "=$on") return 0 ;;
      =*) herdr-app do space.rename --id "$on/$hsp" --name "$on" >/dev/null 2>&1 && return 0
          echo "hc-lane: Herdr 창의 $on 공간 이름을 붙이지 못했다(herdr-app do space.rename --id $on/$hsp --name $on)" >&2; return 0 ;;
    esac
    i=$((i + 1)); [ $i -lt 5 ] || { echo "hc-lane: Herdr 창에 $addr 공간($on/$hsp)이 아직 안 보인다 — 창을 확인" >&2; return 0; }
    sleep "$poll"
  done
}
