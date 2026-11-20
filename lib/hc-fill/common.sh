# hc-fill 함수: common (bin/hc-fill가 읽는다. 함수는 bin/hc-fill 안 원래 정의 순서 그대로)

die() { printf 'hc-fill: %s\n' "$*" >&2; exit "${DRC:-1}"; }   # add --then은 DRC=2(거절)
refuse() { printf 'hc-fill: %s\n' "$*" >&2; exit 2; }
usage() {
  cat >&2 <<'EOF'
사용법: hc-fill add --name <레인> --project <프로젝트> --brief <지시문 경로> [--after <레인>[,…]] [--only <기계>[,…]] [--coord <지휘 주소>]
               [--priority <긴급|높음|보통|낮음>] [--no-early] [--no-audit] [--overlap-ok | --solo]   (우선은 영어 urgent|high|normal|low도 받음, 기본 보통)
        hc-fill add --name <이름> --project <프로젝트> --brief <지시문 경로> --then <대기 중인 선행>
        hc-fill ls | rm <레인> | prio <레인> <우선순위> | run [--dry-run] | watch [--every <초, 기본 60>]
  add    대기열 끝에 한 줄. 프로젝트·기계는 설정에 있는 것만
         지시문은 이때 내용을 ~/.local/share/hc-fill/briefs/added/<레인>.md로 복사한다(못 읽거나 비었으면 거절)
         주인 지휘: --coord, 없으면 부른 터미널($HC_TERMINAL_ID). hc-lane coord --set으로 지휘 정보를 남기고 띄울 때 레인 주인
         (HC_LANE_OWNER)으로 넘겨 그 레인의 사건(hc-lane report·멈춤)은 그 지휘 watch에게만 간다. 지휘 터미널이 아니면(레인 안·
         터미널 밖) 주인 없이(-) 넣고 한 줄 — 그 사건은 모든 지휘에게. --coord가 지휘가 아니면 거절
         --no-early: 선행 착지 전 「먼저 띄움」을 끈다(선행의 산출물 없이는 준비조차 못 하는 레인)
         --no-audit: 자동 감사(설정 audit)를 붙이지 않는다(지시문이 자기 감사를 정한 레인)
         --overlap-ok: 이 줄은 겹침(설정 overlap)을 보지 않는다
         --solo: 단독 — 같은 프로젝트에 다른 쓰기 소유 레인이 있으면 기다리고, 떠 있는 동안 그 프로젝트의 다른 쓰기 줄을 띄우지 않는다
         (파일을 가로지르는 이름 바꾸기·sweep 레인). 설정 overlap과 상관없이. --overlap-ok와 함께 못 씀
         --then <선행>: 같은 일의 다음 단계는 새 줄 없이 대기(wait) 중인 선행 줄 지시문 끝에 「## 이어서: <이름>」 절로 붙인다
         (넘길 때마다 맥락이 새고 숨은 결정이 부딪힌다 — 한 레인이 이어서). 그 절의 커밋 제목 머리는 <이름>: 이라 --after <이름>도 풀린다.
         선행이 없음·떠 있음·받은편지·감사 줄·다른 프로젝트, --after·--only·--priority·--no-early·--no-audit·--coord·--overlap-ok·--solo와 함께면 거절(종료 2).
         도는 run이 끝나기를 기다렸다가(run.lock) 붙인다. 겹치는 파일 때문에 순서만 필요한 일은 그대로 --after
         --after에 대기 중인 같은 프로젝트 레인을 주면 넣기는 그대로 하고 --then을 알리는 한 줄
  prio   대기 중인 레인의 우선순위를 바꾼다(떠 있는 레인은 거절)
  ls     레인·프로젝트·상태·주소·주인 지휘(-는 모든 지휘)·우선·선행·비고(이어서 <--then 이름>,…·자동 감사: 감사 중·감사 통과 보냄·감사 지적 n번째·지휘로 넘김·
         겹침 대기(<앞 레인>: <키>|단독)·겹침 봄 끔·단독)
  run    한 바퀴: (설정 inbox 줄이 있으면) 그 기계 받은편지를 걷어 대기열에 넣고, 끝난 받은편지 레인의 결과를 되돌려 쓴 뒤,
         대기 레인을 우선순위 높은 것부터(긴급>높음>보통>낮음, 같으면 그 줄을 --after로 기다리는 줄이 많은 것 — 감사 줄은 그 레인 몫 —,
         그다음 넣은 차례) 보며(기다리는 줄 수는 레인 환경 HERDR_LAND_PRIORITY로 — 착지 스크립트가 착지 줄 차례에 쓴다), --after 레인이 모두 local 저장소 origin/<기본> 커밋 제목 머리에
         들어갔으면(안 풀린 것은 건너뛰고 다음 것을 본다) 설정의 machine 차례에서 처음 자리가 남는 기계에 띄운다. 띄운 뒤 HC_FILL_WAIT초(기본 60) 안에
         hc get status가 안 잡히면 그 기계를 그 레인에서 빼고 다음 기계로 간다(그 터미널은 끄지 않고 ls 비고에 남긴다).
         남긴 터미널이 나중에 살아나면 「중복 위험」을 찍고 그 레인은 더 띄우지 않는다.
         자리: working·blocked는 늘 셈. done·idle은 작업 트리(<저장소>.wt/…)가 남은 쓰기 레인만 세되, 레인 이름이 그 프로젝트
         origin/<기본> 커밋 제목 머리에 있으면(착지함) 또는 설정 hold분(기본 60, 0이면 끔) 넘게 쉬었으면 안 센다(셈 all 기계는 다 셈)
         먼저 띄움: 그러고도 자리가 남으면 --after에만 묶인 대기 레인(--no-early 빼고, 선행 가운데 대기열에서 아직 안 뜬 레인이
         있는 것도 빼고 — 그 선행이 자리를 먼저)을 같은 차례로 띄운다 — run 지시문 끝에
         「먼저 띄움」 절(준비만 → hc-lane park로 가지를 원격 lane/<레인>에 두고 끝냄 — 원격 없는 저장소만 판정 줄로 기다림, 상한
         설정 early_wait시간)을 붙이고. 「주차」를 적은 줄은 대기(parked)로 되돌려, 선행이 들어가면 hc-lane --resume으로 그 가지에서
         「이어 가기」 절을 붙여 다시 띄운다
         ls에 「먼저 띄움(선행 …)」. 살아 있는 먼저 띄움 레인 수 상한은 설정 early(빈 값이면 남은 자리 전부, 0이면 끔).
         기다림 블록의 기한 뒤에는 hc-lane early-wait가 선행 상태(hc-fill이 바퀴마다 그 레인 기계 ~/.local/share/hc-fill/after.tsv에 씀)를 보고
         선행이 살아 진행 중이면 계속 기다린다(막힘·끝남·사라짐·모름이면 멈춤)
         기계 능력: 지시문 「필요: <능력>, …」 줄이 있으면 설정 machine-has <기계> <능력,…>에 다 있는 기계에만. 하나도 없으면 대기로 두고
         주인 지휘에 막힘 한 번(ls 「필요 …」). 빈 지시문: 부록 절(착지·정리 등) 앞에 본문 줄이 없으면 add·받은편지 거절, run은 안 띄움
         놀고 있음: 바퀴 끝에도 자리가 남은 기계가 설정 idle분(기본 10, 0이면 끔) 넘게 이어지면 hc-lane report --name hc-fill-idle
         「<기계> 자리 n개 m분째 빔 — 대기열에 띄울 것 없음」 사건을 대기 줄 주인 지휘들(살아 있는 것, 주인 없는 줄이 있거나
         대기 줄이 없으면 모든 지휘)의 watch에 보낸다. 대기열에 띄울 것 없음이면 비기 시작한 뒤 한 번만(자리가 하나라도 찼다가
         다시 비면 또 한 번). 먼저 띄움 상한 사유면 계속 비는 동안 처음 알림 포함 30분 간격으로 세 번, 그 뒤 2시간마다. 사유가 바뀌면 바로 다시.
         먼저 띄움 상한에 걸려 남긴 자리면 사유를 「먼저 띄움 상한(early n)에 걸려 남김」으로
         자동 감사(설정 audit = 감사 지시문 틀): 쓰기 레인(받은편지·--no-audit 줄 빼고) run 지시문 끝에 「감사·착지 (hc-fill 자동)」 절.
         레인이 hc-lane report 완료 <sha>를 하면 그 기계에 긴급 감사 줄 audit-<레인>(hc-lane --read, 작업 트리에서)을 넣고,
         감사의 판정(「감사 완료 <sha> 높음 n·중간 n·낮음 n」)이 높음·중간 0이면 레인에 「감사 통과」, 아니면 「감사 지적」을
         hc send(지적은 audit_max번까지, 기본 2). 상한·판정 못 읽음·보내기 세 번 실패는 그 레인 주인 지휘에 hc-lane report 막힘
         겹침(설정 overlap, 기본 block): 대기 줄을 띄우기 전에 같은 프로젝트의 쓰기 소유 레인과 견준다. 쓰기 소유 = 실행 기록
         (~/.local/share/hc-fill/own.tsv): 쓰기 터미널 주소를 받은 때(상태 안 잡힘 포함) 적고, 그 기계가 연결된 채 그 주소에 그 이름
         터미널이 없거나(병합기 큐에 있으면 유지) 그 터미널이 시작 뒤 착지를 적으면 지운다 — 기계가 연결 안 됨이면 유지, hold·rm·sent와 따로.
         예약 = 지시문 「파일:」·「쓰기:」 줄 항목의 첫 경로(새 파일도)·FR ID, 언급 = 본문의 저장소 경로·FR ID. warn: 띄우고 「겹침 경고」
         (overlap-warn.log) · block: 예약끼리 겹치면 「겹침 대기」로 남기고 다음 줄(언급은 경고) · off. 선행 예약은 후속을 막고, 후속(그 선행을
         --after 사슬·--then으로 건너서라도 기다림) 예약은 선행을 안 막음. 감사 줄·--overlap-ok 줄은 안 봄. 설정 overlap_ignore <프로젝트> <glob>… 은 뺀다.
         검사와 띄우기는 바퀴마다 뜬 지시문 사본 하나로 하고, 그 사이 대기열 줄이 바뀌면 다음 바퀴에
  watch  run을 --every초마다(한 번에 하나만, 이미 돌면 종료 75)
설정: ${HERDR_TOOLS_CONFIG:-~/.config/herdr-tools}/hc-fill.tsv (칸 설명은 저장소 config.example/hc-fill.tsv)
대기열: ~/.local/share/hc-fill/queue.tsv, run 지시문: 그 기계 ~/.local/share/hc-fill/runs/<레인>-run.md
종료: 0 정상, 1 오류, 2 잘못 부름, 75 다른 run·watch가 돌고 있음
EOF
  exit 2
}

