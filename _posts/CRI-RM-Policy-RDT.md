---
title: CRI Resource Manager RDT (Resource Director Technology)
tags: CRI K8s
---

## 背景

Intel® RDT 提供了缓存和内存分配与监控的功能。在Linux系统中，这些功能通过resctrl文件系统暴露给用户空间。缓存和内存分配在RDT中是通过使用 resource control groups 来处理的。资源分配是在组级别上指定的，每个任务（进程/线程）被分配到一个组中。在CRI Resource Manager的上下文中，我们使用“RDT类”而不是“资源控制组”。

CRI Resource Manager 支持所有可用的 RDT 技术，即 L2 和 L3 缓存分配（CAT）以及代码和数据优先级（CDP）和内存带宽分配（MBA），以及缓存监控（CMT）和内存带宽监控（MBM）。
<!--more-->
## 概览

CRI-RM 中的 RDT 配置是基于类的。每个容器被分配到一个 RDT 类。反过来，容器中的所有进程都被分配到对应的 RDT class of service（CLOS）（位于 /sys/fs/resctrl）中，与 RDT 类相对应。CRI-RM 将在启动时或配置更改时配置 CLOS。

CRI-RM 维护 Pod QoS 类和 RDT 类之间的直接映射。如果启用了 RDT，CRI-RM 尝试将容器分配到与其 Pod QoS 类名称匹配的 RDT 类。这种默认行为可以通过 Pod 注解来覆盖。

## 类分配
默认情况下，容器获得与其 Pod QoS 类同名的 RDT 类（Guaranteed、Burstable 或 BestEffort）。如果缺少 RDT 类，容器将被分配到系统根类。

默认行为可以通过 Pod 注解来覆盖：

- rdtclass.cri-resource-manager.intel.com/pod: <class-name>  # 指定用于所有容器的 Pod 级别默认值
- rdtclass.cri-resource-manager.intel.com/container.<container-name>: <class-name>  # 指定特定容器的分配，优先于可能的 Pod 级别注解

通过 Pod 注解，可以指定 Guaranteed、Burstable 或 BestEffort 之外的 RDT 类。

默认分配也可以被策略覆盖，但目前没有任何内置策略这样做。

## 配置
### 操作模式

RDT 控制器支持三种操作模式，由 `rdt.options.mode`` 配置选项控制。

- Disabled：RDT 控制器实际上被禁用，容器不会被分配，也不会创建monitoring groups。激活此模式时，将从 resctrl 文件系统中删除所有 CRI-RM 特定的控制和监控组。
- Discovery：RDT 控制器检测现有的非 CRI-RM 制定的类，并使用这些类。这些发现的类的配置被视为只读，不会被更改。激活此模式时，将从 resctrl 文件系统中删除所有 CRI-RM 特定的控制组。
- Full：完整操作模式。控制器根据 CRI-RM 配置中的 RDT 类定义管理 resctrl 文件系统的配置。这是默认操作模式。

### RDT Class
CRI-RM 中的 RDT 类配置是一个两级层次结构，包括Partitions和Class。它指定了一组partitions，每个partitions都有一组class。

- partition：partition代表底层Class的逻辑分组，每个partition指定一部分可用资源（L2/L3/MB），由其下的类共享。partition保证非重叠的独占缓存分配— — 即不允许partition之间的cache way之间有重叠。然而，根据技术，MB 分配不是独占的。因此，可以将所有partition分配100%的内存带宽。
- Classes：class代表容器实际被分配到的 RDT 类。与partition不同，特定partition下的类之间的缓存分配可能会重叠（通常确实如此）。可以自由指定 RDT 类的集合，但应确保指定了对应于 Pod QoS 类的类。同时，不得超过底层硬件支持的类（CLOS）的最大数量。

### 示例

以下是一个配置片段，它将60%的L3缓存行专门分配给 Guaranteed 类。剩余的40% L3 是为 Burstable 和 BestEffort 准备的，BestEffort 只获得这部分的50%。Guaranteed 类获得全部内存带宽，而其他类被限制为50%。

```
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

配置还支持更细粒度的控制，例如每个cache-ID的配置（即不同的socket可以有不同的分配）和 Code and Data Prioritization（CDP），允许为代码和数据路径分配不同的缓存。如果已知硬件细节，可以使用原始位掩码或位号（“0x1f”或0-4）代替百分比，以便精确配置缓存分配。有关 RDT 配置格式的详细描述和示例，请参阅 goresctrl 库文档。

### 动态配置
RDT 支持动态配置，即每当通过节点代理等接收到配置更新时，resctrl 文件系统会被重新配置。如果新配置与当前运行的容器不兼容（例如新配置缺少一个运行中的容器被分配到的类），配置更新将被拒绝。
