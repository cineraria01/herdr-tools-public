package main

// 디스크 쓸기: 임시 찌꺼기(go-build*·Test*), Go 빌드 캐시 상한, 디스크 부족 알림 hc-disk-low — 셸 판 lib/hc-sweep/disk.py와 같다

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
)

const gb = 1e9

var hexDir = regexp.MustCompile(`^[0-9a-f]{2}$`)

func tmpDir(r runner) string {
	if t := os.Getenv("TMPDIR"); t != "" {
		return t
	}
	// launchd 작업에는 TMPDIR가 없을 수 있다. go는 터미널의 TMPDIR(= 이 값)를 쓴다
	if res := r(10*time.Second, nil, "getconf", "DARWIN_USER_TEMP_DIR"); res.err == nil && res.rc == 0 &&
		strings.HasPrefix(strings.TrimSpace(res.out), "/") {
		return strings.TrimSpace(res.out)
	}
	return os.TempDir()
}

var errNewer = fmt.Errorf("새 항목")

// scan: (바이트, 오래됨). 하나라도 cutoff보다 새로우면 오래됨 false로 바로 끝. 못 읽는 곳은 오류(건너뜀)
func scan(path string, cutoff time.Time) (int64, bool, error) {
	var size int64
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		st, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if st.ModTime().After(cutoff) {
			return errNewer
		}
		size += blocks(st)
		return nil
	})
	if err == errNewer {
		return size, false, nil
	}
	return size, err == nil, err
}

// removeTree: go 모듈 캐시를 품은 t.TempDir는 폴더가 0555라 그냥은 못 지운다 — 쓰기 막힌 폴더를 u+rwx로 연 뒤 지운다
func removeTree(path string) error {
	filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			if st, err := os.Lstat(p); err == nil && st.Mode().Perm()&0o200 == 0 {
				os.Chmod(p, st.Mode().Perm()|0o700)
			}
		}
		return nil
	})
	return os.RemoveAll(path)
}

// sweepTmp: 임시 찌꺼기 한 바퀴. lsofFailed = 지울 후보가 있었는데 lsof로 열린 경로를 몰라 건너뜀. limit = lsof 한 번의 상한
func sweepTmp(base string, hours float64, limit time.Duration, dry bool, r runner) (asked, lsofFailed bool) {
	ents, err := os.ReadDir(base)
	if err != nil {
		return
	}
	base = realpath(base)
	cutoff := time.Now().Add(-time.Duration(hours * float64(time.Hour)))
	type cand struct {
		p    string
		size int64
	}
	var old []cand
	for _, e := range ents {
		name := e.Name()
		p := filepath.Join(base, name)
		if !strings.HasPrefix(name, "go-build") && !strings.HasPrefix(name, "Test") {
			continue
		}
		st, err := os.Lstat(p)
		if err != nil || !st.IsDir() || !mine(st) {
			continue
		}
		if size, aged, err := scan(p, cutoff); err == nil && aged {
			old = append(old, cand{p, size})
		}
	}
	if len(old) == 0 {
		return
	}
	t0 := time.Now()
	held := openPaths(r, limit)
	if held == nil {
		fmt.Printf("임시 찌꺼기: lsof 실패 — 건너뜀(%.1f초, 상한 %.0f초 sweep.lsof_secs)\n", time.Since(t0).Seconds(), limit.Seconds())
		return true, true
	}
	if dry {
		fmt.Printf("임시 찌꺼기: 후보 %d개 lsof %.1f초\n", len(old), time.Since(t0).Seconds())
	}
	asked = true
	n, total := 0, int64(0)
	for _, c := range old {
		busy := false
		for h := range held {
			if under(h, c.p) {
				busy = true
				break
			}
		}
		if busy {
			continue
		}
		if !dry {
			if err := removeTree(c.p); err != nil {
				fmt.Printf("임시 찌꺼기 못 지움: %s %v\n", c.p, err)
				continue
			}
		}
		n, total = n+1, total+c.size
		if dry {
			fmt.Println("지울 임시:", c.p)
		} else {
			fmt.Println("지운 임시:", c.p)
		}
	}
	if n > 0 && !dry {
		record("임시:"+base, "-", "임시 찌꺼기 지움("+pyFloat(hours)+"시간 넘음)", base, "-", fmt.Sprintf("%d개 %.1fGB", n, float64(total)/gb))
	}
	if n > 0 {
		word := "지움"
		if dry {
			word = "지울 것"
		}
		fmt.Printf("임시 찌꺼기: %s %d개(%.1fGB)\n", word, n, float64(total)/gb)
	}
	return
}

