---
title: Kubernetes API的认证(Authn)和授权(Authz)
tags: Openshift Kubernetes Authn Authz
--- 

## Kubernetes API
可以通过使用 kubectl、client-go 或 REST 请求来访问 Kubernetes API。Kubernetes包含两种账户：
- 代表真实用户的账户：普通账户
- Service账户：Service Account

普通账户和Service Account都可以实现访问API的认证和授权。API request会经过多个阶段的访问控制才会被API Server处理，其中包含认证、授权以及准入控制（Admission Control）等。如下图所示：

![Image](../../../assets/images/posts/access-control-overview.svg){:.rounded}

本文只限于认证和授权的内容，对准入控制不做介绍。

### Authenticating

前文也提到过，Kubernetes包含两种用户：
- 由kubernetes管理的Service Account：
  - 可以由Kubernetes API创建和删除
  - Service Accounts与存储为Secrets的一组证书相关联，这些凭据被挂载到pod中，以便集群进程与Kubernetes API通信。
- 被外部或独立的Service所管理的普通用户。
  - 通过管理员分配的Private Key管理
  - 通过Keystone或者google account管理
  - 一个包含Username和Password列表的文件

Kubernetes 目前没有代表一个普通用户的对象，所以也不支持通过API调用添加和删除普通用户。

API Request与普通用户或Service Accounts相关联，或者被视为匿名请求。这意味着集群内部或外部的每个进程都必须在向 API Server发出请求时进行身份验证，或者被视为匿名用户。Kubernetes支持多种认证机制：
- 客户端证书
- 普通令牌
- 引导令牌
- JSON Web 令牌（JWT，用于Service Account）

这些认证机制可以同时开启多个，但只要其中一个通过即视为认证成功。认证组件只检查HTTP Request的Header或者客户端证书。如果认证成功，则用户的username会传入授权模块做进一步授权验证；对于认证失败的请求则返回HTTP 401。

#### 认证策略
当HTTP Request到达API Server，认证插件会尝试将以下属性与HTTP Request关联：
- UserName：普通用户的字符串。比如"kube-admin"或"xxxx@apiref.com"。
- UID：普通用户的字符串，比UserName更具有唯一性。
- Groups：一组字符串，将常用的user分组。
- Extra fields: 将一些有用的字符串信息映射成的列表。

这些属性的值对于认证系统都是不透明的，只有经过授权解释后才有意义。可以同时启用多个认证方法。但最少需开启两种：
- 为Service Accounts使用service account tokens方法。
- 至少一种认证普通用户的方法。

所有已认证用户都会被添加到system:authenticated组。

##### X509证书

使用API Server`–-client-ca-file=SOMEFILE`选项来启用客户端证书认证。引用的文件必须包含一个或者多个可以用来验证客户端证书的CA。如果客户端提交的证书验证通过，CN（common name）将被用作HTTP Request的用户名。在kubernetes 1.4版本，证书的Organization还被当作是用户的Group。

例如，使用openssl命令管理工具生成证书签名请求：
```
openssl req -new -key jbeda.pem -out jbeda-csr.pem -subj "/CN=jbeda/O=app1/O=app2"
```

这会创建一个使用用户名`jbeda`，属于用户组`app1`和`app2`的CSR。

##### Static Token File
启动静态Token文件需要配置API Server的`--token-auth-file=SOMEFILE`选项，目前，tokens会一直生效，需要重新启用 API server才能修改token list的内容。

token文件是至少包含3列的csv格式文件： 
- token 
- user name
- user uid
- 第四列为可选group。如果有多个group名，列必须用双引号包含其中，例如`token,user,uid,"group1,group2,group3"`

当http客户端使用bearer token认证时，API Server需要一个值为Bearer THETOKEN的Authorization header。例如: Token 31ada4fd-adec-460c-809a-9e56ceb75269会在HTTP header中按下面的方式呈现：
```
Authorization: Bearer 31ada4fd-adec-460c-809a-9e56ceb75269
```

##### Bootstrap Tokens

为了简化新集群的bootstrapping，Kubernetes包含一个动态管理的Bearer令牌类型，称为Bootstrap令牌。这种token作为Secret存储在kube-system namespace中，可以动态管理和创建。Controller Manager包含一个TokenCleaner Controller，Token到期时可以删除Bootstrap Tokens。

