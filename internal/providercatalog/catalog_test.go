package providercatalog

import (
	"net/url"
	"testing"
)

func TestProviderLinksUseOfficialHTTPSPages(t *testing.T) {
	items := All()
	if len(items) != 8 {
		t.Fatalf("expected eight credential providers, got %d", len(items))
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, exists := seen[item.Code]; exists {
			t.Fatalf("duplicate provider code %q", item.Code)
		}
		seen[item.Code] = struct{}{}
		for _, raw := range []string{item.CredentialURL, item.DocsURL} {
			parsed, err := url.Parse(raw)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
				t.Fatalf("provider %s has invalid official URL %q", item.Code, raw)
			}
		}
	}
}
