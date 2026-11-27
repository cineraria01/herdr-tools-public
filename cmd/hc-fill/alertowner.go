package main

// alertowner.go: 레인 사건인데 보내는 쪽이 주인을 안 실은 것(운영 에이전트 「사람이 정한다」·「끝」)의 받는 쪽 — ht-alert-route,
// docs/hc-fill/alert-route.md. 주인을 못 정하면 빈 값: 지휘 watch가 대표를 고른다(hc-lane alertroute.go)

import (
	"strings"
	"time"

	"herdr-tools/internal/knob"
	"herdr-tools/internal/tri"
)

// alertOwner: alert_route one이고 글에 든 대기열 레인(낱말 단위)의 주인이 하나뿐이며 살아 있으면(ownerCheck 있음) 그 주소.
// all(기본)·레인 없음·주인 없는 레인·주인 둘·주인 확인 못 함은 빈 값 — 오늘처럼 --to 없이
func alertOwner(text string) string {
	d, _ := knob.Find("alert_route")
	if v, _, _ := knob.Resolve(d, knobEntries(), knobFile(), "", time.Now()); v != "one" {
		return ""
	}
	o := ""
	for _, r := range qRows() {
		n := r.F(qName)
		if n == "" || strings.HasPrefix(r.F(qAudit), "audit=") || !hasName(text, n) {
			continue
		}
		q := dash(r.F(qOwner))
		if q == "-" || o != "" && o != q {
			return ""
		}
		o = q
	}
	if o == "" {
		return ""
	}
	if st, _ := ownerCheck(o); st != tri.Yes {
		return ""
	}
	return o
}

// hasName: 글에 이름 n이 낱말로(앞뒤가 이름 글자 a-zA-Z0-9_.- 아님) 있나
func hasName(s, n string) bool {
	nameCh := func(b byte) bool {
		return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '.' || b == '-'
	}
	for i := 0; ; {
		j := strings.Index(s[i:], n)
		if j < 0 {
			return false
		}
		a, b := i+j, i+j+len(n)
		if (a == 0 || !nameCh(s[a-1])) && (b == len(s) || !nameCh(s[b])) {
			return true
		}
		i = a + 1
	}
}
