
## 基于本地存储使用裸盘设计方案

### 介绍

- 在业务实际应用场景中，基于提高性能的考虑，磁盘管理不使用lvm,而是直接使用裸盘。

### 功能设计

- 通过配置文件获取扫描时间间隔和磁盘的匹配条件，发现磁盘新增或者删除
- 新增的磁盘当作一个设备注册到kubelet
- 新增磁盘分区作为pv数据存储

### 实现细节

- 用户请求的PV大小为S,carina优先找出所有空闲磁盘（已经有分区）并选择最低满足空间大于S磁盘；如果所有非空磁盘都不适用，则选择最小的磁盘要求其容量大于S，如果选择了相同大小的多个磁盘，则随机选择一个。
- 如果从上述过程中选择了一个磁盘，根据storageClass参数匹配是创建MBR,GPT，则carina将创建一个分区作为PV的真实数据后端。否则，PV绑定将失败。
- 用户可以在PVC中指定注释 `carina.io/exclusivly-disk-claim: true`以声明物理磁盘独家使用。如果已设置此注释且其值为true，然后carina将尝试绑定一个空磁盘（最低要求）作为其数据后端。

### 实现逻辑
#### 创建分区 

- ① 根据参数设置分区类型MBR,GPT创建
- ②如果是MBR,最多支持4个主分区，我们创建的分区是逻辑分区,单个磁盘最多支持11个逻辑分区
```
fdisk /dev/sdd
p
n
e 扩展分区
2048
102048
63
n
l 逻辑分区
102049
$pv容量
63
wq
```
- ②如果是GPT。检测裸盘已有分区。分配新的分区号创建分区
```
sgdisk -p /dev/sdd   # 查看所有GPT分区
sgdisk -n n:0:+1M -c n:"carina $namespace.svc.pod.name" -t n:8300 /dev/sdd  # 创建分区
sgdisk -z  /dev/sdd1  # 清除分区数据
sgdisk -i n /dev/sdd #  查看分区详情
sgdisk -d n /dev/sdd  # 删除第n个分区
```
partprobe
fdisk -lu /dev/sdd
#为分区创建文件系统
mkfs -t ext4 /dev/sdd1
mkfs -t xfs /dev/sdd1
#配置/etc/fstab文件并挂载分区
echo `blkid /dev/sdd1 | awk '{print $2}' | sed 's/\"//g'` /mnt ext4 defaults 0 0 >> /etc/fsta
df -h
```
#### 扩容分区

- ① storage配置参数注解pod独占整块磁盘可以扩容
- ② 多个pods共享磁盘不支持扩容
- ③ 扩展已有MBR分区
```
fdisk -lu /dev/vdb # 记录旧分区的起始和结束的扇区位置和分区表格式。
```
查看数据盘的挂载路径
blkid /dev/vdb1 查看文件系统类型
```
mount | grep "/dev/vdb"
```
取消挂载（umount）数据盘。
```
umount /dev/vdb1
```
使用fdisk工具删除旧分区。
使用fdisk命令新建分区。

运行以下命令再次检查文件系统，确认扩容分区后的文件系统状态为clean。
```
e2fsck -f /dev/vdb1
resize2fs /dev/vdb1  #ext*文件系统
mount  # 重新挂载
xfs_growfs #xfs文件系统先挂载后扩容
```
③ 扩展已有GPT分区
```
执行先查询裸盘挂载位置后卸unmount
<!--备份磁盘分区
sgdisk -b=/tmp/$(dev/sdd).partitiontable /dev/sdd
sgdisk -i n /dev/sdd #  查看分区详情
sgdisk -d n /dev/sdd  # 删除第n个分区
sgdisk -n n:0:+1M+(pvc容量) -c n:"carina $namespace.svc.pod.name" -t n:8300 /dev/sdd  
#恢复数据
sgdisk -R=/tmp/$(dev/sdd).partitiontable /dev/sddn -->
parted  /dev/sdd
接下来输入print来查看分区信息，记住已有分区的End值，以此值作为下一个分区的起始偏移值
start offset
mkpart primary offset+add end
print
quit


运行以下命令再次检查文件系统，确认扩容分区后的文件系统状态为clean。
e2fsck -f /dev/vdb1
resize2fs /dev/vdb1  #ext*文件系统
mount  # 重新挂载
xfs_growfs #xfs文件系统先挂载后扩容
```
#### 删除分区

lv 和裸盘分区绑定，删除lv 就删除裸盘pod占用的分区
sgdisk -d n /dev/sdd  # 删除第n个分区
### 流程细节
#### controller : nodeController,pvcController,webhook,csiControllerGrpc

- 监听 ConfigMap是否变化,lvm是一个vg对应注册一个设备(carina-vg-XXX.sock)，裸设备则是一个裸盘或者分区对应一个注册设备(carina-raw-XXX.sock)；通过切割注册设备，判断注册设备的健康状态来检测使用量。
- PVC创建完成后,根据存储类型(此处为rbd)找到存储类StorageClass
- external-provisioner，watch到指定StorageClass的 PersistentVolumeClaim资源状态变更，会自动地调用csiControllerGrpc这两个CreateVolume、DeleteVolume接口；等待返回成功则创建pv，卷控制器会将 PV 与 PVC 进行绑定。
- CreateVolume 接口还会创建LogicVolume，一个LogicVolume对应一个lv, 增加注解或者标签标识是使用裸盘还是lvm
- k8s组件AttachDetachController 控制器观察到使用 CSI 类型 PV 的 Pod 被调度到某一节点，此时调用内部 in-tree CSI 插件（csiAttacher）的 Attach 函数创建一个 VolumeAttachment 对象到集群中。
- external-attacher watch到VolumeAttachment资源状态变更，会自动地调用外部 CSI插件这两个ControllerPublish、ControllerUnpublish接口。外部 CSI 插件挂载成功后，External Attacher会更新相关 VolumeAttachment 对象的 .Status.Attached 为 true。
- external-resizer  watch到PersistentVolumeClaim资源的容量发生变更，会自动地调用这个ControllerExpandVolume接口。

#### node : logicVolumeController,podController,csiNodeGrpc

- carina-node则负责监听LogicVolume的创建事件，获取lv类型，给LogicVolume绑定裸盘分区驱动设备组和设备id,更新状态。
- 给pods 配置 cgroup  blkio,限制进程读写的 IOPS 和吞吐量
- node-driver-registra 调用接口获取CSI插件信息，并向kubelet进行注册
- Volume Manager（Kubelet 组件）观察到有新的使用 CSI 类型 PV 的 Pod 调度到本节点上，于是调用内部 in-tree CSI 插件函数调用外部插件接口NodePublishVolume，NodeUnpublishVolume
- 启动磁盘检查是否有新裸盘加入，注册裸盘设备
- 一致性检查，清理孤儿卷, 每十分钟会遍历本地volume，然后检查k8s中是否有对应的logicvolume，若是没有则删除本地volume（remove lv）并且删除对应设备分区;
- 每十分钟会遍历k8s中logicvolume，然后检查logicvolume是否有对应的pv，若是没有则删除logicvolume



