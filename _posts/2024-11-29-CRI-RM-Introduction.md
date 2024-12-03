---
title: CRI Resource Manager 介绍
tags: CRI K8s
---

## Introduction

Kubernetes 对集群中节点的硬件拓扑细节感知有限，可能会导致意外的性能损失。
- 内存控制器的层次结构
- 高/低时钟频率的 CPU 核心
- 内存的类型
  
CRI RM 是一个插件，位于 Kubernetes 和 container runtime 之间，可用于提高 *BM* Kubernetes 集群节点上容器化工作负载性能的方法。

<!--more-->

## Background

目前，Kubernetes 通过CPU manager, Topology Manager 和 Memory Manager 实现了多项优化，以提高高优先级工作负载的性能。它支持 CPU 固定和隔离，将专用 CPU 子集独占分配给单个工作负载。它还实现了 CPU、GPU 和其他外围设备的最佳对齐尝试，以尽量减少当这些设备分配给单个工作负载时的 I/O 延迟。它还可以将关键工作负载固定到单个 NUMA 节点上，以减少内存访问的延迟。

尽管 Kubernetes 中的这些优化可以为许多工作负载带来显著的改进。然而，Kubernetes 对硬件的简单化的看法既阻止了这些优化达到其最大潜力，特别是对于在专用裸机集群上运行的特殊用途、性能关键型工作负载。

Kubernetes 工作节点通常是 muti-socket 的机器，每个 socket 可能包含多个 die。每个 die 或 socket 都有众多的 CPU 核心和多个内存控制器。工作负载的放置位置，选择哪些CPU核心来运行工作负载，用于分配内存给工作负载的内存控制器，选择哪些I/O设备分配给工作负载，尤其是这些因素的组合，都会对工作负载的效率产生显著影响，包括吞吐量和延迟。对硬件有更真实的了解，以便做出更好的节点内工作负载放置决策，可以提高性能。

收集监控指标以识别工作负载的动态运行行为随时间的变化趋势，然后在检测到清晰变化后，根据当前行为以更优的方式重新排列放置，有时可以显著提高性能。

CRI Resource Manager提供了多种机制来改善节点内的资源分配。


## 使用场景

### Topology-Aware Resource Alignment

在服务器级硬件上，CPU核心、内存、I/O设备和其他外围设备与内存控制器、I/O总线以及CPU互连形成一个相当复杂的网络。当这些资源中的组合被分配给单个工作负载时，该工作负载的性能会根据数据在它们之间传输的效率，换句话说，根据资源的对齐程度而有很大变化。

如果工作负载没有被分配到一个正确对齐的CPU、内存和设备集合上运行，它将无法达到最佳性能。鉴于硬件的上述特性，为了获得最佳工作负载性能，需要硬件拓扑意识。

### Noisy Neighbors

Cloud 是多租户的环境，单个架构需要托管多个客户的应用程序和数据。然而，在实践中，运行在同一台服务器上的工作负载会相互干扰。如果它们运行在同级超线程上，它们会使用同一个物理CPU核心。如果它们运行在不同的物理CPU核心上，它们可以共享同一个最后级高速缓存、内存控制器和I/O总线带宽。即使工作负载运行在不同的物理CPU插槽上并使用不同的高速缓存和内存通道，它们也可以共享CPU互连带宽、相同的存储设备或I/O总线。

一些工作负载试图使用的共享硬件资源与其总数和其相对重要性的比例不相称。这些工作负载被统称为“吵闹邻居”。集群中的某些工作负载至关重要，例如用于存储和为集群中其他数十或数百工作负载提供数据的数据库，或负责同时向数千或数万个客户交付数据的虚拟化网络服务。当此类关键工作负载与一个或多个“吵闹邻居”共处时，确保关键工作负载在任何时间点都能获得其应得的资源（如CPU、高速缓存、内存、I/O带宽等）以保证平稳运行非常重要。

### Latency-Critical Workloads

越来越多的网络密集型延迟敏感型工作负载已经迁移到云基础设施上运行。其中一些是常规应用程序，如内容分发网络服务器或超高性能Web服务器。硬件的进步使得在基于通用处理器的平台上实现网络功能成为可能。因此，越来越多的这些工作负载是虚拟网络功能（VNF），用于虚拟化网络服务、路由器、防火墙和负载均衡器等，这些传统上需要在专有硬件上运行。

由于云本质上是多租户环境，多个应用程序在同一硬件上共存，因此它自然容易受到应用程序相互干扰的影响。可靠和可预测的性能对NFV至关重要。因此，在多租户云上运行NFV时，需要特别注意和考虑，以保护延迟和其他性能关键型工作负载不受其他共存应用程序的干扰。

### CPU Clock Speed Throttling

