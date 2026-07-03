// Package mobi generates Mobipocket (.mobi) files.
// This is a Go port of mobiWriter.js.
package mobi

import (
	"encoding/binary"
	"time"
)

const (
	mobiHeaderSize    = 232
	palmDocHeaderSize = 16
	palmDbHeaderSize  = 78
	recordInfoSize    = 8
	indxHeaderLen     = 192

	exthAuthor              = 100
	exthCreatorSoftware     = 204
	exthCreatorMajorVersion = 205
	exthCreatorMinorVersion = 206
	exthCreatorBuildNumber  = 207
)

// Book holds the content and metadata for a MOBI file.
type Book struct {
	Title   string
	Author  string
	Content string     // HTML content (UTF-8)
	TOC     []TOCEntry // optional navigation points for the NCX table of contents
}

// Write generates a MOBI file and returns the raw bytes.
// imageRecords holds raw image bytes (JPEG/PNG/GIF) to embed; may be nil.
func Write(book Book, imageRecords [][]byte) ([]byte, error) {
	htmlBytes := []byte(book.Content)

	// Chunk HTML into 4096-byte records, each with a null terminator.
	var textRecords [][]byte
	for i := 0; i < len(htmlBytes); i += 4096 {
		end := i + 4096
		if end > len(htmlBytes) {
			end = len(htmlBytes)
		}
		chunk := make([]byte, end-i+1)
		copy(chunk, htmlBytes[i:end])
		chunk[len(chunk)-1] = 0
		textRecords = append(textRecords, chunk)
	}
	if len(textRecords) == 0 {
		textRecords = append(textRecords, []byte{0})
	}

	// Build the NCX table-of-contents index records (master INDX + data INDX +
	// CNCX). These sit between the text and image records; the MOBI header then
	// points at the master INDX record so Kindle exposes a real TOC.
	var ncxRecords [][]byte
	if len(book.TOC) > 0 {
		ncxRecords = buildNCXRecords(book.TOC, len(htmlBytes))
	}

	exthData := generateExthHeader(book.Author)
	palmDocData := generatePalmDocHeader(len(htmlBytes), len(textRecords))
	titleBytes := []byte(book.Title)
	mobiData := generateMobiHeader(palmDocHeaderSize, len(exthData), len(titleBytes), len(textRecords), len(ncxRecords), len(imageRecords))

	// Record 0: PalmDocHeader + MobiHeader + ExthHeader + padded title
	record0 := concat(palmDocData, mobiData, exthData)
	titlePadding := fourBytesPadding(len(titleBytes) + 2)
	record0 = append(record0, titleBytes...)
	record0 = append(record0, make([]byte, titlePadding+2)...)

	// Records: header, text, NCX index, images, EOF
	records := [][]byte{record0}
	records = append(records, textRecords...)
	records = append(records, ncxRecords...)
	records = append(records, imageRecords...)
	records = append(records, []byte{0xe9, 0x8e, 0x0d, 0x0a}) // EOF record

	palmDbData := generatePalmDatabaseHeader(book.Title, records)

	result := palmDbData
	for _, rec := range records {
		result = append(result, rec...)
	}
	return result, nil
}

func fourBytesPadding(size int) int {
	p := 4 - (size % 4)
	if p == 4 {
		return 0
	}
	return p
}

func concat(slices ...[]byte) []byte {
	var result []byte
	for _, s := range slices {
		result = append(result, s...)
	}
	return result
}

