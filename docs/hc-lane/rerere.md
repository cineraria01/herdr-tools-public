# hc-lane rerere (같은 충돌 다시 풀기)

- rerere(2026-11-19 lane-overlap-guard — 착지를 다시 설 때마다 같은 충돌을 손으로 다시 풀었다): 쓰기 레인 작업 트리를 만들 때 그 저장소의 공용 git 폴더에 `rr-cache`만 만든다 — 저장소 설정에는 아무것도 쓰지 않는다(2026-11-20 overlap-guard-2: 저장소 설정에 `true`를 쓰면 뒤에 뜬 가지의 조건부 `includeIf "onbranch:…"` `false`를 가렸다, 아스트라 2차 중간 7).
  git은 `rerere.enabled`를 정하지 않은 가지에서 `rr-cache`가 있으면 켜고, 정해 둔 값(사용자가 끈 `false`, 전역·조건부)은 그대로 따른다.
  `rerere.autoUpdate`를 켜지 않으므로 land는 `Resolved '<경로>' using previous resolution`으로 푼 경로를 스스로 `git add`한다.
  `hc-lane land`(내장 착지 — `.claude/herdr-land.sh`가 없는 저장소)는 rebase가 멈추면 git 출력(`LC_ALL=C`)을 보고, 커밋 적용 중(`REBASE_HEAD`) **내용 충돌**로 멈췄고 그 충돌 경로가 모두 `Staged '<경로>' using previous resolution`(autoUpdate) 또는 `Resolved '<경로>' …`(git add 한 뒤)이며 미병합(`git diff --diff-filter=U`)과 `git rerere remaining`이 각각 성공하고 비었을 때만 `hc-lane land: rerere가 기록한 풀이로 충돌을 다 풀었다(<경로>) — rebase를 이어 간다`를 찍고 `GIT_EDITOR=true git rebase --continue`(다음 커밋에서 또 멈추면 같은 판정).
  일부만 풀렸거나, 지움·이름 바꿈 충돌이거나, 훅 거절 같은 충돌 아닌 중단이면 멈춘 그대로 종료 3 — 미병합이 비었다는 것만으로는 잇지 않는다.
  풀이 기록은 클론마다(`.git/rr-cache`)라 같은 기계에서 다시 선 착지에만 듣는다.
  `.claude/herdr-land.sh`가 있는 저장소는 그 스크립트가 맡는다.
