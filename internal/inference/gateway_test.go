package inference_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gofurry/sagaflow/internal/inference"
)

type driverFunc func(context.Context, inference.Request, inference.EventSink) (inference.Result, error)

func (f driverFunc) Execute(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	return f(ctx, request, events)
}

func TestGatewayDispatchesRegisteredDriver(t *testing.T) {
	called := false
	gateway, err := inference.NewGateway(map[string]inference.Driver{
		"demo": driverFunc(func(_ context.Context, request inference.Request, _ inference.EventSink) (inference.Result, error) {
			called = request.Target.ID == "model-1"
			return inference.Result{Artifacts: []inference.Artifact{{MediaType: "text", Content: inference.BytesContent([]byte("ok"), "text/plain")}}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = gateway.Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "demo-local", AdapterCode: "DEMO"},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "model-1", Capability: inference.CapabilityText},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("registered driver was not called")
	}
}

func TestGatewayRejectsUnknownProvider(t *testing.T) {
	gateway, err := inference.NewGateway(map[string]inference.Driver{
		"demo": driverFunc(func(context.Context, inference.Request, inference.EventSink) (inference.Result, error) {
			return inference.Result{}, errors.New("should not be called")
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = gateway.Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "missing-local", AdapterCode: "missing"},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "model-1", Capability: inference.CapabilityText},
	}, nil)
	if !inference.IsKind(err, inference.ErrorInvalidRequest) {
		t.Fatalf("expected invalid request, got %v", err)
	}
}
