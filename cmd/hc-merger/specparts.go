package main

// 통과 조각 이어 쓰기(ht-merger-spec-reuse, 2026-11-25 — 추측 검증이 상한에 걸리거나 한 조각만 흔들려 실패하면 병합기가 맥에서 전체
// 검증을 처음부터 다시 돌아 착지 한 번이 약 46분이었다. 그 추측의 다른 조각은 검증 기계에서 이미 통과했다).
//
// 조각(piece)은 검증 스크립트가 정하는 이름(작업 묶음·go 시험 shard 등)이다. 계약:
//   - 일꾼은 검증 스크립트에 HERDR_VERIFY_PARTS=<파일>을 준다. 파일 첫 줄 = 「<트리> <조각…>」 — 그 트리에서 이미 통과한 조각.
//   - 스크립트(따르는 것만)는 시작할 때 HEAD^{tree}가 그 트리면 그 조각을 건너뛰고, 조각이 통과할 때마다 같은 꼴로 파일을 새로 쓴다
//     (건너뛴 조각 포함 — 지금까지 통과한 전부). 따르지 않는 스크립트는 지금 그대로다(파일을 안 읽고 안 씀).
//   - 일꾼은 검증하는 동안 partTick마다, 그리고 통과가 아닌 끝(실패·모름·시간 넘김)에 그 줄을 원격 refs/hc-merger/spec-part/<아이디>
//     (부모 없는 커밋, 트리 = 빈 트리, 본문 첫 줄 = 그 줄)에 올린다. 같은 아이디를 다시 맡은 일꾼(다른 기계여도)은 시작 전에 그것을
//     받아 파일에 깔아 남은 조각만 돈다. 아이디가 다르면(기준·구성원이 바뀜 = 트리가 바뀜) 읽히지 않고, 줄의 트리가 이 추측 트리와
//     다르면 깔지 않는다 — 트리가 바뀌면 이어 쓰지 않는다.
//   - 나눔 몫(specshards.go)은 <아이디>-s<i>에 따로 올리고, 받는 쪽은 <아이디>와 그 몫들의 줄을 이어 붙여 한 줄로 읽는다(specPartsGet).
//   - 참조는 그 추측이 길에서 벗어날 때 spec-fail·spec-run과 함께 치운다(specClean).
//   - 병합기 기계가 직접 검증(착지)하면 맨 앞 추측의 그 줄을 <일지 폴더>/direct.parts에 적어 착지에 HERDR_VERIFY_PARTS_FROM=<파일>로
//     넘긴다(ht-merger-direct-parts — 한 기계에서 시험 하나가 흔들린 추측 뒤 맥이 처음부터 다 돌아 main이 1시간 반 섰다). 착지 트리는
//     추측 트리와 다를 수 있어(문서 접기 등) 트리가 달라도 넘기고, 쓸지는 검증 스크립트가 정한다. 조각이 없으면 빈 값.

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

const specParts = "refs/hc-merger/spec-part/"

// partTick: 검증하는 동안 통과 조각을 올리는 간격(시험이 줄인다)
var partTick = 30 * time.Second

// partPub: 한 일꾼이 올린 통과 조각 — 마지막으로 올린(또는 받은) 줄과 그 커밋(lease)
type partPub struct {
	id, tree, file string
	line, sha      string
	lf             *os.File
}

// specPartsSeed: 앞 실행(나눔 몫들 포함)이 남긴 이 아이디의 통과 조각을 받아 파일에 깐다(같은 트리일 때만). 올리기는 own 참조에
// (몫이면 <아이디>-s<i> — specshards.go). 없거나 못 받으면 빈 파일 없이 처음부터
func specPartsSeed(id, own, tree, file string, lf *os.File) *partPub {
	p := &partPub{id: own, tree: tree, file: file, lf: lf}
	os.Remove(file)
	line, shas, ok := specPartsGet(id, tree)
	p.sha = shas[specParts+own] // 이 값 위로만 고쳐 쓴다(다른 일꾼이 그새 올렸으면 덮지 않는다)
	if !ok {
		return p // 없음(또는 원격 실패) — 처음부터 돈다
	}
	if f := strings.Fields(line); len(f) < 2 || f[0] != tree {
		fmt.Fprintf(lf, "hc-lane merger: 앞 실행의 통과 조각은 다른 트리 것이다 — 이어 쓰지 않는다\n")
		return p
	}
	if err := os.WriteFile(file, []byte(line+"\n"), 0o666); err != nil {
		fmt.Fprintf(lf, "hc-lane merger: 통과 조각 파일을 못 썼다(처음부터 돈다): %v\n", err)
		return p
	}
	p.line = line
	fmt.Fprintf(lf, "hc-lane merger: 앞 실행이 통과한 조각을 이어 쓴다 — %s\n", strings.Join(strings.Fields(line)[1:], " "))
	return p
}

