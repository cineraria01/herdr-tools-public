#!/bin/bash
# herdr-tools-publish 시험: 임시 비공개 저장소(와 bare origin), 임시 공개 bare 저장소, 가짜 설정 폴더. 네트워크·실제 저장소는 쓰지 않는다.
# 실행: bash tests/herdr-tools-publish.test.sh   (다른 사본은 HERDR_TOOLS_PUBLISH=<경로>, 곁에 herdr-tools-config가 있어야 한다)
set -u
here=$(cd "$(dirname "$0")" && pwd)
PUB=${HERDR_TOOLS_PUBLISH:-$here/../bin/herdr-tools-publish}
# 판(go·bash): 기본 Go 판으로 못박는다. tests/ops-go-bash.test.sh가 HERDR_IMPL=bash로 같은 것을
export HERDR_TOOLS_PUBLISH_IMPL=${HERDR_TOOLS_PUBLISH_IMPL:-${HERDR_IMPL:-go}}
T=$(mktemp -d "${TMPDIR:-/tmp}/htp-test.XXXXXX") && T=$(cd "$T" && pwd -P) || exit 1
trap 'rm -rf "$T"' EXIT
export HOME=$T/home HERDR_TOOLS_REPO=$T/priv HERDR_TOOLS_CONFIG=$T/conf GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=priv GIT_AUTHOR_EMAIL=priv-author@private.example GIT_COMMITTER_NAME=priv GIT_COMMITTER_EMAIL=priv-author@private.example
mkdir -p "$HOME" "$T/conf" "$T/tmp"
export TMPDIR=$T/tmp   # 이 시험의 임시 폴더만 본다(같은 기계의 다른 내보내기와 겹치지 않게)

n=0 fails=0
t() { n=$((n + 1)); if eval "$2"; then echo "ok $n - $1"; else fails=$((fails + 1)); echo "FAIL $n - $1"; printf '%s\n' "$out" | sed 's/^/    | /' | tail -20; fi; }
run() { out=$(env -u GIT_AUTHOR_NAME -u GIT_AUTHOR_EMAIL -u GIT_COMMITTER_NAME -u GIT_COMMITTER_EMAIL bash "$PUB" "$@" 2>&1); rc=$?; }
has() { printf '%s\n' "$out" | grep -Fq -- "$1"; }
pub() { git --git-dir="$T/public.git" "$@"; }
pmain() { pub rev-parse -q --verify refs/heads/main 2> /dev/null; }
commit() { git -C "$T/priv" add -A && git -C "$T/priv" commit -qm "$1" && git -C "$T/priv" push -q origin main; }

# 비공개 저장소: 커밋 둘(메시지에 비밀 낱말), 실행 파일·링크·하위 폴더
git init -q --bare -b main "$T/origin.git"
git init -q --bare -b main "$T/public.git"
git clone -q "$T/origin.git" "$T/priv" 2> /dev/null
mkdir -p "$T/priv/bin" "$T/priv/docs"
printf '#!/bin/sh\necho a\n' > "$T/priv/bin/a" && chmod +x "$T/priv/bin/a"
printf 'doc\n' > "$T/priv/docs/d.md"
printf 'gone\n' > "$T/priv/docs/old.md"
ln -s ../docs/d.md "$T/priv/bin/link"
commit "private-msg-one"
printf 'doc2\n' > "$T/priv/docs/d.md"
commit "private-msg-two"
printf 'secret-word\n' > "$T/conf/private-patterns.txt"
cat > "$T/conf/config" << EOF
publish.repo    $T/public.git
publish.dir     ~/pubclone
publish.author  Pub Bot <bot@public.example>
EOF

