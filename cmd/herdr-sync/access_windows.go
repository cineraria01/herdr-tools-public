package main

import "os"

// canExec: Windows에는 access(X_OK)가 없다 — 실행 비트로(빌드만 된다, gofix-ops C07)
func canExec(f string) bool {
	st, err := os.Stat(f)
	return err == nil && st.Mode().Perm()&0o111 != 0
}
