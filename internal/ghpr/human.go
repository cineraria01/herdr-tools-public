package ghpr

// PR 글 읽기(2026-11-25 ht-pr-human-review, docs/hc-fill/pr.md 「사람 리뷰」): 저장소의 최근 갱신 PR들의 리뷰·줄 댓글·대화 댓글을
// GraphQL 한 번으로 — PR마다 부르지 않는다. 판단(도구 글·허용 작성자·이미 본 글)은 부르는 쪽(hc-fill)이 한다

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Note: PR 글 하나. Kind = changes(변경 요청 리뷰) · approve(승인 리뷰) · review(의견 리뷰 본문) · line(줄 댓글) · comment(대화 댓글)
type Note struct {
	PR     int
	ID     string // 종류 머리 + GitHub 번호(r리뷰·l줄 댓글·c대화 댓글) — 저장소 안에서 하나
	Kind   string
	Author string // 로그인(지워진 계정은 빈 값)
	Bot    bool   // 작성자가 봇 계정
	Body   string
	Where  string // 줄 댓글의 「파일:줄」
	At     string // 쓴 때(UTC, GitHub 값 그대로)
}

// humanQ: 최근 갱신 PR $n개(상태 무관)의 대화 댓글·리뷰·리뷰 줄 댓글(각 끝에서 20개) + gh 로그인 이름
const humanQ = `query($owner:String!,$name:String!,$n:Int!){viewer{login}repository(owner:$owner,name:$name){` +
	`pullRequests(first:$n,orderBy:{field:UPDATED_AT,direction:DESC}){nodes{number ` +
	`comments(last:20){nodes{databaseId body createdAt author{__typename login}}} ` +
	`reviews(last:20){nodes{databaseId state body submittedAt author{__typename login} ` +
	`comments(first:20){nodes{databaseId body path line originalLine createdAt author{__typename login}}}}}}}}}`

type ghActor struct {
	Type  string `json:"__typename"`
	Login string `json:"login"`
}

type ghNode struct {
	DatabaseID   int64    `json:"databaseId"`
	State        string   `json:"state"`
	Body         string   `json:"body"`
	Path         string   `json:"path"`
	Line         *int     `json:"line"`
	OriginalLine *int     `json:"originalLine"`
	CreatedAt    string   `json:"createdAt"`
	SubmittedAt  string   `json:"submittedAt"`
	Author       *ghActor `json:"author"`
	Comments     struct {
		Nodes []ghNode `json:"nodes"`
	} `json:"comments"`
}

// Notes: 최근 갱신 PR n개의 글과 gh 로그인 이름 — gh 호출 한 번. 보류(PENDING)·무른(DISMISSED) 리뷰와 본문 없는 의견 리뷰(줄 댓글 묶음)는 뺀다
func (c *Client) Notes(r Repo, n int) (string, []Note, error) {
	a := []string{"api", "graphql", "-f", "query=" + humanQ, "-f", "owner=" + r.Owner, "-f", "name=" + r.Name, "-F", "n=" + strconv.Itoa(n)}
	if r.Host != "" && r.Host != "github.com" {
		a = append(a, "--hostname", r.Host)
	}
	out, err := c.run("", a...)
	if err != nil {
		return "", nil, err
	}
	return parseNotes(out)
}

func parseNotes(out string) (string, []Note, error) {
	var v struct {
		Data struct {
			Viewer     ghActor `json:"viewer"`
			Repository *struct {
				PullRequests struct {
					Nodes []struct {
						Number   int `json:"number"`
						Comments struct {
							Nodes []ghNode `json:"nodes"`
						} `json:"comments"`
						Reviews struct {
							Nodes []ghNode `json:"nodes"`
						} `json:"reviews"`
					} `json:"nodes"`
				} `json:"pullRequests"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil || v.Data.Repository == nil {
		return "", nil, fmt.Errorf("gh api graphql: PR 글을 못 읽음(%s)", cut(out, 200))
	}
	var ns []Note
	note := func(pr int, kind, pre string, x ghNode, at string) {
		n := Note{PR: pr, ID: pre + strconv.FormatInt(x.DatabaseID, 10), Kind: kind, Body: x.Body, At: at}
		if x.Author != nil {
			n.Author, n.Bot = x.Author.Login, x.Author.Type == "Bot"
		}
		if kind == "line" {
			ln := x.Line
			if ln == nil {
				ln = x.OriginalLine
			}
			n.Where = x.Path
			if ln != nil {
				n.Where += ":" + strconv.Itoa(*ln)
			}
		}
		ns = append(ns, n)
	}
	for _, p := range v.Data.Repository.PullRequests.Nodes {
		for _, x := range p.Comments.Nodes {
			note(p.Number, "comment", "c", x, x.CreatedAt)
		}
		for _, x := range p.Reviews.Nodes {
			switch {
			case x.State == "CHANGES_REQUESTED":
				note(p.Number, "changes", "r", x, x.SubmittedAt)
			case x.State == "APPROVED":
				note(p.Number, "approve", "r", x, x.SubmittedAt)
			case x.State == "COMMENTED" && x.Body != "":
				note(p.Number, "review", "r", x, x.SubmittedAt)
			}
			if x.State == "PENDING" {
				continue
			}
			for _, y := range x.Comments.Nodes {
				note(p.Number, "line", "l", y, y.CreatedAt)
			}
		}
	}
	return v.Data.Viewer.Login, ns, nil
}
