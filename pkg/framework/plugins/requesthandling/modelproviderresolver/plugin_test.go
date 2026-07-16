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
	"testing"

	"k8s.io/apimachinery/pkg/types"
)

func TestInfoStore_Provider(t *testing.T) {
	store := newInfoStore()
	key := types.NamespacedName{Namespace: "default", Name: "openai"}

	info := &providerInfo{
		provider:        "openai",
		endpoint:        "api.openai.com",
		secretName:      "openai-key",
		secretNamespace: "default",
		config:          map[string]string{"model_prefix": "gpt"},
	}

	store.addOrUpdateProvider(key, info)

	got, found := store.getProvider(key)
	if !found {
		t.Fatal("provider not found")
	}
	if got.provider != info.provider {
		t.Errorf("provider = %s, want %s", got.provider, info.provider)
	}
	if got.endpoint != info.endpoint {
		t.Errorf("endpoint = %s, want %s", got.endpoint, info.endpoint)
	}

	store.deleteProvider(key)
	if _, found := store.getProvider(key); found {
		t.Error("provider should be deleted")
	}
}

func TestInfoStore_Model(t *testing.T) {
	store := newInfoStore()
	key := types.NamespacedName{Namespace: "tenant-ns", Name: "my-model"}

	info := &externalModelInfo{
		modelName: "gpt-4",
		refs: []*resolvedProviderRef{
			{
				provider:    "openai",
				targetModel: "gpt-4-turbo",
				apiFormat:   OpenAIChatCompletions,
				secretName:  "openai-key",
				weight:      1,
			},
		},
	}

	store.addOrUpdateModel(key, info)

	got, found := store.getModel(key)
	if !found {
		t.Fatal("model not found")
	}
	if got.modelName != info.modelName {
		t.Errorf("modelName = %s, want %s", got.modelName, info.modelName)
	}
	if len(got.refs) != 1 {
		t.Fatalf("refs len = %d, want 1", len(got.refs))
	}
	if got.refs[0].provider != "openai" {
		t.Errorf("refs[0].provider = %s, want openai", got.refs[0].provider)
	}

	store.deleteModel(key)
	if _, found := store.getModel(key); found {
		t.Error("model should be deleted")
	}
}

func TestSelectByWeight(t *testing.T) {
	tests := []struct {
		name string
		refs []*resolvedProviderRef
		want string
	}{
		{
			name: "single ref returns it",
			refs: []*resolvedProviderRef{
				{provider: "openai", weight: 1},
			},
			want: "openai",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectByWeight(tt.refs)
			if got.provider != tt.want {
				t.Errorf("selectByWeight() = %s, want %s", got.provider, tt.want)
			}
		})
	}
}

func TestSelectByWeight_MultipleRefs(t *testing.T) {
	refs := []*resolvedProviderRef{
		{provider: "openai", weight: 80},
		{provider: "anthropic", weight: 20},
	}

	counts := map[string]int{}
	iterations := 1000
	for i := 0; i < iterations; i++ {
		got := selectByWeight(refs)
		counts[got.provider]++
	}

	openaiRatio := float64(counts["openai"]) / float64(iterations)
	if openaiRatio < 0.6 || openaiRatio > 0.95 {
		t.Errorf("openai selection ratio = %f, expected ~0.8", openaiRatio)
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/tenant-ns/my-model/v1/chat/completions", "tenant-ns/my-model/v1/chat/completions"},
		{"tenant-ns/my-model/v1/chat/completions/", "tenant-ns/my-model/v1/chat/completions"},
		{"/tenant-ns/my-model/v1/chat/completions?stream=true", "tenant-ns/my-model/v1/chat/completions"},
		{"  /tenant-ns/my-model/v1/messages  ", "tenant-ns/my-model/v1/messages"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizePath(tt.input)
			if got != tt.want {
				t.Errorf("sanitizePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDetectInputAPIFormat(t *testing.T) {
	tests := []struct {
		path string
		want APIFormat
	}{
		{"tenant-ns/my-model/v1/chat/completions", OpenAIChatCompletions},
		{"ns/model/v1/messages", Messages},
		{"ns/model/v1/responses", OpenAIResponses},
		{"ns/model/v1/embeddings", ""},
		{"ns/model/unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectInputAPIFormat(tt.path)
			if got != tt.want {
				t.Errorf("detectInputAPIFormat(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestMergeConfig(t *testing.T) {
	providerConfig := map[string]string{"key1": "val1", "key2": "val2"}
	modelConfig := map[string]string{"key2": "override", "key3": "val3"}

	merged := mergeConfig(providerConfig, modelConfig)

	if merged["key1"] != "val1" {
		t.Errorf("key1 = %s, want val1", merged["key1"])
	}
	if merged["key2"] != "override" {
		t.Errorf("key2 = %s, want override", merged["key2"])
	}
	if merged["key3"] != "val3" {
		t.Errorf("key3 = %s, want val3", merged["key3"])
	}
	if providerConfig["key2"] != "val2" {
		t.Error("providerConfig was modified")
	}
}
