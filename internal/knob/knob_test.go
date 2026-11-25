//go:build !windows

package knob

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/gitx"
)

// 안전 관문(설계 문서 「안전 불변」)은 등록 표에 없다 — 이름이 표에 없으면 set·읽기 어느 길로도 값이 먹지 않는다
func TestSafetyNotKnobs(t *testing.T) {
	for _, n := range []string{"overlap", "audit", "audit_final", "local", "repo", "machine", "machine-has", "merger", "coord", "agent", "task", "launch"} {
		if _, ok := Find(n); ok {
			t.Errorf("안전·사실 값 %s가 등록 표에 있다", n)
		}
	}
	for _, d := range Defs {
		if d.Per == "" && d.Check(d.Default) != nil { // 묶음 손잡이(seats@ 등)는 기본이 없다 — 설정 줄 값이 기본
			t.Errorf("%s 기본값 %q가 자기 범위 밖", d.Name, d.Default)
		}
	}
	es, err := parse("overlap\toff\t사람\t손으로\t1\t-\t0\n", "x")
	if err != nil {
		t.Fatal(err)
	}
	if es["overlap"].Live(time.Now()) {
		t.Fatal("표에 없는 이름이 살아 있다고 읽혔다")
	}
}

func TestSetPinExpire(t *testing.T) {
	p := t.TempDir() + "/s/knobs.tsv"
	now := time.Unix(1_000_000, 0)
	if err := Set(p, Entry{Name: "idle", Value: "20", By: "a", Why: "왜\t탭", At: now.Unix(), Pin: true}, false, now); err != nil {
		t.Fatal(err)
	}
	if err := Set(p, Entry{Name: "idle", Value: "30", By: "b", Why: "x", At: now.Unix()}, false, now); !errors.Is(err, ErrPinned) {
		t.Fatalf("고정 값을 force 없이 바꿈: %v", err)
	}
	if _, err := Unset(p, "idle", false, now); !errors.Is(err, ErrPinned) {
		t.Fatalf("고정 값을 force 없이 지움: %v", err)
	}
	if err := Set(p, Entry{Name: "idle", Value: "30", By: "b", Why: "x", At: now.Unix(), Until: now.Unix() + 60}, true, now); err != nil {
		t.Fatal(err)
	}
	es, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := Find("idle")
	if v, src, e := Resolve(d, es, nil, "15", now); v != "30" || src != FromKnob || e.By != "b" {
		t.Fatalf("손잡이 값 = %s %s %+v", v, src, e)
	}
	if v, src, _ := Resolve(d, es, nil, "15", now.Add(2*time.Minute)); v != "15" || src != FromConf {
		t.Fatalf("기한 뒤 = %s %s (설정 15여야)", v, src)
	}
	if v, src, _ := Resolve(d, es, nil, "", now.Add(2*time.Minute)); v != "10" || src != FromDefault {
		t.Fatalf("기한 뒤·설정 없음 = %s %s (기본 10이어야)", v, src)
	}
	// 기한 지난 줄은 다음 쓰기에서 걷힌다
	if err := Set(p, Entry{Name: "hold", Value: "5", By: "c", Why: "y", At: now.Unix()}, false, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); strings.Contains(string(b), "idle\t") || !strings.Contains(string(b), "hold\t5\tc\ty\t") {
		t.Fatalf("파일 = %q", b)
	}
}

func TestBrokenFile(t *testing.T) {
	p := t.TempDir() + "/knobs.tsv"
	if err := os.WriteFile(p, []byte("idle\t20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("깨진 파일을 오류 없이 읽음")
	}
	now := time.Now()
	if err := Set(p, Entry{Name: "idle", Value: "5", Why: "x", At: now.Unix()}, false, now); err == nil {
		t.Fatal("깨진 파일 위에 썼다(사람이 보고 지우게 둬야)")
	}
	if es, err := Load(t.TempDir() + "/none"); err != nil || len(es) != 0 {
		t.Fatalf("없는 파일 = %v %v", es, err)
	}
}

func TestCheckSuggest(t *testing.T) {
	d, _ := Find("wake_every")
	for _, v := range []string{"0", "241", "-1", "abc", "07", ""} {
		if d.Check(v) == nil {
			t.Errorf("범위 밖 %q를 받음", v)
		}
	}
	if d.Check("15") != nil {
		t.Error("15를 거절")
	}
	if s := Suggest("idel"); len(s) == 0 || s[0] != "idle" {
		t.Errorf("idel → %v", s)
	}
	if s := Suggest("wake"); len(s) == 0 || s[0] != "wake_every" {
		t.Errorf("wake → %v", s)
	}
}

// 원격 참조에 실은 값을 공용 함수가 읽는다. 참조가 없거나 못 읽으면 기본값
func TestRemoteRead(t *testing.T) {
	defer func(o []Def) { Defs = o }(Defs)
	Defs = append(append([]Def(nil), Defs...), Def{Name: "t_remote", Tool: "t", Default: "3", Min: 1, Max: 9, Remote: true})
	now := time.Now()
	es := map[string]Entry{
		"t_remote": {Name: "t_remote", Value: "7", Until: now.Unix() + 600},
		"idle":     {Name: "idle", Value: "20"}, // Remote 아님 — 싣지 않는다
	}
	msg := FormatRemote(es, now)
	if strings.Contains(msg, "idle") || !strings.Contains(msg, "t_remote\t7\t") {
		t.Fatalf("메시지 = %q", msg)
	}
	if r := ParseRemote(msg, now.Add(time.Hour)); r.Get("t_remote") != "3" {
		t.Fatal("원격 값도 기한이 지나면 기본값")
	}

	d := t.TempDir()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	g := func(dir string, args ...string) string {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		o, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, o)
		}
		return strings.TrimSpace(string(o))
	}
	g(d, "init", "-q", "--bare", "o.git")
	g(d, "clone", "-q", "o.git", "r")
	if r, st := ReadRemote(d+"/r", io.Discard, now); st != gitx.No || r.Get("t_remote") != "3" {
		t.Fatalf("참조 없음 = %v %s", r, st)
	}
	et := g(d+"/r", "hash-object", "-t", "tree", "-w", "/dev/null")
	c := g(d+"/r", "commit-tree", et, "-m", msg)
	g(d+"/r", "push", "-q", "origin", c+":"+Ref)
	g(d, "clone", "-q", "o.git", "r2")
	r, st := ReadRemote(d+"/r2", io.Discard, now)
	if st != gitx.Yes || r.Get("t_remote") != "7" {
		t.Fatalf("원격 값 = %v %s", r, st)
	}
	if r, st := ReadRemote(d+"/nope", io.Discard, now); st != gitx.Unknown || r.Get("t_remote") != "3" {
		t.Fatalf("못 읽음 = %v %s", r, st)
	}
}
