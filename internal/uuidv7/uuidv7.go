// Package uuidv7 makes version 7 UUIDs (RFC 9562) with the standard library
// only.
//
// A version 7 UUID starts with the Unix time in milliseconds, so sorting the
// strings sorts them by time, and the time can be read back from the ID.
package uuidv7

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"time"
)

// New returns a version 7 UUID for the current time, in the usual
// 8-4-4-4-12 hexadecimal form.
func New() string {
	return NewAt(time.Now())
}

// NewAt returns a version 7 UUID whose timestamp is t.
//
// Layout (RFC 9562, section 5.7):
//
//	48 bits  unix_ts_ms
//	 4 bits  version (0111)
//	12 bits  rand_a
//	 2 bits  variant (10)
//	62 bits  rand_b
func NewAt(t time.Time) string {
	var b [16]byte
	// crypto/rand.Read never returns an error on supported platforms.
	rand.Read(b[6:])

	var ms [8]byte
	binary.BigEndian.PutUint64(ms[:], uint64(t.UnixMilli()))
	copy(b[0:6], ms[2:8])

	b[6] = b[6]&0x0f | 0x70
	b[8] = b[8]&0x3f | 0x80
	return format(b)
}

// Time returns the timestamp stored in a version 7 UUID, and false if s is
// not one.
func Time(s string) (time.Time, bool) {
	b, ok := parse(s)
	if !ok || b[6]>>4 != 7 || b[8]>>6 != 2 {
		return time.Time{}, false
	}
	var ms [8]byte
	copy(ms[2:8], b[0:6])
	return time.UnixMilli(int64(binary.BigEndian.Uint64(ms[:]))), true
}

func format(b [16]byte) string {
	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:])
}

func parse(s string) ([16]byte, bool) {
	var b [16]byte
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return b, false
	}
	groups := []struct{ from, to, at int }{{0, 8, 0}, {9, 13, 4}, {14, 18, 6}, {19, 23, 8}, {24, 36, 10}}
	for _, g := range groups {
		if _, err := hex.Decode(b[g.at:], []byte(s[g.from:g.to])); err != nil {
			return b, false
		}
	}
	return b, true
}