# 1. 설정 없음·검사 도구 없음
mv "$T/conf/config" "$T/conf/config.keep"
run
t "publish.repo 없음: 종료 2, 공개 쪽 그대로" '[ $rc = 2 ] && has "publish.repo" && [ -z "$(pmain)" ]'
mv "$T/conf/config.keep" "$T/conf/config"
mkdir -p "$T/lone" && cp "$PUB" "$T/lone/herdr-tools-publish"
out=$(bash "$T/lone/herdr-tools-publish" 2>&1); rc=$?
# 셸 판은 곁에 검사 도구가 있어야 한다. Go 판은 검사가 바이너리 안에 있고, 홀로 둔 사본은 공통 실행기가 없어 Go 판을 못 쓴다(못박았으면 종료 1)
t "곁에 검사 도구(herdr-tools-config)·공통 실행기 없음: 종료 1, push 없음" '[ $rc = 1 ] && { has "herdr-tools-config" || has "공통 실행기 없음"; } && [ -z "$(pmain)" ]'
mv "$T/conf/private-patterns.txt" "$T/pp.keep"
run
t "개인 패턴 파일 없음: 검사 못 함 → 종료 1, push 없음" '[ $rc = 1 ] && has "검사 못 함" && [ -z "$(pmain)" ]'
printf '# 주석만\n' > "$T/conf/private-patterns.txt"
run
t "패턴 줄 없음: 종료 1, push 없음" '[ $rc = 1 ] && has "검사 못 함" && [ -z "$(pmain)" ]'
mv "$T/pp.keep" "$T/conf/private-patterns.txt"

# 2. --dry-run: 바뀔 파일 목록·검사 결과만, push 없음
run --dry-run
t "--dry-run: 파일 목록과 검사 통과, push 없음" '[ $rc = 0 ] && has "A	bin/a" && has "A	docs/old.md" && has "검사 통과" && [ -z "$(pmain)" ]'

# 3. 첫 내보냄: 커밋 하나, 기록·비공개 메시지·이메일 없음, 트리는 비공개 main과 같음
run
psha=$(git -C "$T/priv" rev-parse --short origin/main)
t "첫 내보냄: 종료 0" '[ $rc = 0 ] && has "내보냄"'
t "공개 쪽 커밋 하나" '[ "$(pub rev-list --count main)" = 1 ]'
t "메시지는 「herdr-tools <sha> 내보냄」 한 줄" '[ "$(pub log -1 --format=%B main)" = "herdr-tools $psha 내보냄" ]'
t "작성자는 publish.author" '[ "$(pub log -1 --format="%an <%ae>|%cn <%ce>" main)" = "Pub Bot <bot@public.example>|Pub Bot <bot@public.example>" ]'
t "비공개 메시지·이메일 없음 (git log -p)" '! pub log -p --format=fuller main | grep -Eq "private-msg|priv-author"'
t "트리가 비공개 origin/main과 같음 (실행 비트·링크 포함)" '[ "$(pub rev-parse "main^{tree}")" = "$(git -C "$T/priv" rev-parse "origin/main^{tree}")" ]'
t "publish.dir의 ~는 집" '[ -d "$HOME/pubclone" ]'

# 4. 바뀐 것 없음 → 커밋 없음
h=$(pmain)
run
t "바뀐 것 없음: 「같음」, 커밋 없음" '[ $rc = 0 ] && has "같음" && [ "$(pmain)" = "$h" ]'

# 5. 파일 지움 반영, 부모는 앞 공개 커밋 (기록 없이 이어짐)
git -C "$T/priv" rm -q docs/old.md && git -C "$T/priv" commit -qm "private-msg-three" && git -C "$T/priv" push -q origin main
run
t "지움 반영: 공개 트리에 old.md 없음, 커밋 둘" '[ $rc = 0 ] && ! pub cat-file -e main:docs/old.md 2> /dev/null && [ "$(pub rev-list --count main)" = 2 ] && [ "$(pub rev-parse main^)" = "$h" ]'

# 6. 개인 패턴 걸림 → push 없음, 파일:줄, 종료 1
h=$(pmain)
printf 'x\nhas secret-word here\n' > "$T/priv/docs/leak.md" && commit "leak"
run
t "개인 패턴: 종료 1, 파일:줄, push 없음" '[ $rc = 1 ] && has "docs/leak.md:2:" && [ "$(pmain)" = "$h" ]'
run --dry-run
t "개인 패턴 --dry-run: 종료 1과 걸린 줄" '[ $rc = 1 ] && has "docs/leak.md:2:"'
git -C "$T/priv" rm -q docs/leak.md && commit "unleak"

