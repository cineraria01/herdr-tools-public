# hc-lane 함수: clean (bin/hc-lane가 읽는다. 함수는 bin/hc-lane 안 원래 정의 순서 그대로)

# hook <setup|teardown>: 워크트리 안에서 .claude/herdr-<이름>.sh. 기록은 그 워크트리의 git 관리 폴더(worktree remove 때 같이 지워진다).
# --on이면 hc 코어 환경이라 ~/.zshrc의 PATH(bun 등)가 없다(150·151 실측). zsh·~/.zshrc가 있으면 읽고 돈다.
hook() {
  core=; [ -n "$on" ] || [ -n "${HC_TERMINAL_ID-}" ] || core=1   # hc 터미널 밖(launchd 등) 로컬이면 코어 문맥으로(x 위 주석)
  log=$(x git -C "$wt" rev-parse --absolute-git-dir)/hc-lane-$1.log
  zx "$wt" env "LANE_REPO=$repo" "LANE_WORKTREE=$wt" "LANE_BRANCH=$branch" "LANE_BASE=$base" \
    sh -c 'exec bash "$1" >"$2" 2>&1 </dev/null' hc-lane ".claude/herdr-$1.sh" "$log"
  r_=$?; core=; return $r_
}

# clean: 지우기 전에 다 확인하고, 하나라도 걸리면 아무것도 지우지 않는다(teardown도 안 돌린다).
stop() {  # stop <한 줄 이유> [자세한 목록]: stderr에 알리고 JSON, 종료 1
  printf 'hc-lane clean: %s\n' "$1" >&2
  if [ -n "${2-}" ]; then
    printf '%s\n' "$2" | sed -n '1,30s/^/    /p' >&2
    k=$(printf '%s\n' "$2" | wc -l | tr -d ' '); [ "$k" -le 30 ] || echo "    … (모두 ${k}줄)" >&2
  fi
  if [ -n "$did" ]; then printf 'hc-lane clean: 여기까지 했다: %s. 원인을 고친 뒤 같은 명령을 다시 돌린다\n' "$did" >&2
  else printf 'hc-lane clean: 아무것도 지우지 않았다. 원인을 고친 뒤 같은 명령을 다시 돌린다(폴더를 rm -rf로 직접 지우지 않는다)\n' >&2; fi
  cj stopped "$1"
  exit 1
}
# 있음(0)·없음(1)만 믿고, 그 밖의 종료 코드(hc 연결 끊김 등)는 멈춘다. "없음"으로 읽어 거짓 성공하지 않게.
exists() { x sh -c '[ -e "$1" ] || [ -L "$1" ]' hc-lane "$wt"; case $? in 0) return 0 ;; 1) return 1 ;; esac; stop "폴더를 확인하지 못했다(hc·연결?): $wt"; }
has_branch() { x git -C "$repo" show-ref --verify -q "refs/heads/$branch"; case $? in 0) return 0 ;; 1) return 1 ;; esac; stop "브랜치를 확인하지 못했다: $branch"; }
# leftover: 등록이 지워진 폴더에서 메인 폴더의 무시 규칙으로 무시되지 않는 것(추적 파일 포함)을 찍는다. 검사 오류는 종료 2.
# .git은 지워진 등록(<공용 git 폴더>/worktrees/…)을 가리키는 파일일 때만 git이 남긴 것으로 본다.
# ponytail: 규칙은 메인 폴더 작업 트리의 .gitignore — 뒤처져 규칙이 없으면 멈추고(안전 쪽), 로컬로 고쳤으면 아예 멈춘다.
#           자주 걸리면 origin/<기본> 트리의 .gitignore로 판정하게 바꾼다.
leftover() {
  x sh -c '
    cd "$2" || exit 2
    t=$(mktemp "${TMPDIR:-/tmp}/hc-lane-left.XXXXXX") || exit 2
    trap "rm -f \"\$t\" \"\$t.i\"" EXIT
    if [ -f .git ] && [ ! -L .git ]; then
      g=$(sed -n "s/^gitdir: //p" .git)
      case $g in "$3"/worktrees/?*) [ ! -e "$g" ] || echo .git ;; *) echo .git ;; esac
    elif [ -e .git ] || [ -L .git ]; then echo .git
    fi
    find . -mindepth 1 -path ./.git -prune -o ! -type d -print | sed "s|^\./||" >"$t" || exit 2
    git -c core.quotePath=false -C "$1" check-ignore --stdin <"$t" >"$t.i"; [ $? -le 1 ] || exit 2
    grep -vxF -f "$t.i" "$t"; [ $? -le 1 ] || exit 2' hc-lane "$repo" "$wt" "$common"
}
left_check() {  # left_check <어떤 폴더>
  g=$(x git -C "$repo" status --porcelain --untracked-files=all -- ':(glob)**/.gitignore') || stop "메인 폴더 git status 실패: $repo"
  [ -z "$g" ] || stop "메인 폴더의 .gitignore에 커밋 안 된 변경이 있어 $1 폴더의 무시 판정을 믿을 수 없다: $repo" "$g"
  left=$(leftover) || stop "$1 폴더를 검사하지 못했다: $wt"
  [ -z "$left" ] || stop "$1 폴더에 .gitignore로 무시되지 않는 것(추적 파일 포함)이 있다: $wt" "$left"
}

