//go:build windows

package main

import "errors"

// Windows는 빌드만 된다(레인은 macOS에서만 돈다 — 운영 모델). 마스크·fd 물려주기·exec·자손 거두기가 없다

func umask() int { return 0 }

func execve(bin string, argv, env []string) error { return errors.New("Windows는 exec을 못 한다") }

func killTree(root int) {}
