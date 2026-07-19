---
title: "Kata Containers Image、Snapshotter 与 Rootfs 挂载原理——源码深度追踪"
tags: kata-containers containerd snapshotter virtio-fs rootfs VM container runtime source-code
---

# Kata Containers Image、Snapshotter 与 Rootfs 挂载原理

> 本文整理自对 Kata Containers 源码的跟踪，回答以下递进问题：
> 1. Kata 和 runc 在 rootfs 处理上有何根本不同？
> 2. containerd shim v2 如何将 rootfs 信息传递给 Kata runtime？
> 3. Kata 有哪些 rootfs 共享方式？各自的原理和调用链是什么？
> 4. 每步操作发生在哪里——Host、Guest VM、还是 Guest 内的 Container？
> 5. Rootfs 如何在 Guest VM 内最终挂载为容器的根文件系统？
> 6. 抛开源码，从磁盘/文件的角度，各路径下实际产生了哪些目录、文件、挂载点？
> 7. runtime-rs (Rust) 和 runtime (Go) 在 rootfs 处理上有何差异？

<!--more-->

---


## 一、Kata vs runc：VM 边界带来的根本差异

### runc 的 rootfs 挂载

在标准 runc 场景下（参见 [containerd ctr run rootfs 准备与挂载机制](/2026/07/18/containerd-ctr-run-rootfs-source-trace/)）：
- containerd snapshotter 管理镜像层（overlay lowerdir/upperdir/workdir）
- shim 在 **host 命名空间**内执行 `mount.All`，将 overlay 挂到 `bundle/rootfs`
- runc 执行 `pivot_root`/`chroot` 切换到该 rootfs
- **所有操作都在 host kernel 上完成**

### Kata 的 rootfs 挂载

Kata 的容器运行在独立的 VM 内，有自己的 guest kernel。这意味着：
- **host 上的 rootfs 不能直接作为 guest 容器的根文件系统**
- 需要一种跨 VM 的文件系统共享机制
- 或者将镜像层作为块设备直通到 VM

### 核心问题

> 如何将 host 上 containerd snapshotter 准备好的容器 rootfs（overlay 或块设备），传递到 guest VM 内，成为 `/proc/1/root`？

Kata 提供了多种方式，按出现时间和技术路线可分为：

| 方式 | 传输介质 | 适用场景 |
|---|---|---|
| **virtio-fs** (默认) | 共享文件系统 | 通用场景，文件级别的 rootfs |
| **virtio-blk** (block device) | 块设备直通 | rootfs 源是块设备时（如 devmapper） |
| **Nydus + virtio-fs/nydusd** | rafs 懒加载 + overlay | 镜像加速（按需加载），机密容器 |
| **EROFS + virtio-blk** | 只读压缩块设备 | 高性能只读 rootfs |
| **Guest Pull** | 在 VM 内直接拉取镜像 | 机密容器（防 host 篡改） |

---

## 二、整体架构：从 containerd 到 Guest VM

### 两层 runtime

Kata Containers 有两个 runtime 实现：

```
src/runtime/          ← Go 实现 (containerd-shim-kata-v2)，当前主线
src/runtime-rs/       ← Rust 实现，新一代 runtime
```

本文以 Go runtime 为主介绍，Rust runtime 在第八章单独对比。

### 核心组件调用关系

```
[Host]
kubelet / ctr
    │ CRI / native gRPC
    ▼
containerd                          ────── [Host 进程]
    │ TaskService.Create
    ▼
containerd-shim-kata-v2 (src/runtime/cmd/containerd-shim-kata-v2/)
    │ shim v2 ttrpc                   ── [Host 进程]
    ▼
┌─── virtcontainers ─────────────── [Host 进程内执行] ───────────┐
│  Sandbox                                                      │
│   ├─ hypervisor (QEMU / CLH / Firecracker / Dragonball)       │
│   │    └─ 在 Host 上以独立进程运行，启动 Guest VM               │
│   ├─ fsShare (FilesystemSharer)     ── 操作 Host 文件系统      │
│   │    ├─ Prepare / Cleanup                                   │
│   │    ├─ ShareRootFilesystem()   ← 核心：rootfs 跨 VM 共享   │
│   │    └─ ShareFile()             ← 普通 volume mount 共享     │
│   ├─ agent (kata-agent client)      ── vsock 通信              │
│   └─ Container                                                │
│        ├─ rootFs (RootFs{Source, Type, Options, Mounted})     │
│        ├─ mounts ([]Mount)                                    │
│        └─ devices ([]ContainerDevice)                         │
└───────────────────────────────────────────────────────────────┘
    │ vsock / hybrid-vsock                                        │
    ▼                                                            │
┌─── Guest VM ───────────────── [Guest kernel 内] ───────────────┤
│  kata-agent (src/agent/)         ── Guest 进程                 │
│   ├─ RPC: CreateContainerRequest                              │
│   ├─ STORAGE_HANDLERS: 处理各种 storage 类型                   │
│   │    ├─ VirtioFsHandler      (virtio-fs)                     │
│   │    ├─ VirtioBlkPciHandler  (virtio-blk PCI)                │
│   │    ├─ VirtioBlkMmioHandler (virtio-blk MMIO)               │
│   │    ├─ OverlayfsHandler     (overlayfs)                     │
│   │    ├─ ImagePullHandler     (guest pull)                    │
│   │    ├─ EphemeralHandler     (ephemeral 存储)                 │
│   │    └─ ScsiHandler          (virtio-scsi)                   │
│   ├─ mount/baremount                                           │
│   │    └─ mount(2) 系统调用，挂载 rootfs 到容器工作目录          │
│   └─ rustjail ────────────────────────────────────────────┐    │
│        │  创建 namespace, pivot_root 到 rootfs             │    │
│        ▼                                                  │    │
│  ┌── Container ── [Guest 容器 namespace 内] ──────────┐   │    │
│  │  /proc/1/root = 最终 rootfs                         │   │    │
│  └─────────────────────────────────────────────────────┘   │    │
└─────────────────────────────────────────────────────────────┘
```

---

## 三、Rootfs 结构体定义 **[Host 侧 — shim 进程内]**

### 从 containerd 传入的 Rootfs

```go
// src/runtime/virtcontainers/container.go:324
type RootFs struct {
    Source  string   // BlockDevice 路径，或 overlay lowerdir
    Target  string   // 如果已挂载，则为挂载点路径
    Type    string   // 文件系统类型，如 "overlay", "ext4", "fuse.nydus-overlayfs"
    Options []string // fstab 风格的挂载选项
    Mounted bool     // rootfs 是否已被 containerd shim 挂载
}
```

### shim 层：如何获得 Rootfs

在 `src/runtime/pkg/containerd-shim-v2/create.go:76` 的 `create()` 函数中：

```go
// create.go:76-83
rootFs := virtcontainers.RootFs{}
if len(r.Rootfs) == 1 {
    m := r.Rootfs[0]
    rootFs.Source = m.Source
    rootFs.Type = m.Type
    rootFs.Options = m.Options
}
```

其中 `r *taskAPI.CreateTaskRequest` 的 `r.Rootfs` 正是来自 containerd 的 `container.go` 中 `handleMounts` 填充的 `types.Mount` 列表（与 runc 拿到的是同一份）。

### checkAndMount：决定是否由 shim 提前挂载

`create.go:310-337` 的 `checkAndMount` 函数判断：

```go
func checkAndMount(s *service, r *taskAPI.CreateTaskRequest) (bool, error) {
    if len(r.Rootfs) == 1 {
        m := r.Rootfs[0]

        // 1. 块设备 → 不挂载，直通给 VM
        if katautils.IsBlockDevice(m.Source) && !s.config.DisableBlockDeviceUse {
            return false, nil
        }
        // 2. FileSystem Layer → 不挂载
        if HasOptionPrefix(m.Options, annotations.FileSystemLayer) {
            return false, nil
        }
        // 3. EROFS → 不挂载
        if IsErofsRootFS(RootFs{Options: m.Options, Type: m.Type}) {
            return false, nil
        }
        // 4. Nydus → 不挂载
        if IsNydusRootFSType(m.Type) {
            return false, nil
        }
    }
    // 5. 默认路径：在 host 上挂载 rootfs 到 bundle/rootfs
    rootfs := filepath.Join(r.Bundle, "rootfs")
    if err := doMount(r.Rootfs, rootfs); err != nil {
        return false, err
    }
    return true, nil  // rootFs.Mounted = true
}
```

**关键决策：**
- 如果 rootFs.Mounted = false，Kata 会直接使用 rootFs.Source（块设备路径等）传递给 guest
- 如果 rootFs.Mounted = true，rootFs.Target 会被设置，Kata 通过 virtio-fs 等共享方式让 guest 看到这份已挂载的内容

### copyLayersToMounts：FileSystemLayer 的处理

`create.go:54-74` 的 `copyLayersToMounts` 函数处理 `io.katacontainers.fs-opt.layer=` 前缀的 rootfs option：

