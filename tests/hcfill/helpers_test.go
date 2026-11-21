package hcfill_test

// 여러 절이 같이 쓰는 도우미: 받은편지(10c~)·자동 감사(10h~)·대기열 칸.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// --- 받은편지(셸 시험 10c의 IS·IB·IH·RP·LED·RPL과 ifresh·req·rid·lst·ino) ---

func (k *K) IS() string  { return k.T + "/in-slow" }
func (k *K) IB() string  { return k.T + "/in-box2" }
func (k *K) IH() string  { return k.DATA + "/inbox" }
func (k *K) RP() string  { return k.DATA + "/reports" }
func (k *K) LED() string { return k.DATA + "/inbox.tsv" }
func (k *K) RPL() string { return k.DATA + "/replies" }

// ifresh: fresh + 받은편지 폴더 비우기 + 설정에 inbox 줄 셋(slow·box-2·shared ~/…).
func (k *K) ifresh() {
	k.fresh()
	rm(k.IS(), k.IB(), k.RP(), k.IH())
	k.cpConf()
	k.confAppend("inbox slow "+k.IS(), "inbox box-2 "+k.IB(), "inbox shared ~/.local/share/hc-fill/inbox")
}

// req <폴더> <이름> <project> [after] [only]: 지시문 먼저, .req는 마지막에 mv. after가 "-"면 안 적는다.
func (k *K) req(dir, name, project string, opt ...string) {
	write(k.t, dir+"/"+name+".brief.md", "# 요청 "+name+"\n")
	s := "project\t" + project + "\n"
	if len(opt) > 0 && opt[0] != "" && opt[0] != "-" {
		s += "after\t" + opt[0] + "\n"
	}
	if len(opt) > 1 && opt[1] != "" {
		s += "only\t" + opt[1] + "\n"
	}
	write(k.t, dir+"/"+name+".req.tmp", s)
	if err := os.Rename(dir+"/"+name+".req.tmp", dir+"/"+name+".req"); err != nil {
		k.t.Fatal(err)
	}
}

// rid: 그 줄의 요청 ID(11칸의 # 뒤).
func (k *K) rid(name string) string {
	r := k.qcol(name, 11)
	if i := strings.LastIndex(r, "#"); i >= 0 {
		return r[i+1:]
	}
	return r
}

// lst: 장부(inbox.tsv)에서 그 요청 ID의 마지막 상태(4칸).
func (k *K) lst(id string) string {
	s := ""
	for _, r := range rows(k.LED()) {
		if col(r, 1) == id {
			s = col(r, 4)
		}
	}
	return s
}

// ino: 파일의 inode(stat -f %i, 없으면 "").
func ino(p string) string {
	fi, err := os.Stat(p)
	if err != nil {
		return ""
	}
	return strconv.FormatUint(fi.Sys().(*syscall.Stat_t).Ino, 10)
}

// --- 자동 감사(셸 시험 10h의 AT·VD·ABR과 afresh·ev·ev10·s40·aidof·acol·verd·vkey·nrep) ---

func (k *K) AT() string  { return k.DATA + "/audit.tsv" }
func (k *K) VD() string  { return k.DATA + "/verdicts" }
func (k *K) ABR() string { return k.DATA + "/briefs/audit" }

// afresh: fresh + 사건·보낸 기록·레인 기록 비우기, 설정에 audit 줄, 사건 시각 0부터.
func (k *K) afresh() {
	k.fresh()
	rm(k.T+"/events", k.T+"/events.log", k.T+"/send.log", k.LT)
	k.cpConf()
	k.confAppend("audit    " + k.EXA)
	k.ek = 0
	k.unset("HC_FAKE_CREATED", "HC_FAKE_REVNONE", "HC_FAKE_SHA40", "HC_FILL_CRASH_AT")
}

func (k *K) evTime() string {
	k.ek++
	return fmt.Sprintf("2026-11-%02dT%02d:%02d:00Z", 19+k.ek/1440, k.ek/60%24, k.ek%60)
}

// ev <터미널> <이름> <상태> <sha> <한 줄>: $T/events에 사건 한 줄(8칸).
func (k *K) ev(term, name, state, sha, msg string) {
	appendFile(k.t, k.T+"/events", strings.Join([]string{k.evTime(), "m", term, name, state, sha, msg, "1"}, "\t")+"\n")
}

// ev10: ev + 발행 실행(10칸).
func (k *K) ev10(term, name, state, sha, msg, run string) {
	appendFile(k.t, k.T+"/events", strings.Join([]string{k.evTime(), "m", term, name, state, sha, msg, "1", "-", run}, "\t")+"\n")
}

// s40: 가짜 git이 푸는 40자(뒤를 0으로).
func s40(s string) string { return s + strings.Repeat("0", 40-len(s)) }

// aidof: 그 레인의 마지막 감사 차수(audit.tsv 17칸 이상 줄의 2칸이 레인).
func (k *K) aidof(lane string) string {
	a := ""
	for _, r := range rows(k.AT()) {
		if len(r) >= 17 && col(r, 2) == lane {
			a = col(r, 1)
		}
	}
	return a
}

// acol: 차수 id 줄의 c칸(마지막 줄).
func (k *K) acol(id string, c int) string {
	v := ""
	for _, r := range rows(k.AT()) {
		if len(r) >= 17 && col(r, 1) == id {
			v = col(r, c)
		}
	}
	return v
}

// verd <감사 터미널> <ID> <높음> <중간> [<낮음>]: 감사 판정 사건.
func (k *K) verd(term, id, high, mid string, low ...string) {
	l := "0"
	if len(low) > 0 && low[0] != "" {
		l = low[0]
	}
	k.ev(term, "audit-"+id, "판정", k.acol(id, 5), "감사 완료 "+id+" 높음 "+high+"·중간 "+mid+"·낮음 "+l)
}

// vkey <레인> <키> <ID>: 판정 파일 verdicts/<레인>/<ID>.md에서 그 키 줄의 값.
func (k *K) vkey(lane, key, id string) string {
	v := ""
	for _, r := range rows(k.VD() + "/" + lane + "/" + id + ".md") {
		if col(r, 1) == key {
			v += col(r, 2) + "\n"
		}
	}
	return strings.TrimSuffix(v, "\n")
}

// nrep: report.log에서 그 글이 든 사건 수.
func (k *K) nrep(s string) int { return countF(k.T+"/report.log", s) }

// --- 대기열 칸 ---

func (k *K) col14(name string) string { return k.qcol(name, 14) }
func (k *K) col15(name string) string { return k.qcol(name, 15) }
func (k *K) col16(name string) string { return k.qcol(name, 16) }
