# Go 릴리스 바이너리

- **릴리스 바이너리**(2026-11-21 사용자 「각 컴퓨터 마다 빌드 하지말고 github 에다가 빌드 한것도 처리해서 차라리 각 컴퓨터에서는 받아서 하면 어때 ? <앱 저장소> 과 같은 방식으로」 — 앱 저장소 이름은 개인 값이라 가림): `herdr-sync`(`--check` 아님)가 공개 내보내기(herdr-tools-publish — 소스 검사)에 성공하면, 게시기가 검사·내보낸 바로 그 비공개 커밋(`HERDR_TOOLS_PUBLISH_RESULT`로 받는다)을 `git archive`로 꺼내 바이너리 줄의 도구를 맥에서 darwin/arm64로, 리눅스로 가는 도구(바이너리 줄의 대상이 `all`이거나 리눅스 기계 이름 — sync.list에서 고른다)는 linux/amd64로도(자산 이름 `<도구>-linux-amd64`, 같은 `SHA256SUMS`) 한 번 지어(-trimpath, CGO_ENABLED=0, GOTOOLCHAIN=local, 커밋 12자를 `-X herdr-tools/internal/version.Commit`로 — `--version`에 보임, 캐시 `~/.local/state/herdr-sync/go-build`, 도구마다 상한 `HERDR_TIMEOUT_BUILD` 600초) `publish.repo`(GitHub)의 릴리스 `bin-<커밋 12자>`에 도구별 바이너리와 `SHA256SUMS`를 `gh`로 올린다.
  검사한 커밋의 트리가 범용 저장소 HEAD(기계에 깐 소스)와 다르거나(그사이 origin/main이 앞섬) 게시기가 커밋을 알리지 않으면 그 실행은 올리지 않는다.
  이미 올린 자산은 덮지 않는다(`--clobber` 없음): 릴리스 조회가 「없음」이 아닌 오류(인증·통신·시간 넘김)거나 `SHA256SUMS`를 못 받으면 모름 — 올리지도 받지도 않고, `SHA256SUMS`가 이번 빌드와 다르면 경고하고 두며, 같으면 빠진 자산만 채운다.
  로그에는 `publish.repo` 주소 원문을 찍지 않는다.
  그다음 맥과 기계마다(리눅스는 그 도구들만) 맥이 계산한 sha256을 넘겨 위 「바이너리 줄」대로 깐다: 공개 릴리스를 토큰 없이 `curl`로 받아 해시가 같고 `uname -m`이 맞을 때만 그것, 아니면 그 기계 Go 소스 판(go.mod·cmd/**·internal/**의 숨김 아닌 일반 파일 전부의 cksum, go:embed 글 포함)이 같을 때만 그 기계에서 짓는다(GOOS·GOARCH·GOFLAGS는 비움).
  도구마다 응답 한 줄: 이미 이 판·받음·지음·못 깖(까닭, 옛 본체 그대로).
