package main

// 이 기계의 이름(sync-local-name, 2026-11-24 — 사용자 「테일 스케일 컴퓨터 이름들과 동일하게 해서 좀 여러 컴퓨터가 안헷갈리면」): 진행 줄·
// 표·배포 기록·--status·일지 요약에 이 기계를 적는 이름. 개인 설정 hc-fill.tsv의 local 줄(대기열이 이 기계를 부르는 이름 — 같은 값을 두 번
// 적지 않게)이고, 줄이 없으면 옛 이름 「맥」이다(설정을 안 바꾼 사람의 출력은 그대로). 표기만 바꾼다 — 무엇을 어디에 까는지는 그대로.
// 옛 배포 기록의 「맥」은 이 기계로 읽는다(기록은 고쳐 쓰지 않는다)

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// oldLocal: 이 기계의 옛(기본) 이름
const oldLocal = "맥"

// LOCAL: 이 기계의 이름(main이 정한다)
var LOCAL = oldLocal

// localName: conf/hc-fill.tsv의 첫 local 줄 값(hc-fill과 같은 줄). 파일·줄·값이 없으면 「맥」. 못 읽음·값이 한 낱말이 아님·기록 칸을
// 깨는 글자(= :)·hc machine ls의 다른 기계와 같은 이름은 오류(모름 — 두 기계를 한 이름으로 적지 않게)
func localName(conf string, machines []string) (string, error) {
	p := conf + "/hc-fill.tsv"
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return oldLocal, nil
	}
	if err != nil {
		return "", fmt.Errorf("이 기계 이름을 읽지 못했다 (%s): %w", p, err)
	}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) == 0 || f[0] != "local" {
			continue
		}
		if len(f) == 1 {
			return oldLocal, nil
		}
		if len(f) > 2 || strings.ContainsAny(f[1], "=:") {
			return "", fmt.Errorf("이 기계 이름이 틀렸다 (%s의 local 줄 — 한 낱말, = : 없이): %s", p, strings.Join(f[1:], " "))
		}
		for _, m := range machines {
			if m == f[1] {
				return "", fmt.Errorf("이 기계 이름 %s가 hc machine ls의 다른 기계 이름과 같다 (%s의 local 줄)", m, p)
			}
		}
		return f[1], nil
	}
	return oldLocal, nil
}

// localOf: 배포 기록의 기계 이름 — 옛 이름 「맥」은 이 기계
func localOf(m string) string {
	if m == oldLocal {
		return LOCAL
	}
	return m
}

// localSpec: --take <기계>:<경로>의 옛 이름 「맥」을 이 기계 이름으로 — 이름을 정한 뒤에도 옛 꼴이 같은 것을 덮는다
func localSpec(s string) string {
	if before(s) == oldLocal {
		return LOCAL + ":" + after(s)
	}
	return s
}

// mustLocal: localName(개인 설정 폴더 CONF) — 모르면 아무것도 쓰지 않고 종료 1
func mustLocal(machines []string) string {
	n, err := localName(CONF, machines)
	if err != nil {
		fmt.Fprintf(os.Stderr, "herdr-sync: %v — 모름, 아무것도 쓰지 않음\n", err)
		os.Exit(1)
	}
	return n
}

// notLocal: 고른 기계 목록에서 이 기계 이름을 뺀다 — 이 기계는 늘 먼저(따로) 맞추고 보이니, hc --on으로 자기에게 가거나 줄이 두 번 나오지
// 않게(상태 한 장의 「herdr-sync --status <기계>」 권함이 이 이름을 댄다). 옛 이름 「맥」도 이 기계(이름을 정하지 않았을 때 hc --on 맥으로
// 가 「연결 실패」로 적히던 것, ht-sync-arg-guards — --take 맥:… 만 쳐도 이 기계만 돈다)
func notLocal(ms []string) []string {
	r := []string{}
	for _, m := range ms {
		if m != LOCAL && m != oldLocal {
			r = append(r, m)
		}
	}
	return r
}

// localRes: 배포 기록 결과 칸(「기계=결과」 낱말들)을 이 기계 이름으로 읽은 것
func localRes(res string) string {
	if LOCAL == oldLocal {
		return res
	}
	w := strings.Fields(res)
	for i, x := range w {
		if m, r, ok := strings.Cut(x, "="); ok {
			w[i] = localOf(m) + "=" + r
		}
	}
	return strings.Join(w, " ")
}
