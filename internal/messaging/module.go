package messaging

import (
	"context"
	"time"

	"go.uber.org/fx"
)

// Module wires SQS client and background workers (consumer, publisher, pending-ref).
var Module = fx.Module("messaging",
	fx.Provide(
		NewClient,
		NewPublisher,
		NewConsumer,
		NewPendingRefWorker,
	),
	fx.Invoke(registerWorkers),
)

type workerParams struct {
	fx.In
	LC        fx.Lifecycle
	Consumer  *Consumer
	Publisher *Publisher
	Pending   *PendingRefWorker
}

func registerWorkers(p workerParams) {
	ctx, cancel := context.WithCancel(context.Background())
	p.LC.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go p.Consumer.Run(ctx)
			go p.Publisher.Run(ctx)
			go p.Pending.Run(ctx)
			return nil
		},
		OnStop: func(stopCtx context.Context) error {
			cancel()
			// Finish in-flight work within Fx stop deadline (or 25s).
			deadline := 25 * time.Second
			if dl, ok := stopCtx.Deadline(); ok {
				if rem := time.Until(dl); rem > 0 && rem < deadline {
					deadline = rem
				}
			}
			waitCtx, waitCancel := context.WithTimeout(context.Background(), deadline)
			defer waitCancel()
			_ = p.Consumer.Wait(waitCtx)
			_ = p.Publisher.Wait(waitCtx)
			_ = p.Pending.Wait(waitCtx)
			return nil
		},
	})
}
