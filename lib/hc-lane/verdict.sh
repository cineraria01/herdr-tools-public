# hc-lane 함수: verdict (bin/hc-lane가 읽는다. 함수는 bin/hc-lane 안 원래 정의 순서 그대로)

# ---- 승인 착지 land --verdict <aid>(fill-handoff-integrity) ----
# 감사받은 커밋 S의 변경(갈림점..S)의 patch-id를 시작 표시(landing)에 고정하고, push 직전 HEAD의 변경(<기준>..HEAD)이 같은 patch-id일
# 때만 push한다 — 차이가 rebase뿐일 때만. 충돌 풀이·검증 고침이 내용을 바꾸면 종료 2(새 커밋으로 report 완료 → 새 감사).
# --binary라 바이너리 내용 차이도 가린다(rebase가 바이너리 기준을 바꾸면 같은 변경도 다르게 나온다 — 그때도 새 감사, fail-closed)
# 문맥 줄 없이(-U0): rebase로 바뀐 바로 옆 줄(문맥)은 같은 변경으로 본다 — 문맥 3줄을 넣으면 깨끗한 rebase도 다르게 나온다(실측)
lpatch() {  # lpatch <A> <B> [<경로>]: A..B 변경(경로를 주면 그 경로만)의 patch-id(변경 없음은 -). git 실패는 1
  d_=$(git diff -U0 --binary "$1" "$2" ${3:+-- "$3"}) || return 1
  [ -n "$d_" ] || { echo -; return 0; }
  p_=$(printf '%s\n' "$d_" | git patch-id --stable) || return 1
  p_=${p_%% *}; [ -n "$p_" ] || return 1; echo "$p_"
}
lverdict_start() {
  [ -n "${HC_LANE-}" ] || die "land --verdict: 레인 터미널 안에서 부른다(\$HC_LANE 없음)"
  vload "$HC_LANE" "$lvd"
  case $? in 1) die "land --verdict: 판정 $lvd 가 없다($vf)" ;; 2) die "land --verdict: 판정 파일 꼴이 틀렸다: $vf" ;; esac
  [ "$vv" = pass ] || { echo "hc-lane land: 판정 $lvd 는 통과가 아니다($vv) — 착지하지 않았다" >&2; exit 2; }
  vmyrun || die "land --verdict: 이 터미널의 실행을 못 읽었다(\$HC_LANE_JOB·hc get) — 착지하지 않았다, 같은 줄을 다시"
  [ "$vr" = "$vme" ] || { echo "hc-lane land: 판정 $lvd 는 다른 실행($vr)의 것 — 착지하지 않았다" >&2; exit 2; }
  [ ! -e "$vdn" ] || { echo "hc-lane land: 판정 $lvd 는 이미 착지했다($(cut -f2 "$vdn" | cut -c 1-8))"; exit 0; }
  lst_=$(vstate)
  if [ "$lst_" = none ]; then
    lh_=$(git rev-parse --verify -q HEAD) || die "land --verdict: HEAD를 읽지 못했다(git 실패) — 착지하지 않았다, 같은 줄을 다시"
    [ "$lh_" = "$vs" ] || { echo "hc-lane land: 승인 커밋이 아니다 — 판정 $lvd ${vs:0:8}, HEAD ${lh_:0:8}. 착지하지 않았다. 지금 커밋으로 hc-lane report 완료 ${lh_:0:8} 를 부르고 기다린다" >&2; exit 2; }
    lmb_=$(git merge-base "$vs" "$bref") || die "land --verdict: $blab 와의 갈림점을 못 찾았다 — 착지하지 않았다"
    lpid=$(lpatch "$lmb_" "$vs") || die "land --verdict: 승인 변경의 patch-id를 못 구했다(git 실패) — 착지하지 않았다, 같은 줄을 다시"
    vmark "landing$tab$(utc)$tab$vme$tab$lpid"; case $? in 2) die "land --verdict: 시작 표시를 두지 못했다: $vx" ;; esac
    lst_=$(vstate)
  fi
  case $lst_ in
    landing) [ "$(cut -f3 "$vx")" = "$vme" ] || { echo "hc-lane land: 판정 $lvd 착지를 다른 실행이 시작했다 — 착지하지 않았다" >&2; exit 2; }
      lpid=$(cut -f4 "$vx"); [ -n "$lpid" ] || die "land --verdict: 시작 표시에 patch-id가 없다: $vx"
      echo "hc-lane land: 판정 $lvd(${vs:0:8}) 승인 착지 — 변경 patch-id ${lpid:0:12}" ;;
    void) echo "hc-lane land: 판정 $lvd 는 hc-fill이 취소했다 — 착지하지 않았다. 다음 지시를 기다린다" >&2; exit 2 ;;
    *) echo "hc-lane land: 판정 $lvd 표시가 착지 시작이 아니다($lst_) — 착지하지 않았다" >&2; exit 2 ;;
  esac
}

