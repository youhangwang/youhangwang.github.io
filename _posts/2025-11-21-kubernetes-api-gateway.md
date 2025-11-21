---
title: Kubernetes API Gateway Overview
tags: K8S ingress APIGateway
---

## Introduction

Gateway API 是 Kubernetes 的一个官方项目，专注于 Kubernetes 中的 L4 和 L7 路由。该项目代表了下一代 Kubernetes Ingress、负载均衡和服务网格 API。从一开始，它就被设计为通用、富有表现力且面向角色的。

其整体资源模型聚焦于 3 个独立的人员角色以及他们各自预期管理的相应资源：

![gateway-api-role.png](../../../assets/images/posts/gateway-api-role.png)

该 API 中的大部分配置都包含在路由层中。这些特定于协议的资源（HTTPRoute、GRPCRoute 等）为 Ingress 和网格（Mesh）提供了高级路由功能。

<!--more-->


### Gateway API Ingress
使用 Gateway API 管理入口流量时，`Gateway` 资源定义了一个访问点，流量可由此在多个上下文中路由——例如，从集群外部进入集群内部（南北向流量）。
每个 Gateway 都关联一个 `GatewayClass`，后者描述实际处理该 `Gateway` 流量的网关控制器类型；随后，单个路由资源（如 `HTTPRoute`）再与 `Gateway` 资源关联。将不同关注点拆分到不同资源，是 Gateway API“面向角色”设计的核心，也允许同一集群内同时存在多种网关控制器（由 `GatewayClass` 资源表示），且每种控制器可有多个实例（由 `Gateway` 资源表示）。

### Gateway API 用于服务网格（GAMMA 计划）
用 Gateway API 管理服务网格时情况略有不同。由于集群通常只运行一个网格，因此不再使用 `Gateway` 与 `GatewayClass` 资源；取而代之的是，将单个路由资源（如 `HTTPRoute`）直接关联到 `Service` 资源，使网格能够管理所有指向该 `Service` 的流量，同时保持 Gateway API 的面向角色特性。
迄今为止，GAMMA 仅对 Gateway API 做了极少改动便支撑了网格功能。不过，其中一个迅速变得关键的方向，是对 Service 资源不同“切面”的明确定义。

### Gateway API 核心概念
以下设计目标驱动了 Gateway API 的概念体系，展示了它如何超越现行的 Ingress 标准：
- 面向角色 – Gateway 由一系列 API 资源组成，这些资源建模了使用并配置 Kubernetes 服务网络的组织角色。
- 可移植 – 这不是改进点，而是需要保持的特性。正如 Ingress 是一份通用规范、拥有众多实现，Gateway API 也被设计为可由多种实现支持的便携规范。
- 表达力强 – Gateway API 资源原生支持基于头部的匹配、流量权重等核心功能，而这些能力在 Ingress 中只能通过自定义注解实现。
- 可扩展 – Gateway API 允许在 API 的不同层级关联自定义资源，从而在最合适的位置进行细粒度定制。

其他值得注意的能力包括：
- GatewayClasses – 将负载均衡实现类型形式化，让用户通过 Kubernetes 资源模型即可清晰了解可用能力。
- 共享 Gateway 与跨命名空间支持 – 允许独立的路由资源绑定到同一 Gateway，实现负载均衡器与 VIPs 的共享，使团队（甚至跨命名空间）无需直接协同即可安全共享基础设施。
- 类型化路由与类型化后端 – 支持类型化的 Route 资源以及多种后端类型，使 API 能够灵活支持 HTTP、gRPC 等不同协议，以及 Kubernetes Service、存储桶、函数等不同后端目标。
- 实验性的服务网格支持（GAMMA 计划） – 通过将路由资源与 Service 资源关联，Gateway API 可同时配置服务网格与入口控制器。


