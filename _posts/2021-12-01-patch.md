---
title: Kubernetes Patch and Update 机制
tags: Kubernetes client-go
--- 

Kubernetes提供了两种更新资源的的机制: update和patch。本文将会介绍这这种方式的具体使用方法，区别和原理。
<!--more-->

## Kubernetes Update 机制

在kubernetes中，每个资源都有一个[resourceversion](https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions)，针对该资源的每次修改都会导致resourceversion的变化。借助resourceversion，Kubernetes Apiserver 采用了类似乐观锁的方式实现Update机制。

当要对某个资源做Update操作时，需要带上该资源的resourceversion，Apiserver会检查请求中的resourceversion是否与server端保存的最新resourceversion一致：
- 如果相同，则接受Update请求并改变server端该资源的的resourceversion
- 如果不相同，则返回有冲突的错误，此时由客户端决定接下去的步骤，例如是将错误直接返回给用户还是获取最新的版本然后重新发起Update请求。

Kubernetes Client中使用Update：
```
// Init kubernetes client here
k8sClient.Get(context.TODO(), types.NamespacedName{
	Namespace: "default",
	Name:      "my-sample",
}, object)

// Make changes to the objec here
k8sClient.Update(context.TODO(), object)
```

## Kubernetes Patch 机制

Update需要传输整个object到Apiserver，使用的是PUT方法。而Patch只需要传输变化的部分给Apiserver，Kubernetes 支持的Patch方式一共有四种：
- json patch
- json merge patch
- strategic merge patch
- apply patch
  - client-side apply patch
  - server-side apply patch

### Json Patch

