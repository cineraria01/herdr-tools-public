# hc-lane 함수: guard (bin/hc-lane가 읽는다. 함수는 bin/hc-lane 안 원래 정의 순서 그대로)

guard_cmd() {
  common=$(git -C "$repo" rev-parse --path-format=absolute --git-common-dir 2>/dev/null) || die "guard: git 저장소가 아니다: $repo"
  hp=$(git -C "$repo" config --get core.hooksPath)
  f=$common/hooks/pre-push
  st=none
  if [ -e "$f" ] || [ -L "$f" ]; then if grep -Fqx "$gmark" "$f" 2>/dev/null; then st=ours; else st=other; fi; fi
  if [ -z "$(git -C "$repo" remote)" ] && [ $act != remove ]; then
    echo "guard: 원격이 없는 저장소라 push가 없다. 설치할 것 없음(착지는 hc-lane land가 로컬 <기본>을 fast-forward로 올린다): $repo"
    return 0
  fi
  case $act in
    status)
      case $st in
        ours) b=$(sed -n 's/^base=//p' "$f")
              if [ -n "$hp" ]; then echo "guard: 깔려 있지만 core.hooksPath=$hp 라 git이 부르지 않는다: $f"; return 1; fi
              echo "guard: 깔림(기본 브랜치 $b): $f"; return 0 ;;
        none) echo "guard: 없음: $f"; return 1 ;;
        other) echo "guard: 다른 pre-push 훅이 있다(hc-lane 관문 아님): $f"; return 3 ;;
      esac ;;
    install)
      [ -z "$hp" ] || die "guard: core.hooksPath=$hp 라 $f 는 git이 부르지 않는다. 건드리지 않았다(그 훅 폴더에 연결할지 지휘가 정한다)"
      [ $st != other ] || die "guard: 다른 pre-push 훅이 있다: $f. 덮지 않았다(그 훅에 관문을 이을지 지휘가 정한다)"
      b=${branch:-$(origin_base "$repo")}
      plain "$b" || die "guard: 기본 브랜치 이름에 쓸 수 없는 글자가 있다: $b"
      mkdir -p "$common/hooks" || die "guard: 훅 폴더를 만들지 못했다: $common/hooks"
      tmp=$f.hc-lane.$$
      cat >"$tmp" <<EOF || die "guard: 쓰지 못했다: $tmp"
#!/bin/sh
$gmark
# hc-lane guard install이 넣었다(지우기: hc-lane guard remove --repo <경로>). refs/heads/<기본>으로 가는 push는
# 착지 줄(hc-lane land, .claude/herdr-land.sh)이 주는 HERDR_LAND=1일 때만 받는다. 다른 브랜치·참조는 통과.
base=$b
[ "\${HERDR_LAND-}" = 1 ] && exit 0
while read -r lref lsha rref rsha; do
  [ "\$rref" = "refs/heads/\$base" ] || continue
  cat >/dev/null
  echo "push 막힘: \$base 로 가는 push는 착지 줄로만 한다." >&2
  echo "새 착지 줄: hc-lane land \$base" >&2
  echo "(사람이 손으로 넣을 때만 HERDR_LAND=1 git push … 로 명시)" >&2
  exit 1
done
exit 0
EOF
      chmod +x "$tmp" && mv -f "$tmp" "$f" || { rm -f "$tmp"; die "guard: 넣지 못했다: $f"; }
      [ $st = ours ] && echo "guard: 새로 씀(기본 브랜치 $b): $f" || echo "guard: 깔았다(기본 브랜치 $b): $f" ;;
    remove)
      case $st in
        ours) rm -f "$f" || die "guard: 지우지 못했다: $f"; echo "guard: 지웠다: $f" ;;
        none) echo "guard: 지울 것 없음: $f" ;;
        other) die "guard: hc-lane 관문이 아닌 pre-push 훅이라 지우지 않았다: $f" ;;
      esac ;;
  esac
}
