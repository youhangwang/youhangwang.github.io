---
title: ODF Regional Disaster Recovery
tags: ODF RegionalDR
---

本文的目的是详细说明Regional DR的步骤和命令，Regional DR能够将应用程序从一个 OpenShift Container Platform (OCP) 集群failover到另一个集群，然后再将同一应用程序failback到primary集群。在这种情况下，将使用 Red Hat Advanced Cluster Management 或 RHACM 创建或导入 OCP 集群。

<!--more-->

请注意，本文使用的 OpenShift Data Foundation (ODF) 版本是 v4.11， RHACM的版本是 v2.5。除了这两个称为托管集群的集群之外，目前还需要第三个 OCP 集群，即高级集群管理 (ACM) Hub cluster。

一般情况下，执行Regional DR的具体步骤包括：

1. 在 hub 集群上安装 ACM Operator。
  - 创建 OCP hub 集群后，从 OperatorHub 中安装 ACM Operator。在 Operator 和关联的 Pod 运行后，创建 `MultiClusterHub` 资源。
2. 创建托管 OCP 集群或将其导入 ACM hub cluster。
  - 使用 RHACM 控制台导入或创建两个具有足够 ODF 资源（计算节点、内存、cpu）的托管集群。
3. 确保集群具有唯一的private network地址范围。
  - 确保primary和secondary OCP 集群具有唯一的private network地址范围。
4. 使用 Submariner add-ons连接 private network。
  - 使用 RHACM Submariner 附加组件连接托管的 OCP 私有网络（集群和服务）。
5. 在托管集群上安装 ODF 4.10。
  - 在primary和secondary OCP 托管集群上安装 ODF 4.10 并验证部署。
6. 在 ACM hub 集群上安装 `ODF Multicluster Orchestrator`。
  - 从 ACM hub 集群上的 OperatorHub 安装 `ODF Multicluster Orchestrator`。还将安装 `OpenShift DR Hub Operator`(Ramen Hub Operator)。
7. 配置 S3 端点之间的 SSL 访问
  - 如果托管 OpenShift 集群未使用有效证书，则必须通过创建包含证书的 user-ca-bundle ConfigMap 来完成此步骤。
8. 启用多集群 Web 控制台。
  - 这是创建 `DRPolicy` 之前需要的一项新的`技术预览`功能。它仅在 ACM 所在的 Hub 集群上需要。
9. 创建一个或多个 `DRPolicy`
  - 使用 All Clusters Data Services UI 通过选择策略将应用到的两个托管集群来创建 DRPolicy。
10. 验证是否安装了 `OpenShift DR Cluster operator`(Ramen Cluster Operator)。
  - 创建第一个 `DRPolicy` 后，将触发在 UI 中选择的两个托管集群上创建 `DR Cluster operators`。
11. 使用 ACM 控制台创建示例应用程序。
  - 使用来自 github.com/RamenDR/ocm-ramen-samples 的示例应用程序示例创建用于故障转移和故障恢复测试的 busybox deployment。
12. 验证示例应用程序部署。
  - 在两个受管集群上使用 CLI 命令验证应用程序是否正在运行。
13. 将 DRPolicy 应用于示例应用程序。
  - 使用 All Clusters Data Services UI 将新的 DRPolicy 应用到示例应用程序。应用后，在 Hub 集群的`应用程序`命名空间中创建 `DRPlacementControl` 资源。
14. 将示例应用程序故障转移到secondary托管集群。
  - 修改Hub Cluster上的application DRPlacementControl资源，添加Failover动作，指定failoverCluster触发failover。
15. 将示例应用程序failback到primary托管集群。
  - 修改 Hub 集群上的应用程序 `DRPlacementControl` 资源并将操作更改为 `Relocate` 以触发failback到 preferredCluster。

## [Deploy and Configure ACM for Multisite connectivity](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-multisite-ramen.html#_deploy_and_configure_acm_for_multisite_connectivity)

## [OpenShift Data Foundation Installation](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-multisite-ramen.html#_openshift_data_foundation_installation)

## [OpenShift Data Foundation Installation](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-Regional-ramen.html#_openshift_data_foundation_installation)

## [Install ODF Multicluster Orchestrator Operator on Hub cluster](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-multisite-ramen.html#_install_odf_multicluster_orchestrator_operator_on_hub_cluster)

## [Configure SSL access between S3 endpoints](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-multisite-ramen.html#_configure_ssl_access_between_s3_endpoints)


## [Enabling Multicluster Web Console](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-multisite-ramen.html#_enabling_multicluster_web_console)




## [Create Data Policy on Hub cluster](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-multisite-ramen.html#_create_data_policy_on_hub_cluster)

RegionalDR 使用 Hub 集群上的 DRPolicy 资源来跨托管集群进行Failover和relocate workload。 DRPolicy 需要一组连接到两个安装ODF 4.11的 DRCluster 或peer集群。 ODF MultiCluster Orchestrator Operator 有助于通过多集群 Web 控制台创建每个 DRPolicy 和相应的 DRCluster。

