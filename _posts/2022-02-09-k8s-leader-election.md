---
title: Kubernetes Leader Election
tags: Kubernetes
--- 

Leader Election 是一种实现高可用的方法。其主要思想是部署多实例，并从这些实例中选举一个Leader运行，当某些原因导致Leader失效时，其余实例可竞选Leader角色。Kubernetes的核心组件kube-schduler和kube-controller-manager都采用了Leader Election实现高可用，即同时部署多副本，而真正在工作的实例其实只有一个。
<!--more-->

## Overview

Leader Election的实现是基于Kubernetes对某些资源操作的原子性，其本质上是一个分布式锁。多副本在不断的竞争中进行选举，选中为leader的实例才会执行具体的业务代码。

Kubernetes的Leader Election使用如下几种资源构建分布式锁：
- endpoints
- configmaps
- leases
- 同时使用endpoints和leases两种资源
- 同时使用configmaps和leases两种资源

Leader Election源码主要位于[client-go](https://github.com/kubernetes/client-go/tree/master/tools/leaderelection) pkg中。[contoller-runtime](https://github.com/kubernetes-sigs/controller-runtime) pkg有构建operator时开启leader election的代码。

本文主要通过controller-runtime包中的Manager为入口，详细分析Leader Election的实现。

## Leader Elector 创建

### Manager的初始化

在main.go文件中，需要使用Controller-Runtime Options创建manager，并调用mgr.Start启动manager：
```
func main() {
...
    // 使用Option创建Manager
    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
    ...
        LeaderElection:         enableLeaderElection,
        LeaderElectionID:       "52dcdb12.my.domain",
    })
...
  // Start 启动manager
  mgr.Start(ctrl.SetupSignalHandler())
...
}

// ctrl.NewManager()调用的函数
func New(config *rest.Config, options Options) (Manager, error) {
    // 设置default options
    options = setOptionsDefaults(options)
  ...
    // 创建Resource Lock
    leaderConfig := options.LeaderElectionConfig
    if leaderConfig == nil {
        leaderConfig = rest.CopyConfig(config)
    }
    resourceLock, err := options.newResourceLock(leaderConfig, recorderProvider, leaderelection.Options{
        LeaderElection:             options.LeaderElection,
        LeaderElectionResourceLock: options.LeaderElectionResourceLock,
        LeaderElectionID:           options.LeaderElectionID,
        LeaderElectionNamespace:    options.LeaderElectionNamespace,
    })
  ...

  // 创建manager
  return &controllerManager{
    ...
        resourceLock:                  resourceLock,
        leaseDuration:                 *options.LeaseDuration,
        renewDeadline:                 *options.RenewDeadline,
        retryPeriod:                   *options.RetryPeriod,
        leaderElectionStopped:         make(chan struct{}),
        leaderElectionReleaseOnCancel: options.LeaderElectionReleaseOnCancel,
    ...
    }, nil
}

type Options struct {
...
    // 启动Manager时是否开启Leader Election.
    LeaderElection bool

    // LeaderElectionResourceLock指定使用哪种resource lock，默认使用configmapsleases。
    // 全部的resource lock包括：endpoints，configmaps，leases，endpointsleases，configmapsleases
    LeaderElectionResourceLock string

    // LeaderElectionNamespace指定在哪个Namespace中创建 leader election resource
    LeaderElectionNamespace string

    // LeaderElectionID 指定要创建的leader election resource名字
    LeaderElectionID string

    // LeaderElectionConfig用来指定覆盖创建leader election client使用的默认配置
    LeaderElectionConfig *rest.Config

    // LeaderElectionReleaseOnCancel指定当Manger停止的时候，Leader是否会Step Down。此设置可以加速Leader的切换，因为新Leader无需等待到过期时间。
    LeaderElectionReleaseOnCancel bool

    // 资源锁的过期时间，默认15s
    LeaseDuration *time.Duration
    // Leader renew（reclaim）Leader角色时所能使用的最长时间。默认10s
    RenewDeadline *time.Duration
    // Retry之间的时间，默认2s
    RetryPeriod *time.Duration
...
}
```

Controller Manager中与leader election功能相关的字段主要有：
- resourceLock: 使用那种resource作为争抢Leader的锁
- leaseDuration: 资源锁的过期时间，默认15s
- renewDeadline: leader renew（reclaim）Leader角色时能用的最长时间。默认10s
- retryPeriod: retry之间的时间，默认2s
- leaderElectionStopped:   一个信号，用来通知shutdown程序LeaderElection 程序已经完成。
- leaderElectionCancel: 用来取消leader election程序。
- leaderElectionReleaseOnCancel: Manager ShutDown时是否应该释放锁。
- elected: 一个channel，当manger竞争成为leader或者leader election功能没有被开启时，此Channl会被关闭。
- onStoppedLeading： 当Leader election失去lease时，即不再是Leader，该函数会被调用


options.newResourceLock实际调用的是controller runtime pkg中的leaderelection.NewResourceLock

#### NewResourceLock
NewResourceLock 会使用client-go pkg中的resourcelock.New创建一个的resource lock。主要过程如下：
1. 如果没有设置LeaderElectionResourceLock，则使用默认值configmapsleases
2. 如果没有设置Lock资源的namespace，并且Manager是在Cluster中运行的话，则从/var/run/secrets/kubernetes.io/serviceaccount/namespace文件中读取Namespace
3. 使用Hostname生成当前manager的Id
4. 创建两种Client，一种为了创建Configmap和Endpoint，另一种负责创建Lease
5. 调用client-go pkg中的resourcelock.New创建资源锁。

```
func NewResourceLock(config *rest.Config, recorderProvider recorder.Provider, options Options) (resourcelock.Interface, error) {
    if !options.LeaderElection {
        return nil, nil
    }

    // 设置默认的resource lock为configmapsleases，很早之前的版本默认的resource lock是configmap。使用configmapsleases是为了将来迁移到leases resource lock做过渡准备
    if options.LeaderElectionResourceLock == "" {
        options.LeaderElectionResourceLock = resourcelock.ConfigMapsLeasesResourceLock
    }

    // LeaderElectionID 是创建的leader election resource名字
    if options.LeaderElectionID == "" {
        return nil, errors.New("LeaderElectionID must be configured")
    }

    // 如果没有设置namespace，并且是在Cluster中运行的话，从/var/run/secrets/kubernetes.io/serviceaccount/namespace文件中读取Namespace
    if options.LeaderElectionNamespace == "" {
        var err error
        options.LeaderElectionNamespace, err = getInClusterNamespace()
        if err != nil {
            return nil, fmt.Errorf("unable to find leader election namespace: %w", err)
        }
    }

    // 获取 Leader id
    id, err := os.Hostname()
    if err != nil {
        return nil, err
    }
    id = id + "_" + string(uuid.NewUUID())

    // 创建 leader election clients
    rest.AddUserAgent(config, "leader-election")
    corev1Client, err := corev1client.NewForConfig(config)
    if err != nil {
        return nil, err
    }

    coordinationClient, err := coordinationv1client.NewForConfig(config)
    if err != nil {
        return nil, err
    }

    return resourcelock.New(options.LeaderElectionResourceLock,
        options.LeaderElectionNamespace,
        options.LeaderElectionID,
        corev1Client,
        coordinationClient,
        resourcelock.ResourceLockConfig{
            Identity:      id,
            EventRecorder: recorderProvider.GetEventRecorderFor(id),
        })
}
```

### Manager的启动

在main.go中，通过调用Start函数启动Manager。Start函数中与leader election相关的主要流程：
1. 启动无需leader election功能的程序，例如Webhook。此处特别说明Webhook的启动要先于build cache，否则conversion webhook可能会与cache sync有冲突，导致webhook无法启动。
2. 判断是否需要启用Leader Election
  - 2.1 如果需要，则调用cm.startLeaderElection
  - 2.2 如果不需要，则启动所有获取leader时才能启动的程序（等效于不启用leader election），并关闭cm.elected channel

```
func (cm *controllerManager) Start(ctx context.Context) (err error) {
...
  //启动无需leader election功能的程序，例如Webhook
    go cm.startNonLeaderElectionRunnables()

    go func() {
    // 如果LeaderElection为false，则cm.resourceLock== nil
        if cm.resourceLock != nil {
      // 启动leader election 程序
            err := cm.startLeaderElection()
            if err != nil {
                cm.errChan <- err
            }
        } else {
      // 可以将关闭leader election功能看作是所有实例均是Leader 
            cm.startLeaderElectionRunnables()
            close(cm.elected)
        }
    }()
...
}
```

startLeaderElection使用[client-go](k8s.io/client-go/tools/leaderelection) pkg 中的NewLeaderElector创建了一个LeaderElector实例，具体流程为：
1. 配置cm.leaderElectionCancel 
2. 配置cm.onStoppedLeading
3. NewLeaderElector创建LeaderElector
4. 调用LeaderElector的Run方法启动
5. Run方法完成退出时，关闭leaderElectionStopped channel

```
func (cm *controllerManager) startLeaderElection() (err error) {
    ctx, cancel := context.WithCancel(context.Background())
    cm.mu.Lock()
    cm.leaderElectionCancel = cancel
    cm.mu.Unlock()

    if cm.onStoppedLeading == nil {
        cm.onStoppedLeading = func() {
            // Make sure graceful shutdown is skipped if we lost the leader lock without
            // intending to.
            cm.gracefulShutdownTimeout = time.Duration(0)
            // Most implementations of leader election log.Fatal() here.
            // Since Start is wrapped in log.Fatal when called, we can just return
            // an error here which will cause the program to exit.
            cm.errChan <- errors.New("leader election lost")
        }
    }
    l, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
        Lock:          cm.resourceLock,
        LeaseDuration: cm.leaseDuration,
        RenewDeadline: cm.renewDeadline,
        RetryPeriod:   cm.retryPeriod,
        Callbacks: leaderelection.LeaderCallbacks{
            OnStartedLeading: func(_ context.Context) {
                cm.startLeaderElectionRunnables()
                close(cm.elected)
            },
            OnStoppedLeading: cm.onStoppedLeading,
        },
        ReleaseOnCancel: cm.leaderElectionReleaseOnCancel,
    })
    if err != nil {
        return err
    }

    // Start the leader elector process
    go func() {
        l.Run(ctx)
        <-ctx.Done()
        close(cm.leaderElectionStopped)
    }()
    return nil
}
```

### Leader Elector 的初始化

[client-go](k8s.io/client-go/tools/leaderelection) pkg中leaderelection.go中的LeaderElector结构体是选举程序执行的主体：
```
type LeaderElector struct {
    config LeaderElectionConfig
    // internal bookkeeping
    observedRecord    rl.LeaderElectionRecord
    observedRawRecord []byte
    observedTime      time.Time
    // used to implement OnNewLeader(), may lag slightly from the
    // value observedRecord.HolderIdentity if the transition has
    // not yet been reported.
    reportedLeader string

    // clock is wrapper around time to allow for less flaky testing
    clock clock.Clock

    // used to lock the observedRecord
    observedRecordLock sync.Mutex

    metrics leaderMetricsAdapter
}


type LeaderElectionConfig struct {
    // Lock is the resource that will be used for locking
    Lock rl.Interface

    // LeaseDuration is the duration that non-leader candidates will
    // wait to force acquire leadership. This is measured against time of
    // last observed ack.
    //
    // A client needs to wait a full LeaseDuration without observing a change to
    // the record before it can attempt to take over. When all clients are
    // shutdown and a new set of clients are started with different names against
    // the same leader record, they must wait the full LeaseDuration before
    // attempting to acquire the lease. Thus LeaseDuration should be as short as
    // possible (within your tolerance for clock skew rate) to avoid a possible
    // long waits in the scenario.
    //
    // Core clients default this value to 15 seconds.
    LeaseDuration time.Duration
    // RenewDeadline is the duration that the acting master will retry
    // refreshing leadership before giving up.
    //
    // Core clients default this value to 10 seconds.
    RenewDeadline time.Duration
    // RetryPeriod is the duration the LeaderElector clients should wait
    // between tries of actions.
    //
    // Core clients default this value to 2 seconds.
    RetryPeriod time.Duration

    // Callbacks are callbacks that are triggered during certain lifecycle
    // events of the LeaderElector
    Callbacks LeaderCallbacks

    // WatchDog is the associated health checker
    // WatchDog may be null if its not needed/configured.
    WatchDog *HealthzAdaptor

    // ReleaseOnCancel should be set true if the lock should be released
    // when the run context is cancelled. If you set this to true, you must
    // ensure all code guarded by this lease has successfully completed
    // prior to cancelling the context, or you may have two processes
    // simultaneously acting on the critical path.
    ReleaseOnCancel bool

    // Name is the name of the resource lock for debugging
    Name string
}

// LeaderCallbacks are callbacks that are triggered during certain
// lifecycle events of the LeaderElector. These are invoked asynchronously.
//
// possible future callbacks:
//  * OnChallenge()
type LeaderCallbacks struct {
    // OnStartedLeading is called when a LeaderElector client starts leading
    OnStartedLeading func(context.Context)
    // OnStoppedLeading is called when a LeaderElector client stops leading
    OnStoppedLeading func()
    // OnNewLeader is called when the client observes a leader that is
    // not the previously observed leader. This includes the first observed
    // leader when the client starts.
    OnNewLeader func(identity string)
}
```
其主要包含以下重要内容：
- LeaderElectionConfig：LeaderElection的配置
  - Lock：资源锁
  - LeaseDuration: 非leader尝试争抢Leader的间隔，默认15s
  - renewDeadline: leader renew（reclaim）Leader角色时能用的最长时间。默认10s
  - retryPeriod: retry之间的时间，默认2s
  - Callbacks: Leader election不同阶段调用的函数：
    - OnStartedLeading: 获取Leader时调用的函数，主要内容是调用startLeaderElectionRunnables启动程序，并关闭cm.elected channel用来表示已经成为leader。
    - OnStoppedLeading: 不是leader时调用的函数，摇摇内容就是设置manager的gracefulShutdownTimeout为0，立即
    - OnNewLeader: 当leader出现变化时调用的函数
  - ReleaseOnCancel: 当LeaderElector Run方法结束时，是否立刻释放资源锁。
- observedRecord,observedRawRecord,observedTime: Leader在资源锁中写入的值

## Leader Elector 启动
Run方法用于启动Leader Elector，其主要流程是：
1. 调用le.acquire(ctx)不断尝试获取资源锁，即竞选Leader
2. 一旦获取成功，则调用OnStartedLeading callback，此处即是启动所有enable leader election的程序
3. 调用le.renew不断的尝试更新锁
4. 如果renew超过RenewDeadline还没有成功，则调用OnStoppedLeading callback，此处即是设置Manager gracefullshotdowntime 为0s。

```
func (le *LeaderElector) Run(ctx context.Context) {
    defer runtime.HandleCrash()
    defer func() {
        le.config.Callbacks.OnStoppedLeading()
    }()

    if !le.acquire(ctx) {
        return // ctx signalled done
    }
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()
    go le.config.Callbacks.OnStartedLeading(ctx)
    le.renew(ctx)
}
```

### 获取资源锁(竞争leader)
LeaderElector通过调用acquire方法获取资源锁，一旦获取成功，该方法会立刻返回true。其内部通过wait.JitterUntil不断地调用le.tryAcquireOrRenew。
```
func (le *LeaderElector) acquire(ctx context.Context) bool {
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()
    succeeded := false
    desc := le.config.Lock.Describe()
    klog.Infof("attempting to acquire leader lease %v...", desc)
    wait.JitterUntil(func() {
        succeeded = le.tryAcquireOrRenew(ctx)
        le.maybeReportTransition()
        if !succeeded {
            klog.V(4).Infof("failed to acquire lease %v", desc)
            return
        }
        le.config.Lock.RecordEvent("became leader")
        le.metrics.leaderOn(le.config.Name)
        klog.Infof("successfully acquired lease %v", desc)
        cancel()
    }, le.config.RetryPeriod, JitterFactor, true, ctx.Done())
    return succeeded
}
```
### 更新资源锁(刷新leader任期)
LeaderElector通过调用renew方法更新资源锁。其内部通过wait.Until和wait.PollImmediateUntil不断地调用le.tryAcquireOrRenew。当更新失败时，根据是否设置了ReleaseOnCancel调用le.release()
```
func (le *LeaderElector) renew(ctx context.Context) {
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()
    wait.Until(func() {
        timeoutCtx, timeoutCancel := context.WithTimeout(ctx, le.config.RenewDeadline)
        defer timeoutCancel()
        err := wait.PollImmediateUntil(le.config.RetryPeriod, func() (bool, error) {
            return le.tryAcquireOrRenew(timeoutCtx), nil
        }, timeoutCtx.Done())

        le.maybeReportTransition()
        desc := le.config.Lock.Describe()
        if err == nil {
            klog.V(5).Infof("successfully renewed lease %v", desc)
            return
        }
        le.config.Lock.RecordEvent("stopped leading")
        le.metrics.leaderOff(le.config.Name)
        klog.Infof("failed to renew lease %v: %v", desc, err)
        cancel()
    }, le.config.RetryPeriod, ctx.Done())

    // if we hold the lease, give it up
    if le.config.ReleaseOnCancel {
        le.release()
    }
}
```

如果调用release方法是Leader，实际上就是更新资源锁的有效时间为1s。这样其余的实例便可以立刻竞争leader
```
func (le *LeaderElector) release() bool {
    if !le.IsLeader() {
        return true
    }
    now := metav1.Now()
    leaderElectionRecord := rl.LeaderElectionRecord{
        LeaderTransitions:    le.observedRecord.LeaderTransitions,
        LeaseDurationSeconds: 1,
        RenewTime:            now,
        AcquireTime:          now,
    }
    if err := le.config.Lock.Update(context.TODO(), leaderElectionRecord); err != nil {
        klog.Errorf("Failed to release lock: %v", err)
        return false
    }

    le.setObservedRecord(&leaderElectionRecord)
    return true
}
```

可以看到，无论是renew还是acquire，最终调用的都是tryAcquireOrRenew函数。其内部主要实现：
1. 获取或者创建ElectionRecord
2. 记录获取到的ElectionRecord，并检查Identity & Time
3. 如果锁已经过期则尝试竞争Leader

```
func (le *LeaderElector) tryAcquireOrRenew(ctx context.Context) bool {
    now := metav1.Now()
    leaderElectionRecord := rl.LeaderElectionRecord{
        HolderIdentity:       le.config.Lock.Identity(),
        LeaseDurationSeconds: int(le.config.LeaseDuration / time.Second),
        RenewTime:            now,
        AcquireTime:          now,
    }

    // 1. obtain or create the ElectionRecord
    oldLeaderElectionRecord, oldLeaderElectionRawRecord, err := le.config.Lock.Get(ctx)
    if err != nil {
        if !errors.IsNotFound(err) {
            klog.Errorf("error retrieving resource lock %v: %v", le.config.Lock.Describe(), err)
            return false
        }
        if err = le.config.Lock.Create(ctx, leaderElectionRecord); err != nil {
            klog.Errorf("error initially creating leader election record: %v", err)
            return false
        }

        le.setObservedRecord(&leaderElectionRecord)

        return true
    }

    // 2. Record obtained, check the Identity & Time
    if !bytes.Equal(le.observedRawRecord, oldLeaderElectionRawRecord) {
        le.setObservedRecord(oldLeaderElectionRecord)

        le.observedRawRecord = oldLeaderElectionRawRecord
    }
    if len(oldLeaderElectionRecord.HolderIdentity) > 0 &&
        le.observedTime.Add(le.config.LeaseDuration).After(now.Time) &&
        !le.IsLeader() {
        klog.V(4).Infof("lock is held by %v and has not yet expired", oldLeaderElectionRecord.HolderIdentity)
        return false
    }

    // 3. We're going to try to update. The leaderElectionRecord is set to it's default
    // here. Let's correct it before updating.
    if le.IsLeader() {
        leaderElectionRecord.AcquireTime = oldLeaderElectionRecord.AcquireTime
        leaderElectionRecord.LeaderTransitions = oldLeaderElectionRecord.LeaderTransitions
    } else {
        leaderElectionRecord.LeaderTransitions = oldLeaderElectionRecord.LeaderTransitions + 1
    }

    // update the lock itself
    if err = le.config.Lock.Update(ctx, leaderElectionRecord); err != nil {
        klog.Errorf("Failed to update lock: %v", err)
        return false
    }

    le.setObservedRecord(&leaderElectionRecord)
    return true
}
```

对于资源锁的操作都是基于le.config.Lock实现的。接下来我们看看资源锁的具体实现方法。

### Resource Lock
NewResourceLock最后会调用client-go/tools/leaderelection/resourcelock中的New方法，该方法根据不同的lockType返回不同的Lock实例。
```
func New(lockType string, ns string, name string, coreClient corev1.CoreV1Interface, coordinationClient coordinationv1.CoordinationV1Interface, rlc ResourceLockConfig) (Interface, error) {
    endpointsLock := &EndpointsLock{
        EndpointsMeta: metav1.ObjectMeta{
            Namespace: ns,
            Name:      name,
        },
        Client:     coreClient,
        LockConfig: rlc,
    }
    configmapLock := &ConfigMapLock{
        ConfigMapMeta: metav1.ObjectMeta{
            Namespace: ns,
            Name:      name,
        },
        Client:     coreClient,
        LockConfig: rlc,
    }
    leaseLock := &LeaseLock{
        LeaseMeta: metav1.ObjectMeta{
            Namespace: ns,
            Name:      name,
        },
        Client:     coordinationClient,
        LockConfig: rlc,
    }
    switch lockType {
    case EndpointsResourceLock:
        return endpointsLock, nil
    case ConfigMapsResourceLock:
        return configmapLock, nil
    case LeasesResourceLock:
        return leaseLock, nil
    case EndpointsLeasesResourceLock:
        return &MultiLock{
            Primary:   endpointsLock,
            Secondary: leaseLock,
        }, nil
    case ConfigMapsLeasesResourceLock:
        return &MultiLock{
            Primary:   configmapLock,
            Secondary: leaseLock,
        }, nil
    default:
        return nil, fmt.Errorf("Invalid lock-type %s", lockType)
    }
}
```

每种Lock实例都实现了Interface接口中的方法。该Interface提供了一套通用的接口，用于锁定任意在Leader Election中使用的资源。
```
type Interface interface {
    // 获取 LeaderElectionRecord
    Get(ctx context.Context) (*LeaderElectionRecord, []byte, error)
    // 创建一个 LeaderElectionRecord
    Create(ctx context.Context, ler LeaderElectionRecord) error
    // 更新一个已经存在的 LeaderElectionRecord
    Update(ctx context.Context, ler LeaderElectionRecord) error
    // 记录事件
    RecordEvent(string)
    // 返回 locks Identity
    Identity() string

    // 将resource lock的描述信息以字符串形式输出
    Describe() string
}

// Lock Resource上的annotation key
LeaderElectionRecordAnnotationKey = "control-plane.alpha.kubernetes.io/leader"

// LeaderElectionRecord 是在leader election annotation的记录。
type LeaderElectionRecord struct {
  // HolderIdentity 是拥有租约的 ID。如果为空，则没有人拥有此租约，并且所有调用者都可以获取租约。当客户自愿放弃租约时，此值设置为空。
    // HolderIdentity is the ID that owns the lease. If empty, no one owns this lease and
    // all callers may acquire. Versions of this library prior to Kubernetes 1.14 will not
    // attempt to acquire leases with empty identities and will wait for the full lease
    // interval to expire before attempting to reacquire. This value is set to empty when
    // a client voluntarily steps down.
    HolderIdentity       string      `json:"holderIdentity"`
    LeaseDurationSeconds int         `json:"leaseDurationSeconds"`
    AcquireTime          metav1.Time `json:"acquireTime"`
    RenewTime            metav1.Time `json:"renewTime"`
    LeaderTransitions    int         `json:"leaderTransitions"`
}
```

#### ConfigmapLock
先看ConfigMapLock的结构
```
type ConfigMapLock struct {
    // ConfigMapMeta 包含Leader Election想创建的资源name和namespace
    ConfigMapMeta metav1.ObjectMeta
  // 创建Configmap的client
    Client        corev1client.ConfigMapsGetter
  // 包含由Host生成的ID和Record
    LockConfig    ResourceLockConfig
  // 创建成功的Configmap
    cm            *v1.ConfigMap
}
```

Get 很简单，就是从Configmap里拿LeaderElectionRecordAnnotation，并将其内容返回
```
// Get returns the election record from a ConfigMap Annotation
func (cml *ConfigMapLock) Get(ctx context.Context) (*LeaderElectionRecord, []byte, error) {
    var record LeaderElectionRecord
    var err error
    cml.cm, err = cml.Client.ConfigMaps(cml.ConfigMapMeta.Namespace).Get(ctx, cml.ConfigMapMeta.Name, metav1.GetOptions{})
    if err != nil {
        return nil, nil, err
    }
    if cml.cm.Annotations == nil {
        cml.cm.Annotations = make(map[string]string)
    }
    recordStr, found := cml.cm.Annotations[LeaderElectionRecordAnnotationKey]
    recordBytes := []byte(recordStr)
    if found {
        if err := json.Unmarshal(recordBytes, &record); err != nil {
            return nil, nil, err
        }
    }
    return &record, recordBytes, nil
}
```

Create则是以ConfigMapMeta和LeaderElectionRecord创建一个Configmap
```
func (cml *ConfigMapLock) Create(ctx context.Context, ler LeaderElectionRecord) error {
    recordBytes, err := json.Marshal(ler)
    if err != nil {
        return err
    }
    cml.cm, err = cml.Client.ConfigMaps(cml.ConfigMapMeta.Namespace).Create(ctx, &v1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      cml.ConfigMapMeta.Name,
            Namespace: cml.ConfigMapMeta.Namespace,
            Annotations: map[string]string{
                LeaderElectionRecordAnnotationKey: string(recordBytes),
            },
        },
    }, metav1.CreateOptions{})
    return err
}
```

Updata 负责修改Configmap的annotation
```
// Update will update an existing annotation on a given resource.
func (cml *ConfigMapLock) Update(ctx context.Context, ler LeaderElectionRecord) error {
    if cml.cm == nil {
        return errors.New("configmap not initialized, call get or create first")
    }
    recordBytes, err := json.Marshal(ler)
    if err != nil {
        return err
    }
    if cml.cm.Annotations == nil {
        cml.cm.Annotations = make(map[string]string)
    }
    cml.cm.Annotations[LeaderElectionRecordAnnotationKey] = string(recordBytes)
    cm, err := cml.Client.ConfigMaps(cml.ConfigMapMeta.Namespace).Update(ctx, cml.cm, metav1.UpdateOptions{})
    if err != nil {
        return err
    }
    cml.cm = cm
    return nil
}
```

Describe会获取Configmap的name和namespace，Identity则是获取LockConfig中由Host生成的id。
```
// Describe is used to convert details on current resource lock
// into a string
func (cml *ConfigMapLock) Describe() string {
    return fmt.Sprintf("%v/%v", cml.ConfigMapMeta.Namespace, cml.ConfigMapMeta.Name)
}

// Identity returns the Identity of the lock
func (cml *ConfigMapLock) Identity() string {
    return cml.LockConfig.Identity
}
```
#### EndpointsLock
EndpointsLock的实现同ConfigmapLock基本是一样的，只不过资源由Configmap修改成了Endpoint。这里就不做展示了。

#### LeaseLock
LeaseLock的实现同ConfigmapLock和EndpointsLock也是基本一样，只不过lease资源中的Spec就含有LeaderElectionRecord相关的字段，所以LeaderElectionRecord不用存在Annotation中，而是经过转换存在lease的spec中。

#### MultiLock

对于上述几种方法，MultiLock会依次对Primary和Secondry resource做处理。