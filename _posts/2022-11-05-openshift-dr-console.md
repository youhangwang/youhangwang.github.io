---
title: ODF Disaster Recovery Console
tags: ODF DR DRConsole
---

Openshift ACM console上提供了DR的UI，用户可以操作该UI实现ODF的MetroDR和RegionalDR的设置和操作。其中DR的UI是Openshift ACM console的plugin，其源码位于[odf-console](https://github.com/red-hat-storage/odf-console) github repo中。

<!--more-->

目前Console上只提供两种功能：

1. 创建DR Policy，并根据所选的cluster自动决定Replication Policy是 Async 还是 Sync。
2. 将DR Policy Apply给某一个application

## Create DR Policy

![drcreatedrpolicyUI.png](../../../assets/images/posts/drcreatedrpolicyUI.png)

UI 创建 DR Policy的[代码](https://github.com/red-hat-storage/odf-console/blob/c25c0bc00085deb00721964ce9077ffeaceefaa0/packages/mco/components/disaster-recovery/create-dr-policy/create-dr-policy.tsx#L138)

主要逻辑非常简单：

1. 根据UI界面所选内容在Hub cluster上创建cluster scoped DRPolicy:
```
      const payload: DRPolicyKind = {
        apiVersion: getAPIVersionForModel(DRPolicyModel),
        kind: DRPolicyModel.kind,
        metadata: { name: state.policyName },
        spec: {
          schedulingInterval:
            state.replication === REPLICATION_TYPE.ASYNC
              ? state.syncTime
              : '0m',
          drClusters: peerNames,
        },
      };
```
```
type DRPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DRPolicySpec   `json:"spec,omitempty"`
	Status DRPolicyStatus `json:"status,omitempty"`
}
type DRPolicySpec struct {
	// Important: Run "make" to regenerate code after modifying this file

	// scheduling Interval for replicating Persistent Volume
	// data to a peer cluster. Interval is typically in the
	// form <num><m,h,d>. Here <num> is a number, 'm' means
	// minutes, 'h' means hours and 'd' stands for days.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^\d+[mhd]$`
	SchedulingInterval string `json:"schedulingInterval,omitempty"`

	// Label selector to identify all the VolumeReplicationClasses.
	// This selector is assumed to be the same for all subscriptions that
	// need DR protection. It will be passed in to the VRG when it is created
	//+optional
	ReplicationClassSelector metav1.LabelSelector `json:"replicationClassSelector,omitempty"`

	// Label selector to identify all the VolumeSnapshotClasses.
	// This selector is assumed to be the same for all subscriptions that
	// need DR protection. It will be passed in to the VRG when it is created
	//+optional
	VolumeSnapshotClassSelector metav1.LabelSelector `json:"volumeSnapshotClassSelector,omitempty"`

	// List of DRCluster resources that are governed by this policy
	DRClusters []string `json:"drClusters"`
}
```
2. 在Hub cluster上，根据UI上所选择`drClusters`成员匹配是否已经存在包含相同的`drClusters`的cluster scoped mirrorPeer资源
   - 如果存在，则根具UI所选更改mirrorPeer的`/spec/schedulingIntervals`内容
   - 如果不存在，创建一个新的`mirrorPeer`
```
const payload: MirrorPeerKind = {
          apiVersion: getAPIVersionForModel(MirrorPeerModel),
          kind: MirrorPeerModel.kind,
          metadata: { generateName: 'mirror-peer-' },
          spec: {
            manageS3: true,
            type: state.replication,
            schedulingIntervals:
              state.replication === REPLICATION_TYPE.ASYNC
                ? [state.syncTime]
                : [],
            items: state.selectedClusters.map((cluster) => ({
              clusterName: cluster?.name,
              storageClusterRef: {
                name: cluster.storageClusterName,
                namespace: CEPH_STORAGE_NAMESPACE,
              },
            })),
          },
        };
        await k8sCreate({
          model: MirrorPeerModel,
          data: payload,
          cluster: HUB_CLUSTER_NAME,
        });
```

```
type MirrorPeer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MirrorPeerSpec   `json:"spec,omitempty"`
	Status MirrorPeerStatus `json:"status,omitempty"`
}

