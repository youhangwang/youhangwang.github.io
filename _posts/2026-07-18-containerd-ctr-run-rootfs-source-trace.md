---
title: "containerd ctr run rootfs 准备与挂载机制——源码深度追踪"
tags: containerd snapshot overlay container runtime source-code
---

# containerd `ctr run` rootfs 准备与挂载机制

> 本文整理自对 containerd v2 源码的跟踪，回答四个递进问题：
> 1. `ctr run` 时 containerd 如何准备 rootfs 并挂载？（完整调用链）
> 2. 创建容器时，`Prepare` 的 `parent` 从哪来？`diffID` 是什么？
> 3. 为什么 `parent` 的计算在客户端（client 库）完成？kubelet 场景是否也要重复实现？
> 4. `ctr` 会调用 CRI server 吗？为什么 `ctr` 不走 CRI 接口？

<!--more-->

---

## 一、`ctr run` rootfs 准备与挂载的完整调用链

### 总览

`ctr run` 分两个阶段产生 rootfs：

1. **创建容器**（`NewContainer`）：通过 snapshotter `Prepare` 一个可写快照层。此时只建立快照元数据，返回该快照的 `mount.Mount` 描述（**尚未真正挂载**）。
2. **创建任务**（`NewTask`）：向 snapshotter `Mounts` 取回该快照的挂载描述 → 经 task service → v2 TaskManager → runc shim → shim 内 `mount.All` 真正执行 mount 系统调用把 overlay 挂到 `bundle/rootfs` → 调 `runc create`（`pivot_root` 在 runc 内完成）。

### 关键分界

- **`Prepare`/`Mounts` 只产出挂载描述**，不执行挂载。
- **真正的 `mount(2)` 系统调用**发生在 runc shim 的 `mount.All`（`cmd/containerd-shim-runc-v2/runc/container.go:120` → `core/mount/mount_linux.go:80`）。
- **`pivot_root`/`chroot`** 在外部 `runc` 二进制内完成（`process/init.go:149` 经 `go-runc` exec `runc create`）。

### 详细调用链（带 file:line）

#### ① CLI 入口

`cmd/ctr/commands/run/run.go`
- `run.go:146` `Command.Action` — 解析 ref/id，调用 `NewContainer` 然后 `tasks.NewTask`。
- `run.go:185` `container, err := NewContainer(ctx, client, cliContext)`
- `run.go:230` `task, err := tasks.NewTask(...)` → `run.go:268` `task.Start(ctx)`

#### ② 创建容器 & 准备快照（Prepare）

`cmd/ctr/commands/run/run_unix.go`
- `run_unix.go:93` `NewContainer(...)`
- `run_unix.go:172-180` `image.IsUnpacked` → 若未解包则 `image.Unpack(ctx, snapshotter, ...)`（把镜像层应用到 snapshotter 存储）
- `run_unix.go:213` `cOpts = append(cOpts, containerd.WithNewSnapshot(id, image, ...))`

`client/container_opts.go`（**client 库，非 ctr**）
- `container_opts.go:242` `WithNewSnapshot` → `withNewSnapshot`
- `container_opts.go:254` `diffIDs, err := i.RootFS(ctx)`（取镜像 diffID 序列）
- `container_opts.go:259` `parent := identity.ChainID(diffIDs).String()`（算 ChainID 作为 parent）
- `container_opts.go:264` `s, err := client.getSnapshotter(ctx, c.Snapshotter)`（拿到 proxy snapshotter，gRPC 到 containerd snapshot 服务）
- `container_opts.go:276` `_, err = s.Prepare(ctx, id, parent, opts...)`（创建可写快照层）
- `container_opts.go:281` `c.SnapshotKey = id`（写入 container 元数据）

`client/client.go:340` `(c *Client) NewContainer` 运行各 `NewContainerOpts` 后，`client.go:372` 调 `ContainerService().Create` 持久化 container 记录（含 `Snapshotter`/`SnapshotKey`）。

snapshotter proxy / service 层：
- 客户端代理 `core/snapshots/proxy/proxy.go:94` `Prepare`、`:83` `Mounts`（gRPC 到 containerd）。
- 服务端 `plugins/services/snapshots/service.go:81` `Prepare`、`:121` `Mounts`。

snapshotter 实现（overlay 为例）`plugins/snapshots/overlay/overlay.go`
- `overlay.go:265` `(o *snapshotter) Prepare` — 建快照目录、生成 overlay 的 `mount.Mount{Type:"overlay", ...}` 并返回（此时**还没挂载**）。
- `overlay.go:428` `createSnapshot(KindActive)`。
- `overlay.go:277` `Mounts` → `:552` `mounts` 实际拼出 overlay 的 `lowerdir/upperdir/workdir`（无父层时退化为 bind）。

#### ③ 创建任务 & 取回 rootfs 挂载描述（Mounts）

`client/container.go`
- `container.go:226` `(c *container) NewTask(...)`
- `container.go:247` `c.handleMounts(ctx, request)`
- `container.go:328` `handleMounts`：
  - `container.go:340` `s, err := c.client.getSnapshotter(ctx, r.Snapshotter)`
  - `container.go:344` `mounts, err := s.Mounts(ctx, r.SnapshotKey)`（取回该快照的挂载描述）
  - `container.go:357-362` 把每个 `mount.Mount` 转成 `types.Mount` 塞进 `request.Rootfs`
- `container.go:297` `response, err := c.client.TaskService().Create(ctx, request)`（gRPC 到 tasks service）

#### ④ Tasks service → v2 runtime manager

`plugins/services/tasks/service.go:62` `(s *service) Create` → `s.local.Create`。

`plugins/services/tasks/local.go`
- `local.go:171` `(l *local) Create`
- `local.go:259-266` 把 `r.Rootfs` 还原为 `mount.Mount` 放入 `opts.Rootfs`
- `local.go:277` `c, err := rtime.Create(ctx, r.ContainerID, opts)`（`rtime` 即 v2 `TaskManager`）

#### ⑤ v2 TaskManager：建 bundle、（可选）激活系统挂载、起 shim

