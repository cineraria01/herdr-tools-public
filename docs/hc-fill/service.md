# hc-fill: 늘 돌리기

- 늘 돌리기(기본 꺼짐): `cp launchd/com.herdr-tools.hc-fill.plist ~/Library/LaunchAgents/ && launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.herdr-tools.hc-fill.plist`. 끄기 `launchctl bootout gui/$(id -u)/com.herdr-tools.hc-fill`(그다음 Herdr 터미널 `hc-fill-watch`도 `hc kill hc-fill-watch`).
  기록 `~/.local/state/hc-fill/watch.log`. launchd는 지킴이만 띄운다(아래 「launchd 작업」).
