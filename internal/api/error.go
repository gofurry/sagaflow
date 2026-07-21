package api

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	platformstorage "github.com/gofurry/sagaflow/internal/platform/storage"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/gofurry/sagaflow/internal/store/db"
	"go.uber.org/zap"
)

func errorHandler(log *zap.Logger) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		status := fiber.StatusInternalServerError
		code := "internal_error"
		message := "internal server error"
		var fiberErr *fiber.Error
		if errors.As(err, &fiberErr) {
			status = fiberErr.Code
			message = fiberErr.Message
			code = "request_error"
		} else if errors.Is(err, service.ErrUnauthorized) {
			status = fiber.StatusUnauthorized
			code = "unauthorized"
			message = "unauthorized"
		} else if errors.Is(err, service.ErrInvalidCredentials) {
			status = fiber.StatusUnauthorized
			code = "invalid_credentials"
			message = "invalid credentials"
		} else if errors.Is(err, service.ErrForbidden) {
			status = fiber.StatusForbidden
			code = "forbidden"
			message = "forbidden"
		} else if errors.Is(err, service.ErrAuthNotInitialized) {
			status = fiber.StatusConflict
			code = "auth_not_initialized"
			message = "auth is not initialized"
		} else if errors.Is(err, service.ErrNotFound) || errors.Is(err, db.ErrNotFound) {
			status = fiber.StatusNotFound
			code = "not_found"
			message = "resource not found"
		} else if errors.Is(err, db.ErrConflict) {
			status = fiber.StatusConflict
			code = "conflict"
			message = strings.TrimPrefix(err.Error(), db.ErrConflict.Error()+": ")
		} else if errors.Is(err, service.ErrInvalidInput) {
			status = fiber.StatusBadRequest
			code = "invalid_input"
			message = strings.TrimPrefix(err.Error(), service.ErrInvalidInput.Error()+": ")
		} else if errors.Is(err, service.ErrProviderCredentialMissing) || errors.Is(err, service.ErrCredentialEncryption) {
			status = fiber.StatusConflict
			code = "provider_not_ready"
			message = err.Error()
		} else if errors.Is(err, platformstorage.ErrStorageNotConfigured) {
			status = fiber.StatusConflict
			code = "storage_not_configured"
			message = "请先在管理设置中添加并启用一个默认存储连接"
		} else if errors.Is(err, service.ErrProviderUnavailable) {
			status = fiber.StatusServiceUnavailable
			code = "provider_unavailable"
			message = strings.TrimPrefix(err.Error(), service.ErrProviderUnavailable.Error()+": ")
		} else if errors.Is(err, service.ErrStorageUnavailable) {
			status = fiber.StatusServiceUnavailable
			code = "storage_unavailable"
			message = strings.TrimPrefix(err.Error(), service.ErrStorageUnavailable.Error()+": ")
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			status, code, message = fiber.StatusConflict, "conflict", "resource already exists"
		} else if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
			status, code, message = fiber.StatusBadRequest, "invalid_reference", "referenced resource does not exist"
		} else if strings.Contains(err.Error(), "CHECK constraint failed") || strings.Contains(err.Error(), "NOT NULL constraint failed") {
			status, code, message = fiber.StatusBadRequest, "invalid_input", "invalid data"
		}
		if status >= 500 {
			log.Error("request failed", zap.String("method", c.Method()), zap.String("path", c.Path()), zap.Error(err))
		}
		return c.Status(status).JSON(responseEnvelope{Error: &errorBody{Code: code, Message: message}})
	}
}
