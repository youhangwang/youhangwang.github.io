---
title: ODF Metro Disaster Recovery
tags: ODF MetroDR
---

本文的目的是详细说明Metro DR的步骤和命令，MetroDR能够将应用程序从一个 OpenShift Container Platform (OCP) 集群failover到另一个集群，然后再将同一应用程序failback到原始主集群。在这种情况下，将使用 Red Hat Advanced Cluster Management 或 RHACM 创建或导入 OCP 集群，并且 OCP 集群之间的距离限制小于 10 毫秒 RTT 延迟。

应用程序的持久存储将由外部的两个 Stretched Red Hat Ceph 存储 (RHCS) 集群提供。在站点中断的情况下，还需要在第三个Location（不同于部署 OCP 集群的位置）部署一个具有存储监控服务的仲裁节点来为 RHCS 集群建立仲裁。第三个位置没有距离限制，从存储集群连接到 OCP 实例可以有 100+ RTT 延迟。

请注意，本文使用的 OpenShift Data Foundation (ODF) 版本是 v4.11， RHACM的版本是 v2.5。除了这两个称为托管集群的集群之外，目前还需要第三个 OCP 集群，即高级集群管理 (ACM) Hub cluster。

一般情况下，执行MetroDR的具体步骤包括：

1. 在 hub 集群上安装 ACM Operator。
  - 创建 OCP hub 集群后，从 OperatorHub 中安装 ACM Operator。在 Operator 和关联的 Pod 运行后，创建 `MultiClusterHub` 资源。
2. 创建托管 OCP 集群或将其导入 ACM hub cluster。
  - 使用 RHACM 控制台导入或创建两个具有足够 ODF 资源（计算节点、内存、cpu）的托管集群。
3. 使用 Arbiter 安装 Red Hat Ceph Storage Stretch Cluster。
  - 使用stretched mode正确设置部署在两个不同数据中心的 Ceph 集群。
4. 在托管集群上安装 ODF 4.11。
  - 在priamry 和 secondary OCP 集群上安装 ODF 4.11，并将两个实例连接到stretched Ceph 集群。
5. 在 ACM hub 集群上安装 `ODF Multicluster Orchestrator`。
  - 从 ACM hub 集群上的 OperatorHub 安装 `ODF Multicluster Orchestrator`。还将安装 `OpenShift DR Hub Operator`(Ramen Hub Operator)。
6. 配置 S3 端点之间的 SSL 访问
  - 如果托管 OpenShift 集群未使用有效证书，则必须通过创建包含证书的 user-ca-bundle ConfigMap 来完成此步骤。
7. 启用多集群 Web 控制台。
  - 这是创建 `DRPolicy` 之前需要的一项新的`技术预览`功能。它仅在 ACM 所在的 Hub 集群上需要。
8. 创建一个或多个 `DRPolicy`
  - 使用 All Clusters Data Services UI 通过选择策略将应用到的两个托管集群来创建 DRPolicy。
9. 验证是否安装了 `OpenShift DR Cluster operator`(Ramen Cluster Operator)。
  - 创建第一个 `DRPolicy` 后，将触发在 UI 中选择的两个托管集群上创建 `DR Cluster operators`。
10. 配置 DRClusters 以实现Fencing自动化。
  - 此配置是为在应用程序故障转移之前启用 Fenced 做准备。 DRCluster 资源将在 Hub 集群上进行编辑，以包含一个新的 CIDR 部分和额外的 DR 注释。
11. 使用 ACM 控制台创建示例应用程序。
  - 使用来自 github.com/RamenDR/ocm-ramen-samples 的示例应用程序示例创建用于故障转移和故障恢复测试的 busybox deployment。
12. 验证示例应用程序部署。
  - 在两个受管集群上使用 CLI 命令验证应用程序是否正在运行。
13. 将 DRPolicy 应用于示例应用程序。
  - 使用 All Clusters Data Services UI 将新的 DRPolicy 应用到示例应用程序。应用后，在 Hub 集群的`应用程序`命名空间中创建 `DRPlacementControl` 资源。
14. 将示例应用程序故障转移到secondary托管集群。
  - fencing primary 托管集群后，修改Hub Cluster上的application DRPlacementControl资源，添加Failover动作，指定failoverCluster触发failover。