### 为什么“面向角色”的 API 至关重要？
无论是道路、电网、数据中心还是 Kubernetes 集群，基础设施天生就是要被共享的。然而，共享基础设施总会带来一个共同难题：如何在保证基础设施所有者掌控力的同时，为使用者提供足够的灵活性？

Gateway API 通过“面向角色”的设计解决了这一矛盾。它将 Kubernetes 服务网络划分为不同组织角色，既让分布式团队获得灵活自主权，又让集群运营方保留集中约束与治理的能力。换句话说，它让硬件负载均衡、云网络、集群内代理等共享网络基础设施，可以被许多互不协同的团队安全使用，而这些团队都必须遵守集群运维设定的策略与边界。

Gateway API 的设计围绕三种典型角色展开：


| 角色                      | 谁                                             | 关注点                          | 视角                    |
| ----------------------- | --------------------------------------------- | ---------------------------- | --------------------- |
| **基础设施提供者** <br> (Ian)  | 他负责跨租户、跨集群的底层基础设施，例如多租户环境或混合云底座。              | 让“所有”租户集体受益，而非偏袒某一个。         | “这些集群整体得稳定、安全、可扩展。”   |
| **集群运维** <br> (Chihiro) | 他们管理单个 Kubernetes 集群，对接多个业务团队。                | 保证集群资源、策略、安全对“所有”用户都够用。      | “集群策略我说了算，但得让各业务跑得快。” |
| **应用开发者** <br> (Ana)    | 她只关心业务功能，Kubernetes 和 Gateway API 对她只是“拦路摩擦”。 | 快速把流量路由到她的服务，别让她写 YAML 写到崩溃。 | “别让我管基础设施，我只想把代码跑起来。” |

显然，Ian、Chihiro、Ana 并非事事意见一致，却必须协同才能让系统顺畅运转——这正是 Gateway API 要解决的核心挑战。

#### 用例演示

官方给出的示例用例刻意用这三位角色的视角来叙述，展示同一套 API 如何适配完全不同的组织形态与实现方式，同时保持可移植与标准化。归根结底，Gateway API 是给人用的：它必须贴合 Ian 的治理需求、Chihiro 的运维需求，以及 Ana“越少越好”的开发体验。


### Gateway API 与 API Gateway 有何不同？
- API Gateway（API 网关）： 是一种“工具”，用于把多个独立的应用 API 聚合到同一个入口。它把认证、授权、限流等横切功能从业务代码里抽出来，集中到一个统一的地方管理，最终作为（通常是外部）API 消费者的统一界面。

- Gateway API：则是一组“接口规范”——具体说，是 Kubernetes 里用来建模服务网络的一整套资源定义。它的核心资源之一也叫 Gateway，但这里的 Gateway 只是声明“要实例化的网关类型/类及其配置”。作为 Gateway 提供商，你可以通过实现 Gateway API，以表达力强、可扩展且面向角色的方式来建模 Kubernetes 内的服务网络。

一句话：API 网关是“软件产品/中间件”；Gateway API 是“Kubernetes 资源规范”。当然，有些 API 网关产品本身就可以用 Gateway API 来配置和编程。

## Concepts

### API Oerview

Gateway API 主要面向 3 种角色（详见 roles and personas）：
- Ian（基础设施提供者）
- Chihiro（集群运维）
- Ana（应用开发者）

#### 资源模型
**说明：下文所有资源均位于 API 组 gateway.networking.k8s.io，以 CRD 形式提供。未显式写 API 组的资源名均默认属于该组。**
模型包含三大类对象：
- GatewayClass： 定义一组具有相同配置与行为的 Gateway。每个 GatewayClass 由单一控制器处理，一个控制器可处理多个 GatewayClass。
  - 集群级资源
  - 至少需存在一个 GatewayClass 才能创建有效的 Gateway
  - 作用类似于 Ingress 的 IngressClass 或 PV 的 StorageClass
