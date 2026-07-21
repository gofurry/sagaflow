package inference

import (
	"bytes"
	"context"
	"io"
)

type byteContent struct {
	data     []byte
	mimeType string
}

func BytesContent(data []byte, mimeType string) ContentSource {
	owned := append([]byte(nil), data...)
	return byteContent{data: owned, mimeType: mimeType}
}

func (c byteContent) Open(context.Context) (io.ReadCloser, ContentInfo, error) {
	return io.NopCloser(bytes.NewReader(c.data)), ContentInfo{MIMEType: c.mimeType, Size: int64(len(c.data))}, nil
}
