// OpenCode 飞牛应用网关（Go 实现，架构对齐 androidemu 3.9.1 网关）
//
// 子命令：
//   start      幂等启动：健康则不动（避免掐断在线会话），否则拉起 supervise 守护
//   stop       停止 supervise 与 opencode serve，清理 socket
//   status     通过 socket 探活，退出码 0=运行 3=停止
//   probe      探活并打印详情
//   supervise  守护循环（内部）：flock 互斥、监听 unix socket、看护 opencode serve 子进程、自愈重监听
//
// 访问链路：浏览器 → 飞牛统一网关(HTTPS) → 本网关(app.sock, unix) → 127.0.0.1:14096 (opencode serve)
// 安全约束：无任何 TCP 监听；remote 公网隧道钉死关闭（启动子进程前 sed 兜底 + 环境变量禁用）。
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"sync/atomic"
)

const VERSION = "1.19.3"

var (
	APP_DEST  = envOr("TRIM_APPDEST", "/var/apps/opencode/target")
	VAR_DIR   = envOr("TRIM_PKGVAR", "/var/apps/opencode/var")
	SOCK_PATH = filepath.Join(APP_DEST, "app.sock")
	PREFIX    = "/app/opencode"
	UP_ADDR   = "127.0.0.1:14096"
	OC_PORT   = 14096

	PID_FILE     = vfile("gateway.pid")     // supervise 进程
	SERVE_PID    = vfile("serve.pid")       // opencode serve 子进程
	LOCK_FILE    = vfile("gateway.lock")
	LOG_FILE     = vfile("gateway.log")
	STATUS_FILE  = vfile("gw_status")
	OC_HOME      = filepath.Join(VAR_DIR, "oc_home") // opencode 的 HOME（配置 ~/.opencode 写在这里）
	MAX_HTML_BUF = 4 << 20
)

var PB = []byte(PREFIX)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func vfile(name string) string { return filepath.Join(VAR_DIR, name) }

func logf(format string, a ...interface{}) {
	f, err := os.OpenFile(LOG_FILE, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"),
		fmt.Sprintf(format, a...))
	f.Close()
}

func statusWrite(ok bool, msg string) {
	v := "0"
	if ok {
		v = "1"
	}
	_ = os.WriteFile(STATUS_FILE,
		[]byte(fmt.Sprintf("OK=%s\nMSG=%s\nTS=%d\n", v, msg, time.Now().Unix())), 0644)
}

func archBin() string {
	if runtime.GOARCH == "arm64" {
		return filepath.Join(APP_DEST, "opencode-linux-arm64")
	}
	return filepath.Join(APP_DEST, "opencode-linux-x64")
}

// ---------------------------------------------------------------- 入口 / 探活

func main() {
	_ = os.MkdirAll(VAR_DIR, 0755)
	_ = os.MkdirAll(OC_HOME, 0755)
	cmd := "start"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "start":
		doStart()
	case "stop":
		doStop()
	case "status":
		ok := probe()
		if ok {
			fmt.Println("running")
			os.Exit(0)
		}
		fmt.Println("stopped")
		os.Exit(3)
	case "probe":
		fmt.Println("probe:", probe())
	case "supervise":
		supervise()
	case "version":
		fmt.Println(VERSION)
	default:
		fmt.Println("用法: gateway {start|stop|status|probe|supervise|version}")
		os.Exit(1)
	}
}

// ---------------------------------------------------------------- 探活

func probe() bool {
	c := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", SOCK_PATH)
			},
		},
	}
	resp, err := c.Get("http://unix/gateway/healthz")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// ---------------------------------------------------------------- start / stop

