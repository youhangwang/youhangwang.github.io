---
title: CRI Resource Manager Policy 全面介绍
tags: CRI K8s
---

## 一、概述

CRI Resource Manager（CRI-RM）是一个 Kubernetes CRI 代理，它在容器创建时拦截 CRI 请求，根据可配置的策略为工作负载做出资源分配决策。本文档全面介绍 CRI-RM 的核心组件、内置策略以及跨策略特性。

CRI-RM 的架构由以下层次组成：

- **CPU Allocator**：为策略提供 CPU 核心分配的底层组件，支持硬件拓扑感知和 CPU 优先级检测。
- **RDT（Resource Director Technology）**：提供缓存和内存带宽的分配与监控能力。
- **策略层**：内置四种策略——Topology-Aware、Dynamic-Pools、Pod-Pool、Static-Pools，每种策略面向不同的场景。
- **跨策略特性**：容器亲和性/反亲和性机制，允许用户通过注解表达容器间的放置偏好。

<!--more-->

---

## 二、核心组件

### 2.1 CPU Allocator

CRI Resource Manager 拥有一个独立的 CPU Allocator 组件，帮助策略为工作负载做出明智的 CPU 核心分配。目前，除 Static-Pools 策略外，所有策略都使用内置的 CPU Allocator。

#### 基于拓扑的分配

CPU Allocator 尝试根据硬件拓扑优化 CPU 的分配，目标是将一个请求的所有 CPU "靠近"彼此打包，以最小化 CPU 之间的内存延迟。

#### CPU 优先级

CPU Allocator 通过检测 CPU 特性及其配置参数自动进行 CPU 优先级排序。CPU 被分为三个优先级类别：**高**、**正常**和**低**。使用 CPU Allocator 的策略可能会选择为某些类型的工作负载偏好特定的优先级类别。

##### Intel Speed Select Technology (SST)

CRI Resource Manager 支持检测所有 Intel Speed Select Technology (SST) 特性：SST-PP（Performance Profile）、SST-BF（Base Frequency）、SST-TF（Turbo Frequency）和 SST-CP（Core Power）。

CPU 优先级基于检测到的当前激活的 SST 特性及其参数配置：

1. **SST-TF 已启用** → 所有 SST-TF 优先级的 CPU 被标记为高优先级。
2. **SST-CP 已启用但 SST-TF 未启用** → 与最高优先级 CLOS 关联的 CPU 标记为高优先级，最低优先级 CLOS 关联的 CPU 标记为低优先级，中间的标记为正常优先级。
3. **SST-BF 已启用且 SST-TF 和 SST-CP 处于非活动状态** → 所有 BF 高优先级核心（具有更高保证的基频）标记为高优先级。

##### Linux CPUFreq

基于 CPUFreq 的优先级排序仅在 SST 被禁用（或不支持）时生效。CRI-RM 根据两个参数将 CPU 核心分为优先级类别：

- **base frequency**：高基频（相对于系统中其他核心）→ 高优先级；低基频 → 低优先级。
- **EPP（Energy-Performance Preference）**：高 EPP 优先级 → 高优先级核心。

---

### 2.2 RDT（Resource Director Technology）

Intel® RDT 提供了缓存和内存分配与监控的功能。在 Linux 系统中，这些功能通过 `resctrl` 文件系统暴露给用户空间。在 CRI-RM 的上下文中，使用"RDT 类"（RDT Class）来管理资源控制组。

CRI-RM 支持所有可用的 RDT 技术：L2/L3 缓存分配（CAT）、代码和数据优先级（CDP）、内存带宽分配（MBA），以及缓存监控（CMT）和内存带宽监控（MBM）。

#### 类分配

CRI-RM 维护 Pod QoS 类和 RDT 类之间的直接映射。默认情况下，容器获得与其 Pod QoS 类同名的 RDT 类（Guaranteed、Burstable 或 BestEffort）。如果缺少对应的 RDT 类，容器将被分配到系统根类。

默认行为可以通过 Pod 注解覆盖：

```yaml
# Pod 级别默认值，应用于所有容器
rdtclass.cri-resource-manager.intel.com/pod: <class-name>
# 特定容器分配，优先于 Pod 级别注解
rdtclass.cri-resource-manager.intel.com/container.<container-name>: <class-name>
```

#### 操作模式

