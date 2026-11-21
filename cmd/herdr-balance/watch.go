package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"herdr-tools/internal/pyjson"
)

func sameSet(a, b []string) bool {
	m := map[string]bool{}
	for _, x := range a {
		m[x] = true
	}
	n := map[string]bool{}
	for _, x := range b {
		if !m[x] {
			return false
		}
		n[x] = true
	}
	return len(m) == len(n)
}

type seenAt struct {
	ws []string
	at time.Time
}

// round: --watch 한 회차. 맞춘 탭마다 「맞춤 {…}」 한 줄. 오류면 그 회차를 거기서 멈춘다(셸 판과 같다)
func round(remote []string, seen map[string]seenAt) error {
	now := time.Now
	ts, err := tabs()
	if err != nil {
		return err
	}
	for _, t := range ts {
		tid := t.ID
		if (strings.Contains(tid, "/") && !hasPrefix(tid, remote)) || len(t.Terminals) == 0 {
			continue
		}
		main := t.Terminals[0]
		if !truthy(main.App) {
			continue // 첫 터미널이 에이전트(지휘)인 탭만
		}
		ws := workersOf(t, main.Terminal)
		last, ok := saved(tid)
		if !ok {
			if err := save(tid, ws); err != nil { // 처음 보는 탭은 지금 배치를 기준으로 삼는다
				return err
			}
			continue
		}
		if sameSet(ws, last) {
			delete(seen, tid)
			continue
		}
		prev, ok := seen[tid]
		if !ok || !sameSet(prev.ws, ws) {
			seen[tid] = seenAt{ws, now()}
			continue
		}
		if now().Sub(prev.at) < 4*time.Second {
			continue
		}
		delete(seen, tid)
		r, err := balance(main.Terminal, nil, nil)
		if err != nil {
			return err
		}
		fmt.Println(stamp(), "맞춤", pyjson.Dumps(r, false))
	}
	return nil
}

func hasPrefix(s string, ps []string) bool {
	for _, p := range ps {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func stamp() string { return time.Now().Format("2006-01-02 15:04:05") }

// watch: 2초 회차. ctx가 끝나면(TERM·INT) 그 회차를 마치고 돌아온다
func watch(ctx context.Context) {
	remote := remotePrefixes()
	seen := map[string]seenAt{}
	lastErr, hadErr := "", false
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
		if err := round(remote, seen); err != nil {
			if !hadErr || err.Error() != lastErr { // 창이 없거나 일시 오류: 같은 오류는 한 번만 남기고 다음 회차에 다시
				fmt.Println(stamp(), err)
				lastErr, hadErr = err.Error(), true
			}
			continue
		}
		hadErr = false
	}
}