`core/runtime/v2/task_manager.go`
- `task_manager.go:159` `(m *TaskManager) Create`
- `task_manager.go:160` `bundle, err := NewBundle(...)` — `core/runtime/v2/bundle.go:47`：在 `<root>/io.containerd.runtime.v2.task/<ns>/<id>/` 下建 bundle、`rootfs` 子目录，写 `config.json`（`bundle.go:108-117`）
- `task_manager.go:180-186` 读取 shim 的 `handledMounts`（来自 shim 注解 `containerd.io/runtime-allow-mounts`，`task_manager.go:53`），决定哪些挂载类型由 runtime 自行处理
- `task_manager.go:189` `ai, err := m.mounts.Activate(ctx, taskID, opts.Rootfs, ...)` — `core/mount/manager/manager.go:132`。runc 声明能处理 `overlay`/`native` 等，这些留在 `opts.Rootfs` 交给 shim 自己挂；其他"系统挂载"由 daemon 激活，结果用 `ai.System` 替换 `opts.Rootfs`。若 shim 不实现该机制则返回 `NotImplemented`（`task_manager.go:209`），全部交给 shim
- `task_manager.go:213` `shim, err := m.manager.Start(ctx, taskID, bundle, opts)`（`ShimManager.Start`，`core/runtime/v2/shim_manager.go:208` — 启动/复用 `containerd-shim-runc-v2` 进程）
- `task_manager.go:232` `t, err := shimTask.Create(ctx, opts)`

#### ⑥ shim client → shim task service

`core/runtime/v2/shim.go`
- `shim.go:611` `(s *shimTask) Create`：把 `opts.Rootfs` 组装进 `task.CreateTaskRequest.Rootfs`（`shim.go:626-633`）
- `shim.go:635` `_, err := s.task.Create(ctx, request)`（ttrpc 调 shim 进程）

#### ⑦ runc shim：真正挂载 rootfs + runc create

`cmd/containerd-shim-runc-v2/task/service.go:223` `(s *service) Create` → `runc.NewContainer(ctx, s.platform, r)`（`service.go:229`）。

`cmd/containerd-shim-runc-v2/runc/container.go`
- `container.go:47` `NewContainer`
- `container.go:65-72` 把 `r.Rootfs` 转成 `process.Mount`
- `container.go:74-80` `rootfs = filepath.Join(r.Bundle, "rootfs")` 并 `os.Mkdir`
- `container.go:120` **`mount.All(mounts, rootfs)`** — 真正执行 mount 系统调用，把 overlay（及各层）挂到 `bundle/rootfs`
- `container.go:124` `p, err := newInit(ctx, r.Bundle, ..., rootfs)`

`cmd/containerd-shim-runc-v2/process/init.go`
- `init.go:110` `(p *Init) Create`
- `init.go:137-141` 构造 `runc.CreateOpts{PidFile, NoPivot: p.NoPivotRoot, ...}`
- `init.go:149` **`p.runtime.Create(ctx, r.ID, r.Bundle, opts)`** — `go-runc` 库 exec `runc create <id> <bundle>`，runc 读 `bundle/config.json`，以 `bundle/rootfs` 为根，在其内部完成 `pivot_root`/`chroot`。

#### ⑧ 底层 mount 系统调用

`core/mount/mount.go:51` `All(mounts, target)` 逐个调用 `Mount.mount`。
`core/mount/mount_linux.go:80` `(m *Mount) mount(target)` — 解析 options，最终调用 `unix.Mount(...)`（fuse 类型走 `mount.fuse` helper）。

### 调用链一图总结

```
ctr run (run.go:146 Action)
 └─ NewContainer (run_unix.go:93)
 │   └─ WithNewSnapshot (container_opts.go:242)
 │       └─ snapshotter.Prepare (overlay.go:265)   # 建可写快照，返回 mount 描述
 └─ tasks.NewTask → container.NewTask (container.go:226)
     └─ handleMounts (container.go:328)
     │   └─ snapshotter.Mounts (overlay.go:277)     # 取回 rootfs 的 mount.Mount
     └─ TaskService.Create (container.go:297)
         └─ local.Create (local.go:171)
             └─ TaskManager.Create (task_manager.go:159)
                 ├─ NewBundle (写 config.json)
                 ├─ mount.Manager.Activate (task_manager.go:189)  # 可选：daemon 挂系统挂载
                 ├─ ShimManager.Start (起 containerd-shim-runc-v2)
                 └─ shimTask.Create (shim.go:611) ─ttrpc─▶ shim
                     └─ runc.NewContainer (container.go:47)
                         ├─ mount.All(mounts, bundle/rootfs) (container.go:120)  # 真正 mount 系统调用
                         └─ Init.Create → go-runc.Create (init.go:149)  # exec runc create，内部 pivot_root
```

---

## 二、`Prepare` 的 `parent` 从哪来？`diffID` 是什么？

### 核心代码

`client/container_opts.go` `withNewSnapshot`：

```go
diffIDs, err := i.RootFS(ctx)                  // :254
parent := identity.ChainID(diffIDs).String()   // :259
...
parent, err = resolveSnapshotOptions(...)      // :268（userns 等可能改写）
s.Prepare(ctx, id, parent, opts...)            // :276
```

### 1. diffID 是什么

**diffID 是镜像每一层"解压后内容"的 SHA256 摘要**（压缩层 digest 解压后的内容哈希），存放在 OCI image config 的 `rootfs.diff_ids` 数组里。

`core/images/image.go:413` `RootFS(...)`：

```go
p, _ := content.ReadBlob(ctx, provider, configDesc)   // 读 image config blob
var config ocispec.Image
json.Unmarshal(p, &config)
return config.RootFS.DiffIDs, nil                     // []digest.Digest
```

`client/image.go:161` `(i *image).RootFS(ctx)` 返回的就是 `config.RootFS.DiffIDs` —— 第 i 个 diffID 对应第 i 层解压后的 tar 流哈希。

**diffID 的作用：**
- 解包（unpack）时校验：`diff.Apply` 把压缩层解压后，用 diffID 校验内容是否正确（`core/unpack/unpacker.go:542` `a.Apply`，随后 `unpacker.go:507` 把 `labels.LabelUncompressed = diffIDs[i]` 写到 content 上）。
- 作为计算 **ChainID** 的输入。

**两个 digest 要区分：**
- **layer digest**（manifest 里 `layers[].digest`）= 压缩后 blob 的哈希，用来从 content store 取字节。
- **diffID**（image config 里 `rootfs.diff_ids`）= 解压后内容的哈希，用来校验 + 算 ChainID。

### 2. ChainID 是什么 / 怎么算

ChainID 是 OCI 定义的"逐层累积身份"。对 diffID 序列 `[A, B, C]`，递归地：

```
chainID(A)       = A
chainID(A,B)     = sha256("A B")        // digest.FromBytes([]byte(A + " " + B))
chainID(A,B,C)   = sha256("chainID(A,B) C")
```

