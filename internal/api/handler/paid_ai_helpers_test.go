package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
)

// markPaidAI stands in for the paid-AI gate in a test that is about something else — a
// route's auth shape, a handler's own behaviour — and reads no plan. It marks the request
// exactly as the real gate does on a yes, so the binding hands out a client. A test about
// the gate itself mounts paidAIGate with a real (or fake) plan instead.
func markPaidAI(plan.Feature) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.SetUserContext(plan.WithPaidAI(c.UserContext()))
		return c.Next()
	}
}
