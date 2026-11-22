# herdr-sync: 맞추지 않는 것 (기계마다 다른 것)

- 로그인·계정·토큰 파일과 `~/.config/*`
- `~/.claude/rules/<기계>.md` 같은 기계 이름 규칙(맞출 것만 `sync.list`에 적는다)
- `~/.claude/CLAUDE.md`의 `<!-- User customizations -->` 줄까지(그 기계의 머리 블록)
- `~/.codex/AGENTS.md`의 codex-shared 표식 밖
- 셸 rc, launchd 작업(`launchd/`는 손으로 깐다)
- herdr가 관리하는 `~/.claude/hooks/herdr-agent-state.sh`
- settings의 `keys.json` `refs` 키(`--check`에서 「참고」로만 보인다)
- 리눅스 기계는 `all` 묶음만 받는다(도구가 macOS 명령을 전제로 한다).
- 맥에서 지운 파일은 기계에서 저절로 지우지 않는다.
  지울 것은 `sync.list`에 `지움` 줄로 더한다.
