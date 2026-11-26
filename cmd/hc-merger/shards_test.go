//go:build !windows

package main

// 맨 앞 추측 나눠 돌리기(ht-merger-shards, specshards.go): 손잡이 merger_shards(HC_LANE_MERGER_SHARDS)가 K>1이고 맡길 기계가 K대면 맨 앞
// 추측 하나를 같은 아이디로 K대에 HERDR_VERIFY_SHARD=i/K 일꾼으로 띄운다. 몫은 조각 줄만 따로 올리고 통과 기록은 쓰지 않는다. 모든 몫이
// 끝나면(또는 맨 앞 기다림 상한) 한 기계에서 지금 추측 검증을 그대로 한 번(마무리) — 몫들의 조각을 이어 받아 빈틈만 돌고 통과 기록을 쓴다.
// 틀은 specFx + 조각 셋(p1·p2·p3)을 아는 검증 스크립트: 나눔 i/K면 n번째 조각은 (n−1)%K+1 = i인 몫만 돌고($PARTDIR/gap이면 p3은 어느 몫도
// 안 돈다 — 나눔이 어긋난 스크립트), $PARTDIR/envfail-<기계>면 시험 줄 없이 실패(환경 실패 — envfail-<조각>-<기계>면 그 조각에서),
// $PARTDIR/fail-<조각>-<기계>면 시험 실패.
// 돈 것은 $HOOKLOG에 「run <조각>@<기계>」

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/verified"
)

const shardVerify = `#!/bin/bash
t=$(git rev-parse 'HEAD^{tree}')
done=" "
if [ -n "$HERDR_VERIFY_PARTS" ] && [ -f "$HERDR_VERIFY_PARTS" ]; then
  read -r pt rest <"$HERDR_VERIFY_PARTS"
  [ "$pt" = "$t" ] && done=" $rest "
fi
i=; k=
if [ -n "${HERDR_VERIFY_SHARD:-}" ]; then i=${HERDR_VERIFY_SHARD%/*}; k=${HERDR_VERIFY_SHARD#*/}; fi
passed=
n=0
for p in p1 p2 p3; do
  n=$((n+1))
  case "$done" in *" $p "*) passed="$passed $p"; continue;; esac
  if [ -n "$k" ]; then
    [ $(( (n-1) % k + 1 )) = "$i" ] || continue
    if [ -e "$PARTDIR/gap" ] && [ "$p" = p3 ]; then continue; fi
  fi
  echo "run $p@$HC_LANE_MACHINE" >>"$HOOKLOG"
  [ ! -e "$PARTDIR/envfail-$HC_LANE_MACHINE" ] || exit 1
  [ ! -e "$PARTDIR/envfail-$p-$HC_LANE_MACHINE" ] || exit 1
  if [ -e "$PARTDIR/fail-$p-$HC_LANE_MACHINE" ]; then printf -- '--- FAIL: TestX (0.01s)\nFAIL\texample/%s\t0.1s\n' "$p"; exit 1; fi
  passed="$passed $p"
  [ -z "$HERDR_VERIFY_PARTS" ] || echo "$t$passed" >"$HERDR_VERIFY_PARTS"
done
exit 0
`

