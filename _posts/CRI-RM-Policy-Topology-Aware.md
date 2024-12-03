---
title: CRI Resource Manager Topology-Aware Policy
tags: CRI K8s
---

## 背景
在服务器级硬件上，CPU核心、I/O设备和其他外设与内存控制器、I/O总线层次结构以及CPU互连形成了一个相当复杂的网络。当这些资源的组合被分配给单个工作负载时，工作负载的性能可能会因它们之间的数据传输效率，或者换句话说，资源的对齐程度而大不相同。
<!--more-->
有许多固有的硬件架构属性，如果不适当考虑，可能会导致资源错位和工作负载性能下降。有大量的CPU核心可供运行工作负载。有大量的内存控制器，这些工作负载可以使用它们从主内存中存储和检索数据。有大量的I/O设备连接到工作负载可以访问的大量I/O总线上。CPU核心可以被划分为多个组，每个组对每个内存控制器和I/O设备的访问延迟和带宽都不同。

如果工作负载没有被分配到一组正确对齐的CPU、内存和设备上运行，它将无法达到最佳性能。鉴于硬件的特异性，为最佳工作负载性能分配一组正确对齐的资源需要识别和理解硬件中存在的多个访问延迟局部性的维度，或者换句话说，需要硬件拓扑感知。

## 概览
Topology-Aware Policy基于检测到的硬件拓扑自动构建一个 Pool 树。每个 Pool 都分配了一组CPU和内存区域作为其资源。工作负载的资源分配首先选择最适合工作负载资源需求的 Pool ，然后从此 Pool 分配CPU和内存。

从底部到顶部不同深度的 Pool 节点代表NUMA节点、die、插座，最后是根节点上的整个系统。叶NUMA节点被分配了它们控制器/区域背后的内存和对这种内存访问延迟/惩罚最小的CPU核心。如果机器有多种类型的内存分别对内核和用户空间可见，例如DRAM和PMEM，每种特殊类型的内存区域都被分配给最近的NUMA节点 Pool 。

树中的每个非叶 Pool 节点都被分配了其子节点资源的并集。因此，在实践中，die节点最终包含了相应die中的所有CPU核心和内存区域，插座节点最终包含了相应插座的die中的CPU核心和内存区域，根节点最终包含了所有插座中的所有CPU核心和内存区域。

有了这种设置，树中的每个 Pool 都有一组拓扑对齐的CPU和内存资源。可用资源的数量从下到上逐渐增加，而对齐的严格性逐渐放松。换句话说，当从下到上移动时，适应工作负载变得越来越容易，但为此付出的代价是内存访问和CPU核心之间数据传输的最大潜在成本或惩罚逐渐增加。

这种设置的另一个特性是，同一层级中兄弟 Pool 的资源集是不相交的，而沿树同一条路径的后代 Pool 的资源集部分重叠，随着 Pool 之间的距离增加，交集减少。这使得工作负载之间容易隔离。只要工作负载被分配到没有其他共同祖先的 Pool ，这些工作负载的资源应该在给定硬件上尽可能地相互隔离。

有了这种安排，该策略应该能够在没有任何特殊或额外配置的情况下处理拓扑感知的资源对齐。在分配资源时，策略：

- 过滤掉所有空闲容量不足的 Pool 
- 为剩余的 Pool 运行评分算法
- 选择得分最高的 Pool 
- 从那里为工作负载分配资源

尽管随着实现的演进，评分算法的细节可能会改变，但其基本原则大致是：

- 优先选择树中较低的 Pool ，即更严格的对齐和更低的延迟
- 优先选择空闲 Pool 而不是忙碌的 Pool ，即更多的剩余空闲容量和较少的工作负载
- 优先选择整体设备对齐更好的 Pool 

## 特性
Topology-Aware Policy具有以下特性：

- 拓扑对齐的CPU和内存分配
- 为工作负载分配最紧密可用对齐的CPU和内存
- 设备的对齐分配
- 根据已分配设备的局部性选择工作负载的 Pool 
- CPU核心的共享分配
- 将工作负载分配给 Pool CPU的共享子集
- CPU核心的独占分配
- 从共享子集中动态切分CPU核心并分配给工作负载
- CPU核心的混合分配
- 为工作负载分配独占和共享CPU核心
- 发现和使用内核隔离的CPU核心（‘isolcpus’）
- 使用内核隔离的CPU核心进行独占分配的CPU核心
- 向工作负载暴露分配的资源
- 通知工作负载资源分配的变化
- 动态放宽内存对齐以防止OOM
- 动态扩大工作负载内存集以避免 Pool /工作负载OOM
- 多级内存分配
- 将工作负载分配给它们首选类型的内存区域
- 策略了解三种类型的内存：
  - DRAM是常规的系统主内存
  - PMEM是大容量内存，如Intel® Optane™内存
  - HBM是高速内存，通常在一些特殊用途的计算系统上找到