```go
func copyLayersToMounts(rootFs *virtcontainers.RootFs, spec *specs.Spec) error {
    for _, o := range rootFs.Options {
        if !strings.HasPrefix(o, annotations.FileSystemLayer) {
            continue
        }
        fields := strings.Split(o[len(annotations.FileSystemLayer):], ",")
        // fields = [path, fstype, options...]
        spec.Mounts = append(spec.Mounts, specs.Mount{
            Destination: "/run/kata-containers/sandbox/layers/" + filepath.Base(fields[0]),
            Type:        fields[1],
            Source:      fields[0],
            Options:     fields[2:],
        })
    }
    return nil
}
```

这些 layer 会被附加到 OCI spec 的 mounts 列表中，最终由 agent 在 guest 内处理（例如用 `OverlayfsHandler` 组装）。

### ForceGuestPull 注解

如果 sandbox 带有 `io.katacontainers.config.experimental_force_guest_pull` 注解（`annotations.go:310`），`ShareRootFilesystem` 的第一个判断就会走 `forceGuestPull` 路径（`fs_share_linux.go:662-678`），强制让所有容器的镜像都在 guest 内拉取，覆盖 snapshotter 的正常行为。

---

## 四、Rootfs 共享路径详解

### 4.1 virtio-fs 共享路径（最常见）**[Host→Guest 通过 virtio-fs 文件系统共享]**

#### 路径结构 **[Host]**

Kata 在 host 上维护了一个共享目录层次结构：

```
[Host]
/run/kata-containers/shared/sandboxes/<sandbox_id>/
    ├── shared/              ← virtio-fs/virtiofsd 共享给 guest 的根目录
    │                           （guest 内 mount 到 /run/kata-containers/shared）
    └── mounts/              ← host 侧挂载点（bind mount 到这里后，guest 可见）
         └── <container_id>-<random>-rootfs/
              └── (容器 rootfs 内容)

[Guest VM]
/run/kata-containers/shared/containers/<container_id>/rootfs
                              ← 通过 virtio-fs 从 Host 的 sharePath 映射而来

核心常量定义（`kata_agent.go:113-128`）：

```go
defaultKataGuestSharedDir = "/run/kata-containers/shared/containers/"
typeOverlayFS             = "overlay"
kataOverlayDevType        = "overlayfs"   // 告诉 agent 用 OverlayfsHandler
kataBlkDevType            = "blk"         // 告诉 agent 用 VirtioBlkPciHandler
```

路径函数（`kata_agent.go:229-243`）：

```go
func GetSharePath(id string) string {   // guest 可见的共享路径
    return filepath.Join(kataHostSharedDir(), id, "shared")
}
func getMountPath(id string) string {    // host 侧挂载点路径
    return filepath.Join(kataHostSharedDir(), id, "mounts")
}
```

#### Prepare 阶段（创建 sandbox 时）**[Host]**

`fs_share_linux.go:210` `FilesystemShare.Prepare()`:

1. 创建 `sharePath` 和 `mountPath` 目录
2. 将 `mountPath` bind mount 到 `sharePath`（**slave** 模式）
   - slave 模式意味着：在 `mountPath` 下的新挂载会传播到 `sharePath`
   - 这样 host 在 `mountPath` 下 bind mount 的 rootfs，自动出现在 guest 可见的 `sharePath` 中

```go
// fs_share_linux.go:244
// slave mount so that future mountpoints under mountPath are shown in sharePath as well
if err = bindMount(ctx, mountPath, sharePath, true, "slave"); err != nil {
    return err
}
```

#### ShareRootFilesystem：virtio-fs 路径 **[Host, 操作 Host 文件系统, 结果通过 virtio-fs 在 Guest 可见]**

当 rootfs **不是** nydus、不是 EROFS、不是块设备、不是 virtual volume 时，走这条路径：

`fs_share_linux.go:681` → 末尾（行 780）：

```go
// This is not a block based device rootfs.
// With virtiofs/9pfs we don't need to ask the agent to mount the rootfs
// as the shared directory (kataGuestSharedDir) is already mounted in the guest.
// We only need to mount the rootfs from the host and it will show up in the guest.
if err := bindMountContainerRootfs(ctx, getMountPath(sandboxID), c.id, c.rootFs.Target, false); err != nil {
    return nil, err
}

return &SharedFile{
    containerStorages: nil,    // ← 不需要 agent 再处理
    guestPath:         rootfsGuestPath,
}, nil
```

**关键设计：** 对于 virtio-fs 路径，`containerStorages` 为 nil。因为整个 `kataGuestSharedDir` 已经在 guest 内通过 virtio-fs 挂载好了，host 上 bind mount rootfs 的内容在 guest 中自动可见。这是最简洁的路径。

#### 在 agent 侧的 mount 处理 **[Guest VM]**

在 `kata_agent.go:939`，agent 处理容器 mount 时，如果 source 位于 `kataGuestSharedDir()`：
```go
if strings.HasPrefix(mnt.Source, kataGuestSharedDir()) {
    // 这些 mount 的 source 已经是 guest 内的路径，不需要额外 storage
    // 直接用 baremount 挂载
}
```

### 4.2 块设备路径（Block Device）**[Host→Guest 通过 virtio-blk 直通]**

当 rootfs 源是块设备（且 hypervisor 支持 BlockDevice）时：

#### 检测与 hotplug **[Host→Guest]**

`container.go:1905` `hotplugDrive`：

```go
func (c *Container) hotplugDrive(ctx context.Context) error {
    // 检查 rootfs 是否未挂载的块设备
    if !c.rootFs.Mounted {
        dev, err = getDeviceForPath(c.rootFs.Source)  // 获取 major/minor
        c.rootfsSuffix = ""  // 块设备没有 "rootfs" 子目录
    } else {
        dev, err = getDeviceForPath(c.rootFs.Target)
    }

    // 根据 BlockDeviceDriver 类型，hotplug 到 VM
    // 返回 device 信息，保存到 c.state.BlockDeviceID
}
```

#### ShareRootFilesystem：块设备路径 **[Host, 构建 Storage 消息发给 Guest Agent]**

`fs_share_linux.go:714`：

```go
if c.state.Fstype != "" && c.state.BlockDeviceID != "" {
    device := f.sandbox.devManager.GetDeviceByID(c.state.BlockDeviceID)

    blockDrive := device.GetDeviceInfo().(*config.BlockDrive)
    // 根据 BlockDeviceDriver 类型决定 agent storage driver：
    switch f.sandbox.config.HypervisorConfig.BlockDeviceDriver {
    case config.VirtioMmio:
        rootfsStorage.Driver = kataMmioBlkDevType    // "mmioblk"
        rootfsStorage.Source = blockDrive.VirtPath
    case config.VirtioBlock:
        rootfsStorage.Driver = kataBlkDevType         // "blk"
        rootfsStorage.Source = blockDrive.PCIPath.String()
    case config.VirtioSCSI:
        rootfsStorage.Driver = kataSCSIDevType        // "scsi"
        rootfsStorage.Source = blockDrive.SCSIAddr
    // ...
    }
    rootfsStorage.MountPoint = filepath.Join(kataGuestSharedDir(), c.id)
    rootfsStorage.Fstype = c.state.Fstype

    return &SharedFile{
        containerStorages: []*grpc.Storage{rootfsStorage},  // ← 带 storage 信息
        guestPath:         rootfsGuestPath,
    }, nil
}
```

**与 virtio-fs 路径的关键区别：** `containerStorages` 非 nil。agent 收到后，由对应的 `VirtioBlkPciHandler` 等将块设备挂载到指定目录。

### 4.3 Nydus 路径（镜像加速）**[Host→Guest 通过 nydusd virtio-fs + rafs]**

#### 原理

Nydus 使用 `nydusd` 替代 `virtiofsd` 作为 virtio-fs 守护进程，支持两种文件系统：
- **PassthroughFS**：将 host 共享目录透传到 guest（等同于 virtio-fs 功能）
- **Rafs**：Registry Acceleration File System，按需从 registry 拉取数据

#### 架构

```
Host:
  containerd-nydus-grpc (nydus snapshotter 代理)
      │  返回 fuse.nydus-overlayfs 类型的 mount
      ▼
  nydusd daemon (替代 virtiofsd)
      │  virtio-fs 协议
      ▼
Guest:
  kata-agent
      │  mount virtio-fs 到 /run/kata-containers/shared
      ▼
  /run/kata-containers/shared/
      ├── containers/          ← passthrough
      └── rafs/<cid>/lowerdir  ← rafs (按需加载的镜像层)
```

#### 容器 rootfs = overlay(lowerdir=rafs, upperdir=snapshotdir/fs, workdir=snapshotdir/work)

#### Nydus mount 的 extraoption 机制

由于 containerd 不认识 `rafs`，nydus snapshotter 使用一个巧妙的方式传递 rafs 参数：

```go
// nydusd.go:485-511
const extraOptionKey = "extraoption="

