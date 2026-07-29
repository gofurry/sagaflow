package cli

import (
	"errors"
	"os/user"
	"testing"
)

func TestDefaultServiceUserUsesExecutingUser(t *testing.T) {
	for _, username := range []string{"root", "creator"} {
		if got := defaultServiceUser(&user.User{Username: username}, nil); got != username {
			t.Fatalf("defaultServiceUser(%q) = %q, want %q", username, got, username)
		}
	}
}

func TestDefaultServiceUserFallsBackWhenLookupFails(t *testing.T) {
	if got := defaultServiceUser(nil, errors.New("lookup failed")); got != "sagaflow" {
		t.Fatalf("defaultServiceUser() = %q, want sagaflow", got)
	}
}

func TestSystemdPathHasNoOuterQuotes(t *testing.T) {
	if got := systemdPath("/root/sagaflow/data"); got != "/root/sagaflow/data" {
		t.Fatalf("systemdPath() = %q, want an unquoted absolute path", got)
	}
}

func TestSystemdPathEscapesSpecialCharacters(t *testing.T) {
	if got := systemdPath("/srv/Saga Flow/100%"); got != `/srv/Saga\x20Flow/100%%` {
		t.Fatalf("systemdPath() = %q, want escaped systemd path", got)
	}
}
