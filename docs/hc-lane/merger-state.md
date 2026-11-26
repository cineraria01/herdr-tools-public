# 병합기 상태 한 장 (merger-state)

2026-11-26 사용자 결정 「구조를 다시 짜기」(ht-merger-state-record). hc-fill이 병합기 상태를 **밖에서 짐작**하던 것(터미널 이름 정규식, 일지 마지막 줄 시각, specwait·child 직접 읽기, 일지 「착지 실행」 줄로 착지 기계 짐작)을 다섯 번 고쳤다. 이제 **병합기가 자기 상태를 한 장에 쓰고 hc-fill은 그 한 장만 읽는다**.

## 기록 — 병합기가 쓴다
- 자리: 병합기 클론의 `<git-path hc-merger>/state.json`(specwait·specshard·child가 있는 폴더). 임시 파일 → rename(반쯤 쓴 판 없음). 꼴과 읽기·쓰기는 `internal/mergerstate` 한 곳.
- 언제: 바퀴마다(끝에서 한 번), 단계가 바뀔 때, 착지 중(병합기 기계·워커)에는 30초마다(바퀴 일꾼이 착지를 기다리며 막혀 있어도). 쓰기 실패는 경고만 — 차례·검증·착지 동작은 바꾸지 않는다.
- 칸:

| 칸 | 뜻 |
|---|---|
| `v` | 판 번호(지금 1). 읽는 쪽이 모르는 판이면 「모름(기록 판 n)」 |
| `at` | 기록 시각(초) |
| `progress_at` | **마지막 진행 시각** — 아래 「진행」 |
| `machine`·`term`·`base` | 병합기 기계, 병합기 터미널(`HC_TERMINAL_ID`), 기본 브랜치 |
| `phase`·`phase_since`·`why` | 단계(아래 목록), 그 단계 시작, 사람이 읽는 한 줄 까닭 |
| `front`·`members` | 맨 앞 제출 SHA, 지금 도는 묶음의 구성원 레인(차례대로) |
| `queue` | 병합기가 본 줄 차례(레인, 우선 반영) — 길이 = 큐 수 |
| `waits` | 기다리는 대상마다 `machine`·`addr`(터미널 **주소**, 못 찾았으면 빈 값)·`role`(추측·몫 i/K·마무리·착지 실행)·`at`(띄운 시각) |
| `inner` | 착지 안쪽 단계 `line`(착지 스크립트가 쓴 한 줄)·`at`(병합기가 그 줄을 처음 본 때). 안 쓰면 없음 = 「안쪽 단계 모름」 |
| `land_on` | 마지막 착지 실행 결정 「<기계>(<까닭>)」(이력 — status 병합기 줄) |
| `probe_at` | 기다리는 일꾼의 진행을 마지막으로 잰 때(병합기 안 — 바퀴마다 원격을 재지 않게 60초에 한 번) |

- 단계: `idle`(줄 빔·할 일 없음) · `round`(그 밖 바퀴 일 — 큐 읽기·풀이·미리 훑기·일시 실패로 쉼, `why`에 까닭) · `spec-wait`(맨 앞 추측 검증 기다림 — 추측·나눔 몫 i/K·마무리는 `waits`의 역할로) · `load-wait`(그 추측 일꾼이 검증 기계 부하가 기준 아래로 내려가길 기다림 — 정상 대기, `why`에 기계·잰 부하·기준. 지휘 전달 2026-11-26 22:4x 사례) · `land`(병합기 기계에서 착지: 준비 훅·`hc-lane land` 또는 착지 스크립트) · `land-on`(워커에서 착지 실행) · `prev-land`(앞 병합기 실행이 남긴 착지 자식이 아직 돎 — 다시 띄운 직후).
- **진행**(`progress_at`을 지금으로): 단계·맨 앞이 바뀜, 바퀴가 일을 함(착지·반송·풀이 — 바퀴 종료 76), 일꾼을 띄움, 기다리는 일꾼이 진행 중(그 기계에서 로그·작업 트리·부하 대기 표시·안쪽 단계 파일이 `merger_spec_fresh`분 안에 갱신 — `verified.FreshSh`, 나눔이면 몫 하나라도), 병합기 기계 착지 자식이 살아 있음, 안쪽 단계 줄이 바뀜. **진행 아님**: 같은 단계로 바퀴만 돎(병합기는 몇 초마다 바퀴를 돈다), 기록만 다시 씀. 사람에게는 「마지막 진행 N분 전」으로 보인다(옛 글 「마지막 바퀴」는 뜻이 틀려 바꿨다 — json 칸 이름 `last_round_age_s`만 그대로). 다시 띄운 병합기는 앞 기록의 `progress_at`·`phase_since`·`waits`·`inner`를 같은 단계·같은 맨 앞이면 이어 쓴다(`spec-wait`↔`load-wait`는 같은 기다림).
- 안쪽 단계 계약(착지와 추측 검증 같은 길): 병합기는 착지(`hc-lane land`·`.claude/herdr-land.sh`)와 추측 일꾼의 검증 스크립트에 `HERDR_LAND_PHASE=<파일>`을 준다. 스크립트는 단계가 바뀔 때 그 파일을 한 줄로 **덮어쓴다**(예 `공개 선점 기다림 — 쥔 쪽 ln-x(box-2)`·`부하 기다림 — 코어당 5.1 · 기준 4`). 안 쓰면 「안쪽 단계 모름」. 병합기 기계 착지는 `<기록 폴더>/land-phase`, 워커 착지는 `<클론>.land-wt/<차례 12>.phase`, 추측 일꾼은 `<클론>.spec-wt/<아이디 8>[-s<i>].phase`(진행을 잴 때 한 번의 `hc exec`로 함께 읽음). 추측 일꾼 자신의 검증 자리 기다림(`busy_wait`)은 일꾼이 `<…>.wait`에 쓰고 병합기가 `load-wait`로 적는다 — 새 판정 층 없이 같은 기록 칸.