15. 将示例应用程序failback到primary托管集群。
  - 在取消对主要托管集群的fence并重新启动工作节点后，修改 Hub 集群上的应用程序 `DRPlacementControl` 资源并将操作更改为 `Relocat`e 以触发failback到 preferredCluster。

## [Deploy and Configure ACM for Multisite connectivity](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-metro-ramen.html#_deploy_and_configure_acm_for_multisite_connectivity)

## [Red Hat Ceph Storage Installation](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-metro-ramen.html#_red_hat_ceph_storage_installation)

## [OpenShift Data Foundation Installation](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-metro-ramen.html#_openshift_data_foundation_installation)

## [Install ODF Multicluster Orchestrator Operator on Hub cluster](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-metro-ramen.html#_install_odf_multicluster_orchestrator_operator_on_hub_cluster)

## [Configure SSL access between S3 endpoints](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-metro-ramen.html#_configure_ssl_access_between_s3_endpoints)

## [Enabling Multicluster Web Console](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-metro-ramen.html#_enabling_multicluster_web_console)

## [Create Data Policy on Hub cluster](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-metro-ramen.html#_create_data_policy_on_hub_cluster)

MetroDR 使用 Hub 集群上的 DRPolicy 资源来跨托管集群进行Failover和relocate workload。 DRPolicy 需要一组连接到同一个 Red Hat Ceph Storage 集群的两个 DRCluster 或peer集群。 ODF MultiCluster Orchestrator Operator 有助于通过多集群 Web 控制台创建每个 DRPolicy 和相应的 DRCluster。

在 Hub 集群上导航到All Clusters。然后导航到Data services菜单下的Data policies。如果这是创建的第一个 DRPolicy，则将在页面底部看到创建 Create DRpolicy。

![MCO-create-first-drpolicy](../../../assets/images/posts/MCO-create-first-drpolicy.png)

单击创建 DRPolicy。从托管集群列表中选择希望参与 DRPolicy 的集群，并为策略指定一个唯一名称（即 ocp4perf1-ocp4perf2）。

![MCO-drpolicy-selections](../../../assets/images/posts/MCO-drpolicy-selections.png)

Replication policy的灰色下拉选项将根据所选的 OpenShift 集群自动选择为`sync`。选择Create。这应该在 Hub 集群上创建两个 DRCluster 资源和 DRPolicy。此外，当创建初始 DRPolicy 时，将发生以下情况：
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

## [Configure DRClusters for Fencing automation](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-metro-ramen.html#_configure_drclusters_for_fencing_automation)

### Add Node IP addresses to DRClusters

第一步是获取托管集群中所有节点的 IP 地址。这可以通过在primary托管集群和secondary托管集群中运行此命令来完成：
```
oc get nodes -o jsonpath='{range .items[*]}{.status.addresses[?(@.type=="ExternalIP")].address}{"\n"}{end}'
```

获得 IP 地址后，就可以为每个托管集群修改 DRCluster 资源。首先，需要获取需要修改 DRCluster 的名称。在 Hub 集群上执行此命令：
```
oc get drcluster
```

现在，每个 DRCluster 都需要编辑，添加IP 地址。
```
oc edit drcluster <drcluster_name>

apiVersion: ramendr.openshift.io/v1alpha1
kind: DRCluster
metadata:
[...]
spec:
  s3ProfileName: s3profile-<drcluster_name>-ocs-external-storagecluster
  ## Add this section
  cidrs:
    -  <IP_Address1>/32
    -  <IP_Address2>/32
    -  <IP_Address3>/32
    -  <IP_Address4>/32
    -  <IP_Address5>/32
    -  <IP_Address6>/32
[...]
```

### Add Fencing Annotations to DRClusters

将以下注释添加到所有 DRCluster 资源。这些注释包括`NetworkFence`资源所需的详细信息（在测试应用程序failover之前）：
```
oc edit drcluster <drcluster_name>

apiVersion: ramendr.openshift.io/v1alpha1
kind: DRCluster
metadata:
  ## Add this section
  annotations:
    drcluster.ramendr.openshift.io/storage-clusterid: openshift-storage
    drcluster.ramendr.openshift.io/storage-driver: openshift-storage.rbd.csi.ceph.com
    drcluster.ramendr.openshift.io/storage-secret-name: rook-csi-rbd-provisioner
    drcluster.ramendr.openshift.io/storage-secret-namespace: openshift-storage
[...]
```

