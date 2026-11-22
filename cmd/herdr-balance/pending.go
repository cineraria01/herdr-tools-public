package main

// 떼다 만 맞춤의 복구(R-26): worker를 떼기 전에 목표 배치를 <탭>.pending.json에 적고, 맞춤이 끝나 상태를 저장하면 지운다

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/proc"
)

// pending: worker를 떼기 전에 적는 목표 배치(<탭>.pending.json — R-26). 맞춤이 끝나 상태를 저장하면 지운다.
// 남아 있으면 그 실행이 떼다 말았다(KILL·기계 재시작·오류) — 다음 맞춤이 떼인 채 남은 worker를 되붙인다
type pending struct {
	PID     int      `json:"pid"`
	Start   string   `json:"start"` // 그 실행의 시작 시각(proc.StartTime) — pid 재사용 대조, 모르면 빈 값
	Workers []string `json:"workers"`
}

func pendingPath(tabID string) string {
	return filepath.Join(stateDir(), strings.ReplaceAll(tabID, "/", "__")+".pending.json")
}

func savePending(tabID string, workers []string) error {
	st, _ := proc.StartTime(os.Getpid()) // 모르면 빈 값 — 다음 실행은 이 pid가 살아 있는 동안 보류한다
	b, err := json.Marshal(pending{PID: os.Getpid(), Start: st, Workers: append([]string{}, workers...)})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir(), 0o777); err != nil {
		return fmt.Errorf("상태 폴더 만들기: %w", err)
	}
	if err := atomic.WriteFile(pendingPath(tabID), b, 0o644); err != nil {
		return fmt.Errorf("미완료 배치 저장: %w", err)
	}
	return nil
}

// leftover: 앞 실행이 떼다 만 목표 worker. 없으면 nil. 그 실행이 아직 돌거나(같은 pid·시작 시각) 살았는지 모르거나
// 파일을 못 읽으면 오류 — 보류(떼는 중인 남의 맞춤 밑에서 되붙이지 않는다)
func leftover(tabID string) ([]string, error) {
	p := pendingPath(tabID)
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("미완료 배치 %s 못 읽음: %w", p, err)
	}
	var pd pending
	if err := json.Unmarshal(b, &pd); err != nil || pd.PID <= 0 || pd.Workers == nil {
		return nil, fmt.Errorf("미완료 배치 %s 를 못 읽음(꼴이 틀림) — 보류", p)
	}
	if pd.PID != os.Getpid() && proc.Alive(pd.PID) { // 같은 pid면 이 실행(--watch)이 앞 회차에 떼다 만 것
		st, err := proc.StartTime(pd.PID)
		if err != nil || pd.Start == "" || st == pd.Start {
			return nil, fmt.Errorf("다른 실행(pid %d)이 %s 탭을 맞추는 중이거나 모름 — 보류(%s)", pd.PID, tabID, p)
		}
	}
	return pd.Workers, nil
}
