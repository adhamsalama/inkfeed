package mobi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
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

func pad4(b []byte) []byte {
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

// buildNCXRecords builds the three PDB records that make up a flat MOBI NCX
// index: the master INDX record (with its TAGX tag table), the data INDX record
// (one entry per TOC point, plus an IDXT offset trailer), and the CNCX record
// (the label strings). The layout mirrors what calibre's MOBI 6 output writer
// produces for a single-level TOC. textLen is the total byte length of the HTML
// text stream, used to size the final entry's span.
func buildNCXRecords(toc []TOCEntry, textLen int) [][]byte {
	n := len(toc)
	width := len(strconv.Itoa(n - 1))
	if width < 2 {
		width = 2
	}

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
		name := fmt.Sprintf("%0*d", width, i)
		ent.WriteByte(byte(len(name)))
		ent.WriteString(name)
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

	lastName := fmt.Sprintf("%0*d", width, n-1)
	var te bytes.Buffer
	te.WriteByte(byte(len(lastName)))
	te.WriteString(lastName)
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
