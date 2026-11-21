package main

// 색인 쓸기 — 셸 판 lib/hc-sweep/index.py와 같다. SQLite는 표준 라이브러리에 없어 sqlite3 명령(-readonly)으로 읽는다

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// indexGone: 색인의 원본이 모두 사라졌나. 빈 목록·상대 경로·/Volumes/ 아래(실제 경로로도)·하나라도 있음(읽기 오류 포함)이면 false
func indexGone(roots []string, exists func(string) bool) bool {
	if len(roots) == 0 {
		return false
	}
	for _, r := range roots {
		if !strings.HasPrefix(r, "/") || strings.HasPrefix(r, "/Volumes/") || strings.HasPrefix(realpath(r), "/Volumes/") {
			return false
		}
	}
	for _, r := range roots {
		if exists(r) {
			return false
		}
	}
	return true
}

// indexRoots: 색인 DB의 root_path들(빈 값 빼고). 열기·읽기 실패·표 없음·sqlite3 없음은 빈 목록(건너뜀)
func indexRoots(db string, r runner) []string {
	// -readonly는 쓰지 않는다: macOS sqlite3는 -shm이 없는 WAL DB를 읽기 전용으로 못 연다(파이썬 mode=ro는 연다). 대신 query_only로
	// 쓰기를 막는다 — 열며 생긴 -wal·-shm은 sweepOne이 다시 모아 함께 지운다
	res := r(30*time.Second, nil, "sqlite3", "-batch", "-noheader", "-cmd", ".timeout 2000", "-cmd", "PRAGMA query_only=1", db,
		"SELECT root_path FROM projects WHERE root_path <> ''")
	if res.err != nil || res.rc != 0 {
		return nil
	}
	var roots []string
	for _, l := range strings.Split(strings.TrimRight(res.out, "\n"), "\n") {
		if l != "" {
			roots = append(roots, l)
		}
	}
	return roots
}

func lexists(p string) bool { _, err := os.Lstat(p); return err == nil }

// sweepOne: 색인 DB 하나. 지웠으면(dry면 지울 것이면) (MB, roots), 아니면 (0, nil). 나이는 열기 전에 .db·-journal·비지 않은
// -wal로 본다(-shm·빈 -wal은 읽기 전용으로 열기만 해도 생긴다). 곁 파일은 연 뒤 다시 모으고 .db를 맨 끝에 지운다
func sweepOne(db string, dry bool, minAge float64) (float64, []string, error) {
	side := func() []string {
		var s []string
		for _, x := range []string{db + "-wal", db + "-shm", db + "-journal"} {
			if lexists(x) {
				s = append(s, x)
			}
		}
		return s
	}
	if st, err := os.Stat(db); err != nil || !st.Mode().IsRegular() {
		return 0, nil, nil
	}
	newest := time.Time{}
	for _, x := range append([]string{db}, side()...) {
		st, err := os.Stat(x)
		if err != nil {
			return 0, nil, err
		}
		if strings.HasSuffix(x, "-shm") || (strings.HasSuffix(x, "-wal") && st.Size() == 0) {
			continue
		}
		if st.ModTime().After(newest) {
			newest = st.ModTime()
		}
	}
	if time.Since(newest).Seconds() < minAge {
		return 0, nil, nil
	}
	roots := indexRoots(db, run)
	if !indexGone(roots, exists) {
		return 0, nil, nil
	}
	files := append(side(), db)
	var size int64
	for _, x := range files {
		if st, err := os.Stat(x); err == nil && st.Mode().IsRegular() {
			size += st.Size()
		}
	}
	mb := float64(size) / 1e6
	if !dry {
		for _, x := range files {
			// unlink: 폴더면 실패(파이썬 os.remove와 같이 — 빈 폴더도 지우지 않는다)
			if err := syscall.Unlink(x); err != nil && !os.IsNotExist(err) {
				return 0, nil, fmt.Errorf("%v: '%s'", err, x)
			}
		}
		record("색인:"+strings.TrimSuffix(filepath.Base(db), ".db"), "-", "색인 지움(원본 없음)", strings.Join(roots, " "), "-",
			fmt.Sprintf("크기 %.1fMB", mb))
	}
	return mb, roots, nil
}

// sweepIndex: 색인 쓸기 한 바퀴. 지운(dry면 지울) 수
func sweepIndex(dry bool, minAge float64) int {
	c := os.Getenv("XDG_CACHE_HOME")
	if c == "" {
		c = filepath.Join(home, ".cache")
	}
	d := filepath.Join(c, "codebase-memory-mcp")
	ents, err := os.ReadDir(d)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		fmt.Println("색인 쓸기 실패:", err)
		return 0
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	n, total := 0, 0.0
	for _, f := range names {
		if !strings.HasSuffix(f, ".db") || f == "_config.db" {
			continue
		}
		short := strings.TrimSuffix(f, ".db")
		mb, roots, err := sweepOne(filepath.Join(d, f), dry, minAge)
		if err != nil { // 이 DB만 건너뛴다(다음 바퀴에 다시)
			fmt.Printf("색인 쓸기 실패: %s %v\n", short, err)
			continue
		}
		if roots != nil {
			word := "지운 색인:"
			if dry {
				word = "지울 색인:"
			}
			fmt.Printf("%s %s %s 크기 %.1fMB\n", word, short, strings.Join(roots, " "), mb)
			n++
			total += mb
		}
	}
	if n > 0 || dry { // 0개면 조용히(기계마다 하루 288줄)
		word := "지움"
		if dry {
			word = "지울 것"
		}
		fmt.Printf("색인 쓸기: %s %d개(%.1fMB)\n", word, n, total)
	}
	return n
}
