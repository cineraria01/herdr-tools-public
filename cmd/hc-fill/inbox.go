package main

// 받은편지 걷기·되돌려 주기 — 셸 판 lib/hc-fill/tick.sh의 irej·s_ilist·s_iclaim·inbox_tick·s_ipub·rdone·reply_tick.
// 원격 sh 본문은 셸 판 글자 그대로(원격 기계에는 셸만 있다). onm이 X 머리를 붙인다

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

// inbIrejSh: irej 원격 본문. 셸 판과 다름: 사유(.why)를 먼저 쓴 뒤 .req·.brief.md를 옮겨, 옮겨진 요청에 사유가 늘 함께 있다
const inbIrejSh = `mkdir -p "$d/rejected" && printf "%s\n" "$2" >"$d/rejected/$1.why.tmp" && mv -f "$d/rejected/$1.why.tmp" "$d/rejected/$1.why" &&
    for e in req brief.md; do [ ! -f "$d/$1.$e" ] || mv -f "$d/$1.$e" "$d/rejected/" || exit 1; done`

var reqIDRe = regexp.MustCompile(`^(r[0-9a-z]+-[0-9a-z]{6}|rl[0-9a-f]{8})$`)

// reqIDOK: 받은편지 요청 ID 꼴(newid("r") 또는 옛 줄 rl<cksum 8자리>)
func reqIDOK(id string) bool { return reqIDRe.MatchString(id) }

// s_ilist: 받은편지 이름 목록(dry-run이 아니면 .req 없는 접수 표시를 지움)
const inbSIlist = `if [ "$1" = 0 ]; then for c in "$d"/.hc-fill/claim/*; do [ -f "$c" ] || continue; n=${c##*/}; case $n in *.tmp) continue ;; esac; [ -f "$d/$n.req" ] || rm -f "$c"; done; fi
for f in "$d"/*.req; do [ -f "$f" ] || continue; f=${f##*/}; printf "%s\n" "${f%.req}"; done`

// s_iclaim: 「접수ID 접수.req해시 접수지시문해시 지금.req해시 지금지시문해시」(없는 칸은 -)
const inbSIclaim = `h() { if [ -f "$1" ]; then shasum -a 256 "$1" | cut -d " " -f 1; else echo -; fi; }
c=$d/.hc-fill/claim/$1; r=- cq=- cb=-; [ ! -f "$c" ] || read -r r cq cb <"$c" || :
if [ "$r" != - ] && [ -f "$d/taken/$1.$r.req" ]; then r=- cq=- cb=-; fi
b=$d/$1.brief.md; [ -f "$b" ] || [ "$r" = - ] || b=$d/taken/$1.$r.brief.md
q=$(h "$d/$1.req"); [ "$q" != - ] || exit 3
echo "${r:--} ${cq:--} ${cb:--} $q $(h "$b")"`

// s_ipub: 고정본을 요청자 done/<이름>.md에 한 번만(대상마다 잠금, 크기·sha256 확인). SENT·ALREADY·CONFLICT·EXISTS·BADINPUT
const inbSIpub = `mkdir -p "$d/done/.hc-fill" || exit 1
exec lockf -k -t 30 "$d/done/.hc-fill/$1.lock" sh -c 'd=$1 n=$2 r=$3 h=$4 z=$5 m=$d/done/.hc-fill f=$d/done/$2.md o=$d/done/.hc-fill/owner/$2
 hs() { shasum -a 256 "$1" | cut -d " " -f 1; }
 setowner() { mkdir -p "${o%/*}" && printf "%s\n" "$r" >"$o.tmp" && mv -f "$o.tmp" "$o"; }
 mark() { printf "%s\n" "$h" >"$m/$r.sent.tmp" && mv -f "$m/$r.sent.tmp" "$m/$r.sent"; }
if [ -f "$m/$r.sent" ]; then if [ "$(cat "$m/$r.sent")" = "$h" ]; then echo ALREADY; else echo CONFLICT; fi; exit 0; fi
ready=$m/$r.ready; intent=$m/$r.publishing
if [ -f "$intent" ]; then
  [ "$(cat "$intent")" = "$h" ] || { echo CONFLICT; exit 0; }
  # ready가 사라지는 연산은 요청자 경로로의 rename뿐이다. 요청자가 이미 수거했어도 다시 공개하지 않는다.
  [ -e "$ready" ] || { mv -f "$intent" "$m/$r.sent" && echo ALREADY; exit; }
fi
t=$m/$r.tmp; cat >"$t" || exit 1
if [ "$(wc -c <"$t" | tr -d " ")" != "$z" ] || [ "$(hs "$t")" != "$h" ]; then rm -f "$t"; echo BADINPUT; exit 0; fi
if [ -f "$f" ]; then
  rm -f "$t"
  if [ -f "$o" ]; then
    owner=$(cat "$o") || exit 1
    [ "$owner" = "$r" ] || { echo EXISTS; exit 0; }
  fi
  if [ "$(hs "$f")" = "$h" ]; then setowner && mark && echo ALREADY; exit; fi
  echo EXISTS; exit 0
fi
setowner &&
  mv -f "$t" "$ready" && printf "%s\n" "$h" >"$intent.tmp" && mv -f "$intent.tmp" "$intent" &&
  mv -f "$ready" "$f" && mv -f "$intent" "$m/$r.sent" && echo SENT' _ "$d" "$@"`

