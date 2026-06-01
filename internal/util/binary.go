package util

import (
	"bytes"
	"io"
	"os"
)

const SniffSize = 8 * 1024

var knownBinaryMagic = [][]byte{
	{0x7F, 'E', 'L', 'F'},
	{0xFE, 0xED, 0xFA, 0xCE},
	{0xFE, 0xED, 0xFA, 0xCF},
	{0xCE, 0xFA, 0xED, 0xFE},
	{0xCF, 0xFA, 0xED, 0xFE},
	{0xCA, 0xFE, 0xBA, 0xBE},
	{'M', 'Z'},
	{0x00, 'a', 's', 'm'},
	{0x21, 0x3C, 0x61, 0x72},
	{0x1F, 0x8B},
	{0x50, 0x4B, 0x03, 0x04},
	{0x1F, 0x9D},
	{0x89, 'P', 'N', 'G'},
	{0xFF, 0xD8, 0xFF},
	{'G', 'I', 'F', '8'},
	{'B', 'M'},
	{0x49, 0x49, 0x2A, 0x00},
	{0x4D, 0x4D, 0x00, 0x2A},
	{'%', 'P', 'D', 'F'},
}

// SniffAndRewind opens path, reads the first SniffSize bytes to detect
// binary content, then seeks back to the beginning. Returns the open file
// handle on success (caller must close it).
func SniffAndRewind(path string) (f *os.File, isBinary bool, err error) {
	f, err = os.Open(path)
	if err != nil {
		return nil, false, err
	}
	buf := make([]byte, SniffSize)
	n, rerr := io.ReadFull(f, buf)
	if rerr != nil && rerr != io.EOF && rerr != io.ErrUnexpectedEOF {
		f.Close()
		return nil, false, rerr
	}
	buf = buf[:n]
	for _, magic := range knownBinaryMagic {
		if bytes.HasPrefix(buf, magic) {
			f.Close()
			return nil, true, nil
		}
	}
	if bytes.IndexByte(buf, 0x00) >= 0 {
		f.Close()
		return nil, true, nil
	}
	if _, err := f.Seek(0, 0); err != nil {
		f.Close()
		return nil, false, err
	}
	return f, false, nil
}

func SniffBinary(path string) (isBinary bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, SniffSize)
	n, rerr := io.ReadFull(f, buf)
	if rerr != nil && rerr != io.EOF && rerr != io.ErrUnexpectedEOF {
		return false, rerr
	}
	buf = buf[:n]

	for _, magic := range knownBinaryMagic {
		if bytes.HasPrefix(buf, magic) {
			return true, nil
		}
	}

	if bytes.IndexByte(buf, 0x00) >= 0 {
		return true, nil
	}

	return false, nil
}
