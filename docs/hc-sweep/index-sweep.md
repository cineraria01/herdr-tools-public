# hc-sweep: 색인 쓸기

- **색인 쓸기**: `hc-sweep`은 매 바퀴 먼저(hc ls 실패와 상관없이) codebase-memory 색인 폴더(`${XDG_CACHE_HOME:-~/.cache}/codebase-memory-mcp`)의 `<이름>.db`(`_config.db` 빼고) 가운데 projects 표의 root_path가 모두 지워진 것을 `-wal`·`-shm`·`-journal`과 함께 지우고 closed.log에 `색인:<이름>`·root·크기를 남긴다.
  `--min-age`분 안(색인 중)·못 읽음·root가 `/Volumes/` 아래(링크를 풀어서도)·root 읽기 오류면 남긴다.
  지운 것이 없으면 요약 줄도 없다.
  teardown이 돌지 않은 워크트리(옛 워크트리·손으로 지움·rollback)의 색인이 기계마다 수십 GB 쌓였다(2026-11-18).
