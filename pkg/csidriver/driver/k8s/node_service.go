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

package k8s

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/carina-io/carina/pkg/configuration"
	"github.com/carina-io/carina/pkg/csidriver/csi"
	"github.com/carina-io/carina/utils"
	"github.com/carina-io/carina/utils/log"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/rand"

	carinav1 "github.com/carina-io/carina/api/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// This annotation is present on K8s 1.11 release.
const annAlphaSelectedNode = "volume.alpha.kubernetes.io/selected-node"

type nodeService interface {
	getNodes(ctx context.Context) (*corev1.NodeList, error)
	getNodeDevices(ctx context.Context) (*carinav1.NodeDeviceList, error)
	// SelectVolumeNode 支持 volume size 及 topology match
	SelectVolumeNode(ctx context.Context, request int64, deviceGroup string, requirement *csi.TopologyRequirement) (string, string, map[string]string, error)
	SelectNodeDevice(ctx context.Context, request int64, deviceGroup string, requirement *csi.TopologyRequirement, exclusivityDisk bool) (string, string, map[string]string, error)
	GetCapacityByNodeName(ctx context.Context, nodeName, deviceGroup string) (int64, error)
	GetTotalCapacity(ctx context.Context, deviceGroup string, topology *csi.Topology) (int64, error)
	SelectDeviceGroup(ctx context.Context, request int64, nodeName string) (string, error)
	// HaveSelectedNode sc WaitForConsumer
	HaveSelectedNode(ctx context.Context, namespace, name string) (string, error)

	// SelectMultiVolumeNode multi volume node select
	SelectMultiVolumeNode(ctx context.Context, backendDeviceGroup, cacheDeviceGroup string, backendRequestGb, cacheRequestGb int64, requirement *csi.TopologyRequirement) (string, map[string]string, error)
}

// ErrNodeNotFound represents the error that node is not found.
var ErrNodeNotFound = errors.New("node not found")

// NodeService represents node service.
type NodeService struct {
	client.Client
}

// NewNodeService returns NodeService.
func NewNodeService(mgr manager.Manager) *NodeService {
	return &NodeService{Client: mgr.GetClient()}
}

func (s NodeService) getNodes(ctx context.Context) (*corev1.NodeList, error) {
	nl := new(corev1.NodeList)
	err := s.List(ctx, nl)
	if err != nil {
		return nil, err
	}
	return nl, nil
}

func (s NodeService) SelectVolumeNode(ctx context.Context, requestGb int64, deviceGroup string, requirement *csi.TopologyRequirement) (string, string, map[string]string, error) {
	// 在并发场景下，兼顾调度效率与调度公平，将pv分配到不同时间段
	time.Sleep(time.Duration(rand.Int63nRange(1, 30)) * time.Second)

	var nodeName, selectDeviceGroup string
	segments := map[string]string{}
	nl, err := s.getNodes(ctx)
	if err != nil {
		return "", "", segments, err
	}

	type paris struct {
		Key   string
		Value int64
	}

	preselectNode := []paris{}

	for _, node := range nl.Items {

		// topology selector
		// 若是sc配置了allowedTopologies，在此过滤出符合条件的node
		if requirement != nil {
			topologySelector := false
			for _, topo := range requirement.GetRequisite() {
				selector := labels.SelectorFromSet(topo.GetSegments())
				if selector.Matches(labels.Set(node.Labels)) {
					topologySelector = true
					break
				}
			}
			// 如果没有通过topology selector则节点不可用
			if !topologySelector {
				continue
			}
		}

		// capacity selector
		// 注册设备时有特殊前缀的，若是sc指定了设备组则过滤出所有节点上符合条件的设备组
		for key, value := range node.Status.Allocatable {

			if strings.HasPrefix(string(key), utils.DeviceCapacityKeyPrefix) {
				if deviceGroup != "" && string(key) != deviceGroup && string(key) != utils.DeviceCapacityKeyPrefix+deviceGroup {
					continue
				}
				if value.Value() < requestGb {
					continue
				}
				preselectNode = append(preselectNode, paris{
					Key:   node.Name + "-*-" + string(key),
					Value: value.Value(),
				})
			}
		}
	}
	if len(preselectNode) < 1 {
		return "", "", segments, ErrNodeNotFound
	}

	sort.Slice(preselectNode, func(i, j int) bool {
		return preselectNode[i].Value < preselectNode[j].Value
	})

	// 根据配置文件中设置算法进行节点选择
	if configuration.SchedulerStrategy() == configuration.SchedulerBinpack {
		nodeName = strings.Split(preselectNode[0].Key, "-*-")[0]
		selectDeviceGroup = strings.Split(preselectNode[0].Key, "/")[1]
	} else if configuration.SchedulerStrategy() == configuration.Schedulerspreadout {
		nodeName = strings.Split(preselectNode[len(preselectNode)-1].Key, "-*-")[0]
		selectDeviceGroup = strings.Split(preselectNode[len(preselectNode)-1].Key, "/")[1]
	} else {
		return "", "", segments, errors.New(fmt.Sprintf("no support scheduler strategy %s", configuration.SchedulerStrategy()))
	}

	// 获取选择节点的label
	for _, node := range nl.Items {
		if node.Name == nodeName {
			for _, topo := range requirement.GetRequisite() {
				for k, _ := range topo.GetSegments() {
					segments[k] = node.Labels[k]
				}
			}
		}
	}

	return nodeName, selectDeviceGroup, segments, nil
}

