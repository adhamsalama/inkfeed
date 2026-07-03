package mobi

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"strings"
)

// TOCEntry is one navigation point in the MOBI NCX (table of contents).
// Offset is the byte offset of the target into the uncompressed HTML text
// stream (the same value used for filepos links); Label is the display title.
type TOCEntry struct {
	Offset int
	Label  string
}

// maxNCXLabelBytes caps a chapter label so its length always fits in a 2-byte
// forward-varint and the CNCX record stays small.
const maxNCXLabelBytes = 500

// encVarForward encodes n as a MOBI forward variable-length integer: base-128,
// most-significant group first, with the high bit set on the final byte.
func encVarForward(n int) []byte {
	if n <= 0 {
		return []byte{0x80}
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte(n & 0x7f)}, b...)
		n >>= 7
	}
	b[len(b)-1] |= 0x80
	return b
}

// encVarBackward encodes n as a MOBI backward variable-length integer: like
// encVarForward but with the high bit set on the FINAL byte, suitable for
// appending to a buffer (the reader scans it from the end).
func encVarBackward(n int) []byte {
	if n <= 0 {
		return []byte{0x80}
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte(n & 0x7f)}, b...)
		n >>= 7
	}
	b[len(b)-1] |= 0x80
	return b
}

// encodeTBS encodes val together with flag_size low bits of flags as a
// forward-varint, then appends the flagged extras in calibre's order:
// 0b010 → varint value, 0b100 → single count byte, 0b001 → varint value.
func encodeTBS(val int, flags int, val010, count100, val001 int, has010, has100, has001 bool) []byte {
	fv := (val << 3) | (flags & 0b111)
	ans := encVarForward(fv)
	if has010 {
		ans = append(ans, encVarForward(val010)...)
	}
	if has100 {
		ans = append(ans, byte(count100))
	}
	if has001 {
		ans = append(ans, encVarForward(val001)...)
	}
	return ans
}

// tbsNode is an index entry positioned in the text stream.
type tbsNode struct {
	offset, nextOffset, index int
}

// buildBookTBS computes the trailing byte sequence (TBS) content for each text
// record of a (non-periodical) book, porting calibre's
// Indexer.calculate_trailing_byte_sequences + TBS.book_tbs. Element r is the
// TBS content for text record r (may be empty). textLen is the uncompressed
// text length; numRecords is the number of 4096-byte text records.
func buildBookTBS(toc []TOCEntry, textLen, numRecords int) [][]byte {
	nodes := make([]tbsNode, len(toc))
	for i, e := range toc {
		next := textLen
		if i+1 < len(toc) {
			next = toc[i+1].Offset
		}
		nodes[i] = tbsNode{offset: e.Offset, nextOffset: next, index: i}
	}

	out := make([][]byte, numRecords)
	for r := 0; r < numRecords; r++ {
		offset := r * 4096
		next := offset + 4096
		var starts, completes, ends []tbsNode
		var spanner *tbsNode
		for k := range nodes {
			nd := nodes[k]
			if nd.offset >= next {
				break // sorted by offset, all remaining start later (flat: all same depth)
			}
			if nd.nextOffset <= offset {
				continue
			}
			if nd.offset >= offset {
				if nd.nextOffset <= next {
					completes = append(completes, nd)
				} else {
					starts = append(starts, nd)
				}
			} else if nd.nextOffset <= next {
				ends = append(ends, nd)
			} else {
				n := nodes[k]
				spanner = &n
			}
		}
		out[r] = bookTBSBytes(starts, completes, ends, spanner)
	}
	return out
}

func bookTBSBytes(starts, completes, ends []tbsNode, spanner *tbsNode) []byte {
	if spanner != nil {
		// flags 0b010|0b001, both values 0
		return encodeTBS(spanner.index, 0b011, 0, 0, 0, true, false, true)
	}
	if len(completes) == 0 &&
		((len(starts) == 1 && len(ends) == 0) || (len(ends) == 1 && len(starts) == 0)) {
		nd := ends
		if len(starts) > 0 {
			nd = starts
		}
		return encodeTBS(nd[0].index, 0b010, 0, 0, 0, true, false, false)
	}
	all := append(append(append([]tbsNode{}, starts...), completes...), ends...)
	if len(all) == 0 {
		return nil
	}
	min := all[0]
	for _, n := range all {
		if n.index < min.index {
			min = n
		}
	}
	// flags 0b010 (value 0) | 0b100 (count byte)
	return encodeTBS(min.index, 0b110, 0, len(all), 0, true, true, false)
}

