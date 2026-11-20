#!/bin/bash
# hc-lane 병합기(제출·merger) 시험: 임시 bare origin + 레인 클론 둘 + 병합기 전용 클론, 가짜 hc. 실제 hc·네트워크는 쓰지 않는다.
# 실행: bash tests/hc-lane-merger.test.sh   (다른 사본은 HC_LANE_BIN=<경로>)
set -u
here=$(cd "$(dirname "$0")" && pwd)
LANE=${HC_LANE_BIN:-$here/../bin/hc-lane}
[ -f "$LANE" ] || { echo "hc-lane 없음: $LANE"; exit 1; }
T=$(mktemp -d "${TMPDIR:-/tmp}/hc-lane-merger-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
trap 'rm -rf "$T"' EXIT
. "$here/isolate.sh"
W="$T/공백 있는 폴더"
mkdir -p "$T/bin" "$W" "$T/vf"
cat >"$T/bin/hc" <<'EOF'
#!/bin/bash
case $1 in
  get) case $2 in
         t-live) echo '{"foreground":{"name":"node"},"name": "l-live","state":"live"}'; exit 0 ;;
         box2/t-far) echo '{"name": "l-far","state":"live"}'; exit 0 ;;
         t-fg) echo '{"foreground":{"name":"l-fg"},"name":"other","state":"live"}'; exit 0 ;;   # 레인 이름은 foreground에만
       esac; echo '{"error":{"kind":"not_found"}}' >&2; exit 3 ;;
  ls) echo '{"terminals":[]}' ;;
  send) [ ! -e "$T/hcfail" ] || exit 1; printf '%s\t%s\n' "$2" "$3" >>"$T/hc.send" ;;
  *) exit 0 ;;
esac
EOF
chmod +x "$T/bin/hc"
export PATH="$T/bin:$PATH" T
unset HC_LANE HC_TERMINAL_ID HC_LANE_MACHINE XDG_STATE_HOME HC_LANE_OWNER HERDR_MERGER HERDR_TOOLS_CONFIG HERDR_LAND_PUSH_EXTRA
export HC_LANE_MACHINE=box   # 이 기계 이름 = 켬 표식 refs/herdr/merger/box의 기계(병합기는 바퀴마다 그 표식이 자기 것인지 본다)
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t GIT_CONFIG_NOSYSTEM=1 HOME="$T"
EV=$T/.local/state/hc-lane/events.tsv LE=$T/.local/state/hc-lane/land-events.tsv tab=$(printf '\t')

# macOS awk의 ==는 로캘(strcoll) 비교라 en_US.UTF-8에서 「중간」=「막힘」이 참이다(hc-fill.test.sh와 같은 함정) — 시험의 awk는 바이트 비교
awk() { LC_ALL=C command awk "$@"; }
n=0 fails=0
t() { n=$((n + 1)); if eval "$2"; then echo "ok $n - $1"; else fails=$((fails + 1)); echo "FAIL $n - $1"; fi; }
conf() { mkdir -p "$T/.config/herdr-tools"; printf '%s\n' "$@" >"$T/.config/herdr-tools/config"; }
O=$W/origin.git
og() { git -C "$O" "$@"; }
qrefs() { og for-each-ref --format='%(refname)' refs/merge-queue/; }
mainlog() { og log --format=%s main; }

# origin: 검증 스크립트는 가지 이름의 실패 표지($T/vf/<가지>)가 있으면 실패하고, 돈 가지를 verify.log에 적는다
git init -q --bare -b main "$O"
git init -q -b main "$W/seed"
(cd "$W/seed" && mkdir .claude && cat >.claude/herdr-verify.sh <<'EOF'
#!/bin/bash
b=$(git rev-parse --abbrev-ref HEAD)
echo "verify $b" >>"$T/verify.log"
[ ! -x "$T/vhook.$b" ] || "$T/vhook.$b"   # 시험이 검증 도중의 일을 끼워 넣는다(같은 레인 다시 제출 등)
[ ! -e "$T/vf/$b" ] || { echo "vfail $b"; exit 1; }
[ ! -e bad.txt ] || { echo "vfail bad.txt"; exit 1; }   # 내용으로 실패(묶음 시험 — 어느 가지 이름으로 돌든)
EOF
printf 'base\n' >shared.txt && git add -A && git commit -qm base && git remote add origin "$O" && git push -q origin main)
# 레인 클론 둘(메인 폴더 이름이 프로젝트 이름 proj), 병합기 전용 클론
git clone -q "$O" "$W/a/proj"; git clone -q "$O" "$W/b/proj"; git clone -q "$O" "$W/m/proj"
# lane <클론> <레인> <파일> <내용>: 그 클론에서 origin/main 위 새 레인 가지에 커밋 하나
lane() { (cd "$W/$1/proj" && git fetch -q origin && git checkout -q -B "$2" origin/main && printf '%s\n' "$4" >"$3" && git add -- "$3" && git commit -qm "$2" -- "$3"); }
land() { out=$(cd "$W/$1/proj" && HC_LANE=$2 "$LANE" land 2>"$T/err"); rc=$?; }
# 병합기 MVP-1은 한 건씩(묶음 없음). 켬 표식 조회 재시도는 기다리지 않게
export HC_LANE_MGON_WAITS=0
merge() { mout=$(cd "$W/m/proj" && "$LANE" merger --repo "$W/m/proj" --once "$@" 2>"$T/merr"); mrc=$?; }

# 1. 설정이 없으면 land는 예전 그대로(제출 없이 main에 착지)
lane a l-old old.txt old
land a l-old
t "끔: land가 그대로 착지" '[ $rc = 0 ] && [ "$(mainlog | head -n 1)" = l-old ] && [ -z "$(qrefs)" ] || { cat "$T/err"; false; }'

# 2. 켬(원격 참조 refs/herdr/merger/<기계>): land가 제출로 — lane/<레인> + 큐 표식 하나, main은 그대로.
#    개인 설정(lane.*)에 무엇이 있어도 원격 참조만 본다
conf "lane.merger nothing=here"
on_() { og update-ref refs/herdr/merger/box main; }
off_() { og update-ref -d refs/herdr/merger/box; }
on_
lane a l-a a.txt a
sa=$(git -C "$W/a/proj" rev-parse HEAD)
land a l-a
t "제출: 종료 0, main은 그대로" '[ $rc = 0 ] && [ "$(mainlog | head -n 1)" = l-old ] && printf "%s" "$out" | grep -Fq "병합기(box)" || { cat "$T/err"; false; }'
t "제출: lane/l-a와 큐 표식이 같은 SHA" '[ "$(og rev-parse refs/heads/lane/l-a)" = "$sa" ] && [ "$(qrefs | wc -l | tr -d " ")" = 1 ] && [ "$(og rev-parse "$(qrefs)")" = "$sa" ] && qrefs | grep -Eq "^refs/merge-queue/[0-9]{10}-l-a$"'
fk=$(git -C "$W/a/proj" merge-base HEAD origin/main)
t "제출: 기록 refs/merge-queue-meta/<같은 이름> — 부모 = 원본, 원본·갈래점·제출 ID" 'm=$(qrefs | sed "s|refs/merge-queue/|refs/merge-queue-meta/|"); [ "$(og rev-parse "$m^")" = "$sa" ] && og log -1 --format=%B "$m" | grep -Fq "orig=$sa fork=$fk" && og log -1 --format=%B "$m" | grep -Eq "lane=l-a id=[^ ]+-[0-9]{8}T[0-9]{6}-[0-9]+ "'
# 기록은 병합기 일지와 기존 report 사건만(2026-11-20 지휘 — land-metrics의 착지 사건 파일은 이 가지에 없다)
t "제출: 안내 줄, 착지 사건 파일은 안 씀" 'printf "%s" "$out" | grep -Fq "hc-lane land: 제출" && { [ ! -e "$LE" ] || ! grep -Fq "$sa" "$LE"; }'
# 3. 같은 레인이 다시 제출하면 큐에는 새 것 하나
(cd "$W/a/proj" && printf 'a2\n' >a.txt && git commit -qam l-a2)
sa=$(git -C "$W/a/proj" rev-parse HEAD)
sleep 1   # 같은 초에 다시 제출해도 이름이 겹치지 않는지는 아래 7에서
land a l-a
t "다시 제출: 큐 표식은 새 SHA 하나, lane 가지도 새 SHA" '[ $rc = 0 ] && [ "$(qrefs | wc -l | tr -d " ")" = 1 ] && [ "$(og rev-parse "$(qrefs)")" = "$sa" ] && [ "$(og rev-parse refs/heads/lane/l-a)" = "$sa" ] || { cat "$T/err"; false; }'
# 4. 둘째 레인 제출 → 병합기가 순서대로 둘 다 착지, 큐·lane 가지 지움, 착지 사건 --name
lane b l-b b.txt b
land b l-b
v0=$(grep -c . "$T/verify.log")
merge
t "병합기: 둘 다 제출 순서대로 착지" '[ $mrc = 0 ] && [ "$(mainlog | head -n 2 | paste -sd , -)" = l-b,l-a2 ] || { cat "$T/merr"; echo "$mout"; false; }'
t "병합기: 큐·기록·lane 가지가 비고 실패 표식 없음" '[ -z "$(qrefs)" ] && [ -z "$(og for-each-ref refs/heads/lane/ refs/merge-failed/ refs/merge-queue-meta/)" ]'
t "다시 제출: 옛 기록도 지워져 기록이 쌓이지 않음(위 줄)·착지 사건에 제출 ID" 'awk -F"\t" "\$4 == \"l-a\" && \$5 == \"착지\"" "$EV" | grep -Fq "제출 ID"'
t "병합기: 레인 이름으로 착지 사건" 'awk -F"\t" "\$4 == \"l-a\" && \$5 == \"착지\"" "$EV" | grep -q . && awk -F"\t" "\$4 == \"l-b\" && \$5 == \"착지\"" "$EV" | grep -q .'
t "병합기: 하나씩 — 검증 두 번(가지 = 레인 이름)" '[ "$(grep -c . "$T/verify.log")" = "$((v0 + 2))" ] && tail -n 2 "$T/verify.log" | paste -sd , - | grep -Fxq "verify l-a,verify l-b"'
t "병합기: 착지 사건은 레인마다 「병합기 착지」" 'awk -F"\t" "\$5 == \"착지\"" "$EV" | grep -F "병합기 착지" | grep -c . | grep -qx 2'
# 아스트라 2차 중간 14·3차 중간 10: 접수가 확정되면 사건 상태 「제출」(한 줄 첫 낱말이 제출 ID — 중간이면 wait가 뜻밖의 종료로 봤다)
t "제출 사건: 레인마다 상태 「제출」, 「<ID> → <큐>」" 'awk -F"\t" "\$4 == \"l-b\" && \$5 == \"제출\"" "$EV" | grep -Eq "	[^ 	]+-[0-9]{8}T[0-9]{6}-[0-9]+ → refs/merge-queue/[0-9]{10}-l-b"'
t "병합기: 클론 작업 트리는 그대로·깨끗함, 로컬 레인 가지·임시 작업 트리 없음" '[ -z "$(git -C "$W/m/proj" status --porcelain)" ] && ! git -C "$W/m/proj" show-ref -q --verify refs/heads/l-a && [ "$(git -C "$W/m/proj" worktree list | grep -c .)" = 1 ]'

