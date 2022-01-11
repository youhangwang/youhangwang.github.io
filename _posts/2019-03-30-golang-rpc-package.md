---
title: golang rpc package分析
tags: golang rpc
--- 

​最近，由于微服务概念的流行又将RPC这一概念重新带回开发者的视野中，相比构建于HTTP协议之上的RESTful API来讲，RPC通常会构建于tcp协议之上。<!--more-->由于避免了一些HTTP协议的缺点：
1. HTTP是无状态的，导致每次通信会有过于冗余的header。
2. 短链接。HTTP/1.0默认使用短连接，每次通信都会重新发送TCP链接请求，而TCP连接建立时会有一系列握手动作。即使在1.1中默认使用的了长连接，但是该模式下的pipeline依然会存在HOLB问题。同时由于TCP协议的拥塞控制总是慢开始，也会导致性能的下降。
3. 文本传输。HTTP使用文本传输，效率低。

所以rpc的效率通常会比RESTful API高一些，可以节省服务器资源，尤其是对于各个服务之间需要频繁通信的场景。当然，上述HTTP的缺点已经在HTTP/2中解决了。在grpc框架里，google采用的rpc over http/2的解决方案。所以在HTTP/2时代中，个人认为，RESTful API和rpc的性能已经基本没有差距。主要应该是在编程风格的差异上。

废话不多说，直接看code。在MacOS下，rpc包位于/usr/local/go/src/net/rpc，我们先从Client看起。

## Client
在Client中，package提供了三个函数：
1. func DialHTTP(network, address string) (*Client, error)
2. func DialHTTPPath(network, address, path string) (*Client, error)
3. func Dial(network, address string) (*Client, error)

其中`DialHTTP`使用了默认的 path /_goRPC_ 调用http connect方法建立隧道。这里只是使用HTTP的connect方法建立隧道连接，与RESTful API还是有区别的：
```
io.WriteString(conn, "CONNECT "+path+" HTTP/1.0\n\n")
```

而Dial则是单纯的建立一条TCP连接。

建立conn之后就是创建一个rpc client。rpc client的定义如下。主要包含一下几个内容：
1. 编解码器
2. request：包括调用的method，请求的编号和一个指针。
3. seq：请求的编号
4. pending-map：存放着发送出去但是还没有response的call
5. closing，shutdown：client的两种状态

这里的Call和Requst有一些不同。Call指的是一次完整的调用，包含的信息更多。比如本次调用的参数，需要返回的内容和一个能够接受返回内容的channel。如果是非同步调用，该channel必须是buffered。而request只是代表一次请求，内容只有请求的方法，编号和一个指针。这个指针在Client里没有使用，在server端code的分析中我们会再来讨论。除此之外，这里还有两把锁，负责并发下不同的控制粒度。第一把锁reqMutex sync.Mutex可以叫做方法锁，一个client可以并发的发送request，所以它在发送request的时候会被使用。第二把锁mutex sync.Mutex 是map锁，同java一样，go中的map也是非线程安全的。在并发条件下对map进行操作需要上锁。在发送请求的方法中，每发送一个请求就会向pending-map里面添加一个Call。
```
// Client represents an RPC Client.
// There may be multiple outstanding Calls associated
// with a single Client, and a Client may be used by
// multiple goroutines simultaneously.
type Client struct {
    codec ClientCodec

    reqMutex sync.Mutex // protects following
    request  Request

    mutex    sync.Mutex // protects following
    seq      uint64
    pending  map[uint64]*Call
    closing  bool // user has called Close
    shutdown bool // server has told us to stop
}
```

rpc client中还包含一个编解码器，默认情况下会使用gob进行编解码：
```
func NewClient(conn io.ReadWriteCloser) *Client {
    encBuf := bufio.NewWriter(conn)
    client := &gobClientCodec{conn, gob.NewDecoder(conn), gob.NewEncoder(encBuf), encBuf}
    return NewClientWithCodec(client)
}
```

