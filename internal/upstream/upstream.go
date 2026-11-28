// Package upstream: 「도구가 믿는 상대의 판」 기록 한 곳(ht-upstream-contract, docs/hc-sweep/upstream.md). herdr-tools는 에이전트 CLI(Claude)와
// hc의 문서화되지 않은 동작(세션 파일·기록 자리·끝 표시·hc get 칸)에 기댄다 — 상대 판이 바뀌면 그 기대를 진짜 상대에게 다시 돌려 볼 때까지
// (업계의 계약 시험 contract test) 그 판의 터미널을 닫는 판정을 믿지 않는다. 판 묶음 = 세션의 CLI 판 + 그 기계 hc 판. 기계마다 한 파일이고
// hc-sweep만 쓴다(바퀴의 판 기록·실물 점검 결과), 쉼 판정(internal/rest)·hc-lane merger 점검 줄이 읽는다
package upstream

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/lockx"
)

// 판 묶음 상태
const (
	Trusted = "trusted" // 믿음: 실물 점검 통과 또는 처음 깔릴 때의 기준
	Pending = "pending" // 바뀐 뒤 아직 점검 안 됨(보류)
	Failed  = "failed"  // 실물 점검에서 틀림(보류 이어짐)
)

// Bundle: 판 묶음 하나. Base = 처음 깔릴 때 적은 기준(믿되 점검은 아직 — 차례로 한 번). Told = 보낸 사건(new·fail·up)
type Bundle struct {
	State  string   `json:"state"`
	Base   bool     `json:"base,omitempty"`
	Seen   string   `json:"seen"`
	Probed string   `json:"probed,omitempty"` // 마지막 점검 때(RFC3339)
	Result string   `json:"result,omitempty"` // pass · fail · unknown
	Rows   []string `json:"rows,omitempty"`   // 마지막 점검 표(항목<탭>맞음|틀림|못 봄|참고<탭>자세히)
	Told   []string `json:"told,omitempty"`
	// Skip: 점검이 이 묶음을 맡았는데 설치본은 다른 판이었다(옛 판으로 도는 옛 세션) — 점검으로 잴 수 없어 다시 띄우지 않는다
	Skip bool `json:"unmeasurable,omitempty"`
}

// Rec: 기계 하나의 기록. HC = 마지막으로 읽은 그 기계 hc 판(세션 판과 묶어 그 터미널의 판 묶음이 된다)
type Rec struct {
	HC      string             `json:"hc"`
	Last    string             `json:"last,omitempty"` // 마지막으로 본 가장 새 판 묶음(사건 글의 「옛 판」)
	Bundles map[string]*Bundle `json:"bundles"`
	// Live: 마지막 바퀴에 살아 있는 세션의 묶음(감사 atp89b9 중간 1 — 보류·점검 대상은 이것만: 세션이 끝난 판은 닫을 것이 없다)
	Live []string `json:"live,omitempty"`
}

// Path: 기록 파일 — <XDG_STATE_HOME 또는 ~/.local/state>/hc-sweep/upstream.json. HERDR_UPSTREAM_FILE이면 그 파일(손 점검·시험용 사본 —
// XDG_STATE_HOME을 바꾸면 hc 소켓 자리까지 바뀐다)
func Path() string {
	if f := os.Getenv("HERDR_UPSTREAM_FILE"); f != "" {
		return f
	}
	s := os.Getenv("XDG_STATE_HOME")
	if s == "" {
		s = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	return filepath.Join(s, "hc-sweep", "upstream.json")
}

var bad = regexp.MustCompile(`[^A-Za-z0-9.-]`)

// ID: 판 묶음 이름 — 사건 키 글자(A-Za-z0-9_.:/-) 안에서만(판 글자의 다른 글자는 -로). 둘 중 하나라도 비면 빈 값(판 모름)
func ID(cli, hc string) string {
	if cli == "" || hc == "" {
		return ""
	}
	return "claude-" + bad.ReplaceAllString(cli, "-") + "_hc-" + bad.ReplaceAllString(hc, "-")
}

// Load: 기록. 파일이 없으면 nil·nil(아직 안 깔림 — 보류 없음), 못 읽거나 꼴이 틀리면 오류(부르는 쪽이 모름으로)
func Load(p string) (*Rec, error) {
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r Rec
	if err := json.Unmarshal(b, &r); err != nil || r.Bundles == nil {
		return nil, fmt.Errorf("판 기록 꼴이 틀림(%s)", p)
	}
	return &r, nil
}

// Update: 잠금 안에서 읽고 f로 고쳐 쓴다(바퀴와 실물 점검이 같은 파일을 쓴다). f가 nil 기록을 받으면 새로 만들 수 있다 — 돌려준 기록이 nil이면 안 쓴다
func Update(p string, f func(*Rec) *Rec) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	l, err := lockx.Lock(p + ".lock")
	if err != nil {
		return err
	}
	defer l.Close()
	r, err := Load(p)
	if err != nil {
		return err
	}
	if r = f(r); r == nil {
		return nil
	}
	b, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		return err
	}
	return atomic.WriteFile(p, append(b, '\n'), 0o644)
}

// Hold: 판 cli(세션 파일의 version)인 터미널을 닫는 판정을 믿지 못할 까닭(빈 값 = 믿음). 기록이 없으면 믿음(처음 깔릴 때 보류 없이 —
// 지휘 정정 2), 판을 못 읽으면 믿지 않음(꼴이 바뀐 판일 수 있다 — 닫지 않는 쪽)
func (r *Rec) Hold(cli string) string {
	if r == nil {
		return ""
	}
	id := ID(cli, r.HC)
	if id == "" {
		return "판 모름(세션 판 " + dash(cli) + " · hc " + dash(r.HC) + ")"
	}
	switch b := r.Bundles[id]; {
	case b == nil || b.State == Pending:
		return "이 판은 아직 확인 안 됨(" + id + ")"
	case b.State == Failed:
		return "이 판은 실물 점검에서 틀림(" + id + ")"
	case b.State != Trusted:
		return "판 상태 모름(" + id + " " + b.State + ")"
	}
	return ""
}

// Owed: 살아 있는 묶음(Live) 가운데 점검할 것(보류인 것 먼저, 없으면 아직 점검 안 한 기준 — 이름순, 잴 수 없음은 빼고)과 보류인 살아
// 있는 묶음이 있는가. 틀린 묶음은 다시 점검하지 않는다(새 판이 와야 바뀐다 — 손으로는 hc-sweep --upstream-probe). 세션이 다 끝난 묶음은
// 보류도 점검 대상도 아니다
func (r *Rec) Owed() (string, bool) {
	if r == nil {
		return "", false
	}
	ids := append([]string(nil), r.Live...)
	sort.Strings(ids)
	held, pend, base := false, "", ""
	for _, id := range ids {
		b := r.Bundles[id]
		switch {
		case b == nil:
		case b.State == Pending:
			held = true
			if pend == "" && !b.Skip {
				pend = id
			}
		case b.State == Failed:
			held = true
		case b.Base && b.Probed == "" && !b.Skip && base == "":
			base = id
		}
	}
	if pend != "" {
		return pend, held
	}
	return base, held
}

// HasTold: 그 사건(new·fail·up)을 보냈나
func (b *Bundle) HasTold(k string) bool {
	for _, t := range b.Told {
		if t == k {
			return true
		}
	}
	return false
}

func dash(s string) string {
	if s = strings.TrimSpace(s); s == "" {
		return "-"
	}
	return s
}
