// Package shx: Go가 sh -c·hc exec로 보내는 셸 조각의 머리·끝 표식(gofix3-shx, 재감사 2바퀴 묶음 E).
// 앞 단계 실패를 뒤 단계(sed·printf)가 가리거나, 응답이 표식 앞에서 잘렸거나, 마지막 단계가 1로 끝나는 것을 한 계약으로 가른다:
// 조각의 출력은 Cut이 ok일 때만 쓰고, 아니면 모름(부르는 쪽 상태 불변).
package shx

import "strings"

// Mark: 조각의 성공 경로 맨 끝에 한 줄로 찍힌다(RS 글자로 시작 — 보통 출력에 안 나온다)
const Mark = "\x1eherdr-shx-end"

// head: pipefail을 못 켜는 셸이면 97(모름). 하위 셸에서 먼저 시험한다 — 특수 내장 set의 실패는 셸마다 종료 코드가 달라서
const head = "(set -o pipefail) 2>/dev/null || exit 97\nset -u -o pipefail\n"

// Script: body 앞에 엄격 머리(set -u · set -o pipefail — 못 켜는 셸이면 97로 끝)를, 끝에 표식 줄을 붙인다.
// body는 성공하면 끝까지 흘러야 한다(중간 exit 0 금지 — 표식이 안 찍힌다). 실패는 0 아닌 exit로.
// set -e는 쓰지 않는다(&& 목록·함수 반환 예외가 많다) — 각 단계는 `|| exit N`으로 명시하고, 마지막 단계의 1은 결과를 바꾸지 않는다
func Script(body string) string {
	return head + Tail(body)
}

// Tail: 엄격 머리 없이 끝 표식만 — pipefail이 없는 셸에서도 돌아야 하는 조각용(리눅스 기계의 sh = dash 0.5.12, 데비안 12·우분투 24.04:
// dash는 0.5.13부터 pipefail을 받는다). 그런 조각은 파이프 앞 단계의 실패가 드러나지 않으니, 실패가 결과를 바꾸는 단계는
// 파이프에 두지 않고 파일로 받아 종료 코드를 따로 본다(|| exit N). 출력은 Script와 같이 Cut으로 읽는다
func Tail(body string) string {
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return body + "printf '\\036herdr-shx-end\\n'\n"
}

// ZX: 그 기계의 셸 환경에서 명령 — `sh -c ZX <이름> <폴더> <셸 환경> <명령…>`. <셸 환경>이 비어 있지 않고 zsh·~/.zshrc가 있으면
// 로그인 zsh로 ~/.zshrc까지 읽고(hc 코어 PATH에는 bun 등이 없다) <폴더>에서, 아니면 지금 환경 그대로 <폴더>에서 exec.
// hc-lane 레인의 준비 단계·fetch(zx)와 병합기 추측 일꾼의 준비 훅·검증이 함께 쓴다(merger-journal-rounds B, 2026-11-23)
const ZX = `cd "$1" || exit 1
    if [ -n "$2" ] && [ -f "$HOME/.zshrc" ] && command -v zsh >/dev/null 2>&1; then
      d=$1; shift 2
      exec zsh -lc "source \"\$HOME/.zshrc\" >/dev/null 2>&1; cd \"\$1\" && shift && exec \"\$@\"" zsh "$d" "$@"
    fi
    shift 2; exec "$@"`

// Cut: 조각 출력에서 표식을 떼고 본문을. rc 0이고 마지막 줄이 정확히 Mark(+LF)일 때만 ok.
// 끝 LF는 없어도 된다(부르는 쪽이 끝 줄바꿈을 뺀 출력) — 표식 글자가 다 왔으면 본문은 온전하다
func Cut(out string, rc int) (body string, ok bool) {
	if rc != 0 {
		return "", false
	}
	b, found := strings.CutSuffix(strings.TrimSuffix(out, "\n"), Mark)
	if !found || b != "" && !strings.HasSuffix(b, "\n") {
		return "", false
	}
	return b, true
}
