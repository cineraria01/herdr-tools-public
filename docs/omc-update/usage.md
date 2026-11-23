# omc-update: OMC 플러그인과 omc CLI를 기계마다 같은 판으로

- **왜**(2026-11-23 사용자 「Omc Update 안되는것도 수정 처리 하고 모든 서버 omc 도 설치하자」): `omc update`는 GitHub `releases/latest`와 npm latest만 본다.
  OMC 원 저장소가 git 태그·main(= Claude Code 마켓 클론)에만 낸 판은 릴리스·npm에 없어 `omc update`가 늘 「최신」이라며 아무것도 올리지 않는다.
- **사용**: `omc-update`(이 기계) · `omc-update --all`(`hc machine ls`의 연결된 기계 모두 + 이 기계, 동시에) · `omc-update --on <기계>[,<기계>…]` · `--check`(아무것도 바꾸지 않고 판만).
- **기계마다 순서**(`cmd/omc-update/remote.sh`를 그 기계의 로그인 셸 환경에서 — macOS는 `zsh -l` + `~/.zshrc`, 리눅스는 `bash -l`, 원격은 `hc exec --timeout 600`, 대상에 이 바이너리가 없어도 된다):
  1. 플러그인: `claude plugin marketplace update omc` → `claude plugin update oh-my-claudecode@omc`, 그 뒤 `~/.claude/plugins/installed_plugins.json`의 `oh-my-claudecode@omc` 첫 항목 판·`installPath`가 목표 판이다.
  2. CLI: 로그인 셸의 `omc --version`이 목표 판이면 건너뜀(`같음`).
     아니면 npm에 그 판이 있으면 `npm install -g oh-my-claude-sisyphus@<판>`(`npm`), 없으면 `installPath`(빌드된 dist가 든 플러그인 캐시)에서 `npm pack --ignore-scripts` → 그 묶음 `npm install -g` → 임시 폴더 지움(`묶음`).
  3. 이미 깔린 omc가 있으면 그 실제 경로(링크를 푼 것)의 `…/lib/node_modules` 앞이 접두사이고, 그 접두사의 npm(`<접두사>/bin/npm`, `npm_config_prefix=<접두사>`)으로 그 자리에 깐다 — 기본 npm(예: Homebrew)이 둘째 omc를 깔고 옛 판이 남지 않게. 없을 때만 기본 npm.
  4. PATH: 깐 뒤에도 로그인 셸에서 `omc`가 안 잡히고 `$(npm prefix -g)/bin/omc`가 있으면 `~/.local/bin`(PATH에 있을 때만)에 `omc`·`oh-my-claudecode`·`omc-cli` 링크(`+링크`). 링크 아닌 파일이 있으면 덮지 않고 경고.
  5. 확인: 로그인 셸의 `omc --version`이 목표 판이어야 그 기계 성공.
  6. 경고만(고치지 않음): `~/.claude/CLAUDE.md`의 `<!-- OMC:VERSION:x -->`가 목표 판과 다를 때, PATH에 omc가 둘 이상일 때(실제 경로로 중복 제거, 경로·판을 찍고 지우지 않음).
- **출력**: 기계마다 `<기계>  플러그인 <판>  CLI <전>→<후> (같음|npm|묶음[+링크])  CLAUDE.md <판>`, 실패면 그 줄 끝이나 `<기계>  실패: <까닭>`, 경고는 `<기계>  경고: …` 줄.
  `--check`는 `CLI <판> (같음|다름)`이고 플러그인 올리기·설치·링크를 하지 않으며, 플러그인은 지금 깔린 판을 목표로 본다(마켓의 새 판은 모른다).
  연결 안 된 기계와 sh를 못 띄우거나 Darwin·Linux가 아닌 기계는 `건너뜀`(종료에 안 셈). 종료: 0 · 1 실패 있음(`--check`는 CLI가 다른 기계도) · 2 잘못 부름.
- **하지 않는 것**: `omc update`·`omc setup`·`omc update-reconcile`(settings.json·CLAUDE.md·HUD를 다시 써 사람 손질을 덮는다), 플러그인 캐시·마켓 클론을 직접 고치기, sudo.
- **알려진 한계**: 리눅스 갈래 `bash -l`은 비대화라 흔한 `.bashrc`(맨 위에서 비대화 셸은 `return`)의 nvm PATH를 못 본다 — nvm에만 깔린 omc·npm은 「CLI 없음」으로 보이고 기본 npm 설치가 실패해 종료 1이 된다(해는 없음). 리눅스 줄이 「없음」이면 이 까닭부터 본다.
- 시험 `cmd/omc-update/main_test.go`(가짜 hc·claude·npm 접두사 둘·zsh·bash·uname).
