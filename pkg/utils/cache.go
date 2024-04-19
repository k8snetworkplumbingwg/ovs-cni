// Copyright (c) 2021 Red Hat, Inc.
// Copyright (c) 2021 CNI authors
// Copyright (c) 2021 Nordix Foundation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var (
	// DefaultCacheDir used for caching
	DefaultCacheDir = "/var/lib/cni/ovs-cni/cache"
	// OldDefaultCacheDir path to the old caching dir
	OldDefaultCacheDir = "/tmp/ovscache"
	// used for tests
	rootDir = ""
)

// SaveCache takes in key as string and a json encoded struct Conf and save this Conf in cache dir
func SaveCache(key string, conf interface{}) error {
	confBytes, err := json.Marshal(conf)
	if err != nil {
		return fmt.Errorf("error serializing delegate conf: %v", err)
	}
	path := getKeyPath(key)
	cacheDir := filepath.Dir(path)
	// save the rendered conf for cmdDel
	if err = os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("failed to create the sriov data directory(%q): %v", cacheDir, err)
	}
	err = os.WriteFile(path, confBytes, 0600)
	if err != nil {
		return fmt.Errorf("failed to write container data in the path(%q): %v", path, err)
	}
	return nil
}

// ReadCache read cached conf from disk for the given key and returns data in byte array
func ReadCache(key string) ([]byte, error) {
	path := getKeyPath(key)
	oldPath := getOldKeyPath(key)
	data, err := readCacheFile(path)
	if err != nil {
		return nil, err
	}
	if data == nil {
		data, err = readCacheFile(oldPath)
		if err != nil {
			return nil, err
		}
	}
	if data == nil {
		return nil, fmt.Errorf("failed to read container data from old(%q) and current(%q) path: not found", oldPath, path)
	}
	return data, nil
}

// CleanCache removes cached conf from disk for the given key
func CleanCache(key string) error {
	if err := removeCacheFile(getKeyPath(key)); err != nil {
		return nil
	}
	return removeCacheFile(getOldKeyPath(key))
}

// read content from the file in the provided path, returns nil, nil
// if file not found
func readCacheFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read container data in the path(%q): %v", path, err)
	}
	return data, nil
}

// remove file in the provided path, returns nil if file not found
func removeCacheFile(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("failed to remove container data from the path(%q): %v", path, err)
	}
	return nil
}

func getKeyPath(key string) string {
	return filepath.Join(rootDir, DefaultCacheDir, key)
}

func getOldKeyPath(key string) string {
	return filepath.Join(rootDir, OldDefaultCacheDir, key)
}
