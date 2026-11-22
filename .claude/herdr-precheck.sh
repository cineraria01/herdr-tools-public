#!/bin/bash
# herdr-tools 제출 전 사전 검사: hc-lane land가 병합기 큐에 넣기 직전에 부른다(실패면 넣지 않음, go-followups).
# 검증의 싼 검사(go vet ./... · go test ./tests/repo/)만 돈다. 무거운 시험은 병합기가 착지 전에 돈다
exec bash "$(dirname "$0")/herdr-verify.sh" --cheap-only
