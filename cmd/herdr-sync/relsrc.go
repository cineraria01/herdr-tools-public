package main

// 릴리스·깔기가 쓰는 소스 꺼내기와 판 계산(release.go에서 나눔 — 줄 예산)

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/cksum"
	"herdr-tools/internal/proc"
)

// srcDigest: 모듈 소스 판 — go.mod·cmd/**·internal/** 의 일반 파일 전부(go:embed 글 포함 — 임베드만 바뀐 판도 알아보게, gofix-core B10·M4)를
// 상대 경로 바이트 순으로 「crc 크기 경로」 줄로 적은 목록의 cksum(crc.크기). 숨김 파일·숨김 폴더 아래는 뺀다(herdr-sync 나무 줄이 걷지 않아 기계에 없다).
// installSh의 cur(`find … ! -path '*/.*' | LC_ALL=C sort | xargs cksum | cksum`)와 같은 값
func srcDigest(dir string) (string, error) {
	_, d, err := srcFiles(dir)
	return d, err
}

// srcFiles: srcDigest가 세는 파일(상대 경로, 바이트 순)과 그 판 — 기계에서 지을 때 이 목록만 판을 재고 꺼내 짓는다(filesSh)
func srcFiles(dir string) ([]string, string, error) {
	var files []string
	for _, top := range []string{"go.mod", "cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(dir, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type().IsRegular() {
				rel, _ := filepath.Rel(dir, p)
				files = append(files, rel)
			}
			return nil
		})
		if err != nil {
			return nil, "", fmt.Errorf("소스 걷기 %s: %w", top, err)
		}
	}
	sort.Strings(files)
	var b bytes.Buffer
	for _, f := range files {
		fh, err := os.Open(filepath.Join(dir, f))
		if err != nil {
			return nil, "", fmt.Errorf("소스 읽기 %s: %w", f, err)
		}
		c, n, err := cksum.Reader(fh)
		fh.Close()
		if err != nil {
			return nil, "", err
		}
		fmt.Fprintf(&b, "%d %d %s\n", c, n, f)
	}
	return files, cksum.Dot(b.Bytes()), nil
}

// filesSh: installSh 앞에 붙이는 파일 목록 변수(files, 줄마다 하나). 비면 그 기계가 제 폴더를 걷는다(옛 길)
func filesSh(files []string) string {
	return "files='" + strings.ReplaceAll(strings.Join(files, "\n"), "'", `'\''`) + "'\n"
}

// gh: 표준 출력·종료 코드·표준 오류(그대로도 흘린다). 상한(proc.Limit — gh는 HERDR_TIMEOUT_REMOTE)에 걸리면 124 = 모름
func gh(args ...string) (string, int, string) {
	var eb bytes.Buffer
	c := exec.Command("gh", args...)
	c.Stderr = io.MultiWriter(os.Stderr, &eb)
	out, rc := output(c)
	return out, rc, eb.String()
}

// buildMax: 릴리스 빌드 하나의 상한(A09) — 넘으면 그 무리째(컴파일러 자손까지) 끄고 이번 릴리스를 보류한다
func buildMax() time.Duration {
	if n, err := strconv.Atoi(os.Getenv("HERDR_TIMEOUT_BUILD")); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 600 * time.Second
}

func goBin() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	for _, p := range []string{"/opt/homebrew/bin/go", "/usr/local/go/bin/go", "/usr/local/bin/go"} {
		if isFile(p) {
			return p
		}
	}
	return ""
}

func sha256File(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// exportTree: 커밋 commit의 트리를 원본 blob 바이트 그대로 dst에 푼다(ls-tree + cat-file --batch). git archive는 저장소 속성
// (export-ignore·export-subst·eol·필터)을 먹어 게시기가 검사한 blob과 빌드 입력이 갈린다(R-14). 실행 권한(100755)·링크(120000)는
// 커밋 그대로, 하위 모듈(160000)은 건너뛴다. 조회가 하나라도 실패하거나 출력이 목록과 안 맞으면 오류
func exportTree(repo, commit, dst string) error {
	ls, err := proc.Output(exec.Command("git", "-C", repo, "ls-tree", "-r", "-z", "--full-tree", commit))
	if err != nil {
		return fmt.Errorf("git ls-tree: %v", err)
	}
	type ent struct{ mode, sha, path string }
	var es []ent
	var in bytes.Buffer
	for _, rec := range strings.Split(strings.TrimSuffix(string(ls), "\x00"), "\x00") {
		if rec == "" {
			continue
		}
		meta, p, ok := strings.Cut(rec, "\t")
		f := strings.Fields(meta)
		if !ok || len(f) != 3 || p == "" {
			return fmt.Errorf("git ls-tree 줄이 깨졌다: %q", rec)
		}
		if f[1] == "commit" {
			continue
		}
		if f[1] != "blob" {
			return fmt.Errorf("git ls-tree: 모르는 종류 %s (%s)", f[1], p)
		}
		es = append(es, ent{f[0], f[2], p})
		in.WriteString(f[2] + "\n")
	}
	c := exec.Command("git", "-C", repo, "cat-file", "--batch")
	c.Stdin = &in
	out, err := proc.Output(c)
	if err != nil {
		return fmt.Errorf("git cat-file: %v", err)
	}
	for _, e := range es {
		hdr, rest, ok := bytes.Cut(out, []byte("\n"))
		f := strings.Fields(string(hdr))
		if !ok || len(f) != 3 || f[0] != e.sha || f[1] != "blob" {
			return fmt.Errorf("git cat-file 출력이 목록과 다르다: %q (%s)", hdr, e.path)
		}
		n, err := strconv.Atoi(f[2])
		if err != nil || n < 0 || len(rest) < n+1 || rest[n] != '\n' {
			return fmt.Errorf("git cat-file 출력이 잘렸다 (%s)", e.path)
		}
		data := rest[:n]
		out = rest[n+1:]
		p := filepath.Join(dst, filepath.FromSlash(e.path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		switch e.mode {
		case "120000":
			err = os.Symlink(string(data), p)
		case "100755":
			err = os.WriteFile(p, data, 0o755)
		default:
			err = os.WriteFile(p, data, 0o644)
		}
		if err != nil {
			return err
		}
	}
	if len(out) != 0 {
		return fmt.Errorf("git cat-file 출력이 목록보다 길다")
	}
	return nil
}
