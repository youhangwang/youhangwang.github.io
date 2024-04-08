## drenv start envs/regional-dr.yaml
|Cluster|Component|
|-|-|
|Hub|cert-manager, coredns, olm, OCM, submariner|
|dr1|cert-manager, coredns, olm, OCM, submariner, csi-addons, snapshot-controller, minio, rook-ceph, velero, volsync|
|dr2|cert-manager, coredns, olm, OCM, submariner, csi-addons, snapshot-controller, minio, rook-ceph, velero, volsync|


```
fedora ➜  ~ kubectl get deploy -A --context hub
NAMESPACE                     NAME                                       READY   UP-TO-DATE   AVAILABLE   AGE
cert-manager                  cert-manager                               1/1     1            1           30m
cert-manager                  cert-manager-cainjector                    1/1     1            1           30m
cert-manager                  cert-manager-webhook                       1/1     1            1           30m
kube-system                   coredns                                    1/1     1            1           35m
olm                           catalog-operator                           1/1     1            1           29m
olm                           olm-operator                               1/1     1            1           29m
olm                           packageserver                              2/2     2            2           28m
open-cluster-management-hub   cluster-manager-addon-manager-controller   1/1     1            1           34m
open-cluster-management-hub   cluster-manager-placement-controller       1/1     1            1           34m
open-cluster-management-hub   cluster-manager-registration-controller    1/1     1            1           34m
open-cluster-management-hub   cluster-manager-registration-webhook       1/1     1            1           34m
open-cluster-management-hub   cluster-manager-work-webhook               1/1     1            1           34m
open-cluster-management       cluster-manager                            1/1     1            1           34m
open-cluster-management       governance-policy-addon-controller         1/1     1            1           33m
open-cluster-management       governance-policy-propagator               1/1     1            1           33m
open-cluster-management       multicluster-operators-appsub-summary      1/1     1            1           33m
open-cluster-management       multicluster-operators-channel             1/1     1            1           33m
open-cluster-management       multicluster-operators-placementrule       1/1     1            1           33m
open-cluster-management       multicluster-operators-subscription        1/1     1            1           33m
open-cluster-management       ocm-controller                             1/1     1            1           31m
submariner-operator           submariner-operator                        1/1     1            1           34m
```

```
fedora ➜  ~ kubectl get deploy -A --context dr1
NAMESPACE                             NAME                            READY   UP-TO-DATE   AVAILABLE   AGE
cert-manager                          cert-manager                    1/1     1            1           34m
cert-manager                          cert-manager-cainjector         1/1     1            1           34m
cert-manager                          cert-manager-webhook            1/1     1            1           34m
csi-addons-system                     csi-addons-controller-manager   1/1     1            1           34m
kube-system                           coredns                         1/1     1            1           37m
kube-system                           snapshot-controller             2/2     2            2           36m
minio                                 minio                           1/1     1            1           29m
olm                                   catalog-operator                1/1     1            1           31m
olm                                   olm-operator                    1/1     1            1           31m
olm                                   packageserver                   2/2     2            2           30m
open-cluster-management-agent-addon   application-manager             1/1     1            1           29m
open-cluster-management-agent-addon   config-policy-controller        1/1     1            1           29m
open-cluster-management-agent-addon   governance-policy-framework     1/1     1            1           29m
open-cluster-management-agent-addon   klusterlet-addon-workmgr        1/1     1            1           29m
open-cluster-management-agent         klusterlet-registration-agent   1/1     1            1           31m
open-cluster-management-agent         klusterlet-work-agent           1/1     1            1           31m
open-cluster-management               klusterlet                      1/1     1            1           34m
rook-ceph                             csi-cephfsplugin-provisioner    1/1     1            1           23m
rook-ceph                             csi-rbdplugin-provisioner       1/1     1            1           23m
rook-ceph                             rook-ceph-exporter-dr1          1/1     1            1           22m
rook-ceph                             rook-ceph-mgr-a                 1/1     1            1           22m
rook-ceph                             rook-ceph-mon-a                 1/1     1            1           23m
rook-ceph                             rook-ceph-operator              1/1     1            1           26m
rook-ceph                             rook-ceph-osd-0                 1/1     1            1           20m
rook-ceph                             rook-ceph-rbd-mirror-a          1/1     1            1           18m
rook-ceph                             rook-ceph-tools                 1/1     1            1           19m
submariner-operator                   submariner-lighthouse-agent     1/1     1            1           34m
submariner-operator                   submariner-lighthouse-coredns   2/2     2            2           34m
submariner-operator                   submariner-operator             1/1     1            1           35m
velero                                velero                          1/1     1            1           26m
volsync-system                        volsync                         1/1     1            1           18m
```

```
fedora ➜  ~ kubectl get deploy -A --context dr2
NAMESPACE                             NAME                            READY   UP-TO-DATE   AVAILABLE   AGE
cert-manager                          cert-manager                    1/1     1            1           35m
cert-manager                          cert-manager-cainjector         1/1     1            1           35m
cert-manager                          cert-manager-webhook            1/1     1            1           35m
csi-addons-system                     csi-addons-controller-manager   1/1     1            1           30m
kube-system                           coredns                         1/1     1            1           37m
kube-system                           snapshot-controller             2/2     2            2           37m
minio                                 minio                           1/1     1            1           23m
olm                                   catalog-operator                1/1     1            1           27m
olm                                   olm-operator                    1/1     1            1           27m
olm                                   packageserver                   2/2     2            2           25m
open-cluster-management-agent-addon   application-manager             1/1     1            1           31m
open-cluster-management-agent-addon   config-policy-controller        1/1     1            1           31m
open-cluster-management-agent-addon   governance-policy-framework     1/1     1            1           31m
open-cluster-management-agent-addon   klusterlet-addon-workmgr        1/1     1            1           31m
open-cluster-management-agent         klusterlet-registration-agent   1/1     1            1           33m
open-cluster-management-agent         klusterlet-work-agent           1/1     1            1           33m
open-cluster-management               klusterlet                      1/1     1            1           35m
rook-ceph                             csi-cephfsplugin-provisioner    1/1     1            1           27m
rook-ceph                             csi-rbdplugin-provisioner       1/1     1            1           28m
rook-ceph                             rook-ceph-exporter-dr2          1/1     1            1           27m
rook-ceph                             rook-ceph-mgr-a                 1/1     1            1           26m
rook-ceph                             rook-ceph-mon-a                 1/1     1            1           28m
rook-ceph                             rook-ceph-operator              1/1     1            1           32m
rook-ceph                             rook-ceph-osd-0                 1/1     1            1           24m
rook-ceph                             rook-ceph-rbd-mirror-a          1/1     1            1           19m
rook-ceph                             rook-ceph-tools                 1/1     1            1           22m
submariner-operator                   submariner-lighthouse-agent     1/1     1            1           34m
submariner-operator                   submariner-lighthouse-coredns   2/2     2            2           34m
submariner-operator                   submariner-operator             1/1     1            1           35m
velero                                velero                          1/1     1            1           22m
volsync-system                        volsync                         1/1     1            1           19m
```

## ramenctl deploy test/envs/regional-dr.yaml
```
(ramen) fedora ➜  ramen git:(timeout) ramenctl deploy test/envs/regional-dr.yaml
2024-03-18 22:54:47,767 INFO    [ramenctl] Starting deploy
2024-03-18 22:54:47,775 INFO    [ramenctl] Saving image 'quay.io/ramendr/ramen-operator:latest'
2024-03-18 22:54:50,226 INFO    [ramenctl] Loading image in cluster 'dr1'
2024-03-18 22:54:50,226 INFO    [ramenctl] Loading image in cluster 'dr2'
2024-03-18 22:54:50,227 INFO    [ramenctl] Loading image in cluster 'hub'
2024-03-18 22:55:24,105 INFO    [ramenctl] Deploying ramen operator in cluster 'hub'
2024-03-18 22:55:34,249 INFO    [ramenctl] Deploying ramen operator in cluster 'dr1'
2024-03-18 22:55:39,592 INFO    [ramenctl] Deploying ramen operator in cluster 'dr2'
2024-03-18 22:55:48,129 INFO    [ramenctl] Finished deploy in 60.36 seconds
```

