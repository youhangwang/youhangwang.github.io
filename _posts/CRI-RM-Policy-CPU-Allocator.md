---
title: CRI Resource Manager CPU Allocator
tags: CRI K8s
---

CRI Resource Manager 拥有一个独立的 CPU Allocator 组件，该组件帮助策略为工作负载做出明智的CPU核心分配。目前，除了静态池策略外，所有策略都使用内置的CPU Allocator。

<!--more-->

## 基于拓扑的分配
CPU Allocator尝试根据硬件拓扑优化CPU的分配。更具体地说，它的目标是将一个请求的所有CPU“靠近”彼此打包，以最小化CPU之间的内存延迟。

## CPU优先级
CPU Allocator还通过检测CPU特性及其配置参数自动进行CPU优先级排序。目前，CRI Resource Manager支持基于Linux CPUFreq子系统中的intel_pstate scaling driver，以及Intel Speed Select Technology (SST)进行CPU优先级检测。

CPU被分为三个优先级类别，即高、正常和低。使用CPU Allocator的策略可能会选择为某些类型的工作负载偏好特定的优先级类别。例如，为高优先级工作负载偏好（并保留）高优先级CPU。

## Intel Speed Select Technology (SST)
CRI Resource Manager支持检测所有Intel Speed Select Technology (SST)特性，即Speed Select Technology Performance Profile (SST-PP)、Base Frequency (SST-BF)、Turbo Frequency (SST-TF)和 Core Power (SST-CP)。

CPU优先级基于检测到的当前激活的SST特性及其参数配置：

1. 如果SST-TF已启用，所有SST-TF优先级的CPU都被标记为高优先级。
2. 如果SST-CP已启用但SST-TF未启用，CPU Allocator会检查激活的服务类别（CLOSes）及其参数。与最高优先级CLOS关联的CPU将被标记为高优先级，最低优先级CLOS关联的CPU将被标记为低优先级，可能的“中等优先级”CLOS关联的CPU将被标记为正常优先级。
3. 如果SST-BF已启用且SST-TF和SST-CP处于非活动状态，所有BF高优先级核心（具有更高保证的基频）将被标记为高优先级。

## Linux CPUFreq
基于CPUFreq的优先级排序仅在Intel Speed Select Technology (SST)被禁用（或不支持）时生效。CRI-RM根据两个参数将CPU核心分为优先级类别：

- base frequency
- EPP（Energy-Performance Preference）

具有高基频（相对于系统中其他核心）的CPU核心将被标记为高优先级。相应的，低基频将被映射为低优先级。

具有高EPP优先级（相对于系统中其他核心）的CPU核心将被标记为高优先级核心。
