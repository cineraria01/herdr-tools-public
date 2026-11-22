# 설정

개인 값(기계 이름·경로·계정)은 저장소 밖 설정 폴더 `HERDR_TOOLS_CONFIG`(기본 `~/.config/herdr-tools`)에 둔다.
꼴과 키는 `config.example/`(가짜 값)을 본다.

```sh
herdr-tools-config get balance.remote ""   # 값. 키가 없으면 기본값, 기본값도 없으면 종료 3
herdr-tools-config file hc-fill.tsv        # 설정 폴더의 그 파일 절대 경로. 없으면 종료 3
herdr-tools-config lint [--strict] [<파일>…]  # git ls-files(또는 준 파일)에서 개인 값 패턴(private-patterns.txt)에 걸린 줄. 걸리면 종료 1
herdr-tools-config sync-lint <범용 저장소> [<sync.list>]  # sync.list의 바이너리 줄 검사(아래). 걸리면 지울 줄·더할 줄을 찍고 종료 1
```

- `sync-lint`가 막는 것: 걷힌 셸 판 줄(저장소에 없는 `tools:lib/…`·`tools:…*.sh`·`*.py`, `cmd/<도구>`가 있는데 남은 `tools:bin/<도구>` — 바꿀 바이너리 줄을 찍는다), 틀린 바이너리 줄(맥 경로가 `<뿌리>bin/<도구>`가 아니거나 `cmd/<도구>`가 없음), 딸린 줄 빠짐(바이너리 줄의 대상을 덮는 Go 소스 나무 줄 셋, 곁 도구 — `cmd/<곁>/OWNER` 첫 줄이 그 도구 — 의 바이너리 줄), 줄 끝 CR·끝 줄바꿈 없음(`<파일>:<줄>:`).
  `<sync.list>`를 빈 글로 주면 기본 경로다.
- herdr-sync는 저장소를 pull한 직후 `sync-lint`를 부르고, 걸리면 아무 데도 쓰지 않고 종료 2로 멈춘다(`--check`도).
  셸 판 줄이 남으면 맥에 남은 옛 파일을 저장소로 다시 모으기 때문이다(맥이 정본).

- `config`는 한 줄에 `<키> <값>`(값은 줄 끝까지), 키 혼자면 빈 값, `#` 줄은 주석. 같은 키 두 번·잘못 부름은 종료 2.
- lint 패턴은 `$HERDR_TOOLS_CONFIG/private-patterns.txt`, 없으면 `~/.local/share/herdr-tools/private-patterns.txt`. 둘 다 없으면 건너뛴다(종료 0).
  `--strict`면 패턴 파일이 없거나 패턴 줄이 없을 때 종료 3(`get`·`file`의 「설정 없음」과 같은 코드).
- 검증(`.claude/herdr-verify.sh`)은 `lint --strict`를 부른다: 패턴 파일이 빠진 기계에서는 「실패 개인 값 검사」로 착지하지 않는다(herdr-sync로 패턴 파일을 맞춘다).
  패턴이 없는 게 정당한 곳(공개 사본을 받아 검증을 돌리는 쪽 — 공개 트리는 이 저장소 트리와 같아 같은 검증이 들어 있다)만 `HERDR_VERIFY_NO_LINT=1`로 건너뛴다(「건너뜀 개인 값 검사」).
  레인 안(`HC_LANE`이 있을 때)에서는 이 값을 무시하고 strict로 돈다(「레인 안이라 HERDR_VERIFY_NO_LINT 무시」).
- lint는 이진·UTF-8 아닌 파일도 바이트로 본다.
  상대 경로로 받은 파일(인자 없을 때의 `git ls-files` 포함)은 경로 글자도 보고 `파일:0: 경로 <패턴>`으로 찍는다(절대 경로는 임시 파일 자리라 내용만).
  못 읽는 파일이 있으면 종료 2(검사 못 함).
  `git ls-files`에 있지만 작업 트리에서 지운 파일은 뺀다.
  패턴은 바이트 단위다(`LC_ALL=C` — 한글 같은 비ASCII 글자에서 `.`은 글자 하나가 아니라 바이트 하나).
  대소문자는 무시한다(`-i`, ASCII만 접힘).
