package main

// 착지 뒤(2026-11-24 deploy-log-rollback — 사용자 「배포나 설계를 지휘에서 자동 조정 바로바로 … 에이전트 스타일로」): 도구는 재는 것·
// 손잡이·알림만 주고 「배포할지」는 지휘가 정한다.
//   herdr-sync --landed   hc-fill 착지 뒤 명령(land-every)이 범용 저장소 착지마다 부른다(잇단 착지는 hc-fill 실행기가 한 번으로 합친다).
//                         끝 줄 「배포할 것 <k>커밋: <제목 머리들> · 병합 줄 <n>건 남음」이 지휘 사건 글이 된다. 스스로 배포가 꺼져
//                         있으면(기본) 그 줄만 찍고 끝(종료 0). 켜져 있으면 병합 줄이 빌 때까지 미루되 상한(HERDR_SYNC_AUTO_WAIT,
//                         기본 1800초, 틈 HERDR_SYNC_AUTO_POLL 기본 30초) 뒤 보통 배포를 한 번 돌리고 그 종료 코드로 끝난다
//                         (0이 아니면 hc-fill이 막힘 사건). 그 배포는 기록에 부른 곳 「스스로」로 남는다.
//   herdr-sync --auto [on|off]  스스로 배포 손잡이 — 표식 파일 STATE/auto-deploy 하나(있으면 켬). 다음 --landed부터 바로 먹는다.
//                         범위는 이 herdr-sync가 맡는 범용 저장소(HERDR_TOOLS_REPO) 하나다.
// 스스로 배포는 기본에서 공개 단계를 부르지 않는다(HERDR_SYNC_NO_PUBLISH — herdr-tools-publish 내보내기·릴리스 올림 없음, 승인 밖 공개를
// 도구가 스스로 하지 않게). 워커는 바이너리를 그 커밋의 릴리스로 받고, 릴리스는 공개 단계(검사한 커밋으로 지어 공개 저장소에 올림)와 한
// 덩이다 — 그래서 그 커밋의 릴리스가 없으면(없음·모름) 깔 수 없는 배포를 시작하지 않는다(맥에도 안 깖, 「스스로 배포 못 함」 한 줄,
// 종료 1 — sync-auto-fix, 2026-11-24 첫 실행이 맥만 깔고 워커 여덟 대가 옛 판에 남았다). publish.repo가 GitHub가 아니면(릴리스 길이
// 없음) 기계가 각자 짓는 보통 배포와 같다.
//   herdr-sync --auto-publish [on|off]  손잡이 — 표식 STATE/auto-publish(있으면 켬): 스스로 배포가 보통 herdr-sync처럼 공개 단계까지 한다.

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/proc"
	"herdr-tools/internal/tri"
)

func autoFile() string    { return STATE + "/auto-deploy" }
func autoPubFile() string { return STATE + "/auto-publish" }

func autoOn() bool    { _, err := os.Stat(autoFile()); return err == nil }
func autoPubOn() bool { _, err := os.Stat(autoPubFile()); return err == nil }

func autoPubLine() string {
	if autoPubOn() {
		return "스스로 배포가 공개 단계까지 한다 (" + autoPubFile() + ")"
	}
	return "스스로 배포 공개 단계 끔"
}

func autoLine() string {
	if autoOn() {
		return "스스로 배포 켬 (" + autoFile() + ")"
	}
	return "스스로 배포 끔"
}

// autoCmd: --auto [on|off] · --auto-publish [on|off] — 표식 파일 f 하나를 켜고 끈다
func autoCmd(args []string, f string, line func() string, journal func(bool, string)) int {
	why := ""
	if n := len(args); n == 3 && args[1] == "--why" {
		args, why = args[:1], args[2]
	}
	on := func() bool { _, err := os.Stat(f); return err == nil }
	was := on()
	switch {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "on":
		if err := os.WriteFile(f, []byte(time.Now().Format(time.RFC3339)+"\t"+deployBy()+"\n"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "herdr-sync: 표식을 쓰지 못했다 (%s): %v\n", f, err)
			return 1
		}
	case len(args) == 1 && args[0] == "off":
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "herdr-sync: 표식을 지우지 못했다 (%s): %v\n", f, err)
			return 1
		}
	default:
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	fmt.Println(line())
	if now := on(); now != was {
		journal(now, why)
	}
	return 0
}

// noRelease: 스스로 배포가 깔 수 없는 까닭 한 줄(st 없음·모름)
func noRelease(c12 string, st tri.State, why string) string {
	s := "이 커밋의 릴리스가 없다 (bin-" + c12 + ")"
	if st != tri.No {
		s = "이 커밋의 릴리스가 있는지 모른다 (bin-" + c12 + " — " + why + ")"
	}
	return "스스로 배포 못 함: " + s + " — herdr-sync로 배포하거나 herdr-sync --auto-publish on"
}