rpc client初始化完成之后会闯将一个pending-map，并新起一个协程client.input()监听server端的response：
```
func NewClientWithCodec(codec ClientCodec) *Client {
    client := &Client{
        codec:   codec,
        pending: make(map[uint64]*Call),
    }
    go client.input()
    return client
}
```

在这个协程中，一个for循环不断的监听server的response，直到发生error为止。response分为两个部分：header和body。header中主要包含两个信息：serviceMethod和一个序列号。这个序列号对应着一个call。首先，client需要解析header。还记得之前初始化的gob client吗？对的，这里需要编解码器的解码。拿到header之后的第一件事就是取出序列号seq。拿到序列号之后去client的pending map中找到对应的call。接着将 body解析到该call中的reply中。并将call放入channel中。
```
func (client *Client) input() {
    var err error
    var response Response
    for err == nil {
        response = Response{}
        err = client.codec.ReadResponseHeader(&response)
        if err != nil {
            break
        }
        seq := response.Seq
        client.mutex.Lock()
        call := client.pending[seq]
        delete(client.pending, seq)
        client.mutex.Unlock()

        switch {
        case call == nil:
            // We've got no pending call. That usually means that
            // WriteRequest partially failed, and call was already
            // removed; response is a server telling us about an
            // error reading request body. We should still attempt
            // to read error body, but there's no one to give it to.
            err = client.codec.ReadResponseBody(nil)
            if err != nil {
                err = errors.New("reading error body: " + err.Error())
            }
        case response.Error != "":
            // We've got an error response. Give this to the request;
            // any subsequent requests will get the ReadResponseBody
            // error if there is one.
            call.Error = ServerError(response.Error)
            err = client.codec.ReadResponseBody(nil)
            if err != nil {
                err = errors.New("reading error body: " + err.Error())
            }
            call.done()
        default:
            err = client.codec.ReadResponseBody(call.Reply)
            if err != nil {
                call.Error = errors.New("reading body " + err.Error())
            }
            call.done()
        }
    }
    // Terminate pending calls.
    client.reqMutex.Lock()
    client.mutex.Lock()
    client.shutdown = true
    closing := client.closing
    if err == io.EOF {
        if closing {
            err = ErrShutdown
        } else {
            err = io.ErrUnexpectedEOF
        }
    }
    for _, call := range client.pending {
        call.Error = err
        call.done()
    }
    client.mutex.Unlock()
    client.reqMutex.Unlock()
    if debugLog && err != io.EOF && !closing {
        log.Println("rpc: client protocol error:", err)
    }
}
```
    
以上部分就是初始化client的code。接下来是client发送请求的code.

rpc package提供了两个方法：
```
func (client *Client) Go(serviceMethod string, args interface{}, reply interface{}, done chan *Call) *Call 
func (client *Client) Call(serviceMethod string, args interface{}, reply interface{}) error
```

Call是同步调用，Go是异步调用。其中Call只是使用了一个non-buffered channel去阻塞的调用Go方法，实现同步调用：
```
// Call invokes the named function, waits for it to complete, and returns its error status.
func (client *Client) Call(serviceMethod string, args interface{}, reply interface{}) error {
    call := <-client.Go(serviceMethod, args, reply, make(chan *Call, 1)).Done
    return call.Error
}
```

Go方法中使用参数初始化call，并调用send方法。
```
call := new(Call)
call.ServiceMethod = serviceMethod
call.Args = args
call.Reply = reply

done = make(chan *Call, 10)
call.Done = done
client.send(call)
```

