---
title: CRI Resource Manager Dynamic-Pools Policy
tags: CRI K8s
---


## 概览
Dynamic-Pools Policy 可以将工作负载放入不同的 Dynamic-Pools 中。每个 Dynamic-Pool 包含多个CPU，并且可以根据特定算法动态调整大小。

算法的主要思想是：在每个 Dynamic-Pool 中的 CPU 能够满足 Pod 的请求的前提下，根据工作负载的CPU利用率进行CPU分配。Dynamic-Pools Policy试图保持CPU利用率的平衡。

Dynamic-Pools中的CPU可以配置，例如，通过设置CPU核心和uncore的最小和最大频率。
<!--more-->

## 如何工作

1. 用户配置 Dynamic-Pools 类型，策略从这些类型实例化Dynamic-Pools。除了用户配置的Dynamic-Pools外，还有一个内置的名为共享 Pool 的Dynamic-Pools。
2. Dynamic-Pools有一组CPU和一组在这些CPU上运行的容器。
3. 每个容器被分配到一个Dynamic-Pools。Dynamic-Pools Policy允许容器使用其Pool中的所有CPU，不允许使用其他CPU。
4. 每个逻辑CPU恰好属于一个Dynamic-Pool。不能有不属于任何Dynamic-Pools的CPU。
5. Dynamic-Pools中的CPU数量可以更改。如果向Dynamic-Pools添加CPU，则Pool中的所有容器都可以使用更多的CPU。如果移除CPU，则情况相反。
6. 随着CPU被添加到或从Dynamic-Pools中移除，CPU将根据Dynamic-Pools的CPU类别属性或空闲CPU类别属性重新配置。
7. 更新Dynamic-Pools中的CPU数量：
  - Dynamic-Pools Policy需要在启动策略、创建Pod、删除Pod、更新配置以及定期间隔时更新Dynamic-Pools中的CPU数量。
  - Dynamic-Pools中的CPU数量由容器的请求和Dynamic-Pools中的CPU利用率决定。
  - 每个Dynamic-Pools中分配的CPU数量是Pool中容器请求的总和以及根据工作负载的CPU利用率分配的CPU。
8. 当在Kubernetes节点上创建新容器时，策略首先决定将运行容器的Dynamic-Pools类型。决策基于Pod的注解，如果没有给出注解，则基于命名空间。

## 参数
Dynamic-Pools Policy参数：

- PinCPU 控制将容器固定到其Dynamic-Pools的CPU。默认为true：容器不能使用其他CPU。
- PinMemory 控制将容器固定到最接近其Dynamic-PoolsCPU的内存。默认为true：只允许使用最近NUMA节点的内存。警告：这可能会导致内核因最近NUMA节点没有足够内存而因内存不足错误而杀死工作负载。在这种情况下，考虑将此选项切换为false。
- ReservedPoolNamespaces是分配给特殊保留Dynamic-Pools的命名空间列表（允许使用通配符），即将在保留CPU上运行。这总是包括kube-system命名空间。
- DynamicPoolTypes是Dynamic-Pools类型定义的列表。每种类型都可以使用以下参数进行配置：
  - Dynamic-Pools类型的名称。这用于在Pod注解中分配容器到这种类型的Dynamic-Pools。
  - Namespaces是其Pod应该被分配到这种Dynamic-Pools类型的命名空间列表（允许使用通配符），除非被Pod注解覆盖。
  - CpuClass指定根据哪个CPU类别配置Dynamic-Pools的CPU。
  - AllocatorPriority（0：高，1：正常，2：低，3：无）。CPU分配器参数，用于创建新的或调整现有Dynamic-Pools的大小。

相关配置参数：

- policy.ReservedResources.CPU指定特殊保留Dynamic-Pools中的CPU（数量）。默认情况下，kube-system命名空间中的所有容器都被分配到保留Dynamic-Pools。
- policy.AvailableResources.CPU指定策略可以使用的CPU，包括policy.ReservedResources.CPU。
- cpu.classes定义CPU类别及其参数（例如minFreq、maxFreq、uncoreMinFreq和uncoreMaxFreq）。

#### 示例
```yaml
cpu:
  classes:
    pool1-cpuclass:
      maxFreq: 1500000
      minFreq: 2000000
    pool2-cpuclass:
      maxFreq: 2000000
      minFreq: 2500000
policy:
  Active: dynamic-pools
  ReservedResources:
      CPU: cpuset:0
  dynamic-pools:
    PinCPU: true
    PinMemory: true
    DynamicPoolTypes:
      - Name: "pool1"
        Namespaces:
          - "pool1"
        CPUClass: "pool1-cpuclass"
      - Name: "pool2"
        Namespaces:
          - "pool2"
        CPUClass: "pool2-cpuclass"
```
#### 定期更新 Dynamic-Pools
Dynamic-Pools Policy可以基于每个Pool中工作负载的CPU利用率定期设置，以更新CPU分配，并使用--rebalance-interval选项设置间隔。

#### 将容器分配给Dynamic-Pools
可以在Pod注解中定义容器的Dynamic-Pools类型。以下示例中，第一个注解设置了单个容器（CONTAINER_NAME）的Dynamic-Pools类型（DPT）。最后两个注解为Pod中的所有容器设置了默认的Dynamic-Pools类型。
```yaml
dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/container.CONTAINER_NAME: DPT
dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: DPT
dynamic-pool.dynamic-pools.cri-resource-manager.intel.com: DPT
```
如果Pod没有注解，其命名空间将与Dynamic-Pools类型的命名空间匹配。使用第一个匹配的Dynamic-Pools类型。

如果命名空间不匹配，容器将被分配给共享Dynamic-Pools。

#### 指标和调试
为了从Dynamic-Pools Policy启用更详细的日志记录和指标导出，请在CRI-RM全局配置中启用仪表和策略调试：
```yaml
instrumentation:
  # Dynamic-Pools Policy导出每个Dynamic-Pools中运行的容器和Dynamic-Pools的cpusets。可在命令行访问：
  # curl --silent http://localhost:8891/metrics
  HTTPEndpoint: :8891
  PrometheusExport: true
logger:
  Debug: policy
```
使用--metrics-interval选项设置更新指标数据的间隔。