# 5. 충돌 제출은 merge-failed로, 다음 제출은 계속
lane a l-c shared.txt c-side
lane b l-d shared.txt d-side
land a l-c; land b l-d
lane a l-e e.txt e
land a l-e
merge
t "충돌: 먼저 낸 l-c는 착지" '[ "$(og show main:shared.txt)" = c-side ] || { cat "$T/merr"; false; }'
t "충돌: l-d는 refs/merge-failed로, lane/l-d는 남음" 'og for-each-ref --format="%(refname)" refs/merge-failed/ | grep -Eq "^refs/merge-failed/[0-9]{10}-l-d$" && og rev-parse -q --verify refs/heads/lane/l-d >/dev/null'
t "충돌: 다음 제출 l-e는 계속 착지" 'mainlog | grep -Fxq l-e && [ -z "$(qrefs)" ]'
t "충돌: l-d 막힘 사건(까닭·원본·기준·제출 ID·로그 경로)" 'awk -F"\t" "\$4 == \"l-d\" && \$5 == \"막힘\"" "$EV" | grep -F "착지 실패(종료 3)" | grep -F "원본 " | grep -F "기준 " | grep -F "제출 ID" | grep -Fq merger-logs'
t "충돌: 제출 기록은 refs/merge-failed-meta로" 'og for-each-ref --format="%(refname)" refs/merge-failed-meta/ | grep -Eq "^refs/merge-failed-meta/[0-9]{10}-l-d$" && [ -z "$(og for-each-ref refs/merge-queue-meta/)" ]'

# 6. 검증 실패도 merge-failed
lane a l-f f.txt f
land a l-f
: >"$T/vf/l-f"
merge
t "검증 실패: merge-failed·막힘, main에 안 들어감" '! mainlog | grep -Fxq l-f && og for-each-ref refs/merge-failed/ | grep -q l-f && awk -F"\t" "\$4 == \"l-f\" && \$5 == \"막힘\"" "$EV" | grep -q .'

# 7. 이미 main에 들어간 제출은 정리만(다시 착지하지 않음)
s_in=$(og rev-parse main~1)
og update-ref refs/merge-queue/1000000000-l-in "$s_in"
merge
t "이미 들어감: 큐 표식만 지우고 착지 사건, main 그대로" '[ -z "$(qrefs)" ] && [ "$(mainlog | head -n 1)" = l-e ] && awk -F"\t" "\$4 == \"l-in\" && \$5 == \"착지\"" "$EV" | grep -Fq "이미"'

# 8. 끊긴 앞 실행(구조 A — 일지 하나): 일지의 후보가 이미 main에 들어갔으면, 새 제출이 둘 와 있어도 묶음을 고르기 **전에** 그 제출을
#    정리만 한다(1차 높음 2 — 묶음이 원본을 다시 얹었다). main이 움직인 뒤 rebase한 후보라 원본 SHA는 main의 조상이 아니다
lane a l-h h.txt h
land a l-h
qh=$(qrefs) sh=$(og rev-parse "$(qrefs)")
lane b l-hx hx.txt hx; (cd "$W/b/proj" && HERDR_MERGER=1 HC_LANE=l-hx "$LANE" land >/dev/null 2>&1)   # main을 움직인다
bh=$(og rev-parse main)
(cd "$W/m/proj" && git fetch -q origin && git checkout -q -B l-h "$sh" && HERDR_MERGER=1 HC_LANE=l-h "$LANE" land >/dev/null 2>&1)
mkdir -p "$W/m/proj/.git/hc-merger" && printf '%s %s %s\n%s %s %s %s\n' l-h "$bh" "$sh" "$qh" "$sh" l-h - >"$W/m/proj/.git/hc-merger/cur"
git -C "$W/m/proj" checkout -q --detach
og update-ref -d "$qh"   # 그 착지의 main push가 큐 표식을 함께 지운 것처럼
lane a l-h2 h2.txt h2; land a l-h2
lane b l-h3 h3.txt h3; land b l-h3
vb=$(grep -c "^verify l-h$" "$T/verify.log")
merge
t "이어 받기: 앞 실행이 넣은 제출은 다시 넣지 않고 정리(착지 사건 「앞 병합기 실행이 넣었음」), 새 둘도 들어감" '! og merge-base --is-ancestor "$sh" main && [ "$(mainlog | grep -cx l-h)" = 1 ] && [ "$(grep -c "^verify l-h$" "$T/verify.log")" = "$vb" ] && awk -F"\t" "\$4 == \"l-h\" && \$5 == \"착지\"" "$EV" | grep -Fq "앞 병합기 실행이 넣었음" && mainlog | grep -Fxq l-h2 && mainlog | grep -Fxq l-h3 && [ -z "$(qrefs)" ] && [ ! -e "$W/m/proj/.git/hc-merger/cur" ] || { cat "$T/merr"; echo "$mout"; false; }'

# 9. 병합기는 한 클론에 하나(구조 C — 잠금 파일의 flock): 누가 쥐고 있으면 종료 75
ck=$(printf %s "$W/m/proj" | tr -c 'A-Za-z0-9_-' _)
mkdir -p "$T/.local/state/hc-lane"
perl -e 'use Fcntl qw(:flock); open(my $f, ">>", $ARGV[0]) or die; flock($f, LOCK_EX) or die; open(my $r, ">", $ARGV[1]); close $r; sleep 60' "$T/.local/state/hc-lane/merger.$ck.lock" "$T/held" & hp=$!
for i in $(seq 50); do [ -e "$T/held" ] && break; sleep 0.1; done
merge
t "잠금: 이미 돌면 종료 75" '[ $mrc = 75 ] && grep -Fq "아직 돈다" "$T/merr"'
kill "$hp"; wait "$hp" 2>/dev/null; rm -f "$T/held"

# 10. 제출 전 커밋 안 된 변경이면 멈춤, 아무것도 안 보냄
lane a l-dirty x.txt x
echo dirty >>"$W/a/proj/x.txt"
land a l-dirty
t "더러움: 제출 안 함" '[ $rc = 1 ] && [ -z "$(qrefs)" ] && ! og rev-parse -q --verify refs/heads/lane/l-dirty >/dev/null && grep -Fq "커밋 안 된" "$T/err"'
git -C "$W/a/proj" checkout -q -- x.txt

# 11. git 실패를 「없음」으로 읽지 않는다: 제출 중 ls-remote가 실패하면 멈추고 아무것도 안 보냄
rg=$(command -v git)
mkdir -p "$T/fail"
printf '#!/bin/sh\ncase " $* " in *" ls-remote "*merge-queue*) echo "fatal: unable to access" >&2; exit 128 ;; esac\nexec %s "$@"\n' "$rg" >"$T/fail/git"
chmod +x "$T/fail/git"
out=$(cd "$W/a/proj" && PATH="$T/fail:$PATH" HC_LANE=l-dirty "$LANE" land 2>"$T/err"); rc=$?
t "원격 못 읽음: 제출 멈춤" '[ $rc = 1 ] && [ -z "$(qrefs)" ] && grep -Fq "원격을 읽지 못했다" "$T/err"'
# 병합기 바퀴에서 ls-remote가 실패하면 큐가 빈 것으로 읽지 않고 다음 바퀴(아무것도 안 지움)
land a l-dirty
mout=$(cd "$W/m/proj" && PATH="$T/fail:$PATH" "$LANE" merger --repo "$W/m/proj" --once 2>"$T/merr"); mrc=$?
t "병합기 원격 못 읽음: 큐 그대로·안내, --once는 75(빈 큐와 가림 — 1차 중간 4)" '[ $mrc = 75 ] && [ "$(qrefs | wc -l | tr -d " ")" = 1 ] && grep -Fq "원격 큐를 읽지 못했다" "$T/merr"'
merge

# 12. 제출 push 응답 유실: 서버는 받았는데 실패로 돌아와도 두 참조가 그 SHA면 제출로
printf '#!/bin/sh\ncase " $* " in *" push "*--atomic*) %s "$@" >/dev/null 2>&1; echo "fatal: the remote end hung up unexpectedly" >&2; exit 128 ;; esac\nexec %s "$@"\n' "$rg" "$rg" >"$T/fail/git"
lane a l-lost l.txt l
out=$(cd "$W/a/proj" && PATH="$T/fail:$PATH" HC_LANE=l-lost "$LANE" land 2>"$T/err"); rc=$?
t "응답 유실: 원격을 다시 읽어 제출로" '[ $rc = 0 ] && qrefs | grep -q l-lost || { cat "$T/err"; false; }'
merge
t "응답 유실 뒤 병합기가 넣음" 'mainlog | grep -Fxq l-lost'

# 13. 병합기 안 착지(HERDR_MERGER=1)는 설정이 켜져 있어도 제출하지 않고 착지
lane a l-direct d.txt d
out=$(cd "$W/a/proj" && HERDR_MERGER=1 HC_LANE=l-direct "$LANE" land 2>"$T/err"); rc=$?
t "HERDR_MERGER=1: 착지 그대로" '[ $rc = 0 ] && [ "$(mainlog | head -n 1)" = l-direct ] && ! qrefs | grep -q l-direct'

# 14. 레인 이름 꼴이 아니면 제출하지 않는다
lane a Bad_Name bn.txt bn
land a Bad_Name
t "레인 이름 꼴: 제출 안 함" '[ $rc = 1 ] && [ -z "$(qrefs)" ] && grep -Fq "레인 이름" "$T/err" || { echo "rc=$rc"; cat "$T/err"; qrefs; false; }'

# 15. 제출한 가지는 원격 lane/*에 그 SHA 그대로 있을 때(ls-remote — 지금 원격) clean이 지운다 — 레인이 접수 뒤 바로 자리를 비운다(3차
#     중간 9·지휘). 원격 참조가 지워졌는데 낡은 추적 참조만 남았으면(2차 높음 4) 지우지 않는다
lane b l-cl cl.txt cl
land b l-cl
lane b l-cs cs.txt cs; land b l-cs
og update-ref -d refs/heads/lane/l-cs   # 원격에서는 지워짐(로컬 추적 참조 origin/lane/l-cs는 남음)
(cd "$W/b/proj" && git checkout -q --detach)
cout=$("$LANE" clean --repo "$W/b/proj" --branch l-cl 2>&1); crc=$?
t "clean: 원격 lane/<레인>에 그대로 있는 제출 가지는 지움(제출됨)" '[ $crc = 0 ] && ! git -C "$W/b/proj" show-ref -q --verify refs/heads/l-cl && printf "%s" "$cout" | grep -Fq "제출됨" || { echo "$cout"; false; }'
cout=$("$LANE" clean --repo "$W/b/proj" --branch l-cs 2>&1); crc=$?
t "clean: 원격 참조가 지워지고 낡은 추적 참조만 있으면 지우지 않음(멈춤)" '[ $crc = 1 ] && git -C "$W/b/proj" show-ref -q --verify refs/heads/l-cs || { echo "$cout"; false; }'
for r in $(og for-each-ref --format='%(refname)' refs/merge-queue/ refs/merge-queue-meta/ | grep l-cs); do og update-ref -d "$r"; done
lane b l-ns ns.txt ns
(cd "$W/b/proj" && git checkout -q --detach)
cout=$("$LANE" clean --repo "$W/b/proj" --branch l-ns 2>&1); crc=$?
t "clean: 제출 안 한 가지는 멈춤(그대로)" '[ $crc = 1 ] && git -C "$W/b/proj" show-ref -q --verify refs/heads/l-ns'

