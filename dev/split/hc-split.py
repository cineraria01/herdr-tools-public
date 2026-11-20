#!/usr/bin/env python3
"""hc-split: bin/hc-lane·bin/hc-fill의 최상위 함수를 기능별 lib/<도구>/<모듈>.sh로 기계적으로 옮기고(split), 대조한다(check).

  python3 -I dev/split/hc-split.py split          # 작업 트리의 bin/<도구>를 나눈다(계획 dev/split/plan.tsv)
  python3 -I dev/split/hc-split.py check <기준 rev> # 기준 rev의 원본과 지금 작업 트리를 대조

나누기 규칙: 최상위 함수(첫 칸의 `이름()`)와 그 바로 위에 붙은 주석 줄(빈 줄 없이 붙은 첫 칸 `#` 줄)이 한 덩이다.
덩이 끝은 bash 파서로 정한다: 시작 줄, 그다음 첫 칸 `}` 줄들 가운데 처음으로 `bash -n`이 경고 없이 받는 곳.
덩이마다 다음을 bash로 증명한다(아니면 멈춤): 덩이 앞 원본 전체가 경고 없이 파싱된다(=덩이가 최상위, if·case·heredoc 밖),
덩이 하나만 source하면 정확히 그 이름의 함수 하나만 생긴다.
본체에는 덩이를 지운 나머지를 원래 순서 그대로 두고, 첫 덩이 자리에 lib를 읽는 몇 줄을 넣는다(지운 자리에 빈 줄이
겹치면 하나만 남긴다). lib 파일은 머리 주석 한 줄 + 그 모듈 덩이를 원래 순서대로.

check(대조): 기준 원본에서 덩이를 다시 뽑아(위 증명 포함)
  1) 모든 함수가 lib 가운데 정확히 한 곳에 정확히 한 번, 본문 글자 그대로(덩이 글자 = lib 안 덩이 글자)
  2) lib 안 덩이 순서 = 원본 순서, lib에는 덩이 말고 머리 주석·빈 줄뿐
  3) 본체 = 원본에서 덩이를 지운 것(빈 줄 겹침만 다름) + 로더 줄
  4) bash -n 전부 통과, lib를 모두 source한 깨끗한 bash의 함수 목록·declare -f = 원본 덩이들의 것
  5) 원본에서 어떤 함수 이름이 그 정의보다 앞 줄에 명령 낱말로 나오면 목록으로 알린다(정의가 앞당겨져 뜻이 바뀌는지 사람이 본다)
"""
import os, re, subprocess, sys, tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))
BASH = "/bin/bash"
FN = re.compile(r"^([A-Za-z_][A-Za-z0-9_-]*)\(\)")


def die(msg):
    sys.exit("hc-split: " + msg)


def parses(text):
    r = subprocess.run([BASH, "-n"], input=text, capture_output=True, text=True)
    return r.returncode == 0 and not r.stderr.strip()


def plan():
    out, cur = [], None
    for ln in open(os.path.join(HERE, "plan.tsv"), encoding="utf-8"):
        f = ln.split()
        if not f or f[0].startswith("#"):
            continue
        if f[0] == "=":
            cur = {"script": f[1], "lib": f[2], "pfx": f[3], "opens": []}
            out.append(cur)
        else:
            cur["opens"].append((f[1], f[0]))
    return out


def blocks(lines, what):
    """[(시작 줄(주석 포함), 함수 줄, 끝 줄(포함), 이름)] — 0부터 센 줄 번호. bash로 증명."""
    out, i = [], 0
    while i < len(lines):
        m = FN.match(lines[i])
        if not m:
            i += 1
            continue
        name, end = m.group(1), None
        for e in [i] + [k for k in range(i + 1, len(lines)) if lines[k].startswith("}")]:
            if parses("".join(lines[i:e + 1])):
                end = e
                break
        if end is None:
            die("%s:%d %s 끝을 못 찾았다" % (what, i + 1, name))
        s = i
        while s > 0 and lines[s - 1].startswith("#") and not lines[s - 1].startswith("#!") and \
                (not out or s - 1 > out[-1][2]):
            s -= 1
        out.append((s, i, end, name))
        i = end + 1
    for s, i, e, name in out:
        if not parses("".join(lines[:s])):
            die("%s:%d %s 앞이 최상위로 끝나지 않는다(if·case·heredoc 안?)" % (what, i + 1, name))
        got = defined("".join(lines[s:e + 1]))
        if got != [name]:
            die("%s:%d 덩이가 함수 %s 하나가 아니다: %s" % (what, i + 1, name, got))
    names = [b[3] for b in out]
    dup = sorted({n for n in names if names.count(n) > 1})
    if dup:
        die("%s: 같은 이름 함수가 두 번: %s" % (what, dup))
    return out


