// Package jobtmp: hc가 띄우는 작업마다 자기 임시 폴더(TMPDIR)를 주는 감싸개(ht-job-tmpdir, 2026-11-27 사용자 「작업별 임시 폴더」).
// 폴더는 <뿌리>/<6글자>이고 안에 표식 .hc-job 세 줄「<터미널 주소>」「<작업 이름>」「<유닉스 초>」 — 누구 것인지를 이름이 아니라 자리로 안다.
// 지우는 쪽은 hc-sweep(닫은 터미널·주인 없는 폴더)과 run 꼴 감싸개 자신이다(docs/hc-sweep/job-tmp.md).
package jobtmp

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	Key     = "jobtmp.root"                              // 설정 키(뿌리, off면 끔)
	Default = "~/.hctmp.noindex"                         // .noindex: Spotlight가 색인하지 않음
	Marker  = ".hc-job"                                  // 작업 폴더 안 표식
	MaxRoot = 41                                         // TMPDIR 값 <뿌리>/XXXXXX/가 macOS 기본 TMPDIR(/var/folders/xx/<30>/T/ 49자)보다 길지 않게
	MaxDir  = MaxRoot + 1 + 6                            // 48(작업 폴더, macOS TMPDIR 값은 끝 「/」 더해 49)
	SockMax = 104                                        // macOS sun_path(NUL 포함)
	Longest = "com.google.Chrome.XXXXXX/SingletonSocket" // 알려진 가장 긴 소켓 꼴(화면 점검의 Chrome)
)

// script: sh -c <script> jobtmp <뿌리> <이름> <exec|run> <명령…>. hc 터미널 안($HC_TERMINAL_ID)에서만 폴더를 만든다 — 주인을 모르는
// 폴더는 만들지 않는다. 못 만들면 한 줄 알리고 기본 TMPDIR로 돈다(작업은 막지 않는다). run 꼴은 명령이 끝나거나 HUP·TERM·INT를
// 받으면 폴더를 지운다(병합기 일꾼 — --rm 터미널). exec 꼴은 명령이 그대로 터미널의 프로그램(에이전트 판정이 바뀌지 않게)이고 hc-sweep이 지운다.
// 한 줄로 붙여 넘기고 역슬래시를 쓰지 않는다(scriptSrc의 줄은 모두 ;·then·else·&&로 끝난다) — hc run 인자를 한 줄로 적고 읽는 기록·
// 시험 대역이 깨지지 않게(macOS /bin/sh의 echo는 \t·\n을 풀어 줄을 끊는다)
// (빈칸 여럿도 하나로 — 따옴표 안 글에 겹친 빈칸이 없다)
var script = strings.Join(strings.Fields(scriptSrc), " ")

const scriptSrc = `r=$1 n=$2 m=$3; shift 3;
case $r in "~") r=$HOME ;; "~/"*) r=$HOME/${r#"~/"} ;; esac;
d=;
if [ -n "${HC_TERMINAL_ID-}" ] && [ "$r" != off ]; then
  if [ ${#r} -gt 41 ]; then
    echo "jobtmp: 뿌리 $r 가 41자를 넘어(소켓 경로 상한) 기본 TMPDIR로 돈다" >&2;
  elif { [ -d "$r" ] || { mkdir -p "$r" && chmod 700 "$r"; }; } &&
    d=$(mktemp -d "$r/XXXXXX") && { printf %s "$HC_TERMINAL_ID"; echo; printf %s "$n"; echo; date +%s; } >"$d/.hc-job"; then
    TMPDIR=$d; export TMPDIR;
    [ "$(uname)" != Darwin ] || { TMPDIR=$d/; MAC_CHROMIUM_TMPDIR=$d/; export MAC_CHROMIUM_TMPDIR; };
  else
    [ -z "$d" ] || rm -rf "$d";
    d=;
    echo "jobtmp: 작업 임시 폴더를 못 만듦($r) — 기본 TMPDIR로 돈다" >&2;
  fi;
fi;
if [ "$m" = run ] && [ -n "$d" ]; then
  trap 'rm -rf "$d"; exit 129' HUP TERM INT;
  "$@"; s=$?; rm -rf "$d"; exit $s;
fi;
exec "$@"`

// Wrap: argv 앞에 감싸개를 붙인다. run이면 명령이 끝날 때 폴더를 지우는 꼴. 뿌리가 비었거나 off, cwd가 Windows 꼴(C:\…)이면 argv 그대로
func Wrap(root, name, cwd string, run bool, argv []string) []string {
	if root == "" || root == "off" || Windows(cwd) || len(argv) == 0 {
		return argv
	}
	m := "exec"
	if run {
		m = "run"
	}
	return append([]string{"sh", "-c", script, "jobtmp", root, name, m}, argv...)
}

// Windows: cwd가 Windows 경로 꼴인가(C:\… · C:/… · 역슬래시)
func Windows(cwd string) bool {
	return len(cwd) >= 2 && cwd[1] == ':' || strings.Contains(cwd, `\`)
}

// Expand: 뿌리의 ~를 home으로. off·빈 값은 ""(끔)
func Expand(root, home string) string {
	switch {
	case root == "" || root == "off":
		return ""
	case root == "~":
		return home
	case strings.HasPrefix(root, "~/"):
		return filepath.Join(home, root[2:])
	}
	return root
}

// Owner: 작업 폴더 dir의 표식(주소·이름·유닉스 초). 못 읽거나 꼴이 틀리면 ok false
func Owner(dir string) (addr, name string, at int64, ok bool) {
	b, err := os.ReadFile(filepath.Join(dir, Marker))
	if err != nil {
		return "", "", 0, false
	}
	f := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(f) != 3 || f[0] == "" {
		return "", "", 0, false
	}
	var n int64
	for _, c := range f[2] {
		if c < '0' || c > '9' {
			return "", "", 0, false
		}
		n = n*10 + int64(c-'0')
	}
	return f[0], f[1], n, f[2] != ""
}
