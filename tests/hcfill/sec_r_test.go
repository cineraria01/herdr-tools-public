package hcfill_test

// fill-edit-after(2026-11-22): 대기열 줄의 선행·우선·기계·지시문을 rm·add 없이 바꾸기(hc-fill edit). rm → add는 줄 ID와 주차 표식(14칸
// parked — 선행이 들어가면 refs/park/<레인>에서 --resume으로 이어 띄움)을 버려, 먼저 해 둔 준비 커밋 없이 새로 뜨게 했다.
// rm은 주차 줄을 --force 없이 지우지 않는다. add·edit --after는 다른 프로젝트 레인을 선행으로 받지 않는다(착지를 이 프로젝트 main에서만
// 찾아 영원히 「아직」) — --after-any-project면 <이름>@<프로젝트>로 적고 그 프로젝트 main에서 찾는다.

import "strings"

func init() { reg(98, "r", secR) }

func secR(k *K) {
	Q := k.Q
	lane := k.T + "/lane.log"
	k.fresh()
	rm(k.T + "/events")
	k.cpConf()

	// 먼저 띄움 → 주차(14칸 parked)
	k.add("ea", "--after", "pz", "--only", "box-1")
	k.run("run")
	k.lj("box-1/ea:claude-code:ea:idle:/home/me/src/app.wt/ea")
	k.set(`HC_FAKE_LS_LANE={"terminals":[{"address":"box-1/ea"}]}`)
	write(k.t, k.T+"/events", "2026-11-20T01:00:00Z\tm\tbox-1/ea\tea\t주차\tabcdef12\tlane/ea 준비\t1\n")
	k.run("run")
	rid := k.qcol("ea", 17)
	k.check("edit 앞: 주차", k.where("ea") == "wait - -" && k.col14("ea") == "parked" && rid != "", read(Q))
	k.lj()
	k.unset("HC_FAKE_LS_LANE")

	// 잘못된 값은 종료 2, 대기열 그대로
	q0 := read(Q)
	bad := [][]string{
		{"edit", "nope", "--priority", "high"},
		{"edit", "ea"},
		{"edit", "ea", "--priority", "아주"},
		{"edit", "ea", "--only", "no-such-box"},
		{"edit", "ea", "--after", "Bad!"},
		{"edit", "ea", "--brief", k.T + "/no-such.md"},
		{"edit", "ea", "--bogus", "x"},
	}
	for _, a := range bad {
		k.run(a...)
		k.check("잘못된 edit 종료 2: "+strings.Join(a[1:], " "), k.rc == 2 && read(Q) == q0, k.out)
	}

	// 선행만 바꿈: 주차·줄 ID·나머지 칸 그대로
	k.run("edit", "ea", "--after", "pz2,pz3")
	k.check("edit --after: 선행만 바뀌고 주차·줄 ID 그대로",
		k.rc == 0 && k.qcol("ea", 4) == "pz2,pz3" && k.col14("ea") == "parked" && k.qcol("ea", 17) == rid && k.where("ea") == "wait - -" &&
			k.qcol("ea", 5) == "box-1", k.out, read(Q))

	// rm: 주차 줄은 --force 없이 안 지움
	k.run("rm", "ea")
	k.check("rm 주차 줄: 경고·거절(종료 2), 줄 그대로",
		k.rc == 2 && strings.Contains(k.out, "주차 중(refs/park/ea)") && strings.Contains(k.out, "hc-fill edit") && k.qcol("ea", 17) == rid, k.out)

	// 선행이 들어가면 주차 가지에서 이어 띄움
	write(k.t, k.T+"/commits", "aaaa1111 pz2: 끝\n")
	rm(lane)
	k.run("run")
	k.check("바꾼 선행 하나만 들어감: 아직 안 띄움", k.where("ea") == "wait - -" && !nonEmpty(lane), k.out)
	write(k.t, k.T+"/commits", "aaaa1111 pz2: 끝\nbbbb2222 pz3: 끝\n")
	k.run("run")
	k.check("바꾼 선행이 다 들어감: --resume으로 주차 가지에서 이어 띄움",
		k.where("ea") == "up box-1 box-1/ea" && hasFx(lane, "--resume") && k.col14("ea") == "resumed", k.out, read(lane))

	// 떠 있는 줄: --brief 거절, 우선은 바꾸고 큐에 안 냈으면 알림
	q0 = read(Q)
	k.run("edit", "ea", "--brief", k.brief("ea2"))
	k.check("떠 있는 줄 --brief: 종료 2", k.rc == 2 && read(Q) == q0, k.out)
	k.run("edit", "ea", "--priority", "높음")
	k.check("떠 있는 줄 --priority: 우선 칸 high, 병합기 큐에 안 냈다 알림",
		k.rc == 0 && k.qcol("ea", 13) == "high" && strings.Contains(k.out, "아직 병합기 큐에 안 냈다"), k.out)

	// 대기 줄: 우선·기계·선행 비우기·지시문(사본으로)
	k.add("eb", "--after", "pz", "--only", "box-1", "--no-early")
	rb := k.qcol("eb", 17)
	nb := k.T + "/eb-new.md"
	write(k.t, nb, "# 새 지시문 eb\n")
	k.run("edit", "eb", "--priority", "urgent", "--only", "-", "--after", "-", "--brief", nb)
	bf := k.qcol("eb", 3)
	k.check("대기 줄 edit: 우선·기계·선행·지시문(데이터 폴더 사본), 줄 ID·먼저 칸 그대로",
		k.rc == 0 && k.qcol("eb", 13) == "urgent" && k.qcol("eb", 5) == "-" && k.qcol("eb", 4) == "-" &&
			strings.HasPrefix(bf, k.DATA+"/briefs/added/") && read(bf) == "# 새 지시문 eb\n" && k.qcol("eb", 17) == rb && k.col14("eb") == "no-early",
		k.out, read(Q))
	k.run("edit", "eb", "--only", "box-2")
	k.check("edit --only: 기계 제한", k.rc == 0 && k.qcol("eb", 5) == "box-2", k.out)

	// rm --force: 주차 줄도 지움
	k.hSetQ("eb", map[int]string{14: "parked"})
	k.run("rm", "eb", "--force")
	k.check("rm --force: 주차 줄 지움", k.rc == 0 && k.where("eb") == "", k.out)
	k.add("ec")
	k.run("rm", "ec")
	k.check("rm 주차 아닌 줄: 그대로 지움", k.rc == 0 && k.where("ec") == "", k.out)

	// 다른 프로젝트 선행
	k.fresh()
	rm(k.T + "/events")
	k.run("add", "--name", "spec", "--project", "tools", "--brief", k.brief("spec"))
	k.check("다른 프로젝트(tools) 레인 넣기", k.rc == 0, k.out)
	k.run("add", "--name", "xa", "--project", "app", "--brief", k.brief("xa"), "--after", "spec")
	k.check("add --after 다른 프로젝트 레인: 경고·거절(종료 2)",
		k.rc == 2 && strings.Contains(k.out, "tools") && strings.Contains(k.out, "--after-any-project") && k.where("xa") == "", k.out)
	k.add("xc", "--only", "box-1", "--no-early")
	k.run("edit", "xc", "--after", "spec")
	k.check("edit --after 다른 프로젝트 레인: 종료 2", k.rc == 2 && k.qcol("xc", 4) == "-", k.out)
	k.run("edit", "xc", "--after", "spec", "--after-any-project")
	k.check("edit --after-any-project: <이름>@<프로젝트>", k.rc == 0 && k.qcol("xc", 4) == "spec@tools", k.out)
	k.run("add", "--name", "xa", "--project", "app", "--brief", k.brief("xa"), "--after", "spec", "--after-any-project", "--only", "box-1", "--no-early")
	k.check("add --after-any-project: <이름>@<프로젝트>로 적음", k.rc == 0 && k.qcol("xa", 4) == "spec@tools", k.out)
	k.run("add", "--name", "xd", "--project", "app", "--brief", k.brief("xd"), "--after", "ea", "--only", "box-1", "--no-early")
	k.check("add --after 같은 프로젝트·대기열에 없는 이름(손으로 띄운 레인일 수 있음): 넣음", k.rc == 0 && k.qcol("xd", 4) == "ea", k.out)
	rm(lane)
	rm(k.T + "/git.log")
	k.run("run")
	k.check("다른 프로젝트 선행 아직: 안 띄움", k.where("xa") == "wait - -" && strings.Contains(k.out, "xa ← spec@tools: 아직"), k.out)
	write(k.t, k.T+"/commits", "aaaa1111 spec: 끝\n")
	rm(k.T + "/git.log")
	k.run("run")
	k.check("다른 프로젝트 선행 착지: 그 프로젝트(tools) 저장소에서 찾아 띄움",
		k.up("xa") && strings.Contains(k.out, "xa ← spec@tools: 들어감") && hasF(k.T+"/git.log", "-C /Users/me/src/tools log"), k.out, read(k.T+"/git.log"))
}
