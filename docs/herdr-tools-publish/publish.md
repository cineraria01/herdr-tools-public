# 공개 저장소로 내보내기 (herdr-tools-publish)

작업은 이 저장소(비공개)에서 하고, 공개 저장소에는 지금 파일만 기록 없이 내보낸다.
비공개 기록(커밋 메시지·작성자 이메일, 옛 판의 개인 값)은 나가지 않는다.

```sh
herdr-tools-publish --dry-run   # 바뀔 파일 목록(A·M·D)과 검사 결과만. push 없음, publish.dir을 만들지 않음(없으면 임시로)
herdr-tools-publish             # 검사 → 공개 main 위에 커밋 하나 → push
```

- 설정 키(개인 설정 `config`): `publish.repo` 공개 저장소 주소(없으면 종료 2, herdr-sync는 부르지 않는다) · `publish.dir` 공개 쪽 bare clone 자리(기본 `~/.local/share/herdr-tools/public`, 없으면 만든다.
  있는데 bare git이 아니거나 origin이 `publish.repo`와 다르면 종료 1) · `publish.author` 공개 커밋 작성자·커미터 `이름 <메일>`(없으면 공개 clone의 git 설정 `user.name`·`user.email`. git이 호스트 이름으로 지어낸 값은 받지 않고, 부른 쪽 환경의 `GIT_AUTHOR_*`·`GIT_COMMITTER_*`·`EMAIL`은 지운다 — noreply 메일을 둔다) · `publish.secrets` 더 막을 비밀 꼴 ERE 하나(여러 꼴은 `|`).
- 하는 일: `git fetch origin main` → `refs/remotes/origin/main`(같은 이름의 로컬 가지에 속지 않게 전체 이름) 트리의 블롭마다 `cat-file`로 `<sha>` 이름 파일에 꺼내 검사(경로대로 디스크에 꺼내지 않는다 — macOS 디스크는 대소문자·정규화를 구분하지 않아 경로가 겹치고 링크를 따라간다.
  `git archive`는 `export-subst` 같은 attributes로 비공개 커밋 정보를 넣을 수 있어 쓰지 않는다) → 그 트리·블롭 객체만 공개 쪽으로 옮겨(비공개 커밋 객체는 안 옮김, 공개 트리 sha = 비공개 트리 sha — 지운 파일·실행 비트·링크 포함) 공개 main 위에 커밋 하나, 메시지는 `herdr-tools <비공개 짧은 sha> 내보냄` 한 줄 → fast-forward push(강제 push 없음).
  공개 main이 없으면 부모 없는 첫 커밋. 트리가 공개 main과 같으면 `같음`, 커밋하지 않는다.
- **막는 것(fail-closed)**: 아래 하나라도 걸리면 아무것도 push하지 않고 걸린 `파일:줄: 까닭`을 찍고 종료 1(비밀 값 자체는 찍지 않는다).
  - 개인 값: `herdr-tools-config lint`(개인 설정 `private-patterns.txt`) — 블롭 내용(이진·UTF-8 아닌 파일도 바이트로), 경로, 링크 대상 글자, 공개 커밋 작성자·커미터. 그 파일이 없거나 패턴 줄이 없거나 lint가 실패하면(못 읽은 파일 포함) 「검사 못 함」으로 종료 1.
  - 비밀 꼴(코드 안 범용 꼴): 개인 키 머리(BEGIN … PRIVATE KEY), GitHub 토큰(`ghp_`·`gho_`… `github_pat_`), `sk-` 키, Slack `xox?-`, AWS `AKIA`, Google `AIza`, 40자 넘는 16진, 32자 넘게 이어지고 대·소문자·숫자가 다 든 base64 꼴 토큰(알파벳 목록 `ABCDEFGH…`·`0123456789`가 든 것은 뺌), 그리고 `publish.secrets`. 내용·경로·링크 대상 글자를 바이트로 본다.
    경로가 걸리면 이름은 가리고 폴더와 몇 번째 파일인지만 찍는다.
  - 글이 아닌 블롭: NUL이 들었거나 UTF-8로 안 풀리는 블롭(UTF-16 글·압축·이미지 등 — 바이트 검사가 속을 못 본다).
    걸린 경로만 찍는다.
    범용 도구 저장소에 이진 파일은 없어야 정상이라 허용 목록은 없다(필요해지면 그때 설정 키로).
  - 받는 항목은 일반 파일·실행 파일·링크 블롭과 비어 있지 않은 하위 트리뿐이다.
    빈 하위 트리·하위 모듈·낯선 mode는 막는다(이름은 가리고 폴더만 찍음).
    하위 트리 이름도 경로 검사를 받는다(`ls-tree -r -t`).
  - 줄바꿈이 든 경로, 대소문자·유니코드 정규화(NFC)만 다른 경로 쌍(폴더 포함, `n번째·m번째 파일`로 찍음).
  - 공개 커밋 작성자 검사는 커밋을 만들 그 공개 clone에서 한다.
    push·fetch는 `origin` 설정(pushurl 등)이 아니라 `publish.repo` 주소로 직접 한다.
    replace ref는 따르지 않는다(`GIT_NO_REPLACE_OBJECTS=1` — 검사한 블롭 = 옮기는 블롭).
    `--dry-run`은 실제 `publish.dir`에 아무것도 쓰지 않는다(객체는 임시 bare에).
  - 검사 도구(`herdr-tools-config`)가 이 도구 곁에 없으면 종료 1.
- push·fetch는 인증 질문에서 멈추지 않는다(`GIT_TERMINAL_PROMPT=0`, `GIT_SSH_COMMAND`가 없으면 `ssh -o BatchMode=yes`).
- 결과 파일(2026-11-21 Go 전환 감사 A02): `HERDR_TOOLS_PUBLISH_RESULT=<파일>`이면 시작 때 그 파일을 지우고, 게시가 성공(`내보냄`·`같음`, 종료 0, `--dry-run` 아님)했을 때만 검사한 비공개 커밋 전체 sha 한 줄을 반쪽 없이 쓴다(실패·막힘·미리보기는 파일 없음).
  부른 쪽은 이 커밋으로 릴리스를 짓는다.
- 출력·오류 글에 `publish.repo` 주소 원문을 싣지 않는다(키 이름으로 — ls-remote·fetch·push의 git 오류 글도 주소를 바꿔 찍는다.
  git이 주소를 다른 꼴로 바꿔 적은 것은 못 가린다).
- 막히면 `--dry-run`도 가린 진단 뒤 바로 종료 1(변경 목록 없음), 작성자·커미터도 비밀 꼴 검사, 변경 목록(`diff-tree`)을 못 읽으면 종료 1, 객체 옮기기(`pack-objects | unpack-objects`)는 `HERDR_TIMEOUT_GIT`(60초) 안에서만 — 넘기면 둘 다 거두고 「시간 넘김」·종료 1(모름, push 없음).
  push 뒤 로컬 `refs/heads/main`을 못 바꾸면 경고만(종료 코드는 원격 결과), 다음 실행이 확인한 원격 값으로 맞춘다.
- 늘 내보내기: `herdr-sync`가 맥에서 다 맞춘 뒤(`--check`가 아닐 때) `publish.repo`가 있으면 부른다.
  실패하면 `!!! … !!!`로 크게 경고하고, herdr-sync 종료 코드는 그대로다(기계 맞추기와 별개).
- 시험이 비밀 꼴을 다룰 때는 조각을 이어 만든다(시험 파일 자체가 검사에 걸리지 않게).
