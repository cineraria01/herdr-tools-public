package main

// 판 맞추기 — hc-core-relaunch --update <기계>… · --all (2026-11-24 core-update-cmd). 손 절차(hc machine update → 맥 계열은 코어가
// SSH 문맥으로 떠 키체인이 막히니 다시 띄우기 → 코어 문맥·키체인 확인)를 한 명령으로. 기준 판은 이 기계의 hc(`hc -V`).
// 도구는 재고(상태 먼저), 손잡이(--busy·--timeout·--dry-run)를 주고, 판단(언제·어느 기계)은 부르는 쪽(지휘)이 한다.
// 실패로 치는 것: 판을 모름(연결 안 됨·목록에 없음·OS 모름) · 업데이트·다시 띄우기 실패 · 뒤에도 판이 다름 · 사라진 터미널 ·
// (macOS) 코어 문맥·키체인 점검이 통과가 아님 · 전체 상한 넘김. 「모름」을 성공·한가함으로 읽지 않는다.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	wm "herdr-tools/internal/workermac"
)

// specLabel: 병합기 추측 검증 일꾼 터미널의 라벨(cmd/hc-merger) — 그 기계는 「검증 중」
const specLabel = "hc-merger-spec=1"

type machine struct {
	Name    string  `json:"name"`
	State   string  `json:"state"`
	Version *string `json:"version"`
}

type term struct {
	Address string  `json:"address"`
	Name    *string `json:"name"`
	State   string  `json:"state"`
}

// on: 터미널 주소의 기계(「<기계>/<주소>」, 앞머리 없으면 이 기계 = "")
func (t term) on() string {
	if i := strings.IndexByte(t.Address, '/'); i > 0 {
		return t.Address[:i]
	}
	return ""
}

func (t term) name() string {
	if t.Name == nil || *t.Name == "" {
		return t.Address
	}
	return *t.Name
}

// hcJSON: hc … --json을 v로 푼다. 실패는 err(모름)
func hcJSON(ctx context.Context, v any, args ...string) error {
	out, rc := wm.Output(ctx, 60, nil, "hc", args...)
	if rc != 0 {
		return fmt.Errorf("hc %s 실패(종료 %d)", strings.Join(args, " "), rc)
	}
	if err := json.Unmarshal([]byte(out), v); err != nil {
		return fmt.Errorf("hc %s 출력을 못 읽음: %w", strings.Join(args, " "), err)
	}
	return nil
}

func machines(ctx context.Context) (map[string]machine, []string, error) {
	var v struct {
		Machines []machine `json:"machines"`
	}
	if err := hcJSON(ctx, &v, "machine", "ls", "--json"); err != nil {
		return nil, nil, err
	}
	r, order := map[string]machine{}, []string{}
	for _, m := range v.Machines {
		r[m.Name] = m
		order = append(order, m.Name)
	}
	return r, order, nil
}

// terms: 기계 m의 터미널(all이면 끝난 것도, label이 있으면 그 라벨만)
func terms(ctx context.Context, m string, all bool, label string) ([]term, error) {
	args := []string{"ls", "--json"}
	if all {
		args = append(args, "--all")
	}
	if label != "" {
		args = append(args, "--label", label)
	}
	var v struct {
		Terminals []term `json:"terminals"`
	}
	if err := hcJSON(ctx, &v, args...); err != nil {
		return nil, err
	}
	var r []term
	for _, t := range v.Terminals {
		if t.on() == m && (all || t.State == "live") {
			r = append(r, t)
		}
	}
	return r, nil
}

// baseVersion: 이 기계 hc의 판(「hc 0.1.22」의 둘째 낱말)
func baseVersion(ctx context.Context) (string, error) {
	out, rc := wm.Output(ctx, 30, nil, "hc", "-V")
	f := strings.Fields(out)
	if rc != 0 || len(f) < 2 {
		return "", fmt.Errorf("hc -V 실패(종료 %d) — 기준 판을 모른다", rc)
	}
	return f[1], nil
}

func osOf(ctx context.Context, m string) string {
	out, rc := wm.Output(ctx, 30, nil, "hc", "exec", "--on", m, "--", "uname", "-s")
	if rc != 0 {
		return ""
	}
	return strings.TrimSpace(out)
}

type result struct{ m, kind, why string }

