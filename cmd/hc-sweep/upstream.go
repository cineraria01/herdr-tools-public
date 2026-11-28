package main

// 상대 판 기록(ht-upstream-contract, docs/hc-sweep/upstream.md — 사용자 「문제점들 다 고치자」 → 「다섯 묶음 모두 (추천)」. 업계의 계약 시험
// contract test: 기대는 목록으로 두고 상대 판이 바뀔 때마다 진짜 상대에게 돌려 본다). 건강 점검 바퀴가 도는 김에 이 기계의 판 묶음(살아 있는
// Claude 세션 파일의 version × hc --version)을 읽어 internal/upstream 기록에 적는다. 처음 본 묶음들은 믿는 기준(보류 없이 — 지휘 정정 2,
// 점검은 hc-fill이 한 대씩 차례로), 그 뒤 새로 나타난 묶음은 보류(쉼 판정이 그 판 터미널에 모름)이고 운영 사건 hc-upstream 소식 한 번.
// 판을 못 읽으면 모름 = 사건 없음·기록 그대로. 실물 점검은 upstreamprobe.go

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/proc"
	"herdr-tools/internal/rest"
	"herdr-tools/internal/upstream"
)

// hcVersion: 이 기계 hc 판(「hc 0.1.22」의 끝 칸). 못 읽으면 빈 값
func hcVersion(r runner) string {
	res := r(15*time.Second, nil, "hc", "--version")
	f := strings.Fields(res.out)
	if res.err != nil || res.rc != 0 || len(f) != 2 || f[0] != "hc" {
		return ""
	}
	return f[1]
}

// cliVersions: 살아 있는 Claude 세션 파일들의 판(이름 = pid, 그 pid가 살아 있는 것만 — 죽은 세션이 남긴 파일의 옛 판을 새 판으로 읽지 않게)
func cliVersions() []string {
	fis, _ := filepath.Glob(filepath.Join(rest.SessDir, "*.json"))
	seen := map[string]bool{}
	var vs []string
	for _, p := range fis {
		pid, err := strconv.Atoi(strings.TrimSuffix(filepath.Base(p), ".json"))
		if err != nil || pid <= 0 || !proc.Alive(pid) {
			continue
		}
		var m struct {
			Version string `json:"version"`
		}
		if b, err := os.ReadFile(p); err != nil || json.Unmarshal(b, &m) != nil || m.Version == "" || seen[m.Version] {
			continue
		}
		seen[m.Version] = true
		vs = append(vs, m.Version)
	}
	sort.Strings(vs)
	return vs
}

// upstreamCheck: 한 바퀴. report(상태, 글, 키)는 운영 사건 한 줄. 찍을 줄들
func upstreamCheck(machine, path string, dry bool, r runner, clis []string, report func(st, msg, key string) bool) []string {
	hcv := hcVersion(r)
	if hcv == "" {
		return []string{"판 기록: 모름(hc --version 못 읽음) — 기록 그대로"}
	}
	var lines []string
	now := time.Now().Format(time.RFC3339)
	err := upstream.Update(path, func(rec *upstream.Rec) *upstream.Rec {
		if rec == nil {
			if len(clis) == 0 { // 세션이 하나도 없으면 기준을 아직 적지 않는다(나중 첫 세션의 판을 「바뀜」으로 읽지 않게)
				return nil
			}
			rec = &upstream.Rec{Bundles: map[string]*upstream.Bundle{}}
			for _, c := range clis {
				rec.Bundles[upstream.ID(c, hcv)] = &upstream.Bundle{State: upstream.Trusted, Base: true, Seen: now}
			}
			rec.HC, rec.Last, rec.Live = hcv, upstream.ID(clis[len(clis)-1], hcv), liveIDs(clis, hcv)
			lines = append(lines, "판 기록: 기준을 적음(믿음, 점검은 차례로) — "+strings.Join(keys(rec.Bundles), " "))
			if dry {
				return nil
			}
			return rec
		}
		rec.HC, rec.Live = hcv, liveIDs(clis, hcv) // 세션이 끝난 묶음은 보류·점검 대상에서 빠진다(기록은 남김 — 다시 보이면 사건을 되풀이하지 않게)
		for _, c := range clis {
			if id := upstream.ID(c, hcv); rec.Bundles[id] == nil {
				rec.Bundles[id] = &upstream.Bundle{State: upstream.Pending, Seen: now}
			}
		}
		for _, id := range keys(rec.Bundles) {
			b := rec.Bundles[id]
			if b.State != upstream.Pending || b.HasTold("new") {
				continue
			}
			msg := fmt.Sprintf("에이전트 판 바뀜(%s): %s → %s — 실물 점검을 통과하기 전까지 이 판의 에이전트 터미널은 닫기 보류(쉼 모름, R3는 그대로)",
				machine, dashS(rec.Last), id)
			if dry {
				lines = append(lines, "판 기록 (보낼 것): "+msg)
				continue
			}
			if !report("중간", msg, "hc-upstream:"+machine+":"+id+":new") {
				lines = append(lines, "판 기록: "+msg+" — 알림 실패, 다음 바퀴에 같은 키로")
				continue
			}
			b.Told = append(b.Told, "new")
			rec.Last = id
			lines = append(lines, "판 기록: "+msg)
		}
		if dry {
			return nil
		}
		return rec
	})
	if err != nil {
		lines = append(lines, "판 기록을 못 씀: "+err.Error())
	}
	return lines
}

func liveIDs(clis []string, hcv string) []string {
	var o []string
	for _, c := range clis {
		o = append(o, upstream.ID(c, hcv))
	}
	return o
}

func keys(m map[string]*upstream.Bundle) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func dashS(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// upstreamPrint: hc-sweep --upstream — 판 기록과 묶음마다 마지막 점검 표(재지 않는다). 0 보류 없음 · 1 보류 있음 · 3 기록 없음·못 읽음
func upstreamPrint(path string) int {
	rec, err := upstream.Load(path)
	if err != nil || rec == nil {
		fmt.Printf("판 기록 없음·못 읽음(%s) — 이 기계 hc-sweep 건강 점검이 아직 Claude 세션을 못 봤다\n", path)
		return 3
	}
	fmt.Printf("hc %s · 마지막 판 %s\n", rec.HC, dashS(rec.Last))
	rc := 0
	for _, id := range keys(rec.Bundles) {
		b := rec.Bundles[id]
		base := ""
		if b.Base {
			base = " (처음 기준)"
		}
		fmt.Printf("%s: %s%s · 점검 %s %s\n", id, b.State, base, dashS(b.Result), dashS(b.Probed))
		for _, row := range b.Rows {
			fmt.Println("  " + strings.ReplaceAll(row, "\t", " | "))
		}
		if b.State != upstream.Trusted {
			rc = 1
		}
	}
	return rc
}
