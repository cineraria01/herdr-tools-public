# go-run.sh: Go 판 도구 공통 실행기 — bin/<도구>(또는 그 lib)가 source해 go_run을 부른다 (2026-11-21 사용자 「Go로」, go-foundation)
#
# go_run <도구> [인자…]: Go 판(cmd/<도구>, 단일 바이너리)으로 exec — 되면 돌아오지 않는다. 돌아오면 종료 코드로 셸 판을 고른다:
#   0 셸 판으로 돈다(스위치가 bash거나, auto인데 Go 판을 쓸 수 없어 크게 경고함) · 1 스위치가 go인데 쓸 수 없음 · 2 스위치 값이 틀림
#  · 스위치: <도구>_IMPL(대문자·-는 _, 예 HC_MERGER_IMPL), 없으면 HERDR_IMPL, 없으면 auto. go·auto·bash(sh) 가운데 하나
#  · 바이너리: <도구>_BIN이 있으면 그것(빌드 안 함). 없으면 <모듈>/.bin/<도구>와 곁의 <도구>.src(그 바이너리의 소스 판).
#    .src가 지금 소스 판(go_run_src — go.mod·cmd/**·internal/**의 일반 파일 전부, go:embed 글 포함)과 같으면 그대로 쓰고, 다르거나 .src가
#    없으면(판 모름) 그 기계에서 go build(-trimpath, CGO_ENABLED=0, GOTOOLCHAIN=local, GOOS·GOARCH·GOFLAGS 등은 비움 — 이 기계 것,
#    빌드 캐시·GOPATH <모듈>/.bin/ — HOME과 상관없이). 지으면 빌드 시작 때 계산한 판을 .src에 적는다. 빌드가 안 되면 옛 바이너리를
#    쓰지 않는다(auto는 셸 판). 시각은 보지 않는다 — herdr-sync가 맥의 시각 그대로 깔고, 지운 파일은 시각에 안 보인다(2026-11-21 gofix-core B17·L1).
#    판 대조는 부를 때마다(한 번 약 8ms). herdr-sync는 소스만 깐다(점으로 시작하는 .bin은 나무 줄이 걷지 않는다 — 판도 숨김 파일을 뺀다)
#  · 상한(gofix-core B09): go build 600초(GO_RUN_BUILD_SECS)·--version 10초(GO_RUN_VERSION_SECS) — 넘으면 그 무리째 끊고 실패로 친다
#  · GO_RUN_BUILD=bg(Claude Code 훅 — 제한 5초 안에 짓지 않는다, 2026-11-21 hooks-go): 판이 다르거나 모르면 짓기를 뒤에 떼어 띄우고
#    (잠금 <바이너리>.build.lock — 짓는 무리가 쥔 flock이라 끝나거나 죽으면 저절로 풀리고 나이로 뺏지 않는다, 기록 <바이너리>.build.log)
#    이번에는 경고 없이 0 — 셸 판으로 돈다. 다음 부름부터 새 바이너리. 낡은 바이너리는 부르지 않는다(셸 판과 판정이 다를 수 있다)
#  · 모듈 자리: 저장소에서는 이 파일의 ../(go.mod가 있는 루트), 깔린 곳에서는 이 파일 곁 herdr-go/(~/.local/lib/herdr-go)
#  · 바이너리는 --version이 0으로 끝나야 쓴다(깨진 바이너리를 부르지 않게). 기본 자리 바이너리는 확인한 뒤 <바이너리>.ok를 남겨
#    그보다 새 바이너리가 오기 전까지 다시 확인하지 않는다(훅은 도구 호출마다 돈다 — 2026-11-21 hooks-go, 한 번 4ms).
#    판이 같은데 --version이 안 되면(남의 OS로 지어짐 등) 다시 짓는다
#  · 받은 바이너리(2026-11-21 사용자 「각 컴퓨터 마다 빌드 하지말고 github 에다가 빌드 한것도 처리해서 … 받아서」): herdr-sync Go 판이 맥에서 지어
#    공개 릴리스에 올리고 기계마다 <모듈>/.bin/<도구>에 받아 두며 곁에 <도구>.src를 적는다 — 위의 판 대조가 그대로 받은 것을 알아본다
#  · PATH와 상관없이 돈다(훅·시험이 PATH를 비운다): 바깥 명령은 절대 경로·/usr/bin:/bin을 먼저, 대문자 바꾸기는 bash 안에서
go_run_root=${BASH_SOURCE[0]%/*}
[ "$go_run_root" != "${BASH_SOURCE[0]}" ] || go_run_root=.
go_run_root=$(cd "$go_run_root" && pwd)
if [ -f "$go_run_root/../go.mod" ]; then go_run_root=$(cd "$go_run_root/.." && pwd); else go_run_root=$go_run_root/herdr-go; fi
go_run_gos=${GO_RUN_GOS-/opt/homebrew/bin/go /usr/local/go/bin/go /usr/local/bin/go}   # PATH 밖에서 찾아볼 go(시험은 비운다)

# go_run_go: 쓸 go 경로를 찍는다(없으면 빈 줄)
go_run_go() {
  local a
  command -v go 2>/dev/null && return 0
  for a in $go_run_gos; do [ -x "$a" ] && { echo "$a"; return 0; }; done
  return 0
}

# go_run_up <이름>: 대문자로, -는 _ — 결과는 go_run_up_r (포크 없이: bash 3.2에는 ${x^^}가 없다)
go_run_up() {
  local s=$1 c lo=abcdefghijklmnopqrstuvwxyz up=ABCDEFGHIJKLMNOPQRSTUVWXYZ pre
  go_run_up_r=
  while [ -n "$s" ]; do
    c=${s:0:1} s=${s:1}
    case $c in
      -) c=_ ;;
      [abcdefghijklmnopqrstuvwxyz]) pre=${lo%%"$c"*}; c=${up:${#pre}:1} ;;
    esac
    go_run_up_r=$go_run_up_r$c
  done
}

# go_run_lim <초> <명령…>: 그 초 안에 안 끝나면 그 명령이 띄운 무리(따로 둔 프로세스 그룹)째 TERM → 1초 뒤 KILL, 종료 124.
# 이 쪽이 INT·TERM·HUP을 받아도 무리를 거두고 끝난다. macOS에는 timeout 명령이 없어 perl(fork+setpgrp+alarm) — 없으면 상한 없이
go_run_lim_pl='my $t = shift; my $p = fork; defined $p or exit 126; if (!$p) { setpgrp; exec @ARGV or exit 127 } setpgrp $p, $p;
my $k = sub { kill "TERM", -$p; sleep 1; kill "KILL", -$p; exit $_[0] }; $SIG{ALRM} = sub { $k->(124) }; $SIG{$_} = sub { $k->(130) } for qw(INT TERM HUP);
alarm $t; waitpid $p, 0; exit($? & 127 ? 128 + ($? & 127) : $? >> 8)'
go_run_lim() {
  if [ -x /usr/bin/perl ]; then /usr/bin/perl -e "$go_run_lim_pl" "$@"; else shift; "$@"; fi
}

# go_run_build <도구> <바이너리> <go> [<판>]: 짓기 — 다른 실행이 같이 지어도 반쪽 파일을 부르지 않게 제 이름으로 만든 뒤 바꿔 넣고,
# 곁 .src에 빌드 시작 때 계산한 판(<판>)을 적는다(그사이 소스가 또 바뀌었으면 다음 부름이 판이 달라 다시 짓는다)
# GOPATH도 곁에: 기본값 $HOME/go에 pkg/mod를 만들지 않게(시험이 HOME을 임시 폴더로 둔다). -buildvcs=false: 저장소 안에서도 git 상태에 안 걸림
# GOOS·GOARCH·GOFLAGS·GOARM 등은 비운다: 교차 빌드 환경에서 불려도 이 기계 바이너리를(gofix-core L2)
go_run_build() {
  /bin/mkdir -p "$go_run_root/.bin" || return 1
  (cd "$go_run_root" && GOCACHE=${GOCACHE:-$PWD/.bin/go-build} GOPATH=${GOPATH:-$PWD/.bin/gopath} GOTOOLCHAIN=local CGO_ENABLED=0 \
    GOOS= GOARCH= GOFLAGS= GOARM= GOARM64= GOAMD64= GO386= GOMIPS= GOPPC64= GORISCV64= GOWASM= \
    go_run_lim "${GO_RUN_BUILD_SECS:-600}" "$3" build -trimpath -buildvcs=false -o ".bin/$1.$$" "./cmd/$1" </dev/null) >&2 &&
    /bin/rm -f "$2.src" && /bin/mv -f "$go_run_root/.bin/$1.$$" "$2" || { /bin/rm -f "$go_run_root/.bin/$1.$$"; return 1; }
  /bin/rm -f "$2.ok"
  [ -z "${4-}" ] || printf '%s\n' "$4" 2>/dev/null >"$2.src"
}

# go_run_bg <도구> <바이너리> <go> <판>: 짓기를 뒤에 떼어 띄운다(GO_RUN_BUILD=bg). 잠금은 <바이너리>.build.lock의 flock(gofix-core B30):
# 짓는 무리(fd 9를 물려받음)가 쥐고 있다가 끝나거나 죽으면 커널이 푼다 — 나이로 뺏지 않고 남의 잠금을 풀 일이 없다. 쥐었으면 돌아온다
go_run_bg() {
  /bin/mkdir -p "$go_run_root/.bin" || return 0
  ( exec 9>>"$2.build.lock" || exit 0
    [ ! -x /usr/bin/perl ] || /usr/bin/perl -MFcntl=:flock -e 'open(my $f, ">&=", 9) or exit 1; exit(flock($f, LOCK_EX | LOCK_NB) ? 0 : 1)' || exit 0
    go_run_build "$@" </dev/null >"$2.build.log" 2>&1 &
  ) >/dev/null 2>&1
}

# go_run_ok <바이너리> [<.ok>]: 쓸 수 있는가 — --version이 상한 안에 0(.ok가 바이너리보다 새면 확인 없이)
go_run_ok() {
  [ -x "$1" ] || return 1
  [ -n "${2-}" ] && [ "$2" -nt "$1" ] && return 0
  go_run_lim "${GO_RUN_VERSION_SECS:-10}" "$1" --version </dev/null >/dev/null 2>&1 || return 1
  [ -z "${2-}" ] || : 2>/dev/null >"$2"
}

# go_run_src: 모듈 소스 판 — go.mod·cmd/**·internal/** 의 일반 파일 전부(go:embed 글 포함, 숨김 파일·숨김 폴더 아래는 뺌 — herdr-sync 나무 줄이
# 걷지 않는다)를 상대 경로 바이트 순으로 cksum한 목록의 cksum(crc.크기). herdr-sync Go 판 srcDigest와 같은 값(그 시험이 견준다).
# PATH가 비어도(훅) 돌게 /usr/bin:/bin을 앞에
go_run_src() {
  (PATH=/usr/bin:/bin:$PATH; cd "$go_run_root" && find go.mod cmd internal -type f ! -path '*/.*' 2>/dev/null | LC_ALL=C sort | tr '\n' '\0' |
    xargs -0 cksum | cksum | awk '{print $1"."$2}')
}

