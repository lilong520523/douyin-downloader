package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const (
	userAgent = "Mozilla/5.0 (Linux; U; Android 8.1.0; zh-cn; BLA-AL00 Build/HUAWEIBLA-AL00) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/57.0.2987.132 MQQBrowser/8.9 Mobile Safari/537.36"
	PASSWORD  = "as123456" // 验证密码
)

// 从分享文本中提取第一个URL
func extractURL(text string) string {
	re := regexp.MustCompile(`[a-zA-Z]+://[^\s]+`)
	return re.FindString(text)
}

// 从URL中提取视频ID
func extractVideoID(videoURL string) string {
	re := regexp.MustCompile(`(?:video|douyin\.com)/(\d+)`)
	matches := re.FindStringSubmatch(videoURL)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// 获取重定向后的真实地址
func getRedirectedURL(initialURL string) (string, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest("GET", initialURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("未找到 Location 头")
	}
	return location, nil
}

// 解析无水印视频地址
func getVideoRealURL(redirectedURL string) (string, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", redirectedURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	html := string(body)
	start := strings.Index(html, `"play_addr":`)
	if start == -1 {
		return "", fmt.Errorf("未找到 play_addr 字段")
	}

	braceStart := strings.Index(html[start:], "{")
	if braceStart == -1 {
		return "", fmt.Errorf("未找到 play_addr 对象起始")
	}
	braceStart += start

	depth := 0
	end := -1
	for i := braceStart; i < len(html); i++ {
		if html[i] == '{' {
			depth++
		} else if html[i] == '}' {
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
	}
	if end == -1 {
		return "", fmt.Errorf("未找到 play_addr 结尾")
	}

	playAddrJSON := html[braceStart : end+1]
	var playAddr struct {
		URLList []string `json:"url_list"`
	}
	if err := json.Unmarshal([]byte(playAddrJSON), &playAddr); err != nil {
		return "", fmt.Errorf("解析JSON失败: %v", err)
	}
	if len(playAddr.URLList) == 0 {
		return "", fmt.Errorf("url_list 为空")
	}

	rawURL := playAddr.URLList[0]
	rawURL = strings.ReplaceAll(rawURL, `\u002F`, "/")
	rawURL = strings.ReplaceAll(rawURL, "playwm", "play")
	return rawURL, nil
}

// 下载视频
func downloadVideo(w http.ResponseWriter, videoURL string) error {
	client := &http.Client{}
	req, err := http.NewRequest("GET", videoURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("视频源状态码错误: %d", resp.StatusCode)
	}

	w.Header().Set("Content-Type", "video/mp4")
	_, err = io.Copy(w, resp.Body)
	return err
}

// 密码输入界面（只显示：拨号按钮 + 密码框）
func loginPage(errorMsg string) string {
	errHtml := ""
	if errorMsg != "" {
		errHtml = `<p style="color:red;text-align:center;">` + errorMsg + `</p>`
	}

	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>验证</title>
    <style>
        body {font-family: 'Segoe UI',sans-serif; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); margin:0; padding:0; display:flex; justify-content:center; align-items:center; height:100vh;}
        .container {background:white; border-radius:10px; box-shadow:0 10px 25px rgba(0,0,0,0.1); padding:40px; width:90%; max-width:500px;}
        h2 {text-align:center; color:#333; margin-bottom:25px;}
        .form-item {margin-bottom:20px;}
        label {font-size:14px; font-weight:600; color:#555; margin-bottom:8px; display:block;}
        input {padding:12px 15px; border:1px solid #ddd; border-radius:5px; font-size:16px; width:100%; box-sizing:border-box;}
        .btn {width:100%; padding:14px; border:none; border-radius:8px; font-size:16px; font-weight:bold; cursor:pointer; margin-top:10px;}
        .submit-btn {background:#667eea; color:white;}
        .call-btn {background:#25D366; color:white; margin-top:15px;}
    </style>
</head>
<body>
    <div class="container">
        <h2>请输入密码</h2>
        ` + errHtml + `
        <form method="post">
            <div class="form-item">
                <label>密码</label>
                <input type="password" name="password" placeholder="请输入密码" required>
            </div>
            <button type="submit" class="btn submit-btn">进入工具</button>
        </form>

        <!-- 拨号按钮 -->
        <a href="tel:18568561913" style="text-decoration:none;">
            <button class="btn call-btn">📞 联系客服</button>
        </a>
    </div>
</body>
</html>
    `
}

// 视频下载主界面
func mainPage() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>抖音无水印下载</title>
    <style>
        body {font-family: 'Segoe UI',sans-serif; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); margin:0; padding:0; display:flex; justify-content:center; align-items:center; height:100vh;}
        .container {background:white; border-radius:10px; box-shadow:0 10px 25px rgba(0,0,0,0.1); padding:40px; width:90%; max-width:500px;}
        h1 {text-align:center; color:#333; margin-bottom:30px;}
        form {display:flex; flex-direction:column;}
        label {font-size:14px; font-weight:600; color:#555; margin-bottom:8px;}
        input {padding:12px 15px; border:1px solid #ddd; border-radius:5px; font-size:16px; margin-bottom:20px;}
        input:focus {border-color:#667eea; outline:none;}
        .button-group {display:flex; gap:10px;}
        button {flex:1; padding:12px; border:none; border-radius:5px; font-size:16px; font-weight:600; cursor:pointer;}
        button[type="submit"] {background:#667eea; color:white;}
        button[type="button"] {background:#e2e8f0; color:#333;}
    </style>
    <script>function clearInput(){document.getElementById('url').value='';}</script>
</head>
<body>
    <div class="container">
        <h1>抖音无水印下载</h1>
        <form action="/" method="post">
            <label for="url">粘贴分享链接/文本：</label>
            <input type="text" id="url" name="url" placeholder="https://v.douyin.com/xxx/" required>
            <div class="button-group">
                <button type="submit">下载视频</button>
                <button type="button" onclick="clearInput()">清空</button>
            </div>
        </form>
    </div>
</body>
</html>
    `
}

// Vercel 入口函数
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// 密码验证
	if r.Method == http.MethodPost {
		r.ParseForm()
		pwd := strings.TrimSpace(r.FormValue("password"))
		videoUrl := strings.TrimSpace(r.FormValue("url"))

		// 处理密码提交
		if pwd != "" {
			if pwd == PASSWORD {
				fmt.Fprint(w, mainPage())
				return
			}
			fmt.Fprint(w, loginPage("密码错误，请重试"))
			return
		}

		// 处理视频下载
		if videoUrl != "" {
			videoPageURL := extractURL(videoUrl)
			if videoPageURL == "" {
				http.Error(w, "未检测到有效链接", http.StatusBadRequest)
				return
			}

			redirectedURL, err := getRedirectedURL(videoPageURL)
			if err != nil {
				http.Error(w, "解析失败："+err.Error(), http.StatusInternalServerError)
				return
			}

			realURL, err := getVideoRealURL(redirectedURL)
			if err != nil {
				http.Error(w, "获取视频失败："+err.Error(), http.StatusInternalServerError)
				return
			}

			videoID := extractVideoID(videoPageURL)
			if videoID == "" {
				videoID = "douyin"
			}

			w.Header().Set("Content-Disposition", "attachment; filename="+videoID+".mp4")
			_ = downloadVideo(w, realURL)
			return
		}
	}

	// 默认显示密码界面
	fmt.Fprint(w, loginPage(""))
}