RDT 控制器支持三种操作模式，由 `rdt.options.mode` 配置：

| 模式 | 说明 |
|------|------|
| **Disabled** | RDT 控制器被禁用，所有 CRI-RM 特定的控制/监控组从 resctrl 文件系统中删除 |
| **Discovery** | 检测现有的非 CRI-RM 创建的类并以只读方式使用，删除 CRI-RM 特定的控制组 |
| **Full**（默认） | 完整操作模式，根据 CRI-RM 配置中的 RDT 类定义管理 resctrl 文件系统 |

#### RDT 类配置

RDT 类配置采用两级层次结构：**Partition（分区）** 与 **Class（类）**。

- **Partition**：代表底层 Class 的逻辑分组，每个 Partition 指定一部分可用资源（L2/L3/MB），由其下的类共享。Partition 之间保证非重叠的独占缓存分配。
- **Class**：代表容器实际被分配到的 RDT 类。同一 Partition 下的类之间缓存分配可以重叠。

#### 配置示例

以下配置将 60% 的 L3 缓存行专门分配给 Guaranteed 类，剩余 40% 由 Burstable 和 BestEffort 共享（BestEffort 仅获得其中的 50%）。Guaranteed 类获得全部内存带宽，其他类被限制为 50%：

```yaml
rdt:
  options:
    mode: Full
    monitoringDisabled: false
  l3:
    optional: true
  mb:
    optional: true
  partitions:
    exclusive:
      l3Allocation: "60%"
      mbAllocation: ["100%"]
      classes:
        Guaranteed:
          l3Allocation: "100%"
    shared:
      l3Allocation: "40%"
      mbAllocation: ["50%"]
      classes:
        Burstable:
          l3Allocation: "100%"
        BestEffort:
          l3Allocation: "50%"
```

RDT 支持**动态配置**——当通过节点代理接收到配置更新时，resctrl 文件系统会被重新配置。如果新配置与当前运行的容器不兼容（如新配置缺少某运行中容器的类），配置更新将被拒绝。

---

## 三、内置策略

### 3.1 Topology-Aware Policy（拓扑感知策略）

Topology-Aware Policy 是 CRI-RM 的旗舰策略。它基于检测到的硬件拓扑自动构建一个 **Pool 树**，为工作负载分配拓扑对齐的 CPU 和内存资源。

#### 硬件背景

在服务器级硬件上，CPU 核心、I/O 设备、内存控制器和总线层次结构形成了一个复杂的网络。有许多固有的硬件架构属性，如果不适当考虑，可能导致资源错位和工作负载性能下降：

- 大量 CPU 核心可供运行工作负载
- 大量内存控制器，用于从主内存中存取数据
- 大量 I/O 设备连接到大量 I/O 总线
- CPU 核心被划分为多个组，每个组对每个内存控制器和 I/O 设备的访问延迟和带宽都不同

如果工作负载没有被分配到一组正确对齐的 CPU、内存和设备上运行，它将无法达到最佳性能。

#### Pool 树结构

从底部到顶部不同深度的 Pool 节点代表：

```
Root (整个系统)
  ├── Socket 0
  │   ├── Die 0
  │   │   ├── NUMA Node 0  (DRAM)
  │   │   ├── NUMA Node 1  (DRAM)
  │   │   └── NUMA Node 0  (PMEM)
  │   └── Die 1
  │       └── ...
  └── Socket 1
      └── ...
```

- **叶节点**：代表 NUMA 节点，被分配了其控制器/区域背后的内存和对该内存访问延迟最小的 CPU 核心。如果机器有多种类型的内存（如 DRAM 和 PMEM），每种特殊类型的内存区域都被分配给最近的 NUMA 节点 Pool。
- **非叶节点**：被分配了其子节点资源的并集——die 节点包含对应 die 中的所有 CPU 和内存，socket 节点包含对应 socket 的所有 die 的资源，根节点包含整个系统。

**核心特性**：从下到上，可用资源逐渐增加，而对齐的严格性逐渐放松。当从下到上移动时，适应工作负载变得越来越容易，但内存访问和 CPU 核心之间数据传输的延迟逐渐增加。

同一层级中兄弟 Pool 的资源集不相交，沿树同一路径的后代 Pool 资源集部分重叠。这使工作负载之间容易隔离——只要工作负载被分配到没有其他共同祖先的 Pool，它们的资源就能尽可能相互隔离。

