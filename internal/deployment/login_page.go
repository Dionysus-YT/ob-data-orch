package deployment

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"html/template"
	"net/http"
)

// 登录页在认证前渲染，不能依赖需会话的网页资源；样式、Token、品牌标识与插画随二进制嵌入。
var (
	//go:embed loginpage/login.html
	loginPageHTML string
	//go:embed loginpage/tokens.css
	loginPageTokens string
	//go:embed loginpage/login.css
	loginPageStyles string
	//go:embed loginpage/brand-logo-transparent.png
	loginPageLogo []byte
	//go:embed loginpage/ldo-mark.png
	loginPageMark []byte
	//go:embed loginpage/orchestration.jpg
	loginPageIllustration []byte
)

var (
	loginTemplate = template.Must(template.New("login").Parse(loginPageHTML))
	// 样式和图片均为编译期可信常量；认证前使用内嵌资源，不引入外部请求或会话依赖。
	loginStyles       = template.CSS(loginPageTokens + loginPageStyles)
	loginLogo         = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(loginPageLogo))
	loginMark         = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(loginPageMark))
	loginIllustration = template.URL("data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(loginPageIllustration))
)

type loginView struct {
	Styles       template.CSS
	Logo         template.URL
	Mark         template.URL
	Illustration template.URL
	Message      string
}

// renderLogin 以指定状态码输出登录页；message 只能是固定中文提示，不回显服务端错误或任何口令材料。
// 表单不预置或回填任何账户和口令，刷新及后退不会恢复上次失败输入。
func renderLogin(w http.ResponseWriter, status int, message string) {
	var page bytes.Buffer
	if err := loginTemplate.Execute(&page, loginView{Styles: loginStyles, Logo: loginLogo, Mark: loginMark, Illustration: loginIllustration, Message: message}); err != nil {
		// 模板为编译期常量，失败只可能来自实现缺陷；失败关闭为纯文本，不输出半页内容。
		http.Error(w, "登录页暂不可用", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(page.Bytes())
}
