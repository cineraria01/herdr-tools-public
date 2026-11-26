# 사람과 에이전트가 같은 명령으로 (--skill · --json · --dry-run)

- 2026-11-24 agent-skill-guides — 사용자 「수동으로 지시도 가능해야 하지만 허더 툴로, ai 에이전트가 사용하여 관리하고 도움이 될수 있는 툴로 진화를 원하는거야」.
- 따로 떨어진 자동 관리자는 없다. 사람이 손으로 부르는 명령과 지휘 에이전트가 부르는 명령이 **같다**. 에이전트용으로 더한 것은 셋뿐이고, 사람용 출력·인자·종료 코드는 그대로다.
  - `--skill`: 도구가 에이전트에게 주는 안내(하는 일·흔한 흐름·상태 보기·조정과 되돌리기·사건·사람에게 물을 것·하지 않을 것).
  - `--json`: 자주 읽는 명령의 기계가 읽는 꼴. 칸 이름은 아래에 고정이고, 시험이 코드와 맞춘다.
  - `--dry-run`: 바꾸는 명령이 무엇을 바꿀지만 보인다(「미리 보기:」 줄, 아무것도 안 씀).
- 흔한 실패(모르는 인자·없는 레인·잠금 겹침·닿지 못한 기계)에는 원래 오류 글 다음 줄에 `<도구>: 다음: …` 한 줄이 붙는다. 첫 줄과 종료 코드는 그대로다.
- 왜: 에이전트는 도움말과 오류 글을 읽고 도구를 쓴다. 없는 인자를 짐작해 되풀이하거나(한 레인이 `hc-lane events --name`을 20초마다 81번), 사람용 표를 눈으로 풀어 읽었다.

## 안내 (--skill)

- `hc-fill --skill` · `hc-lane --skill` · `hc-merger --skill` · `herdr-sync --skill` · `hc-sweep --skill` — 종료 0, 설정·기계 확인 없이 나온다.
- 안내는 `cmd/<도구>/skill.md` 한 파일이고 바이너리에 들어간다. 한 화면 남짓(90줄 아래), 깊은 것은 문서 경로로.
- 안내에 나온 하위 명령·인자가 실제로 등록·구현돼 있고, 등록된 하위 명령이 안내에 있다(아니면 「안내에서 뺌」 목록에 까닭) — `tests/repo/skill_test.go`가 본다. 새 하위 명령을 더하면 안내에도 한 줄.
- hc·화면 도구의 `--skill`과 같은 자리다.

## 에이전트에게 맡길 때 (본보기 지시)

```text
herdr-tools로 <할 일>을 해라. 먼저 쓸 도구의 `<도구> --skill`을 읽는다.
상태는 `hc-fill state --json`으로 보고, 바꾸는 명령은 `--dry-run`으로 먼저 본 뒤 `--why "<까닭>"`을 붙여 부른다.
되돌릴 수 없는 일·바깥 공개·권한이 걸리면 멈추고 묻는다. 찾아낸 프로세스는 죽이지 않는다.
보고에는 부른 명령과 `hc-fill journal --since 2h`의 되돌리는 명령을 적는다.
```

## --json이 있는 읽는 명령

| 명령 | 꼴 | 칸 |
|---|---|---|
| `hc-fill advise --json` | 한 줄 객체 | [지금 손댈 것](advise.md) |
| `hc-fill state --json` | 한 줄 객체 | [상태 한 장](state.md) |
| `hc-fill knob ls --json` | 배열 | [손잡이](knobs.md) |
| `hc-fill journal --json` | 줄마다 객체 | [운영 일지](journal.md) |
| `hc-fill ls --json` | 한 줄 객체 | 아래 |
| `hc-fill machine --json` | 한 줄 배열 | 아래 |
| `herdr-sync --log [<n>] --json` | 한 줄 배열 | 아래 |

### hc-fill ls --json

