package httpapi

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/techbudol1/budol-api/internal/store"

	"github.com/gofiber/fiber/v2"
)

type MarketAlertRequest struct {
	PriceEnabled      bool   `json:"priceEnabled"`
	PriceDirection    string `json:"priceDirection"`
	PriceThreshold    int64  `json:"priceThreshold"`
	ClosingEnabled    bool   `json:"closingEnabled"`
	ResolutionEnabled bool   `json:"resolutionEnabled"`
}

func (s Server) marketAlert(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	alert, ok, err := s.store.GetMarketAlert(c.Context(), user.ID, c.Params("slug"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load market alert")
	}
	if !ok {
		return c.JSON(fiber.Map{"alert": fiber.Map{
			"slug":              strings.TrimSpace(c.Params("slug")),
			"enabled":           false,
			"priceEnabled":      false,
			"priceDirection":    "above",
			"priceThreshold":    50,
			"closingEnabled":    false,
			"resolutionEnabled": false,
		}})
	}
	return c.JSON(fiber.Map{"alert": alert})
}

func (s Server) updateMarketAlert(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	var request MarketAlertRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.PriceDirection = strings.ToLower(strings.TrimSpace(request.PriceDirection))
	if !request.PriceEnabled && !request.ClosingEnabled && !request.ResolutionEnabled {
		return fiber.NewError(fiber.StatusBadRequest, "enable at least one market alert")
	}
	if request.PriceEnabled {
		if request.PriceDirection != "above" && request.PriceDirection != "below" {
			return fiber.NewError(fiber.StatusBadRequest, "priceDirection must be above or below")
		}
		if request.PriceThreshold < 1 || request.PriceThreshold > 99 {
			return fiber.NewError(fiber.StatusBadRequest, "priceThreshold must be between 1 and 99")
		}
	} else {
		request.PriceDirection = "above"
		request.PriceThreshold = 50
	}
	alert, err := s.store.UpsertMarketAlert(c.Context(), user.ID, c.Params("slug"), store.MarketAlertInput{
		PriceEnabled:      request.PriceEnabled,
		PriceDirection:    request.PriceDirection,
		PriceThreshold:    request.PriceThreshold,
		ClosingEnabled:    request.ClosingEnabled,
		ResolutionEnabled: request.ResolutionEnabled,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"alert": alert})
}

func (s Server) deleteMarketAlert(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	if err := s.store.DeleteMarketAlert(c.Context(), user.ID, c.Params("slug")); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to disable market alerts")
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (s Server) startMarketAlertWorker(ctx context.Context) {
	process := func() {
		if _, err := s.store.ProcessDueMarketClosingAlerts(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
			log.Printf("market closing alert worker failed: %v", err)
		}
	}
	go func() {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			process()
		}

		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				process()
			}
		}
	}()
}
