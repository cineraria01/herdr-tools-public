//go:build windows

package main

// Windows는 빌드만 된다(레인은 macOS에서만 돈다 — 운영 모델). 마스크·fd 물려주기가 없다

func umask() int { return 0 }

func closeInherited() {}