[RFC6902](https://datatracker.ietf.org/doc/html/rfc6902)定义了一种用于表示一系列对于目标Json操作的Json结构，它适用于HTTP PATCH方法。media type `application/json-patch+json` 用于标识此类Patch。

Json patch是一系列json对象组成的列表，每一个json对象表示一个对目标json文件的操作，例如：
```
[
    { "op": "test", "path": "/a/b/c", "value": "foo" },
    { "op": "remove", "path": "/a/b/c" },
    { "op": "add", "path": "/a/b/c", "value": [ "foo", "bar" ] },
    { "op": "replace", "path": "/a/b/c", "value": 42 },
    { "op": "move", "from": "/a/b/c", "path": "/a/b/d" },
    { "op": "copy", "from": "/a/b/d", "path": "/a/b/e" }
]
```

操作按照它们在数组中出现的顺序依次生效。序列中的每个操作都应用于目标文档，生成的文档成为下一个操作的目标，直到成功应用所有操作或直到遇到错误条件。

可以使用的操作包括：
- add
- remove
- replace
- move
- copy
- test

由于Json patch中只包含一系列的操作而没有version字段，所以Apiserver没办法对Json patch施加乐观锁。

为了方便演示Kubernetes Client的Patch功能，可以预先定义：
```
type JsonPatch []PatchOperation
type PatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	From  string      `json:"from,omitempty"`
	Value interface{} `json:"value,omitempty"`
}
```

#### add

Add操作的作用：
1. 如果path是一个array的index，则value会插入到array中的相应位置，当前及其后元素的位置向后顺延。如果需要添加元素的位置>len(arrays)，则会报错。
2. 如果path是一个不存在的object member，则新member会被加入。
3. 如果path是一个已经存在的object member，则该member的值会被改变。

此操作的目的是添加元素到现有的对象和数组，其目标位置可能不存在。通常这会触发错误使用指针的报错，但Json patch中的`add`会忽略该错误并添加指定的值。但是，对象本身或包含它的数组必须要存在。例如，目标位置为`/a/b`的`add`操作作用于此文档：
```
{ "a": { "foo": 1 } }
```
尽管`a`中不存在member `b`，但这并不会导致错误，因为`a`存在，member `b`将被添加到其值中。

但如果作用于下面的这个json文档，会导致错误的发生：
```
{ "q": { "bar": 2 } }
```
因为`a`不存在。

Kubernetes Client中使用Json Patch对Deployment做Add操作：
```
jsonPatch := JsonPatch{
	PatchOperation{
		Op:    "add",
		Path:  "/metadata/annotations/provider-name",
		Value: "my-provider",
	},
}
patchByte, err := json.Marshal(jsonPatch)
clientset, err := kubernetes.NewForConfig(ctrl.GetConfigOrDie())

deployment, err := clientset.AppsV1().Deployments(apiv1.NamespaceDefault).Patch(context.TODO(), "my-deployment", types.JSONPatchType, patchByte, metav1.PatchOptions{})
```

#### remove

Remove操作会移除目标位置的元素。目标位置必须存在才能使操作成功。例如：
```
{ "op": "remove", "path": "/a/b/c" }
```

如果从数组中删除一个元素，则指定索引右面的任何元素都将向左移动一个位置。

Kubernetes Client中使用Json Patch对Deployment做Remove操作：
```
jsonPatch := JsonPatch{
	PatchOperation{
		Op:    "remove",
		Path:  "/metadata/annotations/provider-name",
		Value: "my-provider",
	},
}
...
deployment, err := clientset.AppsV1().Deployments(apiv1.NamespaceDefault).Patch(context.TODO(), "my-deployment", types.JSONPatchType, patchByte, metav1.PatchOptions{})

```

#### replace

Replace操作会用新值替换目标位置处的值。操作对象必须包含一个`value`成员用来指定替换值。目标位置必须存在才能使操作成功。例如：
```
{ "op": "replace", "path": "/a/b/c", "value": 42 }
```

Replace操作在功能上可以等效于对某个元素执行`remove`操作，紧接着在与`remove`操作相同的位置执行`add`操作。

Kubernetes Client中使用Json Patch对Deployment做Remove操作：
```
jsonPatch := JsonPatch{
	PatchOperation{
		Op:    "replace",
		Path:  "/metadata/annotations/provider-name",
		Value: "my-provider",
	},
}
...
deployment, err := clientset.AppsV1().Deployments(apiv1.NamespaceDefault).Patch(context.TODO(), "my-deployment", types.JSONPatchType, patchByte, metav1.PatchOptions{})
```

#### move

Move操作移除指定位置的值并将其添加到目标位置。操作对象必须包含一个`from`成员，该值引用目标文档中要从中移动值的位置。`from`位置必须存在才能使操作成功。例如：
```
{ "op": "move", "from": "/a/b/c", "path": "/a/b/d" }
```

此操作在功能上可以等效于对某个元素执行`from`位置的`remove`操作，紧接着在目标位置执行`add`操作。

Kubernetes Client中使用Json Patch对Deployment做Move操作：
```
jsonPatch := JsonPatch{
	PatchOperation{
		Op:   "move",
		From: "/metadata/annotations/provider-name",
		Path: "/metadata/labels/provider-name",
	},
}
...
deployment, err := clientset.AppsV1().Deployments(apiv1.NamespaceDefault).Patch(context.TODO(), "my-deployment", types.JSONPatchType, patchByte, metav1.PatchOptions{})
```

#### copy

Copy操作将指定位置的值复制到目标位置。操作对象必须包含一个`from`成员，该值引用目标文档中要从中复制值的位置。`from`位置必须存在才能使操作成功。例如：
```
{ "op": "copy", "from": "/a/b/c", "path": "/a/b/e" }
```

此操作在功能上可以等效于对目标位置执行`add`操作添加`from`位置的值。

Kubernetes Client中使用Json Patch对Deployment做Copy操作：
```
jsonPatch := JsonPatch{
	PatchOperation{
		Op:   "copy",
		Path: "/metadata/annotations/provider-name",
		From: "/metadata/labels/provider-name",
	},
}
...
deployment, err := clientset.AppsV1().Deployments(apiv1.NamespaceDefault).Patch(context.TODO(), "my-deployment", types.JSONPatchType, patchByte, metav1.PatchOptions{})
```

#### test

Test操作测试目标位置处的值是否等于指定值。操作对象必须包含一个`value`成员，该成员表示要与目标位置的值进行比较的值。目标位置的值和json类型必须等于`value`中指定的值和json类型：

- string: 包含相同数量的Unicode字符并且逐字节相等.
- number: 值在数字上相等。
- arrays: 包含相同数量的值，并且每个值都可以被认为等于另一个数组中相应位置的值
- objects: 包含相同数量的成员，并且较它们的键和它们的值都相等。

例如：
```
{ "op": "test", "path": "/a/b/c", "value": "foo" }
```

Kubernetes Client中使用Json Patch对Deployment做Test操作：
```
jsonPatch := JsonPatch{
	PatchOperation{
		Op:    "test",
		Path:  "/metadata/annotations/provider-name",
		Value: "my-provider",
	},
}
...
deployment, err := clientset.AppsV1().Deployments(apiv1.NamespaceDefault).Patch(context.TODO(), "my-deployment", types.JSONPatchType, patchByte, metav1.PatchOptions{})
}
```

### Json Merge Patch

[RFC7386](https://datatracker.ietf.org/doc/html/rfc7386)定义了JSON Merge Patch的格式和处理规则。它适用于HTTP PATCH方法。media type `merge-patch+json`用于标识此类Patch。

JSON Merge Patch文档描述了对目标 JSON 文档所做的更改，使用的语法与正在修改的文档非常相似。Merge Patch文档的接收者通过将所提供Patch的内容与目标文档的当前内容进行比较来确定所请求的确切更改集。

- 如果提供的Patch包含未出现在目标中的成员，则会添加这些成员。
- 如果目标确实包含该成员，则替换该值。
- Patch中的null被赋予特殊含义以指示删除目标中的现有值。

例如, 原始json文件的内容如下：
```
{
    "a": "b",
    "c": {
        "d": "e",
        "f": "g"
    }
}
```
使用如下Merge Patch可以更改a的值并且删除元素f，其他元素保持不变:
```
{
    "a":"z",
    "c": {
       "f": null
    }
}
```

该方法有几点需要注意:
- 如果Patch内容不是对象，则将用整个Patch替换目标。这导致无法仅替换数组中的某些值。
- 仅适用于不使用显式Null的JSON文档的修改。

Kubernetes Client中使用Json Merge Patch：
```
jsonPatch := []byte(`{
	"metadata":{
		"annotations": {
			"provider-name": "my-provider"
		}
	}
}
`)
...
deployment, err := clientset.AppsV1().Deployments(apiv1.NamespaceDefault).Patch(context.TODO(), "my-deployment", types.MergePatchType, jsonPatch, metav1.PatchOptions{})
```

#### Optimistic Lock
Kubernetes Client 在json merge patch功能中支持乐观锁，但是需要在patch文件中指明resourceversion，例如：
```
jsonPatch := []byte(`{
	"metadata":{
		"annotations": {
			"provider-name": "my-provider"
		},
		"resourceVersion": "1501414"
	}
}
`)
```

只有Merge Patch中的resourceversion与Apiserver中最新的resourceversion相同时，patch才会成功。如果不相同，则返回错误`Operation cannot be fulfilled on deployments.apps "my-deployment": the object has been modified; please apply your changes to the latest version and try again`

### Apply Patch

Apply Patch是一种声明式的Patch方式，主要分为Server-side和Client-side两种方式。

#### Client Side Apply Patch

在使用kubectl apply命令时，默认会采用Client Side Apply Patch的方式，如果想切换至Server Side Apply Patch则需要指定`--server-side=true`。Client Side Apply Patch默认使用Strategic Merge Patch。

##### Last Applied Configuration
kubectl apply 命令将配置文件的内容写入到`kubectl.kubernetes.io/last-applied-configuration` annotaion中。 这些内容用来识别配置文件中已经移除的、因而也需要从Apiserver中保存的配置中删除的字段。 用来计算要删除或设置哪些字段的步骤如下：

1. 计算要删除的字段，即在 last-applied-configuration 中存在但在 配置文件中不再存在的字段。
2. 计算要添加或设置的字段，即在配置文件中存在但其取值与现时配置不同的字段。

关于Client Side Apply Patch的使用，可以参考：[declarative-config](https://kubernetes.io/docs/tasks/manage-kubernetes-objects/declarative-config/)

#### Server Side Apply Patch

顾名思义，Server Side Apply Patch是在server端执行apply patch操作，它适用于HTTP PATCH方法。media type `application/apply-patch+yaml`用于标识此类Patch。

##### Field Management
相对于通过Client Side Apply Patch的last-applied，Server Side Apply Patch使用了一种更具声明式特点的方法： 它持续的跟踪用户的字段管理权，而不仅仅是最后一次的执行状态。这就意味着，需要向外暴露关于用哪一个字段管理器负责管理对象中的哪个字段的这类信息。

当使用Server Side Apply Patch尝试着去改变一个被其他人管理的字段，如果没有使用force参数的情况下会导致请求被拒绝。如果两个或以上的调用者均把同一个字段设置为相同值，他们将共享此字段的所有权。 后续任何改变共享字段值的尝试，不管由那个应用者发起，都会导致冲突。只需从配置文件中删除该字段即可放弃共享字段的所有权。字段管理的信息存储在`managedFields`字段中，该字段是对象的 metadata 中的一部分。

Server Side Apply Patch创建的一个例子：
```
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
  namespace: default
  labels:
    test-label: test
  managedFields:
  - manager: kubectl
    operation: Apply
    apiVersion: v1
    time: "2010-10-10T0:00:00Z"
    fieldsType: FieldsV1
    fieldsV1:
      f:metadata:
        f:labels:
          f:test-label: {}
      f:data:
        f:key: {}
data:
  key: some value
```

##### Conflicts

当在执行 Apply 改变一个字段时，而恰巧该字段被其他用户声明过主权时，会导致冲突。这可以防止一个应用者不小心覆盖掉其他用户设置的值。冲突发生时，应用者有三种办法来解决它：

- 覆盖前值，成为唯一的管理器：如果打算覆盖该值（或应用者是一个自动化部件，比如控制器），应用者应该设置参数 `force` 为 `true`，然后再发送一次请求。这将强制操作成功，改变字段的值，从所有其他管理器的 managedFields 条目中删除该字段。
- 不覆盖前值，放弃管理权：如果应用者不再关注该字段的值，可以从配置文件中删掉该字段，再重新发送请求。
- 不覆盖前值，成为共享的管理器：如果应用者仍然关注字段值，并不想覆盖它，他们可以在配置文件中把字段的值改为和服务器对象一样，再重新发送请求。

需要注意的是：
- 在冲突发生的时候，只有apply操作失败，而update 则不会。 
- apply操作必须通过提供一个 fieldManager 查询参数来标识自身， 而此查询参数对于 update 操作则是可选的。
- 当使用 apply 命令时，请求中的对象不能含有 managedFields字段。

##### Manager

在apply操作中，必须指定fieldManager参数，该参数的值会赋值给managedFields.manager，同时也用来做冲突检查。对于其他操作managedFields.manager字段值通常是从user-agent中计算出来的。kubectl操作产生的manager字段为`kubectl`。

##### Example
```
jsonPatch := []byte(`{
	"apiVersion": "apps/v1",
	"kind": Deployment,
	"metadata":{
		"annotations": {
			"provider-name": "my-provider"
		}
	}
}`)

deployment, err := clientset.AppsV1().Deployments(apiv1.NamespaceDefault).Patch(context.TODO(), "my-deployment", types.ApplyPatchType, jsonPatch, metav1.PatchOptions{
	FieldManager: "test-controller",
})
```

使用`kc get deployment my-deployment -o yaml --show-managed-fields` 查看`managedFields`:
```
managedFields:
- apiVersion: apps/v1
  fieldsType: FieldsV1
  fieldsV1:
    f:metadata:
      f:annotations:
        f:provider-name: {}
  manager: test-controller
  operation: Apply
```

可见provider-name的manager是test-controller。当使用另外一个manager对该字段更新时：
```
jsonPatch := []byte(`{
	"apiVersion": "apps/v1",
	"kind": Deployment,
	"metadata":{
		"annotations": {
			"provider-name": "my-provider-update"
		}
	}
}`)
deployment, err := clientset.AppsV1().Deployments(apiv1.NamespaceDefault).Patch(context.TODO(), "my-deployment", types.ApplyPatchType, jsonPatch, metav1.PatchOptions{
	FieldManager: "another-controller",
})
```

Apiserver 返回错误`Apply failed with 1 conflict: conflict with "test-controller": .metadata.annotations.provider-name`

##### Optimistic Lock
Kubernetes Client 在Server side apply patch功能中支持乐观锁，但是需要在patch文件中指明resourceversion，例如：
```
jsonPatch := []byte(`{
	"apiVersion": "apps/v1",
	"kind": Deployment,
	"metadata":{
		"annotations": {
			"provider-name": "my-provider"
		},
		"resourceVersion": "1501414"
	}
}
`)
```

关于Client Side Apply Patch的使用，可以参考：[server-side-apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/)

### Strategic Merge Patch

trategic Merge Patch并没有一个通用的 RFC 标准，而是Kubernetes单独定义的Patch行为，不过相比Json/Json Merge Patch而言却更为强大的。它适用于HTTP PATCH方法。media type `application/strategic-merge-patch+json`用于标识此类Patch。

Strategic Merge Patch的行为会由Json字段上的标签决定，例如根据其Patch策略替换或合并列表。Patch策略由代码中字段标签中的patchStrategy值指定。比如PodSpec结构体的Containers字段有一个值为merge的patchStrategy:
```
type PodSpec struct {
  ...
  Containers []Container `json:"containers" patchStrategy:"merge" patchMergeKey:"name" ...`
```

具体使用方式同Json merge patch类似，只不过其merge行为由patchStrategy控制：
```
jsonPatch := []byte(`{
  "spec": {
    "template": {
      "spec":{
        "containers": [
          {
          	"name": "patch-demo-ctr-2",
          	"image": "redis"
          }
        ]        
      }
    }
  }
}`)
...
deployment, err := clientset.AppsV1().Deployments(apiv1.NamespaceDefault).Patch(context.TODO(), "my-deployment", types.StrategicMergePatchType, jsonPatch, metav1.PatchOptions{})
```

#### Optimistic Lock
Kubernetes Client在Strategic merge patch功能中支持乐观锁，但是需要在patch文件中指明resourceversion，例如：
```
jsonPatch := []byte(`{
	"metadata":{
		"resourceVersion": "1501414"
	},
	"spec": {
		"template": {
			"spec":{
				"containers": [
					{
						"name": "patch-demo-ctr-2",
						"image": "redis"
					}
				]        
			}
		}
	}
}
`)
```

关于patchStrategy的使用，可以参考：[update-api-object-kubectl-patch](https://kubernetes.io/docs/tasks/manage-kubernetes-objects/update-api-object-kubectl-patch/)

