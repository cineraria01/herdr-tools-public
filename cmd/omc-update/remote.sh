# omc-update가 기계마다(맥이면 로컬 sh, 원격이면 hc exec) 그 기계의 로그인 셸 환경에서 돌리는 조각 — POSIX sh(리눅스 dash도).
# 첫 인자가 일: plugin(플러그인 올리기) · probe(판 읽기) · install <판> <접두사> <플러그인 폴더>(CLI 깔기 + PATH 링크).
# 부르지 않는 것: omc update·omc setup·omc update-reconcile(settings.json·CLAUDE.md·HUD를 다시 써 사람 손질을 덮는다), sudo.
set -u
pkg=oh-my-claude-sisyphus
rp() { realpath "$1" 2>/dev/null || readlink -f "$1" 2>/dev/null || printf '%s\n' "$1"; }
ver() { "$1" --version 2>/dev/null | head -n 1; }
case ${1-} in
plugin)
  claude plugin marketplace update omc >&2 && claude plugin update oh-my-claudecode@omc >&2
  ;;
probe)
  # C <판> <접두사>: PATH가 고르는 omc(접두사 = 링크를 푼 실제 경로의 …/lib/node_modules 앞) · A <경로> <판>: PATH의 omc 모두(실제 경로로 중복 제거)
  # M <CLAUDE.md 표식> · J 줄 뒤는 installed_plugins.json 그대로
  c=$(command -v omc 2>/dev/null) || c=
  p=
  if [ -n "$c" ]; then
    case $(rp "$c") in */lib/node_modules/*) r=$(rp "$c"); p=${r%%/lib/node_modules/*} ;; esac
    printf 'C\t%s\t%s\n' "$(ver "$c")" "$p"
  else
    printf 'C\t\t\n'
  fi
  seen=:
  IFS=:
  for d in $PATH; do
    f=$d/omc
    [ -n "$d" ] && [ -f "$f" ] && [ -x "$f" ] || continue
    r=$(rp "$f")
    case $seen in *":$r:"*) continue ;; esac
    seen=$seen$r:
    printf 'A\t%s\t%s\n' "$f" "$(ver "$f")"
  done
  unset IFS
  printf 'M\t%s\n' "$(grep -o '<!-- OMC:VERSION:[^ ]* -->' "$HOME/.claude/CLAUDE.md" 2>/dev/null | head -n 1)"
  echo J
  cat "$HOME/.claude/plugins/installed_plugins.json" 2>/dev/null
  ;;
install)
  # 이미 깔린 omc가 있으면(접두사 $3) 그 접두사의 npm으로 그 접두사에 깐다 — 기본 npm(예: Homebrew)이 둘째 omc를 깔지 않게
  v=$2 p=$3 ip=$4 op=$PATH npm=npm
  if [ -n "$p" ]; then
    PATH=$p/bin:$PATH npm_config_prefix=$p
    export PATH npm_config_prefix
    [ -x "$p/bin/npm" ] && npm=$p/bin/npm
  fi
  if [ "$("$npm" view "$pkg@$v" version 2>/dev/null)" = "$v" ]; then
    "$npm" install -g "$pkg@$v" >&2 || { echo "npm install -g $pkg@$v 실패" >&2; exit 1; }
    printf 'H\tnpm\n'
  else
    # npm에 없는 판: 플러그인 캐시(빌드된 dist가 든 그 판)를 묶어서 깐다
    t=$(mktemp -d "${TMPDIR:-/tmp}/omc-update.XXXXXX") || exit 1
    f=$(cd "$ip" && "$npm" pack --ignore-scripts --pack-destination "$t" 2>/dev/null | tail -n 1)
    r=1
    [ -n "$f" ] && [ -f "$t/$f" ] && "$npm" install -g "$t/$f" >&2 && r=0
    rm -rf "${t:?}"
    [ "$r" = 0 ] || { echo "npm pack·install 실패 ($ip)" >&2; exit 1; }
    printf 'H\t묶음\n'
  fi
  # 로그인 셸 PATH(바꾸기 전)에서 omc가 안 잡히면 ~/.local/bin(PATH에 있을 때만)에 링크. 링크 아닌 파일은 덮지 않는다
  (PATH=$op; command -v omc >/dev/null 2>&1) && exit 0
  g=$("$npm" prefix -g 2>/dev/null)/bin
  [ -x "$g/omc" ] || { printf 'W\t%s\n' "깐 뒤에도 $g/omc 없음"; exit 0; }
  case ":$op:" in
  *":$HOME/.local/bin:"*) ;;
  *) printf 'W\t%s\n' "~/.local/bin이 PATH에 없어 링크하지 않음 ($g)"; exit 0 ;;
  esac
  mkdir -p "$HOME/.local/bin" || exit 1
  for n in omc oh-my-claudecode omc-cli; do
    [ -e "$g/$n" ] || continue
    d=$HOME/.local/bin/$n
    if [ -e "$d" ] && [ ! -L "$d" ]; then
      printf 'W\t%s\n' "$d 는 링크가 아닌 파일 — 덮지 않음"
      continue
    fi
    ln -sfn "$g/$n" "$d" && printf 'L\t%s\n' "$n"
  done
  ;;
*)
  echo "omc-update 조각: 모르는 일 ${1-}" >&2
  exit 2
  ;;
esac
