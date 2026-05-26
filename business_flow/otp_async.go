package businessflow

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3/log"
)

const asyncOTPDeliveryTimeout = 30 * time.Second

func runAsyncOTPTask(ctx context.Context, action string, fn func(context.Context) error) {
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}

	go func() {
		asyncCtx, cancel := context.WithTimeout(baseCtx, asyncOTPDeliveryTimeout)
		defer cancel()

		defer func() {
			if recovered := recover(); recovered != nil {
				log.Errorf("%s panicked: %v", action, recovered)
			}
		}()

		if err := fn(asyncCtx); err != nil {
			log.Errorf("%s failed: %v", action, err)
		}
	}()
}