// generateExthHeader builds the EXTH header bytes.
// The EXTH length field stores the pre-padding size (matching JS behaviour).
func generateExthHeader(author string) []byte {
	type exthRecord struct {
		typ  uint32
		data []byte
	}

	creatorSoftware := make([]byte, 4)
	binary.BigEndian.PutUint32(creatorSoftware, 201)

	creatorMajor := make([]byte, 4)
	binary.BigEndian.PutUint32(creatorMajor, 1)

	creatorMinor := make([]byte, 4)
	binary.BigEndian.PutUint32(creatorMinor, 2)

	creatorBuild := make([]byte, 4)
	binary.BigEndian.PutUint32(creatorBuild, 33307)

	records := []exthRecord{
		{exthAuthor, []byte(author)},
		{exthCreatorSoftware, creatorSoftware},
		{exthCreatorMajorVersion, creatorMajor},
		{exthCreatorMinorVersion, creatorMinor},
		{exthCreatorBuildNumber, creatorBuild},
	}

	// Pre-padding size = 12-byte EXTH header + all record sizes
	prePaddingSize := 12
	for _, r := range records {
		prePaddingSize += 8 + len(r.data)
	}

	header := make([]byte, 12)
	copy(header[0:4], "EXTH")
	binary.BigEndian.PutUint32(header[4:], uint32(prePaddingSize))
	binary.BigEndian.PutUint32(header[8:], uint32(len(records)))

	buf := header
	for _, r := range records {
		rh := make([]byte, 8)
		binary.BigEndian.PutUint32(rh[0:], r.typ)
		binary.BigEndian.PutUint32(rh[4:], uint32(8+len(r.data)))
		buf = append(buf, rh...)
		buf = append(buf, r.data...)
	}

	// Pad to 4-byte boundary
	buf = append(buf, make([]byte, fourBytesPadding(len(buf)))...)
	return buf
}

func generatePalmDocHeader(textSize, textRecordCount int) []byte {
	h := make([]byte, palmDocHeaderSize)
	binary.BigEndian.PutUint16(h[0:], 1)                              // compression: none
	binary.BigEndian.PutUint32(h[4:], uint32(textSize))               // text_length
	binary.BigEndian.PutUint16(h[8:], uint16(textRecordCount))        // text_record_count
	binary.BigEndian.PutUint16(h[10:], 4096)                          // text_max_record_size
	return h
}