// 그 밖의 onm 본문(셸 판 글자 그대로)
const (
	inbCatReqSh   = `cat "$d/$1.req"`
	inbHasBriefSh = `b=$d/$1.brief.md; [ -f "$b" ] || { echo no; exit 0; }; grep -q "[^[:space:]]" "$b"; c=$?; if [ $c = 0 ]; then echo yes; elif [ $c = 1 ]; then echo no; else exit $c; fi` // 없음·빈 것만 no, 못 읽으면 실패(R-22)
	inbClaimSh    = `mkdir -p "$d/.hc-fill/claim" && printf "%s %s %s\n" "$2" "$3" "$4" >"$d/.hc-fill/claim/$1.tmp" && mv -f "$d/.hc-fill/claim/$1.tmp" "$d/.hc-fill/claim/$1"`
	inbCatBriefSh = `b=$d/$1.brief.md; [ -f "$b" ] || b=$d/taken/$1.$2.brief.md; cat "$b"`
	inbTakenSh    = `mkdir -p "$d/taken" && { [ ! -f "$d/$1.brief.md" ] || mv -f "$d/$1.brief.md" "$d/taken/$1.$2.brief.md"; } &&
        { [ ! -f "$d/$1.req" ] || mv -f "$d/$1.req" "$d/taken/$1.$2.req"; } && rm -f "$d/.hc-fill/claim/$1"`
	inbReportSh = `if [ -f "$d" ]; then echo HAVE; cat "$d"; else echo NONE; fi`
)

// inbPass: 셸 판에서 받지 않은 onm의 표준 출력은 그대로 나간다
func inbPass(out string) { fmt.Print(out) }

// irej <사유>: 받은편지 요청 n(기계 im 폴더 idir)을 rejected/로 옮기고 사유 한 줄
func inbIrej(im, idir, n, why string) {
	out, rc := onm(nil, im, inbIrejSh, idir, n, why)
	inbPass(out)
	if rc == 0 {
		say("거절 %s %s: %s", im, n, why)
	} else {
		say("받은편지 %s %s: 거절(%s)을 쓰지 못했다, 다음 바퀴에", im, n, why)
	}
}

// inbReq: printf '%s\n' "$req" | awk -F'\t' '$1==k {print $2; exit}' — ok=false면 그 줄 없음
func inbReq(req, k string) (string, bool) {
	for _, l := range strings.Split(req, "\n") {
		if r := splitRow(l); r.F(1) == k {
			return r.F(2), true
		}
	}
	return "", false
}

func inbDry() string {
	if dry {
		return "1"
	}
	return "0"
}

