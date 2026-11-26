package main

import "testing"

// carryReplace: 경로 글자가 이어지는 더 긴 경로 안에서는 바꾸지 않고, 문장 끝 마침표·조사·백틱 앞에서는 바꾼다
func TestCarryReplace(t *testing.T) {
	got := carryReplace("`/s/x.py`를 보고 /s/x.py.orig 는 두고 /s/x.py. 끝 a/s/x.py", "/s/x.py", ".qa/carry/x.py")
	want := "`.qa/carry/x.py`를 보고 /s/x.py.orig 는 두고 .qa/carry/x.py. 끝 a/s/x.py"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// carryRel: .qa 밖으로 나가는 경로는 싣지 않는다
func TestCarryRel(t *testing.T) {
	for in, want := range map[string]string{".qa/a/b": ".qa/a/b", "./.qa/a": ".qa/a", ".qa/../x": "", ".qa/a/../../y": "", "qa/a": ""} {
		if got := carryRel(in); got != want {
			t.Errorf("carryRel(%q) = %q, want %q", in, got, want)
		}
	}
}