- HUB
```
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    deployment.kubernetes.io/revision: "1"
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"apps/v1","kind":"Deployment","metadata":{"annotations":{},"labels":{"app":"ramen-hub","control-plane":"controller-manager"},"name":"ramen-hub-operator","namespace":"ramen-system"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"ramen-hub","control-plane":"controller-manager"}},"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/default-container":"manager"},"labels":{"app":"ramen-hub","control-plane":"controller-manager"}},"spec":{"containers":[{"args":["--config=/config/ramen_manager_config.yaml"],"command":["/manager"],"env":[{"name":"POD_NAMESPACE","valueFrom":{"fieldRef":{"fieldPath":"metadata.namespace"}}}],"image":"quay.io/ramendr/ramen-operator:latest","imagePullPolicy":"IfNotPresent","livenessProbe":{"httpGet":{"path":"/healthz","port":8081},"initialDelaySeconds":15,"periodSeconds":20},"name":"manager","ports":[{"containerPort":9443,"name":"webhook-server","protocol":"TCP"}],"readinessProbe":{"httpGet":{"path":"/readyz","port":8081},"initialDelaySeconds":5,"periodSeconds":10},"resources":{"limits":{"cpu":"100m","memory":"300Mi"},"requests":{"cpu":"100m","memory":"200Mi"}},"securityContext":{"allowPrivilegeEscalation":false},"volumeMounts":[{"mountPath":"/tmp/k8s-webhook-server/serving-certs","name":"cert","readOnly":true},{"mountPath":"/config","name":"ramen-manager-config-vol","readOnly":true}]},{"args":["--secure-listen-address=0.0.0.0:8443","--upstream=http://127.0.0.1:9289/","--logtostderr=true","--v=10"],"image":"gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1","name":"kube-rbac-proxy","ports":[{"containerPort":8443,"name":"https"}],"resources":{"limits":{"cpu":"500m","memory":"128Mi"},"requests":{"cpu":"5m","memory":"64Mi"}}}],"securityContext":{"runAsNonRoot":true},"serviceAccountName":"ramen-hub-operator","terminationGracePeriodSeconds":10,"volumes":[{"name":"cert","secret":{"defaultMode":420,"secretName":"webhook-server-cert"}},{"configMap":{"name":"ramen-hub-operator-config"},"name":"ramen-manager-config-vol"}]}}}}
  creationTimestamp: "2024-03-19T02:55:33Z"
  generation: 1
  labels:
    app: ramen-hub
    control-plane: controller-manager
  name: ramen-hub-operator
  namespace: ramen-system
  resourceVersion: "9254"
  uid: 2d7d4cc0-5545-40b3-8ff5-3436c990b30f
spec:
  progressDeadlineSeconds: 600
  replicas: 1
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: ramen-hub
      control-plane: controller-manager
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      annotations:
        kubectl.kubernetes.io/default-container: manager
      creationTimestamp: null
      labels:
        app: ramen-hub
        control-plane: controller-manager
    spec:
      containers:
      - args:
        - --config=/config/ramen_manager_config.yaml
        command:
        - /manager
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.namespace
        image: quay.io/ramendr/ramen-operator:latest
        imagePullPolicy: IfNotPresent
        livenessProbe:
          failureThreshold: 3
          httpGet:
            path: /healthz
            port: 8081
            scheme: HTTP
          initialDelaySeconds: 15
          periodSeconds: 20
          successThreshold: 1
          timeoutSeconds: 1
        name: manager
        ports:
        - containerPort: 9443
          name: webhook-server
          protocol: TCP
        readinessProbe:
          failureThreshold: 3
          httpGet:
            path: /readyz
            port: 8081
            scheme: HTTP
          initialDelaySeconds: 5
          periodSeconds: 10
          successThreshold: 1
          timeoutSeconds: 1
        resources:
          limits:
            cpu: 100m
            memory: 300Mi
          requests:
            cpu: 100m
            memory: 200Mi
        securityContext:
          allowPrivilegeEscalation: false
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /tmp/k8s-webhook-server/serving-certs
          name: cert
          readOnly: true
        - mountPath: /config
          name: ramen-manager-config-vol
          readOnly: true
      - args:
        - --secure-listen-address=0.0.0.0:8443
        - --upstream=http://127.0.0.1:9289/
        - --logtostderr=true
        - --v=10
        image: gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1
        imagePullPolicy: IfNotPresent
        name: kube-rbac-proxy
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        resources:
          limits:
            cpu: 500m
            memory: 128Mi
          requests:
            cpu: 5m
            memory: 64Mi
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext:
        runAsNonRoot: true
      serviceAccount: ramen-hub-operator
      serviceAccountName: ramen-hub-operator
      terminationGracePeriodSeconds: 10
      volumes:
      - name: cert
        secret:
          defaultMode: 420
          secretName: webhook-server-cert
      - configMap:
          defaultMode: 420
          name: ramen-hub-operator-config
        name: ramen-manager-config-vol
status:
  availableReplicas: 1
  conditions:
  - lastTransitionTime: "2024-03-19T02:56:04Z"
    lastUpdateTime: "2024-03-19T02:56:04Z"
    message: Deployment has minimum availability.
    reason: MinimumReplicasAvailable
    status: "True"
    type: Available
  - lastTransitionTime: "2024-03-19T02:55:33Z"
    lastUpdateTime: "2024-03-19T02:56:04Z"
    message: ReplicaSet "ramen-hub-operator-86678759df" has successfully progressed.
    reason: NewReplicaSetAvailable
    status: "True"
    type: Progressing
  observedGeneration: 1
  readyReplicas: 1
  replicas: 1
  updatedReplicas: 1
```

```
apiVersion: v1
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
    volSync:
      destinationCopyMethod: Direct
kind: ConfigMap
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","data":{"ramen_manager_config.yaml":"apiVersion: ramendr.openshift.io/v1alpha1\nkind: RamenConfig\nhealth:\n  healthProbeBindAddress: :8081\nmetrics:\n  bindAddress: 127.0.0.1:9289\nwebhook:\n  port: 9443\nleaderElection:\n  leaderElect: true\n  resourceName: hub.ramendr.openshift.io\nramenControllerType: dr-hub\nmaxConcurrentReconciles: 50\nvolSync:\n  destinationCopyMethod: Direct\n"},"kind":"ConfigMap","metadata":{"annotations":{},"labels":{"cluster.open-cluster-management.io/backup":"resource"},"name":"ramen-hub-operator-config","namespace":"ramen-system"}}
  creationTimestamp: "2024-03-19T02:55:32Z"
  labels:
    cluster.open-cluster-management.io/backup: resource
  name: ramen-hub-operator-config
  namespace: ramen-system
  resourceVersion: "9106"
  uid: abbcc1ef-2a4a-4a00-9630-80c2a539f02c
```

