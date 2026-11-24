# hc-merger — 에이전트 안내 (`hc-merger --skill`)

## 하는 일

- 병합기 본체다. 레인의 `hc-lane land`가 병합기가 켜진 저장소에서는 큐(`refs/merge-queue/*`)에 제출만 하고, 병합기가 우선·제출 차례로 묶어 검증 한 번·push 한 번으로 기본 브랜치에 넣는다.
- 보통 `hc-lane merger …`로 부른다(인자 그대로 넘긴다). 저장소마다 병합기 기계에서 하나만 돈다. 띄우기·다시 띄우기는 hc-fill이 한다.
- 실패한 제출은 반송(`refs/merge-failed/<레인>`)하고 막힘·중간 사건을 남긴다. 일지는 `merger-journal.tsv`(상태 폴더 hc-lane 아래).

## 흔한 흐름

1. 레인이 제출: 레인 작업 트리에서 `hc-lane land main` → 「제출 … 이 레인은 여기서 끝」.
2. 병합기가 착지·반송하고 사건을 남긴다(착지는 병합기가 알린다 — 레인의 report 착지는 없다).
3. 반송(rebase 충돌)이면 hc-fill이 `<레인>-reland`를 넣거나 원래 레인에 다시 맞추라고 보낸다.
4. 병합기 터미널이 없으면: `hc-fill merger restart <프로젝트>`.

## 상태 보기

- `hc-merger --why 20` — 최근 착지 20번의 검증 길 요약 한 줄(건너뜀·직접 검증 까닭·착지 시간 중앙). `hc-lane merger --why 20`과 같다.
- `hc-fill status` · `hc-fill state --json` — 큐에 기다리는 제출, 병합기 살아 있음.
- `hc-lane stats --since 1d` — 제출 → 착지 시간. `hc-lane logs` — 병합기 로그의 실패 줄.
- 원격 큐: `git ls-remote origin 'refs/merge-queue/*'`.

## 조정과 되돌리기

- 바퀴 쉼은 `--tick <초>`, 한 바퀴만 `--once`(시험·손 확인용): `hc-merger --repo <병합기 전용 클론> --once`.
- 묶음 크기·검증 손잡이는 `hc-fill knob ls`의 병합기 줄(원격 손잡이 — 다음 바퀴에 먹는다). 바꾸면 `hc-fill knob set … --why …`, 되돌리기 `hc-fill knob unset …`.
- 제출 하나의 우선: `hc-fill prio <레인> 높음 --why …`(큐 메타를 함께 고친다).
- 큐에서 빼기·순서 바꾸기를 원격 참조로 손으로 하지 않는다 — 명령으로 안 되면 사람에게.

## 사건을 받으면

- `merger` 막힘(병합기 멈춤): `hc-lane logs`로 까닭을 본다. 클론이 rebase 중·더러움이면 무엇이 남았는지 보고 사람에게(남의 변경일 수 있다).
- `merger-<프로젝트>` 막힘(터미널 없음): `hc-fill merger restart <프로젝트>`. 곧 다시 죽으면 사람에게.
- 레인 반송: 반송 글의 까닭(충돌·검증 실패)을 레인 주인에게. 자동 다시 맞춤은 hc-fill이 한다.

## 사람에게 물을 것

- 병합기 클론을 지우거나 되돌리기(`reset --hard` 등), 큐 참조·반송 참조를 손으로 지우기.
- 기본 브랜치에 강제 push, 병합기를 두 기계에서 함께 돌리기.

## 하지 않을 것

- 찾아낸 병합기·착지 자식 프로세스를 죽이지 않는다 — 내가 띄운 정확한 pid만(일꾼은 착지 중이면 그 바퀴를 마친다).
- push 관문·기본 브랜치 CAS(lease)를 우회하지 않는다.
- 속 명령(`--round`·`--spec-verify`·`--spec-idle`)을 손으로 부르지 않는다 — 병합기가 스스로 부른다.

## 더 읽을 곳

- docs/hc-lane/merger.md · docs/hc-lane/merger-batch.md · docs/hc-lane/merger-spec-verify.md · docs/ops/events.md · docs/ops/agents.md
