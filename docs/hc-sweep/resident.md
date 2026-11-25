# hc-sweep: 상주 판 점검 (옛 판으로 도는 상주 찾기)

- 배포해도 옛 판으로 계속 도는 상주 프로세스를 찾아 알린다(2026-11-24 stale-resident-check — 지휘의 사건 감시 하나가 스스로 넘어가기가 없는 옛 판으로 하루 가까이 돌아, 그 사이 아홉 번 넘는 배포가 그 프로세스에 닿지 않았다. 실행 파일은 rename으로 갈려 옛 프로세스는 지워진 파일을 쥔 채 멀쩡히 돌고 `hc ls`로는 구분이 안 된다).
- **판 표시**: 상주(`selfexec.New`를 부르는 고리 — `hc-lane watch`·`hc-fill watch`·`herdr-balance --watch`·`hc-job-server`, 표는 `internal/resident`의 `Kinds`)는 뜰 때와 스스로 넘어간 뒤 `${XDG_STATE_HOME:-~/.local/state}/herdr-tools/resident/pids/<pid>.json`에 도구·pid·실행 파일 경로·판(커밋·크기·수정 시각)·hc 터미널·launchd 라벨·인자를 남긴다.
- **재기**: 건강 점검 길(바퀴마다, macOS)에서 `ps` 한 번으로 상주를 찾고 표시와 지금 깔린 실행 파일의 크기·수정 시각을 견준다. 열린 파일 조회(느린 기계에서 몇 분)는 쓰지 않는다. 표시가 없는 상주만 그 환경을 `ps -E`로 한 번 더 읽는다(다시 띄우는 명령에 쓸 터미널·라벨).
  - 같음 → 지나감. 다름 → 옛 판. 표시 없음(기능이 깔리기 전에 뜬 것·표시를 못 씀) → 판 모름 — 같음으로 읽지 않는다. pid가 같아도 프로세스가 뜨기 전에 쓰인 표시는 표시가 아니다(pid 재사용).
  - 유예: 새 판이 깔린 지도, 상주가 뜬 지도 `sweep_resident_grace`분(손잡이, 기본 5)이 지나야 알린다 — 그 안이면 다음 바퀴에 스스로 넘어갈 수 있다.
  - `ps`를 못 읽으면 모름: 사건을 내지 않고 앞 결과를 지우지 않는다(결과에 모름 까닭).
- **알림**: 운영 사건 `hc-stale-resident` 한 번 — `옛 판으로 도는 상주: <도구>(<터미널> pid <n>, 시작 <시각>) — 판 <판>|판 모름 — 표시 없음(…) · 깔린 판 <시각> — 다시 띄우기: <명령> (<기계>)`. 같은 프로세스·같은 깔린 판에는 되풀이하지 않는다(다시 배포했는데도 옛 판이면 한 번 더).
  - 다시 띄우는 명령: 사건 감시는 `hc kill <터미널> && hc-lane coord --watch <주인>`, 다른 hc 터미널은 `hc kill <터미널>`(지킴이·감독이 다시 띄운다), 터미널 없는 launchd 작업은 `launchctl kickstart -k gui/$(id -u)/<라벨>`, 둘 다 모르면 `kill <pid>` 뒤 같은 명령.
  - 도구는 끄지도 다시 띄우지도 않는다 — 지휘가 부른다.
- **보이는 곳**: 결과 한 장 `…/resident/report.json`을 `hc-fill state`의 「눈에 띄는 것」과 `herdr-sync --status`가 읽어 있을 때만 줄을 보인다(이 기계 것만 — 다른 기계는 그 기계의 사건으로 온다).
- 이 기계만 본다. 다른 기계의 상주는 그 기계 hc-sweep이 본다(원격으로 들여다보지 않는다). 배포 때 모든 기계의 수를 한눈에 보는 것은 [herdr-sync 배포 뒤 상주 확인](../herdr-sync/deploy.md)이다.
- 인자에 시험 임시 경로(`herdr-test-bin.*`·`herdr-verify-ram-*`·임시 폴더 바로 아래 `Test*`)가 든 프로세스는 상주로 세지 않는다 — 시험 고아는 [시험 고아 점검](test-orphans.md)이 따로 알린다(2026-11-26 ht-test-orphan-leak).
- 병합기 바깥 고리 `hc-lane merger`(`hc-merger --repo …`, `--once`·`--round` 빼고)도 표에 있다(2026-11-25 ht-resident-reexec). 표에 없는 것: `hc-sweep`·`hc-core-relaunch`(단발).
- 판 표시의 커밋이 새로 깔린 판과 같으면(실행 파일만 다시 쓰임) 상주가 다음 바퀴에 표시를 그 파일로 다시 쓴다([selfexec](../go/selfexec.md)) — 그때까지 유예 안이다. 새 상주가 `selfexec.New`를 부르면 표에도 한 줄(시험 `TestKindsCoverSelfexecCallers`가 본다).
- 시험: `internal/resident/resident_test.go`(옛 판·유예·판 모름·스스로 넘어감·단발 무시·launchd·ps 풀기·결과 줄), `internal/selfexec/mark_test.go`(뜰 때 표시), `cmd/hc-sweep/resident_test.go`(한 번만·ps 실패는 모름), `cmd/hc-fill/state_resident_test.go`, `cmd/herdr-sync/status_resident_test.go`.
