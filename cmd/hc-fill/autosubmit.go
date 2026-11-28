package main

// 대기열이 제출(2026-11-24 fill-auto-submit, docs/hc-fill/audit.md 「대기열이 제출」): 감사 통과 판정을 둔 바퀴에 레인 터미널이 쉬고
// 있으면(턴 중이 아님), 에이전트를 깨우는 대신 hc-fill이 그 레인 작업 트리에서 레인이 부를 두 줄을 그대로 돌린다 —
// hc-lane verdict <ID> → 출력이 「감사 통과」면 hc-lane land <기본> --verdict <ID>(성공이면 hc-lane report 착지 <sha8>).
// 환경은 레인 것이다 — 레인 터미널 argv의 「env K=V …」 앞머리 그대로(asEnvOf)와 레인 자신의 HC_TERMINAL_ID(기계 접두 없음)라
// 판정 확인(그 실행·그 sha·그 차수)·제출 기록·사건이 레인이 돌린 것과 같다.
// 정확히 한 번: 수거 표시 <ID>.ack는 먼저 만든 쪽만 이기고, 대기열이 만든 시작 표시는 다섯째 칸 fill(HC_LANE_LAND_BY=fill —
// cmd/hc-lane/landby.go). 에이전트의 verdict·land는 그 표시를 보면 손대지 않고, 대기열은 에이전트 표시를 보면 손대지 않는다.
// 끝내지 못하면(충돌·사전 검사·그 밖 종료 ≠ 0·시간 넘김·모름) 표시의 fill 칸을 지워(원자 rename) 에이전트에 넘기고 지금처럼
// 깨운다 — 무엇을 했고 어디서 멈췄는지 한 줄과 함께. 프로젝트마다 손잡이 auto_submit(기본 끔), 시간 상한 auto_submit_max.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/rest"
)

// asubAud: status 감사 비고(anote) — 대기열이 레인 대신 제출한 차수
const asubAud = "감사 통과 — 대기열이 제출함"

// stStage: 단계 한 마디. 대기열이 제출한 레인은 병합기 줄 단계 뒤에도 「대기열이 제출함」을 붙인다(착지했으면 안 붙인다 — 끝났다)
func stStage(in stIn) string {
	s := stStage0(in)
	if in.Aud == asubAud && s != asubAud && !strings.HasPrefix(s, "착지") {
		s += " · 대기열이 제출함"
	}
	if in.Mode == mStacked && in.Lane != "" && !strings.HasPrefix(s, "착지") { // stack.go
		s += " · " + stackNote(in.Lane)
	}
	return s
}

// asNote: 넘길 때 깨우기 글에 붙이는 한 줄(이 차수 이 바퀴). 비어 있지 않으면 깨우기 간격을 기다리지 않는다
var asNote string

// asRest: 쉼 판정이 쉼이 아니라 대신 제출하지 않은 까닭 — 깨우기 글에 붙이되 깨우기 간격은 그대로 지킨다(바퀴마다 다시 정해진다)
var asRest string

// asubOn <프로젝트>: 손잡이·설정 auto_submit 목록에 그 프로젝트가 있나(기본 「-」 = 끔). 바퀴마다 새로 읽어 바로 먹는다
func asubOn(p string) bool {
	return p != "" && inlist(p, strings.ReplaceAll(conf("auto_submit"), " ", ""))
}

// asubMax: 한 번의 시간 상한(초, 10~600, 기본 120)
func asubMax() time.Duration {
	n := atoi10(conf("auto_submit_max"))
	if n < 10 || n > 600 {
		n = 120
	}
	return time.Duration(n) * time.Second
}

