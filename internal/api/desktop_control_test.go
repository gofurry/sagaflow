package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofurry/sagaflow/internal/config"
	"github.com/gofurry/sagaflow/internal/runtimecontract"
)

func TestDesktopShutdownRequiresLoopbackAndToken(t *testing.T) {
	shutdown := make(chan struct{}, 1)
	application := New(Dependencies{
		Config: config.Default(), DesktopControlToken: "desktop-secret",
		Shutdown: func() { shutdown <- struct{}{} },
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer application.Shutdown()
	go func() { _ = application.Listener(listener) }()
	baseURL := "http://" + listener.Addr().String()

	unauthorized, err := http.NewRequest(http.MethodPost, baseURL+"/_desktop/shutdown", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(unauthorized)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", response.StatusCode)
	}

	authorized, err := http.NewRequest(http.MethodPost, baseURL+"/_desktop/shutdown", nil)
	if err != nil {
		t.Fatal(err)
	}
	authorized.Header.Set(runtimecontract.DesktopTokenHeader, "desktop-secret")
	response, err = http.DefaultClient.Do(authorized)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("expected accepted, got %d", response.StatusCode)
	}
	select {
	case <-shutdown:
	case <-time.After(time.Second):
		t.Fatal("shutdown callback was not invoked")
	}
}

func TestDesktopShutdownRouteIsAbsentForHeadlessCore(t *testing.T) {
	application := New(Dependencies{Config: config.Default()})
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/_desktop/shutdown", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	response, err := application.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected route to be absent, got %d", response.StatusCode)
	}
}
