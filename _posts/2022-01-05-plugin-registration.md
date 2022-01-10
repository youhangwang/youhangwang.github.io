---
title: Kubelet Plugin Registration Service
tags: Kubelet PluginRegistration
--- 

可扩展性是Kubernetes一直所遵循的设计哲学。Kubelet在实现上使用Plugin模式在无需修改核心代码的前提下实现功能扩展。<!--more-->目前Kubelet支持两种Plugin类型：
- Device Plugin
- CSI Plugin

为了统一两种插件的注册方式，Kubelet使用一种通用的注册机制为两种Plugin提供注册服务，本文将会对其做详细的介绍。

## Plugin Manger

Kubelet通过使用Plugin Manger对Plugins做注册和注销操作：
```
type Kubelet struct {
    ...
    // pluginmanager 运行一系列的异步循环检查哪些plugin需要注册或注销，并执行相应操作
    pluginManager pluginmanager.PluginManager
    ...
}
type PluginManager interface {
    // 运行 plugin manager及其管理的异步循环
    Run(sourcesReady config.SourcesReady, stopCh <-chan struct{})

    // AddHandler会为每种plugin类型（CSI，Device）添加处理函数。 
    AddHandler(pluginType string, pluginHandler cache.PluginHandler)
}
```
`PluginManager`是一个接口，它包含两个方法：
- Run(sourcesReady config.SourcesReady, stopCh <-chan struct{}): 会启动一系列异步的循环不断的检查有哪些Plugin需要注册和注销
- AddHandler(pluginType string, pluginHandler cache.PluginHandler)：为每一种plugin type添加处理函数

pluginManger实例的创建：
```
klet.pluginManager = pluginmanager.NewPluginManager(
    // 获取Plugin注册Socket的所在目录，通常为$ROOT_DIR + /plugins_registry
    klet.getPluginsRegistrationDir(), /* sockDir */
    kubeDeps.Recorder,
)

func NewPluginManager(
    sockDir string,
    recorder record.EventRecorder) PluginManager {
    asw := cache.NewActualStateOfWorld()
    dsw := cache.NewDesiredStateOfWorld()
    reconciler := reconciler.NewReconciler(
        operationexecutor.NewOperationExecutor(
            operationexecutor.NewOperationGenerator(
                recorder,
            ),
        ),
        loopSleepDuration,
        dsw,
        asw,
    )

    pm := &pluginManager{
        desiredStateOfWorldPopulator: pluginwatcher.NewWatcher(
            sockDir,
            dsw,
        ),
        reconciler:          reconciler,
        desiredStateOfWorld: dsw,
        actualStateOfWorld:  asw,
    }
    return pm
}

// pluginManager 实现了 PluginManager 接口
type pluginManager struct {
    // Watch sockDir中是否有新的socket，并异步地更新desiredStateOfWorld
    desiredStateOfWorldPopulator *pluginwatcher.Watcher

    // 周期性异步地通过operationExecutor调用注册和注销方法使actualStateOfWorld符合desiredStateOfWorld
    reconciler reconciler.Reconciler

    // 真实世界的状态，例如哪些plugin已经被注册，哪些plugin已经被注销。一旦Plugin被成功注册或者reconciler出发点了注销方法，会更新该数据结构。
    actualStateOfWorld cache.ActualStateOfWorld

    // 表示期望世界的状态，例如哪些plugin需要被注册，哪些plugin需要被注销。该数据结构被desiredStateOfWorldPopulator（Watcher）更新
    desiredStateOfWorld cache.DesiredStateOfWorld
}
```
pluginManager中主要包含以下内容：
- actualStateOfWorld：表示真实世界的状态，例如哪些plugin已经被注册，哪些plugin已经被注销。
- desiredStateOfWorld：表示期望世界的状态，例如哪些plugin需要被注册，哪些plugin需要被注销。
- reconciler：周期性异步的通过调用注册和注销方法使actualStateOfWorld符合desiredStateOfWorld。
- desiredStateOfWorldPopulator：异步地watch sockDir中是否有新的socket，并更新desiredStateOfWorld。


添加两种plugin type的处理函数，并运行pluginManager：
```
// 添加CSI Driver的注册回调函数
kl.pluginManager.AddHandler(pluginwatcherapi.CSIPlugin, plugincache.PluginHandler(csi.PluginHandler))
// 添加Device Manager的注册回调函数
kl.pluginManager.AddHandler(pluginwatcherapi.DevicePlugin, kl.containerManager.GetPluginRegistrationHandler())
// 启动 plugin manager
go kl.pluginManager.Run(kl.sourcesReady, wait.NeverStop)
```
`pluginwatcherapi.CSIPlugin`和`pluginwatcherapi.DevicePlugin`分别代表两种Plugin类型：
- CSIPlugin
- DevicePlugin

