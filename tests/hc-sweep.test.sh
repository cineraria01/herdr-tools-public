#!/bin/bash
# hc-sweep: 규칙 자체 시험(--self-test) + 고아 쓸기를 진짜 짧은 고아(sleep·스크립트)와 가짜 hc로.
# --only-under로 이 시험 폴더 아래 작업 트리만 보게 해 실제 기계의 다른 레인은 건드리지 않는다. 끝에 고아 0을 확인한다.
set -u
SWEEP=$(cd "$(dirname "$0")/.." && pwd)/bin/hc-sweep
T=$(mktemp -d "${TMPDIR:-/tmp}/hcsweep-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
P=$T/pids
mkdir -p "$P" "$T/bin" "$T/home" "$T/cfg"
pass=0 fail=0
t() { if eval "$2"; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "실패: $1"; [ -n "${out:-}" ] && printf '%s\n' "$out" | sed 's/^/    /'; fi; }
alive() { kill -0 "$1" 2>/dev/null; }
gone() { for _ in 1 2 3 4 5 6 7 8 9 10; do alive "$1" || return 0; sleep 0.2; done; return 1; }
allgone() { for f in "$P"/*; do gone "$(cat "$f")" || return 1; done; }
cleanup() {
  for f in "$P"/*; do [ -f "$f" ] && kill -KILL "$(cat "$f")" 2>/dev/null; done
  rm -rf "$T"
}
trap cleanup EXIT

out=$(python3 "$SWEEP" --self-test 2>&1)
t "self-test" '[ "$out" = ok ]'

# 가짜 hc: ls는 $T/ls.json, get <주소>는 $T/get-<주소>.json(없으면 실패)
cat >"$T/bin/hc" <<EOF
#!/bin/bash
case \$1 in
  ls) exec cat "$T/ls.json" ;;
  get) [ -f "$T/get-\$2.json" ] && exec cat "$T/get-\$2.json" ;;
esac
exit 1
EOF
chmod +x "$T/bin/hc"
export PATH="$T/bin:$PATH" HOME="$T/home" HERDR_TOOLS_CONFIG="$T/cfg"
sweep() { out=$(python3 "$SWEEP" --only-under "$T" "$@" 2>&1); rc=$?; }
cfg() { printf '%s\n' "$@" >"$T/cfg/config"; }
LOG=$T/home/.claude-tasks/closed.log

# orphan <이름> <cwd> <명령…>: 부모가 1인 프로세스를 띄워 pid를 $P/<이름>에
orphan() {
  local n=$1 d=$2
  shift 2
  ( cd "$d" || exit 1; "$@" </dev/null >/dev/null 2>&1 & echo $! >"$P/$n" )
  for _ in 1 2 3 4 5 6 7 8 9 10; do [ "$(ps -o ppid= -p "$(cat "$P/$n")" | tr -d ' ')" = 1 ] && return; sleep 0.2; done
  echo "준비 실패: $n 부모가 1이 아님"
}
# idle <작업 트리>: 그 아래 bin/idle(자식 sleep 하나를 두고 잠듦)
idle() { mkdir -p "$1/bin" "$1/tests"; printf '#!/bin/bash\nsleep 300\n' >"$1/bin/idle"; chmod +x "$1/bin/idle"; }
kid() { local k; for _ in 1 2 3 4 5 6 7 8 9 10; do k=$(pgrep -P "$(cat "$P/$1")"); [ -n "$k" ] && break; sleep 0.2; done; echo "$k" >"$P/$1-kid"; }
pid() { cat "$P/$1"; }

W=$T/app.wt
A=$W/a A2=$W/a2 B=$W/b C=$W/c E=$W/e K=$W/k H=$T/main.wt/h O=$T/agents/x
mkdir -p "$A" "$A2" "$O" "$T/main" "$T/elsewhere"
ln -s "$T" "$T/link"
for d in "$B" "$C" "$E" "$K" "$H"; do idle "$d"; done
orphan a "$A" sleep 300                    # cwd로만 걸림, 작업 트리 있음 → 찍기만(보류)
orphan a2 "$A2" sleep 300                  # cwd로만 걸림, 작업 트리 지움 → 끔
orphan b "$T" "$B/bin/idle"                # 명령 줄, 터미널이 링크 경로($T/link/…)로 b를 씀 → 남김
orphan c "$T" "$C/tests/../bin/idle"       # 명령 줄, 작업 트리 지움 → 끔(자식 sleep도)
orphan e "$T" "$E/bin/idle"                # 명령 줄, 작업 트리 있음, 쓰는 터미널 없음 → 끔
orphan h "$T" "$H/bin/idle"                # 명령 줄, 본 저장소 $T/main을 터미널이 씀 → 보류, 작업 트리 지우면 끔
orphan k "$T" "$K/bin/idle"                # 명령 줄, 작업 트리 지움, 자손이 live 터미널 프로세스 → 보류
orphan o "$O" sleep 300                    # 작업 트리 밖(launchd 작업 흉내) → 남김
kid b; kid c; kid k
rm -rf "$A2" "$C" "$K"
cat >"$T/ls.json" <<EOF
{"terminals":[{"address":"t1","state":"live","cwd":"$T/link/app.wt/b"},{"address":"t2","state":"finished","cwd":"$A"},
 {"address":"t4","state":"live","cwd":"$T/main/sub"},{"address":"t5","state":"live","cwd":"$T/elsewhere"},
 {"address":"box/t3","state":"live","cwd":"$T/app.wt/a-other"}]}
EOF
printf '{"address":"t1","pid":1}\n' >"$T/get-t1.json"
printf '{"address":"t4","pid":1}\n' >"$T/get-t4.json"
printf '{"address":"t5","pid":%s}\n' "$(cat "$P/k-kid")" >"$T/get-t5.json"
has() { printf "%s" "$out" | grep -q "$1 pid $(pid "$2") "; }
none() { ! printf "%s" "$out" | grep -Eq "(끌|끈) 고아: pid ($(pid b)|$(pid o)) "; }
allalive() { for n in "$@"; do alive "$(pid "$n")" || return 1; done; }
ALL="a a2 b c e h k o"

cfg "sweep.orphan_kill on"
sweep --dry-run --min-age 0
t "dry-run: 끌 것(a2·c·e)만 「끌 고아」, 안 끔" '[ $rc = 0 ] && has "끌 고아:" a2 && has "끌 고아:" c && has "끌 고아:" e && allalive $ALL'
t "dry-run: cwd로만·작업 트리 있음은 보류" 'printf "%s" "$out" | grep -q "보류 고아: pid $(pid a) .*cwd로만"'
t "dry-run: 본 저장소 터미널이 있으면 보류" 'printf "%s" "$out" | grep -q "보류 고아: pid $(pid h) .*본 저장소 터미널 있음"'
t "dry-run: 자손에 live 터미널 프로세스가 있으면 보류" 'printf "%s" "$out" | grep -q "보류 고아: pid $(pid k) .*자손에 터미널"'
t "dry-run: 링크 경로로 쓰는 터미널·작업 트리 밖은 목록에 없음" 'none && ! printf "%s" "$out" | grep -q "pid $(pid b) "'

sweep
t "기본 최소 나이(10분): 막 생긴 고아는 안 끔" '[ $rc = 0 ] && allalive $ALL && ! printf "%s" "$out" | grep -q "고아: pid"'

cfg "sweep.wt_suffix .lanes" "sweep.orphan_kill on"
sweep --dry-run --min-age 0
t "설정 sweep.wt_suffix: 다른 접미사면 .wt 아래는 안 봄" '[ $rc = 0 ] && ! printf "%s" "$out" | grep -q "pid $(pid a2) "'
cfg "sweep.orphan_kill on"

mv "$T/ls.json" "$T/ls.bak"
sweep --min-age 0
t "hc ls 실패: 아무것도 안 끄고 실패" '[ $rc != 0 ] && allalive $ALL'
echo '{}' >"$T/ls.json"
sweep --min-age 0
t "hc ls에 terminals 목록이 없음: 고아 쓸기 건너뜀, 안 끔" 'printf "%s" "$out" | grep -q "고아 쓸기 건너뜀" && allalive $ALL'
mv "$T/ls.bak" "$T/ls.json"

rm "$T/cfg/config"
sweep --min-age 0
sweep --min-age 0
t "끄기 설정 없음(기본 끔): 찍기만, 안 끔" 'printf "%s" "$out" | grep -q "끄기 꺼짐.*pid $(pid e) " && allalive $ALL'
t "끄기 꺼짐: closed.log에 한 번만 기록" '[ "$(grep -c "pid:$(pid e)	.*관찰" "$LOG")" = 1 ]'
cfg "sweep.orphan_kill yes"
sweep --min-age 0
t "모르는 끄기 값: 끄지 않음" 'allalive $ALL'

cfg "sweep.orphan_kill on"
sweep --min-age 0
t "작업 트리 지움(cwd로만) a2: 끔" 'gone "$(pid a2)"'
t "지워진 작업 트리 c: 끔, 자식 sleep도" 'gone "$(pid c)" && gone "$(cat "$P/c-kid")"'
t "작업 트리 있고 쓰는 터미널 없음 e: 끔" 'gone "$(pid e)"'
t "남김: cwd로만 a, 링크 경로 터미널 b, 본 저장소 h, 자손 터미널 k, 밖 o" 'allalive a b h k o'
t "기록: pid·나이·명령 앞부분" 'grep -q "pid:$(pid e)	.*고아(쓰는 터미널 없음)	$E	-	나이 0분 /bin/bash /" "$LOG" && grep -q "pid:$(pid c)	.*작업 트리 지워짐" "$LOG"'

rm -rf "$H"
sweep --min-age 0
t "본 저장소 터미널이 있어도 작업 트리를 지우면 끔" 'gone "$(pid h)"'

kill "$(cat "$P/b-kid")" "$(cat "$P/k-kid")" 2>/dev/null
kill "$(pid a)" "$(pid b)" "$(pid k)" "$(pid o)" 2>/dev/null
t "이 시험의 고아 0" allgone

echo "통과 $pass · 실패 $fail"
[ $fail = 0 ]
