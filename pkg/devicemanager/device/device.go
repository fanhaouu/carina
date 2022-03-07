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
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/carina-io/carina/pkg/devicemanager/types"
	"github.com/carina-io/carina/utils/exec"
	"github.com/carina-io/carina/utils/log"
)

type LocalDevice interface {
	// ListDevices list all devices available on a machine
	ListDevices() ([]string, error)
	ListDevicesDetail(device string) ([]*types.LocalDisk, error)
	GetDiskUsed(device string) (uint64, error)
	GetDiskInfo(device string) (map[string]string, error)
	//"bsd", "dvh", "gpt",  "loop","mac", "msdos", "pc98", or "sun"
	GetDiskPartitionType(device string) (string, error)
}

type LocalDeviceImplement struct {
	Executor exec.Executor
}

func (ld *LocalDeviceImplement) ListDevices() ([]string, error) {
	devices, err := ld.Executor.ExecuteCommandWithOutput("lsblk", "--all", "--noheadings", "--list", "--output", "KNAME")
	if err != nil {
		return nil, fmt.Errorf("failed to list all devices: %+v", err)
	}

	return strings.Split(devices, "\n"), nil
}

// ListDevicesDetail
/*
# lsblk --pairs --paths --bytes --all --output NAME,FSTYPE,MOUNTPOINT,SIZE,STATE,TYPE,ROTA,RO,PKNAME
NAME="/dev/sda" FSTYPE="" MOUNTPOINT="" SIZE="85899345920" STATE="running" TYPE="disk" ROTA="1" RO="0"
NAME="/dev/sda1" FSTYPE="ext4" MOUNTPOINT="/" SIZE="81604378624" STATE="" TYPE="part" ROTA="1" RO="0"
NAME="/dev/sda2" FSTYPE="" MOUNTPOINT="" SIZE="1024" STATE="" TYPE="part" ROTA="1" RO="0"
NAME="/dev/sda5" FSTYPE="swap" MOUNTPOINT="[SWAP]" SIZE="4291821568" STATE="" TYPE="part" ROTA="1" RO="0"
NAME="/dev/sdb" FSTYPE="" MOUNTPOINT="" SIZE="87926702080" STATE="running" TYPE="disk" ROTA="1" RO="0"
NAME="/dev/sr0" FSTYPE="iso9660" MOUNTPOINT="/media/ubuntu/VBox_GAs_6.1.16" SIZE="60987392" STATE="running" TYPE="rom" ROTA="1" RO="0"
NAME="/dev/loop0" FSTYPE="squashfs" MOUNTPOINT="/snap/core/10583" SIZE="102637568" STATE="" TYPE="loop" ROTA="1" RO="1"
NAME="/dev/loop1" FSTYPE="squashfs" MOUNTPOINT="/snap/core/9289" SIZE="101724160" STATE="" TYPE="loop" ROTA="1" RO="1"
NAME="/dev/loop2" FSTYPE="" MOUNTPOINT="" SIZE="" STATE="" TYPE="loop" ROTA="1" RO="0"
NAME="/dev/loop3" FSTYPE="" MOUNTPOINT="" SIZE="" STATE="" TYPE="loop" ROTA="1" RO="0"
NAME="/dev/loop4" FSTYPE="" MOUNTPOINT="" SIZE="" STATE="" TYPE="loop" ROTA="1" RO="0"
NAME="/dev/loop5" FSTYPE="" MOUNTPOINT="" SIZE="" STATE="" TYPE="loop" ROTA="1" RO="0"
NAME="/dev/loop6" FSTYPE="" MOUNTPOINT="" SIZE="" STATE="" TYPE="loop" ROTA="1" RO="0"
NAME="/dev/loop7" FSTYPE="" MOUNTPOINT="" SIZE="" STATE="" TYPE="loop" ROTA="1" RO="0"
*/
func (ld *LocalDeviceImplement) ListDevicesDetail(device string) ([]*types.LocalDisk, error) {
	args := []string{"--pairs", "--paths", "--bytes", "--all", "--output", "NAME,FSTYPE,MOUNTPOINT,SIZE,STATE,TYPE,ROTA,RO,PKNAME"}
	if device != "" {
		args = append(args, device)
	}
	devices, err := ld.Executor.ExecuteCommandWithOutput("lsblk", args...)
	if err != nil {
		log.Error("exec lsblk failed" + err.Error())
		return nil, err
	}

	return parseDiskString(devices), nil
}

