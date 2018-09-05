// Package pcsweb web前端包
package pcsweb

import (
	"fmt"
	"github.com/GeertJohan/go.rice"
	"net/http"
	"golang.org/x/net/websocket"
)

var (
	staticBox    *rice.Box // go.rice 文件盒子
	templatesBox *rice.Box // go.rice 文件盒子
)

func boxInit() (err error) {
	staticBox, err = rice.FindBox("static")
	if err != nil {
		return
	}

	templatesBox, err = rice.FindBox("template")
	if err != nil {
		return
	}

	return nil
}

// StartServer 开启web服务
func StartServer(port uint) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}

	err := boxInit()
	if err != nil {
		return err
	}

	http.HandleFunc("/", rootMiddleware)

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(staticBox.HTTPBox())))
	http.HandleFunc("/index.html", middleware(indexPage))

	http.HandleFunc("/api/v1/logout", activeAuthMiddleware(LogoutHandle))
	http.HandleFunc("/api/v1/user", activeAuthMiddleware(UserHandle))
	http.HandleFunc("/api/v1/quota", activeAuthMiddleware(QuotaHandle))
	http.HandleFunc("/api/v1/share", activeAuthMiddleware(ShareHandle))
	http.HandleFunc("/api/v1/download", activeAuthMiddleware(DownloadHandle))
	http.HandleFunc("/api/v1/setting", activeAuthMiddleware(SettingHandle))
	http.HandleFunc("/api/v1/local_file", activeAuthMiddleware(LocalFileHandle))
	http.HandleFunc("/api/v1/file_operation", activeAuthMiddleware(FileOperationHandle))
	http.HandleFunc("/api/v1/mkdir", activeAuthMiddleware(MkdirHandle))
	http.HandleFunc("/api/v1/files", activeAuthMiddleware(fileList))

	http.Handle("/ws", websocket.Handler(WSHandler))

	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}



func indexPage(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	tmpl := boxTmplParse("index", "index.html")
	tmpl.Execute(w, nil)
}