//if raw divice select node and select free parttion space
//裸盘情况下优先匹配有分区的，如果没有的话再匹配没有分区的裸盘
func (s NodeService) SelectNodeDevice(ctx context.Context, requestGb int64, deviceGroup string, requirement *csi.TopologyRequirement, exclusivityDisk bool) (string, string, map[string]string, error) {
	// 在并发场景下，兼顾调度效率与调度公平，将pv分配到不同时间段
	time.Sleep(time.Duration(rand.Int63nRange(1, 30)) * time.Second)
	var nodeName, selectDeviceGroup string
	segments := map[string]string{}
	type paris struct {
		Key   string
		Value int64
	}
	preselectNode := []paris{}
	nodedevicelist, err := s.getNodeDevices(ctx)
	if err != nil {
		return "", "", segments, err
	}
	for _, nodedevice := range nodedevicelist.Items {
		if nodedevice.Status.NodeState != carinav1.Active {
			continue
		}
		for key, value := range nodedevice.Status.Available {
			if !strings.Contains(key, utils.DeviceRaw) {
				continue
			}
			avaiableRaw, _ := strconv.ParseInt(value, 10, 64)
			if avaiableRaw < requestGb {
				continue
			}

			//匹配节点磁盘
			device, err := s.selectParttionOrRaw(nodedevice.Status.DeviceManage.RawDevices, requestGb, exclusivityDisk)
			if err != nil {
				return "", "", segments, err
			}
			size, _ := strconv.ParseInt(device.Available, 10, 64)
			preselectNode = append(preselectNode, paris{
				Key:   nodedevice.Name + "-" + device.Name,
				Value: size,
			})

		}

	}

	if len(preselectNode) < 1 {
		return "", "", segments, ErrNodeNotFound
	}

	sort.Slice(preselectNode, func(i, j int) bool {
		return preselectNode[i].Value < preselectNode[j].Value
	})

	// 根据配置文件中设置算法进行节点选择
	if configuration.SchedulerStrategy() == configuration.SchedulerBinpack {
		nodeName = strings.Split(preselectNode[0].Key, "-")[0]
		selectDeviceGroup = strings.Split(preselectNode[0].Key, "-")[1]
	} else if configuration.SchedulerStrategy() == configuration.Schedulerspreadout {
		nodeName = strings.Split(preselectNode[len(preselectNode)-1].Key, "-")[0]
		selectDeviceGroup = strings.Split(preselectNode[len(preselectNode)-1].Key, "-")[1]
	} else {
		return "", "", segments, errors.New(fmt.Sprintf("no support scheduler strategy %s", configuration.SchedulerStrategy()))
	}

	node := new(corev1.Node)
	err = s.Get(ctx, client.ObjectKey{Name: nodeName}, node)
	if err != nil {
		log.Error(err, "unable get node ")
		return "", "", segments, err
	}

	// 获取选择节点的label
	for _, topo := range requirement.GetRequisite() {
		for k, _ := range topo.GetSegments() {
			segments[k] = node.Labels[k]
		}
	}

	return nodeName, selectDeviceGroup, segments, nil
}

