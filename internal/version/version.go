// Package version: 바이너리에 새긴 범용 저장소 커밋. herdr-sync Go 판이 릴리스 바이너리를 지을 때
// `-ldflags -X herdr-tools/internal/version.Commit=<커밋 12자>`로 넣는다(2026-11-21 사용자 「github 에다가 빌드 한것도 처리해서 … 받아서」).
// 기계에서 지은 바이너리는 비어 있다.
package version

// Commit: 지은 소스의 범용 저장소 커밋(12자) — 없으면 ""
var Commit string

// String: --version 꼴 「<도구> (Go)」, 커밋이 있으면 「<도구> (Go <커밋>)」
func String(tool string) string {
	if Commit == "" {
		return tool + " (Go)"
	}
	return tool + " (Go " + Commit + ")"
}
