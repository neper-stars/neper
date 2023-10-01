package tests

import "io"

type teeReadCloser struct {
	io.Reader
	io.Closer
}

func TeeReadCloser(r io.ReadCloser, w io.Writer) io.ReadCloser {
	return teeReadCloser{
		Reader: io.TeeReader(r, w),
		Closer: r,
	}
}