func pad4(b []byte) []byte {
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

// encodeNumberAsHex encodes an index entry's ordinal the way kindlegen/calibre
// do: an uppercase hex string (zero-padded to even length) prefixed by its byte
// length. This is the entry "name" the reader keys on, so it must be hex, not
// decimal (e.g. entry 10 is "0A", not "10").
func encodeNumberAsHex(num int) []byte {
	s := strings.ToUpper(strconv.FormatInt(int64(num), 16))
	if len(s)%2 != 0 {
		s = "0" + s
	}
	return append([]byte{byte(len(s))}, []byte(s)...)
}

// flisRecord returns the fixed 36-byte FLIS record kindlegen/calibre emit.
func flisRecord() []byte {
	return []byte{
		'F', 'L', 'I', 'S', 0, 0, 0, 8, 0, 0x41, 0, 0, 0, 0, 0, 0,
		0xff, 0xff, 0xff, 0xff, 0, 1, 0, 3, 0, 0, 0, 3, 0, 0, 0, 1,
		0xff, 0xff, 0xff, 0xff,
	}
}

// fcisRecord returns the 44-byte FCIS record; textLen is the uncompressed text
// length, stored at offset 20 as in calibre's output.
func fcisRecord(textLen int) []byte {
	r := []byte{
		'F', 'C', 'I', 'S', 0, 0, 0, 0x14, 0, 0, 0, 0x10, 0, 0, 0, 1,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x20,
		0, 0, 0, 8, 0, 1, 0, 1, 0, 0, 0, 0,
	}
	binary.BigEndian.PutUint32(r[20:], uint32(textLen))
	return r
}

// buildNCXRecords builds the three PDB records that make up a flat MOBI NCX
// index: the master INDX record (with its TAGX tag table), the data INDX record
// (one entry per TOC point, plus an IDXT offset trailer), and the CNCX record
// (the label strings). The layout mirrors what calibre's MOBI 6 output writer
// produces for a single-level TOC. textLen is the total byte length of the HTML
// text stream, used to size the final entry's span.
func buildNCXRecords(toc []TOCEntry, textLen int) [][]byte {
	n := len(toc)

	// CNCX: each label prefixed by its forward-varint byte length.
	var cncx bytes.Buffer
	labelOff := make([]int, n)
	for i, e := range toc {
		labelOff[i] = cncx.Len()
		lbl := []byte(e.Label)
		if len(lbl) > maxNCXLabelBytes {
			lbl = lbl[:maxNCXLabelBytes]
		}
		cncx.Write(encVarForward(len(lbl)))
		cncx.Write(lbl)
	}
	cncxRec := pad4(cncx.Bytes())

	// Data INDX record entries. Each entry:
	//   [name-len][name][control=0x0f][offset][size][label-offset][depth]
	// where the four tag values are forward varints and control 0x0f selects
	// all four tags defined in the TAGX table.
	var ent bytes.Buffer
	entOffsets := make([]int, n)
	for i, e := range toc {
		entOffsets[i] = indxHeaderLen + ent.Len()
		ent.Write(encodeNumberAsHex(i))
		ent.WriteByte(0x0f)
		ent.Write(encVarForward(e.Offset))
		size := textLen - e.Offset
		if i+1 < n {
			size = toc[i+1].Offset - e.Offset
		}
		if size < 0 {
			size = 0
		}
		ent.Write(encVarForward(size))
		ent.Write(encVarForward(labelOff[i]))
		ent.Write(encVarForward(0)) // depth (flat TOC)
	}
	entBytes := pad4(ent.Bytes())
	dataIdxtOff := indxHeaderLen + len(entBytes)

	dataIdxt := []byte("IDXT")
	for i := 0; i < n; i++ {
		dataIdxt = append(dataIdxt, byte(entOffsets[i]>>8), byte(entOffsets[i]))
	}
	dataIdxt = pad4(dataIdxt)

	dataHdr := make([]byte, indxHeaderLen)
	copy(dataHdr, "INDX")
	binary.BigEndian.PutUint32(dataHdr[4:], indxHeaderLen)
	binary.BigEndian.PutUint32(dataHdr[12:], 1) // record type: data
	binary.BigEndian.PutUint32(dataHdr[20:], uint32(dataIdxtOff))
	binary.BigEndian.PutUint32(dataHdr[24:], uint32(n))
	binary.BigEndian.PutUint32(dataHdr[28:], 0xffffffff)
	binary.BigEndian.PutUint32(dataHdr[32:], 0xffffffff)
	dataRec := concat(dataHdr, entBytes, dataIdxt)

	// Master INDX record: header + TAGX tag table + a single trailing entry
	// naming the last ordinal in the data record + IDXT.
	tagx := []byte{
		'T', 'A', 'G', 'X',
		0, 0, 0, 0x20, // TAGX length = 32
		0, 0, 0, 1, // control byte count
		1, 1, 0x01, 0, // tag 1 (offset)
		2, 1, 0x02, 0, // tag 2 (size)
		3, 1, 0x04, 0, // tag 3 (label offset in CNCX)
		4, 1, 0x08, 0, // tag 4 (depth)
		0, 0, 0, 1, // end/control
	}

	var te bytes.Buffer
	te.Write(encodeNumberAsHex(n - 1))
	te.WriteByte(0x00)
	te.WriteByte(byte(n)) // count of entries in the data record
	teStart := indxHeaderLen + len(tagx)
	teBytes := pad4(te.Bytes())
	masterIdxtOff := teStart + len(teBytes)

	masterIdxt := []byte("IDXT")
	masterIdxt = append(masterIdxt, byte(teStart>>8), byte(teStart))
	masterIdxt = pad4(masterIdxt)

	masterHdr := make([]byte, indxHeaderLen)
	copy(masterHdr, "INDX")
	binary.BigEndian.PutUint32(masterHdr[4:], indxHeaderLen)
	binary.BigEndian.PutUint32(masterHdr[16:], 2) // index type
	binary.BigEndian.PutUint32(masterHdr[20:], uint32(masterIdxtOff))
	binary.BigEndian.PutUint32(masterHdr[24:], 1) // number of data index records
	binary.BigEndian.PutUint32(masterHdr[28:], 65001)
	binary.BigEndian.PutUint32(masterHdr[32:], 0xffffffff)
	binary.BigEndian.PutUint32(masterHdr[36:], uint32(n)) // number of index entries
	binary.BigEndian.PutUint32(masterHdr[52:], 1)         // number of CNCX blocks
	binary.BigEndian.PutUint32(masterHdr[180:], indxHeaderLen)
	masterRec := concat(masterHdr, tagx, teBytes, masterIdxt)

	return [][]byte{masterRec, dataRec, cncxRec}
}
