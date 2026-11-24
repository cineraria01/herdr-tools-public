package main

// ls 명령 — 셸 판 bin/hc-fill의 「ls)」 절(els 포함)

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// lsTerm: hc ls --json 한 터미널(jq '[.address, (.status // "-" | tostring)] | @tsv'에 이름을 더함)
type lsTerm struct{ Addr, Status, Name string }

// lsTostring: jq 「.status // "-" | tostring」 — null·false는 -, 문자열은 그대로, 그 밖은 압축 JSON(키 차례 그대로)
func lsTostring(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	switch {
	case len(raw) == 0 || string(raw) == "null" || string(raw) == "false":
		return "-"
	case raw[0] == '"':
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}
	var c bytes.Buffer
	if json.Compact(&c, raw) != nil {
		return string(raw)
	}
	return c.String()
}

// lsTerms: hc ls --json 출력을 읽는다. ok=false면 jq가 실패할 꼴(JSON 아님·.terminals가 배열 아님·칸이 객체)
func lsTerms(raw string) ([]lsTerm, bool) {
	d := json.NewDecoder(strings.NewReader(raw))
	var ts []lsTerm
	for {
		var top map[string]json.RawMessage
		if err := d.Decode(&top); err == io.EOF {
			return ts, true
		} else if err != nil || top == nil {
			return nil, false
		}
		var arr []json.RawMessage
		if json.Unmarshal(top["terminals"], &arr) != nil || arr == nil {
			return nil, false
		}
		for _, e := range arr {
			var t map[string]json.RawMessage
			if json.Unmarshal(e, &t) != nil {
				return nil, false
			}
			// 주소는 run의 목록(parseTerms)과 같은 기준 — 빈 글이 아닌 글만. 빠진 항목이 있으면 그 목록으로 끝남을 정하지 않는다(재감사 R-04)
			var a string
			if json.Unmarshal(t["address"], &a) != nil || a == "" {
				return nil, false
			}
			var name string
			if json.Unmarshal(t["name"], &name) != nil {
				name = ""
			}
			ts = append(ts, lsTerm{tsvEsc(a), tsvEsc(lsTostring(t["status"])), name})
		}
	}
}

// lsSplit: IFS=$'\t' read 칸 나누기(탭은 IFS 공백이라 이어진 탭은 하나, 앞뒤 탭은 버림). 마지막 변수(n칸째)가 나머지를 받는다
func lsSplit(line string, n int) []string {
	f := strings.FieldsFunc(line, func(r rune) bool { return r == '\t' })
	if len(f) > n {
		f = append(f[:n-1], strings.Join(f[n-1:], "\t"))
	}
	for len(f) < n {
		f = append(f, "")
	}
	return f
}

// lsFirst: awk -F'\t' -v n=… '($1 "")==n { …; exit }' — 그 이름 첫 줄(없으면 nil)
func lsFirst(path, n string) Row {
	for _, r := range readRows(path) {
		if r.F(1) == n {
			return r
		}
	}
	return nil
}

// els <프로젝트> <선행>: 먼저 띄움 레인의 선행을 local 저장소의 지금 있는 origin/<기본>으로(fetch 없이). ok=false면 판정 못 함
func lsEls(p, after string) (string, bool) {
	ew, ed := "", ""
	for _, x := range words(after) {
		n, op := afterAt(x, p) // <이름>@<프로젝트>는 그 프로젝트 저장소에서(일반 배치와 같은 해석 — 재감사2 fill/R-217)
		if FR = repoOf(op, LOCAL); FR == "" {
			return "", false
		}
		var ok bool
		if FB, ok = projectRef(FR); !ok {
			return "", false
		}
		eh, ok := landed(n)
		if !ok {
			return "", false
		}
		if eh != "" || cachedMark(op, n) {
			ed += "," + x
		} else {
			ew += "," + x
		}
	}
	if ew == "" {
		return "선행 들어감 — 본 작업", true
	}
	s := "선행 기다림 " + ew[1:]
	if ed != "" {
		s += " · 들어감 " + ed[1:]
	}
	return s, true
}

func lsLine(c ...string) string {
	w := []int{24, 10, 14, 24, 8, 4}
	var b strings.Builder
	for i, x := range c[:6] {
		b.WriteString(padB(x, w[i]) + " ")
	}
	return b.String() + c[6]
}

func init() { register("ls", lsCmd) }

