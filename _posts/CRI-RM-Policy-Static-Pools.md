---
title: CRI Resource Manager Static-Pools (STP) Policy
tags: CRI K8s
---
## 概览

Static-Pools（STP）内置策略的灵感来自CMK（Kubernetes的CPU管理器）。这是一个示例策略，展示了cri-resource-manager的能力，但并不被认为是生产就绪的。

基本上，STP策略旨在复制 CMK 的 `cmk isolate` 命令的功能。它还具有兼容性特性，可以作为插入式替换，以便更容易地进行测试和原型设计。
<!--more-->
特性：
- 可配置的任意数量的CPU列表Pool
- 通过节点代理动态配置更新

CMK兼容性特性：
- 支持与原始CMK相同的环境变量，除了：
  - CMK_LOCK_TIMEOUT 和 CMK_PROC_FS：在cri-resmgr上下文中不适用的配置变量
  - CMK_LOG_LEVEL：尚未实现
  - CMK_NUM_CORES：在cri-resmgr中不需要，因为我们直接从容器资源请求中获取这个值
- 支持CMK现有的配置目录格式以检索Pool配置
- 解析容器命令/参数，尝试检索cmk isolate的命令行选项
- 支持生成CMK特定的节点标签和污点（默认关闭）

## 配置
需要在配置中指定活动策略。策略特定选项控制Pool配置和遗留节点标签和污点。

```yaml
policy:
  Active: static-pools
  static-pools:
    # 设置为true以创建CMK节点标签
    #LabelNode: false
    # 设置为true以创建CMK节点污点
    #TaintNode: false
  ...
```
有关完整示例，包含所有可用配置选项，请参见示例ConfigMap。

如果使用节点代理进行动态配置，则策略选项（包括Pool配置）可能在运行时更改。

注意：活动策略（policy.Active）不能在运行时更改。要更改活动策略，需要重启cri-resmgr。

## Pool配置
Pool配置有三个可能的来源，按优先级递减顺序：

- CRI-RM全局配置
- 独立的Static-Pools配置文件
- CMK目录树

每当接收到重新配置事件（例如，来自节点代理）时，都会完全评估配置。因此，CRI-RM全局配置中出现的有效的Pool配置将优先于之前活动的基于目录树的配置。同样，从CRI-RM全局配置中移除Pool配置将使本地配置（文件或目录树）生效。

注意：cri-resmgr没有任何用于生成Pool配置的实用工具。因此，您需要自己手动编写一个，或者运行cmk init命令（原始CMK的）以创建遗留的配置目录结构。

## 全局配置

如果指定了全局CRI-RM配置中的配置（在policy.static-pools.pools下），则配置来自全局CRI-RM配置，具有最高优先级。参考示例：

```yaml
policy:
  static-pools:
    pools:
      exclusive:
        exclusive: true
        cpuLists:
        ...
      shared:
        cpuLists:
        ...
      infra:
        cpuLists:
        ...
```

## 独立的YAML文件
可以通过CRI-RM全局配置中的policy.static-pools.ConfFilePath选项指定独立配置文件的路径（默认为空）：

```yaml
policy:
  static-pools:
    ConfFilePath: "/path/to/conf.yaml"
```
配置文件的格式与全局CRI-RM配置中使用的Pool配置类似。您也可以查看示例配置文件作为起点。

## CMK目录树
STP策略还支持原始CMK的配置目录格式。它从CRI-RM全局配置中指定的位置读取配置policy.static-pools.ConfFileDir字段（默认为/etc/cmk）：

```yaml
policy:
  static-pools:
    ConfFileDir: "/etc/cmk"
```

## 调试
为了启用STP策略的更详细日志记录，设置LOGGER_DEBUG=static-pools环境变量或在CRI-RM全局配置中启用调试：

```yaml
logger:
  Debug: static-pools
```

## 运行工作负载
指定Pod配置的首选方式是通过环境变量。但是，必须通过请求cmk.intel.com/exclusive-cores扩展资源来保留独占核心。扩展资源的命名具有cmk前缀，以提供与原始CMK的向后兼容性。

## 使用环境变量配置Pod
支持以下环境变量：

- STP_NO_AFFINITY：不设置CPU亲和性。工作负载负责读取CMK_CPUS_ASSIGNED环境变量并自行设置亲和性。
- STP_POOL：要运行的Pool名称
- STP_SOCKET_ID：应分配核心的Slocket。设置为-1以接受任何Slocket。

在独占Pool中运行工作负载并从SlocketID 0保留一个核心的示例Pod规范：

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

#### 向后兼容cmk isolate
STP策略解析容器命令/参数，尝试检索Pod配置（来自cmk isolate选项）。这是为了与现有的CMK工作负载规范向后兼容。它操纵容器命令和参数，以便移除cmk isolate及其所有参数。

以下示例中，STP策略将在infraPool中运行sh -c "sleep 10000"。

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
