package hcfill_test

// 2026-11-26 ht-fill-carry-qa: 지시문이 가리키는 지휘 기계 .qa 자료·저장소 밖 파일을 레인 작업 트리에 실어 보낸다
// (한 프로젝트 레인이 다른 기계 새 작업 트리에서 떠 「시안·proto.py·DESIGN.md가 이 기계에 없다」로 멈췄다)

import (
	"os"
	"path/filepath"
	"strings"
)

func init() { reg(103, "carryqa", secCarryQA) }

func secCarryQA(k *K) {
	in := strings.Contains
	k.fresh()
	R := k.T + "/coordrepo"
	mkdirs(k.t, R+"/.qa/d", R+"/.qa/proto", k.T+"/scratch")
	write(k.t, R+"/.qa/d/sea.mp4", "영상")
	write(k.t, R+"/.qa/proto/a.py", "print(1)\n")
	X := k.T + "/scratch/DESIGN.md"
	write(k.t, X, "설계\n")
	TL := k.T + "/scratch/tool"
	writeX(k.t, TL, "#!/bin/sh\n")
	k.confSed(`^repo +app +here .*`, "repo     app    here     "+R)

	bp := k.T + "/cq.md"
	write(k.t, bp, "# 지시문 cq\n시안 `.qa/d/sea.mp4`와 .qa/proto 폴더, 없는 .qa/없음.md 를 본다.\n설계 "+X+" 를 읽는다. 절대 "+R+"/.qa/d/sea.mp4 도.\n"+
		"도구 "+TL+" 와 /bin/sh 로 돌린다. 다시 .qa/d/sea.mp4를 본다.\n")
	k.run("add", "--name", "cq", "--project", "app", "--brief", bp)
	cp := read(k.DATA + "/briefs/added/cq.md")
	k.check("add: 저장소에 없는 .qa 경로는 경고", k.rc == 0 && in(k.out, "경고: 지시문의 .qa/없음.md"), k.out)
	k.check("add: 조사가 붙은 있는 경로(.qa/d/sea.mp4를)에는 거짓 「없다」 경고 없음(감사 atp3u25 낮음 1)", !in(k.out, "sea.mp4를 가"), k.out)
	k.check("add: 저장소 밖 파일은 「다른 기계 레인은 못 읽는다 — .qa로」 경고", in(k.out, X+" — 다른 기계 레인은 이 파일을 못 읽는다 — .qa로 옮겨라"), k.out)
	k.check("add: 지시문 사본의 경로를 .qa/carry/<이름>·저장소 상대로 바꿔 씀(원본은 그대로)",
		in(cp, "설계 .qa/carry/DESIGN.md 를") && in(cp, "절대 .qa/d/sea.mp4 도") && !in(cp, X) && !in(cp, R+"/") && in(read(bp), X), cp)
	k.check("add: 저장소 밖 파일 사본을 데이터 폴더에", read(k.DATA+"/carry/cq/DESIGN.md") == "설계\n")
	k.check("add: 실행 파일(도구)·집·임시 폴더 밖(/bin/sh)은 싣지도 바꿔 쓰지도 않음", in(cp, "도구 "+TL+" 와 /bin/sh 로") && !exists(k.DATA+"/carry/cq/tool") && !exists(k.DATA+"/carry/cq/sh"), cp)

	k.run("run")
	cw := k.T + "/carried-cq"
	k.check("run(다른 기계): hc-lane --carry 묶음에 .qa 파일·폴더·사본이 같은 상대 경로로", in(read(k.T+"/lane.log"), "--carry") && in(read(k.T+"/lane.log"), "--on\n") &&
		read(cw+"/.qa/d/sea.mp4") == "영상" && read(cw+"/.qa/proto/a.py") == "print(1)\n" && read(cw+"/.qa/carry/DESIGN.md") == "설계\n", k.out)
	tars, _ := filepath.Glob(k.DATA + "/carry/*.tar")
	k.check("run: 넘긴 묶음 파일은 지움", len(tars) == 0, strings.Join(tars, " "))

	// 깔리기 전에 add된 줄(지시문 사본이 옛 꼴 그대로): 띄울 때 바꿔 쓰고 싣는다(감사 atp3u25 중간 3)
	k.fresh()
	ob := k.T + "/old.md"
	write(k.t, ob, "# 지시문 old\n설계 "+X+" 와 절대 "+R+"/.qa/d/sea.mp4 를 본다.\n")
	k.run("add", "--name", "old", "--project", "app", "--brief", ob)
	write(k.t, k.DATA+"/briefs/added/old.md", read(ob)) // 옛 판이 쓴 사본 꼴
	rm(k.DATA + "/carry/old")
	k.run("run")
	rf := read(k.HOME + "/.local/share/hc-fill/runs/old-run.md")
	k.check("옛 줄: 띄울 때 저장소 .qa 절대 경로·저장소 밖 파일을 바꿔 쓰고 실음", in(rf, "설계 .qa/carry/DESIGN.md 와 절대 .qa/d/sea.mp4 를") &&
		read(k.T+"/carried-old/.qa/carry/DESIGN.md") == "설계\n" && read(k.T+"/carried-old/.qa/d/sea.mp4") == "영상", k.out, rf)

	// 상한 넘침: 띄우지 않고 막힘 사건
	k.fresh()
	write(k.t, R+"/.qa/big/big.bin", strings.Repeat("x", 7340033))
	writeX(k.t, R+"/.qa/big/a-unreadable.bin", "x") // 이름 차례로 먼저 오는 읽을 수 없는 파일: 상한 판정은 읽기 전에(크기로) 한다
	os.Chmod(R+"/.qa/big/a-unreadable.bin", 0)
	bb := k.T + "/cbig.md"
	write(k.t, bb, "# 지시문 cbig\n큰 자료 .qa/big 을 본다\n")
	k.run("add", "--name", "cbig", "--project", "app", "--brief", bb)
	k.run("run")
	k.check("상한 넘침: 띄우지 않음(대기로 남음)", k.state("cbig") == "wait" && !in(read(k.T+"/lane.log"), "cbig") && !exists(k.T+"/carried-cbig"), k.out)
	k.check("상한 넘침: 까닭과 막힘 사건(파일을 읽기 전에 크기로 판정)", in(k.out, "상한 7340032바이트를 넘음") && !in(k.out, "읽지 못해") &&
		in(read(k.T+"/report.log"), "report 막힘"), k.out, read(k.T+"/report.log"))
}