Tokens的形式是`[a-z0-9]{6}.[a-z0-9]{16}`。第一部分是Token ID，第二部分是Token Secret。可以在HTTP header中指定Token，如下所示：
```
Authorization: Bearer 781292.db7bc3a58fc5f07e
```

##### Service Account Tokens
Service Account是一种默认被启用的认证机制，使用经过签名的持有者令牌来验证HTTP Request。该插件可接受两个可选参数：
- service-account-key-file: 一个包含用来为持有者令牌签名的 PEM 编码密钥。 若未指定，则使用 API 服务器的 TLS 私钥。
- service-account-lookup: 如果启用，则从 API 删除的令牌会被回收。

Service Account通常由 API Server自动创建并通过 ServiceAccount Admission Controller 关联到集群中运行的Pod上。持有者令牌会挂载到 Pod 中可预知的位置，允许集群内进程与 API 服务器通信。 Service Account也可以使用 Pod Spec中的 serviceAccountName 字段显式地关联到 Pod 上。

Service Account所关联的Secret中会保存 API 服务器的公开的 CA 证书和一个已签名的 JSON Web 令牌（JWT）。

```
apiVersion: v1
data:
  ca.crt: <Base64 编码的 API 服务器 CA>
  namespace: ZGVmYXVsdA==
  token: <Base64 编码的持有者令牌>
kind: Secret
metadata:
  # ...
type: kubernetes.io/service-account-token
```

服务账号被身份认证后，所确定的用户名为`system:serviceaccount:<名字空间>:<服务账号>`， 并被分配到用户组`system:serviceaccounts`和 `system:serviceaccounts:<名字空间>`。
##### OpenID Connect Tokens

OpenID Connect是一种OAuth2认证方式，被某些OAuth2 Provider支持，例如 Azure、Salesforce 和 Google。该协议对OAuth2的主要扩充体现在有一个附加令牌会和访问令牌一起返回，这一令牌称作ID Token（id_token）。ID 令牌是一种由服务器签名的 JSON Web 令牌（JWT），其中包含一些可预知的字段，例如用户的邮箱地址，

要识别用户，身份认证组件使用 OAuth2 令牌响应 中的 id_token（而非 access_token）作为持有者令牌。 

![Image](../../../assets/images/posts/kubernetes-oidc-login.jpeg){:.rounded}


1. 登录到你的身份服务（Identity Provider）
2. 你的身份服务将为你提供 access_token、id_token 和 refresh_token
3. 在使用 kubectl 时，将 id_token 设置为 --token 标志值，或者将其直接添加到 kubeconfig 中
4. kubectl 将你的 id_token 放到一个称作 Authorization 的头部，发送给 API 服务器
5. API 服务器将负责通过检查配置中引用的证书来确认 JWT 的签名是合法的
6. 检查确认 id_token 尚未过期
7. 确认用户有权限执行操作
8. 鉴权成功之后，API Server向 kubectl 返回响应
9. kubectl 向用户返回信息

由于用来验证你是谁的所有数据都在 id_token 中，Kubernetes 不需要再去联系身份服务。在一个所有HTTP Request都是无状态的模型中，这一工作方式可以使得身份认证的解决方案更容易处理大规模请求。不过，此访问也有一些挑战：
1. Kubernetes 没有提供用来触发身份认证过程的 "Web 界面"。 不存在用来收集用户凭据的浏览器或用户接口，你必须自己先行完成对身份服务的认证过程。
2. id_token 令牌不可收回。因其属性类似于证书，其生命期一般很短（只有几分钟），所以，每隔几分钟就要获得一个新的令牌这件事可能很让人头疼。
3. 如果不使用 kubectl proxy 命令或者一个能够注入 id_token 的反向代理， 向 Kubernetes 控制面板执行身份认证是很困难的。

Kubernetes 并未提供 OpenID Connect 的身份服务。你可以使用现有的公共的 OpenID Connect 身份服务（例如 Google 或者 其他服务）。 或者你也可以选择自己运行一个身份服务，例如 CoreOS dex、Keycloak、CloudFoundry UAA 或者 Tremolo Security 的 OpenUnison。

