package main

// coord-cwd-repo(2026-11-24): 지휘가 다루는 저장소(coordrepo.<주소>)에 지휘 터미널이 서 있는 폴더의 저장소도 넣는다 — 저장소 A 폴더에서
// B에만 레인을 넣은 지휘의 기록에 A가 없어, 받은편지로 들어온 A 레인(주인 없음) 사건이 「A 지휘 없음」으로 모든 지휘에게 갔다.
// 넓히기만 한다: 기록이 없는 지휘(모름 = 모든 저장소)는 그대로 두고, 저장소가 아닌 폴더는 넣지 않는다.

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"herdr-tools/internal/tri"
)

// cwdFake: get은 $D/get.<주소>(없으면 확인된 not_found)
const cwdFake = `#!/bin/sh
case $1 in
get) [ -f "$D/get.$2" ] && { cat "$D/get.$2"; exit 0; }; echo '{"error":{"kind":"not_found"}}' >&2; exit 3 ;;
esac
exit 0
`

type cwdKit struct {
	t *testing.T
	d string
}

func newCwdKit(t *testing.T) *cwdKit {
	d := t.TempDir()
	for _, p := range []string{d + "/bin", d + "/st/hc-lane", d + "/home", d + "/cfg"} {
		if err := os.MkdirAll(p, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(d+"/bin/hc", []byte(cwdFake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+"/bin:"+os.Getenv("PATH"))
	t.Setenv("XDG_STATE_HOME", d+"/st")
	t.Setenv("D", d)
	t.Setenv("HOME", d+"/home") // 손잡이 저장 파일(한 문이 alert_coord 등을 읽는다)
	t.Setenv("HERDR_TOOLS_CONFIG", d+"/cfg")
	up := watchUp
	watchUp = func(string, string) bool { return true } // 가짜 지휘는 watch가 돈다
	t.Cleanup(func() { watchUp = up })
	return &cwdKit{t, d}
}

// repo: origin이 u인 저장소 폴더(원격 없는 저장소는 u "")
func (k *cwdKit) repo(name, u string) string {
	p := k.d + "/" + name
	sh := "git init -q " + p
	if u != "" {
		sh += " && git -C " + p + " remote add origin " + u
	}
	if out, err := exec.Command("sh", "-c", sh).CombinedOutput(); err != nil {
		k.t.Fatalf("%s: %v %s", sh, err, out)
	}
	return p
}

// coord: 살아 있는 지휘(작업 폴더 cwd)의 hc get 응답과 지휘 정보, 이미 적힌 저장소 기록(옛 판 꼴)
func (k *cwdKit) coord(a, pid, cwd string, repos ...string) {
	k.write("get."+a, `{"address":"`+a+`","cwd":"`+cwd+`","foreground":{"name":"node","pid":7},"name":"`+a+`","pid":`+pid+`,"state":"live"}`)
	k.write("st/hc-lane/coord."+a, a+tab+pid+tab+a+tab+"0\n")
	var b strings.Builder
	for _, r := range repos {
		b.WriteString(pid + tab + r + "\n")
	}
	if len(repos) > 0 {
		k.write("st/hc-lane/coordrepo."+a, b.String())
	}
}

func (k *cwdKit) write(rel, s string) {
	if err := os.WriteFile(k.d+"/"+rel, []byte(s), 0o600); err != nil {
		k.t.Fatal(err)
	}
}

// repos: 그 지휘가 다루는 저장소(정렬) — 모름이면 "?"
func repoList(a string) string {
	rs, ok := coordRepos(a)
	if !ok {
		return "?"
	}
	var l []string
	for r := range rs {
		l = append(l, r)
	}
	if len(l) > 1 && l[0] > l[1] {
		l[0], l[1] = l[1], l[0]
	}
	return strings.Join(l, ",")
}

// sends: 주인 없는 저장소 r 레인 사건을 그 지휘의 watch가 보내나(한 문 — 받는 쪽은 한 명)
func sends(k *cwdKit, a, r string) bool {
	l := strings.Join([]string{utcNow(), "m", "t8", "inbox-x", "중간", "-", "요약", "-", "-", "-", "*", "-", r}, tab)
	e := &ev{ed: k.d + "/st/hc-lane", wto: a, cur: k.d + "/cur." + a, win: []string{l}, lrepo: laneRepos([]string{l})}
	return len(e.gate([]string{l})) == 1
}

const cwdA, cwdB = "example.com/team/a", "example.com/team/b"

// ① 지휘 X(작업 폴더 = 저장소 A, 넣은 레인은 B뿐)·지휘 Y(B만) → 주인 없는 A 레인 사건은 X만. ④ X가 끝나면 다시 Y도
func TestCoordCwdRepoRoutes(t *testing.T) {
	k := newCwdKit(t)
	a := k.repo("a", "git@Example.com:Team/A.git")
	k.coord("tx1", "1111", a, cwdB)
	k.coord("ty2", "2222", "/", cwdB)
	// X의 watch 바퀴(wcheck → coordCheck)가 먼저 돈다 — 필터 전에
	if _, why, rc := coordCheck("tx1"); rc != tri.Yes {
		t.Fatalf("coordCheck tx1 %v %s", rc, why)
	}
	if got := repoList("tx1"); got != cwdA+","+cwdB {
		t.Errorf("① X 기록 %q, 기대 A·B", got)
	}
	if !sends(k, "tx1", cwdA) || sends(k, "ty2", cwdA) {
		t.Errorf("① 주인 없는 A 사건: X 보냄 %v·Y 보냄 %v(기대 X만)", sends(k, "tx1", cwdA), sends(k, "ty2", cwdA))
	}
	if sends(k, "tx1", cwdB) == sends(k, "ty2", cwdB) {
		t.Error("① B 사건은 X·Y 가운데 한 명(한 문 — B를 다루던 X도 대표 후보)")
	}
	// ④ X가 끝남(hc get not_found) → A 사건은 Y가 받는다
	if err := os.Remove(k.d + "/get.tx1"); err != nil {
		t.Fatal(err)
	}
	if !sends(k, "ty2", cwdA) {
		t.Error("④ X가 끝나면 A 사건은 다시 Y도 받아야 한다")
	}
}

// ② 작업 폴더가 저장소가 아님·원격 없음 → 기록 그대로. 기록이 없는 지휘(모름 = 모든 저장소)는 작업 폴더가 저장소여도 좁히지 않는다
func TestCoordCwdRepoNoRepo(t *testing.T) {
	k := newCwdKit(t)
	n := k.repo("n", "")
	a := k.repo("a", "https://example.com/team/a")
	k.coord("ty2", "2222", "/", cwdB)
	k.coord("tn3", "3333", n, cwdB)
	k.coord("tz4", "4444", a)
	for _, c := range []string{"ty2", "tn3", "tz4"} {
		if _, why, rc := coordCheck(c); rc != tri.Yes {
			t.Fatalf("coordCheck %s %v %s", c, rc, why)
		}
	}
	if repoList("ty2") != cwdB || repoList("tn3") != cwdB {
		t.Errorf("② 저장소 아님·원격 없음은 그대로 B: %q %q", repoList("ty2"), repoList("tn3"))
	}
	if got := repoList("tz4"); got != "?" {
		t.Errorf("② 기록 없는 지휘는 모름 그대로(좁히지 않음): %q", got)
	}
	got := 0
	for _, c := range []string{"ty2", "tn3", "tz4"} {
		if sends(k, c, cwdB) {
			got++
		}
	}
	if got != 1 || sends(k, "tz4", cwdB) {
		t.Errorf("② B 사건은 B를 맡은 지휘 한 명(한 문 — 기록 없는 지휘는 전역 대표 후보일 뿐): %d", got)
	}
}

// ③ 이미 떠 있던 지휘(옛 판이 적은 기록 — 작업 폴더 저장소 없음)도 다음 coord 갱신(coordSet)·다음 바퀴(coordCheck)에 채워진다.
// 다른 pid(지난 지휘)의 줄은 지금 지휘 것이 아니다. 두 번 돌아도 한 줄
func TestCoordCwdRepoBackfill(t *testing.T) {
	k := newCwdKit(t)
	a := k.repo("a", "ssh://git@example.com:22/team/a.git")
	k.coord("tx1", "1111", a, cwdB)
	if !coordSet("tx1", false) {
		t.Fatal("coordSet tx1")
	}
	if got := repoList("tx1"); got != cwdA+","+cwdB {
		t.Errorf("③ coord 갱신 뒤 기록 %q, 기대 A·B", got)
	}
	coordSet("tx1", false)
	coordCheck("tx1")
	b, _ := os.ReadFile(k.d + "/st/hc-lane/coordrepo.tx1")
	if strings.Count(string(b), cwdA) != 1 {
		t.Errorf("③ 같은 저장소는 한 줄: %q", b)
	}
	k.coord("tw5", "5555", a, cwdB)
	if _, _, rc := coordCheck("tw5"); rc != tri.Yes || repoList("tw5") != cwdA+","+cwdB {
		t.Errorf("③ 다음 watch 바퀴(coordCheck)에도 채워진다: %v %q", rc, repoList("tw5"))
	}
}