在sent中初始化request，并且将call放入pending map中发送调用请求：
```
func (client *Client) send(call *Call) {
    client.reqMutex.Lock()
    defer client.reqMutex.Unlock()

    // Register this call.
    client.mutex.Lock()
    if client.shutdown || client.closing {
        client.mutex.Unlock()
        call.Error = ErrShutdown
        call.done()
        return
    }
    seq := client.seq
    client.seq++
    client.pending[seq] = call
    client.mutex.Unlock()

    // Encode and send the request.
    client.request.Seq = seq
    client.request.ServiceMethod = call.ServiceMethod
    err := client.codec.WriteRequest(&client.request, call.Args)
    if err != nil {
        client.mutex.Lock()
        call = client.pending[seq]
        delete(client.pending, seq)
        client.mutex.Unlock()
        if call != nil {
            call.Error = err
            call.done()
        }
    }
}
```
## Server
我们再来看server端的code，rpc包中给了两个函数：
```
// Register publishes the receiver's methods in the DefaultServer.
func Register(rcvr interface{}) error { return DefaultServer.Register(rcvr) }

// RegisterName is like Register but uses the provided name for the type
// instead of the receiver's concrete type.
func RegisterName(name string, rcvr interface{}) error {
    return DefaultServer.RegisterName(name, rcvr)
}
      他们用来将服务注册到rpc server。其中Register会使用服务的类型名来充当服务的名字，而RegisterName则会使用自定义的服务名。在register方法中，会检查服务以及服务提供的方法是否满足要求：比如参数类型，是否是外部可见的变量。并且创建一个service对象：
func (server *Server) register(rcvr interface{}, name string, useName bool) error {
    s := new(service)
    s.typ = reflect.TypeOf(rcvr)
    s.rcvr = reflect.ValueOf(rcvr)
    sname := reflect.Indirect(s.rcvr).Type().Name()
    if useName {
        sname = name
    }
    if sname == "" {
        s := "rpc.Register: no service name for type " + s.typ.String()
        log.Print(s)
        return errors.New(s)
    }
    if !isExported(sname) && !useName {
        s := "rpc.Register: type " + sname + " is not exported"
        log.Print(s)
        return errors.New(s)
    }
    s.name = sname

    // Install the methods
    s.method = suitableMethods(s.typ, true)

    if len(s.method) == 0 {
        str := ""

        // To help the user, see if a pointer receiver would work.
        method := suitableMethods(reflect.PtrTo(s.typ), false)
        if len(method) != 0 {
            str = "rpc.Register: type " + sname + " has no exported methods of suitable type (hint: pass a pointer to value of that type)"
        } else {
            str = "rpc.Register: type " + sname + " has no exported methods of suitable type"
        }
        log.Print(str)
        return errors.New(str)
    }

    if _, dup := server.serviceMap.LoadOrStore(sname, s); dup {
        return errors.New("rpc: service already defined: " + sname)
    }
    return nil
}
```

注册完service之后，就可以调用Package提供的HandleHTTP方法了，这个方法实际上在调用http包提供的handle方法：
```
func (server *Server) HandleHTTP(rpcPath, debugPath string) {
    http.Handle(rpcPath, server)
    http.Handle(debugPath, debugHTTP{server})
}
```

在ServeHTTP方法中，server会使用Hijack()将HTTP对应的TCP连接取出。对client的connect方法作出回应：
```
// ServeHTTP implements an http.Handler that answers RPC requests.
func (server *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
    if req.Method != "CONNECT" {
        w.Header().Set("Content-Type", "text/plain; charset=utf-8")
        w.WriteHeader(http.StatusMethodNotAllowed)
        io.WriteString(w, "405 must CONNECT\n")
        return
    }
    conn, _, err := w.(http.Hijacker).Hijack()
    if err != nil {
        log.Print("rpc hijacking ", req.RemoteAddr, ": ", err.Error())
        return
    }
    io.WriteString(conn, "HTTP/1.0 "+connected+"\n\n")
    server.ServeConn(conn)
}
```

