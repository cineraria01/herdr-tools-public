// evkeys.go: 사건 열쇠(셸 판 bin/hc-lane의 awk 조각 nkf·evkey·landed)
package main

import "strings"

// nk: 기계 이름을 뺀 열쇠 — 보통 줄은 기계 칸을 *로·주소 칸은 끝의 t…만, 멈춤 열쇠(4칸)는 주소 칸만. # 줄·7칸 못 된 줄은 그대로
func nk(l string) string {
	if l == "" {
		return l
	}
	f := strings.Split(l, tab)
	if f[0] == "멈춤" && len(f) == 4 {
		return "멈춤" + tab + lastSeg(f[1]) + tab + f[2] + tab + f[3]
	}
	if len(f) < 7 || strings.HasPrefix(f[0], "#") {
		return l
	}
	f[2] = lastSeg(f[2])
	return f[0] + tab + "*" + tab + strings.Join(f[2:], tab)
}

// evKey: 사건의 열쇠 — 보통 줄은 줄 그대로, 멈춤은 「멈춤 터미널 레인 상태 turn」(done·idle은 「쉼」으로 묶음)
func evKey(f []string) string {
	if fld(f, 5) != "멈춤" {
		return strings.Join(f, tab)
	}
	w := fld(f, 7)
	if strings.HasPrefix(w, "done ") || strings.HasPrefix(w, "idle ") {
		w = "쉼 " + w[5:]
	}
	return "멈춤" + tab + fld(f, 3) + tab + fld(f, 4) + tab + w
}

func lineKey(l string) string { return nk(evKey(strings.Split(l, tab))) }

func num(x string) bool { return digits(x) }

// samesha: -가 아니고, 소문자로 짧은 쪽 길이만큼 앞자리가 같다
func samesha(a, b string) bool {
	if a == "-" || b == "-" || a == "" || b == "" {
		return false
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	return strings.EqualFold(a[:n], b[:n])
}

// lastRep: 레인(열쇠)의 마지막 report(셸 판 lrep). 착지·판정 뒤 같은 턴의 같은 sha 완료는 착지 그대로 두고 turn만 그 완료의 것으로
type lastRep struct {
	lst, ltn, lsh, lt0 map[string]string
}

func newLastRep() *lastRep {
	return &lastRep{map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}}
}

func (r *lastRep) rep(s, h, t, k string) {
	if k == "" {
		k = "x"
	}
	if sh, ok := r.lsh[k]; ok && s == "완료" && samesha(h, sh) && num(t) && num(r.lt0[k]) && atoi(t) <= atoi(r.lt0[k]) {
		r.ltn[k] = t
		return
	}
	r.lst[k], r.ltn[k] = s, t
	delete(r.lsh, k)
	if s == "착지" || s == "판정" {
		r.lsh[k], r.lt0[k] = h, t
	}
}

// turnOf: 사건의 여덟째 칸(없으면 -)
func turnOf(f []string) string {
	if len(f) >= 8 {
		return f[7]
	}
	return "-"
}

// newEvents: 커서·sent에 없는 새 사건(셸 판 ev_cmd의 큰 awk) — 정리됨 판정, 지휘 거름, --state·--since 거름
func (e *ev) newEvents(cand []string, born string) []string {
	seen := map[string]bool{}
	fl := ""
	cl, _ := readLines(e.cur)
	for _, l := range cl {
		if v, ok := strings.CutPrefix(l, "#since"+tab); ok {
			fl = v
		} else {
			seen[nk(l)] = true
		}
	}
	if e.wsf != "" {
		sl, _ := readLines(e.wsf)
		for _, l := range sl {
			seen[nk(l)] = true
		}
	}
	kind := e.lkinds()
	par := map[string]string{}
	for _, r := range e.pmap {
		par[r[0]] = r[1]
	}
	W := "," + e.sts + ","
	lr := newLastRep()
	done := map[string]bool{}
	var out []string
	for _, l := range cand {
		if !strings.Contains(l, tab) {
			continue
		}
		f := strings.Split(l, tab)
		t2 := fld(f, 3) + "\x1c" + fld(f, 4)
		if fld(f, 5) == "정리됨" {
			ls, ln := lr.lst[t2], lr.ltn[t2]
			if handedOff(ls) { // 제출·주차 뒤 정리는 turn과 상관없이 조용히(stopped.go handedOff)
				continue
			}
			fin := ls == "착지" || ls == "판정" ||
				ls == "완료" && (kind[[2]string{lastSeg(f[2]), f[3]}] == "read" || kind[[2]string{lastSeg(f[2]), f[3]}] == "fill")
			if fin && num(fld(f, 8)) && num(ln) && atoi(fld(f, 8)) <= atoi(ln)+1 {
				continue
			}
			if seen[nk("멈춤"+tab+f[2]+tab+f[3]+tab+"쉼 turn "+fld(f, 8))] {
				continue
			}
			f[4], f[5], f[6] = "멈춤", "-", "뜻밖의 종료(정리됨) "+f[6]
		}
		if f[4] != "멈춤" {
			lr.rep(f[4], f[5], turnOf(f), t2)
		}
		if my := e.myp; my != "" {
			o := fld(f, 11)
			if len(f) >= 11 && o != "-" && o != "*" {
				ow := strings.Split(o, "@")
				addr := ow[0]
				if addr != my && !strings.HasSuffix(addr, "/"+my) && !strings.HasSuffix(my, "/"+addr) {
					continue
				}
				if len(ow) > 1 && (len(ow) != 2 || !num(ow[1]) || ow[1] != born) {
					continue
				}
			}
			if len(f) < 11 || o == "-" {
				if p, ok := par[f[2]]; ok && p != "-" && p != my && !strings.HasSuffix(p, "/"+my) {
					continue
				}
			}
		}
		k := nk(evKey(f))
		if f[0] >= e.since && f[0] >= fl && (W == ",," || strings.Contains(W, ","+f[4]+",")) && !seen[k] && !done[k] {
			done[k] = true
			out = append(out, strings.Join(f, tab))
		}
	}
	sortByTime(out)
	return out
}
