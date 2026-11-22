// Package helptext: 하위 명령마다 둔 도움말 조각(<폴더>/<명령>.txt)을 한 사용법 글로 잇는다(2026-11-22 go-cmd-registry —
// 새 하위 명령을 더하는 레인이 한 도움말 파일의 같은 자리를 고치다 부딪히지 않게). 차례는 first 조각 먼저, 나머지는 파일 이름순.
package helptext

import (
	"io/fs"
	"strings"
)

// Join: dir 안 *.txt를 first(확장자 뺀 이름, 빈 글이면 없음) 먼저, 나머지는 이름순으로 이은 글. 바이너리에 든 글이라 못 읽으면 panic
func Join(fsys fs.FS, dir, first string) string {
	es, err := fs.ReadDir(fsys, dir) // 이름순
	if err != nil {
		panic(err)
	}
	var head, b strings.Builder
	for _, e := range es {
		if !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		t, err := fs.ReadFile(fsys, dir+"/"+e.Name())
		if err != nil {
			panic(err)
		}
		if first != "" && strings.TrimSuffix(e.Name(), ".txt") == first {
			head.Write(t)
		} else {
			b.Write(t)
		}
	}
	return head.String() + b.String()
}

// Names: dir 안 조각 이름(확장자 뺀 것, 이름순) — 등록표와 조각이 짝인지 보는 시험용
func Names(fsys fs.FS, dir string) []string {
	es, err := fs.ReadDir(fsys, dir)
	if err != nil {
		panic(err)
	}
	var n []string
	for _, e := range es {
		if strings.HasSuffix(e.Name(), ".txt") {
			n = append(n, strings.TrimSuffix(e.Name(), ".txt"))
		}
	}
	return n
}
