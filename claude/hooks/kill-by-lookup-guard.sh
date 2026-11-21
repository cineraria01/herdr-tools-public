#!/bin/bash
# kill-by-lookup-guard: PreToolUse(Bash) 훅. 포트·이름으로 찾은 프로세스를 한꺼번에 죽이는 명령을 막는다.
# 2026-11-17 mini-m4 레인이 화면 캡처용 포트를 정리하다 launchd가 띄운 포트 포워드 socat 둘에 SIGTERM을 보냈다
# (KeepAlive로 8초 안에 되살아남). 레인 규칙은 「자기가 띄운 프로세스의 pid를 기록해 그 pid만」이다.
# 막는 꼴: lsof -t·pgrep 결과와 kill을 한 명령에(xargs kill · kill $(…) · 변수 경유), ps … | … kill,
#   fuser -k, pkill(-P <부모 pid> 지정은 통과), killall, kill 0 · kill -1(범위 kill).
# 문자열 안 설명(echo·grep 패턴·커밋 메시지·heredoc 본문·주석)은 빼고 본다. 단 bash/sh/zsh -c · eval · ssh ·
# hc exec|run 이 있으면 따옴표 안도 실행될 수 있어 그대로 본다.
# jq·perl이 없거나 입력을 못 읽으면 막지 않는다(편의 막이라 fail-open, hc-lane-wait-guard와 같다).
# Go 판(cmd/herdr-hooks — 같은 판정)으로 넘긴다. HERDR_HOOKS_IMPL=bash(또는 HERDR_IMPL=bash)면 아래 셸 판 그대로(2026-11-21 hooks-go).
# 바이너리가 없거나 소스보다 낡았으면 이번은 셸 판, 짓기는 뒤에서(훅 제한 안에 짓지 않는다 — lib/go-run.sh GO_RUN_BUILD=bg)
# 실행기는 이 파일이 놓인 배치의 것 하나만(저장소 lib/ · 설치 ~/.local/lib/), 없는데 Go 판을 못박았으면 1 — cotenant-guard.sh와 같다
_hd=${BASH_SOURCE[0]%/*} _hg=${BASH_SOURCE[0]%/*}/../../.local/lib/go-run.sh
[ -r "$_hd/../../cmd/herdr-hooks/main.go" ] && _hg=$_hd/../../lib/go-run.sh
if [ -r "$_hg" ]; then . "$_hg" && GO_RUN_BUILD=bg go_run herdr-hooks kill-by-lookup-guard || exit 1
elif [ "${HERDR_HOOKS_IMPL:-${HERDR_IMPL:-auto}}" = go ]; then echo "kill-by-lookup-guard: 공통 실행기가 없다: $_hg (HERDR_HOOKS_IMPL=go — 셸 판으로 넘어가지 않는다)" >&2; exit 1; fi
JQ=/usr/bin/jq; [ -x "$JQ" ] || JQ=$(command -v jq) || exit 0
PERL=/usr/bin/perl; [ -x "$PERL" ] || PERL=$(command -v perl) || exit 0
in=$(cat) || exit 0
cmd=$(printf '%s' "$in" | "$JQ" -r '.tool_input.command // ""' 2>/dev/null) || exit 0
case $cmd in *kill*|*fuser*) ;; *) exit 0 ;; esac
CMD=$cmd "$PERL" -e '
  $_ = $ENV{CMD};
  # heredoc 본문 지움(여는 줄은 남김)
  s/(<<-?[ \t]*([\x27"]?)(\w+)\2[^\n]*)\n(?:.*?\n)?[ \t]*\3[ \t]*(?=\n|$)/$1/gs;
  unless (/(?<![\w.-])(?:bash|sh|zsh|dash)\s+(?:-\w+\s+)*-\w*c\b|(?<![\w.-])(?:eval|ssh)(?![\w.-])|\bhc\s+(?:exec|run)\b/) {
    s/\x27[^\x27]*\x27/\x27\x27/g;
    s{"((?:[^"\\]|\\.)*)"}{ my $q = $1; ($q =~ /\$\(|`/) ? "\"$q\"" : "\"\"" }ge;   # "$(…)"·"`…`"는 실행되니 남김
    s/(^|\s)#[^\n]*/$1/g;
  }
  s/\bhc\s+kill\b/hc_k/g;   # hc kill <주소>는 터미널 주소라 통과
  my $W = qr/(?<![\w.\/-])(?:\S*\/)?/;   # 낱말 앞(경로 /usr/bin/… 허용)
  my $E = qr/(?![\w.-])/;
  my $seg = qr/[^;&|\n)`]*/;
  my $kill = /${W}kill$E/;
  sub out { exit 2 }
  out() if /${W}killall$E/;
  while (/${W}pkill$E($seg)/g) { my $a = $1; out() unless $a =~ /(?:^|\s)(?:-P|--parent)(?:\s|=|\d)/ }
  out() if /${W}fuser$E$seg\s-[a-zA-Z]*k/;
  out() if $kill && /${W}lsof$E$seg\s-[a-zA-Z]*t/;
  if ($kill) { while (/${W}pgrep$E($seg)/g) { my $a = $1; out() unless $a =~ /(?:^|\s)(?:-P|--parent)(?:\s|=|\d)/ } }
  out() if /${W}ps$E[^;&\n]*\|[^;&\n]*${W}kill$E/ || /${W}kill$E[^;&\n]*\$\([^)]*${W}ps$E/;
  while (/${W}kill$E($seg)/g) {
    my @t = grep { length } split /\s+/, $1; my $i = 0;
    if (@t && ($t[0] eq "-s" || $t[0] eq "-n")) { $i = 2 } elsif (@t && $t[0] =~ /^-./ && $t[0] ne "--") { $i = 1 }
    $i++ if $i < @t && $t[$i] eq "--";
    out() if grep { $_ eq "0" || $_ eq "-1" } @t[$i .. $#t];
  }
  exit 0;
'
[ $? = 2 ] || exit 0
echo "포트·이름으로 찾은 프로세스를 죽이지 마세요 — 자기가 띄운 프로세스의 pid를 기록해 그 pid만 kill 하세요(launchd 포워드·다른 레인 서버를 함께 죽입니다)." >&2
exit 2
