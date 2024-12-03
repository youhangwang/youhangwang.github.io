---
title: CRI Resource Manager Container Affinity and Anti-Affinity
tags: CRI K8s
---

## 引言

一些策略允许用户给出关于如何在节点内共同定位特定容器的提示。特别是，这些提示表达了容器在硬件拓扑意义上应该“靠近”彼此还是“远离”彼此。

由于这些提示总是由特定的策略实现解释，所以“靠近”和“远离”的确切定义也有些策略特定。然而，作为一般规则，运行在同一个NUMA节点上的CPU上的容器被认为是“靠近”彼此的，在同一个Socket的不同NUMA节点上的CPU上的容器被认为是“更远”的，而在不同Socket上的CPU上的容器被认为是“远离”彼此的。

<!--more-->

这些提示通过Pod上的容器亲和性注解来表达。有两种类型的亲和性：

- 亲和性（或正亲和性）：导致受影响的容器互相靠近
- 反亲和性（或负亲和性）：导致受影响的容器互相远离

策略尝试放置容器：

- 靠近对容器有亲和性的容器
- 远离对容器有反亲和性的容器

## 亲和性注解语法

亲和性被定义为`cri-resource-manager.intel.com/affinity`注解。反亲和性被定义为`cri-resource-manager.intel.com/anti-affinity`注解。它们在Pod YAML的metadata部分的annotations下被指定，每个键是注解所属的Pod内的容器名称。

```yaml
metadata:
  annotations:
    cri-resource-manager.intel.com/affinity: |
      container1:
        - scope:
            key: key-ref
            operator: op
            values:
            - value1
            ...
            - valueN
          match:
            key: key-ref
            operator: op
            values:
            - value1
            ...
            - valueN
          weight: w
```
反亲和性类似地定义，但使用`cri-resource-manager.intel.com/anti-affinity`作为注解键。

```yaml
metadata:
  annotations:
    cri-resource-manager.intel.com/anti-affinity: |
      container1:
        - scope:
            key: key-ref
            operator: op
            values:
            - value1
            ...
            - valueN
          match:
            key: key-ref
            operator: op
            values:
            - value1
            ...
            - valueN
          weight: w
```

## 亲和性语义
亲和性由三部分组成：

- 作用域表达式：定义了亲和性针对哪些容器进行评估
- 匹配表达式：定义了亲和性适用于哪些容器（在作用域内）
- 权重：定义了亲和性引起的拉力或推力的强度

亲和性有时也被称为正亲和性，而反亲和性被称为负亲和性。这是因为它们之间唯一的区别在于亲和性有正权重，而反亲和性有负权重。

亲和性的作用域定义了亲和性可以应用到的容器的边界集。亲和性表达式针对作用域内的容器进行评估，并选择亲和性实际产生影响的容器。权重指定了是拉力还是推力的效果。正权重导致拉力，而负权重导致推力。此外，权重还指定了推力或拉力的强度。这在策略需要做出一些妥协，因为无法实现最佳放置时非常有用。权重还作为指定各种妥协之间的优先级偏好的方式：权重越重，拉力或推力越强，如果可能的话，它被尊重的可能性就越大。

如果省略了亲和性的作用域，则意味着Pod作用域，换句话说，就是定义亲和性的容器所属的Pod中的所有容器的作用域。

权重也可以省略，在这种情况下，默认为反亲和性为-1，亲和性为+1。权重目前限制在[-1000,1000]范围内。

亲和性的作用域和表达式都选择容器，因此它们是相同的。它们都是表达式。一个表达式由三部分组成：

- 键：指定从容器中提取哪些元数据进行评估
- 操作（op）：指定表达式评估的逻辑操作
- 值：一组字符串，用于评估键的值

支持的键包括：

- 对于Pod：
  - name
  - namespace
  - qosclass
  - labels/<label-key>
  - id
  - uid