# 17. 재제출 경합(구조 B — 1차 높음 4): 옛 제출을 검증하는 동안 레인이 옛 파일을 뺀 새 제출을 내면, 옛 제출의 main push는 큐 표식
#     lease로 통째로 거절된다(main 그대로) — 옛 파일은 main에 들어가지 않고, 막힘 없이 새 제출이 들어간다
merge   # 앞 시험(15)이 남긴 l-cl 제출을 먼저 넣는다
lane a l-rs rs.txt rs1
land a l-rs
# 첫 검증(옛 제출) 도중 레인이 기준에서 다시 갈라 rs.txt 없이 다시 제출한다(검증은 통과)
printf '#!/bin/sh\nrm -f "$T/vhook.l-rs"\ncd "$W/a/proj" && git checkout -q -B l-rs origin/main && printf "rs2\\n" >rs2.txt && git add rs2.txt && git commit -qm l-rs2 rs2.txt && env -u HERDR_MERGER HC_LANE=l-rs "$LANE" land >/dev/null 2>&1\nexit 0\n' >"$T/vhook.l-rs"
chmod +x "$T/vhook.l-rs"
export W LANE
merge
t "재제출 경합: 병합기 계속(종료 0), 옛 파일 rs.txt는 main에 없음, 새 제출 착지, 막힘 없음" '[ $mrc = 0 ] && ! og cat-file -e main:rs.txt 2>/dev/null && og cat-file -e main:rs2.txt && mainlog | grep -Fxq l-rs2 && ! awk -F"\t" "\$4 == \"l-rs\" && \$5 == \"막힘\"" "$EV" | grep -q . && [ -z "$(qrefs)" ] && grep -Fq "lease 참조가 바뀜" "$T/merr" || { cat "$T/merr"; echo "$mout"; qrefs; false; }'

# 18. 큐 표식을 받지 못하면(그새 바뀜·망) 그 바퀴는 쉰다 — --once는 끝나고(몰아치며 되풀이하지 않음) 큐는 그대로
lane a l-ff ff.txt ff
land a l-ff
printf '#!/bin/sh\ncase " $* " in *" fetch "*refs/merge-queue/*) echo "fatal: could not fetch" >&2; exit 1 ;; esac\nexec %s "$@"\n' "$rg" >"$T/fail/git"
mout=$(cd "$W/m/proj" && PATH="$T/fail:$PATH" perl -e 'alarm 60; exec @ARGV' "$LANE" merger --repo "$W/m/proj" --once 2>"$T/merr"); mrc=$?
t "큐 받기 실패: --once가 75로 끝나고(몰아치지 않음) 큐 그대로" '[ $mrc = 75 ] && qrefs | grep -q l-ff && grep -Fq "git fetch 실패" "$T/merr"'
merge

# 19. 켬 표식을 읽지 못하면(켜졌는지 모름) 다시 보고, 끝내 모르면 착지하지 않는다 — 종료 75, main·큐·lane 가지 그대로
#     (1차 높음 5 — 조회 한 번 실패로 큐를 건너뛰어 main을 올렸다, 중간 9)
printf '#!/bin/sh\ncase " $* " in *" ls-remote "*herdr/merger*) echo "fatal: unable to access" >&2; exit 128 ;; esac\nexec %s "$@"\n' "$rg" >"$T/fail/git"
lane a l-unk unk.txt unk
m0=$(og rev-parse main)
out=$(cd "$W/a/proj" && PATH="$T/fail:$PATH" HC_LANE=l-unk "$LANE" land 2>"$T/err"); rc=$?
t "켬 모름: 착지하지 않고 75, main·큐 그대로" '[ $rc = 75 ] && [ "$(og rev-parse main)" = "$m0" ] && ! qrefs | grep -q l-unk && grep -Fq "켜졌는지 모른다" "$T/err" || { echo "rc=$rc"; cat "$T/err"; false; }'
t "켬 모름: lane 가지도 안 보냄" '! og rev-parse -q --verify refs/heads/lane/l-unk >/dev/null'
land a l-unk; merge   # 정리: 이제 제출·착지

# 20. 선행(HERDR_LAND_AFTER)은 최소판에서 뺐다(2026-11-20 지휘, 1차 높음 6·8) — 주면 제출하지 않는다(조용히 무시하지 않음)
lane a l-dep dep.txt dep
out=$(cd "$W/a/proj" && HERDR_LAND_AFTER=l-pre HC_LANE=l-dep "$LANE" land 2>"$T/err"); rc=$?
t "선행: 주면 제출 안 함(안내)" '[ $rc = 1 ] && ! qrefs | grep -q l-dep && grep -Fq "HERDR_LAND_AFTER" "$T/err"'

# 21. MVP-1은 묶음이 없다(2026-11-20 지휘 t1cj 2차 — 묶음·준비 훅은 MVP-2): --batch는 모르는 인자
merge --batch 3
t "MVP-1: --batch는 없는 인자(종료 1)" '[ $mrc = 1 ] && grep -Fq "알 수 없는 인자: --batch" "$T/merr"'
# 아스트라 2차 높음 3: 병합기가 넘긴 착지 인자(HERDR_LAND_PUSH_EXTRA·HERDR_MERGER)는 검증 자식에 물려주지 않는다 — 저장소 시험이 다시
# 부르는 hc-lane land가 그 큐 삭제 인자를 자기 가짜 원격 push에 실어 거절됐다
lane a l-env env.txt env; land a l-env
printf '#!/bin/sh\nprintf "%%s|%%s\\n" "${HERDR_LAND_PUSH_EXTRA-없음}" "${HERDR_MERGER-없음}" >"$T/env.l-env"\nrm -f "$T/vhook.l-env"\n' >"$T/vhook.l-env"; chmod +x "$T/vhook.l-env"
merge
t "검증 자식 환경: HERDR_LAND_PUSH_EXTRA·HERDR_MERGER 없음, 착지는 됨" '[ "$(cat "$T/env.l-env")" = "없음|없음" ] && mainlog | grep -Fxq l-env && [ -z "$(qrefs)" ] || { cat "$T/env.l-env"; cat "$T/merr"; false; }'

# 24. 제출한 커밋의 report 착지는 적지 않는다(감사 중간 1 — 거짓 착지는 겹침 소유를 풀고 지표에 남는다). 착지는 병합기가 --name으로
lane a l-rp rp.txt rp; land a l-rp
srp=$(git -C "$W/a/proj" rev-parse --short HEAD)
rout=$(cd "$W/a/proj" && HC_LANE=l-rp "$LANE" report 착지 "$srp" "넣음" 2>&1); rrc=$?
t "제출 뒤 report 착지: 종료 0, 안 적고 안내" '[ $rrc = 0 ] && printf "%s" "$rout" | grep -Fq "제출만" && ! awk -F"\t" "\$4 == \"l-rp\" && \$5 == \"착지\"" "$EV" | grep -q . || { echo "$rout"; false; }'
t "제출 안내 줄: report 착지는 적히지 않음" 'printf "%s" "$out" | grep -Fq "report 착지는 부르지 않아도"'
merge
t "제출 뒤 착지 사건은 병합기 것 하나" '[ "$(awk -F"\t" "\$4 == \"l-rp\" && \$5 == \"착지\"" "$EV" | grep -c .)" = 1 ]'

# 25. 병합기가 쓰는 가지 이름(기본 브랜치·묶음 가지)은 레인 이름으로 받지 않는다(감사 낮음 3)
lane a merge-batch mbn.txt mbn; land a merge-batch
t "레인 이름 merge-batch: 제출 안 함" '[ $rc = 1 ] && [ -z "$(qrefs)" ] && grep -Fq "병합기가 쓰는 가지 이름" "$T/err"'
(cd "$W/a/proj" && git checkout -q --detach && git branch -q -D merge-batch)

# 26. 일시 실패(착지 안 git fetch 실패·종료 75·4)는 돌려보내지 않고 큐에 두고 쉰다 — 3번 넘으면 그때 돌려보냄(감사 중간 2)
lane a l-tr tr.txt tr; land a l-tr
printf '#!/bin/sh\n[ "$*" != "fetch -q origin" ] || { echo "fatal: unable to access" >&2; exit 128; }\nexec %s "$@"\n' "$rg" >"$T/fail/git"
mfk() { mout=$(cd "$W/m/proj" && PATH="$T/fail:$PATH" "$LANE" merger --repo "$W/m/proj" --once 2>"$T/merr"); mrc=$?; }
mfk
# --once는 큐를 남긴 바퀴라 75(아스트라 2차 중간 11 — 0이면 「다 넣음」과 가리지 못한다)
t "일시 실패: 큐 그대로·merge-failed·막힘 없음, --once 75" '[ $mrc = 75 ] && qrefs | grep -q l-tr && ! og for-each-ref refs/merge-failed/ | grep -q l-tr && ! awk -F"\t" "\$4 == \"l-tr\" && \$5 == \"막힘\"" "$EV" | grep -q . && grep -Fq "일시 실패(종료 1" "$T/merr" || { cat "$T/merr"; false; }'
mfk; mfk; mfk
t "일시 실패가 3번 넘으면 돌려보냄(merge-failed·막힘)" 'og for-each-ref refs/merge-failed/ | grep -q l-tr && awk -F"\t" "\$4 == \"l-tr\" && \$5 == \"막힘\"" "$EV" | grep -Fq "일시 실패가 3번" && ! qrefs | grep -q l-tr || { cat "$T/merr"; false; }'
t "일시 실패 횟수 파일은 정리됨" '[ ! -e "$W/m/proj/.git/hc-merger/l-tr.tr" ]'

# 27. 착지 뒤 fetch가 실패해도 병합기는 멈추지 않고, 다음 바퀴에 일지로 정리한다(감사 중간 4)
lane a l-nf nf.txt nf; land a l-nf
printf '#!/bin/sh\n[ "$*" != "fetch -q origin +refs/heads/main:refs/remotes/origin/main" ] || { echo "fatal: unable to access" >&2; exit 128; }\nexec %s "$@"\n' "$rg" >"$T/fail/git"
mfk
t "착지 뒤 fetch 실패: 멈추지 않고 일지를 두고 쉼(--once 75), main엔 들어감" '[ $mrc = 75 ] && mainlog | grep -Fxq l-nf && [ -e "$W/m/proj/.git/hc-merger/cur" ] && grep -Fq "일지를 두고" "$T/merr" || { cat "$T/merr"; false; }'
merge
t "착지 뒤 fetch 실패: 다음 바퀴가 정리(다시 넣지 않음, 착지 사건 하나)" '[ -z "$(qrefs)" ] && [ "$(mainlog | grep -cx l-nf)" = 1 ] && [ "$(awk -F"\t" "\$4 == \"l-nf\" && \$5 == \"착지\"" "$EV" | grep -c .)" = 1 ] || { cat "$T/merr"; false; }'