// autoGate: 스스로 배포(공개 단계 없음)를 시작해도 되나 — origin/main 커밋의 릴리스가 올라 있으면 "", 아니면 까닭 한 줄.
// 릴리스 길이 없으면(publish.repo가 GitHub가 아님) "" — 기계가 각자 짓는 보통 배포와 같다
func autoGate() string {
	slug, _ := relSlug()
	if slug == "" {
		return ""
	}
	note := fetchMain()
	om, rc := gitq("-C", REPO, "rev-parse", "-q", "--verify", "refs/remotes/origin/main^{commit}")
	if rc != 0 || !hex40.MatchString(om) {
		return "스스로 배포 못 함: origin/main을 읽지 못했다" + note + " — herdr-sync로 배포한다"
	}
	_, rc, errs := gh("release", "view", "bin-"+om[:12], "--repo", slug, "--json", "assets", "--jq", ".assets[].name")
	switch {
	case rc == 0:
		return ""
	case strings.TrimSpace(errs) == "release not found":
		return noRelease(om[:12], tri.No, "")
	}
	return noRelease(om[:12], tri.Unknown, "gh release view 실패(인증·통신·시간 넘김)")
}

// envSecs: 초(소수 받음), 없거나 틀리면 def
func envSecs(k string, def float64) time.Duration {
	if v, err := strconv.ParseFloat(os.Getenv(k), 64); err == nil && v >= 0 {
		def = v
	}
	return time.Duration(def * float64(time.Second))
}

// landed: --landed 본체
func landed() int {
	log, err := readLog()
	if err != nil {
		fmt.Fprintf(os.Stderr, "herdr-sync: 배포 기록을 읽지 못했다 (%s): %v\n", deployLogFile(), err)
		return 1
	}
	note := fetchMain()
	line, k := pendingLine(macCommit(log))
	if !autoOn() || k == 0 {
		fmt.Println(line + note)
		return 0
	}
	poll := envSecs("HERDR_SYNC_AUTO_POLL", 30)
	end := time.Now().Add(envSecs("HERDR_SYNC_AUTO_WAIT", 1800))
	for n := queueLeft(); n != 0; n = queueLeft() {
		if !time.Now().Before(end) {
			fmt.Printf("병합 줄이 %s 남았지만 기다림 상한 — 지금 배포한다\n", countText(n, "건"))
			break
		}
		time.Sleep(poll)
	}
	pub := autoPubOn()
	if !pub {
		if g := autoGate(); g != "" {
			fmt.Println(g + " · " + line)
			return 1
		}
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "herdr-sync: 자기 경로를 모른다: %v\n", err)
		return 1
	}
	rc := 0
	for {
		c := exec.Command(self, "--why", "착지 뒤 스스로 배포 — "+line)
		c.Env = append(os.Environ(), "HERDR_SYNC_BY=스스로")
		if !pub {
			c.Env = append(c.Env, "HERDR_SYNC_NO_PUBLISH=1")
		}
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		// 따로 무리로 기다린다 — 신호를 받으면 onSignals의 StopCurrent가 그 배포까지 거둔다
		if rc = proc.WaitMax(c, "", 0); rc != 75 || !time.Now().Before(end) {
			break // 75 = 다른 herdr-sync가 돈다 — 상한까지 다시
		}
		time.Sleep(poll)
	}
	if rc == 0 {
		fmt.Println("스스로 배포 끝 — " + line + " (기록 herdr-sync --log)")
	} else {
		fmt.Printf("스스로 배포 실패 (종료 %d) — %s (기록 herdr-sync --log)\n", rc, line)
	}
	return rc
}

// autoRelease: 스스로 배포 — 올려 둔 bin-<커밋 12자>를 받는다(올리지 않음). 없거나 모르면 아무 데도 깔기 전에 멈춘다(종료 1 —
// --landed의 확인과 이 배포 사이에 릴리스 없는 착지가 끼어도 반쪽 배포를 남기지 않게). 릴리스 길이 없으면 기계가 각자 짓는다
func autoRelease(rel *relInfo) *relInfo {
	slug, _ := relSlug()
	if slug == "" {
		return rel
	}
	r, st, why := fetchRelease(slug, rel.c12, rel.src)
	if st != tri.Yes {
		die("%s", noRelease(rel.c12, st, why))
	}
	r.at, r.files = false, rel.files
	return r
}

// modes: 읽기·손잡이 모드(--log·--status·--auto·--landed) — 맥 확인 뒤, 잠금 전에(도는 배포를 막지 않게). 아니면 ok 거짓
func modes(a, all []string) (int, bool) {
	if len(a) == 0 {
		return 0, false
	}
	switch a[0] {
	case "--log":
		return showLog(a[1:]), true
	case "--auto":
		return autoCmd(a[1:], autoFile(), autoLine, autoJournal), true
	case "--auto-publish":
		return autoCmd(a[1:], autoPubFile(), autoPubLine, autoPubJournal), true
	case "--status":
		ms := a[1:]
		if len(ms) == 0 {
			ms = all
		}
		return showStatus(ms), true
	case "--landed":
		if len(a) > 1 {
			fmt.Fprint(os.Stderr, usage)
			return 2, true
		}
		onSignals()
		return landed(), true
	}
	return 0, false
}
