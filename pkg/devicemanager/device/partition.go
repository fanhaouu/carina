/*
   Copyright @ 2021 bocloud <fushaosong@beyondcent.com>.

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

package device

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/carina-io/carina/utils/exec"
	"github.com/carina-io/carina/utils/log"
)

const (
	// DiskType is a disk type
	DiskType = "disk"
	// SSDType is an sdd type
	SSDType = "ssd"
	// PartType is a partition type
	PartType = "part"
	// LVMType is an LVM type
	LVMType = "lvm"
)

// Partition represents a partition metadata
type Partition struct {
	Number     string
	Start      string
	End        string
	Size       string
	Filesystem string
	Name       string
	Flags      string
}

type LocalPartition interface {
	AddDevicePartition(device string) (partitions []Partition, unusedSpace uint64, err error)
	DelDevicePartition(device, partition string) (partitions []Partition, unusedSpace uint64, err error)
	GetDevicePartitions(device string) (partitions []Partition, err error)
	GetDeviceUnUsePartitions(device string) (partitions []Partition, unusedSpace uint64, err error)
	IsPartType(devicePath string) (bool, error)
	GetUdevInfo(device string) (map[string]string, error)
}

type LocalPartitionImplement struct {
	LocalDeviceImplement
	Executor exec.Executor
}

// add partition to give device
func (ld *LocalPartitionImplement) AddDevicePartition(device string, name, start, end string) (partition Partition, err error) {
	parttiontype, err := ld.LocalDeviceImplement.GetDiskPartitionType(device)
	if err != nil {
		return partition, err
	}
	if parttiontype == " " || parttiontype == "unknown" {
		//rebuild parttion
		_, err := ld.Executor.ExecuteCommandWithOutput("parted", "-s", fmt.Sprintf("/dev/%s", device), "mklable", "gpt")
		if err != nil {
			return partition, err
		}
	}
	_, err = ld.Executor.ExecuteCommandWithOutput("parted", "-s", fmt.Sprintf("/dev/%s", device), "mkpart", name, start, end)
	if err != nil {
		log.Error("exec parted -s", fmt.Sprintf("/dev/%s", device), "mkpart", name, start, end, "failed"+err.Error())
		return partition, err
	}
	output, err := ld.Executor.ExecuteCommandWithOutput("parted", "-s", fmt.Sprintf("/dev/%s", device), "p")
	if err != nil {
		return partition, err
	}

	partitionString := strings.ReplaceAll(output, "\"", "")
	partitionsList := strings.Split(partitionString, "\n")
	locationNum := 0

	for i, partitions := range partitionsList {

		if strings.Contains(partitions, "Number") {
			locationNum = i
		}
		if locationNum == 0 || i <= locationNum {
			continue
		}
		log.Infof("found partition in line %s", i)
		tmp := strings.Split(partitions, " ")
		partition.Number = tmp[0]
		partition.Start = tmp[1]
		partition.End = tmp[2]
		partition.Size = tmp[3]
		partition.Filesystem = tmp[4]
		partition.Name = tmp[5]
		partition.Flags = tmp[6]
		if partition.Name == name {
			return partition, nil
		}

	}

	return partition, nil
}

// delete a partition on a given device
func (ld *LocalPartitionImplement) DelDevicePartition(device, partitionNumber string) (bool, error) {
	_, err := ld.Executor.ExecuteCommandWithOutput("parted", fmt.Sprintf("/dev/%s", device), "rm", partitionNumber)
	if err != nil {
		log.Error("exec parted -s", fmt.Sprintf("/dev/%s", device), "rm", partitionNumber, "failed"+err.Error())
		return false, err
	}

	return true, nil
}

// GetDevicePartitions gets partitions on a given device
func (ld *LocalPartitionImplement) GetDevicePartitions(device string) (partitions []Partition, err error) {

	var devicePath string
	splitDevicePath := strings.Split(device, "/")
	if len(splitDevicePath) == 1 {
		devicePath = fmt.Sprintf("/dev/%s", device) //device path for OSD on devices.
	} else {
		devicePath = device //use the exact device path (like /mnt/<pvc-name>) in case of PVC block device
	}

	output, err := ld.Executor.ExecuteCommandWithOutput("parted", "-s", fmt.Sprintf("/dev/%s", devicePath), "p")
	log.Infof("Output: %+v", output)
	if err != nil {
		return partitions, fmt.Errorf("failed to get device %s partitions. %+v", device, err)
	}
	partitions = parsePartitionString(output)
	return partitions, nil
}

//
func (ld *LocalPartitionImplement) GetDeviceUnUsePartitions(device string) (partitions []Partition, unusedSpace uint64, err error) {
	var devicePath string
	splitDevicePath := strings.Split(device, "/")
	if len(splitDevicePath) == 1 {
		devicePath = fmt.Sprintf("/dev/%s", device) //device path for OSD on devices.
	} else {
		devicePath = device //use the exact device path (like /mnt/<pvc-name>) in case of PVC block device
	}

	output, err := ld.Executor.ExecuteCommandWithOutput("parted", "-s", fmt.Sprintf("/dev/%s", devicePath), "p", "free")
	log.Infof("Output: %+v", output)
	if err != nil {
		return partitions, 0, fmt.Errorf("failed to get device %s partitions. %+v", device, err)
	}
	partitions, unusedSpace = parsePartitionUnUseString(output)
	return partitions, unusedSpace, nil
}

// GetUdevInfo gets udev information
func (ld *LocalPartitionImplement) GetUdevInfo(device string) (map[string]string, error) {
	output, err := ld.Executor.ExecuteCommandWithOutput("udevadm", "info", "--query=property", fmt.Sprintf("/dev/%s", device))
	if err != nil {
		return nil, err
	}

	return parseUdevInfo(output), nil
}

// IsPartType returns if a device is owned by lvm or partition
func (ld *LocalPartitionImplement) IsPartType(device string) (bool, error) {
	devProps, err := ld.LocalDeviceImplement.ListDevicesDetail(device)
	if err != nil {
		return false, fmt.Errorf("failed to get device properties for %q: %+v", device, err)
	}
	return devProps[0].Type == PartType, nil
}

func parsePartitionString(partitionString string) []Partition {
	resp := []Partition{}

	if partitionString == "" {
		return resp
	}
	partitionString = strings.ReplaceAll(partitionString, "\"", "")
	partitionsList := strings.Split(partitionString, "\n")
	locationNum := 0
	for i, partitions := range partitionsList {
		partition := Partition{}
		if strings.Contains(partitions, "Number") {
			locationNum = i
		}
		if locationNum == 0 || i <= locationNum {
			continue
		}
		log.Infof("found partition in line %s", i)
		tmp := strings.Split(partitions, " ")
		partition.Number = tmp[0]
		partition.Start = tmp[1]
		partition.End = tmp[2]
		partition.Size = tmp[3]
		partition.Filesystem = tmp[4]
		partition.Name = tmp[5]
		partition.Flags = tmp[6]
		resp = append(resp, partition)

	}
	return resp

}

func parsePartitionUnUseString(partitionString string) (partitions []Partition, unusedSpace uint64) {
	resp := []Partition{}
	if partitionString == "" {
		return resp, 0
	}
	partitionString = strings.ReplaceAll(partitionString, "\"", "")
	partitionsList := strings.Split(partitionString, "\n")
	for i, partitions := range partitionsList {
		partition := Partition{}
		if !strings.Contains(partitions, "Free Space") {
			continue
		}

		log.Infof("found partition Free Space in line %s %s", i, partitions)
		tmp := strings.Split(partitions, " ")
		partition.Number = tmp[0]
		partition.Start = tmp[1]
		partition.End = tmp[2]
		partition.Size = tmp[3]
		partition.Filesystem = tmp[4]
		partition.Name = tmp[5]
		partition.Flags = tmp[6]
		resp = append(resp, partition)
		size, _ := strconv.Atoi(partition.Size)
		unusedSpace += uint64(size)

	}
	return resp, unusedSpace

}

func parseUdevInfo(output string) map[string]string {
	lines := strings.Split(output, "\n")
	result := make(map[string]string, len(lines))
	for _, v := range lines {
		pairs := strings.Split(v, "=")
		if len(pairs) > 1 {
			result[pairs[0]] = pairs[1]
		}
	}
	return result
}
