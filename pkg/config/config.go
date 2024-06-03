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

package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/imdario/mergo"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/types"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/utils"
)

const (
	linkstateCheckRetries  = 5
	linkStateCheckInterval = 600 // in milliseconds
)

// LoadConf parses and validates stdin netconf and returns NetConf object
func LoadConf(data []byte) (*types.NetConf, error) {
	netconf, err := loadNetConf(data)
	if err != nil {
		return nil, err
	}
	flatNetConf, err := loadFlatNetConf[types.NetConf](netconf.ConfigurationPath)
	if err != nil {
		return nil, err
	}
	netconf, err = mergeConf(netconf, flatNetConf)
	if err != nil {
		return nil, err
	}

	if netconf.LinkStateCheckRetries == 0 {
		netconf.LinkStateCheckRetries = linkstateCheckRetries
	}

	if netconf.LinkStateCheckInterval == 0 {
		netconf.LinkStateCheckInterval = linkStateCheckInterval
	}
	return netconf, nil
}

// LoadMirrorConf parses and validates stdin netconf and returns MirrorNetConf object
func LoadMirrorConf(data []byte) (*types.MirrorNetConf, error) {
	netconf, err := loadMirrorNetConf(data)
	if err != nil {
		return nil, err
	}
	flatNetConf, err := loadFlatNetConf[types.MirrorNetConf](netconf.ConfigurationPath)
	if err != nil {
		return nil, err
	}
	netconf, err = mergeConf(netconf, flatNetConf)
	if err != nil {
		return nil, err
	}
	return netconf, nil
}

// LoadPrevResultConfFromCache retrieve preResult config from cache
func LoadPrevResultConfFromCache(cRef string) (*types.CachedPrevResultNetConf, error) {
	netCache := &types.CachedPrevResultNetConf{}
	netConfBytes, err := utils.ReadCache(cRef)
	if err != nil {
		return nil, fmt.Errorf("error reading cached prevResult conf with name %s: %v", cRef, err)
	}

	if err = json.Unmarshal(netConfBytes, netCache); err != nil {
		return nil, fmt.Errorf("failed to parse prevResult conf: %v", err)
	}

	return netCache, nil
}

// LoadConfFromCache retrieve net config from cache
func LoadConfFromCache(cRef string) (*types.CachedNetConf, error) {
	netCache := &types.CachedNetConf{}
	netConfBytes, err := utils.ReadCache(cRef)
	if err != nil {
		return nil, fmt.Errorf("error reading cached NetConf with name %s: %v", cRef, err)
	}

	if err = json.Unmarshal(netConfBytes, netCache); err != nil {
		return nil, fmt.Errorf("failed to parse NetConf: %v", err)
	}

	return netCache, nil
}

// GetCRef unique identifier for a container interface
func GetCRef(cid, podIfName string) string {
	return strings.Join([]string{cid, podIfName}, "-")
}

func loadNetConf(bytes []byte) (*types.NetConf, error) {
	netconf := &types.NetConf{}
	if err := json.Unmarshal(bytes, netconf); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}
	return netconf, nil
}

func loadMirrorNetConf(bytes []byte) (*types.MirrorNetConf, error) {
	netconf := &types.MirrorNetConf{}
	if err := json.Unmarshal(bytes, netconf); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}

	// Parse previous result
	if netconf.RawPrevResult != nil {
		resultBytes, err := json.Marshal(netconf.RawPrevResult)
		if err != nil {
			return nil, fmt.Errorf("loadNetConf: could not serialize prevResult: %v", err)
		}
		res, err := version.NewResult(netconf.CNIVersion, resultBytes)
		if err != nil {
			return nil, fmt.Errorf("loadNetConf: could not parse prevResult: %v", err)
		}
		netconf.RawPrevResult = nil
		netconf.PrevResult, err = current.NewResultFromResult(res)
		if err != nil {
			return nil, fmt.Errorf("loadNetConf: could not convert result to current version: %v", err)
		}
	}

	return netconf, nil
}

func loadFlatNetConf[T types.NetConfs](configPath string) (*T, error) {
	confFiles := getOvsConfFiles()
	if configPath != "" {
		confFiles = append([]string{configPath}, confFiles...)
	}

	// loop through the path and parse the JSON config
	flatNetConf := new(T)
	for _, confFile := range confFiles {
		confExists, err := pathExists(confFile)
		if err != nil {
			return nil, fmt.Errorf("error checking ovs config file: error: %v", err)
		}
		if confExists {
			jsonFile, err := os.Open(confFile)
			if err != nil {
				return nil, fmt.Errorf("open ovs config file %s error: %v", confFile, err)
			}
			defer jsonFile.Close()
			jsonBytes, err := io.ReadAll(jsonFile)
			if err != nil {
				return nil, fmt.Errorf("load ovs config file %s: error: %v", confFile, err)
			}
			if err := json.Unmarshal(jsonBytes, flatNetConf); err != nil {
				return nil, fmt.Errorf("parse ovs config file %s: error: %v", confFile, err)
			}
			break
		}
	}

	return flatNetConf, nil
}

func mergeConf[T types.NetConfs](netconf, flatNetConf *T) (*T, error) {
	if err := mergo.Merge(netconf, flatNetConf); err != nil {
		return nil, fmt.Errorf("merge with ovs config file: error: %v", err)
	}
	return netconf, nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func getOvsConfFiles() []string {
	return []string{"/etc/kubernetes/cni/net.d/ovs.d/ovs.conf", "/etc/cni/net.d/ovs.d/ovs.conf"}
}