lverdict_check() {
  lc_=$(lpatch "$bref" HEAD) || die "land: 승인 대조 patch-id를 못 구했다(git 실패) — push하지 않았다, 같은 줄을 다시"
  [ "$lc_" = "$lpid" ] && return 0
  lbad_= LRRN=0
  lpa_=$(git -c core.quotePath=false diff --name-only "$lmb" "$lorig") && lpb_=$(git -c core.quotePath=false diff --name-only "$bref" HEAD) ||
    die "land: 승인 대조 경로를 못 읽었다(git 실패) — push하지 않았다, 같은 줄을 다시"
  lps_=$(printf '%s\n%s\n' "$lpa_" "$lpb_" | sort -u)
  while IFS= read -r lp_; do
    [ -n "$lp_" ] || continue
    la_=$(lpatch "$lmb" "$lorig" "$lp_") && lb_=$(lpatch "$bref" HEAD "$lp_") || die "land: 경로 대조 patch-id 실패(git) — push하지 않았다: $lp_"
    [ "$la_" != "$lb_" ] || continue
    if awk -F'\t' -v p="$lpid" -v f="$lp_" '$1 == p && $2 == f { x = 1 } END { exit !x }' "$lrrf" 2>/dev/null; then LRRN=$((LRRN + 1))
    else lbad_="$lbad_${lbad_:+, }$lp_"; fi
  done <<EOF
$lps_
EOF
  if [ -z "$lbad_" ]; then echo "hc-lane land: rerere로 바뀐 경로 ${LRRN}개 — 승인 변경과 다른 곳이 rerere가 푼 경로뿐이라 이어 간다"; return 0; fi
  echo "hc-lane land: rebase 뒤 변경이 승인 내용과 다르다(rerere가 푼 곳 밖 — $lbad_). push하지 않았다. 지금 커밋으로 hc-lane report 완료 $(git rev-parse --short=8 HEAD) 를 부르고 기다린다(새 감사)" >&2
  exit 2
}
lverdict_precheck() {  # 위임 경로: 위임 전에 같은 기준으로 rebase(rerere 이어 가기 포함)해 차이가 rebase뿐인지(충돌 종료 3, 다르면 종료 2)
  [ -z "$(git status --porcelain --untracked-files=no)" ] || die "land: 커밋 안 된 변경이나 멈춘 rebase가 있다(git status). 커밋하거나 rebase를 마친 뒤 같은 줄을 다시"
  [ $remote = 0 ] || fetchr git fetch -q origin || die "land: git fetch 실패 — 착지하지 않았다"
  rb_ "$bref" || { echo "hc-lane land: 승인 착지 중 rebase 충돌 — 양쪽 내용을 살려 풀고 git add → git rebase --continue 뒤 커밋으로 hc-lane report 완료 <sha>(새 감사)" >&2; exit 3; }
  lverdict_check
}
lverdict_done() {  # 착지 끝 표시(hc-fill이 본다). 못 쓰면 알림만 — 착지는 됐고 착지 사건도 남는다
  [ -n "$lvd" ] || return 0   # 병합기 착지(판정 파일 없음)는 착지 사건 키 hf:<aid>:landed가 끝 표시
  ld_=$(git rev-parse HEAD 2>/dev/null)
  { printf 'landed\t%s\t%s\n' "${ld_:--}" "$(utc)" >"$vdn.tmp.$$" && mv -f "$vdn.tmp.$$" "$vdn"; } 2>/dev/null ||
    echo "hc-lane land: 착지는 됐는데 끝 표시를 못 썼다: $vdn — hc-lane report 착지 를 꼭 부른다" >&2
}