// shardFx: 검증 기계 셋(v1·v2·v3), 조각을 아는 검증 스크립트, 손잡이 merger_shards = k
func shardFx(t *testing.T, k string) (*sfx, string, string) {
	s, hl, pd := partsFx(t, false)
	os.WriteFile(s.seed+"/.claude/herdr-verify.sh", []byte(shardVerify), 0o777)
	s.git(s.seed, "add", "-A")
	s.git(s.seed, "commit", "-qm", "shard verify")
	s.git(s.seed, "push", "-q", "origin", "main")
	s.git(s.dir, "clone", "-q", s.origin, s.dir+"/v3")
	var vs []verified.Verifier
	for _, v := range []string{"v1", "v2", "v3"} {
		vs = append(vs, verified.Verifier{Machine: v, Path: s.dir + "/" + v})
	}
	et := s.git(s.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	vc := s.git(s.seed, "commit-tree", et, "-m", verified.FormatVerifiers(vs))
	s.git(s.seed, "push", "-q", "-f", "origin", vc+":"+verified.VerifiersRef)
	t.Setenv("HC_LANE_MERGER_SHARDS", k)
	return s, hl, pd
}

var shardRe = regexp.MustCompile(`--on (\S+) .*--spec-verify ([0-9a-f]{40}) main(?: (\d+/\d+))?\s*$`)

// launches: 띄운 차례대로 「기계 아이디 몫」(몫 없으면 "")
func (s *sfx) launches() [][3]string {
	b, _ := os.ReadFile(s.hclog)
	var o [][3]string
	for _, l := range strings.Split(string(b), "\n") {
		if m := shardRe.FindStringSubmatch(l); m != nil {
			o = append(o, [3]string{m[1], m[2], m[3]})
		}
	}
	return o
}

// runOn: 그 기계 클론에서 일꾼(몫이 있으면 나눔 일꾼)
func (s *sfx) runOn(d [3]string) int {
	s.t.Helper()
	s.t.Setenv("HC_LANE_MACHINE", d[0])
	defer s.t.Setenv("HC_LANE_MACHINE", "box")
	s.t.Chdir(s.dir + "/" + d[0])
	defer s.t.Chdir(s.clone)
	a := []string{d[1], "main"}
	if d[2] != "" {
		a = append(a, d[2])
	}
	return specVerifyMain(a)
}

// threeShards: 맨 앞 a를 셋으로 나눠 띄운 것(기계 셋·몫 1/3·2/3·3/3·같은 아이디)
func threeShards(t *testing.T, s *sfx) [][3]string {
	t.Helper()
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.launches()
	if len(d) != 3 {
		t.Fatalf("나눔 일꾼 셋: %v", d)
	}
	seen := map[string]bool{}
	for i, x := range d {
		if x[2] != []string{"1/3", "2/3", "3/3"}[i] || x[1] != d[0][1] || seen[x[0]] {
			t.Fatalf("몫 i/3 · 같은 아이디 · 서로 다른 기계: %v", d)
		}
		seen[x[0]] = true
	}
	return d
}

// ① 손잡이 1(기본) → 지금과 같은 띄움(몫 없음)
func TestShardsKnobOneAsBefore(t *testing.T) {
	s, _, _ := shardFx(t, "")
	s.sub(1000000001, "a", false)
	s.round(1)
	if d := s.launches(); len(d) != 1 || d[0][2] != "" {
		t.Fatalf("① 하나·몫 없음: %v", d)
	}
}

// ② 손잡이 3·기계 3 → 일꾼 셋, 몫만 통과해서는 통과 기록 없음 → 마무리 한 번(빈틈 없음 — 다 건너뜀) → 통과 기록·착지. 일지 한 줄에 나눔·마무리
func TestShardsThreeThenFinish(t *testing.T) {
	s, hl, _ := shardFx(t, "3")
	d := threeShards(t, s)
	for _, x := range d {
		if rc := s.runOn(x); rc != 0 {
			t.Fatalf("몫 %v 종료 %d", x, rc)
		}
	}
	if got := ran(t, hl); got != "run p1@"+d[0][0]+"\nrun p2@"+d[1][0]+"\nrun p3@"+d[2][0] {
		t.Fatalf("② 몫마다 자기 조각만: %q", got)
	}
	if r := s.refs(verified.Prefix); len(r) != 0 {
		t.Fatalf("② 몫만 통과해서는 통과 기록이 없다: %v", r)
	}
	s.round(1)
	l := s.launches()
	if len(l) != 4 || l[3][2] != "" || l[3][1] != d[0][1] || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("② 마무리 한 번(같은 아이디·몫 없음): %v 착지 %v", l, s.lines(s.landlog))
	}
	if rc := s.runOn(l[3]); rc != 0 {
		t.Fatalf("마무리 종료 %d", rc)
	}
	if got := ran(t, hl); got != "" {
		t.Fatalf("② 마무리는 몫들이 통과한 조각을 건너뛴다: %q", got)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" {
		t.Fatalf("② 마무리 통과 기록으로 착지: %q\n%s", got, jdump())
	}
	w := jrows(t, "추측 기다림")
	if len(w) != 1 || !strings.Contains(w[0][11], "나눔 3("+d[0][0]+","+d[1][0]+","+d[2][0]+")") || !strings.Contains(w[0][11], "마무리 "+l[3][0]+" ") {
		t.Fatalf("② 일지 한 줄에 「나눔 3(…) · 마무리 <기계> n초」:\n%s", jdump())
	}
	if got := whySummary(strings.Split(jdump(), "\n"), 20); !strings.Contains(got, "검증 맡김 1번") || !strings.Contains(got, "나눔 1") {
		t.Fatalf("② --why: %s", got)
	}
	s.round(1) // 큐가 빈 바퀴가 몫의 조각·끝 참조도 치운다
	if r := append(s.refs(specParts), s.refs(specShards)...); len(r) != 0 {
		t.Fatalf("② 착지 뒤 몫 참조를 치운다: %v", r)
	}
}

