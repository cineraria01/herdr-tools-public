package main

// 상태 한 장의 배포 칸(sync-local-name, 2026-11-24): herdr-sync가 이 기계를 설정 local 이름으로 적기 전 기록(「맥」)과 뒤 기록(local)을
// 한 기계로 읽는다 — 같은 기계가 두 줄로 보이지 않게
import (
	"strings"
	"testing"
)

func TestDeployLocalAlias(t *testing.T) {
	defer func(o string) { LOCAL = o }(LOCAL)
	ls := []string{
		"2026-11-24T10:00:00+09:00\t배포\t0\taaaaaaaaaaaa\t-\tcccccccccccc\t-\t맥=씀 w1=씀",
		"2026-11-24T11:00:00+09:00\t배포\t1\tbbbbbbbbbbbb\t-\tcccccccccccc\t-\tbox-1=실패 w1=씀",
		"2026-11-24T12:00:00+09:00\t배포\t0\tdddddddddddd\t-\tcccccccccccc\t-\tw1=건너뜀",
	}
	LOCAL = "box-1"
	ver, order, _ := deployInstalled(ls)
	if strings.Join(order, " ") != "box-1 w1" || ver["box-1"] != "aaaaaaaaaaaa" || ver["w1"] != "bbbbbbbbbbbb" {
		t.Fatalf("옛 「맥」 줄과 새 이름 줄이 한 기계여야: %v %v", order, ver)
	}
	if sk := deploySkipOnly(ls); sk["box-1"] || sk["맥"] {
		t.Fatalf("건너뜀뿐: %v", sk)
	}
	LOCAL = ""
	if _, order, _ := deployInstalled(ls); strings.Join(order, " ") != "맥 w1 box-1" {
		t.Fatalf("local이 없으면 적힌 이름 그대로: %v", order)
	}
}
