// worker-mac-setup: 새 워커 맥에 처음 한 번 돌리는 권한·문맥 셋업과 워커끼리 연결(mesh)(Go 판, 2026-11-21 사용자 「모두 고로 옮기고 싶어」).
// 계약(인자·출력 줄·사람 몫 체크리스트·종료 코드 0·1·2·3·4·64·만드는 파일)은 셸 판 lib/worker-mac-setup/worker-mac-setup.sh와 같다
// — 설명은 help.txt(-h). 원격 단계는 remote.sh·mesh.sh를 bash로 보낸다(원격에는 Go가 없다). 인자·설정·hc 호출·상한 시간·
// 받은 줄 판정은 여기. 공개 키·키 내용은 화면에 찍지 않는다.
package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"herdr-tools/internal/version"
	wm "herdr-tools/internal/workermac"
)

var (
	//go:embed remote.sh
	remote string
	//go:embed help.txt
	help string
)

const usage = "사용: worker-mac-setup <기계> [--slots N] [--dry-run] | mesh [--check] [<기계>]"

type opts struct {
	m, slots                 string
	ss, dry, meshOnly, check bool
}

func main() {
	wm.Who = "worker-mac-setup"
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "--version" { // 공통 실행기가 바이너리를 쓰기 전에 본다
		fmt.Println(version.String("worker-mac-setup")) // 릴리스 커밋이 보이게(B25)
		return
	}
	o, code := parse(args)
	if code >= 0 {
		os.Exit(code)
	}
	ctx := wm.Signals()
	cfg := &wm.Config{Ctx: ctx}
	mesh := cfg.Get("worker.mesh")
	if cfg.Err != nil {
		fmt.Fprintln(os.Stderr, cfg.Err)
		os.Exit(2)
	}
	d := time.Now().Format("20060102")
	if o.meshOnly {
		if mesh == "" {
			fmt.Fprintln(os.Stderr, "설정 worker.mesh가 없다(서로 잇는 워커 이름들, 예: config.example/config)")
			os.Exit(2)
		}
		// --dry-run도 읽기만(머리말 「워커 연결은 --check」 — 셸 판은 mesh 부명령에서 --dry-run을 보지 않아 바꿨다)
		os.Exit(meshRun(ctx, mesh, o.m, o.check || o.dry, d))
	}
	os.Exit(setup(ctx, cfg, o, mesh, d))
}

// parse: 인자. code가 0 이상이면 그 코드로 끝낸다(도움말 0·사용 오류 64)
func parse(args []string) (opts, int) {
	o := opts{slots: "1"}
	if len(args) > 0 && args[0] == "mesh" {
		o.meshOnly, args = true, args[1:]
	}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--slots":
			o.ss, o.slots = true, ""
			if i+1 < len(args) {
				o.slots = args[i+1]
				i++
			}
		case "--dry-run":
			o.dry = true
		case "--check":
			o.check = true
		case "-h", "--help":
			fmt.Print(help)
			return o, 0
		default:
			if strings.HasPrefix(a, "-") { // 모르는 옵션·-로 시작하는 이름: ssh·hc가 옵션으로 읽는다(E10)
				fmt.Fprintln(os.Stderr, usage)
				return o, 64
			}
			o.m = a
		}
	}
	if o.m == "" && !o.meshOnly {
		fmt.Fprintln(os.Stderr, usage)
		return o, 64
	}
	return o, -1
}

func say(f string, a ...any) { fmt.Printf("▸ "+f+"\n", a...) }

