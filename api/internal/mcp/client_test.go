package mcp

import "testing"

func TestNewClientTrimsBaseURL(t *testing.T) {
	client := NewClient(ClientConfig{BaseURL: "http://localhost:8080/api/v1/mcp///"})
	if client.baseURL != "http://localhost:8080/api/v1/mcp" {
		t.Fatalf("baseURL = %q", client.baseURL)
	}
}
