# hc-lane 함수: verdict (bin/hc-lane가 읽는다. 함수는 bin/hc-lane 안 원래 정의 순서 그대로)

# ---- 승인 착지 land --verdict <aid>(fill-handoff-integrity) ----
# 시작 표시와 기존 제출 기록은 승인 원본의 patch-id를 쓴다. 후보의 내용은 lverdict_check에서 Git 트리로 대조한다.
lpatch() {
  d_=$(git --literal-pathspecs diff -U0 --binary "$1" "$2" --) || return 1
  [ -n "$d_" ] || { echo -; return 0; }
  p_=$(printf '%s\n' "$d_" | git patch-id --stable) || return 1
  p_=${p_%% *}; [ -n "$p_" ] || return 1; echo "$p_"
}
lverdict_start() {
  [ -n "${HC_LANE-}" ] || die "land --verdict: 레인 터미널 안에서 부른다(\$HC_LANE 없음)"
  vload "$HC_LANE" "$lvd"
  case $? in 1) die "land --verdict: 판정 $lvd 가 없다($vf)" ;; 2) die "land --verdict: 판정 파일 꼴이 틀렸다: $vf" ;; esac
  [ "$vv" = pass ] || { echo "hc-lane land: 판정 $lvd 는 통과가 아니다($vv) — 착지하지 않았다" >&2; exit 2; }
  [ "${vb:-main}" = "$base" ] || { echo "hc-lane land: 판정 $lvd 의 기본 브랜치는 ${vb:-main} 이다" >&2; exit 2; }
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

