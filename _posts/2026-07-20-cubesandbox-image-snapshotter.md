---
title: "CubeSandbox 镜像与快照原理分析——与 containerd、Kata 对比"
tags: cubesandbox containerd kata-containers snapshotter reflink cubecow virtio-pmem rootfs microvm source-code
---

> 本文分析 CubeSandbox 中 **image（镜像）** 与 **snapshot（快照）** 的实现原理与具体细节，并与 **containerd**（overlayfs / erofs snapshotter）和 **Kata Containers** 的实现进行对比。
>
> 参考文档：`ubcache-snapshoter/docs/notes-ctr-run-rootfs.md`（containerd rootfs 链路）、`ubcache-snapshoter/docs/notes-kata-image-snapshotter-rootfs.md`（Kata 镜像/snapshotter）、`ubcache-snapshoter/docs/erofs-snapshotter.md`（erofs snapshotter）。

<!--more-->

## 0. 一句话总览

| 系统 | 镜像形态 | 快照单位 | rootfs 进入容器/VM 的方式 | COW 机制 |
|---|---|---|---|---|
| **containerd (overlayfs)** | OCI 分层 tar | **文件系统分层**（每层一个目录） | host 上 `mount -t overlay` 后 `pivot_root` | overlayfs copy-up |
| **containerd (erofs)** | 每层一个 `.erofs` blob | **blob 分层**（不可变只读镜像文件） | overlay 组合 blob 挂载点 / VM 块透传 | overlayfs（上层 ext4） |
| **Kata Containers** | 复用 containerd snapshotter | **VM 级**（跨 VM 传递 mount 描述） | virtio-fs / virtio-blk / pmem / nydus / erofs | 取决于 snapshotter |
| **CubeSandbox** | OCI 镜像经 CubeMaster 烘焙成 ext4 template（官方唯一路径） | **VM 级**（rootfs + 内存 整机快照） | **virtio-pmem (NVDIMM)** + virtio-fs | **cubecow：XFS reflink (FICLONE) 文件级 COW** |

CubeSandbox 的本质：**它不是 containerd snapshotter 的替代者，而是在 containerd pull/unpack 之上，叠加了一套 VM 级的快照子系统**——用 `cubecow`（XFS `FICLONE` reflink）把"rootfs 镜像文件 + VM 内存"做成 O(1) 的 COW 对象，配合 cloud-hypervisor 的 `vm.snapshot`/`VmRestore` 实现**毫秒级 clone / snapshot / rollback**，并通过 virtio-pmem 把 rootfs 注入 microVM。

---

## 1. containerd 的镜像与 snapshotter 原理（基线）

理解 CubeSandbox 之前，先建立 containerd 的基线。containerd 的核心是 **content store（存 blob）** + **snapshotter（存分层文件系统状态）** 两套原语。

### 1.1 拉取 → 解包 → Commit → Prepare → Mounts 全链路

```
ctr run (run.go)
 └─ NewContainer (run_unix.go)
 │   └─ WithNewSnapshot (container_opts.go:242)
 │       └─ image.RootFS() → 取 diffIDs
 │       └─ parent = ChainID(diffIDs)            # 算父链
 │       └─ snapshotter.Prepare(ctx, id, parent) # 建可写快照，返回 mount 描述
 └─ container.NewTask (container.go:226)
     └─ handleMounts (container.go:328)
     │   └─ snapshotter.Mounts(ctx, snapshotKey) # 取回 rootfs 的 mount.Mount
     └─ TaskService.Create → local.Create → TaskManager.Create
         ├─ NewBundle (写 config.json)
         ├─ ShimManager.Start (起 containerd-shim-runc-v2)
         └─ shimTask.Create ─ttrpc─▶ shim
             └─ runc.NewContainer
                 ├─ mount.All(mounts, bundle/rootfs)  # 真正 mount 系统调用
                 └─ Init.Create → go-runc.Create      # exec runc create，内部 pivot_root
```

关键点：**parent 的计算在 client 库里完成**（`client/container_opts.go:259` `parent := identity.ChainID(diffIDs).String()`），不在 daemon 里。kubelet 只发 CRI `CreateContainer`，从不碰 diffID/ChainID。

### 1.2 ChainID / DiffID / parent 链接

- **layer digest**（manifest 的 `layers[].digest`）= 压缩 blob 的哈希，用于从 content store 取字节。
- **diffID**（image config 的 `rootfs.diff_ids`）= **解压后**内容的哈希，用于解包校验 + 算 ChainID。
- **ChainID**：累积身份，递归 `chainID(A,B) = sha256("chainID(A) B")`。

解包时（`core/unpack/unpacker.go`），**每一层都以其 ChainID 为 key 被 Commit 进 snapshotter**：

```
chainID(layer0) → chainID(layer0,1) → … → chainID(all)   ← 顶层只读快照
```

容器创建时，取**整条 diffID 链的 ChainID**（= 顶层只读快照的 key）作为 parent，`Prepare(ctx, id=容器ID, parent)` 分叉出一个可写快照。

### 1.3 overlayfs snapshotter 磁盘布局

```
/var/lib/containerd/io.containerd.snapshotter.v1.overlayfs/
  metadata.db          ← BoltDB: string key(ChainID) → snapshot 编号 + parent + labels
  snapshots/
    1/  fs/  work/      ← 一层解压后的文件系统（lowerdir/upperdir 来源）
    2/  fs/  work/
    3/  fs/  work/
    4/  fs/  work/      ← 新容器可写层（upperdir=4/fs, workdir=4/work）
```

- BoltDB 里每个快照是一个以 string key 命名的 sub-bucket，O(1) 点查；`parent` 字段持久化，`parents()` 函数递归回溯祖先链得到 `ParentIDs=["3","2","1"]`。
- `Mounts()` 用 `ParentIDs` 拼出：

```
type=overlay
lowerdir=snapshots/1/fs:snapshots/2/fs:snapshots/3/fs
upperdir=snapshots/4/fs
workdir=snapshots/4/work
```

shim 的 `mount.All(mounts, bundle/rootfs)` 执行真正的 `mount(2)`，runc 再 `pivot_root`。

### 1.4 erofs snapshotter（参考 `erofs-snapshotter.md`）

erofs snapshotter 把每条 committed 快照存为一个 **EROFS 只读 blob 文件**（`layer.erofs`），而非逐文件解压的目录树：

```
snapshots/<N>/
  .erofslayer          ← 标记：erofs snapshotter 管理
  layer.erofs          ← EROFS 只读 blob
  fs/                  ← layer.erofs 的挂载点（lowerdir 来源）
  work/                ← overlay 工作目录
```

**挂载侧不变**——仍用 OverlayFS 组合（`lowerdir` 来自各 parent 的 `fs/`，即各 blob 挂载点）。差异在磁盘侧：解包用 differ 把 tar 流直接转成 EROFS blob（避免逐文件 inode 操作）；删除是 unlink 一个 blob；支持 `FS_IMMUTABLE_FL` + fs-verity + dm-verity 三层完整性；EROFS blob 可作为 virtio-blk/virtio-pmem 直接给 guest VM。

> 本质：erofs snapshotter 是"blockfile/devmapper 思路在文件系统层面的实现"——每条 committed 快照是一个不可变 blob，但仍通过 overlayfs 暴露给容器。

---

## 2. Kata Containers 的镜像与 rootfs 原理（VM 透传基线）

Kata 是 **containerd snapshotter 的消费者，不是替代者**。containerd 照常 `unpack → Prepare → Commit`（按 ChainID）、为每个容器 `Prepare(containerID, parent=ChainID)`、返回 `[]mount.Mount`。Kata 不像 runc 那样在 host 上挂载它，而是**把这份 rootfs 描述跨 VM 传递**给 guest 内的 kata-agent 执行最终 mount。

### 2.1 决策点：`checkAndMount`

`pkg/containerd-shim-v2/create.go:310` 根据 `CreateTaskRequest.Rootfs[0]` 的 `Type`/`Options` 分流：

```
Source 是块设备 (devmapper)        → Mounted=false → 块设备直通
Options 含 io.katacontainers.fs-opt.layer → FileSystemLayer
IsErofsRootFS()                    → EROFS 块透传
Type == "fuse.nydus-overlayfs"     → Nydus lazy-load
否则                               → host 上挂 overlay → virtio-fs 共享
```

`ShareRootFilesystem` 的关键分水岭是返回的 `SharedFile.containerStorages` 是否为 nil：

