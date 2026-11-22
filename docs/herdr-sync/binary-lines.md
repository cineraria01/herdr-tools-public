# herdr-sync: 바이너리 줄

- **바이너리 줄** `<묶음> .local/bin/<도구> tools:cmd/<도구> <대상>`: herdr-sync가 맥에서 지어 `publish.repo` 릴리스(`bin-<커밋 12자>`)에 올리고, 대상 기계는 sha256을 확인해 받아 같은 폴더 임시 파일 → `mv` 한 번으로 `~/.local/bin/<도구>`를 바꾼다.
  못 받으면 그 기계 `~/.local/lib/herdr-go`(Go 소스 나무 줄 셋 `go.mod`·`cmd/**`·`internal/**`)의 소스 판이 범용 저장소와 같을 때만 그 기계에서 `go build`. 둘 다 안 되면 그 기계 줄은 `다름 (바이너리)`, 크게 경고하고 그 기계의 설정·지움을 미룬다.
  기록 `~/.local/state/herdr-sync/bin.tsv`(도구·sha256·소스 판)는 「이미 이 판인가」에만 쓴다.
  `--check`는 `다름:바이너리 <도구> (까닭)`. 처음 부를 때 짓는 길은 없다.
