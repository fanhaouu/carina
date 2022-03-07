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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	carinav1 "github.com/carina-io/carina/api/v1"
	"github.com/carina-io/carina/pkg/configuration"
	deviceManager "github.com/carina-io/carina/pkg/devicemanager"
	"github.com/carina-io/carina/utils"
	"github.com/carina-io/carina/utils/log"
)

// NodeDeviceReconciler reconciles a NodeDevice object
type NodeDeviceReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	NodeName      string
	DeviceManager deviceManager.DeviceManager
}

//+kubebuilder:rbac:groups=carina.storage.io,resources=nodedevices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=carina.storage.io,resources=nodedevices/status,verbs=get;update;patch

func (r *NodeDeviceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Infof("nodeDevice %s reconcile manager...", req.Name)
	globalConfigMap := configuration.DiskConfig.DiskSelectors
	nodeName := os.Getenv("NODE_NAME")
	node := new(corev1.Node)
	err := r.Get(ctx, client.ObjectKey{Name: nodeName}, node)
	if err != nil {
		log.Error(err, "unable get node ")
		return ctrl.Result{}, err
	}

	DiskSelector := []carinav1.DiskSelector{}
	for _, v := range globalConfigMap {
		if len(v.NodeLabel) == 0 {
			DiskSelector = append(DiskSelector, carinav1.DiskSelector{
				Name:   v.Name,
				Re:     v.Re,
				Policy: v.Policy,
			})
			continue
		}
		if _, ok := node.Labels[v.NodeLabel]; ok {
			DiskSelector = append(DiskSelector, carinav1.DiskSelector{
				Name:   v.Name,
				Re:     v.Re,
				Policy: v.Policy,
			})
			continue
		}
	}
	// your logic here
	nodeDevice := &carinav1.NodeDevice{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: nodeName}, nodeDevice); err != nil {
		if !apierrs.IsNotFound(err) {
			log.Error(err, "unable to fetch NodeDevice ")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if nodeDevice.Spec.NodeName != nodeName {
		log.Info("unfiltered logic value nodeName ", nodeDevice.Spec.NodeName)
		return ctrl.Result{}, nil
	}

	if nodeDevice.ObjectMeta.DeletionTimestamp != nil {
		if utils.ContainsString(nodeDevice.Finalizers, utils.NodeDeviceFinalizer) {
			nodeDevice.Finalizers = utils.SliceRemoveString(nodeDevice.Finalizers, utils.NodeDeviceFinalizer)
			patch := client.MergeFrom(nodeDevice)
			if err := r.Patch(ctx, nodeDevice, patch); err != nil {
				log.Error(err, " failed to remove finalizer name ", nodeDevice.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, nil
	}

	// finalization
	if !utils.ContainsString(nodeDevice.Finalizers, utils.NodeDeviceFinalizer) {
		lnodeDeviceCopy := nodeDevice.DeepCopy()
		lnodeDeviceCopy.Finalizers = utils.SliceRemoveString(lnodeDeviceCopy.Finalizers, utils.LogicVolumeFinalizer)
		patch := client.MergeFrom(nodeDevice)
		if err := r.Patch(ctx, lnodeDeviceCopy, patch); err != nil {
			log.Error(err, " failed to add finalizer name ", nodeDevice.Name)
			return ctrl.Result{}, err
		}
		// Our finalizer has finished, so the reconciler can do nothing.
		return ctrl.Result{}, nil
	}

	log.Info("start finalizing Nodedevice name ", nodeDevice.Name)

	//discover raw device
	if !reflect.DeepEqual(nodeDevice.Spec.DiskSelector, DiskSelector) {
		nodeDevice.Spec.DiskSelector = DiskSelector
		patch := client.MergeFrom(nodeDevice)
		if err := r.Patch(ctx, nodeDevice, patch); err != nil {
			log.Error(err, " failed to update nodeDevice name ", nodeDevice.Name)
			return ctrl.Result{}, err
		}
	}
	//update status
	listDevicess, err := r.DeviceManager.DiskManager.ListDevicesDetail("")
	if err != nil {
		log.Error(err, " failed to list node devices ", nodeDevice.Name)
		return ctrl.Result{}, err
	}
	var capacity, allocatable int64
	tmpCapacity, tmpAvalibel := map[string]string{}, map[string]string{}
	for _, device := range listDevicess {
		for _, rg := range DiskSelector {
			diskSelector, err := regexp.Compile(strings.Join(rg.Re, "|"))
			if err != nil {
				log.Warnf("disk regex %s error %v ", strings.Join(rg.Re, "|"), err)
				return ctrl.Result{}, err
			}
			if diskSelector.MatchString(device.Name) {
				if strings.ToLower(rg.Policy) == "raw" {
					rawDevicesItem := new(carinav1.RawDevice)
					tmp, err := json.Marshal(device)
					if err != nil {
						log.Infof("failed to marshal  %s,%+v", device.Name, err)
					}
					json.Unmarshal(tmp, &rawDevicesItem)
					//check device healthz
					deviceinfo, err := r.DeviceManager.DiskManager.GetDiskInfo(device.Name)
					if err != nil {
						log.Errorf("failed to check device healthz: $s,%+v", device.Name, err)
						rawDevicesItem.State = string(carinav1.Inactive)
						continue
					}
					rawDevicesItem.Capacity = deviceinfo["Size"]
					rawDevicesItem.Available = deviceinfo["Avail"]

					//get partition info
					rawDevicesItem.Partition, err = r.DeviceManager.PartitionManager.GetDevicePartitions(device.Name)
					if err != nil {
						log.Errorf("failed to list devices Partition: $s,%+v", device.Name, err)
						continue
					}
					freeSpace, _, _ := r.DeviceManager.PartitionManager.GetDeviceUnUsePartitions(device.Name)
					rawDevicesItem.FreeSpace = freeSpace

					size, _ := strconv.ParseInt(rawDevicesItem.Capacity, 10, 64)
					capacity += size
					tmpCapacity[utils.DeviceRaw+rawDevicesItem.Name] = fmt.Sprintf("%d", capacity)
					avail, _ := strconv.ParseInt(rawDevicesItem.Available, 10, 64)
					allocatable += avail
					tmpAvalibel[utils.DeviceRaw+rawDevicesItem.Name] = fmt.Sprintf("%d", allocatable)
					//add or update nodeDevice.Status.DeviceManage.RawDevices
					tmpSlice := []string{}
					for _, rawDevice := range nodeDevice.Status.DeviceManage.RawDevices {
						tmpSlice = append(tmpSlice, rawDevice.Name)
					}
					if !utils.ContainsString(tmpSlice, rawDevicesItem.Name) {
						nodeDevice.Status.DeviceManage.RawDevices = append(nodeDevice.Status.DeviceManage.RawDevices, *rawDevicesItem)
						continue
					}
					for k, rawDevice := range nodeDevice.Status.DeviceManage.RawDevices {
						if rawDevice.Name == rawDevicesItem.Name && !reflect.DeepEqual(rawDevice, *rawDevicesItem) {
							nodeDevice.Status.DeviceManage.RawDevices[k] = *rawDevicesItem
						}
					}

				}
				if strings.ToLower(rg.Policy) == "lvm" {
					vgGroupItem := new(carinav1.VgGroup)
					vgs, err := r.DeviceManager.VolumeManager.GetCurrentVgStruct()
					if err != nil {
						log.Error(err, " failed to get current vgs on node name ", nodeDevice.Name)
					}
					for _, vg := range vgs {
						if rg.Name == vg.PVName {
							tmp, err := json.Marshal(vg)
							if err != nil {
								log.Infof("failed to marshal vg  %s,%+v", vg.VGName, err)
							}
							json.Unmarshal(tmp, &vgGroupItem)
							//get VgGroup info

							nodeDevice.Status.DeviceManage.VgGroups = append(nodeDevice.Status.DeviceManage.VgGroups, *vgGroupItem)
						}
					}
				}
			}
		}
	}
	nodeDevice.Annotations = node.Annotations
	nodeDevice.Labels = node.Labels
	nodeDevice.Status.NodeState = carinav1.Active
	nodeDevice.Status.Capacity = tmpCapacity
	nodeDevice.Status.Available = tmpAvalibel
	//total nodedevice size and total nodedevice use
	if err := r.Status().Update(ctx, nodeDevice); err != nil {
		log.Error(err, " failed to update nodeDevice status name ", nodeDevice.Name)
	}

	return ctrl.Result{}, nil
}

func (r *NodeDeviceReconciler) CreateOrUpdateNodeDevice() {
	log.Info("---------------------CreateOrUpdateNodeDevice---------------------")
	nl := new(corev1.NodeList)
	ctx := context.Background()
	err := r.Client.List(ctx, nl)
	if err != nil {
		log.Errorf("get node  list err %s", err)
		return
	}
	globalConfigMap := configuration.DiskConfig.DiskSelectors
	for _, node := range nl.Items {
		DiskSelector := []carinav1.DiskSelector{}
		for _, v := range globalConfigMap {
			if len(v.NodeLabel) == 0 {
				DiskSelector = append(DiskSelector, carinav1.DiskSelector{
					Name:   v.Name,
					Re:     v.Re,
					Policy: v.Policy,
				})
				continue
			}
			if _, ok := node.Labels[v.NodeLabel]; ok {
				DiskSelector = append(DiskSelector, carinav1.DiskSelector{
					Name:   v.Name,
					Re:     v.Re,
					Policy: v.Policy,
				})
				continue
			}
		}
		//nodestatus
		var nodeStatus carinav1.State = carinav1.Active
		//when node is delete, clear pods
		if node.DeletionTimestamp != nil || node.Status.Phase == corev1.NodeTerminated {
			nodeStatus = carinav1.Inactive
			log.Infof("get node  name: %s status: %s", node.Name, node.Status.Phase)
		}
		//when node is nodeready, clear pods
		for _, s := range node.Status.Conditions {
			if s.Type == corev1.NodeReady && s.Status != corev1.ConditionTrue {
				nodeStatus = carinav1.Inactive
				log.Infof("get node  name: %s ,type: %s,status: %s", node.Name, s.Type, s.Status)
			}
		}

		//create nodedevice
		nodeDevice := new(carinav1.NodeDevice)
		if err := r.Client.Get(ctx, client.ObjectKey{Name: node.Name}, nodeDevice); err != nil {
			if apierrs.IsNotFound(err) {
				log.Error(err, "unable to fetch NodeDevice ")
				nodeDevice := &carinav1.NodeDevice{
					TypeMeta: metav1.TypeMeta{
						Kind:       "NodeDevice",
						APIVersion: "carina.storage.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:        node.Name,
						Namespace:   configuration.RuntimeNamespace(),
						Labels:      node.Labels,
						Annotations: node.Annotations,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "",
								Kind:       "CSIDriver",
								Name:       "csi-carina-com",
							},
						},
					},
					Spec: carinav1.NodeDeviceSpec{
						NodeName:     node.Name,
						DiskSelector: DiskSelector,
					},
				}

				log.Info("create nodedevice name" + node.Name)
				if err = r.Client.Create(ctx, nodeDevice); err != nil {
					log.Error(err, "unable to create NodeDevice ", nodeDevice.Name)
				}

			}
		}
		//update nodedevice
		nodeDevice.Status.NodeState = nodeStatus
		if err := r.Status().Update(ctx, nodeDevice); err != nil {
			log.Error(err, " failed to update nodeDevice status name ", nodeDevice.Name)
		}
	}

}

func (r *NodeDeviceReconciler) SetupWithManager(mgr ctrl.Manager, stopChan <-chan struct{}) error {

	ticker := time.NewTicker(60 * time.Second)
	go func(t *time.Ticker) {
		defer ticker.Stop()
		for {
			select {
			case <-t.C:
				r.CreateOrUpdateNodeDevice()
			case <-stopChan:
				log.Info("stop node reconcile...")
				return
			}
		}
	}(ticker)
	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// if  e.Object.(*corev1.PersistentVolume).Spec.CSI == utils.CSIPluginName {

			// }
			return true

		},
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		UpdateFunc:  func(event.UpdateEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(&nodeDeviceFilter{r.NodeName}).
		WithOptions(controller.Options{
			RateLimiter: workqueue.NewItemFastSlowRateLimiter(10*time.Second, 60*time.Second, 5),
		}).
		For(&carinav1.NodeDevice{}).
		Watches(&source.Kind{Type: &corev1.PersistentVolume{}}, &handler.EnqueueRequestForObject{}).WithEventFilter(pred).
		Complete(r)
}

// filter logicVolume
type nodeDeviceFilter struct {
	nodeName string
}

func (f nodeDeviceFilter) filter(nd *carinav1.NodeDevice) bool {
	if nd == nil {
		return false
	}
	if nd.Spec.NodeName == f.nodeName {
		return true
	}
	return false
}

func (f nodeDeviceFilter) Create(e event.CreateEvent) bool {
	return f.filter(e.Object.(*carinav1.NodeDevice))
}

func (f nodeDeviceFilter) Delete(e event.DeleteEvent) bool {
	return f.filter(e.Object.(*carinav1.NodeDevice))
}

func (f nodeDeviceFilter) Update(e event.UpdateEvent) bool {
	return f.filter(e.ObjectNew.(*carinav1.NodeDevice))
}

func (f nodeDeviceFilter) Generic(e event.GenericEvent) bool {
	return f.filter(e.Object.(*carinav1.NodeDevice))
}
