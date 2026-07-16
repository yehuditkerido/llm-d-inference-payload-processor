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

package modelproviderresolver

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	inferencev1alpha1 "github.com/llm-d/llm-d-inference-payload-processor/api/inference/v1alpha1"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/plugin"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/requesthandling"
)

func intPtr(i int) *int { return &i }

// TestProcessRequest_EndToEnd tests the full flow: store populated → ProcessRequest → CycleState written.
func TestProcessRequest_EndToEnd(t *testing.T) {
	store := newInfoStore()

	// Simulate reconciler populating the store with provider info
	providerKey := types.NamespacedName{Namespace: "tenant-ns", Name: "openai-provider"}
	store.addOrUpdateProvider(providerKey, &providerInfo{
		provider:        "openai",
		endpoint:        "api.openai.com",
		secretName:      "openai-secret",
		secretNamespace: "tenant-ns",
		config:          map[string]string{"org": "my-org"},
	})

	// Simulate reconciler populating the store with model info
	modelKey := types.NamespacedName{Namespace: "tenant-ns", Name: "my-gpt4"}
	store.addOrUpdateModel(modelKey, &externalModelInfo{
		modelName: "gpt-4-turbo",
		refs: []*resolvedProviderRef{
			{
				provider:        "openai",
				targetModel:     "gpt-4-turbo-preview",
				apiFormat:       OpenAIChatCompletions,
				secretName:      "openai-secret",
				secretNamespace: "tenant-ns",
				config:          map[string]string{"org": "my-org"},
				weight:          1,
			},
		},
	})

	// Create plugin with pre-populated store
	p := &ModelProviderResolverPlugin{
		typedName: plugin.TypedName{Type: ModelProviderResolverPluginType, Name: "test"},
		store:     store,
	}

	// Create request simulating path: /tenant-ns/my-gpt4/v1/chat/completions
	request := requesthandling.NewInferenceRequest()
	request.Headers[":path"] = "/tenant-ns/my-gpt4/v1/chat/completions"
	request.Body["model"] = "gpt-4-turbo"

	cycleState := plugin.NewCycleState()

	err := p.ProcessRequest(context.Background(), cycleState, request)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	// Verify CycleState was populated correctly
	tests := []struct {
		key  string
		want string
	}{
		{ProviderKey, "openai"},
		{ModelKey, "gpt-4-turbo-preview"},
		{APIFormatKey, string(OpenAIChatCompletions)},
		{CredsRefName, "openai-secret"},
		{CredsRefNamespace, "tenant-ns"},
		{InputAPIFormatKey, string(OpenAIChatCompletions)},
	}

	for _, tt := range tests {
		val, err := cycleState.Read(tt.key)
		if err != nil {
			t.Errorf("CycleState.Read(%q) error: %v", tt.key, err)
			continue
		}
		if got, ok := val.(string); !ok || got != tt.want {
			t.Errorf("CycleState[%q] = %v, want %q", tt.key, val, tt.want)
		}
	}

	// Verify config map
	configVal, err := cycleState.Read(ModelConfigKey)
	if err != nil {
		t.Fatalf("CycleState.Read(%q) error: %v", ModelConfigKey, err)
	}
	config, ok := configVal.(map[string]string)
	if !ok {
		t.Fatalf("CycleState[%q] is not map[string]string", ModelConfigKey)
	}
	if config["org"] != "my-org" {
		t.Errorf("config[\"org\"] = %q, want \"my-org\"", config["org"])
	}
}

// TestProcessRequest_ModelNotInStore tests passthrough when model is not in store.
func TestProcessRequest_ModelNotInStore(t *testing.T) {
	store := newInfoStore()
	p := &ModelProviderResolverPlugin{
		typedName: plugin.TypedName{Type: ModelProviderResolverPluginType, Name: "test"},
		store:     store,
	}

	request := requesthandling.NewInferenceRequest()
	request.Headers[":path"] = "/tenant-ns/unknown-model/v1/chat/completions"
	request.Body["model"] = "some-model"

	cycleState := plugin.NewCycleState()

	err := p.ProcessRequest(context.Background(), cycleState, request)
	if err != nil {
		t.Fatalf("ProcessRequest should pass through, got error: %v", err)
	}

	// CycleState should be empty (passthrough)
	if _, err := cycleState.Read(ProviderKey); err == nil {
		t.Error("CycleState should be empty for unknown model")
	}
}

