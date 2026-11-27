package main

// 탭 구분 파일·줄의 칸 이름(Row.F·Set의 칸 번호, 1부터). 파일 종류마다 한 묶음 — 칸을 더하면 여기에 한 줄 더하고,
// 이 파일 밖에서는 숫자로 칸을 읽지 않는다(cols_guard_test.go). 칸 번호가 곧 파일 꼴이니 값을 바꾸지 않는다(fill-named-cols, 2026-11-24)

// 대기열 queue.tsv(docs/hc-fill/queue.md) — 10~16칸 옛 줄도 읽는다(없는 칸은 빈 값)
const (
	qName    = 1  // 레인 이름
	qProj    = 2  // 프로젝트
	qBrief   = 3  // 지시문 경로
	qAfter   = 4  // 선행(쉼표, 없으면 -)
	qOnly    = 5  // 기계 제한(쉼표, 없으면 -)
	qState   = 6  // wait·up·sent
	qMach    = 7  // 띄운 기계
	qAddr    = 8  // 띄운 터미널 주소
	qStuck   = 9  // 남긴것
	qAdded   = 10 // 넣은 때
	qReply   = 11 // 되돌릴곳(<기계>:<경로>#<요청 ID>, 손으로 넣은 줄은 -)
	qOwner   = 12 // 주인 지휘
	qPrio    = 13 // urgent·high·normal·low
	qEarly   = 14 // 먼저 띄움·다시 띄울 출처 — relaunch.go의 rowMode·setMode로만 읽고 쓴다(값 일곱 가지, queue.md)
	qAudit   = 15 // 자동 감사(-·no-audit·auto·audit=<레인>)
	qOverlap = 16 // 겹침칸(-·solo·overlap-ok)
	qRID     = 17 // 줄 ID(16칸까지의 옛 줄은 빈 값)
	qWeight  = 18 // 무게(light·normal — 지휘가 add·edit --light|--weight로, 없거나 -면 지시문으로 추정, weight.go)
	qNF      = 18 // 지금 꼴 칸 수(18칸은 무게를 정한 줄에만 — 그 밖은 17칸)
)

// 감사 기록 audit.tsv(docs/hc-fill/audit.md) — 한 줄 = 한 차수. 17칸 미만(옛 7칸) 줄은 아래 ato…로 읽어 한 번 옮긴다
const (
	atID        = 1  // 차수 ID
	atLane      = 2  // 쓰기 레인
	atRound     = 3  // 몇 차
	atRun       = 4  // 부모 실행
	atSHA       = 5  // 감사할 sha
	atMach      = 6  // 기계
	atAddr      = 7  // 부모 주소
	atState     = 8  // 상태(queued·deliver·taken·superseded·escalate:… 등)
	atFixes     = 9  // 지적 바퀴 수
	atVerdict   = 10 // 판정
	atWakes     = 11 // 깨움 수
	atWaited    = 12 // 기다린 초
	atChecked   = 13 // 마지막 확인
	atWoke      = 14 // 마지막 깨움
	atNoVerdict = 15 // 판정 없음 바퀴 수
	atFlags     = 16 // 표시(쉼표)
	atNote      = 17 // 비고(끝 칸 — 탭이 든 옛 값은 auRest로 끝까지)
	atNF        = 17

	atoLane  = 1 // 옛 꼴: 레인
	atoSHA   = 2 // 옛 꼴: sha
	atoRound = 3 // 옛 꼴: 몇 차
	atoFixes = 4 // 옛 꼴: 지적 바퀴 수
	atoState = 5 // 옛 꼴: 상태(옛 줄로 치는 가장 짧은 칸 수)
)

// hc-lane 사건 줄(EV — internal/events, docs/hc-lane/events.md). 옛 7~11칸 줄도 같은 칸
const (
	evTime  = 1  // 시각(UTC, 2006-01-02T15:04:05Z)
	evMach  = 2  // 기계
	evTerm  = 3  // 터미널 주소
	evLane  = 4  // 레인
	evState = 5  // 상태(완료·제출·착지·막힘·판정·주차·정리됨 …)
	evSHA   = 6  // sha
	evMsg   = 7  // 한 줄
	evTo    = 9  // --to(받는 지휘)
	evRun   = 10 // 발행 실행(옛 사건은 없거나 -)
)

