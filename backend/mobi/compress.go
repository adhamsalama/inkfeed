package mobi

import "bytes"

// compressPalmDoc compresses a record with the PalmDoc (LZ77 + space/char)
// scheme, a faithful port of calibre's py_compress_doc. Kindle expects indexed
// MOBI content to be PalmDoc-compressed (compression type 2).
func compressPalmDoc(data []byte) []byte {
	var out []byte
	i := 0
	n := len(data)
	for i < n {
		// LZ77 back-reference: longest match (3..10 bytes) within 2047 bytes.
		if i > 10 && (n-i) > 10 {
			match := -1
			mlen := 0
			for j := 10; j > 2; j-- {
				idx := bytes.LastIndex(data[:i], data[i:i+j])
				if idx < 0 {
					continue
				}
				if i-idx <= 2047 {
					match = idx
					mlen = j
					break
				}
			}
			if match >= 0 {
				m := i - match
				code := 0x8000 + ((m << 3) & 0x3ff8) + (mlen - 3)
				out = append(out, byte(code>>8), byte(code&0xff))
				i += mlen
				continue
			}
		}

		ch := data[i]
		i++

		// Space followed by an ASCII letter packs into a single byte.
		if ch == ' ' && i+1 < n {
			nx := data[i]
			if nx >= 0x40 && nx < 0x80 {
				out = append(out, nx^0x80)
				i++
				continue
			}
		}

		if ch == 0 || (ch > 8 && ch < 0x80) {
			out = append(out, ch)
		} else {
			// Literal run of "binary" bytes prefixed by its length (1..8).
			binseq := []byte{ch}
			j := i
			for j < n && len(binseq) < 8 {
				c := data[j]
				if c == 0 || (c > 8 && c < 0x80) {
					break
				}
				binseq = append(binseq, c)
				j++
			}
			out = append(out, byte(len(binseq)))
			out = append(out, binseq...)
			i += len(binseq) - 1
		}
	}
	return out
}
