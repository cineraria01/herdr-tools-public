//go:build !windows

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"herdr-tools/internal/atomic"
)

// 재감사 hooks R-M03: own은 임대 파일을 internal/atomic.WriteFile로 바꿔 넣는다(임시 파일 O_EXCL·fsync·닫기·rename·폴더 fsync).
// 옛 os.WriteFile(owner.<pid>)+Rename은 그 이름에 미리 놓인 링크를 따라가 남의 파일을 덮었고, fsync 없이 편집을 허용했다
func TestOwnWritesLeaseAtomically(t *testing.T) {
	ld := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, ld+"/owner."+strconv.Itoa(os.Getpid())); err != nil {
		t.Fatal(err)
	}
	var eb bytes.Buffer
	g := &cg{meDone: true, sid: "s1", mypid: "1", mystart: "x", stderr: &eb}
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("own이 막았다: %v %s", r, eb.String())
			}
		}()
		g.own(ld, "a.go")
	}()
	if b, _ := os.ReadFile(victim); string(b) != "keep\n" {
		t.Errorf("미리 놓인 링크를 따라가 남의 파일을 덮었다: %q", b)
	}
	fi, err := os.Lstat(ld + "/owner")
	if err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("owner가 보통 파일이 아니다: %v %v", fi, err)
	}
	if b, _ := os.ReadFile(ld + "/owner"); !strings.HasPrefix(string(b), "s1"+us+"1"+us+"x"+us) || !strings.HasSuffix(string(b), us+"a.go\n") {
		t.Errorf("owner 줄: %q", b)
	}
}

// runCG: 훅 판정 하나를 돌려 종료 코드를 돌려준다(끝까지 가면 0)
func runCG(f func()) (rc int) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(cgExit)
			if !ok {
				panic(r)
			}
			rc = e.code
		}
	}()
	f()
	return 0
}

// stepRec: atomic의 단계(파일 fsync·rename·폴더 열기·폴더 fsync)를 기록하고 fail 단계에서 실패를 끼워 넣는다
type stepRec struct {
	got  []string
	fail map[string]bool
}

func (r *stepRec) step(s, arg string) error {
	r.got = append(r.got, s+" "+arg)
	if r.fail[s] {
		return errors.New("끼워 넣은 " + s + " 실패")
	}
	return nil
}

func (r *stepRec) install(t *testing.T) {
	old := atomic.SwapSteps(atomic.Steps{
		SyncFile: func(f *os.File) error {
			if err := r.step("syncFile", filepath.Base(f.Name())); err != nil {
				return err
			}
			return f.Sync()
		},
		Rename: func(a, b string) error {
			if err := r.step("rename", filepath.Base(b)); err != nil {
				return err
			}
			return os.Rename(a, b)
		},
		OpenDir: func(p string) (*os.File, error) {
			if err := r.step("openDir", p); err != nil {
				return nil, err
			}
			return os.Open(p)
		},
		SyncDir: func(f *os.File) error {
			if err := r.step("syncDir", f.Name()); err != nil {
				return err
			}
			return f.Sync()
		},
	})
	t.Cleanup(func() { atomic.SwapSteps(old) })
}

// 재감사 2바퀴 hooks/R-M209: own의 임대 쓰기가 임시 파일 fsync → owner로 rename → 임대 폴더 열기·fsync 순서이고,
// 어느 단계가 실패해도 편집을 허용하지 않는다(파일·폴더 Sync를 빼면 기록 순서가 달라져 이 시험이 실패한다)
func TestOwnLeaseSyncOrder(t *testing.T) {
	for _, fail := range []string{"", "syncFile", "rename", "openDir", "syncDir"} {
		t.Run("실패="+fail, func(t *testing.T) {
			ld := t.TempDir()
			r := &stepRec{fail: map[string]bool{fail: true}}
			r.install(t)
			var eb bytes.Buffer
			g := &cg{meDone: true, sid: "s1", mypid: "1", mystart: "x", stderr: &eb}
			rc := runCG(func() { g.own(ld, "a.go") })
			want := []string{"syncFile .owner.tmp", "rename owner", "openDir " + ld, "syncDir " + ld}
			if len(r.got) == 0 || !strings.HasPrefix(r.got[0], want[0]) {
				t.Fatalf("첫 단계는 임시 파일 fsync여야: %v", r.got)
			}
			r.got[0] = want[0]
			for i, s := range want {
				if fail != "" && strings.HasPrefix(s, fail+" ") {
					want = want[:i+1]
					break
				}
			}
			if strings.Join(r.got, "|") != strings.Join(want, "|") {
				t.Errorf("단계 %v, 바람 %v", r.got, want)
			}
			if wantRC := map[bool]int{true: 0, false: 2}[fail == ""]; rc != wantRC {
				t.Errorf("rc=%d, 바람 %d (%s)", rc, wantRC, eb.String())
			}
		})
	}
}

// 재감사 2바퀴 hooks/R-M202: rename 뒤 폴더 동기화가 실패한 임대(새 주인 줄이 이미 보임)를 같은 주인의 재시도가
// 「자기 임대」로 읽고 그냥 허용했다. 재시도는 임대 폴더 동기화를 마친 뒤에만 허용한다
func TestLeaseRetryAfterCommitError(t *testing.T) {
	leases := t.TempDir()
	rel := "a.go"
	ld := leases + "/" + md5hex(rel)
	if err := os.Mkdir(ld, 0o777); err != nil {
		t.Fatal(err)
	}
	var eb bytes.Buffer
	g := &cg{meDone: true, sid: "s1", mypid: "1", mystart: "x", stderr: &eb, leases: leases}
	r := &stepRec{fail: map[string]bool{"openDir": true}}
	r.install(t)
	if rc := runCG(func() { g.own(ld, rel) }); rc != 2 {
		t.Fatalf("첫 호출: rc=%d, 바람 2", rc)
	}
	if !g.owner(ld) || !g.mine() {
		t.Fatal("rename은 됐으니 주인 줄은 이미 자기 것이어야(재현 전제)")
	}
	r.got = nil
	if rc := runCG(func() { g.lease(rel, "1") }); rc != 2 {
		t.Errorf("폴더 동기화가 여전히 실패하는 재시도: rc=%d, 바람 2 (%s)", rc, eb.String())
	}
	if len(r.got) == 0 {
		t.Error("재시도가 임대 폴더 동기화를 시도하지 않았다")
	}
	r.fail, r.got = nil, nil
	if rc := runCG(func() { g.lease(rel, "1") }); rc != 0 {
		t.Errorf("폴더 동기화가 되는 재시도: rc=%d, 바람 0 (%s)", rc, eb.String())
	}
	if strings.Join(r.got, "|") != "openDir "+ld+"|syncDir "+ld {
		t.Errorf("재시도는 임대 폴더를 동기화한 뒤 허용해야: %v", r.got)
	}
}