func ssh(m string, cmd ...string) []string {
	return append([]string{"-n", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", "--", m}, cmd...) // -- : 이름을 옵션으로 읽지 않게(E10)
}

// lsSec·addSec: hc machine ls·hc machine add --install의 상한 초(시험이 줄인다). add는 원격에 코어를 깔아 길게
var lsSec, addSec = 30, 600

// machineKnown: hc machine ls --json을 풀어 정확한 이름 m이 있는가. 조회 실패·시간 넘김·JSON 아님은 err — 「없음」으로 읽지 않는다(E02)
func machineKnown(ctx context.Context, m string) (bool, error) {
	out, rc := wm.Output(ctx, lsSec, nil, "hc", "machine", "ls", "--json")
	switch {
	case rc == wm.TimeoutCode:
		return false, fmt.Errorf("시간 넘김(%d초)", lsSec)
	case rc != 0:
		return false, fmt.Errorf("종료 %d", rc)
	}
	var v struct {
		Machines []struct {
			Name string `json:"name"`
		} `json:"machines"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil || v.Machines == nil {
		return false, fmt.Errorf("JSON 목록이 아님")
	}
	for _, x := range v.Machines {
		if x.Name == m {
			return true, nil
		}
	}
	return false, nil
}

// addMachine: hc machine add <m> --install --name <m>(addSec 상한). 넘기면 등록됐는지 모른다 — 목록을 다시 보고 알리되 실패로 끝낸다(E11)
func addMachine(ctx context.Context, m string) bool {
	actx, cancel := context.WithTimeout(ctx, time.Duration(addSec)*time.Second)
	defer cancel()
	c := wm.Command(actx, "hc", "machine", "add", m, "--install", "--name", m)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	switch rc := wm.Run(actx, c); {
	case rc == wm.TimeoutCode:
		k, err := machineKnown(ctx, m)
		st := "목록에 없음"
		if err != nil {
			st = "목록도 모름(" + err.Error() + ")"
		} else if k {
			st = "목록에는 보이나 설치가 끝났는지 모름"
		}
		fmt.Fprintf(os.Stderr, "✗ hc machine add 시간 넘김(%d초) — 등록 결과를 모른다(%s). 다시 돌리면 목록부터 다시 본다\n", addSec, st)
		return false
	case rc != 0:
		fmt.Fprintln(os.Stderr, "✗ hc machine add 실패")
		return false
	}
	return true
}

// remoteCmd: 원격에서 「bash -c <스크립트>」 — 스크립트는 base64로 넘긴다(따옴표·줄바꿈을 원격 셸이 다시 읽지 않게)
func remoteCmd(script string, args ...string) string {
	s := `bash -c "$(echo ` + base64.StdEncoding.EncodeToString([]byte(script)) + ` | base64 -D)" _`
	for _, a := range args {
		s += " " + a
	}
	return s
}

// slotsOK: --slots 값은 1 이상 숫자(셸 판처럼 「0」 하나만 0으로 본다)
func slotsOK(s string) bool {
	return s != "" && s != "0" && !nonDigit.MatchString(s)
}

var nonDigit = regexp.MustCompile(`[^0-9]`)

func setup(ctx context.Context, cfg *wm.Config, o opts, mesh, d string) int {
	slotsFile, trust := cfg.Get("worker.slots_file"), cfg.Get("worker.trust", "Documents/app")
	key := cfg.Get("worker.ssh_key", "~/.ssh/id_ed25519")
	if cfg.Err != nil {
		fmt.Fprintln(os.Stderr, cfg.Err)
		return 2
	}
	miss := cfg.Miss
	cl, ok := cfg.File("worker-checklist.txt")
	if !ok {
		miss = append(miss, "worker-checklist.txt")
	}
	if o.ss && slotsFile == "" {
		fmt.Fprintln(os.Stderr, "--slots를 쓰려면 설정 worker.slots_file이 있어야 한다")
		return 64
	}
	if !slotsOK(o.slots) {
		fmt.Fprintln(os.Stderr, "--slots는 1 이상 숫자")
		return 64
	}
	m := o.m
	if _, rc := wm.Output(ctx, 15, nil, "ssh", ssh(m, "true")...); rc != 0 {
		fmt.Printf(`✗ ssh %[1]s 키 로그인이 안 된다. 먼저 사람이 할 일:
  1. 그 맥: 시스템 설정 → 일반 → 공유 → 원격 로그인 켜기, (i)에서 「원격 사용자에 대한 전체 디스크 접근 허용」 켜기.
  2. 그 맥: Tailscale 로그인, 관리 화면에서 키 만료 끄기.
  3. 이 맥 ~/.ssh/config에 Host %[1]s 블록(HostName=Tailscale 주소, User=그 맥 계정, IdentityFile %[2]s)을 넣고
     ! ssh-copy-id -i %[2]s.pub %[1]s     (그 맥 계정 암호는 사용자가 직접 친다)
  4. 다시: worker-mac-setup %[1]s
`, m, key)
		return 2
	}
	u, _ := wm.Output(ctx, 30, os.Stderr, "ssh", "-n", "-o", "BatchMode=yes", "--", m, "id -un")
	h, _ := wm.Output(ctx, 30, os.Stderr, "ssh", "-n", "-o", "BatchMode=yes", "--", m, "echo $HOME")
	user := wm.Trim(u)
	dry := ""
	if o.dry {
		dry = " — dry-run"
	}
	say("대상 %s (계정 %s, 홈 %s)%s", m, user, wm.Trim(h), dry)

	// 1. hc 기계 등록(코어 설치). 설치 직후 코어는 SSH 문맥이라 아래 2에서 launchd로 옮긴다.
	known, err := machineKnown(ctx, m)
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "✗ hc machine ls 실패(%v) — 등록돼 있는지 몰라 멈춤(등록하지 않았다)\n", err)
		return 1
	case known:
		say("hc 기계 %s: 이미 있음", m)
	case o.dry:
		say("(dry-run) hc machine add %s --install --name %s", m, m)
	default:
		say("hc 기계 등록")
		if !addMachine(ctx, m) {
			return 1
		}
	}

	// 2. 원격 자동 단계(SSH 문맥). 결과 줄은 원격이 그대로 찍는다. 앞에 설정 값(TRUST·SLOTS_FILE)이 붙는다.
	dryArg := "0"
	if o.dry {
		dryArg = "1"
	}
	tctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	c := wm.Command(tctx, "ssh", "-n", "-o", "BatchMode=yes", "--", m,
		remoteCmd("TRUST="+wm.Quote(trust)+" SLOTS_FILE="+wm.Quote(slotsFile)+"\n"+remote, dryArg, o.slots, d))
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	// 끊김(ssh 255)·시간 넘김을 삼키지 않는다 — 위에 찍힌 ▸ 줄까지가 한 것(E03)
	switch rc := wm.Run(tctx, c); {
	case rc == 3:
		return 3
	case rc == wm.TimeoutCode:
		fmt.Println("✗ 원격 자동 단계 시간 넘김(180초) — 위 줄까지 했고 나머지는 모른다. 다시 돌린다(있는 것은 건너뛴다)")
		return 1
	case rc != 0:
		fmt.Printf("✗ 원격 자동 단계 끊김(종료 %d) — 위 줄까지 했다. 다시 돌린다(있는 것은 건너뛴다)\n", rc)
		return 1
	}

	// 3. 문서 폴더 접근 질문을 지금 띄운다(그 맥 화면에서 허용). 허용 전엔 hc 터미널의 ~/Documents 작업이 멈춘다.
	if !o.dry {
		time.Sleep(3 * time.Second)
		r, _ := wm.Output(ctx, 12, nil, "hc", "exec", "--on", m, "--", "/bin/sh", "-c", `ls "$HOME/Documents" >/dev/null 2>&1 && echo ok || echo no`)
		switch wm.Trim(r) {
		case "ok":
			say("문서 폴더(hc 문맥): 이미 허용")
		case "no":
			say("문서 폴더(hc 문맥): 거부됨 — 아래 체크리스트 5")
		default:
			say("문서 폴더(hc 문맥): 응답 없음 — 그 맥 화면에 「herdr-core … 문서 폴더」 질문이 떠 있다. 허용한다(체크리스트 5)")
		}
	}

	// 4. 워커 연결: 이 기계가 worker.mesh에 있으면 다른 워커들과 서로 잇는다(dry-run은 --check)
	mrc := 0 // 워커 연결의 문제(1)도 최종 결과에(E03) — 체크리스트는 그대로 찍는다
	if mesh != "" {
		if inList(strings.Fields(mesh), m) {
			if mrc = meshRun(ctx, mesh, m, o.dry, d); mrc == 4 {
				return 4
			}
		} else {
			say("워커 연결: %s 은(는) worker.mesh에 없어 건너뜀 (워커면 개인 설정 worker.mesh에 더하고 worker-mac-setup mesh %s)", m, m)
		}
	}

	fmt.Println()
	for _, k := range miss {
		w := map[string]string{"worker.slots_file": "검증 자리 파일 단계를 건너뜀", "worker-checklist.txt": "개인 단계를 붙이지 않음",
			"worker.mesh": "워커 연결 단계를 건너뜀"}[k]
		fmt.Printf("※ 모름 — 설정 없음(%s): %s (%s, 예: config.example/)\n", k, w, wm.ConfigDir())
	}
	checklist(m, user, cl)
	return mrc
}

func inList(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// checklist: 끝 「사람 몫」 — 1~7 고정, 개인 단계(설정 폴더의 worker-checklist.txt)를 8번부터, 그 뒤 Xcode·점검
func checklist(m, user, cl string) {
	fmt.Printf(`━━ 사람 몫 (순서대로, 그 맥 화면 또는 이 맥에서 `+"`!`"+`로) ━━
 1. [그 맥] 시스템 설정 → 일반 → 공유 → 원격 로그인 → (i) → 「원격 사용자에 대한 전체 디스크 접근 허용」 켜기
    (worker-mac-check가 TCC 기록을 읽고, SSH 문맥 작업이 폴더 질문에 안 막힌다)
 2. [그 맥] 시스템 설정 → 개인정보 보호 및 보안 → FileVault → 끄기 (복호화 끝날 때까지 기다림. 켜져 있으면 자동 로그인 메뉴가 없다)
 3. [그 맥] 시스템 설정 → 사용자 및 그룹 → 자동으로 로그인 → %[2]s
 4. [이 맥] 관리자 단계(암호는 그 맥 계정 것, 사용자가 직접 친다):
    ! ssh -t %[1]s 'sudo /bin/bash ~/.local/state/worker-mac-setup/root-setup.sh'     → ROOT_SETUP_DONE
 5. [그 맥] 「herdr-core이(가) 문서 폴더의 파일에 접근하려고 합니다」 → 허용.
    닫혔으면 시스템 설정 → 개인정보 보호 및 보안 → 파일 및 폴더 → herdr-core → 문서 폴더 켜기
 6. [이 맥] gh 로그인(그 맥에서 따로, 토큰 복사 안 함):
    ! ssh -t %[1]s 'zsh -lic "gh auth login --hostname github.com --git-protocol https --web && gh auth setup-git"'
 7. [이 맥] Claude 로그인(그 맥에서 따로): ! ssh -t %[1]s 'zsh -lic "claude auth login"'
`, m, user)
	n := 8
	if cl != "" {
		n = personal(cl, m, n)
	}
	fmt.Printf("%2d. %s\n", n, "[그 맥] (Xcode 앱을 깐 경우만) 4의 root-setup이 라이선스를 동의한다. 깐 뒤 4를 다시 돌린다.")
	n++
	fmt.Printf("%2d. %s\n", n, "[이 맥] 점검: worker-mac-check "+m+"  — 실패 0이면 끝. 코어 문맥이 실패면 그 맥 로그인 뒤 worker-mac-setup "+m+" 을 다시 돌린다.")
	fmt.Printf(" 가방에 넣을 땐: ! ssh -t %s 'sudo pmset -a disablesleep 0'\n", m)
}

// personal: 개인 단계 파일을 n번부터 찍고 다음 번호를 돌려준다. 빈칸으로 시작하는 줄은 이어지는 줄, 빈 줄·# 줄은 건너뜀, {M}은 기계 이름
func personal(path, m string, n int) int {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: 개인 단계 파일을 못 읽음: %v\n", wm.Who, err)
		return n
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.ReplaceAll(strings.TrimSuffix(sc.Text(), "\r"), "{M}", m)
		switch {
		case l == "" || l[0] == '#':
		case strings.ContainsRune(" \t\n\v\f\r", rune(l[0])):
			fmt.Println(l)
		default:
			fmt.Printf("%2d. %s\n", n, l)
			n++
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: 개인 단계 파일을 읽다 멈춤: %v\n", wm.Who, err)
	}
	return n
}
