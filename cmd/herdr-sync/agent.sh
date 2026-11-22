S=$HOME/.local/state/herdr-sync
B=${HSB:-$S/base.tsv}
T=$(mktemp -d "${TMPDIR:-/tmp}/herdr-sync.XXXXXX") || exit 1
G=$T
MARK='<!-- User customizations -->'
CS='<!-- codex-shared:start -->'
CE='<!-- codex-shared:end -->'
tab=$(printf '\t')
trap 'rm -rf "$T" "$G"' EXIT
trap 'exit 1' HUP INT TERM
at() { p=${1%%#*}; p=${p%@}; f=$HOME/$p; }
mark() { n=$(grep -n -x -F -- "$MARK" "$f" | head -n 1 | cut -d: -f1); [ -n "$n" ]; }
# gone <경로>: 그 자리에 일반 파일이 없음이 확정인가 — 무엇이 있거나(폴더·링크), 가장 가까운 있는 위 폴더를 뒤질 수 있거나, 위가 일반 파일.
# 위 폴더를 뒤질 수 없으면(권한) 있는지 모른다 — 「없음」으로 답하면 모음이 저장소 파일을 지운다(R-01)
gone() {
  { [ -e "$1" ] || [ -L "$1" ]; } && return 0
  g=${1%/*}
  while [ -n "$g" ] && [ ! -e "$g" ] && [ ! -L "$g" ]; do g=${g%/*}; done
  [ -z "$g" ] || [ ! -d "$g" ] || [ -x "$g" ]
}
val() {
  at "$1"
  case $1 in
    *'#body') [ -f "$f" ] || { gone "$f" || return 6; return 1; }; [ -r "$f" ] || return 6; mark || return 3; tail -n +"$((n + 1))" "$f" || return 6 ;;
    *'#codex-shared') [ -f "$f" ] || { gone "$f" || return 6; return 4; }; [ -r "$f" ] || return 6; grep -q -x -F -- "$CS" "$f" && grep -q -x -F -- "$CE" "$f" || return 5
      awk -v s="$CS" -v e="$CE" '$0==e{k=0} k; $0==s{k=1}' "$f" || return 6 ;;
    *) [ -f "$f" ] || { gone "$f" || return 6; return 1; }; cat "$f" || return 6 ;;
  esac
}
sum() { cksum < "$1" | awk '{print $1"."$2}'; }
sig() {
  case $1 in *@) at "$1"; if [ -L "$f" ]; then readlink "$f"; elif [ -e "$f" ]; then echo '!'; elif gone "$f"; then echo -; else echo "읽지 못했다: $f" >&2; return 1; fi; return ;; esac
  val "$1" > "$T/v"
  case $? in 0) ;; 3) echo '!'; return ;; 4) echo '~'; return ;; 5) echo '?'; return ;; 6) echo "읽지 못했다: $f" >&2; return 1 ;; *) echo -; return ;; esac
  x=-; case $1 in *'#'*) ;; *) [ -x "$f" ] && x=x ;; esac
  echo "$(sum "$T/v").$x"
}
bsig() { if [ -f "$B" ]; then awk -F'\t' -v k="$1" '$1==k{s=$2} END{print (s=="" ? "-" : s)}' "$B"; else echo -; fi; }
setbf() {   # setbf <파일>: 그 파일의 「키 서명」 줄들로 base를 한 번에 고친다
  [ "$B" = - ] && return 0
  [ -s "$1" ] || return 0
  mkdir -p "${B%/*}" && { [ -f "$B" ] || : > "$B"; } || return 1
  awk -F'\t' -v p="$1" 'BEGIN { while ((getline l < p) > 0) { sub(/\t.*/, "", l); v[l] = 1 } } !($1 in v)' "$B" > "$B.$$" \
    && cat "$1" >> "$B.$$" && mv -f "$B.$$" "$B" || { rm -f "$B.$$"; return 1; }
}
bak() { at "$1"; [ -f "$f" ] || return 0; o=$S/bak/$2/$p; mkdir -p "$(dirname "$o")" && cp -p "$f" "$o"; }
# 나무 걷기. 링크는 따라가지 않고(L로 알린다), 이름이 .으로 시작하는 것·__pycache__·node_modules와 제외 파일의 상대 경로는 들어가지 않는다.
# 7MB(512바이트 블록 14336개, hc exec 입출력 한도 8MB 아래 묶음 상한) 넘는 파일은 cksum하지 않고 G로 알린다. cksum·find는 폴더마다 몇 번만 돈다. 걷기·cksum이 하나라도 실패하면 종료 1(B08)
# ponytail: 이름에 탭·줄바꿈이 든 파일, 제외 경로의 glob 글자([*?)는 다루지 않는다
lt() {
  ld=$1 lp=$2 lx=$3
  if [ -L "$ld" ]; then printf 'L\t%s\n' "$lp"; return 0; fi
  [ -d "$ld" ] || { gone "$ld" && return 0; echo "나무를 읽지 못했다: $lp" >&2; return 1; }
  set -- \( -name '.*' -o -name __pycache__ -o -name node_modules
  if [ -f "$lx" ]; then while IFS= read -r e; do [ -n "$e" ] && set -- "$@" -o -path "./$e"; done < "$lx"; fi
  set -- "$@" \)
  ( cd "$ld" || exit 1
    find . ! -name . "$@" -prune -o -type l -exec printf 'L\t%s\n' {} + -o -type f -size +14336 -exec printf 'G\t%s\n' {} + \
      -o -type f -perm -100 -exec printf 'X\t%s\n' {} + &&
    find . ! -name . "$@" -prune -o -type f ! -size +14336 -exec cksum {} + ) > "$T/lw" || { echo "나무를 읽지 못했다: $lp" >&2; return 1; }
  awk -v p="$lp" '
      /^[LGX]\t/ { t = substr($0, 1, 1); f = substr($0, 3) }
      !/^[LGX]\t/ { t = "F"; c = $1 "." $2; f = $0; sub(/^[^ ]+ [^ ]+ /, "", f) }
      { sub(/^\.\//, "", f); f = p "/" f
        if (t == "X") x[f] = 1; else if (t == "F") printf "F\t%s\t%s.%s\n", f, c, ((f in x) ? "x" : "-"); else printf "%s\t%s\n", t, f }' "$T/lw"
}
q() {
  for s in k r t x y; do : > "$T/$s"; done
  s=k
  for a in "$@"; do
    if [ "$a" = -- ]; then case $s in k) s=r ;; r) s=t ;; t) s=x ;; x) s=y ;; esac; continue; fi
    printf '%s\n' "$a" >> "$T/$s"
  done
  while IFS= read -r a; do s=$(sig "$a") || exit 1; printf 'U\t%s\t%s\n' "$s" "$(bsig "$a")"; done < "$T/k"
  while IFS= read -r a; do if [ -e "$HOME/$a" ] || [ -L "$HOME/$a" ]; then printf 'R\t%s\n' "$a"; fi; done < "$T/r"
  while IFS= read -r a; do if [ -e "$HOME/$a" ]; then printf 'Y\t%s\n' "$a"; fi; done < "$T/y"
  while IFS= read -r a; do
    awk -v r="$a/" 'index($0, r) == 1 { print substr($0, length(r) + 1) }' "$T/x" > "$T/ex"
    lt "$HOME/$a" "$a" "$T/ex" || exit 1
  done < "$T/t"
  if [ -s "$T/t" ] && [ -f "$B" ]; then
    awk -F'\t' -v t="$T/t" 'BEGIN { while ((getline l < t) > 0) r[l "/"] = 1 } { for (p in r) if (index($1, p) == 1) { print "B\t" $0; next } }' "$B"
  fi
  printf 'H\t%s\nO\t%s\n' "$HOME" "$(uname -s)"
}
W() {   # 받은 원본을 같은 파일 시스템의 임시 폴더에 풀고, cksum이 맞는 것만 백업한 뒤 바꿔 넣는다. 키마다 「W 키」 또는 「E 키 까닭」
  for o in "$S"/w.*; do   # hc --timeout이 KILL로 끝낸 지난 W가 남긴 임시 폴더 (그 pid가 이제 없으면)
    [ -d "$o" ] && ! kill -0 "${o##*.}" 2> /dev/null && rm -rf "$o"
  done
  G=$S/w.$$
  rm -rf "$G" && mkdir -p "$G" || return 1
  : > "$T/m"
  l=
  while IFS= read -r l; do [ "$l" = . ] && break; printf '%s\n' "$l" >> "$T/m"; done
  [ "$l" = . ] || { echo "받은 내용이 깨졌다: 목록" >&2; return 1; }
  tar -xf - -C "$G" 2> "$T/te" || sed 's/^/tar: /' "$T/te" >&2   # 일부만 풀렸어도 아래 cksum이 가른다
  (cd "$G" && find . -type f -exec cksum {} +) | awk '{ c = $1 "." $2; sub(/^[^ ]+ [^ ]+ \.\//, ""); print $0 "\t" c }' > "$T/s"
  awk -F'\t' -v s="$T/s" 'BEGIN { while ((getline l < s) > 0) { n = split(l, a, "\t"); h[a[1]] = a[n] } }
    { w = $3; sub(/\.[^.]*$/, "", w); print ((($4 in h) && h[$4] == w) ? "ok" : "bad") "\t" $0 }' "$T/m" > "$T/v2"
  : > "$T/p"
  while IFS=$tab read -r ok k x want src; do
    f=$HOME/$k
    if [ "$ok" != ok ]; then printf 'E\t%s\t받은 내용이 깨졌다\n' "$k"; continue; fi
    if [ -d "$f" ] && [ ! -L "$f" ]; then printf 'E\t%s\t그 자리에 폴더가 있다\n' "$k"; continue; fi
    g=$G/$src
    if [ "$x" = x ]; then [ -x "$g" ] || chmod +x "$g"; elif [ -x "$g" ]; then chmod a-x "$g"; fi
    [ -d "${f%/*}" ] || mkdir -p "${f%/*}" || { printf 'E\t%s\t폴더를 만들지 못했다\n' "$k"; continue; }
    if [ -f "$f" ]; then
      o=$S/bak/$1/$k
      { [ -d "${o%/*}" ] || mkdir -p "${o%/*}"; } && cp -p "$f" "$o" || { printf 'E\t%s\t백업하지 못했다\n' "$k"; continue; }
    fi
    mv -f "$g" "$f" || { printf 'E\t%s\t바꿔 넣지 못했다\n' "$k"; continue; }
    printf '%s\t%s\n' "$k" "$want" >> "$T/p"
    printf 'W\t%s\n' "$k"
  done < "$T/v2"
  setbf "$T/p"
}
T() {   # 있는 일반 파일만. bsdtar는 확장 속성을 빼고, 이름을 바이트 그대로 둔다(기본 꼴은 macOS에서 한글 이름을 NFD로 바꿔 푼다)
  while IFS= read -r k; do if [ -f "$HOME/$k" ]; then printf '%s\n' "$k"; fi; done > "$T/l"
  set --
  tar --version 2> /dev/null | grep -q bsdtar && set -- --no-xattrs --no-acls --no-mac-metadata --format=pax --options=hdrcharset=BINARY
  cd "$HOME" && COPYFILE_DISABLE=1 tar "$@" -cf - -T "$T/l"
}
w() {
  k=$1 x=$2 want=$3
  cat > "$T/c"
  [ "$(sum "$T/c")" = "${want%.*}" ] || { echo "받은 내용이 깨졌다: $k" >&2; return 1; }
  at "$k"
  t=$f.hsync.$$
  case $k in
    *'#body') [ -f "$f" ] && mark || return 3
      cp -p "$f" "$t" && { head -n "$n" "$f"; cat "$T/c"; } > "$t" ;;
    *'#codex-shared') [ -f "$f" ] || return 4
      if grep -q -x -F -- "$CS" "$f" && grep -q -x -F -- "$CE" "$f"; then
        cp -p "$f" "$t" && awk -v body="$T/c" -v s="$CS" -v e="$CE" '$0==s{print; while ((getline l < body) > 0) print l; skip=1; next} $0==e{skip=0} !skip' "$f" > "$t"
      else
        cp -p "$f" "$t" && awk '/^<!-- codebase-memory-mcp:start -->$/{k=1} k{print} /^<!-- codebase-memory-mcp:end -->$/{k=0}' "$f" > "$t" \
          && { [ ! -s "$t" ] || echo >> "$t"; } && { echo "$CS"; cat "$T/c"; echo "$CE"; } >> "$t"
      fi ;;
    *) mkdir -p "$(dirname "$f")" && cat "$T/c" > "$t" && if [ "$x" = x ]; then chmod +x "$t"; else chmod a-x "$t"; fi ;;
  esac || { rm -f "$t"; return 1; }
  bak "$k" "$4" || { rm -f "$t"; return 1; }
  if [ -L "$f" ]; then cat "$t" > "$f" && rm -f "$t"; else mv -f "$t" "$f"; fi && printf '%s\t%s\n' "$k" "$want" > "$T/p" && setbf "$T/p"
}
d() { at "$1"; { [ -e "$f" ] || [ -L "$f" ]; } || return 0; o=$S/bak/$2/$p; mkdir -p "$(dirname "$o")" && mv "$f" "$o"; }
l() { at "$1"; { [ -e "$f" ] || [ -L "$f" ]; } && return 1; [ -e "$HOME/$2" ] || return 3; mkdir -p "$(dirname "$f")" && ln -s "$HOME/$2" "$f"; }
op=$1; shift
case $op in
  q) q "$@" ;; t) lt "$@" ;; r) val "$1" ;; w) w "$@" ;; W) W "$@" ;; T) T ;; d) d "$@" ;; l) l "$@" ;; B) cat > "$T/p" && setbf "$T/p" ;; *) exit 2 ;;
esac
