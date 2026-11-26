package hclane_test

// --carry <tar>: 작업 트리를 만든 직후 그 안에 푼다(2026-11-26 ht-fill-carry-qa — 지휘 기계 .qa 자료가 다른 기계 작업 트리에 없어 레인이 멈췄다)

import (
	"archive/tar"
	"bytes"
	"path/filepath"
	"strings"
)

func init() { reg(90, "s25-carry", s25Carry) }

func s25tar(k *K, files map[string]string) string {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	for n, s := range files {
		if tw.WriteHeader(&tar.Header{Name: n, Mode: 0o644, Size: int64(len(s))}) != nil {
			k.t.Fatal("tar")
		}
		tw.Write([]byte(s))
	}
	tw.Close()
	p := filepath.Join(k.T, "carry.tar")
	write(k.t, p, b.String())
	return p
}

func s25Carry(k *K) {
	k.mk("t25")
	tp := s25tar(k, map[string]string{".qa/d/sea.mp4": "영상", ".qa/carry/proto.py": "print(1)\n"})
	k.run("--on", "box", "--carry", tp, "--name", "lane-c", "--repo", k.R, "--", "echo", "hi")
	wt := k.R + ".wt/lane-c"
	k.check("--carry --on: 다른 기계(가짜 hc exec -i) 작업 트리 같은 상대 경로에 풂", k.rc == 0 &&
		read(wt+"/.qa/d/sea.mp4") == "영상" && read(wt+"/.qa/carry/proto.py") == "print(1)\n" && contains(k.hclog(".exec"), "tar"))
	k.run("--carry", tp, "--name", "lane-l", "--repo", k.R, "--", "echo", "hi")
	k.check("--carry 로컬: 작업 트리에 풂", k.rc == 0 && read(k.R+".wt/lane-l/.qa/d/sea.mp4") == "영상")
	bad := filepath.Join(k.T, "bad.tar")
	write(k.t, bad, "tar 아님")
	k.run("--on", "box", "--carry", bad, "--name", "lane-b", "--repo", k.R, "--", "echo", "hi")
	k.check("--carry 못 풂: 띄우지 않고 방금 만든 작업 트리를 되돌림", k.rc != 0 && !exists(k.R+".wt/lane-b") && !contains(k.hclog(""), "lane-b"))
	k.run("--read", "--carry", tp, "--name", "lane-r", "--repo", k.R, "--", "echo", "hi")
	k.check("--carry --read: 인자 거절", k.rc != 0)
	s25Clean(k)
}

// s25land: --carry로 띄우고 커밋 하나를 main에 넣는다(감사 atp3u25 중간 1·2 — 실은 파일이 clean·gc를 막거나 메인 폴더로 옮겨졌다)
func s25land(k *K, name, tp string) string {
	k.run("--on", "box", "--carry", tp, "--name", name, "--repo", k.R, "--", "true")
	wt := k.R + ".wt/" + name
	k.sh(wt, `echo "$1" >"$1.txt" && git add -- "$1.txt" && git commit -qm "$1" -- "$1.txt" && git push -q origin HEAD:main`, name)
	return wt
}