// 받은편지 장부 inbox.tsv(LED, docs/hc-fill/inbox.md)
const (
	ldRID   = 1 // 요청 ID
	ldLane  = 2 // 레인
	ldReply = 3 // 되돌릴곳
	ldState = 4 // open·deliver·sent·held·…
	ldKind  = 5 // 종류(result·cancel·noreport …)
	ldHash  = 6 // 고정본 해시
	ldSize  = 7 // 고정본 크기
	ldNote  = 8 // 비고
	ldTime  = 9 // 마지막으로 바꾼 시각
)

// 마지막 보고 last.tsv(docs/hc-fill/last-report.md — run 바퀴가 evTick에서 다시 쓴다)
const (
	lsName  = 1 // 레인
	lsState = 2 // 사건 상태(없으면 -)
	lsSHA   = 3 // sha 앞 8자
	lsWhen  = 4 // 사건 시각(이 기계 15:04, 모르면 ?)
	lsMsg   = 5 // 한 줄(40자)
	lsSeen  = 6 // 읽은 시각
	lsFresh = 7 // fresh·old(끊긴 기계의 지난 값)
	lsRID   = 8 // 대기열 줄 ID
)

// 시도 기록 try.tsv(docs/hc-fill/launch-check.md) — tryput이 7칸으로 쓰고, 미확정 시도는 8·9칸을 더한다
const (
	trName  = 1 // 레인
	trRID   = 2 // 대기열 줄 ID
	trMach  = 3 // 기계
	trEarly = 4 // 먼저 띄움 칸 값
	trAudit = 5 // 감사 칸 값
	trAddr  = 6 // 주소(? 미확정, - 확정 실패)
	trRun   = 7 // 확정한 실행 키
	trGot   = 8 // 미확정 때 받은 주소
	trWaits = 9 // 미확정 기다림 바퀴 수
)

// 다시 맞춤 장부 reland.tsv(reland.go) — 첫 줄 「#since<TAB>시각」, 그 밖은 반송 하나의 한 걸음
const (
	rlFailed  = 1 // 반송 이름(refs/merge-failed/<이름>), 머리 줄은 #since
	rlSinceAt = 2 // 머리 줄: 장부를 연 시각
	rlLane    = 2 // 원래 레인
	rlBase    = 3 // 바탕(기본 가지)
	rlWhat    = 4 // 알림·넣음·보냄·다시 냄·막힘 …
	rlArg     = 5 // 넣음: 넣은 줄 이름, 알림: 결과, 보냄·다시 냄: 주소
	rlAt      = 6 // 시각(UTC)
	rlOwner   = 7 // 주인 지휘(알림이 안 간 줄을 다시 알릴 때)
	rlMsg     = 8 // 알림 글
)

// 띄우기 거절·실패 기록 reject.tsv(reject.go)
const (
	rjRID   = 1 // 대기열 줄 ID
	rjLane  = 2 // 레인
	rjKind  = 3 // reject·fail
	rjFirst = 4 // 처음(초)
	rjLast  = 5 // 마지막(초)
	rjTold  = 6 // 알린 사건 키(아직이면 -)
	rjMach  = 7 // 기계
	rjWhy   = 8 // 까닭
)

// 쓰기 소유 실행 기록 own.tsv(OREC, docs/hc-fill/overlap.md)
const (
	owLane    = 1 // 레인
	owProj    = 2 // 프로젝트
	owMach    = 3 // 기계
	owAddr    = 4 // 주소(받기 전이면 pending:…)
	owStart   = 5 // 시작 UTC
	owBrief   = 6 // 지시문 사본
	owAfter   = 7 // 「,선행,」
	owOverlap = 8 // 겹침칸
)

// 겹침 판정의 줄(메모리) — 키 줄 「종류 키」(lkeys·actKeys·candKeys, 종류 R 예약·M 언급·A 실제), 지시문 토막 「종류 꼴 값 원문」(ktok),
// 소유 키 OWN 「프로젝트 레인 종류 키」, 견줄 소유 키 「레인 종류 키」, 겹침 결과 「레인 언급|예약|실제 키들」(ovcmp·OVCHAIN),
// 쓰기 소유 레인 「레인 겹침칸」(owners), 기다림 사슬 QEDGES 「레인 ,선행,」·THENMAP 「이어 붙인 이름 받은 줄」
const (
	kyKind = 1
	kyKey  = 2

	tkKind = 1
	tkType = 2
	tkVal  = 3
	tkRaw  = 4

	onProj = 1
	onLane = 2
	onKind = 3
	onKey  = 4

	okLane = 1
	okKind = 2
	okKey  = 3

	ovLane  = 1
	ovGrade = 2
	ovKeys  = 3

	orLane    = 1
	orOverlap = 2

	qeLane  = 1
	qeAfter = 2
	tmName  = 1
	tmInto  = 2
)