type extraOption struct {
    Source      string `json:"source"`       // nydus 镜像 bootstrap/metadata 文件
    Config      string `json:"config"`        // nydus 配置（backend 类型、缓存等）
    Snapshotdir string `json:"snapshotdir"`   // snapshotter 分配的目录
}
```

在挂载选项中，nydus snapshotter 返回：
```
Type: "fuse.nydus-overlayfs"
Options: [lowerdir=..., upperdir=..., workdir=..., extraoption=base64({source:..., config:..., snapshotdir:...})]
```

#### ShareRootFilesystem：Nydus 路径 **[Host, 通过 nydusd API 设置 Guest 内 rafs 挂载]**

`fs_share_linux.go:486` `shareRootFilesystemWithNydus`：

1. 解析 `extraOption`（base64 解码 + JSON 反序列化）
2. 通过 nydusd API (`/api/v1/mount`) 在 guest 中挂载 rafs 到 `/rafs/<cid>/lowerdir`
3. Bind mount snapshotter 分配的 `snapshotdir` 到 guest 可见路径
4. 构建 overlay storage：
   ```go
   rootfs.Driver = kataOverlayDevType  // "overlayfs"
   rootfs.Source = typeOverlayFS       // "overlay"
   rootfs.Options = [
       "upperdir=/.../<cid>/snapshotdir/fs",
       "workdir=/.../<cid>/snapshotdir/work",
       "lowerdir=/.../rafs/<cid>/lowerdir"
   ]
   ```

5. 返回带有 `containerStorages` 的 `SharedFile`，agent 用 `OverlayfsHandler` 创建容器 rootfs。

### 4.4 EROFS 路径 **[Host→Guest 通过 virtio-blk 直通 EROFS 块设备]**

EROFS 路径和 Nydus 类似，Agent 在 Guest 内做 overlay，但传输方式不同——EROFS 走 **virtio-blk 块设备**而非 virtio-fs 文件系统。

```
[Host]      containerd EROFS snapshotter 将每层镜像转为 .erofs 文件
            可写层是 rwlayer.img (ext4 sparse file)
            多层 EROFS 通过 VMDK descriptor 合并为单个虚拟块设备
              ↓ virtio-blk hotplug (QEMU: -drive file=xxx.erofs,format=raw,if=virtio)
[Guest]     /dev/vdc → erofs mount (只读 lowerdir)
            /dev/vdb → ext4 mount (可写 upperdir)
              ↓ agent: mount -t overlay -o lowerdir=/erofs_mount,upperdir=/ext4_mount/upper,workdir=/ext4_mount/work
[Container] overlay  ← 最终容器 rootfs
```

**关键特点**：
- 不需要 virtio-fs（可以 `shared_fs = "none"`）
- EROFS 是内核原生支持的高性能只读压缩文件系统
- 可选的 dm-verity 可以校验镜像完整性（防篡改）
- 多层 EROFS 使用 VMDK flat-extent descriptor 合并为单个块设备，由 QEMU 的 VMDK driver 处理
- 单层 EROFS：直接 attach raw block；多层：生成 VMDK descriptor 合并

对应的 host 侧代码：`fs_share_linux.go:584-660` `shareRootFilesystemWithErofs`。`IsErofsRootFS()` 通过检测 overlay lowerdir 中是否包含 `io.containerd.snapshotter.v1.erofs` 来判断。

### 4.5 FileSystemLayer 路径 **[Host→Guest, agent 在 Guest 内做 overlay]**

当 rootfs Options 中包含 `io.katacontainers.fs-opt.layer=` 前缀的标记时，Kata 走 FileSystemLayer 路径。

```
[Host]      containerd snapshotter 不 mount overlay
            rootfs Options 中包含每层的路径和类型：
              io.katacontainers.fs-opt.layer=/path/to/layer1,tar
              io.katacontainers.fs-opt.layer=/path/to/layer2,tar
              ↓ copyLayersToMounts 将每层信息注入 OCI spec mounts
[Guest]     agent 收到 OCI spec 中的 layer mounts
              ↓ OverlayfsHandler: mount -t overlay -o lowerdir=L1:L2,upperdir=...,workdir=...
[Container] overlay  ← 最终容器 rootfs
```

对应代码：`create.go:54-74` `copyLayersToMounts`（Host 侧处理层信息注入 OCI spec），`fs_handler.rs` `OverlayfsHandler`（Guest 侧执行 overlay mount）。

### 4.6 Guest Pull 路径（机密容器）**[Host 无镜像层, Guest 内拉取镜像]**

与前面所有路径不同，Guest Pull 路径下 **Host 上不存在镜像层的数据**。

```
[Host]      只有 image metadata (manifest, config)
            nydus snapshotter (proxy 模式) 不实际下载镜像层
              ↓ 通过 VirtualVolume 向 agent 传递 image URL + metadata
[Guest]     Confidential Data Hub 直接从 registry 拉取镜像
            解包到 /run/kata-containers/image/layers/sha256:xxx
              ↓ agent: mount -t overlay -o lowerdir=L1:L2,upperdir=...,workdir=...
[Container] overlay  ← 最终容器 rootfs
```

**pause 镜像特殊处理**：如果容器是 sandbox/pod 类型（`container_type: sandbox`），不会从 registry 拉取，而是使用 Guest rootfs 内预装的 pause 镜像（`unpack_pause_image`）。

**对应代码**：Host 侧 `fs_share_linux.go:662-678` `forceGuestPull`、`:571-582` `shareRootFilesystemWithVirtualVolume`；Guest 侧 `image_pull_handler.rs` `ImagePullHandler`。详见 `docs/design/kata-guest-image-management-design.md`。

### 4.7 补充：virtio-pmem 与 virtio-blk 的区别

pmem（持久内存）和 virtio-blk 都是块设备层面的传输方式，Guest 内看到的不同：

| 传输方式 | Guest 看到的设备 | Guest 内核访问方式 | 典型用途 |
|---|---|---|---|
| **virtio-blk** | `/dev/vdb`, `/dev/vdc` | 块 I/O（read/write 系统调用） | EROFS rootfs, devmapper rootfs |
| **virtio-pmem** | `/dev/pmem0` | DAX（直接内存映射，旁路 page cache） | Volume 挂载（`-v`），非 rootfs |

pmem 在 Kata 中主要用于 **volume** 场景（`handleBlockVolume` 在 `kata_agent.go:1902-1907`），rootfs 场景基本不使用 pmem。Agent 的 `PmemHandler` 处理 pmem 设备，mount 时带 `-o dax`。

如果 pmem 用于 rootfs（极少见），Container 内的 `/` 是 **ext4/xfs + DAX**，不是 overlay。

### 4.8 关键总结：各路径下 Container 最终使用什么文件系统？

这是理解 Kata 的 rootfs 机制时最容易被忽略的问题：**Container 内的 `/` 到底是什么文件系统类型？**

```
                        Host 做了 overlay?   Guest 做了 mount?   Container 内 / 的文件系统
                        ──────────────────   ────────────────    ────────────────────────
virtio-fs (默认)        ✅ containerd overlay  ❌ 不需要             不是 overlay!
                        bind mount merge 结果                      容器看到的是 virtio-fs
                        bundle/rootfs 已经是                       透传的 merge 后文件树
                        merge 后的完整文件树

块设备 (devmapper等)    ❌ 就是块设备          ✅ agent mount 设备    ext4 / xfs
                                                                (块设备本身的文件系统)

Nydus                   ❌ 提供 rafs +         ✅ agent mount       overlay
                         snapshotdir            rafs + overlay     (lowerdir=rafs,
                                                                  upperdir=snapshotdir)

EROFS                   ❌ 提供 .erofs +       ✅ agent mount       overlay
                         rwlayer.img            erofs + ext4       (lowerdir=erofs,
                                                + overlay          upperdir=ext4)

FileSystemLayer         ❌ 提供 layer 路径      ✅ agent mount       overlay
                                                overlay

Guest Pull              ❌ 什么都没提供         ✅ agent mount       overlay
                         Guest 自己拉取          overlay

pmem (volume,非rootfs)  ❌ 文件/pmem 后备       ✅ agent mount 设备   ext4 / xfs [DAX]
```

**一句话**：

> **virtio-fs 路径下 Container 不使用 overlay** — containerd 在 Host 上已经把 overlay 做好了，Guest 通过 virtio-fs 看到的是 merge 后的完整文件树，不感知 lower/upper/work 分层。**其他所有路径下 Container 都使用 overlay** — Guest 内的 kata-agent 将 Host 传来的各层（rafs/erofs/ext4/tar 等）通过 `mount -t overlay` 组装成最终的容器 rootfs。pmem/blk/ext4/xfs 只是底层数据的"运输方式"，不影响是否需要 overlay。

---

## 五、完整调用链

### Sandbox 创建（PodSandbox / SingleContainer）

```
[Host] ctr run / kubelet CreatePodSandbox
  │
  ▼
[Host] containerd TaskService.Create
  │  taskAPI.CreateTaskRequest (含 Rootfs)
  ▼
