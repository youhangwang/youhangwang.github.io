---
title: Noobaa Operator
tags: Noobaa Openshift MCG
---

NooBaa 是一种适用于混合和多云环境的对象数据服务。 NooBaa 在 kubernetes 上运行，向集群内部和外部的客户端提供 S3 对象存储服务（以及支持 Bucket 触发的 Lambda 函数），使用集群内部或外部的存储资源，通过灵活的放置策略来自动化数据使用。

<!--more-->

Noobaaa服务主要分为两部分：
- Noobaa Core
- Noobaa Operator

本文将会深入分析Noobaa Operator的实现以及其源码。

## NooBaa Operator Overview

Noobaa Operator 采用Go语言编写，通过命令行指定命令和参数可以实现多种功能：

```
#                       # 
#    /~~\___~___/~~\    # 
#   |               |   # 
#    \~~|\     /|~~/    # 
#        \|   |/        # 
#         |   |         # 
#         \~~~/         # 
#                       # 
#      N O O B A A      #

Install:
  install        Install the operator and create the noobaa system
  uninstall      Uninstall the operator and delete the system
  status         Status of the operator and the system

Manage:
  backingstore   Manage backing stores
  namespacestore Manage namespace stores
  bucketclass    Manage bucket classes
  account        Manage noobaa accounts
  obc            Manage object bucket claims
  diagnose       Collect diagnostics
  ui             Open the NooBaa UI
  db-dump        Collect db dump

Advanced:
  operator       Deployment using operator
  system         Manage noobaa systems
  api            Make api call
  bucket         Manage noobaa buckets
  pvstore        Manage noobaa pv store
  crd            Deployment of CRDs
  olm            OLM related commands

Other Commands:
  completion     Generates bash completion scripts
  options        Print the list of global flags
  version        Show version

Use "noobaa <command> --help" for more information about a given command.
```

各种命令的使用方式可以参考 help message。接下来我们将会通过解析源码来分析每个command的内部实现原理

在main函数中，使用cobra注册了多个指令：
```
func Cmd() *cobra.Command {

	util.InitLogger(logrus.DebugLevel)

	rand.Seed(time.Now().UTC().UnixNano())

	logo := ASCIILogo1
	if rand.Intn(2) == 0 { // 50% chance
		logo = ASCIILogo2
	}

	// Root command
	rootCmd := &cobra.Command{
		Use:   "noobaa",
		Short: logo,
	}

	rootCmd.PersistentFlags().AddFlagSet(options.FlagSet)
	rootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	optionsCmd := options.Cmd()

	completionCmd := &cobra.Command{
		Use:   "completion",
		Short: "Generates bash completion scripts",
		Long: fmt.Sprintf(`
Load noobaa completion to bash:
(add to your ~/.bashrc and ~/.bash_profile to auto load)

. <(%s completion)
`, rootCmd.Name()),
		Run: func(cmd *cobra.Command, args []string) {
			alias, _ := cmd.Flags().GetString("alias")
			if alias != "" {
				rootCmd.Use = alias
			}
			err := rootCmd.GenBashCompletion(os.Stdout)
			if err != nil {
				fmt.Printf("got error on GenBashCompletion. %v", err)
			}

		},
		Args: cobra.NoArgs,
	}
	completionCmd.Flags().String("alias", "", "Custom alias name to generate the completion for")

	groups := templates.CommandGroups{{
		Message: "Install:",
		Commands: []*cobra.Command{
			install.CmdInstall(),
			install.CmdUninstall(),
			install.CmdStatus(),
		},
	}, {
		Message: "Manage:",
		Commands: []*cobra.Command{
			backingstore.Cmd(),
			namespacestore.Cmd(),
			bucketclass.Cmd(),
			noobaaaccount.Cmd(),
			obc.Cmd(),
			cosi.Cmd(),
			diagnostics.CmdDiagnoseDeprecated(),
			diagnostics.CmdDbDumpDeprecated(),
			diagnostics.Cmd(),
		},
	}, {
		Message: "Advanced:",
		Commands: []*cobra.Command{
			operator.Cmd(),
			system.Cmd(),
			system.CmdAPICall(),
			bucket.Cmd(),
			pvstore.Cmd(),
			crd.Cmd(),
			olm.Cmd(),
		},
	}}

	groups.Add(rootCmd)

	rootCmd.AddCommand(
		version.Cmd(),
		optionsCmd,
		completionCmd,
	)

	templates.ActsAsRootCommand(rootCmd, []string{}, groups...)
	templates.UseOptionsTemplates(optionsCmd)

	return rootCmd
}
```

## Install

Install Group中主要包含三个命令：
- install
- uninstall
- status

### install

```
// CmdInstall returns a CLI command
func CmdInstall() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the operator and create the noobaa system",
		Run:   RunInstall,
		Args:  cobra.NoArgs,
	}
	cmd.Flags().Bool("use-obc-cleanup-policy", false, "Create NooBaa system with obc cleanup policy")
	cmd.AddCommand(
		CmdYaml(),
	)
	return cmd
}

func CmdYaml() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "yaml",
		Short: "Show install yaml, expected usage \"noobaa install 1> install.yaml\"",
		Run:   RunYaml,
		Args:  cobra.NoArgs,
	}
	return cmd
}
```

Install 命令主要调用RunYaml方法部署Noobaa所需要的Yaml文件

```
// RunYaml dumps a combined yaml of all installation yaml
// including CRD, operator and system
func RunYaml(cmd *cobra.Command, args []string) {
	log := util.Logger()
	log.Println("Dump CRD yamls...")
	crd.RunYaml(cmd, args)
	fmt.Println("---") // yaml resources separator
	log.Println("Dump operator yamls...")
	operator.RunYaml(cmd, args)
	fmt.Println("---") // yaml resources separator
	log.Println("Dump system yamls...")
	system.RunYaml(cmd, args)
	log.Println("✅ Done dumping installation yaml")
}
```


### uninstall
### status

## Manage
## Advanced
## Other Commands