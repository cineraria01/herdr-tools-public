package atomic

import "os"

// Append: data를 path 뒤에 한 번의 O_APPEND 쓰기로 붙인다(없으면 perm으로 만든다) — 함께 쓰는 실행끼리 줄이 섞이지 않게.
// 오류는 os의 것 그대로(쓰기가 실패하면 닫기 오류보다 쓰기 오류). 줄바꿈은 부르는 쪽이 붙인다
func Append(path string, perm os.FileMode, data string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, perm)
	if err != nil {
		return err
	}
	_, err = f.WriteString(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
