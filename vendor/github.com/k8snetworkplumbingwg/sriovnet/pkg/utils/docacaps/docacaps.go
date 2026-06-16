/*
Copyright 2026 NVIDIA CORPORATION & AFFILIATES

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

package docacaps

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	// DefaultDocaCapsBin is the default path to the doca_caps CLI tool.
	DefaultDocaCapsBin = "/opt/mellanox/doca/tools/doca_caps"
	// DocaCapsBinEnvVar is the environment variable that can be used to override the default path to the doca_caps CLI tool.
	DocaCapsBinEnvVar = "SRIOVNET_DOCA_CAPS_BIN"

	listRepDevsFlags  = "--list-rep-devs"
	pciPrefix         = "PCI:"
	representorPrefix = "representor-PCI:"

	indentLevelDevice      = 0
	indentLevelRepresentor = 2
	indentLevelAttributes  = 3

	docaCapsCmdTimeout = 5 * time.Second
)

// DocaCapRepDev represents a representor device as returned by doca_caps --list-rep-devs command.
type DocaCapRepDev struct {
	// ECPFPCIAddress is the PCI address of the ECPF device
	ECPFPCIAddress string
	// RepresentorPCIAddress is the PCI address of the represented device
	RepresentorPCIAddress string
	// Attributes is a map of attributes for the represented device
	Attributes map[string]string
}

// NewDocaCaps creates a new DOCACaps instance.
// it is a package-level variable to allow overriding in unit tests.
var NewDocaCaps func() DOCACaps = newDOCACaps

// DOCACaps is an interface that wraps docacaps functionality.
type DOCACaps interface {
	// GetDocaCapRepDevByVUID returns the single representor device whose vuid
	// attribute matches the provided value.
	GetDocaCapRepDevByVUID(vuid string) (*DocaCapRepDev, error)
}

// newDOCACaps creates a new DOCACaps instance.
func newDOCACaps() DOCACaps {
	return newDOCACapsInternal(defaultRunDocaCapsCmdFn)
}

// newDOCACapsInternal creates a new DOCACaps instance with supplied parameters.
// used mainly for testing.
func newDOCACapsInternal(runDocaCapsCmdFn func(args ...string) (string, error)) *docaCapsImpl {
	return &docaCapsImpl{
		runDocaCapsCmdFn: runDocaCapsCmdFn,
	}
}

// defaultRunDocaCapsCmdFn executes the doca_caps CLI tool with the supplied flags string.
func defaultRunDocaCapsCmdFn(args ...string) (string, error) {
	docaCapsBin := DefaultDocaCapsBin
	if altPath := os.Getenv(DocaCapsBinEnvVar); altPath != "" {
		docaCapsBin = altPath
	}

	cmdCtx, cancel := context.WithTimeout(context.Background(), docaCapsCmdTimeout)
	defer cancel()

	out, err := exec.CommandContext(cmdCtx, docaCapsBin, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run %q %q: %w (output: %s)", docaCapsBin, args, err, string(out))
	}
	return string(out), nil
}

// docaCapsImpl is the implementation of the DOCACaps interface.
type docaCapsImpl struct {
	runDocaCapsCmdFn func(args ...string) (string, error)
}

// parseDocaCapsRepDevs invokes `doca_caps --list-rep-devs` and parses its
// output into a slice of DocaCapRepDev entries.
//
// The expected output format is a series of top-level "PCI:" blocks. Each
// top-level block may contain zero or more nested "representor-PCI:" blocks,
// and each representor block contains a list of indented "key value" attribute
// lines. For example:
//
//	PCI: 0006:01:00.0
//	        representor-PCI: 0000:26:00.0
//	            pci_func_type                                 PF
//	            vuid                                          27f7781043874693bf26a22165715a32MLNXS0D0F0
//	            ...
//
// Parsing is driven by leading-space indentation, where one indent level is
// four spaces (tabs are not supported). Lines are classified as follows:
//
//   - indentLevelDevice(0): a "PCI:" line sets the current ECPF PCI address. Any other
//     top-level line clears the current ECPF, causing the following indented
//     lines to be ignored until the next "PCI:" line is seen.
//   - indentLevelRepresentor(2): a "representor-PCI:" line starts a new representor
//     entry under the current ECPF and is appended to the result. Any other
//     line at this indent level clears the current representor so that
//     subsequent attribute lines are ignored until the next "representor-PCI:".
//   - indentLevelAttributes(3): treated as a "key value" attribute attached to the
//     current representor entry. Lines that do not split into exactly two
//     whitespace-separated fields are ignored.
//   - All other indent levels and blank lines are silently skipped.
//
// The parser is intentionally tolerant: orphan representor or attribute lines
// that appear before any "PCI:" header (or before a representor, respectively)
// are dropped rather than treated as errors. An error is only returned if the
// underlying doca_caps invocation fails or the scanner reports a read error.
func (d *docaCapsImpl) parseDocaCapsRepDevs() ([]*DocaCapRepDev, error) {
	out, err := d.runDocaCapsCmdFn(listRepDevsFlags)
	if err != nil {
		return nil, err
	}

	var (
		devs        []*DocaCapRepDev
		currentECPF string
		currentDev  *DocaCapRepDev
	)

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()

		indentLevel := (len(line) - len(strings.TrimLeft(line, " "))) / 4

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		switch indentLevel {
		case indentLevelDevice:
			if strings.HasPrefix(trimmed, pciPrefix) {
				currentECPF = strings.TrimSpace(strings.TrimPrefix(trimmed, pciPrefix))
			} else {
				// skip non PCI prefixed top level devices
				currentECPF = ""
			}
			currentDev = nil

		case indentLevelRepresentor:
			if currentECPF == "" {
				continue
			}

			if !strings.HasPrefix(trimmed, representorPrefix) {
				// skip non-representor related fields at this indent level
				currentDev = nil
				continue
			}

			rep := strings.TrimSpace(strings.TrimPrefix(trimmed, representorPrefix))
			currentDev = &DocaCapRepDev{
				ECPFPCIAddress:        currentECPF,
				RepresentorPCIAddress: rep,
				Attributes:            make(map[string]string),
			}
			devs = append(devs, currentDev)

		case indentLevelAttributes:
			if currentECPF == "" || currentDev == nil {
				continue
			}

			fields := strings.Fields(trimmed)
			if len(fields) != 2 {
				continue
			}
			currentDev.Attributes[fields[0]] = fields[1]

		default:
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read doca_caps output: %w", err)
	}

	return devs, nil
}

// GetDocaCapRepDevByVUID returns the single representor device whose vuid
// attribute matches the provided value.
func (d *docaCapsImpl) GetDocaCapRepDevByVUID(vuid string) (*DocaCapRepDev, error) {
	devs, err := d.parseDocaCapsRepDevs()
	if err != nil {
		return nil, fmt.Errorf("failed to parse doca_caps rep devs: %w", err)
	}

	var matchedDevs []*DocaCapRepDev
	for _, dev := range devs {
		if dev.Attributes["vuid"] == vuid {
			matchedDevs = append(matchedDevs, dev)
		}
	}

	if len(matchedDevs) == 0 {
		return nil, fmt.Errorf("representor device with VUID %q not found", vuid)
	}

	if len(matchedDevs) > 1 {
		return nil, fmt.Errorf("multiple representor devices (%d) found with VUID %q", len(matchedDevs), vuid)
	}

	return matchedDevs[0], nil
}
