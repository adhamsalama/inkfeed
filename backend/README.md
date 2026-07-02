# Inkfeed Backend

Go REST API server for Inkfeed (feed parsing, article extraction, EPUB/MOBI
generation, email delivery). See [CLAUDE.md](./CLAUDE.md) for architecture and
commands.

## TODO

- **Add JFIF header to pass-through JPEGs in the MOBI path.** WebP- and
  PNG-converted images are re-encoded and get a JFIF APP0 header injected
  (`insertJFIFHeader` in `download.go`), which Kindle's inline renderer
  requires. But JPEGs downloaded from the origin are embedded verbatim, so a
  headerless JPEG (SOI straight into a quantization table, `FF D8 FF DB` — as
  served by some optimizers, e.g. Quanta Magazine) won't render inline. Fix:
  run pass-through JPEGs through `insertJFIFHeader` too, but only when an
  `APPn` marker is absent (byte after SOI is not `FF E0`–`FF EF`), so images
  that already render stay byte-for-byte unchanged.