实现见 `vendor/github.com/opencontainers/image-spec/identity/chainid.go:30` `ChainID` / `:56` `ChainIDs`。整条链的 ChainID 唯一标识"把所有层依次应用后得到的文件系统状态"。

### 3. Prepare 的 parent 怎么得到

`container_opts.go:259`：

```go
parent := identity.ChainID(diffIDs).String()  // 整条链的 ChainID
```

**parent = 镜像全部 diffID 的 ChainID。** 这能成立，是因为 **unpack 时每一层就是以自己的 ChainID 为 key Commit 成快照的**：

`core/unpack/unpacker.go`
- `:372` `chainIDs = identity.ChainIDs(chainIDs)` 算出每层的累积 chainID
- `:415` `mounts, err = sn.Prepare(ctx, key, parent, ...)`（`key` 是临时解包用 key）
- `:494` **`sn.Commit(ctx, chainID, key, opts...)`** —— 解压校验通过后，把临时快照 Commit 成以 `chainID` 命名的永久快照
- `:491` `parent = chainIDs[i-1].String()`，下一层的 parent 就是上一层 chainID

所以 snapshotter 里已存在一条以 chainID 命名的快照链：
```
chainID(layer0) -> chainID(layer0,1) -> ... -> chainID(全部层)   ← 顶层只读快照
```

容器创建时，`withNewSnapshot` 取**最后一个 ChainID**（= 顶层只读快照的 key）作为 `parent`，然后 `Prepare(ctx, id=<容器id>, parent=<顶层chainID>)` 在其上分出一个可写快照（overlay 的 upperdir/workdir）。`c.SnapshotKey = id`（`container_opts.go:281`）记录的就是这个新的可写快照。

### 4. parent 还可能被微调

`container_opts.go:268` `resolveSnapshotOptions(...)`：启用 user namespace remap（`WithUserNSRemappedSnapshot` / `WithUserNSRemapperLabels`）等场景时，可在原 chainID 基础上派生新的 parent（例如带 remap 标签的快照）。默认普通路径下原样返回。

### 流程小结

```
镜像 image config
  └─ rootfs.diff_ids = [diffID0, diffID1, ..., diffIDN]   ← 每层"解压后内容"的 sha256

unpack 阶段（pull/Unpack 时一次性完成）
  逐层: Prepare(临时key, parent=上一层chainID) → Apply 解压+用 diffID 校验 → Commit(chainID_i, 临时key)
  最终在 snapshotter 里留下一条 chainID 命名的只读快照链，顶层 = ChainID(全部 diffID)

创建容器（NewContainer / WithNewSnapshot）
  diffIDs    = image.RootFS(ctx)                         ← container_opts.go:254
  parent     = identity.ChainID(diffIDs).String()         ← :259，= 顶层只读快照的 key
  parent     = resolveSnapshotOptions(..., parent, ...)   ← :268（userns 等可能改写）
  s.Prepare(ctx, id=<容器id>, parent)                     ← :276，在顶层只读快照上分出可写层
  c.SnapshotKey = id                                      ← :281
```

**一句话：parent 是镜像所有层 diffID 经 ChainID 算出的累积身份；因为 unpack 时每层快照正好以各自的 ChainID 命名 Commit，所以这个 ChainID 就直接对应 snapshotter 里那条只读快照链的顶层，作为新容器可写快照的 parent。diffID 是每层解压后内容的哈希，既用于解包校验，也是算 ChainID 的原料。**

---

## 三、为什么 parent 在客户端（client 库）算？kubelet 是否要重复实现？

### 纠正前提

parent 的计算**不在 `ctr` 二进制里，而在 containerd 的 Go client 库 `client/container_opts.go` 里**。`ctr` 只是调用该库的 CLI 薄壳。

- **谁 import 这个 client 库，parent 就在谁的进程里算。**
- `ctr` 进程里算 → 算完通过 gRPC `ContainerService.Create` 把整个 container 记录（含 `Snapshotter`/`SnapshotKey`）发给 daemon。
- containerd 的 gRPC 服务端（`ContainerService`）**不会**自己算 parent —— 它只接收已成型 container 对象，再由 snapshotter 服务端按客户端给的 `parent` 去 `Prepare`。

### kubelet 场景：kubelet 自己不算 parent

```
kubelet  ──CRI gRPC──>  containerd 进程内的 CRI server (internal/cri/server)
                          │
                          └─ CreateContainer (container_create.go)
                               └─ customopts.WithNewSnapshot(...)        ← internal/cri/opts/container.go:44
                                    └─ containerd.WithNewSnapshot(...)   ← client/container_opts.go:242 (同一个库函数!)
                                         └─ i.RootFS() → identity.ChainID(diffIDs) → parent
                                         └─ snapshotter.Prepare(ctx, id, parent)
```

关键点：
1. **kubelet 只发 CRI `CreateContainer` 请求**，里面带 image ref、container config 等，**完全不碰 diffID/ChainID**。
2. CRI server 跑在 **containerd daemon 进程内**，通过 in-process 的 `containerd.Client` 调用同一套 client 库。`internal/cri/opts/container.go:45` `f := containerd.WithNewSnapshot(id, i, opts...)` 就是直接复用 `client/container_opts.go` 的实现。
3. parent 计算逻辑**只有一份**，在 client 库里；ctr 和 CRI server 都复用它，**没有重复实现，kubelet 也不需要**。

CRI 版本只是在外面套了一层：如果发现 snapshot 还没解包（`NotFound`），先 `unpackImage`（`internal/cri/opts/container.go:52`，走 `core/unpack` 的 unpacker，同样按 chainID Commit 每层），再重试 `WithNewSnapshot`。

### 为什么设计成"客户端算 parent"而不是服务端算

containerd 服务端只提供**原语**（content store 存 blob/manifest/config、snapshotter 提供 Prepare/Commit/Mounts），而"用哪个镜像、哪条 diffID 链、要不要 userns remap、要不要多平台、用哪个 snapshotter"这些**策略**是客户端决定的。把 ChainID→parent 这层编排放在 client 库里，ctr / CRI / buildkit / nerdctl 等不同消费者就能各自定策略，但共享同一份正确实现。

服务端要算也不是不行（image config 就在 content store 里），但那样就把"快照编排策略"硬编码进 daemon，违背 containerd "原语在服务端、编排在客户端"的分层。

### 小结

