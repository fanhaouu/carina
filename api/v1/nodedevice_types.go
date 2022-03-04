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
	NodeName     string       `json:"nodeName,omitempty"`
	DeviceManage DeviceManage `json:"deviceManage,omitempty"`
}

type DeviceManage struct {
	Lvms []VgGroup `json:"vggroup,omitempty"`
	// BlacklistMountPoints defines the user specified mount points which are not allowed for scheduling
	RawDevices []RawDevice `json:"rawdevice,omitempty"`
}

type VgGroup struct {
	VGName    string    `json:"vgName"`
	PVName    string    `json:"pvName"`
	PVCount   uint64    `json:"pvCount"`
	LVCount   uint64    `json:"lvCount"`
	SnapCount uint64    `json:"snapCount"`
	VGAttr    string    `json:"vgAttr"`
	VGSize    uint64    `json:"vgSize"`
	VGFree    uint64    `json:"vgFree"`
	PVS       []*PVInfo `json:"pvs"`
}

// PVInfo pv详细信息
type PVInfo struct {
	PVName string `json:"pvName"`
	VGName string `json:"vgName"`
	PVFmt  string `json:"pvFmt"`
	PVAttr string `json:"pvAttr"`
	PVSize uint64 `json:"pvSize"`
	PVFree uint64 `json:"pvFree"`
}

type RawDevice struct {
	Name      int64       `json:"name"`
	Size      int64       `json:"size"`
	Type      string      `json:"type"`     //ssd hdd loop
	RealPath  string      `json:"realPath"` //dev/loop1
	Major     uint32      `json:"major"`
	Minor     uint32      `json:"minor"`
	UUID      string      `json:"uuid"`
	Capacity  string      `json:"capacity"`
	Available string      `json:"available"`
	Partition []Partition `json:partition`
	FreeSpace []Partition `json:freespace`
}

type Partition struct {
	Number     string
	Start      string
	End        string
	Size       string
	Filesystem string
	Name       string
	Flags      string
}

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
	Capacity map[string]string `json:"capacity"`
	//total rawdeivce,lvmdeivce,raiddevice avaliable
	Available    map[string]string `json:"available"`
	DeviceStatus []DeviceStatus    `json:"state"`
}

type DeviceStatus struct {
	State      State     `json:"state"`
	ManageType string    `json:"managetype"` //lvm || raw
	RawDevice  RawDevice `json：rawDevice`
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