- ManagedCluster
```
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    deployment.kubernetes.io/revision: "1"
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"apps/v1","kind":"Deployment","metadata":{"annotations":{},"labels":{"app":"ramen-dr-cluster","control-plane":"controller-manager"},"name":"ramen-dr-cluster-operator","namespace":"ramen-system"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"ramen-dr-cluster","control-plane":"controller-manager"}},"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/default-container":"manager"},"labels":{"app":"ramen-dr-cluster","control-plane":"controller-manager"}},"spec":{"containers":[{"args":["--config=/config/ramen_manager_config.yaml"],"command":["/manager"],"env":[{"name":"POD_NAMESPACE","valueFrom":{"fieldRef":{"fieldPath":"metadata.namespace"}}}],"image":"quay.io/ramendr/ramen-operator:latest","imagePullPolicy":"IfNotPresent","livenessProbe":{"httpGet":{"path":"/healthz","port":8081},"initialDelaySeconds":15,"periodSeconds":20},"name":"manager","readinessProbe":{"httpGet":{"path":"/readyz","port":8081},"initialDelaySeconds":5,"periodSeconds":10},"resources":{"limits":{"cpu":"100m","memory":"300Mi"},"requests":{"cpu":"100m","memory":"200Mi"}},"securityContext":{"allowPrivilegeEscalation":false},"volumeMounts":[{"mountPath":"/config","name":"ramen-manager-config-vol","readOnly":true}]},{"args":["--secure-listen-address=0.0.0.0:8443","--upstream=http://127.0.0.1:9289/","--logtostderr=true","--v=10"],"image":"gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1","name":"kube-rbac-proxy","ports":[{"containerPort":8443,"name":"https"}],"resources":{"limits":{"cpu":"500m","memory":"128Mi"},"requests":{"cpu":"5m","memory":"64Mi"}}}],"securityContext":{"runAsNonRoot":true},"serviceAccountName":"ramen-dr-cluster-operator","terminationGracePeriodSeconds":10,"volumes":[{"configMap":{"name":"ramen-dr-cluster-operator-config"},"name":"ramen-manager-config-vol"}]}}}}
  creationTimestamp: "2024-03-19T02:55:39Z"
  generation: 1
  labels:
    app: ramen-dr-cluster
    control-plane: controller-manager
  name: ramen-dr-cluster-operator
  namespace: ramen-system
  resourceVersion: "15691"
  uid: c9967317-4ad3-4a26-bfce-2b9b20c75b77
spec:
  progressDeadlineSeconds: 600
  replicas: 1
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: ramen-dr-cluster
      control-plane: controller-manager
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      annotations:
        kubectl.kubernetes.io/default-container: manager
      creationTimestamp: null
      labels:
        app: ramen-dr-cluster
        control-plane: controller-manager
    spec:
      containers:
      - args:
        - --config=/config/ramen_manager_config.yaml
        command:
        - /manager
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.namespace
        image: quay.io/ramendr/ramen-operator:latest
        imagePullPolicy: IfNotPresent
        livenessProbe:
          failureThreshold: 3
          httpGet:
            path: /healthz
            port: 8081
            scheme: HTTP
          initialDelaySeconds: 15
          periodSeconds: 20
          successThreshold: 1
          timeoutSeconds: 1
        name: manager
        readinessProbe:
          failureThreshold: 3
          httpGet:
            path: /readyz
            port: 8081
            scheme: HTTP
          initialDelaySeconds: 5
          periodSeconds: 10
          successThreshold: 1
          timeoutSeconds: 1
        resources:
          limits:
            cpu: 100m
            memory: 300Mi
          requests:
            cpu: 100m
            memory: 200Mi
        securityContext:
          allowPrivilegeEscalation: false
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /config
          name: ramen-manager-config-vol
          readOnly: true
      - args:
        - --secure-listen-address=0.0.0.0:8443
        - --upstream=http://127.0.0.1:9289/
        - --logtostderr=true
        - --v=10
        image: gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1
        imagePullPolicy: IfNotPresent
        name: kube-rbac-proxy
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        resources:
          limits:
            cpu: 500m
            memory: 128Mi
          requests:
            cpu: 5m
            memory: 64Mi
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext:
        runAsNonRoot: true
      serviceAccount: ramen-dr-cluster-operator
      serviceAccountName: ramen-dr-cluster-operator
      terminationGracePeriodSeconds: 10
      volumes:
      - configMap:
          defaultMode: 420
          name: ramen-dr-cluster-operator-config
        name: ramen-manager-config-vol
status:
  availableReplicas: 1
  conditions:
  - lastTransitionTime: "2024-03-19T02:56:07Z"
    lastUpdateTime: "2024-03-19T02:56:07Z"
    message: Deployment has minimum availability.
    reason: MinimumReplicasAvailable
    status: "True"
    type: Available
  - lastTransitionTime: "2024-03-19T02:55:39Z"
    lastUpdateTime: "2024-03-19T02:56:07Z"
    message: ReplicaSet "ramen-dr-cluster-operator-6755f8c564" has successfully progressed.
    reason: NewReplicaSetAvailable
    status: "True"
    type: Progressing
  observedGeneration: 1
  readyReplicas: 1
  replicas: 1
  updatedReplicas: 1
```

```
apiVersion: v1
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
      resourceName: dr-cluster.ramendr.openshift.io
    ramenControllerType: dr-cluster
    maxConcurrentReconciles: 50
    volSync:
      destinationCopyMethod: Direct
kind: ConfigMap
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","data":{"ramen_manager_config.yaml":"apiVersion: ramendr.openshift.io/v1alpha1\nkind: RamenConfig\nhealth:\n  healthProbeBindAddress: :8081\nmetrics:\n  bindAddress: 127.0.0.1:9289\nwebhook:\n  port: 9443\nleaderElection:\n  leaderElect: true\n  resourceName: dr-cluster.ramendr.openshift.io\nramenControllerType: dr-cluster\nmaxConcurrentReconciles: 50\nvolSync:\n  destinationCopyMethod: Direct\n"},"kind":"ConfigMap","metadata":{"annotations":{},"name":"ramen-dr-cluster-operator-config","namespace":"ramen-system"}}
  creationTimestamp: "2024-03-19T02:55:39Z"
  name: ramen-dr-cluster-operator-config
  namespace: ramen-system
  resourceVersion: "15543"
  uid: d6663bd0-98dd-42c0-b596-a78d3435fc2e
```

## ramenctl config test/envs/regional-dr.yaml
```
(ramen) fedora ➜  ramen git:(timeout) ✗ ramenctl config test/envs/regional-dr.yaml
2024-03-18 23:23:44,210 INFO    [ramenctl] Starting config
2024-03-18 23:23:44,699 INFO    [ramenctl] Waiting until ramen-hub-operator is rolled out
2024-03-18 23:23:44,800 INFO    [ramenctl] Creating ramen s3 secrets in cluster 'hub'
2024-03-18 23:23:47,197 INFO    [ramenctl] Updating ramen config map in cluster 'hub'
2024-03-18 23:23:47,903 INFO    [ramenctl] Creating dr-clusters for regional-dr
2024-03-18 23:23:48,511 INFO    [ramenctl] Creating dr-policy for regional-dr
2024-03-18 23:23:49,124 INFO    [ramenctl] Waiting until s3 secrets are propagated to managed clusters
2024-03-18 23:24:05,282 INFO    [ramenctl] Waiting until DRClusters report phase
2024-03-18 23:24:05,624 INFO    [ramenctl] Waiting until DRClusters phase is available
2024-03-18 23:24:06,094 INFO    [ramenctl] Waiting until DRPolicy is validated
2024-03-18 23:24:06,281 INFO    [ramenctl] Finished config in 22.07 seconds
```