[Host] containerd-shim-kata-v2 service.go → create.go:create()
  │
  ├─ [Host] [1] 构建 RootFs 结构 (create.go:78-83)
  │      rootFs.Source = r.Rootfs[0].Source
  │      rootFs.Type   = r.Rootfs[0].Type
  │      rootFs.Options = r.Rootfs[0].Options
  │
  ├─ [Host] [2] checkAndMount (create.go:311-337)
  │      决定是否在 host 上挂载 rootfs
  │      块设备/nydus/erofs → rootFs.Mounted = false
  │      普通 overlay → rootFs.Mounted = true, rootFs.Target = bundle/rootfs
  │
  ├─ [Host] [3] katautils.CreateSandbox (create.go:190)
  │      │
  │      ├─ 设置 sandboxConfig.HypervisorConfig.SharedPath = GetSharePath(id)
  │      │
  │      └─ vci.CreateSandbox(sandboxConfig, ...)
  │           │
  │           ├─ [Host] sandbox.go:694  fsShare = NewFilesystemShare(s)
  │           │
  │           ├─ [Host] sandbox.go:637  fsShare.Prepare(ctx)
  │           │    │  fs_share_linux.go:210
  │           │    ├─ 创建 sharePath 和 mountPath (Host 文件系统)
  │           │    ├─ bindMount(mountPath, sharePath, "slave")
  │           │    │  保证 mountPath 下的新挂载传播到 sharePath
  │           │    └─ prepareBindMounts (sandbox bind mounts)
  │           │
  │           ├─ [Host] 启动 hypervisor (QEMU/CLH/Firecracker)
  │           │    │
  │           │    ├─ 若使用 nydus：启动 nydusd 代替 virtiofsd
  │           │    │   nydusd.go:85  nydusd.Start()
  │           │    │   nydusd 提供 virtio-fs 接口
  │           │    │
  │           │    └─ 否则：启动 virtiofsd (virtio-fs daemon)
  │           │        将 sharePath 作为 virtio-fs 共享目录
  │           │
  │           └─ [Guest VM] 连接 kata-agent，发送 CreateSandbox RPC
  │                └─ kata-agent 初始化 guest 内的 mount namespace
  │
  └─ [Host] [4] 返回 container 对象，记录 rootFs.Mounted
```

### 容器创建（PodContainer）

```
[Host] containerd TaskService.Create (容器)
  │
  ▼
[Host] containerd-shim-kata-v2 create.go:create()  [case PodContainer]
  │
  ├─ [Host] [1] checkAndMount → rootFs.Mounted 状态
  │
  ├─ [Host] [2] katautils.CreateContainer (create.go:231)
  │      │
  │      └─ sandbox.CreateContainer(ctx, contConfig)
  │           │  sandbox.go:1679
  │           │
  │           ├─ [Host] newContainer(ctx, sandbox, contConfig)
  │           │    container.go:738
  │           │    构造函数，保存 rootFs, mounts, devices 等
  │           │
  │           └─ [Host] c.create(ctx)
  │                container.go:1510
  │                │
  │                ├─ [Host→Guest] [3] 如果是块设备：c.hotplugDrive(ctx)
  │                │    将块设备 hotplug 到 VM (host 侧操作 QEMU/CLH)
  │                │
  │                ├─ [Host] [4] c.attachDevices(ctx)
  │                │    附加容器设备
  │                │
  │                └─ [Host] [5] sandbox.agent.createContainer(ctx, sandbox, c)
  │                     kata_agent.go:1564
  │                     │
  │                     ├─ [Host] [5a] sandbox.fsShare.ShareRootFilesystem(ctx, c)
  │                     │       │  fs_share_linux.go:681
  │                     │       │  ★ 操作 Host 文件系统：bind mount / 块设备 hotplug
  │                     │       │  ★ 构建 Storage 消息（将发往 Guest）
  │                     │       │
  │                     │       ├─ 判断 rootfs 类型，走不同路径：
  │                     │       │
  │                     │       │  isGuestPullForced()? → forceGuestPull
  │                     │       │  HasOptionPrefix(VirtualVolumePrefix)? → shareRootFilesystemWithVirtualVolume
  │                     │       │  IsNydusRootFSType? → shareRootFilesystemWithNydus
  │                     │       │  IsErofsRootFS? → shareRootFilesystemWithErofs
  │                     │       │  有 FileSystemLayer 选项? → 直接返回 storage
  │                     │       │  是块设备? → 构建 block Drive storage
  │                     │       │  否则 → bindMountContainerRootfs (virtio-fs 路径)
  │                     │       │
  │                     │       └─ 返回 SharedFile{containerStorages, guestPath}
  │                     │
  │                     ├─ [Host] [5b] c.mountSharedDirMounts (处理 volume mount)
  │                     │      每个 volume 调用 fsShare.ShareFile
  │                     │      收集 sharedDirMounts / ignoredMounts
  │                     │
  │                     ├─ [Guest VM] 以下步骤通过 vsock RPC 在 Guest 内执行：
  │                     │
  │                     └─ [Host→Guest] [5c] 发送 CreateContainerRequest RPC 到 kata-agent
  │                            │
  │                            ├─ container_id, OCI spec
  │                            ├─ storages = [rootfsStorages, shareStorages, ...]
  │                            ├─ devices
  │                            └─ 其他配置
  │
  └─ [Host] 返回
```

### Guest 侧 (kata-agent) **[Guest VM 内执行]**

```
kata-agent rpc.rs:1483
  │  收到 CreateContainerRequest
  │
  ├─ [1] add_storages(req.storages, &sandbox)
  │      storage/mod.rs:291
  │      │
  │      遍历每个 Storage，根据 driver 类型分派给对应 handler:
  │      │
  │      ├─ "blk" / "mmioblk" / "scsi" / "pmem"
  │      │    → 块设备 handler
  │      │    → 等待 /dev/vdX 设备出现
  │      │    → 可选：创建文件系统 (mkfs.ext4)
  │      │    → 可选：LUKS2 加密
  │      │    → mount(2) 将块设备挂载到指定 mount_point
  │      │
  │      ├─ "overlayfs"
  │      │    → OverlayfsHandler
  │      │    → 先创建 upperdir 和 workdir
  │      │    → mount(2) overlay:
  │      │        mount -t overlay overlay -o lowerdir=...,upperdir=...,workdir=... <mountpoint>
  │      │
  │      ├─ "virtiofs"
  │      │    → VirtioFsHandler
  │      │    → virtio-fs 共享目录已经在 guest 中挂载好了
  │      │    → 使用 bind mount 或直接在已有挂载路径上使用
  │      │
  │      ├─ "image_guest_pull"
  │      │    → ImagePullHandler
  │      │    → 判断是否为 sandbox 容器 → unpack_pause_image (使用 guest rootfs 内置的 pause 镜像)
  │      │    → 普通容器 → 调用 Confidential Data Hub (ttrpc) 从 registry 直接拉取镜像
  │      │    → 拉取的镜像存储在 /run/kata-containers/image/
  │      │
  │      ├─ "erofs.multi-layer"
  │      │    → MultiLayerErofsHandler
  │      │    → 处理多层 EROFS + ext4 upper + dm-verity
  │      │
  │      └─ "ephemeral" / "local"
  │           → EphemeralHandler / LocalHandler
  │           → 在 guest 侧创建 tmpfs 或 bind mount
  │
  ├─ [Guest VM] [2] 根据 OCI spec 处理容器启动
  │      - 创建 namespace (Guest VM 内核)
  │      - 加载 OCI spec 中的 mounts
  │      - [Container] pivot_root 到 rootfs
  │
  └─ [Container] [3] 容器进程启动，/proc/1/root = 最终 rootfs