- **nil** → virtio-fs 路径：`kataGuestSharedDir` 已通过 virtio-fs 挂进 guest，host bind-mount 自动可见，agent 无需 mount。
- **非 nil** → block / nydus / erofs / fs-layer / virtual-volume：agent 收到 `[]grpc.Storage` 并按 driver 字符串分发到 `STORAGE_HANDLERS`。

### 2.2 五种 rootfs 传递方式与容器 `/` 文件系统

| 路径 | host 是否做 overlay | guest 是否 mount | 容器 `/` 文件系统 |
|---|---|---|---|
| **virtio-fs（默认）** | 是（containerd overlay） | 否 | **非 overlay**——virtio-fs 暴露已合并的树 |
| **块设备（devmapper）** | 否 | 是（agent mount /dev/vdX） | ext4 / xfs |
| **Nydus** | 否（rafs + snapshotdir） | 是（rafs + overlay） | overlay（lowerdir=rafs, upperdir=snapshotdir） |
| **EROFS** | 否（.erofs + rwlayer.img） | 是（erofs + ext4 + overlay） | overlay（lowerdir=erofs, upperdir=ext4） |
| **Guest Pull** | 否（guest 内拉镜像） | 是（overlay） | overlay |

存储 driver 字符串 → agent handler：`"overlayfs"`→OverlayfsHandler、`"blk"`→VirtioBlkPciHandler、`"mmioblk"`、`"scsi"`、`"pmem"`（带 `-o dax`）、`"virtiofs"`、`"erofs.multi-layer"`→MultiLayerErofsHandler、`"image_guest_pull"`→ImagePullHandler（经 CDH 在 guest 内拉镜像，用于机密容器）。

### 2.3 Kata 的关键洞察

> virtio-fs 路径下 Container 不使用 overlay——containerd 在 host 上已把 overlay 做好，guest 通过 virtio-fs 看到合并后的完整树，不感知 lower/upper/work。其他路径下 guest 内的 kata-agent 用 `mount -t overlay` 组装最终 rootfs。pmem/blk/ext4/xfs 只是数据"运输方式"，不影响是否需要 overlay。

Kata 没有自己定义"VM 级整机快照/回滚"——它的 snapshot 概念仍来自 containerd snapshotter（分层文件系统）。整机 checkpoint/restore 在 Kata 中是另一套（基于 QEMU migrate），不是其 rootfs 体系的核心。

---

## 3. CubeSandbox 的镜像实现

CubeSandbox 的 **Cubelet**（Go）实现了 CRI 风格的镜像服务。**官方镜像流程是单一路径**：OCI 镜像 → CubeMaster `tpl create-from-image` 烘焙成 ext4 template → Cubelet 下载 ext4 作 pmem rootfs（详见 §3.2、§3.4）。Cubelet 的 `CubeImageService.PullImage`（`cube_image_pull_route.go:127`）按 `ImageSpec.StorageMedia` 分发，有 ext4 与 docker 两个分支，但**只有 ext4 分支被官方 image→sandbox 流程使用**；docker 分支（§3.1）是代码里存在的另一条路径，非官方启动入口。

### 3.1 docker media 代码路径（containerd overlayfs，非官方启动入口）

> 本节描述 Cubelet `StorageMedia=docker`（默认值）分支的实现。官方 image→sandbox 流程走 §3.2 ext4 + §3.4 template，不经过此分支。

当 `StorageMedia` 为 `docker`（`isImageStorageMediaType` 的默认值，`cube_container_create.go:1492`）时，Cubelet 是 containerd v2 pull+unpack 管线的薄封装。`PullRegistryImage`（`image_pull.go:98`）调用 `c.client.Pull(...)`，带：

- `containerd.WithPullSnapshotter(snapshotter)`（来自 `snapshotterFromPodSandboxConfig`）
- `containerd.WithPullUnpack`——让 containerd 把分层解包进配置的 snapshotter

`config.toml` 配置的是标准 overlayfs snapshotter：

```toml
[plugins."io.containerd.snapshotter.v1.overlayfs"]
  root_path = "/data/cubelet/state"   # metadata (BoltDB)
  data_path = "/data/cubelet/root"    # 解包后的分层/snapshot 数据
[plugins."io.containerd.metadata.v1.bolt"]
  root_path = "/data/cubelet/state"
```

**即 CubeSandbox 复用 containerd 的 content store 和 overlayfs snapshotter**，没有注册自定义 containerd snapshotter 插件。`emount.go:139` 调 `rootfs.SnapshotRefFs` → `sn.View(...)` 并解析 overlay mount 的 `lowerdir=` 选项，佐证是原生 overlayfs。

镜像 store（`internal/cube/store/image/image.go`）是 per-namespace 的内存缓存（`images map[string]Image`），从 containerd `images.Store` 读穿填充，`Image` 结构记录 `ChainID`、`Snapshots`、`HostLayers`、`MediaType`、`UidFiles`。`HostLayers` 读取 `LabelImageLayerDirs`/`LabelImageHostLowerDirs` 标签——这是 image-as-volume 挂载获取 lowerdir 的来源。

### 3.2 ext4 OS 镜像（pmem 路径，官方 image→template 流程）

当 `cubeSpec.StorageMedia == ImageStorageMediaType_ext4` 时，**完全不经过 containerd**。`PullImage`（`cube_image_pull_route.go:167`）调 `ext4image.EnsurePmemFile(...)` 后返回 `ErrSkip`——即不发生 containerd pull。

这是一个**预构建的单文件 ext4 原始镜像 + 内核 blob**，通过 image-spec 注解里的 URL 拉取（`MasterAnnotationRootfsArtifactURL` / `MasterAnnotationRootfsArtifactSHA256`）。`EnsurePmemRootfs` + `tryDownloadPmemFile`（`ext4image/utils.go:36`）：

1. 检查 `<imageID>.ext4` 是否存在；
2. 缺失则从 artifact URL 流式下载到 `<imagePath>.download`，同时 SHA256 哈希；
3. 用 `MetaServerConfig.MetaServerEndpoint` 改写下载 host（`rewriteDownloadHost`）；
4. 校验 SHA256 通过后原子 `rename`。

磁盘布局（`Cubelet/pkg/container/pmem/pmem.go`）：

```
<baseDirPath>/<instanceType>_os_image/<imageID>/<imageID>.ext4   # raw ext4 rootfs
<baseDirPath>/<instanceType>_os_image/<imageID>/<imageID>.vm     # kernel artifact
<baseDirPath>/<instanceType>_os_image/<imageID>/<imageID>.ko
<baseDirPath>/cube-kernel-scf/vmlinux                            # 共享内核
```

`ensureArtifactRuntimeFiles` 保证 per-image `.vm` 内核文件存在（从共享内核刷新）。**注意：ext4 镜像不是在这里创建的**，而是从 meta-server 下载的预构建产物；`mkfs.ext4` 只在 storage plugin 里用于初始化 cubecow 支持的 ext4 emptydir 卷（`storage/plugin.go:49` `reflinkExt4InitCommands`），不用于 OS 镜像产物。

镜像删除有两条独立路径：标准 OCI 走 `image_remove.go` 的 `imageDeleteScheduler`（带 bolt 支持的孤儿镜像缓存 + 限速删除任务）；ext4 走 `ext4image/destroy.go` 的 `DestroyPmemArtifact`（带 in-use 检查，拒绝删被运行中 sandbox 引用的产物，不经过 `cdp.PreDelete`，因为模板级引用计数由 CubeMaster 权威维护）。

### 3.3 ext4image 包职责小结

| 文件 | 职责 |
|---|---|
| `ext4image/utils.go` | `EnsurePmemFile` = `EnsurePmemRootfs` + `ensureArtifactRuntimeFiles`；URL 下载、SHA256 校验、原子 rename、内核文件刷新 |
| `ext4image/destroy.go` | `DestroyPmemArtifact`：ID 校验、in-use 检查、路径校验（`pathutil.ValidatePathUnderBase`）、`os.RemoveAll`，幂等 |

### 3.4 OCI 镜像与 template 的关系：镜像必须先转成 template

CubeSandbox 官方不支持直接用 docker/OCI 镜像启动 sandbox。OCI 镜像须先用 `cubemastercli tpl create-from-image` 转成 ext4 template，再由 `Sandbox.create(template=...)` 创建 sandbox。

#### 3.4.1 官方工作流（image → template → sandbox）

