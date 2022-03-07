/*


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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NodeDeviceSpec defines the desired state of NodeDevice
type NodeDeviceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	NodeName     string         `json:"nodeName,omitempty"`
	DiskSelector []DiskSelector `json:"diskSelector,omitempty"`
}

//match Global csi-config to each node
type DiskSelector struct {
	Name   string   `json:"name,omitempty"`
	Re     []string `json:"re,omitempty"`
	Policy string   `json:"policy,omitempty"`
}

//device mananged type
type DeviceManage struct {
	VgGroups   []VgGroup    `json:"vgGroups,omitempty"`
	RawDevices []RawDevice  `json:"rawDevices,omitempty"`
	RAIDs      []RaidDevice `json:"raids,omitempty"`
}

//fuature
type RaidDevice struct {
}

//lvm
type VgGroup struct {
	VGName    string    `json:"vgName,omitempty"`
	PVName    string    `json:"pvName,omitempty"`
	PVCount   uint64    `json:"pvCount,omitempty"`
	LVCount   uint64    `json:"lvCount,omitempty"`
	SnapCount uint64    `json:"snapCount,omitempty"`
	VGAttr    string    `json:"vgAttr,omitempty"`
	VGSize    uint64    `json:"vgSize,omitempty"`
	VGFree    uint64    `json:"vgFree,omitempty"`
	PVS       []*PVInfo `json:"pvs,omitempty"`
}

// PVInfo pv详细信息
type PVInfo struct {
	PVName string `json:"pvName,omitempty"`
	VGName string `json:"vgName,omitempty"`
	PVFmt  string `json:"pvFmt,omitempty"`
	PVAttr string `json:"pvAttr,omitempty"`
	PVSize uint64 `json:"pvSize,omitempty"`
	PVFree uint64 `json:"pvFree,omitempty"`
}

//raw
type RawDevice struct {
	// Name      int64       `json:"name,omitempty"`
	// Size      int64       `json:"size,omitempty"`
	// Type      string      `json:"type,omitempty"`     //ssd hdd loop
	// RealPath  string      `json:"realPath,omitempty"` //dev/loop1
	// Major     uint32      `json:"major,omitempty"`
	// Minor     uint32      `json:"minor,omitempty"`
	// UUID      string      `json:"uuid,omitempty"`

	Name string `json:"name"`
	// mount point
	MountPoint string `json:"mountPoint"`
	// Size is the device capacity in byte
	Size uint64 `json:"size"`
	// status
	State string `json:"state"`
	// Type is disk type
	Type string `json:"type"`
	// 1 for hdd, 0 for ssd and nvme
	Rotational string `json:"rotational"`
	// ReadOnly is the boolean whether the device is readonly
	Readonly bool `json:"readOnly"`
	// Filesystem is the filesystem currently on the device
	Filesystem string `json:"filesystem"`
	// has used
	Used uint64 `json:"used"`
	// parent Name
	ParentName string      `json:"parentName"`
	Capacity   string      `json:"capacity,omitempty"`
	Available  string      `json:"available,omitempty"`
	Partition  []Partition `json:"partition,omitempty"`
	FreeSpace  []Partition `json:"freespace,omitempty"`
}

//raw partition
type Partition struct {
	Number     string `json:"number,omitempty"`
	Start      string `json:"start,omitempty"`
	End        string `json:"end,omitempty"`
	Size       string `json:"size,omitempty"`
	Filesystem string `json:"filesystem,omitempty"`
	Name       string `json:"name,omitempty"`
	Flags      string `json:"flags,omitempty"`
}

//status
type State string

const (
	Active   State = "Active"
	Inactive State = "Inactive"
	Unknown  State = "Unknown"
)

// NodeDeviceStatus defines the observed state of NodeDevice
type NodeDeviceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	NodeState State `json:"state"`
	//total rawdeivce,lvmdeivce,raiddevice capacity
	// DeviceRawSSD  = "carina-raw-ssd"
	// DeviceRawHDD  = "carina-raw-hdd"
	// DeviceRawLOOP = "carina-raw-loop"
	// DeviceRawAny  = "carina-raw-Any"
	// DeviceVGSSD = "carina-vg-ssd"
	// DeviceVGHDD = "carina-vg-hdd"
	//sample like this:
	// map[string]string{
	//     "carina-raw-ssd":  "100M",
	//     "carina-vg-hdd": "100M",
	//     "carina-raw-loop":   "1024M",
	// }
	Capacity map[string]string `json:"capacity,omitempty"`
	//total rawdeivce,lvmdeivce,raiddevice avaliable
	Available map[string]string `json:"available,omitempty"`
	//total device status
	DeviceManage DeviceManage `json:"deviceManage,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NodeDevice is the Schema for the nodedevices API
type NodeDevice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeDeviceSpec   `json:"spec,omitempty"`
	Status NodeDeviceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NodeDeviceList contains a list of NodeDevice
type NodeDeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeDevice `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeDevice{}, &NodeDeviceList{})
}