- HUB
```
fedora ➜  ~ kubectl get drpolicy dr-policy --context hub -o yaml
apiVersion: ramendr.openshift.io/v1alpha1
kind: DRPolicy
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"ramendr.openshift.io/v1alpha1","kind":"DRPolicy","metadata":{"annotations":{},"name":"dr-policy"},"spec":{"drClusters":["dr1","dr2"],"replicationClassSelector":{},"schedulingInterval":"1m","volumeSnapshotClassSelector":{}}}
  creationTimestamp: "2024-03-19T03:23:49Z"
  finalizers:
  - drpolicies.ramendr.openshift.io/ramen
  generation: 1
  labels:
    cluster.open-cluster-management.io/backup: ramen
  name: dr-policy
  resourceVersion: "13841"
  uid: 203fa67e-bd1e-4335-b64f-dbd9b45ba72d
spec:
  drClusters:
  - dr1
  - dr2
  replicationClassSelector: {}
  schedulingInterval: 1m
  volumeSnapshotClassSelector: {}
status:
  conditions:
  - lastTransitionTime: "2024-03-19T03:23:54Z"
    message: drpolicy validated
    observedGeneration: 1
    reason: Succeeded
    status: "True"
    type: Validated
```

```
fedora ➜  ~ kubectl get drcluster dr1 --context hub -o yaml
apiVersion: ramendr.openshift.io/v1alpha1
kind: DRCluster
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"ramendr.openshift.io/v1alpha1","kind":"DRCluster","metadata":{"annotations":{},"name":"dr1"},"spec":{"region":"west","s3ProfileName":"minio-on-dr1"}}
  creationTimestamp: "2024-03-19T03:23:48Z"
  finalizers:
  - drclusters.ramendr.openshift.io/ramen
  generation: 1
  labels:
    cluster.open-cluster-management.io/backup: ramen
  name: dr1
  resourceVersion: "13838"
  uid: 5c0c5015-bfd5-4108-ba9b-34612eca3b76
spec:
  region: west
  s3ProfileName: minio-on-dr1
status:
  conditions:
  - lastTransitionTime: "2024-03-19T03:23:48Z"
    message: Cluster Clean
    observedGeneration: 1
    reason: Clean
    status: "False"
    type: Fenced
  - lastTransitionTime: "2024-03-19T03:23:48Z"
    message: Cluster Clean
    observedGeneration: 1
    reason: Clean
    status: "True"
    type: Clean
  - lastTransitionTime: "2024-03-19T03:23:54Z"
    message: Validated the cluster
    observedGeneration: 1
    reason: Succeeded
    status: "True"
    type: Validated
  phase: Available
```

```
fedora ➜  ~ kubectl get drcluster dr2 --context hub -o yaml
apiVersion: ramendr.openshift.io/v1alpha1
kind: DRCluster
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"ramendr.openshift.io/v1alpha1","kind":"DRCluster","metadata":{"annotations":{},"name":"dr2"},"spec":{"region":"east","s3ProfileName":"minio-on-dr2"}}
  creationTimestamp: "2024-03-19T03:23:48Z"
  finalizers:
  - drclusters.ramendr.openshift.io/ramen
  generation: 1
  labels:
    cluster.open-cluster-management.io/backup: ramen
  name: dr2
  resourceVersion: "13847"
  uid: ff6e19fd-2ccc-45ac-a833-f58d2d961ce7
spec:
  region: east
  s3ProfileName: minio-on-dr2
status:
  conditions:
  - lastTransitionTime: "2024-03-19T03:23:50Z"
    message: Cluster Clean
    observedGeneration: 1
    reason: Clean
    status: "False"
    type: Fenced
  - lastTransitionTime: "2024-03-19T03:23:50Z"
    message: Cluster Clean
    observedGeneration: 1
    reason: Clean
    status: "True"
    type: Clean
  - lastTransitionTime: "2024-03-19T03:23:55Z"
    message: Validated the cluster
    observedGeneration: 1
    reason: Succeeded
    status: "True"
    type: Validated
  phase: Available
```

```
fedora ➜  ramen git:(timeout) ✗ kubectl get configmap ramen-hub-operator-config -n ramen-system -o yaml --context hub
apiVersion: v1
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
      s3CompatibleEndpoint: http://192.168.124.202:30000
      s3Region: us-west-1
      s3SecretRef:
        name: ramen-s3-secret-dr1
        namespace: ramen-system
      veleroNamespaceSecretKeyRef:
        key: cloud
        name: cloud-credentials
    - s3ProfileName: minio-on-dr2
      s3Bucket: bucket
      s3CompatibleEndpoint: http://192.168.124.137:30000
      s3Region: us-east-1
      s3SecretRef:
        name: ramen-s3-secret-dr2
        namespace: ramen-system
      veleroNamespaceSecretKeyRef:
        key: cloud
        name: cloud-credentials
kind: ConfigMap
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","data":{"ramen_manager_config.yaml":"apiVersion: ramendr.openshift.io/v1alpha1\nkind: RamenConfig\nhealth:\n  healthProbeBindAddress: :8081\nmetrics:\n  bindAddress: 127.0.0.1:9289\nwebhook:\n  port: 9443\nleaderElection:\n  leaderElect: true\n  resourceName: hub.ramendr.openshift.io\nramenControllerType: dr-hub\nmaxConcurrentReconciles: 50\ndrClusterOperator:\n  deploymentAutomationEnabled: true\n  s3SecretDistributionEnabled: true\n  channelName: alpha\n  packageName: ramen-dr-cluster-operator\n  namespaceName: ramen-system\n  catalogSourceName: ramen-catalog\n  catalogSourceNamespaceName: ramen-system\n  clusterServiceVersionName: ramen-dr-cluster-operator.v0.0.1\nkubeObjectProtection:\n  veleroNamespaceName: velero\nvolSync:\n  disabled: false\ns3StoreProfiles:\n- s3ProfileName: minio-on-dr1\n  s3Bucket: bucket\n  s3CompatibleEndpoint: http://192.168.124.202:30000\n  s3Region: us-west-1\n  s3SecretRef:\n    name: ramen-s3-secret-dr1\n    namespace: ramen-system\n  veleroNamespaceSecretKeyRef:\n    key: cloud\n    name: cloud-credentials\n- s3ProfileName: minio-on-dr2\n  s3Bucket: bucket\n  s3CompatibleEndpoint: http://192.168.124.137:30000\n  s3Region: us-east-1\n  s3SecretRef:\n    name: ramen-s3-secret-dr2\n    namespace: ramen-system\n  veleroNamespaceSecretKeyRef:\n    key: cloud\n    name: cloud-credentials\n"},"kind":"ConfigMap","metadata":{"annotations":{},"labels":{"cluster.open-cluster-management.io/backup":"resource"},"name":"ramen-hub-operator-config","namespace":"ramen-system"}}
  creationTimestamp: "2024-03-19T02:55:32Z"
  labels:
    cluster.open-cluster-management.io/backup: resource
  name: ramen-hub-operator-config
  namespace: ramen-system
  resourceVersion: "13795"
  uid: abbcc1ef-2a4a-4a00-9630-80c2a539f02c
```

```
apiVersion: v1
data:
  AWS_ACCESS_KEY_ID: bWluaW8=
  AWS_SECRET_ACCESS_KEY: bWluaW8xMjM=
kind: Secret
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","kind":"Secret","metadata":{"annotations":{},"name":"ramen-s3-secret-dr1","namespace":"ramen-system"},"stringData":{"AWS_ACCESS_KEY_ID":"minio","AWS_SECRET_ACCESS_KEY":"minio123"}}
  creationTimestamp: "2024-03-19T03:23:46Z"
  finalizers:
  - drpolicies.ramendr.openshift.io/policy-protection
  name: ramen-s3-secret-dr1
  namespace: ramen-system
  resourceVersion: "13843"
  uid: fb657fce-aa8a-460e-b05d-649e42393088
type: Opaque
```

