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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testSecret(namespace, name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Data:       data,
	}
}

func TestSecretStore_AddAndGet(t *testing.T) {
	s := newSecretStore()

	secret := testSecret("ns", "my-secret", map[string][]byte{
		"api-key": []byte("sk-test-123"),
	})

	if err := s.addOrUpdate("ns", "my-secret", secret); err != nil {
		t.Fatalf("addOrUpdate() error = %v", err)
	}

	creds, ok := s.get("ns", "my-secret")
	if !ok {
		t.Fatal("get() returned false, expected true")
	}
	if creds["api-key"] != "sk-test-123" {
		t.Errorf("creds[api-key] = %q, want %q", creds["api-key"], "sk-test-123")
	}
}

func TestSecretStore_Update(t *testing.T) {
	s := newSecretStore()

	secret := testSecret("ns", "creds", map[string][]byte{
		"api-key": []byte("old-key"),
	})
	if err := s.addOrUpdate("ns", "creds", secret); err != nil {
		t.Fatalf("addOrUpdate() error = %v", err)
	}

	updated := testSecret("ns", "creds", map[string][]byte{
		"api-key": []byte("new-key"),
	})
	if err := s.addOrUpdate("ns", "creds", updated); err != nil {
		t.Fatalf("addOrUpdate() error = %v", err)
	}

	creds, ok := s.get("ns", "creds")
	if !ok {
		t.Fatal("get() returned false after update")
	}
	if creds["api-key"] != "new-key" {
		t.Errorf("creds[api-key] = %q, want %q after update", creds["api-key"], "new-key")
	}
}

func TestSecretStore_Delete(t *testing.T) {
	s := newSecretStore()

	secret := testSecret("ns", "creds", map[string][]byte{
		"api-key": []byte("key"),
	})
	if err := s.addOrUpdate("ns", "creds", secret); err != nil {
		t.Fatalf("addOrUpdate() error = %v", err)
	}

	s.delete("ns", "creds")

	_, ok := s.get("ns", "creds")
	if ok {
		t.Fatal("get() returned true after delete")
	}
}

func TestSecretStore_DeleteNonExistent(t *testing.T) {
	s := newSecretStore()
	s.delete("ns", "does-not-exist")
}

func TestSecretStore_GetNonExistent(t *testing.T) {
	s := newSecretStore()

	_, ok := s.get("ns", "missing")
	if ok {
		t.Fatal("get() returned true for non-existent secret")
	}
}

func TestSecretStore_EmptyData(t *testing.T) {
	s := newSecretStore()

	secret := testSecret("ns", "empty", map[string][]byte{})

	err := s.addOrUpdate("ns", "empty", secret)
	if err == nil {
		t.Fatal("expected error for secret with no data fields")
	}
}

func TestSecretStore_EmptyFieldValue(t *testing.T) {
	s := newSecretStore()

	secret := testSecret("ns", "bad", map[string][]byte{
		"api-key": {},
	})

	err := s.addOrUpdate("ns", "bad", secret)
	if err == nil {
		t.Fatal("expected error for secret with empty field")
	}

	_, ok := s.get("ns", "bad")
	if ok {
		t.Fatal("secret with empty field should not be in store")
	}
}

func TestSecretStore_MultipleNamespaces(t *testing.T) {
	s := newSecretStore()

	s1 := testSecret("ns1", "creds", map[string][]byte{"key": []byte("val1")})
	s2 := testSecret("ns2", "creds", map[string][]byte{"key": []byte("val2")})

	if err := s.addOrUpdate("ns1", "creds", s1); err != nil {
		t.Fatalf("addOrUpdate(ns1) error = %v", err)
	}
	if err := s.addOrUpdate("ns2", "creds", s2); err != nil {
		t.Fatalf("addOrUpdate(ns2) error = %v", err)
	}

	c1, ok := s.get("ns1", "creds")
	if !ok || c1["key"] != "val1" {
		t.Errorf("ns1 creds = %v, %v; want val1, true", c1, ok)
	}
	c2, ok := s.get("ns2", "creds")
	if !ok || c2["key"] != "val2" {
		t.Errorf("ns2 creds = %v, %v; want val2, true", c2, ok)
	}

	s.delete("ns1", "creds")
	_, ok = s.get("ns1", "creds")
	if ok {
		t.Fatal("ns1 should be deleted")
	}
	_, ok = s.get("ns2", "creds")
	if !ok {
		t.Fatal("ns2 should still exist after ns1 delete")
	}
}

func TestSecretStore_MultipleFields(t *testing.T) {
	s := newSecretStore()

	secret := testSecret("ns", "aws", map[string][]byte{
		"aws-access-key-id":     []byte("AKID"),
		"aws-secret-access-key": []byte("SECRET"),
		"aws-session-token":     []byte("TOKEN"),
	})

	if err := s.addOrUpdate("ns", "aws", secret); err != nil {
		t.Fatalf("addOrUpdate() error = %v", err)
	}

	creds, ok := s.get("ns", "aws")
	if !ok {
		t.Fatal("get() returned false")
	}
	if len(creds) != 3 {
		t.Errorf("expected 3 fields, got %d", len(creds))
	}
	if creds["aws-access-key-id"] != "AKID" {
		t.Errorf("access key = %q, want %q", creds["aws-access-key-id"], "AKID")
	}
}