func update(ctx context.Context, o opts, w io.Writer) int {
	base, err := baseVersion(ctx)
	if err != nil {
		fmt.Fprintln(w, "✗ "+err.Error())
		return 1
	}
	ms, order, err := machines(ctx)
	if err != nil {
		fmt.Fprintln(w, "✗ 기계 목록을 모른다: "+err.Error())
		return 1
	}
	targets := o.ms
	if o.all {
		targets = order
	}
	status(ctx, w, base, ms, targets)
	var rs []result
	for _, m := range targets {
		fmt.Fprintln(w, "== "+m)
		k, why := one(ctx, o, w, base, ms, m)
		fmt.Fprintf(w, "→ %s: %s %s\n", m, k, why)
		rs = append(rs, result{m, k, why})
	}
	fmt.Fprintln(w, "-- 결과 (기준 hc "+base+")")
	rc := 0
	for _, r := range rs {
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.m, r.kind, r.why)
		switch {
		case r.kind == "실패":
			rc = 1
		case r.kind == "건너뜀" && rc == 0:
			rc = 3
		}
	}
	return rc
}

// status: 하기 전에 기계마다 판·연결·도는 터미널·병합 줄을 보인다(판단은 부르는 쪽이). 못 읽으면 「모름」
func status(ctx context.Context, w io.Writer, base string, ms map[string]machine, targets []string) {
	fmt.Fprintln(w, "-- 상태 (기준 hc "+base+")")
	for _, m := range targets {
		mi, ok := ms[m]
		if !ok {
			fmt.Fprintf(w, "%s\t목록에 없음\n", m)
			continue
		}
		line := fmt.Sprintf("%s\t판 %s\t%s", m, ver(mi), mi.State)
		if ts, err := terms(ctx, m, false, ""); err != nil {
			line += "\t터미널 모름"
		} else {
			var lanes, merge []string
			for _, t := range ts {
				if n := t.name(); strings.HasPrefix(n, "merger-") {
					merge = append(merge, n)
				} else {
					lanes = append(lanes, n)
				}
			}
			line += fmt.Sprintf("\t터미널 %d: %s\t병합기: %s", len(ts), list(lanes), list(merge))
		}
		if sp, err := terms(ctx, m, false, specLabel); err != nil {
			line += "\t검증 중: 모름"
		} else {
			line += "\t검증 중: " + list(names(sp))
		}
		fmt.Fprintln(w, line)
	}
}

func ver(mi machine) string {
	if mi.Version == nil || *mi.Version == "" {
		return "모름"
	}
	return *mi.Version
}

func names(ts []term) []string {
	var r []string
	for _, t := range ts {
		r = append(r, t.name())
	}
	return r
}

func list(s []string) string {
	if len(s) == 0 {
		return "없음"
	}
	return strings.Join(s, ", ")
}

// pollSec: --busy wait에서 다시 보는 간격(초) — HC_CORE_RELAUNCH_POLL(시험이 줄인다)
func pollSec() time.Duration {
	if n, err := strconv.Atoi(os.Getenv("HC_CORE_RELAUNCH_POLL")); err == nil && n >= 0 {
		return time.Duration(n) * time.Second
	}
	return 15 * time.Second
}