官方文档（`docs/guide/quickstart.md`、`docs/guide/tutorials/template-from-image.md`、`docs/guide/tutorials/bring-your-own-image.md`）给出统一流程：

```
OCI Image  ──pull──►  ext4 rootfs  ──boot──►  Snapshot  ──register──►  Template READY
```

1. 写 Dockerfile，`FROM ghcr.io/tencentcloud/cubesandbox-base:...`（**必须内置 `envd` 监听 `:49983`**，否则就绪探针失败、SDK 不可用）。
2. `docker build && push` 到 registry。
3. `cubemastercli tpl create-from-image --image <OCI image> --expose-port <p> --probe <p> --probe-path <path>`——异步管线 `PULLING → BUILDING → DISTRIBUTING → READY`。镜像内必须跑 HTTP server，Cube 通过 HTTP 探针判定就绪后才拍快照。
4. 等 `template_status: READY`，记下 `template_id`。
5. `Sandbox.create(template=<template_id>)`——SDK 只吃 `template`，不吃 docker image。

#### 3.4.2 ext4 rootfs 怎么来的（CubeMaster 侧 `image.BuildExt4`）

`tpl create-from-image` 在 **CubeMaster 节点**把 OCI 镜像烘焙成单个 ext4 文件（`CubeMaster/pkg/templatecenter/image/`）：

1. `skopeo copy docker://<ref> oci:<dir>`——拉成本地 OCI layout；
2. `umoci unpack --rootless`——解成 rootfs 目录（`export.go`）；
3. 烘焙 CubeEgress root CA 进目录（`PostRootfsExport`，`mkfs.ext4` 冻结布局前）；
4. `mkfs.ext4 -F -d <rootfsDir> <artifactID>.ext4`——把目录冻结成单个 ext4 镜像文件（`ext4.go:52`）；
5. 算 SHA256 + size，注册为 rootfs artifact，生成 template 创建请求（`finalizeArtifact`）。

产出的 `.ext4` 就是 §3.2 里经 URL 下载、作 pmem rootfs 的预构建镜像。**OCI 镜像在 CubeMaster 侧被转成 ext4，再走 ext4/pmem 路径**——用 skopeo+umoci+mkfs.ext4，**不是** containerd pull，也**不是**把 OCI 分层直接经 virtiofs 给 guest 作 rootfs。

#### 3.4.3 Cubelet 代码里的 docker media / virtiofs 路径

Cubelet 代码里存在 docker media type（`StorageMedia` 默认 `docker`，`isImageStorageMediaType:1492`）分支：containerd pull OCI 镜像、解包进 overlayfs、把各层经 virtiofs 共享进 guest 作 overlay lowerdir、cubecow 卷作 upperdir（§3.1、`cube_container_create.go:562-614`）。`prepareContainerFiles` 按 per-container 的 `ImageSpec.StorageMedia` 分发，`RunCubeSandboxRequest` 支持 `repeated ContainerConfig containers`，同一 sandbox 内不同容器可分别走 ext4 与 docker 分支。该分支不在官方 image→template→sandbox 流程内（quickstart/SDK 一律用 `template=`）。

#### 3.4.4 template 传承链与 fast clone / 快照 / 回滚

从 template 创建 sandbox 时，`GetSnapshotTemplateID()`（`Cubelet/plugins/workflow/engine.go:160`）返回 templateID（需 `MasterAnnotationAppSnapshotTemplateID` 注解 + `InstanceType=cubebox`），存储走 `dealCowV2SandboxDefaultMedium` → `CreateSandboxRootfsFromTemplate`：

| 对象 | 何时产生 | 命名 |
|---|---|---|
| `tpl-<id>-build-rootfs` | AppSnapshot 构建模板（`dealCowTemplateBuildRootfs`） | volume |
| `tpl-<id>-rootfs` | CommitSandbox/AppSnapshot 提交（FICLONE） | snapshot |
| `tpl-<id>-memory` | 同上，VM 内存 | volume/snapshot |
| `sb-<id>-rootfs-gen<N>` | **从 template 创建 sandbox**（`CreateSandboxRootfsFromTemplate`，FICLONE 自 `tpl-<id>-rootfs`） | snapshot |

fast clone / 整机快照 / 原地回滚都围绕这条 rootfs-gen 传承链运作（`CommitSandbox` 的 `selectSnapshotRootfs` 按 `sb-<id>-rootfs-gen` 前缀找根卷，`cubecow_snapshot_artifacts.go:382`）。因为 ext4 template 是**自包含**的单文件镜像，`tpl-<id>-rootfs` 快照不依赖任何外部共享层，clone/rollback 才能干净地 O(1) FICLONE。

> 一句话：**官方不支持直接用 docker image 启动 sandbox；必须先 `cubemastercli tpl create-from-image` 把镜像转成 ext4 template，再 `Sandbox.create(template=...)`。template 既是启动单元，也是 fast clone/快照/回滚的基底。**

### 3.5 template 的存储特征

#### 3.5.1 无 overlay 分层：单个扁平 ext4 镜像

template 的 rootfs 内容是一个**单个扁平 ext4 文件系统**，没有 overlay 式的分层结构。形成过程在 CubeMaster 烘焙环节（`image.BuildExt4`）：

```
skopeo copy docker://<ref> oci:...   →   umoci unpack --rootless   →   合并后的 rootfs 目录
                                                                          │
                                            mkfs.ext4 -F -d <rootfsDir> <id>.ext4
                                                                          ▼
                                                              单个扁平 ext4 镜像文件
```

`umoci unpack` 已把 OCI 各层合并成一个目录，`mkfs.ext4 -d` 再把目录冻结成一个 ext4 文件系统。OCI 的 layer 边界、ChainID、diffID 在此消失——最终是一个普通 ext4，没有 lowerdir/upperdir/workdir，没有只读层叠加。

#### 3.5.2 pagecache 与磁盘共享

| | overlayfs（containerd） | CubeSandbox |
|---|---|---|
| 只读层磁盘共享 | lowerdir 目录被多容器共享 | 经 **FICLONE reflink**（XFS extent 引用计数）共享 |
| 页缓存共享 | 同一 lowerdir 文件 = 同一 host inode → 多容器共享一份 host pagecache | 不共享——每个 sandbox VM 在 guest 内核挂自己的 ext4，guest pagecache 各自独立 |

CubeSandbox 把 rootfs 做成块设备（virtio-pmem/NVDIMM）注入 VM，ext4 在 guest 内核里挂载。两个 sandbox 读同一文件，是两个 guest 内核各自把块读进自己的页缓存，跨 VM 不共享 pagecache。N 个跑同一镜像的 sandbox，每个在自己 guest 内存里缓存一份热数据，内存开销随 sandbox 数线性增长。

磁盘层面 FICLONE 省空间：`sb-<id>-rootfs-gen0` 与 `tpl-<id>-rootfs` 共享同一批 XFS extent，只有 guest 写入时才 CoW 断开。N 个 sandbox 的磁盘占用 ≈ 1 份 template + 各自脏页，不是 N 份完整 rootfs。

#### 3.5.3 template 在 Cubelet 磁盘上的形态

两类文件并存：

**(a) 烘焙好的 ext4 产物**（CubeMaster 构建、分发到 Cubelet 节点，`ext4image.EnsurePmemFile` 下载 + SHA256 校验）：

```
<baseDirPath>/<instanceType>_os_image/<imageID>/<imageID>.ext4   # 单个 raw ext4 镜像（几 GB）
<baseDirPath>/<instanceType>_os_image/<imageID>/<imageID>.vm     # 内核 artifact
```

路径来自 `Cubelet/pkg/container/pmem/pmem.go`。

**(b) cubecow 对象**（XFS(reflink=1) 上的常规文件，root_dir 一般为 `/data/cubelet/storage/cubecow-reflink`）：

```
<data_path>/cubecow-reflink/volumes/
    tpl-<templateID>-rootfs/
        └── tpl-<templateID>-rootfs        # template rootfs = 一个 ext4 镜像文件（cubecow snapshot）
    tpl-<templateID>-memory/
        └── tpl-<templateID>-memory        # VM 内存快照
    sb-<sandboxID>-rootfs-gen0/
        └── sb-<sandboxID>-rootfs-gen0     # sandbox 克隆 = FICLONE(tpl-...-rootfs)，共享 extent
    sb-<sandboxID>-rootfs-gen1/
        └── ...                            # rollback 后的新代
```

