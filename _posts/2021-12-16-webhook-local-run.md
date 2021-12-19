---
title: 本地运行 Kubernetes Webhook
tags: Kubernetes Webhook
--- 

Kubernetes提供了动态Webhook机制，它是一种用于接收准入请求并对其进行处理的HTTP回调机制。主要包括两种类型：
- validation webhook
- mutation webhook

其中Mutation Webhook会先于Validation Webhook调用，目的是保证Validation Webhook能够对资源的所有修改都进行验证。为了保证Apiserver到Webhook的网络安全，Apiserver使用HTTPS协议同webhook通信。
<!--more-->

## Background

如果读者开发过Operator，那么一定有过这样的使用经验：本地运行controller读取config文件同远端的Kube Apiserver通信，这样可以直接在本地修改代码并实时运行进行debug。

不同于Controller到Apiserver之间的网络连接，webhook到Apiserver的连接是由Apiserver发起的，所以如果想让Webhook像Controller一样能够本地运行方便debug，则要求Apiserver可以主动向webhook server发起网络请求。

本地运行Webhook需要完成以下步骤：
1. 使用Cert-Manager在集群中创建Certficate
2. 将创建好的Cert Copy到本地，方便本地运行的Webhook使用该证书
3. 本地运行Webhook
4. 在Cluster中创建ValidatingWebhookConfiguration或者Mutatingwebhookconfigurations

上述步骤与正常使用Webhook的配置并无本质上的区别，但是细节上有一些不同。

## 创建Certificate

正常使用Webhook时，Cert都是针对该Webhook Service DNS创建的。但是在本地run webhook时，需要使用本机IP。
```
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: k8s-webhook-local-run-serving-cert
  namespace: k8s-webhook-local-run-system
spec:
  ipAddresses:
  - <local ip address>
  issuerRef:
    kind: Issuer
    name: k8s-webhook-local-run-selfsigned-issuer
  secretName: webhook-server-cert
```

如果使用的Cluster是在Mac OS上使用KinD创建的，Docker会为本地IP自动创建一个DNS `host.docker.internal`。也可以使用该DNS创建Cert：
```
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: k8s-webhook-local-run-serving-cert
  namespace: k8s-webhook-local-run-system
spec:
  dnsNames:
  - host.docker.internal
  issuerRef:
    kind: Issuer
    name: k8s-webhook-local-run-selfsigned-issuer
  secretName: webhook-server-cert
```

## 从集群中复制Cert到本地

当Cert-Manager将证书创建成功时，会生成一个Secret保存相关文件。找到Secret，将其内容以文件的形式复制到本地。

## 本地运行Webhook

本地运行Webhook，请注意：
- 推荐使用HTTPS端口443
- 证书地址配置为上一步中复制Cert的地址

## 创建ValidatingWebhookConfiguration或者Mutatingwebhookconfigurations

ValidatingWebhookConfiguration和Mutatingwebhookconfigurations的配置类似，本文只以其中一种为例：
```
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  annotations:
    cert-manager.io/inject-ca-from: <Cert-Namespace>/<Cert-Name>
  name: k8s-webhook-local-run-validating-webhook-configuration
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    url: https://<IP-Address-Or-DNS-In-Cert>/validate-webapp-my-domain-v1-guestbook
  failurePolicy: Fail
  name: vguestbook.kb.io
  rules:
  - apiGroups:
    - webapp.my.domain
    apiVersions:
    - v1
    operations:
    - CREATE
    - UPDATE
    resources:
    - guestbooks
  sideEffects: None
```

其中annotations.cert-manager.io/inject-ca-from的值为证书的namespace/name，该项的作用是使Apiserver承认Cert使用的CA。clientConfig.url为本地运行webhook的URL。

一旦完成上述配置，则Kubernetes集群中Apiserver便可以调用Webhook相关URL。