// 겹침 대기 overlap.tsv(OVWAIT 「레인 앞레인 키|단독 기계제한 프로젝트」)·능력 막힘 need.tsv(「레인 능력 보냄」)·
// 먼저 띄움 선행 겹침 early-chain.tsv(「레인 줄ID 앞레인 키」)·실제 변경 조회 줄(「프로젝트 레인 +|=파일|!까닭」)
const (
	wtLane  = 1
	wtFront = 2
	wtKey   = 3
	wtOnly  = 4
	wtProj  = 5
	wtQueue = 6 // 「줄」: 앞선 대기 줄 뒤(overlap_fair.go), 없으면 쓰기 소유 레인과 겹침

	ndLane = 1
	ndNeed = 2

	ecLane  = 1
	ecRID   = 2
	ecFront = 3
	ecKeys  = 4

	acProj = 1
	acLane = 2
	acVal  = 3
)

// 병합기: 원격 참조 MQS 「프로젝트 참조 SHA」, 켬 표식 merger.tsv 「프로젝트 기계」, 병합기 살아 있음 merger-alive.tsv(maRow 칸 차례)
const (
	mqProj = 1
	mqRef  = 2
	mqSHA  = 3

	mgProj = 1
	mgMach = 2

	maProj    = 1
	maMach    = 2
	maState   = 3
	maSince   = 4
	maAddr    = 5
	maQueued  = 6
	maOldest  = 7
	maChecked = 8
	maAlerted = 9
	maWhy     = 10
	maOldLane = 11 // 가장 오래된 제출의 레인(advise-merge-stall-age — 옛 기록 10칸은 빈 값)
	maQLanes  = 12 // 줄 차례대로 레인(쉼표, 빈 줄 -, 큐 못 읽음 ? — ht-fill-submitted-line, 옛 기록 11칸은 빈 값 = 모름)
	maRec     = 13 // 병합기 기록 한 장(internal/mergerstate JSON 그대로, 없으면 - — ht-merger-state-record)
	maRecWhy  = 14 // 그 기록을 못 읽은 까닭(읽었으면 -)
)

