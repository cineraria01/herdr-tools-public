# hc-lane 병합기: 추측 검증 (병합기 병렬 검증)

2026-11-22 사용자 「병합기가 1개씩 처리를 하잖아 이거 … 병렬로 여러개를 동시에 처리는 못하는건가 근본적으로?」 → 「병합기에 추측 병렬 검증을 넣을까요?」에 「넣기 — 지금 바로」.
왜: 처리량의 병목이 검증 시간(큰 저장소 11~25분)인데 워커 맥은 놀았다.
설계는 `cmd/hc-merger/spec.go` 머리말.

- **무엇**: 큐가 A·B·C면 병합기는 A를 지금처럼 직접 검증·착지하면서, 검증 기계(설정 줄 `merger_verifiers <프로젝트> <기계,…>` — `config.example/hc-fill.tsv`)에 main+A+B·main+A+B+C를 함께 띄운다(`hc run --on <기계> --cwd <그 기계 repo 줄 클론> hc-lane merger --spec-verify <아이디> <기본>`).
  묶음(크기 `HC_LANE_MERGER_BATCH`)은 후보 하나다.
  착지는 큐 차례(우선 → 시각) 그대로이고, 맨 앞 후보의 추측이 도는 중이면 그 바퀴는 기다린다.
- **정확성은 지금과 같다**: 착지하는 트리는 정확히 검증한 트리다.
  추측은 「부모 트리 + 후보」로 이름이 붙고, 착지(`hc-lane land`, 병합기 안)는 rebase한 트리가 그 추측 트리와 같고 원격 기록이 있을 때만 검증을 건너뛴다.
  그 사이 main이 움직였거나 rebase 결과가 다르면 기록이 없어 지금처럼 병합기 기계에서 검증한다(그 뒤 추측은 버리고 새 기준에서 다시).
- **실패·모름**: 추측이 실패하면 그 후보는 맨 앞이 될 때 지금처럼 병합기가 직접 검증해 지금 규칙대로 반송하고(검증 기계 한 대의 흔들림으로 반송하지 않는다), 그 후보를 품은 뒤 추측은 버리고 그것을 뺀 추측(main+A+C)을 다시 띄운다.
  못 띄움·시간 넘김(`HC_LANE_SPEC_TIMEOUT`, 기본 3600초)·준비 훅 실패·검증이 트리를 바꿈은 「모름」 — 그 후보는 직접 검증.
- **검증 기계 부하**: 한 기계에 추측 하나씩, 병합기 기계는 빼고, 일꾼은 1분 부하가 코어 수를 넘으면 30초씩 기다린다(`HC_LANE_SPEC_LOAD_WAIT`, 기본 900초).
  검증 자리는 저장소 검증 스크립트의 기계 잠금(기계별 검증 자리 수 등)이 레인 검증과 나눠 쓴다.
- **원격 참조가 일지다**(병합기가 죽어도 다음 병합기가 바퀴의 ls-remote로 이어 간다): `refs/herdr/spec-verifiers`(검증 기계 목록 — hc-fill이 씀), `refs/hc-merger/spec/<아이디>`(추측 — 트리 = 후보 트리, 제목 `hc-merger-spec parent=… group=… machine=… at=… state=run|local|lost|conflict`), `refs/hc-merger/spec-fail/<아이디>`(실패), `refs/hc-merger/spec-run/<아이디>`(일꾼이 떴다는 표식 — 띄운 뒤 `HC_LANE_SPEC_START`(기본 300초) 안에 없으면 모름), `refs/herdr/verified/<트리>`(통과 기록, `internal/verified`).
  길에서 벗어난 추측과 짝 없는 결과는 다음 바퀴(큐가 비면 그때)에 치운다.
  끄기: 설정 줄을 지운다(남은 참조는 병합기가 치운다).
- **검증 기록 프로토콜**(저장소 검증 스크립트가 받는 쪽):
  - 기록: `refs/herdr/verified/<트리 SHA>` → 부모 없는 커밋, 그 커밋의 트리 = 검증한 트리, 제목 `herdr-verified tree=<트리> script=<그 트리의 .claude/herdr-verify.sh blob> machine=<기계> at=<유닉스 초>`.
  - 병합기는 맨 앞 추측이 통과했으면 착지에 `HERDR_SPEC_TREE=<트리>`를 준다.
    `hc-lane land`(`HERDR_MERGER=1`일 때만)는 그 기록이 있고 제목의 `script`가 그 트리의 검증 스크립트 blob과 같을 때만 검증 스크립트(내장 길) 또는 저장소 착지 스크립트(`.claude/herdr-land.sh` — 그 스크립트가 검증 자식에 물려준다)에 **`HERDR_VERIFIED_TREE=<트리>`** 를 넘긴다.
    내장 길은 rebase한 트리가 그 값과 다르면 넘기지 않는다.
    물려받은 `HERDR_VERIFIED_TREE`는 착지가 늘 지운다.
  - 검증 스크립트는 **지금 `git rev-parse HEAD^{tree}`가 그 값과 같고, `git status --porcelain --untracked-files=no`가 비고, `git ls-remote origin refs/herdr/verified/<그 값>`이 있을 때만** 검증 없이 종료 0(한 줄 「건너뜀 검증」).
    하나라도 아니면 그대로 검증한다.
    사전 검사(`--cheap-only`)는 늘 돈다.
    이 저장소의 `.claude/herdr-verify.sh`가 본보기다.
  - 추측 일꾼은 `HERDR_SPEC_VERIFY=1`, `HC_LANE=spec-<아이디 8>`, 빈 `HERDR_VERIFIED_TREE`로 검증 스크립트를 부른다(건너뛰지 않는다).
    작업 트리는 `<클론>.spec-wt/<아이디 8>`, 로그는 그 옆 `.log`.
- **켤 때 할 일**: 검증 기계마다 그 프로젝트 클론을 repo 줄 경로에 두고(origin 같게, 그 기계에서 `.claude/herdr-setup.sh`·검증이 돌 준비 — 레인 클론과 같은 클론을 써도 된다: 일꾼은 분리 작업 트리만 만든다), 그 기계에 `hc-lane`·`hc-merger` 바이너리 줄을 두고, 맥 개인 `hc-fill.tsv`에 `merger_verifiers <프로젝트> <기계,…>` 한 줄.
