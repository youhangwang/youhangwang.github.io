---
title: OCM Placement Extensible Scheduling 
tags: OCM Placement
--- 

在实现基于Placement的调度时，在某些情况下，prioritizer需要额外的数据来计算托管集群的分数。例如，需要根据来自集群的资源监控数据进行调度。

所以需要要一种可扩展的方式来支持基于自定义分数的调度。
<!--more-->

## User Stories
- 用户可以使用从每个托管集群推送的数据来选择集群。
  - 在每个托管集群上，都有一个定制的代理监控集群的资源使用情况（例如 CPU ）。它将计算分数并将结果推送到Hub Cluster。
  - 作为用户，可以配置 placement yaml 以使用此分数来选择集群。
- 用户可以使用Hub Cluster收集的指标来选择集群。
  - 在hub cluster上，有一个自定义代理可以从 Thanos 获取指标（例如集群可分配内存）。它将为每个集群生成一个分数。
  - 作为用户，可以配置 place yaml 以使用此分数来选择集群。
- 灾难恢复, 工作负载可以自动切换到 其他可用集群。
  - 一个用户有两个集群，一个主集群和一个备份集群，存储可以从主集群同步到备份集群。
  - 作为用户，首先在主集群上部署工作负载。并且当主集群宕机时，工作负载会自动切换到备份集群。
- 用户可以使用第三方控制器提供的数据来选择集群。
  - 第三方控制器可以综合评估集群的延迟、区域、iops 等，并根据集群可以提供的 SLA 对集群进行评级。
  - 作为用户，可以配置Placement yaml 以使用此分数来选择集群。

## Design Details

### AddOnPlacementScore API
`AddOnPlacementScore` 表示一个Manged Cluster上以系列的分值，例如
```
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope="Namespaced"
// +kubebuilder:subresource:status

// AddOnPlacementScore represents a bundle of scores of one managed cluster, which could be used by placement.
// AddOnPlacementScore is a namespace scoped resource. The namespace of the resource is the cluster namespace.
type AddOnPlacementScore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Status represents the status of the AddOnPlacementScore.
	// +optional
	Status AddOnPlacementScoreStatus `json:"status,omitempty"`
}

//AddOnPlacementScoreStatus represents the current status of AddOnPlacementScore.
type AddOnPlacementScoreStatus struct {
	// Conditions contain the different condition statuses for this AddOnPlacementScore.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Scores contain a list of score name and value of this managed cluster.
	// +listType=map
	// +listMapKey=name
	// +optional
	Scores []AddOnPlacementScoreItem `json:"scores,omitempty"`

	// ValidUntil defines the valid time of the scores.
	// After this time, the scores are considered to be invalid by placement. nil means never expire.
	// The controller owning this resource should keep the scores up-to-date.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=date-time
	// +optional
	ValidUntil *metav1.Time `json:"validUntil"`
}

//AddOnPlacementScoreItem represents the score name and value.
type AddOnPlacementScoreItem struct {
	// Name is the name of the score
	// +kubebuilder:validation:Required
	// +required
	Name string `json:"name"`

	// Value is the value of the score. The score range is from -100 to 100.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum:=-100
	// +kubebuilder:validation:Maximum:=100
	// +required
	Value int32 `json:"value"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AddOnPlacementScoreList is a collection of AddOnPlacementScore.
type AddOnPlacementScoreList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of AddOnPlacementScore
	Items []AddOnPlacementScore `json:"items"`
}
```

### Placement API

1. 在`PrioritizerConfig`中增加`ScoreCoordinate`字段，其代表prioritizer的配置和自定义分数。
2. 支持负数的权重值。

```
type Placement struct {
  ...
	Spec PlacementSpec `json:"spec"`
}

type PlacementSpec struct {
  ...

	// PrioritizerPolicy defines the policy of the prioritizers.
	// If this field is unset, then default prioritizer mode and configurations are used.
	// Referring to PrioritizerPolicy to see more description about Mode and Configurations.
	// +optional
	PrioritizerPolicy PrioritizerPolicy `json:"prioritizerPolicy"`
}

// PrioritizerPolicy represents the policy of prioritizer
type PrioritizerPolicy struct {
  ...

	// +optional
	Configurations []PrioritizerConfig `json:"configurations,omitempty"`
}

// PrioritizerConfig represents the configuration of prioritizer
type PrioritizerConfig struct {
	// Name will be removed in v1beta1 and replaced by ScoreCoordinate.BuiltIn.
	// If both Name and ScoreCoordinate.BuiltIn are defined, will use the value
	// in ScoreCoordinate.BuiltIn.
	// Name is the name of a prioritizer. Below are the valid names:
	// 1) Balance: balance the decisions among the clusters.
	// 2) Steady: ensure the existing decision is stabilized.
	// 3) ResourceAllocatableCPU & ResourceAllocatableMemory: sort clusters based on the allocatable.
	// +optional
	Name string `json:"name,omitempty"`

	// ScoreCoordinate represents the configuration of the prioritizer and score source.
	// +optional
	ScoreCoordinate *ScoreCoordinate `json:"scoreCoordinate,omitempty"`

	// Weight defines the weight of the prioritizer score. The value must be ranged in [-10,10].
	// Each prioritizer will calculate an integer score of a cluster in the range of [-100, 100].
	// The final score of a cluster will be sum(weight * prioritizer_score).
	// A higher weight indicates that the prioritizer weights more in the cluster selection,
	// while 0 weight indicates that the prioritizer is disabled. A negative weight indicates
	// wants to select the last ones.
	// +kubebuilder:validation:Minimum:=-10
	// +kubebuilder:validation:Maximum:=10
	// +kubebuilder:default:=1
	// +optional
	Weight int32 `json:"weight,omitempty"`
}

