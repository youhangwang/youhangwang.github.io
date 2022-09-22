---
title: Ceph Introduction
tags: Ceph
---

Ceph可以同时提供Object、块和文件存储服务，它是一个高度可靠、易于管理且免费的软件。 Ceph提供了非常强大的可扩展性：支持数以千计的Client访问 PB 到 EB 级别的数据。 

<!--more-->

Ceph 提供了一个基于RADOS 的可无限扩展的Storage Cluster，一个 Ceph Storage Cluster由多种类型的daemon进程组成：
- Ceph Monitor: 维护Cluster map的主副本。 Ceph Monitor集群可确保某个Monitor程序发生故障时的高可用性。Client从 Ceph Monitor 获取Cluster map的副本。
- Ceph OSD Daemon: 检查自己的状态和其他 OSD 的状态，然后向Monitor报告。
- Ceph Manager: 充当监控、编排和插件模块的Endpoint。
- Ceph Metadata Server: 元数据服务器 (MDS) 管理文件元数据。

Client和每个 OSD 进程使用 CRUSH 算法来有效地计算有关数据位置的信息，而不必依赖于集中式查找表。 Ceph 的高级特性包括：
- 通过 librados 连接到 Ceph Storage Cluster的原生接口
- 在 librados 之上构建的许多服务接口。

![arch](../../../assets/images/posts/ceph-stack.webp)

## 数据的存储

Ceph Storage Cluster从 Ceph Client 接收数据 —— 无论是通过 Ceph 块设备、Ceph Object存储、Ceph 文件系统还是使用 librados 创建的自定义实现 —— 都存储为 RADOS Object。每个Object都存储在一个 Object Storage Device （OSD）上。 Ceph OSD Daemons处理storage drives上的读、写和复制操作:

- 如果使用老的 Filestore 作为后端，则每个 RADOS Object都作为单独的文件存储在传统文件系统（通常是 XFS）上。
- 如果使用新的默认的 BlueStore 作为后端，Object会以类似数据库的整体方式存储。

![data-storing](../../../assets/images/posts/ceph-data-storing.webp)

Ceph OSD Daemons将数据作为Object存储在扁平的命名空间中（没有目录层次结构）。Object具有标识符、二进制数据和由一组Name/Value对组成的元数据。语义完全取决于 Ceph Client。例如，CephFS 使用元数据来存储文件属性：文件所有者、创建日期、最后修改日期等。

![object](../../../assets/images/posts/ceph-object.webp)

## 扩展性和高可用性

传统架构一般采用Client与一个集中式的组件（例如网关、代理、API、外观等）对话，该集中式组件充当复杂子系统的单一入口点。这在对性能和可扩展性都施加了限制的同时，还引入了单点故障（即，如果集中式组件出现故障，整个系统也会出现故障）。相比之下：

- Ceph 中并没有采用集中式网关，而是使Client能够直接与 Ceph OSD Daemons交互。 
- Ceph OSD Daemons 会在其他 Ceph 节点上创建Object副本，以确保数据安全和高可用性。 
- Ceph 还使用一组Monitor 实例用来确保Monitor 服务的高可用。
- 为了消除中心化，Ceph 使用了一种称为 CRUSH 的算法。

### CRUSH 介绍

Ceph Client和 Ceph OSD Daemons都使用 CRUSH 算法来计算有关Object位置的信息，而不必依赖于一个集中式的查找表。CRUSH 提供了更好的数据管理机制，并通过将工作分配给集群中的所有Client和 OSD Daemons来实现大规模扩展。 CRUSH 使用智能的数据复制来确保弹性，适合超大规模存储。

### Cluster Map

Ceph实现上述功能的前提是Ceph Client和 Ceph OSD Daemons都能了解集群的拓扑结构，其中包括 5 个统称为`Cluster Map`的 Map：

- Monitor Map：包含集群 fsid、每个Monitor的位置、名称地址和端口。它还指示当前纪元(epoch)、创建的时间以及上次更改的时间。要查看Monitor Map，请执行`ceph mon dump`。
- OSD Map：包含集群 fsid、Map的创建和上次修改时间、Pool列表、副本大小、PG 数量、OSD 列表及其状态（例如，up、in）。要查看 OSD Map，请执行 `ceph osd dump`。
- PG Map：包含 PG 版本、它的时间戳、最新 OSD map epoch、填充比率和每个PG的详细信息，例如 PG ID、Up Set、Acting Set、PG 的状态（例如, active + clean) 和每个Pool的数据使用统计信息。
- CRUSH Map：包含 storage device 列表、故障域层次结构（例如，设备、主机、机架、行、房间等），以及在存储数据时遍历层次结构的规则。查看 CRUSH Map，执行 `ceph osd getcrushmap -o {filename}`; 然后，通过执行`crushtool -d {comp-crushmap-filename} -o {decomp-crushmap-filename}`反编译它。您可以在文本编辑器或 cat 中查看反编译的Map。
- MDS Map：包含当前 MDS Map时代、Map创建时间和上次更改时间。它还包含用于存储元数据的Pool、元数据服务器列表以及哪些元数据服务器已Up和In。要查看 MDS Map，请执行`ceph fs dump`。

