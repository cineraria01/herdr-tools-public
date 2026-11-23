//go:build !windows

package main

// 메모리 디스크 임시 폴더(2026-11-24 verify-ramdisk): 무거운 단계의 병목은 CPU가 아니라 디스크의 작은 쓰기였다(시험들이 임시 폴더에
// git 저장소를 만들고 커밋·push하기를 수없이 되풀이 — 5KB 쓰기 초당 약 1만 번에서 멈춤, 자리 수와 무관). TMPDIR을 메모리 디스크로
// 주면 워커 맥 10코어에서 542 → 309초였다. 그래서 macOS에서 무거운 단계 앞에 메모리 디스크(APFS, 대소문자 구분 없음 — 부팅 볼륨과
// 같은 꼴)를 만들어 조각의 TMPDIR로 주고, 끝나면(신호로 끝날 때도) 뗀다 — 메모리는 떼야 돌아온다.
//
//	HERDR_VERIFY_RAMTMP   없음 = 켬(ramDefaultMB) · 0 = 끔 · <MB> = 그 크기
//
// 기계 메모리의 1/4을 넘는 크기면 켜지 않는다(작은 기계 보호). 만들지 못하면(명령 실패·시간 넘김·확인 실패) 한 줄 알리고 보통 임시
// 폴더로 그대로 검증한다 — 메모리 디스크 때문에 검증이 실패로 끝나지 않는다.
//
// 지우기 안전: 포맷(diskutil erasevolume)은 바로 그 호출에서 방금 붙인 장치 하나에만 한다 — 붙이기 출력이 「/dev/disk<수>」 한 줄이고,
// hdiutil info에서 그 장치가 ① 메모리 디스크 이미지(image-path ram://…) ② 크기가 요청한 섹터 수 ③ 마운트된 볼륨이 없을 때만.
// 떼기는 ①을, 치우기는 ①과 우리 이름 꼴(herdr-verify-ram-<pid>)을 hdiutil info에서 다시 확인한 뒤에만 그 이미지의 장치를 뗀다.
// 장치 번호를 추측하거나 이름만으로 고르지 않고, 명령은 exec 인자로만 넘긴다(셸 문자열 없음).
// 죽은 검증이 남긴 디스크: 시작할 때 우리 이름 꼴 볼륨 가운데 이름의 pid가 죽은(또는 이 프로세스 자신인 — 아직 아무것도 안 만들었다) 것만 뗀다.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	ramDefaultMB = 1024 // 기본 크기: 2026-11-24 워커 맥 10코어 실측 최대 사용 354MB의 약 3배(docs/repo/tests.md 「메모리 디스크」)
	ramPrefix    = "herdr-verify-ram-"
)

// 시험 고리: OS·볼륨 뿌리·명령 상한·기계 메모리
var (
	goos       = runtime.GOOS
	volRoot    = "/Volumes"
	ramTimeout = time.Minute
	memBytes   = func() int64 {
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}
		n, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		return n
	}
)

// ramDisk: 붙여 포맷한 메모리 디스크. dir은 조각의 TMPDIR
type ramDisk struct {
	dev, name, vol, dir string
	sizeMB              int
	took                time.Duration
	peak                int64 // 바이트
	stop                chan struct{}
	done                sync.WaitGroup
	mu                  sync.Mutex
	closed              bool
}

// ramMB: HERDR_VERIFY_RAMTMP → 크기(MB), 0이면 끔
func ramMB(w *os.File) int {
	v := os.Getenv("HERDR_VERIFY_RAMTMP")
	switch n, err := strconv.Atoi(v); {
	case v == "":
		return ramDefaultMB
	case err != nil || n < 0:
		fmt.Fprintf(w, "verify: HERDR_VERIFY_RAMTMP=%q 는 0 또는 MB 수가 아니라 기본 %dMB로 한다\n", v, ramDefaultMB)
		return ramDefaultMB
	default:
		return n
	}
}