func doStart() {
	if probe() {
		logf("start: 网关健康，跳过")
		statusWrite(true, "already-running")
		return
	}
	// 清理上一版/异常残留的 supervise
	killStale()
	logf("start: 拉起 supervise")
	cmd := exec.Command(os.Args[0], "supervise")
	cmd.Env = append(os.Environ())
	cmd.Stdout, _ = os.OpenFile(LOG_FILE, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	cmd.Stderr = cmd.Stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logf("start: supervise 拉起失败: %v", err)
		statusWrite(false, "spawn-failed")
		os.Exit(1)
	}
	_ = os.WriteFile(PID_FILE, []byte(strconv.Itoa(cmd.Process.Pid)), 0644)
	// 等待 socket 就绪
	for i := 0; i < 30; i++ {
		if probe() {
			statusWrite(true, "started")
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	statusWrite(probe(), "start-timeout")
}

func killStale() {
	for _, pf := range []string{PID_FILE, SERVE_PID} {
		b, err := os.ReadFile(pf)
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil || pid <= 1 {
			continue
		}
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Signal(syscall.SIGTERM)
		}
		_ = os.Remove(pf)
	}
	// 按身份兜底清理本应用进程（cmdline 同时含网关特征与 opencode 路径）
	entries, _ := os.ReadDir("/proc")
	self := os.Getpid()
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		cl, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil {
			continue
		}
		s := string(cl)
		if strings.Contains(s, "supervise") &&
			(strings.Contains(s, APP_DEST) || strings.Contains(s, "opencode")) {
			if p, err := os.FindProcess(pid); err == nil {
				_ = p.Signal(syscall.SIGKILL)
			}
		}
	}
	_ = os.Remove(SOCK_PATH)
}

func doStop() {
	logf("stop: 收到停止指令")
	killStale()
	statusWrite(false, "stopped")
}

// ---------------------------------------------------------------- supervise：守护循环

