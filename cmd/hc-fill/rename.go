package main

// 기계 이름 바꾸기 hc-fill machine --rename <옛> <새> [--dry-run] [--json] [--why <까닭>] (2026-11-24 machine-rename,
// docs/hc-fill/machine-rename.md — 사용자 「테일 스케일 컴퓨터 이름들과 동일하게 해서 좀 여러 컴퓨터가 안헷갈리면 좋겠는데」).
// run.lock·대기열 잠금 안에서 설정 파일의 기계 칸, 상태 파일의 기계 칸·목록 항목·주소 접두, 기계별 손잡이(seats@·pause@),
// 병합기 켬 표식(refs/herdr/merger/<기계>)을 한 번에 옮긴다. 글 칸(요약·까닭·사건 글)은 건드리지 않고, 칸·항목이 정확히 같을 때만 바꾼다.
// 도중 실패면 쓴 파일을 원본으로 되돌린다(원격 쓰기 실패도). 설정 파일은 운영자의 것 — 고치기만 하고 커밋은 하지 않는다

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/gitx"
	"herdr-tools/internal/knob"
	"herdr-tools/internal/lockx"
	"herdr-tools/internal/verified"
)

// 칸 바꾸는 꼴: 같음 · 쉼표 목록 항목 · 주소(<기계>/… · pending:<기계>) · 남긴것(<기계>=<주소>,…) · 되돌릴곳(<기계>:<경로>#…) ·
// 주소이되 다시 맞춤 장부의 보냄·다시 냄 줄만(그 밖 줄의 그 칸은 레인 이름·결과 글)
const (
	rnEq = iota
	rnList
	rnAddr
	rnStuck
	rnReply
	rnRelandAddr
)

type rnFix struct{ col, how int }

// rnFile: DATA 아래 상태 파일(* 꼴이면 그 폴더의 맞는 파일 전부). min칸 미만 줄은 옛 꼴이라 칸 뜻이 달라 안 본다
type rnFile struct {
	name string
	min  int
	fix  []rnFix
}

var rnFiles = []rnFile{
	{"queue.tsv", 0, []rnFix{{qOnly, rnList}, {qMach, rnEq}, {qAddr, rnAddr}, {qStuck, rnStuck}, {qReply, rnReply}, {qOwner, rnAddr}}},
	{"inbox.tsv", 0, []rnFix{{ldReply, rnReply}}},
	{"audit.tsv", atNF, []rnFix{{atMach, rnEq}, {atAddr, rnAddr}}},
	{"try.tsv", 0, []rnFix{{trMach, rnEq}, {trAddr, rnAddr}, {trGot, rnAddr}}},
	{"reject.tsv", 0, []rnFix{{rjMach, rnEq}}},
	{"own.tsv", 0, []rnFix{{owMach, rnEq}, {owAddr, rnAddr}}},
	{"overlap.tsv", 0, []rnFix{{wtOnly, rnList}}},
	{"merger.tsv", 0, []rnFix{{mgMach, rnEq}}},
	{"merger-alive.tsv", 0, []rnFix{{maMach, rnEq}, {maAddr, rnAddr}}},
	{"load.tsv", 0, []rnFix{{loMach, rnEq}}},
	{"reach.tsv", 0, []rnFix{{rcMach, rnEq}}},
	{"idle.tsv", 0, []rnFix{{idMach, rnEq}}},
	{"held.tsv", 0, []rnFix{{hdAddr, rnAddr}}},
	{"seen.tsv", 0, []rnFix{{snMach, rnEq}, {snAddr, rnAddr}}},
	{"nudge.tsv", 0, []rnFix{{nuAddr, rnAddr}}},
	{"onland.tsv", 0, []rnFix{{olOwner, rnAddr}}},
	{"onland/*.pending", 0, []rnFix{{opOwner, rnAddr}}},
	{"reland.tsv", 0, []rnFix{{rlOwner, rnAddr}, {rlArg, rnRelandAddr}}},
}

