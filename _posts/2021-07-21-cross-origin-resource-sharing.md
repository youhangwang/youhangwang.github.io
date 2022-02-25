---
title: 跨域资源共享
tags: CrossOriginResourceSharing 
--- 

跨域资源共享是一种基于 HTTP Header的机制，该机制通过允许服务器标示除了它自己以外的其它Origin（域，协议和端口），这样浏览器可以加载这些资源。跨域资源共享还通过一种机制来检查托管跨域资源的服务器是否会允许处理将要被发送的真实请求，该机制通过浏览器发起一个到该服务器的`preflight`请求。在preflight请求中，浏览器发送的Header中标示有HTTP方法和真实请求中会用到的Header。<!--more-->

跨域HTTP请求的一个例子：运行在`https://domain-a.com`的 JavaScript代码使用 XMLHttpRequest 发起一个到`https://domain-b.com/data.json`的请求。

出于安全性，浏览器限制脚本内发起的跨域HTTP请求。 例如，XMLHttpRequest和Fetch API都需要遵循同源策略。这意味着使用这些API的Web应用程序只能从加载应用程序的同一个域请求HTTP资源，除非响应报文包含了正确 CORS 响应头。跨域域资源共享（CORS）机制允许 Web 应用服务器进行跨域访问控制，从而使跨域数据传输得以安全进行。现代浏览器支持在API请求中（例如 XMLHttpRequest 或 Fetch）使用CORS，以降低跨域HTTP请求所带来的风险。


## Overview

跨域资源共享标准新增了一组HTTP Header字段，允许服务器声明哪些Origin通过浏览器有权限访问哪些资源。另外，规范要求，对那些可能对服务器数据产生副作用的 HTTP 请求方法（特别是GET以外的HTTP请求，或者搭配某些MIME类型的POST请求），浏览器必须首先使用OPTIONS方法发起一个预检请求（preflight request），从而获知服务端是否允许该跨域请求。服务器确认允许之后，才发起实际的 HTTP 请求。在预检请求的返回中，服务器端也可以通知客户端，是否需要携带身份凭证（包括 Cookies 和 HTTP认证 相关数据）。

CORS请求失败会产生错误，但是为了安全，在JavaScript代码层面是无法获知到底具体是哪里出了问题。你只能查看浏览器的控制台以得知具体是哪里出现了错误。

## HTTP Response Header
本节列出了规范所定义的响应首部字段。

### Access-Control-Allow-Origin
响应Header中可以携带一个[Access-Control-Allow-Origin](https://developer.mozilla.org/zh-CN/docs/Web/HTTP/Headers/Access-Control-Allow-Origin)字段，其语法如下：
```
Access-Control-Allow-Origin: <origin> | * 
```
其中，origin参数的值指定了允许访问该资源的外域URI。对于不需要携带身份凭证的请求，服务器可以指定该字段的值为通配符，表示允许来自所有域的请求。

例如，下面的字段值将允许来自`https://mozilla.org`的请求：
```
Access-Control-Allow-Origin: https://mozilla.org
Vary: Origin
```
如果服务端指定了具体的域名而非`*`，那么响应首部中的Vary字段的值必须包含Origin。这将告诉客户端：`服务器对不同的源站返回不同的内容`。
### Access-Control-Expose-Headers
在跨源访问时，XMLHttpRequest对象的getResponseHeader()方法只能拿到一些最基本的响应Header，例如：Cache-Control、Content-Language、Content-Type、Expires、Last-Modified、Pragma，如果要访问其他Header，则需要服务器设置本Header。

[Access-Control-Expose-Headers](https://developer.mozilla.org/zh-CN/docs/Web/HTTP/Headers/Access-Control-Expose-Headers) Header让服务器把允许浏览器访问的Header放入白名单，例如：
```
Access-Control-Expose-Headers: X-My-Custom-Header, X-Another-Custom-Header
```
这样浏览器就能够通过getResponseHeader访问X-My-Custom-Header和X-Another-Custom-Header响应头了。

### Access-Control-Max-Age
[Access-Control-Max-Age](https://developer.mozilla.org/zh-CN/docs/Web/HTTP/Headers/Access-Control-Max-Age)Header指定了preflight请求的结果能够被缓存多久。
```
Access-Control-Max-Age: <delta-seconds>
```
### Access-Control-Allow-Credentials
[Access-Control-Allow-Credentials](https://developer.mozilla.org/zh-CN/docs/Web/HTTP/Headers/Access-Control-Allow-Credentials)Header指定了当浏览器的 credentials 设置为 true 时是否允许浏览器读取 response 的内容。当用在对 preflight 预检测请求的响应中时，它指定了实际的请求是否可以使用 credentials。请注意：简单 GET 请求不会被预检；如果对此类请求的响应中不包含该字段，这个响应将被忽略掉，并且浏览器也不会将相应内容返回给网页。
```
Access-Control-Allow-Credentials: true
```
### Access-Control-Allow-Methods
[Access-Control-Allow-Methods](https://developer.mozilla.org/zh-CN/docs/Web/HTTP/Headers/Access-Control-Allow-Methods)字段用于预检请求的响应。其指明了实际请求所允许使用的 HTTP 方法。
```
Access-Control-Allow-Methods: <method>[, <method>]*
```
### Access-Control-Allow-Headers
[Access-Control-Allow-Headers](https://developer.mozilla.org/zh-CN/docs/Web/HTTP/Headers/Access-Control-Allow-Headers)字段用于预检请求的响应。其指明了实际请求中允许携带的首部字段。
```
Access-Control-Allow-Headers: <field-name>[, <field-name>]*
```
## HTTP Request Header
本节列出了可用于发起跨源请求的首部字段。这些首部字段无须手动设置。当开发者使用 XMLHttpRequest 对象发起跨源请求时，它们已经被设置就绪。

### Origin
[Origin](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Origin)字段表明预检请求或实际请求的源站。
```
Origin: <origin>
```
origin 参数的值为源站 URI。它不包含任何路径信息，只是服务器名称。有时候将该字段的值设置为空字符串是有用的，例如，当源站是一个 data URL 时。在所有访问控制请求（Access control request）中，Origin 首部字段总是被发送。

### Access-Control-Request-Method
[Access-Control-Request-Method](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Access-Control-Request-Method)字段用于预检请求。其作用是，将实际请求所使用的 HTTP 方法告诉服务器。
```
Access-Control-Request-Method: <method>
```
### Access-Control-Request-Headers
[Access-Control-Request-Headers](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Access-Control-Request-Headers)字段用于预检请求。其作用是，将实际请求所携带的首部字段告诉服务器。
```
Access-Control-Request-Headers: <field-name>[, <field-name>]*
```

## Spec
- [Spec](https://fetch.spec.whatwg.org/#cors-protocol)
- [Documentation](https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS)