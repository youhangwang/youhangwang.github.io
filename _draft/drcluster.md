## 触发


1. DRCluster
2. Configmap : 
   Map: name `ramen-hub-operator-config`, requeue all the drclusters
3. secret：
   Predict: CreateOrDeleteOrResourceVersionUpdatePredicate
   Map: requeue drcluster with the s3profile which has the same secret ref
4. DRPlacementControl
   predict: delete 
            or Spec.Action from non-failover to failover
            or Spec.FailoverCluster changed
            or Status.ConditionAvailable from false to true
   Map: requeue failover cluster in the drpc
5. ManifestWork
   predict: all the mw with status changed
   Map: requeue annotation `drcluster.ramendr.openshift.io/drcluster-name` 
6. ManagedClusterView
   Predict: all the mcv with status changed
   Map: requeue annotation `drcluster.ramendr.openshift.io/drcluster-name` 


## 动作

1. set starting conditions
2. 获取ramen config
3. add label `cluster.open-cluster-management.io/backup: ramen` and finalizer `drclusters.ramendr.openshift.io/ramen`
4. Deploy DR Cluster: 
   deploy dr cluster operator based on manifestwork: `ramen-dr-cluster`
   deploy volsync based on managed cluster addon: `volsync`
5. 验证dr cluster的cidr
6. 处理drcluster.Spec.ClusterFence
7. 处理mMode？
8. validate S3 profile
9. Update status
   