// s_asub <작업 트리> <레인> <aid> <기본> <터미널> <sha8> [<레인 환경 K=V>…](d = 판정 폴더): 물려받은 레인 값(HC_LANE*·LANE_*·
// HERDR_LAND_PRIORITY — hc-fill·코어 환경에 남은 것)을 지우고 레인 환경으로 두 줄을 돌리고
// 단계마다 「ASUB <단계> <종료> <표시>」(표시 = none·taken·landing·void, 대기열 시작이면 +fill)
const sAsub = `x=$d/$3.ack; w=$1 aid=$3 b=$4 s8=$6
mk() { if [ -f "$x" ]; then a=$(head -n 1 "$x" | cut -f 1); [ "$(head -n 1 "$x" | cut -s -f 5)" != fill ] || a=$a+fill; printf %s "$a"; else printf none; fi; }
cd "$w" 2>/dev/null || { echo "ASUB cd 1 $(mk)"; exit 0; }
for v in $(env | awk -F= '/^(HC_LANE[A-Z0-9_]*|LANE_[A-Z0-9_]*|HERDR_LAND_PRIORITY)=/ { print $1 }'); do unset "$v"; done
export HC_LANE="$2" HC_TERMINAL_ID="$5"
shift 6; for kv; do export "$kv" || exit 1; done
export HC_LANE_LAND_BY=fill HC_FILL_VERDICTS="${d%/*}"
o=$(hc-lane verdict "$aid" 2>&1); r=$?; printf '%s\n' "$o"
case $o in "감사 통과 "*) ;; *) echo "ASUB verdict $r $(mk)"; exit 0 ;; esac
o=$(hc-lane land "$b" --verdict "$aid" 2>&1); r=$?; printf '%s\n' "$o"
if [ $r -eq 0 ]; then p=$(hc-lane report 착지 "$s8" 2>&1); q=$?; [ $q -eq 0 ] || printf '%s\n' "$p"; echo "ASUB report $q $(mk)"; fi
echo "ASUB land $r $(mk)"`

// s_ahand <aid>: 대기열 시작 표시(다섯째 칸 fill)를 넷째 칸까지로 원자 rename — 에이전트에 넘김. 찍는 것 HANDED · KEEP(대기열 것 아님) ·
// NONE · BUSY(그 판정의 hc-lane land가 아직 돈다 — 시간 넘김·hc-fill이 죽은 뒤 남은 land와 에이전트 land가 겹치지 않게). pgrep 오류는 모름(종료 1)
const sAhand = `x=$d/$1.ack; [ -f "$x" ] || { echo NONE; exit 0; }
l=$(head -n 1 "$x"); [ "$(printf '%s\n' "$l" | cut -s -f 5)" = fill ] || { echo KEEP; exit 0; }
pgrep -f "hc-lane land .*--verdict $1\$" >/dev/null 2>&1; case $? in 0) echo BUSY; exit 0 ;; 1) ;; *) exit 1 ;; esac
t=$(mktemp "$d/.hand.XXXXXX") || exit 1
printf '%s\n' "$l" | cut -f 1-4 >"$t" && mv -f "$t" "$x" || { rm -f "$t"; exit 1; }
echo HANDED`

type asStep struct{ rc, mark string }

// asParse: 「ASUB 단계 종료 표시」 줄들과 마지막 다른 줄(멈춘 자리 한 줄)
func asParse(out string) (map[string]asStep, string) {
	st, last := map[string]asStep{}, ""
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		switch {
		case len(f) == 4 && f[0] == "ASUB":
			st[f[1]] = asStep{f[2], f[3]}
		case strings.TrimSpace(l) != "":
			last = strings.TrimSpace(l)
		}
	}
	return st, last
}

// asEnvOf <argv>: 레인 터미널을 띄운 「env K=V … 명령」 앞머리에서 레인 값(HC_LANE*·LANE_*·HERDR_LAND_PRIORITY, HC_LANE_LAND_BY 빼고)만.
// 레인이 받은 값 그대로라 주인 실행(HC_LANE_OWNER_RUN)·착지 차례(HERDR_LAND_PRIORITY)·실행(HC_LANE_JOB, 있었을 때만)이 같다 —
// 병합기는 제출 기록의 job으로 착지 표식을 하나 더 두니, 레인에 없던 값은 만들지 않는다
func asEnvOf(argv []string) []string {
	if len(argv) == 0 || argv[0] != "env" {
		return nil
	}
	var o []string
	for _, kv := range argv[1:] {
		k, _, ok := strings.Cut(kv, "=")
		if !ok || k == "" || strings.Trim(k, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_") != "" {
			break // 명령(또는 env 옵션)부터는 환경이 아니다
		}
		if (strings.HasPrefix(k, "HC_LANE") || strings.HasPrefix(k, "LANE_") || k == "HERDR_LAND_PRIORITY") && k != "HC_LANE_LAND_BY" {
			o = append(o, kv)
		}
	}
	return o
}