每个Map都维护其操作状态变化的迭代历史。 Ceph Monitor 维护Cluster Map的主副本，包括集群成员、状态、更改和 Ceph Storage Cluster的整体健康状况。

### 高可用Monitors

在 Ceph Client可以读取或写入数据之前，它们必须联系 Ceph Monitor 以获取最新的Cluster Map副本。一个 Ceph Storage Cluster可以只运行单个Monitor；但是，这会引入单点故障（即，如果Monitor出现故障，Ceph Client将无法读取或写入数据）。

为了增加可靠性和容错性，Ceph 支持Monitor集群。在Monitor集群中，延迟和其他故障可能导致一个或多个Monitor落后于集群的当前状态。出于这个原因，Ceph 必须在各种Monitor实例之间就集群的状态达成一致。 Ceph 始终使用大多数Monitor（例如 1、2:3、3:5、4:6 等）和 Paxos 算法在Monitor之间就集群的当前状态建立共识。

### 高可用认证

为了识别用户身份并防止中间人攻击，Ceph 提供了`cephx`身份验证系统来对用户和Daemons进行身份验证。

Cephx 使用共享密钥进行身份验证，Client和Monitor集群都拥有Client密钥的副本。身份验证协议使得双方能够相互证明他们拥有密钥的副本，而无需实际透露它。这提供了相互身份验证，这意味着集群确定用户拥有密钥，并且用户确定集群具有密钥的副本。

Ceph 可扩展特性的一个关键是避免实现了一个 Ceph Object Store的集中式接口，这意味着 Ceph Client必须能够直接与 OSD 交互。为了保护数据，Ceph 提供了其 cephx 身份验证系统，该系统对操作 Ceph Client的用户进行身份验证：

- 用户首先调用 Ceph Client来连接Monitor: 每个Monitor都可以对用户进行身份验证并分发密钥，因此使用 cephx 时不会出现单点故障或瓶颈。
- Monitor返回一个类似于 Kerberos ticket的身份验证数据结构，其中包含用于获取 Ceph 服务的会话密钥。这个会话密钥本身是使用用户的永久密钥加密的，因此只有用户可以从 Ceph Monitor(s) 请求服务。
- Client然后使用会话密钥向Monitor请求其所需的服务
- Monitor向Client提供一张Ticket，实际处理数据的 OSD可以使用该Ticket验证Client。 Ceph Monitor和 OSD 共享一个密钥，因此Client可以使用Monitor提供的Ticket与集群中的任何 OSD 或元数据服务器一起使用。

与 Kerberos 一样，cephx Ticket有过期时间，因此攻击者无法使用偷偷获得的过期Ticket或会话密钥。只要用户的密钥在过期之前没有泄露，这种形式的身份验证将防止有权访问通信介质的攻击者在另一个用户的身份下创建虚假消息或更改另一个用户的合法消息。

具体流程为：

1. 要使用 cephx，管理员必须先设置用户。在下图中，client.admin 用户从命令行调用`ceph auth get-or-create-key`来生成用户名和密钥。 Ceph 的 auth 子系统生成用户名和密钥，将副本存储在Monitor中，并将用户的秘钥传输回 client.admin 用户。这意味着Client和Monitor共享一个密钥。

![cephx-step1](../../../assets/images/posts/cephx-step1.webp)


2. 为了向Monitor验证身份，Client将用户名传递给Monitor，Monitor生成一个会话密钥并使用与用户名关联的密钥对其进行加密。然后，Monitor将加密的Ticket返回给Client。Client使用共享密钥解密该Ticket的有效负载并取会话密钥。会话密钥标识了当前会话的用户。然后，Client代表由会话密钥签名的用户请求Ticket。Monitor生成一个新的Ticket，使用用户的密钥对其进行加密并将其传输回Client。Client解密Ticket并使用它对整个集群的 OSD 和元数据服务器的请求进行签名。

![cephx-step2](../../../assets/images/posts/cephx-step2.webp)

cephx 协议验证 Client 和 Ceph 服务器之间的通信。在初始身份验证之后，Client和服务器之间发送的每条消息都使用生成的Ticket进行签名。Monitor、OSD 和元数据服务器可以使用它们的共享密钥进行对该签名进行验证。

![cephx-all](../../../assets/images/posts/cephx-all.webp)

