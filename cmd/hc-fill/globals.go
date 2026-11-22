// hc-fill Go 판(2026-11-21 사용자 「모두 고로 옮기고 싶어」, 레인 hc-fill-go). 셸 판(bin/hc-fill·lib/hc-fill/*.sh)을 그대로 옮겼다 —
// 명령·인자·출력·종료 코드·상태 파일·사건 꼴이 같고, 두 판이 같은 상태를 번갈아 읽어도 깨지지 않는다. 바뀐 것: 잠금이 자식에 물리지
// 않음(O_CLOEXEC), 상태 파일 쓰기는 같은 폴더 임시 → fsync → rename, git·hc 결과를 있음·없음·모름으로 가름.
//
// 전역 상태: 셸 판의 바퀴 전역 변수를 같은 이름 꼴로 둔다(한 바퀴 = 한 프로세스, 차례로 돈다). 빈 값 뜻도 셸 판과 같다.
package main

import "sync/atomic"

// 실행 환경(main이 채움)
var (
	CONF   string // 설정 파일 hc-fill.tsv
	DATA   string // 데이터 폴더 ~/.local/share/hc-fill
	Q      string // 대기열 DATA/queue.tsv
	LED    string // 받은편지 장부 DATA/inbox.tsv
	AT     string // 감사 기록 DATA/audit.tsv
	WAIT   = 60   // HC_FILL_WAIT
	POLL   = 5    // HC_FILL_POLL
	LOCAL  string // 설정 local
	AGENT  string // 설정 agent(기본 claude-code)
	SELF   string // 이 도구를 다시 부를 경로(이 바이너리 — HC_FILL_SELF가 있으면 그것)
	HOME   string
	dry    bool   // run --dry-run
	P      string // dry-run이면 "[dry-run] "
	DRC    = 1    // die 종료 코드(add --then은 2)
	gcLock string // _gc가 쥔 잠금 경로(명령 줄에 보이게 인자로 받는다)
)

// X: 원격 sh 머리 — 첫 인자 경로의 ~/ 를 그 기계 HOME으로
const X = `d=$1; shift; case $d in "~/"*) d=$HOME/${d#"~/"} ;; esac; `

// HELDW: 자리를 다 셌다(held.tsv를 비우지 않음) — run 신호 고루틴의 끝내기 훅도 읽는다(R-20)
var HELDW atomic.Bool

// 바퀴 상태(run이 채움). 목록은 셸 판의 줄 목록 변수와 같은 뜻
var (
	MQP, MQBAD          string // 병합기 큐를 읽은 프로젝트·못 읽은 프로젝트(「,<프로젝트>」 목록)
	MQS                 []Row  // 「프로젝트 참조 SHA」
	SB                  string // DATA/.run-brief.tmp — 이번 줄 지시문 사본
	IDLE                int
	TASK, LAUNCH, COORD string
	AUD, AFIN           string // 설정 audit(~ 펼침)·audit_final
	AMAX                = 2
	TERMS               []Term
	LANEADDR            []string // 라벨 hc-lane=1 터미널 주소
	CONN                []string // 연결된 기계
	ICON                []string // inbox·reply가 쓰는 연결 목록
	WAKE, ACKMAX        int
	EV                  []string // hc-lane events 출력 줄(사건 줄과 「모름 <기계>」)
	EVBAD               bool
	MGM                 string // 켜진 병합기 기계(공백 목록, 앞 공백 있음)
	OWNF, OWND          string // DATA/own.tsv·DATA/own
	OREC                []Row  // 쓰기 소유 실행 기록 「레인 프로젝트 기계 주소 시작UTC 지시문사본 ,선행, 겹침칸」
	OSEED               bool
	AUDN                []string // 자동 감사 줄 이름
	AWT                 []string // 자동 감사 기다림(자리는 감사 줄이) 쓰기 레인 주소
	TAL                 map[string]string
	WC                  map[string]int // 줄마다 그 줄을 --after로 기다리는 줄 수
	ROWS                []Row          // 이번 바퀴 대기열 줄(정렬)
	NOW, NOWL           []string       // 이번 바퀴에 띄운 기계(에이전트 수·hc-fill 레인 수)
	OFF                 string         // 이번 바퀴에 연결 안 됨으로 알린 기계(「,<기계>」)
	FETCHED, FETCHBAD   string         // 「,<프로젝트>」
	FR, FB              string         // fetch_proj: local 저장소·기본 가지
	ALLM                []string       // 셈 all 기계
	HOLD                int
	LKIND               map[string]bool // 「주소<TAB>이름」 → 레인 기록이 read
	HELD                map[string]string
	HNEW                []string // 「주소<TAB>쉬기 시작 초」 (held.tsv에 쓸 차례대로)
	HNOW                int64
	SEATS               []string
	ASEC                string // 자동 감사 쓰기 레인의 run 지시문 끝 절
	OVM                 string // 설정 overlap: block|warn|off
	OWN                 []Row  // 쓰기 소유 키 「프로젝트 레인 R|M 키」
	OVWAIT              []Row  // 이번 바퀴 겹침 대기 「레인 앞레인 키|단독 기계제한 프로젝트」
	QEDGES              []Row  // 「레인 ,선행,」
	THENMAP             []Row  // 「이어 붙인 이름 받은 줄」
	NDOLD               []string
	NDNOW               []string // 「레인<TAB>능력<TAB>보냄1|0」
	ELIST               string   // 「,<레인>=<기본 가지>」
	UPNOW               string   // 「,<레인>」
	ECAPHIT             bool
	ECAPM               string
	ECAP                string // 설정 early 그대로(빈 값 = 남은 자리 전부)
	EWAIT               int
	SEENOFF             []Row // 끊긴 기계의 지난 터미널 「기계 이름 주소」
)

// 줄 하나를 띄우는 동안의 값(셸 판 run 고리의 변수 — place·launch·own_add가 읽는다)
var (
	RID       string // 대기열 줄 ID(17칸)
	NEED      string // 지시문 「필요:」 능력(쉼표)
	stuck     string // 이번 줄의 남긴것(9칸)
	proj      string // 이번 줄 프로젝트
	RESUME    bool   // 주차한 줄을 그 가지에서 이어 띄움
	RSEC      string // 이어 가기 절
	ESEC      string // 먼저 띄움 절
	AK        string // 감사 칸(15칸, 빈 값은 -)
	PRIO      string // 우선 칸(13칸) — 띄울 때 레인 환경 HC_LANE_PRIORITY(제출 우선)로
	PAFT      string // 선행(4칸)
	PSOLO     string // 겹침칸(16칸, 빈 값은 -)
	placed    bool
	bad       bool
	GOTADDR   bool
	ADDR      string
	START_RUN string
	SWHY      string
	EARLYF    string
	LAUTO     string
	pst_      string // 띄우기 시작 UTC
)

// 조회가 남기는 값(셸 판과 같은 뜻)
var (
	PRUN, PNAME string // pget
	LHIT        string // lane_landed
	LMS, LMFROM string // lmark
	hit         string // landing_hit
	bBranch     string // after_ok가 둔 기본 가지(셸 판 b)
	PSV         string // pred_state
	PKL         string // parkev
	OVWARN      []Row  // ovl_check 경고 「앞레인 예약|언급 키」
	OVKEYS      []Row  // lkeys 「R|M 키」
	AUNK        []string
)

// Term: hc ls 터미널 한 줄 — 기계 주소 종류 이름 상태 cwd(셸 판 TERMS 칸 차례)
type Term struct{ M, Addr, Agent, Name, Status, Cwd string }
