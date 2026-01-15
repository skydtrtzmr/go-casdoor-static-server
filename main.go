package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	ListenAddr    string `json:"listen_addr"`
	BaseURL       string `json:"base_url"`
	QuartzDir     string `json:"quartz_dir"`
	AuthUserParam string `json:"auth_user_param"`
	AuthPwdParam  string `json:"auth_pwd_param"`
	AuthUser      string `json:"auth_user"`
	AuthPwd       string `json:"auth_pwd"`
	CookieMaxAge  int    `json:"cookie_max_age"`
	ForbiddenPage string `json:"forbidden_page"`
}

var conf Config

func main() {
	loadConfig()

	http.HandleFunc("/", handleMain)

	log.Printf("Quartz 网关已启动: %s", conf.BaseURL)
	log.Fatal(http.ListenAndServe(conf.ListenAddr, nil))
}

func handleMain(w http.ResponseWriter, r *http.Request) {
	urlPath := r.URL.Path

	// 放行 favicon
	if urlPath == "/favicon.ico" {
		serveQuartzFile(w, r, urlPath)
		return
	}

	// 放行 401 页面本身
	if urlPath == "/"+conf.ForbiddenPage || urlPath == "/"+conf.ForbiddenPage {
		// 401页面由go服务而非quartz提供。
		http.ServeFile(w, r, conf.ForbiddenPage)
		return
	}

	// 检查认证参数
	userParam := r.URL.Query().Get(conf.AuthUserParam)
	pwdParam := r.URL.Query().Get(conf.AuthPwdParam)

	if userParam != "" && pwdParam != "" {
		// 首次访问带参数，验证并设置 cookie
		if userParam == conf.AuthUser && pwdParam == conf.AuthPwd {
			setAuthCookie(w)
			log.Printf("[AUTH] 参数验证成功")
			// 去除参数后继续处理请求
			removeAuthParams(w, r)
			return
		}
		// 参数错误，重定向到 401 页面
		log.Printf("[AUTH] 参数错误: user=%s", userParam)
		http.Redirect(w, r, "/"+conf.ForbiddenPage, http.StatusFound)
		return
	}

	// 检查 cookie 是否有效
	if !checkAuthCookie(r) {
		log.Printf("[AUTH] 无效的认证，重定向到 401 页面")
		http.Redirect(w, r, "/"+conf.ForbiddenPage, http.StatusFound)
		return
	}

	// 处理路径补全
	finalPath := urlPath
	if strings.HasSuffix(urlPath, "/") {
		finalPath = filepath.Join(urlPath, "index.html")
	} else if filepath.Ext(urlPath) == "" {
		finalPath = urlPath + ".html"
	}

	serveQuartzFile(w, r, finalPath)
}

// 去除 URL 中的认证参数并重定向
func removeAuthParams(w http.ResponseWriter, r *http.Request) {
	// 获取不带参数的路径
	path := r.URL.Path

	// 重定向到纯净路径，cookie 已设置，后续请求会带上 cookie
	http.Redirect(w, r, path, http.StatusFound)
}

// 设置认证 cookie
func setAuthCookie(w http.ResponseWriter) {
	maxAge := conf.CookieMaxAge
	if maxAge <= 0 {
		maxAge = 3600 // 默认 1 小时
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "quartz_auth",
		Value:    "valid",
		Path:     "/",
		HttpOnly: false,
		MaxAge:   maxAge,
	})
}

// 检查认证 cookie 是否有效
func checkAuthCookie(r *http.Request) bool {
	cookie, err := r.Cookie("quartz_auth")
	return err == nil && cookie.Value == "valid"
}

func serveQuartzFile(w http.ResponseWriter, r *http.Request, relPath string) {
	cleanRelPath := filepath.Clean(filepath.FromSlash(relPath))
	fullPath := filepath.Join(conf.QuartzDir, cleanRelPath)

	// 检查是否为 HTML 文件
	isHTML := strings.HasSuffix(relPath, ".html") || strings.HasSuffix(relPath, ".htm")

	if isHTML {
		// 读取文件内容
		file, err := os.Open(fullPath)
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		content, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			http.Error(w, "Read error", http.StatusInternalServerError)
			return
		}

		// 设置缓存控制
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		w.Write(content)
	} else {
		// 非 HTML 文件（JS/CSS/图片等）
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		http.ServeFile(w, r, fullPath)
	}
}

func loadConfig() {
	file, err := os.Open("config.json")
	if err != nil {
		log.Fatal("找不到 config.json")
	}
	defer file.Close()
	if err := json.NewDecoder(file).Decode(&conf); err != nil {
		log.Fatal("配置文件解析错误")
	}
	conf.QuartzDir = filepath.Clean(conf.QuartzDir)

	// 设置默认值
	if conf.AuthUserParam == "" {
		conf.AuthUserParam = "user"
	}
	if conf.AuthPwdParam == "" {
		conf.AuthPwdParam = "pwd"
	}
	if conf.CookieMaxAge <= 0 {
		conf.CookieMaxAge = 3600
	}
	if conf.ForbiddenPage == "" {
		conf.ForbiddenPage = "401.html"
	}
}
