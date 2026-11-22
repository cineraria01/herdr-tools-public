package main

import "os/exec"

// supervise: Windows는 레인을 돌리지 않는다 — 빌드만 된다. 무리가 없어 직접 자식만(CommandContext가 끈다)
func supervise(*exec.Cmd) (func(), error) { return func() {}, nil }
