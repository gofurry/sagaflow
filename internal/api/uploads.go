package api

import (
	"bufio"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v3"
)

const maxUploadSize = int64(512 << 20)

type multipartUpload struct {
	Header   *multipart.FileHeader
	File     multipart.File
	Reader   *bufio.Reader
	MIMEType string
}

func openMultipartUpload(c fiber.Ctx, field string) (multipartUpload, error) {
	header, err := c.FormFile(field)
	if err != nil {
		return multipartUpload{}, fiber.NewError(fiber.StatusBadRequest, field+" is required")
	}
	if header.Size < 0 || header.Size > maxUploadSize {
		return multipartUpload{}, fiber.NewError(fiber.StatusRequestEntityTooLarge, "file is too large")
	}
	file, err := header.Open()
	if err != nil {
		return multipartUpload{}, err
	}
	reader := bufio.NewReaderSize(file, 512)
	mimeType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if mimeType == "" || mimeType == "application/octet-stream" {
		sample, peekErr := reader.Peek(512)
		if peekErr != nil && !errors.Is(peekErr, io.EOF) && !errors.Is(peekErr, bufio.ErrBufferFull) {
			_ = file.Close()
			return multipartUpload{}, peekErr
		}
		mimeType = http.DetectContentType(sample)
	}
	return multipartUpload{Header: header, File: file, Reader: reader, MIMEType: mimeType}, nil
}
