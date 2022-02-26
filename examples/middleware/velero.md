## 准备环境
- Kubernetes：(CSI_VERSION=1.18.0)
- Node OS：Linux
- Kind: 在kind环境搭建的k8s环境使用helm安装carina，须注注释掉内核模块的加载
```
node.
  initContainer:
    modprobe: 
     # - dm_snapshot 
     # - dm_mirror 
     # - dm_thin_pool
```
## 检查环境

```
kubectl get nodes 
NAME                STATUS   ROLES                  AGE   VERSION
e2e-control-plane   Ready    control-plane,master   18h   v1.20.2
e2e-worker          Ready    <none>                 18h   v1.20.2
e2e-worker2         Ready    <none>                 18h   v1.20.2
e2e-worker3         Ready    <none>                 18h   v1.20.2
```

## carina 安装 

```
kubectl get pods -n kube-system |grep carina 
carina-csi-driver-carina-scheduler-c9f5df55b-n74rm   1/1     Running   1          17h
csi-carina-controller-597b8c546-d45hm                4/4     Running   12         17h
csi-carina-node-4bj8s                                2/2     Running   8          17h
csi-carina-node-bph5p                                2/2     Running   9          17h
csi-carina-node-h8q54                                2/2     Running   8          17h
csi-carina-node-xgxcg                                2/2     Running   8          17h
``` 

## 查看默认storageclass使用的是磁盘组
```
kubectl get sc csi-carina-sc -ojson |jq .parameters
{
  "carina.storage.io/disk-type": "hdd",
  "csi.storage.k8s.io/fstype": "xfs"
}
```
## 查看默认磁盘组选择磁盘配置
```
kubectl get configmap -n kube-system carina-csi-driver-csi-config  -oyaml
apiVersion: v1
data:
  config.json: |-
    {
      "diskSelector": [
        {
          "name": "carina-vg-ssd" ,
          "re": ["loop2+"],
          "policy": "LVM",
          "nodeLabel": "kubernetes.io/hostname"
        },
        {
          "name": "carina-vg-hdd",
          "re": ["loop3+"],
          "policy": "LVM",
          "nodeLabel": "kubernetes.io/hostname"
        }
      ],
      "diskScanInterval": "300",
      "schedulerStrategy": "spreadout"
    }
kind: ConfigMap
```

## 节点存储检查 e2e-control-plane, e2e-worker ,e2e-worker2 ,e2e-worker3 
```
kubectl get nodes e2e-worker -o json | jq .status.allocatable
{
  "carina.storage.io/carina-vg-hdd": "189",
  "carina.storage.io/carina-vg-ssd": "189",
  "cpu": "8",
  "ephemeral-storage": "263174212Ki",
  "hugepages-2Mi": "0",
  "memory": "16215404Ki",
  "pods": "110"
}
ubuntu@LAPTOP-4FT6HT3J:~/www/src/github.com/carina-io/carina$ kubectl get nodes e2e-worker -o json | jq .status.capacity
{
  "carina.storage.io/carina-vg-hdd": "200",
  "carina.storage.io/carina-vg-ssd": "200",
  "cpu": "8",
  "ephemeral-storage": "263174212Ki",
  "hugepages-2Mi": "0",
  "memory": "16215404Ki",
  "pods": "110"
}
```

## minio ，valero 服务端 安装
```
minio-58dc5cf789-zwl7g    1/1     Running     0          18h
minio-setup-x9d8b         0/1     Completed   2          18h
restic-569sk              1/1     Running     0          17h
restic-gtbwk              1/1     Running     0          17h
restic-q9vtx              1/1     Running     0          17h
velero-74f49ccbf4-6rdh6   1/1     Running     0          17h
```

## minio 页面查看

```
kubectl port-forward service/minio 9001 9001 -n velero 
```

![minio](./img/minio-1.png)

