---
title: OCM Registration Operator
tags: OCM Registration
---

Registration Operator有2个Operator:
- Cluster Manager: 为Hub Cluster安装OCM的基础组件
- Klusterlet: 在Managed Cluster上安装agent组件

## Cluster Manager
ClusterManager Operator在Hub Cluster上配置Controllers，用于管理[Registration](https://github.com/open-cluster-management-io/registration), [Placement](https://github.com/open-cluster-management-io/placement), [Work](https://github.com/open-cluster-management-io/work)的分发。

Controllers都部署在Hub Cluster上的 `open-cluster-management-hub` namespace中。

## Klusterlet

Klusterlet Operator代表受管集群上的agent controller:
- [Registration](https://github.com/open-cluster-management-io/registration)
- [Work](https://github.com/open-cluster-management-io/work)

Klusterlet 需要在同一命名空间中名为 `bootstrap-hub-kubeconfig` 的密钥，以允许向Hub Cluster发送注册请求。

默认情况下，Controllers都部署在 `open-cluster-management-agent` 命名空间中。命名空间可以在 Klusterlet CR 中指定。

## 源码分析

regisration-operator main.go 中注册了两个Command：
- hub
- klusterlet

```

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	logs.AddFlags(pflag.CommandLine)
	logs.InitLogs()
	defer logs.FlushLogs()

	command := newNucleusCommand()
	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func newNucleusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registration-operator",
		Short: "Nucleus Operator",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
			os.Exit(1)
		},
	}

	if v := version.Get().String(); len(v) == 0 {
		cmd.Version = "<unknown>"
	} else {
		cmd.Version = v
	}

	cmd.AddCommand(operator.NewHubOperatorCmd())
	cmd.AddCommand(operator.NewKlusterletOperatorCmd())

	return cmd
}
```

### registration-operator hub
```
func NewHubOperatorCmd() *cobra.Command {

	options := clustermanager.Options{}
	cmd := controllercmd.
		NewControllerCommandConfig("clustermanager", version.Get(), options.RunClusterManagerOperator).
		NewCommand()
	cmd.Use = "hub"
	cmd.Short = "Start the cluster manager operator"

	return cmd
}
```

Command `hub` 包含函数`options.RunClusterManagerOperator`:

```
// RunClusterManagerOperator starts a new cluster manager operator
func (o *Options) RunClusterManagerOperator(ctx context.Context, controllerContext *controllercmd.ControllerContext) error {
	// Build kubclient client and informer for managed cluster
	kubeClient, err := kubernetes.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	// kubeInformer is for 3 usages: configmapInformer, secretInformer, deploynmentInformer
	// After we introduced hosted mode, the hub components could be installed in a customized namespace.(Before that, it only inform from "open-cluster-management-hub" namespace)
	// It requires us to add filter for each Informer respectively.
	// TODO: Wathc all namespace may cause performance issue.
	kubeInformer := informers.NewSharedInformerFactoryWithOptions(kubeClient, 5*time.Minute)

	// Build operator client and informer
	operatorClient, err := operatorclient.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}
	operatorInformer := operatorinformer.NewSharedInformerFactory(operatorClient, 5*time.Minute)

	clusterManagerController := clustermanagercontroller.NewClusterManagerController(
		kubeClient,
		controllerContext.KubeConfig,
		operatorClient.OperatorV1().ClusterManagers(),
		operatorInformer.Operator().V1().ClusterManagers(),
		kubeInformer.Apps().V1().Deployments(),
		kubeInformer.Core().V1().ConfigMaps(),
		controllerContext.EventRecorder)

	statusController := clustermanagerstatuscontroller.NewClusterManagerStatusController(
		operatorClient.OperatorV1().ClusterManagers(),
		operatorInformer.Operator().V1().ClusterManagers(),
		kubeInformer.Apps().V1().Deployments(),
		controllerContext.EventRecorder)

	certRotationController := certrotationcontroller.NewCertRotationController(
		kubeClient,
		kubeInformer.Core().V1().Secrets(),
		kubeInformer.Core().V1().ConfigMaps(),
		operatorInformer.Operator().V1().ClusterManagers(),
		controllerContext.EventRecorder)

	crdMigrationController := migrationcontroller.NewCRDMigrationController(
		controllerContext.KubeConfig,
		kubeClient,
		operatorInformer.Operator().V1().ClusterManagers(),
		controllerContext.EventRecorder)

	go operatorInformer.Start(ctx.Done())
	go kubeInformer.Start(ctx.Done())
	go clusterManagerController.Run(ctx, 1)
	go statusController.Run(ctx, 1)
	go certRotationController.Run(ctx, 1)
	go crdMigrationController.Run(ctx, 1)

	<-ctx.Done()
	return nil
}
```

此函数中主要启动：

- clusterManagerController.Run(ctx, 1)
- statusController.Run(ctx, 1)
- certRotationController.Run(ctx, 1)
- crdMigrationController.Run(ctx, 1)

#### clusterManagerController

clusterManagerController主要监听`ClusterManager` 资源，并安装或者清理：
- HubResources
    - CRDResources
      - "cluster-manager/hub/0000_00_addon.open-cluster-management.io_clustermanagementaddons.crd.yaml"
	  - "cluster-manager/hub/0000_00_clusters.open-cluster-management.io_managedclusters.crd.yaml"
	  - "cluster-manager/hub/0000_00_clusters.open-cluster-management.io_managedclustersets.crd.yaml"
	  - "cluster-manager/hub/0000_00_work.open-cluster-management.io_manifestworks.crd.yaml"
	  - "cluster-manager/hub/0000_01_addon.open-cluster-management.io_managedclusteraddons.crd.yaml"
	  - "cluster-manager/hub/0000_01_clusters.open-cluster-management.io_managedclustersetbindings.crd.yaml"
	  - "cluster-manager/hub/0000_02_clusters.open-cluster-management.io_placements.crd.yaml"
	  - "cluster-manager/hub/0000_03_clusters.open-cluster-management.io_placementdecisions.crd.yaml"
	  - "cluster-manager/hub/0000_05_clusters.open-cluster-management.io_addonplacementscores.crd.yaml"
    - WebhookResourceFiles
      - RegistrationWebhook
        - "cluster-manager/hub/cluster-manager-registration-webhook-validatingconfiguration.yaml"
		- "cluster-manager/hub/cluster-manager-registration-webhook-mutatingconfiguration.yaml"
		- "cluster-manager/hub/cluster-manager-registration-webhook-clustersetbinding-validatingconfiguration.yaml"
      -  WorkWebhook
        - "cluster-manager/hub/cluster-manager-work-webhook-validatingconfiguration.yaml"
    - RbacResourceFiles
      - "cluster-manager/hub/cluster-manager-registration-clusterrole.yaml"
	  - "cluster-manager/hub/cluster-manager-registration-clusterrolebinding.yaml"
	  - "cluster-manager/hub/cluster-manager-registration-serviceaccount.yaml"
	  - "cluster-manager/hub/cluster-manager-registration-webhook-clusterrole.yaml"
	  - "cluster-manager/hub/cluster-manager-registration-webhook-clusterrolebinding.yaml"
	  - "cluster-manager/hub/cluster-manager-registration-webhook-serviceaccount.yaml"
	  - "cluster-manager/hub/cluster-manager-work-webhook-clusterrole.yaml"
	  - "cluster-manager/hub/cluster-manager-work-webhook-clusterrolebinding.yaml"
	  - "cluster-manager/hub/cluster-manager-work-webhook-serviceaccount.yaml"
	  - "cluster-manager/hub/cluster-manager-placement-clusterrole.yaml"
	  - "cluster-manager/hub/cluster-manager-placement-clusterrolebinding.yaml"
	  - "cluster-manager/hub/cluster-manager-placement-serviceaccount.yaml"
- ManagementResources
  - NamespaceResource
    - cluster-manager/cluster-manager-namespace.yaml 
  - DeploymentFiles
    - "cluster-manager/management/cluster-manager-registration-deployment.yaml"
	- "cluster-manager/management/cluster-manager-registration-webhook-deployment.yaml"
	- "cluster-manager/management/cluster-manager-work-webhook-deployment.yaml"
	- "cluster-manager/management/cluster-manager-placement-deployment.yaml"

#### statusController

#### certRotationController
#### crdMigrationController


### registration-operator klusterlet
```
// NewKlusterletOperatorCmd generatee a command to start klusterlet operator
func NewKlusterletOperatorCmd() *cobra.Command {

	options := klusterlet.Options{}
	cmdConfig := controllercmd.
		NewControllerCommandConfig("klusterlet", version.Get(), options.RunKlusterletOperator)
	cmd := cmdConfig.NewCommand()
	cmd.Use = "klusterlet"
	cmd.Short = "Start the klusterlet operator"

	// add disable leader election flag
	cmd.Flags().BoolVar(&cmdConfig.DisableLeaderElection, "disable-leader-election", false, "Disable leader election for the agent.")
	cmd.Flags().BoolVar(&options.SkipPlaceholderHubSecret, "skip-placeholder-hub-secret", false,
		"If set, will skip ensuring a placeholder hub secret which is originally intended for pulling "+
			"work image before approved")

	return cmd
}
```