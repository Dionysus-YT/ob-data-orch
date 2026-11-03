package deployment

import (
	"net/http"
	"strings"
)

// 登录反馈只保存短时固定枚举，不含账户、口令、会话或任意服务端错误，也不授予任何身份。
type loginFeedback string

const (
	loginFeedbackCookie                   = "__Host-ob_data_orch_login_feedback"
	loginInvalidCredentials loginFeedback = "credentials"
	loginInvalidOrigin      loginFeedback = "origin"
	loginInvalidInput       loginFeedback = "input"
	loginRateLimited        loginFeedback = "limited"
	loginUnavailable        loginFeedback = "unavailable"
	loginSessionsFull       loginFeedback = "sessions"
)

func (feedback loginFeedback) details() (int, string) {
	switch feedback {
	case loginInvalidCredentials:
		return http.StatusUnauthorized, "账户或密码不正确"
	case loginInvalidOrigin:
		return http.StatusForbidden, "请求来源无效，请从平台地址重新打开登录页。"
	case loginInvalidInput:
		return http.StatusBadRequest, "登录信息无效，请重新输入。"
	case loginRateLimited:
		return http.StatusTooManyRequests, "尝试过于频繁，请 1 分钟后再试。"
	case loginUnavailable:
		return http.StatusServiceUnavailable, "登录暂时不可用，请稍后重试。"
	case loginSessionsFull:
		return http.StatusServiceUnavailable, "当前有效会话过多，请稍后重试。"
	default:
		return 0, ""
	}
}

// writeLoginFailure 仅对浏览器 HTML 导航执行 POST→303→GET，防止刷新重新发送口令和累计失败次数。
// Accept/导航头只选择反馈方式，不参与来源判断或认证；非导航客户端保留原失败状态。
func writeLoginFailure(w http.ResponseWriter, r *http.Request, feedback loginFeedback) {
	status, message := feedback.details()
	if status == 0 {
		http.Error(w, "登录页暂不可用", http.StatusInternalServerError)
		return
	}
	if r.Header.Get("Sec-Fetch-Mode") == "navigate" || strings.Contains(r.Header.Get("Accept"), "text/html") {
		// 非秘密反馈允许跨站拒绝后的顶级 GET 消费；正式身份 Cookie 仍保持 Strict。
		http.SetCookie(w, &http.Cookie{Name: loginFeedbackCookie, Value: string(feedback), Path: "/", MaxAge: 60, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "60")
	}
	renderLogin(w, status, message)
}

func clearLoginFeedback(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: loginFeedbackCookie, Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

// consumeLoginFeedback 读取后立即删除，刷新只获取干净 GET；伪造或未知值不回显、不进入身份判断。
func consumeLoginFeedback(w http.ResponseWriter, r *http.Request) (int, string) {
	cookie, err := r.Cookie(loginFeedbackCookie)
	if err != nil {
		return http.StatusOK, ""
	}
	clearLoginFeedback(w)
	status, message := loginFeedback(cookie.Value).details()
	if status == 0 {
		return http.StatusOK, ""
	}
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "60")
	}
	return status, message
}
