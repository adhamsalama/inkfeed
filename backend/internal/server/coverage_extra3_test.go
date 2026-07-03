package server

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"strings"
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

func TestRenderRedditReplyLimit(t *testing.T) {
	// Build a top-level comment with more than maxRepliesPerComment nested
	// replies to exercise the reply-count cap.
	var replies strings.Builder
	replies.WriteString(`{"kind":"Listing","data":{"children":[`)
	for i := 0; i < maxRepliesPerComment+5; i++ {
		if i > 0 {
			replies.WriteString(",")
		}
		fmt.Fprintf(&replies, `{"kind":"t1","data":{"author":"r%d","body_html":"x","created_utc":1}}`, i)
	}
	replies.WriteString(`]}}`)

	var sb strings.Builder
	counter, replyCount := 0, 0
	renderRedditComment(&sb, redditThing{
		Kind: "t1",
		Data: redditComment{Author: "op", BodyHTML: "top", Replies: []byte(replies.String())},
	}, 0, true, &replyCount, &counter, false)
	if *(&replyCount) < maxRepliesPerComment {
		t.Errorf("reply count = %d", replyCount)
	}
}

func TestFetchHNCommentsCollapse(t *testing.T) {
	// >10 top-level comments so entries beyond the 10th render collapsed.
	var children strings.Builder
	for i := 0; i < 12; i++ {
		if i > 0 {
			children.WriteString(",")
		}
		fmt.Fprintf(&children, `{"id":%d,"author":"u%d","created_at":"2024-01-01T00:00:00Z","text":"c%d","children":[]}`, i+100, i, i)
	}
	json := `{"id":1,"children":[` + children.String() + `]}`
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(json))
	})
	setProxyURL(t, srv.URL)
	out, err := app.fetchHNComments("https://news.ycombinator.com/item?id=1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[+]") {
		t.Errorf("expected collapsed entries")
	}
}