// rnConfCols: 설정 줄 종류 → 기계 이름 칸(0부터, 첫 칸이 종류). coord는 주소, merger_verifiers는 쉼표 목록
var rnConfCols = map[string]rnFix{
	"local": {1, rnEq}, "machine": {1, rnEq}, "machine-has": {1, rnEq}, "inbox": {1, rnEq},
	"repo": {2, rnEq}, "merger": {2, rnEq}, "merger_verifiers": {2, rnList}, "coord": {1, rnAddr},
}

// rnVal: 칸 값 하나를 옛 → 새(정확히 같은 기계 이름일 때만 — 앞부분만 같은 이름은 그대로)
func rnVal(v string, how int, o, n string) string {
	switch how {
	case rnEq:
		if v == o {
			return n
		}
	case rnList:
		xs := strings.Split(v, ",")
		for i, x := range xs {
			if x == o {
				xs[i] = n
			}
		}
		return strings.Join(xs, ",")
	case rnAddr, rnRelandAddr:
		if r, ok := strings.CutPrefix(v, "pending:"); ok {
			if r == o {
				return "pending:" + n
			}
			return "pending:" + rnVal(r, rnAddr, o, n)
		}
		if r, ok := strings.CutPrefix(v, o+"/"); ok {
			return n + "/" + r
		}
	case rnStuck:
		xs := strings.Split(v, ",")
		for i, x := range xs {
			if m, a, ok := strings.Cut(x, "="); ok {
				xs[i] = rnVal(m, rnEq, o, n) + "=" + rnVal(a, rnAddr, o, n)
			}
		}
		return strings.Join(xs, ",")
	case rnReply:
		if r, ok := strings.CutPrefix(v, o+":"); ok {
			return n + ":" + r
		}
	}
	return v
}

// rnRow: 탭 줄 하나(바뀌었으면 참). 고칠 칸 말고는 바이트 그대로
func rnRow(l string, f rnFile, o, n string) (string, bool) {
	if l == "" || strings.HasPrefix(l, "#") {
		return l, false
	}
	c := strings.Split(l, "\t")
	if len(c) < f.min {
		return l, false
	}
	ch := false
	for _, x := range f.fix {
		if x.col > len(c) {
			continue
		}
		if x.how == rnRelandAddr && (rlWhat > len(c) || (c[rlWhat-1] != "보냄" && c[rlWhat-1] != "다시 냄")) {
			continue
		}
		if v := rnVal(c[x.col-1], x.how, o, n); v != c[x.col-1] {
			c[x.col-1], ch = v, true
		}
	}
	return strings.Join(c, "\t"), ch
}

// rnConfLine: 설정 줄 하나 — 칸은 공백·탭으로 나누므로 그 칸의 바이트 자리만 바꾼다(주석·다른 칸·사이 공백 그대로)
func rnConfLine(l, o, n string) (string, bool) {
	type span struct{ a, b int }
	var sp []span
	for i := 0; i < len(l); {
		for i < len(l) && (l[i] == ' ' || l[i] == '\t') {
			i++
		}
		j := i
		for j < len(l) && l[j] != ' ' && l[j] != '\t' {
			j++
		}
		if j > i {
			sp = append(sp, span{i, j})
		}
		i = j
	}
	if len(sp) == 0 || strings.HasPrefix(l[sp[0].a:], "#") {
		return l, false
	}
	x, ok := rnConfCols[l[sp[0].a:sp[0].b]]
	if !ok || x.col >= len(sp) {
		return l, false
	}
	s := sp[x.col]
	v := rnVal(l[s.a:s.b], x.how, o, n)
	if v == l[s.a:s.b] {
		return l, false
	}
	return l[:s.a] + v + l[s.b:], true
}

// rnKnobLine: 손잡이 저장 파일 줄 — 기계 묶음 손잡이(seats@<기계>·pause@<기계>)의 이름 칸만
func rnKnobLine(l, o, n string) (string, bool) {
	if l == "" || strings.HasPrefix(l, "#") {
		return l, false
	}
	i := strings.IndexAny(l, " \t") // 저장 파일은 탭, 설정 파일(knob.ConfName)은 공백·탭
	if i < 0 {
		return l, false
	}
	k, rest := l[:i], l[i:]
	d, ok := knob.Find(k)
	if !ok || d.Per != "기계" || d.Of() != o {
		return l, false
	}
	return strings.TrimSuffix(k, o) + n + rest, true
}

