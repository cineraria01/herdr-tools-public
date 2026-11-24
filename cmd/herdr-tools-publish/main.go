// herdr-tools-publish: 범용 저장소(비공개)의 origin/main 트리를 공개 저장소에 기록 없이 내보낸다.
// (2026-11-21 사용자 「모두 고로 옮기고 싶어」 — ops-go, 셸 판은 2026-11-22 걷음). 인자·출력·종료 코드·검사(fail-closed)는 옛 셸 판 그대로다.
// 다른 점: 개인 값 검사를 곁의 herdr-tools-config 대신 같은 꾸러미(internal/toolcfg)로 한다. 경로 겹침(NFC·대소문자 접기)은
// 표준 라이브러리에 정규화가 없어 python3로 본다(없으면 검사 못 함 — 내보내지 않는다).
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/oplog"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/toolcfg"
	"herdr-tools/internal/version"
)

const (
	me    = "herdr-tools-publish"
	usage = `herdr-tools-publish: 범용 저장소(비공개)의 origin/main 트리를 공개 저장소에 기록 없이 내보낸다.
  herdr-tools-publish            검사 → 공개 main 위에 커밋 하나(「herdr-tools <sha> 내보냄」) → push
  herdr-tools-publish --dry-run  바뀔 파일 목록과 검사 결과만 (push 없음, publish.dir을 만들지 않음)
HERDR_TOOLS_PUBLISH_RESULT=<파일>: 게시가 성공(내보냄·같음, --dry-run 아님)했을 때만 검사한 비공개 커밋 sha 한 줄(그 밖은 파일 없음)
비공개 커밋 메시지·작성자는 싣지 않는다. 공개 트리는 비공개 트리와 같은 객체다(attributes를 거치지 않음). 같으면 「같음」.
막기(fail-closed): 겹치는 경로(대소문자·정규화)나 글이 아닌 블롭(NUL·UTF-8 아님)이 있거나, 블롭 내용·경로·링크 대상·공개 커밋 작성자에 개인 값(herdr-tools-config lint)이나 비밀 꼴이 있거나,
검사를 못 하면 아무것도 push하지 않고 종료 1. 설정 키: publish.repo(없으면 종료 2)·publish.dir·publish.author·publish.secrets.
`
)

var tmp string // 실행마다 만든 임시 폴더 — 끝날 때 지운다

func exit(code int) {
	if tmp != "" {
		os.RemoveAll(tmp)
	}
	os.Exit(code)
}

// sweepStaleTmp: 앞 실행이 SIGKILL로 죽으며 남긴 임시 폴더(<TMPDIR>/herdr-tools-publish.*)를 지운다 — owner의 실행이 끝났다고 확정될 때만
// (pid가 없거나 같은 pid의 시작 시각이 다름). owner가 없거나 모르면(ps 실패) 두고 넘어간다
func sweepStaleTmp() {
	base := os.Getenv("TMPDIR")
	if base == "" {
		base = os.TempDir()
	}
	ds, err := filepath.Glob(filepath.Join(base, me+".*"))
	if err != nil {
		return
	}
	for _, d := range ds {
		b, err := os.ReadFile(d + "/owner")
		if err != nil {
			continue
		}
		f := strings.SplitN(strings.TrimRight(string(b), "\n"), "\t", 2)
		pid, err := strconv.Atoi(f[0])
		if err != nil || len(f) != 2 || pid <= 1 || pid == os.Getpid() {
			continue
		}
		if proc.Alive(pid) {
			st, err := proc.StartTime(pid)
			if err != nil || st == f[1] {
				continue // 도는 중이거나 모름
			}
		}
		os.RemoveAll(d)
	}
}

// startTime: 자기 시작 시각(시험이 바꾼다)
var startTime = proc.StartTime

// mkOwnedTmp: 자기 임시 폴더(<TMPDIR>/herdr-tools-publish.*)를 만들고 소유 기록 owner(pid·시작 시각)를 쓴다. 기록을 못 만들면 그 빈
// 폴더만 지우고 오류 — 블롭은 기록이 있는 폴더에만 꺼낸다(재감사 ops/R-213: 시작 시각 조회가 실패해도 기록 없이 블롭을 꺼내, SIGKILL
// 뒤 다음 실행이 소유를 몰라 영영 남겼다)
func mkOwnedTmp() (string, error) {
	d, err := os.MkdirTemp(os.Getenv("TMPDIR"), me+".")
	if err != nil {
		return "", err
	}
	st, err := startTime(os.Getpid())
	if err == nil && (st == "" || strings.ContainsAny(st, "\t\n")) {
		err = fmt.Errorf("시작 시각 꼴이 틀렸다: %q", st)
	}
	if err == nil {
		if err = os.WriteFile(d+"/owner", []byte(fmt.Sprintf("%d\t%s\n", os.Getpid(), st)), 0o600); err != nil {
			os.Remove(d + "/owner")
		}
	}
	if err != nil {
		if rerr := os.Remove(d); rerr != nil {
			err = fmt.Errorf("%w (빈 임시 폴더도 못 지움: %v)", err, rerr)
		}
		return "", fmt.Errorf("소유 기록을 만들지 못했다: %w", err)
	}
	return d, nil
}

func die(f string, a ...any) {
	fmt.Fprintf(os.Stderr, me+": "+f+"\n", a...)
	oplog.Log(me, "", oplog.Die, 1, fmt.Sprintf(f, a...))
	exit(1)
}