- 冷启动
  - 将工作负载独家固定到PMEM以进行初始热身期
- 动态页面降级
  - 强制将很少使用的页面从DRAM迁移到PMEM，适用于被分配使用DRAM和PMEM两种内存类型工作负载

## 激活策略
您可以通过在cri-resmgr的配置中使用以下配置片段来激活Topology-Aware Policy：

```yaml
policy:
  Active: topology-aware
  ReservedResources:
    CPU: 750m
```

## 配置策略
策略有许多配置选项，这些选项会影响其默认行为。这些选项可以作为通过节点代理接收的动态配置的一部分提供，也可以在后备或强制配置文件中提供。这些配置选项包括：

- PinCPU：是否将工作负载固定到分配的 Pool CPU集
- PinMemory：是否将工作负载固定到分配的 Pool 内存区域
- PreferIsolatedCPUs：默认情况下是否为符合独占CPU分配条件的工作负载优先选择隔离的CPU
- PreferSharedCPUs：默认情况下是否为原本符合独占CPU分配条件的工作负载优先选择共享分配
- ReservedPoolNamespaces：将被分配到保留CPU的特殊命名空间列表（或通配符模式）
- ColocatePods：是否尝试在同一Pod或K8s命名空间内共定位容器
- ColocateNamespaces：是否尝试在同一命名空间内共定位容器

## 策略CPU分配偏好
该策略积极检查工作负载属性，以决定工作负载是否可能从额外的资源分配优化中受益。除非配置不同，符合某些相应标准的容器被视为符合这些优化的条件。这将在容器创建/资源分配请求击中策略时反映在分配的资源上。

这些额外优化的集合包括：

- 分配kube-reserved CPU
- 分配独占分配的CPU核心
- 使用内核隔离的CPU核心（用于独占分配）

该策略使用容器的QoS类别和资源需求的组合来决定是否应用这些额外的分配偏好。容器被分为五组，每组的资格标准略有不同。

- kube-system组：kube-system命名空间中的所有容器
- 低优先级组：BestEffort或Burstable QoS类别中的容器
- 子核心组：CPU请求<1 CPU的Guaranteed QoS类别容器
- 混合组：1 <= CPU请求<2的Guaranteed QoS类别容器
- 多核心组：CPU请求>=2的Guaranteed QoS类别容器

这些组的额外优化资格规则略有不同。

**kube-system组：**
- 不符合额外优化的条件
- 有资格在kube-reserved CPU核心上运行
- 始终在共享CPU核心上运行

**低优先级组（low-priority）：**
- 不符合额外优化的条件
- 始终在共享CPU核心上运行

**子核心组（sub-core）：**
- 不符合额外优化的条件
- 始终在共享CPU核心上运行

**混合组（mixed）：**
- 默认符合独占和隔离分配的条件
- 如果设置PreferSharedCPUs为true，则不符合任何一种条件
- 如果注解选择退出独占分配，则不符合任何一种条件
- 如果注解选择退出，则不符合隔离分配条件

**多核心组（multi-core）：**
- CPU请求为小数（(CPU请求 % 1000 milli-CPU) != 0）：
  - 默认不符合额外优化的条件
  - 如果注解选择加入，则符合独占和隔离分配的条件
- CPU请求不为小数：
  - 默认符合独占分配的条件
  - 默认不符合隔离分配的条件
  - 如果注解选择退出，则不符合独占分配条件
  - 如果注解选择加入，则符合隔离分配条件

对于kube-reserved CPU核心分配的资格，应始终有可能满足。如果不是这样，可能是由于配置不正确，导致ReservedResources声明不足。在这种情况下，将使用普通的共享CPU核心代替kube-reserved核心。

对于独占CPU分配的资格，应始终有可能满足。只有在有足够的隔离核心可用，能够仅用隔离核心满足容器CPU请求的独占部分时，才会考虑隔离核心分配的资格。否则，将通过从容器分配池的共享CPU核心子集中切分出独占使用的部分来分配普通CPU。

kube-system组中的容器被固定共享所有kube-reserved CPU核心。低优先级或子核心组中的容器，以及混合和多核心组中仅符合共享CPU核心分配条件的容器，都被固定在容器分配池的共享CPU核心子集上运行。这个共享子集可以并且通常会随着独占CPU核心在池中的分配和释放而动态变化。


## 容器CPU分配偏好注解
容器可以通过注解来偏离策略原本会应用给它们的默认CPU分配偏好。这些Pod注解可以以每个Pod和每个容器的粒度给出。如果任何容器同时存在这两种注解，则容器特定的注解优先。

