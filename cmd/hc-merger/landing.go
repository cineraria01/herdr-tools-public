package main

// 착지 중 표식(internal/landing — reland-live-lane): 착지를 부르기 직전 묶음 제출마다 원격 refs/herdr/landing/<큐 이름>(= 제출 SHA),
// 다음 바퀴 시작(일지 정리 뒤 — 이 병합기가 착지 중이 아님)에 남은 표식을 지운다. 표식은 길 안내다: 못 올리거나 못 지워도 착지는
// 그대로 하고(정확성은 main CAS·큐 lease), hc-lane land 제출·hc-fill 다시 맞춤이 그 레인을 지금 갈아 끼우지 않게 할 뿐이다.

import (
	"fmt"
	"os"
	"strings"

	"herdr-tools/internal/landing"
)

// landMark: 묶음 제출마다 착지 중 표식을 올린다
func (m *M) landMark() {
	args := []string{"push", "-q", "--atomic", "origin"}
	for _, e := range m.b {
		args = append(args, "+"+e.s+":"+landing.Prefix+strings.TrimPrefix(e.q, "refs/merge-queue/"))
	}
	if run(args...) != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 착지 중 표식을 올리지 못했다(git push) — 착지는 그대로 한다")
	}
}

// landUnmark: 이번 바퀴 원격 목록(m.qs)에 남은 착지 중 표식을 지운다
func (m *M) landUnmark() {
	args := []string{"push", "-q", "origin"}
	for _, r := range landing.Parse(m.qs) {
		if strings.HasPrefix(r.Name, landing.Prefix) {
			args = append(args, ":"+r.Name)
		}
	}
	if len(args) > 3 && run(args...) != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 남은 착지 중 표식을 지우지 못했다(git push) — 다음 바퀴에")
	}
}
