package httpapi

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
)

type AccountProfileUpdateRequest struct {
	DisplayName string `json:"displayName"`
}

func (s Server) updateAccountProfile(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	var request AccountProfileUpdateRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	displayName, err := normalizeDisplayName(request.DisplayName)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	updated, err := s.store.UpdatePublicAlias(c.Context(), user.ID, displayName)
	if err != nil {
		return fiber.NewError(fiber.StatusConflict, err.Error())
	}
	return c.JSON(fiber.Map{"user": updated})
}

func normalizeDisplayName(value string) (string, error) {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	length := utf8.RuneCountInString(value)
	if length < 3 || length > 40 {
		return "", errors.New("display name must be between 3 and 40 characters")
	}
	runes := []rune(value)
	if !isDisplayNameLetterOrNumber(runes[0]) || !isDisplayNameLetterOrNumber(runes[len(runes)-1]) {
		return "", errors.New("display name must start and end with a letter or number")
	}
	for _, character := range runes {
		if isDisplayNameLetterOrNumber(character) || character == ' ' || strings.ContainsRune("._-'’", character) {
			continue
		}
		return "", errors.New("display name can only use letters, numbers, spaces, periods, underscores, apostrophes, and hyphens")
	}
	switch strings.ToLower(value) {
	case "admin", "administrator", "budol", "budol admin", "budol support", "budolph", "budolph admin", "budolph support", "moderator", "support", "anonymous trader":
		return "", errors.New("that display name is reserved")
	}
	return value, nil
}

func isDisplayNameLetterOrNumber(value rune) bool {
	return unicode.IsLetter(value) || unicode.IsNumber(value)
}
