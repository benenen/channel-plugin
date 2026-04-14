package agent

import "context"

type Driver interface {
	Run(ctx context.Context, spec Spec, req Request) (Response, error)
}
