# hc-lane 함수: merger (bin/hc-lane가 읽는다. 함수는 bin/hc-lane 안 원래 정의 순서 그대로)

# 제출: HEAD를 refs/heads/lane/<레인>과 큐 표식 refs/merge-queue/<UTC초>-<레인>에 한 번의 원자 push로(같은 레인의 옛 표식은 같은
# push에서 지움 — 다시 제출하면 새 것만 남는다). 기본 브랜치는 건드리지 않는다. 결과를 모르면(전송 오류) 원격을 다시 읽어 둘 다
# 그 SHA면 제출로, 그 밖·읽기 실패는 멈춤(같은 줄을 다시 돌리면 같은 결과로 모인다). 종료 0 제출(또는 이미 들어감), 1 실패
submit_cmd() {
  llog_init
  ln_=${HC_LANE:-$(git symbolic-ref -q --short HEAD 2>/dev/null)}
  lname_ok "$ln_" || die "land(제출): 레인 이름은 a-z 0-9 - _ 32자까지여야 큐에 넣는다: ${ln_:-(분리된 HEAD)}"
  # 병합기 클론의 로컬 가지와 겹치는 이름(기본 브랜치·묶음 가지)은 받지 않는다(감사 낮음 3)
  case $ln_ in "$base"|merge-batch|batch|main|master) die "land(제출): 레인 이름 $ln_ 은 병합기가 쓰는 가지 이름이라 큐에 넣지 않는다 — 레인 작업 트리에서 HC_LANE=<레인>으로" ;; esac
  git remote get-url origin >/dev/null 2>&1 || die "land(제출): origin이 없는 저장소는 병합기로 넣지 않는다"
  for f_ in rebase-merge rebase-apply MERGE_HEAD CHERRY_PICK_HEAD; do
    # 경로 조회가 실패하면 빈 경로가 「없음」으로 통과했다(3차 높음 5) — 모르면 제출하지 않는다
    gp_=$(git rev-parse --git-path "$f_") && [ -n "$gp_" ] || die "land(제출): git rev-parse --git-path 실패 — 진행 중인 rebase·merge가 있는지 모른다"
    [ ! -e "$gp_" ] || die "land(제출): 진행 중인 rebase·merge가 있다($f_). 마친 뒤 같은 줄을 다시"
  done
  # 무시되지 않은 새 파일도 본다(아스트라 1차 중간 8 — 빠진 새 파일로 제출이 되고 뒤 clean이 그 파일로 멈췄다)
  st_=$(git status --porcelain --untracked-files=normal) || die "land(제출): git status 실패 — 작업 트리를 모른다"
  [ -z "$st_" ] || die "land(제출): 커밋 안 된 변경·새 파일이 있다(git status). 커밋하거나 치운 뒤 같은 줄을 다시"
  # 선행(HERDR_LAND_AFTER)은 최소판에서 뺐다(2026-11-20 지휘 — 이름만으로는 그 실행의 선행을 못 가린다, 2판에서 작업 ID로). 선행이 아직이면
  # 제출하지 말고 기다린다(hc-fill --after가 띄우기를 막는다)
  [ -z "${HERDR_LAND_AFTER-}" ] || die "land(제출): 선행(HERDR_LAND_AFTER)은 병합기 최소판에 없다 — 선행이 main에 들어간 뒤 HERDR_LAND_AFTER 없이 같은 줄을"
  sh_=$(git rev-parse -q --verify HEAD) || die "land(제출): HEAD를 읽지 못했다"
  lrun fetchr git fetch -q origin || die "land(제출): git fetch 실패"
  ob_=$(git rev-parse -q --verify "refs/remotes/origin/$base^{commit}") || die "land(제출): origin/$base 가 없다"
  git merge-base --is-ancestor "$sh_" "$ob_"; ia_=$?
  case $ia_ in
    0) echo "hc-lane land: 이 레인의 커밋은 이미 origin/$base 에 있다 — 제출할 것 없음"; exit 0 ;;
    1) ;;
    *) die "land(제출): git merge-base --is-ancestor 실패(종료 $ia_)" ;;
  esac
  rs_=$(git ls-remote origin "refs/heads/lane/$ln_" 'refs/merge-queue/*' 'refs/merge-queue-meta/*' 'refs/merge-failed/*' "refs/merge-done/$ln_") ||
    die "land(제출): 원격을 읽지 못했다(git ls-remote) — 같은 줄을 다시"
  # 병합기가 이 레인의 앞 제출을 넣는 중이어도 다시 제출할 수 있다: 병합기의 main push가 그 큐 표식을 lease로 함께 지우니, 여기서 옛
  # 표식을 지우면 그 push는 통째로 거절된다(구조 B)
  cur_=$(printf '%s\n' "$rs_" | awk -v r="refs/heads/lane/$ln_" '$2 == r { print $1 }')
  # 같은 커밋을 이미 냈으면 새 제출을 만들지 않는다(3차 중간 1 — 접수 확인이 끊겨 같은 줄을 다시 돌리면, 이미 rebase돼 들어간 원본은
  # 조상 검사에서 빠져 새 제출이 됐다): 큐에 그 SHA로 남아 있으면 접수 그대로, refs/merge-done/<레인>(넣은 제출 기록)의 원본이 이
  # 커밋이면 이미 넣음
  if [ "$cur_" = "$sh_" ] && printf '%s\n' "$rs_" | awk -v l="$ln_" -v s="$sh_" '{ n = $2 } sub(/^refs\/merge-queue\/[0-9]+-/, "", n) && n == l && $1 == s { f = 1 } END { exit !f }'; then
    echo "hc-lane land: 이 커밋($(git rev-parse --short "$sh_"))은 이미 병합기 큐에 있다(lane/$ln_) — 새로 내지 않음. 결과는 병합기가 사건으로 알린다"; exit 0
  fi
  dn_=$(printf '%s\n' "$rs_" | awk -v r="refs/merge-done/$ln_" '$2 == r { print $1 }')
  if [ -n "$dn_" ]; then
    fetchr git fetch -q origin "+refs/merge-done/$ln_:refs/hc-lane/merge-done/$ln_" || die "land(제출): 넣은 제출 기록을 받지 못했다(git fetch) — 같은 줄을 다시"
    if [ "$(git rev-parse -q --verify "$dn_^" 2>/dev/null)" = "$sh_" ]; then
      echo "hc-lane land: 이 커밋은 병합기가 이미 $base 에 넣었다(refs/merge-done/$ln_) — 제출할 것 없음"; exit 0
    fi
  fi
  # 같은 레인의 옛 표식·기록(다시 제출하면 새 것만 남는다)
  olds_=$(printf '%s\n' "$rs_" | awk -v l="$ln_" '{ n = $2; if (sub(/^refs\/merge-queue(-meta)?\//, "", n) && n ~ /^[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-/ && substr(n, 12) == l) print $2, $1 }')
  # 큐 이름은 지금 큐와 반송된 것(refs/merge-failed/)에 없는 초로(3차 중간 2 — 같은 초에 세 번 내면 옛 반송 이름과 겹쳤다)
  e_=$(date -u +%s)
  while printf '%s\n' "$rs_" | grep -Eq "[[:space:]]refs/merge-(queue|failed)/$e_-$ln_\$"; do e_=$((e_ + 1)); done
  q_=refs/merge-queue/$e_-$ln_ qm_=refs/merge-queue-meta/$e_-$ln_
  # 제출 기록(아스트라 비교 검증 2026-11-20 ①): 감사받은 그 SHA(원본)·갈래점·고유 제출 ID(이 착지 시도 ID)·기계·터미널을 빈 트리 커밋
  # 메시지로(부모 = 원본). 병합기가 결과 사건·돌려보낼 때 이 값을 남긴다
  fk_=$(git merge-base "$sh_" "$ob_") || die "land(제출): 갈래점을 구하지 못했다(git merge-base)"
  # owner = 결과를 받을 지휘(HC_LANE_OWNER, 없으면 -) — 병합기가 사건 --to로 넘긴다(아스트라 1차 중간 12)
  ow_=$(printf %s "${HC_LANE_OWNER:--}" | tr -c 'A-Za-z0-9_.,/-' -)
  # 기계(병합기가 hc send 할지 견주는 값 — merger의 mme_와 같은 출처)·제출 ID(<기계>-<UTC 시각>-<pid>)
  mt_=$(printf %s "${HC_LANE_MACHINE:-$(hostname -s 2>/dev/null)}" | tr -c 'A-Za-z0-9_.-' -); mt_=${mt_:--}
  sid_=$mt_-$(date -u +%Y%m%dT%H%M%S)-$$
  et_=$(git hash-object -t tree -w /dev/null) &&
    mc_=$(git commit-tree "$et_" -p "$sh_" -m "merge-queue lane=$ln_ id=$sid_ orig=$sh_ fork=$fk_ base=$base host=$mt_ term=${HC_TERMINAL_ID:--} owner=$ow_ job=$(printf %s "${HC_LANE_JOB:--}" | tr -c 'A-Za-z0-9_.~-' -)") ||
    die "land(제출): 제출 기록 커밋을 만들지 못했다"
  set -- "$sh_:refs/heads/lane/$ln_" "$sh_:$q_" "$mc_:$qm_" "--force-with-lease=refs/heads/lane/$ln_:$cur_" "--force-with-lease=$q_:" "--force-with-lease=$qm_:"
  while read -r or_ os_; do
    [ -n "$or_" ] || continue
    set -- "$@" ":$or_" "--force-with-lease=$or_:$os_"
  done <<<"$olds_"
  po_=$(git push --atomic --porcelain origin "$@" 2>&1); pr_=$?
  [ -z "$llog" ] || printf '%s\n' "$po_" >>"$llog"
  # 원격 접수 확인: push가 0이어도 다시 읽어 본다(응답이 끊긴 실패도 같은 판정). 제출 기록 커밋(mc_ — 이 제출 ID가 든 이번 push만의 값)이
  # 큐 기록 자리에 있거나, 그새 병합기가 처리해 refs/merge-done/<레인>(넣음)·refs/merge-failed-meta/<이름>(돌려보냄)에 있으면 접수다.
  # 큐 표식은 병합기의 main push가 lease로 먼저 지우니 보지 않는다(2차 중간 1 — 그 틈에 부르면 접수를 실패로 냈다, 1차 중간 1)
  rb_=$(git ls-remote origin "$qm_" "refs/merge-done/$ln_" "refs/merge-failed-meta/$e_-$ln_") ||
    die "land(제출): 원격 접수를 확인하지 못했다(git ls-remote) — 같은 줄을 다시(다시 제출해도 큐에는 하나만 남는다)"
  printf '%s\n' "$rb_" | awk -v c="$qm_" -v d="refs/merge-done/$ln_" -v f="refs/merge-failed-meta/$e_-$ln_" -v m="$mc_" '
    ($2 == c || $2 == d || $2 == f) && $1 == m { g = 1 } END { exit !g }' ||
    { printf '%s\n' "$po_" >&2; die "land(제출): push 실패(위 git 출력, 종료 $pr_) — 같은 줄을 다시"; }
  # 접수가 확정된 뒤에만(2차 중간 14): 이 기계 기록 <공용 git 폴더>/hc-submitted(레인·SHA·기본 브랜치 — report 착지가 push 응답 유실로
  # 추적 참조가 없어도 제출만 한 커밋을 가린다, 1차 중간 7·2차 중간 8)과 「제출」 사건(제출 ID·큐 이름)
  printf '%s %s %s\n' "$ln_" "$sh_" "$base" >>"$(git rev-parse --git-common-dir)/hc-submitted" || echo "hc-lane land: 제출 기록(hc-submitted)을 쓰지 못했다" >&2
  # 사건 상태 「제출」(3차 중간 10 — 중간이면 wait가 「뜻밖의 종료」로 봤다). 한 줄 첫 낱말이 제출 ID — 병합기의 착지·막힘 사건(「제출 ID <ID>」)과
  # hc-fill이 이것으로 잇는다
  (report_cmd 제출 "$sh_" "$sid_ → $q_ (병합기 $mg_ 가 $base 에 넣는다)" --name "$ln_") >/dev/null || echo "hc-lane land: 제출 사건을 쓰지 못했다" >&2
  echo "hc-lane land: 제출 $(git rev-parse --short "$sh_") → $q_ (lane/$ln_) — 병합기(${mg_})가 $base 에 넣고 결과를 사건(착지·막힘 --name $ln_)으로 알린다. 이 레인은 여기서 끝(report 착지는 부르지 않아도 된다 — 불러도 적히지 않음)"
  exit 0
}
# merger --repo <병합기 전용 클론> [--once] [--tick <초>] [<기본 브랜치>]: 큐를 이름(초) 순으로 하나씩 처리한다. 한 클론에 하나만(잠금).
# 전용 클론: 레인 작업 트리를 두지 않는 곳이어야 한다(제출마다 레인 이름 그대로의 로컬 가지를 만들어 착지한다 — 문서 자리표시
# v0.NEXT@<레인>·착지 스크립트가 가지 이름을 레인으로 읽는다).
merger_cmd() {
  # MVP-1은 한 건씩만(2026-11-20 지휘 t1cj 2차 「최소판을 더 작게」 — 묶음·준비 훅은 MVP-2)
  mo_=0 mtick_=30 mr_= mb_=   # mtick_: 빈 바퀴 쉼(초) — 다른 함수가 쓰지 않는 이름(감사 중간 1: mt_를 merge-tree 결과가 덮어 sleep <SHA>로 쉬지 않고 돌았다)
  while [ $# -gt 0 ]; do
    case $1 in
      --repo) val "$@"; mr_=$2; shift 2 ;;
      --once) mo_=1; shift ;;
      --tick) val "$@"; mtick_=$2; shift 2 ;;
      -h|--help) usage ;;
      -*) die "merger: 알 수 없는 인자: $1" ;;
      *) [ -z "$mb_" ] || die "merger: 남는 인자: $1"; mb_=$1; shift ;;
    esac
  done
  case $mtick_ in ''|*[!0-9]*) die "merger: --tick은 초(정수): $mtick_" ;; esac
  [ -n "$mr_" ] || die "merger: --repo <병합기 전용 클론> 이 필요하다"
  # Go 빌드 캐시를 macOS가 비우는 ~/Library/Caches 밖에(2026-11-20 verify-speed-study: 워커 Mac mini에서 캐시 항목이 사라져 「could not
  # import os … no such file」로 전체 검증을 처음부터 다시 두 번). 병합기는 같은 클론·같은 캐시를 계속 쓰니 캐시가 곧 속도다
  [ -n "${GOCACHE-}" ] || export GOCACHE="$HOME/.cache/go-build"
  case $0 in /*) self_=$0 ;; */*) self_=$PWD/$0 ;; *) self_=$(command -v "$0" 2>/dev/null || echo "$0") ;; esac
  cd "$mr_" 2>/dev/null && top=$(git rev-parse --show-toplevel 2>/dev/null) && cd "$top" || die "merger: git 저장소가 아니다: $mr_"
  git remote get-url origin >/dev/null 2>&1 || die "merger: origin이 없다: $top"
  base=${mb_:-$(origin_base .)}
  plain "$base" || die "merger: 기본 브랜치 이름에 쓸 수 없는 글자가 있다: $base"
  # 켜졌는지는 바퀴마다 본다(mround — 켬 표식이 이 기계가 아니면 큐를 건드리지 않는다)
  # 전용 착지 스크립트(.claude/herdr-land.sh)가 있는 저장소도 받는다 — 착지는 그 스크립트의 origin/<기본> 판을 쓴다(제출 트리 속 판이
  # 아니다, 2단계 2026-11-20 지휘 t1cj). 새 임시 작업 트리의 준비 훅(LANE_REPO)은 이 클론을 메인 폴더로 본다
  mtop_=$top
  mkdir -p "$evdir" || die "merger: 상태 폴더를 만들지 못했다: $evdir"
  # 잠금 = 이 셸의 열린 파일(fd 19)에 건 flock(구조 C). pid를 보지 않는다. macOS엔 flock 명령이 없어 perl의 flock(2)로 같은 fd를 잠근다.
  # 오래 사는 자식(준비 훅·착지·빈 바퀴 sleep)은 fd 19를 닫고 부른다 — 병합기가 죽으면 잠금도 바로 놓인다(자식이 살아 있어도). 물려주면
  # 끊긴 착지의 검증이 남긴 고아(go 시험의 sleep 등, ppid 1)가 잠금을 쥐어 새 병합기가 75로 끝났다(2026-11-20 mini-m6-2, 지휘 t1cj —
  # 옛 1차 높음 9의 「착지가 끝날 때까지 쥠」을 버림. 살아 남은 옛 착지와 새 병합기가 겹쳐도 기본 브랜치·큐 lease CAS가 지킨다)
  # ponytail: 짧은 git 자식은 그대로 물려받음 — ssh ControlPersist처럼 오래 남는 손자가 생기면 그 호출에도 19>&-
  lk=$evdir/merger.$(ckey "$top").lock
  # 먼저 닫는다: macOS bash 3.2는 이미 열린 fd 19에 exec 19>>파일을 해도 fd를 바꾸지 않는다 — 다른 병합기 아래(그 착지가 돌린 시험 등)에서
  # 뜨면 물려받은 바깥 잠금(같은 열린 파일이라 flock도 성공)을 쥐고 자기 잠금은 비운 채 돌았다(2026-11-20 병합기 첫 착지 15분 멈춤)
  exec 19>&-
  exec 19>>"$lk" || die "merger: 잠금 파일을 열지 못했다: $lk"
  if command -v flock >/dev/null 2>&1; then flock -n 19; lr_=$?; [ $lr_ != 1 ] || lr_=75
  else perl -e 'use Fcntl qw(:flock); open(my $f, ">&=", 19) or exit 1; flock($f, LOCK_EX | LOCK_NB) or exit 75; exit 0'; lr_=$?; fi
  case $lr_ in
    0) ;;
    75) echo "hc-lane merger: 이 클론의 병합기가 아직 돈다 — 잠금 $lk" >&2; exit 75 ;;
    *) die "merger: 잠금을 걸지 못했다(종료 $lr_): $lk" ;;
  esac
  mgrun_=1   # 여기부터 die는 막힘 사건도 남긴다
  trap 'exit 143' TERM; trap 'exit 130' INT; trap 'exit 129' HUP
  mj_=$(git rev-parse --git-path hc-merger) && mkdir -p "$mj_/out" "$mj_/fail" && mj_=$(cd "$mj_" && pwd -P) || die "merger: 일지 폴더를 만들지 못했다"
  # 후보 작업 트리 자리: 클론 옆 <클론>.merger-wt/ — .git 밖·점으로 시작하는 마디 없이(2026-11-20 한 저장소의 병합기 첫 착지: .git/hc-merger/wt/.train-c.*
  # 아래라 Vite의 server.fs.deny(**/.git/**)·점 폴더 거절로 브라우저 점검 smoke.html이 403, 지휘 t1cj)
  mwr_=$mtop_.merger-wt
  mkdir -p "$mwr_" && mwr_=$(cd "$mwr_" && pwd -P) || die "merger: 후보 작업 트리 폴더를 만들지 못했다: $mtop_.merger-wt"
  # 클론 작업 트리는 분리 HEAD로 둔다 — 가지를 체크아웃해 두면 착지 뒤 메인 폴더 맞추기(ffmain)가 클론 작업 트리를 바꾼다(3차 높음 6과 같은
  # 꼴: 바꾸다 꺼지면 클론이 더러워져 큐가 멈춘다). 클론이 깨끗할 때만, 한 번
  if git symbolic-ref -q HEAD >/dev/null; then
    st_=$(git status --porcelain --untracked-files=all) && [ -z "$st_" ] && git checkout -q --detach || die "merger: 병합기 클론을 분리 HEAD로 두지 못했다(커밋 안 된 변경?): $top"
  fi
  ml_=$evdir/merger-logs
  mkdir -p "$ml_" || die "merger: 로그 폴더를 만들지 못했다: $ml_"
  mme_=${HC_LANE_MACHINE:-$(hostname -s 2>/dev/null)}
  if [ $mo_ = 1 ]; then echo "hc-lane merger: $top ($base) 큐가 빌 때까지 한 번"; else echo "hc-lane merger: $top ($base) 큐를 본다 — 빈 바퀴 ${mtick_}초"; fi
  while :; do
    mround
    # --once: 큐가 빌 때까지(시험·손으로 한 번 돌리기). 원격 오류로 못 본 바퀴면 75(아스트라 1차 중간 4 — 빈 큐와 가린다)
    if [ $mo_ = 1 ] && [ -z "$mq_" ]; then [ -z "$mer_" ] || exit 75; break; fi
    # 쉼 값은 매번 숫자인지 본다(아니면 30) — 어떤 함수가 덮어도 쉬지 않고 원격을 두드리지 않게
    case $mtick_ in ''|*[!0-9]*) mtick_=30 ;; esac
    [ -n "$mq_" ] || sleep "$mtick_" 19>&-
  done
}
# 한 바퀴. mq_ = 처리한 큐 참조(없거나 원격 오류로 쉬면 빈 값), mer_ = 원격 오류로 못 본 것이 있음.
# 일지 하나(구조 A, $mj_/cur): 착지를 부르기 **전에** 원자적으로(tmp→mv) 「<가지> <기준 B> <후보 C>」 한 줄 + 담긴 제출마다 「<큐 참조>
# <제출 SHA> <레인> <기록 SHA|->」를 쓴다. 바퀴마다 새 제출을 고르기 전에 그 일지부터 정리한다(mrecover). 큐 표식은 기본 브랜치 push가
# lease로 함께 지우고(구조 B), 정리는 기본 브랜치가 후보를 담은 것을 본 뒤에만 한다
mround() {
  mq_= mer_= mbn_= ls0_= ls1_=
  mflush
  # 병합기 클론은 병합기만 쓴다 — 앞 실행이 남긴 멈춘 rebase는 풀고, 그래도 깨끗하지 않으면 멈춘다(모르는 변경을 지우지 않음)
  rbabort
  st_=$(git status --porcelain --untracked-files=all) || die "merger: git status 실패 — 클론 상태를 모른다"
  [ -z "$st_" ] || die "merger: 병합기 클론에 커밋 안 된 변경·새 파일이 있다 — 살펴 치운 뒤 다시 띄운다: $PWD"
  mwtclean
  mrecover || return 0
  mfretry
  qs_=$(git ls-remote origin 'refs/merge-queue/*' 'refs/merge-queue-meta/*' 'refs/herdr/merger/*') || { echo "hc-lane merger: 원격 큐를 읽지 못했다(git ls-remote) — 다음 바퀴에" >&2; mer_=1; return 0; }
  # 켬 표식이 이 기계인지 바퀴마다 본다 — 길 안내일 뿐 정확성 조건이 아니다(3차 지휘 t1cj: main에 쓰는 모든 길이 main CAS라 병합기가
  # 둘 돌거나 전환 중 직접 착지가 끼어도 main은 틀리지 않는다 — 한쪽 push가 거절되고 다시 할 뿐. 큐를 고치는 원격 쓰기는 모두 그 큐
  # 참조의 지금 SHA에 lease를 건다)
  msha_=$(printf '%s\n' "$qs_" | awk -v r="refs/herdr/merger/$mme_" '$2 == r { print $1 }')
  if [ -z "$msha_" ]; then
    mo2_=$(printf '%s\n' "$qs_" | awk 'index($2, "refs/herdr/merger/") == 1 { print substr($2, 19); exit }')
    [ -z "$mo2_" ] || die "merger: 켬 표식이 다른 기계(refs/herdr/merger/$mo2_)다 — 이 병합기($mme_)는 멈춘다(기계를 옮겼거나 이름이 다르면 HC_LANE_MACHINE=<표식의 기계>로)"
    echo "hc-lane merger: 켬 표식 refs/herdr/merger/$mme_ 가 없다(꺼짐) — 큐를 건드리지 않고 쉰다" >&2
    mer_=1; return 0
  fi
  # 제출 기록(원본·갈래점·제출 ID·기본 브랜치·지휘 — 레인이 같은 push로 올림)을 한 번에 받는다. 못 받으면 이 바퀴는 쉰다
  if printf '%s\n' "$qs_" | grep -q 'refs/merge-queue-meta/'; then
    fetchr git fetch -q origin '+refs/merge-queue-meta/*:refs/hc-merger/meta/*' 2>/dev/null ||
      { echo "hc-lane merger: 제출 기록을 받지 못했다(git fetch) — 다음 바퀴에" >&2; mer_=1; return 0; }
  fi
  # 큐를 이름(초) 순으로 앞에서 묶음 크기만큼(HC_LANE_MERGER_BATCH, 기본 3 — 묶음이 실패하면 $mj_/cap으로 하나씩 줄여 다시).
  # 반송을 다시 하는 중(fail/)·다른 기본 브랜치 제출은 건너뛴다
  mN_=${HC_LANE_MERGER_BATCH:-3}; case $mN_ in ''|*[!0-9]*|0) mN_=3 ;; esac
  mc_=$(cat "$mj_/cap" 2>/dev/null) || mc_=; case $mc_ in ''|*[!0-9]*|0) ;; *) [ "$mc_" -ge "$mN_" ] || mN_=$mc_ ;; esac
  bq_=() bs_=() bl_=() bm_=() bi_=() bdl_=() bj_= bd_=
  while read -r mq_ ms_; do
    [ -n "$mq_" ] || continue
    mn_=${mq_#refs/merge-queue/}
    [ ! -f "$mj_/fail/$mn_" ] || continue   # 돌려보내기를 다시 하는 중(mfretry)
    ln_=${mn_#??????????-}
    case $mn_ in [0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-?*) ;; *) ln_= ;; esac
    mm_=$(printf '%s\n' "$qs_" | awk -v r="refs/merge-queue-meta/$mn_" '$2 == r { print $1 }')
    # 레인 이름이 가지 이름으로 못 쓰는 꼴이면 그 항목만 반송(2차 중간 13 — 후보 가지를 만들다 병합기가 멈춰 큐 전체가 막혔다)
    # 묶음 뒤쪽에서 걸리면 묶음을 거기서 끊는다(그 항목은 맨 앞이 될 때 처리 — 앞 제출을 막지 않게)
    if ! lname_ok "$ln_"; then
      [ ${#bq_[@]} = 0 ] || break
      mfail "$mq_" "$ms_" "${ln_:--}" "${mm_:--}" - "큐 이름 꼴이 아니다(<UTC초 10자리>-<레인>, 레인은 가지 이름으로 쓸 수 있는 a-z 0-9 - _)" || mer_=1; mq_=x; return 0
    fi
    if ! mmeta "$mm_" "$ms_"; then
      [ ${#bq_[@]} = 0 ] || break
      echo "hc-lane merger: 제출 기록 $mn_ 을 읽지 못했다(또는 원본과 안 맞음) — 다음 바퀴에" >&2; mer_=1; mq_=; return 0
    fi
    # 다른 기본 브랜치로 낸 제출은 건드리지 않는다 — 그 브랜치의 병합기 몫(1차 높음 7)
    if [ -n "$mbs_" ] && [ "$mbs_" != "$base" ]; then echo "hc-lane merger: $ln_ 건너뜀 — 기본 브랜치 $mbs_ 로 낸 제출(이 병합기는 $base)" >&2; continue; fi
    [ ${#bq_[@]} -gt 0 ] || bj_=$mjb_ bd_=$mid_   # 작업 표식은 맨 앞 제출 것만
    bq_+=("$mq_") bs_+=("$ms_") bl_+=("$ln_") bm_+=("${mm_:--}") bi_+=("$mi_") bdl_+=("$mid_")
    [ ${#bq_[@]} -lt "$mN_" ] || break
  done <<<"$(printf '%s\n' "$qs_" | awk 'NF == 2 && index($2, "refs/merge-queue/") == 1 { print $2, $1 }' | LC_ALL=C sort)"
  mq_=
  [ ${#bq_[@]} -gt 0 ] || return 0
  mq_=${bq_[0]}
  fetchr git fetch -q origin "+refs/heads/$base:refs/remotes/origin/$base" '+refs/merge-queue/*:refs/hc-merger/q/*' ||
    { echo "hc-lane merger: git fetch 실패 — 다음 바퀴에" >&2; mer_=1; mq_=; return 0; }
  B_=$(git rev-parse -q --verify "refs/remotes/origin/$base^{commit}") || die "merger: origin/$base 가 없다"
  # 이미 기본 브랜치에 든 제출은 착지 없이 정리만(병합기를 켜기 전에 들어갔거나 손으로 넣음). 정리가 안 되면 쉰다 — --once가 같은 항목을
  # 쉬지 않고 되풀이하지 않게(3차 중간 3). 후보는 그 제출 자신(작업 표식도 그것으로 — 3차 중간 4)
  if ianc "${bs_[0]}" "$B_"; then
    mbn_=1; mi_=${bi_[0]}; mdone "${bq_[0]}" "${bs_[0]}" "${bl_[0]}" "${bm_[0]}" "${bs_[0]}" "제출이 이미 $base 에 있음" || { mer_=1; mq_=; }
    return 0
  fi
  # 제출 내용이 이미 기본 브랜치에 다 들었으면(같은 패치가 다른 SHA로 — 합친 트리 = 기본 브랜치 트리) 착지·검증 없이 정리만
  # (2026-11-20 지휘 t1cj — 이미 든 커밋을 rebase 뒤 main과 같은 트리로 전체 검증했다)
  if mgt_=$(git merge-tree --write-tree "$B_" "${bs_[0]}" 2>/dev/null) && [ "$mgt_" = "$(git rev-parse "$B_^{tree}")" ]; then
    mbn_=1; mi_=${bi_[0]}; mdone "${bq_[0]}" "${bs_[0]}" "${bl_[0]}" "${bm_[0]}" "$B_" "제출 내용이 이미 $base 에 있음(합친 트리가 같음)" || { mer_=1; mq_=; }
    return 0
  fi
  # 착지 스크립트는 기본 브랜치 판을 쓰니(land_cmd) 제출이 그 스크립트를 고쳐도 된다. 다만 기본 브랜치에 없는 스크립트를 제출이 새로
  # 들이면 반송 — 병합기가 제출 트리 속 착지 코드를 실행하지 않게(2차 높음 1)
  hl_=$(git ls-tree --name-only "${bs_[0]}" -- .claude/herdr-land.sh) && hb_=$(git ls-tree --name-only "$B_" -- .claude/herdr-land.sh) ||
    die "merger: git ls-tree 실패(${bs_[0]} · $B_)"
  if [ -n "$hl_" ] && [ -z "$hb_" ]; then
    mi_=${bi_[0]}; mfail "${bq_[0]}" "${bs_[0]}" "${bl_[0]}" "${bm_[0]}" - "$mhook_" || mer_=1
    return 0
  fi
  # main push에 함께(HERDR_LAND_PUSH_EXTRA = git push 인자 목록 — 착지 표식과 같은 이름·꼴, 지휘 t1cj):
  #  · 큐 표식 삭제 + 그 SHA lease(구조 B — 다시 제출했으면 main도 안 움직인다)
  #  · 제출 ID 표식 HEAD:refs/hc-landed/<제출 ID>(3차 높음 4 — 재시작 때 이 표식이 main의 조상이면 완료, 패치 일치로 판정하지 않는다)
  #  · 작업 표식 HEAD:refs/hc-landed/<작업 ID>(2차 중간 12 — 원격에 아직 없을 때만)
  # 묶음 뒤쪽(2번째부터) 거르기 — 맨 앞이 될 때 따로 처리할 것은 뺀다: 이미 기본 브랜치에 든 것(트리 같음 포함)·기본 브랜치에 없는
  # 착지 스크립트를 들이는 것·저장소가 「한 묶음에 하나만」으로 정한 경로(기본 브랜치의 .claude/herdr-merge-solo, 줄마다 git 경로 —
  # 예: 착지가 한 번에 하나만 접는 문서 조각 폴더)를 앞 구성원과 함께 건드리는 것
  msolo_=(); if mso_=$(git show "$B_:.claude/herdr-merge-solo" 2>/dev/null); then while IFS= read -r l_; do case $l_ in ''|'#'*) ;; *) msolo_+=("$l_") ;; esac; done <<<"$mso_"; fi
  msu_=   # 묶음에 「하나만」 경로를 건드린 구성원이 이미 있음
  # 갈래점 뒤 커밋 하나라도 그 경로를 건드렸으면 — 순 차이로 보지 않는다(더했다 지운 앞선 접기 커밋은 착지가 풀어 조각이 돌아온다)
  mtouch() { [ ${#msolo_[@]} -gt 0 ] && [ -n "$(git rev-list -n 1 "$(git merge-base "$1" "$B_")..$1" -- "${msolo_[@]}" 2>/dev/null)" ]; }
  ! mtouch "${bs_[0]}" || msu_=1
  if [ ${#bq_[@]} -gt 1 ]; then
    k_=(0)
    for i_ in $(seq 1 $((${#bq_[@]} - 1))); do
      s_=${bs_[i_]}
      ianc "$s_" "$B_" && continue
      mgt_=$(git merge-tree --write-tree "$B_" "$s_" 2>/dev/null) && [ "$mgt_" = "$(git rev-parse "$B_^{tree}")" ] && continue
      [ -n "$(git ls-tree --name-only "$s_" -- .claude/herdr-land.sh)" ] && [ -z "$hb_" ] && continue
      if mtouch "$s_"; then [ -z "$msu_" ] || continue; msu_=1; fi
      k_+=("$i_")
    done
    mkeep_ "${k_[@]}"
  fi
  mjx_=   # 작업 표식(맨 앞 제출 것, 원격에 아직 없을 때만)
  if [ -n "$bj_" ] && [ "$bj_" != - ] && [ "$bj_" != "$bd_" ] && git check-ref-format "refs/hc-landed/$bj_"; then
    hk_=$(git ls-remote origin "refs/hc-landed/$bj_") || { echo "hc-lane merger: 작업 표식을 보지 못했다(git ls-remote) — 다음 바퀴에" >&2; mer_=1; mq_=; return 0; }
    [ -n "$hk_" ] || mjx_="HEAD:refs/hc-landed/$bj_"
  fi
  mexbuild
  br_=${bl_[0]}
  lgf_=$ml_/${bl_[0]}-$(date +%Y%m%d-%H%M%S)-$$.log
  C0_=${bs_[0]}
  # 일지 — 행동 전에(작업 트리를 만들기 전에 — 3차 높음 6)
  mjwrite
  # 후보는 매번 새 임시 작업 트리(병합기 자기 폴더 <클론>.merger-wt/)에서 레인 이름 가지로 — 병합기 클론의 작업 트리는 건드리지 않는다.
  # 만들다 꺼져도 다음 바퀴가 그 폴더의 작업 트리를 지우고 다시 만든다(3차 높음 6 — 클론에서 체크아웃하다 꺼지면 더러운 클론으로 큐 전체가 멈췄다)
  mwt_=$(mktemp -d "$mwr_/c-XXXXXX") && git worktree add -q -f -B "$br_" "$mwt_" "$C0_" || die "merger: 후보 작업 트리를 만들지 못했다($br_)"
  # 묶음: 뒤쪽 구성원을 앞 구성원 위에 차례로 얹는다(그 제출이 기본 브랜치에서 갈라진 뒤의 커밋만). 충돌하는 구성원은 이번 묶음에서 빼고
  # 큐에 그대로(맨 앞이 될 때 혼자 — 충돌이면 그때 반송). 착지는 이 가지 전체를 기본 브랜치에 rebase·검증 한 번·push 한 번(CAS)
  if [ ${#bq_[@]} -gt 1 ]; then
    k_=(0)
    for i_ in $(seq 1 $((${#bq_[@]} - 1))); do
      mf_=$(git merge-base "${bs_[i_]}" "$B_") || die "merger: git merge-base 실패(${bl_[i_]})"
      if (cd "$mwt_" && git rebase -q --onto "$br_" "$mf_" "${bs_[i_]}" && git checkout -q -B "$br_") >>"$lgf_" 2>&1 </dev/null; then
        k_+=("$i_")
      else
        (cd "$mwt_" && { git rebase --abort; git checkout -q -f "$br_"; }) >>"$lgf_" 2>&1 </dev/null
        echo "hc-lane merger: 묶음에서 뺌 — ${bl_[i_]}(앞 구성원 위에 얹으면 충돌, 큐에 그대로)" >&2
      fi
    done
    if [ ${#k_[@]} -lt ${#bq_[@]} ]; then
      mkeep_ "${k_[@]}"; mjwrite; mexbuild
    fi
  fi
  mlab_=$(IFS=+; printf '%s' "${bl_[*]}")
  echo "hc-lane merger: 착지 시작 — $mlab_ (기준 $(printf %.8s "$B_")${bq_[1]+, 묶음 ${#bq_[@]}})"
  mbn_=${#bq_[@]} ls0_=$(date +%s)
  # 새 작업 트리라 준비 훅(.claude/herdr-setup.sh — 의존성·빌드 산출물)을 레인을 만들 때처럼 먼저. 실패는 일시 실패로(75 — 망 등)
  # 준비 훅의 코드 색인(곧 지울 폴더를 후보마다 색인하면 검증과 CPU를 다툰다)은 색인 도구를 PATH에서 빼 건너뛰게 한다 — 준비 훅이
  # 「도구가 없으면 색인 안 함」이라 저장소를 고치지 않고 된다
  # ponytail: 도구 이름 하나로 거름, 준비 훅이 다른 색인 길을 쓰면 그때 LANE_MERGER 같은 표시로
  mpath_=$(printf '%s\n' "$PATH" | tr ':' '\n' | while IFS= read -r d_; do [ -x "$d_/codebase-memory-mcp" ] || printf '%s:' "$d_"; done); mpath_=${mpath_%:}
  if [ -f "$mwt_/.claude/herdr-setup.sh" ] &&
    ! (cd "$mwt_" && PATH=$mpath_ LANE_REPO=$mtop_ LANE_BASE=$base exec bash .claude/herdr-setup.sh) >>"$lgf_" 2>&1 </dev/null 19>&-; then
    echo "hc-lane merger: 준비 훅(.claude/herdr-setup.sh) 실패" >>"$lgf_"; lr_=75
  else
    (cd "$mwt_" && HERDR_MERGER=1 HC_LANE=$br_ HERDR_LAND_PUSH_EXTRA="$ex_" exec "$self_" land "$base") >>"$lgf_" 2>&1 </dev/null 19>&-; lr_=$?
  fi
  ls1_=$(date +%s)
  tail -n 5 "$lgf_"
  mjudge
}
# mkeep_ <번호>…: 묶음 배열(bq_ bs_ bl_ bm_ bi_ bdl_)에서 그 번호만 남긴다(순서 그대로)
mkeep_() {
  local q=() s=() l=() m=() i=() d=() x
  for x in "$@"; do q+=("${bq_[x]}") s+=("${bs_[x]}") l+=("${bl_[x]}") m+=("${bm_[x]}") i+=("${bi_[x]}") d+=("${bdl_[x]}"); done
  bq_=("${q[@]}") bs_=("${s[@]}") bl_=("${l[@]}") bm_=("${m[@]}") bi_=("${i[@]}") bdl_=("${d[@]}")
}
# mexbuild: main push에 함께 실을 인자(ex_) — 구성원마다 큐 표식 삭제 + 그 SHA lease와 제출 ID 표식, 끝에 작업 표식(mjx_)
mexbuild() {
  ex_=
  for i_ in "${!bq_[@]}"; do
    ex_="$ex_${ex_:+ }:${bq_[i_]} --force-with-lease=${bq_[i_]}:${bs_[i_]}"
    [ -z "${bdl_[i_]}" ] || ! git check-ref-format "refs/hc-landed/${bdl_[i_]}" || ex_="$ex_ HEAD:refs/hc-landed/${bdl_[i_]}"
  done
  [ -z "$mjx_" ] || ex_="$ex_ $mjx_"
}
# mjwrite: 일지(cur) — 후보 가지·기준·후보 첫 커밋 한 줄, 담긴 제출마다 한 줄. tmp→mv
mjwrite() {
  { printf '%s %s %s\n' "$br_" "$B_" "$C0_"
    for i_ in "${!bq_[@]}"; do printf '%s %s %s %s\n' "${bq_[i_]}" "${bs_[i_]}" "${bl_[i_]}" "${bm_[i_]}"; done
  } >"$mj_/cur.tmp" && mv -f "$mj_/cur.tmp" "$mj_/cur" || die "merger: 일지를 쓰지 못했다"
}

# mwtclean: 병합기 자기 임시 작업 트리(<클론>.merger-wt/ 아래, 앞 판의 .git/hc-merger/wt/ 아래도)를 모두 지운다 — 앞 실행이 남긴 것 포함
# (병합기 것만이라 지워도 된다)
mwtclean() {
  wl_=$(git worktree list --porcelain) || die "merger: git worktree list 실패"
  printf '%s\n' "$wl_" | sed -n 's/^worktree //p' | while IFS= read -r w_; do
    case $w_ in "$mwr_/"*|"$mj_/wt/"*) git worktree remove -f -f "$w_" >/dev/null 2>&1 || echo "hc-lane merger: 남은 작업 트리를 지우지 못했다: $w_" >&2 ;; esac
  done
  git worktree prune >/dev/null 2>&1
  # ponytail: 등록 전에 끊긴 빈 폴더(mktemp만 된 것)는 남는다 — 비어 있어 해가 없다, 쌓이면 손으로
  mwt_=
}
# mjudge: 착지(종료 lr_) 뒤 판정. 종료 코드와 상관없이 먼저 원격이 후보를 담았는지 본다(1차 높음 3 — 응답 유실로 1이어도 들어갔을 수 있다).
# 못 보면 일지를 두고 쉰다(다음 바퀴의 mrecover가 같은 판정)
mjudge() {
  fetchr git fetch -q origin "+refs/heads/$base:refs/remotes/origin/$base" ||
    { echo "hc-lane merger: 착지 뒤 git fetch 실패 — 일지를 두고 다음 바퀴에 판정" >&2; mer_=1; mq_=; return 0; }
  O_=$(git rev-parse -q --verify "refs/remotes/origin/$base^{commit}") && C_=$(git rev-parse -q --verify "refs/heads/$br_") ||
    die "merger: 착지 뒤 후보·origin/$base 를 읽지 못했다 — 다음 실행이 일지로 판정한다"
  if ianc "$C_" "$O_"; then
    mdone_all "$C_" "병합기 착지${bq_[1]+(묶음 ${#bq_[@]}: $mlab_)}" || { mer_=1; mq_=; return 0; }
    # 묶음 2번째부터의 레인 표식 refs/hc-landed/<레인>/<UTC 초>(fill-dup-guard — 맨 앞은 착지가 붙였다). 못 붙여도 착지는 됨
    for i_ in "${!bl_[@]}"; do [ "$i_" = 0 ] || git push -q origin "$C_:refs/hc-landed/${bl_[i_]}/$(date -u +%s)" </dev/null 2>/dev/null ||
      echo "hc-lane merger: ${bl_[i_]} 착지 표식을 두지 못했다(착지는 됨)" >&2; done
    rm -f "$mj_/cap"
    mfin; return 0
  fi
  case $lr_ in
    0) lr_="0(그런데 $base 에 없음)" ;;
    6) echo "hc-lane merger: 함께 실은 lease 참조가 바뀜(그새 다시 제출함) — 후보를 버리고 다음 바퀴에" >&2; mfin; return 0 ;;
  esac
  if mtrans "$lr_" "$lgf_" && mtry "${bq_[0]}" "${bl_[0]}"; then
    # 일시 실패(기다림 상한·push 거절 횟수·원격 받기 실패)는 큐에 그대로 두고 쉰다(1차 중간 2 — 3번 넘으면 돌려보냄).
    # --once는 75로 끝난다(2차 중간 11 — 큐를 남긴 채 0이었다)
    echo "hc-lane merger: ${bl_[0]} 일시 실패(종료 $lr_, 로그 $lgf_) — 큐에 두고 다음 바퀴에" >&2
    mfin; mq_=; mer_=1; return 0
  fi
  # 묶음이 실패하면(일시 실패 아님) 아무도 돌려보내지 않고 마지막 것을 빼고 다시 — 다음 바퀴의 묶음 크기 $mj_/cap = 지금 - 1.
  # 혼자일 때 실패해야 그 제출을 돌려보낸다
  if [ ${#bq_[@]} -gt 1 ]; then
    echo "hc-lane merger: 묶음 ${#bq_[@]}($mlab_) 실패(종료 $lr_, 로그 $lgf_) — 마지막 ${bl_[${#bl_[@]}-1]} 빼고 다시" >&2
    echo $((${#bq_[@]} - 1)) >"$mj_/cap.tmp" && mv -f "$mj_/cap.tmp" "$mj_/cap" || die "merger: 묶음 크기를 적지 못했다"
    mfin; mq_=x; return 0
  fi
  mtrans "$lr_" "$lgf_" && lr_="$lr_, 일시 실패가 3번 넘음"
  mfin; rm -f "$mj_/cap"
  mi_=${bi_[0]}; mfail "${bq_[0]}" "${bs_[0]}" "${bl_[0]}" "${bm_[0]}" "$lgf_" "착지 실패(종료 $lr_)" || mer_=1
}
# mrecover: 일지가 있으면(앞 실행이 끊김) 먼저 정리. 0 정리함(새로 골라도 됨) · 1 원격을 못 봐 쉼.
#  제출 ID 표식이 main에 있음 → 그 제출 완료 / 그 밖 → 후보 버림, 제출은 큐에 남아 있으면 다시 고른다(큐 표식은 main push로만 지워진다)
mrecover() {
  [ -f "$mj_/cur" ] || return 0
  read -r br_ jB_ jC_ <"$mj_/cur" || die "merger: 일지를 읽지 못했다: $mj_/cur"
  fetchr git fetch -q origin "+refs/heads/$base:refs/remotes/origin/$base" ||
    { echo "hc-lane merger: 끊긴 앞 실행 판정에 git fetch 실패 — 다음 바퀴에" >&2; mer_=1; return 1; }
  O_=$(git rev-parse -q --verify "refs/remotes/origin/$base^{commit}") || die "merger: origin/$base 가 없다"
  # 들어갔는지는 main push가 원자로 함께 실은 제출 ID 표식 refs/hc-landed/<제출 ID>로만 본다 — 그 표식이 있고 그 SHA가 main의
  # 조상이면 완료, 아니면 처음부터(제출은 큐에 남아 있으면 다시 고른다). 패치 일치(git cherry)는 쓰지 않는다(3차 높음 4 — 다시 제출해
  # 철회된 옛 제출이 같은 패치로 완료가 됐다). 제출 기록이 없는 큐 항목(손으로 넣음)만 후보 가지 끝으로 본다
  bq_=() bs_=() bl_=() bm_=() bi_=() bd2_=()
  while read -r q_ s_ ln_ m_; do
    [ -n "$q_" ] || continue
    # 제출 기록이 있는데 못 읽으면 쉰다(3차 중간 7 — 제출 ID 없이 끝냈다)
    mmeta "${m_#-}" "$s_" 2>/dev/null || { echo "hc-lane merger: 끊긴 앞 실행의 제출 기록($ln_)을 읽지 못했다 — 일지를 두고 다음 바퀴에" >&2; mer_=1; return 1; }
    bq_+=("$q_") bs_+=("$s_") bl_+=("$ln_") bm_+=("$m_") bi_+=("$mi_") bd2_+=("$mid_")
  done < <(tail -n +2 "$mj_/cur")
  mbn_=${#bq_[@]}
  set --; for i_ in "${!bq_[@]}"; do [ -z "${bd2_[i_]}" ] || set -- "$@" "refs/hc-landed/${bd2_[i_]}"; done
  lk2_=; [ $# = 0 ] || lk2_=$(git ls-remote origin "$@") || { echo "hc-lane merger: 끊긴 앞 실행 판정에 제출 ID 표식을 못 읽음 — 다음 바퀴에" >&2; mer_=1; return 1; }
  for i_ in "${!bq_[@]}"; do
    if [ -n "${bd2_[i_]}" ]; then
      hs_=$(printf '%s\n' "$lk2_" | awk -v r="refs/hc-landed/${bd2_[i_]}" '$2 == r { print $1 }')
      [ -n "$hs_" ] && { git cat-file -e "$hs_^{commit}" 2>/dev/null || fetchr git fetch -q origin "+refs/heads/$base:refs/remotes/origin/$base"; } &&
        ianc "$hs_" "$O_" || { echo "hc-lane merger: 끊긴 앞 실행의 ${bl_[i_]} — 제출 ID 표식이 main에 없다, 처음부터(큐에 남아 있으면 다시 고른다)"; continue; }
    else
      C_=$(git rev-parse -q --verify "refs/heads/$br_"); cr_=$?
      case $cr_ in 0) ;; 1) C_=$jC_ ;; *) echo "hc-lane merger: 끊긴 앞 실행의 후보 가지를 읽지 못했다(종료 $cr_) — 일지를 두고 다음 바퀴에" >&2; mer_=1; return 1 ;; esac
      ianc "$C_" "$O_" || { echo "hc-lane merger: 끊긴 앞 실행의 ${bl_[i_]}(제출 기록 없음) — main에 없다, 처음부터"; continue; }
      hs_=$C_
    fi
    echo "hc-lane merger: 끊긴 앞 실행의 ${bl_[i_]} 가 $base 에 있다($(printf %.8s "$hs_")) — 완료 처리"
    mi_=${bi_[i_]}; mdone "${bq_[i_]}" "${bs_[i_]}" "${bl_[i_]}" "${bm_[i_]}" "$hs_" "앞 병합기 실행이 넣었음" || { mer_=1; return 1; }
  done
  mfin
  mq_=x   # 이 바퀴에 일을 했다(--once가 이어 돈다)
  return 0
}
# mdone_all <C> <까닭>: 담긴 제출(bq_…)을 모두 완료. 망 오류면 1(일지를 두고 다음 바퀴에)
mdone_all() {
  for i_ in "${!bq_[@]}"; do
    mi_=${bi_[i_]}; mdone "${bq_[i_]}" "${bs_[i_]}" "${bl_[i_]}" "${bm_[i_]}" "$1" "$2" || return 1
  done
}
# mfin: 임시 작업 트리·후보 가지·일지를 치운다(병합기 클론의 작업 트리는 건드리지 않는다)
mfin() {
  mwtclean
  [ -z "${br_-}" ] || ! git show-ref -q --verify "refs/heads/$br_" || git branch -q -D "$br_" || die "merger: 후보 가지 $br_ 를 지우지 못했다"
  rm -f "$mj_/cur"
}
# mmeta <기록 SHA|빈 값> <제출 SHA>: 제출 기록을 읽어 mbs_(기본 브랜치)·mi_(사건에 붙일 글)·mo__(지휘)·mh_(기계)·mtm_(터미널)에.
# 기록이 없으면(손으로 넣은 큐 표식) 빈 값으로 0. 읽기 실패·부모가 제출 SHA가 아니면 1(1차 높음 6 — 파이프가 git 실패를 가렸다)
mmeta() {
  mbs_= mi_= mo__= mh_= mtm_= mid_= mjb_=
  [ -n "$1" ] && [ "$1" != - ] || return 0
  mp_=$(git rev-parse -q --verify "$1^" 2>/dev/null) && [ "$mp_" = "$2" ] || return 1
  mb_l=$(git log -1 --format=%B "$1" 2>/dev/null) || return 1
  mb_l=${mb_l%%$'\n'*}
  for kv_ in $mb_l; do
    case $kv_ in base=*) mbs_=${kv_#base=} ;; owner=*) mo__=${kv_#owner=} ;; host=*) mh_=${kv_#host=} ;; term=*) mtm_=${kv_#term=} ;; esac
  done
  [ "$mo__" != - ] || mo__=
  mid_=$(printf '%s\n' "$mb_l" | tr ' ' '\n' | sed -n 's/^id=//p') mjb_=$(printf '%s\n' "$mb_l" | tr ' ' '\n' | sed -n 's/^job=//p')
  mi_=$(printf '%s\n' "$mb_l" | tr ' ' '\n' | awk -F= '$1 == "id" { i = $2 } $1 == "fork" { f = substr($2, 1, 8) } END { if (i != "") printf "제출 ID %s, 갈래점 %s", i, f }')
}
# ianc <a> <b>: a가 b에 들어갔나 — 0 예, 1 아니오, 그 밖은 멈춤(git 실패를 「아니오」로 읽지 않는다)
ianc() { git merge-base --is-ancestor "$1" "$2"; ia_=$?; case $ia_ in 0|1) return $ia_ ;; esac; die "merger: git merge-base --is-ancestor 실패(종료 $ia_)"; }
rbabort() { if [ -e "$(git rev-parse --git-path rebase-merge)" ] || [ -e "$(git rev-parse --git-path rebase-apply)" ]; then git rebase --abort >/dev/null 2>&1; fi; }
# mtrans <종료> <로그>: 다시 하면 될 착지 실패면 0(감사 중간 2) — 종료 75(검증 자리·원격 차례·공개 선점 기다림 상한)·4(push 거절
# 횟수), 원격 받기 실패(내장 착지 「land: git fetch 실패」·「push 결과 모름」, 착지 스크립트 「git fetch origin 실패」). 충돌·검증 실패는 1
# 내장 착지의 「push 실패(다른 쪽 push가 앞선 것이 아니다」도 — push 전송만 끊기고 원격은 후보를 받지 않은 꼴(3차 중간 8 — 한 번 만에
# 반송했다. 들어갔으면 mjudge가 먼저 착지로, lease가 바뀌었으면 종료 6으로 가린다)
# 7: 착지가 rebase로 기본 브랜치에 막 생긴 착지 스크립트를 만남 — 다음 바퀴에는 그 판으로 맡긴다(2단계)
mtrans() { case $1 in 75|4|7) return 0 ;; esac; grep -Eq 'git fetch( origin)? 실패|push 결과 모름|push 실패\(다른 쪽' "$2" 2>/dev/null; }
# mtry <큐> <레인>: 그 제출의 일시 실패를 하나 세어 3번까지면 0(큐에 두고 쉼), 넘으면 1(돌려보냄 — 같은 실패를 끝없이 되풀이하지 않게)
mtry() {
  tf_=$mj_/$2.tr tc_=
  [ ! -f "$tf_" ] || tc_=$(awk -v q="$1" '$1 == q { print $2 }' "$tf_")
  tc_=$(( ${tc_:-0} + 1 ))
  printf '%s %s\n' "$1" "$tc_" >"$tf_" || die "merger: 일시 실패 횟수를 쓰지 못했다: $tf_"
  [ "$tc_" -le 3 ]
}
