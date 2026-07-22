package minimax

import (
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
)

type baseResponse struct {
	StatusCode int64  `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

func (r baseResponse) Error(provider string) error {
	if r.StatusCode == 0 {
		return nil
	}
	message := strings.TrimSpace(r.StatusMsg)
	if message == "" {
		message = "MiniMax request failed"
	}
	return inference.NewError(inference.ErrorProvider, provider, message, false, nil)
}
