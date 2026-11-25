package main

// 2026-11-22 Go 전환 재감사(아스트라 1바퀴) base 지적 가운데 hc-sweep 디스크·Tailscale 쪽 실패 시험(gofix2-base). 이름 앞 [ID]는 지적 ID.
// 기대값은 지적 원문의 「고칠 방향」 계약이다

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// o2DiskRep: du는 가짜 크기, hc-lane report는 rc로 답하고 키를 적는 가짜 실행기
func o2DiskRep(keys *[]string, rc int) runner {
	return func(_ time.Duration, _ []string, name string, args ...string) result {
		if name == "du" {
			return result{out: "1\tx\n"}
		}
		*keys = append(*keys, args[len(args)-1])
		return result{rc: rc}
	}
}

// [R-21] 비었거나 잘린 부족 표식(쓰다 끊김)은 「보냄」이 아니다 — 같은 사건을 다시 확인해 보낸다
func TestO2DiskDamagedMarkResends(t *testing.T) {
	for _, body := range []string{"", pendingMark, pendingMark + "\t", pendingMark + "\t\n"} {
		d := t.TempDir()
		state := filepath.Join(d, "disk-low")
		if err := os.WriteFile(state, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		lane := filepath.Join(d, "hc-lane")
		os.WriteFile(lane, []byte("#!/bin/sh\n"), 0o755)
		var keys []string
		if msg := diskAlert(1, 1, 10, state, d, "", false, lane, o2DiskRep(&keys, 0)); msg == "" || len(keys) != 1 {
			t.Errorf("손상된 표식 %q를 보낸 것으로 읽음(알림 %d번)", body, len(keys))
		}
	}
}

// [R-21] 표식은 원자 저장(임시 → rename) — 제자리 덮어쓰기면 중간에 끊긴 바퀴가 빈·잘린 표식을 남긴다.
// 같은 inode를 가리키는 하드 링크가 옛 내용을 그대로 보면 새 파일로 바꿔 끼운 것이다
func TestO2DiskAlertMarkAtomic(t *testing.T) {
	d := t.TempDir()
	state := filepath.Join(d, "disk-low")
	old := pendingMark + "\t2026-01-01T00:00:00.000000\t옛 글\n"
	if err := os.WriteFile(state, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	link := state + ".o2link"
	if err := os.Link(state, link); err != nil {
		t.Fatal(err)
	}
	lane := filepath.Join(d, "hc-lane")
	os.WriteFile(lane, []byte("#!/bin/sh\n"), 0o755)
	var keys []string
	if diskAlert(1, 1, 10, state, d, "", false, lane, o2DiskRep(&keys, 0)) == "" {
		t.Fatal("대기 표식인데 보내지 않음")
	}
	if b, _ := os.ReadFile(state); strings.HasPrefix(string(b), pendingMark) {
		t.Fatalf("보낸 뒤에도 대기: %q", b)
	}
	if b, _ := os.ReadFile(link); string(b) != old {
		t.Fatalf("표식을 제자리에 덮어씀(원자 저장 아님): 링크가 %q", b)
	}
}

// [R-25] lsof -F n 출력의 레코드 꼴을 검사한다 — 진단 줄(경고)·빈 pid 레코드·요청하지 않은 필드가 섞이면 목록이 온전하지 않다(삭제 보류 = nil)
func TestO2OpenPathsMalformed(t *testing.T) {
	me := strconv.Itoa(os.Getpid())
	self := "p" + me + "\nfcwd\nn/c\n"
	for name, out := range map[string]string{
		"자기 뒤 경고":    self + "lsof: WARNING: can't stat() nfs file system /x\n",
		"자기 앞 경고":    "lsof: WARNING: can't stat() nfs file system /x\n" + self,
		"빈 pid 레코드":  self + "p\nfcwd\nn/held\n",
		"숫자 아닌 pid":  self + "pabc\nfcwd\nn/held\n",
		"pid 앞 경로":   "n/held\n" + self,
		"요청 안 한 필드":  self + "p1\nfcwd\ntDIR\nn/held\n",
		"잘린 마지막 레코드": self + "p1\n",
	} {
		if m := opq(fakeRun(0, out, "", nil)); m != nil {
			t.Errorf("%s: 온전하지 않은 lsof 출력을 받아들임 %v", name, m)
		}
	}
	// 대조: 온전한 출력은 받아들인다
	if m := opq(fakeRun(0, "p1\nfcwd\nn/a/b\n"+self, "", nil)); m == nil || !m["/a/b"] {
		t.Fatalf("온전한 출력 %v", m)
	}
}

// [R-29] status가 끝나지 못함(시간 초과·출력 관 오류)이면 「꺼짐」 문구가 일부 찍혔어도 up을 부르지 않는다
func TestO2TsStatusUnknownNoUp(t *testing.T) {
	for name, st := range map[string]result{
		"시간 초과":   {out: tsStopped, rc: -1, err: errors.New("tailscale: 시간 초과")},
		"출력 관 오류": {out: tsStopped, rc: -1, err: exec.ErrWaitDelay},
	} {
		f := &fakeTS{res: map[string]result{"status": st, "up": {}}}
		tsCheck(t, f, 3, filepath.Join(t.TempDir(), "s"), false)
		if strings.Contains(strings.Join(f.calls, ","), "up") {
			t.Errorf("%s: 상태를 모르는데 up을 부름: %v", name, f.calls)
		}
	}
}

// [R-29] 재시도 플래그도 끝난 up의 출력에서만 읽는다 — up이 시간 초과면 남은 출력의 「tailscale up …」 줄로 다시 부르지 않는다
func TestO2TailscaleUpTimeoutNoRetry(t *testing.T) {
	f := &fakeTS{res: map[string]result{"status": {out: tsStopped, rc: 1},
		"up": {errs: tsNonDef, rc: -1, err: errors.New("tailscale: 시간 초과")},
		"up --accept-routes --advertise-exit-node --hostname=worker-a": {}}}
	tsCheck(t, f, 3, filepath.Join(t.TempDir(), "s"), false)
	if len(f.calls) != 2 {
		t.Fatalf("up이 끝나지 않았는데 출력의 플래그로 다시 부름: %v", f.calls)
	}
}

// [R-30] 시도 기록이 비었거나 잘렸거나 손상이면 한도를 모른다 — 다시 켜지 않고 기다린다(false)
func TestO2TsTakeDamagedRecord(t *testing.T) {
	now := time.Now()
	full := strconv.FormatInt(now.Unix()-60, 10)
	for name, body := range map[string]string{
		"빈 파일":   "",
		"잘린 시각":  full[:6],
		"손상된 줄":  "garbage\n",
		"줄 끝 없음": full,
	} {
		state := filepath.Join(t.TempDir(), "tailscale-up")
		if err := os.WriteFile(state, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if tsTake(state, 1, now) {
			t.Errorf("%s(%q): 손상된 기록인데 한도 1에서 다시 켬", name, body)
		}
	}
}

// [R-30] 기록은 임시 파일 → rename으로 쓴다(제자리 덮어쓰기면 중단 때 한도를 잃는다) — 하드 링크가 옛 내용을 그대로 봐야 한다
func TestO2TsTakeAtomic(t *testing.T) {
	now := time.Now()
	state := filepath.Join(t.TempDir(), "tailscale-up")
	old := strconv.FormatInt(now.Unix()-60, 10) + "\n"
	if err := os.WriteFile(state, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	link := state + ".o2link"
	if err := os.Link(state, link); err != nil {
		t.Fatal(err)
	}
	if !tsTake(state, 3, now) {
		t.Fatal("한도 안인데 false")
	}
	if b, _ := os.ReadFile(state); strings.Count(string(b), "\n") != 2 {
		t.Fatalf("기록 %q", b)
	}
	if b, _ := os.ReadFile(link); string(b) != old {
		t.Fatalf("기록을 제자리에 덮어씀(원자 저장 아님): 링크가 %q", b)
	}
}