```

### Agent 存储引用计数机制

Agent 维护了 `HashMap<String, StorageState>`（`sandbox.rs:130`）来跟踪所有存储设备：

```rust
pub struct StorageState {
    count: Arc<AtomicU32>,           // 引用计数
    device: Arc<dyn StorageDevice>,  // 已挂载的设备
    shared: bool,                     // 是否跨容器共享
}
```

- `add_sandbox_storage()`：创建新存储或递增引用计数
- `remove_sandbox_storage()`：递减引用计数，归零时卸载并清理
- `shared` 标记（用于 emptyDir 等跨容器共享卷）：在容器退出时不清理，仅在 sandbox 销毁时清理

---

## 六、FilesystemShare：跨 VM 文件共享的核心 **[Host 侧实现]**

### 接口定义

`fs_share.go:29-86` 定义了 `FilesystemSharer` 接口：

```go
type FilesystemSharer interface {
    Prepare(context.Context) error
    Cleanup(context.Context) error
    ShareFile(ctx, c, m) (*SharedFile, error)
    UnshareFile(ctx, c, m) error
    ShareRootFilesystem(ctx, c) (*SharedFile, error)   // ★ 核心
    UnshareRootFilesystem(ctx, c) error
    StartFileEventWatcher(ctx) error
    StopFileEventWatcher(ctx)
}
```

### SharedFile 返回值

```go
// fs_share.go:23-27
type SharedFile struct {
    containerStorages []*grpc.Storage   // 需要 agent 处理的 storage 列表
    volumeStorages    []*grpc.Storage   // virtual volume storages
    guestPath         string             // guest 内 rootfs 路径
}
```

**`containerStorages` 是否为 nil 是关键分水岭：**

- **nil**（virtio-fs 路径）：rootfs 已通过 virtio-fs 共享可见，agent 不需要额外处理
- **非 nil**（块设备/nydus/EROFS 路径）：agent 需要根据 storage 信息执行 mount 操作

### K8s ConfigMap/Secret 动态更新 **[Host fsnotify → agent.copyFile → Guest]**

`FilesystemShare` 还负责 K8s configmap/secret/projected-volume/downward-api 的**动态更新**：

- `ShareFile` 中，对这些特殊 volume 添加 `fsnotify` 监听
- `StartFileEventWatcher` 启动事件循环，检测文件变更后通过 `agent.copyFile()` 同步到 guest
- 利用 K8s 的原子写入协议（`..data` 符号链接 + 时间戳目录）

---

## 七、与 containerd 的关系 **[Host 侧]**: Kata 不走 CRI，但共享 snapshotter

### Kata 不替代 snapshotter

Kata Containers **不是** containerd snapshotter。containerd 的 snapshotter 仍然负责：
- 管理镜像层（unpack → Prepare → Commit，以 ChainID 为 key）
- 为每个容器准备可写快照（Prepare containerID with parent=ChainID）
- 返回 `[]mount.Mount` 描述

### Kata 是 mount 描述的消费者

Kata runtime 接收 containerd 传过来的 `[]mount.Mount`（Rootfs），然后**选择不按 runc 的方式挂载**，而是：
1. 将 rootfs 通过 virtio-fs 共享 / 块设备直通 / nydus 等方式传递到 guest
2. 告诉 agent：这是 rootfs 的存储信息，你在 guest 里挂载

### CRI vs native API

与 containerd 文档中讨论的类似：
- `ctr` 直连 containerd 原生 API
- kubelet 通过 CRI → CRI plugin (in-process) → `containerd.WithNewSnapshot` (client 库)
- **无论是 ctr 还是 CRI，最终到达 containerd-shim-kata-v2 的都是同一份 `CreateTaskRequest`**
- parent 的计算（ChainID）仍然在 containerd client 库中完成，kubelet 不参与

---

## 八、runtime-rs (Rust) 的差异

### 架构差异

`src/runtime-rs/crates/resource/src/rootfs/` 提供了 Rust 版本的 rootfs 抽象：

```rust
// rootfs/mod.rs:36-42
#[async_trait]
pub trait Rootfs: Send + Sync {
    async fn get_guest_rootfs_path(&self) -> Result<String>;
    async fn get_rootfs_mount(&self) -> Result<Vec<oci::Mount>>;
    async fn get_storage(&self) -> Option<Vec<Storage>>;
    async fn cleanup(&self, device_manager: &RwLock<DeviceManager>) -> Result<()>;
    async fn get_device_id(&self) -> Result<Option<String>>;
}
```

### Rootfs 类型（Rust 侧）

| 实现 | 文件 | 对应场景 |
|---|---|---|
| `ShareFsRootfs` | `share_fs_rootfs.rs` | virtio-fs 共享路径 |
| `BlockRootfs` | `block_rootfs.rs` | 块设备直通 |
| `NydusRootfs` | `nydus_rootfs.rs` | Nydus 镜像加速 |
| `ErofsMultiLayerRootfs` | `erofs_rootfs.rs` | EROFS 多层镜像 |
| `VirtualVolume` | `virtual_volume.rs` | 机密容器 Guest Pull |

### ShareFs 与 NydusShareFs

Rust runtime 区分两种共享 fs（`share_fs/mod.rs`）：

```rust
// 普通 virtio-fs（对应于 Go 的 virtiofsd 方式）
pub trait ShareFs: Send + Sync { ... }

// Nydus 专用 virtio-fs（对应于 Go 的 nydusd 方式）
pub trait NydusShareFs: Send + Sync { ... }
```

具体实现：
- `share_virtio_fs_standalone.rs` — 独立 virtiofsd
- `share_virtio_fs_inline.rs` — 内联模式（virtiofsd 作为 hypervisor 的 vhost-user 设备）
- `share_virtio_fs_nydus.rs` — nydusd 提供的 virtio-fs

### 路由逻辑

`rootfs/mod.rs:67-182` 的 `handler_rootfs`：

```rust
match rootfs_mounts {
    [] => ShareFsRootfs,                    // 空 rootfs_mounts → virtio-fs 共享
    _ if is_erofs_multi_layer => ...,       // 多层 erofs
    _ if is_single_layer_rootfs => {
        if is_guest_pull_volume => VirtualVolume,
        if is_block_rootfs => BlockRootfs,
        if is_nydus_type => NydusRootfs,
        else => ShareFsRootfs,              // 单层普通 → virtio-fs
    }
    _ => error
}
```

---

## 九、总结：Rootfs 路径决策树

```
[Host] CreateTaskRequest.Rootfs[0]
    │
    ├─ Type == "fuse.nydus-overlayfs"  ──→  [Host] 启动 nydusd，走 Nydus 路径
    │                                        [Host] nydusd API → Guest rafs mount
    │                                        [Guest VM] agent: overlay mount
    │                                        [Container] pivot_root → rafs + overlay rootfs
    │
    ├─ Options 包含 "extraoption="      ──→  [Host] 同 Nydus 路径
    │
    ├─ 包含 "io.katacontainers.volume=" ──→  [Host→Guest] Virtual Volume / Guest Pull
    │                                        [Guest VM] CDH 在 guest 内拉取镜像
    │
    ├─ Options 包含 "io.katacontainers.fs-opt.layer=" ──→ [Guest VM] FileSystem Layer
    │                                        [Host] 将 layer 信息传入 OCI spec
    │                                        [Guest VM] agent OverlayfsHandler mount
    │
    ├─ Type == "erofs" / EROFS markers  ──→  [Host→Guest] 块设备直通 EROFS 镜像
    │                                        [Host] EROFS layer blobs → virtio-blk
    │                                        [Guest VM] agent: erofs mount + ext4 mount + overlay
    │
    ├─ Source 是块设备 (/dev/...)       ──→  [Host→Guest] hotplug 块设备到 VM
    │   且 DisableBlockDeviceUse=false        [Host] QEMU/CLH 热插拔块设备
    │                                        [Guest VM] agent: mount /dev/vdX → rootfs
    │
    └─ 其他（普通 overlay 等）          ──→  [Host→Guest] virtio-fs 共享路径
        [Host] 若未挂载，先在 host mount 到 bundle/rootfs           │
        [Host] 然后 bindMountContainerRootfs 到共享目录              │
        [Guest VM] guest 通过 virtio-fs 自然可见                     │
        [Guest VM] agent 端不需要额外 mount 操作                     │
        [Container] pivot_root → rootfs
```

### 一句话总结

> Kata Containers 不是 containerd snapshotter 的替代者，而是 containerd mount 描述的**跨 VM 传递者**。它通过 virtio-fs（默认）、块设备直通、Nydus lazy-loading、EROFS 块级透传等多种方式，将 host 上 snapshotter 准备好的 rootfs 传递到 guest VM 内，然后由 kata-agent 执行最终的 mount 操作，使容器进程获得完整的根文件系统。

---

## 十、从磁盘/文件角度看 Rootfs 共享

> 本章完全抛开代码，只从磁盘上实际有什么文件、目录、挂载点的角度，解释各条技术路径下 rootfs 是如何从 Host "传递"到 Guest 容器的。如果你想验证这些内容，可以在 Host 和 Guest VM 内分别用 `ls`、`find`、`mount`、`losetup` 等命令观察。

### 核心问题

> Host 上 containerd snapshotter 准备好的 rootfs，究竟变成了 Guest 容器里的哪个目录、哪个挂载点？

答案取决于用了哪条技术路径。下面逐个看。

### 10.1 virtio-fs 路径（默认）

这是最直观的路径。原理一句话：

> Host 侧 bind mount 到共享目录下 → virtio-fs 透传给 Guest → Guest 内核看到共享目录的内容自动出现 → 容器直接使用。

#### 阶段 1：Sandbox 创建后 **[Host]**

在 Host 上出现了以下目录结构：

```
[Host] $ find /run/kata-containers/shared/sandboxes/<sandbox_id>/
/run/kata-containers/shared/sandboxes/<sandbox_id>/
├── shared/          ← 这是 virtio-fs 共享给 guest 的根 (virtiofsd 的 --shared-dir)
└── mounts/          ← host 侧 mount 点
```

此时两个目录都是空的。关键操作已经完成：

```
[Host] $ cat /proc/self/mountinfo | grep sandbox
# mounts/ 被 recursive slave bind mount 到了 shared/：
... <mounts_path> ... <shared_path> ... rslave ...
```

`slave` 传播类型保证了：今后在 `mounts/` 下创建的任何新挂载，都会自动出现在 `shared/` 下。但反过来不行（Guest 里的 mount 不会传播回 Host）。

Guest VM 启动后，virtio-fs 设备被 QEMU/CLH 配置为以 Host 的 `shared/` 作为后端：

```
[Guest VM] $ mount | grep virtio
virtiofs on /run/kata-containers/shared type virtiofs (ro,...)
```

#### 阶段 2：容器创建后 **[Host → Guest]**

containerd 已经将 overlay rootfs 挂到了 `bundle/rootfs`（或直接使用块设备上的文件系统）。Kata shim 执行：

```
[Host] $ mount | grep <container_id>
# bind mount rootfs 到共享目录的 mounts/ 下：
/run/kata-containers/shared/sandboxes/<sid>/mounts/<cid>-<random>-rootfs
    ← bind mount from <bundle>/rootfs (或 containerd snapshotter 的 overlay mount)
