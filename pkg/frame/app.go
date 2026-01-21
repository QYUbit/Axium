package frame

import (
	"context"

	"github.com/QYUbit/Axium/pkg/transport"
)

type App struct {
	useUnreliable bool

	transport transport.Transport

	connService *connService
}

func NewApp(transport transport.Transport) *App {
	return &App{
		transport: transport,
	}
}

func (app *App) Start(ctx context.Context) error {
	for {
		p, err := app.transport.Accept(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				// TODO Log
			}
		}
		app.connService.Handle(ctx, p)
	}
}
