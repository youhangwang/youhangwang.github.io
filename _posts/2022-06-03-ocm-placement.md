---
title: OCM Placement
tags: OCM Placement
---

OCM中有两类Placement API：
- PlacementRule
- Placement
  
其中PlacmentRule出现的时间比较早，被应用到GRC/App/Observability等组件中。但是其并没有定义在OCM 的API repo中，为了扩展其通用性，提出了一个类似于PlacementRule的新API：Placement。同时会有一个通用的Controller消费该API，用以从多个ManagedClusterSet中选择某几个ManagedCluster，并将结果保存在placement decisions中。

Placement API可以被其他的controller使用，以决定将manifest部署在哪些Cluster中。

## Placement API

- [Placement API](./2022-04-17-ocm.md#placement)
- [Placement API Spec](https://github.com/open-cluster-management-io/api/blob/main/cluster/v1beta1/types_placement.go)
- [Placement Decision Spec](https://github.com/open-cluster-management-io/api/blob/main/cluster/v1beta1/types_placementdecision.go)

## [Placement Controller](https://github.com/open-cluster-management-io/placement)

Placement Controller 位于Hub Cluster上。程序的主要入口位于[manager](https://github.com/open-cluster-management-io/placement/blob/main/pkg/controllers/manager.go)

主要包括：
- plugins
- scheduler
- schedulingController
- schedulingControllerResync
- debugger

### Plugins
Plugins是一个接口：
```
// Plugin is the parent type for all the scheduling plugins.
type Plugin interface {
	Name() string
	// Set is to set the placement for the current scheduling.
	Description() string
	// RequeueAfter returns the requeue time interval of the placement
	RequeueAfter(ctx context.Context, placement *clusterapiv1beta1.Placement) PluginRequeueResult
}
```

Placement使用的Filter和Prioritizer也都是plugin的一种：
```
// Fitler defines a filter plugin that filter unsatisfied cluster.
type Filter interface {
	Plugin

	// Filter returns a list of clusters satisfying the certain condition.
	Filter(ctx context.Context, placement *clusterapiv1beta1.Placement, clusters []*clusterapiv1.ManagedCluster) PluginFilterResult
}

// Prioritizer defines a prioritizer plugin that score each cluster. The score is normalized
// as a floating betwween 0 and 1.
type Prioritizer interface {
	Plugin

	// Score gives the score to a list of the clusters, it returns a map with the key as
	// the cluster name.
	Score(ctx context.Context, placement *clusterapiv1beta1.Placement, clusters []*clusterapiv1.ManagedCluster) PluginScoreResult
}
```

Scheduler的功能需要不同的plugins支持，目前已有的plugin有：
- filter
  - predicate 
  - tainttoleration 
- Prioritizer 
  - addon
  - builtin
    - balance
    - resource
    - steady
#### predicate
predicate plugin基于Placement中定义的predicate挑选Clusters。
#### tainttoleration
tainttoleration plugin基于Placement中定义的tolerates确认是否可以容忍Clusters的taints。如果taint具有TolerationSeconds，则还需设置requeue time，从而在taint过期时重新reconcile placement。
#### addon
addon plugin 从AddOnPlacementScores中获取指定resource name和score name的score。没有AddOnPlacementScores的cluster，或者score已经过期了，则score为0。
#### balance
balance plugin在排除当前placement之后，平衡cluster之间的decision数量，拥有最多decision数量的cluster的评分最低。decision的数量会归一化到-100，100。
#### resource
resource 包含两个plugin:
- ResourceAllocatableCPU
- ResourceAllocatableMemory

这两个plugin会给拥有最多资源的cluster最高的评分，资源状态是从ManagedCluster CRD中的Status.Allocatable/Capacity中获取。资源也会被归一化为-100，100

#### steady

Steady Plugin会倾向已有的decision。比如之前具有该placement的decision的cluster会评最高分，没有的decision的cluster会评分最低。

### Scheduler
Scheduler是一个interface：
```
type Scheduler interface {
	Schedule(
		ctx context.Context,
		placement *clusterapiv1beta1.Placement,
		clusters []*clusterapiv1.ManagedCluster,
	) (ScheduleResult, error)
}
```
通过输入Placement和一系列cluster获取调度结果。

pluginScheduler是该接口的一个具体实现：
```
type pluginScheduler struct {
	handle             plugins.Handle
	filters            []plugins.Filter
	prioritizerWeights map[clusterapiv1beta1.ScoreCoordinate]int32
}

func NewPluginScheduler(handle plugins.Handle) *pluginScheduler {
	return &pluginScheduler{
		handle: handle,
		filters: []plugins.Filter{
			predicate.New(handle),
			tainttoleration.New(handle),
		},
		prioritizerWeights: defaultPrioritizerConfig,
	}
}
```
defaultPrioritizerConfig是在指定PrioritizerPolicy.Mode为Additive时使用的默认Prioritizer，包含权重为1的Balance和Steady。

pluginScheduler使用一个plugins.Handle初始化，plugins.Handle也是一个接口：
```
type Handle interface {
	// DecisionLister lists all decisions
	DecisionLister() clusterlisterv1beta1.PlacementDecisionLister

	// ScoreLister lists all AddOnPlacementScores
	ScoreLister() clusterlisterv1alpha1.AddOnPlacementScoreLister

	// ClusterLister lists all ManagedClusters
	ClusterLister() clusterlisterv1.ManagedClusterLister

	// ClusterClient returns the cluster client
	ClusterClient() clusterclient.Interface

	// EventRecorder returns an event recorder.
	EventRecorder() events.EventRecorder
}
```

schedulerHandler是该接口的一个实现，pluginScheduler还使用该handler生成了两个filter：
- [predicate](#predicate)
- [tainttoleration](#tainttoleration)

pluginScheduler的Schedule方法主要流程为：
1. 使用filters对传入的cluster先过滤一遍
2. 获取每个prioritizerWeighets的权重，例如{"Steady": 1, "Balance":1, "AddOn/default/ratio":3}
3. 生成一个只包含非0权重的[prioritizer plugin](#plugins)列表。prioritizer plugin也是一个接口，用于获取不同ScoreCoordinate的score。
4. 计算每一个cluster的加权score，并将weight和原始score记录在scheduler result中。
5. 按分值将cluster排序，如果分数相等，按cluster name排序
6. 如果设置了TolerationSeconds，则需要在scheduler result中设置requeue time

#### schedulingController

schedulingController负责在managedCluster, ClusterSet或者ClusterSetBinding发生变化的时候，将相关的Placement requeue。
```
	// setup event handler for cluster informer.
	// Once a cluster changes, clusterEventHandler enqueues all placements which are
	// impacted potentially for further reconciliation. It might not function before the
	// informers/listers of clusterset/clustersetbinding/placement are synced during
	// controller booting. But that should not cause any problem because all existing
	// placements will be enqueued by the controller anyway when booting.
	clusterInformer.Informer().AddEventHandler(&clusterEventHandler{
		clusterSetLister:        clusterSetInformer.Lister(),
		clusterSetBindingLister: clusterSetBindingInformer.Lister(),
		placementLister:         placementInformer.Lister(),
		enqueuePlacementFunc:    enqueuePlacementFunc,
	})

	// setup event handler for clusterset informer
	// Once a clusterset changes, clusterSetEventHandler enqueues all placements which are
	// impacted potentially for further reconciliation. It might not function before the
	// informers/listers of clustersetbinding/placement are synced during controller
	// booting. But that should not cause any problem because all existing placements will
	// be enqueued by the controller anyway when booting.
	clusterSetInformer.Informer().AddEventHandler(&clusterSetEventHandler{
		clusterSetBindingLister: clusterSetBindingInformer.Lister(),
		placementLister:         placementInformer.Lister(),
		enqueuePlacementFunc:    enqueuePlacementFunc,
	})

	// setup event handler for clustersetbinding informer
	// Once a clustersetbinding changes, clusterSetBindingEventHandler enqueues all placements
	// which are impacted potentially for further reconciliation. It might not function before
	// the informers/listers of clusterset/placement are synced during controller booting. But
	// that should not cause any problem because all existing placements will be enqueued by
	// the controller anyway when booting.
	clusterSetBindingInformer.Informer().AddEventHandler(&clusterSetBindingEventHandler{
		clusterSetLister:     clusterSetInformer.Lister(),
		placementLister:      placementInformer.Lister(),
		enqueuePlacementFunc: enqueuePlacementFunc,
	})
```

每次reconcile主要流程为：
1. 如果是正在删除的placement，则忽略
2. 获取Placement namespace中的clusterset bindings
3. 将获取的clusterset同placement中的clusterset正交，获取有效的cluster list
4. 调用scheduler的Schedule方法，获取scheduler results
5. 如果scheduler results有requeue，则将placement requeue
6. 创建placement decision，并删除多余的。

#### schedulingControllerResync
周期性（每5min）将有addon score的placement重新requeue：
```
// Resync the placement which depends on AddOnPlacementScore periodically
func (c *schedulingController) resync(ctx context.Context, syncCtx factory.SyncContext) error {
	queueKey := syncCtx.QueueKey()
	klog.V(4).Infof("Resync placement %q", queueKey)

	if queueKey == "key" {
		placements, err := c.placementLister.List(labels.Everything())
		if err != nil {
			return err
		}

		for _, placement := range placements {
			for _, config := range placement.Spec.PrioritizerPolicy.Configurations {
				if config.ScoreCoordinate != nil && config.ScoreCoordinate.Type == clusterapiv1beta1.ScoreCoordinateTypeAddOn {
					key, _ := cache.MetaNamespaceKeyFunc(placement)
					klog.V(4).Infof("Requeue placement %s", key)
					syncCtx.Queue().Add(key)
					break
				}
			}
		}

		return nil
	} else {
		placement, err := c.getPlacement(queueKey)
		if errors.IsNotFound(err) {
			// no work if placement is deleted
			return nil
		}
		if err != nil {
			return err
		}
		// Do not pass syncCtx to syncPlacement, since don't want to requeue the placement when resyncing the placement.
		return c.syncPlacement(ctx, nil, placement)
	}
}
```


#### debugger
Debugger 是Placement Controller的一个http server，其endpoint 为 `/debug/placements/<namespace>/<name>`。

Handler接收到http请求的时候：
1. 首先会从url中获取namespace和name，并用其获取placement资源。
2. 获取全部的managed clusters。
3. 对获取到的clusters和placement调用scheduler的Schedule函数，获取scheduleResults
4. 返回scheduleResults以供debug使用。