func s25Clean(k *K) {
	// .qa를 무시하지 않는 저장소: 바뀌지 않은 실은 파일은 clean·gc가 거두고, 레인이 고친 것은 지금처럼 멈춘다
	k.mk("t25c")
	k.seed(".gitignore", ".omc/")
	v1 := s25tar(k, map[string]string{".qa/carry/proto.py": "v1\n", ".qa/d/sea.mp4": "영상"})
	s25land(k, "c-ok", v1)
	k.run("clean", "--repo", k.R, "--branch", "c-ok")
	k.check("clean(.qa 무시 안 함): 바뀌지 않은 실은 파일은 거두고 통과", k.rc == 0 && k.gone("c-ok"))
	wt := s25land(k, "c-mod", v1)
	write(k.t, wt+"/.qa/carry/proto.py", "레인이 고침\n")
	k.run("clean", "--repo", k.R, "--branch", "c-mod")
	k.check("clean: 레인이 고친 untracked 실은 파일은 남기고 멈춤(cksum이 다르면 거두지 않음)", k.rc != 0 && read(wt+"/.qa/carry/proto.py") == "레인이 고침\n" &&
		!exists(wt+"/.qa/d/sea.mp4") && strings.Contains(k.errf(), "proto.py") && k.git(wt, "ls-files", ".qa") == "")
	s25land(k, "g-ok", v1)
	k.run("gc", "--repo", k.R, "--min-age", "0")
	k.check("gc(.qa 무시 안 함): 바뀌지 않은 실은 파일만 남은 작업 트리는 지움", !exists(k.R+".wt/g-ok"))

	// 따로 된 저장소(감사 atp3xcc 중간 1 — 실은 파일을 main에 올리는 시험이 위 c-mod를 추적 파일 꼴로 바꿔 cksum 비교를 덮지 못했다):
	// 레인이 커밋했거나 기준에 이미 있는 실은 파일(추적 파일)은 거두지 않는다
	k.mk("t25m")
	k.seed(".gitignore", ".omc/")
	k.seed(".qa/carry/a1.txt", "추적") // 글롭 [1]이 맞는 추적 파일(감사 atp3xcc 낮음 1)
	wt = k.R + ".wt/c-com"
	k.run("--on", "box", "--carry", v1, "--name", "c-com", "--repo", k.R, "--", "true")
	k.sh(wt, `git add -- .qa/carry/proto.py && git commit -qm c-com -- .qa/carry/proto.py && git push -q origin HEAD:main`)
	k.run("clean", "--repo", k.R, "--branch", "c-com")
	k.check("clean: 레인이 실은 파일을 커밋했으면 거두지 않고(추적 파일) 통과(감사 atp3wc3 중간 1)", k.rc == 0 && k.gone("c-com") && !strings.Contains(k.out, "정리가 끊긴 자리") &&
		strings.Contains(k.git(k.R, "show", "origin/main:.qa/carry/proto.py"), "v1"))
	s25land(k, "g-tr", v1)
	k.run("gc", "--repo", k.R, "--min-age", "0")
	k.check("gc: 기준에 이미 있는 실은 파일(추적)은 거두지 않고 작업 트리는 지움", !exists(k.R+".wt/g-tr"))
	s25land(k, "c-glob", s25tar(k, map[string]string{".qa/carry/a[1].txt": "글롭"}))
	k.run("clean", "--repo", k.R, "--branch", "c-glob")
	k.check("clean: 이름에 [ ]가 든 untracked 실은 파일을 글롭으로 읽지 않고 거둠(감사 atp3xcc 낮음 1)", k.rc == 0 && k.gone("c-glob"))

	// .qa를 무시하는 저장소: 실은 파일은 메인 폴더로 옮기지 않는다 — 같은 이름 다른 내용을 다음 레인이 실어도 clean이 통과
	k.mk("t25q")
	k.seed(".gitignore", ".omc/\n.qa/")
	k.git(k.R, "pull", "-q", "--ff-only")
	wt = s25land(k, "q-1", v1)
	write(k.t, wt+"/.qa/x/REPORT.md", "레인 보고\n")
	k.run("clean", "--repo", k.R, "--branch", "q-1")
	k.check("clean(.qa 무시): 실은 파일은 메인 폴더로 옮기지 않고, 레인이 만든 .qa는 지금처럼 옮김", k.rc == 0 && k.gone("q-1") &&
		!exists(k.R+"/.qa/carry/proto.py") && read(k.R+"/.qa/x/REPORT.md") == "레인 보고\n")
	write(k.t, k.R+"/.qa/carry/proto.py", "메인 폴더의 다른 판\n")
	s25land(k, "q-2", s25tar(k, map[string]string{".qa/carry/proto.py": "v2\n"}))
	k.run("clean", "--repo", k.R, "--branch", "q-2")
	k.check("clean(.qa 무시): 메인 폴더에 같은 이름 다른 내용이 있어도 실은 파일은 거두고 통과", k.rc == 0 && k.gone("q-2") &&
		read(k.R+"/.qa/carry/proto.py") == "메인 폴더의 다른 판\n")
}
