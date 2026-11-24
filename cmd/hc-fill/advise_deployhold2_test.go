package main

// 감사 atozuyd-gi866o: [중간] 보류가 이미 알린 「배포 밀림」 열쇠를 잊게 해, 계속 밀린 기계는 착지마다 같은 사건이 다시 나갔다 —
// 알림 → 다른 착지로 보류 → 유예 뒤에도 같은 밀림이면 사건은 하나뿐. [낮음] 배포가 아닌 착지 뒤 명령(빌드)은 보류 까닭이 아니다.
// 고치기 전 코드로는 두 번째 사건이 나가고(열쇠를 잊음), 빌드 차례만으로도 밀림이 가려져 실패한다

import (
	"os"
	"testing"
	"time"
)

func TestAdviseDeployHoldKeepsKnownKey(t *testing.T) {
	dhFx(t)
	now := time.Now()
	head := onlandDir() + "/deploy.head"
	old := now.Add(-time.Hour)
	if err := os.WriteFile(head, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(head, old, old); err != nil {
		t.Fatal(err)
	}
	adviseTick(now)
	if len(reports(t)) != 1 || !adviseKnown()["deploy:m"] {
		t.Fatalf("밀림이 알려져야: %q", readLines(DATA+"/advise.tsv"))
	}
	t1 := now.Add(6 * time.Minute) // 다른 레인 착지 — 보류
	if err := os.Chtimes(head, t1, t1); err != nil {
		t.Fatal(err)
	}
	adviseTick(t1)
	if !adviseKnown()["deploy:m"] {
		t.Fatalf("보류 동안 알린 열쇠를 잊음: %q", readLines(DATA+"/advise.tsv"))
	}
	adviseTick(t1.Add(6 * time.Minute))
	if r := reports(t); len(r) != 1 {
		t.Fatalf("같은 밀림 사건이 다시 나감: %q", r)
	}
}

func TestAdviseDeployHoldIgnoresNonDeployOnland(t *testing.T) {
	dhFx(t)
	if err := os.WriteFile(onlandDir()+"/build.pending", []byte("lane-a\t-\tabc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(onlandDir()+"/build.head", []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if it := deployItems(time.Now()); len(it) != 1 {
		t.Fatalf("배포 아닌 착지 뒤 명령이 밀림을 가림: %+v", it)
	}
}