与 Noisy Neighbors 类似，某些工作负载可能会在无意中通过 CPU throttling 降低相邻或共存的工作负载性能。

## Descirption

CRI资源管理器包含三个独立的 Components：
- 资源管理器（Resource Manager Daemon）：主要组件，负责工作负载管理和与kubelet及container runtime的通信。
- Resource Manager Node Agent：可选的，负责与Kubernetes控制面的通信，
- Resource Manager Webhook：可选的，负责在 pod 上添加关于其资源请求的annotation。

### CRI Resource Manager

CRI 资源管理器 (CRI-RM) 是一个可插拔的Add-on，用于控制 Kubernetes 集群中分配给容器的资源数量和类型。它是可插拔的，可以将其注入两个现有组件之间的信号路径上，而集群的其他部分不知道它的存在。CRI-RM 位于 Kubernetes kubelet 和节点上运行的container runtime（例如，containerd 或 cri-o）之间。它通过作为向运行时的非透明代理来拦截 kubelet 发出的 CRI 协议请求。CRI-RM 作为代理是非透明的，因为它通常会在转发之前修改捕获到的协议消息。

CRI-RM 会跟踪在 Kubernetes 节点上运行的所有容器的状态。每当它拦截到导致任何容器的资源分配发生变化的 CRI 请求（容器创建、删除或资源分配更新请求）时，CRI-RM 会运行其内置的之一策略算法。该策略将决定如何更新资源对容器的分配。该策略可以更改系统中的任何容器，而不仅仅是与捕获到的 CRI 请求相关联的容器。策略做出决定后，将发送任何必要的消息以调整系统中受影响的容器，并根据决定修改捕获到的请求并转发。

CRI-RM 中有几种策略，每个策略都有一组不同的目标并实现不同的硬件分配策略。随时只有一个策略可以做出决定，即由管理员在 CRI-RM 配置中标记为活动的策略。

尽管大多数策略决定都是通过简单地修改或生成 CRI 协议消息来在 CRI-RM 中强制执行的，但 CRI-RM 还支持超出 CRI 运行时可能控制范围的额外功能。对于此类额外功能，CRI-RM 包含一些专门的组件，通常称为控制器，它们在自己的控制域中执行必要的步骤以强制执行任何策略决定。

#### Available Resources and Controls

CRI RM具有多种的控制功能，用于管理工作负载资源。它支持CRI协议支持的所有控制点。此外，CRI RM还实现了多个“外部”控制器，以适应容器运行时接口不支持的技术和机制。

#### Available Policies

目前，有两个 CRI-RM 策略在缓解 noisy neighbors 和运行 latency critical 工作负载方面特别有趣

##### Topology-Aware Policy

topology-aware policy优化了节点中的工作负载放置，通过使用系统实际硬件拓扑的详细模型。其目标是以自动的方式提供可衡量的好处，同时所需的用户输入最少，并且在需要的地方仍然允许灵活的用户控制进行细粒度调整。

该策略自动检测系统的硬件拓扑，并基于此构建资源池的细粒度树，从中为容器分配资源。该树与系统的内存和CPU层次结构相对应。

拓扑感知策略的主要目标是将以这种方式在池（树中的节点）之间分布容器，以最大程度地提高性能并最小化工作负载之间的干扰。

##### Static-Pools Policy

内置静态Pool策略启发自CMK（Kubernetes的CPU Manager），是用于精细CPU绑定和隔离的解决方案。静态Pool策略围绕预定义的CPU内核pool概念展开。系统被划分为一组不重叠的CPU pool，根据工作负载规范中指定的用户输入分配CPU内核。它使用了排他和共享Pool的结构，为高优先级的工作负载提供了CPU隔离。


静态游泳池策略的目标是实现CMK的cmk(isolate)命令的功能。它具有兼容性功能，可以作为CMK的直接替代品。它支持任意数量的CPU Pool，并通过CRI资源管理器的配置管理系统实现动态配置。静态游泳池策略可以读取CMK的游泳池配置目录，也可以从CRI资源管理器的动态全集群配置中读取。


### CRI Resource Manager Webhook

由于 CRI 协议的固有限制，CRI 资源管理器无法可靠地查看工作负载的所有容器资源需求。CRI 资源管理器 webhook 是一个附加组件，配置为 Kubernetes 集群中的 Mutating Admission Webhook，负责标注所有工作负载（更具体地说是 pod），以便 CRI 资源管理器能够准确地查看它们的资源需求。

