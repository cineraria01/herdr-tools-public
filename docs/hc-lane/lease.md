# hc-lane 레인 임대

- **레인 임대**(2026-11-20 fill-dup-guard — 같은 이름의 레인이 두 기계에 함께 뜨지 않게, 중복 막기는 띄우는 쪽에서): 쓰기 레인은 `hc run` 직전에 프로젝트 원격에 `refs/hc-lanes/<레인>`(빈 트리 커밋, 제목 `hc-lane lease <레인> <기계> <UTC>`)을 **없을 때만** 만든다(`push --force-with-lease=<ref>:` — 원격이 원자적으로 판정).
  이미 있으면 `hc-lane: 이미 돎 …`과 **종료 10**, 조회·push 실패도 띄우지 않고 종료 1(모르면 안 띄움).
  둘 다 방금 만든 워크트리를 되돌린다.
  push 응답만 잃었으면 원격 값이 이 실행의 것인지로 판정한다.
  레인 환경 `HC_LANE_LEASE` = 그 값. `--read`와 origin 없음이 확인된 저장소는 임대가 없다.
  hc-fill은 10을 「못 띄움」으로 읽어 다음 기계로 가고(거기서도 10이면 그 바퀴는 포기), 그래서 응답을 잃고 다른 기계에 다시 시도해도 둘이 뜨지 않는다.
  - 풀기는 둘뿐: `hc-lane land` 성공(아래 표식과 같은 원자 push에서 `HC_LANE_LEASE` 값일 때만 지움)과 `hc-lane clean`. 그 밖(레인이 죽음·지휘가 `hc-fill rm`)에는 임대가 남는다 — **임대가 남아 다시 못 띄우면** `hc-lane lease drop [--on <기계>] --repo <저장소> --name <레인>`(지금 값은 `hc-lane lease show …`).
    `hc-fill rm`은 임대를 건드리지 않는다.
    레인이 죽어 남은 임대는 `hc-lane --revive`가 주인이 같은 레인·기계일 때 놓지 않고 이어 쓴다(docs/hc-lane/revive.md).
    `hc run`이 실패하면 떴는지 몰라 임대를 남기고 그 명령을 알린다.