# 바뀐 경로의 예외는 이 승인 원본에서 rerere를 실제로 거친 결과(모드와 blob)에만 준다.
lverdict_before_rebase() {
  local h bref
  h=$(git rev-parse --verify HEAD) || die "land: rebase 전 HEAD 조회 실패"
  [ "$h" != "$lorig" ] || return 0
  bref=$(git merge-base HEAD "$1") || die "land: rebase 전 갈래점 조회 실패"
  lverdict_check
}
lverdict_entry() { git --literal-pathspecs ls-tree --format='%(objectmode) %(objectname)' "$2" -- "$1"; }
lverdict_resolved() {
  local path entry input
  while IFS= read -r path; do
    entry=$(lverdict_entry "$path" HEAD) && input=$(lverdict_entry "$path" "$2") &&
      printf '%s\t%s\t%s\t%s\t%s\n' "$lorig" "$lpid" "${entry:--}" "$path" "${input:--}" >>"$lrrf" ||
      die "land: rerere 결과를 기록하지 못했다: $path"
  done <<<"$1"
}
lverdict_check() {
  local merged rc tree conflicts= changed paths base_commit input
  base_commit=$(git rev-parse --verify "$bref^{commit}") || die "land: 승인 대조 기준 조회 실패"
  merged=$(git -c core.quotePath=false merge-tree --write-tree --name-only --no-messages --merge-base="$lmb" "$base_commit" "$lorig"); rc=$?
  case $rc in
    0) ;;
    1) [[ $merged == *$'\n'* ]] || die "land: 승인 원본의 충돌 경로를 읽지 못했다"
       conflicts=${merged#*$'\n'} ;;
    *) die "land: 승인 원본을 현재 기준에 병합한 트리를 구하지 못했다" ;;
  esac
  tree=${merged%%$'\n'*}
  changed=$(git -c core.quotePath=false diff-tree -r --no-renames --ignore-submodules=none --name-only "$tree" HEAD --) ||
    die "land: 승인 원본의 병합 트리와 후보의 경로를 대조하지 못했다"
  [ -n "$changed$conflicts" ] || return 0
  paths=$(printf '%s\n' "$changed" "$conflicts" | LC_ALL=C sort -u) || die "land: 승인 대조 경로 정리 실패"
  lbad_= LRRN=0
  while IFS= read -r lp_; do
    [ -n "$lp_" ] || continue
    entry_=$(lverdict_entry "$lp_" HEAD) && input=$(lverdict_entry "$lp_" "$base_commit") || die "land: rerere 경로 조회 실패: $lp_"
    if LC_ALL=C awk -F'\t' -v s="$lorig" -v p="$lpid" -v e="${entry_:--}" -v f="$lp_" -v b="${input:--}" '
      $1 == s && $2 == p && $4 == f && $5 == b { x = ($3 == e) } END { exit !x }' "$lrrf" 2>/dev/null; then LRRN=$((LRRN + 1))
    else lbad_="$lbad_${lbad_:+, }$lp_"; fi
  done <<<"$paths"
  if [ -z "$lbad_" ]; then echo "hc-lane land: rerere로 바뀐 경로 ${LRRN}개 — 승인 변경과 다른 곳이 rerere가 푼 경로뿐이라 이어 간다"; return 0; fi
  echo "hc-lane land: rebase 뒤 변경이 승인 내용과 다르다(rerere가 푼 곳 밖 — $lbad_). push하지 않았다. 지금 커밋으로 hc-lane report 완료 $(git rev-parse --short=8 HEAD) 를 부르고 기다린다(새 감사)" >&2
  exit 2
}
lverdict_precheck() {  # 위임 경로: 위임 전에 같은 기준으로 rebase(rerere 이어 가기 포함)해 차이가 rebase뿐인지(충돌 종료 3, 다르면 종료 2)
  tclean "승인 rebase 전" || die "land: 커밋 안 된 변경이나 멈춘 rebase가 있다(git status). 커밋하거나 rebase를 마친 뒤 같은 줄을 다시"
  [ $remote = 0 ] || fetchr git fetch -q origin || die "land: git fetch 실패 — 착지하지 않았다"
  rb_ "$bref" || { echo "hc-lane land: 승인 착지 중 rebase 충돌 — 양쪽 내용을 살려 풀고 git add → git rebase --continue 뒤 커밋으로 hc-lane report 완료 <sha>(새 감사)" >&2; exit 3; }
  lverdict_check
  [ -z "$lvd" ] || lverdict_save HEAD hook
}
lverdict_save() {
  local sha
  sha=$(git rev-parse --verify "$1^{commit}") && printf '%s %s\n' "$sha" "$2" >"$vdn.candidate.tmp" &&
    mv -f "$vdn.candidate.tmp" "$vdn.candidate" || die "land: 판정 $lvd 의 검증 후보를 기록하지 못했다"
}
lverdict_recover() {
  local candidate= kind= marker= refs sha ref=refs/hc-verdict/$HC_LANE/$lvd
  if [ -f "$vdn.candidate" ]; then
    read -r candidate kind <"$vdn.candidate" || die "land: 판정 $lvd 의 후보 기록 읽기 실패"
    if [ "$kind" = hook ] && [ "$remote" = 1 ]; then
      refs=$(git ls-remote origin "$ref") || die "land: 전용 훅의 착지 표식 조회 실패"
      marker=$(printf '%s\n' "$refs" | awk -v r="$ref" '$2==r {print $1}')
      [ -z "$marker" ] || fetchr git fetch -q origin || die "land: 전용 훅 착지 뒤 git fetch 실패"
    fi
  fi
  for sha in "$marker" "$candidate" "$vs"; do
    [ -n "$sha" ] || continue
    git merge-base --is-ancestor "$sha" "$bref"; case $? in
      0) echo "hc-lane land: 판정 $lvd 의 커밋 ${sha:0:8} 이 이미 $blab 에 있다"
         [ "$remote" = 0 ] || land_known "$sha" || exit $?
         lverdict_done "$sha"; [ "$remote" = 0 ] || ffmain "$(mainwt)" "$base"; exit 0 ;;
      1) ;;
      *) die "land: 판정 $lvd 의 착지 여부 조회 실패" ;;
    esac
  done
}
lverdict_done() {  # 착지 끝 표시(hc-fill이 본다). 못 쓰면 알림만 — 착지는 됐고 착지 사건도 남는다
  [ -n "$lvd" ] || return 0   # 병합기 착지(판정 파일 없음)는 착지 사건 키 hf:<aid>:landed가 끝 표시
  ld_=${1:-$(git rev-parse --verify HEAD)}; [ -n "$ld_" ] || die "land: 끝 표시에 쓸 커밋 조회 실패"
  { printf 'landed\t%s\t%s\n' "$ld_" "$(utc)" >"$vdn.tmp.$$" && mv -f "$vdn.tmp.$$" "$vdn"; } 2>/dev/null ||
    echo "hc-lane land: 착지는 됐는데 끝 표시를 못 썼다: $vdn — hc-lane report 착지 를 꼭 부른다" >&2
}