- dr1
```
fedora ➜  ramen git:(timeout) ✗ kubectl get configmap ramen-dr-cluster-operator-config -n ramen-system -o yaml --context dr1
apiVersion: v1
data:
  ramen_manager_config.yaml: |
    apiVersion: ramendr.openshift.io/v1alpha1
    drClusterOperator:
      catalogSourceName: ramen-catalog
      catalogSourceNamespaceName: ramen-system
      channelName: alpha
      clusterServiceVersionName: ramen-dr-cluster-operator.v0.0.1
      deploymentAutomationEnabled: true
      namespaceName: ramen-system
      packageName: ramen-dr-cluster-operator
      s3SecretDistributionEnabled: true
    health:
      healthProbeBindAddress: :8081
    kind: RamenConfig
    kubeObjectProtection:
      veleroNamespaceName: velero
    leaderElection:
      leaderElect: true
      leaseDuration: 0s
      renewDeadline: 0s
      resourceLock: ""
      resourceName: dr-cluster.ramendr.openshift.io
      resourceNamespace: ""
      retryPeriod: 0s
    maxConcurrentReconciles: 50
    metrics:
      bindAddress: 127.0.0.1:9289
    multiNamespace: {}
    ramenControllerType: dr-cluster
    s3StoreProfiles:
    - s3Bucket: bucket
      s3CompatibleEndpoint: http://192.168.124.202:30000
      s3ProfileName: minio-on-dr1
      s3Region: us-west-1
      s3SecretRef:
        name: ramen-s3-secret-dr1
        namespace: ramen-system
      veleroNamespaceSecretKeyRef:
        key: cloud
        name: cloud-credentials
    - s3Bucket: bucket
      s3CompatibleEndpoint: http://192.168.124.137:30000
      s3ProfileName: minio-on-dr2
      s3Region: us-east-1
      s3SecretRef:
        name: ramen-s3-secret-dr2
        namespace: ramen-system
      veleroNamespaceSecretKeyRef:
        key: cloud
        name: cloud-credentials
    volSync: {}
    webhook:
      port: 9443
kind: ConfigMap
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","data":{"ramen_manager_config.yaml":"apiVersion: ramendr.openshift.io/v1alpha1\nkind: RamenConfig\nhealth:\n  healthProbeBindAddress: :8081\nmetrics:\n  bindAddress: 127.0.0.1:9289\nwebhook:\n  port: 9443\nleaderElection:\n  leaderElect: true\n  resourceName: dr-cluster.ramendr.openshift.io\nramenControllerType: dr-cluster\nmaxConcurrentReconciles: 50\nvolSync:\n  destinationCopyMethod: Direct\n"},"kind":"ConfigMap","metadata":{"annotations":{},"name":"ramen-dr-cluster-operator-config","namespace":"ramen-system"}}
  creationTimestamp: "2024-03-19T02:55:39Z"
  name: ramen-dr-cluster-operator-config
  namespace: ramen-system
  ownerReferences:
  - apiVersion: work.open-cluster-management.io/v1
    kind: AppliedManifestWork
    name: 03a4c386e01a0dd011dd05abe24f8c310fdeec3778410d7c32b0ff366bcf991a-ramen-dr-cluster
    uid: 00f3d09c-51f0-40a2-864b-9dcd4f88d206
  resourceVersion: "23129"
  uid: d6663bd0-98dd-42c0-b596-a78d3435fc2e
```

- dr2
```
fedora ➜  ramen git:(timeout) ✗ kubectl get configmap ramen-dr-cluster-operator-config -n ramen-system -o yaml --context dr2
apiVersion: v1
data:
  ramen_manager_config.yaml: |
    apiVersion: ramendr.openshift.io/v1alpha1
    drClusterOperator:
      catalogSourceName: ramen-catalog
      catalogSourceNamespaceName: ramen-system
      channelName: alpha
      clusterServiceVersionName: ramen-dr-cluster-operator.v0.0.1
      deploymentAutomationEnabled: true
      namespaceName: ramen-system
      packageName: ramen-dr-cluster-operator
      s3SecretDistributionEnabled: true
    health:
      healthProbeBindAddress: :8081
    kind: RamenConfig
    kubeObjectProtection:
      veleroNamespaceName: velero
    leaderElection:
      leaderElect: true
      leaseDuration: 0s
      renewDeadline: 0s
      resourceLock: ""
      resourceName: dr-cluster.ramendr.openshift.io
      resourceNamespace: ""
      retryPeriod: 0s
    maxConcurrentReconciles: 50
    metrics:
      bindAddress: 127.0.0.1:9289
    multiNamespace: {}
    ramenControllerType: dr-cluster
    s3StoreProfiles:
    - s3Bucket: bucket
      s3CompatibleEndpoint: http://192.168.124.202:30000
      s3ProfileName: minio-on-dr1
      s3Region: us-west-1
      s3SecretRef:
        name: ramen-s3-secret-dr1
        namespace: ramen-system
      veleroNamespaceSecretKeyRef:
        key: cloud
        name: cloud-credentials
    - s3Bucket: bucket
      s3CompatibleEndpoint: http://192.168.124.137:30000
      s3ProfileName: minio-on-dr2
      s3Region: us-east-1
      s3SecretRef:
        name: ramen-s3-secret-dr2
        namespace: ramen-system
      veleroNamespaceSecretKeyRef:
        key: cloud
        name: cloud-credentials
    volSync: {}
    webhook:
      port: 9443
kind: ConfigMap
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","data":{"ramen_manager_config.yaml":"apiVersion: ramendr.openshift.io/v1alpha1\nkind: RamenConfig\nhealth:\n  healthProbeBindAddress: :8081\nmetrics:\n  bindAddress: 127.0.0.1:9289\nwebhook:\n  port: 9443\nleaderElection:\n  leaderElect: true\n  resourceName: dr-cluster.ramendr.openshift.io\nramenControllerType: dr-cluster\nmaxConcurrentReconciles: 50\nvolSync:\n  destinationCopyMethod: Direct\n"},"kind":"ConfigMap","metadata":{"annotations":{},"name":"ramen-dr-cluster-operator-config","namespace":"ramen-system"}}
  creationTimestamp: "2024-03-19T02:55:47Z"
  name: ramen-dr-cluster-operator-config
  namespace: ramen-system
  ownerReferences:
  - apiVersion: work.open-cluster-management.io/v1
    kind: AppliedManifestWork
    name: 03a4c386e01a0dd011dd05abe24f8c310fdeec3778410d7c32b0ff366bcf991a-ramen-dr-cluster
    uid: 45230322-33d7-4ac3-9717-caa477cb6486
  resourceVersion: "23015"
  uid: 453227a3-8fd8-45f3-8eaf-7f87ff1ce22b
```

## Deploy app

```
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"cluster.open-cluster-management.io/v1beta1","kind":"Placement","metadata":{"annotations":{},"labels":{"app":"deployment-cephfs"},"name":"placement","namespace":"deployment-cephfs"},"spec":{"clusterSets":["default"],"numberOfClusters":1}}
  creationTimestamp: "2024-03-19T08:58:58Z"
  generation: 1
  labels:
    app: deployment-cephfs
  name: placement
  namespace: deployment-cephfs
  resourceVersion: "67961"
  uid: d4631bcc-e10c-4260-9e07-1f20834b85bd
spec:
  clusterSets:
  - default
  numberOfClusters: 1
status:
  conditions:
  - lastTransitionTime: "2024-03-19T08:58:59Z"
    message: Placement configurations check pass
    reason: Succeedconfigured
    status: "False"
    type: PlacementMisconfigured
  - lastTransitionTime: "2024-03-19T08:58:59Z"
    message: All cluster decisions scheduled
    reason: AllDecisionsScheduled
    status: "True"
    type: PlacementSatisfied
  decisionGroups:
  - clusterCount: 1
    decisionGroupIndex: 0
    decisionGroupName: ""
    decisions:
    - placement-decision-1
  numberOfSelectedClusters: 1
```

