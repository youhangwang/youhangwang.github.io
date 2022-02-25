---
title: 浏览器的同源策略
tags: SameOriginPolicy Security
--- 

同源策略用于限制一个origin加载的文档或者脚本如何能与另一个origin的资源进行交互。它能阻隔潜在的恶意文档，减少被攻击的媒介。例如，它可以防止一个恶意网站通过运行JS脚本从另一个邮件服务（用户已经登陆）或者公司内网中读取数据，并将该数据发送给攻击者。<!--more-->

如果两个URL具有相同的：
- Protocol
- Port
- Host

则这两个URL是同源的。与URL `http://store.company.com/dir/page.html`是否同源的示例如下:

|URL|Result|Reason|
|-|-|-|
|http://store.company.com/dir2/other.html|YES||
|http://store.company.com/dir/inner/another.html|YES||
|https://store.company.com/secure.html|NO|协议不同|
|http://store.company.com:81/dir/etc.html|NO|端口不同|
|http://news.company.com/dir/other.html|NO|主机不同|

### 跨域网络访问

同源策略控制两个不同Origin之间交互，主要可以分为三种：
- Cross-Origin Writes: 一般是允许的，例如Links，Redirection和Form Submission，少数特定的请求需要Preflight。
- Cross-Origin Embedding: 一般是允许的。
- Cross-Origin Reads: 一般是不允许的，但常可以通过内嵌资源来巧妙的进行读取访问。例如，你可以读取嵌入图片的高度和宽度，调用内嵌脚本的方法。

下面是一些可以embedded cross-origin的resources
- <script src="…"></script>的JS
- <link rel="stylesheet" href="…">的CSS
- <img>中的图片
- <video> 和 <audio>中需要播放的媒体.
- <object> 和 <embed>中的外部资源.
- @font-face 应用的字体.
- <iframe>中的任何东西. 网站可以使用 X-Frame-Options header防止cross-origin framing.

#### 如何允许跨域访问

使用[CORS](../../../2021/07/21/cross-origin-resource-sharing.html))

#### 如何阻止跨域访问

- Cross-Origin Writes: 只要检测请求中的一个不可推测的标记(CSRF token)即可，这个标记被称为 Cross-Site Request Forgery (CSRF) 标记。
- Cross-Origin Reads: 请确保资源不可嵌入。通常有必要防止嵌入，因为嵌入资源总是会泄露一些关于它的信息。
- Cross-Origin Embeds: 确保资源不能被解释为上面列出的可嵌入格式之一。当资源不是网站的入口点时，还可以使用CSRF令牌来防止嵌入。

### 跨域数据存储访问
访问存储在浏览器中的数据，如 localStorage 和 IndexedDB，是以Origin进行分割的。每个Origin都拥有自己单独的存储空间，一个源中的 JavaScript 脚本不能对属于其它Origin的数据进行读写操作。

Cookies 使用不同的Origin定义方式。一个页面可以为本域和其父域设置cookie，只要是父域不是公共后缀（public suffix）即可。不管使用哪个协议（HTTP/HTTPS）或端口号，浏览器都允许给定的域以及其任何子域名(sub-domains) 访问 cookie。可以使用 Domain、Path、Secure和 HttpOnly 标记来限定其可访问性。当读取 cookie 时，无法知道它是在哪里被设置的。 即使只使用安全的 https 连接，但任何 cookie 都有可能是使用不安全的连接进行设置的。