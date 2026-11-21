package main

import (
	"fmt"
	"maps"
	"strings"
)

// selfTest: --self-test(셸 판 self_test와 같은 단언). main_test.go도 이것을 부른다
func selfTest() error {
	app := obj{"address": "t1", "cwd": "/r/proj", "where": "proj / 1", "state": "live"}
	remote := obj{"address": "w1/t2", "cwd": "/r/proj", "where": "proj / 1", "state": "live"}
	me := obj{"address": "t9", "cwd": "/r/proj", "labels": obj{}}
	check := func(ok bool, what string, v any) error {
		if !ok {
			return fmt.Errorf("%s: %v", what, v)
		}
		return nil
	}
	pairs, isNew := place(me, []obj{remote, app}, func(a string) string {
		if a == "t1" {
			return "sp1"
		}
		return "spX"
	})
	if err := check(isNew && pairs[space] == "sp1" && strings.HasPrefix(pairs[tabL], "ovis-"), "같은 폴더 앱 칸의 공간", pairs); err != nil {
		return err
	}
	if err := check(pairs[wsName] == "proj" && pairs[tabName] == "지휘 t9", "공간·탭 이름", pairs); err != nil {
		return err
	}
	pairs, isNew = place(me, nil, func(string) string { return "" })
	if err := check(isNew && strings.HasPrefix(pairs[space], "ovis-"), "후보 없으면 새 공간", pairs); err != nil {
		return err
	}
	peer := obj{"address": "t3", "cwd": "/r/proj", "state": "live"} // 같은 폴더의 다른 지휘
	if p, _ := place(me, []obj{peer}, func(string) string { return "ovis-peer" }); p[space] != "ovis-peer" {
		return fmt.Errorf("다른 지휘의 공간: %v", p)
	}
	if p, _ := place(me, []obj{maps.Clone(me)}, func(string) string { return "self" }); p[space] == "self" {
		return fmt.Errorf("자기 자신은 후보가 아니다: %v", p)
	}
	placed := obj{"address": "t9", "cwd": "/r/proj", "labels": obj{space: "sp1", tabL: "tb1", "where": "proj / 1"}}
	p, isNew := place(placed, []obj{app}, func(string) string { return "x" })
	if err := check(!isNew && maps.Equal(p, map[string]string{space: "sp1", tabL: "tb1"}), "이미 붙은 라벨", p); err != nil {
		return err
	}
	l := workerLabels(map[string]string{space: "sp1", tabL: "tb1"}, obj{"address": "t5", "name": "lane-a"}, "t9")
	if err := check(maps.Equal(l, map[string]string{space: "sp1", tabL: "", wsName: "", tabName: "lane-a · t9"}), "worker 라벨", l); err != nil {
		return err
	}
	if l := workerLabels(map[string]string{space: "sp1", tabL: "tb1"}, obj{"address": "t6"}, "t9"); l[tabName] != "t6 · t9" {
		return fmt.Errorf("이름 없는 worker: %v", l)
	}
	if !droppable(obj{"address": "t9"}, "t9") || !droppable(obj{"address": "t5", "parent": "t9"}, "t9") {
		return fmt.Errorf("지휘·자식은 뺄 수 있다")
	}
	if droppable(obj{"address": "t7", "parent": "t8"}, "t9") {
		return fmt.Errorf("남의 자식은 못 뺀다")
	}
	if droppable(obj{"address": "t5", "parent": "t9", "labels": obj{"where": "proj / 1"}}, "t9") {
		return fmt.Errorf("Herdr 앱 칸은 못 뺀다")
	}
	return nil
}
