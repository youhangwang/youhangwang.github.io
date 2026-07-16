---
title: 从零构建一个容器镜像
tags: container OCI podman
---

对于开发者而言，一个容器镜像（container image）本质上是运行容器所需的一组配置。但容器镜像到底是什么？你可能知道容器镜像是什么、知道它由若干层（layer）组成、知道它是一堆 tar 归档的集合。但仍有一些问题没有被回答：一层究竟由什么构成？这些层又是如何组合成一个完整文件系统或多平台镜像的？在本文中，我们将从零开始构建一个容器镜像，并尝试回答所有这些问题，从而理解容器镜像的内部原理。
<!--more-->

> 本文翻译自 SUSE Communities 博客 [Build a Container Image from Scratch](https://www.suse.com/c/build-a-container-image-from-scratch/)，作者 Danish Prakash，首发于 2025 年 8 月 11 日，最后更新于 2025 年 10 月 10 日。译文力求忠于原文，配图均来自原文。

## OCI 镜像

在继续之前，先简单回顾一点历史。大约十年前，docker 格式还是唯一被使用的镜像格式，而随着容器周边工具的不断涌现，标准化的需求变得迫切起来。大约在 2015 年，开放容器倡议（Open Container Initiative，OCI）成立——如今它是 Linux 基金会的一个项目——旨在对容器相关的一切进行标准化。他们提出了针对容器镜像的规范，称为 OCI 规范。现代工具在处理容器镜像时都遵循并符合这套运行时规范，因此在本文中，"OCI 镜像"与"容器镜像"将交替使用。

一个 OCI 镜像由四个核心组件构成——层（layer）、配置（config）、清单（manifest）和索引（index）：

![OCI 镜像的各个组件](../../../assets/images/posts/oci-image-components.png){:.rounded}

接下来我们会逐一理解上述每一个组件，并从零构建一个 "hello" 镜像，以便在实践中真正理解这些组件。如果为我们的 "hello" 镜像写一个 Containerfile，它会是这样的：

```dockerfile
FROM scratch

COPY ./hello ./

ENTRYPOINT ["./hello"]
```

这个镜像使用了 `scratch`，一个空的基础镜像。然后我们把 hello 二进制文件复制到镜像中，并将其设为容器的入口点（entrypoint）。让我们开始吧。

## 1. layer —— 镜像里装了什么

层代表了一个容器镜像里所包含的内容。人们常常把它称作容器镜像的基本构建块。它们由这些成分构成：你复制进镜像的源代码、容器文件系统，或者几乎所有你添加到容器镜像中的东西。

从技术上讲，一个容器镜像就是一个文件系统变更集（filesystem changeset）。一个变更集包含了两个文件系统之间的差异（diff），并以 tar 归档的形式序列化。考虑下面这个容器镜像：

```dockerfile
FROM Alpine

RUN rm -f /bin/ash \
    && apk add bash
```

我们用 alpine 作为基础镜像，删除了 `/bin/ash` 并安装了 bash。我们就用它作为示例，来构建变更集，然后再组装出容器的最终文件系统。

### 1.1 layer：创建一个变更集

为了创建一个层——或者更准确地说，一个文件系统变更集——我们从一个最小化的根文件系统开始；在我们的例子里用的是 alpine，因为我们的示例 Containerfile 就是以 alpine 基础镜像开头的：

```bash
$ wget https://dl-cdn.alpinelinux.org/alpine/v3.18/releases/x86_64/alpine-minirootfs-3.18.4-x86_64.tar.gz && tar -xvf ${_##*/}
$ tree
.
├── alpine-minirootfs-3.18.4-x86_64.tar.gz
├── bin
│   ├── arch
│   ├── ash
│   ├── base64
│   ├── bbconfig
│   ├── busybox
│   ├── cat
│   ├── chattr
...
```

为了创建后续的层，我们对基础文件系统创建一份副本/快照——使用 `cp -rp`——然后通过添加、删除或修改文件或目录，对这份快照进行改动。

一个变更集只包含我们添加、修改和删除的文件。为了创建变更集，我们递归地比较两个文件系统（上一步得到的快照）。然后我们创建一个只包含这个变更集的 tar 归档。

鉴于在我们的示例镜像里我们删除了 `/bin/ash` 并添加了 `/bin/bash`，我们的变更集/层会是这样：

```
./bin/bash
./bin/.wh.ash
```

注意这里的 `.wh`，它表示 whiteout（遮白），用来标记被删除的文件。上面的变更集告诉容器引擎：在上一层/变更集（我们的例子里是 alpine 基础层）之上，我们添加/修改了 bash，并删除了 ash。这就构成了一个变更集，也就是一个层。

### 1.2 layer：创建文件系统

现在我们创建好了一个层。容器引擎会取出多个层，并为最终得到的容器创建一个完整的文件系统。

![创建文件系统](../../../assets/images/posts/creating-the-filesystem.png){:.rounded}

假设我们有 3 个层：一个没有文件系统的空层——也就是起始的变更集；然后我们创建一个变更集，添加两个二进制文件 `/bin/ash` 和 `/bin/bash`；最后一层删除 `/bin/ash` 并修改 `/bin/bash`。

容器引擎会把一个镜像中的所有层从左到右依次叠加，生成最终的文件系统。在这个例子里，把层 1 叠加到层 0 之上、再把层 2 叠加到层 1 之上之后，最终文件系统里只会剩下 `/bin/bash`。这就是容器引擎如何从容器镜像中一组层创建出文件系统的方式。

### 1.3 layer：生命周期

到目前为止，我们已经理解了：

- 一个镜像如何由一个或多个层构成。
- Containerfile 如何创建层。
- 容器引擎如何利用所有这些层，为容器创建最终文件系统。

当你在日常工作流中使用容器时，这一切是如何串起来的呢？考虑一个容器镜像的生命周期：从 Containerfile，到容器镜像，最后到一个运行中的容器：

![层的生命周期](../../../assets/images/posts/layer-lifecycle.png){:.rounded}

你使用 `podman build` 并传入 Containerfile，podman 随后会创建出各个层，并把它们打包成一个"容器镜像"。当你对这个镜像执行 `podman run` 时，引擎会把镜像中的各个层合并起来，为正在运行的容器创建根文件系统。

### 1.4 layer —— 我们的 scratch 镜像层

让我们为 "hello" 镜像创建这一层。作为参考，如果它是一个 Containerfile，我们的镜像会是这样的：

```dockerfile
FROM scratch

COPY ./hello /root/

ENTRYPOINT ["./hello"]
```

这里我们的镜像包含 2 个层。第一层来自 scratch 镜像——它是一种特殊的"镜像"，告诉容器引擎：你的镜像从一个没有文件系统的层开始。正如我们在前几节看到的，这意味着从一个空的文件系统层起步，然后让后续的指令去创建层。Containerfile 里几乎每一条指令都会生成一个新的层。所以在上面的 Containerfile 中，`COPY` 指令创建了第二个层，它包含了相对于上一层文件系统的变更。这里的变更是"添加"一个新文件——hello 二进制——到已有的文件系统，也就是 alpine 根文件系统之上。

让我们为镜像创建这一层。我们先为简单的 "Hello world" 程序编译一个静态链接的 C 二进制：

```bash
$ cat hello.c
#include <stdio.h>

int main(int argc, char *argv[]) {
    if (argc < 2) {
        printf("Usage: %s <name>\n", argv[0]);
        return 1;
    }

    printf("Hello, %s!\n", argv[1]);

    return 0;
}

$ gcc -o hello hello.c -static
$ tar --remove-files -czvf layer.tar.gz hello

$ sha256sum layer.tar.gz
36c412b23a871c4afbec29a45b25faad76197f3a9dbf806f3aef779af926790a layer.tar.gz
$ mv layer.tar.gz 36c412b23a871c4afbec29a45b25faad76197f3a9dbf806f3aef779af926790a
```

然后我们把这个二进制做成 gzip 压缩的 tar 归档。这就构成了我们唯一的一层。

## 2. config —— 如何运行容器

config 描述的是如何运行容器。这个 JSON 文件保存了用来配置容器的各项配置选项，比如环境变量、容器入口点、数据卷（volumes）等。你可以在运行容器时通过命令行提供这些选项，也可以作为 Containerfile 的一部分，由其填充到 config.json 中。

考虑下面这段来自示例 config.json 的片段：

```bash
$ vim sample_config.json
{
    "architecture": "amd64",
    "os": "linux",
    "config": {
        "Entrypoint": [
            "./bin/bash"
        ],
        "User": "danish",
        "ExposedPorts": {
            "8080/tcp": {}
        },
        "Env": [
            "FOO=bar",
        ],
        "Volumes": {
            "/var/logs": {}
        }
    }
}
```

config 部分在文件中设置了各种配置选项，外加一些额外的元数据。你可以设置入口点、用户、端口、环境变量等。让我们为我们的镜像写一份配置：

```bash
$ vim config.json
{
    "architecture": "amd64",
    "os": "linux",
    "config": {
        "Entrypoint": [
            "time",
            "./hello"
        ]
    }
}
```

你会注意到它并不复杂。如果你回头看我们镜像的 Containerfile 等价形式，会发现我们把 "hello" 二进制设为了镜像的入口点，而这也是我们唯一需要设置的配置选项。

## 3. manifest —— 定位层与 config.json

容器引擎使用 manifest 来为镜像定位层和 config.json。考虑下面这段示例 manifest.json 的片段：

```json
{
    "schemaVersion": 2,
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "config": {
        "mediaType": "application/vnd.oci.image.config.v1+json",
        "digest": "sha256:<DIGEST>",
        "size": XYZ
    },
    "layers": [
        {
            "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "digest": "sha256:<DIGEST>",
            "size": XZY
        }
    ]
}
```

manifest 在上面这段里提到了 config 和 layer，并为每一项附上了额外的元数据。但 manifest 并不是通过路径来引用这些组件，而是通过一个摘要（digest）来寻址。这是因为层、config 和 manifest 合在一起，构成了我们所说的内容寻址存储（content-addressable store）。

### 3.1 内容寻址

为了兼顾效率与完整性，OCI 要求 OCI 镜像中的组件基于其内容来标识——也就是说，你可以依据数据的内容而不是其位置（文件路径等）来识别数据。在 OCI 镜像中实现这一点的方式是：我们用一个唯一标识符（通常是一个加密哈希）作为文件名。这就是我们所说的内容寻址。

![内容寻址存储](../../../assets/images/posts/content-addressable-store.png){:.rounded}

它有助于去重、层共享，从而降低内存与性能开销，并保证数据完整性。

在本文中，我们将使用 sha256 作为算法，为各个组件生成标识符。让我们把上一步创建的层归档变成内容寻址的：

```bash
$ sha256sum layer.tar.gz
c37c06cdec9d6a0f2a2d55deb5aa002b26b37b17c02c2eca908fc062af5f53eb layer.tar
$ mv layer.tar c37c06cdec9d6a0f2a2d55deb5aa002b26b37b17c02c2eca908fc062af5f53eb
```

#### 目录结构

OCI 还为 OCI 镜像定义了一套布局，容器引擎在解析之前要求镜像符合该格式。OCI 对它的定义如下：

```
$ tree
├── blobs/<alg>
│       ├── <content addressable config>
│       ├── <content addressable manifest>
│       └── <content addressable layer>
└── index.json
```

内容寻址的内容存放在 blobs 目录下的一个子目录里，该子目录标识了用来编码内容的算法。因为我们用 sha256 来计算各 blob 的摘要，所以我们创建 sha256 目录：

```bash
$ mkdir --parents blobs/sha256
$ mv c37c06cdec9d6a0f2a2d55deb5aa002b26b37b17c02c2eca908fc062af5f53eb $_

$ tree ../..
└── blobs/sha256
        └── c37c06cdec9d6a0f2a2d55deb5aa002b26b37b17c02c2eca908fc062af5f53eb
```

趁现在，我们也把 config 变成内容寻址的，并为我们的 "hello" 镜像创建 manifest.json 文件：

```bash
$ sha256 config.json
99c9d2dcbdbc6e28277d379c2b9a59443b91937720361773963d28d5376252a9  config.json
$ mv config.json 99c9d2dcbdbc6e28277d379c2b9a59443b91937720361773963d28d5376252a9

$ tree ../..
└── blobs/sha256
        ├── c37c06cdec9d6a0f2a2d55deb5aa002b26b37b17c02c2eca908fc062af5f53eb
        └── 99c9d2dcbdbc6e28277d379c2b9a59443b91937720361773963d28d5376252a9

$ vim manifest.json
{
    "schemaVersion": 2,
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "config": {
        "mediaType": "application/vnd.oci.image.config.v1+json",
        "digest": "sha256:99c9d2dcbdbc6e28277d379c2b9a59443b91937720361773963d28d5376252a9",
        "size": 149
    },
    "layers": [
        {
            "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "digest": "sha256:36c412b23a871c4afbec29a45b25faad76197f3a9dbf806f3aef779af926790a",
            "size": 1372604
        }
    ]
}
```

从上面的操作可以看到，我们把 config.json 变成内容寻址的，然后看了一眼当前的镜像布局——它由 blobs/sha256 目录构成，里面放着我们的层和 config。接着我们创建 manifest 文件，通过摘要引用 config 和层，并附上诸如 size 和 label 这样的额外元数据。

现在 manifest 也准备好了，让我们把它也变成内容寻址的：

```bash
$ sha256 manifest.json
2e17c995558ebfa8faacfe64ff78c359ab9f28b3401076bb238fd28c5b3a648b  manifest.json
$ mv manifest.json 2e17c995558ebfa8faacfe64ff78c359ab9f28b3401076bb238fd28c5b3a648b

$ tree .
└── blobs/sha256
        ├── c37c06cdec9d6a0f2a2d55deb5aa002b26b37b17c02c2eca908fc062af5f53eb
        ├── 99c9d2dcbdbc6e28277d379c2b9a59443b91937720361773963d28d5376252a9
        └── 2e17c995558ebfa8faacfe64ff78c359ab9f28b3401076bb238fd28c5b3a648b
```

到目前为止，我们这个未打包的 OCI "hello" 镜像拥有：一个包含静态链接 hello 二进制的层归档、容器的配置，以及镜像的 manifest——全部以各自的 sha256 摘要来编码命名。

## 4. index —— manifest 的 manifest

index.json 文件充当一组镜像的索引，这些镜像可以跨越不同的架构和操作系统。

![index 结构](../../../assets/images/posts/index-structure.png){:.rounded}

一个例子是：一个 OCI 镜像里同时包含两种不同架构的镜像，一个 `linux/arm64` 变体和一个 `linux/amd64` 变体。在这种情况下，index.json 会包含两个条目，通过各自的 manifest 指向这两个镜像变体，而那些 manifest 又会包含如上图所示的、与具体镜像相关的元数据。

带着这些认识，我们来为 "hello" 镜像创建 index.json。我们没有多架构，所以我们的 index.json 只通过摘要指向镜像中唯一的那个 manifest.json：

```bash
$ cd ../..
$ vim index.json
{
    "schemaVersion": 2,
    "manifests": [
        {
            "mediaType": "application/vnd.oci.image.manifest.v1+json",
            "digest": "sha256:2e17c995558ebfa8faacfe64ff78c359ab9f28b3401076bb238fd28c5b3a648b",
            "size": 748,
            "annotations": {
                "org.opencontainers.image.ref.name": "hello:scratch"
            }
        }
    ]
}
```

此外，我们加了一个注解（annotation），表示该 manifest 所引用镜像的名称和标签。index.json 不需要被编码命名，所以它放在镜像的顶层目录，而不是 blobs/ 下——如果你还记得开头讨论的 OCI 镜像整体架构的话。所有组件都已完成，让我们快速回顾一下：

```
$ tree
├── blobs/sha256
│       ├── c37c06cdec9d6a0f2a2d55deb5aa002b26b37b17c02c2eca908fc062af5f53eb
│       ├── 99c9d2dcbdbc6e28277d379c2b9a59443b91937720361773963d28d5376252a9
│       └── 2e17c995558ebfa8faacfe64ff78c359ab9f28b3401076bb238fd28c5b3a648b
└── index.json
```

（顺序如上所示）

- `c37c06cdec9d6a0f2a2d55deb5aa002b26b37b17c02c2eca908fc062af5f53eb` —— 镜像的层，包含 hello 二进制。
- `99c9d2dcbdbc6e28277d379c2b9a59443b91937720361773963d28d5376252a9` —— config.json，告诉容器引擎把 hello 二进制设为由该镜像运行容器的入口点。
- `2e17c995558ebfa8faacfe64ff78c359ab9f28b3401076bb238fd28c5b3a648b` —— manifest.json，为容器引擎定位 config 和层。
- `index.json` —— 一个更高层的 manifest，引用多个镜像 manifest，从而支持容器镜像跨不同平台或架构分发。

## 打包

镜像的所有部件都已就位。让我们把目前创建的这些组件打个归档，并测试我们的镜像：

```bash
$ tree
├── blobs/sha256
│       ├── c37c06cdec9d6a0f2a2d55deb5aa002b26b37b17c02c2eca908fc062af5f53eb
│       ├── 99c9d2dcbdbc6e28277d379c2b9a59443b91937720361773963d28d5376252a9
│       └── 2e17c995558ebfa8faacfe64ff78c359ab9f28b3401076bb238fd28c5b3a648b
└── index.json
$ tar -cf hello.tar *

$ podman load < hello.tar
Getting image source signatures
Copying blob c37c06cdec9d done   |
Copying config cd12bca58e done   |
Writing manifest to image destination
Loaded image: localhost/hello:scratch

$ podman run localhost/hello:scratch world
Hello world!

$ podman image ls hello
REPOSITORY   TAG       IMAGE ID       CREATED   SIZE
hello        scratch    25e8b3bd9720  N/A       3.67MB
```

可以看到镜像被 podman 加载了，并且我们能够基于自己的镜像运行一个容器，正确地向 stdout 输出了 hello world!。我们清楚地知道镜像由什么构成、放进了哪些文件、设置了什么配置，以及镜像里包含了哪些元数据。

## 使用基础镜像

让我们创建一个基于 alpine 而不是 scratch 的镜像版本。这会给镜像引入额外一层，所以我们来看看如何处理多个层。这种情况下，Containerfile 如下：

```dockerfile
FROM alpine:latest

COPY ./hello /root/hello

ENTRYPOINT ["time", "./hello"]
```

注意我们把入口点改成了使用 `time` 工具。这用以演示镜像中两层是如何协同工作的。

到目前为止，镜像只有一个由 hello 二进制构成的层。如前所述，我们可以把一个层看作一次文件系统 diff。所以如果我们用 alpine 作为基础镜像，就需要相应的根文件系统，并在其之上构建第二层（也就是带二进制的那个层）。你可以从项目官网获取 alpine 的根文件系统。

```bash
$ cd blobs/sha256
$ wget https://dl-cdn.alpinelinux.org/alpine/v3.18/releases/x86_64/alpine-minirootfs-3.18.4-x86_64.tar.gz
$ mv alpine-minirootfs-3.18.4-x86_64.tar.gz \
    $(sha256sum alpine-minirootfs-3.18.4-x86_64.tar.gz | awk '{print $1}')
```

下载下来的 rootfs 已经是 gzip 压缩好的，所以我们要做的只是用它的 sha256 摘要来编码命名，就可以直接用了。

```
$ cd ../..
$ tree
├── blobs/sha256
│       ├── 99c9d2dcbdbc6e28277d379c2b9a59443b91937720361773963d28d5376252a9
│       ├── 8289bd1bdc2a1fae2d2d717b7c40baaedc4c5c4d9c9f4f1a1b045287067e9f2c
│       ├── c37c06cdec9d6a0f2a2d55deb5aa002b26b37b17c02c2eca908fc062af5f53eb
│       └── 2e17c995558ebfa8faacfe64ff78c359ab9f28b3401076bb238fd28c5b3a648b
└── index.json
```

除了之前的三个组件——hello 层、config 和 manifest——之外，我们现在多了一个 alpine rootfs 的层（8289bd1）。

接下来，我们需要用新的信息更新 config 和 manifest 文件。之后，还要确保用更新后的 sha256 摘要重新对它们编码命名。

```bash
$ vim config.json
{
    "architecture": "amd64",
    "os": "linux",
    "config": {
        "Entrypoint": [
            "time",
            "./hello"
        ]
    }
}

$ mv config.json $(sha256sum config.json | awk '{print $1}')
```

我们把入口点修改为在 hello 二进制前加上 `time`，以区分我们的 scratch 镜像和 alpine 基础镜像。config.json 不需要其他改动。

在 manifest.json 中，我们更新修改后的 config.json 的摘要，并添加 alpine 基础层的层信息。我们自上而下添加层，所以 alpine 基础层成为数组中的第一个条目，其后跟着 hello 层。

```bash
$ vim manifest.json
{
    "schemaVersion": 2,
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "config": {
        "mediaType": "application/vnd.oci.image.config.v1+json",
        "digest": "sha256:99c9d2dcbdbc6e28277d379c2b9a59443b91937720361773963d28d5376252a9",
        "size": 149
    },
    "layers": [
        {
            "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "digest": "sha256:8289bd1bdc2a1fae2d2d717b7c40baaedc4c5c4d9c9f4f1a1b045287067e9f2c",
            "size": 3279768
        },
        {
            "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "digest": "sha256:c37c06cdec9d6a0f2a2d55deb5aa002b26b37b17c02c2eca908fc062af5f53eb",
            "size": 1372611
        }
    ]
}

$ mv manifest.json $(sha256sum manifest.json | awk '{print $1}')
```

由于我们改动了 manifest，需要用新的摘要和文件大小更新 index.json：

```bash
$ vim index.json
{
    "schemaVersion": 2,
    "manifests": [
        {
            "mediaType": "application/vnd.oci.image.manifest.v1+json",
            "digest": "sha256:8289bd1bdc2a1fae2d2d717b7c40baaedc4c5c4d9c9f4f1a1b045287067e9f2c",
            "size": 748,
            "annotations": {
                "org.opencontainers.image.ref.name": "hello:alpine"
            }
        }
    ]
}
```

如果我们再次打包并用 podman 加载镜像，就能看到改动后的镜像在运行：

```bash
$ tree
├── blobs
│   └── sha256
│       ├── 99c9d2dcbdbc6e28277d379c2b9a59443b91937720361773963d28d5376252a9
│       ├── 8289bd1bdc2a1fae2d2d717b7c40baaedc4c5c4d9c9f4f1a1b045287067e9f2c
│       ├── c37c06cdec9d6a0f2a2d55deb5aa002b26b37b17c02c2eca908fc062af5f53eb
│       └── 2e17c995558ebfa8faacfe64ff78c359ab9f28b3401076bb238fd28c5b3a648b
└── index.json
$ tar -cvf hello-alpine.tar *

$ podman load < hello-alpine.tar
Getting image source signatures
Copying blob c59d5203bc6b done   |
Copying blob c37c06cdec9d done   |
Copying config 75b148a9a5 done   |
Writing manifest to image destination
Loaded image: localhost/hello:alpine

$ podman run --rm localhost/hello:alpine reader
Hello, reader!
real    0m 0.00s
user    0m 0.00s
sys     0m 0.00s

$ podman image ls hello
REPOSITORY   TAG       IMAGE ID       CREATED   SIZE
hello        scratch   25e8b3bd9720   N/A       3.67MB
hello        alpine    3d0268e9a91e   N/A       11MB
```

## 结论

希望这篇文章能帮助你以一种更细致、同时又易于理解的方式认识容器镜像。本文中使用的示例并不是你在日常工作流里会直接用到的东西，它们的目的在于展示容器镜像的内部工作原理。

本文同时发布于作者的个人博客。

## Reference

- 原文：[Build a Container Image from Scratch](https://www.suse.com/c/build-a-container-image-from-scratch/) — Danish Prakash，SUSE Communities（2025-08-11，最后更新 2025-10-10）
- [Open Container Initiative (OCI) Image Format Specification](https://github.com/opencontainers/image-spec)
- [OCI Image Layout](https://github.com/opencontainers/image-spec/blob/main/layout.md)
- [podman 文档](https://docs.podman.io/)
- 本文配图均来自原文，版权归原作者所有。
