package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/techbudol1/budol-api/internal/store"
)

var pilotIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9-]{16,80}$`)

var allowedPilotEvents = map[string]struct{}{
	"pilot_started":           {},
	"wallet_connected":        {},
	"public_trade_completed":  {},
	"private_trade_completed": {},
	"position_sold":           {},
	"public_claim_completed":  {},
	"private_claim_completed": {},
	"feedback_submitted":      {},
}

var allowedFeedbackCategories = map[string]struct{}{
	"bug": {}, "usability": {}, "privacy": {}, "performance": {}, "idea": {}, "other": {},
}

var allowedFeedbackAreas = map[string]struct{}{
	"onboarding": {}, "markets": {}, "trading": {}, "portfolio": {}, "claims": {}, "wallet": {}, "privacy": {}, "other": {},
}

var allowedDeviceClasses = map[string]struct{}{
	"mobile": {}, "tablet": {}, "desktop": {}, "unknown": {},
}

var allowedFeedbackStatuses = map[string]struct{}{
	"new": {}, "reviewed": {}, "resolved": {},
}

type pilotEventRequest struct {
	AnonymousID string `json:"anonymousId"`
	Event       string `json:"event"`
	DeviceClass string `json:"deviceClass"`
}

type pilotFeedbackRequest struct {
	AnonymousID string `json:"anonymousId"`
	Category    string `json:"category"`
	Area        string `json:"area"`
	DeviceClass string `json:"deviceClass"`
	Rating      int64  `json:"rating"`
	Message     string `json:"message"`
}

type pilotFeedbackUpdateRequest struct {
	Status    string `json:"status"`
	AdminNote string `json:"adminNote"`
}

func (s Server) recordPilotEvent(c *fiber.Ctx) error {
	var request pilotEventRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid event payload")
	}
	input, err := validatedPilotEvent(request)
	if err != nil {
		return err
	}
	if err := s.store.RecordPilotEvent(c.Context(), input); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to record pilot event")
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (s Server) createPilotFeedback(c *fiber.Ctx) error {
	var request pilotFeedbackRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid feedback payload")
	}
	input, err := validatedPilotFeedback(request)
	if err != nil {
		return err
	}
	feedback, err := s.store.CreatePilotFeedback(c.Context(), input)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save feedback")
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"feedback": feedback})
}

func (s Server) adminPilotOverview(c *fiber.Ctx) error {
	overview, err := s.store.PilotOverview(c.Context(), 250)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load pilot feedback")
	}
	return c.JSON(fiber.Map{"pilot": overview})
}

func (s Server) adminUpdatePilotFeedback(c *fiber.Ctx) error {
	var request pilotFeedbackUpdateRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid feedback update")
	}
	status := strings.ToLower(strings.TrimSpace(request.Status))
	if _, ok := allowedFeedbackStatuses[status]; !ok {
		return fiber.NewError(fiber.StatusBadRequest, "status must be new, reviewed, or resolved")
	}
	adminNote := strings.TrimSpace(request.AdminNote)
	if len([]rune(adminNote)) > 1000 {
		return fiber.NewError(fiber.StatusBadRequest, "admin note must be 1000 characters or fewer")
	}
	feedback, found, err := s.store.UpdatePilotFeedback(c.Context(), c.Params("id"), status, adminNote)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update feedback")
	}
	if !found {
		return fiber.NewError(fiber.StatusNotFound, "feedback not found")
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), adminActivity(c, s.adminActor(c), "update_pilot_feedback", "pilot_feedback", feedback.ID, "Pilot feedback updated", "Status changed to "+feedback.Status+"."))
	return c.JSON(fiber.Map{"feedback": feedback})
}

func validatedPilotEvent(request pilotEventRequest) (store.PilotEventInput, error) {
	anonymousID, err := hashPilotIdentifier(request.AnonymousID)
	if err != nil {
		return store.PilotEventInput{}, err
	}
	event := strings.ToLower(strings.TrimSpace(request.Event))
	if _, ok := allowedPilotEvents[event]; !ok {
		return store.PilotEventInput{}, fiber.NewError(fiber.StatusBadRequest, "unsupported pilot event")
	}
	deviceClass := normalizedAllowedValue(request.DeviceClass, allowedDeviceClasses, "unknown")
	return store.PilotEventInput{AnonymousID: anonymousID, Event: event, DeviceClass: deviceClass}, nil
}

func validatedPilotFeedback(request pilotFeedbackRequest) (store.PilotFeedbackInput, error) {
	anonymousID, err := hashPilotIdentifier(request.AnonymousID)
	if err != nil {
		return store.PilotFeedbackInput{}, err
	}
	if request.Rating < 1 || request.Rating > 5 {
		return store.PilotFeedbackInput{}, fiber.NewError(fiber.StatusBadRequest, "rating must be between 1 and 5")
	}
	message := strings.TrimSpace(request.Message)
	messageLength := len([]rune(message))
	if messageLength < 10 || messageLength > 1500 {
		return store.PilotFeedbackInput{}, fiber.NewError(fiber.StatusBadRequest, "feedback must be between 10 and 1500 characters")
	}
	category := strings.ToLower(strings.TrimSpace(request.Category))
	if _, ok := allowedFeedbackCategories[category]; !ok {
		return store.PilotFeedbackInput{}, fiber.NewError(fiber.StatusBadRequest, "unsupported feedback category")
	}
	area := strings.ToLower(strings.TrimSpace(request.Area))
	if _, ok := allowedFeedbackAreas[area]; !ok {
		return store.PilotFeedbackInput{}, fiber.NewError(fiber.StatusBadRequest, "unsupported feedback area")
	}
	deviceClass := normalizedAllowedValue(request.DeviceClass, allowedDeviceClasses, "unknown")
	return store.PilotFeedbackInput{AnonymousID: anonymousID, Category: category, Area: area, DeviceClass: deviceClass, Rating: request.Rating, Message: message}, nil
}

func hashPilotIdentifier(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if !pilotIdentifierPattern.MatchString(value) {
		return "", fiber.NewError(fiber.StatusBadRequest, "invalid anonymous pilot identifier")
	}
	digest := sha256.Sum256([]byte("budolph-pilot-v1:" + value))
	return hex.EncodeToString(digest[:]), nil
}

func normalizedAllowedValue(raw string, allowed map[string]struct{}, fallback string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := allowed[value]; ok {
		return value
	}
	return fallback
}
