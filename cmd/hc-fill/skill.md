# hc-fill — 에이전트 안내 (`hc-fill --skill`)

## 하는 일

- 레인 대기열이다. 지휘가 넣은 레인을, 자리가 나면 1분 안에 자리 남는 기계에 `hc-lane`으로 띄운다(바퀴 = `hc-fill run`, 늘 돌기 = `hc-fill watch`).
- 쓰기 레인의 자동 감사(완료 보고 → 감사 → 판정 → 착지), 받은편지, 병합기 살아 있음, 기계 부하·자리 조정, 손잡이·운영 일지도 맡는다.
- 사람과 에이전트가 같은 명령을 쓴다. 정책(문턱·차례)은 손잡이로 바꾸고, 판단은 지휘가 한다.

## 흔한 흐름

1. 넣기: `hc-fill add --name <레인> --project <프로젝트> --brief <지시문> [--after <선행>] [--priority 높음]`
   - 같은 일의 다음 단계는 새 레인 대신 `hc-fill add --name <이름> --project <프로젝트> --brief <지시문> --then <선행>`.
2. 지켜보기: `hc-lane watch`가 사건을 지휘에 보낸다. 상태는 아래 「상태 보기」.
3. 막힘 풀기: 띄우기 거절은 까닭을 고친 뒤 `hc-fill retry <레인>`, 선행·기계·지시문 바꾸기는 `hc-fill edit <레인> --after <선행>|--only <기계>|--brief <지시문>`.
   - 아직 안 뜬 줄의 겹침 검사·먼저 띄움: `hc-fill edit <레인> --overlap-ok|--overlap-check` · `--early|--no-early` (rm 뒤 add로 다시 넣지 않는다).
   - 레인 터미널이 일시 오류로 멈춤(hc 상태 error, 보고 없음): `hc-fill nudge <레인> --why <까닭>` — 이어 가라는 글 한 줄(한 레인에 손잡이 `nudge_every`분 안에 한 번).
   - 병합기가 제출 탓이 아닌 검증 실패로 돌려보냄(시험 환경·흔들림): `hc-fill requeue <레인> --why <까닭> [--dry-run]` — 같은 커밋·같은 감사 판정으로 줄 끝에(충돌 반송은 거절).
   - 줄에 선 내 레인 제출이 진짜로 깨질 것을 앎(먼저 든 변경과 뜻이 부딪힘 등): `hc-fill withdraw <레인> --why <까닭> [--dry-run]` — 병합기 반송과 같은 꼴로 빼서 뒤 제출이 기다리지 않게, 레인은 반송 다시 띄움으로 고쳐 다시 낸다(남의 레인은 `--force`, docs/hc-fill/withdraw.md).
   - 레인이 PR 사람 리뷰를 「반영 안 함」으로 보고(손잡이 `pr_human`): 맞으면 `hc-fill human-ok <레인> --why <까닭>` — 막은 착지를 푼다(docs/hc-fill/pr.md 「사람 리뷰」).
4. 착지: 레인이 스스로 한다(`hc-lane land`). 자동 감사가 붙은 레인은 감사 통과 뒤에.
5. 레인에 말 더하기: `hc-fill note <레인> <글|파일>`(다음 감사 지시문에도 붙는다).
   - 레인에 글 보내기는 이것으로: `hc-fill tell <레인> <글>`(감사 터미널은 `--audit`) — 대기열이 아는 지금 run 터미널 하나에만. `hc ls`에서 이름으로 골라 `hc send`하지 않는다(같은 이름의 앞 run이 남아 있다).

## 상태 보기

- `hc-fill advise --json` — 지금 손댈 것(놀고 있는 자리에 풀 수 있는 대기·한 줄 사슬·멈춘 레인·굶는 감사·밀린 배포 …)을 근거·다음 명령·확신과 함께. 먼저 이것부터. 칸: docs/ops/advise.md
- `hc-fill state --json` — 한 장에 기계·대기열·레인·병합·배포·손잡이·사건. 지문(fingerprint)이 같으면 다시 볼 필요 없다. 기본은 움직이는 것만(숨긴 수 hidden), 전체는 `--all`. 칸: docs/ops/state.md
- `hc-fill ls --json` — 대기열 줄마다(칸: docs/ops/agents.md). 사람용 표는 `hc-fill ls`.
- `hc-fill status` — 레인마다 단계·기계·예상, 끝에 기계 줄과 병합기.
- `hc-fill machine --json` — 기계마다 부하·자리·조정·안 띄운 까닭.
- `hc-fill journal --json --since 2h` — 누가 무엇을 왜 바꿨는지(되돌리는 명령 포함).
- `hc-fill knob ls --json` — 손잡이 지금 값·출처.
- 한 바퀴 미리 보기: `hc-fill run --dry-run`(아무것도 안 띄움).

## 조정과 되돌리기

