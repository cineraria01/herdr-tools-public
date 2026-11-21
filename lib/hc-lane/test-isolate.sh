# 시험 격리(시험 파일이 T를 만든 바로 뒤에 `. "$here/isolate.sh"`): 시험이 바깥 저장소·사용자 상태를 건드리지 않게.
# 2026-11-20 병합기 첫 착지: 병합기가 돌린 herdr-tools 시험이 바깥 병합기 클론(후보 작업 트리의 공용 git 폴더)을 보고, 바깥 병합기의
# 잠금 fd를 물려받은 채 돌아 15분 멈췄다(지휘 t1cj). 그래서:
#  · 호출자가 열어 둔 병합기 잠금(fd 19)과 후보 잠금(fd 18)을 닫는다
#  · 바깥 git 환경(GIT_DIR 등)을 지우고, git이 T 위로 저장소를 찾아 올라가지 못하게 한다(T가 바깥 작업 트리 안이어도)
#  · XDG 폴더를 지워 각 시험의 HOME 아래 기본값을 쓰게 한다(HOME은 시험마다 정한다)
#  · 작업 폴더를 T로
#  · 바깥 레인 환경(HC_LANE·HC_LANE_OWNER 등)을 지운다 — 레인 세션에서 검증을 돌리면 report가 바깥 주인을 11칸에 적어 hc-lane 시험
#    3개(299·579·582)가 졌다(2026-11-21 fill-park-readopt·fill-gc-async가 따로 찾아 합침, 작업 중 발견해 함께 고침).
#    필요한 시험은 스스로 넣는다
exec 18>&- 19>&-
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_COMMON_DIR GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_NAMESPACE GIT_PREFIX \
  XDG_STATE_HOME XDG_CONFIG_HOME XDG_CACHE_HOME XDG_DATA_HOME \
  HC_LANE HC_LANE_OWNER HC_LANE_OWNER_RUN HC_LANE_LEASE HERDR_LAND_PRIORITY HC_LANE_PRIORITY HC_LANE_MERGER_BIN
# Git은 시작 폴더 자체를 ceiling으로 검사하지 않는다. T에서 시작할 때는 그 부모에서 끊는다.
export GIT_CEILING_DIRECTORIES="$T:${T%/*}"
cd "$T" || exit 1