// ScoreCoordinate represents the configuration of the score type and score source
type ScoreCoordinate struct {
	// Type defines the type of the prioritizer score.
	// Type is either "BuiltIn", "AddOn" or "", where "" is "BuiltIn" by default.
	// When the type is "BuiltIn", need to specify a BuiltIn prioritizer name in BuiltIn.
	// When the type is "AddOn", need to configure the score source in AddOn.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=BuiltIn;AddOn
	// +kubebuilder:default:=BuiltIn
	// +required
	Type string `json:"type,omitempty"`

	// BuiltIn defines the name of a BuiltIn prioritizer. Below are the valid BuiltIn prioritizer names.
	// 1) Balance: balance the decisions among the clusters.
	// 2) Steady: ensure the existing decision is stabilized.
	// 3) ResourceAllocatableCPU & ResourceAllocatableMemory: sort clusters based on the allocatable.
	// +optional
	BuiltIn string `json:"builtIn,omitempty"`

	// When type is "AddOn", AddOn defines the resource name and score name.
	// +optional
	AddOn *AddOnScore `json:"addOn,omitempty"`
}

const (
	// Valid ScoreCoordinate type is BuiltIn, AddOn.
	ScoreCoordinateTypeBuiltIn string = "BuiltIn"
	ScoreCoordinateTypeAddOn   string = "AddOn"
)

// AddOnScore represents the configuration of the addon score source.
type AddOnScore struct {
	// ResourceName defines the resource name of the AddOnPlacementScore.
	// The placement prioritizer selects AddOnPlacementScore CR by this name.
	// +kubebuilder:validation:Required
	// +required
	ResourceName string `json:"resourceName"`

	// ScoreName defines the score name inside AddOnPlacementScore.
	// AddOnPlacementScore contains a list of score name and score value, ScoreName specify the score to be used by
	// the prioritizer.
	// +kubebuilder:validation:Required
	// +required
	ScoreName string `json:"scoreName"`
}
```
### Let placement prioritizer support rating clusters with the customized score of the CR
一个有效的`AddOnPlacementScore` CR如下所示：

```
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: AddOnPlacementScore
metadata:
  name: default
  namespace: cluster1
status:
  conditions:
  - lastTransitionTime: "2021-10-28T08:31:39Z"
    message: AddOnPlacementScore updated successfully
    reason: AddOnPlacementScoreUpdated
    status: "True"
    type: AddOnPlacementScoreUpdated
  validUntil: "2021-10-29T18:31:39Z"
  scores:
  - name: "cpuratio"
    value: 88
  - name: "memratio"
    value: 77
  - name: "cpu"
    value: 66
  - name: "mem"
    value: 55
```
在此示例中，`AddOnPlacementScore`资源名称为`default`，命名空间`cluster1`是托管集群命名空间。 `status.scores`包含一个分数项列表，例如，分数`cpuratio`的值为`88`。

作为用户，可以如下定义一个Placement，以使用`AddOnPlacementScore`中的分数对集群进行排序。

```
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: Placement
metadata:
  name: placement
  namespace: ns1
spec:
  numberOfClusters: 3
  prioritizerPolicy:
    mode: Exact
    configurations:
      - scoreCoordinate:
          type: BuildIn
          builtIn: Steady
        weight: 3
      - scoreCoordinate:
          type: AddOn
          addOn:
            resourceName: default
            scoreName: cpuratio
        weight: 1
```

在上面的示例中，placement定义了权重为 3 的内置优先级Steady，以及使用`default` `AddOnPlacementScore`中的`cpuratio`分数对集群进行排序。

创建此`placement.yaml`时，placement控制器使用`BuildIn`优先级`Steady`和`AddOn`分数`cpuratio`对集群进行排序。 `BuildIn`优先级调度行为与之前保持一致，`AddOn`类型优先级调度流程如下：

- First schedule
优先排序器将从`default` `AddOnPlacementScore`中获得自定义分数`cpuratio`，并计算每个托管集群的最终分数。

- Reschedule
`AddOnPlacementScore`中的分数会经常更新。调度程序不会watch `AddOnPlacementScore`中的变化并触发重新调度，而是定期（例如每 5 分钟）重新调度Placement。

#### What if no valid score is generated?

对于分数无效的情况，例如，某些托管集群没有创建`AddOnPlacementScore CR`，或者某些 CR 中缺少分数，或者分数已过期。优先级会给那些集群评分为 0，最终的Placement仍然是由每个优先级的总分做出决定的。

### How to maintain the lifecycle of the AddOnPlacementScore CRs?

- 第三方控制器应该在哪里运行？

第三部分控制器可以在Hub Cluster或托管集群上运行。就像user story 1 和 2 中描述的一样。

- 什么时候应该创建分数？

可以在存在`ManagedCluster`的情况下创建，也可以按需创建以减少Hub Cluster上的对象。

- 分数应该什么时候更新？

建议在更新`score`时设置`ValidUntil`，这样在长时间更新失败的情况下，placement controller 可以知道 score 是否仍然有效。过期的分数将在Placement控制器中被视为分数 0。

当监控数据发生变化时，分数会更新，或者至少需要在它过期之前对其进行更新。

- 如何计算分数。

分数必须在`-100`到`100`的范围内，需要在将分数更新到`AddOnPlacementScore`之前对其进行标准化。

例如，如果是Hub Cluster上的控制器来维护分数，它可以根据它收集的最大值/最小值进行标准化。而如果控制器在托管集群上运行，它可以使用合理的最大值/最小值来标准化分数，例如，如果集群可分配内存大于 100GB，则分数为 100，如果小于 1GB，则分数为 -100。