| 问题 | 答案 |
|---|---|
| parent 在哪算？ | 在 containerd 的 **Go client 库** `client/container_opts.go` 里，不在 `ctr` 二进制内 |
| kubelet 要不要重复实现？ | **不要**。kubelet 只发 CRI `CreateContainer`；真正算 parent 的是 containerd daemon 内的 CRI server，它复用同一个 `containerd.WithNewSnapshot` 库函数 |
| 算 parent 的逻辑有几份？ | **一份**，在 client 库，ctr 和 CRI 共享 |

---

## 四、ctr 会调用 CRI server 吗？为什么 ctr 不走 CRI 接口？

### containerd 的两层 API

```
                    ┌─────────────────────────────────────────┐
   kubelet ─CRI──►  │  CRI plugin (internal/cri, plugins/cri)  │  ← k8s CRI gRPC
                    │   RuntimeService / ImageService          │     独立 socket
                    │   (pod/sandbox/image-pull 概念)          │
                    └────────────────┬────────────────────────┘
                                     │ 内部调用 native API（in-process client）
                    ┌────────────────▼────────────────────────┐
   ctr ─native─►    │  containerd native services              │  ← containerd 原生 gRPC
   nerdctl          │   ContainerService / TaskService /        │     主 socket
   buildkit         │   SnapshotService / ContentService /      │
                    │   ImageService / LeasesService ...        │
                    └──────────────────────────────────────────┘
```

- **原生 API**：containerd 自己定义的、细粒度的 gRPC 服务（`api/services/...`）。把容器、任务、快照、内容 blob、镜像都拆成独立资源。ctr / nerdctl / buildkit 都直接用这层。
- **CRI**：Kubernetes 定义的接口（`k8s.io/cri-api`），是给 kubelet 用的**高层、有主见**的抽象。containerd 把它实现成一个**插件**（`plugins/cri`），跑在 daemon 内部，**作为原生 API 之上的适配层**：把 CRI 的 pod/sandbox 语义翻译成一串原生 API 调用（建 sandbox 容器、建业务容器、配 CNI、按 CRI 规则拉镜像等）。

代码佐证：
- CRI 插件注册在单独 gRPC server：`plugins/cri/cri.go:208-209` `runtime.RegisterRuntimeServiceServer` / `RegisterImageServiceServer`。
- `ctr` 用原生客户端：`cmd/ctr/commands/client.go:67` `containerd.New(socketPath)`。

所以 CRI server 是原生 API 的**消费者/上层封装**，不是 ctr 的"另一条必经之路"。

### 为什么 ctr 不走 CRI

**1. 定位不同：ctr 是低层调试/管理工具，不是 pod 编排器。**
ctr 的目的就是直接驱动 containerd 原生 API，暴露所有细粒度旋钮（snapshotter 选择、runtime 选择、spec 自定义、rootfs 直接指定、checkpoint、namespace、lease……）。CRI 故意把这些藏起来，只暴露 kubelet 需要的 pod 级语义。走 CRI 等于自废武功。

**2. CRI 强制 pod/sandbox 模型，ctr 不需要。**
CRI 的 `CreateContainer` 必须在一个已存在的 **sandbox（pause 容器）** 里创建容器，要传 `SandboxId`、要处理 CNI 网络、要按 CRI 的 imagepull 策略拉镜像。ctr `run` 就是一个独立容器，没有 pod、没有 pause、不强制 CNI。绕道 CRI 还得先造一个 sandbox，纯属多余。

**3. CRI 接口有损。**
CRI 为了 kubelet 的统一抽象（要同时支持 containerd / cri-o / dockershim），刻意只保留最大公约子集。很多 containerd 原生能力（多 snapshotter、自定义 OCI spec、直接操作 content/snapshot、namespace、多 runtime shim、block device 快照如 overlaybd 等）CRI 根本表达不了。ctr 走 CRI 就失去访问这些能力。

**4. 历史与依赖方向。**
containerd 原生 API 先于 CRI 存在；CRI 是后来叠加上去的插件。ctr 一直是原生 API 的"参考客户端/测试工具"。架构上 CRI 依赖原生 API，而不是反过来 —— 让 ctr 去调 CRI 会把依赖方向搞反，也意味着 ctr 要依赖 k8s 的 CRI 类型，破坏它"轻量、与 k8s 解耦"的定位。

### parent 计算在两条路径下的位置

```
ctr 路径:
  ctr 进程 (client 库)  ──native gRPC──►  containerd daemon

CRI 路径:
  kubelet ──CRI gRPC──►  CRI plugin (daemon 内)
                           └─ in-process containerd.Client
                                └─ containerd.WithNewSnapshot  ← client/container_opts.go (同一份库代码)
                                     └─ ChainID(diffIDs) → parent
                                     └─ 调用原生 SnapshotService.Prepare
```

"parent 在 client 库算"对 ctr 和 CRI 都成立 —— ctr 的 client 在 ctr 进程，CRI 的 client 在 daemon 进程内。**kubelet 始终只是个 CRI gRPC 调用方，从不碰 diffID/ChainID。**

### 一句话总结

ctr 不调 CRI server、不走 CRI 接口，是因为 **ctr 和 CRI 是 containerd 两层不同的 API 面**：ctr 直连低层原生 API（细粒度、无 pod 概念），CRI 是给 kubelet 用的高层 pod 抽象插件（叠在原生 API 之上）。两者共享底层原生服务和那份 client 库代码，但各自面向不同消费者 —— ctr 要低层控制力，CRI 要 k8s 统一抽象，互不替代。

---

## 五、从磁盘外部行为角度看：parent 到底是怎么"找到"的

前面几章讲的是代码调用链。这一章完全抛开代码，只看**磁盘上实际有什么文件、parent 这个字符串是怎么对上的**。核心一句话：

> **parent 不是"搜"出来的，是一个字符串 key。unpack 时用这个 key 把快照存进 snapshotter 的元数据表；创建容器时用同一个算法把 key 算出来去查。两次用的是同一份 diffID 和同一个 ChainID 公式，所以天然对得上。**

下面用一个具体例子（3 层的 nginx 镜像）完整走一遍。

### ① content store：镜像的原始字节

路径形如 `/var/lib/containerd/io.containerd.content.v1.content/blobs/sha256/`，里面是按 digest 命名的普通文件（content-addressable，digest 即文件名）：

```
blobs/sha256/
  9f2a...   ← manifest.json        (文件名 = manifest digest)
  a1b2...   ← image config.json    (文件名 = config digest)
  3c4d...   ← layer0 压缩 tar.gz   (文件名 = 压缩层 digest)
  5e6f...   ← layer1 压缩 tar.gz
  7a8b...   ← layer2 压缩 tar.gz
```