##### Webhook Token Authentication
Webhook认证是用来认证 bearer token 的 hook。
- --authentication-token-webhook-config-file 是一个用来描述如何访问远程 webhook 服务的 kubeconfig 文件。
- --authentication-token-webhook-cache-ttl 缓存身份验证策略的时间。默认为两分钟。

配置文件使用 kubeconfig 文件格式。文件中的 ”user“ 指的是 API server 的 webhook，”clusters“ 是指远程服务。见下面的例子：
```
# Kubernetes API version
apiVersion: v1
# kind of the API object
kind: Config
# clusters refers to the remote service.
clusters:
  - name: name-of-remote-authn-service
    cluster:
      certificate-authority: /path/to/ca.pem         # CA for verifying the remote service.
      server: https://authn.example.com/authenticate # URL of remote service to query. 'https' recommended for production.

# users refers to the API server's webhook configuration.
users:
  - name: name-of-api-server
    user:
      client-certificate: /path/to/cert.pem # cert for the webhook plugin to use
      client-key: /path/to/key.pem          # key matching the cert

# kubeconfig files require a context. Provide one for the API server.
current-context: webhook
contexts:
- context:
    cluster: name-of-remote-authn-service
    user: name-of-api-server
  name: webhook
```
当客户端尝试使用 bearer token 与API server 进行认证时，认证 webhook 用包含该 token 的`TokenReview`对象查询远程服务。例如
```
{
  "apiVersion": "authentication.k8s.io/v1",
  "kind": "TokenReview",
  "spec": {
    # Opaque bearer token sent to the API server
    "token": "014fbff9a07c...",
   
    # Optional list of the audience identifiers for the server the token was presented to.
    # Audience-aware token authenticators (for example, OIDC token authenticators) 
    # should verify the token was intended for at least one of the audiences in this list,
    # and return the intersection of this list and the valid audiences for the token in the response status.
    # This ensures the token is valid to authenticate to the server it was presented to.
    # If no audiences are provided, the token should be validated to authenticate to the Kubernetes API server.
    "audiences": ["https://myserver.example.com", "https://myserver.internal.example.com"]
  }
}
```
远程服务将填写请求的 status 字段以指示登录成功。响应主体的 spec 字段被忽略，可以省略。成功验证后的 bearer token 将返回：
```
{
  "apiVersion": "authentication.k8s.io/v1",
  "kind": "TokenReview",
  "status": {
    "authenticated": true,
    "user": {
      # Required
      "username": "janedoe@example.com",
      # Optional
      "uid": "42",
      # Optional group memberships
      "groups": ["developers", "qa"],
      # Optional additional information provided by the authenticator.
      # This should not contain confidential data, as it can be recorded in logs
      # or API objects, and is made available to admission webhooks.
      "extra": {
        "extrafield1": [
          "extravalue1",
          "extravalue2"
        ]
      }
    },
    # Optional list audience-aware token authenticators can return,
    # containing the audiences from the `spec.audiences` list for which the provided token was valid.
    # If this is omitted, the token is considered to be valid to authenticate to the Kubernetes API server.
    "audiences": ["https://myserver.example.com"]
  }
}
```
失败会返回：
```
{
  "apiVersion": "authentication.k8s.io/v1",
  "kind": "TokenReview",
  "status": {
    "authenticated": false,
    # Optionally include details about why authentication failed.
    # If no error is provided, the API will return a generic Unauthorized message.
    # The error field is ignored when authenticated=true.
    "error": "Credentials are expired"
  }
}
```
##### Authenticating Proxy
可以配置 API server 从HTTP Request的header中识别用户，例如`X-Remote-User`。这样的设计是用来与HTTP Request header 的验证代理结合使用。
- --requestheader-username-headers: 必需提供，大小写敏感。按header名称和顺序检查用户标识。第一个包含值的header的值将被作为用户名。
- --requestheader-group-headers: 1.6 以上版本。可选。大小写敏感。建议为 “X-Remote-Group”。按 header 名称和顺序检查用户组。所有指定的 header 中的所有值都将作为组名。
- --requestheader-extra-headers-prefix: 1.6 以上版本。可选，大小写敏感。建议为 “X-Remote-Extra-”。header prefix可用于查找有关用户的额外信息（通常由配置的授权插件使用）。以任何指定的前缀开头的 header 都会删除前缀，header名称的其余部分将成为extra key，而 header 值则是extra value。