- `{"lanes":[…],"mergers":[…]}` — 사람용 표와 같은 사실. `mergers`는 표 끝 병합기 줄 글 그대로.
- 칸(lanes[]): `lane` · `project` · `state` · `code` · `address` · `owner` · `priority` · `after` · `only` · `last` · `note`
- `state`는 표의 상태 글(대기·떠 있음(working)·끝남·모름(연결 안 됨)·띄우기 거절 — …), `code`는 대기열 파일의 상태 값(wait·up·sent …), `after`·`only`는 목록(없으면 빈 목록), `last`는 마지막 보고(없으면 빈 글), `note`는 표의 비고에서 마지막 보고를 뺀 글.

### hc-fill machine --json

- 설정의 기계마다 한 줄. 모르는 값은 `null`(부하를 못 잼, 지난 바퀴 기록 없음).
- 칸: `machine` · `load1` · `cores` · `per_core` · `load_age_s` · `used` · `cap` · `free` · `resting` · `outside` · `adjust` · `why` · `unreach` · `line`
- `free`는 지난 바퀴 띄우기가 판정한 빈 자리(레인상한·멈춤 포함 — 상태 한 장 `machines[].free`와 같은 값, 옛 기록이면 `null`).
- `resting`·`outside`는 자리에 안 드는 쉬는 대기열 터미널·대기열 밖 에이전트 수(상태 한 장 `machines[].resting`·`outside`와 같은 값, 옛 기록이면 `null`).
- `adjust`는 실행 중 조정(손잡이 `seats@`·`pause@`), `why`는 지난 바퀴에 안 띄운 까닭, `unreach`는 끊김 비고, `line`은 사람용 줄 그대로.

### herdr-sync --log --json

- 배포 기록(`deploy.tsv`) 마지막 n줄(기본 20), 오래된 것부터. 기록이 없으면 `[]`.
- 칸: `when` · `kind` · `exit` · `tools_commit` · `source` · `config_commit` · `by` · `machines` · `raw`
- `exit`는 종료 코드(숫자가 아니면 `null`), `machines`는 기계 → `같음|씀|충돌|실패|건너뜀`, `raw`는 기계별 결과 칸 그대로.

### 아직 없는 것 (다음 차례)

- `hc-fill status --json` — 같은 사실은 `hc-fill state --json`의 레인 칸에 있다.
- `hc-merger --why --json`(병합기 일지 요약) — 같은 꾸러미를 고치는 레인(병합기 일지)이 끝난 뒤.
- `herdr-sync --status --json` — 기계마다 원격 조회가 섞여 있어 칸을 먼저 정해야 한다. 그때까지 `hc-fill state --json`의 배포 칸.
- `hc-lane events`·`hc-lane wait`는 이미 탭 구분 칸(docs/hc-lane/events.md)이라 그대로 둔다.

## --dry-run이 있는 바꾸는 명령

| 명령 | 보이는 것 |
|---|---|
| `hc-fill rm <레인> --dry-run` | 뺄 줄·상태·되돌리는 add 한 줄(주차 줄 거절은 실제와 같다) |
| `hc-fill prio <레인> <우선> --dry-run` | 전 → 후, 큐 메타도 바꿀지 |
| `hc-fill knob set <이름> <값> --why <까닭> --dry-run` · `knob unset <이름> --dry-run` | 바뀔 값과 지금 값(이름·범위·고정 검사는 실제와 같다) |
| `hc-fill knob save <이름> --dry-run` | 설정 파일에 정해 둘 값과 그 파일 경로(손잡이·파일 안 씀) |
| `hc-fill machine <기계> --seats <n>\|--pause\|--resume --dry-run` | 바뀔 손잡이(조정 기록도 안 남김) |
| `hc-fill run --dry-run` | 한 바퀴에 띄울 것(전부터 있음) |
| `hc-fill requeue <레인> --why <까닭> --dry-run` | 검증 실패 반송을 옮길 새 큐 이름·제출·감사 판정(검사는 실제와 같다, [다시 넣기](../hc-fill/requeue.md)) |
| `hc-lane --dry-run …` · `hc-lane clean --dry-run` · `hc-lane gc --dry-run` | 띄우기·정리 미리 보기(전부터 있음) |
| `herdr-sync --check` · `herdr-sync --at <커밋> --check` | 맞출 것·되돌릴 것(전부터 있음) |
| `hc-sweep --dry-run` | 닫을 것·지울 것(전부터 있음) |

