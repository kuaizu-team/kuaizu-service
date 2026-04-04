package handler

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/kuaizu-team/kuaizu-service/api"
)

// EmailUnsubscribe 处理邮件退订
// GET /api/email/unsubscribe?token=xxx
func (s *Server) EmailUnsubscribe(c echo.Context, params api.EmailUnsubscribeParams) error {
	err := s.svc.EmailUnsubscribe.Unsubscribe(c.Request().Context(), params.Token)
	if err != nil {
		return c.HTML(http.StatusBadRequest, unsubscribeErrorHTML(err.Error()))
	}

	return c.HTML(http.StatusOK, unsubscribeSuccessHTML())
}

func unsubscribeSuccessHTML() string {
	return `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>退订成功 - 快组校园</title>
    <style>
        body {
            font-family: 'PingFang SC', 'Microsoft YaHei', Arial, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            margin: 0;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .card {
            background: white;
            border-radius: 16px;
            padding: 40px;
            text-align: center;
            max-width: 400px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
        }
        .icon {
            font-size: 64px;
            margin-bottom: 20px;
        }
        h1 {
            color: #333;
            font-size: 24px;
            margin-bottom: 15px;
        }
        p {
            color: #666;
            font-size: 14px;
            line-height: 1.6;
        }
    </style>
</head>
<body>
    <div class="card">
        <div class="icon">✅</div>
        <h1>退订成功</h1>
        <p>您已成功退订邮件推广通知</p>
        <p>如需重新订阅，请在个人中心设置</p>
    </div>
</body>
</html>`
}

func unsubscribeErrorHTML(message string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>退订失败 - 快组校园</title>
    <style>
        body {
            font-family: 'PingFang SC', 'Microsoft YaHei', Arial, sans-serif;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            min-height: 100vh;
            margin: 0;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .card {
            background: white;
            border-radius: 16px;
            padding: 40px;
            text-align: center;
            max-width: 400px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
        }
        .icon {
            font-size: 64px;
            margin-bottom: 20px;
        }
        h1 {
            color: #333;
            font-size: 24px;
            margin-bottom: 15px;
        }
        p {
            color: #666;
            font-size: 14px;
            line-height: 1.6;
        }
    </style>
</head>
<body>
    <div class="card">
        <div class="icon">❌</div>
        <h1>退订失败</h1>
        <p>%s</p>
    </div>
</body>
</html>`, message)
}
