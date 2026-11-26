package main

// 제출 기록(refs/merge-queue-meta/<이름> — 빈 트리 커밋, 부모 = 원본, 메시지 첫 줄에 key=value) 읽기와 main push에 함께 싣는 인자.

import (
	"fmt"
	"regexp"
	"strings"
)

type meta struct {
	base, info, owner, ownerRun, host, term, born, id, job, jobref, verdict, patch string
	pred, pp, sbad                                                                 string // 쌓인 제출(stack=<선행>@<P>)의 선행·P, 꼴이 틀리면 반송 까닭(stack.go)
}

var (
	verdictChars = regexp.MustCompile(`^[A-Za-z0-9_-]*$`)
	sha40        = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// readMeta <기록 SHA|빈 값> <제출 SHA>: 기록이 없으면(손으로 넣은 큐 표식) 빈 값으로 ok. 읽기 실패·부모가 제출 SHA가 아님·
// 표식 이름을 못 씀이면 ok=false(git 실패를 「없음」으로 읽지 않는다)
func readMeta(rec, sub string) (meta, bool) {
	var mt meta
	if rec == "" || rec == "-" {
		return mt, true
	}
	if p, rc := outQ("rev-parse", "-q", "--verify", rec+"^"); rc != 0 || p != sub {
		return meta{}, false
	}
	body, rc := outQ("log", "-1", "--format=%B", rec)
	if rc != 0 {
		return meta{}, false
	}
	line, _, _ := strings.Cut(body, "\n")
	var ids, jobs []string
	fork := ""
	for _, kv := range strings.Fields(line) {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "base":
			mt.base = v
		case "owner":
			mt.owner = v
		case "host":
			mt.host = v
		case "term":
			mt.term = v
		case "owner_run":
			mt.ownerRun = v
		case "created_ms":
			mt.born = v
		case "verdict":
			mt.verdict = v
		case "patch":
			mt.patch = v
		case "id":
			ids = append(ids, v)
		case "job":
			jobs = append(jobs, v)
		case "fork":
			fork = first8(v)
		case "stack":
			mt.pred, mt.pp, _ = strings.Cut(v, "@")
			if !laneOK(mt.pred) || !sha40.MatchString(mt.pp) {
				mt.sbad = "쌓임 기록 꼴(stack=" + v + ")"
			}
		}
	}
	if !verdictChars.MatchString(mt.verdict) {
		return meta{}, false
	}
	if mt.verdict+mt.patch != "" && (mt.verdict == "" || mt.patch != "-" && !sha40.MatchString(mt.patch)) {
		return meta{}, false
	}
	if mt.owner == "-" {
		mt.owner = ""
	}
	mt.id, mt.job = strings.Join(ids, "\n"), strings.Join(jobs, "\n")
	if len(ids) > 0 && ids[len(ids)-1] != "" {
		mt.info = fmt.Sprintf("제출 ID %s, 갈래점 %s", ids[len(ids)-1], fork)
	}
	// 제출 ID 표식은 필수다. 선택적인 작업 ID는 이름이 유효하지 않을 때만 건너뛴다
	if mt.id != "" && runQ("check-ref-format", "refs/hc-landed/"+mt.id) != 0 {
		return meta{}, false
	}
	if mt.job != "" && mt.job != "-" {
		switch runQ("check-ref-format", "refs/hc-landed/"+mt.job) {
		case 0:
			mt.jobref = "refs/hc-landed/" + mt.job
		case 1:
		default:
			return meta{}, false
		}
	}
	return mt, true
}

// key: 그 제출의 결과를 가리는 열쇠 — 제출 기록이 있으면 「제출 ID <ID>」, 없으면 「큐 <이름>」
func (mt meta) key(n string) string {
	if mt.id != "" {
		return "제출 ID " + mt.id
	}
	return "큐 " + n
}

// infoTail: 사건 글 끝에 붙이는 「 (갈래점 xxxxxxxx)」
func (mt meta) infoTail() string {
	if mt.info == "" {
		return ""
	}
	_, after, found := strings.Cut(mt.info, ", ")
	if !found {
		after = mt.info
	}
	return " (" + after + ")"
}

// exbuild: main push에 함께 실을 인자(HERDR_LAND_PUSH_EXTRA — 착지가 그대로 git push 인자로): 큐 표식 삭제 + 그 SHA lease,
// 기록을 완료 참조로 옮김, lane 가지가 그 제출이면 지움, 제출 ID 표식 HEAD:refs/hc-landed/<ID>, 작업 표식(원격에 아직 없을 때만)
func (m *M) exbuild() {
	var p []string
	for _, e := range m.b {
		p = append(p, ":"+e.q, "--force-with-lease="+e.q+":"+e.s)
		if e.m != "-" {
			name := e.q[strings.LastIndex(e.q, "/")+1:]
			mref, done, lane := "refs/merge-queue-meta/"+name, "refs/merge-done/"+e.l, "refs/heads/lane/"+e.l
			p = append(p, ":"+mref, "--force-with-lease="+mref+":"+e.m, e.m+":"+done, "--force-with-lease="+done+":"+refsha(m.qs, done))
			if refsha(m.qs, lane) == e.s {
				p = append(p, ":"+lane, "--force-with-lease="+lane+":"+e.s)
			}
		}
		if e.d != "" {
			p = append(p, "HEAD:refs/hc-landed/"+e.d)
		}
	}
	if m.mjx != "" {
		p = append(p, m.mjx)
	}
	m.ex = strings.Join(p, " ")
}