其中 **image config.json** 里有两段关键字段：

```json
{
  "rootfs": {
    "type": "layers",
    "diff_ids": [
      "sha256:d0...",   ← layer0 解压后内容的 sha256
      "sha256:d1...",   ← layer1 解压后内容的 sha256
      "sha256:d2..."    ← layer2 解压后内容的 sha256
    ]
  }
}
```

注意区分两个 digest（这是最容易混淆的点）：

- **manifest 里的 `layers[].digest`** = **压缩**层的 digest（如 `3c4d...`）。containerd 用它从 content store 取字节流。
- **config 里的 `diff_ids`** = **解压后**内容的 digest（如 `d0...`）。用来校验解压内容、以及算 ChainID。

两者不同，一个对压缩流，一个对解压流。后面 parent 的计算只用 `diff_ids`。

### ② snapshotter：解压后的文件系统层（overlay 为例）

路径 `/var/lib/containerd/io.containerd.snapshotter.v1.overlayfs/`：

```
io.containerd.snapshotter.v1.overlayfs/
  metadata.db          ← BoltDB：字符串 key  →  快照编号 + parent + 标签
  snapshots/
    1/  fs/   work/    ← 一份解压后的文件层
    2/  fs/   work/
    3/  fs/   work/
```

- `snapshots/<编号>/fs/` 是实际文件内容（作为 overlay 的 lowerdir/upperdir）。
- `snapshots/<编号>/work/` 是 overlay 的工作目录。
- `<编号>`（1, 2, 3...）只是磁盘上的目录名，**对外寻址用的不是编号，而是 metadata.db 里的字符串 key**。
- `metadata.db` 是一张映射表：`字符串 key (ChainID)` → `快照编号 + parent 的字符串 key + 标签`。

### ③ unpack 阶段：把字符串 key 存进 metadata.db

`ctr run` 之前镜像必须先 unpack（pull 时自动做，或 `run_unix.go:177` 触发）。unpack 逐层处理，对 layer i（i=0,1,2）：

1. **算这一层的累积 ChainID**（递归定义）：
   - `c0 = d0`
   - `c1 = sha256( c0 + " " + d1 )` = `sha256("sha256:d0... sha256:d1...")`
   - `c2 = sha256( c1 + " " + d2 )`
2. `Prepare(临时key, parent=c(i-1))` —— 在 snapshotter 里分一块新目录。
3. 把 layer i 解压后的 tar 流写进去，**用 diffID `di` 校验**解压内容是否正确（防止传输/解压损坏）。
4. `Commit(key=c(i), 临时key)` —— **把这块快照正式以字符串 `c(i)` 为 key 登记到 metadata.db**，并把它的 parent 字段指向 `c(i-1)`。

unpack 完成后，metadata.db 里就有一条链：

| 字符串 key (ChainID) | 磁盘目录 | parent key | 含义 |
|---|---|---|---|
| `sha256:c0` | snapshots/1/ | (无) | 第 0 层解压后内容 |
| `sha256:c1` | snapshots/2/ | `sha256:c0` | 第 0+1 层叠完 |
| `sha256:c2` | snapshots/3/ | `sha256:c1` | 第 0+1+2 层叠完（顶层只读快照） |

`snapshots/3/fs/` 里就是 nginx 全部 3 层叠完的完整文件系统。**`sha256:c2` 这个字符串就是"顶层只读快照"的名字**，它代表"把这镜像所有层依次应用后的文件系统状态"。

### ④ 创建容器阶段：把同一个 key 算出来去查

`ctr run` 创建容器时（`client/container_opts.go` 的 `withNewSnapshot`）：

1. 从 content store 读 image config.json → 拿到 `diff_ids = [d0, d1, d2]`（就是 ① 里那块内容）。
2. **算 `parent = ChainID([d0, d1, d2]) = sha256:c2`**。
   - 这一步和 unpack 第 ③ 步用的是**同一个公式、同一份 diffID**，所以算出来必定是 `sha256:c2`。
