

## 触发
Watch DRpolicy的变化，一旦ramen-system namespace的中名为ramen-hub-operator-config的configmap发生变化，则requeue所有的drp
ramen-system namespace的任何secret发生变化，则requeue所有的drp，drcluster发生变化，则requeue所有相关的drp

1. DRPolicy
2. Configmap : 
   predict：resourceversion变化， 
   map：名为"ramen-hub-operator-config"，添加label "cluster.open-cluster-management.io/backup" = "ramen"，requeue 所有drp
3. secret：
   predict：CreateOrDeleteOrResourceVersionUpdate
   map：requeue all drps if the secret is in ramen namespace
4. drcluster
   predict：CreateOrDeleteOrResourceVersionUpdate
   map：requeue drp 包含该 drcluster 

## 动作
1. 读取Configmap `ramen-hub-operator-config`, 解析data中的`ramen_manager_config.yaml`
2. 如果dr policy正在被删除：
   如果还有drpc，则阻止drp的删除
   undeploy drp: only delete the secret which is not needed
   remove finalizer
3. 验证DR policy
   DRpolicy中的DRclusters不能为0
   DRCluster CR是否存在
   检查和其他policy是否冲突
4. 添加drp label "cluster.open-cluster-management.io/backup：  ramen"和finalizer "drpolicies.ramendr.openshift.io/ramen"
5. 像drcluster下发secret
   
   从drp中获取包含的全部cluster，根据cluster name从相应的drcluster中获取Spec.S3ProfileName
   根据S3ProfileName在rmnCfg中获取secretname

   使用policyconfig下发secret




- drpc example:
```
fedora ➜  ~ kubectl get cm ramen-hub-operator-config -n ramen-system --context hub -o yaml
data:
  ramen_manager_config.yaml: |
    apiVersion: ramendr.openshift.io/v1alpha1
    kind: RamenConfig
    health:
      healthProbeBindAddress: :8081
    metrics:
      bindAddress: 127.0.0.1:9289
    webhook:
      port: 9443
    leaderElection:
      leaderElect: true
      resourceName: hub.ramendr.openshift.io
    ramenControllerType: dr-hub
    maxConcurrentReconciles: 50
    drClusterOperator:
      deploymentAutomationEnabled: true
      s3SecretDistributionEnabled: true
      channelName: alpha
      packageName: ramen-dr-cluster-operator
      namespaceName: ramen-system
      catalogSourceName: ramen-catalog
      catalogSourceNamespaceName: ramen-system
      clusterServiceVersionName: ramen-dr-cluster-operator.v0.0.1
    kubeObjectProtection:
      veleroNamespaceName: velero
    volSync:
      disabled: false
    s3StoreProfiles:
    - s3ProfileName: minio-on-dr1
      s3Bucket: bucket
      s3CompatibleEndpoint: http://192.168.124.246:30000
      s3Region: us-west-1
      s3SecretRef:
        name: ramen-s3-secret-dr1
        namespace: ramen-system
      veleroNamespaceSecretKeyRef:
        key: cloud
        name: cloud-credentials
    - s3ProfileName: minio-on-dr2
      s3Bucket: bucket
      s3CompatibleEndpoint: http://192.168.124.174:30000
      s3Region: us-east-1
      s3SecretRef:
        name: ramen-s3-secret-dr2
        namespace: ramen-system
      veleroNamespaceSecretKeyRef:
        key: cloud
        name: cloud-credentials
kind: ConfigMap
metadata:
  labels:
    cluster.open-cluster-management.io/backup: resource
  name: ramen-hub-operator-config
  namespace: ramen-system
```