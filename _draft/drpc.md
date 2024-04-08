## drpc controller

### 触发
1. drpc的任何变化
2. MW的status发生变化时，在annotation中查找`drplacementcontrol.ramendr.openshift.io/drpc-name`和`drplacementcontrol.ramendr.openshift.io/drpc-namespace`
3. ManagedClusterView的status发生变化时，在annotation中查找`drplacementcontrol.ramendr.openshift.io/drpc-name`和`drplacementcontrol.ramendr.openshift.io/drpc-namespace`
4. PlacementRule被删除时，在annotation中查找`drplacementcontrol.ramendr.openshift.io/drpc-name`和`drplacementcontrol.ramendr.openshift.io/drpc-namespace`
5. Placement被删除时，在annotation中查找`drplacementcontrol.ramendr.openshift.io/drpc-name`和`drplacementcontrol.ramendr.openshift.io/drpc-namespace`
6. DRCluster有更新时，
   检查新的DRcluster status中是否含有maintenanceModes，如果有，并且FailoverActivated为true，并且老的DRCluster中没有active 相同的maintenanceModes， return true。
   如果drcluster正在被删除，查找所有使用该dr cluster的drp，再进一步找到所有drpc
   如果cluster没有正在被删除，找到正在向该cluster failover的drpc

### 动作
1. 构建DRPC instance
   - 获取drpc
   - 保存drpc.status 到 r.savedInstanceStatus 用以对比vrg 对status的变化
   - 初始化drpc status： 
     - Available: initiating
     - PeerReady: initiating
   - 获取placementrule或者placement，但是两者不能同时出现，必须和drpc在一个namespace中。如果没有被删除，需要做下列检查：
     - placementrule的Spec.SchedulerName必须是ramen，必须只指定一个cluster。
     - placement必须包含annotation "cluster.open-cluster-management.io/experimental-scheduling-disable"，必须只指定一个cluster。
   - 获取drp，drp不能被删除并且Validated condition应该为true
   - updateAndSetOwner
     - 设置placementobj annotation `drplacementcontrol.ramendr.openshift.io/drpc-namespace` `drplacementcontrol.ramendr.openshift.io/drpc-name`, 更新之后重新requeue
     - 设置drpc labels: `cluster.open-cluster-management.io/backup: ramen`, finailzer: `drpc.ramendr.openshift.io/finalizer`, annotation `drplacementcontrol.ramendr.openshift.io/app-namespace: vrgnamespace`
     - 设置placement finalizer `drpc.ramendr.openshift.io/finalizer`
   - ensureDRPCStatusConsistency
   - 创建drpc instance
     - 通过mcv获取vrgs

2. 处理drpc或者placementobect的删除
   1. 如果drpc包含finalizer： drpc.ramendr.openshift.io/finalizer，处理drpc的删除
      - 如果drpc.Spec.PreferredCluster 等于“”，获取clone的PlacementRule并删除： drpc-plrule-<drpc-name>-<drpc-namespace>
      - 清理，volsync的相关资源（policy/plrule/binding）
      - 清理mw
      - 清理mcv
   2. 如果placement obj包含finalizer： `drpc.ramendr.openshift.io/finalizer`， 删除该finailizer
   3. 删除drpc的finalizer `drpc.ramendr.openshift.io/finalizer`
   