// 작은 상태 파일들
const (
	// 착지 뒤 명령 onland.tsv 「레인 이름 프로젝트 주인 접수초」, 차례 onland/<이름>.pending 「레인 주인 sha」
	olLane  = 1
	olName  = 2
	olProj  = 3
	olOwner = 4
	olAt    = 5
	opLane  = 1
	opOwner = 2
	opSHA   = 3

	// 감사 자리 기다림 aseat.tsv 「감사 줄 바퀴 수 처음(초) 알림(1)」
	asName  = 1
	asTurns = 2
	asFirst = 3
	asTold  = 4

	// 놀고 있음 idle.tsv 「기계 비기 시작 보낸 시각 보낸 횟수 빈 자리 사유」
	idMach  = 1
	idSince = 2
	idSent  = 3
	idCount = 4
	idFree  = 5
	idWhy   = 6

	// 기계 부하 load.tsv 「기계 l1 코어 잰 때 쓴 자리 상한 까닭 뜨거워진 때 보낸 때」
	loMach  = 1
	loL1    = 2
	loCores = 3
	loAt    = 4
	loUsed  = 5
	loCap   = 6
	loWhy   = 7
	loHot   = 8
	loSent  = 9
	loVer   = 10 // 도는 저장소 검증 수(「-」 모름) — 옛 판 줄에는 없어 loNF는 그대로
	loFree  = 11 // 그 바퀴 띄우기가 판정한 빈 자리(machRoom) — 옛 판 줄에는 없음(모름)
	loGates = 12 // 도는 검증 가운데 병합기 검증 수(「-」 모름) — 옛 판 줄에는 없음(모름)
	loRest  = 13 // 자리에 안 드는 쉬는(done·idle) 대기열 터미널 수 — 옛 판 줄에는 없음(모름)
	loOut   = 14 // 자리에 안 드는 대기열 밖 에이전트 수(사람·지휘 세션·손 레인·루프) — 옛 판 줄에는 없음(모름)
	loNF    = 9

	// 묶인 기계 pinned.tsv 「레인 줄ID 처음 본 초 까닭」(pinned.go)
	pnLane  = 1
	pnRID   = 2
	pnSince = 3
	pnWhy   = 4

	// 기계 닿지 않음 reach.tsv(reachRow 칸 차례)
	rcMach    = 1
	rcState   = 2
	rcSince   = 3
	rcAlerted = 4
	rcLanes   = 5

	// 저장소 읽기 막힘 격리 cordon.tsv(cordonRow 칸 차례 — cordon.go)
	cdMach    = 1
	cdState   = 2
	cdSince   = 3
	cdAt      = 4
	cdOn      = 5
	cdAlerted = 6
	cdWhy     = 7
	cdNF      = 7

	// 선행 착지 표식 after-landed.tsv 「프로젝트 레인 표식 출처 표식」
	alProj = 1
	alLane = 2
	alFrom = 3
	alMark = 4

	// 쉼 held.tsv·HNEW 「주소 쉬기 시작 초」, hc-lane 레인 기록 lanes.tsv 「주소 이름 … 종류」, 끊긴 기계 seen.tsv 「기계 이름 주소」
	hdAddr  = 1
	hdSince = 2
	lnAddr  = 1
	lnName  = 2
	lnKind  = 4
	snMach  = 1
	snName  = 2
	snAddr  = 3

	// 감사 프로젝트 audit-proj.tsv 「차수 프로젝트」, 낮음 후속 lows.tsv 「넣은 시각(초) 차수 레인 후속 n」
	apAid  = 1
	apProj = 2
	lwAt   = 1

	// evTick의 떠 있음 줄(메모리) 「이름 기계 주소 줄ID」
	upName = 1
	upMach = 2
	upAddr = 3
	upRID  = 4

	// 다시 굴림 기록 nudge.tsv(nudge.go)·멈춤 끊음 기록 hang-esc.tsv(hang.go) 「레인 주소 보낸 시각(초)」
	nuLane = 1
	nuAddr = 2
	nuAt   = 3

	// 멈춘 레인 의심 hang.tsv(hang.go) 「레인 주소 종류(a|b) 처음 본 초 화면 지문」
	hgLane  = 1
	hgAddr  = 2
	hgKind  = 3
	hgSince = 4
	hgFP    = 5

	// 기계 옛 이름 machine-alias.tsv 「옛 새 바꾼 초」(rename.go — 병합기가 옛 이름으로 적은 결과 사건을 새 이름 것으로)
	aaOld = 1
	aaNew = 2
	aaAt  = 3

	// 첫 칸이 열쇠인 「열쇠 값」 줄(knobs-remote.tsv, 받은편지 요청 머리, spec-verifiers.tsv 등) — lsFirst는 어느 파일이든 첫 칸으로 찾는다
	kvKey = 1
	kvVal = 2
)

// 사람 리뷰 장부 pr-human.tsv(prhuman.go, docs/hc-fill/pr.md 「사람 리뷰」) — 첫 칸이 줄 종류, 둘째 칸이 PR(「<호스트/주인/이름>#<번호>」, on 줄은 프로젝트)
const (
	phWhat = 1 // on·note·hand·hold·done·back
	phPR   = 2
	// on 줄: 켠 때(그 앞에 쓴 글은 읽지 않는다)
	phOnAt = 3
	// note 줄: 글 하나
	phID     = 3 // r·l·c + GitHub 번호
	phKind   = 4 // changes·approve·review·line·comment
	phAt     = 5
	phAuthor = 6
	phRes    = 7 // new(넘길 것)·approve(승인 — 넘기지 않음)·skip:bot·skip:author
	phWhere  = 8 // 줄 댓글의 파일:줄(없으면 -)
	phBody   = 9 // 한 줄로 편 본문(상한)
	// hand 줄: 넘김
	phIDs   = 3 // 넘긴 글 id(쉼표, 다시 막아 넘김이면 -)
	phLane  = 4 // 그때의 레인(사건 이름·주인)
	phHow   = 5 // send:<주소>·lane:<새 레인>·tell(막지 않는 글 — 지휘에 알림만)·after(착지·닫힌 뒤 — 사건만)
	phTime  = 6
	phBlock = 7 // 1 = 착지를 막는 글이 있음
	// hold 줄: 병합 줄에서 뺌 / back 줄: 다시 세움
	phQ     = 3 // 뺀 큐 이름(<초>-<레인>)
	phQLane = 4
	phQAt   = 5
	phQKind = 6 // hold: fix(고칠 것)·approve(재승인 기다림), back: 새 큐 이름
	// done 줄: 반영됨
	phSHA    = 3 // 감사를 통과한 고친 커밋(지휘가 풀었으면 -)
	phDoneAt = 4
	phWhy    = 5 // 지휘가 풀었을 때의 까닭
)

// cv: 칸 하나와 값(aupd·lset)
type cv struct {
	c int
	v string
}