例如下面的配置：
```
--requestheader-username-headers=X-Remote-User
--requestheader-group-headers=X-Remote-Group
--requestheader-extra-headers-prefix=X-Remote-Extra-
```

下面的请求：
```
GET / HTTP/1.1
X-Remote-User: fido
X-Remote-Group: dogs
X-Remote-Group: dachshunds
X-Remote-Extra-Acme.com%2Fproject: some-project
X-Remote-Extra-Scopes: openid
X-Remote-Extra-Scopes: profile
```

将会产生下面的user info：
```
name: fido
groups:
- dogs
- dachshunds
extra:
  acme.com/project:
  - some-project
  scopes:
  - openid
  - profile
```

为了防止恶意程序发送这些header，Proxy需要在验证HTTP Request header之前向API server提供有效的客户端证书，以对照指定的 CA 进行验证。
- --requestheader-client-ca-file: 必需。PEM 编码的证书包。在检查用户名的请求 header 之前，必须针对指定文件中的证书颁发机构提交并验证有效的客户端证书。
- --requestheader-allowed-names 可选。Common Name （cn）列表。如果设置了，则在检查用户名的请求 header 之前， 必须提供指定列表中 Common Name（cn）的有效客户端证书。如果为空，则允许使用任何 Common Name。

#### Anonymous requests
启用匿名请求时，未被其他已配置身份验证方法拒绝的请求将被视为匿名请求，并给予system:anonymous的用户名和system:unuthenticated的组名。例如，在配置了令牌认证和启用了匿名访问的服务器上，提供无效的bearer token的请求将收到 401 Unauthorized 错误，没有提供bearer token的请求将被视为匿名请求。

在 1.6+ 版本中，如果使用AlwaysAllow以外的授权模式，则默认启用匿名访问，并且可以使用API Server的`--anonymous-auth=false`选项禁用。从 1.6 开始，ABAC 和 RBAC 需要明确授权 system:annoymous 或 system:unauthenticated 组，因此授予对 * 用户或 * 组访问权的传统策略规则不包括匿名用户。

#### 用户模拟

用户可以通过模拟header充当另一个用户。该请求会覆盖请求认证的用户信息。例如，管理员可以使用此功能通过暂时模拟其他用户并查看请求是否被拒绝来调试授权策略。模拟请求首先认证为请求用户，然后切换到模拟的用户信息。

1. 用户使用他们的凭证和模拟 header 进行 API 调用。
2. API server 认证用户
3. API server 确保经过身份验证的用户具有模拟权限。
4. 请求用户的信息被替换为模拟值
5. 请求被评估，授权作用于模拟的用户信息。

以下 HTTP header 可用于执行模拟请求：

- Impersonate-User：充当的用户名
- Impersonate-Group：作为组名。可以多次使用来设置多个组。可选的，需要 “Impersonate-User”
- Impersonate-Extra-( extra name )：用于将额外字段与用户关联的动态 header。可选。需要 “Impersonate-User”
- Impersonate-Uid：v1.22+，一个 unique identifier用来表示被模仿的用户。可选。需要 “Impersonate-User”

例如：
```
Impersonate-User: jane.doe@example.com
Impersonate-Extra-dn: cn=jane,ou=engineers,dc=example,dc=com
Impersonate-Extra-acme.com%2Fproject: some-project
Impersonate-Extra-scopes: view
Impersonate-Extra-scopes: development
Impersonate-Uid: 06f6ce97-e2c5-4ab8-7ba5-7654dd08d52b
```

为模仿用户、组或设置额外字段，模拟用户必须能够对正在模拟的属性的种类（“用户”，“组”等）执行“模拟”动词。对于启用了 RBAC 授权插件的集群，以下 ClusterRole 包含设置用户和组模拟 header 所需的规则：
```
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: impersonator
rules:
- apiGroups: [""]
  resources: ["users", "groups", "serviceaccounts"]
  verbs: ["impersonate"]
```

