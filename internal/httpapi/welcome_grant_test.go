package httpapi

import (
	"testing"
	"time"

	"github.com/techbudol1/budol-api/internal/store"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

func TestShouldRetryPendingTokenGrantWaitsForReconciliation(t *testing.T) {
	app := fiber.New()
	ctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(ctx)

	fresh := store.TokenGrant{
		Status:    "pending",
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if shouldRetryTokenGrant(ctx, nil, fresh) {
		t.Fatal("fresh pending grant should wait for on-chain reconciliation")
	}

	stale := fresh
	stale.UpdatedAt = time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339)
	if !shouldRetryTokenGrant(ctx, nil, stale) {
		t.Fatal("stale pending grant without a transaction ID should be retried")
	}
}