// inbox_tick: 설정 inbox <기계> <폴더> 줄마다 그 폴더의 <이름>.req를 검사해 대기열에 넣는다
func inboxTick() {
	rows := confRows("inbox")
	if len(rows) == 0 {
		return
	}
	ICON = connected()
	for _, l := range confLines {
		if f := fields(l); len(f) == 0 || f[0] != "inbox" {
			continue
		}
		im, idir := F1(fields(l), 1), restAfter(l, 2)
		if idir == "" {
			say("알림: 설정 inbox 줄에 폴더가 없다: %s", im)
			continue
		}
		if strings.Contains(idir, "#") {
			say("알림: 설정 inbox 폴더에 #를 쓰지 않는다(요청 ID 칸 구분): %s", idir)
			continue
		}
		if !upOK(im) {
			say("받은편지 건너뜀 %s: 연결 안 됨", im)
			continue
		}
		out, rc := onm(nil, im, inbSIlist, idir, inbDry())
		if rc != 0 {
			say("받은편지 건너뜀 %s: 목록을 읽지 못했다", im)
			continue
		}
		for _, n := range strings.Split(sub(out), "\n") {
			if n != "" {
				inbOne(im, idir, n)
			}
		}
	}
}

// inbOne: inbox_tick 안쪽 고리 한 바퀴(이름 하나)
func inbOne(im, idir, n string) {
	why, p, a, o, pr, rid := "", "", "-", "-", "high", ""
	var hq, hb string
	if !okname(n) {
		why = "이름은 a-z 0-9 - _ 32자까지"
	} else if strings.HasPrefix(n, "audit-") {
		why = "이름은 audit- 로 시작하지 않는다(자동 감사 줄 이름 자리)"
	} else if w, ok := kindwhy(n); ok {
		why = w
	} else {
		ci, rc := onm(nil, im, inbSIclaim, idir, n)
		if rc != 0 {
			say("받은편지 %s %s: 접수 표시를 읽지 못했다, 다음 바퀴에", im, n)
			return
		}
		first, _, _ := strings.Cut(sub(ci), "\n")
		cf := fields(first)
		crid, cq, cb := F1(cf, 0), F1(cf, 1), F1(cf, 2)
		hq, hb = F1(cf, 3), restAfter(first, 4)
		if crid != "-" {
			if !reqIDOK(crid) { // 경로에 쓰는 값 — 꼴이 아니면 쓰지 않는다(Fable E1: x/../../escaped로 데이터 폴더 밖에 썼다, 셸 판)
				say("받은편지 %s %s: 접수 표시의 요청 ID 꼴이 아니다(%s) — 건너뜀", im, n, runeCut(crid, 40))
				return
			}
			if cq == hq && cb == hb {
				rid = crid
			} else {
				say("받은편지 %s %s: 접수 중인 요청(%s)과 내용이 다르다 — 그 접수가 끝난 뒤 다음 바퀴에", im, n, crid)
				return
			}
		}
		ro, rrc := onm(nil, im, inbCatReqSh, idir, n)
		if rrc != 0 {
			say("받은편지 %s %s: .req를 읽지 못했다, 다음 바퀴에", im, n)
			return
		}
		req := sub(ro)
		p, _ = inbReq(req, "project")
		a, _ = inbReq(req, "after")
		a = dash(a)
		o, _ = inbReq(req, "only")
		o = dash(o)
		// 우선순위: 적혀 있으면 그 값, 없으면 높음(회원이 겪는 오류 수정이 기능 개발보다 먼저 — 2026-11-17 지휘 결정)
		if x, ok := inbReq(req, "priority"); ok && x != "-" {
			var good bool
			if pr, good = pnorm(x); !good {
				pr, why = "high", "모르는 priority: "+x+" (긴급|높음|보통|낮음 또는 urgent|high|normal|low)"
			}
		}
		known := false
		for _, f := range confRows("repo") {
			if F1(f, 1) == p {
				known = true
				break
			}
		}
		if !known {
			pp := p
			if pp == "" {
				pp = "(없음)"
			}
			why = "설정에 없는 project: " + pp
		}
		if a != "-" {
			for _, x := range words(a) {
				if !okname(x) {
					why = "after 레인 이름: " + x
				}
				if w, ok := kindwhy(x); ok {
					why = "after: " + w
				}
			}
		}
		if o != "-" {
			for _, x := range words(o) {
				if !machineKnown(x) {
					why = "설정에 없는 only 기계: " + x
				}
			}
		}
	}
	if why == "" && rid == "" { // 새 접수: 같은 이름 줄이 있으면 거절, 지시문이 없거나 비었으면 거절
		hst, hrp := "", ""
		if r := lsFirst(Q, n); r != nil {
			hst, hrp = r.F(6), r.F(11)
		}
		// 옛 차례(접수 표시 없음)가 대기열에 넣고 taken/으로 못 옮긴 요청: 옮긴 옛 줄(요청 ID rl…)이 이 받은편지·이름이면 그 ID로 옮기기만
		if strings.HasPrefix(hrp, im+":"+idir+"/done/"+n+".md#rl") && hst != "sent" {
			rid, hst = hrp[strings.LastIndexByte(hrp, '#')+1:], ""
		}
		switch {
		case rid != "":
		case hst == "sent":
			why = "이미 끝난 이름(결과를 되돌려 줌) — 새 이름으로 다시 요청"
		case hst != "":
			why = "이미 대기열에 있는 이름 — 새 이름으로 다시 요청"
		case hb == "-":
			why = "지시문 " + n + ".brief.md가 없거나 비었다"
		default:
			yo, yrc := onm(nil, im, inbHasBriefSh, idir, n)
			if yrc != 0 { // 명령이 실패하면 출력(「no」여도)을 믿지 않는다 — 요청·접수 그대로(R-22)
				yo = ""
			}
			switch sub(yo) {
			case "yes":
			case "no":
				why = "지시문 " + n + ".brief.md가 없거나 비었다"
			default:
				say("받은편지 %s %s: 지시문을 확인하지 못했다, 다음 바퀴에", im, n)
				return
			}
		}
	}
	if why != "" {
		if dry {
			say("%s거절할 %s %s: %s", P, im, n, why)
			return
		}
		inbIrej(im, idir, n, why)
		return
	}
	if dry {
		say("%s받을 %s %s (%s)", P, im, n, p)
		return
	}
	if rid == "" { // 접수 표시를 먼저(요청 ID를 고정)
		rid = newid("r")
		for lget(rid) != nil || exists(DATA+"/replies/"+rid+".md") {
			rid = newid("r")
		}
		out, rc := onm(nil, im, inbClaimSh, idir, n, rid, hq, hb)
		inbPass(out)
		if rc != 0 {
			say("받은편지 %s %s: 접수 표시를 두지 못했다, 다음 바퀴에", im, n)
			return
		}
		crash("inbox-after-claim")
	}
	bf := DATA + "/briefs/" + im + "/" + n + "." + rid + ".md"
	if !nonEmpty(bf) {
		// 셸 판과 다름: <bf>.tmp를 두지 않는다 — 받은 내용의 해시를 먼저 보고 맞을 때만 bf로(가져오기 실패·해시 다름에 남는 파일 없음)
		ok := os.MkdirAll(DATA+"/briefs/"+im, 0o755) == nil
		if ok {
			body, rc := onm(nil, im, inbCatBriefSh, idir, n, rid)
			ok = rc == 0 && sha256hex([]byte(body)) == hb && writeAtomic(bf, body) == nil
		}
		if !ok {
			os.Remove(bf + ".tmp") // 셸 판이 남긴 사본 임시 파일도
			say("받은편지 %s %s: 지시문을 가져오지 못했다(또는 내용이 접수 때와 다르다), 다음 바퀴에", im, n)
			return
		}
	}
	if w, bad := briefBad(bf); bad { // 본문 없음(부록만) 등(fill-dep-needs)
		os.Remove(bf)
		os.Remove(bf + ".tmp")
		inbIrej(im, idir, n, "지시문 "+n+".brief.md: "+w)
		return
	}
	ia, rc := qInadd(n, p, bf, a, o, pr, im+":"+idir+"/done/"+n+".md", rid)
	if rc != 0 {
		return
	}
	crash("inbox-after-add")
	if ia == "ADDED" {
		say("받음 %s %s → 대기열 (%s, 우선 %s, 요청 %s)", im, n, p, plabel(pr), rid)
	}
	out, trc := onm(nil, im, inbTakenSh, idir, n, rid)
	inbPass(out)
	if trc != 0 {
		say("받은편지 %s %s: taken/으로 옮기지 못했다, 다음 바퀴에 다시", im, n)
	}
}