// ③ 맡길 기계 1 → 나눔 없이 지금 길
func TestShardsOneMachineNoSplit(t *testing.T) {
	s, _, _ := shardFx(t, "3")
	s.idle("v2", verifyBusy)
	s.idle("v3", verifyBusy)
	s.sub(1000000001, "a", false)
	s.round(1)
	if d := s.launches(); len(d) != 1 || d[0][0] != "v1" || d[0][2] != "" {
		t.Fatalf("③ v1 하나·몫 없음: %v", d)
	}
}

// ④ 몫 하나 환경 실패 → 실패 기록 없이 「그 몫은 못 돎」, 마무리가 그 몫의 조각을 돈다
func TestShardEnvFailFinishRuns(t *testing.T) {
	s, hl, pd := shardFx(t, "3")
	d := threeShards(t, s)
	os.WriteFile(pd+"/envfail-"+d[1][0], nil, 0o666)
	for _, x := range d {
		s.runOn(x)
	}
	ran(t, hl)
	if s.has(specFails + d[0][1]) {
		t.Fatal("④ 몫의 환경 실패는 추측 실패가 아니다")
	}
	os.Remove(pd + "/envfail-" + d[1][0])
	s.round(1)
	l := s.launches()
	if len(l) != 4 || l[3][2] != "" {
		t.Fatalf("④ 마무리: %v", l)
	}
	s.runOn(l[3])
	if got := ran(t, hl); got != "run p2@"+l[3][0] {
		t.Fatalf("④ 마무리는 못 돈 몫의 p2만: %q", got)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" {
		t.Fatalf("④ 착지: %q", got)
	}
}

// ⑤ 몫 하나 시험 실패 → 추측 실패와 같은 길(다른 기계에서 한 번 더 — 다른 몫이 통과한 조각은 건너뜀) → 그래도 실패면 반송
func TestShardTestFailRecheckThenReject(t *testing.T) {
	s, hl, pd := shardFx(t, "3")
	d := threeShards(t, s)
	for _, v := range []string{"v1", "v2", "v3"} {
		os.WriteFile(pd+"/fail-p2-"+v, nil, 0o666)
	}
	for _, x := range d {
		s.runOn(x)
	}
	ran(t, hl)
	if !s.has(specFails + d[0][1]) {
		t.Fatal("⑤ 몫의 시험 실패는 추측 실패 기록")
	}
	s.round(1)
	l := s.launches()
	if len(l) != 4 || l[3][2] != "" || l[3][0] == d[1][0] {
		t.Fatalf("⑤ 다른 기계에서 한 번 더(몫 없음): %v", l)
	}
	s.runOn(l[3])
	if got := ran(t, hl); got != "run p2@"+l[3][0] {
		t.Fatalf("⑤ 다른 몫의 통과 조각은 버리지 않는다(p2만): %q", got)
	}
	s.round(1)
	if !s.has("refs/merge-failed/1000000001-a") || s.onMain("a.txt") {
		t.Fatalf("⑤ 두 기계 시험 실패 → 반송:\n%s", jdump())
	}
}

// ⑥ 나눔이 어긋나 빈틈(p3을 어느 몫도 안 돎) → 몫만으로는 통과 기록이 없고, 마무리가 빈틈을 돌아 통과
func TestShardGapFinishCovers(t *testing.T) {
	s, hl, pd := shardFx(t, "3")
	os.WriteFile(pd+"/gap", nil, 0o666)
	d := threeShards(t, s)
	for _, x := range d {
		if rc := s.runOn(x); rc != 0 {
			t.Fatalf("몫 종료 %d", rc)
		}
	}
	ran(t, hl)
	if r := s.refs(verified.Prefix); len(r) != 0 {
		t.Fatalf("⑥ 빈틈을 건너뛴 채 통과 기록이 생겼다: %v", r)
	}
	os.Remove(pd + "/gap")
	s.round(1)
	l := s.launches()
	if len(l) != 4 {
		t.Fatalf("⑥ 마무리: %v", l)
	}
	s.runOn(l[3])
	if got := ran(t, hl); got != "run p3@"+l[3][0] {
		t.Fatalf("⑥ 마무리가 빈틈 p3을 돈다: %q", got)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" {
		t.Fatalf("⑥ 착지: %q", got)
	}
}

