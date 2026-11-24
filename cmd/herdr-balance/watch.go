package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"herdr-tools/internal/pyjson"
	"herdr-tools/internal/selfexec"
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
func round(ctx context.Context, remote []string, seen map[string]seenAt) error {
	now := time.Now
	var pendErr error
	ts, err := tabs(ctx)
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
		if _, err := os.Stat(pendingPath(tid)); err == nil { // 떼다 만 맞춤이 남았다(R-26) — 목록이 같아도 바로 되붙인다
			delete(seen, tid)
			r, err := balance(ctx, main.Terminal, nil, nil)
			if err != nil { // 되붙이기가 계속 실패해도(떼인 채 닫힌 worker 등) 다른 탭은 이 회차에 맞춘다 — 오류는 끝에 돌려준다
				if pendErr == nil {
					pendErr = err
				}
				continue
			}
			fmt.Println(stamp(), "맞춤", pyjson.Dumps(r, false))
			continue
		}
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
		r, err := balance(ctx, main.Terminal, nil, nil)
		if err != nil {
			return err
		}
		fmt.Println(stamp(), "맞춤", pyjson.Dumps(r, false))
	}
	return pendErr
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

// watch: 2초 회차. ctx가 끝나면(TERM·INT) 돌던 herdr-app 호출을 끊고 돌아온다(worker를 떼던 맞춤만은 마치고 — balance)
func watch(ctx context.Context) {
	remote := remotePrefixes(ctx)
	seen := map[string]seenAt{}
	lastErr, hadErr := "", false
	self := selfexec.New("herdr-balance", func(l string) { fmt.Println(stamp(), l) })
	for {
		self.Check() // 회차 사이 — 새 판이 깔렸으면 그 판으로 이어 돈다(잠금은 exec 때 풀리고 새 판이 다시 쥔다)
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
		err := round(ctx, remote, seen)
		if ctx.Err() != nil {
			return // 끊은 호출의 오류는 적지 않는다
		}
		if err != nil {
			if !hadErr || err.Error() != lastErr { // 창이 없거나 일시 오류: 같은 오류는 한 번만 남기고 다음 회차에 다시
				fmt.Println(stamp(), err)
				lastErr, hadErr = err.Error(), true
			}
			continue
		}
		hadErr = false
	}
}