# 7. 비밀 꼴 걸림 → 종료 1 (값은 찍지 않는다). 시험 파일 자체에 비밀 꼴이 없게 조각을 이어 만든다
gh=ghp_ key=-----BEGIN a16=Zk3mQ9vT2pLx8RwN b16=4bYc7HdJ1sFgU6eA h20=9f86d081884c7d659a2f
for s in "token: ${gh}$a16$b16$a16" "$key OPENSSH PRIVATE KEY-----" "AK""IAABCDEFGHIJKLMNOP" \
  "x = $a16$b16" "hex $h20$h20" "slack xo""xb-1234567890-abcdefghij" "sk = s""k-$a16$b16"; do
  printf 'ok\n%s\n' "$s" > "$T/priv/docs/sec.md" && commit "sec"
  run
  t "비밀 꼴 (${s:0:6}…): 종료 1, 파일:줄, 값은 안 찍음, push 없음" '[ $rc = 1 ] && has "docs/sec.md:2:" && ! has "$s" && [ "$(pmain)" = "$h" ]'
done
printf 'customtok-ZZZ\n' > "$T/priv/docs/sec.md" && commit "sec"
run
t "설정에 없는 꼴은 통과" '[ $rc = 0 ]'
h=$(pmain)
printf 'publish.secrets customtok-[A-Z]{3}\n' >> "$T/conf/config"
printf 'customtok-YYY\n' > "$T/priv/docs/sec.md" && commit "sec"
run
t "설정 키 publish.secrets로 더한 꼴: 종료 1" '[ $rc = 1 ] && has "docs/sec.md:1:" && [ "$(pmain)" = "$h" ]'
git -C "$T/priv" rm -q docs/sec.md && commit "unsec"
t "흔한 글자(경로·알파벳 목록)는 비밀로 안 봄" 'printf "%s\n" "HOME/Library/Logs/herdr-core-restart" "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789/_" > "$T/priv/docs/ok.md" && commit ok && run && [ $rc = 0 ]'