3. `snapshotter.Prepare(key=容器ID, parent="sha256:c2")`。
4. snapshotter 拿字符串 `sha256:c2` 去 `metadata.db` 查 → 命中 snapshots/3/。于是新建 snapshots/4/（upperdir + workdir），parent 指向 3。

   **这一步的完整源码路径（四层调用链）：**

   **第 1 层 — overlay snapshotter 的 `Prepare`**
   
   `plugins/snapshots/overlay/overlay.go:265-267`：
   ```go
   func (o *snapshotter) Prepare(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
       return o.createSnapshot(ctx, snapshots.KindActive, key, parent, opts)
   }
   ```
   `Prepare` 只是一个薄包装，直接委托给 `createSnapshot`。参数 `key` 是新容器的 ID，`parent` 就是上面算出的 `"sha256:c2"`。

   **第 2 层 — `createSnapshot`：开启 BoltDB 写事务**
   
   `plugins/snapshots/overlay/overlay.go:428-467` 关键行：
   ```go
   func (o *snapshotter) createSnapshot(ctx context.Context, kind snapshots.Kind, key, parent string, opts []snapshots.Opt) (_ []mount.Mount, err error) {
       var (
           s        storage.Snapshot
           td, path string
           info     snapshots.Info
       )

       defer func() {
           if err != nil {
               if td != "" {
                   if err1 := os.RemoveAll(td); err1 != nil {
                       log.G(ctx).WithError(err1).Warn("failed to cleanup temp snapshot directory")
                   }
               }
               if path != "" {
                   if err1 := os.RemoveAll(path); err1 != nil {
                       log.G(ctx).WithError(err1).WithField("path", path).Error("failed to reclaim snapshot directory, directory may need removal")
                       err = fmt.Errorf("failed to remove path: %v: %w", err1, err)
                   }
               }
           }
       }()

       // :451 开启一个 BoltDB 写事务（o.ms.WithTransaction 内部打开 metadata.db）
       if err := o.ms.WithTransaction(ctx, true, func(ctx context.Context) (err error) {
           snapshotDir := filepath.Join(o.root, "snapshots")

           // :453-456 在磁盘上创建 snapshots/<N>/ 目录（N 由 snapshotter 分配）
           td, err = o.prepareDirectory(ctx, snapshotDir, kind)
           if err != nil {
               return fmt.Errorf("failed to create prepare snapshot dir: %w", err)
           }

           // :458 ★★★ 核心：在 metadata.db 中查 parent、创建新快照记录 ★★★
           s, err = storage.CreateSnapshot(ctx, kind, key, parent, opts...)
           if err != nil {
               return fmt.Errorf("failed to create snapshot: %w", err)
           }

           // :463-466 立即从 metadata.db 读回刚写入的 info（含 labels、parent 等）
           _, info, _, err = storage.GetInfo(ctx, key)
           if err != nil {
               return fmt.Errorf("failed to get snapshot info: %w", err)
           }

           // ... 后续处理 uid/gid mapping（:468-507），若启用 user namespace remap 则 chown upperdir
       }); err != nil {
           return nil, err
       }
       return o.mounts(s, info), nil
   }
   ```

   **第 3 层（核心）— `storage.CreateSnapshot`：BoltDB 内查 parent + 建新快照**
   
   `core/snapshots/storage/bolt.go:217-293`，按执行顺序拆解：
   
   **(a) 用 parent 字符串在 BoltDB 中查找父 bucket（:234-238）**：
   ```go
   if parent != "" {
       spbkt = bkt.Bucket([]byte(parent))   // ← 这就是 "metadata.db 查询"！
       if spbkt == nil {
           return fmt.Errorf("missing parent %q bucket: %w", parent, errdefs.ErrNotFound)
       }

       // :240-242 验证父快照必须是 Committed 状态
       if readKind(spbkt) != snapshots.KindCommitted {
           return fmt.Errorf("parent %q is not committed snapshot: %w", parent, errdefs.ErrInvalidArgument)
       }
   }
   ```
   `bkt.Bucket([]byte(parent))` — 直接用 `"sha256:c2"` 这个字符串作为 BoltDB 的 bucket name 做 O(1) 查找。BoltDB 的顶层 `snapshots` bucket 下，每个快照就是一个以字符串 key 命名的子 bucket。找不到就报 `ErrNotFound`，找到了还要验证它是 `KindCommitted`（只读快照必须是已 commit 状态）。
   
   **(b) 为当前 key（容器 ID）创建新 bucket（:244-249）**：
   ```go
   sbkt, err := bkt.CreateBucket([]byte(key))
   if err != nil {
       if err == errbolt.ErrBucketExists {
           err = fmt.Errorf("snapshot %v: %w", key, errdefs.ErrAlreadyExists)
       }
       return err
   }
   ```
   以容器 ID 为名称在 BoltDB 中创建新 bucket。如果已存在则报 `ErrAlreadyExists`。
   
   **(c) 分配数字编号（:252-255）**：
   ```go
   id, err := bkt.NextSequence()
   if err != nil {
       return fmt.Errorf("unable to get identifier for snapshot %q: %w", key, err)
   }
   ```
   BoltDB 的自增序列作为快照的数字编号。比如已有 snapshots/1、2、3，这一步就拿到 `id = 4`。
   
   **(d) 写入快照元数据（:257-267）**：
   ```go
   t := time.Now().UTC()
   si := snapshots.Info{
       Parent:  parent,       // ← 此处记录 parent = "sha256:c2"
       Kind:    kind,
       Labels:  base.Labels,
       Created: t,
       Updated: t,
   }
   if err := putSnapshot(sbkt, id, si); err != nil {
       return err
   }
   ```
   `putSnapshot`（:559）将上述字段序列化写入新 bucket 内的 key：
   - `"id"` → 数字 4 的 varint 编码
   - `"kind"` → 1 byte（Active=0）
   - `"parent"` → 字符串 `"sha256:c2"`（核心：parent 字段就此固化到 metadata.db）
   - `"created"` / `"updated"` → 时间戳
   - labels → 键值对
   
   **(e) 写反向索引 + 递归收集祖先链（:269-282）**：
   ```go
   if spbkt != nil {
       pid := readID(spbkt)   // 读取 parent 的数字 ID（即 3）

       // 在 "parents" bucket 中写一条反向索引：
       //   key = parentKey(pid=3, child=4) = 3\0x00\4 的编码
       //   value = 子快照的字符串 key（容器 ID）
       if err := pbkt.Put(parentKey(pid, id), []byte(key)); err != nil {
           return fmt.Errorf("failed to write parent link for snapshot %q: %w", key, err)
       }

       // 递归向上走完整条祖先链，收进 s.ParentIDs = ["1", "2", "3"]
       s.ParentIDs, err = parents(bkt, spbkt, pid)
       if err != nil {
           return fmt.Errorf("failed to get parent chain for snapshot %q: %w", key, err)
       }
   }

   s.ID = strconv.FormatUint(id, 10)   // s.ID = "4"
   s.Kind = kind
   ```
   `parents` 函数（`bolt.go:507-522`）从 parent bucket 出发，沿着每个 bucket 的 `"parent"` 字段递归向上，收集所有祖先的数字 ID：
   ```go
   func parents(bkt, pbkt *bolt.Bucket, parent uint64) (parents []string, err error) {
       for {
           parents = append(parents, strconv.FormatUint(parent, 10))
           parentKey := pbkt.Get(bucketKeyParent)   // 读当前 bucket 的 "parent" 字段
           if len(parentKey) == 0 {
               return   // 没有 parent 了，到头
           }
           pbkt = bkt.Bucket(parentKey)     // 跳到 parent bucket
           if pbkt == nil {
               return nil, fmt.Errorf("missing parent: %w", errdefs.ErrNotFound)
           }
           parent = readID(pbkt)            // 读 parent 的数字 ID
       }
   }
   ```
   对于例子，返回 `["3", "2", "1"]`（从近到远）。
   
   **辅助函数一览（均在 `bolt.go` 中）**：
   
   | 函数 | 行号 | 作用 |
   |---|---|---|
   | `readID` | :532-535 | 从 bucket 中读取 `"id"` key，varint 解码得到数字 ID |
   | `readKind` | :524-530 | 从 bucket 中读取 `"kind"` key（1 byte） |
   | `readSnapshot` | :537-557 | 从 bucket 中读取 id、kind、parent、时间戳、labels |
   | `putSnapshot` | :559+ | 向 bucket 写入 id、kind、parent、时间戳、labels |
   | `parents` | :507-522 | 递归向上走整条祖先链，收集所有祖先数字 ID |
   | `parentKey` | :54 | 编码 `parentID\0childID` 作为 parents bucket 的复合 key |

   **第 4 层 — BoltDB schema：metadata.db 内部结构**
   
   `core/snapshots/storage/bolt.go:36-50` 定义了所有 bucket/key 常量：
   ```go
   bucketKeyStorageVersion = []byte("v1")
   bucketKeySnapshot       = []byte("snapshots")
   bucketKeyParents        = []byte("parents")

   bucketKeyID     = []byte("id")
   bucketKeyParent = []byte("parent")
   bucketKeyKind   = []byte("kind")
   ```
   
   BoltDB 文件布局：
   ```
   v1/
     snapshots/
       "sha256:c0"          ← 子 bucket（key = ChainID 字符串）
         "id"     = 1       ← 数字 ID
         "kind"   = 3       ← Committed
         "parent" = ""      ← 无（最底层）
         "created" / "updated"
         labels...
       "sha256:c1"          ← 子 bucket
         "id"     = 2
         "kind"   = 3       ← Committed
         "parent" = "sha256:c0"
       "sha256:c2"          ← 子 bucket（顶层只读快照）
         "id"     = 3
         "kind"   = 3       ← Committed
         "parent" = "sha256:c1"
       "容器ID"              ← 子 bucket（新创建的可写快照）
         "id"     = 4
         "kind"   = 0       ← Active
         "parent" = "sha256:c2"
     parents/
       <parentID\0childID>  → 子快照的字符串 key（反向索引）
   ```
   
   关键点：**BoltDB 的 bucket 是以字符串 key 为名、O(1) 查找的**。所以 `bkt.Bucket([]byte("sha256:c2"))` 等同于一次哈希表查找，不需要扫描。`parent` 不是"搜"出来的，是精确点查。

   **第 4 层（补充）— Mounts 阶段也查 metadata.db**
   
   上面的 `createSnapshot` 返回后，overlay 的 `Mounts` 方法（`overlay.go:277-295`）也会在生成 overlay 挂载描述时查询 metadata.db：
   ```go
   func (o *snapshotter) Mounts(ctx context.Context, key string) (_ []mount.Mount, err error) {
       var s storage.Snapshot
       var info snapshots.Info
       if err := o.ms.WithTransaction(ctx, false, func(ctx context.Context) error {
           // :281 用字符串 key 查找快照的 Snapshot 结构（含 ParentIDs）
           s, err = storage.GetSnapshot(ctx, key)
           if err != nil {
               return fmt.Errorf("failed to get active mount: %w", err)
           }
           // :286 用字符串 key 查找快照的 Info（含 labels 等）
           _, info, _, err = storage.GetInfo(ctx, key)
           if err != nil {
               return fmt.Errorf("failed to get snapshot info: %w", err)
           }
           return nil
       }); err != nil {
           return nil, err
       }
       return o.mounts(s, info), nil   // 用 ParentIDs 拼出 lowerdir=1/fs:2/fs:3/fs
   }
   ```
   `GetSnapshot`（`bolt.go:182-214`）内部：
   ```go
   func GetSnapshot(ctx context.Context, key string) (s Snapshot, err error) {
       err = withBucket(ctx, func(ctx context.Context, bkt, pbkt *bolt.Bucket) error {
           sbkt := bkt.Bucket([]byte(key))   // ← 又是通过字符串 key 查 BoltDB
           if sbkt == nil {
               return fmt.Errorf("snapshot does not exist: %w", errdefs.ErrNotFound)
           }
           s.ID = strconv.FormatUint(readID(sbkt), 10)
           s.Kind = readKind(sbkt)
           if s.Kind != snapshots.KindActive && s.Kind != snapshots.KindView {
               return fmt.Errorf("requested snapshot %v not active or view: %w", key, errdefs.ErrFailedPrecondition)
           }
           if parentKey := sbkt.Get(bucketKeyParent); len(parentKey) > 0 {
               spbkt := bkt.Bucket(parentKey)
               if spbkt == nil {
                   return fmt.Errorf("parent does not exist: %w", errdefs.ErrNotFound)
               }
               s.ParentIDs, err = parents(bkt, spbkt, readID(spbkt))
               if err != nil {
                   return fmt.Errorf("failed to get parent chain: %w", err)
               }
           }
           return nil
       })
       return
   }
   ```
   这里再次用 `bkt.Bucket([]byte(key))` 做精确点查，然后读出 parent key 字段，再递归收集 `ParentIDs`。`GetInfo`（`bolt.go:78-95`）同理，内部调 `readSnapshot` 读出 `si.Parent = string(bkt.Get(bucketKeyParent))`（:543）。

   **完整调用链一图总结**：
   ```
   client/container_opts.go:276  s.Prepare(ctx, id, parent, opts...)
     │                            parent = "sha256:c2", id = 容器ID
     ▼
   overlay.go:265               Prepare → createSnapshot
     │
     ▼
   overlay.go:428-467           createSnapshot（开启 BoltDB 写事务）
     │  :451  o.ms.WithTransaction(ctx, true, ...)
     │  :453  o.prepareDirectory → 磁盘上建 snapshots/4/ 目录
     │  :458  storage.CreateSnapshot(ctx, kind, key, parent, opts...)
     │  :463  storage.GetInfo(ctx, key)
     ▼
   bolt.go:217-293              CreateSnapshot —— metadata.db 核心操作
     │  :235  bkt.Bucket([]byte(parent))         ← ★ 查 parent key（sha256:c2）
     │  :240  readKind(spbkt) != KindCommitted   ← 验证 parent 已 commit
     │  :244  bkt.CreateBucket([]byte(key))      ← 建当前快照 bucket（容器ID）
     │  :252  bkt.NextSequence()                 ← 分配数字编号（id=4）
     │  :265  putSnapshot(sbkt, id, si)          ← 写入 parent/kind/labels/时间戳
     │  :274  pbkt.Put(parentKey(pid, id), key)  ← 写反向索引
     │  :278  parents(bkt, spbkt, pid)           ← 递归收集 ParentIDs = [3,2,1]
     ▼
   bolt.go:507-522              parents —— 递归向上走祖先链
     bolt.go:532-535             readID —— 读数字 ID
     bolt.go:524-530             readKind —— 读快照类型
     bolt.go:537-557             readSnapshot —— 读完整 Info
     bolt.go:559+                putSnapshot —— 写完整 Info
   ```
   
   至此，第 4 步中的 metadata.db 查询和快照创建完成。snapshotter 返回的 `s`（`storage.Snapshot`）包含 `ID="4"`、`ParentIDs=["3","2","1"]`；`info` 包含 `Parent="sha256:c2"`、`Kind=Active`、labels 等。overlay 的 `mounts` 方法（`overlay.go:552`）用 `ParentIDs` 拼出 `lowerdir=snapshots/1/fs:snapshots/2/fs:snapshots/3/fs`，加上新创建的 `upperdir=snapshots/4/fs` 和 `workdir=snapshots/4/work`，组装成完整的 overlay 挂载描述返回。
