---
title: CSI Replication
tags: CSI VolumeReplication
--- 

[CSI addon](https://github.com/csi-addons/spec/tree/main/replication) 提供了Volume Replication接口用于扩展CSI的功能。

<!--more-->

该规范为存储提供商 (SP)提供了接口的定义和实现VolumeReplication兼容插件的最低操作和打包建议。还有插件必须提供的接口。
![volume-replication-arch](../../../assets/images/posts/volume-replication-arch.png)

## RPC Interface

```
syntax = "proto3";
package replication;

import "google/protobuf/descriptor.proto";

option go_package = ".;replication";

extend google.protobuf.FieldOptions {
  // Indicates that a field MAY contain information that is sensitive
  // and MUST be treated as such (e.g. not logged).
  bool replication_secret = 1099;

  // Indicates that this field is OPTIONAL and part of an experimental
  // API that may be deprecated and eventually removed between minor
  // releases.
  bool alpha_field = 1100;
}

// Controller holds the RPC methods for replication and all the methods it
// exposes should be idempotent.
service Controller {
  // EnableVolumeReplication RPC call to enable the volume replication.
  rpc EnableVolumeReplication (EnableVolumeReplicationRequest)
  returns (EnableVolumeReplicationResponse) {}
  // DisableVolumeReplication RPC call to disable the volume replication.
  rpc DisableVolumeReplication (DisableVolumeReplicationRequest)
  returns (DisableVolumeReplicationResponse) {}
  // PromoteVolume RPC call to promote the volume.
  rpc PromoteVolume (PromoteVolumeRequest)
  returns (PromoteVolumeResponse) {}
  // DemoteVolume RPC call to demote the volume.
  rpc DemoteVolume (DemoteVolumeRequest)
  returns (DemoteVolumeResponse) {}
  // ResyncVolume RPC call to resync the volume.
  rpc ResyncVolume (ResyncVolumeRequest)
  returns (ResyncVolumeResponse) {}
}
```

## Volume Replication Operator

基于 csi-addons/spec 规范，Volume Replication Operator 为存储灾难恢复提供通用且可重用的 API。

Volume Replication Operator 遵循控制器模式，并为存储灾难恢复提供扩展 API。扩展 API 是通过自定义资源定义 (CRD) 提供的。

### VolumeReplicationClass
VolumeReplicationClass 是一个cluster-scoped资源，其中包含与驱动程序相关的配置参数。

`provisioner`是存储供应商的名称

`parameters`包含传递给驱动程序的键值对。用户可以添加自己的键值对。带有`replication.storage.openshift.io/`前缀的键是保留键，不会传递给驱动程序。

- replication.storage.openshift.io/replication-secret-name
- replication.storage.openshift.io/replication-secret-namespace

```
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeReplicationClass
metadata:
  name: volumereplicationclass-sample
spec:
  provisioner: example.provisioner.io
  parameters:
    replication.storage.openshift.io/replication-secret-name: secret-name
    replication.storage.openshift.io/replication-secret-namespace: secret-namespace
```
### VolumeReplication
`VolumeReplication`是一个namespaced资源，包含对要复制的存储对象的引用和对应于提供复制的驱动程序的`VolumeReplicationClass`。

`replicationState`是被引用的卷的状态。可能的值有`primary`, `secondary`和`resync`。
- primary: 表示该卷是primary
- secondary: 表示卷是secondary
- resync: 表示卷需要重新同步

`dataSource` 引用需要复制的源：
- apiGroup 是被引用资源的组。如果未指定 apiGroup，则指定的 Kind 必须在核心 API 组中。对于任何其他第三方类型，apiGroup 是必需的。
- kind 是被复制的资源类型。例如。 PersistentVolumeClaim。
- name 是资源的名称。

`replicationHandle`（可选）是一个现有的replication ID
```
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeReplication
metadata:
  name: volumereplication-sample
  namespace: default
spec:
  volumeReplicationClass: volumereplicationclass-sample
  replicationState: primary
  replicationHandle: replicationHandle # optional
  dataSource:
    kind: PersistentVolumeClaim
    name: myPersistentVolumeClaim # should be in same namespace as VolumeReplication
```
### Usage
#### Planned Storage Migration
##### Failover

Failover操作是将生产切换到备份设施（通常是恢复站点）的过程。在Failover的情况下，应停止对主站点的访问。应该设置辅助集群作为primary，以便可以在那里恢复访问。

应用程序应该进行定期或一次性备份，这样可用于在辅助站点（集群-b）上进行恢复。

按照以下步骤将工作负载从主集群计划迁移到辅助集群：

- 减少主集群上所有使用 PVC 的应用程序 pod
- 从主集群备份 PVC 和 PV 对象。这可以使用一些备份工具（如 velero）来完成。
- 在主站点的`VolumeReplication CR`中将`replicationState`更新为`secondary`。当Operator观察到这一变化时，它将通过 GRPC 请求将信息向下传递给驱动程序，以将`dataSource`标记为`secondary`。
- 如果在辅助集群上手动重新创建 PVC 和 PV，请删除`PV`对象中的`claimRef`部分。
- 在辅助站点上重新创建`storageclass`、`PVC`和`PV`对象。
- 当您在 PVC 和 PV 之间创建静态绑定时，不会创建新的 PV，PVC 将绑定到现有的 PV。
- 在辅助站点上创建`VolumeReplicationClass`。
- 为启用Replication的所有`PVC`创建`VolumeReplication`
  - 辅助站点上的所有`PVC`，其`replicationState` 应该是`primary`。
- 通过在 `VolumeReplication CR` status下验证volume是否在辅助站点上标记为`primary`。
- 一旦volume被标记为`primary`，PVC 现在就可以使用了。现在，我们可以扩展应用程序以使用 PVC。

在异步DR恢复中，无法获得完整的数据。只能根据快照间隔时间获取crash-consistent数据。

##### Failback

要在计划迁移的情况下对主集群执行Failback操作，只需按相反的顺序执行Failover的步骤。如果主集群上已经存在所需的 yaml，可以跳过备份恢复操作。但是任何新的 PVC 仍需要在主站点上进行恢复。

#### Storage Disaster Recovery
##### Failover

如果发生DR，在辅助站点创建`VolumeReplication CR`。由于与主站点的连接丢失，operator会自动向驱动程序发送 GRPC 请求，以强制将数据源标记为主站点。

- 如果在辅助集群上手动创建 PVC 和 PV，请删除 PV 对象中的 claimRef 部分。
- 在辅助站点上创建`storageclass`、`PVC` 和 `PV` 对象。
- 当在 PVC 和 PV 之间创建静态绑定时，这里不会创建新的 PV，PVC 将绑定到现有的 PV。
- 在辅助站点上创建`VolumeReplicationClass`和`VolumeReplication CR`。
- 通过在`VolumeReplication CR`中的状态验证辅助站点变为primary。
- 一旦volume被标记为`primary`，PVC 现在就可以使用了。现在，我们可以扩展应用程序以使用 PVC。

##### Failback

在主站点故障故障并且希望从辅助站点进行Failback，请执行以下步骤：

- 将主站点上的`VolumeReplication CR` `replicationState` 从主站点更新为辅助站点。
- 缩减辅助站点上的应用程序。
- 在辅助站点中将 `VolumeReplication CR` `replicationState` 从主站点更新为辅助站点。
- 在主站点上，验证`VolumeReplication`状态是否标记为Ready
- 将卷标记为可以使用后，在主站点中将`replicationState`状态从辅助更改为主。
- 在主站点上再次扩展应用程序。