func supervise() {
	lock, err := acquireLock()
	if err != nil {
		fmt.Println("另一 supervise 实例已在运行")
		return
	}
	defer lock.Close()

	var child *exec.Cmd
	childDead := make(chan struct{})
	var childMu sync.Mutex

	startChild := func() bool {
		childMu.Lock()
		defer childMu.Unlock()
		if child != nil && child.Process != nil {
			// 还活着就不重复拉
			if err := child.Process.Signal(syscall.Signal(0)); err == nil {
				return true
			}
		}
		bin := archBin()
		if _, err := os.Stat(bin); err != nil {
			logf("child: 缺少二进制 %s", bin)
			return false
		}
		c := exec.Command(bin, "serve", "--hostname", "127.0.0.1", "--port", strconv.Itoa(OC_PORT))
		// HOME 指向工作目录：项目选择器从 HOME 列目录；配置文件仍走 OC_HOME（OPENCODE_CONFIG 指定）
		// XDG_* 把应用的缓存/配置/数据重定向回 oc_home，避免项目目录被点目录污染
		home := OC_HOME
		if wd := workDir(); wd != "" {
			c.Dir = wd
			home = wd
		}
		xdg := filepath.Join(OC_HOME, "xdg")
		c.Env = append(os.Environ(),
			"HOME="+home,
			"XDG_DATA_HOME="+filepath.Join(xdg, "data"),
			"XDG_CONFIG_HOME="+filepath.Join(xdg, "config"),
			"XDG_CACHE_HOME="+filepath.Join(xdg, "cache"),
			"XDG_STATE_HOME="+filepath.Join(xdg, "state"),
			"BUN_INSTALL="+filepath.Join(xdg, "bun"),
			"npm_config_cache="+filepath.Join(xdg, "cache", "npm"),
			"OPENCODE_CONFIG="+filepath.Join(OC_HOME, ".opencode", "config.json"),
			"OPENCODE_DISABLE_REMOTE=1",
		)
		logFile, _ := os.OpenFile(vfile("serve.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		c.Stdout = &passScanWriter{inner: logFile}
		c.Stderr = c.Stdout
		c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := c.Start(); err != nil {
			logf("child: 拉起失败: %v", err)
			return false
		}
		child = c
		_ = os.WriteFile(SERVE_PID, []byte(strconv.Itoa(c.Process.Pid)), 0644)
		logf("child: opencode serve 已启动 PID=%d", c.Process.Pid)
		go func() {
			_ = c.Wait()
			select {
			case <-childDead:
			default:
				close(childDead)
			}
		}()
		return true
	}

	go func() {
		for {
			if !startChild() {
				logf("child: 10 秒后重试")
			}
			time.Sleep(10 * time.Second)
		}
	}()

	// 前缀剥离 + shim 注入的反向代理（上游 opencode serve）
	proxy := newProxy()
	// 测试辅助：OC_TEST_PORTAL=<端口> 时，另起一个"模拟飞牛门户"监听（非前缀路径 404），
	// 用于在真实浏览器里复现门户环境（服务端 404，比浏览器拦截更接近真实）。
	if p := os.Getenv("OC_TEST_PORTAL"); p != "" {
		go func() {
			h := http.NewServeMux()
			h.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, PREFIX) {
					proxyRoute(proxy, w, r)
					return
				}
				http.NotFound(w, r)
			})
			logf("test-portal: listen :%s（根路径 404）", p)
			_ = http.ListenAndServe(":"+p, h)
		}()
	}


	// 迁移：把历史版本写进项目目录的应用数据点目录移回 oc_home/xdg（保持项目目录干净）
	migrateDotDirs()

	// 定期清理：opencode 即使设置了 XDG 也会在 HOME 建空的 .local 壳目录，
	// 每 30 秒删一次（仅空目录可删，非空不动，零风险）
	go func() {
		for {
			time.Sleep(30 * time.Second)
			if wd := workDir(); wd != "" {
				_ = os.Remove(filepath.Join(wd, ".local"))
			}
		}
	}()

	// 预热：serve 就绪后先请求一次首页/健康检查，减少用户首次打开的白屏等待
	go func() {
		base := "http://127.0.0.1:" + strconv.Itoa(OC_PORT)
		for i := 0; i < 120; i++ {
			time.Sleep(500 * time.Millisecond)
			resp, err := http.Get(base + "/global/health")
			if err == nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				_, err2 := http.Get(base + "/")
				if err2 == nil {
					logf("warmup: 首页已预热")
				}
				return
			}
		}
	}()

	// 直连端口默认关闭：仅当 oc_home/port_tcp 存在时才启用（令牌鉴权），日常只用门户入口
	if _, err := os.Stat(filepath.Join(OC_HOME, "port_tcp")); err == nil {
		go startTCP(proxy)
	}

	serveOnce := func() error {
		_ = os.Remove(SOCK_PATH)
		lst, err := net.Listen("unix", SOCK_PATH)
		if err != nil {
			return err
		}
		_ = os.Chmod(SOCK_PATH, 0666)
		srv := &http.Server{Handler: mux(proxy), ReadHeaderTimeout: 15 * time.Second}
		logf("listen: %s -> %s", SOCK_PATH, UP_ADDR)
		statusWrite(true, "listening")
		err = srv.Serve(lst)
		return err
	}

	for {
		childDead = make(chan struct{})
		err := serveOnce()
		logf("listen: 退出 (%v)，3 秒后自愈重监听", err)
		statusWrite(false, "re-listening")
		time.Sleep(3 * time.Second)
	}
}

