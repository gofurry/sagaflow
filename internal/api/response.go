package api

import "github.com/gofiber/fiber/v3"

type responseEnvelope struct {
	Data  any        `json:"data,omitempty"`
	Error *errorBody `json:"error,omitempty"`
}
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeOK(c fiber.Ctx, data any) error {
	return c.Status(fiber.StatusOK).JSON(responseEnvelope{Data: data})
}
func writeCreated(c fiber.Ctx, data any) error {
	return c.Status(fiber.StatusCreated).JSON(responseEnvelope{Data: data})
}