# 28. 착지가 rebase 충돌(종료 3)로 끝나도 병합기는 멈추지 않는다(감사 중간 3): 검증 도중 원격 main이 충돌하게 움직이면 — 그 제출은
#     merge-failed, 다음 제출은 들어간다, 클론에 멈춘 rebase가 남지 않는다
export O
cat >"$T/vhook.l-m1" <<'EOF'
#!/bin/bash
rm -f "$T/vhook.l-m1"
d=$(mktemp -d "$T/ph.XXXXXX") && git clone -q "$O" "$d/c" && cd "$d/c" && printf 'zz\n' >m3.txt && git add m3.txt && git commit -qm l-m3x && git push -q origin main
EOF
chmod +x "$T/vhook.l-m1"
lane a l-m1 m3.txt m1; land a l-m1
lane a l-m2 m4.txt m2; land a l-m2
merge
t "rebase 충돌: 병합기 계속, l-m1은 merge-failed, l-m2는 착지" '[ $mrc = 0 ] && og for-each-ref refs/merge-failed/ | grep -q l-m1 && mainlog | grep -Fxq l-m2 && mainlog | grep -Fxq l-m3x && [ -z "$(qrefs)" ] || { cat "$T/merr"; echo "$mout"; false; }'
t "rebase 충돌: 클론에 멈춘 rebase 없음" '[ ! -e "$W/m/proj/.git/rebase-merge" ] && [ ! -e "$W/m/proj/.git/rebase-apply" ]'

# 29. 병합기가 멈추면(die) 막힘 사건 --name merger(감사 중간 4)
: >"$W/m/proj/stray.txt"
merge
t "병합기 멈춤: 종료 1, 막힘 사건 「병합기 멈춤」" '[ $mrc = 1 ] && awk -F"\t" "\$4 == \"merger\" && \$5 == \"막힘\"" "$EV" | grep -Fq "병합기 멈춤" || { cat "$T/merr"; false; }'
rm -f "$W/m/proj/stray.txt"

# 30. SIGKILL(구조 A·C): 검증 중 병합기만 죽어도 착지 자식이 잠금을 쥐고 끝까지 간다 — 새 병합기는 75(같은 클론을 안 씀, 1차 높음 9),
#     그 착지가 끝난 뒤 다음 병합기가 일지로 완료 처리(다시 검증 안 함)
lane a l-kill kill.txt kill; land a l-kill
# 기다림 상한 3분 — 시험이 꼬여도 검증 손자가 끝없이 남지 않게(2026-11-20 병합기 첫 착지에서 이 줄이 15분 멈췄다)
printf '#!/bin/sh\n: >"$T/inverify"\ni=0; while [ ! -e "$T/go" ] && [ $i -lt 900 ]; do sleep 0.2; i=$((i + 1)); done\nrm -f "$T/vhook.l-kill"\n' >"$T/vhook.l-kill"; chmod +x "$T/vhook.l-kill"
(cd "$W/m/proj" && exec "$LANE" merger --repo "$W/m/proj" --once) >"$T/mk.out" 2>&1 & mkp=$!
for i in $(seq 300); do [ -e "$T/inverify" ] && break; sleep 0.1; done
kill -9 "$mkp"; wait "$mkp" 2>/dev/null
merge
t "SIGKILL: 착지 자식이 살아 있으면 새 병합기는 75" '[ -e "$T/inverify" ] && [ $mrc = 75 ] || { cat "$T/merr"; cat "$T/mk.out"; false; }'
: >"$T/go"
for i in $(seq 300); do mainlog | grep -Fxq l-kill && break; sleep 0.1; done
for i in $(seq 100); do merge; [ $mrc != 75 ] && break; sleep 0.1; done
t "SIGKILL: 그 착지는 들어갔고 다음 병합기가 일지로 완료(검증 한 번, 착지 사건 하나 — 「앞 병합기 실행」)" 'mainlog | grep -Fxq l-kill && [ $mrc = 0 ] && [ -z "$(qrefs)" ] && [ "$(grep -cx "verify l-kill" "$T/verify.log")" = 1 ] && [ "$(awk -F"\t" "\$4 == \"l-kill\" && \$5 == \"착지\"" "$EV" | grep -c .)" = 1 ] && awk -F"\t" "\$4 == \"l-kill\" && \$5 == \"착지\"" "$EV" | grep -Fq "앞 병합기 실행" || { cat "$T/merr"; false; }'
rm -f "$T/inverify" "$T/go"

# 31. 다른 기본 브랜치로 낸 제출은 건드리지 않는다(1차 높음 7)
og update-ref refs/heads/release main
lane a l-rel rel.txt rel
out=$(cd "$W/a/proj" && git fetch -q origin && HC_LANE=l-rel "$LANE" land release 2>"$T/err"); rc=$?
m0=$(og rev-parse main) r0=$(og rev-parse release)   # 3차 중간·시험 2: 두 브랜치 SHA를 실행 전후로 대조
merge
t "다른 기본 브랜치: 큐에 그대로, 건너뜀 안내, main·release 그대로" '[ $rc = 0 ] && qrefs | grep -q l-rel && grep -Fq "기본 브랜치 release 로 낸 제출" "$T/merr" && [ "$(og rev-parse main)" = "$m0" ] && [ "$(og rev-parse release)" = "$r0" ] || { cat "$T/err"; cat "$T/merr"; false; }'
for r in $(og for-each-ref --format='%(refname)' refs/merge-queue/ refs/merge-queue-meta/ | grep l-rel) refs/heads/lane/l-rel refs/heads/release; do og update-ref -d "$r"; done

# 32. 접수 확인 사이에 병합기가 처리해 세 참조가 사라져도 접수로 본다(1차 중간 1 — 제출 기록이 refs/merge-done/<레인>에)
cat >"$T/fail/git" <<EOF
#!/bin/sh
case "\$1 \$*" in
  push*refs/heads/lane/l-fast*) "$rg" "\$@"; r=\$?; (cd "$W/m/proj" && PATH="$PATH" "$LANE" merger --repo "$W/m/proj" --once >/dev/null 2>&1); exit \$r ;;
esac
exec "$rg" "\$@"
EOF
lane a l-fast fast.txt fast
out=$(cd "$W/a/proj" && PATH="$T/fail:$PATH" HC_LANE=l-fast "$LANE" land 2>"$T/err"); rc=$?
t "빠른 병합기: 제출은 종료 0(접수), main에 들어감" '[ $rc = 0 ] && mainlog | grep -Fxq l-fast || { echo "rc=$rc"; cat "$T/err"; false; }'

# 33. 가지 이름과 레인 이름이 달라도(--branch) 제출은 레인 이름으로. 병합기가 넣은 뒤에는 clean이 그 가지를 지운다(main에 듦)
lane b l-brx brx.txt brx
out=$(cd "$W/b/proj" && HC_LANE=l-bry "$LANE" land 2>"$T/err"); rc=$?
merge
(cd "$W/b/proj" && git checkout -q --detach && git fetch -q origin)
cout=$("$LANE" clean --repo "$W/b/proj" --branch l-brx 2>&1); crc=$?
t "clean: 레인 이름이 다른 제출도 병합기가 넣은 뒤엔 지움" '[ $rc = 0 ] && mainlog | grep -Fxq l-brx && [ $crc = 0 ] && ! git -C "$W/b/proj" show-ref -q --verify refs/heads/l-brx || { echo "$cout"; false; }'

# 34. 제출만 한 커밋은 report 착지 --name으로도, 추적 참조가 없어도(push 응답 유실) 착지로 안 적는다(1차 중간 7)
lane a l-rq rq.txt rq; land a l-rq
srq=$(git -C "$W/a/proj" rev-parse --short HEAD)
rout=$(cd "$W/a/proj" && "$LANE" report 착지 "$srq" x --name l-rq 2>&1)
git -C "$W/a/proj" update-ref -d refs/remotes/origin/lane/l-rq
rout2=$(cd "$W/a/proj" && HC_LANE=l-rq "$LANE" report 착지 "$srq" x 2>&1)
t "report 착지: --name·추적 참조 없음도 안 적음" 'printf "%s" "$rout" | grep -Fq "제출만" && printf "%s" "$rout2" | grep -Fq "제출만" && ! awk -F"\t" "\$4 == \"l-rq\" && \$5 == \"착지\"" "$EV" | grep -q . || { echo "$rout / $rout2"; false; }'
merge

# 35. 무시되지 않은 새 파일이 있으면 제출하지 않는다(1차 중간 8)
lane a l-ut ut.txt ut
: >"$W/a/proj/stray-new.txt"
land a l-ut
t "새 파일: 제출 안 함" '[ $rc = 1 ] && ! qrefs | grep -q l-ut && grep -Fq "새 파일" "$T/err"'
rm -f "$W/a/proj/stray-new.txt"; land a l-ut; merge

# 36. 기본은 한 건씩(1차 중간 11) — --batch 없이 띄우면 제출 둘에 검증 둘
lane a l-d1 d1.txt d1; land a l-d1
lane a l-d2 d2.txt d2; land a l-d2
v0=$(grep -c . "$T/verify.log")
mout=$(cd "$W/m/proj" && "$LANE" merger --repo "$W/m/proj" --once 2>"$T/merr"); mrc=$?
t "기본 하나씩: 검증 두 번" '[ $mrc = 0 ] && [ $(( $(grep -c . "$T/verify.log") - v0 )) = 2 ] && [ -z "$(qrefs)" ]'

# 37. 결과 알림(1차 중간 12): 제출한 레인 터미널이 이 기계에 살아 있고 이름이 맞으면 hc send, 사건은 제출 때 적은 지휘(--to)로
lane a l-live live.txt live
out=$(cd "$W/a/proj" && HC_TERMINAL_ID=t-live HC_LANE_OWNER=t-own HC_LANE_JOB=j1120-abc HC_LANE=l-live "$LANE" land 2>"$T/err"); rc=$?
merge
t "알림: 레인 터미널에 hc send, 착지 사건 --to 지휘" 'grep -F "t-live	" "$T/hc.send" | grep -Fq "착지" && awk -F"\t" "\$4 == \"l-live\" && \$5 == \"착지\" && \$9 == \"t-own\"" "$EV" | grep -q . || { cat "$T/hc.send" 2>/dev/null; grep l-live "$EV"; false; }'

# 병합기 일지(2026-11-20 지휘 — 효과 수치는 병합기 일지 하나에서): 제출마다 12칸 한 줄
MJ=$T/.local/state/hc-lane/merger-journal.tsv
t "병합기 일지: 착지는 후보 40자·묶음 크기·착지 시작/끝·작업 ID, 돌려보냄은 후보 -" 'awk -F"\t" "\$3 == \"l-a\" && NF == 12 && \$10 == \"착지\" && \$11 ~ /^[0-9a-f]{40}\$/ && \$7 == 1 &&\$8 != \"-\" && \$9 != \"-\"" "$MJ" | grep -q . && awk -F"\t" "\$3 == \"l-live\" && \$5 == \"j1120-abc\" && \$4 != \"-\"" "$MJ" | grep -q . && awk -F"\t" "\$3 == \"l-d\" && \$10 == \"돌려보냄\" && \$11 == \"-\"" "$MJ" | grep -q . || { cat "$MJ"; false; }'

