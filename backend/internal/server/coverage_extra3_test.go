package server

import (
	"encoding/binary"
	"testing"
)

// riffChunk builds a RIFF/WEBP container holding a single chunk with the given
// FourCC and payload. Used to drive decodeWebP's manual RIFF walker (the simple
// x/image/webp decoder rejects these, so the walker's switch runs).
func riffChunk(fourcc string, payload []byte) []byte {
	chunk := []byte(fourcc)
	sz := make([]byte, 4)
	binary.LittleEndian.PutUint32(sz, uint32(len(payload)))
	chunk = append(chunk, sz...)
	chunk = append(chunk, payload...)
	if len(payload)%2 == 1 {
		chunk = append(chunk, 0)
	}
	body := append([]byte("WEBP"), chunk...)
	out := []byte("RIFF")
	rsz := make([]byte, 4)
	binary.LittleEndian.PutUint32(rsz, uint32(len(body)))
	out = append(out, rsz...)
	out = append(out, body...)
	return out
}

func TestDecodeWebPWalkerBranches(t *testing.T) {
	// VP8 (lossy) chunk with garbage -> walker enters the "VP8 " case, the frame
	// header decode fails and it returns an error (lines still executed).
	if _, err := decodeWebP(riffChunk("VP8 ", []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})); err == nil {
		t.Log("VP8 garbage unexpectedly decoded")
	}
	// VP8L (lossless) chunk with garbage -> walker enters the "VP8L" case.
	if _, err := decodeWebP(riffChunk("VP8L", []byte{0x2f, 1, 2, 3, 4, 5, 6, 7})); err == nil {
		t.Log("VP8L garbage unexpectedly decoded")
	}
	// Unknown chunk then truncated -> walker loops and finds nothing.
	if _, err := decodeWebP(riffChunk("XXXX", []byte{1, 2, 3, 4})); err == nil {
		t.Error("expected error for container with no VP8 chunk")
	}
}
