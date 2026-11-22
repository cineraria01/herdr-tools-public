# 시험

시험은 모두 Go다(셸 시험 `*.test.sh`는 두지 않는다 — `tests/repo`의 `TestNoShellTests`가 막는다).
계약 시험은 시험 프로세스에서 지은 바이너리(`internal/clitest` `Bin`·`BinDir` — 도구마다 `go build` 한 번)를 가짜 hc·ssh·git(PATH 앞)과 임시 HOME·임시 저장소로 부른다.
실제 기계·레인에 닿지 않는다.

```sh
bash .claude/herdr-verify.sh        # 저장소 검증(hc-lane land가 부른다): go vet ./... → go test ./tests/repo/(싼 검사) → go test -race ./..., 단계마다 고아 검사
bash .claude/herdr-verify.sh --cheap-only   # 싼 검사까지(.claude/herdr-precheck.sh — 병합기 제출 직전)
go test -race ./...                 # 계약 시험 전부
go test ./tests/repo/               # 싼 검사: 줄 수 예산·싼 검사 파일 빠짐·gofmt·셸 문법(남은 .sh)·셸 시험 없음·개인 값(lint --strict)·비밀 꼴
go test ./tests/hclane/ ./tests/merger/ ./tests/hcfill/ ./tests/util/ ./claude/hooks/tests/ ./cmd/...   # 도구별(hc-lane·병합기·hc-fill·작은 도구·훅·운영 도구)
```