拿到connection之后就可以开始初始化server的编解码器了：
```
func (server *Server) ServeConn(conn io.ReadWriteCloser) {
    buf := bufio.NewWriter(conn)
    srv := &gobServerCodec{
        rwc:    conn,
        dec:    gob.NewDecoder(conn),
        enc:    gob.NewEncoder(buf),
        encBuf: buf,
    }
    server.ServeCodec(srv)
}
```

从connection中解码request。同样分成两个部分header和body。还记得 client部分request有一个指针吗？server端每次解析request的时候，如果freeReq中没有现成的request，就去堆中申请。使用完之后并不交给GC回收，而是使用表链的结构将其添加到freeReq中，这样可以减少go GC的调用。response的申请和取消也是一样的。
```
func (server *Server) getRequest() *Request {
    server.reqLock.Lock()
    req := server.freeReq
    if req == nil {
        req = new(Request)
    } else {
        server.freeReq = req.next
        *req = Request{}
    }
    server.reqLock.Unlock()
    return req
}
func (server *Server) freeRequest(req *Request) {
    server.reqLock.Lock()
    req.next = server.freeReq
    server.freeReq = req
    server.reqLock.Unlock()
}
```

拿到request header之后，从其中取出service和mothod name，并拿到相应的method：
```
func (server *Server) readRequestHeader(codec ServerCodec) (svc *service, mtype *methodType, req *Request, keepReading bool, err error) {
    // Grab the request header.
    req = server.getRequest()
    err = codec.ReadRequestHeader(req)
    if err != nil {
      ....

    }

    // We read the header successfully. If we see an error now,
    // we can still recover and move on to the next request.
    keepReading = true

    dot := strings.LastIndex(req.ServiceMethod, ".")
    if dot < 0 {
        err = errors.New("rpc: service/method request ill-formed: " + req.ServiceMethod)
        return
    }
    serviceName := req.ServiceMethod[:dot]
    methodName := req.ServiceMethod[dot+1:]

    // Look up the request.
    svci, ok := server.serviceMap.Load(serviceName)
    if !ok {
        err = errors.New("rpc: can't find service " + req.ServiceMethod)
        return
    }
    svc = svci.(*service)
    mtype = svc.method[methodName]
    if mtype == nil {
        err = errors.New("rpc: can't find method " + req.ServiceMethod)
    }
    return
}
```
   
读取完request就可以调用service的call方法了，实际上是使用反射执行service的相应方法：
```
func (s *service) call(server *Server, sending *sync.Mutex, wg *sync.WaitGroup, mtype *methodType, req *Request, argv, replyv reflect.Value, codec ServerCodec) {
    if wg != nil {
        defer wg.Done()
    }
    mtype.Lock()
    mtype.numCalls++
    mtype.Unlock()
    function := mtype.method.Func
    // Invoke the method, providing a new value for the reply.
    returnValues := function.Call([]reflect.Value{s.rcvr, argv, replyv})
    // The return value for the method is an error.
    errInter := returnValues[0].Interface()
    errmsg := ""
    if errInter != nil {
        errmsg = errInter.(error).Error()
    }
    server.sendResponse(sending, req, replyv.Interface(), codec, errmsg)
    server.freeRequest(req)
}
```

最后调用sendResponse方法返回response：
```
func (server *Server) sendResponse(sending *sync.Mutex, req *Request, reply interface{}, codec ServerCodec, errmsg string) {
    resp := server.getResponse()
    // Encode the response header
    resp.ServiceMethod = req.ServiceMethod
    if errmsg != "" {
        resp.Error = errmsg
        reply = invalidRequest
    }
    resp.Seq = req.Seq
    sending.Lock()
    err := codec.WriteResponse(resp, reply)
    if debugLog && err != nil {
        log.Println("rpc: writing response:", err)
    }
    sending.Unlock()
    server.freeResponse(resp)
}
```

以上就是golang中rpc包的源码分析。由于golang天生对于高并发的支持，可以在很少工作量的前提下实现一个高性能的server。最近也在看http的源码，有机会再做笔记。
