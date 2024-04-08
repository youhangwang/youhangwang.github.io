Finalizer：
- `volumereplicationgroups.ramendr.openshift.io/pvc-vr-protection` on pvc
- `volumereplicationgroups.ramendr.openshift.io/vrg-protection` on vrg
- `kubernetes.io/pvc-protection` on pvc

Annotations:
- `volumereplicationgroups.ramendr.openshift.io/vr-protected` on pvc
- `volumereplicationgroups.ramendr.openshift.io/vr-archived` on pvc
- `drplacementcontrol.ramendr.openshift.io/do-not-delete-pvc` on vrg
- `apps.open-cluster-management.io/do-not-delete` on pvc

Labels
- `app.kubernetes.io/created-by: volsync` on pvc

## 触发
- VRG: 
  1. Update: generation, annotation and label changed
  2. Create: all
  3. Delete: all
- PVC: 
  1. Create: All
  2. Update: 
    - (仅限于volrep使用) 不requeue， finalizer `volumereplicationgroups.ramendr.openshift.io/pvc-vr-protection`的增加和删除
    - (仅限于volrep使用) 不requeue, `volumereplicationgroups.ramendr.openshift.io/vr-protected` 和 `volumereplicationgroups.ramendr.openshift.io/vr-archived` annotation的添加
    - (仅限于volrep使用) 不requeue, pvc要被删除了但是还没不包含finzlizer `volumereplicationgroups.ramendr.openshift.io/pvc-vr-protection`
    - 不requeue, newpvc.Status.Phase != corev1.ClaimBound
    - requeue，label的任何变化
    - requeue，spec的任何变化
    - requeue, PVC.Status.Phase 改为 Bound
    - requeue, `kubernetes.io/pvc-protection` finalizer remove(pvc not in use by pod)
  3. Delete: all
  Map: get vrg with label selector for the pvc 
- configmap: `ramen-dr-cluster-operator-config`, requeue all vrgs
- (仅限于volrep使用) Own VolumeReplication

## actions

1. 构建 VRGInstance
  - 获取 vrg CR
  - 获取 ramen agent 的 ramenconfig
  - 构建 VSHandler
  - 初始化 v.instance.Status.ProtectedPVCs
    - 如果其为 nil 则设置为 []ramendrv1alpha1.ProtectedPVC{}
  - 复制一份vrg CR status至v.savedInstanceStatus，以便后面的对比。如果相同，则无需调用Update方法更新status。
  - v.instance.Status.ProtectedPVCs 中各个pvc的Namespace如果没有设置的话，则将其设置为VRG的namespace。
  - 初始化v.instance.Status.Conditions
    - DataReady： Initializing
    - DataProtected： Initializing
    - ClusterDataReady： Initializing
    - ClusterDataProtected： Initializing
   