type MirrorPeerSpec struct {
	// Type represents the mode of DR operation (sync or async)
	// +kubebuilder:default=async
	// +kubebuilder:validation:Enum=async;sync
	Type DRType `json:"type"`

	// Items is a list of PeerRef.
	// +kubebuilder:validation:MaxItems=2
	// +kubebuilder:validation:MinItems=2
	Items []PeerRef `json:"items"`

	// SchedulingIntervals is a list of intervals at which mirroring snapshots are taken
	// +kubebuilder:validation:Optional
	SchedulingIntervals []string `json:"schedulingIntervals,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	ManageS3 bool `json:"manageS3,omitempty"`
}

type PeerRef struct {
	// ClusterName is the name of ManagedCluster.
	// ManagedCluster matching this name is considered
	// a peer cluster.
	ClusterName string `json:"clusterName"`
	// StorageClusterRef holds a reference to StorageCluster object
	StorageClusterRef StorageClusterRef `json:"storageClusterRef"`
}

type StorageClusterRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}
```
UI 会根据所选择cluster自动识别`REPLICATION_TYPE`:
```
      const cephFSIDs = state.selectedClusters.reduce((ids, cluster) => {
        if (cluster?.cephFSID !== '') {
          ids.add(cluster?.cephFSID);
        }
        return ids;
      }, new Set());

      ...

      dispatch({
        type: DRPolicyActionType.SET_REPLICATION,
        payload:
          isReplicationAutoDetectable && cephFSIDs.size <= 1
            ? REPLICATION_TYPE.SYNC
            : REPLICATION_TYPE.ASYNC,
      });
```
`cephFSIDs` 是从 `ACMManagedCluster` 中` cluster.status.clusterClaims`名为`cephfsid.odf.openshift.io` claim获取的：
```
  const cephFsidClaim = cluster?.status?.clusterClaims?.find(
    (claim) => claim.name === ClusterClaimTypes.CEPH_FSID
  );
  return {
    odfVersion: odfVersionClaim?.value || '',
    storageSystemName: storageSystemNameClaim?.value || '',
    storageClusterName: storageClusterNameClaim?.value || '',
    cephFSID: cephFsidClaim?.value || '',
    isValidODFVersion: isMinimumSupportedODFVersion(
      odfVersionClaim?.value || '',
      requiredODFVersion
    ),
```

## Apply DR Policy

![drapplydrpolicyUI.png](../../../assets/images/posts/drapplydrpolicyUI.png)

一旦选择一个或者多个application apply 该 DR Policy时，UI会执行如下动作：

1. 在hub cluster上为每一个选择的[Application](https://github.com/kubernetes-sigs/application/blob/master/config/crd/bases/app.k8s.io_applications.yaml)找到对应的PlacementRule(使用ACM部署application时在hub cluster上的application namespace中创建，此时spec中不包含schedulerName)，并将其中的`/spec/schedulerName`修改为`ramen`。
```
        const placementRuleId = app.id.split(':')[1];
        const appToPlsRuleMap = appToPlacementRuleMap?.[appId];
        const plsRule =
          appToPlsRuleMap?.placements?.[placementRuleId]?.placementRules;
        const patch = [
          {
            op: 'replace',
            path: '/spec/schedulerName',
            value: DR_SECHEDULER_NAME,
          },
        ];
        promises.push(
          k8sPatch({
            model: ACMPlacementRuleModel,
            resource: plsRule,
            data: patch,
            cluster: HUB_CLUSTER_NAME,
          })
        );
```

1. 对于每一个选择的Application，在hub cluster上都会根据PlacementRule和DrPolicy的内容创建DRPlacementControl:
  - Name:  `<PlacementRule_Name>-drpc`
  - Namespace: 同PlacementRule一样
  - Label：同PlacementRule一样
  - Spec.DrPolicyRef: 所选DR Policy的Name
  - Spec.PlacementRef: PlacementRule的Name
  - Spec.PreferredCluster: 按顺序在PlacementRule.status?.decisions中寻找包含在DrPolicy.spec.drClusters中的cluster
  - Spec.PvcSelector: 如果用户只选择了一个application，则需要用户在UI上指定Pvc label。如果用户选择了多个applications，则不会设置PvcSelector，表示application namespace中的所有pvc都会受到保护。


```
        promises.push(
          k8sCreate({
            model: DRPlacementControlModel,
            data: getDRPlacementControlKindObj(
              plsRule,
              resource,
              managedClusterNames,
              selectedPlacementRules?.length <= 1 ? labels : []
            ),
            cluster: HUB_CLUSTER_NAME,
          })
        );
        
        ...

const getDRPlacementControlKindObj = (
  plsRule: ACMPlacementRuleKind,
  resource: DRPolicyKind,
  managedClusterNames: string[],
  pvcSelectors: string[]
): DRPlacementControlKind => ({
  apiVersion: getAPIVersionForModel(DRPlacementControlModel),
  kind: DRPlacementControlModel.kind,
  metadata: {
    name: `${getName(plsRule)}-drpc`,
    namespace: getNamespace(plsRule),
    labels: getLabels(plsRule),
  },
  spec: {
    drPolicyRef: {
      name: getName(resource),
    },
    placementRef: {
      name: getName(plsRule),
      kind: plsRule?.kind,
    },
    preferredCluster: clusterMatch(plsRule, managedClusterNames)?.[
      'clusterName'
    ],
    pvcSelector: {
      matchLabels: objectify(pvcSelectors),
    },
  },
});
```
```
type DRPlacementControlSpec struct {
	// PlacementRef is the reference to the PlacementRule used by DRPC
	PlacementRef v1.ObjectReference `json:"placementRef"`

	// DRPolicyRef is the reference to the DRPolicy participating in the DR replication for this DRPC
	DRPolicyRef v1.ObjectReference `json:"drPolicyRef"`

	// PreferredCluster is the cluster name that the user preferred to run the application on
	PreferredCluster string `json:"preferredCluster,omitempty"`

	// FailoverCluster is the cluster name that the user wants to failover the application to.
	// If not sepcified, then the DRPC will select the surviving cluster from the DRPolicy
	FailoverCluster string `json:"failoverCluster,omitempty"`

	// Label selector to identify all the PVCs that need DR protection.
	// This selector is assumed to be the same for all subscriptions that
	// need DR protection. It will be passed in to the VRG when it is created
	PVCSelector metav1.LabelSelector `json:"pvcSelector"`

	// Action is either Failover or Relocate operation
	Action DRAction `json:"action,omitempty"`

	// +optional
	KubeObjectProtection *KubeObjectProtectionSpec `json:"kubeObjectProtection,omitempty"`
}
```

## Summary

![Summary](../../../assets/images/posts/odr.drawio.png)