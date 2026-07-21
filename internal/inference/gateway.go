package inference

import (
	"context"
	"fmt"
	"strings"
)

// Gateway is immutable after construction and safe for concurrent use.
type Gateway struct {
	drivers map[string]Driver
}

func NewGateway(drivers map[string]Driver) (*Gateway, error) {
	registered := make(map[string]Driver, len(drivers))
	for code, driver := range drivers {
		code = strings.ToLower(strings.TrimSpace(code))
		if code == "" || driver == nil {
			return nil, fmt.Errorf("inference driver code and implementation are required")
		}
		if _, exists := registered[code]; exists {
			return nil, fmt.Errorf("inference driver %q is registered more than once", code)
		}
		registered[code] = driver
	}
	if len(registered) == 0 {
		return nil, fmt.Errorf("at least one inference driver is required")
	}
	return &Gateway{drivers: registered}, nil
}

func (g *Gateway) Execute(ctx context.Context, request Request, events EventSink) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	provider := strings.ToLower(strings.TrimSpace(request.Runtime.ProviderCode))
	adapter := strings.ToLower(strings.TrimSpace(request.Runtime.AdapterCode))
	if adapter == "" {
		return Result{}, NewError(ErrorInvalidRequest, provider, "runtime adapter code is required", false, nil)
	}
	driver := g.drivers[adapter]
	if driver == nil {
		return Result{}, NewError(ErrorInvalidRequest, provider, fmt.Sprintf("no inference driver is registered for adapter %q", adapter), false, nil)
	}
	if request.Target.Kind != TargetModel && request.Target.Kind != TargetWorkflow {
		return Result{}, NewError(ErrorInvalidRequest, provider, "execution target kind must be model or workflow", false, nil)
	}
	if strings.TrimSpace(request.Target.ID) == "" || request.Target.Capability == "" {
		return Result{}, NewError(ErrorInvalidRequest, provider, "execution target id and capability are required", false, nil)
	}
	result, err := driver.Execute(ctx, request, events)
	if err != nil {
		return Result{}, err
	}
	if len(result.Artifacts) == 0 {
		return Result{}, NewError(ErrorInvalidOutput, provider, "provider returned no artifacts", false, nil)
	}
	for _, artifact := range result.Artifacts {
		if artifact.Content == nil || strings.TrimSpace(artifact.MediaType) == "" {
			return Result{}, NewError(ErrorInvalidOutput, provider, "provider returned an incomplete artifact", false, nil)
		}
	}
	return result, nil
}