每个对象是一个普通文件（不是目录树），文件内容是一个完整 ext4 镜像。`sb-<id>-rootfs-gen<N>` 是 `tpl-<id>-rootfs` 的 FICLONE 克隆，两者在 XFS 上指向同一批数据 extent（refcount=2），guest 写入时才 CoW 出新 extent。template rootfs 的 kind 为 `snapshot`，sandbox 克隆也是 `snapshot`（snap-of-snap 扁平化到同一 origin 目录）。

构建衔接（AppSnapshot）：CubeMaster 下发的 `RunCubeSandboxRequest` 带 `StorageMedia=ext4`，Cubelet 起一个临时 cubebox 从 ext4 产物启动 → envd 探针通过 → `cube-runtime snapshot`（`vm.pause`/`vm.snapshot`/`vm.resume`）捕获内存 + 磁盘状态 → `CreateTemplateRootfsFromBuild` 用 FICLONE 把构建可写层 `tpl-<id>-build-rootfs` 克隆成 `tpl-<id>-rootfs`。template rootfs 是 reflink 快照，不是逐文件拷贝。

---

## 4. CubeSandbox 的快照实现

代码库里"snapshot"一词指代**三件不同的事**，必须区分：

### 4.1 (a) containerd snapshotter 同步器（仅用于 imagefs 统计）

`internal/cube/server/images/snapshots.go` + `internal/cube/store/snapshot/snapshot.go`。**这不是自定义 snapshotter**——`snapshotsSyncer` 周期性 `Walk` 已注册的 containerd snapshotter（overlayfs 等），把 `Kind`/`Size`/`Inodes` 缓存进内存 `Store` 供 imagefs-info 上报。`snapshot.Store` 是 `map[Key]Snapshot`，无持久化，纯统计缓存。

### 4.2 (b) `snapshot.sh`（节点引导）

`Cubelet/config/snapshot.sh` 是一次性引导脚本，对若干预设 CPU/内存规格（1C238M … 2C2192M）各跑一次 `cube-runtime snapshot`，每个 backed by `/data/cube-shim/disks/512Mi-ext4/disk0.raw`，最后 `touch /run/cube-shim/snapshot`。CubeShim 通过 `enable_snapshot()`（`CubeShim/shim/src/hypervisor/snapshot.rs:19`）检查该标志位决定节点是否支持 snapshot/restore。这是**节点级 provisioning**，不是 per-sandbox 操作。

### 4.3 (c) VM 级快照 / commit / rollback（真正的快照系统）

这是 CubeSandbox 的核心：**一套自定义的、VM 级快照机制**，与 containerd snapshotter 无关，作用于运行中的 cubebox（microVM），**同时捕获 rootfs 和内存**。后端唯一支持 `storage_backend = "cubecow"`（`storage/plugin.go:30`）。

#### 4.3.1 对象命名约定（`cubecow_snapshot_artifacts.go:468`）

| 对象名 | kind | 含义 |
|---|---|---|
| `tpl-<templateID>-build-rootfs` | volume | AppSnapshot 期间的可写工作层 |
| `tpl-<templateID>-rootfs` | snapshot | 已提交的 rootfs 快照（FICLONE 自 build-rootfs 或运行中 sandbox rootfs） |
| `tpl-<templateID>-memory` | volume / snapshot | 已提交的内存（full=volume，incremental=FICLONE 自上一个 memory） |
| `sb-<sandboxID>-rootfs-gen<N>` | snapshot | 某 sandbox 第 N 代 rootfs（FICLONE 自模板 rootfs 或另一个 snapshot） |

#### 4.3.2 AppSnapshot（构建模板）

`Cubelet/services/cubebox/appsnapshot.go`，RPC `AppSnapshot`。流程：

1. `storage.CreateTemplateMemoryVolume(templateID, memorySizeBytes)`——创建 cube-runtime 写内存页的 cubecow memory volume；
2. `executeCubeRuntimeSnapshot(...)`——exec `/usr/local/services/cubetoolbox/cube-shim/bin/cube-runtime`，snapshot 类型为 `full` / `incremental` / `soft-dirty`（常量 `appsnapshot.go:557`）。`incremental` 在 reflink 克隆的基内存文件上只写 CoW 匿名页；
3. `storage.CreateTemplateRootfsFromBuild(templateID)`——经 `CommitTemplateRootfs` 把 build rootfs volume 转成模板 rootfs snapshot；
4. 销毁临时 cubebox、去激活 cow 对象、`os.Rename(tmpSnapshotPath, snapshotPath)`。

#### 4.3.3 CommitSandbox（运行中 sandbox 打快照）

`Cubelet/services/cubebox/template_ops.go:30`：

1. 校验 `storage_backend=cubecow`；
2. 解析 sandbox 当前 rootfs 对象 `sb-<sandboxID>-rootfs-gen<N>`（`selectSnapshotRootfs`）；
3. **`storage.CommitTemplateRootfs(ctx, sourceRootfs, templateID)`** → `CowVolumeManager.CommitTemplateRootfs` → `engine.CreateSnapshot(sourceName, "tpl-<templateID>-rootfs", activate=false)`——一次 **FICLONE**；
4. 准备内存产物：reflink 克隆上一个快照的 memory blob 请求 soft-dirty 增量，或建空 volume 做 full（`prepareCommitMemoryArtifact` / `CommitTemplateMemoryFromBase`）；
5. `executeCubeRuntimeSnapshot` 调 cube-runtime → shim 的 snapshot 工具（`CubeShim/shim/src/snapshot/`）对 hypervisor 发 `vm.pause` → `vm.snapshot` → `vm.resume`，把 VM 内存写入 `tpl-<templateID>-memory`（soft-dirty 路径只写脏匿名页），状态目录写到 `<snapshotDir>/cubebox/<templateID>/<specDir>/snapshot/`；
6. 写 `memory.dev`、去激活 cow 对象、持久化快照目录、返回 `RootfsVol`/`MemoryVol`/`SnapshotPath`。

#### 4.3.4 RollbackSandbox（回滚）

`Cubelet/services/cubebox/rollback.go:50`：

1. 校验 backend、sandbox 运行中；
2. 解析当前 rootfs（`sb-<id>-rootfs-gen<currentGen>`），要求 `new_gen > currentGen`；
3. 解析目标快照的 rootfs/memory volume（请求或本地快照目录，`ResolveSnapshotForRollback` → `ResolveDevPath`，对 reflink 后端就是 `cubecow_get_volume_info` 返回文件路径）；
4. **`storage.RollbackDeriveNewGen(...)`** → `engine.CreateSnapshot(snapshotRootfsVol, "sb-<id>-rootfs-gen<newGen>", activate=true)`——**FICLONE** 出全新文件，必要时 `resizeSnapshotIfTooSmall` 扩容；
5. `buildRollbackRestoreConfig` 构造 `{ SourceURL: file://<metaDir>/snapshot, MemoryVolURL: <memory dev>, Disks: [{Path: <new rootfs dev>, ID: "disk-N"}] }`，把快照 disk spec 里的旧 rootfs 路径换成新 rootfs dev（`rollbackDisksFromSnapshotSpec`）；
6. `updateShimForRollback` 推 containerd task 注解 `cube.shimapi.update.action=RollbackSnapshot` + `cube.shimapi.update.rollback.restore_config=<json>`；
7. shim 侧 `service/update_ext.rs:do_rollback_snapshot` → `sb.rollback_vm(restore_config)`（`sb.rs:1301`）：
   - pause VM、断开 agent；
   - `delete_vm()`——`VmDelete` 关 VM 并销毁对象，**VMM 进程保活**（避免把内存写盘，无 checkpoint）；
   - 排干陈旧 hypervisor 事件；
   - `resume_vm_with_config` → `ch.resume_vm_cube_with_config` → hypervisor `VmRestore`/`VmResumeFromSnapshot`，`source_url` 指向快照目录、`memory_vol_url` 指向 cubecow memory 文件、`disks` 带新 rootfs 路径；
   - 重连 agent、reset guest、重启 `watch_oom`/`monitor_vm`；
8. 回到 Cubelet：持久化新 rootfs volume 名 + gen、删旧 rootfs volume（`DeleteCowObject`）、清 `RollingBack` 标志、`SyncByID`。

> 用户侧契约（`examples/snapshot-rollback-clone/09_rollback.py`）：写 v1 → checkpoint → 写 v2 → `sb.rollback(checkpoint_id)` → 状态回到 v1，**同一个 `sandbox_id` 不变**。

