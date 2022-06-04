---
title: Prometheus Exporter
tags: Monitoring Exporter Prometheus
--- 

Exporter是Prometheus Server和应用之间的桥梁，应用通过Exporter向外暴露metrics。通常来讲，Exporter可以分为两种类型：
1. 为Application自身输出Metrics的Exporter。实现这种Exporter的方法通常被称为Direct Instrumentation.
2. 从其他Application或者Componet获取Metrics的Exporter，这种Exporter被称为Custom Collectors.

<!--more-->

## Direct Instrumentation

这种方式实现起来相对简单，Prometheus官方也为其提供了通用的编写[指南](https://prometheus.io/docs/practices/instrumentation/)

Direct Instrumentation非常简单，通过一个例子就可以理解：
```
// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// A simple example exposing fictional RPC latencies with different types of
// random distributions (uniform, normal, and exponential) as Prometheus
// metrics.
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	var (
		addr              = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
		uniformDomain     = flag.Float64("uniform.domain", 0.0002, "The domain for the uniform distribution.")
		normDomain        = flag.Float64("normal.domain", 0.0002, "The domain for the normal distribution.")
		normMean          = flag.Float64("normal.mean", 0.00001, "The mean for the normal distribution.")
		oscillationPeriod = flag.Duration("oscillation-period", 10*time.Minute, "The duration of the rate oscillation period.")
	)

	flag.Parse()

	var (
		// Create a summary to track fictional interservice RPC latencies for three
		// distinct services with different latency distributions. These services are
		// differentiated via a "service" label.
		rpcDurations = prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name:       "rpc_durations_seconds",
				Help:       "RPC latency distributions.",
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			},
			[]string{"service"},
		)
		// The same as above, but now as a histogram, and only for the normal
		// distribution. The buckets are targeted to the parameters of the
		// normal distribution, with 20 buckets centered on the mean, each
		// half-sigma wide.
		rpcDurationsHistogram = prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "rpc_durations_histogram_seconds",
			Help:    "RPC latency distributions.",
			Buckets: prometheus.LinearBuckets(*normMean-5**normDomain, .5**normDomain, 20),
		})
	)

	// Register the summary and the histogram with Prometheus's default registry.
	prometheus.MustRegister(rpcDurations)
	prometheus.MustRegister(rpcDurationsHistogram)
	// Add Go module build info.
	prometheus.MustRegister(collectors.NewBuildInfoCollector())

	start := time.Now()

	oscillationFactor := func() float64 {
		return 2 + math.Sin(math.Sin(2*math.Pi*float64(time.Since(start))/float64(*oscillationPeriod)))
	}

	// Periodically record some sample latencies for the three services.
	go func() {
		for {
			v := rand.Float64() * *uniformDomain
			rpcDurations.WithLabelValues("uniform").Observe(v)
			time.Sleep(time.Duration(100*oscillationFactor()) * time.Millisecond)
		}
	}()

	go func() {
		for {
			v := (rand.NormFloat64() * *normDomain) + *normMean
			rpcDurations.WithLabelValues("normal").Observe(v)
			// Demonstrate exemplar support with a dummy ID. This
			// would be something like a trace ID in a real
			// application.  Note the necessary type assertion. We
			// already know that rpcDurationsHistogram implements
			// the ExemplarObserver interface and thus don't need to
			// check the outcome of the type assertion.
			rpcDurationsHistogram.(prometheus.ExemplarObserver).ObserveWithExemplar(
				v, prometheus.Labels{"dummyID": fmt.Sprint(rand.Intn(100000))},
			)
			time.Sleep(time.Duration(75*oscillationFactor()) * time.Millisecond)
		}
	}()

	go func() {
		for {
			v := rand.ExpFloat64() / 1e6
			rpcDurations.WithLabelValues("exponential").Observe(v)
			time.Sleep(time.Duration(50*oscillationFactor()) * time.Millisecond)
		}
	}()

	// Expose the registered metrics via HTTP.
	http.Handle("/metrics", promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
		},
	))
	log.Fatal(http.ListenAndServe(*addr, nil))
}

```

## Custom Collectors

对于从其他Application或者监控系统中的获取Metrics的Exporter，不应该使用Direct Instrumentation方法，并在每次抓取时更新Metrics。这种情况应该在每次抓取时都创建新的指标。在 Go 中，这是通过 Collect() 方法中的 MustNewConstMetric 完成的。

原因有两个：
- 首先，两次抓取Metrics可能同时发生，然而Direct Instrumentation使用的是文件级别的全局变量，因此会产生竞争条件。
- 其次，如果某个Label的Metrics值不存在了，例如某一个node被下线了，但是它仍然会Exporter暴露出来。

实际上，Custom Collectors的实现也非常简单，[haproxy](https://github.com/prometheus/haproxy_exporter/blob/main/haproxy_exporter.go)是一个非常的例子。