```

由于 `mounts/` → `shared/` 的 slave 传播，这个新的 bind mount 立即出现在 `shared/` 下：

```
[Host] $ ls /run/kata-containers/shared/sandboxes/<sid>/shared/containers/<cid>-<random>-rootfs/
bin  boot  dev  etc  home  lib  ...   ← 容器的完整 rootfs 文件树
```

在 Guest VM 内，virtio-fs 使这片内容自然可见：

```
[Guest VM] $ ls /run/kata-containers/shared/containers/<cid>-<random>-rootfs/
bin  boot  dev  etc  home  lib  ...   ← 与 Host 侧完全一致 (virtio-fs 透传)
```

**关键**：这一步不需要 agent 做任何 mount 操作。Host 上的 bind mount 通过 virtio-fs 自动"出现"在 Guest 中。

#### 阶段 3：容器进程启动后 **[Guest VM → Container]**

kata-agent 将 Guest 内可见的 rootfs 路径写入 OCI spec：

```
OCI spec: root.path = "/run/kata-containers/shared/containers/<cid>-<random>-rootfs"
```

rustjail 创建容器的 mount namespace，执行 `pivot_root`，容器内：

```
[Container] $ ls /
bin  boot  dev  etc  home  lib  ...   ← 这就是 Host 上 containerd snapshotter 准备的 rootfs
[Container] $ mount | grep " / "
/dev/vda1 on / type ext4 (ro)         ← Guest 的根设备
# 容器的 rootfs 是 pivot_root 切换而来，不直接出现在 mount 表中
```

**容器内 `/` 的文件系统类型**：不是 overlay，是 virtio-fs。因为 containerd 在 Host 上已经用 overlay 做好了 merge，virtio-fs 看到的是 merge 后的文件树。Guest 内核通过 virtio-fs 协议向 Host 请求文件，Host 侧的 overlay 内核模块透明地处理 lower/upper/work 的合并逻辑。容器进程完全不知道也不关心底层有几层镜像。

**从一个视角总结 virtio-fs 路径**：Host 文件系统上的一次 bind mount → 通过 virtio-fs 协议传输 → Guest 内核 VFS 层呈现 → 容器 pivot_root 使用。整个过程没有发生"文件复制"，也没有 Guest 侧的额外 mount 操作。

### 10.2 块设备路径（devmapper / raw block）

当 containerd 的 snapshotter 使用 devmapper 等块设备后端时，rootfs 是一个块设备而非 overlay 文件。

#### 阶段 1：容器镜像准备 **[Host]**

containerd devmapper snapshotter 创建了一个 thin device 作为容器 rootfs：

```
[Host] $ ls -la /dev/mapper/
thin_<snapshot_id> -> ../dm-<N>

[Host] $ sudo blkid /dev/mapper/thin_<snapshot_id>
/dev/mapper/thin_<snapshot_id>: UUID="..." TYPE="ext4"
```

containerd TaskService.Create 发给 shim 的 Rootfs：
- `Source = "/dev/mapper/thin_<snapshot_id>"`
- `Type = "ext4"`
- `Mounted = false` ← 没有被 mount 到 bundle/rootfs

#### 阶段 2：Hotplug 到 Guest **[Host → Guest VM]**

Kata shim 检测到 rootfs 源是块设备，不执行 `doMount`，而是通过 QEMU/CLH 将该块设备 hotplug 到 VM：

```
[Host] $ ls /sys/block/  # 确认设备存在
dm-<N>

# shim 通过 QMP/API 将 dm-<N> 作为 virtio-blk 设备加入 VM
# Guest 内核检测到新设备：

[Guest VM] $ dmesg | tail
[  ...] virtio_blk virtio1: [vdb] 2097152 512-byte logical blocks (1.07 GB/1.00 GiB)

[Guest VM] $ ls -la /dev/vdb
brw------- 1 root root 254, 16 ... /dev/vdb
```

#### 阶段 3：Agent 挂载到容器 **[Guest VM]**

kata-agent 收到 `driver: "blk"`, `source: "<PCI path>"`, `mount_point: "/run/kata-containers/shared/containers/<cid>"`, `fstype: "ext4"`：

```
[Guest VM] $ mount | grep vdb
/dev/vdb on /run/kata-containers/shared/containers/<cid> type ext4 (rw, ...)
[Guest VM] $ ls /run/kata-containers/shared/containers/<cid>/
bin  boot  dev  etc  home  ...
```

然后 pivoted 到容器内：

```
[Container] $ ls /
bin  boot  dev  etc  home  ...
```

**容器内 `/` 的文件系统类型**：块设备本身的文件系统（如 ext4 / xfs），不是 overlay。因为 rootfs 就是块设备上的完整文件系统，不需要再叠加写层。

**核心差异**：块设备路径不走 virtio-fs 文件系统协议。块设备直接在 Guest 内核中显示为 `/dev/vdX`，然后 agent 在 Guest 内执行 `mount(2)` 将其挂载。Host 和 Guest 之间传输的是块 I/O，不是文件级操作。

### 10.3 Nydus 路径（镜像加速）

#### 阶段 1：Nydus 镜像准备 **[Host]**

nydus snapshotter 在 Host 上准备了以下文件：

```
[Host] $ tree /var/lib/containerd/io.containerd.snapshotter.v1.nydus/
snapshots/
├── 1/
│   ├── fs/        ← nydus 镜像的 bootstrap (metadata 文件)
│   └── mnt/       ← rafs 挂载点 (空的，供 nydusd 使用)
├── 2/
│   ├── fs/        ← containerd 分配的 upperdir (容器可写层)
│   └── work/      ← overlay workdir
...

$ ls /var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/1/fs/
image.boot   ← nydus bootstrap 文件 (metadata + 文件索引)
```

nydus snapshotter 输出给 containerd 的 mount 看起来像：

```
Type:   fuse.nydus-overlayfs
Source: overlay
Options:
  lowerdir=/var/lib/.../nydus/snapshots/1/mnt     ← rafs mount (nydusd 管理)
  upperdir=/var/lib/.../nydus/snapshots/2/fs      ← 可写层
  workdir=/var/lib/.../nydus/snapshots/2/work
  extraoption=base64({"source":".../image.boot","config":"{...}","snapshotdir":".../snapshots/2"})
```

#### 阶段 2：Sandbox 创建，nydusd 启动 **[Host]**

```
[Host] $ ps aux | grep nydusd
nydusd virtiofs --sock /run/vc/vm/<sid>/vhost-user-fs.sock \
        --apisock /run/vc/vm/<sid>/api.sock
```

nydusd 替代 virtiofsd，提供两层功能：
- `passthrough_fs`：透传 Host 共享目录（与 virtiofsd 功能相同）
- `rafs`：按需加载的镜像文件系统

#### 阶段 3：容器创建 **[Host → Guest VM]**

Kata shim 调用 nydusd API 挂载 rafs：

```
[Host] $ curl --unix-socket /run/vc/vm/<sid>/api.sock \
        -X POST "http://localhost/api/v1/mount?mountpoint=/rafs/<cid>/lowerdir" \
        -d '{"source":"/var/lib/.../nydus/snapshots/1/fs/image.boot",
             "fs_type":"rafs",
             "config":"{...}"}'
```

此时在 Guest VM 内出现了 rafs 挂载（通过 virtio-fs 协议传输到 Guest 内核）：

```
[Guest VM] $ mount | grep rafs
rafs on /run/kata-containers/shared/rafs/<cid>/lowerdir type fuse.rafs (ro,...)

[Guest VM] $ ls /run/kata-containers/shared/rafs/<cid>/lowerdir/
bin  boot  etc  usr  ...   ← nydus 按需呈现的镜像内容
```

同时，Host 上 snapshotter 分配的 `snapshotdir`（upperdir/workdir）通过 `passthrough_fs` 出现在 Guest 中：

```
[Guest VM] $ ls /run/kata-containers/shared/containers/<cid>/
snapshotdir/
    fs/     ← overlay upperdir (可写)
    work/   ← overlay workdir