var inbDoneRe = regexp.MustCompile(`^결과: (고침[[:blank:]]+[[:xdigit:]]{4,40}[[:blank:]]*|(넘김|실패)[[:blank:]]+[^[:space:]].*)$`)

// inbLast: printf '%s\n' "$x" | tail -n 1
func inbLast(s string) string { return s[strings.LastIndexByte(s, '\n')+1:] }

// rdone <본문>: 마지막 줄이 완성 보고(「결과: 고침 <sha>」·「결과: 넘김|실패 <까닭>」)
func rdone(body string) bool { return inbDoneRe.MatchString(inbLast(body)) }

// reply_tick: 받은편지 레인 결과 고정·공개·sent 투영
func replyTick() {
	ICON = CONN
	// 1) 고정
	for _, r := range qRows() {
		if r.F(6) != "up" || r.F(11) == "" || r.F(11) == "-" {
			continue
		}
		f := lsSplit(r.Line(), 12)
		if f[0] != "" {
			inbFix(f[0], f[6], f[7], f[10])
		}
	}
	// 2) 공개
	for _, l := range readLines(LED) {
		f := lsSplit(l, 8)
		if f[3] == "deliver" {
			inbPub(f[0], f[1], f[2], f[5], f[6])
		}
	}
	// 3) 장부 sent인데 대기열 줄이 아직 up이면 sent로(공개·장부 뒤 대기열 기록 전에 죽음)
	if dry || !nonEmpty(LED) {
		return
	}
	sent := map[string]bool{}
	for _, x := range readRows(LED) {
		if x.F(4) == "sent" {
			sent[x.F(1)] = true
		}
	}
	for _, x := range qRows() {
		if x.F(6) != "up" || !strings.Contains(x.F(11), "#") {
			continue
		}
		if rid := x.F(11)[strings.LastIndexByte(x.F(11), '#')+1:]; sent[rid] {
			qset(true, x.F(17), x.F(1), "sent", x.F(7), x.F(8), x.F(9), "", "")
		}
	}
}