// GetDiskUsed
/*
# df /dev/sda
文件系统         1K-块  已用    可用 已用% 挂载点
udev           8193452     0 8193452    0% /dev
*/
func (ld *LocalDeviceImplement) GetDiskUsed(device string) (uint64, error) {
	_, err := os.Stat(device)
	if err != nil {
		return 1, err
	}
	var stat syscall.Statfs_t
	syscall.Statfs(device, &stat)
	return stat.Blocks - stat.Bavail, nil
}

//Filesystem      Size  Used Avail Use% Mounted on
//none            3.9G     0  3.9G   0% /dev
func (ld *LocalDeviceImplement) GetDiskInfo(device string) (map[string]string, error) {
	var devicePath string
	splitDevicePath := strings.Split(device, "/")
	if len(splitDevicePath) == 1 {
		devicePath = fmt.Sprintf("/dev/%s", device) //device path for OSD on devices.
	} else {
		devicePath = device //use the exact device path (like /mnt/<pvc-name>) in case of PVC block device
	}
	output, err := ld.Executor.ExecuteCommandWithOutput("df", "-h", fmt.Sprintf("/dev/%s", devicePath))
	props := strings.Split(output, " ")
	propMap := make(map[string]string, len(props))
	if err != nil {
		log.Error("exec df -h " + fmt.Sprintf("/dev/%s", device) + err.Error())
		return propMap, err
	}
	for _, kvpRaw := range props {
		kvp := strings.Split(kvpRaw, " ")
		if len(kvp) == 2 {
			propMap[kvp[0]] = strings.Replace(kvp[1], `"`, "", -1)
		}
	}
	return propMap, nil
}

// GetDiskPartitionType look up parttion type GPT or MBR
func (ld *LocalDeviceImplement) GetDiskPartitionType(device string) (string, error) {

	output, err := ld.Executor.ExecuteCommandWithOutput("parted", "-s", fmt.Sprintf("/dev/%s", device), "p")
	if err != nil {
		log.Error("exec parted failed" + err.Error())
		return "", err
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Partition Table") {
			words := strings.Split(line, ":")
			return words[1], nil
		}
	}

	return "", fmt.Errorf("uuid not found for device %s. output=%s", device, output)
}

func parseDiskString(diskString string) []*types.LocalDisk {
	resp := []*types.LocalDisk{}

	if diskString == "" {
		return resp
	}

	diskString = strings.ReplaceAll(diskString, "\"", "")
	//diskString = strings.ReplaceAll(diskString, " ", "")

	vgsList := strings.Split(diskString, "\n")
	for _, vgs := range vgsList {
		tmp := types.LocalDisk{}
		vg := strings.Split(vgs, " ")
		for _, v := range vg {
			k := strings.Split(v, "=")

			switch k[0] {
			case "NAME":
				tmp.Name = k[1]
			case "MOUNTPOINT":
				tmp.MountPoint = k[1]
			case "SIZE":
				tmp.Size, _ = strconv.ParseUint(k[1], 10, 64)
			case "STATE":
				tmp.State = k[1]
			case "TYPE":
				tmp.Type = k[1]
			case "ROTA":
				tmp.Rotational = k[1]
			case "RO":
				if k[1] == "1" {
					tmp.Readonly = true
				} else {
					tmp.Readonly = false
				}
			case "FSTYPE":
				tmp.Filesystem = k[1]
			case "PKNAME":
				tmp.ParentName = k[1]
			default:
				log.Warnf("undefined filed %s-%s", k[0], k[1])
			}
		}
		resp = append(resp, &tmp)
	}
	return resp

}