func (s NodeService) selectParttionOrRaw(rawdevices []carinav1.RawDevice, requestGb int64, exclusivityDisk bool) (deviceName carinav1.RawDevice, err error) {
	avaiableRawdevices := []carinav1.RawDevice{}
	avaiablePartitionDevices := []carinav1.RawDevice{}
	for _, device := range rawdevices {
		avaiableRaw, _ := strconv.ParseInt(device.Available, 10, 64)
		if avaiableRaw < requestGb {
			continue
		}

		//如果是独占磁盘，筛选没有分区的磁盘，满足不同的调度策略
		if exclusivityDisk {
			if len(device.Partition) > 1 {
				continue
			}
			size, _ := strconv.ParseInt(device.Available, 10, 64)
			if size < requestGb {
				continue
			}
			avaiableRawdevices = append(avaiableRawdevices, device)
		}

		//如果不是独占磁盘，优先筛选有分区的磁盘去满足调度策略，如果没有匹配则筛选空白磁盘满足调度策略
		if len(device.Partition) > 1 {
			tmpFreeParttionSlice := []int64{}
			for _, v := range device.FreeSpace {
				size, _ := strconv.ParseInt(v.Size, 10, 64)
				if size < requestGb {
					continue
				}
				tmpFreeParttionSlice = append(tmpFreeParttionSlice, size)
			}
			if len(tmpFreeParttionSlice) < 1 {
				log.Error(err, " this deive has no free space ", device.Name)
				continue
			}
			//有满足有分区的可用磁盘
			avaiablePartitionDevices = append(avaiablePartitionDevices, device)
		}
		//空白磁盘
		size, _ := strconv.ParseInt(device.Available, 10, 64)
		if size < requestGb {
			continue
		}
		avaiableRawdevices = append(avaiableRawdevices, device)
	}

	switch {
	case exclusivityDisk == true && len(avaiableRawdevices) < 1:
		log.Error(err, " has not match avaiable device for  exclusivity pod")
		return deviceName, errors.New(fmt.Sprintf("no support exclusivity scheduler strategy %s", configuration.SchedulerStrategy()))
	case exclusivityDisk == true && len(avaiableRawdevices) > 1:
		//有可用独占磁盘
		sort.Slice(avaiableRawdevices, func(i, j int) bool {
			return avaiableRawdevices[i].Available < avaiableRawdevices[j].Available
		})
		// 根据配置文件中设置算法进行节点选择最小适配还是最大优选
		if configuration.SchedulerStrategy() == configuration.SchedulerBinpack {
			deviceName = avaiableRawdevices[0]
		} else if configuration.SchedulerStrategy() == configuration.Schedulerspreadout {
			deviceName = avaiableRawdevices[len(avaiableRawdevices)-1]
		} else {
			deviceName = avaiableRawdevices[0]
		}
		return deviceName, nil

	case exclusivityDisk == false && len(avaiablePartitionDevices) < 1 && len(avaiableRawdevices) < 1:
		//无可用共享磁盘
		log.Error(err, " has not match avaiable device for  pod ")
		return deviceName, errors.New(fmt.Sprintf("no support  scheduler strategy %s", configuration.SchedulerStrategy()))
	case exclusivityDisk == false && len(avaiablePartitionDevices) < 1 && len(avaiableRawdevices) > 1:
		//有可用共享磁盘使用无分区裸盘
		sort.Slice(avaiableRawdevices, func(i, j int) bool {
			return avaiableRawdevices[i].Available < avaiableRawdevices[j].Available
		})
		// 根据配置文件中设置算法进行节点选择最小适配还是最大优选
		if configuration.SchedulerStrategy() == configuration.SchedulerBinpack {
			deviceName = avaiableRawdevices[0]
		} else if configuration.SchedulerStrategy() == configuration.Schedulerspreadout {
			deviceName = avaiableRawdevices[len(avaiableRawdevices)-1]
		} else {
			deviceName = avaiableRawdevices[0]
		}
		return deviceName, nil
	case exclusivityDisk == false && len(avaiablePartitionDevices) > 1:
		//有可用共享磁盘使用有分区磁盘；
		sort.Slice(avaiablePartitionDevices, func(i, j int) bool {
			return avaiablePartitionDevices[i].Available < avaiablePartitionDevices[j].Available
		})
		if configuration.SchedulerStrategy() == configuration.SchedulerBinpack {
			deviceName = avaiablePartitionDevices[0]
		} else if configuration.SchedulerStrategy() == configuration.Schedulerspreadout {
			deviceName = avaiablePartitionDevices[len(avaiablePartitionDevices)-1]
		} else {
			deviceName = avaiablePartitionDevices[0]
		}
		return deviceName, nil
	default:
		log.Error(err, " has not match avaiable device")
		return deviceName, errors.New(fmt.Sprintf("has not match avaiable device %s", configuration.SchedulerStrategy()))
	}

}
func (s NodeService) getNodeDevices(ctx context.Context) (*carinav1.NodeDeviceList, error) {
	nodeDeviceList := new(carinav1.NodeDeviceList)
	err := s.List(ctx, nodeDeviceList)
	if err != nil {
		return nodeDeviceList, err
	}
	return nodeDeviceList, nil
}