### 4.4 快照 RPC 服务（`cubebox.proto`，service `CubeboxMgr`）

| RPC | 作用 |
|---|---|
| `AppSnapshot` | 建 cubebox → app snapshot → 销毁；返回 `rootfs_vol`/`memory_vol`/`rootfs_kind`/`memory_kind`/`rootfs_size_bytes` + 版本元数据 |
| `CommitSandbox` | 对运行中 sandbox 打快照；请求 `sandboxID`/`templateID`/`snapshot_dir`，响应加 `rootfs_dev`/`memory_dev`（诊断用） |
| `RollbackSandbox` | 回滚；请求 `sandboxID`/`snapshotID`/可选 `rootfs_vol`/`memory_vol`/`meta_dir`/`new_gen`/`desired_size`，响应新 `rootfs_vol`/`new_gen`/`old_rootfs_vol`/`old_rootfs_deleted`/`memory_vol` |
| `CleanupTemplate` | 删本地模板快照/基数据（v4 起 `objects` 废弃，cubelet 从本地目录推导） |
| `ListSandboxSnapshots` / `ListLocalSnapshots` / `GetLocalSnapshot` | 检视本地快照目录（`LocalSnapshotInfo` 含 `snapshotID`/`snapshot_path`/`meta_dir`/`rootfs_vol`+`memory_vol`+kinds/`rootfs_size_bytes`/`build_rootfs_vol`/`kind`="template"|"runtime_snapshot"） |
| `GetStorageMetrics` / `InspectStorageVolumes` / `CleanupOrphanStorageFiles` | 存储检视/维护 |

没有显式 `Clone` RPC——克隆隐式由 `RollbackDeriveNewGen` / `CreateSandboxRootfsFromTemplate` 经 FICLONE 派生。`CowObjectRef`（`name`/`kind`="snapshot"|"volume"/`role`="rootfs"|"memory"|"build_rootfs"）。

CLI 入口：`cubecli cubebox snapshot`→`AppSnapshot`、`debug-commit`→`CommitSandbox`（DEBUG，绕过 CubeMaster）、`debug-rollback`→`RollbackSandbox`。

---

## 5. cubecow：FICLONE reflink COW 引擎

cubecow（`cubecow/`，Rust）是 CubeSandbox 快照的存储后端，提供 O(1) 文件级 COW 卷管理，经 C FFI 暴露给 Go。

### 5.1 机制：Linux `FICLONE` ioctl on XFS(reflink=1)

**不使用** btrfs subvolume、dm-thin、overlayfs、fiemap。使用 `FICLONE` ioctl（`0x40049409`）在 reflink-capable 文件系统（典型 XFS `-m reflink=1`，btrfs/OCFS2 亦可）上做文件级 reflink。

- `cubecow/src/engine/reflink.rs:87` `const FICLONE = 0x40049409;`
- `reflink.rs:974` `ficlone(src, dst_path)` 发 `ioctl(dst_fd, FICLONE, src_fd)`；失败则 unlink 半成品 dst 并返回 errno。
- `reflink.rs:998` `probe_reflink_support(dir)` 启动时做一次 dummy clone 探测，不支持则拒绝初始化。

设计取舍（模块文档 `reflink.rs:1`）：dm-thin 在块层给 O(1) 快照，但需 kernel-ioctl 编排、per-pool ledger、沉重的崩溃恢复面。对"大量大镜像文件的短命克隆"场景，reflink 文件系统在**文件层**经 FICLONE 给同样 O(1) 语义，每次操作是单个文件系统事务。

### 5.2 卷/快照模型与磁盘布局

```
<root_dir>/volumes/                ← AppConfig.backend.reflink.root_dir（默认 /var/lib/cubecow/reflink，cubelet 配 /data/cubelet/storage）
    ├── <vol-A>/
    │   ├── <vol-A>                ← 主卷文件（FICLONE 源）
    │   ├── <snap-1>               ← FICLONE(<vol-A>)
    │   └── <snap-2>               ← FICLONE(<snap-1>)，扁平化到 vol-A 目录
    └── <vol-B>/
        └── <vol-B>
```

关键不变量：

- **卷是普通文件**：`create_new` + `set_len(size_bytes)`，返回的 `device_path` 就是 reflink 挂载点内的文件路径（无 `/dev/mapper` 节点，无 device-mapper 激活）。
- **快照是 FICLONE 克隆**：`create_snapshot` 解析源（主卷或另一快照文件），调 `ficlone`。
- **snap-of-snap 扁平化**：`NameKind::Snapshot{origin_volume}` 指向**终极** origin 卷，snap of snap 仍落在 `<volumes_dir>/<ultimate_origin>/<snap>`，对外契约与 dm-thin 一致。
- **无磁盘 ledger**：所有元数据可从布局重建——`readdir(volumes/)` 得卷名，`readdir(volumes/<vol>/)` 减主卷得快照，size 用 `stat`，时间用 `mtime`。内存 `name_index: RwLock<HashMap<String, NameKind>>` 由 `scan_and_rebuild_index`（`reflink.rs:1125`）启动时重建。
- **全局命名空间**：卷名与快照名共享一个空间，不可重名。
- **崩溃恢复**（`reflink.rs:1103`）：零字节快照文件（creat 与 FICLONE 之间崩溃的孤儿）被 unlink，空卷目录被回收，其余重新注册。
- **带活动快照删卷**（`reflink.rs:421`）：只 unlink 主文件，目录因快照是独立 reflink 副本而存活——模板构建清理依赖此契约（删 `tpl-<id>-build-rootfs` 时同目录的 `tpl-<id>-rootfs` 快照存活）。
- **resize 只增**（`reflink.rs:491`），已有快照保持原 size。
- **activate 是 no-op**（`reflink.rs:853`）：device path 就是文件路径，FICLONE 返回即存在；保留仅为与 dm-thin 后端 trait 形状对齐。

### 5.3 Engine trait / 配置 / FFI

`Engine` trait 方法：`create_volume`/`delete_volume`/`resize_volume`/`get_volume_info`/`get_volume_block_info`/`list_volumes`/`create_snapshot`/`delete_snapshot`/`list_snapshots`/`activate_volume`/`deactivate_volume`/`reset_node_storage`/`metrics`。公开类型 `Volume{name,size_bytes,device_path,snapshot_count,created_at}`、`Snapshot{name,size_bytes,device_path,origin_volume,created_at}`、`REFLINK_BLOCK_SIZE=512`。目前仅 `BackendKind::Reflink` 后端。

FFI（`ffi.rs` + `include/cubecow.h`）：不透明句柄 `CubecowEngineHandle`（`Box<Box<dyn Engine>>`，单字指针），约定返回 0 成功/负错误码，字符串经 `char**` out-param 由 `cubecow_free_string` 释放，thread-local `cubecow_last_error()`，每函数 `panic::catch_unwind` 包裹。关键函数：`cubecow_init[_from_json]`、`cubecow_create_volume`、`cubecow_create_snapshot(source, snap, activate, out_dev)`、`cubecow_delete_volume/snapshot`、`cubecow_resize_volume`（只增）、`cubecow_get_volume_info`（卷和快照通用）、`cubecow_get_metrics`。错误码：`COW_OK=0`、`COW_ERR_NOT_FOUND=-1`、`COW_ERR_ALREADY_EXISTS=-2`、`COW_ERR_RESOURCE_EXHAUSTED=-3`、`COW_ERR_PANIC=-99` 等。

Go 绑定：`Cubelet/pkg/cubecow/cubecow.go`（cgo，链 `libcubecow.a`）包装上述 FFI；`Cubelet/storage/cubecow_engine.go` 提供 `Engine()` 访问器；`Cubelet/storage/cubecow_volume_manager.go` 的 `CowVolumeManager` 实现 `cowVolumeManager` 接口，方法 `CreateSandboxRootfsFromTemplate`/`RollbackDeriveNewGen`/`CommitTemplateRootfs`/`CommitTemplateMemory`/`CreateMemoryVolume` 直接映射到 cubecow `CreateVolume`/`CreateSnapshot`。

### 5.4 `reflink_ops` 基准（`benches/reflink_ops.rs`）

在 loopback 挂载的 XFS(reflink=1) 上跑端到端控制面基准，四场景：(1) 单源串行扇出 N 次 `create_snapshot`；(2) 链式快照深度 D；(3) M worker × N 并发扇出（测 `RwLock<name_index>` 写锁与 per-volume `fsync_dir` 争用，报 p50/p99）；(4) 脏 IO 交织（每次 snapshot 前对源 `pwrite`，回归 FICLONE 在脏页文件上需 flush 的抖动）。