// TestProcessRequest_ModelMismatch tests error when body model doesn't match ExternalModel.
func TestProcessRequest_ModelMismatch(t *testing.T) {
	store := newInfoStore()
	store.addOrUpdateModel(
		types.NamespacedName{Namespace: "ns", Name: "my-model"},
		&externalModelInfo{
			modelName: "expected-model",
			refs:      []*resolvedProviderRef{{provider: "test", weight: 1}},
		},
	)

	p := &ModelProviderResolverPlugin{
		typedName: plugin.TypedName{Type: ModelProviderResolverPluginType, Name: "test"},
		store:     store,
	}

	request := requesthandling.NewInferenceRequest()
	request.Headers[":path"] = "/ns/my-model/v1/chat/completions"
	request.Body["model"] = "wrong-model" // doesn't match "expected-model"

	err := p.ProcessRequest(context.Background(), plugin.NewCycleState(), request)
	if err == nil {
		t.Fatal("ProcessRequest should return error for model mismatch")
	}
}

// TestProcessRequest_UnsupportedPath tests error for unsupported API paths.
func TestProcessRequest_UnsupportedPath(t *testing.T) {
	store := newInfoStore()
	store.addOrUpdateModel(
		types.NamespacedName{Namespace: "ns", Name: "my-model"},
		&externalModelInfo{
			modelName: "my-model",
			refs:      []*resolvedProviderRef{{provider: "test", weight: 1}},
		},
	)

	p := &ModelProviderResolverPlugin{
		typedName: plugin.TypedName{Type: ModelProviderResolverPluginType, Name: "test"},
		store:     store,
	}

	request := requesthandling.NewInferenceRequest()
	request.Headers[":path"] = "/ns/my-model/v1/embeddings" // unsupported
	request.Body["model"] = "my-model"

	err := p.ProcessRequest(context.Background(), plugin.NewCycleState(), request)
	if err == nil {
		t.Fatal("ProcessRequest should return error for unsupported path")
	}
}

// TestReconciler_ResolveRef tests the resolveRef helper.
func TestReconciler_ResolveRef(t *testing.T) {
	store := newInfoStore()
	store.addOrUpdateProvider(
		types.NamespacedName{Namespace: "ns", Name: "my-provider"},
		&providerInfo{
			provider:        "anthropic",
			endpoint:        "api.anthropic.com",
			secretName:      "anthropic-key",
			secretNamespace: "ns",
			config:          map[string]string{"version": "2024-01"},
		},
	)

	r := &externalModelReconciler{store: store}

	ref := &inferencev1alpha1.ExternalProviderRef{
		Ref:         inferencev1alpha1.NameReference{Name: "my-provider"},
		TargetModel: "claude-3-opus",
		APIFormat:   "messages",
		Config:      map[string]string{"max_tokens": "4096"},
		Weight:      intPtr(80),
	}

	resolved, found := r.resolveRef("ns", ref)
	if !found {
		t.Fatal("resolveRef should find provider")
	}

	if resolved.provider != "anthropic" {
		t.Errorf("provider = %q, want \"anthropic\"", resolved.provider)
	}
	if resolved.targetModel != "claude-3-opus" {
		t.Errorf("targetModel = %q, want \"claude-3-opus\"", resolved.targetModel)
	}
	if resolved.apiFormat != Messages {
		t.Errorf("apiFormat = %q, want %q", resolved.apiFormat, Messages)
	}
	if resolved.weight != 80 {
		t.Errorf("weight = %d, want 80", resolved.weight)
	}
	// Config should be merged
	if resolved.config["version"] != "2024-01" {
		t.Errorf("config[\"version\"] = %q, want \"2024-01\"", resolved.config["version"])
	}
	if resolved.config["max_tokens"] != "4096" {
		t.Errorf("config[\"max_tokens\"] = %q, want \"4096\"", resolved.config["max_tokens"])
	}
}

// TestReconciler_ResolveRef_AuthOverride tests auth override in ExternalProviderRef.
func TestReconciler_ResolveRef_AuthOverride(t *testing.T) {
	store := newInfoStore()
	store.addOrUpdateProvider(
		types.NamespacedName{Namespace: "ns", Name: "my-provider"},
		&providerInfo{
			provider:        "openai",
			secretName:      "default-secret",
			secretNamespace: "ns",
		},
	)

	r := &externalModelReconciler{store: store}

	ref := &inferencev1alpha1.ExternalProviderRef{
		Ref:         inferencev1alpha1.NameReference{Name: "my-provider"},
		TargetModel: "gpt-4",
		APIFormat:   "openai-chat",
		Auth: &inferencev1alpha1.AuthConfig{
			Type:      "simple",
			SecretRef: inferencev1alpha1.NameReference{Name: "override-secret"},
		},
	}

	resolved, found := r.resolveRef("ns", ref)
	if !found {
		t.Fatal("resolveRef should find provider")
	}

	if resolved.secretName != "override-secret" {
		t.Errorf("secretName = %q, want \"override-secret\"", resolved.secretName)
	}
}
