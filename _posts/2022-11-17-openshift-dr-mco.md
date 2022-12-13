---
title: ODF Disaster Recovery Multicluster Orchestrator
tags: ODF DR MulticlusterOrchestrator
---

ODF Multicluster Orcherstrator 的代码位于[https://github.com/red-hat-storage/odf-multicluster-orchestrator](https://github.ibm.com/ProjectAbell/isf-metrodr-operator)

<!--more-->

它主要包括两个部分：
- Hub Controller：部署在Hub Cluster上
  - MirrorPeerReconciler
  - MirrorPeerSecretReconciler
  - MirrorPeerWebhook
  - Init Console
  - TokenExchangeAddonManager
- Token Exchanger Agent：部署在managed Cluster上

```
func main() {
	cmd.Execute()
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(controllers.NewManagerCommand())
}

func init() {
	rootCmd.AddCommand(tokenexchange.NewAgentCommand())
}
```

## Hub Controller：
Hub Controller主要包含以下几种组件：
- MirrorPeerWebhook
- Init Console
- MirrorPeerReconciler
- MirrorPeerSecretReconciler
- TokenExchangeAddon

### MirrorPeerWebhook
Hub Controller中包含两个webhook对MirrorPeer设置Default值和校验：
- DefaultWebook：用于给MirrorPeer设置default值。
```
func (r *MirrorPeer) Default() {
	if len(r.Spec.SchedulingIntervals) == 0 {
		mirrorpeerlog.Info("No values set for schedulingIntervals, defaulting to 5m", "MirrorPeer", r.ObjectMeta.Name)
		r.Spec.SchedulingIntervals = []string{"5m"}
	}
}
```

- ValidationWebhook：验证MirrorPeer的内容是否合规。
  - Create：Spec.Items不能为nil，Spec.SchedulingIntervals不能为空，interval需满足`^\d+[mhd]$`。
  - Update：Spec.Type不能修改，Spec.Items不能改，同Create验证。
  - Delete：不做验证。

```
// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *MirrorPeer) ValidateCreate() error {
	mirrorpeerlog.Info("validate create", "name", r.ObjectMeta.Name)

	return validateMirrorPeer(r)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *MirrorPeer) ValidateUpdate(old runtime.Object) error {
	mirrorpeerlog.Info("validate update", "name", r.ObjectMeta.Name)
	oldMirrorPeer, ok := old.(*MirrorPeer)
	if !ok {
		return fmt.Errorf("error casting old object to MirrorPeer")
	}

	if len(r.Spec.Items) != len(oldMirrorPeer.Spec.Items) {
		return fmt.Errorf("error updating MirrorPeer, new and old spec.items have different lengths")
	}

	if r.Spec.Type != oldMirrorPeer.Spec.Type {
		return fmt.Errorf("error updating MirrorPeer, the type cannot be changed from %s to %s", oldMirrorPeer.Spec.Type, r.Spec.Type)
	}

	refs := make(map[string]int)
	for idx, pr := range oldMirrorPeer.Spec.Items {
		// Creating a make-shift set of references to check for duplicates
		refs[fmt.Sprintf("%s-%s-%s", pr.ClusterName, pr.StorageClusterRef.Namespace, pr.StorageClusterRef.Name)] = idx
	}

	for _, pr := range r.Spec.Items {
		key := fmt.Sprintf("%s-%s-%s", pr.ClusterName, pr.StorageClusterRef.Namespace, pr.StorageClusterRef.Name)
		if _, ok := refs[key]; !ok {
			return fmt.Errorf("error validating update: new MirrorPeer %s references a StorageCluster %s/%s that is not in the old MirrorPeer", r.ObjectMeta.Name, pr.StorageClusterRef.Namespace, pr.StorageClusterRef.Name)
		}
	}
	return validateMirrorPeer(r)
}

/*
  validateSchedulingInterval checks if the scheduling interval is valid and ending with a 'm' or 'h' or 'd'
  else it will return error.
*/
func validateSchedulingInterval(interval string) error {
	re := regexp.MustCompile(`^\d+[mhd]$`)
	if re.MatchString(interval) {
		return nil
	}
	return fmt.Errorf("invalid scheduling interval %q. interval should have suffix 'm|h|d' ", interval)
}

// validateMirrorPeer validates the MirrorPeer
func validateMirrorPeer(instance *MirrorPeer) error {

	if instance.Spec.Items == nil {
		return fmt.Errorf("Spec.Items can not be nil")
	}

	if instance.Spec.SchedulingIntervals == nil || len(instance.Spec.SchedulingIntervals) == 0 {
		return fmt.Errorf("Spec.SchedulingIntervals can not be nil or empty")
	}

	for _, interval := range instance.Spec.SchedulingIntervals {
		if err := validateSchedulingInterval(interval); err != nil {
			return err
		}
	}
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *MirrorPeer) ValidateDelete() error {
	mirrorpeerlog.Info("validate delete", "name", r.ObjectMeta.Name)
	return nil
}
```


### Init Console

- 检查是否有名为`odf-multicluster-console`的deployment
- 创建`odf-multicluster-console` service， 指向该deployment的pod
- 创建`odf-multicluster-console` ConsolePlugin，包含该service

```
if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
	err = console.InitConsole(ctx, mgr.GetClient(), o.MulticlusterConsolePort, namespace)
	if err != nil {
		setupLog.Error(err, "unable to initialize multicluster console to manager")
		return err
	}
	return nil
})); err != nil {
	setupLog.Error(err, "unable to add multicluster console to manager")
	os.Exit(1)
}
```

### TokenExchangeAddonManager

创建tokenExchangeAddon Manager，该Manager可以通过创建ManagedClusterAddOn在managed Cluster上部署tokenExchangeAddon:

```
agentImage := os.Getenv("TOKEN_EXCHANGE_IMAGE")

	tokenExchangeAddon := tokenexchange.TokenExchangeAddon{
		KubeClient: kubeClient,
		Recorder:   eventRecorder,
		AgentImage: agentImage,
	}

	err = (&multiclusterv1alpha1.MirrorPeer{}).SetupWebhookWithManager(mgr)

	if err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MirrorPeer")
		os.Exit(1)
	}

	setupLog.Info("creating addon manager")
	addonMgr, err := addonmanager.New(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "problem creating addon manager")
	}
	err = addonMgr.AddAgent(&tokenExchangeAddon)
	if err != nil {
		setupLog.Error(err, "problem adding token exchange addon to addon manager")
	}
```

### MirrorPeerReconciler

1. 只有spec发生变化的MirrorPeer才会出发MirrorPeerReconciler
```
// SetupWithManager sets up the controller with the Manager.
func (r *MirrorPeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mpPredicate := utils.ComposePredicates(predicate.GenerationChangedPredicate{})
	return ctrl.NewControllerManagedBy(mgr).
		For(&multiclusterv1alpha1.MirrorPeer{}, builder.WithPredicates(mpPredicate)).
		Complete(r)
}
```
2. 检查MirrorPeer是否满足要求
```
	logger.V(2).Info("Validating MirrorPeer", "MirrorPeer", req.NamespacedName)
	// Validate MirrorPeer
	// MirrorPeer.Spec must be defined
	if err := undefinedMirrorPeerSpec(mirrorPeer.Spec); err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	// MirrorPeer.Spec.Items must be unique
	if err := uniqueSpecItems(mirrorPeer.Spec); err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	for i := range mirrorPeer.Spec.Items {
		// MirrorPeer.Spec.Items must not have empty fields
		if err := emptySpecItems(mirrorPeer.Spec.Items[i]); err != nil {
			// return error and do not requeue since user needs to update the spec
			// when user updates the spec, new reconcile will be triggered
			return reconcile.Result{Requeue: false}, err
		}
		// MirrorPeer.Spec.Items[*].ClusterName must be a valid ManagedCluster
		if err := isManagedCluster(ctx, r.Client, mirrorPeer.Spec.Items[i].ClusterName); err != nil {
			return ctrl.Result{}, err
		}
	}
	logger.V(2).Info("All validations for MirrorPeer passed", "MirrorPeer", req.NamespacedName)
```
   - spec中必须含有内容
   - spec中的Item不能重复，并且Item的内容不能为空
   - Item中的ClusterName必须是一个ManagedCluster
3. 检查GetDeletionTimestamp：
```
	if mirrorPeer.GetDeletionTimestamp().IsZero() {
		if !utils.ContainsString(mirrorPeer.GetFinalizers(), mirrorPeerFinalizer) {
			logger.Info("Finalizer not found on MirrorPeer. Adding Finalizer")
			mirrorPeer.Finalizers = append(mirrorPeer.Finalizers, mirrorPeerFinalizer)
		}
	} else {
		logger.Info("Deleting MirrorPeer")
		mirrorPeer.Status.Phase = multiclusterv1alpha1.Deleting
		statusErr := r.Client.Status().Update(ctx, &mirrorPeer)
		if statusErr != nil {
			logger.Error(statusErr, "Error occurred while updating the status of mirrorpeer", "MirrorPeer", mirrorPeer)
			return ctrl.Result{Requeue: true}, nil
		}
		if utils.ContainsString(mirrorPeer.GetFinalizers(), mirrorPeerFinalizer) {
			if utils.ContainsSuffix(mirrorPeer.GetFinalizers(), addons.SpokeMirrorPeerFinalizer) {
				logger.Info("Waiting for agent to delete resources")
				return reconcile.Result{Requeue: true}, err
			}
			if err := r.deleteSecrets(ctx, mirrorPeer); err != nil {
				logger.Error(err, "Failed to delete resources")
				return reconcile.Result{Requeue: true}, err
			}
			mirrorPeer.Finalizers = utils.RemoveString(mirrorPeer.Finalizers, mirrorPeerFinalizer)
			if err := r.Client.Update(ctx, &mirrorPeer); err != nil {
				logger.Info("Failed to remove finalizer from MirrorPeer")
				return reconcile.Result{}, err
			}
		}
		logger.Info("MirrorPeer deleted, skipping reconcilation")
		return reconcile.Result{}, nil
	}
	...
func (r *MirrorPeerReconciler) deleteSecrets(ctx context.Context, mirrorPeer multiclusterv1alpha1.MirrorPeer) error {
	logger := log.FromContext(ctx)
	for i := range mirrorPeer.Spec.Items {
		peerRefUsed, err := utils.DoesAnotherMirrorPeerPointToPeerRef(ctx, r.Client, &mirrorPeer.Spec.Items[i])
		if err != nil {
			return err
		}
		if !peerRefUsed {
			secretLabels := []string{string(utils.SourceLabel), string(utils.DestinationLabel)}

			if mirrorPeer.Spec.ManageS3 {
				secretLabels = append(secretLabels, string(utils.InternalLabel))
			}

			secretRequirement, err := labels.NewRequirement(utils.SecretLabelTypeKey, selection.In, secretLabels)
			if err != nil {
				logger.Error(err, "cannot parse new requirement")
			}

			secretSelector := labels.NewSelector().Add(*secretRequirement)

			deleteOpt := client.DeleteAllOfOptions{
				ListOptions: client.ListOptions{
					Namespace:     mirrorPeer.Spec.Items[i].ClusterName,
					LabelSelector: secretSelector,
				},
			}

			var secret corev1.Secret
			if err := r.DeleteAllOf(ctx, &secret, &deleteOpt); err != nil {
				logger.Error(err, "Error while deleting secrets for MirrorPeer", "MirrorPeer", mirrorPeer.Name)
			}
		}
	}
	return nil
}
```
   - 如果为0，则添加`hub.multicluster.odf.openshift.io` finalizer
   - 如果不为0，则执行删除操作，并返回。删除操作包括：
     -  如果没有其他MirrorPeer的Item包括了当前Cluster，才会真正执行删除操作。否则直接返回
     -  在Hub cluster上，使用label `multicluster.odf.openshift.io/secret-type` in `BLUE`, `GREEN`, `INTERNAL`(如果mirrorPeer.Spec.ManageS3 = true)获取所有的secrete，并删除。
     -  删除`hub.multicluster.odf.openshift.io` finalizer
4. 设置label `cluster.open-cluster-management.io/backup: resource`
5. 如果`mirrorPeer.Status.Phase == ""`检查 `mirrorPeer.Spec.Type`:
   - 如果mirrorPeer.Spec.Type是Async，则设置mirrorPeer.Status.Phase为ExchangingSecret
   - 如果mirrorPeer.Spec.Type是Sync，则设置mirrorPeer.Status.Phase为S3ProfileSyncing
6. 为每一个mirrorPeer.Spec.Item的Cluster创建ManagedClusterAddOn
7. 在Spoke上创建Addon所需要的RBAC
8. 如果mirrorPeer.Spec.ManageS3 == true，对于每一个peerRef(即每一个cluster的每一个storage cluster)
   - 根据peerRef + `s3profile` prefix 生成的secret name获取secret。
   - secret应该具有`multicluster.odf.openshift.io/secret-type: INTERNAL`
   - secret的data中应该包含如下字段：
     - namespace(storage cluster namespace)
     - storage-cluster-name
     - secret-origin: `S3Origin | RookOrigin`
     - secret-data:
       -  s3ProfileName
       -  s3Bucket
       -  s3CompatibleEndpoint
       -  s3Region
       -  AWS_ACCESS_KEY_ID
       -  AWS_SECRET_ACCESS_KEY
   - 检查secret中的key secret-origin：
     - 如果是 S3Origin：
       - 在Ramen namespace下创建一个secret，名字同 secret 的名字一样，具有label `multicluster.odf.openshift.io/created-by mirrorpeersecret`。 包含AWS_ACCESS_KEY_ID和AWS_SECRET_ACCESS_KEY两个key中的数据。
       - 在Ramen namespace下创建一个configmap，名字为`ramen-hub-operator-config`, data 中含有一个名为`ramen_manager_config.yaml` key, 内部含有一个S3StoreProfiles list，向其增加或者修改如下内容
         - S3ProfileName:        string(data[utils.S3ProfileName]),
		 - S3Bucket:             string(data[utils.S3BucketName]),
		 - S3Region:             string(data[utils.S3Region]),
		 - S3CompatibleEndpoint: string(data[utils.S3Endpoint]),
		 - S3SecretRef：指向刚才创建的secret
     - 如果是 RookOrigin
       -  在Ramen namespace下创建一个secret，名字同 secret 的名字一样，具有label `multicluster.odf.openshift.io/created-by mirrorpeersecret`。具有key `fsid`, 其值从secret的data["fsid"]中获取。
9. 同步Source secret
   - 遍历每一个 mirrorPeerObj.Spec.Items：
     -  并从cluster namespace中获取所有的source secret，从中找到匹配该PeerRef的secret
     -  根据MirrorPeer找到该secret所代表的PeerRef的所有uniqueConnectedPeers
     -  在每一个uniqueConnectedPeers的cluster namespace中创建一个同名的secret，但包含该uniqueConnectedPeer的storageClusterNamespacedName
   - 获取cluster namespace中的所有Green secret，对于每一个Green secret尝试找到Blue secret，如果没有，则删除该Green secret，如果有，则更新Green secret
10. 创建DR Clusters： 遍历mirrorPeer的每一个item（每一个cluster）
    - 获取fsid，并赋值给drcluster.Spec.Region
      - 如果是Sync，则从pod所在的namespace获取secret， secret的名字是从PeerRef生成的
      - 如果是Async，则从cluster namespace中获取 secret，secret的名字是从PeerRef生成的
    - 从cluster namespace中获取s3 secret，secret的名字是`s3profile+<PeerRef生成的>`。并将s3profile 复制给drcluster.Spec.S3ProfileName
    - 创建dr cluster 资源。

### MirrorPeerSecretReconciler

MirrorPeerSecretReconciler会watch 满足如下条件的secret：
- Create：Source或者Internal secret的创建
- Delete：Source或者Destination secret的创建
- UpdateFunc： Source或者Destination或者Internal secret的创建

MirrorPeerSecretReconciler reconcile主要执行一下任务：
1. 一但发现有secret被删除，则先定位删除的是source secret还是destination secret。
  - 如果是source，则需要把对应的destination secret全部删除。
  - 如果是destination，则需要找到对应的source，并更新destination
2. 如果发现source secret有变化，则更新对应的全部destination secret
3. 如果发现destination secret有变化，则获取到source并更新对应的全部destination secret
4. 如果是internal secret有变化，则更新pod namespace中对应的secret和configmap


## Token Exchange Agent:

Token Exchange Agent 需要通过命令行指定三个参数：
- HubKubeconfigFile 
-	SpokeClusterName  
-	DRMode       

```
// AgentOptions defines the flags for agent
type AgentOptions struct {
	HubKubeconfigFile string
	SpokeClusterName  string
	DRMode            string
}

// NewAgentOptions returns the flags with default value set
func NewAgentOptions() *AgentOptions {
	return &AgentOptions{}
}

func (o *AgentOptions) AddFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVar(&o.HubKubeconfigFile, "hub-kubeconfig", o.HubKubeconfigFile, "Location of kubeconfig file to connect to hub cluster.")
	flags.StringVar(&o.SpokeClusterName, "cluster-name", o.SpokeClusterName, "Name of spoke cluster.")
	flags.StringVar(&o.DRMode, "mode", o.DRMode, "The DR mode of token exchange addon. Valid values are: 'sync', 'async'")
}
```

Token Exchange Agent主要启动如下组件：
- HealthProbes
- GreenSecretAgent
- BlueSecretAgent
- MirrorPeerReconciler

### ServeHealthProbes
```
func (o *AgentOptions) RunAgent(ctx context.Context, controllerContext *controllercmd.ControllerContext) error {
...
	cc, err := addonutils.NewConfigChecker("agent kubeconfig checker", o.HubKubeconfigFile)
	if err != nil {
		return err
	}

	go ServeHealthProbes(ctx.Done(), ":8000", cc.Check)
  ...
}
```

```
// ServeHealthProbes starts a server to check healthz and readyz probes
func ServeHealthProbes(stop <-chan struct{}, healthProbeBindAddress string, configCheck healthz.Checker) {
	healthzHandler := &healthz.Handler{Checks: map[string]healthz.Checker{
		"healthz-ping": healthz.Ping,
		"configz-ping": configCheck,
	}}
	readyzHandler := &healthz.Handler{Checks: map[string]healthz.Checker{
		"readyz-ping": healthz.Ping,
	}}

	mux := http.NewServeMux()
	mux.Handle("/readyz", http.StripPrefix("/readyz", readyzHandler))
	mux.Handle("/healthz", http.StripPrefix("/healthz", healthzHandler))

	server := http.Server{
		Handler: mux,
	}

	ln, err := net.Listen("tcp", healthProbeBindAddress)
	if err != nil {
		klog.Errorf("error listening on %s: %v", healthProbeBindAddress, err)
		return
	}

	klog.Infof("Health probes server is running.")
	// Run server
	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			klog.Fatal(err)
		}
	}()

	// Shutdown the server when stop is closed
	<-stop
	if err := server.Shutdown(context.Background()); err != nil {
		klog.Fatal(err)
	}
}
```

ServeHealthProbes 包含两种check:
- healthz
  - healthz-ping: 只返回nil给client
  - configz-ping: 检查HubKubeconfigFile的checksum是否有变化，如果有变化，则返回error。
- readyz-ping: 只返回nil给client

### BlueSecretAgent

`BlueSecretAgent` watch spoke cluster上的secret和configmap，遍历`secretExchangeHandler.RegisteredHandlers`调用其`getBlueSecretFilter`方法过滤满足条件的secret和configmap。在触发的reconcile中遍历`secretExchangeHandler.RegisteredHandlers`调用其`syncBlueSecret`方法同步Blue Secret。

```

func newblueSecretTokenExchangeAgentController(
	hubKubeClient kubernetes.Interface,
	hubSecretInformers corev1informers.SecretInformer,
	spokeKubeClient kubernetes.Interface,
	spokeSecretInformers corev1informers.SecretInformer,
	spokeConfigMapInformers corev1informers.ConfigMapInformer,
	clusterName string,
	recorder events.Recorder,
	spokeKubeConfig *rest.Config,
) factory.Controller {
	c := &blueSecretTokenExchangeAgentController{
		hubKubeClient:        hubKubeClient,
		hubSecretLister:      hubSecretInformers.Lister(),
		spokeKubeClient:      spokeKubeClient,
		spokeSecretLister:    spokeSecretInformers.Lister(),
		spokeConfigMapLister: spokeConfigMapInformers.Lister(),
		clusterName:          clusterName,
		recorder:             recorder,
		spokeKubeConfig:      spokeKubeConfig,
	}
	klog.Infof("creating managed cluster to hub secret sync controller")

	queueKeyFn := func(obj runtime.Object) string {
		key, err := cache.MetaNamespaceKeyFunc(obj)
		if err != nil {
			return ""
		}
		return key
	}

	eventFilterFn := func(obj interface{}) bool {
		isMatched := false
		for _, handler := range secretExchangeHandler.RegisteredHandlers {
			isMatched = handler.getBlueSecretFilter(obj)
		}
		return isMatched
	}

	return factory.New().
		WithFilteredEventsInformersQueueKeyFunc(queueKeyFn, eventFilterFn, spokeSecretInformers.Informer(), spokeConfigMapInformers.Informer()).
		WithSync(c.sync).
		ToController(fmt.Sprintf("managedcluster-secret-%s-controller", TokenExchangeName), recorder)
}

// sync is the main reconcile function that syncs secret from the managed cluster to the hub cluster
func (c *blueSecretTokenExchangeAgentController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	key := syncCtx.QueueKey()
	klog.Infof("reconciling addon deploy %q", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		// ignore secret whose key is not in format: namespace/name
		return nil
	}

	for _, handler := range secretExchangeHandler.RegisteredHandlers {
		err = handler.syncBlueSecret(name, namespace, c)
		if err != nil {
			return err
		}
	}

	return nil
}
```


### GreenSecretAgent

`GreenSecretAgent` watch hub cluster上的secret，遍历`secretExchangeHandler.RegisteredHandlers`调用其`getGreenSecretFilter`方法过滤满足条件的secret。在触发的reconcile中遍历`secretExchangeHandler.RegisteredHandlers`调用其`syncGreenSecret`方法同步Green Secret

```

type greenSecretTokenExchangeAgentController struct {
	hubKubeClient     kubernetes.Interface
	hubSecretLister   corev1lister.SecretLister
	spokeKubeClient   kubernetes.Interface
	spokeSecretLister corev1lister.SecretLister
	clusterName       string
	spokeKubeConfig   *rest.Config
	recorder          events.Recorder
}

func newgreenSecretTokenExchangeAgentController(
	hubKubeClient kubernetes.Interface,
	hubSecretInformers corev1informers.SecretInformer,
	spokeKubeClient kubernetes.Interface,
	spokeSecretInformers corev1informers.SecretInformer,
	clusterName string,
	spokeKubeConfig *rest.Config,
	recorder events.Recorder,
) factory.Controller {
	c := &greenSecretTokenExchangeAgentController{
		hubKubeClient:     hubKubeClient,
		hubSecretLister:   hubSecretInformers.Lister(),
		spokeKubeClient:   spokeKubeClient,
		spokeSecretLister: spokeSecretInformers.Lister(),
		clusterName:       clusterName,
		spokeKubeConfig:   spokeKubeConfig,
		recorder:          recorder,
	}
	queueKeyFn := func(obj runtime.Object) string {
		key, err := cache.MetaNamespaceKeyFunc(obj)
		if err != nil {
			return ""
		}
		return key
	}

	eventFilterFn := func(obj interface{}) bool {
		isMatched := false
		for _, handler := range secretExchangeHandler.RegisteredHandlers {
			isMatched = isMatched || handler.getGreenSecretFilter(obj)
		}
		return isMatched
	}

	hubSecretInformer := hubSecretInformers.Informer()
	return factory.New().
		WithFilteredEventsInformersQueueKeyFunc(queueKeyFn, eventFilterFn, hubSecretInformer).
		WithSync(c.sync).
		ToController(fmt.Sprintf("%s-controller", TokenExchangeName), recorder)
}

// sync secrets with label `multicluster.odf.openshift.io/secret-type: GREEN` from hub to managed cluster
func (c *greenSecretTokenExchangeAgentController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	key := syncCtx.QueueKey()
	klog.V(4).Infof("Reconciling addon deploy %q", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		// ignore secret whose key is not in format: namespace/name
		return nil
	}

	for _, handler := range secretExchangeHandler.RegisteredHandlers {
		err = handler.syncGreenSecret(name, namespace, c)
		if err != nil {
			return err
		}
	}

	return nil
}
```

### SecretExchangeHandler

Token Exchange Agent 提供了一套interface，用于定义Exchange Handler的基本方法，并在GreenSecretTokenExchangeAgentController和BlueSecretTokenExchangeAgentController中调用：

```
type SecretExchangeHandlerInterface interface {
	getBlueSecretFilter(interface{}) bool
	getGreenSecretFilter(interface{}) bool
	syncBlueSecret(string, string, *blueSecretTokenExchangeAgentController) error
	syncGreenSecret(string, string, *greenSecretTokenExchangeAgentController) error
}
```

在RunAgent中注册`S3SecretHandler`和`RookSecretHandlerName`这两种Exchange Handler:
```
func (o *AgentOptions) RunAgent(ctx context.Context, controllerContext *controllercmd.ControllerContext) error {
...
	err = registerHandler(multiclusterv1alpha1.DRType(o.DRMode), controllerContext.KubeConfig, hubRestConfig)
	if err != nil {
		return err
	}
...
}
```

```

// intialize secretExchangeHandler with handlers
func registerHandler(mode multiclusterv1alpha1.DRType, spokeKubeConfig *rest.Config, hubKubeConfig *rest.Config) error {
	if mode != multiclusterv1alpha1.Sync && mode != multiclusterv1alpha1.Async {
		return fmt.Errorf("unknown mode %q detected, please check the mirrorpeer created", mode)
	}
	// rook specific client
	rookClient, err := rookclient.NewForConfig(spokeKubeConfig)
	if err != nil {
		return fmt.Errorf("failed to add rook client: %v", err)
	}

	// a generic client which is common between all handlers
	genericSpokeClient, err := getClient(spokeKubeConfig)
	if err != nil {
		return err
	}
	genericHubClient, err := getClient(hubKubeConfig)
	if err != nil {
		return err
	}

	secretExchangeHandler = &SecretExchangeHandler{
		RegisteredHandlers: make(map[string]SecretExchangeHandlerInterface),
	}

	secretExchangeHandler.RegisteredHandlers[utils.S3SecretHandlerName] = s3SecretHandler{
		spokeClient: genericSpokeClient,
		hubClient:   genericHubClient,
	}
	secretExchangeHandler.RegisteredHandlers[utils.RookSecretHandlerName] = rookSecretHandler{
		rookClient:  rookClient,
		spokeClient: genericSpokeClient,
		hubClient:   genericHubClient,
	}

	return nil
}
```

#### S3 Secret Handler
s3SecretHandler中包含了Hub Cluster和Spoke Cluster两种Client：
```
type s3SecretHandler struct {
	spokeClient client.Client
	hubClient   client.Client
}
```

- getBlueSecretFilter：判断Secret 或者configmap的ownerReference是不是`ObjectBucketClaim`, 并且Name需要包含`odrbucket`

```
func getFilterCondition(ownerReferences []metav1.OwnerReference, name string, blueSecretMatchString string) bool {
	for _, ownerReference := range ownerReferences {
		if ownerReference.Kind != ObjectBucketClaimKind {
			continue
		}
		return strings.Contains(name, blueSecretMatchString)
	}
	return false
}

func (s3SecretHandler) getBlueSecretFilter(obj interface{}) bool {
	blueSecretMatchString := os.Getenv("S3_EXCHANGE_SOURCE_SECRET_STRING_MATCH")
	if blueSecretMatchString == "" {
		blueSecretMatchString = utils.BucketGenerateName
	}

	if s, ok := obj.(*corev1.Secret); ok {
		return getFilterCondition(s.OwnerReferences, s.ObjectMeta.Name, blueSecretMatchString)
	} else if c, ok := obj.(*corev1.ConfigMap); ok {
		return getFilterCondition(c.OwnerReferences, c.ObjectMeta.Name, blueSecretMatchString)
	}

	return false
}
```
- syncBlueSecret: cofigmap, secret 和 bucket claim 在 nooba中具有相同的名字。
  -  获取全部满足Blue Secret Filter 的configmap和secret

```
	secret, err := getSecret(c.spokeSecretLister, name, namespace)
	if err != nil {
		return fmt.Errorf("failed to get the secret %q in namespace %q in managed cluster. Error %v", name, namespace, err)
	}
	isMatch := s.getBlueSecretFilter(secret)
	if !isMatch {
		// ignore handler which secret filter is not matched
		return nil
	}

	// fetch obc config map
	configMap, err := getConfigMap(c.spokeConfigMapLister, name, namespace)
	if err != nil {
		return fmt.Errorf("failed to get the config map %q in namespace %q in managed cluster. Error %v", name, namespace, err)
	}
	isMatch = s.getBlueSecretFilter(configMap)
	if !isMatch {
		// ignore handler which configmap filter is not matched
		return nil
	}
```
  -  遍历全部的MirrorPeers，获取当前cluster的storageClusterRef
```
	mirrorPeers, err := utils.FetchAllMirrorPeers(context.TODO(), s.hubClient)
	if err != nil {
		return err
	}
```
  -  在storageClusterRef.Namespace 中获取名为`s3`的route

```
	var storageClusterRef *v1alpha1.StorageClusterRef
	for _, mirrorPeer := range mirrorPeers {
		storageClusterRef, err = utils.GetCurrentStorageClusterRef(&mirrorPeer, c.clusterName)
		if err == nil {
			break
		}
	}

	if storageClusterRef == nil {
		klog.Error("failed to find storage cluster ref using spoke cluster name %s from mirrorpeers ", c.clusterName)
		return err
	}

	// fetch s3 endpoint
	route := &routev1.Route{}
	err = s.spokeClient.Get(context.TODO(), types.NamespacedName{Name: S3RouteName, Namespace: storageClusterRef.Namespace}, route)
	if err != nil {
		return fmt.Errorf("failed to get the s3 endpoint in namespace %q in managed cluster. Error %v", namespace, err)
	}
```
  - 使用获取到的route和configmap中的`BUCKET_REGION`作为s3Region，创建Secret：
```
	s3Region := configMap.Data[S3BucketRegion]
	if s3Region == "" {
		s3Region = DefaultS3Region
	}
	// s3 secret
	s3Secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: storageClusterRef.Namespace,
		},
		Type: utils.SecretLabelTypeKey,
		Data: map[string][]byte{
			utils.S3ProfileName:      []byte(fmt.Sprintf("%s-%s-%s", utils.S3ProfilePrefix, c.clusterName, storageClusterRef.Name)),
			utils.S3BucketName:       []byte(configMap.Data[S3BucketName]),
			utils.S3Region:           []byte(s3Region),
			utils.S3Endpoint:         []byte(fmt.Sprintf("%s://%s", DefaultS3EndpointProtocol, route.Spec.Host)),
			utils.AwsSecretAccessKey: []byte(secret.Data[utils.AwsSecretAccessKey]),
			utils.AwsAccessKeyId:     []byte(secret.Data[utils.AwsAccessKeyId]),
		},
	}
```
  - 生成`multicluster.odf.openshift.io/secret-type: INTERNAL`BlueSecret， 并在Hub cluster上创建：
```
	customData := map[string][]byte{
		utils.SecretOriginKey: []byte(utils.OriginMap["S3Origin"]),
	}

	newSecret, err := generateBlueSecret(&s3Secret, utils.InternalLabel, utils.CreateUniqueSecretName(c.clusterName, storageClusterRef.Namespace, storageClusterRef.Name, utils.S3ProfilePrefix), storageClusterRef.Name, c.clusterName, customData)
	if err != nil {
		return fmt.Errorf("failed to create secret from the managed cluster secret %q from namespace %v for the hub cluster in namespace %q err: %v", secret.Name, secret.Namespace, c.clusterName, err)
	}
	err = createSecret(c.hubKubeClient, c.recorder, newSecret)
	if err != nil {
		return fmt.Errorf("failed to sync managed cluster secret %q from namespace %v to the hub cluster in namespace %q err: %v", name, namespace, c.clusterName, err)
	}
```
- getGreenSecretFilter: return false
- syncGreenSecret: return nil
#### Rook Secret Handler

rookSecretHandler中包含了Hub Cluster，Spoke Cluster和Rook Client三种Client：
```
type rookSecretHandler struct {
	spokeClient client.Client
	hubClient   client.Client
	rookClient  rookclient.Interface
}
```

- getBlueSecretFilter：如果ceph是external，则secret的名字是`rook-ceph-mon`，并且具有Kind为`StorageCluster`的OwnerReference，如果ceph不是external，则secret的名字包含`cluster-peer-token`，并且具有Kind为`CephCluster`的OwnerReference
- getGreenSecretFilter：具有label `multicluster.odf.openshift.io/secret-type=GREEN`的secret
- syncBlueSecret：
  - 从Spoke Cluster上获取满足BlueSecretFilter条件的secret
  - 从Secret的OwnerReference中获取StorageCluster名字
  - 根据StorageCluster获取Cluster Type，如果是external，则在hub上创建一个`INTERNAL`的secret。如果是CONVERGED，则在hub上创建一个`BLUE`的secret。
- syncGreenSecret： 
  - 从Hub cluster上获取满足GreenSecretFilter条件的secret
  - 获取key为`secret-data`的数据，在Spoke cluster上创建具有label `multicluster.odf.openshift.io/created-by: "tokenexchange"`的`Secret`
  - 更新StorageCluster中的Spec.Mirroring.PeerSecretNames

### MirrorPeerReconciler

`Token Exchange Agent`还会使用Hub Cluster的Config启动一个controller manager，所以该manager是watch hub cluster上的资源：
```
func runManager(ctx context.Context, hubConfig *rest.Config, spokeConfig *rest.Config, spokeClusterName string) {
  mgr, err := ctrl.NewManager(hubConfig, ctrl.Options{
		Scheme: mgrScheme,
		Port:   9443,
	})
	if err != nil {
		klog.Error(err, "unable to start manager")
		os.Exit(1)
	}
...
	if err = (&MirrorPeerReconciler{
		HubClient:        mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		SpokeClient:      spokeClient,
		SpokeClusterName: spokeClusterName,
	}).SetupWithManager(mgr); err != nil {
		klog.Error(err, "unable to create controller", "controller", "MirrorPeer")
		os.Exit(1)
	}
  ...
}
```

并启动`MirrorPeerReconciler`，注意，是Hub cluster上的`MirrorPeer`:
```
// SetupWithManager sets up the controller with the Manager.
func (r *MirrorPeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mirrorPeerSpokeClusterPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return r.hasSpokeCluster(e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return r.hasSpokeCluster(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return r.hasSpokeCluster(e.ObjectNew)
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}

	mpPredicate := utils.ComposePredicates(predicate.GenerationChangedPredicate{}, mirrorPeerSpokeClusterPredicate)
	return ctrl.NewControllerManagedBy(mgr).
		For(&multiclusterv1alpha1.MirrorPeer{}, builder.WithPredicates(mpPredicate)).
		Complete(r)
}
```

Reconciler会自动reconcile所有包括该Cluster的MirrorPeer资源：
```
func (r *MirrorPeerReconciler) hasSpokeCluster(obj client.Object) bool {
	mp, ok := obj.(*multiclusterv1alpha1.MirrorPeer)
	if !ok {
		return false
	}
	for _, v := range mp.Spec.Items {
		if v.ClusterName == r.SpokeClusterName {
			return true
		}
	}
	return false
}
```

该Reconciler的主要流程为：
1. 根据自身的Cluster Name从MirrorPeer中获取StorageClusterRef：
```
	scr, err := utils.GetCurrentStorageClusterRef(&mirrorPeer, r.SpokeClusterName)
	if err != nil {
		klog.Error(err, "Failed to get current storage cluster ref")
		return ctrl.Result{}, err
	}
```
```
// StorageClusterRef holds a reference to a StorageCluster
type StorageClusterRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}
```
2. 添加`<SpokeClusterName>.spoke.multicluster.odf.openshift.io`的finalizer
3. 如果检测到删除`MirrorPeer`，这需要清理该Cluster，清理完成之后去除`<SpokeClusterName>.spoke.multicluster.odf.openshift.io` finalizer。清理的主要流程包括：

```
func (r *MirrorPeerReconciler) deleteMirrorPeer(ctx context.Context, mirrorPeer multiclusterv1alpha1.MirrorPeer, scr *multiclusterv1alpha1.StorageClusterRef) (ctrl.Result, error) {
	klog.Infof("Mirrorpeer is being deleted %q", mirrorPeer.Name)
	peerRef, err := utils.GetPeerRefForSpokeCluster(&mirrorPeer, r.SpokeClusterName)
	if err != nil {
		klog.Errorf("Failed to get current PeerRef %q %v", mirrorPeer.Name, err)
		return ctrl.Result{}, err
	}

	peerRefUsed, err := utils.DoesAnotherMirrorPeerPointToPeerRef(ctx, r.HubClient, peerRef)
	if err != nil {
		klog.Errorf("failed to check if another peer uses peer ref %v", err)
	}
	if !peerRefUsed {
		if err := r.disableMirroring(ctx, scr.Name, scr.Namespace, &mirrorPeer); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to disable mirroring for the storagecluster %q in namespace %q. Error %v", scr.Name, scr.Namespace, err)
		}
		if err := r.disableCSIAddons(ctx, scr.Namespace); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to disable CSI Addons for rook: %v", err)
		}
	}

	if err := r.deleteGreenSecret(ctx, r.SpokeClusterName, scr.Namespace, &mirrorPeer); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete green secrets: %v", err)
	}

	if err := r.deleteVolumeReplicationClass(ctx, &mirrorPeer, peerRef); err != nil {
		return ctrl.Result{}, fmt.Errorf("few failures occured while deleting VolumeReplicationClasses: %v", err)
	}
	if err := r.deleteS3(ctx, mirrorPeer, scr.Namespace); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete s3 buckets")
	}
	return ctrl.Result{}, nil
}
```
  -  获取包含该cluster的PeerRef:
```
type PeerRef struct {
	// ClusterName is the name of ManagedCluster.
	// ManagedCluster matching this name is considered
	// a peer cluster.
	ClusterName string `json:"clusterName"`
	// StorageClusterRef holds a reference to StorageCluster object
	StorageClusterRef StorageClusterRef `json:"storageClusterRef"`
}
```
  - 检查是否有其他MirrorPeer包含当前Cluster，如果没有，则清理Mirroring配置和CSIAddon
```
	peerRefUsed, err := utils.DoesAnotherMirrorPeerPointToPeerRef(ctx, r.HubClient, peerRef)
	if err != nil {
		klog.Errorf("failed to check if another peer uses peer ref %v", err)
	}
	if !peerRefUsed {
		if err := r.disableMirroring(ctx, scr.Name, scr.Namespace, &mirrorPeer); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to disable mirroring for the storagecluster %q in namespace %q. Error %v", scr.Name, scr.Namespace, err)
		}
		if err := r.disableCSIAddons(ctx, scr.Namespace); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to disable CSI Addons for rook: %v", err)
		}
	}
```
Disable Mirroring只需要将StorageClusterRef.Namespace中的ODF StorageCluster资源`spec.Mirroring`设置为false即可。
```
// toggleMirroring changes the state of mirroring in the storage cluster
func (r *MirrorPeerReconciler) toggleMirroring(ctx context.Context, storageClusterName string, namespace string, enabled bool, mp *multiclusterv1alpha1.MirrorPeer) error {
	var sc ocsv1.StorageCluster
	err := r.SpokeClient.Get(ctx, types.NamespacedName{
		Name:      storageClusterName,
		Namespace: namespace,
	}, &sc)

	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("could not find storagecluster %q in namespace %v: %v", storageClusterName, namespace, err)
		}
		return err
	}
	if enabled {
		oppPeers := getOppositePeerRefs(mp, r.SpokeClusterName)
		if hasRequiredSecret(sc.Spec.Mirroring.PeerSecretNames, oppPeers) {
			sc.Spec.Mirroring.Enabled = true
			klog.Info("Enabled mirroring on StorageCluster ", storageClusterName)
		} else {
			klog.Error(err, "StorageCluster does not have required PeerSecrets")
			return err
		}
	} else {
		sc.Spec.Mirroring.Enabled = false
	}
	return r.SpokeClient.Update(ctx, &sc)
}
```
Disable CSI Addon只需要将StorageClusterRef.Namespace中名为`rook-ceph-operator-config`的Configmap中`CSI_ENABLE_OMAP_GENERATOR`设置为false即可。
```
func (r *MirrorPeerReconciler) toggleCSIAddons(ctx context.Context, namespace string, enabled bool) error {
	var rcm corev1.ConfigMap
	err := r.SpokeClient.Get(ctx, types.NamespacedName{
		Name:      RookConfigMapName,
		Namespace: namespace,
	}, &rcm)

	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("could not find rook-ceph-config-map: %v", err)
		}
		return err
	}

	if rcm.Data == nil {
		rcm.Data = make(map[string]string, 0)
	}
	rcm.Data[RookCSIEnableKey] = strconv.FormatBool(enabled)

	return r.SpokeClient.Update(ctx, &rcm)
}
```
  - 遍历MirrorPeer中除自身外的所有PeerRef，删除其GreenSecret。 GreenSecret位于StorageClusterRef.Namespace，名字是由PeerRef中的ClusterName，StorageClusterRef.Namespace, StorageClusterRef.Name一起生成的随机字符串。
```
// deleteGreenSecret deletes the exchanged secret present in the namespace of the storage cluster
func (r *MirrorPeerReconciler) deleteGreenSecret(ctx context.Context, spokeClusterName string, scrNamespace string, mirrorPeer *multiclusterv1alpha1.MirrorPeer) error {
	for _, peerRef := range mirrorPeer.Spec.Items {
		if peerRef.ClusterName != spokeClusterName {
			secretName := utils.GetSecretNameByPeerRef(peerRef)
			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: scrNamespace,
				},
			}
			if err := r.SpokeClient.Delete(ctx, &secret); err != nil {
				if errors.IsNotFound(err) {
					klog.Info("failed to find green secret ", secretName)
					return nil
				} else {
					klog.Error(err, "failed to delete green secret ", secretName)
					return err
				}
			}
			klog.Info("Succesfully deleted ", secretName, " in namespace ", scrNamespace)
		}
	}
	return nil
}
```
  - 删除VolumeReplicationClass
如果MirrorPeer不包含SchedulingIntervals，则什么都不做。如果包含包含`SchedulingInterval`并且没有其他含有该Cluster的MirrorPeer使用相同的`SchedulingIntervals`，则使用`SchedulingIntervals`生成VolumeReplicationClass名字`rbd-volumereplicationclass-<Hash(SchedulingIntervals)>`，并将其删除。
```
func (r *MirrorPeerReconciler) deleteVolumeReplicationClass(ctx context.Context, mp *multiclusterv1alpha1.MirrorPeer, peerRef *multiclusterv1alpha1.PeerRef) error {
	for _, interval := range mp.Spec.SchedulingIntervals {
		intervalUsed, err := utils.DoesAnotherMirrorPeerContainTheSchedulingInterval(ctx, r.HubClient, mp, interval, peerRef)
		if err != nil {
			return err
		}
		if intervalUsed {
			return nil
		}

		vrcName := fmt.Sprintf(RBDVolumeReplicationClassNameTemplate, utils.FnvHash(interval))

		vrc := &replicationv1alpha1.VolumeReplicationClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: vrcName,
			},
		}
		err = r.SpokeClient.Delete(ctx, vrc)
		if errors.IsNotFound(err) {
			klog.Error("Cannot find volume replication class for interval", interval, " ", err)
			return nil
		}

		klog.Info("Succesfully deleted interval ", interval)

	}
	return nil
}
```
  - 删除S3 Bucket ObjectBucketClaim，ObjectBucketClaim位于该cluster的StorageClusterRef.Namespace，名字由全体MirrorPeerRef生成：`odrbucket-<peerAccumulator>`
```
func (r *MirrorPeerReconciler) deleteS3(ctx context.Context, mirrorPeer multiclusterv1alpha1.MirrorPeer, scNamespace string) error {
	noobaaOBC, err := r.getS3bucket(ctx, mirrorPeer, scNamespace)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Info("Could not find ODR ObjectBucketClaim, skipping deletion")
			return nil
		} else {
			klog.Error(err, "Failed to get ODR ObjectBucketClaim")
			return err
		}
	}
	err = r.SpokeClient.Delete(ctx, noobaaOBC)
	if err != nil {
		klog.Error(err, "Failed to delete ODR ObjectBucketClaim")
		return err
	}
	return err
}
...
func (r *MirrorPeerReconciler) getS3bucket(ctx context.Context, mirrorPeer multiclusterv1alpha1.MirrorPeer, scNamespace string) (*obv1alpha1.ObjectBucketClaim, error) {
	var peerAccumulator string
	for _, peer := range mirrorPeer.Spec.Items {
		peerAccumulator += peer.ClusterName
	}
	checksum := sha1.Sum([]byte(peerAccumulator))

	bucketGenerateName := utils.BucketGenerateName
	// truncate to bucketGenerateName + "-" + first 12 (out of 20) byte representations of sha1 checksum
	bucket := fmt.Sprintf("%s-%s", bucketGenerateName, hex.EncodeToString(checksum[:]))[0 : len(bucketGenerateName)+1+12]
	namespace := utils.GetEnv("ODR_NAMESPACE", scNamespace)

	noobaaOBC := &obv1alpha1.ObjectBucketClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bucket,
			Namespace: namespace,
		},
		Spec: obv1alpha1.ObjectBucketClaimSpec{
			BucketName:       bucket,
			StorageClassName: namespace + ".noobaa.io",
		},
	}
	err := r.SpokeClient.Get(ctx, types.NamespacedName{Name: bucket, Namespace: namespace}, noobaaOBC)
	return noobaaOBC, err
}
```

4. 创建S3 Bucket，ObjectBucketClaim位于该cluster的StorageClusterRef.Namespace，名字由全体MirrorPeerRef生成：`odrbucket-<peerAccumulator>`
```
	klog.Infof("creating s3 buckets")
	err = r.createS3(ctx, req, mirrorPeer, scr.Namespace)
	if err != nil {
		klog.Error(err, "Failed to create ODR S3 resources")
		return ctrl.Result{}, err
	}
```
5. 如果mirrorPeer.Spec.Type是Async：
```
	if mirrorPeer.Spec.Type == multiclusterv1alpha1.Async {
		klog.Infof("enabling async mode dependencies")
		err = r.enableCSIAddons(ctx, scr.Namespace)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to start CSI Addons for rook: %v", err)
		}

		err = r.enableMirroring(ctx, scr.Name, scr.Namespace, &mirrorPeer)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to enable mirroring the storagecluster %q in namespace %q in managed cluster. Error %v", scr.Name, scr.Namespace, err)
		}

		clusterFSIDs := make(map[string]string)
		klog.Infof("Fetching clusterFSIDs")
		err = r.fetchClusterFSIDs(ctx, &mirrorPeer, clusterFSIDs)
		if err != nil {
			if errors.IsNotFound(err) {
				return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
			}
			return ctrl.Result{}, fmt.Errorf("an unknown error occured while fetching the cluster fsids, retrying again: %v", err)
		}

		errs := r.createVolumeReplicationClass(ctx, &mirrorPeer, clusterFSIDs)
		if len(errs) > 0 {
			return ctrl.Result{}, fmt.Errorf("few failures occured while creating VolumeReplicationClasses: %v", errs)
		}

		klog.Infof("labeling rbd storageclasses")
		errs = r.labelRBDStorageClasses(ctx, mirrorPeer, scr.Namespace, clusterFSIDs)
		if len(errs) > 0 {
			return ctrl.Result{}, fmt.Errorf("few failures occured while labeling RBD StorageClasses: %v", errs)
		}
	}
```

  - Enable CSI addon: 将StorageClusterRef.Namespace中名为`rook-ceph-operator-config`的Configmap中`CSI_ENABLE_OMAP_GENERATOR`设置为true。
```
func (r *MirrorPeerReconciler) toggleCSIAddons(ctx context.Context, namespace string, enabled bool) error {
	var rcm corev1.ConfigMap
	err := r.SpokeClient.Get(ctx, types.NamespacedName{
		Name:      RookConfigMapName,
		Namespace: namespace,
	}, &rcm)

	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("could not find rook-ceph-config-map: %v", err)
		}
		return err
	}

	if rcm.Data == nil {
		rcm.Data = make(map[string]string, 0)
	}
	rcm.Data[RookCSIEnableKey] = strconv.FormatBool(enabled)

	return r.SpokeClient.Update(ctx, &rcm)
}
```
  - enable Mirroring: 从mirror peer中获取出自身外所有的peerRefs，并遍历每一个peerRef，生成secret name，判断`storageclass.Spec.Mirroring.PeerSecretNames`是否包含所有的secret。如果不是，则返回错误，不enable mirroring
```
// toggleMirroring changes the state of mirroring in the storage cluster
func (r *MirrorPeerReconciler) toggleMirroring(ctx context.Context, storageClusterName string, namespace string, enabled bool, mp *multiclusterv1alpha1.MirrorPeer) error {
	var sc ocsv1.StorageCluster
	err := r.SpokeClient.Get(ctx, types.NamespacedName{
		Name:      storageClusterName,
		Namespace: namespace,
	}, &sc)

	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("could not find storagecluster %q in namespace %v: %v", storageClusterName, namespace, err)
		}
		return err
	}
	if enabled {
		oppPeers := getOppositePeerRefs(mp, r.SpokeClusterName)
		if hasRequiredSecret(sc.Spec.Mirroring.PeerSecretNames, oppPeers) {
			sc.Spec.Mirroring.Enabled = true
			klog.Info("Enabled mirroring on StorageCluster ", storageClusterName)
		} else {
			klog.Error(err, "StorageCluster does not have required PeerSecrets")
			return err
		}
	} else {
		sc.Spec.Mirroring.Enabled = false
	}
	return r.SpokeClient.Update(ctx, &sc)
}
```
  - 遍历PeerRef是，获取每个PeerRef的`clusterFSIDs`:
根据`storageCluster.Spec.ExternalStorage.Enable`判断cluster type，如果为true，则是`External`，如果是false，则是`Converged`。
如果是`External`，则Secret的名字为`rook-ceph-mon`
如果是`Converged`，并且是当前cluster，则Secret的名字为`cluster-peer-token-<StorageClusterRef.Name>-cephcluster`，如果不是当前cluster，则Secret的名字从pr.ClusterName, pr.StorageClusterRef.Namespace, pr.StorageClusterRef.Name一起产生。从token中获取fsid存入以clustername为key的map中。
  - 创建volume replication class:
```
Name: rbd-volumereplicationclass-<hash(interval)>
Labels: ramendr.openshift.io/replicationid = hash(all_fsids_in_mirrorpeer)
Parameters:
  mirroringMode = "snapshot"
	schedulingInterval = <interval>
	replication.storage.openshift.io/replication-secret-name = rook-csi-rbd-provisioner
	replication.storage.openshift.io/replication-secret-namespace = scr.Namespace
Provisioner: <scr.Namespace>.rbd.csi.ceph.com
```
```
func (r *MirrorPeerReconciler) createVolumeReplicationClass(ctx context.Context, mp *multiclusterv1alpha1.MirrorPeer, clusterFSIDs map[string]string) []error {
	scr, err := utils.GetCurrentStorageClusterRef(mp, r.SpokeClusterName)
	var errs []error
	if err != nil {
		klog.Error(err, "Failed to get current storage cluster ref")
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errs
	}
	var fsids []string
	for _, v := range clusterFSIDs {
		fsids = append(fsids, v)
	}

	// To ensure reliability of hash generation
	sort.Strings(fsids)

	replicationId := utils.CreateUniqueReplicationId(fsids)

	for _, interval := range mp.Spec.SchedulingIntervals {
		params := make(map[string]string)
		params[MirroringModeKey] = DefaultMirroringMode
		params[SchedulingIntervalKey] = interval
		params[ReplicationSecretNameKey] = RBDReplicationSecretName
		params[ReplicationSecretNamespaceKey] = scr.Namespace
		vrcName := fmt.Sprintf(RBDVolumeReplicationClassNameTemplate, utils.FnvHash(interval))
		klog.Infof("Creating volume replication class %q with label replicationId %q", vrcName, replicationId)
		found := &replicationv1alpha1.VolumeReplicationClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: vrcName,
			},
		}
		err = r.SpokeClient.Get(ctx, types.NamespacedName{
			Name: found.Name,
		}, found)

		switch {
		case err == nil:
			klog.Infof("VolumeReplicationClass already exists: %s", vrcName)
			continue
		case !errors.IsNotFound(err):
			klog.Error(err, "Failed to get VolumeReplicationClass: %s", vrcName)
			errs = append(errs, err)
			continue
		}
		labels := make(map[string]string)
		labels[fmt.Sprintf(RamenLabelTemplate, ReplicationIDKey)] = replicationId
		vrc := replicationv1alpha1.VolumeReplicationClass{
			ObjectMeta: metav1.ObjectMeta{
				Name:   vrcName,
				Labels: labels,
			},
			Spec: replicationv1alpha1.VolumeReplicationClassSpec{
				Parameters:  params,
				Provisioner: fmt.Sprintf(RBDProvisionerTemplate, scr.Namespace),
			},
		}
		err = r.SpokeClient.Create(ctx, &vrc)
		if err != nil {
			klog.Error(err, "Failed to create VolumeReplicationClass: %s", vrcName)
			errs = append(errs, err)
			continue
		}
		klog.Infof("VolumeReplicationClass created: %s", vrcName)
	}
	return errs
}
```
  - 遍历所有`Provisioner = <storageClusterNamespace>.rbd.csi.ceph.com`或者`Provisioner = <storageClusterNamespace>.cephfs.csi.ceph.com`的storage class，为其添加label`ramendr.openshift.io/storageid=当前cluster的fsid`

## Summary

![Summary](../../../assets/images/posts/odr.drawio.png)