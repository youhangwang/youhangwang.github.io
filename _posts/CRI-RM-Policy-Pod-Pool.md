---
title: CRI Resource Manager Pod Pool
tags: CRI K8s
---

## 概览
Pod Pool策略实现了Pod级别的工作负载放置。它将Pod的所有容器分配到同一个 CPU/Memory Pool中。 Pool中的CPU数量可以由用户配置。
<!--more-->

## 配置

需要在配置中指定活动策略，并至少定义一个Pod Pool。例如，以下配置将节点上95%的非保留CPU专用于双CPU Pool。每个 Pool实例（dualcpu[0]、dualcpu[1]等）包含两个专用CPU，并且容量（MaxPods）为一个Pod。这些CPU仅由分配到 Pool的Pod中的容器使用。剩余的CPU将用于运行不是dualcpu或kube-system的Pod。

```yaml
policy:
  Active: podpools
  ReservedResources:
    CPU: 1
  podpools:
    Pools:
      - Name: dualcpu
        CPU: 2
        MaxPods: 1
        Instances: 95%
```

请注意，上述配置为分配到 Pool的每个Pod分配了两个专用CPU。为了与kube-scheduler资源核算保持一致，这类Pod中所有容器请求的CPU必须总和等于CPU/MaxPods，即本例中的2000m CPU。


## 调试
为了启用podpools策略的更详细日志记录，请在CRI-RM全局配置中启用策略调试：

```yaml
logger:
  Debug: policy
```

## 在Pod Pool中运行Pod
如果Pod有注解：

```yaml
pool.podpools.cri-resource-manager.intel.com: POOLNAME
```

则podpools策略会将Pod分配给Pod Pool实例。

以下Pod在dualcpu Pool中运行。此示例假定dualcpu Pool每个Pod包含两个CPU，如上所述的配置示例。因此，yaml中的容器总共请求2000m CPU。

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: podpools-test
  annotations:
    pool.podpools.cri-resource-manager.intel.com: dualcpu
spec:
  containers:
  - name: testcont0
    image: busybox
    command:
      - "sh"
      - "-c"
      - "while :; do grep _allowed_list /proc/self/status; sleep 5; done"
    resources:
      requests:
        cpu: 1200m
  - name: testcont1
    image: busybox
    command:
      - "sh"
      - "-c"
      - "while :; do grep _allowed_list /proc/self/status; sleep 5; done"
    resources:
      requests:
        cpu: 800m
```

如果Pod没有注解指定运行在任何特定的Pod Pool上，并且它不是kube-system Pod，它将在共享CPU上运行。共享CPU包括创建用户定义 Pool后剩余的CPU。如果所有CPU都分配给了其他 Pool，保留的CPU也将用作共享。