# 7-2. 감사 반려(2026-11-16) 재현: UTF-8 아닌 바이트·NUL 파일, 파일 이름, 링크 대상, export-subst, 작성자
h=$(pmain)
printf 'caf\351 secret-word\n' > "$T/priv/docs/l1.txt" && commit l1
run
t "Latin-1 파일의 개인 값: 종료 1, push 없음" '[ $rc = 1 ] && has "docs/l1.txt:1:" && [ "$(pmain)" = "$h" ]'
printf 'caf\351 %s\n' "AK""IAABCDEFGHIJKLMNOP" > "$T/priv/docs/l1.txt" && commit l1
run
t "Latin-1 파일의 비밀 꼴: 종료 1" '[ $rc = 1 ] && has "docs/l1.txt:1:" && [ "$(pmain)" = "$h" ]'
git -C "$T/priv" rm -q docs/l1.txt
printf 'a\000b\nsecret-word\n' > "$T/priv/docs/b.bin" && commit nul
run
t "NUL 파일의 개인 값: 종료 1" '[ $rc = 1 ] && has "docs/b.bin:2:" && [ "$(pmain)" = "$h" ]'
git -C "$T/priv" rm -q docs/b.bin
printf 'x\n' > "$T/priv/docs/notes-secret-word.md" && commit name
run
t "파일 이름의 개인 값: 종료 1" '[ $rc = 1 ] && has "docs/notes-secret-word.md:0:" && [ "$(pmain)" = "$h" ]'
git -C "$T/priv" rm -q docs/notes-secret-word.md
printf 'x\n' > "$T/priv/docs/$a16$b16.md" && commit name
run
t "파일 이름의 비밀 꼴: 종료 1, 이름은 안 찍음" '[ $rc = 1 ] && ! has "$a16$b16" && [ "$(pmain)" = "$h" ]'
git -C "$T/priv" rm -q "docs/$a16$b16.md"
ln -s "../$a16$b16" "$T/priv/docs/lnk" && commit lnk
run
t "링크 대상 글자의 비밀 꼴: 종료 1" '[ $rc = 1 ] && [ "$(pmain)" = "$h" ]'
git -C "$T/priv" rm -q docs/lnk && commit unlnk
printf 'ver.txt export-subst\n' > "$T/priv/.gitattributes"
printf 'v $Format:%%ae %%s$\n' > "$T/priv/ver.txt" && commit "private-subject"
run
t "export-subst: 비공개 메일·제목이 들어가지 않고 블롭 그대로" '[ $rc = 0 ] && [ "$(pub show main:ver.txt)" = "v \$Format:%ae %s\$" ] && ! pub log -p main | grep -Eq "private-subject|priv-author" && [ "$(pub rev-parse "main^{tree}")" = "$(git -C "$T/priv" rev-parse "origin/main^{tree}")" ]'
h=$(pmain)
printf 'more\n' > "$T/priv/docs/more.md" && commit more
out=$(GIT_AUTHOR_EMAIL=secret-word@x GIT_COMMITTER_EMAIL=secret-word@x bash "$PUB" 2>&1); rc=$?
t "환경의 GIT_AUTHOR_*·GIT_COMMITTER_*는 안 씀(publish.author만)" '[ $rc = 0 ] && [ "$(pub log -1 --format="%ae %ce" main)" = "bot@public.example bot@public.example" ]'
printf 'more2\n' > "$T/priv/docs/more.md" && commit more
h=$(pmain)
sed -i '' '/^publish.author/d' "$T/conf/config"
printf '[user]\n\tname = me\n\temail = me@secret-word.example\n' > "$HOME/.gitconfig"
run
t "publish.author 없음 + 전역 메일이 개인 패턴에 걸림: 종료 1, push 없음" '[ $rc = 1 ] && has "작성자" && [ "$(pmain)" = "$h" ]'
printf '[user]\n\tname = me\n\temail = me@ok.example\n' > "$HOME/.gitconfig"
run
t "publish.author 없음 + 깨끗한 전역 메일: 내보냄" '[ $rc = 0 ] && [ "$(pub log -1 --format=%ae main)" = me@ok.example ]'
printf 'publish.author  Pub Bot <bot@public.example>\n' >> "$T/conf/config"
sed -i '' "s|^publish.dir .*|publish.dir $T/fresh-dir|" "$T/conf/config"
run --dry-run
t "--dry-run: publish.dir을 만들지 않는다" '[ $rc = 0 ] && [ ! -e "$T/fresh-dir" ]'