- Gateway： 描述“如何把流量转译到集群内的 Service”，即请求一种把“不知道 Kubernetes 的外部流量”转译到“知道 Kubernetes 的内部服务”的方式。
  - 可由运维手动创建，也可由 GatewayClass 控制器自动创建
  - spec 中可省略地址、TLS 等字段，由控制器通过 GatewayClass Status 补充，实现更高的可移植性
  - 可绑定一个或多个 Route 资源，将部分流量导向特定服务
- Route 资源：定义“协议专属”的规则，把 Gateway 收到的请求映射到 Kubernetes Service。
  - 当前内置四种 Route，实现者亦可自定义新的 Route 类型。
    - HTTPRoute（标准通道，v0.5.0+）：用于多路复用 HTTP 或已终结的 HTTPS 连接，可基于 HTTP 头路由或修改请求。
    - TLSRoute（实验通道，v0.3.0+）：基于 SNI 多路复用 TLS 连接，只关心 TLS 元数据，不关心上层协议。
      - Passthrough 模式：加密字节流直接透传到后端，由后端解密。
      - Terminate 模式：网关上终结 TLS。
    - TCPRoute / UDPRoute（实验通道，v0.3.0+）
      - 把单个或多个端口映射到单一后端；同一端口无法再做更细区分，因此通常需要为每条 TCPRoute 分配不同监听端口。
      - 可选择是否终结 TLS：终结则透传明文流，不终结则透传加密流。
    - GRPCRoute（标准通道，v1.1.0+）：专为 gRPC 流量设计。支持 GRPCRoute 的 Gateway 必须原生支持 HTTP/2（无需 HTTP/1 升级），确保 gRPC 流量可靠传输。


#### 将 Route 附加到 Gateway ：
当 Route 附加到 Gateway 时，它代表将在 Gateway 上应用的配置，这些配置用于配置底层负载均衡器或代理。如何附加以及哪些 Route 可以附加到 Gateway，由资源本身控制。Route 和 Gateway 资源内置了允许或限制附加方式的机制。与 Kubernetes RBAC 结合使用，这些机制使组织能够强制执行关于 Route 如何暴露以及在哪些 Gateway 上暴露的策略。

在如何将 Route 附加到 Gateway 方面具有很大的灵活性，以实现不同的组织策略和职责范围。Gateway 和 Route 可以具有不同的关系：
- 一对一： 一个 Gateway 和 Route 可能由单个所有者部署和使用，并且具有一对一的关系。
- 一对多： 一个 Gateway 可以有许多 Route 绑定到它，这些 Route 由来自不同命名空间的不同团队拥有。
- 多对一： Route 也可以绑定到多个 Gateway，允许单个 Route 同时在不同的 IP、负载均衡器或网络上控制应用程序的暴露。

Chihiro 在 infra 命名空间中部署了一个共享 Gateway（shared-gw），供不同的应用程序团队用于将他们的应用程序暴露在集群外部。A 团队和 B 团队（分别在命名空间 A 和 B 中）将他们的 Route 附加到这个 Gateway 上。他们彼此不知道对方，只要他们的 Route 规则不冲突，就可以继续独立操作。C 团队有特殊网络需求（可能是性能、安全性或关键性），他们需要一个专用的 Gateway 来将他们的代理到外部世界。C 团队在 C 命名空间中部署了他们自己的专用 Gateway（dedicated-gw），这个 Gateway 只能由 C 命名空间中的应用程序使用。

要让一条 Route 成功附加到 Gateway，必须满足以下条件：
- `Route` 的 `parentRefs` 字段中必须有一项引用该 Gateway。
- `Gateway` 上至少有一个监听器（listener）允许这次附加。

Route 通过在 parentRef 中指定 Gateway 的命名空间（如果与 Route 同命名空间则可省略）和名称来引用它。默认情况下，Route 会附加到 Gateway 的所有监听器，但可以通过 parentRef 中的下列字段把选择范围缩小到部分监听器：
- sectionName：设置后，Route 只选择具有该名称的监听器。
- port：设置后，Route 选择所有在该端口监听且协议与当前 Route 类型兼容的监听器。

