# hc-lane 함수: land (bin/hc-lane가 읽는다. 함수는 bin/hc-lane 안 원래 정의 순서 그대로)

# 대상 기계에서 실행(종료 코드 그대로).
# core=1(hook이 hc 터미널 밖 로컬에서 켬): 로컬도 hc 코어 문맥(hc exec --)으로. launchd(hc-fill)가 띄운 bash는 ~/Documents의
# 스크립트를 못 읽는다(TCC 「Operation not permitted」, 2026-11-17 travel-c0 실측). 코어가 띄우는 프로그램은 읽는다
x() { if [ -n "$on" ]; then hc exec --on "$on" --timeout 600 -- "$@"; elif [ -n "${core-}" ]; then hc exec --timeout 600 -- "$@"; else "$@"; fi; }
xi() { if [ -n "$on" ]; then hc exec -i --on "$on" --timeout 600 -- "$@"; else "$@"; fi; }   # 표준 입력을 넘긴다
# zx <폴더> <명령…>: 대상 기계의 그 폴더에서. --on이면 hc 코어 환경이라 ~/.zshrc의 PATH(bun·gh 자격 도우미 등)가 없다
# (150·151 실측, 2026-11-16 감사 대상 fetch가 「could not read Username」). zsh·~/.zshrc가 있으면 로그인 셸(~/.zprofile의
# brew shellenv)로 ~/.zshrc까지 읽고 돈다. -i(대화형)는 쓰지 않는다: 프롬프트·자동 붙기 같은 대화형 설정이 출력을 섞거나 멈춘다.
zx() {
  x sh -c 'cd "$1" || exit 1
    if [ -n "$2" ] && [ -f "$HOME/.zshrc" ] && command -v zsh >/dev/null 2>&1; then
      d=$1; shift 2
      exec zsh -lc "source \"\$HOME/.zshrc\" >/dev/null 2>&1; cd \"\$1\" && shift && exec \"\$@\"" zsh "$d" "$@"
    fi
    shift 2; exec "$@"' hc-lane "$1" "$on${core-}" "${@:2}"
}
# fetchr <명령…>: git fetch(그대로 또는 x·zx로)를 돌리고, 같은 클론에서 다른 레인의 착지·fetch가 원격 참조를 함께 고쳐 난
# 참조 잠금 경합(cannot lock ref·unable to update local ref·is at … but expected, 한국어 git 메시지 포함)이면 1·2·4초 쉬고
# 최대 3번 다시 한다(2026-11-18 워커 기계의 레인 시작 실패 실측). 다른 실패는 바로 그 종료 코드로. 쉬는 간격은
# HC_LANE_FETCH_WAITS(시험용). 한국어 git(2.56.1 ko 카탈로그, 2026-11-19 mini-m4 실측): 「cannot lock ref」·「is at … but
# expected」는 번역이 없어 영어 그대로, 「unable to update local ref」→「로컬 레퍼런스를 업데이트할 수 없습니다」,
# 「Unable to create '….lock': …」→「'….lock'을 만들 수 없습니다: …」
fetchr() {
  fr_e=$(mktemp "${TMPDIR:-/tmp}/hc-lane-fetch.XXXXXX") || { "$@"; return; }
  for fr_w in ${HC_LANE_FETCH_WAITS:-1 2 4} -; do
    "$@" 2>"$fr_e"; fr_rc=$?
    cat "$fr_e" >&2
    [ $fr_rc != 0 ] && [ "$fr_w" != - ] &&
      grep -Eq "cannot lock ref|unable to update local ref|is at [0-9a-f]+ but expected|로컬 레퍼런스를 업데이트할 수 없습니다|\.lock'을 만들 수 없습니다" "$fr_e" || break
    echo "hc-lane: git fetch 참조 잠금 경합(같은 클론의 다른 fetch·착지) — ${fr_w}초 뒤 다시" >&2
    sleep "$fr_w"
  done
  rm -f "$fr_e"; return $fr_rc
}
q() { printf "'%s'" "$(printf %s "$1" | sed "s/'/'\\\\''/g")"; }