# 7-3. 재감사(2026-11-16) 재현: 대소문자·정규화만 다른 경로가 macOS 디스크에서 겹쳐 한쪽이 검사 없이 나가던 것.
# 작업 트리로는 못 만드는 트리라 index에 직접 넣는다(ix <모드> <경로> <내용>)
G() { git -C "$T/priv" -c core.ignorecase=false -c core.precomposeunicode=false "$@"; }
ix() { G update-index --add --cacheinfo "$1,$(printf '%b' "$3" | G hash-object -w --stdin),$2"; }
icommit() { G commit -qm "$1" && G push -q origin main; }
iclean() { G rm -q --cached -- "$@" > /dev/null && icommit clean; }
h=$(pmain)
ix 100644 Notes.txt 'secret-word\n'; ix 100644 notes.txt 'ok\n'; icommit case
run
t "대소문자만 다른 두 파일: 종료 1, push 없음, 안내는 한 줄" '[ $rc = 1 ] && printf "%s\n" "$out" | grep -Eqx "경로가 겹친다.*: [0-9]+번째·[0-9]+번째 파일" && ! printf "%s\n" "$out" | grep -qx "경로가 겹친다.*: 파일" && [ "$(pmain)" = "$h" ]'
iclean Notes.txt notes.txt
printf 'x\n' | G hash-object -w --stdin > /dev/null
ix 120000 A 'x.txt'; ix 100644 a 'secret-word\n'; icommit linkcase
run
t "링크 A + 파일 a: 종료 1, push 없음" '[ $rc = 1 ] && [ "$(pmain)" = "$h" ]'
iclean A a
printf 'keep\n' > "$T/outside"
ix 120000 D "$T"; ix 100644 d/outside 'secret-word\n'; icommit linkdir
run
t "링크 폴더 D + d/아래 파일: 종료 1, push 없음, 바깥 파일 그대로" '[ $rc = 1 ] && [ "$(pmain)" = "$h" ] && [ "$(cat "$T/outside")" = keep ]'
iclean D d/outside
ix 100644 "$(printf 'cafe\314\201.md')" 'secret-word\n'; ix 100644 "$(printf 'caf\303\251.md')" 'ok\n'; icommit nfd
run
t "NFC·NFD만 다른 두 파일: 종료 1, push 없음" '[ $rc = 1 ] && has "겹" && [ "$(pmain)" = "$h" ]'
iclean "$(printf 'cafe\314\201.md')" "$(printf 'caf\303\251.md')"
ix 100644 Docs/z.md 'ok\n'; icommit dircase
run
t "폴더 이름 대소문자만 다름(Docs·docs): 종료 1" '[ $rc = 1 ] && has "겹" && [ "$(pmain)" = "$h" ]'
iclean Docs/z.md
run
t "겹침을 뺀 뒤: 내보냄" '[ $rc = 0 ]'
h=$(pmain)
git -C "$T/priv" checkout -q -- . 2> /dev/null
printf 'secret-word\n' > "$T/priv/docs/s1.md" && cp "$T/priv/docs/s1.md" "$T/priv/docs/s2.md" && commit same
run
t "같은 블롭 두 경로: 둘 다 찍음" '[ $rc = 1 ] && has "docs/s1.md:1:" && has "docs/s2.md:1:" && [ "$(pmain)" = "$h" ]'
git -C "$T/priv" rm -q docs/s1.md docs/s2.md && printf 'p\n' > "$T/priv/docs/p.md" && commit pushurl
git init -q --bare "$T/other-push.git"
git --git-dir="$T/fresh-dir" config remote.origin.pushurl "$T/other-push.git"
run
t "공개 clone의 pushurl을 따르지 않고 publish.repo로 push" '[ $rc = 0 ] && [ "$(pmain)" != "$h" ] && ! git --git-dir="$T/other-push.git" rev-parse -q --verify refs/heads/main > /dev/null'

# 7-4. 3차 감사(2026-11-16) 재현: UTF-16·압축 블롭 속 값은 바이트 grep을 통과했다 → 글이 아닌 블롭(NUL·UTF-8 아님)은 막는다
h=$(pmain)
printf 'note secret-word\n' | iconv -f UTF-8 -t UTF-16LE > "$T/priv/docs/w.txt" && commit u16
run
t "UTF-16 개인 값: 종료 1, push 없음, 경로 찍음" '[ $rc = 1 ] && has "docs/w.txt:0: 글이 아닌 블롭" && [ "$(pmain)" = "$h" ]'
git -C "$T/priv" rm -q docs/w.txt
printf 'note secret-word\n' | gzip -c > "$T/priv/docs/w.gz" && commit gz
run
t "gzip 개인 값: 종료 1, push 없음" '[ $rc = 1 ] && has "docs/w.gz:0: 글이 아닌 블롭" && [ "$(pmain)" = "$h" ]'
git -C "$T/priv" rm -q docs/w.gz
printf 'tok %s\n' "${gh}$a16$b16$a16" | iconv -f UTF-8 -t UTF-16LE > "$T/priv/docs/t.txt" && commit u16tok
run
t "UTF-16 비밀 꼴: 종료 1, push 없음" '[ $rc = 1 ] && has "docs/t.txt:0: 글이 아닌 블롭" && [ "$(pmain)" = "$h" ]'
git -C "$T/priv" rm -q docs/t.txt && commit unbin
run
t "글이 아닌 블롭을 뺀 뒤: 내보냄" '[ $rc = 0 ]'
h=$(pmain)
# replace ref: 검사(cat-file)와 옮기기(pack-objects)가 다른 블롭을 보면 안 된다
printf 'secret-word\n' > "$T/priv/docs/rep.md" && commit rep
bad_blob=$(git -C "$T/priv" rev-parse HEAD:docs/rep.md)
git -C "$T/priv" replace "$bad_blob" "$(printf 'ok\n' | git -C "$T/priv" hash-object -w --stdin)"
run
t "replace ref가 있어도 진짜 블롭을 검사: 종료 1, push 없음" '[ $rc = 1 ] && [ "$(pmain)" = "$h" ]'
git -C "$T/priv" replace -d "$bad_blob" > /dev/null
# --dry-run은 막힌 블롭을 실제 publish.dir에 쓰지 않는다
run --dry-run
t "--dry-run: 실제 publish.dir에 아무것도 안 씀" '[ $rc = 1 ] && ! git --git-dir="$T/fresh-dir" cat-file -e "$bad_blob" 2> /dev/null'
git -C "$T/priv" rm -q docs/rep.md && commit unrep

