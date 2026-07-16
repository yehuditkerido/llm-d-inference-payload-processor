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
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	errcommon "github.com/llm-d/llm-d-inference-payload-processor/pkg/common/error"
	logutil "github.com/llm-d/llm-d-inference-payload-processor/pkg/common/observability/logging"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/plugin"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/requesthandling"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/plugins/requesthandling/apikeyinjection/authgenerator"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/plugins/requesthandling/modelproviderresolver"
)

const (
	PluginType = "apikey-injection"
)

// compile-time type validation
var _ requesthandling.RequestProcessor = &APIKeyInjectionPlugin{}

// PluginFactory defines the factory function for APIKeyInjectionPlugin.
func PluginFactory(name string, _ json.RawMessage, handle plugin.Handle) (plugin.Plugin, error) {
	p, err := NewAPIKeyInjectionPlugin(handle.ReconcilerBuilder, handle.Client())
	if err != nil {
		return nil, fmt.Errorf("failed to create plugin '%s' - %w", PluginType, err)
	}

	return p.WithName(name), nil
}

// NewAPIKeyInjectionPlugin returns an *APIKeyInjectionPlugin with an initialized
// secretStore and Secret reconciler filtered by the ipp-managed label.
func NewAPIKeyInjectionPlugin(reconcilerBuilder func() *builder.Builder, clientReader client.Reader) (*APIKeyInjectionPlugin, error) {
	store := newSecretStore()
	reconciler := &secretReconciler{
		Reader: clientReader,
		store:  store,
	}

	if err := reconcilerBuilder().For(&corev1.Secret{}).WithEventFilter(ippManagedPredicate()).Complete(reconciler); err != nil {
		return nil, fmt.Errorf("failed to register Secret reconciler for plugin '%s' - %w", PluginType, err)
	}

	return &APIKeyInjectionPlugin{
		typedName: plugin.TypedName{Type: PluginType, Name: PluginType},
		generator: authgenerator.NewAPIKeyAuthGenerator(),
		store:     store,
	}, nil
}

// APIKeyInjectionPlugin injects credentials from a Kubernetes Secret into the
// request headers. The Secret is identified by its namespaced name written to
// CycleState by the model-provider-resolver plugin.
type APIKeyInjectionPlugin struct {
	typedName plugin.TypedName
	generator authgenerator.AuthHeadersGenerator
	store     *secretStore
}

// NewAPIKeyInjectionPluginWithSecrets creates a plugin pre-loaded with the given
// secrets. Intended for integration tests where a controller-runtime Manager is
// not available (same pattern as basemodelextractor.BaseModelToHeaderPlugin
// construction in test/integration/harness.go).
func NewAPIKeyInjectionPluginWithSecrets(secrets []*corev1.Secret) (*APIKeyInjectionPlugin, error) {
	store := newSecretStore()
	for _, s := range secrets {
		if err := store.addOrUpdate(s.Namespace, s.Name, s); err != nil {
			return nil, err
		}
	}
	return &APIKeyInjectionPlugin{
		typedName: plugin.TypedName{Type: PluginType, Name: PluginType},
		generator: authgenerator.NewAPIKeyAuthGenerator(),
		store:     store,
	}, nil
}

// TypedName returns the type and name tuple of this plugin instance.
func (p *APIKeyInjectionPlugin) TypedName() plugin.TypedName {
	return p.typedName
}

// WithName sets the name of the IPP plugin instance.
func (p *APIKeyInjectionPlugin) WithName(name string) *APIKeyInjectionPlugin {
	p.typedName.Name = name
	return p
}

// ProcessRequest reads credential metadata from CycleState (written by
// model-provider-resolver), looks up the Secret in the store, and injects
// auth headers into the request. If credential-ref-name is absent from
// CycleState, this is an internal model request and the plugin is a no-op.
func (p *APIKeyInjectionPlugin) ProcessRequest(ctx context.Context, cycleState *plugin.CycleState, request *requesthandling.InferenceRequest) error {
	logger := log.FromContext(ctx).V(logutil.DEFAULT)

	name, err := plugin.ReadCycleStateKey[string](cycleState, modelproviderresolver.CredsRefName)
	if err != nil {
		if errors.Is(err, plugin.ErrNotFound) {
			return nil
		}
		return errcommon.Error{Code: errcommon.Internal, Msg: fmt.Sprintf("failed to read credential ref from CycleState: %v", err)}
	}
	if name == "" {
		return nil
	}

	namespace, err := plugin.ReadCycleStateKey[string](cycleState, modelproviderresolver.CredsRefNamespace)
	if err != nil || namespace == "" {
		logger.Error(err, "credentialRef namespace missing")
		return errcommon.Error{Code: errcommon.Internal, Msg: "credential-ref-namespace missing from CycleState"}
	}

	credentialsData, found := p.store.get(namespace, name)
	if !found {
		logger.Error(nil, "credentials not found in store", "secret", fmt.Sprintf("%s/%s", namespace, name))
		return errcommon.Error{Code: errcommon.Internal, Msg: fmt.Sprintf("credentials Secret '%s/%s' not found in store", namespace, name)}
	}

	extraData, err := p.generator.ExtractRequestData(cycleState, request)
	if err != nil {
		return errcommon.Error{Code: errcommon.Internal, Msg: fmt.Sprintf("failed to extract request data: %v", err)}
	}
	if len(extraData) > 0 {
		merged := make(map[string]string, len(credentialsData)+len(extraData))
		maps.Copy(merged, credentialsData)
		maps.Copy(merged, extraData)
		credentialsData = merged
	}

	authHeaders, err := p.generator.GenerateAuthHeaders(credentialsData)
	if err != nil {
		logger.Error(err, "auth header generation failed")
		return errcommon.Error{Code: errcommon.Internal, Msg: fmt.Sprintf("failed to generate auth headers: %v", err)}
	}

	for headerKey, headerValue := range authHeaders {
		request.SetHeader(headerKey, headerValue)
	}

	logger.Info("auth headers injected", "secret", fmt.Sprintf("%s/%s", namespace, name))
	return nil
}