## [Create Sample Application for DR testing](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-metro-ramen.html#_create_sample_application_for_dr_testing)

## [Application Failover between managed clusters](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-metro-ramen.html#_application_failover_between_managed_clusters)

Metro Disaster Recovery 的failover方法是基于应用程序的。以这种方式保护的每个应用程序都必须在应用程序命名空间中具有相应的 DRPlacementControl。

### 启用防护
为了对当前运行应用程序的 OpenShift 集群进行故障转移，必须阻止所有应用程序与ODF 外部存储集群进行通信。这是防止两个受管集群同时写入同一持久卷所必需的。Fence 的 OpenShift 集群是应用程序当前运行的集群。在 Hub 集群上编辑此集群的 `DRCluster` 资源。
```
oc edit drcluster <drcluster_name>


apiVersion: ramendr.openshift.io/v1alpha1
kind: DRCluster
metadata:
[...]
spec:
  ## Add this line
  clusterFence: Fenced
  cidrs:
  [...]
[...]
```

然后在Hub 集群中验证primary托管集群的的fencing状态：
```
oc get drcluster.ramendr.openshift.io <drcluster_name> -o jsonpath='{.status.phase}{"\n"}'

Fenced
```

### 修改 DRPlacementControl 以进行failover
failover需要修改 DRPlacementControl。在 Hub 集群上导航到 Installed Operators，然后导航到 Openshift DR Hub Operator。选择 DRPlacementControl。添加 action 和 failoverCluster，如下所示。 failoverCluster 应该是secondary托管集群的 ACM 集群名称。

![ODR-411-DRPlacementControl-failover-metro](../../../assets/images/posts/ODR-411-DRPlacementControl-failover-metro.png)

## [Application Failback between managed clusters](https://red-hat-storage.github.io/ocs-training/training/ocs4/odf411-metro-ramen.html#_application_failback_between_managed_clusters)

failback操作与failover非常相似。failback是基于应用程序的，并且再次使用 DRPlacementControl 操作值来触发failback。在这种情况下，操作是重新定位到primary集群。

### Disable Fencing

在failback或relocated操作成功之前，primary托管集群的 DRCluster 必须取消fencing。待 Unfenced 的 OpenShift 集群是当前未运行应用程序且之前已 Fenced 的集群。在 Hub 集群上编辑此集群的 DRCluster:

```
oc edit drcluster <drcluster_name>

apiVersion: ramendr.openshift.io/v1alpha1
kind: DRCluster
metadata:
[...]
spec:
  cidrs:
  [...]
  ## Modify this line
  clusterFence: Unfenced
  [...]
[...]
```

#### Reboot OCP nodes that were Fenced
此步骤是必需的，因为先前fenced的集群（在本例中为primary托管集群）上的某些应用程序 Pod 处于不健康状态（例如 CreateContainerError、CrashLoopBackOff）。通过一次重启该集群的所有 OpenShift 节点，可以最轻松地解决此问题。

在所有 OpenShift 节点重新启动并再次处于就绪状态后，通过在primary托管集群上运行此命令来验证所有 Pod 是否处于健康状态。

```
oc get pods -A | egrep -v 'Running|Completed'
```

在继续下一步之前，此查询的输出应该是零个 Pod。

#### Validate Fencing status

现在 Unfenced 集群处于健康状态，验证primary托管集群的 Hub 集群中的防护状态。

```
oc get drcluster.ramendr.openshift.io <drcluster_name> -o jsonpath='{.status.phase}{"\n"}'

Unfenced
```

### Modify DRPlacementControl to failback

failback需要修改 DRPlacementControl。在 Hub 集群上导航到 Installed Operators，然后导航到 Openshift DR Hub Operator。选择 DRPlacementControl，如下所示。

![ODR-411-DRPlacementControl-instance](../../../assets/images/posts/ODR-411-DRPlacementControl-instance.png)

选择 `busybox-placement-1-drpc`，然后选择 YAML 表单。将操作修改为 Relocate，如下所示。

![ODR-411-DRPlacementControl-failback-metro](../../../assets/images/posts/ODR-411-DRPlacementControl-failback-metro.png)

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