```

#### 阶段 4：Overlay 组装 **[Guest VM → Container]**

Agent 用 `OverlayfsHandler` 在 Guest 内执行 overlay mount：

```
[Guest VM] $ mount -t overlay overlay \
    -o lowerdir=/run/kata-containers/shared/rafs/<cid>/lowerdir \
    -o upperdir=/run/kata-containers/shared/containers/<cid>/snapshotdir/fs \
    -o workdir=/run/kata-containers/shared/containers/<cid>/snapshotdir/work \
    /run/kata-containers/shared/containers/<cid>/rootfs
```

最终容器 rootfs = **overlay(rafs 只读层 + host 可写层)**，读操作按需从 registry 或本地缓存拉取数据。

```
[Container] $ ls /
bin  boot  etc  home  ...
[Container] $ mount | grep overlay
overlay on / type overlay (rw, lowerdir=..., upperdir=..., workdir=...)
```

**容器内 `/` 的文件系统类型**：overlay（Guest 内 agent 组装）。`lowerdir=rafs, upperdir=snapshotdir/fs, workdir=snapshotdir/work`。

**Nydus 的"懒加载"体现在哪里？** 当 Guest 内核通过 rafs 读取文件时，nydusd 只读取文件实际用到的数据块。整个镜像的 bootstrap（metadata）可能只有几十 MB，即使完整镜像有几个 GB。未读取的数据块不会从 registry/缓存传输到 Guest。

### 10.4 EROFS 路径（块设备级透传）

EROFS 路径完全不使用 virtio-fs。所有数据传输通过 virtio-blk 块设备完成。

#### 阶段 1：镜像转换为 EROFS **[Host]**

containerd EROFS snapshotter 将每层镜像转换为 `.erofs` 文件：

```
[Host] $ ls /var/lib/containerd/io.containerd.snapshotter.v1.erofs/snapshots/
1/
├── layer.erofs     ← 基础层的 EROFS 只读镜像 (~200MB)
2/
├── layer.erofs     ← 第二层 EROFS 只读镜像 (~50MB)
3/
├── rwlayer.img     ← 可写层 ext4 镜像 (~6GB, sparse)
4/
├── fsmeta.erofs    ← 合并后的 metadata (fuse-merger 生成)
```

此外还可能有一个 VMDK descriptor 文件描述多层合并：

```
[Host] $ cat /var/lib/containerd/.../multi-layer.vmdk
# Disk DescriptorFile
version=1
RW 409600 FLAT "/var/lib/.../snapshots/4/fsmeta.erofs" 0
RW 102400 FLAT "/var/lib/.../snapshots/1/layer.erofs" 0
RW 204800 FLAT "/var/lib/.../snapshots/2/layer.erofs" 0
```

#### 阶段 2：块设备直通 **[Host → Guest VM]**

Kata shim 检测到 EROFS rootfs，将 `.erofs` 和 `.img` 文件作为 virtio-blk 设备 hotplug 到 VM：

```
[Host] $ file /var/lib/.../rwlayer.img
rwlayer.img: Linux rev 1.0 ext4 filesystem data ...

[Host 操作] QEMU -drive file=/var/lib/.../rwlayer.img,format=raw,if=virtio
[Host 操作] QEMU -drive file=/var/lib/.../multi-layer.vmdk,format=vmdk,if=virtio

[Guest VM] $ lsblk
NAME   MAJ:MIN RM   SIZE RO TYPE MOUNTPOINTS
vda    254:0    0   256M  0 disk
└─vda1 254:1    0   253M  0 part /
vdb    254:16   0     6G  0 disk    ← rwlayer.img (ext4)
vdc    254:32   0 759.7M  0 disk    ← 合并后的 EROFS 层 (VMDK)
```

#### 阶段 3：Agent 在 Guest 内组装 overlay **[Guest VM]**

Agent 的 EROFS handler 执行以下 mount 顺序：

```
[Guest VM] $ mount -t ext4 /dev/vdb /run/kata-containers/virtual-volumes/<b64-encoded-name>
[Guest VM] $ mount -t erofs /dev/vdc /run/kata-containers/virtual-volumes/<b64-encoded-name2>

[Guest VM] $ mount -t overlay overlay \
    -o lowerdir=/run/kata-containers/virtual-volumes/<erofs_mount> \
    -o upperdir=/run/kata-containers/virtual-volumes/<ext4_mount>/upper \
    -o workdir=/run/kata-containers/virtual-volumes/<ext4_mount>/work \
    /run/kata-containers/<cid>/rootfs
```

#### 阶段 4：容器使用 **[Container]**

```
[Container] $ ls /
bin  boot  etc  home  ...
[Container] $ mount | grep overlay
overlay on / type overlay (rw, lowerdir=..., upperdir=..., workdir=...)
```

**容器内 `/` 的文件系统类型**：overlay（Guest 内 agent 组装）。`lowerdir=erofs_mount, upperdir=ext4_mount/upper, workdir=ext4_mount/work`。

**EROFS 路径的优势**：所有数据传输是块级别的（virtio-blk），没有 FUSE 协议开销。只读的 EROFS 层可以压缩（节省内存），且 EROFS 是 Linux 内核原生支持的文件系统（不需要用户态守护进程）。

### 10.5 Guest Pull 路径（机密容器）

Guest Pull 的独特之处在于：**Host 上没有镜像层**。镜像在 Guest VM 内直接从 registry 拉取。

#### 阶段 1：Host 侧只有 Metadata **[Host]**

```
[Host] $ find /run/kata-containers/shared/sandboxes/<sid>/ -name "rootfs"
(空 — Host 上没有容器的 rootfs 文件)

[Host] $ ls /var/lib/containerd/io.containerd.content.v1.content/blobs/sha256/
# containerd 可能有 image manifest 和 config，但没有拉取 layer blob
```

nydus snapshotter (proxy 模式) 不实际拉取镜像层，只传递 metadata。

#### 阶段 2：Guest 内拉取镜像 **[Guest VM]**

kata-agent 的 `ImagePullHandler` 调用 CDH (Confidential Data Hub)：

```
[Guest VM] $ ls /run/kata-containers/image/
layers/
    sha256:xxx/   ← 拉取的镜像层 (解压后的 tar 内容)
    sha256:yyy/
    ...
overlay/
    <cid>/
        upper/    ← overlay upperdir
        work/     ← overlay workdir
```

pause 镜像特殊处理——如果容器类型是 sandbox/pod，不会从 registry 拉取，而是使用 Guest rootfs 内预装的 pause 镜像：

```
[Guest VM] $ ls /pause/
pause   ← 预装在 guest rootfs 中的静态 pause 二进制
```

#### 阶段 3：Overlay 组装 **[Guest VM → Container]**

与 Nydus 路径类似，agent 在 Guest 内组装 overlay：

```
[Guest VM] $ mount -t overlay overlay \
    -o lowerdir=/run/kata-containers/image/layers/sha256:xxx:/run/kata-containers/image/layers/sha256:yyy \
    -o upperdir=/run/kata-containers/image/overlay/<cid>/upper \
    -o workdir=/run/kata-containers/image/overlay/<cid>/work \
    /run/kata-containers/<cid>/rootfs
```

```
[Container] $ ls /
bin  boot  etc  home  ...   ← 完全在 Guest 内拉取和组装的 rootfs
```

**容器内 `/` 的文件系统类型**：overlay（Guest 内 agent 组装）。`lowerdir` 是 `/run/kata-containers/image/layers/` 下各层，`upperdir/workdir` 在 `/run/kata-containers/image/overlay/<cid>/` 下。

**Guest Pull 的安全意义**：Host 上不存在镜像层的内容（明文）。Host 只能看到加密的块设备（如果配置了加密存储）。Guest VM 在 TCB (Trusted Computing Base) 内部完成镜像拉取和 rootfs 组装，防止 Host 篡改容器镜像。

#### 大镜像的加密存储

当镜像体积超过 `/run` tmpfs 容量时，可以使用 Host 提供的加密块设备：

```
[Guest VM] $ lsblk --fs
sda
└─encrypted_disk_GsLDt  ← LUKS2 加密，Host 无法读取
    mount: /run/kata-containers/image  (ext4)