此身份验证提供的保护位于 Ceph Client和 Ceph 服务器主机之间, 不会扩展到 Ceph Client之外。如果用户从远程主机访问 Ceph Client，则 Ceph 身份验证不会应用于用户主机与Client主机之间的连接。

### 支持超大规模集群的智能Daemon

在许多集群架构中，集群成员的主要目的是让集中式的接口知道它可以访问哪些节点。然后集中式接口通过双重调度向Client提供服务——这是 PB 到 EB 规模提升的巨大瓶颈。

Ceph 消除了这种瓶颈：Ceph 的 OSD Daemons和 Ceph Client是集群感知的。与 Ceph Client一样，每个 Ceph OSD Daemons都知道集群中的其他 Ceph OSD Daemons。这使 Ceph OSD Daemons能够直接与其他 Ceph OSD Daemons或者 Ceph Monitor交互。此外，这使 Ceph Client能够直接与 Ceph OSD Daemons交互。利用这种计算能力的能力带来了几个主要好处：

- OSD 直接服务 Client：由于任何网络设备都对其支持的并发连接数有限制，因此集中式系统在大规模下具有较低的物理限制。通过使 Ceph Client能够直接联系 Ceph OSD Daemons，Ceph 同时提高了性能和总体的系统容量，同时消除了单点故障。 Ceph Client可以在需要时使用特定的 Ceph OSD Daemons而不是集中式的服务器来维护会话。
- OSD 成员和状态：Ceph OSD Daemons加入集群并报告其状态。
  - 在最低级别，Ceph OSD 守护程序状态为`up`或`down`，反映它是否正在运行并能够为 Ceph Client请求提供服务。
    - 如果 Ceph OSD 守护程序 `down`并且 `in` Ceph Storage Cluster，此状态可能表示 Ceph OSD 守护程序发生故障。如果 Ceph OSD 守护程序没有运行（例如，崩溃），则 Ceph OSD 守护程序无法通知 Ceph Monitor它已变为 `down`。 OSD 会定期向 Ceph Monitor 发送消息（`MPGStats` 在版本 pre-luminous， MOSDBeacon 在新的版本 luminous）。如果 Ceph Monitor 在一段可配置的时间后没有看到该消息，那么它会将 OSD 标记为`down`。然而，这种机制是一种故障保护机制。通常，Ceph OSD Daemons将确定相邻 OSD 是否已关闭并将其报告给 Ceph Monitor。这确保了 Ceph Monitor是轻量级进程。
- 数据整理：作为维护数据一致性和数据清洁的一部分，Ceph OSD Daemons可以清理Object。也就是说，Ceph OSD Daemons可以将其本地Object元数据与存储在其他 OSD 上的副本进行比较。清理发生在每个Placement Group 级别。清理（​​通常每天执行）会发现数据大小和其他元数据的不匹配。 Ceph OSD Daemons还通过逐位比较Object中的数据与其校验和来执行更深入的清理。深度清理（通常每周执行一次）会发现驱动器上在轻度清理中没有发现的坏扇区。
- 复制：与 Ceph Client一样，Ceph OSD Daemons使用 CRUSH 算法，但 Ceph OSD Daemons使用它来计算Object副本应该存储在哪里（以及重新平衡）。在典型的写入场景中，Client使用 CRUSH 算法计算存储Object的位置，将Object Map到Pool和Placement Group，然后查看 CRUSH Map以识别Placement Group的主 OSD。

Client将Object写入计算出来的Placement Group中的主 OSD。然后，主 OSD 根据自己的 CRUSH Map副本识别二级和三级 OSD 用于复制目的，并将Object复制到适当Placement Group中的二级和三级 OSD（与附加副本一样多的 OSD），一旦确认Object已成功存储，返回执行结果给Client。凭借执行数据复制的能力，Ceph OSD Daemons将 Ceph Client从这一职责中解脱出来，同时确保高数据可用性和数据安全性。

## DYNAMIC CLUSTER MANAGEMENT

在可扩展性和高可用性部分，我们解释了 Ceph 如何使用 CRUSH、集群感知和智能Daemons来扩展和保持高可用性。 Ceph 设计的关键是自主、自我修复和智能的 Ceph OSD Daemons。

### Pools
Ceph 存储系统支持 `Pool` 的概念，它是存储Object的逻辑分区。

Ceph Client从 Ceph Monitor 获取 Cluster Map，并将Object写入Pool中。Pool的大小或副本数量、CRUSH 规则和Placement Group的数量决定了 Ceph 将如何放置数据。

Pool至少设置以下参数：
- Object的所有权/访问权
- Placement Group的数量
- 要使用的 CRUSH 规则

### 将 Placement group 映射到 OSDS