## EnableDR

- disable placement in hub
```
(ramen) fedora ➜  ramen git:(timeout) ✗ kubectl get placement placement -n deployment-cephfs --context hub -o yaml
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
metadata:
  annotations:
    cluster.open-cluster-management.io/experimental-scheduling-disable: "true"
    drplacementcontrol.ramendr.openshift.io/drpc-name: deployment-cephfs-drpc
    drplacementcontrol.ramendr.openshift.io/drpc-namespace: deployment-cephfs
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"cluster.open-cluster-management.io/v1beta1","kind":"Placement","metadata":{"annotations":{},"labels":{"app":"deployment-cephfs"},"name":"placement","namespace":"deployment-cephfs"},"spec":{"clusterSets":["default"],"numberOfClusters":1}}
  creationTimestamp: "2024-03-19T08:58:58Z"
  finalizers:
  - drpc.ramendr.openshift.io/finalizer
  generation: 2
  labels:
    app: deployment-cephfs
  name: placement
  namespace: deployment-cephfs
  resourceVersion: "78784"
  uid: d4631bcc-e10c-4260-9e07-1f20834b85bd
spec:
  clusterSets:
  - default
  numberOfClusters: 1
  prioritizerPolicy:
    mode: Additive
  spreadPolicy: {}
status:
  conditions:
  - lastTransitionTime: "2024-03-19T08:58:59Z"
    message: Placement configurations check pass
    reason: Succeedconfigured
    status: "False"
    type: PlacementMisconfigured
  - lastTransitionTime: "2024-03-19T08:58:59Z"
    message: All cluster decisions scheduled
    reason: AllDecisionsScheduled
    status: "True"
    type: PlacementSatisfied
  decisionGroups:
  - clusterCount: 1
    decisionGroupIndex: 0
    decisionGroupName: ""
    decisions:
    - placement-decision-1
  numberOfSelectedClusters: 1
```

- create drpc
```
(ramen) fedora ➜  ramen git:(timeout) ✗ kubectl get drpc deployment-cephfs-drpc -n deployment-cephfs --context hub -o yaml
apiVersion: ramendr.openshift.io/v1alpha1
kind: DRPlacementControl
metadata:
  annotations:
    drplacementcontrol.ramendr.openshift.io/app-namespace: deployment-cephfs
    drplacementcontrol.ramendr.openshift.io/last-app-deployment-cluster: dr2
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"ramendr.openshift.io/v1alpha1","kind":"DRPlacementControl","metadata":{"annotations":{},"labels":{"app":"deployment-cephfs"},"name":"deployment-cephfs-drpc","namespace":"deployment-cephfs"},"spec":{"drPolicyRef":{"name":"dr-policy"},"placementRef":{"kind":"Placement","name":"placement"},"preferredCluster":"dr2","pvcSelector":{"matchLabels":{"appname":"busybox"}}}}
  creationTimestamp: "2024-03-19T10:08:27Z"
  finalizers:
  - drpc.ramendr.openshift.io/finalizer
  generation: 1
  labels:
    app: deployment-cephfs
    cluster.open-cluster-management.io/backup: ramen
  name: deployment-cephfs-drpc
  namespace: deployment-cephfs
  ownerReferences:
  - apiVersion: cluster.open-cluster-management.io/v1beta1
    blockOwnerDeletion: true
    controller: true
    kind: Placement
    name: placement
    uid: d4631bcc-e10c-4260-9e07-1f20834b85bd
  resourceVersion: "79019"
  uid: 57756bcf-cfd5-457d-9a3f-0473cb370fd3
spec:
  drPolicyRef:
    name: dr-policy
  placementRef:
    kind: Placement
    name: placement
  preferredCluster: dr2
  pvcSelector:
    matchLabels:
      appname: busybox
status:
  actionDuration: 40.627913492s
  actionStartTime: "2024-03-19T10:08:49Z"
  conditions:
  - lastTransitionTime: "2024-03-19T10:08:31Z"
    message: Initial deployment completed
    observedGeneration: 1
    reason: Deployed
    status: "True"
    type: Available
  - lastTransitionTime: "2024-03-19T10:08:31Z"
    message: Ready
    observedGeneration: 1
    reason: Success
    status: "True"
    type: PeerReady
  lastUpdateTime: "2024-03-19T10:09:29Z"
  phase: Deployed
  preferredDecision:
    clusterName: dr2
    clusterNamespace: dr2
  progression: Completed
  resourceConditions:
    conditions:
    - lastTransitionTime: "2024-03-19T10:08:38Z"
      message: Not all VolSync PVCs are ready
      observedGeneration: 1
      reason: Ready
      status: "False"
      type: DataReady
    - lastTransitionTime: "2024-03-19T10:08:38Z"
      message: Not all VolSync PVCs are protected
      observedGeneration: 1
      reason: DataProtected
      status: "False"
      type: DataProtected
    - lastTransitionTime: "2024-03-19T10:08:37Z"
      message: Nothing to restore
      observedGeneration: 1
      reason: Restored
      status: "True"
      type: ClusterDataReady
    - lastTransitionTime: "2024-03-19T10:08:38Z"
      message: Not all VolSync PVCs are protected
      observedGeneration: 1
      reason: DataProtected
      status: "False"
      type: ClusterDataProtected
    resourceMeta:
      generation: 1
      kind: VolumeReplicationGroup
      name: deployment-cephfs-drpc
      namespace: deployment-cephfs
      protectedpvcs:
      - busybox-pvc
```