// inbFix: reply_tick 1) 한 줄 — 레인 기계 보고를 고정본 replies/<요청 ID>.md로
func inbFix(n, m, a, rp string) {
	i := strings.LastIndexByte(rp, '#')
	if i < 0 {
		return
	}
	rid := rp[i+1:]
	lr := lget(rid)
	if lr.F(4) != "open" {
		return
	}
	if exists(DATA + "/replies/" + rid + ".md") { // 고정 뒤 장부 전에 죽음
		if !dry {
			rfix(rid, lr.F(5), "", false)
		}
		return
	}
	if !upOK(m) {
		say("되돌려 주기 미룸 %s: %s 연결 안 됨", n, m)
		return
	}
	if EVBAD || has(EV, "모름 "+m) {
		say("되돌려 주기 미룸 %s: 레인 사건을 못 읽어 제출 여부를 모름", n)
		return
	}
	// 병합기에 제출하고 끝난 레인(마지막 사건 「제출」)은 그 제출 ID의 병합기 결과(착지·막힘)가 정해진 뒤에 회신하고, 그 결과를 끝 줄로
	mline := ""
	if lx := joblast(n, a, "제출"); lx != "" {
		sid := F1(fields(splitRow(lx).F(7)), 0)
		mr := mgres(n, sid)
		if mr == "" {
			say("되돌려 주기 미룸 %s: 병합기 결과 기다림(제출 ID %s)", n, sid)
			return
		}
		x := splitRow(mr)
		if x.F(5) == "착지" {
			mline = "착지 " + runeCut(x.F(6), 12) + " (병합기 " + x.F(2) + ", 제출 ID " + sid + ")"
		} else {
			mline = "결과: 실패 병합기 막힘 — " + x.F(7)
		}
	}
	rpth := "~/.local/share/hc-fill/reports/" + n + "." + rid + ".md"
	if strings.Contains(lr.F(8), "legacy") {
		rpth = "~/.local/share/hc-fill/reports/" + n + ".md"
	}
	// 종료를 확인한 뒤 보고를 읽어, 마지막 mv와 종료가 같은 바퀴에 일어나도 본문을 놓치지 않는다.
	endrc := pget(a)
	if endrc == 0 {
		if started := attemptRun(n, a); started != "" && PRUN != started {
			endrc = 3
		} else if PNAME != "" && PNAME != "-" && PNAME != n && !lalive(a, n) {
			endrc = 3
		}
	}
	ro, rc := onm(nil, m, inbReportSh, rpth)
	if rc != 0 {
		say("되돌려 주기 미룸 %s: %s 에서 보고를 읽지 못했다", n, m)
		return
	}
	rep := sub(ro)
	kind, body := "result", ""
	switch {
	case strings.HasPrefix(rep, "HAVE"):
		body = rep[4:]
		_, w := utf8.DecodeRuneInString(body) // ${body#?}
		body = body[w:]
	case rep == "NONE":
	default:
		say("되돌려 주기 미룸 %s: %s 에서 보고를 읽지 못했다", n, m)
		return
	}
	if body == "" || !rdone(body) {
		if endrc != 3 {
			if !lalive(a, n) {
				say("되돌려 주기 미룸 %s: 터미널 종료를 확인하지 못했다", n)
			}
			return
		}
		kind = "noreport"
		if body != "" {
			body += "\n결과: 실패 보고 불완전"
		} else {
			body = "결과: 실패 보고 없음"
		}
	}
	if mline != "" {
		body += "\n" + mline
	}
	if dry {
		dest, _, _ := strings.Cut(rp, "#")
		say("%s되돌려 줄 %s → %s (%s)", P, n, dest, inbLast(body))
		return
	}
	if rfix(rid, kind, body, true) != 0 {
		say("되돌려 주기 미룸 %s: 고정본을 두지 못했다", n)
		return
	}
	crash("after-reply-ledger")
}

