package hcfill_test

// 2026-11-28 ht-carry-add-check: 실어 보낼 자료가 상한을 넘으면 띄울 때가 아니라 넣을 때(add·edit·--then) 그 자리에서 거절한다
// (지시문이 큰 .qa 폴더를 가리켜 add는 조용히 받고 띄울 때에야 막힘 사건이 와, 레인이 한 바퀴씩 늦게 떴다)

import (
	"fmt"
	"os"
	"strings"
)

func init() { reg(104, "carryadd", secCarryAdd) }

// 한 파일 묶음 어림 = 1024 + 1536 + 512 올림 — 이 크기까지가 상한(7,340,032) 안
const edgeOK = 7340032 - 1024 - 1536

func secCarryAdd(k *K) {
	in := strings.Contains
	R := k.T + "/coordrepo"
	k.confSed(`^repo +app +here .*`, "repo     app    here     "+R)
	brief := func(nm, body string) string {
		p := k.T + "/" + nm + ".md"
		write(k.t, p, "# 지시문 "+nm+"\n"+body+"\n")
		return p
	}
	add := func(nm, bp string, more ...string) {
		k.run(append([]string{"add", "--name", nm, "--project", "app", "--brief", bp}, more...)...)
	}
	trace := func(nm string) bool { // 대기열 줄·지시문 사본·carry 사본·사건 가운데 하나라도
		return k.state(nm) != "" && k.state(nm) != "-" || exists(k.DATA+"/briefs/added/"+nm+".md") || exists(k.DATA+"/carry/"+nm) || in(read(k.T+"/report.log"), nm)
	}

	// ① 큰 .qa 폴더 → add 거절·흔적 없음·큰 것 목록·안내
	k.fresh()
	write(k.t, R+"/.qa/big/big.bin", strings.Repeat("x", 7340033))
	write(k.t, R+"/.qa/big/small.txt", "s")
	write(k.t, R+"/.qa/tiny.txt", "t")
	bb := brief("cbig", "큰 자료 .qa/big 과 .qa/tiny.txt 를 본다")
	add("cbig", bb)
	out1, rc1 := k.out, k.rc
	k.check("① add: 상한을 넘으면 거절(종료 1 — add의 다른 검사 거절과 같은 꼴)", rc1 == 1 && in(out1, "실어 보낼 자료가 상한 7340032바이트를 넘음(파일 3개 약 ") && in(out1, "바이트) — 넣지 않음"), out1)
	k.check("① add: 큰 것부터 경로·파일 수·바이트, 고칠 안내 한 줄", in(out1, "  .qa/big — 파일 2개 약 ") && strings.Index(out1, ".qa/big —") < strings.Index(out1, ".qa/tiny.txt —") &&
		in(out1, "폴더 통째 대신 레인이 실제로 읽을 파일만 적어라(큰 자료는 저장소에 두거나 레인이 그 기계에서 만들게)"), out1)
	k.check("① add: 흔적 없음(대기열 줄·지시문 사본·carry 사본·사건)", !trace("cbig"), read(k.Q))
	add("cbig", bb)
	k.check("① 두 번째 실행: 같은 거절, 쌓이는 것 없음", k.rc == rc1 && k.out == out1 && !trace("cbig"), k.out)

	// ② edit → 거절·앞 지시문 그대로, 그 뒤 바퀴는 앞 지시문으로 띄움(반쯤 쓴 사본 없음)
	k.fresh()
	add("ce", brief("ce", "작은 자료 .qa/tiny.txt 를 본다"))
	prev := read(k.DATA + "/briefs/added/ce.md")
	k.run("edit", "ce", "--brief", bb, "--why", "시험")
	k.check("② edit: 거절(종료 2)·앞 지시문 그대로", k.rc == 2 && in(k.out, "— 넣지 않음") && read(k.DATA+"/briefs/added/ce.md") == prev && in(prev, ".qa/tiny.txt"), k.out)
	k.run("run")
	k.check("② edit 거절 뒤 바퀴: 앞 지시문으로 띄움", k.state("ce") != "wait" && in(read(k.HOME+"/.local/share/hc-fill/runs/ce-run.md"), "작은 자료"), k.out)

	// ③ --then → 거절·앞 레인 그대로. 셈은 띄울 때 실릴 전체(앞 단계 + 이어 붙일 것)
	k.fresh()
	write(k.t, R+"/.qa/half1/a.bin", strings.Repeat("a", 4000000))
	write(k.t, R+"/.qa/half2/b.bin", strings.Repeat("b", 4000000))
	add("tp", brief("tp", "앞 단계 .qa/half1 을 본다"))
	prev = read(k.DATA + "/briefs/added/tp.md")
	add("tn", brief("tn", "이어서 .qa/half2 를 본다"), "--then", "tp")
	k.check("③ --then: 앞 단계와 합쳐 넘으면 거절(종료 2)·앞 지시문 그대로·새 줄 없음", k.rc == 2 && in(k.out, "(파일 2개 약 ") && in(k.out, "  .qa/half1 — 파일 1개") && in(k.out, "  .qa/half2 — 파일 1개") &&
		read(k.DATA+"/briefs/added/tp.md") == prev && !in(read(k.Q), "tn"), k.out)

	// ④ 상한 바로 아래 → 통과(지금과 같은 출력), 한 바이트 위 → 거절
	k.fresh()
	write(k.t, R+"/.qa/edge/e.bin", strings.Repeat("e", edgeOK))
	add("cedge", brief("cedge", "경계 .qa/edge 를 본다"))
	k.check("④ 상한 바로 아래: 통과·「더함」 한 줄 그대로", k.rc == 0 && strings.HasPrefix(k.out, "더함 cedge (app") && !in(k.out, "상한"), k.out)
	write(k.t, R+"/.qa/edge/e.bin", strings.Repeat("e", edgeOK+1))
	add("cedge2", brief("cedge2", "경계 .qa/edge 를 본다"))
	k.check("④ 한 바이트 위: 거절", k.rc == 1 && in(k.out, "— 넣지 않음") && !trace("cedge2"), k.out)

	// ⑤ 하나하나는 아래, 합이 넘음 → 거절·합계·큰 것 순
	k.fresh()
	write(k.t, R+"/.qa/s5/big.bin", strings.Repeat("5", 5000000))
	add("csum", brief("csum", "둘 .qa/half1 과 .qa/s5 를 본다"))
	k.check("⑤ 합이 넘음: 거절·큰 것(.qa/s5) 먼저", k.rc == 1 && in(k.out, "(파일 2개 약 ") && strings.Index(k.out, ".qa/s5 —") >= 0 && strings.Index(k.out, ".qa/s5 —") < strings.Index(k.out, ".qa/half1 —"), k.out)

	// ⑥ 지휘 기계에 없는 .qa 경로 → 지금처럼 경고만(0으로 셈)
	k.fresh()
	add("cmiss", brief("cmiss", "없는 .qa/없음 과 .qa/tiny.txt 를 본다"))
	k.check("⑥ 없는 경로: 경고만·통과", k.rc == 0 && in(k.out, "경고: 지시문의 .qa/없음") && in(k.out, "더함 cmiss"), k.out)

	// ⑦ 저장소 밖 파일 사본이 섞임 → 합에 들어감(그 경로로 보임)·사본 안 남김
	k.fresh()
	mkdirs(k.t, k.T+"/scratch")
	X := k.T + "/scratch/ref.bin"
	write(k.t, X, strings.Repeat("r", 4000000))
	add("cext", brief("cext", "참고 "+X+" 와 .qa/half1 을 본다"))
	k.check("⑦ 저장소 밖 사본이 합에 들어감·원래 경로로 보임·사본 없음", k.rc == 1 && in(k.out, "  "+X+" — 파일 1개") && !exists(k.DATA+"/carry/cext"), k.out)

	// ⑧ 넣은 뒤 폴더가 커짐(이 판 전에 넣은 큰 줄과 같은 꼴) → 띄울 때 지금처럼 거절·막힘 사건
	k.fresh()
	add("cgrow", brief("cgrow", "자랄 자료 .qa/grow 를 본다"))
	k.check("⑧ 넣을 때는 작아 통과", k.rc == 0, k.out)
	write(k.t, R+"/.qa/grow/big.bin", strings.Repeat("g", 7340033))
	k.run("run")
	k.check("⑧ 띄울 때 거절·대기로 남음·막힘 사건", k.state("cgrow") == "wait" && in(k.out, "실패 cgrow: 실어 보낼 자료가 상한 7340032바이트를 넘음") &&
		in(read(k.T+"/report.log"), "report 막힘"), k.out)

	// ⑨ 두 판정의 일치 + 외부 가정(링크·읽기 권한 없는 파일·아주 많은 작은 파일·이름에 빈칸)
	write(k.t, R+"/.qa/edge/e.bin", strings.Repeat("e", edgeOK))
	write(k.t, R+"/.qa/edge1/e.bin", strings.Repeat("e", edgeOK+1))
	mkdirs(k.t, R+"/.qa/lnk")
	write(k.t, R+"/.qa/lnk/a.txt", "a")
	os.Symlink(R+"/.qa/big/big.bin", R+"/.qa/lnk/to-big") // 링크는 따라가지 않는다
	write(k.t, R+"/.qa/noread/x.bin", strings.Repeat("n", edgeOK))
	os.Chmod(R+"/.qa/noread/x.bin", 0) // 읽기 전에 크기로 센다
	write(k.t, R+"/.qa/sp/이름 빈칸.bin", strings.Repeat("p", edgeOK))
	for i := 0; i < 3584; i++ { // 2,048 × 3,584 + 1,024 = 상한 바로 위
		if i == 0 {
			mkdirs(k.t, R+"/.qa/many")
		}
		os.WriteFile(fmt.Sprintf("%s/.qa/many/%04d", R, i), []byte("m"), 0o644)
	}
	for _, c := range []struct{ nm, body string }{
		{"pa", ".qa/edge"}, {"pa1", ".qa/edge1"}, {"pb", ".qa/big"}, {"pc", ".qa/half1 과 .qa/half2"}, {"pd", ".qa/lnk"}, {"pe", ".qa/noread"}, {"pf", ".qa/sp"}, {"pg", ".qa/many"}, {"ph", ".qa/tiny.txt"},
	} {
		k.fresh()
		add(c.nm+"a", brief(c.nm+"a", "자료 "+c.body+" 를 본다"))
		atAdd := k.rc == 0
		// 띄울 때 판정: 같은 지시문을 자료 없이 넣고, 같은 상태로 되돌린 뒤 띄운다
		os.Rename(R+"/.qa", R+"/qa.hide")
		add(c.nm+"b", brief(c.nm+"b", "자료 "+c.body+" 를 본다"))
		os.Rename(R+"/qa.hide", R+"/.qa")
		k.run("rm", c.nm+"a", "--force")
		k.run("run")
		atLaunch := k.state(c.nm+"b") != "wait"
		k.check("⑨ 넣을 때와 띄울 때 판정이 같다: "+c.body+fmt.Sprintf("(넣을 때 %v, 띄울 때 %v)", atAdd, atLaunch), atAdd == atLaunch, k.out)
	}
	os.Chmod(R+"/.qa/noread/x.bin", 0o644)
}