# go_run_daemon: launchd·cron이 띄운 문맥인가(0 그렇다). 그 문맥의 새 Go 바이너리는 ~/Documents 첫 접근에서 macOS 개인 정보 보호(TCC)
# 창을 띄우고 사람 응답까지 잠든다 — ad-hoc 서명이 빌드마다 바뀌어 「허용」해도 새 판마다 다시 묻는다(2026-11-21 맥 hc-fill 37분 멈춤).
# 실측(2026-11-21 macOS 27): launchd 작업도 XPC_SERVICE_NAME=0이라 그것으로는 못 가른다. launchd 작업에는 TERM·HC_TERMINAL_ID가 없고,
# Herdr 터미널·hc exec(코어 문맥 — 창이 안 뜸)는 조상에 herdr-core가 있다. 그래서: 터미널·Herdr 터미널·SSH 표식이 다 없고
# 조상에 herdr-core·sshd가 없으면 그렇다. ps를 못 읽으면 그렇다(셸 판은 늘 돈다 — 창에 멈추는 쪽을 고르지 않는다).
# TERM=dumb도 없음으로 본다: launchd의 bash는 TERM이 없으면 스스로 dumb을 둔다(실측)
# PATH가 비어도(훅) 돌게 /usr/bin:/bin을 뒤에 — 앞에 두면 PATH의 ps(시험의 가짜 ps)를 가린다(go-integrate)
# macOS에서만 본다: 리눅스 cron·systemd 문맥에는 그 창이 없다 — 받은 바이너리를 쓴다(2026-11-21 go-linux-dist)
go_run_daemon() {
  case ${GO_RUN_OS:-$OSTYPE} in darwin*) ;; *) return 1 ;; esac
  [ -z "${HC_TERMINAL_ID-}" ] && [ "${TERM:-dumb}" = dumb ] && [ -z "${SSH_CONNECTION-}" ] || return 1
  (PATH=${PATH:+$PATH:}/usr/bin:/bin; ps -axo pid=,ppid=,comm= 2>/dev/null | awk -v p="$$" '
    { c = $3; for (i = 4; i <= NF; i++) c = c " " $i; sub(/.*\//, "", c); pp[$1] = $2; cm[$1] = c }
    END { for (k = 0; k < 64 && (p in pp) && p > 1; k++) { if (cm[p] ~ /^(herdr-core|sshd|sshd-session)$/) exit 1; p = pp[p] }; exit 0 }')
}

go_run() {
  local tool=$1 up v impl bin gob cur stale= ok= note
  shift
  go_run_up "$tool"; up=$go_run_up_r
  v=${up}_IMPL impl=${!v:-${HERDR_IMPL:-auto}}
  case $impl in
    bash|sh) return 0 ;;
    go|auto) ;;
    *) echo "$tool: ${up}_IMPL·HERDR_IMPL은 go·bash 가운데 하나: $impl" >&2; return 2 ;;
  esac
  # launchd 문맥이면 auto는 셸 판(IMPL=go로 못박으면 그대로 Go 판). 안내는 도구마다 처음 한 번(표식 파일)
  if [ "$impl" = auto ] && go_run_daemon; then
    note=${XDG_STATE_HOME:-$HOME/.local/state}/herdr-go/launchd-noted-$tool
    if [ ! -e "$note" ]; then
      /bin/mkdir -p "${note%/*}" 2>/dev/null && : >"$note" 2>/dev/null
      echo "$tool: launchd 문맥(터미널 밖)이라 셸 판으로 돈다 — Go 바이너리는 macOS 문서 폴더 권한 창에 멈춘다. Go 판은 hc-launchd-guard로 Herdr 터미널에서(README 「launchd 작업」)" >&2
    fi
    return 0
  fi
  v=${up}_BIN bin=${!v-}
  if [ -z "$bin" ]; then
    bin=$go_run_root/.bin/$tool ok=$go_run_root/.bin/$tool.ok cur=$(go_run_src)
    # 판(.src)이 지금 소스 판과 같아야 그대로 쓴다. 다르거나 .src가 없으면(판 모름) 지을 소스가 있는 한 낡음 — 새 빌드가 된 뒤에만 Go 판
    if [ -n "$cur" ] && [ -x "$bin" ] && [ "$(/bin/cat "$bin.src" 2>/dev/null)" = "$cur" ]; then
      go_run_ok "$bin" "$ok" || stale=1   # 판은 같은데 --version이 안 됨(남의 OS로 지어짐 등): 다시 짓는다(L2)
    elif [ -f "$bin.src" ] || [ -d "$go_run_root/cmd/$tool" ]; then
      stale=1
    fi
    if [ -n "$stale" ] && [ -d "$go_run_root/cmd/$tool" ] && gob=$(go_run_go) && [ -n "$gob" ]; then
      if [ "${GO_RUN_BUILD-}" = bg ]; then go_run_bg "$tool" "$bin" "$gob" "$cur"
      elif go_run_build "$tool" "$bin" "$gob" "$cur"; then stale=
      fi
    fi
    [ -z "$stale" ] || [ "${GO_RUN_BUILD-}" = bg ] || bin="$bin (판이 소스와 다르거나 모르고 이 기계에서 빌드도 못 함)"
  fi
  # 낡은 것(bg 짓기 중·판이 다른데 빌드 실패)은 부르지 않는다
  [ -z "$stale" ] && go_run_ok "$bin" "$ok" && exec "$bin" "$@"
  [ "$impl" = auto ] || { echo "$tool: Go 판을 쓸 수 없다(${up}_IMPL=go): $bin" >&2; return 1; }
  [ "${GO_RUN_BUILD-}" = bg ] && return 0
  # macOS가 아니고 go가 없으면(리눅스 서버 등 — 받은 바이너리가 없거나 판이 다르면) 셸 판이 정상 경로: 크게 경고하지 않고 처음 한 번만 한 줄(go-followups)
  case ${GO_RUN_OS:-$OSTYPE} in
    darwin*) ;;
    *) if [ -z "$(go_run_go)" ]; then
         note=${XDG_STATE_HOME:-$HOME/.local/state}/herdr-go/nogo-noted-$tool
         if [ ! -e "$note" ]; then
           /bin/mkdir -p "${note%/*}" 2>/dev/null && : >"$note" 2>/dev/null
           echo "$tool: 이 기계에는 go가 없어 셸 판으로 돈다(정상 — 다음부터 말하지 않음. Go 판이 필요하면 go를 깐다)" >&2
         fi
         return 0
       fi ;;
  esac
  printf '%s\n' "$tool: !!!!!! 경고: Go 판을 쓸 수 없다($bin — go가 없거나 build 실패) — 셸 판으로 돈다 !!!!!!" \
    "$tool: 고치기: 그 기계에 go를 깔고(brew install go) 다시 부른다. 일부러 셸 판이면 ${up}_IMPL=bash" >&2
  return 0
}