// GetCapacityByNodeName returns VG capacity of specified node by name.
func (s NodeService) GetCapacityByNodeName(ctx context.Context, name, deviceGroup string) (int64, error) {
	node := new(corev1.Node)
	err := s.Get(ctx, client.ObjectKey{Name: name}, node)
	if err != nil {
		return 0, err
	}

	for key, v := range node.Status.Allocatable {
		if string(key) == deviceGroup || string(key) == utils.DeviceCapacityKeyPrefix+deviceGroup {
			return v.Value(), nil
		}
	}
	return 0, errors.New("device group not found")
}

// GetTotalCapacity returns total VG capacity of all nodes.
func (s NodeService) GetTotalCapacity(ctx context.Context, deviceGroup string, topology *csi.Topology) (int64, error) {
	nl, err := s.getNodes(ctx)
	if err != nil {
		return 0, err
	}

	capacity := int64(0)
	for _, node := range nl.Items {
		// topology selector
		if topology != nil {
			selector := labels.SelectorFromSet(topology.GetSegments())
			if !selector.Matches(labels.Set(node.Labels)) {
				continue
			}
		}

		for key, v := range node.Status.Capacity {

			if deviceGroup == "" && strings.HasPrefix(string(key), utils.DeviceCapacityKeyPrefix) {
				capacity += v.Value()
			} else if string(key) == deviceGroup || string(key) == utils.DeviceCapacityKeyPrefix+deviceGroup {
				capacity += v.Value()
			}
		}
	}
	return capacity, nil
}

func (s NodeService) SelectDeviceGroup(ctx context.Context, request int64, nodeName string) (string, error) {
	var selectDeviceGroup string

	nl, err := s.getNodes(ctx)
	if err != nil {
		return "", err
	}

	type paris struct {
		Key   string
		Value int64
	}

	preselectNode := []paris{}

	for _, node := range nl.Items {
		if nodeName != node.Name {
			continue
		}
		// capacity selector
		// 经过上层过滤，这里只会有一个节点
		for key, value := range node.Status.Allocatable {
			if strings.HasPrefix(string(key), utils.DeviceCapacityKeyPrefix) {
				preselectNode = append(preselectNode, paris{
					Key:   string(key),
					Value: value.Value(),
				})
			}
		}
	}
	if len(preselectNode) < 1 {
		return "", ErrNodeNotFound
	}

	sort.Slice(preselectNode, func(i, j int) bool {
		return preselectNode[i].Value < preselectNode[j].Value
	})
	// 这里只能选最小满足的，因为可能存在一个pod多个pv都需要落在这个节点
	for _, p := range preselectNode {
		if p.Value >= request {
			selectDeviceGroup = strings.Split(p.Key, "/")[1]
		}
	}
	return selectDeviceGroup, nil
}

