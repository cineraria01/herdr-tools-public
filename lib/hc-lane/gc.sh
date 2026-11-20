# hc-lane 함수: gc (bin/hc-lane가 읽는다. 함수는 bin/hc-lane 안 원래 정의 순서 그대로)

# gc: 끝난 레인의 작업 폴더·가지와 원격 중간 참조를 정리한다. 하나라도 맞지 않으면 「남김 <대상> <까닭>」, 지우지 않는다.
gin() {  # gin <커밋>: $bref에 다 들어갔나. 0 들어감(조상, 또는 병합 커밋 없이 커밋마다 공백까지 같은 패치가 있음), 1 아님(gn=안 들어간 커밋 수), 2 확인 못 함
  gn=0; x git -C "$repo" merge-base --is-ancestor "$1" "$bref"; case $? in 0) return 0 ;; 1) ;; *) return 2 ;; esac
  gm=$(x git -C "$repo" rev-list --merges "$bref..$1") || return 2
  # 패치 비교는 병합 커밋을 보지 않는다 — 병합이 끼면 같은 내용인지 모른다고 보고, 조상 아닌 커밋을 다 센다
  if [ -n "$gm" ]; then gn=$(x git -C "$repo" rev-list --count "$bref..$1") || gn='?'; return 1; fi
  # git cherry처럼 patch-id로 보되 --verbatim(공백까지): 공백만 다른 변경(들여쓰기 등)은 같은 내용이 아니다(2026-11-16 감사).
  # U <id> = $bref 쪽 커밋, B <id> <커밋> = 가지 쪽, C <커밋> = 가지 커밋 목록(빈 커밋은 id가 없어 안 들어감으로 센다)
  gc_=$(x sh -c 'cd "$1" || exit 2; c=$(git rev-list --no-merges "$2..$3") || exit 2
    git log -p --no-merges --no-color --no-ext-diff "$3..$2" | git patch-id --verbatim | sed "s/^/U /"
    git log -p --no-merges --no-color --no-ext-diff "$2..$3" | git patch-id --verbatim | sed "s/^/B /"
    printf "%s\n" "$c" | sed "/^$/d; s/^/C /"' hc-lane "$repo" "$bref" "$1") || return 2
  gn=$(printf '%s\n' "$gc_" | awk '$1 == "U" { u[$2] = 1 } $1 == "B" { b[$3] = $2 } $1 == "C" { c[++n] = $2 }
    END { for (i = 1; i <= n; i++) if (!(c[i] in b) || !(b[c[i]] in u)) m++; print m + 0 }')
  [ "$gn" = 0 ]
}
# jdg <대상> <커밋> <나이 기준 시각>: 들어갔고 --min-age보다 오래됐으면 0. 아니면 「남김」을 찍고 1
jdg() {
  gin "$2"; case $? in
    0) ;;
    1) keep "$1" "안 들어감 ${gn}개 · 마지막 $(x git -C "$repo" show -s --format=%cs "$2")"; return 1 ;;
    *) keep "$1" "들어갔는지 확인하지 못함"; err=1; return 1 ;;
  esac
  [ $((now - $3)) -ge "$secs" ] || { keep "$1" "새것($(((now - $3) / 3600))시간)"; return 1; }
}
kept() {  # kept <가지>: 남길 목록(설정 gc.keep_branches, 빈칸으로 나눈 glob 무늬)에 맞나
  set -f; for p_ in $kb; do case $1 in $p_) set +f; return 0 ;; esac; done; set +f; return 1
}
# rgc <접두>: 원격 <접두>* 가운데 들어갔고 커밋이 오래된 것을 ls-remote 때 값 그대로일 때만 지운다(refs/heads면 기본·남길 목록 빼고)
rgc() {
  ls=$(x git -C "$repo" ls-remote origin "${1}*") || die "gc: git ls-remote 실패: $repo"
  # ls-remote 무늬는 참조 이름 끝에도 맞으니(refs/heads/refs/integ/x) 접두로 다시 거른다
  refs=$(printf '%s\n' "$ls" | awk -F'\t' -v p="$1" 'index($2, p) == 1 && $2 !~ /\^\{\}$/')
  set --
  while IFS=$tab read -r s r; do
    [ -n "$r" ] || continue
    case $r in refs/heads/*) [ "${r#refs/heads/}" != "$base" ] && ! kept "${r#refs/heads/}" || continue ;; esac
    x git -C "$repo" cat-file -e "$s^{commit}" 2>/dev/null || set -- "$@" "$r"
  done <<EOF
$refs
EOF
  # 판정에 쓸 커밋만 받는다. --refmap=: refs/heads는 기본 refspec에 들어 이름만 줘도 origin/*를 고치므로 참조는 하나도 안 쓴다(dry-run도)
  [ $# = 0 ] || fetchr x git -C "$repo" fetch -q --no-write-fetch-head --refmap= origin "$@" || die "gc: 원격 참조 커밋을 받지 못했다"
  while IFS=$tab read -r s r; do
    [ -n "$r" ] || continue
    case $r in refs/heads/*)
      [ "${r#refs/heads/}" != "$base" ] || continue
      ! kept "${r#refs/heads/}" || { keep "$r" "남길 목록(gc.keep_branches)"; continue; } ;;
    esac
    ct=$(x git -C "$repo" show -s --format=%ct "$s") || { keep "$r" "커밋 시각을 읽지 못함"; err=1; continue; }
    jdg "$r" "$s" "$ct" || continue
    [ $dry = 0 ] || { gone_ "$r" "$s"; continue; }
    # 값이 ls-remote 때 그대로일 때만 지운다(그사이 다시 올린 참조는 남긴다). 되돌리기: git push origin <sha>:<참조>
    x git -C "$repo" push -q --force-with-lease="$r:$s" origin ":$r" >&2 || { keep "$r" "push 지우기 실패"; err=1; continue; }
    gone_ "$r" "$s"
  done <<EOF
$refs
EOF
}
keep() { echo "${P}남김 $1 $2"; kn=$((kn + 1)); }
gone_() { echo "${P}지움 $*"; dn=$((dn + 1)); }
gc_cmd() {
  dn=0 kn=0 err=0 self= tab=$(printf '\t') now=$(date +%s)
  kb=$(cfg gc.keep_branches "") || exit 1
  pairs=$(term_pairs); case $? in
    1) die "gc: hc ls 실패 — 작업 폴더를 쓰는 터미널을 확인하지 못해 아무것도 지우지 않았다" ;;
    2) die "gc: hc ls --json 출력을 읽지 못했다(형식이 바뀌었나?) — 아무것도 지우지 않았다" ;;
  esac
  if [ $remote = 0 ]; then :
  elif [ $dry = 1 ]; then echo "[dry-run] git fetch 건너뜀: 지금 있는 origin/$base 로 판정"
  else fetchr x git -C "$repo" fetch -q origin || die "gc: git fetch 실패: $repo"; fi
  x git -C "$repo" rev-parse -q --verify "$bref^{commit}" >/dev/null || die "gc: $blab 가 없다: $repo"
  cur=$(x git -C "$repo" symbolic-ref -q --short HEAD)
  wl=$(x git -C "$repo" worktree list --porcelain) || die "gc: git worktree list 실패: $repo"
  recs=$(printf '%s\n' "$wl" | awk -v OFS='\t' '
    /^worktree / { if (p != "") print p, h, b, l; p = substr($0, 10); h = "-"; b = "-"; l = "-"; next }
    /^HEAD / { h = substr($0, 6) } /^branch / { b = substr($0, 8) } /^locked/ || /^bare/ { l = "x" }
    END { if (p != "") print p, h, b, l }')
  while IFS=$tab read -r wt h b l; do
    [ -n "$wt" ] && [ "$wt" != "$repo" ] || continue
    branch=; case $b in refs/heads/*) branch=${b#refs/heads/} ;; esac
    [ "$l" = - ] || { keep "$wt" "잠긴 작업 폴더"; continue; }
    if [ -n "$branch" ] && { [ "$branch" = "$base" ] || [ "$branch" = "$cur" ]; }; then keep "$wt" "기본·현재 가지 $branch"; continue; fi
    gin "$h"; case $? in
      0) ;;
      1) keep "$wt" "$blab 에 안 들어감(${branch:-분리 HEAD} $(printf %.12s "$h"))"; continue ;;
      *) keep "$wt" "들어갔는지 확인하지 못함"; err=1; continue ;;
    esac
    x sh -c '[ -e "$1" ] || [ -L "$1" ]' hc-lane "$wt"; case $? in 0) dir=1 ;; 1) dir=0 ;; *) keep "$wt" "폴더를 확인하지 못함"; err=1; continue ;; esac
    if [ $dir = 1 ]; then
      busy=$(printf '%s\n' "$pairs" | busy_terms | cut -d' ' -f1 | tr '\n' ' ')
      [ -z "$busy" ] || { keep "$wt" "살아 있는 터미널 ${busy% }"; continue; }
      adm=$(x git -C "$wt" rev-parse --absolute-git-dir) || { keep "$wt" "git 관리 폴더를 찾지 못함"; err=1; continue; }
      # 나이는 status보다 먼저 본다(status가 index를 다시 쓰면 묵은 폴더가 새것으로 보인다). 0 오래됨, 1 새것, 그 밖 확인 못 함
      [ "$mins" = 0 ] || x sh -c 'm=$1; shift; for f; do [ -e "$f" ] || continue; o=$(find "$f" -prune -mmin -"$m") || exit 2
        [ -z "$o" ] || exit 1; done' hc-lane "$mins" "$adm/HEAD" "$adm/index" "$adm/logs/HEAD" "$wt"
      case $? in 0) ;; 1) keep "$wt" "새것(마지막 git 활동이 --min-age ${minage:-24h} 안)"; continue ;;
        *) keep "$wt" "나이를 확인하지 못함"; err=1; continue ;; esac
      # 멈춘 rebase·merge·cherry-pick·revert·bisect는 작업 폴더에만 있는 상태라 남긴다
      ip=$(x sh -c 'for s in rebase-merge rebase-apply MERGE_HEAD CHERRY_PICK_HEAD REVERT_HEAD BISECT_LOG; do
        [ ! -e "$1/$s" ] || echo "$s"; done' hc-lane "$adm") || { keep "$wt" "진행 상태를 확인하지 못함"; err=1; continue; }
      [ -z "$ip" ] || { keep "$wt" "진행 중 $(echo $ip)"; continue; }
      # --no-optional-locks: 판정만 하므로 index를 다시 쓰지 않는다(dry-run도 아무것도 안 바꾸게)
      st=$(x git --no-optional-locks -c core.quotePath=false -C "$wt" status --porcelain --untracked-files=all --ignore-submodules=none) ||
        { keep "$wt" "git status 실패"; err=1; continue; }
      [ -z "$st" ] || { keep "$wt" "커밋 안 된 변경·untracked $(printf '%s\n' "$st" | grep -c .)개"; continue; }
      ig=$(x git --no-optional-locks -c core.quotePath=false -C "$wt" status --porcelain --ignored --untracked-files=normal) ||
        { keep "$wt" "git status --ignored 실패"; err=1; continue; }
      # ponytail: 보관물은 .qa/ 하나(QA 증거 폴더 관례). 다른 저장소에 보관 폴더가 생기면 여기에 더한다
      qa=$(printf '%s\n' "$ig" | sed -n 's/^!! //p' | grep -E '(^|/)\.qa(/|$)' | tr '\n' ' ')
      [ -z "$qa" ] || { keep "$wt" "무시된 보관물 ${qa% } (hc-lane clean이 메인 폴더로 옮기고 정리 — 손으로 지우지 말 것)"; continue; }
    fi
    what="${branch:+가지 $branch }$(printf %.12s "$h")"
    [ $dry = 0 ] || { gone_ "$wt" "$what"; continue; }
    if [ $dir = 1 ] && x test -f "$wt/.claude/herdr-teardown.sh"; then
      hook teardown || echo "hc-lane gc: herdr-teardown.sh 실패(정리는 계속): $wt" >&2
    fi
    # 셋(들어감·깨끗함·쓰는 터미널 없음)을 다 본 뒤라 강제로 지운다: 무시 파일은 같이, rebase로 들어간 가지는 -d가 거절하므로
    x git -C "$repo" worktree remove --force "$wt" >&2 || { keep "$wt" "git worktree remove 실패"; err=1; continue; }
    if [ -n "$branch" ]; then
      if [ "$(x git -C "$repo" rev-parse -q --verify "refs/heads/$branch")" = "$h" ]; then
        x git -C "$repo" branch -q -D "$branch" >&2 || { echo "hc-lane gc: 가지 $branch 를 지우지 못했다" >&2; err=1; }
      else echo "hc-lane gc: 가지 $branch 가 그사이 움직여 남겼다" >&2; fi
    fi
    gone_ "$wt" "$what"
  done <<EOF
$recs
EOF

  # 작업 폴더 없이 남은 로컬 가지: 기본·현재 HEAD·작업 폴더에 붙은 가지는 빼고 같은 판정. 나이는 커밋과 가지 reflog 가운데 늦은 쪽
  # 멈춘 rebase의 작업 폴더는 porcelain에 분리 HEAD로 보이니 rebase-*/head-name의 가지도 붙은 가지로 센다
  att=$({ printf '%s\n' "$recs" | cut -f3; x sh -c 'c=$(git -C "$1" rev-parse --path-format=absolute --git-common-dir) || exit 0
    cat "$c"/rebase-*/head-name "$c"/worktrees/*/rebase-*/head-name 2>/dev/null; exit 0' hc-lane "$repo"; })
  bl=$(x git -C "$repo" for-each-ref --format='%(refname)%09%(objectname)%09%(committerdate:unix)' refs/heads) ||
    die "gc: git for-each-ref 실패: $repo"
  while IFS=$tab read -r r s ct; do
    [ -n "$r" ] || continue
    b=${r#refs/heads/}
    [ "$b" != "$base" ] && [ "$b" != "$cur" ] || continue
    ! printf '%s\n' "$att" | grep -Fxq -- "$r" || continue
    ! kept "$b" || { keep "가지 $b" "남길 목록(gc.keep_branches)"; continue; }
    rl=$(x git -C "$repo" reflog -1 --date=unix --format=%gd "$r" 2>/dev/null); rl=${rl##*@\{}; rl=${rl%\}}
    case $rl in ''|*[!0-9]*) ;; *) [ "$rl" -le "$ct" ] || ct=$rl ;; esac
    jdg "가지 $b" "$s" "$ct" || continue
    [ $dry = 0 ] || { gone_ "가지 $b" "$s"; continue; }
    # 판정한 값 그대로일 때만 지운다. 되돌리기: git branch <가지> <sha>
    [ "$(x git -C "$repo" rev-parse -q --verify "$r")" = "$s" ] || { keep "가지 $b" "그사이 움직임"; continue; }
    x git -C "$repo" branch -q -D -- "$b" >&2 || { keep "가지 $b" "git branch -D 실패"; err=1; continue; }
    gone_ "가지 $b" "$s"
  done <<EOF
$bl
EOF

  if { [ -n "$integ" ] || [ $rb = 1 ]; } && [ $remote = 0 ]; then echo "원격이 없어 --integ·--remote-branches 는 할 것 없음"
  else
    [ -z "$integ" ] || rgc "$integ"
    [ $rb = 0 ] || rgc refs/heads/
  fi
  echo "${P}지움 $dn · 남김 $kn"
  return $err
}
