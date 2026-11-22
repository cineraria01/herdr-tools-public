package main

// 잠금과 쓰기(2026-11-22 gofix-ops — 감사 B06·A11·B07): 같은 HOME의 병합끼리 잠그고, settings.json은 cp -p 사본에 내용만 다시 써
// rename한다(권한·ACL·확장 속성이 남고, 링크는 두고 최종 대상을 바꿔 넣는다).

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"herdr-tools/internal/lockx"
	"herdr-tools/internal/ojson"
)

// lockName: 같은 HOME의 병합끼리 잠그는 파일(~/.local/state/herdr-sync 아래, flock)
const lockName = "settings-merge.lock"

// held: 병합 잠금 — 프로세스가 끝날 때까지 쥔다(끝나거나 죽으면 커널이 푼다)
var held *os.File

// lockMerge: ST/lockName을 flock으로 잡는다. 60초 안에 못 잡으면 아무것도 쓰지 않고 종료 1(모름 — 다음 실행이 다시)
func lockMerge(ST string) {
	if err := os.MkdirAll(ST, 0o777); err != nil {
		fail("%s 를 만들지 못함", ST)
	}
	said := false
	for end := time.Now().Add(60 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		f, busy, err := lockx.TryLock(ST + "/" + lockName)
		if err != nil {
			fail("잠금 %s: %v", lockName, err)
		}
		if !busy {
			held = f
			return
		}
		if !said { // 한 번 알린다 — 시험도 이 줄로 병합기가 잠금 앞에 왔음을 안다(R-30)
			fmt.Fprintf(os.Stderr, "%s: 다른 병합이 잠금(%s)을 쥐고 있어 기다린다\n", me, lockName)
			said = true
		}
		if time.Now().After(end) {
			fail("다른 병합이 60초 넘게 잠금(%s)을 쥐고 있음", lockName)
		}
	}
}

// writeSettings: 백업(bak/<시각>/settings.json, 같은 초면 -<pid>) 뒤 replace. 백업·교체용 사본 모두 cp -p라 권한·ACL·확장 속성이 남는다
func writeSettings(S, ST string, exists bool, work any) {
	data := []byte(ojson.Pretty(work))
	if obj(work) == nil {
		die1("고친 결과가 올바른 JSON이 아니라 쓰지 않음")
	}
	if err := os.MkdirAll(filepath.Dir(S), 0o777); err != nil {
		os.Exit(1)
	}
	if exists {
		bak := ST + "/bak/" + time.Now().Format("20060102-150405")
		if _, err := os.Lstat(bak); err == nil {
			bak = fmt.Sprintf("%s-%d", bak, os.Getpid())
		}
		if err := os.MkdirAll(bak, 0o777); err != nil || cpP(S, bak+"/settings.json") != nil {
			die1("백업 실패, 쓰지 않음")
		}
	}
	if err := replace(S, data); err != nil {
		die1("임시 파일 쓰기 실패, 쓰지 않음")
	}
}

// replace: S(링크면 최종 대상)를 data로 바꿔 넣는다 — 대상 곁에 사본(cp -p)을 만들어 내용만 다시 쓰고 fsync → rename → 폴더 fsync.
// 링크는 그대로 두고 대상을 제자리에서 자르지 않는다(끊겨도 옛 판이나 새 판). 대상이 없으면 새 파일(0600, umask)
func replace(S string, data []byte) (err error) {
	dst, err := final(S)
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.herdr-tmp.%d", dst, os.Getpid())
	os.Remove(tmp) // 같은 pid였던 옛 실행이 남긴 것
	defer func() {
		if err != nil {
			os.Remove(tmp)
		}
	}()
	if _, serr := os.Stat(dst); serr == nil {
		err = cpP(dst, tmp)
	} else if !os.IsNotExist(serr) {
		err = serr
	} else if f, cerr := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); cerr != nil {
		err = cerr
	} else {
		err = f.Close()
	}
	if err != nil {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, dst); err != nil {
		return err
	}
	d, err := os.Open(dst[:strings.LastIndex(dst, "/")+1] + ".")
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// final: 심볼릭 링크를 끝까지 따라간 경로(대상이 없어도 그 경로). 상대 링크는 링크가 든 폴더 기준 — 경로를 글자로 줄이지 않는다(..는 커널이 푼다)
func final(p string) (string, error) {
	for range 40 {
		fi, err := os.Lstat(p)
		if os.IsNotExist(err) {
			return p, nil
		}
		if err != nil {
			return "", err
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			return p, nil
		}
		l, err := os.Readlink(p)
		if err != nil {
			return "", err
		}
		if !strings.HasPrefix(l, "/") {
			l = p[:strings.LastIndex(p, "/")+1] + l
		}
		p = l
	}
	return "", fmt.Errorf("%s: 링크가 너무 깊다", p)
}

// cpP: /bin/cp -p — 권한·시각에 더해 macOS ACL·확장 속성까지(표준 라이브러리로는 옮기지 못한다, 셸 판과 같은 길). 60초 상한 —
// 넘기면 결과를 모르니 실패로 치고 쓰지 않는다
func cpP(src, dst string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "/bin/cp", "-p", src, dst)
	c.WaitDelay = 2 * time.Second
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("cp -p: %v %s", err, bytes.TrimSpace(out))
	}
	return nil
}