如果 `parentRef` 里同时设置了多个字段，Route 只会选择同时满足全部条件的监听器。例如同时设置了 sectionName 与 port，则必须“名字匹配且端口匹配”的监听器才会被选中。

每个 Gateway 监听器都能通过以下机制限制哪些 Route 可以附加：
- Hostname: 如果监听器设置了 hostname，那么附加的 Route 必须在自身的 hostnames 字段里至少有一个值与监听器重叠。
- Namespaces: 监听器里的 allowedRoutes.namespaces 字段可控制 Route 来源命名空间：
  - Same（默认）：仅允许与 Gateway 同一命名空间里的 Route 附加。
  - All：允许来自所有命名空间的 Route 附加。
  - Selector：仅允许标签选择器选中的一组命名空间里的 Route 附加。使用此值时，必须同时在 namespaces.selector 字段给出标签选择器；该字段与 All 或 Same 不可混用。
- Kinds: 监听器里的 allowedRoutes.kinds 字段可限制允许附加的 Route 类型（如 HTTPRoute、TLSRoute 等）。

如果以上字段均未指定，监听器将信任来自同一命名空间且协议兼容的 Route。

下面的 `my-route` 想附加到位于 `gateway-api-example-ns1` 命名空间的 `foo-gateway`，并且不附加到任何其他 `Gateway`。注意 `foo-gateway` 与 `my-route` 并不在同一命名空间。`foo-gateway` 必须显式允许来自 `gateway-api-example-ns2` 命名空间的 `HTTPRoute` 附加。

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-route
  namespace: gateway-api-example-ns2
spec:
  parentRefs:
  - kind: Gateway
    name: foo-gateway
    namespace: gateway-api-example-ns1
  rules:
  - backendRefs:
    - name: foo-svc
      port: 8080
```


对应的 foo-gateway 配置如下，它允许上面这条 my-route 附加：

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: foo-gateway
  namespace: gateway-api-example-ns1
spec:
  gatewayClassName: foo-lb
  listeners:
  - name: prod-web
    port: 80
    protocol: HTTP
    allowedRoutes:
      kinds:
      - kind: HTTPRoute
      namespaces:
        from: Selector
        selector:
          matchLabels:
            # 该标签从 K8s 1.22 开始自动注入到所有命名空间
            kubernetes.io/metadata.name: gateway-api-example-ns2
```

更宽松的示例。下面的 Gateway 允许所有带标签 `expose-apps: "true"` 的命名空间中的 `HTTPRoute` 附加。

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: prod-gateway
  namespace: gateway-api-example-ns1
spec:
  gatewayClassName: foo-lb
  listeners:
  - name: prod-web
    port: 80
    protocol: HTTP
    allowedRoutes:
      kinds:
      - kind: HTTPRoute
      namespaces:
        from: Selector
        selector:
          matchLabels:
            expose-apps: "true"