5. 返回 overlay 挂载描述：
   ```
   type=overlay
   lowerdir=snapshots/1/fs:snapshots/2/fs:snapshots/3/fs
   upperdir=snapshots/4/fs
   workdir=snapshots/4/work
   ```
6. 之后 task 创建阶段，shim 的 `mount.All` 把这个 overlay 挂到 `bundle/rootfs`，runc 再 `pivot_root` 进去。

### ⑤ 本质：为什么这样一定能"找到" parent

- **不是运行时按内容搜索**，而是**按字符串 key 精确查找**（metadata.db 一次点查）。
- 这个 key（ChainID）是**镜像内容的纯函数**：只要镜像的 `diff_ids` 一样，算出的 key 就一样，与机器、与时间、与谁调用都无关。
- unpack 时用这个 key 作为快照名字存进 metadata.db；创建容器时用同样的 key 去查。**两次计算用的是 image config 里同一份 `diff_ids` + 同一个 ChainID 公式，所以一定能对上**——不需要快照反向去找镜像，也不需要做内容比对。
- 因此 content（存镜像 config 和层字节）和 snapshot（存解压后的文件层）两块存储之间，**靠 ChainID 这个字符串 key 解耦关联**，没有任何运行时反向查找。

### ⑥ 为什么用 ChainID 而不是直接用 diffID 当 key