CSIPlugin和DevicePlugin使用两种不同的处理函数：
- [plugincache.PluginHandler(csi.PluginHandler)](#csi-plugin-handler)
- [kl.containerManager.GetPluginRegistrationHandler()](#containermanger-pluginregistrationhandler)

pluginManager.AddHandler会将处理函数添加到reconciler的handlers中
```
func (pm *pluginManager) AddHandler(pluginType string, handler cache.PluginHandler) {
    pm.reconciler.AddHandler(pluginType, handler)
}
```

pluginManager.Run 方法会启动desiredStateOfWorldPopulator，不断的更新desiredStateOfWorld。再使用协程启动reconciler：
```
func (pm *pluginManager) Run(sourcesReady config.SourcesReady, stopCh <-chan struct{}) {
    defer runtime.HandleCrash()

    pm.desiredStateOfWorldPopulator.Start(stopCh)
    klog.V(2).InfoS("The desired_state_of_world populator (plugin watcher) starts")

    klog.InfoS("Starting Kubelet Plugin Manager")
    go pm.reconciler.Run(stopCh)

    metrics.Register(pm.actualStateOfWorld, pm.desiredStateOfWorld)
    <-stopCh
    klog.InfoS("Shutting down Kubelet Plugin Manager")
}
```

Plugin Manger整体的流程就是这样，接下来我们进入其内部的各个组件，一探究竟。

### DesiredStateOfWorld

`DesiredStateOfWorld`是一个接口，为kubelet plugin manager期望状态的缓存定义了一套线程安全的操作：
```
// DesiredStateOfWorld为kubelet plugin manager的期望状态缓存定义了一套线程安全的操作
// 该缓存包含了一个以该Node上Socket file路径为Key，Plugin Information为Value的map
type DesiredStateOfWorld interface {
    // 如果Cache中不存在该Plugin（Socket Path），则添加。如果存在，则修改timestamp。如果socketPath为空，则报错
    AddOrUpdatePlugin(socketPath string) error

    // 从Cache中删除该Plugin（Socket Path）
    RemovePlugin(socketPath string)

    // 获取Cache中的Plugins
    GetPluginsToRegister() []PluginInfo

    // 检查Plugin是否在Cache中存在
    PluginExists(socketPath string) bool
}
```

其实现是一个并发安全的Map：
```
func NewDesiredStateOfWorld() DesiredStateOfWorld {
    return &desiredStateOfWorld{
        socketFileToInfo: make(map[string]PluginInfo),
    }
}

type desiredStateOfWorld struct {

    // socketFileToInfo 包含了需要被注册的Plugin
    socketFileToInfo map[string]PluginInfo
    sync.RWMutex
}
```

实现的接口也很简单，这里就不做详细介绍了：
```
func (dsw *desiredStateOfWorld) AddOrUpdatePlugin(socketPath string) error {
    dsw.Lock()
    defer dsw.Unlock()

    if socketPath == "" {
        return fmt.Errorf("socket path is empty")
    }
    if _, ok := dsw.socketFileToInfo[socketPath]; ok {
        klog.V(2).InfoS("Plugin exists in actual state cache, timestamp will be updated", "path", socketPath)
    }

    // Update the PluginInfo object.
    // Note that we only update the timestamp in the desired state of world, not the actual state of world
    // because in the reconciler, we need to check if the plugin in the actual state of world is the same
    // version as the plugin in the desired state of world
    dsw.socketFileToInfo[socketPath] = PluginInfo{
        SocketPath: socketPath,
        Timestamp:  time.Now(),
    }
    return nil
}

func (dsw *desiredStateOfWorld) RemovePlugin(socketPath string) {
    dsw.Lock()
    defer dsw.Unlock()

    delete(dsw.socketFileToInfo, socketPath)
}

func (dsw *desiredStateOfWorld) GetPluginsToRegister() []PluginInfo {
    dsw.RLock()
    defer dsw.RUnlock()

    pluginsToRegister := []PluginInfo{}
    for _, pluginInfo := range dsw.socketFileToInfo {
        pluginsToRegister = append(pluginsToRegister, pluginInfo)
    }
    return pluginsToRegister
}

func (dsw *desiredStateOfWorld) PluginExists(socketPath string) bool {
    dsw.RLock()
    defer dsw.RUnlock()

    _, exists := dsw.socketFileToInfo[socketPath]
    return exists
}
```

### ActualStateOfWorld

`ActualStateOfWorld`是一个接口，为kubelet plugin manager真实状态的缓存定义了一套线程安全的操作：：
```
// ActualStateOfWorld为kubelet plugin manager的实际状态缓存定义了一套线程安全的操作
// 该缓存包含了一个以该Node上Socket file路径为Key，Plugin Information为Value的map
type ActualStateOfWorld interface {

    // GetRegisteredPlugins 获取在真实环境中已经被注册的Plugin列表
    GetRegisteredPlugins() []PluginInfo

    // 向Cache中添加Plugin info。与desiredStateOfWorld不同，此时无需更新timestamp,如果有更新，则在真实状态的缓存中会表现为删除+创建
    AddPlugin(pluginInfo PluginInfo) error

    // 从Cache中删除Plugin（Socket Path）
    RemovePlugin(socketPath string)

    // 检查Cache中使用具有相同timestamp和SocketPath的Plugin
    PluginExistsWithCorrectTimestamp(pluginInfo PluginInfo) bool
}
```

ActualStateOfWorld和DesiredStateOfWorld的实现基本一样，也是一个并发安全的Map，这里就不做展开了，有兴趣的同学可以去看源码。
### DesiredStateOfWorldPopulator
`DesiredStateOfWorldPopulator`实际上是一个watcher：
```
pm := &pluginManager{
    desiredStateOfWorldPopulator: pluginwatcher.NewWatcher(
        sockDir,
        dsw,
    ),
    ...
}
func NewWatcher(sockDir string, desiredStateOfWorld cache.DesiredStateOfWorld) *Watcher {
    return &Watcher{
        path:                sockDir,
        fs:                  &utilfs.DefaultFs{},
        desiredStateOfWorld: desiredStateOfWorld,
    }
}
type Watcher struct {
    path                string
    fs                  utilfs.Filesystem
    fsWatcher           *fsnotify.Watcher
    desiredStateOfWorld cache.DesiredStateOfWorld
}
```
它会在Start时：
1. 确认path是否存在，如果不存在，则新建Path。
2. 创建fsWatcher，并将Path及子目录添加至watch列表中。如果已经有存在Plugin注册Socket则调用handleCreateEvent处理
3. 发现有新建/删除的注册socket则调用handleCreateEvent或handleDeleteEvent。

```
// 监听path上plugin socket的创建与删除
func (w *Watcher) Start(stopCh <-chan struct{}) error {
    // 如果Path不存在，则创建相应的目录
    if err := w.init(); err != nil {
        return err
    }

    // 创建Watcher
    fsWatcher, err := fsnotify.NewWatcher()
    if err != nil {
        return fmt.Errorf("failed to start plugin fsWatcher, err: %v", err)
    }
    w.fsWatcher = fsWatcher

    // 监听Path及其children路径，并处理已经存在的plugin Sockets
    if err := w.traversePluginDir(w.path); err != nil {
        klog.ErrorS(err, "Failed to traverse plugin socket path", "path", w.path)
    }

    //不断从fsWatcher.Events/sWatcher.Errors channel中获取消息并处理。
    go func(fsWatcher *fsnotify.Watcher) {
        for {
            select {
            case event := <-fsWatcher.Events:
                //TODO: Handle errors by taking corrective measures
                if event.Op&fsnotify.Create == fsnotify.Create {
                    err := w.handleCreateEvent(event)
                    if err != nil {
                        klog.ErrorS(err, "Error when handling create event", "event", event)
                    }
                } else if event.Op&fsnotify.Remove == fsnotify.Remove {
                    w.handleDeleteEvent(event)
                }
                continue
            case err := <-fsWatcher.Errors:
                if err != nil {
                    klog.ErrorS(err, "FsWatcher received error")
                }
                continue
            case <-stopCh:
                w.fsWatcher.Close()
                return
            }
        }
    }(fsWatcher)

    return nil
}
```

handleDeleteEvent直接从desiredStateOfWorld移除plugin：
```
func (w *Watcher) handleDeleteEvent(event fsnotify.Event) {
    klog.V(6).InfoS("Handling delete event", "event", event)

    socketPath := event.Name
    klog.V(2).InfoS("Removing socket path from desired state cache", "path", socketPath)
    w.desiredStateOfWorld.RemovePlugin(socketPath)
}
```

handleCreateEvent则会:
- 判断新建的是否为Socket，如果是才会加入到desiredStateOfWorld
- 如果新建的是dir，则加入到watcher列表中监听
- 如果是其他文件，则直接忽略
```
func (w *Watcher) handleCreateEvent(event fsnotify.Event) error {
    fi, err := os.Stat(event.Name)
    if err != nil {
        return fmt.Errorf("stat file %s failed: %v", event.Name, err)
    }

    if strings.HasPrefix(fi.Name(), ".") {
        klog.V(5).InfoS("Ignoring file (starts with '.')", "path", fi.Name())
        return nil
    }

    if !fi.IsDir() {
        isSocket, err := util.IsUnixDomainSocket(util.NormalizePath(event.Name))
        if err != nil {
            return fmt.Errorf("failed to determine if file: %s is a unix domain socket: %v", event.Name, err)
        }
        if !isSocket {
            klog.V(5).InfoS("Ignoring non socket file", "path", fi.Name())
            return nil
        }

        return w.handlePluginRegistration(event.Name)
    }

    return w.traversePluginDir(event.Name)
}
```
### reconcile
`Reconciler` 是一个接口，它会运行一个周期性的循环通过调用注册和注销方法不断的reconcile现实和期望状态。
```
type Reconciler interface {
    // 启动一个reconciliation周期性的检查plugin是否被正确的注册和注销。
    Run(stopCh <-chan struct{})

    // AddHandler 会添加指定plugin类型的处理函数。这些处理函数也会被添加到actual state of world缓存中
    AddHandler(pluginType string, pluginHandler cache.PluginHandler)
}
```

创建实例的操作：
```
func NewPluginManager(
...
    reconciler := reconciler.NewReconciler(
        operationexecutor.NewOperationExecutor(
            operationexecutor.NewOperationGenerator(
                recorder,
            ),
        ),
        loopSleepDuration,
        dsw,
        asw,
    )
...
}

func NewReconciler(
    operationExecutor operationexecutor.OperationExecutor,
    loopSleepDuration time.Duration,
    desiredStateOfWorld cache.DesiredStateOfWorld,
    actualStateOfWorld cache.ActualStateOfWorld) Reconciler {
    return &reconciler{
        operationExecutor:   operationExecutor,
        loopSleepDuration:   loopSleepDuration,
        desiredStateOfWorld: desiredStateOfWorld,
        actualStateOfWorld:  actualStateOfWorld,
        handlers:            make(map[string]cache.PluginHandler),
    }
}

type reconciler struct {
    operationExecutor   operationexecutor.OperationExecutor
    loopSleepDuration   time.Duration
    desiredStateOfWorld cache.DesiredStateOfWorld
    actualStateOfWorld  cache.ActualStateOfWorld
    handlers            map[string]cache.PluginHandler
    sync.RWMutex
}
```

其实现主要包含如下内容：
- actualStateOfWorld：[前文](#actualstateofworld)已经详细描述过，表示真实世界的状态，例如哪些plugin已经被注册，哪些plugin已经被注销。
- desiredStateOfWorld：[前文](#desiredstateofworld)已经详细描述过，表示期望世界的状态，例如哪些plugin需要被注册，哪些plugin需要被注销。
- operationExecutor：执行注册和注销plugin的函数。
- loopSleepDuration： reconcile的时间间隔。
- handlers: Plugin的处理函数    


Reconcile的主要逻辑包括：
- 如果plugin在desiredStateOfWorld中存在，但在actualStateOfWorld不存在或者没有正确的timestamp，则使用operationExecutor.RegisterPlugin注册Plugin
- 如果plugin在actualStateOfWorld中存在，但是在desiredStateOfWorld不存在或者timestampe不一样，则operationExecutor.UnregisterPlugin注销Plugin
```
func (rc *reconciler) reconcile() {
    // Unregisterations are triggered before registrations

    // Ensure plugins that should be unregistered are unregistered.
    for _, registeredPlugin := range rc.actualStateOfWorld.GetRegisteredPlugins() {
        unregisterPlugin := false
        if !rc.desiredStateOfWorld.PluginExists(registeredPlugin.SocketPath) {
            unregisterPlugin = true
        } else {
            // We also need to unregister the plugins that exist in both actual state of world
            // and desired state of world cache, but the timestamps don't match.
            // Iterate through desired state of world plugins and see if there's any plugin
            // with the same socket path but different timestamp.
            for _, dswPlugin := range rc.desiredStateOfWorld.GetPluginsToRegister() {
                if dswPlugin.SocketPath == registeredPlugin.SocketPath && dswPlugin.Timestamp != registeredPlugin.Timestamp {
                    klog.V(5).InfoS("An updated version of plugin has been found, unregistering the plugin first before reregistering", "plugin", registeredPlugin)
                    unregisterPlugin = true
                    break
                }
            }
        }

        if unregisterPlugin {
            klog.V(5).InfoS("Starting operationExecutor.UnregisterPlugin", "plugin", registeredPlugin)
            err := rc.operationExecutor.UnregisterPlugin(registeredPlugin, rc.actualStateOfWorld)
            if err != nil &&
                !goroutinemap.IsAlreadyExists(err) &&
                !exponentialbackoff.IsExponentialBackoff(err) {
                // Ignore goroutinemap.IsAlreadyExists and exponentialbackoff.IsExponentialBackoff errors, they are expected.
                // Log all other errors.
                klog.ErrorS(err, "OperationExecutor.UnregisterPlugin failed", "plugin", registeredPlugin)
            }
            if err == nil {
                klog.V(1).InfoS("OperationExecutor.UnregisterPlugin started", "plugin", registeredPlugin)
            }
        }
    }

    // Ensure plugins that should be registered are registered
    for _, pluginToRegister := range rc.desiredStateOfWorld.GetPluginsToRegister() {
        if !rc.actualStateOfWorld.PluginExistsWithCorrectTimestamp(pluginToRegister) {
            klog.V(5).InfoS("Starting operationExecutor.RegisterPlugin", "plugin", pluginToRegister)
            err := rc.operationExecutor.RegisterPlugin(pluginToRegister.SocketPath, pluginToRegister.Timestamp, rc.getHandlers(), rc.actualStateOfWorld)
            if err != nil &&
                !goroutinemap.IsAlreadyExists(err) &&
                !exponentialbackoff.IsExponentialBackoff(err) {
                // Ignore goroutinemap.IsAlreadyExists and exponentialbackoff.IsExponentialBackoff errors, they are expected.
                klog.ErrorS(err, "OperationExecutor.RegisterPlugin failed", "plugin", pluginToRegister)
            }
            if err == nil {
                klog.V(1).InfoS("OperationExecutor.RegisterPlugin started", "plugin", pluginToRegister)
            }
        }
    }
}
```
#### OperationExector
`OperationExector`是一个接口，主要作用是保证对于每一个Plugin的注册Socket只会产生一个协程操作：
```
type OperationExecutor interface {
    // RegisterPlugin 使用handler map中的handler注册plugin，完成之后更新actual state.
    RegisterPlugin(socketPath string, timestamp time.Time, pluginHandlers map[string]cache.PluginHandler, actualStateOfWorld ActualStateOfWorldUpdater) error

    // UnregisterPlugin 使用handler map中的handler注销plugin，完成之后更新actual state
    UnregisterPlugin(pluginInfo cache.PluginInfo, actualStateOfWorld ActualStateOfWorldUpdater) error
}
```

创建实例：
```
func NewOperationExecutor(
    operationGenerator OperationGenerator) OperationExecutor {

    return &operationExecutor{
        pendingOperations:  goroutinemap.NewGoRoutineMap(true /* exponentialBackOffOnError */),
        operationGenerator: operationGenerator,
    }
}
type operationExecutor struct {
    // pendingOperations keeps track of pending attach and detach operations so
    // multiple operations are not started on the same volume
    pendingOperations goroutinemap.GoRoutineMap

    // operationGenerator is an interface that provides implementations for
    // generating volume function
    operationGenerator OperationGenerator
}

func (oe *operationExecutor) RegisterPlugin(
    socketPath string,
    timestamp time.Time,
    pluginHandlers map[string]cache.PluginHandler,
    actualStateOfWorld ActualStateOfWorldUpdater) error {
    generatedOperation :=
        oe.operationGenerator.GenerateRegisterPluginFunc(socketPath, timestamp, pluginHandlers, actualStateOfWorld)

    return oe.pendingOperations.Run(
        socketPath, generatedOperation)
}

func (oe *operationExecutor) UnregisterPlugin(
    pluginInfo cache.PluginInfo,
    actualStateOfWorld ActualStateOfWorldUpdater) error {
    generatedOperation :=
        oe.operationGenerator.GenerateUnregisterPluginFunc(pluginInfo, actualStateOfWorld)

    return oe.pendingOperations.Run(
        pluginInfo.SocketPath, generatedOperation)
}
```

#### OperationGenerator
```
    reconciler := reconciler.NewReconciler(
        operationexecutor.NewOperationExecutor(
            operationexecutor.NewOperationGenerator(
                recorder,
            ),
        ),
        loopSleepDuration,
        dsw,
        asw,
    )
```
`OperationGenerator`是一个interface，主要用来生成注册和注销Plugin的方法：
```
// OperationGenerator interface that extracts out the functions from operation_executor to make it dependency injectable
type OperationGenerator interface {
    // Generates the RegisterPlugin function needed to perform the registration of a plugin
    GenerateRegisterPluginFunc(
        socketPath string,
        timestamp time.Time,
        pluginHandlers map[string]cache.PluginHandler,
        actualStateOfWorldUpdater ActualStateOfWorldUpdater) func() error

    // Generates the UnregisterPlugin function needed to perform the unregistration of a plugin
    GenerateUnregisterPluginFunc(
        pluginInfo cache.PluginInfo,
        actualStateOfWorldUpdater ActualStateOfWorldUpdater) func() error
}
```

生成的RegisterPlugin方法使用两个gRPC接口：
- GetInfo：用来获取Plugin的基本信息，包括Plugin Socket Endpoint。
- NotifyRegistrationStatus：用来通知plugin注册的结果

并调用handler中的ValidatePlugin和RegisterPlugin，完成之后将handler同plugininfo一起放入actualStateOfWorld中。

生成的UnRegisterPlugin方法不使用gRPC接口，只调用handler中的DeRegisterPlugin方法，完成之后从actualStateOfWorld中移除plugin。

```
func (og *operationGenerator) GenerateRegisterPluginFunc(
    socketPath string,
    timestamp time.Time,
    pluginHandlers map[string]cache.PluginHandler,
    actualStateOfWorldUpdater ActualStateOfWorldUpdater) func() error {

    registerPluginFunc := func() error {
        client, conn, err := dial(socketPath, dialTimeoutDuration)
        if err != nil {
            return fmt.Errorf("RegisterPlugin error -- dial failed at socket %s, err: %v", socketPath, err)
        }
        defer conn.Close()

        ctx, cancel := context.WithTimeout(context.Background(), time.Second)
        defer cancel()

        infoResp, err := client.GetInfo(ctx, &registerapi.InfoRequest{})
        if err != nil {
            return fmt.Errorf("RegisterPlugin error -- failed to get plugin info using RPC GetInfo at socket %s, err: %v", socketPath, err)
        }

        handler, ok := pluginHandlers[infoResp.Type]
        if !ok {
            if err := og.notifyPlugin(client, false, fmt.Sprintf("RegisterPlugin error -- no handler registered for plugin type: %s at socket %s", infoResp.Type, socketPath)); err != nil {
                return fmt.Errorf("RegisterPlugin error -- failed to send error at socket %s, err: %v", socketPath, err)
            }
            return fmt.Errorf("RegisterPlugin error -- no handler registered for plugin type: %s at socket %s", infoResp.Type, socketPath)
        }

        if infoResp.Endpoint == "" {
            infoResp.Endpoint = socketPath
        }
        if err := handler.ValidatePlugin(infoResp.Name, infoResp.Endpoint, infoResp.SupportedVersions); err != nil {
            if err = og.notifyPlugin(client, false, fmt.Sprintf("RegisterPlugin error -- plugin validation failed with err: %v", err)); err != nil {
                return fmt.Errorf("RegisterPlugin error -- failed to send error at socket %s, err: %v", socketPath, err)
            }
            return fmt.Errorf("RegisterPlugin error -- pluginHandler.ValidatePluginFunc failed")
        }
        // We add the plugin to the actual state of world cache before calling a plugin consumer's Register handle
        // so that if we receive a delete event during Register Plugin, we can process it as a DeRegister call.
        err = actualStateOfWorldUpdater.AddPlugin(cache.PluginInfo{
            SocketPath: socketPath,
            Timestamp:  timestamp,
            Handler:    handler,
            Name:       infoResp.Name,
        })
        if err != nil {
            klog.ErrorS(err, "RegisterPlugin error -- failed to add plugin", "path", socketPath)
        }
        if err := handler.RegisterPlugin(infoResp.Name, infoResp.Endpoint, infoResp.SupportedVersions); err != nil {
            return og.notifyPlugin(client, false, fmt.Sprintf("RegisterPlugin error -- plugin registration failed with err: %v", err))
        }

        // Notify is called after register to guarantee that even if notify throws an error Register will always be called after validate
        if err := og.notifyPlugin(client, true, ""); err != nil {
            return fmt.Errorf("RegisterPlugin error -- failed to send registration status at socket %s, err: %v", socketPath, err)
        }
        return nil
    }
    return registerPluginFunc
}

func (og *operationGenerator) GenerateUnregisterPluginFunc(
    pluginInfo cache.PluginInfo,
    actualStateOfWorldUpdater ActualStateOfWorldUpdater) func() error {

    unregisterPluginFunc := func() error {
        if pluginInfo.Handler == nil {
            return fmt.Errorf("UnregisterPlugin error -- failed to get plugin handler for %s", pluginInfo.SocketPath)
        }
        // We remove the plugin to the actual state of world cache before calling a plugin consumer's Unregister handle
        // so that if we receive a register event during Register Plugin, we can process it as a Register call.
        actualStateOfWorldUpdater.RemovePlugin(pluginInfo.SocketPath)

        pluginInfo.Handler.DeRegisterPlugin(pluginInfo.Name)

        klog.V(4).InfoS("DeRegisterPlugin called", "pluginName", pluginInfo.Name, "pluginHandler", pluginInfo.Handler)
        return nil
    }
    return unregisterPluginFunc
}

func (og *operationGenerator) notifyPlugin(client registerapi.RegistrationClient, registered bool, errStr string) error {
    ctx, cancel := context.WithTimeout(context.Background(), notifyTimeoutDuration)
    defer cancel()

    status := &registerapi.RegistrationStatus{
        PluginRegistered: registered,
        Error:            errStr,
    }

    if _, err := client.NotifyRegistrationStatus(ctx, status); err != nil {
        return fmt.Errorf("%s: %w", errStr, err)
    }

    if errStr != "" {
        return errors.New(errStr)
    }

    return nil
}
```

## CSI Plugin Handler
CSI Plugin Handler需实现Plugin Handler接口：
```
type PluginHandler interface {
    // Validate returns an error if the information provided by
    // the potential plugin is erroneous (unsupported version, ...)
    ValidatePlugin(pluginName string, endpoint string, versions []string) error
    // RegisterPlugin is called so that the plugin can be register by any
    // plugin consumer
    // Error encountered here can still be Notified to the plugin.
    RegisterPlugin(pluginName, endpoint string, versions []string) error
    // DeRegister is called once the pluginwatcher observes that the socket has
    // been deleted.
    DeRegisterPlugin(pluginName string)
}
```

CSI Plugin Handler的实例为：
```
kl.pluginManager.AddHandler(pluginwatcherapi.CSIPlugin, plugincache.PluginHandler(csi.PluginHandler))
...
var PluginHandler = &RegistrationHandler{}
```

其ValidatePlugin方法主要为了检查：
- 是否已经有更高版本的Driver已经被注册

```
func (h *RegistrationHandler) ValidatePlugin(pluginName string, endpoint string, versions []string) error {
    klog.Infof(log("Trying to validate a new CSI Driver with name: %s endpoint: %s versions: %s",
        pluginName, endpoint, strings.Join(versions, ",")))

    _, err := h.validateVersions("ValidatePlugin", pluginName, endpoint, versions)
    if err != nil {
        return fmt.Errorf("validation failed for CSI Driver %s at endpoint %s: %v", pluginName, endpoint, err)
    }

    return err
}
//检查CSI Driver注册的plugin是否已经有更高的版本被注册过
func (h *RegistrationHandler) validateVersions(callerName, pluginName string, endpoint string, versions []string) (*utilversion.Version, error) {
    if len(versions) == 0 {
        return nil, errors.New(log("%s for CSI driver %q failed. Plugin returned an empty list for supported versions", callerName, pluginName))
    }

    // Validate version
    newDriverHighestVersion, err := highestSupportedVersion(versions)
    if err != nil {
        return nil, errors.New(log("%s for CSI driver %q failed. None of the versions specified %q are supported. err=%v", callerName, pluginName, versions, err))
    }

    existingDriver, driverExists := csiDrivers.Get(pluginName)
    if driverExists {
        if !existingDriver.highestSupportedVersion.LessThan(newDriverHighestVersion) {
            return nil, errors.New(log("%s for CSI driver %q failed. Another driver with the same name is already registered with a higher supported version: %q", callerName, pluginName, existingDriver.highestSupportedVersion))
        }
    }

    return newDriverHighestVersion, nil
}
```

RegisterPlugin方法主要包括：
- 获取CSI支持的最高版本
- 将Driver的endpoint和最高版本保存在csiDriver的集合中
- 从csiDriver获取gRPC endpoint初始化Client
- 调用NodeGetInfo接口，更新Node对象的node ID annotation，并且更新CSINode对象中的CSIDrivers字段，如果CSINode对象不存在，则创建一个新的。

```
// RegisterPlugin is called when a plugin can be registered
func (h *RegistrationHandler) RegisterPlugin(pluginName string, endpoint string, versions []string) error {
    klog.Infof(log("Register new plugin with name: %s at endpoint: %s", pluginName, endpoint))

    highestSupportedVersion, err := h.validateVersions("RegisterPlugin", pluginName, endpoint, versions)
    if err != nil {
        return err
    }

    // Storing endpoint of newly registered CSI driver into the map, where CSI driver name will be the key
    // all other CSI components will be able to get the actual socket of CSI drivers by its name.
    csiDrivers.Set(pluginName, Driver{
        endpoint:                endpoint,
        highestSupportedVersion: highestSupportedVersion,
    })

    // Get node info from the driver.
    csi, err := newCsiDriverClient(csiDriverName(pluginName))
    if err != nil {
        return err
    }

    ctx, cancel := context.WithTimeout(context.Background(), csiTimeout)
    defer cancel()

    driverNodeID, maxVolumePerNode, accessibleTopology, err := csi.NodeGetInfo(ctx)
    if err != nil {
        if unregErr := unregisterDriver(pluginName); unregErr != nil {
            klog.Error(log("registrationHandler.RegisterPlugin failed to unregister plugin due to previous error: %v", unregErr))
        }
        return err
    }

    err = nim.InstallCSIDriver(pluginName, driverNodeID, maxVolumePerNode, accessibleTopology)
    if err != nil {
        if unregErr := unregisterDriver(pluginName); unregErr != nil {
            klog.Error(log("registrationHandler.RegisterPlugin failed to unregister plugin due to previous error: %v", unregErr))
        }
        return err
    }

    return nil
}
```

DeRegisterPlugin主要包括：
- 从csiDrivers中删除plugin
- 清理Node对象的node ID annotation，并且清除CSINode对象中的CSIDrivers字段。
```
func (h *RegistrationHandler) DeRegisterPlugin(pluginName string) {
    klog.Info(log("registrationHandler.DeRegisterPlugin request for plugin %s", pluginName))
    if err := unregisterDriver(pluginName); err != nil {
        klog.Error(log("registrationHandler.DeRegisterPlugin failed: %v", err))
    }
}
func unregisterDriver(driverName string) error {
    csiDrivers.Delete(driverName)

    if err := nim.UninstallCSIDriver(driverName); err != nil {
        return errors.New(log("Error uninstalling CSI driver: %v", err))
    }

    return nil
}
```
## ContainerManger PluginRegistrationHandler

### 使用Plugin Manager
我们首先找到Device Plugin的handle所在。

```
kl.pluginManager.AddHandler(pluginwatcherapi.DevicePlugin, kl.containerManager.GetPluginRegistrationHandler())

...

kubeDeps.ContainerManager, err = cm.NewContainerManager(...)

...

func NewContainerManager(mountUtil mount.Interface, cadvisorInterface cadvisor.Interface, nodeConfig NodeConfig, failSwapOn bool, devicePluginEnabled bool, recorder record.EventRecorder) (ContainerManager, error) {
    ...
    cm := &containerManagerImpl{
        cadvisorInterface:   cadvisorInterface,
        mountUtil:           mountUtil,
        NodeConfig:          nodeConfig,
        subsystems:          subsystems,
        cgroupManager:       cgroupManager,
        capacity:            capacity,
        internalCapacity:    internalCapacity,
        cgroupRoot:          cgroupRoot,
        recorder:            recorder,
        qosContainerManager: qosContainerManager,
    }
    ...
    klog.InfoS("Creating device plugin manager", "devicePluginEnabled", devicePluginEnabled)
    if devicePluginEnabled {
        cm.deviceManager, err = devicemanager.NewManagerImpl(machineInfo.Topology, cm.topologyManager)
        cm.topologyManager.AddHintProvider(cm.deviceManager)
    } else {
        cm.deviceManager, err = devicemanager.NewManagerStub()
    }
    ...
}

...

func (cm *containerManagerImpl) GetPluginRegistrationHandler() cache.PluginHandler {
    return cm.deviceManager.GetWatcherHandler()
}

...

// NewManagerImpl creates a new manager.
func NewManagerImpl(topology []cadvisorapi.Node, topologyAffinityStore topologymanager.Store) (*ManagerImpl, error) {
    socketPath := pluginapi.KubeletSocket
    if runtime.GOOS == "windows" {
        socketPath = os.Getenv("SYSTEMDRIVE") + pluginapi.KubeletSocketWindows
    }
    return newManagerImpl(socketPath, topology, topologyAffinityStore)
}


// GetWatcherHandler returns the plugin handler
func (m *ManagerImpl) GetWatcherHandler() cache.PluginHandler {
    // socketdir: /var/lib/kubelet/device-plugins/
    if f, err := os.Create(m.socketdir + "DEPRECATION"); err != nil {
        klog.ErrorS(err, "Failed to create deprecation file at socket dir", "path", m.socketdir)
    } else {
        f.Close()
        klog.V(4).InfoS("Created deprecation file", "path", f.Name())
    }
    return cache.PluginHandler(m)
}
```

顺藤摸瓜，我们找到了Device Manger实现的Plugin Handler。

ValidatePlugin负责：
1. 检查plugin是否含有pluginapi.SupportedVersions，目前只包含v1beta1。
2. 检查pluginName的格式是否符合ExtendedResourceName

RegisterPlugin负责：
1. 调用GetDevicePluginOptions接口
2. 将plugin信息放入m.endpoints
3. 调用ListAndWatch接口，更新该plugin所管辖的资源状态


DeRegisterPlugin负责：
1. 停止m.endpoints中相应gRPC链接，并更新停止时间。

```
// ValidatePlugin validates a plugin if the version is correct and the name has the format of an extended resource
func (m *ManagerImpl) ValidatePlugin(pluginName string, endpoint string, versions []string) error {
    klog.V(2).InfoS("Got Plugin at endpoint with versions", "plugin", pluginName, "endpoint", endpoint, "versions", versions)

    if !m.isVersionCompatibleWithPlugin(versions) {
        return fmt.Errorf("manager version, %s, is not among plugin supported versions %v", pluginapi.Version, versions)
    }

    if !v1helper.IsExtendedResourceName(v1.ResourceName(pluginName)) {
        return fmt.Errorf("invalid name of device plugin socket: %s", fmt.Sprintf(errInvalidResourceName, pluginName))
    }

    return nil
}

// RegisterPlugin starts the endpoint and registers it
// TODO: Start the endpoint and wait for the First ListAndWatch call
//       before registering the plugin
func (m *ManagerImpl) RegisterPlugin(pluginName string, endpoint string, versions []string) error {
    klog.V(2).InfoS("Registering plugin at endpoint", "plugin", pluginName, "endpoint", endpoint)

    e, err := newEndpointImpl(endpoint, pluginName, m.callback)
    if err != nil {
        return fmt.Errorf("failed to dial device plugin with socketPath %s: %v", endpoint, err)
    }

    options, err := e.client.GetDevicePluginOptions(context.Background(), &pluginapi.Empty{})
    if err != nil {
        return fmt.Errorf("failed to get device plugin options: %v", err)
    }

    m.registerEndpoint(pluginName, options, e)
    go m.runEndpoint(pluginName, e)

    return nil
}

// DeRegisterPlugin deregisters the plugin
// TODO work on the behavior for deregistering plugins
// e.g: Should we delete the resource
func (m *ManagerImpl) DeRegisterPlugin(pluginName string) {
    m.mutex.Lock()
    defer m.mutex.Unlock()

    // Note: This will mark the resource unhealthy as per the behavior
    // in runEndpoint
    if eI, ok := m.endpoints[pluginName]; ok {
        eI.e.stop()
    }
}
```

### 使用Register接口

Device Plugin还可以调用Kubelet提供的Register接口注册服务。Register接口提供的服务和ValidatePlugin + RegisterPlugin 接口提供的服务是一样的。区别在于Plugin Manager会自动发现注册Socket，然后找到Plugin Socket。而Register接口则是要求Plugin主动向Kubelet发起注册请求。

```
// Register registers a device plugin.
func (m *ManagerImpl) Register(ctx context.Context, r *pluginapi.RegisterRequest) (*pluginapi.Empty, error) {
    klog.InfoS("Got registration request from device plugin with resource", "resourceName", r.ResourceName)
    metrics.DevicePluginRegistrationCount.WithLabelValues(r.ResourceName).Inc()
    var versionCompatible bool
    for _, v := range pluginapi.SupportedVersions {
        if r.Version == v {
            versionCompatible = true
            break
        }
    }
    if !versionCompatible {
        err := fmt.Errorf(errUnsupportedVersion, r.Version, pluginapi.SupportedVersions)
        klog.InfoS("Bad registration request from device plugin with resource", "resourceName", r.ResourceName, "err", err)
        return &pluginapi.Empty{}, err
    }

    if !v1helper.IsExtendedResourceName(v1.ResourceName(r.ResourceName)) {
        err := fmt.Errorf(errInvalidResourceName, r.ResourceName)
        klog.InfoS("Bad registration request from device plugin", "err", err)
        return &pluginapi.Empty{}, err
    }

    // TODO: for now, always accepts newest device plugin. Later may consider to
    // add some policies here, e.g., verify whether an old device plugin with the
    // same resource name is still alive to determine whether we want to accept
    // the new registration.
    go m.addEndpoint(r)

    return &pluginapi.Empty{}, nil
}

func (m *ManagerImpl) addEndpoint(r *pluginapi.RegisterRequest) {
    new, err := newEndpointImpl(filepath.Join(m.socketdir, r.Endpoint), r.ResourceName, m.callback)
    if err != nil {
        klog.ErrorS(err, "Failed to dial device plugin with request", "request", r)
        return
    }
    m.registerEndpoint(r.ResourceName, r.Options, new)
    go func() {
        m.runEndpoint(r.ResourceName, new)
    }()
}
```

## Summary
[Plugin Watcher Utility](https://github.com/kubernetes/design-proposals-archive/blob/main/node/plugin-watcher.md)
![Image](../../../assets/images/posts/kubelet_registration.drawio.png){:.rounded}