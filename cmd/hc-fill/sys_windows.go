//go:build windows

package main

// closeInherited: Windows는 핸들을 기본으로 물려주지 않는다 — 할 일 없음(레인은 macOS에서만 돈다, 빌드만 깨지지 않게 — gofix-fill F15)
func closeInherited() {}
