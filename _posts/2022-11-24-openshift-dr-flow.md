---
title: ODF Disaster Recovery Flow
tags: ODF DR DRFlow
---

Openshift ACM console上提供了DR的UI，用户可以操作该UI实现ODF的MetroDR和RegionalDR的设置和操作。其中DR的UI是Openshift ACM console的plugin，其源码位于[odf-console](https://github.com/red-hat-storage/odf-console) github repo中。

<!--more-->

目前redhat并没有文档描述DR的详细流程，例如UI上的action创建了什么CR，该CR背后的动作的是什么，是否会触发新CR的创建。本文，我们将会通过UI以及各个operator的源码分析，将整体的流程串联起来。

阅读文本需要具有ACM/OCM的相关知识。

## [Openshift DR Overview](./2022-10-25-odf-dr.md)
### [Openshift MetroDR Config](./2022-09-25-odf-metrodr.md)
### [Openshift RegionalDR Config](./2022-10-15-odf-regionaldr.md)

## [UI Console](./2022-11-05-openshift-dr-console.md)

## [ODF Multicluster Orchestrator](./2022-11-17-openshift-dr-mco.md)

## [RamenDR](./2022-06-27-ramen.md)