3. reconcileDRPCInstance: 对比drpc.UID和mcv上的annotation `drplacementcontrol.ramendr.openshift.io/drpc-uid`
4. 确保managed cluster上创建的vrg是由该drpc instance管理的
5. startprocessing: 处理placement，依据Spec.Action
   -  runinitialDeployment
      - 获取HomeCluster (getHomeClusterForInitialDeploy)： d.instance.Spec.PreferredCluster, 不能为空
      - 检查homecluster上是否已经部署了vrg (isDeployed)， 如果在其他cluster部署了，则需要更新d.instance.Status.PreferredDecision （updatePreferredDecision） 。 并确保VRGMW也在其他cluster上，并requeue （ensureVRGManifestWork）
      - 如果还没有部署或者placement还没更新
        - 开始部署： create primary VRG ManifestWork
        - 更新 placementdecision
      - 确保VRGMW在cluster已经创建
      - 确保Volsync已经设置
        - 检查vrg中vrg.Status.ProtectedPVCs的ProtectedByVolSync属性判断是否需要volsync
        - ensureVolSyncReplicationCommon
          - 创建dst vrg，但是没有rdspec。
          - 在hub上创建volsync secret，并分发在managed cluster
        - ensureVolSyncReplicationDestination
          - 更新dstvrg，使其包含sourc vrg的Status.ProtectedPVCs
      - 更新drpc的Status.PreferredDecision
   -  failover
      -  获取d.instance.Spec.FailoverCluster，且不能为空
      -  如果在FailoverCluster上存在vrg并且是primary
         -  updatePreferredDecision
            -  更新d.instance.Status.PreferredDecision to d.instance.Spec.PreferredCluster
         -  setDRState： FailedOver
         -  checkReadinessAfterFailover：Failover cluster VRG condition DataReady&ClusterDataReady 必须为true
         -  ensureActionCompleted
            -  ensureVRGManifestWork： 
               -  在failover cluster上查找cache VRG，获取cachedVrg.Spec.ReplicationState
               -  创建或者更新vrg
            -  ensurePlacement
               -  确保Placement decistion等于failover cluster
            -  setProgression： Cleaning Up
            -  ensureCleanupAndVolSyncReplicationSetup
               -  clean RD spec in vrg mw in failover cluster
               -  EnsureCleanup
                  -  cleanupForVolSync: change vrg in other clusters to secondary
                  -  (volrep only)cleanupSecondaries: delete vrgmw,vrg,mca for other clusters
               -  EnsureVolSyncReplicationSetup
                  -  setup vrg without rd spec
                  -  create secret and propagate
                  -  update vrg with rd with vrg from status
            -  setProgression： Completed
            -  setActionDuration
      - 如果在FailoverCluster上存在vrg不是primary，但是placement已经更新为failovercluster，则需等待failovercluster的vrg变为primary
      - 否则，执行switchToFailoverCluster
        - setDRState： FailingOver
        - （只对MDR有效）获取当前homecluster
          - 先从placement decision中获取
          - 再尝试从d.instance.Status.PreferredDecision.ClusterName
          - 再尝试获取peer cluster
        - checkFailoverPrerequisites
          -  regionalDR：只检查failover cluster，因为其他cluster可能联系不上
             -  只和mmode有关系，可以从s3上读取上一次的primary vrg
        - setProgression： FailingOverToCluster
        - switchToCluster
          - createVRGManifestWorkAsPrimary: 更新或者创建 FailoverCluster上的vrg为primary
          - setProgression： WaitingForResourceRestore
          - ensureClusterDataRestored
            - FailoverCluster上的vrg 的ClusterDataReady状态必须为 ConditionTrue
          - updateUserPlacementRule： 更新placement decision 为 FailoverCluster
          - setProgression：UpdatedPlacement
        - updatePreferredDecision
          - 更新d.instance.Status.PreferredDecision to d.instance.Spec.PreferredCluster
        - setDRState： FailedOver
   -  relocate
      -  获取d.instance.Spec.PreferredCluster
      -  快速验证
         -  preferredCluster不能为空
         -  至少含有一个vrg，并且只有一个vrg是primary
         -  获取当前vrg primary的cluster： curHomeCluster
      - 如果preferedCluster含有primary vrg。完成
         - updatePreferredDecision： d.instance.Status.PreferredDecision to PreferredCluster
         - setDRState: Relocated
         - ensureActionCompleted
            -  ensureVRGManifestWork： 
               -  在prefer cluster上查找cache VRG，获取cachedVrg.Spec.ReplicationState
               -  创建或者更新vrg
            -  ensurePlacement
               -  确保Placement decistion等于prefer cluster
            -  setProgression： Cleaning Up
            -  ensureCleanupAndVolSyncReplicationSetup
               -  clean RD spec in vrg mw in prefer cluster
               -  EnsureCleanup
                  -  cleanupForVolSync: change vrg in other clusters to secondary
                  -  (volrep only)cleanupSecondaries: delete vrgmw,vrg,mca for other clusters
               -  EnsureVolSyncReplicationSetup
                  -  setup vrg without rd spec
                  -  create secret and propagate
                  -  update vrg with rd with vrg from status
            -  setProgression： Completed
            -  setActionDuration
      - setStatusInitiating： 设置初始状态
      - 如果preferedCluster和curHomeCluster不相等，并且 readyToSwitchOver 为false，则requeue
         - 如果DR policy是Metro：
         - 检查vrg的DataReady和ClusterDataProtected为true
      - 如果drpc getLastDRState不是ralocating并且PeerReady Condition为false。 则requeue
      - 如果preferedCluster和curHomeCluster不相等，但是 readyToSwitchOver 为true
         - quiesceAndRunFinalSync(curHomeCluster)
            - prepareForFinalSync（curHome）
               - 如果 vrg.Status.PrepareForFinalSyncComplete 为 false，接下去执行
               - updateVRGToPrepareForFinalSync（curHome）：
                 - getVRGFromManifestWork
                 - 更改vrg.Spec.PrepareForFinalSync = true，vrg.Spec.RunFinalSync = false
                 - requeue：给vrg充足时间to prepare
            - 获取ClusterDecision：d.reconciler.getClusterDecision(d.userPlacement)，如果clusterDecision不 为空
               - setDRState： Relocating
               - setProgression： ClearingPlacement
               - clearUserPlacementRuleStatus：清空placement decision
            - runFinalSync
               - 如果vrg.Status.FinalSyncComplete为false，接下去执行
               - updateVRGToRunFinalSync：更改vrg.Spec.PrepareForFinalSync = false，vrg.Spec.RunFinalSync  = true
            - setProgression： FinalSyncComplete
      - relocate： relocate(preferredCluster, preferredClusterNamespace, rmn.Relocating)
         - setDRState：Relocating
         - setupRelocation
            - ensureVRGIsSecondaryEverywhere： 确保除了preferredCluster之外的所有cluster都是secondary
            - ensureDataProtectedOnCluster：确保除了preferredCluster之外的所有cluster的vrg DataProtected都为true
         - switchToCluster
            - createVRGManifestWorkAsPrimary: 更新或者创建 preferredCluster上的vrg为primary
            - setProgression： WaitingForResourceRestore
            - ensureClusterDataRestored
               - preferredCluster上的vrg 的ClusterDataReady状态必须为 ConditionTrue
            - updateUserPlacementRule： 更新placement decision 为 preferredCluster
            - setProgression：UpdatedPlacement
         - updatePreferredDecision：更新d.instance.Status.PreferredDecision为PreferredCluster
         - setDRState： Relocated