CRI 资源管理器 daemon 可以在没有 webhook 帮助的情况下运行，但某些策略决策的准确性可能会降低。运行时没有 webhook 的最严重限制是 CRI-RM 失去了对 Kubernetes 扩展资源的所有可见性。因此，任何依赖于使用特定扩展资源的策略实际上都需要 webhook 。 webhook 缺失的较不严重后果之一是，CPU 请求大于 256 个完整 CPU 的被限制为 256 个 CPU 。这几乎不是一个实际的限制。最后，没有 webhook 的一个较不严重后果是，CRI-RM 无法准确确定 Burstable 容器的内存请求

### CRI Resource Manager Node Agent

CRI 资源管理器有一个独立的节点代理守护程序，负责处理所有与 Kubernetes 控制面的通信。它在功能和安全方面将资源管理守护程序与控制面隔离。节点代理负责： 
- 通过监听特定的 ConfigMap 对象并发送配置更新给 CRI 资源管理器守护程序，处理 CRI 资源管理器的动态配置管理 
- 管理 CRI 资源管理器运行在其上的节点对象（资源容量、标签、注释、污染） 
- 处理容器资源分配的动态调整

虽然 CRI 资源管理器守护程序可以在没有配套节点代理的情况下运行，但这总是阻止使用集中式动态配置，并限制了一些策略的功能。

### Topology-Aware Resource Alignment

topology-aware Policy 会根据检测到的硬件拓扑自动构建一个pool的树。从底部到顶部的各种深度的Pool节点代表 NUMA 节点、die、插槽，最后是根节点处的整个可用硬件。

作为其资源，池节点中的 NUMA 叶子被分配了控制器的内存以及与控制器距离最小的 CPU 内核。树中每个池节点的资源为其子节点资源的并集。

通过这种设置，树中的每个Pool都有与拓扑结构对齐的CPU和内存资源。可用资源的数量以及CPU核心之间的内存访问和数据传输的惩罚，在每个Pool节点中从底部到顶部逐渐增加。此外， Pool树中相同深度的兄弟游泳池节点的资源集是相斥的，而后代Pool是重叠的，而任何节点和根之间的相同路径上的Pool具有重叠的资源。

有了这种设置，topology-aware Policy 应该在没有任何特殊或额外配置的情况下处理资源的拓扑感知对齐。在分配资源时，该策略会过滤掉所有自由容量不足的Pool，然后对剩余的池塘运行评分算法，选择评分最高的一个，并从此处分配资源给工作负载。虽然随着策略实现的发展，评分算法的细节可能会有所变化，但其基本原则如下。 
- 更喜欢树中较低的Pool。换句话说，它更喜欢更紧密的对齐和更低的延迟。 
- 更喜欢空闲的池塘而不是忙碌的池塘。换句话说，它更喜欢剩余自由容量更多和工作负载更少的池塘。

### Noisy Neighbors

topology-aware Policy 的负载放置算法试图将工作负载尽可能地放置在Pool树的叶节点附近，只有在其他更好的对齐位置不可能时，才会将它们提升到树的更高层。它还试图将分配给同一深度的池的工作负载尽可能均匀地分配到可用的池上。因此，该策略试图为工作负载提供最佳的对齐和最佳的隔离。

拓扑感知策略本身永远不会错误地分配资源。相反，如果工作负载无法放入具有最佳资源对齐的游泳池，则会逐渐放宽对齐要求，直到找到适合该工作负载的游泳池。这可能是由于集群节点的整体利用率过高，也可能是因为工作负载本身的规模太大，例如，它请求的内存或CPU内核多于任何单个游泳池叶节点的可用数量。

拓扑感知策略始终将非关键工作负载分配运行在Pool中共享的CPU内核子集上。这些CPU内核由分配运行任何关键工作负载的非隔离内核组成。分配给关键工作负载的CPU内核始终通过排他性分配与其余内核隔离。如果越来越多的工作负载无法适合叶节点，例如由于集群节点利用率的增加，对内存访问和最后一级缓存的隔离严格程度可能会逐渐恶化。然后，该策略需要开始将一些工作负载提升到树中的更高层游泳池节点，资源部分重叠其子游泳池节点的资源。

在内置默认启发式方法证明不足的情况下，可以对嘈杂邻居施加额外的隔离。CRI 资源管理器提供了使用 Intel® 资源目录技术 (Intel® RDT) 和 RDT 控制器对最后级缓存和内存带宽进行额外控制。您可以通过为 Kubernetes 的三个 QoS 类别（BestEffort、Burstable 和 Guaranteed）配置相应的 RDT 类别，来限制非关键工作负载可以访问这些共享资源的多少。例如，您可以使用以下 CRI 资源管理器配置片段，将这些缓存和内存带宽限制为 33%、66% 和 100%

### Latency-Critical Workloads