#### Client-go credential Plugin
`k8s.io/client-go`及使用它的工具（如 kubectl 和 kubelet）可以执行某个外部的命令来获得用户的credentials。这一特性是为了客户端集成那些与k8s.io/client-go不支持的身份认证协议（LDAP、 Kerberos、OAuth2、SAML 等）。插件的实现特定于协议的逻辑，之后返回不透明的credential以供使用。 几乎在所有的credential plugin使用场景中，都需要在Server端存在一个[webhook token authenticator](#webhook-token-authentication)负责解析客户端插件所生成的credential。

设想这样一个应用场景：某组织运行着一个外部的服务，能够将特定用户的LDAP credential转换成Token。此服务还能够对[webhook token authenticator](#webhook-token-authentication)的请求做出响应以验证所提供的令牌。用户需要在自己的工作站上安装一个credential plugin。

要对请求认证身份时：
1. 用户发出 kubectl 命令。
2. credential plugin提示用户输入 LDAP credential，并与外部服务交互，获得令牌。
3. credential plugin将令牌返回给 client-go，后者将其用作持有者令牌提交给 API Server。
4. API Server使用webhook token authenticator向外部服务发出`TokenReview`请求。
5. 外部服务检查令牌上的签名，返回用户的用户名和用户组信息。

### Authorization

HTTP Request在认证成功之后会进入到授权模块。Kubernetes 要求鉴权请求中存在 REST API 常见的属性。 这意味着 Kubernetes 鉴权还适用于现有的组织或云提供商的访问控制系统， 除了 Kubernetes API 之外，它还可以处理其他 API。

Kubernetes 使用 API Server对API Request进行鉴权。它根据所有策略评估全部的请求属性来决定允许或拒绝请求。 一个 API Request必须被某些策略允许才能继续。 这意味着默认情况下拒绝权限。当系统配置了多个鉴权模块时，Kubernetes 将按顺序使用每个模块。 如果任意鉴权模块批准或拒绝请求，则立即返回该决定，并且不会与其他鉴权模块协商。 如果所有模块对请求没有意见，则拒绝该请求。 被拒绝响应返回 HTTP 状态代码 403。

Kubernetes只会对如下API Request的属性做检查：
- user: 身份验证期间提供的 user
- group: 经过身份验证的用户所属的组名列表
- extra: 由身份验证层提供的任意字符串键到字符串值的映射
- API: 指示请求是否针对 API 资源
- Request path: 各种非资源端点的路径，如 /api 或 /healthz
- API request verb: API 动词 get、list、create、update、patch、watch、 proxy、redirect、delete 和 deletecollection。
- HTTP request verb: HTTP 动词 get、post、put 和 delete 用于非资源请求。
- Resource: 正在访问的资源的 ID 或名称（仅限资源请求）- 对于使用 get、update、patch 和 delete 动词的资源请求，你必须提供资源名称。
- Subresource: 正在访问的子资源（仅限资源请求）
- Namespace: 正在访问的对象的名称空间（仅适用于名字空间资源请求）。
- API group: 正在访问的 API 组 （仅限资源请求）。空字符串表示核心 API 组。

`authorization.k8s.io` API Group将 API Server鉴权公开给外部服务。该组中的资源包括：
- SubjectAccessReview: 对任意用户的访问进行评估，当鉴权决策被委派给 API Server时很有用。例如，kubelet 和扩展 API Server使用它来确定用户对自己的 API 的访问权限。
- SelfSubjectAccessReview: 对当前用户的访问进行评估。
- LocalSubjectAccessReview: 与 SubjectAccessReview 类似，但仅限于特定的Namespace。
- SelfSubjectRulesReview: 返回用户可在名字空间内执行的操作集的审阅。 用户可以快速汇总自己的访问权限，或者用于UI中的隐藏/显示动作。

可以通过创建普通的 Kubernetes 资源来查询这些 API，其中返回对象的响应 "status" 字段是查询的结果。
```
kubectl create -f - -o yaml << EOF
apiVersion: authorization.k8s.io/v1
kind: SelfSubjectAccessReview
spec:
  resourceAttributes:
    group: apps
    name: deployments
    verb: create
    namespace: dev
EOF

apiVersion: authorization.k8s.io/v1
kind: SelfSubjectAccessReview
metadata:
  creationTimestamp: null
spec:
  resourceAttributes:
    group: apps
    name: deployments
    namespace: dev
    verb: create
status:
  allowed: true
  denied: false
```

Kubernetes支持多种授权模块，如ABAC模式、RBAC模式和Webhook模式。当管理员创建集群时，他们将会配置在API Server中使用的授权模块。如果配置了多个授权模块，Kubernetes会检查每个模块，当其中任何授权模块通过，则授权成功，如果所有模块都拒绝了该请求，则授权失败（HTTP 403）。授权模块包括：
- Node: 一个专用鉴权组件，根据调度到 kubelet 上运行的 Pod 为 kubelet 授予权限。 了解有关使用节点鉴权模式的更多信息，请参阅节点鉴权。
- ABAC: 基于属性的访问控制（ABAC）定义了一种访问控制范型，通过使用将属性组合在一起的策略，将访问权限授予用户。策略可以使用任何类型的属性（用户属性、资源属性、 对象，环境属性等）。
- RBAC: 基于角色的访问控制（RBAC）是一种基于用户的角色来管理其对计算机或网络资源的访问的方法。
  - 被启用之后，RBAC（基于角色的访问控制）使用 rbac.authorization.k8s.io API 组来驱动鉴权决策，从而允许管理员通过 Kubernetes API 动态配置权限策略。
  - 要启用 RBAC，请使用 --authorization-mode = RBAC 启动 API 服务器。
- Webhook: 实现 WebHook 的 Web 应用程序会在发生某些事情时将消息发布到 URL。

#### RBAC
基于角色（Role）的访问控制（RBAC）是一种基于组织中用户的角色来调节控制对计算机或网络资源的访问的方法。RBAC鉴权机制使用 rbac.authorization.k8s.io API 组 来驱动鉴权决定，允许你通过 Kubernetes API 动态配置策略。

要启用 RBAC，在启动 API 服务器 时将 --authorization-mode 参数设置为一个逗号分隔的列表并确保其中包含 RBAC。
```
kube-apiserver --authorization-mode=Example,RBAC
```

RBAC API 声明了四种 Kubernetes 对象：Role、ClusterRole、RoleBinding 和 ClusterRoleBinding。你可以像使用其他 Kubernetes 对象一样， 通过类似 kubectl 这类工具描述对象, 或修改对象。

#### ABAC
基于属性的访问控制（Attribute-based access control - ABAC）定义了访问控制范例，其中通过使用将属性组合在一起的策略来向用户授予访问权限。

基于 ABAC 模式，可以这样指定策略文件 --authorization-policy-file=SOME_FILENAME。如果现有策略已经声明该用户具有完成请求的权限，则该请求将被授权。

例如，如果 Bob 有以下策略，那么他只能在 projectCaribou 名称空间中读取 Pod:
```
{
    "apiVersion": "abac.authorization.kubernetes.io/v1beta1",
    "kind": "Policy",
    "spec": {
        "user": "bob",
        "namespace": "projectCaribou",
        "resource": "pods",
        "readonly": true
    }
}
```
如果Bob执行以下请求，那么请求会被鉴权，因为允许他读取 projectCaribou 名称空间中的对象：
```
{
  "apiVersion": "authorization.k8s.io/v1beta1",
  "kind": "SubjectAccessReview",
  "spec": {
    "resourceAttributes": {
      "namespace": "projectCaribou",
      "verb": "get",
      "group": "unicorn.example.org",
      "resource": "pods"
    }
  }
}
```

- 如果 Bob 在 projectCaribou 名字空间中请求写（create 或 update）对象，其鉴权请求将被拒绝。 
- 如果 Bob 在其它名字空间中请求读取（get）对象，其鉴权也会被拒绝。


#### Webhook
当在判断用户权限时，Webhook 模式会使 Kubernetes 查询外部的 REST 服务。
Webhook 模式需要一个 HTTP 配置文件，通过 --authorization-webhook-config-file=SOME_FILENAME 的参数声明。配置文件的格式使用 kubeconfig。在文件中，"users" 代表着 API 服务器的 webhook，而 "cluster" 代表着远程服务。使用 HTTPS 客户端认证的配置例子：
```
# Kubernetes API 版本
apiVersion: v1
# API 对象种类
kind: Config
# clusters 代表远程服务。
clusters:
  - name: name-of-remote-authz-service
    cluster:
      # 对远程服务进行身份认证的 CA。
      certificate-authority: /path/to/ca.pem
      # 远程服务的查询 URL。必须使用 'https'。
      server: https://authz.example.com/authorize

# users 代表 API 服务器的 webhook 配置
users:
  - name: name-of-api-server
    user:
      client-certificate: /path/to/cert.pem # webhook plugin 使用 cert
      client-key: /path/to/key.pem          # cert 所对应的 key

# kubeconfig 文件必须有 context。需要提供一个给 API 服务器。
current-context: webhook
contexts:
- context:
    cluster: name-of-remote-authz-service
    user: name-of-api-server
  name: webhook
```

在做认证决策时，API Server会 POST 一个 JSON 序列化的 authorization.k8s.io/v1beta1 SubjectAccessReview 对象来描述这个动作。这个对象包含了描述用户请求的字段，同时也包含了需要被访问资源或请求特征的具体信息。API 服务器还必须启用 authorization.k8s.io/v1beta1 API 扩展组 (--runtime-config=authorization.k8s.io/v1beta1=true)。

一个请求内容的例子：
```
{
  "apiVersion": "authorization.k8s.io/v1beta1",
  "kind": "SubjectAccessReview",
  "spec": {
    "resourceAttributes": {
      "namespace": "kittensandponies",
      "verb": "get",
      "group": "unicorn.example.org",
      "resource": "pods"
    },
    "user": "jane",
    "group": [
      "group1",
      "group2"
    ]
  }
}
```
```
{
  "apiVersion": "authorization.k8s.io/v1beta1",
  "kind": "SubjectAccessReview",
  "status": {
    "allowed": true
  }
}
```

#### Node
节点鉴权是一种特殊用途的鉴权模式，专门对 kubelet 发出的 API 请求进行鉴权。

节点鉴权器允许 kubelet 执行 API 操作。包括：
- 读取操作：
  - services
  - endpoints
  - nodes
  - pods
  - secrets、configmaps、pvcs 以及绑定到 kubelet 节点的与 pod 相关的持久卷
- 写入操作：
  - 节点和节点状态（启用 NodeRestriction 准入插件以限制 kubelet 只能修改自己的节点）
  - Pod 和 Pod 状态 (启用 NodeRestriction 准入插件以限制 kubelet 只能修改绑定到自身的 Pod)
  - 事件
- 鉴权相关操作：
  - 对于基于 TLS 的启动引导过程时使用的 certificationsigningrequests API 的读/写权限
  - 为委派的身份验证/授权检查创建 tokenreviews 和 subjectaccessreviews 的能力

为了获得节点鉴权器的授权，kubelet 必须使用一个凭证以表示它在 system:nodes 组中，用户名为 system:node:<nodeName>。 上述的组名和用户名格式要与 kubelet TLS 启动引导过程中为每个 kubelet 创建的标识相匹配。

要启用节点授权器，请使用 --authorization-mode = Node 启动 apiserver。

### API Server 端口和IP
前面的讨论适用于发送到 API Server的安全端口的请求（典型情况）。API Server实际上可以在 2 个端口上提供服务：
- localhost 端口:
  - 用于测试和引导，以及主控节点上的其他组件（调度器，控制器管理器）与 API 通信
  - 没有 TLS
  - 默认为端口 8080，使用 --insecure-port 进行更改
  - 默认 IP 为 localhost，使用 --insecure-bind-address 进行更改、
  - 请求会绕过Authn和Authz
  - 请求会被准入控制模块处理
  - 受需要访问主机权限的保护
- 安全端口”：
  - 尽可能使用
  - 使用 TLS。 用 --tls-cert-file 设置证书，用 --tls-private-key-file 设置密钥
  - 默认端口 6443，使用 --secure-port 更改
  - 默认 IP 是第一个非本地网络接口，使用 --bind-address 更改
  - 请求须经身份认证和鉴权组件处理
  - 请求须经准入控制模块处理
  - 身份认证和鉴权模块运行


## Openshift API
### Authenticating
### Authorization