def defined(text, want_body=False):
    with tempfile.NamedTemporaryFile("w", suffix=".sh", delete=False, encoding="utf-8") as t:
        t.write(text)
    try:
        r = subprocess.run(["env", "-i", "PATH=/usr/bin:/bin", BASH, "--norc", "--noprofile", "-c",
                            '. "$1" >/dev/null 2>&1 </dev/null || exit 9; ' + ("declare -f" if want_body else "compgen -A function"),
                            "x", t.name], capture_output=True, text=True)
    finally:
        os.unlink(t.name)
    if r.returncode:
        die("source 실패(%d): %s" % (r.returncode, r.stderr.strip()))
    return r.stdout if want_body else r.stdout.split()


def assign(p, bl, what):
    opens, mod, mods = dict(p["opens"]), None, []
    for b in bl:
        mod = opens.get(b[3], mod)
        if mod is None:
            die("%s: 첫 함수 %s의 모듈이 계획에 없다" % (what, b[3]))
        mods.append(mod)
    missing = set(opens) - {b[3] for b in bl}
    if missing:
        die("%s: 계획의 여는 함수가 원본에 없다: %s" % (what, sorted(missing)))
    order = []
    for m in mods:
        if m not in order:
            order.append(m)
    return mods, order


def loader(p, order):
    tool, v = os.path.basename(p["script"]), p["pfx"] + "_"
    return [
        "# 함수는 기능별 파일 %s/*.sh에 있다(2026-11-20 herdr-tools-split — 여러 레인이 한 파일을 고쳐 부딪혀서).\n" % p["lib"],
        "# 깔린 곳은 이 파일 옆 ../lib/%s(~/.local/lib/%s). 링크로 불려도 링크가 가리키는 실제 파일 옆을 본다\n" % (tool, tool),
        "%ss=$0\n" % v,
        'while [ -L "$%ss" ]; do %sl=$(readlink "$%ss"); case $%sl in /*) %ss=$%sl ;; *) %ss=$(dirname "$%ss")/$%sl ;; esac; done\n'
        % (v, v, v, v, v, v, v, v, v),
        '%sd=$(dirname "$%ss")/../lib/%s\n' % (v, v, tool),
        "for %sf in %s; do\n" % (v, " ".join(order)),
        '  . "$%sd/$%sf.sh" || { printf \'%s: 함수 파일을 못 읽었다: %%s (herdr-sync가 lib도 까는지)\\n\' "$%sd/$%sf.sh" >&2; exit 1; }\n'
        % (v, v, tool, v, v),
        "done\n",
    ]


def generate(p, src):
    """(본체 줄들, {모듈: 파일 글자}, 덩이들, 모듈들)"""
    lines = src.splitlines(keepends=True)
    if not lines[-1].endswith("\n"):
        die(p["script"] + ": 끝 줄바꿈 없음")
    bl = blocks(lines, p["script"])
    mods, order = assign(p, bl, p["script"])
    gone = {}
    for b in bl:
        for k in range(b[0], b[2] + 1):
            gone[k] = b
    main, put = [], False
    k = 0
    while k < len(lines):
        if k in gone:
            if not put:
                main += loader(p, order)
                put = True
            k = gone[k][2] + 1
            # 지운 자리에 빈 줄이 겹치면 하나만(덩이는 최상위라 뒤 빈 줄도 최상위 — heredoc 안이 아니다)
            while k < len(lines) and k not in gone and not lines[k].strip() and main and not main[-1].strip():
                k += 1
            continue
        main.append(lines[k])
        k += 1
    libs = {}
    tool = os.path.basename(p["script"])
    for m in order:
        body = ["# %s 함수: %s (%s가 읽는다. 함수는 %s 안 원래 정의 순서 그대로)\n" % (tool, m, p["script"], p["script"])]
        prev = None
        for b, bm in zip(bl, mods):
            if bm != m:
                continue
            # 원본에서 바로 붙어 있던 덩이는 붙여, 아니면 빈 줄 하나를 두고
            if prev is None or not (prev[2] + 1 == b[0]):
                body.append("\n")
            body += lines[b[0]:b[2] + 1]
            prev = b
        libs[m] = "".join(body)
    return main, libs, bl, mods, lines