## 읽기 — hc-fill은 읽기만
- 읽는 명령 하나: `hc-merger --state <클론>` — 그 클론의 기록을 그대로 찍는다(종료 0), 기록 없음 3, 못 읽음 1. hc-fill run 바퀴(병합기 살아 있음, `mergeralive.go`)가 살아 있는 병합기 터미널의 폴더로 `onm`(이 기계면 `sh`, 다른 기계면 `hc exec --on` — 같은 길) 한 번 불러 **merger-alive.tsv 그 줄**에 담는다(13칸 기록, 14칸 못 읽은 까닭). 병합기 살아 있음 기록과 하나다.
- state·advise·status는 그 줄만 읽는다. 기다리는 일꾼이 살아 있는지는 기록의 **주소**를 이번 `hc ls`에서 찾는다. 「마지막 바퀴 N분 전」 = 지금 − `progress_at`.
- 옛 판 병합기(기록 없음)·옛 판 `hc-merger`(`--state` 모름, 종료 2): 짐작하지 않고 「모름(옛 판 병합기 — 다시 띄우면 기록)」, merge-stall을 정하지 않는다. 모르는 판 번호도 모름.
- 병합기가 죽었는지는 밖에서만 — 지금 그대로 병합기 살아 있음(터미널 없음 `merger_dead`분)이 낸다. 기록이 낡음(`at`)은 근거에 「기록 n분 전」으로 보인다.
- 일지(`merger-journal.tsv`)는 이력 — 최근 착지·건너뜀·맡김 수·검증 시간(통계)만 읽고 지금 상태 판정에는 안 쓴다. 일지 꼴은 그대로.
- 줄 차례(status·state의 「병합기 줄 n번째」)는 지금처럼 run 바퀴가 읽은 원격 큐(사실)다 — 병합기가 죽어도 보여야 해서. 기록의 `queue`는 병합기 자신의 차례로 status 병합기 줄에 보인다.

## 판정 (advise)
- 착지 단계(`land`·`land-on`)이고 진행이 `advise_wait`분 안: 안 움직임이 아니라 단계 나이를 `advise_merge_slow`와 견준다(merge-slow). 안쪽 단계가 있으면 「착지 검증 N분째」 대신 그 줄과 나이.
- 그 밖: 조용한 시간 = min(가장 오래된 제출 나이, 지금 − `progress_at`) ≥ `advise_wait`분이면 merge-stall. 근거에 단계·까닭, 기다리는 대상마다 「<역할> <주소> 터미널 없음」 또는 「<역할> <주소> 진행 없음」.

## 걷어낸 것 (hc-fill·opsstate)
`statemerge.go`의 `specIDRe`·`landIDRe`(이름 정규식)·`stateFresh`(이름으로 터미널 찾아 원격 FreshSh)·`stateSpecWait`·`specWaitRec`(specwait 풀기)·`stateLanding`·`landLogRe`·`mergerGitDir`(child·착지 로그 이름 풀기)·`stateLandOn`, `landon.go`의 `maLandOn`(일지 마지막 「착지 실행」 줄), `opsstate`의 `JournalFacts.LastAt`·`LandOn*`·`LandRemote`, `MergeLine`의 `spec_*`·`land_on_{age_s,term,fresh,why}`, 손잡이 `advise_spec_fresh`(병합기 `merger_spec_fresh`가 진행을 잰다). 지킴 시험(`cmd/hc-fill/staterec_test.go`): hc-fill·opsstate 소스에 병합기 이름 꼴(`spec-`·`land-`·`specwait`·`.spec-wt` 등)이 다시 생기면, 그리고 `verified.FreshSh`를 병합기 밖에서 부르면 실패.

## 저장소 전체 훑기 (병합기 상태를 밖에서 짐작하는 곳)
| 곳 | 하는 일 | 판정인가 | 처리 |
|---|---|---|---|
| `cmd/hc-fill` statemerge·landon·state·status | 이름 정규식·specwait·child·착지 로그 이름·일지 마지막 줄 | 판정 | 걷어 내고 기록으로 |
| `internal/opsstate` mergestall·trim | 일지 마지막 줄 시각·「착지 실행」 줄 | 판정 | 기록 칸으로 |
| `internal/verified` FreshSh | 병합기 이름 꼴 폴더로 진행 재기 | 판정 재료 | 병합기만 부름(지킴) |
| `internal/landing` | 병합기가 올린 착지 중 원격 표식 | 계약된 표식 | 그대로 — 병합기가 쓰는 표식이지 짐작이 아니다 |
| `cmd/hc-lane` logs·stats | 일지·착지 로그 모으기·통계 | 아님(이력) | 그대로 |
| `internal/resident`·`internal/hcjson` ServiceTerm | 상주 종류·서비스 터미널 라벨 | 아님 | 그대로 |
| hc-sweep·운영 에이전트 지시문·docs/ops 그 밖 | 병합기 이름·기록을 판정에 쓰지 않음 | — | 해당 없음 |

## 합친 것
- hc-fill이 읽던 다섯 재료(specwait·child·착지 로그 이름·일지 마지막 줄·spec/land 터미널 이름) → 병합기 기록 한 장.
- 그 한 장의 사본 → 병합기 살아 있음 기록(merger-alive.tsv) 한 줄에(따로 파일을 두지 않음).
- specwait·specshard·child는 병합기 안 작업 메모로 남는다(일지 한 줄 모으기·나눔·고아 착지 확인) — 바깥 독자는 이제 없다.