```

`GatewayClass`、`Gateway`、`xRoute` 与 `Service` 的组合共同定义了一个可落地的负载均衡器。下图展示了这些不同资源之间的关系：

![gateapi-conbined-type.png](../../../assets/images/posts/gateapi-conbined-type.png)

一个典型的南北向 API 请求流程（基于反向代理实现的网关）如下：
- 客户端向 http://foo.example.com 发起请求。
- DNS 将该域名解析为某个 Gateway 的地址。
- 反向代理在监听器（Listener）上收到请求，并使用 Host 头匹配到一条 HTTPRoute。
- （可选）根据 HTTPRoute 的匹配规则，反向代理进一步执行请求头或路径匹配。
- （可选）根据 HTTPRoute 的过滤器规则，反向代理可修改请求，例如添加或删除头部。
- 最后，反向代理依据 HTTPRoute 的 backendRefs 规则，把请求转发到集群中的一个或多个对象（例如 Service）。


API 提供了多处扩展点，以应对通用 API 无法覆盖的大量场景。
当前扩展点汇总如下：
- BackendRefs：用于把流量转发到除核心 Kubernetes Service 之外的终点，例如 S3 存储桶、Lambda 函数、文件服务器等。
- HTTPRouteFilter： HTTPRoute 中的这一 API 类型，允许在 HTTP 请求/响应生命周期中插入自定义逻辑。
- 自定义 Route（Custom Routes）：若上述扩展点仍无法满足需求，实现者可以为当前 API 尚未支持的协议创建自定义 Route 资源。自定义 Route 类型必须复用核心 Route 的公共字段，这些字段集中在 CommonRouteSpec 和 RouteStatus 中。




### Roles and Personas

在 Kubernetes 最初的设计里，Ingress 与 Service 资源基于这样一种使用模型：创建 Service 和 Ingress 的开发者能够完全掌控如何定义并暴露他们的应用。

然而现实中，集群及其基础设施往往是共享的，而原始 Ingress 模型对此刻画得并不好。共享基础设施时，使用者关注的东西并不相同；一个基础设施项目要想成功，就必须兼顾所有用户的需求。

这就带来了一个根本挑战：怎样才能既给基础设施使用者提供所需灵活性，又让基础设施所有者保持必要控制？

Gateway API 为此定义了若干独立角色，并为每个角色赋予一个具体人员形象（persona），用来呈现并讨论不同用户的差异化需求，从而在易用性、灵活性与控制性之间取得平衡。Gateway API 的所有设计工作都刻意以这些 persona 为视角展开。

注意：根据环境不同，同一个人可能同时承担多个角色，下文会具体讨论。

Gateway API 定义了三种角色与 persona：

Ian（他）——基础设施提供者：负责维护一套能让多个隔离集群服务多租户的基础设施。他不效忠于任何单一租户，而是集体照看所有租户。Ian 通常就职于云提供商（AWS、Azure、GCP…）或 PaaS 提供商。
Chihiro（他们）——集群运维：负责管理集群，确保其满足众多用户的需求。Chihiro 通常关注策略、网络访问、应用权限等。同样，他们不效忠于任何单一用户，而是要让集群服务所有用户。
Ana（她）——应用开发者：负责在集群中创建并管理应用。从 Gateway API 视角看，Ana 需要管理配置（超时、请求匹配/过滤）与 Service 组合（路径路由到后端）。她所处的位置独特：关注的是业务需求，而非 Kubernetes 或 Gateway API 本身；事实上，她很可能把 Gateway API 和 Kubernetes 视为完成工作的“纯粹阻力”。

同一用户可对应多个角色：
- 把上述角色全给一个人，就得到了自服务模型——可能在裸金属上跑 Kubernetes 的小初创真会如此。
- 更典型的小初创会使用云提供商的集群：此时 Ana 与 Chihiro 可能是同一个人，而 Ian 是云提供商的员工（或自动化流程！）。
- 在大型组织里，每个 persona 通常由不同人承担，往往分属不同团队，甚至彼此几乎没有直接联系。

RBAC（基于角色的访问控制）是 Kubernetes 授权的标准。它让用户可以配置“谁”能在“哪些范围”对“哪些资源”执行什么操作。我们预期每个 persona 大致对应 Kubernetes RBAC 体系中的一个 Role，从而定义资源模型的职责与隔离。

### Security Model

Gateway API 的设计目标是为典型组织中的每个角色提供细粒度授权。

Gateway API 有 3 个核心 API 资源：
- GatewayClass：定义一组具有相同配置与行为的网关。
- Gateway：请求一个流量入口点，将流量转译到集群内的 Service。
- Routes：描述通过 Gateway 进来的流量如何映射到 Services。

Gateway API 定义了 3 个主要角色：
- Ian（基础设施提供者）
- Chihiro（集群运维）
- Ana（应用开发者）

RBAC
RBAC（基于角色的访问控制）是 Kubernetes 授权的标准，可用来配置“谁”能在“哪些范围”对资源执行什么操作。下面聚焦“写权限”，因为大多数场景下所有资源都应对多数角色可读。

3 层模型的写权限（Write Permissions for Simple 3 Tier Model）
| 角色\资源   | GatewayClass | Gateway | Route |
| ------- | ------------ | ------- | ----- |
| 基础设施提供者 | Yes          | Yes     | Yes   |
| 集群运维    | No           | Yes     | Yes   |
| 应用开发者   | No           | No      | Yes   |


4 层模型的写权限（Write Permissions for Advanced 4 Tier Model）
| 角色\资源   | GatewayClass | Gateway   | Route     |
| ------- | ------------ | --------- | --------- |
| 基础设施提供者 | Yes          | Yes       | Yes       |
| 集群运维    | Sometimes    | Yes       | Yes       |
| 应用管理员   | No           | 仅能在指定命名空间 | 仅能在指定命名空间 |
| 应用开发者   | No           | No        | 仅能在指定命名空间 |


Gateway API 提供了新的跨命名边界机制。这些能力非常强大，但必须谨慎使用，以免意外暴露。规则：每次允许跨越命名空间时，都必须完成“握手”。握手有两种方式：

- Route 绑定（Route Binding）

Route 可以绑定到位于不同命名空间的 Gateway。
Gateway 所有者必须在监听器里显式允许来自其他命名空间的 Route 绑定，示例：
```yaml
namespaces:
  from: Selector
  selector:
    matchExpressions:
    - key: kubernetes.io/metadata.name
      operator: In
      values:
      - foo
      - bar