### 5.5 cubecow 如何实现快 clone/snapshot/rollback

- **从模板建新 sandbox**：`CreateSandboxRootfsFromTemplate` → `RollbackDeriveNewGen(source=tpl-<id>-rootfs)` 产出 `sb-<id>-rootfs-gen0`，一次 FICLONE。10 GiB rootfs 毫秒级克隆——XFS 只更新 extent-refcount 元数据，不拷数据。
- **运行中打快照**：`cubecow_create_snapshot(sb-...-genN, tpl-<newID>-rootfs)`，又一次 FICLONE，与 rootfs 大小无关。
- **回滚**：`cubecow_create_snapshot(tpl-<snap>-rootfs, sb-...-gen<N+1>)`，再一次 FICLONE，然后 `VmRestore` 从快照内存状态恢复。无数据拷贝、无 checkpoint 写盘。
- **内存增量快照**：memory blob 本身是 reflink 文件，`CommitTemplateMemoryFromBase` 用 FICLONE 克隆上一个 memory blob，hypervisor 只写脏匿名页，干净页共享基 extents。
- **崩溃安全**：每个快照是单个 XFS 事务（FICLONE + 目录 fsync），无 per-pool ledger 需重启时对账。

---

## 6. CubeSandbox 的 rootfs 组装与 VM 注入

### 6.1 hypervisor 与 shim

hypervisor 是 Tencent fork 的 **cloud-hypervisor**（`hypervisor/`）。shim 是 Kata 派生的 Rust 组件（`CubeShim/`），构建 `VmConfig` 交给 hypervisor 起 VM。

### 6.2 rootfs 经 virtio-pmem (NVDIMM) 注入

**rootfs 是 virtio-pmem 设备**，guest 内出现为 `/dev/pmem0`。`CubeShim/shim/src/hypervisor/config.rs:72`（`impl Default for VmConfig`）是核心：

- 默认 kernel cmdline：`root=/dev/pmem0 rootflags=dax,errors=remount-ro ro rootfstype=ext4 panic=1 …`——guest 内核把 `/dev/pmem0` 挂为 ext4 根，dax 模式，只读。
- 默认 pmems：`PmemConfig{ file: <IMAGE_PATH>, discard_writes: true }`，`IMAGE_PATH = "/usr/local/services/cubetoolbox/cube-image/cube-guest-image-cpu.img"`（`config.rs:28`）。`discard_writes: true` 表示基础 guest 镜像只读。
- `add_pmem/add_pmems` 推额外 `PmemConfig{ file, size, discard_writes, id }`→guest 内 `/dev/pmem1`、`/dev/pmem2`……

`CubeShim/shim/src/sandbox/pmem.rs`：`Pmem::driver()="nvdimm"`（`pmem.rs:58`），`guest_device_path(i) = /dev/pmem{i+offset}`（`DEVICE_INDEX_OFFSET` 默认 1，shim 的 pmem 从 pmem1 起，pmem0 留给 guest image）。注释 `pmem.rs:30`："pmem0 is the guest root image"。

cubebox VM 设备分类：

| 设备 | 后端 | guest 路径 | 用途 |
|---|---|---|---|
| virtio-pmem pmem0 | `cube-guest-image-cpu.img`（只读，`discard_writes=true`） | `/dev/pmem0` 挂为 `/`（ext4 dax, ro） | 基础 guest 镜像 / agent rootfs |
| virtio-pmem pmem1.. | cubecow reflink 文件（`cubecow_get_volume_info` 的 `device_path`） | `/dev/pmem1`… | sandbox 可写 rootfs 层 / 数据卷 |
| virtio-blk | cubecow reflink 文件或 host 目录 | `/dev/vda`…，挂到 `/run/blk-cube/vd*` | 数据盘（`cube.disk` 注解，`disk.rs:28` driver `blk-cube`） |
| virtio-fs | host 共享目录经 virtiofsd | guest agent 挂载 | 共享 host 目录 / image-volume 层 |

> 注意：rootfs 走 virtio-pmem（NVDIMM），**不是** virtio-blk。`snapshot.sh` 里的 `disk0.raw` 是节点引导产物，不是 per-sandbox rootfs。

### 6.3 guest 内 rootfs 组装

shim 把 JSON blob 注入 OCI spec 注解 `cube.rootfs.info`。`agent/cube/src/rootfs.rs:14` `ANNOTATION_K_ROOTFS_INFO = "cube.rootfs.info"`，`RootfsInfo`（`rootfs.rs:40`）：

```rust
pub struct RootfsInfo {
    pub rootfs: Option<String>,
    pub pmem_file: Option<String>,         // 用作 rootfs 的 pmem 设备
    pub overlay_info: Option<OverlayInfo>, // virtiofs_lower_dir 用于 overlay
    pub mounts: Option<Vec<MountInfo>>,    // 额外 virtiofs bind 挂载
    pub ero_image: Option<EroImage>,
}
```

`OverlayInfo{virtiofs_lower_dir: Vec<String>}` 和 `MountInfo{virtiofs_source, container_dest, type, options}` 让 agent 用 virtiofs lower dir 建 overlay rootfs 并 bind-mount 共享 host 目录。`CubeShim/shim/src/container/mod.rs:339` 在构建 OCI spec 时把 `rootfs.pmem_file` 从 host 路径改写为 guest pmem 设备路径（经 `sb_conf.pmem_path_map` → `pmem.guest_bind_source(i)`）——cubecow volume 文件路径（host）变成 `/dev/pmemN`（guest），agent 挂为容器 rootfs。`agent/cube/src/rootfs.rs:82` `do_exec_mount()` 处理 exec 进程的 bind-mount 传播。

### 6.4 shim 的快照工具

`CubeShim/shim/src/snapshot/mod.rs` + `cmd.rs` 是快照创建工具（被 `cube-runtime` 调用），对 hypervisor 的 `vm.snapshot` HTTP API 发请求并写 `metadata.json`。`SnapshotInfo`（`mod.rs:27`）含 kernel/image/ch 版本、`vm_res{cpu,cpu_max,memory,disks,pmems}`、`memory_vol_url`（独立内存卷）、`app_snapshot_container_id`。快照类型：`full`/`incremental`（只 CoW 匿名页，自 restore 累积）/`soft-dirty`（自上次 soft-dirty 的真增量，无 `CONFIG_MEM_SOFT_DIRTY` 回退到 incremental）。`--memory-vol` 让内存数据写到独立 volume/URL——cubecow memory volume 就插在这里。

`do_app_snapshot`（`mod.rs:119`，运行中 VM 路径）：`vm.pause`→`vm.snapshot`→`vm.resume`，写 `metadata.json`。

### 6.5 shim↔Cubelet 控制面（`Cubelet/pkg/apis/shimapi/`）

经 containerd task 注解 + 到 hypervisor 的 unix-socket HTTP API：

- `disk.go` `ChDiskAPI.AddDisk/DelDisk`——热插 virtio-blk 数据盘，打 hypervisor `vm.add-disk`/`vm.remove-device`；
- `devices.go` `CubeShimDeviceAPI.AddDevices/DelDevices`——经注解 `cube.shimapi.update.action=HotplugDevice.Add|Del` + JSON 设备列表；
- `virtiofs.go` `CubeVirtioFSAPI.AddAllowedDirs`——扩展默认 virtiofs 的 `AllowedDirs` 经 `cube.fs` 注解推回，给 guest 授权额外 host 目录。

---

## 7. 端到端全链路

1. **模板构建一次**：`tpl-<id>-build-rootfs`(volume) → FICLONE → `tpl-<id>-rootfs`(snapshot)。
2. **建新 sandbox**：`sb-<id>-rootfs-gen0` = FICLONE(`tpl-<id>-rootfs`)。该文件路径交给 shim → 挂为 virtio-pmem 设备 → guest 挂 `/dev/pmemN` 为可写 rootfs（只读基 `cube-guest-image-cpu.img` 是 pmem0）。
3. **CommitSandbox**：FICLONE(`sb-<id>-rootfs-genN`) → `tpl-<newID>-rootfs`，外加增量内存快照进 FICLONE 克隆的 `tpl-<newID>-memory`。
4. **RollbackSandbox**：FICLONE(`tpl-<snap>-rootfs`) → `sb-<id>-rootfs-gen<N+1>`，然后 `VmDelete` + `VmResumeFromSnapshot`（disk spec 换新 rootfs 路径）。旧 rootfs volume unlink。同 `sandbox_id`，状态回滚。