// git: 표준 출력(끝 줄바꿈 뺌)과 성공. 표준 오류는 그대로 흘린다
func git(args ...string) (string, bool) {
	out, rc := gitx.Out("", os.Stderr, args...)
	return out, rc == 0
}

func masked(p string) string { return filepath.Dir(p) + "/(이름 가림)" } // 이름이 곧 값일 수 있어 막을 때는 폴더만

// main: 단계는 steps.go
func main() {
	dry := false
	switch {
	case len(os.Args) == 1:
	case len(os.Args) == 2 && os.Args[1] == "--dry-run":
		dry = true
	case len(os.Args) == 2 && os.Args[1] == "--version":
		fmt.Println(version.String(me))
		return
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	// 결과 파일(A02): 낡은 값을 남기지 않게 먼저 지운다 — 게시가 성공했을 때만 다시 쓴다(done)
	res := os.Getenv("HERDR_TOOLS_PUBLISH_RESULT")
	if res != "" {
		if err := os.Remove(res); err != nil && !os.IsNotExist(err) {
			die("결과 파일을 지우지 못했다: %v", err)
		}
	}
	c := loadConfig()
	src, tree, short := sourceTree(c.repo)
	sweepStaleTmp()
	t0, err := mkOwnedTmp()
	if err != nil {
		die("임시 폴더를 만들지 못했다 — 게시하지 않는다: %v", err)
	}
	tmp = t0
	// 신호로 끝날 때도 자식 무리를 거둔 뒤 이 임시 폴더를 지운다(재감사 ops R-10 — 옛 판은 TERM이면 남겼다). SIGKILL로 남은 것은
	// 다음 실행이 owner(pid·시작 시각)로 소유를 확인해 지운다(sweepStaleTmp)
	proc.OnStop(func() { os.RemoveAll(t0) })

	s := readTree(c.repo, tree)
	bad := s.bad
	if overlaps(s.names) {
		bad = true
	}
	if s.nonText() {
		bad = true
	}
	if s.leaks(c) {
		bad = true
	}
	// 막히면 --dry-run도 여기서 끝낸다(B13): 뒤의 변경 목록(diff-tree)이 가린 이름을 원래 경로로 찍지 않게
	if bad {
		blocked(short)
	}
	dir, idir := publicDir(c.dir, c.url, dry)
	checkIdent(c, idir, short)
	fmt.Printf("검사 통과 (비공개 %s, 파일 %d개)\n", short, len(s.paths))
	export(c, dir, res, src, tree, short, s.shas, dry)
}

func lines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

var secretPats = []string{`-----BEGIN[A-Z0-9 ]*PRIVATE KEY`, `gh[pousr]_[A-Za-z0-9]{30,}`, `github_pat_[A-Za-z0-9_]{30,}`,
	`(^|[^A-Za-z0-9])sk-[A-Za-z0-9_-]{30,}`, `xox[abprs]-[A-Za-z0-9-]{10,}`, `AKIA[0-9A-Z]{16}`, `AIza[0-9A-Za-z_-]{35}`,
	`(^|[^0-9A-Fa-f])[0-9A-Fa-f]{40,}`}

var (
	tokRe   = regexp.MustCompile(`[A-Za-z0-9+/=_-]{32,}`)
	upRe    = regexp.MustCompile(`[A-Z]`)
	lowRe   = regexp.MustCompile(`[a-z]`)
	digitRe = regexp.MustCompile(`[0-9]`)
	alphaRe = regexp.MustCompile(`ABCDEFGH|abcdefgh|0123456789`)
)

// scan: 걸린 곳마다 「파일:줄: 까닭」(값은 안 찍음). 못 읽거나 설정 꼴이 틀리면 거짓(검사 못 함)
func scan(files []string, extra string) ([]string, bool) {
	pats := append([]string(nil), secretPats...)
	if extra != "" {
		pats = append(pats, extra)
	}
	res := make([]*toolcfg.ERE, len(pats))
	for i, p := range pats {
		re, err := toolcfg.CompileERE(p, false) // 셸 판 LC_ALL=C grep -E와 같은 ERE·바이트 판정
		if err != nil {
			return nil, false
		}
		res[i] = re
	}
	var hits []string
	for _, f := range files {
		d, err := os.ReadFile(f)
		if err != nil {
			return nil, false
		}
		ls := bytes.Split(d, []byte("\n"))
		if len(ls) > 0 && len(ls[len(ls)-1]) == 0 {
			ls = ls[:len(ls)-1]
		}
		for i, l := range ls {
			for j, re := range res {
				if re.Match(l) {
					hits = append(hits, fmt.Sprintf("%s:%d: 비밀 꼴 %s", f, i+1, pats[j]))
				}
			}
			// 긴 base64 꼴 토큰: 32자 넘게 이어지고 대·소문자·숫자가 다 있음. 알파벳 목록(ABCDEFGH…·0123456789)은 뺀다
			for _, tok := range tokRe.FindAll(l, -1) {
				if upRe.Match(tok) && lowRe.Match(tok) && digitRe.Match(tok) && !alphaRe.Match(tok) {
					hits = append(hits, fmt.Sprintf("%s:%d: 비밀 꼴 긴 토큰", f, i+1))
				}
			}
		}
	}
	return hits, true
}
