#!/bin/bash
# lane-no-shell-edit: PreToolUse(Bash) 훅. 레인 세션($HC_LANE이 있을 때 — hc-lane이 넣는다)에서만 셸로 파일을 고치는 꼴을 막는다.
# 2026-11-18 레인 넷(rh-video·topic-in-card·rh-workflow·make-ui-source)과 rh-server-doc이 heredoc 파이썬·sed·heredoc 커밋·
# zsh ${(f)…} 명령에서 Claude Code 권한 창(「can't be sure it read this command the way the shell will」)에 멈췄다.
# 권한 창보다 먼저 거절해 레인이 바로 Edit·Write 도구로 바꿔 이어 가게 한다. 지휘·사람 세션($HC_LANE 없음)은 늘 통과.
# 막는 꼴(거짓 막음이 적게):
#   1 heredoc 커밋 메시지: git commit 과 << 가 한 명령에(-m "$(cat <<EOF …)" · -F - <<EOF)
#   2 zsh 확장 플래그 ${(f)…} 등(작은따옴표·heredoc 본문 안은 뺌)
#   3 파일 쓰기: python·node·bun·deno 코드가 open(…,'w'|'a'|'x')·.write_text(·.write_bytes(·writeFileSync(·appendFileSync(·
#     Bun.write( 로 씀 / sed -i · sed --in-place / perl -i·-pi / > · >> · tee 로 소스 파일(*.go·*.ts·*.md·*.json 등)에 씀.
#     명령에 임시 폴더(/tmp·/private/·/var/folders·$TMPDIR·mktemp·tempfile·scratchpad)나 .qa/ 가 나오면 3은 통과.
#     > 대상이 변수($f)·/dev/·&2 면 통과. 읽기(sed -n·cat·heredoc 읽기)·시험 실행은 막지 않는다.
# jq·perl이 없거나 입력을 못 읽으면 막지 않는다(편의 막이라 fail-open, kill-by-lookup-guard와 같다).
[ -n "${HC_LANE-}" ] || exit 0
JQ=/usr/bin/jq; [ -x "$JQ" ] || JQ=$(command -v jq) || exit 0
PERL=/usr/bin/perl; [ -x "$PERL" ] || PERL=$(command -v perl) || exit 0
in=$(cat) || exit 0
cmd=$(printf '%s' "$in" | "$JQ" -r '.tool_input.command // ""' 2>/dev/null) || exit 0
[ -n "$cmd" ] || exit 0
CMD=$cmd "$PERL" -e '
  my $raw = $ENV{CMD};
  exit 3 if $raw =~ /\bgit\s(?:[^;&|\n]*\s)?commit\b/ && $raw =~ /<<-?[ \t]*[\x27"]?\w/;
  my $s = $raw;
  s/(<<-?[ \t]*([\x27"]?)(\w+)\2[^\n]*)\n(?:.*?\n)?[ \t]*\3[ \t]*(?=\n|$)/$1/gs for $s;   # heredoc 본문 지움(여는 줄은 남김)
  (my $z = $s) =~ s/\x27[^\x27]*\x27/\x27\x27/g;
  exit 4 if $z =~ /\$\{\([^)]*\)/;
  my $tmp = qr{/tmp\b|/private/|/var/folders/|\$\{?TMPDIR\b|\bmktemp\b|\btempfile\b|\bmkdtemp\b|scratchpad|(?:^|[\s/"\x27=])\.qa/};
  exit 0 if $raw =~ $tmp;
  my $W = qr/(?<![\w.\/-])(?:\S*\/)?/;
  exit 5 if $raw =~ /${W}(?:python[\d.]*|node|bun|deno)(?![\w.-])/
    && $raw =~ /\bopen\([^)]*[\x27"][wax][bt+]*[\x27"]|\.write_(?:text|bytes)\(|\b(?:writeFileSync|appendFileSync)\(|\bBun\.write\(/;
  $s =~ s/(>{1,2}\|?[ \t]*)([\x27"])([^\x27"\n]*)\2/$1$3/g;   # > "a.md" 의 따옴표는 벗겨 대상을 본다
  $s =~ s/\x27[^\x27]*\x27/\x27\x27/g;
  $s =~ s{"((?:[^"\\]|\\.)*)"}{ my $q = $1; ($q =~ /\$\(|`/) ? "\"$q\"" : "\"\"" }ge;
  $s =~ s/(^|\s)#[^\n]*/$1/g;
  my $seg = qr/[^;&|\n]*/;
  exit 5 if $s =~ /${W}sed(?![\w.-])$seg\s(?:-[a-zA-Z]*i|--in-place)/;
  exit 5 if $s =~ /${W}perl(?![\w.-])$seg\s-[pnlew0-9]*i/;
  my $src = qr/\.(?:go|tsx?|jsx?|mjs|cjs|md|json|sh|bash|zsh|py|rs|swift|kt|java|rb|toml|ya?ml|css|scss|html|sql)$/;
  while ($s =~ /(?<![<>&\d])>{1,2}\|?[ \t]*([^\s;&|<>()]+)/g) { my $t = $1; exit 5 if $t !~ /^[\$&]|^\/dev\// && $t =~ $src }
  while ($s =~ /${W}tee(?![\w.-])($seg)/g) { for (split /\s+/, $1) { exit 5 if !/^[-\$]/ && $_ =~ $src } }
  exit 0;
'
case $? in
  3) echo "레인 세션에서는 heredoc 커밋 메시지를 쓰지 마세요 — 메시지를 Write 도구로 파일(스크래치 폴더 등)에 쓰고 git commit -F <파일>. heredoc은 권한 창을 띄워 무인 레인이 멈춥니다." >&2 ;;
  4) echo "레인 세션에서는 zsh 배열·확장(\${(f)…})으로 경로를 만들지 마세요 — git add -- 경로를 글자로 적으세요. 권한 창이 떠 무인 레인이 멈춥니다." >&2 ;;
  5) echo "레인 세션에서는 파일을 셸로 고치지 마세요 — 파일 고치기는 Edit·Write 도구로 하세요. 셸로 고치면 권한 창이 떠 무인 레인이 멈춥니다(읽기·시험 실행·임시 폴더·.qa/ 쓰기는 그대로 됩니다)." >&2 ;;
  *) exit 0 ;;
esac
exit 2