# 38. 보낼 함(1차 중간 2): 원격을 고친 뒤 사건을 쓰기 전에 죽어도 다음 바퀴가 보낸다
printf '%s\n' 착지 - "보낼 함 시험" l-ob - - - >"$W/m/proj/.git/hc-merger/out/1700000000-l-ob"
merge
t "보낼 함: 남은 사건을 다음 바퀴가 보내고 지움" 'awk -F"\t" "\$4 == \"l-ob\" && \$5 == \"착지\"" "$EV" | grep -Fq "보낼 함 시험" && [ ! -e "$W/m/proj/.git/hc-merger/out/1700000000-l-ob" ]'

# 39. 돌려보내기를 옮기다 원격이 끊겨도 병합기는 멈추지 않고 다른 제출을 넣는다, 다음 바퀴에 다시 옮긴다(1차 중간 3)
printf '#!/bin/sh\ncase "$1 $*" in push*refs/merge-failed/*) echo "fatal: unable to access" >&2; exit 128 ;; esac\nexec %s "$@"\n' "$rg" >"$T/fail/git"
lane a l-mf mf.txt mf; land a l-mf; : >"$T/vf/l-mf"
lane a l-mg mg.txt mg; land a l-mg
mout=$(cd "$W/m/proj" && PATH="$T/fail:$PATH" "$LANE" merger --repo "$W/m/proj" --once 2>"$T/merr"); mrc=$?
t "옮기기 끊김: 멈추지 않음(75), l-mg는 착지, l-mf는 큐에 남고 막힘은 아직" '[ $mrc = 75 ] && mainlog | grep -Fxq l-mg && qrefs | grep -q l-mf && ! awk -F"\t" "\$4 == \"l-mf\" && \$5 == \"막힘\"" "$EV" | grep -q . || { cat "$T/merr"; false; }'
merge
t "옮기기 끊김 뒤: 다음 바퀴에 merge-failed로 옮기고 막힘" 'og for-each-ref refs/merge-failed/ | grep -q l-mf && awk -F"\t" "\$4 == \"l-mf\" && \$5 == \"막힘\"" "$EV" | grep -q . && ! qrefs | grep -q l-mf || { cat "$T/merr"; false; }'

# ---- 아스트라 2차(a0c63cc) 지적 — 고치기 전 판에서 실패하는 시험 ----
# 높음 5: 원격 설정(git remote get-url)을 읽지 못하면 「꺼짐」이 아니라 모름 — 착지하지 않고 75(원격 없음 = 종료 2일 때만 직접 착지)
printf '#!/bin/sh\n[ "$*" != "remote get-url origin" ] || { echo "fatal: bad config" >&2; exit 128; }\nexec %s "$@"\n' "$rg" >"$T/fail/git"
lane a l-gu gu.txt gu
m0=$(og rev-parse main)
out=$(cd "$W/a/proj" && PATH="$T/fail:$PATH" HC_LANE=l-gu "$LANE" land 2>"$T/err"); rc=$?
t "원격 설정 못 읽음: 착지·제출 안 하고 75" '[ $rc = 75 ] && [ "$(og rev-parse main)" = "$m0" ] && [ "$(git -C "$W/a/proj" rev-parse main)" != "$(git -C "$W/a/proj" rev-parse HEAD)" ] && ! qrefs | grep -q l-gu && grep -Fq "원격 설정" "$T/err" || { echo "rc=$rc"; cat "$T/err"; false; }'
land a l-gu; merge
# 중간 1: 제출 push 직후 병합기가 main push로 큐 표식을 지웠어도(lane·기록만 남음) 접수로 본다
cat >"$T/fail/git" <<EOF
#!/bin/sh
case "\$*" in
  *push*--atomic*refs/heads/lane/l-acc*) "$rg" "\$@"; r=\$?; for q in \$("$rg" -C "$O" for-each-ref --format='%(refname)' 'refs/merge-queue/*-l-acc'); do "$rg" -C "$O" update-ref -d "\$q"; done; exit \$r ;;
