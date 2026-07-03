package mobi

import (
	"bytes"
	"strings"
	"testing"
)

func TestFourBytesPadding(t *testing.T) {
	cases := map[int]int{0: 0, 1: 3, 2: 2, 3: 1, 4: 0, 5: 3, 8: 0}
	for in, want := range cases {
		if got := fourBytesPadding(in); got != want {
			t.Errorf("fourBytesPadding(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestConcat(t *testing.T) {
	got := concat([]byte{1, 2}, nil, []byte{3}, []byte{})
	want := []byte{1, 2, 3}
	if !bytes.Equal(got, want) {
		t.Errorf("concat = %v, want %v", got, want)
	}
	if concat() != nil {
		t.Errorf("concat() should be nil")
	}
}

func TestPad4(t *testing.T) {
	for _, in := range [][]byte{{}, {1}, {1, 2, 3}, {1, 2, 3, 4}} {
		got := pad4(append([]byte{}, in...))
		if len(got)%4 != 0 {
			t.Errorf("pad4(%v) length %d not multiple of 4", in, len(got))
		}
	}
}

func TestEncVarForward(t *testing.T) {
	cases := map[int][]byte{
		0:   {0x80},
		-5:  {0x80},
		1:   {0x81},
		127: {0xff},
		128: {0x01, 0x80},
	}
	for in, want := range cases {
		if got := encVarForward(in); !bytes.Equal(got, want) {
			t.Errorf("encVarForward(%d) = %v, want %v", in, got, want)
		}
	}
}

func TestEncVarBackward(t *testing.T) {
	cases := map[int][]byte{
		0:   {0x80},
		-1:  {0x80},
		1:   {0x81},
		127: {0xff},
		300: {0x02, 0xac},
	}
	for in, want := range cases {
		if got := encVarBackward(in); !bytes.Equal(got, want) {
			t.Errorf("encVarBackward(%d) = %v, want %v", in, got, want)
		}
	}
}

func TestEncodeNumberAsHex(t *testing.T) {
	// entry 10 must be "0A" (hex), zero-padded to even length, prefixed by len.
	got := encodeNumberAsHex(10)
	want := append([]byte{2}, []byte("0A")...)
	if !bytes.Equal(got, want) {
		t.Errorf("encodeNumberAsHex(10) = %v, want %v", got, want)
	}
	// entry 0 -> "00"
	if got := encodeNumberAsHex(0); !bytes.Equal(got, append([]byte{2}, []byte("00")...)) {
		t.Errorf("encodeNumberAsHex(0) = %v", got)
	}
	// entry 255 -> "FF"
	if got := encodeNumberAsHex(255); string(got[1:]) != "FF" {
		t.Errorf("encodeNumberAsHex(255) name = %q", got[1:])
	}
}

func TestEncodeTBS(t *testing.T) {
	// No extras: just the forward varint of (val<<3 | flags).
	got := encodeTBS(1, 0b010, 0, 0, 0, false, false, false)
	if !bytes.Equal(got, encVarForward((1<<3)|0b010)) {
		t.Errorf("encodeTBS base mismatch: %v", got)
	}
	// With all extras present.
	got = encodeTBS(2, 0b111, 5, 3, 7, true, true, true)
	if len(got) < 3 {
		t.Errorf("encodeTBS with extras too short: %v", got)
	}
}

func TestFlisRecord(t *testing.T) {
	r := flisRecord()
	if len(r) != 36 {
		t.Fatalf("flisRecord len = %d, want 36", len(r))
	}
	if string(r[:4]) != "FLIS" {
		t.Errorf("flisRecord magic = %q", r[:4])
	}
}

func TestFcisRecord(t *testing.T) {
	r := fcisRecord(1000)
	if len(r) != 44 {
		t.Fatalf("fcisRecord len = %d, want 44", len(r))
	}
	if string(r[:4]) != "FCIS" {
		t.Errorf("fcisRecord magic = %q", r[:4])
	}
	// text length stored big-endian at offset 20
	if r[20] != 0 || r[21] != 0 || r[22] != 0x03 || r[23] != 0xe8 {
		t.Errorf("fcisRecord textLen bytes = %v", r[20:24])
	}
}

func TestGeneratePalmDocHeader(t *testing.T) {
	h := generatePalmDocHeader(5000, 2, false)
	if len(h) != palmDocHeaderSize {
		t.Fatalf("header len = %d", len(h))
	}
	if h[1] != 1 { // compression = none (1)
		t.Errorf("uncompressed compression byte = %d, want 1", h[1])
	}
	h = generatePalmDocHeader(5000, 2, true)
	if h[1] != 2 { // PalmDoc
		t.Errorf("compressed compression byte = %d, want 2", h[1])
	}
}

func TestGenerateExthHeader(t *testing.T) {
	h := generateExthHeader("Jane Doe")
	if string(h[:4]) != "EXTH" {
		t.Errorf("EXTH magic = %q", h[:4])
	}
	if len(h)%4 != 0 {
		t.Errorf("EXTH not padded to 4 bytes: len %d", len(h))
	}
	if !bytes.Contains(h, []byte("Jane Doe")) {
		t.Errorf("author not embedded")
	}
}

func TestGeneratePalmDatabaseHeader(t *testing.T) {
	recs := [][]byte{{1, 2, 3}, {4, 5}}
	h := generatePalmDatabaseHeader("Title", recs)
	if string(h[60:64]) != "BOOK" {
		t.Errorf("type = %q", h[60:64])
	}
	if string(h[64:68]) != "MOBI" {
		t.Errorf("creator = %q", h[64:68])
	}
	// number_of_records at offset 76 big-endian
	if h[76] != 0 || h[77] != 2 {
		t.Errorf("num records = %v, want 2", h[76:78])
	}
}

func TestGenerateMobiHeader(t *testing.T) {
	h := generateMobiHeader(palmDocHeaderSize, 40, 5, 3, 0, 0, false)
	if len(h) != mobiHeaderSize {
		t.Fatalf("mobi header len = %d", len(h))
	}
	if string(h[:4]) != "MOBI" {
		t.Errorf("MOBI magic = %q", h[:4])
	}
	// file version 5 when no TOC
	if h[23] != 5 {
		t.Errorf("file version without TOC = %d, want 5", h[23])
	}
	hTOC := generateMobiHeader(palmDocHeaderSize, 40, 5, 3, 3, 1, true)
	if hTOC[23] != 6 {
		t.Errorf("file version with TOC = %d, want 6", hTOC[23])
	}
}

func TestCompressPalmDocRoundTrip(t *testing.T) {
	inputs := []string{
		"",
		"hello world hello world hello world",
		strings.Repeat("abcdefghij", 50),
		"tabs\tand\nnewlines and high\x80bytes\xff here",
		"a b c d e f g the quick brown fox the quick brown fox",
	}
	for _, in := range inputs {
		out := compressPalmDoc([]byte(in))
		if dec := decompressPalmDoc(out); dec != in {
			t.Errorf("round trip failed for %q: got %q", in, dec)
		}
	}
}

// decompressPalmDoc is a reference PalmDoc decompressor used to verify
// compressPalmDoc produces valid output.
func decompressPalmDoc(data []byte) string {
	var out []byte
	i := 0
	for i < len(data) {
		c := data[i]
		i++
		switch {
		case c == 0:
			out = append(out, c)
		case c <= 8:
			// literal run of c bytes
			for j := byte(0); j < c && i < len(data); j++ {
				out = append(out, data[i])
				i++
			}
		case c < 0x80:
			out = append(out, c)
		case c >= 0xc0:
			// space + char
			out = append(out, ' ', c^0x80)
		default:
			// 0x80..0xbf: LZ77 back reference (2 bytes)
			if i >= len(data) {
				break
			}
			code := (int(c) << 8) | int(data[i])
			i++
			dist := (code >> 3) & 0x7ff
			length := (code & 0x7) + 3
			start := len(out) - dist
			for j := 0; j < length; j++ {
				out = append(out, out[start+j])
			}
		}
	}
	return string(out)
}

func TestWriteNoTOC(t *testing.T) {
	data, err := mustWrite(t, Book{Title: "My Book", Author: "Me", Content: "<html><body><p>Hello</p></body></html>"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("empty output")
	}
	if !bytes.Contains(data, []byte("BOOKMOBI")) {
		t.Errorf("missing BOOKMOBI signature")
	}
}

func TestWriteEmptyContent(t *testing.T) {
	data, err := Write(Book{Title: "Empty", Content: ""}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("empty output for empty content")
	}
}

func TestWriteWithTOCAndImages(t *testing.T) {
	// Build content large enough to span multiple 4096-byte records so the TBS
	// builder handles starts/completes/ends/spanner cases.
	var sb strings.Builder
	sb.WriteString("<html><body>")
	sb.WriteString(`<a name="inkfeed-toc-0"></a><h2>Chapter 1</h2>`)
	sb.WriteString(strings.Repeat("<p>content one</p>", 400))
	sb.WriteString(`<a name="inkfeed-toc-1"></a><h2>Chapter 2</h2>`)
	sb.WriteString(strings.Repeat("<p>content two</p>", 400))
	sb.WriteString("</body></html>")
	html := sb.String()

	toc := []TOCEntry{
		{Offset: strings.Index(html, `inkfeed-toc-0`), Label: "Chapter 1"},
		{Offset: strings.Index(html, `inkfeed-toc-1`), Label: "Chapter 2"},
	}
	img := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 1, 2, 3}
	data, err := Write(Book{Title: "TOC Book", Author: "A", Content: html, TOC: toc}, [][]byte{img})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("empty output")
	}
}

func TestBuildNCXRecords(t *testing.T) {
	toc := []TOCEntry{
		{Offset: 0, Label: "First"},
		{Offset: 100, Label: strings.Repeat("x", maxNCXLabelBytes+50)}, // exercises truncation
	}
	recs := buildNCXRecords(toc, 500)
	if len(recs) != 3 {
		t.Fatalf("expected 3 NCX records, got %d", len(recs))
	}
	if string(recs[0][:4]) != "INDX" {
		t.Errorf("master record magic = %q", recs[0][:4])
	}
	if string(recs[1][:4]) != "INDX" {
		t.Errorf("data record magic = %q", recs[1][:4])
	}
}

func TestBuildBookTBS(t *testing.T) {
	// Two entries; small text so single record -> completes case.
	toc := []TOCEntry{{Offset: 0, Label: "A"}, {Offset: 10, Label: "B"}}
	tbs := buildBookTBS(toc, 20, 1)
	if len(tbs) != 1 {
		t.Fatalf("expected 1 record TBS, got %d", len(tbs))
	}
	// Spanner case: an entry spanning across a record boundary.
	toc = []TOCEntry{{Offset: 0, Label: "A"}}
	tbs = buildBookTBS(toc, 9000, 3) // entry 0 spans records 1 and 2
	if len(tbs) != 3 {
		t.Fatalf("expected 3 record TBS, got %d", len(tbs))
	}
}

func mustWrite(t *testing.T, b Book, imgs [][]byte) ([]byte, error) {
	t.Helper()
	return Write(b, imgs)
}