// rnEdit: 파일 하나의 바꿈(원본 사본을 들고 있어 되돌린다)
type rnEdit struct {
	Path   string `json:"path"`
	Lines  int    `json:"lines"`
	Sample string `json:"sample"`
	before string
	after  string
	mode   os.FileMode
	fresh  bool // 없던 파일 — 되돌리기는 지운다
}

func rnEditOf(path string, fn func(string) (string, bool)) (*rnEdit, error) {
	st, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("읽기 %s: %w", path, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("읽기 %s: %w", path, err)
	}
	ls := strings.Split(string(b), "\n")
	e := &rnEdit{Path: path, before: string(b), mode: st.Mode().Perm()}
	for i, l := range ls {
		if v, ch := fn(l); ch {
			if e.Lines == 0 {
				e.Sample = flat1(l) + " → " + flat1(v)
			}
			ls[i] = v
			e.Lines++
		}
	}
	if e.Lines == 0 {
		return nil, nil
	}
	e.after = strings.Join(ls, "\n")
	return e, nil
}

// rnPlan: 바꿀 파일들(설정 · 상태 파일 · 손잡이). 못 읽은 파일이 있으면 오류 — 모르는 채 반만 바꾸지 않는다
func rnPlan(o, n string) ([]*rnEdit, error) {
	var es []*rnEdit
	add := func(path string, fn func(string) (string, bool)) error {
		e, err := rnEditOf(path, fn)
		if e != nil {
			es = append(es, e)
		}
		return err
	}
	if err := add(CONF, func(l string) (string, bool) { return rnConfLine(l, o, n) }); err != nil {
		return nil, err
	}
	for _, f := range rnFiles {
		ps := []string{DATA + "/" + f.name}
		if strings.Contains(f.name, "*") {
			var err error
			if ps, err = filepath.Glob(DATA + "/" + f.name); err != nil {
				return nil, err
			}
		}
		for _, p := range ps {
			if err := add(p, func(l string) (string, bool) { return rnRow(l, f, o, n) }); err != nil {
				return nil, err
			}
		}
	}
	for _, p := range []string{knob.Path(HOME), knob.ConfPath()} {
		if err := add(p, func(l string) (string, bool) { return rnKnobLine(l, o, n) }); err != nil {
			return nil, err
		}
	}
	a, err := rnAliasEdit(o, n)
	if err != nil {
		return nil, err
	}
	return append(es, a), nil
}

// 기계 옛 이름(machine-alias.tsv): 병합기 사건의 기계 칸은 띄울 때 받은 HC_LANE_MACHINE이라, 바꾼 순간 병합 중이던(또는 바꾸기 직전에
// 적고 아직 바퀴가 안 읽은) 결과 사건은 옛 이름으로 남는다. 그 사건을 새 이름 병합기의 것으로 쳐야 받은편지 회신·반송 다시 맞춤·쓰기 소유가
// 풀린다(감사 atozsn0-m2qozu 중간)
const rnAliasKeep = 24 * 3600 // ponytail: 하루 — 병합기 한 바퀴·회신 미룸보다 넉넉히. 더 오래 남는 사건이 생기면 늘린다

func rnAliasFile() string { return DATA + "/machine-alias.tsv" }

// rnAliasEdit: 「옛 새 지금」 한 줄을 더한다 — 새 이름이 옛 이름이던 줄(되돌리기 전 것)과 기한 지난 줄은 뺀다
func rnAliasEdit(o, n string) (*rnEdit, error) {
	p := rnAliasFile()
	e := &rnEdit{Path: p, Lines: 1, mode: 0o644}
	b, err := os.ReadFile(p)
	switch {
	case errors.Is(err, os.ErrNotExist):
		e.fresh = true
	case err != nil:
		return nil, fmt.Errorf("읽기 %s: %w", p, err)
	}
	e.before = string(b)
	now := nowUnix()
	var keep []Row
	for _, r := range readRows(p) {
		if r.NF() > 0 && r.F(aaOld) != n && now-int64(num(r.F(aaAt))) < rnAliasKeep {
			keep = append(keep, r)
		}
	}
	add := Row{o, n, fmt.Sprint(now)}
	e.Sample = "+ " + flat1(add.Line())
	e.after = rowsText(append(keep, add))
	return e, nil
}

