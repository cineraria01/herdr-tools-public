package main

// 저장소 읽기 점검(ht-worker-cordon, 2026-11-27, docs/hc-fill/cordon.md): hc-fill이 바퀴마다 이미 부르는 원격 조각(부하 + --spec-idle)에
// 「--repo-read <폴더>…」를 얹어, 코어가 띄운 프로그램(레인과 같은 조건)이 저장소 폴더를 읽을 수 있는지 본다 — 폴더 목록 + 그 안 파일 하나를
// 조금 읽기만(쓰지 않음·fetch 없음). 출력은 --spec-idle 한 줄 뒤에 폴더마다 「repo-read <차례> ok|bad <까닭>|missing」, 끝에
// 「repo-read end <수>」(끝 줄이 없으면 잘린 출력 — 읽는 쪽이 모름으로). 폴더마다 동시에, 전체가 repoReadMax 안에 안 끝나면 그 폴더는
// bad(안 끝남). 2026-11-27 고장은 읽기가 약 21초 막혔다가 EINTR로 끝났다 — 상한을 그보다 짧게 두고 「안 끝남」을 나쁨으로 센다
// (바퀴가 막힌 기계 하나에 수십 초를 쓰지 않게)

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// repoReadMax: 한 기계의 폴더 읽기 전체 상한(시험이 줄인다). 보통 읽기는 밀리초 — 5초 넘게 걸리는 폴더 목록은 레인도 못 쓴다
var repoReadMax = 5 * time.Second

// repoRead: 폴더마다 결과 한 줄(차례는 1부터)과 끝 줄
func repoRead(paths []string) []string {
	res := make([]string, len(paths))
	var mu sync.Mutex
	done := make(chan struct{}, len(paths))
	rd := repoReadOne
	for i, p := range paths {
		go func(i int, p string) {
			r := rd(p)
			mu.Lock()
			res[i] = r
			mu.Unlock()
			done <- struct{}{}
		}(i, p)
	}
	t := time.NewTimer(repoReadMax)
	defer t.Stop()
wait:
	for range paths {
		select {
		case <-done:
		case <-t.C:
			break wait
		}
	}
	mu.Lock()
	defer mu.Unlock()
	var o []string
	for i, r := range res {
		if r == "" { // 막힌 읽기는 그대로 둔다(끝낼 수 없다) — 프로세스가 끝나며 거둔다
			r = fmt.Sprintf("bad 읽기가 %s 안에 안 끝남", repoReadMax)
		}
		o = append(o, fmt.Sprintf("repo-read %d %s", i+1, r))
	}
	return append(o, fmt.Sprintf("repo-read end %d", len(paths)))
}

// repoReadOne: 폴더 하나 읽기(시험이 막힘을 흉내 낼 때 바꾼다)
var repoReadOne = repoReadDir

// repoReadDir: 「ok」 · 「missing」(폴더 없음 — 읽기 막힘이 아니다) · 「bad <까닭>」
func repoReadDir(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		p = filepath.Join(os.Getenv("HOME"), rest)
	}
	es, err := os.ReadDir(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "missing"
	case err != nil:
		return "bad " + flat(err.Error())
	}
	for _, e := range es {
		if !e.Type().IsRegular() {
			continue
		}
		f, err := os.Open(filepath.Join(p, e.Name()))
		if err != nil {
			return "bad " + flat(err.Error())
		}
		defer f.Close()
		if _, err := f.Read(make([]byte, 512)); err != nil && err != io.EOF {
			return "bad " + flat(err.Error())
		}
		break
	}
	return "ok"
}