在 Hub 集群上导航到All Clusters。然后导航到Data services菜单下的Data policies。如果这是创建的第一个 DRPolicy，则将在页面底部看到创建 Create DRpolicy。

![MCO-create-first-drpolicy](../../../assets/images/posts/MCO-create-first-drpolicy.png)

单击创建 DRPolicy。从托管集群列表中选择希望参与 DRPolicy 的集群，并为策略指定一个唯一名称（即 ocp4perf1-ocp4perf2）。

![MCO-drpolicy-selections](../../../assets/images/posts/MCO-drpolicy-selections.png)

Replication policy的灰色下拉选项将根据所选的 OpenShift 集群自动选择为`async`， 并需要填写一个`Sync schedule`。选择Create。这应该在 Hub 集群上创建两个 DRCluster 资源和 DRPolicy。此外，当创建初始 DRPolicy 时，将发生以下情况：

- 创建bootstrap token并在托管集群之间交换此令牌。
- 在每个托管集群上为默认的 CephBlockPool 启用mirroring。
- 使用 DRPolicy 中的 replication interval 在 primary 托管集群和 secondary 托管集群上创建 VolumeReplicationClass。
- 在每个托管集群上创建（使用 MCG）对象存储桶，用于存储 PVC 和 PV 元数据。
- 在 Hub 集群上的 openshift-operators 项目中为每个对象桶创建 Secret。
- Hub 集群上的ramen-hub-operator-config ConfigMap中 s3StoreProfiles 条目会被做相应修改。
- OpenShift DR Cluster operator 将部署在每个托管集群上的 openshift-dr-system 项目中。
- Hub 集群上项目 openshift-operators 中的object buckets Secrets 将被复制到托管集群的 openshift-dr-system 项目中。
- s3StoreProfiles 条目将被复制到托管集群并用于修改 openshift-dr-system 项目中的 ramen-dr-cluster-operator-config ConfigMap。

要验证是否已成功创建 DRPolicy，请在 Hub 集群上为创建的每个 DRPolicy 资源运行此命令。

```
oc get drpolicy <drpolicy_name> -o jsonpath='{.status.conditions[].reason}{"\n"}'
```

## [Create Sample Application for DR testing](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-multisite-ramen.html#_create_sample_application_for_dr_testing)

## [Application Failover between managed clusters](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-multisite-ramen.html#_application_failover_between_managed_clusters)

Regional Disaster Recovery 的failover方法是基于应用程序的。以这种方式保护的每个应用程序都必须在应用程序命名空间中具有相应的 DRPlacementControl。

### 修改 DRPlacementControl 以进行failover
failover需要修改 DRPlacementControl。在 Hub 集群上导航到 Installed Operators，然后导航到 Openshift DR Hub Operator。选择 DRPlacementControl。添加 action 和 failoverCluster，如下所示。 failoverCluster 应该是secondary托管集群的 ACM 集群名称。

![ODR-411-DRPlacementControl-failover-Regional](../../../assets/images/posts/ODR-411-DRPlacementControl-failover-Regional.png)

## [Application Failback between managed clusters](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-Regional-ramen.html#_application_failback_between_managed_clusters)

failback操作与failover非常相似。failback是基于应用程序的，并且再次使用 DRPlacementControl 操作值来触发failback。在这种情况下，操作是重新定位到primary集群。

### Modify DRPlacementControl to failback

failback需要修改 DRPlacementControl。在 Hub 集群上导航到 Installed Operators，然后导航到 Openshift DR Hub Operator。选择 DRPlacementControl，如下所示。

![ODR-411-DRPlacementControl-instance](../../../assets/images/posts/ODR-411-DRPlacementControl-instance.png)

选择 `busybox-placement-1-drpc`，然后选择 YAML 表单。将操作修改为 Relocate，如下所示。

![ODR-411-DRPlacementControl-failback-Regional](../../../assets/images/posts/ODR-411-DRPlacementControl-failback-Regional.png)

使用以下命令检查应用程序 `busybox` 现在是否正在primary托管集群中运行。failback是到 pr​​eferredCluster，它应该是应用程序在failover操作之前运行的地方。

```
oc get pods,pvc -n busybox-sample

NAME          READY   STATUS    RESTARTS   AGE
pod/busybox   1/1     Running   0          60s

NAME                                STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS                  AGE
persistentvolumeclaim/busybox-pvc   Bound    pvc-79f2a74d-6e2c-48fb-9ed9-666b74cfa1bb   5Gi        RWO            ocs-storagecluster-ceph-rbd   61s
```

接下来，使用相同的命令，检查 busybox 是否正在secondary托管集群中运行。 busybox 应用程序不应再在此托管集群上运行。

```
oc get pods,pvc -n busybox-sample

No resources found in busybox-sample namespace.
```