#### 资源分配算法

在分配资源时，策略按以下流程工作：

1. **过滤**：过滤掉所有空闲容量不足的 Pool
2. **评分**：为剩余的 Pool 运行评分算法
3. **选择**：选择得分最高的 Pool
4. **分配**：从选中的 Pool 为工作负载分配资源

评分算法的基本原则：
- 优先选择树中较低的 Pool（更严格的对齐、更低的延迟）
- 优先选择空闲 Pool 而不是忙碌的 Pool（更多剩余空闲容量和较少的工作负载）
- 优先选择整体设备对齐更好的 Pool

#### 特性列表

| 特性 | 说明 |
|------|------|
| 拓扑对齐的 CPU/内存分配 | 为工作负载分配最紧密可用对齐的 CPU 和内存 |
| 设备对齐分配 | 根据已分配设备的局部性选择工作负载的 Pool |
| CPU 共享分配 | 将工作负载分配给 Pool CPU 的共享子集 |
| CPU 独占分配 | 从共享子集中动态切分 CPU 核心并分配给工作负载 |
| CPU 混合分配 | 为工作负载分配独占和共享 CPU 核心 |
| 内核隔离 CPU（`isolcpus`） | 发现并使用内核隔离的 CPU 核心进行独占分配 |
| 资源分配暴露 | 通知工作负载资源分配的变化 |
| 动态内存放宽 | 动态扩大工作负载内存集以防止 OOM |
| 多级内存分配 | 将工作负载分配给首选类型的内存区域（DRAM / PMEM / HBM） |
| 冷启动 | 将工作负载独家固定到 PMEM 进行初始热身期 |
| 动态页面降级 | 强制将很少使用的页面从 DRAM 迁移到 PMEM |

策略了解三种类型的内存：
- **DRAM**：常规系统主内存
- **PMEM**：大容量内存，如 Intel® Optane™
- **HBM**：高速内存，通常用于特殊用途的计算系统

#### 激活和基本配置

```yaml
policy:
  Active: topology-aware
  ReservedResources:
    CPU: 750m
```

主要配置选项：

| 选项 | 说明 |
|------|------|
| `PinCPU` | 是否将工作负载固定到分配的 Pool CPU 集 |
| `PinMemory` | 是否将工作负载固定到分配的 Pool 内存区域 |
| `PreferIsolatedCPUs` | 默认是否为符合独占分配条件的工作负载优先选择隔离 CPU |
| `PreferSharedCPUs` | 默认是否为符合独占分配条件的工作负载优先选择共享分配 |
| `ReservedPoolNamespaces` | 分配到保留 CPU 的特殊命名空间列表（支持通配符） |
| `ColocatePods` | 是否尝试在同一 Pod 内共定位容器 |
| `ColocateNamespaces` | 是否尝试在同一命名空间内共定位容器 |

#### CPU 分配偏好

该策略使用容器的 QoS 类别和资源需求的组合来决定是否应用额外的分配优化。容器被分为五组：

| 组 | 条件 | 分配行为 |
|----|------|----------|
| **kube-system** | kube-system 命名空间中的所有容器 | 固定在 kube-reserved CPU 核心上，始终共享 |
| **低优先级（low-priority）** | BestEffort 或 Burstable QoS 类别 | 始终在共享 CPU 核心上运行 |
| **子核心（sub-core）** | Guaranteed QoS，CPU 请求 < 1 CPU | 始终在共享 CPU 核心上运行 |
| **混合（mixed）** | Guaranteed QoS，1 ≤ CPU 请求 < 2 | 默认符合独占和隔离分配的条件 |
| **多核心（multi-core）** | Guaranteed QoS，CPU 请求 ≥ 2 | 默认符合独占分配条件 |

#### 容器 CPU 分配偏好注解

容器可以通过 Pod 注解偏离默认的 CPU 分配偏好：

**共享/独占/隔离 CPU 偏好**：

```yaml
metadata:
  annotations:
    # 为容器 C1 选择共享 CPU 分配
    prefer-shared-cpus.cri-resource-manager.intel.com/container.C1: "true"
    # 为整个 Pod 选择共享 CPU 分配
    prefer-shared-cpus.cri-resource-manager.intel.com/pod: "true"
    # 为容器 C2 退出共享 CPU 分配（即选择独占分配）
    prefer-shared-cpus.cri-resource-manager.intel.com/container.C2: "false"
```