esac
exec "$rg" "\$@"
EOF
lane a l-acc acc.txt acc
out=$(cd "$W/a/proj" && PATH="$T/fail:$PATH" HC_LANE=l-acc "$LANE" land 2>"$T/err"); rc=$?
t "접수: 큐 표식이 먼저 지워져도 기록이 있으면 제출(종료 0)" '[ $rc = 0 ] && printf "%s" "$out" | grep -Fq "hc-lane land: 제출" || { echo "rc=$rc"; cat "$T/err"; false; }'
for r in $(og for-each-ref --format='%(refname)' refs/merge-queue-meta/ | grep l-acc) refs/heads/lane/l-acc; do og update-ref -d "$r"; done
# 중간 2: 끊긴 앞 실행을 정리할 때 후보 가지를 읽다 git이 실패하면(종료 128 — 「없음」인 1이 아님) 일지를 두고 쉰다
lane a l-rv rv.txt rv; land a l-rv
srv=$(git -C "$W/a/proj" rev-parse HEAD) qrv=$(qrefs | grep l-rv)
(cd "$W/m/proj" && git fetch -q origin "$qrv" && git branch -f l-rv FETCH_HEAD)
printf '%s %s %s\n%s %s %s %s\n' l-rv "$(og rev-parse main)" "$srv" "$qrv" "$srv" l-rv - >"$W/m/proj/.git/hc-merger/cur"
printf '#!/bin/sh\n[ "$*" != "rev-parse -q --verify refs/heads/l-rv" ] || { echo "fatal: bad object" >&2; exit 128; }\nexec %s "$@"\n' "$rg" >"$T/fail/git"
mout=$(cd "$W/m/proj" && PATH="$T/fail:$PATH" "$LANE" merger --repo "$W/m/proj" --once 2>"$T/merr"); mrc=$?
t "복구 중 후보 조회 실패: 일지를 두고 쉼(75), 큐 그대로" '[ $mrc = 75 ] && [ -e "$W/m/proj/.git/hc-merger/cur" ] && qrefs | grep -q l-rv && grep -Fq "후보 가지를 읽지 못했다" "$T/merr" || { echo "rc=$mrc"; cat "$T/merr"; false; }'
merge
t "복구 중 후보 조회 실패 뒤: 다음 바퀴가 넣음" 'mainlog | grep -Fxq l-rv && [ -z "$(qrefs)" ] && [ ! -e "$W/m/proj/.git/hc-merger/cur" ] || { cat "$T/merr"; false; }'
# 중간 3: 착지 뒤 lane·기록을 지우는 push가 실패하면 착지로 적지 않고 일지를 두고 쉰다(큐 표식은 main push가 이미 지웠다)
printf '#!/bin/sh\ncase " $* " in *" push "*" :refs/heads/lane/l-cf "*) echo "fatal: unable to access" >&2; exit 128 ;; esac\nexec %s "$@"\n' "$rg" >"$T/fail/git"
lane a l-cf cf.txt cf; land a l-cf
mout=$(cd "$W/m/proj" && PATH="$T/fail:$PATH" "$LANE" merger --repo "$W/m/proj" --once 2>"$T/merr"); mrc=$?
t "정리 push 실패: main엔 들었지만 착지 사건 없음·일지 남음·lane 남음(75)" '[ $mrc = 75 ] && mainlog | grep -Fxq l-cf && ! awk -F"\t" "\$4 == \"l-cf\" && \$5 == \"착지\"" "$EV" | grep -q . && [ -e "$W/m/proj/.git/hc-merger/cur" ] && og rev-parse -q --verify refs/heads/lane/l-cf >/dev/null || { echo "rc=$mrc"; cat "$T/merr"; false; }'
# 3차 중간 5: 정리가 계속 실패하는 동안에는 다음 바퀴에도 착지 사건이 나가지 않는다(정리 확인 뒤에만 보낼 함에)
mout=$(cd "$W/m/proj" && PATH="$T/fail:$PATH" "$LANE" merger --repo "$W/m/proj" --once 2>"$T/merr"); mrc=$?
t "정리 push가 또 실패: 착지 사건 아직 없음(75)" '[ $mrc = 75 ] && ! awk -F"\t" "\$4 == \"l-cf\" && \$5 == \"착지\"" "$EV" | grep -q . || { echo "rc=$mrc"; cat "$T/merr"; false; }'
merge
t "정리 push 실패 뒤: 다음 바퀴가 지우고 착지 사건 하나" '[ "$(awk -F"\t" "\$4 == \"l-cf\" && \$5 == \"착지\"" "$EV" | grep -c .)" = 1 ] && ! og rev-parse -q --verify refs/heads/lane/l-cf >/dev/null || { cat "$T/merr"; false; }'
# 중간 5·3차 중간 6: 사건을 보낸 뒤 일지를 지우기 전에 끊겨(보낸 표시를 쓰기 전에 끊긴 것 포함) 다시 정리해도 착지 사건·일지 줄은 한 번
ncf=$(awk -F"\t" '$3 == "l-cf" && $10 == "착지" { print $2 }' "$T/.local/state/hc-lane/merger-journal.tsv" | tail -n 1)
mcf=$(og rev-parse main)
printf '%s %s %s\n%s %s %s %s\n' l-cf "$mcf" "$mcf" "refs/merge-queue/$ncf" "$(git -C "$W/a/proj" rev-parse l-cf)" l-cf "$(git -C "$W/m/proj" rev-parse "refs/hc-merger/meta/$ncf")" >"$W/m/proj/.git/hc-merger/cur"
find "$W/m/proj/.git/hc-merger/sent" -type f -delete 2>/dev/null   # 옛 판의 보낸 표시(사건 뒤 따로 쓰던 것)가 없는 꼴
merge
t "다시 정리: 착지 사건·병합기 일지 줄은 그대로 하나" '[ -n "$ncf" ] && [ $mrc = 0 ] && [ "$(awk -F"\t" "\$4 == \"l-cf\" && \$5 == \"착지\"" "$EV" | grep -c .)" = 1 ] && [ "$(awk -F"\t" "\$3 == \"l-cf\" && \$10 == \"착지\"" "$T/.local/state/hc-lane/merger-journal.tsv" | grep -c .)" = 1 ] || { cat "$T/merr"; false; }'
# 중간 4: 돌려보낼 기록이 잘려(빈 파일) 남아도 병합기는 멈추지 않고 그 기록만 버린다
: >"$W/m/proj/.git/hc-merger/fail/1700000001-l-zz"
merge
t "잘린 돌려보낼 기록: 멈추지 않고(0) 버림" '[ $mrc = 0 ] && [ ! -e "$W/m/proj/.git/hc-merger/fail/1700000001-l-zz" ] && grep -Fq "깨졌다" "$T/merr" || { echo "rc=$mrc"; cat "$T/merr"; false; }'
# 중간 8: report 착지는 제출 때의 기본 브랜치로 판정한다 — release로 낸 커밋이 main에만 들었으면 아직 착지 아님
og update-ref refs/heads/release main
lane a l-rb rb.txt rb
out=$(cd "$W/a/proj" && git fetch -q origin && HC_LANE=l-rb "$LANE" land release 2>"$T/err"); rc=$?
srb=$(git -C "$W/a/proj" rev-parse HEAD)
og update-ref refs/heads/main "$srb"; git -C "$W/a/proj" fetch -q origin
rout=$(cd "$W/a/proj" && "$LANE" report 착지 "$srb" x --name l-rb 2>&1)
t "report 착지: release로 낸 제출은 main에 들어도 안 적음" '[ $rc = 0 ] && printf "%s" "$rout" | grep -Fq "제출만" && ! awk -F"\t" "\$4 == \"l-rb\" && \$5 == \"착지\"" "$EV" | grep -q . || { echo "$rout"; false; }'
for r in $(og for-each-ref --format='%(refname)' refs/merge-queue/ refs/merge-queue-meta/ | grep l-rb) refs/heads/lane/l-rb refs/heads/release; do og update-ref -d "$r"; done
git -C "$W/m/proj" fetch -q origin && git -C "$W/m/proj" checkout -q --detach origin/main
# 중간 10: 켬 표식이 다른 기계면 병합기는 멈춘다(길 안내). 3차 지휘: 표식은 정확성 조건이 아니다 — 검증 중에 켬 표식이 사라져도 main
# push는 main CAS(fast-forward)와 큐 lease로 맞으니 그 착지는 들어가고, 다음 바퀴부터 쉰다(표식 lease는 지웠다 — 3차 높음 1·2)
off_; og update-ref refs/herdr/merger/box2 main
merge
t "켬 표식이 다른 기계: 병합기 멈춤(1)" '[ $mrc = 1 ] && grep -Fq "다른 기계" "$T/merr" || { echo "rc=$mrc"; cat "$T/merr"; false; }'
og update-ref -d refs/herdr/merger/box2; on_
printf '#!/bin/sh\nrm -f "$T/vhook.l-mk"\ngit -C "$O" update-ref -d refs/herdr/merger/box\n' >"$T/vhook.l-mk"; chmod +x "$T/vhook.l-mk"
lane a l-mk mk.txt mk; land a l-mk
lane a l-mk2 mk2.txt mk2; land a l-mk2
merge
t "검증 중 켬 표식이 사라짐: 그 착지는 들어가고 다음 제출은 쉼(75)" '[ $mrc = 75 ] && mainlog | grep -Fxq l-mk && qrefs | grep -q l-mk2 && grep -Fq "꺼짐" "$T/merr" || { echo "rc=$mrc"; cat "$T/merr"; false; }'
on_; merge
t "다시 켜면 넣음" 'mainlog | grep -Fxq l-mk2 && [ -z "$(qrefs)" ]'
# 중간 12: 제출 때의 작업 ID로 착지 표식 refs/hc-landed/<작업 ID>를 main push에 함께
lane a l-job job.txt job
out=$(cd "$W/a/proj" && HC_LANE_JOB=j-777 HC_LANE=l-job "$LANE" land 2>"$T/err"); rc=$?
merge
t "작업 표식: refs/hc-landed/j-777 = 넣은 커밋" '[ "$(og rev-parse -q --verify refs/hc-landed/j-777)" = "$(og rev-parse main)" ] && [ "$(og log -1 --format=%s main)" = l-job ] || { cat "$T/merr"; og for-each-ref refs/hc-landed/; false; }'
# 중간 13: 가지 이름이 못 되는 레인 이름(「-audit」)은 제출하지 않고, 큐에 들어온 그런 항목은 그것만 반송(병합기는 계속)
lane a l-bq bq.txt bq
out=$(cd "$W/a/proj" && HC_LANE=-audit "$LANE" land 2>"$T/err"); rc=$?
t "레인 이름 「-audit」: 제출 안 함" '[ $rc = 1 ] && ! qrefs | grep -q -- -audit && grep -Fq "레인 이름" "$T/err"'
(cd "$W/a/proj" && git push -q origin HEAD:refs/heads/tmp-bq); og update-ref refs/merge-queue/1000000002--audit "$(og rev-parse tmp-bq)"; og update-ref -d refs/heads/tmp-bq
merge
t "큐의 「-audit」: 그 항목만 반송, 병합기 계속(0)" '[ $mrc = 0 ] && og rev-parse -q --verify refs/merge-failed/1000000002--audit >/dev/null && ! qrefs | grep -q -- -audit || { echo "rc=$mrc"; cat "$T/merr"; false; }'
# 중간 15: 다른 기계 레인 터미널(<기계>/<터미널>)에도 알리고, 알림이 끊기면 보낼 함에 두고 다음 바퀴에 다시(사건은 한 번)
lane a l-far far.txt far
out=$(cd "$W/a/proj" && HC_LANE_MACHINE=box2 HC_TERMINAL_ID=t-far HC_LANE=l-far "$LANE" land 2>"$T/err"); rc=$?
: >"$T/hcfail"; merge; rm -f "$T/hcfail"
t "원격 레인 알림 실패: 보낼 함에 남음, 착지 사건은 하나" 'mainlog | grep -Fxq l-far && ls "$W/m/proj/.git/hc-merger/out/" | grep -q l-far && [ "$(awk -F"\t" "\$4 == \"l-far\" && \$5 == \"착지\"" "$EV" | grep -c .)" = 1 ] || { cat "$T/merr"; false; }'
merge
t "원격 레인 알림: 다음 바퀴에 box2/t-far 로 hc send, 사건은 그대로 하나" 'grep -q "^box2/t-far	" "$T/hc.send" && ! ls "$W/m/proj/.git/hc-merger/out/" | grep -q l-far && [ "$(awk -F"\t" "\$4 == \"l-far\" && \$5 == \"착지\"" "$EV" | grep -c .)" = 1 ] || { cat "$T/hc.send"; cat "$T/merr"; false; }'
# 내장 착지의 push 응답 유실(구조 A): main에 들고 큐 표식도 지워졌는데 실패로 돌아오면(lease 판정은 종료 6) 원격이 후보를 담았는지
# 먼저 보고 착지로 — 돌려보내지 않는다
printf '#!/bin/sh\ncase " $* " in *" push --atomic origin "*":refs/heads/main "*) echo x >>"$T/inj"; %s "$@"; echo "fatal: the remote end hung up unexpectedly" >&2; exit 128 ;; esac\nexec %s "$@"\n' "$rg" "$rg" >"$T/fail/git"
lane a l-lost3 lost3.txt lost3; land a l-lost3
rm -f "$T/inj"
mout=$(cd "$W/m/proj" && PATH="$T/fail:$PATH" "$LANE" merger --repo "$W/m/proj" --once 2>"$T/merr"); mrc=$?
# 3차 시험 중간 4: 가짜 git이 실제로 응답 유실을 한 번 넣었는지도 본다(인자 순서가 바뀌어 주입을 건너뛰어도 통과하던 시험)
t "내장 착지 응답 유실: 주입 한 번, 착지 처리(merge-failed 아님)" '[ "$(grep -c . "$T/inj" 2>/dev/null)" = 1 ] && [ $mrc = 0 ] && mainlog | grep -Fxq l-lost3 && [ -z "$(qrefs)" ] && ! og for-each-ref refs/merge-failed/ | grep -q l-lost3 && awk -F"\t" "\$4 == \"l-lost3\" && \$5 == \"착지\"" "$EV" | grep -q . || { echo "rc=$mrc"; cat "$T/merr"; false; }'

