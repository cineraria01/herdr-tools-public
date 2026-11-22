# herdr-sync: 정본은 맥의 실제 파일, 두 저장소는 기록

- 정본은 맥의 `~/.local/bin`, `~/.claude/…`, `~/.agents/skills/…` 같은 실제 파일이다.
- 맥에서 고치고 `herdr-sync`를 돌린다.
  맥에서 바뀐 것은 그 줄의 저장소로 모아(**모음**) 저장소마다 따로 커밋·`HERDR_LAND=1` push하고, 모든 기계에 맞춘다.
- 레인이 어느 저장소를 고쳐 착지했으면, 다음 `herdr-sync`가 그것을 맥에 깔고(**깖**) 기계에 맞춘다.
- 무엇을 어디에 맞추는지는 개인 저장소의 `sync.list`가 정한다(꼴 `config.example/sync.list`).
  저장소 경로가 `tools:<경로>`면 이 범용 저장소, 그 밖은 개인 저장소 파일이다.
  새 파일을 맞추려면 한 줄만 더한다.
  설정(`~/.claude/settings.json`)은 개인 저장소 `settings/keys.json`에 적은 키만 맞춘다(꼴 `config.example/settings/keys.json`).
- **개인 값 검사**: 이 범용 저장소로 모을 맥 값이 `herdr-tools-config lint`(개인 저장소 `private-patterns.txt`)에 걸리면 `충돌 저장소 <경로> (개인 값)`을 찍고 아무 데도 쓰지 않고 멈춘다.
  `--take repo:`로도 넘기지 않는다.
  그 값을 범용으로 바꾸거나 그 줄을 개인 저장소로 옮긴다.
  개인 저장소에 `private-patterns.txt`가 없거나 패턴 줄이 없으면 검사를 못 하므로 범용으로 모을 값은 모두 `(개인 값 검사 못 함 — …)` 충돌로 멈춘다.
- 처음 한 번은 저장소에서 바로 돌린다: `go run ./cmd/herdr-sync`. 그러면 herdr-sync 자신도 `~/.local/bin`에 깔린다(`sync.list`에 그 바이너리 줄이 있으면).
