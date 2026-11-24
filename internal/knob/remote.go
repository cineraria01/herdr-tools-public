package knob

// 다른 기계의 도구가 읽는 길: 원격 참조 Ref → 빈 트리 커밋(날짜 0 — 같은 내용이면 같은 SHA), 제목 remoteHead, 빈 줄 뒤
// 줄마다 「이름\t값\t언제까지(초|-)」. 표에서 Remote인 손잡이만 싣고, 지휘 기계의 hc-fill이 바퀴마다 바뀐 때만 올린다.
// 읽는 쪽은 기한·표·범위를 다시 보고, 못 읽으면 기본값으로 돈다(모르는 값으로 돌지 않는다)

import (
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/gitx"
)

const (
	Ref        = "refs/herdr/knobs"
	remoteHead = "hc-knobs"
)

// FormatRemote: 원격 커밋 메시지. 실을 것이 없으면 빈 글(참조를 지움)
func FormatRemote(es map[string]Entry, now time.Time) string {
	var ns []string
	for n, e := range es {
		if d, ok := Find(n); ok && d.Remote && e.Live(now) {
			ns = append(ns, n)
		}
	}
	if len(ns) == 0 {
		return ""
	}
	sort.Strings(ns)
	var b strings.Builder
	b.WriteString(remoteHead + "\n\n")
	for _, n := range ns {
		u := "-"
		if es[n].Until != 0 {
			u = strconv.FormatInt(es[n].Until, 10)
		}
		b.WriteString(n + "\t" + es[n].Value + "\t" + u + "\n")
	}
	return b.String()
}

// Remote: 원격에서 읽은 값(기한 안·표에 있음·범위 안만)
type Remote map[string]string

// ParseRemote: 원격 커밋 메시지를 읽는다. 꼴이 틀린 줄은 버린다
func ParseRemote(msg string, now time.Time) Remote {
	r := Remote{}
	ls := strings.Split(strings.TrimRight(msg, "\n"), "\n")
	if len(ls) == 0 || ls[0] != remoteHead {
		return r
	}
	for _, l := range ls[1:] {
		f := strings.Split(l, "\t")
		if len(f) != 3 {
			continue
		}
		e := Entry{Name: f[0], Value: f[1]}
		if f[2] != "-" {
			u, err := strconv.ParseInt(f[2], 10, 64)
			if err != nil {
				continue
			}
			e.Until = u
		}
		if d, ok := Find(e.Name); ok && d.Remote && e.Live(now) {
			r[e.Name] = e.Value
		}
	}
	return r
}

// Get: 그 손잡이 값, 없으면 표의 기본
func (r Remote) Get(name string) string {
	if v, ok := r[name]; ok {
		return v
	}
	d, _ := Find(name)
	return d.Default
}

// ReadRemote: dir 클론의 origin에서 Ref를 읽는다(ls-remote → fetch → cat-file). 참조가 없으면 빈 Remote와 gitx.No,
// 못 읽으면 빈 Remote와 gitx.Unknown — 어느 쪽이든 Get은 기본값. 이미 바퀴마다 ls-remote를 하는 도구는 그 SHA로 ReadCommit을 부른다
func ReadRemote(dir string, stderr io.Writer, now time.Time) (Remote, gitx.State) {
	m, st := gitx.LsRemote(dir, "origin", stderr, Ref)
	if st != gitx.Yes {
		return Remote{}, st
	}
	return ReadCommit(dir, m[Ref], stderr, now)
}

// ReadCommit: 원격 참조가 가리키는 SHA의 메시지를 받아 읽는다
func ReadCommit(dir, sha string, stderr io.Writer, now time.Time) (Remote, gitx.State) {
	if !gitx.ObjID(sha) {
		return Remote{}, gitx.Unknown
	}
	if _, rc := gitx.Out(dir, stderr, "cat-file", "-e", sha+"^{commit}"); rc != 0 {
		if gitx.Run(dir, io.Discard, stderr, "fetch", "-q", "origin", "+"+Ref+":"+Ref) != 0 {
			return Remote{}, gitx.Unknown
		}
	}
	body, rc := gitx.Out(dir, stderr, "cat-file", "commit", sha)
	if rc != 0 {
		return Remote{}, gitx.Unknown
	}
	_, msg, _ := strings.Cut(body, "\n\n")
	return ParseRemote(msg, now), gitx.Yes
}