- 미리 보기는 저장 파일·대기열·일지·조정 기록을 하나도 쓰지 않는다(시험이 전후 파일을 견준다).

### 아직 없는 것 (다음 차례)

- `hc-fill edit --dry-run` — 지시문 사본·선행 검사가 쓰기와 엮여 있어 나눠야 한다. 그때까지 `hc-fill ls --json`으로 전 값을 보고 일지의 되돌리는 명령을 쓴다.
- `hc-fill add --dry-run`·`hc-fill merger restart --dry-run`·`hc-fill note --dry-run`.
- `herdr-sync --auto on|off --dry-run`(표식 파일 하나라 되돌리기가 바로 된다).

## 막힌 줄·멈춘 레인을 그 자리에서 (2026-11-24 fill-edit-hands)

- 지휘가 손으로 하던 둘을 명령으로 — 사람도 같은 명령이다. 둘 다 일지에 남는다.
- `hc-fill edit <레인> --overlap-ok|--overlap-check · --early|--no-early` — 아직 안 뜬 줄의 겹침 검사·먼저 띄움 칸만(줄 ID 그대로). `rm` 뒤 `add`로 다시 넣지 않는다.
- `hc-fill nudge <레인> [<글>] --why <까닭>` — 레인 터미널이 일시 오류로 멈췄을 때(hc 상태 `error`·`done`, 완료·막힘 보고 없음) 이어 가라는 글 한 줄. 안 보내면 까닭과 종료 1(일하는 중·터미널 없음·이름 다름·보고 있음·손잡이 `nudge_every`분 안).
- `hc-fill tell [--audit] <레인> <글>` — 레인(또는 그 감사)에 글 보내기는 이것으로(2026-11-26 ht-fill-tell). 대기열이 기록한 지금 run 터미널 하나에만 보낸다 — `hc ls`에서 이름으로 고르면 같은 이름의 앞 run에 갈 수 있다. 없거나 끝났으면 보내지 않고 까닭과 종료 1.
- 자세히: [줄 고치기](../hc-fill/edit.md).

## 「다음:」 줄이 붙는 실패

| 실패 | 도구 | 다음 줄 |
|---|---|---|
| 모르는 인자·잘못 부름(사용법) | hc-fill | `hc-fill --skill`을 읽고 사용법의 인자만 |
| 모르는 인자(가까운 인자 없음) | hc-lane | `hc-lane --skill`·`hc-lane --help` (가까우면 전처럼 「대신 …?」) |
| 대기열에 없다(prio·edit·retry 등) | hc-fill | `hc-fill ls`로 레인 이름 |
| 대기열 잠금 30초 못 잡음(종료 75) | hc-fill | 잠시 뒤 같은 줄을 다시(잠금 파일은 지우지 않음) |
| watch가 이미 돎(종료 75) | hc-fill | 새로 띄우지 않고 `hc-fill state --json` |
| `--on <기계>` 원격 실행 실패 | hc-lane | `hc machine ls`로 연결을 보고 붙은 뒤 다시 |

- `hc-fill run`의 「다른 run이 돌고 있다」(종료 75)는 바퀴마다 나는 정상 건너뜀이라 줄을 더하지 않았다. `hc-lane land`의 종료 3·4·75 글은 전부터 「같은 줄을 다시」를 적는다.