即使topology-aware policy在分配拓扑对齐资源和隔离工作负载方面表现良好，有时对于一些最关键的延迟敏感工作负载来说，这还不够，例如一些网络功能虚拟化（NFV）应用程序。在这种情况下，我们通常需要采取一些额外的措施来将这些关键工作负载的性能提升到所需水平。 
- 在内核级别为关键工作负载隔离CPU核心。 
- 如果可用，使用Intel® Speed Select Technology来提升内核隔离核心的时钟频率
- 为关键工作负载分配独占缓存

当CPU核心在内核级别被隔离时，它们不再属于内核调度程序用于在节点上运行和负载平衡进程的常规CPU池。这提供了一层额外的保护，防止其他工作负载和系统进程对其产生干扰。CRI-RM会自动检测内核隔离的核心，并将其保留给那些符合一些额外条件的保证QoS类别中的关键工作负载。条件是，Pod要么需要显式注释以选择内核隔离分配，要么需要一个完整的CPU核心，并且不能使用内核隔离选择退出进行注释。您可以使用以下CRI-RM特定的注释在Pod Spec YAML中注释一个容器以选择退出内核隔离分配：

```
# sample annotation to opt out all containers in a Pod from isolated CPU allocation
metadata:
 name: $POD_NAME
 annotations:
 prefer-isolated-cpus.cri-resource-manager.intel.com/pod: false
```

如果只有选定的容器需要在Pod中选择退出，您可以使用以下注释语法来实现

```
# sample annotation to opt out selected containers in a Pod from isolated CPU allocation
metadata:
 name: $POD_NAME
 annotations:
 prefer-isolated-cpus.cri-resource-manager.intel.com/container.$CONTAINER_NAME1: false
```
Opting in的方式相同，但您需要将注释值设置为“true”，而不是“false”。

如果您需要将大量的Pod和容器从内核隔离分配中退出，通常更容易在拓扑感知策略级别全局更改默认偏好设置。您可以通过在CRI-RM配置中包含以下片段来实现：

```
# sample configuration fragment to disable isolated allocation by default
policy:
 Active: topology-aware
 topology-aware:
 PreferIsolatedCPUs: false
```

在这种配置下，每个符合条件的容器都需要选择启用独立的CPU核心。在隔离CPU核心时，还有一些额外的事项需要记住。首先，通常不希望隔离CPU核心#0或其超线程（HT）兄弟核心。核心#0有些特殊，例如某些中断可能始终由核心#0处理。其次，最好在选择的任何核心中都隔离HT兄弟核心。

为关键工作负载分配独占缓存可以采用与嘈杂邻居相似的方式。唯一的区别在于，我们创建了两个“缓存partition”，一个用于关键工作负载，另一个用于非关键工作负载。在关键partition中，我们创建了一个专门用于关键工作负载的类别。非关键partition我们进一步细分为三个QoS类别。由于partition不重叠，这导致了关键容器专用的一部分缓存被独占使用。

```
# sample configuration fragment for restricting LLC access by non-critical workloads
rdt:
 options:
 l3:
 # Bail out with an error if cache control is not supported.
 optional: false
 partitions:
 critical:
 # We dedicate 60% of the available cache to critical workloads, exclusively
 l3Allocation: "60%"
 classes:
 # - critical workloads get all of the cache lines reserved for this partition
 Critical:
 l3schema: "100%"
 noncritical:
 # Allocate 40% of all cache lines to non-critical workloads.
 l3Allocation: "40%"
 classes:
 # - critical workloads can access the full 100% of the non-critical allocation.
 # - non-critical burstable workloads can access 75% of the non-critical allocation.
 # - best effort workloads can access 50% of the non-critical allocation.
 Guaranteed:
 l3schema: "100%"
 Burstable:
 l3schema: "75%"
 BestEffort:
 l3schema: "50%"
```

此外，我们需要对关键工作负载进行注释，以使用正确的RDT类别，而不是在非关键partition中使用默认的“Guaranteed”类别。可以通过以下注释来完成，适用于Pod中的所有容器：

如果您只需要将Pod中的特定容器分配给关键RDT类别，可以使用以下代码片段实现：

```
# sample annotation to assign selected containers to the critical RDT class
metadata:
 name: $POD_NAME
 annotations:
 rdt-class.cri-resource-manager.intel.com/container.$CONTAINER_NAME1: critical
```

拓扑感知策略还支持混合分配。如果CPU核心的分配既包括独占分配的核心，又包括池中共享的CPU核心子集，则称为混合分配。独占核心可以是内核隔离的核心或普通核心，具体取决于可用性。这种类型的分配有时在单个工作负载（或更准确地说是工作负载中的单个容器）包含关键和非关键进程或线程时非常有用。例如，一个将数据以线速率传输到网络的延迟关键进程，以及另一个运行与数据泵相关的控制协议的进程。默认情况下，拓扑感知策略将任何对具有1到2个CPU核心需求的保证容器的请求视为对混合分配的请求。可以使用以下片段对工作负载进行注释以退出混合分配：

