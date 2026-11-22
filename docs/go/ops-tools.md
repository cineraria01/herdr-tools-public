# Go 운영 도구

- **운영 도구**(2026-11-21 사용자 「모두 고로 옮기고 싶어」 — ops-go): `herdr-sync`·`herdr-tools-config`·`herdr-tools-publish`·`herdr-settings-merge`는 `cmd/<도구>`다.
  상태 파일 base.tsv·repo-base.tsv·settings-base.json·bak. 계약 시험 `cmd/<도구>/contract*_test.go`(임시 HOME·가짜 hc·임시 저장소를 두고, 시험 프로세스에서 지은 바이너리(`internal/clitest` `Bin`)를 밖에서 exec).
  공통 꾸러미: `internal/toolcfg`(설정 읽기·개인 값 검사·sync-lint), `internal/ojson`(키 순서를 지키는 JSON — `jq -cS`·`jq .`과 같은 바이트라 옛 base의 cksum과 같다), `internal/cksum`(POSIX cksum).
  herdr-sync의 기계 쪽 조각은 sh다(`cmd/herdr-sync/agent.sh` 임베드 — 원격 기계에 Go가 없어도 된다).
  잠금은 flock. 개인 값 패턴은 `LC_ALL=C grep -E -i`처럼 바이트 단위로 읽는다(`\<`·`\>`는 낱말 경계) — Go가 못 읽는 ERE(역참조 `\1` 등)는 틀린 패턴(종료 2)이다.
