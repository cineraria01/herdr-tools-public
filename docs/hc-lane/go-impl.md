# hc-lane: Go 구현

- **hc-lane 구현**(2026-11-21 사용자 「모두 고로 옮기고 싶어」, hc-lane-go-start): `cmd/hc-lane`(한 바이너리).
  하위 명령은 `cmd/hc-lane/<명령>.go`의 `init()`에서 `register`로 더한다: 레인 띄우기(`--read`·`--dry-run`·`--print-steps`·`--brief`·`--resume`, 시작 확인·첫 턴 감시·임대 잡기)·`clean`·`gc`·`park`, `land`·제출·`guard`·`lease`·`fetch`, `report`·`events`·`wait`·`watch`·`coord`·`send`·`broadcast`·`early-wait`·`verdict`, `merger`(곁 `hc-merger`로 넘김).
  대상 기계 쪽 판정(`--on`)은 `sh -c` 조각을 `hc exec`로 보낸다.
  도움말 글은 `cmd/hc-lane/usage.txt`. 계약 시험 `tests/hclane/`.
  - **시간 상한·모름**(2026-11-21 gofix-lane — Go 전환 감사): hc-lane이 부르는 git·hc는 모두 `proc.Limit` 상한 안에서만 기다린다(띄우기·clean·gc·park 도우미와 wait·watch 바퀴의 `hc ls`·`hc machine ls`도 — `hc exec --on`은 90초).
    넘으면 결과는 「모름」: 상태를 바꾸지 않고, 지우기·키 보내기 판정은 멈추는 쪽, `land`는 그 뒤 실패를 1이 아니라 75(다시)로 끝낸다.
    작업 트리를 바꾸는 `rebase`·`merge --ff-only`만 상한 없이(같은 무리에서 — 중간에 끊으면 반쯤 바뀐 작업 트리가 남는다).
    잘린 `hc ls`·`hc get` 출력(0으로 끝났어도)은 「없음·끝남」이 아니라 모름이다.
    그 밖에: 잘린 목록·gc reflog 실패·해석 못 하는 `gc.keep_branches` 무늬·`--min-age` 넘침은 멈춤, 실패한 `hc read` 화면으로는 키를 안 보냄, 막힘 줄의 비밀 꼴 값(주소·`키=값`·24자 넘는 토큰 꼴)은 가림, `hc run` 출력에 주소가 없으면 종료 5, 제출 전 사전 검사는 제출 커밋 속 판만, 옛 판 주차(`lane/<레인>` + 같은 SHA의 `주차` 사건)는 clean이 「주차됨」, `--resume --dry-run`도 원격 주차 가지를 읽음, watch 잠금 주인을 `ps`로 못 보면 넘겨받지 않음(75).
    시험 `tests/hclane/gofix_test.go`·`gofix2_test.go`.