// ls 명령. 셸 판과 다름: 「떠 있음」은 주소와 이름이 함께 맞을 때만(재사용된 주소의 남의 터미널은 「끝남」), 그 기계가 연결 안 됨이면
// 「모름(연결 안 됨)」, hc machine ls가 실패해 연결을 모르면 목록에 없는 원격 레인은 「모름(연결 확인 못 함)」
func lsCmd(args []string) {
	out, rc := Cmd{Name: "hc", Args: []string{"ls", "--json"}, Err: errPass}.Run()
	if rc != 0 || sub(out) == "" { // 빈 응답은 빈 목록이 아니다 — 읽은 것이 없다(아스트라 E04)
		die("hc ls 실패")
	}
	terms, ok := lsTerms(sub(out))
	if !ok {
		die("hc ls --json 출력을 읽지 못했다")
	}
	var conn []string
	connKnown := false
	if raw, mrc := run("hc", "machine", "ls", "--json"); mrc == 0 {
		conn, connKnown = jsonList(raw, "machines", "name", isConnected)
	}
	js := lsWantJSON(args)
	var jrows []lsRow
	if !js {
		fmt.Println(lsLine("레인", "프로젝트", "상태", "주소", "주인", "우선", "선행·비고"))
	}
	if !isFile(Q) {
		if js {
			lsPrintJSON(nil)
			exit(0)
		}
		maPrint() // 병합기 한 줄(mergeralive.go)은 표 끝에
		exit(0)
	}
	for _, l := range readLines(Q) {
		f := lsSplit(l, 17)
		name, proj, brief, after, only, state, addr, stuck := f[0], f[1], f[2], f[3], f[4], f[5], f[7], f[8]
		reply, owner, prio, early, ak, ovk := f[10], f[11], f[12], f[13], f[14], f[15]
		live, lr, s, a := "", "", "", ""
		switch state {
		case "wait":
			s, a = "대기", "-"
			if w := rejWhy(name, f[16]); w != "" {
				s = "띄우기 거절 — " + w
			}
		case "sent":
			s, a = "되돌려줌", addr
		default:
			m := LOCAL
			if i := strings.IndexByte(addr, '/'); i >= 0 {
				m = addr[:i]
			}
			if connKnown && m != LOCAL && !has(conn, m) {
				s = "모름(연결 안 됨)"
			} else {
				for _, t := range terms {
					if t.Addr == addr && t.Name == name {
						live = t.Status
						break
					}
				}
				switch {
				case live != "":
					s = "떠 있음(" + live + ")"
				case !connKnown && m != LOCAL: // 연결을 모르면 목록에 없음 ≠ 끝남(아스트라 E04)
					s = "모름(연결 확인 못 함)"
				default:
					s = "끝남"
				}
			}
			a = addr
			// 마지막 보고: run이 바퀴마다 적은 last.tsv(레인 상태 sha8 사건시각 한줄 읽은시각 fresh|old)
			if x := lsFirst(DATA+"/last.tsv", name); x != nil {
				if x.F(2) == "-" {
					lr = "보고 없음"
				} else {
					lr = x.F(2)
					if x.F(3) != "-" {
						lr += " " + x.F(3)
					}
					lr += " " + x.F(4)
					if x.F(5) != "-" {
						lr += " " + x.F(5)
					}
				}
				if x.F(7) == "old" {
					lr += " (옛 값 " + x.F(6) + ")"
				}
			}
		}
		note := ""
		if after != "-" {
			note = "선행 " + after
		}
		if only != "-" {
			note += " 기계 " + only
		}
		ew := "선행 " + after
		if early == "early" && live != "" {
			if e, ok := lsEls(proj, after); ok {
				ew = e
			}
		}
		switch early {
		case "early":
			note = "먼저 띄움(" + ew + ")" + earlyChainNote(name, f[16]) + strings.TrimPrefix(note, "선행 "+after)
		case "no-early":
			note += " 먼저 띄움 끔"
		case "parked":
			note = "주차(선행 " + after + " — 들어가면 lane/" + name + " 에서 이어 띄움)" + strings.TrimPrefix(note, "선행 "+after)
		case "resumed":
			note += " 주차에서 이어 띄움"
		}
		if stuck != "-" {
			note += " 상태 안 잡힘 남김 " + stuck
		}
		if dash(reply) != "-" {
			note += " 받은편지 " + strings.SplitN(reply, ":", 2)[0]
		}
		if tn := strings.Join(thenNames(brief), ","); tn != "" {
			note += " 이어서 " + tn
		}
		switch {
		case ak == "no-audit":
			note += " 자동 감사 끔"
		case strings.HasPrefix(ak, "audit="):
			note += " 감사 대상 " + strings.TrimPrefix(ak, "audit=")
		case ak == "auto":
			note += " " + anote(name)
		}
		switch ovk {
		case "overlap-ok":
			note += " 겹침 봄 끔"
		case "solo":
			note += " 단독"
		}
		if state == "wait" {
			// 겹침 대기: run이 바퀴마다 적은 overlap.tsv(레인 앞레인 파일), 능력 막힘: need.tsv(레인 능력 보냄)
			if x := lsFirst(DATA+"/overlap.tsv", name); x != nil {
				note += " 겹침 대기(" + x.F(2) + ": " + x.F(3) + ")"
			}
			if x := lsFirst(DATA+"/need.tsv", name); x != nil {
				note += " 필요 " + x.F(2) + " — 가진 기계 없음"
			}
		}
		note = strings.TrimPrefix(note, " ")
		if js {
			jrows = append(jrows, lsRow{Lane: name, Project: proj, State: s, Code: state, Address: a, Owner: dash(owner),
				Priority: plabel(prio), After: lsList(after), Only: lsList(only), Last: lr, Note: note})
			continue
		}
		if lr != "" {
			if note != "" {
				note = lr + " · " + note
			} else {
				note = lr
			}
		}
		fmt.Println(lsLine(name, proj, s, a, dash(owner), plabel(prio), note))
	}
	if js {
		lsPrintJSON(jrows)
		exit(0)
	}
	maPrint()
	exit(0)
}
