package main

// 그 완료가 아직 서 있는가(ht-submit-still-stands, 2026-11-28 — 레인이 완료 뒤 막힘을 보고하고 지휘가 선행을 바꾸고 레인이 주차했는데,
// 앞서 돌던 감사가 통과하자 대기열이 그 커밋을 제출했다): 통과 판정을 제출로 보내는 두 길(대기열이 제출 · 「착지하라」 깨우기)이 판정
// 파일을 두기 전 aDeliver에서 이 한 물음을 본다. 서 있지 않으면 판정은 deliver 그대로 두고(다시 서면 다음 바퀴에 이어 간다) 주인 지휘에 한 번.
// 마지막 막음은 hc-lane land(cmd/hc-lane/stands.go — 그 기계의 지금 사건·refs/park)

import (
	"fmt"
	"strings"

	"herdr-tools/internal/tri"
)

// standStates: 「그 완료」를 거두거나 세우는 레인 보고. 중간·정리됨·판정(감사 줄)·제출·착지(aLanded가 먼저 닫음)와 hc-fill이 그 레인
// 이름으로 남긴 막힘(다른 터미널 — runlast가 주소로 거른다)은 세지 않는다
var standStates = []string{"완료", "막힘", "주차"}

// auStands: 이 차수(auLn·auPa·auPrun·auSha)의 완료가 아직 서 있나 — Yes · No(까닭) · Unknown(까닭).
// (가) 그 실행의 마지막 완료·막힘·주차가 이 sha의 완료 (나) 그 줄이 주차로 되돌아가지 않음 (다) 그 줄의 지금 선행이 그 줄 모드의 지금
// 규칙으로 다 풀림(쌓아 띄운 줄은 선행이 감사 통과로 병합 줄에 서 있으면 풀림 — stackPick) (라) 처리 안 된 사람 리뷰 막음 없음(phJudge)
func auStands() (tri.State, string) {
	saved := EV
	EV = nil
	for _, e := range saved {
		if has(standStates, splitRow(e).F(evState)) {
			EV = append(EV, e)
		}
	}
	l := runlast(auLn, auPa, auPrun, "")
	EV = saved
	if st := evF(l, evState); st != "완료" || !auSameSha(auSha, evF(l, evSHA)) {
		return tri.No, "레인이 그 뒤 「" + st + " " + evF(l, evSHA) + "」를 보고했다"
	}
	q := qrow(auLn)
	if q == nil {
		return tri.Yes, ""
	}
	if q.F(qState) == "wait" { // 떠 있는 줄의 14칸 parked는 주차 뒤 채택한 실행일 수 있다 — 주차 자체는 (가)의 사건으로 본다
		return tri.No, "줄이 주차·대기로 돌아갔다"
	}
	if a := dash(q.F(qAfter)); a != "-" {
		switch ar := afterOK(auLn, q.F(qProj), a); {
		case ar == 2:
			return tri.Unknown, "선행 " + a + " 판정 못 함"
		case ar == 1 && rowMode(q) == mStacked && !stackRead():
			return tri.Unknown, "쌓아 띄운 선행 " + a + " 의 병합 줄을 읽지 못함"
		case ar == 1 && !(rowMode(q) == mStacked && stackKeep(q)):
			return tri.No, "선행 " + a + " 가 아직 안 들어갔다"
		}
	}
	if h := phHeld(auLn); h != "" {
		return tri.No, h
	}
	return tri.Yes, ""
}

// stackRead: 바로 앞 afterOK가 fetch한 저장소(FR)의 병합 줄을 읽을 수 있나 — stackPick은 못 읽음도 거짓이라 모름을 먼저 가른다
// shortcut: 제출 기록(merge-queue-meta) 읽기 실패는 아직 「서 있지 않다」로 친다(다음 바퀴에 읽히면 냄), 잦으면 stackPick이 tri를 돌려주게
func stackRead() bool { _, ok := stackQueue(FR); return ok }

