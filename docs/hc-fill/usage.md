# hc-fill (빈 자리에 기다리는 레인을 바로 띄우기, 맥)

```sh
hc-fill add --name <레인> --project <프로젝트> --brief <지시문> [--after <레인>,…] [--only <기계>,…] [--coord <지휘>] [--priority <긴급|높음|보통|낮음>] [--no-early] [--no-audit] [--overlap-ok | --solo] [--on-land <이름>] [--pr-of <원래 레인>]   # 대기열 끝에(--pr-of: 원래 PR을 이어 씀, docs/hc-fill/pr.md)
hc-fill add --name <이름> --project <프로젝트> --brief <지시문> --then <대기 중인 선행>   # 새 줄 없이 선행 레인에 이어 붙임
hc-fill prio <레인> <긴급|높음|보통|낮음>   # 레인의 우선순위 바꾸기 — 이미 병합기 큐에 냈으면 큐 메타도(영어 urgent|high|normal|low도 받음)
hc-fill ls                      # 대기 · 떠 있음(hc 상태) · 끝남 · 되돌려줌(받은편지 결과를 보냄) · 주인 지휘 · 우선 · 마지막 보고 · 먼저 띄움(선행 …) · 이어서 <이름>,… · 자동 감사(감사 중·감사 통과/지적 — 수거 기다림·감사 통과 받음·감사 지적 n번째·지휘로 넘김) · 겹침 대기(<앞 레인>: <키>) · 겹침 봄 끔 · 필요 <능력> — 가진 기계 없음
hc-fill status [--owner <지휘 주소> | --mine] [--all]   # 레인마다 한 줄 — 기계 · 넣은 뒤 · 단계(대기·대기(묶인 기계 까닭 — [묶인 기계](pinned.md))·주차·작업 중·완료·감사 n번째·감사 지적·제출됨(병합기 줄 맨 앞 / n번째, 이 기계 병합기가 그 제출을 도는 묶음에 넣었으면 「· 묶음 검증 중 <기계> n분째」·「· 착지 중 n분째」)·착지 sha·착지 <sha 8> <월-일 시:분>(병합기 줄에 없고 그 줄 접수 뒤의 착지 표식 `refs/hc-landed/<레인>/<초>`가 있으면 — 대기열이 제출한 레인도 「대기열이 제출함」 대신)·막힘: 까닭·모름(까닭)). 끝난 줄(착지·되돌려줌)의 넣은 뒤는 끝난 때까지(되돌려준 때는 장부의 바꾼 시각, 모르면 지금까지). 끝난 지 `state_done`분(손잡이 — [상태 한 장](../ops/state.md)과 같은 값, 기본 60)이 넘고 터미널이 살아 있지 않은 줄은 기본 표에서 빼고 표 끝에 「끝난 레인 n개 숨김(--all로 모두)」 — `--all`은 모두, `--mine`·`--owner`는 제 줄만 세어 같은 규칙(ht-fill-status-done, 2026-11-26 — 표가 848줄, 하루 넘게 전에 착지한 레인이 「감사 통과 — 대기열이 제출함」으로 남았다) · 예상((줄 차례 ÷ 한 번에 넣은 최대 개수) × 최근 착지 6번 간격의 중앙값 — 2분 안의 표식은 묶음 착지 한 번, 근거를 함께, 모르면 비움). 읽기만, 원격은 프로젝트마다 ls-remote 한 번. 줄 차례는 제출 시각 순(병합기의 우선은 안 봄). 표 끝에 바퀴 계측 한 줄(최근 바퀴 걸린 시간·가장 긴 단계·바깥 명령 수, 최근 바퀴들 중앙값 — 「한 바퀴」 문서)
hc-fill knob set pr <프로젝트>[,…] --why <까닭>   # PR 거울 켜기 — 레인마다 PR 한 장(완료 때 열고 감사·검증·병합 결과를 쓰고 착지하면 Merged, docs/hc-fill/pr.md). status·state에 「PR #n」
hc-fill note <레인> <글|파일>   # 지휘 지시를 레인 기계 verdicts/<레인>/coord-notes.md에 덧붙이고 레인에도 hc send — 다음 감사가 {coord_notes}로 본다(docs/hc-fill/audit.md). 떠 있지 않은 레인은 종료 1
hc-fill merger status             # 설정 merger 줄마다 병합기 한 줄(run 바퀴 기록 — 없으면 「기록 없음」, 줄이 없으면 「병합기 없음」), 종료 0. hc-merger status가 부른다
hc-fill merger restart <프로젝트>   # 병합기를 지금 띄우는 길 그대로 한 번 다시 띄운다(docs/hc-lane/merger.md 「병합기 살아 있음」). ls·status 끝에 프로젝트마다 병합기 한 줄. 살아 있으면 안 띄움(종료 1)
hc-fill run --dry-run           # 무엇을 어디에 띄울지만. --dry-run 없이 한 바퀴 띄운다
hc-fill watch --every 60        # 계속(한 번에 하나만, 이미 돌면 종료 75). 바퀴 시작 간격이 60초 — 쉼은 60초 − 바퀴에 든 시간, 하한 설정 rest_min
hc-fill rm <레인> [--force]     # 대기열에서만 뺀다(떠 있는 터미널은 그대로). 주차 줄(refs/park/<레인>에 준비 커밋)은 --force 없이 거절(종료 2)
hc-fill retry <레인>            # 띄우기 거절(hc-lane 종료 11 — 다시 해도 같음)로 멈춘 줄을 다음 바퀴에 다시(edit도 풂, 임대 거절은 임대가 풀리면 스스로, 대기열에 없으면 종료 2)
hc-fill edit <레인> [--after <레인>,…|-] [--after-any-project] [--priority <우선>] [--only <기계>,…|-] [--brief <지시문>] [--overlap-ok | --overlap-check] [--early | --no-early] [--pr-of <원래 레인>|-]   # 그 줄의 칸만 바꾼다(주차·줄 ID·주인 그대로). 겹침 검사·먼저 띄움은 아직 안 뜬 줄만(docs/hc-fill/edit.md)
hc-fill machine --rename <옛> <새> [--dry-run] [--json] [--why <까닭>]   # 기계 이름을 설정·상태 파일·손잡이·병합기 켬 표식에서 한 번에(잠금 안, 실패면 전부 되돌림). 차례 dry-run → 실행 → merger restart → 설정 커밋(docs/hc-fill/machine-rename.md)
hc-fill nudge <레인> [<글>] [--why <까닭>]   # 일시 오류로 턴이 끊겨 멈춘 레인 터미널(hc 상태 error·done, 그 실행의 완료·막힘 등 보고 없음)에 이어 가라는 글 한 줄. 한 레인에 손잡이 nudge_every분(기본 10) 안에 한 번, 안 보내면 까닭과 종료 1(docs/hc-fill/edit.md)
hc-fill tell [--audit] <레인> <글>   # 레인에 글 보내기는 이것으로 — 대기열 줄이 기록한 지금 run 터미널(주소·이름이 맞고 살아 있는 것) 하나에만 hc send. 같은 이름의 앞 run 터미널은 보지 않는다. --audit이면 그 레인의 지금 감사 줄 터미널로. 떠 있지 않음·끝남·이름 다름·셸만 남음·모름은 보내지 않고 까닭과 종료 1
```
