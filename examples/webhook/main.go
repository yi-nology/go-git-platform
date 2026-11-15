// Example: verify an inbound GitHub webhook signature using the unified
// runtime.DefaultWebhookRegistry. This makes no network calls.
package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/runtime"
)

func main() {
	secret := os.Getenv("WEBHOOK_SECRET")
	if secret == "" {
		log.Fatal("set WEBHOOK_SECRET to run this example")
	}

	body := []byte(`{"action":"opened"}`)
	req := httptest.NewRequest("POST", "/webhook", http.NoBody)
	// In a real handler the signature header is set by GitHub. Here we expect
	// the operator to supply it via X-Hub-Signature-256 for demonstration.
	req.Header.Set("X-Hub-Signature-256", os.Getenv("X_HUB_SIGNATURE_256"))

	if err := runtime.DefaultWebhookRegistry().Validate(
		model.PlatformGitHub, req, body, secret,
	); err != nil {
		log.Fatalf("webhook validation failed: %v", err)
	}
	fmt.Println("webhook signature is valid")
}
