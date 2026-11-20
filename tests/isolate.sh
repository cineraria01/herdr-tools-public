# 시험 폴더에서 읽는 공용 진입점. 설치본의 도구 모듈에도 같은 격리 코드가 있다.
isolate_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P) || exit 1
isolate_file=$isolate_dir/../lib/hc-lane/test-isolate.sh
[ -f "$isolate_file" ] || isolate_file=$isolate_dir/../../lib/hc-lane/test-isolate.sh
. "$isolate_file" || exit 1
