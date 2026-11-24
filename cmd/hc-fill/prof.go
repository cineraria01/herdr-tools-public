package main

// 원래 PR 잇기(2026-11-24 fill-polish-1124, docs/hc-fill/pr.md): 지휘가 다시 내는 레인을 꼬리 -reland가 아닌 이름(…-fix)으로 넣으면
// 새 PR이 열려 승인까지 붙은 원래 PR을 손으로 닫아야 했고 감사 이력이 두 PR로 갈렸다. hc-fill add|edit … --pr-of <원래 레인>을
// <DATA>/pr-of.tsv(「레인\t원래」)에 적어 두면 PR 거울이 그 레인의 완료·감사·제출·착지를 원래 PR(머리 가지 pr/<원래>)에 이어 쓴다.
// 병합기는 레인 기계가 올린 짝 참조 refs/herdr/pr-of/<레인>/<원래>로 머리 가지를 찾는다(cmd/hc-merger/prhead.go). 꼬리 -reland 규칙은 그대로

import (
	"fmt"
	"os"
)

func prOfFile() string { return DATA + "/pr-of.tsv" }

// prOfGet <레인>: --pr-of로 적은 원래 레인(없으면 빈 값)
func prOfGet(lane string) string {
	for _, r := range readRows(prOfFile()) {
		if r.F(kvKey) == lane && r.F(kvVal) != "" {
			return r.F(kvVal)
		}
	}
	return ""
}

// prOfPut <레인> <원래>: 그 레인 줄을 바꾼다(원래가 빈 값이나 -이면 뺀다). 줄이 없고 뺄 것뿐이면 파일을 건드리지 않는다
func prOfPut(lane, orig string) error {
	if orig == "-" {
		orig = ""
	}
	var out []Row
	had := false
	for _, r := range readRows(prOfFile()) {
		if r.F(kvKey) == lane {
			had = true
			continue
		}
		out = append(out, r)
	}
	if !had && orig == "" {
		return nil
	}
	if orig != "" {
		out = append(out, Row{lane, orig})
	}
	if len(out) == 0 {
		if err := os.Remove(prOfFile()); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := writeAtomic(prOfFile(), rowsText(out)); err != nil {
		return fmt.Errorf("원래 PR 기록(%s)을 못 씀: %w", prOfFile(), err)
	}
	return nil
}

// prOfCheck <레인> <원래>: add·edit의 --pr-of 값 검사(-는 지움, 빈 값은 안 줌). 틀리면 die
func prOfCheck(lane, orig string) {
	if orig != "" && orig != "-" && (!okname(orig) || orig == lane) {
		die("--pr-of는 원래 레인 이름(a-z 0-9 - _ 32자까지, 이 레인과 다른 것)이나 -: %s", orig)
	}
}

// prOfAdd <레인> <원래>: add가 대기열 잠금 안에서 — 같은 이름 줄이 없을 때만(qAdd가 거절할 줄의 기록을 바꾸지 않게) 적고,
// --pr-of 없이 넣어도 같은 이름의 지난 기록은 지운다(옛 레인의 잇기가 새 레인에 남지 않게). 기록이 없고 값도 없으면 아무것도 안 함
func prOfAdd(lane, orig string) {
	if (orig == "" && prOfGet(lane) == "") || qHasNameW(lane) {
		return
	}
	if err := prOfPut(lane, orig); err != nil {
		die("%v", err)
	}
}

func prOfNote(orig string) string {
	if orig == "" {
		return ""
	}
	return ", 원래 PR " + orig
}

// prBaseOf <레인>: 그 레인이 이어 쓸 원래 PR의 레인 — --pr-of 기록을 먼저, 없으면 다시 맞춤 규칙(prRelandBase), 둘 다 아니면 자기
func prBaseOf(n string) string {
	if o := prOfGet(n); o != "" {
		return o
	}
	return prRelandBase(n)
}

// entOf <레인>: 그 레인의 사건(감사 판정·제출·착지)을 쓸 PR — 제 PR이 있으면 그것, 없으면 이어 쓰는 원래 PR(열린 것만)
func (r *prRun) entOf(lane string) *prEnt {
	if e := r.ents[lane]; e != nil {
		return e
	}
	if r.base == nil {
		r.base = map[string]string{}
	}
	b, ok := r.base[lane]
	if !ok {
		b = prBaseOf(lane)
		r.base[lane] = b
	}
	if e := r.ents[b]; b != lane && e != nil && e.State == "open" {
		return e
	}
	return nil
}