// specPartsGet: 원격 spec-part/<아이디>와 나눔 몫들의 spec-part/<아이디>-s<i> 줄을 이어 붙인 한 줄 「<트리> <조각…>」(트리가 tree인
// 줄만, tree가 빈 값이면 처음 줄의 트리 — 같은 조각 이름은 한 번)과 참조마다의 커밋. 없거나 못 받거나 못 읽으면 ok=false
// (로컬 참조는 남기지 않는다)
func specPartsGet(id, tree string) (line string, shas map[string]string, ok bool) {
	shas = map[string]string{}
	pat := specParts + id
	if fetchr(true, "-q", "origin", "+"+pat+"*:"+pat+"*") != 0 {
		return "", shas, false
	}
	list, rc := outQ("for-each-ref", "--format=%(refname) %(objectname)", specParts)
	if rc != 0 {
		return "", shas, false
	}
	var refs []string
	for _, l := range strings.Split(list, "\n") {
		ref, sha, _ := strings.Cut(l, " ")
		if ref == pat || strings.HasPrefix(ref, pat+"-s") {
			refs, shas[ref] = append(refs, ref), sha
		}
	}
	for _, ref := range refs {
		defer runQ("update-ref", "-d", ref)
	}
	var have []string
	for _, ref := range refs { // for-each-ref 차례 — <아이디> 줄이 몫들보다 앞
		body, rc := outQ("log", "-1", "--format=%b", ref)
		f := strings.Fields(partLine(body))
		if rc != 0 || len(f) < 2 || tree != "" && f[0] != tree {
			continue
		}
		if tree == "" {
			tree = f[0]
		}
		for _, x := range f[1:] {
			if !slices.Contains(have, x) {
				have = append(have, x)
			}
		}
	}
	if len(have) == 0 {
		return "", shas, false
	}
	return tree + " " + strings.Join(have, " "), shas, true
}

// directParts: 병합기 기계가 직접 검증(착지)할 때 맨 앞 추측 id의 통과 조각 줄을 <일지 폴더>/direct.parts에 적고 그 경로
// (착지에 HERDR_VERIFY_PARTS_FROM으로). 조각이 없으면 "". 착지 트리가 추측 트리와 달라도 넘긴다 — 쓸지는 검증 스크립트가 정한다
func (m *M) directParts(id string) string {
	p := m.mj + "/direct.parts"
	os.Remove(p)
	line, _, ok := specPartsGet(id, "")
	if f := strings.Fields(line); !ok || len(f) < 2 {
		return ""
	}
	if err := os.WriteFile(p, []byte(line+"\n"), 0o666); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 통과 조각 파일을 못 썼다(직접 검증은 처음부터): %v\n", err)
		return ""
	}
	fmt.Printf("hc-lane merger: 맨 앞 추측이 통과한 조각을 직접 검증에 넘긴다 — %s (트리 %s, 쓸지는 검증 스크립트가)\n",
		strings.Join(strings.Fields(line)[1:], " "), first8(strings.Fields(line)[0]))
	return p
}

// partLine: 파일·커밋 본문의 첫 줄(앞뒤 빈칸 뺌)
func partLine(s string) string {
	l, _, _ := strings.Cut(strings.TrimLeft(s, "\n"), "\n")
	return strings.TrimSpace(l)
}

// push: 파일 첫 줄이 이 트리의 조각이고 마지막으로 올린 줄과 다르면 원격에 올린다(본 값 lease — 실패는 다음 차례에)
func (p *partPub) push() {
	b, err := os.ReadFile(p.file)
	if err != nil {
		return
	}
	line := partLine(string(b))
	if f := strings.Fields(line); len(f) < 2 || f[0] != p.tree || line == p.line {
		return
	}
	et, rc := outQ("hash-object", "-t", "tree", "-w", "/dev/null")
	if rc != 0 {
		return
	}
	c, ok := commitTree("", et, "", fmt.Sprintf("hc-merger-spec-part id=%s machine=%s at=%d\n\n%s", p.id, machine(), time.Now().Unix(), line))
	if !ok || runQ("push", "-q", "--force-with-lease="+specParts+p.id+":"+p.sha, "origin", c+":"+specParts+p.id) != 0 {
		return
	}
	p.line, p.sha = line, c
	fmt.Fprintf(p.lf, "hc-lane merger: 통과한 조각을 남겼다 — %s\n", strings.Join(strings.Fields(line)[1:], " "))
}

// watch: 검증하는 동안 partTick마다 push. 돌려준 함수를 부르면 멈추고 끝날 때까지 기다린다
func (p *partPub) watch() func() {
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(partTick)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				p.push()
			}
		}
	}()
	return func() { close(stop); <-done }
}
