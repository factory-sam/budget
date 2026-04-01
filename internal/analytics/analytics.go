package analytics

import (
	"crypto/sha256"
	"fmt"
	"os"
	"runtime"
	"sync"

	"github.com/posthog/posthog-go"
)

const (
	defaultEndpoint = "https://us.i.posthog.com"
	envOptOut       = "BUDGET_ANALYTICS_OPT_OUT"
)

func defaultAPIKey() string {
	// PostHog project token (public, client-side safe)
	return "phc_BkjaEesqRDVHbZpnEaAtUmPFAhs8PYyVPcVkpgD6xTqZ"
}

var (
	client   posthog.Client
	mu       sync.Mutex
	disabled bool
)

func Init() {
	mu.Lock()
	defer mu.Unlock()

	if os.Getenv(envOptOut) == "1" || os.Getenv(envOptOut) == "true" {
		disabled = true
		return
	}

	apiKey := os.Getenv("BUDGET_POSTHOG_KEY")
	if apiKey == "" {
		apiKey = defaultAPIKey()
	}
	endpoint := os.Getenv("BUDGET_POSTHOG_HOST")
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	var err error
	client, err = posthog.NewWithConfig(apiKey, posthog.Config{
		Endpoint: endpoint,
	})
	if err != nil {
		disabled = true
	}
}

func Close() {
	mu.Lock()
	defer mu.Unlock()
	if client != nil {
		client.Close()
		client = nil
	}
}

func Track(event string, props map[string]interface{}) {
	mu.Lock()
	c := client
	off := disabled
	mu.Unlock()

	if off || c == nil {
		return
	}

	p := posthog.NewProperties()
	p.Set("os", runtime.GOOS)
	p.Set("arch", runtime.GOARCH)
	for k, v := range props {
		p.Set(k, v)
	}

	if err := c.Enqueue(posthog.Capture{
		DistinctId: distinctID(),
		Event:      event,
		Properties: p,
	}); err != nil {
		return
	}
}

func distinctID() string {
	hostname, _ := os.Hostname()
	home, _ := os.UserHomeDir()
	raw := fmt.Sprintf("%s:%s", hostname, home)
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h[:8])
}