```

以上配置允许来自 foo 与 bar 命名空间的 Route 附加到此 Gateway 监听器。虽然可以用自定义标签，但不如`kubernetes.io/metadata.name`安全。后者由 Kubernetes 自动设置为命名空间名称，具备一致性；而自定义标签（如 env）任何人只要能给命名空间打标签就能改变 Gateway 支持的命名空间范围。

- ReferenceGrant

某些场景允许对象引用跨越命名空间，例如 Gateway 引用 Secret、Route 引用 Backend（通常是 Service）。此时所需的握手通过 ReferenceGrant 完成。该资源放在目标命名空间，允许来自其他命名空间的引用。示例：允许 prod 命名空间中的 HTTPRoute 引用本命名空间内的 Service。

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-prod-traffic
spec:
  from:
  - group: gateway.networking.k8s.io
    kind: HTTPRoute
    namespace: prod
  to:
  - group: ""
    kind: Service
```

某些基础设施提供者或集群运维希望限制 GatewayClass 只能在特定命名空间使用。
目前 API 本身未内置该能力，推荐使用策略引擎（如 Open Policy Agent + Gatekeeper）来强制执行。

### Use Cases

#### 基本南北向用例（Basic north/south use case）  

Ana 开发了一个微服务应用，想在 Kubernetes 里运行。她的应用要供集群外客户端使用；应用虽由 Ana 编写，但集群搭建不在她的职责范围。

1. Ana 找 Chihiro，请其帮忙建集群，并说明客户端将通过以 https://ana.application.com/ 为根的 URL 访问她的 API。  
2. Chihiro 向 Ian 申请集群。  
3. Ian 交付一个运行网关控制器的集群，并带有 GatewayClass 资源，名为 basic-gateway-class；该控制器负责把外部流量引入集群。  
4. Ian 把新集群的凭证交给 Chihiro，并告知可使用 GatewayClass basic-gateway-class 进行后续配置。  
5. Chihiro 提交 Gateway 资源 ana-gateway，指定在 443 端口监听 TLS，并提供主题为 ana.application.com 的证书，将此 Gateway 关联到 basic-gateway-class。  
6. 网关控制器为 ana-gateway 分配负载均衡器与 IP，部署数据面组件，开始监听与 ana-gateway 相关的路由资源。  
7. Chihiro 获取 ana-gateway 的 IP，在集群外为 ana.application.com 创建 DNS 记录。  
8. Chihiro 通知 Ana 可以开工，使用名为 ana-gateway 的 Gateway。  
9. Ana 编写并提交 HTTPRoute，配置允许的路径及对应的微服务，并通过 Route 附加流程把这些 HTTPRoute 绑定到 ana-gateway。  
10. 至此，请求到达负载均衡器后，会按 Ana 的路由规则转发到其应用。

