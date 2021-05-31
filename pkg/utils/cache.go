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
	"strings"
)

var (
	// DefaultCacheDir used for caching
	DefaultCacheDir = "/tmp/ovscache"
)

// SaveCache takes in container ID, Pod interface name and cache dir as string and a json
// encoded struct Conf and save this Conf in cache dir
func SaveCache(cid, podIfName, cacheDir string, conf interface{}) error {
	confBytes, err := json.Marshal(conf)
	if err != nil {
		return fmt.Errorf("error serializing delegate conf: %v", err)
	}

	s := []string{cid, podIfName}
	cRef := strings.Join(s, "-")

	// save the rendered conf for cmdDel
	if err = saveScratchConf(cacheDir, cRef, confBytes); err != nil {
		return err
	}

	return nil
}

func saveScratchConf(cacheDir, cRef string, conf []byte) error {
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("failed to create the sriov data directory(%q): %v", cacheDir, err)
	}

	path := filepath.Join(cacheDir, cRef)

	err := ioutil.WriteFile(path, conf, 0600)
	if err != nil {
		return fmt.Errorf("failed to write container data in the path(%q): %v", path, err)
	}

	return err
}

// ReadCache read cached conf from disk for the given container reference path
// and returns data in byte array
func ReadCache(cRefPath string) ([]byte, error) {
	data, err := ioutil.ReadFile(cRefPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read container data in the path(%q): %v", cRefPath, err)
	}
	return data, err
}

// CleanCache removes cached conf from disk for the given container reference path
func CleanCache(cRefPath string) error {
	if err := os.Remove(cRefPath); err != nil {
		return fmt.Errorf("error removing Conf file %s: %q", cRefPath, err)
	}
	return nil
}
