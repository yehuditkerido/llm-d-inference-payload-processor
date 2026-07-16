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

// CycleState keys written by the resolver and read by downstream plugins
// (apikey-injection, path-rewrite). Aligned with downstream IPP for compatibility.
const (
	ProviderKey       = "provider"
	ModelKey          = "model"
	CredsRefName      = "credential-ref-name"
	CredsRefNamespace = "credential-ref-namespace"
	ModelConfigKey    = "model-config"
	APIFormatKey      = "api-format"
	InputAPIFormatKey = "input-api-format"
)

// APIFormat identifies the API format for request/response translation.
type APIFormat string

const (
	OpenAIChatCompletions APIFormat = "openai-chat"
	Messages              APIFormat = "messages"
	OpenAIResponses       APIFormat = "openai-responses"
)
