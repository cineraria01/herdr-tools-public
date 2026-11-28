package main

// 임시 찌꺼기 쓸기가 묻는 lsof(disk.go에서 뗌). 비용은 출력이 아니라 내 프로세스·fd 열거에 있다(2026-11-25 ht-sweep-lsof 실측:
// 후보만 +D로 물어도, cwd만 물어도 전체를 묻는 것과 걸린 시간이 같다) — 그래서 한 번에 묻고, 느린 기계는 상한 sweep.lsof_secs로 맞춘다

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// lsofPath: lsof는 /usr/sbin에 있는데 launchd 작업의 PATH에는 /usr/sbin이 없다. PATH가 먼저(시험의 가짜 lsof)
func lsofPath() string {
	for _, d := range append(filepath.SplitList(os.Getenv("PATH")), "/usr/sbin", "/sbin") {
		p := filepath.Join(d, "lsof")
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return "lsof"
}

// lsofArgs: 열린 경로를 묻는 인자. -b: 멈출 수 있는 커널 호출(마운트 지점·파일의 stat·readlink)을 피한다 — 응답 없는 네트워크·
// 디스크 이미지 볼륨에서 lsof가 상한까지 묶이지 않게(경로는 커널이 주는 vnode 경로라 그대로 나온다. 피했다는 경고는 -w로 끔).
// -u <내 uid>: 내 프로세스만 — root가 아니면 lsof는 남의 프로세스 파일을 어차피 못 보고, 지울 후보도 내 폴더뿐이다.
// 후보만 묻는 +D는 -b와 함께 못 쓰고(lsof 사용 오류) 열거 비용도 같아 쓰지 않는다.
// -F fn: f 줄도 요청한다 — macOS lsof는 -F n에도 f 줄을 내지만 리눅스 lsof는 n 줄만 내 온전함 검사(레코드마다 f·n)를 못 넘는다
func lsofArgs() []string {
	return []string{"-n", "-P", "-w", "-b", "-u", strconv.Itoa(os.Getuid()), "-F", "fn"}
}

// openPaths: 살아 있는 프로세스가 연 경로 집합. 실패(상한 limit 넘김 포함)면 nil. 출력이 온전한지 본다(E13·R-25): 요청한 -F fn 꼴의 레코드만 —
// 「p<숫자>」로 시작하고 그 뒤 f·n 줄만, 레코드마다 f·n이 하나 이상. 그 밖의 줄(경고·진단·요청 안 한 필드), p 앞 줄, 빈·숫자 아닌 pid,
// 잘린 레코드가 있거나 lsof를 부른 이 프로세스(적어도 cwd)가 없으면 잘렸거나 다른 출력이라 nil(삭제 보류)
func openPaths(r runner, limit time.Duration) map[string]bool {
	res := r(limit, nil, lsofPath(), lsofArgs()...)
	if res.err != nil || res.rc != 0 || res.out == "" {
		return nil
	}
	m, me, pid, self := map[string]bool{}, strconv.Itoa(os.Getpid()), "", false
	nf, nn := 0, 0
	whole := func() bool { return pid == "" || nf > 0 && nn > 0 } // 앞 레코드가 온전한가(첫 레코드 전이면 참)
	for _, l := range strings.Split(strings.TrimSuffix(res.out, "\n"), "\n") {
		switch {
		case l == "":
			return nil
		case l[0] == 'p':
			if !whole() || len(l) == 1 || strings.Trim(l[1:], "0123456789") != "" {
				return nil
			}
			pid, nf, nn = l[1:], 0, 0
		case pid == "": // 첫 p 앞 줄
			return nil
		case l[0] == 'f':
			nf++
		case l[0] == 'n':
			nn++
			if pid == me {
				self = true
			}
			if strings.HasPrefix(l, "n/") {
				m[l[1:]] = true
			}
		default:
			return nil // 요청 안 한 필드·오류 문구·잘린 줄
		}
	}
	if pid == "" || !whole() || !self {
		return nil
	}
	return m
}

// lsofLimit: lsof 한 번의 상한(설정 sweep.lsof_secs, 기본 60초). 양수가 아니면 그렇다고 찍고 60초 — 상한 없이 돌지도, 쓸기를 끄지도 않는다
func lsofLimit(v string, ok bool) time.Duration {
	if ok {
		if s, err := strconv.ParseFloat(v, 64); err == nil && s > 0 {
			return time.Duration(s * float64(time.Second))
		}
		if v != "" {
			fmt.Printf("디스크 쓸기: sweep.lsof_secs 값이 양수가 아님(%s) — 60초로\n", v)
		}
	}
	return 60 * time.Second
}