## velero 客户端安装
```
velero version
Client:
        Version: main
        Git commit: 595e62ee7e6b601a18b1fc47411817a4180b72a6-dirty
Server:
        Version: v1.6.3
# WARNING: the client version does not match the server version. Please update client
```

## 环境检查就绪，安装carina测试示例

```
kubectl apply -f examples/kubernetes/namespace.yaml
kubectl apply -f examples/kubernetes/statefulset.yaml

```
```
kubectl get pods,pvc  -n carina  -o wide 
NAME                    READY   STATUS    RESTARTS   AGE     IP           NODE          NOMINATED NODE   READINESS GATES
pod/carina-stateful-0   1/1     Running   0          5m24s   10.244.2.8   e2e-worker2   <none>           <none>
pod/carina-stateful-1   1/1     Running   0          117s    10.244.2.9   e2e-worker2   <none>           <none>

NAME                                         STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS    AGE     VOLUMEMODE
persistentvolumeclaim/db-carina-stateful-0   Bound    pvc-e063c9e5-e1bb-4f82-b2db-d43bec89b37f   3Gi        RWO            csi-carina-sc   5m26s   Filesystem
persistentvolumeclaim/db-carina-stateful-1   Bound    pvc-e128895d-47a3-40d2-b0f7-752f6fc93089   3Gi        RWO            csi-carina-sc   119s    Filesystem
```

## 添加数据,注意这里svc 对应两个endpoints，所以一个pods的有数据，一个pods没有数据，基于这个情况后面测试恢复

kubectl port-forward service/mysql-service 3306 3306 -n carina
```
mysql -h 127.0.0.1 -uroot -p123456
Welcome to the MariaDB monitor.  Commands end with ; or \g.
Your MySQL connection id is 3
Server version: 5.7.37 MySQL Community Server (GPL)

Copyright (c) 2000, 2018, Oracle, MariaDB Corporation Ab and others.

Type 'help;' or '\h' for help. Type '\c' to clear the current input statement.

MySQL [(none)]> show databases;
+--------------------+
| Database           |
+--------------------+
| information_schema |
| mysql              |
| performance_schema |
| sys                |
+--------------------+
4 rows in set (0.002 sec)

MySQL [(none)]> CREATE DATABASE test_db;
Query OK, 1 row affected (0.001 sec)

MySQL [(none)]> show databases;
+--------------------+
| Database           |
+--------------------+
| information_schema |
| mysql              |
| performance_schema |
| sys                |
| test_db            |
+--------------------+
5 rows in set (0.001 sec)
```
## 开始备份应用前，需要给volume 添加注解 备份pv

```
 kubectl -n carina annotate pod carina-stateful-0   backup.velero.io/backup-volumes=db
 kubectl -n carina annotate pod carina-stateful-1   backup.velero.io/backup-volumes=db
 kubectl get pod -n carina carina-stateful-0 -o jsonpath='{.metadata.annotations}'
 kubectl get pod -n carina carina-stateful-1 -o jsonpath='{.metadata.annotations}'
```
## 创建备份 
velero backup create pvc-backup-1  --snapshot-volumes --include-namespaces carina  --selector app=mysql --default-volumes-to-restic
```
velero backup describe pvc-backup-1
Name:         pvc-backup-1
Namespace:    velero
Labels:       velero.io/storage-location=default
Annotations:  velero.io/source-cluster-k8s-gitversion=v1.20.2
              velero.io/source-cluster-k8s-major-version=1
              velero.io/source-cluster-k8s-minor-version=20

Phase:  Completed

Errors:    0
Warnings:  0

Namespaces:
  Included:  carina
  Excluded:  <none>

Resources:
  Included:        *
  Excluded:        <none>
  Cluster-scoped:  auto

Label selector:  app=mysql

Storage Location:  default

Velero-Native Snapshot PVs:  true

TTL:  720h0m0s

Hooks:  <none>

Backup Format Version:  1.1.0

Started:    2022-02-23 10:36:21 +0800 CST
Completed:  2022-02-23 10:37:36 +0800 CST

Expiration:  2022-03-25 10:36:21 +0800 CST

Total items to be backed up:  12
Items backed up:              12

Velero-Native Snapshots: <none included>

Restic Backups (specify --details for more information):
  Completed:  2
``` 

