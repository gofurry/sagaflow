package service

import "errors"

var (
	ErrAuthNotInitialized        = errors.New("auth not initialized")
	ErrCredentialEncryption      = errors.New("credential encryption is not configured")
	ErrInvalidCredentials        = errors.New("invalid credentials")
	ErrForbidden                 = errors.New("forbidden")
	ErrInvalidInput              = errors.New("invalid input")
	ErrNotFound                  = errors.New("not found")
	ErrProviderCredentialMissing = errors.New("provider credential missing")
	ErrProviderRequest           = errors.New("provider request failed")
	ErrProviderUnavailable       = errors.New("provider unavailable")
	ErrStorageUnavailable        = errors.New("storage unavailable")
	ErrUnauthorized              = errors.New("unauthorized")
)
