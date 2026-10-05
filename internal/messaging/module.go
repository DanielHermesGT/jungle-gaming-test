package messaging

import (
	"context"

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
	LC       fx.Lifecycle
	Consumer *Consumer
	Publisher *Publisher
	Pending  *PendingRefWorker
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
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	})
}
