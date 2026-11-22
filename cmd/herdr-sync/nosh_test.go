package main

import "testing"

// ops/R-3A13: hc는 sh 부재와 그 밖의 띄우기 실패에 같은 kind·머리말을 낸다. 부재(program not found·os error 2·Windows 경로 없음 3)만 참,
// 잠깐의 띄우기 실패(35·24·13)·잘린 JSON·다른 kind·머리말 아닌 글은 거짓(연결 실패)
func TestNosh(t *testing.T) {
	j := func(kind, msg string) string {
		return `{"error":{"hint":"check the program","kind":"` + kind + `","message":"` + msg + `"}}`
	}
	for _, c := range []struct {
		e    string
		want bool
	}{
		{`{"error":{"hint":"","kind":"spawn_failed","message":"cannot start sh: program not found"}}`, true},
		{j("spawn_failed", "cannot start sh: No such file or directory (os error 2)"), true},
		{j("spawn_failed", "cannot start sh: The system cannot find the path specified. (os error 3)"), true},
		{j("spawn_failed", "cannot start sh: Resource temporarily unavailable (os error 35)"), false},
		{j("spawn_failed", "cannot start sh: Too many open files (os error 24)"), false},
		{j("spawn_failed", "cannot start sh: Permission denied (os error 13)"), false},
		{`{"error":{"hint":"","kind":"spawn_failed","message":"cannot start sh: program not fo`, false},
		{j("connect_failed", "cannot start sh: program not found"), false},
		{j("spawn_failed", "x cannot start sh: program not found"), false},
		{j("spawn_failed", "cannot start sh: program not found") + "\n" + j("spawn_failed", "cannot start sh: program not found"), false},
		{"", false},
	} {
		if got := nosh(c.e); got != c.want {
			t.Errorf("nosh(%s) = %v, 기대 %v", c.e, got, c.want)
		}
	}
}
