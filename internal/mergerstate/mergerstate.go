// Package mergerstate: 병합기 상태 한 장(docs/hc-lane/merger-state.md — ht-merger-state-record, 2026-11-26 사용자 「구조를 다시 짜기」).
// 병합기(hc-merger)가 클론의 <git-path hc-merger>/state.json에 쓰고, 읽는 쪽(hc-fill)은 hc-merger --state로 받은 이 꼴만 읽는다.
// 읽는 쪽은 병합기의 터미널 이름·기록 파일 꼴을 모른다 — 기다리는 대상은 주소로, 진행은 progress_at으로.
package mergerstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Version: 기록 판. 읽는 쪽이 모르는 판이면 Parse가 오류(모름)
const Version = 1

// File: 기록 파일 이름(병합기 기록 폴더 안)
const File = "state.json"

// 단계
const (
	Idle     = "idle"      // 줄 빔·할 일 없음
	Round    = "round"     // 그 밖 바퀴 일(큐 읽기·풀이·미리 훑기·일시 실패로 쉼 — Why에 까닭)
	SpecWait = "spec-wait" // 맨 앞 추측 검증 기다림
	LoadWait = "load-wait" // 맨 앞 추측 일꾼이 그 기계 부하가 기준 아래로 내려가길 기다림(정상 대기 — Why에 기계·부하·기준)
	Land     = "land"      // 병합기 기계에서 착지
	LandOn   = "land-on"   // 워커에서 착지 실행
	PrevLand = "prev-land" // 앞 병합기 실행이 남긴 착지 자식이 아직 돎
)

// Wait: 기다리는 대상 하나 — 기계, 터미널 주소(못 찾았으면 빈 값), 역할(사람이 읽는 글), 띄운 시각.
// Name: 띄울 때 준 터미널 이름(Rec.Agents만 — 읽는 쪽이 주소와 함께 맞춘다, 이름 꼴은 몰라도 된다)
type Wait struct {
	Machine string `json:"machine"`
	Addr    string `json:"addr"`
	Name    string `json:"name,omitempty"`
	Role    string `json:"role"`
	At      int64  `json:"at"`
}

// Inner: 착지 안쪽 단계 — 착지 스크립트가 HERDR_LAND_PHASE 파일에 쓴 한 줄, 병합기가 그 줄을 처음 본 때
type Inner struct {
	Line string `json:"line"`
	At   int64  `json:"at"`
}

// Rec: 기록 한 장
type Rec struct {
	V        int      `json:"v"`
	At       int64    `json:"at"`          // 기록 시각
	Progress int64    `json:"progress_at"` // 마지막 진행 시각
	Probe    int64    `json:"probe_at"`    // 기다리는 일꾼의 진행을 마지막으로 잰 때(병합기 안)
	Machine  string   `json:"machine"`
	Term     string   `json:"term"`
	Base     string   `json:"base"`
	Phase    string   `json:"phase"`
	Since    int64    `json:"phase_since"`
	Why      string   `json:"why"`
	Front    string   `json:"front"`
	Members  []string `json:"members"`
	Queue    []string `json:"queue"`
	Waits    []Wait   `json:"waits"`
	Inner    *Inner   `json:"inner,omitempty"`
	LandOn   string   `json:"land_on"` // 마지막 착지 실행 결정 「<기계>(<까닭>)」(이력)
	// Agents: 병합기가 띄워 지금 맡긴 에이전트(충돌 풀이·풀이 감사 — 주소·이름) — hc-fill 자리 셈이 대기열 쪽 도구가 띄운 것으로 센다
	// (ht-seat-working-only). 옛 기록에는 없다(빈 목록)
	Agents []Wait `json:"agents,omitempty"`
}

// Landing: 착지 단계인가(병합기 기계·워커)
func (r Rec) Landing() bool { return r.Phase == Land || r.Phase == LandOn }

// Parse: 기록 한 장. 꼴이 틀리거나 모르는 판이면 오류
func Parse(b []byte) (Rec, error) {
	var r Rec
	if err := json.Unmarshal(b, &r); err != nil {
		return Rec{}, fmt.Errorf("병합기 기록 꼴이 틀림: %w", err)
	}
	if r.V != Version {
		return Rec{}, fmt.Errorf("모르는 병합기 기록 판 %d", r.V)
	}
	return r, nil
}

// Read <기록 폴더>: 없으면 os.IsNotExist 오류
func Read(dir string) (Rec, error) {
	b, err := os.ReadFile(filepath.Join(dir, File))
	if err != nil {
		return Rec{}, err
	}
	return Parse(b)
}

// Write <기록 폴더>: 같은 폴더 임시 파일 → rename(반쯤 쓴 판 없음). V는 늘 Version
func Write(dir string, r Rec) error {
	r.V = Version
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("병합기 기록 만들기: %w", err)
	}
	f, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return fmt.Errorf("병합기 기록 임시 파일: %w", err)
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		os.Remove(f.Name())
		return fmt.Errorf("병합기 기록 쓰기: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return fmt.Errorf("병합기 기록 닫기: %w", err)
	}
	if err := os.Rename(f.Name(), filepath.Join(dir, File)); err != nil {
		os.Remove(f.Name())
		return fmt.Errorf("병합기 기록 바꿔 넣기: %w", err)
	}
	return nil
}