# 7-5. 4차 감사(2026-11-16) 재현: 빈 하위 트리의 이름은 ls-tree -r에 안 나와 검사를 비켜 공개됐다 → 트리 이름도 검사, 빈 트리는 막음
h=$(pmain)
withtree() {   # withtree <이름>: 지금 HEAD 트리에 빈 하위 트리 <이름>을 더한 커밋을 push
  local t
  t=$( { git -C "$T/priv" ls-tree HEAD; printf '040000 tree %s\t%s\n' "$(git -C "$T/priv" hash-object -t tree /dev/null)" "$1"; } | git -C "$T/priv" mktree)
  git -C "$T/priv" update-ref HEAD "$(git -C "$T/priv" commit-tree "$t" -p HEAD -m emptytree)" && git -C "$T/priv" push -q origin main
}
untree() { git -C "$T/priv" update-ref HEAD "$(git -C "$T/priv" commit-tree "$(git -C "$T/priv" rev-parse HEAD~1^{tree})" -p HEAD -m untree)" && git -C "$T/priv" push -q origin main; }
withtree secret-word-dir
run
t "이름이 개인 값인 빈 하위 트리: 종료 1, push 없음" '[ $rc = 1 ] && [ "$(pmain)" = "$h" ]'
untree; withtree "x$a16$b16"
run
t "이름이 비밀 꼴인 빈 하위 트리: 종료 1, 이름 안 찍음" '[ $rc = 1 ] && ! has "$a16$b16" && [ "$(pmain)" = "$h" ]'
untree; withtree plain-empty
run
t "깨끗한 이름이라도 빈 하위 트리는 막음" '[ $rc = 1 ] && has "빈 하위 트리" && [ "$(pmain)" = "$h" ]'
untree
run
t "빈 하위 트리를 뺀 뒤: 같음 또는 내보냄" '[ $rc = 0 ]'
h=$(pmain)
# origin/main은 refs/remotes/origin/main 전체 이름으로 (같은 이름의 로컬 가지에 속지 않음)
git -C "$T/priv" branch origin/main HEAD~3 2> /dev/null
printf 'r\n' > "$T/priv/docs/r.md" && commit remote
run
t "로컬 가지 origin/main이 있어도 원격 origin/main 트리를 내보냄" '[ $rc = 0 ] && [ "$(pub rev-parse "main^{tree}")" = "$(git -C "$T/priv" rev-parse "refs/remotes/origin/main^{tree}")" ]'
git -C "$T/priv" branch -q -D origin/main
h=$(pmain)
# 개인 패턴은 대소문자를 무시한다
printf 'Secret-Word here\n' > "$T/priv/docs/up.md" && commit up
run
t "개인 값 대소문자 달라도 막음" '[ $rc = 1 ] && has "docs/up.md:1:" && [ "$(pmain)" = "$h" ]'
git -C "$T/priv" rm -q docs/up.md && commit unup

# 8. publish.dir에 다른 저장소가 있으면 건드리지 않는다
git init -q --bare "$T/other.git"
sed -i '' "s|^publish.dir .*|publish.dir $T/other.git|" "$T/conf/config"
run
t "publish.dir의 origin이 다름: 종료 1" '[ $rc = 1 ] && has "origin"'

t "임시 파일이 남지 않는다" '[ -z "$(ls -A "$T/tmp")" ]'
echo "$((n - fails))/$n 통과"
[ $fails = 0 ]