因为每一份 committed 快照代表的是"累积到第 i 层的状态"，**不是"第 i 层单独的内容"**：

- 单层内容 → diffID（如 `d1`）。
- 累积状态 → 必须把前面所有层都揉进去的哈希 → ChainID（如 `c1 = sha256(c0 d1)`）。

snapshotter 里 snapshots/2/ 存的是"第 0+1 层叠完"的状态，它的名字必须是 `c1`（累积），而不是 `d1`（单层）。顶层快照 = 所有层叠完 = `ChainID(全部 diffID)`，所以新容器可写快照的 parent 用的就是它。

### ⑦ 一句话串起两块磁盘

> content store 里的 image config.json 给出 `diff_ids` → 算出 `ChainID` 字符串 → 这个字符串正好就是 snapshotter metadata.db 里 unpack 时存好的那条顶层只读快照的 key → 拿它当 parent 一查就找到磁盘上 `snapshots/<N>/fs/` 那叠层 → 在其上分出可写层并组装 overlay 挂载。

### ⑧ 配图：两块存储与 key 的关联

```
┌─────────────── content store ───────────────┐
│ blobs/sha256/                                 │
│   config.json  ──► rootfs.diff_ids = [d0,d1,d2]│
│   layer blobs (压缩)                           │
└───────────────────────┬───────────────────────┘
                        │ 读取 diff_ids
                        ▼
              ChainID([d0,d1,d2]) = "sha256:c2"   ← 字符串 key（镜像内容的纯函数）
                        │
                        │  ① unpack 时：以 c0/c1/c2 为 key 存入
                        │  ② 创建容器时：以 c2 为 parent 查找
                        ▼
┌─────────────── snapshotter ──────────────────┐
│ metadata.db:  "sha256:c2" → snapshots/3/      │  ← 字符串 key 精确点查
│               "sha256:c1" → snapshots/2/      │
│               "sha256:c0" → snapshots/1/      │
│ snapshots/                                    │
│   1/fs  2/fs  3/fs  (只读，叠成完整 rootfs)    │
│   4/fs  4/work  (新建可写层，parent=3)         │
└───────────────────────────────────────────────┘
```

---

## 附：关键 file:line 索引

| 主题 | 位置 |
|---|---|
| CLI 入口 | `cmd/ctr/commands/run/run.go:146`；`run_unix.go:93,213,457` |
| snapshot opt（算 parent） | `client/container_opts.go:242,254,259,268,276` |
| NewContainer | `client/client.go:340,372` |
| snapshotter proxy | `core/snapshots/proxy/proxy.go:83,94` |
| snapshotter service | `plugins/services/snapshots/service.go:81,121` |
| overlay snapshotter | `plugins/snapshots/overlay/overlay.go:265,277,428,552` |
| unpack（按 chainID Commit） | `core/unpack/unpacker.go:372,415,491,494,542` |
| image RootFS / diffID | `core/images/image.go:413`；`client/image.go:161` |
| ChainID 定义 | `vendor/github.com/opencontainers/image-spec/identity/chainid.go:30,56` |
| Task client（取 Mounts） | `client/container.go:226,247,297,328,344,357` |
| Task server | `plugins/services/tasks/local.go:171,259,277` |
| TaskManager（bundle+Activate+shim） | `core/runtime/v2/task_manager.go:159,160,189,213,232` |
| Bundle | `core/runtime/v2/bundle.go:47,108` |
| ShimManager.Start | `core/runtime/v2/shim_manager.go:208` |
| shim client | `core/runtime/v2/shim.go:611,626,635` |
| runc-v2 shim Create | `cmd/containerd-shim-runc-v2/task/service.go:223,229` |
| 真正挂载 rootfs | `cmd/containerd-shim-runc-v2/runc/container.go:47,74,120,124` |
| mount 系统调用 | `core/mount/mount.go:51`；`core/mount/mount_linux.go:80` |
| runc exec | `cmd/containerd-shim-runc-v2/process/init.go:110,137,149` |
| CRI snapshot opt（复用 client 库） | `internal/cri/opts/container.go:44,45,52` |
| CRI 插件注册 | `plugins/cri/cri.go:208,209` |
| ctr 连接方式 | `cmd/ctr/commands/client.go:67` |
| **BoltDB 存储层 — CreateSnapshot** | `core/snapshots/storage/bolt.go:217-293` |
| **BoltDB 存储层 — GetSnapshot** | `core/snapshots/storage/bolt.go:182-214` |
| **BoltDB 存储层 — GetInfo** | `core/snapshots/storage/bolt.go:78-95` |
| **BoltDB 存储层 — parents（递归祖先链）** | `core/snapshots/storage/bolt.go:507-522` |
| **BoltDB 存储层 — readID / readKind / readSnapshot / putSnapshot** | `core/snapshots/storage/bolt.go:532-559+` |
| **BoltDB 存储层 — parentKey 反向索引** | `core/snapshots/storage/bolt.go:54` |
