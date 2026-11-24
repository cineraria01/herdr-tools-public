package proc

// Busy: 이 실행이 띄워 기다리는 무리(Run·Start·Wait)가 있으면 참 — 그 사이 제자리 exec하면 그 자식이 고아로 남는다(selfexec)
func Busy() bool {
	curMu.Lock()
	defer curMu.Unlock()
	return len(bounded) > 0
}
