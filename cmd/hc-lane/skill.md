# hc-lane — 에이전트 안내 (`hc-lane --skill`)

## 하는 일

- 커밋하는 레인을 자기 git 작업 트리에서 띄우고(`hc run`), 착지(`land`)·정리(`clean`·`gc`)한다.
- 레인과 지휘 사이의 사건(`report` → `events`·`wait`·`watch`), 자동 감사 판정 수거(`verdict`), 기본 브랜치 push 관문(`guard`), 병합기(`merger`)도 맡는다.
- 레인은 보통 `hc-fill`이 띄운다. 사람과 에이전트가 같은 명령을 쓴다.

## 흔한 흐름

1. 띄우기(대기열 밖에서 꼭 직접 띄울 때만): `hc-lane --name <레인> --repo <저장소> --brief <지시문> -- <실행 명령…>`
   - 조사·감사는 `--read`(작업 트리 없이), 미리 보기는 `--dry-run`, 착지·정리 절차는 `--print-steps`.
2. 레인 안에서 알리기: `hc-lane report 중간|완료|막힘 <sha|-> "<한 줄>"` — 완료는 `hc-lane report 완료 HEAD "<요약>"`.
3. 지휘가 기다리기: `hc-lane watch --to <지휘 주소>`(늘 돎) 또는 한 번 `hc-lane wait --lane <레인>`.
4. 감사 판정 받기(레인 안): `hc-lane verdict <판정 ID>` — 출력대로만 한다.
5. 착지(레인 작업 트리 안): `hc-lane land main` (감사 통과면 `hc-lane land main --verdict <ID>`).
6. 정리: `hc-lane clean --repo <저장소> --branch <브랜치>` · 오래 남은 것 `hc-lane gc --repo <저장소> --dry-run`부터.
7. 선행을 기다리는 먼저 띄운 레인: `hc-lane park "<한 줄>"` 뒤 clean.

## 상태 보기

- `hc-lane events --since 2h --lane <레인>` — 사건 줄(탭 구분 칸, docs/hc-lane/events.md). `--all`이면 연결된 기계 모두.
- `hc-lane stats --since 1d` — 착지 수·걸린 시간(정의 docs/hc-lane/stats.md).
- `hc-lane logs --all` — 도구 기록의 새 줄만(읽은 위치를 앞당긴다).
- `hc-lane coord` — 지금 지휘 주소. `hc-lane lease show --repo <저장소> --name <레인>` — 레인 임대.
- `hc-lane guard status --repo <저장소>` — push 관문(0 깔림 · 1 없음 · 3 다른 훅).
- 대기열·자리 쪽은 `hc-fill state --json`.

## 조정과 되돌리기

- 미리 보기: `hc-lane --dry-run --name <레인> --repo <저장소>`, `hc-lane clean --dry-run …`, `hc-lane gc --dry-run …`.
- 지운 가지는 `gc`가 찍은 「되돌리기 git branch <가지> <sha>」로 되살린다. `clean --archive`는 안 들어간 커밋을 원격 보관 참조에 남긴 뒤 지운다.
- 남은 임대 때문에 다시 못 띄우면(레인이 죽음): `hc-lane lease drop --repo <저장소> --name <레인>` — 그 레인이 정말 끝났을 때만.
- 레인 모두에게 한 마디: `hc-lane send --all "<글>"`(= `hc-lane broadcast "<글>"`). 원격 저장소 받기: `hc-lane fetch --repo <저장소>`.

## 사건을 받으면

- `완료`(자동 감사 없는 레인): 감사를 띄우거나 레인에 착지를 보낸다. 자동 감사 레인은 hc-fill이 한다.
- `막힘`: 글의 까닭을 읽는다. 착지 종료 3(rebase 충돌)은 레인이 양쪽을 살려 풀고 같은 land 줄을 다시, 4·75는 같은 줄을 다시.
- `뜻밖의 종료`: 그 레인 터미널(`hc read <주소>`)과 작업 트리를 보고, 다시 띄울지 정한다.
- `연결 안 됨 <기계>`: `hc machine ls`로 본다. 운영 사건 이름별 할 일: docs/ops/events.md

## 사람에게 물을 것

- 남의 변경을 버리는 것: `reset --hard`·`clean`·강제 push, 안 들어간 가지 지우기(`clean --archive` 포함 — 보관하더라도 사용자 일이면).
- 바깥 공개·배포, 승인 범위 밖의 저장소에 착지.
- 다른 훅이 있는 클론에 `hc-lane guard install`(도구가 멈춘 까닭이다).

## 하지 않을 것

- 찾아낸 프로세스를 죽이지 않는다 — 내가 띄운 정확한 pid만.
- push 관문을 끄거나(`hc-lane guard remove`) `HERDR_LAND=1`로 손 push하지 않는다 — 착지는 `hc-lane land`로만.
- 작업 폴더를 `rm -rf`로 지우지 않는다 — `hc-lane clean`.
- `hc-lane wait`를 배경(&)이나 출력 /dev/null로 돌리지 않는다(사건을 읽을 사람이 없다 — 종료 2).
- 없는 인자를 짐작해 되풀이하지 않는다(레인 이름 거름은 `--lane`). 인자 오류의 「대신 …?」·「다음:」 줄을 따른다.

## 더 읽을 곳

- docs/ops/agents.md · docs/hc-lane/ 아래(events·watch·land·launch·lease·gc·park·merger·stats·logs) · docs/ops/events.md
- 병합기: `hc-lane merger --repo <병합기 전용 클론>`(병합기 기계에서 하나만) — 안내는 `hc-merger --skill`
- 저장소 착지 스크립트(`.claude/herdr-land.sh`) 안에서 PR 머리 push 인자: `hc-lane land-prheads [<착지 후보>]` — 계약은 docs/hc-fill/pr.md 「착지 스크립트 계약」
- 사용법 전부: `hc-lane --help`