```
# sample annotation to opt all Pod containers out from mixed CPU allocation
metadata:
 name: $POD_NAME
 annotations:
 prefer-shared-cpus.cri-resource-manager.intel.com/pod: true
```

当使用混合分配时，工作负载可以在所有CPU上运行，包括共享和独占的CPU。在这种情况下，工作负载本身需要安排其进程固定到正确的独占CPU核心或共享子集。为了帮助实现这一点，CRI-RM在一个纯文本文件中以Bourne shell兼容的语法公开了分配给容器的资源集合。该文件通过bind mount方式挂载在容器的文件系统中的路径/.cri-resmgr/resources.sh下。在这个文件中，有三个与CPU分配相关的键/变量名。它们是SHARED_CPUS、EXCLUSIVE_CPUS和ISOLATED_CPUS，它们使用内核的破折号和逗号分隔的CPU集合表示相应的分配的CPU。每当容器的分配发生变化时，该文件也会更新。可以使用inotify系统调用族对该文件进行监视，以接收当该文件以及容器的资源分配发生变化时的通知事件

### Static-Pools Policy

与将系统切分为动态CPU和内存池并自动平衡工作负载的拓扑感知策略不同，静态池策略允许对CPU核心进行低级别的配置，并将工作负载分配给它们。从功能上讲，这与使用Kubernetes的CPU管理器（CMK）可以实现的效果类似。为了方便从CMK过渡，静态池策略具有向后兼容的特性，可以将其作为CMK的即插即用替代品使用。 只有在工作负载请求独占CPU时，它才会在独占池中执行。这在一定程度上有助于解决嘈杂邻居问题，前提是已知使用某些CPU的工作负载集合。 启用静态池策略并将工作负载分配给CPU池需要以下步骤：

- 启用CRI资源管理器Webhook（详见第3.4节）。
- 可选地，在每个节点上使用cmk init创建池配置。
- 配置CRI资源管理器策略。
- 在工作负载Pod规范中包含池和CPU需求。 

池配置可以通过在集群的每个节点上运行cmk init命令来创建。有关详细说明，请参阅Kubernetes*技术指南中的CPU管理- CPU固定和隔离。或者，可以在下面概述的静态池策略配置中指定池。 

下面的片段描述了如何在CRI资源管理器配置中激活静态池策略。所包含的池配置仅用于说明目的。



## Implementation Example
### Co-locating Client and Server on a Single NUMA Zone

这个例子演示了配置两个Pod，使它们被放置在同一个NUMA区域。当Pod之间频繁通信，比如在这个例子中的数据库和客户端之间，这个配置可以最小化通信距离，减少互连跳数。出于同样的原因，这个配置可以减轻系统上其他工作负载的内存流量对客户端-服务器通信的干扰。下面的Benefits部分展示了延迟改善的显著性。 首先，让我们定义一个完全正常的Redis数据库部署。下面的redis.yaml对工作负载的放置没有特殊要求。
```
apiVersion: apps/v1
kind: Deployment
metadata:
 name: redis
spec:
 replicas: 1
 selector:
 matchLabels:
 app: redis
 template:
 metadata:
 labels:
 app: redis
 spec:
 containers:
 - name: redis
 image: redis
 ports:
 - containerPort: 6379
 name: redis
 env:
 - name: MASTER
 value: 'true'
```

接下来，我们使用memtier-benchmark作为数据库的客户端。下面的memtier-benchmark.yaml文件定义了基准测试Pod对Redis服务器Pod的亲和性。亲和性告诉CRI资源管理器，一个Pod应该运行在靠近另一个Pod的位置。下面突出显示的亲和性部分将权重10分配给任何名称匹配通配符redis-*的Pod中名为redis的容器
```
apiVersion: batch/v1
kind: Job
metadata:
 name: memtier-benchmark
spec:
 template:
 metadata:
 annotations:
 cri-resource-manager.intel.com/affinity: |+
 memtier-benchmark:
 - scope:
 key: pod/name
 operator: Matches
 values:
 - redis-*
 match:
 key: name
 operator: Equals
 values:
 - redis
 weight: 10
 spec:
 containers:
 - name: memtier-benchmark
 image: redislabs/memtier_benchmark:edge
 args: ['--server=redis-service']
 restartPolicy: Never
```
CRI资源管理器也支持反亲和性。与亲和性注释相反，反亲和性定义了不应该将一个Pod与另一个Pod运行在同一个NUMA区域。CRI资源管理器在放置任何QoS类别的工作负载时都会考虑亲和性和反亲和性。在这个例子中，亲和性和反亲和性之间的性能差异大约是亲和性的吞吐量增加了50％，中位延迟降低了35％，当节点上除了Redis和基准测试之外没有其他东西时

