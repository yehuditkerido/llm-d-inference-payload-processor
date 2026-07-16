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

package apikeyinjection

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	errcommon "github.com/llm-d/llm-d-inference-payload-processor/pkg/common/error"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/plugin"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/requesthandling"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/plugins/requesthandling/apikeyinjection/authgenerator"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/plugins/requesthandling/modelproviderresolver"
)

func newTestPlugin(t *testing.T) (*APIKeyInjectionPlugin, *secretStore) {
	t.Helper()
	store := newSecretStore()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "my-creds"},
		Data: map[string][]byte{
			"api-key": []byte("sk-test-key-123"),
		},
	}
	if err := store.addOrUpdate("default", "my-creds", secret); err != nil {
		t.Fatalf("failed to seed store: %v", err)
	}

	p := &APIKeyInjectionPlugin{
		typedName: plugin.TypedName{Type: PluginType, Name: "test"},
		generator: authgenerator.NewAPIKeyAuthGenerator(),
		store:     store,
	}

	return p, store
}

func TestAPIKeyInjectionPlugin_TypedName(t *testing.T) {
	p := &APIKeyInjectionPlugin{
		typedName: plugin.TypedName{Type: PluginType, Name: "test-plugin"},
	}

	got := p.TypedName()
	if got.Type != PluginType {
		t.Errorf("TypedName().Type = %s, want %s", got.Type, PluginType)
	}
	if got.Name != "test-plugin" {
		t.Errorf("TypedName().Name = %s, want %s", got.Name, "test-plugin")
	}
}

func TestAPIKeyInjectionPlugin_WithName(t *testing.T) {
	p := &APIKeyInjectionPlugin{
		typedName: plugin.TypedName{Type: PluginType, Name: "original"},
	}

	p = p.WithName("renamed")

	if got := p.TypedName().Name; got != "renamed" {
		t.Errorf("Name after WithName = %s, want %s", got, "renamed")
	}
	if got := p.TypedName().Type; got != PluginType {
		t.Errorf("Type should be unchanged = %s, want %s", got, PluginType)
	}
}

func TestProcessRequest_NoCredsRef_SkipSilently(t *testing.T) {
	p, _ := newTestPlugin(t)
	cs := plugin.NewCycleState()
	req := requesthandling.NewInferenceRequest()

	err := p.ProcessRequest(context.Background(), cs, req)
	if err != nil {
		t.Fatalf("expected nil error when no credential ref, got %v", err)
	}
	if len(req.MutatedHeaders()) != 0 {
		t.Errorf("expected no mutated headers, got %v", req.MutatedHeaders())
	}
}

func TestProcessRequest_EmptyCredsRefName_SkipSilently(t *testing.T) {
	p, _ := newTestPlugin(t)
	cs := plugin.NewCycleState()
	cs.Write(modelproviderresolver.CredsRefName, "")
	req := requesthandling.NewInferenceRequest()

	err := p.ProcessRequest(context.Background(), cs, req)
	if err != nil {
		t.Fatalf("expected nil error for empty creds ref name, got %v", err)
	}
}

func TestProcessRequest_HappyPath(t *testing.T) {
	p, _ := newTestPlugin(t)

	cs := plugin.NewCycleState()
	cs.Write(modelproviderresolver.CredsRefName, "my-creds")
	cs.Write(modelproviderresolver.CredsRefNamespace, "default")
	cs.Write(modelproviderresolver.ModelConfigKey, map[string]string{})

	req := requesthandling.NewInferenceRequest()

	err := p.ProcessRequest(context.Background(), cs, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := req.Headers["Authorization"]
	want := "Bearer sk-test-key-123"
	if got != want {
		t.Errorf("Authorization header = %q, want %q", got, want)
	}
}

func TestProcessRequest_CustomHeader(t *testing.T) {
	p, _ := newTestPlugin(t)

	cs := plugin.NewCycleState()
	cs.Write(modelproviderresolver.CredsRefName, "my-creds")
	cs.Write(modelproviderresolver.CredsRefNamespace, "default")
	cs.Write(modelproviderresolver.ModelConfigKey, map[string]string{
		"authHeaderName":  "x-api-key",
		"authValuePrefix": "",
	})

	req := requesthandling.NewInferenceRequest()

	err := p.ProcessRequest(context.Background(), cs, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := req.Headers["x-api-key"]; got != "sk-test-key-123" {
		t.Errorf("x-api-key header = %q, want %q", got, "sk-test-key-123")
	}
	if _, exists := req.Headers["Authorization"]; exists {
		t.Error("Authorization header should not be set when custom header is configured")
	}
}

func TestProcessRequest_MissingCredentialRefNamespace(t *testing.T) {
	p, _ := newTestPlugin(t)

	cs := plugin.NewCycleState()
	cs.Write(modelproviderresolver.CredsRefName, "my-creds")

	req := requesthandling.NewInferenceRequest()

	err := p.ProcessRequest(context.Background(), cs, req)
	if err == nil {
		t.Fatal("expected error for missing credential ref namespace")
	}
	if errcommon.CanonicalCode(err) != errcommon.Internal {
		t.Errorf("expected Internal error code, got %s", errcommon.CanonicalCode(err))
	}
}

func TestProcessRequest_CredentialsNotInStore(t *testing.T) {
	p, _ := newTestPlugin(t)

	cs := plugin.NewCycleState()
	cs.Write(modelproviderresolver.CredsRefName, "nonexistent")
	cs.Write(modelproviderresolver.CredsRefNamespace, "default")

	req := requesthandling.NewInferenceRequest()

	err := p.ProcessRequest(context.Background(), cs, req)
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err.Error())
	}
}

func TestProcessRequest_MutatedHeaders(t *testing.T) {
	p, _ := newTestPlugin(t)

	cs := plugin.NewCycleState()
	cs.Write(modelproviderresolver.CredsRefName, "my-creds")
	cs.Write(modelproviderresolver.CredsRefNamespace, "default")
	cs.Write(modelproviderresolver.ModelConfigKey, map[string]string{})

	req := requesthandling.NewInferenceRequest()

	if err := p.ProcessRequest(context.Background(), cs, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mutated := req.MutatedHeaders()
	if got, ok := mutated["Authorization"]; !ok || got != "Bearer sk-test-key-123" {
		t.Errorf("MutatedHeaders[Authorization] = %q, %v; want %q, true", got, ok, "Bearer sk-test-key-123")
	}
}

func TestProcessRequest_WrongCycleStateType(t *testing.T) {
	p, _ := newTestPlugin(t)

	cs := plugin.NewCycleState()
	cs.Write(modelproviderresolver.CredsRefName, 42) // wrong type: int instead of string

	req := requesthandling.NewInferenceRequest()

	err := p.ProcessRequest(context.Background(), cs, req)
	if err == nil {
		t.Fatal("expected error for wrong CycleState value type")
	}
	if !strings.Contains(err.Error(), "failed to read credential ref") {
		t.Errorf("expected 'failed to read credential ref' in error, got %q", err.Error())
	}
}
