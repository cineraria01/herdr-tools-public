# hc-lane land: 착지 표식·착지 안전

- **착지 표식**: `hc-lane land`는 main push와 같은 `--atomic` push에 `refs/hc-landed/<레인>/<UTC 초>`(값 = 착지한 커밋, 지우지 않음)와 임대 지우기를 싣는다.
  레인 이름은 `HC_LANE`, 없으면 가지 이름(ref에 쓸 수 있을 때만).
  표식·임대 쪽으로 push가 통째 거절되면 검증을 다시 돌지 않고 main만 한 번 더 착지하고, 표식은 그 커밋이 `origin/<기본>`에 들어갔을 때 따로 두고, 임대는 놓지 못하면 알리기만 한다.
  `.claude/herdr-land.sh`에는 `HERDR_LAND_PUSH_EXTRA="HEAD:refs/hc-landed/<레인>/<초> [:refs/hc-lanes/<레인>]"`로 넘겨 그 스크립트가 main push에 싣게 하고, 안 실었으면 착지 뒤 같은 식으로 따로 둔다.
  원격 없는 저장소는 임대·표식이 없다(선행 판정은 제목 규칙만).
  `--direct`로 띄운 쓰기 레인도 임대·표식은 같다.
  병합기 안 착지(`HERDR_MERGER=1`)는 push를 병합기 판 그대로 두고(전송 끊김·응답 유실 판정이 그 인자에 묶임) 표식은 착지 뒤 따로 둔다.
  병합기의 제출 ID 표식 `refs/hc-landed/<제출 ID>`와 같은 이름 공간이라 레인 이름이 어떤 제출 ID와 같으면 그 레인 표식은 못 둔다(제목 규칙만 — 제출 ID는 `<기계>-<UTC>-<pid>` 꼴).
- 병합기가 켜진 저장소의 제출은 기본 브랜치와 3자 합침이 충돌하면 큐에 넣지 않고 종료 3이다(2026-11-23 conflict-fast — 제출할 때 이미 충돌한 제출이 줄에서 기다린 뒤 반송됐다. 기본 브랜치와 같은 patch-id 커밋이 든 제출은 예외 — rebase가 그 커밋을 건너뛰어 깨끗할 수 있어 그대로 낸다). `git rebase origin/<기본>`으로 풀고 푼 파일 둘레만 확인한 뒤 같은 줄을 다시([병합기](merger.md) 「제출 때 충돌 확인」).
- 착지 안전(2026-11-20 land-metrics — 아스트라 감사·운영 모델 「레인은 아무 단계에서나 죽고, push 응답은 사라질 수 있고, git은 실패할 수 있다」): 내장 `hc-lane land`가 보내는 것은 **검증한 커밋 그 자체**다.
  바퀴마다 기준 SHA를 고정해 그 SHA로 rebase하고, rebase 뒤 HEAD(후보)를 고정한다 — 조회가 실패·빈 값이거나 그 값이 이 저장소의 커밋이 아니면(`git cat-file -e <sha>^{commit}`) 아무것도 보내지 않는다(빈 후보로 push하면 `:refs/heads/<기본>`, 기본 브랜치 지우기).
  후보가 기준 그대로면 보낼 것이 없어 push하지 않고 `이미 들어가 있다`·종료 0. rebase 뒤·검증 뒤에 색인·추적 파일이 후보와 같아야 하고(검증이 파일을 고쳐 통과하면 그 수정은 커밋에 없다), 검증하는 동안 HEAD가 바뀌면 보내지 않는다.
  push는 `git push origin <후보>:refs/heads/<기본>`(원격 이름으로 — 주소를 넘기면 `pushInsteadOf`가 다시 적용되고 추적 ref가 안 바뀐다).
  검증 뒤 origin의 push 주소를 다시 읽어 시작 때와 다르면 보내지 않는다.
  진단에 주소는 찍지 않는다(인증정보가 든 주소일 수 있다).
  git 실패는 성공·없음·「아님」으로 읽지 않는다: `git status`가 실패하면 깨끗함으로 읽지 않고 멈추고, push가 실패로 돌아오면 `git ls-remote origin refs/heads/<기본>`에서 둘째 칸이 정확히 `refs/heads/<기본>`인 줄 하나만 받아(없거나 둘이면 모름 — ls-remote는 `refs/archive/refs/heads/<기본>`처럼 이름 끝도 맞춘다) fetch한 뒤 견주고, 후보가 원격 기본에 들어가 있으면(응답만 사라짐) 착지로 끝내며(종료 0), 그 fetch나 견주기(`merge-base` 종료 0·1 밖)가 실패하면 거절로 읽어 다시 하지 않고 `push 결과 모름`으로 멈춘다(다시 하는 것은 사람 — 들어가 있었으면 같은 줄이 `이미 들어가 있다`).
  검증·착지 스크립트는 앞에서 돌려 터미널 Ctrl-C(그룹 SIGINT)면 같이 멈춘다 — hc-lane만 죽고 스크립트가 남아 push하는 일이 없다.
  `git remote get-url origin`은 종료 2(원격 없음)일 때만 원격 없는 저장소로 보고 다른 실패면 멈추며(원격 저장소인데 로컬 기본을 올리던 기존 결함), 원격 없는 착지에서 `git worktree list`가 실패하면 빈 목록으로 읽지 않고 멈춘다(체크아웃된 기본에 `update-ref`해 메인 폴더가 뒤처지던 기존 결함).
  체크아웃된 폴더는 `git merge --ff-only --no-squash`로 올린다 — 저장소 설정 `branch.<기본>.mergeOptions=--squash`면 기본은 그대로인데 0이라 착지한 줄 알았다.
  한계: origin의 push 주소는 하나만 지원한다 — `git remote get-url --push --all origin`이 두 줄 이상이면(또는 조회가 실패하면) 내장 착지는 시작하지 않는다(한 곳만 받아도 착지로 정하게 되므로).
  브랜치별 조건 설정(`includeIf "onbranch:…"`)도 지원하지 않는다 — 하나라도 있거나 설정 조회가 실패하면 시작하지 않는다(HEAD 상태에 따라 push 주소 등이 바뀐다).
  origin의 fetch 주소와 push 주소는 같은 저장소를 가리킨다고 본다(git의 원격 전제 — 다른 저장소면 원격 이름을 따로 둔다).