func acquireLock() (*os.File, error) {
	f, err := os.OpenFile(LOCK_FILE, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// ---------------------------------------------------------------- HTTP 层

// servePass：v2.2.0 起 serve 默认启用鉴权并随机生成密码，
// 网关从 serve 输出中捕获后自动注入上游请求，用户无需感知配对流程。
var servePass atomic.Value // string

type passScanWriter struct {
	inner io.Writer
	line  []byte
}

func (w *passScanWriter) Write(p []byte) (int, error) {
	w.line = append(w.line, p...)
	for {
		i := indexByte(w.line, '\n')
		if i < 0 {
			break
		}
		line := string(w.line[:i+1])
		w.line = w.line[i+1:]
		if j := strings.Index(line, "server password "); j >= 0 {
			pw := strings.TrimSpace(line[j+len("server password "):])
			if pw != "" {
				servePass.Store(pw)
				logf("auth: 已捕获 serve 密码（长度 %d）", len(pw))
			}
		}
	}
	return w.inner.Write(p)
}

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}

func newProxy() *httputil.ReverseProxy {
	target := &url.URL{Scheme: "http", Host: UP_ADDR}
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.FlushInterval = -1 // SSE/打字机流式输出即时刷新；WebSocket 升级由 ReverseProxy 内建支持
	// 上游一律回未压缩内容：shim 需要对 HTML 做文本级改写，
	// 若上游回 brotli/gzip 压缩体，改写会破坏数据导致浏览器渲染乱码（v2.2.0 实测）
	origDir := rp.Director
	rp.Director = func(req *http.Request) {
		origDir(req)
		req.Header.Set("Accept-Encoding", "identity")
		if pw, ok := servePass.Load().(string); ok && pw != "" {
			req.SetBasicAuth("opencode", pw)
		}
	}
	// 上游（opencode serve）尚未就绪时等待重试，避免应用刚启动时首次访问出现 502
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			for i := 0; i < 30; i++ { // 最多等约 15 秒
				time.Sleep(500 * time.Millisecond)
				conn, derr := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(OC_PORT), 800*time.Millisecond)
				if derr == nil {
					_ = conn.Close()
					logf("retry: 上游已就绪，重放 %s %s", r.Method, r.URL.Path)
					rp.ServeHTTP(w, r.Clone(r.Context()))
					return
				}
			}
		}
		logf("proxy error: %s %s: %v", r.Method, r.URL.Path, err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta http-equiv="refresh" content="2"><title>OpenCode 启动中</title><style>body{margin:0;font:15px/1.7 system-ui;background:#16181f;color:#e2e8f0;display:flex;min-height:100vh;align-items:center;justify-content:center}</style></head><body><div style="text-align:center"><h1 style="font-size:18px">OpenCode 正在启动…</h1><p style="color:#94a3b8;font-size:13px">服务就绪后会自动刷新，请稍候</p></div></body></html>`)
	}
	rp.ModifyResponse = func(resp *http.Response) error {
		// 1) 站内 3xx 跳转补前缀（对齐 androidemu 的 rewriteLocation）
		if loc := resp.Header.Get("Location"); loc != "" {
			if strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, PREFIX) {
				resp.Header.Set("Location", PREFIX+loc)
			}
		}
		// 2) HTML：整段读出后注入补丁 + 改写路径（避免流式分块只改到第一块）
		if resp.Request != nil && strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") && strings.Contains(resp.Request.Header.Get("Accept"), "text/html") {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(MAX_HTML_BUF)))
			resp.Body.Close()
			out := injectShim(body)
			resp.Body = io.NopCloser(bytes.NewReader(out))
			resp.ContentLength = int64(len(out))
			resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
			resp.Header.Del("Content-Encoding")
			resp.Header.Del("ETag")
			resp.Header.Set("Cache-Control", "no-store, no-cache, must-revalidate")
		}
		// 3) JS：整段读出后改写动态 import 的绝对资源路径（import() 无法在浏览器侧补丁）
		if resp.Request != nil && (strings.HasSuffix(strings.ToLower(resp.Request.URL.Path), ".js") || strings.HasSuffix(strings.ToLower(resp.Request.URL.Path), ".mjs")) {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(MAX_HTML_BUF)))
			resp.Body.Close()
			out := rewriteAbsPaths(body, []byte(`"/assets/`), []byte(`"`+PREFIX+`/assets/`))
			out = rewriteAbsPaths(out, []byte(`'/assets/`), []byte(`'`+PREFIX+`/assets/`))
			out = rewriteAbsPaths(out, []byte(`"/global/`), []byte(`"`+PREFIX+`/global/`))
			out = rewriteAbsPaths(out, []byte(`"/api/`), []byte(`"`+PREFIX+`/api/`))
			out = rewriteAbsPaths(out, []byte(`"/session`), []byte(`"`+PREFIX+`/session`))
			// 应用硬编码的 Vite 预加载助手：资源路径前拼根斜杠 → 改为拼门户前缀
			out = rewriteAbsPaths(out, []byte(`function(e){return"/"+e}`), []byte(`function(e){return"`+PREFIX+`/"+e}`))
			// 前端内置版本号常量滞后于服务端（1.19.2 vs 1.19.3），统一改写为包版本
			out = rewriteAbsPaths(out, []byte(`"1.19.2"`), []byte(`"`+VERSION+`"`))
			resp.Body = io.NopCloser(bytes.NewReader(out))
			resp.ContentLength = int64(len(out))
			resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
			resp.Header.Del("Content-Encoding")
		}
		// 4) CSS：整段读出后改写 url(/...)
		if resp.Request != nil && strings.HasSuffix(strings.ToLower(resp.Request.URL.Path), ".css") {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(MAX_HTML_BUF)))
			resp.Body.Close()
			out := rewriteAbsPaths(body, []byte("url(/"), []byte("url("+PREFIX+"/"))
			resp.Body = io.NopCloser(bytes.NewReader(out))
			resp.ContentLength = int64(len(out))
			resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
			resp.Header.Del("Content-Encoding")
		}
		// 5) SPA 回落保护：非 HTML 请求却拿到 HTML，说明上游兜底了，
		//    前端会把 HTML 当 JSON/SSE 解析而静默失败（白屏），这里纠正为 404
		if resp.Request != nil {
			accept := resp.Request.Header.Get("Accept")
			ct := strings.ToLower(resp.Header.Get("Content-Type"))
			if strings.Contains(ct, "text/html") && !strings.Contains(accept, "text/html") {
				body := []byte(`{"error":"not found","via":"gateway-spa-guard"}`)
				resp.StatusCode = 404
				resp.Header.Set("Content-Type", "application/json; charset=utf-8")
				resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
				resp.ContentLength = int64(len(body))
				resp.Body = io.NopCloser(bytes.NewReader(body))
				logf("guard: 上游对 %s %s 回了 HTML，已纠正为 404", resp.Request.Method, resp.Request.URL.Path)
			}
		}
		return nil
	}
	return rp
}

