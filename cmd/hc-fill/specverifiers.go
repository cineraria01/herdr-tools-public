package main

// 추측 검증 기계 목록(merger-spec, 2026-11-22): 설정 줄 「merger_verifiers <프로젝트> <기계,…>」와 그 기계의 repo 줄을 원격 표식
// refs/herdr/spec-verifiers(빈 트리 커밋, 메시지 verified.FormatVerifiers, 날짜 0 고정 — 같은 목록이면 같은 SHA)로 올린다. 병합기가 바퀴마다
// 읽어 그 기계들에서 추측 검증을 띄운다(cmd/hc-merger/specrun.go). 줄을 지우면 표식을 지운다(추측 없음 — 지금 동작 그대로).
// 관리한 프로젝트는 ~/.local/share/hc-fill/spec-verifiers.tsv(지울 것을 알게)

import (
	"strings"

	"herdr-tools/internal/verified"
)

func specVerifiersTick() {
	sf := DATA + "/spec-verifiers.tsv"
	want := map[string][]verified.Verifier{}
	var ps []string
	for _, f := range confRows("merger_verifiers") {
		if len(f) < 3 {
			say("추측 검증 줄 꼴: merger_verifiers <프로젝트> <기계,…>")
			continue
		}
		p := f[1]
		ps = append(ps, p)
		for _, m := range strings.Split(f[2], ",") {
			path := strings.TrimRight(repoOf(p, m), "/")
			switch {
			case m == "":
			case !verified.MachineOK(m) || !strings.HasPrefix(path, "/"):
				say("추측 검증 %s: %s 기계 이름이나 repo 줄 경로가 없다 — 그 기계는 뺀다", p, m)
			default:
				want[p] = append(want[p], verified.Verifier{Machine: m, Path: path})
			}
		}
	}
	for _, x := range readRows(sf) {
		ps = append(ps, x.F(kvKey))
	}
	// 미리 줄(preverify) 끄기: 「merger_preverify <프로젝트> off」 — 목록 커밋에 verified.PreOffLine 한 줄(기본 켬)
	off := map[string]bool{}
	for _, f := range confRows("merger_preverify") {
		if len(f) >= 3 && f[2] == "off" {
			off[f[1]] = true
		} else {
			say("미리 검증 줄 꼴: merger_preverify <프로젝트> off")
		}
	}
	var keep []string
	for _, p := range uniq(ps) {
		if p == "" {
			continue
		}
		if !specVerifiersOne(p, want[p], off[p]) {
			keep = append(keep, p) // 못 고쳤다 — 다음 바퀴에 다시
		} else if len(want[p]) > 0 {
			keep = append(keep, p)
		}
	}
	if dry {
		return
	}
	body := strings.Join(keep, "\n")
	if body != "" {
		body += "\n"
	}
	if old, _ := readFile(sf); old != body {
		if err := writeAtomic(sf, body); err != nil {
			say("추측 검증: 관리 기록을 쓰지 못했다: %v", err)
		}
	}
}

// specVerifiersOne: 한 프로젝트의 원격 표식을 맞춘다. 맞췄으면(또는 이미 맞으면) true
func specVerifiersOne(p string, vs []verified.Verifier, preOff bool) bool {
	r := repoOf(p, LOCAL)
	if r == "" {
		say("추측 검증 %s: 설정에 %s repo 줄이 없다 — 표식을 못 고침", p, LOCAL)
		return false
	}
	ref := verified.VerifiersRef
	cur, rc := run("git", "-C", r, "ls-remote", "origin", ref)
	if rc != 0 {
		say("추측 검증 %s: 원격을 못 읽음 — 다음 바퀴에", p)
		return false
	}
	cur = firstField(cur)
	want := ""
	if len(vs) > 0 {
		et, rc := run("git", "-C", r, "hash-object", "-t", "tree", "-w", "/dev/null")
		if rc != 0 {
			return false
		}
		env := []string{"GIT_AUTHOR_NAME=hc-fill", "GIT_AUTHOR_EMAIL=hc-fill@localhost", "GIT_AUTHOR_DATE=@0 +0000",
			"GIT_COMMITTER_NAME=hc-fill", "GIT_COMMITTER_EMAIL=hc-fill@localhost", "GIT_COMMITTER_DATE=@0 +0000"}
		msg := verified.FormatVerifiers(vs)
		if preOff {
			msg += verified.PreOffLine + "\n"
		}
		o, rc := Cmd{Name: "git", Args: []string{"-C", r, "commit-tree", sub(et), "-m", msg}, Env: env}.Run()
		if rc != 0 {
			return false
		}
		want = sub(o)
	}
	if cur == want {
		return true
	}
	src := ""
	if want != "" {
		src = want
	}
	if dry {
		say("%s추측 검증 %s: git push origin %s:%s", P, p, src, ref)
		return true
	}
	if _, rc := run("git", "-C", r, "push", "-q", "--force-with-lease="+ref+":"+cur, "origin", src+":"+ref); rc != 0 {
		say("추측 검증 %s: 표식 push 실패 — 다음 바퀴에", p)
		return false
	}
	if want == "" {
		say("추측 검증 %s: 끔", p)
	} else {
		ms := make([]string, len(vs))
		for i, v := range vs {
			ms[i] = v.Machine
		}
		say("추측 검증 %s: 켬 → %s", p, strings.Join(ms, ","))
	}
	return true
}

func firstField(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}