// tmpSkipAlert: lsof 건너뜀이 n바퀴(sweep.tmp_skip_alert) 잇달면 사건 hc-sweep-tmp-skip 한 번 — 임시 폴더 정리가 사실상 꺼졌다는 것을
// 지휘가 알게. 표식 <상태>/hc-sweep/tmp-skip「<횟수><탭><처음 시각><탭><보냄 0|1>」, lsof가 되면 지워 다음 연속을 새로 센다.
// 키는 처음 시각 — 보낸 뒤 표식을 못 고쳐 다시 보내도 hc-lane이 같은 키를 거른다. dry-run·n<=0이면 아무것도 안 함
func tmpSkipAlert(state string, failed bool, n int, dry bool, lane string, r runner) {
	if dry || n <= 0 {
		return
	}
	if !failed {
		os.Remove(state)
		return
	}
	count, first, sent := 0, "", false
	if b, err := os.ReadFile(state); err == nil {
		if f := strings.Split(strings.TrimSpace(string(b)), "\t"); len(f) == 3 {
			if c, err := strconv.Atoi(f[0]); err == nil && f[1] != "" {
				count, first, sent = c, f[1], f[2] == "1"
			}
		}
	}
	if first == "" { // 없거나 잘린 표식은 새 연속으로
		count, first, sent = 0, time.Now().Format("2006-01-02T15:04:05"), false
	}
	count++
	if count >= n && !sent {
		msg := fmt.Sprintf("임시 찌꺼기 쓸기가 %d바퀴 잇달아 건너뜀(lsof 실패·상한 넘김, %s부터) — 이 기계의 임시 폴더 정리가 꺼진 셈", count, first)
		if sent = laneReport(r, lane, "hc-sweep-tmp-skip", msg, "hc-sweep-tmp-skip:"+first); sent {
			fmt.Println("임시 찌꺼기 건너뜀 알림:", msg)
		} else {
			fmt.Println("임시 찌꺼기 건너뜀 알림 실패(hc-lane report) — 다음 바퀴에 같은 키로 다시")
		}
	}
	mark := "0"
	if sent {
		mark = "1"
	}
	if os.MkdirAll(filepath.Dir(state), 0o755) == nil {
		atomic.WriteFile(state, []byte(fmt.Sprintf("%d\t%s\t%s\n", count, first, mark)), 0o644)
	}
}

// goBusy: go 명령(빌드·시험)이 도는가(ps comm 이름 go — 시험은 HC_SWEEP_GO_COMM으로 바꾼다). 못 보면 true(건드리지 않음).
// 종료 0이어도 목록이 비면(ps는 적어도 이 프로세스·ps 자신을 찍는다) 못 본 것이다(B22)
func goBusy(r runner) bool {
	name := os.Getenv("HC_SWEEP_GO_COMM")
	if name == "" {
		name = "go"
	}
	res := r(30*time.Second, nil, "ps", "-x", "-o", "comm=")
	if res.err != nil || res.rc != 0 || strings.TrimSpace(res.out) == "" {
		return true
	}
	for _, l := range strings.Split(res.out, "\n") {
		if filepath.Base(strings.TrimSpace(l)) == name {
			return true
		}
	}
	return false
}

