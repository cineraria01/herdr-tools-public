package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
)

// 재감사 2바퀴 fill R-212·R-222: 착지 뒤 명령이 정상 바퀴(onlandTick → 실행기 바이너리 _onland)를 거쳐 정확히 한 번 돌고,
// 실행기를 못 띄우거나 실행기가 명령 중에 죽어도 다음 바퀴·다음 실행기가 남은 차례를 걷는다

func TestMain(m *testing.M) { clitest.Main(m) }

// onlandReal: 진짜 실행기(hc-fill 바이너리)가 쓰는 설정·데이터 폴더를 이 시험의 전역과 같게. 명령은 ran에 한 줄씩 적는다
func onlandReal(t *testing.T, repo, cmdline string) (rec string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	rec = onlandEnv(t, repo, cmdline)
	cfg := t.TempDir()
	if err := os.WriteFile(cfg+"/hc-fill.tsv", []byte(strings.Join(confLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HERDR_TOOLS_CONFIG", cfg)
	t.Setenv("HC_FILL_DATA", DATA)
	SELF = clitest.Bin(t, "hc-fill")
	return rec
}

// onlandLanded: origin·로컬 클론을 만들고 접수(onland.tsv 줄 fix-a dev app t9) 뒤 fix-a 착지 커밋을 origin에 올린다
func onlandLanded(t *testing.T, d string) (local, sha string) {
	t.Helper()
	origin, up := d+"/o.git", d+"/up"
	local = d + "/local"
	gitT(t, d, nil, "init", "-q", "--bare", "-b", "main", origin)
	gitT(t, d, nil, "clone", "-q", origin, up)
	gitT(t, up, nil, "commit", "-q", "--allow-empty", "-m", "base")
	gitT(t, up, nil, "push", "-q", "origin", "main")
	gitT(t, d, nil, "clone", "-q", origin, local)
	gitT(t, up, nil, "commit", "-q", "--allow-empty", "-m", "fix-a: 착지")
	gitT(t, up, nil, "push", "-q", "origin", "main")
	return local, gitT(t, up, nil, "rev-parse", "HEAD")
}

func onlandQueue(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(DATA+"/onland.tsv", []byte(fmt.Sprintf("fix-a\tdev\tapp\tt9\t%d\n", nowUnix()-5)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// onlandSettled: 명령 기록 n줄·사건 ev줄이 생기고 차례(pending·running)가 빌 때까지
func onlandSettled(t *testing.T, ran, rec string, n, ev int) {
	t.Helper()
	for end := time.Now().Add(60 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		r, _ := readFile(ran)
		e, _ := readFile(rec)
		if len(lines(r)) == n && len(lines(e)) == ev && !nonEmpty(onlandDir()+"/dev.pending") && !nonEmpty(onlandDir()+"/dev.running") {
			return
		}
		if time.Now().After(end) {
			o, _ := readFile(onlandDir() + "/dev.out")
			t.Fatalf("명령 %q · 사건 %q · pending %v (바람 명령 %d·사건 %d)\n실행기 출력:\n%s", r, e, nonEmpty(onlandDir()+"/dev.pending"), n, ev, o)
		}
	}
}

// R-222: 정상 바퀴가 실행기를 띄워 명령이 정확히 한 번 돌고, 주인에게 성공 사건 하나, 차례가 빈다. 다음 바퀴는 다시 돌리지 않는다
// (onlandTick의 onlandSpawn 호출을 빼면 명령이 안 돌아 실패한다)
func TestOnlandTickRunsCommandOnce(t *testing.T) {
	d := t.TempDir()
	ran := d + "/ran"
	local, sha := onlandLanded(t, d)
	rec := onlandReal(t, local, `echo "$HC_ONLAND_LANES $HC_ONLAND_SHA" >> `+ran)
	onlandQueue(t)
	onlandTick()
	clitest.WaitFile(t, ran, 60*time.Second)
	onlandSettled(t, ran, rec, 1, 1)
	FETCHED = ""
	onlandTick()
	time.Sleep(500 * time.Millisecond) // 다음 바퀴가 (잘못) 띄운 실행기가 있다면 돌 틈
	onlandSettled(t, ran, rec, 1, 1)
	if r, _ := readFile(ran); r != "fix-a "+sha+"\n" {
		t.Fatalf("명령 = %q", r)
	}
	if e, _ := readFile(rec); !strings.HasPrefix(e, "report|중간|-|착지 뒤 dev 성공") || !strings.Contains(e, "|--name|on-land-dev|--to|t9|") {
		t.Fatalf("사건 = %q", e)
	}
}

// R-212 ①: 실행기를 못 띄운 바퀴(실행기 출력 파일을 못 엶) 뒤 원인이 없어지면, 새 착지가 없어도 다음 바퀴가 남은 pending을 걷는다
func TestOnlandTickRecoversPendingAfterSpawnFail(t *testing.T) {
	d := t.TempDir()
	ran := d + "/ran"
	local, _ := onlandLanded(t, d)
	rec := onlandReal(t, local, "echo x >> "+ran)
	onlandQueue(t)
	block := onlandDir() + "/dev.out"
	if err := os.MkdirAll(block, 0o755); err != nil { // 폴더라 O_WRONLY로 못 연다
		t.Fatal(err)
	}
	onlandTick()
	if !nonEmpty(onlandDir()+"/dev.pending") || nonEmpty(DATA+"/onland.tsv") {
		t.Fatalf("준비: 첫 바퀴 뒤 pending %v · onland.tsv %v (바람 pending 있음·장부 빔)", nonEmpty(onlandDir()+"/dev.pending"), nonEmpty(DATA+"/onland.tsv"))
	}
	if !clitest.Absent(t, ran) {
		t.Fatal("준비: 실행기를 못 띄웠는데 명령이 돎")
	}
	if err := os.Remove(block); err != nil {
		t.Fatal(err)
	}
	FETCHED = ""
	onlandTick()
	onlandSettled(t, ran, rec, 1, 1)
}

// R-212 ②: 실행기가 명령 중에 죽으면(KILL) 그 묶음은 「실행 중」 기록으로 남아, 다음 실행기가 앞 명령이 끝난 뒤 한 번 더 돌린다
// (명령 전에 pending을 지우면 잃는다)
func TestOnlandRunRecoversAfterRunnerKilled(t *testing.T) {
	d := t.TempDir()
	ran, flag := d+"/ran", d+"/killed"
	rec := onlandReal(t, d, fmt.Sprintf(`echo x >> %s; if [ ! -e %s ]; then touch %s; kill -KILL $PPID; fi`, ran, flag, flag))
	onlandPoll = 20 * time.Millisecond
	t.Cleanup(func() { onlandPoll = time.Second })
	if err := onlandPend("dev", "fix-a", "t9", "s1"); err != nil {
		t.Fatal(err)
	}
	c := exec.Command(SELF, "_onland", "dev")
	ch := clitest.Spawn(t, c)
	<-ch.Done()
	clitest.WaitFile(t, flag, 10*time.Second) // 원인(명령 중 실행기 KILL)이 일어났다
	if r, _ := readFile(ran); r != "x\n" {
		t.Fatalf("준비: 첫 실행 명령 = %q", r)
	}
	if e, _ := readFile(rec); e != "" {
		t.Fatalf("준비: 죽은 실행기가 사건을 남김 %q", e)
	}
	if rc := onlandRun("dev"); rc != 0 {
		t.Fatalf("다음 실행기 rc %d", rc)
	}
	onlandSettled(t, ran, rec, 2, 1) // 죽은 실행기는 사건을 못 남김 — 다시 돈 묶음의 사건 하나
}
