package main

// alertup.go: 병합기가 낸 막힘의 풀림 짝(ht-alert-sources, 2026-11-28) — 그 상태가 끝날 때 같은 이름(merger)·같은 열쇠 + :up 중간을
// 한 번 적는다. 지휘 watch의 한 문(hc-lane gate.go opsKeyDone)이 그 막힘을 풀림으로, 이 사건을 소식으로 본다.
// 같은 까닭 쉼의 짝은 defer.go deferNote, 풀이 감사 통과는 resolve.go

import (
	"fmt"
	"hash/crc32"
	"os"
	"strings"
	"time"
)

// stopFile: 병합기 멈춤 막힘의 열쇠를 다시 돈 상주가 읽을 표식(상태 폴더, 클론마다)
func stopFile(top string) string { return gEvdir + "/merger-stop." + ckey(top) }

// stopNote: 병합기 멈춤 막힘(열쇠 hc-merger-stop:<초>)을 적고 표식에 그 열쇠를 남긴다 — 클론을 모르면(?) 표식 없이 사건만
func stopNote(top, msg string) {
	key := fmt.Sprintf("hc-merger-stop:%d", time.Now().Unix())
	if reportBlockedKey(msg, key) && top != "?" {
		if err := os.WriteFile(stopFile(top), []byte(key+"\n"), 0o666); err != nil {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 멈춤 표식을 쓰지 못했다(다시 돌 때 풀림을 못 냄): %v\n", err)
		}
	}
}

// stopUp: 앞서 멈춤을 알린 클론의 병합기가 다시 돎 — 같은 열쇠 + :up 한 번, 적으면 표식을 지운다(못 적으면 다음 바퀴에)
func stopUp(top string) {
	key := strings.TrimSpace(readS(stopFile(top)))
	if !strings.HasPrefix(key, "hc-merger-stop:") {
		return
	}
	if reportKey("중간", "병합기 다시 돎 — 앞서 알린 멈춤이 풀림 (클론 "+top+")", key+":up") {
		removeQuiet(stopFile(top))
	}
}

// startKey: 시작 단계 n번 모름 막힘의 열쇠(이 상주 실행·그 단계마다 하나 — 풀림은 + :up)
func startKey(what string) string {
	return fmt.Sprintf("hc-merger-start:%08x:%d", crc32.ChecksumIEEE([]byte(what)), os.Getpid())
}

// startDone: 시작 단계가 됨 — 모름 막힘을 냈던 단계면(n이 deferAlarm 이상) 같은 열쇠 + :up 한 번(못 적어도 경고만 — 상주 시작을 막지 않는다)
func startDone(what string, n int) {
	if n >= deferAlarm {
		reportKey("중간", fmt.Sprintf("병합기 시작 단계 %s 됨 — %d번 이어 모름이 풀림", what, n), startKey(what)+":up")
	}
}
