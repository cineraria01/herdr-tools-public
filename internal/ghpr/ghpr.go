// Package ghpr: PR 거울(2026-11-24 pr-surface, docs/hc-fill/pr.md)이 gh를 부르는 한 곳. PR 열기·댓글·커밋 상태·닫기만 한다 —
// PR에서 무엇을 읽어 판단하지 않는다(댓글 숨은 표식 확인만 — 같은 댓글을 두 번 달지 않으려고).
// 호출마다 시간 상한(Client.Timeout), 한 Client의 호출 수 상한(Client.Max — 넘으면 ErrBudget, 부르지 않는다).
package ghpr

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/proc"
)

// ErrBudget: 이 바퀴의 호출 수 상한에 닿았다 — 부르지 않았다(다음 바퀴에)
var ErrBudget = errors.New("gh 호출 상한")

// ErrTimeout: 호출 하나가 시간 상한을 넘겼다(결과 모름) — 부르는 쪽은 그 바퀴를 멈춘다(느린 gh가 바퀴를 묶지 않게)
var ErrTimeout = errors.New("시간 넘김")

// Available: gh 실행 파일이 있나
func (c *Client) Available() bool {
	bin := c.Bin
	if bin == "" {
		bin = "gh"
	}
	_, err := exec.LookPath(bin)
	return err == nil
}

// Take: gh가 아닌 바깥 일(레인 기계의 커밋 올리기 등)도 같은 호출 상한에 한 번으로 센다. 상한이면 거짓(세지 않음)
func (c *Client) Take() bool {
	if c.Max > 0 && *c.n() >= c.Max {
		return false
	}
	*c.n()++
	return true
}

// ErrNoGh: gh 실행 파일이 없다
var ErrNoGh = errors.New("gh 없음")

// Client: 호출 수(Calls)는 As로 만든 짝과 함께 센다(상한에 막힌 것은 안 셈)
type Client struct {
	Bin     string        // 빈 값이면 "gh"
	Timeout time.Duration // 호출 하나의 상한(0이면 30초)
	Max     int           // 호출 수 상한(0이면 없음)
	Token   string        // 있으면 GH_TOKEN으로(기본 gh 로그인 대신). 출력·오류 글에 찍지 않는다
	used    *int
}

func (c *Client) n() *int {
	if c.used == nil {
		c.used = new(int)
	}
	return c.used
}

// Calls: 실제로 부른 수
func (c *Client) Calls() int { return *c.n() }

// As: 같은 상한·같은 호출 수를 나누되 다른 인증(token — 빈 값이면 기본 로그인)으로 부르는 짝
func (c *Client) As(token string) *Client {
	d := *c
	d.used, d.Token = c.n(), token
	return &d
}

// scrub: 오류 글에서 토큰 값을 지운다
func (c *Client) scrub(s string) string {
	if c.Token == "" {
		return s
	}
	return strings.ReplaceAll(s, c.Token, "‹토큰›")
}

// Repo: 원격 주소의 GitHub 저장소 — gh의 --repo 값([호스트/]주인/이름)
type Repo struct{ Host, Owner, Name string }

func (r Repo) Arg() string {
	if r.Host == "" || r.Host == "github.com" {
		return r.Owner + "/" + r.Name
	}
	return r.Host + "/" + r.Owner + "/" + r.Name
}

func (r Repo) api() string { return "repos/" + r.Owner + "/" + r.Name }

var (
	scpRe = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):([^/]+)/([^/]+?)(?:\.git)?/?$`)
	urlRe = regexp.MustCompile(`^(?:https?|ssh|git)://(?:[^@/]+@)?([^:/]+)(?::[0-9]+)?/([^/]+)/([^/]+?)(?:\.git)?/?$`)
)

// ParseRemote: git 원격 주소(https·ssh·scp 꼴)에서 저장소. 로컬 경로·file: 주소·주인/이름 두 칸이 아닌 꼴은 false
func ParseRemote(u string) (Repo, bool) {
	u = strings.TrimSpace(u)
	if u == "" || strings.HasPrefix(u, "/") || strings.HasPrefix(u, ".") || strings.HasPrefix(u, "file:") {
		return Repo{}, false
	}
	m := urlRe.FindStringSubmatch(u)
	if m == nil && !strings.Contains(u, "://") {
		m = scpRe.FindStringSubmatch(u)
	}
	if m == nil || m[2] == "" || m[3] == "" {
		return Repo{}, false
	}
	return Repo{Host: strings.ToLower(m[1]), Owner: m[2], Name: m[3]}, true
}