// 몫 하나가 멈춤 → 맨 앞 기다림 상한에서 처음부터 다시 돌지 않고 마무리에 넘긴다(끝난 몫의 조각은 건너뜀)
func TestShardStuckHandsToFinish(t *testing.T) {
	s, hl, _ := shardFx(t, "3")
	d := threeShards(t, s)
	s.runOn(d[0])
	s.runOn(d[1])
	ran(t, hl)
	s.age(d[0][1], 700)
	s.round(1)
	l := s.launches()
	if len(l) != 4 || l[3][2] != "" || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("상한 → 마무리(직접 검증 아님): %v 착지 %v", l, s.lines(s.landlog))
	}
	s.runOn(l[3])
	if got := ran(t, hl); got != "run p3@"+l[3][0] {
		t.Fatalf("마무리는 멈춘 몫의 p3만: %q", got)
	}
}

// 감사 atp3qsv 중간 1: 상한을 넘겼어도 남은 몫이 그 기계에서 진행 중(터미널 spec-<아이디 8>-<i> 살아 있음·작업 트리 갱신)이면 마무리도
// 직접 검증도 하지 않고 더 기다린다(진행 판정은 몫 전체로). 그 몫이 멈추면 마무리에 넘긴다
func TestShardOverrunWaitsWhileShardRuns(t *testing.T) {
	s, _, _ := shardFx(t, "3")
	d := threeShards(t, s)
	s.runOn(d[0])
	s.runOn(d[1])
	id8, mach := d[0][1][:8], d[2][0]
	js := fmt.Sprintf(`{"address":"%s/t9","state":"live","cwd":"%s","name":"spec-%s-3","status":"working","pid":1,"created_ms":1}`, mach, s.dir+"/"+mach, id8)
	os.WriteFile(s.dir+"/idle/get-spec-"+id8+"-3", []byte(js+"\n"), 0o666)
	lg := s.dir + "/" + mach + ".spec-wt/" + id8 + "-s3.log"
	os.MkdirAll(filepath.Dir(lg), 0o777)
	os.WriteFile(lg, []byte("go test ...\n"), 0o666)
	s.age(d[0][1], 700)
	s.round(1)
	if l := s.launches(); len(l) != 3 || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("진행 중인 몫이 있으면 더 기다림(마무리·직접 검증 없음): %v 착지 %v\n%s", l, s.lines(s.landlog), jdump())
	}
	old := time.Now().Add(-20 * time.Minute)
	os.Chtimes(lg, old, old)
	s.round(1)
	if l := s.launches(); len(l) != 4 || l[3][2] != "" || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("그 몫이 멈추면 마무리: %v 착지 %v", l, s.lines(s.landlog))
	}
}

// ⑦ 도중에 기준이 바뀜(아이디 달라짐) → 도는 몫은 지금처럼 끝까지 두고, 몫이 다 끝나면 옛 추측(몫만으로는 통과가 아님)을 모름으로
// 풀어 그 spec·조각·끝 참조를 치운다 — 「도는 중」으로 남아 기계를 붙잡지 않게
func TestShardBaseMovesCleans(t *testing.T) {
	s, _, _ := shardFx(t, "3")
	d := threeShards(t, s)
	s.runOn(d[0])
	if !s.has(specShards+d[0][1]+"-s1of3") || !s.has(specParts+d[0][1]+"-s1") {
		t.Fatalf("⑦ 몫 1의 끝·조각 참조: %v %v", s.refs(specShards), s.refs(specParts))
	}
	s.moveMain("m2")
	s.round(1)
	s.runOn(d[1])
	s.runOn(d[2])
	s.round(1)
	for _, r := range append(append(s.refs(specParts), s.refs(specShards)...), s.refs(specRefs)...) {
		if strings.Contains(r, d[0][1]) {
			t.Fatalf("⑦ 옛 아이디의 참조가 남음: %s", r)
		}
	}
}