// trimGocache: Go 빌드 캐시 줄이기 한 바퀴(부르는 쪽이 디스크가 문턱 가까울 때만). GOCACHE(없으면 ~/.cache/go-build — 병합기 기본)의
// <두 자리 16진>/ 아래 항목을 mtime 오래된 것부터 cap 아래로, 옛 자리 ~/Library/Caches/go-build는 GOCACHE와 다르면 통째로 비운다
func trimGocache(capGB float64, dry bool, r runner) {
	cache := os.Getenv("GOCACHE")
	if cache == "" {
		cache = filepath.Join(home, ".cache", "go-build")
	}
	old := filepath.Join(home, "Library", "Caches", "go-build")
	if goBusy(r) {
		fmt.Println("Go 캐시 줄이기: go가 도는 중이거나 프로세스 목록을 못 읽음 — 건너뜀")
		return
	}
	type ent struct {
		mt   time.Time
		size int64
		p    string
	}
	var files []ent
	var total int64
	filepath.WalkDir(cache, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !hexDir.MatchString(filepath.Base(filepath.Dir(p))) {
			return nil
		}
		if st, err := os.Lstat(p); err == nil && st.Mode().IsRegular() {
			files = append(files, ent{st.ModTime(), blocks(st), p})
			total += blocks(st)
		}
		return nil
	})
	sort.SliceStable(files, func(i, j int) bool { return files[i].mt.Before(files[j].mt) })
	over, n, freed := float64(total)-capGB*gb, 0, int64(0)
	for _, f := range files { // go는 쓸 때 mtime을 새로 한다: 오래된 것부터
		if float64(freed) >= over {
			break
		}
		if !dry && os.Remove(f.p) != nil {
			continue
		}
		n, freed = n+1, freed+f.size
	}
	var oldSize int64
	if st, err := os.Lstat(old); err == nil && st.IsDir() && realpath(old) != realpath(cache) {
		size, _, err := scan(old, time.Now().Add(24*time.Hour))
		if err == nil && !dry {
			err = removeTree(old)
		}
		if err != nil {
			fmt.Printf("Go 옛 캐시 못 비움: %s %v\n", old, err)
		} else {
			oldSize = size
		}
	}
	if n == 0 && oldSize == 0 {
		return
	}
	msg := fmt.Sprintf("캐시 %s 항목 %d개 %.1fGB(상한 %gGB)", cache, n, float64(freed)/gb, capGB)
	if oldSize > 0 {
		msg += fmt.Sprintf(" · 옛 캐시 %s %.1fGB", old, float64(oldSize)/gb)
	}
	if dry {
		fmt.Println("Go 캐시 줄이기 (할 것):", msg)
		return
	}
	fmt.Println("Go 캐시 줄이기:", msg)
	record("go-cache:"+cache, "-", "Go 캐시 줄임(디스크 문턱 가까움)", cache, "-", msg)
}

func du(path string, r runner) string {
	res := r(300*time.Second, nil, "du", "-sk", path)
	if f := strings.Fields(res.out); res.err == nil && len(f) > 0 {
		if k, err := strconv.ParseFloat(f[0], 64); err == nil {
			return fmt.Sprintf("%.1fGB", k*1024/gb)
		}
	}
	if exists(path) {
		return "?"
	}
	return "없음"
}

// pendingMark: 부족 표식 첫 칸 — 「보내는 중」(보고가 받아들여졌는지 아직 모름). 보낸 뒤 「<시각><탭><글>」로 바꾼다(E06).
// 중단·실패 뒤 다음 바퀴는 표식의 시각으로 만든 같은 키로 다시 보낸다(hc-lane이 같은 키는 한 번만 적는다 — 두 바퀴 겹쳐도 한 건, B07).
const pendingMark = "대기"

// readMark: 표식 (있음, 대기 중이면 그 시각). 못 읽으면 있음·대기 아님(다시 보내지 않는 쪽). 비었거나 잘린 표식(빈 파일·첫 줄에
// 탭 없음·「대기」인데 시각 빔 — 쓰다 끊김)은 보냈는지 모르니 없는 것으로 본다(R-21 — 같은 부족을 다시 확인해 보낸다)
func readMark(state string) (bool, string) {
	if !lexists(state) {
		return false, ""
	}
	b, err := os.ReadFile(state)
	if err != nil {
		return true, ""
	}
	f := strings.SplitN(strings.SplitN(string(b), "\n", 2)[0], "\t", 3)
	switch {
	case len(f) >= 2 && f[0] == pendingMark && f[1] != "":
		return true, f[1]
	case len(f) >= 2 && f[0] != "" && f[0] != pendingMark: // 「<시각><탭><글>」 = 보냄(셸 판 표식도 같은 꼴)
		return true, ""
	}
	return false, ""
}

