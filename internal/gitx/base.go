package gitx

import "strings"

// DefaultBase: 원격이 있는가와 기본 가지(셸 repo_defaults). origin이 있으면(get-url 0) refs/remotes/origin/HEAD가 가리키는 가지(없으면
// main), 없으면(2) 로컬 main(없고 master만 있으면 master). 조회가 실패하면 rc는 그 git 종료 코드(0 아님 — 부른 쪽이 모름·실패를 가린다).
// git은 g로 부른다 — 인자는 git 뒤(「-C <폴더>」·원격 실행은 부른 쪽이 붙인다), quiet면 출력을 모두 버리고 종료 코드만 본다
func DefaultBase(g func(quiet bool, args ...string) (string, int)) (remote bool, base string, rc int) {
	switch _, rc = g(true, "remote", "get-url", "origin"); rc {
	case 0:
		refs, rc := g(false, "for-each-ref", "--format=%(symref)", "refs/remotes/origin/HEAD")
		if rc != 0 {
			return false, "", rc
		}
		if base = strings.TrimPrefix(refs, "refs/remotes/origin/"); base == "" {
			base = "main"
		}
		return true, base, 0
	case 2:
		refs, rc := g(false, "for-each-ref", "--format=%(refname)", "refs/heads/main", "refs/heads/master")
		if rc != 0 {
			return false, "", rc
		}
		l := "\n" + refs + "\n"
		if !strings.Contains(l, "\nrefs/heads/main\n") && strings.Contains(l, "\nrefs/heads/master\n") {
			return false, "master", 0
		}
		return false, "main", 0
	}
	return false, "", rc
}
