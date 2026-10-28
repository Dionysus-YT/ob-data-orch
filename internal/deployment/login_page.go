package deployment

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"html/template"
	"net/http"
	"unicode/utf8"
)

// 登录页在认证前渲染，不能依赖需会话的网页资源；样式、Token 与品牌标识随二进制嵌入，开发与安装包形态一致。
var (
	//go:embed loginpage/login.html
	loginPageHTML string
	//go:embed loginpage/tokens.css
	loginPageTokens string
	//go:embed loginpage/login.css
	loginPageStyles string
	//go:embed loginpage/brand-logo.png
	loginPageLogo []byte
)

var (
	loginTemplate = template.Must(template.New("login").Parse(loginPageHTML))
	// 样式与标识都是编译期嵌入的可信常量，不含任何请求输入，因此可按可信 CSS / URL 注入模板。
	loginStyles = template.CSS(loginPageTokens + loginPageStyles)
	loginLogo   = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(loginPageLogo))
)

type loginView struct {
	Styles   template.CSS
	Logo     template.URL
	Message  string
	Username string
}

// renderLogin 以指定状态码输出登录页；message 只能是固定中文提示，不回显服务端错误或任何口令材料。
// 账户名仅在短且为有效 UTF-8 时回填，由模板转义，其余情况恢复固定管理员账户。
func renderLogin(w http.ResponseWriter, status int, message, username string) {
	if username == "" || len(username) > 64 || !utf8.ValidString(username) {
		username = "admin"
	}
	var page bytes.Buffer
	if err := loginTemplate.Execute(&page, loginView{Styles: loginStyles, Logo: loginLogo, Message: message, Username: username}); err != nil {
		// 模板为编译期常量，失败只可能来自实现缺陷；失败关闭为纯文本，不输出半页内容。
		http.Error(w, "登录页暂不可用", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(page.Bytes())
}