- create vrg in both dr1 and dr2
```
apiVersion: v1
items:
- apiVersion: ramendr.openshift.io/v1alpha1
  kind: VolumeReplicationGroup
  metadata:
    annotations:
      drplacementcontrol.ramendr.openshift.io/destination-cluster: dr1
      drplacementcontrol.ramendr.openshift.io/do-not-delete-pvc: ""
      drplacementcontrol.ramendr.openshift.io/drpc-uid: 5dd7615b-3170-4390-a11f-dfc613fc86fb
    creationTimestamp: "2024-03-20T15:43:08Z"
    finalizers:
    - volumereplicationgroups.ramendr.openshift.io/vrg-protection
    generation: 1
    name: deployment-cephfs-drpc
    namespace: deployment-cephfs
    ownerReferences:
    - apiVersion: work.open-cluster-management.io/v1
      kind: AppliedManifestWork
      name: 37409a11f0590740171893179af6aa2e23f776973c410b87436c88613b3dae1f-deployment-cephfs-drpc-deployment-cephfs-vrg-mw
      uid: a85c8b78-55fa-4359-8ba4-d759720875d9
    resourceVersion: "222162"
    uid: 5ae2ce96-c643-42ef-8b02-09544c1654b9
  spec:
    async:
      replicationClassSelector: {}
      schedulingInterval: 1m
      volumeSnapshotClassSelector: {}
    pvcSelector:
      matchLabels:
        appname: busybox
    replicationState: primary
    s3Profiles:
    - minio-on-dr1
    - minio-on-dr2
    volSync: {}
  status:
    conditions:
    - lastTransitionTime: "2024-03-20T15:44:16Z"
      message: All VolSync PVCs are ready
      observedGeneration: 1
      reason: Ready
      status: "True"
      type: DataReady
    - lastTransitionTime: "2024-03-20T15:47:51Z"
      message: All VolSync PVCs are protected
      observedGeneration: 1
      reason: DataProtected
      status: "True"
      type: DataProtected
    - lastTransitionTime: "2024-03-20T15:43:09Z"
      message: Nothing to restore
      observedGeneration: 1
      reason: Restored
      status: "True"
      type: ClusterDataReady
    - lastTransitionTime: "2024-03-21T01:57:04Z"
      message: All VolSync PVCs are protected
      observedGeneration: 1
      reason: DataProtected
      status: "True"
      type: ClusterDataProtected
    kubeObjectProtection: {}
    lastGroupSyncDuration: 1m14.455434152s
    lastGroupSyncTime: "2024-03-21T02:22:04Z"
    lastUpdateTime: "2024-03-21T02:23:10Z"
    observedGeneration: 1
    protectedPVCs:
    - accessModes:
      - ReadWriteMany
      annotations:
        apps.open-cluster-management.io/hosting-subscription: deployment-cephfs/subscription
        apps.open-cluster-management.io/reconcile-option: merge
      conditions:
      - lastTransitionTime: "2024-03-20T15:44:16Z"
        message: Ready
        observedGeneration: 1
        reason: SourceInitialized
        status: "True"
        type: ReplicationSourceSetup
      labels:
        app: deployment-cephfs
        app.kubernetes.io/part-of: deployment-cephfs
        appname: busybox
      lastSyncDuration: 1m3.56056628s
      lastSyncTime: "2024-03-21T02:22:04Z"
      name: busybox-pvc
      namespace: deployment-cephfs
      protectedByVolSync: true
      replicationID:
        id: ""
      resources:
        requests:
          storage: 1Gi
      storageClassName: ocs-storagecluster-cephfs
      storageID:
        id: ""
    - accessModes:
      - ReadWriteOnce
      annotations:
        apps.open-cluster-management.io/hosting-subscription: deployment-cephfs/subscription
        apps.open-cluster-management.io/reconcile-option: merge
      conditions:
      - lastTransitionTime: "2024-03-20T15:44:16Z"
        message: Ready
        observedGeneration: 1
        reason: SourceInitialized
        status: "True"
        type: ReplicationSourceSetup
      labels:
        app: deployment-cephfs
        app.kubernetes.io/part-of: deployment-cephfs
        appname: busybox
      lastSyncDuration: 1m14.455434152s
      lastSyncTime: "2024-03-21T02:22:15Z"
      name: busybox-another-pvc
      namespace: deployment-cephfs
      protectedByVolSync: true
      replicationID:
        id: ""
      resources:
        requests:
          storage: 1Gi
      storageClassName: ocs-storagecluster-cephfs
      storageID:
        id: ""
    state: Primary
kind: List
metadata:
  resourceVersion: ""
```

```
fedora ➜  ramen git:(timeout) ✗ kubectl get vrg -n deployment-cephfs --context dr2 -o yaml
apiVersion: v1
items:
- apiVersion: ramendr.openshift.io/v1alpha1
  kind: VolumeReplicationGroup
  metadata:
    annotations:
      drplacementcontrol.ramendr.openshift.io/destination-cluster: dr2
      drplacementcontrol.ramendr.openshift.io/do-not-delete-pvc: ""
      drplacementcontrol.ramendr.openshift.io/drpc-uid: 5dd7615b-3170-4390-a11f-dfc613fc86fb
    creationTimestamp: "2024-03-20T15:43:36Z"
    finalizers:
    - volumereplicationgroups.ramendr.openshift.io/vrg-protection
    generation: 6
    name: deployment-cephfs-drpc
    namespace: deployment-cephfs
    ownerReferences:
    - apiVersion: work.open-cluster-management.io/v1
      kind: AppliedManifestWork
      name: 37409a11f0590740171893179af6aa2e23f776973c410b87436c88613b3dae1f-deployment-cephfs-drpc-deployment-cephfs-vrg-mw
      uid: e44a375d-057c-4762-9ab0-04d164227f8f
    resourceVersion: "216706"
    uid: 5be74b23-0a3f-4ac6-9553-bc477df5decf
  spec:
    async:
      replicationClassSelector: {}
      schedulingInterval: 1m
      volumeSnapshotClassSelector: {}
    pvcSelector:
      matchLabels:
        appname: busybox
    replicationState: secondary
    s3Profiles:
    - minio-on-dr1
    - minio-on-dr2
    volSync:
      rdSpec:
      - protectedPVC:
          accessModes:
          - ReadWriteMany
          annotations:
            apps.open-cluster-management.io/hosting-subscription: deployment-cephfs/subscription
            apps.open-cluster-management.io/reconcile-option: merge
          conditions:
          - lastTransitionTime: "2024-03-20T15:44:16Z"
            message: Ready
            observedGeneration: 1
            reason: SourceInitialized
            status: "True"
            type: ReplicationSourceSetup
          labels:
            app: deployment-cephfs
            app.kubernetes.io/part-of: deployment-cephfs
            appname: busybox
          lastSyncDuration: 1m10.075047613s
          lastSyncTime: "2024-03-21T01:21:10Z"
          name: busybox-pvc
          namespace: deployment-cephfs
          protectedByVolSync: true
          replicationID:
            id: ""
          resources:
            requests:
              storage: 1Gi
          storageClassName: ocs-storagecluster-cephfs
          storageID:
            id: ""
      - protectedPVC:
          accessModes:
          - ReadWriteOnce
          annotations:
            apps.open-cluster-management.io/hosting-subscription: deployment-cephfs/subscription
            apps.open-cluster-management.io/reconcile-option: merge
          conditions:
          - lastTransitionTime: "2024-03-20T15:44:16Z"
            message: Ready
            observedGeneration: 1
            reason: SourceInitialized
            status: "True"
            type: ReplicationSourceSetup
          labels:
            app: deployment-cephfs
            app.kubernetes.io/part-of: deployment-cephfs
            appname: busybox
          lastSyncDuration: 1m10.082425516s
          lastSyncTime: "2024-03-21T01:21:10Z"
          name: busybox-another-pvc
          namespace: deployment-cephfs
          protectedByVolSync: true
          replicationID:
            id: ""
          resources:
            requests:
              storage: 1Gi
          storageClassName: ocs-storagecluster-cephfs
          storageID:
            id: ""
  status:
    kubeObjectProtection: {}
    lastUpdateTime: "2024-03-21T02:22:32Z"
    observedGeneration: 6
    state: Secondary
kind: List
metadata:
  resourceVersion: ""
```

```
fedora ➜  ramen git:(timeout) ✗ kubectl get replicationsource busybox-another-pvc -n deployment-cephfs --context dr1 -o yaml
apiVersion: volsync.backube/v1alpha1
kind: ReplicationSource
metadata:
  creationTimestamp: "2024-03-20T15:44:16Z"
  generation: 1
  labels:
    volumereplicationgroups-owner: deployment-cephfs-drpc
  name: busybox-another-pvc
  namespace: deployment-cephfs
  ownerReferences:
  - apiVersion: ramendr.openshift.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: VolumeReplicationGroup
    name: deployment-cephfs-drpc
    uid: 5ae2ce96-c643-42ef-8b02-09544c1654b9
  resourceVersion: "222719"
  uid: 39ab1616-d966-4e39-b721-0eb93fd368d0
spec:
  rsyncTLS:
    accessModes:
    - ReadWriteOnce
    address: volsync-rsync-tls-dst-busybox-another-pvc.deployment-cephfs.svc.clusterset.local
    copyMethod: Snapshot
    keySecret: deployment-cephfs-drpc-vs-secret
    storageClassName: ocs-storagecluster-cephfs
    volumeSnapshotClassName: cephfs-snapclass
  sourcePVC: busybox-another-pvc
  trigger:
    schedule: '*/1 * * * *'
status:
  conditions:
  - lastTransitionTime: "2024-03-21T02:25:00Z"
    message: Synchronization in-progress
    reason: SyncInProgress
    status: "True"
    type: Synchronizing
  lastSyncDuration: 1m14.13372425s
  lastSyncStartTime: "2024-03-21T02:25:00Z"
  lastSyncTime: "2024-03-21T02:24:14Z"
  latestMoverStatus:
    logs: |-
      sent 233 bytes  received 761 bytes  397.60 bytes/sec
      total size is 84.85K  speedup is 85.36
      sent 75 bytes  received 19 bytes  62.67 bytes/sec
      total size is 84,851  speedup is 902.67
      rsync completed in 3s
    result: Successful
  nextSyncTime: "2024-03-21T02:25:00Z"
  rsyncTLS: {}
```