// mgSet: 병합기 사건으로 칠 기계 — 켜진 병합기 기계(MGM)와, 하루 안에 그 이름으로 바뀐 옛 이름(이어 바꾼 것도 거슬러)
func mgSet() map[string]bool {
	mg := map[string]bool{}
	for _, m := range fields(MGM) {
		mg[m] = true
	}
	rs, now := readRows(rnAliasFile()), nowUnix()
	for i := len(rs) - 1; i >= 0; i-- { // 새것부터 — a→b 뒤 b→c면 c에서 b, b에서 a
		if r := rs[i]; mg[r.F(aaNew)] && now-int64(num(r.F(aaAt))) < rnAliasKeep {
			mg[r.F(aaOld)] = true
		}
	}
	return mg
}

// rnStale: 상태 파일에 이미 새 이름이 기계 칸에 든 줄 수(설정에서 지운 기계의 묵은 줄 — 바꾸면 그 줄과 합쳐진다)
func rnStale(n string) int {
	k := 0
	for _, f := range rnFiles {
		ps, _ := filepath.Glob(DATA + "/" + f.name)
		for _, p := range ps {
			if e, err := rnEditOf(p, func(l string) (string, bool) { return rnRow(l, f, n, "\x00") }); err == nil && e != nil {
				k += e.Lines
			}
		}
	}
	return k
}

// rnWriteHook: 시험 자리(평소 nil) — 그 경로 쓰기를 실패시킨다
var rnWriteHook func(path string) error

// rnApply: 차례로 쓴다. 하나라도 실패하면 앞서 쓴 것을 원본으로 되돌리고 오류
func rnApply(es []*rnEdit) error {
	for i, e := range es {
		err := error(nil)
		if rnWriteHook != nil {
			err = rnWriteHook(e.Path)
		}
		if err == nil {
			err = atomic.WriteFile(e.Path, []byte(e.after), e.mode)
		}
		if err != nil {
			return errors.Join(fmt.Errorf("쓰기 %s: %w", e.Path, err), rnUndo(es[:i]))
		}
	}
	return nil
}

// rnUndo: 원본으로 되돌린다. 못 되돌린 파일은 오류에 모두(손으로 고칠 곳)
func rnUndo(es []*rnEdit) error {
	var errs []error
	for _, e := range es {
		if e.fresh {
			if err := os.Remove(e.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, fmt.Errorf("되돌리지 못함(지울 새 파일) %s: %w", e.Path, err))
			}
			continue
		}
		if err := atomic.WriteFile(e.Path, []byte(e.before), e.mode); err != nil {
			errs = append(errs, fmt.Errorf("되돌리지 못함 %s(원본 %d바이트): %w", e.Path, len(e.before), err))
		}
	}
	return errors.Join(errs...)
}

// rnRef: 옮길 병합기 켬 표식 하나
type rnRef struct {
	Proj string `json:"project"`
	SHA  string `json:"sha"`
	repo string
}

const rnMergerRef = "refs/herdr/merger/"

// rnRefs: 옛 기계에 켠 병합기(설정 merger 줄·관리 기록 merger.tsv)의 표식. 원격을 못 읽거나 새 표식이 이미 있으면 오류
func rnRefs(o, n string) ([]rnRef, error) {
	var ps []string
	for _, f := range confRows("merger") {
		if F1(f, 2) == o {
			ps = append(ps, f[1])
		}
	}
	for _, r := range readRows(DATA + "/merger.tsv") {
		if r.F(mgMach) == o {
			ps = append(ps, r.F(mgProj))
		}
	}
	var out []rnRef
	for _, p := range uniq(ps) {
		r := repoOf(p, LOCAL)
		if r == "" {
			return nil, fmt.Errorf("병합기 %s: 설정에 %s repo 줄이 없어 켬 표식을 옮길 수 없다", p, LOCAL)
		}
		refs, st := gitx.LsRemote(r, "origin", nil, rnMergerRef+o, rnMergerRef+n)
		if st == gitx.Unknown {
			return nil, fmt.Errorf("병합기 %s: 원격 표식을 못 읽음(%s) — 모르는 채 옮기지 않는다", p, r)
		}
		if _, ok := refs[rnMergerRef+n]; ok {
			return nil, fmt.Errorf("병합기 %s: 새 이름 표식 %s%s 이 이미 있다", p, rnMergerRef, n)
		}
		if sha, ok := refs[rnMergerRef+o]; ok {
			out = append(out, rnRef{p, sha, r})
		}
	}
	return out, nil
}

