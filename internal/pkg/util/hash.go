/*
Copyright 2025.

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

package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
)

// CalculateHash computes a SHA256 hash of the provided data map.
// Keys are sorted to ensure deterministic hash calculation.
// This is used to detect actual data changes in Secrets and ConfigMaps.
func CalculateHash(data map[string][]byte) string {
	if data == nil || len(data) == 0 {
		return ""
	}

	// Sort keys for deterministic hashing
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Create hash
	hasher := sha256.New()

	// Write all key-value pairs in sorted order
	for _, key := range keys {
		hasher.Write([]byte(key))
		hasher.Write([]byte(":"))
		hasher.Write(data[key])
		hasher.Write([]byte(";"))
	}

	return hex.EncodeToString(hasher.Sum(nil))
}

// CalculateHashFromStringMap converts a string map to byte map and calculates hash.
// This is useful for ConfigMap.Data which uses map[string]string.
func CalculateHashFromStringMap(data map[string]string) string {
	if data == nil || len(data) == 0 {
		return ""
	}

	byteData := make(map[string][]byte, len(data))
	for k, v := range data {
		byteData[k] = []byte(v)
	}

	return CalculateHash(byteData)
}

// MergeDataMaps merges ConfigMap.Data (string) and ConfigMap.BinaryData ([]byte)
// into a single map for unified hash calculation.
func MergeDataMaps(stringData map[string]string, binaryData map[string][]byte) map[string][]byte {
	result := make(map[string][]byte)

	// Add string data
	for k, v := range stringData {
		result[k] = []byte(v)
	}

	// Add binary data (overwrites if key exists)
	for k, v := range binaryData {
		result[k] = v
	}

	return result
}

// GetResourceDataAndHash extracts data from a Secret or ConfigMap and calculates its hash
// This consolidates the duplicate data extraction logic from reconcile functions
func GetResourceDataAndHash(obj interface{}) (string, error) {
	switch resource := obj.(type) {
	case *corev1.Secret:
		return CalculateHash(resource.Data), nil
	case *corev1.ConfigMap:
		data := MergeDataMaps(resource.Data, resource.BinaryData)
		return CalculateHash(data), nil
	default:
		return "", fmt.Errorf("unsupported resource type: %T", obj)
	}
}
