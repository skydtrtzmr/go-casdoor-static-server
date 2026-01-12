package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"encoding/base64"
)

type Config struct {
	ListenAddr   string `json:"listen_addr"`
	BaseURL      string `json:"base_url"`
	QuartzDir    string `json:"quartz_dir"`
	CasdoorAddr  string `json:"casdoor_addr"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	AppName      string `json:"app_name"`
	RedirectPath string `json:"redirect_path"`
}

var conf Config

func main() {
	loadConfig()

	http.HandleFunc("/callback", handleCallback)
	http.HandleFunc("/logout", handleLogout)
	http.HandleFunc("/", handleMain)

	log.Printf("🚀 Quartz 网关已启动: %s", conf.BaseURL)
	log.Fatal(http.ListenAndServe(conf.ListenAddr, nil))
}

func handleMain(w http.ResponseWriter, r *http.Request) {
	urlPath := r.URL.Path

	// 1. 静态资源直通
	if isStaticResource(urlPath) {
		serveQuartzFile(w, r, urlPath)
		return
	}

	// 2. 鉴权检查
	if !checkAuth(r) {
		redirectToLogin(w, r)
		return
	}

	// 3. Quartz 路径补全
	finalPath := urlPath
	if urlPath == "/" {
		finalPath = "/index.html"
	} else if filepath.Ext(urlPath) == "" {
		finalPath = urlPath + ".html"
	}

	serveQuartzFile(w, r, finalPath)
}

// 获取用户信息并登录
func handleCallback(w http.ResponseWriter, r *http.Request) {
	log.Println("[AUTH] Callback accessed")
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Code missing", http.StatusBadRequest)
		return
	}

	// 1. 去 Casdoor 换取真实的用户名
	realUsername := fetchRealUsername(code)

	// 2. 写入 Session Cookie (保镖用)
	http.SetCookie(w, &http.Cookie{
		Name:     "quartz_session",
		Value:    "is_authenticated",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   3600 * 24 * 7,
	})

	// 3. 写入展示用的用户名 (给 Quartz 组件用)
	// 记得编码，防止中文导致 'å' 报错
	http.SetCookie(w, &http.Cookie{
		Name:     "quartz_username",
		Value:    url.QueryEscape(realUsername),
		Path:     "/",
		HttpOnly: false,
		MaxAge:   3600 * 24 * 7,
	})

	log.Printf("[AUTH] 用户 %s 登录成功，正在跳转首页", realUsername)
	http.Redirect(w, r, "/", http.StatusFound)
}

// 核心：调用 Casdoor 接口获取账号信息
func fetchRealUsername(code string) string {
	// 构造换取 token 的请求
	// 注意：这里为了保持代码精简，使用 Casdoor 提供的简易验证逻辑
	// 实际生产中建议使用 Casdoor SDK
	tokenURL := fmt.Sprintf("%s/api/login/oauth/access_token", conf.CasdoorAddr)
	
	resp, err := http.PostForm(tokenURL, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {conf.ClientID},
		"client_secret": {conf.ClientSecret},
		"code":          {code},
	})

	if err != nil {
		log.Printf("Token 换取失败: %v", err)
		return "Guest"
	}
	defer resp.Body.Close()

	// 解析返回的 JSON
	var data struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(resp.Body).Decode(&data)

	// Casdoor 的 AccessToken 是 JWT 格式，里面包含了用户名
	// 我们可以简单地解析 JWT 的中间部分（Payload）
	parts := strings.Split(data.AccessToken, ".")
	if len(parts) < 2 {
		return "Guest"
	}

	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Name string `json:"name"` // Casdoor 默认在 name 字段存用户名
		ID   string `json:"id"`   // 有些配置下是 id
	}
	json.NewDecoder(strings.NewReader(string(payload))).Decode(&claims)

	if claims.Name != "" {
		return claims.Name
	}
	return claims.ID
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	// 清除所有本地 Cookie
	clearCookie(w, "quartz_session")
	clearCookie(w, "quartz_username")

	// 动态拼接 Casdoor 退出地址
	logoutURL := fmt.Sprintf("%s/api/logout?redirect_uri=%s",
		conf.CasdoorAddr, url.QueryEscape(conf.BaseURL))
	
	http.Redirect(w, r, logoutURL, http.StatusFound)
}

// ---------------- 辅助函数  ----------------

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	loginURL := fmt.Sprintf("%s/login/oauth/authorize?client_id=%s&response_type=code&redirect_uri=%s&scope=read&state=%s",
		conf.CasdoorAddr, conf.ClientID, url.QueryEscape(conf.RedirectPath), conf.AppName)
	http.Redirect(w, r, loginURL, http.StatusFound)
}

func serveQuartzFile(w http.ResponseWriter, r *http.Request, relPath string) {
	fullPath := filepath.Join(conf.QuartzDir, filepath.FromSlash(strings.TrimPrefix(relPath, "/")))
	http.ServeFile(w, r, fullPath)
}

func checkAuth(r *http.Request) bool {
	cookie, err := r.Cookie("quartz_session")
	return err == nil && cookie.Value == "is_authenticated"
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/", MaxAge: -1,
	})
}

func isStaticResource(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	return ext != "" && ext != ".html" && ext != ".htm"
}

func loadConfig() {
	file, err := os.Open("config.json")
	if err != nil {
		log.Fatal("❌ 找不到 config.json")
	}
	defer file.Close()
	if err := json.NewDecoder(file).Decode(&conf); err != nil {
		log.Fatal("❌ 配置文件解析错误")
	}
	conf.QuartzDir = filepath.Clean(conf.QuartzDir)
}