// rnPush: 표식 하나를 같은 커밋으로 옮긴다 — 새 참조는 없을 때만 만들고 옛 참조는 본 값일 때만 지운다(한 번에)
func rnPush(x rnRef, from, to string) bool {
	_, rc := run("git", "-C", x.repo, "push", "-q", "--atomic",
		"--force-with-lease="+rnMergerRef+to+":", "--force-with-lease="+rnMergerRef+from+":"+x.SHA,
		"origin", x.SHA+":"+rnMergerRef+to, ":"+rnMergerRef+from)
	return rc == 0
}

// rnLive: 옛 기계에 떠 있는 대기열 줄(레인·감사) 이름
func rnLive(o string) []string {
	var o2 []string
	for _, r := range readRows(Q) {
		if r.F(qMach) == o && (r.F(qState) == "up" || r.F(qState) == "sent") {
			o2 = append(o2, r.F(qName))
		}
	}
	return o2
}

// rnOut: 출력(--json 꼴과 같은 사실)
type rnOut struct {
	Old    string    `json:"old"`
	New    string    `json:"new"`
	DryRun bool      `json:"dry_run"`
	Files  []*rnEdit `json:"files"`
	Refs   []rnRef   `json:"merger_refs"`
	Live   []string  `json:"live"`
	Stale  int       `json:"stale"`
	Next   []string  `json:"next"`
}

func rnNext(x *rnOut) {
	for _, r := range x.Refs {
		x.Next = append(x.Next, "hc-fill merger restart "+r.Proj+" — 옛 이름 병합기(HC_LANE_MACHINE="+x.Old+")는 표식이 옮겨져 스스로 멈춘다")
	}
	for _, e := range x.Files {
		if e.Path == CONF {
			x.Next = append(x.Next, fmt.Sprintf("설정 파일을 고쳤다: %s %d줄 — 커밋은 직접", CONF, e.Lines))
		}
	}
	if x.Stale > 0 {
		x.Next = append(x.Next, fmt.Sprintf("상태 파일에 이미 %s 이 기계 칸에 든 줄이 %d줄 있다(설정에서 지운 기계의 묵은 줄) — 바꾼 줄과 합쳐진다. 원치 않으면 다른 이름으로", x.New, x.Stale))
	}
	x.Next = append(x.Next, "hc 기계 표·SSH 별칭·다른 기계의 설정(herdr-sync 기계 표 등)에 "+x.Old+"이 있으면 "+x.New+"으로(이 명령은 이 기계의 대기열 설정·상태만 바꾼다)")
	if len(x.Live) > 0 {
		x.Next = append(x.Next, "떠 있는 줄 "+strings.Join(x.Live, ",")+"은 기계 칸이 함께 바뀌어 다음 바퀴가 이어 본다 — hc가 그 기계를 "+x.New+"으로 보여야 한다(hc machine ls)")
	}
}

func rnPrint(x *rnOut, js bool) {
	if js {
		printJSON(x)
		return
	}
	head := "바꿈"
	if x.DryRun {
		head = "미리 보기(--dry-run — 바꾸지 않음)"
	}
	say("기계 이름 %s → %s %s", x.Old, x.New, head)
	for _, e := range x.Files {
		say("  %s  %d줄  예: %s", e.Path, e.Lines, e.Sample)
	}
	for _, r := range x.Refs {
		say("  병합기 %s  %s%s → %s%s (%s)", r.Proj, rnMergerRef, x.Old, rnMergerRef, x.New, r.SHA[:min(8, len(r.SHA))])
	}
	if len(x.Live) > 0 {
		say("  떠 있는 줄: %s", strings.Join(x.Live, ","))
	}
	for _, s := range x.Next {
		say("다음: %s", s)
	}
}