每次 clone/snapshot/rollback 在 XFS reflink 层是 O(extent count)——guest 写入前不拷字节，写入时 XFS 惰性 CoW 断开共享 extent。这就是 CubeSandbox 快照/回滚/克隆快的根因。

---

## 8. 与 containerd、Kata 的对比

### 8.1 维度对比表

| 维度 | containerd (overlayfs) | containerd (erofs) | Kata Containers | **CubeSandbox** |
|---|---|---|---|---|
| **快照单位** | 文件系统分层（目录） | blob 分层（不可变 `.erofs`） | 复用 containerd（分层）；VM 级 checkpoint 另算 | **VM 级整机（rootfs + 内存）** |
| **快照接口** | `snapshotter` Prepare/Commit/Mounts/Stat/Remove/Walk | 同左 | 消费 `Mounts()`，不定义自己的 snapshotter | **自定义 RPC**（AppSnapshot/CommitSandbox/RollbackSandbox），非 snapshotter 接口 |
| **镜像形态** | OCI 分层 tar | OCI 分层 → EROFS blob | OCI 分层（复用 containerd） | **OCI 镜像 → CubeMaster 烘焙成 ext4 template**（官方唯一）；Cubelet 另有 docker media 代码路径（virtiofs，非官方启动入口） |
| **最终 rootfs 文件系统** | overlay（lowerdir+upperdir+workdir 目录） | overlay（lower=blob 挂载点，upper=ext4） | 视路径：virtio-fs 非 overlay / block=ext4 / nydus=overlay / erofs=overlay | **guest 内 ext4 on pmem**（pmem0 只读基 + pmemN 可写 cubecow 卷），必要时 agent overlay（virtiofs lower） |
| **COW 机制** | overlayfs copy-up（文件级，写时拷贝） | overlayfs（上层 ext4） | 取决于 snapshotter（overlay copy-up / dm-thin / nydus） | **XFS reflink FICLONE**（extent 级元数据 COW，文件级） |
| **rootfs 进入容器/VM** | host `mount -t overlay` + `pivot_root` | 同左 / VM 块透传 | virtio-fs / virtio-blk / pmem / nydus / erofs | **virtio-pmem (NVDIMM)** 为主，virtio-fs/virtio-blk 辅 |
| **是否容器进程直接用** | 是（runc 进程） | 是 | 否（VM 内 kata-agent） | 否（VM 内 cube agent） |
| **内存快照** | 无 | 无 | 有（QEMU migrate，独立体系） | **一等公民**：full/incremental/soft-dirty，reflink 克隆基内存只写脏页 |
| **回滚语义** | 无原生回滚（需重建容器） | 同左 | 无原生 rootfs 回滚 | **原地回滚**：同 `sandbox_id`，FICLONE 新代 + VmRestore |
| **O(1) 克隆** | 部分（lowerdir 共享，但 upper 写时拷贝） | 是（blob 共享） | 视 snapshotter | **是**（FICLONE，毫秒级，与 rootfs 大小无关） |
| **元数据存储** | BoltDB `metadata.db` | blob 文件 + 标记 | containerd BoltDB | **无 ledger**——从 XFS 目录布局重建 |
| **完整性** | 无内置 | FS_IMMUTABLE_FL + fs-verity + dm-verity | 视 snapshotter | 无内置（依赖文件系统） |
| **与 containerd 关系** | 即 containerd | 即 containerd | snapshotter 消费者 | template 构建用 skopeo+umoci+mkfs.ext4（不经 containerd）；Cubelet docker media 代码路径复用 containerd pull/overlayfs（非官方启动入口）+ 自建 VM 快照子系统 |

### 8.2 设计哲学差异

- **containerd**：通用容器运行时原语。snapshotter 是**文件系统分层**抽象，ChainID 串起 diffID 链，parent 在 client 库算。镜像与快照都是"分层 tar → 目录树/blob"的 OCI 范式，目标是"任何容器运行时（runc/kata/buildkit）都能消费同一份 mount 描述"。无内存、无 VM、无回滚概念。

- **Kata**：**跨 VM 传递 mount 描述者**。不重新发明 snapshotter，而是把 containerd 的 `Mounts()` 结果经 virtio-fs/blk/pmem/nydus/erofs 之一送进 VM，agent 内做最终 mount。其复杂度在"运输方式选择"与 guest agent 的 storage handler 分发。rootfs 仍是分层文件系统概念；整机快照是 QEMU migrate 的另一套，与 rootfs 体系松耦合。

- **CubeSandbox**：**VM 级整机快照子系统**。它接受了 Kata 的"rootfs 进 VM"思路，但**把快照单位从"文件系统分层"提升到"整机（rootfs+内存）"**，并用 **XFS reflink (FICLONE)** 替代 overlay copy-up / dm-thin，换来：
  - 毫秒级 clone/snapshot/rollback（只动 extent 元数据）；
  - 无 ledger 的崩溃恢复（目录布局即真相）；
  - 内存增量快照（reflink 克隆基内存 + soft-dirty 只写脏页）；
  - 原地回滚（同 sandbox_id 换 rootfs 代 + VmRestore）。

  代价是：强依赖 XFS(reflink=1) 文件系统；rootfs 是单文件 ext4 镜像（ OCI 分层只在标准镜像路径用，cubebox 模板走预构建 ext4 ）；放弃了 containerd overlayfs 的"任意目录共享 lower"灵活性，换 O(1) 块级 COW。

### 8.3 与 Kata 的具体技术差异

CubeSandbox 的 shim/agent 明显是 Kata 派生（同样的 `RootfsInfo` 注解模式、`STORAGE_HANDLERS` 风格、virtio-fs/virtio-blk 设备模型），但有关键分叉：

1. **rootfs 传输主通道不同**：Kata 默认 **virtio-fs**（host 做 overlay，guest 看合并树）；CubeSandbox 默认 **virtio-pmem/NVDIMM**（rootfs 是块设备文件 `cube-guest-image-cpu.img` + cubecow 卷，guest 直接 mount ext4 dax）。pmem 在 Kata 主要用于**卷**（`-o dax`），**几乎不用于 rootfs**；CubeSandbox 反过来把 pmem 作为 rootfs 主通道。
2. **快照语义不同**：Kata 没有"VM 级 rootfs+内存一体化快照/原地回滚"的原生 RPC——它的 snapshot 来自 containerd（分层）。CubeSandbox 定义了 `AppSnapshot`/`CommitSandbox`/`RollbackSandbox` RPC，配合 cloud-hypervisor 的 `vm.snapshot`/`VmRestore` 做整机快照恢复，且 rollback 是**原地**（同 sandbox_id）。
3. **COW 后端不同**：Kata 的 writable 层取决于所选 snapshotter（overlay copy-up / devmapper dm-thin / nydus）。CubeSandbox 唯一后端是 **cubecow (FICLONE)**，文件级 extent COW，无 ledger。
4. **内存快照一等公民**：CubeSandbox 的 memory volume 也是 cubecow 对象，支持 full/incremental/soft-dirty；Kata 的整机快照走 QEMU migrate，与 rootfs 体系分离。

### 8.4 与 containerd overlayfs 的代码级差异