**隔离独占 CPU 核心偏好**：

```yaml
metadata:
  annotations:
    prefer-isolated-cpus.cri-resource-manager.intel.com/container.C1: "true"
    prefer-isolated-cpus.cri-resource-manager.intel.com/pod: "true"
```

**拓扑提示控制**：

CRI-RM 自动为分配给容器的设备生成硬件拓扑提示，Topology-Aware Policy 在选择 Pool 时会考虑这些提示。可以使用注解选择退出：

```yaml
metadata:
  annotations:
    # 仅忽略容器 C1 的拓扑提示
    topologyhints.cri-resource-manager.intel.com/container.C1: "false"
    # 默认忽略所有容器的提示
    topologyhints.cri-resource-manager.intel.com/pod: "false"
    # 但考虑容器 C2 的提示
    topologyhints.cri-resource-manager.intel.com/container.C2: "true"
```

#### 保留 Pool 命名空间

用户可以标记某些命名空间以保留 CPU 分配，这些容器将只在全局 CPU 保留中运行：

```yaml
policy:
  Active: topology-aware
  topology-aware:
    ReservedPoolNamespaces: ["my-pool", "reserved-*"]
```

注意：`kube-system` 中的工作负载会自动分配到保留 CPU 类别，无需显式列出。

#### 隐式拓扑共定位

`ColocatePods` 和 `ColocateNamespaces` 配置选项控制策略是否尝试在拓扑上接近地分配同一 Pod 或命名空间内的容器。两个选项默认为 `false`，设置为 `true` 相当于为每个容器添加权重为 10 的亲和性。

> ⚠️ 具有用户定义亲和性的容器不会扩展这些共定位亲和性，但仍可能对其他容器的共定位产生亲和性影响。混合使用需要仔细考虑。

#### 容器内存请求和限制

由于 CRI-RM 计算 Burstable QoS 类 Pod 的内存请求可能不准确，建议使用 Limit 来设置 Burstable Pod 中容器的内存量，或运行资源注解 Webhook。

---

### 3.2 Dynamic-Pools Policy（动态池策略）

Dynamic-Pools Policy 可以将工作负载放入不同的 Dynamic-Pool 中。每个 Dynamic-Pool 包含多个 CPU，并且可以根据工作负载的实际利用率**动态调整大小**。

#### 核心思想

在每个 Dynamic-Pool 中的 CPU 能够满足 Pod 请求的前提下，根据工作负载的 CPU 利用率进行 CPU 分配，目标是保持各 Pool 之间 CPU 利用率的平衡。Dynamic-Pool 中的 CPU 可以配置最小/最大频率等参数。

#### 工作原理

1. 用户配置 Dynamic-Pool 类型，策略从这些类型实例化 Dynamic-Pool。除用户配置的外，还有一个内置的**共享 Pool**。
2. 每个 Dynamic-Pool 有一组 CPU 和一组在这些 CPU 上运行的容器。
3. 每个容器被分配到一个 Dynamic-Pool，只能使用其 Pool 中的所有 CPU，不允许使用其他 CPU。
4. 每个逻辑 CPU 恰好属于一个 Dynamic-Pool——不存在不属于任何 Pool 的 CPU。
5. Dynamic-Pool 中的 CPU 数量可以动态更改。随着 CPU 被添加或移除，CPU 会根据 Pool 的 CPU 类别属性重新配置。
6. CPU 数量更新触发时机：策略启动、Pod 创建/删除、配置更新，以及定期间隔。
7. 每个 Dynamic-Pool 分配的 CPU 数量 = 容器请求总和 + 基于 CPU 利用率的分配。
8. 新容器的 Pool 分配：优先看 Pod 注解 → 没有则看命名空间 → 否则分配给共享 Pool。

#### 配置参数