// machineRename: machine --rename <옛> <새> [--dry-run] [--json] [--why <까닭>]
func machineRename(args []string) {
	args, dryR := takeDry(args)
	args, why := takeWhy(args)
	js := false
	var names []string
	for _, a := range args {
		switch {
		case a == "--json":
			js = true
		case strings.HasPrefix(a, "-"):
			usage()
		default:
			names = append(names, a)
		}
	}
	if len(names) != 2 {
		usage()
	}
	o, n := names[0], names[1]
	x := &rnOut{Old: o, New: n, DryRun: dryR, Files: []*rnEdit{}, Refs: []rnRef{}}
	body := func() {
		if t, ok := readFile(CONF); ok { // 잠금 안에서 다시 읽는다(main이 읽은 뒤 바뀌었을 수 있다)
			confLines = lines(t)
			LOCAL = confRaw("local")
		}
		if why := rnRefuse(o, n); why != "" {
			refuse("%s", why)
		}
		if kp := knob.Path(HOME); !dryR && exists(kp) { // 손잡이 set·unset과 겹치지 않게(저장 파일이 없으면 바꿀 것도 없다)
			lk, err := lockx.Lock(kp + ".lock")
			if err != nil {
				die("손잡이 잠금: %v — 아무것도 안 바꿈", err)
			}
			defer lk.Close()
		}
		es, err := rnPlan(o, n)
		if err != nil {
			die("%v — 아무것도 안 바꿈", err)
		}
		refs, err := rnRefs(o, n)
		if err != nil {
			die("%v — 아무것도 안 바꿈", err)
		}
		x.Files, x.Refs, x.Live, x.Stale = es, refs, rnLive(o), rnStale(n)
		rnNext(x)
		if dryR {
			return
		}
		if err := rnApply(es); err != nil {
			die("%v — 쓴 파일은 원본으로 되돌렸다", err)
		}
		for i, r := range refs {
			if rnPush(r, o, n) {
				continue
			}
			var errs []error
			for _, d := range refs[:i] {
				if !rnPush(d, n, o) {
					errs = append(errs, fmt.Errorf("병합기 %s 표식을 되돌리지 못함 — 손으로: git -C %s push origin %s:%s%s :%s%s", d.Proj, d.repo, d.SHA, rnMergerRef, o, rnMergerRef, n))
				}
			}
			die("병합기 %s 켬 표식 push 실패 — 파일을 원본으로 되돌렸다 %v", r.Proj, errors.Join(append(errs, rnUndo(es))...))
		}
		jot("machine rename", o+" → "+n, why, fmt.Sprintf("파일 %d개 · 표식 %d개", len(es), len(refs)), "hc-fill machine --rename "+n+" "+o, "")
	}
	if dryR {
		body()
	} else if rc := withRun(body); rc != 0 {
		exit(rc)
	}
	rnPrint(x, js)
	exit(0)
}

// rnRefuse: 거절 까닭(없으면 빈 글) — 이름 꼴·옛 이름이 설정 machine 줄에 없음·새 이름이 설정 어느 기계 칸에 이미 있음
func rnRefuse(o, n string) string {
	switch {
	case !verified.MachineOK(o) || !verified.MachineOK(n):
		return fmt.Sprintf("기계 이름 꼴: a-z 0-9 _ - 32자 안(- 로 시작 못 함) — %q %q", o, n)
	case o == n:
		return "옛 이름과 새 이름이 같다: " + o
	case !machineKnown(o):
		return "설정 machine 줄에 없는 기계: " + o
	}
	for _, l := range confLines {
		if _, ch := rnConfLine(l, n, "\x00"); ch {
			return fmt.Sprintf("새 이름 %s 이 설정에 이미 있다: %s", n, strings.TrimSpace(l))
		}
	}
	return ""
}