### 共享、独占和隔离CPU偏好
容器可以使用以下Pod注解选择加入或选择退出共享CPU分配。

```yaml
metadata:
  annotations:
    # 选择加入容器C1的共享CPU核心分配
    prefer-shared-cpus.cri-resource-manager.intel.com/container.C1: "true"
    # 选择加入整个Pod的共享CPU核心分配
    prefer-shared-cpus.cri-resource-manager.intel.com/pod: "true"
    # 选择性地选择退出容器C2的共享CPU核心分配
    prefer-shared-cpus.cri-resource-manager.intel.com/container.C2: "false"
```
选择加入独占分配是通过选择退出共享分配来实现的，而选择退出独占分配是通过选择加入共享分配来实现的。

容器可以使用以下Pod注解选择加入或选择退出隔离的独占CPU核心分配。

```yaml
metadata:
  annotations:
    # 选择加入容器C1的隔离独占CPU核心分配
    prefer-isolated-cpus.cri-resource-manager.intel.com/container.C1: "true"
    # 选择加入整个Pod的隔离独占CPU核心分配
    prefer-isolated-cpus.cri-resource-manager.intel.com/pod: "true"
    # 选择性地选择退出容器C2的隔离独占CPU核心分配
    prefer-isolated-cpus.cri-resource-manager.intel.com/container.C2: "false"
```
这些Pod注解对不符合独占分配条件的容器没有影响。

### 隐式硬件拓扑提示
CRI Resource Manager在将容器交给活动策略进行资源分配之前，会自动为分配给容器的设备生成硬件拓扑提示。Topology-Aware Policy是提示感知的，通常在选择合适的 Pool 分配资源时会考虑拓扑提示。提示指示设备访问的最优硬件局部性，它们可以显著改变为容器选择的 Pool 。

由于设备拓扑提示是隐式生成的，有时可能希望策略完全忽略这些提示。例如，当容器使用本地卷但不以任何性能关键的方式使用时。

容器可以使用以下Pod注解选择退出和选择性加入提示感知 Pool 选择。

```yaml
metadata:
  annotations:
    # 仅忽略容器C1的提示
    topologyhints.cri-resource-manager.intel.com/container.C1: "false"
    # 默认忽略所有容器的提示
    topologyhints.cri-resource-manager.intel.com/pod: "false"
    # 但考虑容器C2的提示
    topologyhints.cri-resource-manager.intel.com/container.C2: "true"
```
拓扑提示生成默认全局启用。因此，使用Pod注解作为选择加入只有在整个Pod被注解为退出提示感知 Pool 选择时才有效。

### 隐式拓扑共定位的Pod和命名空间
ColocatePods或ColocateNamespaces配置选项控制策略是否尝试共定位，即在拓扑上接近地分配同一Pod或K8s命名空间内的容器。

这两个选项默认为false。将它们设置为true是为同一Pod或命名空间中的每个容器添加权重为10的亲和性的简写。

具有用户定义亲和性的容器永远不会扩展这些共定位亲和性。然而，这样的容器仍然可以对其他扩展了共定位的容器产生亲和性影响。因此，将用户定义的亲和性与隐式共定位混合需要仔细考虑和充分理解亲和性评估，或者应该完全避免。


## 容器内存请求和限制
由于cri-resmgr计算Burstable QoS类Pod的内存请求不准确，您应该使用Limit来设置Burstable Pod中容器的内存量，或者运行资源注解Webhook，以提供从Pod Spec复制的资源需求的确切副本作为额外的Pod注解。

## 保留 Pool 命名空间
用户可以标记某些命名空间以保留CPU分配。属于这些命名空间的容器将只在根据策略部分中ReservedResources配置选项配置的全局CPU保留中运行。ReservedPoolNamespaces选项是一个将被分配到保留CPU类别的命名空间通配符列表。

例如：

```yaml
policy:
  Active: topology-aware
  topology-aware:
    ReservedPoolNamespaces: ["my-pool","reserved-*"]
```
在这种设置下，所有my-pool命名空间中的工作负载以及以reserved-字符串开头的命名空间都被分配到保留CPU类别。kube-system中的工作负载会自动分配到保留CPU类别，因此不需要在此列表中提及kube-system。

## 保留CPU注解
用户可以通过注解标记某些Pod和容器以保留CPU分配。具有此类注解的容器将只在根据策略部分中ReservedResources配置选项配置的全局CPU保留中运行。

例如：

```yaml
metadata:
  annotations:
    prefer-reserved-cpus.cri-resource-manager.intel.com/pod: "true"
    prefer-reserved-cpus.cri-resource-manager.intel.com/container.special: "false"
```