每个Pool都有许多的Placement Group。 CRUSH 将 PG 动态映射到 OSD。当 Ceph Client存储Object时，CRUSH 会将每个Object Map到一个Placement Group。

将Object Map到Placement Group会在 Ceph OSD Daemons和 Ceph Client之间创建一个中间层。 因为Object是动态存储的， Ceph Storage Cluster必须能够增长（或缩小）和重新平衡。如果 Ceph Client知道哪个 Ceph OSD Daemons拥有哪个Object，那将在 Ceph Client和 Ceph OSD Daemons之间产生紧密耦合。相反，CRUSH 算法将每个Object Map到一个Placement Group，然后将每个Placement Group Map到一个或多个 Ceph OSD Daemons。这中间层允许 Ceph 在新的 Ceph OSD Daemons以及底层 OSD 设备上线时动态地重新平衡。下图描述了 CRUSH 如何将Object Map到Placement Group，并将Placement Group Map到 OSD。

![ceph-mapping](../../../assets/images/posts/ceph-mapping.webp)


通过使用Cluster Map 和 CRUSH 算法，Client在读取或写入特定Object时可以准确地计算需要使用哪个 OSD。

### Calculating PG IDs 

当 Ceph Client 连接到 Ceph Monitor时，它会获取最新的Cluster Map副本。通过Cluster Map，Client可以了解集群中的所有Monitor、OSD 和元数据服务器。但是，它对Object位置一无所知。Object的位置是通过计算得到的。

为了执行该计算，Client需要的唯一输入是`Object ID`和`Pool`。过程很简单：Ceph 将数据存储在命名Pool中（例如 “liverpool”）。当Client想要存储一个命名Object（例如，“john”、“paul”、“george”、“ringo”等）时，它会使用Object名称、一个哈希码、Pool中的PG 数量和Pool名称来计算Placement Group。Ceph Client使用以下步骤来计算 PG ID。

- Client输入Pool名称和Object ID。 （例如，pool = “liverpool” 和 object-id = “john”）
- Ceph 获取Object ID 并对其进行哈希处理。
- Ceph 以 PG 的数量为模计算哈希值。 （例如 58）来获取 PG ID。
- Ceph 根据Pool名称获取Pool ID（例如，“liverpool” = 4）
- Ceph 将Pool ID 和 PG ID 连接到一起（例如，4.58）。

计算Object位置比通过聊天会话的形式执行Object位置查询要快得多。 CRUSH 算法允许Client计算Object应该存储在哪里，并使Client能够联系主 OSD 以存储或获取Object。

### Peering and Sets

Ceph OSD Daemons会检查彼此的心跳并向 Ceph Monitor 报告。 Ceph OSD Daemons所做的另一件事称为`peering`：
- 这是使Placement Group (PG) 中所有存储的 OSD 就该 PG 中所有的Object（及其元数据）的状态达成一致的过程。

Ceph OSD Daemons会向 Ceph Monitor报告`peering`执行失败。peering问题通常会自行解决；但是，如果问题没有自行解决，可能需要参考peering故障故障排除部分。

Ceph Storage Cluster至少存储Object的两个副本（即大小 = 2），这是数据安全的最低要求。为了实现高可用性，Ceph Storage Cluster应该存储两个以上的Object副本（例如，大小 = 3 和最小大小 = 2），以便它可以在降级状态下可以继续运行，同时保持数据安全。

当一系列 OSD 负责一个Placement Group时，我们将这一系列 OSD 称为 Acting Set。按照惯例，Primary 是 Acting Set 中的第一个 OSD，同时负责协调该Placement Group的`peering`进程，并且它是唯一一个接受Client发起的Object写入请求的 OSD。对于给定的Placement Group，它充当主节点。Acting Set的 一部分 Ceph OSD Daemons可能并不总是处于`up`状态。当 Acting Set 中的 OSD 处于`up`状态时，它是 Up Set 的一部分。 Up Set 是一个重要的区别，因为当 OSD 发生故障时，Ceph 可以将 PG 重新Map到其他 Ceph OSD Daemons。

### Rebalancing

将新的 Ceph OSD Daemon添加到 Ceph Storage Cluster时，Cluster Map会使用新的 OSD 进行更新。这会更改Cluster Map。因此，它改变了Object的放置，因为它改变了计算的输入。下图描述了重新平衡过程，其中一些但不是所有 PG 从现有 OSD（OSD 1 和 OSD 2）迁移到新 OSD（OSD 3 ）。即使在再平衡时，CR​​USH 也是稳定的。许多Placement Group保持原来的配置，每个 OSD 都获得了一些额外的容量，因此在重新平衡完成后新 OSD 上没有负载峰值。

![ceph-rebalanceing](../../../assets/images/posts/ceph-rebalanceing.webp)