[Guest VM] $ du -h --max-depth=1 /run/kata-containers/image/
2.1G    /run/kata-containers/image/layers/
60K     /run/kata-containers/image/overlay/
```

---

### 十条路径对照总图

```
┌────────────────────────────────────────────────────────────────────────┐
│                               HOST                                     │
│                                                                         │
│  ┌─ virtio-fs ─────────────────────────────────────────────┐           │
│  │ [Host fs] mounts/<cid>/rootfs  ←bind── [Host fs] bundle/rootfs      │
│  │    ↓ slave propagation                                             │
│  │ [Host fs] shared/<cid>/rootfs  ──virtio-fs──→ [Guest] 直接可见     │
│  └────────────────────────────────────────────────────────┘           │
│                                                                         │
│  ┌─ block ───────────────────────────────────────────────────┐         │
│  │ [Host blk] /dev/mapper/thin_xxx ──virtio-blk──→ [Guest] /dev/vdb  │
│  └──────────────────────────────────────────────────────────┘         │
│                                                                         │
│  ┌─ Nydus ───────────────────────────────────────────────────┐         │
│  │ [Host fs] nydus/image.boot ──nydusd API──→ [Guest] /rafs/<cid>/lowerdir │
│  │ [Host fs] nydus/snapshotdir  ──passthrough──→ [Guest] /shared/... │
│  └──────────────────────────────────────────────────────────┘         │
│                                                                         │
│  ┌─ EROFS ───────────────────────────────────────────────────┐         │
│  │ [Host fs] *.erofs + *.vmdk ──virtio-blk──→ [Guest] /dev/vdc       │
│  │ [Host fs] rwlayer.img     ──virtio-blk──→ [Guest] /dev/vdb       │
│  └──────────────────────────────────────────────────────────┘         │
│                                                                         │
│  ┌─ Guest Pull ───────────────────────────────────────────────┐        │
│  │ [Host]    只有 image metadata (manifest, config)                   │
│  │ [Guest]   直接从 registry pull → /run/kata-containers/image/      │
│  └──────────────────────────────────────────────────────────┘         │
│                                                                         │
└────────────────────────────────────────────────────────────────────────┘
                                    │
          vsock / virtio-fs / virtio-blk / nydusd API
                                    │
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                             GUEST VM                                    │
│                                                                         │
│  virtio-fs:  rootfs 已在 /run/kata-containers/shared/... 可见          │
│              → agent 不做 mount，直接 pivot_root                       │
│                                                                         │
│  block:      agent mount /dev/vdb → <mountpoint>                       │
│              → pivot_root                                              │
│                                                                         │
│  Nydus:      agent mount overlay(lowerdir=rafs, upper=snapshotdir)     │
│              → pivot_root                                              │
│                                                                         │
│  EROFS:      agent mount erofs /dev/vdc, mount ext4 /dev/vdb           │
│              agent mount overlay(lowerdir=erofs, upper=ext4)           │
│              → pivot_root                                              │
│                                                                         │
│  Guest Pull: CDH pull image → unpack to /run/kata-containers/image/    │
│              agent mount overlay(lowerdir=layers, upper=overlay)       │
│              → pivot_root                                              │
│                                                                         │
│                                    │                                    │
│                               pivot_root                               │
│                                    │                                    │
│                                    ▼                                    │
│  ┌── Container ──────────────────────────────────────────────────┐     │
│  │  /                                                              │     │
│  │  ├── bin/  etc/  home/  lib/  ...                              │     │
│  │  ├── (对于 overlay: .overlay/upper/ 中的可写修改)                │     │
│  │  └── rootfs 来源取决于技术路径                                   │     │
│  └────────────────────────────────────────────────────────────────┘     │
└────────────────────────────────────────────────────────────────────────┘
```

---

## 附：关键 file:line 索引

### Go Runtime (src/runtime/)

| 主题 | 位置 |
|---|---|
| **shim 入口** | |
| CreateTask 处理 | `pkg/containerd-shim-v2/service.go` |
| RootFs 构建 & checkAndMount | `pkg/containerd-shim-v2/create.go:76-337` |
| copyLayersToMounts | `pkg/containerd-shim-v2/create.go:54-74` |
| CreateSandbox 调用 | `pkg/katautils/create.go:120-219` |
| CreateContainer 调用 | `pkg/katautils/create.go:250-299` |
| **RootFs 结构体** | |
| RootFs 定义 | `virtcontainers/container.go:324-336` |
| Container 结构（含 rootFs） | `virtcontainers/container.go:338-362` |
| **FilesystemSharer 接口** | |
| 接口定义 | `virtcontainers/fs_share.go:29-86` |
| **Linux 实现（核心）** | |
| FilesystemShare.Prepare | `virtcontainers/fs_share_linux.go:210-261` |
| FilesystemShare.ShareRootFilesystem | `virtcontainers/fs_share_linux.go:681-788` |
| virtio-fs 路径（bind mount） | `virtcontainers/fs_share_linux.go:780-787` |
| Nydus 路径 | `virtcontainers/fs_share_linux.go:486-538` |
| EROFS 路径 | `virtcontainers/fs_share_linux.go:584-660` |
| Virtual Volume 路径 | `virtcontainers/fs_share_linux.go:571-582` |
| Block 设备路径 | `virtcontainers/fs_share_linux.go:714-773` |
| Guest Pull (force) | `virtcontainers/fs_share_linux.go:662-678` |
| **Sandbox** | |
| CreateSandbox | `virtcontainers/sandbox.go:637+` |
| CreateContainer | `virtcontainers/sandbox.go:1679-1744` |
| fsShare 初始化 | `virtcontainers/sandbox.go:694-698` |
| IsGuestPullForced | `virtcontainers/sandbox.go:540-544` |
| **Container** | |
| newContainer | `virtcontainers/container.go:738` |
| c.create (hotplug) | `virtcontainers/container.go:1510-1553` |
| hotplugDrive | `virtcontainers/container.go:1905+` |
| **Agent 通信** | |
| createContainer (guest RPC) | `virtcontainers/kata_agent.go:1564+` |
| ShareRootFilesystem 调用点 | `virtcontainers/kata_agent.go:1583` |
| 路径常量定义 | `virtcontainers/kata_agent.go:113-287` |
| GetSharePath / getMountPath | `virtcontainers/kata_agent.go:229-243` |
| **Nydus** | |
| nydusd 启动 & API | `virtcontainers/nydusd.go:85-296` |
| extraOption 解析 | `virtcontainers/nydusd.go:478-512` |
| **Mount 相关** | |
| Mount 结构体 | `virtcontainers/mount.go:238-287` |
| bind mount 工具函数 | `virtcontainers/mount.go` |
| **路径常量** | |
| defaultKataGuestSharedDir | `virtcontainers/kata_agent.go:113` |
| kataGuestSharedDir 函数 | `virtcontainers/kata_agent.go:272-278` |

### Rust Runtime (src/runtime-rs/)

| 主题 | 位置 |
|---|---|
| **Rootfs trait** | `crates/resource/src/rootfs/mod.rs:36-42` |
| **handler_rootfs (路由)** | `crates/resource/src/rootfs/mod.rs:67-182` |
| **ShareFsRootfs** | `crates/resource/src/rootfs/share_fs_rootfs.rs` |
| **BlockRootfs** | `crates/resource/src/rootfs/block_rootfs.rs` |
| **NydusRootfs** | `crates/resource/src/rootfs/nydus_rootfs.rs` |
| **ErofsRootfs** | `crates/resource/src/rootfs/erofs_rootfs.rs` |
| **VirtualVolume** | `crates/resource/src/rootfs/virtual_volume.rs` |
| **ShareFs trait** | `crates/resource/src/share_fs/mod.rs` |
| **ShareFs 实现** | `crates/resource/src/share_fs/share_virtio_fs_standalone.rs` |
| **NydusShareFs 实现** | `crates/resource/src/share_fs/share_virtio_fs_nydus.rs` |
| **Container 创建** | `crates/runtimes/virt_container/src/container_manager/container.rs` |

### Agent (src/agent/)

| 主题 | 位置 |
|---|---|
| **STORAGE_HANDLERS 注册** | `src/storage/mod.rs:134-159` |
| **add_storages (入口)** | `src/storage/mod.rs:291` |
| **StorageHandler trait** | `src/storage/mod.rs:121-131` |
| **OverlayfsHandler** | `src/storage/fs_handler.rs:44+` |
| **VirtioBlkPciHandler** | `src/storage/block_handler.rs` |
| **VirtioFsHandler** | `src/storage/fs_handler.rs` |
| **ImagePullHandler** | `src/storage/image_pull_handler.rs` |
| **EphemeralHandler** | `src/storage/ephemeral_handler.rs` |
| **LocalHandler** | `src/storage/local_handler.rs` |
| **BindWatcherHandler** | `src/storage/bind_watcher_handler.rs` |
| **MultiLayerErofsHandler** | `src/storage/multi_layer_erofs.rs` |
| **RPC: CreateContainer** | `src/rpc.rs:1483` |
| **StorageState (引用计数)** | `src/sandbox.rs:64-110, 179-239` |

### 设计文档

| 主题 | 文档 |
|---|---|
| Guest Image Management | `docs/design/kata-guest-image-management-design.md` |
| Nydus 设计 | `docs/design/kata-nydus-design.md` |
| Guest Pull HOWTO | `docs/how-to/how-to-pull-images-in-guest-with-kata.md` |
| virtio-fs HOWTO | `docs/how-to/how-to-use-virtio-fs-with-kata.md` |
| virtio-fs + Nydus | `docs/how-to/how-to-use-virtio-fs-nydus-with-kata.md` |
| EROFS Snapshotter | `docs/how-to/how-to-use-erofs-snapshotter-with-kata.md` |