- 对于容器：
  - pod/<pod-key>
  - name
  - namespace
  - qosclass
  - labels/<label-key>
  - tags/<tag-key>
  - id

本质上，一个表达式定义了一个形式为（key op values）的逻辑操作。评估这个逻辑表达式将取键的值，结果为真或假。目前支持以下操作：

- Equals：相等性，如果键的值等于values中的单个项目，则为真
- NotEqual：不等性，如果键的值不等于values中的单个项目，则为真
- In：成员资格，如果键的值等于values中的任何一个，则为真
- NotIn：否定成员资格，如果键的值不等于values中的任何一个，则为真
- Exists：如果给定的键存在任何值，则为真
- NotExists：如果给定的键不存在，则为真
- AlwaysTrue：总是评估为真，可用于表示节点全局作用域（所有容器）
- Matches：如果键的值与values中的globbing模式匹配，则为真
- MatchesNot：如果键的值与values中的globbing模式不匹配，则为真
- MatchesAny：如果键的值与values中的任何globbing模式匹配，则为真
- MatchesNone：如果键的值与values中的任何globbing模式都不匹配，则为真

容器C_1和C_2之间的有效亲和性A(C_1, C_2)是所有成对作用域匹配亲和性W(C_1, C_2)的权重之和。换句话说，评估容器C_1的亲和性是通过首先使用作用域（表达式）来确定哪些容器在亲和性的作用域内。然后，对于作用域内的每个容器C_2，如果匹配表达式评估为真，则取亲和性的权重，并加到有效亲和性A(C_1, C_2)上。

请注意，目前（对于拓扑感知策略），这种评估是不对称的：A(C_1, C_2)和A(C_2, C_1)可以并且会不同，除非亲和性注解被设计成防止这种情况（通过使它们完全对称）。此外，A(C_1, C_2)在为C_1分配资源时被计算并考虑，而A(C_2, C_1)在为C_2分配资源时被计算并考虑。这可能在未来版本中改变。

目前亲和性表达式不支持布尔运算符（与、或、非）。有时，这种限制可以通过使用联合键来克服，特别是与匹配运算符一起。联合键语法允许将多个键的值用分隔符连接成一个单一的值。联合键可以以简单格式或完整格式指定：

- 简单：<colon-separated-subkeys>, this is equivalent to :::<colon-separated-subkeys>
- 完整：<ksep><vsep><ksep-separated-keylist>

联合键评估为所有<ksep>分隔的子键的值，由<vsep>连接。不存在的子键评估为空字符串。例如，联合键

```
:pod/qosclass:pod/name:name
```

评估为

```
<qosclass>:<pod name>:<container name>
```

对于存在运算符，如果联合键的任何子键存在，则认为联合键存在。

## 示例
将容器peter靠近容器sheep放置，但远离容器wolf。

```yaml
metadata:
  annotations:
    cri-resource-manager.intel.com/affinity: |
      peter:
      - match:
          key: name
          operator: Equals
          values:
          - sheep
        weight: 5
    cri-resource-manager.intel.com/anti-affinity: |
      peter:
      - match:
          key: name
          operator: Equals
          values:
          - wolf
        weight: 5
```

#### 简写符号
对于被认为是最常见情况的亲和性定义：在同一Pod内定义容器之间的亲和性，有另一种简写符号。使用这种符号，只需要给出容器的名称，如下例所示。

```yaml
annotations:
  cri-resource-manager.intel.com/affinity: |
    container3: [ container1 ]
  cri-resource-manager.intel.com/anti-affinity: |
    container3: [ container2 ]
    container4: [ container2, container3 ]
```

这种简写符号定义了：

- container3具有对container1的亲和性（权重1）和对container2的反亲和性（权重-1）
- container4具有对container2和container3的反亲和性（权重-1）

等效的完整语法注解将是：

```yaml
metadata:
  annotations:
    cri-resource-manager.intel.com/affinity: |+
      container3
```