// asEnv: 이 차수 레인 터미널(이름·실행이 같음)의 레인 값. 못 읽거나 다른 실행이면 ok=false — 모르면 돌리지 않는다
func asEnv() ([]string, bool) {
	t, found, err := hcGet(auPa)
	if err != nil || !found || t.State != "live" || t.Name != auLn {
		return nil, false
	}
	var argv []string
	if r := t.Raw["argv"]; len(r) == 0 || string(r) == "null" || json.Unmarshal(r, &argv) != nil {
		return nil, false
	}
	env := asEnvOf(argv)
	run := asTerm() + "@" + strconv.FormatInt(t.Created, 10)
	for _, kv := range env {
		if j, ok := strings.CutPrefix(kv, "HC_LANE_JOB="); ok && j != "" {
			run = j
		}
	}
	return env, run == auPrun
}

// asTerm: 레인 자신의 HC_TERMINAL_ID — hc-fill이 본 주소(원격은 「기계/tN」)의 마지막 칸(워커 기계의 hc는 자기 기계 이름을 모른다)
func asTerm() string { return auPa[strings.LastIndexByte(auPa, '/')+1:] }

// asubRun: 레인 기계에서 s_asub(로컬은 sh, 원격은 hc exec --timeout). 종료 ≠ 0(시간 넘김 124 포함)은 모름
func asubRun(wt, base string, env []string) (string, int) {
	max := asubMax()
	a := append([]string{"-c", X + sAsub, "_", "~/.local/share/hc-fill/verdicts/" + auLn, wt, auLn, auAid, base, asTerm(), head(auSha, 8)}, env...)
	if auAm == LOCAL {
		return Cmd{Name: "sh", Args: a, Err: errMerge, Max: max}.Run()
	}
	h := []string{"exec", "--on", auAm, "--timeout", strconv.Itoa(int(max / time.Second)), "--", "sh"}
	return Cmd{Name: "hc", Args: append(h, a...), Err: errMerge, Max: max + 30*time.Second}.Run()
}

// asHand: 대기열 시작 표시를 에이전트 것으로. 참 = 넘김(또는 넘길 것 없음), 거짓 = 모름(다음 바퀴에)
func asHand() bool {
	o, rc := onm(nil, auAm, sAhand, "~/.local/share/hc-fill/verdicts/"+auLn, auAid)
	switch o = sub(o); {
	case rc == 0 && (o == "HANDED" || o == "KEEP" || o == "NONE"):
		return true
	case rc == 0 && o == "BUSY":
		fmt.Println("넘기기 미룸 " + auAid + " → " + auAm + ": 대기열이 시작한 hc-lane land가 아직 돈다, 다음 바퀴에")
		return false
	}
	fmt.Println("넘기기 미룸 " + auAid + " → " + auAm + ": 대기열 시작 표시를 넘기지 못했다(종료 " + strconv.Itoa(rc) + "), 다음 바퀴에")
	return false
}

// asDone <한 줄> <report 종료(실패일 때만)>: 대기열 제출이 끝났다 — 차수 taken(16칸 asubok), 레인에 정리 알림 한 번, 감사 줄 정리.
// report 종료가 빈 값이 아니면 레인에 report 착지를 다시 부르게 한다(앞 바퀴가 도중에 죽었으면 「?」 — 불렀는지 모름)
func asDone(line, rr string) {
	f := strings.TrimPrefix(strings.TrimPrefix(auAfl+",asubok", "-,"), ",")
	aupd(auAid, cv{atState, "taken"}, cv{atFlags, f})
	auAfl = f
	fmt.Println("대기열이 제출 " + auLn + " ← 판정 " + auAid + " (" + head(auSha, 8) + "): " + line)
	msg := "hc-fill: 대기열이 판정 " + auAid + " 를 레인 대신 제출했다 — hc-lane verdict·land 는 하지 않는다. 지시문 정리 절차의 hc-lane clean 줄만 돌리고 끝낸다"
	if rr != "" {
		msg += " (그 앞에 hc-lane report 착지 " + head(auSha, 8) + " 를 부른다 — 대기열의 report 종료 " + rr + ")"
	}
	if !auSend(auPa, msg) {
		fmt.Println("정리 알림 실패 " + auLn + " " + auPa + " — 레인이 쉬는 채로 남는다(지휘가 hc-lane clean)")
	}
	aCleanupNow()
}

