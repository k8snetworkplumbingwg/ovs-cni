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
	"io/ioutil"
	"os"
	"path/filepath"
)

var (
	// DefaultCacheDir used for caching
	DefaultCacheDir = "/tmp/ovscache"
)

// SaveCache takes in key as string and a json encoded struct Conf and save this Conf in cache dir
func SaveCache(key string, conf interface{}) error {
	confBytes, err := json.Marshal(conf)
	if err != nil {
		return fmt.Errorf("error serializing delegate conf: %v", err)
	}

	// save the rendered conf for cmdDel
	if err = os.MkdirAll(DefaultCacheDir, 0700); err != nil {
		return fmt.Errorf("failed to create the sriov data directory(%q): %v", DefaultCacheDir, err)
	}
	path := getKeyPath(key)
	err = ioutil.WriteFile(path, confBytes, 0600)
	if err != nil {
		return fmt.Errorf("failed to write container data in the path(%q): %v", path, err)
	}
	return nil
}

// ReadCache read cached conf from disk for the given key and returns data in byte array
func ReadCache(key string) ([]byte, error) {
	path := getKeyPath(key)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read container data in the path(%q): %v", path, err)
	}
	return data, err
}

// CleanCache removes cached conf from disk for the given key
func CleanCache(key string) error {
	path := getKeyPath(key)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("error removing Conf file %s: %q", path, err)
	}
	return nil
}

func getKeyPath(key string) string {
	return filepath.Join(DefaultCacheDir, key)
}