// ramCmd: 상한(ramTimeout)을 둔 명령. 표준 출력과 오류
func ramCmd(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ramTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, name, args...)
	c.WaitDelay = 2 * time.Second // 출력을 붙든 손자가 있어도 상한 뒤 돌아온다
	var errb strings.Builder
	c.Stderr = &errb
	out, err := c.Output()
	if ctx.Err() != nil {
		return string(out), fmt.Errorf("%s %s: %v 넘김", name, strings.Join(args, " "), ramTimeout)
	}
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %v %s", name, strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return string(out), nil
}

// image: hdiutil info의 이미지 한 덩이. devs는 「/dev/…」 줄의 장치, mounts는 그 장치들의 마운트 위치(비지 않은 것만)
type image struct {
	path   string
	blocks string
	devs   []string
	mounts []string
}

// images: hdiutil info를 이미지 덩이로(「====…」 줄이 덩이를 나눈다)
func images() ([]image, error) {
	out, err := ramCmd("hdiutil", "info")
	if err != nil {
		return nil, err
	}
	var ims []image
	var cur *image
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		l := sc.Text()
		switch {
		case strings.HasPrefix(l, "===="):
			ims = append(ims, image{})
			cur = &ims[len(ims)-1]
		case cur == nil:
		case strings.HasPrefix(l, "/dev/"):
			f := strings.Split(l, "\t")
			cur.devs = append(cur.devs, strings.TrimSpace(f[0]))
			if len(f) >= 3 && strings.TrimSpace(f[2]) != "" {
				cur.mounts = append(cur.mounts, strings.TrimSpace(strings.Join(f[2:], "\t")))
			}
		default:
			k, v, ok := strings.Cut(l, ":")
			if !ok {
				continue
			}
			switch strings.TrimSpace(k) {
			case "image-path":
				cur.path = strings.TrimSpace(v)
			case "blockcount":
				cur.blocks = strings.TrimSpace(v)
			}
		}
	}
	return ims, nil
}

// imageOf: dev를 「/dev/…」 줄의 첫 장치로 가진 이미지(정확히 하나). 없거나 여럿이면 오류
func imageOf(dev string) (image, error) {
	ims, err := images()
	if err != nil {
		return image{}, err
	}
	var hit []image
	for _, im := range ims {
		if len(im.devs) > 0 && im.devs[0] == dev {
			hit = append(hit, im)
		}
	}
	if len(hit) != 1 {
		return image{}, fmt.Errorf("hdiutil info에 %s 이미지가 %d개", dev, len(hit))
	}
	return hit[0], nil
}

func isRAM(im image) bool { return strings.HasPrefix(im.path, "ram://") }

var devLine = regexp.MustCompile(`^/dev/disk[0-9]+$`)

// attachedDev: 붙이기 표준 출력 → 장치. 비지 않은 줄이 정확히 하나이고 그 첫 칸이 /dev/disk<수>, 나머지 칸이 비었을 때만
func attachedDev(out string) (string, error) {
	var ls []string
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			ls = append(ls, l)
		}
	}
	if len(ls) != 1 {
		return "", fmt.Errorf("붙이기 출력이 한 줄이 아님: %q", out)
	}
	f := strings.Fields(ls[0])
	if len(f) != 1 || !devLine.MatchString(f[0]) {
		return "", fmt.Errorf("붙이기 출력이 /dev/disk<수>가 아님: %q", out)
	}
	return f[0], nil
}

// checkFresh: 포맷 전 확인 ①②③. 통과면 nil
func checkFresh(dev string, sectors int64) error {
	im, err := imageOf(dev)
	switch {
	case err != nil:
		return err
	case !isRAM(im):
		return fmt.Errorf("%s 는 메모리 디스크가 아님(image-path %q)", dev, im.path)
	case im.blocks != strconv.FormatInt(sectors, 10):
		return fmt.Errorf("%s 크기 %s 섹터 ≠ 요청 %d", dev, im.blocks, sectors)
	case len(im.mounts) > 0:
		return fmt.Errorf("%s 에 이미 마운트된 볼륨이 있음(%s)", dev, strings.Join(im.mounts, ", "))
	}
	return nil
}

