package api

import (
	"github.com/gofiber/fiber/v3"
)

func (s *Server) currentUser(c fiber.Ctx) error {
	current, err := principal(c)
	if err != nil {
		return err
	}
	return writeOK(c, fiber.Map{"user": current})
}

func (s *Server) updateProfile(c fiber.Ctx) error {
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	account, err := s.auth.UpdateProfile(c.Context(), req.Username, req.DisplayName)
	if err != nil {
		return err
	}
	return writeOK(c, account)
}

func (s *Server) changePassword(c fiber.Ctx) error {
	current, err := principal(c)
	if err != nil {
		return err
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	account, err := s.auth.ChangePassword(c.Context(), current, req.CurrentPassword, req.NewPassword)
	if err != nil {
		return err
	}
	return writeOK(c, account)
}