# ---- 아스트라 3차(5c2ec47) 지적 — 고치기 전 판에서 실패하는 시험 ----
# 높음 4: 재시작 때 제출 ID 표식(refs/hc-landed/<제출 ID>)으로만 완료를 본다 — 다시 제출해 철회된 옛 제출이 같은 패치로 main에 있어도
# 완료로 치지 않는다(패치 일치 git cherry를 쓰지 않음)
lane a l-wd wd.txt wd; land a l-wd
swd=$(git -C "$W/a/proj" rev-parse HEAD) qwd=$(qrefs | grep l-wd); nwd=${qwd#refs/merge-queue/}
git -C "$W/m/proj" fetch -q origin '+refs/merge-queue-meta/*:refs/hc-merger/meta/*'
mwd=$(git -C "$W/m/proj" rev-parse "refs/hc-merger/meta/$nwd")
(cd "$W/b/proj" && git fetch -q origin && git checkout -q -B tmp-wd origin/main && printf 'wd\n' >wd.txt && git add wd.txt && git commit -qm l-wd-same && git push -q origin HEAD:main)   # 같은 패치가 다른 커밋으로 main에
for r in "$qwd" "refs/merge-queue-meta/$nwd"; do og update-ref -d "$r"; done   # 레인이 다시 제출해 옛 표식이 바뀐(철회된) 꼴
printf '%s %s %s\n%s %s %s %s\n' l-wd "$(og rev-parse main~1)" "$swd" "$qwd" "$swd" l-wd "$mwd" >"$W/m/proj/.git/hc-merger/cur"
merge
t "철회된 옛 제출: 같은 패치가 main에 있어도 착지로 안 침" '[ $mrc = 0 ] && ! awk -F"\t" "\$4 == \"l-wd\" && \$5 == \"착지\"" "$EV" | grep -q . && [ ! -e "$W/m/proj/.git/hc-merger/cur" ] || { echo "rc=$mrc"; cat "$T/merr"; false; }'
og update-ref -d refs/heads/lane/l-wd
# 높음 4(정상 길): main push에 제출 ID 표식이 원자로 함께 실린다
lane a l-sid sid.txt sid; land a l-sid
sidv=$(awk -F"\t" '$4 == "l-sid" && $5 == "제출" { print $7 }' "$EV" | tail -n 1 | cut -d" " -f1)
merge
t "제출 ID 표식: refs/hc-landed/<제출 ID> = 넣은 커밋" '[ -n "$sidv" ] && [ "$(og rev-parse -q --verify "refs/hc-landed/$sidv")" = "$(og rev-parse main)" ] || { echo "sid=$sidv"; og for-each-ref refs/hc-landed/; false; }'
# 높음 5: git이 관리 경로(--git-path)를 못 알려 주면 진행 중인 rebase가 있는지 모르니 제출하지 않는다
printf '#!/bin/sh\ncase " $* " in *" rev-parse --git-path "*) echo "fatal: bad" >&2; exit 128 ;; esac\nexec %s "$@"\n' "$rg" >"$T/fail/git"
lane a l-gp gp.txt gp
out=$(cd "$W/a/proj" && PATH="$T/fail:$PATH" HC_LANE=l-gp "$LANE" land 2>"$T/err"); rc=$?
t "git-path 조회 실패: 제출 안 함(1)" '[ $rc = 1 ] && ! qrefs | grep -q l-gp && grep -Fq "git rev-parse --git-path 실패" "$T/err" || { echo "rc=$rc"; cat "$T/err"; false; }'
land a l-gp; merge
# 높음 6: 후보는 임시 작업 트리에서 — 검증하는 동안에도 병합기 클론의 작업 트리(HEAD·파일)는 그대로다. 앞 실행이 남긴 임시 작업 트리는
# 다음 바퀴가 지운다
hm0=$(git -C "$W/m/proj" rev-parse HEAD)
printf '#!/bin/sh\nrm -f "$T/vhook.l-wt"\nprintf "%%s|%%s\\n" "$(git -C "$W/m/proj" rev-parse HEAD)" "$(git -C "$W/m/proj" status --porcelain | wc -l | tr -d " ")" >"$T/wt.seen"\n' >"$T/vhook.l-wt"; chmod +x "$T/vhook.l-wt"
lane a l-wt wt.txt wt; land a l-wt
mkdir -p "$W/m/proj/.git/hc-merger/wt" && git -C "$W/m/proj" worktree add -q --detach "$W/m/proj/.git/hc-merger/wt/c.left" HEAD && : >"$W/m/proj/.git/hc-merger/wt/c.left/half-written"   # 앞 판 자리
mkdir -p "$W/m/proj.merger-wt" && git -C "$W/m/proj" worktree add -q --detach "$W/m/proj.merger-wt/c-left2" HEAD   # 지금 자리
merge
t "임시 작업 트리: 검증 중 병합기 클론 HEAD 그대로·깨끗함, 남은 작업 트리는 지움" '[ "$(cat "$T/wt.seen")" = "$hm0|0" ] && mainlog | grep -Fxq l-wt && [ "$(git -C "$W/m/proj" worktree list | grep -c .)" = 1 ] || { cat "$T/wt.seen"; git -C "$W/m/proj" worktree list; cat "$T/merr"; false; }'
# 중간 1: 같은 커밋을 다시 land하면(접수 확인이 끊겨 같은 줄을 다시) 새 제출을 만들지 않는다 — 큐에 있으면 그대로, 넣었으면 할 것 없음
lane a l-dup dup.txt dup; land a l-dup
qd0=$(qrefs | grep l-dup)
land a l-dup
t "다시 land: 이미 큐에 있으면 새로 내지 않음(큐 이름 그대로)" '[ $rc = 0 ] && [ "$(qrefs | grep l-dup)" = "$qd0" ] && printf "%s" "$out" | grep -Fq "이미 병합기 큐에 있다" || { echo "$out"; cat "$T/err"; false; }'
(cd "$W/b/proj" && git fetch -q origin && git checkout -q -B tmp-dd origin/main && printf 'dd\n' >dd.txt && git add dd.txt && git commit -qm l-dd-move && git push -q origin HEAD:main)   # main을 움직여 rebase되게
merge
land a l-dup
t "다시 land: 병합기가 rebase해 넣었으면 할 것 없음(새 제출 없음)" '[ $rc = 0 ] && ! qrefs | grep -q l-dup && printf "%s" "$out" | grep -Fq "이미 $(printf main) 에 넣었다" || { echo "$out"; cat "$T/err"; qrefs; false; }'
# 중간 2: 큐 이름은 반송된 이름(refs/merge-failed/)과도 겹치지 않는다
now=$(date -u +%s); lane a l-col col.txt col
for d in 0 1 2 3; do og update-ref "refs/merge-failed/$((now + d))-l-col" main; done
land a l-col
t "큐 이름: 반송된 이름과 겹치지 않음" '[ $rc = 0 ] && qc=$(qrefs | grep l-col) && ! og rev-parse -q --verify "refs/merge-failed/${qc#refs/merge-queue/}" >/dev/null || { qrefs; false; }'
merge
# 중간 3·4: 이미 main에 든 제출의 정리가 실패하면 --once는 75로 쉬고(같은 항목을 쉬지 않고 되풀이하지 않음), 정리되면 그 제출의 작업
# 표식(refs/hc-landed/<작업 ID>)도 남긴다
lane a l-ear ear.txt ear
out=$(cd "$W/a/proj" && HC_LANE_JOB=j-ear HC_LANE=l-ear "$LANE" land 2>"$T/err"); rc=$?
sear=$(git -C "$W/a/proj" rev-parse HEAD); og update-ref refs/heads/main "$sear"   # 병합기 밖에서 먼저 들어감
printf '#!/bin/sh\ncase " $* " in *" push "*refs/merge-done/l-ear*) echo "fatal: unable to access" >&2; exit 128 ;; esac\nexec %s "$@"\n' "$rg" >"$T/fail/git"
mout=$(cd "$W/m/proj" && PATH="$T/fail:$PATH" perl -e 'alarm 60; exec @ARGV' "$LANE" merger --repo "$W/m/proj" --once 2>"$T/merr"); mrc=$?
t "이미 든 제출 정리 실패: --once 75(되풀이 없음), 큐 그대로" '[ $mrc = 75 ] && qrefs | grep -q l-ear || { echo "rc=$mrc"; cat "$T/merr"; false; }'
merge
t "이미 든 제출 정리: 작업 표식 refs/hc-landed/j-ear = 그 제출" '[ "$(og rev-parse -q --verify refs/hc-landed/j-ear)" = "$sear" ] && ! qrefs | grep -q l-ear || { og for-each-ref refs/hc-landed/; cat "$T/merr"; false; }'
# 중간 7: 착지 뒤 정리할 때 제출 기록을 못 읽으면(git log 실패 — 고를 때는 읽혔다) 제출 ID 없이 끝내지 않고 쉰다
printf '#!/bin/sh\ncase " $* " in *" log -1 --format=%%B "*) [ ! -e "$T/mmfail" ] || { echo "fatal: bad object" >&2; exit 128; } ;; esac\nexec %s "$@"\n' "$rg" >"$T/fail/git"
printf '#!/bin/sh\nrm -f "$T/vhook.l-mm"\n: >"$T/mmfail"\n' >"$T/vhook.l-mm"; chmod +x "$T/vhook.l-mm"
lane a l-mm mm.txt mm; land a l-mm
mout=$(cd "$W/m/proj" && PATH="$T/fail:$PATH" "$LANE" merger --repo "$W/m/proj" --once 2>"$T/merr"); mrc=$?
t "제출 기록 못 읽음: 착지·반송 없이 쉼(75)" '[ $mrc = 75 ] && mainlog | grep -Fxq l-mm && ! awk -F"\t" "\$4 == \"l-mm\" && (\$5 == \"착지\" || \$5 == \"막힘\")" "$EV" | grep -q . || { echo "rc=$mrc"; cat "$T/merr"; false; }'
rm -f "$T/mmfail"; merge
t "제출 기록 읽히면: 착지 사건에 제출 ID" 'awk -F"\t" "\$4 == \"l-mm\" && \$5 == \"착지\"" "$EV" | grep -Fq "제출 ID"'
# 중간 8: main push 전송만 끊기고(원격은 안 받음) 큐 lease가 그대로면 일시 실패 — 한 번 만에 반송하지 않는다
printf '#!/bin/sh\ncase " $* " in *" push --atomic origin "*":refs/heads/main "*) [ -e "$T/pf" ] && { rm -f "$T/pf"; echo "fatal: unable to access" >&2; exit 128; } ;; esac\nexec %s "$@"\n' "$rg" >"$T/fail/git"
lane a l-pf pf.txt pf; land a l-pf
: >"$T/pf"
mout=$(cd "$W/m/proj" && PATH="$T/fail:$PATH" "$LANE" merger --repo "$W/m/proj" --once 2>"$T/merr"); mrc=$?
t "push 전송 끊김: 반송하지 않고 큐에 둠(75)" '[ $mrc = 75 ] && qrefs | grep -q l-pf && ! og for-each-ref refs/merge-failed/ | grep -q l-pf || { echo "rc=$mrc"; cat "$T/merr"; false; }'
merge
# 중간 12: 알림은 최상위 name·state live로 본다 — 레인 이름이 다른 터미널의 foreground에만 있으면 보내지 않는다
lane a l-fg fg.txt fg
out=$(cd "$W/a/proj" && HC_TERMINAL_ID=t-fg HC_LANE=l-fg "$LANE" land 2>"$T/err"); rc=$?
merge
t "알림: foreground 이름만 맞는 터미널에는 hc send 안 함" 'mainlog | grep -Fxq l-fg && ! grep -q "^t-fg	" "$T/hc.send" 2>/dev/null || { cat "$T/hc.send"; false; }'
# 중간 13: 레인 이름 규칙은 레인을 띄울 때와 같다 — hc-fill·git 가지 이름이 받는 「_audit」은 제출·착지된다
lane a _audit au.txt au
land a _audit
merge
t "레인 이름 「_audit」: 제출·착지" '[ $rc = 0 ] && mainlog | grep -Fxq _audit || { echo "rc=$rc"; cat "$T/err"; cat "$T/merr"; false; }'
# 시험 중간 1: 먼저 낸 것이 먼저 — 이름이 사전순으로 뒤인 레인을 먼저 내도 그 순서로 넣는다
lane a l-zz zz.txt zz; land a l-zz; sleep 1
lane b l-aa aa.txt aa; land b l-aa
merge
t "순서: 먼저 낸 l-zz 다음 l-aa(이름순 아님)" '[ "$(mainlog | head -n 2 | paste -sd , -)" = l-aa,l-zz ] || { mainlog | head -n 3; false; }'
# 시험 중간 3: 작업 표식은 rebase해 넣은 커밋을 가리키고, 표식 push가 거절되면(그새 다른 값) main도 그대로
printf '#!/bin/sh\nrm -f "$T/vhook.l-jr"\nd=$(mktemp -d "$T/jr.XXXXXX") && git clone -q "$O" "$d/c" && cd "$d/c" && printf "jr\\n" >jr-other.txt && git add jr-other.txt && git commit -qm l-jr-other && git push -q origin main\n' >"$T/vhook.l-jr"; chmod +x "$T/vhook.l-jr"
lane a l-jr jr.txt jr
out=$(cd "$W/a/proj" && HC_LANE_JOB=j-jr HC_LANE=l-jr "$LANE" land 2>"$T/err"); rc=$?
sjr=$(git -C "$W/a/proj" rev-parse HEAD)
merge
t "작업 표식: rebase해 넣은 커밋(원본 아님)" '[ "$(og rev-parse -q --verify refs/hc-landed/j-jr)" = "$(og rev-parse main)" ] && [ "$(og rev-parse refs/hc-landed/j-jr)" != "$sjr" ] && [ "$(og log -1 --format=%s main)" = l-jr ] || { og for-each-ref refs/hc-landed/; mainlog | head -n 3; false; }'
lane a l-jx jx.txt jx
out=$(cd "$W/a/proj" && HC_LANE_JOB=j-jx HC_LANE=l-jx "$LANE" land 2>"$T/err"); rc=$?
printf '#!/bin/sh\nrm -f "$T/vhook.l-jx"\ngit -C "$O" update-ref refs/hc-landed/j-jx "$(git -C "$O" commit-tree "$(git -C "$O" hash-object -t tree -w /dev/null)" -m other)"\n' >"$T/vhook.l-jx"; chmod +x "$T/vhook.l-jx"
mj0=$(og rev-parse main)
mout=$(cd "$W/m/proj" && perl -e 'alarm 120; exec @ARGV' "$LANE" merger --repo "$W/m/proj" --once 2>"$T/merr"); mrc=$?
t "작업 표식 push 거절: main 그대로·큐 그대로" '[ "$(og rev-parse main)" = "$mj0" ] && qrefs | grep -q l-jx || { echo "rc=$mrc"; cat "$T/merr"; false; }'
og update-ref -d refs/hc-landed/j-jx; merge

# 16. 끄기: 원격 참조를 지우면 다음 land부터 예전처럼 착지
off_
lane a l-off2 off2.txt off2
land a l-off2
t "끄기: 표식을 지우면 착지 그대로" '[ $rc = 0 ] && [ "$(mainlog | head -n 1)" = l-off2 ] && ! qrefs | grep -q l-off2 || { cat "$T/err"; false; }'

# 2단계(2026-11-20 지휘 t1cj): 전용 착지 스크립트(.claude/herdr-land.sh)가 있는 저장소도 받는다 — 착지는 그 스크립트의 기본 브랜치 판.
# 기본 브랜치 판 = 큐 lease를 싣는 진짜 꼴(HERDR_LAND_PUSH_EXTRA를 main push에 --atomic), 레인이 고친 판 = 표시만 남기고 아무것도 안 함
export O
cat >"$T/land-main.sh" <<'EOF'
#!/bin/bash
echo "LAND_MAIN $(git rev-parse --abbrev-ref HEAD)" >>"$T/landscript.log"
git fetch -q origin && git rebase -q "origin/$1" || exit 3
pa=("HEAD:refs/heads/$1"); for e in ${HERDR_LAND_PUSH_EXTRA-}; do pa+=("$e"); done
HERDR_LAND=1 git push -q --atomic origin "${pa[@]}"
EOF
printf '#!/bin/bash\necho "LAND_LANE $(git rev-parse --abbrev-ref HEAD)" >>"$T/landscript.log"\n' >"$T/land-lane.sh"
# opush <저장소 안 경로> <원본 파일>: 새 클론에서 main에 그 파일 하나를 커밋해 push(시험 밖 손 착지)
opush() { (d=$(mktemp -d "$T/op.XXXXXX") && git clone -q "$O" "$d/c" && cd "$d/c" && mkdir -p "$(dirname "$1")" && cp "$2" "$1" && git add -- "$1" && git commit -qm "o-$1" && git push -q origin main); }
on_
# 기본 브랜치에 없는 착지 스크립트를 제출이 새로 들이면 반송 — 그 스크립트는 돌지 않는다
(cd "$W/a/proj" && git fetch -q origin && git checkout -q -B l-add origin/main && cp "$T/land-lane.sh" .claude/herdr-land.sh && git add .claude/herdr-land.sh && git commit -qm l-add)
land a l-add
merge
t "기본 브랜치에 없는 착지 스크립트를 들이는 제출: 반송, 그 스크립트는 안 돎" '[ $mrc = 0 ] && og for-each-ref refs/merge-failed/ | grep -q l-add && ! mainlog | grep -Fxq l-add && ! grep -qs LAND_LANE "$T/landscript.log" && awk -F"\t" "\$4 == \"l-add\" && \$5 == \"막힘\"" "$EV" | grep -Fq "들이는 제출" || { echo "rc=$mrc"; cat "$T/merr"; false; }'

# 검증하는 사이 기본 브랜치에 착지 스크립트가 생기면 그 바퀴는 일시 실패(종료 7 — 반송 아님), 다음 바퀴가 기본 브랜치 판으로 넣는다
printf '#!/bin/sh\nrm -f "$T/vhook.l-nh"\nd=$(mktemp -d "$T/nh.XXXXXX") && git clone -q "$O" "$d/c" && cd "$d/c" && cp "$T/land-main.sh" .claude/herdr-land.sh && git add .claude/herdr-land.sh && git commit -qm o-land && git push -q origin main\n' >"$T/vhook.l-nh"; chmod +x "$T/vhook.l-nh"
lane a l-nh nh.txt nh; land a l-nh
merge
t "검증 중 기본 브랜치에 생긴 착지 스크립트: 그 바퀴는 일시 실패(75), 큐에 그대로, 반송 아님" '[ $mrc = 75 ] && qrefs | grep -q l-nh && ! og for-each-ref refs/merge-failed/ | grep -q l-nh && ! mainlog | grep -Fxq l-nh || { echo "rc=$mrc"; cat "$T/merr"; false; }'
merge
t "다음 바퀴: 기본 브랜치 판 착지 스크립트로 넣음(큐 lease·제출 ID 표식 함께)" '[ $mrc = 0 ] && mainlog | grep -Fxq l-nh && grep -Fxq "LAND_MAIN l-nh" "$T/landscript.log" && [ -z "$(qrefs)" ] && og rev-parse -q --verify refs/merge-done/l-nh >/dev/null && [ -n "$(og for-each-ref refs/hc-landed/)" ] || { echo "rc=$mrc"; cat "$T/merr"; false; }'

# 레인이 착지 스크립트를 고친 제출: 기본 브랜치 판으로 넣는다(제출 트리 속 판은 안 돎), 고친 판은 내용으로 들어간다
: >"$T/landscript.log"
(cd "$W/a/proj" && git fetch -q origin && git checkout -q -B l-hs origin/main && cp "$T/land-lane.sh" .claude/herdr-land.sh && git add .claude/herdr-land.sh && git commit -qm l-hs)
land a l-hs
merge
t "착지 스크립트를 고친 제출: 기본 브랜치 판으로 착지, 레인 판은 안 돎" '[ $mrc = 0 ] && mainlog | grep -Fxq l-hs && grep -Fxq "LAND_MAIN l-hs" "$T/landscript.log" && ! grep -q LAND_LANE "$T/landscript.log" && [ -z "$(qrefs)" ] || { echo "rc=$mrc"; cat "$T/merr"; cat "$T/landscript.log"; false; }'
t "착지 스크립트 길도 착지 뒤 레인 표식 refs/hc-landed/<레인>/<초> = main에 든 커밋(fill-dup-guard)" 'r=$(og for-each-ref --format="%(refname) %(objectname)" "refs/hc-landed/l-hs/" | head -n 1); [ -n "$r" ] && og merge-base --is-ancestor "${r#* }" main || { og for-each-ref refs/hc-landed/; false; }'
opush .claude/herdr-land.sh "$T/land-main.sh"   # 다음 시험을 위해 기본 브랜치 판으로 되돌림

# 새 임시 작업 트리마다 준비 훅(.claude/herdr-setup.sh)을 먼저 — 메인 폴더(LANE_REPO)는 병합기 클론, 코드 색인 도구는 PATH에서 빠짐.
# 실패하면 일시 실패(큐에 그대로)
printf '#!/bin/bash\necho "$LANE_REPO $LANE_BASE cbm=$(command -v codebase-memory-mcp || echo none)" >>"$T/setup.log"\nprintf "%%s\\n" "$PWD" >"$T/setup.pwd"\n[ ! -e "$T/setupfail" ]\n' >"$T/setup.sh"
opush .claude/herdr-setup.sh "$T/setup.sh"
mkdir -p "$T/cbm" && printf '#!/bin/sh\nexit 0\n' >"$T/cbm/codebase-memory-mcp" && chmod +x "$T/cbm/codebase-memory-mcp"
touch "$T/setupfail"
lane a l-su su.txt su; land a l-su
PATH="$T/cbm:$PATH" merge
mtop=$(cd "$W/m/proj" && pwd -P)
t "준비 훅 실패: 일시 실패(75), 큐에 그대로, 착지 안 함" '[ $mrc = 75 ] && qrefs | grep -q l-su && ! mainlog | grep -Fxq l-su && grep -q "^$mtop main " "$T/setup.log" || { echo "rc=$mrc"; cat "$T/merr"; cat "$T/setup.log"; false; }'
# 2026-11-20 한 저장소의 병합기 첫 착지: 후보가 .git/hc-merger/wt/.train-c.* 라 Vite가 브라우저 점검 파일을 403(fs.deny **/.git/**·점 폴더)
sp=$(cat "$T/setup.pwd")
t "후보 작업 트리: 클론 옆 <클론>.merger-wt/ 아래 — 경로에 .git 마디·점으로 시작하는 마디 없음, 준비 훅에 색인 도구 안 보임" 'case $sp in "$mtop.merger-wt/"*) ;; *) false ;; esac && case "/${sp#"$(dirname "$mtop")"/}" in */.*) false ;; esac && tail -n 1 "$T/setup.log" | grep -Fq "cbm=none" || { cat "$T/setup.log"; false; }'
rm -f "$T/setupfail"
merge
t "준비 훅 성공: 착지" '[ $mrc = 0 ] && mainlog | grep -Fxq l-su && [ -z "$(qrefs)" ] || { echo "rc=$mrc"; cat "$T/merr"; false; }'

# 제출 내용이 이미 기본 브랜치에 있으면(같은 변경이 다른 커밋으로 먼저 들어감) 준비·검증 없이 정리만(지휘 t1cj)
lane a l-dup same-dup.txt samedup; land a l-dup
printf 'samedup\n' >"$T/dup.src"; opush same-dup.txt "$T/dup.src"
: >"$T/setup.log"; v0=$(grep -c . "$T/verify.log")
merge
t "이미 든 내용의 제출: 검증·준비 없이 큐 정리, merge-done, 착지 사건 「이미」" '[ $mrc = 0 ] && [ -z "$(qrefs)" ] && og rev-parse -q --verify refs/merge-done/l-dup >/dev/null && [ "$(grep -c . "$T/verify.log")" = "$v0" ] && [ ! -s "$T/setup.log" ] && awk -F"\t" "\$4 == \"l-dup\" && \$5 == \"착지\"" "$EV" | grep -Fq "합친 트리가 같음" || { echo "rc=$mrc"; cat "$T/merr"; false; }'

# 격리(2026-11-20 지휘 t1cj): 바깥 병합기 잠금을 fd 19로 물려받아 쥔 채 돌려도 병합기는 시험 HOME 아래 자기 잠금을 쓰고 끝난다
off_; og update-ref refs/herdr/merger/box main
perl -e 'use Fcntl qw(:flock); open(my $f, ">>", $ARGV[0]) or exit 1; flock($f, LOCK_EX) or exit 1; sleep 60' "$T/outer.lock" & op=$!
sleep 0.3
lane a l-iso iso.txt iso; land a l-iso
mout=$(cd "$W/m/proj" && exec 19>>"$T/outer.lock" && perl -e 'alarm 120; exec @ARGV' "$LANE" merger --repo "$W/m/proj" --once 2>"$T/merr"); mrc=$?
kill "$op" 2>/dev/null; wait "$op" 2>/dev/null
t "격리: 바깥 잠금 fd 19를 쥔 채여도 병합기는 시험 HOME 아래 자기 잠금으로 큐를 처리(잠금에 안 막힘)" '[ $mrc = 0 ] && [ -z "$(qrefs)" ] && ! grep -Fq "아직 돈다" "$T/merr" && ls "$T"/.local/state/hc-lane/merger.*.lock >/dev/null 2>&1 || { echo "rc=$mrc"; cat "$T/merr"; false; }'
t "격리: 시험은 T 밖 저장소를 못 찾는다(GIT_CEILING_DIRECTORIES), 물려받은 fd 19는 닫힘" '! git -C "$T" rev-parse --git-dir >/dev/null 2>&1 && ! { : >&19; } 2>/dev/null'

echo "${n}개 중 실패 ${fails}개"
[ $fails = 0 ]