# term_pairs: hc ls --json에서 터미널마다 "<주소> <cwd>" 한 줄. 1 hc ls 실패, 2 형식을 못 읽음(뽑은 수가 "cwd" 수와 다름).
term_pairs() {
  tl=$(hc ls --json 2>/dev/null) || return 1
  tp=$(printf '%s\n' "$tl" | tr '{' '\n' | sed -n 's/.*"address" *: *"\([^"]*\)".*"cwd" *: *"\([^"]*\)".*/\1 \2/p')
  [ "$(printf '%s\n' "$tl" | grep -o '"cwd" *:' | grep -c .)" = "$(printf '%s\n' "$tp" | grep -c .)" ] || return 2
  printf '%s\n' "$tp"
}
# busy_terms: "<주소> <cwd>" 줄(표준 입력) 가운데 대상 기계의 터미널이고 cwd가 그 폴더 안인 것을 찍는다.
busy_terms() {
  while read -r a c; do
    case $a in "${on:+$on/}"*) a=${a#"${on:+$on/}"} ;; *) continue ;; esac
    case $a in */*) continue ;; esac                               # 다른 기계
    [ -n "$on" ] || [ "$a" != "$self" ] || continue   # 부른 터미널 자신(clean 착지 절차의 레인. gc는 빼지 않는다)
    case $c/ in "$wt"/*) echo "${on:+$on/}$a $c" ;; esac
  done
}
clean_cmd() {
  td=none did=
  [ "$branch" != "$base" ] || die "기본 브랜치($base)는 정리하지 않는다"
  here=$(pwd -P 2>/dev/null)
  common=$(x git -C "$repo" rev-parse --path-format=absolute --git-common-dir) || stop "git 관리 폴더를 찾지 못했다: $repo"
  wl=$(x git -C "$repo" worktree list --porcelain) || stop "git worktree list 실패: $repo"
  blk=$(printf '%s\n' "$wl" | W="worktree $wt" awk '$0 == ENVIRON["W"] { f = 1; next } f && /^worktree / { exit } f')
  reg=0; printf '%s\n' "$wl" | grep -Fxq "worktree $wt" && reg=1
  dir=0; exists && dir=1
  br=0; has_branch && br=1
  if [ $reg = 0 ] && [ $dir = 0 ] && [ $br = 0 ]; then
    echo "정리할 것 없음: 워크트리 $wt 와 브랜치 $branch 가 이미 없다"
    clean_lease
    cj nothing
    return 0
  fi

  # 0) 그 폴더가 이 브랜치의 워크트리인지(브랜치 이름의 / 를 - 로 바꾼 폴더 이름은 겹칠 수 있다), 잠기지 않았는지,
  #    살아 있는 다른 터미널이 그 폴더에서 돌지 않는지(착지 절차에서 자기 자신을 지우는 레인은 $HC_TERMINAL_ID로 뺀다)
  if [ $reg = 1 ]; then
    wb=$(printf '%s\n' "$blk" | sed -n 's/^branch //p')
    [ -z "$wb" ] || [ "$wb" = "refs/heads/$branch" ] || stop "이 폴더는 다른 브랜치(${wb#refs/heads/})의 워크트리다: $wt"
    [ -n "$wb" ] || [ $br = 0 ] || stop "이 폴더는 분리 HEAD 워크트리인데 브랜치 $branch 는 따로 있다(다른 레인의 폴더일 수 있다): $wt"
    printf '%s\n' "$blk" | grep -q '^locked' && stop "잠긴 워크트리다(git worktree unlock 뒤 다시): $wt"
  fi
  if [ $dir = 1 ]; then
    pairs=$(term_pairs); case $? in
      1) stop "hc ls 실패: 이 폴더에서 도는 터미널을 확인하지 못했다" ;;
      2) stop "hc ls --json 출력을 읽지 못했다(형식이 바뀌었나?): 이 폴더에서 도는 터미널을 확인하지 못했다" ;;
    esac
    busy=$(printf '%s\n' "$pairs" | busy_terms)
    [ -z "$busy" ] || stop "살아 있는 터미널이 이 폴더에서 돈다(끝났으면 hc kill <주소> 뒤 다시): $wt" "$busy"
  fi

  # 1) 브랜치와 워크트리 HEAD가 origin/<기본>에 들어갔는지(폴더가 없어도 등록의 HEAD로 본다)
  if [ $remote = 0 ]; then :
  elif [ $dry = 1 ]; then echo "[dry-run] git fetch 건너뜀: 지금 있는 origin/$base 로 판정"
  else fetchr x git -C "$repo" fetch -q origin || stop "git fetch 실패: $repo"; fi
  x git -C "$repo" rev-parse -q --verify "$bref^{commit}" >/dev/null || stop "$blab 가 없다: $repo"
  set --
  [ $br = 1 ] && set -- "refs/heads/$branch"
  if [ $reg = 1 ]; then
    h=$(printf '%s\n' "$blk" | sed -n 's/^HEAD //p')
    [ -n "$h" ] || stop "워크트리 HEAD를 읽지 못했다: $wt"
    set -- "$@" "$h"
  fi
  arch= btip= subm= lrr_=-
  [ $br = 0 ] || btip=$(x git -C "$repo" rev-parse -q --verify "refs/heads/$branch^{commit}") || stop "브랜치를 읽지 못했다: $branch"
  for c in "$@"; do
    case $c in refs/heads/*) lab="브랜치 $branch" ;; *) lab="워크트리 HEAD $(printf %.12s "$c")" ;; esac
    x git -C "$repo" merge-base --is-ancestor "$c" "$bref"; ia=$?
    [ $ia != 0 ] || continue
    # 병합기에 제출한 커밋: 원격에 지금(ls-remote — 낡은 추적 참조가 아니라) 그 SHA 그대로인 lane/* 참조가 있으면 원격이 그 커밋을 쥐고
    # 있으니 지워도 된다 — 레인이 접수 뒤 바로 자리를 비운다(3차 중간 9, 지휘). 원격에서 그 참조가 지워졌거나 다른 커밋이면 지우지 않는다
    # (2차 높음 4). 원격을 못 읽으면 멈춘다
    if [ $ia = 1 ] && [ $remote = 1 ] && [ $br = 1 ]; then
      [ "$lrr_" != - ] || lrr_=$(x git -C "$repo" ls-remote origin 'refs/heads/lane/*') || stop "git ls-remote 실패: $lab · origin lane/*"
      s_=$(x git -C "$repo" rev-parse -q --verify "$c^{commit}") || stop "커밋을 읽지 못했다: $lab"
      lh_=$(printf '%s\n' "$lrr_" | awk -v s="$s_" '$1 == s { print $2; exit }')
      if [ -n "$lh_" ]; then echo "${P}제출됨: $lab 은 원격 $lh_ 에 그대로 있다(병합기가 넣는다)"; subm=1; continue; fi
    fi
    if [ $ia = 1 ] && [ $archive = 1 ]; then   # 보관할 커밋(안 들어간 끝 커밋)으로 모은다 — 지우기 전 확인을 다 거친 뒤 push
      [ $remote = 1 ] || stop "원격(origin)이 없어 --archive로 보관할 수 없다: $repo"
      s_=$(x git -C "$repo" rev-parse -q --verify "$c^{commit}") || stop "커밋을 읽지 못했다: $lab"
      case " $arch " in *" $s_ "*) ;; *) arch="$arch $s_" ;; esac
      continue
    fi
    stop "$lab 이(가) $blab 에 들어가지 않았다(착지 전이거나 다른 커밋). 착지 절차의 hc-lane land부터 다시 한다(다른 곳에서 rebase돼 착지했으면 --archive)" \
      "$(x git -C "$repo" log --oneline -n 10 "$bref..$c")"
  done

  # 2) 지울 폴더에 추적 변경·무시되지 않은 untracked가 없는지(status.showUntrackedFiles 설정과 무관하게 모두 본다)
  if [ $reg = 1 ] && [ $dir = 1 ]; then
    st=$(x git -c core.quotePath=false -C "$wt" status --porcelain --untracked-files=all --ignore-submodules=none) || stop "git status 실패: $wt"
    [ -z "$st" ] || stop "워크트리에 커밋 안 된 변경이나 무시되지 않은 untracked가 있다: $wt" "$st"
    ig=$(x git -c core.quotePath=false -C "$wt" status --porcelain --ignored --untracked-files=normal | sed -n 's/^!! //p' | tr '\n' ' ')
    [ -z "$ig" ] || echo "${P}무시된 것도 같이 지운다: $ig"
  elif [ $dir = 1 ]; then
    left_check "등록이 지워진"
  fi

  # 2.5) --archive: 안 들어간 커밋을 원격 refs/archive/<가지>-<sha8>에 보관(sweep-rename 4). 하나라도 실패하면 아무것도 지우지 않는다
  for s_ in $arch; do
    ar=refs/archive/$branch-$(printf %.8s "$s_")
    if [ $dry = 1 ]; then echo "[dry-run] git -C $(q "$repo") push origin $s_:$ar"; continue; fi
    zx "$repo" git push -q origin "$s_:$ar" >&2 || stop "보관 push 실패: $ar ($s_)"
    echo "보관 $ar ($s_) — 되살리기: git fetch origin $ar"
  done

  # 2.7) 무시된 보관물 .qa: 지우기 전 메인 폴더(첫 작업 트리)의 같은 자리로 복사(덮지 않음)하고, 모든 파일이 메인 쪽과 같을 때만
  #      지운다(2026-11-19 — 한 레인이 gc의 「무시된 보관물 .qa」를 보고 손으로 지우다 무료 실행 출력을 잃었다). 같은 이름의
  #      다른 파일이 있거나 복사 실패·메인 폴더 못 찾음이면 아무것도 안 지우고 멈춘다. --on이면 그 기계 안 복사
  if [ $dir = 1 ]; then
    mw=$(printf '%s\n' "$wl" | awk 'NR == 1 { p = substr($0, 10) } /^bare/ { p = "" } /^$/ { exit } END { print p }')
    [ "$mw" != "$wt" ] || mw=
    gd=$repo; [ $reg = 0 ] || gd=$wt
    qa=$(x sh -c 'cd "$1" || exit 2
      r=$(find . -path ./.git -prune -o -type d -name .qa -print -prune) || exit 2
      [ -n "$r" ] || exit 0
      f=$(printf "%s\n" "$r" | while IFS= read -r d; do find "$d" ! -type d; done | sed "s|^\./||") || exit 2
      [ -n "$f" ] || exit 0
      i=$(printf "%s\n" "$f" | git -c core.quotePath=false -C "$3" check-ignore --stdin); [ $? -le 1 ] || exit 2
      [ -n "$i" ] || exit 0
      [ -n "$2" ] && [ -d "$2" ] || exit 3
      [ "$4" = 0 ] || { printf "%s\n" "$i" | sed "s/^/n /"; exit 0; }
      printf "%s\n" "$i" | while IFS= read -r p; do
        if [ -e "$2/$p" ] || [ -L "$2/$p" ]; then k=s; cmp -s "$p" "$2/$p" || k=d
        else k=c; { mkdir -p "$2/$(dirname "$p")" && cp -pP "$p" "$2/$p" && cmp -s "$p" "$2/$p"; } || k=f; fi
        echo "$k $p"
      done' hc-lane "$wt" "$mw" "$gd" "$dry")
    case $? in 0) ;; 3) stop "메인 폴더를 찾지 못해 무시된 .qa를 옮길 수 없다(손으로 옮긴 뒤 다시): $wt" ;;
      *) stop "무시된 .qa를 메인 폴더로 옮기지 못했다: $wt → ${mw:-?}" ;; esac
    if [ -n "$qa" ]; then
      qp=$(printf '%s\n' "$qa" | sed 's/^. //; s|^\(.*\.qa\)/.*|\1|' | sort -u |
        awk -v w="$wt" -v m="$mw" '{ printf "%s%s/%s → %s/%s", (NR > 1 ? ", " : ""), w, $0, m, $0 }')
      nf=$(printf '%s\n' "$qa" | grep -c .)
      if [ $dry = 1 ]; then echo "[dry-run] 무시된 보관물 복사(덮지 않음) 뒤 지움: $qp (파일 ${nf}개)"
      else
        dq=$(printf '%s\n' "$qa" | sed -n 's/^d /다름(메인 것 둠) /p; s/^f /복사 실패 /p')
        [ -z "$dq" ] || stop "무시된 .qa 파일 일부를 메인 폴더로 옮기지 못했다(같은 이름의 다른 파일은 덮지 않는다) — 비교·정리한 뒤 다시. 아무것도 안 지웠다: $wt" "$dq"
        echo "옮김 $qp (파일 ${nf}개 — 새로 $(printf '%s\n' "$qa" | grep -c '^c '), 이미 같음 $(printf '%s\n' "$qa" | grep -c '^s '))"
        did="${did:+$did, }.qa 메인 폴더로 복사"
      fi
    fi
  fi

  # 3) teardown(있으면, 실패해도 계속 — herdr-teardown.sh의 약속) → worktree remove → 남은 무시 파일 폴더 → branch -d
  if [ $reg = 1 ] && [ $dir = 1 ] && x test -f "$wt/.claude/herdr-teardown.sh"; then
    if [ $dry = 1 ]; then echo "[dry-run] 워크트리에서 bash .claude/herdr-teardown.sh"; td=dry-run
    elif hook teardown; then td=ran did=teardown
    else
      x tail -n 20 "$log" >&2
      echo "hc-lane clean: herdr-teardown.sh 실패(위는 기록 끝 20줄), 정리는 계속한다" >&2
      td=failed did=teardown
    fi
  fi
  if [ $reg = 1 ]; then
    if [ $dry = 1 ]; then echo "[dry-run] git -C $(q "$repo") worktree remove $(q "$wt")"
    else
      # 무시된 파일 때문에 "Directory not empty"로 끝나도 등록은 지워질 수 있다(git이 멈추지 않는다). 등록으로 판정한다.
      x git -C "$repo" worktree remove "$wt" >&2
      wl=$(x git -C "$repo" worktree list --porcelain) || stop "git worktree list 실패: $repo"
      printf '%s\n' "$wl" | grep -Fxq "worktree $wt" && stop "git worktree remove 실패: $wt"
      did="${did:+$did, }워크트리 등록 제거"
    fi
  fi
  if [ $dry = 1 ]; then
    [ $reg = 1 ] || [ $dir = 0 ] || echo "[dry-run] rm -rf -- $(q "$wt")  # 무시된 파일만 남은 폴더"
  elif exists; then
    left_check 남은
    x rm -rf -- "$wt" || stop "남은 폴더를 지우지 못했다: $wt"
    did="${did:+$did, }남은 무시 파일 폴더 삭제"
  fi
  if [ $br = 1 ]; then
    if [ $dry = 1 ]; then echo "[dry-run] git -C $(q "$repo") branch -d $branch"
    else
      if [ -n "$arch$subm" ]; then   # 보관·제출했으니 병합 여부 대신 확인한 값 그대로일 때만 지운다
        x git -C "$repo" update-ref -d "refs/heads/$branch" "$btip" >&2 || stop "브랜치를 지우지 못했다(보관 뒤 값이 바뀌었나?): $branch"
      else x git -C "$repo" branch -q -d "$branch" >&2 || stop "git branch -d 실패: $branch"; fi
      did="${did:+$did, }브랜치 삭제"
    fi
  fi

  case $td in ran) tdk=통과 ;; none) tdk='없음(건너뜀)' ;; failed) tdk='실패(정리는 계속)' ;; *) tdk='돌릴 예정' ;; esac
  if [ $dry = 1 ]; then echo "[dry-run] 정리할 것: 워크트리 $wt, 브랜치 $branch (teardown: $tdk)"
  else echo "정리 끝: 워크트리 $wt, 브랜치 $branch${on:+ @$on} (teardown: $tdk)"; fi
  if [ -z "$on" ] && [ $dry = 0 ]; then
    case "$PWD/|$here/" in "$wt"/*|*"|$wt"/*) echo "알림: 지금 셸 위치는 지운 폴더 안이었다. 다음 명령 전에 cd $(q "$repo")" ;; esac
  fi
  clean_lease
  if [ $dry = 1 ]; then cj planned; else cj done; fi
}
# clean_lease: 정리가 끝난 레인의 임대 refs/hc-lanes/<레인>을 놓는다(지휘 결정 — 풀기는 land 성공·clean). 원격 없음·이미 없음은 조용히,
# 못 놓으면 알리기만(정리는 됨 — 손으로 hc-lane lease drop)
clean_lease() {
  cl_=${name:-$branch}
  case $cl_ in '' | *[!abcdefghijklmnopqrstuvwxyz0123456789_-]*) return 0 ;; esac
  [ $remote = 1 ] || return 0
  if [ $dry = 1 ]; then echo "[dry-run] 임대 refs/hc-lanes/$cl_ 가 있으면 놓음"; return 0; fi
  clo_=$(lease_x drop "$cl_") || { echo "임대 refs/hc-lanes/$cl_ 를 놓지 못했다(정리는 됨) — hc-lane lease drop${on:+ --on $on} --repo $(q "$repo") --name $cl_" >&2; return 0; }
  [ "$clo_" = no-origin ] || echo "임대 확인: refs/hc-lanes/$cl_ 없음(놓음)"
}
