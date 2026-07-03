package server

// riffChunk builds a RIFF/WEBP container holding a single chunk with the given
// FourCC and payload. Used to drive decodeWebP's manual RIFF walker (the simple
// x/image/webp decoder rejects these, so the walker's switch runs).
