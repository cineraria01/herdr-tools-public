//go:build windows

package main

// closeInherited: Windows는 빌드만 된다(병합기를 돌리지 않는다)
func closeInherited() {}
