package hclane_test

// ht-clean-unstick(2026-11-25): 레인이 판정을 받자마자 clean으로 가면 hc-fill이 띄운 감사 터미널(끝남)이 작업 트리를 쥐고 있어
// clean이 멈췄다(hc-fill이 닫는 바퀴보다 레인이 빨랐다). clean은 이 레인 실행의 판정(수거 표시 있음)을 낸 감사 터미널이 끝났으면
// 그 주소로 닫고 계속하고, 일하는 중이면 그대로 멈춘다. 정리가 끝나면 마지막 report가 막힘일 때 「막힘 풀림」 중간을 적는다.

import (
	"fmt"
	"path/filepath"
	"strings"
)

func init() { reg(31, "clean-audit", cleanAuditSec) }

func cleanAuditSec(k *K) {
	k.mk("t31")
	k.seed(".gitignore", ".omc/")
	k.land("c-aud")
	wt := k.wt
	vd := filepath.Join(k.T, ".local/share/hc-fill/verdicts/c-aud")
	write(k.t, vd+"/a1-x.md", "aid\ta1-x\nlane\tc-aud\nrun\tt7@100\nsha\t"+strings.Repeat("ab", 20)+"\nverdict\tpass\nbase\tmain\n")
	ls := func(terms ...string) string {
		var b []string
		for _, a := range terms {
			b = append(b, fmt.Sprintf(`{"address":"%s","agent":"claude-code","cwd":"%s","state":"live"}`, a, wt))
		}
		return `{"machines":[{"name":"box","state":"connected"}],"terminals":[` + strings.Join(b, ",") + `]}`
	}
	get := func(st string) string {
		return fmt.Sprintf(`{"address":"t8","name":"audit-a1-x","cwd":"%s","state":"live","status":{"app":"claude-code","state":"%s"}}`, wt, st)
	}
	lane := []string{"HC_TERMINAL_ID=t7", "HC_LANE=c-aud", "HC_LANE_JOB=t7@100", "HC_FAKE_GET_ADDR=t8"}
	clean := func(st string) {
		write(k.t, k.HCLOG+".lsfile", ls("t7", "t8")+"\n")
		write(k.t, k.HCLOG+".lsfile.afterkill", ls("t7")+"\n")
		k.runE(append(lane, "HC_FAKE_GET_BODY="+get(st)), "clean", "--repo", k.R, "--branch", "c-aud")
	}
	kills := func() string { return read(k.HCLOG + ".kill") }

	// 레인이 clean 멈춤으로 막힘을 적은 꼴(지휘에 간 사건)
	k.runE(lane, "report", "막힘", "-", "clean 멈춤: 감사 터미널 t8 이 이 작업 트리에서 살아 있음")

	clean("working")
	k.check("clean: 일하는 중인 감사 터미널은 닫지 않고 멈춤", k.rc != 0 && strings.Contains(k.errf(), "t8") && k.kept("c-aud") && kills() == "")

	clean("done")
	k.check("clean: 판정을 아직 수거하지 않았으면(.ack 없음) 끝난 감사도 닫지 않고 멈춤", k.rc != 0 && k.kept("c-aud") && kills() == "")

	write(k.t, vd+"/a1-x.ack", "taken\n")
	k.runE(append(lane, "HC_LANE_JOB=t9@1", "HC_FAKE_GET_BODY="+get("done")), "clean", "--dry-run", "--repo", k.R, "--branch", "c-aud")
	k.check("clean: 판정이 이 실행 것이 아니면(다른 실행 ID) 닫지 않고 멈춤", k.rc != 0 && k.kept("c-aud") && kills() == "")

	k.runE(append(lane, "HC_FAKE_GET_BODY="+get("done")), "clean", "--dry-run", "--repo", k.R, "--branch", "c-aud")
	k.check("clean --dry-run: 닫을 감사 터미널을 알리고 닫지 않음", k.rc == 0 && strings.Contains(k.out, "t8") && kills() == "" && k.kept("c-aud"))

	clean("done")
	k.check("clean: 이 레인 실행의 판정을 수거한 끝난 감사 터미널은 그 주소로 닫고 정리", k.rc == 0 && strings.TrimSpace(kills()) == "t8" && k.gone("c-aud") &&
		strings.Contains(k.out, "끝난 감사 터미널 닫음 t8"))
	ev := read(filepath.Join(k.T, ".local/state/hc-lane/events.tsv"))
	k.check("clean: 정리가 끝나고 마지막 report가 막힘이면 「막힘 풀림」 중간을 적음", strings.Contains(ev, "\t중간\t-\t막힘 풀림"))

	k.runE(lane, "clean", "--repo", k.R, "--branch", "c-aud")
	k.check("clean: 다시 돌려도 풀림을 또 적지 않음", k.rc == 0 && strings.Count(read(filepath.Join(k.T, ".local/state/hc-lane/events.tsv")), "막힘 풀림") == 1)

	// 감사 atp1pxd 낮음 2: hc-fill 바퀴가 같은 감사 터미널을 먼저 닫아 clean의 hc kill이 실패해도, 다시 본 목록에 없으면 멈추지 않는다
	k.land("c-aud2")
	wt = k.wt
	vd2 := filepath.Join(k.T, ".local/share/hc-fill/verdicts/c-aud2")
	write(k.t, vd2+"/a1-x.md", "aid\ta1-x\nlane\tc-aud2\nrun\tt7@100\nsha\t"+strings.Repeat("ab", 20)+"\nverdict\tpass\nbase\tmain\n")
	write(k.t, vd2+"/a1-x.ack", "taken\n")
	write(k.t, k.HCLOG+".killfail", "1\n")
	write(k.t, k.HCLOG+".lsfile", ls("t7", "t8")+"\n")
	write(k.t, k.HCLOG+".lsfile.afterkill", ls("t7")+"\n")
	k.runE([]string{"HC_TERMINAL_ID=t7", "HC_LANE=c-aud2", "HC_LANE_JOB=t7@100", "HC_FAKE_GET_ADDR=t8", "HC_FAKE_GET_BODY=" + get("done")},
		"clean", "--repo", k.R, "--branch", "c-aud2")
	k.check("clean: 감사 터미널 닫기가 실패해도(이미 닫힘) 다시 본 목록에 없으면 정리", k.rc == 0 && k.gone("c-aud2"))
}