func (s NodeService) HaveSelectedNode(ctx context.Context, namespace, name string) (string, error) {
	node := ""
	pvc := new(corev1.PersistentVolumeClaim)
	err := s.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, pvc)
	if err != nil {
		return node, err
	}
	node = pvc.Annotations[utils.AnnSelectedNode]
	if node == "" {
		node = pvc.Annotations[annAlphaSelectedNode]
	}

	return node, nil
}

func (s NodeService) SelectMultiVolumeNode(ctx context.Context, backendDeviceGroup, cacheDeviceGroup string, backendRequestGb, cacheRequestGb int64, requirement *csi.TopologyRequirement) (string, map[string]string, error) {
	// 在并发场景下，兼顾调度效率与调度公平，将pv分配到不同时间段
	time.Sleep(time.Duration(rand.Int63nRange(1, 30)) * time.Second)

	var nodeName string
	segments := map[string]string{}
	nl, err := s.getNodes(ctx)
	if err != nil {
		return "", segments, err
	}

	type paris struct {
		Key   string
		Value int64
	}

	preselectNode := []paris{}

	for _, node := range nl.Items {

		// topology selector
		// 若是sc配置了allowedTopologies，在此过滤出符合条件的node
		if requirement != nil {
			topologySelector := false
			for _, topo := range requirement.GetRequisite() {
				selector := labels.SelectorFromSet(topo.GetSegments())
				if selector.Matches(labels.Set(node.Labels)) {
					topologySelector = true
					break
				}
			}
			// 如果没有通过topology selector则节点不可用
			if !topologySelector {
				continue
			}
		}

		// capacity selector
		// 注册设备时有特殊前缀的，若是sc指定了设备组则过滤出所有节点上符合条件的设备组
		backendFilter := int64(0)
		cacheFileter := int64(0)
		for key, value := range node.Status.Allocatable {

			if strings.HasPrefix(string(key), utils.DeviceCapacityKeyPrefix) {
				if strings.Contains(string(key), backendDeviceGroup) {
					if value.Value() >= backendRequestGb {
						backendFilter = value.Value()
					}
				}
				if strings.Contains(string(key), cacheDeviceGroup) {
					if value.Value() >= cacheRequestGb {
						cacheFileter = value.Value()
					}
				}
			}
		}

		if backendFilter > 0 && cacheFileter >= 0 {
			preselectNode = append(preselectNode, paris{
				Key:   node.Name,
				Value: backendFilter,
			})
		}
	}
	if len(preselectNode) < 1 {
		return "", segments, ErrNodeNotFound
	}

	sort.Slice(preselectNode, func(i, j int) bool {
		return preselectNode[i].Value < preselectNode[j].Value
	})

	// 根据配置文件中设置算法进行节点选择
	if configuration.SchedulerStrategy() == configuration.SchedulerBinpack {
		nodeName = preselectNode[0].Key
	} else if configuration.SchedulerStrategy() == configuration.Schedulerspreadout {
		nodeName = preselectNode[len(preselectNode)-1].Key
	} else {
		return "", segments, errors.New(fmt.Sprintf("no support scheduler strategy %s", configuration.SchedulerStrategy()))
	}

	// 获取选择节点的label
	for _, node := range nl.Items {
		if node.Name == nodeName {
			for _, topo := range requirement.GetRequisite() {
				for k, _ := range topo.GetSegments() {
					segments[k] = node.Labels[k]
				}
			}
		}
	}

	return nodeName, segments, nil
}
