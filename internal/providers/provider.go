package providers

import (
	"context"

	"panelofexperts/internal/model"
)

type AgentProvider interface {
	ID() model.ProviderID
	Detect(ctx context.Context) model.Capability
	Run(ctx context.Context, request model.Request, progress chan<- model.ProgressEvent) (model.Result, error)
}