// one: 기계 하나 — 결과 종류(같음·성공·할 일·건너뜀·실패)와 까닭
func one(ctx context.Context, o opts, w io.Writer, base string, ms map[string]machine, m string) (string, string) {
	mi, ok := ms[m]
	switch {
	case late(ctx):
		return "실패", fmt.Sprintf("전체 상한(%d초) 넘김 — 손대지 않음", o.timeout)
	case !ok:
		return "실패", "hc 기계 목록에 없음(hc machine ls)"
	case mi.State != "connected":
		return "실패", "연결 안 됨(" + mi.State + ") — 판을 모른다"
	case ver(mi) == base:
		return "같음", "판 " + base + " — 안 함"
	}
	from := ver(mi)
	osName := osOf(ctx, m)
	if osName != "Darwin" && osName != "Linux" {
		return "실패", "OS를 모른다(hc exec uname -s: " + or(osName, "응답 없음") + ")"
	}
	plan := "hc machine update " + m + " (" + from + " → " + base + ")"
	if osName == "Darwin" {
		plan += " · 다시 띄우기 · 코어 문맥·키체인 점검"
	}
	// 검증 중인 기계: 손잡이 --busy (skip 기본 · wait · go)
	for {
		sp, err := terms(ctx, m, false, specLabel)
		if err != nil {
			return "실패", "병합기 검증 터미널을 모른다: " + err.Error()
		}
		if len(sp) == 0 || o.busy == "go" {
			break
		}
		if o.dry || o.busy == "skip" {
			if o.dry {
				return "할 일", plan + " — 지금은 검증 중(" + list(names(sp)) + "), --busy " + o.busy
			}
			return "건너뜀", "병합기 검증 중(" + list(names(sp)) + ") — 기다리려면 --busy wait, 그대로 하려면 --busy go"
		}
		fmt.Fprintf(w, "기다림: 검증 중 %s\n", list(names(sp)))
		select {
		case <-ctx.Done():
			return "실패", fmt.Sprintf("검증이 끝나기를 기다리다 전체 상한(%d초) 넘김", o.timeout)
		case <-time.After(pollSec()):
		}
	}
	if o.dry {
		return "할 일", plan
	}
	before, err := terms(ctx, m, false, "")
	if err != nil {
		return "실패", "업데이트 전 터미널 목록을 모른다: " + err.Error()
	}
	fmt.Fprintln(w, "업데이트: "+plan)
	c := wm.Command(ctx, "hc", "machine", "update", m)
	c.Stdout, c.Stderr = w, w
	if r := wm.Run(ctx, c); r != 0 {
		return "실패", "hc machine update " + failWhy(ctx, r, o.timeout)
	}
	if osName == "Darwin" {
		if r := relaunch(ctx, w, m); r != 0 {
			return "실패", "다시 띄우기 " + failWhy(ctx, r, o.timeout)
		}
	}
	// 다시 붙을 때까지 본다(전체 상한 안) — 붙기 전 목록에는 그 기계 터미널이 없어 모두 「사라짐」으로 잘못 보인다
	var ms2 map[string]machine
	for {
		if ms2, _, err = machines(ctx); err != nil {
			return "실패", "업데이트 뒤 기계 상태를 모른다: " + err.Error()
		}
		if ms2[m].State == "connected" {
			break
		}
		select {
		case <-ctx.Done():
			return "실패", fmt.Sprintf("업데이트 뒤 다시 연결되지 않음(%s) — 터미널·판 확인 못 함, 전체 상한(%d초)", or(ms2[m].State, "목록에 없음"), o.timeout)
		case <-time.After(pollSec()):
		}
	}
	// 끝난 것까지(--all) 받아 전에 살아 있던 터미널이 지금도 live인지 본다 — 그 사이 끝난 것(finished)은 스스로 끝났는지
	// 재시작에 죽었는지 가릴 수 없으니 실패로 알린다(모름은 모름)
	after, err := terms(ctx, m, true, "")
	if err != nil {
		return "실패", "업데이트 뒤 터미널 목록을 모른다: " + err.Error()
	}
	if gone := vanished(before, after); len(gone) > 0 {
		return "실패", "사라지거나 끝난 터미널: " + strings.Join(gone, ", ")
	}
	if v := ver(ms2[m]); v != base {
		return "실패", "업데이트 뒤에도 판 " + v + " (기준 " + base + ")"
	}
	if osName == "Darwin" {
		if ok, why := check(ctx, w, m); !ok {
			return "실패", why
		}
		return "성공", from + " → " + base + ", 다시 띄움, 코어 문맥·키체인 통과"
	}
	return "성공", from + " → " + base + " (리눅스 — 업데이트만)"
}

// vanished: before(live)에 있었는데 after에 없거나 live가 아닌 터미널 — 「이름 (주소) 없어짐」·「이름 (주소) <상태> — …」
func vanished(before, after []term) []string {
	st := map[string]string{}
	for _, t := range after {
		st[t.Address] = t.State
	}
	var r []string
	for _, t := range before {
		switch s, ok := st[t.Address]; {
		case !ok:
			r = append(r, t.name()+" ("+t.Address+") 없어짐")
		case s != "live":
			r = append(r, t.name()+" ("+t.Address+") "+or(s, "상태 모름")+" — 재시작에 죽었는지 확인")
		}
	}
	sort.Strings(r)
	return r
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