- 바꾸는 명령은 먼저 `--dry-run`으로 무엇이 바뀔지 본다: `hc-fill rm <레인> --dry-run`, `hc-fill prio <레인> <우선> --dry-run`, `hc-fill knob set <이름> <값> --why <까닭> --dry-run`, `hc-fill machine <기계> --pause --why <까닭> --dry-run`.
- 우선순위: `hc-fill prio <레인> 긴급|높음|보통|낮음 --why <까닭>`
- 손잡이: `hc-fill knob set <이름> <값> --why <까닭> [--for <분>]` · 되돌리기 `hc-fill knob unset <이름>` — 잠깐 바꾼 값. 정해 둘 값이면 `hc-fill knob save <이름> --why <까닭>`(개인 설정 knobs.tsv에 쓰고 손잡이는 지움, 커밋은 직접 — docs/ops/knobs.md 「설정 파일」)
- 기계: `hc-fill machine <기계> --seats <n> --why <까닭> [--for <분>]` · `--pause` · 되돌리기 `--resume` 또는 `--seats -`
- 기계 이름 바꾸기: hc가 새 이름을 보인 뒤 `hc-fill machine --rename <옛> <새> --dry-run` → `--why <까닭>`으로 실행 → 출력 「다음:」대로 `hc-fill merger restart <프로젝트>` → 설정 파일 커밋(이 명령은 커밋 안 함). 되돌리기 `hc-fill machine --rename <새> <옛>`
- 빼기: `hc-fill rm <레인> --why <까닭>` (주차 중이면 `--force`가 있어야 — 준비 커밋을 버린다). 되돌리는 add 한 줄이 일지에 남는다.
- 병합기 다시 띄우기: `hc-fill merger restart <프로젝트>`
- PR 거울(레인마다 PR 한 장 — 대기열이 원본, PR은 거울): `hc-fill knob set pr <프로젝트> --why <까닭>` · 끄기 `hc-fill knob unset pr`. PR에서 머지하지 않는다(병합기가 넣으면 Merged). docs/hc-fill/pr.md
- 조치마다 일지에 「되돌리는 명령」이 남는다 — `hc-fill journal`에서 그대로 부른다.

## 사건을 받으면

- `hc-fill-idle`(자리가 빔): `hc-fill ls`로 보고, 사용자 요청 범위 안의 남은 일만 `hc-fill add`. 없으면 그대로 둔다.
- `hc-fill-skew`(한 기계에 몰림): `hc-fill machine`을 보고 `--seats`·`--pause`나 `hc-fill edit <레인> --only <기계>`.
- `merger-<프로젝트>`(병합기 없음): `hc-fill status` → `hc-fill merger restart <프로젝트>`.
- `hc-fill-unreach`(기계 끊김): `hc machine ls`로 보고, 오래면 그 레인을 다른 기계로 다시 띄울지 정한다.
- `hc-fill-advise`(손댈 것이 생김): `hc-fill advise`로 보고 확신 `확실`은 다음 명령을 그대로, `판단 필요`는 근거를 읽고 정한다.
- `hc-fill-pr`(PR 거울이 못 씀): `gh auth status`를 보고, 못 고치면 `hc-fill knob set pr - --why <까닭>`. 대기열·병합은 그대로 돈다.
- `hc-fill-agent`(운영 에이전트 끝·사람이 정한다): `hc-fill agent status`와 `hc-fill journal --since 2h`로 한 조치·되돌리기를 본다. 운영 에이전트는 손잡이 `ops_agent`(끔|판단만|조치)로 켜고, `hc-fill agent run`이 지금 한 바퀴(docs/ops/agent.md).
- 레인 `막힘`: 글의 까닭대로 — 띄우기 거절이면 고치고 `hc-fill retry <레인>`.
- 사건마다 뜻·흔한 까닭·다음 명령: docs/ops/events.md

## 사람에게 물을 것

- 되돌릴 수 없는 것: 주차한 줄을 `hc-fill rm <레인> --force`로 버리기, 고정(`--pin`)된 손잡이를 `--force`로 바꾸기(고정한 사람의 결정이다).
- 바깥 공개·배포·권한: 사용자 승인 범위 밖이면 묻는다.
- 사용자 요청 범위 밖의 일을 대기열에 넣는 것.

## 하지 않을 것

- 찾아낸 프로세스를 죽이지 않는다 — 내가 띄운 정확한 pid만.
- 안전 관문(겹침·자동 감사·push 관문)을 끄거나 우회하지 않는다. 손잡이는 `hc-fill knob ls`에 있는 것뿐이다.
- 대기열 파일(`queue.tsv`)·손잡이 저장 파일을 손으로 고치지 않는다 — 명령으로.
- 같은 명령을 짐작한 인자로 되풀이하지 않는다. 인자 오류가 나면 `hc-fill --skill`과 사용법을 읽는다.

## 더 읽을 곳

- docs/ops/agents.md(사람·에이전트가 같은 명령 · `--json` 칸 · `--dry-run` 목록) · docs/ops/advise.md · docs/ops/state.md · docs/ops/events.md · docs/ops/knobs.md · docs/ops/journal.md
- docs/hc-fill/ 아래 기능별 문서(부하 고르기·놀고 있음·착지 뒤 명령·닿지 않음)
- 사용법 전부: `hc-fill` (인자 없이 — 종료 2와 함께 사용법)