// detachRAM: dev가 메모리 디스크 이미지의 장치일 때만(그리고 want가 있으면 그 이미지에 want 마운트가 있을 때만) 뗀다
func detachRAM(dev, want string) error {
	im, err := imageOf(dev)
	if err != nil {
		return err
	}
	if !isRAM(im) {
		return fmt.Errorf("%s 는 메모리 디스크가 아니라 안 뗀다(image-path %q)", dev, im.path)
	}
	if want != "" && !contains(im.mounts, want) {
		return fmt.Errorf("%s 에 %s 마운트가 없어 안 뗀다(%s)", dev, want, strings.Join(im.mounts, ", "))
	}
	if _, err := ramCmd("hdiutil", "detach", dev); err != nil {
		if _, err2 := ramCmd("hdiutil", "detach", dev, "-force"); err2 != nil {
			return fmt.Errorf("%v; -force: %v", err, err2)
		}
	}
	return nil
}

func contains(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}

var ourVol = regexp.MustCompile(`^` + regexp.QuoteMeta(ramPrefix) + `([0-9]+)$`)

// sweepStale: 죽은 검증이 남긴 우리 이름 꼴 메모리 디스크를 뗀다(이름의 pid가 죽었거나 이 프로세스 자신)
func sweepStale(w *os.File) {
	ims, err := images()
	if err != nil {
		fmt.Fprintf(w, "알림 메모리 디스크 치우기: %v\n", err)
		return
	}
	for _, im := range ims {
		if !isRAM(im) || len(im.devs) == 0 {
			continue
		}
		for _, m := range im.mounts {
			if filepath.Dir(m) != volRoot {
				continue
			}
			s := ourVol.FindStringSubmatch(filepath.Base(m))
			if s == nil {
				continue
			}
			pid, _ := strconv.Atoi(s[1])
			if pid != os.Getpid() && !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
				continue // 살아 있음(EPERM도 산 것)
			}
			if err := detachRAM(im.devs[0], m); err != nil {
				fmt.Fprintf(w, "알림 죽은 검증(pid %d)의 메모리 디스크를 못 뗌: %v\n", pid, err)
			} else {
				fmt.Fprintf(w, "치움 죽은 검증(pid %d)의 메모리 디스크 %s\n", pid, m)
			}
		}
	}
}

// ramSetup: 켜져 있으면 메모리 디스크를 만들어 돌려준다. 끔·macOS 아님·너무 큼·만들기 실패면 nil(실패는 한 줄 알림)
func ramSetup(w *os.File) *ramDisk {
	mb := ramMB(w)
	if goos != "darwin" || mb == 0 {
		return nil
	}
	if m := memBytes(); m <= 0 {
		fmt.Fprintln(w, "알림 메모리 디스크 안 씀 — 기계 메모리(sysctl hw.memsize)를 못 읽었다")
		return nil
	} else if int64(mb)<<20 > m/4 {
		fmt.Fprintf(w, "알림 메모리 디스크 안 씀 — %dMB가 기계 메모리(%dMB)의 1/4을 넘는다(HERDR_VERIFY_RAMTMP=<MB>로 줄이거나 0으로 끈다)\n", mb, m>>20)
		return nil
	}
	t0 := time.Now()
	sweepStale(w)
	r, err := ramMake(mb)
	if err != nil {
		fmt.Fprintf(w, "알림 메모리 디스크를 못 만들어 보통 임시 폴더로 검증한다: %v\n", err)
		return nil
	}
	r.took = time.Since(t0)
	r.stop = make(chan struct{})
	r.done.Add(1)
	go r.watch()
	fmt.Fprintf(w, "메모리 디스크 %s(%dMB, %.1f초) — 무거운 단계의 TMPDIR\n", r.vol, mb, r.took.Seconds())
	return r
}