// run: gh <args>(stdin 있으면 그 글). 출력(표준 출력)과 오류(표준 오류를 담음)
func (c *Client) run(stdin string, args ...string) (string, error) {
	if c.Max > 0 && *c.n() >= c.Max {
		return "", ErrBudget
	}
	bin := c.Bin
	if bin == "" {
		bin = "gh"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return "", ErrNoGh
	}
	*c.n()++
	to := c.Timeout
	if to <= 0 {
		to = 30 * time.Second
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(cmd.Environ(), "GH_PROMPT_DISABLED=1", "NO_COLOR=1")
	if c.Token != "" {
		cmd.Env = append(cmd.Env, "GH_TOKEN="+c.Token, "GH_ENTERPRISE_TOKEN="+c.Token)
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := proc.RunMax(cmd, to) // 넘기면 무리째 거둔다 — 결과는 모름(오류)
	if errors.Is(err, proc.ErrTimeout) {
		return "", fmt.Errorf("%w: gh %s %v", ErrTimeout, args[0], to)
	}
	if err != nil {
		return c.scrub(out.String()), fmt.Errorf("gh %s: %v: %s", strings.Join(args[:min(2, len(args))], " "), err, cut(c.scrub(strings.TrimSpace(errb.String())), 300))
	}
	return c.scrub(out.String()), nil
}

// SetRef: 가지 refs/heads/<branch>를 sha로(없으면 만들고, 있으면 강제로 옮긴다). 커밋은 이미 원격에 있어야 한다
func (c *Client) SetRef(r Repo, branch, sha string) error {
	host := func(a []string) []string {
		if r.Host != "" && r.Host != "github.com" {
			a = append(a, "--hostname", r.Host)
		}
		return a
	}
	_, err := c.run("", host([]string{"api", "-X", "PATCH", r.api() + "/git/refs/heads/" + branch, "-f", "sha=" + sha, "-F", "force=true"})...)
	if err == nil || !strings.Contains(err.Error(), "does not exist") && !strings.Contains(err.Error(), "Not Found") {
		return err
	}
	_, err = c.run("", host([]string{"api", "-X", "POST", r.api() + "/git/refs", "-f", "ref=refs/heads/" + branch, "-f", "sha=" + sha})...)
	return err
}

// Review: 진짜 리뷰(승인 또는 변경 요청). 자기 PR이면 GitHub가 거절한다 — ErrOwnPR
func (c *Client) Review(r Repo, num int, approve bool, body string) error {
	k := "--request-changes"
	if approve {
		k = "--approve"
	}
	_, err := c.run(body, "pr", "review", strconv.Itoa(num), "--repo", r.Arg(), k, "--body-file", "-")
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "own pull request") {
		return ErrOwnPR
	}
	return err
}

// ErrOwnPR: 자기 PR은 승인·변경 요청을 할 수 없다
var ErrOwnPR = errors.New("자기 PR이라 리뷰 불가")

func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

var prURLRe = regexp.MustCompile(`https?://\S+/pull/([0-9]+)`)

// PR: 번호와 주소
type PR struct {
	Num int
	URL string
}

// Create: PR을 연다. 이미 열린 PR이 그 머리에 있으면(gh가 「already exists」와 주소를 알림) 그 PR을 돌려준다
func (c *Client) Create(r Repo, base, head, title, body string) (PR, error) {
	out, err := c.run(body, "pr", "create", "--repo", r.Arg(), "--base", base, "--head", head, "--title", title, "--body-file", "-")
	if m := prURLRe.FindStringSubmatch(out); m != nil {
		n, _ := strconv.Atoi(m[1])
		return PR{n, m[0]}, nil
	}
	if err != nil && strings.Contains(err.Error(), "already exists") {
		if m := prURLRe.FindStringSubmatch(err.Error()); m != nil {
			n, _ := strconv.Atoi(m[1])
			return PR{n, m[0]}, nil
		}
	}
	if err == nil {
		err = fmt.Errorf("gh pr create: 주소를 못 읽음(%s)", cut(strings.TrimSpace(out), 200))
	}
	return PR{}, err
}

// Comment: 댓글 하나
func (c *Client) Comment(r Repo, num int, body string) error {
	_, err := c.run(body, "pr", "comment", strconv.Itoa(num), "--repo", r.Arg(), "--body-file", "-")
	return err
}

// HasMarker: 그 PR 댓글에 표식 글자가 이미 있나(앞 시도가 응답 없이 끝났을 때만 — 두 번 달지 않으려고)
func (c *Client) HasMarker(r Repo, num int, marker string) (bool, error) {
	return c.hasIn(r, "/issues/"+strconv.Itoa(num)+"/comments", marker)
}

// HasReviewMarker: 그 PR 리뷰 본문에 표식 글자가 이미 있나
func (c *Client) HasReviewMarker(r Repo, num int, marker string) (bool, error) {
	return c.hasIn(r, "/pulls/"+strconv.Itoa(num)+"/reviews", marker)
}

func (c *Client) hasIn(r Repo, path, marker string) (bool, error) {
	a := []string{"api", "--paginate", r.api() + path, "--jq", ".[].body"}
	if r.Host != "" && r.Host != "github.com" {
		a = append(a, "--hostname", r.Host)
	}
	out, err := c.run("", a...)
	if err != nil {
		return false, err
	}
	return strings.Contains(out, marker), nil
}

// Status: 커밋 상태(state = success·failure·pending·error). 설명은 140자 안으로 자른다(GitHub 상한)
func (c *Client) Status(r Repo, sha, state, context, desc string) error {
	a := []string{"api", "-X", "POST", r.api() + "/statuses/" + sha, "-f", "state=" + state, "-f", "context=" + context, "-f", "description=" + cut(desc, 139)}
	if r.Host != "" && r.Host != "github.com" {
		a = append(a, "--hostname", r.Host)
	}
	_, err := c.run("", a...)
	return err
}

// Close: PR을 닫는다. 이미 닫혔거나 이미 들어간(Merged) PR이면 성공으로 본다
func (c *Client) Close(r Repo, num int) error {
	_, err := c.run("", "pr", "close", strconv.Itoa(num), "--repo", r.Arg())
	if err != nil && (strings.Contains(err.Error(), "already closed") || strings.Contains(err.Error(), "already merged")) {
		return nil
	}
	return err
}
