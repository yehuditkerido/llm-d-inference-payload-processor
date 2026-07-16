/*
Copyright 2026 The llm-d Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package authgenerator

import (
	"testing"

	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/plugin"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/plugins/requesthandling/modelproviderresolver"
)

func TestAPIKeyAuthGenerator_DefaultBearerHeader(t *testing.T) {
	g := NewAPIKeyAuthGenerator()

	cs := plugin.NewCycleState()
	cs.Write(modelproviderresolver.ModelConfigKey, map[string]string{})

	extra, err := g.ExtractRequestData(cs, nil)
	if err != nil {
		t.Fatalf("ExtractRequestData() error = %v", err)
	}

	creds := map[string]string{
		"api-key":          "sk-test",
		APIKeyAuthHeaderName:  extra[APIKeyAuthHeaderName],
		APIKeyAuthValuePrefix: extra[APIKeyAuthValuePrefix],
	}

	headers, err := g.GenerateAuthHeaders(creds)
	if err != nil {
		t.Fatalf("GenerateAuthHeaders() error = %v", err)
	}

	want := "Bearer sk-test"
	if got := headers["Authorization"]; got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestAPIKeyAuthGenerator_CustomHeader(t *testing.T) {
	g := NewAPIKeyAuthGenerator()

	cs := plugin.NewCycleState()
	cs.Write(modelproviderresolver.ModelConfigKey, map[string]string{
		APIKeyAuthHeaderName:  "x-api-key",
		APIKeyAuthValuePrefix: "",
	})

	extra, err := g.ExtractRequestData(cs, nil)
	if err != nil {
		t.Fatalf("ExtractRequestData() error = %v", err)
	}

	creds := map[string]string{
		"api-key":          "sk-anthropic-key",
		APIKeyAuthHeaderName:  extra[APIKeyAuthHeaderName],
		APIKeyAuthValuePrefix: extra[APIKeyAuthValuePrefix],
	}

	headers, err := g.GenerateAuthHeaders(creds)
	if err != nil {
		t.Fatalf("GenerateAuthHeaders() error = %v", err)
	}

	if got := headers["x-api-key"]; got != "sk-anthropic-key" {
		t.Errorf("x-api-key = %q, want %q", got, "sk-anthropic-key")
	}
	if _, exists := headers["Authorization"]; exists {
		t.Error("Authorization should not be set when custom header is used")
	}
}

func TestAPIKeyAuthGenerator_CustomHeaderWithPrefix(t *testing.T) {
	g := NewAPIKeyAuthGenerator()

	cs := plugin.NewCycleState()
	cs.Write(modelproviderresolver.ModelConfigKey, map[string]string{
		APIKeyAuthHeaderName:  "X-Custom-Auth",
		APIKeyAuthValuePrefix: "Token ",
	})

	extra, err := g.ExtractRequestData(cs, nil)
	if err != nil {
		t.Fatalf("ExtractRequestData() error = %v", err)
	}

	creds := map[string]string{
		"api-key":          "my-token",
		APIKeyAuthHeaderName:  extra[APIKeyAuthHeaderName],
		APIKeyAuthValuePrefix: extra[APIKeyAuthValuePrefix],
	}

	headers, err := g.GenerateAuthHeaders(creds)
	if err != nil {
		t.Fatalf("GenerateAuthHeaders() error = %v", err)
	}

	if got := headers["X-Custom-Auth"]; got != "Token my-token" {
		t.Errorf("X-Custom-Auth = %q, want %q", got, "Token my-token")
	}
}

func TestAPIKeyAuthGenerator_MissingAPIKey(t *testing.T) {
	g := NewAPIKeyAuthGenerator()

	creds := map[string]string{
		APIKeyAuthHeaderName:  "Authorization",
		APIKeyAuthValuePrefix: "Bearer ",
	}

	_, err := g.GenerateAuthHeaders(creds)
	if err == nil {
		t.Fatal("expected error for missing api-key field")
	}
}

func TestAPIKeyAuthGenerator_MissingModelConfig(t *testing.T) {
	g := NewAPIKeyAuthGenerator()

	cs := plugin.NewCycleState()

	_, err := g.ExtractRequestData(cs, nil)
	if err == nil {
		t.Fatal("expected error for missing ModelConfigKey in CycleState")
	}
}