func mux(rp *httputil.ReverseProxy) http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/gateway/status", func(w http.ResponseWriter, r *http.Request) {
		wd := workDir()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		// 应用自身数据区（VAR_DIR，即 应用文件/opencode）内的目录视为正确位置
		varReal, _ := filepath.EvalSymlinks(VAR_DIR)
		wdReal, _ := filepath.EvalSymlinks(wd)
		visible := strings.HasPrefix(wdReal, varReal)
		_, _ = fmt.Fprintf(w, `{"workdir":%q,"visible":%t}`, wd, visible)
	})

	m.HandleFunc("/gateway/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	})
	// 补丁脚本：门户路径带前缀，必须同时注册带前缀与不带前缀的两种路径
	shimHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, ShimScript())
	}
	m.HandleFunc("/__gw_shim.js", shimHandler)
	m.HandleFunc(PREFIX+"/__gw_shim.js", shimHandler)
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { proxyRoute(rp, w, r) })
	return m
}

// statusWriter：捕获响应码用于请求日志
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(c int) {
	if w.code == 0 {
		w.code = c
	}
	w.ResponseWriter.WriteHeader(c)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.code = 200
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// proxyRoute：剥离 /app/opencode 前缀 → 注入 shim / 改写 CSS → 转发上游
func proxyRoute(rp *httputil.ReverseProxy, w http.ResponseWriter, r *http.Request) {
	sw := &statusWriter{ResponseWriter: w}
	defer func() {
		if sw.code == 0 {
			sw.code = 200
		}
		logf("%s %s -> %d", r.Method, r.URL.Path, sw.code)
	}()
	w = sw
	p := r.URL.Path
	if p == string(PB) {
		p = "/"
	} else if len(p) > len(PB) && p[:len(PB)] == PREFIX {
		p = p[len(PB):]
		if p == "" {
			p = "/"
		}
	}
	// 补丁脚本与 SW 走内部路由，两种入口（socket / TCP）都命中
	if p == "/__gw_shim.js" {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, ShimScript())
		return
	}
	if p == "/__gw_sw.js" {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Service-Worker-Allowed", PREFIX+"/")
		_, _ = io.WriteString(w, GW_SW)
		return
	}
	if p == "/__gw_sw.js" {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Service-Worker-Allowed", PREFIX+"/")
		_, _ = io.WriteString(w, GW_SW)
		return
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = p
	r2.URL.RawPath = ""
	rp.ServeHTTP(w, r2)
}

func wantsHTML(r *http.Request) bool {
	// 导航类请求才注入（API/XHR 拉取的 html 片段通常 Accept 也含 text/html，注入无害但省流起见仅对无扩展名路径注入）
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html")
}

// shimWriter：对 HTML 响应注入 shim；其余响应直接透传。
// 状态机：WriteHeader 先缓存（HTML 才拦截）；缓冲超过上限或首次 Flush 时决策，
// 注入一次性完成后进入直通模式（配合 ReverseProxy FlushInterval=-1 的边写边刷）。
type shimWriter struct {
	rw          http.ResponseWriter
	hdr         http.Header
	code        int  // 待发送状态码（-1 表示尚未 WriteHeader）
	buf         []byte
	passthrough bool // true：一切直接透传
	doneHeader  bool // 已向底层写出 WriteHeader
	flusher     http.Flusher
}

func newShimWriter(w http.ResponseWriter) *shimWriter {
	return &shimWriter{rw: w, hdr: w.Header(), code: -1, flusher: w.(http.Flusher)}
}

func (s *shimWriter) Header() http.Header { return s.hdr }

func (s *shimWriter) emitHeader() {
	if s.doneHeader {
		return
	}
	s.doneHeader = true
	if s.code < 0 {
		s.code = 200
	}
	s.rw.WriteHeader(s.code)
}

func (s *shimWriter) WriteHeader(code int) {
	if s.passthrough {
		s.emitHeader()
		return
	}
	s.code = code
	ct := strings.ToLower(s.hdr.Get("Content-Type"))
	if !strings.Contains(ct, "text/html") {
		s.passthrough = true
		s.emitHeader()
	}
}

func (s *shimWriter) Write(b []byte) (int, error) {
	if s.passthrough {
		s.emitHeader()
		return s.rw.Write(b)
	}
	s.buf = append(s.buf, b...)
	if len(s.buf) > MAX_HTML_BUF {
		// 超大响应放弃注入
		s.passthrough = true
		s.emitHeader()
		_, err := s.rw.Write(s.buf)
		s.buf = nil
		return len(b), err
	}
	return len(b), nil
}

func (s *shimWriter) Flush() {
	if s.passthrough {
		s.emitHeader()
		s.flusher.Flush()
		return
	}
	if s.buf == nil {
		// 关键：首个 body chunk 到来前的 Flush 不能透传——否则底层会提前提交响应头
		// （含原始 Content-Length），后续注入改长 body 就会被服务端掐断。
		return
	}
	// 决策点：注入后转直通
	body := s.buf
	s.buf = nil
	s.passthrough = true
	ce := strings.ToLower(s.hdr.Get("Content-Encoding"))
	if ce != "" && ce != "identity" {
		// 压缩响应无法注入，原样透传
		s.emitHeader()
		_, _ = s.rw.Write(body)
		s.flusher.Flush()
		return
	}
	// 注入会改变 body 长度，必须删掉上游的 Content-Length（转 chunked）
	s.hdr.Del("Content-Length")
	out := injectShim(body)
	s.emitHeader()
	_, _ = s.rw.Write(out)
	s.flusher.Flush()
}

// CSS 响应改写器：把 url(/x) 改成 url(/app/opencode/x)
type cssWriter struct {
	rw          http.ResponseWriter
	hdr         http.Header
	code        int
	buf         []byte
	passthrough bool
	doneHeader  bool
	flusher     http.Flusher
}

func newCSSWriter(w http.ResponseWriter) *cssWriter {
	return &cssWriter{rw: w, hdr: w.Header(), code: -1, flusher: w.(http.Flusher)}
}

func (s *cssWriter) Header() http.Header { return s.hdr }

func (s *cssWriter) emitHeader() {
	if s.doneHeader {
		return
	}
	s.doneHeader = true
	if s.code < 0 {
		s.code = 200
	}
	s.rw.WriteHeader(s.code)
}

func (s *cssWriter) WriteHeader(code int) {
	s.code = code
}

func (s *cssWriter) Write(b []byte) (int, error) {
	if s.passthrough {
		s.emitHeader()
		return s.rw.Write(b)
	}
	s.buf = append(s.buf, b...)
	if len(s.buf) > MAX_HTML_BUF {
		s.passthrough = true
		s.emitHeader()
		_, _ = s.rw.Write(s.buf)
		s.buf = nil
	}
	return len(b), nil
}

func (s *cssWriter) Flush() {
	if s.passthrough {
		s.emitHeader()
		s.flusher.Flush()
		return
	}
	if s.buf == nil {
		return
	}
	out := rewriteAbsPaths(s.buf, []byte("url(/"), []byte("url("+PREFIX+"/"))
	s.buf = nil
	s.passthrough = true
	s.hdr.Del("Content-Length")
	s.hdr.Set("Cache-Control", "no-store, must-revalidate")
	s.emitHeader()
	_, _ = s.rw.Write(out)
	s.flusher.Flush()
}

// CloseNotify 透传（ReverseProxy 需要时退化）
func (s *cssWriter) CloseNotify() <-chan bool {
	if cn, ok := s.rw.(http.CloseNotifier); ok {
		return cn.CloseNotify()
	}
	ch := make(chan bool, 1)
	return ch
}

// CloseNotify 透传（ReverseProxy 需要时退化）
func (s *shimWriter) CloseNotify() <-chan bool {
	if cn, ok := s.rw.(http.CloseNotifier); ok {
		return cn.CloseNotify()
	}
	ch := make(chan bool, 1)
	return ch
}

// ---------------------------------------------------------------- 直连端口（令牌鉴权）

func tokenPath() string { return filepath.Join(OC_HOME, ".access_token") }

func ensureToken() string {
	if b, err := os.ReadFile(tokenPath()); err == nil {
		if t := strings.TrimSpace(string(b)); len(t) >= 16 {
			return t
		}
	}
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	t := hex.EncodeToString(buf)
	_ = os.WriteFile(tokenPath(), []byte(t+"\n"), 0600)
	logf("tcp: 已生成访问令牌 %s", tokenPath())
	return t
}

func tcpPort() string {
	if b, err := os.ReadFile(filepath.Join(OC_HOME, "port_tcp")); err == nil {
		if p := strings.TrimSpace(string(b)); p != "" {
			if n, err := strconv.Atoi(p); err == nil && n >= 1024 && n <= 65535 {
				return p
			}
		}
	}
	return "4096"
}

func startTCP(rp *httputil.ReverseProxy) {
	tok := ensureToken()
	addr := ":" + tcpPort()
	for {
		h := http.NewServeMux()
		h.HandleFunc("/gateway/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			_, _ = w.Write([]byte("ok"))
		})
		h.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// 令牌校验：cookie 或 ?token=（校验通过后种 cookie 并去参重定向）
			if q := r.URL.Query().Get("token"); q != "" && q == tok {
				http.SetCookie(w, &http.Cookie{
					Name: "oc_token", Value: tok, Path: "/", HttpOnly: true,
					MaxAge: 30 * 24 * 3600, SameSite: http.SameSiteLaxMode,
				})
				qq := r.URL.Query()
				qq.Del("token")
				u := *r.URL
				u.RawQuery = qq.Encode()
				target := u.Path
				if target == "" {
					target = "/"
				}
				if u.RawQuery != "" {
					target += "?" + u.RawQuery
				}
				http.Redirect(w, r, target, http.StatusFound)
				return
			}
			ok := false
			if c, err := r.Cookie("oc_token"); err == nil && c.Value == tok {
				ok = true
			}
			if !ok {
				loginPage(w)
				return
			}
			sw := &statusWriter{ResponseWriter: w}
			defer func() {
				if sw.code == 0 {
					sw.code = 200
				}
				logf("tcp %s %s -> %d", r.Method, r.URL.Path, sw.code)
			}()
			// 直连入口同时兼容带前缀与不带前缀两种路径（便于自测与反向代理）
			proxyRoute(rp, sw, r)
		})
		srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 15 * time.Second}
		logf("tcp: listen %s（令牌鉴权）", addr)
		if err := srv.ListenAndServe(); err != nil {
			logf("tcp: 退出 %v，3 秒后重试", err)
			time.Sleep(3 * time.Second)
		}
	}
}

func loginPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)
	_, _ = io.WriteString(w, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>OpenCode · 访问验证</title>
<style>body{margin:0;font:15px/1.7 system-ui,-apple-system,"Segoe UI",sans-serif;background:#16181f;color:#e2e8f0;display:flex;min-height:100vh;align-items:center;justify-content:center}
.card{background:#1e222c;border:1px solid #2c3341;border-radius:14px;padding:28px 30px;max-width:420px;width:92%}
h1{font-size:19px;margin:0 0 10px}p{color:#94a3b8;margin:0 0 16px;font-size:14px}
input{width:100%;box-sizing:border-box;padding:10px 12px;border-radius:9px;border:1px solid #2c3341;background:#14171f;color:#e2e8f0;font-size:14px}
button{margin-top:12px;width:100%;padding:10px 12px;border:0;border-radius:9px;background:#5eead4;color:#0b1020;font-weight:600;cursor:pointer}
code{background:#14171f;padding:1px 5px;border-radius:5px;font-size:12.5px;word-break:break-all}
.tip{margin-top:14px;font-size:12.5px;color:#64748b}</style></head><body>
<div class="card"><h1>OpenCode 直连访问</h1>
<p>此入口需要访问令牌。令牌在服务端文件：</p>
<p><code>/vol1/@appdata/opencode/oc_home/.access_token</code></p>
<form method="get" action="/"><input name="token" placeholder="粘贴访问令牌" autofocus autocomplete="off">
<button type="submit">进入</button></form>
<div class="tip">也可直接访问 「/你的项目路径?token=令牌」；验证一次后浏览器记住 30 天。不想用直连端口可改用飞牛门户入口。</div>
</div></body></html>`)
}

// workDir：opencode 的默认工作目录（项目根）。
// 可通过 oc_home/workdir 覆盖；默认优先 /vol1/1000（用户共享目录），否则用应用数据目录下的 workspace。
// migrateDotDirs：历史版本曾把 HOME 设为项目目录，应用在项目目录里生成了
// .local/.config 等数据目录；统一搬回 oc_home/xdg，保持项目目录只放用户项目。
func migrateDotDirs() {
	wd := workDir()
	if wd == "" {
		return
	}
	for _, name := range []string{".local", ".config", ".cache", ".opencode"} {
		src := filepath.Join(wd, name)
		st, err := os.Lstat(src)
		if err != nil || !st.IsDir() {
			continue
		}
		dst := filepath.Join(OC_HOME, "xdg", strings.TrimPrefix(name, "."), "migrated")
		if err := os.MkdirAll(dst, 0755); err != nil {
			continue
		}
		_ = exec.Command("mv", src, filepath.Join(dst, name)).Run()
	}
}

func workDir() string {
	// 项目目录：应用数据区内固定位置（文件管理 → 应用文件 → opencode → projects 可见）
	p := filepath.Join(VAR_DIR, "projects")
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return p
	}
	if err := os.MkdirAll(filepath.Join(p, "我的项目"), 0775); err == nil {
		return p
	}
	return VAR_DIR
}

