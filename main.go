package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	ListenAddr   string `json:"listen_addr"`
	QuartzDir    string `json:"quartz_dir"` // 绝对路径
	CasdoorAddr  string `json:"casdoor_addr"`
	ClientID     string `json:"client_id"`
	AppName      string `json:"app_name"`
	RedirectPath string `json:"redirect_path"`
}

var conf Config

func main() {
	loadConfig()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		urlPath := r.URL.Path

		// 1. 处理回调
		if urlPath == "/callback" {
			handleCallback(w, r)
			return
		}

		// 2. 静态资源放行 (js/css/图片/字体等)
		if isStaticResource(urlPath) {
			serveQuartzFile(w, r, urlPath)
			return
		}

		// 3. 鉴权拦截
		if !checkAuth(r) {
			log.Printf("[REJECT] %s -> Redirecting to Casdoor", urlPath)
			redirectToLogin(w, r)
			return
		}

		// 4. Quartz 路径处理：如果是访问文件夹或无后缀路径，尝试匹配 .html
		finalPath := urlPath
		if urlPath == "/" {
			finalPath = "/index.html"
		} else if filepath.Ext(urlPath) == "" {
			finalPath = urlPath + ".html"
		}

		log.Printf("[OK] %s -> Serving: %s | %v", urlPath, finalPath, time.Since(start))
		serveQuartzFile(w, r, finalPath)
	})

	log.Printf("🚀 Quartz 网关已启动: http://127.0.0.1%s", conf.ListenAddr)
	log.Printf("📂 守卫绝对路径: %s", conf.QuartzDir)
	log.Fatal(http.ListenAndServe(conf.ListenAddr, nil))
}

// 使用绝对路径直接读取文件，避免 FileServer 的 301 重定向
func serveQuartzFile(w http.ResponseWriter, r *http.Request, relPath string) {
	// 关键：将配置的绝对路径和请求的相对路径拼接
	// 去掉 relPath 开头的 / 以防拼接成奇怪的路径
	fullPath := filepath.Join(conf.QuartzDir, filepath.FromSlash(strings.TrimPrefix(relPath, "/")))
	
	// 检查文件是否存在
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		// 如果补全 .html 后还是不存在，尝试返回 404.html
		errorPage := filepath.Join(conf.QuartzDir, "404.html")
		if _, err404 := os.Stat(errorPage); err404 == nil {
			http.ServeFile(w, r, errorPage)
		} else {
			http.NotFound(w, r)
		}
		return
	}
	
	http.ServeFile(w, r, fullPath)
}

func checkAuth(r *http.Request) bool {
	cookie, err := r.Cookie("quartz_session")
	return err == nil && cookie.Value == "is_authenticated"
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "quartz_session",
		Value:    "is_authenticated",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   3600 * 24 * 7,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	loginURL := fmt.Sprintf("%s/login/oauth/authorize?client_id=%s&response_type=code&redirect_uri=%s&scope=read&state=%s",
		conf.CasdoorAddr, conf.ClientID, strings.ReplaceAll(conf.RedirectPath, ":", "%3A"), conf.AppName)
	http.Redirect(w, r, loginURL, http.StatusFound)
}

func isStaticResource(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	// 只要不是空（无后缀）且不是 .html，就认为是资源文件
	return ext != "" && ext != ".html" && ext != ".htm"
}

func loadConfig() {
	file, err := os.Open("config.json")
	if err != nil {
		log.Fatalf("无法打开 config.json: %v", err)
	}
	defer file.Close()
	json.NewDecoder(file).Decode(&conf)
	// 确保绝对路径是干净的
	conf.QuartzDir = filepath.Clean(conf.QuartzDir)
}