1. **containerd overlayfs 仍在**（`config.toml` 里配），但只用于标准 OCI 镜像路径；CubeSandbox **不注册自定义 containerd snapshotter 插件**，`snapshots.go` 是统计同步器，`store/snapshot/snapshot.go` 是内存缓存。
2. **真"快照"是 VM 级**，不是文件系统分层级。标准 overlayfs snapshot 是目录层经 overlay mount 组合；CubeSandbox 的 commit/rollback 捕获整个运行中 microVM（rootfs+内存），经 `cube-runtime` + cubecow reflink。rootfs 是单文件 ext4 镜像（下载产物）或 cubecow FICLONE 卷，不是 overlay 目录树。
3. **Cubelet 按 `StorageMedia` 分发两个分支**：`""`/`docker`→containerd content+overlayfs（解包进 `/data/cubelet/root`）；`"ext4"`→`ext4image.EnsurePmemFile` 下载预构建 `.ext4`+内核，containerd 完全 bypass（`PullImage` 返回 `ErrSkip`）。**官方 image→sandbox 流程只用 ext4 分支**（见 §3.4）；docker 分支是代码路径，非官方启动入口。
4. **可写 sandbox 存储是 cubecow (FICLONE on XFS)，非 overlayfs**。新 sandbox rootfs 由 FICLONE 克隆模板 rootfs 快照（`RollbackDeriveNewGen`/`CreateSandboxRootfsFromTemplate`）；rollback = 从快照 FICLONE 新代 + 换 pmem 后端文件。O(1) 元数据级，对比 overlayfs 文件级 copy-up。
5. **rootfs 投递是 virtio-pmem/nvdimm，非 host pivot_root on overlay**。ext4 镜像/cubecow 卷以 `/dev/pmem0`(nvdimm, `pmem.rs:57`) 挂入 guest；image-volume 层和 host 目录经 virtiofs（`/run/virtiofs/<id>`）。
6. **内存快照一等公民**：`CommitSandbox`/`AppSnapshot` 把 VM 内存捕获进 cubecow memory volume（`tpl-<id>-memory`），full/incremental/soft-dirty。overlayfs 无内存概念。
7. **镜像 GC 自定义**：`image_remove.go` 的 `imageDeleteScheduler`（bolt 孤儿缓存 + 限速任务）；ext4 路径 `DestroyPmemArtifact` 带 in-use 检查。独立于 containerd 默认 GC。

### 8.5 snapshotter 模块角色映射：containerd ↔ cubecow

CubeSandbox 放弃了 overlay 分层，官方流程里没有 containerd 那种"按层组合"的 snapshotter 模块；快照管理职责由 cubecow 承担。对照关系：

| containerd | CubeSandbox 对应物 |
|---|---|
| `snapshotter` 接口（Prepare/Commit/Mounts/Stat/Remove/Walk） | cubecow `Engine` trait（`create_volume`/`create_snapshot`/`delete_snapshot`/`list_snapshots`/`resize_volume`/`metrics`） |
| snapshotter 插件实现（overlayfs / erofs） | cubecow reflink 后端（XFS FICLONE） |
| snapshot gRPC service | `CubeboxMgr` RPC（`AppSnapshot`/`CommitSandbox`/`RollbackSandbox`/`ListLocalSnapshots`） |
| `core/snapshots/storage` + proxy 层 | `CowVolumeManager`（Cubelet 侧封装，`CreateSandboxRootfsFromTemplate`/`CommitTemplateRootfs`/`RollbackDeriveNewGen`） |
| snapshot 单位 = 文件系统分层（ChainID 链） | snapshot 单位 = VM 整机（rootfs ext4 文件 + 内存） |
| 层链 `chainID(layer0) → chainID(0,1) → …` | 传承链 `tpl-<id>-rootfs → sb-<id>-rootfs-gen0 → gen1 → …`（FICLONE snap-of-snap） |

containerd overlayfs snapshotter 仍嵌入 Cubelet 二进制（`config.toml` 配 `[plugins."io.containerd.snapshotter.v1.overlayfs"]`），用于两处，均非官方 sandbox rootfs 路径：

1. docker media 代码分支（§3.1）——拉 OCI 镜像、解包进 overlayfs、virtiofs 共享分层；
2. imagefs 统计同步器（§4.1，`snapshotsSyncer` 周期 `Walk` 各 snapshotter 缓存 `Kind`/`Size`/`Inodes` 供 imagefs-info 上报）。

官方 image→template→sandbox 流程里，rootfs 走 ext4 + cubecow，containerd snapshotter 不参与（ext4 路径 `PullImage` 返回 `ErrSkip`，bypass containerd）。

---

## 9. 关键代码索引

### CubeSandbox（镜像 / 快照 / rootfs）

| 主题 | 位置 |
|---|---|
| 镜像拉取分发 | `Cubelet/internal/cube/server/images/cube_image_pull_route.go:127` |
| OCI 路径拉取 | `Cubelet/internal/cube/server/images/image_pull.go:98` |
| ext4 镜像下载/校验 | `Cubelet/internal/cube/server/images/ext4image/utils.go:36` |
| ext4 镜像销毁 | `Cubelet/internal/cube/server/images/ext4image/destroy.go:47` |
| 镜像 store（内存缓存） | `Cubelet/internal/cube/store/image/image.go` |
| snapshot 统计同步器 | `Cubelet/internal/cube/server/images/snapshots.go`；`internal/cube/store/snapshot/snapshot.go` |
| 节点 snapshot 引导 | `Cubelet/config/snapshot.sh` |
| AppSnapshot | `Cubelet/services/cubebox/appsnapshot.go` |
| CommitSandbox | `Cubelet/services/cubebox/template_ops.go:30` |
| RollbackSandbox | `Cubelet/services/cubebox/rollback.go:50` |
| 对象命名约定 | `Cubelet/storage/cubecow_snapshot_artifacts.go:468` |
| cubecow 卷管理器 | `Cubelet/storage/cubecow_volume_manager.go` |
| cubecow Go 绑定 | `Cubelet/pkg/cubecow/cubecow.go` |
| 快照 RPC 定义 | `Cubelet/api/services/cubebox/v1/cubebox.proto`（service `CubeboxMgr`） |
| pmem 设备模型 | `CubeShim/shim/src/sandbox/pmem.rs` |
| VmConfig 默认（kernel cmdline/pmem0） | `CubeShim/shim/src/hypervisor/config.rs:72` |
| guest rootfs 组装 | `agent/cube/src/rootfs.rs`；`CubeShim/shim/src/container/mod.rs:339` |
| shim 快照工具 | `CubeShim/shim/src/snapshot/mod.rs`、`cmd.rs` |
| shim rollback | `CubeShim/shim/src/sandbox/sb.rs:1301`；`service/update_ext.rs:98` |
| shim↔Cubelet 控制面 | `Cubelet/pkg/apis/shimapi/{disk,devices,virtiofs}.go` |
| cubecow reflink 引擎 | `cubecow/src/engine/reflink.rs`（FICLONE `:974`、probe `:998`、create_snapshot `:637`、scan_and_rebuild `:1125`） |
| cubecow FFI | `cubecow/src/ffi.rs`；`cubecow/include/cubecow.h` |
| cubecow 基准 | `cubecow/benches/reflink_ops.rs` |
| CLI | `Cubelet/cmd/cubecli/commands/cubebox/{snapshot,debug_commit,debug_rollback}.go` |
| SDK 示例 | `examples/snapshot-rollback-clone/` |

### 参考文档（ubcache-snapshoter/docs）

| 主题 | 位置 |
|---|---|
| containerd rootfs 全链路 | `notes-ctr-run-rootfs.md` |
| Kata 镜像/snapshotter/rootfs | `notes-kata-image-snapshotter-rootfs.md` |
| erofs snapshotter | `erofs-snapshotter.md` |

---

## 10. 小结

CubeSandbox 在镜像侧的**官方流程是单一路径**：OCI 镜像经 CubeMaster 的 `tpl create-from-image`（skopeo+umoci+mkfs.ext4）烘焙成 ext4 template，Cubelet 下载该 ext4 作 pmem rootfs（bypass containerd）。Cubelet 代码里另有 docker media 路径（containerd pull + virtiofs 共享 OCI 分层），但**非官方启动入口**，不在文档推荐流程内。在快照侧，它**没有重新实现 containerd snapshotter**，而是在 containerd 之上叠加了一套**VM 级整机快照子系统**：以 `cubecow`（XFS `FICLONE` reflink，文件级 extent COW，无 ledger）为唯一存储后端，把 rootfs 镜像文件和 VM 内存都做成 O(1) COW 对象，配合 cloud-hypervisor 的 `vm.snapshot`/`VmRestore` 与 virtio-pmem(NVDIMM) rootfs 注入，实现毫秒级 clone / snapshot / rollback 与原地回滚。

与 containerd 相比：快照单位从"文件系统分层"升级为"整机（rootfs+内存）"，COW 从 overlay copy-up 换成 reflink extent COW，rootfs 投递从 host pivot_root 换成 guest 内 ext4-on-pmem。与 Kata 相比：同样是"rootfs 进 VM"的 Kata 派生架构，但 rootfs 主通道从 virtio-fs 改为 virtio-pmem，并补齐了 Kata 所缺的"VM 级一体化快照/原地回滚/内存增量快照"原生 RPC——本质是用 reflink 把 Kata 的"运输层"扩展成了"运输 + 快照 + 回滚"一体层。
