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
	"testing"
)

func TestCalculateHash(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string][]byte
		expected string
		wantErr  bool
	}{
		{
			name: "simple data",
			data: map[string][]byte{
				"key1": []byte("value1"),
				"key2": []byte("value2"),
			},
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", // This will be different, just placeholder
			wantErr:  false,
		},
		{
			name:     "empty data",
			data:     map[string][]byte{},
			expected: "",
			wantErr:  false,
		},
		{
			name:     "nil data",
			data:     nil,
			expected: "",
			wantErr:  false,
		},
		{
			name: "single key",
			data: map[string][]byte{
				"password": []byte("secret123"),
			},
			expected: "", // Will be calculated
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := CalculateHash(tt.data)

			// For non-empty data, ensure hash is not empty
			if len(tt.data) > 0 && hash == "" {
				t.Errorf("CalculateHash() returned empty hash for non-empty data")
			}

			// For empty/nil data, ensure hash is empty
			if len(tt.data) == 0 && hash != "" {
				t.Errorf("CalculateHash() expected empty hash for empty data, got %s", hash)
			}
		})
	}
}

func TestCalculateHashDeterministic(t *testing.T) {
	// Test that hash is deterministic regardless of insertion order
	data1 := map[string][]byte{
		"a": []byte("value1"),
		"b": []byte("value2"),
		"c": []byte("value3"),
	}

	data2 := map[string][]byte{
		"c": []byte("value3"),
		"a": []byte("value1"),
		"b": []byte("value2"),
	}

	hash1 := CalculateHash(data1)
	hash2 := CalculateHash(data2)

	if hash1 != hash2 {
		t.Errorf("CalculateHash() not deterministic: hash1=%s, hash2=%s", hash1, hash2)
	}
}

func TestCalculateHashFromStringMap(t *testing.T) {
	data := map[string]string{
		"config.yaml":    "server:\n  port: 8080",
		"app.properties": "env=production",
	}

	hash := CalculateHashFromStringMap(data)

	if hash == "" {
		t.Errorf("CalculateHashFromStringMap() returned empty hash")
	}

	// Test that it matches byte map conversion
	byteData := map[string][]byte{
		"config.yaml":    []byte("server:\n  port: 8080"),
		"app.properties": []byte("env=production"),
	}

	expectedHash := CalculateHash(byteData)

	if hash != expectedHash {
		t.Errorf("CalculateHashFromStringMap() = %s, want %s", hash, expectedHash)
	}
}

func TestMergeDataMaps(t *testing.T) {
	stringData := map[string]string{
		"config.txt": "text data",
		"override":   "string value",
	}

	binaryData := map[string][]byte{
		"binary.dat": []byte{0x00, 0x01, 0x02},
		"override":   []byte("binary value"),
	}

	result := MergeDataMaps(stringData, binaryData)

	// Check all keys are present
	if len(result) != 3 {
		t.Errorf("MergeDataMaps() returned %d keys, want 3", len(result))
	}

	// Check binary data takes precedence for "override"
	if string(result["override"]) != "binary value" {
		t.Errorf("MergeDataMaps() binary data should override string data")
	}

	// Check string data is converted
	if string(result["config.txt"]) != "text data" {
		t.Errorf("MergeDataMaps() string data not converted correctly")
	}

	// Check binary data is preserved
	if len(result["binary.dat"]) != 3 {
		t.Errorf("MergeDataMaps() binary data not preserved")
	}
}

func TestCalculateHashChangeDetection(t *testing.T) {
	// Original data
	original := map[string][]byte{
		"username": []byte("admin"),
		"password": []byte("secret123"),
	}

	// Modified data
	modified := map[string][]byte{
		"username": []byte("admin"),
		"password": []byte("newsecret456"),
	}

	// Identical data
	identical := map[string][]byte{
		"username": []byte("admin"),
		"password": []byte("secret123"),
	}

	hashOriginal := CalculateHash(original)
	hashModified := CalculateHash(modified)
	hashIdentical := CalculateHash(identical)

	// Modified should have different hash
	if hashOriginal == hashModified {
		t.Errorf("CalculateHash() failed to detect data change")
	}

	// Identical should have same hash
	if hashOriginal != hashIdentical {
		t.Errorf("CalculateHash() produced different hash for identical data")
	}
}
