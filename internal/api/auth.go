package api

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/google/uuid"
)

const principalLocal = "sagaflow.principal"

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) requireAuth(c fiber.Ctx) error {
	if s.auth == nil {
		return service.ErrUnauthorized
	}
	principal, err := s.auth.AuthenticateToken(c.Context(), c.Cookies(s.auth.CookieName()))
	if err != nil {
		return err
	}
	c.Locals(principalLocal, principal)
	return c.Next()
}

func principal(c fiber.Ctx) (service.Principal, error) {
	value, ok := c.Locals(principalLocal).(service.Principal)
	if !ok || value.AccountID == uuid.Nil {
		return service.Principal{}, service.ErrUnauthorized
	}
	return value, nil
}

func (s *Server) setupAccount(c fiber.Ctx) error {
	initialized, err := s.auth.Initialized(c.Context())
	if err != nil {
		return err
	}
	if initialized {
		return service.ErrForbidden
	}
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	account, err := s.auth.Initialize(c.Context(), req.Username, req.DisplayName, req.Password)
	if err != nil {
		return err
	}
	return writeCreated(c, account)
}

func (s *Server) authStatus(c fiber.Ctx) error {
	enabled := s.auth != nil && s.auth.Enabled()
	initialized := false
	authenticated := false
	var current *service.Principal
	if s.auth != nil {
		var err error
		initialized, err = s.auth.Initialized(c.Context())
		if err != nil {
			return err
		}
		if value, authErr := s.auth.AuthenticateToken(c.Context(), c.Cookies(s.auth.CookieName())); authErr == nil {
			authenticated = true
			current = &value
		}
	}
	return writeOK(c, fiber.Map{"enabled": enabled, "initialized": initialized, "authenticated": authenticated, "user": current})
}

func (s *Server) login(c fiber.Ctx) error {
	if s.auth == nil {
		return service.ErrUnauthorized
	}
	var req loginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	ip := c.IP()
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if !s.loginLimiter.Allow(ip, username) {
		c.Set(fiber.HeaderRetryAfter, "60")
		return fiber.NewError(fiber.StatusTooManyRequests, "too many login attempts; please try again later")
	}
	token, expires, current, err := s.auth.Login(c.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			s.loginLimiter.RecordFailure(ip, username)
		}
		return err
	}
	s.loginLimiter.RecordSuccess(ip, username)
	c.Cookie(&fiber.Cookie{Name: s.auth.CookieName(), Value: token, Path: "/", MaxAge: int(s.auth.TokenTTL().Seconds()), Expires: expires, SameSite: fiber.CookieSameSiteLaxMode, Secure: s.auth.CookieSecure(), HTTPOnly: true})
	return writeOK(c, fiber.Map{"ok": true, "expires_at": expires, "user": current})
}

func (s *Server) logout(c fiber.Ctx) error {
	if s.auth != nil {
		if value, err := s.auth.AuthenticateToken(c.Context(), c.Cookies(s.auth.CookieName())); err == nil {
			_ = s.auth.Logout(c.Context(), value)
		}
		c.Cookie(&fiber.Cookie{Name: s.auth.CookieName(), Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(0, 0), SameSite: fiber.CookieSameSiteLaxMode, Secure: s.auth.CookieSecure(), HTTPOnly: true})
	}
	return writeOK(c, fiber.Map{"ok": true})
}