```yaml
policy:
  Active: dynamic-pools
  ReservedResources:
    CPU: cpuset:0
  dynamic-pools:
    PinCPU: true          # 将容器固定到其 Dynamic-Pool 的 CPU
    PinMemory: true       # 将容器固定到最近 NUMA 节点的内存（可能导致 OOM，按需关闭）
    DynamicPoolTypes:
      - Name: "pool1"
        Namespaces:
          - "pool1"
        CPUClass: "pool1-cpuclass"
        AllocatorPriority: 0     # 0:高, 1:正常, 2:低, 3:无
      - Name: "pool2"
        Namespaces:
          - "pool2"
        CPUClass: "pool2-cpuclass"

cpu:
  classes:
    pool1-cpuclass:
      maxFreq: 1500000
      minFreq: 2000000
    pool2-cpuclass:
      maxFreq: 2000000
      minFreq: 2500000
```

主要参数说明：

| 参数 | 说明 |
|------|------|
| `PinCPU` | 控制将容器固定到其 Dynamic-Pool 的 CPU，默认 `true` |
| `PinMemory` | 控制将容器固定到最接近其 Dynamic-Pool CPU 的内存，默认 `true` |
| `ReservedPoolNamespaces` | 分配到保留 Dynamic-Pool 的命名空间列表（支持通配符） |
| `DynamicPoolTypes` | Dynamic-Pool 类型定义列表 |
| `--rebalance-interval` | 定期更新 Dynamic-Pool CPU 分配的间隔 |

#### 将容器分配给 Dynamic-Pool

通过 Pod 注解指定 Dynamic-Pool 类型：

```yaml
# 为单个容器指定
dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/container.CONTAINER_NAME: DPT
# 为 Pod 中所有容器设置默认值
dynamic-pool.dynamic-pools.cri-resource-manager.intel.com/pod: DPT
```

如果 Pod 没有注解，则匹配其命名空间与 Dynamic-Pool 类型的命名空间列表。如果命名空间也不匹配，容器将被分配给共享 Dynamic-Pool。

#### 指标和调试

```yaml
instrumentation:
  HTTPEndpoint: :8891       # curl --silent http://localhost:8891/metrics
  PrometheusExport: true
logger:
  Debug: policy
```

使用 `--metrics-interval` 选项设置更新指标数据的间隔。

---

### 3.3 Pod-Pool Policy（Pod 池策略）

Pod-Pool Policy 实现了 **Pod 级别**的工作负载放置。它将 Pod 的所有容器分配到同一个 CPU/Memory Pool 中。Pool 中的 CPU 数量可以由用户配置。

#### 配置示例

以下配置将节点上 95% 的非保留 CPU 专用于双 CPU Pool。每个 Pool 实例（`dualcpu[0]`、`dualcpu[1]` 等）包含两个专用 CPU，容量（MaxPods）为一个 Pod：

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

> ⚠️ 为与 kube-scheduler 资源核算保持一致，分配到 Pool 的 Pod 中所有容器请求的 CPU 总和必须等于 `CPU / MaxPods`（本例中为 2000m CPU）。

#### 在 Pod Pool 中运行 Pod

使用 Pod 注解指定 Pool：

```yaml
pool.podpools.cri-resource-manager.intel.com: POOLNAME
```

完整示例（`dualcpu` Pool，两个容器共请求 2000m CPU）：

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

如果 Pod 没有注解指定运行在任何特定 Pod Pool 上，并且不是 kube-system Pod，它将在**共享 CPU** 上运行。共享 CPU 包括创建用户定义 Pool 后剩余的 CPU。

---

### 3.4 Static-Pools (STP) Policy（静态池策略）

Static-Pools（STP）策略受 CMK（Kubernetes 的 CPU Manager）启发，是一个展示 CRI-RM 能力的示例策略，**并不被认为是生产就绪的**。

STP 策略旨在复制 CMK 的 `cmk isolate` 命令的功能，具有兼容性特性，可作为插入式替换以进行测试和原型设计。

#### 特性

- 可配置任意数量的 CPU 列表 Pool
- 通过节点代理动态配置更新
- CMK 兼容性特性：
  - 支持与原 CMK 相同的环境变量（除不适用的 `CMK_LOCK_TIMEOUT`、`CMK_PROC_FS` 等）
  - 支持 CMK 现有的配置目录格式
  - 解析容器命令/参数以检索 `cmk isolate` 的命令行选项
  - 支持生成 CMK 特定的节点标签和污点（默认关闭）

#### 配置

```yaml
policy:
  Active: static-pools
  static-pools:
    # LabelNode: false   # 创建 CMK 节点标签
    # TaintNode: false   # 创建 CMK 节点污点
```

