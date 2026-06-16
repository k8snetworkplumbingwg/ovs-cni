/*
Copyright 2023 NVIDIA CORPORATION & AFFILIATES

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

package sriovnet

import (
	"fmt"
	"log"
	"path/filepath"

	utilfs "github.com/k8snetworkplumbingwg/sriovnet/pkg/utils/filesystem"
)

const (
	NetSysDir        = "/sys/class/net"
	PciSysDir        = "/sys/bus/pci/devices"
	AuxSysDir        = "/sys/bus/auxiliary/devices"
	pcidevPrefix     = "device"
	netdevDriverDir  = "device/driver"
	netdevUnbindFile = "unbind"
	netdevBindFile   = "bind"

	netDevMaxVfCountFile     = "sriov_totalvfs"
	netDevCurrentVfCountFile = "sriov_numvfs"
	netDevVfDevicePrefix     = "virtfn"
)

type VfObject struct {
	NetdevName string
	PCIDevName string
}

func netDevDeviceDir(netDevName string) string {
	devDirName := filepath.Join(NetSysDir, netDevName, pcidevPrefix)
	return devDirName
}

func getMaxVfCount(pfNetdevName string) (int, error) {
	devDirName := netDevDeviceDir(pfNetdevName)

	maxDevFile := fileObject{
		Path: filepath.Join(devDirName, netDevMaxVfCountFile),
	}

	maxVfs, err := maxDevFile.ReadInt()
	if err != nil {
		return 0, err
	}
	log.Println("max_vfs = ", maxVfs)
	return maxVfs, nil
}

func setMaxVfCount(pfNetdevName string, maxVfs int) error {
	devDirName := netDevDeviceDir(pfNetdevName)

	maxDevFile := fileObject{
		Path: filepath.Join(devDirName, netDevCurrentVfCountFile),
	}

	return maxDevFile.WriteInt(maxVfs)
}

func getCurrentVfCount(pfNetdevName string) (int, error) {
	devDirName := netDevDeviceDir(pfNetdevName)

	maxDevFile := fileObject{
		Path: filepath.Join(devDirName, netDevCurrentVfCountFile),
	}

	curVfs, err := maxDevFile.ReadInt()
	if err != nil {
		return 0, err
	}
	log.Println("cur_vfs = ", curVfs)
	return curVfs, nil
}

func vfNetdevNameFromParent(pfNetdevName string, vfIndex int) string {
	devDirName := netDevDeviceDir(pfNetdevName)
	vfNetdev, _ := lsFilesWithPrefix(fmt.Sprintf("%s/%s%v/net", devDirName,
		netDevVfDevicePrefix, vfIndex), "", false)
	if len(vfNetdev) == 0 {
		return ""
	}
	return vfNetdev[0]
}

func getPCIFromDeviceName(netdevName string) (string, error) {
	symbolicLink := filepath.Join(NetSysDir, netdevName, pcidevPrefix)
	pciDevDir, err := utilfs.Fs.Readlink(symbolicLink)
	if err != nil {
		return "", fmt.Errorf("failed to read symbolic link %s: %v", symbolicLink, err)
	}
	pciAddress := filepath.Base(pciDevDir)
	return pciAddress, nil
}

func vfPCIDevNameFromVfIndex(pfNetdevName string, vfIndex int) (string, error) {
	symbolicLink := filepath.Join(NetSysDir, pfNetdevName, pcidevPrefix, fmt.Sprintf("%s%v",
		netDevVfDevicePrefix, vfIndex))
	pciDevDir, err := utilfs.Fs.Readlink(symbolicLink)
	if err != nil {
		return "", fmt.Errorf("failed to read symbolic link %s: %v", symbolicLink, err)
	}
	pciAddress := filepath.Base(pciDevDir)
	return pciAddress, nil
}

func GetVfPciDevList(pfNetdevName string) ([]string, error) {
	var i int
	devDirName := netDevDeviceDir(pfNetdevName)

	virtFnDirs, err := lsFilesWithPrefix(devDirName, netDevVfDevicePrefix, true)

	if err != nil {
		return nil, err
	}

	i = 0
	vfDirList := make([]string, 0, len(virtFnDirs))
	for _, vfDir := range virtFnDirs {
		vfDirList = append(vfDirList, vfDir)
		i++
	}
	return vfDirList, nil
}