// stackKeep: stackPick 판정만(이번 띄움 전역 STACK·STREF는 그대로)
func stackKeep(q Row) bool {
	s, r := STACK, STREF
	defer func() { STACK, STREF = s, r }()
	return stackPick(q)
}

// phHeld <레인>: 그 레인 PR에 처리 안 된 사람 리뷰 막음이 있으면 그 이름(「사람 리뷰 반영 중」 등), 없거나 거울이 꺼졌으면 빈 값 — humanHold와 같은 판정
func phHeld(lane string) string {
	if !prHumanOn() {
		return ""
	}
	r := &prRun{}
	r.ents, _ = prLoad()
	e := r.entOf(lane)
	if e == nil || e.State != "open" {
		return ""
	}
	P := e.Repo + "#" + e.Num
	rows := readRows(phFile())
	T, _ := phOpen(rows, P)
	if T == "" {
		return ""
	}
	_, _, label := phJudge(rows, P, T, r.phFamily(e))
	return label
}

// aGate: aDeliver가 판정 파일을 두기 전 — 통과일 때만 묻는다(지적은 제출 길이 아니다). stop = 서 있지 않아 보류함, unk = 모름(파일은 두되
// 이 바퀴는 제출·깨우기 없이 다음 바퀴에 — 수거 기한 ack_max는 지금처럼 세어 끝없이 미루지 않는다)
func aGate() (unk, stop bool) {
	if auAv != "pass" {
		return false, false
	}
	switch s, why := auStands(); s {
	case tri.No:
		aHold(why)
		return false, true
	case tri.Unknown:
		fmt.Println("판정 보류 " + auAn + " → " + auLn + ": 완료가 아직 서 있는지 모른다(" + why + ") — 이 바퀴는 제출·깨우기 없이, 다음 바퀴에")
		return true, false
	case tri.Yes:
	}
	return false, false
}

// aHeldEnd: 보류(held)한 차수의 부모 실행이 끝났다(주차한 레인을 닫음 등) — 보류할 때 지휘에 알렸으니 다시 알리지 않고 superseded
func aHeldEnd() bool {
	if !inlist("held", dash(auAfl)) {
		return false
	}
	aupd(auAid, cv{atState, "superseded"})
	fmt.Println("판정 취소 " + auAn + ": 보류한 통과의 부모 실행(" + auPrun + ")이 끝났다 — 보류 때 지휘에 알림, 새 실행의 완료로 새 차수")
	return true
}

// aHold <까닭>:서 있지 않은 통과 — 판정 파일을 두지 않고(제출·깨우기 없음) 주인 지휘에 한 번(키 hf:<ID>:held, 차수 16칸 held).
// 끝난 감사 터미널은 지금처럼 닫는다(주차한 레인의 hc-lane clean이 그 터미널에 막히지 않게)
func aHold(why string) {
	if !inlist("held", dash(auAfl)) || dry { // 보류 중 바퀴마다 같은 줄을 쌓지 않는다
		fmt.Println("판정 보류 " + auAn + " → " + auLn + ": 완료(" + head(auSha, 8) + ")가 서 있지 않다 — " + why + ". 제출·착지 깨우기 안 함")
	}
	if dry {
		return
	}
	if !inlist("held", dash(auAfl)) && aesc(auLn, qown(auLn), fmt.Sprintf("%s 감사 %s번째 통과(%s, %s)를 제출하지 않았다 — %s. 판정은 그 줄에 남는다: 같은 커밋 완료를 다시 보고하거나 선행이 들어가거나 사람 리뷰가 처리되면 다음 바퀴에 이어 낸다",
		auLn, auRnd, auAn, head(auSha, 8), strings.TrimSuffix(why, ".")), "hf:"+auAid+":held") == 0 {
		auAfl = addFlag(auAfl, "held")
		aupd(auAid, cv{atFlags, auAfl})
	}
	aCloseDelivered()
}
