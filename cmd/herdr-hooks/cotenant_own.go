package main

// cotenant-guard Go 판 — 임대 넘겨받기(claimgen·takeover)와 막힌 Bash 호출의 되돌리기(undoMade) (cotenant_lease.go에서 옮김 —
// 600줄 한도, gofix3-hooks R-M201)

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"time"

	"herdr-tools/internal/atomic"
)

// madeLease: 이번 호출이 만들거나 넘겨받은 임대 — 폴더·거기 쓴 주인 줄과, 넘겨받기면 그 전 상태(주인 줄·시각, 폴더 시각,
// 이번에 만든 claim, 다시 무장한 묵은 claim과 그 시각). 둘째 대상이 막히면 세대(주인 줄)를 확인한 뒤 전 상태로 되돌린다(R-M201)
type madeLease struct {
	ld, line string
	newDir   bool
	prev     []byte // 넘겨받기 전 주인 줄(없었으면 hadPrev=false)
	hadPrev  bool
	ownerMt  time.Time
	ldMt     time.Time
	claims   []string // 이번에 만든 claim 파일
	rearm    string   // 다시 무장한 묵은 claim(그 시각을 되돌린다)
	rearmMt  time.Time
}

// undoMade: 이번 호출이 만들거나 넘겨받은 임대를 뒤에서부터 되돌린다 — 주인 줄이 이번에 쓴 그대로일 때만(남이 손댄 세대는 그대로).
// 새 폴더는 주인 파일 하나뿐일 때 폴더째, 넘겨받기는 전 주인 줄(또는 없음)·시각으로 되돌리고 이번에 만든 claim을 지운다.
// 되돌리는 동안 이번 claim이 남아 있어 같은 세대를 남이 넘겨받지 못한다(claim은 맨 뒤에 지운다)
func (g *cg) undoMade() {
	for i := len(g.made) - 1; i >= 0; i-- {
		m := g.made[i]
		o := m.ld + "/owner"
		if b, err := os.ReadFile(o); err != nil || string(b) != m.line {
			continue
		}
		if m.newDir {
			ents, err := os.ReadDir(m.ld)
			if err != nil || len(ents) != 1 || ents[0].Name() != "owner" {
				continue
			}
			if os.Remove(o) == nil {
				os.Remove(m.ld) //nolint:errcheck // 그 사이 누가 넣었으면 남긴다
			}
			continue
		}
		if m.hadPrev {
			if atomic.WriteFile(o, m.prev, 0o666) != nil {
				continue // 전 주인 줄을 되돌리지 못하면 claim도 남긴다(넘겨받은 채 — 다음 판정이 주인으로 본다)
			}
			os.Chtimes(o, m.ownerMt, m.ownerMt) //nolint:errcheck // 시각은 10분 판정용 — 못 맞추면 더 오래 지킬 뿐
		} else if os.Remove(o) != nil {
			continue
		}
		if m.rearm != "" {
			os.Chtimes(m.rearm, m.rearmMt, m.rearmMt) //nolint:errcheck // 묵은 claim 시각 — 못 되돌리면 60초 더 기다릴 뿐
		}
		for _, c := range m.claims {
			os.Remove(c) //nolint:errcheck // 없으면 그만
		}
		os.Chtimes(m.ld, m.ldMt, m.ldMt) //nolint:errcheck // 폴더 시각(10초 「잡는 중」 판정) — 못 되돌리면 10초 기다릴 뿐
	}
	g.made = nil
}

// claimgen: 주인 한 세대를 잡는다 — 60초 넘은 claim은 남은 찌꺼기라, 먼저 다시 무장(mtime = 지금)한 뒤
// FILE.steal-<묵은 mtime>을 만든 한 쪽만 가진다(셸 판 S3·R1 설명 그대로). 잡으면 만든 파일·다시 무장한 claim을 g.got에
func (g *cg) claimgen(p string) bool {
	g.got = madeLease{}
	if claim(p) {
		g.got.claims = []string{p}
		return true
	}
	m, ok := lmtime(p)
	if !ok {
		g.claimAge = 0
		return false
	}
	g.claimAge = g.nowT() - m
	if g.claimAge < 60 {
		return false
	}
	if exists(p) {
		if f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666); err == nil {
			f.Close()
		} else {
			touch(p)
		}
	}
	s := p + ".steal-" + strconv.FormatInt(m, 10)
	if !claim(s) {
		g.claimAge = 0
		return false
	}
	g.got = madeLease{claims: []string{s}, rearm: p, rearmMt: time.Unix(m, 0)}
	return true
}

// takeover: 끝난 주인·멈춘 첫 획득·10분 넘게 쉰 깨끗한 산 주인의 임대를 넘겨받는다 — 넘겨받기 전 상태를 g.made에 남긴다(R-M201:
// 둘째 대상이 거절되면 첫 대상의 새 주인 줄이 남아, 돌지 않은 명령이 다음 소유 판정을 바꿨다)
func (g *cg) takeover(ld, rel string) {
	c := ld + "/claim-" + g.oPID + "-" + digitsOnly(g.oSince)
	if g.oPID+g.oSince == "" {
		c = ld + "/claim-orphan"
	}
	m := madeLease{ld: ld}
	if fi, err := os.Lstat(ld); err == nil {
		m.ldMt = fi.ModTime()
	}
	// 전 상태를 모르면(없음 말고 읽기 오류) 되돌릴 수 없다 — 넘겨받지 않는다(「전 주인 없음」으로 읽어 되돌릴 때 지우지 않게)
	fi, err := os.Lstat(ld + "/owner")
	if err == nil {
		m.ownerMt = fi.ModTime()
		m.prev, err = os.ReadFile(ld + "/owner")
		m.hadPrev = err == nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		g.block(rel + ": 넘겨받기 전 임대 주인을 읽지 못했다(" + err.Error() + "). 판정을 되돌릴 수 없어 막았다. 권한·디스크를 사람에게 알려라.")
	}
	if !g.claimgen(c) {
		g.block(fmt.Sprintf("%s: 다른 에이전트가 %d초 전 이 파일 임대를 넘겨받기 시작했다. 같은 편집을 곧바로 되풀이하지 말고 다른 일을 먼저 하라. 그 넘겨받기가 끝나지 않았다면 60초가 지나면 저절로 풀린다. 그 뒤에도 막히면 지휘에 알려라.", rel, g.claimAge))
	}
	m.claims, m.rearm, m.rearmMt = g.got.claims, g.got.rearm, g.got.rearmMt
	m.line = g.own(ld, rel)
	g.made = append(g.made, m)
}