# ffmain <메인 폴더> <기본>: 메인 폴더를 origin/<기본>으로 fast-forward(2026-11-18 main-folder-ff — 워커 기계 메인 폴더가 이틀 동안
# 안 앞당겨져 main보다 375~497커밋 뒤, --read 감사가 옛 CLAUDE.md·계약·시험을 읽었다). 기본 브랜치에 있고 추적 파일 변경과
# 진행 중인 merge·rebase·cherry-pick·revert가 없을 때만 merge --ff-only. 추적 안 된 파일과 부딪히면 git이 거절해 그대로 둔다.
# 무시된 파일도 덮지 않는다(--no-overwrite-ignore — git 기본은 덮음, 감사 M1: 무시된 .env를 upstream이 추적하기 시작하면 조용히 바뀜).
# skip-worktree·assume-unchanged 표시 파일은 status에 안 보여 건너뛴다(감사 L4). 그 기계에서 메인 폴더(아래)를 cwd로 쓰는 살아 있는
# working·blocked 터미널(부른 터미널 자신 빼고)이 있으면 건너뛴다 — 일하는 세션 발밑에서 파일이 바뀌지 않게(감사 M2). idle·done이면 앞당긴다.
# stash·reset·checkout은 쓰지 않는다. 결과 한 줄(앞당김·최신은 표준 출력, 건너뜀은 오류 출력), 늘 종료 0(레인·착지는 막지 않음)
# mainbusy <폴더>: 그 기계에서 그 폴더(아래)를 cwd로 쓰는 일하는 터미널 주소를 빈칸으로. hc ls 실패·꼴을 못 읽으면 종료 1.
# 일하는 터미널: status가 working·blocked, 또는 status가 없는데(셸에서 실행기로 띄운 대화형 지휘 — hc ls에 agent zsh·status 없음,
# 재감사 R-M1) hc get의 foreground pid가 터미널 pid와 다름(셸 위에 에이전트·빌드 등이 돎). 그 판정을 못 하면 일하는 중으로 친다
mainbusy() {
  mb_=$(hc ls --json 2>/dev/null </dev/null) || return 1
  case $mb_ in *'"terminals"'*) ;; *) return 1 ;; esac   # 성공했는데 꼴이 다르면 모름(재감사 R-L1)
  printf '%s\n' "$mb_" | jrec address state status cwd | while IFS=$tab read -r a s ss c; do
    case $a in "${on:+$on/}"*) a=${a#"${on:+$on/}"} ;; *) continue ;; esac
    case $a in */*) continue ;; esac                         # 다른 기계
    [ -n "$on" ] || [ "$a" != "${HC_TERMINAL_ID-}" ] || continue   # 부른 터미널 자신(앞당김을 청한 쪽)
    [ "$s" = live ] || continue
    case $c/ in "$1"/*) ;; *) continue ;; esac
    case $ss in
      working|blocked) ;;
      -) g_=$(hc get "${on:+$on/}$a" --json </dev/null 2>/dev/null)
         fp_=$(printf %s "$g_" | sed -n 's/.*"foreground" *: *{[^}]*"pid" *: *\([0-9][0-9]*\).*/\1/p')
         tp_=$(printf %s "$g_" | sed -n 's/.*"pid" *: *\([0-9][0-9]*\).*/\1/p')   # 맨 위 pid는 foreground 뒤라 마지막 것
         [ -z "$fp_" ] || [ "$fp_" != "$tp_" ] || continue ;;           # 셸만 떠 있음
      *) continue ;;
    esac
    printf '%s ' "${on:+$on/}$a"
  done
}
ffmain() {
  if ! fb=$(mainbusy "$1"); then fr="건너뜀: hc ls 실패(메인 폴더에서 일하는 세션을 모름)"
  elif [ -n "$fb" ]; then fr="건너뜀: 메인 폴더에서 일하는 세션 ${fb% }"
  else
  fr=$(x sh -c 'cd "$1" 2>/dev/null || { echo "건너뜀: 폴더 없음"; exit 1; }
    b=$2 gd=$(git rev-parse --absolute-git-dir 2>/dev/null) || { echo "건너뜀: git 폴더를 못 찾음"; exit 1; }
    [ "$(git rev-parse --is-bare-repository 2>/dev/null)" != true ] || { echo "건너뜀: 맨(bare) 저장소"; exit 1; }
    for f in MERGE_HEAD rebase-merge rebase-apply CHERRY_PICK_HEAD REVERT_HEAD; do
      [ ! -e "$gd/$f" ] || { echo "건너뜀: 진행 중인 merge/rebase($f)"; exit 1; }
    done
    h=$(git symbolic-ref -q --short HEAD)
    [ "$h" = "$b" ] || { echo "건너뜀: 기본 브랜치($b)가 아니다(${h:-분리된 HEAD})"; exit 1; }
    # 조회가 실패하면 깨끗함·표시 없음으로 읽지 않는다(아스트라 4차 중간 1 — 빈 출력 128이어도 merge했다)
    s=$(git status --porcelain --untracked-files=no 2>&1) || { echo "건너뜀: git status 실패(작업 트리를 모름)"; exit 1; }
    [ -z "$s" ] || { echo "건너뜀: 추적 파일에 커밋 안 된 변경"; exit 1; }
    l=$(git ls-files -v 2>/dev/null) || { echo "건너뜀: git ls-files 실패(색인 표시를 모름)"; exit 1; }
    ! printf "%s\n" "$l" | grep -q "^[a-zS]" || { echo "건너뜀: skip-worktree·assume-unchanged 표시 파일이 있다"; exit 1; }
    o=$(git rev-parse -q --verify "refs/remotes/origin/$b^{commit}") || { echo "건너뜀: origin/$b 없음"; exit 1; }
    a=$(git rev-parse --short HEAD) n=$(git rev-parse --short "$o")
    [ "$(git rev-parse HEAD)" != "$o" ] || { echo "최신 $a"; exit 0; }
    git merge-base --is-ancestor HEAD "$o" || { echo "건너뜀: 로컬 $b 에 origin/$b 에 없는 커밋이 있다"; exit 1; }
    e=$(git merge --ff-only --no-squash --no-overwrite-ignore -q "$o" 2>&1) || { echo "건너뜀: git이 거절 — $(printf %s "$e" | head -n 1)"; exit 1; }
    # --no-squash: 저장소 설정 mergeOptions=--squash면 HEAD는 그대로·색인만 바뀐 채 0이었다. 앞당겼다고 말하기 전에 HEAD를 본다
    # merge는 이미 성공했다 — HEAD를 못 읽으면 「그대로 둠」이 아니라 확인 불가로(아스트라 4차 중간 3)
    h2=$(git rev-parse -q --verify HEAD) || { echo "확인불가: merge는 성공했는데 최종 HEAD를 확인하지 못했다(git rev-parse 실패) — 폴더가 ${n}으로 갔을 수 있다"; exit 1; }
    # 다른 착지가 같은 메인 폴더를 그새 더 앞당겼으면(목표를 품은 HEAD) 그대로 둔 것이 아니다 — 실제 HEAD를 알린다(아스트라 5차 중간 1)
    if [ "$h2" != "$o" ]; then
      git merge-base --is-ancestor "$o" "$h2"; c=$?
      case $c in
        0) echo "앞당김 ${a}→$(git rev-parse --short "$h2" 2>/dev/null || echo "$h2") (목표 ${n}을 품음 — 다른 착지가 더 앞당김)"; exit 0 ;;
        1) echo "건너뜀: merge 뒤 HEAD가 origin/$b 를 품지 않는다(저장소 merge 설정?)"; exit 1 ;;
        *) echo "확인불가: merge는 성공했는데 최종 HEAD가 목표를 품는지 확인하지 못했다(git merge-base 종료 $c)"; exit 1 ;;
      esac
    fi
    echo "앞당김 ${a}→${n}"' hc-lane "$1" "$2" 2>/dev/null)
  fi
  case $fr in
    앞당김*|최신*) echo "hc-lane: 메인 폴더 $fr (${on:+$on:}$1)" ;;
    확인불가:*) echo "hc-lane: 경고 — 메인 폴더 ${fr#확인불가: } (${on:+$on:}$1)" >&2 ;;
    *) echo "hc-lane: 경고 — 메인 폴더 앞당김 ${fr:-건너뜀: 결과를 못 읽음} — 그대로 둠 (${on:+$on:}$1)" >&2 ;;
  esac
  return 0
}

# 출력하는 명령에 따옴표 없이 넣으므로 브랜치 이름은 이 글자만 받는다.
plain() { case $1 in '' | *[!ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789/_.-]*) return 1 ;; esac; }

