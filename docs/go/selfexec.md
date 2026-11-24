# 새 판으로 이어 돌기 (selfexec)

- **오래 도는 고리는 새로 깔린 판으로 스스로 넘어간다**(2026-11-24 reexec-longrun — 배포해도 `herdr-balance --watch`·`hc-job-server`가 손으로 다시 띄울 때까지 옛 판으로 돌았다).
  `internal/selfexec`: 뜰 때 잡은 자기 실행 파일(`os.Executable`)의 stat이 바뀌고 그 바뀐 stat이 두 바퀴 연속 같으면(반쯤 쓴 판은 안 탐) 같은 경로·인자(`os.Args`)·환경으로 exec한다.
  pid·작업 폴더·표준 입출력이 그대로라 launchd(KeepAlive는 같은 pid라 다시 띄우지 않음)·`hc-launchd-guard`·hc 터미널 어느 쪽이 띄웠든 그 자리에서 이어 돈다.
  부르는 자리는 바퀴 사이(이번 바퀴를 마치고 다음 바퀴 전)뿐이다 — 쥔 바퀴 잠금이 없다.
  수명 내내 쥔 `lockx` 잠금은 O_CLOEXEC라 exec 때 풀리고 새 판이 다시 쥔다.
  이 실행이 띄워 기다리는 자식(`proc.Start`·`Run`·`Wait` — `proc.Busy`)이 있으면 하지 않고 다음 바퀴에 본다(exec하면 그 자식이 고아로 남는다).
  메모리 상태는 새 판에서 처음부터다.
- **기록 한 줄**: 그 도구의 기록 꼴로 `넘겨줌 <경로> 새 판이 깔렸다(옛 판 <커밋> <수정 시각> → 새 판 <수정 시각>) — 새 판으로 이어 돈다`.
  exec가 실패하면 `새 판으로 exec 실패(<까닭>) — 이 판으로 계속` 한 줄 뒤 이 판으로 계속 돌고, 같은 판으로 다시 시도하지 않는다(그 뒤 또 새 판이면 다시 한다).
  실행 파일을 못 읽으면(까는 중·지움) 아무것도 안 한다.
  Windows는 빌드만 된다(이 판으로 계속).
- **고리 표**(누가 새 판을 타는가):

  | 고리 | 새 판 | 어떻게 |
  |---|---|---|
  | `hc-lane watch` | 탐 | 바퀴 첫머리 `selfexec`(watch.log·표준 오류) |
  | `herdr-balance --watch` | 탐 | 2초 회차 사이 `selfexec`(표준 출력, 시각 머리) |
  | `hc-job-server` | 탐 | 틱 사이 `selfexec`(표준 출력, 시각·이름 머리) |
  | `hc-fill watch` | 탐 | 바퀴 `run`은 늘 새로 띄우고, 고리 자신은 바퀴 사이 `selfexec`(표준 출력) |
  | `hc-lane merger`(병합기 상주) | 탐(그대로 둠) | 바퀴마다 일꾼 `hc-merger --round`를 새 경로로 찾아 띄운다 — 고리 자신은 얇은 쉼·띄우기뿐 |
  | `hc-sweep` | 해당 없음 | launchd 주기 실행(단발) |
  | `hc-core-relaunch`의 기다림 | 해당 없음 | 전체 상한 안의 한 번짜리 기다림 |
- 시험 `internal/selfexec/selfexec_test.go`(가짜 바이너리 둘로 실제 exec — 바뀜·안 바뀜·자식 도는 동안 미룸·exec 실패 뒤 계속·인자·환경·작업 폴더 유지).