// aAutoSubmit <기본 가지>: 판정 파일을 둔 뒤(표시 none) 깨우기 앞에. 참 = 이 바퀴는 끝(제출함·에이전트가 먼저 시작함·넘기기 미룸),
// 거짓 = 지금처럼 깨운다(asNote가 있으면 바로, 그 한 줄과 함께)
func aAutoSubmit(base string) bool {
	asNote, asRest = "", ""
	p := aproj(auAid)
	if auAv != "pass" || inlist("asub", auAfl) || !asubOn(p) || !lalive(auPa, auLn) {
		return false
	}
	switch alive(auPa) {
	case "idle", "done":
	default:
		return false // 에이전트가 턴 중 — 손대지 않는다
	}
	// 턴이 끝났어도 안에서 일이 돌거나(활동) 쉼을 모르면 레인이 손을 뗐는지 모른다(ht-rest-one-gate): 대신 제출하지 않고 턴 중일 때처럼
	// 판정을 전달한다 — 배경 작업이 아직 필요한지는 그 레인만 안다(ht-autosubmit-busy-wake). 깨우기 간격은 그대로라 바퀴마다 되풀이하지 않는다
	if v := rest.Ask(restRun, auPa, LOCAL); v.State != rest.Quiet {
		asRest = "대기열이 대신 제출하지 않았다: 레인 터미널 " + rest.Why(v)
		fmt.Println("대기열 제출 안 함 " + auLn + ": 레인 터미널 " + auPa + " " + rest.Why(v) + " — 판정을 지금처럼 전달한다")
		return false
	}
	repo := repoOf(p, auAm)
	if repo == "" {
		return false
	}
	env, ok := asEnv()
	if !ok {
		fmt.Println("대기열 제출 안 함 " + auLn + ": 레인 터미널 " + auPa + " 의 환경·실행을 확인하지 못했다 — 지금처럼 깨운다")
		return false
	}
	// 시도 표시를 먼저: 도중에 죽어도 다시 돌리지 않는다(다음 바퀴는 표시 landing+fill을 보고 넘긴다)
	auAfl = strings.TrimPrefix(strings.TrimPrefix(auAfl+",asub", "-,"), ",")
	aupd(auAid, cv{atFlags, auAfl})
	out, rc := asubRun(strings.TrimSuffix(repo, "/")+".wt/"+auLn, base, env)
	st, last := asParse(out)
	ld, landed := st["land"]
	end := ld
	if !landed {
		end = st["verdict"]
		if e, ok := st["cd"]; ok {
			end = e
		}
	}
	switch {
	case rc != 0 || len(st) == 0: // 결과 모름 — 성공으로 치지 않는다
		w := "종료 "
		if auAm != LOCAL {
			w = "원격 종료 "
		}
		asNote = "대기열이 제출을 시도했으나 결과 모름(" + w + strconv.Itoa(rc) + ")"
	case landed && ld.rc == "0" && ld.mark == "landing+fill":
		rr := ""
		if r, ok := st["report"]; ok && r.rc != "0" {
			rr = r.rc
		}
		asDone(last, rr)
		return true
	case end.mark != "none" && !strings.HasSuffix(end.mark, "+fill"):
		fmt.Println("대기열 제출 안 함 " + auLn + ": 판정 " + auAid + " 표시가 이미 " + end.mark + "(에이전트가 먼저 받음·취소) — 손대지 않는다")
		return true
	case landed && strings.Contains(last, "의 완료(") && strings.Contains(last, "가 서 있지 않다"):
		// land --verdict가 그 기계의 지금 사건으로 거절(hc-lane stands.go lvStands — 바퀴를 읽은 뒤 레인이 막힘·주차를 적음). 시작 표시 전이라
		// 넘길 것이 없고, 「착지하라」로 깨우지 않는다 — 다음 바퀴의 aGate가 새 사건으로 보류한다(ht-submit-still-stands 감사 낮음 2)
		fmt.Println("대기열 제출 멈춤 " + auLn + ": " + last + " — 깨우지 않고 다음 바퀴에")
		return true
	default:
		step := "verdict"
		if landed {
			step = "land"
		} else if _, ok := st["cd"]; ok {
			step = "작업 트리로 cd"
		}
		if r := []rune(last); len(r) > 160 {
			last = string(r[:160]) + "…"
		}
		asNote = "대기열이 제출을 시도했으나 " + step + " 종료 " + end.rc + "에서 멈췄다: " + last
	}
	if !asHand() {
		return true
	}
	fmt.Println("대기열 제출 넘김 " + auLn + " — " + asNote)
	return false
}