// diskAlert: 부족 알림 한 바퀴(free·used = diskUsage). 보냈으면(dry면 보낼 것이면) 그 한 줄, 아니면 ""
func diskAlert(free, used, limitGB float64, state, base string, dry bool, lane string, r runner) string {
	if free >= limitGB*1.5*gb {
		if lexists(state) && !dry {
			os.Remove(state)
		}
		return ""
	}
	marked, pendingAt := readMark(state)
	if marked && pendingAt == "" {
		return "" // 이번 부족은 이미 보냄
	}
	if free >= limitGB*gb && !marked {
		return ""
	}
	short := func(p string) string {
		if strings.HasPrefix(p, home+"/") {
			return "~" + p[len(home):]
		}
		return p
	}
	var sizes []string
	for _, p := range []string{base, filepath.Join(home, "Library/Caches/go-build"), filepath.Join(home, ".cache/go-build")} {
		sizes = append(sizes, short(strings.TrimRight(p, "/"))+" "+du(p, r))
	}
	total := used + free
	if total == 0 {
		total = 1
	}
	msg := fmt.Sprintf("디스크 여유 %.1fGB(사용 %.0f%%) < 문턱 %sGB — %s (임시 찌꺼기는 hc-sweep이 지움, Go 캐시는 쓰는 곳이 없을 때만 손으로"+
		" — ~/Library/Caches/go-build는 macOS 기본 GOCACHE)", free/gb, 100*used/total, pyFloat(limitGB), strings.Join(sizes, " · "))
	if dry {
		return msg
	}
	// 대기 표식을 먼저 쓴다. 디스크가 차서 못 쓰면 날짜 키로 보내 hc-lane이 그날 두 번 적지 않게 한다
	at := pendingAt
	if at == "" {
		// 키 글자(hc-lane report --key: A-Za-z0-9_.:/-). 초 아래까지 — 회복 뒤 같은 초의 새 부족이 같은 키로 걸러지지 않게
		at = time.Now().Format("2006-01-02T15:04:05.000000")
		if os.MkdirAll(filepath.Dir(state), 0o755) != nil || atomic.WriteFile(state, []byte(pendingMark+"\t"+at+"\t"+msg+"\n"), 0o644) != nil {
			at = time.Now().Format("2006-01-02")
		}
	}
	if !laneReport(r, lane, "hc-disk-low", msg, "hc-disk-low:"+at) {
		fmt.Println("디스크 부족 알림 실패(hc-lane report) — 다음 바퀴에 같은 키로 다시")
		return ""
	}
	if lexists(state) { // 보냄: 대기 표식을 「보냄」으로 원자 저장(못 바꾸면 다음 바퀴가 같은 키로 한 번 더 — hc-lane이 거른다)
		if err := atomic.WriteFile(state, []byte(at+"\t"+msg+"\n"), 0o644); err != nil {
			fmt.Println("디스크 부족 표식을 「보냄」으로 못 바꿈 — 다음 바퀴에 같은 키로 한 번 더:", err)
		}
	}
	return msg
}

// laneReport: 터미널 밖 사건 한 줄(hc-lane report 중간 - <msg> --name <name> [--key <key>]). 터미널 표식은 뗀다(이 터미널 사건이 되지 않게)
func laneReport(r runner, lane, name, msg, key string) bool {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=") // = 없는 항목도 패닉 없이(L5)
		if k != "HC_TERMINAL_ID" && k != "HC_LANE" && k != "HC_LANE_OWNER" {
			env = append(env, kv)
		}
	}
	if !executable(lane) {
		lane = "hc-lane"
		if p, err := exec.LookPath("hc-lane"); err == nil {
			lane = p
		}
	}
	args := []string{"report", "중간", "-", msg, "--name", name}
	if key != "" {
		args = append(args, "--key", key)
	}
	res := r(60*time.Second, env, lane, args...)
	return res.err == nil && res.rc == 0
}

func freeBytes(path string) (float64, error) {
	free, _, err := diskUsage(path)
	return free, err
}

// diskRun: 디스크 쓸기 한 바퀴. 설정을 못 읽으면 그 쓸기는 건너뛴다
func diskRun(dry bool, r runner) error {
	numConf := func(key, def string) float64 { // 숫자가 아니면 그 키만 끈다
		v, ok := conf(key, def)
		if !ok {
			return 0
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			fmt.Printf("디스크 쓸기: %s 값이 숫자가 아님(%s) — 그 쓸기만 끔\n", key, v)
			return 0
		}
		return f
	}
	base := tmpDir(r)
	if hours := numConf("sweep.tmp_hours", "3"); hours > 0 {
		if asked, failed := sweepTmp(base, hours, lsofLimit(conf("sweep.lsof_secs", "60")), dry, r); asked {
			tmpSkipAlert(filepath.Join(stateHome(), "hc-sweep", "tmp-skip"), failed, int(numConf("sweep.tmp_skip_alert", "3")), dry,
				filepath.Join(binDir, "hc-lane"), r)
		}
	}
	limit := numConf("sweep.disk_free_gb", "10")
	if limit <= 0 {
		return nil
	}
	if capGB := numConf("sweep.gocache_gb", "6"); capGB > 0 {
		free, err := freeBytes(home)
		if err != nil {
			return err
		}
		if free < limit*1.5*gb {
			trimGocache(capGB, dry, r)
		}
	}
	free, used, err := diskUsage(home)
	if err != nil {
		return err
	}
	state := filepath.Join(stateHome(), "hc-sweep", "disk-low")
	if msg := diskAlert(free, used, limit, state, base, dry, filepath.Join(binDir, "hc-lane"), r); msg != "" {
		if dry {
			fmt.Println("디스크 부족 알림 (보낼 것):", msg)
		} else {
			fmt.Println("디스크 부족 알림:", msg)
		}
	}
	return nil
}