## 查看monio
![minio](./img/minio-2.png)
![minio](./img/minio-3.png)
![minio](./img/minio-4.png)
![minio](./img/minio-5.png)

## 删除应用及其数据
```
kubectl delete sts carina-stateful
kubectl delete pvc db-carina-stateful-0 db-carina-stateful-1   -n carina 
```
```
ubuntu@LAPTOP-4FT6HT3J:~/www/src/github.com/carina-io/carina$ kubectl get pods,pvc -n carina 
No resources found in carina namespace
```
## 恢复应用
```
ubuntu@LAPTOP-4FT6HT3J:~/tmp$ velero backup get
NAME           STATUS      ERRORS   WARNINGS   CREATED                         EXPIRES   STORAGE LOCATION   SELECTOR
pvc-backup-1   Completed   0        0          2022-02-23 10:36:21 +0800 CST   29d       default            app=mysql
ubuntu@LAPTOP-4FT6HT3J:~/tmp$ velero restore create --from-backup pvc-backup-1
Restore request "pvc-backup-1-20220223105341" submitted successfully.
Run `velero restore describe pvc-backup-1-20220223105341` or `velero restore logs pvc-backup-1-20220223105341` for more details.
ubuntu@LAPTOP-4FT6HT3J:~/tmp$ velero restore get
NAME                          BACKUP         STATUS       STARTED                         COMPLETED   ERRORS   WARNINGS   CREATED                         SELECTOR
pvc-backup-1-20220223105341   pvc-backup-1   InProgress   2022-02-23 10:53:42 +0800 CST   <nil>       0        0          2022-02-23 10:53:41 +0800 CST   <none>
ubuntu@LAPTOP-4FT6HT3J:~/tmp$ velero restore describe pvc-backup-1-20220223105341
Name:         pvc-backup-1-20220223105341
Namespace:    velero
Labels:       <none>
Annotations:  <none>

Phase:                       Completed
Total items to be restored:  12
Items restored:              12

Started:    2022-02-23 10:53:42 +0800 CST
Completed:  2022-02-23 10:55:54 +0800 CST
Warnings:   <error getting warnings: Get "http://minio.velero.svc:9000/velero/restores/pvc-backup-1-20220223105341/restore-pvc-backup-1-20220223105341-results.gz?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=minio%2F20220223%2Fminio%2Fs3%2Faws4_request&X-Amz-Date=20220223T025610Z&X-Amz-Expires=600&X-Amz-SignedHeaders=host&X-Amz-Signature=f75bd3377cd1356ac275000a874eefb2440f5b4b6505d6f6a316bb57ce883d26": dial tcp: lookup minio.velero.svc on 172.29.96.1:53: no such host>

Errors:  <error getting errors: Get "http://minio.velero.svc:9000/velero/restores/pvc-backup-1-20220223105341/restore-pvc-backup-1-20220223105341-results.gz?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=minio%2F20220223%2Fminio%2Fs3%2Faws4_request&X-Amz-Date=20220223T025610Z&X-Amz-Expires=600&X-Amz-SignedHeaders=host&X-Amz-Signature=f75bd3377cd1356ac275000a874eefb2440f5b4b6505d6f6a316bb57ce883d26": dial tcp: lookup minio.velero.svc on 172.29.96.1:53: no such host>

Backup:  pvc-backup-1

Namespaces:
  Included:  all namespaces found in the backup
  Excluded:  <none>

Resources:
  Included:        *
  Excluded:        nodes, events, events.events.k8s.io, backups.velero.io, restores.velero.io, resticrepositories.velero.io
  Cluster-scoped:  auto

Namespace mappings:  <none>

Label selector:  <none>

Restore PVs:  auto

Restic Restores (specify --details for more information):
  Completed:  2

Preserve Service NodePorts:  auto
```