def split():
    for p in plan():
        path = os.path.join(ROOT, p["script"])
        main, libs, bl, mods, _ = generate(p, open(path, encoding="utf-8").read())
        d = os.path.join(ROOT, p["lib"])
        os.makedirs(d, exist_ok=True)
        for m, t in libs.items():
            open(os.path.join(d, m + ".sh"), "w", encoding="utf-8").write(t)
        open(path, "w", encoding="utf-8").write("".join(main))
        print("%s: 함수 %d개 → %s/{%s}.sh, 본체 %d줄" % (p["script"], len(bl), p["lib"], ",".join(libs), len(main)))


def check(rev):
    bad = 0
    for p in plan():
        src = subprocess.run(["git", "-C", ROOT, "show", "%s:%s" % (rev, p["script"])], capture_output=True,
                             text=True, check=True).stdout
        main, libs, bl, mods, lines = generate(p, src)
        tool = os.path.basename(p["script"])
        # 3) 본체
        cur = open(os.path.join(ROOT, p["script"]), encoding="utf-8").read()
        if cur != "".join(main):
            print("FAIL %s 본체가 원본-덩이+로더와 다르다" % p["script"]); bad = 1
        if not parses(cur):
            print("FAIL %s bash -n" % p["script"]); bad = 1
        # lib 폴더에 계획 밖 파일이 없어야
        d = os.path.join(ROOT, p["lib"])
        extra = sorted(set(os.listdir(d)) - {m + ".sh" for m in libs})
        if extra:
            print("FAIL %s 계획 밖 파일: %s" % (p["lib"], extra)); bad = 1
        allsrc = ""
        where = {}
        for m, want in libs.items():
            got = open(os.path.join(d, m + ".sh"), encoding="utf-8").read()
            if got != want:
                print("FAIL %s/%s.sh 기대와 다르다" % (p["lib"], m)); bad = 1
            if not parses(got):
                print("FAIL %s/%s.sh bash -n" % (p["lib"], m)); bad = 1
            # 1)·2) lib를 다시 덩이로 나눠 견준다: 덩이 말고는 머리 주석·빈 줄뿐, 순서·글자 그대로
            gl = got.splitlines(keepends=True)
            gb = blocks(gl, "%s/%s.sh" % (p["lib"], m))
            used = {k for b in gb for k in range(b[0], b[2] + 1)}
            rest = [x for k, x in enumerate(gl) if k not in used and x.strip() and k != 0]
            if rest:
                print("FAIL %s/%s.sh 덩이 밖 줄: %s" % (p["lib"], m, rest[:3])); bad = 1
            exp = [b for b, bm in zip(bl, mods) if bm == m]
            if [b[3] for b in gb] != [b[3] for b in exp]:
                print("FAIL %s/%s.sh 순서·목록 다름" % (p["lib"], m)); bad = 1
            for g, e in zip(gb, exp):
                if "".join(gl[g[0]:g[2] + 1]) != "".join(lines[e[0]:e[2] + 1]):
                    print("FAIL %s/%s.sh %s 글자 다름" % (p["lib"], m, g[3])); bad = 1
            for g in gb:
                where.setdefault(g[3], []).append(m)
            allsrc += got
        once = [n for n in (b[3] for b in bl) if len(where.get(n, [])) != 1]
        if once or set(where) - {b[3] for b in bl}:
            print("FAIL %s 정확히 한 번이 아닌 함수: %s / 원본에 없던 함수: %s" % (tool, once, sorted(set(where) - {b[3] for b in bl}))); bad = 1
        # 4) bash가 본 정의 그대로
        orig_defs = defined("".join("".join(lines[b[0]:b[2] + 1]) for b in bl), True)
        if defined(allsrc, True) != orig_defs:
            print("FAIL %s declare -f 다름" % tool); bad = 1
        # 5) 앞당겨지는 정의
        for s, i, e, name in bl:
            pat = re.compile(r"(^|[;&|({`]|\$\(|then|do|else|\|\||&&)\s*" + re.escape(name) + r"(\s|;|$|\))")
            hits = [n + 1 for n in range(s) if pat.search(lines[n]) and not any(b[0] <= n <= b[2] for b in bl)]
            if hits:
                print("알림 %s: %s()는 %d줄에 정의되는데 그 앞 최상위 줄 %s에 명령 낱말로 나온다" % (tool, name, i + 1, hits[:8]))
        print("%s %s: 함수 %d개, 모듈 %s, 본체 %d줄" % ("통과" if not bad else "실패", tool, len(bl), ",".join(libs), len(main)))
    sys.exit(bad)


if __name__ == "__main__":
    a = sys.argv[1:]
    if a[:1] == ["split"]:
        split()
    elif a[:1] == ["check"] and len(a) == 2:
        check(a[1])
    else:
        sys.exit(__doc__)