// ramMake: 붙이기 → 확인 ①②③ → 포맷 → 마운트 확인 → .metadata_never_index·t 폴더. 붙인 뒤 실패면 (①을 확인하고) 뗀다
func ramMake(mb int) (*ramDisk, error) {
	sectors := int64(mb) << 11 // 512바이트 섹터
	out, err := ramCmd("hdiutil", "attach", "-nomount", "ram://"+strconv.FormatInt(sectors, 10))
	if err != nil {
		return nil, err // 장치를 모른다 — 떼지 않는다(추측 금지)
	}
	dev, err := attachedDev(out)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*ramDisk, error) {
		if e := detachRAM(dev, ""); e != nil {
			return nil, fmt.Errorf("%v (붙인 %s 떼기도 실패: %v)", err, dev, e)
		}
		return nil, err
	}
	if err := checkFresh(dev, sectors); err != nil {
		return fail(err)
	}
	name := ramPrefix + strconv.Itoa(os.Getpid())
	if _, err := ramCmd("diskutil", "erasevolume", "APFS", name, dev); err != nil {
		return fail(err)
	}
	vol := filepath.Join(volRoot, name)
	im, err := imageOf(dev)
	if err != nil {
		return fail(err)
	}
	if !contains(im.mounts, vol) {
		return fail(fmt.Errorf("포맷한 %s 가 %s 에 마운트되지 않음(%s)", dev, vol, strings.Join(im.mounts, ", ")))
	}
	dir := filepath.Join(vol, "t")
	if err := os.WriteFile(filepath.Join(vol, ".metadata_never_index"), nil, 0o644); err != nil {
		return fail(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return fail(err)
	}
	return &ramDisk{dev: dev, name: name, vol: vol, dir: dir, sizeMB: mb}, nil
}

// used: 볼륨의 쓰인 바이트
func (r *ramDisk) used() int64 {
	var st syscall.Statfs_t
	if syscall.Statfs(r.vol, &st) != nil {
		return 0
	}
	return int64(st.Blocks-st.Bfree) * int64(st.Bsize)
}

// watch: 2초마다 사용량을 재 최대를 남긴다(가득 차 ENOSPC로 시험이 거짓 실패했는지 알아보게)
func (r *ramDisk) watch() {
	defer r.done.Done()
	for {
		u := r.used()
		r.mu.Lock()
		r.peak = max(r.peak, u)
		r.mu.Unlock()
		select {
		case <-r.stop:
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// env: 조각에 더할 환경 — TMPDIR은 메모리 디스크, HERDR_VERIFY_BOOT_TMPDIR은 원래 임시 폴더(부팅 볼륨이 필요한 시험용)
func (r *ramDisk) env() []string {
	if r == nil {
		return nil
	}
	return []string{"TMPDIR=" + r.dir + "/", "HERDR_VERIFY_BOOT_TMPDIR=" + os.TempDir()}
}

// teardown: 사용량 한 줄과 떼기. nil이거나 이미 뗐으면 아무것도 안 한다(신호 길의 defer와 끝 줄 앞 부름이 겹쳐도)
func (r *ramDisk) teardown(w *os.File) {
	if r == nil || r.closed {
		return
	}
	r.closed = true
	close(r.stop)
	r.done.Wait()
	peak := max(r.peak, r.used())
	t0 := time.Now()
	err := detachRAM(r.dev, r.vol)
	full := ""
	if peak*10 >= int64(r.sizeMB)<<20*9 {
		full = " — 거의 가득 참: ENOSPC로 시험이 거짓 실패했을 수 있다(HERDR_VERIFY_RAMTMP=<MB>로 늘리거나 0으로 끈다)"
	}
	fmt.Fprintf(w, "메모리 디스크 사용량 최대 %dMB / %dMB, 만들기 %.1f초 · 떼기 %.1f초%s\n", peak>>20, r.sizeMB, r.took.Seconds(), time.Since(t0).Seconds(), full)
	if err != nil {
		fmt.Fprintf(w, "알림 메모리 디스크를 못 뗐다(다음 검증이 시작할 때 치운다): %v\n", err)
	}
}