注意： velero restore logs 访问有错误，但实际不影响恢复流程

## 查看po、pvc、pv是否自动恢复创建
```
ubuntu@LAPTOP-4FT6HT3J:~/www/src/github.com/carina-io/carina$ kubectl get pods,pvc -n carina
NAME                    READY   STATUS    RESTARTS   AGE
pod/carina-stateful-0   1/1     Running   0          5m48s
pod/carina-stateful-1   1/1     Running   0          5m45s

NAME                                         STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS    AGE
persistentvolumeclaim/db-carina-stateful-0   Bound    pvc-33bc9d75-b52a-47e5-bbaa-496f66434724   3Gi        RWO            csi-carina-sc   5m56s
persistentvolumeclaim/db-carina-stateful-1   Bound    pvc-a2e80627-b955-4e60-adce-0b66f7b3a7fb   3Gi        RWO            csi-carina-sc   5m53s
```
### 查看minio 

![minio](./img/minio-6.png)

## 查看数据
```
kubectl -n carina  exec -it carina-stateful-0  -- mysql -uroot -p123456 
mysql: [Warning] Using a password on the command line interface can be insecure.
Welcome to the MySQL monitor.  Commands end with ; or \g.
Your MySQL connection id is 3
Server version: 5.7.37 MySQL Community Server (GPL)

Copyright (c) 2000, 2022, Oracle and/or its affiliates.

Oracle is a registered trademark of Oracle Corporation and/or its
affiliates. Other names may be trademarks of their respective
owners.

Type 'help;' or '\h' for help. Type '\c' to clear the current input statement.

mysql> show databases;
+--------------------+
| Database           |
+--------------------+
| information_schema |
| mysql              |
| performance_schema |
| sys                |
+--------------------+
4 rows in set (0.00 sec)

mysql> exit 
Bye
ubuntu@LAPTOP-4FT6HT3J:/mnt/c/Users/admin/AppData/Local/Programs/Microsoft VS Code$ kubectl -n carina  exec -it carina-stateful-1  -- mysql -uroot -p123456 
mysql: [Warning] Using a password on the command line interface can be insecure.
Welcome to the MySQL monitor.  Commands end with ; or \g.
Your MySQL connection id is 2
Server version: 5.7.37 MySQL Community Server (GPL)

Copyright (c) 2000, 2022, Oracle and/or its affiliates.

Oracle is a registered trademark of Oracle Corporation and/or its
affiliates. Other names may be trademarks of their respective
owners.

Type 'help;' or '\h' for help. Type '\c' to clear the current input statement.

mysql> show databases;
+--------------------+
| Database           |
+--------------------+
| information_schema |
| mysql              |
| performance_schema |
| sys                |
| test_db            |
+--------------------+
5 rows in set (0.00 sec)
```

## 可以看到恢复结果符合预期，velero创建备份默认保留一个月。也可以创建周期性备份应用

velero schedule create pvc-backup-2 --schedule "0 1 * * *" --snapshot-volumes --include-namespaces carina  --selector app=mysql --default-volumes-to-restic 
```
ubuntu@LAPTOP-4FT6HT3J:~$ velero backup get
NAME                          STATUS      ERRORS   WARNINGS   CREATED                         EXPIRES   STORAGE LOCATION   SELECTOR
pvc-backup-1                  Completed   0        0          2022-02-23 10:36:21 +0800 CST   29d       default            app=mysql
pvc-backup-2-20220223032620   Completed   0        0          2022-02-23 11:26:21 +0800 CST   29d       default            <none>
pvc-backup-3-20220223035404   Completed   0        0          2022-02-23 11:54:05 +0800 CST   29d       default            app=mysql
```