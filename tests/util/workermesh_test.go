package util_test

// 옛 tests/worker-mesh.test.sh: worker-mac-setup mesh(워커끼리 ssh·hc 연결) 시험. 가짜 ssh·ssh-keyscan·tailscale·hc,
// 기계마다 임시 HOME($T/h/<기계>). ssh-keygen은 진짜를 쓴다(임시 키). 실제 기계·네트워크는 쓰지 않는다.
// 다른 사본은 WORKER_MAC_BIN=<bin 폴더>.

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestWorkerMesh(t *testing.T) { eachImpl(t, "worker-mac-setup", testWorkerMesh) }

func testWorkerMesh(t *testing.T, T string, e Env) {
	BIN := workerMacBin()
	TLOG, HOME := T+"/log", T+"/mac"
	mkdirs(t, T+"/bin", TLOG, HOME+"/.ssh", T+"/ip", T+"/conf")
	os.Symlink(BIN+"/herdr-tools-config", T+"/bin/herdr-tools-config")
	e = e.With("T="+T, "T_LOG="+TLOG, "HOME="+HOME, "HERDR_TOOLS_CONFIG="+T+"/conf", "PATH="+T+"/bin:"+e["PATH"])

	// 가짜 ssh. 맥(HOME=$T/mac)에서: 기계 이름을 ssh.log에, 그 기계 HOME에서 sh -c. 없는 기계는 255.
	// 워커에서: -G <이름>은 ~/.ssh/config Host 블록의 hostname·user, 접속은 config로 주소를 풀어
	// 그 기계 known_hosts에 주소가 있고 받는 기계 authorized_keys에 내 공개 키가 있을 때만 0(conn.log에 「보낸→받은」).
	script(t, T+"/bin/ssh", T, `#!/bin/bash
G=0
while [ $# -gt 0 ]; do case $1 in -o) shift 2 ;; -n|-t) shift ;; -G) G=1; shift ;; *) break ;; esac; done
cfg() { awk -v h="$1" -v k="$2" 'tolower($1)=="host"{m=0; for(i=2;i<=NF;i++) if($i==h) m=1; next} m && tolower($1)==k {print $2; exit}' "$HOME/.ssh/config" 2>/dev/null; }
if [ "$HOME" = "$T/mac" ]; then
  echo "$1" >>"$T_LOG/ssh.log"
  [ -d "$T/h/$1" ] || exit 255
  m=$1; shift
  cd "$T/h/$m" && HOME=$T/h/$m PATH=$T/h/$m/.local/bin:/usr/bin:/bin exec /bin/sh -c "$*"
fi
h=$(cfg "$1" hostname); u=$(cfg "$1" user)
if [ $G = 1 ]; then echo "hostname ${h:-$1}"; echo "user ${u:-$(id -un)}"; exit 0; fi
[ -n "$h" ] && [ -f "$T/ip/$h" ] || exit 255
to=$(cat "$T/ip/$h")
ssh-keygen -F "$h" -f "$HOME/.ssh/known_hosts" >/dev/null 2>&1 || exit 255
body=$(awk '{print $2}' "$HOME/.ssh/id_ed25519.pub")
grep -qF "$body" "$T/h/$to/.ssh/authorized_keys" 2>/dev/null || exit 255
echo "${HOME##*/}→$to" >>"$T_LOG/conn.log"
`)
	// 가짜 ssh-keyscan: 마지막 인자 주소의 호스트 키 줄($T/hk/<주소>)
	script(t, T+"/bin/ssh-keyscan", T, "#!/bin/bash\nfor a; do :; done\ncat \"$T/hk/$a\" 2>/dev/null\n")
	// 가짜 tailscale: ip -4 / ip -6 → ~/.ts4 / ~/.ts6
	script(t, T+"/bin/tailscale", T, "#!/bin/bash\n[ \"$1\" = ip ] && cat \"$HOME/.ts${2#-}\"\n")
	// 가짜 hc(워커 쪽): machine ls는 ~/.hcm 줄, machine add <대상> --name <이름>은 ~/.hcm에 더함
	script(t, T+"/bin/hc", T, `#!/bin/bash
f=$HOME/.hcm
case "$1 $2" in
  "machine ls") printf '{"machines":[%s]}\n' "$(paste -sd, "$f" 2>/dev/null)" ;;
  "machine add") echo "${HOME##*/} $*" >>"$T_LOG/hc-add.log"; printf '{"name":"%s","state":"connected"}\n' "$5" >>"$f" ;;
  *) exit 2 ;;
esac
`)

	// 기계 하나: HOME, Tailscale 주소, 호스트 키(진짜 ed25519), 가짜 명령 링크. 맥 known_hosts에 그 호스트 키
	host := func(name, ip4, ip6 string) {
		bash(t, T, e, `d=$T/h/$1
mkdir -p "$d/.local/bin" "$T/hk"
for c in ssh ssh-keyscan tailscale hc; do ln -sf "$T/bin/$c" "$d/.local/bin/$c"; done
echo "$2" >"$d/.ts4"; echo "$3" >"$d/.ts6"; echo "$1" >"$T/ip/$2"
ssh-keygen -q -t ed25519 -N '' -C hostkey -f "$T/hk/$1" </dev/null
printf '%s %s\n' "$2" "$(cut -d' ' -f1,2 "$T/hk/$1.pub")" >"$T/hk/$2"
cat "$T/hk/$2" >>"$HOME/.ssh/known_hosts"`, name, ip4, ip6)
	}
	fresh := func() {
		for _, d := range []string{T + "/h", T + "/hk", T + "/ip", TLOG} {
			os.RemoveAll(d)
		}
		mkdirs(t, T+"/h", T+"/ip", TLOG)
		write(t, HOME+"/.ssh/known_hosts", "")
		host("wa", "192.0.2.1", "2001:db8::1")
		host("wb", "192.0.2.2", "2001:db8::2")
		host("wc", "192.0.2.3", "2001:db8::3")
		host("loopbox", "192.0.2.8", "2001:db8::8")
		host("linbox", "192.0.2.9", "2001:db8::9")
		write(t, T+"/conf/config", "worker.mesh  wa wb wc\n")
	}

	var out string
	var rc int
	c := checker{t: t, out: &out, rc: &rc}
	has := func(s string) bool { return strings.Contains(out, s) }
	mesh := func(args ...string) {
		out, rc = run(T, e, BIN+"/worker-mac-setup", append([]string{"mesh"}, args...)...)
	}
	pubbody := func(m string) string {
		f := strings.Fields(read(T + "/h/" + m + "/.ssh/id_ed25519.pub"))
		if len(f) < 2 {
			return "\x00없음"
		}
		return f[1]
	}
	// snap: 모든 기계의 .ssh 아래 파일 내용 요약(cksum 대신 sha256)
	snap := func() string {
		var l []string
		filepath.Walk(T+"/h", func(p string, fi os.FileInfo, err error) error {
			if err == nil && fi.Mode().IsRegular() && strings.Contains(p, "/.ssh/") {
				l = append(l, fmt.Sprintf("%x %s", sha256.Sum256([]byte(read(p))), p))
			}
			return nil
		})
		sort.Strings(l)
		return strings.Join(l, "\n")
	}
	keygen := func(args ...string) string { o, _ := exec.Command("ssh-keygen", args...).Output(); return string(o) }
	fp := func(s string) string { // ssh-keygen -lf의 둘째 칸
		f := strings.Fields(s)
		if len(f) < 2 {
			return ""
		}
		return f[1]
	}
	addLog := func() string { return read(TLOG + "/hc-add.log") }

	fresh()
	mesh("--check")
	c.ok("mesh --check: 아무것도 없으면 종료 1, 바꾸지 않음", rc == 1 && !exists(T+"/h/wa/.ssh/id_ed25519") && !exists(T+"/h/wb/.ssh/authorized_keys") && has("wa → wb"))

	mesh()
	c.ok("mesh: 성공", rc == 0 && has("6방향"))
	c.ok("mesh: 기계마다 키를 만들고 주석은 <기계>-workers-<날짜>", exists(T+"/h/wa/.ssh/id_ed25519") && grep(read(T+"/h/wa/.ssh/id_ed25519.pub"), ` wa-workers-[0-9]{8}$`))
	c.ok("mesh: 받는 쪽 authorized_keys에 from=보낸 기계 Tailscale 주소",
		strings.Contains(read(T+"/h/wb/.ssh/authorized_keys"), `from="192.0.2.1,2001:db8::1",no-agent-forwarding,no-X11-forwarding ssh-ed25519 `+pubbody("wa")+" "))
	c.ok("mesh: ~/.ssh/config Host 블록(HostName Tailscale, User 그 기계 계정)", func() bool {
		o, _ := run(T, e.With("HOME="+T+"/h/wa"), T+"/bin/ssh", "-G", "wc")
		u, _ := exec.Command("id", "-un").Output()
		return strings.ReplaceAll(o, "\n", " ") == "hostname 192.0.2.3 user "+strings.TrimSpace(string(u))+" "
	}())
	c.ok("mesh: known_hosts는 맥과 같은 지문", func() bool {
		var kept []string
		for _, l := range strings.Split(keygen("-F", "192.0.2.2", "-f", T+"/h/wa/.ssh/known_hosts"), "\n") {
			if l != "" && !strings.HasPrefix(l, "#") {
				kept = append(kept, l)
			}
		}
		cmd := exec.Command("ssh-keygen", "-lf", "-")
		cmd.Stdin = strings.NewReader(strings.Join(kept, "\n") + "\n")
		got, _ := cmd.Output()
		want := fp(keygen("-lf", T+"/hk/wb.pub"))
		return want != "" && fp(string(got)) == want
	}())
	c.ok("mesh: 워커마다 hc machine add <이름> --name <이름>", hasLine(addLog(), "wa machine add wb --name wb") && lines(addLog()) == 6)
	c.ok("mesh: 6방향 접속 시험", func() bool {
		u := map[string]bool{}
		for _, l := range strings.Split(read(TLOG+"/conn.log"), "\n") {
			if l != "" {
				u[l] = true
			}
		}
		return len(u) == 6
	}())
	c.ok("mesh: 키 내용은 찍지 않음", !has(pubbody("wa")) && !has(pubbody("wc")))
	c.ok("mesh: 제외 기계(루프·리눅스)는 건드리지 않음", func() bool {
		for _, f := range []string{"ssh.log", "hc-add.log", "conn.log"} {
			if grep(read(TLOG+"/"+f), "loopbox|linbox") {
				return false
			}
		}
		return !exists(T+"/h/loopbox/.ssh") && !exists(T+"/h/linbox/.ssh") && !strings.Contains(read(T+"/h/wa/.ssh/config"), "loopbox")
	}())

	s1 := snap()
	mesh()
	s2 := snap()
	c.ok("mesh: 두 번째는 바꾸지 않음(멱등)", rc == 0 && s1 == s2 && lines(addLog()) == 6)
	mesh("--check")
	c.ok("mesh --check: 다 있으면 「모두 있음」 종료 0", rc == 0 && has("모두 있음 (6방향)"))

	// 새 워커 wd를 더함: 처음 있던 파일은 .bak-mesh-<날짜>로 한 번 남기고, wd와 얽힌 방향만 더한다
	host("wd", "192.0.2.4", "2001:db8::4")
	write(t, T+"/conf/config", "worker.mesh  wa wb wc wd\n")
	write(t, T+"/h/wa/.ssh/config", read(T+"/h/wa/.ssh/config")+"# 손으로 둔 줄\n")
	write(t, TLOG+"/hc-add.log", "")
	mesh("wd")
	c.ok("mesh <기계>: 그 기계와 얽힌 6방향만", rc == 0 && has("6방향") && lines(addLog()) == 6 && !strings.Contains(addLog(), "wa machine add wb"))
	c.ok("mesh: 고친 파일의 처음 판을 .bak-mesh-<날짜>로", func() bool {
		var names []string
		ents, _ := os.ReadDir(T + "/h/wa/.ssh")
		for _, en := range ents {
			names = append(names, en.Name())
		}
		ls := strings.Join(names, "\n")
		return grep(ls, `^config\.bak-mesh-[0-9]{8}$`) && grep(ls, `^authorized_keys\.bak-mesh-`)
	}())
	mesh("--check")
	c.ok("mesh --check: 넷이면 12방향 모두 있음", rc == 0 && has("모두 있음 (12방향)"))

	// 지문 불일치: 맥이 아는 wb 호스트 키와 실제(ssh-keyscan)가 다르면 known_hosts에 넣지 않고 멈춘다
	fresh()
	bash(t, T, e, `ssh-keygen -q -t ed25519 -N '' -f "$T/other" </dev/null
printf '192.0.2.2 %s\n' "$(cut -d' ' -f1,2 "$T/other.pub")" >"$T/hk/192.0.2.2"`)
	mesh()
	c.ok("mesh: 지문이 다르면 종료 4로 멈춤", rc == 4 && has("지문"))
	c.ok("mesh: 다른 지문은 known_hosts에 넣지 않음", func() bool {
		other := strings.Fields(read(T + "/other.pub"))[1]
		ms, _ := filepath.Glob(T + "/h/*/.ssh/known_hosts")
		for _, m := range ms {
			if strings.Contains(read(m), other) {
				return false
			}
		}
		return true
	}())

	// 맥 known_hosts에 그 기계가 없으면 대조할 수 없으니 바꾸기 전에 멈춘다
	fresh()
	var kh []string
	for _, l := range strings.SplitAfter(read(HOME+"/.ssh/known_hosts"), "\n") {
		if l != "" && !strings.HasPrefix(l, "192.0.2.3 ") {
			kh = append(kh, l)
		}
	}
	write(t, HOME+"/.ssh/known_hosts", strings.Join(kh, ""))
	mesh()
	c.ok("mesh: 맥 known_hosts에 없으면 종료 4, 아무것도 바꾸지 않음", rc == 4 && !exists(T+"/h/wa/.ssh/authorized_keys") && !exists(T+"/h/wa/.ssh/id_ed25519"))

	// 설정·인자
	os.Remove(T + "/conf/config")
	mesh("--check")
	c.ok("mesh: worker.mesh 설정이 없으면 종료 2", rc == 2 && has("worker.mesh"))
	fresh()
	mesh("loopbox")
	c.ok("mesh: 목록 밖 기계를 주면 종료 64, 접속 안 함", rc == 64 && read(TLOG+"/ssh.log") == "")
}
