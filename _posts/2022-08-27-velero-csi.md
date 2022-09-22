---
title: Velero CSI Snapshot Support
tags: Velero VolumeSnapshot
---

集成CSI Snapshot，使Velero能够使用 Kubernetes CSI Snapshot API 备份和恢复CSI支持的卷。

通过支持 CSI Snapshot API，Velero 可以支持任何具有 CSI 驱动程序的volume provider，而无需提供特定于 Velero 的插件。本页概述了如何通过 CSI 插件向 Velero 添加对 CSI Snapshot的支持。
<!--more-->

## Pre-requests
- Kubernetes 版本 1.20 或更高版本。
- 运行能够支持 v1 版本的 VolumeSnapshot。
- 跨集群恢复 CSI VolumeSnapshot 时，目的集群中的 CSI 驱动程序名称与源集群上的名称相同，以保证 CSI VolumeSnapshots 的跨集群可移植性

注意：并非所有云提供商的 CSI 驱动程序都保证Snapshot的持久性，这意味着 VolumeSnapshot 和 VolumeSnapshotContent 对象可能与原始 PersistentVolume 存储在相同的对象存储系统位置，并且可能容易受到数据丢失的影响。

## 安装CSI Support

要将 Velero 与 CSI VolumeSnapshot API 集成，必须启用 EnableCSI 功能标志并在 Velero 服务器上安装 Velero CSI 插件。

这两个都可以使用 velero install 命令添加:

```
velero install \
--features=EnableCSI \
--plugins=<object storage plugin>,velero/velero-plugin-for-csi:v0.3.0 \
```

## Implementation Choices

本节记录了在实施 Velero CSI 插件期间所做的一些选择：

- Velero CSI 插件创建的 VolumeSnapshot 仅在Backup的生命周期内保留，即使 VolumeSnapshotClass 上的 DeletionPolicy 设置为 Retain。为此，在删除备份期间，在删除 VolumeSnapshot 之前，会修改 VolumeSnapshotContent 对象以将其 DeletionPolicy 设置为 Delete。删除 VolumeSnapshot 对象将导致 VolumeSnapshotContent 和 storage provider 中的Snapshot的级联删除。
- 在 velero 备份期间创建的VolumeSnapshotContent 对象空置，未绑定到 VolumeSnapshot 对象，将使用标签发现并在备份删除时删除。
- Velero CSI 插件将在集群中选择具有相同驱动程序名称并且设置了`velero.io/csi-volumesnapshot-class` 标签的 VolumeSnapshotClass，例如

```
  velero.io/csi-volumesnapshot-class: "true"
```

- VolumeSnapshot 对象会在备份上传到对象存储后从集群中移除，以便在 DeletionPolicy 为`Delete`时无需删除存储提供程序中的快照即可删除备份的命名空间。

## How it Works - Overview

Velero 的 CSI 支持不依赖于 Velero VolumeSnapshotter 插件接口。相反，Velero 使用一组 BackupItemAction 插件，这些插件首先针对 PersistentVolumeClaims 采取行动。

当此 BackupItemAction 看到 PersistentVolumeClaims 指向由 CSI 驱动程序支持的 PersistentVolume 时，它​​将选择具有相同驱动程序名称且具有`velero.io/csi-volumesnapshot-class`标签的`VolumeSnapshotClass`来创建以`PersistentVolumeClaim`作为源的`CSI VolumeSnapshot`对象。此`VolumeSnapshot`对象与用作源的`PersistentVolumeClaim`位于同一命名空间中。

这时，CSI External Snapshot Controller 将看到`VolumeSnapshot`并创建一个`VolumeSnapshotContent`对象，这是一个集群范围的资源，它将指向存储系统中实际的、基于磁盘的Snapshot。 external-snapshotter插件会调用CSI驱动的snapshot方法，驱动会调用存储系统的API生成Snapshot。 一旦生成 ID 并且存储系统将Snapshot标记为可用于恢复，VolumeSnapshotContent 对象将更新 status.snapshotHandle，并且设置 status.readyToUse 字段。

Velero 将在备份 tarball 中包含生成的 VolumeSnapshot 和 VolumeSnapshotContent 对象，并将 JSON 文件中的所有 VolumeSnapshots 和 VolumeSnapshotContents 对象上传到对象存储系统。

当 Velero 将备份同步到新集群时，VolumeSnapshotContent 对象和 VolumeSnapshotClass 也将同步到集群中，以便 Velero 可以适当地管理备份。

VolumeSnapshotContent 上的 DeletionPolicy 将与用于创建 VolumeSnapshot 的 VolumeSnapshotClass 上的 DeletionPolicy 相同。在 VolumeSnapshotClass 上设置 Retain 的 DeletionPolicy 将在 Velero 备份的生命周期内保留存储系统中的卷Snapshot，并防止在发生灾难时删除存储系统中的卷Snapshot，其中命名空间与VolumeSnapshot 对象可能会丢失。

当 Velero 备份到期时，VolumeSnapshot 对象将被删除，VolumeSnapshotContent 对象将更新为具有 Delete 的 DeletionPolicy，以释放存储系统上的空间。


![csi-snapshot](../../../assets/images/posts/csi-snapshot.drawio.png)
