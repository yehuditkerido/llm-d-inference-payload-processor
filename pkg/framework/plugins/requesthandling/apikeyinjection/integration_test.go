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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/plugin"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/requesthandling"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/plugins/requesthandling/apikeyinjection/authgenerator"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/plugins/requesthandling/modelproviderresolver"
)

// TestFullPipeline_ResolverThenInjection simulates the actual pipeline:
//
//	model-provider-resolver writes CycleState → apikey-injection reads it → headers injected.
//
// This validates that the two plugins are aligned on CycleState key names and value types.
func TestFullPipeline_ResolverThenInjection(t *testing.T) {
	store := newSecretStore()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "tenant-ns", Name: "openai-secret"},
		Data: map[string][]byte{
			"api-key": []byte("sk-prod-key-abc123"),
		},
	}
	if err := store.addOrUpdate("tenant-ns", "openai-secret", secret); err != nil {
		t.Fatalf("failed to seed secret store: %v", err)
	}

	p := &APIKeyInjectionPlugin{
		typedName: plugin.TypedName{Type: PluginType, Name: "test"},
		generator: authgenerator.NewAPIKeyAuthGenerator(),
		store:     store,
	}

	// Simulate what model-provider-resolver writes to CycleState
	// (copied from modelproviderresolver/plugin.go ProcessRequest lines 157-163)
	cs := plugin.NewCycleState()
	cs.Write(modelproviderresolver.ProviderKey, "openai")
	cs.Write(modelproviderresolver.ModelKey, "gpt-4-turbo-preview")
	cs.Write(modelproviderresolver.APIFormatKey, "openai-chat")
	cs.Write(modelproviderresolver.CredsRefName, "openai-secret")
	cs.Write(modelproviderresolver.CredsRefNamespace, "tenant-ns")
	cs.Write(modelproviderresolver.ModelConfigKey, map[string]string{"org": "my-org"})
	cs.Write(modelproviderresolver.InputAPIFormatKey, "openai-chat")

	request := requesthandling.NewInferenceRequest()
	request.Body["model"] = "gpt-4-turbo"

	err := p.ProcessRequest(context.Background(), cs, request)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	got := request.Headers["Authorization"]
	want := "Bearer sk-prod-key-abc123"
	if got != want {
		t.Errorf("Authorization header = %q, want %q", got, want)
	}

	mutated := request.MutatedHeaders()
	if _, ok := mutated["Authorization"]; !ok {
		t.Error("Authorization should appear in MutatedHeaders for ext-proc response")
	}
}

// TestFullPipeline_CustomProviderHeader tests custom header name from provider config
// (e.g. Anthropic-style x-api-key with no Bearer prefix).
func TestFullPipeline_CustomProviderHeader(t *testing.T) {
	store := newSecretStore()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "anthropic-secret"},
		Data: map[string][]byte{
			"api-key": []byte("sk-ant-key"),
		},
	}
	if err := store.addOrUpdate("ns", "anthropic-secret", secret); err != nil {
		t.Fatalf("failed to seed secret store: %v", err)
	}

	p := &APIKeyInjectionPlugin{
		typedName: plugin.TypedName{Type: PluginType, Name: "test"},
		generator: authgenerator.NewAPIKeyAuthGenerator(),
		store:     store,
	}

	cs := plugin.NewCycleState()
	cs.Write(modelproviderresolver.CredsRefName, "anthropic-secret")
	cs.Write(modelproviderresolver.CredsRefNamespace, "ns")
	cs.Write(modelproviderresolver.ModelConfigKey, map[string]string{
		"authHeaderName":  "x-api-key",
		"authValuePrefix": "",
	})

	request := requesthandling.NewInferenceRequest()

	err := p.ProcessRequest(context.Background(), cs, request)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if got := request.Headers["x-api-key"]; got != "sk-ant-key" {
		t.Errorf("x-api-key header = %q, want %q", got, "sk-ant-key")
	}
	if _, exists := request.Headers["Authorization"]; exists {
		t.Error("Authorization should not be set when custom header is used")
	}
}

// TestFullPipeline_InternalModel_NoInjection verifies that when model-provider-resolver
// does NOT write credential keys (internal model passthrough), apikey-injection is a no-op.
func TestFullPipeline_InternalModel_NoInjection(t *testing.T) {
	store := newSecretStore()
	p := &APIKeyInjectionPlugin{
		typedName: plugin.TypedName{Type: PluginType, Name: "test"},
		generator: authgenerator.NewAPIKeyAuthGenerator(),
		store:     store,
	}

	cs := plugin.NewCycleState()

	request := requesthandling.NewInferenceRequest()
	request.Body["model"] = "llama-3-70b"

	err := p.ProcessRequest(context.Background(), cs, request)
	if err != nil {
		t.Fatalf("ProcessRequest should be no-op for internal models, got: %v", err)
	}
	if len(request.MutatedHeaders()) != 0 {
		t.Errorf("no headers should be mutated for internal models, got %v", request.MutatedHeaders())
	}
}

// TestFullPipeline_SecretNotYetSynced verifies the error path when model-provider-resolver
// wrote credential refs but the Secret hasn't been synced to the store yet.
func TestFullPipeline_SecretNotYetSynced(t *testing.T) {
	store := newSecretStore()
	p := &APIKeyInjectionPlugin{
		typedName: plugin.TypedName{Type: PluginType, Name: "test"},
		generator: authgenerator.NewAPIKeyAuthGenerator(),
		store:     store,
	}

	cs := plugin.NewCycleState()
	cs.Write(modelproviderresolver.CredsRefName, "not-synced-yet")
	cs.Write(modelproviderresolver.CredsRefNamespace, "ns")

	request := requesthandling.NewInferenceRequest()

	err := p.ProcessRequest(context.Background(), cs, request)
	if err == nil {
		t.Fatal("expected error when Secret is not in store")
	}
}