#### Pool 配置来源（优先级递减）

1. **CRI-RM 全局配置**（最高优先级）
2. **独立 Static-Pools 配置文件**
3. **CMK 目录树**

```yaml
# 方式一：全局配置
policy:
  static-pools:
    pools:
      exclusive:
        exclusive: true
        cpuLists: ...
      shared:
        cpuLists: ...
      infra:
        cpuLists: ...

# 方式二：独立配置文件
policy:
  static-pools:
    ConfFilePath: "/path/to/conf.yaml"

# 方式三：CMK 目录树
policy:
  static-pools:
    ConfFileDir: "/etc/cmk"
```

#### 运行工作负载

支持以下环境变量：

| 环境变量 | 说明 |
|----------|------|
| `STP_NO_AFFINITY` | 不设置 CPU 亲和性，工作负载自行读取 `CMK_CPUS_ASSIGNED` |
| `STP_POOL` | 要运行的 Pool 名称 |
| `STP_SOCKET_ID` | 应分配核心的 Socket（`-1` 表示接受任何 Socket） |

在独占 Pool 中运行工作负载的示例：

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: stp-test
spec:
  containers:
  - name: stp-test
    image: busybox
    env:
      - name: STP_POOL
        value: "exclusive"
      - name: STP_SOCKET_ID
        value: "0"
    command:
      - "sh"
      - "-c"
      - "while :; do echo ASSIGNED: $CMK_CPUS_ASSIGNED; sleep 1; done"
    resources:
      requests:
        cmk.intel.com/exclusive-cores: "1"
      limits:
        cmk.intel.com/exclusive-cores: "1"
```

#### 向后兼容 cmk isolate

STP 策略解析容器命令/参数，自动识别并处理 `cmk isolate` 选项：

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: cmk-test
spec:
  containers:
  - name: cmk-test
    image: busybox
    command:
      - "sh"
      - "-c"
    args:
      - "/opt/bin/cmk isolate --conf-dir=/etc/cmk --pool=infra sleep 10000"
```

STP 策略将在 `infra` Pool 中运行 `sh -c "sleep 10000"`（自动剥离 `cmk isolate` 及其参数）。

---

## 四、容器亲和性与反亲和性

容器亲和性与反亲和性是跨策略的特性，允许用户通过 Pod 注解表达容器在硬件拓扑意义上的放置偏好——容器应该"靠近"还是"远离"彼此。

### 基本概念

作为一般规则：
- 运行在**同一个 NUMA 节点**的 CPU 上的容器被认为是"靠近"的
- 运行在**同一个 Socket 的不同 NUMA 节点**的 CPU 上的容器被认为是"更远"的
- 运行在**不同 Socket** 的 CPU 上的容器被认为是"远离"的

两种类型的亲和性：

| 类型 | 效果 | 注解键 |
|------|------|--------|
| **亲和性（正亲和性）** | 导致受影响的容器互相靠近 | `cri-resource-manager.intel.com/affinity` |
| **反亲和性（负亲和性）** | 导致受影响的容器互相远离 | `cri-resource-manager.intel.com/anti-affinity` |

### 亲和性语法

```yaml
metadata:
  annotations:
    cri-resource-manager.intel.com/affinity: |
      container1:
        - scope:              # 可选，定义亲和性针对哪些容器评估
            key: key-ref
            operator: op
            values:
            - value1
            ...
            - valueN
          match:              # 必需，定义亲和性适用于哪些容器
            key: key-ref
            operator: op
            values:
            - value1
            ...
            - valueN
          weight: w           # 可选，默认亲和性 +1，反亲和性 -1
```

### 亲和性语义

每条亲和性由三部分组成：

| 部分 | 说明 |
|------|------|
| **scope（作用域表达式）** | 定义亲和性针对哪些容器进行评估。省略默认为 Pod 作用域 |
| **match（匹配表达式）** | 定义亲和性适用于作用域内的哪些容器 |
| **weight（权重）** | 定义拉力（正）或推力（负）的强度。范围 [-1000, 1000] |

**有效亲和性**：容器 C₁ 和 C₂ 之间的有效亲和性 A(C₁, C₂) 是所有成对作用域匹配亲和性 W(C₁, C₂) 的权重之和。评估流程为：（1）用 scope 确定作用域内的容器；（2）对于作用域内的每个容器，如果 match 表达式为真，取其权重加到有效亲和性上。

