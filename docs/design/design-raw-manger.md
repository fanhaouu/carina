
#### 基于本地存储使用裸盘设计方案

#### 介绍

- 在业务实际应用场景中，基于提高性能的考虑，磁盘管理不使用lvm,而是直接使用裸盘。

#### 功能设计

- 通过配置文件获取扫描时间间隔和磁盘的匹配条件，发现磁盘新增或者删除
- 新增的磁盘或者分区当作一个设备注册到kubelet

#### 实现细节

- 用户请求的PV大小为S,carina优先找出所有空闲磁盘（已经有分区）并选择最低满足空间大于S磁盘；如果所有非空磁盘都不适用，则选择最小的磁盘要求其容量大于S，如果选择了相同大小的多个磁盘，则随机选择一个。

- 如果从上述过程中选择了一个磁盘，则carina将创建一个分区作为PV的真实数据后端。否则，PV绑定将失败。

- 用户可以在PVC中指定注释 `carina.io/exclusivly-disk-claim: true`以声明物理磁盘独家使用。如果已设置此注释且其值为true，然后carina将尝试绑定一个空磁盘（最低要求）作为其数据后端。

#### 实现逻辑
- 创建分区 
- 扩容分区
- 删除分区

### 流程细节
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

- carina-node则负责监听LogicVolume的创建事件，获取lv类型，给LogicVolume绑定驱动设备组和设备id,更新状态。
- 给pods 配置 cgroup  blkio,限制进程读写的 IOPS 和吞吐量
- node-driver-registra 调用接口获取CSI插件信息，并向kubelet进行注册
- Volume Manager（Kubelet 组件）观察到有新的使用 CSI 类型 PV 的 Pod 调度到本节点上，于是调用内部 in-tree CSI 插件函数调用外部插件接口NodePublishVolume，NodeUnpublishVolume
- 启动磁盘检查是否有新盘加入
- 一致性检查，清理孤儿卷, 每十分钟会遍历本地volume，然后检查k8s中是否有对应的logicvolume，若是没有则删除本地volume（remove lv）;每十分钟会遍历k8s中logicvolume，然后检查logicvolume是否有对应的pv，若是没有则删除logicvolume
- 设备注册


### 问题： 

- 创建分区是二进制linux交互命令，程序中如何执行传入参数
- pod可以共享裸盘意味着裸盘对应有多个分区，但实际行裸盘最多只有4分主分区，一个扩展分区，多个逻辑分区，那么在执行中是创建的什么类型分区
- 分区扩容，裸盘已经创建了大于两个的问题，并且是连续的空间，那么扩容第一个分区的时候意味着没有可扩容的空间了。
- lv 删除意味着不保留数据，是不是要清除已经创建的分区和数据
