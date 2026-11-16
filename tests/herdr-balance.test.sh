#!/bin/bash
# herdr-balance 시험: balance.remote 읽기(설정 있음·파일 없음·키 없음·도우미 없음). herdr-app·실제 설정은 쓰지 않는다.
# 실행: bash tests/herdr-balance.test.sh
set -u
here=$(cd "$(dirname "$0")/.." && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/hb-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
trap 'rm -rf "$T"' EXIT
mkdir -p "$T/bin" "$T/home" "$T/conf"
ln -s "$here/bin/herdr-tools-config" "$T/bin/herdr-tools-config"
PY=$(command -v python3) || { echo "python3 없음"; exit 1; }
# remote <PATH> <설정 폴더>: remote_prefixes()의 결과를 「a|b」로, 경고는 $T/err
remote() {
  env -i PYTHONDONTWRITEBYTECODE=1 HOME="$T/home" PATH="$1" HERDR_TOOLS_CONFIG="$2" "$PY" -c '
import importlib.machinery, importlib.util, sys
p = sys.argv[1]
spec = importlib.util.spec_from_file_location("hb", p, loader=importlib.machinery.SourceFileLoader("hb", p))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
print("|".join(m.remote_prefixes()))' "$here/bin/herdr-balance" 2>"$T/err"
}
n=0 fails=0
t() { n=$((n + 1)); if [ "$2" = "$3" ]; then echo "ok $n - $1"; else fails=$((fails + 1)); echo "FAIL $n - $1: [$2] != [$3] err=[$(cat "$T/err")]"; fi; }

t "예시 설정: 목록" "$(remote "$T/bin:/usr/bin:/bin" "$here/config.example")" "worker-a/|worker-b/"
t "예시 설정: 경고 없음" "$(cat "$T/err")" ""
t "설정 파일 없음: 빈 목록" "$(remote "$T/bin:/usr/bin:/bin" "$T/conf")" ""
t "설정 파일 없음: 경고 한 줄" "$(grep -c '설정 파일 없음' "$T/err")" 1
printf 'sync.prebuild true\n' >"$T/conf/config"
t "키만 없음: 빈 목록" "$(remote "$T/bin:/usr/bin:/bin" "$T/conf")" ""
t "키만 없음: 경고 없음" "$(cat "$T/err")" ""
printf 'balance.remote a/\nbalance.remote b/\n' >"$T/conf/config"
t "같은 키 두 번: 빈 목록" "$(remote "$T/bin:/usr/bin:/bin" "$T/conf")" ""
t "같은 키 두 번: 경고 한 줄" "$(grep -c 'balance.remote 못 읽음' "$T/err")" 1
t "도우미가 PATH·~/.local/bin에 없음: 빈 목록" "$(remote "/usr/bin:/bin" "$here/config.example")" ""
t "도우미 없음: 경고 한 줄" "$(grep -c 'herdr-tools-config 못 부름' "$T/err")" 1
mkdir -p "$T/home/.local/bin" && ln -s "$here/bin/herdr-tools-config" "$T/home/.local/bin/herdr-tools-config"
t "PATH에 없어도 ~/.local/bin의 도우미(launchd)" "$(remote "/usr/bin:/bin" "$here/config.example")" "worker-a/|worker-b/"

echo "${n}개 중 실패 ${fails}개"
[ $fails = 0 ]