> ⚠️ 目前（对于 Topology-Aware 策略），评估是**不对称的**：A(C₁, C₂) 和 A(C₂, C₁) 可以且通常会不同。A(C₁, C₂) 在为 C₁ 分配资源时计算，A(C₂, C₁) 在为 C₂ 分配资源时计算。

### 表达式

scope 和 match 都是表达式，由三部分组成：

| 组件 | 说明 |
|------|------|
| **key** | 从容器中提取哪些元数据进行评估 |
| **op** | 逻辑操作 |
| **values** | 用于评估键值的字符串集合 |

支持的键：

| 作用域 | 键 |
|--------|-----|
| Pod | `name`, `namespace`, `qosclass`, `labels/<label-key>`, `id`, `uid` |
| 容器 | `pod/<pod-key>`, `name`, `namespace`, `qosclass`, `labels/<label-key>`, `tags/<tag-key>`, `id` |

支持的操作符：`Equals`, `NotEqual`, `In`, `NotIn`, `Exists`, `NotExists`, `AlwaysTrue`, `Matches`, `MatchesNot`, `MatchesAny`, `MatchesNone`

### 联合键

当前表达式不支持布尔运算符（与、或、非）。可以通过**联合键**来克服此限制。联合键将多个键的值用分隔符连接成一个单一值：

- **简单格式**：`<colon-separated-subkeys>`（等价于 `:::<colon-separated-subkeys>`）
- **完整格式**：`<ksep><vsep><ksep-separated-keylist>`

示例：联合键 `:pod/qosclass:pod/name:name` 评估为 `<qosclass>:<pod name>:<container name>`。

### 简写符号

对于同一 Pod 内容器之间的亲和性，支持更简洁的写法：

```yaml
annotations:
  cri-resource-manager.intel.com/affinity: |
    container3: [ container1 ]              # container3 对 container1 有亲和性（权重 1）
  cri-resource-manager.intel.com/anti-affinity: |
    container3: [ container2 ]              # container3 对 container2 有反亲和性（权重 -1）
    container4: [ container2, container3 ]  # container4 对两者有反亲和性（权重 -1）
```

### 完整示例

将容器 `peter` 靠近 `sheep` 但远离 `wolf`：

```yaml
metadata:
  annotations:
    cri-resource-manager.intel.com/affinity: |
      peter:
      - match:
          key: name
          operator: Equals
          values:
          - sheep
        weight: 5
    cri-resource-manager.intel.com/anti-affinity: |
      peter:
      - match:
          key: name
          operator: Equals
          values:
          - wolf
        weight: 5
```

---

## 五、策略选型指南

| 策略 | 适用场景 | 成熟度 |
|------|----------|--------|
| **Topology-Aware** | 需要硬件拓扑感知自动优化、支持 DRAM/PMEM/HBM 多级内存、复杂 NUMA 环境 | 生产就绪 ⭐ |
| **Dynamic-Pools** | 需要基于 CPU 利用率动态调整 Pool 大小、按命名空间隔离工作负载、配置 CPU 频率 | 生产就绪 |
| **Pod-Pool** | 需要 Pod 级别资源隔离、固定数量 CPU 独占分配、简单的 Pool 模型 | 生产就绪 |
| **Static-Pools** | CMK 兼容性需求、测试和原型设计 | 非生产就绪 |

### 关键差异

| 维度 | Topology-Aware | Dynamic-Pools | Pod-Pool | Static-Pools |
|------|:---:|:---:|:---:|:---:|
| 自动拓扑感知 | ✅ | ❌ | ❌ | ❌ |
| 动态调整 CPU | ❌ | ✅ | ❌ | ❌ |
| Pod 级别隔离 | ✅ | ❌ | ✅ | ❌ |
| CPU 独占分配 | ✅ | ❌ | ✅ | ✅ |
| 多级内存支持 | ✅ | ❌ | ❌ | ❌ |
| RDT 集成 | ✅ | ❌ | ❌ | ❌ |
| CMK 兼容 | ❌ | ❌ | ❌ | ✅ |
| 容器亲和性/反亲和性 | ✅ | ❌ | ❌ | ❌ |
| 使用 CPU Allocator | ✅ | ✅ | ✅ | ❌ |