## Benefits

CRI资源管理器带来的最大性能优势是在集群节点同时运行许多内存密集型后台工作负载时获得的。然而，即使在同一节点上没有其他工作负载运行的情况下，以及在CPU密集型工作负载的情况下，性能至少保持不变，更常见的是性能会有所提升。

在2020年12月16日至12月18日期间，我们使用memtier-benchmark性能测试对Redis内存数据库进行了基准测试。测试报告了数据库GET/SET操作的中位数、99%和99.9%的百分位延迟。延迟越低，性能越好。基准测试环境如下表所述。



# 核心技术分享: CRI-RM based CPU and NUMA Affinity | 龙蜥大讲堂28期

CMK(sunset) -> CRI-RM
 
RDT(Resource Director Technology) x MPAM: 提供了LLC（Last Level Cache）以及MB（Memory Bandwidth）内存带宽的分配和监控能力

CPU Pinning : CPU manager only for guaranteed, CRI RM for all QoS

SST(Spped Select Techonology): 不同的Core具有不同的基频和power

K8s： 所有的core 都一样

Clock domain： core
TDP domain：socket


Topology-Aware Resource Alignment：
- 过滤掉资源不足的pool
- 选大的pool
- 喜欢靠的近的，延时小的

Paper：
- 最小pool算法
- 最佳实践

CRI RM在Node level，不会跨Node。

RDT的话，OS level还需支持（需要驱动）

对K8s工作流程无任何改动

k8s CPU manager 和 cri rm的底层算法不一样。

是不是会动态调整？


# cri rm offical docs

Policies are applied by either 
- modifying a request before forwarding it or 
- by performing extra actions related to the request during its processing and proxying. 


- Install CRI Resource Manager.
- Set up kubelet to use CRI Resource Manager as the runtime.
- Set up CRI Resource Manager to use the runtime with a policy.


For providing a configuration there are two options:
- use a local configuration YAML file
  - easy
  - for dev and test
  - cri-resmgr --force-config <config-file> --runtime-socket <path>
- use the CRI Resource Manager Node Agent and a ConfigMap
  - manage policy configuration for your cluster as a single source, and
  - dynamically update that configuration
  - need to deploy node agent
  - this configuration is stored in the cache and becomes the default configuration to take into use the next time CRI Resource Manager is restarted
  - CRI资源管理器会自动尝试使用代理来获取配置
  - 每当从代理获取到新的配置并成功使用时，该配置将存储在缓存中，并成为下次重新启动CRI资源管理器时要使用的默认配置
  - the agent uses a readiness probe to propagate the status of the last configuration update back to the control plane.
  - cri-resmgr-config.node.$NODE_NAME: primary, node-specific configuration
  - cri-resmgr-config.group.$GROUP_NAME: secondary group-specific node configuration
  - cri-resmgr-config.default: secondary: secondary default node configuration
  - You can assign a node to a configuration group by setting the cri-resource-manager.intel.com/group label on the node to the name of the configuration group. You can remove a node from its group by deleting the node group label.


## Container adjustments:
  - When the agent is in use, it is also possible to adjust container resource assignments externally
  - scope:
    - the nodes and containers to which the adjustment applies
  - adjustment data:
    - updated native/compute resources (cpu/memory requests and limits)
    - updated RDT and/or Block I/O class
    - updated top tier (practically now DRAM) memory limit

CRI Resource Manager is tested with containerd and CRI-O. If any other runtime is detected during startup, cri-resmgr will refuse to start. This default behavior can be changed using the --allow-untested-runtimes command line option

## Webhook

默认情况下，CRI资源管理器无法看到Pod Spec中指定的原始容器资源需求。它尝试使用CRI容器创建请求中的相关参数来计算cpu和内存计算资源的需求。得出的估计通常对于cpu和内存限制是准确的。然而，无法使用这些参数来估计内存请求或任何扩展资源

如果您想确保CRI资源管理器使用原始Pod规范的资源需求，您需要将这些需求作为注释复制到Pod上。如果您计划使用或编写需要扩展资源的策略，这是必需的。

使用CRI资源管理器注释Webhook可以完全自动化此过程。一旦您使用提供的Dockerfile构建了Docker*镜像并发布了它，您可以按照以下步骤设置Webhook

## Architecture

CRI资源管理器（CRI-RM）是一个可插拔的附加组件，用于控制在Kubernetes集群中为容器分配多少和哪些资源。它是一个附加组件，因为您需要将其安装在正常组件选择之外。它是可插拔的，因为您将其注入到两个现有组件之间的信令路径上，而集群的其余部分对其存在毫无察觉。

