package main

// --carry: hc-fill이 실어 보낸 지휘 기계 .qa 자료(tar)를 작업 트리에 풀고, 정리(clean·gc) 때 바뀌지 않은 것을 거둔다
// (2026-11-26 ht-fill-carry-qa, 감사 atp3u25 중간 1·2 — 실은 파일이 .qa를 무시하지 않는 저장소에서 clean·gc를 막고, 무시하는
// 저장소에서는 메인 폴더로 옮겨져 다음 레인의 같은 이름 다른 내용에서 clean이 멈췄다). 실은 파일은 레인 산출물이 아니다

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"herdr-tools/internal/cksum"
)

// carryList: 작업 트리 git 폴더 안 실은 파일 목록(줄마다 「<cksum 출력><탭><작업 트리 상대 경로>」). worktree remove가 함께 지운다
const carryList = "hc-carry"

// carryManifest: tar의 일반 파일마다 목록 한 줄
func carryManifest(r io.Reader) (string, error) {
	var b strings.Builder
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return b.String(), nil
		}
		if err != nil {
			return "", fmt.Errorf("묶음 읽기: %w", err)
		}
		if h.Typeflag != tar.TypeReg || strings.ContainsAny(h.Name, "\t\n") {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return "", fmt.Errorf("묶음 읽기: %w", err)
		}
		b.WriteString(cksum.Space(data) + "\t" + h.Name + "\n")
	}
}

// carryIn: --carry tar(이 기계 파일)를 대상 기계 작업 트리에 풀고(--on이면 hc exec -i) 목록을 적는다. 못 하면 방금 만든 작업 트리를 되돌리고 멈춘다
func carryIn() {
	if carry == "" {
		return
	}
	b, err := os.ReadFile(carry)
	if err != nil {
		die("실어 보낼 묶음을 열지 못했다(%v). %s", err, rollback())
	}
	m, err := carryManifest(bytes.NewReader(b))
	if err != nil {
		die("실어 보낼 묶음이 tar가 아니다(%v). %s", err, rollback())
	}
	if _, r := xi(bytes.NewReader(b), "tar", "-xf", "-", "-C", wt); r != 0 {
		die("실어 보낼 자료를 작업 트리 %s%s 에 풀지 못했다(종료 %d). %s", onc(), wt, r, rollback())
	}
	if _, r := xi(strings.NewReader(m), "sh", "-c", `d=$(git -C "$1" rev-parse --absolute-git-dir) && cat >"$d/$2"`, "hc-lane", wt, carryList); r != 0 {
		die("실은 파일 목록을 작업 트리 %s%s 에 적지 못했다(종료 %d). %s", onc(), wt, r, rollback())
	}
}

// carryDropScript: $1 작업 트리 — 목록의 파일 가운데 cksum이 그대로이고 git이 따라가지 않는 것을 지우고 빈 위 폴더를 걷는다. 지운 경로를 한 줄씩.
// 추적 파일(레인이 커밋했거나 기준에 이미 있는 것)은 지우지 않는다(감사 atp3wc3 중간 1 — 지우면 「커밋 안 된 변경」으로 clean·gc가 멈췄다).
// 경로는 글롭이 아니라 글자 그대로 본다(--literal-pathspecs, 감사 atp3xcc 낮음 1)
const carryDropScript = `w=$1; g=$(git -C "$w" rev-parse --absolute-git-dir) || exit 1; m=$g/` + carryList + `
[ -f "$m" ] || exit 0
while IFS='	' read -r s p; do
  f=$w/$p
  [ -f "$f" ] && [ ! -L "$f" ] || continue
  [ "$(cksum <"$f")" = "$s" ] || continue
  ! git --literal-pathspecs -C "$w" ls-files --error-unmatch -- "$p" >/dev/null 2>&1 || continue
  rm -f "$f" || exit 1
  echo "$p"
  d=${p%/*}
  while [ "$d" != "$p" ] && rmdir "$w/$d" 2>/dev/null; do p=$d; d=${d%/*}; done
done <"$m"`

// carryDrop: 정리 전에 지금 작업 트리(wt)의 바뀌지 않은 실은 파일을 거둔다(--dry-run은 건너뜀). 레인이 고친 것은 그대로 — 남은 것 검사·.qa 옮기기가 다룬다
func carryDrop(P string) bool {
	if dry {
		return true
	}
	out, r := xout("sh", "-c", carryDropScript, "hc-lane", wt)
	if r != 0 {
		return false
	}
	if out != "" {
		fmt.Printf("%s실어 보낸 자료(바뀌지 않음) %d개를 거뒀다\n", P, len(strings.Split(out, "\n")))
	}
	return true
}