// generateMobiHeader builds the 232-byte MOBI header.
// Fields with swapEndian=false in the JS source are written little-endian here.
// All 0xFFFFFFFF fields are identical in both endiannesses; only
// unknown_bytes_2=0x00000001 is materially little-endian.
func generateMobiHeader(palmDocLen, exthLen, titleLen, textRecordsCount, ncxRecordsCount, imageRecordsCount int) []byte {
	h := make([]byte, mobiHeaderSize)
	o := 0

	copy(h[o:], "MOBI")
	o += 4
	binary.BigEndian.PutUint32(h[o:], mobiHeaderSize) // header_length
	o += 4
	binary.BigEndian.PutUint32(h[o:], 2) // mobi_file_type
	o += 4
	binary.BigEndian.PutUint32(h[o:], 65001) // text_encoding: UTF-8
	o += 4
	binary.BigEndian.PutUint32(h[o:], 2596053606) // unique_id
	o += 4
	binary.BigEndian.PutUint32(h[o:], 5) // mobi_file_version
	o += 4

	// These fields use swapEndian=false (little-endian) in the JS source.
	// All happen to be 0xFFFFFFFF so endianness is irrelevant, but we match exactly.
	leFF := []int{o, o + 4, o + 8, o + 12} // orthographic, inflection, index_names, index_keys
	for _, off := range leFF {
		binary.LittleEndian.PutUint32(h[off:], 0xffffffff)
	}
	o += 16
	for i := 0; i < 6; i++ { // extra_index_0..5
		binary.LittleEndian.PutUint32(h[o:], 0xffffffff)
		o += 4
	}

	binary.BigEndian.PutUint32(h[o:], uint32(textRecordsCount+1)) // first_non_book_index
	o += 4

	bookTitleOffset := palmDocLen + mobiHeaderSize + exthLen
	binary.BigEndian.PutUint32(h[o:], uint32(bookTitleOffset)) // book_title_offset
	o += 4
	binary.BigEndian.PutUint32(h[o:], uint32(titleLen)) // book_title_length
	o += 4
	binary.BigEndian.PutUint32(h[o:], 1033) // locale: en-US
	o += 4
	o += 8 // dictionary_input/output_language: 0
	binary.BigEndian.PutUint32(h[o:], 6) // minimum_mobipocket_version
	o += 4
	binary.BigEndian.PutUint32(h[o:], uint32(textRecordsCount+1+ncxRecordsCount)) // first_image_index
	o += 4
	o += 16 // huffman fields: 0
	binary.BigEndian.PutUint32(h[o:], 80) // exth_flags
	o += 4
	o += 32 // unknown_bytes_0: zeroed

	binary.LittleEndian.PutUint32(h[o:], 0xffffffff) // drm_offset
	o += 4
	binary.LittleEndian.PutUint32(h[o:], 0xffffffff) // drm_count
	o += 4
	o += 8  // drm_size, drm_flags: 0
	o += 12 // unknown_bytes_1: zeroed

	binary.BigEndian.PutUint16(h[o:], 1) // first_content_record_number
	o += 2
	binary.BigEndian.PutUint16(h[o:], uint16(textRecordsCount+ncxRecordsCount+imageRecordsCount)) // last_content_record_number
	o += 2

	binary.LittleEndian.PutUint32(h[o:], 0x00000001) // unknown_bytes_2 (LE!)
	o += 4
	binary.LittleEndian.PutUint32(h[o:], 0xffffffff) // fcis_record_number
	o += 4
	binary.LittleEndian.PutUint32(h[o:], 0) // fcis_record_count
	o += 4
	binary.LittleEndian.PutUint32(h[o:], 0xffffffff) // flis_record_number
	o += 4
	binary.LittleEndian.PutUint32(h[o:], 0) // flis_record_count
	o += 4
	o += 8 // unknown_bytes_3: zeroed
	binary.LittleEndian.PutUint32(h[o:], 0xffffffff) // unknown_bytes_4
	o += 4
	o += 4 // unknown_bytes_5: zeroed
	binary.LittleEndian.PutUint32(h[o:], 0xffffffff) // unknown_bytes_6
	o += 4
	binary.LittleEndian.PutUint32(h[o:], 0xffffffff) // unknown_bytes_7
	o += 4
	o += 2 // unknown_bytes_8: zeroed
	binary.BigEndian.PutUint16(h[o:], 1) // traildata_flags
	o += 2
	// first_indx_record_number: points at the master NCX INDX record so Kindle
	// exposes a Table of Contents. 0xFFFFFFFF when there is no TOC.
	if ncxRecordsCount > 0 {
		binary.BigEndian.PutUint32(h[o:], uint32(textRecordsCount+1))
	} else {
		binary.LittleEndian.PutUint32(h[o:], 0xffffffff)
	}
	// o += 4  (last field)

	return h
}

func generatePalmDatabaseHeader(title string, records [][]byte) []byte {
	totalSize := palmDbHeaderSize + len(records)*recordInfoSize + 2
	h := make([]byte, palmDbHeaderSize)

	// name: 32 bytes, null-padded book title
	titleBytes := []byte(title)
	for i := 0; i < 32 && i < len(titleBytes); i++ {
		h[i] = titleBytes[i]
	}

	// timestamps: seconds since Palm epoch (Jan 1, 1904)
	palmEpochUnix := time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	ts := uint32(time.Now().Unix() - palmEpochUnix)
	binary.BigEndian.PutUint32(h[36:], ts) // creation_date
	binary.BigEndian.PutUint32(h[40:], ts) // modification_date

	copy(h[60:], "BOOK") // type
	copy(h[64:], "MOBI") // creator

	binary.BigEndian.PutUint16(h[76:], uint16(len(records))) // number_of_records

	result := make([]byte, 0, totalSize)
	result = append(result, h...)

	// Record info list: each entry is offset (4) + attributes (1) + unique_id (3)
	recordDataOffset := totalSize
	for _, rec := range records {
		ri := make([]byte, recordInfoSize)
		binary.BigEndian.PutUint32(ri[0:], uint32(recordDataOffset))
		result = append(result, ri...)
		recordDataOffset += len(rec)
	}

	result = append(result, 0, 0) // 2-byte gap
	return result
}