这样，Chihiro 可在 Gateway 层统一强制 TLS 等政策，而 Ana 及其同事仍能掌控应用路由逻辑与灰度发布（如流量拆分）。

#### 单 Gateway 后承载多应用（Multiple applications behind a single Gateway）  

与基本南北向用例非常相似，但存在多个应用团队：

- Ana 的团队在 store 命名空间运营店铺应用  
- Allison 的团队在 site 命名空间运营官网

1. Ian 与 Chihiro 同样交付集群、GatewayClass 与共享 Gateway。  
2. Ana 与 Allison 各自独立部署工作负载与 HTTPRoute，并绑定到同一个 Gateway 资源。

再次体现关注点分离：Chihiro 在 Gateway 层统一强制 TLS 等政策；Ana 与 Allison 各自在独立命名空间运行应用，将 Route 附加到同一共享 Gateway，从而独立控制路由逻辑、流量拆分灰度等，而无需关心 Ian 与 Chihiro 负责的基础设施事务。

![gateway-api-multi-app.png](../../../assets/images/posts/gateway-api-multi-app.png)



#### 基本东西向（east/west）用例  

在这种场景下，Ana 已经构建了一个工作负载，并在一个**符合 GAMMA 规范的服务网格**中运行。她希望利用网格来保护自己的工作负载：

- 拒绝携带**错误 URL 路径**的调用；  
- 对任何请求**强制执行超时**。

具体分工如下：

- **Chihiro 与 Ian** 已提供**已安装好服务网格**的集群，Ana **无需再向他们提任何需求**。  
- Ana 编写一条 **HTTPRoute**，在其中定义**允许的路由和超时策略**，并把 `parentRef` 指向自己工作负载的 **Service**。  
- Ana 把这条 HTTPRoute **提交到与工作负载相同的 Namespace**。  
- 网格**立即自动生效**这条路由策略，无需额外操作。

通过这样的角色分离，Ana 就能**零阻塞地**利用服务网格的定制路由能力，而无需等待 Chihiro 或 Ian 的配合。


#### 网关 + 网格综合用例  

该场景相当于“**多应用共享一个 Gateway**”与“**基本东西向**”两者的结合：

1. **Chihiro & Ian**  
   负责准备集群、GatewayClass 以及 Gateway。

2. **Ana & Allison**  
   把各自的应用部署到合适的 Namespace，然后按需提交 HTTPRoute。

但因为有**服务网格**介入，出现了两个**关键变化**：

- 若 Chihiro 部署的网关控制器**默认按 Service 做路由**，则**必须重新配置为按 endpoint 路由**（GAMMA 仍在持续完善，但社区推荐 endpoint 路由）。  
- Ana / Allison 需要把 HTTPRoute **同时绑定到各自工作负载的 Service**，以配置**网格侧的东西向路由逻辑**。  
  这些 HTTPRoute 可以是**仅用于网格的独立资源**，也可以**同一条 HTTPRoute 既绑定 Gateway（南北向）又绑定 Service（东西向）**。

最终，通过这样清晰的职责划分：

- **Chihiro** 可在 Gateway 层统一实施**TLS、限频、审计**等集中策略；  
- **Ana & Allison** 则各自独立掌控**路由规则、流量灰度、超时重试**等细节，无论流量来自**南北向（外部→Gateway→Pod）**还是**东西向（Pod→Pod）**。