CRI-RM插入到kubelet和CRI之间，kubelet是Kubernetes节点代理，CRI是容器运行时实现。CRI-RM拦截来自kubelet的CRI协议请求，充当一个非透明代理，转发给运行时。CRI-RM的代理是非透明的，因为它通常会在转发之前修改拦截的协议消息。


CRI-RM是一个跟踪运行在Kubernetes节点上的所有容器状态的工具。每当它拦截到导致任何容器资源分配发生变化的CRI请求（如容器创建、删除或资源分配更新请求），CRI-RM会运行其内置的策略算法之一。该策略会决定如何更新资源的分配，并最终根据这个决策修改拦截到的请求。该策略可以对系统中的任何容器进行更改，而不仅仅是与拦截到的CRI请求相关联的容器。因此，它不直接操作CRI请求。相反，CRI-RM的内部状态跟踪缓存提供了一个抽象，用于修改容器，策略使用这个抽象来记录其决策。

除了策略外，CRI-RM还有一些内置的资源控制器。这些控制器用于将策略决策（实际上是策略对容器所做的待定更改）付诸实施。一个特殊的内部CRI控制器用于控制通过CRI运行时可控制的所有资源。该控制器处理更新拦截到的CRI请求的实际细节，并为策略决策更新的其他现有容器生成任何额外的非请求更新请求。还有其他的离线控制器用于对当前CRI运行时无法处理的资源进行控制。

为了确定哪些容器需要交给各个控制器进行更新，CRI-RM利用内部状态跟踪缓存的能力来确定哪些容器有待执行的未强制更改，并确定这些更改属于哪个控制器的领域。CRI控制器目前处理CPU和内存资源，包括大页。控制级别涵盖每个容器的CPU集合、CFS参数化、内存限制、OOM分数调整和固定到内存控制器。现有的两个带外控制器，Intel® Resource Director Technology (Intel® RDT)和Block I/O，分别处理最后一级缓存和内存带宽分配以及Block I/O带宽的仲裁。

CRI-RM的许多操作细节是可配置的。例如，CRI-RM内部的哪个策略是活动的、为活动策略配置资源分配算法以及各种资源控制器的配置等。虽然可以使用运行CRI-RM的节点上的配置文件来配置CRI-RM，但在集群中配置所有CRI-RM实例的首选方法是使用Kubernetes ConfigMaps和CRI-RM节点代理。

### Components
1. Node Agent
 
节点代理是CRI-RM本身外部的一个组件。CRI-RM与Kubernetes控制平面的所有交互都通过节点代理进行，节点代理代表CRI-RM执行任何直接的交互操作。

节点代理使用两个gRPC接口与CRI-RM进行通信
- 配置接口用于：
  - 将更新的外部配置数据推送给CRI-RM
  - 将容器资源分配的调整推送给CRI-RM
- 集群接口实现了CRI-RM在内部暴露给policies和其他components的必要底层功能。该接口实现了以下功能：
  - 更新节点的资源容量
  - 获取、设置或删除节点上的标签
  - 获取、设置或删除节点上的注释
  - 获取、设置或删除节点上的污点

配置接口在CRI-RM中定义并且运行其gRPC服务器。代理充当此接口的gRPC客户端。底层集群接口在代理中定义并且其gRPC service 在其中运行。CRI-RM充当底层管道接口的gRPC客户端。

此外，随CRI-RM一起提供的默认节点代理实现了以下方案：
- 管理所有CRI-RM实例的配置管理
- 管理对容器资源分配的动态调整

2. Resource Manager

CRI-RM实现了一个请求处理流水线和一个事件处理流水线。请求处理流水线负责在CRI客户端和CRI运行时之间代理CRI请求和响应。事件处理流水线处理一组不与CRI请求直接相关或结果无关的其他事件。这些事件通常是在CRI-RM内部生成的。它们可以是某些容器状态的变化或共享系统资源的利用，这可能需要尝试重新平衡容器之间的资源分配，使系统更接近最佳状态。某些事件也可以由策略生成。

CRI-RM的资源管理器组件实现了这两个处理流水线的基本控制流程。它在处理请求或事件的各个阶段将控制权传递给CRI-RM的所有必要子组件。此外，它对这些请求或事件的处理进行串行化，确保在任何时间点最多只有一个（被拦截的）请求或事件正在处理。

请求处理管道的高级控制流程如下：

