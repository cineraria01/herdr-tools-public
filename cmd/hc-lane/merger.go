package main

// merger: 병합기 본체 hc-merger로 넘긴다(옛 bin/hc-lane·lib/hc-lane/merger-go.sh가 하던 것). 이 바이너리 곁(링크를 따른 실제 폴더)의
// hc-merger, 없으면 PATH의 것. HC_LANE_SELF = 이 hc-lane 경로(병합기가 착지·사건을 부른다), 인자 그대로. 잠금 fd(18·19)는
// closeInherited가 CLOEXEC로 막아 물려주지 않는다. 실행 파일을 환경 변수로 바꿔 끼우는 길은 없다

import (
	"os"
	"os/exec"
	"path/filepath"
)

func init() { register("merger", mergerMain, noHelp) }

func mergerMain(args []string) int {
	self := selfExe()
	bin := filepath.Join(filepath.Dir(self), "hc-merger")
	if st, err := os.Stat(bin); err != nil || !st.Mode().IsRegular() || st.Mode()&0o111 == 0 {
		p, err := exec.LookPath("hc-merger")
		if err != nil {
			die("merger: 병합기 hc-merger를 찾지 못했다(%s 곁에도 PATH에도 없음 — herdr-sync가 까는지)", filepath.Dir(self))
		}
		bin = p
	}
	env := append(os.Environ(), "HC_LANE_SELF="+self)
	err := execve(bin, append([]string{bin}, args...), env)
	die("merger: %s 를 띄우지 못했다: %v", bin, err)
	return 1
}

// selfExe: 이 바이너리의 실제 경로(링크를 따름). 모르면 PATH의 hc-lane
func selfExe() string {
	p, err := os.Executable()
	if err != nil {
		return "hc-lane"
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
