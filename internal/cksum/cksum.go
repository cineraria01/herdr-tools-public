// Package cksum: POSIX cksum(1)과 같은 CRC·길이. herdr-sync 서명(「crc.크기」)과 herdr-settings-merge base(「CRC 길이」)가
// 셸 판(cksum 명령)과 같은 값을 내야 두 판이 같은 상태 파일을 번갈아 읽는다.
package cksum

import (
	"fmt"
	"io"
)

var table [256]uint32

func init() {
	for i := range table {
		c := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if c&0x80000000 != 0 {
				c = c<<1 ^ 0x04C11DB7
			} else {
				c <<= 1
			}
		}
		table[i] = c
	}
}

// Digest: 흘려 넣는 cksum
type Digest struct {
	crc uint32
	n   int64
}

func (d *Digest) Write(p []byte) (int, error) {
	c := d.crc
	for _, b := range p {
		c = c<<8 ^ table[byte(c>>24)^b]
	}
	d.crc, d.n = c, d.n+int64(len(p))
	return len(p), nil
}

// Sum: (CRC, 길이) — cksum 출력의 두 수
func (d *Digest) Sum() (uint32, int64) {
	c := d.crc
	for n := d.n; n > 0; n >>= 8 {
		c = c<<8 ^ table[byte(c>>24)^byte(n)]
	}
	return ^c, d.n
}

// Bytes: b의 (CRC, 길이)
func Bytes(b []byte) (uint32, int64) {
	var d Digest
	d.Write(b)
	return d.Sum()
}

// Reader: r을 끝까지 읽은 (CRC, 길이)
func Reader(r io.Reader) (uint32, int64, error) {
	var d Digest
	if _, err := io.Copy(&d, r); err != nil {
		return 0, 0, fmt.Errorf("cksum 읽기: %w", err)
	}
	c, n := d.Sum()
	return c, n, nil
}

// Dot: 「crc.크기」(herdr-sync 서명 꼴)
func Dot(b []byte) string {
	c, n := Bytes(b)
	return fmt.Sprintf("%d.%d", c, n)
}

// Space: 「CRC 길이」(herdr-settings-merge base 꼴)
func Space(b []byte) string {
	c, n := Bytes(b)
	return fmt.Sprintf("%d %d", c, n)
}
