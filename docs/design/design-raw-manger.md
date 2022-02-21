
#### 基于本地存储使用裸盘设计方案

#### 介绍

- 在业务实际应用场景中，基于提高性能的考虑，磁盘管理不使用lvm,而是直接使用裸盘。

#### 功能设计

- 通过配置文件获取扫描时间间隔和磁盘的匹配条件，发现磁盘新增或者删除
- 新增的磁盘或者分区当作一个设备注册到kubelet

#### 实现细节

- 创建pvc后，控制服务接收创建volume请求，并创建CRD

  ```
  +------------+         +--------------------+         +---------------------+
  | PVC Create |-------->|  Controller Server |-------->| Create CRD Resource |
  +------------+         +--------------------+         +---------------------+  
  ```

- 节点服务运行架构图

  - 磁盘管理模块提供裸盘操作接口，独占裸盘类型的 PV 不支持 PV 扩容、PV 快照等操作。

  - 定时协程，定时扫描本次磁盘与配置对比，完成磁盘初始化以及与device-plugins配额校准

  - CRD调谐程序负责根据创建LogicVolume事件

  - Kubelet调用Node grpc服务，完成volume卷操作

#### 实现细节

- rawDeviceCSIDriver-> CSIDriver
- rawDeviceProvisioner->Deploy
- rawDevicePlugin->Demonset

#### storageClass 新增加参数volumeType

  ```yaml
  apiVersion: storage.k8s.io/v1
  kind: StorageClass
  metadata:
    name: csi-carina-sc
  provisioner: carina.storage.io # 这是该CSI驱动的名称，不允许更改
  parameters:
    # 这是kubernetes内置参数，我们支持xfs,ext4两种文件格式，如果不填则默认ext4
    csi.storage.k8s.io/fstype: xfs
    # 这是选择磁盘分组，该项目会自动将SSD及HDD磁盘分组
    # SSD：ssd HDD: hdd
    # 如果不填会随机选择磁盘类型
    carina.storage.io/disk-type: hdd
    carina.storage.io/volume-type: raw  # lvm || raw ,默认lvm
  reclaimPolicy: Delete
  allowVolumeExpansion: true # 支持扩容，定为true便可
  # WaitForFirstConsumer表示被容器绑定调度后再创建pv
  volumeBindingMode: WaitForFirstConsumer
  # 支持挂载参数设置，默认为空
  # 如果没有特殊的需求，为空便可满足大部分要求
  mountOptions:
    - rw
  ```

#### controller : nodeController,pvcController,webhook,csiControllerGrpc

- 监听 ConfigMap是否变化,lvm是一个vg对应注册一个设备(carina-vg-XXX.sock)，裸设备则是一个裸盘或者分区对应一个注册设备(carina-raw-XXX.sock)；通过切割注册设备，判断注册设备的健康状态来检测使用量。
- PVC创建完成后,根据存储类型(此处为rbd)找到存储类StorageClass
- external-provisioner，watch到指定StorageClass的 PersistentVolumeClaim资源状态变更，会自动地调用csiControllerGrpc这两个CreateVolume、DeleteVolume接口；等待返回成功则创建pv，卷控制器会将 PV 与 PVC 进行绑定。
- CreateVolume 接口还会创建LogicVolume，一个LogicVolume对应一个lv, 增加注解或者标签标识是独占磁盘还是lvm

```yaml
apiVersion: carina.storage.io/v1
kind: LogicVolume
metadata:
  creationTimestamp: "2022-01-24T07:30:55Z"
  finalizers:
  - carina.storage.io/logicvolume
  generation: 1
  name: pvc-205f5451-c22b-40d6-bd32-777138a58585
  namespace: default
  resourceVersion: "50405102"
  uid: 0d54e616-6ef9-4d27-beae-bef64604a104
spec:
  deviceGroup: carina-vg-hdd  # raw 或者 lvm的vg
  nameSpace: carina
  nodeName: dev1-node-2.novalocal
  pvc: data-my-cluster-mysql-0
  size: 10Gi
status:
  currentSize: 10Gi
  deviceMajor: 252   # lvInfo.LVKernelMajor 
  deviceMinor: 4     # lvInfo.LVKernelMinor
  status: Success
  volumeID: volume-pvc-205f5451-c22b-40d6-bd32-777138a58585     # "volume-" + lv.Name
```
- k8s组件AttachDetachController 控制器观察到使用 CSI 类型 PV 的 Pod 被调度到某一节点，此时调用内部 in-tree CSI 插件（csiAttacher）的 Attach 函数创建一个 VolumeAttachment 对象到集群中。
- external-attacher watch到VolumeAttachment资源状态变更，会自动地调用外部 CSI插件这两个ControllerPublish、ControllerUnpublish接口。外部 CSI 插件挂载成功后，External Attacher会更新相关 VolumeAttachment 对象的 .Status.Attached 为 true。
- external-resizer  watch到PersistentVolumeClaim资源的容量发生变更，会自动地调用这个ControllerExpandVolume接口。

#### node : logicVolumeController,podController,csiNodeGrpc

> carina-node则负责监听LogicVolume的创建事件，给LogicVolume绑定设备组和设备id,更新状态。

- node-driver-registra 调用接口获取CSI插件信息，并向kubelet进行注册

- Volume Manager（Kubelet 组件）观察到有新的使用 CSI 类型 PV 的 Pod 调度到本节点上，于是调用内部 in-tree CSI 插件函数调用外部插件接口NodePublishVolume，NodeUnpublishVolume

，