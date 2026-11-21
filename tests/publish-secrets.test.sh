#!/bin/bash
# 착지 전 비밀 꼴 검사(2026-11-21 publish-lint-go — Go 시험의 가짜 SHA 40자가 착지한 뒤 공개 내보내기에서야 걸려 릴리스가 막혔다).
# 추적 파일의 내용·경로를 herdr-tools-publish와 같은 scan으로 본다(cmd/herdr-tools-publish/tracked_test.go). .claude/herdr-verify.sh의 싼 검사.
# 스스로 시험: 가짜 40자 hex를 추적한 임시 저장소는 실패해야 한다.
# 실행: bash tests/publish-secrets.test.sh
set -u
cd "$(dirname "$0")/.." || exit 1
gob=$(command -v go 2>/dev/null) || gob=
for a in /opt/homebrew/bin/go /usr/local/go/bin/go /usr/local/bin/go; do [ -n "$gob" ] || [ ! -x "$a" ] || gob=$a; done
[ -n "$gob" ] || { echo "FAIL - go가 없다(brew install go)"; exit 1; }
export GOTOOLCHAIN=local GOCACHE=${GOCACHE:-$PWD/.bin/go-build}
fail=0
scan() { HERDR_SCAN_ROOT=$1 "$gob" test -count=1 -run '^TestTrackedNoSecret$' ./cmd/herdr-tools-publish 2>&1; }

if out=$(scan "$PWD"); then echo "ok - 추적 파일에 비밀 꼴 없음"
else echo "FAIL - 추적 파일에 비밀 꼴(공개 내보내기가 막는다):"; printf '%s\n' "$out" | grep -E '비밀 꼴|가짜 값|못 함'; fail=1; fi

T=$(mktemp -d "${TMPDIR:-/tmp}/publish-secrets.XXXXXX") || exit 1
git -C "$T" init -q
printf 'sha = "%s%s"\n' abc1234000000000000 000000000000000000000 >"$T/fake.txt"
git -C "$T" add -- fake.txt
if out=$(scan "$T"); then echo "FAIL - 가짜 40자 hex를 추적해도 통과"; fail=1
elif printf '%s' "$out" | grep -q 'fake.txt:1: 비밀 꼴'; then echo "ok - 가짜 40자 hex를 추적하면 실패"
else echo "FAIL - 가짜 40자 hex: 다른 까닭으로 실패"; printf '%s\n' "$out" | tail -5; fail=1; fi
rm -rf "${T:?}"
exit $fail