```
fedora ➜  ramen git:(timeout) ✗ kubectl get replicationdestination busybox-another-pvc -n deployment-cephfs --context dr2 -o yaml
apiVersion: volsync.backube/v1alpha1
kind: ReplicationDestination
metadata:
  annotations:
    ramendr.openshift.io/owner-name: deployment-cephfs-drpc
    ramendr.openshift.io/owner-namespace: deployment-cephfs
  creationTimestamp: "2024-03-20T15:44:09Z"
  generation: 1
  labels:
    volumereplicationgroups-owner: deployment-cephfs-drpc
  name: busybox-another-pvc
  namespace: deployment-cephfs
  ownerReferences:
  - apiVersion: ramendr.openshift.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: VolumeReplicationGroup
    name: deployment-cephfs-drpc
    uid: 5be74b23-0a3f-4ac6-9553-bc477df5decf
  resourceVersion: "217367"
  uid: 51d0a563-dc43-4421-b9f7-eb32c029472b
spec:
  rsyncTLS:
    accessModes:
    - ReadWriteOnce
    capacity: 1Gi
    copyMethod: Snapshot
    keySecret: deployment-cephfs-drpc-vs-secret
    serviceType: ClusterIP
    storageClassName: ocs-storagecluster-cephfs
    volumeSnapshotClassName: cephfs-snapclass
status:
  conditions:
  - lastTransitionTime: "2024-03-21T02:24:37Z"
    message: Synchronization in-progress
    reason: SyncInProgress
    status: "True"
    type: Synchronizing
  lastSyncDuration: 1m58.416572571s
  lastSyncStartTime: "2024-03-21T02:24:34Z"
  lastSyncTime: "2024-03-21T02:24:24Z"
  latestImage:
    apiGroup: snapshot.storage.k8s.io
    kind: VolumeSnapshot
    name: volsync-busybox-another-pvc-dst-20240321022418
  latestMoverStatus:
    logs: |-
      2024/03/21 02:24:02 [100] sent 766 bytes  received 241 bytes  total size 84851
      2024/03/21 02:24:03 [105] sent 24 bytes  received 96 bytes  total size 84851
      2024/03/21 02:24:03 [108] sent 40 bytes  received 4560 bytes  total size 4469
    result: Successful
  rsyncTLS:
    address: 10.105.10.187
```

```
fedora ➜  ramen git:(timeout) ✗ kubectl get persistentvolumeclaim/volsync-busybox-another-pvc-src -n deployment-cephfs --context dr1 -o yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  annotations:
    pv.kubernetes.io/bind-completed: "yes"
    pv.kubernetes.io/bound-by-controller: "yes"
    volume.beta.kubernetes.io/storage-provisioner: rook-ceph.cephfs.csi.ceph.com
    volume.kubernetes.io/storage-provisioner: rook-ceph.cephfs.csi.ceph.com
  creationTimestamp: "2024-03-21T02:27:06Z"
  finalizers:
  - kubernetes.io/pvc-protection
  labels:
    app.kubernetes.io/created-by: volsync
    volsync.backube/cleanup: 39ab1616-d966-4e39-b721-0eb93fd368d0
  name: volsync-busybox-another-pvc-src
  namespace: deployment-cephfs
  ownerReferences:
  - apiVersion: volsync.backube/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: ReplicationSource
    name: busybox-another-pvc
    uid: 39ab1616-d966-4e39-b721-0eb93fd368d0
  resourceVersion: "223606"
  uid: 79315ac0-2f44-4aef-a9a3-3c05e83a4f25
spec:
  accessModes:
  - ReadWriteOnce
  dataSource:
    apiGroup: snapshot.storage.k8s.io
    kind: VolumeSnapshot
    name: volsync-busybox-another-pvc-src
  dataSourceRef:
    apiGroup: snapshot.storage.k8s.io
    kind: VolumeSnapshot
    name: volsync-busybox-another-pvc-src
  resources:
    requests:
      storage: 1Gi
  storageClassName: ocs-storagecluster-cephfs
  volumeMode: Filesystem
  volumeName: pvc-79315ac0-2f44-4aef-a9a3-3c05e83a4f25
status:
  accessModes:
  - ReadWriteOnce
  capacity:
    storage: 1Gi
  phase: Bound
```

```
fedora ➜  ramen git:(timeout) ✗ kubectl get persistentvolumeclaim/volsync-busybox-another-pvc-dst -n deployment-cephfs --context dr2 -o yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  annotations:
    pv.kubernetes.io/bind-completed: "yes"
    pv.kubernetes.io/bound-by-controller: "yes"
    volume.beta.kubernetes.io/storage-provisioner: rook-ceph.cephfs.csi.ceph.com
    volume.kubernetes.io/storage-provisioner: rook-ceph.cephfs.csi.ceph.com
  creationTimestamp: "2024-03-20T15:44:09Z"
  finalizers:
  - kubernetes.io/pvc-protection
  labels:
    app.kubernetes.io/created-by: volsync
  name: volsync-busybox-another-pvc-dst
  namespace: deployment-cephfs
  ownerReferences:
  - apiVersion: volsync.backube/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: ReplicationDestination
    name: busybox-another-pvc
    uid: 51d0a563-dc43-4421-b9f7-eb32c029472b
  resourceVersion: "218582"
  uid: 64ead101-3fc9-439d-beab-35b3576267d8
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: ocs-storagecluster-cephfs
  volumeMode: Filesystem
  volumeName: pvc-64ead101-3fc9-439d-beab-35b3576267d8
status:
  accessModes:
  - ReadWriteOnce
  capacity:
    storage: 1Gi
  phase: Bound
```

## Failover



## Utils
watch kubectl get maintenancemode,VolumeReplicationGroup,protectedvolumereplicationgrouplists,ReplicationSource,ReplicationDestination -A --context dr1
watch kubectl get maintenancemode,VolumeReplicationGroup,protectedvolumereplicationgrouplists,ReplicationSource,ReplicationDestination -A --context dr2
watch kubectl get DRPlacementControl,DRPolicy,DRCluster -A --context hub

curl volsync-rsync-tls-dst-busybox-pvc.deployment-cephfs.svc.clusterset.local


kubectl apply -f test/addons/rook-pool/cephfs.yaml --context dr1
kubectl apply -f test/addons/rook-pool/cephfs.yaml --context dr2
kubectl apply -f test/addons/rook-pool/storage-class-fs.yaml --context dr1
kubectl apply -f test/addons/rook-pool/storage-class-fs.yaml --context dr2
kubectl apply -f test/addons/rook-pool/snapshotclass.yaml --context dr1
kubectl apply -f test/addons/rook-pool/snapshotclass.yaml --context dr2
kubectl apply -k  https://github.com/youhangwang/ocm-ramen-samples.git/channel\?ref\=cephfs --context hub

