package hcfill_test

// 2026-11-28 ht-fill-smalls-1128 ③: 쓰기 레인 지시문에 「파일:」·「쓰기:」 예약 줄이 없으면 add·edit(지시문 바꿈) 때 알림 한 줄(막지 않음).
// 11-27 같은 폴더를 고치는 두 레인이 나란히 떠 뒤 것이 합친 뒤 컴파일 실패로 반송됐다 — 겹침 대기는 그 줄로만 걸리는데 한쪽 지시문에 없었다

import "strings"

func init() { reg(105, "claimnote", secClaimNote) }

const claimNote = "알림: 지시문에 「파일:」 줄이 없다 — 다른 레인과 겹쳐도 기다리지 않는다"

func secClaimNote(k *K) {
	in := strings.Contains
	R := k.T + "/coordrepo"
	k.confSed(`^repo +app +here .*`, "repo     app    here     "+R)
	write(k.t, R+"/.keep", "")
	brief := func(nm, body string) string {
		p := k.T + "/" + nm + ".md"
		write(k.t, p, "# 지시문 "+nm+"\n"+body+"\n")
		return p
	}
	add := func(nm, bp string, more ...string) {
		k.run(append([]string{"add", "--name", nm, "--project", "app", "--brief", bp}, more...)...)
	}
	notes := func() int { return strings.Count(k.out, claimNote) }

	k.fresh()
	add("cn1", brief("cn1", "본문만 있다"))
	k.check("① 「파일:」 줄 없음: 알림 한 줄·넣기는 됨(종료 0)", k.rc == 0 && notes() == 1 && k.state("cn1") == "wait", k.out)
	add("cn2", brief("cn2", "파일: cmd/x.go · docs/x.md"))
	k.check("② 「파일:」 줄 있음: 알림 없음", k.rc == 0 && notes() == 0, k.out)
	add("cn3", brief("cn3", "- 쓰기: docs/"))
	k.check("② 「- 쓰기:」 줄 있음: 알림 없음", k.rc == 0 && notes() == 0, k.out)
	add("cn4", brief("cn4", "파일:"))
	k.check("② 값 없는 「파일:」은 예약이 아니다: 알림", k.rc == 0 && notes() == 1, k.out)
	add("cn5", brief("cn5", "본문만"), "--overlap-ok")
	k.check("③ --overlap-ok: 알림 없음", k.rc == 0 && notes() == 0, k.out)
	add("cn6", brief("cn6", "본문만"), "--solo")
	k.check("③ --solo: 알림 없음", k.rc == 0 && notes() == 0, k.out)
	add("cn7", brief("cn7", "본문만"), "--no-audit")
	k.check("③ --no-audit(쓰기 레인 그대로): 알림", k.rc == 0 && notes() == 1, k.out)
	add("cn8", brief("cn8", "본문만"), "--no-audit", "--overlap-ok")
	k.check("③ --no-audit --overlap-ok: 알림 없음", k.rc == 0 && notes() == 0, k.out)
	add("cn9", brief("cn9", "이어서 본문만"), "--then", "cn1")
	k.check("③ --then: 알림 없음(선행 줄의 설정을 따른다)", k.rc == 0 && notes() == 0 && in(k.out, "이어 붙임 cn9 → cn1"), k.out)

	// edit: 지시문을 바꿀 때만
	k.run("edit", "cn2", "--brief", brief("cn2b", "본문만 바꿈"))
	k.check("④ edit 지시문을 「파일:」 없는 것으로: 알림 한 줄", k.rc == 0 && notes() == 1 && in(k.out, "바꿈 cn2"), k.out)
	k.run("edit", "cn2", "--brief", brief("cn2c", "파일: cmd/y.go"))
	k.check("④ edit 지시문을 「파일:」 있는 것으로: 알림 없음", k.rc == 0 && notes() == 0, k.out)
	k.run("edit", "cn1", "--priority", "높음")
	k.check("④ edit 지시문 안 바꿈: 알림 없음", k.rc == 0 && notes() == 0, k.out)
	k.run("edit", "cn5", "--brief", brief("cn5b", "본문만 바꿈"))
	k.check("④ edit 겹침 봄 끔 줄: 알림 없음", k.rc == 0 && notes() == 0, k.out)

	// 다른 알림(.qa 경고·비슷한 레인)과 차례 — 그것들은 그대로, 이 알림은 맨 끝 한 줄
	k.fresh()
	write(k.t, k.T+"/dup-a.md", "# 같은 일 고치기\n파일: a.go\n")
	add("dup-a", k.T+"/dup-a.md")
	write(k.t, k.T+"/dup-b.md", "# 같은 일 고치기\n.qa/none.txt 를 본다\n")
	add("dup-b", k.T+"/dup-b.md")
	ls := strings.Split(k.out, "\n")
	idx := func(p string) int {
		for i, l := range ls {
			if strings.HasPrefix(l, p) {
				return i
			}
		}
		return -1
	}
	w, d, s, n := idx("경고: 지시문의 .qa/none.txt"), idx("더함 dup-b"), idx("알림: 비슷한 레인"), idx(claimNote)
	k.check("⑤ 차례: .qa 경고 → 더함 → 비슷한 레인 → 이 알림(맨 끝)", w >= 0 && w < d && d < s && s < n && n == len(ls)-1 && notes() == 1, k.out)
}