2. processVRG
  - validateVRGState（instance.Spec.ReplicationState 必须等于 primary 和 secondary 中的一个）
  - validateVRGMode (v.instance.Spec.Async 和 v.instance.Spec.Sync 不能全都为nil)
  - （分叉）RecipeElementsGet: 赋值 v.recipeElements，如果没有设置recipe，则只包含vrg中的pvc selector。如果有设置recipe，则包含其他Selector。
  - （分叉）updatePVCList： （此步骤目的在于仅更新v.volSyncPVCs和v.volRepPVCs）
    - 使用pvc selector获取pvc，如果enable了volsync，则需排除带有label `app.kubernetes.io/created-by: volsync`的pvc
    - (仅限于volrep)如果不是Async或者Volsync是disable的状态，将获取到的pvcs赋值给v.volRepPVCs
    - (仅限于volrep, cephfs VRG 不会被删除)如果VRG正在被删除，根据v.instance.Status.ProtectedPVCs 的属性 ProtectedByVolSync 将获取到的PVC分类到 v.volSyncPVCs 和 v.volRepPVCs
    - 如果没有找到replClass，将获取到的pvcs赋值给v.volSyncPVCs
    - 通过storageClass.Provisioner 是否等于 replicationClass.Spec.Provisioner 判断pvc是否属于cephfs，将其分类至 v.volSyncPVCs 和 v.volRepPVCs
  - 通过遍历vrg.Spec.S3Profiles，创建v.s3StoreAccessors
  - （分叉）(仅限于volrep, cephfs VRG 不会被删除)如果 vrg 被删除
    - disownPVCs： 
      - 检查VRG 的 annotation `drplacementcontrol.ramendr.openshift.io/do-not-delete-pvc` 如果不为true，则直接返回nil
      - 为volSyncPVCs中的每一个pvc 删除OwnerReferences， 删除`apps.open-cluster-management.io/do-not-delete` Annotations.
    - deleteVRGHandleMode: 删除VR
    - kubeObjectsProtectionDelete
    - 如果VRG的状态是primary： 从每个v.instance.Spec.S3Profiles中删除以v.namespacedname prefix的objects
  - 向 vrg 添加 finalizer `volumereplicationgroups.ramendr.openshift.io/vrg-protection`
  - 执行 processAsPrimary: 如果v.instance.Spec.ReplicationState 为 ramendrv1alpha1.Primary:
    - (分叉)pvcsDeselectedUnprotect ? (VolumeUnprotectionEnabledForAsyncVolSync)
    - (分叉)如果需要恢复ClusterData：如果是Final sync或者有ClusterDataReady，则无需恢复ClusterData
      - restorePVsAndPVCsForVolSync： 对每一个v.instance.Spec.VolSync.RDSpec，恢复 pv/pvc from volsync
        - 如果len(v.instance.Spec.VolSync.RDSpec) 为 0，则表示没有pvc需要restore。初始部署时，满足此条件，无需执行restore
        - 对于每一个v.instance.Spec.VolSync.RDSpec：
          - EnsurePVCfromRD:
            - 从RD中获取Latest Image
            - 从latest image中获取volumesnapshot，并添加label `volsync.backube/do-not-delete: true`
            - 如果是CopyMethodDirect，则获取PVC，如果是failover，则还需要将其rollback到last snapshot
              - 从latest snapshot中恢复read only pvc
              - 创建lrs和lrd用来恢复pvc到最新的snapshot
            - 如果不是，则直接从snapshot恢复pvc
            - 向PVC添加 rdSpec.ProtectedPVC.Annotations 中ocm相关的annotation
            - 向snapshot添加vrg的owner reference
          - 添加恢复的pvc到v.instance.Status.ProtectedPVCs
      - ClusterDataReady 设置为true
    - reconcileAsPrimary(分叉)
      - reconcileVolsyncAsPrimary
        - 如果len(v.volSyncPVCs) == 0，设置finalSyncPrepared为true
        - 删除残留的RD resoruce（带有label `volumereplicationgroups-owner: vrgname`, 却不在v.instance.Spec.VolSync.RDSpec， 并且不包含label `volsync.backube/do-not-delete: true`）
        - 为每一个pvc创建replicationsource: reconcilePVCAsVolSyncPrimary:
          - 如果pvc 不在 v.instance.Status.ProtectedPVCs， 则添加
          - 如果prepFinalSync或者copyMethodDirect，向pvc添加annotation `apps.open-cluster-management.io/do-not-delete`, 并设置vrg 为 owner
          - ReconcileRS （包含一个workaround），如果spec.runFinalSync 为true，则需检查pod是否还在背pod使用，如果没有pod使用，则创建或修改RS为手动模式，并检查手动模式是否完成
          - set ReplicationSourceSetup ready
        - 设置 finalSyncPrepared 为 true
      - vrgObjectProtect
        - upload vrg to minio for each s3profile
      - 如果vrg.Spec.PrepareForFinalSync为true，更新vrg.Status.PrepareForFinalSyncComplete = finalSyncPrepared.volSync
    - updateVRGConditionsAndStatus(TODO)
  - 否则执行 processAsSecondary:
    - 初始化：v.instance.Status.LastGroupSyncTime = nil 
    - reconcileAsSecondary(分叉)：volsync和volrep
      - reconcileVolSyncAsSecondary
        - 如果 v.instance.Spec.VolSync.RDSpec 没有设置, 则将v.instance.Status.ProtectedPVCs 中 protectedPVC.ProtectedByVolSync 为true的都移除
        - 重置 v.instance.Status.PrepareForFinalSyncComplete 和 v.instance.Status.FinalSyncComplete 为 false
        - 对于每一个 v.instance.Spec.VolSync.RDSpec， 调用 ReconcileRD 
          - 创建或者更新RD
          - 创建ServiceExporter
      - reconcileVolRepsAsSecondary
    - updateVRGConditionsAndStatus(TODO)
      - updateVRGConditions： VRGConditionTypeDataReady， VRGConditionTypeClusterDataProtected and VRGConditionDataProtected
      - updateVRGStatus