- 如果请求不需要策略控制，则让其绕过处理管道；将其交给日志记录，然后将其转发到服务器，并将相应的响应返回给客户端。
- 如果请求需要拦截进行策略控制，则执行以下操作：
    1. 锁定处理管道序列化锁。
    2. 查找/创建请求的缓存对象（pod/container）。
    3. 如果请求没有资源分配，则进行代理（步骤6）。
    4. 否则，调用资源分配的策略层：
      - 将其传递给配置的活动策略，该策略将
        - 为容器分配资源。
        - 更新缓存中容器的分配情况。
        - 更新缓存中受分配影响的任何其他容器。
    5. 调用控制器层进行策略后处理，该层将：
      - 收集其控制范围内具有待处理更改的控制器
      - 对于每个控制器，调用与请求对应的策略后处理函数。
      - 清除控制器的待处理标记。
    6. 代理请求：
      - 将请求转发到服务器。
      - 发送任何其他受影响容器的更新请求。
      - 根据响应情况必要时更新缓存。
      - 将响应返回给客户端。
    7. 释放处理管道序列化锁。

事件处理管道的高级控制流程根据事件类型如下：

- 对于特定于策略的事件：
  - 启用处理管道锁。
  - 调用策略事件处理程序。
  - 调用控制器层进行策略后处理（与请求的步骤5相同）。
  - 释放管道锁。

- 对于metrics event：
  - 执行收集/处理/关联。
  - 启用处理管道锁。
  - 根据需要更新缓存对象。
  - 根据需要进行请求重新平衡。
  - 释放管道锁。

- 对于重新平衡事件：
  - 启用处理管道锁。
  - 调用策略层进行重新平衡。
  - 调用控制器层进行策略后处理（与请求的步骤5相同）。
  - 释放管道锁。

3. Cache

缓存是CRI-RM内部的共享存储位置。它跟踪CRI-RM已知的Pod和容器的运行时状态，以及CRI-RM本身的状态，包括活动配置和活动策略的状态。缓存被保存到文件系统的永久存储中，并用于在重新启动时恢复CRI-RM的运行时状态。

缓存提供了查询和更新Pod和容器状态的功能。这是活动策略用于进行资源分配决策的机制。策略只需根据决策更新缓存中受影响容器的状态。

缓存关联和跟踪容器的更改与资源域，用于执行策略决策。通用控制器层首先查询哪些容器有待处理的更改，然后为每个容器调用每个控制器。控制器使用缓存提供的查询功能来判断是否需要更改其资源/控制域中的任何内容，然后相应地采取行动。

对缓存的访问需要进行串行化。然而，这种串行化不是由缓存本身提供的。相反，它假设调用者确保适当的保护措施来防止并发的读写访问。资源管理器中的请求和事件处理流水线使用锁来串行化请求和事件处理，从而访问缓存。

如果策略需要处理资源管理器未请求的处理，即处理资源管理器的内部策略后端API调用之外的处理，则应将策略事件注入到资源管理器的事件循环中。这会导致资源管理器从策略的事件处理程序中以注入的事件作为参数进行回调，并且缓存被正确锁定。


4. 通用策略层 

通用策略层定义了CRI-RM的抽象接口，用于与策略实现进行交互，并负责激活和分派调用到配置的活动策略。

5. 通用资源控制器层

通用资源控制器层定义了CRI-RM与资源控制器实现进行交互的抽象接口，并负责将调用分派给控制器实现，以进行决策的后策略执行。

6. 指标收集器 

指标收集器收集有关节点上运行的容器的一组运行时指标。可以配置CRI-RM定期评估收集到的数据，以确定当前容器资源分配的优化程度，并在可能且必要时尝试重新平衡/重新分配。


7. Policy Implementations

- None: 一个空的策略，不做任何策略决策。仅仅是为了完整性而包含在内，类似于kubelet中的none策略。
- Static Pools: 对CRI-RM进行了向后兼容的CMK重新实现，增加了一些额外的功能。
- Static: 从kubelet中的CPU Manager的静态策略中提取的一部分代码，经过了一些修改以在CRI-RM中工作。仅仅作为一个概念验证，证明当前的kubelet策略可以在CRI-RM中实现。
- Static Plus: 一个相对简单的策略，类似于kubelet中的静态策略，增加了一些额外的功能。
- Topology Aware: 一个能够处理多个层次/类型内存的拓扑感知策略，通常是配置为2层内存模式的DRAM/PMEM组合。

8. Resource Controller Implementations：
- Intel RDT: 一个资源控制器实现，负责将容器与Intel RDT类关联的实际细节。该类有效地确定容器可用的最后一级缓存和内存带宽的数量。该控制器使用Linux内核的resctrl伪文件系统进行控制。
- Block I/O: 一个资源控制器实现，负责将容器与块I/O类关联的实际细节。该类有效地确定容器可用的块I/O带宽的数量。该控制器使用Linux内核的blkio cgroup控制器和cgroupfs伪文件系统进行控制。
- CRI: 一个资源控制器，负责修改拦截的CRI容器创建请求，并根据活动策略对容器创建CRI容器资源更新请求。

