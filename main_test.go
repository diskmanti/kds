// This file contains the test suite for the kds application.
package main

import (
	"encoding/base64"
	"os"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// TestFetchSecrets verifies that the command to fetch all secrets works correctly.
func TestFetchSecrets(t *testing.T) {
	testNamespace := "default"
	t.Run("should fetch a list of secrets successfully", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret-a", Namespace: testNamespace}},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret-b", Namespace: testNamespace}},
		)
		expectedItems := itemSource{
			{name: "secret-a", namespace: testNamespace},
			{name: "secret-b", namespace: testNamespace},
		}
		msg := fetchSecrets(clientset, testNamespace)()
		items, ok := msg.(itemSource)
		if !ok {
			t.Fatalf("Expected message of type itemSource, but got %T", msg)
		}
		if !reflect.DeepEqual(items, expectedItems) {
			t.Errorf("Expected secrets %v, but got %v", expectedItems, items)
		}
	})
	t.Run("should return an error if no secrets are found", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		msg := fetchSecrets(clientset, testNamespace)()
		if _, ok := msg.(fatalErrorMsg); !ok {
			t.Fatalf("Expected message of type fatalErrorMsg, but got %T", msg)
		}
	})
}

// TestFetchSecretData verifies that fetching and decoding a single secret works.
func TestFetchSecretData(t *testing.T) {
	testNamespace := "default"
	secretName := "my-secret"
	secretKey := "token"
	decodedValue := "hello-world"
	encodedValue := base64.StdEncoding.EncodeToString([]byte(decodedValue))
	t.Run("should fetch and decode a secret's data successfully", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace},
			Data:       map[string][]byte{secretKey: []byte(encodedValue)},
		}
		clientset := fake.NewSimpleClientset(secret)
		msg := fetchSecretData(clientset, secretName, testNamespace)()
		dataMsg, ok := msg.(secretDataLoadedMsg)
		if !ok {
			t.Fatalf("Expected message of type secretDataLoadedMsg, but got %T", msg)
		}
		if dataMsg.data[secretKey] != decodedValue {
			t.Errorf("Expected decoded value '%s', but got '%s'", decodedValue, dataMsg.data[secretKey])
		}
	})
	t.Run("should return an error if the secret does not exist", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		msg := fetchSecretData(clientset, "non-existent-secret", testNamespace)()
		if _, ok := msg.(secretDataErrorMsg); !ok {
			t.Fatalf("Expected message of type secretDataErrorMsg, but got %T", msg)
		}
	})
}

// TestGetNamespaceFromKubeconfig verifies parsing the active namespace from a kubeconfig file.
func TestGetNamespaceFromKubeconfig(t *testing.T) {
	expectedNamespace := "my-test-namespace"
	kubeconfigFile, err := createFakeKubeconfig(expectedNamespace)
	if err != nil {
		t.Fatalf("Failed to create fake kubeconfig: %v", err)
	}
	defer os.Remove(kubeconfigFile.Name())
	namespace, err := getNamespaceFromKubeconfig(kubeconfigFile.Name())
	if err != nil {
		t.Fatalf("Expected no error, but got: %v", err)
	}
	if namespace != expectedNamespace {
		t.Errorf("Expected namespace '%s', but got '%s'", expectedNamespace, namespace)
	}
}

// createFakeKubeconfig is a helper function to create a temporary kubeconfig file.
func createFakeKubeconfig(namespace string) (*os.File, error) {
	config := clientcmdapi.Config{
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"user": {}},
		Clusters:  map[string]*clientcmdapi.Cluster{"cluster": {Server: "https://example.com"}},
		Contexts: map[string]*clientcmdapi.Context{
			"my-context": {AuthInfo: "user", Cluster: "cluster", Namespace: namespace},
		},
		CurrentContext: "my-context",
	}
	file, err := os.CreateTemp("", "kubeconfig-")
	if err != nil {
		return nil, err
	}
	if err := clientcmd.WriteToFile(config, file.Name()); err != nil {
		return nil, err
	}
	return file, nil
}
