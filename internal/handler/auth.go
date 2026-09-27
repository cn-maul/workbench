package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	sessionCookie = "wb_session"
	sessionTTL    = 7 * 24 * time.Hour // 自登录起 7 天，到点需重新输密码
)

func hashPassword(pwd string) string {
	sum := sha256.Sum256([]byte("workbench:" + pwd))
	return hex.EncodeToString(sum[:])
}

func (s *Server) sessionValid(c *gin.Context) bool {
	tok, err := c.Cookie(sessionCookie)
	if err != nil || tok == "" {
		return false
	}
	s.SessMu.Lock()
	defer s.SessMu.Unlock()
	loginAt, ok := s.Sessions[tok]
	if !ok {
		return false
	}
	if time.Since(loginAt) > sessionTTL {
		delete(s.Sessions, tok)
		return false
	}
	return true
}

// dropSessions 清空除当前设备外的所有会话（改/清密码后其他设备需重新登录）。
func (s *Server) dropSessions(keep string) {
	s.SessMu.Lock()
	defer s.SessMu.Unlock()
	fresh := map[string]time.Time{}
	if keep != "" {
		if at, ok := s.Sessions[keep]; ok {
			fresh[keep] = at
		}
	}
	s.Sessions = fresh
}

// pruneExpired 清掉全部过期 token。sessionValid 只在 token 被访问命中时删单个，
// 沉底的过期 token 会永久占内存——登录时顺带全表扫一遍（map 很小，代价可忽略）。
func (s *Server) pruneExpired() {
	now := time.Now()
	for tok, at := range s.Sessions {
		if now.Sub(at) > sessionTTL {
			delete(s.Sessions, tok)
		}
	}
}

// RequireAuth 密码为空时放行；否则校验 wb_session cookie（内存态，重启失效，登录后 7 天过期）。
func (s *Server) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.Cfg.PasswordHash == "" || s.sessionValid(c) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "需要密码"})
	}
}

// HandleAuthStatus 前端引导用：是否设了密码、当前会话是否已通过。
func (s *Server) HandleAuthStatus(c *gin.Context) {
	has := s.Cfg.PasswordHash != ""
	c.JSON(200, gin.H{"has_password": has, "authorized": !has || s.sessionValid(c)})
}

// HandleLogin {"password": "..."} → 成功后下发 httpOnly cookie（Path=/ 覆盖页面与下载链接）。
func (s *Server) HandleLogin(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "需要 JSON: {\"password\": \"...\"}")
		return
	}
	want := s.Cfg.PasswordHash
	got := hashPassword(req.Password)
	if want == "" || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
		fail(c, http.StatusUnauthorized, "密码错误")
		return
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		fail(c, 500, err.Error())
		return
	}
	tok := hex.EncodeToString(buf)
	s.SessMu.Lock()
	if s.Sessions == nil {
		s.Sessions = map[string]time.Time{}
	}
	s.pruneExpired()
	s.Sessions[tok] = time.Now()
	s.SessMu.Unlock()
	c.SetCookie(sessionCookie, tok, int(sessionTTL.Seconds()), "/", "", false, true)
	c.JSON(200, gin.H{"ok": true})
}

// HandleLogout 主动退出：删除服务端会话并清 cookie。
func (s *Server) HandleLogout(c *gin.Context) {
	if tok, err := c.Cookie(sessionCookie); err == nil && tok != "" {
		s.SessMu.Lock()
		delete(s.Sessions, tok)
		s.SessMu.Unlock()
	}
	c.SetCookie(sessionCookie, "", -1, "/", "", false, true)
	c.JSON(200, gin.H{"ok": true})
}

// HandlePutAccess 更新访问设置：password 传空串=清除（LAN 一并关闭），lan 传 null=不改。
// 必须已登录（或尚未设密码）才能调用——路由挂在 RequireAuth 之后。
func (s *Server) HandlePutAccess(c *gin.Context) {
	var req struct {
		Password *string `json:"password"`
		Lan      *bool   `json:"lan"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Password == nil && req.Lan == nil) {
		fail(c, 400, `需要 JSON: {"password": "..." 或 null, "lan": true/false 或 null}`)
		return
	}
	oldHash := s.Cfg.PasswordHash
	hash := oldHash
	if req.Password != nil {
		pwd := strings.TrimSpace(*req.Password)
		hash = ""
		if pwd != "" {
			hash = hashPassword(pwd)
		}
	}
	lan := s.Cfg.Lan
	if req.Lan != nil {
		lan = *req.Lan
	}
	if hash == "" {
		lan = false // 无密码不放 LAN（与 Load 的自动失效逻辑一致）
	}
	if err := s.Cfg.SetAccess(hash, lan); err != nil {
		fail(c, 400, err.Error())
		return
	}
	if req.Password != nil && hash != oldHash {
		keep, _ := c.Cookie(sessionCookie)
		s.dropSessions(keep)
	}
	c.JSON(200, accessJSON(s.Cfg.PasswordHash, s.Cfg.Lan))
}

func accessJSON(hash string, lan bool) gin.H {
	return gin.H{"has_password": hash != "", "lan_enabled": lan}
}