# origin_base <저장소>: origin/HEAD가 가리키는 기본 브랜치, 없으면 main. origin이 없으면(로컬 저장소) 로컬 main, 없으면 master
origin_base() {
  if git -C "$1" remote get-url origin >/dev/null 2>&1; then
    b=$(git -C "$1" symbolic-ref --short -q refs/remotes/origin/HEAD 2>/dev/null); b=${b#origin/}
  elif ! git -C "$1" show-ref -q --verify refs/heads/main && git -C "$1" show-ref -q --verify refs/heads/master; then b=master
  else b=; fi
  echo "${b:-main}"
}

# land: 레인 작업 폴더에서 착지. 저장소의 .claude/herdr-land.sh가 있으면 그것에 맡기고, 없으면 같은 순서를 여기서 한다.
# 검증 스크립트가 없으면 push하지 않는다(옛 착지 줄의 「건너뜀」 뒤 push를 막는다). push는 HERDR_LAND=1로(pre-push 관문 통과).
# 착지 로그(2026-11-17 land-log — senior-web 착지 실패가 끝 15줄에 패키지 없는 FAIL만 남겨 어느 시험인지 몰랐다): 검증(또는
# herdr-land.sh) 출력을 화면 그대로 + 상태 폴더 land-logs/<레인>-<시각>-<pid>.log에. 작업 폴더 .qa가 아닌 까닭: clean이 무시
# 파일과 함께 지우고, gc는 .qa가 남은 작업 폴더를 안 지운다. 실패면 실패 줄(최대 40)·이번에 생긴 .qa/*/herdr-verify-* 폴더·로그
# 경로를 찍고, 레인 안($HC_LANE)이면 「중간」 사건에도 로그 경로를 싣는다
llog_init() {
  ld=$evdir/land-logs
  mkdir -p "$ld" 2>/dev/null || { llog=; return 0; }   # 로그를 못 남겨도 착지는 막지 않는다
  find "$ld" -name '*.log' -mtime +14 -delete 2>/dev/null   # ponytail: 14일 지난 것만 지움, 더 오래 둘 일이 생기면 설정으로
  ln_=${HC_LANE:-$(git symbolic-ref -q --short HEAD 2>/dev/null)}
  llog=$ld/$(printf %s "${ln_:-land}" | tr -c 'A-Za-z0-9._-' '-')-$(date +%Y%m%d-%H%M%S)-$$.log
  : >"$llog" 2>/dev/null || llog=
}
qadirs() { find .qa -mindepth 2 -maxdepth 2 -type d -name 'herdr-verify-*' 2>/dev/null | sort; }
# lrun <명령…>: 출력(오류 출력 포함)을 화면과 로그에. 종료 코드는 명령 것
lrun() {
  [ -n "$llog" ] || { "$@"; return; }
  printf '=== %s HEAD %s: %s\n' "$(date '+%F %T')" "$(git rev-parse --short HEAD 2>/dev/null)" "$*" >>"$llog"
  "$@" 2>&1 | tee -a "$llog"
  return "${PIPESTATUS[0]}"
}
lfail() {
  [ -n "$llog" ] || return 0
  qn=$(qadirs | grep -vxF -- "$qa0")
  echo "hc-lane land: 실패 줄(로그에서, 최대 40줄):" >&2
  { cat "$llog"
    printf '%s\n' "$qn" | while IFS= read -r d; do [ -z "$d" ] || find "$d" -type f -name '*.log' -exec cat {} + 2>/dev/null; done
  } | LC_ALL=C grep -E -- '--- FAIL:|^FAIL[[:space:]]+[^[:space:]]|panic:|\(fail\)|✗|✖|not ok' | awk '!s[$0]++' | head -n 40 >&2
  printf '%s\n' "$qn" | while IFS= read -r d; do [ -z "$d" ] || echo "hc-lane land: 검증 로그 폴더 $PWD/${d#./}" >&2; done
  echo "hc-lane land: 전체 로그 $llog" >&2
  [ -z "${HC_LANE-}" ] || (report_cmd 중간 - "land 실패(종료 $1) — 전체 로그 $llog") >/dev/null 2>&1 || true
}
mainwt() { git worktree list --porcelain | sed -n '1s/^worktree //p'; }   # 메인 폴더(첫 작업 트리)
rr_paths() { printf '%s\n' "$1" | sed -n 's/^CONFLICT ([^)]*): Merge conflict in //p'; }   # LC_ALL=C rebase 출력의 내용 충돌 경로
# rr_ok <rebase 출력>: 멈춘 rebase를 rerere가 다 풀었나(2026-11-19 lane-overlap-guard, 지휘 검토 ④ — 미병합이 비었다는 것만으로는
# 이어 가지 않는다). 커밋 적용 중 멈췄고(REBASE_HEAD), 충돌 줄이 모두 내용 충돌(지움·이름 바꿈 충돌은 rerere가 못 푼다)이며, 그 경로가
# 모두 「Staged '<경로>' using previous resolution」(autoUpdate) 또는 「Resolved '<경로>' …」(이 경로들은 git add 한다 — 2차 중간 7부터
# 저장소에 autoUpdate를 쓰지 않는다)이고, 미병합·rerere remaining이 비었을 때만 0. 훅 실패 등 다른 중단은 1(멈춤 그대로)
rr_ok() {
  git rev-parse -q --verify REBASE_HEAD >/dev/null || return 1
  # 「Resolved」로만 푼 경로는 작업 트리만 풀려 있다 — autoUpdate처럼 올려 둔다(일부만 풀렸어도 푼 것은 — 아래에서 멈추면 남은 것만 미병합)
  rv_=$(printf '%s\n' "$1" | sed -n "s/^Resolved '\\(.*\\)' using previous resolution\\.\$/\\1/p")
  [ -z "$rv_" ] || printf '%s\n' "$rv_" | while IFS= read -r p_; do git add -- "$p_" || exit 1; done || return 1
  rp_=$(rr_paths "$1")
  [ -n "$rp_" ] && [ "$(printf '%s\n' "$1" | grep -c '^CONFLICT ')" = "$(printf '%s\n' "$rp_" | grep -c .)" ] || return 1
  rs_=$(printf '%s\n' "$1" | sed -n -e "s/^Staged '\\(.*\\)' using previous resolution\\.\$/\\1/p" -e "s/^Resolved '\\(.*\\)' using previous resolution\\.\$/\\1/p")
  printf '%s\n' "$rp_" | RS_="$rs_" awk 'BEGIN { n = split(ENVIRON["RS_"], a, "\n"); for (i = 1; i <= n; i++) ok[a[i]] = 1 } !($0 in ok) { f = 1 } END { exit f }' || return 1
  # 두 확인 명령이 각각 성공했고 결과가 빌 때만(명령이 실패해 빈 출력이면 이어 가지 않는다 — 아스트라 감사 중간 1)
  u_=$(git diff --name-only --diff-filter=U) && [ -z "$u_" ] && u_=$(git rerere remaining) && [ -z "$u_" ]
}
# lerr <글>: 화면(오류 출력)과 착지 로그에(빈 글은 안 찍음)
lerr() { [ -n "$1" ] || return 0; printf '%s\n' "$1" >&2; [ -z "$llog" ] || printf '%s\n' "$1" >>"$llog"; }
# rb_ <기준>: land의 rebase 한 번 — rerere가 다 푼 충돌(rr_ok)은 이어 간다. 0 아니면 rebase는 멈춘 채(먼저 rebase·착지 반복 두 자리가
# 같이 쓴다 — land-stale-lows 감사 낮음 1, 한쪽만 고쳐 갈라지지 않게)
rb_() {
  ro=$(LC_ALL=C git rebase -q "$1" 2>&1); rr=$?
  lerr "$ro"
  while [ $rr != 0 ] && rr_ok "$ro"; do
    lerr "hc-lane land: rerere가 기록한 풀이로 충돌을 다 풀었다($(rr_paths "$ro" | paste -sd , -)) — rebase를 이어 간다"
    ro=$(LC_ALL=C GIT_EDITOR=true git rebase --continue 2>&1); rr=$?
    lerr "$ro"
  done
  return $rr
}
# ---- 병합기(2026-11-20 레인 land-merger-mvp — 사용자 결정 「중앙 병합기, 브랜치만 (추천)」 → 「병합기 최소 버전을 먼저 만들어서
# 전체 적용시키고 일단 사용부터」. 왜: 레인마다 fetch·rebase·검증·차례·push를 하니 rebase 충돌·차례 다툼·재검증으로 착지가 밀렸다.
# main에 쓰는 프로세스를 하나로 하면 그 다툼이 통째로 없어진다) ----
# 켜는 곳은 원격 하나: 원격 참조 refs/herdr/merger/<병합기 기계>(값은 아무 커밋)가 있으면 그 저장소의 hc-lane land는 제출(lane/<레인> +
# 큐 표식)로 바뀌고, 그 기계의 hc-lane merger가 큐를 하나씩 기존 착지(HERDR_MERGER=1 hc-lane land)로 넣는다. 지우면 다음 착지부터 예전
# 그대로. 개인 설정이 아닌 까닭: 워커 기계에는 개인 설정(herdr-tools-config)이 없어 레인이 도는 기계가 읽지 못한다. 원격 참조는
# ls-remote 한 번으로 모든 기계·레인(rebase 안 해도)에 바로 닿고, 착지의 fetch 횟수를 늘리지 않는다.
# 원격을 못 읽으면(켜졌는지 모름) 몇 번 다시 보고, 끝내 못 읽으면 직접 착지하지 않는다(아스트라 1차 높음 5 — 조회 한 번 실패로
# 큐·차례를 건너뛰어 main을 올렸다). 「조회 성공 + 표식 없음」일 때만 예전 착지 길
mgon() {  # 0 켜짐(병합기 기계를 mg_에), 1 꺼짐(조회 성공·표식 없음), 2 모름(조회 실패)
  # 원격 없는 저장소(get-url 종료 2)만 병합기가 없다. 그 밖의 실패는 모름(2차 높음 5 — 종료 128을 「꺼짐」으로 읽어 큐를 건너뛰었다)
  git remote get-url origin >/dev/null 2>&1
  case $? in 0) ;; 2) return 1 ;; *) echo "hc-lane land: 원격 설정(git remote get-url origin)을 읽지 못해 병합기가 켜졌는지 모른다 — 착지하지 않았다. 같은 줄을 다시" >&2; return 2 ;; esac
  for w_ in ${HC_LANE_MGON_WAITS:-1 2 4} -; do
    mgt_=$(git ls-remote origin 'refs/herdr/merger/*' 2>/dev/null) && break
    [ "$w_" = - ] && { echo "hc-lane land: 원격을 읽지 못해 병합기가 켜졌는지 모른다 — 착지하지 않았다. 같은 줄을 다시" >&2; return 2; }
    sleep "$w_"
  done
  mg_=$(printf '%s\n' "$mgt_" | awk 'NF == 2 { sub(/^refs\/herdr\/merger\//, "", $2); print $2; exit }')
  [ -n "$mg_" ]
}
# 레인 이름 = hc-fill 레인 이름 규칙(okname: a-z 0-9 _ - 32자) + git 가지 이름으로 쓸 수 있음(git check-ref-format --branch). 레인을 띄울
# 때(hc-lane이 그 이름으로 가지를 만든다)와 같은 두 검사다(3차 중간 13 — 「_audit」을 받는 쪽과 거절하는 쪽이 갈렸다, 2차 중간 13 — 「-audit」).
# 글자를 늘어놓는다: [a-z] 범위는 로캘에 따라 대문자도 받는다(시험 14에서 Bad_Name이 통과)
lname_ok() {
  case $1 in ''|*[!abcdefghijklmnopqrstuvwxyz0123456789_-]*) return 1 ;; esac
  [ ${#1} -le 32 ] && git check-ref-format --branch "$1" >/dev/null 2>&1
}

# ---- 레인 임대(2026-11-20 fill-dup-guard, 지휘 결정): 프로젝트 원격 ref refs/hc-lanes/<레인>(빈 트리 커밋, 제목 「hc-lane lease <레인>
# <기계> <UTC 시각>」) 하나가 그 이름의 실행 하나다. 띄울 때(hc run 직전) 잡고, land 성공(main과 같은 원자 push)·clean에서 놓는다.
# lease_core <take|show|drop> <저장소> <레인> [<기계(take)>|<기대값(drop)>] — 원격 기계에서도 그대로 돌게 이 함수만으로(declare -f).
#   take: 0 잡음(찍는 값 = 임대 커밋 — push 응답을 잃었어도 원격 값이 이것이면) · 3 누가 쥐고 있다(「값 제목」) ·
#         4 원격(origin) 없음이 확인됨(remote get-url 종료 2 — 「no-origin」을 찍는다. 부르는 쪽은 그 글자도 본다: hc exec의 종료 4와 가름) ·
#         1 못 잡음·조회 실패(저장소 아님 포함) · 5 모름(push 실패 뒤 원격 조회도 실패)
#   show: 「값 제목」 또는 「-」. 0 · 1 조회 실패 · 4 원격 없음
#   drop: 기대값을 주면 그 값일 때만(다르면 3, 지금 값을 찍음), 안 주면 지금 값을 lease로 지움. 0 지움(없음·원격 없음 포함) · 1 실패
lease_core() {
  lc_op=$1 lc_r=$2 lc_n=$3 lc_x=${4-} lc_ref=refs/hc-lanes/$3
  lc_g() { GIT_AUTHOR_NAME=hc-lane GIT_AUTHOR_EMAIL=hc-lane@localhost GIT_COMMITTER_NAME=hc-lane GIT_COMMITTER_EMAIL=hc-lane@localhost git -C "$lc_r" "$@"; }
  lc_cur() { lc_l=$(lc_g ls-remote origin "$lc_ref" </dev/null) || return 1; printf '%s' "${lc_l%%[[:space:]]*}"; }
  lc_sub() {  # 그 임대 커밋의 제목(없으면 받아 온다 — FETCH_HEAD는 안 씀)
    lc_g cat-file -e "$1^{commit}" 2>/dev/null || lc_g fetch -q --no-tags --no-write-fetch-head origin "$lc_ref" </dev/null 2>/dev/null
    lc_g log -1 --format=%s "$1" 2>/dev/null || echo '?'
  }
  lc_g remote get-url origin >/dev/null 2>&1; lc_o=$?
  if [ $lc_o = 2 ]; then echo no-origin; [ "$lc_op" != drop ] || return 0; return 4; fi
  [ $lc_o = 0 ] || return 1
  case $lc_op in
    take)
      lc_t=$(lc_g hash-object -t tree -w /dev/null) &&
        # 본문의 고유값: 같은 기계·같은 초·같은 이름이면 커밋 해시가 같아져 남의 임대를 자기 것으로 읽는다(응답 유실 판정)
        lc_c=$(printf 'hc-lane lease %s %s %s\n\n%s-%s-%s\n' "$lc_n" "${lc_x:--}" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$$" "$RANDOM" "$RANDOM" | lc_g commit-tree "$lc_t") || return 1
      lc_g push -q --porcelain origin "--force-with-lease=$lc_ref:" "$lc_c:$lc_ref" </dev/null >/dev/null 2>&1 && { echo "$lc_c"; return 0; }
      lc_v=$(lc_cur) || return 5   # push 실패: 원격 값으로 판정(응답만 잃었을 수 있다)
      [ "$lc_v" != "$lc_c" ] || { echo "$lc_c"; return 0; }
      [ -z "$lc_v" ] || { echo "$lc_v $(lc_sub "$lc_v")"; return 3; }
      return 1 ;;
    show) lc_v=$(lc_cur) || return 1; if [ -n "$lc_v" ]; then echo "$lc_v $(lc_sub "$lc_v")"; else echo -; fi; return 0 ;;
    drop)
      lc_v=$(lc_cur) || return 1
      [ -n "$lc_v" ] || return 0
      [ -z "$lc_x" ] || [ "$lc_x" = "$lc_v" ] || { echo "$lc_v"; return 3; }
      lc_g push -q --porcelain origin "--force-with-lease=$lc_ref:$lc_v" ":$lc_ref" </dev/null >/dev/null 2>&1 && return 0
      lc_v=$(lc_cur) && [ -z "$lc_v" ] && return 0   # 지운 응답만 잃었는지
      return 1 ;;
  esac
  return 1
}
# lease_x <take|show|drop> <레인> [<기계|기대값>]: 대상 기계($on)의 $repo에서 — 원격은 로그인 셸(zx: git 자격 도우미 PATH)로
lease_x() { zx "$repo" bash -c "$(declare -f lease_core); lease_core \"\$@\"" hc-lane-lease "$1" "$repo" "$2" "${3-}"; }
# lease_cmd: hc-lane lease show|drop [--on <기계>] --repo <저장소> --name <레인> — 임대가 남아 다시 못 띄우면(레인이 죽음·지휘가 rm) 손으로
lease_cmd() {
  lo_=$(lease_x "$act" "$name"); r_=$?
  case $act:$r_ in
    show:0) printf '%s\n' "$lo_" ;;
    show:4) [ "$lo_" = no-origin ] && echo "- (원격 없음 — 임대 없음)" || { echo "hc-lane lease: 조회하지 못했다(종료 4)" >&2; return 1; } ;;
    drop:0) echo "hc-lane lease: 놓음 refs/hc-lanes/$name" ;;
    *) echo "hc-lane lease: ${act}하지 못했다(git ls-remote·push origin, 종료 $r_): ${on:+$on:}$repo" >&2; return 1 ;;
  esac
}
# ---- 착지 표식(2026-11-20 fill-dup-guard, 지휘 결정 「착지 증거를 하나로」 — carousel-save-race가 선행 main-split의 착지를 커밋 제목
# 머리로 못 알아봐(「feat(frontend): main-split-1 …」) 먼저 띄움 기한을 넘겼다): land는 main과 같은 --atomic push에
# refs/hc-landed/<레인>/<UTC 초>(값 = 착지한 커밋)과 그 레인 임대 지우기(HC_LANE_LEASE 값 lease)를 싣는다. 기다리는 쪽(hc-fill 선행
# 판정·먼저 띄움 기다림 줄)은 그 선행 줄을 넣은 뒤의 표식 sha가 origin/<기본>의 조상인지 본다. 표식은 지우지 않는다(작고, 지우는 쪽 경합)
# lmname: 이 레인 이름(레인 환경 HC_LANE, 없으면 가지 이름) — 표식·임대 ref에 쓸 수 있을 때만
lmname() {
  lmn_=${HC_LANE:-$(git branch --show-current 2>/dev/null)}
  case $lmn_ in '' | *[!abcdefghijklmnopqrstuvwxyz0123456789_-]*) return 1 ;; esac
  echo "$lmn_"
}
# land_after <기본> <표식 ref>: main push 뒤 — 표식이 그 push에 못 실렸으면 따로(그 커밋이 origin/<기본>에 들어갔을 때만), 임대가 남았으면
# 놓는다. 못 하면 알리기만(착지는 됨 — 표식이 없으면 기다리는 쪽이 커밋 제목 규칙으로만 풀려 더 기다릴 뿐, 남은 임대는 clean·손으로)
land_after() {
  if [ -n "${2-}" ]; then
    la_=$(git ls-remote origin "$2" </dev/null 2>/dev/null)
    if [ -z "$la_" ] && ! { git fetch -q origin </dev/null 2>/dev/null && git merge-base --is-ancestor HEAD "refs/remotes/origin/$1" &&
        git push -q origin "HEAD:$2" </dev/null 2>/dev/null; }; then
      echo "hc-lane land: 착지 표식 $2 를 두지 못했다(착지는 됨 — 기다리는 레인은 커밋 제목 규칙으로만 풀린다)" >&2
    fi
  fi
  [ -n "${HC_LANE_LEASE-}" ] && lmn_=$(lmname) || return 0
  ld_=$(git ls-remote origin "refs/hc-lanes/$lmn_" </dev/null 2>/dev/null) || { echo "hc-lane land: 임대를 확인하지 못했다(착지는 됨) — hc-lane clean이 놓는다" >&2; return 0; }
  [ "${ld_%%[[:space:]]*}" = "$HC_LANE_LEASE" ] || return 0   # 이미 놓았거나 다른 실행의 것
  git push -q origin "--force-with-lease=refs/hc-lanes/$lmn_:$HC_LANE_LEASE" ":refs/hc-lanes/$lmn_" </dev/null 2>/dev/null &&
    echo "hc-lane land: 임대 놓음 refs/hc-lanes/$lmn_" || echo "hc-lane land: 임대를 놓지 못했다(착지는 됨) — hc-lane clean이 놓는다" >&2
}
# tclean <때>: 색인·추적 파일이 HEAD와 같은가. git status가 실패하면 「깨끗함」으로 읽지 않고 멈춘다(운영 모델 — git 실패를 성공·없음으로 안 읽음)
tclean() {
  st_=$(git status --porcelain --untracked-files=no) || die "land: $1 git status 실패 — 작업 트리를 모른다, 보내지 않았다. 같은 줄을 다시"
  [ -z "$st_" ]
}
# park [<한 줄>]: 먼저 띄운 레인이 준비를 끝내고 선행만 기다릴 때 자리를 쥐고 놀지 않게(2026-11-20 사용자 「노는 워크트리 없이」).
# HEAD를 원격 refs/heads/lane/<레인>(지금 값에 lease)에, 이 레인 임대 지우기(HC_LANE_LEASE 값일 때만)와 한 번의 --atomic push로 —
# 가지가 원격에 없는데 임대만 풀리는 일이 없게. 결과는 원격을 다시 읽어 본다(push 응답 유실). 그다음 「주차」 사건(hc-fill이 보고
# 그 줄을 「주차」로 되돌리고, 선행 착지 뒤 hc-lane --resume으로 그 가지에서 다시 띄운다). 원격 없는 저장소는 주차 못 함(종료 3)
park_cmd() {
  top=$(git rev-parse --show-toplevel 2>/dev/null) && cd "$top" || die "park: 레인 작업 폴더 안에서 부른다: $PWD"
  ln_=$(lmname) || die "park: 레인 이름을 모른다(HC_LANE 또는 레인 가지)"
  git remote get-url origin >/dev/null 2>&1
  case $? in 0) ;; 2) echo "hc-lane park: 원격(origin)이 없어 주차하지 못한다 — 지시문의 기다림 블록으로" >&2; exit 3 ;; *) die "park: git remote get-url 실패" ;; esac
  for f_ in rebase-merge rebase-apply MERGE_HEAD CHERRY_PICK_HEAD; do
    gp_=$(git rev-parse --git-path "$f_") && [ -n "$gp_" ] || die "park: git rev-parse --git-path 실패 — 진행 중인 rebase·merge가 있는지 모른다"
    [ ! -e "$gp_" ] || die "park: 진행 중인 rebase·merge가 있다($f_). 마친 뒤 다시"
  done
  st_=$(git status --porcelain --untracked-files=normal) || die "park: git status 실패 — 작업 트리를 모른다"
  [ -z "$st_" ] || die "park: 커밋 안 된 변경·새 파일이 있다(git status) — 준비를 커밋한 뒤 다시"
  sh_=$(git rev-parse -q --verify HEAD) || die "park: HEAD를 읽지 못했다"
  pr_=refs/heads/lane/$ln_ pl_=refs/hc-lanes/$ln_
  rs_=$(git ls-remote origin "$pr_" "$pl_" </dev/null) || die "park: 원격을 읽지 못했다(git ls-remote) — 같은 줄을 다시"
  cur_=$(printf '%s\n' "$rs_" | awk -v r="$pr_" '$2 == r { print $1 }')
  lv_=$(printf '%s\n' "$rs_" | awk -v r="$pl_" '$2 == r { print $1 }')
  set -- "$sh_:$pr_" "--force-with-lease=$pr_:$cur_"
  dl_=; if [ -n "${HC_LANE_LEASE-}" ] && [ "$lv_" = "$HC_LANE_LEASE" ]; then dl_=1; set -- "$@" ":$pl_" "--force-with-lease=$pl_:$lv_"; fi
  [ "$cur_" = "$sh_" ] && [ -z "$dl_" ] || po_=$(git push -q --atomic --porcelain origin "$@" </dev/null 2>&1)
  rb_=$(git ls-remote origin "$pr_" "$pl_" </dev/null) || die "park: 원격을 다시 읽지 못했다 — 같은 줄을 다시(주차는 같은 값으로 모인다)"
  [ "$(printf '%s\n' "$rb_" | awk -v r="$pr_" '$2 == r { print $1 }')" = "$sh_" ] ||
    { printf '%s\n' "${po_-}" >&2; die "park: push 실패 — lane/$ln_ 이 이 커밋이 아니다. 같은 줄을 다시"; }
  lk_="임대 없음"; [ -z "${HC_LANE_LEASE-}" ] || lk_="임대 남음(다른 값) — hc-lane clean이 놓는다"
  if [ -n "$dl_" ]; then
    [ -z "$(printf '%s\n' "$rb_" | awk -v r="$pl_" '$2 == r')" ] && lk_="임대 놓음" || lk_="임대를 놓지 못했다 — hc-lane clean이 놓는다"
  fi
  (report_cmd 주차 "$sh_" "lane/$ln_ — 선행 착지 뒤 hc-fill이 이 가지에서 다시 띄운다${pmsg:+ · $pmsg}" --name "$ln_") >/dev/null ||
    die "park: 주차 사건을 쓰지 못했다(가지는 lane/$ln_ 에 있다) — 같은 줄을 다시"
  echo "hc-lane park: 주차 $(git rev-parse --short "$sh_") → lane/$ln_ ($lk_). 다음: 착지·정리 절차의 hc-lane clean 줄 → 턴을 끝낸다"
}
land_cmd() {
  top=$(git rev-parse --show-toplevel 2>/dev/null) && cd "$top" || die "land: git 작업 폴더 안에서 부른다(레인 워크트리): $PWD"
  base=${lbase:-$(origin_base .)}
  plain "$base" || die "land: 기본 브랜치 이름에 쓸 수 없는 글자가 있다: $base"
  # 병합기가 켜진 저장소(원격 refs/herdr/merger/<기계>)면 착지 대신 제출 — 병합기 안에서 부른 착지(HERDR_MERGER=1)는 아래 그대로.
  # 켜졌는지 모르면 착지하지 않는다(종료 75 — 같은 줄을 다시). 이 분기만 병합기 몫이고 아래 본문은 착지 안전(land-safety) 몫이다
  # (2026-11-20 t68과 정한 경계 — 나중에 착지하는 쪽이 rebase로 맞춘다)
  if [ "${HERDR_MERGER-}" != 1 ]; then
    mgon; case $? in 0) submit_cmd ;; 2) exit 75 ;; esac
  fi
  # 이 착지의 인자는 이 착지만 쓴다 — 검증 자식(저장소 시험이 다시 부르는 hc-lane land 등)에 물려주지 않는다(2차 높음 3: 병합기가 넘긴
  # 큐 삭제 인자가 herdr-tools 자체 시험의 가짜 원격 push에 실려 거절됐다). 착지 스크립트에는 이름을 붙여 넘긴다
  px_=${HERDR_LAND_PUSH_EXTRA-} mgx_=${HERDR_MERGER-}
  unset HERDR_LAND_PUSH_EXTRA HERDR_MERGER
  # 착지 로그는 첫 fetch·rebase 전에 연다 — 먼저 rebase·rebase 충돌(종료 3)도 로그에 남게(land-stale-lows 감사 낮음 3)
  qa0=$(qadirs); llog_init
  # 낡은 착지 스크립트(2026-11-20 land-stale-script — 한 레인이 문서 조각 접기 전 판 herdr-land.sh로 착지해 조각이
  # main에 남았다. bash는 rebase 전에 읽은 옛 판을 끝까지 돈다). fetch는 스크립트가 없는 가지도 한다(<기본>에 새로 생긴 것을 보려고): 가지가 그 스크립트를 스스로 바꾸지 않았는데(HEAD 판 = 갈래점 판)
  # 갈래점 판이 <기본> 판과 다르면(새로 생기거나 지워진 것 포함) 먼저 rebase해 <기본> 판으로 이어 간다. 바꾼 가지는 제 판 그대로
  lf_=.claude/herdr-land.sh
  if git remote get-url origin >/dev/null 2>&1; then ob_=refs/remotes/origin/$base obl_=origin/$base; fetchr git fetch -q origin || die "land: git fetch 실패"
  else ob_=refs/heads/$base obl_="로컬 $base"; fi
  # 병합기 안 착지: 기본 브랜치에 전용 착지 스크립트가 있으면 그 기본 브랜치 판(방금 받은 것)을 꺼내 맡긴다 — 제출 트리 속 판(레인이
  # 고쳤을 수 있음)은 실행하지 않는다(2단계 2026-11-20 지휘 t1cj). 스크립트는 자기 자리($0)를 쓰지 않는 것을 전제로 임시 파일에서 돈다
  if [ "$mgx_" = 1 ] && git cat-file -e "$ob_:$lf_" 2>/dev/null; then
    mls_=$(mktemp "${TMPDIR:-/tmp}/hc-merger-land.XXXXXX") && git show "$ob_:$lf_" >"$mls_" || die "land: 기본 브랜치의 $lf_ 를 꺼내지 못했다"
    echo "hc-lane land: 병합기 — $obl_ 판 $lf_ 에 맡긴다"
    lrun env HERDR_LAND=1 ${px_:+HERDR_LAND_PUSH_EXTRA="$px_"} bash "$mls_" "$base"; rc=$?
    rm -f "$mls_"
    # 내장 길의 병합기 안 착지와 같게: push는 병합기 인자 그대로, 착지 표식 refs/hc-landed/<레인>/<UTC 초>는 착지 뒤 따로(fill-dup-guard)
    if [ $rc = 0 ]; then lmr_=; ! lmn_=$(lmname) || lmr_=refs/hc-landed/$lmn_/$(date -u +%s); land_after "$base" "$lmr_"; else lfail $rc; fi
    exit $rc
  fi
  lb_() { git rev-parse -q --verify "$1:$lf_" 2>/dev/null; }   # 그 판의 스크립트 blob(없으면 빈 값)
  if mb_=$(git merge-base HEAD "$ob_" 2>/dev/null) && [ "$(lb_ HEAD)" = "$(lb_ "$mb_")" ] && [ "$(lb_ "$mb_")" != "$(lb_ "$ob_")" ]; then
    lerr "▸ hc-lane land: 가지의 $lf_ 가 $obl_ 보다 낡음 → 먼저 rebase"
    [ -z "$(git status --porcelain --untracked-files=no)" ] || die "land: 커밋 안 된 변경이나 멈춘 rebase가 있다(git status). 커밋하거나 rebase를 마친 뒤 같은 줄을 다시"
    rb_ "$ob_" || { lerr "hc-lane land: rebase 충돌 — 양쪽 내용을 살려 풀고 git add → git rebase --continue 한 뒤 착지 줄(hc-lane land $base)을 다시"; exit 3; }
  fi
  # 병합기 안 착지: 제출·기준에 있던 스크립트는 병합기가 이미 반송하니, 여기 있는 것은 위 rebase가 들인 것 — 맡기지 않고 7(감사 7c010d9 중간 1)
  [ "$mgx_" != 1 ] || [ ! -f .claude/herdr-land.sh ] ||
    { echo "hc-lane land: 기본 브랜치에 전용 착지 스크립트(.claude/herdr-land.sh)가 생겼다 — 병합기 MVP-1은 그 스크립트에 맡기지 않는다" >&2; exit 7; }
  if [ -f .claude/herdr-land.sh ]; then
    # 저장소 착지 스크립트에는 먼저 맡긴다(원격 조회 오류가 스크립트 착지를 막지 않게 — 아스트라 3차 중간 2). 원격 조회는 착지
    # 성공 직후에 새로 해 메인 폴더 앞당김에만 쓰고(실행 전 결과를 쓰지 않는다 — 4차 중간 2), 못 읽었으면 앞당기지 않고 경고만
    echo "hc-lane land: .claude/herdr-land.sh $base 에 맡긴다"
    # 표식(과 임대 지우기)은 HERDR_LAND_PUSH_EXTRA(공백으로 나눈 refspec — 병합기가 넘긴 인자 뒤에)로 넘겨 그 스크립트가 main push에
    # --atomic으로 싣게 한다 — 임대 지우기는 원격 임대가 이 레인 값일 때만 넣는다(없는 ref 지우기로 원자 push가 실패하지 않게). 못 실었으면 land_after가 따로
    lmr_= lpx_=; if lmn_=$(lmname) && git remote get-url origin >/dev/null 2>&1; then
      lmr_=refs/hc-landed/$lmn_/$(date -u +%s) lpx_="HEAD:$lmr_"
      if [ -n "${HC_LANE_LEASE-}" ]; then lh_=$(git ls-remote origin "refs/hc-lanes/$lmn_" </dev/null 2>/dev/null) && [ "${lh_%%[[:space:]]*}" = "$HC_LANE_LEASE" ] && lpx_="$lpx_ :refs/hc-lanes/$lmn_"; fi
    fi
    ax_="${px_:+$px_${lpx_:+ }}$lpx_"
    lp_=${HERDR_LAND_PRIORITY-}; case $lp_ in '' | *[!0-9]*) lp_=0 ;; esac   # 착지 줄 우선(README 계약): 0 이상 정수, 아니면 0
    lrun env HERDR_LAND=1 HERDR_LAND_PRIORITY="$lp_" ${ax_:+HERDR_LAND_PUSH_EXTRA="$ax_"} bash .claude/herdr-land.sh "$base"; rc=$?
    if [ $rc = 0 ]; then
      git remote get-url origin >/dev/null 2>&1; gu_=$?
      case $gu_ in
        0) ffmain "$(mainwt)" "$base"; land_after "$base" "$lmr_" ;;
        2) ;;
        *) echo "hc-lane: 경고 — 메인 폴더 앞당김 건너뜀: 원격 origin을 읽지 못했다(git remote get-url 종료 $gu_) — 그대로 둠(착지 표식·임대 놓기도 건너뜀)" >&2 ;;
      esac
    else lfail $rc; fi
    exit $rc
  fi
  # 원격 없음(git remote get-url 종료 2)과 조회 실패를 가른다 — 실패를 없음으로 읽으면 원격 저장소인데 로컬 <기본>을 올린다
  git remote get-url origin >/dev/null 2>&1; gu_=$?
  case $gu_ in 0) remote=1 ;; 2) remote=0 ;; *) die "land: 원격 origin을 읽지 못했다(git remote get-url 실패) — 원격 없음으로 읽지 않는다. 같은 줄을 다시" ;; esac
  # push 주소가 여럿(pushurl·url 여러 줄)이면 한 곳만 받아도 fetch로 본 첫 곳으로 착지를 정하게 된다 — 지원하지 않고 막는다(아스트라 6차 높음 1)
  if [ $remote = 1 ]; then
    pu_=$(git remote get-url --push --all origin) || die "land: origin의 push 주소를 읽지 못했다(git remote get-url --push --all 실패) — 모르면 착지하지 않는다. 같은 줄을 다시"
    [ "$(printf '%s\n' "$pu_" | grep -c .)" = 1 ] ||
      die "land: origin의 push 주소가 하나가 아니다 — 여러 push 주소(remote.origin.pushurl·url 여러 줄)는 지원하지 않는다. 하나로 줄인 뒤 같은 줄을 다시"
    # 브랜치별 조건 설정(includeIf onbranch)은 HEAD 상태(검증이 분리만 해도)에 따라 push 주소 등을 바꾼다 — 지원하지 않고 막는다
    # (아스트라 7차 높음, 지휘 결정). 조회 실패도 막는다(종료 1 = 없음만 통과)
    git config --get-regexp '^includeif\.onbranch:' >/dev/null 2>&1; ic_=$?
    case $ic_ in
      1) ;;
      0) die "land: 브랜치별 조건 설정(includeIf onbranch)은 지원하지 않는다 — push 주소가 HEAD 상태에 따라 바뀔 수 있다. 그 설정을 뺀 뒤 같은 줄을 다시" ;;
      *) die "land: git 설정을 읽지 못했다(git config --get-regexp 종료 $ic_) — 브랜치별 조건 설정이 있는지 몰라 착지하지 않는다. 같은 줄을 다시" ;;
    esac
  fi
  # origin이 없는 저장소(맥의 game·hwanhon 등)는 로컬 <기본>이 기준이고 착지는 그것을 fast-forward로 올리는 것이다
  if [ $remote = 1 ]; then bref=refs/remotes/origin/$base blab=origin/$base
  else bref=refs/heads/$base blab="로컬 $base"; fi
  tries=${HERDR_LAND_TRIES:-5}
  case $tries in ''|*[!0-9]*|0) tries=5 ;; esac
  i=0
  while :; do
    i=$((i + 1))
    tclean "시작 때" || die "land: 커밋 안 된 변경이나 멈춘 rebase가 있다(git status). 커밋하거나 rebase를 마친 뒤 같은 줄을 다시"
    [ $remote = 0 ] || fetchr git fetch -q origin || die "land: git fetch 실패"
    # 기준 SHA를 이 바퀴에 고정해 그 SHA로 rebase(rebase 뒤 참조가 움직여도 기준이 바뀌지 않게)
    bs_=$(git rev-parse -q --verify "$bref^{commit}") || die "land: $blab 가 없다"
    rb_ "$bs_" || { lerr "hc-lane land: rebase 충돌. 양쪽 내용을 살려 풀고 git add → git rebase --continue 뒤 같은 줄(hc-lane land $base)을 다시"; exit 3; }
    # 병합기 안 착지: rebase한 기본 브랜치에 전용 착지 스크립트가 생겼으면 내장 길로 넣지 않는다 — 종료 7, 병합기가 반송(3차 높음 7)
    [ "$mgx_" != 1 ] || [ ! -f .claude/herdr-land.sh ] ||
      { echo "hc-lane land: 기본 브랜치에 전용 착지 스크립트(.claude/herdr-land.sh)가 생겼다 — 병합기 MVP-1은 내장 길로 넣지 않는다" >&2; exit 7; }
    # 보내는 것은 검증한 커밋 그 자체(2026-11-20 land-metrics — 아스트라 감사): 후보 SHA를 고정하고, 그 값이 이 저장소의 커밋인지
    # 본다(조회가 빈 값이면 「:refs/heads/<기본>」 push — 기본 브랜치 지우기). 후보가 기준 그대로면 보낼 것이 없다
    cand=$(git rev-parse -q --verify HEAD) && [ -n "$cand" ] ||
      die "land: rebase 뒤 HEAD를 읽지 못했다(git 실패) — 아무것도 보내지 않았다. 같은 줄을 다시"
    git cat-file -e "$cand^{commit}" 2>/dev/null ||
      die "land: rebase 뒤 HEAD 값($cand)이 이 저장소의 커밋이 아니다(git 조회 이상) — 아무것도 보내지 않았다. 같은 줄을 다시"
    # rebase 뒤 작업 트리 확인은 「이미 들어가 있다」보다 먼저(아스트라 3차 중간 3 — rebase 훅이 추적 파일을 바꿔도 0으로 끝나 뒤
    # clean이 그 변경에서 멈췄다)
    tclean "rebase 뒤" || die "land: rebase 뒤 색인·추적 파일이 후보와 다르다(git status) — 보내지 않았다. 같은 줄을 다시"
    # 함께 실을 것(HERDR_LAND_PUSH_EXTRA — 병합기 큐 표식 삭제·착지 표식)이 있으면 그것을 실으러 아래 push까지 간다(예전 그대로)
    if [ "$cand" = "$bs_" ] && [ -z "$px_" ]; then
      echo "hc-lane land: 이미 들어가 있다 ${cand:0:9} = $blab (보낼 것 없음)"
      [ $remote = 0 ] || ffmain "$(mainwt)" "$base"
      return 0
    fi
    [ -f .claude/herdr-verify.sh ] ||
      die "land: .claude/herdr-verify.sh 가 없어 push하지 않았다(검증 없는 착지는 없다). 저장소에 검증 스크립트(또는 .claude/herdr-land.sh)를 두거나 지휘에 알린다"
    lrun bash .claude/herdr-verify.sh || { lfail $?; die "land: 검증 실패, push하지 않았다. 고쳐 커밋한 뒤 같은 줄을 다시"; }
    # 검증한 그 커밋·그 파일만 보낸다: 검증하는 동안 HEAD가 바뀌었거나 검증이 추적 파일·색인을 고쳤으면 보내지 않는다
    h_=$(git rev-parse -q --verify HEAD) || die "land: 검증 뒤 HEAD를 읽지 못했다(git 실패) — 보내지 않았다. 같은 줄을 다시"
    [ "$h_" = "$cand" ] ||
      die "land: 검증하는 동안 HEAD가 바뀌었다(검증한 ${cand:0:9} → 지금 ${h_:0:9}) — 검증하지 않은 커밋은 보내지 않았다. 같은 줄을 다시"
    tclean "검증 뒤" ||
      die "land: 검증이 추적 파일·색인을 바꿨다(git status) — 검증한 파일과 보낼 커밋이 달라 보내지 않았다. 검증 스크립트를 고치거나 바뀐 것을 커밋한 뒤 같은 줄을 다시"
    if [ $remote = 1 ]; then
      # 검증 뒤 push 주소를 다시 읽어 시작 때와 다르면(검증이 설정을 바꿈) 보내지 않는다(아스트라 7차 높음). 진단에 주소는 찍지 않는다
      # (인증정보가 든 주소일 수 있다 — 8차 높음 3). push는 원격 이름으로: 주소를 넘기면 pushInsteadOf가 다시 적용되고 추적 ref가 안 바뀐다(8차)
      pu2_=$(git remote get-url --push --all origin) || die "land: 검증 뒤 origin의 push 주소를 읽지 못했다 — 보내지 않았다. 같은 줄을 다시"
      [ "$pu2_" = "$pu_" ] ||
        die "land: 검증하는 동안 origin의 push 주소가 바뀌었다 — 보내지 않았다. 설정을 확인한 뒤 같은 줄을 다시"
      # 함께 실을 것 HERDR_LAND_PUSH_EXTRA = 공백으로 나눈 git push 인자(refspec — 왼쪽 HEAD는 기본 브랜치에 넣는 그 커밋(= 후보) — 와
      # --force-with-lease=<참조>:<SHA>). 있으면 기본 브랜치와 한 번의 --atomic push(전부 아니면 전무). 2026-11-20 지휘 t1cj — 착지 표식
      # (fill-dup-guard 「HEAD:refs/hc-landed/<레인>/<UTC 초>」 — 내장 길은 아래 mx_로 직접 싣는다)과 병합기 큐 표식 삭제(「:<큐> --force-with-lease=<큐>:<SHA>」, 1차 높음 4·구조 B)가
      # 한 이름·한 꼴. lease 걸린 참조가 바뀌었으면(재제출) 기본 브랜치를 올리지 않고 종료 6
      pa_=("$cand:refs/heads/$base") ex_=
      for e_ in $px_; do pa_+=("$e_"); ex_=--atomic; done
      # 착지 표식 refs/hc-landed/<레인>/<UTC 초>와 이 레인 임대 지우기(HC_LANE_LEASE 값 lease)도 같은 --atomic push에(fill-dup-guard).
      # 표식·임대 쪽으로 함께 실패했으면(임대를 손으로 놓았음 등) 검증을 다시 돌지 않고 기본 브랜치 + 병합기 인자만 한 번 더(병합기 lease가
      # 바뀌었으면 이것도 실패해 아래 종료 6) — 표식·임대는 land_after가 따로. 병합기 안 착지(HERDR_MERGER=1)는 push를 병합기 판
      # 그대로 둔다(전송 끊김 75·응답 유실 판정이 그 인자에 묶임) — 표식은 착지 뒤 land_after가 따로
      lmr_= mx_=(); if lmn_=$(lmname); then
        lmr_=refs/hc-landed/$lmn_/$(date -u +%s)
        if [ "$mgx_" != 1 ]; then
          mx_=("$cand:$lmr_") ex_=--atomic
          [ -z "${HC_LANE_LEASE-}" ] || mx_+=("--force-with-lease=refs/hc-lanes/$lmn_:$HC_LANE_LEASE" ":refs/hc-lanes/$lmn_")
        fi
      fi
      if HERDR_LAND=1 git push $ex_ origin "${pa_[@]}" ${mx_[0]+"${mx_[@]}"} ||
        { [ -n "${mx_[0]-}" ] && HERDR_LAND=1 git push ${pa_[1]+--atomic} origin "${pa_[@]}"; }; then
        echo "hc-lane land: 착지 ${cand:0:9} → origin/$base${lmr_:+ (착지 표식 $lmr_)}"
        ffmain "$(mainwt)" "$base"
        land_after "$base" "$lmr_"
        return 0
      fi
      # push 응답은 사라질 수 있다(운영 모델): 다시 보고 후보가 원격 기본에 들어가 있으면 레인 일은 끝. 다시 못 보거나 견주지 못하면
      # (merge-base 0·1 밖) 착지·거절로 정하지 않고 「모름」으로 멈춘다 — 다시 하는 것은 사람
      # ls-remote는 이름 끝도 맞춘다(refs/archive/refs/heads/<기본> 등) — 둘째 칸이 정확히 refs/heads/<기본>인 줄 하나만 받고, 없거나
      # 둘이면 모름(아스트라 8차 높음 2). fetch로 그 커밋과 추적 ref(메인 폴더 앞당김이 읽는 것)를 맞춘 뒤 견준다
      rl_=$(git ls-remote origin "refs/heads/$base") ||
        die "land: push 결과 모름 — push가 실패로 돌아왔고 다시 볼 git ls-remote도 실패했다. 원격 $base 에 ${cand:0:9} 가 들어갔는지 확인한 뒤 같은 줄을 다시"
      rs_=$(printf '%s\n' "$rl_" | R="refs/heads/$base" awk '$2 == ENVIRON["R"] { print $1; n++ } END { exit n != 1 }') ||
        die "land: push 결과 모름 — git ls-remote에 refs/heads/$base 줄이 하나가 아니다. 들어갔는지 확인한 뒤 같은 줄을 다시"
      fetchr git fetch -q origin && git cat-file -e "$rs_^{commit}" 2>/dev/null ||
        die "land: push 결과 모름 — 원격 $base(${rs_:0:9})를 가져오지 못했다(git fetch 실패). 들어갔는지 확인한 뒤 같은 줄을 다시"
      git merge-base --is-ancestor "$cand" "$rs_"; ia_=$?
      if [ $ia_ = 0 ]; then
        echo "hc-lane land: push가 실패로 돌아왔지만 원격 $base 에 ${cand:0:9} 가 들어가 있다(응답만 사라졌거나 같은 것이 먼저 들어감) — 착지 끝"
        ffmain "$(mainwt)" "$base"
        land_after "$base" "$lmr_"
        return 0
      fi
      [ $ia_ = 1 ] || die "land: push 결과 모름 — 원격 $base 와 견주지 못했다(git merge-base 종료 $ia_). 들어갔는지 확인한 뒤 같은 줄을 다시"
      # 후보가 원격에 없다(거절) — 함께 실은 lease 참조가 바뀌었으면(재제출) 기본 브랜치를 올리지 않고 종료 6(병합기)
      for e_ in $px_; do
        case $e_ in --force-with-lease=?*:*) ;; *) continue ;; esac
        r_=${e_#--force-with-lease=}; s_=${r_#*:}; r_=${r_%%:*}
        xr_=$(git ls-remote origin "$r_") || die "land: push가 거절됐는데 lease 참조 $r_ 를 다시 보지 못했다(git ls-remote) — 같은 줄을 다시"
        [ "$(printf '%s\n' "$xr_" | awk -v r="$r_" '$2 == r { print $1 }')" = "$s_" ] ||
          { echo "hc-lane land: 함께 실은 lease 참조 $r_ 가 바뀌었다(다시 제출) — $base 를 올리지 않았다" >&2; exit 6; }
      done
      git merge-base --is-ancestor "$rs_" "$cand"; ia_=$?
      [ $ia_ != 0 ] || die "land: push 실패(다른 쪽 push가 앞선 것이 아니다 — 위 git 출력을 본다)"
      [ $ia_ = 1 ] || die "land: push 결과 모름 — 원격 $base 와 견주지 못했다(git merge-base 종료 $ia_). 들어갔는지 확인한 뒤 같은 줄을 다시"
    else
      new=$cand old=$(git rev-parse -q --verify "$bref^{commit}") || die "land: $blab 를 읽지 못했다(git 실패) — 같은 줄을 다시"
      # <기본>이 체크아웃된 폴더(보통 메인 클론)가 있으면 그 폴더 작업 트리도 같이 올리고, 없으면 기대값 확인 update-ref
      # 목록을 못 읽으면 빈 목록으로 읽지 않는다 — 체크아웃된 <기본>에 update-ref하면 그 폴더 작업 트리·색인이 뒤처진다
      wl_=$(git worktree list --porcelain) || die "land: git worktree list 실패 — 로컬 $base 가 체크아웃된 폴더를 모른다. 같은 줄을 다시"
      at=$(printf '%s\n' "$wl_" | B="branch $bref" awk '/^worktree /{w=substr($0,10)} $0==ENVIRON["B"]{print w; exit}')
      # --ff-only --no-squash 고정: 저장소 설정(branch.<기본>.mergeOptions=--squash 등)이 착지를 바꾸지 않게 — squash면 main은 그대로인데 0이었다
      if [ -n "$at" ]; then HERDR_LAND=1 git -C "$at" merge --ff-only --no-squash -q "$new"
      else git merge-base --is-ancestor "$old" "$new" && git update-ref "$bref" "$new" "$old"; fi &&
        { echo "hc-lane land: 착지 ${new:0:9} → 로컬 $base${at:+ (폴더 $at)}"; return 0; }
      git merge-base --is-ancestor "$bref" "$new"; ia_=$?
      [ $ia_ != 0 ] || die "land: 로컬 $base 를 fast-forward로 올리지 못했다${at:+(폴더 $at 에 커밋 안 된 변경?)} — 위 git 출력을 본다"
      [ $ia_ = 1 ] || die "land: 로컬 $base 와 견주지 못했다(git merge-base 종료 $ia_) — 같은 줄을 다시"
    fi
    if [ $i -ge $tries ]; then
      echo "hc-lane land: 다른 쪽 착지가 먼저 들어가 ${tries}번 거절됐다. 같은 줄을 다시" >&2
      exit 4
    fi
    echo "hc-lane land: 다른 쪽 착지가 먼저 들어가 거절됨, 처음부터 다시 ($((i + 1))/$tries)" >&2
  done
}