conf() { awk -v k="$1" '$1==k { sub(/^[ \t]*[^ \t]+[ \t]*/, ""); sub(/[ \t]+$/, ""); print; exit }' "$CONF"; }
machines() { awk '$1=="machine" { print $2, $3, ($4 == "" ? "-" : $4), ($5 == "" ? "-" : $5) }' "$CONF"; }   # 차례대로: 이름 상한 레인상한 셈
repo_of() { awk -v p="$1" -v m="$2" '$1=="repo" && $2==p && $3==m { sub(/^[ \t]*repo[ \t]+[^ \t]+[ \t]+[^ \t]+[ \t]+/, ""); sub(/[ \t]+$/, ""); print; exit }' "$CONF"; }
# landed <레인>: FR 저장소 origin/<FB>에서 제목 머리가 그 레인인 커밋 줄(아래 「선행·착지 판정」 규칙). fetch_proj 뒤, ls는 fetch 없이
landed() { git -C "$FR" log "origin/$FB" --format='%h %s' | grep -E -m 1 "^[0-9a-f]+ ([a-z]+(\([^)]*\))?!?: *)?$1([^a-z0-9_-]|\$)"; }

# 우선순위: 한·영 값을 대기열 값으로(모르면 실패), 대기열 값을 ls 글로, 줄 정렬 순번(옛 줄·빈 칸은 보통)
pnorm() { case $1 in 긴급 | urgent) echo urgent ;; 높음 | high) echo high ;; 보통 | normal) echo normal ;; 낮음 | low) echo low ;; *) return 1 ;; esac; }
plabel() { case $1 in urgent) echo 긴급 ;; high) echo 높음 ;; low) echo 낮음 ;; *) echo 보통 ;; esac; }
okname() { case $1 in '' | *[!abcdefghijklmnopqrstuvwxyz0123456789_-]*) return 1 ;; esac; [ ${#1} -le 32 ]; }
# kindwhy <이름>: 커밋 종류 낱말과 같은 이름이면 사유를 찍고 0 — 착지·선행 판정의 「<종류>(<범위>): 」 머리가 이름이 빈 채로도 맞아서
kindwhy() {
  case $1 in feat | fix | docs | style | refactor | perf | test | build | ci | chore | revert) ;; *) return 1 ;; esac
  printf '레인 이름이 커밋 종류(%s)와 같으면 착지·선행 판정이 아무 %s: 커밋에나 맞는다 — 다른 이름을 쓴다' "$1" "$1"
}
inlist() { case ",$2," in *",$1,"*) return 0 ;; esac; return 1; }   # inlist <값> <쉼표 목록>
# 기계 능력(2026-11-19 fill-dep-needs — railway CLI 없는 기계에 웹 배포 레인이 떠 막혔다). needs <지시문>: 「필요: <능력>, …」 줄(목록 머리
# -·* 허용, 능력 이름은 a-z 0-9 _ - 낱말을 쉼표로 — 다른 글이 섞인 줄은 문장으로 보고 안 셈)의 능력을 쉼표 목록으로(여러 줄이면 합침)
needs() {
  sed -n -E 's/^[[:space:]]*([-*][[:space:]]*)?필요:[[:space:]]*//p' "$1" 2>/dev/null |
    awk '{ g = $0; gsub(/[ \t]*,[ \t]*/, ",", g); sub(/[ \t]+$/, "", g); if (g ~ /^[a-z0-9_-]+(,[a-z0-9_-]+)*$/) print g }' |
    tr , '\n' | awk 'NF && !s[$0]++' | paste -sd , -
}
mhas() {  # mhas <기계> <능력 쉼표 목록|빈 값>: 설정 machine-has <기계> <능력,…>(여러 줄이면 합침)에 다 있으면 0
  [ -n "$2" ] || return 0
  hv_=$(awk -v m="$1" '$1=="machine-has" && $2==m { for (i = 3; i <= NF; i++) print $i }' "$CONF" | tr , '\n')
  for c_ in $(printf %s "$2" | tr , ' '); do printf '%s\n' "$hv_" | grep -Fxq -- "$c_" || return 1; done
}
# brief_bad <지시문>: 실제로 열어 읽어 본다(launchd 프로그램은 TCC로 ~/Documents가 stat은 되고 읽기만 막힌다).
# 못 읽거나 비었으면 까닭을 찍고 0, 읽히면 1
brief_bad() {
  [ -f "$1" ] || { echo 없음; return 0; }
  e=$(cat -- "$1" 2>&1 >/dev/null) || { e=${e##*: }; echo "${e:-읽기 실패}"; return 0; }
  grep -q '[^[:space:]]' -- "$1" || { echo "빈 파일"; return 0; }   # 공백·줄바꿈만 든 것도 빈 것
  # 본문 없음(2026-11-16 travel-ta: 본문 없이 착지 부록만 든 run 지시문으로 레인이 떴다): 첫 부록 절(hc-lane·hc-fill이 붙이는 착지·정리·
  # 감사·착지·먼저 띄움·결과 보고) 앞에 제목(#) 아닌 글 줄이 없으면
  awk '/^#+[ \t]*(착지·정리|감사·착지|먼저 띄움|결과 보고)/ { a = 1; exit } /[^[:space:]]/ && !/^[ \t]*#/ { f = 1; exit } END { exit (a && !f) }' <"$1" ||
    { echo "본문 없음(제목·부록 절만)"; return 0; }
  return 1
}

# 대기열 쓰기는 모두 이 잠금 안에서(add·rm·run의 상태 바꿈이 서로 덮지 않게)
qlock() { mkdir -p "$DATA" && HC_FILL_Q=1 lockf -k -t 30 "$DATA/queue.lock" "$0" "$@"; }
qset() {  # qset <레인> <상태> <기계> <주소> <남긴것> [<먼저>] [<감사>]: 먼저 칸(14)·감사 칸(15)을 주면 옛 줄의 빈 칸은 -로 채워 거기에
  # 환경 QRID가 있으면 줄 ID(17칸)가 그 값인 줄만(run이 대기열을 읽은 뒤 rm·다시 add된 새 줄을 옛 실행으로 덮지 않게 — 4차 B 높음 7).
  # 바꾼 줄이 없으면 1
  [ "${HC_FILL_Q-}" = 1 ] || { qlock _set "$@"; return; }
  awk -F'\t' -v OFS='\t' -v n="$1" -v s="$2" -v m="$3" -v a="$4" -v k="$5" -v e="${6-}" -v u="${7-}" -v r="${QRID-}" -v hr="${QRID+1}" \
    '($1 "")==n && (hr == "" || ($17 "") == r) {$6=s; $7=m; $8=a; $9=k; f = 1
    if (e != "") { for (i = NF + 1; i < 14; i++) $i = (i == 13 ? "normal" : "-"); $14 = e }
    if (u != "") { for (i = NF + 1; i < 15; i++) $i = (i == 13 ? "normal" : "-"); $15 = u } } 1 END { exit !f }' "$Q" >"$Q.tmp"; qr_=$?
  [ $qr_ -le 1 ] || return 2   # awk가 못 읽음 — 대기열을 그대로 둔다
  mv "$Q.tmp" "$Q" && return $qr_
}
# 줄 ID(17칸): _add·_put이 매긴다(r<초>-<pid>-<난수> — 같은 이름 줄이라도 넣을 때마다 다르다). 16칸까지의 옛 줄은 빈 값
rowid() { echo "r$(date +%s)-$$-$RANDOM"; }
pad16() { ROW=("$@"); while [ ${#ROW[@]} -lt 16 ]; do ROW+=(-); done; ROW+=("$(rowid)"); }

# thenof <이름>: 그 이름이 「## 이어서: <이름> 」으로 붙은 대기열 줄 이름(없으면 빈 줄)
thenof() {
  awk -F'\t' '{print $1 "\t" $3}' "$Q" 2>/dev/null | while IFS=$'\t' read -r tn tb; do
    grep -q "^## 이어서: $1 " -- "$tb" 2>/dev/null && { echo "$tn"; break; }
  done
}