// inbPub: reply_tick 2) 한 줄 — 장부 deliver를 요청자 기계에 s_ipub로
func inbPub(rid, n, dest, h, z string) {
	rq, rpath := dest, dest
	if i := strings.IndexByte(dest, ':'); i >= 0 {
		rq, rpath = dest[:i], dest[i+1:]
	}
	rdir := rpath
	if i := strings.LastIndex(rpath, "/done/"); i >= 0 {
		rdir = rpath[:i]
	}
	rnm := strings.TrimSuffix(rpath[strings.LastIndexByte(rpath, '/')+1:], ".md")
	if !upOK(rq) {
		say("되돌려 주기 미룸 %s: %s 연결 안 됨", n, rq)
		return
	}
	rf := DATA + "/replies/" + rid + ".md"
	if !isFile(rf) {
		say("되돌려 주기 미룸 %s: 고정본이 없다 %s", n, rf)
		return
	}
	last := func() string { s, _ := readFile(rf); return lastLine(s) }
	if dry {
		say("%s되돌려 줄 %s → %s (%s)", P, n, dest, last())
		return
	}
	in, err := os.Open(rf)
	if err != nil {
		say("되돌려 주기 실패 %s → %s, 다음 바퀴에", n, dest)
		return
	}
	out, rc := onm(in, rq, inbSIpub, rdir, rnm, rid, h, z)
	in.Close()
	if rc != 0 {
		say("되돌려 주기 실패 %s → %s, 다음 바퀴에", n, dest)
		return
	}
	po := sub(out)
	crash("after-publish")
	switch po {
	case "SENT", "ALREADY":
		if lset(rid, "4", "sent") != 0 {
			say("되돌려 줌 %s → %s — 장부 기록 실패, 다음 바퀴에 같은 내용으로 다시(요청자는 ALREADY)", n, dest)
			return
		}
		say("되돌려 줌 %s → %s (%s)", n, dest, last())
	case "CONFLICT", "EXISTS":
		if aesc(n, "-", "받은편지 결과를 못 둠 "+n+" → "+dest+": 그 자리에 다른 결과가 있다("+po+" — 덮지 않음). 지휘 판단(고정본 "+rf+")", "hf:"+rid+":held") == 0 {
			lset(rid, "4", "held")
		}
	default:
		say("되돌려 주기 미룸 %s → %s: 「%s」, 다음 바퀴에", n, dest, po)
	}
}
