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
	"fmt"

	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/plugin"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/requesthandling"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/plugins/requesthandling/modelproviderresolver"
)

const (
	apiKeyField            = "api-key"
	defaultAuthHeader      = "Authorization"
	defaultAuthValuePrefix = "Bearer "
)

var _ AuthHeadersGenerator = &APIKeyAuthGenerator{}

func NewAPIKeyAuthGenerator() *APIKeyAuthGenerator {
	return &APIKeyAuthGenerator{}
}

// APIKeyAuthGenerator generates a single auth header from an API key.
// Supports custom header name and value prefix via model config in CycleState.
// Defaults to "Authorization: Bearer <key>".
type APIKeyAuthGenerator struct{}

// ExtractRequestData resolves the auth header name and value prefix from the
// model config in CycleState. Provider-specific defaults (e.g. "x-api-key" for
// Anthropic) are injected into the config by the provider reconciler. If not
// set, falls back to "Authorization" with "Bearer " prefix.
func (g *APIKeyAuthGenerator) ExtractRequestData(cycleState *plugin.CycleState, _ *requesthandling.InferenceRequest) (map[string]string, error) {
	config, err := plugin.ReadCycleStateKey[map[string]string](cycleState, modelproviderresolver.ModelConfigKey)
	if err != nil {
		return nil, fmt.Errorf("failed to extract config from cycle state - %w", err)
	}

	authHeader := defaultAuthHeader
	authValuePrefix := defaultAuthValuePrefix

	if headerName, ok := config[APIKeyAuthHeaderName]; ok && headerName != "" {
		authHeader = headerName
		authValuePrefix = ""
		if valuePrefix, ok := config[APIKeyAuthValuePrefix]; ok {
			authValuePrefix = valuePrefix
		}
	}

	return map[string]string{
		APIKeyAuthHeaderName:  authHeader,
		APIKeyAuthValuePrefix: authValuePrefix,
	}, nil
}

func (g *APIKeyAuthGenerator) GenerateAuthHeaders(credentialsData map[string]string) (map[string]string, error) {
	apiKey, ok := credentialsData[apiKeyField]
	if !ok {
		return nil, fmt.Errorf("credentials missing required field %s", apiKeyField)
	}

	headerName, ok := credentialsData[APIKeyAuthHeaderName]
	if !ok {
		return nil, fmt.Errorf("credentials missing required field %s", APIKeyAuthHeaderName)
	}

	valuePrefix, ok := credentialsData[APIKeyAuthValuePrefix]
	if !ok {
		return nil, fmt.Errorf("credentials missing required field %s", APIKeyAuthValuePrefix)
	}

	return map[string]string{
		headerName: fmt.Sprintf("%s%s", valuePrefix, apiKey),
	}, nil
}
